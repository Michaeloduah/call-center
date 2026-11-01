package department

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"callcenter/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page := httpx.PageFromQuery(r)
	items, total, err := h.svc.List(r.Context(), FilterFromQuery(r.URL.Query().Get("q"), r.URL.Query().Get("active")), page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(items, page, total))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Department")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	d, err := h.svc.Get(r.Context(), id)
	respond(w, r, http.StatusOK, d, err)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	d, err := h.svc.Create(r.Context(), in)
	respond(w, r, http.StatusCreated, d, err)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	withBody(w, r, func(ctx context.Context, id uuid.UUID, in *Input) (any, error) { return h.svc.Update(ctx, id, *in) })
}

func (h *Handler) SetMembers(w http.ResponseWriter, r *http.Request) {
	withBody(w, r, func(ctx context.Context, id uuid.UUID, in *UsersInput) (any, error) {
		return h.svc.SetMembers(ctx, id, *in)
	})
}

func (h *Handler) SetSupervisors(w http.ResponseWriter, r *http.Request) {
	withBody(w, r, func(ctx context.Context, id uuid.UUID, in *UsersInput) (any, error) {
		return h.svc.SetSupervisors(ctx, id, *in)
	})
}

func (h *Handler) SetBusinessHours(w http.ResponseWriter, r *http.Request) {
	withBody(w, r, func(ctx context.Context, id uuid.UUID, in *HoursInput) (any, error) {
		return h.svc.SetBusinessHours(ctx, id, *in)
	})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "Department")
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

// withBody parses the department id and a validated JSON body of type T, then calls fn.
func withBody[T any, PT interface {
	*T
	httpx.Validatable
}](w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, id uuid.UUID, in *T) (any, error)) {
	id, err := httpx.PathUUID(r, "id", "Department")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := new(T)
	if err := httpx.DecodeValid(w, r, PT(in)); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := fn(r.Context(), id, in)
	respond(w, r, http.StatusOK, out, err)
}

func respond(w http.ResponseWriter, r *http.Request, status int, out any, err error) {
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, status, out)
}
