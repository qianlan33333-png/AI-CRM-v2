package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

type ContactPolicyRepository struct{}

var (
	_ contactapp.ContactPolicyStore  = (*ContactPolicyRepository)(nil)
	_ contactport.EligibilityChecker = (*ContactPolicyRepository)(nil)
)

func NewContactPolicyRepository() *ContactPolicyRepository { return &ContactPolicyRepository{} }

func (repository *ContactPolicyRepository) ReadActiveCustomerPolicy(ctx context.Context, customerID contactport.CustomerID, evaluatedAt time.Time) (contactapp.ContactPolicy, error) {
	if repository == nil || customerID <= 0 || evaluatedAt.IsZero() {
		return contactapp.ContactPolicy{}, contactapp.ErrContactPolicyUnavailable
	}
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return contactapp.ContactPolicy{}, contactapp.ErrContactPolicyUnavailable
	}
	if _, err = queries.LockActiveCustomerForContactPolicy(ctx, int64(customerID)); errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ContactPolicy{}, contactapp.ErrContactPolicyNotFound
	} else if err != nil {
		return contactapp.ContactPolicy{}, mapContactPolicyDatabaseError(err)
	}
	stored, present, err := readStoredContactPolicy(ctx, queries, customerID)
	if err != nil {
		return contactapp.ContactPolicy{}, err
	}
	if !present {
		return contactapp.ContactPolicy{CustomerID: customerID, Eligible: true, LocalOnly: true}, nil
	}
	return storedContactPolicyProjection(stored, evaluatedAt.UTC()), nil
}

func (repository *ContactPolicyRepository) ReserveContactPolicyReceipt(ctx context.Context, reservation contactapp.ContactPolicyReceiptReservation) (contactapp.ContactPolicyReceipt, bool, error) {
	queries, err := queriesFromContext(ctx)
	if repository == nil || err != nil {
		return contactapp.ContactPolicyReceipt{}, false, contactapp.ErrContactPolicyUnavailable
	}
	params := contactdb.ReserveCustomerContactPolicyReceiptParams{
		Operation: reservation.Operation, ActorScope: reservation.ActorScope,
		KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:],
		CreatedAt: pgtype.Timestamptz{Time: reservation.CreatedAt.UTC(), Valid: true},
	}
	row, err := queries.ReserveCustomerContactPolicyReceipt(ctx, params)
	if err == nil {
		return contactPolicyReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ContactPolicyReceipt{}, false, mapContactPolicyDatabaseError(err)
	}
	existing, err := queries.GetCustomerContactPolicyReceipt(ctx, contactdb.GetCustomerContactPolicyReceiptParams{
		Operation: reservation.Operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:],
	})
	if err != nil {
		return contactapp.ContactPolicyReceipt{}, false, mapContactPolicyDatabaseError(err)
	}
	return contactPolicyReceipt(existing.ID, existing.Operation, existing.ActorScope, existing.KeyDigest, existing.PayloadDigest, existing.State, existing.ResultSnapshot), false, nil
}

func (repository *ContactPolicyRepository) CompleteContactPolicyReceipt(ctx context.Context, receiptID int64, snapshot json.RawMessage, completedAt time.Time) (contactapp.ContactPolicyReceipt, error) {
	queries, err := queriesFromContext(ctx)
	if repository == nil || err != nil || receiptID <= 0 || len(snapshot) == 0 || completedAt.IsZero() {
		return contactapp.ContactPolicyReceipt{}, contactapp.ErrContactPolicyUnavailable
	}
	row, err := queries.CompleteCustomerContactPolicyReceipt(ctx, contactdb.CompleteCustomerContactPolicyReceiptParams{
		ID: receiptID, ResultSnapshot: append([]byte(nil), snapshot...),
		CompletedAt: pgtype.Timestamptz{Time: completedAt.UTC(), Valid: true},
	})
	if err != nil {
		return contactapp.ContactPolicyReceipt{}, mapContactPolicyDatabaseError(err)
	}
	return contactPolicyReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func (repository *ContactPolicyRepository) LockContactPolicyCustomer(ctx context.Context, customerID contactport.CustomerID) error {
	queries, err := queriesFromContext(ctx)
	if repository == nil || err != nil || customerID <= 0 {
		return contactapp.ErrContactPolicyUnavailable
	}
	// The namespaced PostgreSQL hash lock serializes Contact policy decisions.
	// A hash collision can only over-serialize unrelated customers; it is not
	// treated as a globally collision-free identity.
	if err = queries.LockCustomerContactPolicyKey(ctx, int64(customerID)); err != nil {
		return mapContactPolicyDatabaseError(err)
	}
	if _, err = queries.LockActiveCustomerForContactPolicy(ctx, int64(customerID)); errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ErrContactPolicyNotFound
	}
	return mapContactPolicyDatabaseError(err)
}

