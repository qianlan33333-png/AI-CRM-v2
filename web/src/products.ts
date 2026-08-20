import {
  createProduct,
  getProductLocalEntitlement,
  getProduct,
  grantProductLocalEntitlement as generatedGrantProductLocalEntitlement,
  listProductLocalEntitlements,
  listProducts,
  revokeProductLocalEntitlement as generatedRevokeProductLocalEntitlement,
  updateProduct,
  type CreateProductRequest,
  type ListProductsParams,
} from "./api/generated/health";

export type ProductsRole = "admin" | "ops" | "sales";
export const PRODUCT_PAGE_SIZE = 50;

export interface ProductListItem {
  readonly id: number;
  readonly productCode: string;
  readonly name: string;
  readonly description: string;
  readonly priceMinor: number;
  readonly currency: string;
	readonly stockQuantity: number;
	readonly images: readonly string[];
	readonly createdBy: number;
  readonly createdAt: string;
  readonly updatedAt: string;
	// Present only once the native v2 CAS transport is wired by Lane E. The
	// existing read-only generated transport has no version field yet.
	readonly version?: number;
}

export interface ProductPage {
  readonly items: readonly ProductListItem[];
  readonly nextCursor?: string;
}

export interface ProductDraft {
  readonly productCode: string;
  readonly name: string;
  readonly description: string;
  readonly priceMinor: string;
  readonly currency: string;
  readonly stockQuantity: string;
}

export interface ProductUpdateDraft {
	readonly expectedVersion: number;
	readonly name: string;
	readonly description: string;
	readonly priceMinor: string;
	readonly currency: string;
	readonly stockQuantity: string;
}

export interface ProductUpdateRequest {
	readonly expected_version: number;
	readonly name: string;
	readonly description: string;
	readonly price_minor: number;
	readonly currency: string;
	readonly stock_quantity: number;
}

export interface LocalEntitlement {
	readonly id: number;
	readonly productId: number;
	readonly orderId: number;
	readonly state: "active" | "revoked";
	readonly version: number;
	readonly grantedAt: string;
	readonly revokedAt?: string;
}

export interface ProductsTransportResponse { readonly status: number; readonly data: unknown }
export interface ProductsTransport {
  // eslint-disable-next-line no-unused-vars -- named transport arguments document the generated GET contract.
  readonly list: (params: ListProductsParams, options: RequestInit) => Promise<ProductsTransportResponse>;
  // eslint-disable-next-line no-unused-vars -- named transport arguments document the generated GET contract.
  readonly get: (productID: number, options: RequestInit) => Promise<ProductsTransportResponse>;
  // Optional only to preserve existing injected read-only test transports; the
  // generated production transport always supplies the canonical local writer.
  // eslint-disable-next-line no-unused-vars -- named arguments document the generated POST contract.
  readonly create?: (request: CreateProductRequest, options: RequestInit) => Promise<ProductsTransportResponse>;
	// Lane E replaces these optional injectable ports with the generated clients
	// after the frozen OpenAPI operations land.
	// eslint-disable-next-line no-unused-vars -- frozen future generated update contract.
	readonly update?: (productID: number, request: ProductUpdateRequest, options: RequestInit) => Promise<ProductsTransportResponse>;
	// eslint-disable-next-line no-unused-vars -- frozen future generated entitlement-list contract.
	readonly listEntitlements?: (productID: number, params: { readonly limit: number }, options: RequestInit) => Promise<ProductsTransportResponse>;
	// eslint-disable-next-line no-unused-vars -- frozen future generated entitlement-detail contract.
	readonly getEntitlement?: (entitlementID: number, options: RequestInit) => Promise<ProductsTransportResponse>;
	// eslint-disable-next-line no-unused-vars -- frozen future generated entitlement-grant contract.
	readonly grantEntitlement?: (productID: number, request: { readonly order_id: number }, options: RequestInit) => Promise<ProductsTransportResponse>;
	// eslint-disable-next-line no-unused-vars -- frozen future generated entitlement-revoke contract.
	readonly revokeEntitlement?: (entitlementID: number, request: { readonly expected_version: number }, options: RequestInit) => Promise<ProductsTransportResponse>;
}

