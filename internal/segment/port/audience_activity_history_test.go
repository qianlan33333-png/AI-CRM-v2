package port

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAudienceActivityViewsExcludePrivateHistoryFields(t *testing.T) {
	stamp := time.Date(2026, 8, 29, 8, 30, 0, 0, time.UTC)
	view := AudienceActivityMemberEventView{ID: 1, PackageHistoryID: 2, EventType: "entered", OccurredAt: stamp, CreatedAt: stamp}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"source_id", "private_digest", "identity", "union", "mobile", "owner", "payload", "idempotency"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe view leaked %q: %s", forbidden, encoded)
		}
	}
}
