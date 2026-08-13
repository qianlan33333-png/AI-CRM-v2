// Package store provides transaction-bound PostgreSQL persistence for Identity.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

var _ identityapp.UpsertStore = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) UpsertNormalized(
	ctx context.Context,
	identity identityapp.NormalizedIdentity,
) (int64, bool, error) {
	if repository == nil {
		return 0, false, identityapp.ErrInvalidIdentity
	}
	if err := identityapp.ValidateNormalized(identity); err != nil {
		return 0, false, identityapp.ErrInvalidIdentity
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, false, err
	}
	row, err := identitydb.New(tx).UpsertNormalizedIdentity(ctx, identitydb.UpsertNormalizedIdentityParams{
		Kind:            string(identity.Kind),
		Scope:           identity.Scope,
		NormalizedValue: identity.NormalizedValue,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, identityapp.ErrIdentityUpsertFailed
		}
		return 0, false, err
	}
	if row.ID <= 0 {
		return 0, false, identityapp.ErrIdentityUpsertFailed
	}
	return row.ID, row.Created, nil
}
