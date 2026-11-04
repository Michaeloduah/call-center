package callback_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/callback"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/widget"
)

type fakeWidgets struct{ tenant, dept uuid.UUID }

func (w fakeWidgets) Resolve(_ context.Context, key string) (*widget.Resolved, error) {
	if key != "wk_ok" {
		return nil, nil
	}
	return &widget.Resolved{TenantID: w.tenant, DepartmentID: w.dept}, nil
}

type fakeAuth struct{ view call.CallerView }

func (a fakeAuth) Authenticate(_ context.Context, id uuid.UUID, token string) (*call.CallerView, error) {
	if token != "good" {
		return nil, apperr.Unauthenticated("invalid_caller_token", "expired")
	}
	v := a.view
	v.ID = id
	return &v, nil
}

type fakeNotifier struct {
	mu     sync.Mutex
	admins [][]callback.Admin
	sent   []callback.Notification
}

func (n *fakeNotifier) NewCallback(_ context.Context, _ string, admins []callback.Admin, cb callback.Notification) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.admins = append(n.admins, admins)
	n.sent = append(n.sent, cb)
}

type env struct {
	*dbtest.Fixture
	svc      *callback.Service
	notifier *fakeNotifier
	tenant   uuid.UUID
	dept     uuid.UUID
}

func setup(t *testing.T) *env {
	f := dbtest.New(t)
	tenant := f.Tenant()
	dept := f.Department(tenant, "Support")
	n := &fakeNotifier{}
	svc := callback.NewService(f.DB, callback.NewRepository(), fakeWidgets{tenant, dept}, fakeAuth{call.CallerView{Status: call.StatusMissed}}, n,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &env{Fixture: f, svc: svc, notifier: n, tenant: tenant, dept: dept}
}

func (e *env) as(user uuid.UUID, role reqctx.Role) context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: user, TenantID: e.tenant, Role: role})
}

func (e *env) newCallback() uuid.UUID {
	e.T.Helper()
	id, err := e.svc.CreateForWidget(e.Ctx, "wk_ok", callback.CreateInput{Name: "Ada", Phone: "+2348011112222", Reason: "Refund"})
	if err != nil {
		e.T.Fatalf("create callback: %v", err)
	}
	return id
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}

func TestCreateInputNeedsAWayToReachTheCaller(t *testing.T) {
	cases := map[string]struct {
		in      callback.CreateInput
		wantErr bool
	}{
		"phone only":      {callback.CreateInput{Name: "Ada", Phone: "+234 801 111 2222"}, false},
		"email only":      {callback.CreateInput{Name: "Ada", Email: "Ada@Example.com"}, false},
		"both":            {callback.CreateInput{Name: "Ada", Phone: "08011112222", Email: "a@example.com"}, false},
		"no contact":      {callback.CreateInput{Name: "Ada", Reason: "call me"}, true},
		"no name":         {callback.CreateInput{Phone: "+2348011112222"}, true},
		"bad phone":       {callback.CreateInput{Name: "Ada", Phone: "abc"}, true},
		"bad email":       {callback.CreateInput{Name: "Ada", Email: "nope"}, true},
		"reason too long": {callback.CreateInput{Name: "Ada", Phone: "+2348011112222", Reason: strings.Repeat("x", 501)}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := tc.in
			if err := in.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, want error: %v", err, tc.wantErr)
			}
		})
	}
	in := callback.CreateInput{Name: " Ada ", Email: "Ada@Example.com"}
	if err := in.Validate(); err != nil || in.Name != "Ada" || in.Email != "ada@example.com" {
		t.Fatalf("normalised input = %+v, %v", in, err)
	}
}

