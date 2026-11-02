// Package media abstracts the real-time media server (LiveKit). Services depend on Provider,
// so call logic can be tested without a running media server.
package media

import (
	"context"
	"net/http"
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
)

// Event is a verified webhook notification.
type Event struct {
	ID                  string
	Kind                EventKind
	Room                string
	ParticipantIdentity string
}

type Provider interface {
	CreateRoom(ctx context.Context, name string) error
	// DeleteRoom disconnects everyone and closes the room. A missing room is not an error.
	DeleteRoom(ctx context.Context, name string) error
	JoinToken(room string, p Participant) (string, error)
	// PublicURL is the address browsers connect to.
	PublicURL() string
	// ParseWebhook verifies the request signature and decodes the event.
	ParseWebhook(r *http.Request) (Event, error)
}
