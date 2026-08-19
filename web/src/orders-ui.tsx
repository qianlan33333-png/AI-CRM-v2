import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedOrdersTransport,
  filterSafeOrders,
  filterSafeRefunds,
  loadOrderDetail,
  loadOrderItems,
  loadLocalExternalEffects,
  loadLocalRefunds,
  loadOrders,
  nextLocalRefundOffset,
  nextOrderOffset,
  previousLocalRefundOffset,
  previousOrderOffset,
  type LocalRefundPage,
  type LocalExternalEffectPage,
  type OrderDetail,
  type OrderItemSnapshot,
  type OrderListPage,
  type OrderListItem,
  type OrdersFailure,
  type OrdersRole,
  type OrdersTransport,
  type SafeOrderFilter,
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

export type OrderDetailViewState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly orderNo: string; readonly previous?: OrderDetail }
  | { readonly kind: "ready"; readonly detail: OrderDetail }
  | { readonly kind: "error"; readonly orderNo: string; readonly failure: OrdersFailure; readonly previous?: OrderDetail };

export type LocalRefundViewState =
  | { readonly kind: "loading"; readonly previous?: LocalRefundPage }
  | { readonly kind: "ready"; readonly page: LocalRefundPage }
  | { readonly kind: "error"; readonly failure: OrdersFailure; readonly previous?: LocalRefundPage };

export type LocalExternalEffectViewState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly orderNo: string; readonly previous?: LocalExternalEffectPage }
  | { readonly kind: "ready"; readonly orderNo: string; readonly page: LocalExternalEffectPage }
  | { readonly kind: "error"; readonly orderNo: string; readonly failure: OrdersFailure; readonly previous?: LocalExternalEffectPage };

export type OrderItemsViewState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly orderNo: string; readonly previous?: OrderItemSnapshot }
  | { readonly kind: "ready"; readonly item: OrderItemSnapshot }
  | { readonly kind: "error"; readonly orderNo: string; readonly failure: OrdersFailure; readonly previous?: OrderItemSnapshot };

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

export interface OrderDetailLoadController {
  readonly role: OrdersRole;
  readonly item: OrderListItem;
  readonly transport: OrdersTransport;
  readonly generation: { current: number };
  readonly inFlight: { current: ReadonlyMap<string, symbol> };
  readonly verified: { current: OrderDetail | undefined };
  // eslint-disable-next-line no-unused-vars -- named transition value documents the detail state sink.
  readonly setState: (state: OrderDetailViewState) => void;
  readonly onUnauthenticated?: () => void;
}

export interface LocalRefundLoadController {
  readonly role: OrdersRole;
  readonly offset: number;
  readonly transport: OrdersTransport;
  readonly generation: { current: number };
  readonly inFlight: { current: symbol | undefined };
  readonly verified: { current: LocalRefundPage | undefined };
  // eslint-disable-next-line no-unused-vars -- named transition value documents the state sink.
  readonly setState: (state: LocalRefundViewState) => void;
  readonly onUnauthenticated?: () => void;
}

export interface OrderItemsLoadController {
  readonly role: OrdersRole;
  readonly item: OrderListItem;
  readonly transport: OrdersTransport;
  readonly generation: { current: number };
  readonly inFlight: { current: ReadonlyMap<string, symbol> };
  readonly verified: { current: OrderItemSnapshot | undefined };
  // eslint-disable-next-line no-unused-vars -- named transition value documents the state sink.
  readonly setState: (state: OrderItemsViewState) => void;
  readonly onUnauthenticated?: () => void;
}

