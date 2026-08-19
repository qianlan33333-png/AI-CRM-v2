import {
  getLegacyInternalEvent,
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

export type AutomationSourceEventConsumer =
  | "automation.tag-trigger.v1"
  | "stats.tag-applied.v1";
export type AutomationSourceEventDeliveryStatus =
  | "pending"
  | "processing"
  | "completed"
  | "final_failed"
  | "outcome_unknown";

export interface AutomationSourceEventDelivery {
  readonly consumer: AutomationSourceEventConsumer;
  readonly status: AutomationSourceEventDeliveryStatus;
  readonly attemptCount: number;
  readonly completedAt: string | null;
}

export interface AutomationSourceEvent {
  readonly eventID: number;
  readonly eventType: "customer.tag_applied";
  readonly occurredAt: string;
  readonly dispatched: boolean;
  readonly deliveries: readonly AutomationSourceEventDelivery[];
  readonly observedAt: string;
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

async function generatedSourceEvent(
  eventID: number,
  options: RequestInit,
): Promise<AutomationRunsTransportResponse> {
  return getLegacyInternalEvent(String(eventID), options);
}

export interface AutomationRunsTransport {
  readonly list: typeof generatedList;
  // Optional only to preserve existing injected list-only test transports;
  // production always supplies the generated same-origin reader below.
  readonly sourceEvent?: typeof generatedSourceEvent;
}

export const generatedAutomationRunsTransport: AutomationRunsTransport = {
  list: generatedList,
  sourceEvent: generatedSourceEvent,
};

export type AutomationRunsFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type AutomationRunsResult =
  | { readonly status: "loaded"; readonly page: AutomationRunsPage }
  | { readonly status: AutomationRunsFailure };

export type AutomationSourceEventFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "invalid"
  | "unavailable";

export type AutomationSourceEventResult =
  | { readonly status: "loaded"; readonly sourceEvent: AutomationSourceEvent }
  | { readonly status: AutomationSourceEventFailure };

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

const SOURCE_EVENT_CONSUMERS: ReadonlySet<AutomationSourceEventConsumer> =
  new Set(["automation.tag-trigger.v1", "stats.tag-applied.v1"]);
const SOURCE_EVENT_DELIVERY_STATUSES: ReadonlySet<AutomationSourceEventDeliveryStatus> =
  new Set([
    "pending",
    "processing",
    "completed",
    "final_failed",
    "outcome_unknown",
  ]);
const MAX_INT32 = 2_147_483_647;

function terminalSourceEventDelivery(
  status: AutomationSourceEventDeliveryStatus,
): boolean {
  return (
    status === "completed" ||
    status === "final_failed" ||
    status === "outcome_unknown"
  );
}

function parseAutomationSourceEventDelivery(
  value: unknown,
): AutomationSourceEventDelivery | undefined {
  if (
    !record(value) ||
    !exact(value, ["consumer", "status", "attempt_count", "completed_at"]) ||
    typeof value.consumer !== "string" ||
    !SOURCE_EVENT_CONSUMERS.has(value.consumer as AutomationSourceEventConsumer) ||
    typeof value.status !== "string" ||
    !SOURCE_EVENT_DELIVERY_STATUSES.has(
      value.status as AutomationSourceEventDeliveryStatus,
    ) ||
    !nonnegative(value.attempt_count) ||
    value.attempt_count > MAX_INT32 ||
    (terminalSourceEventDelivery(
      value.status as AutomationSourceEventDeliveryStatus,
    )
      ? !timestamp(value.completed_at)
      : value.completed_at !== null)
  ) {
    return undefined;
  }
  return {
    consumer: value.consumer as AutomationSourceEventConsumer,
    status: value.status as AutomationSourceEventDeliveryStatus,
    attemptCount: value.attempt_count,
    completedAt: terminalSourceEventDelivery(
      value.status as AutomationSourceEventDeliveryStatus,
    )
      ? (value.completed_at as string)
      : null,
  };
}

export function parseAutomationSourceEvent(
  value: unknown,
  expectedEventID: number,
): AutomationSourceEvent | undefined {
  if (
    !positive(expectedEventID) ||
    !record(value) ||
    !exact(value, [
      "ok",
      "item",
      "observed_at",
      "registry_id",
      "source_status",
      "delivery_observation_available",
      "external_delivery",
      "route_owner",
      "real_external_call_executed",
    ]) ||
    value.ok !== true ||
    !record(value.item) ||
    !exact(value.item, [
      "event_id",
      "event_type",
      "occurred_at",
      "dispatched",
      "deliveries",
    ]) ||
    value.item.event_id !== expectedEventID ||
    value.item.event_type !== "customer.tag_applied" ||
    !timestamp(value.item.occurred_at) ||
    typeof value.item.dispatched !== "boolean" ||
    !Array.isArray(value.item.deliveries) ||
    value.item.deliveries.length > SOURCE_EVENT_CONSUMERS.size ||
    !timestamp(value.observed_at) ||
    value.registry_id !== "v2-internal-events.v1" ||
    value.source_status !== "local_read_model" ||
    value.delivery_observation_available !== true ||
    value.external_delivery !== "unknown" ||
    value.route_owner !== "ai_crm_next" ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const deliveries = value.item.deliveries.map(parseAutomationSourceEventDelivery);
  if (
    deliveries.includes(undefined) ||
    new Set(
      (deliveries as readonly AutomationSourceEventDelivery[]).map(
        (delivery) => delivery.consumer,
      ),
    ).size !== deliveries.length
  ) {
    return undefined;
  }
  return {
    eventID: expectedEventID,
    eventType: "customer.tag_applied",
    occurredAt: value.item.occurred_at,
    dispatched: value.item.dispatched,
    deliveries: deliveries as readonly AutomationSourceEventDelivery[],
    observedAt: value.observed_at,
  };
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

function sourceEventFailure(status: number): AutomationSourceEventFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
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

export async function loadAutomationSourceEvent(
  transport: AutomationRunsTransport,
  eventID: number,
): Promise<AutomationSourceEventResult> {
  if (!positive(eventID)) return { status: "invalid" };
  if (!transport.sourceEvent) return { status: "unavailable" };
  try {
    const response = await transport.sourceEvent(eventID, {
      credentials: "same-origin",
    });
    if (response.status !== 200) {
      return { status: sourceEventFailure(response.status) };
    }
    const sourceEvent = parseAutomationSourceEvent(response.data, eventID);
    return sourceEvent
      ? { status: "loaded", sourceEvent }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

// React does not commit a disabled button between two synchronous clicks. The
// ref gate prevents a second local GET while the first source-event read is
// still pending; it never retries or mutates state on its own.
export function startAutomationSourceEventRead(
  lock: { current: boolean },
  execute: () => Promise<void>,
): Promise<void> | undefined {
  if (lock.current) return undefined;
  lock.current = true;
  return (async () => {
    try {
      await execute();
    } finally {
      lock.current = false;
    }
  })();
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
