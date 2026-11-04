package user

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/auth"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
)

// Inviter issues and sends invitation emails (implemented by auth.Service).
type Inviter interface {
	IssueInvite(ctx context.Context, userID uuid.UUID) (string, error)
	SendInvite(ctx context.Context, in auth.InviteEmail) bool
}

// SessionRevoker signs users out (implemented by auth.Service).
type SessionRevoker interface {
	RevokeUserSessions(ctx context.Context, userID uuid.UUID) error
}

// Limits enforces the tenant's plan (implemented by limits.Service).
type Limits interface {
	CheckEnableUser(ctx context.Context, tenantID, userID uuid.UUID) error
}

type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Service implements tenant-admin user management. The tenant always comes from the principal.
type Service struct {
	tx       database.TxManager
	repo     Repository
	inviter  Inviter
	sessions SessionRevoker
	audit    AuditRecorder
	plan     Limits
}

func NewService(tx database.TxManager, repo Repository, inviter Inviter, sessions SessionRevoker, recorder AuditRecorder, plan Limits) *Service {
	return &Service{tx: tx, repo: repo, inviter: inviter, sessions: sessions, audit: recorder, plan: plan}
}

var (
	errSelfAction = apperr.Forbidden("You cannot perform this action on your own account.")
	errLastAdmin  = apperr.Conflict("last_admin", "Your organisation must keep at least one active admin.")
	errNotFound   = apperr.NotFound("User")
)

func (s *Service) List(ctx context.Context, f Filter, page httpx.Page) ([]User, int, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, 0, err
	}
	var (
		users []User
		total int
	)
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		users, total, err = s.repo.List(ctx, p.TenantID, f, page)
		return err
	})
	return users, total, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Detail, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var d *Detail
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		u, err := s.mustGet(ctx, p.TenantID, id)
		if err != nil {
			return err
		}
		member, supervised, err := s.repo.Departments(ctx, p.TenantID, id)
		if err != nil {
			return err
		}
		d = &Detail{User: *u, Departments: member, SupervisedDepartments: supervised}
		return nil
	})
	return d, err
}

// InviteResult reports whether the invitation email was delivered, so the UI can offer a resend.
type InviteResult struct {
	User            User `json:"user"`
	InviteEmailSent bool `json:"invite_email_sent"`
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*InviteResult, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var (
		created    User
		token      string
		tenantName string
	)
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		created, err = s.repo.Insert(ctx, NewUser{
			TenantID: p.TenantID, Email: in.Email, FullName: in.FullName, Role: in.Role, Status: StatusInvited,
		})
		if err != nil {
			return err
		}
		if token, err = s.inviter.IssueInvite(ctx, created.ID); err != nil {
			return err
		}
		if tenantName, err = s.repo.TenantName(ctx, p.TenantID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			TenantID: p.TenantID, Action: audit.ActionUserInvited, EntityType: "user", EntityID: created.ID,
			Metadata: map[string]any{"email": created.Email, "role": created.Role},
		})
	})
	if err != nil {
		return nil, err
	}
	sent := s.inviter.SendInvite(ctx, auth.InviteEmail{To: created.Email, FullName: created.FullName, TenantName: tenantName, Token: token})
	return &InviteResult{User: created, InviteEmailSent: sent}, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*User, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var updated *User
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		current, err := s.mustGet(ctx, p.TenantID, id)
		if err != nil {
			return err
		}
		roleChanged := current.Role != in.Role
		if roleChanged {
			if id == p.UserID {
				return apperr.Forbidden("You cannot change your own role.")
			}
			if current.Role == reqctx.RoleTenantAdmin {
				if err := s.ensureNotLastAdmin(ctx, p.TenantID, id); err != nil {
					return err
				}
			}
		}
		if err := s.repo.UpdateProfile(ctx, p.TenantID, id, in.FullName, in.Role); err != nil {
			return err
		}
		// Only supervisors may be assigned to supervise departments.
		if roleChanged && current.Role == reqctx.RoleSupervisor {
			if err := s.repo.RemoveSupervisorAssignments(ctx, p.TenantID, id); err != nil {
				return err
			}
		}
		entry := audit.Entry{TenantID: p.TenantID, Action: audit.ActionUserUpdated, EntityType: "user", EntityID: id,
			Metadata: map[string]any{"full_name": in.FullName}}
		if roleChanged {
			entry.Action = audit.ActionUserRoleChanged
			entry.Metadata["from"], entry.Metadata["to"] = current.Role, in.Role
		}
		if err := s.audit.Record(ctx, entry); err != nil {
			return err
		}
		updated, err = s.repo.Get(ctx, p.TenantID, id)
		return err
	})
	return updated, err
}

