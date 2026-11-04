package routing

import (
	"context"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/media"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/presence"
)

// Service is the agent-facing side of routing: what the console shows and what it can do.
type Service struct {
	tx         database.TxManager
	repo       Repository
	members    presence.Repository
	media      media.Provider
	dispatcher presence.Dispatcher
}

func NewService(tx database.TxManager, repo Repository, members presence.Repository, mp media.Provider, dispatcher presence.Dispatcher) *Service {
	return &Service{tx: tx, repo: repo, members: members, media: mp, dispatcher: dispatcher}
}

// Session returns the signed-in agent's current state.
func (s *Service) Session(ctx context.Context) (*Session, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var session *Session
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		if err := s.requireAgent(ctx, p.UserID); err != nil {
			return err
		}
		snap, err := s.repo.Snapshot(ctx, p.UserID)
		if err != nil {
			return err
		}
		session = &Session{
			Status:                   snap.Status,
			State:                    presence.Effective(snap.Status, snap.Fresh, snap.Offer != nil, snap.Call != nil),
			Offer:                    snap.Offer,
			Call:                     snap.Call,
			HeartbeatIntervalSeconds: int(presence.HeartbeatInterval.Seconds()),
		}
		return nil
	})
	return session, err
}

// Accept takes the offered call and returns what the agent needs to join its room.
func (s *Service) Accept(ctx context.Context, offerID uuid.UUID) (*AcceptResult, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var result AcceptResult
	var room string
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		accepted, err := s.repo.AcceptOffer(ctx, offerID, p.UserID)
		if err != nil {
			return err
		}
		if accepted == nil {
			return errOfferGone
		}
		active, err := s.repo.ActiveCall(ctx, p.UserID)
		if err != nil {
			return err
		}
		if active == nil {
			return errOfferGone
		}
		if room, err = s.repo.AssignedRoom(ctx, active.ID, p.UserID); err != nil {
			return err
		}
		result.Call = *active
		topics := newTopics()
		topics.affected(*accepted)
		return topics.publish(ctx)
	})
	if err != nil {
		return nil, err
	}
	grant, err := s.grant(room, p)
	if err != nil {
		return nil, err
	}
	result.MediaURL, result.MediaToken = grant.MediaURL, grant.MediaToken
	return &result, nil
}

// Decline hands the offered call back to the queue for someone else.
func (s *Service) Decline(ctx context.Context, offerID uuid.UUID) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		declined, err := s.repo.DeclineOffer(ctx, offerID, p.UserID)
		if err != nil {
			return err
		}
		if declined == nil {
			return errOfferGone
		}
		topics := newTopics()
		topics.affected(*declined)
		return topics.publish(ctx)
	})
	if err != nil {
		return err
	}
	s.dispatcher.Trigger(p.TenantID)
	return nil
}

// MediaToken re-issues a join token for the call the agent is on, for example after a reload.
func (s *Service) MediaToken(ctx context.Context, callID uuid.UUID) (*MediaGrant, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var room string
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		room, err = s.repo.AssignedRoom(ctx, callID, p.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if room == "" {
		return nil, errNoCall
	}
	return s.grant(room, p)
}

func (s *Service) grant(room string, p reqctx.Principal) (*MediaGrant, error) {
	token, err := s.media.JoinToken(room, media.Participant{
		Identity: call.AgentIdentityPrefix + p.UserID.String(), Name: p.FullName, Role: media.RoleAgent,
	})
	if err != nil {
		return nil, err
	}
	return &MediaGrant{MediaURL: s.media.PublicURL(), MediaToken: token}, nil
}

func (s *Service) requireAgent(ctx context.Context, userID uuid.UUID) error {
	departments, err := s.members.Departments(ctx, userID)
	if err != nil {
		return err
	}
	if len(departments) == 0 {
		return errNotAgent
	}
	return nil
}
