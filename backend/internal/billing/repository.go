package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/limits"
	"callcenter/internal/platform/database"
)

// UsagePeriod is a closed billing period's usage and overage.
type UsagePeriod struct {
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	PlanID         uuid.UUID
	Start, End     time.Time
	CallSeconds    int64
	StorageBytes   int64
	OverageMinutes int
	OverageGB      int
	OverageAmount  int64
	Currency       string
}

// NewPayment is a payment to record.
type NewPayment struct {
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	Kind           string
	Provider       ProviderName
	Reference      string
	AmountMinor    int64
	Currency       string
	Status         string
	Failure        string
}

// Activation is what a successful checkout sets on a subscription.
type Activation struct {
	TenantID               uuid.UUID
	PlanID                 uuid.UUID
	PriceID                uuid.UUID
	Provider               ProviderName
	ExternalCustomerID     string
	ExternalSubscriptionID string
	Authorization          string
	BillingEmail           string
	PeriodStart, PeriodEnd time.Time
}

type PlanInput struct {
	Code        string
	Name        string
	Description string
	IsPublic    bool
	IsActive    bool
	limits.Caps
}

type PriceInput struct {
	Currency              string
	Interval              Interval
	AmountMinor           int64
	OveragePerMinuteMinor int64
	OveragePerGBMinor     int64
	Provider              ProviderName
	IsActive              bool
}

type Repository interface {
	// Plans and prices (platform-owned).
	ListPlans(ctx context.Context, publicOnly bool) ([]Plan, error)
	GetPlan(ctx context.Context, id uuid.UUID) (*Plan, error)
	FreePlan(ctx context.Context) (*Plan, error)
	InsertPlan(ctx context.Context, in PlanInput) (uuid.UUID, error)
	UpdatePlan(ctx context.Context, id uuid.UUID, in PlanInput) (bool, error)
	DeletePlan(ctx context.Context, id uuid.UUID) (bool, error)
	InsertPrice(ctx context.Context, planID uuid.UUID, in PriceInput) (uuid.UUID, error)
	GetPrice(ctx context.Context, id uuid.UUID) (*Price, error)
	UpdatePrice(ctx context.Context, id uuid.UUID, active bool, perMinute, perGB int64) (bool, error)
	DeletePrice(ctx context.Context, id uuid.UUID) (bool, error)
	SetPriceExternalID(ctx context.Context, id uuid.UUID, externalID string) error

	// Subscriptions.
	GetSubscription(ctx context.Context, tenantID uuid.UUID) (*Subscription, error)
	FindByExternal(ctx context.Context, provider ProviderName, externalSubscriptionID string) (*Subscription, error)
	FindByCustomer(ctx context.Context, provider ProviderName, externalCustomerID string) (*Subscription, error)
	Activate(ctx context.Context, a Activation) error
	// LinkSubscription records the provider's subscription id for a customer, once.
	LinkSubscription(ctx context.Context, tenantID uuid.UUID, externalSubscriptionID string) error
	SetPastDue(ctx context.Context, tenantID uuid.UUID) (bool, error)
	ClearPastDue(ctx context.Context, tenantID uuid.UUID) error
	SetCancelAtPeriodEnd(ctx context.Context, tenantID uuid.UUID, cancel bool) error
	FallBackToFree(ctx context.Context, tenantID, freePlanID uuid.UUID, start, end time.Time) error
	RollPeriod(ctx context.Context, tenantID uuid.UUID, start, end time.Time) error
	DueSubscriptions(ctx context.Context, now time.Time) ([]uuid.UUID, error)
	PastDueSince(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error)
	LockTenant(ctx context.Context, tenantID uuid.UUID) error

	// Usage, payments, notices, policies, webhooks.
	InsertUsagePeriod(ctx context.Context, u UsagePeriod) (uuid.UUID, bool, error)
	LinkOveragePayment(ctx context.Context, usageID, paymentID uuid.UUID) error
	InsertPayment(ctx context.Context, p NewPayment) (uuid.UUID, bool, error)
	ListPayments(ctx context.Context, tenantID uuid.UUID, limit int) ([]Payment, error)
	InsertNotice(ctx context.Context, tenantID uuid.UUID, periodStart time.Time, cap string, threshold int) (bool, error)
	SetPolicies(ctx context.Context, tenantID uuid.UUID, p limits.Policies) error
	ResetPolicies(ctx context.Context, tenantID uuid.UUID) error
	InsertWebhookEvent(ctx context.Context, provider ProviderName, externalID, eventType string, payload []byte) (bool, error)
	MarkWebhookProcessed(ctx context.Context, provider ProviderName, externalID string) error
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func conn(ctx context.Context) (database.Querier, error) { return database.Conn(ctx) }

// ---- plans and prices ----------------------------------------------------------------------------

const planColumns = `id, code, name, description, is_free, is_public, is_active,
	max_agents, max_call_minutes_per_month, max_storage_bytes, max_departments, max_widgets, created_at, updated_at`

func scanPlan(row interface{ Scan(...any) error }) (Plan, error) {
	var p Plan
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.IsFree, &p.IsPublic, &p.IsActive,
		&p.MaxAgents, &p.MaxCallMinutes, &p.MaxStorageBytes, &p.MaxDepartments, &p.MaxWidgets, &p.CreatedAt, &p.UpdatedAt)
	p.Prices = []Price{}
	return p, err
}

