import { describe, expect, it, vi } from "vitest";
import {
  AUTOMATION_RUNS_PAGE_SIZE,
  loadAutomationDiagnostics,
  loadAutomationRuns,
  loadAutomationSourceEvent,
  nextAutomationRunsPage,
  parseAutomationSourceEvent,
  parseAutomationDiagnostics,
  parseAutomationRun,
  parseAutomationRunsPage,
  previousAutomationRunsPage,
  startAutomationSourceEventRead,
  type AutomationRunsTransport,
} from "./automation-runs";

const run = {
  run_id: "automation-trigger:11",
  request_id: "event:21",
  agent_code: "tag-trigger-v1",
  run_status: "completed",
  trigger_source: "customer.tag_applied",
  customer_id: 31,
  tag_id: 41,
  source_event_id: 51,
  triggered_event_id: 61,
  started_at: "2026-08-19T08:00:00Z",
  completed_at: "2026-08-19T08:00:01Z",
  has_error: false,
};

const sourceEvent = {
  ok: true,
  item: {
    event_id: 51,
    event_type: "customer.tag_applied",
    occurred_at: "2026-08-19T08:00:00Z",
    dispatched: true,
    deliveries: [
      {
        consumer: "automation.tag-trigger.v1",
        status: "completed",
        attempt_count: 1,
        completed_at: "2026-08-19T08:00:01Z",
      },
      {
        consumer: "stats.tag-applied.v1",
        status: "processing",
        attempt_count: 0,
        completed_at: null,
      },
    ],
  },
  observed_at: "2026-08-19T08:00:02Z",
  registry_id: "v2-internal-events.v1",
  source_status: "local_read_model",
  delivery_observation_available: true,
  external_delivery: "unknown",
  route_owner: "ai_crm_next",
  real_external_call_executed: false,
};

const diagnostics = {
  ok: true,
  filters: { event_type: "", consumer: "", status: "" },
  event_count: 2,
  undispatched_event_count: 1,
  delivery_counts: {
    pending: 1, processing: 2, completed: 3, final_failed: 4, outcome_unknown: 5,
  },
  consumer_registry: [
    { consumer: "automation.tag-trigger.v1", event_types: ["customer.tag_applied"] },
    { consumer: "stats.tag-applied.v1", event_types: ["customer.tag_applied"] },
    { consumer: "operation-cycle.fact.v1", event_types: ["operation_cycle.fact_recorded"] },
  ],
  observed_at: "2026-08-19T08:00:02Z",
  registry_id: "v2-internal-events.v1",
  source_status: "local_read_model",
  observed_domains: ["event_log", "event_deliveries"],
  unobserved_domains: ["river_queue", "outbound_provider", "external_delivery"],
  external_delivery: "unknown",
  route_owner: "ai_crm_next",
  real_external_call_executed: false,
};

function transport(
  overrides: Partial<AutomationRunsTransport> = {},
): AutomationRunsTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    sourceEvent: vi.fn(async () => ({ status: 503, data: {} })),
    diagnostics: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  } as AutomationRunsTransport;
}

describe("automation run receipt contract", () => {
  it("accepts only the exact masked receipt fields", () => {
    expect(parseAutomationRun(run)).toMatchObject({
      runID: "automation-trigger:11",
      customerID: 31,
      hasError: false,
    });
    expect(
      parseAutomationRunsPage(
        {
          items: [run],
          total: 51,
          page: 1,
          page_size: AUTOMATION_RUNS_PAGE_SIZE,
          visibility: "masked",
        },
        1,
      ),
    ).toMatchObject({ total: 51, page: 1 });
  });

  it.each([
    { ...run, unionid: "raw-identity" },
    { ...run, userid: "raw-identity" },
    { ...run, run_id: "automation-trigger:0" },
    { ...run, request_id: "event:0" },
    { ...run, agent_code: "other" },
    { ...run, run_status: "running" },
    { ...run, trigger_source: "manual" },
    { ...run, started_at: "2026-02-30T08:00:00Z" },
    { ...run, completed_at: "2026-08-19T07:59:59Z" },
    { ...run, has_error: true },
  ])("rejects expanded or contradictory receipt %#", (value) => {
    expect(parseAutomationRun(value)).toBeUndefined();
  });

  it("rejects wrong page, size, visibility, and duplicate run identifiers", () => {
    const valid = {
      items: [run],
      total: 1,
      page: 1,
      page_size: AUTOMATION_RUNS_PAGE_SIZE,
      visibility: "masked",
    };
    expect(parseAutomationRunsPage({ ...valid, page: 2 }, 1)).toBeUndefined();
    expect(
      parseAutomationRunsPage({ ...valid, page_size: 20 }, 1),
    ).toBeUndefined();
    expect(
      parseAutomationRunsPage({ ...valid, visibility: "full" }, 1),
    ).toBeUndefined();
    expect(
      parseAutomationRunsPage({ ...valid, items: [run, run], total: 2 }, 1),
    ).toBeUndefined();
  });
});

