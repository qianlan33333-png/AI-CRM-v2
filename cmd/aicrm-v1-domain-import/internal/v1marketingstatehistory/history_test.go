package v1marketingstatehistory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptHistoryPreservesTypedInertFacts(t *testing.T) {
	current, history, valueCurrent, valueHistory := fixtures()
	adapted := AdaptHistory(
		[]v1archive.ArchivedRow{archiveRow(t, MarketingStateCurrentTableID, 1, current)},
		[]v1archive.ArchivedRow{archiveRow(t, MarketingStateHistoryTableID, 1, history)},
		[]v1archive.ArchivedRow{archiveRow(t, ValueSegmentCurrentTableID, 1, valueCurrent)},
		[]v1archive.ArchivedRow{archiveRow(t, ValueSegmentHistoryTableID, 1, valueHistory)},
	)
	if adapted.SourceCount() != 4 || adapted.TerminalCount() != 4 {
		t.Fatal("source row conservation failed")
	}

	marketing := mustCandidate(t, adapted.MarketingStateCurrent[0]).Fact
	if marketing.SourceID != 11 || marketing.CustomerID != nil || marketing.PersonSourceID == nil || *marketing.PersonSourceID != -9 || marketing.LastBatchSourceID == nil || *marketing.LastBatchSourceID != 0 || !marketing.Activated || marketing.Converted || !marketing.EligibleForConversion {
		t.Fatal("marketing-current source scalars or unresolved customer boundary changed")
	}
	if marketing.LastActivationAt != "source civil time" || marketing.LastConversionMarkedAt != "" || marketing.LastMessageAt != "not a date" || marketing.StatePayloadDigest == (OpaqueDigest{}) || marketing.ExternalUserIDDigest == (OpaqueDigest{}) || marketing.Source.SourceKeyDigest == (OpaqueDigest{}) {
		t.Fatal("marketing-current text, digest, or archive envelope was changed")
	}
	if marketing.EnteredAt == nil || marketing.ExitedAt != nil || marketing.CreatedAt.Format(time.RFC3339Nano) != "2026-08-28T09:30:00.123456+08:00" {
		t.Fatal("marketing-current nullable or timestamp fields changed")
	}

	transition := mustCandidate(t, adapted.MarketingStateHistory[0]).Fact
	if transition.CustomerID != nil || transition.PersonSourceID != nil || transition.BatchSourceID == nil || *transition.BatchSourceID != -3 || transition.ChangeReason != "legacy change" || transition.RecordedAt.Format(time.RFC3339Nano) != "2026-08-28T09:31:00.123456+08:00" {
		t.Fatal("marketing-history source references or timestamps changed")
	}

	currentSegment := mustCandidate(t, adapted.ValueSegmentCurrent[0]).Fact
	if currentSegment.CustomerID != nil || currentSegment.SegmentRank != -1 || currentSegment.Score != 0 || currentSegment.SubmissionSourceID == nil || *currentSegment.SubmissionSourceID != -4 || currentSegment.ComputedReason != "legacy reason" || currentSegment.MatchedQuestionIDsDigest == currentSegment.SourcePayloadDigest {
		t.Fatal("value-current source scalars, references, or domain-separated digests changed")
	}

	historySegment := mustCandidate(t, adapted.ValueSegmentHistory[0]).Fact
	if historySegment.CustomerID != nil || historySegment.SegmentRank != 0 || historySegment.Score != -5 || historySegment.SubmissionSourceID != nil || historySegment.ChangeReason != "history reason" || historySegment.EvaluatedAt.Format(time.RFC3339Nano) != "2026-08-28T09:32:00.123456+08:00" {
		t.Fatal("value-history source fields changed")
	}
}

