// Package store provides transaction-bound PostgreSQL persistence for Identity.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

var _ identityapp.UpsertStore = (*Repository)(nil)
var _ identityapp.ResolveStore = (*Repository)(nil)
var _ identityapp.BindStore = (*Repository)(nil)

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

func (repository *Repository) LookupNormalized(
	ctx context.Context,
	identity identityapp.NormalizedIdentity,
) (identityapp.ResolveRecord, error) {
	if repository == nil {
		return identityapp.ResolveRecord{}, identityapp.ErrInvalidIdentity
	}
	if err := identityapp.ValidateNormalized(identity); err != nil {
		return identityapp.ResolveRecord{}, identityapp.ErrInvalidIdentity
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.ResolveRecord{}, err
	}
	row, err := identitydb.New(tx).LookupNormalizedIdentity(ctx, identitydb.LookupNormalizedIdentityParams{
		Kind:            string(identity.Kind),
		Scope:           identity.Scope,
		NormalizedValue: identity.NormalizedValue,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identityapp.ResolveRecord{}, nil
	}
	if err != nil {
		return identityapp.ResolveRecord{}, err
	}
	if !row.IdentityCustomerID.Valid {
		return identityapp.ResolveRecord{}, nil
	}
	if row.IdentityCustomerID.Int64 <= 0 || !row.CustomerIsDeleted.Valid || row.CustomerIsDeleted.Bool {
		return identityapp.ResolveRecord{Conflict: true}, nil
	}
	return identityapp.ResolveRecord{CustomerID: row.IdentityCustomerID.Int64}, nil
}

func (repository *Repository) ReserveBindReceipt(ctx context.Context, keyDigest, payloadHMAC []byte) (identityapp.BindReceipt, error) {
	if repository == nil || len(keyDigest) != 32 || len(payloadHMAC) != 32 {
		return identityapp.BindReceipt{}, identityapp.ErrIdentityBindFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.BindReceipt{}, err
	}
	queries := identitydb.New(tx)
	id, err := queries.ReserveBindReceipt(ctx, identitydb.ReserveBindReceiptParams{KeyDigest: keyDigest, PayloadHmac: payloadHMAC})
	if err == nil {
		if id <= 0 {
			return identityapp.BindReceipt{}, identityapp.ErrIdentityBindFailed
		}
		return identityapp.BindReceipt{ID: id}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return identityapp.BindReceipt{}, err
	}
	row, err := queries.LoadBindReceipt(ctx, keyDigest)
	if err != nil || row.State != "completed" || !row.ResultStatus.Valid {
		if err != nil {
			return identityapp.BindReceipt{}, err
		}
		return identityapp.BindReceipt{}, identityapp.ErrIdentityBindFailed
	}
	result, err := bindResultFromReceipt(row.ResultStatus.String, row.ResultCustomerID)
	if err != nil {
		return identityapp.BindReceipt{}, err
	}
	return identityapp.BindReceipt{Found: true, PayloadHMAC: append([]byte(nil), row.PayloadHmac...), Result: result}, nil
}

func (repository *Repository) BindNormalized(ctx context.Context, identity identityapp.NormalizedIdentity, customerID int64) (identityapp.BindRecord, error) {
	if repository == nil || customerID <= 0 || identityapp.ValidateNormalized(identity) != nil {
		return identityapp.BindRecord{}, identityapp.ErrIdentityBindFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.BindRecord{}, err
	}
	queries := identitydb.New(tx)
	if _, err = queries.LockActiveBindCustomer(ctx, customerID); errors.Is(err, pgx.ErrNoRows) {
		return identityapp.BindRecord{Status: identityport.BindRejected}, nil
	} else if err != nil {
		return identityapp.BindRecord{}, err
	}
	row, err := queries.LockIdentityForBind(ctx, identitydb.LockIdentityForBindParams{
		Kind: string(identity.Kind), Scope: identity.Scope, NormalizedValue: identity.NormalizedValue,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identityapp.BindRecord{Status: identityport.BindRejected}, nil
	}
	if err != nil || row.ID <= 0 {
		if err != nil {
			return identityapp.BindRecord{}, err
		}
		return identityapp.BindRecord{}, identityapp.ErrIdentityBindFailed
	}
	if row.CustomerID.Valid {
		if row.CustomerID.Int64 == customerID {
			return identityapp.BindRecord{Status: identityport.BindAlreadyBound, IdentityID: row.ID}, nil
		}
		return identityapp.BindRecord{Status: identityport.BindRejected, IdentityID: row.ID}, nil
	}
	boundID, err := queries.BindFloatingIdentity(ctx, identitydb.BindFloatingIdentityParams{CustomerID: customerID, IdentityID: row.ID})
	if err != nil {
		return identityapp.BindRecord{}, err
	}
	if boundID != row.ID {
		return identityapp.BindRecord{}, identityapp.ErrIdentityBindFailed
	}
	return identityapp.BindRecord{Status: identityport.BindBound, IdentityID: boundID}, nil
}

func (repository *Repository) CompleteBindReceipt(ctx context.Context, receipt identityapp.BindReceipt, result identityport.BindResult) error {
	if repository == nil || receipt.ID <= 0 || !bindReceiptResultValid(result) {
		return identityapp.ErrIdentityBindFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	params := identitydb.CompleteBindReceiptParams{ID: receipt.ID, ResultStatus: string(result.Status)}
	if result.CustomerID > 0 {
		params.ResultCustomerID = pgtype.Int8{Int64: int64(result.CustomerID), Valid: true}
	}
	updated, err := identitydb.New(tx).CompleteBindReceipt(ctx, params)
	if err != nil {
		return err
	}
	if updated != 1 {
		return identityapp.ErrIdentityBindFailed
	}
	return nil
}

func bindResultFromReceipt(status string, customerID pgtype.Int8) (identityport.BindResult, error) {
	switch identityport.BindStatus(status) {
	case identityport.BindBound, identityport.BindAlreadyBound:
		if !customerID.Valid || customerID.Int64 <= 0 {
			return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
		}
		return identityport.BindResult{Status: identityport.BindStatus(status), CustomerID: contactport.CustomerID(customerID.Int64)}, nil
	case identityport.BindRejected:
		if customerID.Valid {
			return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
		}
		return identityport.BindResult{Status: identityport.BindRejected}, nil
	default:
		return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
	}
}

func bindReceiptResultValid(result identityport.BindResult) bool {
	if result.PrimaryCustomerID != 0 || result.MergeAuditID != 0 || result.ReviewID != 0 {
		return false
	}
	switch result.Status {
	case identityport.BindBound, identityport.BindAlreadyBound:
		return result.CustomerID > 0
	case identityport.BindRejected:
		return result.CustomerID == 0
	default:
		return false
	}
}
