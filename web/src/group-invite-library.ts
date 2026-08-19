import {
  listLegacyGroupInvites,
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

export interface GroupInviteLibraryTransport {
  readonly list: typeof generatedList;
}

export const generatedGroupInviteLibraryTransport: GroupInviteLibraryTransport = {
  list: generatedList,
};

export type GroupInviteLibraryFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type GroupInviteLibraryResult =
  | { readonly status: "loaded"; readonly page: GroupInviteLibraryPageData }
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
): GroupInviteLibraryItem | undefined {
  if (!record(value)) return undefined;
  const keys = Object.keys(value);
  const optionalCover = keys.includes("cover_image_id");
  if (
    !exact(
      value,
      optionalCover
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
    Date.parse(value.updated_at) < Date.parse(value.created_at)
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
        item.updatedAt === right[index]?.updatedAt,
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
  const items = value.items.map(parseGroupInviteLibraryItem);
  const mirrored = value.group_invites.map(parseGroupInviteLibraryItem);
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

function failure(status: number): GroupInviteLibraryFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
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
