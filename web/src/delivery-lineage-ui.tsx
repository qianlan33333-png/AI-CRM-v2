import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedDeliveryLineageTransport,
  loadDeliveryLineage,
  nextDeliveryLineagePage,
  previousDeliveryLineagePage,
  type DeliveryLineageFailure,
  type DeliveryLineagePageData,
  type DeliveryLineageRole,
  type DeliveryLineageTransport,
} from "./delivery-lineage";

const messages: Record<DeliveryLineageFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有投递处理谱系访问权限。",
  invalid: "投递处理谱系响应不符合已冻结的内部只读合同。",
  unavailable: "投递处理谱系暂时不可用，请稍后重试。",
};

export type DeliveryLineageState =
  | { readonly kind: "loading"; readonly previous?: DeliveryLineagePageData }
  | { readonly kind: "ready"; readonly page: DeliveryLineagePageData }
  | {
      readonly kind: "error";
      readonly failure: DeliveryLineageFailure;
      readonly previous?: DeliveryLineagePageData;
    };

function LineageRows({ page }: { readonly page: DeliveryLineagePageData }): React.ReactElement {
  return page.items.length === 0 ? (
    <p role="status">当前页没有内部处理记录。</p>
  ) : (
    <table>
      <thead>
        <tr>
          <th>谱系 ID</th>
          <th>记录类型</th>
          <th>内部状态</th>
          <th>尝试次数</th>
          <th>更新时间</th>
        </tr>
      </thead>
      <tbody>
        {page.items.map((item) => (
          <tr key={item.lineageID}>
            <td>{item.lineageID}</td>
            <td>{item.recordKind}</td>
            <td>{item.internalState}</td>
            <td>{item.attemptCount}</td>
            <td>{item.updatedAt}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function DeliveryLineageView({
  onLoad,
  state,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoad: (offset: number) => void;
  readonly state: DeliveryLineageState;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">自动化运营 · 内部只读</p>
      <h1 id="app-title">投递处理谱系</h1>
      <p>
        本页仅展示本地内部处理状态；不表示任何企业微信、支付或其他外部投递已经执行、送达或收到回执。
      </p>
      {page ? (
        <>
          <p>当前从第 {page.offset + 1} 条内部记录开始。</p>
          <LineageRows page={page} />
        </>
      ) : null}
      {state.kind === "loading" ? <p role="status">正在读取投递处理谱系。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      {state.kind === "error" && !page ? (
        <button type="button" onClick={() => onLoad(0)}>重试读取</button>
      ) : null}
      {page ? (
        <p>
          <button
            type="button"
            disabled={state.kind === "loading" || !previousDeliveryLineagePage(page)}
            onClick={() => {
              const previous = previousDeliveryLineagePage(page);
              if (previous !== undefined) onLoad(previous);
            }}
          >
            上一页
          </button>{" "}
          <button
            type="button"
            disabled={state.kind === "loading" || !nextDeliveryLineagePage(page)}
            onClick={() => {
              const next = nextDeliveryLineagePage(page);
              if (next !== undefined) onLoad(next);
            }}
          >
            下一页
          </button>
        </p>
      ) : null}
    </section>
  );
}

export function DeliveryLineagePage({
  role,
  transport = generatedDeliveryLineageTransport,
  onUnauthenticated,
}: {
  readonly role: DeliveryLineageRole;
  readonly transport?: DeliveryLineageTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0);
  const verified = useRef<DeliveryLineagePageData>();
  const [state, setState] = useState<DeliveryLineageState>({ kind: "loading" });

  const load = useCallback(
    async (offset: number) => {
      const currentGeneration = ++generation.current;
      setState({ kind: "loading", previous: verified.current });
      const result = await loadDeliveryLineage(transport, offset);
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
    if (canRead) void load(0);
    return () => {
      generation.current += 1;
    };
  }, [canRead, load]);

  if (!canRead) {
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">投递处理谱系</h1>
        <p>当前账号没有投递处理谱系访问权限。</p>
      </section>
    );
  }
  return <DeliveryLineageView onLoad={(offset) => void load(offset)} state={state} />;
}
