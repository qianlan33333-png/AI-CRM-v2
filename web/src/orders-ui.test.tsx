import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import {
  OrdersContent,
  OrdersPage,
  canReadOrders,
  startOrderDetailLoad,
  startOrdersLoad,
  type OrderDetailViewState,
  type OrdersViewState,
} from "./orders-ui";
import { type OrdersTransport } from "./orders";

const page = {
  items: [{ orderNo: "M-1", merchantOrderNo: "M-1", outTradeNo: "M-1", platformTransactionNo: "WX-1", transactionId: "WX-1", detailUrl: "/api/admin/orders/M-1", provider: "wechat", status: "paid", payerName: "张三", mobile: "13800000000", productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY", statusLabel: "已支付", providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z" }],
  total: 1, offset: 0, hasMore: false,
} as const;

function detail(extra: Record<string, unknown> = {}) {
  return {
    id: 17, created_at: "2026-08-19T00:00:00Z", merchant_order_no: "M-1", out_trade_no: "M-1",
    order_no: "M-1", platform_transaction_no: "WX-1", transaction_id: "WX-1", payer_name: "张三",
    mobile: "13800000000", product_code: "SKU-1", product_name: "商品", amount_yuan: "19.90",
    currency: "CNY", status: "paid", status_label: "已支付", provider: "wechat", provider_label: "微信支付",
    detail_url: "/api/admin/orders/M-1", refundable_amount_total: 1990, ...extra,
  };
}

type TransportResponse = { status: number; data: unknown };

function client(
  response: () => Promise<TransportResponse>,
  detailResponse: () => Promise<TransportResponse> = async () => ({ status: 200, data: detail() }),
): OrdersTransport {
  return { list: vi.fn(response), detail: vi.fn(detailResponse) } as unknown as OrdersTransport;
}

describe("order overview UI boundary", () => {
  it("renders only approved order fields and never turns a local projection into a link", () => {
    const state: OrdersViewState = { kind: "ready", page };
    const html = renderToStaticMarkup(<OrdersContent state={state} detail={{ kind: "idle" }} onLoad={vi.fn()} onLoadDetail={vi.fn()} />);
    expect(html).toContain("订单号");
    expect(html).toContain("M-1");
    expect(html).toContain("张三");
    expect(html).toContain("13800000000");
    expect(html).not.toContain("detail_url");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("transaction_id");
    expect(html).not.toContain("退款已经执行");
    expect(html).not.toContain("<a");
  });

  it("keeps the previous button enabled when offset 50 returns to offset 0", () => {
    const html = renderToStaticMarkup(
      <OrdersContent
        state={{ kind: "ready", page: { ...page, offset: 50, total: 51 } }}
        detail={{ kind: "idle" }}
        onLoad={vi.fn()}
        onLoadDetail={vi.fn()}
      />,
    );
    expect(html).toContain('<button type="button">上一页</button>');
  });

  it("keeps admin and ops reads local while sales issues no transport request", () => {
    const response = async () => ({ status: 503, data: {} });
    const transport = client(response);
    const admin = renderToStaticMarkup(<OrdersPage role="admin" transport={transport} />);
    const ops = renderToStaticMarkup(<OrdersPage role="ops" transport={transport} />);
    const sales = renderToStaticMarkup(<OrdersPage role="sales" transport={transport} />);
    expect(admin).toContain("正在读取本地订单总览。");
    expect(ops).toContain("正在读取本地订单总览。");
    expect(sales).not.toContain("正在读取本地订单总览。");
    expect(transport.list).not.toHaveBeenCalled();
  });

});

