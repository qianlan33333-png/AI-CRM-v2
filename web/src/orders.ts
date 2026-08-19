import {
  getLegacyOrder,
  getLegacyOrderItems,
  listLegacyWechatOrderExternalEffects,
  listLegacyOrders,
  listLegacyRefunds,
  type GetLegacyOrderParams,
  type GetLegacyOrderItemsParams,
  type ListLegacyOrdersParams,
  type ListLegacyRefundsParams,
} from "./api/generated/health";

export type OrdersRole = "admin" | "ops" | "sales";
export const ORDER_PAGE_SIZE = 50;
const MAXIMUM_OFFSET = 1_000_000;
export type OrderProvider = "wechat" | "alipay" | "wechat_shop";

export interface OrderListItem {
  readonly orderNo: string;
  readonly provider: OrderProvider;
  readonly status: string;
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
  readonly productCode: string;
  readonly productName: string;
  readonly amountYuan: string;
  readonly currency: string;
  readonly statusLabel: string;
  readonly providerLabel: string;
  readonly createdAt: string;
  readonly refundableAmountTotal: number;
}

/** A purchased-product snapshot is local order data, never a media or provider link. */
export interface OrderItemSnapshot {
  readonly orderNo: string;
  readonly provider: OrderProvider;
  readonly productCode: string;
  readonly productName: string;
  readonly amountYuan: string;
  readonly currency: string;
  readonly createdAt: string;
}

export type LocalRefundState =
  | "pending_external_gate"
  | "outcome_unknown"
  | "completed"
  | "final_failed";

export interface LocalRefundRecord {
  readonly id: number;
  readonly provider: OrderProvider;
  readonly orderNo: string;
  readonly refundAmountTotal: number;
  readonly currency: "CNY";
  readonly status: LocalRefundState;
  readonly externalEffectState: LocalRefundState;
  readonly createdAt: string;
}

export interface LocalRefundPage {
  readonly items: readonly LocalRefundRecord[];
  readonly total: number;
  readonly offset: number;
  readonly hasMore: boolean;
}

