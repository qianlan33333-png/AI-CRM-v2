import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  archiveGroupInvite,
  createGroupInvite,
  generatedGroupInviteLibraryTransport,
  loadGroupInviteDetail,
  loadGroupInviteLibrary,
  nextGroupInvitePage,
  previousGroupInvitePage,
  updateGroupInvite,
  type GroupInviteLibraryDraft,
  type GroupInviteLibraryDetailResult,
  type GroupInviteLibraryFailure,
  type GroupInviteLibraryItem,
  type GroupInviteLibraryPageData,
  type GroupInviteLibraryRole,
  type GroupInviteLibraryTransport,
} from "./group-invite-library";

const messages: Record<GroupInviteLibraryFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有群邀请素材库访问权限。",
  not_found: "该群邀请素材已不存在或已归档。",
  conflict: "群邀请素材正在被其他操作处理，请刷新后再试。",
  invalid: "群邀请素材请求或回执不符合已冻结的本地合同。",
  unavailable: "群邀请素材库暂时不可用，请稍后重试。",
};

const emptyDraft: GroupInviteLibraryDraft = {
  name: "",
  title: "",
  description: "",
  joinURL: "",
  enabled: true,
};

export type GroupInviteLibraryState =
  | { readonly kind: "loading"; readonly previous?: GroupInviteLibraryPageData }
  | { readonly kind: "ready"; readonly page: GroupInviteLibraryPageData }
  | {
      readonly kind: "error";
      readonly failure: GroupInviteLibraryFailure;
      readonly previous?: GroupInviteLibraryPageData;
    };

export type GroupInviteLibraryDetailState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly itemID: number; readonly previous?: GroupInviteLibraryItem }
  | { readonly kind: "ready"; readonly item: GroupInviteLibraryItem }
  | {
      readonly kind: "error";
      readonly itemID: number;
      readonly failure: GroupInviteLibraryFailure;
      readonly previous?: GroupInviteLibraryItem;
    };

type Editor =
  | { readonly mode: "create"; readonly draft: GroupInviteLibraryDraft }
  | {
      readonly mode: "edit";
      readonly item: GroupInviteLibraryItem;
      readonly draft: GroupInviteLibraryDraft;
    };

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

function commandKey(operation: "create" | "update" | "archive"): string | undefined {
  const randomUUID = globalThis.crypto?.randomUUID;
  return typeof randomUUID === "function"
    ? `group-invite:${operation}:${randomUUID.call(globalThis.crypto)}`
    : undefined;
}

function draftFor(item: GroupInviteLibraryItem): GroupInviteLibraryDraft {
  return {
    name: item.name,
    title: item.title,
    description: item.description,
    joinURL: item.joinURL,
    enabled: item.enabled,
  };
}

