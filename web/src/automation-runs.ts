import {
  listAutomationTriggerRuns,
  type ListAutomationTriggerRunsParams,
} from "./api/generated/health";

export type AutomationRunsRole = "admin" | "ops" | "sales";
export const AUTOMATION_RUNS_PAGE_SIZE = 50;

export interface AutomationRunRecord {
  readonly runID: string;
  readonly requestID: string;
  readonly agentCode: "tag-trigger-v1";
  readonly runStatus: "completed";
  readonly triggerSource: "customer.tag_applied";
  readonly customerID: number;
  readonly tagID: number;
  readonly sourceEventID: number;
  readonly triggeredEventID: number;
  readonly startedAt: string;
  readonly completedAt: string;
  readonly hasError: false;
}

export interface AutomationRunsPage {
  readonly items: readonly AutomationRunRecord[];
  readonly total: number;
  readonly page: number;
  readonly pageSize: number;
}

export interface AutomationRunsTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  params: ListAutomationTriggerRunsParams,
  options: RequestInit,
): Promise<AutomationRunsTransportResponse> {
  return listAutomationTriggerRuns(params, options);
}

export interface AutomationRunsTransport {
  readonly list: typeof generatedList;
}

export const generatedAutomationRunsTransport: AutomationRunsTransport = {
  list: generatedList,
};

export type AutomationRunsFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type AutomationRunsResult =
  | { readonly status: "loaded"; readonly page: AutomationRunsPage }
  | { readonly status: AutomationRunsFailure };

function record(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null)
  );
}

function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function timestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) ||
    !Number.isFinite(Date.parse(value))
  ) {
    return false;
  }
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
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

export function parseAutomationRun(value: unknown): AutomationRunRecord | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "run_id",
      "request_id",
      "agent_code",
      "run_status",
      "trigger_source",
      "customer_id",
      "tag_id",
      "source_event_id",
      "triggered_event_id",
      "started_at",
      "completed_at",
      "has_error",
    ]) ||
    typeof value.run_id !== "string" ||
    !/^automation-trigger:[1-9]\d*$/.test(value.run_id) ||
    typeof value.request_id !== "string" ||
    !/^event:[1-9]\d*$/.test(value.request_id) ||
    value.agent_code !== "tag-trigger-v1" ||
    value.run_status !== "completed" ||
    value.trigger_source !== "customer.tag_applied" ||
    !positive(value.customer_id) ||
    !positive(value.tag_id) ||
    !positive(value.source_event_id) ||
    !positive(value.triggered_event_id) ||
    !timestamp(value.started_at) ||
    !timestamp(value.completed_at) ||
    Date.parse(value.completed_at) < Date.parse(value.started_at) ||
    value.has_error !== false
  ) {
    return undefined;
  }

  return {
    runID: value.run_id,
    requestID: value.request_id,
    agentCode: value.agent_code,
    runStatus: value.run_status,
    triggerSource: value.trigger_source,
    customerID: value.customer_id,
    tagID: value.tag_id,
    sourceEventID: value.source_event_id,
    triggeredEventID: value.triggered_event_id,
    startedAt: value.started_at,
    completedAt: value.completed_at,
    hasError: false,
  };
}

export function parseAutomationRunsPage(
  value: unknown,
  expectedPage: number,
): AutomationRunsPage | undefined {
  if (
    !positive(expectedPage) ||
    expectedPage > 10_000 ||
    !record(value) ||
    !exact(value, ["items", "total", "page", "page_size", "visibility"]) ||
    !Array.isArray(value.items) ||
    !nonnegative(value.total) ||
    value.page !== expectedPage ||
    !positive(value.page_size) ||
    value.page_size !== AUTOMATION_RUNS_PAGE_SIZE ||
    value.visibility !== "masked" ||
    value.items.length > value.page_size
  ) {
    return undefined;
  }
  const items = value.items.map(parseAutomationRun);
  if (
    items.some((item) => item === undefined) ||
    value.total < items.length ||
    new Set(items.map((item) => item?.runID)).size !== items.length
  ) {
    return undefined;
  }
  return {
    items: items as AutomationRunRecord[],
    total: value.total,
    page: value.page,
    pageSize: value.page_size,
  };
}

function failure(status: number): AutomationRunsFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 400) return "invalid";
  return "unavailable";
}

export async function loadAutomationRuns(
  transport: AutomationRunsTransport,
  page = 1,
): Promise<AutomationRunsResult> {
  if (!positive(page) || page > 10_000) return { status: "invalid" };
  try {
    const response = await transport.list(
      { page, page_size: AUTOMATION_RUNS_PAGE_SIZE, visibility: "masked" },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const parsed = parseAutomationRunsPage(response.data, page);
    return parsed ? { status: "loaded", page: parsed } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export function previousAutomationRunsPage(
  page: AutomationRunsPage,
): number | undefined {
  return page.page > 1 ? page.page - 1 : undefined;
}

export function nextAutomationRunsPage(
  page: AutomationRunsPage,
): number | undefined {
  return page.page < 10_000 && page.page * page.pageSize < page.total
    ? page.page + 1
    : undefined;
}
