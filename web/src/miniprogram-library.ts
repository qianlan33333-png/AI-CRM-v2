import {
  createLegacyMiniProgram,
  deleteLegacyMiniProgram,
  getLegacyImageList,
  listLegacyMiniPrograms,
  testResolveLegacyMiniProgram,
  updateLegacyMiniProgram,
  uploadLegacyImage,
  type GetLegacyImageListParams,
  type LegacyMiniProgramCreateRequest,
  type LegacyMiniProgramUpdateRequest,
  type ListLegacyMiniProgramsParams,
  type UploadLegacyImageBody,
} from "./api/generated/health";

export type MiniProgramRole = "admin" | "ops" | "sales";

export const MINIPROGRAM_PAGE_SIZE = 20;
export const IMAGE_PICKER_PAGE_SIZE = 24;
export const MAX_IMAGE_FILE_SIZE = 10_485_760;
const IMAGE_MIME_TYPES: ReadonlySet<string> = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
]);

export interface MiniProgramRecord {
  readonly id: number;
  readonly name: string;
  readonly appID: string;
  readonly pagePath: string;
  readonly title: string;
  readonly thumbImageID?: number;
  readonly thumbMediaID: string;
  readonly enabled: boolean;
  readonly version: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface ThumbnailResolution {
  readonly status: "resolved" | "not_available" | "outcome_unknown";
  readonly cacheOwner: "media.thumbnail_cache";
  readonly cacheReceipt: string;
  readonly thumbMediaID?: string;
  readonly thumbMediaIDExpiresAt?: string;
}

export interface LibraryImage {
  readonly id: number;
  readonly name: string;
  readonly fileName: string;
  readonly mimeType: string;
  readonly fileSize: number;
  readonly width: number;
  readonly height: number;
  readonly enabled: boolean;
  readonly thumb160URL: string;
}

export interface UploadedImage {
  readonly id: number;
  readonly name: string;
}

export interface MiniProgramDraft {
  readonly name: string;
  readonly appID: string;
  readonly pagePath: string;
  readonly title: string;
  readonly thumbImageID?: number;
  readonly thumbImageName?: string;
}

export interface MiniProgramListQuery {
  readonly search: string;
  readonly enabledOnly: boolean;
  readonly offset: number;
}

export interface LibraryImageListQuery {
  readonly search: string;
  readonly offset: number;
}

async function generatedList(
  params: ListLegacyMiniProgramsParams,
  options: RequestInit,
) {
  return listLegacyMiniPrograms(params, options);
}
async function generatedCreate(
  request: LegacyMiniProgramCreateRequest,
  options: RequestInit,
) {
  return createLegacyMiniProgram(request, options);
}
async function generatedUpdate(
  id: number,
  request: LegacyMiniProgramUpdateRequest,
  options: RequestInit,
) {
  return updateLegacyMiniProgram(id, request, options);
}
async function generatedDelete(id: number, options: RequestInit) {
  return deleteLegacyMiniProgram(id, options);
}
async function generatedResolve(id: number, options: RequestInit) {
  return testResolveLegacyMiniProgram(id, options);
}
async function generatedListImages(
  params: GetLegacyImageListParams,
  options: RequestInit,
) {
  return getLegacyImageList(params, options);
}
async function generatedUploadImage(
  body: UploadLegacyImageBody,
  options: RequestInit,
) {
  return uploadLegacyImage(body, options);
}

export interface MiniProgramLibraryTransport {
  readonly list: typeof generatedList;
  readonly create: typeof generatedCreate;
  readonly update: typeof generatedUpdate;
  readonly remove: typeof generatedDelete;
  readonly resolve: typeof generatedResolve;
  readonly listImages: typeof generatedListImages;
  readonly uploadImage: typeof generatedUploadImage;
}

export const generatedMiniProgramLibraryTransport: MiniProgramLibraryTransport =
  {
    list: generatedList,
    create: generatedCreate,
    update: generatedUpdate,
    remove: generatedDelete,
    resolve: generatedResolve,
    listImages: generatedListImages,
    uploadImage: generatedUploadImage,
  };

export type MiniProgramFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";
export type MiniProgramListResult =
  | {
      readonly status: "loaded";
      readonly items: readonly MiniProgramRecord[];
      readonly total: number;
      readonly limit: number;
      readonly offset: number;
    }
  | { readonly status: MiniProgramFailure };
export type MiniProgramMutationResult =
  | {
      readonly status: "saved";
      readonly item: MiniProgramRecord;
      readonly changed: boolean;
      readonly thumbResolve?: ThumbnailResolution;
    }
  | { readonly status: MiniProgramFailure };
export type MiniProgramDeleteResult =
  { readonly status: "deleted" } | { readonly status: MiniProgramFailure };
export type MiniProgramResolveResult =
  | {
      readonly status: "ok";
      readonly item: MiniProgramRecord;
      readonly resolution: ThumbnailResolution;
      readonly changed: boolean;
    }
  | { readonly status: MiniProgramFailure };
export type LibraryImageListResult =
  | {
      readonly status: "loaded";
      readonly items: readonly LibraryImage[];
      readonly total: number;
      readonly hasMore: boolean;
      readonly nextOffset?: number;
    }
  | { readonly status: MiniProgramFailure };
export type LibraryImageUploadResult =
  | { readonly status: "uploaded"; readonly image: UploadedImage }
  | { readonly status: MiniProgramFailure };

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function timestamp(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) &&
    Number.isFinite(Date.parse(value))
  );
}
function runeLength(value: string): number {
  return [...value].length;
}
function frozenText(
  value: unknown,
  limit: number,
  allowEmpty: boolean,
): value is string {
  return (
    typeof value === "string" &&
    (allowEmpty || value.length > 0) &&
    runeLength(value) <= limit &&
    value.trim() === value &&
    !value.includes("\x00")
  );
}
function exactKeys(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  const actual = Object.keys(value);
  return (
    actual.length === keys.length && actual.every((key) => keys.includes(key))
  );
}
function frozenLocalFlags(value: Record<string, unknown>): boolean {
  return (
    value.local_only === true &&
    value.provider_call_executed === false &&
    value.real_external_call_executed === false
  );
}

