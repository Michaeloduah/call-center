package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/media"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
)

// CallerAuth checks a caller token against a call, inside the caller's transaction (call.Service).
type CallerAuth interface {
	Authenticate(ctx context.Context, id uuid.UUID, callerToken string) (*call.CallerView, error)
}

type Service struct {
	tx      database.TxManager
	repo    Repository
	auth    CallerAuth
	media   media.Provider
	limiter *httpx.RateLimiter
	logger  *slog.Logger
}

func NewService(tx database.TxManager, repo Repository, auth CallerAuth, mp media.Provider, limiter *httpx.RateLimiter, logger *slog.Logger) *Service {
	return &Service{tx: tx, repo: repo, auth: auth, media: mp, limiter: limiter, logger: logger}
}

// NewLimiter bounds how fast one call can chat.
func NewLimiter() *httpx.RateLimiter { return httpx.NewRateLimiter(messagesPerMinute, time.Minute) }

// SendAsCaller saves and delivers a message from the caller of an active call.
func (s *Service) SendAsCaller(ctx context.Context, callID uuid.UUID, callerToken string, in Input) (*Message, error) {
	var msg *Message
	var room string
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		if _, err := s.auth.Authenticate(ctx, callID, callerToken); err != nil {
			return err
		}
		if !s.limiter.Allow(callID.String()) {
			return apperr.TooManyRequests()
		}
		var err error
		if msg, room, err = s.repo.InsertFromCaller(ctx, callID, in.Body); err != nil {
			return err
		}
		if msg == nil {
			return errUnavailable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.push(ctx, room, msg)
	return msg, nil
}

// SendAsAgent saves and delivers a message from the agent on the call.
func (s *Service) SendAsAgent(ctx context.Context, callID uuid.UUID, in Input) (*Message, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if !s.limiter.Allow(callID.String()) {
		return nil, apperr.TooManyRequests()
	}
	var msg *Message
	var room string
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		msg, room, err = s.repo.InsertFromAgent(ctx, callID, p.UserID, p.FullName, in.Body)
		return err
	})
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, errNoCall // not this agent's call, or it is no longer active
	}
	s.push(ctx, room, msg)
	return msg, nil
}

// CallerMessages returns the transcript to the caller of the call.
func (s *Service) CallerMessages(ctx context.Context, callID uuid.UUID, callerToken string) ([]Message, error) {
	var out []Message
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		if _, err := s.auth.Authenticate(ctx, callID, callerToken); err != nil {
			return err
		}
		var err error
		out, err = s.repo.List(ctx, callID)
		return err
	})
	return out, err
}

// AgentMessages returns the transcript to the agent who handled the call.
func (s *Service) AgentMessages(ctx context.Context, callID uuid.UUID) ([]Message, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var out []Message
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		ok, err := s.repo.IsAssignedAgent(ctx, callID, p.UserID)
		if err != nil {
			return err
		}
		if !ok {
			return errNoCall
		}
		out, err = s.repo.List(ctx, callID)
		return err
	})
	return out, err
}

// push delivers a saved message to the room. The message is already in the transcript, so a
// delivery failure is logged and the other side catches up from the history endpoint.
func (s *Service) push(ctx context.Context, room string, m *Message) {
	payload, err := json.Marshal(m)
	if err != nil {
		s.logger.Error("encode chat message", "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pushTimeout)
	defer cancel()
	if err := s.media.SendData(ctx, room, Topic, payload); err != nil {
		s.logger.Warn("deliver chat message", "room", room, "error", err)
	}
}
