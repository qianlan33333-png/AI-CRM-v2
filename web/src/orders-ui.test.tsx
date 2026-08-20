import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import {
  OrdersContent,
  OrdersPage,
  canReadOrders,
  startOrderDetailLoad,
  startOrderItemsLoad,
  startLocalRefundLoad,
  startLocalOrderExport,
  startOrdersLoad,
  type LocalRefundViewState,
  type OrderDetailViewState,
  type OrdersViewState,
} from "./orders-ui";
import { type OrdersTransport } from "./orders";

function exportReceipt(extra: Record<string, unknown> = {}) {
  return {
    job_id: "exp_0000000000000001",
    resource: "orders",
    format: "csv",
    status: "completed",
    created_at: "2026-08-19T00:00:00Z",
    operator: 7,
    download_url: "/api/admin/exports/exp_0000000000000001",
    content_type: "text/csv",
    file_name: "exp_0000000000000001.csv",
    content_text: "order_no,transaction_id\nM-1,WX-1\n",
    ...extra,
  };
}

const page = {
  items: [
    {
      orderNo: "M-1",
      merchantOrderNo: "M-1",
      outTradeNo: "M-1",
      platformTransactionNo: "WX-1",
      transactionId: "WX-1",
      detailUrl: "/api/admin/orders/M-1",
      provider: "wechat",
      status: "paid",
      payerName: "张三",
      mobile: "13800000000",
      productCode: "SKU-1",
      productName: "商品",
      amountYuan: "19.90",
      currency: "CNY",
      statusLabel: "已支付",
      providerLabel: "微信支付",
      createdAt: "2026-08-19T00:00:00Z",
    },
  ],
  total: 1,
  offset: 0,
  hasMore: false,
  providerScope: "all",
} as const;

function detail(extra: Record<string, unknown> = {}) {
  return {
    id: 17,
    created_at: "2026-08-19T00:00:00Z",
    merchant_order_no: "M-1",
    out_trade_no: "M-1",
    order_no: "M-1",
    platform_transaction_no: "WX-1",
    transaction_id: "WX-1",
    payer_name: "张三",
    mobile: "13800000000",
    product_code: "SKU-1",
    product_name: "商品",
    amount_yuan: "19.90",
    currency: "CNY",
    status: "paid",
    status_label: "已支付",
    provider: "wechat",
    provider_label: "微信支付",
    detail_url: "/api/admin/orders/M-1",
    refundable_amount_total: 1990,
    ...extra,
  };
}

function itemSnapshot(extra: Record<string, unknown> = {}) {
  return {
    items: [
      {
        created_at: "2026-08-19T00:00:00Z",
        merchant_order_no: "M-1",
        out_trade_no: "M-1",
        order_no: "M-1",
        platform_transaction_no: "WX-1",
        transaction_id: "WX-1",
        payer_name: "张三",
        mobile: "13800000000",
        product_code: "SKU-1",
        product_name: "商品",
        amount_yuan: "19.90",
        currency: "CNY",
        status: "paid",
        status_label: "已支付",
        provider: "wechat",
        provider_label: "微信支付",
        detail_url: "/api/admin/orders/M-1",
        ...extra,
      },
    ],
  };
}

const refunds = {
  items: [
    {
      id: 23,
      orderID: 17,
      provider: "wechat" as const,
      orderNo: "M-1",
      transactionID: "WX-1",
      refundID: "rfd_provider-1",
      outRefundNo: "rfd_local-1",
      refundAmountTotal: 1990,
      currency: "CNY" as const,
      reason: "重复支付",
      status: "completed" as const,
      externalEffectID: 31,
      externalEffectState: "completed" as const,
      autoRetryAllowed: false as const,
      createdAt: "2026-08-19T00:00:00Z",
    },
  ],
  total: 1,
  offset: 0,
  hasMore: false,
} as const;

type TransportResponse = { status: number; data: unknown };

