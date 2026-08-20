import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { UserOpsWorkspace } from "./user-ops-ui";
import { USER_OPS_PATH, USER_OPS_UI_PATH } from "./user-ops";

describe("UserOpsWorkspace", () => {
  it.each([USER_OPS_PATH, USER_OPS_UI_PATH] as const)(
    "renders the complete safe review workflow for %s",
    (pathname) => {
      const html = renderToStaticMarkup(
        <UserOpsWorkspace
          role="admin"
          route={{ kind: "review_workspace", pathname }}
        />,
      );
      for (const heading of [
        "运营总览与筛选",
        "客户投影与免打扰边界",
        "导出边界",
        "内容与目标预览",
        "AI 助手人工审阅",
        "发送记录与回执核验",
      ]) {
        expect(html).toContain(heading);
      }
      expect(html).toContain('href="/admin/user-ops"');
    },
  );

  it("keeps CRM, PII, task and external-effect boundaries closed", () => {
    const html = renderToStaticMarkup(
      <UserOpsWorkspace
        role="admin"
        route={{ kind: "review_workspace", pathname: USER_OPS_PATH }}
      />,
    );
    expect(html).toContain("不读取客户明细，也不修改免打扰状态");
    expect(html).toContain("不生成或声称导出了真实 PII");
    expect(html).toContain("预览不发送，也不创建任务");
    expect(html).toContain("不提交计划、不批准计划，也不执行发送");
    expect(html).toContain("unknown_after_dispatch 不会自动标记为可重试或已送达");
    expect(html).not.toContain("<button");
    expect(html).not.toContain("<form");
    expect(html).not.toContain("<input");
  });

  it.each(["ops", "sales"] as const)("denies the %s role", (role) => {
    const html = renderToStaticMarkup(
      <UserOpsWorkspace
        role={role}
        route={{ kind: "review_workspace", pathname: USER_OPS_PATH }}
      />,
    );
    expect(html).toContain("当前角色无权访问此工作区");
    expect(html).not.toContain("运营总览与筛选");
  });
});
