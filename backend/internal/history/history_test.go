package history_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/chat"
	"callcenter/internal/history"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
)

type env struct {
	*dbtest.Fixture
	svc *history.Service
}

func setup(t *testing.T) *env {
	f := dbtest.New(t)
	return &env{Fixture: f, svc: history.NewService(f.DB, history.NewRepository(), chat.NewRepository())}
}

func (e *env) as(tenant, user uuid.UUID, role reqctx.Role) context.Context {
	return reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: user, TenantID: tenant, Role: role})
}

func ids(items []history.Item) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, it := range items {
		out[it.ID] = true
	}
	return out
}

var page = httpx.Page{Number: 1, PerPage: 100}

func TestEachRoleSeesOnlyItsShare(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	support, sales := e.Department(tenant, "Support"), e.Department(tenant, "Sales")
	ann, bob := e.Agent(tenant, support), e.Agent(tenant, sales)
	admin := e.User(tenant, "tenant_admin")
	supervisor := e.Supervisor(tenant, support)

	annCall := e.EndedCall(tenant, support, ann, 5)
	bobCall := e.EndedCall(tenant, sales, bob, 6)
	missedSupport := e.MissedCall(tenant, support)

	list := func(user uuid.UUID, role reqctx.Role) map[uuid.UUID]bool {
		items, _, err := e.svc.List(e.as(tenant, user, role), history.Filter{}, page)
		if err != nil {
			t.Fatal(err)
		}
		return ids(items)
	}
	if got := list(admin, reqctx.RoleTenantAdmin); len(got) != 3 {
		t.Fatalf("admin sees %d calls, want all 3", len(got))
	}
	if got := list(supervisor, reqctx.RoleSupervisor); !got[annCall] || !got[missedSupport] || got[bobCall] || len(got) != 2 {
		t.Fatalf("supervisor of Support sees %v, want exactly its department's calls", got)
	}
	if got := list(ann, reqctx.RoleAgent); !got[annCall] || len(got) != 1 {
		t.Fatalf("agent sees %v, want only the call they handled", got)
	}

	// Out-of-scope calls look like they do not exist.
	if _, err := e.svc.Get(e.as(tenant, ann, reqctx.RoleAgent), bobCall); err == nil {
		t.Fatal("an agent opened another agent's call")
	}
	if _, err := e.svc.Get(e.as(tenant, supervisor, reqctx.RoleSupervisor), bobCall); err == nil {
		t.Fatal("a supervisor opened another department's call")
	}
	otherTenant := e.Tenant()
	intruder := e.User(otherTenant, "tenant_admin")
	if _, err := e.svc.Get(e.as(otherTenant, intruder, reqctx.RoleTenantAdmin), annCall); err == nil {
		t.Fatal("another tenant's admin opened the call")
	}
	if items, total, _ := e.svc.List(e.as(otherTenant, intruder, reqctx.RoleTenantAdmin), history.Filter{}, page); total != 0 || len(items) != 0 {
		t.Fatalf("another tenant sees %d calls", total)
	}
}