function client(
  response: () => Promise<TransportResponse>,
  detailResponse: () => Promise<TransportResponse> = async () => ({
    status: 200,
    data: detail(),
  }),
  refundResponse: () => Promise<TransportResponse> = async () => ({
    status: 200,
    data: {},
  }),
  itemResponse: () => Promise<TransportResponse> = async () => ({
    status: 200,
    data: itemSnapshot(),
  }),
): OrdersTransport {
  return {
    list: vi.fn(response),
    detail: vi.fn(detailResponse),
    items: vi.fn(itemResponse),
    refunds: vi.fn(refundResponse),
  } as unknown as OrdersTransport;
}

describe("order overview UI boundary", () => {
  it("renders only approved order fields and never turns a local projection into a link", () => {
    const state: OrdersViewState = { kind: "ready", page };
    const html = renderToStaticMarkup(
      <OrdersContent
        state={state}
        detail={{ kind: "idle" }}
        refunds={{ kind: "loading" }}
        onLoad={vi.fn()}
        onLoadDetail={vi.fn()}
        onLoadRefunds={vi.fn()}
      />,
    );
    expect(html).toContain("订单号");
    expect(html).toContain("M-1");
    expect(html).not.toContain("张三");
    expect(html).not.toContain("13800000000");
    expect(html).not.toContain("detail_url");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("transaction_id");
    expect(html).not.toContain("退款已经执行");
    expect(html).not.toContain("<a");
  });

  it("renders safe local export metadata without CSV content or a download surface", () => {
    const html = renderToStaticMarkup(
      <OrdersContent
        state={{ kind: "ready", page }}
        detail={{ kind: "idle" }}
        refunds={{ kind: "loading" }}
        exportState={{
          kind: "completed",
          value: {
            jobID: "exp_0000000000000001",
            resource: "orders",
            createdAt: "2026-08-19T00:00:00Z",
            fileName: "exp_0000000000000001.csv",
          },
        }}
        onLoad={vi.fn()}
        onLoadDetail={vi.fn()}
        onLoadRefunds={vi.fn()}
      />,
    );
    expect(html).toContain("本地导出已确认");
    expect(html).toContain("exp_0000000000000001.csv");
    expect(html).not.toContain("WX-1");
    expect(html).not.toContain("download_url");
    expect(html).not.toContain("/api/admin/exports/");
    expect(html).not.toContain("<a");
  });

  it("renders a local refund record without creating a refund or claiming provider success", () => {
    const state: OrdersViewState = { kind: "ready", page };
    const refundState: LocalRefundViewState = { kind: "ready", page: refunds };
    const html = renderToStaticMarkup(
      <OrdersContent
        state={state}
        detail={{ kind: "idle" }}
        refunds={refundState}
        onLoad={vi.fn()}
        onLoadDetail={vi.fn()}
        onLoadRefunds={vi.fn()}
      />,
    );
    expect(html).toContain("本地退款意图历史");
    expect(html).not.toContain("rfd_provider-1");
    expect(html).toContain("本地外效状态");
    expect(html).toContain("不代表支付渠道已执行、送达或退款成功");
    expect(html).not.toContain("transaction_id");
    expect(html).not.toContain("重复支付");
    expect(html).not.toContain("退款申请");
    expect(html).not.toContain("重试退款");
    expect(html).not.toContain("<a");
  });

  it("renders the local product snapshot without retaining identity or provider metadata", () => {
    const html = renderToStaticMarkup(
      <OrdersContent
        state={{ kind: "ready", page }}
        detail={{ kind: "idle" }}
        items={{
          kind: "ready",
          item: {
            orderNo: "M-1",
            provider: "wechat",
            productCode: "SKU-1",
            productName: "商品",
            amountYuan: "19.90",
            currency: "CNY",
            createdAt: "2026-08-19T00:00:00Z",
          },
        }}
        refunds={{ kind: "loading" }}
        onLoad={vi.fn()}
        onLoadDetail={vi.fn()}
        onLoadItems={vi.fn()}
        onLoadRefunds={vi.fn()}
      />,
    );
    expect(html).toContain("本地购买商品项");
    expect(html).toContain("商品项仅为已持久化的本地购买快照");
    expect(html).not.toContain("张三");
    expect(html).not.toContain("13800000000");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("detail_url");
  });

  it("keeps the previous button enabled when offset 50 returns to offset 0", () => {
    const html = renderToStaticMarkup(
      <OrdersContent
        state={{ kind: "ready", page: { ...page, offset: 50, total: 51 } }}
        detail={{ kind: "idle" }}
        refunds={{ kind: "loading" }}
        onLoad={vi.fn()}
        onLoadDetail={vi.fn()}
        onLoadRefunds={vi.fn()}
      />,
    );
    expect(html).toContain('<button type="button">上一页</button>');
  });

  it("keeps admin and ops reads local while sales issues no transport request", () => {
    const response = async () => ({ status: 503, data: {} });
    const transport = client(response);
    const admin = renderToStaticMarkup(
      <OrdersPage role="admin" transport={transport} />,
    );
    const ops = renderToStaticMarkup(
      <OrdersPage role="ops" transport={transport} />,
    );
    const sales = renderToStaticMarkup(
      <OrdersPage role="sales" transport={transport} />,
    );
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
      inFlight: { current: undefined as symbol | undefined },
      verified: { current: previous },
      setState: vi.fn(),
    };
  }

  it.each(["admin", "ops"] as const)(
    "issues the canonical local GET for %s",
    async (role) => {
      const transport = client(async () => ({
        status: 200,
        data: { items: [], total: 0, limit: 50, has_more: false },
      }));
      const state = controller(role, transport);
      await startOrdersLoad(state);
      expect(transport.list).toHaveBeenCalledWith(
        { provider: "all", limit: 50, offset: 0 },
        { credentials: "same-origin" },
      );
      expect(state.setState).toHaveBeenLastCalledWith({
        kind: "ready",
        page: {
          items: [],
          total: 0,
          offset: 0,
          hasMore: false,
          providerScope: "all",
        },
      });
    },
  );

  it("keeps sales fail-closed without a transport call", () => {
    const transport = client(async () => ({ status: 200, data: {} }));
    const state = controller("sales", transport);
    expect(canReadOrders("sales")).toBe(false);
    expect(startOrdersLoad(state)).toBeUndefined();
    expect(transport.list).not.toHaveBeenCalled();
  });

  it("locks a second interaction, discards stale data, and releases the lock", async () => {
    let resolve:
      // eslint-disable-next-line no-unused-vars -- promise resolver parameter is intentionally invoked by the test.
      ((value: { status: number; data: unknown }) => void) | undefined;
    const transport = client(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const state = controller("admin", transport);
    const first = startOrdersLoad(state);
    expect(startOrdersLoad(state)).toBeUndefined();
    expect(transport.list).toHaveBeenCalledOnce();
    state.generation.current += 1;
    resolve?.({
      status: 200,
      data: { items: [], total: 0, limit: 50, has_more: false },
    });
    await first;
    expect(state.setState).toHaveBeenCalledTimes(1);
    expect(state.inFlight.current).toBeUndefined();
  });

  it("retains the verified page on a local failure and calls back on 401", async () => {
    const unavailable = controller(
      "admin",
      client(async () => ({ status: 503, data: {} })),
      page,
    );
    await startOrdersLoad(unavailable);
    expect(unavailable.setState).toHaveBeenLastCalledWith({
      kind: "error",
      failure: "unavailable",
      previous: page,
    });

    const onUnauthenticated = vi.fn();
    const unauthenticated = {
      ...controller(
        "admin",
        client(async () => ({ status: 401, data: {} })),
        page,
      ),
      onUnauthenticated,
    };
    await startOrdersLoad(unauthenticated);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(unauthenticated.setState).toHaveBeenLastCalledWith({
      kind: "error",
      failure: "unauthenticated",
      previous: page,
    });
  });

  it("lets a replacement provider read start and keeps its token when old finally resolves", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver parameter is supplied by the deferred request.
    let resolveA: ((value: TransportResponse) => void) | undefined;
    // eslint-disable-next-line no-unused-vars -- resolver parameter is supplied by the deferred request.
    let resolveB: ((value: TransportResponse) => void) | undefined;
    const generation = { current: 0 };
    const inFlight = { current: undefined as symbol | undefined };
    const verified = { current: undefined as typeof page | undefined };
    const first = startOrdersLoad({
      role: "admin",
      offset: 0,
      providerScope: "all",
      transport: client(
        () =>
          new Promise((done) => {
            resolveA = done;
          }),
      ),
      generation,
      inFlight,
      verified,
      setState: vi.fn(),
    });
    generation.current += 1;
    inFlight.current = undefined;
    const setStateB = vi.fn();
    const second = startOrdersLoad({
      role: "admin",
      offset: 0,
      providerScope: "alipay",
      transport: client(
        () =>
          new Promise((done) => {
            resolveB = done;
          }),
      ),
      generation,
      inFlight,
      verified,
      setState: setStateB,
    });
    const tokenB = inFlight.current;
    resolveA?.({ status: 401, data: {} });
    await first;
    expect(inFlight.current).toBe(tokenB);
    resolveB?.({
      status: 200,
      data: { items: [], total: 0, limit: 50, has_more: false },
    });
    await second;
    expect(setStateB).toHaveBeenLastCalledWith({
      kind: "ready",
      page: {
        items: [],
        total: 0,
        offset: 0,
        hasMore: false,
        providerScope: "alipay",
      },
    });
    expect(inFlight.current).toBeUndefined();
  });
});

