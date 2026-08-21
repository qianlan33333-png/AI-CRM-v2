import {
  IMAGE_LIBRARY_PAGE_SIZE,
  generatedImageLibraryTransport,
  imageDeleteIdempotencyKey,
  loadImageDetail,
  loadImages,
  nextPageOffset,
  normalizeTagsInput,
  previousPageOffset,
  type ImageDeleteReferenceCounts,
  type ImageDetail,
  type ImageDetailResult,
  type ImageItem,
  type ImageUploadMetadata,
  type UploadedImage,
  type ImageLibraryFailure,
  type ImageLibraryTransport,
  type ImageListQuery,
  type ImageListResult,
} from "./image-library";
import {
  MINIPROGRAM_PAGE_SIZE,
  generatedMiniProgramLibraryTransport,
  loadMiniProgramDetail,
  loadMiniPrograms,
  type MiniProgramDetailResult,
  type MiniProgramFailure,
  type MiniProgramLibraryTransport,
  type MiniProgramListQuery,
  type MiniProgramListResult,
  type MiniProgramRecord,
} from "./miniprogram-library";
import {
  generatedGroupInviteLibraryTransport,
  loadGroupInviteDetail,
  loadGroupInviteLibrary,
  nextGroupInvitePage,
  previousGroupInvitePage,
  type GroupInviteLibraryDetailResult,
  type GroupInviteLibraryFailure,
  type GroupInviteLibraryItem,
  type GroupInviteLibraryResult,
  type GroupInviteLibraryTransport,
} from "./group-invite-library";

export const MEDIA_ASSET_KINDS = [
  "images",
  "miniprograms",
  "groupInvites",
] as const;

export type MediaAssetKind = (typeof MEDIA_ASSET_KINDS)[number];
export type MediaAssetsCenterRole = "admin" | "ops" | "sales";
export type MediaAssetStatusFilter = "all" | "enabled" | "disabled";
export type MediaAssetReadFailure =
  | ImageLibraryFailure
  | MiniProgramFailure
  | GroupInviteLibraryFailure;

export interface MediaAssetsCenterFilters {
  readonly search: string;
  readonly status: MediaAssetStatusFilter;
  readonly imageCategory: string;
  readonly imageTags: string;
  readonly imageOnlyUnlabeled: boolean;
}

export interface MediaAssetsCenterOffsets {
  readonly images: number;
  readonly miniprograms: number;
  readonly groupInvites: number;
}

export interface MediaAssetsCenterQuery {
  readonly filters: MediaAssetsCenterFilters;
  readonly offsets: MediaAssetsCenterOffsets;
}

export const INITIAL_MEDIA_ASSETS_CENTER_QUERY: MediaAssetsCenterQuery = {
  filters: {
    search: "",
    status: "all",
    imageCategory: "",
    imageTags: "",
    imageOnlyUnlabeled: false,
  },
  offsets: { images: 0, miniprograms: 0, groupInvites: 0 },
};

export interface MediaAssetNavigationItem {
  readonly kind: MediaAssetKind;
  readonly label: string;
  readonly shortLabel: string;
  readonly localFactDescription: string;
}

export const MEDIA_ASSET_NAVIGATION: readonly MediaAssetNavigationItem[] = [
  {
    kind: "images",
    label: "图片素材",
    shortLabel: "图片",
    localFactDescription: "本地图片元数据、变体地址和本地引用事实",
  },
  {
    kind: "miniprograms",
    label: "小程序卡片",
    shortLabel: "小程序",
    localFactDescription: "本地小程序卡片配置；缩略图仅绑定 Media 图片 ID",
  },
  {
    kind: "groupInvites",
    label: "群邀请素材",
    shortLabel: "群邀请",
    localFactDescription: "本地群邀请卡元数据，不证明群或二维码可用",
  },
];

export interface MediaAssetsCenterTransports {
  readonly images: ImageLibraryTransport;
  readonly miniprograms: MiniProgramLibraryTransport;
  readonly groupInvites: GroupInviteLibraryTransport;
}

