import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ProductsPage } from "./products-ui";

describe("ProductsPage", () => {
  it("does not render a products reader for sales", () => {
    expect(renderToStaticMarkup(<ProductsPage role="sales" />)).toBe("");
  });
  it("states its local-only and non-payment boundary", () => {
    const html = renderToStaticMarkup(<ProductsPage role="admin" transport={{ list: async () => ({ status: 503, data: {} }) }} />);
    expect(html).toContain("不展示图片链接");
    expect(html).toContain("不执行支付");
  });
});
