// Package paystack is a thin Paystack client behind billing.Provider (D13, D19). It uses only
// the standard library. API reference: https://paystack.com/docs/api
package paystack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/billing"
)

const (
	DefaultBaseURL = "https://api.paystack.co"
	// SignatureHeader carries the hex HMAC-SHA512 of the raw body, keyed with the secret key.
	SignatureHeader = "X-Paystack-Signature"
	maxBody         = 1 << 20
)

type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// New creates a client. baseURL is DefaultBaseURL outside tests.
func New(baseURL, secretKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), secret: secretKey, http: &http.Client{Timeout: 20 * time.Second}}
}

func (*Client) Name() billing.ProviderName { return billing.ProviderPaystack }

type apiResponse struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call sends a JSON request and returns the response's data. Provider error messages are
// included (they are not secret); the key never is.
func (c *Client) call(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("paystack: encode request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("paystack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paystack: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("paystack: read response: %w", err)
	}
	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("paystack: %s %s: status %d, unreadable response", method, path, resp.StatusCode)
	}
	if resp.StatusCode >= 300 || !out.Status {
		return nil, fmt.Errorf("paystack: %s %s: status %d: %s", method, path, resp.StatusCode, out.Message)
	}
	return out.Data, nil
}

func interval(i billing.Interval) string {
	if i == billing.IntervalYearly {
		return "annually"
	}
	return "monthly"
}

func (c *Client) SyncPrice(ctx context.Context, plan billing.Plan, price billing.Price) (string, error) {
	data, err := c.call(ctx, http.MethodPost, "/plan", map[string]any{
		"name": plan.Name + " (" + price.Currency + " " + string(price.Interval) + ")", "interval": interval(price.Interval),
		"amount": price.AmountMinor, "currency": price.Currency,
	})
	if err != nil {
		return "", err
	}
	var plan2 struct {
		PlanCode string `json:"plan_code"`
	}
	if err := json.Unmarshal(data, &plan2); err != nil || plan2.PlanCode == "" {
		return "", fmt.Errorf("paystack: create plan: no plan code in response")
	}
	return plan2.PlanCode, nil
}

func (c *Client) CreateCheckout(ctx context.Context, req billing.CheckoutRequest) (string, error) {
	if req.Price.ExternalID == nil {
		return "", fmt.Errorf("paystack: price %s is not synced", req.Price.ID)
	}
	data, err := c.call(ctx, http.MethodPost, "/transaction/initialize", map[string]any{
		"email": req.CustomerEmail, "amount": req.Price.AmountMinor, "currency": req.Price.Currency,
		"plan": *req.Price.ExternalID, "reference": req.Reference, "callback_url": req.ReturnURL,
		// Echoed back in webhooks so the first payment can be tied to the tenant and price.
		"metadata": map[string]any{"tenant_id": req.TenantID, "plan_price_id": req.Price.ID, "reference": req.Reference},
	})
	if err != nil {
		return "", err
	}
	var init struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	if err := json.Unmarshal(data, &init); err != nil || init.AuthorizationURL == "" {
		return "", fmt.Errorf("paystack: initialize transaction: no authorization url in response")
	}
	return init.AuthorizationURL, nil
}

// CancelSubscription disables the subscription so it will not renew. The current period stays
// paid; when to end the tenant's access is the billing service's decision, so atPeriodEnd needs no
// separate handling here.
func (c *Client) CancelSubscription(ctx context.Context, code string, _ bool) error {
	data, err := c.call(ctx, http.MethodGet, "/subscription/"+code, nil)
	if err != nil {
		return err
	}
	var s struct {
		EmailToken string `json:"email_token"`
	}
	if err := json.Unmarshal(data, &s); err != nil || s.EmailToken == "" {
		return fmt.Errorf("paystack: subscription %s has no email token", code)
	}
	_, err = c.call(ctx, http.MethodPost, "/subscription/disable", map[string]string{"code": code, "token": s.EmailToken})
	return err
}

func (c *Client) ChargeOverage(ctx context.Context, ch billing.OverageCharge) (string, error) {
	if ch.Authorization == "" || ch.CustomerEmail == "" {
		return "", fmt.Errorf("paystack: no saved card to charge")
	}
	data, err := c.call(ctx, http.MethodPost, "/transaction/charge_authorization", map[string]any{
		"authorization_code": ch.Authorization, "email": ch.CustomerEmail, "amount": ch.AmountMinor,
		"currency": ch.Currency, "reference": ch.Reference, "metadata": map[string]any{"description": ch.Description},
	})
	if err != nil {
		return "", err
	}
	var tx struct {
		Status    string `json:"status"`
		Reference string `json:"reference"`
		Gateway   string `json:"gateway_response"`
	}
	if err := json.Unmarshal(data, &tx); err != nil {
		return "", fmt.Errorf("paystack: charge: unreadable response")
	}
	if tx.Status != "success" {
		return "", fmt.Errorf("paystack: charge %s: %s", tx.Status, tx.Gateway)
	}
	return tx.Reference, nil
}

