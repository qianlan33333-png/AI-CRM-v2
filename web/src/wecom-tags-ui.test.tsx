import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  copyWecomTagID,
  WecomTagDetails,
  WecomTagsPage,
  WecomTagsView,
} from "./wecom-tags-ui";
import type { WecomTagsTransport } from "./wecom-tags";

const catalog = {
  totalTags: 2,
  tagLimit: 1000,
  snapshotAt: "2026-08-19T00:00:00Z",
  groups: [
    {
      id: 1,
      name: "意向",
      sortOrder: 0,
      tags: [
        { id: 10, groupID: 1, groupName: "意向", name: "高意向", sortOrder: 0 },
        { id: 11, groupID: 1, groupName: "意向", name: "低意向", sortOrder: 1 },
      ],
    },
  ],
  tags: [
    { id: 10, groupID: 1, groupName: "意向", name: "高意向", sortOrder: 0 },
    { id: 11, groupID: 1, groupName: "意向", name: "低意向", sortOrder: 1 },
  ],
} as const;

function transport(): WecomTagsTransport {
  return {
    read: vi.fn(async () => ({ status: 503, data: {} })),
  } as unknown as WecomTagsTransport;
}

describe("WecomTagsView", () => {
  it.each(["admin", "ops"] as const)(
    "renders the read-only catalog for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <WecomTagsView role={role} state={{ kind: "ready", catalog }} />,
      );
      expect(html).toContain('<h1 id="app-title">企微标签目录</h1>');
      expect(html).toContain("标签总数");
      expect(html).toContain("标签上限");
      expect(html).toContain("本地目录快照时间（非企微同步）");
      expect(html).toContain("2026-08-19T00:00:00Z");
      expect(html).toContain("搜索标签组、标签名称或标签 ID");
      expect(html).toContain("高意向");
      expect(html).toContain("标签 ID");
      expect(html).toContain("上一页");
      expect(html).toContain("下一页");
      expect(html).toMatch(/<button[^>]*disabled=""[^>]*>上一页<\/button>/);
      expect(html).toMatch(/<button[^>]*disabled=""[^>]*>下一页<\/button>/);
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toMatch(/usage|sync|live/i);
      expect(html).not.toMatch(/csrf|provider|token|secret/i);
    },
  );

  it("clears the catalog for an unavailable result", () => {
    const html = renderToStaticMarkup(
      <WecomTagsView role="admin" state={{ kind: "error" }} />,
    );
    expect(html).toContain("企微标签目录暂不可用。");
    expect(html).not.toContain("高意向");
    expect(html).not.toContain("搜索标签组");
  });

  it("escapes tag text from the frozen catalog", () => {
    const html = renderToStaticMarkup(
      <WecomTagsView
        role="admin"
        state={{
          kind: "ready",
          catalog: {
            ...catalog,
            groups: [
              {
                ...catalog.groups[0],
                tags: [
                  {
                    ...catalog.groups[0].tags[0],
                    name: '<img src=x onerror="bad">',
                  },
                ],
              },
            ],
            tags: [
              {
                ...catalog.tags[0],
                name: '<img src=x onerror="bad">',
              },
            ],
          },
        }}
      />,
    );
    expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).not.toContain("<img");
  });

  it("keeps sales fail-closed without issuing a request", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <WecomTagsPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有企微标签目录访问权限。");
    expect(html).not.toContain("搜索标签组");
    expect(html).not.toContain("标签总数");
    expect(client.read).not.toHaveBeenCalled();
  });

  it("renders only the frozen tag detail fields and keeps text escaped", () => {
    const html = renderToStaticMarkup(
      <WecomTagDetails
        copyStatus="idle"
        onCopy={vi.fn()}
        tag={{
          id: 10,
          name: '<img src=x onerror="bad">',
          groupName: "意向",
        }}
      />,
    );
    expect(html).toContain("标签详情");
    expect(html).toContain("标签名称");
    expect(html).toContain("标签 ID");
    expect(html).toContain("标签组名称");
    expect(html).toContain("复制标签 ID");
    expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).not.toContain("<img");
    expect(html).not.toMatch(/usage_count|使用次数/i);
  });

  it("reports copy success or failure once and leaves the displayed ID available for manual copy", async () => {
    const writeText = vi.fn(async () => undefined);
    await expect(copyWecomTagID(10, { writeText })).resolves.toBe("copied");
    expect(writeText).toHaveBeenCalledOnce();
    expect(writeText).toHaveBeenCalledWith("10");

    const failedWrite = vi.fn(async () => {
      throw new Error("denied");
    });
    await expect(copyWecomTagID(10, { writeText: failedWrite })).resolves.toBe(
      "failed",
    );
    expect(failedWrite).toHaveBeenCalledOnce();
    await expect(copyWecomTagID(10, undefined)).resolves.toBe("unavailable");

    const failed = renderToStaticMarkup(
      <WecomTagDetails
        copyStatus="failed"
        onCopy={vi.fn()}
        tag={{ id: 10, name: "高意向", groupName: "意向" }}
      />,
    );
    expect(failed).toContain("<dd>10</dd>");
    expect(failed).toContain("复制失败，请手工复制上方标签 ID。");
  });
});
