import { describe, expect, it, vi } from "vitest";
import {
  PUSH_CENTER_LANE_KEYS,
  PUSH_CENTER_SECTION_KEYS,
  PUSH_CENTER_STATUS_KEYS,
  loadPushCenterSnapshot,
  normalizePushCenterFilters,
  parsePushCenterPair,
  parsePushCenterRuntimeQueue,
  type PushCenterFilters,
  type PushCenterTransport,
} from "./push-center";

function normalizedFilters(
  value: Record<string, string> = {},
): PushCenterFilters {
  const result = normalizePushCenterFilters(value);
  if (result.status !== "valid") throw new Error(result.message);
  return result.filters;
}

function statusDefinitions() {
  return PUSH_CENTER_STATUS_KEYS.map((key) => ({
    key,
    label: `状态 ${key}`,
    definition: `本地定义 ${key}`,
  }));
}

function sections() {
  return PUSH_CENTER_SECTION_KEYS.map((key, index) => ({
    key,
    label: `分区 ${index}`,
    effect_types: index === 0 ? ["wecom.message.private.send"] : [],
    capability_key: index === 0 ? "questionnaire_external_push" : "",
    count: index === 0 ? 6 : 0,
  }));
}

function internalEvent(seed = 0) {
  return {
    raw_open: seed + 6,
    raw_due: seed + 5,
    eligible: seed + 4,
    failed_retryable: seed + 3,
    failed_terminal: seed + 2,
    blocked: seed + 1,
  };
}

function runtimeQueue() {
  return {
    policy_version: "queue-policy-v1",
    active_generation: 7,
    claim_enabled: true,
    rollout_mode: "observe",
    lanes: PUSH_CENTER_LANE_KEYS.map((lane, index) => ({
      lane,
      max_in_flight: index + 1,
      enabled: index % 2 === 0,
      rollout_mode: "observe",
      blocked_until: index === 0 ? "2026-08-20T12:30:00Z" : null,
      policy_version: "queue-policy-v1",
      raw_open: index + 10,
      held: index + 1,
      eligible: index + 2,
      policy_gated: index + 3,
      scheduled: index + 4,
      retry_wait: index + 5,
      rate_limited: index + 6,
      in_flight: index + 7,
      unknown: index + 8,
      dlq: index + 9,
      oldest_eligible_age_seconds: index + 0.5,
      internal_event: internalEvent(index),
      throughput_last_minute: index + 1.25,
      accepted_last_minute: index + 2.25,
      p95_queue_wait_ms: index + 3.25,
      p95_provider_call_ms: index + 4.25,
      estimated_drain_seconds: index + 5.25,
      rate_limit_count_last_hour: index + 6.25,
      task_acceptance_rate_1m: index + 0.75,
    })),
    raw_open: 60,
    held: 6,
    eligible: 12,
    policy_gated: 18,
    scheduled: 24,
    retry_wait: 30,
    rate_limited: 36,
    in_flight: 42,
    unknown: 48,
    dlq: 54,
    internal_event: internalEvent(),
  };
}

function normalResponses(filters: PushCenterFilters = {}) {
  const shared = {
    ok: true,
    filters,
    route_owner: "ai_crm_next",
  };
  return {
    sections: {
      ...shared,
      sections: sections(),
      status_definitions: statusDefinitions(),
    },
    stats: {
      ...shared,
      sections: sections(),
      status_definitions: statusDefinitions(),
      counts: {
        total: 6,
        by_effective_status: {
          pending: 1,
          running: 1,
          succeeded: 1,
          sent: 1,
          failed: 1,
          reconciled: 1,
        },
        by_status: {
          pending: 1,
          running: 1,
          succeeded: 1,
          sent: 1,
          failed: 1,
          sent_with_shadow_warning: 1,
        },
        by_section: { questionnaire: 6 },
        pending: 1,
        running: 1,
        succeeded: 1,
        sent: 2,
        failed: 1,
        shadow_warning: 1,
      },
      real_external_call_executed: false,
      runtime_queue: runtimeQueue(),
      capability_owner: "ai_crm_next/platform_foundation/push_center",
    },
  };
}

