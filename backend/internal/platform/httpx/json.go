// Package httpx contains HTTP transport helpers shared by all handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"callcenter/internal/platform/apperr"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// Decode parses a JSON request body into dst, rejecting unknown fields and trailing data.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		return badRequest("Content-Type must be application/json.")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &maxErr):
			return badRequest("Request body is too large.")
		case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
			return badRequest("Request body is not valid JSON.")
		case errors.As(err, &typeErr):
			return apperr.Validation(map[string]string{typeErr.Field: "Has the wrong type."})
		case errors.Is(err, io.EOF):
			return badRequest("Request body is empty.")
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			return badRequest(fmt.Sprintf("Unknown field %s.", strings.TrimPrefix(err.Error(), "json: unknown field ")))
		default:
			return badRequest("Request body could not be read.")
		}
	}
	if dec.More() {
		return badRequest("Request body must contain a single JSON object.")
	}
	return nil
}

// Validatable is implemented by every request DTO.
type Validatable interface{ Validate() error }

// DecodeValid decodes a JSON body into dst and then validates it.
func DecodeValid(w http.ResponseWriter, r *http.Request, dst Validatable) error {
	if err := Decode(w, r, dst); err != nil {
		return err
	}
	return dst.Validate()
}

func badRequest(msg string) error {
	return &apperr.Error{Kind: apperr.KindValidation, Code: "bad_request", Message: msg}
}

// JSON writes v with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "error", err)
	}
}

func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

var statusByKind = map[apperr.Kind]int{
	apperr.KindValidation:      http.StatusUnprocessableEntity,
	apperr.KindNotFound:        http.StatusNotFound,
	apperr.KindConflict:        http.StatusConflict,
	apperr.KindUnauthenticated: http.StatusUnauthorized,
	apperr.KindForbidden:       http.StatusForbidden,
	apperr.KindTooManyRequests: http.StatusTooManyRequests,
}

// Error writes err as a JSON error response. Unknown errors are logged and hidden from the client.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	if appErr, ok := apperr.As(err); ok {
		status := statusByKind[appErr.Kind]
		if appErr.Code == "bad_request" {
			status = http.StatusBadRequest
		}
		JSON(w, status, errorBody{Error: errorDetail{Code: appErr.Code, Message: appErr.Message, Fields: appErr.Fields}})
		return
	}
	slog.ErrorContext(r.Context(), "unhandled error", "error", err, "method", r.Method, "path", r.URL.Path)
	JSON(w, http.StatusInternalServerError, errorBody{Error: errorDetail{Code: "internal_error", Message: "Something went wrong. Please try again."}})
}
