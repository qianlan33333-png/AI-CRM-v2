package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	cycle "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
)

func TestCycleObservationDigestsCoverEveryFact(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789000, time.UTC)
	metric := cycleMetricFixture(1, at)
	metric.ID = 1
	metricCases := []func(*cycle.HistoricalCycleMetric){
		func(v *cycle.HistoricalCycleMetric) { v.ID++ },
		func(v *cycle.HistoricalCycleMetric) { v.SourceID++ },
		func(v *cycle.HistoricalCycleMetric) { v.SourceKeyDigest[0]++ },
		func(v *cycle.HistoricalCycleMetric) { v.SourcePayloadDigest[0]++ },
		func(v *cycle.HistoricalCycleMetric) { v.SourceFieldDigest[0]++ },
		func(v *cycle.HistoricalCycleMetric) { v.RunSourceID++ },
		func(v *cycle.HistoricalCycleMetric) { v.MetricKey += "x" },
		func(v *cycle.HistoricalCycleMetric) { v.Label += "x" },
		func(v *cycle.HistoricalCycleMetric) { n := *v.Numerator + 1; v.Numerator = &n },
		func(v *cycle.HistoricalCycleMetric) { n := *v.Denominator + 1; v.Denominator = &n },
		func(v *cycle.HistoricalCycleMetric) { n := *v.Value + 1; v.Value = &n },
		func(v *cycle.HistoricalCycleMetric) { v.Unit += "x" },
		func(v *cycle.HistoricalCycleMetric) { v.ObservationWindow += "x" },
		func(v *cycle.HistoricalCycleMetric) { v.DataSource += "x" },
		func(v *cycle.HistoricalCycleMetric) { v.DataQuality += "x" },
		func(v *cycle.HistoricalCycleMetric) { v.LimitationsJSON = append(v.LimitationsJSON, ' ') },
		func(v *cycle.HistoricalCycleMetric) { v.IsCausal = !v.IsCausal },
		func(v *cycle.HistoricalCycleMetric) { v.ValueStatus += "x" },
		func(v *cycle.HistoricalCycleMetric) { v.LastSnapshotSourceID++ },
		func(v *cycle.HistoricalCycleMetric) { v.CreatedAt = v.CreatedAt.Add(time.Microsecond) },
		func(v *cycle.HistoricalCycleMetric) { v.UpdatedAt = v.UpdatedAt.Add(time.Microsecond) },
	}
	assertDigestMutations(t, metric, HistoricalCycleMetricDigest, metricCases)

	reference := cycleReferenceFixture(2, at)
	reference.ID = 1
	referenceCases := []func(*cycle.HistoricalCycleReference){
		func(v *cycle.HistoricalCycleReference) { v.ID++ },
		func(v *cycle.HistoricalCycleReference) { v.SourceID++ },
		func(v *cycle.HistoricalCycleReference) { v.SourceKeyDigest[0]++ },
		func(v *cycle.HistoricalCycleReference) { v.SourcePayloadDigest[0]++ },
		func(v *cycle.HistoricalCycleReference) { v.SourceFieldDigest[0]++ },
		func(v *cycle.HistoricalCycleReference) { v.RunSourceID++ },
		func(v *cycle.HistoricalCycleReference) { v.ReferenceKey += "x" },
		func(v *cycle.HistoricalCycleReference) { v.ReferenceType += "x" },
		func(v *cycle.HistoricalCycleReference) { v.Label += "x" },
		func(v *cycle.HistoricalCycleReference) { v.SourceSystem += "x" },
		func(v *cycle.HistoricalCycleReference) { v.ReferenceSourceID += "x" },
		func(v *cycle.HistoricalCycleReference) { v.Href += "x" },
		func(v *cycle.HistoricalCycleReference) { v.EvidenceHash += "x" },
		func(v *cycle.HistoricalCycleReference) { v.DataStatus += "x" },
		func(v *cycle.HistoricalCycleReference) { v.LastSnapshotSourceID++ },
		func(v *cycle.HistoricalCycleReference) { v.CreatedAt = v.CreatedAt.Add(time.Microsecond) },
		func(v *cycle.HistoricalCycleReference) { v.UpdatedAt = v.UpdatedAt.Add(time.Microsecond) },
	}
	assertDigestMutations(t, reference, HistoricalCycleReferenceDigest, referenceCases)
}

