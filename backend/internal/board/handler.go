package board

import (
	"net/http"

	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/realtime"
)

type Handler struct {
	svc *Service
	hub *realtime.Hub
}

func NewHandler(svc *Service, hub *realtime.Hub) *Handler { return &Handler{svc: svc, hub: hub} }

// Board handles GET /v1/live/board.
func (h *Handler) Board(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.Get(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, b)
}

// Events handles GET /v1/live/events (Server-Sent Events) for the user's visible departments.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	p, err := reqctx.TenantPrincipal(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	topics, err := h.svc.Topics(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.hub.Stream(w, r, p.UserID, topics); err != nil {
		httpx.Error(w, r, err)
	}
}
