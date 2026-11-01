// Package apperr defines transport-agnostic application errors.
// Services return these; the HTTP layer maps them to status codes.
package apperr

import (
	"errors"
	"fmt"
)

type Kind int

const (
	KindValidation Kind = iota + 1
	KindNotFound
	KindConflict
	KindUnauthenticated
	KindForbidden
	KindTooManyRequests
)

// Error is a typed application error with a stable machine-readable code.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Fields  map[string]string // per-field validation messages
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func Validation(fields map[string]string) *Error {
	return &Error{Kind: KindValidation, Code: "validation_failed", Message: "Please correct the highlighted fields.", Fields: fields}
}

func NotFound(entity string) *Error {
	return &Error{Kind: KindNotFound, Code: "not_found", Message: entity + " not found."}
}

func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

func Unauthenticated(code, message string) *Error {
	return &Error{Kind: KindUnauthenticated, Code: code, Message: message}
}

func Forbidden(message string) *Error {
	return &Error{Kind: KindForbidden, Code: "forbidden", Message: message}
}

func TooManyRequests() *Error {
	return &Error{Kind: KindTooManyRequests, Code: "too_many_requests", Message: "Too many attempts. Please wait and try again."}
}

// FieldConflict is a conflict tied to a single form field (e.g. an email already in use).
func FieldConflict(field, message string) *Error {
	return &Error{Kind: KindConflict, Code: "conflict", Message: message, Fields: map[string]string{field: message}}
}

func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
