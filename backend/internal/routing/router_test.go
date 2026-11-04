package routing_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/media"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/presence"
	"callcenter/internal/routing"
)

// These tests run against a real Postgres (see `make test`) because the routing rules live in SQL.

type fakeMedia struct {
	mu      sync.Mutex
	deleted []string
}

func (m *fakeMedia) CreateRoom(context.Context, string) error { return nil }
func (m *fakeMedia) DeleteRoom(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, name)
	return nil
}
func (m *fakeMedia) JoinToken(string, media.Participant) (string, error) { return "token", nil }
func (m *fakeMedia) StartRecording(context.Context, string, media.RecordingTarget) (string, error) {
	return "EG_test", nil
}
func (m *fakeMedia) SendData(context.Context, string, string, []byte) error { return nil }
func (m *fakeMedia) PublicURL() string                                      { return "ws://livekit.test" }
func (m *fakeMedia) ParseWebhook(*http.Request) (media.Event, error)        { return media.Event{}, nil }

type fakeRecorder struct {
	mu      sync.Mutex
	started []uuid.UUID
}

func (r *fakeRecorder) Start(_, callID uuid.UUID, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, callID)
}

type env struct {
	*dbtest.Fixture
	router *routing.Router
	svc    *routing.Service
	media  *fakeMedia
	rec    *fakeRecorder
}

func setup(t *testing.T) *env {
	f := dbtest.New(t)
	mp := &fakeMedia{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	members := presence.NewRepository()
	repo := routing.NewRepository()
	router := routing.NewRouter(f.DB, repo, members, mp, logger)
	rec := &fakeRecorder{}
	return &env{Fixture: f, router: router, svc: routing.NewService(f.DB, repo, members, mp, router, rec), media: mp, rec: rec}
}

func (e *env) dispatch(tenant uuid.UUID) {
	e.T.Helper()
	if err := e.router.Dispatch(e.Ctx, tenant); err != nil {
		e.T.Fatalf("dispatch: %v", err)
	}
}

func (e *env) as(tenant, user uuid.UUID) context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: user, TenantID: tenant, Role: reqctx.RoleAgent, FullName: "Agent"})
}

func (e *env) expireOffer(callID uuid.UUID) {
	e.T.Helper()
	e.Exec(`UPDATE call_offers SET expires_at = now() - interval '1 second' WHERE call_id = $1 AND outcome = 'pending'`, callID)
}

func wantStatus(t *testing.T, e *env, call uuid.UUID, want string) {
	t.Helper()
	if got, _ := e.CallStatus(call); got != want {
		t.Fatalf("call status = %q, want %q", got, want)
	}
}

func TestOffersCallToLongestIdleAgent(t *testing.T) {
	e := setup(t)
	tenant, dept := e.Tenant(), uuid.Nil
	dept = e.Department(tenant, "Support")
	recent, longest := e.Agent(tenant, dept), e.Agent(tenant, dept)
	e.Available(tenant, recent, time.Minute)
	e.Available(tenant, longest, 10*time.Minute)
	c := e.Call(tenant, dept, time.Second)

	e.dispatch(tenant)

	wantStatus(t, e, c, "ringing")
	if got := e.OfferedAgent(c); got != longest {
		t.Fatalf("offered to %v, want the longest-idle agent %v", got, longest)
	}
}

func TestSkipsAgentsWhoCannotTakeTheCall(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept, other := e.Department(tenant, "Support"), e.Department(tenant, "Sales")

	away := e.Agent(tenant, dept)
	e.Presence(tenant, away, "away", time.Hour, 0)
	offline := e.Agent(tenant, dept)
	e.Presence(tenant, offline, "offline", time.Hour, 0)
	stale := e.Agent(tenant, dept)
	e.Presence(tenant, stale, "available", time.Hour, presence.StaleAfter+time.Second)
	wrongDept := e.Agent(tenant, other)
	e.Available(tenant, wrongDept, time.Hour)
	disabled := e.Agent(tenant, dept)
	e.Available(tenant, disabled, time.Hour)
	e.Exec(`UPDATE users SET status = 'disabled' WHERE id = $1`, disabled)

	c := e.Call(tenant, dept, time.Second)
	e.dispatch(tenant)

	wantStatus(t, e, c, "queued")
	if got := e.OfferedAgent(c); got != uuid.Nil {
		t.Fatalf("call was offered to %v although nobody could take it", got)
	}
}

