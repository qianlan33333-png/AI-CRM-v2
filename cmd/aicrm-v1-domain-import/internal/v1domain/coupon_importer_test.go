package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestCouponImporterImportsReadOnlyHistoryAndReplays(t *testing.T) {
	archive := couponImportArchive(t)
	importer, writer, journals := newCouponImporterForTest(t, archive, couponResolverFake{
		products: map[int64]int64{301: 41}, customers: map[string]*int64{"union-101": couponID(51)}, orders: map[int64]*int64{701: couponID(61)},
	})

	created, err := importer.Import(context.Background(), "archive-run")
	wantCreated := CouponImportResult{ImportedDefinitions: 1, ImportedBindings: 1, ImportedClaims: 1, ImportedRedemptions: 1}
	if err != nil || created != wantCreated {
		t.Fatalf("created/error = %#v/%v", created, err)
	}
	if len(writer.definitions) != 1 || len(writer.claims) != 1 || len(writer.redemptions) != 1 {
		t.Fatalf("writer values = %#v", writer)
	}
	definition := writer.definition(couponSourceKey(t, archive.rows[couponDefinitionsTableID][0]))
	if !definition.HistoryOnly || definition.Status != "archived" || definition.AvailabilityStatus != "archived" || definition.CreatedBy != 7 || definition.UpdatedBy != 7 || definition.Version != 1 || len(definition.TargetRefs) != 1 || definition.TargetRefs[0] != "standard_product:41" {
		t.Fatalf("definition = %#v", definition)
	}
	claim := writer.claim(couponSourceKey(t, archive.rows[couponClaimsTableID][0]))
	if claim.CustomerID == nil || *claim.CustomerID != 51 || claim.SourceCouponID != 101 || claim.CouponID < 1 || claim.Status != "claimed" {
		t.Fatalf("claim = %#v", claim)
	}
	redemption := writer.redemption(couponSourceKey(t, archive.rows[couponRedemptionsTableID][0]))
	if redemption.OrderID == nil || *redemption.OrderID != 61 || redemption.ClaimHistoryID < 1 || redemption.ReleaseReason != "原始原因" || redemption.Status != "consumed" {
		t.Fatalf("redemption = %#v", redemption)
	}
	bindingTerminal := journals[couponBindingsKind].(*couponTerminalFake).values[couponSourceKey(t, archive.rows[couponBindingsTableID][0])]
	if bindingTerminal.Disposition != "import" || bindingTerminal.TargetID != HistoricalCouponBindingTargetID(1, 0) || bindingTerminal.TargetDigest != HistoricalCouponBindingTargetDigest(1, 0, 41) {
		t.Fatalf("binding terminal = %#v", bindingTerminal)
	}

	replayed, err := importer.Import(context.Background(), "archive-run")
	wantReplayed := CouponImportResult{ImportedDefinitions: 1, ImportedBindings: 1, ImportedClaims: 1, ImportedRedemptions: 1,
		ReplayedDefinitions: 1, ReplayedBindings: 1, ReplayedClaims: 1, ReplayedRedemptions: 1}
	if err != nil || replayed != wantReplayed {
		t.Fatalf("replayed/error = %#v/%v", replayed, err)
	}
}

func TestCouponImporterQuarantinesDependentRowsWhenDefinitionCannotResolve(t *testing.T) {
	archive := couponImportArchive(t)
	importer, writer, journals := newCouponImporterForTest(t, archive, couponResolverFake{products: map[int64]int64{301: 0}})

	result, err := importer.Import(context.Background(), "archive-run")
	want := CouponImportResult{QuarantinedDefinitions: 1, QuarantinedBindings: 1, QuarantinedClaims: 1, QuarantinedRedemptions: 1}
	if err != nil || result != want || len(writer.definitions) != 0 || len(writer.claims) != 0 || len(writer.redemptions) != 0 {
		t.Fatalf("result/error/writer = %#v/%v/%#v", result, err, writer)
	}
	for kind, reason := range map[string]string{
		couponDefinitionsKind: "coupon_binding_product_unresolved", couponBindingsKind: "coupon_binding_product_unresolved",
		couponClaimsKind: "coupon_claim_parent_coupon_unavailable", couponRedemptionsKind: "coupon_redemption_parent_claim_unavailable",
	} {
		row := archive.rows[tableForCouponKind(kind)][0]
		terminal := journals[kind].(*couponTerminalFake).values[couponSourceKey(t, row)]
		if terminal.Disposition != "quarantine" || terminal.Reason != reason {
			t.Fatalf("%s terminal = %#v", kind, terminal)
		}
	}
}