export const generatedMediaAssetsCenterTransports: MediaAssetsCenterTransports = {
  images: generatedImageLibraryTransport,
  miniprograms: generatedMiniProgramLibraryTransport,
  groupInvites: generatedGroupInviteLibraryTransport,
};

export interface MediaAssetsCenterLoaders {
  // eslint-disable-next-line no-unused-vars -- function parameter names document the adapter contract.
  readonly images: (query: ImageListQuery) => Promise<ImageListResult>;
  readonly miniprograms: (
    // eslint-disable-next-line no-unused-vars -- function parameter name documents the adapter contract.
    query: MiniProgramListQuery,
  ) => Promise<MiniProgramListResult>;
  // eslint-disable-next-line no-unused-vars -- function parameter names document the adapter contract.
  readonly groupInvites: (offset: number) => Promise<GroupInviteLibraryResult>;
}

export function createMediaAssetsCenterLoaders(
  transports: MediaAssetsCenterTransports,
): MediaAssetsCenterLoaders {
  return {
    images: (query) => loadImages(transports.images, query),
    miniprograms: (query) => loadMiniPrograms(transports.miniprograms, query),
    groupInvites: (offset) => loadGroupInviteLibrary(transports.groupInvites, offset),
  };
}

export function canAccessMediaAssetsCenter(
  role: MediaAssetsCenterRole,
): boolean {
  return role === "admin" || role === "ops";
}

export interface MediaAssetLoadedSection<T> {
  readonly status: "loaded";
  readonly items: readonly T[];
  readonly total: number;
  readonly offset: number;
  readonly limit: number;
  readonly sourceCount: number;
  readonly visibleCount: number;
  readonly hasPrevious: boolean;
  readonly hasNext: boolean;
  readonly previousOffset?: number;
  readonly nextOffset?: number;
  readonly filterScope: "server" | "server_and_current_page";
}

export interface MediaAssetFailedSection {
  readonly status: "error";
  readonly failure: MediaAssetReadFailure;
}

export type MediaAssetSection<T> =
  | MediaAssetLoadedSection<T>
  | MediaAssetFailedSection;

export interface MediaAssetsCenterSnapshot {
  readonly query: MediaAssetsCenterQuery;
  readonly images: MediaAssetSection<ImageItem>;
  readonly miniprograms: MediaAssetSection<MiniProgramRecord>;
  readonly groupInvites: MediaAssetSection<GroupInviteLibraryItem>;
  readonly verifiedAt: number;
}

export type MediaAssetsCenterReadResult =
  | { readonly status: "current"; readonly snapshot: MediaAssetsCenterSnapshot }
  | { readonly status: "stale" }
  | { readonly status: "forbidden" };

function normalizeOffset(value: number): number {
  return Number.isSafeInteger(value) && value > 0 ? value : 0;
}

export function firstPageMediaAssetsQuery(
  query: MediaAssetsCenterQuery,
): MediaAssetsCenterQuery {
  return {
    filters: query.filters,
    offsets: { images: 0, miniprograms: 0, groupInvites: 0 },
  };
}

export function withMediaAssetOffset(
  query: MediaAssetsCenterQuery,
  kind: MediaAssetKind,
  offset: number,
): MediaAssetsCenterQuery {
  return {
    filters: query.filters,
    offsets: {
      ...query.offsets,
      [kind]: normalizeOffset(offset),
    },
  };
}

function statusMatches(
  enabled: boolean,
  status: MediaAssetStatusFilter,
): boolean {
  return status === "all" || (status === "enabled" ? enabled : !enabled);
}

function safeSearchText(value: string): string {
  return value.trim().toLocaleLowerCase();
}

