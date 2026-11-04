package stripe_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/billing"
	"callcenter/internal/billing/stripe"
)

const (
	key           = "sk_test_secret"
	webhookSecret = "whsec_test"
)

type recorded struct {
	Method, Path, User, Idempotency string
	Form                            url.Values
}

func server(t *testing.T, responses ...string) (*stripe.Client, *[]recorded) {
	t.Helper()
	var reqs []recorded
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		user, _, _ := r.BasicAuth()
		reqs = append(reqs, recorded{r.Method, r.URL.Path, user, r.Header.Get("Idempotency-Key"), form})
		status := http.StatusOK
		resp := responses[min(i, len(responses)-1)]
		if strings.HasPrefix(resp, "!") {
			status, resp = http.StatusPaymentRequired, resp[1:]
		}
		i++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return stripe.New(srv.URL, key, webhookSecret), &reqs
}

func signedRequest(body string, at time.Time) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	unix := at.Unix()
	r.Header.Set(stripe.SignatureHeader, fmt.Sprintf("t=%d,v1=%s", unix, stripe.Sign(webhookSecret, unix, []byte(body))))
	return r
}

func TestSyncPriceCreatesAProductAndARecurringPrice(t *testing.T) {
	c, reqs := server(t, `{"id":"prod_1"}`, `{"id":"price_1"}`)
	id, err := c.SyncPrice(context.Background(), billing.Plan{Name: "Pro"}, billing.Price{Currency: "USD", Interval: billing.IntervalYearly, AmountMinor: 49000})
	if err != nil || id != "price_1" {
		t.Fatalf("SyncPrice = %q, %v", id, err)
	}
	p := (*reqs)[1]
	if (*reqs)[0].Path != "/products" || (*reqs)[0].User != key || p.Path != "/prices" || p.Form.Get("product") != "prod_1" ||
		p.Form.Get("unit_amount") != "49000" || p.Form.Get("currency") != "usd" || p.Form.Get("recurring[interval]") != "year" {
		t.Fatalf("requests = %+v", *reqs)
	}
}

