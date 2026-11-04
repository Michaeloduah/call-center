// Package audit records and lists security-relevant actions.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
)

// Actions recorded across the application.
const (
	ActionTenantSignedUp       = "tenant.signed_up"
	ActionTenantCreated        = "tenant.created"
	ActionTenantUpdated        = "tenant.updated"
	ActionTenantSuspended      = "tenant.suspended"
	ActionTenantActivated      = "tenant.activated"
	ActionTenantDeleted        = "tenant.deleted"
	ActionTenantSettingsUpdate = "tenant_settings.updated"
	ActionPlatformSettings     = "platform_settings.updated"
	ActionPlatformAdminCreated = "platform_admin.created"
	ActionUserInvited          = "user.invited"
	ActionUserUpdated          = "user.updated"
	ActionUserRoleChanged      = "user.role_changed"
	ActionUserDisabled         = "user.disabled"
	ActionUserEnabled          = "user.enabled"
	ActionUserDeleted          = "user.deleted"
	ActionUserInviteResent     = "user.invite_resent"
	ActionDepartmentCreated    = "department.created"
	ActionDepartmentUpdated    = "department.updated"
	ActionDepartmentDeleted    = "department.deleted"
	ActionDepartmentMembers    = "department.members_updated"
	ActionDepartmentSupervisor = "department.supervisors_updated"
	ActionDepartmentHours      = "department.hours_updated"
	ActionWidgetCreated        = "widget.created"
	ActionWidgetUpdated        = "widget.updated"
	ActionWidgetDeleted        = "widget.deleted"
	ActionRecordingPlayed      = "recording.played"
	ActionRecordingDownloaded  = "recording.downloaded"
	ActionRecordingDeleted     = "recording.deleted"
	ActionRecordingRestored    = "recording.restored"
	ActionPlanCreated          = "plan.created"
	ActionPlanUpdated          = "plan.updated"
	ActionPlanDeleted          = "plan.deleted"
	ActionPriceCreated         = "plan_price.created"
	ActionPriceUpdated         = "plan_price.updated"
	ActionPriceDeleted         = "plan_price.deleted"
	ActionPriceSynced          = "plan_price.synced"
	ActionSubscriptionActive   = "subscription.activated"
	ActionSubscriptionFallback = "subscription.fell_back_to_free"
	ActionCheckoutStarted      = "billing.checkout_started"
	ActionSubscriptionCancel   = "billing.cancel_requested"
	ActionCapPoliciesUpdated   = "billing.cap_policies_updated"
)

// Entry describes one action. TenantID is uuid.Nil for platform-level actions.
type Entry struct {
	TenantID   uuid.UUID
	Action     string
	EntityType string
	EntityID   uuid.UUID
	Metadata   map[string]any
	// ActorEmail is used when there is no authenticated principal (e.g. self sign-up).
	ActorEmail string
}

// Recorder writes entries inside the caller's scoped transaction, so the audit
// row commits or rolls back together with the action it describes.
type Recorder struct{}

func NewRecorder() *Recorder { return &Recorder{} }

func (Recorder) Record(ctx context.Context, e Entry) error {
	q, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	var actorID *uuid.UUID
	actorEmail := e.ActorEmail
	if p, ok := reqctx.PrincipalFrom(ctx); ok {
		actorID = &p.UserID
		actorEmail = p.Email
	}
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	var ip *string
	if m := reqctx.MetaFrom(ctx); m.IP.IsValid() {
		s := m.IP.String()
		ip = &s
	}
	_, err = q.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, actor_user_id, actor_email, action, entity_type, entity_id, metadata, ip)
		VALUES ($1, $2, nullif($3, ''), $4, $5, $6, $7, $8::inet)`,
		nullableUUID(e.TenantID), actorID, actorEmail, e.Action, e.EntityType, nullableUUID(e.EntityID), metaJSON, ip)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func nullableUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// LogEntry is a stored audit entry as returned to platform admins.
type LogEntry struct {
	ID         uuid.UUID      `json:"id"`
	TenantID   *uuid.UUID     `json:"tenant_id"`
	TenantName *string        `json:"tenant_name"`
	ActorID    *uuid.UUID     `json:"actor_user_id"`
	ActorEmail *string        `json:"actor_email"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   *uuid.UUID     `json:"entity_id"`
	Metadata   map[string]any `json:"metadata"`
	IP         *string        `json:"ip"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Filter narrows the platform audit log listing.
type Filter struct {
	TenantID uuid.UUID // uuid.Nil = all
	Action   string    // exact match, empty = all
}
