// Package history is the read model behind the call history pages: a list of calls and the
// detail of one call (caller, consent, offer timeline, recording, transcript), scoped by role.
package history

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/chat"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/recording"
)

// Item is one row of the call list.
type Item struct {
	ID              uuid.UUID  `json:"id"`
	Status          string     `json:"status"`
	EndReason       *string    `json:"end_reason"`
	DepartmentID    uuid.UUID  `json:"department_id"`
	DepartmentName  string     `json:"department_name"`
	CallerName      *string    `json:"caller_name"`
	CallerPhone     *string    `json:"caller_phone"`
	AgentName       *string    `json:"agent_name"`
	QueuedAt        time.Time  `json:"queued_at"`
	AnsweredAt      *time.Time `json:"answered_at"`
	EndedAt         *time.Time `json:"ended_at"`
	DurationSeconds *int       `json:"duration_seconds"`
	RecordingStatus *string    `json:"recording_status"`
	MessageCount    int        `json:"message_count"`
}

// OfferEntry is one step of the routing timeline.
type OfferEntry struct {
	AgentName   *string    `json:"agent_name"`
	OfferedAt   time.Time  `json:"offered_at"`
	RespondedAt *time.Time `json:"responded_at"`
	Outcome     string     `json:"outcome"`
}

// Permissions tell the UI which recording actions to offer. The API enforces them regardless.
type Permissions struct {
	PlayRecording   bool `json:"play_recording"`
	DeleteRecording bool `json:"delete_recording"`
}

// CallbackRef links a call to the callback its caller left.
type CallbackRef struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}

// Detail is a call with everything known about it. Caller IP and user agent are stored for abuse
// handling but deliberately not shown.
type Detail struct {
	Item
	CallerEmail    *string         `json:"caller_email"`
	ConsentText    string          `json:"consent_text"`
	ConsentGivenAt time.Time       `json:"consent_given_at"`
	Offers         []OfferEntry    `json:"offers"`
	Recording      *recording.View `json:"recording"`
	Messages       []chat.Message  `json:"messages"`
	Callback       *CallbackRef    `json:"callback"`
	Permissions    Permissions     `json:"permissions"`
}

// Scope decides which calls a user may see: admins all, everyone else the calls of departments
// they supervise plus the calls they handled themselves.
type Scope struct {
	All    bool
	UserID uuid.UUID
}

type Filter struct {
	Status       string
	DepartmentID uuid.UUID // uuid.Nil = any
	AgentID      uuid.UUID // uuid.Nil = any
	From, To     *time.Time
	Query        string
}

var errNotFound = apperr.NotFound("Call")
