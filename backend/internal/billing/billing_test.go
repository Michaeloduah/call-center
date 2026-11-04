package billing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/billing"
	"callcenter/internal/billing/fake"
	"callcenter/internal/limits"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/tenant"
)

const secret = "test-secret"

// Stored webhook events are never deleted, so every event id must be unique, across tests and runs.
func eid(name string) string { return name + "_" + uuid.NewString()[:8] }

type fakeNotifier struct {
	mu     sync.Mutex
	failed []billing.Notice
	fell   []billing.Notice
	usage  []string
}

func (n *fakeNotifier) PaymentFailed(_ context.Context, notice billing.Notice) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failed = append(n.failed, notice)
}
func (n *fakeNotifier) FellBackToFree(_ context.Context, notice billing.Notice) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.fell = append(n.fell, notice)
}
func (n *fakeNotifier) UsageThreshold(_ context.Context, _ billing.Notice, cap string, percent int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.usage = append(n.usage, fmt.Sprintf("%s:%d", cap, percent))
}
func (n *fakeNotifier) usageEvents() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.usage)
}

type env struct {
	*dbtest.Fixture
	svc      *billing.Service
	provider *fake.Provider
	notifier *fakeNotifier
	tenant   uuid.UUID
	admin    uuid.UUID
}

func setup(t *testing.T) *env {
	f := dbtest.New(t)
	provider := fake.New(secret, "http://web.test/fake-checkout")
	n := &fakeNotifier{}
	svc := billing.NewService(f.DB, billing.NewRepository(), limits.NewService(limits.NewRepository()),
		billing.NewRegistry(true, []string{"NGN"}, provider), tenant.NewRepository(), n, audit.NewRecorder(),
		billing.Config{ReturnURL: "http://web.test/admin/billing"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tenantID := f.Tenant()
	admin := f.User(tenantID, "tenant_admin")
	return &env{Fixture: f, svc: svc, provider: provider, notifier: n, tenant: tenantID, admin: admin}
}

func (e *env) adminCtx() context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: e.admin, TenantID: e.tenant, Role: reqctx.RoleTenantAdmin, Email: "admin@example.test", FullName: "Ada Admin"})
}

func (e *env) platformCtx() context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: e.PlatformAdmin(), Role: reqctx.RolePlatformAdmin, Email: "p@example.test"})
}

// newPlan creates a paid plan with one USD price and removes it (and the tenants on it) afterwards.
func (e *env) newPlan(caps limits.Caps, perMinute, perGB int64) (billing.Plan, billing.Price) {
	e.T.Helper()
	ctx := e.platformCtx()
	plan, err := e.svc.CreatePlan(ctx, billing.PlanBody{Code: "t-" + uuid.NewString()[:8], Name: "Pro", IsPublic: true, IsActive: true, Caps: caps})
	if err != nil {
		e.T.Fatalf("create plan: %v", err)
	}
	plan, err = e.svc.CreatePrice(ctx, plan.ID, billing.PriceBody{Currency: "USD", Interval: billing.IntervalMonthly, AmountMinor: 4900,
		OveragePerMinuteMinor: perMinute, OveragePerGBMinor: perGB, IsActive: true})
	if err != nil || len(plan.Prices) != 1 {
		e.T.Fatalf("create price: %v", err)
	}
	e.T.Cleanup(func() {
		_ = e.DB.InPlatform(context.Background(), func(ctx context.Context) error {
			q, _ := database.Conn(ctx)
			if _, err := q.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, e.tenant); err != nil {
				return err
			}
			if _, err := q.Exec(ctx, `DELETE FROM plan_prices WHERE plan_id = $1`, plan.ID); err != nil {
				return err
			}
			_, err := q.Exec(ctx, `DELETE FROM plans WHERE id = $1`, plan.ID)
			return err
		})
	})
	return *plan, plan.Prices[0]
}

func (e *env) webhook(p fake.Payload) error {
	e.T.Helper()
	body, _ := json.Marshal(p)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/fake", bytes.NewReader(body))
	req.Header.Set(fake.SignatureHeader, fake.Sign(secret, body))
	return e.svc.HandleWebhook(e.Ctx, billing.ProviderFake, req)
}

