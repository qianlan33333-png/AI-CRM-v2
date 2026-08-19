import {
  archiveLegacyWecomTagGroup,
  createLegacyWecomTagGroup,
  listLegacyWecomTags,
  updateLegacyWecomTagGroupPatch,
  updateLegacyWecomTagPatch,
  type LegacyTagArchiveRequest,
  type LegacyTagGroupCreateRequest,
} from "./api/generated/health";

export type WecomTagsRole = "admin" | "ops" | "sales";
export const WECOM_TAGS_PAGE_SIZE = 20;
const WECOM_TAG_LIMIT = 1000;
const MAX_WECOM_TAG_SORT_ORDER = 2_147_483_647;

export interface WecomTag {
  readonly id: number;
  readonly groupID: number;
  readonly groupName: string;
  readonly name: string;
  readonly sortOrder: number;
}

export interface WecomTagGroup {
  readonly id: number;
  readonly name: string;
  readonly sortOrder: number;
  readonly tags: readonly WecomTag[];
}

export interface WecomTagCatalog {
  readonly totalTags: number;
  readonly tagLimit: number;
  readonly snapshotAt: string;
  readonly groups: readonly WecomTagGroup[];
  readonly tags: readonly WecomTag[];
}

export type WecomTagsFailure =
  "unauthenticated" | "forbidden" | "unavailable" | "invalid";

export type WecomTagCatalogResult =
  | { readonly status: "loaded"; readonly catalog: WecomTagCatalog }
  | { readonly status: WecomTagsFailure };

export type WecomTagGroupCreateResult =
  | {
      readonly status: "created";
      readonly group: Omit<WecomTagGroup, "tags">;
      readonly tag: WecomTag;
    }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" }
  | { readonly status: "unknown" };

export type WecomTagRenameResult =
  | { readonly status: "confirmed"; readonly tag: WecomTag }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" }
  | { readonly status: "unknown" };

export type WecomTagGroupRenameResult =
  | {
      readonly status: "confirmed";
      readonly group: Omit<WecomTagGroup, "tags">;
    }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" }
  | { readonly status: "unknown" };

export type WecomTagGroupArchiveResult =
  | {
      readonly status: "archived";
      readonly group: Omit<WecomTagGroup, "tags">;
    }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" }
  | { readonly status: "unknown" };

async function generatedRead(options?: RequestInit) {
  return listLegacyWecomTags({ credentials: "same-origin", ...options });
}

async function generatedCreate(
  request: LegacyTagGroupCreateRequest,
  options?: RequestInit,
) {
  return createLegacyWecomTagGroup(request, {
    credentials: "same-origin",
    ...options,
  });
}

async function generatedRename(
  tagID: number,
  request: { readonly tag_name: string },
  options?: RequestInit,
) {
  const response = await updateLegacyWecomTagPatch(tagID, request, {
    credentials: "same-origin",
    ...options,
  });
  return { status: response.status, data: response.data };
}

async function generatedGroupRename(
  groupID: number,
  request: { readonly group_name: string },
  options?: RequestInit,
) {
  const response = await updateLegacyWecomTagGroupPatch(groupID, request, {
    credentials: "same-origin",
    ...options,
  });
  return { status: response.status, data: response.data };
}

async function generatedGroupArchive(
  groupID: number,
  request: LegacyTagArchiveRequest,
  options?: RequestInit,
) {
  const response = await archiveLegacyWecomTagGroup(groupID, request, {
    credentials: "same-origin",
    ...options,
  });
  return { status: response.status, data: response.data };
}

export interface WecomTagsTransport {
  readonly read: typeof generatedRead;
  readonly create: typeof generatedCreate;
  readonly rename: typeof generatedRename;
  readonly renameGroup: typeof generatedGroupRename;
  readonly archiveGroup: typeof generatedGroupArchive;
}

