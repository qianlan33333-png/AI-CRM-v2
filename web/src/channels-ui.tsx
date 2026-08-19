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
  loadChannelDetail,
  loadChannels,
  newChannelStatusIdempotencyKey,
  updateChannelStatus,
  type ChannelListItem,
  type ChannelDetail,
  type ChannelDetailResult,
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
export type ChannelDetailState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly item: ChannelListItem; readonly previous?: ChannelDetail }
  | { readonly kind: "ready"; readonly item: ChannelListItem; readonly detail: ChannelDetail }
  | { readonly kind: "error"; readonly item: ChannelListItem; readonly failure: ChannelsFailure; readonly previous?: ChannelDetail };

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
  const [detail, setDetail] = useState<ChannelDetailState>({ kind: "idle" });
  const loadGeneration = useRef(0);
  const updateInFlight = useRef(false);
  const detailGeneration = useRef(0);
  const detailInflight = useRef(new Set<number>());

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
  const onLoadDetail = useCallback((item: ChannelListItem) => {
    if (detailInflight.current.has(item.id)) return;
    detailInflight.current.add(item.id);
    const generation = ++detailGeneration.current;
    const previous = detail.kind !== "idle" && detail.item.id === item.id
      ? detail.kind === "ready" ? detail.detail : detail.previous
      : undefined;
    setDetail({ kind: "loading", item, previous });
    void loadChannelDetail(transport, item).then((result: ChannelDetailResult) => {
      if (generation !== detailGeneration.current) return;
      if (result.status === "loaded") { setDetail({ kind: "ready", item, detail: result.detail }); return; }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setDetail({ kind: "error", item, failure: result.status, previous });
    }).finally(() => detailInflight.current.delete(item.id));
  }, [detail, onUnauthenticated, transport]);

  return (
    <ChannelsView
      busy={busy}
      notice={notice}
      detail={detail}
      onLoadDetail={onLoadDetail}
      onStatusChange={onStatusChange}
      role={role}
      state={state}
    />
  );
}

export function ChannelsView({
  busy = false,
  detail = { kind: "idle" },
  notice,
  onLoadDetail = noopDetail,
  onStatusChange = noopStatusChange,
  role,
  state,
}: {
  readonly busy?: boolean;
  readonly detail?: ChannelDetailState;
  readonly notice?: string;
  readonly onLoadDetail?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    item: ChannelListItem,
  ) => void;
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
      <ChannelDetailPanel state={detail} />
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
              <th>本地配置</th>
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
                <td><button type="button" disabled={busy || (detail.kind === "loading" && detail.item.id === item.id)} onClick={() => onLoadDetail(item)}>查看本地配置</button></td>
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
function noopDetail(): void {}

function ChannelDetailPanel({ state }: { readonly state: ChannelDetailState }): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const value = state.kind === "ready" ? state.detail : state.previous;
  return <section data-testid="channel-detail">
    <h2>本地配置：{state.item.name}</h2>
    {value ? <dl>
      <dt>渠道类型</dt><dd>{value.channelType ?? "未配置"}</dd>
      <dt>载体类型</dt><dd>{value.carrierType ?? "未配置"}</dd>
      <dt>场景值</dt><dd>{value.sceneValue ?? "未配置"}</dd>
      <dt>欢迎语</dt><dd>{value.welcomeMessage ?? "未配置"}</dd>
      <dt>本地素材引用</dt><dd>图片 {value.imageMaterialCount}，小程序 {value.miniProgramMaterialCount}，附件 {value.attachmentMaterialCount}，群邀请 {value.groupInviteMaterialCount}</dd>
      <dt>分配配置</dt><dd>{value.hasAssignmentConfig ? "已配置" : "未配置"}</dd>
    </dl> : null}
    {state.kind === "loading" ? <p>正在读取本地配置。</p> : state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
  </section>;
}

function runtimeCookieHeader(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}
