package auth

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
)

// Repository is the persistence contract for auth. All methods run inside the
// caller's scoped transaction (see database.Conn).
type Repository interface {
	FindCredentialsByEmail(ctx context.Context, email string) (*credentials, error)
	FindCredentialsByID(ctx context.Context, userID uuid.UUID) (*credentials, error)
	CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time, ip netip.Addr, userAgent string) error
	PrincipalBySessionHash(ctx context.Context, tokenHash []byte) (*reqctx.Principal, error)
	RevokeSession(ctx context.Context, sessionID uuid.UUID) error
	RevokeUserSessions(ctx context.Context, userID uuid.UUID) error
	RevokeTenantSessions(ctx context.Context, tenantID uuid.UUID) error
	TouchLastLogin(ctx context.Context, userID uuid.UUID) error
	InvalidateTokens(ctx context.Context, userID uuid.UUID, purpose Purpose) error
	InsertToken(ctx context.Context, userID uuid.UUID, purpose Purpose, tokenHash []byte, expiresAt time.Time) error
	ClaimToken(ctx context.Context, tokenHash []byte, purpose Purpose) (uuid.UUID, error)
	PeekToken(ctx context.Context, tokenHash []byte, purpose Purpose) (uuid.UUID, error)
	SetActivePassword(ctx context.Context, userID uuid.UUID, passwordHash string) (bool, error)
	ActivateInvitedUser(ctx context.Context, userID uuid.UUID, passwordHash string) (bool, error)
	MarkEmailVerified(ctx context.Context, userID uuid.UUID) error
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

const credentialsSelect = `
	SELECT u.id, u.tenant_id, t.name, t.status, u.email::text, u.full_name, u.role, u.status,
	       u.password_hash, u.email_verified_at
	FROM users u
	LEFT JOIN tenants t ON t.id = u.tenant_id`

func scanCredentials(row interface{ Scan(...any) error }) (*credentials, error) {
	var c credentials
	err := row.Scan(&c.UserID, &c.TenantID, &c.TenantName, &c.TenantStatus, &c.Email, &c.FullName,
		&c.Role, &c.Status, &c.PasswordHash, &c.EmailVerifiedAt)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan credentials: %w", err)
	}
	return &c, nil
}

// FindCredentialsByEmail returns nil, nil when no user has that email.
func (pgRepository) FindCredentialsByEmail(ctx context.Context, email string) (*credentials, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanCredentials(q.QueryRow(ctx, credentialsSelect+` WHERE u.email = $1`, email))
}

// FindCredentialsByID returns nil, nil when the user does not exist.
func (pgRepository) FindCredentialsByID(ctx context.Context, userID uuid.UUID) (*credentials, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return scanCredentials(q.QueryRow(ctx, credentialsSelect+` WHERE u.id = $1`, userID))
}

func (pgRepository) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time, ip netip.Addr, userAgent string) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	var ipArg *string
	if ip.IsValid() {
		s := ip.String()
		ipArg = &s
	}
	_, err = q.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4::inet, nullif($5, ''))`, userID, tokenHash, expiresAt, ipArg, userAgent)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// PrincipalBySessionHash returns nil, nil when the session is unknown, expired,
// revoked, or its user/tenant is no longer allowed to sign in.
func (pgRepository) PrincipalBySessionHash(ctx context.Context, tokenHash []byte) (*reqctx.Principal, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var p reqctx.Principal
	var tenantID *uuid.UUID
	err = q.QueryRow(ctx, `
		SELECT s.id, u.id, u.tenant_id, u.email::text, u.full_name, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN tenants t ON t.id = u.tenant_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
		  AND u.status = 'active'
		  AND (u.tenant_id IS NULL OR t.status = 'active')`, tokenHash).
		Scan(&p.SessionID, &p.UserID, &tenantID, &p.Email, &p.FullName, &p.Role)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup session: %w", err)
	}
	if tenantID != nil {
		p.TenantID = *tenantID
	}
	return &p, nil
}

func (pgRepository) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	return exec(ctx, "revoke session",
		`UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID)
}

