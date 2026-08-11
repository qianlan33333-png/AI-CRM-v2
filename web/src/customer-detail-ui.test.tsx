import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CustomerDetailPage, parseProfileDraft } from "./customer-detail-ui";
import type { CustomerDetailSnapshot, CustomerDetailTransport } from "./customer-detail";

const snapshot: CustomerDetailSnapshot = {
  customer: {
    id: 7,
    name: "林小姐",
    gender: 1,
    stageID: 3,
    ownerStaffID: 11,
    channelID: 5,
    addedAt: "2026-08-12T00:00:00Z",
    lastInteractAt: "2026-08-12T01:00:00Z",
    isDeleted: false,
    createdAt: "2026-08-11T00:00:00Z",
    updatedAt: "2026-08-12T02:00:00Z",
  },
  tags: [{ id: 9, groupName: "意向", name: "已报名", sortOrder: 10 }],
  tagCatalog: [
    { id: 9, groupName: "意向", name: "已报名", sortOrder: 10 },
    { id: 10, groupName: "意向", name: "待联系", sortOrder: 20 },
  ],
  events: [
    {
      id: 12,
      eventType: "stage_changed",
      actor: "后台账号 #1",
      occurredAt: "2026-08-12T03:00:00Z",
    },
  ],
  eventsHaveMore: true,
};

function transport(): CustomerDetailTransport {
  return {
    get: vi.fn(),
    update: vi.fn(),
    setStage: vi.fn(),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    listEvents: vi.fn(),
    listTags: vi.fn(),
  } as unknown as CustomerDetailTransport;
}

describe("CustomerDetailPage", () => {
  it("renders accessible detail, profile, stage, tag, and timeline controls", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={snapshot}
        transport={transport()}
      />,
    );
    expect(html).toContain('<h1 id="app-title">客户详情</h1>');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
    expect(html).toContain("资料操作");
    expect(html).toContain("阶段编号");
    expect(html).toContain("添加标签");
    expect(html).toContain("时间线");
    expect(html).toContain("后台账号 #1");
    expect(html).toContain("仅展示最近 50 条，更多记录待后续加载。");
    expect(html).toContain("<fieldset");
    expect(html).toContain("<label");
    expect(html).not.toContain("aicrm_csrf");
    expect(html).not.toContain("X-CSRF-Token");
  });

  it("starts with an accessible loading status without requiring browser globals", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage customerID={7} transport={transport()} />,
    );
    expect(html).toContain("正在读取客户资料、标签和时间线");
    expect(html).toContain('role="status"');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
  });

  it.each([
    "javascript:alert(1)",
    "data:text/plain,unsafe",
    "ftp://assets.invalid/a",
    "https:assets.invalid/a",
    "https://name:secret@assets.invalid/a",
  ])("rejects an unsafe profile avatar URL %s", (avatarURL) => {
    expect(
      parseProfileDraft({
        name: "林小姐",
        avatarURL,
        gender: "1",
        ownerStaffID: "11",
        channelID: "5",
      }),
    ).toBeUndefined();
  });

  it("rejects an out-of-range profile gender before transport", () => {
    expect(
      parseProfileDraft({
        name: "林小姐",
        avatarURL: "https://assets.invalid/avatar.png",
        gender: "32768",
        ownerStaffID: "11",
        channelID: "5",
      }),
    ).toBeUndefined();
  });
});
