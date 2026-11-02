package settings

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
)

// Repository persists settings inside the caller's scoped transaction.
type Repository interface {
	GetPlatform(ctx context.Context) (*PlatformSettings, error)
	UpdatePlatform(ctx context.Context, defaultConsentText string, updatedBy uuid.UUID) error
	GetTenant(ctx context.Context, tenantID uuid.UUID) (*TenantSettings, error)
	UpdateTenant(ctx context.Context, tenantID uuid.UUID, in UpdateTenantInput) error
}

type pgRepository struct{}

func NewRepository() Repository { return pgRepository{} }

func (pgRepository) GetPlatform(ctx context.Context) (*PlatformSettings, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var ps PlatformSettings
	if err := q.QueryRow(ctx, `SELECT default_consent_text, updated_at FROM platform_settings WHERE id = 1`).
		Scan(&ps.DefaultConsentText, &ps.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get platform settings: %w", err)
	}
	return &ps, nil
}

func (pgRepository) UpdatePlatform(ctx context.Context, defaultConsentText string, updatedBy uuid.UUID) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `UPDATE platform_settings SET default_consent_text = $1, updated_by = $2 WHERE id = 1`,
		defaultConsentText, updatedBy); err != nil {
		return fmt.Errorf("update platform settings: %w", err)
	}
	return nil
}

func (pgRepository) GetTenant(ctx context.Context, tenantID uuid.UUID) (*TenantSettings, error) {
	q, err := database.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var ts TenantSettings
	if err := q.QueryRow(ctx, `
		SELECT s.offer_timeout_seconds, s.max_queue_wait_seconds, s.consent_text_override, p.default_consent_text, s.updated_at
		FROM tenant_settings s CROSS JOIN platform_settings p
		WHERE s.tenant_id = $1 AND p.id = 1`, tenantID).
		Scan(&ts.OfferTimeoutSeconds, &ts.MaxQueueWaitSeconds, &ts.ConsentTextOverride, &ts.DefaultConsentText, &ts.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get tenant settings: %w", err)
	}
	ts.EffectiveConsentText = ts.DefaultConsentText
	if ts.ConsentTextOverride != nil {
		ts.EffectiveConsentText = *ts.ConsentTextOverride
	}
	return &ts, nil
}

func (pgRepository) UpdateTenant(ctx context.Context, tenantID uuid.UUID, in UpdateTenantInput) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `
		UPDATE tenant_settings
		SET offer_timeout_seconds = $2, max_queue_wait_seconds = $3, consent_text_override = $4
		WHERE tenant_id = $1`, tenantID, in.OfferTimeoutSeconds, in.MaxQueueWaitSeconds, in.ConsentTextOverride); err != nil {
		return fmt.Errorf("update tenant settings: %w", err)
	}
	return nil
}
