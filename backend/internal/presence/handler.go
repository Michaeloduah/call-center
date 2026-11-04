package presence

import (
	"net/http"

	"callcenter/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// SetStatusInput is the request body of PUT /v1/agent/presence.
type SetStatusInput struct {
	Status Status `json:"status"`
}

func (in *SetStatusInput) Validate() error {
	if !in.Status.Valid() {
		return errStatus
	}
	return nil
}

// SetStatus handles PUT /v1/agent/presence.
func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	var in SetStatusInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.SetStatus(r.Context(), in.Status); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// Heartbeat handles POST /v1/agent/presence/heartbeat.
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.Heartbeat(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, SetStatusInput{Status: status})
}