export interface LocalExternalEffectLoadController {
  readonly role: OrdersRole;
  readonly detail: OrderDetail;
  readonly transport: OrdersTransport;
  readonly generation: { current: number };
  readonly inFlight: { current: ReadonlyMap<string, symbol> };
  readonly verified: { current: LocalExternalEffectPage | undefined };
  // eslint-disable-next-line no-unused-vars -- named transition value documents the state sink.
  readonly setState: (state: LocalExternalEffectViewState) => void;
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

// startOrderDetailLoad permits one read per order identity and makes a list
// change or unmount invalidate any response that was started before it.
export function startOrderDetailLoad(
  controller: OrderDetailLoadController,
): Promise<void> | undefined {
  if (!canReadOrders(controller.role) || controller.inFlight.current.has(controller.item.orderNo)) return undefined;
  const token = Symbol(controller.item.orderNo);
  const nextInFlight = new Map(controller.inFlight.current);
  nextInFlight.set(controller.item.orderNo, token);
  controller.inFlight.current = nextInFlight;
  const currentGeneration = ++controller.generation.current;
  const previous = controller.verified.current?.orderNo === controller.item.orderNo
    ? controller.verified.current
    : undefined;
  controller.setState({ kind: "loading", orderNo: controller.item.orderNo, previous });
  return loadOrderDetail(controller.transport, controller.item)
    .then((result) => {
      if (currentGeneration !== controller.generation.current) return;
      if (result.status === "loaded") {
        controller.verified.current = result.detail;
        controller.setState({ kind: "ready", detail: result.detail });
        return;
      }
      if (result.status === "unauthenticated") controller.onUnauthenticated?.();
      controller.setState({
        kind: "error",
        orderNo: controller.item.orderNo,
        failure: result.status,
        previous,
      });
    })
    .finally(() => {
      if (controller.inFlight.current.get(controller.item.orderNo) === token) {
        const next = new Map(controller.inFlight.current);
        next.delete(controller.item.orderNo);
        controller.inFlight.current = next;
      }
    });
}

// startOrderItemsLoad reads the already-persisted one-item product snapshot.
// It is deliberately separate from order detail: no payment/provider endpoint is used.
export function startOrderItemsLoad(
  controller: OrderItemsLoadController,
): Promise<void> | undefined {
  if (!canReadOrders(controller.role) || controller.inFlight.current.has(controller.item.orderNo)) return undefined;
  const token = Symbol(controller.item.orderNo);
  const nextInFlight = new Map(controller.inFlight.current);
  nextInFlight.set(controller.item.orderNo, token);
  controller.inFlight.current = nextInFlight;
  const currentGeneration = ++controller.generation.current;
  const previous = controller.verified.current?.orderNo === controller.item.orderNo
    ? controller.verified.current
    : undefined;
  controller.setState({ kind: "loading", orderNo: controller.item.orderNo, previous });
  return loadOrderItems(controller.transport, controller.item)
    .then((result) => {
      if (currentGeneration !== controller.generation.current) return;
      if (result.status === "loaded") {
        controller.verified.current = result.item;
        controller.setState({ kind: "ready", item: result.item });
        return;
      }
      if (result.status === "unauthenticated") controller.onUnauthenticated?.();
      controller.setState({ kind: "error", orderNo: controller.item.orderNo, failure: result.status, previous });
    })
    .finally(() => {
      if (controller.inFlight.current.get(controller.item.orderNo) === token) {
        const next = new Map(controller.inFlight.current);
        next.delete(controller.item.orderNo);
        controller.inFlight.current = next;
      }
    });
}

// startLocalRefundLoad is a read-only local-history transition. It never
// invokes the adjacent refund-intent POST or any provider/retry operation.
export function startLocalRefundLoad(
  controller: LocalRefundLoadController,
): Promise<void> | undefined {
  if (!canReadOrders(controller.role) || controller.inFlight.current) return undefined;
  const token = Symbol("local-refund-history");
  controller.inFlight.current = token;
  const currentGeneration = ++controller.generation.current;
  controller.setState({ kind: "loading", previous: controller.verified.current });
  return loadLocalRefunds(controller.transport, controller.offset)
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
      if (controller.inFlight.current === token) controller.inFlight.current = undefined;
    });
}