const priceColumns = `id, plan_id, currency, interval, amount_minor, overage_per_minute_minor, overage_per_gb_minor, provider, external_id, is_active`

func scanPrice(row interface{ Scan(...any) error }) (Price, error) {
	var p Price
	err := row.Scan(&p.ID, &p.PlanID, &p.Currency, &p.Interval, &p.AmountMinor, &p.OveragePerMinuteMinor, &p.OveragePerGBMinor,
		&p.Provider, &p.ExternalID, &p.IsActive)
	p.Synced = p.ExternalID != nil
	return p, err
}

func (pgRepository) ListPlans(ctx context.Context, publicOnly bool) ([]Plan, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT `+planColumns+` FROM plans WHERE (NOT $1 OR (is_public AND is_active)) ORDER BY is_free DESC, created_at, id`, publicOnly)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	var plans []Plan
	index := map[uuid.UUID]int{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		index[p.ID] = len(plans)
		plans = append(plans, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	prices, err := q.Query(ctx, `SELECT `+priceColumns+` FROM plan_prices WHERE (NOT $1 OR is_active) ORDER BY currency, interval`, publicOnly)
	if err != nil {
		return nil, fmt.Errorf("list prices: %w", err)
	}
	defer prices.Close()
	for prices.Next() {
		p, err := scanPrice(prices)
		if err != nil {
			return nil, fmt.Errorf("scan price: %w", err)
		}
		if i, ok := index[p.PlanID]; ok {
			plans[i].Prices = append(plans[i].Prices, p)
		}
	}
	return plans, prices.Err()
}

func (r pgRepository) GetPlan(ctx context.Context, id uuid.UUID) (*Plan, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	p, err := scanPlan(q.QueryRow(ctx, `SELECT `+planColumns+` FROM plans WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+priceColumns+` FROM plan_prices WHERE plan_id = $1 ORDER BY currency, interval`, id)
	if err != nil {
		return nil, fmt.Errorf("get plan prices: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		pr, err := scanPrice(rows)
		if err != nil {
			return nil, fmt.Errorf("scan price: %w", err)
		}
		p.Prices = append(p.Prices, pr)
	}
	return &p, rows.Err()
}

func (r pgRepository) FreePlan(ctx context.Context) (*Plan, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	if err := q.QueryRow(ctx, `SELECT id FROM plans WHERE is_free`).Scan(&id); err != nil {
		return nil, fmt.Errorf("free plan: %w", err)
	}
	return r.GetPlan(ctx, id)
}

