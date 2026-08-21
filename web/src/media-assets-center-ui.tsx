import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { readCSRFCookie } from "./auth";
import {
  deleteImage,
  formatFileSize,
  loadImageDetail,
  setImageEnabled,
  updateImageMetadata,
  uploadFileProblem,
  uploadIdempotencyKey,
  uploadImage,
  uploadMetadataProblem,
  type ImageDeleteReferenceCounts,
  type ImageDetail,
  type ImageItem,
  type ImageLibraryFailure,
  type ImageMetadataDraft,
  type ImagePreviewMode,
  type ImageUploadMetadata,
} from "./image-library";
import { ImagePreviewPanel } from "./image-library-ui";
import {
  deleteMiniProgram,
  draftProblem,
  editorDraft,
  loadLibraryImages,
  loadMiniProgramDetail,
  saveMiniProgram,
  setMiniProgramEnabled,
  imagePickerPreviousOffset,
  type LibraryImage,
  type LibraryImageListResult,
  type MiniProgramDraft,
  type MiniProgramFailure,
  type MiniProgramRecord,
} from "./miniprogram-library";
import {
  archiveGroupInvite,
  createGroupInvite,
  loadGroupInviteDetail,
  updateGroupInvite,
  type GroupInviteLibraryDraft,
  type GroupInviteLibraryFailure,
  type GroupInviteLibraryItem,
} from "./group-invite-library";
import {
  INITIAL_MEDIA_ASSETS_CENTER_QUERY,
  MEDIA_ASSET_NAVIGATION,
  MediaAssetsCenterReadController,
  canAccessMediaAssetsCenter,
  createMediaAssetsCenterLoaders,
  firstPageMediaAssetsQuery,
  generatedMediaAssetsCenterTransports,
  imageCenterDeleteIdempotencyKey,
  imageReferenceBlockerRows,
  imageReferenceBlockerTotal,
  mediaAssetFailureMessage,
  mediaMutationIdempotencyKey,
  verifyGroupInviteArchived,
  verifyGroupInviteReadback,
  verifyImageDeleted,
  verifyImageReadback,
  verifyImageUploadReadback,
  verifyMiniProgramDeleted,
  verifyMiniProgramReadback,
  withMediaAssetOffset,
  type ImageReferenceBlockerRow,
  type MediaAssetKind,
  type MediaAssetLoadedSection,
  type MediaAssetNavigationItem,
  type MediaAssetSection,
  type MediaAssetsCenterFilters,
  type MediaAssetsCenterQuery,
  type MediaAssetsCenterRole,
  type MediaAssetsCenterSnapshot,
  type MediaAssetsCenterTransports,
  type MediaMutationOperation,
  type MediaWriteReadbackResult,
} from "./media-assets-center";
import "./media-assets-center.css";

const IMAGE_FAILURE_MESSAGES: Record<ImageLibraryFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有图片素材写权限。",
  conflict: "图片素材存在引用或版本冲突；系统不会自动重试。",
  invalid: "图片文件、元数据或服务端响应未通过已冻结合同校验。",
  unavailable: "写入结果未知；本资源写操作已锁定，系统不会自动重试。",
};

const EMPTY_IMAGE_UPLOAD: ImageUploadMetadata = {
  name: "",
  description: "",
  tags: "",
  category: "",
};

function imageMetadataDraft(image: ImageDetail): ImageMetadataDraft {
  return {
    name: image.name,
    description: image.description,
    tags: image.tags.join(","),
    category: image.category,
  };
}

const MINI_FAILURE_MESSAGES: Record<MiniProgramFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有小程序卡片写权限。",
  not_found: "该小程序卡片已不存在，请刷新列表。",
  conflict: "小程序卡片存在版本或引用冲突；系统不会自动重试。",
  invalid: "小程序卡片请求或响应未通过已冻结合同校验。",
  unavailable: "写入结果未知；本资源写操作已锁定，系统不会自动重试。",
};

const GROUP_FAILURE_MESSAGES: Record<GroupInviteLibraryFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有群邀请素材写权限。",
  not_found: "该群邀请素材已不存在或已归档。",
  conflict: "群邀请素材存在版本或引用冲突；系统不会自动重试。",
  invalid: "群邀请素材请求或响应未通过已冻结合同校验。",
  unavailable: "写入结果未知；本资源写操作已锁定，系统不会自动重试。",
};

const EMPTY_GROUP_DRAFT: GroupInviteLibraryDraft = {
  name: "",
  title: "",
  description: "",
  joinURL: "",
  enabled: true,
};

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

function readMutationCookie(readCookie: () => string): string {
  try {
    return readCookie();
  } catch {
    return "";
  }
}

function readMutationCSRF(readCookie: () => string): string | undefined {
  return readCSRFCookie(readMutationCookie(readCookie));
}

function sectionTotal<T>(section: MediaAssetSection<T>): string {
  return section.status === "loaded" ? String(section.total) : "—";
}

function navigationStatus<T>(section: MediaAssetSection<T>): string {
  if (section.status === "error") return "读取失败";
  if (section.visibleCount === 0) return "空";
  return `${section.visibleCount} 条可见`;
}

export interface MediaAssetsOverviewProps {
  readonly activeTab: MediaAssetKind;
  readonly snapshot?: MediaAssetsCenterSnapshot;
  // eslint-disable-next-line no-unused-vars -- named callback parameter documents navigation contract.
  readonly onSelect: (kind: MediaAssetKind) => void;
}

function LoadingNavigationCard({
  item,
  active,
  onSelect,
}: {
  readonly item: MediaAssetNavigationItem;
  readonly active: boolean;
  readonly onSelect: () => void;
}): React.ReactElement {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      className="media-assets-center__nav-card"
      onClick={onSelect}
    >
      <span>{item.label}</span>
      <strong>读取中</strong>
      <small>{item.localFactDescription}</small>
    </button>
  );
}

function NavigationCard<T>({
  item,
  active,
  section,
  onSelect,
}: {
  readonly item: MediaAssetNavigationItem;
  readonly active: boolean;
  readonly section: MediaAssetSection<T>;
  readonly onSelect: () => void;
}): React.ReactElement {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      className="media-assets-center__nav-card"
      onClick={onSelect}
    >
      <span>{item.label}</span>
      <strong>{sectionTotal(section)}</strong>
      <small>{navigationStatus(section)} · {item.localFactDescription}</small>
    </button>
  );
}

export function MediaAssetsOverview({
  activeTab,
  snapshot,
  onSelect,
}: MediaAssetsOverviewProps): React.ReactElement {
  return (
    <nav
      className="media-assets-center__navigation"
      aria-label="媒体资源导航与计数"
      role="tablist"
    >
      {MEDIA_ASSET_NAVIGATION.map((item) => {
        if (!snapshot) {
          return (
            <LoadingNavigationCard
              key={item.kind}
              item={item}
              active={item.kind === activeTab}
              onSelect={() => onSelect(item.kind)}
            />
          );
        }
        if (item.kind === "images") {
          return (
            <NavigationCard
              key={item.kind}
              item={item}
              active={item.kind === activeTab}
              section={snapshot.images}
              onSelect={() => onSelect(item.kind)}
            />
          );
        }
        if (item.kind === "miniprograms") {
          return (
            <NavigationCard
              key={item.kind}
              item={item}
              active={item.kind === activeTab}
              section={snapshot.miniprograms}
              onSelect={() => onSelect(item.kind)}
            />
          );
        }
        return (
          <NavigationCard
            key={item.kind}
            item={item}
            active={item.kind === activeTab}
            section={snapshot.groupInvites}
            onSelect={() => onSelect(item.kind)}
          />
        );
      })}
    </nav>
  );
}

