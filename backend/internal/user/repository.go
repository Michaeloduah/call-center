package user

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
)

// Repository persists users. Methods run inside the caller's scoped transaction and
// filter by tenant explicitly, in addition to row-level security.
type Repository interface {
	Insert(ctx context.Context, u NewUser) (User, error)
	List(ctx context.Context, tenantID uuid.UUID, f Filter, page httpx.Page) ([]User, int, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*User, error)
	Departments(ctx context.Context, tenantID, id uuid.UUID) (member, supervised []DepartmentRef, err error)
	UpdateProfile(ctx context.Context, tenantID, id uuid.UUID, fullName string, role reqctx.Role) error
	Disable(ctx context.Context, tenantID, id uuid.UUID) error
	Enable(ctx context.Context, tenantID, id uuid.UUID) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	LockActiveAdmins(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)
	RemoveSupervisorAssignments(ctx context.Context, tenantID, id uuid.UUID) error
	Accounts(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]Account, error)
	TenantName(ctx context.Context, tenantID uuid.UUID) (string, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func nullableTenant(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

const userColumns = `id, email::text, full_name, role, status, email_verified_at, last_login_at, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.Status, &u.EmailVerifiedAt, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (pgRepository) Insert(ctx context.Context, n NewUser) (User, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return User{}, err
	}
	u, err := scanUser(q.QueryRow(ctx, `
		INSERT INTO users (tenant_id, email, full_name, role, status, password_hash, email_verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $7 THEN now() END)
		RETURNING `+userColumns,
		nullableTenant(n.TenantID), n.Email, n.FullName, n.Role, n.Status, n.PasswordHash, n.EmailVerified))
	if database.IsUniqueViolation(err, "users_email_key") {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (pgRepository) List(ctx context.Context, tenantID uuid.UUID, f Filter, page httpx.Page) ([]User, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	const where = `
		WHERE tenant_id = $1
		  AND ($2 = '' OR full_name ILIKE $2 OR email::text ILIKE $2)
		  AND ($3 = '' OR role = $3)
		  AND ($4 = '' OR status = $4)`
	args := []any{tenantID, database.ContainsPattern(f.Query), string(f.Role), f.Status}

	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+userColumns+` FROM users`+where+`
		ORDER BY created_at DESC, id DESC LIMIT $5 OFFSET $6`, append(args, page.PerPage, page.Offset())...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// Get returns nil, nil when the user does not exist in the tenant.
func (pgRepository) Get(ctx context.Context, tenantID, id uuid.UUID) (*User, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	u, err := scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE tenant_id = $1 AND id = $2`, tenantID, id))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

func (pgRepository) Departments(ctx context.Context, tenantID, id uuid.UUID) ([]DepartmentRef, []DepartmentRef, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT d.id, d.name, 'member' FROM department_members m JOIN departments d ON d.id = m.department_id
		WHERE m.tenant_id = $1 AND m.user_id = $2
		UNION ALL
		SELECT d.id, d.name, 'supervisor' FROM department_supervisors s JOIN departments d ON d.id = s.department_id
		WHERE s.tenant_id = $1 AND s.user_id = $2
		ORDER BY 2`, tenantID, id)
	if err != nil {
		return nil, nil, fmt.Errorf("list user departments: %w", err)
	}
	defer rows.Close()
	member, supervised := []DepartmentRef{}, []DepartmentRef{}
	for rows.Next() {
		var ref DepartmentRef
		var kind string
		if err := rows.Scan(&ref.ID, &ref.Name, &kind); err != nil {
			return nil, nil, fmt.Errorf("scan user department: %w", err)
		}
		if kind == "member" {
			member = append(member, ref)
		} else {
			supervised = append(supervised, ref)
		}
	}
	return member, supervised, rows.Err()
}

func (pgRepository) UpdateProfile(ctx context.Context, tenantID, id uuid.UUID, fullName string, role reqctx.Role) error {
	return exec(ctx, "update user", `UPDATE users SET full_name = $3, role = $4 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, fullName, role)
}

func (pgRepository) Disable(ctx context.Context, tenantID, id uuid.UUID) error {
	return exec(ctx, "disable user", `UPDATE users SET status = 'disabled' WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

// Enable restores a disabled user to active, or to invited if they never set a password.
func (pgRepository) Enable(ctx context.Context, tenantID, id uuid.UUID) error {
	return exec(ctx, "enable user", `
		UPDATE users SET status = CASE WHEN password_hash IS NULL THEN 'invited' ELSE 'active' END
		WHERE tenant_id = $1 AND id = $2 AND status = 'disabled'`, tenantID, id)
}

func (pgRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return exec(ctx, "delete user", `DELETE FROM users WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

// LockActiveAdmins row-locks the tenant's active admins and returns their IDs. Holding the lock
// for the rest of the transaction stops two concurrent requests from removing the last admin.
func (pgRepository) LockActiveAdmins(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT id FROM users WHERE tenant_id = $1 AND role = 'tenant_admin' AND status = 'active'
		ORDER BY id FOR UPDATE`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("lock admins: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan admin: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (pgRepository) RemoveSupervisorAssignments(ctx context.Context, tenantID, id uuid.UUID) error {
	return exec(ctx, "remove supervisor assignments",
		`DELETE FROM department_supervisors WHERE tenant_id = $1 AND user_id = $2`, tenantID, id)
}

// Accounts returns role and status for the given users that exist in the tenant.
func (pgRepository) Accounts(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]Account, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT id, role, status FROM users WHERE tenant_id = $1 AND id = ANY($2)`, tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("lookup accounts: %w", err)
	}
	defer rows.Close()
	out := make(map[uuid.UUID]Account, len(ids))
	for rows.Next() {
		var id uuid.UUID
		var a Account
		if err := rows.Scan(&id, &a.Role, &a.Status); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		out[id] = a
	}
	return out, rows.Err()
}

func (pgRepository) TenantName(ctx context.Context, tenantID uuid.UUID) (string, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return "", err
	}
	var name string
	if err := q.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, tenantID).Scan(&name); err != nil {
		return "", fmt.Errorf("get tenant name: %w", err)
	}
	return name, nil
}

func exec(ctx context.Context, op, sql string, args ...any) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
