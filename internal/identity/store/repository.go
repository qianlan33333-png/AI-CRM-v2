// Package store provides transaction-bound PostgreSQL persistence for Identity.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

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
var _ identityapp.IngestStore = (*Repository)(nil)
var _ identityapp.PendingReplayStore = (*Repository)(nil)
var _ identityapp.MergeReviewStore = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) ListPendingMergeReviews(
	ctx context.Context,
	afterID int64,
	limit int32,
) ([]identityapp.MergeReviewRecord, error) {
	if repository == nil || afterID < 0 || limit < 1 || limit > identityapp.MergeReviewMaximumLimit+1 {
		return nil, identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := identitydb.New(tx).ListPendingMergeReviews(ctx, identitydb.ListPendingMergeReviewsParams{
		AfterID: afterID, PageLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]identityapp.MergeReviewRecord, 0, len(rows))
	for _, row := range rows {
		record, convertErr := mergeReviewRecord(
			row.ID, row.State, row.IdentityID, row.Kind, row.Scope, row.NormalizedValue,
			row.ReviewFingerprint, row.FingerprintKeyVersion, row.CandidateCustomerIds,
			row.PolicyVersion, row.Version, row.CreatedAt, row.ResolvedAt,
		)
		if convertErr != nil {
			return nil, convertErr
		}
		result = append(result, record)
	}
	return result, nil
}

func (repository *Repository) ReserveMergeReviewReceipt(
	ctx context.Context,
	operation string,
	keyDigest, payloadHMAC []byte,
) (identityapp.MergeReviewReceipt, error) {
	if repository == nil || !validMergeReviewOperation(operation) || len(keyDigest) != 32 || len(payloadHMAC) != 32 {
		return identityapp.MergeReviewReceipt{}, identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.MergeReviewReceipt{}, err
	}
	queries := identitydb.New(tx)
	id, err := queries.ReserveMergeReviewReceipt(ctx, identitydb.ReserveMergeReviewReceiptParams{
		Operation: operation, KeyDigest: keyDigest, PayloadHmac: payloadHMAC,
	})
	if err == nil {
		if id <= 0 {
			return identityapp.MergeReviewReceipt{}, identityapp.ErrMergeReviewUnavailable
		}
		return identityapp.MergeReviewReceipt{ID: id}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return identityapp.MergeReviewReceipt{}, err
	}
	row, err := queries.LoadMergeReviewReceipt(ctx, identitydb.LoadMergeReviewReceiptParams{
		Operation: operation, KeyDigest: keyDigest,
	})
	if err != nil {
		return identityapp.MergeReviewReceipt{}, err
	}
	status := identityport.MergeReviewStatus(row.ResultStatus.String)
	if row.State != "completed" || !row.ResultStatus.Valid || !row.ResultPendingEventID.Valid ||
		row.ResultPendingEventID.Int64 <= 0 ||
		(status != identityport.MergeReviewApproved && status != identityport.MergeReviewRejected) {
		return identityapp.MergeReviewReceipt{}, identityapp.ErrMergeReviewUnavailable
	}
	return identityapp.MergeReviewReceipt{
		Found: true, PayloadHMAC: append([]byte(nil), row.PayloadHmac...),
		ReviewID: row.ResultPendingEventID.Int64, Status: status,
	}, nil
}

func (repository *Repository) LockMergeReview(ctx context.Context, reviewID int64) (identityapp.MergeReviewRecord, error) {
	if repository == nil || reviewID <= 0 {
		return identityapp.MergeReviewRecord{}, identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.MergeReviewRecord{}, err
	}
	row, err := identitydb.New(tx).LockMergeReview(ctx, reviewID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identityapp.MergeReviewRecord{}, identityapp.ErrMergeReviewNotFound
	}
	if err != nil {
		return identityapp.MergeReviewRecord{}, err
	}
	return mergeReviewRecord(
		row.ID, row.State, row.IdentityID, row.Kind, row.Scope, row.NormalizedValue,
		row.ReviewFingerprint, row.FingerprintKeyVersion, row.CandidateCustomerIds,
		row.PolicyVersion, row.Version, row.CreatedAt, row.ResolvedAt,
	)
}

func (repository *Repository) LockActiveMergeReviewCustomers(
	ctx context.Context,
	customerIDs []contactport.CustomerID,
) ([]contactport.CustomerID, error) {
	if repository == nil || len(customerIDs) != 2 || customerIDs[0] <= 0 || customerIDs[0] >= customerIDs[1] {
		return nil, identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := identitydb.New(tx).LockActiveMergeReviewCustomers(ctx, []int64{int64(customerIDs[0]), int64(customerIDs[1])})
	if err != nil {
		return nil, err
	}
	result := make([]contactport.CustomerID, 0, len(rows))
	for _, id := range rows {
		result = append(result, contactport.CustomerID(id))
	}
	return result, nil
}

func (repository *Repository) InsertManualCustomerMergeAudit(ctx context.Context, audit identityapp.ManualMergeAudit) (int64, error) {
	if repository == nil || audit.PrimaryCustomerID <= 0 || audit.MergedCustomerID <= 0 ||
		audit.PrimaryCustomerID == audit.MergedCustomerID || strings.TrimSpace(audit.PolicyVersion) == "" ||
		len(audit.PolicyVersion) > 200 || len(audit.ReviewFingerprint) != 16 || audit.FingerprintVersion <= 0 ||
		strings.TrimSpace(string(audit.Actor)) == "" || len(audit.Actor) > 200 || !json.Valid(audit.Detail) {
		return 0, identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	id, err := identitydb.New(tx).InsertManualCustomerMergeAudit(ctx, identitydb.InsertManualCustomerMergeAuditParams{
		PrimaryCustomerID: int64(audit.PrimaryCustomerID), MergedCustomerID: int64(audit.MergedCustomerID),
		PolicyVersion: audit.PolicyVersion, ReviewFingerprint: append([]byte(nil), audit.ReviewFingerprint...),
		FingerprintKeyVersion: audit.FingerprintVersion, OperatedBy: string(audit.Actor),
		Detail: append([]byte(nil), audit.Detail...),
	})
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, identityapp.ErrMergeReviewUnavailable
	}
	return id, nil
}

func (repository *Repository) ResolveMergeReview(
	ctx context.Context,
	reviewID, expectedVersion int64,
	status identityport.MergeReviewStatus,
) (identityapp.MergeReviewRecord, error) {
	if repository == nil || reviewID <= 0 || expectedVersion <= 0 ||
		(status != identityport.MergeReviewApproved && status != identityport.MergeReviewRejected) {
		return identityapp.MergeReviewRecord{}, identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.MergeReviewRecord{}, err
	}
	queries := identitydb.New(tx)
	changed, err := queries.ResolveMergeReview(ctx, identitydb.ResolveMergeReviewParams{
		ResultStatus: string(status), ReviewID: reviewID, ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return identityapp.MergeReviewRecord{}, err
	}
	if changed != 1 {
		return identityapp.MergeReviewRecord{}, identityapp.ErrMergeReviewConflict
	}
	return repository.LockMergeReview(ctx, reviewID)
}

func (repository *Repository) CompleteMergeReviewReceipt(
	ctx context.Context,
	receipt identityapp.MergeReviewReceipt,
	review identityapp.MergeReviewRecord,
	mergeAuditID int64,
) error {
	if repository == nil || receipt.ID <= 0 || review.ReviewID <= 0 ||
		(review.Status != identityport.MergeReviewApproved && review.Status != identityport.MergeReviewRejected) ||
		(review.Status == identityport.MergeReviewApproved) != (mergeAuditID > 0) {
		return identityapp.ErrMergeReviewInvalid
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	params := identitydb.CompleteMergeReviewReceiptParams{
		ResultStatus: string(review.Status), ReviewID: review.ReviewID,
		PolicyVersion: review.PolicyVersion, ReceiptID: receipt.ID,
	}
	if mergeAuditID > 0 {
		params.ResultMergeAuditID = pgtype.Int8{Int64: mergeAuditID, Valid: true}
	}
	changed, err := identitydb.New(tx).CompleteMergeReviewReceipt(ctx, params)
	if err != nil {
		return err
	}
	if changed != 1 {
		return identityapp.ErrMergeReviewUnavailable
	}
	return nil
}

func mergeReviewRecord(
	id int64,
	state string,
	identityID int64,
	kind, scope, normalizedValue string,
	fingerprint []byte,
	fingerprintVersion pgtype.Int2,
	customerIDs []int64,
	policy string,
	version int64,
	createdAt, resolvedAt pgtype.Timestamptz,
) (identityapp.MergeReviewRecord, error) {
	status := identityport.MergeReviewStatus(state)
	if id <= 0 || identityID <= 0 || !fingerprintVersion.Valid || !createdAt.Valid ||
		len(customerIDs) != 2 || customerIDs[0] <= 0 || customerIDs[0] >= customerIDs[1] {
		return identityapp.MergeReviewRecord{}, identityapp.ErrMergeReviewUnavailable
	}
	record := identityapp.MergeReviewRecord{
		ReviewID: id, Status: status, Kind: identityport.IDKind(kind), Scope: scope,
		NormalizedValue: normalizedValue, IdentityID: identityID,
		IdentityFingerprint: append([]byte(nil), fingerprint...), FingerprintVersion: fingerprintVersion.Int16,
		CustomerIDs:   []contactport.CustomerID{contactport.CustomerID(customerIDs[0]), contactport.CustomerID(customerIDs[1])},
		PolicyVersion: policy, Version: version, CreatedAt: createdAt.Time.UTC(),
	}
	if resolvedAt.Valid {
		resolved := resolvedAt.Time.UTC()
		record.ResolvedAt = &resolved
	}
	return record, nil
}

func validMergeReviewOperation(operation string) bool {
	return operation == "merge_review_approve" || operation == "merge_review_reject"
}

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
	result, err := bindResultFromReceipt(row.ResultStatus.String, row.ResultCustomerID, row.ResultMergeAuditID, row.ResultPendingEventID, row.ResultPolicyVersion)
	if err != nil {
		return identityapp.BindReceipt{}, err
	}
	if result.Status == identityport.BindMerged {
		audit, err := queries.LoadCustomerMergeAudit(ctx, result.MergeAuditID)
		if err != nil || audit.PrimaryCustomerID <= 0 || audit.PolicyVersion != identityport.MergePolicyVerifiedUnionIDUniqueWeCom {
			if err != nil {
				return identityapp.BindReceipt{}, err
			}
			return identityapp.BindReceipt{}, identityapp.ErrIdentityBindFailed
		}
		result.PrimaryCustomerID = contactport.CustomerID(audit.PrimaryCustomerID)
	}
	if result.Status == identityport.BindManualReview {
		if _, err = queries.LoadBindMergeReview(ctx, result.ReviewID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return identityapp.BindReceipt{}, identityapp.ErrIdentityBindFailed
			}
			return identityapp.BindReceipt{}, err
		}
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
	if !row.CustomerID.Valid {
		if _, err = queries.LockActiveBindCustomer(ctx, customerID); errors.Is(err, pgx.ErrNoRows) {
			return identityapp.BindRecord{Status: identityport.BindRejected}, nil
		} else if err != nil {
			return identityapp.BindRecord{}, err
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
	if row.CustomerID.Int64 == customerID {
		if _, err = queries.LockActiveBindCustomer(ctx, customerID); errors.Is(err, pgx.ErrNoRows) {
			return identityapp.BindRecord{Status: identityport.BindRejected}, nil
		} else if err != nil {
			return identityapp.BindRecord{}, err
		}
		return identityapp.BindRecord{Status: identityport.BindAlreadyBound, IdentityID: row.ID}, nil
	}
	customers, err := queries.LockActiveBindCustomersForMerge(ctx, []int64{customerID, row.CustomerID.Int64})
	if err != nil {
		return identityapp.BindRecord{}, err
	}
	if len(customers) != 2 {
		return identityapp.BindRecord{Status: identityport.BindRejected, IdentityID: row.ID}, nil
	}
	requestedVerifiedWeCom, err := queries.HasVerifiedWeComIdentityForBindCustomer(ctx, customerID)
	if err != nil {
		return identityapp.BindRecord{}, err
	}
	existingVerifiedWeCom, err := queries.HasVerifiedWeComIdentityForBindCustomer(ctx, row.CustomerID.Int64)
	if err != nil {
		return identityapp.BindRecord{}, err
	}
	return identityapp.BindRecord{
		Status:                           identityport.BindRejected,
		IdentityID:                       row.ID,
		ExistingCustomerID:               contactport.CustomerID(row.CustomerID.Int64),
		RequestedHasVerifiedWeCom:        requestedVerifiedWeCom,
		ExistingCustomerHasVerifiedWeCom: existingVerifiedWeCom,
	}, nil
}

func (repository *Repository) RebindIdentitiesForCustomerMerge(
	ctx context.Context,
	primaryCustomerID contactport.CustomerID,
	mergedCustomerID contactport.CustomerID,
) error {
	if repository == nil || primaryCustomerID <= 0 || mergedCustomerID <= 0 || primaryCustomerID == mergedCustomerID {
		return identityapp.ErrIdentityBindFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	changed, err := identitydb.New(tx).RebindIdentitiesForCustomerMerge(ctx, identitydb.RebindIdentitiesForCustomerMergeParams{
		PrimaryCustomerID: int64(primaryCustomerID),
		MergedCustomerID:  int64(mergedCustomerID),
	})
	if err != nil {
		return err
	}
	if changed < 1 {
		return identityapp.ErrIdentityBindFailed
	}
	return nil
}

func (repository *Repository) InsertAutoCustomerMergeAudit(ctx context.Context, audit identityapp.AutoMergeAudit) (int64, error) {
	if repository == nil || audit.PrimaryCustomerID <= 0 || audit.MergedCustomerID <= 0 ||
		audit.PrimaryCustomerID == audit.MergedCustomerID || audit.PolicyVersion != identityport.MergePolicyVerifiedUnionIDUniqueWeCom ||
		len(audit.ReviewFingerprint) != 16 || audit.FingerprintVersion <= 0 ||
		strings.TrimSpace(string(audit.Actor)) == "" || len(audit.Actor) > 200 || !json.Valid(audit.Detail) {
		return 0, identityapp.ErrIdentityBindFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	id, err := identitydb.New(tx).InsertAutoCustomerMergeAudit(ctx, identitydb.InsertAutoCustomerMergeAuditParams{
		PrimaryCustomerID:     int64(audit.PrimaryCustomerID),
		MergedCustomerID:      int64(audit.MergedCustomerID),
		PolicyVersion:         audit.PolicyVersion,
		ReviewFingerprint:     append([]byte(nil), audit.ReviewFingerprint...),
		FingerprintKeyVersion: audit.FingerprintVersion,
		OperatedBy:            string(audit.Actor),
		Detail:                append([]byte(nil), audit.Detail...),
	})
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, identityapp.ErrIdentityBindFailed
	}
	return id, nil
}

func (repository *Repository) InsertVerifiedPhoneMergeReview(
	ctx context.Context,
	identityID int64,
	candidates []contactport.CustomerID,
	fingerprint []byte,
) (int64, error) {
	if repository == nil || identityID <= 0 || len(candidates) != 2 || candidates[0] <= 0 ||
		candidates[0] >= candidates[1] || len(fingerprint) != 16 {
		return 0, identityapp.ErrIdentityBindFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	id, err := identitydb.New(tx).InsertVerifiedPhoneMergeReview(ctx, identitydb.InsertVerifiedPhoneMergeReviewParams{
		IdentityIds:          []int64{identityID},
		CandidateCustomerIds: []int64{int64(candidates[0]), int64(candidates[1])},
		ReviewFingerprint:    append([]byte(nil), fingerprint...),
	})
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, identityapp.ErrIdentityBindFailed
	}
	return id, nil
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
	if result.MergeAuditID > 0 {
		params.ResultMergeAuditID = pgtype.Int8{Int64: result.MergeAuditID, Valid: true}
		params.ResultPolicyVersion = pgtype.Text{String: identityport.MergePolicyVerifiedUnionIDUniqueWeCom, Valid: true}
	}
	if result.ReviewID > 0 {
		params.ResultPendingEventID = pgtype.Int8{Int64: result.ReviewID, Valid: true}
		params.ResultPolicyVersion = pgtype.Text{String: identityapp.VerifiedPhoneMergeReviewPolicy, Valid: true}
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

func (repository *Repository) ReserveIngestReceipt(ctx context.Context, keyDigest, payloadHMAC []byte) (identityapp.IngestReceipt, error) {
	if repository == nil || len(keyDigest) != 32 || len(payloadHMAC) != 32 {
		return identityapp.IngestReceipt{}, identityapp.ErrIdentityIngestFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.IngestReceipt{}, err
	}
	queries := identitydb.New(tx)
	id, err := queries.ReserveIngestReceipt(ctx, identitydb.ReserveIngestReceiptParams{KeyDigest: keyDigest, PayloadHmac: payloadHMAC})
	if err == nil {
		if id <= 0 {
			return identityapp.IngestReceipt{}, identityapp.ErrIdentityIngestFailed
		}
		return identityapp.IngestReceipt{ID: id}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return identityapp.IngestReceipt{}, err
	}
	row, err := queries.LoadIngestReceipt(ctx, keyDigest)
	if err != nil || row.State != "completed" || !row.ResultStatus.Valid ||
		!row.ResultPolicyVersion.Valid || row.ResultPolicyVersion.String != identityapp.IngestAttributionPolicy {
		if err != nil {
			return identityapp.IngestReceipt{}, err
		}
		return identityapp.IngestReceipt{}, identityapp.ErrIdentityIngestFailed
	}
	result, err := ingestResultFromReceipt(row.ResultStatus.String, row.ResultCustomerID, row.ResultEventID, row.ResultPendingEventID)
	if err != nil {
		return identityapp.IngestReceipt{}, err
	}
	if result.Status == identityport.IngestPending || result.Status == identityport.IngestConflict {
		kind, err := queries.LoadPendingIngest(ctx, result.PendingEventID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return identityapp.IngestReceipt{}, identityapp.ErrIdentityIngestFailed
			}
			return identityapp.IngestReceipt{}, err
		}
		wantKind := "attribution"
		if result.Status == identityport.IngestConflict {
			wantKind = "conflict"
		}
		if kind != wantKind {
			return identityapp.IngestReceipt{}, identityapp.ErrIdentityIngestFailed
		}
	}
	return identityapp.IngestReceipt{Found: true, PayloadHMAC: append([]byte(nil), row.PayloadHmac...), Result: result}, nil
}

func (repository *Repository) InsertPendingIngest(ctx context.Context, pending identityapp.PendingIngest) (int64, error) {
	if repository == nil || (pending.Status != identityport.IngestPending && pending.Status != identityport.IngestConflict) ||
		!validSortedIdentityIDs(pending.IdentityIDs) || strings.TrimSpace(pending.EventType) == "" || len(pending.EventType) > 200 ||
		strings.TrimSpace(pending.Source) == "" || len(pending.Source) > 200 || strings.TrimSpace(pending.IdempotencyKey) == "" ||
		len(pending.IdempotencyKey) > 512 || pending.OccurredAt.IsZero() || !validJSONObject(pending.Payload) {
		return 0, identityapp.ErrIdentityIngestFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	kind := "attribution"
	if pending.Status == identityport.IngestConflict {
		kind = "conflict"
	}
	id, err := identitydb.New(tx).InsertPendingIngest(ctx, identitydb.InsertPendingIngestParams{
		Kind: kind, IdentityIds: append([]int64(nil), pending.IdentityIDs...), EventType: pending.EventType,
		Payload: append([]byte(nil), pending.Payload...), Source: pending.Source, IdempotencyKey: pending.IdempotencyKey,
		OccurredAt: pgtype.Timestamptz{Time: pending.OccurredAt.UTC().Truncate(time.Microsecond), Valid: true},
	})
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, identityapp.ErrIdentityIngestFailed
	}
	return id, nil
}

func (repository *Repository) ClaimPendingReplay(ctx context.Context) (identityapp.PendingReplay, bool, error) {
	if repository == nil {
		return identityapp.PendingReplay{}, false, identityapp.ErrPendingReplayFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityapp.PendingReplay{}, false, err
	}
	queries := identitydb.New(tx)
	row, err := queries.ClaimPendingReplay(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return identityapp.PendingReplay{}, false, nil
	}
	if err != nil {
		return identityapp.PendingReplay{}, false, err
	}
	if row.ID <= 0 || !row.EventType.Valid || !row.IdempotencyKey.Valid || !row.OccurredAt.Valid ||
		!validSortedIdentityIDs(row.IdentityIds) || !validJSONObject(row.Payload) {
		return identityapp.PendingReplay{}, false, identityapp.ErrPendingReplayFailed
	}
	identityRows, err := queries.LockPendingReplayIdentities(ctx, row.IdentityIds)
	if err != nil {
		return identityapp.PendingReplay{}, false, err
	}
	if len(identityRows) != len(row.IdentityIds) {
		return identityapp.PendingReplay{}, false, identityapp.ErrPendingReplayFailed
	}
	identities := make([]identityapp.PendingReplayIdentity, 0, len(identityRows))
	for index, identityRow := range identityRows {
		if identityRow.ID != row.IdentityIds[index] {
			return identityapp.PendingReplay{}, false, identityapp.ErrPendingReplayFailed
		}
		identity := identityapp.NormalizedIdentity{
			Kind: identityport.IDKind(identityRow.Kind), Scope: identityRow.Scope,
			NormalizedValue: identityRow.NormalizedValue, NormalizerVersion: identityRow.NormalizerVersion,
		}
		if identityapp.ValidateNormalized(identity) != nil {
			return identityapp.PendingReplay{}, false, identityapp.ErrPendingReplayFailed
		}
		identities = append(identities, identityapp.PendingReplayIdentity{ID: identityRow.ID, Identity: identity})
	}
	return identityapp.PendingReplay{
		ID: row.ID, Kind: row.Kind, Identities: identities, EventType: row.EventType.String,
		Payload: append([]byte(nil), row.Payload...), Source: row.Source,
		IdempotencyKey: row.IdempotencyKey.String, OccurredAt: row.OccurredAt.Time.UTC().Truncate(time.Microsecond),
		Version: row.Version,
	}, true, nil
}

func (repository *Repository) CompletePendingReplay(ctx context.Context, pendingEventID, expectedVersion int64) error {
	if repository == nil || pendingEventID <= 0 || expectedVersion <= 0 {
		return identityapp.ErrPendingReplayFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	updated, err := identitydb.New(tx).CompletePendingReplay(ctx, identitydb.CompletePendingReplayParams{
		PendingEventID: pendingEventID, ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return err
	}
	if updated != 1 {
		return identityapp.ErrPendingReplayFailed
	}
	return nil
}

func (repository *Repository) DeferPendingReplay(ctx context.Context, pendingEventID, expectedVersion int64) error {
	if repository == nil || pendingEventID <= 0 || expectedVersion <= 0 {
		return identityapp.ErrPendingReplayFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	updated, err := identitydb.New(tx).DeferPendingReplay(ctx, identitydb.DeferPendingReplayParams{
		PendingEventID: pendingEventID, ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return err
	}
	if updated != 1 {
		return identityapp.ErrPendingReplayFailed
	}
	return nil
}

func (repository *Repository) CompleteIngestReceipt(ctx context.Context, receipt identityapp.IngestReceipt, result identityport.IngestResult) error {
	if repository == nil || receipt.ID <= 0 || !ingestReceiptResultValid(result) {
		return identityapp.ErrIdentityIngestFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	params := identitydb.CompleteIngestReceiptParams{ID: receipt.ID, ResultStatus: string(result.Status)}
	if result.CustomerID > 0 {
		params.ResultCustomerID = pgtype.Int8{Int64: int64(result.CustomerID), Valid: true}
	}
	if result.EventID > 0 {
		params.ResultEventID = pgtype.Int8{Int64: int64(result.EventID), Valid: true}
	}
	if result.PendingEventID > 0 {
		params.ResultPendingEventID = pgtype.Int8{Int64: result.PendingEventID, Valid: true}
	}
	updated, err := identitydb.New(tx).CompleteIngestReceipt(ctx, params)
	if err != nil {
		return err
	}
	if updated != 1 {
		return identityapp.ErrIdentityIngestFailed
	}
	return nil
}

func ingestResultFromReceipt(status string, customerID, eventID, pendingEventID pgtype.Int8) (identityport.IngestResult, error) {
	result := identityport.IngestResult{Status: identityport.IngestStatus(status)}
	switch result.Status {
	case identityport.IngestAttributed:
		if !customerID.Valid || customerID.Int64 <= 0 || !eventID.Valid || eventID.Int64 <= 0 || pendingEventID.Valid {
			return identityport.IngestResult{}, identityapp.ErrIdentityIngestFailed
		}
		result.CustomerID = contactport.CustomerID(customerID.Int64)
		result.EventID = contactport.EventID(eventID.Int64)
	case identityport.IngestPending, identityport.IngestConflict:
		if customerID.Valid || eventID.Valid || !pendingEventID.Valid || pendingEventID.Int64 <= 0 {
			return identityport.IngestResult{}, identityapp.ErrIdentityIngestFailed
		}
		result.PendingEventID = pendingEventID.Int64
	default:
		return identityport.IngestResult{}, identityapp.ErrIdentityIngestFailed
	}
	return result, nil
}

func ingestReceiptResultValid(result identityport.IngestResult) bool {
	switch result.Status {
	case identityport.IngestAttributed:
		return result.CustomerID > 0 && result.EventID > 0 && result.PendingEventID == 0
	case identityport.IngestPending, identityport.IngestConflict:
		return result.CustomerID == 0 && result.EventID == 0 && result.PendingEventID > 0
	default:
		return false
	}
}

func validSortedIdentityIDs(ids []int64) bool {
	if len(ids) == 0 {
		return false
	}
	for index, id := range ids {
		if id <= 0 || (index > 0 && ids[index-1] >= id) {
			return false
		}
	}
	return true
}

func validJSONObject(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(raw)
}

func bindResultFromReceipt(status string, customerID pgtype.Int8, mergeAuditID pgtype.Int8, reviewID pgtype.Int8, policyVersion pgtype.Text) (identityport.BindResult, error) {
	switch identityport.BindStatus(status) {
	case identityport.BindBound, identityport.BindAlreadyBound:
		if !customerID.Valid || customerID.Int64 <= 0 || mergeAuditID.Valid || reviewID.Valid || policyVersion.Valid {
			return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
		}
		return identityport.BindResult{Status: identityport.BindStatus(status), CustomerID: contactport.CustomerID(customerID.Int64)}, nil
	case identityport.BindRejected:
		if customerID.Valid || mergeAuditID.Valid || reviewID.Valid || policyVersion.Valid {
			return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
		}
		return identityport.BindResult{Status: identityport.BindRejected}, nil
	case identityport.BindMerged:
		if !customerID.Valid || customerID.Int64 <= 0 || !mergeAuditID.Valid || mergeAuditID.Int64 <= 0 || reviewID.Valid ||
			!policyVersion.Valid || policyVersion.String != identityport.MergePolicyVerifiedUnionIDUniqueWeCom {
			return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
		}
		return identityport.BindResult{Status: identityport.BindMerged, CustomerID: contactport.CustomerID(customerID.Int64), MergeAuditID: mergeAuditID.Int64}, nil
	case identityport.BindManualReview:
		if customerID.Valid || mergeAuditID.Valid || !reviewID.Valid || reviewID.Int64 <= 0 ||
			!policyVersion.Valid || policyVersion.String != identityapp.VerifiedPhoneMergeReviewPolicy {
			return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
		}
		return identityport.BindResult{Status: identityport.BindManualReview, ReviewID: reviewID.Int64}, nil
	default:
		return identityport.BindResult{}, identityapp.ErrIdentityBindFailed
	}
}

func bindReceiptResultValid(result identityport.BindResult) bool {
	if result.Status != identityport.BindManualReview && result.ReviewID != 0 {
		return false
	}
	switch result.Status {
	case identityport.BindBound, identityport.BindAlreadyBound:
		return result.CustomerID > 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0
	case identityport.BindRejected:
		return result.CustomerID == 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0
	case identityport.BindMerged:
		return result.CustomerID > 0 && result.PrimaryCustomerID > 0 && result.MergeAuditID > 0
	case identityport.BindManualReview:
		return result.CustomerID == 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0 && result.ReviewID > 0
	default:
		return false
	}
}
