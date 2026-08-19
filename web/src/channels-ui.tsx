import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { readCSRFCookie } from "./auth";
import {
  filterChannels,
  generatedChannelsTransport,
  loadChannels,
  newChannelStatusIdempotencyKey,
  updateChannelStatus,
  type ChannelListItem,
  type ChannelListResult,
  type ChannelStatus,
  type ChannelStatusUpdateResult,
  type ChannelsFailure,
  type ChannelsRole,
  type ChannelsTransport,
  type ChannelStatusFilter,
} from "./channels";

const messages: Record<ChannelsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有渠道列表访问权限。",
  unavailable: "本地渠道列表暂不可用。",
  invalid: "渠道列表响应不符合已冻结合同。",
};

export type ChannelsViewState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly items: readonly ChannelListItem[] }
  | { readonly kind: "error"; readonly failure: ChannelsFailure };

export interface ChannelStatusUpdateInput {
  readonly channelID: number;
  readonly status: ChannelStatus;
  readonly idempotencySource?: { readonly randomUUID: () => string };
  readonly readCookie: () => string;
  readonly transport: ChannelsTransport;
}

export async function performChannelStatusUpdate({
  channelID,
  status,
  idempotencySource,
  readCookie,
  transport,
}: ChannelStatusUpdateInput): Promise<ChannelStatusUpdateResult> {
  let csrfToken: string | undefined;
  try {
    csrfToken = readCSRFCookie(readCookie());
  } catch {
    csrfToken = undefined;
  }
  if (!csrfToken) return { status: "forbidden" };
  const idempotencyKey = newChannelStatusIdempotencyKey(idempotencySource);
  if (!idempotencyKey) return { status: "unknown" };
  return updateChannelStatus(
    transport,
    channelID,
    status,
    csrfToken,
    idempotencyKey,
  );
}

export function startChannelStatusUpdate(
  lock: { current: boolean },
  execute: () => Promise<void>,
): Promise<void> | undefined {
  if (lock.current) return undefined;
  lock.current = true;
  return (async () => {
    try {
      await execute();
    } finally {
      lock.current = false;
    }
  })();
}

export function ChannelsPage({
  role,
  transport = generatedChannelsTransport,
  readCookie = runtimeCookieHeader,
  onUnauthenticated,
}: {
  readonly role: ChannelsRole;
  readonly transport?: ChannelsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<ChannelsViewState>({ kind: "loading" });
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string>();
  const loadGeneration = useRef(0);
  const updateInFlight = useRef(false);

  useEffect(() => {
    if (!canAccess) return undefined;
    let active = true;
    const generation = ++loadGeneration.current;
    void loadChannels(transport).then((result: ChannelListResult) => {
      if (!active || generation !== loadGeneration.current) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setState(
        result.status === "loaded"
          ? { kind: "ready", items: result.items }
          : { kind: "error", failure: result.status },
      );
    });
    return () => {
      active = false;
    };
  }, [canAccess, onUnauthenticated, transport]);

  const onStatusChange = useCallback(
    async (item: ChannelListItem, status: ChannelStatus) => {
      const operation = startChannelStatusUpdate(updateInFlight, async () => {
        ++loadGeneration.current;
        setBusy(true);
        try {
          const result = await performChannelStatusUpdate({
            channelID: item.id,
            status,
            readCookie,
            transport,
          });
          if (result.status === "confirmed") {
            setState({ kind: "ready", items: result.items });
            setNotice("本地渠道状态已更新。");
          } else {
            if (result.status === "unauthenticated") onUnauthenticated?.();
            setNotice(
              result.status === "unknown"
                ? "更新结果未知，请刷新确认。"
                : messages[result.status],
            );
          }
        } finally {
          setBusy(false);
        }
      });
      if (operation) await operation;
    },
    [onUnauthenticated, readCookie, transport],
  );

  return (
    <ChannelsView
      busy={busy}
      notice={notice}
      onStatusChange={onStatusChange}
      role={role}
      state={state}
    />
  );
}

export function ChannelsView({
  busy = false,
  notice,
  onStatusChange = noopStatusChange,
  role,
  state,
}: {
  readonly busy?: boolean;
  readonly notice?: string;
  readonly onStatusChange?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
    item: ChannelListItem,
    // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
    status: ChannelStatus,
  ) => void;
  readonly role: ChannelsRole;
  readonly state: ChannelsViewState;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState<ChannelStatusFilter>("all");
  const items = useMemo(
    () =>
      state.kind === "ready"
        ? filterChannels(state.items, keyword, status)
        : [],
    [keyword, state, status],
  );

  if (!canAccess)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">渠道列表</h1>
        <p role="alert">当前账号没有渠道列表访问权限。</p>
      </section>
    );
  if (state.kind === "loading")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">渠道列表</h1>
        <p>正在读取本地渠道列表。</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">渠道列表</h1>
        <p role="alert">{messages[state.failure]}</p>
      </section>
    );

  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">只读本地渠道</p>
      <h1 id="app-title">渠道列表</h1>
      <p>状态更新仅写入本地渠道，不执行外部渠道能力。</p>
      {notice ? <p role="status">{notice}</p> : null}
      <p>
        <label>
          搜索渠道名称或编码
          <input
            disabled={busy}
            type="search"
            value={keyword}
            onChange={(event) => setKeyword(event.currentTarget.value)}
          />
        </label>
      </p>
      <p>
        <label>
          渠道状态
          <select
            disabled={busy}
            value={status}
            onChange={(event) =>
              setStatus(event.currentTarget.value as ChannelStatusFilter)
            }
          >
            <option value="all">全部</option>
            <option value="active">active</option>
            <option value="inactive">inactive</option>
            <option value="archived">archived</option>
          </select>
        </label>
      </p>
      {items.length === 0 ? (
        <p>
          {state.items.length === 0 ? "当前没有本地渠道。" : "没有匹配的渠道。"}
        </p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>渠道 ID</th>
              <th>名称</th>
              <th>编码</th>
              <th>状态</th>
              <th>本地分配人数</th>
              <th>本地进入人数</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>更新本地状态</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>{item.id}</td>
                <td>{item.name}</td>
                <td>{item.code}</td>
                <td>{item.status}</td>
                <td>{item.assigneeCount}</td>
                <td>{item.contactCount}</td>
                <td>{item.createdAt}</td>
                <td>{item.updatedAt}</td>
                <td>
                  <select
                    aria-label={`更新${item.name}的本地状态`}
                    disabled={busy}
                    onChange={(event) =>
                      onStatusChange(
                        item,
                        event.currentTarget.value as ChannelStatus,
                      )
                    }
                    value={item.status}
                  >
                    <option value="active">active</option>
                    <option value="inactive">inactive</option>
                    <option value="archived">archived</option>
                  </select>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function noopStatusChange(): void {}

function runtimeCookieHeader(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}
