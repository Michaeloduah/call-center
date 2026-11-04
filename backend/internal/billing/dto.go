package billing

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"callcenter/internal/limits"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/validate"
)

var (
	planCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,39}$`)
	currency = regexp.MustCompile(`^[A-Z]{3}$`)
)

// PlanBody is the platform admin's plan form.
type PlanBody struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
	IsActive    bool   `json:"is_active"`
	limits.Caps
}

func (b *PlanBody) Validate() error {
	v := validate.Errors{}
	b.Code = strings.ToLower(strings.TrimSpace(b.Code))
	if !planCode.MatchString(b.Code) {
		v.Add("code", "Use 2–40 lowercase letters, digits, hyphens or underscores.")
	}
	b.Name = v.RequiredString("name", b.Name, 80)
	b.Description = v.OptionalString("description", b.Description, 500)
	nonNegative := func(field string, n *int64) {
		if n != nil && *n < 0 {
			v.Add(field, "Must be zero or more, or empty for unlimited.")
		}
	}
	asInt64 := func(n *int) *int64 {
		if n == nil {
			return nil
		}
		x := int64(*n)
		return &x
	}
	nonNegative("max_agents", asInt64(b.MaxAgents))
	nonNegative("max_call_minutes_per_month", asInt64(b.MaxCallMinutes))
	nonNegative("max_storage_bytes", b.MaxStorageBytes)
	nonNegative("max_departments", asInt64(b.MaxDepartments))
	nonNegative("max_widgets", asInt64(b.MaxWidgets))
	return v.Err()
}

func (b PlanBody) input() PlanInput {
	return PlanInput{Code: b.Code, Name: b.Name, Description: b.Description, IsPublic: b.IsPublic, IsActive: b.IsActive, Caps: b.Caps}
}

// PriceBody creates a price.
type PriceBody struct {
	Currency              string   `json:"currency"`
	Interval              Interval `json:"interval"`
	AmountMinor           int64    `json:"amount_minor"`
	OveragePerMinuteMinor int64    `json:"overage_per_minute_minor"`
	OveragePerGBMinor     int64    `json:"overage_per_gb_minor"`
	IsActive              bool     `json:"is_active"`
}

func (b *PriceBody) Validate() error {
	v := validate.Errors{}
	b.Currency = strings.ToUpper(strings.TrimSpace(b.Currency))
	if !currency.MatchString(b.Currency) {
		v.Add("currency", "Use a three-letter currency code such as NGN or USD.")
	}
	if !b.Interval.Valid() {
		v.Add("interval", "Choose monthly or yearly.")
	}
	if b.AmountMinor <= 0 {
		v.Add("amount_minor", "Must be greater than zero (in the currency's smallest unit).")
	}
	if b.OveragePerMinuteMinor < 0 {
		v.Add("overage_per_minute_minor", "Must be zero or more.")
	}
	if b.OveragePerGBMinor < 0 {
		v.Add("overage_per_gb_minor", "Must be zero or more.")
	}
	return v.Err()
}

// PriceUpdateBody changes the parts of a price that may change after subscribers exist. The
// amount, currency and interval never change in place: add a new price instead.
type PriceUpdateBody struct {
	IsActive              bool  `json:"is_active"`
	OveragePerMinuteMinor int64 `json:"overage_per_minute_minor"`
	OveragePerGBMinor     int64 `json:"overage_per_gb_minor"`
}

func (b *PriceUpdateBody) Validate() error {
	v := validate.Errors{}
	if b.OveragePerMinuteMinor < 0 {
		v.Add("overage_per_minute_minor", "Must be zero or more.")
	}
	if b.OveragePerGBMinor < 0 {
		v.Add("overage_per_gb_minor", "Must be zero or more.")
	}
	return v.Err()
}

// PoliciesBody is the tenant admin's choice per cap (D16).
type PoliciesBody struct {
	Minutes limits.Policy `json:"minutes_cap_policy"`
	Storage limits.Policy `json:"storage_cap_policy"`
}

func (b *PoliciesBody) Validate() error {
	v := validate.Errors{}
	if !b.Minutes.Valid() {
		v.Add("minutes_cap_policy", "Choose block or overage.")
	}
	if !b.Storage.Valid() {
		v.Add("storage_cap_policy", "Choose block or overage.")
	}
	return v.Err()
}

// CheckoutBody starts a checkout for a price.
type CheckoutBody struct {
	PriceID uuid.UUID `json:"plan_price_id"`
}

func (b *CheckoutBody) Validate() error {
	if b.PriceID == uuid.Nil {
		return apperr.Validation(map[string]string{"plan_price_id": "Choose a plan."})
	}
	return nil
}

// CheckoutResult is where to send the tenant admin to pay.
type CheckoutResult struct {
	RedirectURL string `json:"redirect_url"`
}
