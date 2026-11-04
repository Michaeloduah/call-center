package billing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// EventType is what the billing service understands from any provider.
type EventType string

const (
	EventSubscriptionActivated EventType = "subscription.activated"
	EventPaymentSucceeded      EventType = "payment.succeeded"
	EventPaymentFailed         EventType = "payment.failed"
	EventSubscriptionCancelled EventType = "subscription.cancelled"
	// EventSubscriptionLinked tells us a provider subscription id for a customer we already know
	// (Paystack announces it separately from the first payment).
	EventSubscriptionLinked EventType = "subscription.linked"
	// EventIgnored is a verified event the service has no use for. It is still stored.
	EventIgnored EventType = "ignored"
)

// Event is a verified provider webhook mapped to the service's vocabulary. Adapters fill in what
// the provider knows; the service resolves the rest.
type Event struct {
	ID       string // the provider's event id, unique per provider
	Provider ProviderName
	Type     EventType
	Raw      []byte // the verified payload, kept for debugging

	// Set from the checkout metadata (subscription.activated).
	TenantID    uuid.UUID
	PriceID     uuid.UUID
	CheckoutRef string

	ExternalCustomerID     string
	CustomerEmail          string
	ExternalSubscriptionID string
	Authorization          string
	PeriodStart            *time.Time
	PeriodEnd              *time.Time

	PaymentReference string
	AmountMinor      int64
	Currency         string
	Failure          string
}

// CheckoutRequest asks a provider for a hosted payment page.
type CheckoutRequest struct {
	TenantID      uuid.UUID
	Plan          Plan
	Price         Price
	Reference     string // our reference, echoed back in webhooks
	CustomerEmail string
	CustomerName  string
	CustomerID    string // the provider's customer id if the tenant has one
	ReturnURL     string
}

// OverageCharge is a one-off charge for usage beyond the plan.
type OverageCharge struct {
	Reference     string
	AmountMinor   int64
	Currency      string
	Description   string
	CustomerID    string
	Authorization string
	CustomerEmail string
}

// Provider is a payment provider. Implementations are thin HTTP clients; the service never
// sees provider-specific types.
type Provider interface {
	Name() ProviderName
	// SyncPrice makes the price exist at the provider and returns its id there.
	SyncPrice(ctx context.Context, plan Plan, price Price) (externalID string, err error)
	CreateCheckout(ctx context.Context, req CheckoutRequest) (redirectURL string, err error)
	CancelSubscription(ctx context.Context, externalSubscriptionID string, atPeriodEnd bool) error
	ChargeOverage(ctx context.Context, charge OverageCharge) (externalReference string, err error)
	// ParseWebhook verifies the request's signature and maps the event. A bad signature returns ErrInvalidWebhook.
	ParseWebhook(r *http.Request) (Event, error)
}

// ErrInvalidWebhook is returned for unsigned, mis-signed or stale webhooks.
var ErrInvalidWebhook = fmt.Errorf("billing: invalid webhook")

// Registry chooses the provider for a currency (D19) and hands out provider clients.
type Registry struct {
	providers          map[ProviderName]Provider
	paystackCurrencies map[string]bool
	fake               bool
}

// NewRegistry builds a registry. With fake set (development) every currency uses the fake provider.
func NewRegistry(fake bool, paystackCurrencies []string, providers ...Provider) Registry {
	r := Registry{providers: map[ProviderName]Provider{}, paystackCurrencies: map[string]bool{}, fake: fake}
	for _, c := range paystackCurrencies {
		r.paystackCurrencies[c] = true
	}
	for _, p := range providers {
		r.providers[p.Name()] = p
	}
	return r
}

// ProviderFor decides which provider charges a currency: configured currencies (NGN) go to Paystack, the rest to Stripe.
func (r Registry) ProviderFor(currency string) ProviderName {
	switch {
	case r.fake:
		return ProviderFake
	case r.paystackCurrencies[currency]:
		return ProviderPaystack
	default:
		return ProviderStripe
	}
}

// Get returns the client for a provider, or errProviderDown when it is not configured.
func (r Registry) Get(name ProviderName) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, errProviderDown
	}
	return p, nil
}
