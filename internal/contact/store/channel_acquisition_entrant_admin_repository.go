package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type ChannelAcquisitionEntrantReceiptRepository struct{}

func NewChannelAcquisitionEntrantReceiptRepository() *ChannelAcquisitionEntrantReceiptRepository {
	return &ChannelAcquisitionEntrantReceiptRepository{}
}

const entrantReceiptProjection = `SELECT r.id, r.channel_id, r.effect_id, r.asset_kind, r.asset_version, r.status,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconciled_at, r.reconcile_reason, r.created_at, r.updated_at
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id = r.inbox_id
LEFT JOIN channel_acquisition_asset_bindings b ON (b.effect_id,b.channel_id,b.asset_kind,b.asset_version) = (r.effect_id,r.channel_id,r.asset_kind,r.asset_version) AND b.corp_id = i.corp_id
JOIN admin_users a ON a.id = $1 AND a.is_active AND a.login_enabled AND a.wecom_corp_id = i.corp_id
WHERE (
  (r.channel_id = $2 AND b.corp_id = i.corp_id)
  OR (r.channel_id IS NULL AND EXISTS (
    SELECT 1 FROM channel_acquisition_asset_bindings scope
    WHERE scope.channel_id = $2 AND scope.corp_id = i.corp_id
  ))
)`

func (repository *ChannelAcquisitionEntrantReceiptRepository) ListChannelAcquisitionEntrantReceipts(ctx context.Context, actorID, channelID int64, limit int, after int64) ([]contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil || actorID < 1 || channelID < 1 || limit < 1 {
		return nil, contactapp.ErrChannelAcquisitionEntrantReceiptUnavailable
	}
	query := entrantReceiptProjection
	args := []any{actorID, channelID}
	if after > 0 {
		query += " AND r.id < $3"
		args = append(args, after)
	}
	query += fmt.Sprintf(" ORDER BY r.id DESC LIMIT $%d", len(args)+1)
	args = append(args, limit)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]contactapp.ChannelAcquisitionEntrantReceiptItem, 0, limit)
	for rows.Next() {
		item, scanErr := scanEntrantReceipt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (repository *ChannelAcquisitionEntrantReceiptRepository) GetChannelAcquisitionEntrantReceipt(ctx context.Context, actorID, channelID, receiptID int64) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	item, err := scanEntrantReceipt(tx.QueryRow(ctx, entrantReceiptProjection+" AND r.id = $3", actorID, channelID, receiptID))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	}
	return item, err
}

const entrantReceiptReconcileScopeSQL = `SELECT r.id, r.status, r.effect_id, r.channel_id, r.asset_kind, r.asset_version,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconcile_actor_id, r.reconcile_key_digest, r.reconcile_payload_digest,
       i.external_userid, i.external_contact_wecom_userid, i.corp_id,
       b.asset_kind, b.asset_version,
       CASE WHEN b.state = 'executed' THEN (SELECT max(f.completed_at) FROM channel_acquisition_asset_attempt_facts f WHERE f.effect_id=b.effect_id AND f.state='executed')
            WHEN b.state = 'reconciled' AND b.reconcile_resolution='provider_applied' THEN (SELECT max(f.reconciled_at) FROM channel_acquisition_asset_reconciliation_facts f WHERE f.effect_id=b.effect_id AND f.resolution='provider_applied') END AS published_at,
       b.assignee_wecom_userids
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id=r.inbox_id
JOIN admin_users a ON a.id=$1 AND a.is_active AND a.login_enabled AND a.wecom_corp_id=i.corp_id
LEFT JOIN channel_acquisition_asset_bindings b ON b.effect_id=$4 AND b.channel_id=$2 AND b.corp_id=i.corp_id
WHERE r.id=$3 AND (
  (r.channel_id=$2 AND EXISTS (
    SELECT 1 FROM channel_acquisition_asset_bindings current_binding
    WHERE (current_binding.effect_id,current_binding.channel_id,current_binding.asset_kind,current_binding.asset_version) = (r.effect_id,r.channel_id,r.asset_kind,r.asset_version)
      AND current_binding.corp_id=i.corp_id
  ))
  OR (r.channel_id IS NULL AND EXISTS (
    SELECT 1 FROM channel_acquisition_asset_bindings scope
    WHERE scope.channel_id=$2 AND scope.corp_id=i.corp_id
  ))
)
FOR UPDATE OF r`

