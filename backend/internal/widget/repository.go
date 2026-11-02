package widget

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/department"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
)

// Repository persists widgets inside the caller's scoped transaction.
type Repository interface {
	List(ctx context.Context, tenantID uuid.UUID, f Filter, page httpx.Page) ([]Widget, int, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*Widget, error)
	Insert(ctx context.Context, tenantID uuid.UUID, publicKey string, in Input) (uuid.UUID, error)
	Update(ctx context.Context, tenantID, id uuid.UUID, in Input) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	// Resolve looks a widget up by public key across tenants (platform scope). It returns nil
	// when the widget does not exist, is inactive, or its tenant is not active.
	Resolve(ctx context.Context, publicKey string) (*Resolved, error)
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

const widgetSelect = `
	SELECT w.id, w.department_id, d.name, w.name, w.public_key, w.allowed_origins, w.is_active,
	       w.button_label, w.accent_color, w.greeting, w.form_enabled, w.name_field, w.phone_field, w.email_field,
	       w.created_at, w.updated_at
	FROM widgets w JOIN departments d ON d.id = w.department_id`

func scanWidget(row interface{ Scan(...any) error }) (Widget, error) {
	var w Widget
	err := row.Scan(&w.ID, &w.DepartmentID, &w.DepartmentName, &w.Name, &w.PublicKey, &w.AllowedOrigins, &w.IsActive,
		&w.ButtonLabel, &w.AccentColor, &w.Greeting, &w.CallerForm.Enabled, &w.CallerForm.Name, &w.CallerForm.Phone,
		&w.CallerForm.Email, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

func (pgRepository) List(ctx context.Context, tenantID uuid.UUID, f Filter, page httpx.Page) ([]Widget, int, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, 0, err
	}
	const where = ` WHERE w.tenant_id = $1 AND ($2 = '' OR w.name ILIKE $2 OR d.name ILIKE $2)`
	args := []any{tenantID, database.ContainsPattern(f.Query)}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM widgets w JOIN departments d ON d.id = w.department_id`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count widgets: %w", err)
	}
	rows, err := q.Query(ctx, widgetSelect+where+` ORDER BY w.created_at DESC, w.id DESC LIMIT $3 OFFSET $4`,
		append(args, page.PerPage, page.Offset())...)
	if err != nil {
		return nil, 0, fmt.Errorf("list widgets: %w", err)
	}
	defer rows.Close()
	var items []Widget
	for rows.Next() {
		w, err := scanWidget(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan widget: %w", err)
		}
		items = append(items, w)
	}
	return items, total, rows.Err()
}

// Get returns nil, nil when the widget does not exist in the tenant.
func (pgRepository) Get(ctx context.Context, tenantID, id uuid.UUID) (*Widget, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	w, err := scanWidget(q.QueryRow(ctx, widgetSelect+` WHERE w.tenant_id = $1 AND w.id = $2`, tenantID, id))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get widget: %w", err)
	}
	return &w, nil
}

func (pgRepository) Insert(ctx context.Context, tenantID uuid.UUID, publicKey string, in Input) (uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO widgets (tenant_id, department_id, name, public_key, allowed_origins, is_active, button_label,
		                     accent_color, greeting, form_enabled, name_field, phone_field, email_field)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id`,
		tenantID, in.DepartmentID, in.Name, publicKey, in.AllowedOrigins, *in.IsActive, in.ButtonLabel, in.AccentColor,
		in.Greeting, in.CallerForm.Enabled, in.CallerForm.Name, in.CallerForm.Phone, in.CallerForm.Email).Scan(&id)
	// The composite (department_id, tenant_id) key also rejects another tenant's department.
	if database.IsForeignKeyViolation(err) {
		return uuid.Nil, errDepartmentNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert widget: %w", err)
	}
	return id, nil
}

func (pgRepository) Update(ctx context.Context, tenantID, id uuid.UUID, in Input) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		UPDATE widgets
		SET department_id = $3, name = $4, allowed_origins = $5, is_active = $6, button_label = $7, accent_color = $8,
		    greeting = $9, form_enabled = $10, name_field = $11, phone_field = $12, email_field = $13
		WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, in.DepartmentID, in.Name, in.AllowedOrigins, *in.IsActive, in.ButtonLabel, in.AccentColor,
		in.Greeting, in.CallerForm.Enabled, in.CallerForm.Name, in.CallerForm.Phone, in.CallerForm.Email)
	if database.IsForeignKeyViolation(err) {
		return errDepartmentNotFound
	}
	if err != nil {
		return fmt.Errorf("update widget: %w", err)
	}
	return nil
}

func (pgRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM widgets WHERE tenant_id = $1 AND id = $2`, tenantID, id); err != nil {
		return fmt.Errorf("delete widget: %w", err)
	}
	return nil
}

func (pgRepository) Resolve(ctx context.Context, publicKey string) (*Resolved, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var r Resolved
	err = q.QueryRow(ctx, `
		SELECT w.id, w.tenant_id, t.name, d.id, d.name, d.is_active, d.timezone, w.public_key, w.allowed_origins,
		       w.button_label, w.accent_color, w.greeting, w.form_enabled, w.name_field, w.phone_field, w.email_field,
		       coalesce(ts.consent_text_override, ps.default_consent_text)
		FROM widgets w
		JOIN tenants t ON t.id = w.tenant_id AND t.status = 'active'
		JOIN departments d ON d.id = w.department_id
		JOIN tenant_settings ts ON ts.tenant_id = w.tenant_id
		CROSS JOIN platform_settings ps
		WHERE w.public_key = $1 AND w.is_active`, publicKey).
		Scan(&r.WidgetID, &r.TenantID, &r.TenantName, &r.DepartmentID, &r.DepartmentName, &r.DepartmentActive, &r.Timezone,
			&r.PublicKey, &r.AllowedOrigins, &r.ButtonLabel, &r.AccentColor, &r.Greeting, &r.CallerForm.Enabled,
			&r.CallerForm.Name, &r.CallerForm.Phone, &r.CallerForm.Email, &r.ConsentText)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve widget: %w", err)
	}
	rows, err := q.Query(ctx, `
		SELECT weekday, to_char(opens_at, 'HH24:MI'), to_char(closes_at, 'HH24:MI')
		FROM department_business_hours WHERE department_id = $1 ORDER BY weekday, opens_at`, r.DepartmentID)
	if err != nil {
		return nil, fmt.Errorf("resolve widget hours: %w", err)
	}
	defer rows.Close()
	r.Hours = []department.HoursRange{}
	for rows.Next() {
		var h department.HoursRange
		if err := rows.Scan(&h.Weekday, &h.Opens, &h.Closes); err != nil {
			return nil, fmt.Errorf("scan widget hours: %w", err)
		}
		r.Hours = append(r.Hours, h)
	}
	return &r, rows.Err()
}
