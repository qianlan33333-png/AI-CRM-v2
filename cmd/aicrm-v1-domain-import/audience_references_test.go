package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type audienceStaffReferenceSpy struct {
	contactport.HistoricalImportTarget
	lineage                    contactport.HistoricalImportLineage
	receipt                    contactport.HistoricalImportRowReceipt
	lineageFound, receiptFound bool
	staff                      contactport.HistoricalImportStaffFact
	lockedSource               contactport.HistoricalImportSource
	lockedKey                  []byte
	lockedTarget               int64
}

func (s *audienceStaffReferenceSpy) LockHistoricalImportSource(_ context.Context, source contactport.HistoricalImportSource, key []byte) error {
	s.lockedSource, s.lockedKey = source, append([]byte(nil), key...)
	return nil
}
func (s *audienceStaffReferenceSpy) LockHistoricalImportLineage(context.Context, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportLineage, bool, error) {
	return s.lineage, s.lineageFound, nil
}
func (s *audienceStaffReferenceSpy) FindHistoricalImportRowReceipt(context.Context, int64, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	return s.receipt, s.receiptFound, nil
}
func (s *audienceStaffReferenceSpy) LockHistoricalImportStaffTarget(_ context.Context, id int64) (contactport.HistoricalImportStaffFact, error) {
	s.lockedTarget = id
	return s.staff, nil
}

func TestAudienceHistoryReferencesUseVerifiedCustomerAndStaffFacts(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	payload := bytes.Repeat([]byte{3}, 32)
	staff := contactport.HistoricalImportStaffFact{WeComUserID: "staff-1", Name: "历史员工", Active: true,
		CreatedAt: time.Date(2026, 8, 1, 1, 2, 3, 4000, time.UTC), UpdatedAt: time.Date(2026, 8, 2, 1, 2, 3, 5000, time.UTC)}
	field, err := audienceHistoryStaffDigest(key, staff)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "missing", "wrong_run", "missing_receipt", "receipt_drift", "actual_drift"} {
		t.Run(scenario, func(t *testing.T) {
			spy := &audienceStaffReferenceSpy{lineageFound: true, receiptFound: true, staff: staff,
				lineage: contactport.HistoricalImportLineage{TargetID: 71, LastRunID: 4, PayloadHMAC: payload, FieldDigest: field},
				receipt: contactport.HistoricalImportRowReceipt{Disposition: contactport.HistoricalImportImported, PayloadHMAC: payload, FieldDigest: field}}
			switch scenario {
			case "missing":
				spy.lineageFound = false
			case "wrong_run":
				spy.lineage.LastRunID++
			case "missing_receipt":
				spy.receiptFound = false
			case "receipt_drift":
				spy.receipt.FieldDigest = bytes.Repeat([]byte{9}, 32)
			case "actual_drift":
				spy.staff.Name = "已变更"
			}
			r := &audienceHistoryReferences{contacts: spy, run: 4, key: key}
			id, resolveErr := r.ResolveAudienceHistoryStaff(context.Background(), "staff-1")
			if scenario == "valid" {
				wantKey, keyErr := contactmigration.SourceKeyHMAC(key, audienceStaffSourceTable, "staff-1")
				if keyErr != nil || resolveErr != nil || id == nil || *id != 71 || spy.lockedSource != contactport.HistoricalImportOwnerRoleMap || !bytes.Equal(spy.lockedKey, wantKey) || spy.lockedTarget != 71 {
					t.Fatalf("id=%v err=%v source=%v target=%d", id, resolveErr, spy.lockedSource, spy.lockedTarget)
				}
				return
			}
			if scenario == "missing" {
				if resolveErr != nil || id != nil {
					t.Fatalf("missing lineage id=%v err=%v", id, resolveErr)
				}
				return
			}
			if id != nil || !errors.Is(resolveErr, v1domain.ErrConflict) {
				t.Fatalf("scenario=%s id=%v err=%v", scenario, id, resolveErr)
			}
		})
	}
}

func TestAudienceHistoryReferencesDelegateCustomerLineage(t *testing.T) {
	digest := bytes.Repeat([]byte{1}, 32)
	spy := &channelCustomerReferenceSpy{found: true, receiptFound: true,
		lineage: contactport.HistoricalImportLineage{TargetID: 72, LastRunID: 2, PayloadHMAC: digest, FieldDigest: digest},
		receipt: contactport.HistoricalImportRowReceipt{Disposition: contactport.HistoricalImportImported, PayloadHMAC: digest, FieldDigest: digest}}
	r := &audienceHistoryReferences{customer: &channelCustomerResolver{contacts: spy, run: 2, key: digest}}
	id, err := r.ResolveAudienceHistoryCustomer(context.Background(), "verified-unionid")
	if err != nil || id == nil || *id != 72 || spy.locked != 72 {
		t.Fatalf("customer id=%v err=%v", id, err)
	}
	if id, err = r.ResolveAudienceHistoryCustomer(context.Background(), ""); err != nil || id != nil {
		t.Fatalf("empty customer id=%v err=%v", id, err)
	}
}