func (pgRepository) InsertPlan(ctx context.Context, in PlanInput) (uuid.UUID, error) {
	q, err := conn(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO plans (code, name, description, is_public, is_active, max_agents, max_call_minutes_per_month,
		                   max_storage_bytes, max_departments, max_widgets)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		in.Code, in.Name, in.Description, in.IsPublic, in.IsActive, in.MaxAgents, in.MaxCallMinutes, in.MaxStorageBytes,
		in.MaxDepartments, in.MaxWidgets).Scan(&id)
	return id, err
}

func (pgRepository) UpdatePlan(ctx context.Context, id uuid.UUID, in PlanInput) (bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return false, err
	}
	// The free plan keeps its code and stays active and public; everything else can change.
	tag, err := q.Exec(ctx, `
		UPDATE plans SET code = CASE WHEN is_free THEN code ELSE $2 END, name = $3, description = $4,
		    is_public = CASE WHEN is_free THEN true ELSE $5 END, is_active = CASE WHEN is_free THEN true ELSE $6 END,
		    max_agents = $7, max_call_minutes_per_month = $8, max_storage_bytes = $9, max_departments = $10, max_widgets = $11
		WHERE id = $1`,
		id, in.Code, in.Name, in.Description, in.IsPublic, in.IsActive, in.MaxAgents, in.MaxCallMinutes, in.MaxStorageBytes,
		in.MaxDepartments, in.MaxWidgets)
	return tag.RowsAffected() == 1, err
}

func (pgRepository) DeletePlan(ctx context.Context, id uuid.UUID) (bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return false, err
	}
	// Prices go with their plan, unless a subscription still uses one (the foreign key refuses).
	if _, err := q.Exec(ctx, `DELETE FROM plan_prices WHERE plan_id = $1 AND EXISTS (SELECT 1 FROM plans WHERE id = $1 AND NOT is_free)`, id); err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `DELETE FROM plans WHERE id = $1 AND NOT is_free`, id)
	return tag.RowsAffected() == 1, err
}

func (pgRepository) InsertPrice(ctx context.Context, planID uuid.UUID, in PriceInput) (uuid.UUID, error) {
	q, err := conn(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO plan_prices (plan_id, currency, interval, amount_minor, overage_per_minute_minor, overage_per_gb_minor, provider, is_active)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8 FROM plans WHERE id = $1 AND NOT is_free
		RETURNING id`, planID, in.Currency, in.Interval, in.AmountMinor, in.OveragePerMinuteMinor, in.OveragePerGBMinor, in.Provider, in.IsActive).Scan(&id)
	return id, err
}

func (pgRepository) GetPrice(ctx context.Context, id uuid.UUID) (*Price, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	p, err := scanPrice(q.QueryRow(ctx, `SELECT `+priceColumns+` FROM plan_prices WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get price: %w", err)
	}
	return &p, nil
}

func (pgRepository) UpdatePrice(ctx context.Context, id uuid.UUID, active bool, perMinute, perGB int64) (bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `UPDATE plan_prices SET is_active = $2, overage_per_minute_minor = $3, overage_per_gb_minor = $4 WHERE id = $1`,
		id, active, perMinute, perGB)
	return tag.RowsAffected() == 1, err
}

func (pgRepository) DeletePrice(ctx context.Context, id uuid.UUID) (bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `DELETE FROM plan_prices WHERE id = $1`, id)
	return tag.RowsAffected() == 1, err
}

