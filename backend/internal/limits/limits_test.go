package limits_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/department"
	"callcenter/internal/limits"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/dbtest"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/user"
)

type env struct {
	*dbtest.Fixture
	svc    *limits.Service
	tenant uuid.UUID
}

func setup(t *testing.T, caps dbtest.Caps) *env {
	f := dbtest.New(t)
	tenant := f.Tenant()
	f.PlanFor(tenant, caps)
	return &env{Fixture: f, svc: limits.NewService(limits.NewRepository()), tenant: tenant}
}

// in runs fn inside a tenant transaction, like every real caller does.
func (e *env) in(fn func(ctx context.Context) error) error {
	return e.DB.InTenant(e.Ctx, e.tenant, fn)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}

func TestDepartmentsAndWidgetsStopAtThePlan(t *testing.T) {
	e := setup(t, dbtest.Caps{Departments: dbtest.Int(2), Widgets: dbtest.Int(1)})
	dept := e.Department(e.tenant, "One")
	if err := e.in(func(ctx context.Context) error { return e.svc.CheckCreate(ctx, e.tenant, limits.KindDepartments) }); err != nil {
		t.Fatalf("second department refused: %v", err)
	}
	e.Department(e.tenant, "Two")
	wantCode(t, e.in(func(ctx context.Context) error { return e.svc.CheckCreate(ctx, e.tenant, limits.KindDepartments) }), "plan_limit")

	if err := e.in(func(ctx context.Context) error { return e.svc.CheckCreate(ctx, e.tenant, limits.KindWidgets) }); err != nil {
		t.Fatalf("first widget refused: %v", err)
	}
	e.Exec(`INSERT INTO widgets (tenant_id, department_id, name, public_key, allowed_origins) VALUES ($1, $2, 'W', $3, ARRAY['https://a.test'])`,
		e.tenant, dept, "wk_"+uuid.NewString()[:22])
	wantCode(t, e.in(func(ctx context.Context) error { return e.svc.CheckCreate(ctx, e.tenant, limits.KindWidgets) }), "plan_limit")
}

func TestUnlimitedPlanNeverRefuses(t *testing.T) {
	e := setup(t, dbtest.Caps{})
	for i := range 5 {
		e.Department(e.tenant, "D"+string(rune('A'+i)))
	}
	if err := e.in(func(ctx context.Context) error { return e.svc.CheckCreate(ctx, e.tenant, limits.KindDepartments) }); err != nil {
		t.Fatalf("unlimited plan refused: %v", err)
	}
	blocked, err := false, error(nil)
	err = e.in(func(ctx context.Context) (err error) { blocked, err = e.svc.CallsBlocked(ctx, e.tenant); return err })
	if err != nil || blocked {
		t.Fatalf("unlimited plan blocks calls: %v %v", blocked, err)
	}
}

func TestSeatsAreCountedAcrossDepartments(t *testing.T) {
	e := setup(t, dbtest.Caps{Agents: dbtest.Int(2)})
	support, sales := e.Department(e.tenant, "Support"), e.Department(e.tenant, "Sales")
	a, b, c := e.Agent(e.tenant, support), e.User(e.tenant, "agent"), e.User(e.tenant, "agent")

	check := func(dept uuid.UUID, ids ...uuid.UUID) error {
		return e.in(func(ctx context.Context) error { return e.svc.CheckSeatsForDepartment(ctx, e.tenant, dept, ids) })
	}
	if err := check(support, a, b); err != nil {
		t.Fatalf("two agents on a two-seat plan: %v", err)
	}
	e.Exec(`INSERT INTO department_members (department_id, user_id, tenant_id) VALUES ($1, $2, $3)`, support, b, e.tenant)
	// The same people in another department use no new seat; a third person does.
	if err := check(sales, a, b); err != nil {
		t.Fatalf("existing agents in a second department were refused: %v", err)
	}
	wantCode(t, check(sales, a, b, c), "plan_limit")
	// Replacing a member keeps the count the same.
	if err := check(support, a, c); err != nil {
		t.Fatalf("swapping an agent was refused: %v", err)
	}
}

