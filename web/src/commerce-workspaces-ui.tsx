import React from "react";
import {
  commerceWorkspaceLinks,
  type CommerceWorkspaceRole,
  type CommerceWorkspaceRoute,
} from "./commerce-workspaces";
import {
  CommerceTransactionWorkspace,
  isCommerceTransactionRoute,
} from "./commerce-transactions-ui";
import { type OrdersTransport } from "./orders";

const workspaceCopy: Record<
  CommerceWorkspaceRoute["kind"],
  { readonly title: string; readonly status: string }
> = {
  alipay_transactions: {
    title: "支付宝交易",
    status: "交易列表载体已就绪；尚未接入已冻结的支付宝交易读模型。",
  },
  service_products: {
    title: "周期商品",
    status: "周期商品列表载体已就绪；尚未接入已冻结的周期商品读模型。",
  },
  service_product_new: {
    title: "新建周期商品",
    status: "新建载体已就绪；当前不猜测周期、权益或外部推送字段。",
  },
  service_product_edit: {
    title: "编辑周期商品",
    status: "编辑载体已就绪；当前不提供未冻结的保存或外部推送操作。",
  },
  service_product_data: {
    title: "周期商品会员数据",
    status: "会员数据载体已就绪；当前不猜测成员、共享视图或协作者字段。",
  },
  wechat_pay_product_new: {
    title: "新建微信支付商品",
    status: "新建载体已就绪；当前不猜测支付、库存或外部推送字段。",
  },
  wechat_pay_product_edit: {
    title: "编辑微信支付商品",
    status: "编辑载体已就绪；当前不提供未冻结的保存或 Provider 操作。",
  },
  wechat_pay_transactions: {
    title: "微信支付交易",
    status: "交易列表载体已就绪；尚未接入已冻结的支付交易读模型。",
  },
  wechat_pay_transaction: {
    title: "微信支付交易详情",
    status: "交易详情载体已就绪；当前不提供退款、重试或 Provider 操作。",
  },
  wechat_shop_transactions: {
    title: "微信小店交易",
    status: "交易列表载体已就绪；尚未接入已冻结的小店交易读模型。",
  },
  wechat_shop_transaction: {
    title: "微信小店交易详情",
    status: "交易详情载体已就绪；当前不提供退款、重试或 Provider 操作。",
  },
};

export function CommerceWorkspaceBoundary(): React.ReactElement {
  return (
    <p role="note">
      此工作区只承载同源本地页面。页面可见、资源标识或本地状态均不表示支付、退款、商品发布、Provider
      调用或外部效果已经发生。
    </p>
  );
}

export function CommerceWorkspaces({
  role,
  route,
  ordersTransport,
  onUnauthenticated,
}: {
  readonly role: CommerceWorkspaceRole;
  readonly route: CommerceWorkspaceRoute;
  readonly ordersTransport?: OrdersTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  if (role !== "admin") {
    return (
      <section aria-labelledby="commerce-workspace-title">
        <h1 id="commerce-workspace-title">交易与周期商品</h1>
        <p role="alert">当前账号没有交易与周期商品工作区权限。</p>
      </section>
    );
  }
  const copy = workspaceCopy[route.kind];
  return (
    <main>
      <nav aria-label="交易与周期商品工作区">
        {commerceWorkspaceLinks.map((link) => (
          <a key={link.href} href={link.href}>
            {link.label}
          </a>
        ))}
      </nav>
      {isCommerceTransactionRoute(route) ? (
        <CommerceTransactionWorkspace
          role={role}
          route={route}
          transport={ordersTransport}
          onUnauthenticated={onUnauthenticated}
        />
      ) : (
        <section aria-labelledby="commerce-workspace-title">
          <h1 id="commerce-workspace-title">{copy.title}</h1>
          <CommerceWorkspaceBoundary />
          {"resourceID" in route ? (
            <dl>
              <dt>资源标识</dt>
              <dd>{route.resourceID}</dd>
            </dl>
          ) : null}
          <p role="status">{copy.status}</p>
        </section>
      )}
    </main>
  );
}
