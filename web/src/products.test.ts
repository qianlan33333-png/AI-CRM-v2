import { describe, expect, it } from "vitest";
import { createLocalProduct, grantProductLocalEntitlement, loadProductDetail, loadProductLocalEntitlement, loadProductLocalEntitlements, loadProducts, newLocalProductIdempotencyKey, newProductIdempotencyKey, parseProductDetail, parseProductPage, productCreateRequest, productUpdateRequest, revokeProductLocalEntitlement, updateLocalProduct } from "./products";

const product = { id: 1, product_code: "SKU-1", name: "商品", description: "本地描述", price_minor: 1990, currency: "CNY", stock_quantity: 3, images: ["opaque-image-value"], created_by: 7, created_at: "2026-08-19T00:00:00Z", updated_at: "2026-08-19T00:00:00Z", version: 1 };
describe("products", () => {
  it("accepts the closed local page while retaining immutable facts for CAS verification", () => {
    expect(parseProductPage({ items: [product] })).toEqual({ items: [{ id: 1, productCode: "SKU-1", name: "商品", description: "本地描述", priceMinor: 1990, currency: "CNY", stockQuantity: 3, images: ["opaque-image-value"], createdBy: 7, createdAt: product.created_at, updatedAt: product.updated_at, version: 1 }] });
  });
  it("accepts only a direct closed detail for the requested product and keeps hidden immutable facts", () => {
    expect(parseProductDetail(product, 1)).toEqual({ id: 1, productCode: "SKU-1", name: "商品", description: "本地描述", priceMinor: 1990, currency: "CNY", stockQuantity: 3, images: ["opaque-image-value"], createdBy: 7, createdAt: product.created_at, updatedAt: product.updated_at, version: 1 });
    expect(parseProductDetail(product, 2)).toBeUndefined();
    expect(parseProductDetail({ ...product, extra: true }, 1)).toBeUndefined();
    expect(parseProductDetail({ ...product, images: [" https://unsafe.example"] }, 1)).toBeUndefined();
		expect(parseProductDetail(({ ...product, version: undefined } as unknown), 1)).toBeUndefined();
  });
  it("rejects extra fields, unsafe product values, and non-monotonic pages", () => {
    expect(parseProductPage({ items: [{ ...product, payment_url: "https://bad.example" }] })).toBeUndefined();
    expect(parseProductPage({ items: [{ ...product, images: [" https://bad.example"] }] })).toBeUndefined();
    expect(parseProductPage({ items: [{ ...product, price_minor: -1 }] })).toBeUndefined();
    expect(parseProductPage({ items: [{ ...product, stock_quantity: 2_147_483_648 }] })).toBeUndefined();
    expect(parseProductDetail({ ...product, updated_at: "2026-08-18T23:59:59Z" }, 1)).toBeUndefined();
    expect(parseProductPage({ items: [product, { ...product, id: 1 }] })).toBeUndefined();
  });
  it("treats cursors as opaque and maps response failures", async () => {
    const calls: unknown[] = [];
    const transport = { list: async (params: unknown) => { calls.push(params); return { status: 200, data: { items: Array.from({ length: 50 }, (_, index) => ({ ...product, id: index + 1, product_code: `SKU-${index + 1}` })), next_cursor: "opaque-next" } }; }, get: async () => ({ status: 200, data: product }) };
    expect((await loadProducts(transport, "opaque-input")).status).toBe("loaded");
    expect(calls).toEqual([{ limit: 50, cursor: "opaque-input" }]);
    expect((await loadProducts({ list: async () => ({ status: 401, data: {} }), get: async () => ({ status: 200, data: product }) })).status).toBe("unauthenticated");
    expect((await loadProductDetail({ list: async () => ({ status: 200, data: { items: [] } }), get: async (id: number, options: RequestInit) => { calls.push([id, options.credentials]); return { status: 200, data: product }; } }, 1)).status).toBe("loaded");
    expect(calls.at(-1)).toEqual([1, "same-origin"]);
    expect((await loadProductDetail({ list: async () => ({ status: 200, data: { items: [] } }), get: async () => ({ status: 401, data: {} }) }, 1)).status).toBe("unauthenticated");
  });
  it("creates only the normalized local record with an empty image list and a mirrored 201 receipt", async () => {
    const draft = { productCode: " SKU-2 ", name: " 本地商品 ", description: " 本地描述 ", priceMinor: " 1990 ", currency: " cny ", stockQuantity: " 3 " };
    expect(productCreateRequest(draft)).toEqual({ product_code: "SKU-2", name: "本地商品", description: "本地描述", price_minor: 1990, currency: "CNY", stock_quantity: 3, images: [] });
    const calls: unknown[] = [];
    const created = { ...product, id: 2, product_code: "SKU-2", name: "本地商品", description: "本地描述", images: [] };
    const result = await createLocalProduct({ list: async () => ({ status: 200, data: { items: [] } }), get: async () => ({ status: 200, data: product }), create: async (request, options) => { calls.push([request, options]); return { status: 201, data: created }; } }, draft, "c".repeat(43), "product-create:123e4567-e89b-42d3-a456-426614174000");
    expect(result).toMatchObject({ status: "created", product: { id: 2, productCode: "SKU-2", currency: "CNY" } });
    expect(calls).toEqual([[{ product_code: "SKU-2", name: "本地商品", description: "本地描述", price_minor: 1990, currency: "CNY", stock_quantity: 3, images: [] }, { credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": "c".repeat(43), "Idempotency-Key": "product-create:123e4567-e89b-42d3-a456-426614174000" } }]]);
  });
  it("fails closed before POST for malformed drafts, CSRF, or idempotency and rejects malformed 201 mirrors", async () => {
    const draft = { productCode: "SKU", name: "商品", description: "", priceMinor: "1", currency: "CNY", stockQuantity: "0" };
    expect(productCreateRequest({ ...draft, stockQuantity: "2147483648" })).toBeUndefined();
    expect(productCreateRequest({ ...draft, priceMinor: "01" })).toBeUndefined();
    expect(productCreateRequest({ ...draft, currency: "CNYX" })).toBeUndefined();
    expect(productCreateRequest({ ...draft, name: "x".repeat(201) })).toBeUndefined();
    let posts = 0;
    const transport = { list: async () => ({ status: 200, data: { items: [] } }), get: async () => ({ status: 200, data: product }), create: async () => { posts++; return { status: 201, data: product }; } };
    expect((await createLocalProduct(transport, draft, "bad", "product-create:123e4567-e89b-42d3-a456-426614174000")).status).toBe("invalid");
    expect((await createLocalProduct(transport, draft, "c".repeat(43), "bad")).status).toBe("invalid");
    expect(posts).toBe(0);
    const badReceipt = { ...product, product_code: "SKU", images: ["opaque-image-value"] };
    const receiptTransport = { ...transport, create: async () => ({ status: 201, data: badReceipt }) };
    expect((await createLocalProduct(receiptTransport, draft, "c".repeat(43), "product-create:123e4567-e89b-42d3-a456-426614174000")).status).toBe("unknown");
    expect((await createLocalProduct({ ...transport, create: async () => ({ status: 201, data: { ...product, product_code: "SKU", images: [], unexpected: true } }) }, draft, "c".repeat(43), "product-create:123e4567-e89b-42d3-a456-426614174000")).status).toBe("unknown");
    for (const invalidTime of ["2026-02-30T00:00:00Z", "2026-01-01T24:00:00Z", "2026-01-01T00:60:00+24:60"]) {
      expect((await createLocalProduct({ ...transport, create: async () => ({ status: 201, data: { ...product, product_code: "SKU", images: [], created_at: invalidTime } }) }, draft, "c".repeat(43), "product-create:123e4567-e89b-42d3-a456-426614174000")).status).toBe("unknown");
      expect((await createLocalProduct({ ...transport, create: async () => ({ status: 201, data: { ...product, product_code: "SKU", images: [], updated_at: invalidTime } }) }, draft, "c".repeat(43), "product-create:123e4567-e89b-42d3-a456-426614174000")).status).toBe("unknown");
    }
  });
  it("maps closed write failures without retries and makes a valid unique key", async () => {
    const draft = { productCode: "SKU", name: "商品", description: "", priceMinor: "1", currency: "CNY", stockQuantity: "0" };
    const base = { list: async () => ({ status: 200, data: { items: [] } }), get: async () => ({ status: 200, data: product }) };
    for (const [status, expected] of [[400, "invalid"], [401, "unauthenticated"], [403, "forbidden"], [409, "conflict"], [503, "unknown"]] as const) {
      expect((await createLocalProduct({ ...base, create: async () => ({ status, data: {} }) }, draft, "c".repeat(43), "product-create:123e4567-e89b-42d3-a456-426614174000")).status).toBe(expected);
    }
    expect((await createLocalProduct({ ...base, create: async () => { throw new Error("offline"); } }, draft, "c".repeat(43), "product-create:123e4567-e89b-42d3-a456-426614174000")).status).toBe("unknown");
    expect(newProductIdempotencyKey({ randomUUID: () => "123e4567-e89b-42d3-a456-426614174000" })).toBe("product-create:123e4567-e89b-42d3-a456-426614174000");
    expect(newProductIdempotencyKey({ randomUUID: () => "not-a-uuid" })).toBeUndefined();
  });
  it("uses exact CAS and local-entitlement DTOs without accepting actor or caller-owned facts", async () => {
    const versioned = { ...product, version: 1 };
    const update = { expectedVersion: 1, name: "更新商品", description: "更新说明", priceMinor: "2", currency: "cny", stockQuantity: "3" };
    expect(productUpdateRequest(update)).toEqual({ expected_version: 1, name: "更新商品", description: "更新说明", price_minor: 2, currency: "CNY", stock_quantity: 3 });
    const calls: unknown[] = [];
    const entitlement = { id: 19, product_id: 1, order_id: 44, state: "active", version: 1, granted_at: "2026-08-20T09:00:00Z", revoked_at: null };
    const transport = {
      list: async () => ({ status: 200, data: { items: [versioned] } }), get: async () => ({ status: 200, data: versioned }),
      update: async (id: number, request: unknown, options: RequestInit) => { calls.push(["update", id, request, options]); return { status: 200, data: { ...versioned, name: "更新商品", description: "更新说明", price_minor: 2, stock_quantity: 3, version: 2 } }; },
      listEntitlements: async (id: number, params: unknown) => { calls.push(["list", id, params]); return { status: 200, data: { items: [entitlement] } }; },
      getEntitlement: async (id: number) => ({ status: 200, data: { ...entitlement, id } }),
      grantEntitlement: async (id: number, request: unknown, options: RequestInit) => { calls.push(["grant", id, request, options]); return { status: 201, data: entitlement }; },
      revokeEntitlement: async (id: number, request: unknown, options: RequestInit) => { calls.push(["revoke", id, request, options]); return { status: 200, data: { ...entitlement, state: "revoked", version: 2, revoked_at: "2026-08-20T10:00:00Z" } }; },
    };
    const csrf = "c".repeat(43); const key = "product-update:123e4567-e89b-42d3-a456-426614174000";
    expect(await updateLocalProduct(transport, { id: 1, productCode: "SKU-1", name: "商品", description: "本地描述", priceMinor: 1990, currency: "CNY", stockQuantity: 3, images: ["opaque-image-value"], createdBy: 7, createdAt: product.created_at, updatedAt: product.updated_at, version: 1 }, update, csrf, key)).toMatchObject({ status: "updated", product: { version: 2, name: "更新商品" } });
    expect(await loadProductLocalEntitlements(transport, 1)).toMatchObject({ status: "loaded", items: [{ orderId: 44 }] });
    expect(await loadProductLocalEntitlement(transport, 19)).toMatchObject({ status: "loaded", entitlement: { id: 19 } });
    expect(await grantProductLocalEntitlement(transport, 1, 44, csrf, "product-grant:123e4567-e89b-42d3-a456-426614174000")).toMatchObject({ status: "granted", entitlement: { productId: 1, orderId: 44 } });
    expect(await revokeProductLocalEntitlement(transport, { id: 19, productId: 1, orderId: 44, state: "active", version: 1, grantedAt: "2026-08-20T09:00:00Z" }, csrf, "product-revoke:123e4567-e89b-42d3-a456-426614174000")).toMatchObject({ status: "revoked", entitlement: { version: 2, state: "revoked" } });
    expect(calls).toContainEqual(["grant", 1, { order_id: 44 }, expect.objectContaining({ headers: expect.objectContaining({ "X-CSRF-Token": csrf }) })]);
    expect(calls).toContainEqual(["revoke", 19, { expected_version: 1 }, expect.anything()]);
    expect(newLocalProductIdempotencyKey("grant", { randomUUID: () => "123e4567-e89b-42d3-a456-426614174000" })).toBe("product-grant:123e4567-e89b-42d3-a456-426614174000");
    expect(await grantProductLocalEntitlement({ list: transport.list, get: transport.get, grantEntitlement: async () => ({ status: 201, data: { ...entitlement, granted_by: 7 } }) }, 1, 44, csrf, "product-grant:123e4567-e89b-42d3-a456-426614174000")).toEqual({ status: "unknown" });
  });
});
