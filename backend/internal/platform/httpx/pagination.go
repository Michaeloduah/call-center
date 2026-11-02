package httpx

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
)

const (
	defaultPerPage = 20
	maxPerPage     = 100
)

// Page is a validated pagination request.
type Page struct {
	Number  int
	PerPage int
}

func (p Page) Offset() int { return (p.Number - 1) * p.PerPage }

// PageFromQuery reads ?page and ?per_page, falling back to safe defaults.
func PageFromQuery(r *http.Request) Page {
	p := Page{Number: 1, PerPage: defaultPerPage}
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		p.Number = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && n > 0 {
		p.PerPage = min(n, maxPerPage)
	}
	return p
}

// Paginated is the standard list response envelope.
type Paginated[T any] struct {
	Items   []T `json:"items"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

func NewPaginated[T any](items []T, page Page, total int) Paginated[T] {
	if items == nil {
		items = []T{} // serialise as [] rather than null
	}
	return Paginated[T]{Items: items, Page: page.Number, PerPage: page.PerPage, Total: total}
}

// PathUUID parses a UUID path parameter, returning a 404 for malformed IDs.
func PathUUID(r *http.Request, name, entity string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, apperr.NotFound(entity)
	}
	return id, nil
}
