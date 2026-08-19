import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  GroupInviteLibraryPage,
  GroupInviteLibraryView,
} from "./group-invite-library-ui";
import type { GroupInviteLibraryTransport } from "./group-invite-library";

function transport(): GroupInviteLibraryTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
  } as GroupInviteLibraryTransport;
}

describe("GroupInviteLibraryPage", () => {
  it.each(["admin", "ops"] as const)(
    "renders %s a local read-only group-invite library without exposing its join URL",
    (role) => {
      const html = renderToStaticMarkup(
        <GroupInviteLibraryPage role={role} transport={transport()} />,
      );
      expect(html).toContain('<h1 id="app-title">群邀请素材库</h1>');
      expect(html).toContain("本地群邀请卡元数据");
      expect(html).toContain("正在读取本地群邀请素材。");
      expect(html).not.toContain("入群地址");
      expect(html).not.toMatch(/href=|clipboard|provider|join_url/i);
    },
  );

  it("validates but never renders, copies, or links the join URL", () => {
    const html = renderToStaticMarkup(
      <GroupInviteLibraryView
        onLoad={vi.fn()}
        state={{
          kind: "ready",
          page: {
            items: [
              {
                id: 7,
                name: "体验群",
                title: "加入体验群",
                description: "本地说明",
                joinURL: "https://work.weixin.qq.com/gm/strictly-validated-token",
                coverImageID: 19,
                enabled: true,
                createdAt: "2026-08-19T08:00:00Z",
                updatedAt: "2026-08-19T08:00:01Z",
              },
            ],
            total: 1,
            limit: 100,
            offset: 0,
          },
        }}
      />,
    );
    expect(html).toContain("体验群");
    expect(html).toContain("创建时间");
    expect(html).toContain("更新时间");
    expect(html).not.toContain("strictly-validated-token");
    expect(html).not.toMatch(/join_url|入群地址|href=|clipboard|provider/i);
  });

  it("keeps sales fail-closed without issuing a list request", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <GroupInviteLibraryPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有群邀请素材库访问权限。");
    expect(html).not.toContain("正在读取本地群邀请素材。");
    expect(client.list).not.toHaveBeenCalled();
  });
});
