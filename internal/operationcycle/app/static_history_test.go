package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	cycle "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
)

func TestStaticCycleHistoryDigestsCoverTypedFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 456789000, time.UTC)
	optional := at.Add(time.Second)
	strategy := staticCycleStrategyFixture(at)
	strategy.ID = 1
	version := staticCycleVersionFixture(at)
	version.ID, version.StrategyHistoryID, version.EffectiveFrom, version.ConfirmedAt = 1, 2, &optional, nil
	document := staticCycleDocumentFixture(at)
	document.ID, document.VersionHistoryID, document.ExecutionGuideGeneratedAt, document.CopyGuideGeneratedAt, document.MeasurementGuideGeneratedAt = 1, 3, &optional, nil, &optional
	for _, test := range []struct {
		name   string
		value  any
		digest func(any) ([32]byte, error)
		change func(any)
	}{
		{"strategy_source", strategy, func(v any) ([32]byte, error) { return HistoricalCycleStrategyDigest(v.(cycle.HistoricalCycleStrategy)) }, func(v any) {
			x := v.(*cycle.HistoricalCycleStrategy)
			x.SourceID++
			x.StrategyKey += "x"
			x.Title += "x"
			x.Description += "x"
			x.Cadence += "x"
			x.Timezone += "x"
			x.OriginalStatus += "x"
			x.CurrentVersion++
			x.CreatedAt = x.CreatedAt.Add(time.Microsecond)
			x.UpdatedAt = x.UpdatedAt.Add(time.Microsecond)
			x.SourceKeyDigest[0]++
			x.SourcePayloadDigest[0]++
		}},
		{"version_source", version, func(v any) ([32]byte, error) { return HistoricalCycleVersionDigest(v.(cycle.HistoricalCycleVersion)) }, func(v any) {
			x := v.(*cycle.HistoricalCycleVersion)
			x.SourceID++
			x.StrategySourceID++
			x.StrategyHistoryID++
			x.Version++
			x.Label += "x"
			x.Objective += "x"
			x.VersionHash += "x"
			x.OriginalGovernance += "x"
			x.OperationSkillHash += "x"
			x.CreatedAt = x.CreatedAt.Add(time.Microsecond)
			x.SourceKeyDigest[0]++
			x.SourcePayloadDigest[0]++
		}},
		{"document_source", document, func(v any) ([32]byte, error) { return HistoricalCycleDocumentDigest(v.(cycle.HistoricalCycleDocument)) }, func(v any) {
			x := v.(*cycle.HistoricalCycleDocument)
			x.SourceID++
			x.StrategyVersionSourceID++
			x.VersionHistoryID++
			x.SchemaVersion += "x"
			x.ExecutionGuideSHA256 += "x"
			x.CopyGuideSHA256 += "x"
			x.MeasurementGuideSHA256 += "x"
			x.DocumentPackHash += "x"
			x.CreatedAt = x.CreatedAt.Add(time.Microsecond)
			x.SourceKeyDigest[0]++
			x.SourcePayloadDigest[0]++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseline, err := test.digest(test.value)
			if err != nil || baseline == ([32]byte{}) {
				t.Fatalf("baseline digest: %v", err)
			}
			switch value := test.value.(type) {
			case cycle.HistoricalCycleStrategy:
				changed := value
				test.change(&changed)
				digest, err := test.digest(changed)
				if err != nil || digest == baseline {
					t.Fatalf("strategy digest did not bind fields: %v", err)
				}
			case cycle.HistoricalCycleVersion:
				changed := value
				test.change(&changed)
				digest, err := test.digest(changed)
				if err != nil || digest == baseline {
					t.Fatalf("version digest did not bind fields: %v", err)
				}
			case cycle.HistoricalCycleDocument:
				changed := value
				test.change(&changed)
				digest, err := test.digest(changed)
				if err != nil || digest == baseline {
					t.Fatalf("document digest did not bind fields: %v", err)
				}
			}
		})
	}
	if _, err := HistoricalCycleVersionDigest(staticCycleVersionFixture(at)); !errors.Is(err, cycle.ErrStaticCycleHistoryInvalid) {
		t.Fatal("missing parent accepted")
	}
	if _, err := HistoricalCycleDocumentDigest(staticCycleDocumentFixture(at)); !errors.Is(err, cycle.ErrStaticCycleHistoryInvalid) {
		t.Fatal("missing parent accepted")
	}
}

func TestStaticCycleWriterReplaysAndRejectsDrift(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 456789123, time.FixedZone("+8", 8*3600))
	store := &staticCycleFakeStore{}
	journal := &staticCycleFakeJournal{}
	writer, err := NewStaticCycleHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	strategy := staticCycleStrategyFixture(at)
	strategy.SourceID, strategy.CurrentVersion, strategy.Title = -7, -9, ""
	source := hex.EncodeToString(strategy.SourceKeyDigest[:])
	receipt, err := writer.ImportCycleStrategy(context.Background(), source, strategy)
	if err != nil || receipt.Kind != staticCycleStrategyKind || receipt.Replayed || receipt.TargetID != 1 {
		t.Fatalf("first import = %#v, %v", receipt, err)
	}
	if store.strategy.CreatedAt.Location() != time.UTC || store.strategy.CreatedAt.Nanosecond() != 456789000 || !store.strategy.UpdatedAt.Before(store.strategy.CreatedAt) {
		t.Fatal("timestamp/source fidelity lost")
	}
	replay, err := writer.ImportCycleStrategy(context.Background(), source, strategy)
	if err != nil || !replay.Replayed || store.strategyCreates != 1 {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	changed := strategy
	changed.Title = "drift"
	if _, err := writer.ImportCycleStrategy(context.Background(), source, changed); !errors.Is(err, cycle.ErrStaticCycleHistoryConflict) {
		t.Fatalf("drift = %v", err)
	}
	if _, err := writer.ImportCycleStrategy(context.Background(), "bad", strategy); !errors.Is(err, cycle.ErrStaticCycleHistoryInvalid) {
		t.Fatalf("invalid source = %v", err)
	}

	version := staticCycleVersionFixture(at)
	version.StrategyHistoryID, version.StrategySourceID, version.Version = 1, -2, -3
	journal.found = false
	if _, err := writer.ImportCycleVersion(context.Background(), hex.EncodeToString(version.SourceKeyDigest[:]), version); err != nil {
		t.Fatalf("version import: %v", err)
	}
	document := staticCycleDocumentFixture(at)
	document.VersionHistoryID, document.StrategyVersionSourceID = 1, -3
	journal.found = false
	if _, err := writer.ImportCycleDocument(context.Background(), hex.EncodeToString(document.SourceKeyDigest[:]), document); err != nil {
		t.Fatalf("document import: %v", err)
	}
}

