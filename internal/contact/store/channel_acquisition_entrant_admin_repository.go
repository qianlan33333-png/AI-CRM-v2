package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

type ChannelAcquisitionEntrantReceiptRepository struct{}

func NewChannelAcquisitionEntrantReceiptRepository() *ChannelAcquisitionEntrantReceiptRepository {
	return &ChannelAcquisitionEntrantReceiptRepository{}
}

func (repository *ChannelAcquisitionEntrantReceiptRepository) ListChannelAcquisitionEntrantReceipts(ctx context.Context, actorID, channelID int64, limit int, after int64) ([]contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	queries, err := channelQueries(ctx)
	if err != nil || actorID < 1 || channelID < 1 || limit < 1 {
		return nil, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	rows, err := queries.ListAdminChannelAcquisitionEntrantReceipts(ctx, contactdb.ListAdminChannelAcquisitionEntrantReceiptsParams{ActorID: actorID, ChannelID: channelID, AfterReceiptID: after, PageLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	items := make([]contactapp.ChannelAcquisitionEntrantReceiptItem, 0, len(rows))
	for _, row := range rows {
		item, err := entrantReceiptItem(row.ID, row.ChannelID, row.EffectID, row.AssetKind, row.AssetVersion, row.Status, row.CustomerID, row.CustomerEventID, row.OccurredAt, row.ReconciledAt, row.ReconcileReason, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (repository *ChannelAcquisitionEntrantReceiptRepository) GetChannelAcquisitionEntrantReceipt(ctx context.Context, actorID, channelID, receiptID int64) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	queries, err := channelQueries(ctx)
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	row, err := queries.GetAdminChannelAcquisitionEntrantReceipt(ctx, contactdb.GetAdminChannelAcquisitionEntrantReceiptParams{ActorID: actorID, ChannelID: channelID, ReceiptID: receiptID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	}
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	return entrantReceiptItem(row.ID, row.ChannelID, row.EffectID, row.AssetKind, row.AssetVersion, row.Status, row.CustomerID, row.CustomerEventID, row.OccurredAt, row.ReconciledAt, row.ReconcileReason, row.CreatedAt, row.UpdatedAt)
}

func (repository *ChannelAcquisitionEntrantReceiptRepository) ListUnassignedChannelAcquisitionEntrantReceipts(ctx context.Context, actorID int64, limit int, after int64) ([]contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	queries, err := channelQueries(ctx)
	if err != nil || actorID < 1 || limit < 1 {
		return nil, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	rows, err := queries.ListAdminUnassignedChannelAcquisitionEntrantReceipts(ctx, contactdb.ListAdminUnassignedChannelAcquisitionEntrantReceiptsParams{ActorID: actorID, AfterReceiptID: after, PageLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	items := make([]contactapp.ChannelAcquisitionEntrantReceiptItem, 0, len(rows))
	for _, row := range rows {
		item, err := entrantReceiptItem(row.ID, row.ChannelID, row.EffectID, row.AssetKind, row.AssetVersion, row.Status, row.CustomerID, row.CustomerEventID, row.OccurredAt, row.ReconciledAt, row.ReconcileReason, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (repository *ChannelAcquisitionEntrantReceiptRepository) GetUnassignedChannelAcquisitionEntrantReceipt(ctx context.Context, actorID, receiptID int64) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	queries, err := channelQueries(ctx)
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	row, err := queries.GetAdminUnassignedChannelAcquisitionEntrantReceipt(ctx, contactdb.GetAdminUnassignedChannelAcquisitionEntrantReceiptParams{ActorID: actorID, ReceiptID: receiptID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	}
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	return entrantReceiptItem(row.ID, row.ChannelID, row.EffectID, row.AssetKind, row.AssetVersion, row.Status, row.CustomerID, row.CustomerEventID, row.OccurredAt, row.ReconciledAt, row.ReconcileReason, row.CreatedAt, row.UpdatedAt)
}

func (repository *ChannelAcquisitionEntrantReceiptRepository) ReconcileChannelAcquisitionEntrantReceipt(ctx context.Context, command contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand, keyDigest, payloadDigest string) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	queries, err := channelQueries(ctx)
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	if command.Unassigned != (command.ChannelID == 0) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
	}
	effectID := channelAcquisitionEntrantEffectID(command.EffectID)
	if effectID < 1 {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
	}
	var status, externalUserID, callbackUserID, corpID string
	var priorEffect, priorChannel, priorVersion, priorActor pgtype.Int8
	var priorKind, priorKey, priorPayload pgtype.Text
	var occurredAt pgtype.Timestamptz
	if command.Unassigned {
		row, err := queries.LockAdminUnassignedChannelAcquisitionEntrantReceipt(ctx, contactdb.LockAdminUnassignedChannelAcquisitionEntrantReceiptParams{ActorID: command.ActorID, ReceiptID: command.ReceiptID, KeyDigest: keyDigest, PayloadDigest: payloadDigest})
		if errors.Is(err, pgx.ErrNoRows) {
			return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
		}
		if err != nil {
			return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
		}
		status, externalUserID, callbackUserID, corpID = row.Status, row.ExternalUserid, row.ExternalContactWecomUserid, row.CorpID
		priorEffect, priorChannel, priorVersion, priorActor = row.EffectID, row.ChannelID, row.AssetVersion, row.ReconcileActorID
		priorKind, priorKey, priorPayload, occurredAt = row.AssetKind, row.ReconcileKeyDigest, row.ReconcilePayloadDigest, row.OccurredAt
	} else {
		row, err := queries.LockAdminChannelAcquisitionEntrantReceipt(ctx, contactdb.LockAdminChannelAcquisitionEntrantReceiptParams{ActorID: command.ActorID, ReceiptID: command.ReceiptID, ChannelID: command.ChannelID})
		if errors.Is(err, pgx.ErrNoRows) {
			return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
		}
		if err != nil {
			return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
		}
		status, externalUserID, callbackUserID, corpID = row.Status, row.ExternalUserid, row.ExternalContactWecomUserid, row.CorpID
		priorEffect, priorChannel, priorVersion, priorActor = row.EffectID, row.ChannelID, row.AssetVersion, row.ReconcileActorID
		priorKind, priorKey, priorPayload, occurredAt = row.AssetKind, row.ReconcileKeyDigest, row.ReconcilePayloadDigest, row.OccurredAt
	}
	keyReceiptID, keyErr := queries.FindAdminChannelAcquisitionEntrantReconcileKey(ctx, contactdb.FindAdminChannelAcquisitionEntrantReconcileKeyParams{ActorID: command.ActorID, KeyDigest: keyDigest})
	if keyErr != nil && !errors.Is(keyErr, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, keyErr
	}
	if keyErr == nil && keyReceiptID != command.ReceiptID {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	if status == string(contactport.ChannelAcquisitionEntrantReconciled) {
		if priorChannel.Valid && priorChannel.Int64 > 0 && priorActor.Valid && priorActor.Int64 == command.ActorID && priorKey.Valid && priorKey.String == keyDigest && priorPayload.Valid && priorPayload.String == payloadDigest {
			return repository.GetChannelAcquisitionEntrantReceipt(ctx, command.ActorID, priorChannel.Int64, command.ReceiptID)
		}
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	current := contactport.ChannelAcquisitionEntrantStatus(status)
	if command.Unassigned && current != contactport.ChannelAcquisitionEntrantUnmatchedAsset && current != contactport.ChannelAcquisitionEntrantAmbiguousAsset {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	if current == contactport.ChannelAcquisitionEntrantIgnored || current == contactport.ChannelAcquisitionEntrantAttributed || !current.CanTransitionTo(contactport.ChannelAcquisitionEntrantReconciled) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	target, err := queries.LockAdminChannelAcquisitionEntrantTargetBinding(ctx, contactdb.LockAdminChannelAcquisitionEntrantTargetBindingParams{EffectID: effectID, CorpID: corpID, ChannelID: command.ChannelID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	}
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	if target.ChannelID < 1 || target.AssetKind == "" || target.AssetVersion < 1 {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	}
	resolvedChannelID := command.ChannelID
	if command.Unassigned {
		resolvedChannelID = target.ChannelID
	}
	if !target.PublishedAt.Valid || target.PublishedAt.Time.After(occurredAt.Time) || !containsEntrantAssignee(target.AssigneeWecomUserids, callbackUserID) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	if (current == contactport.ChannelAcquisitionEntrantPendingIdentity || current == contactport.ChannelAcquisitionEntrantConflict) &&
		(!priorEffect.Valid || priorEffect.Int64 != effectID || !priorChannel.Valid || priorChannel.Int64 != resolvedChannelID || !priorVersion.Valid || priorVersion.Int64 != target.AssetVersion || !priorKind.Valid || priorKind.String != target.AssetKind) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	if _, err = queries.LockAdminChannelAcquisitionEntrantCustomer(ctx, command.CustomerID); err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	identityCustomer, identityErr := queries.LockAdminChannelAcquisitionEntrantIdentity(ctx, contactdb.LockAdminChannelAcquisitionEntrantIdentityParams{CorpID: corpID, ExternalUserid: externalUserID})
	if identityErr != nil && !errors.Is(identityErr, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, identityErr
	}
	if identityErr == nil && identityCustomer.Valid && identityCustomer.Int64 != command.CustomerID {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	eventAt := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"receipt_id": command.ReceiptID, "channel_id": resolvedChannelID, "effect_id": command.EffectID, "asset_version": target.AssetVersion})
	eventID, err := queries.AppendCustomerEvent(ctx, contactdb.AppendCustomerEventParams{CustomerID: command.CustomerID, EventType: "channel.acquisition.entrant.reconciled", Actor: "admin:" + strconv.FormatInt(command.ActorID, 10), Payload: payload, OccurredAt: timestamp(eventAt)})
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	affected, err := queries.CompleteAdminChannelAcquisitionEntrantReconciliation(ctx, contactdb.CompleteAdminChannelAcquisitionEntrantReconciliationParams{EffectID: effectID, ChannelID: resolvedChannelID, AssetKind: target.AssetKind, AssetVersion: target.AssetVersion, CustomerID: command.CustomerID, CustomerEventID: eventID, EventAt: timestamp(eventAt), Reason: command.Reason, ActorID: command.ActorID, KeyDigest: keyDigest, PayloadDigest: payloadDigest, ReceiptID: command.ReceiptID, PriorStatus: status})
	if err != nil || affected != 1 {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	return repository.GetChannelAcquisitionEntrantReceipt(ctx, command.ActorID, resolvedChannelID, command.ReceiptID)
}

func entrantReceiptItem(id int64, channelID, effectID pgtype.Int8, kind pgtype.Text, version pgtype.Int8, status string, customerID, eventID pgtype.Int8, occurred, reconciled pgtype.Timestamptz, reason string, created, updated pgtype.Timestamptz) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	item := contactapp.ChannelAcquisitionEntrantReceiptItem{ReceiptID: id, Status: contactport.ChannelAcquisitionEntrantStatus(status), ReconcileReason: reason}
	hasBinding := channelID.Valid || effectID.Valid || kind.Valid || version.Valid
	if hasBinding {
		if !channelID.Valid || !effectID.Valid || !kind.Valid || !version.Valid || channelID.Int64 < 1 || effectID.Int64 < 1 || version.Int64 < 1 {
			return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable
		}
		item.ChannelID = channelID.Int64
		item.EffectID = "eer_" + strconv.FormatInt(effectID.Int64, 10)
		item.Kind = contactport.AcquisitionAssetKind(kind.String)
		item.AssetVersion = version.Int64
	}
	if customerID.Valid {
		item.CustomerID = customerID.Int64
	}
	if eventID.Valid {
		item.CustomerEventID = eventID.Int64
	}
	item.OccurredAt = occurred.Time.UTC()
	item.CreatedAt = created.Time.UTC()
	item.UpdatedAt = updated.Time.UTC()
	if reconciled.Valid {
		value := reconciled.Time.UTC()
		item.ReconciledAt = &value
	}
	needsBinding := entrantStatusNeedsBinding(item.Status)
	hasCustomer := customerID.Valid || eventID.Valid
	needsCustomer := item.Status == contactport.ChannelAcquisitionEntrantAttributed || item.Status == contactport.ChannelAcquisitionEntrantReconciled
	if item.ReceiptID < 1 || !item.Status.Valid() || hasBinding != needsBinding ||
		(hasBinding && item.Kind != contactport.AcquisitionAssetQRCode && item.Kind != contactport.AcquisitionAssetLink) ||
		(customerID.Valid != eventID.Valid) || hasCustomer != needsCustomer ||
		item.OccurredAt.IsZero() || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() ||
		(needsCustomer && (item.CustomerID < 1 || item.CustomerEventID < 1)) ||
		(item.Status == contactport.ChannelAcquisitionEntrantReconciled) != (item.ReconciledAt != nil && item.ReconcileReason != "") {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	return item, nil
}

func containsEntrantAssignee(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func channelAcquisitionEntrantEffectID(value string) int64 {
	if len(value) < 5 || value[:4] != "eer_" {
		return 0
	}
	id, _ := strconv.ParseInt(value[4:], 10, 64)
	return id
}
