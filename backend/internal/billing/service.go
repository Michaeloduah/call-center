package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/limits"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/tenant"
)

type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Tenants reads tenants and their admins (implemented by tenant.Repository).
type Tenants interface {
	Get(ctx context.Context, id uuid.UUID) (*tenant.Tenant, error)
	Admins(ctx context.Context, id uuid.UUID) ([]tenant.AdminRef, error)
}

// Usage reads a tenant's limits and usage (implemented by limits.Service).
type Usage interface {
	Snapshot(ctx context.Context, tenantID uuid.UUID) (*limits.Snapshot, error)
}

type Config struct {
	// ReturnURL is where providers send the browser after checkout (the billing page).
	ReturnURL string
	// RecentPayments is how many payments the billing page lists.
	RecentPayments int
}

type Service struct {
	tx        database.TxManager
	repo      Repository
	usage     Usage
	providers Registry
	tenants   Tenants
	notifier  Notifier
	audit     AuditRecorder
	cfg       Config
	logger    *slog.Logger
	now       func() time.Time
}

func NewService(tx database.TxManager, repo Repository, usage Usage, providers Registry, tenants Tenants, notifier Notifier,
	recorder AuditRecorder, cfg Config, logger *slog.Logger) *Service {
	if cfg.RecentPayments == 0 {
		cfg.RecentPayments = 20
	}
	return &Service{tx: tx, repo: repo, usage: usage, providers: providers, tenants: tenants, notifier: notifier,
		audit: recorder, cfg: cfg, logger: logger, now: time.Now}
}

// ---- time ---------------------------------------------------------------------------------------

// monthOf returns the UTC calendar month containing t (the free tier's billing period).
func monthOf(t time.Time) (start, end time.Time) {
	t = t.UTC()
	start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func addInterval(t time.Time, i Interval) time.Time {
	if i == IntervalYearly {
		return t.AddDate(1, 0, 0)
	}
	return t.AddDate(0, 1, 0)
}

// ---- platform: plans and prices ---------------------------------------------------------------

func (s *Service) ListPlans(ctx context.Context) ([]Plan, error) {
	var out []Plan
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		out, err = s.repo.ListPlans(ctx, false)
		return err
	})
	return out, err
}

func (s *Service) GetPlan(ctx context.Context, id uuid.UUID) (*Plan, error) {
	var p *Plan
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		if p, err = s.repo.GetPlan(ctx, id); err == nil && p == nil {
			err = errPlanNotFound
		}
		return err
	})
	return p, err
}

func (s *Service) CreatePlan(ctx context.Context, b PlanBody) (*Plan, error) {
	var p *Plan
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		id, err := s.repo.InsertPlan(ctx, b.input())
		if database.IsUniqueViolation(err, "") {
			return apperr.FieldConflict("code", "A plan with this code already exists.")
		}
		if err != nil {
			return err
		}
		if p, err = s.repo.GetPlan(ctx, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{Action: audit.ActionPlanCreated, EntityType: "plan", EntityID: id, Metadata: map[string]any{"code": b.Code}})
	})
	return p, err
}

func (s *Service) UpdatePlan(ctx context.Context, id uuid.UUID, b PlanBody) (*Plan, error) {
	var p *Plan
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		ok, err := s.repo.UpdatePlan(ctx, id, b.input())
		if database.IsUniqueViolation(err, "") {
			return apperr.FieldConflict("code", "A plan with this code already exists.")
		}
		if err != nil {
			return err
		}
		if !ok {
			return errPlanNotFound
		}
		if p, err = s.repo.GetPlan(ctx, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{Action: audit.ActionPlanUpdated, EntityType: "plan", EntityID: id, Metadata: map[string]any{"code": p.Code}})
	})
	return p, err
}

func (s *Service) DeletePlan(ctx context.Context, id uuid.UUID) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		plan, err := s.repo.GetPlan(ctx, id)
		if err != nil {
			return err
		}
		if plan == nil {
			return errPlanNotFound
		}
		if plan.IsFree {
			return errFreePlan
		}
		ok, err := s.repo.DeletePlan(ctx, id)
		if database.IsForeignKeyViolation(err) {
			return errPlanInUse
		}
		if err != nil {
			return err
		}
		if !ok {
			return errPlanNotFound
		}
		return s.audit.Record(ctx, audit.Entry{Action: audit.ActionPlanDeleted, EntityType: "plan", EntityID: id, Metadata: map[string]any{"code": plan.Code}})
	})
}

