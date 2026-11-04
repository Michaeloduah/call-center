package routing_test

import (
	"context"

	"callcenter/internal/platform/database"
)

func execErr(ctx context.Context, sql string, args ...any) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, sql, args...)
	return err
}

func scanOne(ctx context.Context, sql string, dest ...any) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	return q.QueryRow(ctx, sql).Scan(dest...)
}
