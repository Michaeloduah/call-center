package routing

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

// Session handles GET /v1/agent/session.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Session(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

// Accept handles POST /v1/agent/offers/{id}/accept.
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Offer")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Accept(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// Decline handles POST /v1/agent/offers/{id}/decline.
func (h *Handler) Decline(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Offer")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Decline(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// Media handles GET /v1/agent/calls/{id}/media.
func (h *Handler) Media(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Call")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	grant, err := h.svc.MediaToken(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, grant)
}

// Events handles GET /v1/agent/events (Server-Sent Events). The topic is derived from the
// session, never from the request.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	p, err := reqctx.TenantPrincipal(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.hub.Stream(w, r, p.UserID, []string{realtime.UserTopic(p.UserID)}); err != nil {
		httpx.Error(w, r, err)
	}
}