func (repository *ContactPolicyRepository) ReadStoredContactPolicy(ctx context.Context, customerID contactport.CustomerID) (contactapp.StoredContactPolicy, bool, error) {
	queries, err := queriesFromContext(ctx)
	if repository == nil || err != nil || customerID <= 0 {
		return contactapp.StoredContactPolicy{}, false, contactapp.ErrContactPolicyUnavailable
	}
	return readStoredContactPolicy(ctx, queries, customerID)
}

func (repository *ContactPolicyRepository) InsertContactPolicy(ctx context.Context, value contactapp.StoredContactPolicy) (contactapp.StoredContactPolicy, error) {
	queries, err := queriesFromContext(ctx)
	if repository == nil || err != nil {
		return contactapp.StoredContactPolicy{}, contactapp.ErrContactPolicyUnavailable
	}
	row, err := queries.InsertCustomerContactPolicy(ctx, contactdb.InsertCustomerContactPolicyParams{
		CustomerID: int64(value.CustomerID), ReasonCode: value.ReasonCode,
		SuppressedUntil: nullablePolicyTime(value.SuppressedUntil),
		ChangedAt:       pgtype.Timestamptz{Time: value.CreatedAt.UTC(), Valid: true},
	})
	if err != nil {
		return contactapp.StoredContactPolicy{}, mapContactPolicyDatabaseError(err)
	}
	return storedContactPolicy(row), nil
}