export const generatedProductsTransport: ProductsTransport = {
  list: listProducts,
  get: getProduct,
  create: createProduct,
  update: updateProduct,
  listEntitlements: listProductLocalEntitlements,
  getEntitlement: getProductLocalEntitlement,
  grantEntitlement: generatedGrantProductLocalEntitlement,
  revokeEntitlement: generatedRevokeProductLocalEntitlement,
};
export type ProductsFailure = "unauthenticated" | "forbidden" | "invalid" | "unavailable";
export type ProductsResult = { readonly status: "loaded"; readonly page: ProductPage } | { readonly status: ProductsFailure };
export type ProductDetailResult = { readonly status: "loaded"; readonly product: ProductListItem } | { readonly status: ProductsFailure };
export type ProductCreateResult =
  | { readonly status: "created"; readonly product: ProductListItem }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" | "conflict" | "unknown" };

export const defaultProductDraft: ProductDraft = {
  productCode: "",
  name: "",
  description: "",
  priceMinor: "0",
  currency: "CNY",
  stockQuantity: "0",
};

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);
}
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}
function positive(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function nonnegative(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 0; }
function text(value: unknown, maximum: number, nonempty = false): value is string {
  return typeof value === "string" && value === value.trim() && (!nonempty || value.length > 0) &&
    new TextEncoder().encode(value).length <= maximum;
}
function timestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const matched = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-](\d{2}):(\d{2}))$/.exec(value);
  if (!matched) return false;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, , offsetHourText, offsetMinuteText] = matched;
  const year = Number(yearText); const month = Number(monthText); const day = Number(dayText);
  const hour = Number(hourText); const minute = Number(minuteText); const second = Number(secondText);
  const offsetHour = offsetHourText === undefined ? 0 : Number(offsetHourText);
  const offsetMinute = offsetMinuteText === undefined ? 0 : Number(offsetMinuteText);
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const monthDays = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return month >= 1 && month <= 12 && day >= 1 && day <= monthDays[month - 1] &&
    hour <= 23 && minute <= 59 && second <= 59 && offsetHour <= 23 && offsetMinute <= 59;
}
function images(value: unknown): boolean {
  return Array.isArray(value) && value.length <= 20 && value.every((item) => text(item, 2048, true));
}
const PRODUCT_KEYS = ["id", "product_code", "name", "description", "price_minor", "currency", "stock_quantity", "images", "created_by", "created_at", "updated_at", "version"] as const;

function parseProduct(value: unknown): ProductListItem | undefined {
	if (!record(value) || !exact(value, PRODUCT_KEYS) || !positive(value.id) || !text(value.product_code, 200, true) ||
		!text(value.name, 200, true) || !text(value.description, 10000) || !nonnegative(value.price_minor) ||
		typeof value.currency !== "string" || !/^[A-Z]{3}$/.test(value.currency) || !nonnegative(value.stock_quantity) || value.stock_quantity > 2_147_483_647 ||
		!images(value.images) || !positive(value.created_by) || !timestamp(value.created_at) || !timestamp(value.updated_at) || !positive(value.version)) return undefined;
	if (Date.parse(value.updated_at) < Date.parse(value.created_at)) return undefined;
	return { id: value.id, productCode: value.product_code, name: value.name, description: value.description,
		priceMinor: value.price_minor, currency: value.currency, stockQuantity: value.stock_quantity, images: value.images as string[], createdBy: value.created_by,
		createdAt: value.created_at, updatedAt: value.updated_at, version: value.version };
}

export function parseProductDetail(value: unknown, requestedID: number): ProductListItem | undefined {
  const product = parseProduct(value);
  return product?.id === requestedID ? product : undefined;
}

export function parseProductPage(value: unknown): ProductPage | undefined {
  if (!record(value) || !Object.keys(value).every((key) => key === "items" || key === "next_cursor") || !Array.isArray(value.items)) return undefined;
  if ("next_cursor" in value && !text(value.next_cursor, 512, true)) return undefined;
  const items = value.items.map(parseProduct);
  if (items.some((item) => item === undefined) || items.length > PRODUCT_PAGE_SIZE) return undefined;
  const parsed = items as ProductListItem[];
  if (parsed.some((item, index) => index > 0 && item.id <= parsed[index - 1].id)) return undefined;
  if ("next_cursor" in value && parsed.length !== PRODUCT_PAGE_SIZE) return undefined;
  return "next_cursor" in value ? { items: parsed, nextCursor: value.next_cursor as string } : { items: parsed };
}

export function canReadProducts(role: ProductsRole): boolean { return role === "admin" || role === "ops"; }

function parseNonnegativeInput(value: string, maximum: number): number | undefined {
  if (!/^(?:0|[1-9]\d*)$/.test(value)) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed >= 0 && parsed <= maximum ? parsed : undefined;
}

