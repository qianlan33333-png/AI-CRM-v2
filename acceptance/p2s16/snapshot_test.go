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
	if !bytes.Equal(first, second) {
		t.Fatal("snapshot generator changed across consecutive runs")
	}
	var document snapshotDocument
	if err = json.Unmarshal(first, &document); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if document.Version != 1 || len(document.Cases) != 3 {
		t.Fatalf("snapshot version/cases = %d/%d, want 1/3", document.Version, len(document.Cases))
	}
	for _, item := range document.Cases {
		if item.ActualResponse.Status < 200 || item.ActualResponse.Status >= 300 {
			t.Fatalf("%s status = %d", item.OperationID, item.ActualResponse.Status)
		}
	}
}
