import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { GroupOpsWorkspace } from "./group-ops-ui";
import {
  GROUP_OPS_PLANS_PATH,
  type GroupOpsRoute,
} from "./group-ops";

describe("GroupOpsWorkspace", () => {
  it("renders the complete safe local workspace package without action controls", () => {
    const routes: readonly GroupOpsRoute[] = [
      { kind: "plans", pathname: GROUP_OPS_PLANS_PATH },
      {
        kind: "plan_detail",
        pathname: "/admin/automation-conversion/group-ops/plans/42",
        planID: "42",
      },
    ];
    const html = routes
      .map((route) =>
        renderToStaticMarkup(<GroupOpsWorkspace role="admin" route={route} />),
      )
      .join("\n");

    for (const capability of [
      "计划列表与筛选",
      "计划生命周期",
      "计划基础配置",
      "标准编排节点",
      "Webhook 信息",
    ]) {
      expect(html).toContain(capability);
    }
    expect(html).toContain("不表示 Provider 已调用");
    expect(html).not.toMatch(/<(button|form|input|select|textarea)\b/u);
    expect(html).not.toContain("join_url");
    expect(html).not.toContain("external_userid");
  });

  it.each(["ops", "sales"] as const)(
    "keeps %s fail-closed without workspace facts",
    (role) => {
      const html = renderToStaticMarkup(
        <GroupOpsWorkspace
          role={role}
          route={{ kind: "plans", pathname: GROUP_OPS_PLANS_PATH }}
        />,
      );
      expect(html).toContain("当前账号没有群运营本地工作区权限");
      expect(html).not.toContain("计划列表与筛选");
      expect(html).not.toContain('href="');
    },
  );

  it("renders only the canonical numeric plan ID as text", () => {
    const html = renderToStaticMarkup(
      <GroupOpsWorkspace
        role="admin"
        route={{
          kind: "plan_detail",
          pathname: "/admin/automation-conversion/group-ops/plans/42",
          planID: "42",
        }}
      />,
    );
    expect(html).toContain("<dd>42</dd>");
    expect(html.match(/<h1\b/gu)).toHaveLength(1);
  });
});
