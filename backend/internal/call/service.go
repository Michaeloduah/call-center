package call

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/media"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/platform/secret"
	"callcenter/internal/widget"
)

// WidgetResolver looks a widget up by public key inside the caller's transaction (widget.Repository).
type WidgetResolver interface {
	Resolve(ctx context.Context, publicKey string) (*widget.Resolved, error)
}

// Limits tells whether the tenant's plan still allows new calls (implemented by limits.Service).
type Limits interface {
	CallsBlocked(ctx context.Context, tenantID uuid.UUID) (bool, error)
}

// UsageWatcher is told when a call has used up minutes so plan-usage warnings can go out (billing.Service).
type UsageWatcher interface {
	CheckUsage(tenantID uuid.UUID)
}

type Service struct {
	tx      database.TxManager
	repo    Repository
	widgets WidgetResolver
	media   media.Provider
	router  Router
	plan    Limits
	usage   UsageWatcher
	logger  *slog.Logger
	now     func() time.Time
}

func NewService(tx database.TxManager, repo Repository, widgets WidgetResolver, mp media.Provider, router Router, plan Limits, usage UsageWatcher, logger *slog.Logger) *Service {
	return &Service{tx: tx, repo: repo, widgets: widgets, media: mp, router: router, plan: plan, usage: usage, logger: logger, now: time.Now}
}

// Create queues a new call from a widget and returns the caller's credentials for the call room.
func (s *Service) Create(ctx context.Context, publicKey string, in CreateInput) (*CreateResult, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate call id: %w", err)
	}
	rawToken, tokenHash, err := secret.New()
	if err != nil {
		return nil, err
	}
	expiresAt := s.now().Add(callerTokenTTL)
	room := "call-" + id.String()

	var tenantID uuid.UUID
	err = s.tx.InPlatform(ctx, func(ctx context.Context) error {
		w, err := s.widgets.Resolve(ctx, publicKey)
		if err != nil {
			return err
		}
		if w == nil {
			return errWidgetNotFound
		}
		if w.CapReached, err = s.plan.CallsBlocked(ctx, w.TenantID); err != nil {
			return err
		}
		switch availability, err := widget.AvailabilityOf(w, s.now()); {
		case err != nil:
			return err
		case availability == widget.AvailabilityClosed:
			return errClosed
		case availability == widget.AvailabilityUnavailable:
			return errUnavailable
		case availability == widget.AvailabilityLimitReached:
			return errLimitReached
		case availability == widget.AvailabilityNoAgents:
			return errNoAgents
		}
		if err := in.ApplyForm(w.CallerForm); err != nil {
			return err
		}
		tenantID = w.TenantID
		if err := s.repo.LockTenant(ctx, w.TenantID); err != nil {
			return err
		}
		live, err := s.repo.CountLive(ctx, w.TenantID)
		if err != nil {
			return err
		}
		if live >= maxLiveCallsPerTenant {
			return errBusy
		}
		meta := reqctx.MetaFrom(ctx)
		var ip string
		if meta.IP.IsValid() {
			ip = meta.IP.String()
		}
		return s.repo.Insert(ctx, NewCall{
			ID: id, TenantID: w.TenantID, WidgetID: w.WidgetID, DepartmentID: w.DepartmentID, RoomName: room,
			CallerName: in.Name, CallerPhone: in.Phone, CallerEmail: in.Email,
			// The consent text is taken from the server-side configuration, never from the client.
			ConsentText:     w.ConsentText,
			CallerTokenHash: tokenHash, CallerTokenExpiresAt: expiresAt,
			CallerIP: ip, CallerUserAgent: meta.UserAgent,
		})
	})
	if err != nil {
		return nil, err
	}

	// The room is created after the call row commits, so a webhook for the room always finds its call.
	if err := s.media.CreateRoom(ctx, room); err != nil {
		s.fail(ctx, id, err)
		return nil, err
	}
	displayName := in.Name
	if displayName == "" {
		displayName = "Caller"
	}
	mediaToken, err := s.media.JoinToken(room, media.Participant{Identity: callerIdentityPrefix + id.String(), Name: displayName, Role: media.RoleCaller})
	if err != nil {
		s.fail(ctx, id, err)
		return nil, err
	}
	s.router.Trigger(tenantID)
	return &CreateResult{
		CallID: id, CallerToken: rawToken, ExpiresAt: expiresAt, Status: StatusQueued,
		MediaURL: s.media.PublicURL(), MediaToken: mediaToken,
	}, nil
}

