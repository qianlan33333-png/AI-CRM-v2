import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { SegmentCampaignEntry, SegmentsPage } from "./segments-ui";
import type { SegmentTransport } from "./segments";

function transport(): SegmentTransport {
  const unavailable = async () => ({ status: 503, data: {} });
  return { list: vi.fn(unavailable), create: vi.fn(unavailable), update: vi.fn(unavailable), members: vi.fn(unavailable), refresh: vi.fn(unavailable) } as unknown as SegmentTransport;
}

describe("SegmentsPage shell", () => {
  it.each(["admin", "ops"] as const)("renders the complete UI flow shell for %s", (role) => {
    const html = renderToStaticMarkup(<SegmentsPage role={role} transport={transport()} />);
    expect(html).toContain('<h1 id="app-title">人群包</h1>');
    expect(html).toContain("人群包列表");
    expect(html).toContain("条件编辑器");
    expect(html).toContain("组合 AND/OR 条件");
    expect(html).toContain("成员预览");
    expect(html).toContain("手动刷新");
    expect(html).toContain("不接受 JSON、标识符或 SQL 文本");
    expect(html).toContain('role="status"');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
    expect(html).not.toContain("aicrm_csrf");
  });

  it("keeps sales fail-closed without rendering data controls", () => {
    const client = transport();
    const html = renderToStaticMarkup(<SegmentsPage role="sales" transport={client} />);
    expect(html).toContain("当前账号没有人群包访问权限。");
    expect(html).not.toContain("条件编辑器");
    expect(html).not.toContain("成员预览");
    expect(client.list).not.toHaveBeenCalled();
  });

  it("uses only the selected SegmentRecord ID for the Campaign entry", () => {
    const html = renderToStaticMarkup(
      <SegmentCampaignEntry
        segment={{ id: 42 }}
      />,
    );
    expect(html).toContain(
      'href="/admin/cloud-orchestrator/campaigns?source_kind=segment_members&amp;source_id=42"',
    );
    expect(html).not.toContain("成员");
  });
});
