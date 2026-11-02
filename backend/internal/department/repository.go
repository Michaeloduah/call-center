package department

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
)

// Repository persists departments inside the caller's tenant-scoped transaction.
type Repository interface {
	List(ctx context.Context, tenantID uuid.UUID, f Filter, page httpx.Page) ([]Summary, int, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*Department, error)
	Insert(ctx context.Context, tenantID uuid.UUID, in Input) (*Department, error)
	Update(ctx context.Context, tenantID, id uuid.UUID, in Input) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	Members(ctx context.Context, tenantID, id uuid.UUID) ([]UserRef, error)
	Supervisors(ctx context.Context, tenantID, id uuid.UUID) ([]UserRef, error)
	Hours(ctx context.Context, tenantID, id uuid.UUID) ([]HoursRange, error)
	ReplaceMembers(ctx context.Context, tenantID, id uuid.UUID, userIDs []uuid.UUID) error
	ReplaceSupervisors(ctx context.Context, tenantID, id uuid.UUID, userIDs []uuid.UUID) error
	ReplaceHours(ctx context.Context, tenantID, id uuid.UUID, hours []HoursRange) error
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

const departmentColumns = `id, name, description, timezone, is_active, created_at, updated_at`

func scanDepartment(row interface{ Scan(...any) error }) (Department, error) {
	var d Department
	err := row.Scan(&d.ID, &d.Name, &d.Description, &d.Timezone, &d.IsActive, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (pgRepository) List(ctx context.Context, tenantID uuid.UUID, f Filter, page httpx.Page) ([]Summary, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	const where = `WHERE d.tenant_id = $1 AND ($2 = '' OR d.name ILIKE $2) AND ($3::boolean IS NULL OR d.is_active = $3)`
	args := []any{tenantID, database.ContainsPattern(f.Query), f.Active}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM departments d `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count departments: %w", err)
	}
	rows, err := q.Query(ctx, `
		SELECT d.id, d.name, d.description, d.timezone, d.is_active, d.created_at, d.updated_at,
		       (SELECT count(*) FROM department_members m WHERE m.department_id = d.id),
		       (SELECT count(*) FROM department_supervisors s WHERE s.department_id = d.id)
		FROM departments d `+where+`
		ORDER BY lower(d.name), d.id LIMIT $4 OFFSET $5`, append(args, page.PerPage, page.Offset())...)
	if err != nil {
		return nil, 0, fmt.Errorf("list departments: %w", err)
	}
	defer rows.Close()
	var items []Summary
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Timezone, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
			&s.MembersCount, &s.SupervisorsCount); err != nil {
			return nil, 0, fmt.Errorf("scan department: %w", err)
		}
		items = append(items, s)
	}
	return items, total, rows.Err()
}

// Get returns nil, nil when the department does not exist in the tenant.
func (pgRepository) Get(ctx context.Context, tenantID, id uuid.UUID) (*Department, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	d, err := scanDepartment(q.QueryRow(ctx, `SELECT `+departmentColumns+` FROM departments WHERE tenant_id = $1 AND id = $2`, tenantID, id))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get department: %w", err)
	}
	return &d, nil
}

func (pgRepository) Insert(ctx context.Context, tenantID uuid.UUID, in Input) (*Department, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	d, err := scanDepartment(q.QueryRow(ctx, `
		INSERT INTO departments (tenant_id, name, description, timezone, is_active)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+departmentColumns,
		tenantID, in.Name, in.Description, in.Timezone, *in.IsActive))
	if database.IsUniqueViolation(err, "departments_tenant_name_key") {
		return nil, errNameTaken
	}
	if err != nil {
		return nil, fmt.Errorf("insert department: %w", err)
	}
	return &d, nil
}

func (pgRepository) Update(ctx context.Context, tenantID, id uuid.UUID, in Input) error {
	err := exec(ctx, "update department", `
		UPDATE departments SET name = $3, description = $4, timezone = $5, is_active = $6
		WHERE tenant_id = $1 AND id = $2`, tenantID, id, in.Name, in.Description, in.Timezone, *in.IsActive)
	if database.IsUniqueViolation(err, "departments_tenant_name_key") {
		return errNameTaken
	}
	return err
}

// Delete fails with errInUse while widgets point at the department or calls reference it.
func (pgRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	err := exec(ctx, "delete department", `DELETE FROM departments WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if database.IsForeignKeyViolation(err) {
		return errInUse
	}
	return err
}

func (pgRepository) Members(ctx context.Context, tenantID, id uuid.UUID) ([]UserRef, error) {
	return userRefs(ctx, `
		SELECT u.id, u.full_name, u.email::text, u.role, u.status
		FROM department_members m JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1 AND m.department_id = $2 ORDER BY lower(u.full_name)`, tenantID, id)
}

func (pgRepository) Supervisors(ctx context.Context, tenantID, id uuid.UUID) ([]UserRef, error) {
	return userRefs(ctx, `
		SELECT u.id, u.full_name, u.email::text, u.role, u.status
		FROM department_supervisors s JOIN users u ON u.id = s.user_id
		WHERE s.tenant_id = $1 AND s.department_id = $2 ORDER BY lower(u.full_name)`, tenantID, id)
}

func userRefs(ctx context.Context, sql string, args ...any) ([]UserRef, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list department users: %w", err)
	}
	defer rows.Close()
	refs := []UserRef{}
	for rows.Next() {
		var r UserRef
		if err := rows.Scan(&r.ID, &r.FullName, &r.Email, &r.Role, &r.Status); err != nil {
			return nil, fmt.Errorf("scan department user: %w", err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

func (pgRepository) Hours(ctx context.Context, tenantID, id uuid.UUID) ([]HoursRange, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT weekday, to_char(opens_at, 'HH24:MI'), to_char(closes_at, 'HH24:MI')
		FROM department_business_hours
		WHERE tenant_id = $1 AND department_id = $2 ORDER BY weekday, opens_at`, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("list business hours: %w", err)
	}
	defer rows.Close()
	hours := []HoursRange{}
	for rows.Next() {
		var h HoursRange
		if err := rows.Scan(&h.Weekday, &h.Opens, &h.Closes); err != nil {
			return nil, fmt.Errorf("scan business hours: %w", err)
		}
		hours = append(hours, h)
	}
	return hours, rows.Err()
}

func (pgRepository) ReplaceMembers(ctx context.Context, tenantID, id uuid.UUID, userIDs []uuid.UUID) error {
	return replaceLinks(ctx, "department_members", tenantID, id, userIDs)
}

func (pgRepository) ReplaceSupervisors(ctx context.Context, tenantID, id uuid.UUID, userIDs []uuid.UUID) error {
	return replaceLinks(ctx, "department_supervisors", tenantID, id, userIDs)
}

// replaceLinks swaps the full set of user links. table is a compile-time constant, never user input.
func replaceLinks(ctx context.Context, table string, tenantID, id uuid.UUID, userIDs []uuid.UUID) error {
	if err := exec(ctx, "clear "+table, `DELETE FROM `+table+` WHERE tenant_id = $1 AND department_id = $2`, tenantID, id); err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil
	}
	return exec(ctx, "insert "+table, `
		INSERT INTO `+table+` (department_id, user_id, tenant_id)
		SELECT $2, u, $1 FROM unnest($3::uuid[]) AS u`, tenantID, id, userIDs)
}

func (pgRepository) ReplaceHours(ctx context.Context, tenantID, id uuid.UUID, hours []HoursRange) error {
	if err := exec(ctx, "clear business hours",
		`DELETE FROM department_business_hours WHERE tenant_id = $1 AND department_id = $2`, tenantID, id); err != nil {
		return err
	}
	if len(hours) == 0 {
		return nil
	}
	weekdays := make([]int16, len(hours))
	opens := make([]string, len(hours))
	closes := make([]string, len(hours))
	for i, h := range hours {
		weekdays[i], opens[i], closes[i] = int16(h.Weekday), h.Opens, h.Closes
	}
	return exec(ctx, "insert business hours", `
		INSERT INTO department_business_hours (tenant_id, department_id, weekday, opens_at, closes_at)
		SELECT $1, $2, w, o::time, c::time FROM unnest($3::smallint[], $4::text[], $5::text[]) AS h(w, o, c)`,
		tenantID, id, weekdays, opens, closes)
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