function degradedResponse(filters: PushCenterFilters, stats: boolean) {
  return {
    ok: true,
    degraded: true,
    error: "",
    error_code: "production_read_unavailable",
    source_status: "production_unavailable",
    read_model_status: "unavailable",
    capability_owner: "ai_crm_next/platform_foundation/push_center",
    page_error: "推送中心读模型暂不可用，请稍后重试。",
    diagnostics: {
      production_data_ready: false,
      fixture_mode: false,
      allow_fixture_repo_in_prod: false,
      error_class: "ReadModelUnavailableError",
    },
    route_owner: "ai_crm_next",
    fallback_used: false,
    real_external_call_executed: false,
    status_code: 200,
    items: [],
    total: 0,
    counts: {
      total: 0,
      by_effective_status: {},
      by_status: {},
      by_section: {},
      pending: 0,
      running: 0,
      sent: 0,
      failed: 0,
    },
    status_definitions: statusDefinitions(),
    filters,
    limit: 50,
    offset: 0,
    sections: [],
    ...(stats ? { runtime_queue: {} } : {}),
  };
}

function transport(
  sectionsData: unknown,
  statsData: unknown,
  sectionsStatus = 200,
  statsStatus = 200,
): PushCenterTransport {
  return {
    readSections: vi.fn(async () => ({
      status: sectionsStatus,
      data: sectionsData,
    })),
    readStats: vi.fn(async () => ({ status: statsStatus, data: statsData })),
  };
}

describe("Push Center filter boundary", () => {
  it("trims, canonicalizes time, and keeps only the eight allowed non-empty filters", () => {
    expect(
      normalizePushCenterFilters({
        section: " questionnaire ",
        effect_type: " wecom.message.private.send ",
        status: " sent ",
        business_type: " order ",
        business_id: " biz-7 ",
        source_module: " outbound ",
        created_from: "2026-08-20T08:00:00+08:00",
        created_to: "2026-08-20T09:00:00+08:00",
      }),
    ).toEqual({
      status: "valid",
      filters: {
        section: "questionnaire",
        effect_type: "wecom.message.private.send",
        status: "sent",
        business_type: "order",
        business_id: "biz-7",
        source_module: "outbound",
        created_from: "2026-08-20T00:00:00.000Z",
        created_to: "2026-08-20T01:00:00.000Z",
      },
      key: expect.any(String),
    });
  });

  it.each([
    [{ target_id: "sensitive" }, "未允许字段"],
    [{ section: "unknown" }, "section"],
    [{ status: "completed" }, "status"],
    [{ business_id: "bad\u0000value" }, "非法字符"],
    [{ created_from: "2026-08-20" }, "RFC3339"],
    [
      {
        created_from: "2026-08-21T00:00:00Z",
        created_to: "2026-08-20T00:00:00Z",
      },
      "不能晚于",
    ],
  ])("rejects illegal local filters %#", (value, message) => {
    expect(normalizePushCenterFilters(value)).toMatchObject({
      status: "invalid",
      message: expect.stringContaining(message),
    });
  });
});

