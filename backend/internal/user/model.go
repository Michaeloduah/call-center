// Package user manages the users of a tenant (admins, supervisors, agents).
package user

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/reqctx"
)

const (
	StatusInvited  = "invited"
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

var statuses = []string{StatusInvited, StatusActive, StatusDisabled}

type User struct {
	ID              uuid.UUID   `json:"id"`
	Email           string      `json:"email"`
	FullName        string      `json:"full_name"`
	Role            reqctx.Role `json:"role"`
	Status          string      `json:"status"`
	EmailVerifiedAt *time.Time  `json:"email_verified_at"`
	LastLoginAt     *time.Time  `json:"last_login_at"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// DepartmentRef is a lightweight reference shown on the user detail page.
type DepartmentRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Detail struct {
	User
	Departments           []DepartmentRef `json:"departments"`
	SupervisedDepartments []DepartmentRef `json:"supervised_departments"`
}

// NewUser is the persistence input for creating a user. TenantID is uuid.Nil for platform admins.
type NewUser struct {
	TenantID      uuid.UUID
	Email         string
	FullName      string
	Role          reqctx.Role
	Status        string
	PasswordHash  *string
	EmailVerified bool
}

type Filter struct {
	Query  string
	Role   reqctx.Role
	Status string
}

// Account is the role/status pair other packages use to validate user references.
type Account struct {
	Role   reqctx.Role
	Status string
}

// ErrEmailTaken is returned when an email is already registered (emails are globally unique).
var ErrEmailTaken = apperr.FieldConflict("email", "This email address is already in use.")
