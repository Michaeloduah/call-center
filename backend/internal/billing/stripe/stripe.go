// Package stripe is a thin Stripe client behind billing.Provider (D13, D19). It uses only the
// standard library (form-encoded requests, HMAC webhook verification). API reference:
// https://docs.stripe.com/api
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/billing"
)

const (
	DefaultBaseURL = "https://api.stripe.com/v1"
	// SignatureHeader is "t=<unix>,v1=<hex hmac>[,v1=...]" over "<t>.<body>".
	SignatureHeader = "Stripe-Signature"
	// Tolerance is how old a signed webhook may be (replay protection).
	Tolerance = 5 * time.Minute
	maxBody   = 1 << 20
)

type Client struct {
	baseURL       string
	secret        string
	webhookSecret string
	http          *http.Client
	now           func() time.Time
}

// New creates a client. baseURL is DefaultBaseURL outside tests.
func New(baseURL, secretKey, webhookSecret string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), secret: secretKey, webhookSecret: webhookSecret,
		http: &http.Client{Timeout: 20 * time.Second}, now: time.Now}
}

func (*Client) Name() billing.ProviderName { return billing.ProviderStripe }

// call sends a form-encoded request. idempotencyKey (optional) makes a retried request safe.
func (c *Client) call(ctx context.Context, method, path string, form url.Values, idempotencyKey string, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("stripe: build request: %w", err)
	}
	req.SetBasicAuth(c.secret, "")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("stripe: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("stripe: read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("stripe: %s %s: status %d: %s", method, path, resp.StatusCode, e.Error.Message)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("stripe: %s %s: unreadable response", method, path)
	}
	return nil
}

type idResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (c *Client) SyncPrice(ctx context.Context, plan billing.Plan, price billing.Price) (string, error) {
	var product idResponse
	if err := c.call(ctx, http.MethodPost, "/products", url.Values{"name": {plan.Name}}, "", &product); err != nil {
		return "", err
	}
	every := "month"
	if price.Interval == billing.IntervalYearly {
		every = "year"
	}
	var p idResponse
	err := c.call(ctx, http.MethodPost, "/prices", url.Values{
		"product": {product.ID}, "unit_amount": {strconv.FormatInt(price.AmountMinor, 10)},
		"currency": {strings.ToLower(price.Currency)}, "recurring[interval]": {every},
	}, "", &p)
	if err != nil || p.ID == "" {
		return "", fmt.Errorf("stripe: create price: %w", err)
	}
	return p.ID, nil
}

func (c *Client) CreateCheckout(ctx context.Context, req billing.CheckoutRequest) (string, error) {
	if req.Price.ExternalID == nil {
		return "", fmt.Errorf("stripe: price %s is not synced", req.Price.ID)
	}
	form := url.Values{
		"mode": {"subscription"}, "line_items[0][price]": {*req.Price.ExternalID}, "line_items[0][quantity]": {"1"},
		"success_url": {req.ReturnURL + "?checkout=success"}, "cancel_url": {req.ReturnURL + "?checkout=cancelled"},
		"client_reference_id": {req.Reference},
		// Echoed back in webhooks so the payment can be tied to the tenant and price.
		"metadata[tenant_id]": {req.TenantID.String()}, "metadata[plan_price_id]": {req.Price.ID.String()},
	}
	if req.CustomerID != "" {
		form.Set("customer", req.CustomerID)
	} else {
		form.Set("customer_email", req.CustomerEmail)
	}
	var s idResponse
	if err := c.call(ctx, http.MethodPost, "/checkout/sessions", form, req.Reference, &s); err != nil || s.URL == "" {
		return "", fmt.Errorf("stripe: create checkout session: %w", err)
	}
	return s.URL, nil
}

func (c *Client) CancelSubscription(ctx context.Context, id string, atPeriodEnd bool) error {
	if atPeriodEnd {
		return c.call(ctx, http.MethodPost, "/subscriptions/"+id, url.Values{"cancel_at_period_end": {"true"}}, "", nil)
	}
	return c.call(ctx, http.MethodDelete, "/subscriptions/"+id, nil, "", nil)
}

// ChargeOverage bills the overage as a one-off invoice paid at once with the customer's saved
// payment method. The reference doubles as the idempotency key, so a retried charge is not repeated.
func (c *Client) ChargeOverage(ctx context.Context, ch billing.OverageCharge) (string, error) {
	if ch.CustomerID == "" {
		return "", fmt.Errorf("stripe: no customer to charge")
	}
	currency := strings.ToLower(ch.Currency)
	if err := c.call(ctx, http.MethodPost, "/invoiceitems", url.Values{
		"customer": {ch.CustomerID}, "amount": {strconv.FormatInt(ch.AmountMinor, 10)}, "currency": {currency}, "description": {ch.Description},
	}, ch.Reference+"_item", nil); err != nil {
		return "", err
	}
	var invoice idResponse
	if err := c.call(ctx, http.MethodPost, "/invoices", url.Values{
		"customer": {ch.CustomerID}, "collection_method": {"charge_automatically"}, "auto_advance": {"false"},
		"pending_invoice_items_behavior": {"include"}, "currency": {currency},
	}, ch.Reference+"_invoice", &invoice); err != nil {
		return "", err
	}
	if err := c.call(ctx, http.MethodPost, "/invoices/"+invoice.ID+"/pay", url.Values{}, ch.Reference+"_pay", nil); err != nil {
		return "", err
	}
	return invoice.ID, nil
}

