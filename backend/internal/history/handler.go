package history

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

const dateLayout = "2006-01-02"

// parseFilter reads the list filters. Dates are calendar days in UTC; "to" is inclusive.
func parseFilter(r *http.Request) (Filter, error) {
	query := r.URL.Query()
	f := Filter{Status: query.Get("status"), Query: strings.TrimSpace(query.Get("q"))}
	fields := map[string]string{}
	parseID := func(name string, dst *uuid.UUID) {
		if raw := query.Get(name); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				fields[name] = "Unknown value."
				return
			}
			*dst = id
		}
	}
	parseID("department_id", &f.DepartmentID)
	parseID("agent_id", &f.AgentID)
	parseDay := func(name string, plusDays int) *time.Time {
		raw := query.Get(name)
		if raw == "" {
			return nil
		}
		t, err := time.Parse(dateLayout, raw)
		if err != nil {
			fields[name] = "Use the format YYYY-MM-DD."
			return nil
		}
		t = t.AddDate(0, 0, plusDays)
		return &t
	}
	f.From, f.To = parseDay("from", 0), parseDay("to", 1)
	if len(fields) > 0 {
		return f, apperr.Validation(fields)
	}
	return f, nil
}

// List handles GET /v1/calls.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page := httpx.PageFromQuery(r)
	items, total, err := h.svc.List(r.Context(), f, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(items, page, total))
}

// Get handles GET /v1/calls/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Call")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	d, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}
