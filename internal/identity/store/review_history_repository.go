package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// ListMergeReviewsByStatus returns a transaction-bound page containing only
// fields allowed in the closed administrator DTO. The canonical sqlc query does
// not read normalized identities, provider identifiers, raw payloads or review
// fingerprints.
func (repository *Repository) ListMergeReviewsByStatus(
	ctx context.Context,
	status identityport.MergeReviewStatus,
	afterID int64,
	limit int32,
) ([]identityapp.MergeReviewHistoryRecord, error) {
	if repository == nil || !status.Valid() || afterID < 0 || limit < 1 || limit > identityapp.MergeReviewMaximumLimit+1 {
		return nil, identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := identitydb.New(tx).ListMergeReviewsByStatus(ctx, identitydb.ListMergeReviewsByStatusParams{
		ReviewStatus: string(status),
		AfterID:      afterID,
		PageLimit:    limit,
	})
	if err != nil {
		return nil, err
	}

	result := make([]identityapp.MergeReviewHistoryRecord, 0, len(rows))
	for _, row := range rows {
		record, convertErr := mergeReviewHistoryRecord(
			row.ID, row.State, row.Kind, row.Scope, row.CandidateCustomerIds,
			row.ReviewFingerprint, row.FingerprintKeyVersion,
			row.Version, row.CreatedAt, row.ResolvedAt,
		)
		if convertErr != nil || record.Status != status {
			if convertErr != nil {
				return nil, convertErr
			}
			return nil, identityapp.ErrMergeReviewUnavailable
		}
		result = append(result, record)
	}
	return result, nil
}

func mergeReviewHistoryRecord(
	id int64,
	state, kind, scope string,
	customerIDs []int64,
	fingerprint []byte,
	fingerprintVersion pgtype.Int2,
	version int64,
	createdAt, resolvedAt pgtype.Timestamptz,
) (identityapp.MergeReviewHistoryRecord, error) {
	if id <= 0 || version <= 0 || !createdAt.Valid || len(fingerprint) != 16 ||
		!fingerprintVersion.Valid || fingerprintVersion.Int16 <= 0 || len(customerIDs) != 2 ||
		customerIDs[0] <= 0 || customerIDs[0] >= customerIDs[1] {
		return identityapp.MergeReviewHistoryRecord{}, identityapp.ErrMergeReviewUnavailable
	}
	record := identityapp.MergeReviewHistoryRecord{
		ReviewID: id,
		Status:   identityport.MergeReviewStatus(state),
		Kind:     identityport.IDKind(kind),
		Scope:    scope,
		CustomerIDs: []contactport.CustomerID{
			contactport.CustomerID(customerIDs[0]),
			contactport.CustomerID(customerIDs[1]),
		},
		IdentityFingerprint: append([]byte(nil), fingerprint...),
		FingerprintVersion:  fingerprintVersion.Int16,
		Version:             version,
		CreatedAt:           createdAt.Time.UTC(),
	}
	if resolvedAt.Valid {
		resolved := resolvedAt.Time.UTC()
		record.ResolvedAt = &resolved
	}
	return record, nil
}