describe("Push Center closed pair decoder", () => {
  it("decodes normal sections, counts, 9 definitions, and the full runtime queue", () => {
    const filters = normalizedFilters({ section: "questionnaire" });
    const payload = normalResponses(filters);
    const snapshot = parsePushCenterPair(
      payload.sections,
      payload.stats,
      filters,
    );
    expect(snapshot).toMatchObject({
      boundary: "normal",
      filters,
      counts: { total: 6, sent: 2, shadowWarning: 1 },
      runtimeQueue: {
        status: "reported",
        activeGeneration: 7,
        lanes: expect.arrayContaining([
          expect.objectContaining({ lane: "wecom_media", dlq: 14 }),
        ]),
      },
    });
    if (snapshot?.boundary !== "normal") throw new Error("normal snapshot");
    expect(snapshot.sections).toHaveLength(13);
    expect(snapshot.statusDefinitions).toHaveLength(9);
    expect(snapshot.sections[0]).toMatchObject({
      effectTypes: ["wecom.message.private.send"],
      capabilityKey: "questionnaire_external_push",
    });
  });

  it("decodes a matched degraded boundary without inventing section counts", () => {
    const filters = normalizedFilters({ source_module: "outbound" });
    expect(
      parsePushCenterPair(
        degradedResponse(filters, false),
        degradedResponse(filters, true),
        filters,
      ),
    ).toEqual({
      boundary: "degraded",
      filters,
      statusDefinitions: expect.any(Array),
      runtimeQueue: { status: "not_reported" },
      realExternalCallExecuted: false,
      degraded: {
        sourceStatus: "production_unavailable",
        readModelStatus: "unavailable",
        pageError: "推送中心读模型暂不可用，请稍后重试。",
        diagnostics: {
          productionDataReady: false,
          fixtureMode: false,
          allowFixtureRepoInProd: false,
          errorClass: "ReadModelUnavailableError",
        },
      },
    });
  });

  it("accepts only empty or fully closed runtime_queue shapes", () => {
    expect(parsePushCenterRuntimeQueue({})).toEqual({ status: "not_reported" });
    expect(parsePushCenterRuntimeQueue(runtimeQueue())).toMatchObject({
      status: "reported",
      lanes: expect.any(Array),
    });
    expect(
      parsePushCenterRuntimeQueue({ ...runtimeQueue(), extra: "no" }),
    ).toBeUndefined();
  });

  it.each([
    (payload: ReturnType<typeof normalResponses>) => {
      payload.sections.sections[1] = {
        ...payload.sections.sections[1],
        key: "questionnaire",
      } as (typeof payload.sections.sections)[number];
    },
    (payload: ReturnType<typeof normalResponses>) => {
      payload.stats.counts.total = -1;
    },
    (payload: ReturnType<typeof normalResponses>) => {
      payload.stats.runtime_queue.lanes[0].blocked_until = "not-a-time";
    },
    (payload: ReturnType<typeof normalResponses>) => {
      Object.assign(payload.stats, { provider_receipt: "must-not-pass" });
    },
  ])("fails closed for duplicate sections, invalid counts/times, or extensions", (mutate) => {
    const payload = normalResponses();
    mutate(payload);
    expect(
      parsePushCenterPair(payload.sections, payload.stats, {}),
    ).toBeUndefined();
  });

  it("fails the whole pair when filters, sections, or status definitions disagree", () => {
    const filters = normalizedFilters({ business_id: "biz-7" });
    for (const mutate of [
      (payload: ReturnType<typeof normalResponses>) => {
        payload.stats.filters = { business_id: "biz-8" };
      },
      (payload: ReturnType<typeof normalResponses>) => {
        payload.stats.sections[0] = {
          ...payload.stats.sections[0],
          label: "另一标签",
        };
      },
      (payload: ReturnType<typeof normalResponses>) => {
        payload.stats.status_definitions[0] = {
          ...payload.stats.status_definitions[0],
          definition: "另一定义",
        };
      },
    ]) {
      const payload = normalResponses(filters);
      mutate(payload);
      expect(
        parsePushCenterPair(payload.sections, payload.stats, filters),
      ).toBeUndefined();
    }
  });

  it("rejects sensitive response filters even though the backend schema can carry them", () => {
    const payload = normalResponses();
    payload.sections.filters = { external_userid: "wx-secret" } as PushCenterFilters;
    payload.stats.filters = { external_userid: "wx-secret" } as PushCenterFilters;
    expect(
      parsePushCenterPair(payload.sections, payload.stats, {}),
    ).toBeUndefined();
  });

  it("rejects a normal/degraded half-pair", () => {
    const payload = normalResponses();
    expect(
      parsePushCenterPair(
        payload.sections,
        degradedResponse({}, true),
        {},
      ),
    ).toBeUndefined();
  });
});

describe("Push Center paired client", () => {
  it("sends the identical legal filters to sections and stats once", async () => {
    const filters = normalizedFilters({
      section: "questionnaire",
      created_from: "2026-08-20T00:00:00Z",
    });
    const payload = normalResponses(filters);
    const client = transport(payload.sections, payload.stats);
    await expect(loadPushCenterSnapshot(filters, client)).resolves.toMatchObject({
      status: "loaded",
      snapshot: { boundary: "normal" },
    });
    expect(client.readSections).toHaveBeenCalledTimes(1);
    expect(client.readStats).toHaveBeenCalledTimes(1);
    expect(client.readSections).toHaveBeenCalledWith(
      filters,
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(client.readStats).toHaveBeenCalledWith(
      filters,
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it.each([
    [401, 401, "unauthenticated"],
    [403, 403, "forbidden"],
    [200, 503, "unavailable"],
    [503, 200, "unavailable"],
  ] as const)("fails closed for paired status %s/%s", async (left, right, status) => {
    const payload = normalResponses();
    await expect(
      loadPushCenterSnapshot(
        {},
        transport(payload.sections, payload.stats, left, right),
      ),
    ).resolves.toEqual({ status });
  });

  it("returns invalid rather than exposing a half-verified 200", async () => {
    const payload = normalResponses();
    payload.stats.sections[0] = { ...payload.stats.sections[0], count: 5 };
    await expect(
      loadPushCenterSnapshot({}, transport(payload.sections, payload.stats)),
    ).resolves.toEqual({ status: "invalid" });
  });
});