// subscribe runs a checkout to completion: the webhook that activates the price for the tenant.
func (e *env) subscribe(price billing.Price, externalSub string) {
	e.T.Helper()
	err := e.webhook(fake.Payload{ID: "evt_" + uuid.NewString()[:8], Type: string(billing.EventSubscriptionActivated), TenantID: e.tenant,
		PlanPriceID: price.ID, ExternalCustomerID: "cus_" + e.tenant.String()[:8], ExternalSubscriptionID: externalSub,
		Authorization: "auth_1", PaymentReference: "pay_" + uuid.NewString()[:8], AmountMinor: price.AmountMinor, Currency: price.Currency})
	if err != nil {
		e.T.Fatalf("activate subscription: %v", err)
	}
}

type subState struct {
	Plan      uuid.UUID
	Status    string
	PastDue   *time.Time
	Cancel    bool
	Provider  *string
	ExtSub    *string
	PeriodEnd time.Time
}

func (e *env) sub() (s subState) {
	e.T.Helper()
	e.Query(`SELECT plan_id, status, past_due_since, cancel_at_period_end, provider, external_subscription_id, current_period_end
	         FROM subscriptions WHERE tenant_id = $1`, []any{e.tenant}, &s.Plan, &s.Status, &s.PastDue, &s.Cancel, &s.Provider, &s.ExtSub, &s.PeriodEnd)
	return s
}

func (e *env) count(sql string, args ...any) (n int) {
	e.T.Helper()
	e.Query(sql, args, &n)
	return n
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}

func intp(n int) *int { return &n }

// ---- overage maths -------------------------------------------------------------------------------

