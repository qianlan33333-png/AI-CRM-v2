import {
  getCustomerContext,
  type GetCustomerContextParams,
} from "./api/generated/health";

export interface CustomerContextCustomer {
  readonly id: number;
  readonly name: string;
  readonly stageID?: number;
  readonly ownerStaffID?: number;
  readonly channelID?: number;
  readonly addedAt?: string;
  readonly lastInteractAt?: string;
}

export interface CustomerContextTag {
  readonly id: number;
  readonly groupName?: string;
  readonly groupSortOrder: number;
  readonly name: string;
  readonly sortOrder: number;
}

export interface CustomerContextTimelineEntry {
  readonly id: number;
  readonly eventType: string;
  readonly occurredAt: string;
}

export interface CustomerContextChatEntry {
  readonly chatType: "private" | "group";
  readonly messageType: string;
  readonly sentAt: string;
}

export interface CustomerContextSnapshot {
  readonly customer: CustomerContextCustomer;
  readonly tags: readonly CustomerContextTag[];
  readonly timeline: readonly CustomerContextTimelineEntry[];
  readonly timelineNextCursor?: string;
  readonly chat: {
    readonly localArchiveAvailable: boolean;
    readonly items: readonly CustomerContextChatEntry[];
    readonly total: number;
  };
}

export interface CustomerContextTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function loadGeneratedCustomerContext(
  customerID: number,
  params: GetCustomerContextParams,
  options: RequestInit,
): Promise<CustomerContextTransportResponse> {
  const response = await getCustomerContext(customerID, params, options);
  return { status: response.status, data: response.data };
}

export interface CustomerContextTransport {
  readonly get: typeof loadGeneratedCustomerContext;
}

export const generatedCustomerContextTransport: CustomerContextTransport = {
  get: loadGeneratedCustomerContext,
};

export type CustomerContextLoadResult =
  | { readonly status: "loaded"; readonly snapshot: CustomerContextSnapshot }
  | { readonly status: "unauthenticated" }
  | { readonly status: "forbidden" }
  | { readonly status: "not_found" }
  | { readonly status: "unavailable" };

export type CustomerContextTimelineLoadResult =
  | {
      readonly status: "loaded";
      readonly timeline: readonly CustomerContextTimelineEntry[];
      readonly nextCursor?: string;
    }
  | Exclude<CustomerContextLoadResult, { readonly status: "loaded" }>;

const SAME_ORIGIN: RequestInit = { credentials: "same-origin" };
export const CUSTOMER_CONTEXT_TIMELINE_PAGE_SIZE = 50;
const RFC3339_TIMESTAMP_PATTERN =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

function plainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function exactObject(
  value: unknown,
  required: readonly string[],
  allowed: readonly string[],
): value is Record<string, unknown> {
  if (!plainRecord(value)) return false;
  const keys = Object.keys(value);
  return (
    required.every((key) => Object.hasOwn(value, key)) &&
    keys.every((key) => allowed.includes(key))
  );
}

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function int32(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value >= -2_147_483_648 &&
    value <= 2_147_483_647
  );
}

function isCalendarDate(year: number, month: number, day: number): boolean {
  if (month < 1 || month > 12 || day < 1) return false;
  const days = [
    31,
    year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28,
    31,
    30,
    31,
    30,
    31,
    31,
    30,
    31,
    30,
    31,
  ];
  return day <= days[month - 1];
}

export function isStrictRFC3339Timestamp(value: unknown): value is string {
  if (typeof value !== "string" || value.length === 0) return false;
  const match = RFC3339_TIMESTAMP_PATTERN.exec(value);
  if (!match) return false;
  const [, year, month, day, hour, minute, second] = match.map(Number);
  return (
    isCalendarDate(year, month, day) &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59 &&
    Number.isFinite(Date.parse(value))
  );
}

function optionalPositiveInteger(
  value: unknown,
): value is number | null | undefined {
  return value === undefined || value === null || positiveInteger(value);
}

function optionalTimestamp(value: unknown): value is string | null | undefined {
  return (
    value === undefined || value === null || isStrictRFC3339Timestamp(value)
  );
}

