package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	cyclehistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1cycleobservationhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	cycleapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	cycleport "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	cyclestore "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store"
)

const CycleObservationHistoryVersion = "v1-cycle-observation-history-a1"

const (
	cycleObservationMetricKind    = "cycle_metric"
	cycleObservationReferenceKind = "cycle_reference"

	cycleObservationDomain          = "operationcycle"
	cycleObservationMetricTarget    = "operation_cycle_v1_metric_history"
	cycleObservationReferenceTarget = "operation_cycle_v1_reference_history"
)

// CycleObservationHistoryResult records only immutable historical writes. It
// never identifies, starts, or modifies an operation-cycle run.
type CycleObservationHistoryResult struct {
	Selected, Imported, Replayed int
	Reconciliation               *ReconciliationResult `json:",omitempty"`
}

// cycleObservationHistoryJournal binds one typed owner receipt kind to one
// generic journal scope in the same caller transaction.
type cycleObservationHistoryJournal struct {
	journal *Journal
	kind    string
}

var _ cycleport.CycleObservationJournal = cycleObservationHistoryJournal{}

func (bridge cycleObservationHistoryJournal) LoadCycleObservation(ctx context.Context, kind, source string) (cycleport.CycleObservationReceipt, bool, error) {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != CycleObservationHistoryVersion || kind != bridge.kind {
		return cycleport.CycleObservationReceipt{}, false, ErrInvalidScope
	}
	value, found, err := bridge.journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return cycleport.CycleObservationReceipt{}, found, err
	}
	key, keyErr := ParseSourceIdentifier(source)
	id, idErr := positiveID(value.TargetID)
	if keyErr != nil || idErr != nil || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) || value.SourceKeyDigest != key || value.PayloadDigest == ([sha256.Size]byte{}) || value.TargetDigest == ([sha256.Size]byte{}) || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 || strconv.FormatInt(id, 10) != value.TargetID {
		return cycleport.CycleObservationReceipt{}, false, ErrConflict
	}
	return cycleport.CycleObservationReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: value.PayloadDigest, TargetDigest: value.TargetDigest, TargetID: id}, true, nil
}

func (bridge cycleObservationHistoryJournal) RecordCycleObservation(ctx context.Context, value cycleport.CycleObservationReceipt) error {
	if bridge.journal == nil || bridge.journal.scope.ImportVersion != CycleObservationHistoryVersion || value.Kind != bridge.kind || value.Replayed || value.TargetID < 1 || value.PayloadDigest == ([sha256.Size]byte{}) || value.TargetDigest == ([sha256.Size]byte{}) {
		return ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(value.SourceIdentifier)
	if err != nil || key == ([sha256.Size]byte{}) || value.SourceIdentifier != SourceIdentifier(key) {
		return ErrInvalidScope
	}
	return bridge.journal.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: value.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(value.TargetID, 10), TargetDigest: value.TargetDigest})
}

type cycleObservationEntry struct {
	scope               Scope
	kind, source        string
	key, payload, field [sha256.Size]byte
	journal             *Journal
	write               func(context.Context) (cycleport.CycleObservationReceipt, error)
	verify              func(context.Context, int64) ([sha256.Size]byte, error)
}

// cycleObservationEntries converts the complete already-authenticated source
// selection into sorted, caller-Tx-bound entries. Source run and snapshot IDs
// remain historical source values; this package creates no V2 current link.
func cycleObservationEntries(ctx context.Context, selected cyclehistory.Selection, run string, tx pgx.Tx) ([]cycleObservationEntry, error) {
	if ctx == nil || run == "" || tx == nil {
		return nil, ErrInvalidScope
	}
	entries := make([]cycleObservationEntry, 0, selected.Total())
	for _, selectedMetric := range selected.Metrics {
		entry, err := cycleObservationMetricEntry(selectedMetric, run, tx)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	for _, selectedReference := range selected.References {
		entry, err := cycleObservationReferenceEntry(selectedReference, run, tx)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].scope.TableID+"/"+entries[i].source < entries[j].scope.TableID+"/"+entries[j].source
	})
	for index, entry := range entries {
		if entry.source != SourceIdentifier(entry.key) || entry.key == ([sha256.Size]byte{}) || entry.payload == ([sha256.Size]byte{}) || entry.field == ([sha256.Size]byte{}) || (index > 0 && entries[index-1].scope.TableID == entry.scope.TableID && entries[index-1].source == entry.source) {
			return nil, ErrConflict
		}
	}
	return entries, nil
}

func cycleObservationMetricEntry(selected cyclehistory.MetricCandidate, run string, tx pgx.Tx) (cycleObservationEntry, error) {
	if selected.SourceOrdinal < 1 {
		return cycleObservationEntry{}, ErrConflict
	}
	fact, err := cycleObservationMetricFact(selected.Fact)
	if err != nil {
		return cycleObservationEntry{}, err
	}
	scope := Scope{ImportVersion: CycleObservationHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: cyclehistory.MetricsTableID, TargetDomain: cycleObservationDomain, TargetTable: cycleObservationMetricTarget}
	journal, err := NewJournal(scope)
	if err != nil {
		return cycleObservationEntry{}, err
	}
	writer, err := cycleapp.NewCycleObservationWriter(cyclestore.NewCycleObservationStore(), cycleObservationHistoryJournal{journal: journal, kind: cycleObservationMetricKind})
	if err != nil {
		return cycleObservationEntry{}, err
	}
	reader := cyclestore.NewCycleObservationReader(tx)
	probe := fact
	probe.ID = 1
	if _, err = cycleapp.HistoricalCycleMetricDigest(probe); err != nil {
		return cycleObservationEntry{}, err
	}
	source := SourceIdentifier(fact.SourceKeyDigest)
	return cycleObservationEntry{
		scope: scope, kind: cycleObservationMetricKind, source: source, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
		write: func(ctx context.Context) (cycleport.CycleObservationReceipt, error) {
			return writer.ImportHistoricalCycleMetric(ctx, source, fact)
		},
		verify: func(ctx context.Context, id int64) ([sha256.Size]byte, error) {
			actual, err := reader.GetHistoricalCycleMetric(ctx, id)
			if err != nil || actual.ID != id {
				return [sha256.Size]byte{}, ErrConflict
			}
			expected := fact
			expected.ID = id
			want, wantErr := cycleapp.HistoricalCycleMetricDigest(expected)
			got, gotErr := cycleapp.HistoricalCycleMetricDigest(actual)
			if wantErr != nil || gotErr != nil || want != got {
				return [sha256.Size]byte{}, ErrConflict
			}
			return got, nil
		},
	}, nil
}

