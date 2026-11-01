package user

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"callcenter/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := httpx.PageFromQuery(r)
	users, total, err := h.svc.List(r.Context(), FilterFromQuery(strings.TrimSpace(q.Get("q")), q.Get("role"), q.Get("status")), page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(users, page, total))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "User")
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
	id, err := httpx.PathUUID(r, "id", "User")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in UpdateInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) Disable(w http.ResponseWriter, r *http.Request) { h.action(w, r, h.svc.Disable) }
func (h *Handler) Enable(w http.ResponseWriter, r *http.Request)  { h.action(w, r, h.svc.Enable) }
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request)  { h.action(w, r, h.svc.Delete) }

func (h *Handler) ResendInvite(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "User")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.ResendInvite(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) action(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, id uuid.UUID) error) {
	id, err := httpx.PathUUID(r, "id", "User")
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
