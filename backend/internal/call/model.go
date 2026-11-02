// Package call manages the lifecycle of customer calls: creation from a widget, queueing,
// and ending. Routing to agents arrives in M3.
package call

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRinging   Status = "ringing"
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusAbandoned Status = "abandoned"
	StatusMissed    Status = "missed"
	StatusFailed    Status = "failed"
)

// Reasons recorded in calls.end_reason.
const (
	EndCallerHungUp     = "caller_hung_up"
	EndCallerLeft       = "caller_left"
	EndRoomClosed       = "room_closed"
	EndMediaUnavailable = "media_unavailable"
)

const (
	// How long a caller's token stays valid for status checks and hanging up.
	callerTokenTTL = 4 * time.Hour
	// Soft cap on simultaneous live calls per tenant; protects against widget abuse.
	maxLiveCallsPerTenant = 50
	callerIdentityPrefix  = "caller:"
)

// NewCall is the persistence input for a queued call.
type NewCall struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	WidgetID             uuid.UUID
	DepartmentID         uuid.UUID
	RoomName             string
	CallerName           string
	CallerPhone          string
	CallerEmail          string
	ConsentText          string
	CallerTokenHash      []byte
	CallerTokenExpiresAt time.Time
	CallerIP             string
	CallerUserAgent      string
}

// CallerView is what a caller (authenticated by their caller token) may see about their call.
type CallerView struct {
	ID            uuid.UUID `json:"id"`
	Status        Status    `json:"status"`
	QueuePosition *int      `json:"queue_position"` // only while queued
	EndReason     *string   `json:"end_reason"`
	DepartmentID  uuid.UUID `json:"-"`
	RoomName      string    `json:"-"`
	QueuedAt      time.Time `json:"-"`
}

// CreateResult is returned to the widget when a call starts.
type CreateResult struct {
	CallID      uuid.UUID `json:"call_id"`
	CallerToken string    `json:"caller_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	Status      Status    `json:"status"`
	MediaURL    string    `json:"media_url"`
	MediaToken  string    `json:"media_token"`
}

var (
	errWidgetNotFound  = apperr.NotFound("Widget")
	errClosed          = apperr.Conflict("closed", "We're closed right now. Please call back during our opening hours.")
	errUnavailable     = apperr.Conflict("unavailable", "Calls to this team are not available right now.")
	errBusy            = apperr.Conflict("busy", "All our lines are busy. Please try again in a few minutes.")
	errInvalidCaller   = apperr.Unauthenticated("invalid_caller_token", "This call session has expired.")
	errConsentRequired = apperr.Validation(map[string]string{"consent": "You must agree to the recording to start the call."})
)
