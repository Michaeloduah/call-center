package billing

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// sweepInterval is how often due billing periods and expired grace periods are processed.
const sweepInterval = time.Minute

// Run processes billing work until ctx ends. Every step is safe to repeat and safe to run on
// several API instances at once (they claim work through unique rows and the tenant lock).
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
				s.logger.Error("billing sweep", "error", err)
			}
		}
	}
}

// RunOnce closes every due billing period and moves expired past-due subscriptions to the free plan.
func (s *Service) RunOnce(ctx context.Context) error {
	now := s.now()
	var due, expired []uuid.UUID
	if err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		if due, err = s.repo.DueSubscriptions(ctx, now); err != nil {
			return err
		}
		expired, err = s.repo.PastDueSince(ctx, now.Add(-PastDueGrace))
		return err
	}); err != nil {
		return err
	}
	for _, id := range expired {
		if err := s.expirePastDue(ctx, id); err != nil {
			s.logger.Error("fall back to free plan", "tenant_id", id, "error", err)
		}
	}
	for _, id := range due {
		if err := s.closePeriod(ctx, id); err != nil {
			s.logger.Error("close billing period", "tenant_id", id, "error", err)
		}
	}
	return nil
}

// expirePastDue moves a tenant whose grace period ran out to the free plan (D18).
func (s *Service) expirePastDue(ctx context.Context, tenantID uuid.UUID) error {
	var after []postCommit
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		if err := s.repo.LockTenant(ctx, tenantID); err != nil {
			return err
		}
		sub, err := s.repo.GetSubscription(ctx, tenantID)
		if err != nil || sub == nil || sub.Status != StatusPastDue || sub.PastDueSince == nil ||
			sub.PastDueSince.After(s.now().Add(-PastDueGrace)) {
			return err // paid in the meantime
		}
		after, err = s.fallBackToFree(ctx, tenantID, sub.PlanID, true)
		return err
	})
	for _, f := range after {
		f(ctx)
	}
	return err
}

// pendingCharge is an overage to collect once the period's snapshot has been committed.
type pendingCharge struct {
	usageID uuid.UUID
	sub     Subscription
	price   Price
	amount  int64
	minutes int
	gb      int
	email   string
}

// closePeriod snapshots the finished period, rolls the subscription forward and, when the tenant
// chose to pay for overage, charges it once. The snapshot row is the claim: a second sweep finds
// it already there and charges nothing.
func (s *Service) closePeriod(ctx context.Context, tenantID uuid.UUID) error {
	var charge *pendingCharge
	var after []postCommit
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		if err := s.repo.LockTenant(ctx, tenantID); err != nil {
			return err
		}
		sub, err := s.repo.GetSubscription(ctx, tenantID)
		if err != nil || sub == nil || sub.PeriodEnd.After(s.now()) {
			return err // not due (or closed by another instance)
		}
		snap, err := s.usage.Snapshot(ctx, tenantID)
		if err != nil {
			return err
		}
		var price *Price
		var overage Overage
		if price, err = s.currentPrice(ctx, sub); err != nil {
			return err
		}
		currency := ""
		if price != nil {
			overage = ComputeOverage(snap.Usage.CallSeconds, snap.Usage.StorageBytes, snap.Caps, snap.Policies, *price)
			currency = price.Currency
		}
		usageID, inserted, err := s.repo.InsertUsagePeriod(ctx, UsagePeriod{
			TenantID: tenantID, SubscriptionID: sub.ID, PlanID: sub.PlanID, Start: sub.PeriodStart, End: sub.PeriodEnd,
			CallSeconds: snap.Usage.CallSeconds, StorageBytes: snap.Usage.StorageBytes,
			OverageMinutes: overage.Minutes, OverageGB: overage.GB, OverageAmount: overage.Amount, Currency: currency,
		})
		if err != nil {
			return err
		}

		switch {
		case sub.CancelAtPeriodEnd:
			// The tenant asked to stop: the paid period ran out, so back to the free plan.
			after, err = s.fallBackToFree(ctx, tenantID, sub.PlanID, false)
		case price == nil:
			start, end := monthOf(sub.PeriodEnd)
			err = s.repo.RollPeriod(ctx, tenantID, start, end)
		default:
			err = s.repo.RollPeriod(ctx, tenantID, sub.PeriodEnd, addInterval(sub.PeriodEnd, price.Interval))
		}
		if err != nil {
			return err
		}

		// Overage is only collected from a subscription in good standing, and only by the sweep that created the snapshot.
		if inserted && price != nil && overage.Amount > 0 && sub.Status == StatusActive && !sub.CancelAtPeriodEnd {
			charge = &pendingCharge{usageID: usageID, sub: *sub, price: *price, amount: overage.Amount, minutes: overage.Minutes, gb: overage.GB}
			if admins, err := s.tenants.Admins(ctx, tenantID); err == nil {
				for _, a := range admins {
					if a.Status == "active" {
						charge.email = a.Email
						break
					}
				}
			}
		}
		return nil
	})
	for _, f := range after {
		f(ctx)
	}
	if err != nil || charge == nil {
		return err
	}
	return s.collectOverage(ctx, *charge)
}

// collectOverage charges the overage through the provider and records the outcome. A failed
// charge starts the grace period like any failed payment.
func (s *Service) collectOverage(ctx context.Context, c pendingCharge) error {
	reference := "ovg_" + c.usageID.String()
	status, failure := "succeeded", ""
	client, err := s.providers.Get(*c.sub.Provider)
	if err == nil {
		var providerRef string
		providerRef, err = client.ChargeOverage(ctx, OverageCharge{
			Reference: reference, AmountMinor: c.amount, Currency: c.price.Currency,
			Description: overageDescription(c.minutes, c.gb), CustomerID: deref(c.sub.ExternalCustomerID),
			Authorization: deref(c.sub.ExternalAuthorization), CustomerEmail: firstNonEmpty(deref(c.sub.BillingEmail), c.email),
		})
		if err == nil && providerRef != "" {
			reference = providerRef
		}
	}
	if err != nil {
		s.logger.Error("charge overage", "tenant_id", c.sub.TenantID, "error", err)
		status, failure = "failed", "The overage charge could not be collected."
	}

	var after []postCommit
	txErr := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		paymentID, _, err := s.repo.InsertPayment(ctx, NewPayment{
			TenantID: c.sub.TenantID, SubscriptionID: c.sub.ID, Kind: "overage", Provider: *c.sub.Provider, Reference: reference,
			AmountMinor: c.amount, Currency: c.price.Currency, Status: status, Failure: failure,
		})
		if err != nil {
			return err
		}
		if status == "succeeded" {
			if paymentID != uuid.Nil {
				return s.repo.LinkOveragePayment(ctx, c.usageID, paymentID)
			}
			return nil
		}
		sub, err := s.repo.GetSubscription(ctx, c.sub.TenantID)
		if err != nil || sub == nil {
			return err
		}
		after, err = s.enterPastDue(ctx, sub)
		return err
	})
	for _, f := range after {
		f(ctx)
	}
	return txErr
}

func overageDescription(minutes, gb int) string {
	switch {
	case minutes > 0 && gb > 0:
		return "Extra call minutes and recording storage"
	case gb > 0:
		return "Extra recording storage"
	default:
		return "Extra call minutes"
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
