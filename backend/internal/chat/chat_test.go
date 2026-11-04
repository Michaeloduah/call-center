package chat_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/chat"
	"callcenter/internal/media"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
)

type fakeAuth struct{}

func (fakeAuth) Authenticate(_ context.Context, id uuid.UUID, token string) (*call.CallerView, error) {
	if token != "good" {
		return nil, apperr.Unauthenticated("invalid_caller_token", "expired")
	}
	return &call.CallerView{ID: id}, nil
}

type pushed struct {
	room, topic string
	payload     []byte
}

type fakeMedia struct {
	mu   sync.Mutex
	sent []pushed
}

func (m *fakeMedia) SendData(_ context.Context, room, topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, pushed{room, topic, payload})
	return nil
}
func (m *fakeMedia) CreateRoom(context.Context, string) error { return nil }
func (m *fakeMedia) DeleteRoom(context.Context, string) error { return nil }
func (m *fakeMedia) StartRecording(context.Context, string, media.RecordingTarget) (string, error) {
	return "", nil
}
func (m *fakeMedia) JoinToken(string, media.Participant) (string, error) { return "t", nil }
func (m *fakeMedia) PublicURL() string                                   { return "ws://x" }
func (m *fakeMedia) ParseWebhook(*http.Request) (media.Event, error)     { return media.Event{}, nil }

type env struct {
	*dbtest.Fixture
	svc   *chat.Service
	media *fakeMedia
}

func setup(t *testing.T, perMinute int) *env {
	f := dbtest.New(t)
	mp := &fakeMedia{}
	svc := chat.NewService(f.DB, chat.NewRepository(), fakeAuth{}, mp, httpx.NewRateLimiter(perMinute, time.Minute),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &env{Fixture: f, svc: svc, media: mp}
}

func (e *env) agentCtx(tenant, user uuid.UUID) context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: user, TenantID: tenant, Role: reqctx.RoleAgent, FullName: "Grace Agent"})
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}

func TestConversationIsSavedDeliveredAndOrdered(t *testing.T) {
	e := setup(t, 100)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	callID := e.ActiveCall(tenant, dept, agent)

	m1, err := e.svc.SendAsCaller(e.Ctx, callID, "good", chat.Input{Body: "Hello, my order is late"})
	if err != nil || m1.Sender != chat.SenderCaller || m1.Name != "Caller" {
		t.Fatalf("caller message = %+v, %v", m1, err)
	}
	m2, err := e.svc.SendAsAgent(e.agentCtx(tenant, agent), callID, chat.Input{Body: "Let me check"})
	if err != nil || m2.Sender != chat.SenderAgent || m2.Name != "Grace Agent" {
		t.Fatalf("agent message = %+v, %v", m2, err)
	}

	// Delivered into the call's room on the chat topic, exactly what was saved.
	if len(e.media.sent) != 2 || e.media.sent[0].topic != chat.Topic || e.media.sent[0].room != "call-"+callID.String() {
		t.Fatalf("pushed = %+v", e.media.sent)
	}
	var got chat.Message
	if err := json.Unmarshal(e.media.sent[1].payload, &got); err != nil || got.ID != m2.ID || got.Body != "Let me check" {
		t.Fatalf("pushed payload = %+v, %v", got, err)
	}

	fromCaller, err := e.svc.CallerMessages(e.Ctx, callID, "good")
	if err != nil || len(fromCaller) != 2 || fromCaller[0].ID != m1.ID || fromCaller[1].ID != m2.ID {
		t.Fatalf("caller transcript = %+v, %v", fromCaller, err)
	}
	fromAgent, err := e.svc.AgentMessages(e.agentCtx(tenant, agent), callID)
	if err != nil || len(fromAgent) != 2 {
		t.Fatalf("agent transcript = %+v, %v", fromAgent, err)
	}
	if _, err := e.svc.CallerMessages(e.Ctx, callID, "bad"); err == nil {
		t.Fatal("transcript served to a caller with the wrong token")
	}
}

func TestChatOnlyWhileCallIsActive(t *testing.T) {
	e := setup(t, 100)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)

	queued := e.Call(tenant, dept, time.Second)
	_, err := e.svc.SendAsCaller(e.Ctx, queued, "good", chat.Input{Body: "anyone there?"})
	wantCode(t, err, "chat_unavailable") // D24: only once an agent has joined

	ended := e.EndedCall(tenant, dept, agent, 2)
	_, err = e.svc.SendAsCaller(e.Ctx, ended, "good", chat.Input{Body: "too late"})
	wantCode(t, err, "chat_unavailable")
	_, err = e.svc.SendAsAgent(e.agentCtx(tenant, agent), ended, chat.Input{Body: "too late"})
	wantCode(t, err, "not_found")

	var n int
	e.Query(`SELECT count(*)::int FROM call_messages WHERE tenant_id = $1`, []any{tenant}, &n)
	if n != 0 || len(e.media.sent) != 0 {
		t.Fatalf("%d messages saved and %d delivered for calls that were not active", n, len(e.media.sent))
	}
}

func TestOnlyTheAssignedAgentCanChat(t *testing.T) {
	e := setup(t, 100)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent, colleague := e.Agent(tenant, dept), e.Agent(tenant, dept)
	callID := e.ActiveCall(tenant, dept, agent)

	_, err := e.svc.SendAsAgent(e.agentCtx(tenant, colleague), callID, chat.Input{Body: "I'm not on this call"})
	wantCode(t, err, "not_found")
	if _, err := e.svc.AgentMessages(e.agentCtx(tenant, colleague), callID); err == nil {
		t.Fatal("another agent read the transcript")
	}

	otherTenant := e.Tenant()
	otherDept := e.Department(otherTenant, "Support")
	stranger := e.Agent(otherTenant, otherDept)
	if _, err := e.svc.SendAsAgent(e.agentCtx(otherTenant, stranger), callID, chat.Input{Body: "x"}); err == nil {
		t.Fatal("an agent of another tenant sent into the call")
	}
}

func TestMessagesAreRateLimitedPerCall(t *testing.T) {
	e := setup(t, 3)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	callID := e.ActiveCall(tenant, dept, e.Agent(tenant, dept))

	for i := range 3 {
		if _, err := e.svc.SendAsCaller(e.Ctx, callID, "good", chat.Input{Body: strings.Repeat("a", i+1)}); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
	}
	_, err := e.svc.SendAsCaller(e.Ctx, callID, "good", chat.Input{Body: "spam"})
	wantCode(t, err, "too_many_requests")

	other := e.ActiveCall(tenant, dept, e.Agent(tenant, dept))
	if _, err := e.svc.SendAsCaller(e.Ctx, other, "good", chat.Input{Body: "fine"}); err != nil {
		t.Fatalf("another call was throttled too: %v", err)
	}
}

func TestInputValidation(t *testing.T) {
	for name, body := range map[string]string{"empty": "", "blank": "   \n ", "too long": strings.Repeat("x", chat.MaxBodyRunes+1)} {
		in := chat.Input{Body: body}
		if err := in.Validate(); err == nil {
			t.Errorf("%s message accepted", name)
		}
	}
	in := chat.Input{Body: "  hello  "}
	if err := in.Validate(); err != nil || in.Body != "hello" {
		t.Errorf("valid message: %q, %v", in.Body, err)
	}
	long := chat.Input{Body: strings.Repeat("é", chat.MaxBodyRunes)} // limit counts characters, not bytes
	if err := long.Validate(); err != nil {
		t.Errorf("a message of exactly %d characters was rejected: %v", chat.MaxBodyRunes, err)
	}
}
