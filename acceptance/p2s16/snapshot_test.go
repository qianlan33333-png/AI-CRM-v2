package p2s16

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestStageSnapshotIsDeterministicAndComplete(t *testing.T) {
	first, err := GenerateStageSnapshot()
	if err != nil {
		t.Fatalf("GenerateStageSnapshot() error = %v", err)
	}
	second, err := GenerateStageSnapshot()
	if err != nil {
		t.Fatalf("GenerateStageSnapshot(second) error = %v", err)
	}
	if !bytes.Equal(normalizeIgnoredSnapshotFields(t, first), normalizeIgnoredSnapshotFields(t, second)) {
		t.Fatal("snapshot generator changed across consecutive runs")
	}
	var document snapshotDocument
	if err = json.Unmarshal(first, &document); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if document.Version != 1 || len(document.Cases) != 6 {
		t.Fatalf("snapshot version/cases = %d/%d, want 1/6", document.Version, len(document.Cases))
	}
	for _, item := range document.Cases {
		if item.OperationID == "listTags" && item.CaseID == "unavailable" && item.ActualResponse.Status == 503 {
			continue
		}
		if item.ActualResponse.Status < 200 || item.ActualResponse.Status >= 300 {
			t.Fatalf("%s status = %d", item.OperationID, item.ActualResponse.Status)
		}
	}
}

func normalizeIgnoredSnapshotFields(t *testing.T, data []byte) []byte {
	t.Helper()
	var document snapshotDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode snapshot for normalization: %v", err)
	}
	for index := range document.Cases {
		item := &document.Cases[index]
		if item.OperationID != "listTags" || item.CaseID != "unavailable" {
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(item.ActualResponse.Body, &body); err != nil {
			t.Fatalf("decode ignored error response: %v", err)
		}
		body["request_id"] = "ignored-by-canonical-snapshot"
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode normalized error response: %v", err)
		}
		item.ActualResponse.Body = encoded
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode normalized snapshot: %v", err)
	}
	return encoded
}
