package tenant

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
)

// Repository persists tenants. Platform-admin methods run in platform scope.
type Repository interface {
	// InsertIfSlugFree returns nil, nil when the slug is already taken.
	InsertIfSlugFree(ctx context.Context, name, slug, status string) (*Tenant, error)
	InsertDefaultSettings(ctx context.Context, tenantID uuid.UUID) error
	List(ctx context.Context, f Filter, page httpx.Page) ([]Summary, int, error)
	Get(ctx context.Context, id uuid.UUID) (*Tenant, error)
	Counts(ctx context.Context, id uuid.UUID) (users, departments int, err error)
	Admins(ctx context.Context, id uuid.UUID) ([]AdminRef, error)
	Update(ctx context.Context, id uuid.UUID, name, slug string) error
	SetStatus(ctx context.Context, id uuid.UUID, status string) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

const tenantColumns = `id, name, slug, status, created_at, updated_at`

func scanTenant(row interface{ Scan(...any) error }) (Tenant, error) {
	var t Tenant
	err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (pgRepository) InsertIfSlugFree(ctx context.Context, name, slug, status string) (*Tenant, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	// ON CONFLICT avoids aborting the surrounding transaction on a slug collision.
	t, err := scanTenant(q.QueryRow(ctx, `
		INSERT INTO tenants (name, slug, status) VALUES ($1, $2, $3)
		ON CONFLICT (slug) DO NOTHING
		RETURNING `+tenantColumns, name, slug, status))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("insert tenant: %w", err)
	}
	return &t, nil
}

func (pgRepository) InsertDefaultSettings(ctx context.Context, tenantID uuid.UUID) error {
	return exec(ctx, "insert tenant settings", `INSERT INTO tenant_settings (tenant_id) VALUES ($1)`, tenantID)
}

func (pgRepository) List(ctx context.Context, f Filter, page httpx.Page) ([]Summary, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	const where = `WHERE ($1 = '' OR t.name ILIKE $1 OR t.slug ILIKE $1) AND ($2 = '' OR t.status = $2)`
	args := []any{database.ContainsPattern(f.Query), f.Status}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM tenants t `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tenants: %w", err)
	}
	rows, err := q.Query(ctx, `
		SELECT t.id, t.name, t.slug, t.status, t.created_at, t.updated_at,
		       (SELECT count(*) FROM users u WHERE u.tenant_id = t.id)
		FROM tenants t `+where+`
		ORDER BY t.created_at DESC, t.id DESC LIMIT $3 OFFSET $4`, append(args, page.PerPage, page.Offset())...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()
	var items []Summary
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.ID, &s.Name, &s.Slug, &s.Status, &s.CreatedAt, &s.UpdatedAt, &s.UsersCount); err != nil {
			return nil, 0, fmt.Errorf("scan tenant: %w", err)
		}
		items = append(items, s)
	}
	return items, total, rows.Err()
}

// Get returns nil, nil when the tenant does not exist.
func (pgRepository) Get(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	t, err := scanTenant(q.QueryRow(ctx, `SELECT `+tenantColumns+` FROM tenants WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &t, nil
}

func (pgRepository) Counts(ctx context.Context, id uuid.UUID) (int, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return 0, 0, err
	}
	var users, departments int
	err = q.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM users WHERE tenant_id = $1),
		       (SELECT count(*) FROM departments WHERE tenant_id = $1)`, id).Scan(&users, &departments)
	if err != nil {
		return 0, 0, fmt.Errorf("count tenant resources: %w", err)
	}
	return users, departments, nil
}

func (pgRepository) Admins(ctx context.Context, id uuid.UUID) ([]AdminRef, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT id, full_name, email::text, status FROM users
		WHERE tenant_id = $1 AND role = 'tenant_admin' ORDER BY created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("list tenant admins: %w", err)
	}
	defer rows.Close()
	admins := []AdminRef{}
	for rows.Next() {
		var a AdminRef
		if err := rows.Scan(&a.ID, &a.FullName, &a.Email, &a.Status); err != nil {
			return nil, fmt.Errorf("scan tenant admin: %w", err)
		}
		admins = append(admins, a)
	}
	return admins, rows.Err()
}

func (pgRepository) Update(ctx context.Context, id uuid.UUID, name, slug string) error {
	err := exec(ctx, "update tenant", `UPDATE tenants SET name = $2, slug = $3 WHERE id = $1`, id, name, slug)
	if database.IsUniqueViolation(err, "tenants_slug_key") {
		return errSlugTaken
	}
	return err
}

func (pgRepository) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	return exec(ctx, "set tenant status", `UPDATE tenants SET status = $2 WHERE id = $1`, id, status)
}

func (pgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return exec(ctx, "delete tenant", `DELETE FROM tenants WHERE id = $1`, id)
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
