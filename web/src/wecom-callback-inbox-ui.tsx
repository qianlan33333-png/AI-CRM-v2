import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedCallbackInboxTransport,
  loadCallbackAudit,
  loadCallbackAuditDetail,
  nextCallbackAuditOffset,
  previousCallbackAuditOffset,
  type CallbackAuditItem,
  type CallbackAuditPage,
  type CallbackDisposition,
  type CallbackInboxFailure,
  type CallbackInboxTransport,
  type WeComCallbackRole,
} from "./wecom-callback-inbox";

const messages: Record<CallbackInboxFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有本地回调审计权限。",
  invalid: "本地回调审计响应不符合已冻结合同。",
  unavailable: "本地回调审计暂时不可用，请稍后刷新确认。",
};

export type CallbackAuditPageState =
  | { readonly kind: "loading"; readonly previous?: CallbackAuditPage }
  | { readonly kind: "ready"; readonly page: CallbackAuditPage }
  | {
      readonly kind: "error";
      readonly failure: CallbackInboxFailure;
      readonly previous?: CallbackAuditPage;
    };

export type CallbackAuditDetailState = {
  readonly loading: boolean;
  readonly item?: CallbackAuditItem;
  readonly failure?: CallbackInboxFailure;
};

export type CallbackAuditListFlight = {
  readonly key: string;
  readonly token: number;
};

export interface CallbackAuditListController {
  readonly transport: CallbackInboxTransport;
  readonly onUnauthenticated?: () => void;
  readonly token: { current: number };
  readonly flight: { current?: CallbackAuditListFlight };
  readonly verified: { current?: CallbackAuditPage };
  readonly unauthenticatedNotified: { current: boolean };
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setState: (state: CallbackAuditPageState) => void;
}

export function requestCallbackAudit(
  controller: CallbackAuditListController,
  disposition: CallbackDisposition,
  offset: number,
): Promise<void> | undefined {
  const key = `${disposition}:${offset}`;
  if (controller.flight.current?.key === key) return undefined;
  const token = ++controller.token.current;
  const owner = { key, token };
  controller.flight.current = owner;
  controller.setState({
    kind: "loading",
    previous: controller.verified.current,
  });
  return (async () => {
    try {
      const result = await loadCallbackAudit(
        controller.transport,
        disposition,
        offset,
      );
      if (token !== controller.token.current) return;
      if (result.status === "loaded") {
        controller.verified.current = result.page;
        controller.setState({ kind: "ready", page: result.page });
        return;
      }
      if (
        result.status === "unauthenticated" &&
        !controller.unauthenticatedNotified.current
      ) {
        controller.unauthenticatedNotified.current = true;
        controller.onUnauthenticated?.();
      }
      controller.setState({
        kind: "error",
        failure: result.status,
        previous: controller.verified.current,
      });
    } finally {
      if (controller.flight.current === owner)
        controller.flight.current = undefined;
    }
  })();
}

export function invalidateCallbackAuditList(
  token: { current: number },
  flight: { current?: CallbackAuditListFlight },
): void {
  const invalidatedToken = ++token.current;
  if ((flight.current?.token ?? invalidatedToken) < invalidatedToken)
    flight.current = undefined;
}

export interface CallbackAuditDetailController {
  readonly transport: CallbackInboxTransport;
  readonly onUnauthenticated?: () => void;
  readonly token: { current: number };
  readonly flight: { current?: string };
  readonly verified: { current?: CallbackAuditItem };
  readonly unauthenticatedNotified: { current: boolean };
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setState: (state: CallbackAuditDetailState) => void;
}

export function requestCallbackAuditDetail(
  controller: CallbackAuditDetailController,
  disposition: CallbackDisposition,
  eventID: number,
): Promise<void> | undefined {
  const key = `${disposition}:${eventID}`;
  if (controller.flight.current === key) return undefined;
  controller.flight.current = key;
  const token = ++controller.token.current;
  controller.setState({ loading: true, item: controller.verified.current });
  return (async () => {
    try {
      const result = await loadCallbackAuditDetail(
        controller.transport,
        disposition,
        eventID,
      );
      if (token !== controller.token.current) return;
      if (result.status === "loaded") {
        controller.verified.current = result.item;
        controller.setState({ loading: false, item: result.item });
        return;
      }
      if (
        result.status === "unauthenticated" &&
        !controller.unauthenticatedNotified.current
      ) {
        controller.unauthenticatedNotified.current = true;
        controller.onUnauthenticated?.();
      }
      controller.setState({
        loading: false,
        item: controller.verified.current,
        failure: result.status,
      });
    } finally {
      if (controller.flight.current === key)
        controller.flight.current = undefined;
    }
  })();
}

