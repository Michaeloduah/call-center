package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"callcenter/internal/platform/reqctx"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so the first one listed is the outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the real writer (flushing, per-write deadlines
// for long-lived event streams).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logging logs one line per request with a request id.
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" || len(requestID) > 64 {
				requestID = newRequestID()
			}
			w.Header().Set("X-Request-ID", requestID)
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.InfoContext(r.Context(), "http request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Recover turns panics into 500 responses instead of crashing the server.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.ErrorContext(r.Context(), "panic", "value", v, "stack", string(debug.Stack()))
				JSON(w, http.StatusInternalServerError, errorBody{Error: errorDetail{Code: "internal_error", Message: "Something went wrong. Please try again."}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

const maxUserAgentLen = 512

// ClientMeta records the client IP and user agent. X-Forwarded-For is honoured only when the
// direct peer is a trusted proxy (the Next.js server), otherwise clients could spoof their IP.
func ClientMeta(trusted []netip.Prefix) Middleware {
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := peerAddr(r.RemoteAddr)
			if ip.IsValid() && isTrusted(ip) {
				// Walk right-to-left and take the first address not belonging to a trusted proxy.
				parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
				for i := len(parts) - 1; i >= 0; i-- {
					a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
					if err != nil {
						break
					}
					ip = a.Unmap()
					if !isTrusted(ip) {
						break
					}
				}
			}
			ua := r.UserAgent()
			if fwd := r.Header.Get("X-Forwarded-User-Agent"); fwd != "" && isTrusted(peerAddr(r.RemoteAddr)) {
				ua = fwd
			}
			if len(ua) > maxUserAgentLen {
				ua = ua[:maxUserAgentLen]
			}
			ctx := reqctx.WithMeta(r.Context(), reqctx.Meta{IP: ip, UserAgent: ua})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func peerAddr(remoteAddr string) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

// SecurityHeaders sets conservative headers for a JSON API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
