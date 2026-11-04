// Package widget manages the embeddable "Call us" widgets that route callers to a department.
package widget

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/department"
	"callcenter/internal/platform/apperr"
)

// FieldMode controls one caller-form field (D8).
type FieldMode string

const (
	FieldHidden   FieldMode = "hidden"
	FieldOptional FieldMode = "optional"
	FieldRequired FieldMode = "required"
)

func (m FieldMode) Valid() bool {
	return m == FieldHidden || m == FieldOptional || m == FieldRequired
}

// CallerForm is the pre-call form configuration. When Enabled is false every field is hidden.
type CallerForm struct {
	Enabled bool      `json:"enabled"`
	Name    FieldMode `json:"name"`
	Phone   FieldMode `json:"phone"`
	Email   FieldMode `json:"email"`
}

// Effective returns the mode actually applied to callers, accounting for a disabled form.
func (f CallerForm) Effective(m FieldMode) FieldMode {
	if !f.Enabled {
		return FieldHidden
	}
	return m
}

type Widget struct {
	ID             uuid.UUID  `json:"id"`
	DepartmentID   uuid.UUID  `json:"department_id"`
	DepartmentName string     `json:"department_name"`
	Name           string     `json:"name"`
	PublicKey      string     `json:"public_key"`
	AllowedOrigins []string   `json:"allowed_origins"`
	IsActive       bool       `json:"is_active"`
	ButtonLabel    string     `json:"button_label"`
	AccentColor    string     `json:"accent_color"`
	Greeting       string     `json:"greeting"`
	CallerForm     CallerForm `json:"caller_form"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Filter struct {
	Query string
}

// Availability tells the caller whether a call can be started right now.
type Availability string

const (
	AvailabilityOpen        Availability = "open"
	AvailabilityClosed      Availability = "closed"      // outside business hours
	AvailabilityUnavailable Availability = "unavailable" // department deactivated
	AvailabilityNoAgents    Availability = "no_agents"   // open, but no agent is online to take a call
)

// Resolved is everything the public side needs about a widget, loaded in one query.
// Only active widgets of active tenants resolve.
type Resolved struct {
	WidgetID         uuid.UUID
	TenantID         uuid.UUID
	TenantName       string
	DepartmentID     uuid.UUID
	DepartmentName   string
	DepartmentActive bool
	// AgentsAvailable is true when at least one department member is available with a live heartbeat.
	// Agents currently on a call still count: the caller waits in the queue for them.
	AgentsAvailable bool
	Timezone        string
	Hours           []department.HoursRange
	PublicKey       string
	AllowedOrigins  []string
	ButtonLabel     string
	AccentColor     string
	Greeting        string
	CallerForm      CallerForm
	ConsentText     string // tenant override, else platform default
}

// PublicConfig is the caller-facing widget configuration. It contains nothing secret.
type PublicConfig struct {
	PublicKey      string                  `json:"public_key"`
	TenantName     string                  `json:"tenant_name"`
	DepartmentName string                  `json:"department_name"`
	ButtonLabel    string                  `json:"button_label"`
	AccentColor    string                  `json:"accent_color"`
	Greeting       string                  `json:"greeting"`
	CallerForm     CallerForm              `json:"caller_form"`
	ConsentText    string                  `json:"consent_text"`
	Availability   Availability            `json:"availability"`
	Timezone       string                  `json:"timezone"`
	BusinessHours  []department.HoursRange `json:"business_hours"`
	// FrameAncestors are the origins allowed to embed the widget (sent as CSP frame-ancestors).
	FrameAncestors []string `json:"frame_ancestors"`
}

var (
	errNotFound           = apperr.NotFound("Widget")
	errDepartmentNotFound = apperr.Validation(map[string]string{"department_id": "Choose an existing department."})
)
