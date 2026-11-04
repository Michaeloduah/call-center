package routing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/call"
	"callcenter/internal/media"
	"callcenter/internal/platform/database"
	"callcenter/internal/presence"
	"callcenter/internal/realtime"
)

// Router decides which agent is offered which call. It implements call.Router.
//
// Every pass for a tenant runs in one transaction under the tenant lock, so passes never overlap,
// across goroutines or API instances. Passes are idempotent, which is why running them from
// triggers (low latency) and from a sweeper (self-healing) at the same time is safe.
type Router struct {
	tx      database.TxManager
	repo    Repository
	members presence.Repository
	media   media.Provider
	logger  *slog.Logger
	trigger chan uuid.UUID
}

func NewRouter(tx database.TxManager, repo Repository, members presence.Repository, mp media.Provider, logger *slog.Logger) *Router {
	return &Router{tx: tx, repo: repo, members: members, media: mp, logger: logger, trigger: make(chan uuid.UUID, triggerBuffer)}
}

// Trigger asks for a dispatch pass for the tenant. It never blocks.
func (r *Router) Trigger(tenantID uuid.UUID) {
	select {
	case r.trigger <- tenantID:
	default: // the sweeper will pick the tenant up within SweepInterval
	}
}

// CallEnded runs inside the transaction that ended a call (see call.Router).
func (r *Router) CallEnded(ctx context.Context, e call.EndedCall) error {
	cancelled, err := r.repo.CancelCallOffers(ctx, e.ID)
	if err != nil {
		return err
	}
	topics := newTopics()
	topics.department(e.DepartmentID)
	for _, id := range cancelled {
		topics.user(id)
	}
	if e.AgentID != nil {
		if err := r.repo.FreeAgent(ctx, *e.AgentID, e.Reason == call.EndAgentLeft); err != nil {
			return err
		}
		topics.user(*e.AgentID)
	}
	return topics.publish(ctx)
}

// Run processes triggers and sweeps until ctx ends.
func (r *Router) Run(ctx context.Context) {
	ticker := time.NewTicker(SweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-r.trigger:
			r.dispatchLogged(ctx, id)
		case <-ticker.C:
			r.sweep(ctx)
		}
	}
}

func (r *Router) sweep(ctx context.Context) {
	var tenants []uuid.UUID
	if err := r.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		tenants, err = r.repo.TenantsWithWork(ctx)
		return err
	}); err != nil {
		if ctx.Err() == nil {
			r.logger.Error("routing sweep", "error", err)
		}
		return
	}
	for _, id := range tenants {
		r.dispatchLogged(ctx, id)
	}
}

func (r *Router) dispatchLogged(ctx context.Context, tenantID uuid.UUID) {
	if err := r.Dispatch(ctx, tenantID); err != nil && !errors.Is(err, context.Canceled) {
		r.logger.Error("routing dispatch", "tenant_id", tenantID, "error", err)
	}
}

// Dispatch runs one pass for a tenant: it retires what is no longer valid, then offers each
// waiting call (oldest first) to the longest-idle available agent of its department.
func (r *Router) Dispatch(ctx context.Context, tenantID uuid.UUID) error {
	var missed []call.EndedCall
	err := r.tx.InPlatform(ctx, func(ctx context.Context) error {
		if err := r.repo.LockTenant(ctx, tenantID); err != nil {
			return err
		}
		timings, err := r.repo.Timings(ctx, tenantID)
		if err != nil {
			return err
		}
		topics := newTopics()

		stale, err := r.repo.MarkStaleOffline(ctx, tenantID)
		if err != nil {
			return err
		}
		for _, id := range stale {
			if err := r.agentTopics(ctx, topics, id); err != nil {
				return err
			}
		}
		for _, step := range []func(context.Context, uuid.UUID) ([]Affected, error){
			r.repo.CancelUnavailableOffers, r.repo.ExpireOffers, r.repo.RequeueOrphans,
		} {
			affected, err := step(ctx, tenantID)
			if err != nil {
				return err
			}
			topics.affected(affected...)
		}

		if missed, err = r.repo.MissOverdue(ctx, tenantID, timings.MaxQueueWait); err != nil {
			return err
		}
		for _, m := range missed {
			topics.department(m.DepartmentID)
			if m.AgentID != nil {
				topics.user(*m.AgentID)
			}
		}

		queued, err := r.repo.QueuedCalls(ctx, tenantID)
		if err != nil {
			return err
		}
		for _, c := range queued {
			agent, ok, err := r.repo.PickAgent(ctx, tenantID, c.ID, c.DepartmentID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := r.repo.CreateOffer(ctx, tenantID, c.ID, agent, timings.OfferTimeout); err != nil {
				return err
			}
			topics.user(agent)
			topics.department(c.DepartmentID)
		}
		return topics.publish(ctx)
	})
	if err != nil {
		return fmt.Errorf("dispatch tenant %s: %w", tenantID, err)
	}
	for _, m := range missed {
		if err := r.media.DeleteRoom(ctx, m.RoomName); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.Warn("close room of missed call", "room", m.RoomName, "error", err)
		}
	}
	return nil
}

// agentTopics adds an agent's console and every department they serve.
func (r *Router) agentTopics(ctx context.Context, t *topicSet, agentID uuid.UUID) error {
	t.user(agentID)
	departments, err := r.members.Departments(ctx, agentID)
	if err != nil {
		return err
	}
	for _, id := range departments {
		t.department(id)
	}
	return nil
}

// topicSet collects the hints to publish when a transaction commits, without duplicates.
type topicSet struct{ set map[string]struct{} }

func newTopics() *topicSet { return &topicSet{set: map[string]struct{}{}} }

func (t *topicSet) user(id uuid.UUID)       { t.set[realtime.UserTopic(id)] = struct{}{} }
func (t *topicSet) department(id uuid.UUID) { t.set[realtime.DepartmentTopic(id)] = struct{}{} }

func (t *topicSet) affected(items ...Affected) {
	for _, a := range items {
		t.department(a.DepartmentID)
		if a.AgentID != nil {
			t.user(*a.AgentID)
		}
	}
}

func (t *topicSet) publish(ctx context.Context) error {
	topics := make([]string, 0, len(t.set))
	for topic := range t.set {
		topics = append(topics, topic)
	}
	return realtime.Publish(ctx, topics...)
}
