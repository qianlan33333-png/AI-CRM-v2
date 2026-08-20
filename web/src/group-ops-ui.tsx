import React from "react";
import {
  GROUP_OPS_PLANS_PATH,
  groupOpsWorkspaceLinks,
  type GroupOpsRole,
  type GroupOpsRoute,
} from "./group-ops";

function Boundary(): React.ReactElement {
  return (
    <p role="note">
      本工作区只承载本地页面与能力边界。页面可见、队列计数、计划状态或 webhook
      信息均不表示 Provider 已调用、外部任务已执行或消息已送达。
    </p>
  );
}

function Navigation(): React.ReactElement {
  return (
    <nav aria-label="群运营工作区">
      {groupOpsWorkspaceLinks.map((link) => (
        <a href={link.href} key={link.href}>
          {link.label}
        </a>
      ))}
    </nav>
  );
}

function PlansWorkspace(): React.ReactElement {
  return (
    <section aria-labelledby="group-ops-title">
      <h1 id="group-ops-title">群运营计划</h1>
      <Boundary />
      <section aria-labelledby="group-ops-list-title">
        <h2 id="group-ops-list-title">计划列表与筛选</h2>
        <p role="status">尚无已冻结的本地计划读模型，不展示或推断计划数据。</p>
      </section>
      <section aria-labelledby="group-ops-lifecycle-title">
        <h2 id="group-ops-lifecycle-title">计划生命周期</h2>
        <p>
          创建、启用、停用和归档命令尚未接入；本页不会同步运营人员或创建运行任务。
        </p>
      </section>
    </section>
  );
}

function PlanDetailWorkspace({ planID }: { readonly planID: string }): React.ReactElement {
  const capabilities = [
    ["计划基础配置", "名称、类型、运营成员与状态合同尚未接入，不读取或保存推测字段。"],
    ["标准编排节点", "节点、话术与素材合同尚未接入，不创建到期任务。"],
    ["Webhook 信息", "签名、allowlist 与地址合同尚未冻结，当前不展示或复制地址。"],
  ] as const;
  return (
    <section aria-labelledby="group-ops-title">
      <h1 id="group-ops-title">群运营计划明细</h1>
      <Boundary />
      <dl>
        <dt>计划标识</dt>
        <dd>{planID}</dd>
      </dl>
      {capabilities.map(([title, message]) => (
        <section aria-label={title} key={title}>
          <h2>{title}</h2>
          <p role="status">{message}</p>
        </section>
      ))}
      <a href={GROUP_OPS_PLANS_PATH}>返回计划工作区</a>
    </section>
  );
}

export function GroupOpsWorkspace({
  role,
  route,
}: {
  readonly role: GroupOpsRole;
  readonly route: GroupOpsRoute;
}): React.ReactElement {
  if (role !== "admin") {
    return (
      <section aria-labelledby="group-ops-title">
        <h1 id="group-ops-title">群运营计划</h1>
        <p role="alert">当前账号没有群运营本地工作区权限。</p>
      </section>
    );
  }

  return (
    <main>
      <Navigation />
      {route.kind === "plans" ? <PlansWorkspace /> : null}
      {route.kind === "plan_detail" ? (
        <PlanDetailWorkspace planID={route.planID} />
      ) : null}
    </main>
  );
}
