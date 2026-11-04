package callback

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
)

// CallInfo is what creating a callback for a call needs to know.
type CallInfo struct {
	TenantID     uuid.UUID
	DepartmentID uuid.UUID
}

// Admin is a tenant administrator to notify.
type Admin struct {
	Email string
	Name  string
}

type Repository interface {
	Insert(ctx context.Context, tenantID, departmentID uuid.UUID, callID *uuid.UUID, in CreateInput) (uuid.UUID, error)
	CallInfo(ctx context.Context, callID uuid.UUID) (*CallInfo, error)
	// MemberIDs lists the agents of a department; they get a live hint when callbacks change.
	MemberIDs(ctx context.Context, departmentID uuid.UUID) ([]uuid.UUID, error)
	// Admins lists a tenant's active administrators and the tenant's name.
	Admins(ctx context.Context, tenantID uuid.UUID) (tenantName string, admins []Admin, err error)

	List(ctx context.Context, scope Scope, f Filter, page httpx.Page) ([]Callback, int, error)
	Get(ctx context.Context, id uuid.UUID, scope Scope) (*Callback, error)

	// Each transition is one conditional update and reports whether it changed a row.
	Claim(ctx context.Context, id, userID uuid.UUID) (bool, error)
	Release(ctx context.Context, id, userID uuid.UUID, admin bool) (bool, error)
	Complete(ctx context.Context, id, userID uuid.UUID, admin bool, notes string) (bool, error)
	Cancel(ctx context.Context, id uuid.UUID) (bool, error)
	Assign(ctx context.Context, id, userID uuid.UUID) (bool, error)
	IsMember(ctx context.Context, departmentID, userID uuid.UUID) (bool, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func (pgRepository) Insert(ctx context.Context, tenantID, departmentID uuid.UUID, callID *uuid.UUID, in CreateInput) (uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO callback_requests (tenant_id, department_id, call_id, name, phone, email, reason)
		VALUES ($1, $2, $3, $4, nullif($5, ''), nullif($6, ''), $7) RETURNING id`,
		tenantID, departmentID, callID, in.Name, in.Phone, in.Email, in.Reason).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert callback: %w", err)
	}
	return id, nil
}

func (pgRepository) CallInfo(ctx context.Context, callID uuid.UUID) (*CallInfo, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var c CallInfo
	err = q.QueryRow(ctx, `SELECT tenant_id, department_id FROM calls WHERE id = $1`, callID).Scan(&c.TenantID, &c.DepartmentID)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("callback call info: %w", err)
	}
	return &c, nil
}

func (pgRepository) MemberIDs(ctx context.Context, departmentID uuid.UUID) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT user_id FROM department_members WHERE department_id = $1`, departmentID)
	if err != nil {
		return nil, fmt.Errorf("list department members: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (pgRepository) Admins(ctx context.Context, tenantID uuid.UUID) (string, []Admin, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return "", nil, err
	}
	var tenantName string
	if err := q.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, tenantID).Scan(&tenantName); err != nil {
		return "", nil, fmt.Errorf("tenant name: %w", err)
	}
	rows, err := q.Query(ctx, `
		SELECT email::text, full_name FROM users
		WHERE tenant_id = $1 AND role = 'tenant_admin' AND status = 'active' ORDER BY created_at`, tenantID)
	if err != nil {
		return "", nil, fmt.Errorf("list tenant admins: %w", err)
	}
	defer rows.Close()
	var out []Admin
	for rows.Next() {
		var a Admin
		if err := rows.Scan(&a.Email, &a.Name); err != nil {
			return "", nil, fmt.Errorf("scan admin: %w", err)
		}
		out = append(out, a)
	}
	return tenantName, out, rows.Err()
}

const callbackSelect = `
	SELECT cb.id, cb.department_id, d.name, cb.call_id, cb.name, cb.phone, cb.email::text, cb.reason, cb.status,
	       cb.claimed_by, u.full_name, cb.claimed_at, cb.resolution_notes, cb.resolved_at, cb.created_at
	FROM callback_requests cb
	JOIN departments d ON d.id = cb.department_id
	LEFT JOIN users u ON u.id = cb.claimed_by`

// visible restricts callbacks to the scope ($1 = all, $2 = user id).
const visible = `($1 OR EXISTS (SELECT 1 FROM department_members m WHERE m.department_id = cb.department_id AND m.user_id = $2))`

func scanCallback(row interface{ Scan(...any) error }) (Callback, error) {
	var c Callback
	err := row.Scan(&c.ID, &c.DepartmentID, &c.DepartmentName, &c.CallID, &c.Name, &c.Phone, &c.Email, &c.Reason, &c.Status,
		&c.ClaimedBy, &c.ClaimedByName, &c.ClaimedAt, &c.ResolutionNotes, &c.ResolvedAt, &c.CreatedAt)
	return c, err
}

func (pgRepository) List(ctx context.Context, scope Scope, f Filter, page httpx.Page) ([]Callback, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := ` WHERE ` + visible + ` AND ($3 = '' OR cb.status = $3) AND ($4 = '00000000-0000-0000-0000-000000000000'::uuid OR cb.department_id = $4)`
	args := []any{scope.All, scope.UserID, string(f.Status), f.DepartmentID}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM callback_requests cb`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count callbacks: %w", err)
	}
	// Open and claimed requests first: they are the work to do.
	rows, err := q.Query(ctx, callbackSelect+where+`
		ORDER BY (cb.status IN ('open', 'claimed')) DESC, cb.created_at DESC, cb.id DESC LIMIT $5 OFFSET $6`,
		append(args, page.PerPage, page.Offset())...)
	if err != nil {
		return nil, 0, fmt.Errorf("list callbacks: %w", err)
	}
	defer rows.Close()
	var out []Callback
	for rows.Next() {
		c, err := scanCallback(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan callback: %w", err)
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (pgRepository) Get(ctx context.Context, id uuid.UUID, scope Scope) (*Callback, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	c, err := scanCallback(q.QueryRow(ctx, callbackSelect+` WHERE cb.id = $3 AND `+visible, scope.All, scope.UserID, id))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get callback: %w", err)
	}
	return &c, nil
}

func changed(q database.Querier, ctx context.Context, what, sql string, args ...any) (bool, error) {
	tag, err := q.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	return tag.RowsAffected() == 1, nil
}

func (pgRepository) Claim(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	return changed(q, ctx, "claim callback", `
		UPDATE callback_requests SET status = 'claimed', claimed_by = $2, claimed_at = now()
		WHERE id = $1 AND status = 'open'`, id, userID)
}

func (pgRepository) Release(ctx context.Context, id, userID uuid.UUID, admin bool) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	return changed(q, ctx, "release callback", `
		UPDATE callback_requests SET status = 'open', claimed_by = NULL, claimed_at = NULL
		WHERE id = $1 AND status = 'claimed' AND ($3 OR claimed_by = $2)`, id, userID, admin)
}

func (pgRepository) Complete(ctx context.Context, id, userID uuid.UUID, admin bool, notes string) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	return changed(q, ctx, "complete callback", `
		UPDATE callback_requests SET status = 'completed', resolution_notes = $4, resolved_at = now()
		WHERE id = $1 AND status = 'claimed' AND ($3 OR claimed_by = $2)`, id, userID, admin, notes)
}

func (pgRepository) Cancel(ctx context.Context, id uuid.UUID) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	return changed(q, ctx, "cancel callback", `
		UPDATE callback_requests SET status = 'cancelled', resolved_at = now()
		WHERE id = $1 AND status IN ('open', 'claimed')`, id)
}

func (pgRepository) Assign(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	return changed(q, ctx, "assign callback", `
		UPDATE callback_requests SET status = 'claimed', claimed_by = $2, claimed_at = now()
		WHERE id = $1 AND status IN ('open', 'claimed')`, id, userID)
}

func (pgRepository) IsMember(ctx context.Context, departmentID, userID uuid.UUID) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM department_members WHERE department_id = $1 AND user_id = $2)`,
		departmentID, userID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check membership: %w", err)
	}
	return ok, nil
}
