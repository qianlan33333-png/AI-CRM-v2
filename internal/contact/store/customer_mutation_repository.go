package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// CustomerMutationRepository persists contact-owned customer mutations through
// the transaction supplied by the caller's UnitOfWork.
type CustomerMutationRepository struct{}

var _ contactapp.CustomerMutationStore = (*CustomerMutationRepository)(nil)

func NewCustomerMutationRepository() *CustomerMutationRepository {
	return &CustomerMutationRepository{}
}

func (repository *CustomerMutationRepository) UpdateCustomer(
	ctx context.Context,
	command contactapp.CustomerUpdateCommand,
) (contactapp.CustomerProfileMutation, error) {
	if repository == nil {
		return contactapp.CustomerProfileMutation{}, contactapp.ErrCustomerMutationFailed
	}
	if err := validateCustomerUpdateCommand(command); err != nil {
		return contactapp.CustomerProfileMutation{}, err
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return contactapp.CustomerProfileMutation{}, err
	}

	locked, err := queries.LockActiveCustomerForMutation(ctx, int64(command.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerProfileMutation{}, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return contactapp.CustomerProfileMutation{}, mapCustomerMutationDatabaseError(err)
	}
	current := customerRecordFromRow(locked)
	if !customerUpdateChanges(current, command) {
		return contactapp.CustomerProfileMutation{Customer: current}, nil
	}

	row, err := queries.UpdateCustomer(ctx, updateCustomerParams(command))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerProfileMutation{}, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return contactapp.CustomerProfileMutation{}, mapCustomerMutationDatabaseError(err)
	}
	return contactapp.CustomerProfileMutation{
		Customer:    customerRecordFromRow(row),
		StateChange: true,
	}, nil
}

