import {
  getLegacyInternalEvent,
  listLegacyInternalEvents,
  type ListLegacyInternalEventsParams,
} from "./api/generated/health";

export type WeComCallbackRole = "admin" | "ops" | "sales";
export type CallbackDisposition = "accepted" | "rejected";
export const CALLBACK_INBOX_PAGE_SIZE = 50;

const callbackEventTypes: Record<CallbackDisposition, string> = {
  accepted: "wecom.callback.accepted",
  rejected: "wecom.callback.rejected",
};
const deliveryConsumers = new Set([
  "automation.tag-trigger.v1",
  "stats.tag-applied.v1",
  "operation-cycle.fact.v1",
]);
const deliveryStatuses = new Set([
  "pending",
  "processing",
  "completed",
  "final_failed",
  "outcome_unknown",
]);

export interface CallbackAuditItem {
  readonly eventID: number;
  readonly disposition: CallbackDisposition;
  readonly occurredAt: string;
  readonly dispatched: boolean;
}

export interface CallbackAuditPage {
  readonly items: readonly CallbackAuditItem[];
  readonly total: number;
  readonly offset: number;
  readonly observedAt: string;
}

interface CallbackInboxResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  params: ListLegacyInternalEventsParams,
  options: RequestInit,
): Promise<CallbackInboxResponse> {
  return listLegacyInternalEvents(params, options);
}

async function generatedDetail(
  eventID: string,
  options: RequestInit,
): Promise<CallbackInboxResponse> {
  return getLegacyInternalEvent(eventID, options);
}

export interface CallbackInboxTransport {
  readonly list: typeof generatedList;
  readonly detail: typeof generatedDetail;
}

export const generatedCallbackInboxTransport: CallbackInboxTransport = {
  list: generatedList,
  detail: generatedDetail,
};

export type CallbackInboxFailure =
  "unauthenticated" | "forbidden" | "invalid" | "unavailable";

export type CallbackInboxResult =
  | { readonly status: "loaded"; readonly page: CallbackAuditPage }
  | { readonly status: CallbackInboxFailure };

function record(value: unknown): value is Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
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

function safeNonnegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function positiveID(value: unknown): value is number {
  return safeNonnegativeInteger(value) && value > 0;
}

function timestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/.test(value)
  )
    return false;
  const match = /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)/.exec(value);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
  if (hour > 23 || minute > 59 || second > 59) return false;
  const calendar = new Date(0);
  calendar.setUTCFullYear(year, month - 1, day);
  calendar.setUTCHours(hour, minute, second, 0);
  return (
    calendar.getUTCFullYear() === year &&
    calendar.getUTCMonth() === month - 1 &&
    calendar.getUTCDate() === day &&
    Number.isFinite(Date.parse(value))
  );
}

function eventDisposition(value: unknown): CallbackDisposition | undefined {
  if (value === callbackEventTypes.accepted) return "accepted";
  if (value === callbackEventTypes.rejected) return "rejected";
  return undefined;
}

function parseDeliveries(value: unknown): boolean {
  if (!Array.isArray(value)) return false;
  return value.every(
    (delivery) =>
      record(delivery) &&
      exact(delivery, [
        "consumer",
        "status",
        "attempt_count",
        "completed_at",
      ]) &&
      typeof delivery.consumer === "string" &&
      deliveryConsumers.has(delivery.consumer) &&
      typeof delivery.status === "string" &&
      deliveryStatuses.has(delivery.status) &&
      safeNonnegativeInteger(delivery.attempt_count) &&
      delivery.attempt_count <= 2_147_483_647 &&
      (delivery.completed_at === null || timestamp(delivery.completed_at)),
  );
}

function parseItem(
  value: unknown,
  expected: CallbackDisposition,
  eventID?: number,
): CallbackAuditItem | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "event_id",
      "event_type",
      "occurred_at",
      "dispatched",
      "deliveries",
    ]) ||
    !positiveID(value.event_id) ||
    (eventID !== undefined && value.event_id !== eventID) ||
    eventDisposition(value.event_type) !== expected ||
    !timestamp(value.occurred_at) ||
    typeof value.dispatched !== "boolean" ||
    !parseDeliveries(value.deliveries)
  )
    return undefined;
  return {
    eventID: value.event_id,
    disposition: expected,
    occurredAt: value.occurred_at,
    dispatched: value.dispatched,
  };
}

