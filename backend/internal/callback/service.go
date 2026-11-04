package callback

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/realtime"
	"callcenter/internal/widget"
)

// WidgetResolver finds a widget by public key inside the caller's transaction (widget.Repository).
type WidgetResolver interface {
	Resolve(ctx context.Context, publicKey string) (*widget.Resolved, error)
}

// CallerAuth checks a caller token against a call, inside the caller's transaction (call.Service).
type CallerAuth interface {
	Authenticate(ctx context.Context, id uuid.UUID, callerToken string) (*call.CallerView, error)
}

// Notifier tells tenant administrators about a new callback request.
type Notifier interface {
	NewCallback(ctx context.Context, tenantName string, admins []Admin, cb Notification)
}

// Notification is the part of a callback the notification email needs.
type Notification struct {
	ID             uuid.UUID
	DepartmentName string
	Name           string
	Phone          string
	Email          string
	Reason         string
}

type Service struct {
	tx       database.TxManager
	repo     Repository
	widgets  WidgetResolver
	auth     CallerAuth
	notifier Notifier
	logger   *slog.Logger
}

func NewService(tx database.TxManager, repo Repository, widgets WidgetResolver, auth CallerAuth, notifier Notifier, logger *slog.Logger) *Service {
	return &Service{tx: tx, repo: repo, widgets: widgets, auth: auth, notifier: notifier, logger: logger}
}

// ---- Public side (callers) ------------------------------------------------------------------

// CreateForWidget records a callback request from a caller who could not start a call
// (the department was closed or nobody was online).
func (s *Service) CreateForWidget(ctx context.Context, publicKey string, in CreateInput) (uuid.UUID, error) {
	var id uuid.UUID
	var made *created
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		w, err := s.widgets.Resolve(ctx, publicKey)
		if err != nil {
			return err
		}
		if w == nil {
			return errWidget
		}
		if made, err = s.insert(ctx, w.TenantID, w.DepartmentID, nil, in); err != nil {
			return err
		}
		id = made.id
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	s.announce(ctx, made, in)
	return id, nil
}

// CreateForCall records a callback request from the caller of a call nobody answered.
func (s *Service) CreateForCall(ctx context.Context, callID uuid.UUID, callerToken string, in CreateInput) (uuid.UUID, error) {
	var made *created
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		view, err := s.auth.Authenticate(ctx, callID, callerToken)
		if err != nil {
			return err
		}
		if view.Status != call.StatusMissed {
			return errNotMissed
		}
		info, err := s.repo.CallInfo(ctx, callID)
		if err != nil {
			return err
		}
		if info == nil {
			return errNotFound
		}
		made, err = s.insert(ctx, info.TenantID, info.DepartmentID, &callID, in)
		if database.IsUniqueViolation(err, "") {
			return errExists
		}
		return err
	})
	if err != nil {
		return uuid.Nil, err
	}
	s.announce(ctx, made, in)
	return made.id, nil
}

type created struct {
	id       uuid.UUID
	tenantID uuid.UUID
}

func (s *Service) insert(ctx context.Context, tenantID, departmentID uuid.UUID, callID *uuid.UUID, in CreateInput) (*created, error) {
	id, err := s.repo.Insert(ctx, tenantID, departmentID, callID, in)
	if err != nil {
		return nil, err
	}
	if err := s.publish(ctx, departmentID); err != nil {
		return nil, err
	}
	return &created{id: id, tenantID: tenantID}, nil
}

// announce emails the tenant's administrators once the request is saved. Failure is only logged:
// the callback exists and shows up on the agents' consoles.
func (s *Service) announce(ctx context.Context, c *created, in CreateInput) {
	var tenantName string
	var admins []Admin
	var departmentName string
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		if tenantName, admins, err = s.repo.Admins(ctx, c.tenantID); err != nil {
			return err
		}
		cb, err := s.repo.Get(ctx, c.id, Scope{All: true})
		if err != nil || cb == nil {
			return err
		}
		departmentName = cb.DepartmentName
		return nil
	})
	if err != nil {
		s.logger.Warn("prepare callback notification", "callback_id", c.id, "error", err)
		return
	}
	s.notifier.NewCallback(ctx, tenantName, admins, Notification{
		ID: c.id, DepartmentName: departmentName, Name: in.Name, Phone: in.Phone, Email: in.Email, Reason: in.Reason,
	})
}

