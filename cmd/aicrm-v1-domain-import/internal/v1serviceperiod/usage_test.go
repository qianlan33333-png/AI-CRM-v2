package v1serviceperiod

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestAdaptUsageSnapshotsPreservesNullableAndUnlinkedSourceFacts(t *testing.T) {
	first := usageSnapshotFixture()
	first.UnionID = ""
	first.LearningCurrent, first.LearningTotal, first.LastOpenAt = nil, nil, nil
	second := usageSnapshotFixture()
	second.HuangyoucanUserID, second.UnionID, second.MobileMD5 = "hxc-2", "unionid-2", "d41d8cd98f00b204e9800998ecf8427e"
	second.LastOpenAt = usageTimePtr(time.Date(2026, 8, 28, 9, 1, 2, 3000, time.UTC))
	result := AdaptUsageSnapshots([]UsageSnapshotSource{{Payload: usageRaw(t, first)}, {Payload: usageRaw(t, second)}})
	if len(result) != 2 || result[0].Disposition != UsageSnapshotCandidate || result[1].Disposition != UsageSnapshotCandidate ||
		!reflect.DeepEqual(result[0].Fact, &first) || !reflect.DeepEqual(result[1].Fact, &second) {
		t.Fatalf("usage result=%+v", result)
	}
	encoded := usageRaw(t, result[0].Fact)
	for _, forbidden := range []string{"product", "customer", "entitlement", "provider", "command"} {
		if containsJSONKey(encoded, forbidden) {
			t.Fatalf("candidate fabricated %s association: %s", forbidden, encoded)
		}
	}
}

func TestAdaptUsageSnapshotsRejectsRedactionAndInvalidRequiredFacts(t *testing.T) {
	for _, field := range usageSnapshotFields {
		result := AdaptUsageSnapshots([]UsageSnapshotSource{{Payload: usageRaw(t, usageSnapshotFixture()), RedactedFields: []string{field}}})
		if result[0].Disposition != UsageSnapshotInvalid || result[0].Reason != "usage_snapshot_redacted" {
			t.Fatalf("redacted %s=%+v", field, result[0])
		}
	}

	for name, mutate := range map[string]func(map[string]json.RawMessage){
		"missing":       func(row map[string]json.RawMessage) { delete(row, "learning_plan_id") },
		"required-null": func(row map[string]json.RawMessage) { row["mobile_md5"] = json.RawMessage("null") },
		"negative-count": func(row map[string]json.RawMessage) {
			row["open_count_7d"] = json.RawMessage("-1")
		},
		"bad-time": func(row map[string]json.RawMessage) { row["refreshed_at"] = json.RawMessage(`"not-a-time"`) },
		"bad-nullable": func(row map[string]json.RawMessage) {
			row["learning_plan_total"] = json.RawMessage(`"9"`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var row map[string]json.RawMessage
			if err := json.Unmarshal(usageRaw(t, usageSnapshotFixture()), &row); err != nil {
				t.Fatal(err)
			}
			mutate(row)
			result := AdaptUsageSnapshots([]UsageSnapshotSource{{Payload: usageRaw(t, row)}})
			if result[0].Disposition != UsageSnapshotInvalid {
				t.Fatalf("invalid %s=%+v", name, result[0])
			}
		})
	}
}

func TestAdaptUsageSnapshotsConservesOrderAndDoesNotReplaceNulls(t *testing.T) {
	valid := usageSnapshotFixture()
	invalid := usageSnapshotFixture()
	invalid.HuangyoucanUserID = ""
	result := AdaptUsageSnapshots([]UsageSnapshotSource{{Payload: usageRaw(t, valid)}, {Payload: usageRaw(t, invalid)}, {Payload: []byte(`{`)}})
	if len(result) != 3 || result[0].Fact == nil || result[1].Reason != "usage_snapshot_shape_invalid" || result[2].Reason != "usage_snapshot_json_invalid" {
		t.Fatalf("rows were dropped or reordered: %+v", result)
	}
}

func usageSnapshotFixture() UsageSnapshotFact {
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 123000, time.UTC)
	current, total := int32(2), int32(10)
	return UsageSnapshotFact{HuangyoucanUserID: "hxc-1", UnionID: "unionid-1", MobileMD5: "098f6bcd4621d373cade4e832627b4f6",
		FormallyLoggedIn: true, HasTokenUsage: true, LearningPlanID: "plan-a", LearningCurrent: &current, LearningTotal: &total,
		OpenCount7d: 3, LastOpenAt: usageTimePtr(stamp), RefreshedAt: stamp.Add(time.Hour)}
}

func usageTimePtr(value time.Time) *time.Time { return &value }

func usageRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func containsJSONKey(raw []byte, key string) bool {
	var fields map[string]json.RawMessage
	return json.Unmarshal(raw, &fields) == nil && fields[key] != nil
}
