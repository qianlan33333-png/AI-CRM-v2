package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

func TestCampaignDefinitionHistoryWriterCreatesAndReplaysDefinitionAndStep(t *testing.T) {
	store := &campaignDefinitionHistoryStoreFake{}
	journal := &campaignDefinitionHistoryJournalFake{receipts: map[string]campaignport.CampaignHistoryReceipt{}}
	writer := NewCampaignDefinitionHistoryWriter(store, journal)
	definition, step := campaignDefinitionHistoryFixtures()
	definitionSource := hex.EncodeToString(definition.SourceKeyDigest[:])
	stepSource := hex.EncodeToString(step.SourceKeyDigest[:])

	definitionReceipt, err := writer.WriteDefinition(context.Background(), definitionSource, definition)
	if err != nil || definitionReceipt.Replayed || definitionReceipt.TargetID < 1 {
		t.Fatalf("definition first=%#v err=%v", definitionReceipt, err)
	}
	definition.RedactedRoots[0] = "changed-after-write"
	definitionReplay, err := writer.WriteDefinition(context.Background(), definitionSource, campaignDefinitionHistoryFixturesDefinition())
	if err != nil || !definitionReplay.Replayed || definitionReplay.TargetDigest != definitionReceipt.TargetDigest || store.definitions != 1 || store.definitionGets != 1 {
		t.Fatalf("definition replay=%#v store=%#v err=%v", definitionReplay, store, err)
	}
	if len(store.definition.RedactedRoots) != 1 || store.definition.RedactedRoots[0] != "payload.secret" || store.definition.CreatedAt.Location() != time.UTC || store.definition.CreatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("definition normalization/clone=%#v", store.definition)
	}

	stepReceipt, err := writer.WriteStep(context.Background(), stepSource, step)
	if err != nil || stepReceipt.Replayed || stepReceipt.TargetID < 1 {
		t.Fatalf("step first=%#v err=%v", stepReceipt, err)
	}
	stepReplay, err := writer.WriteStep(context.Background(), stepSource, step)
	if err != nil || !stepReplay.Replayed || stepReplay.TargetDigest != stepReceipt.TargetDigest || store.steps != 1 || store.stepGets != 1 {
		t.Fatalf("step replay=%#v store=%#v err=%v", stepReplay, store, err)
	}
}

