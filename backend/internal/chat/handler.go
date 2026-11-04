package chat

import (
	"github.com/google/uuid"

	"net/http"

	"callcenter/internal/call"
	"callcenter/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func callID(w http.ResponseWriter, r *http.Request) (id uuid.UUID, ok bool) {
	parsed, err := httpx.PathUUID(r, "id", "Call")
	if err != nil {
		httpx.Error(w, r, err)
		return id, false
	}
	return parsed, true
}

// CallerSend handles POST /v1/public/calls/{id}/messages.
func (h *Handler) CallerSend(w http.ResponseWriter, r *http.Request) {
	id, ok := callID(w, r)
	if !ok {
		return
	}
	var in Input
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	msg, err := h.svc.SendAsCaller(r.Context(), id, r.Header.Get(call.CallerTokenHeader), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, msg)
}

// CallerList handles GET /v1/public/calls/{id}/messages.
func (h *Handler) CallerList(w http.ResponseWriter, r *http.Request) {
	id, ok := callID(w, r)
	if !ok {
		return
	}
	msgs, err := h.svc.CallerMessages(r.Context(), id, r.Header.Get(call.CallerTokenHeader))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, msgs)
}

// AgentSend handles POST /v1/agent/calls/{id}/messages.
func (h *Handler) AgentSend(w http.ResponseWriter, r *http.Request) {
	id, ok := callID(w, r)
	if !ok {
		return
	}
	var in Input
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	msg, err := h.svc.SendAsAgent(r.Context(), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, msg)
}

// AgentList handles GET /v1/agent/calls/{id}/messages.
func (h *Handler) AgentList(w http.ResponseWriter, r *http.Request) {
	id, ok := callID(w, r)
	if !ok {
		return
	}
	msgs, err := h.svc.AgentMessages(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, msgs)
}
