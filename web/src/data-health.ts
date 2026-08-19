import {
  getLegacyDataHealthCheck,
  getLegacyDataHealthSummary,
  listLegacyDataHealthChecks,
} from "./api/generated/health";

export type DataHealthRole = "admin" | "ops" | "sales";

export const DATA_HEALTH_CHECK_IDS = [
  "database_readiness",
  "migration_compatibility",
  "outbound_outcome_unknown_backlog",
  "release_sha_complete",
] as const;

export type DataHealthCheckID = (typeof DATA_HEALTH_CHECK_IDS)[number];

export const DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS = [
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
] as const;

type CheckStatus = "ok" | "warn" | "fail";
type Severity = "green" | "yellow" | "red";
type GateDecision = "pass" | "warn" | "block";
type Evidence = Readonly<Record<string, boolean | number | string>>;

export interface DataHealthCheck {
  readonly checkID: DataHealthCheckID;
  readonly title: string;
  readonly status: CheckStatus;
  readonly severity: Severity;
  readonly summary: string;
  readonly evidence: Evidence;
  readonly remediation: string;
  readonly gateDecision: GateDecision;
  readonly reasonCode: string;
  readonly owner: "platform_readiness";
  readonly candidateRelated: false;
  readonly firstObservedAt: string;
  readonly lastObservedAt: string;
  readonly replayPolicy: "manual_after_remediation";
}

export interface DataHealthRegistry {
  readonly ok: boolean;
  readonly checks: readonly DataHealthCheck[];
  readonly registryID: "v2-core-readiness.v1";
  readonly registrySHA256: string;
  readonly excludedLegacyCheckIDs: readonly string[];
  readonly observedAt: string;
}

export interface DataHealthSummary extends DataHealthRegistry {
  readonly overallStatus: CheckStatus;
  readonly counts: Readonly<Record<CheckStatus | "notApplicable", number>>;
  readonly gateCounts: Readonly<Record<GateDecision, number>>;
}

export interface DataHealthDetail {
  readonly ok: boolean;
  readonly check: DataHealthCheck;
  readonly registryID: "v2-core-readiness.v1";
  readonly registrySHA256: string;
  readonly observedAt: string;
}

export interface DataHealthTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  options: RequestInit,
): Promise<DataHealthTransportResponse> {
  return listLegacyDataHealthChecks(options);
}

async function generatedSummary(
  options: RequestInit,
): Promise<DataHealthTransportResponse> {
  return getLegacyDataHealthSummary(options);
}

async function generatedDetail(
  checkID: DataHealthCheckID,
  options: RequestInit,
): Promise<DataHealthTransportResponse> {
  return getLegacyDataHealthCheck(checkID, options);
}

export interface DataHealthTransport {
  readonly list: typeof generatedList;
  readonly summary: typeof generatedSummary;
  readonly detail: typeof generatedDetail;
}

export const generatedDataHealthTransport: DataHealthTransport = {
  list: generatedList,
  summary: generatedSummary,
  detail: generatedDetail,
};

export type DataHealthFailure =
  "unauthenticated" | "forbidden" | "not_found" | "invalid" | "unavailable";

export type DataHealthOverviewResult =
  | {
      readonly status: "loaded";
      readonly registry: DataHealthRegistry;
      readonly summary: DataHealthSummary;
    }
  | { readonly status: DataHealthFailure };

export type DataHealthDetailResult =
  | { readonly status: "loaded"; readonly detail: DataHealthDetail }
  | { readonly status: DataHealthFailure };

function record(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null)
  );
}

function exact(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  const actual = Object.keys(value);
  return (
    actual.length === keys.length && actual.every((key) => keys.includes(key))
  );
}

function timestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) ||
    !Number.isFinite(Date.parse(value))
  ) {
    return false;
  }
  const parts = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  if (!parts) return false;
  const [year, month, day, hour, minute, second] = parts.slice(1).map(Number);
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, 0);
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day &&
    date.getUTCHours() === hour &&
    date.getUTCMinutes() === minute &&
    date.getUTCSeconds() === second
  );
}