describe("order overview load transition", () => {
  function controller(
    role: "admin" | "ops" | "sales",
    transport: OrdersTransport,
    previous = undefined as typeof page | undefined,
  ) {
    return {
      role,
      offset: 0,
      transport,
      generation: { current: 0 },
      inFlight: { current: false },
      verified: { current: previous },
      setState: vi.fn(),
    };
  }

  it.each(["admin", "ops"] as const)("issues the canonical local GET for %s", async (role) => {
    const transport = client(async () => ({ status: 200, data: { items: [], total: 0, limit: 50, has_more: false } }));
    const state = controller(role, transport);
    await startOrdersLoad(state);
    expect(transport.list).toHaveBeenCalledWith(
      { provider: "all", limit: 50, offset: 0 },
      { credentials: "same-origin" },
    );
    expect(state.setState).toHaveBeenLastCalledWith({ kind: "ready", page: { items: [], total: 0, offset: 0, hasMore: false } });
  });

  it("keeps sales fail-closed without a transport call", () => {
    const transport = client(async () => ({ status: 200, data: {} }));
    const state = controller("sales", transport);
    expect(canReadOrders("sales")).toBe(false);
    expect(startOrdersLoad(state)).toBeUndefined();
    expect(transport.list).not.toHaveBeenCalled();
  });

  it("locks a second interaction, discards stale data, and releases the lock", async () => {
    // eslint-disable-next-line no-unused-vars -- promise resolver parameter is intentionally invoked by the test.
    let resolve: ((value: { status: number; data: unknown }) => void) | undefined;
    const transport = client(() => new Promise((done) => { resolve = done; }));
    const state = controller("admin", transport);
    const first = startOrdersLoad(state);
    expect(startOrdersLoad(state)).toBeUndefined();
    expect(transport.list).toHaveBeenCalledOnce();
    state.generation.current += 1;
    resolve?.({ status: 200, data: { items: [], total: 0, limit: 50, has_more: false } });
    await first;
    expect(state.setState).toHaveBeenCalledTimes(1);
    expect(state.inFlight.current).toBe(false);
  });

  it("retains the verified page on a local failure and calls back on 401", async () => {
    const unavailable = controller("admin", client(async () => ({ status: 503, data: {} })), page);
    await startOrdersLoad(unavailable);
    expect(unavailable.setState).toHaveBeenLastCalledWith({ kind: "error", failure: "unavailable", previous: page });

    const onUnauthenticated = vi.fn();
    const unauthenticated = { ...controller("admin", client(async () => ({ status: 401, data: {} })), page), onUnauthenticated };
    await startOrdersLoad(unauthenticated);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(unauthenticated.setState).toHaveBeenLastCalledWith({ kind: "error", failure: "unauthenticated", previous: page });
  });
});

