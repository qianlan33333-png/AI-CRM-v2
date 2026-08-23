import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AudiencePackageWorkspace } from "./audience-packages-ui";
import {
  AUDIENCE_PACKAGES_PATH,
  type AudiencePackageRoute,
} from "./audience-packages";

describe("AudiencePackageWorkspace", () => {
  it("renders both approved administrator workspaces without business actions", () => {
    const routes: readonly AudiencePackageRoute[] = [
      { kind: "packages", pathname: AUDIENCE_PACKAGES_PATH },
      {
        kind: "package_detail",
        pathname: "/admin/automation-conversion/packages/42",
        packageID: "42",
      },
    ];
    for (const route of routes) {
      const html = renderToStaticMarkup(
        <AudiencePackageWorkspace role="admin" route={route} />,
      );
      expect(html).toContain("AI Audience 人群包");
      expect(html).toContain('href="/admin/automation-conversion"');
      expect(html).not.toContain("<button");
      expect(html).not.toContain("<form");
      expect(html).not.toContain("<input");
      expect(html).not.toContain("已发送成功");
    }
  });

  it("makes the local and external-effect boundaries explicit", () => {
    const list = renderToStaticMarkup(
      <AudiencePackageWorkspace
        role="admin"
        route={{ kind: "packages", pathname: AUDIENCE_PACKAGES_PATH }}
      />,
    );
    expect(list).toContain("列表、分组与浏览");
    expect(list).toContain("分组与生命周期边界");
    expect(list).toContain("当前载体不执行这些操作");

    const detail = renderToStaticMarkup(
      <AudiencePackageWorkspace
        role="admin"
        route={{
          kind: "package_detail",
          pathname: "/admin/automation-conversion/packages/9007199254740993",
          packageID: "9007199254740993",
        }}
      />,
    );
    expect(detail).toContain("基础配置与筛选模板");
    expect(detail).toContain("话术绑定与发送人白名单");
    expect(detail).toContain("成员预览");
    expect(detail).toContain("发送记录");
    expect(detail).toContain("9007199254740993");
    expect(detail).toContain("不证明发送权限、Provider 状态或任何消息已经发送");
    expect(detail).toContain("没有回执时不会标记为已送达");
    const campaignDetail = renderToStaticMarkup(
      <AudiencePackageWorkspace
        role="admin"
        route={{
          kind: "package_detail",
          pathname: "/admin/automation-conversion/packages/42",
          packageID: "42",
        }}
      />,
    );
    expect(campaignDetail).toContain(
      'href="/admin/cloud-orchestrator/campaigns?source_kind=ai_audience_package_members&amp;source_id=42"',
    );
  });

  it.each(["ops", "sales"] as const)("denies the %s role", (role) => {
    const html = renderToStaticMarkup(
      <AudiencePackageWorkspace
        role={role}
        route={{ kind: "packages", pathname: AUDIENCE_PACKAGES_PATH }}
      />,
    );
    expect(html).toContain("当前角色无权访问此工作区");
    expect(html).not.toContain("人群包工作区导航");
    expect(html).not.toContain("列表、分组与浏览");
  });
});
