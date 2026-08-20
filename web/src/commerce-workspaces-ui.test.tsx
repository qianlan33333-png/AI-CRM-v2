import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  SERVICE_PRODUCTS_PATH,
  WECHAT_PAY_TRANSACTIONS_PATH,
  commerceWorkspaceRoute,
  type CommerceWorkspaceRole,
} from "./commerce-workspaces";
import { CommerceWorkspaces } from "./commerce-workspaces-ui";

function render(
  pathname: string,
  role: CommerceWorkspaceRole = "admin",
): string {
  const route = commerceWorkspaceRoute(pathname);
  if (!route) throw new Error("test route must be valid");
  return renderToStaticMarkup(<CommerceWorkspaces role={role} route={route} />);
}

describe("CommerceWorkspaces", () => {
  it("renders the cycle-product list carrier without inventing writes", () => {
    const html = render(SERVICE_PRODUCTS_PATH);
    expect(html).toContain("周期商品列表载体已就绪");
    expect(html).toContain("支付、退款、商品发布");
    expect(html).not.toContain("<button");
    expect(html).not.toContain("<form");
  });

  it("renders the exact transaction identifier while keeping refund closed", () => {
    const html = render(`${WECHAT_PAY_TRANSACTIONS_PATH}/order%20one`);
    expect(html).toContain("order one");
    expect(html).toContain("不提供退款、重试或 Provider 操作");
    expect(html).not.toContain("<button");
  });

  it.each(["ops", "sales"] as const)("fails closed for %s", (role) => {
    const html = render(SERVICE_PRODUCTS_PATH, role);
    expect(html).toContain("没有交易与周期商品工作区权限");
    expect(html).not.toContain("周期商品列表载体已就绪");
  });
});