func TestNewStaticCycleHistoryWriterFailsClosed(t *testing.T) {
	var store *staticCycleFakeStore
	if writer, err := NewStaticCycleHistoryWriter(store, &staticCycleFakeJournal{}); writer != nil || !errors.Is(err, cycle.ErrStaticCycleHistoryUnavailable) {
		t.Fatalf("typed nil writer = %v, %v", writer, err)
	}
}

type staticCycleFakeStore struct {
	strategy        cycle.HistoricalCycleStrategy
	version         cycle.HistoricalCycleVersion
	document        cycle.HistoricalCycleDocument
	strategyCreates int
}

func (s *staticCycleFakeStore) CreateHistoricalCycleStrategy(_ context.Context, v cycle.HistoricalCycleStrategy) (cycle.HistoricalCycleStrategy, error) {
	s.strategyCreates++
	v.ID = int64(s.strategyCreates)
	s.strategy = v
	return v, nil
}
func (s *staticCycleFakeStore) GetHistoricalCycleStrategy(_ context.Context, id int64) (cycle.HistoricalCycleStrategy, error) {
	if s.strategy.ID != id {
		return cycle.HistoricalCycleStrategy{}, cycle.ErrStaticCycleHistoryUnavailable
	}
	return s.strategy, nil
}
func (s *staticCycleFakeStore) CreateHistoricalCycleVersion(_ context.Context, v cycle.HistoricalCycleVersion) (cycle.HistoricalCycleVersion, error) {
	v.ID = 1
	s.version = v
	return v, nil
}
func (s *staticCycleFakeStore) GetHistoricalCycleVersion(_ context.Context, id int64) (cycle.HistoricalCycleVersion, error) {
	if s.version.ID != id {
		return cycle.HistoricalCycleVersion{}, cycle.ErrStaticCycleHistoryUnavailable
	}
	return s.version, nil
}
func (s *staticCycleFakeStore) CreateHistoricalCycleDocument(_ context.Context, v cycle.HistoricalCycleDocument) (cycle.HistoricalCycleDocument, error) {
	v.ID = 1
	s.document = v
	return v, nil
}
func (s *staticCycleFakeStore) GetHistoricalCycleDocument(_ context.Context, id int64) (cycle.HistoricalCycleDocument, error) {
	if s.document.ID != id {
		return cycle.HistoricalCycleDocument{}, cycle.ErrStaticCycleHistoryUnavailable
	}
	return s.document, nil
}

type staticCycleFakeJournal struct {
	receipt cycle.StaticCycleHistoryReceipt
	found   bool
}

func (j *staticCycleFakeJournal) LoadStaticCycleHistory(_ context.Context, _, _ string) (cycle.StaticCycleHistoryReceipt, bool, error) {
	return j.receipt, j.found, nil
}
func (j *staticCycleFakeJournal) RecordStaticCycleHistory(_ context.Context, r cycle.StaticCycleHistoryReceipt) error {
	j.receipt, j.found = r, true
	return nil
}

func staticCycleStrategyFixture(at time.Time) cycle.HistoricalCycleStrategy {
	return cycle.HistoricalCycleStrategy{SourceID: 0, SourceKeyDigest: staticCycleDigestByte(1), SourcePayloadDigest: staticCycleDigestByte(2), StrategyKey: " key ", Title: "", Description: "\n", Cadence: "", Timezone: "", OriginalStatus: "", CurrentVersion: 0, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func staticCycleVersionFixture(at time.Time) cycle.HistoricalCycleVersion {
	return cycle.HistoricalCycleVersion{SourceID: 0, SourceKeyDigest: staticCycleDigestByte(3), SourcePayloadDigest: staticCycleDigestByte(4), StrategySourceID: 0, Version: 0, Label: "", Objective: "", VersionHash: "", OriginalGovernance: "", OperationSkillHash: "", CreatedAt: at}
}
func staticCycleDocumentFixture(at time.Time) cycle.HistoricalCycleDocument {
	return cycle.HistoricalCycleDocument{SourceID: 0, SourceKeyDigest: staticCycleDigestByte(5), SourcePayloadDigest: staticCycleDigestByte(6), StrategyVersionSourceID: 0, SchemaVersion: "", ExecutionGuideSHA256: "", CopyGuideSHA256: "", MeasurementGuideSHA256: "", DocumentPackHash: "", CreatedAt: at}
}
func staticCycleDigestByte(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