func (s *Service) Disable(ctx context.Context, id uuid.UUID) error {
	return s.changeStatus(ctx, id, func(ctx context.Context, p reqctx.Principal, u *User) error {
		if u.Status == StatusDisabled {
			return nil
		}
		if u.Role == reqctx.RoleTenantAdmin {
			if err := s.ensureNotLastAdmin(ctx, p.TenantID, id); err != nil {
				return err
			}
		}
		if err := s.repo.Disable(ctx, p.TenantID, id); err != nil {
			return err
		}
		if err := s.sessions.RevokeUserSessions(ctx, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionUserDisabled, EntityType: "user", EntityID: id})
	})
}

func (s *Service) Enable(ctx context.Context, id uuid.UUID) error {
	return s.changeStatus(ctx, id, func(ctx context.Context, p reqctx.Principal, u *User) error {
		if u.Status != StatusDisabled {
			return nil
		}
		if err := s.plan.CheckEnableUser(ctx, p.TenantID, id); err != nil {
			return err
		}
		if err := s.repo.Enable(ctx, p.TenantID, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionUserEnabled, EntityType: "user", EntityID: id})
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.changeStatus(ctx, id, func(ctx context.Context, p reqctx.Principal, u *User) error {
		if u.Role == reqctx.RoleTenantAdmin && u.Status == StatusActive {
			if err := s.ensureNotLastAdmin(ctx, p.TenantID, id); err != nil {
				return err
			}
		}
		// Record first: the audit row's actor/entity references must exist when inserted.
		if err := s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionUserDeleted, EntityType: "user", EntityID: id,
			Metadata: map[string]any{"email": u.Email, "role": u.Role}}); err != nil {
			return err
		}
		return s.repo.Delete(ctx, p.TenantID, id)
	})
}

// ResendInvite issues a fresh invitation (invalidating the previous link) and emails it.
func (s *Service) ResendInvite(ctx context.Context, id uuid.UUID) (*InviteResult, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var (
		u          *User
		token      string
		tenantName string
	)
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		if u, err = s.mustGet(ctx, p.TenantID, id); err != nil {
			return err
		}
		if u.Status != StatusInvited {
			return apperr.Conflict("not_invited", "Only users who have not accepted their invitation can be re-invited.")
		}
		if token, err = s.inviter.IssueInvite(ctx, id); err != nil {
			return err
		}
		if tenantName, err = s.repo.TenantName(ctx, p.TenantID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionUserInviteResent, EntityType: "user", EntityID: id})
	})
	if err != nil {
		return nil, err
	}
	sent := s.inviter.SendInvite(ctx, auth.InviteEmail{To: u.Email, FullName: u.FullName, TenantName: tenantName, Token: token})
	return &InviteResult{User: *u, InviteEmailSent: sent}, nil
}

// changeStatus loads the target user and rejects actions on one's own account before running fn.
func (s *Service) changeStatus(ctx context.Context, id uuid.UUID, fn func(ctx context.Context, p reqctx.Principal, u *User) error) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	if id == p.UserID {
		return errSelfAction
	}
	return s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		u, err := s.mustGet(ctx, p.TenantID, id)
		if err != nil {
			return err
		}
		return fn(ctx, p, u)
	})
}

func (s *Service) mustGet(ctx context.Context, tenantID, id uuid.UUID) (*User, error) {
	u, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errNotFound
	}
	return u, nil
}

// ensureNotLastAdmin fails if removing adminID would leave the tenant without an active admin.
func (s *Service) ensureNotLastAdmin(ctx context.Context, tenantID, adminID uuid.UUID) error {
	admins, err := s.repo.LockActiveAdmins(ctx, tenantID)
	if err != nil {
		return err
	}
	others := slices.DeleteFunc(admins, func(id uuid.UUID) bool { return id == adminID })
	if len(others) == 0 {
		return errLastAdmin
	}
	return nil
}
