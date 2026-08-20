import React from "react";
import {
  CLOUD_ORCHESTRATOR_PLANS_PATH,
  cloudOrchestratorWorkspaceLinks,
  type CloudOrchestratorRole,
  type CloudOrchestratorRoute,
} from "./cloud-orchestrator";

export function CloudOrchestratorBoundary(): React.ReactElement {
  return (
    <p role="note">
      此处仅承载本地审阅与导航。草稿、待审阅、页面可见或本地统计均不表示
      Provider 已调用、外部发送已执行或消息已送达。
    </p>
  );
}

export function CloudOrchestratorNavigation(): React.ReactElement {
  return (
    <nav aria-label="AI 助手工作区">
      {cloudOrchestratorWorkspaceLinks.map((link) => (
        <a key={link.href} href={link.href}>
          {link.label}
        </a>
      ))}
    </nav>
  );
}

function PlansWorkspace(): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">运营计划审阅</h1>
      <CloudOrchestratorBoundary />
      <p role="status">
        页面载体已就绪；计划列表必须由后续冻结的本地只读合同提供，当前不猜测计划、受众或审批字段。
      </p>
    </section>
  );
}

function PlanDetailWorkspace({
  planID,
}: {
  readonly planID: string;
}): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">运营计划明细</h1>
      <CloudOrchestratorBoundary />
      <dl>
        <dt>计划标识</dt>
        <dd>{planID}</dd>
      </dl>
      <section aria-labelledby="cloud-plan-recipients">
        <h2 id="cloud-plan-recipients">目标人员审阅</h2>
        <p role="status">尚无已冻结的目标人员读模型，不展示或推断人员数据。</p>
      </section>
      <section aria-labelledby="cloud-plan-single-review">
        <h2 id="cloud-plan-single-review">单人审批</h2>
        <p role="status">尚无已冻结的单人审批合同，不提供可变更状态的操作。</p>
      </section>
      <a href={CLOUD_ORCHESTRATOR_PLANS_PATH}>返回运营计划</a>
    </section>
  );
}

function CampaignsWorkspace(): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">Campaign 审阅工作区</h1>
      <CloudOrchestratorBoundary />
      <p role="status">
        页面载体已就绪；素材、群聊或 Campaign 执行事实必须由各自 owner
        的已冻结合同接入。
      </p>
      <a href="/admin/cloud-orchestrator/observability">查看可观察性入口</a>
    </section>
  );
}

function ObservabilityWorkspace(): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">AI 助手可观察性</h1>
      <CloudOrchestratorBoundary />
      <div aria-label="可观察性入口">
        <section>
          <h2>工单</h2>
          <p>仅接入已冻结的本地工单读模型。</p>
        </section>
        <section>
          <h2>审计</h2>
          <p>仅接入已冻结的本地审计读模型。</p>
        </section>
        <section>
          <h2>漏斗</h2>
          <p>仅接入已冻结的本地漏斗读模型。</p>
        </section>
        <section>
          <h2>Tool 调用统计</h2>
          <p>仅接入已冻结的本地调用观测。</p>
        </section>
      </div>
    </section>
  );
}

export function CloudOrchestratorWorkspace({
  role,
  route,
}: {
  readonly role: CloudOrchestratorRole;
  readonly route: CloudOrchestratorRoute;
}): React.ReactElement {
  if (role !== "admin") {
    return (
      <section aria-labelledby="cloud-orchestrator-title">
        <h1 id="cloud-orchestrator-title">AI 助手</h1>
        <p role="alert">当前账号没有 AI 助手本地审阅权限。</p>
      </section>
    );
  }

  return (
    <main>
      <CloudOrchestratorNavigation />
      {route.kind === "root" || route.kind === "plans" ? (
        <PlansWorkspace />
      ) : null}
      {route.kind === "plan_detail" ? (
        <PlanDetailWorkspace planID={route.planID} />
      ) : null}
      {route.kind === "campaigns" ? <CampaignsWorkspace /> : null}
      {route.kind === "observability" ? <ObservabilityWorkspace /> : null}
    </main>
  );
}