func TestOverTheLimitTenantCanStillShrink(t *testing.T) {
	e := setup(t, dbtest.Caps{Agents: dbtest.Int(1)})
	dept := e.Department(e.tenant, "Support")
	a, b := e.Agent(e.tenant, dept), e.Agent(e.tenant, dept) // downgrade left two agents on a one-seat plan

	// Removing one is allowed, and so is changing who stays; adding a third is not.
	if err := e.in(func(ctx context.Context) error {
		return e.svc.CheckSeatsForDepartment(ctx, e.tenant, dept, []uuid.UUID{a})
	}); err != nil {
		t.Fatalf("shrinking was refused: %v", err)
	}
	c := e.User(e.tenant, "agent")
	wantCode(t, e.in(func(ctx context.Context) error {
		return e.svc.CheckSeatsForDepartment(ctx, e.tenant, dept, []uuid.UUID{a, b, c})
	}), "plan_limit")
}

func TestEarliestMembershipsKeepTheirSeat(t *testing.T) {
	e := setup(t, dbtest.Caps{Agents: dbtest.Int(1)})
	dept := e.Department(e.tenant, "Support")
	first := e.Agent(e.tenant, dept)
	time.Sleep(20 * time.Millisecond)
	second := e.Agent(e.tenant, dept)

	holds := func(id uuid.UUID) (ok bool) {
		if err := e.in(func(ctx context.Context) (err error) { ok, err = e.svc.HoldsSeat(ctx, e.tenant, id); return err }); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !holds(first) || holds(second) {
		t.Fatalf("seats: first=%v second=%v, want the earliest membership to keep the only seat", holds(first), holds(second))
	}
	var snap *limits.Snapshot
	if err := e.in(func(ctx context.Context) (err error) { snap, err = e.svc.Snapshot(ctx, e.tenant); return err }); err != nil {
		t.Fatal(err)
	}
	if len(snap.AgentsWithout) != 1 || snap.AgentsWithout[0].ID != second || snap.Usage.Agents != 2 {
		t.Fatalf("snapshot: %d agents, without a seat: %+v", snap.Usage.Agents, snap.AgentsWithout)
	}
}

func TestDisabledAgentsDoNotHoldSeatsAndEnablingChecksTheLimit(t *testing.T) {
	e := setup(t, dbtest.Caps{Agents: dbtest.Int(1)})
	dept := e.Department(e.tenant, "Support")
	active := e.Agent(e.tenant, dept)
	gone := e.Agent(e.tenant, dept)
	e.Exec(`UPDATE users SET status = 'disabled' WHERE id = $1`, gone)

	var usage limits.Usage
	if err := e.in(func(ctx context.Context) (err error) {
		snap, err := e.svc.Snapshot(ctx, e.tenant)
		if snap != nil {
			usage = snap.Usage
		}
		return err
	}); err != nil || usage.Agents != 1 {
		t.Fatalf("agents counted = %d (%v), want the disabled one excluded", usage.Agents, err)
	}
	wantCode(t, e.in(func(ctx context.Context) error { return e.svc.CheckEnableUser(ctx, e.tenant, gone) }), "plan_limit")
	if err := e.in(func(ctx context.Context) error {
		return e.svc.CheckEnableUser(ctx, e.tenant, e.User(e.tenant, "tenant_admin"))
	}); err != nil {
		t.Fatalf("enabling someone who takes no calls was refused: %v", err)
	}
	_ = active
}

func TestCallsAreBlockedOnlyWhenThePolicyBlocks(t *testing.T) {
	e := setup(t, dbtest.Caps{Minutes: dbtest.Int(10), StorageBytes: dbtest.Int64(1000)})
	dept := e.Department(e.tenant, "Support")
	agent := e.Agent(e.tenant, dept)
	blocked := func() (b bool) {
		if err := e.in(func(ctx context.Context) (err error) { b, err = e.svc.CallsBlocked(ctx, e.tenant); return err }); err != nil {
			t.Fatal(err)
		}
		return b
	}
	setPolicy := func(minutes, storage string) {
		e.Exec(`UPDATE tenant_settings SET minutes_cap_policy = $2, storage_cap_policy = $3 WHERE tenant_id = $1`, e.tenant, minutes, storage)
	}

	if blocked() {
		t.Fatal("blocked with no usage")
	}
	call := e.AnsweredCall(e.tenant, dept, agent, time.Now().Add(-time.Minute), 9*60+59)
	if blocked() {
		t.Fatal("blocked just below the minutes cap")
	}
	e.AnsweredCall(e.tenant, dept, agent, time.Now().Add(-30*time.Second), 5)
	if !blocked() {
		t.Fatal("not blocked at the minutes cap with policy block")
	}
	setPolicy("overage", "block")
	if blocked() {
		t.Fatal("blocked on minutes although the policy is overage")
	}
	e.Recording(e.tenant, call, 1000)
	if !blocked() {
		t.Fatal("not blocked at the storage cap with policy block")
	}
	// Soft-deleted recordings still count (D17).
	e.Exec(`UPDATE recordings SET deleted_at = now(), deleted_by = $2 WHERE call_id = $1`, call, e.User(e.tenant, "tenant_admin"))
	if !blocked() {
		t.Fatal("deleting a recording freed storage; it must still count")
	}
	setPolicy("overage", "overage")
	if blocked() {
		t.Fatal("blocked although both policies are overage")
	}
}

func TestCallsOutsideThePeriodDoNotCount(t *testing.T) {
	e := setup(t, dbtest.Caps{Minutes: dbtest.Int(1)})
	dept := e.Department(e.tenant, "Support")
	agent := e.Agent(e.tenant, dept)
	e.AnsweredCall(e.tenant, dept, agent, time.Now().AddDate(0, -2, 0), 3600) // two months ago
	var blocked bool
	if err := e.in(func(ctx context.Context) (err error) { blocked, err = e.svc.CallsBlocked(ctx, e.tenant); return err }); err != nil || blocked {
		t.Fatalf("an old call blocks this month: %v %v", blocked, err)
	}
}

func TestDepartmentServiceEnforcesThePlan(t *testing.T) {
	e := setup(t, dbtest.Caps{Departments: dbtest.Int(1), Agents: dbtest.Int(1)})
	svc := department.NewService(e.DB, department.NewRepository(), user.NewRepository(), audit.NewRecorder(), e.svc)
	admin := e.User(e.tenant, "tenant_admin")
	ctx := reqctx.WithPrincipal(e.Ctx, reqctx.Principal{UserID: admin, TenantID: e.tenant, Role: reqctx.RoleTenantAdmin})

	active := true
	first, err := svc.Create(ctx, department.Input{Name: "Support", Timezone: "UTC", IsActive: &active})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(ctx, department.Input{Name: "Sales", Timezone: "UTC", IsActive: &active})
	wantCode(t, err, "plan_limit")

	a, b := e.User(e.tenant, "agent"), e.User(e.tenant, "agent")
	if _, err := svc.SetMembers(ctx, first.ID, department.UsersInput{UserIDs: []uuid.UUID{a}}); err != nil {
		t.Fatalf("first agent refused: %v", err)
	}
	_, err = svc.SetMembers(ctx, first.ID, department.UsersInput{UserIDs: []uuid.UUID{a, b}})
	wantCode(t, err, "plan_limit")
	// Supervisors take no seat.
	sup := e.User(e.tenant, "supervisor")
	if _, err := svc.SetSupervisors(ctx, first.ID, department.UsersInput{UserIDs: []uuid.UUID{sup}}); err != nil {
		t.Fatalf("supervisor refused: %v", err)
	}
	var n int
	e.Query(`SELECT count(*)::int FROM departments WHERE tenant_id = $1`, []any{e.tenant}, &n)
	if n != 1 {
		t.Fatalf("%d departments exist, want the refused one not to be created", n)
	}
	_ = database.ErrNoTx
}