export function ImageReferenceBlockerTable({
  imageID,
  references,
}: {
  readonly imageID: number;
  readonly references: ImageDeleteReferenceCounts;
}): React.ReactElement {
  const rows = imageReferenceBlockerRows(references);
  return (
    <section
      className="media-assets-center__blocker"
      aria-label="图片引用阻塞明细"
      role="alert"
    >
      <h3>图片 #{imageID} 未删除</h3>
      <p>
        本地删除端点已完成引用检查，共发现 {imageReferenceBlockerTotal(references)}
        项引用。引用清理前不会绕过检查或强制删除。
      </p>
      <table>
        <thead>
          <tr><th>引用类型</th><th>数量</th><th>处理结果</th></tr>
        </thead>
        <tbody>
          {rows.map((row: ImageReferenceBlockerRow) => (
            <tr key={row.key}>
              <td>{row.label}</td>
              <td>{row.count}</td>
              <td>阻塞删除</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

export function StructuredMutationBlocker({
  resource,
  operation,
  detail,
}: {
  readonly resource: string;
  readonly operation: string;
  readonly detail: string;
}): React.ReactElement {
  return (
    <dl className="media-assets-center__blocker" role="alert">
      <dt>资源</dt><dd>{resource}</dd>
      <dt>操作</dt><dd>{operation}</dd>
      <dt>阻塞类型</dt><dd>本地引用或版本冲突</dd>
      <dt>服务端结果</dt><dd>{detail}</dd>
      <dt>自动重试</dt><dd>未执行</dd>
    </dl>
  );
}

function ReadState({
  loading,
  section,
  resourceLabel,
  onRetry,
}: {
  readonly loading: boolean;
  readonly section?: MediaAssetSection<unknown>;
  readonly resourceLabel: string;
  readonly onRetry: () => void;
}): React.ReactElement | null {
  if (loading && !section) return <p role="status">正在并行读取{resourceLabel}…</p>;
  if (!section || section.status === "loaded") return null;
  return (
    <div className="media-assets-center__state" role="alert">
      <p>{mediaAssetFailureMessage(section.failure)}</p>
      <button type="button" onClick={onRetry}>仅重试本地读取</button>
    </div>
  );
}

function CurrentPageScopeNotice<T>({
  section,
}: {
  readonly section: MediaAssetLoadedSection<T>;
}): React.ReactElement | null {
  return section.filterScope === "server_and_current_page" ? (
    <p className="media-assets-center__scope-note">
      该资源的冻结接口不支持当前全部筛选组合；总数保留服务端事实，当前页再执行本地安全筛选。
    </p>
  ) : null;
}

function ResourcePager<T>({
  kind,
  section,
  disabled,
  onPage,
}: {
  readonly kind: MediaAssetKind;
  readonly section: MediaAssetLoadedSection<T>;
  readonly disabled: boolean;
  // eslint-disable-next-line no-unused-vars -- named parameters document paging contract.
  readonly onPage: (kind: MediaAssetKind, offset: number) => void;
}): React.ReactElement {
  const first = section.sourceCount === 0 ? 0 : section.offset + 1;
  const last = section.offset + section.sourceCount;
  return (
    <div className="media-assets-center__pager" aria-label="资源分页">
      <span>第 {first}–{last} 条 / 共 {section.total} 条</span>
      <button
        type="button"
        disabled={disabled || !section.hasPrevious}
        onClick={() => {
          if (section.previousOffset !== undefined) onPage(kind, section.previousOffset);
        }}
      >
        上一页
      </button>
      <button
        type="button"
        disabled={disabled || !section.hasNext}
        onClick={() => {
          if (section.nextOffset !== undefined) onPage(kind, section.nextOffset);
        }}
      >
        下一页
      </button>
    </div>
  );
}

function WriteLockBanner({ resource }: { readonly resource: string }): React.ReactElement {
  return (
    <section className="media-assets-center__write-lock" role="alert">
      <h3>{resource}写操作已锁定</h3>
      <p>
        上一次写入或写后回读的结果未知。当前页面不会自动重试，也不会生成新的幂等键；请人工核对本地服务端事实后重新进入页面。
      </p>
    </section>
  );
}

function readbackMessage(result: MediaWriteReadbackResult): string {
  if (result.status === "unauthenticated") return "登录状态已失效，写后回读未完成。";
  if (result.status === "forbidden") return "写后回读被权限拒绝。";
  if (result.status === "conflict") return "写后回读与写响应不一致。";
  return "写后回读结果未知。";
}

interface ImageOperationsProps {
  readonly section: MediaAssetLoadedSection<ImageItem>;
  readonly transport: MediaAssetsCenterTransports["images"];
  readonly readCookie: () => string;
  readonly onUnauthenticated?: () => void;
  readonly writeLocked: boolean;
  readonly onUnknown: () => void;
  readonly onReload: () => void;
  readonly pagingBusy: boolean;
  // eslint-disable-next-line no-unused-vars -- named parameters document paging contract.
  readonly onPage: (kind: MediaAssetKind, offset: number) => void;
}

type CenterImageDetailState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly imageID: number;
      readonly previous?: ImageDetail;
    }
  | { readonly kind: "ready"; readonly image: ImageDetail }
  | {
      readonly kind: "error";
      readonly imageID: number;
      readonly failure: ImageLibraryFailure;
      readonly previous?: ImageDetail;
    };

function currentImageDetail(
  state: CenterImageDetailState,
): ImageDetail | undefined {
  if (state.kind === "ready") return state.image;
  return state.kind === "loading" || state.kind === "error"
    ? state.previous
    : undefined;
}

function ImageOperations({
  section,
  transport,
  readCookie,
  onUnauthenticated,
  writeLocked,
  onUnknown,
  onReload,
  pagingBusy,
  onPage,
}: ImageOperationsProps): React.ReactElement {
  const [detailState, setDetailState] = useState<CenterImageDetailState>({
    kind: "idle",
  });
  const [metadataDraft, setMetadataDraft] = useState<ImageMetadataDraft>();
  const [uploadDraft, setUploadDraft] =
    useState<ImageUploadMetadata>(EMPTY_IMAGE_UPLOAD);
  const [uploadFile, setUploadFile] = useState<File>();
  const [notice, setNotice] = useState<string>();
  const [blocker, setBlocker] = useState<{
    readonly imageID: number;
    readonly references: ImageDeleteReferenceCounts;
  }>();
  const [conflict, setConflict] = useState<string>();
  const [confirming, setConfirming] = useState<number>();
  const [busy, setBusy] = useState(false);
  const [previewMode, setPreviewMode] =
    useState<ImagePreviewMode>("standard");
  const [previewErrorMode, setPreviewErrorMode] =
    useState<ImagePreviewMode>();
  const writeInFlight = useRef(false);
  const detailGeneration = useRef(0);
  const verifiedDetail = useRef<ImageDetail>();
  const fileInput = useRef<HTMLInputElement | null>(null);
  const detail = currentImageDetail(detailState);

  useEffect(
    () => () => {
      detailGeneration.current += 1;
    },
    [],
  );

  const lockUnknown = (message: string) => {
    setNotice(message);
    onUnknown();
  };

  const handleFailure = (
    failure: ImageLibraryFailure | "csrf_missing",
    operation: string,
  ) => {
    if (failure === "unauthenticated") onUnauthenticated?.();
    if (failure === "unavailable") {
      lockUnknown(IMAGE_FAILURE_MESSAGES[failure]);
      return;
    }
    if (failure === "conflict") {
      setConflict(`${operation}被本地服务端的引用或版本检查阻塞。`);
    }
    setNotice(
      failure === "csrf_missing"
        ? "安全令牌缺失，未发送写请求。"
        : IMAGE_FAILURE_MESSAGES[failure],
    );
  };

  const loadDetail = async (imageID: number) => {
    const generation = ++detailGeneration.current;
    const previous =
      verifiedDetail.current?.id === imageID
        ? verifiedDetail.current
        : undefined;
    setDetailState({ kind: "loading", imageID, ...(previous ? { previous } : {}) });
    setNotice(undefined);
    setConflict(undefined);
    setBlocker(undefined);
    setPreviewMode("standard");
    setPreviewErrorMode(undefined);
    const result = await loadImageDetail(transport, imageID);
    if (generation !== detailGeneration.current) return;
    if (result.status !== "loaded") {
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setDetailState({
        kind: "error",
        imageID,
        failure: result.status,
        ...(previous ? { previous } : {}),
      });
      return;
    }
    verifiedDetail.current = result.image;
    setDetailState({ kind: "ready", image: result.image });
    setMetadataDraft(imageMetadataDraft(result.image));
  };

  const commitVerifiedDetail = (image: ImageDetail, message: string) => {
    verifiedDetail.current = image;
    setDetailState({ kind: "ready", image });
    setMetadataDraft(imageMetadataDraft(image));
    setNotice(message);
    setConflict(undefined);
    setBlocker(undefined);
    onReload();
  };

  const completeDetailReadback = async (
    expected: ImageDetail,
    message: string,
  ) => {
    const readback = await verifyImageReadback(transport, expected);
    if (readback.status !== "verified") {
      if (readback.status === "unauthenticated") onUnauthenticated?.();
      lockUnknown(`${readbackMessage(readback)} ${message}未被标记为完成。`);
      return;
    }
    commitVerifiedDetail(expected, message);
  };

  const submitUpload = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (writeLocked || writeInFlight.current) return;
    if (!uploadFile) {
      setNotice("请先选择本地图片文件。");
      return;
    }
    const fileProblem = uploadFileProblem(uploadFile);
    const metadataProblem = uploadMetadataProblem(uploadDraft);
    if (fileProblem || metadataProblem) {
      setNotice(fileProblem ?? metadataProblem);
      return;
    }
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setConflict(undefined);
    setBlocker(undefined);
    try {
      const result = await uploadImage(
        transport,
        readMutationCookie(readCookie),
        uploadFile,
        uploadDraft,
        uploadIdempotencyKey(),
      );
      if (result.status !== "uploaded") {
        handleFailure(result.status, "创建");
        return;
      }
      const readback = await verifyImageUploadReadback(
        transport,
        result.image,
        uploadDraft,
      );
      if (readback.status !== "verified") {
        if (readback.status === "unauthenticated") onUnauthenticated?.();
        lockUnknown(`${readbackMessage(readback)} 图片创建结果未被标记为完成。`);
        return;
      }
      if (fileInput.current) fileInput.current.value = "";
      setUploadFile(undefined);
      setUploadDraft(EMPTY_IMAGE_UPLOAD);
      commitVerifiedDetail(
        readback.image,
        `图片已创建为本地素材 #${readback.image.id}，并通过写后回读。`,
      );
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  const saveMetadata = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!detail || !metadataDraft || writeLocked || writeInFlight.current) return;
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setConflict(undefined);
    setBlocker(undefined);
    try {
      const result = await updateImageMetadata(
        transport,
        readMutationCookie(readCookie),
        detail.id,
        metadataDraft,
      );
      if (result.status !== "saved") {
        handleFailure(result.status, "元数据保存");
        return;
      }
      await completeDetailReadback(
        result.image,
        "图片元数据已保存并通过本地写后回读。",
      );
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  const toggle = async () => {
    if (!detail || writeLocked || writeInFlight.current) return;
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setConflict(undefined);
    setBlocker(undefined);
    try {
      const result = await setImageEnabled(
        transport,
        readMutationCookie(readCookie),
        detail.id,
        !detail.enabled,
      );
      if (result.status !== "saved") {
        handleFailure(result.status, "启停");
        return;
      }
      await completeDetailReadback(
        result.image,
        `图片已${result.image.enabled ? "启用" : "停用"}并通过本地写后回读。`,
      );
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!detail || writeLocked || writeInFlight.current) return;
    if (confirming !== detail.id) {
      setConfirming(detail.id);
      setBlocker(undefined);
      setConflict(undefined);
      setNotice(`请再次确认删除图片 #${detail.id}。删除端点会先执行本地引用检查。`);
      return;
    }
    const imageID = detail.id;
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setBlocker(undefined);
    setConflict(undefined);
    try {
      const result = await deleteImage(
        transport,
        readMutationCookie(readCookie),
        imageID,
        imageCenterDeleteIdempotencyKey(),
      );
      if (result.status === "referenced") {
        setConfirming(undefined);
        setBlocker({ imageID, references: result.references });
        return;
      }
      if (result.status !== "deleted") {
        setConfirming(undefined);
        handleFailure(result.status, "删除");
        return;
      }
      const readback = await verifyImageDeleted(transport, imageID);
      if (readback.status !== "verified") {
        if (readback.status === "unauthenticated") onUnauthenticated?.();
        lockUnknown(`${readbackMessage(readback)} 图片删除结果未被标记为完成。`);
        return;
      }
      detailGeneration.current += 1;
      verifiedDetail.current = undefined;
      setDetailState({ kind: "idle" });
      setMetadataDraft(undefined);
      setConfirming(undefined);
      setNotice(`图片 #${imageID} 已删除，并通过本地不存在回读。`);
      onReload();
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  return (
    <section className="media-assets-center__resource" aria-labelledby="media-images-title">
      <div className="media-assets-center__resource-heading">
        <div>
          <h2 id="media-images-title">图片素材</h2>
          <p>
            只管理本地文件与元数据；预览仅使用详情响应中已验证的本地变体地址，不证明公开访问、对象访问或外发可用。
          </p>
        </div>
      </div>
      <CurrentPageScopeNotice section={section} />
      {notice ? <p role="status">{notice}</p> : null}
      {writeLocked ? <WriteLockBanner resource="图片素材" /> : null}
      {blocker ? (
        <ImageReferenceBlockerTable
          imageID={blocker.imageID}
          references={blocker.references}
        />
      ) : null}
      {conflict ? (
        <StructuredMutationBlocker
          resource="图片素材"
          operation="创建/编辑/启停/删除"
          detail={conflict}
        />
      ) : null}

      <div className="media-assets-center__operation-grid">
        <section aria-label="图片素材列表">
          {section.items.length === 0 ? (
            <p className="media-assets-center__empty" role="status">
              当前筛选下没有图片素材。
            </p>
          ) : (
            <ul className="media-assets-center__select-list">
              {section.items.map((image) => (
                <li key={image.id}>
                  <button
                    type="button"
                    aria-pressed={detail?.id === image.id}
                    disabled={busy}
                    onClick={() => void loadDetail(image.id)}
                  >
                    <strong>{image.name || image.fileName}</strong>
                    <span>本地 ID #{image.id} · {image.enabled ? "启用" : "停用"}</span>
                    <span>{image.category || "未分类"} · {image.width}×{image.height}</span>
                    <span>{formatFileSize(image.fileSize)} · {image.tags.length} 个标签</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <ResourcePager
            kind="images"
            section={section}
            disabled={pagingBusy || busy}
            onPage={onPage}
          />

          <form className="media-assets-center__editor" onSubmit={submitUpload}>
            <h3>创建本地图片素材</h3>
            <p>只上传到本地 Media 存储；不上传到外部平台，也不支持数据 URL 或历史导入。</p>
            <fieldset disabled={busy || writeLocked}>
              <label>
                图片文件
                <input
                  ref={fileInput}
                  type="file"
                  accept="image/png,image/jpeg,image/gif"
                  onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
                    setUploadFile(event.currentTarget.files?.[0])
                  }
                />
              </label>
              <label>名称<input maxLength={200} value={uploadDraft.name} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setUploadDraft({ ...uploadDraft, name: event.currentTarget.value })} /></label>
              <label>描述<textarea maxLength={10000} value={uploadDraft.description} onChange={(event: React.ChangeEvent<HTMLTextAreaElement>) => setUploadDraft({ ...uploadDraft, description: event.currentTarget.value })} /></label>
              <label>标签<input maxLength={10000} value={uploadDraft.tags} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setUploadDraft({ ...uploadDraft, tags: event.currentTarget.value })} /></label>
              <label>分类<input maxLength={200} value={uploadDraft.category} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setUploadDraft({ ...uploadDraft, category: event.currentTarget.value })} /></label>
              <button type="submit">创建本地图片</button>
            </fieldset>
          </form>
        </section>

        <section className="media-assets-center__editor" aria-label="图片素材本地详情与编辑">
          <h3>图片本地详情</h3>
          {detailState.kind === "loading" ? <p role="status">正在读取本地图片详情…</p> : null}
          {detailState.kind === "error" ? <p role="alert">{IMAGE_FAILURE_MESSAGES[detailState.failure]}</p> : null}
          {!detail ? <p className="media-assets-center__empty">从左侧选择图片查看详情。</p> : null}
          {detail ? (
            <>
              <ImagePreviewPanel
                image={detail}
                mode={previewMode}
                errorMode={previewErrorMode}
                onSelectMode={(mode) => {
                  setPreviewMode(mode);
                  setPreviewErrorMode(undefined);
                }}
                onPreviewError={setPreviewErrorMode}
              />
              <dl className="media-assets-center__detail">
                <dt>本地 ID</dt><dd>{detail.id}</dd>
                <dt>文件名</dt><dd>{detail.fileName}</dd>
                <dt>MIME</dt><dd>{detail.mimeType}</dd>
                <dt>尺寸</dt><dd>{detail.width}×{detail.height}</dd>
                <dt>大小</dt><dd>{formatFileSize(detail.fileSize)}</dd>
                <dt>状态</dt><dd>{detail.enabled ? "启用" : "停用"}</dd>
                <dt>更新时间</dt><dd>{detail.updatedAt}</dd>
              </dl>
              {metadataDraft ? (
                <form onSubmit={saveMetadata}>
                  <fieldset disabled={busy || writeLocked}>
                    <label>名称<input maxLength={200} value={metadataDraft.name} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setMetadataDraft({ ...metadataDraft, name: event.currentTarget.value })} /></label>
                    <label>描述<textarea maxLength={10000} value={metadataDraft.description} onChange={(event: React.ChangeEvent<HTMLTextAreaElement>) => setMetadataDraft({ ...metadataDraft, description: event.currentTarget.value })} /></label>
                    <label>标签<input maxLength={10000} value={metadataDraft.tags} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setMetadataDraft({ ...metadataDraft, tags: event.currentTarget.value })} /></label>
                    <label>分类<input maxLength={200} value={metadataDraft.category} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setMetadataDraft({ ...metadataDraft, category: event.currentTarget.value })} /></label>
                    <button type="submit">保存本地元数据</button>
                  </fieldset>
                </form>
              ) : null}
              <div className="media-assets-center__actions">
                <button type="button" disabled={busy} onClick={() => void loadDetail(detail.id)}>刷新本地详情</button>
                <button type="button" disabled={busy || writeLocked} onClick={() => void toggle()}>{detail.enabled ? "停用本地图片" : "启用本地图片"}</button>
                <button type="button" disabled={busy || writeLocked} onClick={() => void remove()}>{confirming === detail.id ? "确认删除本地图片" : "删除前检查引用"}</button>
              </div>
            </>
          ) : null}
        </section>
      </div>
    </section>
  );
}

interface MiniProgramOperationsProps {
  readonly section: MediaAssetLoadedSection<MiniProgramRecord>;
  readonly transport: MediaAssetsCenterTransports["miniprograms"];
  readonly readCookie: () => string;
  readonly randomUUID?: () => string;
  readonly onUnauthenticated?: () => void;
  readonly writeLocked: boolean;
  // eslint-disable-next-line no-unused-vars -- named parameter documents the failure callback contract.
  readonly onUnknown: (message: string) => void;
  readonly onReload: () => void;
  readonly pagingBusy: boolean;
  // eslint-disable-next-line no-unused-vars -- named parameters document paging contract.
  readonly onPage: (kind: MediaAssetKind, offset: number) => void;
}

function miniDraftWithImage(
  draft: MiniProgramDraft,
  image: LibraryImage,
): MiniProgramDraft {
  return {
    ...draft,
    thumbImageID: image.id,
    thumbImageName: image.name || image.fileName,
  };
}

function MiniProgramOperations({
  section,
  transport,
  readCookie,
  randomUUID,
  onUnauthenticated,
  writeLocked,
  onUnknown,
  onReload,
  pagingBusy,
  onPage,
}: MiniProgramOperationsProps): React.ReactElement {
  const [selected, setSelected] = useState<MiniProgramRecord>();
  const [draft, setDraft] = useState<MiniProgramDraft>(editorDraft());
  const [detail, setDetail] = useState<MiniProgramRecord>();
  const [notice, setNotice] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [blocker, setBlocker] = useState<string>();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [imageSearch, setImageSearch] = useState("");
  const [imageOffset, setImageOffset] = useState(0);
  const [imageState, setImageState] = useState<
    | { readonly status: "idle" | "loading" }
    | { readonly status: "loaded"; readonly result: Extract<LibraryImageListResult, { readonly status: "loaded" }> }
    | { readonly status: "error"; readonly failure: MiniProgramFailure }
  >({ status: "idle" });
  const writeInFlight = useRef(false);
  const detailGeneration = useRef(0);
  const imageGeneration = useRef(0);

  const commandKey = (operation: MediaMutationOperation): string | undefined =>
    mediaMutationIdempotencyKey(
      "miniprograms",
      operation,
      randomUUID,
    );

  const lockUnknown = (message: string) => {
    setNotice(message);
    onUnknown(message);
  };

  const handleFailure = (failure: MiniProgramFailure, operation: string) => {
    if (failure === "unauthenticated") onUnauthenticated?.();
    if (failure === "unavailable") {
      lockUnknown(MINI_FAILURE_MESSAGES[failure]);
      return;
    }
    if (failure === "conflict") {
      setBlocker(`${operation}被本地服务端的引用或版本检查阻塞。`);
    }
    setNotice(MINI_FAILURE_MESSAGES[failure]);
  };

  const startCreate = () => {
    if (busy) return;
    detailGeneration.current += 1;
    setSelected(undefined);
    setDetail(undefined);
    setDraft(editorDraft());
    setConfirmingDelete(false);
    setBlocker(undefined);
    setNotice("已开始创建本地小程序卡片。缩略图只能从现有本地图片中选择。" );
  };

  const select = (item: MiniProgramRecord) => {
    if (busy) return;
    detailGeneration.current += 1;
    setSelected(item);
    setDetail(item);
    setDraft(editorDraft(item));
    setConfirmingDelete(false);
    setBlocker(undefined);
    setNotice(undefined);
  };

  const refreshDetail = async () => {
    if (!selected) return;
    const generation = ++detailGeneration.current;
    setNotice("正在读取小程序卡片本地详情。" );
    const result = await loadMiniProgramDetail(transport, selected.id);
    if (generation !== detailGeneration.current) return;
    if (result.status === "loaded") {
      setSelected(result.item);
      setDetail(result.item);
      setDraft(editorDraft(result.item));
      setNotice("本地详情已刷新。" );
      return;
    }
    handleFailure(result.status, "详情读取");
  };

  const completeReadback = async (
    expected: MiniProgramRecord,
    successMessage: string,
  ) => {
    const readback = await verifyMiniProgramReadback(transport, expected);
    if (readback.status !== "verified") {
      if (readback.status === "unauthenticated") onUnauthenticated?.();
      lockUnknown(`${readbackMessage(readback)} ${successMessage}未被标记为完成。`);
      return;
    }
    setSelected(expected);
    setDetail(expected);
    setDraft(editorDraft(expected));
    setNotice(successMessage);
    onReload();
  };

  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (writeLocked || writeInFlight.current) return;
    const problem = draftProblem(draft);
    if (problem) {
      setNotice(problem);
      return;
    }
    const csrf = readMutationCSRF(readCookie);
    const key = commandKey(selected ? "update" : "create");
    if (!csrf || !key) {
      setNotice("安全令牌或幂等命令标识缺失，未发送保存请求。" );
      return;
    }
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setBlocker(undefined);
    try {
      const result = await saveMiniProgram(transport, selected, draft, csrf, key);
      if (result.status !== "saved") {
        handleFailure(result.status, selected ? "编辑" : "创建");
        return;
      }
      await completeReadback(
        result.item,
        selected
          ? "小程序卡片已保存并通过本地写后回读。"
          : "小程序卡片已创建并通过本地写后回读。",
      );
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  const toggle = async () => {
    if (!selected || writeLocked || writeInFlight.current) return;
    const csrf = readMutationCSRF(readCookie);
    const key = commandKey("toggle");
    if (!csrf || !key) {
      setNotice("安全令牌或幂等命令标识缺失，未发送启停请求。" );
      return;
    }
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setBlocker(undefined);
    try {
      const result = await setMiniProgramEnabled(
        transport,
        selected.id,
        !selected.enabled,
        csrf,
        key,
      );
      if (result.status !== "saved") {
        handleFailure(result.status, "启停");
        return;
      }
      await completeReadback(
        result.item,
        `小程序卡片已${result.item.enabled ? "启用" : "停用"}并通过本地写后回读。`,
      );
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!selected || writeLocked || writeInFlight.current) return;
    if (!confirmingDelete) {
      setConfirmingDelete(true);
      setNotice("请再次确认删除。本地删除端点会执行既有引用检查，冲突时不会重试。" );
      return;
    }
    const csrf = readMutationCSRF(readCookie);
    const key = commandKey("delete");
    if (!csrf || !key) {
      setNotice("安全令牌或幂等命令标识缺失，未发送删除请求。" );
      return;
    }
    const id = selected.id;
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setBlocker(undefined);
    try {
      const result = await deleteMiniProgram(transport, id, csrf, key);
      if (result.status !== "deleted") {
        handleFailure(result.status, "删除");
        setConfirmingDelete(false);
        return;
      }
      const readback = await verifyMiniProgramDeleted(transport, id);
      if (readback.status !== "verified") {
        if (readback.status === "unauthenticated") onUnauthenticated?.();
        lockUnknown(`${readbackMessage(readback)} 删除结果未被标记为完成。`);
        return;
      }
      detailGeneration.current += 1;
      setSelected(undefined);
      setDetail(undefined);
      setDraft(editorDraft());
      setConfirmingDelete(false);
      setNotice("小程序卡片已删除，并通过本地不存在回读。" );
      onReload();
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  const loadImages = useCallback(async () => {
    if (!pickerOpen) return;
    const generation = ++imageGeneration.current;
    setImageState({ status: "loading" });
    const result = await loadLibraryImages(transport, {
      search: imageSearch.trim(),
      offset: imageOffset,
    });
    if (generation !== imageGeneration.current) return;
    if (result.status === "loaded") {
      setImageState({ status: "loaded", result });
      return;
    }
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setImageState({ status: "error", failure: result.status });
  }, [imageOffset, imageSearch, onUnauthenticated, pickerOpen, transport]);

  useEffect(() => {
    if (pickerOpen) void loadImages();
    return () => {
      imageGeneration.current += 1;
    };
  }, [loadImages, pickerOpen]);

  return (
    <section className="media-assets-center__resource" aria-labelledby="media-miniprograms-title">
      <div className="media-assets-center__resource-heading">
        <div>
          <h2 id="media-miniprograms-title">小程序卡片</h2>
          <p>
            本操作区复用既有解析器与 transport，但不提供图片上传、URL 抓取或缩略图缓存解析；缩略图只能绑定已验证的本地 Media 图片 ID。
          </p>
        </div>
        <button type="button" disabled={busy || writeLocked} onClick={startCreate}>新建本地卡片</button>
      </div>
      <CurrentPageScopeNotice section={section} />
      {writeLocked ? <WriteLockBanner resource="小程序卡片" /> : null}
      {notice ? <p role="status">{notice}</p> : null}
      {blocker ? (
        <StructuredMutationBlocker
          resource="小程序卡片"
          operation={confirmingDelete ? "删除" : "保存/启停"}
          detail={blocker}
        />
      ) : null}
      <div className="media-assets-center__operation-grid">
        <section aria-label="小程序卡片列表">
          {section.items.length === 0 ? (
            <p className="media-assets-center__empty" role="status">当前筛选下没有小程序卡片。</p>
          ) : (
            <ul className="media-assets-center__select-list">
              {section.items.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    aria-pressed={selected?.id === item.id}
                    disabled={busy}
                    onClick={() => select(item)}
                  >
                    <strong>{item.name || item.title}</strong>
                    <span>#{item.id} · {item.enabled ? "启用" : "停用"} · 版本 {item.version}</span>
                    <span>{item.thumbImageID === undefined ? "无缩略图" : `本地图片 #${item.thumbImageID}`}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <ResourcePager
            kind="miniprograms"
            section={section}
            disabled={pagingBusy || busy}
            onPage={onPage}
          />
        </section>
        <form className="media-assets-center__editor" onSubmit={save}>
          <h3>{selected ? "编辑本地卡片" : "新建本地卡片"}</h3>
          <fieldset disabled={busy || writeLocked}>
            <label>名称<input maxLength={200} value={draft.name} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, name: event.currentTarget.value })} /></label>
            <label>AppID<input maxLength={120} value={draft.appID} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, appID: event.currentTarget.value })} /></label>
            <label>页面路径<input maxLength={500} value={draft.pagePath} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, pagePath: event.currentTarget.value })} /></label>
            <label>标题<input maxLength={200} value={draft.title} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, title: event.currentTarget.value })} /></label>
            <div className="media-assets-center__thumbnail-picker">
              <p>
                {draft.thumbImageID === undefined
                  ? "未绑定缩略图。"
                  : `已选择现有本地图片 #${draft.thumbImageID}${draft.thumbImageName ? `（${draft.thumbImageName}）` : ""}。`}
              </p>
              <button
                type="button"
                onClick={() => {
                  imageGeneration.current += 1;
                  setPickerOpen((value) => !value);
                }}
              >
                {pickerOpen ? "收起现有图片" : "选择现有本地图片"}
              </button>{" "}
              {draft.thumbImageID !== undefined ? (
                <button type="button" onClick={() => setDraft({ name: draft.name, appID: draft.appID, pagePath: draft.pagePath, title: draft.title })}>清除绑定</button>
              ) : null}
              {pickerOpen ? (
                <div className="media-assets-center__picker" aria-label="现有本地图片选择器">
                  <div role="search">
                    <label>搜索现有图片<input maxLength={200} value={imageSearch} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setImageSearch(event.currentTarget.value)} /></label>
                    <button
                      type="button"
                      onClick={() => {
                        setImageOffset(0);
                        void loadImages();
                      }}
                    >搜索</button>
                  </div>
                  {imageState.status === "loading" ? <p role="status">正在读取现有本地图片…</p> : null}
                  {imageState.status === "error" ? <p role="alert">{MINI_FAILURE_MESSAGES[imageState.failure]}</p> : null}
                  {imageState.status === "loaded" ? (
                    <>
                      {imageState.result.items.length === 0 ? <p role="status">没有匹配的已启用本地图片。</p> : null}
                      <ul>
                        {imageState.result.items.map((image) => (
                          <li key={image.id}>
                            <button
                              type="button"
                              onClick={() => {
                                setDraft(miniDraftWithImage(draft, image));
                                setPickerOpen(false);
                                imageGeneration.current += 1;
                              }}
                            >
                              #{image.id} {image.name || image.fileName}
                            </button>
                          </li>
                        ))}
                      </ul>
                      <div className="media-assets-center__pager">
                        <span>共 {imageState.result.total} 张本地图片</span>
                        <button type="button" disabled={imageOffset === 0} onClick={() => setImageOffset(imagePickerPreviousOffset(imageOffset))}>上一页</button>
                        <button
                          type="button"
                          disabled={!imageState.result.hasMore}
                          onClick={() => setImageOffset(imageState.result.nextOffset ?? imageOffset + imageState.result.items.length)}
                        >下一页</button>
                      </div>
                    </>
                  ) : null}
                </div>
              ) : null}
            </div>
            <button type="submit">{selected ? "保存本地修改" : "创建本地卡片"}</button>
          </fieldset>
          {selected ? (
            <div className="media-assets-center__actions">
              <button type="button" disabled={busy} onClick={() => void refreshDetail()}>刷新本地详情</button>
              <button type="button" disabled={busy || writeLocked} onClick={() => void toggle()}>{selected.enabled ? "停用本地卡片" : "启用本地卡片"}</button>
              <button type="button" disabled={busy || writeLocked} onClick={() => void remove()}>{confirmingDelete ? "确认删除本地卡片" : "删除前执行引用检查"}</button>
            </div>
          ) : null}
          {detail ? (
            <dl className="media-assets-center__detail" aria-label="小程序卡片本地详情">
              <dt>本地 ID</dt><dd>{detail.id}</dd>
              <dt>名称</dt><dd>{detail.name || "—"}</dd>
              <dt>AppID</dt><dd>{detail.appID}</dd>
              <dt>页面路径</dt><dd>{detail.pagePath}</dd>
              <dt>标题</dt><dd>{detail.title}</dd>
              <dt>本地缩略图图片 ID</dt><dd>{detail.thumbImageID ?? "—"}</dd>
              <dt>状态</dt><dd>{detail.enabled ? "启用" : "停用"}</dd>
              <dt>版本</dt><dd>{detail.version}</dd>
              <dt>更新时间</dt><dd>{detail.updatedAt}</dd>
            </dl>
          ) : null}
        </form>
      </div>
    </section>
  );
}