export function normalizeProductDraft(draft: ProductDraft): ProductDraft | undefined {
  const normalized: ProductDraft = {
    productCode: draft.productCode.trim(),
    name: draft.name.trim(),
    description: draft.description.trim(),
    priceMinor: draft.priceMinor.trim(),
    currency: draft.currency.trim().toUpperCase(),
    stockQuantity: draft.stockQuantity.trim(),
  };
  const priceMinor = parseNonnegativeInput(normalized.priceMinor, Number.MAX_SAFE_INTEGER);
  const stockQuantity = parseNonnegativeInput(normalized.stockQuantity, 2_147_483_647);
  if (
    !text(normalized.productCode, 200, true) ||
    !text(normalized.name, 200, true) ||
    !text(normalized.description, 10_000) ||
    priceMinor === undefined ||
    stockQuantity === undefined ||
    !/^[A-Z]{3}$/.test(normalized.currency)
  ) return undefined;
  return normalized;
}

export function productDraftProblem(draft: ProductDraft): string | undefined {
  const normalized = normalizeProductDraft(draft);
  if (!normalized) return "请填写合法的产品码、名称、描述、金额、货币与库存。";
  return undefined;
}

export function productCreateRequest(draft: ProductDraft): CreateProductRequest | undefined {
  const normalized = normalizeProductDraft(draft);
  if (!normalized) return undefined;
  const priceMinor = parseNonnegativeInput(normalized.priceMinor, Number.MAX_SAFE_INTEGER);
  const stockQuantity = parseNonnegativeInput(normalized.stockQuantity, 2_147_483_647);
  if (priceMinor === undefined || stockQuantity === undefined) return undefined;
  // This browser scope creates only local catalog records. Image references
  // remain out of scope and are therefore always the closed empty array.
  return {
    product_code: normalized.productCode,
    name: normalized.name,
    description: normalized.description,
    price_minor: priceMinor,
    currency: normalized.currency,
    stock_quantity: stockQuantity,
    images: [],
  };
}

export function newProductIdempotencyKey(
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  try {
    const uuid = source?.randomUUID();
    if (
      typeof uuid !== "string" ||
      !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid)
    ) return undefined;
    return `product-create:${uuid}`;
  } catch {
    return undefined;
  }
}

function matchesCreatedProduct(
  value: unknown,
  request: CreateProductRequest,
): ProductListItem | undefined {
  const product = parseProduct(value);
  if (!product || !record(value) || !Array.isArray(value.images) || value.images.length !== 0) return undefined;
  return (
    product.productCode === request.product_code &&
    product.name === request.name &&
    product.description === request.description &&
    product.priceMinor === request.price_minor &&
    product.currency === request.currency &&
    product.stockQuantity === request.stock_quantity
  ) ? product : undefined;
}

export async function createLocalProduct(
  transport: ProductsTransport,
  draft: ProductDraft,
  csrfToken: string,
  idempotencyKey: string,
): Promise<ProductCreateResult> {
  const request = productCreateRequest(draft);
  if (
    !request ||
    !/^[A-Za-z0-9_-]{43}$/.test(csrfToken) ||
    !/^[A-Za-z0-9:_-]{16,128}$/.test(idempotencyKey) ||
    transport.create === undefined
  ) return { status: "invalid" };
  try {
    const response = await transport.create(request, {
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": csrfToken,
        "Idempotency-Key": idempotencyKey,
      },
    });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 409) return { status: "conflict" };
    if (response.status === 400) return { status: "invalid" };
    if (response.status !== 201) return { status: "unknown" };
    const product = matchesCreatedProduct(response.data, request);
    return product ? { status: "created", product } : { status: "unknown" };
  } catch {
    return { status: "unknown" };
  }
}

