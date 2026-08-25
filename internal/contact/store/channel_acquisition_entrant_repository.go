package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

// ChannelAcquisitionEntrantRepository owns the durable CH03 receipt.  Its
// caller already owns the UnitOfWork: all checks, event append and receipt
// transition occur on that same transaction-bound query handle.
type ChannelAcquisitionEntrantRepository struct{}

var _ contactport.ChannelAcquisitionEntrantRecorder = (*ChannelAcquisitionEntrantRepository)(nil)

func NewChannelAcquisitionEntrantRepository() *ChannelAcquisitionEntrantRepository {
	return &ChannelAcquisitionEntrantRepository{}
}

func (repository *ChannelAcquisitionEntrantRepository) FindTerminalChannelAcquisitionEntrant(ctx context.Context, inboxID int64, inputDigest string) (contactport.ChannelAcquisitionEntrantReceipt, bool, error) {
	if repository == nil || inboxID < 1 || !validEntrantDigest(inputDigest) {
		return contactport.ChannelAcquisitionEntrantReceipt{}, false, contactport.ErrExternalEventConflict
	}
	queries, err := channelQueries(ctx)
	if err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, false, err
	}
	existing, err := queries.LockChannelAcquisitionEntrantReceipt(ctx, inboxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.ChannelAcquisitionEntrantReceipt{}, false, nil
	}
	if err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, false, err
	}
	if existing.InputDigest != inputDigest {
		return contactport.ChannelAcquisitionEntrantReceipt{}, false, contactport.ErrExternalEventConflict
	}
	if existing.Status != string(contactport.ChannelAcquisitionEntrantAttributed) && existing.Status != string(contactport.ChannelAcquisitionEntrantReconciled) {
		return contactport.ChannelAcquisitionEntrantReceipt{}, false, nil
	}
	receipt, err := entrantReceipt(existing.ID, existing.InboxID, existing.InputDigest, existing.Status, existing.EffectID, existing.ChannelID, existing.AssetKind, existing.AssetVersion, existing.CustomerID, existing.CustomerEventID, existing.OccurredAt)
	if err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, false, err
	}
	return receipt, true, nil
}

func (repository *ChannelAcquisitionEntrantRepository) RecordChannelAcquisitionEntrant(ctx context.Context, command contactport.ChannelAcquisitionEntrantCommand) (contactport.ChannelAcquisitionEntrantReceipt, error) {
	if repository == nil || !validEntrantCommand(command) {
		return contactport.ChannelAcquisitionEntrantReceipt{}, contactport.ErrExternalEventConflict
	}
	queries, err := channelQueries(ctx)
	if err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, err
	}
	if err = queries.LockChannelAcquisitionEntrantReceiptKey(ctx, strconv.FormatInt(command.InboxID, 10)); err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, err
	}
	existing, err := queries.LockChannelAcquisitionEntrantReceipt(ctx, command.InboxID)
	if err == nil {
		return repository.replayOrTransition(ctx, queries, command, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactport.ChannelAcquisitionEntrantReceipt{}, err
	}
	if err = validateEntrantBinding(ctx, queries, command); err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, err
	}
	eventID, eventAt, err := appendEntrantEvent(ctx, queries, command)
	if err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, err
	}
	row, err := queries.InsertChannelAcquisitionEntrantReceipt(ctx, entrantInsertParams(command, eventID, eventAt))
	if err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, err
	}
	return entrantReceipt(row.ID, row.InboxID, row.InputDigest, row.Status, row.EffectID, row.ChannelID, row.AssetKind, row.AssetVersion, row.CustomerID, row.CustomerEventID, row.OccurredAt)
}

