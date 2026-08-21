/* eslint-disable no-unused-vars -- transport names document the generated adapter contract. */
import {
  listCustomerChatActivity,
  type ListCustomerChatActivityParams,
} from "./api/generated/health";
import { isStrictRFC3339Timestamp } from "./customer-context";

export const CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT = 30;
export const CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT = 100;
// Retain the public name for existing consumers; the first safe summary page is
// now deliberately limited to the legacy 30-row recent-activity window.
export const CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE =
  CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT;

export type CustomerChatActivityRole = "admin" | "ops" | "sales";
export type CustomerChatActivityFilter = "all" | "private" | "group";

export interface CustomerChatActivityEntry {
  readonly chatType: "private" | "group";
  readonly messageType: string;
  readonly sentAt: string;
}

export interface CustomerChatActivityPage {
  readonly customerID: number;
  readonly chatType: CustomerChatActivityFilter;
  /** Request-bound pagination facts; neither value is trusted from response data. */
  readonly limit: number;
  readonly offset: number;
  readonly items: readonly CustomerChatActivityEntry[];
  readonly total: number;
  readonly nextCursor?: string;
  readonly previousCursor?: string;
}

export interface CustomerChatActivityTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

export interface CustomerChatActivityTransport {
  readonly get: (
    customerID: number,
    params: {
      readonly chat_type?: "private" | "group";
      readonly cursor?: string;
      readonly limit: number;
    },
    options: RequestInit,
  ) => Promise<CustomerChatActivityTransportResponse>;
}

async function loadGeneratedCustomerChatActivity(
  customerID: number,
  params: ListCustomerChatActivityParams,
  options: RequestInit,
): Promise<CustomerChatActivityTransportResponse> {
  const response = await listCustomerChatActivity(customerID, params, options);
  return { status: response.status, data: response.data };
}

export const generatedCustomerChatActivityTransport: CustomerChatActivityTransport =
  {
    get: loadGeneratedCustomerChatActivity,
  };

export type CustomerChatActivityLoadResult =
  | { readonly status: "loaded"; readonly page: CustomerChatActivityPage }
  | {
      readonly status:
        | "unauthenticated"
        | "forbidden"
        | "not_found"
        | "invalid"
        | "unavailable";
    };

const SAME_ORIGIN: RequestInit = { credentials: "same-origin" };

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function exact(
  value: unknown,
  keys: readonly string[],
): value is Record<string, unknown> {
  return (
    record(value) &&
    Object.keys(value).length === keys.length &&
    keys.every((key) => Object.hasOwn(value, key))
  );
}

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function validLimit(value: unknown): value is number {
  return positiveInteger(value) && value <= CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT;
}

function nonnegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function safeText(value: unknown, maximum: number): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    [...value].length <= maximum &&
    value.trim() === value &&
    !/[\u0000-\u001f\u007f]/u.test(value)
  );
}

export function parseCustomerChatActivityPage(
  value: unknown,
  customerID: number,
  filter: CustomerChatActivityFilter,
  limit = CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
  offset = 0,
): CustomerChatActivityPage | undefined {
  const keys = [
    "customer_id",
    "chat_type",
    "items",
    "total",
    "next_cursor",
    "previous_cursor",
    "non_atomic_snapshot",
    "message_content_included",
    "identity_values_included",
    "provider_receipts_included",
    "real_external_call_executed",
  ];
  if (
    !positiveInteger(customerID) ||
    !validLimit(limit) ||
    !nonnegativeInteger(offset) ||
    !exact(value, keys) ||
    value.customer_id !== customerID ||
    value.chat_type !== filter ||
    !Array.isArray(value.items) ||
    value.items.length > limit ||
    !nonnegativeInteger(value.total) ||
    value.total < value.items.length ||
    !(value.next_cursor === null || safeText(value.next_cursor, 512)) ||
    !(value.previous_cursor === null || safeText(value.previous_cursor, 512)) ||
    value.non_atomic_snapshot !== true ||
    value.message_content_included !== false ||
    value.identity_values_included !== false ||
    value.provider_receipts_included !== false ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const consumed = offset + value.items.length;
  const hasMore = consumed < value.total;
  if (
    !Number.isSafeInteger(consumed) ||
    value.total < consumed ||
    (offset === 0
      ? value.previous_cursor !== null
      : value.previous_cursor === null) ||
    (hasMore ? value.next_cursor === null : value.next_cursor !== null) ||
    (hasMore && value.items.length !== limit) ||
    (offset < value.total && value.items.length === 0)
  ) {
    return undefined;
  }
  const items: CustomerChatActivityEntry[] = [];
  let previousTime = Number.POSITIVE_INFINITY;
  for (const item of value.items) {
    if (
      !exact(item, ["chat_type", "message_type", "sent_at"]) ||
      (item.chat_type !== "private" && item.chat_type !== "group") ||
      (filter !== "all" && item.chat_type !== filter) ||
      !safeText(item.message_type, 128) ||
      !isStrictRFC3339Timestamp(item.sent_at)
    ) {
      return undefined;
    }
    const sentAt = Date.parse(item.sent_at);
    if (sentAt > previousTime) return undefined;
    previousTime = sentAt;
    items.push({
      chatType: item.chat_type,
      messageType: item.message_type,
      sentAt: item.sent_at,
    });
  }
  return {
    customerID,
    chatType: filter,
    limit,
    offset,
    items,
    total: value.total,
    ...(typeof value.next_cursor === "string"
      ? { nextCursor: value.next_cursor }
      : {}),
    ...(typeof value.previous_cursor === "string"
      ? { previousCursor: value.previous_cursor }
      : {}),
  };
}

export function canLoadNextCustomerChatActivityPage(
  page: CustomerChatActivityPage,
): boolean {
  if (
    !validLimit(page.limit) ||
    !nonnegativeInteger(page.offset) ||
    page.nextCursor === undefined
  ) {
    return false;
  }
  const nextEnd = page.offset + page.limit * 2;
  return (
    Number.isSafeInteger(nextEnd) &&
    nextEnd <= CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT
  );
}

export async function loadCustomerChatActivity(
  transport: CustomerChatActivityTransport,
  customerID: number,
  filter: CustomerChatActivityFilter,
  cursor?: string,
  limit = CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
  offset = 0,
): Promise<CustomerChatActivityLoadResult> {
  if (
    !positiveInteger(customerID) ||
    !["all", "private", "group"].includes(filter) ||
    !validLimit(limit) ||
    !nonnegativeInteger(offset) ||
    (offset > 0 && cursor === undefined) ||
    (cursor !== undefined && !safeText(cursor, 512))
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.get(
      customerID,
      {
        limit,
        ...(filter === "all" ? {} : { chat_type: filter }),
        ...(cursor ? { cursor } : {}),
      },
      SAME_ORIGIN,
    );
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 404) return { status: "not_found" };
    if (response.status === 400) return { status: "invalid" };
    if (response.status !== 200) return { status: "unavailable" };
    const page = parseCustomerChatActivityPage(
      response.data,
      customerID,
      filter,
      limit,
      offset,
    );
    return page ? { status: "loaded", page } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
