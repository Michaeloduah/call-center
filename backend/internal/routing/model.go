// Package routing offers queued calls to available agents and manages the offer lifecycle:
// accept, decline, timeout, cancellation and the maximum wait.
package routing

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/presence"
)

const (
	// SweepInterval is how often the sweeper looks for expired offers, overdue calls and stale agents.
	// Triggers make the common case instant; the sweeper is what makes the system self-healing.
	SweepInterval = 2 * time.Second
	// TimeoutsBeforeAway is how many unanswered offers in a row make an agent go away.
	TimeoutsBeforeAway = 2
	// triggerBuffer is how many tenants may wait for a dispatch pass. When it is full a trigger is
	// dropped, which is safe: the sweeper visits every tenant that has waiting calls.
	triggerBuffer = 256
)

// Affected names what a routing change touched, so the right screens get a hint.
type Affected struct {
	CallID       uuid.UUID
	DepartmentID uuid.UUID
	AgentID      *uuid.UUID
}

// Timings are a tenant's routing settings.
type Timings struct {
	OfferTimeout time.Duration
	MaxQueueWait time.Duration
}

// QueuedCall is a call waiting for an agent.
type QueuedCall struct {
	ID           uuid.UUID
	DepartmentID uuid.UUID
}

// OfferView is the ringing call shown to an agent. The caller's phone and email are withheld
// until the agent accepts; the name is enough to decide.
type OfferView struct {
	ID             uuid.UUID `json:"id"`
	CallID         uuid.UUID `json:"call_id"`
	DepartmentName string    `json:"department_name"`
	CallerName     *string   `json:"caller_name"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// ActiveCallView is the call an agent is on.
type ActiveCallView struct {
	ID             uuid.UUID `json:"id"`
	DepartmentName string    `json:"department_name"`
	CallerName     *string   `json:"caller_name"`
	CallerPhone    *string   `json:"caller_phone"`
	CallerEmail    *string   `json:"caller_email"`
	AnsweredAt     time.Time `json:"answered_at"`
}

// Session is everything an agent console needs to draw itself.
type Session struct {
	Status presence.Status `json:"status"`
	State  presence.State  `json:"state"`
	Offer  *OfferView      `json:"offer"`
	Call   *ActiveCallView `json:"call"`
	// OpenCallbacks counts callback requests waiting in the agent's departments.
	OpenCallbacks            int `json:"open_callbacks"`
	HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
}

// AcceptResult is returned when an agent takes a call.
type AcceptResult struct {
	Call       ActiveCallView `json:"call"`
	MediaURL   string         `json:"media_url"`
	MediaToken string         `json:"media_token"`
}

// MediaGrant lets an agent (re)join the room of the call they are on.
type MediaGrant struct {
	MediaURL   string `json:"media_url"`
	MediaToken string `json:"media_token"`
}

// Recorder records a call that has just been answered. Start must return immediately.
type Recorder interface {
	Start(tenantID, callID uuid.UUID, room string)
}

var (
	errOfferGone = apperr.Conflict("offer_expired", "This call is no longer available.")
	errNotAgent  = apperr.Forbidden("You are not assigned to any department, so you cannot take calls.")
	errNoCall    = apperr.NotFound("Call")
)
