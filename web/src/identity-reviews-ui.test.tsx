import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  IdentityMergeReviewsPage,
  newIdentityReviewCommandKeys,
} from "./identity-reviews-ui";
import type { IdentityReviewTransport } from "./identity-reviews";

function transport(): IdentityReviewTransport {
  const unavailable = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(unavailable),
    approve: vi.fn(unavailable),
    reject: vi.fn(unavailable),
  } as IdentityReviewTransport;
}

describe("IdentityMergeReviewsPage shell", () => {
  it.each(["admin", "ops"] as const)(
    "renders all three review partitions for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <IdentityMergeReviewsPage role={role} transport={transport()} />,
      );
      expect(html).toContain('<h1 id="app-title">OneID 人工审核</h1>');
      expect(html).toContain("待审核");
      expect(html).toContain("已批准");
      expect(html).toContain("已拒绝");
      expect(html).toContain("待审核列表");
      expect(html).toContain("审阅与决策");
      expect(html).toContain("仅展示封闭审核事实与候选 OneID");
      expect(html).toContain('role="status"');
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toContain("aicrm_csrf");
    },
  );

  it("renders approved history read-only without decision controls", () => {
    const html = renderToStaticMarkup(
      <IdentityMergeReviewsPage
        role="admin"
        initialStatus="approved"
        transport={transport()}
      />,
    );
    expect(html).toContain("已批准列表");
    expect(html).toContain("只读审核历史");
    expect(html).not.toContain("批准并合并");
    expect(html).not.toContain("拒绝合并");
    expect(html).not.toContain("primary-customer");
  });

  it("keeps sales fail-closed without rendering review controls", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <IdentityMergeReviewsPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有人工审核的访问权限。");
    expect(html).not.toContain("批准并合并");
    expect(client.list).not.toHaveBeenCalled();
  });

  it("creates separate valid command keys for approve and reject", () => {
    const keys = newIdentityReviewCommandKeys();
    expect(keys.approve).not.toBe(keys.reject);
    expect(keys.approve.length).toBeGreaterThanOrEqual(16);
    expect(keys.reject.length).toBeLessThanOrEqual(128);
  });
});
