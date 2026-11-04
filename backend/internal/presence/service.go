package presence

import (
	"context"

	"github.com/google/uuid"

	"callcenter/internal/limits"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/realtime"
)

// Seats decides whether an agent holds one of the plan's agent seats (implemented by limits.Service).
type Seats interface {
	HoldsSeat(ctx context.Context, tenantID, userID uuid.UUID) (bool, error)
}

type Service struct {
	tx         database.TxManager
	repo       Repository
	dispatcher Dispatcher
	seats      Seats
}

func NewService(tx database.TxManager, repo Repository, dispatcher Dispatcher, seats Seats) *Service {
	return &Service{tx: tx, repo: repo, dispatcher: dispatcher, seats: seats}
}

// SetStatus records the signed-in agent's choice. Any change may free or remove an agent, so
// routing is asked to look again once it has committed.
func (s *Service) SetStatus(ctx context.Context, status Status) error {
	if !status.Valid() {
		return errStatus
	}
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		departments, err := s.repo.Departments(ctx, p.UserID)
		if err != nil {
			return err
		}
		if len(departments) == 0 {
			return errNotAgent
		}
		if status == StatusAvailable {
			holds, err := s.seats.HoldsSeat(ctx, p.TenantID, p.UserID)
			if err != nil {
				return err
			}
			if !holds {
				return limits.ErrSeatNotHeld
			}
		}
		if err := s.repo.SetStatus(ctx, p.TenantID, p.UserID, status); err != nil {
			return err
		}
		topics := []string{realtime.UserTopic(p.UserID)}
		for _, id := range departments {
			topics = append(topics, realtime.DepartmentTopic(id))
		}
		return realtime.Publish(ctx, topics...)
	})
	if err != nil {
		return err
	}
	s.dispatcher.Trigger(p.TenantID)
	return nil
}

// Heartbeat keeps the signed-in agent counted as present and returns their stored status, so a
// console that was marked offline while disconnected learns about it.
func (s *Service) Heartbeat(ctx context.Context) (Status, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return "", err
	}
	var status Status
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		status, err = s.repo.Heartbeat(ctx, p.TenantID, p.UserID)
		return err
	})
	return status, err
}
