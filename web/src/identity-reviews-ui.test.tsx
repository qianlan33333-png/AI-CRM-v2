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
    "renders the complete review flow shell for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <IdentityMergeReviewsPage role={role} transport={transport()} />,
      );
      expect(html).toContain('<h1 id="app-title">人工待合并</h1>');
      expect(html).toContain("待合并列表");
      expect(html).toContain("审阅与决策");
      expect(html).toContain("批准并合并");
      expect(html).toContain("拒绝合并");
      expect(html).toContain("只展示去标识指纹与候选 OneID");
      expect(html).toContain('role="status"');
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toContain("aicrm_csrf");
    },
  );

  it("keeps sales fail-closed without rendering review controls", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <IdentityMergeReviewsPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有人工待合并的访问权限。");
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
