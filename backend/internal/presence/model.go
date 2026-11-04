// Package presence tracks whether agents are available to take calls.
//
// An agent chooses available, away or offline. Whether they are ringing or on a call is derived
// from offers and calls (see Effective), so there is no second copy of that fact to drift.
package presence

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
)

type Status string

const (
	StatusOffline   Status = "offline"
	StatusAvailable Status = "available"
	StatusAway      Status = "away"
)

func (s Status) Valid() bool {
	return s == StatusOffline || s == StatusAvailable || s == StatusAway
}

const (
	// HeartbeatInterval is how often an open console reports in.
	HeartbeatInterval = 10 * time.Second
	// StaleAfter is how long without a heartbeat before an agent counts as gone.
	StaleAfter = 3 * HeartbeatInterval
)

// State is what an agent is doing right now, as shown to the agent and to supervisors.
type State string

const (
	StateOffline State = "offline"
	StateAway    State = "away"
	StateIdle    State = "idle"
	StateRinging State = "ringing"
	StateOnCall  State = "on_call"
)

// Effective combines the stored status with the live facts that refine it.
// An agent whose heartbeat has stopped is offline whatever they last chose.
func Effective(status Status, fresh, hasOffer, hasCall bool) State {
	switch {
	case status == StatusOffline || !fresh:
		return StateOffline
	case status == StatusAway:
		return StateAway
	case hasCall:
		return StateOnCall
	case hasOffer:
		return StateRinging
	default:
		return StateIdle
	}
}

// Presence is one agent's stored presence.
type Presence struct {
	Status          Status    `json:"status"`
	StatusChangedAt time.Time `json:"status_changed_at"`
	LastHeartbeatAt time.Time `json:"-"`
}

var (
	errNotAgent = apperr.Forbidden("You are not assigned to any department, so you cannot take calls.")
	errStatus   = apperr.Validation(map[string]string{"status": "Choose available, away or offline."})
)

// Dispatcher is told when routing may have new work (implemented by routing).
type Dispatcher interface {
	Trigger(tenantID uuid.UUID)
}
