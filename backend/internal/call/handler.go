package call

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"callcenter/internal/media"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/widget"
)

// CallerTokenHeader carries the caller's call-scoped token on public call endpoints.
const CallerTokenHeader = "X-Caller-Token"

type Handler struct {
	svc         *Service
	events      media.Listener
	media       media.Provider
	createLimit *httpx.RateLimiter
	logger      *slog.Logger
}

// NewHandler builds the call endpoints. Verified media webhooks are delivered to events.
func NewHandler(svc *Service, events media.Listener, mp media.Provider, createLimit *httpx.RateLimiter, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, events: events, media: mp, createLimit: createLimit, logger: logger}
}

// NewCreateLimiter bounds how many calls one IP can start.
func NewCreateLimiter() *httpx.RateLimiter { return httpx.NewRateLimiter(10, 10*time.Minute) }

// Create handles POST /v1/public/widgets/{key}/calls.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !widget.ValidPublicKey(key) {
		httpx.Error(w, r, errWidgetNotFound)
		return
	}
	if !h.createLimit.Allow(reqctx.MetaFrom(r.Context()).IP.String()) {
		httpx.Error(w, r, apperr.TooManyRequests())
		return
	}
	var in CreateInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Create(r.Context(), key, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

// Status handles GET /v1/public/calls/{id}.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Call")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	view, err := h.svc.Status(r.Context(), id, r.Header.Get(CallerTokenHeader))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

// HangUp handles POST /v1/public/calls/{id}/hangup.
func (h *Handler) HangUp(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Call")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.HangUp(r.Context(), id, r.Header.Get(CallerTokenHeader)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// AgentEnd handles POST /v1/agent/calls/{id}/end.
func (h *Handler) AgentEnd(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Call")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.AgentEnd(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// MediaWebhook handles POST /v1/webhooks/livekit. The signature is verified before anything is read.
func (h *Handler) MediaWebhook(w http.ResponseWriter, r *http.Request) {
	ev, err := h.media.ParseWebhook(r)
	if errors.Is(err, media.ErrInvalidWebhook) {
		h.logger.Warn("rejected media webhook", "error", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.events.HandleMediaEvent(r.Context(), ev); err != nil {
		// A 5xx makes LiveKit retry the delivery.
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
