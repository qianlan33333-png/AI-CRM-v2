import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  listLegacyAlipayTransactions,
  listLegacyWechatTransactions,
} from "./api/generated/health";
import {
  generatedOrdersTransport,
  loadOrderDetailReference,
  loadOrders,
  ORDER_PAGE_SIZE,
  type OrderDetail,
  type OrderListPage,
  type OrderProvider,
  type OrdersFailure,
  type OrdersTransport,
} from "./orders";
import {
  ALIPAY_TRANSACTIONS_PATH,
  WECHAT_PAY_TRANSACTIONS_PATH,
  WECHAT_SHOP_TRANSACTIONS_PATH,
  commerceWorkspaceRoute,
  type CommerceWorkspaceRole,
  type CommerceWorkspaceRoute,
} from "./commerce-workspaces";

export type CommerceTransactionRoute = Extract<
  CommerceWorkspaceRoute,
  {
    readonly kind:
      | "alipay_transactions"
      | "wechat_pay_transactions"
      | "wechat_pay_transaction"
      | "wechat_shop_transactions"
      | "wechat_shop_transaction";
  }
>;

export type CommerceTransactionListState =
  | { readonly kind: "loading"; readonly previous?: OrderListPage }
  | { readonly kind: "ready"; readonly page: OrderListPage }
  | {
      readonly kind: "error";
      readonly failure: OrdersFailure;
      readonly previous?: OrderListPage;
    };

export type CommerceTransactionDetailState =
  | { readonly kind: "loading"; readonly previous?: OrderDetail }
  | { readonly kind: "ready"; readonly detail: OrderDetail }
  | {
      readonly kind: "error";
      readonly failure: OrdersFailure;
      readonly previous?: OrderDetail;
    };

interface ListLoadController {
  readonly role: CommerceWorkspaceRole;
  readonly provider: OrderProvider;
  readonly offset: number;
  readonly transport: OrdersTransport;
  readonly generation: { current: number };
  readonly inFlight: { current: symbol | undefined };
  readonly verified: { current: OrderListPage | undefined };
  // eslint-disable-next-line no-unused-vars -- named transition value documents the state sink.
  readonly setState: (state: CommerceTransactionListState) => void;
  readonly onUnauthenticated?: () => void;
}

interface DetailLoadController {
  readonly role: CommerceWorkspaceRole;
  readonly provider: OrderProvider;
  readonly reference: string;
  readonly transport: OrdersTransport;
  readonly generation: { current: number };
  readonly inFlight: { current: symbol | undefined };
  readonly verified: { current: OrderDetail | undefined };
  // eslint-disable-next-line no-unused-vars -- named transition value documents the state sink.
  readonly setState: (state: CommerceTransactionDetailState) => void;
  readonly onUnauthenticated?: () => void;
}

const failureMessages: Record<OrdersFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有读取交易工作区的权限。",
  invalid: "本地交易投影不符合已冻结合同，未展示未验证数据。",
  unavailable: "本地交易投影暂时不可用，请稍后重试。",
};

// Preserve each legacy workspace's already-generated read entry. WeChat Shop
// intentionally uses the unified endpoint because no separate shop endpoint is
// frozen; the provider query remains mandatory and is revalidated on response.
export const generatedCommerceTransactionTransport: OrdersTransport = {
  ...generatedOrdersTransport,
  list: async (params, options) => {
    const page = { limit: params.limit, offset: params.offset };
    if (params.provider === "alipay")
      return listLegacyAlipayTransactions(page, options);
    if (params.provider === "wechat")
      return listLegacyWechatTransactions(page, options);
    return params.provider === "wechat_shop"
      ? generatedOrdersTransport.list(params, options)
      : { status: 400, data: {} };
  },
};

