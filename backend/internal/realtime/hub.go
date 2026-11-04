// Package realtime pushes "something changed" hints to browsers.
//
// Events carry no data. A client that receives one re-reads its state over a normal
// authenticated request, so a lost or reordered event can never leave a screen wrong for long.
package realtime

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
)

// channel is the Postgres NOTIFY channel all hints travel on.
const channel = "cc_events"

// maxStreamsPerUser caps open event streams per user (a few tabs, not unlimited).
const maxStreamsPerUser = 3

// UserTopic addresses one user's console (offers, presence, their call).
func UserTopic(id uuid.UUID) string { return "user:" + id.String() }

// DepartmentTopic addresses everyone watching a department (live board).
func DepartmentTopic(id uuid.UUID) string { return "dept:" + id.String() }

// Publish sends a hint on each topic inside the current transaction; it is delivered only if
// the transaction commits.
func Publish(ctx context.Context, topics ...string) error {
	for _, t := range topics {
		if err := database.Notify(ctx, channel, t); err != nil {
			return err
		}
	}
	return nil
}

var errTooManyStreams = apperr.TooManyRequests()

// Hub fans hints received from Postgres out to the open streams of this process.
type Hub struct {
	mu     sync.Mutex
	subs   map[*subscription]struct{}
	perKey map[uuid.UUID]int
}

type subscription struct {
	key    uuid.UUID
	topics map[string]struct{}
	// ch has room for one pending hint: while a hint is unread, further ones add nothing.
	ch chan struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[*subscription]struct{}{}, perKey: map[uuid.UUID]int{}}
}

// Run feeds the hub from Postgres until ctx ends.
func (h *Hub) Run(ctx context.Context, db *database.DB, logger *slog.Logger) {
	db.Listen(ctx, channel, logger, h.dispatch)
}

// dispatch wakes every subscription interested in topic.
func (h *Hub) dispatch(topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if _, ok := s.topics[topic]; !ok {
			continue
		}
		select {
		case s.ch <- struct{}{}:
		default:
		}
	}
}

// Subscribe registers interest in topics for a user (key). The returned channel receives a value
// when any topic changes; call cancel when the stream ends.
func (h *Hub) Subscribe(key uuid.UUID, topics []string) (<-chan struct{}, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.perKey[key] >= maxStreamsPerUser {
		return nil, nil, errTooManyStreams
	}
	s := &subscription{key: key, topics: make(map[string]struct{}, len(topics)), ch: make(chan struct{}, 1)}
	for _, t := range topics {
		s.topics[t] = struct{}{}
	}
	h.subs[s] = struct{}{}
	h.perKey[key]++
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[s]; !ok {
			return
		}
		delete(h.subs, s)
		if h.perKey[key]--; h.perKey[key] == 0 {
			delete(h.perKey, key)
		}
	}
	return s.ch, cancel, nil
}
