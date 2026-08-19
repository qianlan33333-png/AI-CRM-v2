import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedAutomationRunsTransport,
  loadAutomationDiagnostics,
  loadAutomationRuns,
  loadAutomationSourceEvent,
  nextAutomationRunsPage,
  previousAutomationRunsPage,
  startAutomationSourceEventRead,
  type AutomationSourceEvent,
  type AutomationSourceEventFailure,
  type AutomationRunsFailure,
  type AutomationRunsPage,
  type AutomationRunsRole,
  type AutomationRunsTransport,
  type AutomationDiagnostics,
} from "./automation-runs";

const messages: Record<AutomationRunsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有自动化运行记录访问权限。",
  invalid: "自动化运行记录响应不符合脱敏只读契约。",
  unavailable: "自动化运行记录暂时不可用，请稍后重试。",
};

const sourceEventMessages: Record<AutomationSourceEventFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有读取源内部事件的权限。",
  not_found: "该运行关联的本地源内部事件已不存在。",
  invalid: "源内部事件响应不符合已冻结的本地只读契约。",
  unavailable: "源内部事件暂时不可用，请稍后重试。",
};

export type AutomationRunsState =
  | { readonly kind: "loading"; readonly previous?: AutomationRunsPage }
  | { readonly kind: "ready"; readonly page: AutomationRunsPage }
  | {
      readonly kind: "error";
      readonly failure: AutomationRunsFailure;
      readonly previous?: AutomationRunsPage;
    };

export type AutomationSourceEventState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly eventID: number;
      readonly previous?: AutomationSourceEvent;
    }
  | { readonly kind: "ready"; readonly sourceEvent: AutomationSourceEvent }
  | {
      readonly kind: "error";
      readonly eventID: number;
      readonly failure: AutomationSourceEventFailure;
      readonly previous?: AutomationSourceEvent;
    };

export type AutomationDiagnosticsState =
  | { readonly kind: "loading"; readonly previous?: AutomationDiagnostics }
  | { readonly kind: "ready"; readonly diagnostics: AutomationDiagnostics }
  | { readonly kind: "error"; readonly failure: AutomationRunsFailure; readonly previous?: AutomationDiagnostics };

export interface AutomationSourceEventReadController {
  readonly generation: { current: number };
  readonly inFlight: { current: boolean };
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onState: (state: AutomationSourceEventState) => void;
  readonly onUnauthenticated?: () => void;
  readonly state: { current: AutomationSourceEventState };
  readonly transport: AutomationRunsTransport;
}

export interface AutomationDiagnosticsReadController {
  readonly generation: { current: number };
  readonly inFlight: { current: symbol | undefined };
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onState: (state: AutomationDiagnosticsState) => void;
  readonly onUnauthenticated?: () => void;
  readonly state: { current: AutomationDiagnosticsState };
  readonly transport: AutomationRunsTransport;
}

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