function isCheckID(value: unknown): value is DataHealthCheckID {
  return (
    typeof value === "string" &&
    DATA_HEALTH_CHECK_IDS.includes(value as DataHealthCheckID)
  );
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function stringArrayEquals(
  value: unknown,
  expected: readonly string[],
): value is string[] {
  return (
    Array.isArray(value) &&
    value.length === expected.length &&
    value.every((item, index) => item === expected[index])
  );
}

function expectedState(
  id: DataHealthCheckID,
  evidence: Record<string, unknown>,
):
  | {
      readonly title: string;
      readonly status: CheckStatus;
      readonly summary: string;
      readonly remediation: string;
      readonly reasonCode: string;
    }
  | undefined {
  if (id === "database_readiness") {
    if (
      !exact(evidence, ["database_readable"]) ||
      typeof evidence.database_readable !== "boolean"
    )
      return undefined;
    return evidence.database_readable
      ? {
          title: "Database readiness",
          status: "ok",
          summary: "Local database is readable.",
          remediation: "No action required.",
          reasonCode: "database_readable",
        }
      : {
          title: "Database readiness",
          status: "fail",
          summary: "Local database readiness is unavailable.",
          remediation: "Restore local database readability and retry manually.",
          reasonCode: "database_unreadable",
        };
  }
  if (id === "migration_compatibility") {
    if (
      !exact(evidence, ["schema_compatible"]) ||
      typeof evidence.schema_compatible !== "boolean"
    )
      return undefined;
    return evidence.schema_compatible
      ? {
          title: "Migration compatibility",
          status: "ok",
          summary: "Local schema is compatible.",
          remediation: "No action required.",
          reasonCode: "schema_compatible",
        }
      : {
          title: "Migration compatibility",
          status: "fail",
          summary: "Local schema compatibility is unavailable.",
          remediation: "Restore schema compatibility and retry manually.",
          reasonCode: "schema_incompatible",
        };
  }
  if (id === "outbound_outcome_unknown_backlog") {
    if (
      evidence.queue_observation_available === false &&
      exact(evidence, ["queue_observation_available"])
    ) {
      return {
        title: "Outbound outcome-unknown backlog",
        status: "fail",
        summary: "Outcome-unknown backlog observation is unavailable.",
        remediation: "Restore the local queue observation and retry manually.",
        reasonCode: "queue_observation_unavailable",
      };
    }
    if (
      evidence.queue_observation_available !== true ||
      !nonnegative(evidence.outcome_unknown_count) ||
      !exact(evidence, ["queue_observation_available", "outcome_unknown_count"])
    )
      return undefined;
    return evidence.outcome_unknown_count === 0
      ? {
          title: "Outbound outcome-unknown backlog",
          status: "ok",
          summary: "No outcome-unknown backlog is observed.",
          remediation: "No action required.",
          reasonCode: "outcome_unknown_backlog_clear",
        }
      : {
          title: "Outbound outcome-unknown backlog",
          status: "warn",
          summary: "Outcome-unknown backlog requires review.",
          remediation:
            "Resolve the local outcome-unknown backlog and retry manually.",
          reasonCode: "outcome_unknown_backlog_present",
        };
  }
  if (
    !exact(evidence, ["sha_complete", "environment"]) ||
    typeof evidence.sha_complete !== "boolean" ||
    (evidence.environment !== "production" &&
      evidence.environment !== "non_production")
  )
    return undefined;
  if (evidence.sha_complete) {
    return {
      title: "Release SHA completeness",
      status: "ok",
      summary: "Release SHA is complete.",
      remediation: "No action required.",
      reasonCode: "release_sha_complete",
    };
  }
  return evidence.environment === "production"
    ? {
        title: "Release SHA completeness",
        status: "fail",
        summary: "Production release SHA is incomplete.",
        remediation: "Set a complete release SHA and retry manually.",
        reasonCode: "release_sha_incomplete_production",
      }
    : {
        title: "Release SHA completeness",
        status: "warn",
        summary: "Non-production release SHA is incomplete.",
        remediation: "Set a complete release SHA before production promotion.",
        reasonCode: "release_sha_incomplete_non_production",
      };
}

export function parseDataHealthCheck(
  value: unknown,
): DataHealthCheck | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "check_id",
      "title",
      "status",
      "severity",
      "summary",
      "evidence",
      "remediation",
      "gate_decision",
      "reason_code",
      "owner",
      "candidate_related",
      "first_observed_at",
      "last_observed_at",
      "replay_policy",
    ]) ||
    !isCheckID(value.check_id) ||
    !record(value.evidence) ||
    !timestamp(value.first_observed_at) ||
    value.first_observed_at !== value.last_observed_at ||
    !timestamp(value.last_observed_at) ||
    value.owner !== "platform_readiness" ||
    value.candidate_related !== false ||
    value.replay_policy !== "manual_after_remediation"
  )
    return undefined;

  const expected = expectedState(value.check_id, value.evidence);
  const severity: Record<CheckStatus, Severity> = {
    ok: "green",
    warn: "yellow",
    fail: "red",
  };
  const gateDecision: Record<CheckStatus, GateDecision> = {
    ok: "pass",
    warn: "warn",
    fail: "block",
  };
  if (
    !expected ||
    value.title !== expected.title ||
    value.status !== expected.status ||
    value.severity !== severity[expected.status] ||
    value.summary !== expected.summary ||
    value.remediation !== expected.remediation ||
    value.reason_code !== expected.reasonCode ||
    value.gate_decision !== gateDecision[expected.status]
  )
    return undefined;

  return {
    checkID: value.check_id,
    title: value.title,
    status: expected.status,
    severity: severity[expected.status],
    summary: value.summary,
    evidence: value.evidence as Evidence,
    remediation: value.remediation,
    gateDecision: gateDecision[expected.status],
    reasonCode: value.reason_code,
    owner: "platform_readiness",
    candidateRelated: false,
    firstObservedAt: value.first_observed_at,
    lastObservedAt: value.last_observed_at,
    replayPolicy: "manual_after_remediation",
  };
}

