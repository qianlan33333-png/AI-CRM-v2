import {
  getLegacyDeliveryLineage,
  type GetLegacyDeliveryLineageParams,
} from "./api/generated/health";

export type DeliveryLineageRole = "admin" | "ops" | "sales";
export const DELIVERY_LINEAGE_PAGE_SIZE = 50;
const MAXIMUM_OFFSET = 1_000_000;
const MAXIMUM_INT64 = "9223372036854775807";
const OUTBOUND_STATES = new Set([
  "pending",
  "sending",
  "sent",
  "retryable_failed",
  "final_failed",
  "outcome_unknown",
  "cancelled",
]);
const EVENT_STATES = new Set([
  "pending",
  "processing",
  "completed",
  "final_failed",
  "outcome_unknown",
]);

export interface DeliveryLineageRecord {
  readonly lineageID: string;
  readonly recordKind: "outbound_task" | "event_delivery";
  readonly internalState:
    | "pending"
    | "sending"
    | "sent"
    | "retryable_failed"
    | "final_failed"
    | "outcome_unknown"
    | "cancelled"
    | "processing"
    | "completed";
  readonly attemptCount: number;
  readonly updatedAt: string;
}

export interface DeliveryLineagePageData {
  readonly items: readonly DeliveryLineageRecord[];
  readonly limit: number;
  readonly offset: number;
  readonly hasMore: boolean;
}

export interface DeliveryLineageTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  params: GetLegacyDeliveryLineageParams,
  options: RequestInit,
): Promise<DeliveryLineageTransportResponse> {
  return getLegacyDeliveryLineage(params, options);
}

export interface DeliveryLineageTransport {
  readonly list: typeof generatedList;
}

export const generatedDeliveryLineageTransport: DeliveryLineageTransport = {
  list: generatedList,
};

export type DeliveryLineageFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type DeliveryLineageResult =
  | { readonly status: "loaded"; readonly page: DeliveryLineagePageData }
  | { readonly status: DeliveryLineageFailure };

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

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function canonicalPositiveInt64(value: string): boolean {
  return (
    /^[1-9]\d*$/.test(value) &&
    (value.length < MAXIMUM_INT64.length ||
      (value.length === MAXIMUM_INT64.length && value <= MAXIMUM_INT64))
  );
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

export function parseDeliveryLineageRecord(
  value: unknown,
): DeliveryLineageRecord | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "lineage_id",
      "record_kind",
      "internal_state",
      "attempt_count",
      "updated_at",
      "external_delivery",
      "external_receipt",
    ]) ||
    typeof value.lineage_id !== "string" ||
    (value.record_kind !== "outbound_task" && value.record_kind !== "event_delivery") ||
    typeof value.internal_state !== "string" ||
    (value.record_kind === "outbound_task" &&
      (!value.lineage_id.startsWith("outbound-task:") ||
        !canonicalPositiveInt64(value.lineage_id.slice("outbound-task:".length)) ||
        !OUTBOUND_STATES.has(value.internal_state))) ||
    (value.record_kind === "event_delivery" &&
      (!/^event-delivery:v1:[0-9a-f]{64}$/.test(value.lineage_id) ||
        !EVENT_STATES.has(value.internal_state))) ||
    !nonnegative(value.attempt_count) ||
    !timestamp(value.updated_at) ||
    value.external_delivery !== "unknown" ||
    value.external_receipt !== "unknown"
  ) {
    return undefined;
  }
  return {
    lineageID: value.lineage_id,
    recordKind: value.record_kind,
    internalState: value.internal_state as DeliveryLineageRecord["internalState"],
    attemptCount: value.attempt_count,
    updatedAt: value.updated_at,
  };
}

export function parseDeliveryLineagePage(
  value: unknown,
  expectedOffset: number,
): DeliveryLineagePageData | undefined {
  if (
    !nonnegative(expectedOffset) ||
    expectedOffset > MAXIMUM_OFFSET ||
    expectedOffset % DELIVERY_LINEAGE_PAGE_SIZE !== 0 ||
    !record(value) ||
    !exact(value, ["ok", "items", "limit", "offset", "has_more", "interpretation"]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    value.limit !== DELIVERY_LINEAGE_PAGE_SIZE ||
    value.offset !== expectedOffset ||
    typeof value.has_more !== "boolean" ||
    value.items.length > DELIVERY_LINEAGE_PAGE_SIZE ||
    (value.has_more && value.items.length !== DELIVERY_LINEAGE_PAGE_SIZE) ||
    !record(value.interpretation) ||
    !exact(value.interpretation, ["kind", "external_delivery", "external_receipt"]) ||
    value.interpretation.kind !== "internal_processing_only" ||
    value.interpretation.external_delivery !== "unknown" ||
    value.interpretation.external_receipt !== "unknown"
  ) {
    return undefined;
  }
  const items = value.items.map(parseDeliveryLineageRecord);
  if (
    items.includes(undefined) ||
    new Set((items as readonly DeliveryLineageRecord[]).map((item) => item.lineageID))
      .size !== items.length
  ) {
    return undefined;
  }
  return {
    items: items as readonly DeliveryLineageRecord[],
    limit: DELIVERY_LINEAGE_PAGE_SIZE,
    offset: expectedOffset,
    hasMore: value.has_more,
  };
}

function failure(status: number): DeliveryLineageFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 422) return "invalid";
  return "unavailable";
}

export async function loadDeliveryLineage(
  transport: DeliveryLineageTransport,
  offset = 0,
): Promise<DeliveryLineageResult> {
  if (
    !nonnegative(offset) ||
    offset > MAXIMUM_OFFSET ||
    offset % DELIVERY_LINEAGE_PAGE_SIZE !== 0
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.list(
      { limit: DELIVERY_LINEAGE_PAGE_SIZE, offset },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseDeliveryLineagePage(response.data, offset);
    return page ? { status: "loaded", page } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export function previousDeliveryLineagePage(
  page: DeliveryLineagePageData,
): number | undefined {
  return page.offset >= page.limit ? page.offset - page.limit : undefined;
}

export function nextDeliveryLineagePage(
  page: DeliveryLineagePageData,
): number | undefined {
  return page.hasMore && page.offset + page.limit <= MAXIMUM_OFFSET
    ? page.offset + page.limit
    : undefined;
}
