import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App, carrierPathname, routeForPathname, routeForURL } from "./main";
import {
  ADMIN_CUSTOMER_LIST_PATH,
  customerPageRoute,
} from "./customer-admin-routes";
import type { AuthRole } from "./auth";

afterEach(() => vi.unstubAllGlobals());

function renderAppAt(
  pathname: string,
  search = "",
  role: AuthRole = "admin",
): string {
  vi.stubGlobal("window", { location: { pathname, search } });
  return renderToStaticMarkup(
    <App
      initialSession={{
        status: "authenticated",
        principal: {
          adminUserID: 7,
          role,
          ...(role === "sales" ? { staffID: 31 } : {}),
        },
      }}
    />,
  );
}

describe("customer admin central aliases", () => {
  it("maps the admin list to the existing canonical CustomerListPage", () => {
    expect(routeForPathname(ADMIN_CUSTOMER_LIST_PATH)).toBe(
      routeForPathname("/customers"),
    );
    const html = renderAppAt(ADMIN_CUSTOMER_LIST_PATH);
    expect(html).toContain("正在读取客户列表");
    expect(html).toContain('href="/customers"');
  });

  it.each(["admin", "ops", "sales"] as const)(
    "renders the same closed detail alias for %s",
    (role) => {
      const html = renderAppAt("/admin/customers/42", "", role);
      expect(html).toContain("正在读取客户资料、标签和时间线");
      expect(html).toContain('href="/admin/customers"');
      expect(html).toContain('href="/admin/customer-360/42"');
    },
  );

  it("renders the standalone alias through the existing CustomerContextPanel", () => {
    const html = renderAppAt("/admin/customer-360/42");
    expect(html).toContain("Customer 360 安全摘要");
    expect(html).toContain("正在读取安全本地投影…");
    expect(html).toContain('href="/admin/customers"');
    expect(html).toContain('href="/admin/customers/42"');
  });

  it("hydrates the exact server carrier without broadening its query", () => {
    const search =
      "?legacy_admin_path=%2Fadmin%2Fcustomer-360%2F42";
    expect(carrierPathname("/", search)).toBe(
      "/admin/customer-360/42",
    );
    const html = renderAppAt("/", search);
    expect(html).toContain("Customer 360 安全摘要");
  });

  it.each([
    "/admin/customers/",
    "/admin/customers/0",
    "/admin/customers/-1",
    "/admin/customers/01",
    "/admin/customers/9007199254740992",
    "/admin/customers/not-a-number",
    "/admin/customers/42/extra",
    "/admin/customers/42\\extra",
    "/admin\\customers\\42",
    "/admin/customers/42%2Fextra",
    "/admin%2Fcustomers%2F42",
    "/admin%252Fcustomers%252F42",
    "/admin/customer-360",
    "/admin/customer-360/",
    "/admin/customer-360/0",
    "/admin/customer-360/01",
    "/admin/customer-360/42/extra",
    "/admin/customer-360/42%2Fextra",
    "/admin%2Fcustomer-360%2F42",
    "/admin%252Fcustomer-360%252F42",
  ])("renders a fixed missing state for malformed route %s", (pathname) => {
    expect(customerPageRoute(pathname)).toBeUndefined();
    const html = renderAppAt(pathname);
    expect(html).toContain("404");
    expect(html).toContain('href="/admin/customers"');
    expect(html).toContain("返回客户列表");
    expect(html).not.toContain(pathname);
  });

  it("fails closed while parsing malformed URLs", () => {
    for (const href of [
      "https://crm.example/admin/customers/42%2Fextra",
      "https://crm.example/admin/customers/42\\extra",
      "http://[::1",
    ]) {
      expect(routeForURL(href)).toBeUndefined();
    }
  });
});