func TestCycleMetricDigestDistinguishesRawJSONStatesAndRejectsNonFinite(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789000, time.UTC)
	states := []json.RawMessage{nil, json.RawMessage{}, json.RawMessage("null"), json.RawMessage("[]"), json.RawMessage(`"scalar"`)}
	digests := map[[32]byte]bool{}
	for _, raw := range states {
		value := cycleMetricFixture(3, at)
		value.ID, value.LimitationsJSON = 1, raw
		digest, err := HistoricalCycleMetricDigest(value)
		if err != nil || digests[digest] {
			t.Fatalf("raw JSON state digest=%x err=%v", digest, err)
		}
		digests[digest] = true
	}
	for _, number := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		value := cycleMetricFixture(4, at)
		value.ID, value.Value = 1, &number
		if _, err := HistoricalCycleMetricDigest(value); !errors.Is(err, cycle.ErrCycleObservationInvalid) {
			t.Fatalf("non-finite value = %v", err)
		}
	}
}

func TestCycleObservationWriterReplaysAndRejectsDrift(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789123, time.FixedZone("source", 8*3600))
	store := &cycleObservationStoreFake{metrics: map[int64]cycle.HistoricalCycleMetric{}, references: map[int64]cycle.HistoricalCycleReference{}}
	journal := &cycleObservationJournalFake{entries: map[string]cycle.CycleObservationReceipt{}}
	writer, err := NewCycleObservationWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	metric := cycleMetricFixture(5, at)
	metric.SourceID, metric.RunSourceID, metric.LastSnapshotSourceID = -7, 0, -9
	source := hex.EncodeToString(metric.SourceKeyDigest[:])
	receipt, err := writer.ImportHistoricalCycleMetric(context.Background(), source, metric)
	if err != nil || receipt.Kind != cycleMetricKind || receipt.Replayed || receipt.TargetID < 1 {
		t.Fatalf("metric first import = %#v, %v", receipt, err)
	}
	stored := store.metrics[receipt.TargetID]
	if stored.CreatedAt.Location() != time.UTC || stored.CreatedAt.Nanosecond() != 456789000 || string(stored.LimitationsJSON) != `[]` {
		t.Fatalf("metric source fidelity lost: %#v", stored)
	}
	metric.LimitationsJSON[0] = '{'
	if string(store.metrics[receipt.TargetID].LimitationsJSON) != `[]` {
		t.Fatal("metric raw JSON was aliased into store")
	}
	metric.LimitationsJSON = json.RawMessage("[]")
	replay, err := writer.ImportHistoricalCycleMetric(context.Background(), source, metric)
	if err != nil || !replay.Replayed || store.metricCreates != 1 || replay.TargetDigest != receipt.TargetDigest {
		t.Fatalf("metric replay = %#v, %v", replay, err)
	}
	metric.SourceFieldDigest[0]++
	if _, err := writer.ImportHistoricalCycleMetric(context.Background(), source, metric); !errors.Is(err, cycle.ErrCycleObservationConflict) {
		t.Fatalf("metric private drift = %v", err)
	}

	reference := cycleReferenceFixture(6, at)
	reference.SourceID, reference.RunSourceID, reference.LastSnapshotSourceID = 0, -1, -2
	source = hex.EncodeToString(reference.SourceKeyDigest[:])
	receipt, err = writer.ImportHistoricalCycleReference(context.Background(), source, reference)
	if err != nil || receipt.Kind != cycleReferenceKind || receipt.Replayed {
		t.Fatalf("reference first import = %#v, %v", receipt, err)
	}
	reference.Href += "/drift"
	if _, err := writer.ImportHistoricalCycleReference(context.Background(), source, reference); !errors.Is(err, cycle.ErrCycleObservationConflict) {
		t.Fatalf("private href drift = %v", err)
	}
}

