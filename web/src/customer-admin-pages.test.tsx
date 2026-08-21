import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  CustomerAdminContextPage,
  CustomerAdminDetailPage,
} from "./customer-admin-pages";

describe("customer admin page aliases", () => {
  it("wraps the existing detail page with safe list and 360 navigation", () => {
    const html = renderToStaticMarkup(
      <CustomerAdminDetailPage customerID={42} />,
    );

    expect(html).toContain('href="/admin/customers"');
    expect(html).toContain('href="/admin/customers/42"');
    expect(html).toContain('href="/admin/customer-360/42"');
    expect(html).toContain("客户详情");
  });

  it("reuses the existing context panel rather than a second read model", () => {
    const html = renderToStaticMarkup(
      <CustomerAdminContextPage customerID={42} />,
    );

    expect(html).toContain("Customer 360 安全摘要");
    expect(html).toContain('href="/admin/customers"');
    expect(html).toContain('href="/admin/customers/42"');
    expect(html).toContain("正在读取安全本地投影…");
  });
});