func TestFilters(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	support, sales := e.Department(tenant, "Support"), e.Department(tenant, "Sales")
	ann, bob := e.Agent(tenant, support), e.Agent(tenant, sales)
	admin := e.User(tenant, "tenant_admin")
	asAdmin := e.as(tenant, admin, reqctx.RoleTenantAdmin)

	recent := e.EndedCall(tenant, support, ann, 5)
	old := e.EndedCall(tenant, sales, bob, 3*24*60)
	missed := e.MissedCall(tenant, support)
	e.Exec(`UPDATE calls SET caller_name = 'Ada Lovelace', caller_phone = '+2348011112222' WHERE id = $1`, recent)

	count := func(f history.Filter) int {
		_, total, err := e.svc.List(asAdmin, f, page)
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	tests := map[string]struct {
		filter history.Filter
		want   int
	}{
		"status":       {history.Filter{Status: "missed"}, 1},
		"department":   {history.Filter{DepartmentID: sales}, 1},
		"agent":        {history.Filter{AgentID: ann}, 1},
		"from":         {history.Filter{From: &yesterday}, 2},
		"to":           {history.Filter{To: &yesterday}, 1},
		"name search":  {history.Filter{Query: "lovelace"}, 1},
		"phone search": {history.Filter{Query: "801111"}, 1},
		"no match":     {history.Filter{Query: "zzz"}, 0},
		"wildcards":    {history.Filter{Query: "%"}, 0}, // LIKE wildcards are escaped
		"combination":  {history.Filter{DepartmentID: support, Status: "completed"}, 1},
	}
	for name, tc := range tests {
		if got := count(tc.filter); got != tc.want {
			t.Errorf("%s: %d calls, want %d", name, got, tc.want)
		}
	}
	_ = old
	_ = missed
}

func TestDetailBringsTheWholeStoryAndRespectsSoftDelete(t *testing.T) {
	e := setup(t)
	tenant := e.Tenant()
	dept, other := e.Department(tenant, "Support"), e.Department(tenant, "Sales")
	agent := e.Agent(tenant, dept)
	admin := e.User(tenant, "tenant_admin")
	supervisor := e.Supervisor(tenant, dept)
	otherSupervisor := e.Supervisor(tenant, other)

	callID := e.EndedCall(tenant, dept, agent, 4)
	e.Exec(`UPDATE calls SET caller_name = 'Ada', caller_email = 'ada@example.com' WHERE id = $1`, callID)
	e.Exec(`INSERT INTO call_offers (tenant_id, call_id, agent_id, expires_at, responded_at, outcome) VALUES ($1, $2, $3, now(), now(), 'accepted')`, tenant, callID, agent)
	e.Exec(`INSERT INTO call_messages (tenant_id, call_id, sender_type, sender_name, body) VALUES ($1, $2, 'caller', 'Ada', 'Hello')`, tenant, callID)
	e.Exec(`INSERT INTO recordings (tenant_id, call_id, status, storage_key, size_bytes, duration_seconds, egress_id) VALUES ($1, $2, 'ready', 'k', 100, 300, 'EG_h')`, tenant, callID)
	e.Exec(`INSERT INTO callback_requests (tenant_id, department_id, call_id, name, phone) VALUES ($1, $2, $3, 'Ada', '+2348011112222')`, tenant, dept, callID)

	d, err := e.svc.Get(e.as(tenant, admin, reqctx.RoleTenantAdmin), callID)
	if err != nil {
		t.Fatal(err)
	}
	if d.CallerEmail == nil || *d.CallerEmail != "ada@example.com" || d.ConsentText != "consent" || d.AgentName == nil {
		t.Fatalf("detail = %+v", d)
	}
	if len(d.Offers) != 1 || d.Offers[0].Outcome != "accepted" || len(d.Messages) != 1 || d.Messages[0].Body != "Hello" {
		t.Fatalf("offers=%+v messages=%+v", d.Offers, d.Messages)
	}
	if d.Recording == nil || d.Recording.Status != "ready" || d.Callback == nil || d.DurationSeconds == nil || *d.DurationSeconds != 300 {
		t.Fatalf("recording=%+v callback=%+v duration=%v", d.Recording, d.Callback, d.DurationSeconds)
	}
	if !d.Permissions.PlayRecording || !d.Permissions.DeleteRecording {
		t.Fatalf("admin permissions = %+v", d.Permissions)
	}

	sup, err := e.svc.Get(e.as(tenant, supervisor, reqctx.RoleSupervisor), callID)
	if err != nil || !sup.Permissions.PlayRecording || sup.Permissions.DeleteRecording {
		t.Fatalf("supervisor permissions = %+v, %v", sup.Permissions, err)
	}
	if _, err := e.svc.Get(e.as(tenant, otherSupervisor, reqctx.RoleSupervisor), callID); err == nil {
		t.Fatal("a supervisor of another department opened the call")
	}
	ag, err := e.svc.Get(e.as(tenant, agent, reqctx.RoleAgent), callID)
	if err != nil || ag.Permissions.PlayRecording || ag.Permissions.DeleteRecording {
		t.Fatalf("agent permissions = %+v, %v", ag.Permissions, err)
	}

	// A soft-deleted recording disappears from the call, which itself stays.
	e.Exec(`UPDATE recordings SET deleted_at = now(), deleted_by = $2 WHERE call_id = $1`, callID, admin)
	d, err = e.svc.Get(e.as(tenant, admin, reqctx.RoleTenantAdmin), callID)
	if err != nil || d.Recording != nil || d.RecordingStatus != nil {
		t.Fatalf("after soft delete: recording=%+v status=%v err=%v", d.Recording, d.RecordingStatus, err)
	}
}