func (repository *ContactPolicyRepository) UpdateContactPolicy(ctx context.Context, value contactapp.StoredContactPolicy, expectedVersion int64) (contactapp.StoredContactPolicy, error) {
	queries, err := queriesFromContext(ctx)
	if repository == nil || err != nil {
		return contactapp.StoredContactPolicy{}, contactapp.ErrContactPolicyUnavailable
	}
	row, err := queries.UpdateCustomerContactPolicy(ctx, contactdb.UpdateCustomerContactPolicyParams{
		CustomerID: int64(value.CustomerID), ExpectedVersion: expectedVersion,
		ReasonCode: value.ReasonCode, SuppressedUntil: nullablePolicyTime(value.SuppressedUntil),
		ChangedAt: pgtype.Timestamptz{Time: value.UpdatedAt.UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.StoredContactPolicy{}, contactapp.ErrContactPolicyConflict
	}
	if err != nil {
		return contactapp.StoredContactPolicy{}, mapContactPolicyDatabaseError(err)
	}
	return storedContactPolicy(row), nil
}

func (repository *ContactPolicyRepository) DeleteContactPolicy(ctx context.Context, customerID contactport.CustomerID, expectedVersion int64) (bool, error) {
	queries, err := queriesFromContext(ctx)
	if repository == nil || err != nil || customerID <= 0 || expectedVersion <= 0 {
		return false, contactapp.ErrContactPolicyUnavailable
	}
	count, err := queries.DeleteCustomerContactPolicy(ctx, contactdb.DeleteCustomerContactPolicyParams{
		CustomerID: int64(customerID), ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return false, mapContactPolicyDatabaseError(err)
	}
	return count == 1, nil
}

func (repository *ContactPolicyRepository) CheckContactEligibility(ctx context.Context, check contactport.ContactEligibilityCheck) ([]contactport.ContactEligibility, error) {
	if repository == nil || ctx == nil || ctx.Err() != nil || check.EvaluatedAt.IsZero() ||
		(check.Checkpoint != contactport.ContactEligibilityPreview && check.Checkpoint != contactport.ContactEligibilityDispatch) ||
		len(check.CustomerIDs) == 0 || len(check.CustomerIDs) > contactport.ContactEligibilityMaximumCustomers {
		return nil, contactport.ErrInvalidContactEligibility
	}
	ids := append([]contactport.CustomerID(nil), check.CustomerIDs...)
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for index, id := range ids {
		if id <= 0 || index > 0 && id == ids[index-1] {
			return nil, contactport.ErrInvalidContactEligibility
		}
	}
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return nil, contactport.ErrContactEligibilityUnavailable
	}
	numericIDs := make([]int64, len(ids))
	for index, id := range ids {
		numericIDs[index] = int64(id)
	}
	if err = queries.LockCustomerContactPolicyKeys(ctx, numericIDs); err != nil {
		return nil, contactport.ErrContactEligibilityUnavailable
	}
	rows, err := queries.LockActiveCustomerContactPolicies(ctx, numericIDs)
	if err != nil {
		return nil, contactport.ErrContactEligibilityUnavailable
	}
	result := make([]contactport.ContactEligibility, len(ids))
	rowIndex := 0
	for index, customerID := range numericIDs {
		result[index] = contactport.ContactEligibility{
			CustomerID: contactport.CustomerID(customerID),
			Exclusion:  contactport.ContactEligibilityExclusionInactiveCustomer,
		}
		if rowIndex >= len(rows) || rows[rowIndex].CustomerID != customerID {
			continue
		}
		row := rows[rowIndex]
		rowIndex++
		eligible := !row.PolicyCustomerID.Valid || row.SuppressedUntil.Valid && !row.SuppressedUntil.Time.After(check.EvaluatedAt.UTC())
		exclusion := contactport.ContactEligibilityExclusionNone
		if !eligible {
			exclusion = contactport.ContactEligibilityExclusionContactPolicy
		}
		result[index] = contactport.ContactEligibility{
			CustomerID: contactport.CustomerID(customerID), CustomerActive: true,
			Eligible: eligible, Exclusion: exclusion,
		}
	}
	if rowIndex != len(rows) {
		return nil, contactport.ErrContactEligibilityUnavailable
	}
	for index := 1; index < len(result); index++ {
		if result[index-1].CustomerID >= result[index].CustomerID {
			return nil, contactport.ErrContactEligibilityUnavailable
		}
	}
	return result, nil
}

func readStoredContactPolicy(ctx context.Context, queries *contactdb.Queries, customerID contactport.CustomerID) (contactapp.StoredContactPolicy, bool, error) {
	row, err := queries.GetCustomerContactPolicy(ctx, int64(customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.StoredContactPolicy{}, false, nil
	}
	if err != nil {
		return contactapp.StoredContactPolicy{}, false, mapContactPolicyDatabaseError(err)
	}
	return storedContactPolicy(row), true, nil
}

func storedContactPolicy(row contactdb.CustomerContactPolicy) contactapp.StoredContactPolicy {
	value := contactapp.StoredContactPolicy{
		CustomerID: contactport.CustomerID(row.CustomerID), ReasonCode: row.ReasonCode,
		Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.SuppressedUntil.Valid {
		until := row.SuppressedUntil.Time.UTC()
		value.SuppressedUntil = &until
	}
	return value
}

func storedContactPolicyProjection(stored contactapp.StoredContactPolicy, evaluatedAt time.Time) contactapp.ContactPolicy {
	active := stored.SuppressedUntil == nil || stored.SuppressedUntil.After(evaluatedAt)
	reason := stored.ReasonCode
	result := contactapp.ContactPolicy{
		CustomerID: stored.CustomerID, Version: stored.Version, PolicyPresent: true,
		Eligible: !active, SuppressionActive: active, ReasonCode: &reason, LocalOnly: true,
	}
	if stored.SuppressedUntil != nil {
		until := stored.SuppressedUntil.UTC()
		result.SuppressedUntil = &until
	}
	return result
}

func nullablePolicyTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func contactPolicyReceipt(id int64, operation, actorScope string, keyDigest, payloadDigest []byte, state string, snapshot []byte) contactapp.ContactPolicyReceipt {
	value := contactapp.ContactPolicyReceipt{
		ID: id, Operation: operation, ActorScope: actorScope, State: state,
		ResultSnapshot: append([]byte(nil), snapshot...),
	}
	copy(value.KeyDigest[:], keyDigest)
	copy(value.PayloadDigest[:], payloadDigest)
	return value
}

func mapContactPolicyDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23505" || postgresError.Code == "40001" || postgresError.Code == "40P01") {
		return errors.Join(contactapp.ErrContactPolicyConflict, err)
	}
	return errors.Join(contactapp.ErrContactPolicyUnavailable, err)
}
