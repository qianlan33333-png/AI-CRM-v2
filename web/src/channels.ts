import { listLegacyChannels } from "./api/generated/health";

export type ChannelsRole = "admin" | "ops" | "sales";
export type ChannelStatus = "active" | "inactive" | "archived";
export type ChannelStatusFilter = "all" | ChannelStatus;

export interface ChannelListItem {
  readonly id: number;
  readonly name: string;
  readonly code: string;
  readonly status: ChannelStatus;
  readonly assigneeCount: 0;
  readonly contactCount: 0;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export type ChannelsFailure =
  | "unauthenticated"
  | "forbidden"
  | "unavailable"
  | "invalid";

export type ChannelListResult =
  | { readonly status: "loaded"; readonly items: readonly ChannelListItem[] }
  | { readonly status: ChannelsFailure };

const channelListParams = { limit: 300, include_archived: true } as const;

async function generatedRead(options?: RequestInit) {
  return listLegacyChannels(channelListParams, {
    credentials: "same-origin",
    ...options,
  });
}

export interface ChannelsTransport {
  readonly read: typeof generatedRead;
}

export const generatedChannelsTransport: ChannelsTransport = {
  read: generatedRead,
};

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return (
    actual.length === keys.length && actual.every((key) => keys.includes(key))
  );
}

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function frozenText(value: unknown, allowEmpty = false): value is string {
  return (
    typeof value === "string" &&
    (allowEmpty || value.length > 0) &&
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

function parseChannel(value: unknown): ChannelListItem | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "id",
      "channel_name",
      "channel_code",
      "status",
      "assignee_count",
      "channel_contact_count",
      "created_at",
      "updated_at",
    ]) ||
    !positiveInteger(value.id) ||
    !frozenText(value.channel_name, true) ||
    !frozenText(value.channel_code, true) ||
    (value.status !== "active" &&
      value.status !== "inactive" &&
      value.status !== "archived") ||
    value.assignee_count !== 0 ||
    value.channel_contact_count !== 0 ||
    !frozenTimestamp(value.created_at) ||
    !frozenTimestamp(value.updated_at)
  ) {
    return undefined;
  }
  return {
    id: value.id,
    name: value.channel_name,
    code: value.channel_code,
    status: value.status,
    assigneeCount: 0,
    contactCount: 0,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

export function parseChannelList(
  value: unknown,
): readonly ChannelListItem[] | undefined {
  if (
    !record(value) ||
    !exact(value, ["ok", "channels", "reason", "source"]) ||
    value.ok !== true ||
    !Array.isArray(value.channels) ||
    value.reason !== "channels_listed" ||
    value.source !== "ai_crm_next"
  ) {
    return undefined;
  }
  const items = value.channels.map(parseChannel);
  if (
    items.includes(undefined) ||
    new Set((items as readonly ChannelListItem[]).map((item) => item.id)).size !==
      items.length
  ) {
    return undefined;
  }
  return items as readonly ChannelListItem[];
}

function platformError(value: unknown, code: string): boolean {
  if (!record(value)) return false;
  const expected = ["code", "message", "request_id"];
  const withDetails = [...expected, "details"];
  return (
    (exact(value, expected) || exact(value, withDetails)) &&
    value.code === code &&
    frozenText(value.message) &&
    frozenText(value.request_id) &&
    (value.details === undefined || Array.isArray(value.details))
  );
}

function compatibilityError(value: unknown): boolean {
  return (
    record(value) &&
    exact(value, ["ok", "detail"]) &&
    value.ok === false &&
    frozenText(value.detail)
  );
}

function failure(status: number, body: unknown): ChannelsFailure {
  if (status === 401 && platformError(body, "UNAUTHENTICATED")) {
    return "unauthenticated";
  }
  if (status === 403 && platformError(body, "UNAUTHORIZED")) {
    return "forbidden";
  }
  if (status === 400 && compatibilityError(body)) return "invalid";
  if (
    status === 503 &&
    (compatibilityError(body) || platformError(body, "DEPENDENCY_UNAVAILABLE"))
  ) {
    return "unavailable";
  }
  return "invalid";
}

export async function loadChannels(
  transport: ChannelsTransport = generatedChannelsTransport,
): Promise<ChannelListResult> {
  let response: Awaited<ReturnType<ChannelsTransport["read"]>>;
  try {
    response = await transport.read();
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200) {
    return { status: failure(response.status, response.data) };
  }
  const items = parseChannelList(response.data);
  return items ? { status: "loaded", items } : { status: "invalid" };
}

export function filterChannels(
  items: readonly ChannelListItem[],
  keyword: string,
  status: ChannelStatusFilter,
): readonly ChannelListItem[] {
  const query = keyword.trim().toLocaleLowerCase();
  return items.filter(
    (item) =>
      (status === "all" || item.status === status) &&
      (query === "" ||
        item.name.toLocaleLowerCase().includes(query) ||
        item.code.toLocaleLowerCase().includes(query)),
  );
}
