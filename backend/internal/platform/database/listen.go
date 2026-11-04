package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	listenRetryMin = 500 * time.Millisecond
	listenRetryMax = 10 * time.Second
)

// LockTenant takes a row lock on the tenant for the rest of the transaction. Work that must be
// serialised per tenant (creating calls under the live-call cap, dispatching calls to agents)
// takes this lock first, which also makes it correct across several API instances.
func LockTenant(ctx context.Context, tenantID uuid.UUID) error {
	q, err := Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `SELECT 1 FROM tenants WHERE id = $1 FOR UPDATE`, tenantID); err != nil {
		return fmt.Errorf("lock tenant: %w", err)
	}
	return nil
}

// Notify sends a NOTIFY on channel inside the current transaction. Postgres delivers it only if
// the transaction commits, so listeners never hear about changes that were rolled back.
func Notify(ctx context.Context, channel, payload string) error {
	q, err := Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, payload); err != nil {
		return fmt.Errorf("notify %s: %w", channel, err)
	}
	return nil
}

// Listen delivers every notification on channel to handle until ctx ends. It uses a dedicated
// connection (not the pool) and reconnects with backoff. Notifications sent while it is
// disconnected are lost; callers must treat them as hints and re-read state.
func (db *DB) Listen(ctx context.Context, channel string, logger *slog.Logger, handle func(payload string)) {
	backoff := listenRetryMin
	for ctx.Err() == nil {
		err := db.listenOnce(ctx, channel, handle, func() { backoff = listenRetryMin })
		if ctx.Err() != nil {
			return
		}
		logger.Warn("notification listener disconnected", "channel", channel, "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, listenRetryMax)
	}
}

func (db *DB) listenOnce(ctx context.Context, channel string, handle func(string), connected func()) error {
	conn, err := pgx.ConnectConfig(ctx, db.pool.Config().ConnConfig)
	if err != nil {
		return fmt.Errorf("listen connect: %w", err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return fmt.Errorf("listen %s: %w", channel, err)
	}
	connected()
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		handle(n.Payload)
	}
}
