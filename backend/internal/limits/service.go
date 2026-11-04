package limits

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

// Service answers "may this tenant do that?". Every method runs inside the caller's scoped
// transaction (a repository call without one fails with database.ErrNoTx).
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// CheckCreate refuses to create one more department or widget beyond the plan.
func (s *Service) CheckCreate(ctx context.Context, tenantID uuid.UUID, kind Kind) error {
	cur, err := s.repo.Current(ctx, tenantID)
	if err != nil {
		return err
	}
	var max *int
	switch kind {
	case KindDepartments:
		max = cur.Caps.MaxDepartments
	case KindWidgets:
		max = cur.Caps.MaxWidgets
	}
	if max == nil {
		return nil
	}
	usage, err := s.repo.Usage(ctx, tenantID, cur.PeriodStart, cur.PeriodEnd)
	if err != nil {
		return err
	}
	have := usage.Departments
	if kind == KindWidgets {
		have = usage.Widgets
	}
	if have >= *max {
		return planLimit(kind, *max)
	}
	return nil
}

// CheckSeatsForDepartment refuses a membership change that would raise the number of agents
// above the plan. A change that does not add seats is always allowed, even for a tenant that is
// over its limit after a downgrade, so the admin can fix it.
func (s *Service) CheckSeatsForDepartment(ctx context.Context, tenantID, departmentID uuid.UUID, userIDs []uuid.UUID) error {
	cur, err := s.repo.Current(ctx, tenantID)
	if err != nil {
		return err
	}
	if cur.Caps.MaxAgents == nil {
		return nil
	}
	after, err := s.repo.SeatsAfterReplacing(ctx, tenantID, departmentID, userIDs)
	if err != nil {
		return err
	}
	usage, err := s.repo.Usage(ctx, tenantID, cur.PeriodStart, cur.PeriodEnd)
	if err != nil {
		return err
	}
	if after > *cur.Caps.MaxAgents && after > usage.Agents {
		return errSeatLimit(*cur.Caps.MaxAgents)
	}
	return nil
}

// CheckEnableUser refuses to re-enable a disabled agent when that would exceed the seat limit.
func (s *Service) CheckEnableUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	cur, err := s.repo.Current(ctx, tenantID)
	if err != nil {
		return err
	}
	if cur.Caps.MaxAgents == nil {
		return nil
	}
	agent, err := s.repo.IsAgent(ctx, tenantID, userID)
	if err != nil || !agent {
		return err
	}
	usage, err := s.repo.Usage(ctx, tenantID, cur.PeriodStart, cur.PeriodEnd)
	if err != nil {
		return err
	}
	if usage.Agents+1 > *cur.Caps.MaxAgents {
		return errSeatLimit(*cur.Caps.MaxAgents)
	}
	return nil
}

// HoldsSeat reports whether the agent may take calls. When a tenant has more agents than seats
// (after a downgrade) the earliest memberships keep their seats.
func (s *Service) HoldsSeat(ctx context.Context, tenantID, userID uuid.UUID) (bool, error) {
	cur, err := s.repo.Current(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if cur.Caps.MaxAgents == nil {
		return true, nil
	}
	holders, err := s.repo.SeatHolders(ctx, tenantID, *cur.Caps.MaxAgents)
	if err != nil {
		return false, err
	}
	return slices.Contains(holders, userID), nil
}

// CallsBlocked reports whether a cap with the "block" policy has been reached, so new calls must
// not start (callers see the callback form instead). Calls already in progress are never cut off.
func (s *Service) CallsBlocked(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	cur, err := s.repo.Current(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if !blocksMinutes(cur) && !blocksStorage(cur) {
		return false, nil
	}
	usage, err := s.repo.Usage(ctx, tenantID, cur.PeriodStart, cur.PeriodEnd)
	if err != nil {
		return false, err
	}
	return (blocksMinutes(cur) && usage.CallSeconds >= int64(*cur.Caps.MaxCallMinutes)*SecondsPerMinute) ||
		(blocksStorage(cur) && usage.StorageBytes >= *cur.Caps.MaxStorageBytes), nil
}

func blocksMinutes(c *Current) bool {
	return c.Caps.MaxCallMinutes != nil && c.Policies.Minutes == PolicyBlock
}
func blocksStorage(c *Current) bool {
	return c.Caps.MaxStorageBytes != nil && c.Policies.Storage == PolicyBlock
}

// Snapshot returns limits, policies and usage for the billing page and the usage notices.
func (s *Service) Snapshot(ctx context.Context, tenantID uuid.UUID) (*Snapshot, error) {
	cur, err := s.repo.Current(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	usage, err := s.repo.Usage(ctx, tenantID, cur.PeriodStart, cur.PeriodEnd)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{Current: *cur, Usage: usage, AgentsWithout: []Person{}}
	if cur.Caps.MaxAgents != nil && usage.Agents > *cur.Caps.MaxAgents {
		if snap.AgentsWithout, err = s.repo.AgentsBeyond(ctx, tenantID, *cur.Caps.MaxAgents); err != nil {
			return nil, err
		}
	}
	return snap, nil
}
