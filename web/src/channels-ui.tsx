import React, { useEffect, useMemo, useState } from "react";
import {
  filterChannels,
  generatedChannelsTransport,
  loadChannels,
  type ChannelListItem,
  type ChannelListResult,
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

export function ChannelsPage({
  role,
  transport = generatedChannelsTransport,
  onUnauthenticated,
}: {
  readonly role: ChannelsRole;
  readonly transport?: ChannelsTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<ChannelsViewState>({ kind: "loading" });

  useEffect(() => {
    if (!canAccess) return undefined;
    let active = true;
    void loadChannels(transport).then((result: ChannelListResult) => {
      if (!active) return;
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

  return <ChannelsView role={role} state={state} />;
}

export function ChannelsView({
  role,
  state,
}: {
  readonly role: ChannelsRole;
  readonly state: ChannelsViewState;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState<ChannelStatusFilter>("all");
  const items = useMemo(
    () =>
      state.kind === "ready" ? filterChannels(state.items, keyword, status) : [],
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
      <p>
        <label>
          搜索渠道名称或编码
          <input
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
        <p>{state.items.length === 0 ? "当前没有本地渠道。" : "没有匹配的渠道。"}</p>
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
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