func (s *Service) CreatePrice(ctx context.Context, planID uuid.UUID, b PriceBody) (*Plan, error) {
	var p *Plan
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		plan, err := s.repo.GetPlan(ctx, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return errPlanNotFound
		}
		if plan.IsFree {
			return errFreePlan
		}
		id, err := s.repo.InsertPrice(ctx, planID, PriceInput{
			Currency: b.Currency, Interval: b.Interval, AmountMinor: b.AmountMinor, OveragePerMinuteMinor: b.OveragePerMinuteMinor,
			OveragePerGBMinor: b.OveragePerGBMinor, Provider: s.providers.ProviderFor(b.Currency), IsActive: b.IsActive,
		})
		if database.IsUniqueViolation(err, "") {
			return apperr.FieldConflict("currency", "This plan already has a price in that currency and interval.")
		}
		if err != nil {
			return err
		}
		if p, err = s.repo.GetPlan(ctx, planID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{Action: audit.ActionPriceCreated, EntityType: "plan_price", EntityID: id,
			Metadata: map[string]any{"plan": plan.Code, "currency": b.Currency, "interval": b.Interval, "amount_minor": b.AmountMinor}})
	})
	return p, err
}

func (s *Service) UpdatePrice(ctx context.Context, id uuid.UUID, b PriceUpdateBody) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		ok, err := s.repo.UpdatePrice(ctx, id, b.IsActive, b.OveragePerMinuteMinor, b.OveragePerGBMinor)
		if err != nil {
			return err
		}
		if !ok {
			return errPriceNotFound
		}
		return s.audit.Record(ctx, audit.Entry{Action: audit.ActionPriceUpdated, EntityType: "plan_price", EntityID: id})
	})
}

func (s *Service) DeletePrice(ctx context.Context, id uuid.UUID) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		ok, err := s.repo.DeletePrice(ctx, id)
		if database.IsForeignKeyViolation(err) {
			return errPriceInUse
		}
		if err != nil {
			return err
		}
		if !ok {
			return errPriceNotFound
		}
		return s.audit.Record(ctx, audit.Entry{Action: audit.ActionPriceDeleted, EntityType: "plan_price", EntityID: id})
	})
}

// SyncPrice makes a price exist at its provider so tenants can check out with it.
func (s *Service) SyncPrice(ctx context.Context, id uuid.UUID) error {
	var plan *Plan
	var price *Price
	if err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		var err error
		if price, err = s.repo.GetPrice(ctx, id); err != nil {
			return err
		}
		if price == nil {
			return errPriceNotFound
		}
		plan, err = s.repo.GetPlan(ctx, price.PlanID)
		return err
	}); err != nil {
		return err
	}
	provider, err := s.providers.Get(price.Provider)
	if err != nil {
		return err
	}
	externalID, err := provider.SyncPrice(ctx, *plan, *price)
	if err != nil {
		s.logger.Error("sync price", "price_id", id, "provider", price.Provider, "error", err)
		return errProviderDown
	}
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		if err := s.repo.SetPriceExternalID(ctx, id, externalID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{Action: audit.ActionPriceSynced, EntityType: "plan_price", EntityID: id})
	})
}

// PlatformTenantBilling shows one tenant's subscription, usage and payments to the platform admin.
func (s *Service) PlatformTenantBilling(ctx context.Context, tenantID uuid.UUID) (*PlatformTenantBilling, error) {
	var out PlatformTenantBilling
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		sub, err := s.repo.GetSubscription(ctx, tenantID)
		if err != nil {
			return err
		}
		if sub == nil {
			return apperr.NotFound("Subscription")
		}
		out.Subscription = *sub
		plan, err := s.repo.GetPlan(ctx, sub.PlanID)
		if err != nil {
			return err
		}
		out.Plan = *plan
		if sub.PriceID != nil {
			if out.Price, err = s.repo.GetPrice(ctx, *sub.PriceID); err != nil {
				return err
			}
		}
		snap, err := s.usage.Snapshot(ctx, tenantID)
		if err != nil {
			return err
		}
		out.Usage = snap.Usage
		out.Payments, err = s.repo.ListPayments(ctx, tenantID, s.cfg.RecentPayments)
		return err
	})
	return &out, err
}

// ---- tenant: overview, policies, checkout, cancel --------------------------------------------