export function filterGroupInviteCurrentPage(
  items: readonly GroupInviteLibraryItem[],
  filters: MediaAssetsCenterFilters,
): readonly GroupInviteLibraryItem[] {
  const search = safeSearchText(filters.search);
  return items.filter((item) => {
    if (!statusMatches(item.enabled, filters.status)) return false;
    if (search === "") return true;
    // Deliberately exclude joinURL. The unified center never searches, renders,
    // opens, copies, or otherwise treats a local join URL as a verified QR code.
    const values = [
      item.name,
      item.title,
      item.description,
      String(item.id),
      item.coverImageID === undefined ? "" : String(item.coverImageID),
    ];
    return values.some((value) => value.toLocaleLowerCase().includes(search));
  });
}

function imageSection(
  result: ImageListResult,
  filters: MediaAssetsCenterFilters,
): MediaAssetSection<ImageItem> {
  if (result.status !== "loaded") {
    return { status: "error", failure: result.status };
  }
  const items = result.items.filter((item) =>
    statusMatches(item.enabled, filters.status),
  );
  const previous = result.offset > 0 ? previousPageOffset(result.offset) : undefined;
  const next = nextPageOffset(result);
  return {
    status: "loaded",
    items,
    total: result.total,
    offset: result.offset,
    limit: result.limit,
    sourceCount: result.count,
    visibleCount: items.length,
    hasPrevious: previous !== undefined,
    hasNext: next !== undefined,
    ...(previous === undefined ? {} : { previousOffset: previous }),
    ...(next === undefined ? {} : { nextOffset: next }),
    filterScope:
      filters.status === "disabled" ? "server_and_current_page" : "server",
  };
}

function miniProgramSection(
  result: MiniProgramListResult,
  filters: MediaAssetsCenterFilters,
): MediaAssetSection<MiniProgramRecord> {
  if (result.status !== "loaded") {
    return { status: "error", failure: result.status };
  }
  const items = result.items.filter((item) =>
    statusMatches(item.enabled, filters.status),
  );
  const previous = result.offset > 0
    ? Math.max(0, result.offset - result.limit)
    : undefined;
  const next = result.offset + result.items.length < result.total
    ? result.offset + result.limit
    : undefined;
  return {
    status: "loaded",
    items,
    total: result.total,
    offset: result.offset,
    limit: result.limit,
    sourceCount: result.items.length,
    visibleCount: items.length,
    hasPrevious: previous !== undefined,
    hasNext: next !== undefined,
    ...(previous === undefined ? {} : { previousOffset: previous }),
    ...(next === undefined ? {} : { nextOffset: next }),
    filterScope:
      filters.status === "disabled" ? "server_and_current_page" : "server",
  };
}

function groupInviteSection(
  result: GroupInviteLibraryResult,
  filters: MediaAssetsCenterFilters,
): MediaAssetSection<GroupInviteLibraryItem> {
  if (result.status !== "loaded") {
    return { status: "error", failure: result.status };
  }
  const items = filterGroupInviteCurrentPage(result.page.items, filters);
  const previous = previousGroupInvitePage(result.page);
  const next = nextGroupInvitePage(result.page);
  return {
    status: "loaded",
    items,
    total: result.page.total,
    offset: result.page.offset,
    limit: result.page.limit,
    sourceCount: result.page.items.length,
    visibleCount: items.length,
    hasPrevious: previous !== undefined,
    hasNext: next !== undefined,
    ...(previous === undefined ? {} : { previousOffset: previous }),
    ...(next === undefined ? {} : { nextOffset: next }),
    filterScope: "server_and_current_page",
  };
}

function unavailableImageResult(): ImageListResult {
  return { status: "unavailable" };
}
function unavailableMiniProgramResult(): MiniProgramListResult {
  return { status: "unavailable" };
}
function unavailableGroupInviteResult(): GroupInviteLibraryResult {
  return { status: "unavailable" };
}

async function safeLoad<T>(
  load: () => Promise<T>,
  unavailable: () => T,
): Promise<T> {
  try {
    return await load();
  } catch {
    return unavailable();
  }
}

export class MediaAssetsCenterReadController {
  private generation = 0;
  private readonly now: () => number;