export const generatedWecomTagsTransport: WecomTagsTransport = {
  read: generatedRead,
  create: generatedCreate,
  rename: generatedRename,
  renameGroup: generatedGroupRename,
  archiveGroup: generatedGroupArchive,
};

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function exact(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  const actual = Object.keys(value);
  return (
    actual.length === keys.length && actual.every((key) => keys.includes(key))
  );
}

function nonnegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function positiveInteger(value: unknown): value is number {
  return nonnegativeInteger(value) && value > 0;
}

function validSortOrder(value: unknown): value is number {
  return nonnegativeInteger(value) && value <= MAX_WECOM_TAG_SORT_ORDER;
}

function frozenText(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    [...value].length <= 200 &&
    value.trim() === value &&
    !value.includes("\x00")
  );
}

function frozenTimestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const parts = value.match(
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$/,
  );
  if (parts === null) return false;
  const [year, month, day, hour, minute, second] = parts
    .slice(1, 7)
    .map(Number);
  const offset = parts[7];
  if (
    hour > 23 ||
    minute > 59 ||
    second > 59 ||
    (offset !== "Z" &&
      (Number(offset.slice(1, 3)) > 23 || Number(offset.slice(4, 6)) > 59))
  ) {
    return false;
  }
  const calendar = new Date(0);
  calendar.setUTCFullYear(year, month - 1, day);
  calendar.setUTCHours(hour, minute, second, 0);
  return (
    calendar.getUTCFullYear() === year &&
    calendar.getUTCMonth() === month - 1 &&
    calendar.getUTCDate() === day &&
    calendar.getUTCHours() === hour &&
    calendar.getUTCMinutes() === minute &&
    calendar.getUTCSeconds() === second &&
    Number.isFinite(Date.parse(value))
  );
}

function parseTag(value: unknown): WecomTag | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "tag_id",
      "id",
      "group_id",
      "group_name",
      "tag_name",
      "name",
      "sort_order",
    ]) ||
    !positiveInteger(value.tag_id) ||
    value.id !== value.tag_id ||
    !positiveInteger(value.group_id) ||
    !frozenText(value.group_name) ||
    !frozenText(value.tag_name) ||
    value.name !== value.tag_name ||
    !validSortOrder(value.sort_order)
  ) {
    return undefined;
  }
  return {
    id: value.tag_id,
    groupID: value.group_id,
    groupName: value.group_name,
    name: value.tag_name,
    sortOrder: value.sort_order,
  };
}

function parseCreatedGroup(
  value: unknown,
): Omit<WecomTagGroup, "tags"> | undefined {
  if (
    !record(value) ||
    !exact(value, ["group_id", "group_name", "sort_order"]) ||
    !positiveInteger(value.group_id) ||
    !frozenText(value.group_name) ||
    !validSortOrder(value.sort_order)
  ) {
    return undefined;
  }
  return {
    id: value.group_id,
    name: value.group_name,
    sortOrder: value.sort_order,
  };
}

function parseCreatedTag(value: unknown): WecomTag | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "tag_id",
      "group_id",
      "group_name",
      "tag_name",
      "sort_order",
    ]) ||
    !positiveInteger(value.tag_id) ||
    !positiveInteger(value.group_id) ||
    !frozenText(value.group_name) ||
    !frozenText(value.tag_name) ||
    !validSortOrder(value.sort_order)
  ) {
    return undefined;
  }
  return {
    id: value.tag_id,
    groupID: value.group_id,
    groupName: value.group_name,
    name: value.tag_name,
    sortOrder: value.sort_order,
  };
}

