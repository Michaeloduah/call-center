package callback

import (
	"net/http"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/widget"
)

type Handler struct {
	svc          *Service
	createLimits *httpx.RateLimiter
}

func NewHandler(svc *Service, createLimits *httpx.RateLimiter) *Handler {
	return &Handler{svc: svc, createLimits: createLimits}
}

// NewCreateLimiter bounds how many callback requests one IP can create.
func NewCreateLimiter() *httpx.RateLimiter {
	return httpx.NewRateLimiter(createsPerWindow, createWindow)
}

type createdResponse struct {
	ID uuid.UUID `json:"id"`
}

func (h *Handler) allowed(w http.ResponseWriter, r *http.Request) bool {
	if h.createLimits.Allow(reqctx.MetaFrom(r.Context()).IP.String()) {
		return true
	}
	httpx.Error(w, r, apperr.TooManyRequests())
	return false
}

// CreateForWidget handles POST /v1/public/widgets/{key}/callbacks.
func (h *Handler) CreateForWidget(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !widget.ValidPublicKey(key) {
		httpx.Error(w, r, errWidget)
		return
	}
	if !h.allowed(w, r) {
		return
	}
	var in CreateInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	id, err := h.svc.CreateForWidget(r.Context(), key, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, createdResponse{ID: id})
}

// CreateForCall handles POST /v1/public/calls/{id}/callback.
func (h *Handler) CreateForCall(w http.ResponseWriter, r *http.Request) {
	callID, err := httpx.PathUUID(r, "id", "Call")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if !h.allowed(w, r) {
		return
	}
	var in CreateInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	id, err := h.svc.CreateForCall(r.Context(), callID, r.Header.Get(call.CallerTokenHeader), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, createdResponse{ID: id})
}

// List handles GET /v1/callbacks.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f := Filter{Status: Status(r.URL.Query().Get("status"))}
	if raw := r.URL.Query().Get("department_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.Error(w, r, apperr.Validation(map[string]string{"department_id": "Unknown department."}))
			return
		}
		f.DepartmentID = id
	}
	page := httpx.PageFromQuery(r)
	items, total, err := h.svc.List(r.Context(), f, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(items, page, total))
}

// Get handles GET /v1/callbacks/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Callback")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	cb, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cb)
}

// transition adapts a workflow method without a body to a handler.
func transition(do func(*Service, *http.Request, uuid.UUID) error, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := httpx.PathUUID(r, "id", "Callback")
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := do(svc, r, id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.NoContent(w)
	}
}

func (h *Handler) Claim(w http.ResponseWriter, r *http.Request) {
	transition(func(s *Service, r *http.Request, id uuid.UUID) error { return s.Claim(r.Context(), id) }, h.svc)(w, r)
}

func (h *Handler) Release(w http.ResponseWriter, r *http.Request) {
	transition(func(s *Service, r *http.Request, id uuid.UUID) error { return s.Release(r.Context(), id) }, h.svc)(w, r)
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	transition(func(s *Service, r *http.Request, id uuid.UUID) error { return s.Cancel(r.Context(), id) }, h.svc)(w, r)
}

// Complete handles POST /v1/callbacks/{id}/complete.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Callback")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in CompleteInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Complete(r.Context(), id, in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// Assign handles POST /v1/admin/callbacks/{id}/assign.
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Callback")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in AssignInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Assign(r.Context(), id, in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