  constructor(now: () => number = Date.now) {
    this.now = now;
  }

  invalidate(): void {
    this.generation += 1;
  }

  async load(
    role: MediaAssetsCenterRole,
    query: MediaAssetsCenterQuery,
    loaders: MediaAssetsCenterLoaders,
  ): Promise<MediaAssetsCenterReadResult> {
    if (!canAccessMediaAssetsCenter(role)) return { status: "forbidden" };
    const generation = ++this.generation;
    const normalizedQuery: MediaAssetsCenterQuery = {
      filters: {
        search: query.filters.search.trim(),
        status: query.filters.status,
        imageCategory: query.filters.imageCategory.trim(),
        imageTags: query.filters.imageTags,
        imageOnlyUnlabeled: query.filters.imageOnlyUnlabeled,
      },
      offsets: {
        images: normalizeOffset(query.offsets.images),
        miniprograms: normalizeOffset(query.offsets.miniprograms),
        groupInvites: normalizeOffset(query.offsets.groupInvites),
      },
    };
    const imageQuery: ImageListQuery = {
      search: normalizedQuery.filters.search,
      category: normalizedQuery.filters.imageCategory,
      tags: normalizedQuery.filters.imageTags,
      onlyUnlabeled: normalizedQuery.filters.imageOnlyUnlabeled,
      includeDisabled: normalizedQuery.filters.status !== "enabled",
      offset: normalizedQuery.offsets.images,
    };
    const miniProgramQuery: MiniProgramListQuery = {
      search: normalizedQuery.filters.search,
      enabledOnly: normalizedQuery.filters.status === "enabled",
      offset: normalizedQuery.offsets.miniprograms,
    };

    // These reads are intentionally independent. Every leaf loader validates
    // its own closed response; one malformed or failed section is represented
    // only in that section and cannot replace another verified result.
    const [images, miniprograms, groupInvites] = await Promise.all([
      safeLoad(() => loaders.images(imageQuery), unavailableImageResult),
      safeLoad(
        () => loaders.miniprograms(miniProgramQuery),
        unavailableMiniProgramResult,
      ),
      safeLoad(
        () => loaders.groupInvites(normalizedQuery.offsets.groupInvites),
        unavailableGroupInviteResult,
      ),
    ]);

    if (generation !== this.generation) return { status: "stale" };
    return {
      status: "current",
      snapshot: {
        query: normalizedQuery,
        images: imageSection(images, normalizedQuery.filters),
        miniprograms: miniProgramSection(
          miniprograms,
          normalizedQuery.filters,
        ),
        groupInvites: groupInviteSection(
          groupInvites,
          normalizedQuery.filters,
        ),
        verifiedAt: this.now(),
      },
    };
  }
}

/* eslint-disable no-unused-vars -- overload parameter names document the public projection contract. */
export function mediaAssetSection(
  snapshot: MediaAssetsCenterSnapshot,
  kind: "images",
): MediaAssetSection<ImageItem>;
export function mediaAssetSection(
  snapshot: MediaAssetsCenterSnapshot,
  kind: "miniprograms",
): MediaAssetSection<MiniProgramRecord>;
export function mediaAssetSection(
  snapshot: MediaAssetsCenterSnapshot,
  kind: "groupInvites",
): MediaAssetSection<GroupInviteLibraryItem>;
/* eslint-enable no-unused-vars */
export function mediaAssetSection(
  snapshot: MediaAssetsCenterSnapshot,
  kind: MediaAssetKind,
): MediaAssetSection<ImageItem | MiniProgramRecord | GroupInviteLibraryItem> {
  return snapshot[kind] as MediaAssetSection<
    ImageItem | MiniProgramRecord | GroupInviteLibraryItem
  >;
}

