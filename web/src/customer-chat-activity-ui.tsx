import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
  CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
  canLoadNextCustomerChatActivityPage,
  loadCustomerChatActivity,
  type CustomerChatActivityFilter,
  type CustomerChatActivityPage,
  type CustomerChatActivityRole,
  type CustomerChatActivityTransport,
} from "./customer-chat-activity";

export interface CustomerChatActivityPanelProps {
  readonly customerID: number;
  readonly role: CustomerChatActivityRole;
  readonly transport: CustomerChatActivityTransport;
  readonly onUnauthenticated?: () => void;
}

type State =
  | { readonly kind: "loading"; readonly page?: CustomerChatActivityPage }
  | { readonly kind: "ready"; readonly page: CustomerChatActivityPage }
  | { readonly kind: "forbidden" | "not_found" | "unavailable" };

function failureMessage(kind: "forbidden" | "not_found" | "unavailable") {
  switch (kind) {
    case "forbidden":
      return "当前账号无权读取该客户的本地聊天活动。";
    case "not_found":
      return "客户已不可见，请刷新客户详情。";
    default:
      return "本地聊天活动暂不可用，已验证页面保持不变。";
  }
}

function label(filter: CustomerChatActivityFilter): string {
  switch (filter) {
    case "private":
      return "单聊";
    case "group":
      return "群聊";
    default:
      return "全部";
  }
}

export function CustomerChatActivityPanel({
  customerID,
  role,
  transport,
  onUnauthenticated,
}: CustomerChatActivityPanelProps): React.ReactElement | null {
  const [filter, setFilter] = useState<CustomerChatActivityFilter>("all");
  const [state, setState] = useState<State>({ kind: "loading" });
  const [notice, setNotice] = useState<string>();
  const generation = useRef(0);
  const token = useRef<symbol>();
  const verified = useRef<CustomerChatActivityPage>();
  const latestCustomerID = useRef(customerID);
  const unauthenticatedGeneration = useRef<number>();
  latestCustomerID.current = customerID;

  const load = useCallback(
    async (
      cursor?: string,
      limit = CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
      offset = 0,
    ) => {
      if (
        (role !== "admin" && role !== "ops" && role !== "sales") ||
        token.current
      )
        return;
      const owner = Symbol("customer-chat-activity");
      const currentGeneration = generation.current;
      token.current = owner;
      setNotice(undefined);
      setState({
        kind: "loading",
        ...(verified.current ? { page: verified.current } : {}),
      });
      try {
        const result = await loadCustomerChatActivity(
          transport,
          customerID,
          filter,
          cursor,
          limit,
          offset,
        );
        if (
          generation.current !== currentGeneration ||
          token.current !== owner ||
          latestCustomerID.current !== customerID
        ) {
          return;
        }
        if (result.status === "loaded") {
          verified.current = result.page;
          setState({ kind: "ready", page: result.page });
          return;
        }
        if (
          result.status === "unauthenticated" &&
          unauthenticatedGeneration.current !== currentGeneration
        ) {
          unauthenticatedGeneration.current = currentGeneration;
          onUnauthenticated?.();
        }
        const kind =
          result.status === "forbidden"
            ? "forbidden"
            : result.status === "not_found"
              ? "not_found"
              : "unavailable";
        setNotice(
          result.status === "unauthenticated"
            ? "登录状态已失效，请重新登录。"
            : failureMessage(kind),
        );
        setState(
          verified.current
            ? { kind: "ready", page: verified.current }
            : { kind },
        );
      } finally {
        if (token.current === owner) token.current = undefined;
      }
    },
    [customerID, filter, onUnauthenticated, role, transport],
  );

  useEffect(() => {
    generation.current += 1;
    token.current = undefined;
    verified.current = undefined;
    setState({ kind: "loading" });
    if (role === "admin" || role === "ops" || role === "sales") void load();
    return () => {
      generation.current += 1;
      token.current = undefined;
      verified.current = undefined;
    };
  }, [load, role]);

  if (role !== "admin" && role !== "ops" && role !== "sales") return null;

  const page =
    state.kind === "ready" || state.kind === "loading"
      ? state.page
      : undefined;
  const offset = page?.offset ?? 0;
  const visibleItems = page
    ? page.items.slice(
        0,
        Math.max(0, CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT - offset),
      )
    : [];
  const visibleEnd = offset + visibleItems.length;
  const canLoadPrevious = page?.previousCursor !== undefined && offset > 0;
  const canLoadNext =
    page !== undefined && canLoadNextCustomerChatActivityPage(page);
  const canExpand =
    page !== undefined &&
    page.limit < CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT &&
    page.total > visibleItems.length;

  return (
    <section
      className="customer-detail-page__card"
      aria-labelledby="customer-chat-activity-title"
    >
      <h2 id="customer-chat-activity-title">本地聊天活动</h2>
      <p className="customer-detail-page__meta">
        仅显示单聊/群聊类型、消息类型和时间；不读取正文、身份值、媒体、手机号、Provider
        回执或外部状态。默认读取最近 30 条摘要，最多展示前 100 条安全元数据。
      </p>
      <fieldset disabled={state.kind === "loading"}>
        <legend>活动类型</legend>
        {(["all", "private", "group"] as const).map((value) => (
          <label key={value}>
            <input
              type="radio"
              name="customer-chat-activity-filter"
              value={value}
              checked={filter === value}
              onChange={() => setFilter(value)}
            />
            {label(value)}
          </label>
        ))}
      </fieldset>
      {notice && <p role="alert">{notice}</p>}
      {!page && state.kind === "loading" ? (
        <p role="status">正在读取本地聊天活动…</p>
      ) : !page ? (
        <p role="status">
          {failureMessage(
            state.kind as "forbidden" | "not_found" | "unavailable",
          )}
        </p>
      ) : (
        <>
          <p className="customer-detail-page__meta">
            本地匹配记录 {page.total} 条 · 当前筛选 {label(page.chatType)}
            {visibleItems.length > 0
              ? ` · 当前显示第 ${offset + 1}–${visibleEnd} 条`
              : ""}
          </p>
          {visibleItems.length === 0 ? (
            <p role="status">当前筛选暂无本地聊天活动。</p>
          ) : (
            <ol className="customer-detail-page__timeline">
              {visibleItems.map((item, index) => (
                <li
                  key={`${item.sentAt}-${item.chatType}-${item.messageType}-${offset + index}`}
                >
                  <strong>
                    {label(item.chatType)} / {item.messageType}
                  </strong>
                  <time dateTime={item.sentAt}>
                    {new Date(item.sentAt).toLocaleString("zh-CN", {
                      hour12: false,
                    })}
                  </time>
                </li>
              ))}
            </ol>
          )}
          <p className="customer-detail-page__meta">
            <button
              type="button"
              disabled={state.kind === "loading" || !canLoadPrevious}
              onClick={() =>
                void load(
                  page.previousCursor,
                  page.limit,
                  Math.max(0, offset - page.limit),
                )
              }
            >
              上一页
            </button>
            <button
              type="button"
              disabled={state.kind === "loading" || !canLoadNext}
              onClick={() =>
                void load(page.nextCursor, page.limit, offset + page.limit)
              }
            >
              下一页
            </button>
            {canExpand && (
              <button
                type="button"
                disabled={state.kind === "loading"}
                onClick={() =>
                  void load(
                    undefined,
                    CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
                    0,
                  )
                }
              >
                展开至最多 100 条
              </button>
            )}
          </p>
        </>
      )}
    </section>
  );
}