describe("local order export transition", () => {
  function exportController(
    role: "admin" | "ops" | "sales",
    transport: OrdersTransport,
  ) {
    return {
      role,
      resource: "orders" as const,
      transport,
      readCookie: () => `aicrm_csrf=${"c".repeat(43)}`,
      generation: { current: 0 },
      inFlight: { current: undefined as symbol | undefined },
      outcomeUnknown: { current: false },
      setState: vi.fn(),
      onUnauthenticated: vi.fn(),
    };
  }

  it.each(["admin", "ops"] as const)(
    "creates and rereads once for %s",
    async (role) => {
      const createExport = vi.fn(
        async (_request: unknown, _options?: RequestInit) => {
          void _request; void _options;
          return { status: 200, data: exportReceipt() };
        },
      );
      const getExport = vi.fn(
        async (_jobID: string, _options?: RequestInit) => {
          void _jobID; void _options;
          return { status: 200, data: exportReceipt() };
        },
      );
      const state = exportController(role, {
        ...client(async () => ({ status: 200, data: {} })),
        createExport,
        getExport,
      } as OrdersTransport);
      await startLocalOrderExport(state);
      expect(createExport).toHaveBeenCalledOnce();
      expect(getExport).toHaveBeenCalledOnce();
      const options = createExport.mock.calls[0]?.[1] as RequestInit;
      expect(options.credentials).toBe("same-origin");
      expect(options.headers).toMatchObject({ "X-CSRF-Token": "c".repeat(43) });
      expect(
        (options.headers as Record<string, string>)["Idempotency-Key"],
      ).toMatch(/^order-export:[0-9a-f-]{36}$/i);
      expect(state.setState).toHaveBeenLastCalledWith({
        kind: "completed",
        value: {
          jobID: "exp_0000000000000001",
          resource: "orders",
          createdAt: "2026-08-19T00:00:00Z",
          fileName: "exp_0000000000000001.csv",
        },
      });
    },
  );

  it("keeps sales at zero calls and permanently locks an unknown result", async () => {
    const createExport = vi.fn(async () => ({ status: 503, data: {} }));
    const getExport = vi.fn();
    const transport = {
      ...client(async () => ({ status: 200, data: {} })),
      createExport,
      getExport,
    } as OrdersTransport;
    const sales = exportController("sales", transport);
    expect(startLocalOrderExport(sales)).toBeUndefined();
    expect(createExport).not.toHaveBeenCalled();
    const admin = exportController("admin", transport);
    await startLocalOrderExport(admin);
    expect(admin.outcomeUnknown.current).toBe(true);
    expect(admin.setState).toHaveBeenLastCalledWith({
      kind: "unknown",
      message: expect.stringContaining("结果未知"),
    });
    expect(startLocalOrderExport(admin)).toBeUndefined();
    expect(createExport).toHaveBeenCalledOnce();
  });

  it("calls unauthenticated once for an active request and does not mark it unknown", async () => {
    const createExport = vi.fn(async () => ({ status: 401, data: {} }));
    const state = exportController("admin", {
      ...client(async () => ({ status: 200, data: {} })),
      createExport,
      getExport: vi.fn(),
    } as OrdersTransport);
    await startLocalOrderExport(state);
    expect(state.onUnauthenticated).toHaveBeenCalledOnce();
    expect(state.outcomeUnknown.current).toBe(false);
  });

  it.each([401, 403] as const)("locks after POST success when readback returns %s", async (status) => {
    const createExport = vi.fn(async () => ({ status: 200, data: exportReceipt() }));
    const getExport = vi.fn(async () => ({ status, data: {} }));
    const state = exportController("admin", { ...client(async () => ({ status: 200, data: {} })), createExport, getExport } as OrdersTransport);
    await startLocalOrderExport(state);
    expect(state.outcomeUnknown.current).toBe(true);
    expect(state.setState).toHaveBeenLastCalledWith({ kind: "unknown", message: expect.stringContaining("结果未知") });
    expect(state.onUnauthenticated).toHaveBeenCalledTimes(status === 401 ? 1 : 0);
    expect(startLocalOrderExport(state)).toBeUndefined();
    expect(createExport).toHaveBeenCalledOnce();
  });
});