func TestBusyAgentsAreNotOffered(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	e.Available(tenant, agent, time.Hour)

	first := e.Call(tenant, dept, 10*time.Second)
	second := e.Call(tenant, dept, time.Second)
	e.dispatch(tenant)

	if got := e.OfferedAgent(first); got != agent {
		t.Fatalf("oldest call offered to %v, want %v", got, agent)
	}
	wantStatus(t, e, second, "queued") // the agent already holds an offer
}

func TestOldestCallFirstAcrossDepartments(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	support, sales := e.Department(tenant, "Support"), e.Department(tenant, "Sales")
	agent := e.Agent(tenant, support, sales)
	e.Available(tenant, agent, time.Hour)

	newer := e.Call(tenant, support, time.Minute)
	older := e.Call(tenant, sales, 4*time.Minute)
	e.dispatch(tenant)

	if got := e.OfferedAgent(older); got != agent {
		t.Fatalf("the older call was not offered first")
	}
	wantStatus(t, e, newer, "queued")
}

func TestDeclinedCallGoesToAnotherAgentNeverBackToTheDecliner(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	a, b := e.Agent(tenant, dept), e.Agent(tenant, dept)
	e.Available(tenant, a, 10*time.Minute)
	e.Available(tenant, b, time.Minute)
	c := e.Call(tenant, dept, time.Second)

	e.dispatch(tenant)
	offer := e.PendingOfferID(c)
	if got := e.OfferedAgent(c); got != a {
		t.Fatalf("first offer went to %v, want %v", got, a)
	}
	if err := e.svc.Decline(e.as(tenant, a), offer); err != nil {
		t.Fatalf("decline: %v", err)
	}
	wantStatus(t, e, c, "queued")

	e.dispatch(tenant)
	if got := e.OfferedAgent(c); got != b {
		t.Fatalf("after the decline the call went to %v, want %v", got, b)
	}

	if err := e.svc.Decline(e.as(tenant, b), e.PendingOfferID(c)); err != nil {
		t.Fatalf("decline: %v", err)
	}
	e.dispatch(tenant)
	wantStatus(t, e, c, "queued") // both already had their chance
}

func TestTimedOutOfferMovesOnAndTwoInARowMakeAgentAway(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	a, b := e.Agent(tenant, dept), e.Agent(tenant, dept)
	e.Available(tenant, a, 10*time.Minute)
	e.Available(tenant, b, time.Minute)
	first, second := e.Call(tenant, dept, 20*time.Second), e.Call(tenant, dept, 10*time.Second)

	e.dispatch(tenant) // a gets the first call, b the second
	e.expireOffer(first)
	e.expireOffer(second)
	e.dispatch(tenant) // both time out; each call is now only eligible for the other agent

	if got := e.OfferOutcome(first, a); got != "timeout" {
		t.Fatalf("a's outcome on first call = %q, want timeout", got)
	}
	if got := e.OfferedAgent(first); got != b {
		t.Fatalf("first call went to %v after the timeout, want %v", got, b)
	}
	if got := e.OfferedAgent(second); got != a {
		t.Fatalf("second call went to %v after the timeout, want %v", got, a)
	}

	e.expireOffer(first)
	e.expireOffer(second)
	e.dispatch(tenant) // a second timeout in a row for both agents

	var status string
	e.Query(`SELECT status FROM agent_presence WHERE user_id = $1`, []any{a}, &status)
	if status != "away" {
		t.Fatalf("agent status after two timeouts = %q, want away", status)
	}
}

