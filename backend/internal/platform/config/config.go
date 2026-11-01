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
	ShutdownTimeout time.Duration
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
	}

	if cfg.Env != "development" && cfg.Env != "production" {
		errs = append(errs, fmt.Errorf("APP_ENV must be development or production, got %q", cfg.Env))
	}

	var err error
	if cfg.SMTP.Port, err = strconv.Atoi(getDefault("SMTP_PORT", "587")); err != nil {
		errs = append(errs, fmt.Errorf("SMTP_PORT: %w", err))
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
