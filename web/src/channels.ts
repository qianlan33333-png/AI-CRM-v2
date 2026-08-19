import {
  listLegacyChannels,
  updateLegacyChannel,
} from "./api/generated/health";

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
  "unauthenticated" | "forbidden" | "unavailable" | "invalid";

export type ChannelListResult =
  | { readonly status: "loaded"; readonly items: readonly ChannelListItem[] }
  | { readonly status: ChannelsFailure };

export type ChannelStatusUpdateResult =
  | {
      readonly status: "confirmed";
      readonly items: readonly ChannelListItem[];
    }
  | { readonly status: "unknown" }
  | { readonly status: ChannelsFailure };

const channelListParams = { limit: 300, include_archived: true } as const;

async function generatedRead(options?: RequestInit) {
  return listLegacyChannels(channelListParams, {
    credentials: "same-origin",
    ...options,
  });
}

async function generatedWrite(
  channelID: number,
  status: ChannelStatus,
  options?: RequestInit,
) {
  return updateLegacyChannel(
    channelID,
    { status },
    { ...options, credentials: "same-origin" },
  );
}

export interface ChannelsTransport {
  readonly read: typeof generatedRead;
  readonly write: typeof generatedWrite;
}

export const generatedChannelsTransport: ChannelsTransport = {
  read: generatedRead,
  write: generatedWrite,
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
    new Set((items as readonly ChannelListItem[]).map((item) => item.id))
      .size !== items.length
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

export function newChannelStatusIdempotencyKey(
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  try {
    const uuid = source?.randomUUID();
    if (
      typeof uuid !== "string" ||
      !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
        uuid,
      )
    ) {
      return undefined;
    }
    return `channel-status:${uuid}`;
  } catch {
    return undefined;
  }
}

export async function updateChannelStatus(
  transport: ChannelsTransport,
  channelID: number,
  status: ChannelStatus,
  csrfToken: string,
  idempotencyKey: string,
): Promise<ChannelStatusUpdateResult> {
  if (
    !positiveInteger(channelID) ||
    !/^[A-Za-z0-9_-]{43}$/.test(csrfToken) ||
    (status !== "active" && status !== "inactive" && status !== "archived") ||
    idempotencyKey.length < 16 ||
    idempotencyKey.length > 128 ||
    idempotencyKey.trim() !== idempotencyKey
  ) {
    return { status: "invalid" };
  }

  let response: Awaited<ReturnType<ChannelsTransport["write"]>>;
  try {
    response = await transport.write(channelID, status, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": csrfToken,
        "Idempotency-Key": idempotencyKey,
      },
    });
  } catch {
    return { status: "unknown" };
  }

  if (response.status !== 200) {
    const result = failure(response.status, response.data);
    return result === "unauthenticated"
      ? { status: result }
      : { status: "unknown" };
  }

  // A successful mutation response includes an unfrozen legacy projection.
  // Do not read it; the strict local list is the only confirmation source.
  const reloaded = await loadChannels(transport);
  if (reloaded.status !== "loaded") {
    return reloaded.status === "unauthenticated"
      ? reloaded
      : { status: "unknown" };
  }
  return reloaded.items.some(
    (item) => item.id === channelID && item.status === status,
  )
    ? { status: "confirmed", items: reloaded.items }
    : { status: "unknown" };
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
