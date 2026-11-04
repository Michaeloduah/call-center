// Package callback handles requests from callers nobody could answer (D5, D23): the caller
// leaves contact details and an agent of the department claims and works the request.
package callback

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/validate"
)

type Status string

const (
	StatusOpen      Status = "open"
	StatusClaimed   Status = "claimed"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
)

func (s Status) Valid() bool {
	return s == StatusOpen || s == StatusClaimed || s == StatusCompleted || s == StatusCancelled
}

const (
	maxNameLen   = 120
	maxReasonLen = 500
	maxNotesLen  = 2000
	// createsPerWindow limits how many callback requests one IP can create.
	createsPerWindow = 5
	createWindow     = 10 * time.Minute
)

// Callback is a callback request as shown to agents and admins.
type Callback struct {
	ID              uuid.UUID  `json:"id"`
	DepartmentID    uuid.UUID  `json:"department_id"`
	DepartmentName  string     `json:"department_name"`
	CallID          *uuid.UUID `json:"call_id"`
	Name            string     `json:"name"`
	Phone           *string    `json:"phone"`
	Email           *string    `json:"email"`
	Reason          string     `json:"reason"`
	Status          Status     `json:"status"`
	ClaimedBy       *uuid.UUID `json:"claimed_by"`
	ClaimedByName   *string    `json:"claimed_by_name"`
	ClaimedAt       *time.Time `json:"claimed_at"`
	ResolutionNotes string     `json:"resolution_notes"`
	ResolvedAt      *time.Time `json:"resolved_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Scope limits what a tenant user may see: admins everything, everyone else only the callbacks
// of departments they are an agent in.
type Scope struct {
	All    bool
	UserID uuid.UUID
}

type Filter struct {
	Status       Status
	DepartmentID uuid.UUID // uuid.Nil = any
}

// CreateInput is submitted by the caller from the widget.
type CreateInput struct {
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

func (in *CreateInput) Validate() error {
	v := validate.Errors{}
	in.Name = v.RequiredString("name", in.Name, maxNameLen)
	in.Phone = v.OptionalPhone("phone", in.Phone)
	in.Email = v.OptionalEmail("email", in.Email)
	in.Reason = v.OptionalString("reason", in.Reason, maxReasonLen)
	// Without a way to reach the caller there is nothing to call back.
	if in.Phone == "" && in.Email == "" {
		v.Add("phone", "Enter a phone number or an email address so we can reach you.")
	}
	return v.Err()
}

// CompleteInput closes a callback with the agent's notes.
type CompleteInput struct {
	Notes string `json:"notes"`
}

func (in *CompleteInput) Validate() error {
	v := validate.Errors{}
	in.Notes = v.OptionalString("notes", in.Notes, maxNotesLen)
	return v.Err()
}

// AssignInput hands a callback to a department member (admin only).
type AssignInput struct {
	UserID uuid.UUID `json:"user_id"`
}

func (in *AssignInput) Validate() error {
	if in.UserID == uuid.Nil {
		return apperr.Validation(map[string]string{"user_id": "Choose a user."})
	}
	return nil
}

var (
	errNotFound    = apperr.NotFound("Callback")
	errTaken       = apperr.Conflict("callback_taken", "Someone else already claimed this callback.")
	errState       = apperr.Conflict("callback_state", "This callback cannot be changed right now.")
	errNotMissed   = apperr.Conflict("call_not_missed", "A callback can only be requested for a call nobody answered.")
	errExists      = apperr.Conflict("callback_exists", "A callback was already requested for this call.")
	errWidget      = apperr.NotFound("Widget")
	errNotMember   = apperr.Validation(map[string]string{"user_id": "Choose someone who is an agent in this department."})
	errAdminOnly   = apperr.Forbidden("Only administrators can do this.")
	errInvalidStat = apperr.Validation(map[string]string{"status": "Unknown status."})
)
