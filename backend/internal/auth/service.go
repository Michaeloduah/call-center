package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/platform/secret"
)

// TenantProvisioner creates a tenant and its first admin for self-service sign-up.
// Implemented by the tenant package; it runs inside the caller's transaction.
type TenantProvisioner interface {
	ProvisionSignup(ctx context.Context, acct SignupAccount) (ProvisionedAccount, error)
}

// AuditRecorder writes audit entries inside the caller's transaction.
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

type Service struct {
	tx          database.TxManager
	repo        Repository
	notifier    *Notifier
	provisioner TenantProvisioner
	audit       AuditRecorder
	sessionTTL  time.Duration
	logger      *slog.Logger
}

func NewService(tx database.TxManager, repo Repository, notifier *Notifier, provisioner TenantProvisioner,
	recorder AuditRecorder, sessionTTL time.Duration, logger *slog.Logger) *Service {
	return &Service{tx: tx, repo: repo, notifier: notifier, provisioner: provisioner, audit: recorder,
		sessionTTL: sessionTTL, logger: logger}
}

// LoginResult carries the new session secret. The caller (the Next.js server) stores it in an HttpOnly cookie.
type LoginResult struct {
	SessionToken string    `json:"session_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         Me        `json:"user"`
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginResult, error) {
	var creds *credentials
	if err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		creds, err = s.repo.FindCredentialsByEmail(ctx, in.Email)
		return err
	}); err != nil {
		return nil, err
	}

	// Hash verification happens outside the transaction; it is deliberately slow.
	hash := dummyHash
	if creds != nil && creds.PasswordHash != nil {
		hash = *creds.PasswordHash
	}
	ok, err := VerifyPassword(in.Password, hash)
	if err != nil {
		return nil, fmt.Errorf("verify password: %w", err)
	}
	if !ok || creds == nil || creds.PasswordHash == nil {
		return nil, errInvalidCredentials
	}
	if err := checkCanSignIn(creds); err != nil {
		return nil, err
	}

	raw, tokenHash, err := secret.New()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(s.sessionTTL)
	meta := reqctx.MetaFrom(ctx)
	if err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		if err := s.repo.CreateSession(ctx, creds.UserID, tokenHash, expiresAt, meta.IP, meta.UserAgent); err != nil {
			return err
		}
		return s.repo.TouchLastLogin(ctx, creds.UserID)
	}); err != nil {
		return nil, err
	}
	return &LoginResult{SessionToken: raw, ExpiresAt: expiresAt, User: creds.me()}, nil
}

// checkCanSignIn applies account-state rules after the password has been verified,
// so these specific messages are only revealed to someone who knows the password.
func checkCanSignIn(c *credentials) error {
	switch {
	case c.Status == StatusDisabled:
		return errAccountDisabled
	case c.Status != StatusActive:
		return errInvalidCredentials
	case c.Role != reqctx.RolePlatformAdmin && c.EmailVerifiedAt == nil:
		return errEmailNotVerified
	case c.TenantStatus != nil && *c.TenantStatus == TenantStatusSuspended:
		return errTenantSuspended
	case c.TenantStatus != nil && *c.TenantStatus != TenantStatusActive:
		return errEmailNotVerified
	}
	return nil
}

// Authenticate resolves a session secret into a principal.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (reqctx.Principal, error) {
	var p *reqctx.Principal
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		p, err = s.repo.PrincipalBySessionHash(ctx, secret.Hash(rawToken))
		return err
	})
	if err != nil {
		return reqctx.Principal{}, err
	}
	if p == nil {
		return reqctx.Principal{}, errInvalidSession
	}
	return *p, nil
}

func (s *Service) Logout(ctx context.Context) error {
	p, ok := reqctx.PrincipalFrom(ctx)
	if !ok {
		return errInvalidSession
	}
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		return s.repo.RevokeSession(ctx, p.SessionID)
	})
}

func (s *Service) Me(ctx context.Context) (Me, error) {
	p, ok := reqctx.PrincipalFrom(ctx)
	if !ok {
		return Me{}, errInvalidSession
	}
	var creds *credentials
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		creds, err = s.repo.FindCredentialsByID(ctx, p.UserID)
		return err
	})
	if err != nil {
		return Me{}, err
	}
	if creds == nil {
		return Me{}, errInvalidSession
	}
	return creds.me(), nil
}

// SignupResult tells the client whether the verification email went out.
type SignupResult struct {
	Email     string `json:"email"`
	EmailSent bool   `json:"email_sent"`
}

func (s *Service) Signup(ctx context.Context, in SignupInput) (*SignupResult, error) {
	passwordHash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	var rawToken string
	err = s.tx.InPlatform(ctx, func(ctx context.Context) error {
		acct, err := s.provisioner.ProvisionSignup(ctx, SignupAccount{
			CompanyName: in.CompanyName, FullName: in.FullName, Email: in.Email, PasswordHash: passwordHash,
		})
		if err != nil {
			return err
		}
		if rawToken, err = s.issueToken(ctx, acct.UserID, PurposeVerifyEmail); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			TenantID: acct.TenantID, Action: audit.ActionTenantSignedUp, EntityType: "tenant", EntityID: acct.TenantID,
			ActorEmail: in.Email, Metadata: map[string]any{"name": acct.TenantName},
		})
	})
	if err != nil {
		return nil, err
	}
	sent := s.deliver("verification", in.Email, func() error {
		return s.notifier.sendVerifyEmail(ctx, in.Email, in.FullName, rawToken)
	})
	return &SignupResult{Email: in.Email, EmailSent: sent}, nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		userID, err := s.repo.ClaimToken(ctx, secret.Hash(token), PurposeVerifyEmail)
		if err != nil {
			return err
		}
		if userID == uuid.Nil {
			return errInvalidToken
		}
		return s.repo.MarkEmailVerified(ctx, userID)
	})
}

// ResendVerification always succeeds from the caller's view so it cannot be used to probe accounts.
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	return s.sendTokenEmail(ctx, email, PurposeVerifyEmail,
		func(c *credentials) bool { return c.Status == StatusActive && c.EmailVerifiedAt == nil },
		s.notifier.sendVerifyEmail)
}

// ForgotPassword always succeeds from the caller's view so it cannot be used to probe accounts.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	return s.sendTokenEmail(ctx, email, PurposeResetPassword,
		func(c *credentials) bool { return c.Status == StatusActive },
		s.notifier.sendPasswordReset)
}

func (s *Service) sendTokenEmail(ctx context.Context, email string, purpose Purpose,
	eligible func(*credentials) bool, send func(ctx context.Context, to, name, token string) error) error {
	var (
		creds    *credentials
		rawToken string
	)
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		creds, err = s.repo.FindCredentialsByEmail(ctx, email)
		if err != nil || creds == nil || !eligible(creds) {
			return err
		}
		rawToken, err = s.issueToken(ctx, creds.UserID, purpose)
		return err
	})
	if err != nil {
		return err
	}
	if rawToken != "" {
		s.deliver(string(purpose), creds.Email, func() error { return send(ctx, creds.Email, creds.FullName, rawToken) })
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	passwordHash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		userID, err := s.repo.ClaimToken(ctx, secret.Hash(token), PurposeResetPassword)
		if err != nil {
			return err
		}
		if userID == uuid.Nil {
			return errInvalidToken
		}
		ok, err := s.repo.SetActivePassword(ctx, userID, passwordHash)
		if err != nil {
			return err
		}
		if !ok {
			return errInvalidToken
		}
		// Receiving the reset email proves ownership of the address.
		if err := s.repo.MarkEmailVerified(ctx, userID); err != nil {
			return err
		}
		return s.repo.RevokeUserSessions(ctx, userID)
	})
}

func (s *Service) LookupInvite(ctx context.Context, token string) (*InviteInfo, error) {
	var info *InviteInfo
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		userID, err := s.repo.PeekToken(ctx, secret.Hash(token), PurposeInvite)
		if err != nil {
			return err
		}
		if userID == uuid.Nil {
			return errInvalidToken
		}
		creds, err := s.repo.FindCredentialsByID(ctx, userID)
		if err != nil {
			return err
		}
		if creds == nil || creds.Status != StatusInvited {
			return errInvalidToken
		}
		info = &InviteInfo{Email: creds.Email, FullName: creds.FullName, TenantName: creds.TenantName}
		return nil
	})
	return info, err
}

func (s *Service) AcceptInvite(ctx context.Context, token, password string) error {
	passwordHash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		userID, err := s.repo.ClaimToken(ctx, secret.Hash(token), PurposeInvite)
		if err != nil {
			return err
		}
		if userID == uuid.Nil {
			return errInvalidToken
		}
		ok, err := s.repo.ActivateInvitedUser(ctx, userID, passwordHash)
		if err != nil {
			return err
		}
		if !ok {
			return errInvalidToken
		}
		return nil
	})
}

// issueToken replaces any outstanding token of the same purpose. Must run inside a transaction.
func (s *Service) issueToken(ctx context.Context, userID uuid.UUID, purpose Purpose) (string, error) {
	raw, hash, err := secret.New()
	if err != nil {
		return "", err
	}
	if err := s.repo.InvalidateTokens(ctx, userID, purpose); err != nil {
		return "", err
	}
	if err := s.repo.InsertToken(ctx, userID, purpose, hash, time.Now().Add(tokenTTL[purpose])); err != nil {
		return "", err
	}
	return raw, nil
}

// deliver sends an email after the transaction committed. Failures are logged rather than
// returned: the state change already happened and the user can request the email again.
func (s *Service) deliver(kind, to string, send func() error) bool {
	if err := send(); err != nil {
		s.logger.Error("send email failed", "kind", kind, "to", to, "error", err)
		return false
	}
	return true
}

// IssueInvite creates an invitation token for an invited user. It must run inside the
// caller's transaction; send the email with SendInvite after commit.
func (s *Service) IssueInvite(ctx context.Context, userID uuid.UUID) (string, error) {
	return s.issueToken(ctx, userID, PurposeInvite)
}

// SendInvite emails an invitation and reports whether delivery succeeded.
func (s *Service) SendInvite(ctx context.Context, in InviteEmail) bool {
	return s.deliver("invite", in.To, func() error { return s.notifier.sendInvite(ctx, in) })
}

// RevokeUserSessions signs a user out everywhere. Runs inside the caller's transaction.
func (s *Service) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	return s.repo.RevokeUserSessions(ctx, userID)
}

// RevokeTenantSessions signs out every user of a tenant. Runs inside the caller's transaction.
func (s *Service) RevokeTenantSessions(ctx context.Context, tenantID uuid.UUID) error {
	return s.repo.RevokeTenantSessions(ctx, tenantID)
}
