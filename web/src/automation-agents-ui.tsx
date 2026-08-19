import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedAutomationAgentsTransport,
  loadAutomationAgents,
  type AutomationAgentsFailure,
  type AutomationAgentsRole,
  type AutomationAgentsSnapshot,
  type AutomationAgentsTransport,
  type AutomationAgentStatus,
  type AutomationAgentType,
  filterAutomationAgents,
} from "./automation-agents";

const messages: Record<AutomationAgentsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有自动化话术目录访问权限。",
  invalid: "自动化话术目录响应不符合已冻结的本地只读合同。",
  unavailable: "自动化话术目录暂时不可用，请稍后手动刷新。",
};

export type AutomationAgentsState =
  | { readonly kind: "loading"; readonly previous?: AutomationAgentsSnapshot }
  | { readonly kind: "ready"; readonly snapshot: AutomationAgentsSnapshot }
  | {
      readonly kind: "error";
      readonly failure: AutomationAgentsFailure;
      readonly previous?: AutomationAgentsSnapshot;
    };

export interface AutomationAgentsReadController {
  readonly generation: { current: number };
  readonly inFlight: { current: symbol | undefined };
  readonly verified: { current: AutomationAgentsSnapshot | undefined };
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onState: (state: AutomationAgentsState) => void;
  readonly onUnauthenticated?: () => void;
}

export function invalidateAutomationAgentsRead(
  controller: AutomationAgentsReadController,
): void {
  controller.generation.current += 1;
  controller.inFlight.current = undefined;
}

export async function startAutomationAgentsRead(
  controller: AutomationAgentsReadController,
  transport: AutomationAgentsTransport,
): Promise<void> {
  if (controller.inFlight.current) return;
  const token = Symbol("automation-agents-read");
  controller.inFlight.current = token;
  const currentGeneration = ++controller.generation.current;
  controller.onState({
    kind: "loading",
    previous: controller.verified.current,
  });
  try {
    const result = await loadAutomationAgents(transport);
    if (currentGeneration !== controller.generation.current) return;
    if (result.status === "loaded") {
      controller.verified.current = result.snapshot;
      controller.onState({ kind: "ready", snapshot: result.snapshot });
      return;
    }
    if (result.status === "unauthenticated") controller.onUnauthenticated?.();
    controller.onState({
      kind: "error",
      failure: result.status,
      previous: controller.verified.current,
    });
  } finally {
    if (controller.inFlight.current === token) controller.inFlight.current = undefined;
  }
}

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

function statusLabel(status: AutomationAgentStatus): string {
  return status === "active" ? "启用" : "暂停";
}

export function AutomationAgentsView({
  state,
  keyword,
  type,
  status,
  onKeywordChange,
  onTypeChange,
  onStatusChange,
  onLoad,
}: {
  readonly state: AutomationAgentsState;
  readonly keyword: string;
  readonly type: AutomationAgentType | "all";
  readonly status: AutomationAgentStatus | "all";
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onKeywordChange: (value: string) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onTypeChange: (value: AutomationAgentType | "all") => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onStatusChange: (value: AutomationAgentStatus | "all") => void;
  readonly onLoad: () => void;
}): React.ReactElement {
  const snapshot = state.kind === "ready" ? state.snapshot : state.previous;
  const items = snapshot ? filterAutomationAgents(snapshot, keyword, type, status) : [];
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">自动化话术 · 本地只读</p>
      <h1 id="app-title">自动化话术</h1>
      <p>只展示本地配置摘要；不读取 prompt、内容、素材 ID 或绑定详情，也不执行发布、复制或任何 Provider 调用。</p>
      <p>
        <label>
          本地搜索（名称或编码）
          <input
            value={keyword}
            onChange={(event) => onKeywordChange(event.currentTarget.value)}
          />
        </label>{" "}
        <label>
          类型
          <select
            value={type}
            onChange={(event) => onTypeChange(event.currentTarget.value as AutomationAgentType | "all")}
          >
            <option value="all">全部</option>
            <option value="agent">Agent 机器人</option>
            <option value="fixed_script">固定话术</option>
          </select>
        </label>{" "}
        <label>
          状态
          <select
            value={status}
            onChange={(event) => onStatusChange(event.currentTarget.value as AutomationAgentStatus | "all")}
          >
            <option value="all">全部</option>
            <option value="active">启用</option>
            <option value="paused">暂停</option>
          </select>
        </label>
      </p>
      {snapshot ? <p>已验证本地摘要共 {snapshot.total} 条，当前筛选 {items.length} 条。</p> : null}
      {items.length > 0 ? (
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>编码</th>
              <th>类型</th>
              <th>状态</th>
              <th>固定素材计数</th>
              <th>更新时间</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>{item.name}</td>
                <td>{item.code}</td>
                <td>{item.typeLabel}</td>
                <td>{statusLabel(item.status)}</td>
                <td>{`图片 ${item.materialSummary.imageCount}；小程序 ${item.materialSummary.miniprogramCount}；附件 ${item.materialSummary.attachmentCount}；群邀请 ${item.materialSummary.groupInviteCount}`}</td>
                <td>{displayDate(item.updatedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : snapshot ? (
        <p role="status">没有符合当前本地筛选条件的话术。</p>
      ) : null}
      {state.kind === "loading" ? <p role="status">正在读取本地自动化话术摘要。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      <p>
        <button type="button" disabled={state.kind === "loading"} onClick={onLoad}>
          手动刷新本地摘要
        </button>
      </p>
    </section>
  );
}

export function AutomationAgentsPage({
  role,
  transport = generatedAutomationAgentsTransport,
  onUnauthenticated,
}: {
  readonly role: AutomationAgentsRole;
  readonly transport?: AutomationAgentsTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0);
  const inFlight = useRef<symbol>();
  const verified = useRef<AutomationAgentsSnapshot>();
  const [state, setState] = useState<AutomationAgentsState>({ kind: "loading" });
  const [keyword, setKeyword] = useState("");
  const [type, setType] = useState<AutomationAgentType | "all">("all");
  const [status, setStatus] = useState<AutomationAgentStatus | "all">("all");
  const load = useCallback(
    () => startAutomationAgentsRead({ generation, inFlight, verified, onState: setState, onUnauthenticated }, transport),
    [onUnauthenticated, transport],
  );

  useEffect(() => {
    if (canRead) void load();
    return () => invalidateAutomationAgentsRead({ generation, inFlight, verified, onState: setState });
  }, [canRead, load]);

  if (!canRead) {
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">自动化话术</h1>
        <p>当前账号没有自动化话术目录访问权限。</p>
      </section>
    );
  }
  return (
    <AutomationAgentsView
      state={state}
      keyword={keyword}
      type={type}
      status={status}
      onKeywordChange={setKeyword}
      onTypeChange={setType}
      onStatusChange={setStatus}
      onLoad={() => { void load(); }}
    />
  );
}