export function isCommerceTransactionRoute(
  route: CommerceWorkspaceRoute,
): route is CommerceTransactionRoute {
  return (
    route.kind === "alipay_transactions" ||
    route.kind === "wechat_pay_transactions" ||
    route.kind === "wechat_pay_transaction" ||
    route.kind === "wechat_shop_transactions" ||
    route.kind === "wechat_shop_transaction"
  );
}

export function commerceTransactionProvider(
  route: CommerceTransactionRoute,
): OrderProvider {
  if (route.kind === "alipay_transactions") return "alipay";
  return route.kind === "wechat_shop_transactions" ||
    route.kind === "wechat_shop_transaction"
    ? "wechat_shop"
    : "wechat";
}

function listPath(provider: OrderProvider): string {
  if (provider === "alipay") return ALIPAY_TRANSACTIONS_PATH;
  return provider === "wechat"
    ? WECHAT_PAY_TRANSACTIONS_PATH
    : WECHAT_SHOP_TRANSACTIONS_PATH;
}

export function commerceTransactionDetailPath(
  provider: OrderProvider,
  orderNo: string,
): string | undefined {
  if (provider === "alipay") return undefined;
  const path = `${listPath(provider)}/${encodeURIComponent(orderNo)}`;
  const route = commerceWorkspaceRoute(path);
  return route &&
    isCommerceTransactionRoute(route) &&
    "resourceID" in route &&
    route.resourceID === orderNo &&
    commerceTransactionProvider(route) === provider
    ? path
    : undefined;
}

// The carrier is admin-only even though the reusable Order API also permits
// ops. Guard before transport so a hidden carrier can never become a data path.
export function startCommerceTransactionListLoad(
  controller: ListLoadController,
): Promise<void> | undefined {
  if (controller.role !== "admin" || controller.inFlight.current)
    return undefined;
  const token = Symbol("commerce-transaction-page");
  controller.inFlight.current = token;
  const generation = ++controller.generation.current;
  const previous =
    controller.verified.current?.providerScope === controller.provider
      ? controller.verified.current
      : undefined;
  controller.setState({ kind: "loading", previous });
  return loadOrders(
    controller.transport,
    controller.offset,
    controller.provider,
  )
    .then((result) => {
      if (generation !== controller.generation.current) return;
      if (result.status === "loaded") {
        controller.verified.current = result.page;
        controller.setState({ kind: "ready", page: result.page });
        return;
      }
      if (result.status === "unauthenticated") controller.onUnauthenticated?.();
      controller.setState({ kind: "error", failure: result.status, previous });
    })
    .finally(() => {
      if (controller.inFlight.current === token)
        controller.inFlight.current = undefined;
    });
}

export function startCommerceTransactionDetailLoad(
  controller: DetailLoadController,
): Promise<void> | undefined {
  if (controller.role !== "admin" || controller.inFlight.current)
    return undefined;
  const token = Symbol("commerce-transaction-detail");
  controller.inFlight.current = token;
  const generation = ++controller.generation.current;
  const previous =
    controller.verified.current?.provider === controller.provider
      ? controller.verified.current
      : undefined;
  controller.setState({ kind: "loading", previous });
  return loadOrderDetailReference(
    controller.transport,
    controller.reference,
    controller.provider,
  )
    .then((result) => {
      if (generation !== controller.generation.current) return;
      if (result.status === "loaded") {
        controller.verified.current = result.detail;
        controller.setState({ kind: "ready", detail: result.detail });
        return;
      }
      if (result.status === "unauthenticated") controller.onUnauthenticated?.();
      controller.setState({ kind: "error", failure: result.status, previous });
    })
    .finally(() => {
      if (controller.inFlight.current === token)
        controller.inFlight.current = undefined;
    });
}

function LocalReadBoundary(): React.ReactElement {
  return (
    <p role="note">
      这里只读取已持久化的本地订单投影；页面状态不证明付款、退款、回调、同步或任何
      Provider 外部效果已经发生。本页没有支付、退款、导出、重试或 Provider
      操作。
    </p>
  );
}

