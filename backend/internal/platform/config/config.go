// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env             string // "development" or "production"
	HTTPAddr        string
	DatabaseURL     string // runtime role (subject to RLS)
	MigrationsDBURL string // owner role, used only by the CLI for migrations
	WebBaseURL      string // used to build links in emails
	SessionTTL      time.Duration
	TrustedProxies  []netip.Prefix // peers allowed to set X-Forwarded-For (the Next.js server)
	SMTP            SMTPConfig
	LiveKit         LiveKitConfig
	Storage         StorageConfig
	Billing         BillingConfig
	ShutdownTimeout time.Duration
}

// BillingConfig selects and configures payment providers (D13, D19).
type BillingConfig struct {
	// Fake replaces every provider with an in-memory one (development and tests only).
	Fake       bool
	FakeSecret string
	// PaystackCurrencies are charged through Paystack; every other currency goes to Stripe.
	PaystackCurrencies []string
	PaystackSecretKey  string
	StripeSecretKey    string
	StripeWebhookKey   string
}

// StorageConfig is the S3-compatible object store that holds call recordings. The same store is
// reached at up to three addresses: by the API, by the recorder (inside the media network) and
// by browsers playing a recording through a presigned URL.
type StorageConfig struct {
	Endpoint       string // host:port used by the API, e.g. localhost:19000
	PublicEndpoint string // host:port browsers use; presigned URLs are signed for it
	EgressEndpoint string // full URL the recorder uploads to, e.g. http://minio:9000
	AccessKey      string
	SecretKey      string
	Bucket         string
	Region         string
	UseSSL         bool // for Endpoint and PublicEndpoint
}

type LiveKitConfig struct {
	URL       string // server API base, e.g. http://localhost:7880
	PublicURL string // WebSocket URL browsers connect to, e.g. ws://localhost:7880
	APIKey    string
	APISecret string
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads configuration and fails fast on anything missing or malformed.
func Load() (Config, error) {
	var errs []error
	get := func(key string) string {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
		return v
	}
	getDefault := func(key, def string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := Config{
		Env:             getDefault("APP_ENV", "development"),
		HTTPAddr:        getDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:     get("DATABASE_URL"),
		MigrationsDBURL: getDefault("MIGRATIONS_DATABASE_URL", ""),
		WebBaseURL:      strings.TrimRight(get("WEB_BASE_URL"), "/"),
		SMTP: SMTPConfig{
			Host:     get("SMTP_HOST"),
			Username: getDefault("SMTP_USERNAME", ""),
			Password: getDefault("SMTP_PASSWORD", ""),
			From:     get("SMTP_FROM"),
		},
		LiveKit: LiveKitConfig{
			URL:       get("LIVEKIT_URL"),
			PublicURL: get("LIVEKIT_PUBLIC_URL"),
			APIKey:    get("LIVEKIT_API_KEY"),
			APISecret: get("LIVEKIT_API_SECRET"),
		},
		Storage: StorageConfig{
			Endpoint:       get("STORAGE_ENDPOINT"),
			PublicEndpoint: get("STORAGE_PUBLIC_ENDPOINT"),
			EgressEndpoint: get("STORAGE_EGRESS_ENDPOINT"),
			AccessKey:      get("STORAGE_ACCESS_KEY"),
			SecretKey:      get("STORAGE_SECRET_KEY"),
			Bucket:         get("STORAGE_BUCKET"),
			Region:         getDefault("STORAGE_REGION", "us-east-1"),
		},
	}

	if cfg.Env != "development" && cfg.Env != "production" {
		errs = append(errs, fmt.Errorf("APP_ENV must be development or production, got %q", cfg.Env))
	}

	var err error
	if cfg.Billing.Fake, err = strconv.ParseBool(getDefault("BILLING_FAKE", strconv.FormatBool(cfg.Env != "production"))); err != nil {
		errs = append(errs, fmt.Errorf("BILLING_FAKE: %w", err))
	}
	if cfg.Billing.Fake && cfg.IsProduction() {
		errs = append(errs, errors.New("BILLING_FAKE must not be enabled in production"))
	}
	cfg.Billing.FakeSecret = getDefault("BILLING_FAKE_SECRET", "fake-billing-dev-secret")
	cfg.Billing.PaystackSecretKey = getDefault("PAYSTACK_SECRET_KEY", "")
	cfg.Billing.StripeSecretKey = getDefault("STRIPE_SECRET_KEY", "")
	cfg.Billing.StripeWebhookKey = getDefault("STRIPE_WEBHOOK_SECRET", "")
	for _, c := range strings.Split(getDefault("BILLING_PAYSTACK_CURRENCIES", "NGN"), ",") {
		if c = strings.ToUpper(strings.TrimSpace(c)); c != "" {
			cfg.Billing.PaystackCurrencies = append(cfg.Billing.PaystackCurrencies, c)
		}
	}
	if cfg.SMTP.Port, err = strconv.Atoi(getDefault("SMTP_PORT", "587")); err != nil {
		errs = append(errs, fmt.Errorf("SMTP_PORT: %w", err))
	}
	if cfg.Storage.UseSSL, err = strconv.ParseBool(getDefault("STORAGE_USE_SSL", "false")); err != nil {
		errs = append(errs, fmt.Errorf("STORAGE_USE_SSL: %w", err))
	}
	if cfg.SessionTTL, err = time.ParseDuration(getDefault("SESSION_TTL", "720h")); err != nil {
		errs = append(errs, fmt.Errorf("SESSION_TTL: %w", err))
	}
	if cfg.ShutdownTimeout, err = time.ParseDuration(getDefault("SHUTDOWN_TIMEOUT", "15s")); err != nil {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err))
	}
	for _, raw := range strings.Split(getDefault("TRUSTED_PROXIES", "127.0.0.1/32,::1/128"), ",") {
		p, perr := netip.ParsePrefix(strings.TrimSpace(raw))
		if perr != nil {
			errs = append(errs, fmt.Errorf("TRUSTED_PROXIES: %w", perr))
			continue
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, p)
	}

	return cfg, errors.Join(errs...)
}
