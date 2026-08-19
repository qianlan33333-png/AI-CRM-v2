import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { WeComCallbackInboxPage } from "./wecom-callback-inbox-ui";
import type { CallbackInboxTransport } from "./wecom-callback-inbox";

function transport(): CallbackInboxTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    detail: vi.fn(async () => ({ status: 503, data: {} })),
  } as CallbackInboxTransport;
}

describe("WeComCallbackInboxPage", () => {
  it("renders the admin local-audit boundary without callback payload or provider actions", () => {
    const html = renderToStaticMarkup(
      <WeComCallbackInboxPage role="admin" transport={transport()} />,
    );
    expect(html).toContain("企微回调本地审计");
    expect(html).toContain("不展示回调内容、身份标识或摘要");
    expect(html).toContain("已接受");
    expect(html).toContain("已拒绝");
    expect(html).not.toMatch(/payload|digest|identity|provider|发送|重试/i);
  });

  it.each(["ops", "sales"] as const)(
    "keeps %s from invoking callback reads",
    (role) => {
      const client = transport();
      const html = renderToStaticMarkup(
        <WeComCallbackInboxPage role={role} transport={client} />,
      );
      expect(html).toContain("当前账号没有本地回调审计权限。");
      expect(client.list).not.toHaveBeenCalled();
      expect(client.detail).not.toHaveBeenCalled();
    },
  );
});