export function parseWecomTagGroupCreateSuccess(
  value: unknown,
):
  | Extract<WecomTagGroupCreateResult, { readonly status: "created" }>
  | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "reason",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "sync_executed",
      "fixture_used",
      "dry_run",
      "group",
      "tag",
    ]) ||
    value.ok !== true ||
    value.reason !== "group_created" ||
    value.source_status !== "local_catalog" ||
    value.route_owner !== "ai_crm_next" ||
    value.fallback_used !== false ||
    value.real_external_call_executed !== false ||
    value.sync_executed !== false ||
    value.fixture_used !== false ||
    value.dry_run !== false
  ) {
    return undefined;
  }
  const group = parseCreatedGroup(value.group);
  const tag = parseCreatedTag(value.tag);
  if (
    !group ||
    !tag ||
    tag.groupID !== group.id ||
    tag.groupName !== group.name
  ) {
    return undefined;
  }
  return { status: "created", group, tag };
}

export function parseWecomTagRenameSuccess(
  value: unknown,
): WecomTag | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "reason",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "sync_executed",
      "fixture_used",
      "dry_run",
      "tag",
    ]) ||
    value.ok !== true ||
    value.reason !== "tag_updated" ||
    value.source_status !== "local_catalog" ||
    value.route_owner !== "ai_crm_next" ||
    value.fallback_used !== false ||
    value.real_external_call_executed !== false ||
    value.sync_executed !== false ||
    value.fixture_used !== false ||
    value.dry_run !== false
  ) {
    return undefined;
  }
  return parseCreatedTag(value.tag);
}

export function parseWecomTagGroupRenameSuccess(
  value: unknown,
): Omit<WecomTagGroup, "tags"> | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "reason",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "sync_executed",
      "fixture_used",
      "dry_run",
      "group",
    ]) ||
    value.ok !== true ||
    value.reason !== "group_updated" ||
    value.source_status !== "local_catalog" ||
    value.route_owner !== "ai_crm_next" ||
    value.fallback_used !== false ||
    value.real_external_call_executed !== false ||
    value.sync_executed !== false ||
    value.fixture_used !== false ||
    value.dry_run !== false
  ) {
    return undefined;
  }
  return parseCreatedGroup(value.group);
}

export type WecomTagGroupArchiveEnvelope =
  | {
      readonly status: "archived";
      readonly group: Omit<WecomTagGroup, "tags">;
    }
  | { readonly status: "validated" };

export function parseWecomTagGroupArchiveSuccess(
  value: unknown,
): WecomTagGroupArchiveEnvelope | undefined {
  if (!record(value)) return undefined;
  const common =
    value.ok === true &&
    value.source_status === "local_catalog" &&
    value.route_owner === "ai_crm_next" &&
    value.fallback_used === false &&
    value.real_external_call_executed === false &&
    value.sync_executed === false &&
    value.fixture_used === false;
  if (!common) return undefined;
  if (
    exact(value, [
      "ok",
      "reason",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "sync_executed",
      "fixture_used",
      "dry_run",
      "group",
    ]) &&
    value.reason === "group_archived" &&
    value.dry_run === false
  ) {
    const group = parseCreatedGroup(value.group);
    return group ? { status: "archived", group } : undefined;
  }
  if (
    exact(value, [
      "ok",
      "reason",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "sync_executed",
      "fixture_used",
      "dry_run",
    ]) &&
    value.reason === "group_archive_validated" &&
    value.dry_run === true
  ) {
    return { status: "validated" };
  }
  return undefined;
}

function sameTags(
  left: readonly WecomTag[],
  right: readonly WecomTag[],
): boolean {
  return (
    left.length === right.length &&
    left.every(
      (tag, index) =>
        tag.id === right[index]?.id &&
        tag.groupID === right[index]?.groupID &&
        tag.groupName === right[index]?.groupName &&
        tag.name === right[index]?.name &&
        tag.sortOrder === right[index]?.sortOrder,
    )
  );
}

