// Package app wires services together. It is the single composition root shared by
// the API server and the CLI, so dependencies are assembled in exactly one place.
package app

import (
	"context"
	"log/slog"

	"callcenter/internal/audit"
	"callcenter/internal/auth"
	"callcenter/internal/billing"
	"callcenter/internal/billing/fake"
	"callcenter/internal/billing/paystack"
	"callcenter/internal/billing/stripe"
	"callcenter/internal/board"
	"callcenter/internal/call"
	"callcenter/internal/callback"
	"callcenter/internal/chat"
	"callcenter/internal/department"
	"callcenter/internal/history"
	"callcenter/internal/limits"
	"callcenter/internal/media"
	"callcenter/internal/platform/config"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/mailer"
	"callcenter/internal/presence"
	"callcenter/internal/realtime"
	"callcenter/internal/recording"
	"callcenter/internal/routing"
	"callcenter/internal/settings"
	"callcenter/internal/storage"
	"callcenter/internal/tenant"
	"callcenter/internal/user"
	"callcenter/internal/widget"
)

type App struct {
	Config config.Config
	DB     *database.DB
	Logger *slog.Logger

	Audit          *audit.Recorder
	AuditQuery     *audit.QueryService
	Auth           *auth.Service
	AuthLimits     *auth.Limiters
	Users          *user.Service
	UserRepo       user.Repository
	Tenants        *tenant.Service
	Settings       *settings.Service
	Departments    *department.Service
	Widgets        *widget.Service
	Calls          *call.Service
	Billing        *billing.Service
	Recordings     *recording.Service
	Chat           *chat.Service
	Callbacks      *callback.Service
	History        *history.Service
	Storage        storage.Store
	Presence       *presence.Service
	Routing        *routing.Service
	Router         *routing.Router
	Board          *board.Service
	Hub            *realtime.Hub
	Media          media.Provider
	CallLimits     *httpx.RateLimiter
	ChatLimits     *httpx.RateLimiter
	CallbackLimits *httpx.RateLimiter
	// FakeBilling is set in development; its webhook endpoint is only routed then.
	FakeBilling *fake.Provider
}

func New(cfg config.Config, db *database.DB, logger *slog.Logger) (*App, error) {
	smtp, err := mailer.NewSMTP(cfg.SMTP)
	if err != nil {
		return nil, err
	}
	recorder := audit.NewRecorder()
	planLimits := limits.NewService(limits.NewRepository())
	userRepo := user.NewRepository()
	tenantRepo := tenant.NewRepository()
	provisioner := tenant.NewProvisioner(tenantRepo, userRepo)

	authSvc := auth.NewService(db, auth.NewRepository(), auth.NewNotifier(smtp, cfg.WebBaseURL), provisioner,
		recorder, cfg.SessionTTL, logger)

	mediaProvider := media.NewLiveKit(cfg.LiveKit)
	widgetRepo := widget.NewRepository()
	store, err := storage.New(cfg.Storage)
	if err != nil {
		return nil, err
	}
	registry, fakeBilling := newBillingRegistry(cfg)
	billingSvc := billing.NewService(db, billing.NewRepository(), planLimits, registry, tenantRepo,
		billing.NewEmailNotifier(smtp, cfg.WebBaseURL, logger), recorder,
		billing.Config{ReturnURL: cfg.WebBaseURL + "/admin/billing"}, logger)
	recordings := recording.NewService(db, recording.NewRepository(), mediaProvider, store, recorder, billingSvc, logger)
	presenceRepo := presence.NewRepository()
	routingRepo := routing.NewRepository()
	router := routing.NewRouter(db, routingRepo, presenceRepo, mediaProvider, logger)

	calls := call.NewService(db, call.NewRepository(), widgetRepo, mediaProvider, router, planLimits, billingSvc, logger)
	chatLimits := chat.NewLimiter()
	chatRepo := chat.NewRepository()

	return &App{
		Config:      cfg,
		DB:          db,
		Logger:      logger,
		Audit:       recorder,
		AuditQuery:  audit.NewQueryService(db),
		Auth:        authSvc,
		AuthLimits:  auth.NewLimiters(),
		Users:       user.NewService(db, userRepo, authSvc, authSvc, recorder, planLimits),
		UserRepo:    userRepo,
		Tenants:     tenant.NewService(db, tenantRepo, provisioner, authSvc, authSvc, recorder),
		Settings:    settings.NewService(db, settings.NewRepository(), recorder),
		Departments: department.NewService(db, department.NewRepository(), userRepo, recorder, planLimits),
		Widgets:     widget.NewService(db, widgetRepo, recorder, planLimits),
		Calls:       calls,
		Chat:        chat.NewService(db, chatRepo, calls, mediaProvider, chatLimits, logger),
		Callbacks: callback.NewService(db, callback.NewRepository(), widgetRepo, calls,
			callback.NewEmailNotifier(smtp, cfg.WebBaseURL, logger), logger),
		History:        history.NewService(db, history.NewRepository(), chatRepo),
		ChatLimits:     chatLimits,
		CallbackLimits: callback.NewCreateLimiter(),
		Billing:        billingSvc,
		FakeBilling:    fakeBilling,
		Recordings:     recordings,
		Storage:        store,
		Presence:       presence.NewService(db, presenceRepo, router, planLimits),
		Routing:        routing.NewService(db, routingRepo, presenceRepo, mediaProvider, router, recordings),
		Router:         router,
		Board:          board.NewService(db),
		Hub:            realtime.NewHub(),
		Media:          mediaProvider,
		CallLimits:     call.NewCreateLimiter(),
	}, nil
}

// RunBackground starts the long-running workers (call routing, event fan-out) and returns
// immediately. They stop when ctx ends. Only the API server runs them, not the CLI.
func (a *App) RunBackground(ctx context.Context) {
	if err := a.Storage.EnsureBucket(ctx); err != nil {
		a.Logger.Error("recording storage is not ready; recordings will fail until it is", "error", err)
	}
	go a.Router.Run(ctx)
	go a.Recordings.Run(ctx)
	go a.Billing.Run(ctx)
	go a.Hub.Run(ctx, a.DB, a.Logger)
}

// newBillingRegistry builds the payment providers from configuration. In development every
// currency uses the in-memory provider; otherwise Paystack and Stripe are enabled by their keys.
func newBillingRegistry(cfg config.Config) (billing.Registry, *fake.Provider) {
	b := cfg.Billing
	if b.Fake {
		f := fake.New(b.FakeSecret, cfg.WebBaseURL+"/admin/billing/fake-checkout")
		return billing.NewRegistry(true, b.PaystackCurrencies, f), f
	}
	var providers []billing.Provider
	if b.PaystackSecretKey != "" {
		providers = append(providers, paystack.New(paystack.DefaultBaseURL, b.PaystackSecretKey))
	}
	if b.StripeSecretKey != "" && b.StripeWebhookKey != "" {
		providers = append(providers, stripe.New(stripe.DefaultBaseURL, b.StripeSecretKey, b.StripeWebhookKey))
	}
	return billing.NewRegistry(false, b.PaystackCurrencies, providers...), nil
}
