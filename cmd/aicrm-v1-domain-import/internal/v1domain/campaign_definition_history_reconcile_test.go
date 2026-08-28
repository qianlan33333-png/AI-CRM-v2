package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"
	"time"

	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestVerifyCampaignDefinitionHistoryRowChecksEverySourceDigestAndParent(t *testing.T) {
	reader := campaignDefinitionHistoryReconcileFixture()
	for _, row := range []reconciliationRow{
		campaignDefinitionHistoryReconciliationRow(t, campaignDefinitionHistoryDefinitionTable, campaignDefinitionHistoryDefinitionTarget, reader.definition),
		campaignDefinitionHistoryReconciliationStepRow(t, reader.stepHistory),
		campaignDefinitionHistoryReconciliationStepRow(t, reader.stepCurrent),
		campaignDefinitionHistoryReconciliationStepRow(t, reader.stepUnresolved),
	} {
		if proof, err := campaignDefinitionHistoryVerifyRow(context.Background(), reader, row); err != nil || proof == "" {
			t.Fatalf("proof=%q err=%v", proof, err)
		}
	}
}

func TestVerifyCampaignDefinitionHistoryRowFailsClosedOnDrift(t *testing.T) {
	reader := campaignDefinitionHistoryReconcileFixture()
	row := campaignDefinitionHistoryReconciliationStepRow(t, reader.stepHistory)
	field := campaignDefinitionHistoryReconcileDigest(99)
	row.FieldDigest = field[:]
	if _, err := campaignDefinitionHistoryVerifyRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("field drift err=%v", err)
	}

	row = campaignDefinitionHistoryReconciliationStepRow(t, reader.stepHistory)
	reader.stepHistory.ContentDigest[0] ^= 1
	if _, err := campaignDefinitionHistoryVerifyRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("target drift err=%v", err)
	}

	reader = campaignDefinitionHistoryReconcileFixture()
	row = campaignDefinitionHistoryReconciliationStepRow(t, reader.stepHistory)
	reader.definition.SourceID++
	if _, err := campaignDefinitionHistoryVerifyRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("history parent source drift err=%v", err)
	}

	row = campaignDefinitionHistoryReconciliationStepRow(t, reader.stepCurrent)
	reader.stepCurrent.HistoryDefinitionID = campaignDefinitionHistoryReconcileID(3)
	if _, err := campaignDefinitionHistoryVerifyRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("mixed parent states accepted err=%v", err)
	}
}

func TestVerifyCampaignDefinitionHistoryRowRechecksCurrentParentReceipt(t *testing.T) {
	reader := campaignDefinitionHistoryReconcileFixture()
	row := campaignDefinitionHistoryReconciliationStepRow(t, reader.stepCurrent)
	if _, err := verifyCampaignDefinitionHistoryRow(context.Background(), reader, campaignDefinitionHistoryReconcileParentFake{code: "wrong"}, row, campaignDefinitionHistoryTestKey); !errors.Is(err, ErrConflict) {
		t.Fatalf("current receipt code drift err=%v", err)
	}
}

func TestReconcileCampaignDefinitionHistoryRejectsInvalidScopeBeforeDatabase(t *testing.T) {
	if _, err := ReconcileCampaignDefinitionHistory(context.Background(), nil, "other", "archive", campaignDefinitionHistoryTestKey); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong version err=%v", err)
	}
	if _, err := ReconcileCampaignDefinitionHistory(context.Background(), nil, campaignDefinitionHistoryImportVersion, "archive", campaignDefinitionHistoryTestKey[:sha256.Size-1]); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("short key err=%v", err)
	}
}

type campaignDefinitionHistoryReconcileReader struct {
	definition     campaignport.HistoricalCampaignDefinition
	stepHistory    campaignport.HistoricalCampaignDefinitionStep
	stepCurrent    campaignport.HistoricalCampaignDefinitionStep
	stepUnresolved campaignport.HistoricalCampaignDefinitionStep
}

type campaignDefinitionHistoryReconcileParentFake struct{ code string }

func (fake campaignDefinitionHistoryReconcileParentFake) ResolveVerifiedCurrentCampaignDefinition(_ context.Context, sourceID int64, sourceKey [sha256.Size]byte) (string, bool, error) {
	expected, err := v1archive.SourceKeyHMAC(campaignDefinitionHistoryTestKey, "campaigns", []byte("["+strconv.FormatInt(sourceID, 10)+"]"))
	if err != nil || expected != sourceKey {
		return "", false, ErrConflict
	}
	return fake.code, true, nil
}

func campaignDefinitionHistoryVerifyRow(ctx context.Context, reader campaignDefinitionHistoryReconcileReader, row reconciliationRow) (string, error) {
	return verifyCampaignDefinitionHistoryRow(ctx, reader, campaignDefinitionHistoryReconcileParentFake{code: "paused-current"}, row, campaignDefinitionHistoryTestKey)
}

