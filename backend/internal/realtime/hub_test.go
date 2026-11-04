package realtime

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHubWakesOnlyInterestedSubscribers(t *testing.T) {
	h := NewHub()
	mine, theirs := uuid.New(), uuid.New()
	topicA, topicB := UserTopic(uuid.New()), UserTopic(uuid.New())

	chA, cancelA, err := h.Subscribe(mine, []string{topicA})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelA()
	chB, cancelB, err := h.Subscribe(theirs, []string{topicB})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelB()

	h.dispatch(topicA)
	select {
	case <-chA:
	case <-time.After(time.Second):
		t.Fatal("subscriber to topicA was not woken")
	}
	select {
	case <-chB:
		t.Fatal("subscriber to topicB was woken by topicA")
	default:
	}
}

func TestHubCoalescesBurstsIntoOneWakeup(t *testing.T) {
	h := NewHub()
	topic := DepartmentTopic(uuid.New())
	ch, cancel, _ := h.Subscribe(uuid.New(), []string{topic})
	defer cancel()
	for range 20 {
		h.dispatch(topic) // must never block, even when nobody is reading
	}
	<-ch
	select {
	case <-ch:
		t.Fatal("burst delivered more than one pending wakeup")
	default:
	}
}

func TestHubLimitsStreamsPerUser(t *testing.T) {
	h := NewHub()
	user := uuid.New()
	var cancels []func()
	for range maxStreamsPerUser {
		_, cancel, err := h.Subscribe(user, nil)
		if err != nil {
			t.Fatalf("subscribe within the limit: %v", err)
		}
		cancels = append(cancels, cancel)
	}
	if _, _, err := h.Subscribe(user, nil); err == nil {
		t.Fatal("subscribed beyond the per-user limit")
	}
	cancels[0]()
	cancels[0]() // cancel is idempotent
	if _, cancel, err := h.Subscribe(user, nil); err != nil {
		t.Fatalf("subscribe after a stream closed: %v", err)
	} else {
		cancel()
	}
	if _, _, err := h.Subscribe(uuid.New(), nil); err != nil {
		t.Fatalf("another user hit the limit: %v", err)
	}
}

func TestStreamSendsReadyThenChanged(t *testing.T) {
	h := NewHub()
	topic := UserTopic(uuid.New())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h.Stream(w, r, uuid.New(), []string{topic})
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	waitFor := func(prefix string) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("stream closed before %q", prefix)
				}
				if strings.HasPrefix(l, prefix) {
					return
				}
			case <-deadline:
				t.Fatalf("timed out waiting for %q", prefix)
			}
		}
	}
	waitFor("event: ready")
	h.dispatch(topic)
	waitFor("event: changed")
}