func TestAdaptHistoryQuarantinesInvalidRequiredTypesAndEnvelope(t *testing.T) {
	current, history, valueCurrent, valueHistory := fixtures()
	for _, test := range []struct {
		name  string
		table string
		value map[string]any
		field string
		bad   any
	}{
		{"marketing bool", MarketingStateCurrentTableID, current, "activated", "true"},
		{"marketing source text", MarketingStateHistoryTableID, history, "last_activation_at", 7},
		{"value score", ValueSegmentCurrentTableID, valueCurrent, "score", "0"},
		{"value json", ValueSegmentHistoryTableID, valueHistory, "source_payload_json", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := clone(test.value)
			copy[test.field] = test.bad
			row := archiveRow(t, test.table, 1, copy)
			if resultState(adaptHistoryForTable(test.table, row)) != DispositionQuarantine {
				t.Fatal("invalid required type was accepted")
			}
		})
	}

	row := archiveRow(t, MarketingStateCurrentTableID, 1, current)
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = MarketingStateHistoryTableID },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.Payload = []byte(`{`) },
		func(value *v1archive.ArchivedRow) { value.RedactedFields = []string{"state_payload_json[0].secret"} },
	} {
		changed := row
		mutate(&changed)
		if resultState(AdaptHistory(marketingRows(changed), nil, nil, nil).MarketingStateCurrent[0]) != DispositionQuarantine {
			t.Fatal("invalid archive envelope or nested redaction was accepted")
		}
	}
	if !redactionPathMatches("state_payload_json", "state_payload_json[0].secret") || !redactionPathMatches("state_payload_json", "state_payload_json.nested") || redactionPathMatches("state_payload_json", "state_payload_jsonish[0]") {
		t.Fatal("nested redaction path matching changed")
	}
}

func TestAdaptHistoryQuarantinesDuplicateSourceIDsAndDoesNotLeakPrivateInputs(t *testing.T) {
	current, _, valueCurrent, _ := fixtures()
	row := archiveRow(t, MarketingStateCurrentTableID, 1, current)
	second := archiveRow(t, MarketingStateCurrentTableID, 2, current)
	history := AdaptHistory([]v1archive.ArchivedRow{row, second}, nil, []v1archive.ArchivedRow{archiveRow(t, ValueSegmentCurrentTableID, 1, valueCurrent)}, nil)
	for _, result := range history.MarketingStateCurrent {
		if result.Disposition != DispositionQuarantine || result.Reason != "customer_marketing_state_current_source_ambiguous" || result.Fact != nil {
			t.Fatal("duplicate source IDs were not quarantined")
		}
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-external-userid", "state-private", "matched-question-private", "source-payload-private"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private source input escaped candidate serialization")
		}
	}
}

func TestSignedSourceIDsPreserved(t *testing.T) {
	current, history, valueCurrent, valueHistory := fixtures()
	tables := []string{MarketingStateCurrentTableID, MarketingStateHistoryTableID, ValueSegmentCurrentTableID, ValueSegmentHistoryTableID}
	values := []map[string]any{current, history, valueCurrent, valueHistory}
	for i, table := range tables {
		for _, id := range []int64{-9223372036854775808, -1, 0, 9223372036854775807} {
			value := clone(values[i])
			value["id"] = id
			result := adaptHistoryForTable(table, archiveRow(t, table, 1, value))
			if resultState(result) != DispositionCandidate {
				t.Fatalf("%s rejected signed source id", table)
			}
			var got int64
			switch v := result.(type) {
			case Result[MarketingStateCurrentFact]:
				got = v.Fact.SourceID
			case Result[MarketingStateHistoryFact]:
				got = v.Fact.SourceID
			case Result[ValueSegmentCurrentFact]:
				got = v.Fact.SourceID
			case Result[ValueSegmentHistoryFact]:
				got = v.Fact.SourceID
			}
			if got != id {
				t.Fatalf("%s changed source id", table)
			}
		}
	}
}

