package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	cyclehistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1cycleobservationhistory"
	cycleapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
)

func TestCycleObservationFactsPreserveFullSourceShape(t *testing.T) {
	at := time.Date(2026, 8, 29, 8, 9, 10, 123456789, time.FixedZone("source", 8*3600))
	metricSource := cycleObservationMetricFixture(at)
	metric, err := cycleObservationMetricFact(metricSource)
	if err != nil {
		t.Fatal(err)
	}
	metric.ID = 1
	if _, err = cycleapp.HistoricalCycleMetricDigest(metric); err != nil || metric.SourceID != -1 || metric.RunSourceID != 0 || metric.LastSnapshotSourceID != -2 || metric.Numerator == nil || *metric.Numerator != -1.5 || metric.Denominator != nil || metric.Value == nil || *metric.Value != 0 || string(metric.LimitationsJSON) != "null" || metric.CreatedAt.Location() != time.UTC || metric.CreatedAt.Nanosecond() != 123456000 {
		t.Fatalf("metric=%+v err=%v", metric, err)
	}
	metricSource.LimitationsJSON[0] = '['
	if string(metric.LimitationsJSON) != "null" {
		t.Fatal("limitations JSON aliases private source fact")
	}

	referenceSource := cycleObservationReferenceFixture(at)
	reference, err := cycleObservationReferenceFact(referenceSource)
	if err != nil {
		t.Fatal(err)
	}
	reference.ID = 1
	if _, err = cycleapp.HistoricalCycleReferenceDigest(reference); err != nil || reference.SourceID != 0 || reference.RunSourceID != -3 || reference.LastSnapshotSourceID != -4 || reference.ReferenceSourceID != "" || reference.Href != "https://private.example/path?token=kept-private" || reference.CreatedAt.Location() != time.UTC || reference.CreatedAt.Nanosecond() != 123456000 {
		t.Fatalf("reference=%+v err=%v", reference, err)
	}
	encoded, err := json.Marshal(reference)
	if err != nil || strings.Contains(string(encoded), reference.Href) {
		t.Fatalf("private href leaked: %s err=%v", encoded, err)
	}
}

func TestCycleObservationEntriesBindExactScopesAndSort(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789000, time.UTC)
	selection := cyclehistory.Selection{
		Metrics:    []cyclehistory.MetricCandidate{{SourceOrdinal: 1, Fact: cycleObservationMetricFixture(at)}},
		References: []cyclehistory.ReferenceCandidate{{SourceOrdinal: 1, Fact: cycleObservationReferenceFixture(at)}},
	}
	entries, err := cycleObservationEntries(context.Background(), selection, "v1-full-archive-20260827", cycleObservationTxStub{})
	if err != nil || len(entries) != 2 || entries[0].scope.TableID != cyclehistory.MetricsTableID || entries[0].scope.TargetDomain != cycleObservationDomain || entries[0].scope.TargetTable != cycleObservationMetricTarget || entries[1].scope.TableID != cyclehistory.ReferencesTableID || entries[1].scope.TargetTable != cycleObservationReferenceTarget {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	if entries[0].kind != cycleObservationMetricKind || entries[1].kind != cycleObservationReferenceKind || entries[0].source != SourceIdentifier(entries[0].key) || entries[1].payload == ([sha256.Size]byte{}) || entries[1].field == ([sha256.Size]byte{}) {
		t.Fatal("entry source envelope or kind lost")
	}
	if _, err = cycleObservationEntries(context.Background(), cyclehistory.Selection{Metrics: []cyclehistory.MetricCandidate{{SourceOrdinal: 0, Fact: cycleObservationMetricFixture(at)}}}, "run", cycleObservationTxStub{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("invalid ordinal err=%v", err)
	}
	if _, err = cycleObservationEntries(context.Background(), selection, "run", nil); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("nil tx err=%v", err)
	}
}

func TestCycleObservationHistoryJournalRejectsMalformedReplay(t *testing.T) {
	bridge := cycleObservationHistoryJournal{journal: &Journal{scope: Scope{ImportVersion: CycleObservationHistoryVersion, ArchiveRunID: "run", AdapterID: "v1_full_archive", TableID: cyclehistory.MetricsTableID, TargetDomain: cycleObservationDomain, TargetTable: cycleObservationMetricTarget}}, kind: cycleObservationMetricKind}
	source := SourceIdentifier(cycleObservationDigest("source"))
	if err := bridge.RecordCycleObservation(context.Background(), cycleport.CycleObservationReceipt{Kind: cycleObservationMetricKind, SourceIdentifier: source, PayloadDigest: cycleObservationDigest("payload"), TargetDigest: cycleObservationDigest("target"), TargetID: 1, Replayed: true}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("replayed record err=%v", err)
	}
	if err := bridge.RecordCycleObservation(context.Background(), cycleport.CycleObservationReceipt{Kind: cycleObservationReferenceKind, SourceIdentifier: source, PayloadDigest: cycleObservationDigest("payload"), TargetDigest: cycleObservationDigest("target"), TargetID: 1}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong kind err=%v", err)
	}
	if err := bridge.RecordCycleObservation(context.Background(), cycleport.CycleObservationReceipt{Kind: cycleObservationMetricKind, SourceIdentifier: "not-hex", PayloadDigest: cycleObservationDigest("payload"), TargetDigest: cycleObservationDigest("target"), TargetID: 1}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("malformed source err=%v", err)
	}
}

type cycleObservationTxStub struct{}

func (cycleObservationTxStub) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("unused")
}
func (cycleObservationTxStub) Commit(context.Context) error   { return errors.New("unused") }
func (cycleObservationTxStub) Rollback(context.Context) error { return errors.New("unused") }
func (cycleObservationTxStub) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unused")
}
func (cycleObservationTxStub) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (cycleObservationTxStub) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (cycleObservationTxStub) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("unused")
}
func (cycleObservationTxStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unused")
}
func (cycleObservationTxStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unused")
}
func (cycleObservationTxStub) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (cycleObservationTxStub) Conn() *pgx.Conn                                  { return nil }

func cycleObservationMetricFixture(at time.Time) cyclehistory.MetricFact {
	numerator, value := -1.5, 0.0
	return cyclehistory.MetricFact{Source: cycleObservationEnvelope(1), SourceID: -1, RunID: 0, MetricKey: "metric", Label: "label", Numerator: &numerator, Value: &value, Unit: "count", ObservationWindow: "week", DataSource: "legacy", DataQuality: "partial", LimitationsJSON: json.RawMessage("null"), IsCausal: false, ValueStatus: "unknown", LastSnapshotID: -2, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}

func cycleObservationReferenceFixture(at time.Time) cyclehistory.ReferenceFact {
	return cyclehistory.ReferenceFact{Source: cycleObservationEnvelope(20), SourceID: 0, RunID: -3, ReferenceKey: "reference", ReferenceType: "source", Label: "label", SourceSystem: "legacy", ReferenceSourceID: "", Href: "https://private.example/path?token=kept-private", EvidenceHash: "evidence", DataStatus: "partial", LastSnapshotID: -4, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}

func cycleObservationEnvelope(seed byte) cyclehistory.SourceEnvelope {
	return cyclehistory.SourceEnvelope{SourceKeyDigest: cycleObservationDigest(string([]byte{seed, 1})), PayloadDigest: cycleObservationDigest(string([]byte{seed, 2})), FieldDigest: cycleObservationDigest(string([]byte{seed, 3}))}
}

func cycleObservationDigest(value string) [sha256.Size]byte { return sha256.Sum256([]byte(value)) }