func (repository *ChannelAcquisitionEntrantRepository) replayOrTransition(ctx context.Context, queries *contactdb.Queries, command contactport.ChannelAcquisitionEntrantCommand, existing contactdb.LockChannelAcquisitionEntrantReceiptRow) (contactport.ChannelAcquisitionEntrantReceipt, error) {
	if existing.InputDigest != command.InputDigest {
		return contactport.ChannelAcquisitionEntrantReceipt{}, contactport.ErrExternalEventConflict
	}
	if err := validateEntrantBinding(ctx, queries, command); err != nil {
		return contactport.ChannelAcquisitionEntrantReceipt{}, err
	}
	if existing.Status == string(contactport.ChannelAcquisitionEntrantPendingIdentity) && command.Status == contactport.ChannelAcquisitionEntrantAttributed {
		eventID, eventAt, err := appendEntrantEvent(ctx, queries, command)
		if err != nil {
			return contactport.ChannelAcquisitionEntrantReceipt{}, err
		}
		row, err := queries.TransitionChannelAcquisitionEntrantPending(ctx, contactdb.TransitionChannelAcquisitionEntrantPendingParams{
			CustomerID: int64(command.CustomerID), CustomerEventID: int64(eventID), CustomerEventOccurredAt: timestamp(eventAt), InboxID: command.InboxID, InputDigest: command.InputDigest,
		})
		if err != nil {
			return contactport.ChannelAcquisitionEntrantReceipt{}, err
		}
		return entrantReceipt(row.ID, row.InboxID, row.InputDigest, row.Status, row.EffectID, row.ChannelID, row.AssetKind, row.AssetVersion, row.CustomerID, row.CustomerEventID, row.OccurredAt)
	}
	receipt, err := entrantReceipt(existing.ID, existing.InboxID, existing.InputDigest, existing.Status, existing.EffectID, existing.ChannelID, existing.AssetKind, existing.AssetVersion, existing.CustomerID, existing.CustomerEventID, existing.OccurredAt)
	if err != nil || !sameEntrantReplay(receipt, command) {
		return contactport.ChannelAcquisitionEntrantReceipt{}, contactport.ErrExternalEventConflict
	}
	return receipt, nil
}

func validateEntrantBinding(ctx context.Context, queries *contactdb.Queries, command contactport.ChannelAcquisitionEntrantCommand) error {
	if !entrantStatusNeedsBinding(command.Status) {
		return nil
	}
	effectID, err := channelAcquisitionEffectID(command.Match.EffectID)
	if err != nil {
		return contactport.ErrExternalEventConflict
	}
	row, err := queries.LockChannelAcquisitionEntrantBinding(ctx, contactdb.LockChannelAcquisitionEntrantBindingParams{EffectID: effectID, ChannelID: command.Match.ChannelID, AssetKind: string(command.Match.Kind), AssetVersion: command.Match.AssetVersion})
	if err != nil || !row.CorpID.Valid || !row.CorrelationKey.Valid || row.CorpID.String != command.CorpID || row.CorrelationKey.String != command.CallbackState || !containsExact(row.AssigneeWecomUserids, command.WeComUserID) || !row.PublishedAt.Valid || row.PublishedAt.Time.After(command.OccurredAt.UTC()) {
		return contactport.ErrExternalEventConflict
	}
	return nil
}

func appendEntrantEvent(ctx context.Context, queries *contactdb.Queries, command contactport.ChannelAcquisitionEntrantCommand) (contactport.EventID, time.Time, error) {
	if command.Status != contactport.ChannelAcquisitionEntrantAttributed {
		return 0, time.Time{}, nil
	}
	// A receipt key is already locked, making the append + receipt write one
	// atomic effect. The payload contains no external identity or callback data.
	eventAt := command.OccurredAt.UTC().Truncate(time.Microsecond)
	payload, _ := json.Marshal(map[string]any{"asset_version": command.Match.AssetVersion, "channel_id": command.Match.ChannelID, "effect_id": command.Match.EffectID, "inbox_id": command.InboxID})
	id, err := queries.AppendCustomerEvent(ctx, contactdb.AppendCustomerEventParams{CustomerID: int64(command.CustomerID), EventType: "channel.acquisition.entrant", Payload: payload, Actor: "wecom.callback", OccurredAt: timestamp(eventAt)})
	if err != nil || id < 1 {
		return 0, time.Time{}, contactport.ErrExternalEventConflict
	}
	return contactport.EventID(id), eventAt, nil
}

func entrantInsertParams(command contactport.ChannelAcquisitionEntrantCommand, eventID contactport.EventID, eventAt time.Time) contactdb.InsertChannelAcquisitionEntrantReceiptParams {
	params := contactdb.InsertChannelAcquisitionEntrantReceiptParams{InboxID: command.InboxID, InputDigest: command.InputDigest, Status: string(command.Status), OccurredAt: timestamp(command.OccurredAt), ReconcileReason: ""}
	if entrantStatusNeedsBinding(command.Status) {
		effectID, _ := channelAcquisitionEffectID(command.Match.EffectID)
		params.EffectID, params.ChannelID = int8v(effectID), int8v(command.Match.ChannelID)
		params.AssetKind, params.AssetVersion = textv(string(command.Match.Kind)), int8v(command.Match.AssetVersion)
	}
	if eventID > 0 {
		params.CustomerID, params.CustomerEventID, params.CustomerEventOccurredAt = int8v(int64(command.CustomerID)), int8v(int64(eventID)), timestamp(eventAt)
	}
	return params
}