func (pgRepository) SetPriceExternalID(ctx context.Context, id uuid.UUID, externalID string) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE plan_prices SET external_id = $2 WHERE id = $1`, id, externalID)
	return err
}

// ---- subscriptions -------------------------------------------------------------------------------

const subColumns = `id, tenant_id, plan_id, plan_price_id, provider, external_customer_id, external_subscription_id,
	external_authorization, billing_email, status, past_due_since, cancel_at_period_end, current_period_start, current_period_end`

func scanSub(row interface{ Scan(...any) error }) (*Subscription, error) {
	var s Subscription
	err := row.Scan(&s.ID, &s.TenantID, &s.PlanID, &s.PriceID, &s.Provider, &s.ExternalCustomerID, &s.ExternalSubscriptionID,
		&s.ExternalAuthorization, &s.BillingEmail, &s.Status, &s.PastDueSince, &s.CancelAtPeriodEnd, &s.PeriodStart, &s.PeriodEnd)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan subscription: %w", err)
	}
	return &s, nil
}

func (pgRepository) GetSubscription(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanSub(q.QueryRow(ctx, `SELECT `+subColumns+` FROM subscriptions WHERE tenant_id = $1`, tenantID))
}

func (pgRepository) FindByExternal(ctx context.Context, provider ProviderName, externalSubscriptionID string) (*Subscription, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanSub(q.QueryRow(ctx, `SELECT `+subColumns+` FROM subscriptions WHERE provider = $1 AND external_subscription_id = $2`,
		provider, externalSubscriptionID))
}

func (pgRepository) FindByCustomer(ctx context.Context, provider ProviderName, externalCustomerID string) (*Subscription, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanSub(q.QueryRow(ctx, `SELECT `+subColumns+` FROM subscriptions WHERE provider = $1 AND external_customer_id = $2`,
		provider, externalCustomerID))
}

func (pgRepository) Activate(ctx context.Context, a Activation) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		UPDATE subscriptions SET plan_id = $2, plan_price_id = $3, provider = $4,
		    external_customer_id = nullif($5, ''), external_subscription_id = nullif($6, ''), external_authorization = nullif($7, ''),
		    billing_email = nullif($8, ''),
		    status = 'active', past_due_since = NULL, cancel_at_period_end = false,
		    current_period_start = $9, current_period_end = $10
		WHERE tenant_id = $1`,
		a.TenantID, a.PlanID, a.PriceID, a.Provider, a.ExternalCustomerID, a.ExternalSubscriptionID, a.Authorization,
		a.BillingEmail, a.PeriodStart, a.PeriodEnd)
	return err
}

func (pgRepository) LinkSubscription(ctx context.Context, tenantID uuid.UUID, externalSubscriptionID string) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE subscriptions SET external_subscription_id = $2 WHERE tenant_id = $1 AND external_subscription_id IS NULL`,
		tenantID, externalSubscriptionID)
	return err
}

func (pgRepository) SetPastDue(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `UPDATE subscriptions SET status = 'past_due', past_due_since = now() WHERE tenant_id = $1 AND status = 'active' AND plan_price_id IS NOT NULL`, tenantID)
	return tag.RowsAffected() == 1, err
}

func (pgRepository) ClearPastDue(ctx context.Context, tenantID uuid.UUID) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE subscriptions SET status = 'active', past_due_since = NULL WHERE tenant_id = $1 AND status = 'past_due'`, tenantID)
	return err
}

func (pgRepository) SetCancelAtPeriodEnd(ctx context.Context, tenantID uuid.UUID, cancel bool) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE subscriptions SET cancel_at_period_end = $2 WHERE tenant_id = $1`, tenantID, cancel)
	return err
}

func (pgRepository) FallBackToFree(ctx context.Context, tenantID, freePlanID uuid.UUID, start, end time.Time) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		UPDATE subscriptions SET plan_id = $2, plan_price_id = NULL, provider = NULL, external_customer_id = NULL,
		    external_subscription_id = NULL, external_authorization = NULL, billing_email = NULL, status = 'active', past_due_since = NULL,
		    cancel_at_period_end = false, current_period_start = $3, current_period_end = $4
		WHERE tenant_id = $1`, tenantID, freePlanID, start, end)
	return err
}