describe("automation internal-event diagnostics contract", () => {
  it("accepts only the exact unfiltered local diagnostic summary", () => {
    expect(parseAutomationDiagnostics(diagnostics)).toMatchObject({
      eventCount: 2,
      undispatchedEventCount: 1,
      deliveryCounts: { completed: 3, outcome_unknown: 5 },
      observedDomains: ["event_log", "event_deliveries"],
      unobservedDomains: ["river_queue", "outbound_provider", "external_delivery"],
    });
  });

  it.each([
    { ...diagnostics, extra: true },
    { ...diagnostics, filters: { ...diagnostics.filters, status: "completed" } },
    { ...diagnostics, event_count: -1 },
    { ...diagnostics, undispatched_event_count: 3 },
    { ...diagnostics, delivery_counts: { ...diagnostics.delivery_counts, extra: 1 } },
    { ...diagnostics, consumer_registry: [...diagnostics.consumer_registry].reverse() },
    { ...diagnostics, consumer_registry: [{ ...diagnostics.consumer_registry[0], event_types: ["operation_cycle.fact_recorded"] }, ...diagnostics.consumer_registry.slice(1)] },
    { ...diagnostics, observed_domains: ["event_deliveries", "event_log"] },
    { ...diagnostics, unobserved_domains: ["river_queue", "external_delivery", "outbound_provider"] },
    { ...diagnostics, external_delivery: "sent" },
    { ...diagnostics, real_external_call_executed: true },
  ])("fails closed when diagnostic facts drift %#", (value) => {
    expect(parseAutomationDiagnostics(value)).toBeUndefined();
  });

  it("uses only one fixed-empty-filter same-origin GET and never retries", async () => {
    const client = transport({
      diagnostics: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: diagnostics })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadAutomationDiagnostics(client)).resolves.toMatchObject({ status: "loaded" });
    expect(client.diagnostics).toHaveBeenCalledWith({}, { credentials: "same-origin" });
    await expect(loadAutomationDiagnostics(client)).resolves.toEqual({ status: "unauthenticated" });
    await expect(loadAutomationDiagnostics(client)).resolves.toEqual({ status: "unavailable" });
    expect(client.diagnostics).toHaveBeenCalledTimes(3);
  });
});

