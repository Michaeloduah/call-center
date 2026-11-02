// Package settings manages platform-wide and per-tenant configuration.
package settings

import (
	"context"
	"time"

	"callcenter/internal/audit"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/platform/validate"
)

type PlatformSettings struct {
	DefaultConsentText string    `json:"default_consent_text"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type TenantSettings struct {
	OfferTimeoutSeconds  int       `json:"offer_timeout_seconds"`
	MaxQueueWaitSeconds  int       `json:"max_queue_wait_seconds"`
	ConsentTextOverride  *string   `json:"consent_text_override"`
	DefaultConsentText   string    `json:"default_consent_text"`
	EffectiveConsentText string    `json:"effective_consent_text"`
	UpdatedAt            time.Time `json:"updated_at"`
}

const maxConsentTextLen = 2000

type UpdatePlatformInput struct {
	DefaultConsentText string `json:"default_consent_text"`
}

func (in *UpdatePlatformInput) Validate() error {
	v := validate.Errors{}
	in.DefaultConsentText = v.RequiredString("default_consent_text", in.DefaultConsentText, maxConsentTextLen)
	return v.Err()
}

type UpdateTenantInput struct {
	OfferTimeoutSeconds int     `json:"offer_timeout_seconds"`
	MaxQueueWaitSeconds int     `json:"max_queue_wait_seconds"`
	ConsentTextOverride *string `json:"consent_text_override"` // null or blank = use platform default
}

func (in *UpdateTenantInput) Validate() error {
	v := validate.Errors{}
	v.IntRange("offer_timeout_seconds", in.OfferTimeoutSeconds, 5, 120)
	v.IntRange("max_queue_wait_seconds", in.MaxQueueWaitSeconds, 30, 3600)
	if in.ConsentTextOverride != nil {
		text := v.OptionalString("consent_text_override", *in.ConsentTextOverride, maxConsentTextLen)
		if text == "" {
			in.ConsentTextOverride = nil
		} else {
			in.ConsentTextOverride = &text
		}
	}
	return v.Err()
}

type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

type Service struct {
	tx    database.TxManager
	repo  Repository
	audit AuditRecorder
}

func NewService(tx database.TxManager, repo Repository, recorder AuditRecorder) *Service {
	return &Service{tx: tx, repo: repo, audit: recorder}
}

func (s *Service) GetPlatform(ctx context.Context) (*PlatformSettings, error) {
	var out *PlatformSettings
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		out, err = s.repo.GetPlatform(ctx)
		return err
	})
	return out, err
}

func (s *Service) UpdatePlatform(ctx context.Context, in UpdatePlatformInput) (*PlatformSettings, error) {
	p, _ := reqctx.PrincipalFrom(ctx)
	var out *PlatformSettings
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		if err := s.repo.UpdatePlatform(ctx, in.DefaultConsentText, p.UserID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{Action: audit.ActionPlatformSettings, EntityType: "platform_settings"}); err != nil {
			return err
		}
		out, err = s.repo.GetPlatform(ctx)
		return err
	})
	return out, err
}

func (s *Service) GetTenant(ctx context.Context) (*TenantSettings, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var out *TenantSettings
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		out, err = s.repo.GetTenant(ctx, p.TenantID)
		return err
	})
	return out, err
}

func (s *Service) UpdateTenant(ctx context.Context, in UpdateTenantInput) (*TenantSettings, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var out *TenantSettings
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		if err := s.repo.UpdateTenant(ctx, p.TenantID, in); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{TenantID: p.TenantID, Action: audit.ActionTenantSettingsUpdate,
			EntityType: "tenant_settings", EntityID: p.TenantID, Metadata: map[string]any{
				"offer_timeout_seconds": in.OfferTimeoutSeconds, "max_queue_wait_seconds": in.MaxQueueWaitSeconds,
				"consent_overridden": in.ConsentTextOverride != nil}}); err != nil {
			return err
		}
		out, err = s.repo.GetTenant(ctx, p.TenantID)
		return err
	})
	return out, err
}
