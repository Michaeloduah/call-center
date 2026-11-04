package recording

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"callcenter/internal/audit"
	"callcenter/internal/media"
	"callcenter/internal/platform/apperr"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
	"callcenter/internal/storage"
)

// AuditRecorder writes audit entries inside the caller's transaction.
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

type Service struct {
	tx     database.TxManager
	repo   Repository
	media  media.Provider
	store  storage.Store
	audit  AuditRecorder
	logger *slog.Logger
	now    func() time.Time
}

func NewService(tx database.TxManager, repo Repository, mp media.Provider, store storage.Store, recorder AuditRecorder, logger *slog.Logger) *Service {
	return &Service{tx: tx, repo: repo, media: mp, store: store, audit: recorder, logger: logger, now: time.Now}
}

// objectKey is where a call's recording lives in the bucket.
func objectKey(tenantID, callID uuid.UUID) string {
	return fmt.Sprintf("tenants/%s/recordings/%s.ogg", tenantID, callID)
}

// Start begins recording a call that has just been answered. It returns immediately: the
// recorder needs a few seconds to start and the agent must not wait for it. A recording that
// cannot start is marked failed; the call itself is never affected (v0.4 5.3).
func (s *Service) Start(tenantID, callID uuid.UUID, room string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
		defer cancel()
		if err := s.start(ctx, tenantID, callID, room); err != nil {
			s.logger.Error("start recording", "call_id", callID, "error", err)
		}
	}()
}

func (s *Service) start(ctx context.Context, tenantID, callID uuid.UUID, room string) error {
	var id uuid.UUID
	var created bool
	if err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		id, created, err = s.repo.Insert(ctx, tenantID, callID)
		return err
	}); err != nil {
		return err
	}
	if !created {
		return nil // already recording
	}

	key := objectKey(tenantID, callID)
	t := s.store.Target()
	egressID, startErr := s.media.StartRecording(ctx, room, media.RecordingTarget{
		Key: key, Endpoint: t.Endpoint, Bucket: t.Bucket, AccessKey: t.AccessKey, Secret: t.Secret, Region: t.Region,
	})
	// The row is updated with a context that outlives a timeout of the start request itself.
	ctx = context.WithoutCancel(ctx)
	if startErr != nil {
		_ = s.tx.InPlatform(ctx, func(ctx context.Context) error {
			return s.repo.Fail(ctx, id, "The recorder could not be started.")
		})
		return startErr
	}
	return s.tx.InPlatform(ctx, func(ctx context.Context) error { return s.repo.Attach(ctx, id, egressID, key) })
}

// HandleMediaEvent records the recorder's final report. It is idempotent.
func (s *Service) HandleMediaEvent(ctx context.Context, ev media.Event) error {
	if ev.Kind != media.EventEgressEnded || ev.Recording == nil {
		return nil
	}
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		found, err := s.repo.Finish(ctx, ev.EgressID, *ev.Recording)
		if err != nil {
			return err
		}
		if !found {
			s.logger.Warn("egress ended for an unknown recording", "egress_id", ev.EgressID, "room", ev.Room)
		}
		return nil
	})
}

// Run marks recordings that never completed as failed, until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			var n int
			if err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
				n, err = s.repo.FailStale(ctx)
				return err
			}); err != nil {
				if ctx.Err() == nil {
					s.logger.Error("recording sweep", "error", err)
				}
				continue
			}
			if n > 0 {
				s.logger.Warn("recordings marked failed after no result", "count", n)
			}
		}
	}
}

// Access issues a short-lived link to play or download a call's recording. Tenant admins may
// reach any recording of their tenant, supervisors those of the departments they supervise.
// Every issued link is audit-logged.
func (s *Service) Access(ctx context.Context, callID uuid.UUID, download bool) (*AccessGrant, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if !p.HasRole(reqctx.RoleTenantAdmin, reqctx.RoleSupervisor) {
		return nil, apperr.Forbidden("You do not have permission to play recordings.")
	}
	var loc *Located
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		if loc, err = s.repo.FindVisible(ctx, callID, Scope{All: p.Role == reqctx.RoleTenantAdmin, UserID: p.UserID}); err != nil {
			return err
		}
		if loc == nil {
			return errNotFound
		}
		return s.authorise(ctx, loc, download, false)
	})
	if err != nil {
		return nil, err
	}
	return s.grant(ctx, loc, download)
}

