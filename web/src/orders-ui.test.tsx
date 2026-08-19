import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import {
  OrdersContent,
  OrdersPage,
  canReadOrders,
  startOrdersLoad,
  type OrdersViewState,
} from "./orders-ui";
import { type OrdersTransport } from "./orders";

const page = {
  items: [{ orderNo: "M-1", payerName: "张三", mobile: "13800000000", productCode: "SKU-1", productName: "商品", amountYuan: "19.90", currency: "CNY", statusLabel: "已支付", providerLabel: "微信支付", createdAt: "2026-08-19T00:00:00Z" }],
  total: 1, offset: 0, hasMore: false,
} as const;

function client(response: () => Promise<{ status: number; data: unknown }>): OrdersTransport {
  return { list: vi.fn(response) } as unknown as OrdersTransport;
}

describe("order overview UI boundary", () => {
  it("renders only approved order fields and never turns a local projection into a link", () => {
    const state: OrdersViewState = { kind: "ready", page };
    const html = renderToStaticMarkup(<OrdersContent state={state} onLoad={vi.fn()} />);
    expect(html).toContain("订单号");
    expect(html).toContain("M-1");
    expect(html).toContain("张三");
    expect(html).toContain("13800000000");
    expect(html).not.toContain("detail_url");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("<a");
  });

  it("keeps the previous button enabled when offset 50 returns to offset 0", () => {
    const html = renderToStaticMarkup(
      <OrdersContent
        state={{ kind: "ready", page: { ...page, offset: 50, total: 51 } }}
        onLoad={vi.fn()}
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
