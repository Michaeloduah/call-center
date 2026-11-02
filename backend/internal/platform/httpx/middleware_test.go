package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"callcenter/internal/platform/reqctx"
)

func TestClientMetaTrustsForwardedForOnlyFromTrustedProxy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	var got netip.Addr
	h := ClientMeta(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = reqctx.MetaFrom(r.Context()).IP
	}))

	tests := []struct {
		name, remote, xff, want string
	}{
		{"trusted proxy forwards client ip", "127.0.0.1:5000", "203.0.113.7", "203.0.113.7"},
		{"spoofed left-most entry is ignored", "127.0.0.1:5000", "6.6.6.6, 203.0.113.7", "203.0.113.7"},
		{"untrusted peer cannot spoof", "198.51.100.1:5000", "203.0.113.7", "198.51.100.1"},
		{"no header uses peer", "127.0.0.1:5000", "", "127.0.0.1"},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tt.remote
		if tt.xff != "" {
			r.Header.Set("X-Forwarded-For", tt.xff)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if got.String() != tt.want {
			t.Errorf("%s: ip = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewRateLimiter(2, time.Minute)
	l.now = func() time.Time { return now }
	if !l.Allow("k") || !l.Allow("k") {
		t.Fatal("first two attempts should be allowed")
	}
	if l.Allow("k") {
		t.Fatal("third attempt should be blocked")
	}
	if !l.Allow("other") {
		t.Fatal("keys must be independent")
	}
	now = now.Add(time.Minute + time.Second)
	if !l.Allow("k") {
		t.Fatal("limit should reset after the window")
	}
}
