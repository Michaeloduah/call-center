package tenant

import "testing"

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Acme Ltd":            "acme-ltd",
		"  Café  Déjà Vu!! ":  "cafe-deja-vu",
		"A&B":                 "a-b",
		"日本":                  "org",
		"ab":                  "org-ab",
		"Hello---World__2026": "hello-world-2026",
	}
	for in, want := range tests {
		got := Slugify(in)
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
		if !ValidSlug(got) {
			t.Errorf("Slugify(%q) = %q is not a valid slug", in, got)
		}
	}
}

func TestSlugifyBoundsLengthWithRoomForSuffix(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "a"
	}
	s := withRandomSuffix(Slugify(long))
	if !ValidSlug(s) {
		t.Fatalf("suffixed slug %q (len %d) is invalid", s, len(s))
	}
}
