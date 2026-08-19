import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  generatedGroupInviteLibraryTransport,
  loadGroupInviteLibrary,
  nextGroupInvitePage,
  previousGroupInvitePage,
  type GroupInviteLibraryFailure,
  type GroupInviteLibraryPageData,
  type GroupInviteLibraryRole,
  type GroupInviteLibraryTransport,
} from "./group-invite-library";

const messages: Record<GroupInviteLibraryFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有群邀请素材库访问权限。",
  invalid: "群邀请素材库响应不符合已冻结的本地只读合同。",
  unavailable: "群邀请素材库暂时不可用，请稍后重试。",
};

export type GroupInviteLibraryState =
  | { readonly kind: "loading"; readonly previous?: GroupInviteLibraryPageData }
  | { readonly kind: "ready"; readonly page: GroupInviteLibraryPageData }
  | {
      readonly kind: "error";
      readonly failure: GroupInviteLibraryFailure;
      readonly previous?: GroupInviteLibraryPageData;
    };

function InviteRows({ page }: { readonly page: GroupInviteLibraryPageData }): React.ReactElement {
  return page.items.length === 0 ? (
    <p role="status">当前页没有本地群邀请素材。</p>
  ) : (
    <table>
      <thead>
        <tr>
          <th>素材 ID</th>
          <th>名称</th>
          <th>标题</th>
          <th>说明</th>
          <th>封面素材 ID</th>
          <th>状态</th>
          <th>创建时间</th>
          <th>更新时间</th>
        </tr>
      </thead>
      <tbody>
        {page.items.map((item) => (
          <tr key={item.id}>
            <td>{item.id}</td>
            <td>{item.name}</td>
            <td>{item.title}</td>
            <td>{item.description || "—"}</td>
            <td>{item.coverImageID ?? "—"}</td>
            <td>{item.enabled ? "enabled" : "disabled"}</td>
            <td>{item.createdAt}</td>
            <td>{item.updatedAt}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function GroupInviteLibraryView({
  onLoad,
  state,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoad: (offset: number) => void;
  readonly state: GroupInviteLibraryState;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">素材库 · 本地只读</p>
      <h1 id="app-title">群邀请素材库</h1>
      <p>
        本页仅显示本地群邀请卡元数据；不代表企业微信入群方式已创建、可用、已发送或已发生其他外部效果。
      </p>
      {page ? (
        <>
          <p>共 {page.total} 条本地未归档素材，当前从第 {page.offset + 1} 条开始。</p>
          <InviteRows page={page} />
        </>
      ) : null}
      {state.kind === "loading" ? <p role="status">正在读取本地群邀请素材。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      {state.kind === "error" && !page ? (
        <button type="button" onClick={() => onLoad(0)}>重试读取</button>
      ) : null}
      {page ? (
        <p>
          <button
            type="button"
            disabled={state.kind === "loading" || !previousGroupInvitePage(page)}
            onClick={() => {
              const previous = previousGroupInvitePage(page);
              if (previous !== undefined) onLoad(previous);
            }}
          >
            上一页
          </button>{" "}
          <button
            type="button"
            disabled={state.kind === "loading" || !nextGroupInvitePage(page)}
            onClick={() => {
              const next = nextGroupInvitePage(page);
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

export function GroupInviteLibraryPage({
  role,
  transport = generatedGroupInviteLibraryTransport,
  onUnauthenticated,
}: {
  readonly role: GroupInviteLibraryRole;
  readonly transport?: GroupInviteLibraryTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin" || role === "ops";
  const generation = useRef(0);
  const verified = useRef<GroupInviteLibraryPageData>();
  const [state, setState] = useState<GroupInviteLibraryState>({ kind: "loading" });

  const load = useCallback(
    async (offset: number) => {
      const currentGeneration = ++generation.current;
      setState({ kind: "loading", previous: verified.current });
      const result = await loadGroupInviteLibrary(transport, offset);
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
        <h1 id="app-title">群邀请素材库</h1>
        <p>当前账号没有群邀请素材库访问权限。</p>
      </section>
    );
  }
  return <GroupInviteLibraryView onLoad={(offset) => void load(offset)} state={state} />;
}