func cycleObservationReferenceEntry(selected cyclehistory.ReferenceCandidate, run string, tx pgx.Tx) (cycleObservationEntry, error) {
	if selected.SourceOrdinal < 1 {
		return cycleObservationEntry{}, ErrConflict
	}
	fact, err := cycleObservationReferenceFact(selected.Fact)
	if err != nil {
		return cycleObservationEntry{}, err
	}
	scope := Scope{ImportVersion: CycleObservationHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: cyclehistory.ReferencesTableID, TargetDomain: cycleObservationDomain, TargetTable: cycleObservationReferenceTarget}
	journal, err := NewJournal(scope)
	if err != nil {
		return cycleObservationEntry{}, err
	}
	writer, err := cycleapp.NewCycleObservationWriter(cyclestore.NewCycleObservationStore(), cycleObservationHistoryJournal{journal: journal, kind: cycleObservationReferenceKind})
	if err != nil {
		return cycleObservationEntry{}, err
	}
	reader := cyclestore.NewCycleObservationReader(tx)
	probe := fact
	probe.ID = 1
	if _, err = cycleapp.HistoricalCycleReferenceDigest(probe); err != nil {
		return cycleObservationEntry{}, err
	}
	source := SourceIdentifier(fact.SourceKeyDigest)
	return cycleObservationEntry{
		scope: scope, kind: cycleObservationReferenceKind, source: source, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
		write: func(ctx context.Context) (cycleport.CycleObservationReceipt, error) {
			return writer.ImportHistoricalCycleReference(ctx, source, fact)
		},
		verify: func(ctx context.Context, id int64) ([sha256.Size]byte, error) {
			actual, err := reader.GetHistoricalCycleReference(ctx, id)
			if err != nil || actual.ID != id {
				return [sha256.Size]byte{}, ErrConflict
			}
			expected := fact
			expected.ID = id
			want, wantErr := cycleapp.HistoricalCycleReferenceDigest(expected)
			got, gotErr := cycleapp.HistoricalCycleReferenceDigest(actual)
			if wantErr != nil || gotErr != nil || want != got {
				return [sha256.Size]byte{}, ErrConflict
			}
			return got, nil
		},
	}, nil
}

func cycleObservationMetricFact(value cyclehistory.MetricFact) (cycleport.HistoricalCycleMetric, error) {
	if !validCycleObservationSource(value.Source) || !json.Valid(value.LimitationsJSON) {
		return cycleport.HistoricalCycleMetric{}, ErrConflict
	}
	return cycleport.HistoricalCycleMetric{
		SourceID: value.SourceID, SourceKeyDigest: value.Source.SourceKeyDigest, SourcePayloadDigest: value.Source.PayloadDigest, SourceFieldDigest: value.Source.FieldDigest,
		RunSourceID: value.RunID, MetricKey: value.MetricKey, Label: value.Label, Numerator: cloneCycleObservationFloat(value.Numerator), Denominator: cloneCycleObservationFloat(value.Denominator), Value: cloneCycleObservationFloat(value.Value),
		Unit: value.Unit, ObservationWindow: value.ObservationWindow, DataSource: value.DataSource, DataQuality: value.DataQuality, LimitationsJSON: append(json.RawMessage(nil), value.LimitationsJSON...),
		IsCausal: value.IsCausal, ValueStatus: value.ValueStatus, LastSnapshotSourceID: value.LastSnapshotID, CreatedAt: value.CreatedAt.UTC().Truncate(time.Microsecond), UpdatedAt: value.UpdatedAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func cycleObservationReferenceFact(value cyclehistory.ReferenceFact) (cycleport.HistoricalCycleReference, error) {
	if !validCycleObservationSource(value.Source) {
		return cycleport.HistoricalCycleReference{}, ErrConflict
	}
	return cycleport.HistoricalCycleReference{
		SourceID: value.SourceID, SourceKeyDigest: value.Source.SourceKeyDigest, SourcePayloadDigest: value.Source.PayloadDigest, SourceFieldDigest: value.Source.FieldDigest,
		RunSourceID: value.RunID, ReferenceKey: value.ReferenceKey, ReferenceType: value.ReferenceType, Label: value.Label, SourceSystem: value.SourceSystem, ReferenceSourceID: value.ReferenceSourceID,
		Href: value.Href, EvidenceHash: value.EvidenceHash, DataStatus: value.DataStatus, LastSnapshotSourceID: value.LastSnapshotID, CreatedAt: value.CreatedAt.UTC().Truncate(time.Microsecond), UpdatedAt: value.UpdatedAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func validCycleObservationSource(value cyclehistory.SourceEnvelope) bool {
	return value.SourceKeyDigest != ([sha256.Size]byte{}) && value.PayloadDigest != ([sha256.Size]byte{}) && value.FieldDigest != ([sha256.Size]byte{})
}

func cloneCycleObservationFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
