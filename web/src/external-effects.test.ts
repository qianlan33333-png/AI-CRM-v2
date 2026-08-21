import { describe, expect, it, vi } from "vitest";
import {
  EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT,
  EXTERNAL_EFFECTS_DELIVERY_SEMANTICS,
  EXTERNAL_EFFECTS_PAGE_SIZE,
  externalEffectClassificationForStatus,
  loadExternalEffectsSnapshot,
  normalizeExternalEffectsFilters,
  parseExternalEffectJobPage,
  parseExternalEffectsDiagnostics,
  type ExternalEffectClassification,
  type ExternalEffectStatus,
  type ExternalEffectsFilters,
  type ExternalEffectsTransport,
} from "./external-effects";

const generatedAt = "2026-08-21T05:00:00Z";
const jobID = "eej_v1_AAAAAAAAAAAAAAAAAAAAAA";
const nextCursor = `eec_v1_${"A".repeat(24)}`;

function wireJob(
  status: ExternalEffectStatus = "outcome_unknown",
  id = jobID,
) {
  return {
    id,
    status,
    classification: externalEffectClassificationForStatus(status),
    attempt_count: status === "pending" || status === "cancelled" ? 0 : 2,
    created_at: "2026-08-21T04:00:00Z",
    status_updated_at: "2026-08-21T04:30:00Z",
  };
}

function wireJobs(
  filters: ExternalEffectsFilters = {},
  items = [wireJob()],
) {
  return {
    ok: true,
    items,
    next_cursor: null as string | null,
    page_size: EXTERNAL_EFFECTS_PAGE_SIZE,
    applied_filters: {
      status: filters.status ?? null,
      classification: filters.classification ?? null,
    },
    provider_execution_eligible: false,
    real_external_call_executed: false,
    delivery_proven: false,
    local_fact_only: true,
    delivery_semantics: EXTERNAL_EFFECTS_DELIVERY_SEMANTICS,
  };
}

function wireDiagnostics() {
  return {
    ok: true,
    counts: {
      total: 28,
      by_status: {
        pending: 1,
        sending: 2,
        sent: 3,
        retryable_failed: 4,
        final_failed: 5,
        outcome_unknown: 6,
        cancelled: 7,
      },
      by_classification: {
        safe_local_handling: 5,
        frozen: 12,
        manual_review: 11,
      },
    },
    risk_summary: {
      level: "outcome_unknown_present",
      outcome_unknown_count: 6,
      manual_review_count: 11,
      manual_review_required: true,
    },
    generated_at: generatedAt,
    provider_execution_eligible: false,
    real_external_call_executed: false,
    delivery_proven: false,
    local_fact_only: true,
    delivery_semantics: EXTERNAL_EFFECTS_DELIVERY_SEMANTICS,
  };
}

function transport(
  jobs: unknown = wireJobs(),
  diagnostics: unknown = wireDiagnostics(),
): ExternalEffectsTransport {
  return {
    readJobs: vi.fn(async () => ({ status: 200, data: jobs })),
    readDiagnostics: vi.fn(async () => ({ status: 200, data: diagnostics })),
  };
}

function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

