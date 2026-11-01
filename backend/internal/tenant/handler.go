package tenant

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"callcenter/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page := httpx.PageFromQuery(r)
	items, total, err := h.svc.List(r.Context(), FilterFromQuery(r.URL.Query().Get("q"), r.URL.Query().Get("status")), page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(items, page, total))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Tenant")
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

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Tenant")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in UpdateInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) Suspend(w http.ResponseWriter, r *http.Request)  { h.action(w, r, h.svc.Suspend) }
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) { h.action(w, r, h.svc.Activate) }
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request)   { h.action(w, r, h.svc.Delete) }

func (h *Handler) action(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, id uuid.UUID) error) {
	id, err := httpx.PathUUID(r, "id", "Tenant")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := fn(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
