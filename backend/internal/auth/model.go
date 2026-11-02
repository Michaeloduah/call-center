// Package auth handles authentication: sessions, sign-up, email verification,
// password reset and invitations.
package auth

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/reqctx"
)

// Purpose identifies what a one-time emailed token is for.
type Purpose string

const (
	PurposeVerifyEmail   Purpose = "verify_email"
	PurposeResetPassword Purpose = "reset_password"
	PurposeInvite        Purpose = "invite"
)

var tokenTTL = map[Purpose]time.Duration{
	PurposeVerifyEmail:   24 * time.Hour,
	PurposeResetPassword: time.Hour,
	PurposeInvite:        7 * 24 * time.Hour,
}

const (
	StatusInvited  = "invited"
	StatusActive   = "active"
	StatusDisabled = "disabled"

	TenantStatusPending   = "pending_verification"
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
)

// credentials is what login and token flows need to know about a user.
type credentials struct {
	UserID          uuid.UUID
	TenantID        *uuid.UUID
	TenantName      *string
	TenantStatus    *string
	Email           string
	FullName        string
	Role            reqctx.Role
	Status          string
	PasswordHash    *string
	EmailVerifiedAt *time.Time
}

// Me is the authenticated user's profile returned by /v1/auth/me and login.
type Me struct {
	ID       uuid.UUID   `json:"id"`
	Email    string      `json:"email"`
	FullName string      `json:"full_name"`
	Role     reqctx.Role `json:"role"`
	Tenant   *MeTenant   `json:"tenant"`
}

type MeTenant struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func (c credentials) me() Me {
	m := Me{ID: c.UserID, Email: c.Email, FullName: c.FullName, Role: c.Role}
	if c.TenantID != nil && c.TenantName != nil {
		m.Tenant = &MeTenant{ID: *c.TenantID, Name: *c.TenantName}
	}
	return m
}

// SignupAccount is the data needed to provision a self-service tenant.
type SignupAccount struct {
	CompanyName  string
	FullName     string
	Email        string
	PasswordHash string
}

// ProvisionedAccount identifies the tenant and admin created by sign-up.
type ProvisionedAccount struct {
	TenantID   uuid.UUID
	TenantName string
	UserID     uuid.UUID
}

// InviteInfo is shown on the accept-invite page before the user sets a password.
type InviteInfo struct {
	Email      string  `json:"email"`
	FullName   string  `json:"full_name"`
	TenantName *string `json:"tenant_name"`
}

// Errors returned to clients.
var (
	errInvalidCredentials = apperr.Unauthenticated("invalid_credentials", "Incorrect email or password.")
	errAccountDisabled    = apperr.Unauthenticated("account_disabled", "This account has been disabled. Contact your administrator.")
	errEmailNotVerified   = apperr.Unauthenticated("email_not_verified", "Please verify your email address before signing in.")
	errTenantSuspended    = apperr.Unauthenticated("tenant_suspended", "This organisation's account is suspended. Contact support.")
	errInvalidSession     = apperr.Unauthenticated("unauthenticated", "Your session has expired. Please sign in again.")
	errInvalidToken       = &apperr.Error{Kind: apperr.KindValidation, Code: "invalid_token", Message: "This link is invalid or has expired."}
)