function parseGroup(value: unknown): WecomTagGroup | undefined {
  if (
    !record(value) ||
    !exact(value, ["group_id", "group_name", "name", "sort_order", "tags"]) ||
    !positiveInteger(value.group_id) ||
    !frozenText(value.group_name) ||
    value.name !== value.group_name ||
    !validSortOrder(value.sort_order) ||
    !Array.isArray(value.tags)
  ) {
    return undefined;
  }
  const tags = value.tags.map(parseTag);
  if (
    tags.includes(undefined) ||
    (tags as readonly WecomTag[]).some(
      (tag) =>
        tag.groupID !== value.group_id || tag.groupName !== value.group_name,
    )
  ) {
    return undefined;
  }
  return {
    id: value.group_id,
    name: value.group_name,
    sortOrder: value.sort_order,
    tags: tags as readonly WecomTag[],
  };
}

export function parseWecomTagCatalog(
  value: unknown,
): WecomTagCatalog | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "items",
      "tags",
      "groups",
      "count",
      "total_tags",
      "tag_limit",
      "synced_at",
      "source_status",
      "read_model_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "sync_executed",
      "fixture_used",
    ]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    !Array.isArray(value.tags) ||
    !Array.isArray(value.groups) ||
    !nonnegativeInteger(value.count) ||
    !nonnegativeInteger(value.total_tags) ||
    value.tag_limit !== WECOM_TAG_LIMIT ||
    value.total_tags > value.tag_limit ||
    !frozenTimestamp(value.synced_at) ||
    value.source_status !== "local_catalog" ||
    value.read_model_status !== "ready" ||
    value.route_owner !== "ai_crm_next" ||
    value.fallback_used !== false ||
    value.real_external_call_executed !== false ||
    value.sync_executed !== false ||
    value.fixture_used !== false
  ) {
    return undefined;
  }

  const groups = value.groups.map(parseGroup);
  const tags = value.tags.map(parseTag);
  const items = value.items.map(parseTag);
  if (
    groups.includes(undefined) ||
    tags.includes(undefined) ||
    items.includes(undefined) ||
    value.count !== value.total_tags ||
    value.total_tags !== tags.length ||
    !sameTags(tags as readonly WecomTag[], items as readonly WecomTag[]) ||
    new Set((groups as readonly WecomTagGroup[]).map((group) => group.id))
      .size !== groups.length ||
    new Set((tags as readonly WecomTag[]).map((tag) => tag.id)).size !==
      tags.length ||
    !sameTags(
      (groups as readonly WecomTagGroup[]).flatMap((group) => group.tags),
      tags as readonly WecomTag[],
    )
  ) {
    return undefined;
  }
  return {
    totalTags: value.total_tags,
    tagLimit: value.tag_limit,
    snapshotAt: value.synced_at,
    groups: groups as readonly WecomTagGroup[],
    tags: tags as readonly WecomTag[],
  };
}

export async function loadWecomTagCatalog(
  transport: WecomTagsTransport = generatedWecomTagsTransport,
): Promise<WecomTagCatalogResult> {
  let response: Awaited<ReturnType<WecomTagsTransport["read"]>>;
  try {
    response = await transport.read();
  } catch {
    return { status: "unavailable" };
  }
  if (response.status === 401) return { status: "unauthenticated" };
  if (response.status === 403) return { status: "forbidden" };
  if (response.status !== 200) return { status: "unavailable" };
  const catalog = parseWecomTagCatalog(response.data);
  return catalog ? { status: "loaded", catalog } : { status: "invalid" };
}

function normalizedCreateText(value: string): string | undefined {
  const normalized = value.trim();
  return frozenText(normalized) ? normalized : undefined;
}

function validCSRFToken(value: string): boolean {
  return /^[A-Za-z0-9_-]{43}$/.test(value);
}

function validIdempotencyKey(value: string): boolean {
  return (
    value.length >= 16 &&
    value.length <= 128 &&
    value.trim() === value &&
    /^[A-Za-z0-9_-]+$/.test(value)
  );
}

