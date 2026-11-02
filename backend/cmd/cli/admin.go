package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"callcenter/internal/app"
	"callcenter/internal/audit"
	"callcenter/internal/auth"
	"callcenter/internal/platform/config"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/platform/validate"
	"callcenter/internal/user"
)

// runCreatePlatformAdmin bootstraps a platform admin and emails them an invitation link.
// Passwords are never passed on the command line; the admin sets one via the invite.
func runCreatePlatformAdmin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create-platform-admin", flag.ContinueOnError)
	email := fs.String("email", "", "admin email address")
	name := fs.String("name", "", "admin full name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	v := validate.Errors{}
	*email = v.Email("email", *email)
	*name = v.RequiredString("name", *name, 120)
	if len(v) > 0 {
		return fmt.Errorf("invalid input: %v", map[string]string(v))
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	a, err := app.New(cfg, db, logger)
	if err != nil {
		return err
	}

	var (
		created user.User
		token   string
	)
	err = db.InPlatform(ctx, func(ctx context.Context) (err error) {
		created, err = a.UserRepo.Insert(ctx, user.NewUser{
			Email: *email, FullName: *name, Role: reqctx.RolePlatformAdmin, Status: user.StatusInvited,
		})
		if err != nil {
			return err
		}
		if token, err = a.Auth.IssueInvite(ctx, created.ID); err != nil {
			return err
		}
		return a.Audit.Record(ctx, audit.Entry{Action: audit.ActionPlatformAdminCreated, EntityType: "user",
			EntityID: created.ID, ActorEmail: "cli", Metadata: map[string]any{"email": created.Email}})
	})
	if errors.Is(err, user.ErrEmailTaken) {
		return fmt.Errorf("a user with email %s already exists", *email)
	}
	if err != nil {
		return err
	}
	if !a.Auth.SendInvite(ctx, auth.InviteEmail{To: created.Email, FullName: created.FullName, Token: token}) {
		return errors.New("platform admin created, but the invitation email could not be sent; check SMTP settings")
	}
	fmt.Printf("Platform admin %s created. An invitation email was sent to %s.\n", created.FullName, created.Email)
	return nil
}
