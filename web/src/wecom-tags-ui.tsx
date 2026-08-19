import React, { useEffect, useMemo, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  confirmsCreatedWecomTagGroup,
  confirmsRenamedWecomTag,
  createWecomTagGroup,
  filterWecomTagGroups,
  generatedWecomTagsTransport,
  loadWecomTagCatalog,
  newWecomTagIdempotencyKey,
  nextWecomTagPage,
  previousWecomTagPage,
  renameWecomTag,
  wecomTagPage,
  wecomTagPageCount,
  wecomTagSearchState,
  type WecomTagCatalog,
  type WecomTagGroupCreateResult,
  type WecomTagRenameResult,
  type WecomTagsRole,
  type WecomTagsTransport,
  type WecomTag,
} from "./wecom-tags";

export interface WecomTagsPageProps {
  readonly role: WecomTagsRole;
  readonly transport?: WecomTagsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}

export type WecomTagsViewState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly catalog: WecomTagCatalog }
  | { readonly kind: "error" };

export type WecomTagCopyStatus = "idle" | "copied" | "unavailable" | "failed";

type ClipboardWriter = Pick<Clipboard, "writeText">;

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

const createMessages: Record<
  Exclude<WecomTagGroupCreateResult["status"], "created"> | "csrf_missing",
  string
> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有本地标签目录创建权限。",
  invalid: "标签组和首个标签均须为 1–200 个字符。",
  unknown: "创建结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
  csrf_missing: "安全令牌缺失，未发送创建请求。",
};

