import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedCustomerContextTransport,
  loadCustomerContext,
  loadCustomerContextTimelinePage,
  type CustomerContextLoadResult,
  type CustomerContextSnapshot,
  type CustomerContextTransport,
} from "./customer-context";

export interface CustomerContextPanelProps {
  readonly customerID: number;
  readonly transport?: CustomerContextTransport;
  readonly onUnauthenticated?: () => void;
  readonly initialSnapshot?: CustomerContextSnapshot;
}

type PanelState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly snapshot: CustomerContextSnapshot }
  | { readonly kind: "unauthenticated" }
  | { readonly kind: "forbidden" }
  | { readonly kind: "not_found" }
  | { readonly kind: "unavailable" };

function formatDateTime(value: string | undefined): string {
  if (!value) return "未记录";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? "未记录"
    : parsed.toLocaleString("zh-CN", { hour12: false });
}

function tagLabel(tag: CustomerContextSnapshot["tags"][number]): string {
  return tag.groupName ? `${tag.groupName} / ${tag.name}` : tag.name;
}

function localReadFailureMessage(
  status: Exclude<
    CustomerContextLoadResult,
    { readonly status: "loaded" }
  >["status"],
): string {
  switch (status) {
    case "unauthenticated":
      return "登录状态已失效，请重新登录。";
    case "forbidden":
      return "当前账号没有读取 Customer 360 本地投影的权限。";
    case "not_found":
      return "客户已不可见，请刷新确认。";
    default:
      return "Customer 360 本地投影暂时不可用，已显示的内容保持不变。";
  }
}