func (pgRepository) RollPeriod(ctx context.Context, tenantID uuid.UUID, start, end time.Time) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE subscriptions SET current_period_start = $2, current_period_end = $3 WHERE tenant_id = $1`, tenantID, start, end)
	return err
}

func (pgRepository) tenantIDs(ctx context.Context, sql string, args ...any) ([]uuid.UUID, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
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

func (r pgRepository) DueSubscriptions(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	return r.tenantIDs(ctx, `SELECT tenant_id FROM subscriptions WHERE current_period_end <= $1 AND status IN ('active', 'past_due') ORDER BY current_period_end LIMIT 100`, now)
}

func (r pgRepository) PastDueSince(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	return r.tenantIDs(ctx, `SELECT tenant_id FROM subscriptions WHERE status = 'past_due' AND past_due_since <= $1 LIMIT 100`, cutoff)
}

func (pgRepository) LockTenant(ctx context.Context, tenantID uuid.UUID) error {
	return database.LockTenant(ctx, tenantID)
}

// ---- usage, payments, notices, policies, webhooks ----------------------------------------------

func (pgRepository) InsertUsagePeriod(ctx context.Context, u UsagePeriod) (uuid.UUID, bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO usage_periods (tenant_id, subscription_id, plan_id, period_start, period_end, call_seconds, storage_bytes,
		                           overage_minutes, overage_gb, overage_amount_minor, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, nullif($11, ''))
		ON CONFLICT (tenant_id, period_start) DO NOTHING RETURNING id`,
		u.TenantID, u.SubscriptionID, u.PlanID, u.Start, u.End, u.CallSeconds, u.StorageBytes, u.OverageMinutes, u.OverageGB,
		u.OverageAmount, u.Currency).Scan(&id)
	if database.IsNoRows(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("insert usage period: %w", err)
	}
	return id, true, nil
}

func (pgRepository) LinkOveragePayment(ctx context.Context, usageID, paymentID uuid.UUID) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE usage_periods SET overage_payment_id = $2 WHERE id = $1`, usageID, paymentID)
	return err
}

func (pgRepository) InsertPayment(ctx context.Context, p NewPayment) (uuid.UUID, bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO payments (tenant_id, subscription_id, kind, provider, external_reference, amount_minor, currency, status, failure)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, nullif($9, ''))
		ON CONFLICT (provider, external_reference, status) DO NOTHING RETURNING id`,
		p.TenantID, p.SubscriptionID, p.Kind, p.Provider, p.Reference, p.AmountMinor, p.Currency, p.Status, p.Failure).Scan(&id)
	if database.IsNoRows(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("insert payment: %w", err)
	}
	return id, true, nil
}

func (pgRepository) ListPayments(ctx context.Context, tenantID uuid.UUID, limit int) ([]Payment, error) {
	q, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT id, kind, provider, external_reference, amount_minor, currency, status, failure, paid_at
		FROM payments WHERE tenant_id = $1 ORDER BY paid_at DESC, id DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()
	out := []Payment{}
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.Kind, &p.Provider, &p.Reference, &p.AmountMinor, &p.Currency, &p.Status, &p.Failure, &p.PaidAt); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (pgRepository) InsertNotice(ctx context.Context, tenantID uuid.UUID, periodStart time.Time, cap string, threshold int) (bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `INSERT INTO usage_notices (tenant_id, period_start, cap, threshold) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		tenantID, periodStart, cap, threshold)
	return tag.RowsAffected() == 1, err
}

func (pgRepository) SetPolicies(ctx context.Context, tenantID uuid.UUID, p limits.Policies) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE tenant_settings SET minutes_cap_policy = $2, storage_cap_policy = $3 WHERE tenant_id = $1`,
		tenantID, p.Minutes, p.Storage)
	return err
}

func (r pgRepository) ResetPolicies(ctx context.Context, tenantID uuid.UUID) error {
	return r.SetPolicies(ctx, tenantID, limits.Policies{Minutes: limits.PolicyBlock, Storage: limits.PolicyBlock})
}

func (pgRepository) InsertWebhookEvent(ctx context.Context, provider ProviderName, externalID, eventType string, payload []byte) (bool, error) {
	q, err := conn(ctx)
	if err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, `
		INSERT INTO billing_webhook_events (provider, external_event_id, type, payload) VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (provider, external_event_id) DO NOTHING`, provider, externalID, eventType, string(payload))
	return tag.RowsAffected() == 1, err
}

func (pgRepository) MarkWebhookProcessed(ctx context.Context, provider ProviderName, externalID string) error {
	q, err := conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE billing_webhook_events SET processed_at = now() WHERE provider = $1 AND external_event_id = $2`, provider, externalID)
	return err
}