describe("local order detail transition", () => {
  function controller(
    role: "admin" | "ops" | "sales",
    transport: OrdersTransport,
    previous = undefined as
      | undefined
      | {
          readonly id: number;
          readonly orderNo: string;
          readonly provider: "wechat";
          readonly productCode: string;
          readonly productName: string;
          readonly amountYuan: string;
          readonly currency: string;
          readonly statusLabel: string;
          readonly providerLabel: string;
          readonly createdAt: string;
          readonly refundableAmountTotal: number;
        },
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

  it.each(["admin", "ops"] as const)(
    "issues exactly one local detail GET for %s",
    async (role) => {
      const transport = client(async () => ({
        status: 200,
        data: { items: [], total: 0, limit: 50, has_more: false },
      }));
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
          id: 17,
          orderNo: "M-1",
          provider: "wechat",
          productCode: "SKU-1",
          productName: "商品",
          amountYuan: "19.90",
          currency: "CNY",
          statusLabel: "已支付",
          providerLabel: "微信支付",
          createdAt: "2026-08-19T00:00:00Z",
          refundableAmountTotal: 1990,
        },
      });
    },
  );

  it("keeps sales fail-closed with no detail transport call", () => {
    const transport = client(async () => ({ status: 200, data: {} }));
    const state = controller("sales", transport);
    expect(startOrderDetailLoad(state)).toBeUndefined();
    expect(transport.detail).not.toHaveBeenCalled();
  });

  it("cleans each order token when concurrent orders resolve out of order", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver argument is intentionally supplied by the Promise implementation.
    const resolvers: ((value: { status: number; data: unknown }) => void)[] =
      [];
    const transport = client(
      async () => ({
        status: 200,
        data: { items: [], total: 0, limit: 50, has_more: false },
      }),
      () =>
        new Promise((done) => {
          resolvers.push(done);
        }),
    );
    const firstState = controller("admin", transport);
    const secondState = {
      ...firstState,
      item: { ...page.items[0], orderNo: "M-2" },
    };
    const first = startOrderDetailLoad(firstState);
    const second = startOrderDetailLoad(secondState);
    expect(startOrderDetailLoad(firstState)).toBeUndefined();
    expect(transport.detail).toHaveBeenCalledTimes(2);
    resolvers[1]?.({
      status: 200,
      data: detail({
        merchant_order_no: "M-2",
        out_trade_no: "M-2",
        order_no: "M-2",
      }),
    });
    await second;
    expect(firstState.inFlight.current.has("M-1")).toBe(true);
    expect(firstState.inFlight.current.has("M-2")).toBe(false);
    resolvers[0]?.({ status: 200, data: detail() });
    await first;
    expect(firstState.inFlight.current.size).toBe(0);
  });

  it("does not let an old response release a new same-order request after reload", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver argument is intentionally supplied by the Promise implementation.
    const resolvers: ((value: { status: number; data: unknown }) => void)[] =
      [];
    const transport = client(
      async () => ({
        status: 200,
        data: { items: [], total: 0, limit: 50, has_more: false },
      }),
      () =>
        new Promise((done) => {
          resolvers.push(done);
        }),
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
      id: 17,
      orderNo: "M-1",
      provider: "wechat" as const,
      productCode: "SKU-1",
      productName: "商品",
      amountYuan: "19.90",
      currency: "CNY",
      statusLabel: "已支付",
      providerLabel: "微信支付",
      createdAt: "2026-08-19T00:00:00Z",
      refundableAmountTotal: 1990,
    };
    const unavailable = controller(
      "admin",
      client(
        async () => ({ status: 200, data: {} }),
        async () => ({ status: 503, data: {} }),
      ),
      previous,
    );
    await startOrderDetailLoad(unavailable);
    expect(unavailable.setState).toHaveBeenLastCalledWith({
      kind: "error",
      orderNo: "M-1",
      failure: "unavailable",
      previous,
    });

    const onUnauthenticated = vi.fn();
    const unauthenticated = {
      ...controller(
        "admin",
        client(
          async () => ({ status: 200, data: {} }),
          async () => ({ status: 401, data: {} }),
        ),
        previous,
      ),
      onUnauthenticated,
    };
    await startOrderDetailLoad(unauthenticated);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(unauthenticated.setState).toHaveBeenLastCalledWith({
      kind: "error",
      orderNo: "M-1",
      failure: "unauthenticated",
      previous,
    });
  });

  it("renders verified local fields only, with no identity, provider URL, or action surface", () => {
    const detailState: OrderDetailViewState = {
      kind: "ready",
      detail: {
        id: 17,
        orderNo: "M-1",
        provider: "wechat",
        productCode: "SKU-1",
        productName: "商品",
        amountYuan: "19.90",
        currency: "CNY",
        statusLabel: "已支付",
        providerLabel: "微信支付",
        createdAt: "2026-08-19T00:00:00Z",
        refundableAmountTotal: 1990,
      },
    };
    const html = renderToStaticMarkup(
      <OrdersContent
        state={{ kind: "ready", page }}
        detail={detailState}
        refunds={{ kind: "loading" }}
        onLoad={vi.fn()}
        onLoadDetail={vi.fn()}
        onLoadRefunds={vi.fn()}
      />,
    );
    expect(html).toContain("本地可退金额（分）");
    expect(html).toContain("不代表退款已经执行");
    expect(html).not.toContain("detail_url");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("<a");
    expect(html).not.toContain("退款申请");
  });
});