func TestCycleObservationWriterFailsClosedAndDoesNotRecordFailedWrite(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	var store *cycleObservationStoreFake
	if writer, err := NewCycleObservationWriter(store, &cycleObservationJournalFake{}); writer != nil || !errors.Is(err, cycle.ErrCycleObservationUnavailable) {
		t.Fatalf("typed nil writer = %v, %v", writer, err)
	}
	store = &cycleObservationStoreFake{metrics: map[int64]cycle.HistoricalCycleMetric{}, references: map[int64]cycle.HistoricalCycleReference{}, createErr: errors.New("write failed")}
	journal := &cycleObservationJournalFake{entries: map[string]cycle.CycleObservationReceipt{}}
	writer, err := NewCycleObservationWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	metric := cycleMetricFixture(7, at)
	if _, err := writer.ImportHistoricalCycleMetric(context.Background(), hex.EncodeToString(metric.SourceKeyDigest[:]), metric); !errors.Is(err, cycle.ErrCycleObservationUnavailable) {
		t.Fatalf("write error = %v", err)
	}
	if len(journal.entries) != 0 {
		t.Fatalf("failed write recorded receipt: %#v", journal.entries)
	}
	metric = cycleMetricFixture(8, at)
	metric.SourceFieldDigest = [32]byte{}
	if _, err := writer.ImportHistoricalCycleMetric(context.Background(), hex.EncodeToString(metric.SourceKeyDigest[:]), metric); !errors.Is(err, cycle.ErrCycleObservationInvalid) {
		t.Fatalf("missing field HMAC = %v", err)
	}
	metric = cycleMetricFixture(9, at)
	if _, err := writer.ImportHistoricalCycleMetric(context.Background(), "wrong", metric); !errors.Is(err, cycle.ErrCycleObservationInvalid) {
		t.Fatalf("wrong source identifier = %v", err)
	}

	store.createErr = nil
	journal.loadErr = errors.New("receipt read failed")
	metric = cycleMetricFixture(10, at)
	if _, err := writer.ImportHistoricalCycleMetric(context.Background(), hex.EncodeToString(metric.SourceKeyDigest[:]), metric); !errors.Is(err, cycle.ErrCycleObservationUnavailable) {
		t.Fatalf("receipt read error = %v", err)
	}
	journal.loadErr = nil
	metric = cycleMetricFixture(11, at)
	source := hex.EncodeToString(metric.SourceKeyDigest[:])
	journal.entries[cycleMetricKind+":"+source] = cycle.CycleObservationReceipt{Kind: cycleMetricKind, SourceIdentifier: source}
	if _, err := writer.ImportHistoricalCycleMetric(context.Background(), source, metric); !errors.Is(err, cycle.ErrCycleObservationConflict) {
		t.Fatalf("malformed receipt = %v", err)
	}
	delete(journal.entries, cycleMetricKind+":"+source)
	journal.recordErr = errors.New("receipt write failed")
	metric = cycleMetricFixture(12, at)
	if _, err := writer.ImportHistoricalCycleMetric(context.Background(), hex.EncodeToString(metric.SourceKeyDigest[:]), metric); !errors.Is(err, cycle.ErrCycleObservationUnavailable) {
		t.Fatalf("receipt write error = %v", err)
	}
	if len(journal.entries) != 0 {
		t.Fatalf("failed receipt write recorded receipt: %#v", journal.entries)
	}
}

func assertDigestMutations[T any](t *testing.T, value T, digest func(T) ([32]byte, error), changes []func(*T)) {
	t.Helper()
	baseline, err := digest(value)
	if err != nil || baseline == ([32]byte{}) {
		t.Fatalf("baseline digest = %x, %v", baseline, err)
	}
	for index, change := range changes {
		changed := value
		change(&changed)
		actual, err := digest(changed)
		if err != nil || actual == baseline {
			t.Fatalf("mutation %d digest = %x, %v", index, actual, err)
		}
	}
}

