package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"

	"callcenter/migrations"
)

// Migrate runs a goose command ("up", "down", "status", ...) against url.
// url must use the owner role: the runtime role cannot create tables.
func Migrate(ctx context.Context, url, command string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open migrations db: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.RunContext(ctx, command, db, "."); err != nil {
		return fmt.Errorf("goose %s: %w", command, err)
	}
	return nil
}
