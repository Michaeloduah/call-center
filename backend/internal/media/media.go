// Package media abstracts the real-time media server (LiveKit). Services depend on Provider,
// so call logic can be tested without a running media server.
package media

import (
	"context"
	"net/http"
	"time"
)

// Role decides what a participant may do in a call room.
type Role int

const (
	// RoleCaller may publish microphone audio and listen. Chat goes through the API, not the data channel.
	RoleCaller Role = iota + 1
	// RoleAgent may publish microphone audio and listen.
	RoleAgent
)

// Participant identifies who a join token is for.
type Participant struct {
	Identity string // stable, unique within the room (e.g. "caller:<call id>")
	Name     string // display name shown to the other side
	Role     Role
}

// EventKind is the subset of media-server events the application reacts to.
type EventKind string

const (
	EventParticipantJoined EventKind = "participant_joined"
	EventParticipantLeft   EventKind = "participant_left"
	EventRoomFinished      EventKind = "room_finished"
	EventEgressEnded       EventKind = "egress_ended"
)

// RecordingTarget says where a recording is written. The recorder uploads it itself.
type RecordingTarget struct {
	Key       string // object key
	Endpoint  string
	Bucket    string
	AccessKey string
	Secret    string
	Region    string
}

// RecordingResult is the outcome of a finished recording.
type RecordingResult struct {
	Succeeded bool
	Error     string
	SizeBytes int64
	Duration  time.Duration
}

// Event is a verified webhook notification.
type Event struct {
	ID                  string
	Kind                EventKind
	Room                string
	ParticipantIdentity string
	EgressID            string           // egress events
	Recording           *RecordingResult // set on EventEgressEnded
}

// Listener reacts to verified media events.
type Listener interface {
	HandleMediaEvent(ctx context.Context, ev Event) error
}

// Listeners delivers an event to several listeners, stopping at the first error so the media
// server redelivers the event (handlers are idempotent).
type Listeners []Listener

func (ls Listeners) HandleMediaEvent(ctx context.Context, ev Event) error {
	for _, l := range ls {
		if err := l.HandleMediaEvent(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

type Provider interface {
	CreateRoom(ctx context.Context, name string) error
	// DeleteRoom disconnects everyone and closes the room. A missing room is not an error.
	DeleteRoom(ctx context.Context, name string) error
	JoinToken(room string, p Participant) (string, error)
	// StartRecording records the room's audio, mixed into one file, and returns the recorder's id
	// for it. The recording ends by itself when the room closes.
	StartRecording(ctx context.Context, room string, target RecordingTarget) (egressID string, err error)
	// SendData delivers a message to everyone in the room over the media connection.
	SendData(ctx context.Context, room, topic string, payload []byte) error
	// PublicURL is the address browsers connect to.
	PublicURL() string
	// ParseWebhook verifies the request signature and decodes the event.
	ParseWebhook(r *http.Request) (Event, error)
}
