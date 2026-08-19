import React, { useEffect, useMemo, useState } from "react";
import {
  filterWecomTagGroups,
  generatedWecomTagsTransport,
  loadWecomTagCatalog,
  nextWecomTagPage,
  previousWecomTagPage,
  wecomTagPage,
  wecomTagPageCount,
  wecomTagSearchState,
  type WecomTagCatalog,
  type WecomTagsRole,
  type WecomTagsTransport,
} from "./wecom-tags";

export interface WecomTagsPageProps {
  readonly role: WecomTagsRole;
  readonly transport?: WecomTagsTransport;
  readonly onUnauthenticated?: () => void;
}

export type WecomTagsViewState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly catalog: WecomTagCatalog }
  | { readonly kind: "error" };

export function WecomTagsPage({
  role,
  transport = generatedWecomTagsTransport,
  onUnauthenticated,
}: WecomTagsPageProps): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<WecomTagsViewState>({ kind: "loading" });

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

  return <WecomTagsView role={role} state={state} />;
}

export function WecomTagsView({
  role,
  state,
}: {
  readonly role: WecomTagsRole;
  readonly state: WecomTagsViewState;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [query, setQuery] = useState("");
  const groups = useMemo(
    () =>
      state.kind === "ready" ? filterWecomTagGroups(state.catalog, query) : [],
    [query, state],
  );
  const [selectedGroupID, setSelectedGroupID] = useState<number>();
  const [page, setPage] = useState(0);

  useEffect(() => {
    const next =
      state.kind === "ready"
        ? wecomTagSearchState(state.catalog, query)
        : { selectedGroupID: undefined, page: 0 as const };
    setSelectedGroupID(next.selectedGroupID);
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
                    <td>{tag.name}</td>
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
        </section>
      ) : null}
    </section>
  );
}