// publish tells the department's agents (and its board) that callbacks changed.
func (s *Service) publish(ctx context.Context, departmentID uuid.UUID) error {
	members, err := s.repo.MemberIDs(ctx, departmentID)
	if err != nil {
		return err
	}
	topics := []string{realtime.DepartmentTopic(departmentID)}
	for _, id := range members {
		topics = append(topics, realtime.UserTopic(id))
	}
	return realtime.Publish(ctx, topics...)
}

// ---- Authenticated side (agents and admins) ----------------------------------------------------

func scopeOf(p reqctx.Principal) Scope {
	return Scope{All: p.Role == reqctx.RoleTenantAdmin, UserID: p.UserID}
}

func (s *Service) List(ctx context.Context, f Filter, page httpx.Page) ([]Callback, int, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, 0, err
	}
	if f.Status != "" && !f.Status.Valid() {
		return nil, 0, errInvalidStat
	}
	var items []Callback
	var total int
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		items, total, err = s.repo.List(ctx, scopeOf(p), f, page)
		return err
	})
	return items, total, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Callback, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var cb *Callback
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		if cb, err = s.repo.Get(ctx, id, scopeOf(p)); err == nil && cb == nil {
			err = errNotFound
		}
		return err
	})
	return cb, err
}

// change runs one workflow transition: it checks the callback is visible, applies the update and
// tells the department. A failed conditional update becomes failure.
func (s *Service) change(ctx context.Context, id uuid.UUID, failure error, apply func(ctx context.Context, p reqctx.Principal, cb *Callback) (bool, error)) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	return s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		cb, err := s.repo.Get(ctx, id, scopeOf(p))
		if err != nil {
			return err
		}
		if cb == nil {
			return errNotFound
		}
		ok, err := apply(ctx, p, cb)
		if err != nil {
			return err
		}
		if !ok {
			return failure
		}
		return s.publish(ctx, cb.DepartmentID)
	})
}

func (s *Service) Claim(ctx context.Context, id uuid.UUID) error {
	return s.change(ctx, id, errTaken, func(ctx context.Context, p reqctx.Principal, cb *Callback) (bool, error) {
		return s.repo.Claim(ctx, id, p.UserID)
	})
}

func (s *Service) Release(ctx context.Context, id uuid.UUID) error {
	return s.change(ctx, id, errState, func(ctx context.Context, p reqctx.Principal, cb *Callback) (bool, error) {
		return s.repo.Release(ctx, id, p.UserID, p.Role == reqctx.RoleTenantAdmin)
	})
}

func (s *Service) Complete(ctx context.Context, id uuid.UUID, in CompleteInput) error {
	return s.change(ctx, id, errState, func(ctx context.Context, p reqctx.Principal, cb *Callback) (bool, error) {
		return s.repo.Complete(ctx, id, p.UserID, p.Role == reqctx.RoleTenantAdmin, in.Notes)
	})
}

func (s *Service) Cancel(ctx context.Context, id uuid.UUID) error {
	return s.change(ctx, id, errState, func(ctx context.Context, p reqctx.Principal, cb *Callback) (bool, error) {
		if p.Role != reqctx.RoleTenantAdmin {
			return false, errAdminOnly
		}
		return s.repo.Cancel(ctx, id)
	})
}

// Assign gives a callback to a department member (administrators only).
func (s *Service) Assign(ctx context.Context, id uuid.UUID, in AssignInput) error {
	return s.change(ctx, id, errState, func(ctx context.Context, p reqctx.Principal, cb *Callback) (bool, error) {
		if p.Role != reqctx.RoleTenantAdmin {
			return false, errAdminOnly
		}
		member, err := s.repo.IsMember(ctx, cb.DepartmentID, in.UserID)
		if err != nil {
			return false, err
		}
		if !member {
			return false, errNotMember
		}
		return s.repo.Assign(ctx, id, in.UserID)
	})
}
