package billing

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"callcenter/internal/audit"
)

// postCommit is work that must happen only after the transaction that decided it has committed
// (talking to a provider, sending email).
type postCommit func(ctx context.Context)

// HandleWebhook verifies a provider webhook and applies it. Every verified event is stored once
// (replays are no-ops); handlers only move state forward through conditional updates.
func (s *Service) HandleWebhook(ctx context.Context, name ProviderName, r *http.Request) error {
	provider, err := s.providers.Get(name)
	if err != nil {
		return err
	}
	ev, err := provider.ParseWebhook(r)
	if err != nil {
		return err
	}
	if ev.ID == "" {
		return unexpected("%s event without an id", name)
	}
	ev.Provider = name

	var after []postCommit
	err = s.tx.InPlatform(ctx, func(ctx context.Context) error {
		inserted, err := s.repo.InsertWebhookEvent(ctx, name, ev.ID, string(ev.Type), ev.Raw)
		if err != nil || !inserted {
			return err // a replay is acknowledged without doing anything
		}
		if after, err = s.apply(ctx, ev); err != nil {
			return err
		}
		return s.repo.MarkWebhookProcessed(ctx, name, ev.ID)
	})
	if err != nil {
		return err
	}
	for _, f := range after {
		f(ctx)
	}
	return nil
}

func (s *Service) apply(ctx context.Context, ev Event) ([]postCommit, error) {
	switch ev.Type {
	case EventSubscriptionActivated:
		return s.activate(ctx, ev)
	case EventPaymentSucceeded:
		return nil, s.paymentSucceeded(ctx, ev)
	case EventPaymentFailed:
		return s.paymentFailed(ctx, ev)
	case EventSubscriptionCancelled:
		return s.providerCancelled(ctx, ev)
	case EventSubscriptionLinked:
		return nil, s.linkSubscription(ctx, ev)
	default:
		return nil, nil
	}
}

// subscriptionFor finds the subscription an event is about. nil means it is not ours (or not yet
// known) and the event is ignored.
func (s *Service) subscriptionFor(ctx context.Context, ev Event) (*Subscription, error) {
	switch {
	case ev.TenantID != uuid.Nil:
		return s.repo.GetSubscription(ctx, ev.TenantID)
	case ev.ExternalSubscriptionID != "":
		return s.repo.FindByExternal(ctx, ev.Provider, ev.ExternalSubscriptionID)
	case ev.ExternalCustomerID != "":
		return s.repo.FindByCustomer(ctx, ev.Provider, ev.ExternalCustomerID)
	}
	return nil, nil
}