function parseChecks(
  value: unknown,
  observedAt: string,
): readonly DataHealthCheck[] | undefined {
  if (!Array.isArray(value) || value.length !== DATA_HEALTH_CHECK_IDS.length)
    return undefined;
  const checks = value.map(parseDataHealthCheck);
  if (
    checks.some((check) => check === undefined) ||
    !checks.every(
      (check, index) =>
        check?.checkID === DATA_HEALTH_CHECK_IDS[index] &&
        check.lastObservedAt === observedAt,
    )
  )
    return undefined;
  return checks as DataHealthCheck[];
}

function parseRegistryBase(
  value: Record<string, unknown>,
):
  | {
      readonly registryID: "v2-core-readiness.v1";
      readonly registrySHA256: string;
      readonly observedAt: string;
    }
  | undefined {
  if (
    value.registry_id !== "v2-core-readiness.v1" ||
    typeof value.registry_sha256 !== "string" ||
    !/^[a-f0-9]{64}$/.test(value.registry_sha256) ||
    value.registry_matches_manifest !== true ||
    !timestamp(value.observed_at) ||
    value.real_external_call_executed !== false
  )
    return undefined;
  return {
    registryID: value.registry_id,
    registrySHA256: value.registry_sha256,
    observedAt: value.observed_at,
  };
}

function expectedCounts(checks: readonly DataHealthCheck[]): {
  readonly counts: Record<CheckStatus, number>;
  readonly gates: Record<GateDecision, number>;
} {
  const counts: Record<CheckStatus, number> = { ok: 0, warn: 0, fail: 0 };
  const gates: Record<GateDecision, number> = { pass: 0, warn: 0, block: 0 };
  for (const check of checks) {
    counts[check.status] += 1;
    gates[check.gateDecision] += 1;
  }
  return { counts, gates };
}

export function parseDataHealthRegistry(
  value: unknown,
): DataHealthRegistry | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "checks",
      "registry_id",
      "registry_sha256",
      "registry_matches_manifest",
      "excluded_legacy_check_ids",
      "observed_at",
      "real_external_call_executed",
    ])
  )
    return undefined;
  const base = parseRegistryBase(value);
  if (
    !base ||
    !stringArrayEquals(
      value.excluded_legacy_check_ids,
      DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
    )
  )
    return undefined;
  const checks = parseChecks(value.checks, base.observedAt);
  if (!checks) return undefined;
  const { gates } = expectedCounts(checks);
  if (value.ok !== (gates.block === 0)) return undefined;
  return {
    ok: value.ok,
    checks,
    ...base,
    excludedLegacyCheckIDs: [...DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS],
  };
}

