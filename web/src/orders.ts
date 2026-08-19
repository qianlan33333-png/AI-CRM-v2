import {
  listLegacyOrders,
  type ListLegacyOrdersParams,
} from "./api/generated/health";

export type OrdersRole = "admin" | "ops" | "sales";
export const ORDER_PAGE_SIZE = 50;
const MAXIMUM_OFFSET = 1_000_000;

export interface OrderListItem {
  readonly orderNo: string;
  readonly payerName: string;
  readonly mobile: string;
  readonly productCode: string;
  readonly productName: string;
  readonly amountYuan: string;
  readonly currency: string;
  readonly statusLabel: string;
  readonly providerLabel: string;
  readonly createdAt: string;
}

export interface OrderListPage {
  readonly items: readonly OrderListItem[];
  readonly total: number;
  readonly offset: number;
  readonly hasMore: boolean;
}

export interface OrdersTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  params: ListLegacyOrdersParams,
  options: RequestInit,
): Promise<OrdersTransportResponse> {
  return listLegacyOrders(params, options);
}

export interface OrdersTransport {
  readonly list: typeof generatedList;
}

export const generatedOrdersTransport: OrdersTransport = { list: generatedList };

export type OrdersFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";
export type OrdersResult =
  | { readonly status: "loaded"; readonly page: OrderListPage }
  | { readonly status: OrdersFailure };

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

function text(value: unknown, maximum: number, nonempty = false): value is string {
  return (
    typeof value === "string" &&
    (nonempty ? value.length > 0 : true) &&
    value === value.trim() &&
    [...value].length <= maximum &&
    !value.includes("\x00")
  );
}

function timestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) ||
    !Number.isFinite(Date.parse(value))
  ) return false;
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, 0);
  return (
    date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day && date.getUTCHours() === hour &&
    date.getUTCMinutes() === minute && date.getUTCSeconds() === second
  );
}

function safeDetailPath(value: unknown): value is string {
  return (
    text(value, 2048, true) &&
    value.startsWith("/") &&
    !value.startsWith("//") &&
    !/[\t\r\n]/.test(value)
  );
}

const REQUIRED_ITEM_KEYS = [
  "created_at", "merchant_order_no", "out_trade_no", "order_no",
  "platform_transaction_no", "transaction_id", "payer_name", "mobile",
  "product_code", "product_name", "amount_yuan", "currency", "status",
  "status_label", "provider", "provider_label", "detail_url",
] as const;
const OPTIONAL_IDENTITY_KEYS = ["userid", "external_userid", "unionid"] as const;
const ITEM_KEYS: readonly string[] = [...REQUIRED_ITEM_KEYS, ...OPTIONAL_IDENTITY_KEYS];

function parseOrderItem(value: unknown): OrderListItem | undefined {
  if (!record(value) || !Object.keys(value).every((key) => ITEM_KEYS.includes(key)) || !REQUIRED_ITEM_KEYS.every((key) => key in value)) return undefined;
  const identityKeys = OPTIONAL_IDENTITY_KEYS.filter((key) => key in value);
  if (
    identityKeys.length > 1 ||
    identityKeys.some((key) => !text(value[key], 200, true)) ||
    !timestamp(value.created_at) ||
    !text(value.merchant_order_no, 200, true) ||
    !text(value.out_trade_no, 200, true) ||
    !text(value.order_no, 200, true) ||
    !text(value.platform_transaction_no, 200) ||
    !text(value.transaction_id, 200) ||
    !text(value.payer_name, 200) ||
    !text(value.mobile, 80) ||
    !text(value.product_code, 200, true) ||
    !text(value.product_name, 200) ||
    typeof value.amount_yuan !== "string" ||
    !/^(?:0|[1-9]\d*)\.\d{2}$/.test(value.amount_yuan) ||
    value.amount_yuan.length > 20 ||
    typeof value.currency !== "string" || !/^[A-Z]{3}$/.test(value.currency) ||
    !text(value.status, 80, true) ||
    !text(value.status_label, 80, true) ||
    (value.provider !== "wechat" && value.provider !== "alipay" && value.provider !== "wechat_shop") ||
    !text(value.provider_label, 80, true) ||
    !safeDetailPath(value.detail_url)
  ) return undefined;
  return {
    orderNo: value.order_no,
    payerName: value.payer_name,
    mobile: value.mobile,
    productCode: value.product_code,
    productName: value.product_name,
    amountYuan: value.amount_yuan,
    currency: value.currency,
    statusLabel: value.status_label,
    providerLabel: value.provider_label,
    createdAt: value.created_at,
  };
}

function parseOrderPage(value: unknown, offset: number): OrderListPage | undefined {
  if (
    !record(value) || !exact(value, ["items", "total", "limit", "has_more"]) ||
    !Array.isArray(value.items) || !nonnegative(value.total) ||
    value.limit !== ORDER_PAGE_SIZE || typeof value.has_more !== "boolean" ||
    value.items.length > ORDER_PAGE_SIZE || value.total < offset + value.items.length ||
    (value.items.length === 0 && value.has_more) ||
    value.has_more !== (offset + value.items.length < value.total)
  ) return undefined;
  const items = value.items.map(parseOrderItem);
  if (items.some((item) => item === undefined)) return undefined;
  const parsed = items as OrderListItem[];
  if (new Set(parsed.map((item) => item.orderNo)).size !== parsed.length) return undefined;
  return { items: parsed, total: value.total, offset, hasMore: value.has_more };
}

function failure(status: number): OrdersFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 400 || status === 404 || status === 409) return "invalid";
  return "unavailable";
}

export async function loadOrders(
  transport: OrdersTransport,
  offset = 0,
): Promise<OrdersResult> {
  if (!nonnegative(offset) || offset > MAXIMUM_OFFSET || offset % ORDER_PAGE_SIZE !== 0) return { status: "invalid" };
  try {
    const response = await transport.list(
      { provider: "all", limit: ORDER_PAGE_SIZE, offset },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseOrderPage(response.data, offset);
    return page ? { status: "loaded", page } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export function previousOrderOffset(page: OrderListPage): number | undefined {
  return page.offset >= ORDER_PAGE_SIZE ? page.offset - ORDER_PAGE_SIZE : undefined;
}

export function nextOrderOffset(page: OrderListPage): number | undefined {
  const next = page.offset + ORDER_PAGE_SIZE;
  return page.hasMore && next <= MAXIMUM_OFFSET ? next : undefined;
}
