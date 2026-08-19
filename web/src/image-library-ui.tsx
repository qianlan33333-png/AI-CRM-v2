import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  firstPageQuery,
  formatFileSize,
  generatedImageLibraryTransport,
  loadImageDetail,
  loadFacets,
  loadImages,
  nextPageOffset,
  previousPageOffset,
  uploadFileProblem,
  uploadIdempotencyKey,
  uploadImage,
  uploadMetadataProblem,
  updateImageMetadata,
  type ImageItem,
  type ImageDetail,
  type ImageLibraryFailure,
  type ImageLibraryRole,
  type ImageLibraryTransport,
  type ImageListQuery,
  type ImageMetadataDraft,
  type ImageMetadataUpdateResult,
  type ImagePreviewMode,
  type ImageUploadMetadata,
  type ImageUploadResult,
  imagePreviewURL,
} from "./image-library";
import "./image-library.css";

export interface ImageLibraryPageProps {
  readonly role: ImageLibraryRole;
  readonly transport?: ImageLibraryTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}

type ListState =
  | { readonly kind: "loading" }
  | {
      readonly kind: "ready";
      readonly items: readonly ImageItem[];
      readonly total: number;
      readonly offset: number;
      readonly count: number;
      readonly hasMore: boolean;
      readonly nextOffset?: number;
    }
  | { readonly kind: "error"; readonly failure: ImageLibraryFailure };
type FacetsState =
  | { readonly kind: "loading" }
  | {
      readonly kind: "ready";
      readonly categories: readonly string[];
      readonly tags: readonly string[];
    }
  | { readonly kind: "error"; readonly failure: ImageLibraryFailure };
type DetailState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly imageID: number }
  | { readonly kind: "ready"; readonly image: ImageDetail }
  | {
      readonly kind: "error";
      readonly imageID: number;
      readonly failure: ImageLibraryFailure;
    };

const readMessages: Record<ImageLibraryFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有图片素材库访问权限。",
  conflict: "请求与当前状态冲突，请刷新后重试。",
  invalid: "请求未被服务端接受，请调整筛选后重试。",
  unavailable: "图片素材服务暂时不可用，请稍后重试。",
};

type UploadNoticeStatus = ImageLibraryFailure | "csrf_missing";
const uploadMessages: Record<UploadNoticeStatus, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有图片素材库上传权限。",
  conflict: "本次上传与已有操作冲突，请刷新后重试。",
  invalid: "图片或元数据不符合已冻结的上传规则。",
  unavailable: "上传结果未知，系统不会自动重试；请刷新列表确认后再操作。",
  csrf_missing: "安全令牌缺失，未发送上传请求。",
};
const metadataMessages: Record<UploadNoticeStatus, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有图片元数据保存权限。",
  conflict: "元数据保存与当前状态冲突，请刷新后重试。",
  invalid: "图片元数据不符合已冻结的保存规则。",
  unavailable: "保存结果未知，系统不会自动重试；请刷新列表确认后再操作。",
  csrf_missing: "安全令牌缺失，未发送保存请求。",
};

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

export interface SearchKeyEvent {
  readonly key: string;
  preventDefault(): void;
  stopPropagation(): void;
}

export function handleSearchKeyDown(
  event: SearchKeyEvent,
  search: () => void,
): void {
  if (event.key !== "Enter") return;
  event.preventDefault();
  event.stopPropagation();
  search();
}

export interface UploadThenReloadOptions {
  readonly transport: ImageLibraryTransport;
  readonly cookie: string;
  readonly file: Blob;
  readonly metadata: ImageUploadMetadata;
  readonly idempotencyKey: string;
  readonly reload: () => void;
}

export async function uploadThenReload(
  options: UploadThenReloadOptions,
): Promise<ImageUploadResult> {
  const result = await uploadImage(
    options.transport,
    options.cookie,
    options.file,
    options.metadata,
    options.idempotencyKey,
  );
  if (result.status === "uploaded") options.reload();
  return result;
}

export interface MetadataSaveThenReloadOptions {
  readonly transport: ImageLibraryTransport;
  readonly cookie: string;
  readonly imageID: number;
  readonly draft: ImageMetadataDraft;
  readonly reload: () => void;
}

