package call

import (
	"testing"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/widget"
)

func TestApplyForm(t *testing.T) {
	form := widget.CallerForm{Enabled: true, Name: widget.FieldRequired, Phone: widget.FieldOptional, Email: widget.FieldHidden}

	in := CreateInput{Name: "Ada", Email: "ada@example.com", Consent: true}
	if err := in.ApplyForm(form); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if in.Email != "" {
		t.Errorf("hidden field kept: %q", in.Email)
	}

	in = CreateInput{Consent: true}
	err := in.ApplyForm(form)
	if e, ok := apperr.As(err); !ok || e.Fields["name"] == "" {
		t.Errorf("missing required name not reported: %v", err)
	}

	in = CreateInput{Name: "Ada"}
	if err := in.ApplyForm(form); err != errConsentRequired {
		t.Errorf("missing consent: got %v", err)
	}

	disabled := widget.CallerForm{Enabled: false, Name: widget.FieldRequired, Phone: widget.FieldRequired, Email: widget.FieldRequired}
	in = CreateInput{Phone: "+2348012345678", Consent: true}
	if err := in.ApplyForm(disabled); err != nil {
		t.Errorf("disabled form must not require fields: %v", err)
	}
	if in.Phone != "" {
		t.Errorf("disabled form must discard fields, kept phone %q", in.Phone)
	}
}

func TestCreateInputValidate(t *testing.T) {
	for _, phone := range []string{"+234 801 234 5678", "(080) 1234-5678"} {
		in := CreateInput{Phone: phone}
		if err := in.Validate(); err != nil {
			t.Errorf("phone %q rejected: %v", phone, err)
		}
	}
	for _, phone := range []string{"12345", "call me", "+1-800-FLOWERS"} {
		in := CreateInput{Phone: phone}
		if err := in.Validate(); err == nil {
			t.Errorf("phone %q accepted", phone)
		}
	}
	in := CreateInput{Email: "not-an-email"}
	if err := in.Validate(); err == nil {
		t.Error("invalid email accepted")
	}
}