describe("local order detail transition", () => {
  function controller(
    role: "admin" | "ops" | "sales",
    transport: OrdersTransport,
    previous = undefined as undefined | { readonly id: number; readonly orderNo: string; readonly provider: "wechat"; readonly payerName: string; readonly mobile: string; readonly productCode: string; readonly productName: string; readonly amountYuan: string; readonly currency: string; readonly statusLabel: string; readonly providerLabel: string; readonly createdAt: string; readonly refundableAmountTotal: number },
  ) {
    return {
      role,
      item: page.items[0],
      transport,
      generation: { current: 0 },
      inFlight: { current: new Map<string, symbol>() },
      verified: { current: previous },
      setState: vi.fn(),
    };
  }

  it.each(["admin", "ops"] as const)("issues exactly one local detail GET for %s", async (role) => {
    const transport = client(async () => ({ status: 200, data: { items: [], total: 0, limit: 50, has_more: false } }));
    const state = controller(role, transport);
    await startOrderDetailLoad(state);
    expect(transport.detail).toHaveBeenCalledWith(
      "M-1",
      { provider: "wechat" },
      { credentials: "same-origin" },
    );
    expect(state.setState).toHaveBeenLastCalledWith({
      kind: "ready",
      detail: {
        id: 17, orderNo: "M-1", provider: "wechat", payerName: "张三", mobile: "13800000000",
        productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY",
        statusLabel: "已支付", providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z",
        refundableAmountTotal: 1990,
      },
    });
  });

  it("keeps sales fail-closed with no detail transport call", () => {
    const transport = client(async () => ({ status: 200, data: {} }));
    const state = controller("sales", transport);
    expect(startOrderDetailLoad(state)).toBeUndefined();
    expect(transport.detail).not.toHaveBeenCalled();
  });

  it("cleans each order token when concurrent orders resolve out of order", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver argument is intentionally supplied by the Promise implementation.
    const resolvers: ((value: { status: number; data: unknown }) => void)[] = [];
    const transport = client(
      async () => ({ status: 200, data: { items: [], total: 0, limit: 50, has_more: false } }),
      () => new Promise((done) => { resolvers.push(done); }),
    );
    const firstState = controller("admin", transport);
    const secondState = { ...firstState, item: { ...page.items[0], orderNo: "M-2" } };
    const first = startOrderDetailLoad(firstState);
    const second = startOrderDetailLoad(secondState);
    expect(startOrderDetailLoad(firstState)).toBeUndefined();
    expect(transport.detail).toHaveBeenCalledTimes(2);
    resolvers[1]?.({ status: 200, data: detail({ merchant_order_no: "M-2", out_trade_no: "M-2", order_no: "M-2" }) });
    await second;
    expect(firstState.inFlight.current.has("M-1")).toBe(true);
    expect(firstState.inFlight.current.has("M-2")).toBe(false);
    resolvers[0]?.({ status: 200, data: detail() });
    await first;
    expect(firstState.inFlight.current.size).toBe(0);
  });

  it("does not let an old response release a new same-order request after reload", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver argument is intentionally supplied by the Promise implementation.
    const resolvers: ((value: { status: number; data: unknown }) => void)[] = [];
    const transport = client(
      async () => ({ status: 200, data: { items: [], total: 0, limit: 50, has_more: false } }),
      () => new Promise((done) => { resolvers.push(done); }),
    );
    const state = controller("admin", transport);
    const oldRequest = startOrderDetailLoad(state);
    state.generation.current += 1;
    state.inFlight.current = new Map();
    const newRequest = startOrderDetailLoad(state);
    expect(transport.detail).toHaveBeenCalledTimes(2);
    resolvers[0]?.({ status: 200, data: detail() });
    await oldRequest;
    expect(state.inFlight.current.has("M-1")).toBe(true);
    resolvers[1]?.({ status: 200, data: detail() });
    await newRequest;
    expect(state.inFlight.current.has("M-1")).toBe(false);
  });

  it("retains a verified same-order detail on failure and calls back on 401", async () => {
    const previous = {
      id: 17, orderNo: "M-1", provider: "wechat" as const, payerName: "张三", mobile: "13800000000",
      productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY", statusLabel: "已支付",
      providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z", refundableAmountTotal: 1990,
    };
    const unavailable = controller("admin", client(async () => ({ status: 200, data: {} }), async () => ({ status: 503, data: {} })), previous);
    await startOrderDetailLoad(unavailable);
    expect(unavailable.setState).toHaveBeenLastCalledWith({ kind: "error", orderNo: "M-1", failure: "unavailable", previous });

    const onUnauthenticated = vi.fn();
    const unauthenticated = { ...controller("admin", client(async () => ({ status: 200, data: {} }), async () => ({ status: 401, data: {} })), previous), onUnauthenticated };
    await startOrderDetailLoad(unauthenticated);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(unauthenticated.setState).toHaveBeenLastCalledWith({ kind: "error", orderNo: "M-1", failure: "unauthenticated", previous });
  });

  it("renders verified local fields only, with no identity, provider URL, or action surface", () => {
    const detailState: OrderDetailViewState = {
      kind: "ready",
      detail: {
        id: 17, orderNo: "M-1", provider: "wechat", payerName: "张三", mobile: "13800000000",
        productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY", statusLabel: "已支付",
        providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z", refundableAmountTotal: 1990,
      },
    };
    const html = renderToStaticMarkup(<OrdersContent state={{ kind: "ready", page }} detail={detailState} onLoad={vi.fn()} onLoadDetail={vi.fn()} />);
    expect(html).toContain("本地可退金额（分）");
    expect(html).toContain("不代表退款已经执行");
    expect(html).not.toContain("detail_url");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("<a");
    expect(html).not.toContain("退款申请");
  });
});