func (reader campaignDefinitionHistoryReconcileReader) GetHistoricalCampaignDefinition(_ context.Context, id int64) (campaignport.HistoricalCampaignDefinition, error) {
	if id != reader.definition.ID {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return reader.definition, nil
}

func (reader campaignDefinitionHistoryReconcileReader) GetHistoricalCampaignDefinitionStep(_ context.Context, id int64) (campaignport.HistoricalCampaignDefinitionStep, error) {
	for _, value := range []campaignport.HistoricalCampaignDefinitionStep{reader.stepHistory, reader.stepCurrent, reader.stepUnresolved} {
		if value.ID == id {
			return value, nil
		}
	}
	return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryUnavailable
}

func (reader campaignDefinitionHistoryReconcileReader) ListHistoricalCampaignDefinitions(_ context.Context, _ int32, _ int32) ([]campaignport.HistoricalCampaignDefinition, int64, error) {
	return []campaignport.HistoricalCampaignDefinition{reader.definition}, 1, nil
}

func (reader campaignDefinitionHistoryReconcileReader) ListHistoricalCampaignDefinitionSteps(_ context.Context, _ *int64, _ int32, _ int32) ([]campaignport.HistoricalCampaignDefinitionStep, int64, error) {
	return []campaignport.HistoricalCampaignDefinitionStep{reader.stepHistory, reader.stepCurrent, reader.stepUnresolved}, 3, nil
}

func campaignDefinitionHistoryReconcileFixture() campaignDefinitionHistoryReconcileReader {
	at := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC)
	definition := campaignport.HistoricalCampaignDefinition{
		ID: 3, SourceID: -8, Code: "", DisplayName: "old definition", Intent: "", AnchorMode: "legacy", AnchorDate: "", ReviewStatus: "rejected", RunStatus: "stopped",
		CreatedAt: at, UpdatedAt: at, OriginalDisposition: "archive", OriginalReason: "legacy", PrivateDigest: campaignDefinitionHistoryReconcileDigest(1),
		SourceKeyDigest: campaignDefinitionHistoryReconcileDigest(2), SourcePayloadDigest: campaignDefinitionHistoryReconcileDigest(3), SourceFieldDigest: campaignDefinitionHistoryReconcileDigest(4), RedactedRoots: []string{"private"},
	}
	base := campaignport.HistoricalCampaignDefinitionStep{
		SourceID: -11, CampaignSourceID: -8, SegmentSourceID: 0, StepIndex: -1, DayOffset: 0, SendTime: "", Timezone: "Asia/Shanghai", ContentMasked: "masked", StopOnReply: false, SkipRecentDays: -1,
		CreatedAt: at, UpdatedAt: at, OriginalDisposition: "quarantine", OriginalReason: "legacy", ContentDigest: campaignDefinitionHistoryReconcileDigest(5), PrivateDigest: campaignDefinitionHistoryReconcileDigest(6),
		SourceKeyDigest: campaignDefinitionHistoryReconcileDigest(7), SourcePayloadDigest: campaignDefinitionHistoryReconcileDigest(8), SourceFieldDigest: campaignDefinitionHistoryReconcileDigest(9), RedactedRoots: []string{"content"},
	}
	history := base
	history.ID, history.HistoryDefinitionID, history.SourceParentState = 4, campaignDefinitionHistoryReconcileID(definition.ID), "history_definition"
	current := base
	current.ID, current.SourceID, current.SourceKeyDigest, current.SourcePayloadDigest, current.SourceFieldDigest = 5, -12, campaignDefinitionHistoryReconcileDigest(10), campaignDefinitionHistoryReconcileDigest(11), campaignDefinitionHistoryReconcileDigest(12)
	current.CampaignSourceID = -99
	current.SourceParentState, current.CurrentCampaignCode = "current_definition", campaignDefinitionHistoryReconcileString("paused-current")
	unresolved := base
	unresolved.ID, unresolved.SourceID, unresolved.SourceKeyDigest, unresolved.SourcePayloadDigest, unresolved.SourceFieldDigest = 6, -13, campaignDefinitionHistoryReconcileDigest(13), campaignDefinitionHistoryReconcileDigest(14), campaignDefinitionHistoryReconcileDigest(15)
	unresolved.SourceParentState = "unresolved_definition"
	return campaignDefinitionHistoryReconcileReader{definition: definition, stepHistory: history, stepCurrent: current, stepUnresolved: unresolved}
}

func campaignDefinitionHistoryReconciliationRow(t *testing.T, table, target string, value campaignport.HistoricalCampaignDefinition) reconciliationRow {
	t.Helper()
	digest, err := campaignapp.HistoricalCampaignDefinitionDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	row := campaignDefinitionHistoryReconciliationRowValue(table, target, value.ID, digest, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest)
	row.PriorDisposition, row.PriorReason = value.OriginalDisposition, value.OriginalReason
	return row
}

func campaignDefinitionHistoryReconciliationStepRow(t *testing.T, value campaignport.HistoricalCampaignDefinitionStep) reconciliationRow {
	t.Helper()
	digest, err := campaignapp.HistoricalCampaignDefinitionStepDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	row := campaignDefinitionHistoryReconciliationRowValue(campaignDefinitionHistoryStepTable, campaignDefinitionHistoryStepTarget, value.ID, digest, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest)
	row.PriorDisposition, row.PriorReason = value.OriginalDisposition, value.OriginalReason
	return row
}

func campaignDefinitionHistoryReconciliationRowValue(table, target string, id int64, digest, key, payload, field [sha256.Size]byte) reconciliationRow {
	domain, targetID := campaignDefinitionHistoryDomain, strconv.FormatInt(id, 10)
	return reconciliationRow{TableID: table, SourceKeyDigest: key[:], PayloadDigest: payload[:], FieldDigest: field[:], TargetDomain: &domain, TargetTable: &target, TargetID: &targetID, TargetDigest: digest[:]}
}

func campaignDefinitionHistoryReconcileDigest(value byte) [sha256.Size]byte {
	return sha256.Sum256([]byte{value})
}

func campaignDefinitionHistoryReconcileID(value int64) *int64       { return &value }
func campaignDefinitionHistoryReconcileString(value string) *string { return &value }
