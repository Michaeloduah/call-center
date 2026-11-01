package audit

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
)

// QueryService lists audit entries for platform admins.
type QueryService struct {
	tx database.TxManager
}

func NewQueryService(tx database.TxManager) *QueryService { return &QueryService{tx: tx} }

func (s *QueryService) List(ctx context.Context, f Filter, page httpx.Page) ([]LogEntry, int, error) {
	var (
		items []LogEntry
		total int
	)
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		q, err := database.Conn(ctx)
		if err != nil {
			return err
		}
		const where = `WHERE ($1::uuid IS NULL OR a.tenant_id = $1) AND ($2 = '' OR a.action = $2)`
		tenant := nullableUUID(f.TenantID)
		if err := q.QueryRow(ctx, `SELECT count(*) FROM audit_logs a `+where, tenant, f.Action).Scan(&total); err != nil {
			return fmt.Errorf("count audit logs: %w", err)
		}
		rows, err := q.Query(ctx, `
			SELECT a.id, a.tenant_id, t.name, a.actor_user_id, a.actor_email::text, a.action, a.entity_type,
			       a.entity_id, a.metadata, host(a.ip), a.created_at
			FROM audit_logs a
			LEFT JOIN tenants t ON t.id = a.tenant_id
			`+where+`
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3 OFFSET $4`, tenant, f.Action, page.PerPage, page.Offset())
		if err != nil {
			return fmt.Errorf("list audit logs: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var e LogEntry
			if err := rows.Scan(&e.ID, &e.TenantID, &e.TenantName, &e.ActorID, &e.ActorEmail, &e.Action,
				&e.EntityType, &e.EntityID, &e.Metadata, &e.IP, &e.CreatedAt); err != nil {
				return fmt.Errorf("scan audit log: %w", err)
			}
			items = append(items, e)
		}
		return rows.Err()
	})
	return items, total, err
}

// Handler exposes the platform audit log.
type Handler struct {
	svc *QueryService
}

func NewHandler(svc *QueryService) *Handler { return &Handler{svc: svc} }

// List handles GET /v1/platform/audit-logs?tenant_id=&action=&page=&per_page=
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	var f Filter
	if raw := r.URL.Query().Get("tenant_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.JSON(w, http.StatusOK, httpx.NewPaginated[LogEntry](nil, httpx.PageFromQuery(r), 0))
			return
		}
		f.TenantID = id
	}
	f.Action = r.URL.Query().Get("action")
	page := httpx.PageFromQuery(r)
	items, total, err := h.svc.List(r.Context(), f, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPaginated(items, page, total))
}