export async function saveMetadataThenReload(
  options: MetadataSaveThenReloadOptions,
): Promise<ImageMetadataUpdateResult> {
  const result = await updateImageMetadata(
    options.transport,
    options.cookie,
    options.imageID,
    options.draft,
  );
  if (result.status === "saved") options.reload();
  return result;
}

// React does not commit disabled state between two synchronous click handlers.
// The ref prevents a second local PUT until the complete first flow settles.
export function startImageMetadataSave(
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

export function ImagePreviewPanel({
  image,
  mode,
  errorMode,
  onSelectMode,
  onPreviewError,
}: {
  readonly image: ImageDetail;
  readonly mode: ImagePreviewMode;
  readonly errorMode?: ImagePreviewMode;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onSelectMode: (mode: ImagePreviewMode) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onPreviewError: (mode: ImagePreviewMode) => void;
}): React.ReactElement {
  const selectedURL = imagePreviewURL(image, mode);
  const label = image.name !== "" ? image.name : image.fileName;
  return (
    <>
      <img
        key={`${image.id}:${mode}`}
        alt={label}
        src={selectedURL}
        onError={() => onPreviewError(mode)}
      />
      <div aria-label="本地预览模式">
        <button
          type="button"
          disabled={mode === "standard"}
          onClick={() => onSelectMode("standard")}
        >
          标准预览
        </button>
        <button
          type="button"
          disabled={mode === "original"}
          onClick={() => onSelectMode("original")}
        >
          查看原图
        </button>
      </div>
      {errorMode !== undefined ? (
        <p role="alert">
          {errorMode === "original" ? "原图" : "标准"}
          本地预览未能加载；已保留当前图片详情，系统不会自动重试。
        </p>
      ) : null}
    </>
  );
}

export function ImageLibraryPage({
  role,
  transport = generatedImageLibraryTransport,
  readCookie = browserCookie,
  onUnauthenticated,
}: ImageLibraryPageProps): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [query, setQuery] = useState<ImageListQuery>({
    search: "",
    category: "",
    tags: "",
    onlyUnlabeled: false,
    offset: 0,
  });
  const [searchInput, setSearchInput] = useState("");
  const [tagsInput, setTagsInput] = useState("");
  const [list, setList] = useState<ListState>({ kind: "loading" });
  const [facets, setFacets] = useState<FacetsState>({ kind: "loading" });
  const [uploadDraft, setUploadDraft] = useState<ImageUploadMetadata>({
    name: "",
    description: "",
    tags: "",
    category: "",
  });
  const [uploadFile, setUploadFile] = useState<File>();
  const [uploading, setUploading] = useState(false);
  const [metadataDraft, setMetadataDraft] = useState<ImageMetadataDraft>();
  const [savingMetadata, setSavingMetadata] = useState(false);
  const [notice, setNotice] = useState<{
    readonly kind: "status" | "alert";
    readonly text: string;
  }>();
  const listGeneration = useRef(0);
  const facetsGeneration = useRef(0);
  const detailGeneration = useRef(0);
  const metadataSaveInFlight = useRef(false);
  const [detail, setDetail] = useState<DetailState>({ kind: "idle" });
  const [previewMode, setPreviewMode] =
    useState<ImagePreviewMode>("standard");
  const [previewErrorMode, setPreviewErrorMode] =
    useState<ImagePreviewMode>();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadList = useCallback(
    async (next: ImageListQuery) => {
      const generation = ++listGeneration.current;
      setList({ kind: "loading" });
      const result = await loadImages(transport, next);
      if (generation !== listGeneration.current) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setList(
        result.status === "loaded"
          ? {
              kind: "ready",
              items: result.items,
              total: result.total,
              offset: result.offset,
              count: result.count,
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
  const loadFacetData = useCallback(async () => {
    const generation = ++facetsGeneration.current;
    setFacets({ kind: "loading" });
    const result = await loadFacets(transport);
    if (generation !== facetsGeneration.current) return;
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setFacets(
      result.status === "loaded"
        ? {
            kind: "ready",
            categories: result.facets.categories,
            tags: result.facets.tags,
          }
        : { kind: "error", failure: result.status },
    );
  }, [onUnauthenticated, transport]);

  useEffect(() => {
    if (canAccess) void loadList(query);
  }, [canAccess, loadList, query]);
  useEffect(() => {
    if (canAccess) void loadFacetData();
  }, [canAccess, loadFacetData]);

  const listBusy = list.kind === "loading";

  const submitSearch = () => {
    if (listBusy) return;
    setQuery({
      ...query,
      search: searchInput.trim(),
      tags: tagsInput,
      offset: 0,
    });
  };

  const reloadLibrary = () => {
    setQuery((current) => firstPageQuery(current));
    void loadFacetData();
  };

  const showDetail = (imageID: number) => {
    const generation = ++detailGeneration.current;
    setPreviewMode("standard");
    setPreviewErrorMode(undefined);
    setDetail({ kind: "loading", imageID });
    void loadImageDetail(transport, imageID).then((result) => {
      // A later selection (or close) wins over an older local read result.
      if (generation !== detailGeneration.current) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "loaded") {
        setMetadataDraft({
          name: result.image.name,
          description: result.image.description,
          tags: result.image.tags.join(","),
          category: result.image.category,
        });
      }
      setDetail(
        result.status === "loaded"
          ? { kind: "ready", image: result.image }
          : { kind: "error", imageID, failure: result.status },
      );
    });
  };

  const closeDetail = () => {
    detailGeneration.current += 1;
    setMetadataDraft(undefined);
    setPreviewMode("standard");
    setPreviewErrorMode(undefined);
    setDetail({ kind: "idle" });
  };

  const selectPreviewMode = (mode: ImagePreviewMode) => {
    setPreviewMode(mode);
    setPreviewErrorMode(undefined);
  };

  const submitMetadata = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canAccess || detail.kind !== "ready" || !metadataDraft) return;
    const imageID = detail.image.id;
    const operation = startImageMetadataSave(metadataSaveInFlight, async () => {
      setSavingMetadata(true);
      setNotice(undefined);
      let cookie = "";
      try {
        cookie = readCookie();
      } catch {
        cookie = "";
      }
      try {
        const result = await saveMetadataThenReload({
          transport,
          cookie,
          imageID,
          draft: metadataDraft,
          reload: reloadLibrary,
        });
        if (result.status === "saved") {
          setDetail({ kind: "ready", image: result.image });
          setMetadataDraft({
            name: result.image.name,
            description: result.image.description,
            tags: result.image.tags.join(","),
            category: result.image.category,
          });
          setNotice({
            kind: "status",
            text: `图片 #${result.image.id} 的本地元数据已保存；列表与筛选已按服务端事实刷新。`,
          });
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setNotice({ kind: "alert", text: metadataMessages[result.status] });
      } finally {
        setSavingMetadata(false);
      }
    });
    if (operation) await operation;
  };

  const submitUpload = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canAccess || uploading) return;
    if (!uploadFile) {
      setNotice({ kind: "alert", text: "请先选择要上传的图片文件。" });
      return;
    }
    const fileProblem = uploadFileProblem(uploadFile);
    if (fileProblem !== undefined) {
      setNotice({ kind: "alert", text: fileProblem });
      return;
    }
    const metadataProblem = uploadMetadataProblem(uploadDraft);
    if (metadataProblem !== undefined) {
      setNotice({ kind: "alert", text: metadataProblem });
      return;
    }
    let cookie = "";
    try {
      cookie = readCookie();
    } catch {
      cookie = "";
    }
    setUploading(true);
    setNotice(undefined);
    const result = await uploadThenReload({
      transport,
      cookie,
      file: uploadFile,
      metadata: uploadDraft,
      idempotencyKey: uploadIdempotencyKey(),
      reload: reloadLibrary,
    });
    setUploading(false);
    if (result.status === "uploaded") {
      if (fileInputRef.current) fileInputRef.current.value = "";
      setUploadFile(undefined);
      setNotice({
        kind: "status",
        text: `图片已上传为本地素材 #${result.image.id}；列表与筛选已按服务端事实刷新。`,
      });
      return;
    }
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setNotice({ kind: "alert", text: uploadMessages[result.status] });
  };

  if (!canAccess) {
    return (
      <section className="image-library" aria-labelledby="app-title">
        <p className="route-card__eyebrow">素材库</p>
        <h1 id="app-title">图片素材库</h1>
        <p className="image-library__state" role="alert">
          当前账号没有图片素材库访问权限。
        </p>
      </section>
    );
  }

  return (
    <section className="image-library" aria-labelledby="app-title">
      <div className="image-library__heading">
        <div>
          <p className="route-card__eyebrow">素材库</p>
          <h1 id="app-title">图片素材库</h1>
          <p>
            本页仅证明本地素材元数据和上传结果，不证明
            variant/对象访问或真实外发可用。
          </p>
        </div>
      </div>
      {notice && (
        <p
          className={`image-library__notice image-library__notice--${notice.kind}`}
          role={notice.kind === "alert" ? "alert" : "status"}
        >
          {notice.text}
        </p>
      )}
      <div className="image-library__grid">
        <section
          className="image-library__panel"
          aria-labelledby="image-list-title"
        >
          <h2 id="image-list-title">图片列表</h2>
          <form
            className="image-library__filters"
            onSubmit={(event) => {
              event.preventDefault();
              submitSearch();
            }}
          >
            <label>
              搜索图片
              <input
                value={searchInput}
                maxLength={200}
                placeholder="名称、文件名、描述、分类或标签"
                onChange={(event) => setSearchInput(event.currentTarget.value)}
                onKeyDown={(event) =>
                  handleSearchKeyDown(event, submitSearch)
                }
              />
            </label>
            <label>
              标签筛选
              <input
                value={tagsInput}
                maxLength={1000}
                placeholder="多个标签用英文逗号分隔"
                onChange={(event) => setTagsInput(event.currentTarget.value)}
                onKeyDown={(event) =>
                  handleSearchKeyDown(event, submitSearch)
                }
              />
            </label>
            <label>
              分类筛选
              <select
                value={query.category}
                disabled={listBusy || facets.kind !== "ready"}
                onChange={(event) =>
                  setQuery({
                    ...query,
                    category: event.currentTarget.value,
                    offset: 0,
                  })
                }
              >
                <option value="">全部分类</option>
                {facets.kind === "ready" &&
                  facets.categories.map((category) => (
                    <option key={category} value={category}>
                      {category}
                    </option>
                  ))}
              </select>
            </label>
            <label className="image-library__checkbox">
              <input
                type="checkbox"
                checked={query.onlyUnlabeled}
                disabled={listBusy}
                onChange={(event) =>
                  setQuery({
                    ...query,
                    onlyUnlabeled: event.currentTarget.checked,
                    offset: 0,
                  })
                }
              />
              仅看未标注
            </label>
            <button type="submit" disabled={listBusy}>
              搜索
            </button>
          </form>
          <div className="image-library__facets">
            {facets.kind === "loading" && (
              <p role="status">正在读取分类与标签…</p>
            )}
            {facets.kind === "error" && (
              <div role="alert">
                <p>
                  {readMessages[facets.failure]}
                  分类与标签筛选暂不可用；图片列表事实不受影响。
                </p>
                <button type="button" onClick={() => void loadFacetData()}>
                  重试
                </button>
              </div>
            )}
            {facets.kind === "ready" && (
              <p className="image-library__meta">
                可用分类：
                {facets.categories.length > 0
                  ? facets.categories.join("、")
                  : "暂无分类"}
                ；可用标签：
                {facets.tags.length > 0 ? facets.tags.join("、") : "暂无标签"}
              </p>
            )}
          </div>
          {list.kind === "loading" && <p role="status">正在读取图片素材…</p>}
          {list.kind === "error" && (
            <div role="alert">
              <p>{readMessages[list.failure]}</p>
              <button type="button" onClick={() => void loadList(query)}>
                重试
              </button>
            </div>
          )}
          {list.kind === "ready" && (
            <>
              {list.items.length === 0 && (
                <p role="status">暂无匹配的图片素材。</p>
              )}
              {list.items.length > 0 && (
                <ul className="image-grid">
                  {list.items.map((item) => (
                    <li className="image-card" key={item.id}>
                      <div className="image-card__preview" aria-hidden="true">
                        无预览
                      </div>
                      <dl className="image-card__meta">
                        <div>
                          <dt>名称</dt>
                          <dd>{item.name !== "" ? item.name : "（未命名）"}</dd>
                        </div>
                        <div>
                          <dt>文件名</dt>
                          <dd>{item.fileName}</dd>
                        </div>
                        <div>
                          <dt>分类</dt>
                          <dd>
                            {item.category !== "" ? item.category : "未分类"}
                          </dd>
                        </div>
                        <div>
                          <dt>标签</dt>
                          <dd>
                            {item.tags.length > 0
                              ? item.tags.join("、")
                              : "无标签"}
                          </dd>
                        </div>
                        <div>
                          <dt>尺寸</dt>
                          <dd>
                            {item.width}×{item.height}
                          </dd>
                        </div>
                        <div>
                          <dt>大小</dt>
                          <dd>{formatFileSize(item.fileSize)}</dd>
                        </div>
                        <div>
                          <dt>更新时间</dt>
                          <dd>{item.updatedAt}</dd>
                        </div>
                        <div>
                          <dt>状态</dt>
                          <dd>已启用</dd>
                        </div>
                        <div>
                          <dt>本地 ID</dt>
                          <dd>#{item.id}</dd>
                        </div>
                      </dl>
                      <button
                        type="button"
                        disabled={
                          detail.kind === "loading" &&
                          detail.imageID === item.id
                        }
                        onClick={() => showDetail(item.id)}
                      >
                        {detail.kind === "loading" &&
                        detail.imageID === item.id
                          ? "正在读取详情…"
                          : "查看详情"}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              {detail.kind !== "idle" && (
                <section
                  className="image-library__panel"
                  aria-live="polite"
                  aria-labelledby="image-detail-title"
                >
                  <div className="image-library__heading">
                    <h2 id="image-detail-title">图片详情</h2>
                    <button type="button" onClick={closeDetail}>
                      关闭详情
                    </button>
                  </div>
                  {detail.kind === "loading" && (
                    <p role="status">正在读取本地素材详情…</p>
                  )}
                  {detail.kind === "error" && (
                    <p role="alert">
                      {readMessages[detail.failure]}
                      详情未加载；当前列表保持不变。
                    </p>
                  )}
                  {detail.kind === "ready" && (
                    <>
                      <ImagePreviewPanel
                        image={detail.image}
                        mode={previewMode}
                        errorMode={previewErrorMode}
                        onSelectMode={selectPreviewMode}
                        onPreviewError={setPreviewErrorMode}
                      />
                      <dl className="image-card__meta">
                        <div>
                          <dt>名称</dt>
                          <dd>
                            {detail.image.name !== ""
                              ? detail.image.name
                              : "（未命名）"}
                          </dd>
                        </div>
                        <div>
                          <dt>文件名</dt>
                          <dd>{detail.image.fileName}</dd>
                        </div>
                        <div>
                          <dt>描述</dt>
                          <dd>
                            {detail.image.description !== ""
                              ? detail.image.description
                              : "无描述"}
                          </dd>
                        </div>
                        <div>
                          <dt>分类</dt>
                          <dd>
                            {detail.image.category !== ""
                              ? detail.image.category
                              : "未分类"}
                          </dd>
                        </div>
                        <div>
                          <dt>标签</dt>
                          <dd>
                            {detail.image.tags.length > 0
                              ? detail.image.tags.join("、")
                              : "无标签"}
                          </dd>
                        </div>
                        <div>
                          <dt>状态</dt>
                          <dd>{detail.image.enabled ? "已启用" : "已停用"}</dd>
                        </div>
                      </dl>
                      <p className="image-library__meta">
                        预览只读取已验证的本地变体，不证明公开访问、对象访问或真实外发可用。
                      </p>
                      {detail.image.enabled && metadataDraft ? (
                        <form onSubmit={submitMetadata}>
                          <h3>保存图片元数据</h3>
                          <fieldset disabled={savingMetadata}>
                            <label>
                              名称
                              <input
                                maxLength={200}
                                value={metadataDraft.name}
                                onChange={(event) =>
                                  setMetadataDraft({
                                    ...metadataDraft,
                                    name: event.currentTarget.value,
                                  })
                                }
                              />
                            </label>
                            <label>
                              描述
                              <textarea
                                maxLength={10000}
                                rows={3}
                                value={metadataDraft.description}
                                onChange={(event) =>
                                  setMetadataDraft({
                                    ...metadataDraft,
                                    description: event.currentTarget.value,
                                  })
                                }
                              />
                            </label>
                            <label>
                              标签（英文逗号分隔）
                              <input
                                maxLength={1000}
                                value={metadataDraft.tags}
                                onChange={(event) =>
                                  setMetadataDraft({
                                    ...metadataDraft,
                                    tags: event.currentTarget.value,
                                  })
                                }
                              />
                            </label>
                            <label>
                              分类
                              <input
                                maxLength={200}
                                value={metadataDraft.category}
                                onChange={(event) =>
                                  setMetadataDraft({
                                    ...metadataDraft,
                                    category: event.currentTarget.value,
                                  })
                                }
                              />
                            </label>
                            <p className="image-library__meta">
                              仅保存本地名称、描述、标签和分类；不会修改图片文件、启停状态或变体。
                            </p>
                            <button type="submit" disabled={savingMetadata}>
                              {savingMetadata ? "正在保存…" : "保存元数据"}
                            </button>
                          </fieldset>
                        </form>
                      ) : null}
                    </>
                  )}
                </section>
              )}
              <div className="image-pagination">
                <p role="status">
                  第 {list.count === 0 ? 0 : list.offset + 1}–
                  {list.offset + list.count} 条 / 共 {list.total} 条
                </p>
                <button
                  type="button"
                  disabled={listBusy || list.offset === 0}
                  onClick={() =>
                    setQuery({
                      ...query,
                      offset: previousPageOffset(list.offset),
                    })
                  }
                >
                  上一页
                </button>
                <button
                  type="button"
                  disabled={listBusy || !list.hasMore}
                  onClick={() => {
                    const next = nextPageOffset({
                      offset: list.offset,
                      count: list.count,
                      hasMore: list.hasMore,
                      ...(list.nextOffset !== undefined
                        ? { nextOffset: list.nextOffset }
                        : {}),
                    });
                    if (next !== undefined) {
                      setQuery({ ...query, offset: next });
                    }
                  }}
                >
                  下一页
                </button>
              </div>
            </>
          )}
        </section>
        <form
          className="image-library__panel image-upload"
          onSubmit={submitUpload}
        >
          <h2>上传图片</h2>
          <fieldset disabled={uploading}>
            <label>
              图片文件
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/gif"
                onChange={(event) =>
                  setUploadFile(event.currentTarget.files?.[0])
                }
              />
            </label>
            <label>
              名称（可选）
              <input
                maxLength={200}
                value={uploadDraft.name}
                onChange={(event) =>
                  setUploadDraft({
                    ...uploadDraft,
                    name: event.currentTarget.value,
                  })
                }
              />
            </label>
            <label>
              分类（可选）
              <input
                maxLength={200}
                value={uploadDraft.category}
                onChange={(event) =>
                  setUploadDraft({
                    ...uploadDraft,
                    category: event.currentTarget.value,
                  })
                }
              />
            </label>
            <label>
              标签（可选，英文逗号分隔）
              <input
                maxLength={1000}
                value={uploadDraft.tags}
                onChange={(event) =>
                  setUploadDraft({
                    ...uploadDraft,
                    tags: event.currentTarget.value,
                  })
                }
              />
            </label>
            <label>
              描述（可选）
              <textarea
                maxLength={10000}
                rows={3}
                value={uploadDraft.description}
                onChange={(event) =>
                  setUploadDraft({
                    ...uploadDraft,
                    description: event.currentTarget.value,
                  })
                }
              />
            </label>
            <p className="image-library__meta">
              仅支持 PNG、JPEG、GIF，单个文件不超过 10
              MiB；每次提交使用一次性幂等键，网络结果未知时不会自动重试。
            </p>
            <button type="submit" disabled={uploading || !uploadFile}>
              {uploading ? "正在上传…" : "上传图片"}
            </button>
          </fieldset>
        </form>
      </div>
    </section>
  );
}
