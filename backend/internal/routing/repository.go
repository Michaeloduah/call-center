package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/platform/database"
	"callcenter/internal/presence"
)

// Repository holds every query routing needs. All of them run inside the caller's scoped transaction.
type Repository interface {
	LockTenant(ctx context.Context, tenantID uuid.UUID) error
	Timings(ctx context.Context, tenantID uuid.UUID) (Timings, error)

	// Sweeping and dispatch (one pass per tenant, under the tenant lock).
	MarkStaleOffline(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)
	CancelUnavailableOffers(ctx context.Context, tenantID uuid.UUID) ([]Affected, error)
	ExpireOffers(ctx context.Context, tenantID uuid.UUID) ([]Affected, error)
	RequeueOrphans(ctx context.Context, tenantID uuid.UUID) ([]Affected, error)
	MissOverdue(ctx context.Context, tenantID uuid.UUID, maxWait time.Duration) ([]call.EndedCall, error)
	QueuedCalls(ctx context.Context, tenantID uuid.UUID) ([]QueuedCall, error)
	PickAgent(ctx context.Context, tenantID, callID, departmentID uuid.UUID) (uuid.UUID, bool, error)
	CreateOffer(ctx context.Context, tenantID, callID, agentID uuid.UUID, timeout time.Duration) error
	TenantsWithWork(ctx context.Context) ([]uuid.UUID, error)

	// Call ended hook.
	CancelCallOffers(ctx context.Context, callID uuid.UUID) ([]uuid.UUID, error)
	FreeAgent(ctx context.Context, agentID uuid.UUID, goOffline bool) error

	// Agent actions.
	Snapshot(ctx context.Context, agentID uuid.UUID) (*Snapshot, error)
	ActiveCall(ctx context.Context, agentID uuid.UUID) (*ActiveCallView, error)
	AcceptOffer(ctx context.Context, offerID, agentID uuid.UUID) (*Affected, error)
	DeclineOffer(ctx context.Context, offerID, agentID uuid.UUID) (*Affected, error)
	AssignedRoom(ctx context.Context, callID, agentID uuid.UUID) (string, error)
}

// Snapshot is the raw state behind a Session.
type Snapshot struct {
	Status        presence.Status
	Fresh         bool
	Offer         *OfferView
	Call          *ActiveCallView
	OpenCallbacks int
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func (pgRepository) LockTenant(ctx context.Context, tenantID uuid.UUID) error {
	return database.LockTenant(ctx, tenantID)
}

func (pgRepository) Timings(ctx context.Context, tenantID uuid.UUID) (Timings, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return Timings{}, err
	}
	var offer, wait int
	if err := q.QueryRow(ctx, `SELECT offer_timeout_seconds, max_queue_wait_seconds FROM tenant_settings WHERE tenant_id = $1`,
		tenantID).Scan(&offer, &wait); err != nil {
		return Timings{}, fmt.Errorf("routing timings: %w", err)
	}
	return Timings{OfferTimeout: time.Duration(offer) * time.Second, MaxQueueWait: time.Duration(wait) * time.Second}, nil
}

func staleSeconds() float64 { return presence.StaleAfter.Seconds() }

func (pgRepository) MarkStaleOffline(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		UPDATE agent_presence SET status = 'offline', status_changed_at = now()
		WHERE tenant_id = $1 AND status <> 'offline' AND last_heartbeat_at <= now() - make_interval(secs => $2)
		RETURNING user_id`, tenantID, staleSeconds())
	if err != nil {
		return nil, fmt.Errorf("mark stale agents offline: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stale agent: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanAffected(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]Affected, error) {
	var out []Affected
	for rows.Next() {
		var a Affected
		if err := rows.Scan(&a.CallID, &a.AgentID, &a.DepartmentID); err != nil {
			return nil, fmt.Errorf("scan affected: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// An offer is cancelled (not counted as a timeout) when its agent is no longer available:
// they went away or offline, or stopped sending heartbeats. The call returns to the queue.
func (pgRepository) CancelUnavailableOffers(ctx context.Context, tenantID uuid.UUID) ([]Affected, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		WITH gone AS (
		    UPDATE call_offers o SET outcome = 'cancelled', responded_at = now()
		    FROM calls c
		    WHERE o.tenant_id = $1 AND o.outcome = 'pending' AND o.agent_id IS NOT NULL AND c.id = o.call_id
		      AND NOT EXISTS (SELECT 1 FROM agent_presence p
		                      WHERE p.user_id = o.agent_id AND p.status = 'available'
		                        AND p.last_heartbeat_at > now() - make_interval(secs => $2))
		    RETURNING o.call_id, o.agent_id, c.department_id
		), requeue AS (
		    UPDATE calls SET status = 'queued' WHERE id IN (SELECT call_id FROM gone) AND status = 'ringing'
		)
		SELECT call_id, agent_id, department_id FROM gone`, tenantID, staleSeconds())
	if err != nil {
		return nil, fmt.Errorf("cancel unavailable offers: %w", err)
	}
	defer rows.Close()
	return scanAffected(rows)
}

