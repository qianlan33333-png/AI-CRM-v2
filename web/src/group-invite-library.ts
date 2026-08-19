import {
  archiveLegacyGroupInvite,
  createLegacyGroupInvite,
  getLegacyGroupInvite,
  listLegacyGroupInvites,
  updateLegacyGroupInvite,
  type LegacyGroupInviteCreateRequest,
  type LegacyGroupInviteUpdateRequest,
  type ListLegacyGroupInvitesParams,
} from "./api/generated/health";

export type GroupInviteLibraryRole = "admin" | "ops" | "sales";
export const GROUP_INVITE_PAGE_SIZE = 100;
const MAXIMUM_OFFSET = 1_000_000;

export interface GroupInviteLibraryItem {
  readonly id: number;
  readonly name: string;
  readonly title: string;
  readonly description: string;
  readonly joinURL: string;
  readonly coverImageID?: number;
  readonly enabled: boolean;
  readonly createdAt: string;
  readonly updatedAt: string;
  readonly createdBy?: number;
  readonly updatedBy?: number;
  readonly version?: number;
  readonly archivedAt?: string;
}

export interface GroupInviteLibraryDraft {
  readonly name: string;
  readonly title: string;
  readonly description: string;
  readonly joinURL: string;
  readonly enabled: boolean;
}

export interface GroupInviteLibraryPageData {
  readonly items: readonly GroupInviteLibraryItem[];
  readonly total: number;
  readonly limit: number;
  readonly offset: number;
}

export interface GroupInviteLibraryTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  params: ListLegacyGroupInvitesParams,
  options: RequestInit,
): Promise<GroupInviteLibraryTransportResponse> {
  return listLegacyGroupInvites(params, options);
}

async function generatedDetail(
  itemID: number,
  options: RequestInit,
): Promise<GroupInviteLibraryTransportResponse> {
  return getLegacyGroupInvite(itemID, options);
}

async function generatedCreate(
  request: LegacyGroupInviteCreateRequest,
  options: RequestInit,
): Promise<GroupInviteLibraryTransportResponse> {
  return createLegacyGroupInvite(request, options);
}

async function generatedUpdate(
  itemID: number,
  request: LegacyGroupInviteUpdateRequest,
  options: RequestInit,
): Promise<GroupInviteLibraryTransportResponse> {
  return updateLegacyGroupInvite(itemID, request, options);
}

async function generatedArchive(
  itemID: number,
  options: RequestInit,
): Promise<GroupInviteLibraryTransportResponse> {
  return archiveLegacyGroupInvite(itemID, options);
}

export interface GroupInviteLibraryTransport {
  readonly list: typeof generatedList;
  readonly detail?: typeof generatedDetail;
  readonly create?: typeof generatedCreate;
  readonly update?: typeof generatedUpdate;
  readonly archive?: typeof generatedArchive;
}

export const generatedGroupInviteLibraryTransport: GroupInviteLibraryTransport = {
  list: generatedList,
  detail: generatedDetail,
  create: generatedCreate,
  update: generatedUpdate,
  archive: generatedArchive,
};

export type GroupInviteLibraryFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";

export type GroupInviteLibraryResult =
  | { readonly status: "loaded"; readonly page: GroupInviteLibraryPageData }
  | { readonly status: GroupInviteLibraryFailure };

export type GroupInviteLibraryDetailResult =
  | { readonly status: "loaded"; readonly item: GroupInviteLibraryItem }
  | { readonly status: GroupInviteLibraryFailure };

export type GroupInviteLibraryMutationResult =
  | { readonly status: "saved"; readonly item: GroupInviteLibraryItem }
  | { readonly status: "archived"; readonly item: GroupInviteLibraryItem }
  | { readonly status: GroupInviteLibraryFailure };

function record(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null)
  );
}

function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function boundedText(value: unknown, maximumBytes: number, empty = false): value is string {
  return (
    typeof value === "string" &&
    (empty || value.length > 0) &&
    value.trim() === value &&
    !value.includes("\x00") &&
    new TextEncoder().encode(value).length <= maximumBytes
  );
}

function name(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    value.trim() === value &&
    !value.includes("\x00")
  );
}

function joinURL(value: unknown): value is string {
  if (
    !boundedText(value, 2048) ||
    !value.startsWith("https://work.weixin.qq.com/gm/")
  ) {
    return false;
  }
  try {
    const parsed = new URL(value);
    return (
      parsed.protocol === "https:" &&
      parsed.host === "work.weixin.qq.com" &&
      parsed.username === "" &&
      parsed.password === "" &&
      parsed.pathname.startsWith("/gm/") &&
      parsed.pathname.length > "/gm/".length &&
      parsed.search === "" &&
      parsed.hash === ""
    );
  } catch {
    return false;
  }
}

function timestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) ||
    !Number.isFinite(Date.parse(value))
  ) {
    return false;
  }
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, 0);
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day &&
    date.getUTCHours() === hour &&
    date.getUTCMinutes() === minute &&
    date.getUTCSeconds() === second
  );
}

export function parseGroupInviteLibraryItem(
  value: unknown,
  allowArchived = false,
): GroupInviteLibraryItem | undefined {
  if (!record(value)) return undefined;
  const keys = Object.keys(value);
  const optionalCover = keys.includes("cover_image_id");
  const archived = keys.includes("archived_at");
  if (
    !exact(
      value,
      optionalCover && archived
        ? [
            "id",
            "name",
            "title",
            "description",
            "join_url",
            "cover_image_id",
            "enabled",
            "created_by",
            "updated_by",
            "version",
            "created_at",
            "updated_at",
            "archived_at",
          ]
        : optionalCover
        ? [
            "id",
            "name",
            "title",
            "description",
            "join_url",
            "cover_image_id",
            "enabled",
            "created_by",
            "updated_by",
            "version",
            "created_at",
            "updated_at",
          ]
        : archived
          ? [
              "id",
              "name",
              "title",
              "description",
              "join_url",
              "enabled",
              "created_by",
              "updated_by",
              "version",
              "created_at",
              "updated_at",
              "archived_at",
            ]
        : [
            "id",
            "name",
            "title",
            "description",
            "join_url",
            "enabled",
            "created_by",
            "updated_by",
            "version",
            "created_at",
            "updated_at",
          ],
    ) ||
    !positive(value.id) ||
    !name(value.name) ||
    !boundedText(value.title, 128) ||
    !boundedText(value.description, 512, true) ||
    !joinURL(value.join_url) ||
    (optionalCover && !nonnegative(value.cover_image_id)) ||
    typeof value.enabled !== "boolean" ||
    !positive(value.created_by) ||
    !positive(value.updated_by) ||
    !positive(value.version) ||
    !timestamp(value.created_at) ||
    !timestamp(value.updated_at) ||
    Date.parse(value.updated_at) < Date.parse(value.created_at) ||
    (archived &&
      (!allowArchived ||
        !timestamp(value.archived_at) ||
        value.enabled !== false ||
        Date.parse(value.archived_at) < Date.parse(value.updated_at)))
  ) {
    return undefined;
  }
  const coverImageID =
    typeof value.cover_image_id === "number" && value.cover_image_id > 0
      ? value.cover_image_id
      : undefined;
  return {
    id: value.id,
    name: value.name,
    title: value.title,
    description: value.description,
    joinURL: value.join_url,
    ...(coverImageID === undefined ? {} : { coverImageID }),
    enabled: value.enabled,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
    createdBy: value.created_by,
    updatedBy: value.updated_by,
    version: value.version,
    ...(archived ? { archivedAt: value.archived_at as string } : {}),
  };
}

function sameItems(
  left: readonly GroupInviteLibraryItem[],
  right: readonly GroupInviteLibraryItem[],
): boolean {
  return (
    left.length === right.length &&
    left.every(
      (item, index) =>
        item.id === right[index]?.id &&
        item.name === right[index]?.name &&
        item.title === right[index]?.title &&
        item.description === right[index]?.description &&
        item.joinURL === right[index]?.joinURL &&
        item.coverImageID === right[index]?.coverImageID &&
        item.enabled === right[index]?.enabled &&
        item.createdAt === right[index]?.createdAt &&
        item.updatedAt === right[index]?.updatedAt &&
        item.createdBy === right[index]?.createdBy &&
        item.updatedBy === right[index]?.updatedBy &&
        item.version === right[index]?.version &&
        item.archivedAt === right[index]?.archivedAt,
    )
  );
}