describe("local purchased-product transition", () => {
  it.each(["admin", "ops"] as const)(
    "issues one same-origin item GET for %s",
    async (role) => {
      const transport = client(async () => ({
        status: 200,
        data: { items: [], total: 0, limit: 50, has_more: false },
      }));
      const state = {
        role,
        item: page.items[0],
        transport,
        generation: { current: 0 },
        inFlight: { current: new Map<string, symbol>() },
        verified: { current: undefined },
        setState: vi.fn(),
      };
      await startOrderItemsLoad(state);
      expect(transport.items).toHaveBeenCalledWith(
        "M-1",
        { provider: "wechat" },
        { credentials: "same-origin" },
      );
      expect(state.setState).toHaveBeenLastCalledWith({
        kind: "ready",
        item: {
          orderNo: "M-1",
          provider: "wechat",
          productCode: "SKU-1",
          productName: "商品",
          amountYuan: "19.90",
          currency: "CNY",
          createdAt: "2026-08-19T00:00:00Z",
        },
      });
    },
  );

  it("keeps sales fail-closed and preserves a verified snapshot on a 401", async () => {
    const salesTransport = client(async () => ({ status: 200, data: {} }));
    const sales = {
      role: "sales" as const,
      item: page.items[0],
      transport: salesTransport,
      generation: { current: 0 },
      inFlight: { current: new Map<string, symbol>() },
      verified: { current: undefined },
      setState: vi.fn(),
    };
    expect(startOrderItemsLoad(sales)).toBeUndefined();
    expect(salesTransport.items).not.toHaveBeenCalled();

    const onUnauthenticated = vi.fn();
    const previous = {
      orderNo: "M-1",
      provider: "wechat" as const,
      productCode: "SKU-1",
      productName: "商品",
      amountYuan: "19.90",
      currency: "CNY",
      createdAt: "2026-08-19T00:00:00Z",
    };
    const admin = {
      role: "admin" as const,
      item: page.items[0],
      transport: client(
        async () => ({ status: 200, data: {} }),
        async () => ({ status: 200, data: detail() }),
        async () => ({ status: 200, data: {} }),
        async () => ({ status: 401, data: {} }),
      ),
      generation: { current: 0 },
      inFlight: { current: new Map<string, symbol>() },
      verified: { current: previous },
      setState: vi.fn(),
      onUnauthenticated,
    };
    await startOrderItemsLoad(admin);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(admin.setState).toHaveBeenLastCalledWith({
      kind: "error",
      orderNo: "M-1",
      failure: "unauthenticated",
      previous,
    });
  });
});