function validEnvelope(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  return (
    exact(value, keys) &&
    value.ok === true &&
    timestamp(value.observed_at) &&
    value.registry_id === "v2-internal-events.v1" &&
    value.source_status === "local_read_model" &&
    value.delivery_observation_available === true &&
    value.external_delivery === "unknown" &&
    value.route_owner === "ai_crm_next" &&
    value.real_external_call_executed === false
  );
}

export function parseCallbackAuditPage(
  value: unknown,
  expected: CallbackDisposition,
  offset: number,
): CallbackAuditPage | undefined {
  if (
    !record(value) ||
    !validEnvelope(value, [
      "ok",
      "items",
      "total",
      "limit",
      "offset",
      "observed_at",
      "registry_id",
      "source_status",
      "delivery_observation_available",
      "external_delivery",
      "route_owner",
      "real_external_call_executed",
    ]) ||
    !Array.isArray(value.items) ||
    !safeNonnegativeInteger(value.total) ||
    value.limit !== CALLBACK_INBOX_PAGE_SIZE ||
    value.offset !== offset ||
    value.items.length > CALLBACK_INBOX_PAGE_SIZE
  )
    return undefined;
  if (
    value.items.length === 0
      ? offset < value.total
      : value.total < offset + value.items.length ||
        (offset + value.items.length < value.total &&
          value.items.length !== CALLBACK_INBOX_PAGE_SIZE)
  )
    return undefined;
  const items = value.items.map((item) => parseItem(item, expected));
  if (items.includes(undefined)) return undefined;
  const safeItems = items as readonly CallbackAuditItem[];
  if (new Set(safeItems.map((item) => item.eventID)).size !== safeItems.length)
    return undefined;
  return {
    items: safeItems,
    total: value.total,
    offset,
    observedAt: value.observed_at as string,
  };
}

export function parseCallbackAuditDetail(
  value: unknown,
  expected: CallbackDisposition,
  eventID: number,
): CallbackAuditItem | undefined {
  if (
    !record(value) ||
    !validEnvelope(value, [
      "ok",
      "item",
      "observed_at",
      "registry_id",
      "source_status",
      "delivery_observation_available",
      "external_delivery",
      "route_owner",
      "real_external_call_executed",
    ])
  )
    return undefined;
  return parseItem(value.item, expected, eventID);
}

function failureForStatus(status: number): CallbackInboxFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 400 || status === 404 || status === 422) return "invalid";
  return "unavailable";
}

export async function loadCallbackAudit(
  transport: CallbackInboxTransport,
  disposition: CallbackDisposition,
  offset = 0,
): Promise<CallbackInboxResult> {
  if (
    !safeNonnegativeInteger(offset) ||
    offset > 100_000 ||
    offset % CALLBACK_INBOX_PAGE_SIZE !== 0
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.list(
      {
        event_type: callbackEventTypes[disposition],
        limit: "50",
        offset: String(offset),
      },
      { credentials: "same-origin" },
    );
    if (response.status !== 200)
      return { status: failureForStatus(response.status) };
    const page = parseCallbackAuditPage(response.data, disposition, offset);
    return page ? { status: "loaded", page } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadCallbackAuditDetail(
  transport: CallbackInboxTransport,
  disposition: CallbackDisposition,
  eventID: number,
): Promise<
  | { readonly status: "loaded"; readonly item: CallbackAuditItem }
  | { readonly status: CallbackInboxFailure }
> {
  if (!positiveID(eventID)) return { status: "invalid" };
  try {
    const response = await transport.detail(String(eventID), {
      credentials: "same-origin",
    });
    if (response.status !== 200)
      return { status: failureForStatus(response.status) };
    const item = parseCallbackAuditDetail(response.data, disposition, eventID);
    return item ? { status: "loaded", item } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export function previousCallbackAuditOffset(
  page: CallbackAuditPage,
): number | undefined {
  return page.offset >= CALLBACK_INBOX_PAGE_SIZE
    ? page.offset - CALLBACK_INBOX_PAGE_SIZE
    : undefined;
}

export function nextCallbackAuditOffset(
  page: CallbackAuditPage,
): number | undefined {
  return page.offset + CALLBACK_INBOX_PAGE_SIZE < page.total
    ? page.offset + CALLBACK_INBOX_PAGE_SIZE
    : undefined;
}
