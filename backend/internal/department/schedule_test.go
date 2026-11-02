package department

import (
	"testing"
	"time"
)

func TestIsOpen(t *testing.T) {
	weekdays := []HoursRange{{1, "09:00", "17:00"}, {6, "00:00", "24:00"}}
	lagos := func(s string) time.Time {
		loc, _ := time.LoadLocation("Africa/Lagos")
		tm, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"monday morning", lagos("2026-11-02 09:00"), true},
		{"monday closing minute is closed", lagos("2026-11-02 17:00"), false},
		{"monday before opening", lagos("2026-11-02 08:59"), false},
		{"sunday", lagos("2026-11-01 12:00"), false},
		{"saturday late night (24:00 close)", lagos("2026-11-07 23:59"), true},
		// 08:30 UTC is 09:30 in Lagos (UTC+1): evaluated in the department's zone, not the server's.
		{"utc instant converted to department zone", time.Date(2026, 11, 2, 8, 30, 0, 0, time.UTC), true},
	}
	for _, tt := range tests {
		got, err := IsOpen("Africa/Lagos", weekdays, tt.at)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
	if open, _ := IsOpen("UTC", nil, time.Now()); open {
		t.Error("empty schedule must be closed")
	}
}
