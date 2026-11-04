// Package app wires services together. It is the single composition root shared by
// the API server and the CLI, so dependencies are assembled in exactly one place.
package app

import (
	"context"
	"log/slog"

	"callcenter/internal/audit"
	"callcenter/internal/auth"
	"callcenter/internal/board"
	"callcenter/internal/call"
	"callcenter/internal/department"
	"callcenter/internal/media"
	"callcenter/internal/platform/config"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/mailer"
	"callcenter/internal/presence"
	"callcenter/internal/realtime"
	"callcenter/internal/routing"
	"callcenter/internal/settings"
	"callcenter/internal/tenant"
	"callcenter/internal/user"
	"callcenter/internal/widget"
)

type App struct {
	Config config.Config
	DB     *database.DB
	Logger *slog.Logger

	Audit       *audit.Recorder
	AuditQuery  *audit.QueryService
	Auth        *auth.Service
	AuthLimits  *auth.Limiters
	Users       *user.Service
	UserRepo    user.Repository
	Tenants     *tenant.Service
	Settings    *settings.Service
	Departments *department.Service
	Widgets     *widget.Service
	Calls       *call.Service
	Presence    *presence.Service
	Routing     *routing.Service
	Router      *routing.Router
	Board       *board.Service
	Hub         *realtime.Hub
	Media       media.Provider
	CallLimits  *httpx.RateLimiter
}

func New(cfg config.Config, db *database.DB, logger *slog.Logger) (*App, error) {
	smtp, err := mailer.NewSMTP(cfg.SMTP)
	if err != nil {
		return nil, err
	}
	recorder := audit.NewRecorder()
	userRepo := user.NewRepository()
	tenantRepo := tenant.NewRepository()
	provisioner := tenant.NewProvisioner(tenantRepo, userRepo)

	authSvc := auth.NewService(db, auth.NewRepository(), auth.NewNotifier(smtp, cfg.WebBaseURL), provisioner,
		recorder, cfg.SessionTTL, logger)

	mediaProvider := media.NewLiveKit(cfg.LiveKit)
	widgetRepo := widget.NewRepository()
	presenceRepo := presence.NewRepository()
	routingRepo := routing.NewRepository()
	router := routing.NewRouter(db, routingRepo, presenceRepo, mediaProvider, logger)

	return &App{
		Config:      cfg,
		DB:          db,
		Logger:      logger,
		Audit:       recorder,
		AuditQuery:  audit.NewQueryService(db),
		Auth:        authSvc,
		AuthLimits:  auth.NewLimiters(),
		Users:       user.NewService(db, userRepo, authSvc, authSvc, recorder),
		UserRepo:    userRepo,
		Tenants:     tenant.NewService(db, tenantRepo, provisioner, authSvc, authSvc, recorder),
		Settings:    settings.NewService(db, settings.NewRepository(), recorder),
		Departments: department.NewService(db, department.NewRepository(), userRepo, recorder),
		Widgets:     widget.NewService(db, widgetRepo, recorder),
		Calls:       call.NewService(db, call.NewRepository(), widgetRepo, mediaProvider, router, logger),
		Presence:    presence.NewService(db, presenceRepo, router),
		Routing:     routing.NewService(db, routingRepo, presenceRepo, mediaProvider, router),
		Router:      router,
		Board:       board.NewService(db),
		Hub:         realtime.NewHub(),
		Media:       mediaProvider,
		CallLimits:  call.NewCreateLimiter(),
	}, nil
}

// RunBackground starts the long-running workers (call routing, event fan-out) and returns
// immediately. They stop when ctx ends. Only the API server runs them, not the CLI.
func (a *App) RunBackground(ctx context.Context) {
	go a.Router.Run(ctx)
	go a.Hub.Run(ctx, a.DB, a.Logger)
}
