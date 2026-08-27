package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1finance"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

var financeArchiveRun = flag.String("finance-archive-run", "", "optional reconciled V2 archive run for read-only financial input validation")

// The nil embedded dependencies must remain unreachable: the sentinel UoW
// returns before invoking the transaction callback, after input validation.
type unreachableFinanceArchiveStore struct {
	orderport.HistoricalImportStore
}
type unreachableFinanceArchiveJournal struct {
	orderport.HistoricalImportJournal
}

func TestReconciledFinanceArchiveValidatesWithoutWrites(t *testing.T) {
	if *financeArchiveRun == "" {
		t.Skip("supply -finance-archive-run and V2 archive environment for read-only validation")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	ctx := context.Background()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("finance_archive_open_failed")
	}
	defer archive.Close()
	reader := &FinanceImporter{archive: archive}
	orders, err := reader.readRows(ctx, *financeArchiveRun, financeOrdersTableID, "id", "coupon_claim_id")
	if err != nil {
		t.Fatal("finance_orders_archive_read_failed")
	}
	refunds, err := reader.readRows(ctx, *financeArchiveRun, financeRefundsTableID, "id", "order_id")
	if err != nil {
		t.Fatal("finance_refunds_archive_read_failed")
	}
	if len(orders) != 708 || len(refunds) != 115 {
		t.Fatalf("finance_archive_count_mismatch orders=%d refunds=%d", len(orders), len(refunds))
	}
	if !validFinanceArchiveValidationRows(orders, financeOrdersTableID) || !validFinanceArchiveValidationRows(refunds, financeRefundsTableID) {
		t.Fatal("finance_archive_scope_or_digest_invalid")
	}
	orderPayloads, refundPayloads := make([]json.RawMessage, len(orders)), make([]json.RawMessage, len(refunds))
	for index := range orders {
		orderPayloads[index] = orders[index].archive.Payload
	}
	for index := range refunds {
		refundPayloads[index] = refunds[index].archive.Payload
	}
	history := v1finance.AdaptHistory(orderPayloads, refundPayloads)
	if len(history.Orders) != len(orders) || len(history.Refunds) != len(refunds) {
		t.Fatal("finance_archive_row_conservation_failed")
	}
	service := financeArchiveValidationService(t)
	validatedOrders, validatedRefunds, targetRejected := 0, 0, 0
	reasons := map[string]int{}
	for index, decision := range history.Orders {
		if orders[index].redactionReason != "" {
			reasons[orders[index].redactionReason]++
			continue
		}
		if decision.Disposition != v1finance.DispositionCandidate {
			if decision.Reason == "" {
				t.Fatal("finance_order_disposition_invalid")
			}
			reasons[decision.Reason]++
			continue
		}
		if decision.Fact == nil {
			t.Fatal("finance_order_candidate_missing")
		}
		// Customer and product target references are deliberately not resolved.
		_, err := service.ImportOrder(ctx, orderport.HistoricalOrderRecord{
			Fact:  financeHistoricalFact(orders[index], financeMappedOrderFieldDigest(orders[index].fieldDigest, nil, nil)),
			Order: financeOrderRecord(*decision.Fact, nil, nil),
		})
		if errors.Is(err, errSurveyValidated) {
			validatedOrders++
		} else if errors.Is(err, orderport.ErrHistoricalInput) {
			reasons["order_target_invalid"]++
			targetRejected++
		} else {
			t.Fatal("finance_order_validation_sentinel_missing")
		}
	}
	for index, decision := range history.Refunds {
		if refunds[index].redactionReason != "" {
			reasons[refunds[index].redactionReason]++
			continue
		}
		if decision.Disposition != v1finance.DispositionCandidate {
			if decision.Reason == "" {
				t.Fatal("finance_refund_disposition_invalid")
			}
			reasons[decision.Reason]++
			continue
		}
		if decision.Fact == nil {
			t.Fatal("finance_refund_candidate_missing")
		}
		// Dummy ID is only for input shape; no parent lookup/FK is exercised.
		_, err := service.ImportRefund(ctx, orderport.HistoricalRefundRecord{
			Fact:   financeHistoricalFact(refunds[index], financeMappedRefundFieldDigest(refunds[index].fieldDigest, 1)),
			Refund: financeRefundRecord(*decision.Fact, 1),
		})
		if errors.Is(err, errSurveyValidated) {
			validatedRefunds++
		} else if errors.Is(err, orderport.ErrHistoricalInput) {
			reasons["refund_target_invalid"]++
			targetRejected++
		} else {
			t.Fatal("finance_refund_validation_sentinel_missing")
		}
	}
	keys, quarantined := make([]string, 0, len(reasons)), 0
	for reason, count := range reasons {
		keys = append(keys, reason)
		quarantined += count
	}
	sort.Strings(keys)
	for _, reason := range keys {
		t.Logf("reason=%s count=%d", reason, reasons[reason])
	}
	if validatedOrders+validatedRefunds+quarantined != len(orders)+len(refunds) {
		t.Fatal("finance_validation_row_conservation_failed")
	}
	t.Logf("shape_only orders=%d refunds=%d validated_orders=%d validated_refunds=%d quarantine_rows=%d customer_product_fk_unverified=1 refund_parent_fk_unverified=1 target_writes=0", len(orders), len(refunds), validatedOrders, validatedRefunds, quarantined)
	if targetRejected != 0 {
		t.Errorf("finance_target_input_rejected count=%d", targetRejected)
	}
}

