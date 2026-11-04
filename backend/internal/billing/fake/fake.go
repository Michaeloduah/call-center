// Package fake is an in-memory billing provider for tests and local development. Its webhooks
// are signed with a shared secret (HMAC-SHA256 of the body), so the same verification path as a
// real provider is exercised.
package fake

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/billing"
)

// SignatureHeader carries the hex HMAC of the request body.
const SignatureHeader = "X-Fake-Signature"

const maxBody = 1 << 20

// Sign returns the signature a webhook body must carry.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Payload is the JSON a fake webhook carries. It maps one to one onto billing.Event.
type Payload struct {
	ID                     string     `json:"id"`
	Type                   string     `json:"type"`
	TenantID               uuid.UUID  `json:"tenant_id"`
	PlanPriceID            uuid.UUID  `json:"plan_price_id"`
	ExternalCustomerID     string     `json:"external_customer_id"`
	ExternalSubscriptionID string     `json:"external_subscription_id"`
	Authorization          string     `json:"authorization"`
	PaymentReference       string     `json:"payment_reference"`
	AmountMinor            int64      `json:"amount_minor"`
	Currency               string     `json:"currency"`
	Failure                string     `json:"failure"`
	PeriodStart            *time.Time `json:"period_start"`
	PeriodEnd              *time.Time `json:"period_end"`
}

// Provider implements billing.Provider without any network access.
type Provider struct {
	secret      string
	checkoutURL string

	mu       sync.Mutex
	charges  []billing.OverageCharge
	cancels  []string
	failNext bool
}

// New creates a fake provider. checkoutURL is the page that simulates paying (the web app's fake checkout).
func New(secret, checkoutURL string) *Provider {
	return &Provider{secret: secret, checkoutURL: checkoutURL}
}

func (*Provider) Name() billing.ProviderName { return billing.ProviderFake }

func (*Provider) SyncPrice(_ context.Context, _ billing.Plan, price billing.Price) (string, error) {
	return "fake_price_" + price.ID.String(), nil
}

func (p *Provider) CreateCheckout(_ context.Context, req billing.CheckoutRequest) (string, error) {
	q := url.Values{}
	q.Set("tenant_id", req.TenantID.String())
	q.Set("plan_price_id", req.Price.ID.String())
	q.Set("reference", req.Reference)
	return p.checkoutURL + "?" + q.Encode(), nil
}

func (p *Provider) CancelSubscription(_ context.Context, externalID string, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancels = append(p.cancels, externalID)
	return nil
}

func (p *Provider) ChargeOverage(_ context.Context, c billing.OverageCharge) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failNext {
		p.failNext = false
		return "", errors.New("fake: card declined")
	}
	p.charges = append(p.charges, c)
	return "fake_" + c.Reference, nil
}

func (p *Provider) ParseWebhook(r *http.Request) (billing.Event, error) {
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody))
	if err != nil {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	if !hmac.Equal([]byte(r.Header.Get(SignatureHeader)), []byte(Sign(p.secret, body))) {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	var pl Payload
	if err := json.Unmarshal(body, &pl); err != nil {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	return billing.Event{
		ID: pl.ID, Provider: billing.ProviderFake, Type: billing.EventType(pl.Type), Raw: body,
		TenantID: pl.TenantID, PriceID: pl.PlanPriceID,
		ExternalCustomerID: pl.ExternalCustomerID, ExternalSubscriptionID: pl.ExternalSubscriptionID, Authorization: pl.Authorization,
		PeriodStart: pl.PeriodStart, PeriodEnd: pl.PeriodEnd, PaymentReference: pl.PaymentReference,
		AmountMinor: pl.AmountMinor, Currency: pl.Currency, Failure: pl.Failure,
	}, nil
}

// FailNextCharge makes the next overage charge fail (tests).
func (p *Provider) FailNextCharge() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNext = true
}

// Charges returns the overage charges made so far (tests).
func (p *Provider) Charges() []billing.OverageCharge {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]billing.OverageCharge(nil), p.charges...)
}

// Cancelled returns the provider subscriptions cancelled so far (tests).
func (p *Provider) Cancelled() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.cancels...)
}