// Plans lists the plans a tenant can choose from.
func (s *Service) Plans(ctx context.Context) ([]Plan, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var out []Plan
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		out, err = s.repo.ListPlans(ctx, true)
		return err
	})
	return out, err
}

func (s *Service) Overview(ctx context.Context) (*Overview, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var o Overview
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		sub, err := s.repo.GetSubscription(ctx, p.TenantID)
		if err != nil {
			return err
		}
		if sub == nil {
			return apperr.NotFound("Subscription")
		}
		o.Subscription = *sub
		plan, err := s.repo.GetPlan(ctx, sub.PlanID)
		if err != nil {
			return err
		}
		o.Plan = *plan
		if sub.PriceID != nil {
			if o.Price, err = s.repo.GetPrice(ctx, *sub.PriceID); err != nil {
				return err
			}
		}
		snap, err := s.usage.Snapshot(ctx, p.TenantID)
		if err != nil {
			return err
		}
		o.Limits, o.Policies, o.Usage, o.AgentsWithoutSeat = snap.Caps, snap.Policies, snap.Usage, snap.AgentsWithout
		if o.Payments, err = s.repo.ListPayments(ctx, p.TenantID, s.cfg.RecentPayments); err != nil {
			return err
		}
		o.Plans, err = s.repo.ListPlans(ctx, true)
		return err
	})
	return &o, err
}

// SetPolicies stores the tenant's choice per cap (D16). "Pay overage" needs a paid plan with a
// rate for that cap, so it can never run up an unpriced bill.
func (s *Service) SetPolicies(ctx context.Context, b PoliciesBody) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	return s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		sub, err := s.repo.GetSubscription(ctx, p.TenantID)
		if err != nil {
			return err
		}
		var price *Price
		if sub.PriceID != nil {
			if price, err = s.repo.GetPrice(ctx, *sub.PriceID); err != nil {
				return err
			}
		}
		if b.Minutes == limits.PolicyOverage || b.Storage == limits.PolicyOverage {
			if price == nil {
				return errNeedsPaidPlan
			}
			if (b.Minutes == limits.PolicyOverage && price.OveragePerMinuteMinor == 0) ||
				(b.Storage == limits.PolicyOverage && price.OveragePerGBMinor == 0) {
				return errNoOverageRate
			}
		}
		if err := s.repo.SetPolicies(ctx, p.TenantID, limits.Policies{Minutes: b.Minutes, Storage: b.Storage}); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionCapPoliciesUpdated, EntityType: "tenant_settings",
			EntityID: p.TenantID, Metadata: map[string]any{"minutes": b.Minutes, "storage": b.Storage}})
	})
}

// Checkout starts a hosted checkout for a price and returns where to send the admin to pay.
// Nothing changes locally until the provider's webhook arrives.
func (s *Service) Checkout(ctx context.Context, b CheckoutBody) (*CheckoutResult, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var req CheckoutRequest
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		price, err := s.repo.GetPrice(ctx, b.PriceID)
		if err != nil {
			return err
		}
		if price == nil || !price.IsActive {
			return errPriceNotFound
		}
		plan, err := s.repo.GetPlan(ctx, price.PlanID)
		if err != nil {
			return err
		}
		if plan == nil || !plan.IsActive || !plan.IsPublic {
			return errPriceNotFound
		}
		if price.ExternalID == nil && price.Provider != ProviderFake {
			return errNotSynced
		}
		sub, err := s.repo.GetSubscription(ctx, p.TenantID)
		if err != nil {
			return err
		}
		if sub.PriceID != nil && *sub.PriceID == price.ID && !sub.CancelAtPeriodEnd {
			return errAlreadyOnPrice
		}
		req = CheckoutRequest{
			TenantID: p.TenantID, Plan: *plan, Price: *price, Reference: "chk_" + uuid.NewString(),
			CustomerEmail: p.Email, CustomerName: p.FullName, ReturnURL: s.cfg.ReturnURL,
		}
		if sub.ExternalCustomerID != nil && sub.Provider != nil && *sub.Provider == price.Provider {
			req.CustomerID = *sub.ExternalCustomerID
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionCheckoutStarted, EntityType: "plan_price",
			EntityID: price.ID, Metadata: map[string]any{"plan": plan.Code, "reference": req.Reference}})
	})
	if err != nil {
		return nil, err
	}
	provider, err := s.providers.Get(req.Price.Provider)
	if err != nil {
		return nil, err
	}
	url, err := provider.CreateCheckout(ctx, req)
	if err != nil {
		s.logger.Error("create checkout", "tenant_id", p.TenantID, "provider", req.Price.Provider, "error", err)
		return nil, errProviderDown
	}
	return &CheckoutResult{RedirectURL: url}, nil
}