func TestCreateFromWidgetNotifiesAdmins(t *testing.T) {
	e := setup(t)
	e.User(e.tenant, "tenant_admin")
	id := e.newCallback()

	var status, name string
	e.Query(`SELECT status, name FROM callback_requests WHERE id = $1`, []any{id}, &status, &name)
	if status != "open" || name != "Ada" {
		t.Fatalf("callback = %s %s", status, name)
	}
	if len(e.notifier.sent) != 1 || len(e.notifier.admins[0]) != 1 || e.notifier.sent[0].DepartmentName != "Support" {
		t.Fatalf("notification = %+v admins=%+v", e.notifier.sent, e.notifier.admins)
	}
	if _, err := e.svc.CreateForWidget(e.Ctx, "wk_nope", callback.CreateInput{Name: "Ada", Phone: "+2348011112222"}); err == nil {
		t.Fatal("callback accepted for an unknown widget")
	}
}

func TestCreateForCallOnlyAfterMissedAndOnlyOnce(t *testing.T) {
	e := setup(t)
	missed := e.MissedCall(e.tenant, e.dept)
	in := callback.CreateInput{Name: "Ada", Phone: "+2348011112222"}

	if _, err := e.svc.CreateForCall(e.Ctx, missed, "bad", in); err == nil {
		t.Fatal("callback created with a wrong caller token")
	}
	id, err := e.svc.CreateForCall(e.Ctx, missed, "good", in)
	if err != nil {
		t.Fatal(err)
	}
	var linked uuid.UUID
	e.Query(`SELECT call_id FROM callback_requests WHERE id = $1`, []any{id}, &linked)
	if linked != missed {
		t.Fatalf("callback linked to %v, want %v", linked, missed)
	}
	_, err = e.svc.CreateForCall(e.Ctx, missed, "good", in)
	wantCode(t, err, "callback_exists")

	notMissed := callback.NewService(e.DB, callback.NewRepository(), fakeWidgets{}, fakeAuth{call.CallerView{Status: call.StatusActive}}, e.notifier,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err = notMissed.CreateForCall(e.Ctx, e.ActiveCall(e.tenant, e.dept, e.Agent(e.tenant, e.dept)), "good", in)
	wantCode(t, err, "call_not_missed")
}

func TestOnlyOneConcurrentClaimWins(t *testing.T) {
	e := setup(t)
	id := e.newCallback()
	agents := make([]uuid.UUID, 8)
	for i := range agents {
		agents[i] = e.Agent(e.tenant, e.dept)
	}
	var wg sync.WaitGroup
	results := make(chan error, len(agents))
	for _, a := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- e.svc.Claim(e.as(a, reqctx.RoleAgent), id)
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
		case errors.As(err, &appErr) && appErr.Code == "callback_taken":
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d agents claimed the same callback, want exactly 1", wins)
	}
}

func TestWorkflowPermissions(t *testing.T) {
	e := setup(t)
	id := e.newCallback()
	agent, colleague := e.Agent(e.tenant, e.dept), e.Agent(e.tenant, e.dept)
	admin := e.User(e.tenant, "tenant_admin")
	outsider := e.Agent(e.tenant, e.Department(e.tenant, "Sales"))
	supervisor := e.Supervisor(e.tenant, e.dept)

	// Agents of other departments and plain supervisors cannot see or touch it.
	wantCode(t, e.svc.Claim(e.as(outsider, reqctx.RoleAgent), id), "not_found")
	wantCode(t, e.svc.Claim(e.as(supervisor, reqctx.RoleSupervisor), id), "not_found")

	if err := e.svc.Claim(e.as(agent, reqctx.RoleAgent), id); err != nil {
		t.Fatal(err)
	}
	// Only the claimer (or an admin) may release or complete.
	wantCode(t, e.svc.Release(e.as(colleague, reqctx.RoleAgent), id), "callback_state")
	wantCode(t, e.svc.Complete(e.as(colleague, reqctx.RoleAgent), id, callback.CompleteInput{Notes: "x"}), "callback_state")
	wantCode(t, e.svc.Cancel(e.as(agent, reqctx.RoleAgent), id), "forbidden")

	if err := e.svc.Release(e.as(agent, reqctx.RoleAgent), id); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Claim(e.as(colleague, reqctx.RoleAgent), id); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Complete(e.as(colleague, reqctx.RoleAgent), id, callback.CompleteInput{Notes: "Refund issued"}); err != nil {
		t.Fatal(err)
	}
	cb, err := e.svc.Get(e.as(admin, reqctx.RoleTenantAdmin), id)
	if err != nil || cb.Status != callback.StatusCompleted || cb.ResolutionNotes != "Refund issued" || cb.ResolvedAt == nil {
		t.Fatalf("completed callback = %+v, %v", cb, err)
	}
	wantCode(t, e.svc.Claim(e.as(agent, reqctx.RoleAgent), id), "callback_taken") // finished requests cannot be reopened
}

