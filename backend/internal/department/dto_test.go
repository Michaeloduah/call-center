package department

import (
	"testing"

	"callcenter/internal/platform/apperr"
)

func TestHoursValidation(t *testing.T) {
	tests := []struct {
		name      string
		hours     []HoursRange
		wantField string // empty = valid
	}{
		{"valid split day", []HoursRange{{1, "09:00", "12:00"}, {1, "13:00", "17:00"}}, ""},
		{"touching ranges are allowed", []HoursRange{{1, "09:00", "12:00"}, {1, "12:00", "17:00"}}, ""},
		{"end of day", []HoursRange{{6, "00:00", "24:00"}}, ""},
		{"empty schedule", nil, ""},
		{"overlap", []HoursRange{{1, "09:00", "12:00"}, {1, "11:00", "17:00"}}, "business_hours.1"},
		{"open after close", []HoursRange{{2, "17:00", "09:00"}}, "business_hours.0"},
		{"24:00 cannot open", []HoursRange{{2, "24:00", "24:00"}}, "business_hours.0"},
		{"24:30 invalid", []HoursRange{{2, "09:00", "24:30"}}, "business_hours.0"},
		{"bad weekday", []HoursRange{{7, "09:00", "17:00"}}, "business_hours.0"},
		{"bad format", []HoursRange{{1, "9am", "5pm"}}, "business_hours.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := HoursInput{Hours: tt.hours}
			err := in.Validate()
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			e, ok := apperr.As(err)
			if !ok {
				t.Fatalf("expected validation error, got %v", err)
			}
			if _, has := e.Fields[tt.wantField]; !has {
				t.Fatalf("expected error on %s, got %v", tt.wantField, e.Fields)
			}
		})
	}
}

func TestHoursValidationNormalisesTimes(t *testing.T) {
	in := HoursInput{Hours: []HoursRange{{1, " 09:00 ", "17:30"}}}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
	if in.Hours[0].Opens != "09:00" {
		t.Fatalf("opens = %q, want 09:00", in.Hours[0].Opens)
	}
}

func TestInputRejectsUnknownTimezone(t *testing.T) {
	active := true
	for _, tz := range []string{"Mars/Base", "Local"} {
		in := Input{Name: "Support", Timezone: tz, IsActive: &active}
		if err := in.Validate(); err == nil {
			t.Errorf("timezone %q accepted", tz)
		}
	}
	in := Input{Name: "Support", Timezone: "Africa/Lagos", IsActive: &active}
	if err := in.Validate(); err != nil {
		t.Errorf("valid timezone rejected: %v", err)
	}
}