describe("local refund-history transition", () => {
  function controller(
    role: "admin" | "ops" | "sales",
    transport: OrdersTransport,
    previous = undefined as typeof refunds | undefined,
  ) {
    return {
      role,
      offset: 0,
      transport,
      generation: { current: 0 },
      inFlight: { current: undefined as symbol | undefined },
      verified: { current: previous },
      setState: vi.fn(),
    };
  }

  it.each(["admin", "ops"] as const)(
    "issues one fixed local refund GET for %s",
    async (role) => {
      const transport = client(
        async () => ({
          status: 200,
          data: { items: [], total: 0, limit: 50, has_more: false },
        }),
        async () => ({ status: 200, data: detail() }),
        async () => ({
          status: 200,
          data: {
            items: [
              {
                id: 23,
                order_id: 17,
                provider: "wechat",
                order_no: "M-1",
                transaction_id: "WX-1",
                refund_id: "rfd_provider-1",
                out_refund_no: "rfd_local-1",
                refund_amount_total: 1990,
                currency: "CNY",
                reason: "重复支付",
                status: "completed",
                external_effect_id: 31,
                external_effect_state: "completed",
                auto_retry_allowed: false,
                created_at: "2026-08-19T00:00:00Z",
              },
            ],
            total: 1,
            limit: 50,
            has_more: false,
          },
        }),
      );
      const state = controller(role, transport);
      await startLocalRefundLoad(state);
      expect(transport.refunds).toHaveBeenCalledWith(
        { provider: "all", limit: 50, offset: 0 },
        { credentials: "same-origin" },
      );
      expect(state.setState).toHaveBeenLastCalledWith({
        kind: "ready",
        page: {
          items: [
            {
              id: 23,
              provider: "wechat",
              orderNo: "M-1",
              refundAmountTotal: 1990,
              currency: "CNY",
              status: "completed",
              externalEffectState: "completed",
              createdAt: "2026-08-19T00:00:00Z",
            },
          ],
          total: 1,
          offset: 0,
          hasMore: false,
        },
      });
    },
  );

  it("keeps sales fail-closed with no refund transport call", () => {
    const transport = client(async () => ({ status: 200, data: {} }));
    const state = controller("sales", transport);
    expect(startLocalRefundLoad(state)).toBeUndefined();
    expect(transport.refunds).not.toHaveBeenCalled();
  });

  it("locks same-tick reads, discards stale responses, and retains verified state on 401 or unavailable", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver parameter is intentionally supplied by Promise resolution.
    let resolve: ((value: TransportResponse) => void) | undefined;
    const transport = client(
      async () => ({
        status: 200,
        data: { items: [], total: 0, limit: 50, has_more: false },
      }),
      async () => ({ status: 200, data: detail() }),
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const state = controller("admin", transport);
    const first = startLocalRefundLoad(state);
    expect(startLocalRefundLoad(state)).toBeUndefined();
    expect(transport.refunds).toHaveBeenCalledOnce();
    state.generation.current += 1;
    resolve?.({ status: 503, data: {} });
    await first;
    expect(state.setState).toHaveBeenCalledTimes(1);
    expect(state.inFlight.current).toBeUndefined();

    const previous = refunds;
    const unavailable = controller(
      "admin",
      client(
        async () => ({ status: 200, data: {} }),
        async () => ({ status: 200, data: detail() }),
        async () => ({ status: 503, data: {} }),
      ),
      previous,
    );
    await startLocalRefundLoad(unavailable);
    expect(unavailable.setState).toHaveBeenLastCalledWith({
      kind: "error",
      failure: "unavailable",
      previous,
    });

    const onUnauthenticated = vi.fn();
    const unauthenticated = {
      ...controller(
        "admin",
        client(
          async () => ({ status: 200, data: {} }),
          async () => ({ status: 200, data: detail() }),
          async () => ({ status: 401, data: {} }),
        ),
        previous,
      ),
      onUnauthenticated,
    };
    await startLocalRefundLoad(unauthenticated);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(unauthenticated.setState).toHaveBeenLastCalledWith({
      kind: "error",
      failure: "unauthenticated",
      previous,
    });
  });

  it("keeps a reload token after cleanup while an old transport finally resolves", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver parameter is intentionally supplied by Promise resolution.
    let resolveA: ((value: TransportResponse) => void) | undefined;
    // eslint-disable-next-line no-unused-vars -- resolver parameter is intentionally supplied by Promise resolution.
    let resolveB: ((value: TransportResponse) => void) | undefined;
    const transportA = client(
      async () => ({ status: 200, data: {} }),
      async () => ({ status: 200, data: detail() }),
      () =>
        new Promise((done) => {
          resolveA = done;
        }),
    );
    const transportB = client(
      async () => ({ status: 200, data: {} }),
      async () => ({ status: 200, data: detail() }),
      () =>
        new Promise((done) => {
          resolveB = done;
        }),
    );
    const generation = { current: 0 };
    const inFlight = { current: undefined as symbol | undefined };
    const verified = { current: undefined as typeof refunds | undefined };
    const first = startLocalRefundLoad({
      role: "admin",
      offset: 0,
      transport: transportA,
      generation,
      inFlight,
      verified,
      setState: vi.fn(),
    });
    expect(inFlight.current).toBeDefined();

    // Mirrors effect cleanup before a role/transport reload: invalidate A and
    // release only A's lock so the next effect can begin B.
    generation.current += 1;
    inFlight.current = undefined;
    const setStateB = vi.fn();
    const second = startLocalRefundLoad({
      role: "admin",
      offset: 0,
      transport: transportB,
      generation,
      inFlight,
      verified,
      setState: setStateB,
    });
    const tokenB = inFlight.current;
    expect(tokenB).toBeDefined();
    expect(transportB.refunds).toHaveBeenCalledOnce();

    resolveA?.({ status: 503, data: {} });
    await first;
    expect(inFlight.current).toBe(tokenB);

    resolveB?.({
      status: 200,
      data: {
        items: [
          {
            id: 23,
            order_id: 17,
            provider: "wechat",
            order_no: "M-1",
            transaction_id: "WX-1",
            refund_id: "rfd_provider-1",
            out_refund_no: "rfd_local-1",
            refund_amount_total: 1990,
            currency: "CNY",
            reason: "重复支付",
            status: "completed",
            external_effect_id: 31,
            external_effect_state: "completed",
            auto_retry_allowed: false,
            created_at: "2026-08-19T00:00:00Z",
          },
        ],
        total: 1,
        limit: 50,
        has_more: false,
      },
    });
    await second;
    expect(setStateB).toHaveBeenLastCalledWith({
      kind: "ready",
      page: {
        items: [
          {
            id: 23,
            provider: "wechat",
            orderNo: "M-1",
            refundAmountTotal: 1990,
            currency: "CNY",
            status: "completed",
            externalEffectState: "completed",
            createdAt: "2026-08-19T00:00:00Z",
          },
        ],
        total: 1,
        offset: 0,
        hasMore: false,
      },
    });
    expect(inFlight.current).toBeUndefined();
  });
});
