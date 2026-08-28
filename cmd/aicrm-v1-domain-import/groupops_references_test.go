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

type groupOpsStaffReferenceSpy struct {
	contactport.HistoricalImportTarget
	mode, state                string
	runErr, lockErr            error
	lineage                    contactport.HistoricalImportLineage
	receipt                    contactport.HistoricalImportRowReceipt
	lineageFound, receiptFound bool
	staff                      contactport.HistoricalImportStaffFact
	lockedSource               contactport.HistoricalImportSource
	lockedKey                  []byte
	lockedTarget               int64
}

func (s *groupOpsStaffReferenceSpy) ReadHistoricalImportRun(context.Context, int64) (string, string, error) {
	return s.mode, s.state, s.runErr
}

func (s *groupOpsStaffReferenceSpy) LockHistoricalImportSource(_ context.Context, source contactport.HistoricalImportSource, key []byte) error {
	s.lockedSource, s.lockedKey = source, append([]byte(nil), key...)
	return s.lockErr
}

func (s *groupOpsStaffReferenceSpy) LockHistoricalImportLineage(context.Context, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportLineage, bool, error) {
	return s.lineage, s.lineageFound, nil
}

func (s *groupOpsStaffReferenceSpy) FindHistoricalImportRowReceipt(context.Context, int64, contactport.HistoricalImportSource, []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	return s.receipt, s.receiptFound, nil
}

func (s *groupOpsStaffReferenceSpy) LockHistoricalImportStaffTarget(_ context.Context, id int64) (contactport.HistoricalImportStaffFact, error) {
	s.lockedTarget = id
	return s.staff, nil
}

func TestGroupOpsStaffResolverUsesVerifiedDM01Staff(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	staff := contactport.HistoricalImportStaffFact{
		WeComUserID: "staff-1", Name: "历史员工", Active: true,
		CreatedAt: time.Date(2026, 8, 1, 1, 2, 3, 4000, time.UTC),
		UpdatedAt: time.Date(2026, 8, 2, 1, 2, 3, 5000, time.UTC),
	}
	field, err := groupOpsStaffTargetDigest(key, staff)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{3}, 32)

	for _, scenario := range []string{"valid", "missing_lineage", "wrong_run", "missing_receipt", "receipt_drift", "staff_drift"} {
		t.Run(scenario, func(t *testing.T) {
			spy := &groupOpsStaffReferenceSpy{
				lineageFound: true, receiptFound: true, staff: staff,
				lineage: contactport.HistoricalImportLineage{TargetID: 71, LastRunID: 4, PayloadHMAC: payload, FieldDigest: field},
				receipt: contactport.HistoricalImportRowReceipt{Disposition: contactport.HistoricalImportImported, PayloadHMAC: payload, FieldDigest: field},
			}
			switch scenario {
			case "missing_lineage":
				spy.lineageFound = false
			case "wrong_run":
				spy.lineage.LastRunID++
			case "missing_receipt":
				spy.receiptFound = false
			case "receipt_drift":
				spy.receipt.FieldDigest = bytes.Repeat([]byte{9}, 32)
			case "staff_drift":
				spy.staff.Name = "已变更"
			}
			resolver := &groupOpsStaffResolver{contacts: spy, runID: 4, key: key}
			id, resolveErr := resolver.ResolveGroupOpsStaff(context.Background(), "staff-1")
			if scenario == "valid" {
				wantKey, keyErr := contactmigration.SourceKeyHMAC(key, groupOpsStaffSourceTable, "staff-1")
				if keyErr != nil || resolveErr != nil || id == nil || *id != 71 || spy.lockedSource != contactport.HistoricalImportOwnerRoleMap || !bytes.Equal(spy.lockedKey, wantKey) || spy.lockedTarget != 71 {
					t.Fatalf("id=%v err=%v source=%v target=%d", id, resolveErr, spy.lockedSource, spy.lockedTarget)
				}
				return
			}
			if scenario == "missing_lineage" {
				if resolveErr != nil || id != nil {
					t.Fatalf("missing lineage result=%v err=%v", id, resolveErr)
				}
				return
			}
			if id != nil || !errors.Is(resolveErr, v1domain.ErrConflict) {
				t.Fatalf("scenario=%s id=%v err=%v", scenario, id, resolveErr)
			}
		})
	}
}

func TestGroupOpsStaffResolverRejectsUnverifiedSourceAndInvalidRun(t *testing.T) {
	key := bytes.Repeat([]byte{4}, 32)
	resolver := &groupOpsStaffResolver{contacts: &groupOpsStaffReferenceSpy{}, runID: 2, key: key}
	if id, err := resolver.ResolveGroupOpsStaff(context.Background(), " staff-1"); id != nil || !errors.Is(err, v1domain.ErrConflict) {
		t.Fatalf("uncanonical source id=%v err=%v", id, err)
	}
	for _, value := range []struct {
		name, mode, state string
		err               error
	}{
		{"imported", "full", "imported", nil},
		{"wrong_mode", "preflight", "imported", v1domain.ErrConflict},
		{"wrong_state", "full", "reconciled", v1domain.ErrConflict},
	} {
		t.Run(value.name, func(t *testing.T) {
			spy := &groupOpsStaffReferenceSpy{mode: value.mode, state: value.state}
			err := validateGroupOpsStaffRun(context.Background(), spy, 2)
			if value.err == nil && err != nil {
				t.Fatalf("valid run error=%v", err)
			}
			if value.err != nil && !errors.Is(err, value.err) {
				t.Fatalf("error=%v want=%v", err, value.err)
			}
		})
	}
}
