package v1cycleobservationhistory

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestSelectorPairsCompleteGenericArchiveTerminalsInSourceOrder(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	metricOne := cycleMetricRow(t, key, 1, 7, at)
	metricTwo := cycleMetricRow(t, key, -2, -9, at)
	metricTwo.SourceOrdinal = 2
	reference := cycleReferenceRow(t, key, 0, -4, at)
	archive := cycleSelectorArchive{rows: map[string][]v1archive.ArchivedRow{MetricsTableID: {metricOne, metricTwo}, ReferencesTableID: {reference}}}
	terminals := cycleSelectorTerminals{rows: map[string][]ArchiveTerminalReceipt{MetricsTableID: {cycleArchiveTerminal(metricOne), cycleArchiveTerminal(metricTwo)}, ReferencesTableID: {cycleArchiveTerminal(reference)}}}
	selector, err := NewSelector(archive, terminals)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := selector.Select(context.Background(), SelectionOptions{ArchiveRunID: "run", SourceHMACKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Total() != 3 || len(selection.Metrics) != 2 || selection.Metrics[0].SourceOrdinal != 1 || selection.Metrics[0].Fact.SourceID != 1 || selection.Metrics[1].SourceOrdinal != 2 || selection.Metrics[1].Fact.SourceID != -2 || len(selection.References) != 1 || selection.References[0].Fact.SourceID != 0 {
		t.Fatalf("selection=%#v", selection)
	}
}

func TestSelectorFailsClosedOnSourceOrGenericTerminalDrift(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	metric := cycleMetricRow(t, key, 1, 7, at)
	reference := cycleReferenceRow(t, key, 1, 8, at)
	baseRows := map[string][]v1archive.ArchivedRow{MetricsTableID: {metric}, ReferencesTableID: {reference}}
	baseTerminals := map[string][]ArchiveTerminalReceipt{MetricsTableID: {cycleArchiveTerminal(metric)}, ReferencesTableID: {cycleArchiveTerminal(reference)}}
	for _, test := range []struct {
		name            string
		mutateRows      func(map[string][]v1archive.ArchivedRow)
		mutateTerminals func(map[string][]ArchiveTerminalReceipt)
	}{
		{name: "source ordinal gap", mutateRows: func(rows map[string][]v1archive.ArchivedRow) { rows[MetricsTableID][0].SourceOrdinal = 2 }},
		{name: "duplicate source key", mutateRows: func(rows map[string][]v1archive.ArchivedRow) {
			duplicate := rows[MetricsTableID][0]
			duplicate.SourceOrdinal = 2
			rows[MetricsTableID] = append(rows[MetricsTableID], duplicate)
		}},
		{name: "source HMAC", mutateRows: func(rows map[string][]v1archive.ArchivedRow) { rows[MetricsTableID][0].SourceKeyHMAC[0]++ }},
		{name: "missing terminal", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID] = nil }},
		{name: "extra terminal", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) {
			extra := rows[MetricsTableID][0]
			extra.SourceKeyDigest[0]++
			rows[MetricsTableID] = append(rows[MetricsTableID], extra)
		}},
		{name: "payload mismatch", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID][0].PayloadDigest[0]++ }},
		{name: "field mismatch", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID][0].FieldDigest[0]++ }},
		{name: "wrong run", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID][0].ArchiveRunID = "other" }},
		{name: "wrong adapter", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID][0].AdapterID = "other" }},
		{name: "wrong table", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID][0].TableID = ReferencesTableID }},
		{name: "wrong disposition", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID][0].Disposition = "import" }},
		{name: "operation present", mutateTerminals: func(rows map[string][]ArchiveTerminalReceipt) { rows[MetricsTableID][0].Operation = "archive" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows, terminals := cloneCycleSelectorRows(baseRows), cloneCycleSelectorTerminals(baseTerminals)
			if test.mutateRows != nil {
				test.mutateRows(rows)
			}
			if test.mutateTerminals != nil {
				test.mutateTerminals(terminals)
			}
			selector, err := NewSelector(cycleSelectorArchive{rows: rows}, cycleSelectorTerminals{rows: terminals})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = selector.Select(context.Background(), SelectionOptions{ArchiveRunID: "run", SourceHMACKey: key}); !errors.Is(err, ErrSealedDrift) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestSelectorRejectsInvalidInputs(t *testing.T) {
	if _, err := NewSelector(nil, cycleSelectorTerminals{}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("nil archive error=%v", err)
	}
	selector, err := NewSelector(cycleSelectorArchive{}, cycleSelectorTerminals{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = selector.Select(context.Background(), SelectionOptions{ArchiveRunID: " run", SourceHMACKey: []byte(strings.Repeat("k", sha256.Size))}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("invalid options error=%v", err)
	}
}

type cycleSelectorArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (source cycleSelectorArchive) EachTableRow(_ context.Context, _ string, table string, emit func(v1archive.ArchivedRow) error) error {
	for _, row := range source.rows[table] {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type cycleSelectorTerminals struct {
	rows map[string][]ArchiveTerminalReceipt
}

func (source cycleSelectorTerminals) EachArchiveTerminal(_ context.Context, _ string, table string, emit func(ArchiveTerminalReceipt) error) error {
	for _, row := range source.rows[table] {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

func cycleMetricRow(t *testing.T, key []byte, id, runID int64, at time.Time) v1archive.ArchivedRow {
	t.Helper()
	return cycleObservationRow(t, key, MetricsTableID, id, map[string]any{
		"id": id, "run_id": runID, "metric_key": "", "label": "", "numerator": nil, "denominator": nil, "value": nil,
		"unit": "", "observation_window": "", "data_source": "", "data_quality": "", "limitations_json": nil, "is_causal": false,
		"value_status": "", "last_snapshot_id": int64(0), "created_at": at, "updated_at": at,
	})
}

func cycleReferenceRow(t *testing.T, key []byte, id, runID int64, at time.Time) v1archive.ArchivedRow {
	t.Helper()
	return cycleObservationRow(t, key, ReferencesTableID, id, map[string]any{
		"id": id, "run_id": runID, "reference_key": "", "reference_type": "", "label": "", "source_system": "", "source_id": "", "href": "", "evidence_hash": "", "data_status": "", "last_snapshot_id": int64(0), "created_at": at, "updated_at": at,
	})
}

func cycleArchiveTerminal(row v1archive.ArchivedRow) ArchiveTerminalReceipt {
	return ArchiveTerminalReceipt{ArchiveRunID: "run", AdapterID: v1archive.DefaultAdapterID, TableID: row.TableID, SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC, Disposition: "archive"}
}

func cloneCycleSelectorRows(input map[string][]v1archive.ArchivedRow) map[string][]v1archive.ArchivedRow {
	output := make(map[string][]v1archive.ArchivedRow, len(input))
	for table, rows := range input {
		output[table] = append([]v1archive.ArchivedRow(nil), rows...)
	}
	return output
}

func cloneCycleSelectorTerminals(input map[string][]ArchiveTerminalReceipt) map[string][]ArchiveTerminalReceipt {
	output := make(map[string][]ArchiveTerminalReceipt, len(input))
	for table, rows := range input {
		output[table] = append([]ArchiveTerminalReceipt(nil), rows...)
	}
	return output
}
