import { describe, expect, it, vi } from "vitest";
import {
  AUTOMATION_RUNS_PAGE_SIZE,
  loadAutomationRuns,
  nextAutomationRunsPage,
  parseAutomationRun,
  parseAutomationRunsPage,
  previousAutomationRunsPage,
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

function transport(overrides: Partial<AutomationRunsTransport> = {}): AutomationRunsTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
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
    expect(parseAutomationRunsPage({ ...valid, page_size: 20 }, 1)).toBeUndefined();
    expect(parseAutomationRunsPage({ ...valid, visibility: "full" }, 1)).toBeUndefined();
    expect(parseAutomationRunsPage({ ...valid, items: [run, run], total: 2 }, 1)).toBeUndefined();
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
    await expect(loadAutomationRuns(client, 2)).resolves.toMatchObject({ status: "loaded" });
    expect(client.list).toHaveBeenCalledWith(
      { page: 2, page_size: 50, visibility: "masked" },
      { credentials: "same-origin" },
    );
  });

  it("fails closed for invalid page, response, status, and network failures", async () => {
    const client = transport({
      list: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: { items: [], total: 0, page: 1, page_size: 50, visibility: "unmasked" } })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadAutomationRuns(client, 0)).resolves.toEqual({ status: "invalid" });
    expect(client.list).not.toHaveBeenCalled();
    await expect(loadAutomationRuns(client)).resolves.toEqual({ status: "invalid" });
    await expect(loadAutomationRuns(client)).resolves.toEqual({ status: "unauthenticated" });
    await expect(loadAutomationRuns(client)).resolves.toEqual({ status: "unavailable" });
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
