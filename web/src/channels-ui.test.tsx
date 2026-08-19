import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  ChannelsPage,
  ChannelsView,
  performChannelStatusUpdate,
  startChannelStatusUpdate,
} from "./channels-ui";
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
    "renders local fields and a bounded status action for %s",
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
      expect(html).toContain("更新本地状态");
      expect(html).toContain("状态更新仅写入本地渠道，不执行外部渠道能力。");
      expect(html).toContain("2026-08-19T00:00:00Z");
      expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
      expect(html).not.toContain("<img");
      expect(html).not.toMatch(
        /welcome|owner|tag|material|link_url|share_url|copy_text/i,
      );
      expect(html).toContain(">查看本地配置</button>");
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
      <ChannelsView
        role="admin"
        state={{ kind: "error", failure: "invalid" }}
      />,
    );
    expect(invalid).toContain("渠道列表响应不符合已冻结合同。");
    expect(invalid).not.toContain("渠道 ID");
  });

  it("renders only safe local detail text without URL or QR activation", () => {
    const html = renderToStaticMarkup(
      <ChannelsView
        role="admin"
        state={{ kind: "ready", items }}
        detail={{
          kind: "ready",
          item: items[0],
          detail: {
            item: items[0], channelType: "qrcode", carrierType: "qrcode",
            sceneValue: "scene-1", welcomeMessage: "欢迎", hasAssignmentConfig: true,
            imageMaterialCount: 1, miniProgramMaterialCount: 0,
            attachmentMaterialCount: 0, groupInviteMaterialCount: 0,
          },
        }}
      />,
    );
    expect(html).toContain('data-testid="channel-detail"');
    expect(html).toContain("本地配置：&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).toContain("图片 1，小程序 0，附件 0，群邀请 0");
    expect(html).not.toMatch(/href=|qr_url|share_url|copy_text|assignment_config_json/i);
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

  it("disables every status action while a table-wide update is in flight", () => {
    const html = renderToStaticMarkup(
      <ChannelsView busy role="admin" state={{ kind: "ready", items }} />,
    );
    expect(html).toMatch(/更新.*本地状态/);
    expect(html.match(/disabled=""/g)?.length).toBeGreaterThanOrEqual(3);
  });

  it("uses the full-page lock and a valid CSRF cookie before dispatching a status update", async () => {
    const client: ChannelsTransport = {
      read: vi.fn(async () => ({
        status: 200,
        data: {
          ok: true,
          channels: [
            {
              id: 1,
              channel_name: "渠道",
              channel_code: "course",
              status: "inactive",
              assignee_count: 0,
              channel_contact_count: 0,
              created_at: "2026-08-19T00:00:00Z",
              updated_at: "2026-08-19T01:02:03Z",
            },
          ],
          reason: "channels_listed",
          source: "ai_crm_next",
        },
        headers: new Headers(),
      })),
      write: vi.fn(async () => ({
        status: 200,
        data: {},
        headers: new Headers(),
      })),
    } as unknown as ChannelsTransport;
    const result = await performChannelStatusUpdate({
      channelID: 1,
      status: "inactive",
      idempotencySource: {
        randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
      },
      readCookie: () => `aicrm_csrf=${"a".repeat(43)}`,
      transport: client,
    });
    expect(result.status).toBe("confirmed");
    expect(client.write).toHaveBeenCalledTimes(1);

    const lock = { current: false };
    let release: (() => void) | undefined;
    const first = startChannelStatusUpdate(
      lock,
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    expect(
      startChannelStatusUpdate(lock, async () => undefined),
    ).toBeUndefined();
    release?.();
    await first;
    expect(lock.current).toBe(false);

    await expect(
      performChannelStatusUpdate({
        channelID: 1,
        status: "inactive",
        readCookie: () => "aicrm_csrf=bad",
        transport: client,
      }),
    ).resolves.toEqual({ status: "forbidden" });
    expect(client.write).toHaveBeenCalledTimes(1);
  });
});
