// Package board is the read model behind the supervisor live board: queues, calls in progress
// and agent states, limited to the departments the signed-in user may see (D22).
package board

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/presence"
	"callcenter/internal/realtime"
)

type AgentRow struct {
	ID       uuid.UUID      `json:"id"`
	FullName string         `json:"full_name"`
	State    presence.State `json:"state"`
}

type LiveCall struct {
	ID         uuid.UUID `json:"id"`
	Status     string    `json:"status"` // ringing | active
	CallerName *string   `json:"caller_name"`
	AgentName  *string   `json:"agent_name"`
	Since      time.Time `json:"since"` // when it started ringing (queued_at) or was answered
}

type DepartmentBoard struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	Queued            int        `json:"queued"`
	OldestQueuedSince *time.Time `json:"oldest_queued_since"`
	Agents            []AgentRow `json:"agents"`
	Calls             []LiveCall `json:"calls"`
}

type Board struct {
	Departments []DepartmentBoard `json:"departments"`
}

type Service struct {
	tx database.TxManager
}

func NewService(tx database.TxManager) *Service { return &Service{tx: tx} }

// Get returns the board for the signed-in supervisor or tenant admin.
func (s *Service) Get(ctx context.Context) (*Board, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var b *Board
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		b, err = load(ctx, p)
		return err
	})
	return b, err
}

// Topics returns the event topics for the departments the user may see.
func (s *Service) Topics(ctx context.Context) ([]string, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var topics []string
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		ids, err := visibleDepartments(ctx, p)
		if err != nil {
			return err
		}
		for _, id := range ids {
			topics = append(topics, realtime.DepartmentTopic(id))
		}
		return nil
	})
	return topics, err
}

// visibleDepartments: tenant admins see every active department, supervisors only their own.
func visibleDepartments(ctx context.Context, p reqctx.Principal) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT d.id FROM departments d
		WHERE d.is_active AND ($1 OR EXISTS (SELECT 1 FROM department_supervisors s WHERE s.department_id = d.id AND s.user_id = $2))
		ORDER BY d.name`, p.Role == reqctx.RoleTenantAdmin, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("list visible departments: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan department: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func load(ctx context.Context, p reqctx.Principal) (*Board, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := visibleDepartments(ctx, p)
	if err != nil {
		return nil, err
	}
	b := &Board{Departments: make([]DepartmentBoard, 0, len(ids))}
	index := make(map[uuid.UUID]*DepartmentBoard, len(ids))

	rows, err := q.Query(ctx, `SELECT id, name FROM departments WHERE id = ANY($1) ORDER BY name`, ids)
	if err != nil {
		return nil, fmt.Errorf("board departments: %w", err)
	}
	for rows.Next() {
		d := DepartmentBoard{Agents: []AgentRow{}, Calls: []LiveCall{}}
		if err := rows.Scan(&d.ID, &d.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan board department: %w", err)
		}
		b.Departments = append(b.Departments, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range b.Departments {
		index[b.Departments[i].ID] = &b.Departments[i]
	}

	if rows, err = q.Query(ctx, `
		SELECT department_id, count(*), min(queued_at) FROM calls
		WHERE status = 'queued' AND department_id = ANY($1) GROUP BY department_id`, ids); err != nil {
		return nil, fmt.Errorf("board queues: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		var n int
		var oldest time.Time
		if err := rows.Scan(&id, &n, &oldest); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan board queue: %w", err)
		}
		if d := index[id]; d != nil {
			d.Queued, d.OldestQueuedSince = n, &oldest
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if rows, err = q.Query(ctx, `
		SELECT c.department_id, c.id, c.status, c.caller_name, coalesce(au.full_name, ou.full_name),
		       coalesce(c.answered_at, c.queued_at)
		FROM calls c
		LEFT JOIN users au ON au.id = c.assigned_agent_id
		LEFT JOIN call_offers o ON o.call_id = c.id AND o.outcome = 'pending'
		LEFT JOIN users ou ON ou.id = o.agent_id
		WHERE c.status IN ('ringing', 'active') AND c.department_id = ANY($1)
		ORDER BY coalesce(c.answered_at, c.queued_at)`, ids); err != nil {
		return nil, fmt.Errorf("board calls: %w", err)
	}
	for rows.Next() {
		var dept uuid.UUID
		var c LiveCall
		if err := rows.Scan(&dept, &c.ID, &c.Status, &c.CallerName, &c.AgentName, &c.Since); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan board call: %w", err)
		}
		if d := index[dept]; d != nil {
			d.Calls = append(d.Calls, c)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if rows, err = q.Query(ctx, `
		SELECT m.department_id, u.id, u.full_name, coalesce(p.status, 'offline'),
		       coalesce(p.last_heartbeat_at > now() - make_interval(secs => $2), false),
		       EXISTS (SELECT 1 FROM call_offers o WHERE o.agent_id = u.id AND o.outcome = 'pending'),
		       EXISTS (SELECT 1 FROM calls c WHERE c.assigned_agent_id = u.id AND c.status = 'active')
		FROM department_members m
		JOIN users u ON u.id = m.user_id AND u.status = 'active'
		LEFT JOIN agent_presence p ON p.user_id = u.id
		WHERE m.department_id = ANY($1)
		ORDER BY u.full_name, u.id`, ids, presence.StaleAfter.Seconds()); err != nil {
		return nil, fmt.Errorf("board agents: %w", err)
	}
	for rows.Next() {
		var dept uuid.UUID
		var a AgentRow
		var status presence.Status
		var fresh, hasOffer, hasCall bool
		if err := rows.Scan(&dept, &a.ID, &a.FullName, &status, &fresh, &hasOffer, &hasCall); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan board agent: %w", err)
		}
		a.State = presence.Effective(status, fresh, hasOffer, hasCall)
		if d := index[dept]; d != nil {
			d.Agents = append(d.Agents, a)
		}
	}
	rows.Close()
	return b, rows.Err()
}
