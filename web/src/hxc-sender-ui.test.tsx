import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { HXCSenderView } from "./hxc-sender-ui";

describe("HXCSenderView", () => {
  it("renders only the frozen local read-only projection", () => {
    const html = renderToStaticMarkup(
      <HXCSenderView
        role="admin"
        state={{
          status: "loaded",
          model: {
            sendConfigs: [],
            members: [
              {
                wecomUserID: "alice",
                displayName: "Alice",
                position: "",
                wecomStatus: 0,
                isSender: true,
                priority: 2,
                isActive: true,
              },
            ],
            directoryCount: 1,
            senderCount: 1,
            activeSenderCount: 1,
            lastSyncedAt: "2026-08-19T00:00:00Z",
            warning:
              "HXC senders use the local staff projection; no WeCom directory call was executed.",
            emptyState: false,
          },
        }}
      />,
    );

    expect(html).toContain('<h1 id="app-title">HXC 发件人配置</h1>');
    expect(html).toContain("只读本地投影");
    expect(html).toContain("可用目录");
    expect(html).toContain("企业微信 ID");
    expect(html).toContain("Alice");
    expect(html).not.toMatch(/refresh|upsert|delete|button/i);
    expect(html).not.toMatch(/mobile|avatar|department|provider|token|secret/i);
  });

  it("shows empty and unavailable states without action controls", () => {
    const empty = renderToStaticMarkup(
      <HXCSenderView
        role="admin"
        state={{
          status: "loaded",
          model: {
            sendConfigs: [],
            members: [],
            directoryCount: 0,
            senderCount: 0,
            activeSenderCount: 0,
            lastSyncedAt: "",
            warning:
              "HXC senders use the local staff projection; no WeCom directory call was executed.",
            emptyState: true,
          },
        }}
      />,
    );
    const unavailable = renderToStaticMarkup(
      <HXCSenderView role="admin" state={{ status: "unavailable" }} />,
    );
    expect(empty).toContain("当前没有可用本地成员。");
    expect(unavailable).toContain("本地发件人投影暂不可用。");
    expect(`${empty}${unavailable}`).not.toContain("<button");
  });
});
