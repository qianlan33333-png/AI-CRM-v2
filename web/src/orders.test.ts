import { describe, expect, it, vi } from "vitest";
import {
  loadOrders,
  nextOrderOffset,
  previousOrderOffset,
  type OrdersTransport,
} from "./orders";

function item(extra: Record<string, unknown> = {}) {
  return {
    created_at: "2026-08-19T00:00:00Z", merchant_order_no: "M-1", out_trade_no: "M-1",
    order_no: "M-1", platform_transaction_no: "WX-1", transaction_id: "WX-1",
    payer_name: "张三", mobile: "13800000000", product_code: "SKU-1", product_name: "商品",
    amount_yuan: "19.90", currency: "CNY", status: "paid", status_label: "已支付",
    provider: "wechat", provider_label: "微信支付", detail_url: "/api/admin/orders/1", ...extra,
  };
}
function page(items: unknown[] = [item()], extra: Record<string, unknown> = {}) {
  return { items, total: items.length, limit: 50, has_more: false, ...extra };
}
function transport(status: number, data: unknown): OrdersTransport {
  return { list: vi.fn(async () => ({ status, data })) } as unknown as OrdersTransport;
}

describe("local order overview read boundary", () => {
  it("uses only the canonical same-origin order list request", async () => {
    const client = transport(200, page());
    await expect(loadOrders(client)).resolves.toMatchObject({ status: "loaded" });
    expect(client.list).toHaveBeenCalledWith(
      { provider: "all", limit: 50, offset: 0 },
      { credentials: "same-origin" },
    );
  });

  it("accepts a full persisted projection while hiding identity and detail-url fields", async () => {
    const client = transport(200, page([item({ external_userid: "wmid-1" })]));
    await expect(loadOrders(client)).resolves.toEqual({
      status: "loaded",
      page: {
        items: [{ orderNo: "M-1", payerName: "张三", mobile: "13800000000", productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY", statusLabel: "已支付", providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z" }],
        total: 1, offset: 0, hasMore: false,
      },
    });
  });

  it("fails closed for malformed pages and unsafe or extra persisted facts", async () => {
    const duplicate = [item(), item({ merchant_order_no: "M-2" })];
    for (const data of [
      page([item({ unexpected: true })]), page([item({ amount_yuan: "19.9" })]),
      page([item({ created_at: "2026-02-31T00:00:00Z" })]), page([item({ provider: "stripe" })]),
      page([item({ detail_url: "//outside.example/orders/1" })]),
      page([item({ userid: "a", unionid: "b" })]), page(duplicate, { total: 2 }),
      page([item()], { total: 2, has_more: false }), page([item()], { limit: 20 }),
      page([], { total: 1, has_more: true }),
      { ...page(), extra: true },
    ]) {
      await expect(loadOrders(transport(200, data))).resolves.toEqual({ status: "invalid" });
    }
  });

  it("maps read failures without retry and preserves only safe offset navigation", async () => {
    await expect(loadOrders(transport(401, {}))).resolves.toEqual({ status: "unauthenticated" });
    await expect(loadOrders(transport(403, {}))).resolves.toEqual({ status: "forbidden" });
    await expect(loadOrders(transport(503, {}))).resolves.toEqual({ status: "unavailable" });
    await expect(loadOrders(transport(200, page()), 1)).resolves.toEqual({ status: "invalid" });
    expect(previousOrderOffset({ items: [], total: 100, offset: 50, hasMore: true })).toBe(0);
    expect(nextOrderOffset({ items: [], total: 100, offset: 0, hasMore: true })).toBe(50);
  });
});
