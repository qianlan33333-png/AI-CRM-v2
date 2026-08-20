/* eslint-disable no-unused-vars -- transport callback parameter names document the generated adapter contract. */
import {
  listCustomerMergeHistory,
  type ListCustomerMergeHistoryParams,
} from "./api/generated/health";
export const CUSTOMER_MERGE_HISTORY_PAGE_SIZE = 50;

export type CustomerMergeHistoryRole = "admin" | "ops" | "sales";

export interface CustomerMergeHistoryItem {
  readonly mergeAuditID: number;
  readonly primaryCustomerID: number;
  readonly mergedCustomerID: number;
  readonly mode: "auto" | "manual";
  readonly policyVersion: string;
  readonly mergedAt: string;
}

export interface CustomerMergeHistoryPageData {
  readonly customerID: number;
  readonly items: readonly CustomerMergeHistoryItem[];
  readonly nextCursor?: string;
}

export interface CustomerMergeHistoryTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

export interface CustomerMergeHistoryTransport {
  readonly get: (
    customerID: number,
    params: { readonly cursor?: string; readonly limit: number },
    options: RequestInit,
  ) => Promise<CustomerMergeHistoryTransportResponse>;
}

async function loadGeneratedCustomerMergeHistory(
  customerID: number,
  params: ListCustomerMergeHistoryParams,
  options: RequestInit,
): Promise<CustomerMergeHistoryTransportResponse> {
  const response = await listCustomerMergeHistory(customerID, params, options);
  return { status: response.status, data: response.data };
}

export const generatedCustomerMergeHistoryTransport: CustomerMergeHistoryTransport =
  {
    get: loadGeneratedCustomerMergeHistory,
  };

export type CustomerMergeHistoryLoadResult =
  | { readonly status: "loaded"; readonly page: CustomerMergeHistoryPageData }
  | {
      readonly status:
        "unauthenticated" | "forbidden" | "invalid" | "unavailable";
    };

const SAME_ORIGIN: RequestInit = { credentials: "same-origin" };
const RFC3339 =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

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

function safeText(value: unknown, maximum: number): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    value.length <= maximum &&
    value.trim() === value &&
    !/[\u0000-\u001f\u007f]/u.test(value)
  );
}

function timestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = RFC3339.exec(value);
  if (!match) return false;
  const [, year, month, day, hour, minute, second] = match.map(Number);
  const utc = new Date(Date.UTC(year, month - 1, day, hour, minute, second));
  return (
    Number.isFinite(Date.parse(value)) &&
    utc.getUTCFullYear() === year &&
    utc.getUTCMonth() === month - 1 &&
    utc.getUTCDate() === day &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59
  );
}

export function parseCustomerMergeHistoryPage(
  value: unknown,
  customerID: number,
): CustomerMergeHistoryPageData | undefined {
  const pageKeys = [
    "customer_id",
    "scope",
    "items",
    "next_cursor",
    "identity_values_included",
    "operator_identifiers_included",
    "chat_content_included",
    "real_external_call_executed",
  ];
  if (
    !positiveInteger(customerID) ||
    !exact(value, pageKeys) ||
    value.customer_id !== customerID ||
    value.scope !== "connected_component" ||
    !Array.isArray(value.items) ||
    value.items.length > CUSTOMER_MERGE_HISTORY_PAGE_SIZE ||
    !(value.next_cursor === null || safeText(value.next_cursor, 512)) ||
    value.identity_values_included !== false ||
    value.operator_identifiers_included !== false ||
    value.chat_content_included !== false ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const itemKeys = [
    "merge_audit_id",
    "primary_customer_id",
    "merged_customer_id",
    "mode",
    "policy_version",
    "merged_at",
  ];
  const items: CustomerMergeHistoryItem[] = [];
  const seen = new Set<number>();
  let previousID = Number.POSITIVE_INFINITY;
  for (const item of value.items) {
    if (
      !exact(item, itemKeys) ||
      !positiveInteger(item.merge_audit_id) ||
      !positiveInteger(item.primary_customer_id) ||
      !positiveInteger(item.merged_customer_id) ||
      item.primary_customer_id === item.merged_customer_id ||
      (item.mode !== "auto" && item.mode !== "manual") ||
      !safeText(item.policy_version, 200) ||
      !timestamp(item.merged_at) ||
      seen.has(item.merge_audit_id) ||
      item.merge_audit_id >= previousID
    ) {
      return undefined;
    }
    seen.add(item.merge_audit_id);
    previousID = item.merge_audit_id;
    items.push({
      mergeAuditID: item.merge_audit_id,
      primaryCustomerID: item.primary_customer_id,
      mergedCustomerID: item.merged_customer_id,
      mode: item.mode,
      policyVersion: item.policy_version,
      mergedAt: item.merged_at,
    });
  }
  if (
    value.next_cursor !== null &&
    items.length !== CUSTOMER_MERGE_HISTORY_PAGE_SIZE
  )
    return undefined;
  return {
    customerID,
    items,
    ...(typeof value.next_cursor === "string"
      ? { nextCursor: value.next_cursor }
      : {}),
  };
}

export async function loadCustomerMergeHistory(
  transport: CustomerMergeHistoryTransport,
  customerID: number,
  cursor?: string,
): Promise<CustomerMergeHistoryLoadResult> {
  if (
    !positiveInteger(customerID) ||
    (cursor !== undefined && !safeText(cursor, 512))
  )
    return { status: "invalid" };
  try {
    const response = await transport.get(
      customerID,
      {
        limit: CUSTOMER_MERGE_HISTORY_PAGE_SIZE,
        ...(cursor ? { cursor } : {}),
      },
      SAME_ORIGIN,
    );
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 400) return { status: "invalid" };
    if (response.status !== 200) return { status: "unavailable" };
    const page = parseCustomerMergeHistoryPage(response.data, customerID);
    return page ? { status: "loaded", page } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
