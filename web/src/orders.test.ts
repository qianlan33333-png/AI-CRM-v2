import { describe, expect, it, vi } from "vitest";
import {
  filterSafeOrders,
  filterSafeRefunds,
  loadOrderDetail,
  loadOrderItems,
  loadLocalRefunds,
  loadOrders,
  nextLocalRefundOffset,
  nextOrderOffset,
  previousLocalRefundOffset,
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
function detail(extra: Record<string, unknown> = {}) {
  return { id: 17, ...item(), refundable_amount_total: 1990, ...extra };
}
function refund(extra: Record<string, unknown> = {}) {
  return {
    id: 23, order_id: 17, provider: "wechat", order_no: "M-1", transaction_id: "WX-1",
    refund_id: "rfd_provider-1", out_refund_no: "rfd_local-1", refund_amount_total: 1990,
    currency: "CNY", reason: "重复支付", status: "pending_external_gate",
    external_effect_id: 31, external_effect_state: "pending_external_gate",
    auto_retry_allowed: false, created_at: "2026-08-19T00:00:00Z", ...extra,
  };
}
function refundPage(items: unknown[] = [refund()], extra: Record<string, unknown> = {}) {
  return { items, total: items.length, limit: 50, has_more: false, ...extra };
}
function transport(
  status: number,
  data: unknown,
  detailStatus = status,
  detailData = data,
  refundStatus = status,
  refundData = data,
  itemStatus = detailStatus,
  itemData = detailData,
): OrdersTransport {
  return {
    list: vi.fn(async () => ({ status, data })),
    detail: vi.fn(async () => ({ status: detailStatus, data: detailData })),
    items: vi.fn(async () => ({ status: itemStatus, data: itemData })),
    refunds: vi.fn(async () => ({ status: refundStatus, data: refundData })),
  } as unknown as OrdersTransport;
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
        items: [{ orderNo: "M-1", provider: "wechat", status: "paid", productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY", statusLabel: "已支付", providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z" }],
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

  it("uses one canonical same-origin detail GET and retains only mirrored local facts", async () => {
    const client = transport(200, page([item({ external_userid: "wmid-1" })]), 200, detail({ external_userid: "wmid-1" }));
    const loaded = await loadOrders(client);
    if (loaded.status !== "loaded") throw new Error("expected local item");
    await expect(loadOrderDetail(client, loaded.page.items[0])).resolves.toEqual({
      status: "loaded",
      detail: {
        id: 17, orderNo: "M-1", provider: "wechat",
        productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY",
        statusLabel: "已支付", providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z",
        refundableAmountTotal: 1990,
      },
    });
    expect(client.detail).toHaveBeenCalledWith(
      "M-1",
      { provider: "wechat" },
      { credentials: "same-origin" },
    );
  });

  it("fails closed for direct-detail envelopes, malformed fields, and safe-record drift", async () => {
    const loaded = await loadOrders(transport(200, page()));
    if (loaded.status !== "loaded") throw new Error("expected local item");
    const missingId: Record<string, unknown> = detail();
    delete missingId.id;
    for (const data of [
      { order: detail() }, missingId, detail({ unexpected: true }), detail({ id: 0 }),
      detail({ refundable_amount_total: 1991 }), detail({ refundable_amount_total: -1 }),
      detail({ order_no: "M-2" }),
      detail({ provider: "alipay" }), detail({ status: "created" }),
      detail({ external_userid: "wmid-1", unionid: "u-1" }),
    ]) {
      await expect(loadOrderDetail(transport(200, page(), 200, data), loaded.page.items[0]))
        .resolves.toEqual({ status: "invalid" });
    }
  });

  it("drops opaque identity fields from the validated list and detail projection", async () => {
    const listed = await loadOrders(transport(200, page([item({ external_userid: "wmid-1" })])));
    if (listed.status !== "loaded") throw new Error("expected local item");
    await expect(loadOrderDetail(transport(200, page(), 200, detail({ external_userid: "wmid-2" })), listed.page.items[0]))
      .resolves.toMatchObject({ status: "loaded" });
  });

  it("keeps accepted local detail-url text inert without inventing stricter URL semantics", async () => {
    for (const detailUrl of ["/api/orders?cursor=1", "/api/orders#local", "/api/%2ftext", "/api\\local", "/api/../local"]) {
      const loaded = await loadOrders(transport(200, page([item({ detail_url: detailUrl })])));
      if (loaded.status !== "loaded") throw new Error("expected local item");
      await expect(loadOrderDetail(transport(200, page(), 200, detail({ detail_url: detailUrl })), loaded.page.items[0]))
        .resolves.toMatchObject({ status: "loaded" });
    }
  });

  it("maps detail failures without retries", async () => {
    const loaded = await loadOrders(transport(200, page()));
    if (loaded.status !== "loaded") throw new Error("expected local item");
    for (const [status, expected] of [[401, "unauthenticated"], [403, "forbidden"], [404, "invalid"], [503, "unavailable"]] as const) {
      const client = transport(200, page(), status, {});
      await expect(loadOrderDetail(client, loaded.page.items[0])).resolves.toEqual({ status: expected });
      expect(client.detail).toHaveBeenCalledOnce();
    }
  });

  it("uses exactly the fixed same-origin local refund-history GET", async () => {
    const client = transport(200, page(), 200, detail(), 200, refundPage());
    await expect(loadLocalRefunds(client)).resolves.toEqual({
      status: "loaded",
      page: {
        items: [{
          id: 23, provider: "wechat", orderNo: "M-1", refundAmountTotal: 1990,
          currency: "CNY", status: "pending_external_gate", externalEffectState: "pending_external_gate",
          createdAt: "2026-08-19T00:00:00Z",
        }],
        total: 1, offset: 0, hasMore: false,
      },
    });
    expect(client.refunds).toHaveBeenCalledWith(
      { provider: "all", limit: 50, offset: 0 },
      { credentials: "same-origin" },
    );
  });

  it("reads the existing one-item snapshot and projects only safe product fields", async () => {
    const client = transport(200, page(), 200, detail(), 200, refundPage(), 200, { items: [item({ external_userid: "wmid-hidden" })] });
    const listed = await loadOrders(client);
    if (listed.status !== "loaded") throw new Error("expected order");
    await expect(loadOrderItems(client, listed.page.items[0])).resolves.toEqual({
      status: "loaded",
      item: { orderNo: "M-1", provider: "wechat", productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY", createdAt: "2026-08-19T00:00:00Z" },
    });
    expect(client.items).toHaveBeenCalledWith("M-1", { provider: "wechat" }, { credentials: "same-origin" });
  });

  it("fails closed for item-envelope drift and filters only existing safe page facts", async () => {
    const client = transport(200, page());
    const listed = await loadOrders(client);
    if (listed.status !== "loaded") throw new Error("expected order");
    await expect(loadOrderItems(transport(200, page(), 200, detail(), 200, refundPage(), 200, { items: [item()], extra: true }), listed.page.items[0])).resolves.toEqual({ status: "invalid" });
    expect(filterSafeOrders(listed.page.items, { keyword: "SKU", provider: "wechat", status: "paid" })).toHaveLength(1);
    expect(filterSafeOrders(listed.page.items, { keyword: "张三", provider: "all", status: "" })).toHaveLength(0);
    const local = await loadLocalRefunds(transport(200, page(), 200, detail(), 200, refundPage()));
    if (local.status !== "loaded") throw new Error("expected refunds");
    expect(filterSafeRefunds(local.page.items, { keyword: "M-1", provider: "wechat", status: "pending_external_gate" })).toHaveLength(1);
  });

  it("fails closed for all local refund DTO, page, and external-effect contract drift", async () => {
    const invalid = [
      refund({ unexpected: true }), refund({ id: 0 }), refund({ order_id: 0 }),
      refund({ provider: "stripe" }), refund({ order_no: "" }), refund({ transaction_id: "" }),
      refund({ refund_id: "legacy-1" }), refund({ out_refund_no: "legacy-1" }),
      refund({ refund_amount_total: 0 }), refund({ currency: "USD" }),
      refund({ reason: "" }), refund({ status: "succeeded" }),
      refund({ external_effect_state: "succeeded" }), refund({ auto_retry_allowed: true }),
      refund({ created_at: "2026-02-31T00:00:00Z" }),
    ];
    for (const data of [
      ...invalid.map((entry) => refundPage([entry])),
      refundPage([refund(), refund({ id: 24 })]),
      refundPage([refund(), refund({ id: 24, out_refund_no: "rfd_local-2" })]),
      refundPage([refund(), refund({ id: 24, refund_id: "rfd_provider-2" })]),
      refundPage([], { total: 1, has_more: true }),
      refundPage([refund()], { total: 2, has_more: false }), refundPage([refund()], { limit: 20 }),
      { ...refundPage(), unexpected: true },
    ]) {
      await expect(loadLocalRefunds(transport(200, page(), 200, detail(), 200, data)))
        .resolves.toEqual({ status: "invalid" });
    }
  });

  it("maps local refund-history failures without retry and keeps its offset boundary", async () => {
    for (const [status, expected] of [[401, "unauthenticated"], [403, "forbidden"], [503, "unavailable"]] as const) {
      const client = transport(200, page(), 200, detail(), status, {});
      await expect(loadLocalRefunds(client)).resolves.toEqual({ status: expected });
      expect(client.refunds).toHaveBeenCalledOnce();
    }
    await expect(loadLocalRefunds(transport(200, page(), 200, detail(), 200, refundPage()), 1))
      .resolves.toEqual({ status: "invalid" });
    const localPage = { items: [], total: 51, offset: 50, hasMore: false };
    expect(previousLocalRefundOffset(localPage)).toBe(0);
    expect(nextLocalRefundOffset({ ...localPage, offset: 0, hasMore: true })).toBe(50);
  });
});