type event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type checkoutSession struct {
	Customer      string            `json:"customer"`
	Subscription  string            `json:"subscription"`
	Invoice       string            `json:"invoice"`
	Currency      string            `json:"currency"`
	AmountTotal   int64             `json:"amount_total"`
	Metadata      map[string]string `json:"metadata"`
	CustomerEmail string            `json:"customer_email"`
}

type invoice struct {
	ID            string `json:"id"`
	Customer      string `json:"customer"`
	CustomerEmail string `json:"customer_email"`
	Subscription  string `json:"subscription"`
	BillingReason string `json:"billing_reason"`
	AmountPaid    int64  `json:"amount_paid"`
	AmountDue     int64  `json:"amount_due"`
	Currency      string `json:"currency"`
	Lines         struct {
		Data []struct {
			Period struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"period"`
		} `json:"data"`
	} `json:"lines"`
}

// ParseWebhook verifies the signature (and its age) and maps the event.
func (c *Client) ParseWebhook(r *http.Request) (billing.Event, error) {
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody))
	if err != nil {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	if !c.validSignature(r.Header.Get(SignatureHeader), body) {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	var e event
	if err := json.Unmarshal(body, &e); err != nil || e.ID == "" {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	ev := billing.Event{ID: e.ID, Provider: billing.ProviderStripe, Raw: body, Type: billing.EventIgnored}

	switch e.Type {
	case "checkout.session.completed":
		var s checkoutSession
		if json.Unmarshal(e.Data.Object, &s) != nil {
			return ev, nil
		}
		tenant, tErr := uuid.Parse(s.Metadata["tenant_id"])
		price, pErr := uuid.Parse(s.Metadata["plan_price_id"])
		if tErr != nil || pErr != nil || s.Subscription == "" {
			return ev, nil
		}
		ev.Type, ev.TenantID, ev.PriceID = billing.EventSubscriptionActivated, tenant, price
		ev.ExternalCustomerID, ev.ExternalSubscriptionID, ev.CustomerEmail = s.Customer, s.Subscription, s.CustomerEmail
		// The first invoice's id is the payment reference, so invoice.paid for it does not record it twice.
		ev.PaymentReference, ev.AmountMinor, ev.Currency = s.Invoice, s.AmountTotal, strings.ToUpper(s.Currency)
	case "invoice.paid", "invoice.payment_failed":
		var inv invoice
		if json.Unmarshal(e.Data.Object, &inv) != nil || inv.BillingReason == "manual" {
			return ev, nil // our own one-off overage invoices are accounted for by the charge itself
		}
		ev.ExternalCustomerID, ev.ExternalSubscriptionID, ev.PaymentReference = inv.Customer, inv.Subscription, inv.ID
		ev.Currency = strings.ToUpper(inv.Currency)
		if e.Type == "invoice.paid" {
			ev.Type, ev.AmountMinor = billing.EventPaymentSucceeded, inv.AmountPaid
		} else {
			ev.Type, ev.AmountMinor, ev.Failure = billing.EventPaymentFailed, inv.AmountDue, "The payment was declined."
		}
	case "customer.subscription.deleted":
		var sub struct {
			ID       string `json:"id"`
			Customer string `json:"customer"`
		}
		if json.Unmarshal(e.Data.Object, &sub) != nil {
			return ev, nil
		}
		ev.Type, ev.ExternalSubscriptionID, ev.ExternalCustomerID = billing.EventSubscriptionCancelled, sub.ID, sub.Customer
	}
	return ev, nil
}

// validSignature checks "t=...,v1=..." against HMAC-SHA256(webhookSecret, "<t>.<body>") and the
// timestamp's age, in constant time.
func (c *Client) validSignature(header string, body []byte) bool {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		switch {
		case !ok:
		case k == "t":
			ts = v
		case k == "v1":
			sigs = append(sigs, v)
		}
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return false
	}
	if age := c.now().Sub(time.Unix(sec, 0)); age > Tolerance || age < -Tolerance {
		return false
	}
	want := Sign(c.webhookSecret, sec, body)
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(want)) {
			return true
		}
	}
	return false
}

// Sign returns the v1 signature for a webhook body at a time (used by tests and tooling).
func Sign(secret string, unix int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(unix, 10) + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