// authorise checks the recording can be played and writes the audit entry in the open transaction.
func (s *Service) authorise(ctx context.Context, loc *Located, download, byPlatform bool) error {
	if loc.Status != StatusReady || loc.Key == nil {
		return errUnavailable
	}
	action := audit.ActionRecordingPlayed
	if download {
		action = audit.ActionRecordingDownloaded
	}
	return s.audit.Record(ctx, audit.Entry{
		TenantID: loc.TenantID, Action: action, EntityType: "recording", EntityID: loc.ID,
		Metadata: map[string]any{"call_id": loc.CallID, "platform_admin": byPlatform},
	})
}

func (s *Service) grant(ctx context.Context, loc *Located, download bool) (*AccessGrant, error) {
	filename := ""
	if download {
		filename = "call-" + loc.CallID.String() + ".ogg"
	}
	url, err := s.store.PresignGet(ctx, *loc.Key, AccessLinkTTL, filename)
	if err != nil {
		return nil, err
	}
	return &AccessGrant{URL: url, ExpiresAt: s.now().Add(AccessLinkTTL)}, nil
}

// Delete hides a call's recording from the tenant (D7). The file is kept and still counts
// toward storage (D17); the platform admin can restore it.
func (s *Service) Delete(ctx context.Context, callID uuid.UUID) error {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return err
	}
	if !p.HasRole(reqctx.RoleTenantAdmin) {
		return apperr.Forbidden("Only administrators can delete recordings.")
	}
	return s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		loc, err := s.repo.SoftDelete(ctx, callID, p.UserID)
		if err != nil {
			return err
		}
		if loc == nil {
			return errNotFound
		}
		return s.audit.Record(ctx, audit.Entry{
			TenantID: loc.TenantID, Action: audit.ActionRecordingDeleted, EntityType: "recording", EntityID: loc.ID,
			Metadata: map[string]any{"call_id": loc.CallID},
		})
	})
}

// ListDeleted lists soft-deleted recordings across tenants (platform admin).
func (s *Service) ListDeleted(ctx context.Context, page httpx.Page) ([]DeletedItem, int, error) {
	var items []DeletedItem
	var total int
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		items, total, err = s.repo.ListDeleted(ctx, page)
		return err
	})
	return items, total, err
}

// PlatformAccess issues a playback link for any recording, deleted or not (platform admin).
func (s *Service) PlatformAccess(ctx context.Context, id uuid.UUID, download bool) (*AccessGrant, error) {
	var loc *Located
	err := s.tx.InPlatform(ctx, func(ctx context.Context) (err error) {
		if loc, err = s.repo.FindByID(ctx, id); err != nil {
			return err
		}
		if loc == nil {
			return errNotFound
		}
		return s.authorise(ctx, loc, download, true)
	})
	if err != nil {
		return nil, err
	}
	return s.grant(ctx, loc, download)
}

// Restore makes a soft-deleted recording visible to its tenant again (platform admin).
func (s *Service) Restore(ctx context.Context, id uuid.UUID) error {
	return s.tx.InPlatform(ctx, func(ctx context.Context) error {
		loc, err := s.repo.Restore(ctx, id)
		if err != nil {
			return err
		}
		if loc == nil {
			return errNotFound
		}
		return s.audit.Record(ctx, audit.Entry{
			TenantID: loc.TenantID, Action: audit.ActionRecordingRestored, EntityType: "recording", EntityID: loc.ID,
			Metadata: map[string]any{"call_id": loc.CallID},
		})
	})
}