export async function createWecomTagGroup(
  transport: WecomTagsTransport,
  groupName: string,
  firstTagName: string,
  csrfToken: string,
  idempotencyKey: string,
): Promise<WecomTagGroupCreateResult> {
  const group_name = normalizedCreateText(groupName);
  const first_tag_name = normalizedCreateText(firstTagName);
  if (
    !group_name ||
    !first_tag_name ||
    !validCSRFToken(csrfToken) ||
    !validIdempotencyKey(idempotencyKey)
  ) {
    return { status: "invalid" };
  }

  let response: Awaited<ReturnType<WecomTagsTransport["create"]>>;
  try {
    response = await transport.create(
      { group_name, first_tag_name },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": csrfToken,
          "Idempotency-Key": idempotencyKey,
        },
      },
    );
  } catch {
    return { status: "unknown" };
  }

  if (response.status === 401) return { status: "unauthenticated" };
  if (response.status === 403) return { status: "forbidden" };
  if (response.status === 400) return { status: "invalid" };
  if (response.status !== 200) return { status: "unknown" };
  const created = parseWecomTagGroupCreateSuccess(response.data);
  return created &&
    created.group.name === group_name &&
    created.tag.name === first_tag_name
    ? created
    : { status: "unknown" };
}

function validRenameTarget(tag: WecomTag): boolean {
  return (
    positiveInteger(tag.id) &&
    positiveInteger(tag.groupID) &&
    frozenText(tag.groupName) &&
    frozenText(tag.name) &&
    validSortOrder(tag.sortOrder)
  );
}

export async function renameWecomTag(
  transport: WecomTagsTransport,
  tag: WecomTag,
  rawName: string,
  csrfToken: string,
  idempotencyKey: string,
): Promise<WecomTagRenameResult> {
  const tag_name = normalizedCreateText(rawName);
  if (
    !validRenameTarget(tag) ||
    !tag_name ||
    !validCSRFToken(csrfToken) ||
    !validIdempotencyKey(idempotencyKey)
  ) {
    return { status: "invalid" };
  }

  let response: Awaited<ReturnType<WecomTagsTransport["rename"]>>;
  try {
    response = await transport.rename(
      tag.id,
      { tag_name },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": csrfToken,
          "Idempotency-Key": idempotencyKey,
        },
      },
    );
  } catch {
    return { status: "unknown" };
  }

  if (response.status === 401) return { status: "unauthenticated" };
  if (response.status === 403) return { status: "forbidden" };
  if (response.status === 400 || response.status === 404)
    return { status: "invalid" };
  if (response.status !== 200) return { status: "unknown" };
  const renamed = parseWecomTagRenameSuccess(response.data);
  return renamed &&
    renamed.id === tag.id &&
    renamed.groupID === tag.groupID &&
    renamed.groupName === tag.groupName &&
    renamed.sortOrder === tag.sortOrder &&
    renamed.name === tag_name
    ? { status: "confirmed", tag: renamed }
    : { status: "unknown" };
}

function validRenameGroupTarget(group: Omit<WecomTagGroup, "tags">): boolean {
  return (
    positiveInteger(group.id) &&
    frozenText(group.name) &&
    validSortOrder(group.sortOrder)
  );
}

function validArchiveGroupTarget(group: WecomTagGroup): boolean {
  return (
    validRenameGroupTarget(group) &&
    group.tags.every(
      (tag) =>
        validRenameTarget(tag) &&
        tag.groupID === group.id &&
        tag.groupName === group.name,
    ) &&
    new Set(group.tags.map((tag) => tag.id)).size === group.tags.length
  );
}