func (s *Service) activate(ctx context.Context, ev Event) ([]postCommit, error) {
	if ev.TenantID == uuid.Nil || ev.PriceID == uuid.Nil {
		s.logger.Warn("activation without checkout metadata", "provider", ev.Provider, "event", ev.ID)
		return nil, nil
	}
	price, err := s.repo.GetPrice(ctx, ev.PriceID)
	if err != nil || price == nil {
		return nil, err
	}
	plan, err := s.repo.GetPlan(ctx, price.PlanID)
	if err != nil || plan == nil {
		return nil, err
	}
	sub, err := s.repo.GetSubscription(ctx, ev.TenantID)
	if err != nil || sub == nil {
		return nil, err
	}

	start := s.now().UTC()
	if ev.PeriodStart != nil {
		start = ev.PeriodStart.UTC()
	}
	end := addInterval(start, price.Interval)
	if ev.PeriodEnd != nil {
		end = ev.PeriodEnd.UTC()
	}
	if err := s.repo.Activate(ctx, Activation{
		TenantID: ev.TenantID, PlanID: plan.ID, PriceID: price.ID, Provider: ev.Provider,
		ExternalCustomerID: ev.ExternalCustomerID, ExternalSubscriptionID: ev.ExternalSubscriptionID,
		Authorization: ev.Authorization, BillingEmail: ev.CustomerEmail, PeriodStart: start, PeriodEnd: end,
	}); err != nil {
		return nil, err
	}
	if ev.PaymentReference != "" {
		if _, _, err := s.repo.InsertPayment(ctx, NewPayment{
			TenantID: ev.TenantID, SubscriptionID: sub.ID, Kind: "subscription", Provider: ev.Provider,
			Reference: ev.PaymentReference, AmountMinor: orAmount(ev.AmountMinor, price.AmountMinor),
			Currency: orCurrency(ev.Currency, price.Currency), Status: "succeeded",
		}); err != nil {
			return nil, err
		}
	}
	if err := s.audit.Record(ctx, audit.Entry{TenantID: ev.TenantID, Action: audit.ActionSubscriptionActive, EntityType: "subscription",
		EntityID: sub.ID, Metadata: map[string]any{"plan": plan.Code, "provider": ev.Provider}}); err != nil {
		return nil, err
	}

	// A new checkout replaces the old paid subscription: stop the old one so it cannot renew.
	var after []postCommit
	if sub.Provider != nil && sub.ExternalSubscriptionID != nil && *sub.ExternalSubscriptionID != ev.ExternalSubscriptionID {
		oldProvider, oldID := *sub.Provider, *sub.ExternalSubscriptionID
		after = append(after, func(ctx context.Context) { s.cancelAtProvider(ctx, oldProvider, oldID, false) })
	}
	return after, nil
}

// linkSubscription attaches the provider's subscription id to the tenant that owns the customer.
func (s *Service) linkSubscription(ctx context.Context, ev Event) error {
	if ev.ExternalCustomerID == "" || ev.ExternalSubscriptionID == "" {
		return nil
	}
	sub, err := s.repo.FindByCustomer(ctx, ev.Provider, ev.ExternalCustomerID)
	if err != nil || sub == nil {
		return err
	}
	return s.repo.LinkSubscription(ctx, sub.TenantID, ev.ExternalSubscriptionID)
}

func (s *Service) paymentSucceeded(ctx context.Context, ev Event) error {
	sub, err := s.subscriptionFor(ctx, ev)
	if err != nil || sub == nil {
		return err
	}
	if ev.PaymentReference != "" {
		price, err := s.currentPrice(ctx, sub)
		if err != nil {
			return err
		}
		if _, _, err := s.repo.InsertPayment(ctx, NewPayment{
			TenantID: sub.TenantID, SubscriptionID: sub.ID, Kind: "subscription", Provider: providerOf(sub, ev),
			Reference: ev.PaymentReference, AmountMinor: orAmount(ev.AmountMinor, priceAmount(price)),
			Currency: orCurrency(ev.Currency, priceCurrency(price)), Status: "succeeded",
		}); err != nil {
			return err
		}
	}
	// Paying within the grace period restores the subscription (D18).
	return s.repo.ClearPastDue(ctx, sub.TenantID)
}

func (s *Service) paymentFailed(ctx context.Context, ev Event) ([]postCommit, error) {
	sub, err := s.subscriptionFor(ctx, ev)
	if err != nil || sub == nil {
		return nil, err
	}
	price, err := s.currentPrice(ctx, sub)
	if err != nil {
		return nil, err
	}
	reference := ev.PaymentReference
	if reference == "" {
		reference = "failed_" + ev.ID
	}
	if _, _, err := s.repo.InsertPayment(ctx, NewPayment{
		TenantID: sub.TenantID, SubscriptionID: sub.ID, Kind: "subscription", Provider: providerOf(sub, ev), Reference: reference,
		AmountMinor: orAmount(ev.AmountMinor, priceAmount(price)), Currency: orCurrency(ev.Currency, priceCurrency(price)),
		Status: "failed", Failure: ev.Failure,
	}); err != nil {
		return nil, err
	}
	return s.enterPastDue(ctx, sub)
}

