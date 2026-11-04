// Package chat carries text messages between a caller and the agent on their call (D14, D24).
//
// A message is validated and saved first, then pushed into the call's media room. There is one
// write path, so the saved transcript and what the two people saw cannot diverge.
package chat

import (
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/validate"
)

const (
	// Topic labels chat packets on the media connection so clients can tell them from other data.
	Topic = "chat"
	// MaxBodyRunes bounds a single message.
	MaxBodyRunes = 1000
	// messagesPerMinute limits how fast one call can chat.
	messagesPerMinute = 20
	// pushTimeout bounds delivery to the media room after the message is saved.
	pushTimeout = 5 * time.Second
)

type Sender string

const (
	SenderCaller Sender = "caller"
	SenderAgent  Sender = "agent"
)

// Message is one chat line, as stored and as pushed to clients.
type Message struct {
	ID     uuid.UUID `json:"id"`
	Sender Sender    `json:"sender"`
	Name   string    `json:"name"`
	Body   string    `json:"body"`
	SentAt time.Time `json:"sent_at"`
}

// Input is the body of the send endpoints.
type Input struct {
	Body string `json:"body"`
}

func (in *Input) Validate() error {
	v := validate.Errors{}
	in.Body = v.RequiredString("body", in.Body, MaxBodyRunes)
	if utf8.RuneCountInString(in.Body) == 0 {
		v.Add("body", "Write a message first.")
	}
	return v.Err()
}

var (
	errUnavailable = apperr.Conflict("chat_unavailable", "Chat is only available while you are on a call.")
	errNoCall      = apperr.NotFound("Call")
)