export async function renameWecomTagGroup(
  transport: WecomTagsTransport,
  group: Omit<WecomTagGroup, "tags">,
  rawName: string,
  csrfToken: string,
  idempotencyKey: string,
): Promise<WecomTagGroupRenameResult> {
  const group_name = normalizedCreateText(rawName);
  if (
    !validRenameGroupTarget(group) ||
    !group_name ||
    !validCSRFToken(csrfToken) ||
    !validIdempotencyKey(idempotencyKey)
  ) {
    return { status: "invalid" };
  }

  let response: Awaited<ReturnType<WecomTagsTransport["renameGroup"]>>;
  try {
    response = await transport.renameGroup(
      group.id,
      { group_name },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": csrfToken,
          "Idempotency-Key": idempotencyKey,
        },
      },
    );
  } catch {
    return { status: "unknown" };
  }

  if (response.status === 401) return { status: "unauthenticated" };
  if (response.status === 403) return { status: "forbidden" };
  if (response.status === 400 || response.status === 404)
    return { status: "invalid" };
  if (response.status !== 200) return { status: "unknown" };
  const renamed = parseWecomTagGroupRenameSuccess(response.data);
  return renamed &&
    renamed.id === group.id &&
    renamed.sortOrder === group.sortOrder &&
    renamed.name === group_name
    ? { status: "confirmed", group: renamed }
    : { status: "unknown" };
}

export async function archiveWecomTagGroup(
  transport: WecomTagsTransport,
  group: WecomTagGroup,
  csrfToken: string,
  idempotencyKey: string,
): Promise<WecomTagGroupArchiveResult> {
  if (
    !validArchiveGroupTarget(group) ||
    !validCSRFToken(csrfToken) ||
    !validIdempotencyKey(idempotencyKey)
  ) {
    return { status: "invalid" };
  }

  let response: Awaited<ReturnType<WecomTagsTransport["archiveGroup"]>>;
  try {
    response = await transport.archiveGroup(
      group.id,
      {},
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": csrfToken,
          "Idempotency-Key": idempotencyKey,
        },
      },
    );
  } catch {
    return { status: "unknown" };
  }

  if (response.status === 401) return { status: "unauthenticated" };
  if (response.status === 403) return { status: "forbidden" };
  if (response.status === 400 || response.status === 404)
    return { status: "invalid" };
  if (response.status !== 200) return { status: "unknown" };
  const archived = parseWecomTagGroupArchiveSuccess(response.data);
  return archived?.status === "archived" &&
    archived.group.id === group.id &&
    archived.group.sortOrder === group.sortOrder &&
    archived.group.name === `archived:${group.id}`
    ? archived
    : { status: "unknown" };
}

export function newWecomTagIdempotencyKey(): string | undefined {
  if (
    typeof globalThis.crypto === "undefined" ||
    typeof globalThis.crypto.getRandomValues !== "function" ||
    typeof globalThis.btoa !== "function"
  ) {
    return undefined;
  }
  const bytes = new Uint8Array(32);
  globalThis.crypto.getRandomValues(bytes);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return globalThis
    .btoa(binary)
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replace(/=+$/, "");
}

export function confirmsCreatedWecomTagGroup(
  catalog: WecomTagCatalog,
  created: Extract<WecomTagGroupCreateResult, { readonly status: "created" }>,
): boolean {
  const group = catalog.groups.find((item) => item.id === created.group.id);
  const tag = catalog.tags.find((item) => item.id === created.tag.id);
  return (
    group?.name === created.group.name &&
    group.sortOrder === created.group.sortOrder &&
    tag?.groupID === created.tag.groupID &&
    tag.groupName === created.tag.groupName &&
    tag.name === created.tag.name &&
    tag.sortOrder === created.tag.sortOrder &&
    group.tags.some(
      (item) =>
        item.id === created.tag.id &&
        item.groupID === created.tag.groupID &&
        item.groupName === created.tag.groupName &&
        item.name === created.tag.name &&
        item.sortOrder === created.tag.sortOrder,
    )
  );
}