export function parseGroupInviteLibraryPage(
  value: unknown,
  expectedOffset: number,
): GroupInviteLibraryPageData | undefined {
  if (
    !nonnegative(expectedOffset) ||
    expectedOffset > MAXIMUM_OFFSET ||
    expectedOffset % GROUP_INVITE_PAGE_SIZE !== 0 ||
    !record(value) ||
    !exact(value, [
      "ok",
      "items",
      "group_invites",
      "total",
      "limit",
      "offset",
      "provider_call_executed",
    ]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    !Array.isArray(value.group_invites) ||
    !nonnegative(value.total) ||
    value.limit !== GROUP_INVITE_PAGE_SIZE ||
    value.offset !== expectedOffset ||
    value.provider_call_executed !== false ||
    value.items.length > GROUP_INVITE_PAGE_SIZE ||
    value.total < expectedOffset + value.items.length ||
    (value.items.length < GROUP_INVITE_PAGE_SIZE &&
      expectedOffset + value.items.length < value.total)
  ) {
    return undefined;
  }
  const items = value.items.map((item) => parseGroupInviteLibraryItem(item));
  const mirrored = value.group_invites.map((item) => parseGroupInviteLibraryItem(item));
  if (
    items.includes(undefined) ||
    mirrored.includes(undefined) ||
    !sameItems(
      items as readonly GroupInviteLibraryItem[],
      mirrored as readonly GroupInviteLibraryItem[],
    ) ||
    new Set((items as readonly GroupInviteLibraryItem[]).map((item) => item.id)).size !==
      items.length
  ) {
    return undefined;
  }
  return {
    items: items as readonly GroupInviteLibraryItem[],
    total: value.total,
    limit: GROUP_INVITE_PAGE_SIZE,
    offset: expectedOffset,
  };
}

export function parseGroupInviteLibraryDetail(
  value: unknown,
  expectedItemID: number,
): GroupInviteLibraryItem | undefined {
  if (
    !positive(expectedItemID) ||
    !record(value) ||
    !exact(value, ["ok", "item", "group_invite", "provider_call_executed"]) ||
    value.ok !== true ||
    value.provider_call_executed !== false
  ) {
    return undefined;
  }
  const item = parseGroupInviteLibraryItem(value.item);
  const mirrored = parseGroupInviteLibraryItem(value.group_invite);
  return item &&
    mirrored &&
    item.id === expectedItemID &&
    mirrored.id === expectedItemID &&
    sameItems([item], [mirrored])
    ? item
    : undefined;
}

function failure(status: number): GroupInviteLibraryFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400) return "invalid";
  return "unavailable";
}

