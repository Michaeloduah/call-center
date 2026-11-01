package department

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/user"
)

// UserDirectory resolves user references within a tenant (implemented by user.Repository).
type UserDirectory interface {
	Accounts(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]user.Account, error)
}

type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Service implements tenant-admin department management. The tenant always comes from the principal.
type Service struct {
	tx    database.TxManager
	repo  Repository
	users UserDirectory
	audit AuditRecorder
}

func NewService(tx database.TxManager, repo Repository, users UserDirectory, recorder AuditRecorder) *Service {
	return &Service{tx: tx, repo: repo, users: users, audit: recorder}
}

func (s *Service) List(ctx context.Context, f Filter, page httpx.Page) ([]Summary, int, error) {
	var (
		items []Summary
		total int
	)
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) (err error) {
		items, total, err = s.repo.List(ctx, tenantID, f, page)
		return err
	})
	return items, total, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Detail, error) {
	var d *Detail
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) (err error) {
		d, err = s.detail(ctx, tenantID, id)
		return err
	})
	return d, err
}

func (s *Service) Create(ctx context.Context, in Input) (*Department, error) {
	var d *Department
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) (err error) {
		if d, err = s.repo.Insert(ctx, tenantID, in); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionDepartmentCreated,
			EntityType: "department", EntityID: d.ID, Metadata: map[string]any{"name": d.Name}})
	})
	return d, err
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (*Department, error) {
	var d *Department
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) error {
		if _, err := s.mustGet(ctx, tenantID, id); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, tenantID, id, in); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionDepartmentUpdated,
			EntityType: "department", EntityID: id, Metadata: map[string]any{"name": in.Name, "is_active": *in.IsActive}}); err != nil {
			return err
		}
		var err error
		d, err = s.repo.Get(ctx, tenantID, id)
		return err
	})
	return d, err
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) error {
		d, err := s.mustGet(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionDepartmentDeleted,
			EntityType: "department", EntityID: id, Metadata: map[string]any{"name": d.Name}}); err != nil {
			return err
		}
		return s.repo.Delete(ctx, tenantID, id)
	})
}

// SetMembers replaces the agents who take calls for the department. Any non-disabled tenant user can be a member.
func (s *Service) SetMembers(ctx context.Context, id uuid.UUID, in UsersInput) (*Detail, error) {
	return s.setUsers(ctx, id, in, audit.ActionDepartmentMembers, s.repo.ReplaceMembers,
		func(a user.Account) bool { return a.Status != user.StatusDisabled },
		"Every member must be an existing, non-disabled user.")
}

// SetSupervisors replaces the supervisors who can see the department. Only supervisor-role users qualify.
func (s *Service) SetSupervisors(ctx context.Context, id uuid.UUID, in UsersInput) (*Detail, error) {
	return s.setUsers(ctx, id, in, audit.ActionDepartmentSupervisor, s.repo.ReplaceSupervisors,
		func(a user.Account) bool { return a.Role == reqctx.RoleSupervisor && a.Status != user.StatusDisabled },
		"Every supervisor must be an existing, non-disabled user with the Supervisor role.")
}

func (s *Service) setUsers(ctx context.Context, id uuid.UUID, in UsersInput, action string,
	replace func(ctx context.Context, tenantID, id uuid.UUID, userIDs []uuid.UUID) error,
	eligible func(user.Account) bool, invalidMsg string) (*Detail, error) {
	var d *Detail
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) (err error) {
		if _, err := s.mustGet(ctx, tenantID, id); err != nil {
			return err
		}
		accounts, err := s.users.Accounts(ctx, tenantID, in.UserIDs)
		if err != nil {
			return err
		}
		for _, uid := range in.UserIDs {
			if a, ok := accounts[uid]; !ok || !eligible(a) {
				return apperr.Validation(map[string]string{"user_ids": invalidMsg})
			}
		}
		if err := replace(ctx, tenantID, id, in.UserIDs); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: action, EntityType: "department", EntityID: id,
			Metadata: map[string]any{"user_ids": in.UserIDs}}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tenantID, id)
		return err
	})
	return d, err
}

// SetBusinessHours replaces the weekly schedule. An empty schedule means the department is always closed.
func (s *Service) SetBusinessHours(ctx context.Context, id uuid.UUID, in HoursInput) (*Detail, error) {
	var d *Detail
	err := s.inTenant(ctx, func(ctx context.Context, tenantID uuid.UUID) (err error) {
		if _, err := s.mustGet(ctx, tenantID, id); err != nil {
			return err
		}
		if err := s.repo.ReplaceHours(ctx, tenantID, id, in.Hours); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: tenantID, Action: audit.ActionDepartmentHours, EntityType: "department",
			EntityID: id, Metadata: map[string]any{"ranges": len(in.Hours)}}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tenantID, id)
		return err
	})
	return d, err
}

func (s *Service) inTenant(ctx context.Context, fn func(ctx context.Context, tenantID uuid.UUID) error) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	return s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error { return fn(ctx, p.TenantID) })
}

func (s *Service) mustGet(ctx context.Context, tenantID, id uuid.UUID) (*Department, error) {
	d, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, errNotFound
	}
	return d, nil
}

func (s *Service) detail(ctx context.Context, tenantID, id uuid.UUID) (*Detail, error) {
	d, err := s.mustGet(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.Members(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	supervisors, err := s.repo.Supervisors(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	hours, err := s.repo.Hours(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("department detail: %w", err)
	}
	return &Detail{Department: *d, Members: members, Supervisors: supervisors, BusinessHours: hours}, nil
}