export function CommerceTransactionListContent({
  provider,
  state,
  onLoad,
}: {
  readonly provider: OrderProvider;
  readonly state: CommerceTransactionListState;
  // eslint-disable-next-line no-unused-vars -- named offset documents pagination semantics.
  readonly onLoad: (offset: number) => void;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  return (
    <section aria-labelledby="commerce-workspace-title">
      <h1 id="commerce-workspace-title">
        {provider === "alipay"
          ? "支付宝交易"
          : provider === "wechat"
            ? "微信支付交易"
            : "微信小店交易"}
      </h1>
      <LocalReadBoundary />
      {page ? (
        <>
          <p>
            已验证本地交易共 {page.total} 条，当前偏移 {page.offset}。
          </p>
          {page.items.length === 0 ? (
            <p role="status">当前页没有本地交易。</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>订单号</th>
                  <th>商品</th>
                  <th>金额</th>
                  <th>状态</th>
                  <th>创建时间</th>
                  {provider === "alipay" ? null : <th>本地详情</th>}
                </tr>
              </thead>
              <tbody>
                {page.items.map((item) => {
                  const detailPath = commerceTransactionDetailPath(
                    provider,
                    item.orderNo,
                  );
                  return (
                    <tr key={item.orderNo}>
                      <td>{item.orderNo}</td>
                      <td>
                        {item.productName === ""
                          ? item.productCode
                          : `${item.productName}（${item.productCode}）`}
                      </td>
                      <td>
                        {item.amountYuan} {item.currency}
                      </td>
                      <td>{item.statusLabel}</td>
                      <td>
                        <time dateTime={item.createdAt}>{item.createdAt}</time>
                      </td>
                      {provider === "alipay" ? null : (
                        <td>
                          {detailPath ? (
                            <a href={detailPath}>查看本地详情</a>
                          ) : (
                            "标识不可安全导航"
                          )}
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </>
      ) : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取本地交易投影。</p>
      ) : null}
      {state.kind === "error" ? (
        <p role="alert">{failureMessages[state.failure]}</p>
      ) : null}
      <p>
        <button
          type="button"
          disabled={!page || page.offset === 0 || state.kind === "loading"}
          onClick={() =>
            onLoad(Math.max(0, (page?.offset ?? 0) - ORDER_PAGE_SIZE))
          }
        >
          上一页
        </button>{" "}
        <button
          type="button"
          disabled={!page?.hasMore || state.kind === "loading"}
          onClick={() => onLoad((page?.offset ?? 0) + ORDER_PAGE_SIZE)}
        >
          下一页
        </button>{" "}
        <button
          type="button"
          disabled={state.kind === "loading"}
          onClick={() => onLoad(page?.offset ?? 0)}
        >
          刷新本地投影
        </button>
      </p>
    </section>
  );
}

export function CommerceTransactionDetailContent({
  provider,
  state,
  onReload,
}: {
  readonly provider: OrderProvider;
  readonly state: CommerceTransactionDetailState;
  readonly onReload: () => void;
}): React.ReactElement {
  const detail = state.kind === "ready" ? state.detail : state.previous;
  return (
    <section aria-labelledby="commerce-workspace-title">
      <h1 id="commerce-workspace-title">
        {provider === "wechat" ? "微信支付交易详情" : "微信小店交易详情"}
      </h1>
      <LocalReadBoundary />
      {detail ? (
        <dl>
          <dt>本地订单 ID</dt>
          <dd>{detail.id}</dd>
          <dt>订单号</dt>
          <dd>{detail.orderNo}</dd>
          <dt>支付渠道</dt>
          <dd>{detail.providerLabel}</dd>
          <dt>商品</dt>
          <dd>
            {detail.productName === ""
              ? detail.productCode
              : `${detail.productName}（${detail.productCode}）`}
          </dd>
          <dt>订单金额</dt>
          <dd>
            {detail.amountYuan} {detail.currency}
          </dd>
          <dt>本地状态</dt>
          <dd>{detail.statusLabel}</dd>
          <dt>创建时间</dt>
          <dd>
            <time dateTime={detail.createdAt}>{detail.createdAt}</time>
          </dd>
        </dl>
      ) : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取本地交易详情。</p>
      ) : null}
      {state.kind === "error" ? (
        <p role="alert">{failureMessages[state.failure]}</p>
      ) : null}
      <p>
        <a href={listPath(provider)}>返回交易列表</a>{" "}
        <button
          type="button"
          disabled={state.kind === "loading"}
          onClick={onReload}
        >
          刷新本地详情
        </button>
      </p>
    </section>
  );
}

function CommerceTransactionListPage({
  role,
  route,
  transport,
  onUnauthenticated,
}: {
  readonly role: CommerceWorkspaceRole;
  readonly route: CommerceTransactionRoute;
  readonly transport: OrdersTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const provider = commerceTransactionProvider(route);
  const generation = useRef(0);
  const inFlight = useRef<symbol>();
  const verified = useRef<OrderListPage>();
  const [state, setState] = useState<CommerceTransactionListState>({
    kind: "loading",
  });
  const load = useCallback(
    (offset: number) =>
      startCommerceTransactionListLoad({
        role,
        provider,
        offset,
        transport,
        generation,
        inFlight,
        verified,
        setState,
        onUnauthenticated,
      }),
    [onUnauthenticated, provider, role, transport],
  );
  useEffect(() => {
    verified.current = undefined;
    void load(0);
    return () => {
      generation.current += 1;
      inFlight.current = undefined;
    };
  }, [load]);
  return (
    <CommerceTransactionListContent
      provider={provider}
      state={state}
      onLoad={(offset) => void load(offset)}
    />
  );
}

function CommerceTransactionDetailPage({
  role,
  route,
  transport,
  onUnauthenticated,
}: {
  readonly role: CommerceWorkspaceRole;
  readonly route: Extract<
    CommerceTransactionRoute,
    { readonly kind: "wechat_pay_transaction" | "wechat_shop_transaction" }
  >;
  readonly transport: OrdersTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const provider = commerceTransactionProvider(route);
  const generation = useRef(0);
  const inFlight = useRef<symbol>();
  const verified = useRef<OrderDetail>();
  const [state, setState] = useState<CommerceTransactionDetailState>({
    kind: "loading",
  });
  const load = useCallback(
    () =>
      startCommerceTransactionDetailLoad({
        role,
        provider,
        reference: route.resourceID,
        transport,
        generation,
        inFlight,
        verified,
        setState,
        onUnauthenticated,
      }),
    [onUnauthenticated, provider, role, route.resourceID, transport],
  );
  useEffect(() => {
    verified.current = undefined;
    void load();
    return () => {
      generation.current += 1;
      inFlight.current = undefined;
    };
  }, [load]);
  return (
    <CommerceTransactionDetailContent
      provider={provider}
      state={state}
      onReload={() => void load()}
    />
  );
}

export function CommerceTransactionWorkspace({
  role,
  route,
  transport = generatedCommerceTransactionTransport,
  onUnauthenticated,
}: {
  readonly role: CommerceWorkspaceRole;
  readonly route: CommerceTransactionRoute;
  readonly transport?: OrdersTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  if (
    route.kind === "wechat_pay_transaction" ||
    route.kind === "wechat_shop_transaction"
  )
    return (
      <CommerceTransactionDetailPage
        key={`${commerceTransactionProvider(route)}:${route.resourceID}`}
        role={role}
        route={route}
        transport={transport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  return (
    <CommerceTransactionListPage
      key={commerceTransactionProvider(route)}
      role={role}
      route={route}
      transport={transport}
      onUnauthenticated={onUnauthenticated}
    />
  );
}
