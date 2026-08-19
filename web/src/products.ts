import { getProduct, listProducts, type ListProductsParams } from "./api/generated/health";

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
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface ProductPage {
  readonly items: readonly ProductListItem[];
  readonly nextCursor?: string;
}

export interface ProductsTransportResponse { readonly status: number; readonly data: unknown }
export interface ProductsTransport {
  // eslint-disable-next-line no-unused-vars -- named transport arguments document the generated GET contract.
  readonly list: (params: ListProductsParams, options: RequestInit) => Promise<ProductsTransportResponse>;
  // eslint-disable-next-line no-unused-vars -- named transport arguments document the generated GET contract.
  readonly get: (productID: number, options: RequestInit) => Promise<ProductsTransportResponse>;
}

export const generatedProductsTransport: ProductsTransport = { list: listProducts, get: getProduct };
export type ProductsFailure = "unauthenticated" | "forbidden" | "invalid" | "unavailable";
export type ProductsResult = { readonly status: "loaded"; readonly page: ProductPage } | { readonly status: ProductsFailure };
export type ProductDetailResult = { readonly status: "loaded"; readonly product: ProductListItem } | { readonly status: ProductsFailure };

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
  return typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value));
}
function images(value: unknown): boolean {
  return Array.isArray(value) && value.length <= 20 && value.every((item) => text(item, 2048, true));
}
const PRODUCT_KEYS = ["id", "product_code", "name", "description", "price_minor", "currency", "stock_quantity", "images", "created_by", "created_at", "updated_at"] as const;

function parseProduct(value: unknown): ProductListItem | undefined {
  if (!record(value) || !exact(value, PRODUCT_KEYS) || !positive(value.id) || !text(value.product_code, 200, true) ||
    !text(value.name, 200, true) || !text(value.description, 10000) || !nonnegative(value.price_minor) ||
    typeof value.currency !== "string" || !/^[A-Z]{3}$/.test(value.currency) || !nonnegative(value.stock_quantity) || value.stock_quantity > 2_147_483_647 ||
    !images(value.images) || !positive(value.created_by) || !timestamp(value.created_at) || !timestamp(value.updated_at)) return undefined;
  return { id: value.id, productCode: value.product_code, name: value.name, description: value.description,
    priceMinor: value.price_minor, currency: value.currency, stockQuantity: value.stock_quantity,
    createdAt: value.created_at, updatedAt: value.updated_at };
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