const MINIPROGRAM_KEYS: readonly string[] = [
  "id",
  "name",
  "appid",
  "pagepath",
  "page_path",
  "title",
  "thumb_image_url",
  "thumb_image_base64",
  "thumb_media_id",
  "thumb_media_id_expires_at",
  "thumb_image_id",
  "enabled",
  "created_at",
  "updated_at",
  "created_by",
  "updated_by",
  "version",
];
const MINIPROGRAM_REQUIRED: readonly string[] = [
  "id",
  "name",
  "appid",
  "pagepath",
  "page_path",
  "title",
  "thumb_image_url",
  "thumb_image_base64",
  "thumb_media_id",
  "enabled",
  "created_at",
  "updated_at",
  "created_by",
  "updated_by",
  "version",
];

export function parseMiniProgram(
  value: unknown,
): MiniProgramRecord | undefined {
  if (!record(value)) return undefined;
  if (Object.keys(value).some((key) => !MINIPROGRAM_KEYS.includes(key))) {
    return undefined;
  }
  if (MINIPROGRAM_REQUIRED.some((key) => !(key in value))) return undefined;
  if (
    !positive(value.id) ||
    !positive(value.version) ||
    !positive(value.created_by) ||
    !positive(value.updated_by)
  ) {
    return undefined;
  }
  if (
    !frozenText(value.name, 200, true) ||
    !frozenText(value.appid, 120, false) ||
    !frozenText(value.pagepath, 500, false) ||
    !frozenText(value.page_path, 500, false) ||
    !frozenText(value.title, 200, false)
  ) {
    return undefined;
  }
  if (value.pagepath !== value.page_path) return undefined;
  if (
    typeof value.thumb_image_url !== "string" ||
    value.thumb_image_url.length > 2048
  ) {
    return undefined;
  }
  if (value.thumb_image_base64 !== "") return undefined;
  if (
    typeof value.thumb_media_id !== "string" ||
    value.thumb_media_id.length > 255
  ) {
    return undefined;
  }
  if (
    "thumb_media_id_expires_at" in value &&
    (typeof value.thumb_media_id_expires_at !== "string" ||
      value.thumb_media_id_expires_at.length < 1 ||
      value.thumb_media_id_expires_at.length > 64)
  ) {
    return undefined;
  }
  if ("thumb_image_id" in value && !positive(value.thumb_image_id)) {
    return undefined;
  }
  if (typeof value.enabled !== "boolean") return undefined;
  if (!timestamp(value.created_at) || !timestamp(value.updated_at)) {
    return undefined;
  }
  return {
    id: value.id,
    name: value.name as string,
    appID: value.appid as string,
    pagePath: value.pagepath as string,
    title: value.title as string,
    ...(positive(value.thumb_image_id)
      ? { thumbImageID: value.thumb_image_id }
      : {}),
    thumbMediaID: value.thumb_media_id,
    enabled: value.enabled,
    version: value.version,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

export function parseThumbnailResolution(
  value: unknown,
): ThumbnailResolution | undefined {
  if (!record(value)) return undefined;
  if (
    !exactKeys(value, [
      "status",
      "cache_owner",
      "cache_receipt",
      "side_effect_executed",
      "real_external_call_executed",
    ]) &&
    !exactKeys(value, [
      "status",
      "cache_owner",
      "cache_receipt",
      "thumb_media_id",
      "side_effect_executed",
      "real_external_call_executed",
    ]) &&
    !exactKeys(value, [
      "status",
      "cache_owner",
      "cache_receipt",
      "thumb_media_id_expires_at",
      "side_effect_executed",
      "real_external_call_executed",
    ]) &&
    !exactKeys(value, [
      "status",
      "cache_owner",
      "cache_receipt",
      "thumb_media_id",
      "thumb_media_id_expires_at",
      "side_effect_executed",
      "real_external_call_executed",
    ])
  ) {
    return undefined;
  }
  if (
    value.status !== "resolved" &&
    value.status !== "not_available" &&
    value.status !== "outcome_unknown"
  ) {
    return undefined;
  }
  if (value.cache_owner !== "media.thumbnail_cache") return undefined;
  if (
    typeof value.cache_receipt !== "string" ||
    value.cache_receipt.length < 1 ||
    value.cache_receipt.length > 512
  ) {
    return undefined;
  }
  if (
    value.side_effect_executed !== false ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  if (
    "thumb_media_id" in value &&
    (typeof value.thumb_media_id !== "string" ||
      value.thumb_media_id.length < 1 ||
      value.thumb_media_id.length > 255)
  ) {
    return undefined;
  }
  if (
    "thumb_media_id_expires_at" in value &&
    (typeof value.thumb_media_id_expires_at !== "string" ||
      value.thumb_media_id_expires_at.length < 1 ||
      value.thumb_media_id_expires_at.length > 64)
  ) {
    return undefined;
  }
  return {
    status: value.status,
    cacheOwner: "media.thumbnail_cache",
    cacheReceipt: value.cache_receipt,
    ...(typeof value.thumb_media_id === "string"
      ? { thumbMediaID: value.thumb_media_id }
      : {}),
    ...(typeof value.thumb_media_id_expires_at === "string"
      ? { thumbMediaIDExpiresAt: value.thumb_media_id_expires_at }
      : {}),
  };
}

const IMAGE_VARIANT_URL =
  /^\/api\/admin\/image-library\/[1-9][0-9]*\/variants\/(thumb_160|thumb_320|mobile_1080|large_1440|original)$/;
function variantURL(value: unknown, variant: string): value is string {
  if (typeof value !== "string") return false;
  const match = IMAGE_VARIANT_URL.exec(value);
  return match !== null && match[1] === variant;
}

const LIBRARY_IMAGE_KEYS: readonly string[] = [
  "id",
  "name",
  "file_name",
  "mime_type",
  "file_size",
  "enabled",
  "description",
  "tags",
  "category",
  "width",
  "height",
  "created_at",
  "updated_at",
  "thumb_160_url",
  "thumb_320_url",
  "thumb_url",
  "preview_url",
  "mobile_1080_url",
  "large_1440_url",
  "original_url",
];

export function parseLibraryImage(value: unknown): LibraryImage | undefined {
  if (!record(value) || !exactKeys(value, LIBRARY_IMAGE_KEYS)) return undefined;
  if (!positive(value.id)) return undefined;
  if (typeof value.name !== "string" || runeLength(value.name) > 200) {
    return undefined;
  }
  if (!frozenText(value.file_name, 255, false)) return undefined;
  if (
    typeof value.mime_type !== "string" ||
    value.mime_type.length < 1 ||
    value.mime_type.length > 128
  ) {
    return undefined;
  }
  if (
    !positive(value.file_size) ||
    value.file_size > MAX_IMAGE_FILE_SIZE ||
    typeof value.enabled !== "boolean"
  ) {
    return undefined;
  }
  if (
    typeof value.description !== "string" ||
    value.description.length > 10000
  ) {
    return undefined;
  }
  if (
    !Array.isArray(value.tags) ||
    value.tags.length > 50 ||
    value.tags.some((tag) => typeof tag !== "string")
  ) {
    return undefined;
  }
  if (typeof value.category !== "string" || runeLength(value.category) > 200) {
    return undefined;
  }
  if (
    !positive(value.width) ||
    value.width > 10000 ||
    !positive(value.height) ||
    value.height > 10000
  ) {
    return undefined;
  }
  if (
    typeof value.created_at !== "string" ||
    value.created_at.length < 1 ||
    value.created_at.length > 64 ||
    typeof value.updated_at !== "string" ||
    value.updated_at.length < 1 ||
    value.updated_at.length > 64
  ) {
    return undefined;
  }
  if (
    !variantURL(value.thumb_160_url, "thumb_160") ||
    !variantURL(value.thumb_320_url, "thumb_320") ||
    !variantURL(value.thumb_url, "thumb_320") ||
    !variantURL(value.preview_url, "mobile_1080") ||
    !variantURL(value.mobile_1080_url, "mobile_1080") ||
    !variantURL(value.large_1440_url, "large_1440") ||
    !variantURL(value.original_url, "original")
  ) {
    return undefined;
  }
  return {
    id: value.id,
    name: value.name,
    fileName: value.file_name,
    mimeType: value.mime_type,
    fileSize: value.file_size,
    width: value.width,
    height: value.height,
    enabled: value.enabled,
    thumb160URL: value.thumb_160_url,
  };
}

const UPLOADED_IMAGE_KEYS: readonly string[] = [
  "id",
  "name",
  "file_name",
  "file_size",
  "mime_type",
  "width",
  "height",
  "description",
  "tags",
  "category",
  "created_at",
  "updated_at",
];

function parseUploadedImage(value: unknown): UploadedImage | undefined {
  if (!record(value) || !exactKeys(value, UPLOADED_IMAGE_KEYS))
    return undefined;
  if (!positive(value.id)) return undefined;
  if (typeof value.name !== "string" || runeLength(value.name) > 200) {
    return undefined;
  }
  if (!frozenText(value.file_name, 255, false)) return undefined;
  if (
    typeof value.mime_type !== "string" ||
    !IMAGE_MIME_TYPES.has(value.mime_type)
  ) {
    return undefined;
  }
  if (!positive(value.file_size) || value.file_size > MAX_IMAGE_FILE_SIZE) {
    return undefined;
  }
  if (
    !positive(value.width) ||
    value.width > 10000 ||
    !positive(value.height) ||
    value.height > 10000
  ) {
    return undefined;
  }
  if (
    typeof value.description !== "string" ||
    value.description.length > 10000
  ) {
    return undefined;
  }
  if (typeof value.tags !== "string" || value.tags.length > 10000)
    return undefined;
  if (typeof value.category !== "string" || runeLength(value.category) > 200) {
    return undefined;
  }
  if (
    typeof value.created_at !== "string" ||
    value.created_at.length < 1 ||
    value.created_at.length > 64 ||
    typeof value.updated_at !== "string" ||
    value.updated_at.length < 1 ||
    value.updated_at.length > 64
  ) {
    return undefined;
  }
  return { id: value.id, name: value.name };
}

function parseMiniProgramPage(
  data: unknown,
):
  | { items: MiniProgramRecord[]; total: number; limit: number; offset: number }
  | undefined {
  if (!record(data)) return undefined;
  if (
    !exactKeys(data, [
      "ok",
      "items",
      "miniprograms",
      "total",
      "limit",
      "offset",
      "local_only",
      "provider_call_executed",
      "real_external_call_executed",
    ])
  ) {
    return undefined;
  }
  if (data.ok !== true || !frozenLocalFlags(data)) return undefined;
  if (
    !Array.isArray(data.items) ||
    !Array.isArray(data.miniprograms) ||
    data.items.length !== data.miniprograms.length
  ) {
    return undefined;
  }
  const items = data.items.map(parseMiniProgram);
  const mirrored = data.miniprograms.map(parseMiniProgram);
  if (
    items.some((item) => item === undefined) ||
    mirrored.some((item) => item === undefined)
  ) {
    return undefined;
  }
  if (
    (items as MiniProgramRecord[]).some(
      (item, index) => item.id !== (mirrored[index] as MiniProgramRecord).id,
    )
  ) {
    return undefined;
  }
  if (
    !nonnegative(data.total) ||
    !positive(data.limit) ||
    !nonnegative(data.offset)
  ) {
    return undefined;
  }
  return {
    items: items as MiniProgramRecord[],
    total: data.total,
    limit: data.limit,
    offset: data.offset,
  };
}

function parseMiniProgramMutation(
  data: unknown,
):
  | {
      item: MiniProgramRecord;
      changed: boolean;
      thumbResolve?: ThumbnailResolution;
    }
  | undefined {
  if (!record(data)) return undefined;
  if (
    !exactKeys(data, [
      "ok",
      "item",
      "miniprogram",
      "item_id",
      "changed",
      "thumb_resolve",
      "local_only",
      "provider_call_executed",
      "real_external_call_executed",
    ])
  ) {
    return undefined;
  }
  if (data.ok !== true || !frozenLocalFlags(data)) return undefined;
  const item = parseMiniProgram(data.item);
  const mirrored = parseMiniProgram(data.miniprogram);
  if (!item || !mirrored || mirrored.id !== item.id) return undefined;
  if (data.item_id !== item.id || typeof data.changed !== "boolean") {
    return undefined;
  }
  if (data.thumb_resolve !== null) {
    const resolution = parseThumbnailResolution(data.thumb_resolve);
    if (!resolution) return undefined;
    return { item, changed: data.changed, thumbResolve: resolution };
  }
  return { item, changed: data.changed };
}

function parseMiniProgramDelete(data: unknown): number | undefined {
  if (!record(data)) return undefined;
  if (
    !exactKeys(data, [
      "ok",
      "id",
      "item_id",
      "deleted",
      "local_only",
      "provider_call_executed",
      "real_external_call_executed",
    ])
  ) {
    return undefined;
  }
  if (data.ok !== true || data.deleted !== true || !frozenLocalFlags(data)) {
    return undefined;
  }
  if (!positive(data.id) || data.item_id !== data.id) return undefined;
  return data.id;
}

function parseMiniProgramResolve(
  data: unknown,
):
  | {
      item: MiniProgramRecord;
      resolution: ThumbnailResolution;
      changed: boolean;
    }
  | undefined {
  if (!record(data)) return undefined;
  if (
    !exactKeys(data, [
      "ok",
      "item",
      "miniprogram",
      "resolution",
      "changed",
      "thumb_media_id",
      "local_only",
      "provider_call_executed",
      "real_external_call_executed",
    ])
  ) {
    return undefined;
  }
  if (data.ok !== true || !frozenLocalFlags(data)) return undefined;
  const item = parseMiniProgram(data.item);
  const mirrored = parseMiniProgram(data.miniprogram);
  if (!item || !mirrored || mirrored.id !== item.id) return undefined;
  const resolution = parseThumbnailResolution(data.resolution);
  if (!resolution || typeof data.changed !== "boolean") return undefined;
  if (
    typeof data.thumb_media_id !== "string" ||
    data.thumb_media_id.length > 255
  ) {
    return undefined;
  }
  return { item, resolution, changed: data.changed };
}

function parseLibraryImagePage(
  data: unknown,
):
  | {
      items: LibraryImage[];
      total: number;
      hasMore: boolean;
      nextOffset?: number;
    }
  | undefined {
  if (!record(data)) return undefined;
  if (
    !exactKeys(data, [
      "ok",
      "items",
      "total",
      "limit",
      "offset",
      "count",
      "has_more",
      "next_offset",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "storage_adapter_mode",
      "adapter_mode",
    ])
  ) {
    return undefined;
  }
  if (data.ok !== true || data.real_external_call_executed !== false) {
    return undefined;
  }
  if (
    data.source_status !== "next_media_library" ||
    data.route_owner !== "ai_crm_next" ||
    data.storage_adapter_mode !== "postgresql" ||
    data.adapter_mode !== "postgresql" ||
    typeof data.fallback_used !== "boolean"
  ) {
    return undefined;
  }
  if (!Array.isArray(data.items)) return undefined;
  const items = data.items.map(parseLibraryImage);
  if (items.some((item) => item === undefined)) return undefined;
  if (
    !nonnegative(data.total) ||
    !positive(data.limit) ||
    data.limit > 500 ||
    !nonnegative(data.offset) ||
    data.count !== items.length ||
    typeof data.has_more !== "boolean"
  ) {
    return undefined;
  }
  if (data.next_offset !== null && !nonnegative(data.next_offset)) {
    return undefined;
  }
  return {
    items: items as LibraryImage[],
    total: data.total,
    hasMore: data.has_more,
    ...(typeof data.next_offset === "number"
      ? { nextOffset: data.next_offset }
      : {}),
  };
}

function parseLibraryImageUpload(data: unknown): UploadedImage | undefined {
  if (!record(data)) return undefined;
  if (
    !exactKeys(data, [
      "ok",
      "item",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "storage_adapter_mode",
      "adapter_mode",
    ])
  ) {
    return undefined;
  }
  if (data.ok !== true || data.real_external_call_executed !== false) {
    return undefined;
  }
  if (
    data.source_status !== "local_upload" ||
    data.route_owner !== "ai_crm_next" ||
    data.storage_adapter_mode !== "postgresql" ||
    data.adapter_mode !== "postgresql" ||
    typeof data.fallback_used !== "boolean"
  ) {
    return undefined;
  }
  return parseUploadedImage(data.item);
}

function failure(status: number): MiniProgramFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400 || status === 422) return "invalid";
  return "unavailable";
}

function requestHeaders(
  csrfToken: string,
  idempotencyKey: string,
): RequestInit {
  return {
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrfToken, "Idempotency-Key": idempotencyKey },
  };
}

