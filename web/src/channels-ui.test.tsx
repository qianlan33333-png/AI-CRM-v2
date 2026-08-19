import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { ChannelsPage, ChannelsView } from "./channels-ui";
import type { ChannelsTransport } from "./channels";

const items = [
  {
    id: 1,
    name: '<img src=x onerror="bad">',
    code: "course",
    status: "active" as const,
    assigneeCount: 0 as const,
    contactCount: 0 as const,
    createdAt: "2026-08-19T00:00:00Z",
    updatedAt: "2026-08-19T01:02:03Z",
  },
] as const;

function transport(): ChannelsTransport {
  return {
    read: vi.fn(async () => ({ status: 503, data: {} })),
  } as unknown as ChannelsTransport;
}

describe("ChannelsView", () => {
  it.each(["admin", "ops"] as const)(
    "renders exactly the read-only local fields for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <ChannelsView role={role} state={{ kind: "ready", items }} />,
      );
      expect(html).toContain('<h1 id="app-title">渠道列表</h1>');
      expect(html).toContain("搜索渠道名称或编码");
      expect(html).toContain("渠道状态");
      expect(html).toContain("渠道 ID");
      expect(html).toContain("本地分配人数");
      expect(html).toContain("本地进入人数");
      expect(html).toContain("2026-08-19T00:00:00Z");
      expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
      expect(html).not.toContain("<img");
      expect(html).not.toMatch(/welcome|owner|tag|material|link_url|share_url|copy_text/i);
      expect(html).not.toContain("<button");
      expect(html.match(/<h1\b/g)).toHaveLength(1);
    },
  );

  it("renders local empty and unavailable states without stale rows", () => {
    const empty = renderToStaticMarkup(
      <ChannelsView role="admin" state={{ kind: "ready", items: [] }} />,
    );
    expect(empty).toContain("当前没有本地渠道。");
    expect(empty).not.toContain("渠道 ID");

    const unavailable = renderToStaticMarkup(
      <ChannelsView
        role="admin"
        state={{ kind: "error", failure: "unavailable" }}
      />,
    );
    expect(unavailable).toContain("本地渠道列表暂不可用。");
    expect(unavailable).not.toContain("渠道 ID");

    const invalid = renderToStaticMarkup(
      <ChannelsView role="admin" state={{ kind: "error", failure: "invalid" }} />,
    );
    expect(invalid).toContain("渠道列表响应不符合已冻结合同。");
    expect(invalid).not.toContain("渠道 ID");
  });

  it("keeps sales fail-closed without issuing a read", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <ChannelsPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有渠道列表访问权限。");
    expect(html).not.toContain("搜索渠道名称或编码");
    expect(client.read).not.toHaveBeenCalled();
  });
});