// webhook is the part of a Paystack event the service uses.
type webhook struct {
	Event string `json:"event"`
	Data  struct {
		ID        json.Number `json:"id"`
		Reference string      `json:"reference"`
		Amount    int64       `json:"amount"`
		Currency  string      `json:"currency"`
		Status    string      `json:"status"`
		Customer  struct {
			Code  string `json:"customer_code"`
			Email string `json:"email"`
		} `json:"customer"`
		Authorization struct {
			Code     string `json:"authorization_code"`
			Reusable bool   `json:"reusable"`
		} `json:"authorization"`
		Metadata struct {
			TenantID    string `json:"tenant_id"`
			PlanPriceID string `json:"plan_price_id"`
			Reference   string `json:"reference"`
		} `json:"metadata"`
		SubscriptionCode string `json:"subscription_code"`
		InvoiceCode      string `json:"invoice_code"`
		Subscription     struct {
			Code string `json:"subscription_code"`
		} `json:"subscription"`
		GatewayResponse string `json:"gateway_response"`
	} `json:"data"`
}

// ParseWebhook verifies the signature and maps the event. Paystack events carry no event id, so
// one is built from the event name and the object it is about: the same event delivered twice
// maps to the same id.
func (c *Client) ParseWebhook(r *http.Request) (billing.Event, error) {
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody))
	if err != nil {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	mac := hmac.New(sha512.New, []byte(c.secret))
	mac.Write(body)
	if !hmac.Equal([]byte(strings.ToLower(r.Header.Get(SignatureHeader))), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return billing.Event{}, billing.ErrInvalidWebhook
	}
	var w webhook
	if err := json.Unmarshal(body, &w); err != nil {
		return billing.Event{}, billing.ErrInvalidWebhook
	}

	d := w.Data
	ev := billing.Event{
		Provider: billing.ProviderPaystack, Raw: body, Type: billing.EventIgnored,
		ExternalCustomerID: d.Customer.Code, CustomerEmail: d.Customer.Email,
		PaymentReference: d.Reference, AmountMinor: d.Amount, Currency: d.Currency,
	}
	switch w.Event {
	case "charge.success":
		ev.ID = w.Event + ":" + firstNonEmpty(d.ID.String(), d.Reference)
		if d.Authorization.Reusable {
			ev.Authorization = d.Authorization.Code
		}
		tenant, tErr := uuid.Parse(d.Metadata.TenantID)
		price, pErr := uuid.Parse(d.Metadata.PlanPriceID)
		if tErr == nil && pErr == nil {
			// The first payment of a checkout: it activates the plan.
			ev.Type, ev.TenantID, ev.PriceID, ev.CheckoutRef = billing.EventSubscriptionActivated, tenant, price, d.Metadata.Reference
		} else {
			ev.Type = billing.EventPaymentSucceeded // a renewal
		}
	case "subscription.create":
		ev.ID = w.Event + ":" + firstNonEmpty(d.SubscriptionCode, d.ID.String())
		ev.Type, ev.ExternalSubscriptionID, ev.PaymentReference = billing.EventSubscriptionLinked, d.SubscriptionCode, ""
	case "invoice.payment_failed":
		ev.ID = w.Event + ":" + firstNonEmpty(d.InvoiceCode, d.ID.String(), d.Subscription.Code)
		ev.Type, ev.ExternalSubscriptionID, ev.Failure = billing.EventPaymentFailed, d.Subscription.Code, d.GatewayResponse
		ev.PaymentReference = d.InvoiceCode
	case "subscription.disable":
		ev.ID = w.Event + ":" + firstNonEmpty(d.SubscriptionCode, d.ID.String())
		ev.Type, ev.ExternalSubscriptionID = billing.EventSubscriptionCancelled, d.SubscriptionCode
	default:
		ev.ID = w.Event + ":" + firstNonEmpty(d.ID.String(), d.Reference, d.SubscriptionCode, d.InvoiceCode)
	}
	return ev, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" && v != "0" {
			return v
		}
	}
	return ""
}