/** Safe local projection only. Provider receipts and review metadata are verified then discarded. */
export interface LocalExternalEffect {
  readonly id: number;
  readonly kind: "refund" | "external_push";
  readonly state: LocalRefundState;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface LocalExternalEffectPage {
  readonly items: readonly LocalExternalEffect[];
  readonly total: number;
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

async function generatedItems(
  orderNo: string,
  params: GetLegacyOrderItemsParams,
  options: RequestInit,
): Promise<OrdersTransportResponse> {
  return getLegacyOrderItems(orderNo, params, options);
}

async function generatedRefunds(
  params: ListLegacyRefundsParams,
  options: RequestInit,
): Promise<OrdersTransportResponse> {
  return listLegacyRefunds(params, options);
}

async function generatedExternalEffects(
  orderNo: string,
  options: RequestInit,
): Promise<OrdersTransportResponse> {
  return listLegacyWechatOrderExternalEffects(orderNo, options);
}

export interface OrdersTransport {
  readonly list: typeof generatedList;
  readonly detail: typeof generatedDetail;
  readonly items: typeof generatedItems;
  readonly refunds: typeof generatedRefunds;
  readonly externalEffects?: typeof generatedExternalEffects;
}

export const generatedOrdersTransport: OrdersTransport = {
  list: generatedList,
  detail: generatedDetail,
  items: generatedItems,
  refunds: generatedRefunds,
  externalEffects: generatedExternalEffects,
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
export type OrderItemsResult =
  | { readonly status: "loaded"; readonly item: OrderItemSnapshot }
  | { readonly status: OrdersFailure };
export type LocalRefundResult =
  | { readonly status: "loaded"; readonly page: LocalRefundPage }
  | { readonly status: OrdersFailure };
export type LocalExternalEffectResult =
  | { readonly status: "loaded"; readonly page: LocalExternalEffectPage }
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
  return {
    orderNo: value.order_no,
    provider: value.provider,
    status: value.status,
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
    value.order_no !== expected.orderNo ||
    item.orderNo !== expected.orderNo ||
    item.provider !== expected.provider ||
    item.status !== expected.status ||
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

function parseOrderItems(value: unknown, expected: OrderListItem): OrderItemSnapshot | undefined {
  if (!record(value) || !exact(value, ["items"]) || !Array.isArray(value.items) || value.items.length !== 1) {
    return undefined;
  }
  const item = parseOrderItem(value.items[0]);
  if (
    !item || item.orderNo !== expected.orderNo || item.provider !== expected.provider ||
    item.productCode !== expected.productCode || item.productName !== expected.productName ||
    item.amountYuan !== expected.amountYuan || item.currency !== expected.currency ||
    item.createdAt !== expected.createdAt
  ) return undefined;
  return {
    orderNo: item.orderNo, provider: item.provider, productCode: item.productCode,
    productName: item.productName, amountYuan: item.amountYuan, currency: item.currency,
    createdAt: item.createdAt,
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
    provider: value.provider,
    orderNo: value.order_no,
    refundAmountTotal: value.refund_amount_total,
    currency: "CNY",
    status: value.status,
    externalEffectState: value.external_effect_state,
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
  const raw = value.items as Record<string, unknown>[];
  if (
    new Set(parsed.map((item) => item.id)).size !== parsed.length ||
    new Set(raw.map((item) => `${item.provider}:${item.refund_id}`)).size !== parsed.length ||
    new Set(raw.map((item) => `${item.provider}:${item.out_refund_no}`)).size !== parsed.length
  ) return undefined;
  return { items: parsed, total: value.total, offset, hasMore: value.has_more };
}

const LOCAL_EXTERNAL_EFFECT_KEYS = [
  "id", "order_id", "provider", "effect_kind", "state", "auto_retry_allowed", "provider_receipt", "manual_review_requested_at", "created_at", "updated_at",
] as const;

function parseLocalExternalEffect(value: unknown, orderID: number): LocalExternalEffect | undefined {
  const required = LOCAL_EXTERNAL_EFFECT_KEYS.slice(0, 8);
  if (!record(value) || !Object.keys(value).every((key) => LOCAL_EXTERNAL_EFFECT_KEYS.includes(key as typeof LOCAL_EXTERNAL_EFFECT_KEYS[number])) || !required.every((key) => Object.hasOwn(value, key)) || !nonnegative(value.id) || value.id < 1 || value.order_id !== orderID || value.provider !== "wechat" || (value.effect_kind !== "refund" && value.effect_kind !== "external_push") || !localRefundState(value.state) || value.auto_retry_allowed !== false || (Object.hasOwn(value, "provider_receipt") && value.provider_receipt !== null && !text(value.provider_receipt, 4096, true)) || (Object.hasOwn(value, "manual_review_requested_at") && value.manual_review_requested_at !== null && !timestamp(value.manual_review_requested_at)) || !timestamp(value.created_at) || !timestamp(value.updated_at) || Date.parse(value.updated_at) < Date.parse(value.created_at)) return undefined;
  return { id: value.id, kind: value.effect_kind, state: value.state, createdAt: value.created_at, updatedAt: value.updated_at };
}

export function parseLocalExternalEffectPage(value: unknown, orderID: number): LocalExternalEffectPage | undefined {
  if (!record(value) || !exact(value, ["items", "total"]) || !Array.isArray(value.items) || !nonnegative(value.total) || value.total !== value.items.length) return undefined;
  const items = value.items.map((item) => parseLocalExternalEffect(item, orderID));
  return items.includes(undefined) || new Set((items as readonly LocalExternalEffect[]).map((item) => item.id)).size !== items.length ? undefined : { items: items as readonly LocalExternalEffect[], total: value.total };
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

export async function loadOrderItems(
  transport: OrdersTransport,
  item: OrderListItem,
): Promise<OrderItemsResult> {
  try {
    const response = await transport.items(
      item.orderNo,
      { provider: item.provider },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const snapshot = parseOrderItems(response.data, item);
    return snapshot ? { status: "loaded", item: snapshot } : { status: "invalid" };
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

export async function loadLocalExternalEffects(transport: OrdersTransport, detail: OrderDetail): Promise<LocalExternalEffectResult> {
  if (detail.provider !== "wechat" || !text(detail.orderNo, 200, true) || !transport.externalEffects) return { status: "invalid" };
  try {
    const response = await transport.externalEffects(detail.orderNo, { credentials: "same-origin" });
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseLocalExternalEffectPage(response.data, detail.id);
    return page ? { status: "loaded", page } : { status: "invalid" };
  } catch { return { status: "unavailable" }; }
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

export interface SafeOrderFilter {
  readonly keyword: string;
  readonly provider: "all" | OrderProvider;
  readonly status: string;
}

export const defaultSafeOrderFilter: SafeOrderFilter = { keyword: "", provider: "all", status: "" };

function localIncludes(haystack: string, needle: string): boolean {
  return haystack.toLocaleLowerCase().includes(needle.toLocaleLowerCase());
}

/** Filters only an already validated in-memory page; it never broadens a server query. */
export function filterSafeOrders(
  items: readonly OrderListItem[],
  filter: SafeOrderFilter,
): readonly OrderListItem[] {
  const keyword = filter.keyword.trim();
  return items.filter((item) =>
    (filter.provider === "all" || item.provider === filter.provider) &&
    (filter.status === "" || item.status === filter.status) &&
    (keyword === "" || [item.orderNo, item.productCode, item.productName].some((value) => localIncludes(value, keyword))),
  );
}

/** The refund filter is also local-only and omits raw provider identifiers and reasons. */
export function filterSafeRefunds(
  items: readonly LocalRefundRecord[],
  filter: SafeOrderFilter,
): readonly LocalRefundRecord[] {
  const keyword = filter.keyword.trim();
  return items.filter((item) =>
    (filter.provider === "all" || item.provider === filter.provider) &&
    (filter.status === "" || item.status === filter.status) &&
    (keyword === "" || localIncludes(item.orderNo, keyword)),
  );
}