export async function loadProducts(transport: ProductsTransport, cursor?: string): Promise<ProductsResult> {
  try {
    const response = await transport.list(cursor === undefined ? { limit: PRODUCT_PAGE_SIZE } : { limit: PRODUCT_PAGE_SIZE, cursor }, { credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status !== 200) return { status: "unavailable" };
    const page = parseProductPage(response.data);
    return page === undefined ? { status: "invalid" } : { status: "loaded", page };
  } catch { return { status: "unavailable" }; }
}

export async function loadProductDetail(transport: ProductsTransport, productID: number): Promise<ProductDetailResult> {
  if (!positive(productID)) return { status: "invalid" };
  try {
    const response = await transport.get(productID, { credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status !== 200) return { status: "unavailable" };
    const product = parseProductDetail(response.data, productID);
    return product === undefined ? { status: "invalid" } : { status: "loaded", product };
  } catch { return { status: "unavailable" }; }
}

export const defaultProductUpdateDraft = (product: ProductListItem): ProductUpdateDraft | undefined => product.version === undefined ? undefined : {
	expectedVersion: product.version,
	name: product.name,
	description: product.description,
	priceMinor: String(product.priceMinor),
	currency: product.currency,
	stockQuantity: String(product.stockQuantity),
};

export function productUpdateRequest(draft: ProductUpdateDraft): ProductUpdateRequest | undefined {
	const name = draft.name.trim(); const description = draft.description.trim(); const currency = draft.currency.trim().toUpperCase();
	const priceMinor = parseNonnegativeInput(draft.priceMinor.trim(), Number.MAX_SAFE_INTEGER);
	const stockQuantity = parseNonnegativeInput(draft.stockQuantity.trim(), 2_147_483_647);
	if (!positive(draft.expectedVersion) || draft.expectedVersion === Number.MAX_SAFE_INTEGER || !text(name, 200, true) || !text(description, 10_000) || priceMinor === undefined || stockQuantity === undefined || !/^[A-Z]{3}$/.test(currency)) return undefined;
	return { expected_version: draft.expectedVersion, name, description, price_minor: priceMinor, currency, stock_quantity: stockQuantity };
}

export type ProductMutationResult =
	| { readonly status: "updated"; readonly product: ProductListItem }
	| { readonly status: "granted" | "revoked"; readonly entitlement: LocalEntitlement }
	| { readonly status: "unauthenticated" | "forbidden" | "invalid" | "conflict" | "unknown" };
export type EntitlementReadResult = { readonly status: "loaded"; readonly entitlement: LocalEntitlement } | { readonly status: ProductsFailure };
export type EntitlementListResult = { readonly status: "loaded"; readonly items: readonly LocalEntitlement[] } | { readonly status: ProductsFailure };

function localMutationOptions(csrfToken: string, idempotencyKey: string): RequestInit | undefined {
	if (!/^[A-Za-z0-9_-]{43}$/.test(csrfToken) || !/^[A-Za-z0-9:_-]{16,128}$/.test(idempotencyKey)) return undefined;
	return { credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken, "Idempotency-Key": idempotencyKey } };
}
function mutationFailure(status: number): Exclude<ProductMutationResult, { readonly status: "updated" } | { readonly status: "granted" } | { readonly status: "revoked" }> {
	if (status === 401) return { status: "unauthenticated" };
	if (status === 403) return { status: "forbidden" };
	if (status === 400) return { status: "invalid" };
	if (status === 409) return { status: "conflict" };
	return { status: "unknown" };
}

export async function updateLocalProduct(transport: ProductsTransport, product: ProductListItem, draft: ProductUpdateDraft, csrfToken: string, idempotencyKey: string): Promise<ProductMutationResult> {
	const request = productUpdateRequest(draft); const options = localMutationOptions(csrfToken, idempotencyKey);
	if (!positive(product.id) || product.version === undefined || request === undefined || options === undefined || transport.update === undefined) return { status: "invalid" };
	try {
		const response = await transport.update(product.id, request, options);
		if (response.status !== 200) return mutationFailure(response.status);
		const result = parseProductDetail(response.data, product.id);
		return result !== undefined && result.version === request.expected_version + 1 && result.productCode === product.productCode && result.createdBy === product.createdBy && result.createdAt === product.createdAt && result.images.length === product.images.length && result.images.every((image, index) => image === product.images[index]) && Date.parse(result.updatedAt) >= Date.parse(result.createdAt) && result.name === request.name && result.description === request.description && result.priceMinor === request.price_minor && result.currency === request.currency && result.stockQuantity === request.stock_quantity ? { status: "updated", product: result } : { status: "unknown" };
	} catch { return { status: "unknown" }; }
}

const ENTITLEMENT_KEYS = ["id", "product_id", "order_id", "state", "version", "granted_at", "revoked_at"] as const;
function parseLocalEntitlement(value: unknown): LocalEntitlement | undefined {
	if (!record(value) || !exact(value, ENTITLEMENT_KEYS) || !positive(value.id) || !positive(value.product_id) || !positive(value.order_id) || !positive(value.version) || !timestamp(value.granted_at) || (value.state !== "active" && value.state !== "revoked")) return undefined;
	if (value.state === "active" && value.revoked_at !== null) return undefined;
	if (value.state === "revoked" && !timestamp(value.revoked_at)) return undefined;
	return { id: value.id, productId: value.product_id, orderId: value.order_id, state: value.state, version: value.version, grantedAt: value.granted_at, revokedAt: value.revoked_at === null ? undefined : value.revoked_at as string };
}
function parseLocalEntitlementList(value: unknown, productID: number): readonly LocalEntitlement[] | undefined {
	if (!record(value) || !exact(value, ["items"]) || !Array.isArray(value.items) || value.items.length > PRODUCT_PAGE_SIZE) return undefined;
	const items = value.items.map(parseLocalEntitlement);
	if (items.some((item) => item === undefined)) return undefined;
	const parsed = items as LocalEntitlement[];
	return parsed.every((item, index) => item.productId === productID && (index === 0 || item.id < parsed[index - 1].id)) ? parsed : undefined;
}

export async function loadProductLocalEntitlements(transport: ProductsTransport, productID: number): Promise<EntitlementListResult> {
	if (!positive(productID) || transport.listEntitlements === undefined) return { status: "invalid" };
	try {
		const response = await transport.listEntitlements(productID, { limit: PRODUCT_PAGE_SIZE }, { credentials: "same-origin" });
		if (response.status === 401) return { status: "unauthenticated" }; if (response.status === 403) return { status: "forbidden" }; if (response.status !== 200) return { status: "unavailable" };
		const items = parseLocalEntitlementList(response.data, productID); return items === undefined ? { status: "invalid" } : { status: "loaded", items };
	} catch { return { status: "unavailable" }; }
}
export async function loadProductLocalEntitlement(transport: ProductsTransport, entitlementID: number): Promise<EntitlementReadResult> {
	if (!positive(entitlementID) || transport.getEntitlement === undefined) return { status: "invalid" };
	try {
		const response = await transport.getEntitlement(entitlementID, { credentials: "same-origin" });
		if (response.status === 401) return { status: "unauthenticated" }; if (response.status === 403) return { status: "forbidden" }; if (response.status !== 200) return { status: "unavailable" };
		const entitlement = parseLocalEntitlement(response.data); return entitlement === undefined || entitlement.id !== entitlementID ? { status: "invalid" } : { status: "loaded", entitlement };
	} catch { return { status: "unavailable" }; }
}
export async function grantProductLocalEntitlement(transport: ProductsTransport, productID: number, orderID: number, csrfToken: string, idempotencyKey: string): Promise<ProductMutationResult> {
	const options = localMutationOptions(csrfToken, idempotencyKey);
	if (!positive(productID) || !positive(orderID) || options === undefined || transport.grantEntitlement === undefined) return { status: "invalid" };
	try {
		const response = await transport.grantEntitlement(productID, { order_id: orderID }, options);
		if (response.status !== 201) return mutationFailure(response.status);
		const entitlement = parseLocalEntitlement(response.data); return entitlement !== undefined && entitlement.productId === productID && entitlement.orderId === orderID && entitlement.state === "active" && entitlement.version === 1 ? { status: "granted", entitlement } : { status: "unknown" };
	} catch { return { status: "unknown" }; }
}
export async function revokeProductLocalEntitlement(transport: ProductsTransport, entitlement: LocalEntitlement, csrfToken: string, idempotencyKey: string): Promise<ProductMutationResult> {
	const options = localMutationOptions(csrfToken, idempotencyKey);
	if (!positive(entitlement.id) || entitlement.state !== "active" || options === undefined || transport.revokeEntitlement === undefined) return { status: "invalid" };
	try {
		const response = await transport.revokeEntitlement(entitlement.id, { expected_version: entitlement.version }, options);
		if (response.status !== 200) return mutationFailure(response.status);
		const result = parseLocalEntitlement(response.data); return result !== undefined && result.id === entitlement.id && result.productId === entitlement.productId && result.orderId === entitlement.orderId && result.grantedAt === entitlement.grantedAt && result.state === "revoked" && result.revokedAt !== undefined && Date.parse(result.revokedAt) >= Date.parse(result.grantedAt) && result.version === entitlement.version + 1 ? { status: "revoked", entitlement: result } : { status: "unknown" };
	} catch { return { status: "unknown" }; }
}

export function newLocalProductIdempotencyKey(operation: "update" | "grant" | "revoke", source: { readonly randomUUID: () => string } | undefined = globalThis.crypto): string | undefined {
	const key = newProductIdempotencyKey(source);
	return key === undefined ? undefined : key.replace("product-create:", `product-${operation}:`);
}