export function mediaAssetFailureMessage(
  failure: MediaAssetReadFailure,
): string {
  switch (failure) {
    case "unauthenticated":
      return "登录状态已失效，请重新登录。";
    case "forbidden":
      return "当前账号没有该媒体资源的访问权限。";
    case "not_found":
      return "本地资源已不存在或已归档。";
    case "conflict":
      return "本地资源发生版本或引用冲突；系统不会自动重试。";
    case "invalid":
      return "服务端响应未通过已冻结合同校验，本区数据已拒绝。";
    case "unavailable":
      return "本地媒体资源暂时不可用，本区不会覆盖其他已验证数据。";
  }
}

export interface ImageReferenceBlockerRow {
  readonly key: keyof ImageDeleteReferenceCounts;
  readonly label: string;
  readonly count: number;
}

const IMAGE_REFERENCE_LABELS: Readonly<
  Record<keyof ImageDeleteReferenceCounts, string>
> = {
  miniprograms: "小程序卡片",
  campaignSteps: "活动步骤",
  groupInvites: "群邀请素材",
  automationAgents: "自动化 Agent",
  channels: "渠道配置",
  importPreflights: "导入预检只读记录",
};

export function imageReferenceBlockerRows(
  references: ImageDeleteReferenceCounts,
): readonly ImageReferenceBlockerRow[] {
  return (Object.keys(IMAGE_REFERENCE_LABELS) as Array<
    keyof ImageDeleteReferenceCounts
  >)
    .filter((key) => references[key] > 0)
    .map((key) => ({
      key,
      label: IMAGE_REFERENCE_LABELS[key],
      count: references[key],
    }));
}

export function imageReferenceBlockerTotal(
  references: ImageDeleteReferenceCounts,
): number {
  return imageReferenceBlockerRows(references).reduce(
    (total, row) => total + row.count,
    0,
  );
}

export class MediaAssetsWriteLock {
  private readonly locked = new Set<MediaAssetKind>();

  lock(kind: MediaAssetKind): void {
    this.locked.add(kind);
  }

  isLocked(kind: MediaAssetKind): boolean {
    return this.locked.has(kind);
  }

  snapshot(): Readonly<Record<MediaAssetKind, boolean>> {
    return {
      images: this.isLocked("images"),
      miniprograms: this.isLocked("miniprograms"),
      groupInvites: this.isLocked("groupInvites"),
    };
  }
}

export type MediaMutationOperation =
  | "create"
  | "update"
  | "toggle"
  | "delete"
  | "archive";

export function mediaMutationIdempotencyKey(
  kind: Exclude<MediaAssetKind, "images">,
  operation: MediaMutationOperation,
  randomUUID: (() => string) | undefined = globalThis.crypto?.randomUUID?.bind(
    globalThis.crypto,
  ),
): string | undefined {
  if (!randomUUID) return undefined;
  let value: string;
  try {
    value = randomUUID();
  } catch {
    return undefined;
  }
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)) {
    return undefined;
  }
  const key = `media-center:${kind}:${operation}:${value}`;
  return key.length >= 16 && key.length <= 128 ? key : undefined;
}

export function imageCenterDeleteIdempotencyKey(
  random: () => number = Math.random,
): string {
  return imageDeleteIdempotencyKey(random);
}


export function sameImageDetail(
  left: ImageDetail,
  right: ImageDetail,
): boolean {
  return (
    left.id === right.id &&
    left.name === right.name &&
    left.fileName === right.fileName &&
    left.mimeType === right.mimeType &&
    left.fileSize === right.fileSize &&
    left.enabled === right.enabled &&
    left.description === right.description &&
    left.tags.length === right.tags.length &&
    left.tags.every((tag, index) => tag === right.tags[index]) &&
    left.category === right.category &&
    left.width === right.width &&
    left.height === right.height &&
    left.createdAt === right.createdAt &&
    left.updatedAt === right.updatedAt &&
    left.previewURL === right.previewURL &&
    left.originalURL === right.originalURL
  );
}

