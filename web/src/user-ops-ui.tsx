import React from "react";
import { USER_OPS_PATH, type UserOpsRole, type UserOpsRoute } from "./user-ops";

export function UserOpsBoundary(): React.ReactElement {
  return (
    <p role="note">
      此工作区仅承载本地筛选、预览与人工审阅边界。选中、eligible、pending_review、task_id、sent_count 或 msgid
      均不证明 Provider 已调用、外部发送已执行或消息已送达。
    </p>
  );
}

function ReviewWorkflow(): React.ReactElement {
  return (
    <section aria-labelledby="user-ops-title">
      <h1 id="user-ops-title">用户运营批量审阅</h1>
      <UserOpsBoundary />
      <ol aria-label="用户运营安全审阅流程">
        <li>
          <h2>运营总览与筛选</h2>
          <p>仅接入已冻结的本地读模型；页面计数和 external_userid 不会被当作 v2 身份事实。</p>
        </li>
        <li>
          <h2>客户投影与免打扰边界</h2>
          <p>客户详情属于 CRM owner，免打扰属于受审计的本地命令；当前工作区不读取客户明细，也不修改免打扰状态。</p>
        </li>
        <li>
          <h2>导出边界</h2>
          <p>旧导出只有空结果存根；当前工作区不生成或声称导出了真实 PII。</p>
        </li>
        <li>
          <h2>内容与目标预览</h2>
          <p>预览必须默认排除免打扰和缺失 external_userid 的记录；预览不发送，也不创建任务。</p>
        </li>
        <li>
          <h2>AI 助手人工审阅</h2>
          <p>提交边界最多形成 draft 或 pending_review 候选；当前工作区不提交计划、不批准计划，也不执行发送。</p>
        </li>
        <li>
          <h2>发送记录与回执核验</h2>
          <p>任务、技术尝试、Provider 结果和最终送达分别核验；unknown_after_dispatch 不会自动标记为可重试或已送达。</p>
        </li>
      </ol>
      <a href={USER_OPS_PATH}>返回用户运营工作区</a>
    </section>
  );
}

export function UserOpsWorkspace({
  role,
  route,
}: {
  readonly role: UserOpsRole;
  readonly route: UserOpsRoute;
}): React.ReactElement {
  if (role !== "admin") {
    return (
      <section aria-labelledby="user-ops-title">
        <h1 id="user-ops-title">用户运营批量审阅</h1>
        <p role="alert">当前角色无权访问此工作区。</p>
      </section>
    );
  }
  return route.kind === "review_workspace" ? <ReviewWorkflow /> : <></>;
}
