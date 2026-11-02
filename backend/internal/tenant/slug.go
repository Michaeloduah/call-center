package tenant

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	minSlugLen      = 3
	maxSlugBaseLen  = 50 // leaves room for a "-xxxxxx" uniqueness suffix within the 60 limit
	fallbackSlug    = "org"
	slugSuffixBytes = 3
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidSlug reports whether s satisfies the database slug constraint.
func ValidSlug(s string) bool {
	return len(s) >= minSlugLen && len(s) <= 60 && slugPattern.MatchString(s)
}

// Slugify derives a URL-safe slug from an organisation name. Accents are stripped
// (é -> e); any other non-alphanumeric run becomes a single hyphen.
func Slugify(name string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range norm.NFKD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r): // combining accent marks
			continue
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			hyphen = false
		default:
			if !hyphen && b.Len() > 0 {
				b.WriteByte('-')
				hyphen = true
			}
		}
		if b.Len() >= maxSlugBaseLen {
			break
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) < minSlugLen {
		s = strings.Trim(fallbackSlug+"-"+s, "-")
	}
	return s
}

// withRandomSuffix appends a short random suffix used when the base slug is taken.
func withRandomSuffix(base string) string {
	b := make([]byte, slugSuffixBytes)
	_, _ = rand.Read(b)
	return base + "-" + hex.EncodeToString(b)
}