function imageReadbackFailure(
  result: Exclude<ImageDetailResult, { readonly status: "loaded" }>,
): Exclude<MediaWriteReadbackResult, { readonly status: "verified" }> {
  if (result.status === "unauthenticated") return { status: "unauthenticated" };
  if (result.status === "forbidden") return { status: "forbidden" };
  if (result.status === "conflict") return { status: "conflict" };
  return { status: "unavailable" };
}

export async function verifyImageReadback(
  transport: ImageLibraryTransport,
  expected: ImageDetail,
): Promise<MediaWriteReadbackResult> {
  const result = await loadImageDetail(transport, expected.id);
  if (result.status !== "loaded") return imageReadbackFailure(result);
  return sameImageDetail(result.image, expected)
    ? { status: "verified" }
    : { status: "conflict" };
}

function normalizedUploadTags(value: string): readonly string[] {
  const normalized = normalizeTagsInput(value);
  return normalized === "" ? [] : normalized.split(",");
}

export async function verifyImageUploadReadback(
  transport: ImageLibraryTransport,
  uploaded: UploadedImage,
  metadata: ImageUploadMetadata,
): Promise<
  | { readonly status: "verified"; readonly image: ImageDetail }
  | Exclude<MediaWriteReadbackResult, { readonly status: "verified" }>
> {
  const result = await loadImageDetail(transport, uploaded.id);
  if (result.status !== "loaded") return imageReadbackFailure(result);
  const expectedTags = normalizedUploadTags(metadata.tags);
  const image = result.image;
  const matches =
    image.id === uploaded.id &&
    image.name === metadata.name.trim() &&
    image.fileName === uploaded.fileName &&
    image.fileSize === uploaded.fileSize &&
    image.width === uploaded.width &&
    image.height === uploaded.height &&
    image.description === metadata.description &&
    image.category === metadata.category.trim() &&
    image.enabled === true &&
    image.tags.length === expectedTags.length &&
    image.tags.every((tag, index) => tag === expectedTags[index]);
  return matches ? { status: "verified", image } : { status: "conflict" };
}

