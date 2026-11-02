// Package department manages a tenant's departments, their agents, supervisors and business hours.
package department

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/reqctx"
)

type Department struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Timezone    string    `json:"timezone"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Summary is a list row.
type Summary struct {
	Department
	MembersCount     int `json:"members_count"`
	SupervisorsCount int `json:"supervisors_count"`
}

type UserRef struct {
	ID       uuid.UUID   `json:"id"`
	FullName string      `json:"full_name"`
	Email    string      `json:"email"`
	Role     reqctx.Role `json:"role"`
	Status   string      `json:"status"`
}

// HoursRange is one opening window on a weekday, in the department's timezone.
// Times are "HH:MM"; Closes may be "24:00" to mean end of day.
type HoursRange struct {
	Weekday int    `json:"weekday"` // 0 = Sunday ... 6 = Saturday
	Opens   string `json:"opens"`
	Closes  string `json:"closes"`
}

type Detail struct {
	Department
	Members       []UserRef    `json:"members"`
	Supervisors   []UserRef    `json:"supervisors"`
	BusinessHours []HoursRange `json:"business_hours"`
}

type Filter struct {
	Query  string
	Active *bool
}

var (
	errNotFound  = apperr.NotFound("Department")
	errNameTaken = apperr.FieldConflict("name", "A department with this name already exists.")
	errInUse     = apperr.Conflict("department_in_use",
		"This department has widgets or call history. Deactivate it instead, or move its widgets to another department first.")
)
