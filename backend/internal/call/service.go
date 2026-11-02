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

type Service struct {
	tx      database.TxManager
	repo    Repository
	widgets WidgetResolver
	media   media.Provider
	logger  *slog.Logger
	now     func() time.Time
}

func NewService(tx database.TxManager, repo Repository, widgets WidgetResolver, mp media.Provider, logger *slog.Logger) *Service {
	return &Service{tx: tx, repo: repo, widgets: widgets, media: mp, logger: logger, now: time.Now}
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

	err = s.tx.InPlatform(ctx, func(ctx context.Context) error {
		w, err := s.widgets.Resolve(ctx, publicKey)
		if err != nil {
			return err
		}
		if w == nil {
			return errWidgetNotFound
		}
		switch availability, err := widget.AvailabilityOf(w, s.now()); {
		case err != nil:
			return err
		case availability == widget.AvailabilityClosed:
			return errClosed
		case availability == widget.AvailabilityUnavailable:
			return errUnavailable
		}
		if err := in.ApplyForm(w.CallerForm); err != nil {
			return err
		}
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
		if view, err = s.authenticate(ctx, id, callerToken); err != nil {
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
	var room string
	err := s.tx.InPlatform(ctx, func(ctx context.Context) error {
		if _, err := s.authenticate(ctx, id, callerToken); err != nil {
			return err
		}
		var err error
		room, err = s.repo.End(ctx, id, EndCallerHungUp)
		return err
	})
	if err != nil || room == "" {
		return err
	}
	s.closeRoom(ctx, room)
	return nil
}

// HandleMediaEvent reacts to verified media-server webhooks. Handling is idempotent because
// every update only touches calls that are still live.
func (s *Service) HandleMediaEvent(ctx context.Context, ev media.Event) error {
	switch ev.Kind {
	case media.EventParticipantLeft:
		rawID, ok := strings.CutPrefix(ev.ParticipantIdentity, callerIdentityPrefix)
		if !ok {
			return nil // agents leaving are handled in M3
		}
		id, err := uuid.Parse(rawID)
		if err != nil {
			return fmt.Errorf("media event: malformed caller identity %q", ev.ParticipantIdentity)
		}
		var room string
		if err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
			room, err = s.repo.End(ctx, id, EndCallerLeft)
			return err
		}); err != nil {
			return err
		}
		if room != "" {
			s.closeRoom(ctx, room)
		}
		return nil
	case media.EventRoomFinished:
		return s.tx.InPlatform(ctx, func(ctx context.Context) error {
			return s.repo.EndByRoom(ctx, ev.Room, EndRoomClosed)
		})
	default:
		return nil
	}
}

func (s *Service) authenticate(ctx context.Context, id uuid.UUID, callerToken string) (*CallerView, error) {
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
