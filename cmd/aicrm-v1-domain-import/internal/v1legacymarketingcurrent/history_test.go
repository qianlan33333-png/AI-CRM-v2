package v1legacymarketingcurrent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptHistoryPreservesLegacyMarketingSourceFacts(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.FixedZone("source", 8*60*60))
	stateRow := legacyMarketingRow(t, MarketingStateCurrentTableID, 1, legacyMarketingStateValue(stamp))
	valueRow := legacyMarketingRow(t, MarketingValueSegmentCurrentTableID, 1, legacyMarketingValueSegmentValue(stamp))
	history := AdaptHistory([]v1archive.ArchivedRow{stateRow}, []v1archive.ArchivedRow{valueRow})
	if history.SourceCount() != 2 || history.TerminalCount() != 2 {
		t.Fatalf("source/terminal count=%d/%d", history.SourceCount(), history.TerminalCount())
	}
	state := mustCandidate(t, history.MarketingStateCurrent[0])
	if state.SourceID != -7 || state.Source.SourceKeyHMAC != stateRow.SourceKeyHMAC || state.Source.PayloadHMAC != stateRow.PayloadHMAC || state.Source.FieldHMAC != stateRow.FieldHMAC ||
		state.ScenarioKey != "legacy-scenario" || state.MarketingPhase != "phase-a" || state.LastBatchSourceID != nil || state.EnteredAt != nil || state.ExitedAt != nil ||
		!state.CreatedAt.Equal(stamp) || !state.UpdatedAt.Equal(stamp) || !bytes.Equal(state.SourcePayload, []byte(`{"legacy":true}`)) {
		t.Fatal("marketing state source fact changed")
	}
	value := mustCandidate(t, history.MarketingValueSegmentCurrent[0])
	if value.SourceID != -9 || value.Score != -12 || value.ScenarioKey != "legacy-scenario" || !value.CreatedAt.Equal(stamp) || !value.UpdatedAt.Equal(stamp) ||
		!bytes.Equal(value.ScoreBreakdown, []byte(`{"score":-12}`)) || !bytes.Equal(value.SourcePayload, []byte(`{"legacy":true}`)) {
		t.Fatal("marketing value-segment source fact changed")
	}
}

func TestAdaptHistoryQuarantinesRedactionShapeAndDuplicateSourceIDs(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 10, 11, 12, 0, time.UTC)
	state := legacyMarketingRow(t, MarketingStateCurrentTableID, 1, legacyMarketingStateValue(stamp))
	state.RedactedFields = []string{"external_userid"}
	if result := AdaptHistory([]v1archive.ArchivedRow{state}, nil).MarketingStateCurrent[0]; result.Disposition != DispositionQuarantine || result.Reason != "marketing_state_current_archive_invalid" || result.Fact != nil {
		t.Fatal("redacted external identity was accepted")
	}

	broken := legacyMarketingValueSegmentValue(stamp)
	broken["score"] = "not-an-integer"
	if result := AdaptHistory(nil, []v1archive.ArchivedRow{legacyMarketingRow(t, MarketingValueSegmentCurrentTableID, 1, broken)}).MarketingValueSegmentCurrent[0]; result.Disposition != DispositionQuarantine || result.Reason != "marketing_value_segment_current_shape_invalid" || result.Fact != nil {
		t.Fatal("invalid source score was accepted")
	}

	first := legacyMarketingRow(t, MarketingStateCurrentTableID, 1, legacyMarketingStateValue(stamp))
	second := legacyMarketingRow(t, MarketingStateCurrentTableID, 2, legacyMarketingStateValue(stamp))
	for _, result := range AdaptHistory([]v1archive.ArchivedRow{first, second}, nil).MarketingStateCurrent {
		if result.Disposition != DispositionQuarantine || result.Reason != "marketing_state_current_source_id_ambiguous" || result.Fact != nil {
			t.Fatal("duplicate source ID was accepted")
		}
	}
}

func TestAdaptHistoryRejectsMissingNullablePresence(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 10, 11, 12, 0, time.UTC)
	value := legacyMarketingStateValue(stamp)
	delete(value, "exited_at")
	result := AdaptHistory([]v1archive.ArchivedRow{legacyMarketingRow(t, MarketingStateCurrentTableID, 1, value)}, nil).MarketingStateCurrent[0]
	if result.Disposition != DispositionQuarantine || result.Reason != "marketing_state_current_shape_invalid" || result.Fact != nil {
		t.Fatal("missing nullable source field was accepted")
	}
}

func legacyMarketingStateValue(stamp time.Time) map[string]any {
	return map[string]any{
		"id": -7, "scenario_key": "legacy-scenario", "external_userid": "private-external", "marketing_phase": "phase-a", "phase_label": "label", "phase_reason": "reason",
		"lifecycle_status": "active", "last_batch_id": nil, "last_batch_status": "", "last_batch_window_start": "civil-start", "last_batch_window_end": "civil-end",
		"last_trigger_message_at": "civil-trigger", "entered_at": nil, "exited_at": nil, "exit_reason": "", "source_payload_json": json.RawMessage(`{"legacy":true}`),
		"created_at": stamp, "updated_at": stamp,
	}
}

func legacyMarketingValueSegmentValue(stamp time.Time) map[string]any {
	return map[string]any{
		"id": -9, "scenario_key": "legacy-scenario", "external_userid": "private-external", "value_segment": "value-a", "segment_label": "label",
		"score": -12, "score_breakdown_json": json.RawMessage(`{"score":-12}`), "source_payload_json": json.RawMessage(`{"legacy":true}`),
		"created_at": stamp, "updated_at": stamp,
	}
}

func legacyMarketingRow(t *testing.T, tableID string, ordinal int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	seed := []byte(tableID + "/" + string(rune(ordinal)))
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: tableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256(append([]byte("source/"), seed...)), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256(append([]byte("field/"), seed...)), Payload: payload,
	}
}

func mustCandidate[T any](t *testing.T, result Result[T]) *T {
	t.Helper()
	if result.Disposition != DispositionCandidate || result.Fact == nil || result.Reason != "" {
		t.Fatal("source row was not a candidate")
	}
	return result.Fact
}
