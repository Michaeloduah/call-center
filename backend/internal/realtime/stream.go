package realtime

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	keepAliveInterval = 20 * time.Second
	// Streams end after this long; browsers reconnect on their own, which also refreshes
	// anything the topic list was derived from (for example a supervisor's departments).
	maxStreamAge = 15 * time.Minute
	// Clients wait this long before reconnecting (milliseconds, sent as the SSE "retry" field).
	reconnectDelayMillis = 3000
)

// Stream serves a Server-Sent Events response that emits a "changed" event whenever one of
// topics is published, until the client disconnects or the stream reaches its maximum age.
func (h *Hub) Stream(w http.ResponseWriter, r *http.Request, key uuid.UUID, topics []string) error {
	rc := http.NewResponseController(w)
	flush := func() error { return rc.Flush() }
	// The server's WriteTimeout would cut a long-lived stream; each write is bounded here instead.
	setDeadline := func() { _ = rc.SetWriteDeadline(time.Now().Add(2 * keepAliveInterval)) }

	ch, cancel, err := h.Subscribe(key, topics)
	if err != nil {
		return err
	}
	defer cancel()

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Accel-Buffering", "no") // reverse proxies must not buffer the stream
	w.WriteHeader(http.StatusOK)
	setDeadline()
	if _, err := fmt.Fprintf(w, "retry: %d\nevent: ready\ndata: {}\n\n", reconnectDelayMillis); err != nil {
		return nil
	}
	if err := flush(); err != nil {
		return nil
	}

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()
	expire := time.NewTimer(maxStreamAge)
	defer expire.Stop()
	for {
		var frame string
		select {
		case <-r.Context().Done():
			return nil
		case <-expire.C:
			return nil
		case <-ch:
			frame = "event: changed\ndata: {}\n\n"
		case <-keepAlive.C:
			frame = ": keep-alive\n\n"
		}
		setDeadline()
		if _, err := fmt.Fprint(w, frame); err != nil {
			return nil
		}
		if err := flush(); err != nil {
			return nil
		}
	}
}