export function RunRows({
  page,
  sourceEventBusy,
  onLoadSourceEvent,
}: {
  readonly page: AutomationRunsPage;
  readonly sourceEventBusy: boolean;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoadSourceEvent: (eventID: number) => void;
}): React.ReactElement {
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
              <th>源内部事件</th>
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
                <td>
                  <button
                    type="button"
                    disabled={sourceEventBusy}
                    onClick={() => onLoadSourceEvent(run.sourceEventID)}
                  >
                    查看源事件
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

export function AutomationSourceEventPanel({
  state,
}: {
  readonly state: AutomationSourceEventState;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const sourceEvent =
    state.kind === "ready" ? state.sourceEvent : state.previous;
  return (
    <section aria-live="polite" data-testid="automation-source-event">
      <h2>源内部事件</h2>
      <p>
        仅展示本地内部事件和处理观测；外部投递为 unknown，不能据此推断任何
        provider 已执行、送达或成功。
      </p>
      {sourceEvent ? (
        <>
          <dl>
            <dt>事件 ID</dt>
            <dd>{sourceEvent.eventID}</dd>
            <dt>事件类型</dt>
            <dd>{sourceEvent.eventType}</dd>
            <dt>发生时间</dt>
            <dd>{displayDate(sourceEvent.occurredAt)}</dd>
            <dt>已调度</dt>
            <dd>{sourceEvent.dispatched ? "是" : "否"}</dd>
            <dt>观测时间</dt>
            <dd>{displayDate(sourceEvent.observedAt)}</dd>
          </dl>
          {sourceEvent.deliveries.length === 0 ? (
            <p>当前没有本地 consumer 处理记录。</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>本地 consumer</th>
                  <th>内部状态</th>
                  <th>尝试次数</th>
                  <th>完成时间</th>
                </tr>
              </thead>
              <tbody>
                {sourceEvent.deliveries.map((delivery) => (
                  <tr key={delivery.consumer}>
                    <td>{delivery.consumer}</td>
                    <td>{delivery.status}</td>
                    <td>{delivery.attemptCount}</td>
                    <td>
                      {delivery.completedAt
                        ? displayDate(delivery.completedAt)
                        : "未完成"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      ) : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取源内部事件。</p>
      ) : null}
      {state.kind === "error" ? (
        <p role="alert">{sourceEventMessages[state.failure]}</p>
      ) : null}
    </section>
  );
}

export function AutomationDiagnosticsPanel({
  state,
}: {
  readonly state: AutomationDiagnosticsState;
}): React.ReactElement {
  const diagnostics = state.kind === "ready" ? state.diagnostics : state.previous;
  return (
    <section aria-live="polite" data-testid="automation-diagnostics">
      <h2>内部事件诊断摘要</h2>
      <p>
        仅统计本地事件与本地 consumer 的内部处理观测；“内部处理完成”不代表外部投递、送达或成功。
      </p>
      {diagnostics ? <>
        <dl>
          <dt>本地事件数</dt><dd>{diagnostics.eventCount}</dd>
          <dt>未调度事件数</dt><dd>{diagnostics.undispatchedEventCount}</dd>
          <dt>观测时间</dt><dd>{displayDate(diagnostics.observedAt)}</dd>
        </dl>
        <table>
          <thead><tr><th>内部状态</th><th>数量</th></tr></thead>
          <tbody>
            <tr><td>待处理</td><td>{diagnostics.deliveryCounts.pending}</td></tr>
            <tr><td>内部处理中</td><td>{diagnostics.deliveryCounts.processing}</td></tr>
            <tr><td>内部处理完成</td><td>{diagnostics.deliveryCounts.completed}</td></tr>
            <tr><td>内部处理最终失败</td><td>{diagnostics.deliveryCounts.final_failed}</td></tr>
            <tr><td>内部结果未知</td><td>{diagnostics.deliveryCounts.outcome_unknown}</td></tr>
          </tbody>
        </table>
        <p>本地 consumer：{diagnostics.consumerRegistry.map((binding) => `${binding.consumer} (${binding.eventType})`).join("；")}</p>
        <p>已观测本地域：{diagnostics.observedDomains.join("、")}；未观测域：{diagnostics.unobservedDomains.join("、")}。</p>
      </> : null}
      <p>外部投递状态为 unknown，且未执行真实外部调用。</p>
      {state.kind === "loading" ? <p role="status">正在读取内部事件诊断摘要。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
    </section>
  );
}

// The page calls this controller directly.  Keeping its mutable state in refs
// makes the same-tick gate and the unmount generation boundary executable in
// tests without adding a DOM test dependency.
export function loadAutomationSourceEventState(
  controller: AutomationSourceEventReadController,
  eventID: number,
): Promise<void> | undefined {
  return startAutomationSourceEventRead(controller.inFlight, async () => {
    const currentGeneration = ++controller.generation.current;
    const previous = retainedSourceEvent(controller.state.current, eventID);
    controller.onState({ kind: "loading", eventID, previous });
    const result = await loadAutomationSourceEvent(
      controller.transport,
      eventID,
    );
    if (currentGeneration !== controller.generation.current) return;
    if (result.status === "loaded") {
      controller.onState({ kind: "ready", sourceEvent: result.sourceEvent });
      return;
    }
    if (result.status === "unauthenticated") controller.onUnauthenticated?.();
    controller.onState({
      kind: "error",
      eventID,
      failure: result.status,
      previous,
    });
  });
}

export function loadAutomationDiagnosticsState(
  controller: AutomationDiagnosticsReadController,
): Promise<void> | undefined {
  if (controller.inFlight.current !== undefined) return undefined;
  const token = Symbol("automation-diagnostics-read");
  controller.inFlight.current = token;
  const currentGeneration = ++controller.generation.current;
  const previous = controller.state.current.kind === "ready"
    ? controller.state.current.diagnostics
    : controller.state.current.previous;
  controller.onState({ kind: "loading", previous });
  return loadAutomationDiagnostics(controller.transport)
    .then((result) => {
      if (currentGeneration !== controller.generation.current) return;
      if (result.status === "loaded") {
        controller.onState({ kind: "ready", diagnostics: result.diagnostics });
        return;
      }
      if (result.status === "unauthenticated") controller.onUnauthenticated?.();
      controller.onState({ kind: "error", failure: result.status, previous });
    })
    .finally(() => {
      if (controller.inFlight.current === token) controller.inFlight.current = undefined;
    });
}

function AutomationRunsContent({
  onLoad,
  onLoadSourceEvent,
  sourceEvent,
  diagnostics,
  state,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoad: (page: number) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoadSourceEvent: (eventID: number) => void;
  readonly sourceEvent: AutomationSourceEventState;
  readonly diagnostics: AutomationDiagnosticsState;
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
      <AutomationSourceEventPanel state={sourceEvent} />
      <AutomationDiagnosticsPanel state={diagnostics} />
      {page ? (
        <RunRows
          page={page}
          sourceEventBusy={sourceEvent.kind === "loading"}
          onLoadSourceEvent={onLoadSourceEvent}
        />
      ) : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取自动化运行记录。</p>
      ) : null}
      {state.kind === "error" ? (
        <p role="alert">{messages[state.failure]}</p>
      ) : null}
      {state.kind === "error" && !page ? (
        <button type="button" onClick={() => onLoad(1)}>
          重试读取
        </button>
      ) : null}
      {page ? (
        <p>
          <button
            type="button"
            disabled={
              state.kind === "loading" || !previousAutomationRunsPage(page)
            }
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
  const sourceEventGeneration = useRef(0);
  const sourceEventInFlight = useRef(false);
  const [state, setState] = useState<AutomationRunsState>({ kind: "loading" });
  const [sourceEvent, setSourceEvent] = useState<AutomationSourceEventState>({
    kind: "idle",
  });
  const sourceEventState = useRef<AutomationSourceEventState>({ kind: "idle" });
  const diagnosticsGeneration = useRef(0);
  const diagnosticsInFlight = useRef<symbol | undefined>(undefined);
  const diagnosticsState = useRef<AutomationDiagnosticsState>({ kind: "loading" });
  const [diagnostics, setDiagnostics] = useState<AutomationDiagnosticsState>({ kind: "loading" });

  const setSourceEventState = useCallback(
    (next: AutomationSourceEventState) => {
      sourceEventState.current = next;
      setSourceEvent(next);
    },
    [],
  );

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

  const setDiagnosticsState = useCallback((next: AutomationDiagnosticsState) => {
    diagnosticsState.current = next;
    setDiagnostics(next);
  }, []);

  const loadDiagnostics = useCallback(() => {
    const operation = loadAutomationDiagnosticsState({
      generation: diagnosticsGeneration,
      inFlight: diagnosticsInFlight,
      onState: setDiagnosticsState,
      onUnauthenticated,
      state: diagnosticsState,
      transport,
    });
    if (operation) void operation;
  }, [onUnauthenticated, setDiagnosticsState, transport]);

  useEffect(() => {
    if (canRead) {
      void load(1);
      loadDiagnostics();
    }
    return () => {
      generation.current += 1;
      sourceEventGeneration.current += 1;
      diagnosticsGeneration.current += 1;
      diagnosticsInFlight.current = undefined;
    };
  }, [canRead, load, loadDiagnostics]);

  const loadSourceEvent = useCallback(
    (eventID: number) => {
      const operation = loadAutomationSourceEventState(
        {
          generation: sourceEventGeneration,
          inFlight: sourceEventInFlight,
          onState: setSourceEventState,
          onUnauthenticated,
          state: sourceEventState,
          transport,
        },
        eventID,
      );
      if (operation) void operation;
    },
    [onUnauthenticated, setSourceEventState, transport],
  );

  if (!canRead)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">自动化运行记录</h1>
        <p>当前账号没有自动化运行记录访问权限。</p>
      </section>
    );

  return (
    <AutomationRunsContent
      onLoad={(page) => void load(page)}
      onLoadSourceEvent={loadSourceEvent}
      sourceEvent={sourceEvent}
      diagnostics={diagnostics}
      state={state}
    />
  );
}

function retainedSourceEvent(
  state: AutomationSourceEventState,
  eventID: number,
): AutomationSourceEvent | undefined {
  if (state.kind === "idle") return undefined;
  const sourceEvent =
    state.kind === "ready" ? state.sourceEvent : state.previous;
  return sourceEvent?.eventID === eventID ? sourceEvent : undefined;
}