func TestOnlyOneConcurrentAcceptWins(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	e.Available(tenant, agent, time.Hour)
	c := e.Call(tenant, dept, time.Second)
	e.dispatch(tenant)
	offer := e.PendingOfferID(c)

	const attempts = 8
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Accept(e.as(tenant, agent), offer)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	wins := 0
	for err := range results {
		var appErr *apperr.Error
		switch {
		case err == nil:
			wins++
		case errors.As(err, &appErr) && appErr.Code == "offer_expired":
		default:
			t.Fatalf("unexpected accept error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d accepts succeeded, want exactly 1", wins)
	}
	wantStatus(t, e, c, "active")
}

func TestAcceptRejectsExpiredAndForeignOffers(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent, other := e.Agent(tenant, dept), e.Agent(tenant, dept)
	e.Available(tenant, agent, time.Hour)
	c := e.Call(tenant, dept, time.Second)
	e.dispatch(tenant)
	offer := e.PendingOfferID(c)

	if _, err := e.svc.Accept(e.as(tenant, other), offer); err == nil {
		t.Fatal("another agent accepted an offer addressed to someone else")
	}
	e.expireOffer(c)
	if _, err := e.svc.Accept(e.as(tenant, agent), offer); err == nil {
		t.Fatal("accepted an expired offer")
	}
	wantStatus(t, e, c, "ringing") // untouched until the sweeper runs
}

func TestMaxWaitMarksCallMissedAndClosesRoom(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	e.Available(tenant, agent, time.Hour)

	ringing := e.Call(tenant, dept, 10*time.Second)
	e.dispatch(tenant)
	waiting := e.Call(tenant, dept, 10*time.Minute) // longer than the default 5-minute maximum
	e.Exec(`UPDATE calls SET queued_at = now() - interval '10 minutes' WHERE id = $1`, ringing)

	e.dispatch(tenant)

	for _, id := range []uuid.UUID{ringing, waiting} {
		status, reason := e.CallStatus(id)
		if status != "missed" || reason == nil || *reason != call.EndMaxWait {
			t.Fatalf("call %v: status=%q reason=%v, want missed/max_wait_exceeded", id, status, reason)
		}
	}
	if got := e.OfferOutcome(ringing, agent); got != "cancelled" {
		t.Fatalf("offer outcome for the missed call = %q, want cancelled", got)
	}
	if len(e.media.deleted) != 2 {
		t.Fatalf("closed %d rooms, want 2", len(e.media.deleted))
	}
}

func TestStaleAgentLosesOfferWithoutPenaltyAndGoesOffline(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	gone, spare := e.Agent(tenant, dept), e.Agent(tenant, dept)
	e.Available(tenant, gone, time.Hour)
	c := e.Call(tenant, dept, time.Second)
	e.dispatch(tenant)
	if e.OfferedAgent(c) != gone {
		t.Fatal("setup: offer should be held by the first agent")
	}

	e.Presence(tenant, gone, "available", time.Hour, presence.StaleAfter+time.Second) // heartbeat stops
	e.Available(tenant, spare, time.Minute)
	e.dispatch(tenant)

	if got := e.OfferOutcome(c, gone); got != "cancelled" {
		t.Fatalf("stale agent's offer outcome = %q, want cancelled", got)
	}
	if got := e.OfferedAgent(c); got != spare {
		t.Fatalf("call went to %v, want the other agent %v", got, spare)
	}
	var status string
	var timeouts int
	e.Query(`SELECT status, consecutive_timeouts FROM agent_presence WHERE user_id = $1`, []any{gone}, &status, &timeouts)
	if status != "offline" || timeouts != 0 {
		t.Fatalf("stale agent: status=%q timeouts=%d, want offline/0", status, timeouts)
	}
}

func TestCallEndedCancelsOfferAndFreesAgent(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	e.Available(tenant, agent, time.Hour)
	c := e.Call(tenant, dept, time.Second)
	e.dispatch(tenant)

	ended := call.EndedCall{ID: c, TenantID: tenant, DepartmentID: dept, Reason: call.EndCallerHungUp}
	if err := e.DB.InPlatform(e.Ctx, func(ctx context.Context) error { return e.router.CallEnded(ctx, ended) }); err != nil {
		t.Fatal(err)
	}
	if got := e.OfferOutcome(c, agent); got != "cancelled" {
		t.Fatalf("offer outcome = %q, want cancelled", got)
	}

	// An agent who leaves mid-call goes offline; one whose call simply ends stays available.
	other := e.Agent(tenant, dept)
	e.Available(tenant, other, time.Hour)
	ended = call.EndedCall{ID: uuid.New(), TenantID: tenant, DepartmentID: dept, AgentID: &other, Reason: call.EndAgentLeft}
	if err := e.DB.InPlatform(e.Ctx, func(ctx context.Context) error { return e.router.CallEnded(ctx, ended) }); err != nil {
		t.Fatal(err)
	}
	var status string
	e.Query(`SELECT status FROM agent_presence WHERE user_id = $1`, []any{other}, &status)
	if status != "offline" {
		t.Fatalf("agent who left mid-call: status = %q, want offline", status)
	}
}

func TestSessionReflectsOfferAndCall(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	e.Available(tenant, agent, time.Hour)
	ctx := e.as(tenant, agent)

	s, err := e.svc.Session(ctx)
	if err != nil || s.State != presence.StateIdle || s.Offer != nil || s.Call != nil {
		t.Fatalf("idle session = %+v, err %v", s, err)
	}

	c := e.Call(tenant, dept, time.Second)
	e.dispatch(tenant)
	if s, err = e.svc.Session(ctx); err != nil || s.State != presence.StateRinging || s.Offer == nil {
		t.Fatalf("ringing session = %+v, err %v", s, err)
	}

	res, err := e.svc.Accept(ctx, s.Offer.ID)
	if err != nil || res.MediaToken == "" || res.Call.ID != c {
		t.Fatalf("accept = %+v, err %v", res, err)
	}
	if s, err = e.svc.Session(ctx); err != nil || s.State != presence.StateOnCall || s.Call == nil {
		t.Fatalf("on-call session = %+v, err %v", s, err)
	}
	if len(e.rec.started) != 1 || e.rec.started[0] != c {
		t.Fatalf("recording started for %v, want exactly the accepted call", e.rec.started)
	}
	if _, err := e.svc.MediaToken(ctx, c); err != nil {
		t.Fatalf("media token for own call: %v", err)
	}
	if _, err := e.svc.MediaToken(e.as(tenant, e.Agent(tenant, dept)), c); err == nil {
		t.Fatal("another agent obtained a token for a call that is not theirs")
	}
}

func TestSessionRequiresDepartmentMembership(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	loner := e.User(tenant, "agent")
	if _, err := e.svc.Session(e.as(tenant, loner)); err == nil {
		t.Fatal("a user in no department got an agent session")
	}
}

func TestDatabaseAllowsOneActiveCallPerAgent(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	first, second := e.Call(tenant, dept, time.Second), e.Call(tenant, dept, time.Second)
	e.Exec(`UPDATE calls SET status = 'active', assigned_agent_id = $2, answered_at = now() WHERE id = $1`, first, agent)

	err := e.DB.InPlatform(e.Ctx, func(ctx context.Context) error {
		return execErr(ctx, `UPDATE calls SET status = 'active', assigned_agent_id = $2, answered_at = now() WHERE id = $1`, second, agent)
	})
	if err == nil {
		t.Fatal("database let one agent hold two active calls")
	}
}

func TestRoutingTablesAreTenantIsolated(t *testing.T) {
	e := setup(t)
	a, b := e.Tenant(), e.Tenant()
	deptB := e.Department(b, "Support")
	agentB := e.Agent(b, deptB)
	e.Available(b, agentB, time.Hour)
	callB := e.Call(b, deptB, time.Second)
	e.dispatch(b)

	err := e.DB.InTenant(e.Ctx, a, func(ctx context.Context) error {
		for _, table := range []string{"agent_presence", "call_offers"} {
			var n int
			if err := scanOne(ctx, `SELECT count(*) FROM `+table, &n); err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("tenant A sees %d rows of tenant B in %s", n, table)
			}
		}
		return execErr(ctx, `UPDATE call_offers SET outcome = 'cancelled', responded_at = now() WHERE call_id = $1`, callB)
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.OfferedAgent(callB) != agentB {
		t.Fatal("tenant A cancelled tenant B's offer")
	}
}
