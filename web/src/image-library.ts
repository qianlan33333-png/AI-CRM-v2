import {
  deleteLegacyImage,
  getLegacyImage,
  getLegacyImageFacets,
  getLegacyImageList,
  updateLegacyImage,
  uploadLegacyImage,
  type GetLegacyImageListParams,
  type LegacyImageMetadataUpdateRequest,
  type UploadLegacyImageBody,
} from "./api/generated/health";
import { readCSRFCookie } from "./auth";

export type ImageLibraryRole = "admin" | "ops" | "sales";

export const IMAGE_LIBRARY_PAGE_SIZE = 24;
export const MAX_IMAGE_FILE_SIZE = 10_485_760;
export const IMAGE_MIME_TYPES: ReadonlySet<string> = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
]);

export interface ImageItem {
  readonly id: number;
  readonly name: string;
  readonly fileName: string;
  readonly mimeType: string;
  readonly fileSize: number;
  readonly enabled: boolean;
  readonly description: string;
  readonly tags: readonly string[];
  readonly category: string;
  readonly width: number;
  readonly height: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface ImageDetail {
  readonly id: number;
  readonly name: string;
  readonly fileName: string;
  readonly mimeType: string;
  readonly fileSize: number;
  readonly enabled: boolean;
  readonly description: string;
  readonly tags: readonly string[];
  readonly category: string;
  readonly width: number;
  readonly height: number;
  readonly createdAt: string;
  readonly updatedAt: string;
  readonly previewURL: string;
  readonly originalURL: string;
}

export type ImagePreviewMode = "standard" | "original";

export interface ImageFacets {
  readonly categories: readonly string[];
  readonly tags: readonly string[];
}

// Both URLs come only from the exact, ID-bound detail projection. Do not
// construct a variant path in the UI: that would weaken the decoder's
// same-origin and variant-key checks.
export function imagePreviewURL(
  image: ImageDetail,
  mode: ImagePreviewMode,
): string {
  return mode === "original" ? image.originalURL : image.previewURL;
}

export interface UploadedImage {
  readonly id: number;
  readonly name: string;
  readonly fileName: string;
  readonly fileSize: number;
  readonly width: number;
  readonly height: number;
}

export interface ImageListQuery {
  readonly search: string;
  readonly category: string;
  readonly tags: string;
  readonly onlyUnlabeled: boolean;
  // The persisted enabled flag is local media-library state. Listing disabled
  // rows is opt-in so normal browse semantics remain unchanged.
  readonly includeDisabled?: boolean;
  readonly offset: number;
}

export interface ImageUploadMetadata {
  readonly name: string;
  readonly description: string;
  readonly tags: string;
  readonly category: string;
}

// This is deliberately metadata-only. Image enabled state, bytes, URLs, and
// variants belong to separate contracts and are never sent by this consumer.
export interface ImageMetadataDraft {
  readonly name: string;
  readonly description: string;
  readonly tags: string;
  readonly category: string;
}

async function generatedList(
  params: GetLegacyImageListParams,
  options: RequestInit,
) {
  return getLegacyImageList(params, options);
}
async function generatedDetail(imageID: string) {
  // The detail contract deliberately omits both optional compatibility
  // parameters: this UI never asks for a data URL or a selected variant.
  return getLegacyImage(imageID, undefined, { credentials: "same-origin" });
}
async function generatedFacets(options: RequestInit) {
  return getLegacyImageFacets(options);
}
async function generatedUpload(
  body: UploadLegacyImageBody,
  options: RequestInit,
) {
  return uploadLegacyImage(body, options);
}
async function generatedUpdate(
  imageID: string,
  body: LegacyImageMetadataUpdateRequest,
  options: RequestInit,
) {
  return updateLegacyImage(imageID, body, options);
}
async function generatedRemove(imageID: string, options: RequestInit) {
  // Never send force: a local delete must not attempt to bypass reference
  // checks, and this endpoint deliberately rejects a request body.
  return deleteLegacyImage(imageID, undefined, options);
}

export interface ImageLibraryTransport {
  readonly list: typeof generatedList;
  readonly detail: typeof generatedDetail;
  readonly facets: typeof generatedFacets;
  readonly upload: typeof generatedUpload;
  readonly update: typeof generatedUpdate;
  readonly remove: typeof generatedRemove;
}

export const generatedImageLibraryTransport: ImageLibraryTransport = {
  list: generatedList,
  detail: generatedDetail,
  facets: generatedFacets,
  upload: generatedUpload,
  update: generatedUpdate,
  remove: generatedRemove,
};

export type ImageLibraryFailure =
  | "unauthenticated"
  | "forbidden"
  | "conflict"
  | "invalid"
  | "unavailable";

export type ImageListResult =
  | {
      readonly status: "loaded";
      readonly items: readonly ImageItem[];
      readonly total: number;
      readonly limit: number;
      readonly offset: number;
      readonly count: number;
      readonly hasMore: boolean;
      readonly nextOffset?: number;
    }
  | { readonly status: ImageLibraryFailure };
export type ImageFacetsResult =
  | { readonly status: "loaded"; readonly facets: ImageFacets }
  | { readonly status: ImageLibraryFailure };
export type ImageDetailResult =
  | { readonly status: "loaded"; readonly image: ImageDetail }
  | { readonly status: ImageLibraryFailure };
export type ImageUploadResult =
  | { readonly status: "uploaded"; readonly image: UploadedImage }
  | { readonly status: "csrf_missing" }
  | { readonly status: ImageLibraryFailure };
export type ImageMetadataUpdateResult =
  | { readonly status: "saved"; readonly image: ImageDetail }
  | { readonly status: "csrf_missing" }
  | { readonly status: ImageLibraryFailure };
export type ImageEnabledUpdateResult =
  | { readonly status: "saved"; readonly image: ImageDetail }
  | { readonly status: "csrf_missing" }
  | { readonly status: ImageLibraryFailure };
export interface ImageDeleteReferenceCounts {
  readonly miniprograms: number;
  readonly campaignSteps: number;
  readonly groupInvites: number;
  readonly automationAgents: number;
  readonly channels: number;
  readonly importPreflights: number;
}
export type ImageDeleteResult =
  | { readonly status: "deleted"; readonly id: number }
  | { readonly status: "referenced"; readonly references: ImageDeleteReferenceCounts }
  | { readonly status: "csrf_missing" }
  | { readonly status: ImageLibraryFailure };

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
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
function boundedSafeText(value: unknown, limit: number): value is string {
  return (
    typeof value === "string" &&
    runeLength(value) <= limit &&
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
const TIMESTAMP_PATTERN =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

function timestampText(value: unknown): value is string {
  if (typeof value !== "string" || value.length > 64) return false;
  const match = TIMESTAMP_PATTERN.exec(value);
  if (!match) return false;
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  if (month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59) {
    return false;
  }
  // RFC3339 allows a leap-second 60; the backend only emits Go RFC3339
  // timestamps, which never carry it, so fail closed here.
  if (second > 59) return false;
  const daysInMonth = new Date(
    Date.UTC(Number(match[1]), month, 0),
  ).getUTCDate();
  if (day > daysInMonth) return false;
  const zone = match[7];
  if (zone !== "Z") {
    const offsetHour = Number(zone.slice(1, 3));
    const offsetMinute = Number(zone.slice(4, 6));
    if (offsetHour > 23 || offsetMinute > 59) return false;
  }
  return Number.isFinite(Date.parse(value));
}

function frozenEnvelopeFlags(
  value: Record<string, unknown>,
  sourceStatus:
    | "next_media_library"
    | "local_upload"
    | "local_repository_write"
    | "local_delete",
): boolean {
  return (
    value.source_status === sourceStatus &&
    value.route_owner === "ai_crm_next" &&
    value.fallback_used === false &&
    value.real_external_call_executed === false &&
    value.storage_adapter_mode === "postgresql" &&
    value.adapter_mode === "postgresql"
  );
}

const IMAGE_VARIANT_KEYS = [
  "thumb_160",
  "thumb_320",
  "mobile_1080",
  "large_1440",
  "original",
] as const;
type ImageVariantKey = (typeof IMAGE_VARIANT_KEYS)[number];

function expectedVariantURL(id: number, variant: ImageVariantKey): string {
  return `/api/admin/image-library/${id}/variants/${variant}`;
}

const IMAGE_ITEM_KEYS: readonly string[] = [
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

export function parseImageItem(
  value: unknown,
  allowDisabled = false,
): ImageItem | undefined {
  if (!record(value) || !exactKeys(value, IMAGE_ITEM_KEYS)) return undefined;
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
  if (typeof value.enabled !== "boolean" || (!allowDisabled && !value.enabled)) {
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
    value.tags.some(
      (tag) => typeof tag !== "string" || runeLength(tag) > 64,
    )
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
  if (!timestampText(value.created_at) || !timestampText(value.updated_at)) {
    return undefined;
  }
  const id = value.id;
  const expectations: readonly [unknown, string][] = [
    [value.thumb_160_url, expectedVariantURL(id, "thumb_160")],
    [value.thumb_320_url, expectedVariantURL(id, "thumb_320")],
    [value.thumb_url, expectedVariantURL(id, "thumb_320")],
    [value.preview_url, expectedVariantURL(id, "mobile_1080")],
    [value.mobile_1080_url, expectedVariantURL(id, "mobile_1080")],
    [value.large_1440_url, expectedVariantURL(id, "large_1440")],
    [value.original_url, expectedVariantURL(id, "original")],
  ];
  if (expectations.some(([actual, expected]) => actual !== expected)) {
    return undefined;
  }
  return {
    id,
    name: value.name,
    fileName: value.file_name,
    mimeType: value.mime_type,
    fileSize: value.file_size,
    enabled: value.enabled,
    description: value.description,
    tags: value.tags as string[],
    category: value.category,
    width: value.width,
    height: value.height,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

const IMAGE_DETAIL_ITEM_KEYS: readonly string[] = [
  "id",
  "name",
  "file_name",
  "mime_type",
  "file_size",
  "description",
  "category",
  "width",
  "height",
  "created_at",
  "updated_at",
  "content_type",
  "tags",
  "enabled",
  "source",
  "source_url",
  "thumb_media_id",
  "thumb_media_id_expires_at",
  "ai_metadata",
  "thumb_160_url",
  "thumb_320_url",
  "thumb_url",
  "preview_url",
  "mobile_1080_url",
  "large_1440_url",
  "original_url",
];

// This view always omits include_data and variant, so their conditional fields
// must not be accepted from a purported success response.
export function parseImageDetail(value: unknown): ImageDetail | undefined {
  if (!record(value) || !exactKeys(value, IMAGE_DETAIL_ITEM_KEYS)) {
    return undefined;
  }
  if (!positive(value.id)) return undefined;
  if (!boundedSafeText(value.name, 200)) {
    return undefined;
  }
  if (!frozenText(value.file_name, 255, false)) return undefined;
  if (
    typeof value.mime_type !== "string" ||
    !IMAGE_MIME_TYPES.has(value.mime_type) ||
    value.content_type !== value.mime_type
  ) {
    return undefined;
  }
  if (!positive(value.file_size) || value.file_size > MAX_IMAGE_FILE_SIZE) {
    return undefined;
  }
  if (typeof value.enabled !== "boolean") return undefined;
  if (
    !boundedSafeText(value.description, 10000) ||
    !boundedSafeText(value.category, 200)
  ) {
    return undefined;
  }
  if (
    !Array.isArray(value.tags) ||
    value.tags.length > 50 ||
    value.tags.some(
      (tag) => !boundedSafeText(tag, 64),
    )
  ) {
    return undefined;
  }
  if (
    !positive(value.width) ||
    value.width > 10000 ||
    !positive(value.height) ||
    value.height > 10000 ||
    !timestampText(value.created_at) ||
    !timestampText(value.updated_at)
  ) {
    return undefined;
  }
  if (
    value.source !== "upload" ||
    value.source_url !== "" ||
    value.thumb_media_id !== "" ||
    value.thumb_media_id_expires_at !== "" ||
    !record(value.ai_metadata) ||
    !exactKeys(value.ai_metadata, [])
  ) {
    return undefined;
  }
  const id = value.id;
  const expectations: readonly [unknown, string][] = [
    [value.thumb_160_url, expectedVariantURL(id, "thumb_160")],
    [value.thumb_320_url, expectedVariantURL(id, "thumb_320")],
    [value.thumb_url, expectedVariantURL(id, "thumb_320")],
    [value.preview_url, expectedVariantURL(id, "mobile_1080")],
    [value.mobile_1080_url, expectedVariantURL(id, "mobile_1080")],
    [value.large_1440_url, expectedVariantURL(id, "large_1440")],
    [value.original_url, expectedVariantURL(id, "original")],
  ];
  if (expectations.some(([actual, expected]) => actual !== expected)) {
    return undefined;
  }
  return {
    id,
    name: value.name,
    fileName: value.file_name,
    mimeType: value.mime_type,
    fileSize: value.file_size,
    enabled: value.enabled,
    description: value.description,
    tags: value.tags as string[],
    category: value.category,
    width: value.width,
    height: value.height,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
    previewURL: value.preview_url as string,
    originalURL: value.original_url as string,
  };
}

const IMAGE_LIST_KEYS: readonly string[] = [
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
];

function parseImageListPage(
  data: unknown,
  allowDisabled: boolean,
):
  | {
      items: ImageItem[];
      total: number;
      limit: number;
      offset: number;
      count: number;
      hasMore: boolean;
      nextOffset?: number;
    }
  | undefined {
  if (!record(data) || !exactKeys(data, IMAGE_LIST_KEYS)) return undefined;
  if (data.ok !== true) return undefined;
  if (!frozenEnvelopeFlags(data, "next_media_library")) return undefined;
  if (!Array.isArray(data.items)) return undefined;
  const items = data.items.map((item) => parseImageItem(item, allowDisabled));
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
  // Cross-field consistency mirroring cmd/aicrm/legacy_image_list_api.go:
  // count is bounded by limit and total; an empty page is only legal at or
  // past the end; a nonempty page must fit inside the total window.
  const count = items.length;
  const total = data.total;
  const offset = data.offset;
  if (count > data.limit || count > total) return undefined;
  if (count === 0 && offset < total) return undefined;
  if (count > 0 && offset + count > total) return undefined;
  // has_more is exactly `offset < total && count < total - offset`; when true
  // next_offset is offset+count, when false next_offset is null.
  const expectedHasMore = offset < total && offset + count < total;
  if (data.has_more !== expectedHasMore) return undefined;
  if (data.has_more) {
    if (count === 0 || data.next_offset !== offset + count) return undefined;
  } else if (data.next_offset !== null) {
    return undefined;
  }
  return {
    items: items as ImageItem[],
    total,
    limit: data.limit,
    offset,
    count,
    hasMore: data.has_more,
    ...(typeof data.next_offset === "number"
      ? { nextOffset: data.next_offset }
      : {}),
  };
}

const IMAGE_FACETS_KEYS: readonly string[] = [
  "ok",
  "categories",
  "tags",
  "source_status",
  "route_owner",
  "fallback_used",
  "real_external_call_executed",
  "storage_adapter_mode",
  "adapter_mode",
];

const IMAGE_DETAIL_SUCCESS_KEYS: readonly string[] = [
  "ok",
  "item",
  "source_status",
  "route_owner",
  "fallback_used",
  "real_external_call_executed",
  "storage_adapter_mode",
  "adapter_mode",
];
const IMAGE_DELETE_SUCCESS_KEYS: readonly string[] = [
  "ok",
  "deleted",
  "hard_deleted",
  "id",
  "references_cleared",
  "source_status",
  "route_owner",
  "fallback_used",
  "real_external_call_executed",
  "storage_adapter_mode",
  "adapter_mode",
];
const IMAGE_DELETE_CONFLICT_KEYS: readonly string[] = [
  "ok",
  "error",
  "references",
  "source_status",
  "route_owner",
  "fallback_used",
  "real_external_call_executed",
  "storage_adapter_mode",
  "adapter_mode",
];
const IMAGE_DELETE_REFERENCE_KEYS = [
  "miniprograms",
  "campaign_steps",
  "group_invites",
  "automation_agents",
  "channels",
  "import_preflights",
] as const;

function parseDeleteReferenceIDs(value: unknown): number[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const ids = value.map((item) => {
    if (!record(item) || !exactKeys(item, ["id"]) || !positive(item.id)) {
      return undefined;
    }
    return item.id;
  });
  if (ids.some((id) => id === undefined)) return undefined;
  const values = ids as number[];
  return new Set(values).size === values.length ? values : undefined;
}

function parseImageDeleteSuccess(data: unknown): number | undefined {
  if (!record(data) || !exactKeys(data, IMAGE_DELETE_SUCCESS_KEYS)) {
    return undefined;
  }
  if (
    data.ok !== true ||
    data.deleted !== true ||
    data.hard_deleted !== true ||
    !positive(data.id) ||
    !frozenEnvelopeFlags(data, "local_delete") ||
    !record(data.references_cleared) ||
    !exactKeys(data.references_cleared, [
      "miniprograms_cleared",
      "campaign_steps_cleared",
    ]) ||
    data.references_cleared.miniprograms_cleared !== 0 ||
    data.references_cleared.campaign_steps_cleared !== 0
  ) {
    return undefined;
  }
  return data.id;
}

function parseImageDeleteConflict(
  data: unknown,
): ImageDeleteReferenceCounts | undefined {
  if (!record(data) || !exactKeys(data, IMAGE_DELETE_CONFLICT_KEYS)) {
    return undefined;
  }
  if (
    data.ok !== false ||
    data.error !== "image_has_references" ||
    !frozenEnvelopeFlags(data, "local_delete") ||
    !record(data.references) ||
    !exactKeys(data.references, IMAGE_DELETE_REFERENCE_KEYS)
  ) {
    return undefined;
  }
  const referenceProjection = data.references;
  const miniprograms = parseDeleteReferenceIDs(referenceProjection.miniprograms);
  const campaignSteps = parseDeleteReferenceIDs(referenceProjection.campaign_steps);
  const groupInvites = parseDeleteReferenceIDs(referenceProjection.group_invites);
  const automationAgents = parseDeleteReferenceIDs(
    referenceProjection.automation_agents,
  );
  const channels = parseDeleteReferenceIDs(referenceProjection.channels);
  const importPreflights = parseDeleteReferenceIDs(
    referenceProjection.import_preflights,
  );
  if (
    miniprograms === undefined ||
    campaignSteps === undefined ||
    groupInvites === undefined ||
    automationAgents === undefined ||
    channels === undefined ||
    importPreflights === undefined
  ) {
    return undefined;
  }
  return {
    miniprograms: miniprograms.length,
    campaignSteps: campaignSteps.length,
    groupInvites: groupInvites.length,
    automationAgents: automationAgents.length,
    channels: channels.length,
    importPreflights: importPreflights.length,
  };
}

function parseImageDetailResponse(data: unknown): ImageDetail | undefined {
  if (!record(data) || !exactKeys(data, IMAGE_DETAIL_SUCCESS_KEYS)) {
    return undefined;
  }
  if (data.ok !== true || !frozenEnvelopeFlags(data, "next_media_library")) {
    return undefined;
  }
  return parseImageDetail(data.item);
}

function parseImageFacets(data: unknown): ImageFacets | undefined {
  if (!record(data) || !exactKeys(data, IMAGE_FACETS_KEYS)) return undefined;
  if (data.ok !== true) return undefined;
  if (!frozenEnvelopeFlags(data, "next_media_library")) return undefined;
  if (
    !Array.isArray(data.categories) ||
    data.categories.some((category) => typeof category !== "string")
  ) {
    return undefined;
  }
  if (
    !Array.isArray(data.tags) ||
    data.tags.some(
      (tag) => typeof tag !== "string" || runeLength(tag) > 64,
    )
  ) {
    return undefined;
  }
  return {
    categories: data.categories as string[],
    tags: data.tags as string[],
  };
}

const UPLOAD_SUCCESS_KEYS: readonly string[] = [
  "ok",
  "item",
  "source_status",
  "route_owner",
  "fallback_used",
  "real_external_call_executed",
  "storage_adapter_mode",
  "adapter_mode",
];
const UPLOAD_ITEM_KEYS: readonly string[] = [
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
  if (!record(value) || !exactKeys(value, UPLOAD_ITEM_KEYS)) return undefined;
  if (!positive(value.id)) return undefined;
  if (typeof value.name !== "string") return undefined;
  if (!frozenText(value.file_name, 255, false)) return undefined;
  if (!positive(value.file_size) || value.file_size > MAX_IMAGE_FILE_SIZE) {
    return undefined;
  }
  if (
    typeof value.mime_type !== "string" ||
    !IMAGE_MIME_TYPES.has(value.mime_type)
  ) {
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
    typeof value.tags !== "string" ||
    typeof value.category !== "string"
  ) {
    return undefined;
  }
  if (!timestampText(value.created_at) || !timestampText(value.updated_at)) {
    return undefined;
  }
  return {
    id: value.id,
    name: value.name,
    fileName: value.file_name,
    fileSize: value.file_size,
    width: value.width,
    height: value.height,
  };
}

function parseImageUpload(data: unknown): UploadedImage | undefined {
  if (!record(data) || !exactKeys(data, UPLOAD_SUCCESS_KEYS)) return undefined;
  if (data.ok !== true) return undefined;
  if (!frozenEnvelopeFlags(data, "local_upload")) return undefined;
  return parseUploadedImage(data.item);
}

function parseImageMetadataUpdate(data: unknown): ImageDetail | undefined {
  if (!record(data) || !exactKeys(data, IMAGE_DETAIL_SUCCESS_KEYS)) {
    return undefined;
  }
  if (
    data.ok !== true ||
    !frozenEnvelopeFlags(data, "local_repository_write")
  ) {
    return undefined;
  }
  return parseImageDetail(data.item);
}

function failure(status: number): ImageLibraryFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 400 || status === 422) return "invalid";
  if (status === 409) return "conflict";
  return "unavailable";
}

export function normalizeTagsInput(input: string): string {
  const seen = new Set<string>();
  const tags: string[] = [];
  for (const raw of input.split(",")) {
    const tag = raw.trim();
    if (tag === "" || seen.has(tag)) continue;
    seen.add(tag);
    tags.push(tag);
  }
  return tags.join(",");
}

export function previousPageOffset(offset: number): number {
  if (!Number.isSafeInteger(offset) || offset <= 0) return 0;
  return Math.max(0, offset - IMAGE_LIBRARY_PAGE_SIZE);
}

export function nextPageOffset(page: {
  readonly offset: number;
  readonly count: number;
  readonly hasMore: boolean;
  readonly nextOffset?: number;
}): number | undefined {
  if (!page.hasMore) return undefined;
  return page.nextOffset ?? page.offset + page.count;
}

export function firstPageQuery(query: ImageListQuery): ImageListQuery {
  return { ...query, offset: 0 };
}

export function formatFileSize(bytes: number): string {
  if (!Number.isSafeInteger(bytes) || bytes < 0) return "未知";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

export function uploadIdempotencyKey(random: () => number = Math.random): string {
  const time = Date.now().toString(36);
  const entropy = random().toString(36).slice(2, 14).padEnd(12, "0");
  return `image-upload-${time}-${entropy}`.slice(0, 128);
}

export function imageDeleteIdempotencyKey(
  random: () => number = Math.random,
): string {
  const time = Date.now().toString(36);
  const entropy = random().toString(36).slice(2, 14).padEnd(12, "0");
  return `image-delete-${time}-${entropy}`.slice(0, 128);
}

export async function loadImages(
  transport: ImageLibraryTransport,
  query: ImageListQuery,
): Promise<ImageListResult> {
  const search = query.search.trim();
  const category = query.category.trim();
  const tags = normalizeTagsInput(query.tags);
  const params: GetLegacyImageListParams = {
    limit: String(IMAGE_LIBRARY_PAGE_SIZE),
    offset: String(
      Number.isSafeInteger(query.offset) && query.offset > 0 ? query.offset : 0,
    ),
    enabled_only: query.includeDisabled ? "false" : "true",
    ...(search !== "" ? { q: search } : {}),
    ...(category !== "" ? { category } : {}),
    ...(tags !== "" ? { tags } : {}),
    ...(query.onlyUnlabeled ? { only_unlabeled: "true" as const } : {}),
  };
  try {
    const response = await transport.list(params, {
      credentials: "same-origin",
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseImageListPage(response.data, query.includeDisabled === true);
    return page
      ? {
          status: "loaded",
          items: page.items,
          total: page.total,
          limit: page.limit,
          offset: page.offset,
          count: page.count,
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

export async function loadImageDetail(
  transport: ImageLibraryTransport,
  imageID: number,
): Promise<ImageDetailResult> {
  if (!positive(imageID)) return { status: "invalid" };
  try {
    // Deliberately no query parameters, request body, CSRF header, retry, or
    // mutation: the generated adapter pins this to a same-origin GET.
    const response = await transport.detail(String(imageID));
    if (response.status !== 200) return { status: failure(response.status) };
    const image = parseImageDetailResponse(response.data);
    if (!image || image.id !== imageID) return { status: "unavailable" };
    return { status: "loaded", image };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadFacets(
  transport: ImageLibraryTransport,
): Promise<ImageFacetsResult> {
  try {
    const response = await transport.facets({ credentials: "same-origin" });
    if (response.status !== 200) return { status: failure(response.status) };
    const facets = parseImageFacets(response.data);
    return facets
      ? { status: "loaded", facets }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export function uploadMetadataProblem(
  metadata: ImageUploadMetadata,
): string | undefined {
  if (runeLength(metadata.name.trim()) > 200) {
    return "名称不能超过 200 字。";
  }
  if (metadata.description.length > 10000) {
    return "描述不能超过 10000 字。";
  }
  if (runeLength(metadata.category.trim()) > 200) {
    return "分类不能超过 200 字。";
  }
  if (normalizeTagsInput(metadata.tags).length > 10000) {
    return "标签总长不能超过 10000 字。";
  }
  return undefined;
}

function metadataText(value: string, maximum: number): string | undefined {
  const normalized = value.trim();
  return !normalized.includes("\x00") && runeLength(normalized) <= maximum
    ? normalized
    : undefined;
}

export function imageMetadataRequest(
  draft: ImageMetadataDraft,
): LegacyImageMetadataUpdateRequest | undefined {
  const name = metadataText(draft.name, 200);
  const description = metadataText(draft.description, 10_000);
  const category = metadataText(draft.category, 200);
  if (name === undefined || description === undefined || category === undefined) {
    return undefined;
  }
  const tags: string[] = [];
  const seen = new Set<string>();
  for (const raw of draft.tags.split(",")) {
    const tag = raw.trim();
    if (tag === "") continue;
    if (tag.includes("\x00") || runeLength(tag) > 64 || seen.has(tag)) {
      if (tag.includes("\x00") || runeLength(tag) > 64) return undefined;
      continue;
    }
    seen.add(tag);
    tags.push(tag);
    if (tags.length > 50) return undefined;
  }
  return { name, description, tags, category };
}

function imageMetadataMatches(
  image: ImageDetail,
  request: LegacyImageMetadataUpdateRequest,
): boolean {
  return (
    (request.name === undefined || image.name === request.name) &&
    (request.description === undefined || image.description === request.description) &&
    (request.category === undefined || image.category === request.category) &&
    (request.enabled === undefined || image.enabled === request.enabled) &&
    (request.tags === undefined ||
      (image.tags.length === request.tags.length &&
        image.tags.every((tag, index) => tag === request.tags?.[index])))
  );
}

async function updateLocalImage(
  transport: ImageLibraryTransport,
  cookieHeader: string,
  imageID: number,
  request: LegacyImageMetadataUpdateRequest,
): Promise<ImageEnabledUpdateResult> {
  if (!positive(imageID)) return { status: "invalid" };
  let csrfToken: string | undefined;
  try {
    csrfToken = readCSRFCookie(cookieHeader);
  } catch {
    csrfToken = undefined;
  }
  if (!csrfToken) return { status: "csrf_missing" };
  try {
    const response = await transport.update(String(imageID), request, {
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrfToken },
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const image = parseImageMetadataUpdate(response.data);
    return image && image.id === imageID && imageMetadataMatches(image, request)
      ? { status: "saved", image }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function updateImageMetadata(
  transport: ImageLibraryTransport,
  cookieHeader: string,
  imageID: number,
  draft: ImageMetadataDraft,
): Promise<ImageMetadataUpdateResult> {
  if (!positive(imageID)) return { status: "invalid" };
  const request = imageMetadataRequest(draft);
  if (!request) return { status: "invalid" };
  return updateLocalImage(transport, cookieHeader, imageID, request);
}

export async function setImageEnabled(
  transport: ImageLibraryTransport,
  cookieHeader: string,
  imageID: number,
  enabled: boolean,
): Promise<ImageEnabledUpdateResult> {
  return updateLocalImage(transport, cookieHeader, imageID, { enabled });
}

export async function deleteImage(
  transport: ImageLibraryTransport,
  cookieHeader: string,
  imageID: number,
  idempotencyKey: string,
): Promise<ImageDeleteResult> {
  if (
    !positive(imageID) ||
    idempotencyKey.length < 16 ||
    idempotencyKey.length > 128 ||
    idempotencyKey.trim() !== idempotencyKey
  ) {
    return { status: "invalid" };
  }
  let csrfToken: string | undefined;
  try {
    csrfToken = readCSRFCookie(cookieHeader);
  } catch {
    csrfToken = undefined;
  }
  if (!csrfToken) return { status: "csrf_missing" };
  try {
    const response = await transport.remove(String(imageID), {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": csrfToken,
        "Idempotency-Key": idempotencyKey,
      },
    });
    if (response.status === 200) {
      const deletedID = parseImageDeleteSuccess(response.data);
      return deletedID === imageID
        ? { status: "deleted", id: deletedID }
        : { status: "unavailable" };
    }
    if (response.status === 409) {
      const references = parseImageDeleteConflict(response.data);
      return references
        ? { status: "referenced", references }
        : { status: "unavailable" };
    }
    return { status: failure(response.status) };
  } catch {
    return { status: "unavailable" };
  }
}

export function uploadFileProblem(file: Blob): string | undefined {
  if (!IMAGE_MIME_TYPES.has(file.type)) {
    return "只支持 PNG、JPEG 或 GIF 图片。";
  }
  if (file.size < 1) return "图片文件不能为空。";
  if (file.size > MAX_IMAGE_FILE_SIZE) return "图片不能超过 10 MiB。";
  return undefined;
}

export async function uploadImage(
  transport: ImageLibraryTransport,
  cookieHeader: string,
  file: Blob,
  metadata: ImageUploadMetadata,
  idempotencyKey: string,
): Promise<ImageUploadResult> {
  if (uploadFileProblem(file) !== undefined) return { status: "invalid" };
  if (uploadMetadataProblem(metadata) !== undefined) {
    return { status: "invalid" };
  }
  if (idempotencyKey.length < 16 || idempotencyKey.length > 128) {
    return { status: "invalid" };
  }
  let csrfToken: string | undefined;
  try {
    csrfToken = readCSRFCookie(cookieHeader);
  } catch {
    csrfToken = undefined;
  }
  if (!csrfToken) return { status: "csrf_missing" };

  const name = metadata.name.trim();
  const description = metadata.description;
  const tags = normalizeTagsInput(metadata.tags);
  const category = metadata.category.trim();
  const body: UploadLegacyImageBody = {
    image: file,
    ...(name !== "" ? { name } : {}),
    ...(description !== "" ? { description } : {}),
    ...(tags !== "" ? { tags } : {}),
    ...(category !== "" ? { category } : {}),
  };
  try {
    const response = await transport.upload(body, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": csrfToken,
        "Idempotency-Key": idempotencyKey,
      },
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const image = parseImageUpload(response.data);
    return image ? { status: "uploaded", image } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
