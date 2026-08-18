package readiness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

const DataHealthRegistryID = "v2-core-readiness.v1"

var dataHealthCheckIDs = [...]string{
	"database_readiness",
	"migration_compatibility",
	"outbound_outcome_unknown_backlog",
	"release_sha_complete",
}

var dataHealthExcludedLegacyCheckIDs = [...]string{
	"ai_automation_lane_readiness",
	"broadcast_job_blocked_backlog",
	"customer_360_freshness_guard",
	"deprecated_execution_settings_present",
	"external_effect_approved_not_queued",
	"external_effect_due_retryable_backlog",
	"external_effect_unclassified_blocked_recent",
	"external_effect_unclassified_terminal_recent",
	"fake_stub_route_exposed",
	"identity_legacy_column_guard",
	"identity_resolution_queue_backlog",
	"payment_order_without_user_guard",
	"projection_freshness_customer_read_model",
	"questionnaire_submission_without_user_guard",
	"retired_table_runtime_reference_guard",
	"schema_drift_guard",
	"table_lifecycle_manifest_guard",
	"unionid_orphan_fact_guard",
	"wecom_media_lease_health",
}

// DataHealthCheck is a fully local registry result. Its evidence deliberately
// contains only fixed low-cardinality aggregate values.
type DataHealthCheck struct {
	CheckID          string         `json:"check_id"`
	Title            string         `json:"title"`
	Status           string         `json:"status"`
	Severity         string         `json:"severity"`
	Summary          string         `json:"summary"`
	Evidence         map[string]any `json:"evidence"`
	Remediation      string         `json:"remediation"`
	GateDecision     string         `json:"gate_decision"`
	ReasonCode       string         `json:"reason_code"`
	Owner            string         `json:"owner"`
	CandidateRelated bool           `json:"candidate_related"`
	FirstObservedAt  string         `json:"first_observed_at"`
	LastObservedAt   string         `json:"last_observed_at"`
	ReplayPolicy     string         `json:"replay_policy"`
}

type DataHealthCounts struct {
	OK            int `json:"ok"`
	Warn          int `json:"warn"`
	Fail          int `json:"fail"`
	NotApplicable int `json:"not_applicable"`
}

type DataHealthGateCounts struct {
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
	Block int `json:"block"`
}

type DataHealthAggregate struct {
	Checks                  []DataHealthCheck    `json:"checks"`
	RegistryID              string               `json:"registry_id"`
	RegistrySHA256          string               `json:"registry_sha256"`
	RegistryMatchesManifest bool                 `json:"registry_matches_manifest"`
	ExcludedLegacyCheckIDs  []string             `json:"excluded_legacy_check_ids"`
	ObservedAt              string               `json:"observed_at"`
	Counts                  DataHealthCounts     `json:"counts"`
	GateCounts              DataHealthGateCounts `json:"gate_counts"`
	OverallStatus           string               `json:"overall_status"`
	OK                      bool                 `json:"ok"`
}

func EvaluateDataHealth(input Input, observedAt time.Time) DataHealthAggregate {
	return evaluateDataHealth(input, observedAt, dataHealthCheckIDs[:])
}

func evaluateDataHealth(input Input, observedAt time.Time, manifestIDs []string) DataHealthAggregate {
	observed := observedAt.UTC().Format(time.RFC3339Nano)
	checks := []DataHealthCheck{
		databaseReadinessCheck(input, observed),
		migrationCompatibilityCheck(input, observed),
		outboundBacklogCheck(input, observed),
		releaseSHACompleteCheck(input, observed),
	}
	aggregate := DataHealthAggregate{
		Checks: checks, RegistryID: DataHealthRegistryID, RegistrySHA256: dataHealthRegistrySHA256(),
		RegistryMatchesManifest: sameCheckIDSet(checks, manifestIDs),
		ExcludedLegacyCheckIDs:  append([]string(nil), dataHealthExcludedLegacyCheckIDs[:]...),
		ObservedAt:              observed,
	}
	for _, check := range checks {
		switch check.Status {
		case "ok":
			aggregate.Counts.OK++
		case "warn":
			aggregate.Counts.Warn++
		default:
			aggregate.Counts.Fail++
		}
		switch check.GateDecision {
		case "pass":
			aggregate.GateCounts.Pass++
		case "warn":
			aggregate.GateCounts.Warn++
		default:
			aggregate.GateCounts.Block++
		}
	}
	aggregate.OverallStatus = "ok"
	if !aggregate.RegistryMatchesManifest || aggregate.GateCounts.Block > 0 {
		aggregate.OverallStatus = "fail"
	} else if aggregate.Counts.Warn > 0 {
		aggregate.OverallStatus = "warn"
	}
	aggregate.OK = aggregate.RegistryMatchesManifest && aggregate.GateCounts.Block == 0
	return aggregate
}

