import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedExecutionRuntimeTransport,
  loadExecutionRuntime,
  type ExecutionRuntimeFailure,
  type ExecutionRuntimeRole,
  type ExecutionRuntimeSnapshot,
  type ExecutionRuntimeTransport,
} from "./execution-runtime";

const messages: Record<ExecutionRuntimeFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有执行运行时访问权限。",
  invalid: "执行运行时响应不符合已冻结的安全只读合同。",
  unavailable: "执行运行时本地观测暂时不可用，请稍后手动刷新。",
};

export type ExecutionRuntimeState =
  | { readonly kind: "loading"; readonly previous?: ExecutionRuntimeSnapshot }
  | { readonly kind: "ready"; readonly snapshot: ExecutionRuntimeSnapshot }
  | {
      readonly kind: "error";
      readonly failure: ExecutionRuntimeFailure;
      readonly previous?: ExecutionRuntimeSnapshot;
    };

export interface ExecutionRuntimeReadController {
  readonly generation: { current: number };
  readonly inFlight: { current: boolean };
  readonly verified: { current: ExecutionRuntimeSnapshot | undefined };
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onState: (state: ExecutionRuntimeState) => void;
  readonly onUnauthenticated?: () => void;
}

export function invalidateExecutionRuntimeRead(
  controller: ExecutionRuntimeReadController,
): void {
  controller.generation.current += 1;
  controller.inFlight.current = false;
}

export async function startExecutionRuntimeRead(
  controller: ExecutionRuntimeReadController,
  transport: ExecutionRuntimeTransport,
): Promise<void> {
  if (controller.inFlight.current) return;
  controller.inFlight.current = true;
  const currentGeneration = ++controller.generation.current;
  controller.onState({
    kind: "loading",
    previous: controller.verified.current,
  });
  try {
    const result = await loadExecutionRuntime(transport);
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
    if (currentGeneration === controller.generation.current) {
      controller.inFlight.current = false;
    }
  }
}

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

export function ExecutionRuntimeView({
  state,
  onLoad,
}: {
  readonly state: ExecutionRuntimeState;
  readonly onLoad: () => void;
}): React.ReactElement {
  const snapshot = state.kind === "ready" ? state.snapshot : state.previous;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">执行运行时 · 本地只读</p>
      <h1 id="app-title">执行运行时</h1>
      <p>只展示本地观察；不触发 worker、Provider 或外部调用。</p>
      {snapshot ? (
        <>
          <section aria-labelledby="execution-runtime-control-title">
            <h2 id="execution-runtime-control-title">本地控制面</h2>
            {snapshot.control ? (
              <dl>
                <dt>名称</dt>
                <dd>{snapshot.control.name}</dd>
                <dt>状态</dt>
                <dd>{snapshot.control.state}</dd>
                <dt>观测时间</dt>
                <dd>{displayDate(snapshot.control.observedAt)}</dd>
              </dl>
            ) : (
              <p>本地控制面当前未配置。</p>
            )}
          </section>
          <section aria-labelledby="execution-runtime-observations-title">
            <h2 id="execution-runtime-observations-title">本地队列观察</h2>
            <table>
              <thead>
                <tr>
                  <th>来源</th>
                  <th>队列</th>
                  <th>状态</th>
                  <th>尝试次数</th>
                  <th>观测时间</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.observations.map((observation, index) => (
                  <tr
                    key={`${observation.source}-${observation.queue}-${observation.observedAt}-${index}`}
                  >
                    <td>{observation.source}</td>
                    <td>{observation.queue}</td>
                    <td>{observation.status}</td>
                    <td>{observation.attempt}</td>
                    <td>{displayDate(observation.observedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
          {snapshot.truncated ? <p role="status">本地观测已截断。</p> : null}
        </>
      ) : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取本地执行运行时观测。</p>
      ) : null}
      {state.kind === "error" ? (
        <p role="alert">{messages[state.failure]}</p>
      ) : null}
      <p>
        <button
          type="button"
          disabled={state.kind === "loading"}
          onClick={onLoad}
        >
          手动刷新本地观测
        </button>
      </p>
    </section>
  );
}

export function ExecutionRuntimePage({
  role,
  transport = generatedExecutionRuntimeTransport,
  onUnauthenticated,
}: {
  readonly role: ExecutionRuntimeRole;
  readonly transport?: ExecutionRuntimeTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0);
  const inFlight = useRef(false);
  const verified = useRef<ExecutionRuntimeSnapshot>();
  const [state, setState] = useState<ExecutionRuntimeState>({
    kind: "loading",
  });

  const load = useCallback(
    () =>
      startExecutionRuntimeRead(
        {
          generation,
          inFlight,
          verified,
          onState: setState,
          onUnauthenticated,
        },
        transport,
      ),
    [onUnauthenticated, transport],
  );

  useEffect(() => {
    if (canRead) void load();
    return () => {
      invalidateExecutionRuntimeRead({
        generation,
        inFlight,
        verified,
        onState: setState,
      });
    };
  }, [canRead, load]);

  if (!canRead) {
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">执行运行时</h1>
        <p>当前账号没有执行运行时访问权限。</p>
      </section>
    );
  }
  return (
    <ExecutionRuntimeView
      state={state}
      onLoad={() => {
        void load();
      }}
    />
  );
}
