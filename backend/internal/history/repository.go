package history

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/recording"
)

type Repository interface {
	List(ctx context.Context, scope Scope, f Filter, page httpx.Page) ([]Item, int, error)
	// Get returns nil when the call does not exist or is outside the scope.
	Get(ctx context.Context, id uuid.UUID, scope Scope) (*Detail, error)
	Offers(ctx context.Context, callID uuid.UUID) ([]OfferEntry, error)
	Recording(ctx context.Context, callID uuid.UUID) (*recording.View, error)
	Callback(ctx context.Context, callID uuid.UUID) (*CallbackRef, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

// scope restricts calls to what the user may see ($1 = sees all, $2 = user id).
const scope = `($1 OR c.assigned_agent_id = $2 OR EXISTS (
	SELECT 1 FROM department_supervisors s WHERE s.department_id = c.department_id AND s.user_id = $2))`

const itemSelect = `
	SELECT c.id, c.status, c.end_reason, c.department_id, d.name, c.caller_name, c.caller_phone, au.full_name,
	       c.queued_at, c.answered_at, c.ended_at,
	       CASE WHEN c.answered_at IS NOT NULL AND c.ended_at IS NOT NULL
	            THEN extract(epoch FROM c.ended_at - c.answered_at)::int END,
	       r.status, (SELECT count(*) FROM call_messages m WHERE m.call_id = c.id)::int`

const itemFrom = `
	FROM calls c
	JOIN departments d ON d.id = c.department_id
	LEFT JOIN users au ON au.id = c.assigned_agent_id
	LEFT JOIN recordings r ON r.call_id = c.id AND r.deleted_at IS NULL`

func scanItem(row interface{ Scan(...any) error }, extra ...any) (Item, error) {
	var it Item
	dest := append([]any{&it.ID, &it.Status, &it.EndReason, &it.DepartmentID, &it.DepartmentName, &it.CallerName,
		&it.CallerPhone, &it.AgentName, &it.QueuedAt, &it.AnsweredAt, &it.EndedAt, &it.DurationSeconds,
		&it.RecordingStatus, &it.MessageCount}, extra...)
	return it, row.Scan(dest...)
}

func (pgRepository) List(ctx context.Context, sc Scope, f Filter, page httpx.Page) ([]Item, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	const nilUUID = `'00000000-0000-0000-0000-000000000000'::uuid`
	where := ` WHERE ` + scope + `
		AND ($3 = '' OR c.status = $3)
		AND ($4 = ` + nilUUID + ` OR c.department_id = $4)
		AND ($5 = ` + nilUUID + ` OR c.assigned_agent_id = $5)
		AND ($6::timestamptz IS NULL OR c.queued_at >= $6)
		AND ($7::timestamptz IS NULL OR c.queued_at < $7)
		AND ($8 = '' OR c.caller_name ILIKE $8 OR c.caller_phone ILIKE $8 OR c.caller_email::text ILIKE $8)`
	args := []any{sc.All, sc.UserID, f.Status, f.DepartmentID, f.AgentID, f.From, f.To, database.ContainsPattern(f.Query)}

	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM calls c`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count calls: %w", err)
	}
	rows, err := q.Query(ctx, itemSelect+itemFrom+where+` ORDER BY c.queued_at DESC, c.id DESC LIMIT $9 OFFSET $10`,
		append(args, page.PerPage, page.Offset())...)
	if err != nil {
		return nil, 0, fmt.Errorf("list calls: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan call: %w", err)
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

func (pgRepository) Get(ctx context.Context, id uuid.UUID, sc Scope) (*Detail, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var d Detail
	var supervises bool
	item, err := scanItem(q.QueryRow(ctx, itemSelect+`,
		       c.caller_email::text, c.consent_text, c.consent_given_at,
		       EXISTS (SELECT 1 FROM department_supervisors s WHERE s.department_id = c.department_id AND s.user_id = $2)`+
		itemFrom+` WHERE c.id = $3 AND `+scope, sc.All, sc.UserID, id),
		&d.CallerEmail, &d.ConsentText, &d.ConsentGivenAt, &supervises)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get call: %w", err)
	}
	d.Item = item
	d.Permissions = Permissions{PlayRecording: sc.All || supervises, DeleteRecording: sc.All}
	return &d, nil
}

func (pgRepository) Offers(ctx context.Context, callID uuid.UUID) ([]OfferEntry, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT u.full_name, o.offered_at, o.responded_at, o.outcome
		FROM call_offers o LEFT JOIN users u ON u.id = o.agent_id
		WHERE o.call_id = $1 ORDER BY o.offered_at, o.id`, callID)
	if err != nil {
		return nil, fmt.Errorf("list call offers: %w", err)
	}
	defer rows.Close()
	out := []OfferEntry{}
	for rows.Next() {
		var o OfferEntry
		if err := rows.Scan(&o.AgentName, &o.OfferedAt, &o.RespondedAt, &o.Outcome); err != nil {
			return nil, fmt.Errorf("scan call offer: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (pgRepository) Recording(ctx context.Context, callID uuid.UUID) (*recording.View, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var v recording.View
	err = q.QueryRow(ctx, `
		SELECT id, status, duration_seconds, size_bytes, started_at, ended_at, error
		FROM recordings WHERE call_id = $1 AND deleted_at IS NULL`, callID).
		Scan(&v.ID, &v.Status, &v.DurationSeconds, &v.SizeBytes, &v.StartedAt, &v.EndedAt, &v.Error)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get call recording: %w", err)
	}
	return &v, nil
}

func (pgRepository) Callback(ctx context.Context, callID uuid.UUID) (*CallbackRef, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var c CallbackRef
	err = q.QueryRow(ctx, `SELECT id, status FROM callback_requests WHERE call_id = $1`, callID).Scan(&c.ID, &c.Status)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get call callback: %w", err)
	}
	return &c, nil
}
