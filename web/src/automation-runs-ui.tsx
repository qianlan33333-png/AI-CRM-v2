import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedAutomationRunsTransport,
  loadAutomationRuns,
  nextAutomationRunsPage,
  previousAutomationRunsPage,
  type AutomationRunsFailure,
  type AutomationRunsPage,
  type AutomationRunsRole,
  type AutomationRunsTransport,
} from "./automation-runs";

const messages: Record<AutomationRunsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有自动化运行记录访问权限。",
  invalid: "自动化运行记录响应不符合脱敏只读契约。",
  unavailable: "自动化运行记录暂时不可用，请稍后重试。",
};

export type AutomationRunsState =
  | { readonly kind: "loading"; readonly previous?: AutomationRunsPage }
  | { readonly kind: "ready"; readonly page: AutomationRunsPage }
  | {
      readonly kind: "error";
      readonly failure: AutomationRunsFailure;
      readonly previous?: AutomationRunsPage;
    };

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

function RunRows({ page }: { readonly page: AutomationRunsPage }): React.ReactElement {
  return (
    <>
      <p>
        已验证的脱敏收据：共 {page.total} 条，第 {page.page} 页。
      </p>
      {page.items.length === 0 ? (
        <p role="status">当前页没有自动化运行记录。</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>运行收据</th>
              <th>触发来源</th>
              <th>客户 OneID</th>
              <th>标签 ID</th>
              <th>开始时间</th>
              <th>完成时间</th>
            </tr>
          </thead>
          <tbody>
            {page.items.map((run) => (
              <tr key={run.runID}>
                <td>{run.runID}</td>
                <td>{run.triggerSource}</td>
                <td>{run.customerID}</td>
                <td>{run.tagID}</td>
                <td>{displayDate(run.startedAt)}</td>
                <td>{displayDate(run.completedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

function AutomationRunsContent({
  onLoad,
  state,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoad: (page: number) => void;
  readonly state: AutomationRunsState;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">自动化运营 · 只读脱敏收据</p>
      <h1 id="app-title">自动化运行记录</h1>
      <p>
        仅显示服务端返回的脱敏触发收据。该记录不代表任何企业微信、支付或其他外部效果已经执行或成功。
      </p>
      {page ? <RunRows page={page} /> : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取自动化运行记录。</p>
      ) : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      {state.kind === "error" && !page ? (
        <button type="button" onClick={() => onLoad(1)}>
          重试读取
        </button>
      ) : null}
      {page ? (
        <p>
          <button
            type="button"
            disabled={state.kind === "loading" || !previousAutomationRunsPage(page)}
            onClick={() => {
              const previous = previousAutomationRunsPage(page);
              if (previous) onLoad(previous);
            }}
          >
            上一页
          </button>{" "}
          <button
            type="button"
            disabled={state.kind === "loading" || !nextAutomationRunsPage(page)}
            onClick={() => {
              const next = nextAutomationRunsPage(page);
              if (next) onLoad(next);
            }}
          >
            下一页
          </button>{" "}
          <button
            type="button"
            disabled={state.kind === "loading"}
            onClick={() => onLoad(page.page)}
          >
            刷新当前页
          </button>
        </p>
      ) : null}
    </section>
  );
}

export function AutomationRunsPage({
  role,
  transport = generatedAutomationRunsTransport,
  onUnauthenticated,
}: {
  readonly role: AutomationRunsRole;
  readonly transport?: AutomationRunsTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0);
  const verified = useRef<AutomationRunsPage>();
  const [state, setState] = useState<AutomationRunsState>({ kind: "loading" });

  const load = useCallback(
    async (page: number) => {
      const currentGeneration = ++generation.current;
      setState({ kind: "loading", previous: verified.current });
      const result = await loadAutomationRuns(transport, page);
      if (currentGeneration !== generation.current) return;
      if (result.status === "loaded") {
        verified.current = result.page;
        setState({ kind: "ready", page: result.page });
        return;
      }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setState({
        kind: "error",
        failure: result.status,
        previous: verified.current,
      });
    },
    [onUnauthenticated, transport],
  );

  useEffect(() => {
    if (canRead) void load(1);
    return () => {
      generation.current += 1;
    };
  }, [canRead, load]);

  if (!canRead)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">自动化运行记录</h1>
        <p>当前账号没有自动化运行记录访问权限。</p>
      </section>
    );

  return <AutomationRunsContent onLoad={(page) => void load(page)} state={state} />;
}
