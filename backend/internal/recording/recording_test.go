package recording_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/media"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/recording"
	"callcenter/internal/storage"
)

type fakeStore struct{}

func (fakeStore) EnsureBucket(context.Context) error { return nil }
func (fakeStore) PresignGet(_ context.Context, key string, _ time.Duration, filename string) (string, error) {
	return "https://store.test/" + key + "?download=" + filename, nil
}
func (fakeStore) Target() storage.Target {
	return storage.Target{Endpoint: "http://minio:9000", Bucket: "recordings"}
}

type fakeMedia struct {
	mu      sync.Mutex
	started []string
	err     error
}

func (m *fakeMedia) StartRecording(_ context.Context, room string, _ media.RecordingTarget) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	m.started = append(m.started, room)
	return "EG_" + uuid.NewString()[:8], nil
}
func (m *fakeMedia) CreateRoom(context.Context, string) error               { return nil }
func (m *fakeMedia) DeleteRoom(context.Context, string) error               { return nil }
func (m *fakeMedia) SendData(context.Context, string, string, []byte) error { return nil }
func (m *fakeMedia) JoinToken(string, media.Participant) (string, error)    { return "t", nil }
func (m *fakeMedia) PublicURL() string                                      { return "ws://x" }
func (m *fakeMedia) ParseWebhook(*http.Request) (media.Event, error)        { return media.Event{}, nil }

type env struct {
	*dbtest.Fixture
	svc   *recording.Service
	repo  recording.Repository
	media *fakeMedia
}

func setup(t *testing.T) *env {
	f := dbtest.New(t)
	mp := &fakeMedia{}
	repo := recording.NewRepository()
	svc := recording.NewService(f.DB, repo, mp, fakeStore{}, audit.NewRecorder(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &env{Fixture: f, svc: svc, repo: repo, media: mp}
}

func (e *env) as(tenant, user uuid.UUID, role reqctx.Role) context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: user, TenantID: tenant, Role: role, Email: "u@example.test"})
}

func (e *env) platform(admin uuid.UUID) context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: admin, Role: reqctx.RolePlatformAdmin, Email: "p@example.test"})
}

// readyRecording inserts a finished recording for a call.
func (e *env) readyRecording(tenant, call uuid.UUID) uuid.UUID {
	e.T.Helper()
	var id uuid.UUID
	e.Query(`
		INSERT INTO recordings (tenant_id, call_id, status, storage_key, content_type, size_bytes, duration_seconds, egress_id, ended_at)
		VALUES ($1, $2, 'ready', $3, 'audio/ogg', 480000, 60, $4, now()) RETURNING id`,
		[]any{tenant, call, "tenants/" + tenant.String() + "/recordings/" + call.String() + ".ogg", "EG_" + uuid.NewString()[:8]}, &id)
	return id
}

func (e *env) auditCount(entity uuid.UUID, action string) int {
	e.T.Helper()
	var n int
	e.Query(`SELECT count(*)::int FROM audit_logs WHERE entity_id = $1 AND action = $2`, []any{entity, action}, &n)
	return n
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}

