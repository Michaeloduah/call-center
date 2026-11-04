package recording

import (
	"net/http"

	"callcenter/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// AccessInput is the body of the access endpoints.
type AccessInput struct {
	Download bool `json:"download"`
}

func (*AccessInput) Validate() error { return nil }

func writeGrant(w http.ResponseWriter, grant *AccessGrant) {
	// The link is a bearer credential: it must never be cached.
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, grant)
}

// Access handles POST /v1/calls/{id}/recording/access.
func (h *Handler) Access(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Recording")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in AccessInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	grant, err := h.svc.Access(r.Context(), id, in.Download)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeGrant(w, grant)
}

// Delete handles DELETE /v1/calls/{id}/recording.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Recording")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ListDeleted handles GET /v1/platform/recordings/deleted.
func (h *Handler) ListDeleted(w http.ResponseWriter, r *http.Request) {
	page := httpx.PageFromQuery(r)
	items, total, err := h.svc.ListDeleted(r.Context(), page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(items, page, total))
}

// PlatformAccess handles POST /v1/platform/recordings/{id}/access.
func (h *Handler) PlatformAccess(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Recording")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in AccessInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	grant, err := h.svc.PlatformAccess(r.Context(), id, in.Download)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeGrant(w, grant)
}

// Restore handles POST /v1/platform/recordings/{id}/restore.
func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Recording")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Restore(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
