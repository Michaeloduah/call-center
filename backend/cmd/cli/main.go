// Command cli runs operational tasks: schema migrations and bootstrapping the first platform admin.
//
//	cli migrate up|down|status
//	cli create-platform-admin -email admin@example.com -name "Jane Doe"
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"callcenter/internal/platform/database"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: cli <migrate|create-platform-admin> [args]")
	}
	switch args[0] {
	case "migrate":
		return runMigrate(ctx, args[1:])
	case "create-platform-admin":
		return runCreatePlatformAdmin(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runMigrate(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: cli migrate <up|down|status>")
	}
	url := strings.TrimSpace(os.Getenv("MIGRATIONS_DATABASE_URL"))
	if url == "" {
		return errors.New("MIGRATIONS_DATABASE_URL is required")
	}
	return database.Migrate(ctx, url, args[0])
}