describe("automation source event contract", () => {
  it("accepts only the ID-bound local source-event observation", () => {
    expect(parseAutomationSourceEvent(sourceEvent, 51)).toMatchObject({
      eventID: 51,
      eventType: "customer.tag_applied",
      deliveries: [
        {
          consumer: "automation.tag-trigger.v1",
          completedAt: "2026-08-19T08:00:01Z",
        },
        { consumer: "stats.tag-applied.v1", completedAt: null },
      ],
    });
  });

  it.each(["final_failed", "outcome_unknown"] as const)(
    "accepts terminal %s only with the database-required completion time",
    (status) => {
      expect(
        parseAutomationSourceEvent(
          {
            ...sourceEvent,
            item: {
              ...sourceEvent.item,
              deliveries: [{ ...sourceEvent.item.deliveries[0], status }],
            },
          },
          51,
        ),
      ).toMatchObject({
        deliveries: [{ status, completedAt: "2026-08-19T08:00:01Z" }],
      });
    },
  );

  it.each([
    { ...sourceEvent, extra: true },
    { ...sourceEvent, external_delivery: "sent" },
    { ...sourceEvent, real_external_call_executed: true },
    { ...sourceEvent, item: { ...sourceEvent.item, event_id: 61 } },
    {
      ...sourceEvent,
      item: {
        ...sourceEvent.item,
        event_type: "operation_cycle.fact_recorded",
      },
    },
    {
      ...sourceEvent,
      item: {
        ...sourceEvent.item,
        deliveries: [
          { ...sourceEvent.item.deliveries[0], consumer: "outside.consumer" },
        ],
      },
    },
    {
      ...sourceEvent,
      item: {
        ...sourceEvent.item,
        deliveries: [{ ...sourceEvent.item.deliveries[0], completed_at: null }],
      },
    },
    {
      ...sourceEvent,
      item: {
        ...sourceEvent.item,
        deliveries: [
          {
            ...sourceEvent.item.deliveries[1],
            completed_at: "2026-08-19T08:00:01Z",
          },
        ],
      },
    },
    {
      ...sourceEvent,
      item: {
        ...sourceEvent.item,
        deliveries: [
          sourceEvent.item.deliveries[0],
          sourceEvent.item.deliveries[0],
        ],
      },
    },
  ])("rejects a drifted or externally-claimed source event %#", (value) => {
    expect(parseAutomationSourceEvent(value, 51)).toBeUndefined();
  });

  it("uses only a same-origin GET and fails closed", async () => {
    const client = transport({
      sourceEvent: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: sourceEvent })
        .mockResolvedValueOnce({ status: 404, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadAutomationSourceEvent(client, 51)).resolves.toMatchObject({
      status: "loaded",
    });
    expect(client.sourceEvent).toHaveBeenCalledWith(51, {
      credentials: "same-origin",
    });
    await expect(loadAutomationSourceEvent(client, 51)).resolves.toEqual({
      status: "not_found",
    });
    await expect(loadAutomationSourceEvent(client, 51)).resolves.toEqual({
      status: "unavailable",
    });
    await expect(loadAutomationSourceEvent(client, 0)).resolves.toEqual({
      status: "invalid",
    });
  });

  it("single-flights two synchronous source-event reads", async () => {
    const lock = { current: false };
    let release: (() => void) | undefined;
    const pending = new Promise<void>((resolve) => {
      release = resolve;
    });
    const execute = vi.fn(async () => pending);
    const first = startAutomationSourceEventRead(lock, execute);
    const second = startAutomationSourceEventRead(lock, execute);
    expect(first).toBeDefined();
    expect(second).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();
    release?.();
    await first;
    expect(lock.current).toBe(false);
  });
});

describe("automation run receipt transport", () => {
  it("uses only masked fixed-page same-origin GET parameters", async () => {
    const client = transport({
      list: vi.fn(async () => ({
        status: 200,
        data: {
          items: [run],
          total: 51,
          page: 2,
          page_size: 50,
          visibility: "masked",
        },
      })),
    });
    await expect(loadAutomationRuns(client, 2)).resolves.toMatchObject({
      status: "loaded",
    });
    expect(client.list).toHaveBeenCalledWith(
      { page: 2, page_size: 50, visibility: "masked" },
      { credentials: "same-origin" },
    );
  });

  it("fails closed for invalid page, response, status, and network failures", async () => {
    const client = transport({
      list: vi
        .fn()
        .mockResolvedValueOnce({
          status: 200,
          data: {
            items: [],
            total: 0,
            page: 1,
            page_size: 50,
            visibility: "unmasked",
          },
        })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadAutomationRuns(client, 0)).resolves.toEqual({
      status: "invalid",
    });
    expect(client.list).not.toHaveBeenCalled();
    await expect(loadAutomationRuns(client)).resolves.toEqual({
      status: "invalid",
    });
    await expect(loadAutomationRuns(client)).resolves.toEqual({
      status: "unauthenticated",
    });
    await expect(loadAutomationRuns(client)).resolves.toEqual({
      status: "unavailable",
    });
  });

  it("calculates only safe page transitions", () => {
    const page = parseAutomationRunsPage(
      { items: [run], total: 51, page: 1, page_size: 50, visibility: "masked" },
      1,
    );
    if (!page) throw new Error("expected page");
    expect(previousAutomationRunsPage(page)).toBeUndefined();
    expect(nextAutomationRunsPage(page)).toBe(2);
  });
});
