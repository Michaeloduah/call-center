package history

import (
	"context"

	"github.com/google/uuid"

	"callcenter/internal/chat"
	"callcenter/internal/platform/database"
	"callcenter/internal/platform/httpx"
	"callcenter/internal/platform/reqctx"
)

// Transcripts reads a call's chat messages (chat.Repository).
type Transcripts interface {
	List(ctx context.Context, callID uuid.UUID) ([]chat.Message, error)
}

type Service struct {
	tx          database.TxManager
	repo        Repository
	transcripts Transcripts
}

func NewService(tx database.TxManager, repo Repository, transcripts Transcripts) *Service {
	return &Service{tx: tx, repo: repo, transcripts: transcripts}
}

func scopeOf(p reqctx.Principal) Scope {
	return Scope{All: p.Role == reqctx.RoleTenantAdmin, UserID: p.UserID}
}

func (s *Service) List(ctx context.Context, f Filter, page httpx.Page) ([]Item, int, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, 0, err
	}
	var items []Item
	var total int
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) (err error) {
		items, total, err = s.repo.List(ctx, scopeOf(p), f, page)
		return err
	})
	return items, total, err
}

// Get returns one call. Calls outside the user's scope are reported as not found.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Detail, error) {
	p, err := reqctx.TenantPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	var d *Detail
	err = s.tx.InTenant(ctx, p.TenantID, func(ctx context.Context) error {
		if d, err = s.repo.Get(ctx, id, scopeOf(p)); err != nil {
			return err
		}
		if d == nil {
			return errNotFound
		}
		if d.Offers, err = s.repo.Offers(ctx, id); err != nil {
			return err
		}
		if d.Recording, err = s.repo.Recording(ctx, id); err != nil {
			return err
		}
		if d.Messages, err = s.transcripts.List(ctx, id); err != nil {
			return err
		}
		d.Callback, err = s.repo.Callback(ctx, id)
		return err
	})
	return d, err
}