export function WeComCallbackInboxPage({
  role,
  transport = generatedCallbackInboxTransport,
  onUnauthenticated,
}: {
  readonly role: WeComCallbackRole;
  readonly transport?: CallbackInboxTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const listToken = useRef(0);
  const detailToken = useRef(0);
  const listFlight = useRef<CallbackAuditListFlight>();
  const detailFlight = useRef<string>();
  const verifiedPage = useRef<CallbackAuditPage>();
  const verifiedDetail = useRef<CallbackAuditItem>();
  const unauthenticatedNotified = useRef(false);
  const [disposition, setDisposition] =
    useState<CallbackDisposition>("accepted");
  const [state, setState] = useState<CallbackAuditPageState>({
    kind: "loading",
  });
  const [detail, setDetail] = useState<CallbackAuditDetailState>({
    loading: false,
  });

  const requestPage = useCallback(
    (nextDisposition: CallbackDisposition, offset: number) =>
      requestCallbackAudit(
        {
          transport,
          onUnauthenticated,
          token: listToken,
          flight: listFlight,
          verified: verifiedPage,
          unauthenticatedNotified,
          setState,
        },
        nextDisposition,
        offset,
      ),
    [onUnauthenticated, transport],
  );

  const requestDetail = useCallback(
    (eventID: number) =>
      requestCallbackAuditDetail(
        {
          transport,
          onUnauthenticated,
          token: detailToken,
          flight: detailFlight,
          verified: verifiedDetail,
          unauthenticatedNotified,
          setState: setDetail,
        },
        disposition,
        eventID,
      ),
    [disposition, onUnauthenticated, transport],
  );

  useEffect(() => {
    if (!canRead) return undefined;
    void requestPage(disposition, 0);
    return () => {
      invalidateCallbackAuditList(listToken, listFlight);
      detailToken.current += 1;
    };
  }, [canRead, disposition, requestPage]);

  useEffect(() => {
    detailToken.current += 1;
    detailFlight.current = undefined;
    verifiedDetail.current = undefined;
    setDetail({ loading: false });
  }, [disposition]);

  if (!canRead) {
    return (
      <section
        className="route-card"
        aria-labelledby="wecom-callback-audit-title"
      >
        <h2 id="wecom-callback-audit-title">企微回调本地审计</h2>
        <p>当前账号没有本地回调审计权限。</p>
      </section>
    );
  }

  const page = state.kind === "ready" ? state.page : state.previous;
  const loading = state.kind === "loading";
  return (
    <section
      className="route-card"
      aria-labelledby="wecom-callback-audit-title"
    >
      <p className="route-card__eyebrow">企微 · 本地接收审计</p>
      <h2 id="wecom-callback-audit-title">企微回调本地审计</h2>
      <p>
        仅展示验签后写入本地 event_log
        的去标识审计事实；不展示回调内容、身份标识或摘要，
        不代表任何外部投递、送达或业务处理成功。
      </p>
      <p>
        <button
          type="button"
          disabled={loading}
          onClick={() => setDisposition("accepted")}
        >
          已接受
        </button>{" "}
        <button
          type="button"
          disabled={loading}
          onClick={() => setDisposition("rejected")}
        >
          已拒绝
        </button>{" "}
        <button
          type="button"
          disabled={loading}
          onClick={() => void requestPage(disposition, 0)}
        >
          刷新
        </button>
      </p>
      {page ? (
        <>
          <table>
            <thead>
              <tr>
                <th>本地事件 ID</th>
                <th>本地审计结果</th>
                <th>发生时间</th>
                <th>内部派发标记</th>
                <th>详情</th>
              </tr>
            </thead>
            <tbody>
              {page.items.length === 0 ? (
                <tr>
                  <td colSpan={5}>当前没有本地回调审计记录。</td>
                </tr>
              ) : null}
              {page.items.map((item) => (
                <tr key={item.eventID}>
                  <td>{item.eventID}</td>
                  <td>
                    {item.disposition === "accepted" ? "已接受" : "已拒绝"}
                  </td>
                  <td>{item.occurredAt}</td>
                  <td>{item.dispatched ? "是" : "否"}</td>
                  <td>
                    <button
                      type="button"
                      disabled={detail.loading}
                      onClick={() => void requestDetail(item.eventID)}
                    >
                      读取安全详情
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p>
            <button
              type="button"
              disabled={
                loading || previousCallbackAuditOffset(page) === undefined
              }
              onClick={() => {
                const previous = previousCallbackAuditOffset(page);
                if (previous !== undefined)
                  void requestPage(disposition, previous);
              }}
            >
              上一页
            </button>{" "}
            <button
              type="button"
              disabled={loading || nextCallbackAuditOffset(page) === undefined}
              onClick={() => {
                const next = nextCallbackAuditOffset(page);
                if (next !== undefined) void requestPage(disposition, next);
              }}
            >
              下一页
            </button>
          </p>
        </>
      ) : null}
      {loading ? <p role="status">正在读取本地回调审计。</p> : null}
      {state.kind === "error" ? (
        <p role="alert">{messages[state.failure]}</p>
      ) : null}
      {detail.loading ? <p role="status">正在读取本地回调审计详情。</p> : null}
      {detail.failure ? <p role="alert">{messages[detail.failure]}</p> : null}
      {detail.item ? (
        <p>
          已核对本地事件 {detail.item.eventID}：
          {detail.item.disposition === "accepted" ? "已接受" : "已拒绝"}
          ，仅含本地审计字段。
        </p>
      ) : null}
    </section>
  );
}
