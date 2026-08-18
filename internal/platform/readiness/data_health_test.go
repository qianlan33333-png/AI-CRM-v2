package readiness

import (
	"reflect"
	"testing"
	"time"
)

func TestEvaluateDataHealthFrozenRegistry(t *testing.T) {
	at := time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC)
	aggregate := EvaluateDataHealth(Input{Production: false, Database: DatabaseObservation{Kind: DatabasePostgres, Probe: ProbeHealthy}, Migration: MigrationObservation{Compatibility: MigrationCompatible}, Release: ReleaseObservation{SHAComplete: false}, Queues: QueueObservation{Probe: ProbeHealthy}}, at)
	if got := checkIDs(aggregate.Checks); !reflect.DeepEqual(got, dataHealthCheckIDs[:]) {
		t.Fatalf("check IDs = %v", got)
	}
	if aggregate.RegistryID != DataHealthRegistryID || aggregate.RegistrySHA256 != "9c736c840e20b599825227f519c7542c0aa174d7bc5c3172d0e227f3e3308823" || !aggregate.RegistryMatchesManifest {
		t.Fatalf("registry = %#v", aggregate)
	}
	if aggregate.OverallStatus != "warn" || !aggregate.OK || aggregate.Counts != (DataHealthCounts{OK: 3, Warn: 1}) || aggregate.GateCounts != (DataHealthGateCounts{Pass: 3, Warn: 1}) {
		t.Fatalf("aggregate = %#v", aggregate)
	}
	for _, check := range aggregate.Checks {
		if check.FirstObservedAt != aggregate.ObservedAt || check.LastObservedAt != aggregate.ObservedAt || check.Owner != "platform_readiness" || check.CandidateRelated || check.ReplayPolicy != "manual_after_remediation" {
			t.Fatalf("check=%#v", check)
		}
	}
}

func TestEvaluateDataHealthFailureCasesFailClosed(t *testing.T) {
	at := time.Now().UTC()
	tests := []struct {
		name                       string
		input                      Input
		id, status, severity, gate string
		evidence                   map[string]any
	}{
		{"database unknown", Input{}, "database_readiness", "fail", "red", "block", map[string]any{"database_readable": false}},
		{"migration unknown", Input{}, "migration_compatibility", "fail", "red", "block", map[string]any{"schema_compatible": false}},
		{"migration ahead", Input{Migration: MigrationObservation{Compatibility: MigrationCompatibleAhead}}, "migration_compatibility", "fail", "red", "block", map[string]any{"schema_compatible": false}},
		{"production sha", Input{Production: true, Release: ReleaseObservation{}}, "release_sha_complete", "fail", "red", "block", map[string]any{"sha_complete": false, "environment": "production"}},
		{"queue unavailable", Input{}, "outbound_outcome_unknown_backlog", "fail", "red", "block", map[string]any{"queue_observation_available": false}},
		{"queue budget exhausted", Input{Queues: QueueObservation{Probe: ProbeHealthy, BudgetExhausted: true}}, "outbound_outcome_unknown_backlog", "fail", "red", "block", map[string]any{"queue_observation_available": false}},
		{"queue positive", Input{Queues: QueueObservation{Probe: ProbeHealthy, UnknownAfterDispatch: 1}}, "outbound_outcome_unknown_backlog", "warn", "yellow", "warn", map[string]any{"queue_observation_available": true, "outcome_unknown_count": uint64(1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregate := EvaluateDataHealth(test.input, at)
			var got DataHealthCheck
			for _, check := range aggregate.Checks {
				if check.CheckID == test.id {
					got = check
				}
			}
			if got.Status != test.status || got.Severity != test.severity || got.GateDecision != test.gate || !reflect.DeepEqual(got.Evidence, test.evidence) {
				t.Fatalf("check = %#v", got)
			}
		})
	}
}

func TestEvaluateDataHealthExclusionsAreFrozenAndSorted(t *testing.T) {
	aggregate := EvaluateDataHealth(Input{}, time.Now())
	want := []string{"ai_automation_lane_readiness", "broadcast_job_blocked_backlog", "customer_360_freshness_guard", "deprecated_execution_settings_present", "external_effect_approved_not_queued", "external_effect_due_retryable_backlog", "external_effect_unclassified_blocked_recent", "external_effect_unclassified_terminal_recent", "fake_stub_route_exposed", "identity_legacy_column_guard", "identity_resolution_queue_backlog", "payment_order_without_user_guard", "projection_freshness_customer_read_model", "questionnaire_submission_without_user_guard", "retired_table_runtime_reference_guard", "schema_drift_guard", "table_lifecycle_manifest_guard", "unionid_orphan_fact_guard", "wecom_media_lease_health"}
	if len(aggregate.ExcludedLegacyCheckIDs) != 19 || !reflect.DeepEqual(aggregate.ExcludedLegacyCheckIDs, want) {
		t.Fatalf("exclusions=%d", len(aggregate.ExcludedLegacyCheckIDs))
	}
	for i := 1; i < len(aggregate.ExcludedLegacyCheckIDs); i++ {
		if aggregate.ExcludedLegacyCheckIDs[i-1] >= aggregate.ExcludedLegacyCheckIDs[i] {
			t.Fatalf("unsorted exclusions=%v", aggregate.ExcludedLegacyCheckIDs)
		}
	}
}

func TestEvaluateDataHealthDatabaseOnlyAcceptsPostgres(t *testing.T) {
	for _, kind := range []DatabaseKind{DatabaseFixture, DatabaseMissing, DatabaseKind("unknown")} {
		aggregate := EvaluateDataHealth(Input{Database: DatabaseObservation{Kind: kind, Probe: ProbeHealthy}}, time.Now())
		if aggregate.Checks[0].Status != "fail" || aggregate.Checks[0].Evidence["database_readable"] != false {
			t.Fatalf("kind %q = %#v", kind, aggregate.Checks[0])
		}
	}
}

func TestEvaluateDataHealthManifestMismatchBlocksSummary(t *testing.T) {
	aggregate := evaluateDataHealth(Input{Database: DatabaseObservation{Kind: DatabasePostgres, Probe: ProbeHealthy}, Migration: MigrationObservation{Compatibility: MigrationCompatible}, Release: ReleaseObservation{SHAComplete: true}, Queues: QueueObservation{Probe: ProbeHealthy}}, time.Now(), []string{"database_readiness"})
	if aggregate.RegistryMatchesManifest || aggregate.OK || aggregate.OverallStatus != "fail" {
		t.Fatalf("aggregate = %#v", aggregate)
	}
}

func checkIDs(checks []DataHealthCheck) []string {
	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.CheckID)
	}
	return ids
}