func TestComputeOverage(t *testing.T) {
	caps := limits.Caps{MaxCallMinutes: intp(100), MaxStorageBytes: func() *int64 { n := 2 * billing.BytesPerGB; return &n }()}
	price := billing.Price{OveragePerMinuteMinor: 50, OveragePerGBMinor: 1000}
	over := limits.Policies{Minutes: limits.PolicyOverage, Storage: limits.PolicyOverage}
	block := limits.Policies{Minutes: limits.PolicyBlock, Storage: limits.PolicyBlock}

	tests := map[string]struct {
		seconds, bytes int64
		pol            limits.Policies
		caps           limits.Caps
		want           billing.Overage
	}{
		"under both caps":              {5999, 2 * billing.BytesPerGB, over, caps, billing.Overage{}},
		"one second over rounds up":    {6001, 0, over, caps, billing.Overage{Minutes: 1, Amount: 50}},
		"90 seconds over is 2 minutes": {6090, 0, over, caps, billing.Overage{Minutes: 2, Amount: 100}},
		"one byte over is a whole GB":  {0, 2*billing.BytesPerGB + 1, over, caps, billing.Overage{GB: 1, Amount: 1000}},
		"both":                         {6120, 3 * billing.BytesPerGB, over, caps, billing.Overage{Minutes: 2, GB: 1, Amount: 1100}},
		"block policy bills nothing":   {99999, 9 * billing.BytesPerGB, block, caps, billing.Overage{}},
		"unlimited bills nothing":      {99999, 9 * billing.BytesPerGB, over, limits.Caps{}, billing.Overage{}},
	}
	for name, tc := range tests {
		if got := billing.ComputeOverage(tc.seconds, tc.bytes, tc.caps, tc.pol, price); got != tc.want {
			t.Errorf("%s: %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestProviderIsChosenByCurrency(t *testing.T) {
	r := billing.NewRegistry(false, []string{"NGN"})
	if r.ProviderFor("NGN") != billing.ProviderPaystack || r.ProviderFor("USD") != billing.ProviderStripe || r.ProviderFor("GHS") != billing.ProviderStripe {
		t.Fatal("NGN must go to Paystack and every other currency to Stripe (D19)")
	}
	if billing.NewRegistry(true, []string{"NGN"}).ProviderFor("NGN") != billing.ProviderFake {
		t.Fatal("fake mode must route every currency to the fake provider")
	}
}

// ---- plans ---------------------------------------------------------------------------------------

func TestPlanRules(t *testing.T) {
	e := setup(t)
	plan, price := e.newPlan(limits.Caps{}, 0, 0)
	ctx := e.platformCtx()

	free, err := e.svc.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var freeID uuid.UUID
	for _, p := range free {
		if p.IsFree {
			freeID = p.ID
		}
	}
	if freeID == uuid.Nil {
		t.Fatal("there is no free plan")
	}
	wantCode(t, e.svc.DeletePlan(ctx, freeID), "free_plan")
	_, err = e.svc.CreatePrice(ctx, freeID, billing.PriceBody{Currency: "USD", Interval: billing.IntervalMonthly, AmountMinor: 100})
	wantCode(t, err, "free_plan")

	// One price per currency and interval.
	_, err = e.svc.CreatePrice(ctx, plan.ID, billing.PriceBody{Currency: "USD", Interval: billing.IntervalMonthly, AmountMinor: 100})
	wantCode(t, err, "conflict")
	// A second plan cannot reuse a code.
	_, err = e.svc.CreatePlan(ctx, billing.PlanBody{Code: plan.Code, Name: "Dup", IsActive: true})
	wantCode(t, err, "conflict")

	// A subscribed plan and price cannot be deleted.
	e.subscribe(price, "sub_1")
	wantCode(t, e.svc.DeletePlan(ctx, plan.ID), "plan_in_use")
	wantCode(t, e.svc.DeletePrice(ctx, price.ID), "price_in_use")
	if err := e.svc.UpdatePrice(ctx, price.ID, billing.PriceUpdateBody{IsActive: false, OveragePerMinuteMinor: 5}); err != nil {
		t.Fatalf("deactivating a subscribed price: %v", err)
	}
}

func TestPlanBodyValidation(t *testing.T) {
	bad := map[string]billing.PlanBody{
		"code":     {Code: "Bad Code!", Name: "X"},
		"name":     {Code: "ok", Name: ""},
		"negative": {Code: "ok", Name: "X", Caps: limits.Caps{MaxAgents: intp(-1)}},
	}
	for name, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("%s: invalid plan accepted", name)
		}
	}
	good := billing.PlanBody{Code: "Pro-1", Name: "Pro"}
	if err := good.Validate(); err != nil || good.Code != "pro-1" {
		t.Errorf("valid plan: %q %v", good.Code, err)
	}
	if err := (&billing.PriceBody{Currency: "usd", Interval: billing.IntervalYearly, AmountMinor: 1}).Validate(); err != nil {
		t.Errorf("valid price: %v", err)
	}
	for _, p := range []billing.PriceBody{{Currency: "US", Interval: billing.IntervalMonthly, AmountMinor: 1},
		{Currency: "USD", Interval: "weekly", AmountMinor: 1}, {Currency: "USD", Interval: billing.IntervalMonthly, AmountMinor: 0}} {
		if err := p.Validate(); err == nil {
			t.Errorf("invalid price accepted: %+v", p)
		}
	}
}

// ---- checkout and webhooks -----------------------------------------------------------------------

func TestCheckoutThenWebhookActivatesThePlan(t *testing.T) {
	e := setup(t)
	plan, price := e.newPlan(limits.Caps{MaxAgents: intp(20), MaxCallMinutes: intp(5000)}, 5, 100)

	res, err := e.svc.Checkout(e.adminCtx(), billing.CheckoutBody{PriceID: price.ID})
	if err != nil || res.RedirectURL == "" {
		t.Fatalf("checkout = %+v, %v", res, err)
	}
	if e.sub().Plan == plan.ID {
		t.Fatal("the subscription changed before any webhook arrived")
	}

	e.subscribe(price, "sub_1")
	s := e.sub()
	if s.Plan != plan.ID || s.Status != "active" || s.ExtSub == nil || *s.ExtSub != "sub_1" {
		t.Fatalf("subscription after webhook = %+v", s)
	}
	if n := e.count(`SELECT count(*)::int FROM payments WHERE tenant_id = $1 AND status = 'succeeded'`, e.tenant); n != 1 {
		t.Fatalf("%d payments recorded, want 1", n)
	}
	ov, err := e.svc.Overview(e.adminCtx())
	if err != nil || *ov.Limits.MaxAgents != 20 || ov.Plan.ID != plan.ID {
		t.Fatalf("overview limits = %+v, %v", ov.Limits, err)
	}
	if n := e.count(`SELECT count(*)::int FROM audit_logs WHERE tenant_id = $1 AND action IN ('billing.checkout_started', 'subscription.activated')`, e.tenant); n != 2 {
		t.Fatalf("%d billing audit entries, want 2", n)
	}

	// Choosing the plan you are on is refused; free prices do not exist.
	_, err = e.svc.Checkout(e.adminCtx(), billing.CheckoutBody{PriceID: price.ID})
	wantCode(t, err, "already_subscribed")
	_, err = e.svc.Checkout(e.adminCtx(), billing.CheckoutBody{PriceID: uuid.New()})
	wantCode(t, err, "not_found")
}

func TestWebhooksAreVerifiedAndIdempotent(t *testing.T) {
	e := setup(t)
	_, price := e.newPlan(limits.Caps{}, 0, 0)

	body, _ := json.Marshal(fake.Payload{ID: eid("evt_x"), Type: string(billing.EventSubscriptionActivated), TenantID: e.tenant, PlanPriceID: price.ID, ExternalSubscriptionID: "sub_x"})
	unsigned := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	if err := e.svc.HandleWebhook(e.Ctx, billing.ProviderFake, unsigned); !errors.Is(err, billing.ErrInvalidWebhook) {
		t.Fatalf("unsigned webhook error = %v, want ErrInvalidWebhook", err)
	}
	tampered := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(append(body, ' ')))
	tampered.Header.Set(fake.SignatureHeader, fake.Sign(secret, body))
	if err := e.svc.HandleWebhook(e.Ctx, billing.ProviderFake, tampered); !errors.Is(err, billing.ErrInvalidWebhook) {
		t.Fatalf("tampered webhook error = %v, want ErrInvalidWebhook", err)
	}
	if e.sub().ExtSub != nil {
		t.Fatal("a rejected webhook changed the subscription")
	}

	dupID := eid("evt_dup")
	p := fake.Payload{ID: dupID, Type: string(billing.EventPaymentSucceeded), TenantID: e.tenant, ExternalSubscriptionID: "sub_x", PaymentReference: "pay_dup", AmountMinor: 4900, Currency: "USD"}
	e.subscribe(price, "sub_x")
	for range 3 { // provider retries
		if err := e.webhook(p); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.count(`SELECT count(*)::int FROM payments WHERE external_reference = 'pay_dup'`); n != 1 {
		t.Fatalf("replayed webhook recorded %d payments, want 1", n)
	}
	if n := e.count(`SELECT count(*)::int FROM billing_webhook_events WHERE external_event_id = $1`, dupID); n != 1 {
		t.Fatalf("%d stored events, want 1", n)
	}
	// Events for subscriptions we do not know are stored and ignored.
	if err := e.webhook(fake.Payload{ID: eid("evt_stranger"), Type: string(billing.EventPaymentFailed), ExternalSubscriptionID: "sub_unknown"}); err != nil {
		t.Fatalf("event for an unknown subscription: %v", err)
	}
}

func TestSwitchingPlansStopsTheOldSubscription(t *testing.T) {
	e := setup(t)
	_, first := e.newPlan(limits.Caps{}, 0, 0)
	e.subscribe(first, "sub_old")
	_, second := e.newPlan(limits.Caps{}, 0, 0)
	e.subscribe(second, "sub_new")

	if s := e.sub(); s.ExtSub == nil || *s.ExtSub != "sub_new" {
		t.Fatalf("subscription = %+v, want the new one", s)
	}
	if got := e.provider.Cancelled(); len(got) != 1 || got[0] != "sub_old" {
		t.Fatalf("cancelled at the provider: %v, want the replaced subscription", got)
	}
}

// ---- failure and fallback ------------------------------------------------------------------------

func TestFailedPaymentStartsTheGraceAndPayingEndsIt(t *testing.T) {
	e := setup(t)
	_, price := e.newPlan(limits.Caps{}, 0, 0)
	e.subscribe(price, "sub_1")

	if err := e.webhook(fake.Payload{ID: eid("evt_fail"), Type: string(billing.EventPaymentFailed), ExternalSubscriptionID: "sub_1", PaymentReference: "pay_fail", Failure: "card declined"}); err != nil {
		t.Fatal(err)
	}
	s := e.sub()
	if s.Status != "past_due" || s.PastDue == nil || len(e.notifier.failed) != 1 {
		t.Fatalf("after failure: %+v, %d emails", s, len(e.notifier.failed))
	}
	// A second failure does not restart the clock or send another email.
	first := *s.PastDue
	if err := e.webhook(fake.Payload{ID: eid("evt_fail2"), Type: string(billing.EventPaymentFailed), ExternalSubscriptionID: "sub_1", PaymentReference: "pay_fail2"}); err != nil {
		t.Fatal(err)
	}
	if again := e.sub(); !again.PastDue.Equal(first) || len(e.notifier.failed) != 1 {
		t.Fatal("a repeated failure restarted the grace period or emailed twice")
	}

	if err := e.webhook(fake.Payload{ID: eid("evt_ok"), Type: string(billing.EventPaymentSucceeded), ExternalSubscriptionID: "sub_1", PaymentReference: "pay_ok"}); err != nil {
		t.Fatal(err)
	}
	if s := e.sub(); s.Status != "active" || s.PastDue != nil {
		t.Fatalf("after paying: %+v, want active again", s)
	}
}

func TestFourteenDaysPastDueFallsBackToTheFreePlan(t *testing.T) {
	e := setup(t)
	plan, price := e.newPlan(limits.Caps{}, 5, 100)
	e.subscribe(price, "sub_1")
	e.Exec(`UPDATE tenant_settings SET minutes_cap_policy = 'overage', storage_cap_policy = 'overage' WHERE tenant_id = $1`, e.tenant)
	if err := e.webhook(fake.Payload{ID: eid("evt_fail"), Type: string(billing.EventPaymentFailed), ExternalSubscriptionID: "sub_1"}); err != nil {
		t.Fatal(err)
	}

	e.Exec(`UPDATE subscriptions SET past_due_since = now() - interval '13 days' WHERE tenant_id = $1`, e.tenant)
	if err := e.svc.RunOnce(e.Ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.sub(); s.Plan != plan.ID || s.Status != "past_due" {
		t.Fatalf("after 13 days: %+v, want still on the paid plan in grace", s)
	}

	e.Exec(`UPDATE subscriptions SET past_due_since = now() - interval '14 days 1 minute' WHERE tenant_id = $1`, e.tenant)
	if err := e.svc.RunOnce(e.Ctx); err != nil {
		t.Fatal(err)
	}
	s := e.sub()
	if s.Plan == plan.ID || s.Status != "active" || s.PastDue != nil || s.ExtSub != nil {
		t.Fatalf("after 14 days: %+v, want the free plan", s)
	}
	var minutes, storage string
	e.Query(`SELECT minutes_cap_policy, storage_cap_policy FROM tenant_settings WHERE tenant_id = $1`, []any{e.tenant}, &minutes, &storage)
	if minutes != "block" || storage != "block" {
		t.Fatalf("policies after fallback = %s/%s, want block/block (D16)", minutes, storage)
	}
	if len(e.notifier.fell) != 1 {
		t.Fatalf("%d fallback emails, want 1", len(e.notifier.fell))
	}
	if got := e.provider.Cancelled(); len(got) != 1 || got[0] != "sub_1" {
		t.Fatalf("provider subscriptions cancelled: %v", got)
	}
	// Nothing was deleted: the tenant's data is intact.
	if n := e.count(`SELECT count(*)::int FROM tenants WHERE id = $1`, e.tenant); n != 1 {
		t.Fatal("the tenant disappeared")
	}
}

func TestProviderCancellingEndsTheSubscription(t *testing.T) {
	e := setup(t)
	plan, price := e.newPlan(limits.Caps{}, 0, 0)
	e.subscribe(price, "sub_1")
	// A replaced subscription ending is expected and changes nothing.
	if err := e.webhook(fake.Payload{ID: eid("evt_stale"), Type: string(billing.EventSubscriptionCancelled), TenantID: e.tenant, ExternalSubscriptionID: "sub_other"}); err != nil {
		t.Fatal(err)
	}
	if e.sub().Plan != plan.ID {
		t.Fatal("an unrelated cancellation moved the tenant to the free plan")
	}
	if err := e.webhook(fake.Payload{ID: eid("evt_end"), Type: string(billing.EventSubscriptionCancelled), TenantID: e.tenant, ExternalSubscriptionID: "sub_1"}); err != nil {
		t.Fatal(err)
	}
	if e.sub().Plan == plan.ID {
		t.Fatal("the subscription the provider cancelled is still active")
	}
}

// ---- cancel, policies, period close --------------------------------------------------------------

func TestCancelKeepsThePlanUntilThePeriodEnds(t *testing.T) {
	e := setup(t)
	plan, price := e.newPlan(limits.Caps{}, 0, 0)
	wantCode(t, e.svc.Cancel(e.adminCtx()), "nothing_to_cancel") // free plan

	e.subscribe(price, "sub_1")
	if err := e.svc.Cancel(e.adminCtx()); err != nil {
		t.Fatal(err)
	}
	if s := e.sub(); s.Plan != plan.ID || !s.Cancel {
		t.Fatalf("after cancel: %+v, want paid plan with cancel_at_period_end", s)
	}
	e.Exec(`UPDATE subscriptions SET current_period_start = now() - interval '31 days', current_period_end = now() - interval '1 minute' WHERE tenant_id = $1`, e.tenant)
	if err := e.svc.RunOnce(e.Ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.sub(); s.Plan == plan.ID || s.Cancel {
		t.Fatalf("after the period ended: %+v, want the free plan", s)
	}
	if n := e.count(`SELECT count(*)::int FROM usage_periods WHERE tenant_id = $1`, e.tenant); n != 1 {
		t.Fatalf("%d usage snapshots, want 1", n)
	}
}

func TestCapPoliciesNeedAPaidPlanWithARate(t *testing.T) {
	e := setup(t)
	overage := billing.PoliciesBody{Minutes: limits.PolicyOverage, Storage: limits.PolicyBlock}
	wantCode(t, e.svc.SetPolicies(e.adminCtx(), overage), "overage_needs_paid_plan") // free plan

	_, noRate := e.newPlan(limits.Caps{}, 0, 100)
	e.subscribe(noRate, "sub_1")
	wantCode(t, e.svc.SetPolicies(e.adminCtx(), overage), "no_overage_rate")
	if err := e.svc.SetPolicies(e.adminCtx(), billing.PoliciesBody{Minutes: limits.PolicyBlock, Storage: limits.PolicyOverage}); err != nil {
		t.Fatalf("storage overage has a rate and should be allowed: %v", err)
	}
}

func TestPeriodCloseChargesOverageOnce(t *testing.T) {
	e := setup(t)
	_, price := e.newPlan(limits.Caps{MaxCallMinutes: intp(1)}, 50, 0)
	dept := e.Department(e.tenant, "Support")
	agent := e.Agent(e.tenant, dept)
	e.subscribe(price, "sub_1")
	if err := e.svc.SetPolicies(e.adminCtx(), billing.PoliciesBody{Minutes: limits.PolicyOverage, Storage: limits.PolicyBlock}); err != nil {
		t.Fatal(err)
	}

	// A period that has just ended, with 150 seconds of calls (60 included, 90 over → 2 minutes).
	e.Exec(`UPDATE subscriptions SET current_period_start = now() - interval '31 days', current_period_end = now() - interval '1 minute' WHERE tenant_id = $1`, e.tenant)
	e.AnsweredCall(e.tenant, dept, agent, time.Now().Add(-10*24*time.Hour), 150)

	for range 2 { // a second sweep (another instance) must not charge again
		if err := e.svc.RunOnce(e.Ctx); err != nil {
			t.Fatal(err)
		}
	}
	charges := e.provider.Charges()
	if len(charges) != 1 || charges[0].AmountMinor != 100 || charges[0].Currency != "USD" {
		t.Fatalf("charges = %+v, want one of 100 USD minor units (2 minutes × 50)", charges)
	}
	var seconds int64
	var minutes int
	var amount int64
	var paid *uuid.UUID
	e.Query(`SELECT call_seconds, overage_minutes, overage_amount_minor, overage_payment_id FROM usage_periods WHERE tenant_id = $1`, []any{e.tenant}, &seconds, &minutes, &amount, &paid)
	if seconds != 150 || minutes != 2 || amount != 100 || paid == nil {
		t.Fatalf("usage period = %ds %d min %d minor paid=%v", seconds, minutes, amount, paid)
	}
	if n := e.count(`SELECT count(*)::int FROM payments WHERE tenant_id = $1 AND kind = 'overage' AND status = 'succeeded'`, e.tenant); n != 1 {
		t.Fatalf("%d overage payments, want 1", n)
	}
	if s := e.sub(); s.Status != "active" || !s.PeriodEnd.After(time.Now()) {
		t.Fatalf("subscription after close = %+v, want active in a new period", s)
	}
}

func TestFailedOverageChargeEntersTheGrace(t *testing.T) {
	e := setup(t)
	_, price := e.newPlan(limits.Caps{MaxCallMinutes: intp(1)}, 50, 0)
	dept := e.Department(e.tenant, "Support")
	agent := e.Agent(e.tenant, dept)
	e.subscribe(price, "sub_1")
	if err := e.svc.SetPolicies(e.adminCtx(), billing.PoliciesBody{Minutes: limits.PolicyOverage, Storage: limits.PolicyBlock}); err != nil {
		t.Fatal(err)
	}
	e.Exec(`UPDATE subscriptions SET current_period_start = now() - interval '31 days', current_period_end = now() - interval '1 minute' WHERE tenant_id = $1`, e.tenant)
	e.AnsweredCall(e.tenant, dept, agent, time.Now().Add(-5*24*time.Hour), 300)
	e.provider.FailNextCharge()

	if err := e.svc.RunOnce(e.Ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.sub(); s.Status != "past_due" || len(e.notifier.failed) != 1 {
		t.Fatalf("after a failed overage charge: %+v, %d emails", s, len(e.notifier.failed))
	}
	if n := e.count(`SELECT count(*)::int FROM payments WHERE tenant_id = $1 AND kind = 'overage' AND status = 'failed'`, e.tenant); n != 1 {
		t.Fatalf("%d failed overage payments, want 1", n)
	}
}

func TestNoOverageIsChargedWhenThePolicyBlocksOrTheTenantIsFree(t *testing.T) {
	e := setup(t)
	_, price := e.newPlan(limits.Caps{MaxCallMinutes: intp(1)}, 50, 0)
	dept := e.Department(e.tenant, "Support")
	agent := e.Agent(e.tenant, dept)
	e.subscribe(price, "sub_1") // policy stays "block"
	e.Exec(`UPDATE subscriptions SET current_period_start = now() - interval '31 days', current_period_end = now() - interval '1 minute' WHERE tenant_id = $1`, e.tenant)
	e.AnsweredCall(e.tenant, dept, agent, time.Now().Add(-5*24*time.Hour), 900)
	if err := e.svc.RunOnce(e.Ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.provider.Charges()) != 0 {
		t.Fatal("a tenant with policy block was charged for overage")
	}

	free := setup(t)
	free.Exec(`UPDATE subscriptions SET current_period_start = now() - interval '31 days', current_period_end = now() - interval '1 minute' WHERE tenant_id = $1`, free.tenant)
	if err := free.svc.RunOnce(free.Ctx); err != nil {
		t.Fatal(err)
	}
	if s := free.sub(); !s.PeriodEnd.After(time.Now()) {
		t.Fatalf("free tier did not roll to the next month: %+v", s)
	}
	if len(free.provider.Charges()) != 0 {
		t.Fatal("a free tenant was charged")
	}
}

// ---- usage emails --------------------------------------------------------------------------------

func TestUsageWarningsGoOutOncePerThreshold(t *testing.T) {
	e := setup(t)
	e.PlanFor(e.tenant, dbtest.Caps{Minutes: dbtest.Int(100)})
	dept := e.Department(e.tenant, "Support")
	agent := e.Agent(e.tenant, dept)
	waitFor := func(want int) []string {
		deadline := time.Now().Add(3 * time.Second)
		for {
			if got := e.notifier.usageEvents(); len(got) >= want || time.Now().After(deadline) {
				return got
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	e.svc.CheckUsage(e.tenant)
	time.Sleep(200 * time.Millisecond)
	if got := e.notifier.usageEvents(); len(got) != 0 {
		t.Fatalf("warned with no usage: %v", got)
	}

	e.AnsweredCall(e.tenant, dept, agent, time.Now().Add(-time.Hour), 85*60)
	e.svc.CheckUsage(e.tenant)
	if got := waitFor(1); len(got) != 1 || got[0] != "minutes:80" {
		t.Fatalf("at 85%% got %v, want one 80%% warning", got)
	}
	e.svc.CheckUsage(e.tenant)
	time.Sleep(200 * time.Millisecond)
	if got := e.notifier.usageEvents(); len(got) != 1 {
		t.Fatalf("the same warning was sent again: %v", got)
	}

	e.AnsweredCall(e.tenant, dept, agent, time.Now().Add(-30*time.Minute), 20*60)
	e.svc.CheckUsage(e.tenant)
	if got := waitFor(2); len(got) != 2 || got[1] != "minutes:100" {
		t.Fatalf("at 105%% got %v, want the 100%% warning next", got)
	}
}

// ---- isolation -----------------------------------------------------------------------------------

func TestBillingTablesAreTenantIsolatedAndPlansAreReadOnlyForTenants(t *testing.T) {
	e := setup(t)
	other := e.Tenant()
	e.Exec(`INSERT INTO payments (tenant_id, kind, provider, external_reference, amount_minor, currency, status) VALUES ($1, 'subscription', 'fake', 'p1', 1, 'USD', 'succeeded')`, other)

	err := e.DB.InTenant(e.Ctx, e.tenant, func(ctx context.Context) error {
		q, _ := database.Conn(ctx)
		for _, table := range []string{"subscriptions", "payments", "usage_periods", "usage_notices"} {
			var n int
			if err := q.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE tenant_id <> $1`, e.tenant).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("tenant sees %d foreign rows in %s", n, table)
			}
		}
		var plans int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM plans`).Scan(&plans); err != nil || plans == 0 {
			t.Errorf("a tenant cannot read plans: %d %v", plans, err)
		}
		var events int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM billing_webhook_events`).Scan(&events); err != nil || events != 0 {
			t.Errorf("a tenant can see webhook events: %d %v", events, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A tenant cannot edit plans: the write is rejected by row-level security.
	err = e.DB.InTenant(e.Ctx, e.tenant, func(ctx context.Context) error {
		q, _ := database.Conn(ctx)
		_, err := q.Exec(ctx, `INSERT INTO plans (code, name) VALUES ('sneaky', 'Sneaky')`)
		return err
	})
	if err == nil {
		t.Fatal("a tenant created a plan")
	}
	err = e.DB.InTenant(e.Ctx, e.tenant, func(ctx context.Context) error {
		q, _ := database.Conn(ctx)
		tag, err := q.Exec(ctx, `UPDATE plans SET max_agents = 9999`)
		if err == nil && tag.RowsAffected() != 0 {
			t.Errorf("a tenant changed %d plans", tag.RowsAffected())
		}
		return err
	})
	_ = err
}