func dataHealthCheck(id, title, status, summary, remediation, reason, observed string, evidence map[string]any) DataHealthCheck {
	severity, gate := "green", "pass"
	if status == "warn" {
		severity, gate = "yellow", "warn"
	}
	if status == "fail" {
		severity, gate = "red", "block"
	}
	return DataHealthCheck{CheckID: id, Title: title, Status: status, Severity: severity, Summary: summary, Evidence: evidence, Remediation: remediation, GateDecision: gate, ReasonCode: reason, Owner: "platform_readiness", CandidateRelated: false, FirstObservedAt: observed, LastObservedAt: observed, ReplayPolicy: "manual_after_remediation"}
}

func databaseReadinessCheck(input Input, observed string) DataHealthCheck {
	readable := input.Database.Kind == DatabasePostgres && input.Database.Probe == ProbeHealthy
	if readable {
		return dataHealthCheck("database_readiness", "Database readiness", "ok", "Local database is readable.", "No action required.", "database_readable", observed, map[string]any{"database_readable": true})
	}
	return dataHealthCheck("database_readiness", "Database readiness", "fail", "Local database readiness is unavailable.", "Restore local database readability and retry manually.", "database_unreadable", observed, map[string]any{"database_readable": false})
}

func migrationCompatibilityCheck(input Input, observed string) DataHealthCheck {
	compatible := input.Migration.Compatibility == MigrationCompatible
	if compatible {
		return dataHealthCheck("migration_compatibility", "Migration compatibility", "ok", "Local schema is compatible.", "No action required.", "schema_compatible", observed, map[string]any{"schema_compatible": true})
	}
	return dataHealthCheck("migration_compatibility", "Migration compatibility", "fail", "Local schema compatibility is unavailable.", "Restore schema compatibility and retry manually.", "schema_incompatible", observed, map[string]any{"schema_compatible": false})
}

func releaseSHACompleteCheck(input Input, observed string) DataHealthCheck {
	environment := "non_production"
	if input.Production {
		environment = "production"
	}
	evidence := map[string]any{"sha_complete": input.Release.SHAComplete, "environment": environment}
	if input.Release.SHAComplete {
		return dataHealthCheck("release_sha_complete", "Release SHA completeness", "ok", "Release SHA is complete.", "No action required.", "release_sha_complete", observed, evidence)
	}
	if input.Production {
		return dataHealthCheck("release_sha_complete", "Release SHA completeness", "fail", "Production release SHA is incomplete.", "Set a complete release SHA and retry manually.", "release_sha_incomplete_production", observed, evidence)
	}
	return dataHealthCheck("release_sha_complete", "Release SHA completeness", "warn", "Non-production release SHA is incomplete.", "Set a complete release SHA before production promotion.", "release_sha_incomplete_non_production", observed, evidence)
}

func outboundBacklogCheck(input Input, observed string) DataHealthCheck {
	available := input.Queues.Probe == ProbeHealthy && !input.Queues.BudgetExhausted
	if !available {
		return dataHealthCheck("outbound_outcome_unknown_backlog", "Outbound outcome-unknown backlog", "fail", "Outcome-unknown backlog observation is unavailable.", "Restore the local queue observation and retry manually.", "queue_observation_unavailable", observed, map[string]any{"queue_observation_available": false})
	}
	evidence := map[string]any{"queue_observation_available": true, "outcome_unknown_count": input.Queues.UnknownAfterDispatch}
	if input.Queues.UnknownAfterDispatch == 0 {
		return dataHealthCheck("outbound_outcome_unknown_backlog", "Outbound outcome-unknown backlog", "ok", "No outcome-unknown backlog is observed.", "No action required.", "outcome_unknown_backlog_clear", observed, evidence)
	}
	return dataHealthCheck("outbound_outcome_unknown_backlog", "Outbound outcome-unknown backlog", "warn", "Outcome-unknown backlog requires review.", "Resolve the local outcome-unknown backlog and retry manually.", "outcome_unknown_backlog_present", observed, evidence)
}

func dataHealthRegistrySHA256() string {
	encoded, _ := json.Marshal(dataHealthCheckIDs)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func sameCheckIDSet(checks []DataHealthCheck, manifest []string) bool {
	if len(checks) != len(manifest) {
		return false
	}
	seen := make(map[string]bool, len(checks))
	for _, check := range checks {
		seen[check.CheckID] = true
	}
	for _, id := range manifest {
		if !seen[id] {
			return false
		}
		delete(seen, id)
	}
	return len(seen) == 0
}
