package recording

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/media"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
)

// Scope limits which recordings a tenant user may reach: tenant admins reach all of their
// tenant's, supervisors only those of calls in departments they supervise.
type Scope struct {
	All    bool
	UserID uuid.UUID
}

type Repository interface {
	// Insert creates the row for a call's recording. It returns false when one already exists.
	Insert(ctx context.Context, tenantID, callID uuid.UUID) (uuid.UUID, bool, error)
	Attach(ctx context.Context, id uuid.UUID, egressID, key string) error
	Fail(ctx context.Context, id uuid.UUID, reason string) error
	// Finish applies the recorder's final report. found is false when no recording has that egress id.
	Finish(ctx context.Context, egressID string, res media.RecordingResult) (tenantID uuid.UUID, found bool, err error)
	// FailStale marks recordings that never completed. It returns how many it touched.
	FailStale(ctx context.Context) (int, error)

	// FindVisible returns the call's recording if it is not deleted and within scope.
	FindVisible(ctx context.Context, callID uuid.UUID, scope Scope) (*Located, error)
	// FindByID returns a recording whether or not it is deleted (platform admin).
	FindByID(ctx context.Context, id uuid.UUID) (*Located, error)
	SoftDelete(ctx context.Context, callID, userID uuid.UUID) (*Located, error)
	Restore(ctx context.Context, id uuid.UUID) (*Located, error)
	ListDeleted(ctx context.Context, page httpx.Page) ([]DeletedItem, int, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func (pgRepository) Insert(ctx context.Context, tenantID, callID uuid.UUID) (uuid.UUID, bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO recordings (tenant_id, call_id, status) VALUES ($1, $2, 'starting')
		ON CONFLICT (call_id) DO NOTHING RETURNING id`, tenantID, callID).Scan(&id)
	if database.IsNoRows(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("insert recording: %w", err)
	}
	return id, true, nil
}

func (pgRepository) Attach(ctx context.Context, id uuid.UUID, egressID, key string) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		UPDATE recordings SET status = 'recording', egress_id = $2, storage_key = $3, content_type = $4
		WHERE id = $1 AND status = 'starting'`, id, egressID, key, contentType); err != nil {
		return fmt.Errorf("attach recorder: %w", err)
	}
	return nil
}

func (pgRepository) Fail(ctx context.Context, id uuid.UUID, reason string) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		UPDATE recordings SET status = 'failed', error = left($2, 500), ended_at = now()
		WHERE id = $1 AND status IN ('starting', 'recording')`, id, reason); err != nil {
		return fmt.Errorf("fail recording: %w", err)
	}
	return nil
}

func (pgRepository) Finish(ctx context.Context, egressID string, res media.RecordingResult) (uuid.UUID, bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var tenantID uuid.UUID
	err = q.QueryRow(ctx, `SELECT tenant_id FROM recordings WHERE egress_id = $1`, egressID).Scan(&tenantID)
	if database.IsNoRows(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("find recording by egress: %w", err)
	}
	// Both branches only touch recordings that are still unfinished, so a redelivered event is a no-op.
	if res.Succeeded {
		_, err = q.Exec(ctx, `
			UPDATE recordings SET status = 'ready', size_bytes = $2, duration_seconds = $3, ended_at = now()
			WHERE egress_id = $1 AND status IN ('starting', 'recording')`,
			egressID, res.SizeBytes, int(res.Duration.Round(time.Second).Seconds()))
	} else {
		_, err = q.Exec(ctx, `
			UPDATE recordings SET status = 'failed', error = left($2, 500), ended_at = now()
			WHERE egress_id = $1 AND status IN ('starting', 'recording')`, egressID, res.Error)
	}
	if err != nil {
		return tenantID, true, fmt.Errorf("finish recording: %w", err)
	}
	return tenantID, true, nil
}

func (pgRepository) FailStale(ctx context.Context) (int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return 0, err
	}
	tag, err := q.Exec(ctx, `
		UPDATE recordings r SET status = 'failed', ended_at = now(),
		    error = 'The recorder did not report a result.'
		WHERE (r.status = 'starting' AND r.created_at < now() - make_interval(secs => $1))
		   OR (r.status = 'recording' AND EXISTS (
		        SELECT 1 FROM calls c WHERE c.id = r.call_id AND c.ended_at IS NOT NULL
		          AND c.ended_at < now() - make_interval(secs => $2)))`,
		startingStuckAfter.Seconds(), finalizeGrace.Seconds())
	if err != nil {
		return 0, fmt.Errorf("fail stale recordings: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

const locatedSelect = `SELECT r.id, r.tenant_id, r.call_id, r.status, r.storage_key FROM recordings r JOIN calls c ON c.id = r.call_id`

func scanLocated(row interface{ Scan(...any) error }) (*Located, error) {
	var l Located
	err := row.Scan(&l.ID, &l.TenantID, &l.CallID, &l.Status, &l.Key)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan recording: %w", err)
	}
	return &l, nil
}

func (pgRepository) FindVisible(ctx context.Context, callID uuid.UUID, scope Scope) (*Located, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanLocated(q.QueryRow(ctx, locatedSelect+`
		WHERE r.call_id = $1 AND r.deleted_at IS NULL
		  AND ($2 OR EXISTS (SELECT 1 FROM department_supervisors s WHERE s.department_id = c.department_id AND s.user_id = $3))`,
		callID, scope.All, scope.UserID))
}

func (pgRepository) FindByID(ctx context.Context, id uuid.UUID) (*Located, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanLocated(q.QueryRow(ctx, locatedSelect+` WHERE r.id = $1`, id))
}

func (pgRepository) SoftDelete(ctx context.Context, callID, userID uuid.UUID) (*Located, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanLocated(q.QueryRow(ctx, `
		WITH d AS (
		    UPDATE recordings SET deleted_at = now(), deleted_by = $2
		    WHERE call_id = $1 AND deleted_at IS NULL
		    RETURNING id, tenant_id, call_id, status, storage_key
		)
		SELECT id, tenant_id, call_id, status, storage_key FROM d`, callID, userID))
}

func (pgRepository) Restore(ctx context.Context, id uuid.UUID) (*Located, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanLocated(q.QueryRow(ctx, `
		WITH d AS (
		    UPDATE recordings SET deleted_at = NULL, deleted_by = NULL
		    WHERE id = $1 AND deleted_at IS NOT NULL
		    RETURNING id, tenant_id, call_id, status, storage_key
		)
		SELECT id, tenant_id, call_id, status, storage_key FROM d`, id))
}

func (pgRepository) ListDeleted(ctx context.Context, page httpx.Page) ([]DeletedItem, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM recordings WHERE deleted_at IS NOT NULL`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count deleted recordings: %w", err)
	}
	rows, err := q.Query(ctx, `
		SELECT r.id, r.call_id, r.tenant_id, t.name, d.name, c.caller_name, r.duration_seconds, r.size_bytes,
		       r.deleted_at, u.email::text
		FROM recordings r
		JOIN tenants t ON t.id = r.tenant_id
		JOIN calls c ON c.id = r.call_id
		JOIN departments d ON d.id = c.department_id
		LEFT JOIN users u ON u.id = r.deleted_by
		WHERE r.deleted_at IS NOT NULL
		ORDER BY r.deleted_at DESC, r.id DESC LIMIT $1 OFFSET $2`, page.PerPage, page.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("list deleted recordings: %w", err)
	}
	defer rows.Close()
	var items []DeletedItem
	for rows.Next() {
		var it DeletedItem
		if err := rows.Scan(&it.ID, &it.CallID, &it.TenantID, &it.TenantName, &it.DepartmentName, &it.CallerName,
			&it.DurationSeconds, &it.SizeBytes, &it.DeletedAt, &it.DeletedByEmail); err != nil {
			return nil, 0, fmt.Errorf("scan deleted recording: %w", err)
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}
