package limits

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
)

type Repository interface {
	Current(ctx context.Context, tenantID uuid.UUID) (*Current, error)
	Usage(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (Usage, error)
	// SeatsAfterReplacing counts the distinct people who would hold a seat if the department's
	// members became userIDs.
	SeatsAfterReplacing(ctx context.Context, tenantID, departmentID uuid.UUID, userIDs []uuid.UUID) (int, error)
	// IsAgent reports whether the user is a (non-disabled) member of any department.
	IsAgent(ctx context.Context, tenantID, userID uuid.UUID) (bool, error)
	// SeatHolders returns the users who keep a seat when only max seats exist: the earliest memberships.
	SeatHolders(ctx context.Context, tenantID uuid.UUID, max int) ([]uuid.UUID, error)
	AgentsBeyond(ctx context.Context, tenantID uuid.UUID, max int) ([]Person, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func (pgRepository) Current(ctx context.Context, tenantID uuid.UUID) (*Current, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var c Current
	err = q.QueryRow(ctx, `
		SELECT p.id, p.max_agents, p.max_call_minutes_per_month, p.max_storage_bytes, p.max_departments, p.max_widgets,
		       ts.minutes_cap_policy, ts.storage_cap_policy, s.current_period_start, s.current_period_end
		FROM subscriptions s
		JOIN plans p ON p.id = s.plan_id
		JOIN tenant_settings ts ON ts.tenant_id = s.tenant_id
		WHERE s.tenant_id = $1`, tenantID).
		Scan(&c.PlanID, &c.Caps.MaxAgents, &c.Caps.MaxCallMinutes, &c.Caps.MaxStorageBytes, &c.Caps.MaxDepartments,
			&c.Caps.MaxWidgets, &c.Policies.Minutes, &c.Policies.Storage, &c.PeriodStart, &c.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("current plan limits: %w", err)
	}
	return &c, nil
}

const seatMembers = `
	SELECT dm.user_id FROM department_members dm
	JOIN users u ON u.id = dm.user_id AND u.status <> 'disabled'
	WHERE dm.tenant_id = $1`

func (pgRepository) Usage(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (Usage, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return Usage{}, err
	}
	var u Usage
	err = q.QueryRow(ctx, `
		SELECT
		  (SELECT count(DISTINCT user_id) FROM (`+seatMembers+`) m)::int,
		  (SELECT count(*) FROM departments WHERE tenant_id = $1)::int,
		  (SELECT count(*) FROM widgets WHERE tenant_id = $1)::int,
		  (SELECT coalesce(sum(extract(epoch FROM ended_at - answered_at)), 0)::bigint FROM calls
		     WHERE tenant_id = $1 AND answered_at >= $2 AND answered_at < $3 AND ended_at IS NOT NULL),
		  (SELECT coalesce(sum(size_bytes), 0)::bigint FROM recordings WHERE tenant_id = $1)`,
		tenantID, from, to).Scan(&u.Agents, &u.Departments, &u.Widgets, &u.CallSeconds, &u.StorageBytes)
	if err != nil {
		return Usage{}, fmt.Errorf("tenant usage: %w", err)
	}
	return u, nil
}

func (pgRepository) SeatsAfterReplacing(ctx context.Context, tenantID, departmentID uuid.UUID, userIDs []uuid.UUID) (int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	err = q.QueryRow(ctx, `
		SELECT count(DISTINCT user_id) FROM (
		    SELECT dm.user_id FROM department_members dm
		    JOIN users u ON u.id = dm.user_id AND u.status <> 'disabled'
		    WHERE dm.tenant_id = $1 AND dm.department_id <> $2
		  UNION
		    SELECT unnest($3::uuid[])
		) x`, tenantID, departmentID, userIDs).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count seats after change: %w", err)
	}
	return n, nil
}

func (pgRepository) IsAgent(ctx context.Context, tenantID, userID uuid.UUID) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM department_members WHERE tenant_id = $1 AND user_id = $2)`,
		tenantID, userID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check agent: %w", err)
	}
	return ok, nil
}

const seatOrder = `
	SELECT dm.user_id FROM department_members dm
	JOIN users u ON u.id = dm.user_id AND u.status <> 'disabled'
	WHERE dm.tenant_id = $1
	GROUP BY dm.user_id
	ORDER BY min(dm.created_at), dm.user_id`

func (pgRepository) SeatHolders(ctx context.Context, tenantID uuid.UUID, max int) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, seatOrder+` LIMIT $2`, tenantID, max)
	if err != nil {
		return nil, fmt.Errorf("seat holders: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan seat holder: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (pgRepository) AgentsBeyond(ctx context.Context, tenantID uuid.UUID, max int) ([]Person, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT o.user_id, u.full_name FROM (`+seatOrder+` OFFSET $2) o JOIN users u ON u.id = o.user_id ORDER BY u.full_name`, tenantID, max)
	if err != nil {
		return nil, fmt.Errorf("agents beyond seat limit: %w", err)
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.FullName); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
