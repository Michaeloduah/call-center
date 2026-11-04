package call

import (
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

// Validate checks formats only. Which fields are required depends on the widget, see ApplyForm.
func (in *CreateInput) Validate() error {
	v := validate.Errors{}
	in.Name = v.OptionalString("name", in.Name, 120)
	in.Phone = v.OptionalPhone("phone", in.Phone)
	in.Email = v.OptionalEmail("email", in.Email)
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