function optionalString(value: unknown): value is string | null | undefined {
  return value === undefined || value === null || typeof value === "string";
}

function safeText(value: unknown, maximum: number): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    [...value].length <= maximum &&
    value === value.trim()
  );
}

function parseCustomer(
  value: unknown,
  customerID: number,
): CustomerContextCustomer | undefined {
  const required = ["id", "name"];
  const allowed = [
    ...required,
    "stage_id",
    "owner_staff_id",
    "channel_id",
    "added_at",
    "last_interact_at",
  ];
  if (
    !exactObject(value, required, allowed) ||
    !positiveInteger(value.id) ||
    value.id !== customerID ||
    typeof value.name !== "string" ||
    !optionalPositiveInteger(value.stage_id) ||
    !optionalPositiveInteger(value.owner_staff_id) ||
    !optionalPositiveInteger(value.channel_id) ||
    !optionalTimestamp(value.added_at) ||
    !optionalTimestamp(value.last_interact_at)
  )
    return undefined;
  return {
    id: value.id,
    name: value.name,
    ...(typeof value.stage_id === "number" ? { stageID: value.stage_id } : {}),
    ...(typeof value.owner_staff_id === "number"
      ? { ownerStaffID: value.owner_staff_id }
      : {}),
    ...(typeof value.channel_id === "number"
      ? { channelID: value.channel_id }
      : {}),
    ...(typeof value.added_at === "string" ? { addedAt: value.added_at } : {}),
    ...(typeof value.last_interact_at === "string"
      ? { lastInteractAt: value.last_interact_at }
      : {}),
  };
}

function parseTags(value: unknown): readonly CustomerContextTag[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const seen = new Set<number>();
  const tags: CustomerContextTag[] = [];
  for (const item of value) {
    const required = ["id", "name", "group_sort_order", "sort_order"];
    const allowed = [...required, "group_id", "group_name"];
    if (
      !exactObject(item, required, allowed) ||
      !positiveInteger(item.id) ||
      !safeText(item.name, 200) ||
      !int32(item.group_sort_order) ||
      !int32(item.sort_order) ||
      !optionalPositiveInteger(item.group_id) ||
      !optionalString(item.group_name) ||
      seen.has(item.id)
    )
      return undefined;
    const hasGroupID = typeof item.group_id === "number";
    const hasGroupName = typeof item.group_name === "string";
    if (
      hasGroupID !== hasGroupName ||
      (hasGroupName && !safeText(item.group_name, 200))
    )
      return undefined;
    seen.add(item.id);
    tags.push({
      id: item.id,
      name: item.name,
      groupSortOrder: item.group_sort_order,
      sortOrder: item.sort_order,
      ...(typeof item.group_name === "string"
        ? { groupName: item.group_name }
        : {}),
    });
  }
  return tags;
}

function parseTimeline(
  value: unknown,
): readonly CustomerContextTimelineEntry[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const seen = new Set<number>();
  const timeline: CustomerContextTimelineEntry[] = [];
  for (const item of value) {
    if (
      !exactObject(
        item,
        ["id", "event_type", "occurred_at"],
        ["id", "event_type", "occurred_at"],
      ) ||
      !positiveInteger(item.id) ||
      !safeText(item.event_type, 200) ||
      !isStrictRFC3339Timestamp(item.occurred_at) ||
      seen.has(item.id)
    )
      return undefined;
    seen.add(item.id);
    timeline.push({
      id: item.id,
      eventType: item.event_type,
      occurredAt: item.occurred_at,
    });
  }
  return timeline;
}

