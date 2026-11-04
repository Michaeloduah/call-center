// Package dbtest builds throwaway tenants, users, departments and calls in a real Postgres for
// integration tests. Tests skip when TEST_DATABASE_URL / TEST_APP_DATABASE_URL are not set
// (see `make test`). Everything is created through the runtime role, so row-level security applies.
package dbtest

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
)

type Fixture struct {
	T   *testing.T
	DB  *database.DB
	Ctx context.Context
}

// New connects as the runtime role after migrating the test database.
func New(t *testing.T) *Fixture {
	t.Helper()
	ownerURL, appURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_APP_DATABASE_URL")
	if ownerURL == "" || appURL == "" {
		t.Skip("TEST_DATABASE_URL / TEST_APP_DATABASE_URL not set")
	}
	ctx := context.Background()
	if err := database.Migrate(ctx, ownerURL, "up"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := database.Connect(ctx, appURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	return &Fixture{T: t, DB: db, Ctx: ctx}
}

// Exec runs one statement in platform scope and fails the test on error.
func (f *Fixture) Exec(sql string, args ...any) {
	f.T.Helper()
	f.InPlatform(func(q database.Querier) {
		if _, err := q.Exec(f.Ctx, sql, args...); err != nil {
			f.T.Fatalf("exec %q: %v", sql, err)
		}
	})
}

// InPlatform runs fn in a platform-scoped transaction.
func (f *Fixture) InPlatform(fn func(q database.Querier)) {
	f.T.Helper()
	err := f.DB.InPlatform(f.Ctx, func(ctx context.Context) error {
		q, err := database.Conn(ctx)
		if err != nil {
			return err
		}
		fn(q)
		return nil
	})
	if err != nil {
		f.T.Fatalf("platform tx: %v", err)
	}
}

// Tenant creates an active tenant with default settings and removes it (and everything under
// it) when the test ends.
func (f *Fixture) Tenant() uuid.UUID {
	f.T.Helper()
	slug := "t-" + uuid.NewString()[:8]
	var id uuid.UUID
	f.InPlatform(func(q database.Querier) {
		if err := q.QueryRow(f.Ctx, `INSERT INTO tenants (name, slug, status) VALUES ($1, $1, 'active') RETURNING id`, slug).Scan(&id); err != nil {
			f.T.Fatalf("create tenant: %v", err)
		}
		if _, err := q.Exec(f.Ctx, `INSERT INTO tenant_settings (tenant_id) VALUES ($1)`, id); err != nil {
			f.T.Fatalf("create tenant settings: %v", err)
		}
	})
	f.T.Cleanup(func() {
		_ = f.DB.InPlatform(context.Background(), func(ctx context.Context) error {
			q, _ := database.Conn(ctx)
			_, err := q.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
			return err
		})
	})
	return id
}

func (f *Fixture) Department(tenantID uuid.UUID, name string) uuid.UUID {
	f.T.Helper()
	var id uuid.UUID
	f.InPlatform(func(q database.Querier) {
		if err := q.QueryRow(f.Ctx, `INSERT INTO departments (tenant_id, name) VALUES ($1, $2) RETURNING id`, tenantID, name).Scan(&id); err != nil {
			f.T.Fatalf("create department: %v", err)
		}
	})
	return id
}

// User creates an active user with the given role.
func (f *Fixture) User(tenantID uuid.UUID, role string) uuid.UUID {
	f.T.Helper()
	var id uuid.UUID
	f.InPlatform(func(q database.Querier) {
		name := role + "-" + uuid.NewString()[:6]
		if err := q.QueryRow(f.Ctx, `
			INSERT INTO users (tenant_id, email, password_hash, full_name, role, status)
			VALUES ($1, $2, 'x', $3, $4, 'active') RETURNING id`, tenantID, name+"@example.test", name, role).Scan(&id); err != nil {
			f.T.Fatalf("create user: %v", err)
		}
	})
	return id
}

// Agent creates an agent who is a member of the given departments.
func (f *Fixture) Agent(tenantID uuid.UUID, departments ...uuid.UUID) uuid.UUID {
	f.T.Helper()
	id := f.User(tenantID, "agent")
	for _, d := range departments {
		f.Exec(`INSERT INTO department_members (department_id, user_id, tenant_id) VALUES ($1, $2, $3)`, d, id, tenantID)
	}
	return id
}

func (f *Fixture) Supervisor(tenantID uuid.UUID, departments ...uuid.UUID) uuid.UUID {
	f.T.Helper()
	id := f.User(tenantID, "supervisor")
	for _, d := range departments {
		f.Exec(`INSERT INTO department_supervisors (department_id, user_id, tenant_id) VALUES ($1, $2, $3)`, d, id, tenantID)
	}
	return id
}

// Presence stores an agent's presence. idleFor is how long they have been free; beatAgo is the
// age of their last heartbeat.
func (f *Fixture) Presence(tenantID, userID uuid.UUID, status string, idleFor, beatAgo time.Duration) {
	f.T.Helper()
	f.Exec(`
		INSERT INTO agent_presence (user_id, tenant_id, status, idle_since, last_heartbeat_at)
		VALUES ($1, $2, $3, now() - make_interval(secs => $4), now() - make_interval(secs => $5))
		ON CONFLICT (user_id) DO UPDATE SET status = $3, idle_since = now() - make_interval(secs => $4),
		    last_heartbeat_at = now() - make_interval(secs => $5), consecutive_timeouts = 0`,
		userID, tenantID, status, idleFor.Seconds(), beatAgo.Seconds())
}

// Available makes the agent available with a fresh heartbeat, idle for the given time.
func (f *Fixture) Available(tenantID, userID uuid.UUID, idleFor time.Duration) {
	f.T.Helper()
	f.Presence(tenantID, userID, "available", idleFor, 0)
}

// Call inserts a queued call that entered the queue waitedFor ago.
func (f *Fixture) Call(tenantID, departmentID uuid.UUID, waitedFor time.Duration) uuid.UUID {
	f.T.Helper()
	id := uuid.New()
	token := make([]byte, 32)
	_, _ = rand.Read(token)
	f.Exec(`
		INSERT INTO calls (id, tenant_id, department_id, room_name, status, caller_name, consent_text, consent_given_at,
		                   caller_token_hash, caller_token_expires_at, queued_at)
		VALUES ($1, $2, $3, $4, 'queued', 'Caller', 'consent', now(), $5, now() + interval '1 hour', now() - make_interval(secs => $6))`,
		id, tenantID, departmentID, "call-"+id.String(), token, waitedFor.Seconds())
	return id
}

// Query runs a single-row query and scans the result into dest.
func (f *Fixture) Query(sql string, args []any, dest ...any) {
	f.T.Helper()
	f.InPlatform(func(q database.Querier) {
		if err := q.QueryRow(f.Ctx, sql, args...).Scan(dest...); err != nil {
			f.T.Fatalf("query %q: %v", sql, err)
		}
	})
}

// CallStatus returns a call's status and end reason.
func (f *Fixture) CallStatus(id uuid.UUID) (status string, endReason *string) {
	f.T.Helper()
	f.Query(`SELECT status, end_reason FROM calls WHERE id = $1`, []any{id}, &status, &endReason)
	return status, endReason
}

// OfferOutcome returns the outcome of the most recent offer of a call for an agent ("" if none).
func (f *Fixture) OfferOutcome(callID, agentID uuid.UUID) string {
	f.T.Helper()
	var outcome *string
	f.Query(`SELECT (SELECT outcome FROM call_offers WHERE call_id = $1 AND agent_id = $2 ORDER BY offered_at DESC LIMIT 1)`,
		[]any{callID, agentID}, &outcome)
	if outcome == nil {
		return ""
	}
	return *outcome
}

// PendingOfferID returns the live offer of a call, or uuid.Nil.
func (f *Fixture) PendingOfferID(callID uuid.UUID) uuid.UUID {
	f.T.Helper()
	var id *uuid.UUID
	f.Query(`SELECT (SELECT id FROM call_offers WHERE call_id = $1 AND outcome = 'pending')`, []any{callID}, &id)
	if id == nil {
		return uuid.Nil
	}
	return *id
}

// OfferedAgent returns who holds the live offer of a call, or uuid.Nil.
func (f *Fixture) OfferedAgent(callID uuid.UUID) uuid.UUID {
	f.T.Helper()
	var id *uuid.UUID
	f.Query(`SELECT (SELECT agent_id FROM call_offers WHERE call_id = $1 AND outcome = 'pending')`, []any{callID}, &id)
	if id == nil {
		return uuid.Nil
	}
	return *id
}