func TestStartRecordsCallOnceAndAttachesRecorder(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	call := e.ActiveCall(tenant, dept, agent)

	e.svc.Start(tenant, call, "call-"+call.String())
	e.svc.Start(tenant, call, "call-"+call.String()) // accept retried: must not record twice

	var status string
	var egress, key *string
	deadline := time.Now().Add(5 * time.Second)
	for {
		e.Query(`SELECT coalesce((SELECT status FROM recordings WHERE call_id = $1), ''),
		                (SELECT egress_id FROM recordings WHERE call_id = $1), (SELECT storage_key FROM recordings WHERE call_id = $1)`,
			[]any{call}, &status, &egress, &key)
		if status == "recording" || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != "recording" || egress == nil || key == nil || !strings.HasSuffix(*key, call.String()+".ogg") {
		t.Fatalf("recording = %q egress=%v key=%v", status, egress, key)
	}
	e.media.mu.Lock()
	defer e.media.mu.Unlock()
	if len(e.media.started) != 1 {
		t.Fatalf("recorder started %d times, want once", len(e.media.started))
	}
}

func TestRecorderFailureMarksRecordingFailedNotTheCall(t *testing.T) {
	e := setup(t)
	e.media.err = errors.New("recorder down")
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	call := e.ActiveCall(tenant, dept, e.Agent(tenant, dept))

	e.svc.Start(tenant, call, "call-"+call.String())

	var status string
	var reason *string
	deadline := time.Now().Add(5 * time.Second)
	for {
		e.Query(`SELECT coalesce((SELECT status FROM recordings WHERE call_id = $1), ''), (SELECT error FROM recordings WHERE call_id = $1)`,
			[]any{call}, &status, &reason)
		if status == "failed" || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != "failed" || reason == nil {
		t.Fatalf("recording status = %q (%v), want failed with a reason", status, reason)
	}
	if got, _ := e.CallStatus(call); got != "active" {
		t.Fatalf("call status = %q: a recording failure must not touch the call", got)
	}
}

func TestEgressEndedFinishesRecordingIdempotently(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	call := e.EndedCall(tenant, dept, e.Agent(tenant, dept), 2)
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status, egress_id, storage_key) VALUES ($1, $2, 'recording', 'EG_done', 'k')`, tenant, call)

	ok := media.Event{Kind: media.EventEgressEnded, EgressID: "EG_done", Recording: &media.RecordingResult{Succeeded: true, SizeBytes: 123456, Duration: 61*time.Second + 400*time.Millisecond}}
	for range 2 { // redelivery
		if err := e.svc.HandleMediaEvent(e.Ctx, ok); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	var size, dur int64
	e.Query(`SELECT status, size_bytes, duration_seconds FROM recordings WHERE call_id = $1`, []any{call}, &status, &size, &dur)
	if status != "ready" || size != 123456 || dur != 61 {
		t.Fatalf("recording = %s %d bytes %ds, want ready 123456 61", status, size, dur)
	}

	// A late failure report must not undo a finished recording, and unknown egress ids are ignored.
	late := media.Event{Kind: media.EventEgressEnded, EgressID: "EG_done", Recording: &media.RecordingResult{Error: "boom"}}
	if err := e.svc.HandleMediaEvent(e.Ctx, late); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.HandleMediaEvent(e.Ctx, media.Event{Kind: media.EventEgressEnded, EgressID: "EG_unknown", Recording: &media.RecordingResult{}}); err != nil {
		t.Fatalf("unknown egress id: %v", err)
	}
	e.Query(`SELECT status FROM recordings WHERE call_id = $1`, []any{call}, &status)
	if status != "ready" {
		t.Fatalf("late failure changed a ready recording to %q", status)
	}
}

func TestFailedEgressRecordsTheReason(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	call := e.EndedCall(tenant, dept, e.Agent(tenant, dept), 2)
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status, egress_id, storage_key) VALUES ($1, $2, 'recording', 'EG_bad', 'k')`, tenant, call)
	if err := e.svc.HandleMediaEvent(e.Ctx, media.Event{Kind: media.EventEgressEnded, EgressID: "EG_bad", Recording: &media.RecordingResult{Error: "Start signal not received"}}); err != nil {
		t.Fatal(err)
	}
	var status string
	var reason *string
	e.Query(`SELECT status, error FROM recordings WHERE call_id = $1`, []any{call}, &status, &reason)
	if status != "failed" || reason == nil || *reason != "Start signal not received" {
		t.Fatalf("recording = %s %v", status, reason)
	}
}