export function startLocalExternalEffectLoad(controller: LocalExternalEffectLoadController): Promise<void> | undefined {
  if (!canReadOrders(controller.role) || controller.detail.provider !== "wechat" || !controller.transport.externalEffects || controller.inFlight.current.has(controller.detail.orderNo)) return undefined;
  const token = Symbol(controller.detail.orderNo);
  const next = new Map(controller.inFlight.current); next.set(controller.detail.orderNo, token); controller.inFlight.current = next;
  const generation = ++controller.generation.current;
  const previous = controller.verified.current;
  controller.setState({ kind: "loading", orderNo: controller.detail.orderNo, previous });
  return loadLocalExternalEffects(controller.transport, controller.detail).then((result) => {
    if (generation !== controller.generation.current) return;
    if (result.status === "loaded") { controller.verified.current = result.page; controller.setState({ kind: "ready", orderNo: controller.detail.orderNo, page: result.page }); return; }
    if (result.status === "unauthenticated") controller.onUnauthenticated?.();
    controller.setState({ kind: "error", orderNo: controller.detail.orderNo, failure: result.status, previous });
  }).finally(() => {
    if (controller.inFlight.current.get(controller.detail.orderNo) === token) { const cleared = new Map(controller.inFlight.current); cleared.delete(controller.detail.orderNo); controller.inFlight.current = cleared; }
  });
}

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