// enterPastDue starts the 14-day grace period (once) and emails the admins.
func (s *Service) enterPastDue(ctx context.Context, sub *Subscription) ([]postCommit, error) {
	changed, err := s.repo.SetPastDue(ctx, sub.TenantID)
	if err != nil || !changed {
		return nil, err
	}
	notice, err := s.notice(ctx, sub.TenantID, sub.PlanID)
	if err != nil {
		return nil, err
	}
	return []postCommit{func(ctx context.Context) { s.notifier.PaymentFailed(ctx, notice) }}, nil
}

// providerCancelled handles the provider ending the subscription itself (for example after the
// provider's own retries failed): the tenant returns to the free plan.
func (s *Service) providerCancelled(ctx context.Context, ev Event) ([]postCommit, error) {
	sub, err := s.subscriptionFor(ctx, ev)
	if err != nil || sub == nil {
		return nil, err
	}
	// Only the subscription we currently track counts; a replaced one ending is expected. One we
	// cancelled at the period end keeps its plan until the period closes.
	if sub.ExternalSubscriptionID == nil || *sub.ExternalSubscriptionID != ev.ExternalSubscriptionID || sub.CancelAtPeriodEnd {
		return nil, nil
	}
	return s.fallBackToFree(ctx, sub.TenantID, sub.PlanID, false)
}

// fallBackToFree moves a tenant to the free plan: nothing is deleted, both cap policies return to
// "block" (D16, D18) and the admins are told. When stopProvider is set the provider subscription
// is cancelled right away.
func (s *Service) fallBackToFree(ctx context.Context, tenantID, fromPlanID uuid.UUID, stopProvider bool) ([]postCommit, error) {
	sub, err := s.repo.GetSubscription(ctx, tenantID)
	if err != nil || sub == nil {
		return nil, err
	}
	free, err := s.repo.FreePlan(ctx)
	if err != nil {
		return nil, err
	}
	start, end := monthOf(s.now())
	if err := s.repo.FallBackToFree(ctx, tenantID, free.ID, start, end); err != nil {
		return nil, err
	}
	if err := s.repo.ResetPolicies(ctx, tenantID); err != nil {
		return nil, err
	}
	if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionSubscriptionFallback, EntityType: "subscription", EntityID: sub.ID}); err != nil {
		return nil, err
	}
	notice, err := s.notice(ctx, tenantID, fromPlanID)
	if err != nil {
		return nil, err
	}
	after := []postCommit{func(ctx context.Context) { s.notifier.FellBackToFree(ctx, notice) }}
	if stopProvider && sub.Provider != nil && sub.ExternalSubscriptionID != nil {
		provider, id := *sub.Provider, *sub.ExternalSubscriptionID
		after = append(after, func(ctx context.Context) { s.cancelAtProvider(ctx, provider, id, false) })
	}
	return after, nil
}

// cancelAtProvider is best effort: the local state is already decided and a failure is logged.
func (s *Service) cancelAtProvider(ctx context.Context, name ProviderName, externalID string, atPeriodEnd bool) {
	client, err := s.providers.Get(name)
	if err == nil {
		err = client.CancelSubscription(ctx, externalID, atPeriodEnd)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn("cancel provider subscription", "provider", name, "error", err)
	}
}

func (s *Service) currentPrice(ctx context.Context, sub *Subscription) (*Price, error) {
	if sub.PriceID == nil {
		return nil, nil
	}
	return s.repo.GetPrice(ctx, *sub.PriceID)
}

func providerOf(sub *Subscription, ev Event) ProviderName {
	if sub.Provider != nil {
		return *sub.Provider
	}
	return ev.Provider
}

func orAmount(n, fallback int64) int64 {
	if n > 0 {
		return n
	}
	return fallback
}

func orCurrency(c, fallback string) string {
	if c != "" {
		return c
	}
	return fallback
}

func priceAmount(p *Price) int64 {
	if p == nil {
		return 0
	}
	return p.AmountMinor
}

func priceCurrency(p *Price) string {
	if p == nil {
		return "USD"
	}
	return p.Currency
}