export async function loadGroupInviteLibrary(
  transport: GroupInviteLibraryTransport,
  offset = 0,
): Promise<GroupInviteLibraryResult> {
  if (
    !nonnegative(offset) ||
    offset > MAXIMUM_OFFSET ||
    offset % GROUP_INVITE_PAGE_SIZE !== 0
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.list(
      { limit: GROUP_INVITE_PAGE_SIZE, offset, enabled_only: false },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseGroupInviteLibraryPage(response.data, offset);
    return page ? { status: "loaded", page } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadGroupInviteDetail(
  transport: GroupInviteLibraryTransport,
  itemID: number,
): Promise<GroupInviteLibraryDetailResult> {
  if (!positive(itemID)) return { status: "invalid" };
  const detail = transport.detail;
  if (!detail) return { status: "unavailable" };
  try {
    const response = await detail(itemID, { credentials: "same-origin" });
    if (response.status !== 200) return { status: failure(response.status) };
    const item = parseGroupInviteLibraryDetail(response.data, itemID);
    return item ? { status: "loaded", item } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

function validCSRFToken(value: string): boolean {
  return /^[A-Za-z0-9_-]{43}$/.test(value);
}

function validIdempotencyKey(value: string): boolean {
  return (
    value.length >= 16 &&
    value.length <= 128 &&
    value.trim() === value &&
    /^[\x21-\x7e]+$/.test(value)
  );
}

function mutationOptions(
  csrfToken: string,
  idempotencyKey: string,
): RequestInit | undefined {
  if (!validCSRFToken(csrfToken) || !validIdempotencyKey(idempotencyKey)) {
    return undefined;
  }
  return {
    credentials: "same-origin",
    headers: {
      "X-CSRF-Token": csrfToken,
      "Idempotency-Key": idempotencyKey,
    },
  };
}

interface NormalizedDraft {
  readonly name?: string;
  readonly title: string;
  readonly description: string;
  readonly joinURL: string;
  readonly enabled: boolean;
}

function normalizeDraft(value: GroupInviteLibraryDraft): NormalizedDraft | undefined {
  const nameValue = value.name.trim();
  const title = value.title.trim();
  const description = value.description.trim();
  const join = value.joinURL.trim();
  if (
    !boundedText(title, 128) ||
    !boundedText(description, 512, true) ||
    !joinURL(join) ||
    typeof value.enabled !== "boolean" ||
    (nameValue !== "" && !boundedText(nameValue, 128))
  ) {
    return undefined;
  }
  return {
    ...(nameValue === "" ? {} : { name: nameValue }),
    title,
    description,
    joinURL: join,
    enabled: value.enabled,
  };
}

function expectedItem(
  current: GroupInviteLibraryItem | undefined,
  draft: NormalizedDraft,
): Pick<GroupInviteLibraryItem, "name" | "title" | "description" | "joinURL" | "enabled"> {
  return {
    name: draft.name ?? current?.name ?? draft.title,
    title: draft.title,
    description: draft.description,
    joinURL: draft.joinURL,
    enabled: draft.enabled,
  };
}

function matchesExpected(
  item: GroupInviteLibraryItem,
  expected: Pick<GroupInviteLibraryItem, "name" | "title" | "description" | "joinURL" | "enabled">,
): boolean {
  return (
    item.name === expected.name &&
    item.title === expected.title &&
    item.description === expected.description &&
    item.joinURL === expected.joinURL &&
    item.enabled === expected.enabled &&
    item.archivedAt === undefined
  );
}

function parseMutation(value: unknown): GroupInviteLibraryItem | undefined {
  if (!record(value)) return undefined;
  const baseKeys = [
    "ok",
    "item",
    "group_invite",
    "local_only",
    "provider_call_executed",
    "real_external_call_executed",
  ];
  if (
    !exact(value, baseKeys) &&
    !exact(value, [...baseKeys, "item_id"])
  ) {
    return undefined;
  }
  const item = parseGroupInviteLibraryItem(value.item);
  const mirrored = parseGroupInviteLibraryItem(value.group_invite);
  if (
    value.ok !== true ||
    value.local_only !== true ||
    value.provider_call_executed !== false ||
    value.real_external_call_executed !== false ||
    !item ||
    !mirrored ||
    !sameItems([item], [mirrored]) ||
    ("item_id" in value && value.item_id !== item.id)
  ) {
    return undefined;
  }
  return item;
}

function parseArchive(value: unknown): GroupInviteLibraryItem | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "item",
      "archived",
      "local_only",
      "provider_call_executed",
      "real_external_call_executed",
    ]) ||
    value.ok !== true ||
    value.archived !== true ||
    value.local_only !== true ||
    value.provider_call_executed !== false ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const item = parseGroupInviteLibraryItem(value.item, true);
  return item?.archivedAt !== undefined && item.enabled === false ? item : undefined;
}

export async function createGroupInvite(
  transport: GroupInviteLibraryTransport,
  draft: GroupInviteLibraryDraft,
  csrfToken: string,
  idempotencyKey: string,
): Promise<GroupInviteLibraryMutationResult> {
  const normalized = normalizeDraft(draft);
  const options = mutationOptions(csrfToken, idempotencyKey);
  if (!normalized || !options) return { status: "invalid" };
  const request: LegacyGroupInviteCreateRequest = {
    title: normalized.title,
    description: normalized.description,
    join_url: normalized.joinURL,
    enabled: normalized.enabled,
    ...(normalized.name === undefined ? {} : { name: normalized.name }),
  };
  const create = transport.create;
  if (!create) return { status: "unavailable" };
  try {
    const response = await create(request, options);
    if (response.status !== 200) return { status: failure(response.status) };
    const item = parseMutation(response.data);
    return item && matchesExpected(item, expectedItem(undefined, normalized))
      ? { status: "saved", item }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function updateGroupInvite(
  transport: GroupInviteLibraryTransport,
  current: GroupInviteLibraryItem,
  draft: GroupInviteLibraryDraft,
  csrfToken: string,
  idempotencyKey: string,
): Promise<GroupInviteLibraryMutationResult> {
  const normalized = normalizeDraft(draft);
  const options = mutationOptions(csrfToken, idempotencyKey);
  if (!positive(current.id) || !normalized || !options) return { status: "invalid" };
  const request: LegacyGroupInviteUpdateRequest = {
    title: normalized.title,
    description: normalized.description,
    join_url: normalized.joinURL,
    enabled: normalized.enabled,
    ...(normalized.name === undefined ? {} : { name: normalized.name }),
  };
  const update = transport.update;
  if (!update) return { status: "unavailable" };
  try {
    const response = await update(current.id, request, options);
    if (response.status !== 200) return { status: failure(response.status) };
    const item = parseMutation(response.data);
    return item &&
      item.id === current.id &&
      matchesExpected(item, expectedItem(current, normalized))
      ? { status: "saved", item }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function archiveGroupInvite(
  transport: GroupInviteLibraryTransport,
  current: GroupInviteLibraryItem,
  csrfToken: string,
  idempotencyKey: string,
): Promise<GroupInviteLibraryMutationResult> {
  const options = mutationOptions(csrfToken, idempotencyKey);
  if (!positive(current.id) || !options) return { status: "invalid" };
  const archive = transport.archive;
  if (!archive) return { status: "unavailable" };
  try {
    const response = await archive(current.id, options);
    if (response.status !== 200) return { status: failure(response.status) };
    const item = parseArchive(response.data);
    return item && item.id === current.id
      ? { status: "archived", item }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export function previousGroupInvitePage(
  page: GroupInviteLibraryPageData,
): number | undefined {
  return page.offset >= page.limit ? page.offset - page.limit : undefined;
}

export function nextGroupInvitePage(
  page: GroupInviteLibraryPageData,
): number | undefined {
  return page.offset + page.items.length < page.total &&
    page.offset + page.limit <= MAXIMUM_OFFSET
    ? page.offset + page.limit
    : undefined;
}
