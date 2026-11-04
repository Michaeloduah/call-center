package board_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/board"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/realtime"
)

func principal(tenant, user uuid.UUID, role reqctx.Role) context.Context {
	return reqctx.WithPrincipal(context.Background(), reqctx.Principal{UserID: user, TenantID: tenant, Role: role})
}

func departmentNames(b *board.Board) map[string]board.DepartmentBoard {
	out := map[string]board.DepartmentBoard{}
	for _, d := range b.Departments {
		out[d.Name] = d
	}
	return out
}

func TestSupervisorSeesOnlyAssignedDepartments(t *testing.T) {
	f := dbtest.New(t)
	svc := board.NewService(f.DB)
	tenant := f.Tenant()
	support, sales := f.Department(tenant, "Support"), f.Department(tenant, "Sales")
	supervisor := f.Supervisor(tenant, support)
	admin := f.User(tenant, "tenant_admin")

	agent := f.Agent(tenant, support)
	f.Available(tenant, agent, time.Hour)
	f.Call(tenant, support, 30*time.Second)
	f.Call(tenant, sales, 45*time.Second)

	b, err := svc.Get(principal(tenant, supervisor, reqctx.RoleSupervisor))
	if err != nil {
		t.Fatal(err)
	}
	seen := departmentNames(b)
	if len(seen) != 1 || seen["Support"].ID != support {
		t.Fatalf("supervisor sees %v, want only Support", seen)
	}
	d := seen["Support"]
	if d.Queued != 1 || d.OldestQueuedSince == nil {
		t.Fatalf("Support queue = %d (oldest %v), want 1 waiting call", d.Queued, d.OldestQueuedSince)
	}
	if len(d.Agents) != 1 || d.Agents[0].ID != agent || d.Agents[0].State != "idle" {
		t.Fatalf("Support agents = %+v, want the idle agent", d.Agents)
	}

	topics, err := svc.Topics(principal(tenant, supervisor, reqctx.RoleSupervisor))
	if err != nil || len(topics) != 1 || topics[0] != realtime.DepartmentTopic(support) {
		t.Fatalf("supervisor topics = %v, %v; want only the Support topic", topics, err)
	}

	all, err := svc.Get(principal(tenant, admin, reqctx.RoleTenantAdmin))
	if err != nil {
		t.Fatal(err)
	}
	if got := departmentNames(all); len(got) != 2 || got["Sales"].Queued != 1 {
		t.Fatalf("tenant admin sees %v, want both departments", got)
	}
}

func TestBoardNeverCrossesTenants(t *testing.T) {
	f := dbtest.New(t)
	svc := board.NewService(f.DB)
	a, b := f.Tenant(), f.Tenant()
	f.Department(a, "Support")
	deptB := f.Department(b, "Support")
	f.Call(b, deptB, time.Second)
	adminA := f.User(a, "tenant_admin")

	got, err := svc.Get(principal(a, adminA, reqctx.RoleTenantAdmin))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got.Departments {
		if d.ID == deptB || d.Queued != 0 {
			t.Fatalf("tenant A's board shows tenant B's data: %+v", d)
		}
	}
}