// React does not commit disabled state between two synchronous submits. Keep
// the actual component's ref-backed single-flight rule testable.
export function startWecomTagGroupCreate(
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

export function startWecomTagMutation<T>(
  lock: { current: boolean },
  execute: () => Promise<T>,
): Promise<T> | undefined {
  if (lock.current) return undefined;
  lock.current = true;
  return (async () => {
    try {
      return await execute();
    } finally {
      lock.current = false;
    }
  })();
}

export async function copyWecomTagID(
  tagID: number,
  clipboard: ClipboardWriter | undefined = typeof navigator === "undefined"
    ? undefined
    : navigator.clipboard,
): Promise<Exclude<WecomTagCopyStatus, "idle">> {
  if (!clipboard || typeof clipboard.writeText !== "function") {
    return "unavailable";
  }
  try {
    await clipboard.writeText(String(tagID));
    return "copied";
  } catch {
    return "failed";
  }
}

export function WecomTagsPage({
  role,
  transport = generatedWecomTagsTransport,
  readCookie = browserCookie,
  onUnauthenticated,
}: WecomTagsPageProps): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<WecomTagsViewState>({ kind: "loading" });
  const [groupName, setGroupName] = useState("");
  const [firstTagName, setFirstTagName] = useState("");
  const [creating, setCreating] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [mutationUncertain, setMutationUncertain] = useState(false);
  const [createNotice, setCreateNotice] = useState<string>();
  const mutationInFlight = useRef(false);

  useEffect(() => {
    if (!canAccess) return undefined;
    let active = true;
    void loadWecomTagCatalog(transport).then((result) => {
      if (!active) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setState(
        result.status === "loaded"
          ? { kind: "ready", catalog: result.catalog }
          : { kind: "error" },
      );
    });
    return () => {
      active = false;
    };
  }, [canAccess, onUnauthenticated, transport]);

  const submitCreate = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canAccess || mutationUncertain) return;
    void startWecomTagGroupCreate(mutationInFlight, async () => {
      let csrfToken: string | undefined;
      try {
        csrfToken = readCSRFCookie(readCookie());
      } catch {
        csrfToken = undefined;
      }
      const idempotencyKey = newWecomTagIdempotencyKey();
      if (!csrfToken || !idempotencyKey) {
        setCreateNotice(createMessages.csrf_missing);
        return;
      }
      setCreating(true);
      setCreateNotice(undefined);
      try {
        const result = await createWecomTagGroup(
          transport,
          groupName,
          firstTagName,
          csrfToken,
          idempotencyKey,
        );
        if (result.status === "created") {
          setGroupName("");
          setFirstTagName("");
          setCreateNotice("本地标签组和首个标签已创建。");
          const refreshed = await loadWecomTagCatalog(transport);
          if (refreshed.status === "unauthenticated") onUnauthenticated?.();
          if (
            refreshed.status === "loaded" &&
            confirmsCreatedWecomTagGroup(refreshed.catalog, result)
          ) {
            setState({ kind: "ready", catalog: refreshed.catalog });
          } else {
            setMutationUncertain(true);
            setCreateNotice(
              "创建已确认，但目录刷新未确认；请人工刷新页面后核对目录。",
            );
          }
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        if (result.status === "unknown") setMutationUncertain(true);
        setCreateNotice(createMessages[result.status]);
      } finally {
        setCreating(false);
      }
    });
  };

  const createPanel = canAccess ? (
    <section aria-labelledby="wecom-tag-create-title">
      <h2 id="wecom-tag-create-title">创建本地标签组</h2>
      <p>仅创建本地标签目录记录，不会同步或操作企微联系人。</p>
      <form onSubmit={submitCreate}>
        <fieldset disabled={creating || renaming || mutationUncertain}>
          <label>
            标签组名称
            <input
              value={groupName}
              onChange={(event) => setGroupName(event.currentTarget.value)}
            />
          </label>
          <label>
            首个标签名称
            <input
              value={firstTagName}
              onChange={(event) => setFirstTagName(event.currentTarget.value)}
            />
          </label>
          <button type="submit">{creating ? "正在创建…" : "创建标签组"}</button>
        </fieldset>
      </form>
      {createNotice ? (
        <p aria-live="polite" role={mutationUncertain ? "alert" : "status"}>
          {createNotice}
        </p>
      ) : null}
    </section>
  ) : undefined;

  const onRenameTag = async (
    tag: WecomTag,
    tagName: string,
  ): Promise<WecomTagRenameResult | undefined> => {
    if (!canAccess || mutationUncertain) return undefined;
    return startWecomTagMutation(mutationInFlight, async () => {
      let csrfToken: string | undefined;
      try {
        csrfToken = readCSRFCookie(readCookie());
      } catch {
        csrfToken = undefined;
      }
      const idempotencyKey = newWecomTagIdempotencyKey();
      if (!csrfToken || !idempotencyKey) return { status: "invalid" };

      setRenaming(true);
      try {
        const result = await renameWecomTag(
          transport,
          tag,
          tagName,
          csrfToken,
          idempotencyKey,
        );
        if (result.status !== "confirmed") {
          if (result.status === "unauthenticated") onUnauthenticated?.();
          if (result.status === "unknown") setMutationUncertain(true);
          return result;
        }
        const refreshed = await loadWecomTagCatalog(transport);
        if (refreshed.status === "unauthenticated") onUnauthenticated?.();
        if (
          refreshed.status === "loaded" &&
          confirmsRenamedWecomTag(refreshed.catalog, result.tag)
        ) {
          setState({ kind: "ready", catalog: refreshed.catalog });
          return result;
        }
        setMutationUncertain(true);
        return { status: "unknown" };
      } finally {
        setRenaming(false);
      }
    });
  };

  return (
    <WecomTagsView
      createPanel={createPanel}
      mutationBusy={creating || renaming}
      mutationLocked={mutationUncertain}
      onRenameTag={onRenameTag}
      renaming={renaming}
      role={role}
      state={state}
    />
  );
}

