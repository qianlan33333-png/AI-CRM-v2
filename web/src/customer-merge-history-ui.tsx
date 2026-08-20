import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  loadCustomerMergeHistory,
  type CustomerMergeHistoryItem,
  type CustomerMergeHistoryRole,
  type CustomerMergeHistoryTransport,
} from "./customer-merge-history";

export interface CustomerMergeHistoryPanelProps {
  readonly customerID: number;
  readonly role: CustomerMergeHistoryRole;
  readonly transport: CustomerMergeHistoryTransport;
  readonly onUnauthenticated?: () => void;
}

type State =
  | {
      readonly kind: "loading";
      readonly items: readonly CustomerMergeHistoryItem[];
    }
  | {
      readonly kind: "ready";
      readonly items: readonly CustomerMergeHistoryItem[];
      readonly nextCursor?: string;
    }
  | { readonly kind: "forbidden" | "unavailable" };

function message(kind: "forbidden" | "unavailable"): string {
  return kind === "forbidden"
    ? "当前账号无权读取合并审计。"
    : "本地合并历史暂不可用。";
}

export function CustomerMergeHistoryPanel({
  customerID,
  role,
  transport,
  onUnauthenticated,
}: CustomerMergeHistoryPanelProps): React.ReactElement | null {
  const [state, setState] = useState<State>({ kind: "loading", items: [] });
  const [notice, setNotice] = useState<string>();
  const generation = useRef(0);
  const token = useRef<symbol>();
  const verified = useRef<{
    readonly items: readonly CustomerMergeHistoryItem[];
    readonly nextCursor?: string;
  }>({ items: [] });
  const latestCustomerID = useRef(customerID);
  const unauthenticatedGeneration = useRef<number>();
  latestCustomerID.current = customerID;

  const load = useCallback(
    async (cursor?: string) => {
      if ((role !== "admin" && role !== "ops") || token.current) return;
      const currentGeneration = generation.current;
      const owner = Symbol("customer-merge-history");
      token.current = owner;
      setNotice(undefined);
      setState({ kind: "loading", items: verified.current.items });
      try {
        const result = await loadCustomerMergeHistory(
          transport,
          customerID,
          cursor,
        );
        if (
          generation.current !== currentGeneration ||
          token.current !== owner ||
          latestCustomerID.current !== customerID
        )
          return;
        if (result.status === "unauthenticated") {
          if (unauthenticatedGeneration.current !== currentGeneration) {
            unauthenticatedGeneration.current = currentGeneration;
            onUnauthenticated?.();
          }
          setNotice("登录状态已失效，请重新登录。");
          setState(
            verified.current.items.length > 0
              ? { kind: "ready", ...verified.current }
              : { kind: "unavailable" },
          );
          return;
        }
        if (result.status !== "loaded") {
          const kind =
            result.status === "forbidden" ? "forbidden" : "unavailable";
          setNotice(message(kind));
          setState(
            verified.current.items.length > 0
              ? { kind: "ready", ...verified.current }
              : { kind },
          );
          return;
        }
        const previous = cursor ? verified.current.items : [];
        const ids = new Set(previous.map((item) => item.mergeAuditID));
        if (result.page.items.some((item) => ids.has(item.mergeAuditID))) {
          setNotice("合并历史分页发生漂移，已保留原记录。");
          setState(
            previous.length > 0
              ? { kind: "ready", ...verified.current }
              : { kind: "unavailable" },
          );
          return;
        }
        verified.current = {
          items: [...previous, ...result.page.items],
          ...(result.page.nextCursor
            ? { nextCursor: result.page.nextCursor }
            : {}),
        };
        setState({ kind: "ready", ...verified.current });
      } finally {
        if (token.current === owner) token.current = undefined;
      }
    },
    [customerID, onUnauthenticated, role, transport],
  );

  useEffect(() => {
    generation.current += 1;
    token.current = undefined;
    verified.current = { items: [] };
    setState({ kind: "loading", items: [] });
    if (role === "admin" || role === "ops") void load();
    return () => {
      generation.current += 1;
      token.current = undefined;
    };
  }, [customerID, load, role, transport]);

  if (role !== "admin" && role !== "ops") return null;
  const items =
    state.kind === "ready" || state.kind === "loading" ? state.items : [];
  return (
    <section
      className="customer-detail-page__card"
      aria-labelledby="customer-merge-history-title"
    >
      <h2 id="customer-merge-history-title">OneID 合并历史</h2>
      <p className="customer-detail-page__meta">
        仅本地追加审计；不含身份值、操作者、聊天正文或外部调用。
      </p>
      {notice && (
        <p className="customer-detail-page__notice" role="alert">
          {notice}
        </p>
      )}
      {state.kind === "loading" && items.length === 0 ? (
        <p role="status">正在读取本地合并审计…</p>
      ) : items.length === 0 ? (
        <p role="status">暂无本地合并记录。</p>
      ) : (
        <ol className="customer-detail-page__timeline">
          {items.map((item) => (
            <li key={item.mergeAuditID}>
              <strong>
                {item.mode === "auto" ? "自动合并" : "人工审核合并"}
              </strong>
              <span>
                OneID #{item.mergedCustomerID} → #{item.primaryCustomerID}
              </span>
              <span>
                策略 {item.policyVersion} · 审计 #{item.mergeAuditID}
              </span>
              <time dateTime={item.mergedAt}>
                {new Date(item.mergedAt).toLocaleString("zh-CN", {
                  hour12: false,
                })}
              </time>
            </li>
          ))}
        </ol>
      )}
      {state.kind === "ready" && state.nextCursor && (
        <button type="button" onClick={() => void load(state.nextCursor)}>
          加载更多合并记录
        </button>
      )}
    </section>
  );
}
