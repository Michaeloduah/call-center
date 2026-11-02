package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/platform/secret"
)

// Limiters bounds abuse of the public auth endpoints.
type Limiters struct {
	LoginByIP    *httpx.RateLimiter
	LoginByEmail *httpx.RateLimiter
	SignupByIP   *httpx.RateLimiter
	EmailByIP    *httpx.RateLimiter // resend-verification, forgot-password
	EmailByEmail *httpx.RateLimiter
	TokenByIP    *httpx.RateLimiter // verify, reset, invite lookup/accept
}

func NewLimiters() *Limiters {
	return &Limiters{
		LoginByIP:    httpx.NewRateLimiter(30, 15*time.Minute),
		LoginByEmail: httpx.NewRateLimiter(10, 15*time.Minute),
		SignupByIP:   httpx.NewRateLimiter(5, time.Hour),
		EmailByIP:    httpx.NewRateLimiter(10, 15*time.Minute),
		EmailByEmail: httpx.NewRateLimiter(3, time.Hour),
		TokenByIP:    httpx.NewRateLimiter(30, 15*time.Minute),
	}
}

func (l *Limiters) All() []*httpx.RateLimiter {
	return []*httpx.RateLimiter{l.LoginByIP, l.LoginByEmail, l.SignupByIP, l.EmailByIP, l.EmailByEmail, l.TokenByIP}
}

type Handler struct {
	svc      *Service
	limiters *Limiters
}

func NewHandler(svc *Service, limiters *Limiters) *Handler {
	return &Handler{svc: svc, limiters: limiters}
}

func ipKey(r *http.Request) string { return reqctx.MetaFrom(r.Context()).IP.String() }

func allow(limiters ...func() bool) error {
	for _, ok := range limiters {
		if !ok() {
			return apperr.TooManyRequests()
		}
	}
	return nil
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in LoginInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := allow(
		func() bool { return h.limiters.LoginByIP.Allow(ipKey(r)) },
		func() bool { return h.limiters.LoginByEmail.Allow(in.Email) },
	); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Login(r.Context(), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context()); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	me, err := h.svc.Me(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, me)
}

func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	if err := allow(func() bool { return h.limiters.SignupByIP.Allow(ipKey(r)) }); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in SignupInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Signup(r.Context(), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	h.withToken(w, r, func(in TokenInput) error { return h.svc.VerifyEmail(r.Context(), in.Token) })
}

func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	h.withEmail(w, r, h.svc.ResendVerification)
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	h.withEmail(w, r, h.svc.ForgotPassword)
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	h.withTokenPassword(w, r, h.svc.ResetPassword)
}

func (h *Handler) LookupInvite(w http.ResponseWriter, r *http.Request) {
	if err := allow(func() bool { return h.limiters.TokenByIP.Allow(ipKey(r)) }); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in TokenInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	info, err := h.svc.LookupInvite(r.Context(), in.Token)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, info)
}

func (h *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	h.withTokenPassword(w, r, h.svc.AcceptInvite)
}

func (h *Handler) withToken(w http.ResponseWriter, r *http.Request, fn func(TokenInput) error) {
	if err := allow(func() bool { return h.limiters.TokenByIP.Allow(ipKey(r)) }); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in TokenInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := fn(in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) withTokenPassword(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, token, password string) error) {
	if err := allow(func() bool { return h.limiters.TokenByIP.Allow(ipKey(r)) }); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in TokenPasswordInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := fn(r.Context(), in.Token, in.Password); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) withEmail(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, email string) error) {
	var in EmailInput
	if err := httpx.DecodeValid(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := allow(
		func() bool { return h.limiters.EmailByIP.Allow(ipKey(r)) },
		func() bool { return h.limiters.EmailByEmail.Allow(in.Email) },
	); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := fn(r.Context(), in.Email); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// Authenticate requires a valid "Authorization: Bearer <session token>" header.
func Authenticate(svc *Service) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" || len(token) > secret.MaxLen {
				httpx.Error(w, r, errInvalidSession)
				return
			}
			p, err := svc.Authenticate(r.Context(), token)
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(reqctx.WithPrincipal(r.Context(), p)))
		})
	}
}

// RequireRole allows only principals holding one of roles. Must run after Authenticate.
func RequireRole(roles ...reqctx.Role) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := reqctx.PrincipalFrom(r.Context())
			if !ok {
				httpx.Error(w, r, errInvalidSession)
				return
			}
			if !p.HasRole(roles...) {
				httpx.Error(w, r, apperr.Forbidden("You do not have permission to perform this action."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
