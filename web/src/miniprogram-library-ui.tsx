import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  deleteMiniProgram,
  draftProblem,
  editorDraft,
  generatedMiniProgramLibraryTransport,
  loadLibraryImages,
  loadMiniProgramDetail,
  loadMiniPrograms,
  resolveMiniProgramThumbnail,
  saveMiniProgram,
  setMiniProgramEnabled,
  uploadLibraryImage,
  imagePickerPreviousOffset,
  MINIPROGRAM_PAGE_SIZE,
  type LibraryImage,
  type MiniProgramDraft,
  type MiniProgramDetailResult,
  type MiniProgramFailure,
  type MiniProgramLibraryTransport,
  type MiniProgramListQuery,
  type MiniProgramRecord,
  type MiniProgramRole,
  type ThumbnailResolution,
} from "./miniprogram-library";
import "./miniprogram-library.css";

export interface MiniProgramLibraryPageProps {
  readonly role: MiniProgramRole;
  readonly transport?: MiniProgramLibraryTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}

type ListState =
  | { readonly kind: "loading" }
  | {
      readonly kind: "ready";
      readonly items: readonly MiniProgramRecord[];
      readonly total: number;
      readonly limit: number;
      readonly offset: number;
    }
  | { readonly kind: "error"; readonly failure: MiniProgramFailure };
type ImageListState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading" }
  | {
      readonly kind: "ready";
      readonly items: readonly LibraryImage[];
      readonly total: number;
      readonly hasMore: boolean;
      readonly nextOffset?: number;
    }
  | { readonly kind: "error"; readonly failure: MiniProgramFailure };
type DetailState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly itemID: number;
      readonly previous?: MiniProgramRecord;
    }
  | { readonly kind: "ready"; readonly item: MiniProgramRecord }
  | {
      readonly kind: "error";
      readonly itemID: number;
      readonly failure: MiniProgramFailure;
      readonly previous?: MiniProgramRecord;
    };

const messages: Record<MiniProgramFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有小程序素材操作权限。",
  not_found: "该素材已不存在，请刷新列表后重试。",
  conflict: "重复提交与已有操作冲突，请刷新后重试。",
  invalid: "提交内容不符合已冻结的小程序素材规则。",
  unavailable: "小程序素材服务暂时不可用，请稍后重试。",
};

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}
function mutationKey(operation: string): string {
  return `miniprogram-${operation}-${Date.now().toString(36)}-${Math.random()
    .toString(36)
    .slice(2, 14)}`;
}
function resolutionStatusLabel(status: ThumbnailResolution["status"]): string {
  if (status === "resolved") return "已解析（本地缓存命中）";
  if (status === "not_available") return "本地缓存不可用";
  return "结果未知（不会自动重试）";
}

export interface ImageSearchKeyEvent {
  readonly key: string;
  preventDefault(): void;
  stopPropagation(): void;
}

export function handleImageSearchKeyDown(
  event: ImageSearchKeyEvent,
  search: () => void,
): void {
  if (event.key !== "Enter") return;
  event.preventDefault();
  event.stopPropagation();
  search();
}

