import { describe, expect, it, vi } from "vitest";
import {
  DATA_HEALTH_CHECK_IDS,
  DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
  loadDataHealthDetail,
  loadDataHealthOverview,
  parseDataHealthCheck,
  parseDataHealthDetail,
  parseDataHealthRegistry,
  parseDataHealthSummary,
  type DataHealthTransport,
} from "./data-health";

const observedAt = "2026-08-19T08:00:00Z";
const sha = "a".repeat(64);

const checks = [
  {
    check_id: "database_readiness",
    title: "Database readiness",
    status: "ok",
    severity: "green",
    summary: "Local database is readable.",
    evidence: { database_readable: true },
    remediation: "No action required.",
    gate_decision: "pass",
    reason_code: "database_readable",
    owner: "platform_readiness",
    candidate_related: false,
    first_observed_at: observedAt,
    last_observed_at: observedAt,
    replay_policy: "manual_after_remediation",
  },
  {
    check_id: "migration_compatibility",
    title: "Migration compatibility",
    status: "ok",
    severity: "green",
    summary: "Local schema is compatible.",
    evidence: { schema_compatible: true },
    remediation: "No action required.",
    gate_decision: "pass",
    reason_code: "schema_compatible",
    owner: "platform_readiness",
    candidate_related: false,
    first_observed_at: observedAt,
    last_observed_at: observedAt,
    replay_policy: "manual_after_remediation",
  },
  {
    check_id: "outbound_outcome_unknown_backlog",
    title: "Outbound outcome-unknown backlog",
    status: "ok",
    severity: "green",
    summary: "No outcome-unknown backlog is observed.",
    evidence: { queue_observation_available: true, outcome_unknown_count: 0 },
    remediation: "No action required.",
    gate_decision: "pass",
    reason_code: "outcome_unknown_backlog_clear",
    owner: "platform_readiness",
    candidate_related: false,
    first_observed_at: observedAt,
    last_observed_at: observedAt,
    replay_policy: "manual_after_remediation",
  },
  {
    check_id: "release_sha_complete",
    title: "Release SHA completeness",
    status: "ok",
    severity: "green",
    summary: "Release SHA is complete.",
    evidence: { sha_complete: true, environment: "production" },
    remediation: "No action required.",
    gate_decision: "pass",
    reason_code: "release_sha_complete",
    owner: "platform_readiness",
    candidate_related: false,
    first_observed_at: observedAt,
    last_observed_at: observedAt,
    replay_policy: "manual_after_remediation",
  },
];

function registry() {
  return {
    ok: true,
    checks,
    registry_id: "v2-core-readiness.v1",
    registry_sha256: sha,
    registry_matches_manifest: true,
    excluded_legacy_check_ids: DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
    observed_at: observedAt,
    real_external_call_executed: false,
  };
}

function summary() {
  return {
    ...registry(),
    overall_status: "ok",
    counts: { ok: 4, warn: 0, fail: 0, not_applicable: 0 },
    gate_counts: { pass: 4, warn: 0, block: 0 },
  };
}

function detail() {
  return {
    ok: true,
    check: checks[0],
    registry_id: "v2-core-readiness.v1",
    registry_sha256: sha,
    registry_matches_manifest: true,
    observed_at: observedAt,
    real_external_call_executed: false,
  };
}

function transport(
  overrides: Partial<DataHealthTransport> = {},
): DataHealthTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    summary: vi.fn(async () => ({ status: 503, data: {} })),
    detail: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  } as DataHealthTransport;
}