export function confirmsRenamedWecomTag(
  catalog: WecomTagCatalog,
  renamed: WecomTag,
): boolean {
  const tag = catalog.tags.find((item) => item.id === renamed.id);
  const group = catalog.groups.find((item) => item.id === renamed.groupID);
  return (
    tag?.groupID === renamed.groupID &&
    tag.groupName === renamed.groupName &&
    tag.name === renamed.name &&
    tag.sortOrder === renamed.sortOrder &&
    group !== undefined &&
    group.name === renamed.groupName &&
    group.sortOrder >= 0 &&
    group.tags.some(
      (item) =>
        item.id === renamed.id &&
        item.groupID === renamed.groupID &&
        item.groupName === renamed.groupName &&
        item.name === renamed.name &&
        item.sortOrder === renamed.sortOrder,
    )
  );
}

export function confirmsRenamedWecomTagGroup(
  catalog: WecomTagCatalog,
  renamed: Omit<WecomTagGroup, "tags">,
): boolean {
  const group = catalog.groups.find((item) => item.id === renamed.id);
  const catalogTags = catalog.tags.filter(
    (item) => item.groupID === renamed.id,
  );
  return (
    group !== undefined &&
    group.name === renamed.name &&
    group.sortOrder === renamed.sortOrder &&
    group.tags.length === catalogTags.length &&
    group.tags.every(
      (item) => item.groupID === renamed.id && item.groupName === renamed.name,
    ) &&
    catalogTags.every((item) => item.groupName === renamed.name)
  );
}

export function confirmsArchivedWecomTagGroup(
  catalog: WecomTagCatalog,
  archived: Extract<WecomTagGroupArchiveResult, { readonly status: "archived" }>,
  original: WecomTagGroup,
): boolean {
  const archivedTagIDs = new Set(original.tags.map((tag) => tag.id));
  return (
    !catalog.groups.some((group) => group.id === archived.group.id) &&
    !catalog.tags.some((tag) => archivedTagIDs.has(tag.id)) &&
    !catalog.groups.some((group) =>
      group.tags.some((tag) => archivedTagIDs.has(tag.id)),
    )
  );
}

function matches(value: string, query: string): boolean {
  return value.toLocaleLowerCase().includes(query);
}

export function filterWecomTagGroups(
  catalog: WecomTagCatalog,
  rawQuery: string,
): readonly WecomTagGroup[] {
  const query = rawQuery.trim().toLocaleLowerCase();
  if (query === "") return catalog.groups;
  return catalog.groups.flatMap((group) => {
    if (matches(group.name, query)) return [group];
    const tags = group.tags.filter(
      (tag) => matches(tag.name, query) || String(tag.id).includes(query),
    );
    return tags.length === 0 ? [] : [{ ...group, tags }];
  });
}

export function firstMatchingWecomTagGroupID(
  groups: readonly WecomTagGroup[],
): number | undefined {
  return groups[0]?.id;
}

export function wecomTagSearchState(
  catalog: WecomTagCatalog,
  query: string,
): {
  readonly groups: readonly WecomTagGroup[];
  readonly selectedGroupID: number | undefined;
  readonly page: 0;
} {
  const groups = filterWecomTagGroups(catalog, query);
  return {
    groups,
    selectedGroupID: firstMatchingWecomTagGroupID(groups),
    page: 0,
  };
}

export function wecomTagPageCount(tags: readonly WecomTag[]): number {
  return Math.max(1, Math.ceil(tags.length / WECOM_TAGS_PAGE_SIZE));
}

export function wecomTagPage(
  tags: readonly WecomTag[],
  page: number,
): readonly WecomTag[] {
  const bounded = Math.min(Math.max(0, page), wecomTagPageCount(tags) - 1);
  const start = bounded * WECOM_TAGS_PAGE_SIZE;
  return tags.slice(start, start + WECOM_TAGS_PAGE_SIZE);
}

export function previousWecomTagPage(page: number): number {
  return Math.max(0, page - 1);
}

export function nextWecomTagPage(
  page: number,
  tags: readonly WecomTag[],
): number | undefined {
  return page + 1 < wecomTagPageCount(tags) ? page + 1 : undefined;
}
