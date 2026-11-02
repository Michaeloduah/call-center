package database_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"callcenter/internal/platform/database"
)

// These tests talk to a real Postgres (see `make test`). They prove that the runtime
// role cannot read or write another tenant's rows even if a query forgets its filter.

func setup(t *testing.T) (*database.DB, context.Context) {
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
	return db, ctx
}

func createTenantWithDepartment(t *testing.T, ctx context.Context, db *database.DB) (tenantID, deptID uuid.UUID) {
	t.Helper()
	slug := "rls-" + uuid.NewString()[:8]
	err := db.InPlatform(ctx, func(ctx context.Context) error {
		q, _ := database.Conn(ctx)
		if err := q.QueryRow(ctx, `INSERT INTO tenants (name, slug, status) VALUES ($1, $1, 'active') RETURNING id`, slug).Scan(&tenantID); err != nil {
			return err
		}
		return q.QueryRow(ctx, `INSERT INTO departments (tenant_id, name) VALUES ($1, 'Support') RETURNING id`, tenantID).Scan(&deptID)
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InPlatform(context.Background(), func(ctx context.Context) error {
			q, _ := database.Conn(ctx)
			_, err := q.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
			return err
		})
	})
	return tenantID, deptID
}

func TestTenantScopeHidesOtherTenants(t *testing.T) {
	db, ctx := setup(t)
	tenantA, deptA := createTenantWithDepartment(t, ctx, db)
	_, deptB := createTenantWithDepartment(t, ctx, db)

	err := db.InTenant(ctx, tenantA, func(ctx context.Context) error {
		q, _ := database.Conn(ctx)
		// Deliberately unfiltered query: RLS alone must restrict it.
		rows, err := q.Query(ctx, `SELECT id FROM departments`)
		if err != nil {
			return err
		}
		defer rows.Close()
		var seen []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			seen = append(seen, id)
		}
		if len(seen) != 1 || seen[0] != deptA {
			t.Errorf("tenant A saw departments %v, want only %v", seen, deptA)
		}

		tag, err := q.Exec(ctx, `UPDATE departments SET name = 'hijacked' WHERE id = $1`, deptB)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("tenant A updated tenant B's department")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTenantScopeRejectsWritesIntoOtherTenant(t *testing.T) {
	db, ctx := setup(t)
	tenantA, _ := createTenantWithDepartment(t, ctx, db)
	tenantB, _ := createTenantWithDepartment(t, ctx, db)

	err := db.InTenant(ctx, tenantA, func(ctx context.Context) error {
		q, _ := database.Conn(ctx)
		_, err := q.Exec(ctx, `INSERT INTO departments (tenant_id, name) VALUES ($1, 'Smuggled')`, tenantB)
		return err
	})
	if err == nil {
		t.Fatal("expected row-level security to reject an insert into another tenant")
	}
}

func TestNoScopeSeesNothing(t *testing.T) {
	db, ctx := setup(t)
	createTenantWithDepartment(t, ctx, db)

	// A raw connection with no RLS context (bypassing InTenant/InPlatform) must see no tenant rows.
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_APP_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM departments`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("unscoped query saw %d departments, want 0", n)
	}
}

func TestConnRequiresScopedTransaction(t *testing.T) {
	if _, err := database.Conn(context.Background()); err != database.ErrNoTx {
		t.Fatalf("Conn outside a transaction: got %v, want ErrNoTx", err)
	}
}