describe("External Effects closed client", () => {
  it("loads jobs and diagnostics with one exact filter set and no delivery claim", async () => {
    const filters = {
      status: "outcome_unknown",
      classification: "manual_review",
    } as const;
    const jobs = wireJobs(filters);
    jobs.next_cursor = nextCursor;
    const api = transport(jobs);
    const signal = new AbortController().signal;

    const result = await loadExternalEffectsSnapshot(
      api,
      filters,
      undefined,
      signal,
    );

    expect(result.status).toBe("loaded");
    if (result.status !== "loaded") return;
    expect(result.snapshot.page.providerExecutionEligible).toBe(false);
    expect(result.snapshot.page.realExternalCallExecuted).toBe(false);
    expect(result.snapshot.page.deliveryProven).toBe(false);
    expect(result.snapshot.diagnostics.deliveryProven).toBe(false);
    expect(result.snapshot.page.items[0]).toMatchObject({
      status: "outcome_unknown",
      classification: "manual_review",
    });
    expect(api.readJobs).toHaveBeenCalledWith(
      {
        status: "outcome_unknown",
        classification: "manual_review",
        cursor: undefined,
        limit: EXTERNAL_EFFECTS_PAGE_SIZE,
      },
      { credentials: "same-origin", signal },
    );
    expect(api.readDiagnostics).toHaveBeenCalledWith({
      credentials: "same-origin",
      signal,
    });
    expect(
      (api.readJobs as ReturnType<typeof vi.fn>).mock.calls[0][1],
    ).toBe(
      (api.readDiagnostics as ReturnType<typeof vi.fn>).mock.calls[0][0],
    );
  });

  it("treats sent as frozen local state and never as delivery proof", () => {
    const page = parseExternalEffectJobPage(wireJobs({}, [wireJob("sent")]), {});
    expect(page).toMatchObject({
      deliveryProven: false,
      providerExecutionEligible: false,
      realExternalCallExecuted: false,
      items: [{ status: "sent", classification: "frozen" }],
    });
  });

  it.each([
    "pending",
    "sending",
    "sent",
    "retryable_failed",
    "final_failed",
    "outcome_unknown",
    "cancelled",
  ] as const)(
    "accepts only the approved %s lifecycle projection",
    (status: ExternalEffectStatus) => {
      const expected = externalEffectClassificationForStatus(status);
      const filters = { status, classification: expected };
      expect(
        parseExternalEffectJobPage(wireJobs(filters, [wireJob(status)]), filters),
      ).toBeDefined();
    },
  );

  it("rejects unknown, sensitive, or internally inconsistent successful job DTOs", () => {
    const mutations: readonly ((...[]: [ReturnType<typeof wireJobs>]) => void)[] = [
      (value) => { (value as Record<string, unknown>).recipient = "raw-user"; },
      (value) => { (value.items[0] as Record<string, unknown>).message_body = "secret"; },
      (value) => { (value.items[0] as Record<string, unknown>).payload = { raw: true }; },
      (value) => { (value.items[0] as Record<string, unknown>).provider_token = "token"; },
      (value) => { (value.items[0] as Record<string, unknown>).receipt = "raw"; },
      (value) => { value.items[0].id = "42"; },
      (value) => { value.items[0].status = "queued" as ExternalEffectStatus; },
      (value) => { value.items[0].status = "accepted" as ExternalEffectStatus; },
      (value) => { value.items[0].status = "completed" as ExternalEffectStatus; },
      (value) => { value.items[0].classification = "frozen"; },
      (value) => { value.items[0].attempt_count = 0; },
      (value) => { value.items[0].status_updated_at = "not-a-time"; },
      (value) => { value.provider_execution_eligible = true; },
      (value) => { value.real_external_call_executed = true; },
      (value) => { value.delivery_proven = true; },
      (value) => { value.local_fact_only = false; },
      (value) => { (value as Record<string, unknown>).delivery_semantics = "delivered"; },
      (value) => { value.page_size = 49; },
      (value) => { value.next_cursor = "raw-cursor"; },
      (value) => { value.applied_filters.status = "pending"; },
      (value) => { value.items.push(deepClone(value.items[0])); },
    ];
    for (const mutate of mutations) {
      const value = deepClone(wireJobs());
      mutate(value);
      expect(parseExternalEffectJobPage(value, {})).toBeUndefined();
    }
  });

  it("rejects illegal diagnostics sums, risks, flags, and extra fields", () => {
    const mutations: readonly ((...[]: [ReturnType<typeof wireDiagnostics>]) => void)[] = [
      (value) => { (value as Record<string, unknown>).raw_payload = {}; },
      (value) => { value.counts.total += 1; },
      (value) => { value.counts.by_status.outcome_unknown += 1; },
      (value) => { value.counts.by_classification.manual_review += 1; },
      (value) => { value.risk_summary.level = "none"; },
      (value) => { value.risk_summary.outcome_unknown_count = 0; },
      (value) => { value.risk_summary.manual_review_required = false; },
      (value) => { value.generated_at = "2026-02-31T00:00:00Z"; },
      (value) => { value.provider_execution_eligible = true; },
      (value) => { value.real_external_call_executed = true; },
      (value) => { value.delivery_proven = true; },
      (value) => { value.local_fact_only = false; },
    ];
    for (const mutate of mutations) {
      const value = deepClone(wireDiagnostics());
      mutate(value);
      expect(parseExternalEffectsDiagnostics(value)).toBeUndefined();
    }
  });

  it.each([
    [401, 401, "unauthenticated"],
    [403, 200, "forbidden"],
    [400, 200, "invalid"],
    [422, 200, "invalid"],
    [503, 200, "unavailable"],
    [200, 500, "unavailable"],
  ] as const)(
    "fails closed for paired status %s/%s",
    async (
      jobsStatus: number,
      diagnosticStatus: number,
      expected: "unauthenticated" | "forbidden" | "invalid" | "unavailable",
    ) => {
      const api: ExternalEffectsTransport = {
        readJobs: vi.fn(async () => ({ status: jobsStatus, data: {} })),
        readDiagnostics: vi.fn(async () => ({ status: diagnosticStatus, data: {} })),
      };
      await expect(loadExternalEffectsSnapshot(api, {})).resolves.toEqual({
        status: expected,
      });
    },
  );

  it("rejects an illegal 200 response atomically instead of keeping one half", async () => {
    const invalid = deepClone(wireJobs());
    invalid.items[0].id = "raw-task-id";
    await expect(loadExternalEffectsSnapshot(transport(invalid), {})).resolves.toEqual({
      status: "invalid",
    });
  });

  it("rejects malformed cursor before making either request", async () => {
    const api = transport();
    await expect(loadExternalEffectsSnapshot(api, {}, "raw-42")).resolves.toEqual({
      status: "invalid",
    });
    expect(api.readJobs).not.toHaveBeenCalled();
    expect(api.readDiagnostics).not.toHaveBeenCalled();
  });
});

