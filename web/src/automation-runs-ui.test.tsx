import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { AutomationRunsPage } from "./automation-runs-ui";
import type { AutomationRunsTransport } from "./automation-runs";

function transport(): AutomationRunsTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
  } as AutomationRunsTransport;
}

describe("AutomationRunsPage shell", () => {
  it("renders the local masked receipt shell for admin only", () => {
    const html = renderToStaticMarkup(
      <AutomationRunsPage role="admin" transport={transport()} />,
    );
    expect(html).toContain('<h1 id="app-title">自动化运行记录</h1>');
    expect(html).toContain("只读脱敏收据");
    expect(html).toContain("正在读取自动化运行记录。");
    expect(html).toContain("不代表任何企业微信、支付或其他外部效果已经执行或成功");
    expect(html).not.toContain("unionid");
    expect(html).not.toContain("userid");
  });

  it.each(["ops", "sales"] as const)(
    "keeps %s fail-closed without issuing a read",
    (role) => {
      const client = transport();
      const html = renderToStaticMarkup(
        <AutomationRunsPage role={role} transport={client} />,
      );
      expect(html).toContain("当前账号没有自动化运行记录访问权限。");
      expect(html).not.toContain("正在读取自动化运行记录。");
      expect(client.list).not.toHaveBeenCalled();
    },
  );
});
