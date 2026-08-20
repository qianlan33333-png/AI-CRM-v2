import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  PushCenterPage,
  PushCenterView,
  invalidatePushCenterRead,
  startPushCenterRead,
  type PushCenterReadController,
  type PushCenterState,
} from "./push-center-ui";
import {
  EMPTY_PUSH_CENTER_FILTER_DRAFT,
  PUSH_CENTER_LANE_KEYS,
  PUSH_CENTER_SECTION_KEYS,
  PUSH_CENTER_STATUS_KEYS,
  type PushCenterFilters,
  type PushCenterSnapshot,
  type PushCenterTransport,
} from "./push-center";

function statusDefinitions() {
  return PUSH_CENTER_STATUS_KEYS.map((key) => ({
    key,
    label: `状态 ${key}`,
    definition: `本地定义 ${key}`,
  }));
}

function internalEvent(seed = 0) {
  return {
    rawOpen: seed + 6,
    rawDue: seed + 5,
    eligible: seed + 4,
    failedRetryable: seed + 3,
    failedTerminal: seed + 2,
    blocked: seed + 1,
  };
}

function normalSnapshot(filters: PushCenterFilters = {}): PushCenterSnapshot {
  return {
    boundary: "normal",
    filters,
    sections: PUSH_CENTER_SECTION_KEYS.map((key, index) => ({
      key,
      label: `分区 ${index}`,
      effectTypes: index === 0 ? ["wecom.message.private.send"] : [],
      capabilityKey: index === 0 ? "questionnaire_external_push" : "",
      count: index === 0 ? 6 : 0,
    })),
    statusDefinitions: statusDefinitions(),
    counts: {
      total: 6,
      byEffectiveStatus: { pending: 1, reconciled: 5 },
      byStatus: { pending: 1, sent: 5 },
      bySection: { questionnaire: 6 },
      pending: 1,
      running: 0,
      succeeded: 0,
      sent: 5,
      failed: 0,
      shadowWarning: 0,
    },
    runtimeQueue: {
      status: "reported",
      policyVersion: "queue-policy-v1",
      activeGeneration: 7,
      claimEnabled: true,
      rolloutMode: "observe",
      lanes: PUSH_CENTER_LANE_KEYS.map((lane, index) => ({
        lane,
        maxInFlight: index + 1,
        enabled: true,
        rolloutMode: "observe",
        blockedUntil: null,
        policyVersion: "queue-policy-v1",
        rawOpen: index + 10,
        held: index + 1,
        eligible: index + 2,
        policyGated: index + 3,
        scheduled: index + 4,
        retryWait: index + 5,
        rateLimited: index + 6,
        inFlight: index + 7,
        unknown: index + 8,
        dlq: index + 9,
        oldestEligibleAgeSeconds: index + 0.5,
        internalEvent: internalEvent(index),
        throughputLastMinute: index + 1.25,
        acceptedLastMinute: index + 2.25,
        p95QueueWaitMs: index + 3.25,
        p95ProviderCallMs: index + 4.25,
        estimatedDrainSeconds: index + 5.25,
        rateLimitCountLastHour: index + 6.25,
        taskAcceptanceRate1m: index + 0.75,
      })),
      rawOpen: 60,
      held: 6,
      eligible: 12,
      policyGated: 18,
      scheduled: 24,
      retryWait: 30,
      rateLimited: 36,
      inFlight: 42,
      unknown: 48,
      dlq: 54,
      internalEvent: internalEvent(),
    },
    realExternalCallExecuted: false,
  };
}

function degradedSnapshot(filters: PushCenterFilters = {}): PushCenterSnapshot {
  return {
    boundary: "degraded",
    filters,
    statusDefinitions: statusDefinitions(),
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
  };
}

