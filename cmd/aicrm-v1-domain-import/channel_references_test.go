package main

import (
	"context"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type channelCustomerReferenceSpy struct {
	contactport.HistoricalImportTarget
	lineage             contactport.HistoricalImportLineage
	receipt             contactport.HistoricalImportRowReceipt
	found, receiptFound bool
	rootErr             error
	locked              int64
}

func (s *channelCustomerReferenceSpy) LockHistoricalImportSource(context.Context, contactport.HistoricalImportSource, []byte) error {
	return nil
}
func (s *channelCustomerReferenceSpy) LockHistoricalImportLineage(context.Context, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportLineage, bool, error) {
	return s.lineage, s.found, nil
}
func (s *channelCustomerReferenceSpy) FindHistoricalImportRowReceipt(context.Context, int64, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	return s.receipt, s.receiptFound, nil
}
func (s *channelCustomerReferenceSpy) LockHistoricalImportCustomerTarget(_ context.Context, id int64) (contactport.HistoricalImportCustomerFact, error) {
	s.locked = id
	return contactport.HistoricalImportCustomerFact{}, nil
}
func (s *channelCustomerReferenceSpy) ValidateHistoricalImportCustomerRoot(context.Context, int64) error {
	return s.rootErr
}

func TestChannelCustomerReferenceRequiresVerifiedDM01Lineage(t *testing.T) {
	digest := make([]byte, 32)
	digest[0] = 1
	for _, scenario := range []string{"valid", "missing", "wrong_run", "missing_receipt", "wrong_digest", "root_conflict"} {
		t.Run(scenario, func(t *testing.T) {
			s := &channelCustomerReferenceSpy{found: true, receiptFound: true,
				lineage: contactport.HistoricalImportLineage{TargetID: 72, LastRunID: 2, PayloadHMAC: digest, FieldDigest: digest},
				receipt: contactport.HistoricalImportRowReceipt{Disposition: contactport.HistoricalImportImported, PayloadHMAC: digest, FieldDigest: digest}}
			switch scenario {
			case "missing":
				s.found = false
			case "wrong_run":
				s.lineage.LastRunID = 3
			case "missing_receipt":
				s.receiptFound = false
			case "wrong_digest":
				s.receipt.PayloadHMAC = make([]byte, 32)
			case "root_conflict":
				s.rootErr = v1domain.ErrConflict
			}
			r := &channelCustomerResolver{contacts: s, run: 2, key: digest}
			id, err := r.ResolveHistoricalChannelCustomer(context.Background(), "verified-unionid")
			if scenario == "valid" {
				if err != nil || id == nil || *id != 72 || s.locked != 72 {
					t.Fatalf("valid reference failed: %v", err)
				}
				return
			}
			if scenario == "missing" {
				if err != nil || id != nil {
					t.Fatal("missing lineage must be NULL")
				}
				return
			}
			if id != nil || !errors.Is(err, v1domain.ErrConflict) {
				t.Fatalf("drift accepted: %v", err)
			}
		})
	}
}