func (pgRepository) ExpireOffers(ctx context.Context, tenantID uuid.UUID) ([]Affected, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		WITH expired AS (
		    UPDATE call_offers o SET outcome = 'timeout', responded_at = now()
		    FROM calls c
		    WHERE o.tenant_id = $1 AND o.outcome = 'pending' AND o.expires_at <= now() AND c.id = o.call_id
		    RETURNING o.call_id, o.agent_id, c.department_id
		), requeue AS (
		    UPDATE calls SET status = 'queued' WHERE id IN (SELECT call_id FROM expired) AND status = 'ringing'
		), penalise AS (
		    UPDATE agent_presence p SET
		        idle_since = now(),
		        consecutive_timeouts = p.consecutive_timeouts + 1,
		        status = CASE WHEN p.consecutive_timeouts + 1 >= $2 THEN 'away' ELSE p.status END,
		        status_changed_at = CASE WHEN p.consecutive_timeouts + 1 >= $2 AND p.status <> 'away' THEN now() ELSE p.status_changed_at END
		    WHERE p.user_id IN (SELECT agent_id FROM expired)
		)
		SELECT call_id, agent_id, department_id FROM expired`, tenantID, TimeoutsBeforeAway)
	if err != nil {
		return nil, fmt.Errorf("expire offers: %w", err)
	}
	defer rows.Close()
	return scanAffected(rows)
}

// RequeueOrphans returns ringing calls that no longer have a live offer (for example because the
// offered agent's account was deleted) to the queue. It is a safety net, not a normal path.
func (pgRepository) RequeueOrphans(ctx context.Context, tenantID uuid.UUID) ([]Affected, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		UPDATE calls c SET status = 'queued'
		WHERE c.tenant_id = $1 AND c.status = 'ringing'
		  AND NOT EXISTS (SELECT 1 FROM call_offers o WHERE o.call_id = c.id AND o.outcome = 'pending')
		RETURNING c.id, NULL::uuid, c.department_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("requeue orphaned calls: %w", err)
	}
	defer rows.Close()
	return scanAffected(rows)
}

func (pgRepository) MissOverdue(ctx context.Context, tenantID uuid.UUID, maxWait time.Duration) ([]call.EndedCall, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		WITH missed AS (
		    UPDATE calls SET status = 'missed', ended_at = now(), end_reason = $3
		    WHERE tenant_id = $1 AND status IN ('queued', 'ringing') AND queued_at <= now() - make_interval(secs => $2)
		    RETURNING id, tenant_id, department_id, room_name
		), cancelled AS (
		    UPDATE call_offers SET outcome = 'cancelled', responded_at = now()
		    WHERE call_id IN (SELECT id FROM missed) AND outcome = 'pending'
		    RETURNING call_id, agent_id
		)
		SELECT m.id, m.tenant_id, m.department_id, m.room_name, c.agent_id
		FROM missed m LEFT JOIN cancelled c ON c.call_id = m.id`, tenantID, maxWait.Seconds(), call.EndMaxWait)
	if err != nil {
		return nil, fmt.Errorf("miss overdue calls: %w", err)
	}
	defer rows.Close()
	var out []call.EndedCall
	for rows.Next() {
		e := call.EndedCall{Reason: call.EndMaxWait}
		if err := rows.Scan(&e.ID, &e.TenantID, &e.DepartmentID, &e.RoomName, &e.AgentID); err != nil {
			return nil, fmt.Errorf("scan missed call: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (pgRepository) QueuedCalls(ctx context.Context, tenantID uuid.UUID) ([]QueuedCall, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT id, department_id FROM calls WHERE tenant_id = $1 AND status = 'queued' ORDER BY queued_at, id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list queued calls: %w", err)
	}
	defer rows.Close()
	var out []QueuedCall
	for rows.Next() {
		var c QueuedCall
		if err := rows.Scan(&c.ID, &c.DepartmentID); err != nil {
			return nil, fmt.Errorf("scan queued call: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PickAgent chooses the longest-idle agent who may take the call (see docs/M3-DESIGN.md 4.1).
func (pgRepository) PickAgent(ctx context.Context, tenantID, callID, departmentID uuid.UUID) (uuid.UUID, bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		SELECT p.user_id
		FROM agent_presence p
		JOIN users u ON u.id = p.user_id AND u.status = 'active'
		JOIN department_members m ON m.user_id = p.user_id AND m.department_id = $3
		WHERE p.tenant_id = $1 AND p.status = 'available'
		  AND p.last_heartbeat_at > now() - make_interval(secs => $4)
		  AND NOT EXISTS (SELECT 1 FROM call_offers o WHERE o.agent_id = p.user_id AND o.outcome = 'pending')
		  AND NOT EXISTS (SELECT 1 FROM calls c WHERE c.assigned_agent_id = p.user_id AND c.status = 'active')
		  AND NOT EXISTS (SELECT 1 FROM call_offers o WHERE o.call_id = $2 AND o.agent_id = p.user_id)
		ORDER BY p.idle_since, p.user_id
		LIMIT 1`, tenantID, callID, departmentID, staleSeconds()).Scan(&id)
	if database.IsNoRows(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("pick agent: %w", err)
	}
	return id, true, nil
}

func (pgRepository) CreateOffer(ctx context.Context, tenantID, callID, agentID uuid.UUID, timeout time.Duration) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO call_offers (tenant_id, call_id, agent_id, expires_at)
		VALUES ($1, $2, $3, now() + make_interval(secs => $4))`, tenantID, callID, agentID, timeout.Seconds()); err != nil {
		return fmt.Errorf("create offer: %w", err)
	}
	if _, err := q.Exec(ctx, `UPDATE calls SET status = 'ringing' WHERE id = $1 AND status = 'queued'`, callID); err != nil {
		return fmt.Errorf("mark call ringing: %w", err)
	}
	return nil
}

func (pgRepository) TenantsWithWork(ctx context.Context) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT tenant_id FROM calls WHERE status IN ('queued', 'ringing')
		UNION
		SELECT tenant_id FROM agent_presence WHERE status <> 'offline' AND last_heartbeat_at <= now() - make_interval(secs => $1)`,
		staleSeconds())
	if err != nil {
		return nil, fmt.Errorf("find tenants with routing work: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (pgRepository) CancelCallOffers(ctx context.Context, callID uuid.UUID) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		UPDATE call_offers SET outcome = 'cancelled', responded_at = now()
		WHERE call_id = $1 AND outcome = 'pending' AND agent_id IS NOT NULL
		RETURNING agent_id`, callID)
	if err != nil {
		return nil, fmt.Errorf("cancel call offers: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan cancelled offer: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (pgRepository) FreeAgent(ctx context.Context, agentID uuid.UUID, goOffline bool) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		UPDATE agent_presence SET idle_since = now(),
		    status = CASE WHEN $2 THEN 'offline' ELSE status END,
		    status_changed_at = CASE WHEN $2 AND status <> 'offline' THEN now() ELSE status_changed_at END
		WHERE user_id = $1`, agentID, goOffline); err != nil {
		return fmt.Errorf("free agent: %w", err)
	}
	return nil
}

func (r pgRepository) Snapshot(ctx context.Context, agentID uuid.UUID) (*Snapshot, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	s := Snapshot{Status: presence.StatusOffline}
	err = q.QueryRow(ctx, `
		SELECT status, last_heartbeat_at > now() - make_interval(secs => $2) FROM agent_presence WHERE user_id = $1`,
		agentID, staleSeconds()).Scan(&s.Status, &s.Fresh)
	if err != nil && !database.IsNoRows(err) {
		return nil, fmt.Errorf("snapshot presence: %w", err)
	}

	var o OfferView
	err = q.QueryRow(ctx, `
		SELECT o.id, o.call_id, d.name, c.caller_name, o.expires_at
		FROM call_offers o
		JOIN calls c ON c.id = o.call_id
		JOIN departments d ON d.id = c.department_id
		WHERE o.agent_id = $1 AND o.outcome = 'pending' AND o.expires_at > now()`, agentID).
		Scan(&o.ID, &o.CallID, &o.DepartmentName, &o.CallerName, &o.ExpiresAt)
	switch {
	case err == nil:
		s.Offer = &o
	case !database.IsNoRows(err):
		return nil, fmt.Errorf("snapshot offer: %w", err)
	}

	if s.Call, err = r.ActiveCall(ctx, agentID); err != nil {
		return nil, err
	}
	if err := q.QueryRow(ctx, `
		SELECT count(*)::int FROM callback_requests cb
		WHERE cb.status = 'open'
		  AND EXISTS (SELECT 1 FROM department_members m WHERE m.department_id = cb.department_id AND m.user_id = $1)`,
		agentID).Scan(&s.OpenCallbacks); err != nil {
		return nil, fmt.Errorf("snapshot callbacks: %w", err)
	}
	return &s, nil
}

func (pgRepository) ActiveCall(ctx context.Context, agentID uuid.UUID) (*ActiveCallView, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var c ActiveCallView
	err = q.QueryRow(ctx, `
		SELECT c.id, d.name, c.caller_name, c.caller_phone, c.caller_email::text, c.answered_at
		FROM calls c JOIN departments d ON d.id = c.department_id
		WHERE c.assigned_agent_id = $1 AND c.status = 'active'`, agentID).
		Scan(&c.ID, &c.DepartmentName, &c.CallerName, &c.CallerPhone, &c.CallerEmail, &c.AnsweredAt)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("active call: %w", err)
	}
	return &c, nil
}

// AcceptOffer turns a live offer into an active call. Both updates are conditional, so when two
// things race (the agent, the timeout sweeper, the caller hanging up) exactly one wins.
func (pgRepository) AcceptOffer(ctx context.Context, offerID, agentID uuid.UUID) (*Affected, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	a := Affected{AgentID: &agentID}
	err = q.QueryRow(ctx, `
		UPDATE call_offers SET outcome = 'accepted', responded_at = now()
		WHERE id = $1 AND agent_id = $2 AND outcome = 'pending' AND expires_at > now()
		RETURNING call_id`, offerID, agentID).Scan(&a.CallID)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("accept offer: %w", err)
	}
	err = q.QueryRow(ctx, `
		UPDATE calls SET status = 'active', assigned_agent_id = $2, answered_at = now()
		WHERE id = $1 AND status = 'ringing'
		RETURNING department_id`, a.CallID, agentID).Scan(&a.DepartmentID)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("activate call: %w", err)
	}
	if _, err := q.Exec(ctx, `UPDATE agent_presence SET consecutive_timeouts = 0 WHERE user_id = $1`, agentID); err != nil {
		return nil, fmt.Errorf("reset timeouts: %w", err)
	}
	return &a, nil
}

func (pgRepository) DeclineOffer(ctx context.Context, offerID, agentID uuid.UUID) (*Affected, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	a := Affected{AgentID: &agentID}
	err = q.QueryRow(ctx, `
		UPDATE call_offers SET outcome = 'declined', responded_at = now()
		WHERE id = $1 AND agent_id = $2 AND outcome = 'pending'
		RETURNING call_id`, offerID, agentID).Scan(&a.CallID)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decline offer: %w", err)
	}
	if err := q.QueryRow(ctx, `UPDATE calls SET status = 'queued' WHERE id = $1 AND status = 'ringing' RETURNING department_id`, a.CallID).
		Scan(&a.DepartmentID); err != nil && !database.IsNoRows(err) {
		return nil, fmt.Errorf("requeue declined call: %w", err)
	}
	if _, err := q.Exec(ctx, `UPDATE agent_presence SET idle_since = now() WHERE user_id = $1`, agentID); err != nil {
		return nil, fmt.Errorf("reset idle time: %w", err)
	}
	return &a, nil
}

func (pgRepository) AssignedRoom(ctx context.Context, callID, agentID uuid.UUID) (string, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return "", err
	}
	var room string
	err = q.QueryRow(ctx, `SELECT room_name FROM calls WHERE id = $1 AND assigned_agent_id = $2 AND status = 'active'`, callID, agentID).Scan(&room)
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("assigned room: %w", err)
	}
	return room, nil
}
