package call

import (
	"regexp"
	"strings"

	"callcenter/internal/platform/validate"
	"callcenter/internal/widget"
)

// CreateInput is submitted by the caller from the widget.
type CreateInput struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Consent bool   `json:"consent"`
}

var (
	phoneChars  = regexp.MustCompile(`^\+?[0-9 ()\-.]{6,32}$`)
	phoneDigits = regexp.MustCompile(`[0-9]`)
)

const minPhoneDigits = 6

// Validate checks formats only. Which fields are required depends on the widget, see ApplyForm.
func (in *CreateInput) Validate() error {
	v := validate.Errors{}
	in.Name = v.OptionalString("name", in.Name, 120)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.Phone != "" && (!phoneChars.MatchString(in.Phone) || len(phoneDigits.FindAllString(in.Phone, -1)) < minPhoneDigits) {
		v.Add("phone", "Enter a valid phone number.")
	}
	if strings.TrimSpace(in.Email) != "" {
		in.Email = v.Email("email", in.Email)
	} else {
		in.Email = ""
	}
	return v.Err()
}

// ApplyForm enforces the widget's caller-form rules (D8): required fields must be present and
// hidden fields are discarded so callers cannot attach data the tenant did not ask for.
// Consent is always mandatory (D6).
func (in *CreateInput) ApplyForm(form widget.CallerForm) error {
	v := validate.Errors{}
	apply := func(field string, value *string, mode widget.FieldMode) {
		switch form.Effective(mode) {
		case widget.FieldHidden:
			*value = ""
		case widget.FieldRequired:
			if *value == "" {
				v.Add(field, "This field is required.")
			}
		}
	}
	apply("name", &in.Name, form.Name)
	apply("phone", &in.Phone, form.Phone)
	apply("email", &in.Email, form.Email)
	if err := v.Err(); err != nil {
		return err
	}
	if !in.Consent {
		return errConsentRequired
	}
	return nil
}
