// Package billing manages plans, subscriptions, usage-based overage and payments (D12, D13, D16–D19).
//
// A subscription changes only in response to a verified provider webhook, never because a browser
// came back from checkout. Providers (Paystack, Stripe) sit behind the Provider interface.
package billing

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/limits"
	"callcenter/internal/platform/apperr"
)

type ProviderName string

const (
	ProviderPaystack ProviderName = "paystack"
	ProviderStripe   ProviderName = "stripe"
	// ProviderFake is the in-memory provider used for tests and local development.
	ProviderFake ProviderName = "fake"
)

type Interval string

const (
	IntervalMonthly Interval = "monthly"
	IntervalYearly  Interval = "yearly"
)

func (i Interval) Valid() bool { return i == IntervalMonthly || i == IntervalYearly }

type Status string

const (
	StatusActive     Status = "active"
	StatusPastDue    Status = "past_due"
	StatusCancelled  Status = "cancelled"
	StatusIncomplete Status = "incomplete"
)

const (
	// PastDueGrace is how long a failed payment may stay unpaid before the tenant falls back to the free plan (D18).
	PastDueGrace = 14 * 24 * time.Hour
	// BytesPerGB is the unit storage overage is billed in.
	BytesPerGB = int64(1) << 30
)

// Plan is a set of limits. A nil limit means unlimited.
type Plan struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsFree      bool      `json:"is_free"`
	IsPublic    bool      `json:"is_public"`
	IsActive    bool      `json:"is_active"`
	limits.Caps
	Prices    []Price   `json:"prices"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Price is one way to pay for a plan: a currency and billing interval, with overage rates.
type Price struct {
	ID                    uuid.UUID    `json:"id"`
	PlanID                uuid.UUID    `json:"plan_id"`
	Currency              string       `json:"currency"`
	Interval              Interval     `json:"interval"`
	AmountMinor           int64        `json:"amount_minor"`
	OveragePerMinuteMinor int64        `json:"overage_per_minute_minor"`
	OveragePerGBMinor     int64        `json:"overage_per_gb_minor"`
	Provider              ProviderName `json:"provider"`
	Synced                bool         `json:"synced"`
	ExternalID            *string      `json:"-"`
	IsActive              bool         `json:"is_active"`
}

// Subscription is a tenant's current plan and billing period.
type Subscription struct {
	ID                     uuid.UUID     `json:"id"`
	TenantID               uuid.UUID     `json:"tenant_id"`
	PlanID                 uuid.UUID     `json:"plan_id"`
	PriceID                *uuid.UUID    `json:"plan_price_id"`
	Provider               *ProviderName `json:"provider"`
	ExternalCustomerID     *string       `json:"-"`
	ExternalSubscriptionID *string       `json:"-"`
	ExternalAuthorization  *string       `json:"-"`
	BillingEmail           *string       `json:"-"`
	Status                 Status        `json:"status"`
	PastDueSince           *time.Time    `json:"past_due_since"`
	CancelAtPeriodEnd      bool          `json:"cancel_at_period_end"`
	PeriodStart            time.Time     `json:"current_period_start"`
	PeriodEnd              time.Time     `json:"current_period_end"`
}

type Payment struct {
	ID          uuid.UUID    `json:"id"`
	Kind        string       `json:"kind"`
	Provider    ProviderName `json:"provider"`
	Reference   string       `json:"reference"`
	AmountMinor int64        `json:"amount_minor"`
	Currency    string       `json:"currency"`
	Status      string       `json:"status"`
	Failure     *string      `json:"failure"`
	PaidAt      time.Time    `json:"paid_at"`
}

// Overview is the tenant's billing page.
type Overview struct {
	Subscription      Subscription    `json:"subscription"`
	Plan              Plan            `json:"plan"`
	Price             *Price          `json:"price"`
	Limits            limits.Caps     `json:"limits"`
	Policies          limits.Policies `json:"policies"`
	Usage             limits.Usage    `json:"usage"`
	AgentsWithoutSeat []limits.Person `json:"agents_without_seat"`
	Payments          []Payment       `json:"payments"`
	Plans             []Plan          `json:"plans"` // what the tenant can switch to
}

// PlatformTenantBilling is a tenant's billing as the platform admin sees it.
type PlatformTenantBilling struct {
	Subscription Subscription `json:"subscription"`
	Plan         Plan         `json:"plan"`
	Price        *Price       `json:"price"`
	Usage        limits.Usage `json:"usage"`
	Payments     []Payment    `json:"payments"`
}

var (
	errPlanNotFound   = apperr.NotFound("Plan")
	errPriceNotFound  = apperr.NotFound("Price")
	errPlanInUse      = apperr.Conflict("plan_in_use", "Tenants are subscribed to this plan. Deactivate it instead.")
	errPriceInUse     = apperr.Conflict("price_in_use", "Tenants are subscribed at this price. Deactivate it instead.")
	errFreePlan       = apperr.Conflict("free_plan", "The free plan cannot be deleted or deactivated, and has no prices.")
	errAlreadyOnPrice = apperr.Conflict("already_subscribed", "You are already subscribed at this price.")
	errNothingToStop  = apperr.Conflict("nothing_to_cancel", "You are on the free plan: there is nothing to cancel.")
	errNeedsPaidPlan  = apperr.Conflict("overage_needs_paid_plan", "Paying for extra usage is only available on a paid plan.")
	errNoOverageRate  = apperr.Conflict("no_overage_rate", "Your plan has no overage rate for that, so it cannot be billed.")
	errNotSynced      = apperr.Conflict("price_not_synced", "This price is not available for checkout yet.")
	errProviderDown   = apperr.Conflict("provider_unavailable", "The payment provider is not available right now. Please try again later.")
)
