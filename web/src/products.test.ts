import { describe, expect, it } from "vitest";
import { loadProducts, parseProductPage } from "./products";

const product = { id: 1, product_code: "SKU-1", name: "商品", description: "本地描述", price_minor: 1990, currency: "CNY", stock_quantity: 3, images: ["opaque-image-value"], created_by: 7, created_at: "2026-08-19T00:00:00Z", updated_at: "2026-08-19T00:00:00Z" };
describe("products", () => {
  it("accepts the closed local page and drops image metadata", () => {
    expect(parseProductPage({ items: [product] })).toEqual({ items: [{ id: 1, productCode: "SKU-1", name: "商品", description: "本地描述", priceMinor: 1990, currency: "CNY", stockQuantity: 3, createdAt: product.created_at, updatedAt: product.updated_at }] });
  });
  it("rejects extra fields, unsafe product values, and non-monotonic pages", () => {
    expect(parseProductPage({ items: [{ ...product, payment_url: "https://bad.example" }] })).toBeUndefined();
    expect(parseProductPage({ items: [{ ...product, images: [" https://bad.example"] }] })).toBeUndefined();
    expect(parseProductPage({ items: [{ ...product, price_minor: -1 }] })).toBeUndefined();
    expect(parseProductPage({ items: [{ ...product, stock_quantity: 2_147_483_648 }] })).toBeUndefined();
    expect(parseProductPage({ items: [product, { ...product, id: 1 }] })).toBeUndefined();
  });
  it("treats cursors as opaque and maps response failures", async () => {
    const calls: unknown[] = [];
    const transport = { list: async (params: unknown) => { calls.push(params); return { status: 200, data: { items: Array.from({ length: 50 }, (_, index) => ({ ...product, id: index + 1, product_code: `SKU-${index + 1}` })), next_cursor: "opaque-next" } }; } };
    expect((await loadProducts(transport, "opaque-input")).status).toBe("loaded");
    expect(calls).toEqual([{ limit: 50, cursor: "opaque-input" }]);
    expect((await loadProducts({ list: async () => ({ status: 401, data: {} }) })).status).toBe("unauthenticated");
  });
});
