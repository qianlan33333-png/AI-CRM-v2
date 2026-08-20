/* eslint-disable no-unused-vars -- transport names document the generated adapter contract. */
import { isStrictRFC3339Timestamp } from "./customer-context";
export const CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE = 50;

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
    !exact(value, keys) ||
    value.customer_id !== customerID ||
    value.chat_type !== filter ||
    !Array.isArray(value.items) ||
    value.items.length > CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE ||
    !nonnegativeInteger(value.total) ||
    value.total < value.items.length ||
    !(value.next_cursor === null || safeText(value.next_cursor, 512)) ||
    !(value.previous_cursor === null || safeText(value.previous_cursor, 512)) ||
    value.non_atomic_snapshot !== true ||
    value.message_content_included !== false ||
    value.identity_values_included !== false ||
    value.provider_receipts_included !== false ||
    value.real_external_call_executed !== false ||
    (value.next_cursor !== null &&
      value.items.length !== CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE)
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

export async function loadCustomerChatActivity(
  transport: CustomerChatActivityTransport,
  customerID: number,
  filter: CustomerChatActivityFilter,
  cursor?: string,
): Promise<CustomerChatActivityLoadResult> {
  if (
    !positiveInteger(customerID) ||
    !["all", "private", "group"].includes(filter) ||
    (cursor !== undefined && !safeText(cursor, 512))
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.get(
      customerID,
      {
        limit: CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE,
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
    );
    return page ? { status: "loaded", page } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
