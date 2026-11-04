package billing

import (
	"errors"
	"net/http"

	"callcenter/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.ListPlans(r.Context())
	respond(w, r, plans, err)
}

func (h *Handler) GetPlan(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Plan")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.GetPlan(r.Context(), id)
	respond(w, r, p, err)
}

func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	var b PlanBody
	if err := httpx.DecodeValid(w, r, &b); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.CreatePlan(r.Context(), b)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, p)
}

func (h *Handler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Plan")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var b PlanBody
	if err := httpx.DecodeValid(w, r, &b); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.UpdatePlan(r.Context(), id, b)
	respond(w, r, p, err)
}

func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Plan")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	noContent(w, r, h.svc.DeletePlan(r.Context(), id))
}

func (h *Handler) CreatePrice(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Plan")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var b PriceBody
	if err := httpx.DecodeValid(w, r, &b); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.CreatePrice(r.Context(), id, b)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, p)
}

func (h *Handler) UpdatePrice(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Price")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var b PriceUpdateBody
	if err := httpx.DecodeValid(w, r, &b); err != nil {
		httpx.Error(w, r, err)
		return
	}
	noContent(w, r, h.svc.UpdatePrice(r.Context(), id, b))
}

func (h *Handler) DeletePrice(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Price")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	noContent(w, r, h.svc.DeletePrice(r.Context(), id))
}

func (h *Handler) SyncPrice(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Price")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	noContent(w, r, h.svc.SyncPrice(r.Context(), id))
}

func (h *Handler) PlatformTenantBilling(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Tenant")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.svc.PlatformTenantBilling(r.Context(), id)
	respond(w, r, b, err)
}

func (h *Handler) Plans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.Plans(r.Context())
	respond(w, r, plans, err)
}

func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	o, err := h.svc.Overview(r.Context())
	respond(w, r, o, err)
}

func (h *Handler) SetPolicies(w http.ResponseWriter, r *http.Request) {
	var b PoliciesBody
	if err := httpx.DecodeValid(w, r, &b); err != nil {
		httpx.Error(w, r, err)
		return
	}
	noContent(w, r, h.svc.SetPolicies(r.Context(), b))
}

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	var b CheckoutBody
	if err := httpx.DecodeValid(w, r, &b); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Checkout(r.Context(), b)
	respond(w, r, res, err)
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	noContent(w, r, h.svc.Cancel(r.Context()))
}

// Webhook handles POST /v1/webhooks/{provider}. The provider's signature is verified before
// anything is read or stored. A 5xx makes the provider retry, which is safe because handling is idempotent.
func (h *Handler) Webhook(name ProviderName) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h.svc.HandleWebhook(r.Context(), name, r)
		if errors.Is(err, ErrInvalidWebhook) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func respond(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
