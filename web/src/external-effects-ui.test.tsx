import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  ExternalEffectsPage,
  ExternalEffectsView,
  invalidateExternalEffectsRead,
  startExternalEffectsRead,
  type ExternalEffectsReadController,
  type ExternalEffectsState,
} from "./external-effects-ui";
import {
  EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT,
  type ExternalEffectStatus,
  type ExternalEffectsFilters,
  type ExternalEffectsSnapshot,
  type ExternalEffectsTransport,
} from "./external-effects";

const generatedAt = "2026-08-21T04:00:00Z";
const jobID = "eej_v1_AAAAAAAAAAAAAAAAAAAAAA";

function classification(status: ExternalEffectStatus) {
  if (status === "pending" || status === "retryable_failed") {
    return "safe_local_handling" as const;
  }
  if (status === "final_failed" || status === "outcome_unknown") {
    return "manual_review" as const;
  }
  return "frozen" as const;
}

function jobsPayload(
  filters: ExternalEffectsFilters = {},
  status: ExternalEffectStatus = filters.status ?? "outcome_unknown",
) {
  return {
    ok: true,
    items: [
      {
        id: jobID,
        status,
        classification: classification(status),
        attempt_count: status === "pending" || status === "cancelled" ? 0 : 1,
        created_at: generatedAt,
        status_updated_at: generatedAt,
      },
    ],
    next_cursor: null,
    page_size: 50,
    applied_filters: {
      status: filters.status ?? null,
      classification: filters.classification ?? null,
    },
    provider_execution_eligible: false,
    real_external_call_executed: false,
    delivery_proven: false,
    local_fact_only: true,
    delivery_semantics: "local_state_not_delivery_proof",
  };
}

function diagnosticsPayload() {
  return {
    ok: true,
    counts: {
      total: 1,
      by_status: {
        pending: 0,
        sending: 0,
        sent: 0,
        retryable_failed: 0,
        final_failed: 0,
        outcome_unknown: 1,
        cancelled: 0,
      },
      by_classification: {
        safe_local_handling: 0,
        frozen: 0,
        manual_review: 1,
      },
    },
    risk_summary: {
      level: "outcome_unknown_present",
      outcome_unknown_count: 1,
      manual_review_count: 1,
      manual_review_required: true,
    },
    generated_at: generatedAt,
    provider_execution_eligible: false,
    real_external_call_executed: false,
    delivery_proven: false,
    local_fact_only: true,
    delivery_semantics: "local_state_not_delivery_proof",
  };
}

