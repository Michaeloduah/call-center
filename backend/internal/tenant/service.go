package tenant

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/auth"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/user"
)

type UserCreator interface {
	Insert(ctx context.Context, u user.NewUser) (user.User, error)
}

// Inviter issues and sends invitation emails (implemented by auth.Service).
type Inviter interface {
	IssueInvite(ctx context.Context, userID uuid.UUID) (string, error)
	SendInvite(ctx context.Context, in auth.InviteEmail) bool
}

// SessionRevoker signs out every user of a tenant (implemented by auth.Service).
type SessionRevoker interface {
	RevokeTenantSessions(ctx context.Context, tenantID uuid.UUID) error
}

type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// maxSlugAttempts bounds retries when generated slugs collide.
const maxSlugAttempts = 6

// Provisioner creates a tenant, its default settings and its first admin in one transaction.
// It is shared by self-service sign-up and platform-admin tenant creation.
type Provisioner struct {
	repo  Repository
	users UserCreator
}

func NewProvisioner(repo Repository, users UserCreator) *Provisioner {
	return &Provisioner{repo: repo, users: users}
}

// provision must run inside a platform-scoped transaction.
func (p *Provisioner) provision(ctx context.Context, name, status string, admin user.NewUser) (*Tenant, user.User, error) {
	base := Slugify(name)
	slug := base
	for attempt := 0; attempt < maxSlugAttempts; attempt++ {
		t, err := p.repo.InsertIfSlugFree(ctx, name, slug, status)
		if err != nil {
			return nil, user.User{}, err
		}
		if t != nil {
			if err := p.repo.InsertDefaultSettings(ctx, t.ID); err != nil {
				return nil, user.User{}, err
			}
			admin.TenantID = t.ID
			admin.Role = reqctx.RoleTenantAdmin
			u, err := p.users.Insert(ctx, admin)
			if err != nil {
				return nil, user.User{}, err
			}
			return t, u, nil
		}
		slug = withRandomSuffix(base)
	}
	return nil, user.User{}, errors.New("tenant: could not generate a unique slug")
}

// ProvisionSignup implements auth.TenantProvisioner. The tenant stays pending until the admin verifies their email.
func (p *Provisioner) ProvisionSignup(ctx context.Context, acct auth.SignupAccount) (auth.ProvisionedAccount, error) {
	hash := acct.PasswordHash
	t, u, err := p.provision(ctx, acct.CompanyName, StatusPending, user.NewUser{
		Email: acct.Email, FullName: acct.FullName, Status: user.StatusActive, PasswordHash: &hash,
	})
	if err != nil {
		return auth.ProvisionedAccount{}, err
	}
	return auth.ProvisionedAccount{TenantID: t.ID, TenantName: t.Name, UserID: u.ID}, nil
}

// Service implements platform-admin tenant management.
type Service struct {
	tx          database.TxManager
	repo        Repository
	provisioner *Provisioner
	inviter     Inviter
	sessions    SessionRevoker
	audit       AuditRecorder
}

func NewService(tx database.TxManager, repo Repository, provisioner *Provisioner, inviter Inviter,
	sessions SessionRevoker, recorder AuditRecorder) *Service {
	return &Service{tx: tx, repo: repo, provisioner: provisioner, inviter: inviter, sessions: sessions, audit: recorder}
}

func (s *Service) List(ctx context.Context, f Filter, page httpx.Page) ([]Summary, int, error) {
	var (
		items []Summary
		total int
	)
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		items, total, err = s.repo.List(ctx, f, page)
		return err
	})
	return items, total, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Detail, error) {
	var d *Detail
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		t, err := s.mustGet(ctx, id)
		if err != nil {
			return err
		}
		users, departments, err := s.repo.Counts(ctx, id)
		if err != nil {
			return err
		}
		admins, err := s.repo.Admins(ctx, id)
		if err != nil {
			return err
		}
		d = &Detail{Tenant: *t, UsersCount: users, DepartmentsCount: departments, Admins: admins}
		return nil
	})
	return d, err
}