function responsePayload(filters: PushCenterFilters = {}) {
  const sections = PUSH_CENTER_SECTION_KEYS.map((key, index) => ({
    key,
    label: `分区 ${index}`,
    effect_types: [],
    capability_key: "",
    count: index === 0 ? 1 : 0,
  }));
  const definitions = PUSH_CENTER_STATUS_KEYS.map((key) => ({
    key,
    label: key,
    definition: `definition ${key}`,
  }));
  return {
    sections: {
      ok: true,
      sections,
      status_definitions: definitions,
      filters,
      route_owner: "ai_crm_next",
    },
    stats: {
      ok: true,
      counts: {
        total: 1,
        by_effective_status: { pending: 1 },
        by_status: { pending: 1 },
        by_section: { questionnaire: 1 },
        pending: 1,
        running: 0,
        succeeded: 0,
        sent: 0,
        failed: 0,
        shadow_warning: 0,
      },
      sections,
      status_definitions: definitions,
      filters,
      route_owner: "ai_crm_next",
      real_external_call_executed: false,
      runtime_queue: {},
      capability_owner: "ai_crm_next/platform_foundation/push_center",
    },
  };
}

function deferred<T>() {
  let resolve = (value: T): void => {
    void value;
  };
  const promise = new Promise<T>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

function controller(states: PushCenterState[], onUnauthenticated = vi.fn()) {
  return {
    generation: { current: 0 },
    active: {
      current: undefined as PushCenterReadController["active"]["current"],
    },
    verified: { current: undefined as PushCenterSnapshot | undefined },
    mounted: { current: true },
    unauthenticatedNotified: { current: false },
    onState: (state: PushCenterState) => states.push(state),
    onUnauthenticated,
  } satisfies PushCenterReadController;
}

describe("Push Center complete read-only page", () => {
  it("renders totals, 13 sections, 9 definitions, runtime lanes, and the provider disclaimer", () => {
    const html = renderToStaticMarkup(
      <PushCenterView
        state={{ kind: "ready", snapshot: normalSnapshot() }}
        draft={EMPTY_PUSH_CENTER_FILTER_DRAFT}
        onRefresh={vi.fn()}
      />,
    );
    expect(html).toContain("不代表 Provider 已执行、已送达或业务成功");
    expect(html).toContain("仅展示本地分区聚合计数");
    expect(html).toContain("13 个本地 sections");
    expect(html).toContain("9 个内部 status definitions");
    expect(html).toContain("总数");
    expect(html).toContain("wecom_media");
    expect(html).toContain("本地 internal event summary");
    expect(html).toContain("source_status");
    for (const sensitive of [
      "external_userid",
      "owner_userid",
      "target_id",
      "idempotency_key",
    ]) {
      expect(html).not.toContain(sensitive);
    }
    const buttons = [...html.matchAll(/<button[^>]*>(.*?)<\/button>/gu)].map(
      (match) => match[1],
    );
    expect(buttons.join(" ")).not.toMatch(/取消|重试|回放|发送/u);
  });

  it("renders source_status and refuses to invent 13 counts in degraded mode", () => {
    const html = renderToStaticMarkup(
      <PushCenterView
        state={{ kind: "ready", snapshot: degradedSnapshot() }}
        onRefresh={vi.fn()}
      />,
    );
    expect(html).toContain("production_unavailable");
    expect(html).toContain("degraded 响应没有 13 个分区计数");
    expect(html).not.toContain("13 个本地 sections");
    expect(html).toContain("9 个内部 status definitions");
  });

  it("keeps filter replacement controls enabled while a read is in flight", () => {
    const html = renderToStaticMarkup(
      <PushCenterView
        state={{ kind: "loading", filters: {} }}
        draft={EMPTY_PUSH_CENTER_FILTER_DRAFT}
        onRefresh={vi.fn()}
      />,
    );
    expect(html).toContain('<button type="submit">应用本地筛选</button>');
    expect(html).toContain('<button type="button">清空筛选</button>');
    expect(html).toContain(
      '<button type="button" disabled="">刷新本地观测</button>',
    );
  });

  it.each(["ops", "sales"] as const)("keeps %s fail closed and inert", (role) => {
    const transport: PushCenterTransport = {
      readSections: vi.fn(),
      readStats: vi.fn(),
    };
    const html = renderToStaticMarkup(
      <PushCenterPage role={role} transport={transport} />,
    );
    expect(html).toContain("没有推送中心本地运营观测访问权限");
    expect(html).not.toContain("business_id");
    expect(transport.readSections).not.toHaveBeenCalled();
    expect(transport.readStats).not.toHaveBeenCalled();
  });
});

describe("Push Center generation ownership", () => {
  it("single-flights the same filters and replaces a different filter without late-finally corruption", async () => {
    const oldSections = deferred<{ status: number; data: unknown }>();
    const oldStats = deferred<{ status: number; data: unknown }>();
    const currentSections = deferred<{ status: number; data: unknown }>();
    const currentStats = deferred<{ status: number; data: unknown }>();
    const transport: PushCenterTransport = {
      readSections: vi
        .fn()
        .mockReturnValueOnce(oldSections.promise)
        .mockReturnValueOnce(currentSections.promise),
      readStats: vi
        .fn()
        .mockReturnValueOnce(oldStats.promise)
        .mockReturnValueOnce(currentStats.promise),
    };
    const states: PushCenterState[] = [];
    const control = controller(states);
    const firstFilters = { business_id: "old" };
    const currentFilters = { business_id: "current" };

    const first = startPushCenterRead(control, transport, firstFilters);
    const same = startPushCenterRead(control, transport, firstFilters);
    expect(same).toBe(first);
    expect(transport.readSections).toHaveBeenCalledTimes(1);
    expect(transport.readStats).toHaveBeenCalledTimes(1);

    const second = startPushCenterRead(control, transport, currentFilters);
    expect(transport.readSections).toHaveBeenCalledTimes(2);
    expect(transport.readStats).toHaveBeenCalledTimes(2);
    const currentToken = control.active.current?.token;

    const oldPayload = responsePayload(firstFilters);
    oldSections.resolve({ status: 200, data: oldPayload.sections });
    oldStats.resolve({ status: 200, data: oldPayload.stats });
    await first;
    expect(control.active.current?.token).toBe(currentToken);
    expect(states.at(-1)).toMatchObject({
      kind: "loading",
      filters: currentFilters,
    });

    const currentPayload = responsePayload(currentFilters);
    currentSections.resolve({ status: 200, data: currentPayload.sections });
    currentStats.resolve({ status: 200, data: currentPayload.stats });
    await second;
    expect(control.active.current).toBeUndefined();
    expect(states.at(-1)).toMatchObject({
      kind: "ready",
      snapshot: { filters: currentFilters },
    });
  });

  it("invalidates an unmounted read and ignores its result", async () => {
    const sections = deferred<{ status: number; data: unknown }>();
    const stats = deferred<{ status: number; data: unknown }>();
    const states: PushCenterState[] = [];
    const onUnauthenticated = vi.fn();
    const control = controller(states, onUnauthenticated);
    const transport: PushCenterTransport = {
      readSections: vi.fn(() => sections.promise),
      readStats: vi.fn(() => stats.promise),
    };
    const request = startPushCenterRead(control, transport, {});
    invalidatePushCenterRead(control);
    sections.resolve({ status: 401, data: {} });
    stats.resolve({ status: 401, data: {} });
    await request;
    expect(states).toHaveLength(1);
    expect(states[0]).toMatchObject({ kind: "loading" });
    expect(onUnauthenticated).not.toHaveBeenCalled();
    expect(control.active.current).toBeUndefined();
  });

  it("notifies 401 only once across paired calls and later refreshes", async () => {
    const onUnauthenticated = vi.fn();
    const states: PushCenterState[] = [];
    const control = controller(states, onUnauthenticated);
    const transport: PushCenterTransport = {
      readSections: vi.fn(async () => ({ status: 401, data: {} })),
      readStats: vi.fn(async () => ({ status: 401, data: {} })),
    };
    await startPushCenterRead(control, transport, {});
    await startPushCenterRead(control, transport, { business_id: "next" });
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    expect(states.at(-1)).toMatchObject({
      kind: "error",
      failure: "unauthenticated",
    });
  });

  it("retains the latest verified snapshot when a refresh fails", async () => {
    const payload = responsePayload();
    const transport: PushCenterTransport = {
      readSections: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: payload.sections })
        .mockResolvedValueOnce({ status: 503, data: {} }),
      readStats: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: payload.stats })
        .mockResolvedValueOnce({ status: 503, data: {} }),
    };
    const states: PushCenterState[] = [];
    const control = controller(states);
    await startPushCenterRead(control, transport, {});
    const verified = control.verified.current;
    expect(verified).toBeDefined();
    await startPushCenterRead(control, transport, {});
    expect(states.at(-1)).toEqual({
      kind: "error",
      failure: "unavailable",
      filters: {},
      previous: verified,
    });
    expect(control.verified.current).toBe(verified);
  });
});
