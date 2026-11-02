package settings

import (
	"net/http"

	"callcenter/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetPlatform(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetPlatform(r.Context())
	respond(w, r, out, err)
}

func (h *Handler) UpdatePlatform(w http.ResponseWriter, r *http.Request) {
	var in UpdatePlatformInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.UpdatePlatform(r.Context(), in)
	respond(w, r, out, err)
}

func (h *Handler) GetTenant(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetTenant(r.Context())
	respond(w, r, out, err)
}

func (h *Handler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	var in UpdateTenantInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.UpdateTenant(r.Context(), in)
	respond(w, r, out, err)
}

func respond(w http.ResponseWriter, r *http.Request, out any, err error) {
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