interface GroupInviteOperationsProps {
  readonly section: MediaAssetLoadedSection<GroupInviteLibraryItem>;
  readonly transport: MediaAssetsCenterTransports["groupInvites"];
  readonly readCookie: () => string;
  readonly randomUUID?: () => string;
  readonly onUnauthenticated?: () => void;
  readonly writeLocked: boolean;
  // eslint-disable-next-line no-unused-vars -- named parameter documents the failure callback contract.
  readonly onUnknown: (message: string) => void;
  readonly onReload: () => void;
  readonly pagingBusy: boolean;
  // eslint-disable-next-line no-unused-vars -- named parameters document paging contract.
  readonly onPage: (kind: MediaAssetKind, offset: number) => void;
}

function groupDraft(item: GroupInviteLibraryItem): GroupInviteLibraryDraft {
  return {
    name: item.name,
    title: item.title,
    description: item.description,
    joinURL: item.joinURL,
    enabled: item.enabled,
  };
}

function GroupInviteOperations({
  section,
  transport,
  readCookie,
  randomUUID,
  onUnauthenticated,
  writeLocked,
  onUnknown,
  onReload,
  pagingBusy,
  onPage,
}: GroupInviteOperationsProps): React.ReactElement {
  const [selected, setSelected] = useState<GroupInviteLibraryItem>();
  const [detail, setDetail] = useState<GroupInviteLibraryItem>();
  const [draft, setDraft] = useState<GroupInviteLibraryDraft>(EMPTY_GROUP_DRAFT);
  const [notice, setNotice] = useState<string>();
  const [blocker, setBlocker] = useState<string>();
  const [confirmingArchive, setConfirmingArchive] = useState(false);
  const [busy, setBusy] = useState(false);
  const writeInFlight = useRef(false);
  const detailGeneration = useRef(0);

  const commandKey = (operation: MediaMutationOperation): string | undefined =>
    mediaMutationIdempotencyKey("groupInvites", operation, randomUUID);

  const lockUnknown = (message: string) => {
    setNotice(message);
    onUnknown(message);
  };

  const handleFailure = (failure: GroupInviteLibraryFailure, operation: string) => {
    if (failure === "unauthenticated") onUnauthenticated?.();
    if (failure === "unavailable") {
      lockUnknown(GROUP_FAILURE_MESSAGES[failure]);
      return;
    }
    if (failure === "conflict") {
      setBlocker(`${operation}被本地服务端的引用或版本检查阻塞。`);
    }
    setNotice(GROUP_FAILURE_MESSAGES[failure]);
  };

  const startCreate = () => {
    if (busy) return;
    detailGeneration.current += 1;
    setSelected(undefined);
    setDetail(undefined);
    setDraft(EMPTY_GROUP_DRAFT);
    setConfirmingArchive(false);
    setBlocker(undefined);
    setNotice("已开始创建本地群邀请素材。入群地址只作为本地文本保存，不会被打开或验证。" );
  };

  const select = (item: GroupInviteLibraryItem) => {
    if (busy) return;
    detailGeneration.current += 1;
    setSelected(item);
    setDetail(item);
    setDraft(groupDraft(item));
    setConfirmingArchive(false);
    setBlocker(undefined);
    setNotice(undefined);
  };

  const refreshDetail = async () => {
    if (!selected) return;
    const generation = ++detailGeneration.current;
    setNotice("正在读取群邀请素材本地详情。" );
    const result = await loadGroupInviteDetail(transport, selected.id);
    if (generation !== detailGeneration.current) return;
    if (result.status === "loaded") {
      setSelected(result.item);
      setDetail(result.item);
      setDraft(groupDraft(result.item));
      setNotice("本地详情已刷新。" );
      return;
    }
    handleFailure(result.status, "详情读取");
  };

  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (writeLocked || writeInFlight.current) return;
    const csrf = readMutationCSRF(readCookie);
    const key = commandKey(selected ? "update" : "create");
    if (!csrf || !key) {
      setNotice("安全令牌或幂等命令标识缺失，未发送保存请求。" );
      return;
    }
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setBlocker(undefined);
    try {
      const result = selected
        ? await updateGroupInvite(transport, selected, draft, csrf, key)
        : await createGroupInvite(transport, draft, csrf, key);
      if (result.status !== "saved") {
        handleFailure(result.status === "archived" ? "invalid" : result.status, selected ? "编辑" : "创建");
        return;
      }
      const readback = await verifyGroupInviteReadback(transport, result.item);
      if (readback.status !== "verified") {
        if (readback.status === "unauthenticated") onUnauthenticated?.();
        lockUnknown(`${readbackMessage(readback)} 保存结果未被标记为完成。`);
        return;
      }
      setSelected(result.item);
      setDetail(result.item);
      setDraft(groupDraft(result.item));
      setNotice(selected
        ? "群邀请素材已保存并通过本地写后回读。"
        : "群邀请素材已创建并通过本地写后回读。" );
      onReload();
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  const archive = async () => {
    if (!selected || writeLocked || writeInFlight.current) return;
    if (!confirmingArchive) {
      setConfirmingArchive(true);
      setNotice("请再次确认归档。归档只改变本地记录，不证明群或二维码状态。" );
      return;
    }
    const csrf = readMutationCSRF(readCookie);
    const key = commandKey("archive");
    if (!csrf || !key) {
      setNotice("安全令牌或幂等命令标识缺失，未发送归档请求。" );
      return;
    }
    const id = selected.id;
    writeInFlight.current = true;
    setBusy(true);
    setNotice(undefined);
    setBlocker(undefined);
    try {
      const result = await archiveGroupInvite(transport, selected, csrf, key);
      if (result.status !== "archived") {
        handleFailure(result.status === "saved" ? "invalid" : result.status, "归档");
        setConfirmingArchive(false);
        return;
      }
      const readback = await verifyGroupInviteArchived(transport, id);
      if (readback.status !== "verified") {
        if (readback.status === "unauthenticated") onUnauthenticated?.();
        lockUnknown(`${readbackMessage(readback)} 归档结果未被标记为完成。`);
        return;
      }
      detailGeneration.current += 1;
      setSelected(undefined);
      setDetail(undefined);
      setDraft(EMPTY_GROUP_DRAFT);
      setConfirmingArchive(false);
      setNotice("群邀请素材已归档，并通过本地不存在回读。" );
      onReload();
    } finally {
      writeInFlight.current = false;
      setBusy(false);
    }
  };

  return (
    <section className="media-assets-center__resource" aria-labelledby="media-group-invites-title">
      <div className="media-assets-center__resource-heading">
        <div>
          <h2 id="media-group-invites-title">群邀请素材</h2>
          <p>
            仅展示和编辑本地卡片元数据；本地记录不等于群可用、二维码有效或外部平台已验证。
          </p>
        </div>
        <button type="button" disabled={busy || writeLocked} onClick={startCreate}>新建本地素材</button>
      </div>
      <CurrentPageScopeNotice section={section} />
      {writeLocked ? <WriteLockBanner resource="群邀请素材" /> : null}
      {notice ? <p role="status">{notice}</p> : null}
      {blocker ? (
        <StructuredMutationBlocker
          resource="群邀请素材"
          operation={confirmingArchive ? "归档" : "保存"}
          detail={blocker}
        />
      ) : null}
      <div className="media-assets-center__operation-grid">
        <section aria-label="群邀请素材列表">
          {section.items.length === 0 ? (
            <p className="media-assets-center__empty" role="status">当前筛选下没有本地群邀请素材。</p>
          ) : (
            <ul className="media-assets-center__select-list">
              {section.items.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    aria-pressed={selected?.id === item.id}
                    disabled={busy}
                    onClick={() => select(item)}
                  >
                    <strong>{item.name || item.title}</strong>
                    <span>#{item.id} · {item.enabled ? "启用" : "停用"}</span>
                    <span>{item.coverImageID === undefined ? "无本地封面" : `本地封面图片 #${item.coverImageID}`}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <ResourcePager
            kind="groupInvites"
            section={section}
            disabled={pagingBusy || busy}
            onPage={onPage}
          />
        </section>
        <form className="media-assets-center__editor" onSubmit={save}>
          <h3>{selected ? "编辑本地群邀请素材" : "新建本地群邀请素材"}</h3>
          <p>入群地址只作为本地文本输入；页面不会把它渲染为链接、打开、复制或验证。</p>
          <fieldset disabled={busy || writeLocked}>
            <label>名称（可选）<input maxLength={128} value={draft.name} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, name: event.currentTarget.value })} /></label>
            <label>标题<input required maxLength={128} value={draft.title} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, title: event.currentTarget.value })} /></label>
            <label>说明<textarea maxLength={512} value={draft.description} onChange={(event: React.ChangeEvent<HTMLTextAreaElement>) => setDraft({ ...draft, description: event.currentTarget.value })} /></label>
            <label>入群地址（本地文本）<input required maxLength={2048} value={draft.joinURL} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, joinURL: event.currentTarget.value })} /></label>
            <label className="media-assets-center__checkbox"><input type="checkbox" checked={draft.enabled} onChange={(event: React.ChangeEvent<HTMLInputElement>) => setDraft({ ...draft, enabled: event.currentTarget.checked })} />启用本地卡片</label>
            <button type="submit">{selected ? "保存本地修改" : "创建本地素材"}</button>
          </fieldset>
          {selected ? (
            <div className="media-assets-center__actions">
              <button type="button" disabled={busy} onClick={() => void refreshDetail()}>刷新本地详情</button>
              <button type="button" disabled={busy || writeLocked} onClick={() => void archive()}>{confirmingArchive ? "确认归档本地素材" : "归档本地素材"}</button>
            </div>
          ) : null}
          {detail ? (
            <dl className="media-assets-center__detail" aria-label="群邀请素材本地详情">
              <dt>本地 ID</dt><dd>{detail.id}</dd>
              <dt>名称</dt><dd>{detail.name}</dd>
              <dt>标题</dt><dd>{detail.title}</dd>
              <dt>说明</dt><dd>{detail.description || "—"}</dd>
              <dt>本地封面图片 ID</dt><dd>{detail.coverImageID ?? "—"}</dd>
              <dt>状态</dt><dd>{detail.enabled ? "启用" : "停用"}</dd>
              <dt>版本</dt><dd>{detail.version ?? "—"}</dd>
              <dt>更新时间</dt><dd>{detail.updatedAt}</dd>
            </dl>
          ) : null}
        </form>
      </div>
    </section>
  );
}