func TestAccessRulesAndAudit(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept, other := e.Department(tenant, "Support"), e.Department(tenant, "Sales")
	agent := e.Agent(tenant, dept)
	admin := e.User(tenant, "tenant_admin")
	supervisor := e.Supervisor(tenant, dept)
	otherSupervisor := e.Supervisor(tenant, other)
	call := e.EndedCall(tenant, dept, agent, 3)
	rec := e.readyRecording(tenant, call)

	grant, err := e.svc.Access(e.as(tenant, admin, reqctx.RoleTenantAdmin), call, false)
	if err != nil || !strings.Contains(grant.URL, call.String()+".ogg") || grant.ExpiresAt.Before(time.Now()) {
		t.Fatalf("admin access = %+v, %v", grant, err)
	}
	if got := e.auditCount(rec, audit.ActionRecordingPlayed); got != 1 {
		t.Fatalf("played audit entries = %d, want 1", got)
	}

	grant, err = e.svc.Access(e.as(tenant, supervisor, reqctx.RoleSupervisor), call, true)
	if err != nil || !strings.Contains(grant.URL, "download=call-"+call.String()+".ogg") {
		t.Fatalf("supervisor download = %+v, %v", grant, err)
	}
	if got := e.auditCount(rec, audit.ActionRecordingDownloaded); got != 1 {
		t.Fatalf("downloaded audit entries = %d, want 1", got)
	}

	if _, err := e.svc.Access(e.as(tenant, otherSupervisor, reqctx.RoleSupervisor), call, false); err == nil {
		t.Fatal("a supervisor of another department played the recording")
	} else {
		wantCode(t, err, "not_found")
	}
	if _, err := e.svc.Access(e.as(tenant, agent, reqctx.RoleAgent), call, false); err == nil {
		t.Fatal("an agent played a recording")
	}

	// A recording that is not finished cannot be played.
	pending := e.EndedCall(tenant, dept, agent, 4)
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status) VALUES ($1, $2, 'starting')`, tenant, pending)
	_, err = e.svc.Access(e.as(tenant, admin, reqctx.RoleTenantAdmin), pending, false)
	wantCode(t, err, "recording_unavailable")

	// Another tenant's admin cannot reach it either.
	otherTenant := e.Tenant()
	intruder := e.User(otherTenant, "tenant_admin")
	if _, err := e.svc.Access(e.as(otherTenant, intruder, reqctx.RoleTenantAdmin), call, false); err == nil {
		t.Fatal("another tenant's admin played the recording")
	}
}

func TestSoftDeleteHidesFromTenantAndKeepsFileCountedUntilPlatformRestores(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)
	admin := e.User(tenant, "tenant_admin")
	supervisor := e.Supervisor(tenant, dept)
	platform := e.PlatformAdmin()
	call := e.EndedCall(tenant, dept, agent, 3)
	rec := e.readyRecording(tenant, call)
	asAdmin := e.as(tenant, admin, reqctx.RoleTenantAdmin)

	if err := e.svc.Delete(e.as(tenant, supervisor, reqctx.RoleSupervisor), call); err == nil {
		t.Fatal("a supervisor deleted a recording")
	}
	if err := e.svc.Delete(asAdmin, call); err != nil {
		t.Fatal(err)
	}
	if got := e.auditCount(rec, audit.ActionRecordingDeleted); got != 1 {
		t.Fatalf("deleted audit entries = %d, want 1", got)
	}
	_, err := e.svc.Access(asAdmin, call, false)
	wantCode(t, err, "not_found")
	wantCode(t, e.svc.Delete(asAdmin, call), "not_found") // deleting twice

	// The file still counts toward the tenant's storage (D17).
	var stored int64
	e.Query(`SELECT coalesce(sum(size_bytes), 0) FROM recordings WHERE tenant_id = $1`, []any{tenant}, &stored)
	if stored != 480000 {
		t.Fatalf("storage used = %d, want the deleted recording to still count (480000)", stored)
	}

	items, total, err := e.svc.ListDeleted(e.platform(platform), httpx.Page{Number: 1, PerPage: 50})
	if err != nil || total < 1 {
		t.Fatalf("platform list = %d items, %v", total, err)
	}
	found := false
	for _, it := range items {
		found = found || it.ID == rec
	}
	if !found {
		t.Fatal("deleted recording missing from the platform list")
	}
	if _, err := e.svc.PlatformAccess(e.platform(platform), rec, false); err != nil {
		t.Fatalf("platform admin could not play a deleted recording: %v", err)
	}

	if err := e.svc.Restore(e.platform(platform), rec); err != nil {
		t.Fatal(err)
	}
	if got := e.auditCount(rec, audit.ActionRecordingRestored); got != 1 {
		t.Fatalf("restored audit entries = %d, want 1", got)
	}
	if _, err := e.svc.Access(asAdmin, call, false); err != nil {
		t.Fatalf("restored recording is still hidden: %v", err)
	}
	wantCode(t, e.svc.Restore(e.platform(platform), rec), "not_found") // nothing left to restore
}

func TestSweeperFailsRecordingsThatNeverReportBack(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept := e.Department(tenant, "Support")
	agent := e.Agent(tenant, dept)

	stuckStarting := e.EndedCall(tenant, dept, agent, 1)
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status, created_at) VALUES ($1, $2, 'starting', now() - interval '5 minutes')`, tenant, stuckStarting)
	lost := e.EndedCall(tenant, dept, agent, 30)
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status, egress_id, storage_key) VALUES ($1, $2, 'recording', 'EG_lost', 'k')`, tenant, lost)
	fresh := e.ActiveCall(tenant, dept, e.Agent(tenant, dept))
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status, egress_id, storage_key) VALUES ($1, $2, 'recording', 'EG_live', 'k')`, tenant, fresh)
	justEnded := e.EndedCall(tenant, dept, agent, 1)
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status, egress_id, storage_key) VALUES ($1, $2, 'recording', 'EG_soon', 'k')`, tenant, justEnded)

	err := e.DB.InPlatform(e.Ctx, func(ctx context.Context) error {
		n, err := e.repo.FailStale(ctx)
		if n < 2 {
			t.Errorf("sweeper failed %d recordings, want at least 2", n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	status := func(call uuid.UUID) (s string) {
		e.Query(`SELECT status FROM recordings WHERE call_id = $1`, []any{call}, &s)
		return s
	}
	if status(stuckStarting) != "failed" || status(lost) != "failed" {
		t.Fatalf("stale recordings: starting=%s lost=%s, want failed", status(stuckStarting), status(lost))
	}
	if status(fresh) != "recording" || status(justEnded) != "recording" {
		t.Fatalf("live recordings were failed: live=%s justEnded=%s", status(fresh), status(justEnded))
	}
}

func TestNewTablesAreTenantIsolated(t *testing.T) {
	e := setup(t)
	a, b := e.Tenant(), e.Tenant()
	deptB := e.Department(b, "Support")
	agentB := e.Agent(b, deptB)
	callB := e.EndedCall(b, deptB, agentB, 2)
	e.readyRecording(b, callB)
	e.Exec(`INSERT INTO call_messages (tenant_id, call_id, sender_type, sender_name, body) VALUES ($1, $2, 'caller', 'C', 'hi')`, b, callB)
	e.Exec(`INSERT INTO callback_requests (tenant_id, department_id, name, phone) VALUES ($1, $2, 'N', '+2348011112222')`, b, deptB)

	err := e.DB.InTenant(e.Ctx, a, func(ctx context.Context) error {
		q, _ := database.Conn(ctx)
		for _, table := range []string{"recordings", "call_messages", "callback_requests"} {
			var n int
			if err := q.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("tenant A sees %d rows of tenant B in %s", n, table)
			}
		}
		if _, err := q.Exec(ctx, `UPDATE recordings SET deleted_at = now()`); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `INSERT INTO call_messages (tenant_id, call_id, sender_type, sender_name, body) VALUES ($1, $2, 'caller', 'C', 'x')`, b, callB)
		if err == nil {
			t.Error("tenant A wrote a message into tenant B")
		}
		return nil
	})
	_ = err // the rejected insert aborts the transaction; the assertions above already ran
	var deleted int
	e.Query(`SELECT count(*)::int FROM recordings WHERE tenant_id = $1 AND deleted_at IS NOT NULL`, []any{b}, &deleted)
	if deleted != 0 {
		t.Fatal("tenant A soft-deleted tenant B's recording")
	}
}
