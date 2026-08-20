import React from "react";
import {
  audiencePackageWorkspaceLinks,
  type AudiencePackageRole,
  type AudiencePackageRoute,
} from "./audience-packages";

function WorkspaceNavigation() {
  return (
    <nav aria-label="人群包工作区导航">
      {audiencePackageWorkspaceLinks.map((link) => (
        <a href={link.href} key={link.href}>
          {link.label}
        </a>
      ))}
    </nav>
  );
}

function PackageListWorkspace() {
  return (
    <section aria-labelledby="audience-packages-title">
      <h1 id="audience-packages-title">AI Audience 人群包</h1>
      <p>这里承载已授权的人群包管理工作区，不推断任何人群、成员或发送事实。</p>
      <section aria-labelledby="audience-package-list-title">
        <h2 id="audience-package-list-title">列表、分组与浏览</h2>
        <p>列表、分组、筛选、分页和详情导航将在对应本地读模型合同完成后接入。</p>
      </section>
      <section aria-labelledby="audience-package-lifecycle-title">
        <h2 id="audience-package-lifecycle-title">分组与生命周期边界</h2>
        <p>新建、重命名、删除分组，以及复制、停用、启用和归档均为本地命令；当前载体不执行这些操作。</p>
      </section>
    </section>
  );
}

function PackageDetailWorkspace({ packageID }: { readonly packageID: string }) {
  return (
    <section aria-labelledby="audience-packages-title">
      <h1 id="audience-packages-title">AI Audience 人群包明细</h1>
      <p>本地人群包标识：{packageID}</p>
      <section aria-labelledby="audience-package-config-title">
        <h2 id="audience-package-config-title">基础配置与筛选模板</h2>
        <p>名称、分组、刷新方式、模板、筛选条件和预览必须由闭合本地合同提供；这里不制造成员数或刷新状态。</p>
      </section>
      <section aria-labelledby="audience-package-binding-title">
        <h2 id="audience-package-binding-title">话术绑定与发送人白名单</h2>
        <p>绑定和白名单仅表示本地计划关系，不证明发送权限、Provider 状态或任何消息已经发送。</p>
      </section>
      <section aria-labelledby="audience-package-members-title">
        <h2 id="audience-package-members-title">成员预览</h2>
        <p>成员读取合同尚未接入；external_userid 不会被当作 v2 客户主键或已验证身份。</p>
      </section>
      <section aria-labelledby="audience-package-records-title">
        <h2 id="audience-package-records-title">发送记录</h2>
        <p>记录、技术尝试、Provider 回执与最终送达必须分别展示；没有回执时不会标记为已送达。</p>
      </section>
    </section>
  );
}

export function AudiencePackageWorkspace({
  role,
  route,
}: {
  readonly role: AudiencePackageRole;
  readonly route: AudiencePackageRoute;
}) {
  if (role !== "admin") {
    return (
      <section aria-labelledby="audience-packages-title">
        <h1 id="audience-packages-title">AI Audience 人群包</h1>
        <p role="alert">当前角色无权访问此工作区。</p>
      </section>
    );
  }
  return (
    <>
      <WorkspaceNavigation />
      {route.kind === "packages" ? (
        <PackageListWorkspace />
      ) : (
        <PackageDetailWorkspace packageID={route.packageID} />
      )}
    </>
  );
}
