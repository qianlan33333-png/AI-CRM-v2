import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedOrdersTransport,
  loadOrders,
  nextOrderOffset,
  previousOrderOffset,
  type OrderListPage,
  type OrdersFailure,
  type OrdersRole,
  type OrdersTransport,
} from "./orders";

const messages: Record<OrdersFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有订单总览访问权限。",
  invalid: "订单总览响应不符合已冻结的本地只读合同。",
  unavailable: "本地订单总览暂不可用，请稍后再查看。",
};

export type OrdersViewState =
  | { readonly kind: "loading"; readonly previous?: OrderListPage }
  | { readonly kind: "ready"; readonly page: OrderListPage }
  | { readonly kind: "error"; readonly failure: OrdersFailure; readonly previous?: OrderListPage };

export interface OrdersLoadController {
  readonly role: OrdersRole;
  readonly offset: number;
  readonly transport: OrdersTransport;
  readonly generation: { current: number };
  readonly inFlight: { current: boolean };
  readonly verified: { current: OrderListPage | undefined };
  // eslint-disable-next-line no-unused-vars -- named transition value documents the state sink.
  readonly setState: (state: OrdersViewState) => void;
  readonly onUnauthenticated?: () => void;
}

export function canReadOrders(role: OrdersRole): boolean {
  return role === "admin" || role === "ops";
}

// startOrdersLoad is the sole browser-read transition: it locks the page,
// preserves a verified page on failure, and makes obsolete responses inert.
export function startOrdersLoad(
  controller: OrdersLoadController,
): Promise<void> | undefined {
  if (!canReadOrders(controller.role) || controller.inFlight.current) return undefined;
  controller.inFlight.current = true;
  const currentGeneration = ++controller.generation.current;
  controller.setState({ kind: "loading", previous: controller.verified.current });
  return loadOrders(controller.transport, controller.offset)
    .then((result) => {
      if (currentGeneration !== controller.generation.current) return;
      if (result.status === "loaded") {
        controller.verified.current = result.page;
        controller.setState({ kind: "ready", page: result.page });
        return;
      }
      if (result.status === "unauthenticated") controller.onUnauthenticated?.();
      controller.setState({
        kind: "error",
        failure: result.status,
        previous: controller.verified.current,
      });
    })
    .finally(() => {
      controller.inFlight.current = false;
    });
}

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

function OrderRows({ page }: { readonly page: OrderListPage }): React.ReactElement {
  return (
    <>
      <p>已验证的本地订单投影：共 {page.total} 条，当前从第 {page.offset + 1} 条开始。</p>
      {page.items.length === 0 ? <p role="status">当前页没有本地订单。</p> : (
        <table>
          <thead>
            <tr>
              <th>订单号</th><th>支付渠道</th><th>状态</th><th>商品</th><th>金额</th><th>创建时间</th><th>付款人</th><th>手机号</th>
            </tr>
          </thead>
          <tbody>
            {page.items.map((item) => (
              <tr key={item.orderNo}>
                <td>{item.orderNo}</td><td>{item.providerLabel}</td><td>{item.statusLabel}</td>
                <td>{item.productName === "" ? item.productCode : `${item.productName}（${item.productCode}）`}</td>
                <td>{item.amountYuan} {item.currency}</td><td>{displayDate(item.createdAt)}</td>
                <td>{item.payerName === "" ? "—" : item.payerName}</td><td>{item.mobile === "" ? "—" : item.mobile}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

export function OrdersContent({
  onLoad,
  state,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoad: (offset: number) => void;
  readonly state: OrdersViewState;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">交易权益 · 本地只读投影</p>
      <h1 id="app-title">订单总览</h1>
      <p>仅显示已持久化的本地订单投影，不会查询支付渠道、创建退款、导出数据或触发任何外部操作。</p>
      {page ? <OrderRows page={page} /> : null}
      {state.kind === "loading" ? <p role="status">正在读取本地订单总览。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      {page ? (
        <p>
          <button
            type="button"
            disabled={state.kind === "loading" || previousOrderOffset(page) === undefined}
            onClick={() => {
              const previous = previousOrderOffset(page);
              if (previous !== undefined) onLoad(previous);
            }}
          >上一页</button>{" "}
          <button
            type="button"
            disabled={state.kind === "loading" || nextOrderOffset(page) === undefined}
            onClick={() => {
              const next = nextOrderOffset(page);
              if (next !== undefined) onLoad(next);
            }}
          >下一页</button>
        </p>
      ) : null}
    </section>
  );
}

export function OrdersPage({
  role,
  transport = generatedOrdersTransport,
  onUnauthenticated,
}: {
  readonly role: OrdersRole;
  readonly transport?: OrdersTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = canReadOrders(role);
  const generation = useRef(0);
  const inFlight = useRef(false);
  const verified = useRef<OrderListPage>();
  const [state, setState] = useState<OrdersViewState>({ kind: "loading" });

  const load = useCallback((offset: number) => startOrdersLoad({
    role,
    offset,
    transport,
    generation,
    inFlight,
    verified,
    setState,
    onUnauthenticated,
  }), [onUnauthenticated, role, transport]);

  useEffect(() => {
    if (canRead) void load(0);
    return () => { generation.current += 1; };
  }, [canRead, load]);

  if (!canRead) return (
    <section className="route-card" aria-labelledby="app-title">
      <h1 id="app-title">订单总览</h1>
      <p>当前账号没有订单总览访问权限。</p>
    </section>
  );
  return <OrdersContent state={state} onLoad={(offset) => void load(offset)} />;
}
