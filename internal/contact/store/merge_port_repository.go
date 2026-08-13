package store

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

const (
	maximumIdentityCustomerName = 500
	maximumMergeReason          = 1000
	maximumExternalEventType    = 200
	maximumExternalEventKey     = 200
)

// MergePortRepository implements the contact-owned create and pairwise merge
// half of MergePort plus its external-event append boundary. Every method here
// requires the transaction context supplied by the caller's UnitOfWork and
// never opens a nested transaction.
type MergePortRepository struct{}

type retryableMergeStoreError struct {
	cause error
}

func (err retryableMergeStoreError) Error() string {
	return contactport.ErrMergeStoreFailed.Error()
}

func (err retryableMergeStoreError) Unwrap() []error {
	return []error{contactport.ErrMergeStoreFailed, err.cause}
}

func NewMergePortRepository() *MergePortRepository {
	return &MergePortRepository{}
}

func (repository *MergePortRepository) CreateForIdentity(
	ctx context.Context,
	command contactport.CreateForIdentityCommand,
) (contactport.CustomerID, error) {
	if repository == nil || !validCreateForIdentityCommand(command) {
		return 0, contactport.ErrInvalidMergeCommand
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return 0, err
	}
	id, err := queries.CreateCustomerForIdentity(ctx, contactdb.CreateCustomerForIdentityParams{
		Name:         command.Name,
		OwnerStaffID: nullableInt64(command.OwnerStaffID),
		ChannelID:    nullableInt64(command.ChannelID),
	})
	if err != nil {
		return 0, mapMergePortDatabaseError(err)
	}
	if id <= 0 {
		return 0, contactport.ErrMergeStoreFailed
	}
	return contactport.CustomerID(id), nil
}

func (repository *MergePortRepository) MergeCustomers(
	ctx context.Context,
	command contactport.MergeCustomersCommand,
) error {
	if repository == nil || !validMergeCustomersCommand(command) {
		return contactport.ErrInvalidMergeCommand
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return err
	}

	rows, err := queries.LockCustomersForMerge(ctx, []int64{
		int64(command.PrimaryID), int64(command.MergedID),
	})
	if err != nil {
		return mapMergePortDatabaseError(err)
	}
	if len(rows) != 2 {
		return contactport.ErrMergeCustomerNotFound
	}
	states := make(map[int64]bool, len(rows))
	for _, row := range rows {
		states[row.ID] = row.IsDeleted
	}
	primaryDeleted, primaryFound := states[int64(command.PrimaryID)]
	mergedDeleted, mergedFound := states[int64(command.MergedID)]
	if !primaryFound || !mergedFound {
		return contactport.ErrMergeCustomerNotFound
	}
	if mergedDeleted {
		return replaySameDirectionMerge(ctx, queries, command)
	}
	if primaryDeleted {
		return contactport.ErrMergeConflict
	}

	if _, err = queries.CopyCustomerTagsForMerge(ctx, contactdb.CopyCustomerTagsForMergeParams{
		PrimaryCustomerID: int64(command.PrimaryID),
		MergedCustomerID:  int64(command.MergedID),
	}); err != nil {
		return mapMergePortDatabaseError(err)
	}
	changed, err := queries.MarkCustomerMerged(ctx, int64(command.MergedID))
	if err != nil {
		return mapMergePortDatabaseError(err)
	}
	if changed != 1 {
		return contactport.ErrMergeConflict
	}
	inserted, err := queries.InsertCustomerMergeLineage(ctx, contactdb.InsertCustomerMergeLineageParams{
		MergedCustomerID:  int64(command.MergedID),
		PrimaryCustomerID: int64(command.PrimaryID),
		Actor:             string(command.Actor),
		Reason:            command.Reason,
	})
	if err != nil {
		return mapMergePortDatabaseError(err)
	}
	if inserted != 1 {
		return contactport.ErrMergeConflict
	}
	return nil
}