export interface MediaAssetsCenterPageProps {
  readonly role: MediaAssetsCenterRole;
  readonly transports?: MediaAssetsCenterTransports;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
  readonly initialTab?: MediaAssetKind;
  readonly randomUUID?: () => string;
}

type PageReadState =
  | { readonly kind: "loading"; readonly previous?: MediaAssetsCenterSnapshot }
  | { readonly kind: "ready"; readonly snapshot: MediaAssetsCenterSnapshot };

function currentSnapshot(state: PageReadState): MediaAssetsCenterSnapshot | undefined {
  return state.kind === "ready" ? state.snapshot : state.previous;
}

function activeSection(
  snapshot: MediaAssetsCenterSnapshot | undefined,
  kind: MediaAssetKind,
): MediaAssetSection<unknown> | undefined {
  if (!snapshot) return undefined;
  return snapshot[kind] as MediaAssetSection<unknown>;
}

export function MediaAssetsCenterPage({
  role,
  transports = generatedMediaAssetsCenterTransports,
  readCookie = browserCookie,
  onUnauthenticated,
  initialTab = "images",
  randomUUID,
}: MediaAssetsCenterPageProps): React.ReactElement {
  const canAccess = canAccessMediaAssetsCenter(role);
  const [activeTab, setActiveTab] = useState<MediaAssetKind>(initialTab);
  const [query, setQuery] = useState<MediaAssetsCenterQuery>(
    INITIAL_MEDIA_ASSETS_CENTER_QUERY,
  );
  const [filterDraft, setFilterDraft] = useState<MediaAssetsCenterFilters>(
    INITIAL_MEDIA_ASSETS_CENTER_QUERY.filters,
  );
  const [readState, setReadState] = useState<PageReadState>({ kind: "loading" });
  const [reloadGeneration, setReloadGeneration] = useState(0);
  const [writeLocks, setWriteLocks] = useState<
    Readonly<Record<MediaAssetKind, boolean>>
  >({ images: false, miniprograms: false, groupInvites: false });
  const controller = useRef(new MediaAssetsCenterReadController());
  const verifiedSnapshot = useRef<MediaAssetsCenterSnapshot>();
  const loaders = useMemo(
    () => createMediaAssetsCenterLoaders(transports),
    [transports],
  );
  const snapshot = currentSnapshot(readState);
  const loading = readState.kind === "loading";

  const notifyUnauthenticated = useCallback(
    (next: MediaAssetsCenterSnapshot) => {
      const sections = [next.images, next.miniprograms, next.groupInvites];
      if (
        sections.some(
          (section) =>
            section.status === "error" && section.failure === "unauthenticated",
        )
      ) {
        onUnauthenticated?.();
      }
    },
    [onUnauthenticated],
  );

  const load = useCallback(
    async (nextQuery: MediaAssetsCenterQuery) => {
      const previous = verifiedSnapshot.current;
      setReadState({ kind: "loading", ...(previous ? { previous } : {}) });
      const result = await controller.current.load(role, nextQuery, loaders);
      if (result.status !== "current") return;
      notifyUnauthenticated(result.snapshot);
      verifiedSnapshot.current = result.snapshot;
      setReadState({ kind: "ready", snapshot: result.snapshot });
    },
    [loaders, notifyUnauthenticated, role],
  );

  useEffect(() => {
    if (!canAccess) {
      controller.current.invalidate();
      return;
    }
    void load(query);
    return () => controller.current.invalidate();
    // activeTab is intentional: switching tabs invalidates any older response,
    // even though all sections share the same frozen query.
  }, [activeTab, canAccess, load, query, reloadGeneration]);

  const selectTab = (kind: MediaAssetKind) => {
    if (kind === activeTab) return;
    controller.current.invalidate();
    setActiveTab(kind);
  };

  const applyFilters = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    controller.current.invalidate();
    setQuery(
      firstPageMediaAssetsQuery({
        filters: {
          ...filterDraft,
          search: filterDraft.search.trim(),
          imageCategory: filterDraft.imageCategory.trim(),
        },
        offsets: query.offsets,
      }),
    );
  };

  const clearFilters = () => {
    controller.current.invalidate();
    setFilterDraft(INITIAL_MEDIA_ASSETS_CENTER_QUERY.filters);
    setQuery(INITIAL_MEDIA_ASSETS_CENTER_QUERY);
  };

  const page = (kind: MediaAssetKind, offset: number) => {
    controller.current.invalidate();
    setQuery((current) => withMediaAssetOffset(current, kind, offset));
  };

  const reload = () => {
    controller.current.invalidate();
    setReloadGeneration((value) => value + 1);
  };

  const lockWrite = (kind: MediaAssetKind) => {
    setWriteLocks((current) => ({ ...current, [kind]: true }));
  };

  if (!canAccess) {
    return (
      <main className="media-assets-center" aria-labelledby="media-assets-center-title">
        <p className="route-card__eyebrow">本地媒体运营</p>
        <h1 id="media-assets-center-title">统一媒体资产运营中心</h1>
        <p role="alert">当前账号没有媒体资产运营中心访问权限。</p>
      </main>
    );
  }

  const section = activeSection(snapshot, activeTab);

  return (
    <main className="media-assets-center" aria-labelledby="media-assets-center-title">
      <header className="media-assets-center__header">
        <div>
          <p className="route-card__eyebrow">本地媒体运营</p>
          <h1 id="media-assets-center-title">统一媒体资产运营中心</h1>
          <p>
            统一读取和管理图片、小程序卡片与群邀请素材的本地事实。页面不会上传到外部平台、抓取 URL、刷新外部媒体标识、发送消息或宣称素材已在外部可用。
          </p>
        </div>
        <button type="button" disabled={loading} onClick={reload}>刷新三个本地区域</button>
      </header>

      <MediaAssetsOverview
        activeTab={activeTab}
        snapshot={snapshot}
        onSelect={selectTab}
      />

      <form className="media-assets-center__filters" onSubmit={applyFilters}>
        <label>
          统一搜索
          <input
            maxLength={200}
            value={filterDraft.search}
            placeholder="名称、标题、文件名或本地 ID"
            onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
              setFilterDraft({ ...filterDraft, search: event.currentTarget.value })
            }
          />
        </label>
        <label>
          本地状态
          <select
            value={filterDraft.status}
            onChange={(event: React.ChangeEvent<HTMLSelectElement>) =>
              setFilterDraft({
                ...filterDraft,
                status: event.currentTarget.value as MediaAssetsCenterFilters["status"],
              })
            }
          >
            <option value="all">全部</option>
            <option value="enabled">仅启用</option>
            <option value="disabled">仅停用</option>
          </select>
        </label>
        <label>
          图片分类（仅图片）
          <input
            maxLength={200}
            value={filterDraft.imageCategory}
            onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
              setFilterDraft({
                ...filterDraft,
                imageCategory: event.currentTarget.value,
              })
            }
          />
        </label>
        <label>
          图片标签（仅图片）
          <input
            maxLength={1000}
            value={filterDraft.imageTags}
            onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
              setFilterDraft({ ...filterDraft, imageTags: event.currentTarget.value })
            }
          />
        </label>
        <label className="media-assets-center__checkbox">
          <input
            type="checkbox"
            checked={filterDraft.imageOnlyUnlabeled}
            onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
              setFilterDraft({
                ...filterDraft,
                imageOnlyUnlabeled: event.currentTarget.checked,
              })
            }
          />
          图片仅看未标注
        </label>
        <button type="submit" disabled={loading}>应用筛选</button>
        <button type="button" disabled={loading} onClick={clearFilters}>清空</button>
      </form>

      <ReadState
        loading={loading}
        section={section}
        resourceLabel={MEDIA_ASSET_NAVIGATION.find((item) => item.kind === activeTab)?.label ?? "媒体资源"}
        onRetry={reload}
      />

      {snapshot?.images.status === "loaded" && activeTab === "images" ? (
        <ImageOperations
          section={snapshot.images}
          transport={transports.images}
          readCookie={readCookie}
          onUnauthenticated={onUnauthenticated}
          writeLocked={writeLocks.images}
          onUnknown={() => lockWrite("images")}
          onReload={reload}
          pagingBusy={loading}
          onPage={page}
        />
      ) : null}

      {snapshot?.miniprograms.status === "loaded" && activeTab === "miniprograms" ? (
        <MiniProgramOperations
          section={snapshot.miniprograms}
          transport={transports.miniprograms}
          readCookie={readCookie}
          randomUUID={randomUUID}
          onUnauthenticated={onUnauthenticated}
          writeLocked={writeLocks.miniprograms}
          onUnknown={() => lockWrite("miniprograms")}
          onReload={reload}
          pagingBusy={loading}
          onPage={page}
        />
      ) : null}

      {snapshot?.groupInvites.status === "loaded" && activeTab === "groupInvites" ? (
        <GroupInviteOperations
          section={snapshot.groupInvites}
          transport={transports.groupInvites}
          readCookie={readCookie}
          randomUUID={randomUUID}
          onUnauthenticated={onUnauthenticated}
          writeLocked={writeLocks.groupInvites}
          onUnknown={() => lockWrite("groupInvites")}
          onReload={reload}
          pagingBusy={loading}
          onPage={page}
        />
      ) : null}
    </main>
  );
}
