/* eslint-disable no-unused-vars -- callback parameter names document the test harness contracts. */
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  CommerceTransactionDetailContent,
  CommerceTransactionListContent,
  commerceTransactionDetailPath,
  commerceTransactionProvider,
  generatedCommerceTransactionTransport,
  startCommerceTransactionDetailLoad,
  startCommerceTransactionListLoad,
  type CommerceTransactionDetailState,
  type CommerceTransactionListState,
} from "./commerce-transactions-ui";
import { commerceWorkspaceRoute } from "./commerce-workspaces";
import { type OrdersTransport } from "./orders";

type Response = { readonly status: number; readonly data: unknown };

function deferred(): {
  readonly promise: Promise<Response>;
  resolve(value: Response): void;
} {
  let resolve!: (value: Response) => void;
  return {
    promise: new Promise<Response>((done) => {
      resolve = done;
    }),
    resolve,
  };
}

function rawOrder(
  provider: "wechat" | "alipay" | "wechat_shop" = "wechat",
  extra: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    created_at: "2026-08-20T10:00:00Z",
    merchant_order_no: "M-1",
    out_trade_no: "M-1",
    order_no: "M-1",
    platform_transaction_no: "WX-1",
    transaction_id: "WX-1",
    payer_name: "不得展示的姓名",
    mobile: "13800000000",
    product_code: "SKU-1",
    product_name: "本地商品",
    amount_yuan: "19.90",
    currency: "CNY",
    status: "paid",
    status_label: "已支付",
    provider,
    provider_label:
      provider === "wechat"
        ? "微信支付"
        : provider === "alipay"
          ? "支付宝"
          : "微信小店",
    detail_url: "/api/admin/orders/M-1",
    ...extra,
  };
}

function transport(
  list: () => Promise<Response>,
  detail: () => Promise<Response> = async () => ({
    status: 200,
    data: { ...rawOrder(), id: 7, refundable_amount_total: 1990 },
  }),
): OrdersTransport {
  return {
    list: vi.fn(list),
    detail: vi.fn(detail),
    items: vi.fn(),
    refunds: vi.fn(),
  } as unknown as OrdersTransport;
}

function listController(
  client: OrdersTransport,
  setState: (state: CommerceTransactionListState) => void,
  overrides: Record<string, unknown> = {},
) {
  return {
    role: "admin" as const,
    provider: "wechat" as const,
    offset: 0,
    transport: client,
    generation: { current: 0 },
    inFlight: { current: undefined as symbol | undefined },
    verified: { current: undefined },
    setState,
    ...overrides,
  };
}

function detailController(
  client: OrdersTransport,
  setState: (state: CommerceTransactionDetailState) => void,
  overrides: Record<string, unknown> = {},
) {
  return {
    role: "admin" as const,
    provider: "wechat" as const,
    reference: "7",
    transport: client,
    generation: { current: 0 },
    inFlight: { current: undefined as symbol | undefined },
    verified: { current: undefined },
    setState,
    ...overrides,
  };
}