func (repository *MergePortRepository) AppendExternalEvent(
	ctx context.Context,
	command contactport.ExternalEventCommand,
) (contactport.EventID, error) {
	if repository == nil || !validExternalEventCommand(command) {
		return 0, contactport.ErrInvalidMergeCommand
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return 0, err
	}
	command.OccurredAt = command.OccurredAt.UTC().Truncate(time.Microsecond)
	if err = queries.LockExternalEventIdempotencyKey(ctx, command.IdempotencyKey); err != nil {
		return 0, mapMergePortDatabaseError(err)
	}
	effectiveID, err := resolveEffectiveCustomerRoot(ctx, queries, command.CustomerID)
	if err != nil {
		return 0, err
	}
	existing, err := queries.GetExternalEventIdempotency(ctx, command.IdempotencyKey)
	if err == nil {
		return replayExternalEvent(ctx, queries, effectiveID, command, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, mapMergePortDatabaseError(err)
	}

	eventID, err := queries.AppendCustomerEvent(ctx, contactdb.AppendCustomerEventParams{
		CustomerID: int64(effectiveID),
		EventType:  command.EventType,
		Payload:    append([]byte(nil), command.Payload...),
		Actor:      string(command.Actor),
		OccurredAt: pgtype.Timestamptz{Time: command.OccurredAt, Valid: true},
	})
	if err != nil {
		return 0, mapMergePortDatabaseError(err)
	}
	inserted, err := queries.InsertExternalEventIdempotency(ctx, contactdb.InsertExternalEventIdempotencyParams{
		IdempotencyKey:  command.IdempotencyKey,
		EventOccurredAt: pgtype.Timestamptz{Time: command.OccurredAt, Valid: true},
		EventID:         eventID,
		EventCustomerID: int64(effectiveID),
		EventType:       command.EventType,
		Payload:         append([]byte(nil), command.Payload...),
		Actor:           string(command.Actor),
	})
	if err != nil {
		return 0, mapMergePortDatabaseError(err)
	}
	if inserted != 1 || eventID <= 0 {
		return 0, contactport.ErrExternalEventConflict
	}
	return contactport.EventID(eventID), nil
}

func replaySameDirectionMerge(
	ctx context.Context,
	queries *contactdb.Queries,
	command contactport.MergeCustomersCommand,
) error {
	directParent, err := queries.GetCustomerMergeLineage(ctx, int64(command.MergedID))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && directParent != int64(command.PrimaryID)) {
		return contactport.ErrMergeConflict
	}
	if err != nil {
		return mapMergePortDatabaseError(err)
	}
	primaryRoot, err := resolveEffectiveCustomerRoot(ctx, queries, command.PrimaryID)
	if err != nil {
		return err
	}
	mergedRoot, err := resolveEffectiveCustomerRoot(ctx, queries, command.MergedID)
	if err != nil {
		return err
	}
	if primaryRoot != mergedRoot {
		return contactport.ErrMergeConflict
	}
	return nil
}

func resolveEffectiveCustomerRoot(
	ctx context.Context,
	queries *contactdb.Queries,
	customerID contactport.CustomerID,
) (contactport.CustomerID, error) {
	root, err := queries.ResolveEffectiveCustomerRoot(ctx, int64(customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, contactport.ErrMergeCustomerNotFound
	}
	if err != nil {
		return 0, mapMergePortDatabaseError(err)
	}
	if root <= 0 {
		return 0, contactport.ErrMergeConflict
	}
	return contactport.CustomerID(root), nil
}

func replayExternalEvent(
	ctx context.Context,
	queries *contactdb.Queries,
	effectiveID contactport.CustomerID,
	command contactport.ExternalEventCommand,
	existing contactdb.GetExternalEventIdempotencyRow,
) (contactport.EventID, error) {
	existingRoot, err := resolveEffectiveCustomerRoot(
		ctx,
		queries,
		contactport.CustomerID(existing.EventCustomerID),
	)
	if err != nil {
		return 0, err
	}
	if existing.EventID <= 0 || !existing.EventOccurredAt.Valid || existingRoot != effectiveID ||
		existing.EventType != command.EventType || existing.Actor != string(command.Actor) ||
		!existing.EventOccurredAt.Time.Equal(command.OccurredAt) ||
		!sameJSONObject(existing.Payload, command.Payload) {
		return 0, contactport.ErrExternalEventConflict
	}
	return contactport.EventID(existing.EventID), nil
}

func validCreateForIdentityCommand(command contactport.CreateForIdentityCommand) bool {
	return len(command.Name) <= maximumIdentityCustomerName && utf8.ValidString(command.Name) &&
		validCustomerMutationActor(command.Actor) && positiveOptionalID(command.OwnerStaffID) &&
		positiveOptionalID(command.ChannelID)
}

func validMergeCustomersCommand(command contactport.MergeCustomersCommand) bool {
	return command.PrimaryID > 0 && command.MergedID > 0 && command.PrimaryID != command.MergedID &&
		validCustomerMutationActor(command.Actor) && validTrimmedMergeText(command.Reason, maximumMergeReason)
}

func validExternalEventCommand(command contactport.ExternalEventCommand) bool {
	return command.CustomerID > 0 &&
		validTrimmedExternalText(command.EventType, maximumExternalEventType) &&
		validCustomerMutationActor(command.Actor) &&
		!command.OccurredAt.IsZero() &&
		validCustomerMutationJSONObject(command.Payload) &&
		validTrimmedExternalText(command.IdempotencyKey, maximumExternalEventKey)
}

func validTrimmedExternalText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value)
}

func validTrimmedMergeText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value)
}

func positiveOptionalID(value *int64) bool {
	return value == nil || *value > 0
}

func mapMergePortDatabaseError(err error) error {
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		return contactport.ErrMergeStoreFailed
	}
	switch databaseError.Code {
	case "40001", "40P01":
		return retryableMergeStoreError{cause: err}
	case "23503":
		return contactport.ErrMergeCustomerNotFound
	case "23505", "23514":
		return contactport.ErrMergeConflict
	case "22001", "22003", "22P02":
		return contactport.ErrInvalidMergeCommand
	default:
		return contactport.ErrMergeStoreFailed
	}
}
