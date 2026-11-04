package widget

import (
	"testing"
	"time"

	"callcenter/internal/department"
	"callcenter/internal/platform/validate"
)

func TestNormaliseOrigin(t *testing.T) {
	valid := map[string]string{
		"https://Example.com":           "https://example.com",
		"https://example.com/":          "https://example.com",
		"https://shop.example.com:8443": "https://shop.example.com:8443",
		"http://localhost:5500":         "http://localhost:5500",
		"http://127.0.0.1:8000":         "http://127.0.0.1:8000",
	}
	for in, want := range valid {
		got, ok := normaliseOrigin(in)
		if !ok || got != want {
			t.Errorf("normaliseOrigin(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"http://example.com",       // plain http outside local dev
		"https://example.com/path", // not an origin
		"https://example.com?x=1",  //
		"https://user@example.com", //
		"https://*.example.com",    // wildcards are not supported
		"example.com",              // missing scheme
		"javascript:alert(1)",      //
		"ftp://example.com",        //
	} {
		if got, ok := normaliseOrigin(in); ok {
			t.Errorf("normaliseOrigin(%q) accepted as %q", in, got)
		}
	}
}

func TestValidateOriginsDeduplicatesAndRequiresOne(t *testing.T) {
	v := validate.Errors{}
	got := validateOrigins(v, []string{"https://a.com", " https://A.com/ ", ""})
	if len(v) != 0 || len(got) != 1 {
		t.Fatalf("got %v errors=%v", got, v)
	}
	v = validate.Errors{}
	validateOrigins(v, []string{" "})
	if v["allowed_origins"] == "" {
		t.Fatal("empty origin list accepted")
	}
}

func TestAvailabilityOf(t *testing.T) {
	allWeek := make([]department.HoursRange, 0, 7)
	for d := 0; d < 7; d++ {
		allWeek = append(allWeek, department.HoursRange{Weekday: d, Opens: "00:00", Closes: "24:00"})
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	base := Resolved{DepartmentActive: true, Timezone: "UTC", Hours: allWeek, AgentsAvailable: true}

	tests := []struct {
		name   string
		mutate func(*Resolved)
		want   Availability
	}{
		{"open", func(*Resolved) {}, AvailabilityOpen},
		{"department deactivated", func(r *Resolved) { r.DepartmentActive = false }, AvailabilityUnavailable},
		{"outside business hours", func(r *Resolved) { r.Hours = nil }, AvailabilityClosed},
		{"open but nobody online", func(r *Resolved) { r.AgentsAvailable = false }, AvailabilityNoAgents},
		{"closed wins over nobody online", func(r *Resolved) { r.Hours = nil; r.AgentsAvailable = false }, AvailabilityClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.mutate(&r)
			got, err := AvailabilityOf(&r, now)
			if err != nil || got != tt.want {
				t.Fatalf("AvailabilityOf = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