func TestCouponImporterRejectsUnrelatedOrInvalidArchiveRows(t *testing.T) {
	for name, mutate := range map[string]func(*v1archive.ArchivedRow){
		"unrelated-table": func(row *v1archive.ArchivedRow) { row.TableID = "public/wechat_pay_products" },
		"zero-ordinal":    func(row *v1archive.ArchivedRow) { row.SourceOrdinal = 0 },
		"zero-payload":    func(row *v1archive.ArchivedRow) { row.PayloadHMAC = [sha256.Size]byte{} },
	} {
		t.Run(name, func(t *testing.T) {
			archive := couponImportArchive(t)
			mutate(&archive.rows[couponDefinitionsTableID][0])
			importer, writer, _ := newCouponImporterForTest(t, archive, couponResolverFake{products: map[int64]int64{301: 41}})
			if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || len(writer.definitions) != 0 {
				t.Fatalf("error/writer = %v/%#v", err, writer)
			}
		})
	}
}

func TestCouponImporterRejectsPayloadDriftWithoutNewTarget(t *testing.T) {
	archive := couponImportArchive(t)
	importer, writer, _ := newCouponImporterForTest(t, archive, couponResolverFake{products: map[int64]int64{301: 41}})
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal(err)
	}
	row := &archive.rows[couponDefinitionsTableID][0]
	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload["name"] = "历史券二"
	changed, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	row.Payload = changed
	row.PayloadHMAC = sha256.Sum256(row.Payload)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, couponport.ErrHistoryConflict) || len(writer.definitions) != 1 {
		t.Fatalf("error/definitions = %v/%#v", err, writer.definitions)
	}
}

type couponArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *couponArchiveFake) EachTableRow(_ context.Context, _ string, table string, callback func(v1archive.ArchivedRow) error) error {
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type couponResolverFake struct {
	products  map[int64]int64
	customers map[string]*int64
	orders    map[int64]*int64
	err       error
}

func (resolver couponResolverFake) ResolveCouponProduct(_ context.Context, sourceID, _ int64) (int64, error) {
	return resolver.products[sourceID], resolver.err
}

func (resolver couponResolverFake) ResolveCouponCustomer(_ context.Context, unionID string) (*int64, error) {
	return resolver.customers[unionID], resolver.err
}

func (resolver couponResolverFake) ResolveCouponOrder(_ context.Context, sourceID int64, _ string) (*int64, error) {
	return resolver.orders[sourceID], resolver.err
}

type couponWriterFake struct {
	definitions map[string]couponport.HistoricalDefinition
	claims      map[string]couponport.HistoricalClaim
	redemptions map[string]couponport.HistoricalRedemption
	receipts    map[string]couponport.HistoricalReceipt
}

func (writer *couponWriterFake) ImportDefinition(_ context.Context, source string, payload [sha256.Size]byte, value couponport.HistoricalDefinition) (couponport.HistoricalReceipt, error) {
	if existing, found := writer.receipts["definition/"+source]; found {
		if existing.PayloadDigest != payload {
			return couponport.HistoricalReceipt{}, couponport.ErrHistoryConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	receipt := couponWriterReceipt(source, payload, int64(len(writer.definitions)+1), "definition")
	writer.definitions[source], writer.receipts["definition/"+source] = value, receipt
	return receipt, nil
}

func (writer *couponWriterFake) ImportClaim(_ context.Context, source string, payload [sha256.Size]byte, value couponport.HistoricalClaim) (couponport.HistoricalReceipt, error) {
	if existing, found := writer.receipts["claim/"+source]; found {
		if existing.PayloadDigest != payload {
			return couponport.HistoricalReceipt{}, couponport.ErrHistoryConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	receipt := couponWriterReceipt(source, payload, int64(len(writer.claims)+1), "claim")
	writer.claims[source], writer.receipts["claim/"+source] = value, receipt
	return receipt, nil
}

func (writer *couponWriterFake) ImportRedemption(_ context.Context, source string, payload [sha256.Size]byte, value couponport.HistoricalRedemption) (couponport.HistoricalReceipt, error) {
	if existing, found := writer.receipts["redemption/"+source]; found {
		if existing.PayloadDigest != payload {
			return couponport.HistoricalReceipt{}, couponport.ErrHistoryConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	receipt := couponWriterReceipt(source, payload, int64(len(writer.redemptions)+1), "redemption")
	writer.redemptions[source], writer.receipts["redemption/"+source] = value, receipt
	return receipt, nil
}

func (writer *couponWriterFake) definition(source string) couponport.HistoricalDefinition {
	return writer.definitions[source]
}
func (writer *couponWriterFake) claim(source string) couponport.HistoricalClaim {
	return writer.claims[source]
}
func (writer *couponWriterFake) redemption(source string) couponport.HistoricalRedemption {
	return writer.redemptions[source]
}

func couponWriterReceipt(source string, payload [sha256.Size]byte, targetID int64, kind string) couponport.HistoricalReceipt {
	return couponport.HistoricalReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID, TargetDigest: sha256.Sum256([]byte(kind + "/" + source))}
}

func newCouponImporterForTest(t *testing.T, archive ArchiveSource, resolver CouponReferenceResolver) (*CouponImporter, *couponWriterFake, map[string]couponTerminalJournal) {
	t.Helper()
	writer := &couponWriterFake{definitions: map[string]couponport.HistoricalDefinition{}, claims: map[string]couponport.HistoricalClaim{}, redemptions: map[string]couponport.HistoricalRedemption{}, receipts: map[string]couponport.HistoricalReceipt{}}
	journals := map[string]couponTerminalJournal{
		couponDefinitionsKind: newCouponTerminalFake(), couponBindingsKind: newCouponTerminalFake(),
		couponClaimsKind: newCouponTerminalFake(), couponRedemptionsKind: newCouponTerminalFake(),
	}
	importer, err := newCouponImporter(archive, immediateUOW{}, writer, resolver, journals, "archive-run", 7)
	if err != nil {
		t.Fatal(err)
	}
	return importer, writer, journals
}

func couponImportArchive(t *testing.T) *couponArchiveFake {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC)
	return &couponArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		couponDefinitionsTableID: {couponArchivedJSON(t, couponDefinitionsTableID, 1, map[string]any{
			"id": int64(101), "tenant_id": "tenant-1", "public_slug": "coupon-101", "name": "历史券", "discount_amount_total": int64(1200), "currency": "CNY", "status": "expired",
			"total_issue_limit": int64(10), "per_user_issue_limit": int64(1), "issued_count": int64(0), "claim_starts_at": stamp, "claim_ends_at": stamp.Add(24 * time.Hour),
			"validity_mode": "fixed_range", "use_starts_at": stamp, "use_ends_at": stamp.Add(48 * time.Hour), "relative_validity_days": nil, "instructions": "历史定义", "first_claim_at": nil,
			"created_by": int64(1), "updated_by": int64(1), "created_at": stamp, "updated_at": stamp,
		})},
		couponBindingsTableID: {couponArchivedJSON(t, couponBindingsTableID, 2, map[string]any{"id": int64(201), "tenant_id": "tenant-1", "coupon_id": int64(101), "trade_product_id": int64(301), "created_at": stamp})},
		couponClaimsTableID: {couponArchivedJSON(t, couponClaimsTableID, 3, map[string]any{
			"id": int64(401), "tenant_id": "tenant-1", "coupon_id": int64(101), "claim_no": "C-401", "unionid": "union-101", "discount_amount_total": int64(1200), "currency": "CNY", "status": "claimed",
			"valid_from": stamp, "valid_until": stamp.Add(24 * time.Hour), "claimed_at": stamp, "idempotency_key_hash": "sealed", "created_at": stamp, "updated_at": stamp,
		})},
		couponRedemptionsTableID: {couponArchivedJSON(t, couponRedemptionsTableID, 4, map[string]any{
			"id": int64(501), "tenant_id": "tenant-1", "claim_id": int64(401), "order_id": int64(701), "out_trade_no": "T-701", "status": "consumed",
			"original_amount_total": int64(9900), "discount_amount_total": int64(1200), "payable_amount_total": int64(8700), "currency": "CNY", "reserved_until": stamp.Add(time.Hour), "release_reason": "原始原因", "reserved_at": stamp,
			"idempotency_key_hash": "sealed", "created_at": stamp, "updated_at": stamp,
		})},
	}}
}

func couponArchivedJSON(t *testing.T, table string, ordinal int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(table + "/" + string(rune(ordinal)))), PayloadHMAC: sha256.Sum256(payload), Payload: payload}
}

func couponSourceKey(t *testing.T, row v1archive.ArchivedRow) string {
	t.Helper()
	return SourceIdentifier(row.SourceKeyHMAC)
}

func couponID(value int64) *int64 { return &value }

func tableForCouponKind(kind string) string {
	switch kind {
	case couponDefinitionsKind:
		return couponDefinitionsTableID
	case couponBindingsKind:
		return couponBindingsTableID
	case couponClaimsKind:
		return couponClaimsTableID
	default:
		return couponRedemptionsTableID
	}
}