func (repository *CustomerMutationRepository) SetCustomerStage(
	ctx context.Context,
	command contactapp.CustomerStageCommand,
) (contactapp.CustomerStageMutation, error) {
	if repository == nil {
		return contactapp.CustomerStageMutation{}, contactapp.ErrCustomerMutationFailed
	}
	if err := validateCustomerStageCommand(command); err != nil {
		return contactapp.CustomerStageMutation{}, err
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return contactapp.CustomerStageMutation{}, err
	}

	locked, err := queries.LockActiveCustomerForMutation(ctx, int64(command.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerStageMutation{}, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return contactapp.CustomerStageMutation{}, mapCustomerMutationDatabaseError(err)
	}

	previousID := int64Pointer(locked.StageID)
	if sameNullableInt64(previousID, command.StageID) {
		return contactapp.CustomerStageMutation{
			Customer:   customerRecordFromRow(locked),
			PreviousID: previousID,
		}, nil
	}

	updated, err := queries.SetCustomerStage(ctx, contactdb.SetCustomerStageParams{
		CustomerID: int64(command.ID),
		StageID:    nullableInt64(command.StageID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerStageMutation{}, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return contactapp.CustomerStageMutation{}, mapCustomerMutationDatabaseError(err)
	}
	return contactapp.CustomerStageMutation{
		Customer:    customerRecordFromRow(updated),
		PreviousID:  previousID,
		StateChange: true,
	}, nil
}

func (repository *CustomerMutationRepository) AddCustomerTag(
	ctx context.Context,
	command contactapp.CustomerTagCommand,
) (bool, error) {
	if repository == nil {
		return false, contactapp.ErrCustomerMutationFailed
	}
	if err := validateCustomerTagCommand(command); err != nil {
		return false, err
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return false, err
	}
	if err := lockCustomerAndValidateTag(ctx, queries, command); err != nil {
		return false, err
	}

	changed, err := queries.AddCustomerTag(ctx, contactdb.AddCustomerTagParams{
		CustomerID: int64(command.ID),
		TagID:      command.TagID,
		TaggedBy:   string(command.Actor),
	})
	if err != nil {
		return false, mapCustomerMutationDatabaseError(err)
	}
	return changed == 1, nil
}

func (repository *CustomerMutationRepository) RemoveCustomerTag(
	ctx context.Context,
	command contactapp.CustomerTagCommand,
) (bool, error) {
	if repository == nil {
		return false, contactapp.ErrCustomerMutationFailed
	}
	if err := validateCustomerTagCommand(command); err != nil {
		return false, err
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return false, err
	}
	if err := lockCustomerAndValidateTag(ctx, queries, command); err != nil {
		return false, err
	}

	changed, err := queries.RemoveCustomerTag(ctx, contactdb.RemoveCustomerTagParams{
		CustomerID: int64(command.ID),
		TagID:      command.TagID,
	})
	if err != nil {
		return false, mapCustomerMutationDatabaseError(err)
	}
	return changed == 1, nil
}

func (repository *CustomerMutationRepository) AppendCustomerEvent(
	ctx context.Context,
	command contactapp.CustomerEventAppend,
) (contactport.EventID, error) {
	if repository == nil {
		return 0, contactapp.ErrCustomerMutationFailed
	}
	if err := validateCustomerEventAppend(command); err != nil {
		return 0, err
	}
	queries, err := customerMutationQueriesFromContext(ctx)
	if err != nil {
		return 0, err
	}

	id, err := queries.AppendCustomerEvent(ctx, contactdb.AppendCustomerEventParams{
		CustomerID: int64(command.CustomerID),
		EventType:  command.EventType,
		Payload:    append([]byte(nil), command.Payload...),
		Actor:      string(command.Actor),
		OccurredAt: pgtype.Timestamptz{Time: command.OccurredAt.UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return 0, mapCustomerMutationDatabaseError(err)
	}
	return contactport.EventID(id), nil
}

func lockCustomerAndValidateTag(
	ctx context.Context,
	queries contactdb.Querier,
	command contactapp.CustomerTagCommand,
) error {
	if _, err := queries.LockActiveCustomerForMutation(ctx, int64(command.ID)); errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ErrCustomerNotFound
	} else if err != nil {
		return mapCustomerMutationDatabaseError(err)
	}
	if _, err := queries.GetCustomerTag(ctx, command.TagID); errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ErrCustomerTagNotFound
	} else if err != nil {
		return mapCustomerMutationDatabaseError(err)
	}
	return nil
}

func customerMutationQueriesFromContext(ctx context.Context) (*contactdb.Queries, error) {
	if isNilCustomerMutationValue(ctx) {
		return nil, platformport.ErrTransactionRequired
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if isNilCustomerMutationValue(tx) {
		return nil, platformport.ErrTransactionRequired
	}
	return contactdb.New(tx), nil
}

func updateCustomerParams(command contactapp.CustomerUpdateCommand) contactdb.UpdateCustomerParams {
	params := contactdb.UpdateCustomerParams{
		NameSet:         command.Name != nil,
		Name:            nullableText(command.Name),
		AvatarUrlSet:    command.AvatarURL.Set,
		AvatarUrl:       nullablePatchText(command.AvatarURL),
		GenderSet:       command.Gender.Set,
		Gender:          nullablePatchInt16(command.Gender),
		OwnerStaffIDSet: command.OwnerStaffID.Set,
		OwnerStaffID:    nullablePatchInt64(command.OwnerStaffID),
		ChannelIDSet:    command.ChannelID.Set,
		ChannelID:       nullablePatchInt64(command.ChannelID),
		CustomerID:      int64(command.ID),
	}
	if command.Extra != nil {
		params.ExtraSet = true
		params.Extra = append([]byte(nil), (*command.Extra)...)
	}
	return params
}

func nullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func nullablePatchText(value contactapp.NullablePatch[string]) pgtype.Text {
	if !value.Set {
		return pgtype.Text{}
	}
	return nullableText(value.Value)
}

func nullableInt16(value *int16) pgtype.Int2 {
	if value == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: *value, Valid: true}
}

func nullablePatchInt16(value contactapp.NullablePatch[int16]) pgtype.Int2 {
	if !value.Set {
		return pgtype.Int2{}
	}
	return nullableInt16(value.Value)
}

func nullablePatchInt64(value contactapp.NullablePatch[int64]) pgtype.Int8 {
	if !value.Set {
		return pgtype.Int8{}
	}
	return nullableInt64(value.Value)
}

func sameNullableInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func customerUpdateChanges(current contactapp.CustomerRecord, command contactapp.CustomerUpdateCommand) bool {
	if command.Name != nil && current.Name != *command.Name {
		return true
	}
	if command.AvatarURL.Set && !sameNullableString(current.AvatarURL, command.AvatarURL.Value) {
		return true
	}
	if command.Gender.Set && !sameNullableInt16(current.Gender, command.Gender.Value) {
		return true
	}
	if command.OwnerStaffID.Set && !sameNullableInt64(current.OwnerStaffID, command.OwnerStaffID.Value) {
		return true
	}
	if command.ChannelID.Set && !sameNullableInt64(current.ChannelID, command.ChannelID.Value) {
		return true
	}
	return command.Extra != nil && !sameJSONObject(current.Extra, *command.Extra)
}

func sameNullableString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameNullableInt16(left, right *int16) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameJSONObject(left, right json.RawMessage) bool {
	leftValue, leftOK := decodeJSONObjectWithExactNumbers(left)
	rightValue, rightOK := decodeJSONObjectWithExactNumbers(right)
	return leftOK && rightOK && reflect.DeepEqual(leftValue, rightValue)
}

func decodeJSONObjectWithExactNumbers(raw json.RawMessage) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return value, true
}

func mapCustomerMutationDatabaseError(err error) error {
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		return contactapp.ErrCustomerMutationFailed
	}
	switch databaseError.Code {
	case "23503":
		if strings.Contains(databaseError.ConstraintName, "_stage_id_") {
			return contactport.ErrStageNotFound
		}
		if strings.Contains(databaseError.ConstraintName, "_tag_id_") {
			return contactapp.ErrCustomerTagNotFound
		}
		if strings.Contains(databaseError.ConstraintName, "_customer_id_") {
			return contactapp.ErrCustomerNotFound
		}
		return contactapp.ErrCustomerConflict
	case "23505":
		return contactapp.ErrCustomerConflict
	case "22001", "22003", "22P02", "23514":
		return contactapp.ErrInvalidCustomerMutation
	default:
		return contactapp.ErrCustomerMutationFailed
	}
}

func validateCustomerUpdateCommand(command contactapp.CustomerUpdateCommand) error {
	if command.ID <= 0 || !validCustomerMutationActor(command.Actor) ||
		(command.Name == nil && !command.AvatarURL.Set && !command.Gender.Set &&
			!command.OwnerStaffID.Set && !command.ChannelID.Set && command.Extra == nil) {
		return contactapp.ErrInvalidCustomerMutation
	}
	if command.Name != nil && !utf8.ValidString(*command.Name) {
		return contactapp.ErrInvalidCustomerMutation
	}
	if command.AvatarURL.Set && command.AvatarURL.Value != nil && !validCustomerMutationAvatarURL(*command.AvatarURL.Value) {
		return contactapp.ErrInvalidCustomerMutation
	}
	for _, value := range []*int64{command.OwnerStaffID.Value, command.ChannelID.Value} {
		if value != nil && *value <= 0 {
			return contactapp.ErrInvalidCustomerMutation
		}
	}
	if command.Extra != nil && !validCustomerMutationJSONObject(*command.Extra) {
		return contactapp.ErrInvalidCustomerMutation
	}
	return nil
}

func validateCustomerStageCommand(command contactapp.CustomerStageCommand) error {
	if command.ID <= 0 || !validCustomerMutationActor(command.Actor) ||
		(command.StageID != nil && *command.StageID <= 0) {
		return contactapp.ErrInvalidCustomerMutation
	}
	return nil
}

func validateCustomerTagCommand(command contactapp.CustomerTagCommand) error {
	if command.ID <= 0 || command.TagID <= 0 || !validCustomerMutationActor(command.Actor) {
		return contactapp.ErrInvalidCustomerMutation
	}
	return nil
}

func validateCustomerEventAppend(command contactapp.CustomerEventAppend) error {
	if command.CustomerID <= 0 || !validCustomerMutationActor(command.Actor) ||
		strings.TrimSpace(command.EventType) != command.EventType || command.EventType == "" ||
		command.OccurredAt.IsZero() || !validCustomerMutationJSONObject(command.Payload) {
		return contactapp.ErrInvalidCustomerMutation
	}
	return nil
}

func validCustomerMutationActor(actor contactport.Actor) bool {
	value := string(actor)
	return value != "" && len(value) <= 200 && strings.TrimSpace(value) == value && utf8.ValidString(value)
}

func validCustomerMutationAvatarURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != "" && parsed.User == nil
}

func validCustomerMutationJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(trimmed)
}

func isNilCustomerMutationValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