func adaptHistoryForTable(table string, row v1archive.ArchivedRow) any {
	switch table {
	case MarketingStateCurrentTableID:
		return AdaptHistory([]v1archive.ArchivedRow{row}, nil, nil, nil).MarketingStateCurrent[0]
	case MarketingStateHistoryTableID:
		return AdaptHistory(nil, []v1archive.ArchivedRow{row}, nil, nil).MarketingStateHistory[0]
	case ValueSegmentCurrentTableID:
		return AdaptHistory(nil, nil, []v1archive.ArchivedRow{row}, nil).ValueSegmentCurrent[0]
	default:
		return AdaptHistory(nil, nil, nil, []v1archive.ArchivedRow{row}).ValueSegmentHistory[0]
	}
}

func resultState(value any) Disposition {
	switch value := value.(type) {
	case Result[MarketingStateCurrentFact]:
		return value.Disposition
	case Result[MarketingStateHistoryFact]:
		return value.Disposition
	case Result[ValueSegmentCurrentFact]:
		return value.Disposition
	case Result[ValueSegmentHistoryFact]:
		return value.Disposition
	default:
		return ""
	}
}

func marketingRows(row v1archive.ArchivedRow) []v1archive.ArchivedRow {
	return []v1archive.ArchivedRow{row}
}

func fixtures() (map[string]any, map[string]any, map[string]any, map[string]any) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("v1-source", 8*60*60))
	current := map[string]any{
		"id": int64(11), "person_id": int64(-9), "external_userid": "private-external-userid", "automation_key": "automation-source", "main_stage": "active", "sub_stage": "source", "activated": true, "converted": false, "eligible_for_conversion": true, "lifecycle_status": "legacy", "last_activation_at": "source civil time", "last_conversion_marked_at": "", "last_message_at": "not a date", "last_batch_id": int64(0), "last_batch_status": "queued", "last_batch_window_start": "", "last_batch_window_end": "source window", "last_trigger_message_at": "source trigger", "entered_at": stamp, "exited_at": nil, "exit_reason": "", "state_payload_json": map[string]any{"secret": "state-private"}, "created_at": stamp, "updated_at": stamp,
	}
	history := map[string]any{
		"id": int64(12), "person_id": nil, "external_userid": "private-external-userid", "automation_key": "automation-source", "main_stage": "active", "sub_stage": "source", "activated": false, "converted": true, "eligible_for_conversion": false, "batch_id": int64(-3), "lifecycle_status": "legacy", "exit_reason": "source", "last_activation_at": "", "last_conversion_marked_at": "source civil", "last_message_at": "", "change_reason": "legacy change", "state_payload_json": []any{"state-private"}, "recorded_at": stamp.Add(time.Minute), "created_at": stamp,
	}
	valueCurrent := map[string]any{
		"id": int64(13), "external_userid": "private-external-userid", "segment": "legacy-A", "segment_rank": int64(-1), "score": int64(0), "scoring_version": "v1", "computed_reason": "legacy reason", "submission_id": int64(-4), "matched_question_ids_json": []any{"matched-question-private"}, "source_payload_json": map[string]any{"secret": "source-payload-private"}, "evaluated_at": stamp.Add(2 * time.Minute), "computed_at": stamp, "created_at": stamp, "updated_at": stamp,
	}
	valueHistory := map[string]any{
		"id": int64(14), "external_userid": "private-external-userid", "segment": "legacy-B", "segment_rank": int64(0), "score": int64(-5), "scoring_version": "v1", "change_reason": "history reason", "submission_id": nil, "matched_question_ids_json": []any{}, "source_payload_json": map[string]any{}, "evaluated_at": stamp.Add(2 * time.Minute), "recorded_at": stamp, "created_at": stamp,
	}
	return current, history, valueCurrent, valueHistory
}

func archiveRow(t *testing.T, table string, ordinal int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/%d", table, ordinal))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte("fields/" + table)), Payload: payload}
}

func clone(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func mustCandidate[T any](t *testing.T, result Result[T]) Result[T] {
	t.Helper()
	if result.Disposition != DispositionCandidate || result.Reason != "" || result.Fact == nil {
		t.Fatal("expected historical candidate")
	}
	return result
}