export function CustomerContextPanel({
  customerID,
  transport = generatedCustomerContextTransport,
  onUnauthenticated,
  initialSnapshot,
}: CustomerContextPanelProps): React.ReactElement {
  const [page, setPage] = useState<PanelState>(() =>
    initialSnapshot
      ? { kind: "ready", snapshot: initialSnapshot }
      : { kind: "loading" },
  );
  const [notice, setNotice] = useState<string>();
  const [timelinePending, setTimelinePending] = useState(false);
  const generation = useRef(0);
  const timelineToken = useRef<symbol>();
  const latestCustomerID = useRef(customerID);
  const unauthenticatedGeneration = useRef<number>();
  latestCustomerID.current = customerID;

  const notifyUnauthenticated = useCallback(
    (currentGeneration: number) => {
      if (unauthenticatedGeneration.current === currentGeneration) return;
      unauthenticatedGeneration.current = currentGeneration;
      onUnauthenticated?.();
    },
    [onUnauthenticated],
  );

  const refresh = useCallback(async () => {
    const currentGeneration = generation.current + 1;
    generation.current = currentGeneration;
    timelineToken.current = undefined;
    setTimelinePending(false);
    setNotice(undefined);
    const result = await loadCustomerContext(transport, customerID);
    if (
      generation.current !== currentGeneration ||
      latestCustomerID.current !== customerID
    )
      return;
    if (result.status === "loaded") {
      setPage({ kind: "ready", snapshot: result.snapshot });
      return;
    }
    if (result.status === "unauthenticated")
      notifyUnauthenticated(currentGeneration);
    setNotice(localReadFailureMessage(result.status));
    setPage((current) =>
      current.kind === "ready" ? current : { kind: result.status },
    );
  }, [customerID, notifyUnauthenticated, transport]);

  useEffect(() => {
    void refresh();
    return () => {
      generation.current += 1;
      timelineToken.current = undefined;
    };
  }, [refresh]);

  const loadMoreTimeline = async () => {
    if (
      page.kind !== "ready" ||
      !page.snapshot.timelineNextCursor ||
      timelineToken.current
    )
      return;
    const token = Symbol("customer-context-timeline");
    const currentGeneration = generation.current;
    const cursor = page.snapshot.timelineNextCursor;
    timelineToken.current = token;
    setTimelinePending(true);
    setNotice(undefined);
    try {
      const result = await loadCustomerContextTimelinePage(
        transport,
        customerID,
        cursor,
        new Set(page.snapshot.timeline.map((entry) => entry.id)),
      );
      if (
        generation.current !== currentGeneration ||
        timelineToken.current !== token ||
        latestCustomerID.current !== customerID
      ) {
        return;
      }
      if (result.status !== "loaded") {
        if (result.status === "unauthenticated")
          notifyUnauthenticated(currentGeneration);
        setNotice(localReadFailureMessage(result.status));
        return;
      }
      setPage((current) => {
        if (
          current.kind !== "ready" ||
          current.snapshot.customer.id !== customerID ||
          current.snapshot.timelineNextCursor !== cursor
        ) {
          return current;
        }
        return {
          kind: "ready",
          snapshot: {
            ...current.snapshot,
            timeline: [...current.snapshot.timeline, ...result.timeline],
            ...(result.nextCursor
              ? { timelineNextCursor: result.nextCursor }
              : { timelineNextCursor: undefined }),
          },
        };
      });
    } finally {
      if (timelineToken.current === token) {
        timelineToken.current = undefined;
        if (generation.current === currentGeneration) setTimelinePending(false);
      }
    }
  };

  if (page.kind === "loading") {
    return (
      <section
        className="customer-detail-page__card"
        aria-labelledby="customer-context-title"
      >
        <h2 id="customer-context-title">Customer 360 本地读取</h2>
        <p className="customer-detail-page__meta" role="status">
          正在读取安全本地投影…
        </p>
      </section>
    );
  }
  if (page.kind !== "ready") {
    return (
      <section
        className="customer-detail-page__card"
        aria-labelledby="customer-context-title"
      >
        <h2 id="customer-context-title">Customer 360 本地读取</h2>
        <p className="customer-detail-page__meta" role="alert">
          {localReadFailureMessage(page.kind)}
        </p>
        {page.kind === "unavailable" && (
          <button type="button" onClick={() => void refresh()}>
            重新读取
          </button>
        )}
      </section>
    );
  }

  const { customer, tags, timeline, timelineNextCursor, chat } = page.snapshot;
  return (
    <section
      className="customer-detail-page__card"
      aria-labelledby="customer-context-title"
    >
      <h2 id="customer-context-title">Customer 360 本地读取</h2>
      <p className="customer-detail-page__meta">
        仅本地读取；非原子快照；未执行外部调用。
      </p>
      {notice && (
        <p className="customer-detail-page__notice" role="alert">
          {notice}
        </p>
      )}
      <dl className="customer-detail-page__facts">
        <div>
          <dt>客户</dt>
          <dd>
            {customer.name} · OneID #{customer.id}
          </dd>
        </div>
        <div>
          <dt>阶段编号</dt>
          <dd>
            {customer.stageID === undefined ? "未设置" : `#${customer.stageID}`}
          </dd>
        </div>
        <div>
          <dt>负责人编号</dt>
          <dd>
            {customer.ownerStaffID === undefined
              ? "未设置"
              : `#${customer.ownerStaffID}`}
          </dd>
        </div>
        <div>
          <dt>渠道编号</dt>
          <dd>
            {customer.channelID === undefined
              ? "未设置"
              : `#${customer.channelID}`}
          </dd>
        </div>
        <div>
          <dt>加入时间</dt>
          <dd>{formatDateTime(customer.addedAt)}</dd>
        </div>
        <div>
          <dt>最近互动</dt>
          <dd>{formatDateTime(customer.lastInteractAt)}</dd>
        </div>
      </dl>
      <h3>CRM 标签</h3>
      {tags.length === 0 ? (
        <p className="customer-detail-page__meta" role="status">
          暂无本地 CRM 标签。
        </p>
      ) : (
        <ul className="customer-detail-page__tag-list">
          {tags.map((tag) => (
            <li key={tag.id}>{tagLabel(tag)}</li>
          ))}
        </ul>
      )}
      <h3>时间线</h3>
      {timeline.length === 0 ? (
        <p className="customer-detail-page__meta" role="status">
          暂无时间线记录。
        </p>
      ) : (
        <ol className="customer-detail-page__timeline">
          {timeline.map((entry) => (
            <li key={entry.id}>
              <strong>{entry.eventType}</strong>
              <time dateTime={entry.occurredAt}>
                {formatDateTime(entry.occurredAt)}
              </time>
            </li>
          ))}
        </ol>
      )}
      {timelineNextCursor && (
        <p className="customer-detail-page__meta">
          <button
            type="button"
            disabled={timelinePending}
            onClick={() => void loadMoreTimeline()}
          >
            {timelinePending ? "正在加载…" : "加载更多时间线"}
          </button>
        </p>
      )}
      <h3>本地聊天摘要</h3>
      {!chat.localArchiveAvailable ? (
        <p className="customer-detail-page__meta" role="status">
          本地聊天摘要暂不可用。
        </p>
      ) : chat.items.length === 0 ? (
        <p className="customer-detail-page__meta" role="status">
          暂无本地聊天摘要。
        </p>
      ) : (
        <ul className="customer-detail-page__timeline">
          {chat.items.map((entry, index) => (
            <li
              key={`${entry.sentAt}-${entry.chatType}-${entry.messageType}-${index}`}
            >
              <strong>
                {entry.chatType} / {entry.messageType}
              </strong>
              <time dateTime={entry.sentAt}>
                {formatDateTime(entry.sentAt)}
              </time>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
