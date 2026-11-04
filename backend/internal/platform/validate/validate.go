// Package validate provides small helpers for explicit request validation.
package validate

import (
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"callcenter/internal/platform/apperr"
)

// Errors collects per-field messages. The first message recorded for a field wins.
type Errors map[string]string

func (e Errors) Add(field, msg string) {
	if _, exists := e[field]; !exists {
		e[field] = msg
	}
}

// Err returns a validation error, or nil when nothing was recorded.
func (e Errors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return apperr.Validation(e)
}

// RequiredString trims s and checks it is non-empty and within maxLen runes.
func (e Errors) RequiredString(field, s string, maxLen int) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		e.Add(field, "This field is required.")
	case utf8.RuneCountInString(s) > maxLen:
		e.Add(field, "Must be at most "+itoa(maxLen)+" characters.")
	}
	return s
}

// OptionalString trims s and checks it is within maxLen runes.
func (e Errors) OptionalString(field, s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxLen {
		e.Add(field, "Must be at most "+itoa(maxLen)+" characters.")
	}
	return s
}

const maxEmailLen = 254

// Email normalises and validates an email address.
func (e Errors) Email(field, s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		e.Add(field, "This field is required.")
		return s
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || len(s) > maxEmailLen {
		e.Add(field, "Enter a valid email address.")
	}
	return s
}

const (
	minPasswordLen = 10
	maxPasswordLen = 128 // bounds hashing cost
)

// Password checks length bounds. It does not trim: whitespace is significant.
func (e Errors) Password(field, s string) {
	n := utf8.RuneCountInString(s)
	switch {
	case n < minPasswordLen:
		e.Add(field, "Must be at least "+itoa(minPasswordLen)+" characters.")
	case n > maxPasswordLen:
		e.Add(field, "Must be at most "+itoa(maxPasswordLen)+" characters.")
	}
}

func (e Errors) IntRange(field string, v, min, max int) {
	if v < min || v > max {
		e.Add(field, "Must be between "+itoa(min)+" and "+itoa(max)+".")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

var (
	phoneChars  = regexp.MustCompile(`^\+?[0-9 ()\-.]{6,32}$`)
	phoneDigits = regexp.MustCompile(`[0-9]`)
)

const minPhoneDigits = 6

// OptionalPhone trims s and checks it looks like a phone number. Empty is allowed.
func (e Errors) OptionalPhone(field, s string) string {
	s = strings.TrimSpace(s)
	if s != "" && (!phoneChars.MatchString(s) || len(phoneDigits.FindAllString(s, -1)) < minPhoneDigits) {
		e.Add(field, "Enter a valid phone number.")
	}
	return s
}

// OptionalEmail normalises and validates s when it is not empty.
func (e Errors) OptionalEmail(field, s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return e.Email(field, s)
}
