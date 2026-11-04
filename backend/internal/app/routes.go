package app

import (
	"context"
	"net/http"
	"time"

	"callcenter/internal/audit"
	"callcenter/internal/auth"
	"callcenter/internal/board"
	"callcenter/internal/call"
	"callcenter/internal/callback"
	"callcenter/internal/chat"
	"callcenter/internal/department"
	"callcenter/internal/history"
	"callcenter/internal/media"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/presence"
	"callcenter/internal/recording"
	"callcenter/internal/routing"
	"callcenter/internal/settings"
	"callcenter/internal/tenant"
	"callcenter/internal/user"
	"callcenter/internal/widget"
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
	// Every tenant role can be an agent (membership in a department decides), so these only
	// require a tenant user; the services check the membership and scope.
	tenantUser := func(h http.HandlerFunc) http.Handler {
		return guard(h, reqctx.RoleTenantAdmin, reqctx.RoleSupervisor, reqctx.RoleAgent)
	}
	supervisor := func(h http.HandlerFunc) http.Handler { return guard(h, reqctx.RoleTenantAdmin, reqctx.RoleSupervisor) }
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

	wh := widget.NewHandler(a.Widgets)
	mux.Handle("GET /v1/admin/widgets", tenantAdmin(wh.List))
	mux.Handle("POST /v1/admin/widgets", tenantAdmin(wh.Create))
	mux.Handle("GET /v1/admin/widgets/{id}", tenantAdmin(wh.Get))
	mux.Handle("PUT /v1/admin/widgets/{id}", tenantAdmin(wh.Update))
	mux.Handle("DELETE /v1/admin/widgets/{id}", tenantAdmin(wh.Delete))

	// Public caller endpoints (no user session; calls are authorised by their caller token).
	ch := call.NewHandler(a.Calls, media.Listeners{a.Calls, a.Recordings}, a.Media, a.CallLimits, a.Logger)
	mux.HandleFunc("GET /v1/public/widgets/{key}", wh.Public)
	mux.HandleFunc("POST /v1/public/widgets/{key}/calls", ch.Create)
	mux.HandleFunc("GET /v1/public/calls/{id}", ch.Status)
	mux.HandleFunc("POST /v1/public/calls/{id}/hangup", ch.HangUp)
	mux.HandleFunc("POST /v1/webhooks/livekit", ch.MediaWebhook)

	// Agent console
	ph := presence.NewHandler(a.Presence)
	rh := routing.NewHandler(a.Routing, a.Hub)
	mux.Handle("GET /v1/agent/session", tenantUser(rh.Session))
	mux.Handle("GET /v1/agent/events", tenantUser(rh.Events))
	mux.Handle("PUT /v1/agent/presence", tenantUser(ph.SetStatus))
	mux.Handle("POST /v1/agent/presence/heartbeat", tenantUser(ph.Heartbeat))
	mux.Handle("POST /v1/agent/offers/{id}/accept", tenantUser(rh.Accept))
	mux.Handle("POST /v1/agent/offers/{id}/decline", tenantUser(rh.Decline))
	mux.Handle("GET /v1/agent/calls/{id}/media", tenantUser(rh.Media))
	mux.Handle("POST /v1/agent/calls/{id}/end", tenantUser(ch.AgentEnd))

	// Chat
	chh := chat.NewHandler(a.Chat)
	mux.HandleFunc("POST /v1/public/calls/{id}/messages", chh.CallerSend)
	mux.HandleFunc("GET /v1/public/calls/{id}/messages", chh.CallerList)
	mux.Handle("POST /v1/agent/calls/{id}/messages", tenantUser(chh.AgentSend))
	mux.Handle("GET /v1/agent/calls/{id}/messages", tenantUser(chh.AgentList))

	// Callbacks
	cbh := callback.NewHandler(a.Callbacks, a.CallbackLimits)
	mux.HandleFunc("POST /v1/public/widgets/{key}/callbacks", cbh.CreateForWidget)
	mux.HandleFunc("POST /v1/public/calls/{id}/callback", cbh.CreateForCall)
	mux.Handle("GET /v1/callbacks", tenantUser(cbh.List))
	mux.Handle("GET /v1/callbacks/{id}", tenantUser(cbh.Get))
	mux.Handle("POST /v1/callbacks/{id}/claim", tenantUser(cbh.Claim))
	mux.Handle("POST /v1/callbacks/{id}/release", tenantUser(cbh.Release))
	mux.Handle("POST /v1/callbacks/{id}/complete", tenantUser(cbh.Complete))
	mux.Handle("POST /v1/callbacks/{id}/cancel", tenantUser(cbh.Cancel))
	mux.Handle("POST /v1/admin/callbacks/{id}/assign", tenantAdmin(cbh.Assign))

	// Call history
	hh := history.NewHandler(a.History)
	mux.Handle("GET /v1/calls", tenantUser(hh.List))
	mux.Handle("GET /v1/calls/{id}", tenantUser(hh.Get))

	// Recordings
	rech := recording.NewHandler(a.Recordings)
	mux.Handle("POST /v1/calls/{id}/recording/access", supervisor(rech.Access))
	mux.Handle("DELETE /v1/calls/{id}/recording", tenantAdmin(rech.Delete))
	mux.Handle("GET /v1/platform/recordings/deleted", platformOnly(rech.ListDeleted))
	mux.Handle("POST /v1/platform/recordings/{id}/access", platformOnly(rech.PlatformAccess))
	mux.Handle("POST /v1/platform/recordings/{id}/restore", platformOnly(rech.Restore))

	// Supervisor live board
	bh := board.NewHandler(a.Board, a.Hub)
	mux.Handle("GET /v1/live/board", supervisor(bh.Board))
	mux.Handle("GET /v1/live/events", supervisor(bh.Events))

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
