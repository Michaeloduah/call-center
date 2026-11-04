package paystack_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"callcenter/internal/billing"
	"callcenter/internal/billing/paystack"
)

const key = "sk_test_secret"

type recorded struct {
	Method, Path, Auth string
	Body               map[string]any
}

// server answers every request with the next canned JSON response and records the requests.
func server(t *testing.T, responses ...string) (*paystack.Client, *[]recorded) {
	t.Helper()
	var reqs []recorded
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		reqs = append(reqs, recorded{r.Method, r.URL.Path, r.Header.Get("Authorization"), body})
		status := http.StatusOK
		resp := responses[min(i, len(responses)-1)]
		if strings.HasPrefix(resp, "!") {
			status, resp = http.StatusBadRequest, resp[1:]
		}
		i++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return paystack.New(srv.URL, key), &reqs
}

func sign(body []byte) string {
	mac := hmac.New(sha512.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func webhookRequest(body []byte, signature string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	r.Header.Set(paystack.SignatureHeader, signature)
	return r
}

func TestSyncPriceCreatesAPlan(t *testing.T) {
	c, reqs := server(t, `{"status":true,"message":"ok","data":{"plan_code":"PLN_abc"}}`)
	id, err := c.SyncPrice(context.Background(), billing.Plan{Name: "Pro"}, billing.Price{Currency: "NGN", Interval: billing.IntervalYearly, AmountMinor: 5000000})
	if err != nil || id != "PLN_abc" {
		t.Fatalf("SyncPrice = %q, %v", id, err)
	}
	r := (*reqs)[0]
	if r.Method != "POST" || r.Path != "/plan" || r.Auth != "Bearer "+key || r.Body["interval"] != "annually" || r.Body["amount"] != float64(5000000) || r.Body["currency"] != "NGN" {
		t.Fatalf("request = %+v", r)
	}
}

func TestCheckoutCarriesTenantAndPriceInMetadata(t *testing.T) {
	c, reqs := server(t, `{"status":true,"data":{"authorization_url":"https://checkout.paystack.com/x"}}`)
	ext := "PLN_abc"
	tenant, price := uuid.New(), uuid.New()
	url, err := c.CreateCheckout(context.Background(), billing.CheckoutRequest{
		TenantID: tenant, Price: billing.Price{ID: price, Currency: "NGN", AmountMinor: 100, ExternalID: &ext},
		Reference: "chk_1", CustomerEmail: "a@example.com", ReturnURL: "https://app.test/admin/billing",
	})
	if err != nil || url != "https://checkout.paystack.com/x" {
		t.Fatalf("CreateCheckout = %q, %v", url, err)
	}
	b := (*reqs)[0].Body
	meta := b["metadata"].(map[string]any)
	if b["plan"] != "PLN_abc" || b["reference"] != "chk_1" || b["email"] != "a@example.com" || meta["tenant_id"] != tenant.String() || meta["plan_price_id"] != price.String() {
		t.Fatalf("body = %+v", b)
	}
	if _, err := c.CreateCheckout(context.Background(), billing.CheckoutRequest{Price: billing.Price{}}); err == nil {
		t.Fatal("checkout for an unsynced price succeeded")
	}
}

func TestCancelDisablesWithTheEmailToken(t *testing.T) {
	c, reqs := server(t, `{"status":true,"data":{"email_token":"tok_1"}}`, `{"status":true,"data":{}}`)
	if err := c.CancelSubscription(context.Background(), "SUB_1", true); err != nil {
		t.Fatal(err)
	}
	if len(*reqs) != 2 || (*reqs)[0].Method != "GET" || (*reqs)[0].Path != "/subscription/SUB_1" ||
		(*reqs)[1].Path != "/subscription/disable" || (*reqs)[1].Body["code"] != "SUB_1" || (*reqs)[1].Body["token"] != "tok_1" {
		t.Fatalf("requests = %+v", *reqs)
	}
}

func TestChargeOverage(t *testing.T) {
	c, reqs := server(t, `{"status":true,"data":{"status":"success","reference":"ovg_1"}}`)
	ref, err := c.ChargeOverage(context.Background(), billing.OverageCharge{Reference: "ovg_1", AmountMinor: 2500, Currency: "NGN", Authorization: "AUTH_1", CustomerEmail: "a@example.com"})
	if err != nil || ref != "ovg_1" {
		t.Fatalf("ChargeOverage = %q, %v", ref, err)
	}
	if b := (*reqs)[0].Body; (*reqs)[0].Path != "/transaction/charge_authorization" || b["authorization_code"] != "AUTH_1" || b["amount"] != float64(2500) {
		t.Fatalf("request = %+v", (*reqs)[0])
	}

	declined, _ := server(t, `{"status":true,"data":{"status":"failed","gateway_response":"Insufficient Funds"}}`)
	if _, err := declined.ChargeOverage(context.Background(), billing.OverageCharge{Authorization: "A", CustomerEmail: "a@example.com"}); err == nil {
		t.Fatal("a declined charge reported success")
	}
	if _, err := c.ChargeOverage(context.Background(), billing.OverageCharge{}); err == nil {
		t.Fatal("charging without a saved card succeeded")
	}
}

func TestAPIErrorsKeepTheMessageButNeverTheKey(t *testing.T) {
	c, _ := server(t, `!{"status":false,"message":"Invalid key"}`)
	_, err := c.SyncPrice(context.Background(), billing.Plan{}, billing.Price{})
	if err == nil || !strings.Contains(err.Error(), "Invalid key") || strings.Contains(err.Error(), key) {
		t.Fatalf("error = %v", err)
	}
}

func TestWebhookSignature(t *testing.T) {
	c, _ := server(t, `{}`)
	body := []byte(`{"event":"charge.success","data":{"id":1,"reference":"r1"}}`)
	if _, err := c.ParseWebhook(webhookRequest(body, sign(body))); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if _, err := c.ParseWebhook(webhookRequest(body, strings.ToUpper(sign(body)))); err != nil {
		t.Fatalf("upper-case hex signature rejected: %v", err)
	}
	for name, req := range map[string]*http.Request{
		"missing":  webhookRequest(body, ""),
		"wrong":    webhookRequest(body, sign([]byte("other"))),
		"tampered": webhookRequest(append([]byte(" "), body...), sign(body)),
	} {
		if _, err := c.ParseWebhook(req); !errors.Is(err, billing.ErrInvalidWebhook) {
			t.Errorf("%s signature: error = %v, want ErrInvalidWebhook", name, err)
		}
	}
}

func TestWebhookEventMapping(t *testing.T) {
	c, _ := server(t, `{}`)
	tenant, price := uuid.New(), uuid.New()
	parse := func(body string) billing.Event {
		t.Helper()
		ev, err := c.ParseWebhook(webhookRequest([]byte(body), sign([]byte(body))))
		if err != nil {
			t.Fatal(err)
		}
		return ev
	}

	first := parse(`{"event":"charge.success","data":{"id":11,"reference":"chk_1","amount":490000,"currency":"NGN",
		"customer":{"customer_code":"CUS_1","email":"a@example.com"},"authorization":{"authorization_code":"AUTH_1","reusable":true},
		"metadata":{"tenant_id":"` + tenant.String() + `","plan_price_id":"` + price.String() + `","reference":"chk_1"}}}`)
	if first.Type != billing.EventSubscriptionActivated || first.TenantID != tenant || first.PriceID != price || first.ExternalCustomerID != "CUS_1" ||
		first.Authorization != "AUTH_1" || first.PaymentReference != "chk_1" || first.AmountMinor != 490000 || first.CustomerEmail != "a@example.com" {
		t.Fatalf("first payment = %+v", first)
	}

	renewal := parse(`{"event":"charge.success","data":{"id":12,"reference":"r2","amount":490000,"currency":"NGN","customer":{"customer_code":"CUS_1"},"authorization":{"authorization_code":"AUTH_1","reusable":false}}}`)
	if renewal.Type != billing.EventPaymentSucceeded || renewal.ExternalCustomerID != "CUS_1" || renewal.Authorization != "" {
		t.Fatalf("renewal = %+v", renewal)
	}

	linked := parse(`{"event":"subscription.create","data":{"id":5,"subscription_code":"SUB_9","customer":{"customer_code":"CUS_1"}}}`)
	if linked.Type != billing.EventSubscriptionLinked || linked.ExternalSubscriptionID != "SUB_9" || linked.ExternalCustomerID != "CUS_1" || linked.PaymentReference != "" {
		t.Fatalf("subscription.create = %+v", linked)
	}

	failed := parse(`{"event":"invoice.payment_failed","data":{"id":7,"invoice_code":"INV_1","gateway_response":"Insufficient Funds","subscription":{"subscription_code":"SUB_9"},"customer":{"customer_code":"CUS_1"}}}`)
	if failed.Type != billing.EventPaymentFailed || failed.ExternalSubscriptionID != "SUB_9" || failed.Failure != "Insufficient Funds" {
		t.Fatalf("invoice.payment_failed = %+v", failed)
	}

	disabled := parse(`{"event":"subscription.disable","data":{"id":5,"subscription_code":"SUB_9"}}`)
	if disabled.Type != billing.EventSubscriptionCancelled || disabled.ExternalSubscriptionID != "SUB_9" {
		t.Fatalf("subscription.disable = %+v", disabled)
	}

	other := parse(`{"event":"transfer.success","data":{"id":99,"reference":"t1"}}`)
	if other.Type != billing.EventIgnored || other.ID == "" {
		t.Fatalf("unknown event = %+v", other)
	}
	// The same event delivered twice gets the same id, so the replay is recognised.
	if again := parse(`{"event":"subscription.create","data":{"id":5,"subscription_code":"SUB_9","customer":{"customer_code":"CUS_1"}}}`); again.ID != linked.ID {
		t.Fatalf("ids differ for the same event: %q vs %q", again.ID, linked.ID)
	}
}