function parseChat(
  value: unknown,
): CustomerContextSnapshot["chat"] | undefined {
  if (
    !exactObject(
      value,
      ["local_archive_available", "items", "total"],
      ["local_archive_available", "items", "total"],
    ) ||
    typeof value.local_archive_available !== "boolean" ||
    !Array.isArray(value.items) ||
    value.items.length > 20 ||
    typeof value.total !== "number" ||
    !Number.isSafeInteger(value.total) ||
    value.total < value.items.length
  )
    return undefined;
  const items: CustomerContextChatEntry[] = [];
  for (const item of value.items) {
    if (
      !exactObject(
        item,
        ["chat_type", "message_type", "sent_at"],
        ["chat_type", "message_type", "sent_at"],
      ) ||
      (item.chat_type !== "private" && item.chat_type !== "group") ||
      !safeText(item.message_type, 200) ||
      !isStrictRFC3339Timestamp(item.sent_at)
    )
      return undefined;
    items.push({
      chatType: item.chat_type,
      messageType: item.message_type,
      sentAt: item.sent_at,
    });
  }
  if (
    !value.local_archive_available &&
    (items.length !== 0 || value.total !== 0)
  )
    return undefined;
  return {
    localArchiveAvailable: value.local_archive_available,
    items,
    total: value.total,
  };
}

export function parseCustomerContext(
  value: unknown,
  customerID: number,
): CustomerContextSnapshot | undefined {
  if (!positiveInteger(customerID)) return undefined;
  const required = [
    "customer",
    "tags",
    "timeline",
    "timeline_next_cursor",
    "chat",
    "non_atomic_snapshot",
    "real_external_call_executed",
  ];
  if (
    !exactObject(value, required, required) ||
    value.non_atomic_snapshot !== true ||
    value.real_external_call_executed !== false ||
    (value.timeline_next_cursor !== null &&
      (typeof value.timeline_next_cursor !== "string" ||
        value.timeline_next_cursor.length === 0 ||
        value.timeline_next_cursor.length > 512))
  )
    return undefined;
  const customer = parseCustomer(value.customer, customerID);
  const tags = parseTags(value.tags);
  const timeline = parseTimeline(value.timeline);
  const chat = parseChat(value.chat);
  if (
    !customer ||
    !tags ||
    !timeline ||
    !chat ||
    (typeof value.timeline_next_cursor === "string" && timeline.length === 0)
  )
    return undefined;
  return {
    customer,
    tags,
    timeline,
    ...(typeof value.timeline_next_cursor === "string"
      ? { timelineNextCursor: value.timeline_next_cursor }
      : {}),
    chat,
  };
}

function failure(
  status: number,
): Exclude<CustomerContextLoadResult, { readonly status: "loaded" }> {
  if (status === 401) return { status: "unauthenticated" };
  if (status === 403) return { status: "forbidden" };
  if (status === 404) return { status: "not_found" };
  return { status: "unavailable" };
}

export async function loadCustomerContext(
  transport: CustomerContextTransport,
  customerID: number,
): Promise<CustomerContextLoadResult> {
  if (!positiveInteger(customerID)) return { status: "unavailable" };
  try {
    const response = await transport.get(
      customerID,
      { limit: CUSTOMER_CONTEXT_TIMELINE_PAGE_SIZE },
      SAME_ORIGIN,
    );
    if (response.status !== 200) return failure(response.status);
    const snapshot = parseCustomerContext(response.data, customerID);
    return snapshot
      ? { status: "loaded", snapshot }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadCustomerContextTimelinePage(
  transport: CustomerContextTransport,
  customerID: number,
  cursor: string,
  knownIDs: ReadonlySet<number>,
): Promise<CustomerContextTimelineLoadResult> {
  if (
    !positiveInteger(customerID) ||
    typeof cursor !== "string" ||
    cursor.length === 0 ||
    cursor.length > 512 ||
    [...knownIDs].some((id) => !positiveInteger(id))
  )
    return { status: "unavailable" };
  try {
    const response = await transport.get(
      customerID,
      { cursor, limit: CUSTOMER_CONTEXT_TIMELINE_PAGE_SIZE },
      SAME_ORIGIN,
    );
    if (response.status !== 200) return failure(response.status);
    const parsed = parseCustomerContext(response.data, customerID);
    if (!parsed || parsed.timeline.some((entry) => knownIDs.has(entry.id)))
      return { status: "unavailable" };
    return {
      status: "loaded",
      timeline: parsed.timeline,
      ...(parsed.timelineNextCursor
        ? { nextCursor: parsed.timelineNextCursor }
        : {}),
    };
  } catch {
    return { status: "unavailable" };
  }
}
