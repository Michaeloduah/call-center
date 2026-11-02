package widget

import (
	"testing"

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