func (repository *ChannelAcquisitionEntrantReceiptRepository) ReconcileChannelAcquisitionEntrantReceipt(ctx context.Context, command contactapp.ReconcileChannelAcquisitionEntrantReceiptCommand, keyDigest, payloadDigest string) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	effectID := channelAcquisitionEntrantEffectID(command.EffectID)
	if effectID < 1 {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrInvalidChannelAcquisitionEntrantReceipt
	}
	var status, externalUserID, callbackUserID, corpID string
	var priorEffect, priorChannel, priorVersion, priorCustomer, priorEvent, priorActor pgtype.Int8
	var priorKind, priorKey, priorPayload, targetKind pgtype.Text
	var occurredAt, publishedAt pgtype.Timestamptz
	var targetVersion pgtype.Int8
	var assignees []string
	err = tx.QueryRow(ctx, entrantReceiptReconcileScopeSQL, command.ActorID, command.ChannelID, command.ReceiptID, effectID).Scan(
		new(int64), &status, &priorEffect, &priorChannel, &priorKind, &priorVersion, &priorCustomer, &priorEvent, &occurredAt, &priorActor, &priorKey, &priorPayload,
		&externalUserID, &callbackUserID, &corpID, &targetKind, &targetVersion, &publishedAt, &assignees)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	}
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	var keyReceiptID int64
	keyErr := tx.QueryRow(ctx, `SELECT id FROM channel_acquisition_entrant_receipts WHERE reconcile_actor_id=$1 AND reconcile_key_digest=$2`, command.ActorID, keyDigest).Scan(&keyReceiptID)
	if keyErr != nil && !errors.Is(keyErr, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, keyErr
	}
	if keyErr == nil && keyReceiptID != command.ReceiptID {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	if status == string(contactport.ChannelAcquisitionEntrantReconciled) {
		if priorActor.Valid && priorActor.Int64 == command.ActorID && priorKey.Valid && priorKey.String == keyDigest && priorPayload.Valid && priorPayload.String == payloadDigest {
			return repository.GetChannelAcquisitionEntrantReceipt(ctx, command.ActorID, command.ChannelID, command.ReceiptID)
		}
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	current := contactport.ChannelAcquisitionEntrantStatus(status)
	if current == contactport.ChannelAcquisitionEntrantIgnored || current == contactport.ChannelAcquisitionEntrantAttributed || !current.CanTransitionTo(contactport.ChannelAcquisitionEntrantReconciled) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	if !targetKind.Valid || !targetVersion.Valid {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptNotFound
	}
	if !publishedAt.Valid || publishedAt.Time.After(occurredAt.Time) || !containsEntrantAssignee(assignees, callbackUserID) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	if (current == contactport.ChannelAcquisitionEntrantPendingIdentity || current == contactport.ChannelAcquisitionEntrantConflict) &&
		(!priorEffect.Valid || priorEffect.Int64 != effectID || !priorChannel.Valid || priorChannel.Int64 != command.ChannelID || !priorVersion.Valid || priorVersion.Int64 != targetVersion.Int64 || !priorKind.Valid || priorKind.String != targetKind.String) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	var customerExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customers WHERE id=$1 AND NOT is_deleted)`, command.CustomerID).Scan(&customerExists); err != nil || !customerExists {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	var identityCustomer pgtype.Int8
	err = tx.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind='wecom_external_userid' AND scope='wecom-corp:' || $1 AND normalized_value=$2 AND assurance='verified' FOR UPDATE`, corpID, externalUserID).Scan(&identityCustomer)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	if err == nil && identityCustomer.Valid && identityCustomer.Int64 != command.CustomerID {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	eventAt := time.Now().UTC()
	var eventID int64
	err = tx.QueryRow(ctx, `INSERT INTO customer_events(customer_id,event_type,actor,payload,occurred_at)
VALUES($1,'channel.acquisition.entrant.reconciled',$2, jsonb_build_object('receipt_id',$3::bigint,'channel_id',$4::bigint,'effect_id',$5::text,'asset_version',$6::bigint), $7) RETURNING id`, command.CustomerID, "admin:"+strconv.FormatInt(command.ActorID, 10), command.ReceiptID, command.ChannelID, command.EffectID, targetVersion.Int64, eventAt).Scan(&eventID)
	if err != nil {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, err
	}
	result, err := tx.Exec(ctx, `UPDATE channel_acquisition_entrant_receipts SET status='reconciled', effect_id=$1, channel_id=$2, asset_kind=$3, asset_version=$4,
customer_id=$5, customer_event_id=$6, customer_event_occurred_at=$7, reconciled_at=$7, reconcile_reason=$8,
reconcile_actor_id=$9, reconcile_key_digest=$10, reconcile_payload_digest=$11, updated_at=$7 WHERE id=$12 AND status=$13`, effectID, command.ChannelID, targetKind.String, targetVersion.Int64, command.CustomerID, eventID, eventAt, command.Reason, command.ActorID, keyDigest, payloadDigest, command.ReceiptID, status)
	if err != nil || result.RowsAffected() != 1 {
		return contactapp.ChannelAcquisitionEntrantReceiptItem{}, contactapp.ErrChannelAcquisitionEntrantReceiptConflict
	}
	return repository.GetChannelAcquisitionEntrantReceipt(ctx, command.ActorID, command.ChannelID, command.ReceiptID)
}

type entrantReceiptScanner interface{ Scan(...any) error }

func scanEntrantReceipt(row entrantReceiptScanner) (contactapp.ChannelAcquisitionEntrantReceiptItem, error) {
	var item contactapp.ChannelAcquisitionEntrantReceiptItem
	var channelID, effectID, version, customerID, eventID pgtype.Int8
	var kind pgtype.Text
	var status string
	var occurred, reconciled, created, updated pgtype.Timestamptz
	err := row.Scan(&item.ReceiptID, &channelID, &effectID, &kind, &version, &status, &customerID, &eventID, &occurred, &reconciled, &item.ReconcileReason, &created, &updated)
	if err != nil {
		return item, err
	}
	item.Status = contactport.ChannelAcquisitionEntrantStatus(status)
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