export function editorDraft(item?: MiniProgramRecord): MiniProgramDraft {
  return item
    ? {
        name: item.name,
        appID: item.appID,
        pagePath: item.pagePath,
        title: item.title,
        ...(item.thumbImageID !== undefined
          ? { thumbImageID: item.thumbImageID }
          : {}),
      }
    : { name: "", appID: "", pagePath: "", title: "" };
}

export function draftProblem(draft: MiniProgramDraft): string | undefined {
  if (!frozenText(draft.name.trim(), 200, false) || draft.name.trim() === "") {
    return "名称必填，且不超过 200 字。";
  }
  if (!frozenText(draft.appID.trim(), 120, false)) {
    return "AppID 必填，且不超过 120 字。";
  }
  if (!frozenText(draft.pagePath.trim(), 500, false)) {
    return "页面路径必填，且不超过 500 字。";
  }
  if (!frozenText(draft.title.trim(), 200, false)) {
    return "标题必填，且不超过 200 字。";
  }
  if (
    draft.thumbImageID !== undefined &&
    (!Number.isSafeInteger(draft.thumbImageID) || draft.thumbImageID < 1)
  ) {
    return "缩略图只能使用服务端返回的图片 ID。";
  }
  return undefined;
}

export async function loadMiniPrograms(
  transport: MiniProgramLibraryTransport,
  query: MiniProgramListQuery,
): Promise<MiniProgramListResult> {
  const search = query.search.trim();
  const params: ListLegacyMiniProgramsParams = {
    limit: MINIPROGRAM_PAGE_SIZE,
    offset:
      Number.isSafeInteger(query.offset) && query.offset > 0 ? query.offset : 0,
    enabled_only: query.enabledOnly,
    ...(search !== "" ? { q: search } : {}),
  };
  try {
    const response = await transport.list(params, {
      credentials: "same-origin",
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseMiniProgramPage(response.data);
    return page
      ? {
          status: "loaded",
          items: page.items,
          total: page.total,
          limit: page.limit,
          offset: page.offset,
        }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function saveMiniProgram(
  transport: MiniProgramLibraryTransport,
  existing: MiniProgramRecord | undefined,
  draft: MiniProgramDraft,
  csrfToken: string,
  idempotencyKey: string,
): Promise<MiniProgramMutationResult> {
  if (draftProblem(draft) !== undefined) return { status: "invalid" };
  const name = draft.name.trim();
  const appID = draft.appID.trim();
  const pagePath = draft.pagePath.trim();
  const title = draft.title.trim();
  try {
    if (existing) {
      const request: LegacyMiniProgramUpdateRequest = {
        name,
        appid: appID,
        pagepath: pagePath,
        title,
        thumb_image_id: draft.thumbImageID ?? null,
      };
      const response = await transport.update(
        existing.id,
        request,
        requestHeaders(csrfToken, idempotencyKey),
      );
      if (response.status !== 200) return { status: failure(response.status) };
      const parsed = parseMiniProgramMutation(response.data);
      return !parsed || parsed.item.id !== existing.id
        ? { status: "unavailable" }
        : {
            status: "saved",
            item: parsed.item,
            changed: parsed.changed,
            ...(parsed.thumbResolve
              ? { thumbResolve: parsed.thumbResolve }
              : {}),
          };
    }
    const request: LegacyMiniProgramCreateRequest = {
      name,
      appid: appID,
      pagepath: pagePath,
      title,
      ...(draft.thumbImageID !== undefined
        ? { thumb_image_id: draft.thumbImageID }
        : {}),
    };
    const response = await transport.create(
      request,
      requestHeaders(csrfToken, idempotencyKey),
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const parsed = parseMiniProgramMutation(response.data);
    return !parsed
      ? { status: "unavailable" }
      : {
          status: "saved",
          item: parsed.item,
          changed: parsed.changed,
          ...(parsed.thumbResolve ? { thumbResolve: parsed.thumbResolve } : {}),
        };
  } catch {
    return { status: "unavailable" };
  }
}

export async function setMiniProgramEnabled(
  transport: MiniProgramLibraryTransport,
  id: number,
  enabled: boolean,
  csrfToken: string,
  idempotencyKey: string,
): Promise<MiniProgramMutationResult> {
  try {
    const response = await transport.update(
      id,
      { enabled },
      requestHeaders(csrfToken, idempotencyKey),
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const parsed = parseMiniProgramMutation(response.data);
    return !parsed || parsed.item.id !== id || parsed.item.enabled !== enabled
      ? { status: "unavailable" }
      : {
          status: "saved",
          item: parsed.item,
          changed: parsed.changed,
          ...(parsed.thumbResolve ? { thumbResolve: parsed.thumbResolve } : {}),
        };
  } catch {
    return { status: "unavailable" };
  }
}

export async function deleteMiniProgram(
  transport: MiniProgramLibraryTransport,
  id: number,
  csrfToken: string,
  idempotencyKey: string,
): Promise<MiniProgramDeleteResult> {
  try {
    const response = await transport.remove(
      id,
      requestHeaders(csrfToken, idempotencyKey),
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const deletedID = parseMiniProgramDelete(response.data);
    return deletedID === id ? { status: "deleted" } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function resolveMiniProgramThumbnail(
  transport: MiniProgramLibraryTransport,
  id: number,
  csrfToken: string,
  idempotencyKey: string,
): Promise<MiniProgramResolveResult> {
  try {
    const response = await transport.resolve(
      id,
      requestHeaders(csrfToken, idempotencyKey),
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const parsed = parseMiniProgramResolve(response.data);
    return !parsed || parsed.item.id !== id
      ? { status: "unavailable" }
      : {
          status: "ok",
          item: parsed.item,
          resolution: parsed.resolution,
          changed: parsed.changed,
        };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadLibraryImages(
  transport: MiniProgramLibraryTransport,
  query: LibraryImageListQuery,
): Promise<LibraryImageListResult> {
  const search = query.search.trim();
  const params: GetLegacyImageListParams = {
    limit: String(IMAGE_PICKER_PAGE_SIZE),
    offset: String(
      Number.isSafeInteger(query.offset) && query.offset > 0 ? query.offset : 0,
    ),
    enabled_only: "true",
    ...(search !== "" ? { q: search } : {}),
  };
  try {
    const response = await transport.listImages(params, {
      credentials: "same-origin",
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseLibraryImagePage(response.data);
    return page
      ? {
          status: "loaded",
          items: page.items,
          total: page.total,
          hasMore: page.hasMore,
          ...(page.nextOffset !== undefined
            ? { nextOffset: page.nextOffset }
            : {}),
        }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export function imagePickerPreviousOffset(offset: number): number {
  if (!Number.isSafeInteger(offset) || offset <= 0) return 0;
  return Math.max(0, offset - IMAGE_PICKER_PAGE_SIZE);
}

export async function uploadLibraryImage(
  transport: MiniProgramLibraryTransport,
  file: Blob,
  fileName: string,
  csrfToken: string,
  idempotencyKey: string,
): Promise<LibraryImageUploadResult> {
  if (
    !IMAGE_MIME_TYPES.has(file.type) ||
    file.size < 1 ||
    file.size > MAX_IMAGE_FILE_SIZE
  ) {
    return { status: "invalid" };
  }
  const name = [...fileName.trim()].slice(0, 200).join("");
  if (name === "") return { status: "invalid" };
  try {
    const response = await transport.uploadImage(
      { image: file, name },
      requestHeaders(csrfToken, idempotencyKey),
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const image = parseLibraryImageUpload(response.data);
    return image ? { status: "uploaded", image } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