describe("data-health local registry contract", () => {
  it("accepts exactly the four frozen checks and nineteen exclusions", () => {
    expect(DATA_HEALTH_CHECK_IDS).toHaveLength(4);
    expect(DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS).toHaveLength(19);
    expect(parseDataHealthCheck(checks[0])).toMatchObject({
      checkID: "database_readiness",
      status: "ok",
    });
    expect(parseDataHealthRegistry(registry())).toMatchObject({
      registryID: "v2-core-readiness.v1",
      excludedLegacyCheckIDs: DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
    });
    expect(parseDataHealthSummary(summary())).toMatchObject({
      overallStatus: "ok",
      counts: { ok: 4, warn: 0, fail: 0, notApplicable: 0 },
    });
    expect(parseDataHealthDetail(detail(), "database_readiness")).toMatchObject(
      {
        check: { checkID: "database_readiness" },
      },
    );
  });

  it.each([
    { ...checks[0], unexpected: true },
    {
      ...checks[1],
      evidence: { schema_compatible: true, compatible_ahead: true },
    },
    {
      ...checks[2],
      evidence: {
        queue_observation_available: true,
        outcome_unknown_count: -1,
      },
    },
    {
      ...checks[3],
      evidence: { sha_complete: false, environment: "production" },
    },
  ])("rejects expanded or contradicted local check %#", (value) => {
    expect(parseDataHealthCheck(value)).toBeUndefined();
  });

  it("rejects external effect claims, partial exclusions, and incorrect aggregate counts", () => {
    expect(
      parseDataHealthRegistry({
        ...registry(),
        real_external_call_executed: true,
      }),
    ).toBeUndefined();
    expect(
      parseDataHealthRegistry({
        ...registry(),
        excluded_legacy_check_ids:
          DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS.slice(1),
      }),
    ).toBeUndefined();
    expect(
      parseDataHealthSummary({
        ...summary(),
        counts: { ok: 3, warn: 1, fail: 0, not_applicable: 0 },
      }),
    ).toBeUndefined();
    expect(
      parseDataHealthDetail(
        {
          ...detail(),
          check: { ...checks[0], check_id: "migration_compatibility" },
        },
        "database_readiness",
      ),
    ).toBeUndefined();
  });
});

describe("data-health same-origin local transport", () => {
  it("loads only the summary and registry GETs, without treating their observation times as one snapshot", async () => {
    const laterSummary = {
      ...summary(),
      observed_at: "2026-08-19T08:00:01Z",
      checks: checks.map((check) => ({
        ...check,
        first_observed_at: "2026-08-19T08:00:01Z",
        last_observed_at: "2026-08-19T08:00:01Z",
      })),
    };
    const client = transport({
      list: vi.fn(async () => ({ status: 200, data: registry() })),
      summary: vi.fn(async () => ({ status: 200, data: laterSummary })),
    });
    await expect(loadDataHealthOverview(client)).resolves.toMatchObject({
      status: "loaded",
    });
    expect(client.list).toHaveBeenCalledWith({ credentials: "same-origin" });
    expect(client.summary).toHaveBeenCalledWith({ credentials: "same-origin" });
    expect(client.detail).not.toHaveBeenCalled();
  });

  it("uses detail GET only for one fixed known ID and fails closed otherwise", async () => {
    const client = transport({
      detail: vi.fn(async () => ({ status: 200, data: detail() })),
    });
    await expect(
      loadDataHealthDetail(client, "database_readiness"),
    ).resolves.toMatchObject({ status: "loaded" });
    expect(client.detail).toHaveBeenCalledWith("database_readiness", {
      credentials: "same-origin",
    });
    await expect(
      loadDataHealthDetail(client, "unknown" as never),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.detail).toHaveBeenCalledTimes(1);
  });

  it("prioritizes unauthenticated state and never retries malformed or unavailable reads", async () => {
    const client = transport({
      list: vi.fn(async () => ({ status: 503, data: {} })),
      summary: vi.fn(async () => ({ status: 401, data: {} })),
      detail: vi.fn(async () => ({
        status: 200,
        data: { ...detail(), real_external_call_executed: true },
      })),
    });
    await expect(loadDataHealthOverview(client)).resolves.toEqual({
      status: "unauthenticated",
    });
    await expect(
      loadDataHealthDetail(client, "database_readiness"),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.list).toHaveBeenCalledOnce();
    expect(client.summary).toHaveBeenCalledOnce();
    expect(client.detail).toHaveBeenCalledOnce();
  });
});
