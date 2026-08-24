package store

import (
	"errors"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestScanStageReceiptAcceptsAllCurrentLifecycleOperations(t *testing.T) {
	keyDigest := make([]byte, 32)
	payloadDigest := make([]byte, 32)
	for _, operation := range []string{"create", "rename", "reorder", "archive"} {
		t.Run(operation, func(t *testing.T) {
			receipt, err := scanStageReceipt(stageReceiptRow{
				id:            7,
				operation:     operation,
				actor:         "admin:1",
				keyDigest:     keyDigest,
				payloadDigest: payloadDigest,
				state:         "completed",
				resultIDs:     []int64{11},
			})
			if err != nil {
				t.Fatalf("scanStageReceipt() error = %v", err)
			}
			if receipt.ID != 7 || string(receipt.Operation) != operation || string(receipt.Actor) != "admin:1" || receipt.State != "completed" || len(receipt.ResultIDs) != 1 || receipt.ResultIDs[0] != 11 {
				t.Fatalf("receipt = %#v", receipt)
			}
		})
	}
}

func TestParseHistoricalImportReceiptDispositionLimitsSkippedToOwnerRoleMap(t *testing.T) {
	disposition, err := parseHistoricalImportReceiptDisposition(contactport.HistoricalImportOwnerRoleMap, "skipped")
	if err != nil || disposition != contactport.HistoricalImportSkipped {
		t.Fatalf("owner skipped = %v/%v", disposition, err)
	}
	if _, err = parseHistoricalImportReceiptDisposition(contactport.HistoricalImportCustomerIdentity, "skipped"); !errors.Is(err, ErrHistoricalImportTargetDrift) {
		t.Fatalf("customer skipped = %v", err)
	}
}

type stageReceiptRow struct {
	id            int64
	operation     string
	actor         string
	keyDigest     []byte
	payloadDigest []byte
	state         string
	resultIDs     []int64
}

func (row stageReceiptRow) Scan(dest ...any) error {
	*(dest[0].(*int64)) = row.id
	*(dest[1].(*string)) = row.operation
	*(dest[2].(*string)) = row.actor
	*(dest[3].(*[]byte)) = append([]byte(nil), row.keyDigest...)
	*(dest[4].(*[]byte)) = append([]byte(nil), row.payloadDigest...)
	*(dest[5].(*string)) = row.state
	*(dest[6].(*[]int64)) = append([]int64(nil), row.resultIDs...)
	return nil
}
