package port

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCustomerTimelineHistoryReadDoesNotExposePrivateSourceFields(t *testing.T) {
	value := CustomerTimelineHistoryRead{ID: 7, SourceID: -3, EventID: "event", EventType: "legacy", EventTime: time.Now().UTC(), SourceTable: "table", SourceValue: "source", CreatedAt: time.Now().UTC()}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"unionid", "title", "summary", "metadata_json", "source_key", "payload_digest", "field_digest"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe history DTO leaked %s: %s", forbidden, encoded)
		}
	}
}