describe("commerce transaction consumer", () => {
  it("uses the frozen dedicated list entries and keeps shop on the scoped unified entry", async () => {
    const fetch = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        Promise.resolve(
          new globalThis.Response(
            JSON.stringify({ items: [], total: 0, limit: 50, has_more: false }),
            { status: 200 },
          ),
        ),
    );
    vi.stubGlobal("fetch", fetch);
    try {
      await generatedCommerceTransactionTransport.list(
        { provider: "alipay", limit: 50, offset: 0 },
        { credentials: "same-origin" },
      );
      await generatedCommerceTransactionTransport.list(
        { provider: "wechat", limit: 50, offset: 0 },
        { credentials: "same-origin" },
      );
      await generatedCommerceTransactionTransport.list(
        { provider: "wechat_shop", limit: 50, offset: 0 },
        { credentials: "same-origin" },
      );
      await expect(
        generatedCommerceTransactionTransport.list(
          { provider: "all", limit: 50, offset: 0 },
          { credentials: "same-origin" },
        ),
      ).resolves.toMatchObject({ status: 400 });
      expect(fetch.mock.calls.map((call) => call[0])).toEqual([
        "/api/admin/alipay/transactions?limit=50&offset=0",
        "/api/admin/wechat-pay/orders?limit=50&offset=0",
        "/api/admin/orders?provider=wechat_shop&limit=50&offset=0",
      ]);
      expect(fetch.mock.calls.every((call) => call[1]?.method === "GET")).toBe(
        true,
      );
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("maps the five page capabilities to exact providers and safe detail paths", () => {
    const alipay = commerceWorkspaceRoute("/admin/alipay/transactions");
    const wechat = commerceWorkspaceRoute("/admin/wechat-pay/transactions/7");
    const shop = commerceWorkspaceRoute("/admin/wechat-shop/transactions/M-1");
    if (!alipay || !wechat || !shop) throw new Error("expected routes");
    expect(commerceTransactionProvider(alipay as never)).toBe("alipay");
    expect(commerceTransactionProvider(wechat as never)).toBe("wechat");
    expect(commerceTransactionProvider(shop as never)).toBe("wechat_shop");
    expect(commerceTransactionDetailPath("wechat", "M-1")).toBe(
      "/admin/wechat-pay/transactions/M-1",
    );
    expect(commerceTransactionDetailPath("wechat_shop", "shop one")).toBe(
      "/admin/wechat-shop/transactions/shop%20one",
    );
    expect(commerceTransactionDetailPath("alipay", "M-1")).toBeUndefined();
    expect(
      commerceTransactionDetailPath("wechat", "unsafe/order"),
    ).toBeUndefined();
  });

  it("loads only the selected provider and discards payer, mobile, and transaction identifiers", async () => {
    const client = transport(async () => ({
      status: 200,
      data: { items: [rawOrder()], total: 1, limit: 50, has_more: false },
    }));
    const states: CommerceTransactionListState[] = [];
    const controller = listController(client, (state) => states.push(state));
    await startCommerceTransactionListLoad(controller);
    expect(client.list).toHaveBeenCalledWith(
      { provider: "wechat", limit: 50, offset: 0 },
      { credentials: "same-origin" },
    );
    const ready = states.at(-1);
    expect(ready?.kind).toBe("ready");
    expect(JSON.stringify(ready)).not.toContain("不得展示的姓名");
    expect(JSON.stringify(ready)).not.toContain("13800000000");
    expect(JSON.stringify(ready)).not.toContain("WX-1");
  });

  it("guards before transport, singleflights, ignores stale completion, and reports one active 401", async () => {
    const pending = deferred();
    const client = transport(() => pending.promise);
    const setState = vi.fn();
    const controller = listController(client, setState);
    const first = startCommerceTransactionListLoad(controller);
    expect(startCommerceTransactionListLoad(controller)).toBeUndefined();
    expect(client.list).toHaveBeenCalledTimes(1);
    controller.generation.current += 1;
    pending.resolve({ status: 401, data: {} });
    await first;
    expect(setState).toHaveBeenCalledTimes(1);

    const deniedClient = transport(async () => ({ status: 200, data: {} }));
    expect(
      startCommerceTransactionListLoad(
        listController(deniedClient, vi.fn(), { role: "ops" }),
      ),
    ).toBeUndefined();
    expect(deniedClient.list).not.toHaveBeenCalled();

    const onUnauthenticated = vi.fn();
    const activeClient = transport(async () => ({ status: 401, data: {} }));
    await startCommerceTransactionListLoad(
      listController(activeClient, vi.fn(), { onUnauthenticated }),
    );
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
  });

  it("retains a verified page when a later exact-contract response is invalid", async () => {
    const client = transport(async () => ({
      status: 200,
      data: { extra: true },
    }));
    const previous = {
      items: [],
      total: 0,
      offset: 0,
      hasMore: false,
      providerScope: "wechat" as const,
    };
    const states: CommerceTransactionListState[] = [];
    await startCommerceTransactionListLoad(
      listController(client, (state) => states.push(state), {
        verified: { current: previous },
      }),
    );
    expect(states.at(-1)).toEqual({
      kind: "error",
      failure: "invalid",
      previous,
    });
  });

  it("loads an opaque detail reference without assuming it equals order_no and locks the provider", async () => {
    const detail = vi.fn(async () => ({
      status: 200,
      data: {
        ...rawOrder("wechat"),
        id: 7,
        refundable_amount_total: 1990,
      },
    }));
    const client = transport(async () => ({ status: 503, data: {} }), detail);
    const states: CommerceTransactionDetailState[] = [];
    await startCommerceTransactionDetailLoad(
      detailController(client, (state) => states.push(state)),
    );
    expect(detail).toHaveBeenCalledWith(
      "7",
      { provider: "wechat" },
      { credentials: "same-origin" },
    );
    expect(states.at(-1)?.kind).toBe("ready");
    expect(JSON.stringify(states.at(-1))).not.toContain("不得展示的姓名");
    expect(JSON.stringify(states.at(-1))).not.toContain("13800000000");

    const mismatch = transport(
      async () => ({ status: 503, data: {} }),
      async () => ({
        status: 200,
        data: {
          ...rawOrder("wechat_shop"),
          id: 7,
          refundable_amount_total: 1990,
        },
      }),
    );
    const mismatchStates: CommerceTransactionDetailState[] = [];
    await startCommerceTransactionDetailLoad(
      detailController(mismatch, (state) => mismatchStates.push(state)),
    );
    expect(mismatchStates.at(-1)).toMatchObject({
      kind: "error",
      failure: "invalid",
    });
  });

  it("renders safe list and detail fields with no payment, refund, export, or provider action", () => {
    const page = {
      items: [
        {
          orderNo: "M-1",
          provider: "wechat" as const,
          status: "paid",
          productCode: "SKU-1",
          productName: "本地商品",
          amountYuan: "19.90",
          currency: "CNY",
          statusLabel: "已支付",
          providerLabel: "微信支付",
          createdAt: "2026-08-20T10:00:00Z",
        },
      ],
      total: 1,
      offset: 0,
      hasMore: false,
      providerScope: "wechat" as const,
    };
    const listHTML = renderToStaticMarkup(
      <CommerceTransactionListContent
        provider="wechat"
        state={{ kind: "ready", page }}
        onLoad={vi.fn()}
      />,
    );
    expect(listHTML).toContain("/admin/wechat-pay/transactions/M-1");
    expect(listHTML).toContain(
      "本页没有支付、退款、导出、重试或 Provider 操作",
    );
    expect(listHTML).not.toContain("payer");
    expect(listHTML).not.toContain("mobile");
    expect(listHTML).not.toContain("退款申请");

    const detailHTML = renderToStaticMarkup(
      <CommerceTransactionDetailContent
        provider="wechat"
        state={{
          kind: "ready",
          detail: {
            id: 7,
            orderNo: "M-1",
            provider: "wechat",
            productCode: "SKU-1",
            productName: "本地商品",
            amountYuan: "19.90",
            currency: "CNY",
            statusLabel: "已支付",
            providerLabel: "微信支付",
            createdAt: "2026-08-20T10:00:00Z",
            refundableAmountTotal: 1990,
          },
        }}
        onReload={vi.fn()}
      />,
    );
    expect(detailHTML).toContain("本地订单 ID");
    expect(detailHTML).not.toContain("1990");
    expect(detailHTML).not.toContain("可退金额");
    expect(detailHTML).not.toContain("退款申请");
    expect(detailHTML).not.toContain("重试退款");
  });
});