export async function verifyImageDeleted(
  transport: ImageLibraryTransport,
  imageID: number,
): Promise<MediaWriteReadbackResult> {
  if (!Number.isSafeInteger(imageID) || imageID < 1) {
    return { status: "unavailable" };
  }
  try {
    // The leaf detail helper intentionally collapses 404 into unavailable. A
    // delete readback needs the status only, so reuse the same ID-bound
    // transport method without parsing or constructing another protocol.
    const response = await transport.detail(String(imageID));
    if (response.status === 404) return { status: "verified" };
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 200) {
      return { status: "conflict" };
    }
    return { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export function sameMiniProgramRecord(
  left: MiniProgramRecord,
  right: MiniProgramRecord,
): boolean {
  return (
    left.id === right.id &&
    left.name === right.name &&
    left.appID === right.appID &&
    left.pagePath === right.pagePath &&
    left.title === right.title &&
    left.thumbImageID === right.thumbImageID &&
    left.thumbMediaID === right.thumbMediaID &&
    left.enabled === right.enabled &&
    left.version === right.version &&
    left.createdAt === right.createdAt &&
    left.updatedAt === right.updatedAt
  );
}

export function sameGroupInviteRecord(
  left: GroupInviteLibraryItem,
  right: GroupInviteLibraryItem,
): boolean {
  return (
    left.id === right.id &&
    left.name === right.name &&
    left.title === right.title &&
    left.description === right.description &&
    left.joinURL === right.joinURL &&
    left.coverImageID === right.coverImageID &&
    left.enabled === right.enabled &&
    left.createdAt === right.createdAt &&
    left.updatedAt === right.updatedAt &&
    left.createdBy === right.createdBy &&
    left.updatedBy === right.updatedBy &&
    left.version === right.version &&
    left.archivedAt === right.archivedAt
  );
}

export type MediaWriteReadbackResult =
  | { readonly status: "verified" }
  | { readonly status: "unauthenticated" }
  | { readonly status: "forbidden" }
  | { readonly status: "conflict" }
  | { readonly status: "unavailable" };

function miniReadbackFailure(
  result: Exclude<MiniProgramDetailResult, { readonly status: "loaded" }>,
): MediaWriteReadbackResult {
  if (result.status === "unauthenticated") return { status: "unauthenticated" };
  if (result.status === "forbidden") return { status: "forbidden" };
  if (result.status === "conflict") return { status: "conflict" };
  return { status: "unavailable" };
}

export async function verifyMiniProgramReadback(
  transport: MiniProgramLibraryTransport,
  expected: MiniProgramRecord,
): Promise<MediaWriteReadbackResult> {
  const result = await loadMiniProgramDetail(transport, expected.id);
  if (result.status !== "loaded") return miniReadbackFailure(result);
  return sameMiniProgramRecord(result.item, expected)
    ? { status: "verified" }
    : { status: "conflict" };
}

export async function verifyMiniProgramDeleted(
  transport: MiniProgramLibraryTransport,
  id: number,
): Promise<MediaWriteReadbackResult> {
  const result = await loadMiniProgramDetail(transport, id);
  if (result.status === "not_found") return { status: "verified" };
  if (result.status === "unauthenticated") return { status: "unauthenticated" };
  if (result.status === "forbidden") return { status: "forbidden" };
  if (result.status === "loaded" || result.status === "conflict") {
    return { status: "conflict" };
  }
  return { status: "unavailable" };
}

function groupReadbackFailure(
  result: Exclude<GroupInviteLibraryDetailResult, { readonly status: "loaded" }>,
): MediaWriteReadbackResult {
  if (result.status === "unauthenticated") return { status: "unauthenticated" };
  if (result.status === "forbidden") return { status: "forbidden" };
  if (result.status === "conflict") return { status: "conflict" };
  return { status: "unavailable" };
}

export async function verifyGroupInviteReadback(
  transport: GroupInviteLibraryTransport,
  expected: GroupInviteLibraryItem,
): Promise<MediaWriteReadbackResult> {
  const result = await loadGroupInviteDetail(transport, expected.id);
  if (result.status !== "loaded") return groupReadbackFailure(result);
  return sameGroupInviteRecord(result.item, expected)
    ? { status: "verified" }
    : { status: "conflict" };
}

export async function verifyGroupInviteArchived(
  transport: GroupInviteLibraryTransport,
  id: number,
): Promise<MediaWriteReadbackResult> {
  const result = await loadGroupInviteDetail(transport, id);
  if (result.status === "not_found") return { status: "verified" };
  if (result.status === "unauthenticated") return { status: "unauthenticated" };
  if (result.status === "forbidden") return { status: "forbidden" };
  if (result.status === "loaded" || result.status === "conflict") {
    return { status: "conflict" };
  }
  return { status: "unavailable" };
}

export function centerImageListQuery(
  query: MediaAssetsCenterQuery,
): ImageListQuery {
  return {
    search: query.filters.search.trim(),
    category: query.filters.imageCategory.trim(),
    tags: query.filters.imageTags,
    onlyUnlabeled: query.filters.imageOnlyUnlabeled,
    includeDisabled: query.filters.status !== "enabled",
    offset: normalizeOffset(query.offsets.images),
  };
}

export function centerMiniProgramListQuery(
  query: MediaAssetsCenterQuery,
): MiniProgramListQuery {
  return {
    search: query.filters.search.trim(),
    enabledOnly: query.filters.status === "enabled",
    offset: normalizeOffset(query.offsets.miniprograms),
  };
}

export const MEDIA_ASSET_PAGE_SIZES = {
  images: IMAGE_LIBRARY_PAGE_SIZE,
  miniprograms: MINIPROGRAM_PAGE_SIZE,
} as const;

export async function readImageForSafeDelete(
  transport: ImageLibraryTransport,
  imageID: number,
): Promise<ImageDetailResult> {
  return loadImageDetail(transport, imageID);
}