// Cancel stops renewal at the end of the paid period; the tenant keeps the plan until then.
func (s *Service) Cancel(ctx context.Context) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	var provider ProviderName
	var external string
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		sub, err := s.repo.GetSubscription(ctx, p.TenantID)
		if err != nil {
			return err
		}
		if sub.PriceID == nil || sub.ExternalSubscriptionID == nil || sub.Provider == nil {
			return errNothingToStop
		}
		provider, external = *sub.Provider, *sub.ExternalSubscriptionID
		return nil
	})
	if err != nil {
		return err
	}
	client, err := s.providers.Get(provider)
	if err != nil {
		return err
	}
	if err := client.CancelSubscription(ctx, external, true); err != nil {
		s.logger.Error("cancel subscription", "tenant_id", p.TenantID, "error", err)
		return errProviderDown
	}
	return s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		if err := s.repo.SetCancelAtPeriodEnd(ctx, p.TenantID, true); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionSubscriptionCancel, EntityType: "subscription", EntityID: p.TenantID})
	})
}

// ---- notices -------------------------------------------------------------------------------------

// notice gathers who to email about a tenant, inside the caller's platform transaction.
func (s *Service) notice(ctx context.Context, tenantID, planID uuid.UUID) (Notice, error) {
	t, err := s.tenants.Get(ctx, tenantID)
	if err != nil || t == nil {
		return Notice{}, err
	}
	admins, err := s.tenants.Admins(ctx, tenantID)
	if err != nil {
		return Notice{}, err
	}
	n := Notice{TenantName: t.Name}
	for _, a := range admins {
		if a.Status == "active" {
			n.Admins = append(n.Admins, Admin{Email: a.Email, Name: a.FullName})
		}
	}
	if plan, err := s.repo.GetPlan(ctx, planID); err == nil && plan != nil {
		n.PlanName = plan.Name
	}
	return n, nil
}

// CheckUsage emails the tenant's admins the first time a cap reaches 80% and 100% in a period.
// It runs in the background after calls end and recordings finish; failures are only logged.
func (s *Service) CheckUsage(tenantID uuid.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.checkUsage(ctx, tenantID); err != nil {
			s.logger.Warn("check usage", "tenant_id", tenantID, "error", err)
		}
	}()
}

type usageAlert struct {
	cap     string
	percent int
}

func (s *Service) checkUsage(ctx context.Context, tenantID uuid.UUID) error {
	var alerts []usageAlert
	var notice Notice
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		snap, err := s.usage.Snapshot(ctx, tenantID)
		if err != nil {
			return err
		}
		caps := []struct {
			name        string
			used, limit int64
		}{}
		if snap.Caps.MaxCallMinutes != nil {
			caps = append(caps, struct {
				name        string
				used, limit int64
			}{"minutes", snap.Usage.CallSeconds, int64(*snap.Caps.MaxCallMinutes) * limits.SecondsPerMinute})
		}
		if snap.Caps.MaxStorageBytes != nil {
			caps = append(caps, struct {
				name        string
				used, limit int64
			}{"storage", snap.Usage.StorageBytes, *snap.Caps.MaxStorageBytes})
		}
		for _, c := range caps {
			if c.limit <= 0 {
				continue
			}
			percent := int(c.used * 100 / c.limit)
			highest := 0
			for _, threshold := range []int{80, 100} {
				if percent < threshold {
					continue
				}
				inserted, err := s.repo.InsertNotice(ctx, tenantID, snap.PeriodStart, c.name, threshold)
				if err != nil {
					return err
				}
				if inserted {
					highest = threshold
				}
			}
			if highest > 0 {
				alerts = append(alerts, usageAlert{cap: c.name, percent: highest})
			}
		}
		if len(alerts) == 0 {
			return nil
		}
		notice, err = s.notice(ctx, tenantID, snap.PlanID)
		return err
	})
	if err != nil {
		return err
	}
	for _, a := range alerts {
		s.notifier.UsageThreshold(ctx, notice, a.cap, a.percent)
	}
	return nil
}

var errUnexpected = errors.New("billing: unexpected state")

func unexpected(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUnexpected, fmt.Sprintf(format, args...))
}
