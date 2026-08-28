package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type memberGridHistoryReaderFake struct {
	view  productport.HistoricalMemberView
	usage productport.HistoricalMemberUsage
	err   error
}

func (fake memberGridHistoryReaderFake) GetHistoricalMemberView(_ context.Context, id int64) (productport.HistoricalMemberView, error) {
	if fake.err != nil || fake.view.ID != id {
		return productport.HistoricalMemberView{}, errors.New("view missing")
	}
	return fake.view, nil
}

func (fake memberGridHistoryReaderFake) ListHistoricalMemberViews(context.Context, productport.MemberGridHistoryQuery) ([]productport.HistoricalMemberView, int64, error) {
	return nil, 0, errors.New("not used")
}

func (fake memberGridHistoryReaderFake) GetHistoricalMemberUsage(_ context.Context, id int64) (productport.HistoricalMemberUsage, error) {
	if fake.err != nil || fake.usage.ID != id {
		return productport.HistoricalMemberUsage{}, errors.New("usage missing")
	}
	return fake.usage, nil
}

func (fake memberGridHistoryReaderFake) ListHistoricalMemberUsage(context.Context, productport.MemberGridHistoryQuery) ([]productport.HistoricalMemberUsage, int64, error) {
	return nil, 0, errors.New("not used")
}

func TestVerifyMemberGridHistoryRowChecksCompleteTypedTarget(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	view := productport.HistoricalMemberView{ID: 11, SourceViewID: 7, SourceServiceProductID: 8, Name: "", Position: -1, SchemaVersion: -2, Version: -3,
		CreatedAt: at, UpdatedAt: at, SourceKeyDigest: memberGridHistoryReconcileDigest(1), ConfigDigest: memberGridHistoryReconcileDigest(2), SourcePayloadDigest: memberGridHistoryReconcileDigest(3)}
	usage := productport.HistoricalMemberUsage{ID: 12, FormallyLoggedIn: true, HasTokenUsage: true, LearningPlanID: "", OpenCount7D: 0, RefreshedAt: at,
		SourceKeyDigest: memberGridHistoryReconcileDigest(4), SourcePayloadDigest: memberGridHistoryReconcileDigest(5), RecoveryEntryDigest: memberGridHistoryReconcileDigest(6)}
	reader := memberGridHistoryReaderFake{view: view, usage: usage}
	viewDigest, err := productapp.HistoricalMemberViewDigest(view)
	if err != nil {
		t.Fatal(err)
	}
	usageDigest, err := productapp.HistoricalMemberUsageDigest(usage)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		row  reconciliationRow
	}{
		{name: "view", row: memberGridHistoryReconcileRow(v1membergridhistory.MemberViewsTableID, memberGridHistoryViewTargetTable, view.ID, view.SourceKeyDigest, view.SourcePayloadDigest, viewDigest)},
		{name: "usage", row: memberGridHistoryReconcileRow(v1membergridhistory.UsageSnapshotsTableID, memberGridHistoryUsageTargetTable, usage.ID, usage.SourceKeyDigest, usage.SourcePayloadDigest, usageDigest)},
	} {
		t.Run(test.name, func(t *testing.T) {
			proof, verifyErr := verifyMemberGridHistoryRow(context.Background(), reader, test.row)
			if verifyErr != nil || proof == "" {
				t.Fatalf("proof=%q err=%v", proof, verifyErr)
			}
		})
	}
	bad := memberGridHistoryReconcileRow(v1membergridhistory.MemberViewsTableID, memberGridHistoryViewTargetTable, view.ID, view.SourceKeyDigest, view.SourcePayloadDigest, viewDigest)
	bad.TargetDigest[0] ^= 1
	if _, err = verifyMemberGridHistoryRow(context.Background(), reader, bad); err == nil {
		t.Fatal("target_digest_drift_accepted")
	}
	bad = memberGridHistoryReconcileRow(v1membergridhistory.MemberViewsTableID, memberGridHistoryUsageTargetTable, view.ID, view.SourceKeyDigest, view.SourcePayloadDigest, viewDigest)
	if _, err = verifyMemberGridHistoryRow(context.Background(), reader, bad); err == nil {
		t.Fatal("wrong_target_table_accepted")
	}
}

func TestReconcileMemberGridHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	var pool *pgxpool.Pool
	if _, err := ReconcileMemberGridHistory(context.Background(), pool, "v1-member-grid-history-a2", "archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("err=%v", err)
	}
}

func memberGridHistoryReconcileRow(tableID, targetTable string, id int64, source, payload, digest [sha256.Size]byte) reconciliationRow {
	domain, targetID := "product", strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: tableID, SourceKeyDigest: source[:], PayloadDigest: payload[:], TargetDomain: &domain, TargetTable: &targetTable,
		TargetID: &targetID, TargetDigest: digest[:]}
}

func memberGridHistoryReconcileDigest(first byte) [sha256.Size]byte {
	var value [sha256.Size]byte
	value[0] = first
	return value
}
