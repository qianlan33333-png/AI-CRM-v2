package app

import (
	"context"
	"errors"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrIdentityResolveFailed = errors.New("identity resolve failed")

// ResolveRecord is the minimal fact Resolve needs from Identity-owned storage.
// A conflict indicates a trusted binding whose Contact parent is unavailable.
type ResolveRecord struct {
	CustomerID int64
	Conflict   bool
}

type ResolveStore interface {
	LookupNormalized(context.Context, NormalizedIdentity) (ResolveRecord, error)
}

// ResolveService maps one normalized identity to an explicit, non-leaking
// result. It only reads Identity storage and never creates a customer or edge.
type ResolveService struct {
	uow   platformport.UnitOfWork
	store ResolveStore
}

func NewResolveService(uow platformport.UnitOfWork, store ResolveStore) *ResolveService {
	return &ResolveService{uow: uow, store: store}
}

func (service *ResolveService) Resolve(ctx context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	_, err := Normalize(ref)
	if err != nil {
		return identityport.ResolveResult{}, err
	}
	if service == nil || service.uow == nil || service.store == nil || ctx == nil {
		return identityport.ResolveResult{}, ErrIdentityResolveFailed
	}

	var result identityport.ResolveResult
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		var readErr error
		result, readErr = service.ResolveInTransaction(txCtx, ref)
		return readErr
	})
	if err != nil {
		return identityport.ResolveResult{}, errors.Join(ErrIdentityResolveFailed, err)
	}
	return result, nil
}

// ResolveInTransaction retains Identity normalization and conflict semantics
// for a caller that already owns the UnitOfWork.
func (service *ResolveService) ResolveInTransaction(ctx context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	normalized, err := Normalize(ref)
	if err != nil || service == nil || service.store == nil || ctx == nil || ctx.Err() != nil {
		return identityport.ResolveResult{}, ErrIdentityResolveFailed
	}
	record, err := service.store.LookupNormalized(ctx, normalized)
	if err != nil {
		return identityport.ResolveResult{}, errors.Join(ErrIdentityResolveFailed, err)
	}
	switch {
	case record.Conflict:
		return identityport.ResolveResult{Status: identityport.ResolveConflict}, nil
	case record.CustomerID == 0:
		return identityport.ResolveResult{Status: identityport.ResolveNotFound}, nil
	case record.CustomerID > 0:
		return identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(record.CustomerID)}, nil
	default:
		return identityport.ResolveResult{}, ErrIdentityResolveFailed
	}
}
