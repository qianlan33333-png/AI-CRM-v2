package store

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestEntrantReceiptProjectionsNeverInferChannelOwnershipFromCorp(t *testing.T) {
	for _, fragment := range []string{"JOIN channel_acquisition_asset_bindings", "a.wecom_corp_id = b.corp_id", "WHERE r.channel_id = $2"} {
		if !strings.Contains(entrantReceiptProjection, fragment) {
			t.Fatalf("channel projection is missing %q", fragment)
		}
	}
	if strings.Contains(entrantReceiptProjection, "r.channel_id IS NULL") {
		t.Fatal("channel projection must not expose unassigned receipts")
	}
	for _, fragment := range []string{"a.wecom_corp_id = i.corp_id", "r.effect_id IS NULL", "r.channel_id IS NULL", "r.status IN ('unmatched_asset', 'ambiguous_asset', 'ignored')"} {
		if !strings.Contains(unassignedEntrantReceiptProjection, fragment) {
			t.Fatalf("unassigned projection is missing %q", fragment)
		}
	}
}

func TestEntrantReceiptReconcileLocksExactTargetBinding(t *testing.T) {
	for _, fragment := range []string{
		"b.effect_id=$1",
		"b.corp_id=$2",
		"($3::bigint=0 OR b.channel_id=$3)",
		"FOR UPDATE OF b",
	} {
		if !strings.Contains(entrantReceiptTargetBindingSQL, fragment) {
			t.Fatalf("target binding lock is missing %q", fragment)
		}
	}
}

func TestScanEntrantReceiptAllowsOnlyStateAppropriateNullableAsset(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	timestamp := pgtype.Timestamptz{Time: now, Valid: true}
	bound, err := scanEntrantReceipt(entrantReceiptAdminRow{values: []any{
		int64(90), pgtype.Int8{Int64: 41, Valid: true}, pgtype.Int8{Int64: 7, Valid: true}, pgtype.Text{String: string(contactport.AcquisitionAssetQRCode), Valid: true}, pgtype.Int8{Int64: 3, Valid: true}, string(contactport.ChannelAcquisitionEntrantAttributed),
		pgtype.Int8{Int64: 22, Valid: true}, pgtype.Int8{Int64: 16, Valid: true}, timestamp, pgtype.Timestamptz{}, "", timestamp, timestamp,
	}})
	if err != nil || bound.ChannelID != 41 || bound.EffectID != "eer_7" || bound.Kind != contactport.AcquisitionAssetQRCode || bound.AssetVersion != 3 || bound.CustomerID != 22 || bound.CustomerEventID != 16 {
		t.Fatalf("bound receipt = %#v, err=%v", bound, err)
	}
	unbound, err := scanEntrantReceipt(entrantReceiptAdminRow{values: []any{
		int64(91), pgtype.Int8{}, pgtype.Int8{}, pgtype.Text{}, pgtype.Int8{}, string(contactport.ChannelAcquisitionEntrantIgnored),
		pgtype.Int8{}, pgtype.Int8{}, timestamp, pgtype.Timestamptz{}, "", timestamp, timestamp,
	}})
	if err != nil || unbound.ReceiptID != 91 || unbound.Status != contactport.ChannelAcquisitionEntrantIgnored || unbound.ChannelID != 0 || unbound.EffectID != "" || unbound.Kind != "" || unbound.AssetVersion != 0 {
		t.Fatalf("unbound receipt = %#v, err=%v", unbound, err)
	}
	if _, err = scanEntrantReceipt(entrantReceiptAdminRow{values: []any{
		int64(92), pgtype.Int8{}, pgtype.Int8{}, pgtype.Text{}, pgtype.Int8{}, string(contactport.ChannelAcquisitionEntrantPendingIdentity),
		pgtype.Int8{}, pgtype.Int8{}, timestamp, pgtype.Timestamptz{}, "", timestamp, timestamp,
	}}); err == nil {
		t.Fatal("pending_identity without its exact asset binding must fail closed")
	}
}

type entrantReceiptAdminRow struct {
	values []any
}

func (row entrantReceiptAdminRow) Scan(destinations ...any) error {
	return channelEntrantsAssign(destinations, row.values)
}
