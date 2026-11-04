// Package call manages the lifecycle of customer calls: creation from a widget, queueing,
// and ending. Offering calls to agents is the routing package's job.
package call

import (
	"context"
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
	EndAgentEnded       = "agent_ended"
	EndAgentLeft        = "agent_left"
	EndMaxWait          = "max_wait_exceeded"
)

const (
	// How long a caller's token stays valid for status checks and hanging up.
	callerTokenTTL = 4 * time.Hour
	// Soft cap on simultaneous live calls per tenant; protects against widget abuse.
	maxLiveCallsPerTenant = 50
	callerIdentityPrefix  = "caller:"
	// AgentIdentityPrefix marks LiveKit participants that are agents ("agent:<user id>").
	AgentIdentityPrefix = "agent:"
)

// EndedCall describes a call that has just reached a terminal state.
type EndedCall struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	DepartmentID uuid.UUID
	RoomName     string
	AgentID      *uuid.UUID // nil if no agent had picked the call up
	Reason       string
}

// Router is what the call lifecycle needs from routing. Keeping it a narrow interface lets
// call stay free of routing's offer rules (and lets tests fake it).
type Router interface {
	// CallEnded runs inside the transaction that ended the call: it cancels a pending offer,
	// frees the agent and notifies listeners, so all of it commits or none of it does.
	CallEnded(ctx context.Context, e EndedCall) error
	// Trigger asks routing to look for work once the surrounding transaction has committed.
	Trigger(tenantID uuid.UUID)
}

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
	errCallNotFound    = apperr.NotFound("Call")
	errClosed          = apperr.Conflict("closed", "We're closed right now. Please call back during our opening hours.")
	errUnavailable     = apperr.Conflict("unavailable", "Calls to this team are not available right now.")
	errLimitReached    = apperr.Conflict("limit_reached", "We can't take calls right now. Please leave your details and we'll call you back.")
	errNoAgents        = apperr.Conflict("no_agents", "Nobody is available to take your call right now. Please try again shortly.")
	errBusy            = apperr.Conflict("busy", "All our lines are busy. Please try again in a few minutes.")
	errInvalidCaller   = apperr.Unauthenticated("invalid_caller_token", "This call session has expired.")
	errConsentRequired = apperr.Validation(map[string]string{"consent": "You must agree to the recording to start the call."})
)
