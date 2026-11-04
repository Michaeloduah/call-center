// Package recording records answered calls, keeps their metadata and controls who may play,
// delete or restore them (D6, D7, D17).
package recording

import (
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
)

type Status string

const (
	StatusStarting  Status = "starting"  // row exists, the recorder has not accepted the request yet
	StatusRecording Status = "recording" // the recorder is running
	StatusReady     Status = "ready"
	StatusFailed    Status = "failed"
)

const (
	// AccessLinkTTL is how long a presigned playback/download link works.
	AccessLinkTTL = 5 * time.Minute
	// startTimeout bounds the background work of starting a recording.
	startTimeout = 30 * time.Second
	// sweepInterval is how often unfinished recordings are checked.
	sweepInterval = 30 * time.Second
	// startingStuckAfter: a row still "starting" this long after creation never got a recorder.
	startingStuckAfter = time.Minute
	// finalizeGrace: a recording still running this long after its call ended never reported back.
	finalizeGrace = 10 * time.Minute
	contentType   = "audio/ogg"
)

// View is a recording as shown on the call history. The storage key is never exposed.
type View struct {
	ID              uuid.UUID  `json:"id"`
	Status          Status     `json:"status"`
	DurationSeconds *int       `json:"duration_seconds"`
	SizeBytes       *int64     `json:"size_bytes"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	Error           *string    `json:"error"`
}

// AccessGrant is a short-lived link to a recording.
type AccessGrant struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Located is what an access decision needs to know about a recording.
type Located struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	CallID   uuid.UUID
	Status   Status
	Key      *string
}

// DeletedItem is a soft-deleted recording as listed for the platform admin.
type DeletedItem struct {
	ID              uuid.UUID `json:"id"`
	CallID          uuid.UUID `json:"call_id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	TenantName      string    `json:"tenant_name"`
	DepartmentName  string    `json:"department_name"`
	CallerName      *string   `json:"caller_name"`
	DurationSeconds *int      `json:"duration_seconds"`
	SizeBytes       *int64    `json:"size_bytes"`
	DeletedAt       time.Time `json:"deleted_at"`
	DeletedByEmail  *string   `json:"deleted_by_email"`
}

var (
	errNotFound    = apperr.NotFound("Recording")
	errUnavailable = apperr.Conflict("recording_unavailable", "This recording is not available.")
)