function OrderRows({
  page,
  detail,
  itemSnapshot,
  onLoadDetail,
  onLoadItems,
}: {
  readonly page: OrderListPage;
  readonly detail: OrderDetailViewState;
  readonly itemSnapshot: OrderItemsViewState;
  // eslint-disable-next-line no-unused-vars -- named callback parameter identifies the selected local order.
  readonly onLoadDetail: (item: OrderListItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter identifies the selected local order item.
  readonly onLoadItems: (item: OrderListItem) => void;
}): React.ReactElement {
  return (
    <>
      <p>已验证的本地订单投影：共 {page.total} 条，当前从第 {page.offset + 1} 条开始。</p>
      {page.items.length === 0 ? <p role="status">当前页没有本地订单。</p> : (
        <table>
          <thead>
            <tr>
              <th>订单号</th><th>支付渠道</th><th>状态</th><th>商品</th><th>金额</th><th>创建时间</th><th>本地读取</th>
            </tr>
          </thead>
          <tbody>
            {page.items.map((item) => (
              <tr key={item.orderNo}>
                <td>{item.orderNo}</td><td>{item.providerLabel}</td><td>{item.statusLabel}</td>
                <td>{item.productName === "" ? item.productCode : `${item.productName}（${item.productCode}）`}</td>
                <td>{item.amountYuan} {item.currency}</td><td>{displayDate(item.createdAt)}</td>
                <td><button
                  type="button"
                  disabled={detail.kind === "loading" && detail.orderNo === item.orderNo}
                  onClick={() => onLoadDetail(item)}
                >查看本地详情</button>{" "}<button
                  type="button"
                  disabled={itemSnapshot.kind === "loading" && itemSnapshot.orderNo === item.orderNo}
                  onClick={() => onLoadItems(item)}
                >查看商品项</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

function OrderDetailPanel({ state, effects, onLoadEffects }: { readonly state: OrderDetailViewState; readonly effects: LocalExternalEffectViewState; // eslint-disable-next-line no-unused-vars -- callback identifies the verified local detail.
 readonly onLoadEffects: (detail: OrderDetail) => void; }): React.ReactElement | null {
  const detail = state.kind === "ready" ? state.detail : state.kind === "loading" || state.kind === "error" ? state.previous : undefined;
  if (state.kind === "idle") return null;
  return (
    <section aria-label="本地订单详情" data-testid="order-detail">
      <h2>本地订单详情</h2>
      {detail ? <dl>
        <dt>本地订单 ID</dt><dd>{detail.id}</dd>
        <dt>订单号</dt><dd>{detail.orderNo}</dd>
        <dt>支付渠道</dt><dd>{detail.providerLabel}</dd>
        <dt>状态</dt><dd>{detail.statusLabel}</dd>
        <dt>商品</dt><dd>{detail.productName === "" ? detail.productCode : `${detail.productName}（${detail.productCode}）`}</dd>
        <dt>订单金额</dt><dd>{detail.amountYuan} {detail.currency}</dd>
        <dt>本地可退金额（分）</dt><dd>{detail.refundableAmountTotal}</dd>
        <dt>创建时间</dt><dd>{displayDate(detail.createdAt)}</dd>
      </dl> : null}
      {state.kind === "loading" ? <p role="status">正在读取本地订单详情。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      <p>详情仅来自本地订单投影；可退金额不代表退款已经执行，也不会发起退款。</p>
      {detail?.provider === "wechat" ? <button type="button" disabled={effects.kind === "loading"} onClick={() => onLoadEffects(detail)}>读取本地外效状态</button> : null}
      {effects.kind !== "idle" ? <section aria-label="本地外效状态" data-testid="local-external-effects"><h3>本地外效状态</h3><p>仅显示已持久化的本地状态；任何状态均不代表支付渠道、投递或退款成功，本页不会重试或调用第三方服务。</p>{(effects.kind === "ready" ? effects.page : effects.previous) ? <table><thead><tr><th>本地记录 ID</th><th>本地类型</th><th>本地状态</th><th>创建时间</th><th>更新时间</th></tr></thead><tbody>{(effects.kind === "ready" ? effects.page : effects.previous)!.items.map((item) => <tr key={item.id}><td>{item.id}</td><td>{item.kind}</td><td>{item.state}</td><td>{displayDate(item.createdAt)}</td><td>{displayDate(item.updatedAt)}</td></tr>)}</tbody></table> : null}{effects.kind === "loading" ? <p role="status">正在读取本地外效状态。</p> : null}{effects.kind === "error" ? <p role="alert">{messages[effects.failure]}</p> : null}</section> : null}
    </section>
  );
}

function OrderItemsPanel({ state }: { readonly state: OrderItemsViewState }): React.ReactElement | null {
  const item = state.kind === "ready" ? state.item : state.kind === "loading" || state.kind === "error" ? state.previous : undefined;
  if (state.kind === "idle") return null;
  return (
    <section aria-label="本地购买商品项" data-testid="order-items">
      <h2>本地购买商品项</h2>
      {item ? <dl>
        <dt>订单号</dt><dd>{item.orderNo}</dd>
        <dt>商品</dt><dd>{item.productName === "" ? item.productCode : `${item.productName}（${item.productCode}）`}</dd>
        <dt>金额</dt><dd>{item.amountYuan} {item.currency}</dd>
        <dt>创建时间</dt><dd>{displayDate(item.createdAt)}</dd>
      </dl> : null}
      {state.kind === "loading" ? <p role="status">正在读取本地商品项。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      <p>商品项仅为已持久化的本地购买快照，不会查询支付渠道或媒体链接。</p>
    </section>
  );
}

function LocalRefundHistoryPanel({
  onLoad,
  state,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameter identifies the requested local page.
  readonly onLoad: (offset: number) => void;
  readonly state: LocalRefundViewState;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  const [filter, setFilter] = useState<SafeOrderFilter>({ keyword: "", provider: "all", status: "" });
  const filtered = page ? filterSafeRefunds(page.items, filter) : [];
  return (
    <section aria-label="本地退款意图历史" data-testid="local-refund-history">
      <h2>本地退款意图历史</h2>
      <p>
        仅显示已持久化的本地退款意图与本地外效状态；任何状态（包括 completed）
        都不代表支付渠道已执行、送达或退款成功。
      </p>
      {page ? (
        <>
          <fieldset>
            <legend>当前页本地筛选</legend>
            <label>关键词（订单号）<input value={filter.keyword} onChange={(event) => setFilter({ ...filter, keyword: event.currentTarget.value })} /></label>
            <label>渠道<select value={filter.provider} onChange={(event) => setFilter({ ...filter, provider: event.currentTarget.value as SafeOrderFilter["provider"] })}><option value="all">全部</option><option value="wechat">微信</option><option value="alipay">支付宝</option><option value="wechat_shop">微信小店</option></select></label>
            <label>本地状态<input value={filter.status} onChange={(event) => setFilter({ ...filter, status: event.currentTarget.value })} /></label>
          </fieldset>
          {filtered.length === 0 ? <p role="status">当前页没有符合筛选条件的本地退款意图记录。</p> : (
          <table>
            <thead>
              <tr>
                <th>本地记录 ID</th><th>订单号</th><th>渠道</th>
                <th>金额（分）</th><th>本地意图状态</th><th>本地外效状态</th><th>创建时间</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((item) => (
                <tr key={item.id}>
                  <td>{item.id}</td><td>{item.orderNo}</td><td>{item.provider}</td>
                  <td>{item.refundAmountTotal} {item.currency}</td><td>{item.status}</td>
                  <td>{item.externalEffectState}</td><td>{displayDate(item.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          )}
        </>
      ) : null}
      {state.kind === "loading" ? <p role="status">正在读取本地退款意图历史。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      {page ? (
        <p>
          <button
            type="button"
            disabled={state.kind === "loading" || previousLocalRefundOffset(page) === undefined}
            onClick={() => {
              const previous = previousLocalRefundOffset(page);
              if (previous !== undefined) onLoad(previous);
            }}
          >退款记录上一页</button>{" "}
          <button
            type="button"
            disabled={state.kind === "loading" || nextLocalRefundOffset(page) === undefined}
            onClick={() => {
              const next = nextLocalRefundOffset(page);
              if (next !== undefined) onLoad(next);
            }}
          >退款记录下一页</button>
        </p>
      ) : null}
    </section>
  );
}

export function OrdersContent({
  onLoad,
  onLoadDetail,
  onLoadItems = () => undefined,
  onLoadEffects = () => undefined,
  onLoadRefunds,
  detail,
  items = { kind: "idle" },
  effects = { kind: "idle" },
  refunds,
  state,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoad: (offset: number) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter identifies the selected local order.
  readonly onLoadDetail: (item: OrderListItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter identifies the selected local order item.
  readonly onLoadItems?: (item: OrderListItem) => void;
  // eslint-disable-next-line no-unused-vars -- callback identifies the verified local detail.
  readonly onLoadEffects?: (detail: OrderDetail) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter identifies the requested local-refund page.
  readonly onLoadRefunds: (offset: number) => void;
  readonly detail: OrderDetailViewState;
  readonly items?: OrderItemsViewState;
  readonly effects?: LocalExternalEffectViewState;
  readonly refunds: LocalRefundViewState;
  readonly state: OrdersViewState;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  const [filter, setFilter] = useState<SafeOrderFilter>({ keyword: "", provider: "all", status: "" });
  const filteredPage = page ? { ...page, items: filterSafeOrders(page.items, filter) } : undefined;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">交易权益 · 本地只读投影</p>
      <h1 id="app-title">订单总览</h1>
      <p>仅显示已持久化的本地订单投影，不会查询支付渠道、创建退款、导出数据或触发任何外部操作。</p>
      {page ? <fieldset><legend>当前页本地筛选</legend><label>关键词（订单号或商品）<input value={filter.keyword} onChange={(event) => setFilter({ ...filter, keyword: event.currentTarget.value })} /></label><label>渠道<select value={filter.provider} onChange={(event) => setFilter({ ...filter, provider: event.currentTarget.value as SafeOrderFilter["provider"] })}><option value="all">全部</option><option value="wechat">微信</option><option value="alipay">支付宝</option><option value="wechat_shop">微信小店</option></select></label><label>订单状态<input value={filter.status} onChange={(event) => setFilter({ ...filter, status: event.currentTarget.value })} /></label></fieldset> : null}
      {filteredPage ? <OrderRows page={filteredPage} detail={detail} itemSnapshot={items} onLoadDetail={onLoadDetail} onLoadItems={onLoadItems} /> : null}
      {state.kind === "loading" ? <p role="status">正在读取本地订单总览。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      <OrderDetailPanel state={detail} effects={effects} onLoadEffects={onLoadEffects} />
      <OrderItemsPanel state={items} />
      <LocalRefundHistoryPanel
        state={refunds}
        onLoad={onLoadRefunds}
      />
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
  const detailGeneration = useRef(0);
  const detailInFlight = useRef<ReadonlyMap<string, symbol>>(new Map());
  const verifiedDetail = useRef<OrderDetail>();
  const itemsGeneration = useRef(0);
  const itemsInFlight = useRef<ReadonlyMap<string, symbol>>(new Map());
  const verifiedItems = useRef<OrderItemSnapshot>();
  const refundGeneration = useRef(0);
  const refundInFlight = useRef<symbol>();
  const verifiedRefunds = useRef<LocalRefundPage>();
  const effectsGeneration = useRef(0);
  const effectsInFlight = useRef<ReadonlyMap<string, symbol>>(new Map());
  const verifiedEffects = useRef<LocalExternalEffectPage>();
  const [state, setState] = useState<OrdersViewState>({ kind: "loading" });
  const [detail, setDetail] = useState<OrderDetailViewState>({ kind: "idle" });
  const [items, setItems] = useState<OrderItemsViewState>({ kind: "idle" });
  const [refunds, setRefunds] = useState<LocalRefundViewState>({ kind: "loading" });
  const [effects, setEffects] = useState<LocalExternalEffectViewState>({ kind: "idle" });

  const load = useCallback((offset: number) => {
    detailGeneration.current += 1;
    detailInFlight.current = new Map();
    verifiedDetail.current = undefined;
    itemsGeneration.current += 1;
    itemsInFlight.current = new Map();
    verifiedItems.current = undefined;
    effectsGeneration.current += 1;
    effectsInFlight.current = new Map();
    verifiedEffects.current = undefined;
    setDetail({ kind: "idle" });
    setItems({ kind: "idle" });
    setEffects({ kind: "idle" });
    return startOrdersLoad({
      role,
      offset,
      transport,
      generation,
      inFlight,
      verified,
      setState,
      onUnauthenticated,
    });
  }, [onUnauthenticated, role, transport]);

  const loadDetail = useCallback((item: OrderListItem) => startOrderDetailLoad({
    role,
    item,
    transport,
    generation: detailGeneration,
    inFlight: detailInFlight,
    verified: verifiedDetail,
    setState: setDetail,
    onUnauthenticated,
  }), [onUnauthenticated, role, transport]);

  const loadItems = useCallback((item: OrderListItem) => startOrderItemsLoad({
    role, item, transport, generation: itemsGeneration, inFlight: itemsInFlight,
    verified: verifiedItems, setState: setItems, onUnauthenticated,
  }), [onUnauthenticated, role, transport]);

  const loadRefunds = useCallback((offset: number) => startLocalRefundLoad({
    role,
    offset,
    transport,
    generation: refundGeneration,
    inFlight: refundInFlight,
    verified: verifiedRefunds,
    setState: setRefunds,
    onUnauthenticated,
  }), [onUnauthenticated, role, transport]);

  const loadEffects = useCallback((detailValue: OrderDetail) => startLocalExternalEffectLoad({ role, detail: detailValue, transport, generation: effectsGeneration, inFlight: effectsInFlight, verified: verifiedEffects, setState: setEffects, onUnauthenticated }), [onUnauthenticated, role, transport]);

  useEffect(() => {
    if (canRead) {
      void load(0);
      void loadRefunds(0);
    }
    return () => {
      generation.current += 1;
      detailGeneration.current += 1;
      detailInFlight.current = new Map();
      itemsGeneration.current += 1;
      itemsInFlight.current = new Map();
      refundGeneration.current += 1;
      refundInFlight.current = undefined;
      effectsGeneration.current += 1;
      effectsInFlight.current = new Map();
    };
  }, [canRead, load, loadRefunds]);

  if (!canRead) return (
    <section className="route-card" aria-labelledby="app-title">
      <h1 id="app-title">订单总览</h1>
      <p>当前账号没有订单总览访问权限。</p>
    </section>
  );
  return <OrdersContent
    state={state}
    detail={detail}
    items={items}
    refunds={refunds}
    effects={effects}
    onLoad={(offset) => void load(offset)}
    onLoadDetail={(item) => void loadDetail(item)}
    onLoadItems={(item) => void loadItems(item)}
    onLoadRefunds={(offset) => void loadRefunds(offset)}
    onLoadEffects={(detailValue) => void loadEffects(detailValue)}
  />;
}