func entrantReceipt(id, inboxID int64, digest, status string, effectID, channelID pgtype.Int8, kind pgtype.Text, version, customerID, eventID pgtype.Int8, occurredAt pgtype.Timestamptz) (contactport.ChannelAcquisitionEntrantReceipt, error) {
	if id < 1 || inboxID < 1 || !occurredAt.Valid || !contactport.ChannelAcquisitionEntrantStatus(status).Valid() {
		return contactport.ChannelAcquisitionEntrantReceipt{}, contactport.ErrExternalEventConflict
	}
	r := contactport.ChannelAcquisitionEntrantReceipt{ID: id, InboxID: inboxID, InputDigest: digest, Status: contactport.ChannelAcquisitionEntrantStatus(status), OccurredAt: occurredAt.Time.UTC()}
	if effectID.Valid || channelID.Valid || kind.Valid || version.Valid {
		if !effectID.Valid || !channelID.Valid || !kind.Valid || !version.Valid || effectID.Int64 < 1 || channelID.Int64 < 1 || version.Int64 < 1 {
			return contactport.ChannelAcquisitionEntrantReceipt{}, contactport.ErrExternalEventConflict
		}
		r.EffectID, r.ChannelID, r.Kind, r.AssetVersion = channelAcquisitionFormatEffectID(effectID.Int64), channelID.Int64, contactport.AcquisitionAssetKind(kind.String), version.Int64
	}
	if customerID.Valid || eventID.Valid {
		if !customerID.Valid || !eventID.Valid || customerID.Int64 < 1 || eventID.Int64 < 1 {
			return contactport.ChannelAcquisitionEntrantReceipt{}, contactport.ErrExternalEventConflict
		}
		r.CustomerID, r.CustomerEventID = contactport.CustomerID(customerID.Int64), contactport.EventID(eventID.Int64)
	}
	return r, nil
}

func validEntrantCommand(command contactport.ChannelAcquisitionEntrantCommand) bool {
	if command.InboxID < 1 || !validEntrantDBText(command.SourceKey, 512) || !validEntrantDigest(command.InputDigest) || !validEntrantDBText(command.CorpID, 128) || !command.Status.Valid() || command.OccurredAt.IsZero() {
		return false
	}
	if !entrantStatusNeedsBinding(command.Status) {
		return command.Match.EffectID == "" && command.CustomerID == 0
	}
	if _, err := channelAcquisitionEffectID(command.Match.EffectID); err != nil || command.Match.ChannelID < 1 || command.Match.AssetVersion < 1 || !validEntrantDBText(command.CallbackState, 512) || !validEntrantDBText(command.WeComUserID, 1024) {
		return false
	}
	return (command.Status == contactport.ChannelAcquisitionEntrantAttributed) == (command.CustomerID > 0)
}

func entrantStatusNeedsBinding(status contactport.ChannelAcquisitionEntrantStatus) bool {
	return status == contactport.ChannelAcquisitionEntrantCorrelated || status == contactport.ChannelAcquisitionEntrantAttributed || status == contactport.ChannelAcquisitionEntrantPendingIdentity || status == contactport.ChannelAcquisitionEntrantConflict || status == contactport.ChannelAcquisitionEntrantReconciled
}

func sameEntrantReplay(receipt contactport.ChannelAcquisitionEntrantReceipt, command contactport.ChannelAcquisitionEntrantCommand) bool {
	if receipt.Status == contactport.ChannelAcquisitionEntrantAttributed || receipt.Status == contactport.ChannelAcquisitionEntrantReconciled {
		return receipt.EffectID == command.Match.EffectID && receipt.ChannelID == command.Match.ChannelID && receipt.Kind == command.Match.Kind && receipt.AssetVersion == command.Match.AssetVersion &&
			receipt.CustomerEventID > 0 && (command.CustomerID == 0 || receipt.CustomerID == command.CustomerID)
	}
	return receipt.Status == command.Status && receipt.EffectID == command.Match.EffectID && receipt.ChannelID == command.Match.ChannelID && receipt.Kind == command.Match.Kind && receipt.AssetVersion == command.Match.AssetVersion && receipt.CustomerID == 0 && receipt.CustomerEventID == 0
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func validEntrantDBText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}
func validEntrantDigest(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[7:] {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
func int8v(value int64) pgtype.Int8  { return pgtype.Int8{Int64: value, Valid: true} }
func textv(value string) pgtype.Text { return pgtype.Text{String: value, Valid: true} }
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
