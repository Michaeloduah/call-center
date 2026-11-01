package app

import (
	"context"
	"net/http"
	"time"

	"callcenter/internal/audit"
	"callcenter/internal/auth"
	"callcenter/internal/department"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/settings"
	"callcenter/internal/tenant"
	"callcenter/internal/user"
)

// Handler builds the HTTP API. Authentication uses a bearer session token sent by the
// Next.js server; browsers never call this API with cookies, so CSRF does not apply here.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	authenticated := auth.Authenticate(a.Auth)
	guard := func(h http.HandlerFunc, roles ...reqctx.Role) http.Handler {
		return httpx.Chain(h, authenticated, auth.RequireRole(roles...))
	}
	platformOnly := func(h http.HandlerFunc) http.Handler { return guard(h, reqctx.RolePlatformAdmin) }
	tenantAdmin := func(h http.HandlerFunc) http.Handler { return guard(h, reqctx.RoleTenantAdmin) }
	anyUser := func(h http.HandlerFunc) http.Handler {
		return guard(h, reqctx.RolePlatformAdmin, reqctx.RoleTenantAdmin, reqctx.RoleSupervisor, reqctx.RoleAgent)
	}

	mux.HandleFunc("GET /healthz", a.health)

	// Auth
	ah := auth.NewHandler(a.Auth, a.AuthLimits)
	mux.HandleFunc("POST /v1/auth/login", ah.Login)
	mux.HandleFunc("POST /v1/auth/signup", ah.Signup)
	mux.HandleFunc("POST /v1/auth/verify-email", ah.VerifyEmail)
	mux.HandleFunc("POST /v1/auth/resend-verification", ah.ResendVerification)
	mux.HandleFunc("POST /v1/auth/forgot-password", ah.ForgotPassword)
	mux.HandleFunc("POST /v1/auth/reset-password", ah.ResetPassword)
	mux.HandleFunc("POST /v1/auth/invites/lookup", ah.LookupInvite)
	mux.HandleFunc("POST /v1/auth/invites/accept", ah.AcceptInvite)
	mux.Handle("GET /v1/auth/me", anyUser(ah.Me))
	mux.Handle("POST /v1/auth/logout", anyUser(ah.Logout))

	// Platform admin
	th := tenant.NewHandler(a.Tenants)
	mux.Handle("GET /v1/platform/tenants", platformOnly(th.List))
	mux.Handle("POST /v1/platform/tenants", platformOnly(th.Create))
	mux.Handle("GET /v1/platform/tenants/{id}", platformOnly(th.Get))
	mux.Handle("PUT /v1/platform/tenants/{id}", platformOnly(th.Update))
	mux.Handle("DELETE /v1/platform/tenants/{id}", platformOnly(th.Delete))
	mux.Handle("POST /v1/platform/tenants/{id}/suspend", platformOnly(th.Suspend))
	mux.Handle("POST /v1/platform/tenants/{id}/activate", platformOnly(th.Activate))

	sh := settings.NewHandler(a.Settings)
	mux.Handle("GET /v1/platform/settings", platformOnly(sh.GetPlatform))
	mux.Handle("PUT /v1/platform/settings", platformOnly(sh.UpdatePlatform))
	mux.Handle("GET /v1/platform/audit-logs", platformOnly(audit.NewHandler(a.AuditQuery).List))

	// Tenant admin
	uh := user.NewHandler(a.Users)
	mux.Handle("GET /v1/admin/users", tenantAdmin(uh.List))
	mux.Handle("POST /v1/admin/users", tenantAdmin(uh.Create))
	mux.Handle("GET /v1/admin/users/{id}", tenantAdmin(uh.Get))
	mux.Handle("PUT /v1/admin/users/{id}", tenantAdmin(uh.Update))
	mux.Handle("DELETE /v1/admin/users/{id}", tenantAdmin(uh.Delete))
	mux.Handle("POST /v1/admin/users/{id}/disable", tenantAdmin(uh.Disable))
	mux.Handle("POST /v1/admin/users/{id}/enable", tenantAdmin(uh.Enable))
	mux.Handle("POST /v1/admin/users/{id}/resend-invite", tenantAdmin(uh.ResendInvite))

	dh := department.NewHandler(a.Departments)
	mux.Handle("GET /v1/admin/departments", tenantAdmin(dh.List))
	mux.Handle("POST /v1/admin/departments", tenantAdmin(dh.Create))
	mux.Handle("GET /v1/admin/departments/{id}", tenantAdmin(dh.Get))
	mux.Handle("PUT /v1/admin/departments/{id}", tenantAdmin(dh.Update))
	mux.Handle("DELETE /v1/admin/departments/{id}", tenantAdmin(dh.Delete))
	mux.Handle("PUT /v1/admin/departments/{id}/members", tenantAdmin(dh.SetMembers))
	mux.Handle("PUT /v1/admin/departments/{id}/supervisors", tenantAdmin(dh.SetSupervisors))
	mux.Handle("PUT /v1/admin/departments/{id}/business-hours", tenantAdmin(dh.SetBusinessHours))

	mux.Handle("GET /v1/admin/settings", tenantAdmin(sh.GetTenant))
	mux.Handle("PUT /v1/admin/settings", tenantAdmin(sh.UpdateTenant))

	return httpx.Chain(mux,
		httpx.Recover,
		httpx.Logging(a.Logger),
		httpx.SecurityHeaders,
		httpx.ClientMeta(a.Config.TrustedProxies),
	)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.DB.Ping(ctx); err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
