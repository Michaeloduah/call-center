// Package limits enforces what a tenant's plan allows: agent seats, departments, widgets, call
// minutes per month and recording storage (D12, D16, D17).
//
// Checks run inside the caller's scoped transaction so a limit and the change it guards are
// evaluated together. They are enforced here, in the service layer, never only in the UI.
package limits

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
)

// Policy says what happens when a minutes or storage cap is reached (D16).
type Policy string

const (
	PolicyBlock   Policy = "block"
	PolicyOverage Policy = "overage"
)

func (p Policy) Valid() bool { return p == PolicyBlock || p == PolicyOverage }

// Kind names a counted resource.
type Kind string

const (
	KindDepartments Kind = "departments"
	KindWidgets     Kind = "widgets"
)

// Caps are a plan's limits. nil means unlimited.
type Caps struct {
	MaxAgents       *int   `json:"max_agents"`
	MaxCallMinutes  *int   `json:"max_call_minutes_per_month"`
	MaxStorageBytes *int64 `json:"max_storage_bytes"`
	MaxDepartments  *int   `json:"max_departments"`
	MaxWidgets      *int   `json:"max_widgets"`
}

// Policies are the tenant's choices for the two soft caps.
type Policies struct {
	Minutes Policy `json:"minutes_cap_policy"`
	Storage Policy `json:"storage_cap_policy"`
}

// Usage is what the tenant has used. Seats, departments and widgets are counts right now; call
// seconds are for the current billing period; storage is everything ever recorded, soft-deleted
// recordings included (D17).
type Usage struct {
	Agents       int   `json:"agents"`
	Departments  int   `json:"departments"`
	Widgets      int   `json:"widgets"`
	CallSeconds  int64 `json:"call_seconds"`
	StorageBytes int64 `json:"storage_bytes"`
}

// Current is a tenant's plan limits, policies and billing period in one read.
type Current struct {
	PlanID      uuid.UUID
	Caps        Caps
	Policies    Policies
	PeriodStart time.Time
	PeriodEnd   time.Time
}

// Person is a user shown in a list of agents without a seat.
type Person struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
}

// Snapshot is everything the billing page needs about limits.
type Snapshot struct {
	Current
	Usage         Usage
	AgentsWithout []Person // agents beyond the seat limit (after a downgrade)
}

// SecondsPerMinute converts the minutes cap to the unit usage is measured in.
const SecondsPerMinute = 60

func planLimit(kind Kind, max int) error {
	return apperr.Conflict("plan_limit", fmt.Sprintf("Your plan allows at most %d %s. Upgrade your plan to add more.", max, kind))
}

var errSeatLimit = func(max int) error {
	return apperr.Conflict("plan_limit", fmt.Sprintf("Your plan allows at most %d agents. Upgrade your plan to add more.", max))
}

// ErrSeatNotHeld is returned when an agent is over the seat limit and may not go available.
var ErrSeatNotHeld = apperr.Conflict("seat_limit", "Your plan has no free agent seat for you right now. Ask your administrator.")
