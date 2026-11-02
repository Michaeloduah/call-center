package call

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
)

// Repository persists calls. Public caller flows run in platform scope because the caller is
// anonymous; every query is still constrained by call id + token hash or by room name.
type Repository interface {
	LockTenant(ctx context.Context, tenantID uuid.UUID) error
	CountLive(ctx context.Context, tenantID uuid.UUID) (int, error)
	Insert(ctx context.Context, c NewCall) error
	FindForCaller(ctx context.Context, id uuid.UUID, tokenHash []byte) (*CallerView, error)
	QueuePosition(ctx context.Context, c *CallerView) (int, error)
	// End closes a live call: active becomes completed, queued/ringing become abandoned.
	// It returns the room name, or "" if the call was not live (already ended).
	End(ctx context.Context, id uuid.UUID, reason string) (string, error)
	EndByRoom(ctx context.Context, room, reason string) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

// LockTenant serialises call creation per tenant so the live-call cap cannot be raced past.
func (pgRepository) LockTenant(ctx context.Context, tenantID uuid.UUID) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `SELECT 1 FROM tenants WHERE id = $1 FOR UPDATE`, tenantID); err != nil {
		return fmt.Errorf("lock tenant: %w", err)
	}
	return nil
}

func (pgRepository) CountLive(ctx context.Context, tenantID uuid.UUID) (int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	if err := q.QueryRow(ctx, `
		SELECT count(*) FROM calls WHERE tenant_id = $1 AND status IN ('queued', 'ringing', 'active')`, tenantID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count live calls: %w", err)
	}
	return n, nil
}

func (pgRepository) Insert(ctx context.Context, c NewCall) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO calls (id, tenant_id, widget_id, department_id, room_name, status, caller_name, caller_phone,
		                   caller_email, consent_text, consent_given_at, caller_token_hash, caller_token_expires_at,
		                   caller_ip, caller_user_agent)
		VALUES ($1, $2, $3, $4, $5, 'queued', nullif($6, ''), nullif($7, ''), nullif($8, ''), $9, now(), $10, $11,
		        nullif($12, '')::inet, nullif($13, ''))`,
		c.ID, c.TenantID, c.WidgetID, c.DepartmentID, c.RoomName, c.CallerName, c.CallerPhone, c.CallerEmail,
		c.ConsentText, c.CallerTokenHash, c.CallerTokenExpiresAt, c.CallerIP, c.CallerUserAgent)
	if err != nil {
		return fmt.Errorf("insert call: %w", err)
	}
	return nil
}

// FindForCaller returns nil, nil when the id/token pair does not match or the token expired.
func (pgRepository) FindForCaller(ctx context.Context, id uuid.UUID, tokenHash []byte) (*CallerView, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var v CallerView
	err = q.QueryRow(ctx, `
		SELECT id, status, end_reason, department_id, room_name, queued_at FROM calls
		WHERE id = $1 AND caller_token_hash = $2 AND caller_token_expires_at > now()`, id, tokenHash).
		Scan(&v.ID, &v.Status, &v.EndReason, &v.DepartmentID, &v.RoomName, &v.QueuedAt)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find call for caller: %w", err)
	}
	return &v, nil
}

// QueuePosition is 1 for the caller at the head of their department's queue.
func (pgRepository) QueuePosition(ctx context.Context, c *CallerView) (int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	if err := q.QueryRow(ctx, `
		SELECT count(*) FROM calls
		WHERE department_id = $1 AND status = 'queued' AND (queued_at, id) <= ($2, $3)`,
		c.DepartmentID, c.QueuedAt, c.ID).Scan(&n); err != nil {
		return 0, fmt.Errorf("queue position: %w", err)
	}
	return n, nil
}

const endLiveCall = `
	UPDATE calls
	SET status = CASE WHEN status = 'active' THEN 'completed' ELSE 'abandoned' END,
	    ended_at = now(), end_reason = $2
	WHERE %s AND status IN ('queued', 'ringing', 'active')
	RETURNING room_name`

func (pgRepository) End(ctx context.Context, id uuid.UUID, reason string) (string, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return "", err
	}
	var room string
	err = q.QueryRow(ctx, fmt.Sprintf(endLiveCall, "id = $1"), id, reason).Scan(&room)
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("end call: %w", err)
	}
	return room, nil
}

func (pgRepository) EndByRoom(ctx context.Context, room, reason string) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, fmt.Sprintf(endLiveCall, "room_name = $1"), room, reason); err != nil {
		return fmt.Errorf("end call by room: %w", err)
	}
	return nil
}

func (pgRepository) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		UPDATE calls SET status = 'failed', ended_at = now(), end_reason = $2
		WHERE id = $1 AND status IN ('queued', 'ringing', 'active')`, id, reason); err != nil {
		return fmt.Errorf("mark call failed: %w", err)
	}
	return nil
}
