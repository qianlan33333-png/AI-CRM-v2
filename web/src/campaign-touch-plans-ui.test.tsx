import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CampaignTouchPlansWorkspace } from "./campaign-touch-plans-ui";
import type { CampaignTouchPlansTransport } from "./campaign-touch-plans";

const transport: CampaignTouchPlansTransport = {
  listCampaigns: vi.fn(),
  listAudiencePackages: vi.fn(),
  listPlans: vi.fn(),
  createPlan: vi.fn(),
  listRecipients: vi.fn(),
  getReview: vi.fn(),
  mutateReview: vi.fn(),
  getHandoff: vi.fn(),
  acceptHandoff: vi.fn(),
  reconcileHandoff: vi.fn(),
};

describe("CampaignTouchPlansWorkspace", () => {
  it.each(["admin", "ops"] as const)(
    "exposes the local review flow to %s without outbound claims",
    (role) => {
      const html = renderToStaticMarkup(
        <CampaignTouchPlansWorkspace role={role} transport={transport} />,
      );
      expect(html).toContain("Campaign 本地审核");
      expect(html).toContain("从人群包冻结 touch plan");
      expect(html).toContain("本地 held 交接");
      expect(html).toContain("canonical customer ID");
      expect(html).toContain("内部 Events delivery");
      expect(html).not.toContain("Provider 标识");
      expect(html).not.toContain("发送成功");
      expect(html).not.toContain("已送达");
    },
  );

  it("keeps sales outside the Campaign review flow", () => {
    const html = renderToStaticMarkup(
      <CampaignTouchPlansWorkspace role="sales" transport={transport} />,
    );
    expect(html).toContain("没有 Campaign 本地审阅权限");
    expect(html).not.toContain("从人群包冻结 touch plan");
    expect(transport.listCampaigns).not.toHaveBeenCalled();
  });
});
