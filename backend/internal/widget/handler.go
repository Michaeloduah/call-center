package widget

import (
	"net/http"
	"regexp"

	"callcenter/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page := httpx.PageFromQuery(r)
	items, total, err := h.svc.List(r.Context(), FilterFromQuery(r.URL.Query().Get("q")), page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(items, page, total))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Widget")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	respond(w, r, http.StatusOK, out, err)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.Create(r.Context(), in)
	respond(w, r, http.StatusCreated, out, err)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Widget")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in Input
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.Update(r.Context(), id, in)
	respond(w, r, http.StatusOK, out, err)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Widget")
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

var publicKeyPattern = regexp.MustCompile(`^wk_[A-Za-z0-9_-]{22}$`)

// Public handles GET /v1/public/widgets/{key}. Unauthenticated.
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !publicKeyPattern.MatchString(key) {
		httpx.Error(w, r, errNotFound)
		return
	}
	out, err := h.svc.Public(r.Context(), key)
	respond(w, r, http.StatusOK, out, err)
}

// ValidPublicKey reports whether key has the shape of a widget public key.
func ValidPublicKey(key string) bool { return publicKeyPattern.MatchString(key) }

func respond(w http.ResponseWriter, r *http.Request, status int, out any, err error) {
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, status, out)
}
