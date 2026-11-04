package widget

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/department"
	"callcenter/internal/limits"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
)

// Limits enforces the tenant's plan (implemented by limits.Service).
type Limits interface {
	CheckCreate(ctx context.Context, tenantID uuid.UUID, kind limits.Kind) error
	CallsBlocked(ctx context.Context, tenantID uuid.UUID) (bool, error)
}

type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Service implements tenant-admin widget management and the public widget lookup.
type Service struct {
	tx    database.TxManager
	repo  Repository
	audit AuditRecorder
	plan  Limits
	now   func() time.Time
}

func NewService(tx database.TxManager, repo Repository, recorder AuditRecorder, plan Limits) *Service {
	return &Service{tx: tx, repo: repo, audit: recorder, plan: plan, now: time.Now}
}

func (s *Service) List(ctx context.Context, f Filter, page httpx.Page) ([]Widget, int, error) {
	var (
		items []Widget
		total int
	)
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) (err error) {
		items, total, err = s.repo.List(ctx, tenantID, f, page)
		return err
	})
	return items, total, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Widget, error) {
	var w *Widget
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) (err error) {
		w, err = s.mustGet(ctx, tenantID, id)
		return err
	})
	return w, err
}

func (s *Service) Create(ctx context.Context, in Input) (*Widget, error) {
	key, err := newPublicKey()
	if err != nil {
		return nil, err
	}
	var w *Widget
	err = s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) error {
		if err := s.plan.CheckCreate(ctx, tenantID, limits.KindWidgets); err != nil {
			return err
		}
		id, err := s.repo.Insert(ctx, tenantID, key, in)
		if err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionWidgetCreated, EntityType: "widget",
			EntityID: id, Metadata: map[string]any{"name": in.Name, "allowed_origins": in.AllowedOrigins}}); err != nil {
			return err
		}
		w, err = s.mustGet(ctx, tenantID, id)
		return err
	})
	return w, err
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (*Widget, error) {
	var w *Widget
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) error {
		if _, err := s.mustGet(ctx, tenantID, id); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, tenantID, id, in); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionWidgetUpdated, EntityType: "widget",
			EntityID: id, Metadata: map[string]any{"name": in.Name, "is_active": *in.IsActive, "allowed_origins": in.AllowedOrigins}}); err != nil {
			return err
		}
		var err error
		w, err = s.mustGet(ctx, tenantID, id)
		return err
	})
	return w, err
}

// Delete removes the widget. Its past calls are kept (their widget reference is cleared).
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) error {
		w, err := s.mustGet(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionWidgetDeleted, EntityType: "widget",
			EntityID: id, Metadata: map[string]any{"name": w.Name}}); err != nil {
			return err
		}
		return s.repo.Delete(ctx, tenantID, id)
	})
}

// Public returns the caller-facing configuration, or a 404 for unknown/inactive widgets.
func (s *Service) Public(ctx context.Context, publicKey string) (*PublicConfig, error) {
	var r *Resolved
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		if r, err = s.repo.Resolve(ctx, publicKey); err != nil || r == nil {
			return err
		}
		r.CapReached, err = s.plan.CallsBlocked(ctx, r.TenantID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, errNotFound
	}
	availability, err := AvailabilityOf(r, s.now())
	if err != nil {
		return nil, err
	}
	return &PublicConfig{
		PublicKey:      r.PublicKey,
		TenantName:     r.TenantName,
		DepartmentName: r.DepartmentName,
		ButtonLabel:    r.ButtonLabel,
		AccentColor:    r.AccentColor,
		Greeting:       r.Greeting,
		CallerForm:     r.CallerForm,
		ConsentText:    r.ConsentText,
		Availability:   availability,
		Timezone:       r.Timezone,
		BusinessHours:  r.Hours,
		FrameAncestors: r.AllowedOrigins,
	}, nil
}

// AvailabilityOf decides whether a resolved widget can take a call at the given instant.
func AvailabilityOf(r *Resolved, now time.Time) (Availability, error) {
	if !r.DepartmentActive {
		return AvailabilityUnavailable, nil
	}
	open, err := department.IsOpen(r.Timezone, r.Hours, now)
	if err != nil {
		return "", err
	}
	switch {
	case !open:
		return AvailabilityClosed, nil
	case r.CapReached:
		return AvailabilityLimitReached, nil
	case !r.AgentsAvailable:
		return AvailabilityNoAgents, nil
	default:
		return AvailabilityOpen, nil
	}
}

func (s *Service) inTenant(ctx context.Context, fn func(ctx context.Context, tenantID uuid.UUID) error) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	return s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error { return fn(ctx, p.TenantID) })
}

func (s *Service) mustGet(ctx context.Context, tenantID, id uuid.UUID) (*Widget, error) {
	w, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errNotFound
	}
	return w, nil
}

const publicKeyBytes = 16 // 22 base64url characters, matching the DB check constraint

func newPublicKey() (string, error) {
	b := make([]byte, publicKeyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate widget key: %w", err)
	}
	return "wk_" + base64.RawURLEncoding.EncodeToString(b), nil
}