export function MiniProgramDetailPanel({
  state,
}: {
  readonly state: DetailState;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const item = state.kind === "ready" ? state.item : state.previous;
  return (
    <section className="miniprogram-detail" aria-label="小程序素材本地详情">
      <h3>本地素材详情</h3>
      {item ? (
        <dl>
          <dt>素材 ID</dt><dd>{item.id}</dd>
          <dt>名称</dt><dd>{item.name}</dd>
          <dt>AppID</dt><dd>{item.appID}</dd>
          <dt>页面路径</dt><dd>{item.pagePath}</dd>
          <dt>标题</dt><dd>{item.title}</dd>
          <dt>缩略图素材 ID</dt><dd>{item.thumbImageID ?? "—"}</dd>
          <dt>状态</dt><dd>{item.enabled ? "启用" : "停用"}</dd>
          <dt>版本</dt><dd>{item.version}</dd>
          <dt>创建时间</dt><dd>{item.createdAt}</dd>
          <dt>更新时间</dt><dd>{item.updatedAt}</dd>
        </dl>
      ) : null}
      {state.kind === "loading" ? <p role="status">正在读取本地素材详情。</p> : null}
      {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
    </section>
  );
}

export function MiniProgramLibraryPage({
  role,
  transport = generatedMiniProgramLibraryTransport,
  readCookie = browserCookie,
  onUnauthenticated,
}: MiniProgramLibraryPageProps): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [query, setQuery] = useState<MiniProgramListQuery>({
    search: "",
    enabledOnly: false,
    offset: 0,
  });
  const [searchInput, setSearchInput] = useState("");
  const [list, setList] = useState<ListState>({ kind: "loading" });
  const [selected, setSelected] = useState<MiniProgramRecord>();
  const [draft, setDraft] = useState<MiniProgramDraft>(editorDraft());
  const [notice, setNotice] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [toggling, setToggling] = useState(false);
  const [resolving, setResolving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [resolution, setResolution] = useState<{
    readonly itemID: number;
    readonly value: ThumbnailResolution;
  }>();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [imageSearchInput, setImageSearchInput] = useState("");
  const [imageQuery, setImageQuery] = useState({ search: "", offset: 0 });
  const [images, setImages] = useState<ImageListState>({ kind: "idle" });
  const [uploading, setUploading] = useState(false);
  const detailGeneration = useRef(0);
  const detailInFlight = useRef(false);
  const verifiedDetail = useRef<MiniProgramRecord>();
  const [detail, setDetail] = useState<DetailState>({ kind: "idle" });
  const [detailBusy, setDetailBusy] = useState(false);
  const busy = saving || toggling || resolving || deleting || uploading || detailBusy;

  const loadList = useCallback(
    async (next: MiniProgramListQuery) => {
      setList({ kind: "loading" });
      const result = await loadMiniPrograms(transport, next);
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setList(
        result.status === "loaded"
          ? {
              kind: "ready",
              items: result.items,
              total: result.total,
              limit: result.limit,
              offset: result.offset,
            }
          : { kind: "error", failure: result.status },
      );
    },
    [onUnauthenticated, transport],
  );
  const loadImages = useCallback(
    async (next: { search: string; offset: number }) => {
      setImages({ kind: "loading" });
      const result = await loadLibraryImages(transport, next);
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setImages(
        result.status === "loaded"
          ? {
              kind: "ready",
              items: result.items,
              total: result.total,
              hasMore: result.hasMore,
              ...(result.nextOffset !== undefined
                ? { nextOffset: result.nextOffset }
                : {}),
            }
          : { kind: "error", failure: result.status },
      );
    },
    [onUnauthenticated, transport],
  );
  useEffect(() => {
    if (canAccess) void loadList(query);
  }, [canAccess, loadList, query]);
  useEffect(() => {
    if (canAccess && pickerOpen) void loadImages(imageQuery);
  }, [canAccess, pickerOpen, imageQuery, loadImages]);
  useEffect(
    () => () => {
      detailGeneration.current += 1;
    },
    [],
  );

  const csrf = (): string | undefined => {
    try {
      return readCSRFCookie(readCookie());
    } catch {
      return undefined;
    }
  };
  const handleFailure = (failure: MiniProgramFailure) => {
    setNotice(messages[failure]);
    if (failure === "unauthenticated") onUnauthenticated?.();
    if (failure === "not_found") void loadList(query);
  };
  const clearDetail = () => {
    detailGeneration.current += 1;
    verifiedDetail.current = undefined;
    setDetail({ kind: "idle" });
  };
  const select = (item: MiniProgramRecord) => {
    clearDetail();
    setSelected(item);
    setDraft(editorDraft(item));
    setResolution(undefined);
    setConfirmingDelete(false);
    setNotice(undefined);
  };
  const startCreate = () => {
    clearDetail();
    setSelected(undefined);
    setDraft(editorDraft());
    setResolution(undefined);
    setConfirmingDelete(false);
    setNotice("已开始创建新的小程序素材。");
  };

  const loadDetail = async () => {
    if (!selected || !canAccess || detailInFlight.current) return;
    detailInFlight.current = true;
    setDetailBusy(true);
    const itemID = selected.id;
    const generation = ++detailGeneration.current;
    const previous = verifiedDetail.current?.id === itemID
      ? verifiedDetail.current
      : undefined;
    setDetail({ kind: "loading", itemID, previous });
    try {
      const result: MiniProgramDetailResult = await loadMiniProgramDetail(transport, itemID);
      if (generation !== detailGeneration.current) return;
      if (result.status === "loaded") {
        verifiedDetail.current = result.item;
        setDetail({ kind: "ready", item: result.item });
        return;
      }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setDetail({ kind: "error", itemID, failure: result.status, previous });
    } finally {
      if (generation === detailGeneration.current) {
        detailInFlight.current = false;
        setDetailBusy(false);
      }
    }
  };

  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canAccess || busy) return;
    const problem = draftProblem(draft);
    if (problem) {
      setNotice(problem);
      return;
    }
    const token = csrf();
    if (!token) {
      setNotice("安全令牌缺失，未发送保存请求。");
      return;
    }
    setSaving(true);
    setNotice(undefined);
    const result = await saveMiniProgram(
      transport,
      selected,
      draft,
      token,
      mutationKey(selected ? "update" : "create"),
    );
    setSaving(false);
    if (result.status !== "saved") {
      handleFailure(result.status);
      return;
    }
    clearDetail();
    setSelected(result.item);
    setDraft({
      ...editorDraft(result.item),
      ...(draft.thumbImageID === result.item.thumbImageID &&
      draft.thumbImageName !== undefined
        ? { thumbImageName: draft.thumbImageName }
        : {}),
    });
    setNotice(
      selected
        ? "素材已保存，列表已按服务端事实刷新。"
        : "素材已创建，列表已按服务端事实刷新。",
    );
    void loadList(query);
  };

  const toggleEnabled = async () => {
    if (!selected || !canAccess || busy) return;
    const token = csrf();
    if (!token) {
      setNotice("安全令牌缺失，未发送启停请求。");
      return;
    }
    const next = !selected.enabled;
    setToggling(true);
    setNotice(undefined);
    const result = await setMiniProgramEnabled(
      transport,
      selected.id,
      next,
      token,
      mutationKey("toggle"),
    );
    setToggling(false);
    if (result.status !== "saved") {
      handleFailure(result.status);
      return;
    }
    clearDetail();
    setSelected(result.item);
    setNotice(
      result.item.enabled
        ? "素材已启用；文案以服务端响应为准，列表已刷新。"
        : "素材已停用；文案以服务端响应为准，列表已刷新。",
    );
    void loadList(query);
  };

  const resolveThumb = async () => {
    if (!selected || !canAccess || busy) return;
    const token = csrf();
    if (!token) {
      setNotice("安全令牌缺失，未发送测试解析请求。");
      return;
    }
    setResolving(true);
    setNotice(undefined);
    setResolution(undefined);
    const result = await resolveMiniProgramThumbnail(
      transport,
      selected.id,
      token,
      mutationKey("test-resolve"),
    );
    setResolving(false);
    if (result.status !== "ok") {
      handleFailure(result.status);
      return;
    }
    clearDetail();
    setSelected(result.item);
    setResolution({ itemID: result.item.id, value: result.resolution });
    if (result.changed) void loadList(query);
  };

  const confirmDelete = async () => {
    if (!selected || !canAccess || busy) return;
    const token = csrf();
    if (!token) {
      setNotice("安全令牌缺失，未发送删除请求。");
      return;
    }
    setDeleting(true);
    setNotice(undefined);
    const result = await deleteMiniProgram(
      transport,
      selected.id,
      token,
      mutationKey("delete"),
    );
    setDeleting(false);
    if (result.status !== "deleted") {
      setConfirmingDelete(false);
      handleFailure(result.status);
      return;
    }
    clearDetail();
    setConfirmingDelete(false);
    setSelected(undefined);
    setDraft(editorDraft());
    setResolution(undefined);
    setNotice("素材已删除，列表已按服务端事实刷新；这不表示下游引用已清理。");
    const remaining = list.kind === "ready" ? list.items.length - 1 : 0;
    const nextOffset =
      remaining === 0 && query.offset > 0
        ? Math.max(0, query.offset - MINIPROGRAM_PAGE_SIZE)
        : query.offset;
    const nextQuery = { ...query, offset: nextOffset };
    setQuery(nextQuery);
    void loadList(nextQuery);
  };

  const chooseImage = (image: LibraryImage) => {
    setDraft({
      ...draft,
      thumbImageID: image.id,
      thumbImageName: image.name !== "" ? image.name : image.fileName,
    });
    setPickerOpen(false);
    setNotice(`已选择图片 #${image.id} 作为缩略图，保存后生效。`);
  };
  const clearThumb = () => {
    setDraft({
      name: draft.name,
      appID: draft.appID,
      pagePath: draft.pagePath,
      title: draft.title,
    });
    setNotice("已改为不绑定缩略图，保存后生效。");
  };
  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = "";
    if (!file || !canAccess || busy) return;
    const token = csrf();
    if (!token) {
      setNotice("安全令牌缺失，未发送图片上传请求。");
      return;
    }
    setUploading(true);
    setNotice(undefined);
    const result = await uploadLibraryImage(
      transport,
      file,
      file.name,
      token,
      mutationKey("image-upload"),
    );
    setUploading(false);
    if (result.status !== "uploaded") {
      handleFailure(result.status);
      return;
    }
    setDraft({
      ...draft,
      thumbImageID: result.image.id,
      thumbImageName: result.image.name,
    });
    setNotice(`缩略图已上传为本地图片 #${result.image.id}；保存素材后生效。`);
    if (pickerOpen) void loadImages({ ...imageQuery, offset: 0 });
  };

  if (!canAccess) {
    return (
      <section className="miniprogram-page" aria-labelledby="app-title">
        <p className="route-card__eyebrow">素材库</p>
        <h1 id="app-title">小程序素材库</h1>
        <p className="miniprogram-page__state" role="alert">
          当前账号没有小程序素材访问权限。
        </p>
      </section>
    );
  }

  return (
    <section className="miniprogram-page" aria-labelledby="app-title">
      <div className="miniprogram-page__heading">
        <div>
          <p className="route-card__eyebrow">素材库</p>
          <h1 id="app-title">小程序素材库</h1>
          <p>
            素材、缩略图与解析结果均以服务端当前事实为准；本地素材与本地缓存解析不代表真实企业微信可用、已上传或已发送。
          </p>
        </div>
        <button type="button" onClick={startCreate} disabled={busy}>
          新建素材
        </button>
      </div>
      {notice && (
        <p className="miniprogram-page__notice" role="alert">
          {notice}
        </p>
      )}
      <div className="miniprogram-page__grid">
        <section
          className="miniprogram-page__panel"
          aria-labelledby="miniprogram-list-title"
        >
          <h2 id="miniprogram-list-title">素材列表</h2>
          <form
            className="miniprogram-toolbar"
            onSubmit={(event) => {
              event.preventDefault();
              if (busy) return;
              setQuery({ ...query, search: searchInput.trim(), offset: 0 });
            }}
          >
            <label>
              搜索素材
              <input
                value={searchInput}
                maxLength={200}
                placeholder="名称、AppID、页面路径或标题"
                onChange={(event) => setSearchInput(event.currentTarget.value)}
              />
            </label>
            <label>
              启用状态
              <select
                value={query.enabledOnly ? "enabled" : "all"}
                onChange={(event) =>
                  setQuery({
                    ...query,
                    enabledOnly: event.currentTarget.value === "enabled",
                    offset: 0,
                  })
                }
              >
                <option value="all">全部素材</option>
                <option value="enabled">仅启用</option>
              </select>
            </label>
            <button type="submit" disabled={busy}>
              搜索
            </button>
          </form>
          {list.kind === "loading" && <p role="status">正在读取小程序素材…</p>}
          {list.kind === "error" && (
            <div role="alert">
              <p>{messages[list.failure]}</p>
              <button type="button" onClick={() => void loadList(query)}>
                重试
              </button>
            </div>
          )}
          {list.kind === "ready" && (
            <>
              <ul className="miniprogram-list">
                {list.items.map((item) => (
                  <li key={item.id}>
                    <button
                      aria-pressed={selected?.id === item.id}
                      type="button"
                      disabled={detailBusy}
                      onClick={() => select(item)}
                    >
                      <strong>
                        {item.name !== "" ? item.name : item.title}
                      </strong>
                      <span>
                        AppID {item.appID} · {item.enabled ? "启用" : "停用"}
                        {item.thumbImageID !== undefined
                          ? ` · 缩略图 #${item.thumbImageID}`
                          : " · 无缩略图"}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
              {list.items.length === 0 && (
                <p role="status">暂无小程序素材。请创建第一条素材。</p>
              )}
              <div className="miniprogram-pagination">
                <p role="status">
                  第 {list.items.length === 0 ? 0 : list.offset + 1}–
                  {list.offset + list.items.length} 条 / 共 {list.total} 条
                </p>
                <button
                  type="button"
                  disabled={busy || list.offset === 0}
                  onClick={() =>
                    setQuery({
                      ...query,
                      offset: Math.max(0, list.offset - list.limit),
                    })
                  }
                >
                  上一页
                </button>
                <button
                  type="button"
                  disabled={
                    busy || list.offset + list.items.length >= list.total
                  }
                  onClick={() =>
                    setQuery({ ...query, offset: list.offset + list.limit })
                  }
                >
                  下一页
                </button>
              </div>
            </>
          )}
        </section>
        <form
          className="miniprogram-page__panel miniprogram-editor"
          onSubmit={save}
        >
          <h2>{selected ? "编辑素材" : "新建素材"}</h2>
          <fieldset disabled={saving}>
            <label>
              名称
              <input
                maxLength={200}
                value={draft.name}
                onChange={(event) =>
                  setDraft({ ...draft, name: event.currentTarget.value })
                }
              />
            </label>
            <label>
              AppID
              <input
                maxLength={120}
                value={draft.appID}
                onChange={(event) =>
                  setDraft({ ...draft, appID: event.currentTarget.value })
                }
              />
            </label>
            <label>
              页面路径
              <input
                maxLength={500}
                value={draft.pagePath}
                onChange={(event) =>
                  setDraft({ ...draft, pagePath: event.currentTarget.value })
                }
              />
            </label>
            <label>
              标题
              <input
                maxLength={200}
                value={draft.title}
                onChange={(event) =>
                  setDraft({ ...draft, title: event.currentTarget.value })
                }
              />
            </label>
            <div className="miniprogram-thumb">
              <p className="miniprogram-page__meta">
                {draft.thumbImageID === undefined
                  ? "未绑定缩略图。"
                  : `已绑定缩略图图片 #${draft.thumbImageID}${
                      draft.thumbImageName !== undefined
                        ? `（${draft.thumbImageName}）`
                        : ""
                    }。缩略图只能来自服务端图片库返回的 ID。`}
              </p>
              <div className="miniprogram-thumb__actions">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => setPickerOpen(!pickerOpen)}
                >
                  {pickerOpen ? "收起图片选择" : "选择现有图片"}
                </button>
                <label className="miniprogram-upload">
                  上传新图片
                  <input
                    type="file"
                    accept="image/png,image/jpeg,image/gif"
                    disabled={busy || uploading}
                    onChange={upload}
                  />
                </label>
                {draft.thumbImageID !== undefined && (
                  <button type="button" disabled={busy} onClick={clearThumb}>
                    清除缩略图
                  </button>
                )}
              </div>
              {pickerOpen && (
                <div
                  className="miniprogram-picker"
                  aria-label="从图片库选择缩略图"
                >
                  <div className="miniprogram-toolbar" role="search">
                    <label>
                      搜索图片
                      <input
                        value={imageSearchInput}
                        maxLength={200}
                        placeholder="图片名称或文件名"
                        onChange={(event) =>
                          setImageSearchInput(event.currentTarget.value)
                        }
                        onKeyDown={(event) =>
                          handleImageSearchKeyDown(event, () => {
                            if (busy) return;
                            setImageQuery({
                              search: imageSearchInput.trim(),
                              offset: 0,
                            });
                          })
                        }
                      />
                    </label>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        setImageQuery({
                          search: imageSearchInput.trim(),
                          offset: 0,
                        })
                      }
                    >
                      搜索图片
                    </button>
                  </div>
                  {images.kind === "loading" && (
                    <p role="status">正在读取图片库…</p>
                  )}
                  {images.kind === "error" && (
                    <div role="alert">
                      <p>{messages[images.failure]}</p>
                      <button
                        type="button"
                        onClick={() => void loadImages(imageQuery)}
                      >
                        重试
                      </button>
                    </div>
                  )}
                  {images.kind === "ready" && (
                    <>
                      <ul className="miniprogram-picker__grid">
                        {images.items.map((image) => (
                          <li key={image.id}>
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() => chooseImage(image)}
                            >
                              <img
                                src={image.thumb160URL}
                                alt=""
                                width={80}
                                height={80}
                                loading="lazy"
                              />
                              <span>
                                #{image.id}{" "}
                                {image.name !== ""
                                  ? image.name
                                  : image.fileName}
                              </span>
                            </button>
                          </li>
                        ))}
                      </ul>
                      {images.items.length === 0 && (
                        <p role="status">图片库暂无匹配图片。</p>
                      )}
                      <div className="miniprogram-pagination">
                        <p role="status">共 {images.total} 张图片</p>
                        <button
                          type="button"
                          disabled={busy || imageQuery.offset === 0}
                          onClick={() =>
                            setImageQuery({
                              ...imageQuery,
                              offset: imagePickerPreviousOffset(
                                imageQuery.offset,
                              ),
                            })
                          }
                        >
                          上一页
                        </button>
                        <button
                          type="button"
                          disabled={busy || !images.hasMore}
                          onClick={() =>
                            setImageQuery({
                              ...imageQuery,
                              offset:
                                images.nextOffset ??
                                imageQuery.offset + images.items.length,
                            })
                          }
                        >
                          下一页
                        </button>
                      </div>
                    </>
                  )}
                </div>
              )}
            </div>
            <button type="submit" disabled={busy}>
              {selected ? "保存修改" : "创建素材"}
            </button>
          </fieldset>
          {selected && (
            <div className="miniprogram-actions">
              <h3>素材操作</h3>
              <div className="miniprogram-actions__row">
                <button type="button" disabled={busy} onClick={() => void loadDetail()}>
                  {detailBusy ? "正在刷新详情…" : "刷新本地详情"}
                </button>
                <button type="button" disabled={busy} onClick={toggleEnabled}>
                  {toggling
                    ? "正在提交启停…"
                    : selected.enabled
                      ? "停用素材"
                      : "启用素材"}
                </button>
                <button type="button" disabled={busy} onClick={resolveThumb}>
                  {resolving ? "正在解析…" : "测试解析缩略图"}
                </button>
                {!confirmingDelete && (
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => setConfirmingDelete(true)}
                  >
                    删除素材
                  </button>
                )}
              </div>
              {confirmingDelete && (
                <div className="miniprogram-confirm" role="alert">
                  <p>
                    确认删除素材「
                    {selected.name !== "" ? selected.name : selected.title}
                    」？删除不可撤销；成功仅代表服务端素材记录已删除，不表示下游引用已清理。
                  </p>
                  <div className="miniprogram-actions__row">
                    <button
                      type="button"
                      disabled={busy}
                      onClick={confirmDelete}
                    >
                      {deleting ? "正在删除…" : "确认删除"}
                    </button>
                    <button
                      type="button"
                      disabled={deleting}
                      onClick={() => setConfirmingDelete(false)}
                    >
                      取消
                    </button>
                  </div>
                </div>
              )}
              {resolution && resolution.itemID === selected.id && (
                <section
                  className="miniprogram-resolution"
                  aria-label="缩略图解析结果"
                >
                  <h3>缩略图解析结果</h3>
                  <dl>
                    <dt>解析状态</dt>
                    <dd>{resolutionStatusLabel(resolution.value.status)}</dd>
                    {resolution.value.thumbMediaID !== undefined && (
                      <>
                        <dt>thumb_media_id</dt>
                        <dd>{resolution.value.thumbMediaID}</dd>
                      </>
                    )}
                    <dt>缓存收据</dt>
                    <dd>{resolution.value.cacheReceipt}</dd>
                    <dt>缓存属主</dt>
                    <dd>{resolution.value.cacheOwner}</dd>
                  </dl>
                  <p className="miniprogram-page__meta">
                    以上仅为服务端本地缩略图缓存（media.thumbnail_cache）的解析结果：本次没有发起真实企业微信调用，不代表缩略图已上传、已发送或真实企业微信可用。
                  </p>
                  {resolution.value.status === "outcome_unknown" && (
                    <p className="miniprogram-page__meta">
                      结果未知：系统不会自动重试；请确认本地缓存事实后手动重试。
                    </p>
                  )}
                </section>
              )}
              <MiniProgramDetailPanel state={detail} />
            </div>
          )}
        </form>
      </div>
    </section>
  );
}