func TestAdminAssignAndCancel(t *testing.T) {
	e := setup(t)
	id := e.newCallback()
	admin := e.User(e.tenant, "tenant_admin")
	agent := e.Agent(e.tenant, e.dept)
	nonMember := e.Agent(e.tenant, e.Department(e.tenant, "Sales"))
	asAdmin := e.as(admin, reqctx.RoleTenantAdmin)

	wantCode(t, e.svc.Assign(asAdmin, id, callback.AssignInput{UserID: nonMember}), "validation_failed")
	wantCode(t, e.svc.Assign(e.as(agent, reqctx.RoleAgent), id, callback.AssignInput{UserID: agent}), "forbidden")
	if err := e.svc.Assign(asAdmin, id, callback.AssignInput{UserID: agent}); err != nil {
		t.Fatal(err)
	}
	cb, _ := e.svc.Get(asAdmin, id)
	if cb.Status != callback.StatusClaimed || cb.ClaimedBy == nil || *cb.ClaimedBy != agent {
		t.Fatalf("assigned callback = %+v", cb)
	}
	if err := e.svc.Cancel(asAdmin, id); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.Cancel(asAdmin, id), "callback_state")
}

func TestListScopeAndOrder(t *testing.T) {
	e := setup(t)
	sales := e.Department(e.tenant, "Sales")
	supportAgent := e.Agent(e.tenant, e.dept)
	admin := e.User(e.tenant, "tenant_admin")

	done := e.newCallback()
	open := e.newCallback()
	if err := e.svc.Claim(e.as(supportAgent, reqctx.RoleAgent), done); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Complete(e.as(supportAgent, reqctx.RoleAgent), done, callback.CompleteInput{}); err != nil {
		t.Fatal(err)
	}
	e.Exec(`INSERT INTO callback_requests (tenant_id, department_id, name, phone) VALUES ($1, $2, 'Sales lead', '+2348022223333')`, e.tenant, sales)

	page := httpx.Page{Number: 1, PerPage: 50}
	items, total, err := e.svc.List(e.as(supportAgent, reqctx.RoleAgent), callback.Filter{}, page)
	if err != nil || total != 2 {
		t.Fatalf("agent sees %d callbacks (%v), want only their department's 2", total, err)
	}
	if items[0].ID != open || items[1].ID != done {
		t.Fatalf("order = %v, %v; open work must come first", items[0].Status, items[1].Status)
	}
	if _, total, _ := e.svc.List(e.as(admin, reqctx.RoleTenantAdmin), callback.Filter{}, page); total != 3 {
		t.Fatalf("admin sees %d callbacks, want 3", total)
	}
	if _, total, _ := e.svc.List(e.as(admin, reqctx.RoleTenantAdmin), callback.Filter{Status: callback.StatusOpen, DepartmentID: sales}, page); total != 1 {
		t.Fatalf("filtered list = %d, want 1", total)
	}
	if _, _, err := e.svc.List(e.as(admin, reqctx.RoleTenantAdmin), callback.Filter{Status: "weird"}, page); err == nil {
		t.Fatal("unknown status accepted")
	}
}
