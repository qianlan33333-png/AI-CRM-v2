import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
  CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH,
  CLOUD_ORCHESTRATOR_PLANS_PATH,
  cloudOrchestratorRoute,
  type CloudOrchestratorRole,
} from "./cloud-orchestrator";
import { CloudOrchestratorWorkspace } from "./cloud-orchestrator-ui";

function render(
  pathname: string,
  role: CloudOrchestratorRole = "admin",
): string {
  const route = cloudOrchestratorRoute(pathname);
  if (!route) throw new Error("test route must be valid");
  return renderToStaticMarkup(
    <CloudOrchestratorWorkspace role={role} route={route} />,
  );
}

describe("CloudOrchestratorWorkspace", () => {
  it("renders a plan-list review carrier without inventing an approval action", () => {
    const html = render(CLOUD_ORCHESTRATOR_PLANS_PATH);
    expect(html).toContain("运营计划审阅");
    expect(html).toContain("当前不猜测计划、受众或审批字段");
    expect(html).not.toContain("<button");
    expect(html).not.toContain("<form");
    expect(html).not.toContain("action_token");
  });

  it("renders the exact plan identifier and closed target-person review sections", () => {
    const html = render(`${CLOUD_ORCHESTRATOR_PLANS_PATH}/plan%20one`);
    expect(html).toContain("plan one");
    expect(html).toContain("目标人员审阅");
    expect(html).toContain("单人审批");
    expect(html).toContain("不展示或推断人员数据");
    expect(html).not.toContain("<button");
  });

  it("renders the campaign workspace and observability navigation without execution claims", () => {
    const html = render(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH);
    expect(html).toContain("Campaign 审阅工作区");
    expect(html).toContain('href="/admin/cloud-orchestrator/observability"');
    expect(html).toContain("Provider 已调用");
    expect(html).toContain("外部发送已执行");
  });

  it("renders only the four approved observability entry labels", () => {
    const html = render(CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH);
    for (const label of ["工单", "审计", "漏斗", "Tool 调用统计"]) {
      expect(html).toContain(label);
    }
    expect(html).not.toContain("成功率");
    expect(html).not.toContain("质量分");
  });

  it.each(["ops", "sales"] as const)("fails closed for the %s role", (role) => {
    const html = render(CLOUD_ORCHESTRATOR_PLANS_PATH, role);
    expect(html).toContain("没有 AI 助手本地审阅权限");
    expect(html).not.toContain("运营计划审阅");
  });
});
