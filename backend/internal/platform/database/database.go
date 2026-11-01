// Package database owns the connection pool and tenant-scoped transactions.
//
// Every query runs inside a transaction that sets the row-level security context.
// Repositories obtain that transaction from the context via Conn, so a query can
// never run without an explicit tenant or platform scope.
package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of pgx used by repositories.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxManager runs a function inside a scoped transaction. Services depend on this interface.
type TxManager interface {
	InTenant(ctx context.Context, tenantID uuid.UUID, fn func(ctx context.Context) error) error
	InPlatform(ctx context.Context, fn func(ctx context.Context) error) error
}

type DB struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (db *DB) Close() { db.pool.Close() }

func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

type txKey struct{}

// InTenant runs fn in a transaction where RLS restricts rows to tenantID.
func (db *DB) InTenant(ctx context.Context, tenantID uuid.UUID, fn func(ctx context.Context) error) error {
	if tenantID == uuid.Nil {
		return errors.New("database: tenant scope requires a tenant id")
	}
	return db.inTx(ctx, "SELECT set_config('app.tenant_id', $1, true)", []any{tenantID.String()}, fn)
}

// InPlatform runs fn in a transaction that may see all tenants. Use only for
// platform-admin operations and system code paths (auth, webhooks).
func (db *DB) InPlatform(ctx context.Context, fn func(ctx context.Context) error) error {
	return db.inTx(ctx, "SELECT set_config('app.platform', 'on', true)", nil, fn)
}

func (db *DB) inTx(ctx context.Context, scopeSQL string, scopeArgs []any, fn func(ctx context.Context) error) error {
	if _, nested := ctx.Value(txKey{}).(pgx.Tx); nested {
		return errors.New("database: nested scoped transactions are not supported")
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	if _, err := tx.Exec(ctx, scopeSQL, scopeArgs...); err != nil {
		return fmt.Errorf("set rls scope: %w", err)
	}
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ErrNoTx means a repository was called outside InTenant/InPlatform.
var ErrNoTx = errors.New("database: no scoped transaction in context")

// Conn returns the scoped transaction carried by ctx.
func Conn(ctx context.Context) (Querier, error) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	if !ok {
		return nil, ErrNoTx
	}
	return tx, nil
}

// IsUniqueViolation reports whether err is a unique-constraint violation on the named constraint
// (any constraint when name is empty).
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// IsForeignKeyViolation reports whether err is a foreign-key violation.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// IsNoRows reports whether err means the query matched nothing.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ContainsPattern turns user input into an ILIKE "contains" pattern with LIKE wildcards escaped.
// It returns "" for empty input so queries can use ($n = ” OR col ILIKE $n).
func ContainsPattern(s string) string {
	if s == "" {
		return ""
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}
