package presence

import "testing"

func TestEffective(t *testing.T) {
	tests := []struct {
		name                 string
		status               Status
		fresh, offer, onCall bool
		want                 State
	}{
		{"offline", StatusOffline, true, false, false, StateOffline},
		{"stale heartbeat overrides the stored choice", StatusAvailable, false, false, false, StateOffline},
		{"away", StatusAway, true, false, false, StateAway},
		{"away stays away even with a pending offer", StatusAway, true, true, false, StateAway},
		{"idle", StatusAvailable, true, false, false, StateIdle},
		{"ringing", StatusAvailable, true, true, false, StateRinging},
		{"on call", StatusAvailable, true, false, true, StateOnCall},
		{"on call wins over ringing", StatusAvailable, true, true, true, StateOnCall},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Effective(tt.status, tt.fresh, tt.offer, tt.onCall); got != tt.want {
				t.Fatalf("Effective = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusOffline, StatusAvailable, StatusAway} {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []Status{"", "busy", "AVAILABLE", "on_call"} {
		if s.Valid() {
			t.Errorf("%q should be invalid (busy and on_call are derived, never chosen)", s)
		}
	}
}
