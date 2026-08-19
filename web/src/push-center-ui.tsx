import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedPushCenterTransport,
  loadPushCenterSections,
  type PushCenterFailure,
  type PushCenterRole,
  type PushCenterSnapshot,
  type PushCenterTransport,
} from "./push-center";

const messages: Record<PushCenterFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有推送中心本地总览访问权限。",
  invalid: "推送中心响应不符合已冻结的本地只读合同。",
  unavailable: "推送中心本地聚合暂不可用，请稍后手动刷新。",
};

export type PushCenterState =
  | { readonly kind: "loading"; readonly previous?: PushCenterSnapshot }
  | { readonly kind: "ready"; readonly snapshot: PushCenterSnapshot }
  | {
      readonly kind: "error";
      readonly failure: PushCenterFailure;
      readonly previous?: PushCenterSnapshot;
    };

export interface PushCenterReadController {
  readonly generation: { current: number };
  readonly inFlight: { current: boolean };
  readonly verified: { current: PushCenterSnapshot | undefined };
  // eslint-disable-next-line no-unused-vars -- named callback parameter documents the state sink.
  readonly onState: (state: PushCenterState) => void;
  readonly onUnauthenticated?: () => void;
}

export function invalidatePushCenterRead(
  controller: PushCenterReadController,
): void {
  controller.generation.current += 1;
  controller.inFlight.current = false;
}

export async function startPushCenterRead(
  controller: PushCenterReadController,
  transport: PushCenterTransport,
): Promise<void> {
  if (controller.inFlight.current) return;
  controller.inFlight.current = true;
  const currentGeneration = ++controller.generation.current;
  controller.onState({
    kind: "loading",
    previous: controller.verified.current,
  });
  try {
    const result = await loadPushCenterSections(transport);
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

export function PushCenterView({
  state,
  onLoad,
}: {
  readonly state: PushCenterState;
  readonly onLoad: () => void;
}): React.ReactElement {
  const snapshot = state.kind === "ready" ? state.snapshot : state.previous;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">推送中心 · 本地只读</p>
      <h1 id="app-title">推送中心</h1>
      <p>仅展示本地分区聚合计数；不触发 worker、Provider、外部发送或重试。</p>
      {snapshot ? (
        <section aria-labelledby="push-center-sections-title">
          <h2 id="push-center-sections-title">本地分区计数</h2>
          <table>
            <thead>
              <tr>
                <th>分区</th>
                <th>计数</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.sections.map((section) => (
                <tr key={section.key}>
                  <td>{section.label}</td>
                  <td>{section.count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      ) : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取本地推送中心聚合。</p>
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
          手动刷新本地聚合
        </button>
      </p>
    </section>
  );
}

export function PushCenterPage({
  role,
  transport = generatedPushCenterTransport,
  onUnauthenticated,
}: {
  readonly role: PushCenterRole;
  readonly transport?: PushCenterTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0);
  const inFlight = useRef(false);
  const verified = useRef<PushCenterSnapshot>();
  const [state, setState] = useState<PushCenterState>({ kind: "loading" });
  const load = useCallback(
    () =>
      startPushCenterRead(
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
    return () =>
      invalidatePushCenterRead({
        generation,
        inFlight,
        verified,
        onState: setState,
      });
  }, [canRead, load]);
  if (!canRead) {
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">推送中心</h1>
        <p>当前账号没有推送中心本地总览访问权限。</p>
      </section>
    );
  }
  return (
    <PushCenterView
      state={state}
      onLoad={() => {
        void load();
      }}
    />
  );
}