type cycleObservationStoreFake struct {
	next             int64
	metrics          map[int64]cycle.HistoricalCycleMetric
	references       map[int64]cycle.HistoricalCycleReference
	metricCreates    int
	referenceCreates int
	createErr        error
}

func (s *cycleObservationStoreFake) CreateHistoricalCycleMetric(_ context.Context, value cycle.HistoricalCycleMetric) (cycle.HistoricalCycleMetric, error) {
	if s.createErr != nil {
		return cycle.HistoricalCycleMetric{}, s.createErr
	}
	s.next++
	s.metricCreates++
	value.ID = s.next
	s.metrics[value.ID] = value
	return value, nil
}

func (s *cycleObservationStoreFake) GetHistoricalCycleMetric(_ context.Context, id int64) (cycle.HistoricalCycleMetric, error) {
	value, ok := s.metrics[id]
	if !ok {
		return cycle.HistoricalCycleMetric{}, cycle.ErrCycleObservationUnavailable
	}
	return value, nil
}

func (s *cycleObservationStoreFake) CreateHistoricalCycleReference(_ context.Context, value cycle.HistoricalCycleReference) (cycle.HistoricalCycleReference, error) {
	if s.createErr != nil {
		return cycle.HistoricalCycleReference{}, s.createErr
	}
	s.next++
	s.referenceCreates++
	value.ID = s.next
	s.references[value.ID] = value
	return value, nil
}

func (s *cycleObservationStoreFake) GetHistoricalCycleReference(_ context.Context, id int64) (cycle.HistoricalCycleReference, error) {
	value, ok := s.references[id]
	if !ok {
		return cycle.HistoricalCycleReference{}, cycle.ErrCycleObservationUnavailable
	}
	return value, nil
}

type cycleObservationJournalFake struct {
	entries   map[string]cycle.CycleObservationReceipt
	loadErr   error
	recordErr error
}

func (j *cycleObservationJournalFake) LoadCycleObservation(_ context.Context, kind, source string) (cycle.CycleObservationReceipt, bool, error) {
	if j.loadErr != nil {
		return cycle.CycleObservationReceipt{}, false, j.loadErr
	}
	value, ok := j.entries[kind+":"+source]
	return value, ok, nil
}

func (j *cycleObservationJournalFake) RecordCycleObservation(_ context.Context, value cycle.CycleObservationReceipt) error {
	if j.recordErr != nil {
		return j.recordErr
	}
	if j.entries == nil {
		j.entries = map[string]cycle.CycleObservationReceipt{}
	}
	j.entries[value.Kind+":"+value.SourceIdentifier] = value
	return nil
}

func cycleMetricFixture(first byte, at time.Time) cycle.HistoricalCycleMetric {
	numerator, denominator, value := 1.5, -2.5, 0.0
	return cycle.HistoricalCycleMetric{SourceID: int64(first) - 10, SourceKeyDigest: cycleObservationDigestByte(first), SourcePayloadDigest: cycleObservationDigestByte(first + 30), SourceFieldDigest: cycleObservationDigestByte(first + 60), RunSourceID: -2, MetricKey: "key", Label: "label", Numerator: &numerator, Denominator: &denominator, Value: &value, Unit: "count", ObservationWindow: "week", DataSource: "legacy", DataQuality: "partial", LimitationsJSON: json.RawMessage("[]"), IsCausal: false, ValueStatus: "unknown", LastSnapshotSourceID: -3, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}

func cycleReferenceFixture(first byte, at time.Time) cycle.HistoricalCycleReference {
	return cycle.HistoricalCycleReference{SourceID: int64(first) - 10, SourceKeyDigest: cycleObservationDigestByte(first), SourcePayloadDigest: cycleObservationDigestByte(first + 30), SourceFieldDigest: cycleObservationDigestByte(first + 60), RunSourceID: -2, ReferenceKey: "key", ReferenceType: "type", Label: "label", SourceSystem: "legacy", ReferenceSourceID: "-3", Href: "https://private.example/reference", EvidenceHash: "evidence", DataStatus: "unknown", LastSnapshotSourceID: -4, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}

func cycleObservationDigestByte(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