export function parseDataHealthSummary(
  value: unknown,
): DataHealthSummary | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "overall_status",
      "counts",
      "checks",
      "gate_counts",
      "registry_id",
      "registry_sha256",
      "registry_matches_manifest",
      "excluded_legacy_check_ids",
      "observed_at",
      "real_external_call_executed",
    ]) ||
    !record(value.counts) ||
    !exact(value.counts, ["ok", "warn", "fail", "not_applicable"]) ||
    !record(value.gate_counts) ||
    !exact(value.gate_counts, ["pass", "warn", "block"])
  )
    return undefined;
  const base = parseRegistryBase(value);
  if (
    !base ||
    !stringArrayEquals(
      value.excluded_legacy_check_ids,
      DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
    )
  )
    return undefined;
  const checks = parseChecks(value.checks, base.observedAt);
  if (!checks) return undefined;
  const expected = expectedCounts(checks);
  const overallStatus: CheckStatus =
    expected.gates.block > 0
      ? "fail"
      : expected.counts.warn > 0
        ? "warn"
        : "ok";
  if (
    value.overall_status !== overallStatus ||
    value.ok !== (expected.gates.block === 0) ||
    value.counts.ok !== expected.counts.ok ||
    value.counts.warn !== expected.counts.warn ||
    value.counts.fail !== expected.counts.fail ||
    value.counts.not_applicable !== 0 ||
    value.gate_counts.pass !== expected.gates.pass ||
    value.gate_counts.warn !== expected.gates.warn ||
    value.gate_counts.block !== expected.gates.block
  )
    return undefined;
  return {
    ok: value.ok,
    checks,
    ...base,
    excludedLegacyCheckIDs: [...DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS],
    overallStatus,
    counts: { ...expected.counts, notApplicable: 0 },
    gateCounts: expected.gates,
  };
}

export function parseDataHealthDetail(
  value: unknown,
  expectedID: DataHealthCheckID,
): DataHealthDetail | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "check",
      "registry_id",
      "registry_sha256",
      "registry_matches_manifest",
      "observed_at",
      "real_external_call_executed",
    ])
  )
    return undefined;
  const base = parseRegistryBase(value);
  const check = parseDataHealthCheck(value.check);
  if (
    !base ||
    !check ||
    check.checkID !== expectedID ||
    check.lastObservedAt !== base.observedAt ||
    value.ok !== (check.gateDecision !== "block")
  )
    return undefined;
  return { ok: value.ok, check, ...base };
}

function failure(status: number): DataHealthFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  return "unavailable";
}

function strongestFailure(
  ...failures: readonly DataHealthFailure[]
): DataHealthFailure {
  for (const candidate of [
    "unauthenticated",
    "forbidden",
    "not_found",
    "invalid",
    "unavailable",
  ] as const) {
    if (failures.includes(candidate)) return candidate;
  }
  return "unavailable";
}

export async function loadDataHealthOverview(
  transport: DataHealthTransport,
): Promise<DataHealthOverviewResult> {
  try {
    const [registryResponse, summaryResponse] = await Promise.all([
      transport.list({ credentials: "same-origin" }),
      transport.summary({ credentials: "same-origin" }),
    ]);
    const failures: DataHealthFailure[] = [];
    if (registryResponse.status !== 200)
      failures.push(failure(registryResponse.status));
    if (summaryResponse.status !== 200)
      failures.push(failure(summaryResponse.status));
    if (failures.length > 0) return { status: strongestFailure(...failures) };
    const registry = parseDataHealthRegistry(registryResponse.data);
    const summary = parseDataHealthSummary(summaryResponse.data);
    return registry && summary
      ? { status: "loaded", registry, summary }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadDataHealthDetail(
  transport: DataHealthTransport,
  checkID: DataHealthCheckID,
): Promise<DataHealthDetailResult> {
  if (!isCheckID(checkID)) return { status: "invalid" };
  try {
    const response = await transport.detail(checkID, {
      credentials: "same-origin",
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const detail = parseDataHealthDetail(response.data, checkID);
    return detail ? { status: "loaded", detail } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}
