import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CRMTagCatalogPage } from "./crm-tags-ui";
describe("CRMTagCatalogPage", () => {
  it("keeps sales fail-closed and does not expose mutations", () => { const html = renderToStaticMarkup(<CRMTagCatalogPage role="sales" />); expect(html).toContain("没有本地标签目录访问权限"); expect(html).not.toContain("新建"); });
  it("does not send without the generated local catalog transport", () => { const html = renderToStaticMarkup(<CRMTagCatalogPage role="admin" />); expect(html).toContain("尚未接入"); expect(html).toContain("未发送任何请求"); });
});