func financeArchiveValidationService(t *testing.T) *orderapp.HistoricalImportService {
	t.Helper()
	service, err := orderapp.NewHistoricalImportService(surveyValidationOnlyUOW{}, &unreachableFinanceArchiveStore{}, &unreachableFinanceArchiveJournal{})
	if err != nil {
		t.Fatal("finance_validation_service_unavailable")
	}
	return service
}

func validFinanceArchiveValidationRows(rows []financeArchiveRow, tableID string) bool {
	seen := make(map[[sha256.Size]byte]bool, len(rows))
	zero := [sha256.Size]byte{}
	for index, row := range rows {
		if row.archive.AdapterID != v1archive.DefaultAdapterID || row.archive.TableID != tableID || row.archive.SourceOrdinal != int64(index+1) ||
			row.archive.SourceKeyHMAC == zero || row.archive.PayloadHMAC == zero || row.archive.FieldHMAC == zero || row.fieldDigest == zero || seen[row.archive.SourceKeyHMAC] {
			return false
		}
		seen[row.archive.SourceKeyHMAC] = true
	}
	return true
}

func TestFinanceArchiveValidationStopsBeforeTargetAccess(t *testing.T) {
	service := financeArchiveValidationService(t)
	stamp := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	fact := orderport.HistoricalFact{SourceKeyDigest: [32]byte{1}, PayloadDigest: [32]byte{2}, FieldDigest: [32]byte{3}}
	order := orderport.HistoricalOrderRecord{Fact: fact, Order: financeOrderRecord(v1finance.OrderFact{
		OrderNumber: "test-order", Product: v1finance.ProductSourceRef{Kind: "code", Value: "test-product"},
		AmountMinor: 990, Currency: "CNY", Status: "paid", CreatedAt: stamp, UpdatedAt: stamp,
	}, nil, nil)}
	refund := orderport.HistoricalRefundRecord{Fact: fact, Refund: financeRefundRecord(v1finance.RefundFact{
		SourceID: 1, RefundNumber: "test-refund", AmountMinor: 100, OrderAmount: 990,
		Currency: "CNY", Status: "SUCCESS", CreatedAt: stamp, UpdatedAt: stamp,
	}, 1)}
	if _, err := service.ImportOrder(context.Background(), order); !errors.Is(err, errSurveyValidated) {
		t.Fatal("finance_order_validation_sentinel_missing")
	}
	if _, err := service.ImportRefund(context.Background(), refund); !errors.Is(err, errSurveyValidated) {
		t.Fatal("finance_refund_validation_sentinel_missing")
	}
	order.Order.Currency = "USD"
	refund.Refund.OrderID = 0
	if _, err := service.ImportOrder(context.Background(), order); !errors.Is(err, orderport.ErrHistoricalInput) {
		t.Fatal("finance_invalid_order_not_rejected")
	}
	if _, err := service.ImportRefund(context.Background(), refund); !errors.Is(err, orderport.ErrHistoricalInput) {
		t.Fatal("finance_invalid_refund_not_rejected")
	}
}

func TestFinanceArchiveValidationRejectsScopeAndDigestMismatch(t *testing.T) {
	valid := financeArchiveRow{archive: v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: financeOrdersTableID, SourceOrdinal: 1,
		SourceKeyHMAC: [32]byte{1}, PayloadHMAC: [32]byte{2}, FieldHMAC: [32]byte{3},
	}, fieldDigest: [32]byte{4}}
	if !validFinanceArchiveValidationRows([]financeArchiveRow{valid}, financeOrdersTableID) {
		t.Fatal("finance_valid_archive_row_rejected")
	}
	for _, mutate := range []func(*financeArchiveRow){
		func(row *financeArchiveRow) { row.archive.AdapterID = "wrong-adapter" },
		func(row *financeArchiveRow) { row.archive.TableID = financeRefundsTableID },
		func(row *financeArchiveRow) { row.archive.SourceOrdinal = 2 },
		func(row *financeArchiveRow) { row.archive.SourceKeyHMAC = [32]byte{} },
		func(row *financeArchiveRow) { row.archive.PayloadHMAC = [32]byte{} },
		func(row *financeArchiveRow) { row.archive.FieldHMAC = [32]byte{} },
		func(row *financeArchiveRow) { row.fieldDigest = [32]byte{} },
	} {
		row := valid
		mutate(&row)
		if validFinanceArchiveValidationRows([]financeArchiveRow{row}, financeOrdersTableID) {
			t.Fatal("finance_invalid_archive_row_accepted")
		}
	}
	duplicate := valid
	duplicate.archive.SourceOrdinal = 2
	if validFinanceArchiveValidationRows([]financeArchiveRow{valid, duplicate}, financeOrdersTableID) {
		t.Fatal("finance_duplicate_source_key_accepted")
	}
}
