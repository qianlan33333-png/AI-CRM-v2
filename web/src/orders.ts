import {
  getLegacyOrder,
  listLegacyOrders,
  listLegacyRefunds,
  type GetLegacyOrderParams,
  type ListLegacyOrdersParams,
  type ListLegacyRefundsParams,
} from "./api/generated/health";

export type OrdersRole = "admin" | "ops" | "sales";
export const ORDER_PAGE_SIZE = 50;
const MAXIMUM_OFFSET = 1_000_000;
export type OrderProvider = "wechat" | "alipay" | "wechat_shop";
export type OrderIdentityKind = "userid" | "external_userid" | "unionid";

export interface OrderListItem {
  readonly orderNo: string;
  readonly merchantOrderNo: string;
  readonly outTradeNo: string;
  readonly platformTransactionNo: string;
  readonly transactionId: string;
  readonly detailUrl: string;
  readonly identity?: Readonly<{ readonly kind: OrderIdentityKind; readonly value: string }>;
  readonly provider: OrderProvider;
  readonly status: string;
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

export interface OrderDetail {
  readonly id: number;
  readonly orderNo: string;
  readonly provider: OrderProvider;
  readonly payerName: string;
  readonly mobile: string;
  readonly productCode: string;
  readonly productName: string;
  readonly amountYuan: string;
  readonly currency: string;
  readonly statusLabel: string;
  readonly providerLabel: string;
  readonly createdAt: string;
  readonly refundableAmountTotal: number;
}

export type LocalRefundState =
  | "pending_external_gate"
  | "outcome_unknown"
  | "completed"
  | "final_failed";

export interface LocalRefundRecord {
  readonly id: number;
  readonly orderID: number;
  readonly provider: OrderProvider;
  readonly orderNo: string;
  readonly transactionID: string;
  readonly refundID: string;
  readonly outRefundNo: string;
  readonly refundAmountTotal: number;
  readonly currency: "CNY";
  readonly reason: string;
  readonly status: LocalRefundState;
  readonly externalEffectID: number;
  readonly externalEffectState: LocalRefundState;
  readonly autoRetryAllowed: false;
  readonly createdAt: string;
}

export interface LocalRefundPage {
  readonly items: readonly LocalRefundRecord[];
  readonly total: number;
  readonly offset: number;
  readonly hasMore: boolean;
}

async function generatedList(
  params: ListLegacyOrdersParams,
  options: RequestInit,
): Promise<OrdersTransportResponse> {
  return listLegacyOrders(params, options);
}

async function generatedDetail(
  orderNo: string,
  params: GetLegacyOrderParams,
  options: RequestInit,
): Promise<OrdersTransportResponse> {
  return getLegacyOrder(orderNo, params, options);
}

async function generatedRefunds(
  params: ListLegacyRefundsParams,
  options: RequestInit,
): Promise<OrdersTransportResponse> {
  return listLegacyRefunds(params, options);
}

export interface OrdersTransport {
  readonly list: typeof generatedList;
  readonly detail: typeof generatedDetail;
  readonly refunds: typeof generatedRefunds;
}

export const generatedOrdersTransport: OrdersTransport = {
  list: generatedList,
  detail: generatedDetail,
  refunds: generatedRefunds,
};

export type OrdersFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";
export type OrdersResult =
  | { readonly status: "loaded"; readonly page: OrderListPage }
  | { readonly status: OrdersFailure };
export type OrderDetailResult =
  | { readonly status: "loaded"; readonly detail: OrderDetail }
  | { readonly status: OrdersFailure };
export type LocalRefundResult =
  | { readonly status: "loaded"; readonly page: LocalRefundPage }
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

function provider(value: unknown): value is OrderProvider {
  return value === "wechat" || value === "alipay" || value === "wechat_shop";
}

function localRefundState(value: unknown): value is LocalRefundState {
  return value === "pending_external_gate" || value === "outcome_unknown" ||
    value === "completed" || value === "final_failed";
}

function amountMinor(value: string): bigint | undefined {
  const match = /^(?:0|[1-9]\d*)\.(\d{2})$/.exec(value);
  if (!match) return undefined;
  try {
    return BigInt(value.slice(0, -3)) * 100n + BigInt(match[1]);
  } catch {
    return undefined;
  }
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
    !provider(value.provider) ||
    !text(value.provider_label, 80, true) ||
    !safeDetailPath(value.detail_url)
  ) return undefined;
  const identity = identityKeys.length === 1
    ? { kind: identityKeys[0] as OrderIdentityKind, value: value[identityKeys[0]] as string }
    : undefined;
  return {
    orderNo: value.order_no,
    merchantOrderNo: value.merchant_order_no,
    outTradeNo: value.out_trade_no,
    platformTransactionNo: value.platform_transaction_no,
    transactionId: value.transaction_id,
    detailUrl: value.detail_url,
    identity,
    provider: value.provider,
    status: value.status,
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

const REQUIRED_DETAIL_KEYS = [
  ...REQUIRED_ITEM_KEYS,
  "id",
  "refundable_amount_total",
] as const;
const DETAIL_KEYS: readonly string[] = [
  ...REQUIRED_DETAIL_KEYS,
  ...OPTIONAL_IDENTITY_KEYS,
];

function parseOrderDetail(
  value: unknown,
  expected: OrderListItem,
): OrderDetail | undefined {
  if (
    !record(value) ||
    !Object.keys(value).every((key) => DETAIL_KEYS.includes(key)) ||
    !REQUIRED_DETAIL_KEYS.every((key) => key in value)
  ) return undefined;
  const itemProjection: Record<string, unknown> = {};
  for (const key of ITEM_KEYS) {
    if (key in value) itemProjection[key] = value[key];
  }
  const item = parseOrderItem(itemProjection);
  const total = typeof value.amount_yuan === "string"
    ? amountMinor(value.amount_yuan)
    : undefined;
  if (
    !item ||
    typeof value.id !== "number" || !Number.isSafeInteger(value.id) || value.id < 1 ||
    !nonnegative(value.refundable_amount_total) ||
    total === undefined || BigInt(value.refundable_amount_total) > total ||
    value.merchant_order_no !== expected.orderNo ||
    value.out_trade_no !== expected.orderNo ||
    value.order_no !== expected.orderNo ||
    item.orderNo !== expected.orderNo ||
    item.merchantOrderNo !== expected.merchantOrderNo ||
    item.outTradeNo !== expected.outTradeNo ||
    item.platformTransactionNo !== expected.platformTransactionNo ||
    item.transactionId !== expected.transactionId ||
    item.detailUrl !== expected.detailUrl ||
    item.identity?.kind !== expected.identity?.kind ||
    item.identity?.value !== expected.identity?.value ||
    item.provider !== expected.provider ||
    item.status !== expected.status ||
    item.payerName !== expected.payerName ||
    item.mobile !== expected.mobile ||
    item.productCode !== expected.productCode ||
    item.productName !== expected.productName ||
    item.amountYuan !== expected.amountYuan ||
    item.currency !== expected.currency ||
    item.statusLabel !== expected.statusLabel ||
    item.providerLabel !== expected.providerLabel ||
    item.createdAt !== expected.createdAt
  ) return undefined;
  return {
    id: value.id,
    orderNo: item.orderNo,
    provider: item.provider,
    payerName: item.payerName,
    mobile: item.mobile,
    productCode: item.productCode,
    productName: item.productName,
    amountYuan: item.amountYuan,
    currency: item.currency,
    statusLabel: item.statusLabel,
    providerLabel: item.providerLabel,
    createdAt: item.createdAt,
    refundableAmountTotal: value.refundable_amount_total,
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

const LOCAL_REFUND_KEYS = [
  "id", "order_id", "provider", "order_no", "transaction_id", "refund_id",
  "out_refund_no", "refund_amount_total", "currency", "reason", "status",
  "external_effect_id", "external_effect_state", "auto_retry_allowed", "created_at",
] as const;

function parseLocalRefund(value: unknown): LocalRefundRecord | undefined {
  if (
    !record(value) || !exact(value, LOCAL_REFUND_KEYS) ||
    !nonnegative(value.id) || value.id < 1 ||
    !nonnegative(value.order_id) || value.order_id < 1 ||
    !provider(value.provider) ||
    !text(value.order_no, 200, true) ||
    !text(value.transaction_id, 200, true) ||
    !text(value.refund_id, 200, true) ||
    !value.refund_id.startsWith("rfd_") ||
    !text(value.out_refund_no, 200, true) ||
    !value.out_refund_no.startsWith("rfd_") ||
    !nonnegative(value.refund_amount_total) || value.refund_amount_total < 1 ||
    value.currency !== "CNY" ||
    !text(value.reason, 500, true) ||
    !localRefundState(value.status) ||
    !nonnegative(value.external_effect_id) || value.external_effect_id < 1 ||
    !localRefundState(value.external_effect_state) ||
    value.auto_retry_allowed !== false ||
    !timestamp(value.created_at)
  ) return undefined;
  return {
    id: value.id,
    orderID: value.order_id,
    provider: value.provider,
    orderNo: value.order_no,
    transactionID: value.transaction_id,
    refundID: value.refund_id,
    outRefundNo: value.out_refund_no,
    refundAmountTotal: value.refund_amount_total,
    currency: "CNY",
    reason: value.reason,
    status: value.status,
    externalEffectID: value.external_effect_id,
    externalEffectState: value.external_effect_state,
    autoRetryAllowed: false,
    createdAt: value.created_at,
  };
}

function parseLocalRefundPage(value: unknown, offset: number): LocalRefundPage | undefined {
  if (
    !record(value) || !exact(value, ["items", "total", "limit", "has_more"]) ||
    !Array.isArray(value.items) || !nonnegative(value.total) ||
    value.limit !== ORDER_PAGE_SIZE || typeof value.has_more !== "boolean" ||
    value.items.length > ORDER_PAGE_SIZE || value.total < offset + value.items.length ||
    (value.items.length === 0 && value.has_more) ||
    value.has_more !== (offset + value.items.length < value.total)
  ) return undefined;
  const items = value.items.map(parseLocalRefund);
  if (items.some((item) => item === undefined)) return undefined;
  const parsed = items as LocalRefundRecord[];
  if (
    new Set(parsed.map((item) => item.id)).size !== parsed.length ||
    new Set(parsed.map((item) => `${item.provider}:${item.refundID}`)).size !== parsed.length ||
    new Set(parsed.map((item) => `${item.provider}:${item.outRefundNo}`)).size !== parsed.length
  ) return undefined;
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

export async function loadOrderDetail(
  transport: OrdersTransport,
  item: OrderListItem,
): Promise<OrderDetailResult> {
  try {
    const response = await transport.detail(
      item.orderNo,
      { provider: item.provider },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const detail = parseOrderDetail(response.data, item);
    return detail ? { status: "loaded", detail } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadLocalRefunds(
  transport: OrdersTransport,
  offset = 0,
): Promise<LocalRefundResult> {
  if (!nonnegative(offset) || offset > MAXIMUM_OFFSET || offset % ORDER_PAGE_SIZE !== 0) return { status: "invalid" };
  try {
    const response = await transport.refunds(
      { provider: "all", limit: ORDER_PAGE_SIZE, offset },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseLocalRefundPage(response.data, offset);
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

export function previousLocalRefundOffset(page: LocalRefundPage): number | undefined {
  return page.offset >= ORDER_PAGE_SIZE ? page.offset - ORDER_PAGE_SIZE : undefined;
}

export function nextLocalRefundOffset(page: LocalRefundPage): number | undefined {
  const next = page.offset + ORDER_PAGE_SIZE;
  return page.hasMore && next <= MAXIMUM_OFFSET ? next : undefined;
}