function InviteRows({
  page,
  busy = false,
  onArchive,
  onDetail,
  onEdit,
}: {
  readonly page: GroupInviteLibraryPageData;
  readonly busy?: boolean;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onArchive?: (item: GroupInviteLibraryItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onDetail?: (item: GroupInviteLibraryItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onEdit?: (item: GroupInviteLibraryItem) => void;
}): React.ReactElement {
  const actions = onArchive !== undefined || onDetail !== undefined || onEdit !== undefined;
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
          {actions ? <th>本地操作</th> : null}
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
            {actions ? (
              <td>
                {onDetail ? (
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => onDetail(item)}
                  >
                    查看本地详情
                  </button>
                ) : null}{" "}
                {onEdit ? (
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => onEdit(item)}
                  >
                    编辑本地元数据
                  </button>
                ) : null}{" "}
                {onArchive ? (
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => onArchive(item)}
                  >
                    归档
                  </button>
                ) : null}
              </td>
            ) : null}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function GroupInviteLibraryView({
  busy = false,
  detailState = { kind: "idle" },
  onArchive,
  onDetail,
  onEdit,
  onLoad,
  state,
}: {
  readonly busy?: boolean;
  readonly detailState?: GroupInviteLibraryDetailState;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onArchive?: (item: GroupInviteLibraryItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onDetail?: (item: GroupInviteLibraryItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onEdit?: (item: GroupInviteLibraryItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onLoad: (offset: number) => void;
  readonly state: GroupInviteLibraryState;
}): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">素材库 · 本地元数据</p>
      <h1 id="app-title">群邀请素材库</h1>
      <p>
        本页只管理本地群邀请卡元数据；不代表企业微信入群方式已创建、可用、已发送或已发生其他外部效果。
      </p>
      {page ? (
        <>
          <p>共 {page.total} 条本地未归档素材，当前从第 {page.offset + 1} 条开始。</p>
          <InviteRows
            page={page}
            busy={busy}
            onDetail={onDetail}
            onEdit={onEdit}
            onArchive={onArchive}
          />
        </>
      ) : null}
      {state.kind === "loading" ? <p role="status">正在读取本地群邀请素材。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      {state.kind === "error" && !page ? (
        <button type="button" disabled={busy} onClick={() => onLoad(0)}>重试读取</button>
      ) : null}
      {page ? (
        <p>
          <button
            type="button"
            disabled={busy || !previousGroupInvitePage(page)}
            onClick={() => {
              const previous = previousGroupInvitePage(page);
              if (previous !== undefined) onLoad(previous);
            }}
          >
            上一页
          </button>{" "}
          <button
            type="button"
            disabled={busy || !nextGroupInvitePage(page)}
            onClick={() => {
              const next = nextGroupInvitePage(page);
              if (next !== undefined) onLoad(next);
            }}
          >
            下一页
          </button>
        </p>
      ) : null}
      <GroupInviteLibraryDetailView state={detailState} />
    </section>
  );
}

export function GroupInviteLibraryDetailView({
  state,
}: {
  readonly state: GroupInviteLibraryDetailState;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const item = state.kind === "ready" ? state.item : state.previous;
  return (
    <section aria-label="群邀请素材本地详情">
      <h2>本地素材详情</h2>
      {item ? (
        <dl>
          <dt>素材 ID</dt><dd>{item.id}</dd>
          <dt>名称</dt><dd>{item.name}</dd>
          <dt>标题</dt><dd>{item.title}</dd>
          <dt>说明</dt><dd>{item.description || "—"}</dd>
          <dt>封面素材 ID</dt><dd>{item.coverImageID ?? "—"}</dd>
          <dt>状态</dt><dd>{item.enabled ? "enabled" : "disabled"}</dd>
          <dt>创建时间</dt><dd>{item.createdAt}</dd>
          <dt>更新时间</dt><dd>{item.updatedAt}</dd>
        </dl>
      ) : null}
      {state.kind === "loading" ? <p role="status">正在读取本地素材详情。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
    </section>
  );
}

function EditorForm({
  busy,
  editor,
  onCancel,
  onChange,
  onSubmit,
}: {
  readonly busy: boolean;
  readonly editor: Editor;
  readonly onCancel: () => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onChange: (draft: GroupInviteLibraryDraft) => void;
  readonly onSubmit: () => void;
}): React.ReactElement {
  const { draft } = editor;
  const change = (field: keyof GroupInviteLibraryDraft, value: string | boolean) =>
    onChange({ ...draft, [field]: value });
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <h2>{editor.mode === "create" ? "新建本地群邀请卡" : "编辑本地群邀请卡"}</h2>
      <p>入群地址仅作为本地文本保存，不会被打开、复制或验证实际入群。</p>
      <label>
        名称（可选）
        <input value={draft.name} maxLength={128} disabled={busy} onChange={(event) => change("name", event.target.value)} />
      </label>
      <label>
        标题
        <input value={draft.title} maxLength={128} required disabled={busy} onChange={(event) => change("title", event.target.value)} />
      </label>
      <label>
        说明
        <textarea value={draft.description} maxLength={512} disabled={busy} onChange={(event) => change("description", event.target.value)} />
      </label>
      <label>
        入群地址（本地文本）
        <input value={draft.joinURL} maxLength={2048} required disabled={busy} onChange={(event) => change("joinURL", event.target.value)} />
      </label>
      <label>
        <input type="checkbox" checked={draft.enabled} disabled={busy} onChange={(event) => change("enabled", event.target.checked)} />
        启用本地卡片
      </label>
      <p>
        <button type="submit" disabled={busy}>{editor.mode === "create" ? "保存本地卡片" : "保存本地修改"}</button>{" "}
        {editor.mode === "edit" ? <button type="button" disabled={busy} onClick={onCancel}>取消编辑</button> : null}
      </p>
    </form>
  );
}

export function GroupInviteLibraryPage({
  role,
  transport = generatedGroupInviteLibraryTransport,
  readCookie = browserCookie,
  onUnauthenticated,
  confirmArchive = () =>
    typeof window !== "undefined" && window.confirm("确认归档这张本地群邀请卡吗？"),
}: {
  readonly role: GroupInviteLibraryRole;
  readonly transport?: GroupInviteLibraryTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly confirmArchive?: (item: GroupInviteLibraryItem) => boolean;
}): React.ReactElement {
  const canRead = role === "admin" || role === "ops";
  const generation = useRef(0);
  const inFlight = useRef(false);
  const verified = useRef<GroupInviteLibraryPageData>();
  const detailGeneration = useRef(0);
  const detailInFlight = useRef(false);
  const verifiedDetail = useRef<GroupInviteLibraryItem>();
  const [state, setState] = useState<GroupInviteLibraryState>({ kind: "loading" });
  const [detailState, setDetailState] = useState<GroupInviteLibraryDetailState>({ kind: "idle" });
  const [busy, setBusy] = useState(false);
  const [detailBusy, setDetailBusy] = useState(false);
  const [editor, setEditor] = useState<Editor>({ mode: "create", draft: emptyDraft });
  const [notice, setNotice] = useState<string>();

  const load = useCallback(
    async (offset: number) => {
      if (inFlight.current) return;
      inFlight.current = true;
      setBusy(true);
      const currentGeneration = ++generation.current;
      setState({ kind: "loading", previous: verified.current });
      try {
        const result = await loadGroupInviteLibrary(transport, offset);
        if (currentGeneration !== generation.current) return;
        if (result.status === "loaded") {
          verified.current = result.page;
          setState({ kind: "ready", page: result.page });
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setState({ kind: "error", failure: result.status, previous: verified.current });
      } finally {
        if (currentGeneration === generation.current) {
          inFlight.current = false;
          setBusy(false);
        }
      }
    },
    [onUnauthenticated, transport],
  );

  const csrf = (): string | undefined => {
    try {
      return readCSRFCookie(readCookie());
    } catch {
      return undefined;
    }
  };

  const loadDetail = useCallback(
    async (itemID: number) => {
      if (detailInFlight.current) return;
      detailInFlight.current = true;
      setDetailBusy(true);
      const currentGeneration = ++detailGeneration.current;
      setDetailState({ kind: "loading", itemID, previous: verifiedDetail.current });
      try {
        const result: GroupInviteLibraryDetailResult = await loadGroupInviteDetail(transport, itemID);
        if (currentGeneration !== detailGeneration.current) return;
        if (result.status === "loaded") {
          verifiedDetail.current = result.item;
          setDetailState({ kind: "ready", item: result.item });
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setDetailState({
          kind: "error",
          itemID,
          failure: result.status,
          previous: verifiedDetail.current,
        });
      } finally {
        if (currentGeneration === detailGeneration.current) {
          detailInFlight.current = false;
          setDetailBusy(false);
        }
      }
    },
    [onUnauthenticated, transport],
  );

  const refreshAfterMutation = useCallback(async () => {
    await load(0);
  }, [load]);

  const save = async () => {
    if (inFlight.current) return;
    const csrfToken = csrf();
    const key = commandKey(editor.mode === "create" ? "create" : "update");
    if (!csrfToken || !key) {
      setNotice("安全令牌或本地命令标识缺失，未发送保存请求。");
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    const result = editor.mode === "create"
      ? await createGroupInvite(transport, editor.draft, csrfToken, key)
      : await updateGroupInvite(transport, editor.item, editor.draft, csrfToken, key);
    inFlight.current = false;
    setBusy(false);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    if (result.status !== "saved") {
      setNotice(messages[result.status === "archived" ? "invalid" : result.status]);
      return;
    }
    setEditor({ mode: "create", draft: emptyDraft });
    setNotice("本地群邀请卡已保存，正在重新读取第一页。");
    await refreshAfterMutation();
  };

  const archive = async (item: GroupInviteLibraryItem) => {
    if (inFlight.current || !confirmArchive(item)) return;
    const csrfToken = csrf();
    const key = commandKey("archive");
    if (!csrfToken || !key) {
      setNotice("安全令牌或本地命令标识缺失，未发送归档请求。");
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    const result = await archiveGroupInvite(transport, item, csrfToken, key);
    inFlight.current = false;
    setBusy(false);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    if (result.status !== "archived") {
      setNotice(messages[result.status === "saved" ? "invalid" : result.status]);
      return;
    }
    if (editor.mode === "edit" && editor.item.id === item.id) {
      setEditor({ mode: "create", draft: emptyDraft });
    }
    setNotice("本地群邀请卡已归档，正在重新读取第一页。");
    await refreshAfterMutation();
  };

  useEffect(() => {
    if (canRead) void load(0);
    return () => {
      generation.current += 1;
      detailGeneration.current += 1;
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
  return (
    <>
      <GroupInviteLibraryView
        busy={busy || detailBusy}
        detailState={detailState}
        onLoad={(offset) => void load(offset)}
        onDetail={(item) => void loadDetail(item.id)}
        onEdit={(item) => {
          if (!inFlight.current) setEditor({ mode: "edit", item, draft: draftFor(item) });
        }}
        onArchive={(item) => void archive(item)}
        state={state}
      />
      <section className="route-card" aria-label="群邀请素材本地编辑">
        <EditorForm
          busy={busy}
          editor={editor}
          onCancel={() => setEditor({ mode: "create", draft: emptyDraft })}
          onChange={(draft) => setEditor((current) => ({ ...current, draft }))}
          onSubmit={() => void save()}
        />
        {notice ? <p role="status">{notice}</p> : null}
      </section>
    </>
  );
}
