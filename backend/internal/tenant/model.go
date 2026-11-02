// Package tenant manages organisations (tenants) on the platform.
package tenant

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
)

const (
	StatusPending   = "pending_verification"
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

var statuses = []string{StatusPending, StatusActive, StatusSuspended}

type Tenant struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Summary is a list row.
type Summary struct {
	Tenant
	UsersCount int `json:"users_count"`
}

type AdminRef struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
	Email    string    `json:"email"`
	Status   string    `json:"status"`
}

type Detail struct {
	Tenant
	UsersCount       int        `json:"users_count"`
	DepartmentsCount int        `json:"departments_count"`
	Admins           []AdminRef `json:"admins"`
}

type Filter struct {
	Query  string
	Status string
}

var (
	errNotFound  = apperr.NotFound("Tenant")
	errSlugTaken = apperr.FieldConflict("slug", "This slug is already in use.")
)