function snapshot(filters: ExternalEffectsFilters = {}): ExternalEffectsSnapshot {
  const status = filters.status ?? "outcome_unknown";
  return {
    filters,
    page: {
      items: [
        {
          id: jobID,
          status,
          classification: classification(status),
          attemptCount: status === "pending" || status === "cancelled" ? 0 : 1,
          createdAt: generatedAt,
          statusUpdatedAt: generatedAt,
        },
      ],
      pageSize: 50,
      appliedFilters: filters,
      providerExecutionEligible: false,
      realExternalCallExecuted: false,
      deliveryProven: false,
      localFactOnly: true,
      deliverySemantics: "local_state_not_delivery_proof",
    },
    diagnostics: {
      total: 1,
      byStatus: {
        pending: 0,
        sending: 0,
        sent: 0,
        retryableFailed: 0,
        finalFailed: 0,
        outcomeUnknown: 1,
        cancelled: 0,
      },
      byClassification: {
        safeLocalHandling: 0,
        frozen: 0,
        manualReview: 1,
      },
      risk: {
        level: "outcome_unknown_present",
        outcomeUnknownCount: 1,
        manualReviewCount: 1,
        manualReviewRequired: true,
      },
      generatedAt,
      providerExecutionEligible: false,
      realExternalCallExecuted: false,
      deliveryProven: false,
      localFactOnly: true,
      deliverySemantics: "local_state_not_delivery_proof",
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

function controller(
  states: ExternalEffectsState[],
  onUnauthenticated = vi.fn(),
) {
  return {
    generation: { current: 0 },
    active: {
      current: undefined as ExternalEffectsReadController["active"]["current"],
    },
    verified: { current: undefined as ExternalEffectsSnapshot | undefined },
    mounted: { current: true },
    unauthenticatedNotified: { current: false },
    onState: (state: ExternalEffectsState) => states.push(state),
    onUnauthenticated,
  } satisfies ExternalEffectsReadController;
}

describe("External Effects read-only page", () => {
  it("renders only local facts, explicit no-effect flags, and no write action", () => {
    const html = renderToStaticMarkup(
      <ExternalEffectsView
        state={{ kind: "ready", snapshot: snapshot() }}
        draft={EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT}
        onRefresh={vi.fn()}
      />,
    );

    expect(html).toContain("provider_execution_eligible=false");
    expect(html).toContain("real_external_call_executed=false");
    expect(html).toContain("不构成送达证明");
    expect(html).toContain(jobID);
    expect(html).toContain("外部结果未知");
    const buttons = [...html.matchAll(/<button[^>]*>(.*?)<\/button>/gu)]
      .map((match) => match[1])
      .join(" ");
    expect(buttons).not.toMatch(/执行|发送|重试|取消|run-due|dispatch/iu);
    for (const rawValue of [
      "raw-user-123",
      "secret message body",
      "provider-token-value",
      "provider-receipt-value",
    ]) {
      expect(html).not.toContain(rawValue);
    }
  });

  it.each(["admin", "ops"] as const)(
    "allows the %s page shell",
    (role: "admin" | "ops") => {
      const transport: ExternalEffectsTransport = {
        readJobs: vi.fn(),
        readDiagnostics: vi.fn(),
      };
      const html = renderToStaticMarkup(
        <ExternalEffectsPage role={role} transport={transport} />,
      );
      expect(html).toContain("External Effects");
      expect(html).toContain("正在读取 jobs 与 diagnostics");
    },
  );

  it("keeps sales fail closed and does not touch transport", () => {
    const transport: ExternalEffectsTransport = {
      readJobs: vi.fn(),
      readDiagnostics: vi.fn(),
    };
    const html = renderToStaticMarkup(
      <ExternalEffectsPage role="sales" transport={transport} />,
    );
    expect(html).toContain("没有 External Effects 本地诊断读取权限");
    expect(transport.readJobs).not.toHaveBeenCalled();
    expect(transport.readDiagnostics).not.toHaveBeenCalled();
  });
});

describe("External Effects generation ownership", () => {
  it("single-flights one key and ignores a stale response after filter replacement", async () => {
    const oldJobs = deferred<{ status: number; data: unknown; }>();
    const oldDiagnostics = deferred<{ status: number; data: unknown; }>();
    const currentJobs = deferred<{ status: number; data: unknown; }>();
    const currentDiagnostics = deferred<{ status: number; data: unknown; }>();
    const transport: ExternalEffectsTransport = {
      readJobs: vi
        .fn()
        .mockReturnValueOnce(oldJobs.promise)
        .mockReturnValueOnce(currentJobs.promise),
      readDiagnostics: vi
        .fn()
        .mockReturnValueOnce(oldDiagnostics.promise)
        .mockReturnValueOnce(currentDiagnostics.promise),
    };
    const states: ExternalEffectsState[] = [];
    const control = controller(states);
    const currentFilters = {
      status: "pending",
      classification: "safe_local_handling",
    } as const;

    const first = startExternalEffectsRead(control, transport, {});
    const same = startExternalEffectsRead(control, transport, {});
    expect(same).toBe(first);
    expect(transport.readJobs).toHaveBeenCalledTimes(1);
    expect(transport.readDiagnostics).toHaveBeenCalledTimes(1);

    const second = startExternalEffectsRead(
      control,
      transport,
      currentFilters,
    );
    expect(transport.readJobs).toHaveBeenCalledTimes(2);
    expect(transport.readDiagnostics).toHaveBeenCalledTimes(2);
    const currentToken = control.active.current?.token;

    oldJobs.resolve({ status: 200, data: jobsPayload() });
    oldDiagnostics.resolve({ status: 200, data: diagnosticsPayload() });
    await first;
    expect(control.active.current?.token).toBe(currentToken);
    expect(states.at(-1)).toMatchObject({
      kind: "loading",
      filters: currentFilters,
    });

    currentJobs.resolve({
      status: 200,
      data: jobsPayload(currentFilters, "pending"),
    });
    currentDiagnostics.resolve({ status: 200, data: diagnosticsPayload() });
    await second;
    expect(control.active.current).toBeUndefined();
    expect(states.at(-1)).toMatchObject({
      kind: "ready",
      snapshot: { filters: currentFilters },
    });
  });

  it("aborts on unmount and ignores the late result and 401 callback", async () => {
    const jobs = deferred<{ status: number; data: unknown; }>();
    const diagnostics = deferred<{ status: number; data: unknown; }>();
    const states: ExternalEffectsState[] = [];
    const onUnauthenticated = vi.fn();
    const control = controller(states, onUnauthenticated);
    const transport: ExternalEffectsTransport = {
      readJobs: vi.fn(() => jobs.promise),
      readDiagnostics: vi.fn(() => diagnostics.promise),
    };

    const request = startExternalEffectsRead(control, transport, {});
    const signal = (
      transport.readJobs as ReturnType<typeof vi.fn>
    ).mock.calls[0][1].signal as AbortSignal;
    invalidateExternalEffectsRead(control);
    expect(signal.aborted).toBe(true);
    jobs.resolve({ status: 401, data: {} });
    diagnostics.resolve({ status: 401, data: {} });
    await request;

    expect(states).toHaveLength(1);
    expect(states[0]).toMatchObject({ kind: "loading" });
    expect(onUnauthenticated).not.toHaveBeenCalled();
    expect(control.active.current).toBeUndefined();
  });

  it("notifies unauthenticated only once across paired calls and refreshes", async () => {
    const states: ExternalEffectsState[] = [];
    const onUnauthenticated = vi.fn();
    const control = controller(states, onUnauthenticated);
    const transport: ExternalEffectsTransport = {
      readJobs: vi.fn(async () => ({ status: 401, data: {} })),
      readDiagnostics: vi.fn(async () => ({ status: 401, data: {} })),
    };

    await startExternalEffectsRead(control, transport, {});
    await startExternalEffectsRead(control, transport, {
      status: "pending",
      classification: "safe_local_handling",
    });

    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    expect(states.at(-1)).toMatchObject({
      kind: "error",
      failure: "unauthenticated",
    });
  });

  it("keeps the latest verified snapshot when a later read is unavailable", async () => {
    const transport: ExternalEffectsTransport = {
      readJobs: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: jobsPayload() })
        .mockResolvedValueOnce({ status: 503, data: {} }),
      readDiagnostics: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: diagnosticsPayload() })
        .mockResolvedValueOnce({ status: 503, data: {} }),
    };
    const states: ExternalEffectsState[] = [];
    const control = controller(states);

    await startExternalEffectsRead(control, transport, {});
    const verified = control.verified.current;
    expect(verified).toBeDefined();
    await startExternalEffectsRead(control, transport, {});

    expect(states.at(-1)).toEqual({
      kind: "error",
      failure: "unavailable",
      filters: {},
      cursor: undefined,
      previous: verified,
    });
    expect(control.verified.current).toBe(verified);
  });
});