func (pgRepository) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	return exec(ctx, "revoke user sessions",
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
}

func (pgRepository) RevokeTenantSessions(ctx context.Context, tenantID uuid.UUID) error {
	return exec(ctx, "revoke tenant sessions", `
		UPDATE sessions SET revoked_at = now()
		WHERE revoked_at IS NULL AND user_id IN (SELECT id FROM users WHERE tenant_id = $1)`, tenantID)
}

func (pgRepository) TouchLastLogin(ctx context.Context, userID uuid.UUID) error {
	return exec(ctx, "touch last login", `UPDATE users SET last_login_at = now() WHERE id = $1`, userID)
}

func (pgRepository) InvalidateTokens(ctx context.Context, userID uuid.UUID, purpose Purpose) error {
	return exec(ctx, "invalidate tokens",
		`UPDATE user_tokens SET used_at = now() WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`, userID, purpose)
}

func (pgRepository) InsertToken(ctx context.Context, userID uuid.UUID, purpose Purpose, tokenHash []byte, expiresAt time.Time) error {
	return exec(ctx, "insert token",
		`INSERT INTO user_tokens (user_id, purpose, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, purpose, tokenHash, expiresAt)
}

// ClaimToken atomically marks a valid token as used and returns its user, or uuid.Nil if invalid.
func (pgRepository) ClaimToken(ctx context.Context, tokenHash []byte, purpose Purpose) (uuid.UUID, error) {
	return tokenUser(ctx, `
		UPDATE user_tokens SET used_at = now()
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`, tokenHash, purpose)
}

// PeekToken returns the user of a valid token without consuming it, or uuid.Nil if invalid.
func (pgRepository) PeekToken(ctx context.Context, tokenHash []byte, purpose Purpose) (uuid.UUID, error) {
	return tokenUser(ctx, `
		SELECT user_id FROM user_tokens
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()`, tokenHash, purpose)
}

func tokenUser(ctx context.Context, sql string, args ...any) (uuid.UUID, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, sql, args...).Scan(&id)
	if database.IsNoRows(err) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup token: %w", err)
	}
	return id, nil
}

// SetActivePassword changes the password of an active user. It reports false if the user is not active.
func (pgRepository) SetActivePassword(ctx context.Context, userID uuid.UUID, passwordHash string) (bool, error) {
	return execAffected(ctx, "set password",
		`UPDATE users SET password_hash = $2 WHERE id = $1 AND status = 'active'`, userID, passwordHash)
}

// ActivateInvitedUser sets the first password of an invited user. It reports false if the user is not invited.
func (pgRepository) ActivateInvitedUser(ctx context.Context, userID uuid.UUID, passwordHash string) (bool, error) {
	return execAffected(ctx, "activate invited user", `
		UPDATE users
		SET password_hash = $2, status = 'active', email_verified_at = coalesce(email_verified_at, now())
		WHERE id = $1 AND status = 'invited'`, userID, passwordHash)
}

// MarkEmailVerified verifies the user's email and activates their tenant if it was awaiting verification.
func (pgRepository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	if err := exec(ctx, "mark email verified",
		`UPDATE users SET email_verified_at = now() WHERE id = $1 AND email_verified_at IS NULL`, userID); err != nil {
		return err
	}
	return exec(ctx, "activate pending tenant", `
		UPDATE tenants SET status = 'active'
		WHERE status = 'pending_verification' AND id = (SELECT tenant_id FROM users WHERE id = $1)`, userID)
}

func exec(ctx context.Context, op, sql string, args ...any) error {
	_, err := execAffected(ctx, op, sql, args...)
	return err
}

func execAffected(ctx context.Context, op, sql string, args ...any) (bool, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return false, err
	}
	tag, err := q.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return tag.RowsAffected() > 0, nil
}
