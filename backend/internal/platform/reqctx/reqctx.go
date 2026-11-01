// Package reqctx carries per-request identity and metadata through context.Context.
package reqctx

import (
	"context"
	"net/netip"

	"github.com/google/uuid"

	"callcenter/internal/platform/apperr"
)

type Role string

const (
	RolePlatformAdmin Role = "platform_admin"
	RoleTenantAdmin   Role = "tenant_admin"
	RoleSupervisor    Role = "supervisor"
	RoleAgent         Role = "agent"
)

// TenantRoles are the roles a tenant user can hold (everything except platform admin).
var TenantRoles = []Role{RoleTenantAdmin, RoleSupervisor, RoleAgent}

func (r Role) IsTenantRole() bool {
	for _, t := range TenantRoles {
		if r == t {
			return true
		}
	}
	return false
}

// Principal is the authenticated user making the request.
type Principal struct {
	UserID    uuid.UUID
	TenantID  uuid.UUID // uuid.Nil for platform admins
	SessionID uuid.UUID
	Email     string
	FullName  string
	Role      Role
}

func (p Principal) HasRole(roles ...Role) bool {
	for _, r := range roles {
		if p.Role == r {
			return true
		}
	}
	return false
}

// Meta is request metadata recorded for auditing and rate limiting.
type Meta struct {
	IP        netip.Addr
	UserAgent string
}

type principalKey struct{}
type metaKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

func MetaFrom(ctx context.Context) Meta {
	m, _ := ctx.Value(metaKey{}).(Meta)
	return m
}

// TenantPrincipal returns the principal when it belongs to a tenant. Platform admins and
// anonymous requests are rejected, so tenant services can rely on TenantID being set.
func TenantPrincipal(ctx context.Context) (Principal, error) {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return Principal{}, apperr.Unauthenticated("unauthenticated", "Please sign in.")
	}
	if p.TenantID == uuid.Nil {
		return Principal{}, apperr.Forbidden("This action is only available to organisation users.")
	}
	return p, nil
}
