package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword("correct horse battery", hash); err != nil || !ok {
		t.Fatalf("correct password rejected: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword("wrong", hash); ok {
		t.Fatal("wrong password accepted")
	}
}

func TestHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("two hashes of the same password are identical")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	for _, h := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA", "$argon2id$v=19$m=x$a$b"} {
		if _, err := VerifyPassword("x", h); err == nil {
			t.Errorf("malformed hash %q accepted", h)
		}
	}
}

func TestCheckCanSignIn(t *testing.T) {
	active, suspended, pending := TenantStatusActive, TenantStatusSuspended, TenantStatusPending
	now := time.Now()
	tests := []struct {
		name string
		c    credentials
		want error
	}{
		{"active verified", credentials{Status: StatusActive, Role: "agent", EmailVerifiedAt: &now, TenantStatus: &active}, nil},
		{"disabled", credentials{Status: StatusDisabled, Role: "agent", EmailVerifiedAt: &now, TenantStatus: &active}, errAccountDisabled},
		{"unverified", credentials{Status: StatusActive, Role: "tenant_admin", TenantStatus: &pending}, errEmailNotVerified},
		{"suspended tenant", credentials{Status: StatusActive, Role: "agent", EmailVerifiedAt: &now, TenantStatus: &suspended}, errTenantSuspended},
		{"platform admin needs no tenant", credentials{Status: StatusActive, Role: "platform_admin"}, nil},
	}
	for _, tt := range tests {
		if got := checkCanSignIn(&tt.c); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