func TestCampaignDefinitionHistoryWriterRejectsTargetDriftAndUnsafeFacts(t *testing.T) {
	store := &campaignDefinitionHistoryStoreFake{}
	journal := &campaignDefinitionHistoryJournalFake{receipts: map[string]campaignport.CampaignHistoryReceipt{}}
	writer := NewCampaignDefinitionHistoryWriter(store, journal)
	definition, step := campaignDefinitionHistoryFixtures()
	definitionSource := hex.EncodeToString(definition.SourceKeyDigest[:])
	if _, err := writer.WriteDefinition(context.Background(), definitionSource, definition); err != nil {
		t.Fatal(err)
	}
	store.definition.PrivateDigest[0] ^= 1
	if _, err := writer.WriteDefinition(context.Background(), definitionSource, definition); !errors.Is(err, campaignport.ErrCampaignHistoryConflict) {
		t.Fatalf("definition target drift err=%v", err)
	}

	for name, mutate := range map[string]func(*campaignport.HistoricalCampaignDefinitionStep){
		"missing history parent": func(value *campaignport.HistoricalCampaignDefinitionStep) { value.HistoryDefinitionID = nil },
		"both parents": func(value *campaignport.HistoricalCampaignDefinitionStep) {
			value.CurrentCampaignCode = campaignDefinitionHistoryString("current-code")
		},
		"empty current code": func(value *campaignport.HistoricalCampaignDefinitionStep) {
			value.HistoryDefinitionID = nil
			value.SourceParentState = "current_definition"
			value.CurrentCampaignCode = campaignDefinitionHistoryString("")
		},
		"bad disposition":     func(value *campaignport.HistoricalCampaignDefinitionStep) { value.OriginalDisposition = "import" },
		"empty reason":        func(value *campaignport.HistoricalCampaignDefinitionStep) { value.OriginalReason = "" },
		"zero content digest": func(value *campaignport.HistoricalCampaignDefinitionStep) { value.ContentDigest = [32]byte{} },
		"nul content":         func(value *campaignport.HistoricalCampaignDefinitionStep) { value.ContentMasked = "bad\x00" },
	} {
		t.Run(name, func(t *testing.T) {
			value := step
			mutate(&value)
			if _, err := writer.WriteStep(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	wrongSource := campaignDefinitionHistoryDigest(99)
	if _, err := writer.WriteStep(context.Background(), hex.EncodeToString(wrongSource[:]), step); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("source key mismatch err=%v", err)
	}
	var nilStore *campaignDefinitionHistoryStoreFake
	if _, err := NewCampaignDefinitionHistoryWriter(nilStore, journal).WriteStep(context.Background(), hex.EncodeToString(step.SourceKeyDigest[:]), step); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
		t.Fatalf("typed nil store err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writer.WriteStep(ctx, hex.EncodeToString(step.SourceKeyDigest[:]), step); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
		t.Fatalf("cancelled context err=%v", err)
	}
}

func TestCampaignDefinitionHistoryDigestIncludesPrivateAndParentFields(t *testing.T) {
	definition, step := campaignDefinitionHistoryFixtures()
	definition.ID = 7
	first, err := HistoricalCampaignDefinitionDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	definition.PrivateDigest[0] ^= 1
	changed, err := HistoricalCampaignDefinitionDigest(definition)
	if err != nil || changed == first {
		t.Fatalf("private digest omitted: %x %v", changed, err)
	}
	definition = campaignDefinitionHistoryFixturesDefinition()
	definition.ID = 7
	definition.SourceFieldDigest[0] ^= 1
	changed, err = HistoricalCampaignDefinitionDigest(definition)
	if err != nil || changed == first {
		t.Fatalf("source field digest omitted: %x %v", changed, err)
	}
	definition = campaignDefinitionHistoryFixturesDefinition()
	definition.ID = 7
	definition.RedactedRoots = []string{"different.root"}
	changed, err = HistoricalCampaignDefinitionDigest(definition)
	if err != nil || changed == first {
		t.Fatalf("redacted roots omitted: %x %v", changed, err)
	}
	definition = campaignDefinitionHistoryFixturesDefinition()
	definition.ID = 7
	definition.RedactedRoots = nil
	nilRoots, err := HistoricalCampaignDefinitionDigest(definition)
	definition.RedactedRoots = []string{}
	emptyRoots, emptyErr := HistoricalCampaignDefinitionDigest(definition)
	if err != nil || emptyErr != nil || nilRoots != emptyRoots {
		t.Fatalf("nil roots not canonical: %v %v", err, emptyErr)
	}

	step.ID = 8
	stepDigest, err := HistoricalCampaignDefinitionStepDigest(step)
	if err != nil {
		t.Fatal(err)
	}
	step.CurrentCampaignCode = nil
	step.HistoryDefinitionID = nil
	step.SourceParentState = "unresolved_definition"
	parentChanged, err := HistoricalCampaignDefinitionStepDigest(step)
	if err != nil || parentChanged == stepDigest {
		t.Fatalf("parent fields omitted: %x %v", parentChanged, err)
	}
	step = campaignDefinitionHistoryFixturesStep()
	step.ID = 8
	step.ContentDigest[0] ^= 1
	contentChanged, err := HistoricalCampaignDefinitionStepDigest(step)
	if err != nil || contentChanged == stepDigest {
		t.Fatalf("content digest omitted: %x %v", contentChanged, err)
	}
	step = campaignDefinitionHistoryFixturesStep()
	step.ID = 8
	step.HistoryDefinitionID = nil
	step.SourceParentState = "current_definition"
	step.CurrentCampaignCode = campaignDefinitionHistoryString("legacy-current-code")
	currentCodeDigest, err := HistoricalCampaignDefinitionStepDigest(step)
	if err != nil {
		t.Fatal(err)
	}
	*step.CurrentCampaignCode = "different-current-code"
	changedCurrentCode, err := HistoricalCampaignDefinitionStepDigest(step)
	if err != nil || currentCodeDigest == changedCurrentCode {
		t.Fatalf("current campaign code omitted: %x %v", changedCurrentCode, err)
	}
}

type campaignDefinitionHistoryStoreFake struct {
	definition               campaignport.HistoricalCampaignDefinition
	step                     campaignport.HistoricalCampaignDefinitionStep
	definitions, steps       int
	definitionGets, stepGets int
}

func (store *campaignDefinitionHistoryStoreFake) CreateHistoricalCampaignDefinition(_ context.Context, value campaignport.HistoricalCampaignDefinition) (campaignport.HistoricalCampaignDefinition, error) {
	store.definitions++
	value.ID = 101
	store.definition = value
	return value, nil
}
func (store *campaignDefinitionHistoryStoreFake) GetHistoricalCampaignDefinition(_ context.Context, id int64) (campaignport.HistoricalCampaignDefinition, error) {
	store.definitionGets++
	if id != store.definition.ID {
		return campaignport.HistoricalCampaignDefinition{}, campaignport.ErrCampaignHistoryConflict
	}
	return store.definition, nil
}
func (store *campaignDefinitionHistoryStoreFake) CreateHistoricalCampaignDefinitionStep(_ context.Context, value campaignport.HistoricalCampaignDefinitionStep) (campaignport.HistoricalCampaignDefinitionStep, error) {
	store.steps++
	value.ID = 201
	store.step = value
	return value, nil
}
func (store *campaignDefinitionHistoryStoreFake) GetHistoricalCampaignDefinitionStep(_ context.Context, id int64) (campaignport.HistoricalCampaignDefinitionStep, error) {
	store.stepGets++
	if id != store.step.ID {
		return campaignport.HistoricalCampaignDefinitionStep{}, campaignport.ErrCampaignHistoryConflict
	}
	return store.step, nil
}

type campaignDefinitionHistoryJournalFake struct {
	receipts map[string]campaignport.CampaignHistoryReceipt
}

func (journal *campaignDefinitionHistoryJournalFake) LoadCampaignDefinitionHistory(_ context.Context, kind, source string) (campaignport.CampaignHistoryReceipt, bool, error) {
	receipt, found := journal.receipts[kind+"/"+source]
	return receipt, found, nil
}
func (journal *campaignDefinitionHistoryJournalFake) RecordCampaignDefinitionHistory(_ context.Context, kind string, receipt campaignport.CampaignHistoryReceipt) error {
	if journal.receipts == nil {
		journal.receipts = map[string]campaignport.CampaignHistoryReceipt{}
	}
	journal.receipts[kind+"/"+receipt.SourceIdentifier] = receipt
	return nil
}

func campaignDefinitionHistoryFixtures() (campaignport.HistoricalCampaignDefinition, campaignport.HistoricalCampaignDefinitionStep) {
	return campaignDefinitionHistoryFixturesDefinition(), campaignDefinitionHistoryFixturesStep()
}

func campaignDefinitionHistoryFixturesStep() campaignport.HistoricalCampaignDefinitionStep {
	return campaignport.HistoricalCampaignDefinitionStep{
		SourceID: -11, CampaignSourceID: -5, SegmentSourceID: 0, HistoryDefinitionID: campaignDefinitionHistoryPointer(101), SourceParentState: "history_definition",
		StepIndex: -2, DayOffset: 0, SendTime: "", Timezone: "Asia/Shanghai", ContentMasked: "masked", StopOnReply: false, SkipRecentDays: -1,
		CreatedAt: time.Date(2026, 8, 28, 10, 0, 0, 123456789, time.FixedZone("legacy", 8*3600)), UpdatedAt: time.Date(2026, 8, 28, 10, 1, 0, 987654321, time.FixedZone("legacy", 8*3600)),
		OriginalDisposition: "quarantine", OriginalReason: "old step", ContentDigest: campaignDefinitionHistoryDigest(21), PrivateDigest: campaignDefinitionHistoryDigest(22),
		SourceKeyDigest: campaignDefinitionHistoryDigest(23), SourcePayloadDigest: campaignDefinitionHistoryDigest(24), SourceFieldDigest: campaignDefinitionHistoryDigest(25), RedactedRoots: []string{"content"},
	}
}

func campaignDefinitionHistoryFixturesDefinition() campaignport.HistoricalCampaignDefinition {
	return campaignport.HistoricalCampaignDefinition{
		SourceID: -9, Code: "", DisplayName: "old campaign", Intent: "", AnchorMode: "legacy", AnchorDate: "", ReviewStatus: "rejected", RunStatus: "stopped",
		ApprovedAt: nil, StartedAt: nil, FinishedAt: nil, PausedAt: nil, PausedReason: "", CreatedAt: time.Date(2026, 8, 28, 9, 0, 0, 123456789, time.FixedZone("legacy", 8*3600)), UpdatedAt: time.Date(2026, 8, 28, 9, 1, 0, 987654321, time.FixedZone("legacy", 8*3600)),
		OriginalDisposition: "archive", OriginalReason: "old code", PrivateDigest: campaignDefinitionHistoryDigest(11), SourceKeyDigest: campaignDefinitionHistoryDigest(12), SourcePayloadDigest: campaignDefinitionHistoryDigest(13), SourceFieldDigest: campaignDefinitionHistoryDigest(14), RedactedRoots: []string{"payload.secret"},
	}
}

func campaignDefinitionHistoryDigest(value byte) [32]byte  { return sha256.Sum256([]byte{value}) }
func campaignDefinitionHistoryPointer(value int64) *int64  { return &value }
func campaignDefinitionHistoryString(value string) *string { return &value }