describe("External Effects closed filters", () => {
  it("normalizes empty and approved values", () => {
    expect(normalizeExternalEffectsFilters(EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT)).toMatchObject({
      status: "valid",
      filters: {},
    });
    expect(
      normalizeExternalEffectsFilters({
        status: "sent",
        classification: "frozen",
      }),
    ).toMatchObject({
      status: "valid",
      filters: { status: "sent", classification: "frozen" },
    });
  });

  it.each([
    [{ status: "completed", classification: "" }, "unknown status"],
    [{ status: " pending", classification: "" }, "leading space"],
    [{ status: "sent", classification: "manual_review" }, "mismatch"],
  ] as const)(
    "rejects %s (%s)",
    (
      draft: { readonly status: string; readonly classification: string },
      _reason: string,
    ) => {
      void _reason;
      expect(normalizeExternalEffectsFilters(draft)).toMatchObject({
        status: "invalid",
      });
    },
  );

  it("keeps classification mapping closed", () => {
    const expected: Readonly<Record<ExternalEffectStatus, ExternalEffectClassification>> = {
      pending: "safe_local_handling",
      sending: "frozen",
      sent: "frozen",
      retryable_failed: "safe_local_handling",
      final_failed: "manual_review",
      outcome_unknown: "manual_review",
      cancelled: "frozen",
    };
    for (const [status, classification] of Object.entries(expected)) {
      expect(externalEffectClassificationForStatus(status as ExternalEffectStatus)).toBe(classification);
    }
  });
});