export function WecomTagsView({
  role,
  state,
  createPanel,
  mutationBusy = false,
  mutationLocked = false,
  onRenameTag,
  renaming = false,
}: {
  readonly role: WecomTagsRole;
  readonly state: WecomTagsViewState;
  readonly createPanel?: React.ReactNode;
  readonly mutationBusy?: boolean;
  readonly mutationLocked?: boolean;
  readonly onRenameTag?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tag: WecomTag,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tagName: string,
  ) => Promise<WecomTagRenameResult | undefined>;
  readonly renaming?: boolean;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [query, setQuery] = useState("");
  const groups = useMemo(
    () =>
      state.kind === "ready" ? filterWecomTagGroups(state.catalog, query) : [],
    [query, state],
  );
  const [selectedGroupID, setSelectedGroupID] = useState<number>();
  const [selectedTagID, setSelectedTagID] = useState<number>();
  const [copyStatus, setCopyStatus] = useState<WecomTagCopyStatus>("idle");
  const [page, setPage] = useState(0);

  useEffect(() => {
    const next =
      state.kind === "ready"
        ? wecomTagSearchState(state.catalog, query)
        : { selectedGroupID: undefined, page: 0 as const };
    setSelectedGroupID(next.selectedGroupID);
    setSelectedTagID(undefined);
    setCopyStatus("idle");
    setPage(0);
  }, [state]);

  if (!canAccess)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">企微标签目录</h1>
        <p role="alert">当前账号没有企微标签目录访问权限。</p>
      </section>
    );
  if (state.kind === "loading")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">企微标签目录</h1>
        <p>正在读取企微标签目录。</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">企微标签目录</h1>
        <p role="alert">企微标签目录暂不可用。</p>
      </section>
    );

  const selected =
    groups.find((group) => group.id === selectedGroupID) ?? groups[0];
  const tags = selected?.tags ?? [];
  const currentPage = Math.min(page, wecomTagPageCount(tags) - 1);
  const visibleTags = wecomTagPage(tags, currentPage);
  const nextPage = nextWecomTagPage(currentPage, tags);
  const selectedTag = tags.find((tag) => tag.id === selectedTagID);

  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">只读目录</p>
      <h1 id="app-title">企微标签目录</h1>
      <dl>
        <dt>标签总数</dt>
        <dd>{state.catalog.totalTags}</dd>
        <dt>标签上限</dt>
        <dd>{state.catalog.tagLimit}</dd>
        <dt>本地目录快照时间（非企微同步）</dt>
        <dd>{state.catalog.snapshotAt}</dd>
      </dl>
      {createPanel}
      <label>
        搜索标签组、标签名称或标签 ID
        <input
          type="search"
          value={query}
          onChange={(event) => {
            const nextQuery = event.currentTarget.value;
            const next = wecomTagSearchState(state.catalog, nextQuery);
            setQuery(nextQuery);
            setSelectedGroupID(next.selectedGroupID);
            setSelectedTagID(undefined);
            setCopyStatus("idle");
            setPage(next.page);
          }}
        />
      </label>
      <section aria-label="标签组">
        <h2>标签组</h2>
        {groups.length === 0 ? (
          <p>没有匹配的标签组。</p>
        ) : (
          <ul>
            {groups.map((group) => (
              <li key={group.id}>
                <button
                  type="button"
                  aria-pressed={selected?.id === group.id}
                  onClick={() => {
                    setSelectedGroupID(group.id);
                    setSelectedTagID(undefined);
                    setCopyStatus("idle");
                    setPage(0);
                  }}
                >
                  {group.name}（{group.tags.length}）
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      {selected ? (
        <section aria-labelledby="wecom-tag-list-title">
          <h2 id="wecom-tag-list-title">{selected.name}</h2>
          {visibleTags.length === 0 ? (
            <p>当前筛选下没有标签。</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>标签 ID</th>
                  <th>标签名称</th>
                </tr>
              </thead>
              <tbody>
                {visibleTags.map((tag) => (
                  <tr key={tag.id}>
                    <td>{tag.id}</td>
                    <td>
                      <button
                        type="button"
                        aria-pressed={selectedTag?.id === tag.id}
                        onClick={() => {
                          setSelectedTagID(tag.id);
                          setCopyStatus("idle");
                        }}
                      >
                        {tag.name}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p>
            第 {currentPage + 1} 页，共 {wecomTagPageCount(tags)} 页
          </p>
          <button
            type="button"
            disabled={currentPage === 0}
            onClick={() => setPage(previousWecomTagPage(currentPage))}
          >
            上一页
          </button>
          <button
            type="button"
            disabled={nextPage === undefined}
            onClick={() => setPage(nextPage ?? currentPage)}
          >
            下一页
          </button>
          {selectedTag ? (
            <WecomTagDetails
              copyStatus={copyStatus}
              mutationBusy={mutationBusy}
              mutationLocked={mutationLocked}
              onCopy={() => {
                void copyWecomTagID(selectedTag.id).then(setCopyStatus);
              }}
              onRename={onRenameTag}
              renaming={renaming}
              tag={selectedTag}
            />
          ) : null}
        </section>
      ) : null}
    </section>
  );
}

export function WecomTagDetails({
  copyStatus,
  mutationBusy = false,
  mutationLocked = false,
  onCopy,
  onRename,
  renaming = false,
  tag,
}: {
  readonly copyStatus: WecomTagCopyStatus;
  readonly mutationBusy?: boolean;
  readonly mutationLocked?: boolean;
  readonly onCopy: () => void;
  readonly onRename?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tag: WecomTag,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tagName: string,
  ) => Promise<WecomTagRenameResult | undefined>;
  readonly renaming?: boolean;
  readonly tag: WecomTag;
}): React.ReactElement {
  const [tagName, setTagName] = useState(tag.name);
  const [renameNotice, setRenameNotice] = useState<string>();

  useEffect(() => {
    setTagName(tag.name);
    setRenameNotice(undefined);
  }, [tag.id, tag.name]);

  return (
    <section aria-labelledby="wecom-tag-detail-title">
      <h2 id="wecom-tag-detail-title">标签详情</h2>
      <dl>
        <dt>标签名称</dt>
        <dd>{tag.name}</dd>
        <dt>标签 ID</dt>
        <dd>{tag.id}</dd>
        <dt>标签组名称</dt>
        <dd>{tag.groupName}</dd>
      </dl>
      <button type="button" onClick={onCopy}>
        复制标签 ID
      </button>
      {onRename ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (mutationLocked || mutationBusy) return;
            void onRename(tag, tagName).then((result) => {
              if (!result) return;
              const notices: Record<WecomTagRenameResult["status"], string> = {
                confirmed: "本地标签名称已更新。",
                unauthenticated: "登录状态已失效，请重新登录。",
                forbidden: "当前账号没有本地标签目录改名权限。",
                invalid: "标签名称不符合已冻结的本地合同。",
                unknown:
                  "改名结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
              };
              setRenameNotice(notices[result.status]);
            });
          }}
        >
          <fieldset disabled={mutationLocked || mutationBusy}>
            <label>
              本地标签名称
              <input
                value={tagName}
                onChange={(event) => setTagName(event.currentTarget.value)}
              />
            </label>
            <button type="submit">
              {renaming ? "正在保存…" : "保存本地名称"}
            </button>
          </fieldset>
        </form>
      ) : null}
      {renameNotice ? (
        <p aria-live="polite" role={mutationLocked ? "alert" : "status"}>
          {renameNotice}
        </p>
      ) : null}
      {copyStatus === "copied" ? (
        <p aria-live="polite" role="status">
          标签 ID 已复制。
        </p>
      ) : null}
      {copyStatus === "unavailable" ? (
        <p aria-live="polite" role="status">
          当前浏览器不支持复制，请手工复制上方标签 ID。
        </p>
      ) : null}
      {copyStatus === "failed" ? (
        <p aria-live="polite" role="status">
          复制失败，请手工复制上方标签 ID。
        </p>
      ) : null}
    </section>
  );
}