// CreateResult reports whether the first admin's invitation email was delivered.
type CreateResult struct {
	Tenant          Tenant `json:"tenant"`
	InviteEmailSent bool   `json:"invite_email_sent"`
}

// Create provisions an active tenant and invites its first admin.
func (s *Service) Create(ctx context.Context, in CreateInput) (*CreateResult, error) {
	var (
		t     *Tenant
		admin user.User
		token string
	)
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		t, admin, err = s.provisioner.provision(ctx, in.Name, StatusActive, user.NewUser{
			Email: in.AdminEmail, FullName: in.AdminName, Status: user.StatusInvited,
		})
		if err != nil {
			return err
		}
		if token, err = s.inviter.IssueInvite(ctx, admin.ID); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: t.ID, Action: audit.ActionTenantCreated, EntityType: "tenant", EntityID: t.ID,
			Metadata: map[string]any{"name": t.Name, "admin_email": admin.Email}})
	})
	if err != nil {
		return nil, err
	}
	sent := s.inviter.SendInvite(ctx, auth.InviteEmail{To: admin.Email, FullName: admin.FullName, TenantName: t.Name, Token: token})
	return &CreateResult{Tenant: *t, InviteEmailSent: sent}, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*Tenant, error) {
	var t *Tenant
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		before, err := s.mustGet(ctx, id)
		if err != nil {
			return err
		}
		if err := s.repo.Update(ctx, id, in.Name, in.Slug); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: id, Action: audit.ActionTenantUpdated, EntityType: "tenant", EntityID: id,
			Metadata: map[string]any{"from": map[string]string{"name": before.Name, "slug": before.Slug},
				"to": map[string]string{"name": in.Name, "slug": in.Slug}}}); err != nil {
			return err
		}
		t, err = s.repo.Get(ctx, id)
		return err
	})
	return t, err
}

// Suspend blocks sign-in for every user of the tenant and ends their sessions.
func (s *Service) Suspend(ctx context.Context, id uuid.UUID) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		t, err := s.mustGet(ctx, id)
		if err != nil {
			return err
		}
		if t.Status == StatusSuspended {
			return nil
		}
		if err := s.repo.SetStatus(ctx, id, StatusSuspended); err != nil {
			return err
		}
		if err := s.sessions.RevokeTenantSessions(ctx, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: id, Action: audit.ActionTenantSuspended, EntityType: "tenant", EntityID: id})
	})
}

// Activate re-enables a suspended tenant, or force-activates one still awaiting email verification.
func (s *Service) Activate(ctx context.Context, id uuid.UUID) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		t, err := s.mustGet(ctx, id)
		if err != nil {
			return err
		}
		if t.Status == StatusActive {
			return nil
		}
		if err := s.repo.SetStatus(ctx, id, StatusActive); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{TenantID: id, Action: audit.ActionTenantActivated, EntityType: "tenant", EntityID: id,
			Metadata: map[string]any{"from": t.Status}})
	})
}

// Delete permanently removes a tenant and all its data. The tenant must be suspended first,
// which forces a deliberate two-step action.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		t, err := s.mustGet(ctx, id)
		if err != nil {
			return err
		}
		if t.Status != StatusSuspended {
			return apperr.Conflict("tenant_not_suspended", "Suspend the tenant before deleting it.")
		}
		// Recorded as a platform-level entry so it survives the tenant's deletion.
		if err := s.audit.Record(ctx, audit.Entry{Action: audit.ActionTenantDeleted, EntityType: "tenant", EntityID: id,
			Metadata: map[string]any{"name": t.Name, "slug": t.Slug}}); err != nil {
			return err
		}
		return s.repo.Delete(ctx, id)
	})
}

func (s *Service) mustGet(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, errNotFound
	}
	return t, nil
}
