package v1cycleobservationhistory

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptMetricPreservesNullableSignedAndSourceEnvelope(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 9, 2, 3, 123456789, time.FixedZone("source", 8*3600))
	row := cycleObservationRow(t, key, MetricsTableID, -7, map[string]any{
		"id": int64(-7), "run_id": int64(0), "metric_key": "conversion", "label": "Conversion", "numerator": -1.25, "denominator": nil, "value": math.Copysign(0, -1),
		"unit": "count", "observation_window": "weekly", "data_source": "legacy", "data_quality": "partial", "limitations_json": map[string]any{"note": "original"}, "is_causal": false,
		"value_status": "unknown", "last_snapshot_id": int64(-9), "created_at": at, "updated_at": at.Add(time.Second),
	})
	fact, err := AdaptMetric(row, key)
	if err != nil {
		t.Fatal(err)
	}
	if fact.SourceID != -7 || fact.RunID != 0 || fact.LastSnapshotID != -9 || fact.Numerator == nil || *fact.Numerator != -1.25 || fact.Denominator != nil || fact.Value == nil || !math.Signbit(*fact.Value) || string(fact.LimitationsJSON) != `{"note":"original"}` {
		t.Fatalf("fact=%+v", fact)
	}
	if fact.Source.SourceKeyDigest != row.SourceKeyHMAC || fact.Source.PayloadDigest != row.PayloadHMAC || fact.Source.FieldDigest != row.FieldHMAC || fact.CreatedAt.Location() != time.UTC || fact.CreatedAt.Nanosecond() != 123456000 {
		t.Fatalf("source/time lost: %#v", fact)
	}
}

func TestAdaptReferencePreservesPrivateHrefWithoutExposure(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 654321987, time.UTC)
	row := cycleObservationRow(t, key, ReferencesTableID, 0, map[string]any{
		"id": int64(0), "run_id": int64(-4), "reference_key": "r", "reference_type": "other", "label": "", "source_system": "legacy", "source_id": "-7", "href": "https://private.example/path?token=secret",
		"evidence_hash": "hash", "data_status": "unknown", "last_snapshot_id": int64(0), "created_at": at, "updated_at": at,
	})
	fact, err := AdaptReference(row, key)
	if err != nil {
		t.Fatal(err)
	}
	if fact.SourceID != 0 || fact.RunID != -4 || fact.ReferenceSourceID != "-7" || fact.Href != "https://private.example/path?token=secret" || fact.LastSnapshotID != 0 || fact.UpdatedAt.Nanosecond() != 654321000 {
		t.Fatalf("fact=%+v", fact)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || strings.Contains(string(encoded), "private.example") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("href leaked: %s err=%v", encoded, err)
	}
}

func TestAdaptCycleObservationRejectsArchiveAndShapeDrift(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	metric := cycleObservationRow(t, key, MetricsTableID, 1, map[string]any{
		"id": int64(1), "run_id": int64(1), "metric_key": "", "label": "", "numerator": nil, "denominator": nil, "value": nil, "unit": "", "observation_window": "", "data_source": "", "data_quality": "", "limitations_json": []any{}, "is_causal": false, "value_status": "", "last_snapshot_id": int64(1), "created_at": at, "updated_at": at,
	})
	for name, row := range map[string]v1archive.ArchivedRow{
		"source HMAC":  mutateCycleRow(metric, func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC[0]++ }),
		"payload HMAC": mutateCycleRow(metric, func(value *v1archive.ArchivedRow) { value.PayloadHMAC[0]++ }),
		"field HMAC":   mutateCycleRow(metric, func(value *v1archive.ArchivedRow) { value.FieldHMAC[0]++ }),
		"ordinal":      mutateCycleRow(metric, func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 0 }),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := AdaptMetric(row, key); err == nil {
				t.Fatal("accepted drift")
			}
		})
	}
	redacted := metric
	redacted.RedactedFields = []string{"label"}
	field, err := v1archive.FieldHMAC(key, "operation_cycle_metrics", redacted.RedactedFields)
	if err != nil {
		t.Fatal(err)
	}
	redacted.FieldHMAC = field
	if _, err = AdaptMetric(redacted, key); err != ErrArchiveRow {
		t.Fatalf("redaction error=%v", err)
	}
	extra := cycleObservationRow(t, key, MetricsTableID, 2, map[string]any{
		"id": int64(2), "run_id": int64(1), "metric_key": "", "label": "", "numerator": nil, "denominator": nil, "value": nil, "unit": "", "observation_window": "", "data_source": "", "data_quality": "", "limitations_json": []any{}, "is_causal": false, "value_status": "", "last_snapshot_id": int64(1), "created_at": at, "updated_at": at, "unexpected": true,
	})
	if _, err = AdaptMetric(extra, key); err != ErrFact {
		t.Fatalf("extra field error=%v", err)
	}
}

func cycleObservationRow(t *testing.T, key []byte, tableID string, id int64, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payload, roots, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	table := strings.TrimPrefix(tableID, "public/")
	source, err := v1archive.SourceKeyHMAC(key, table, []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, table, payload)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(key, table, roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: tableID, SourceOrdinal: 1, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: field, Payload: payload, RedactedFields: roots}
}

func mutateCycleRow(row v1archive.ArchivedRow, mutate func(*v1archive.ArchivedRow)) v1archive.ArchivedRow {
	mutate(&row)
	return row
}