func TestCheckoutSessionCarriesTenantAndPrice(t *testing.T) {
	c, reqs := server(t, `{"id":"cs_1","url":"https://checkout.stripe.com/c/pay/cs_1"}`)
	ext, tenant, price := "price_1", uuid.New(), uuid.New()
	url, err := c.CreateCheckout(context.Background(), billing.CheckoutRequest{
		TenantID: tenant, Price: billing.Price{ID: price, ExternalID: &ext}, Reference: "chk_1",
		CustomerEmail: "a@example.com", ReturnURL: "https://app.test/admin/billing",
	})
	if err != nil || !strings.HasPrefix(url, "https://checkout.stripe.com/") {
		t.Fatalf("CreateCheckout = %q, %v", url, err)
	}
	r := (*reqs)[0]
	if r.Path != "/checkout/sessions" || r.Idempotency != "chk_1" || r.Form.Get("mode") != "subscription" || r.Form.Get("line_items[0][price]") != "price_1" ||
		r.Form.Get("metadata[tenant_id]") != tenant.String() || r.Form.Get("metadata[plan_price_id]") != price.String() ||
		r.Form.Get("customer_email") != "a@example.com" || r.Form.Get("customer") != "" {
		t.Fatalf("request = %+v", r)
	}
	// A known customer is reused instead of creating a second one.
	c2, reqs2 := server(t, `{"id":"cs_2","url":"https://checkout.stripe.com/x"}`)
	if _, err := c2.CreateCheckout(context.Background(), billing.CheckoutRequest{Price: billing.Price{ExternalID: &ext}, CustomerID: "cus_1", CustomerEmail: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	if f := (*reqs2)[0].Form; f.Get("customer") != "cus_1" || f.Get("customer_email") != "" {
		t.Fatalf("known customer form = %v", f)
	}
}

func TestCancelAtPeriodEndOrNow(t *testing.T) {
	c, reqs := server(t, `{}`)
	if err := c.CancelSubscription(context.Background(), "sub_1", true); err != nil {
		t.Fatal(err)
	}
	if err := c.CancelSubscription(context.Background(), "sub_2", false); err != nil {
		t.Fatal(err)
	}
	if (*reqs)[0].Method != "POST" || (*reqs)[0].Path != "/subscriptions/sub_1" || (*reqs)[0].Form.Get("cancel_at_period_end") != "true" ||
		(*reqs)[1].Method != "DELETE" || (*reqs)[1].Path != "/subscriptions/sub_2" {
		t.Fatalf("requests = %+v", *reqs)
	}
}

func TestOverageIsAnInvoiceItemThenAnInvoicePaidAtOnceWithIdempotencyKeys(t *testing.T) {
	c, reqs := server(t, `{}`, `{"id":"in_1"}`, `{}`)
	ref, err := c.ChargeOverage(context.Background(), billing.OverageCharge{Reference: "ovg_1", AmountMinor: 2500, Currency: "USD", Description: "Extra call minutes", CustomerID: "cus_1"})
	if err != nil || ref != "in_1" {
		t.Fatalf("ChargeOverage = %q, %v", ref, err)
	}
	paths := []string{(*reqs)[0].Path, (*reqs)[1].Path, (*reqs)[2].Path}
	if strings.Join(paths, " ") != "/invoiceitems /invoices /invoices/in_1/pay" {
		t.Fatalf("paths = %v", paths)
	}
	keys := map[string]bool{}
	for _, r := range *reqs {
		keys[r.Idempotency] = true
		if !strings.HasPrefix(r.Idempotency, "ovg_1") {
			t.Fatalf("idempotency key %q does not derive from the charge reference", r.Idempotency)
		}
	}
	if len(keys) != 3 || (*reqs)[0].Form.Get("amount") != "2500" || (*reqs)[0].Form.Get("customer") != "cus_1" {
		t.Fatalf("keys=%v first=%+v", keys, (*reqs)[0].Form)
	}

	declined, _ := server(t, `{}`, `{"id":"in_2"}`, `!{"error":{"message":"Your card was declined."}}`)
	if _, err := declined.ChargeOverage(context.Background(), billing.OverageCharge{CustomerID: "cus_1"}); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("a declined card reported %v", err)
	}
	if _, err := c.ChargeOverage(context.Background(), billing.OverageCharge{}); err == nil {
		t.Fatal("charging without a customer succeeded")
	}
}

func TestErrorsNeverContainTheKey(t *testing.T) {
	c, _ := server(t, `!{"error":{"message":"No such price"}}`)
	_, err := c.SyncPrice(context.Background(), billing.Plan{}, billing.Price{})
	if err == nil || strings.Contains(err.Error(), key) {
		t.Fatalf("error = %v", err)
	}
}

func TestWebhookSignatureAndAge(t *testing.T) {
	c, _ := server(t, `{}`)
	body := `{"id":"evt_1","type":"invoice.paid","data":{"object":{}}}`
	now := time.Now()
	if _, err := c.ParseWebhook(signedRequest(body, now)); err != nil {
		t.Fatalf("valid webhook rejected: %v", err)
	}
	reject := func(name string, r *http.Request) {
		t.Helper()
		if _, err := c.ParseWebhook(r); !errors.Is(err, billing.ErrInvalidWebhook) {
			t.Errorf("%s: error = %v, want ErrInvalidWebhook", name, err)
		}
	}
	reject("stale", signedRequest(body, now.Add(-10*time.Minute)))
	reject("from the future", signedRequest(body, now.Add(10*time.Minute)))
	tampered := signedRequest(body, now)
	tampered.Body = io.NopCloser(bytes.NewBufferString(body + " "))
	reject("tampered body", tampered)
	reject("no signature", httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	wrongKey := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	wrongKey.Header.Set(stripe.SignatureHeader, fmt.Sprintf("t=%d,v1=%s", now.Unix(), stripe.Sign("another", now.Unix(), []byte(body))))
	reject("wrong secret", wrongKey)
	noV1 := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	noV1.Header.Set(stripe.SignatureHeader, fmt.Sprintf("t=%d", now.Unix()))
	reject("no v1 signature", noV1)

	// Stripe may send several v1 signatures while rotating secrets: any valid one is enough.
	multi := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	multi.Header.Set(stripe.SignatureHeader, fmt.Sprintf("t=%d,v1=deadbeef,v1=%s", now.Unix(), stripe.Sign(webhookSecret, now.Unix(), []byte(body))))
	if _, err := c.ParseWebhook(multi); err != nil {
		t.Fatalf("rotated-secret webhook rejected: %v", err)
	}
}

func TestWebhookEventMapping(t *testing.T) {
	c, _ := server(t, `{}`)
	tenant, price := uuid.New(), uuid.New()
	parse := func(body string) billing.Event {
		t.Helper()
		ev, err := c.ParseWebhook(signedRequest(body, time.Now()))
		if err != nil {
			t.Fatal(err)
		}
		return ev
	}

	done := parse(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"customer":"cus_1","subscription":"sub_1","invoice":"in_1",
		"currency":"usd","amount_total":4900,"customer_email":"a@example.com","metadata":{"tenant_id":"` + tenant.String() + `","plan_price_id":"` + price.String() + `"}}}}`)
	if done.Type != billing.EventSubscriptionActivated || done.ID != "evt_1" || done.TenantID != tenant || done.PriceID != price || done.ExternalSubscriptionID != "sub_1" ||
		done.ExternalCustomerID != "cus_1" || done.PaymentReference != "in_1" || done.AmountMinor != 4900 || done.Currency != "USD" {
		t.Fatalf("checkout.session.completed = %+v", done)
	}
	if ignored := parse(`{"id":"evt_2","type":"checkout.session.completed","data":{"object":{"customer":"cus_1","subscription":"sub_1","metadata":{}}}}`); ignored.Type != billing.EventIgnored {
		t.Fatalf("a checkout without our metadata = %+v, want ignored", ignored)
	}

	paid := parse(`{"id":"evt_3","type":"invoice.paid","data":{"object":{"id":"in_9","customer":"cus_1","subscription":"sub_1","billing_reason":"subscription_cycle","amount_paid":4900,"currency":"usd"}}}`)
	if paid.Type != billing.EventPaymentSucceeded || paid.PaymentReference != "in_9" || paid.AmountMinor != 4900 || paid.ExternalSubscriptionID != "sub_1" {
		t.Fatalf("invoice.paid = %+v", paid)
	}
	failed := parse(`{"id":"evt_4","type":"invoice.payment_failed","data":{"object":{"id":"in_10","customer":"cus_1","subscription":"sub_1","billing_reason":"subscription_cycle","amount_due":4900,"currency":"usd"}}}`)
	if failed.Type != billing.EventPaymentFailed || failed.PaymentReference != "in_10" || failed.Failure == "" {
		t.Fatalf("invoice.payment_failed = %+v", failed)
	}
	// Our own one-off overage invoices are accounted for by the charge call, not by their webhooks.
	manual := parse(`{"id":"evt_5","type":"invoice.paid","data":{"object":{"id":"in_11","customer":"cus_1","billing_reason":"manual","amount_paid":100,"currency":"usd"}}}`)
	if manual.Type != billing.EventIgnored {
		t.Fatalf("manual invoice = %+v, want ignored", manual)
	}
	deleted := parse(`{"id":"evt_6","type":"customer.subscription.deleted","data":{"object":{"id":"sub_1","customer":"cus_1"}}}`)
	if deleted.Type != billing.EventSubscriptionCancelled || deleted.ExternalSubscriptionID != "sub_1" {
		t.Fatalf("customer.subscription.deleted = %+v", deleted)
	}
	if other := parse(`{"id":"evt_7","type":"customer.created","data":{"object":{}}}`); other.Type != billing.EventIgnored {
		t.Fatalf("unrelated event = %+v", other)
	}
}