// fail marks a call that could not be set up. The original error is what the client sees.
func (s *Service) fail(ctx context.Context, id uuid.UUID, cause error) {
	s.logger.Error("call setup failed", "call_id", id, "error", cause)
	if err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		return s.repo.MarkFailed(ctx, id, EndMediaUnavailable)
	}); err != nil {
		s.logger.Error("mark call failed", "call_id", id, "error", err)
	}
}

// Status returns the caller's view of their call, including queue position while queued.
func (s *Service) Status(ctx context.Context, id uuid.UUID, callerToken string) (*CallerView, error) {
	var view *CallerView
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		if view, err = s.Authenticate(ctx, id, callerToken); err != nil {
			return err
		}
		if view.Status == StatusQueued {
			pos, err := s.repo.QueuePosition(ctx, view)
			if err != nil {
				return err
			}
			view.QueuePosition = &pos
		}
		return nil
	})
	return view, err
}

// HangUp ends the caller's call and closes its room. Ending an already-ended call is a no-op.
func (s *Service) HangUp(ctx context.Context, id uuid.UUID, callerToken string) error {
	return s.finish(ctx, func(ctx context.Context) (*EndedCall, error) {
		if _, err := s.Authenticate(ctx, id, callerToken); err != nil {
			return nil, err
		}
		return s.repo.End(ctx, id, EndCallerHungUp)
	})
}

// AgentEnd lets the assigned agent end their active call.
func (s *Service) AgentEnd(ctx context.Context, id uuid.UUID) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	return s.finish(ctx, func(ctx context.Context) (*EndedCall, error) {
		ended, err := s.repo.EndAssigned(ctx, id, p.UserID, EndAgentEnded)
		if err == nil && ended == nil {
			return nil, errCallNotFound
		}
		return ended, err
	}, p.TenantID)
}

// HandleMediaEvent reacts to verified media-server webhooks. Handling is idempotent because
// every update only touches calls that are still live.
func (s *Service) HandleMediaEvent(ctx context.Context, ev media.Event) error {
	switch ev.Kind {
	case media.EventParticipantLeft:
		if rawID, ok := strings.CutPrefix(ev.ParticipantIdentity, callerIdentityPrefix); ok {
			id, err := uuid.Parse(rawID)
			if err != nil {
				return fmt.Errorf("media event: malformed caller identity %q", ev.ParticipantIdentity)
			}
			return s.finish(ctx, func(ctx context.Context) (*EndedCall, error) {
				return s.repo.End(ctx, id, EndCallerLeft)
			})
		}
		if strings.HasPrefix(ev.ParticipantIdentity, AgentIdentityPrefix) {
			return s.finish(ctx, func(ctx context.Context) (*EndedCall, error) {
				return s.repo.EndByRoom(ctx, ev.Room, EndAgentLeft)
			})
		}
		return nil
	case media.EventRoomFinished:
		return s.finish(ctx, func(ctx context.Context) (*EndedCall, error) {
			return s.repo.EndByRoom(ctx, ev.Room, EndRoomClosed)
		})
	default:
		return nil
	}
}

// finish ends a call in one transaction (the router frees the agent and cancels offers inside
// it), then closes the media room and lets routing look for the next piece of work. When the
// caller is a signed-in agent their tenant scopes the transaction; otherwise it is a platform one.
func (s *Service) finish(ctx context.Context, end func(ctx context.Context) (*EndedCall, error), tenant ...uuid.UUID) error {
	var ended *EndedCall
	run := func(ctx context.Context) (err error) {
		if ended, err = end(ctx); err != nil || ended == nil {
			return err
		}
		return s.router.CallEnded(ctx, *ended)
	}
	var err error
	if len(tenant) > 0 {
		err = s.tx.InTenant(ctx, tenant[0], run)
	} else {
		err = s.tx.InPlatform(ctx, run)
	}
	if err != nil || ended == nil {
		return err
	}
	s.closeRoom(ctx, ended.RoomName)
	s.router.Trigger(ended.TenantID)
	s.usage.CheckUsage(ended.TenantID)
	return nil
}

// Authenticate checks a caller token against a call. It runs inside the caller's transaction.
func (s *Service) Authenticate(ctx context.Context, id uuid.UUID, callerToken string) (*CallerView, error) {
	if callerToken == "" || len(callerToken) > secret.MaxLen {
		return nil, errInvalidCaller
	}
	view, err := s.repo.FindForCaller(ctx, id, secret.Hash(callerToken))
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, errInvalidCaller
	}
	return view, nil
}

// closeRoom disconnects everyone. Failure is logged: the call is already ended in the database
// and LiveKit closes empty rooms on its own.
func (s *Service) closeRoom(ctx context.Context, room string) {
	if err := s.media.DeleteRoom(ctx, room); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn("close call room", "room", room, "error", err)
	}
}
