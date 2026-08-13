import {
  listCustomers,
  type ListCustomersParams,
} from "./api/generated/health";

export type CustomerRole = "admin" | "ops" | "sales";

export interface CustomerRecord {
  readonly id: number;
  readonly name: string;
  readonly stageID?: number | null;
  readonly ownerStaffID?: number | null;
  readonly channelID?: number | null;
  readonly addedAt?: string | null;
  readonly lastInteractAt?: string | null;
  readonly isDeleted: boolean;
}

export interface CustomerListPage {
  readonly items: readonly CustomerRecord[];
  /** Opaque server-issued keyset cursor. This module never decodes it. */
  readonly nextCursor?: string;
  readonly total: number;
  readonly totalIsEstimate: boolean;
  readonly watermark: string;
}

/** A server response retained locally so users can return to an earlier page. */
export interface CustomerListHistoryEntry {
  readonly requestCursor?: string;
  readonly page: CustomerListPage;
}

export interface CustomerListFilters {
  readonly limit: number;
  readonly keyword?: string;
  readonly ownerStaffID?: number;
  readonly stageID?: number;
  readonly channelID?: number;
  readonly tagID?: number;
  readonly isDeleted: boolean;
  readonly addedAfter?: string;
  readonly addedBefore?: string;
  readonly lastInteractAfter?: string;
  readonly lastInteractBefore?: string;
}

export interface CustomerListFilterDraft {
  readonly limit: string;
  readonly keyword: string;
  readonly ownerStaffID: string;
  readonly stageID: string;
  readonly channelID: string;
  readonly tagID: string;
  readonly isDeleted: boolean;
  readonly addedAfter: string;
  readonly addedBefore: string;
  readonly lastInteractAfter: string;
  readonly lastInteractBefore: string;
}

export const defaultCustomerListFilterDraft: CustomerListFilterDraft = {
  limit: "50",
  keyword: "",
  ownerStaffID: "",
  stageID: "",
  channelID: "",
  tagID: "",
  isDeleted: false,
  addedAfter: "",
  addedBefore: "",
  lastInteractAfter: "",
  lastInteractBefore: "",
};

export type CustomerFilterParseResult =
  | { readonly ok: true; readonly filters: CustomerListFilters }
  | { readonly ok: false };

export interface CustomerTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function loadGeneratedCustomers(
  params: ListCustomersParams,
  options: RequestInit,
): Promise<CustomerTransportResponse> {
  return listCustomers(params, options);
}

export type CustomerTransport = {
  list: typeof loadGeneratedCustomers;
};

export const generatedCustomerTransport: CustomerTransport = {
  list: loadGeneratedCustomers,
};

export type CustomerLoadFailure =
  "unauthenticated" | "forbidden" | "invalid" | "unavailable";

export type CustomerLoadResult =
  | { readonly status: "loaded"; readonly page: CustomerListPage }
  | { readonly status: CustomerLoadFailure };

const maximumKeywordLength = 200;
const maximumPageSize = 200;

function plainRecord(value: unknown): value is Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function hasExpectedKeys(
  value: Record<string, unknown>,
  required: readonly string[],
  optional: readonly string[],
): boolean {
  const allowed = new Set([...required, ...optional]);
  return (
    required.every((key) => Object.hasOwn(value, key)) &&
    Object.keys(value).every((key) => allowed.has(key))
  );
}

function positiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 1;
}

function safeNonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function validDateTime(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) &&
    !Number.isNaN(Date.parse(value))
  );
}

function validAbsoluteURI(value: unknown): boolean {
  if (typeof value !== "string" || value.length === 0 || value !== value.trim())
    return false;
  try {
    const parsed = new URL(value);
    return (
      (parsed.protocol === "https:" || parsed.protocol === "http:") &&
      parsed.host.length > 0 &&
      parsed.username === "" &&
      parsed.password === ""
    );
  } catch {
    return false;
  }
}

function validInt32(value: unknown): boolean {
  return (
    typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value >= -2_147_483_648 &&
    value <= 2_147_483_647
  );
}

function optionalID(value: unknown): boolean {
  return value === undefined || value === null || positiveSafeInteger(value);
}
function optionalDateTime(value: unknown): boolean {
  return value === undefined || value === null || validDateTime(value);
}
function optionalAvatar(value: unknown): boolean {
  return value === undefined || value === null || validAbsoluteURI(value);
}
function optionalGender(value: unknown): boolean {
  return value === undefined || value === null || validInt32(value);
}

export function parseCustomer(value: unknown): CustomerRecord | undefined {
  if (!plainRecord(value)) return undefined;
  if (
    !hasExpectedKeys(
      value,
      ["id", "name", "is_deleted", "extra", "created_at", "updated_at"],
      [
        "avatar_url",
        "gender",
        "stage_id",
        "owner_staff_id",
        "channel_id",
        "added_at",
        "last_interact_at",
      ],
    )
  ) {
    return undefined;
  }
  const timestampsAreOrdered =
    validDateTime(value.created_at) &&
    validDateTime(value.updated_at) &&
    Date.parse(value.created_at) <= Date.parse(value.updated_at);
  if (
    !positiveSafeInteger(value.id) ||
    typeof value.name !== "string" ||
    typeof value.is_deleted !== "boolean" ||
    !optionalAvatar(value.avatar_url) ||
    !optionalGender(value.gender) ||
    !plainRecord(value.extra) ||
    !timestampsAreOrdered ||
    !optionalID(value.stage_id) ||
    !optionalID(value.owner_staff_id) ||
    !optionalID(value.channel_id) ||
    !optionalDateTime(value.added_at) ||
    !optionalDateTime(value.last_interact_at)
  ) {
    return undefined;
  }

  return {
    id: value.id,
    name: value.name,
    stageID: value.stage_id as number | null | undefined,
    ownerStaffID: value.owner_staff_id as number | null | undefined,
    channelID: value.channel_id as number | null | undefined,
    addedAt: value.added_at as string | null | undefined,
    lastInteractAt: value.last_interact_at as string | null | undefined,
    isDeleted: value.is_deleted,
  };
}

export function parseCustomerListPage(
  value: unknown,
): CustomerListPage | undefined {
  if (!plainRecord(value)) return undefined;
  if (
    !hasExpectedKeys(
      value,
      ["items", "next_cursor", "total", "total_is_estimate", "watermark"],
      [],
    )
  ) {
    return undefined;
  }
  if (
    !Array.isArray(value.items) ||
    !safeNonNegativeInteger(value.total) ||
    value.total > 10_000 ||
    typeof value.total_is_estimate !== "boolean" ||
    (value.total_is_estimate && value.total !== 10_000) ||
    !validDateTime(value.watermark) ||
    (value.next_cursor !== null &&
      (typeof value.next_cursor !== "string" || value.next_cursor.length === 0))
  ) {
    return undefined;
  }

  const items: CustomerRecord[] = [];
  const customerIDs = new Set<number>();
  for (const item of value.items) {
    const parsed = parseCustomer(item);
    if (!parsed || customerIDs.has(parsed.id)) return undefined;
    customerIDs.add(parsed.id);
    items.push(parsed);
  }
  if (
    value.total < items.length ||
    (value.next_cursor !== null && items.length === 0)
  ) {
    return undefined;
  }

  return {
    items,
    nextCursor: value.next_cursor ?? undefined,
    total: value.total,
    totalIsEstimate: value.total_is_estimate,
    watermark: value.watermark,
  };
}

export function appendCustomerListPage(
  current: CustomerListPage,
  next: CustomerListPage,
): CustomerListPage | undefined {
  if (current.watermark !== next.watermark) return undefined;
  const customerIDs = new Set(current.items.map((item) => item.id));
  if (next.items.some((item) => customerIDs.has(item.id))) return undefined;
  return { ...next, items: [...current.items, ...next.items] };
}

/**
 * Adds a server-issued next page to local navigation history. The cursor is
 * compared literally and remains opaque; the client never derives a cursor
 * for a previous page.
 */
export function appendCustomerListHistoryPage(
  history: readonly CustomerListHistoryEntry[],
  requestCursor: string,
  next: CustomerListPage,
): readonly CustomerListHistoryEntry[] | undefined {
  const current = history.at(-1);
  if (!current || current.page.nextCursor !== requestCursor) return undefined;

  const historicalItems = history.flatMap((entry) => entry.page.items);
  const merged = appendCustomerListPage(
    { ...current.page, items: historicalItems },
    next,
  );
  if (!merged) return undefined;

  return [...history, { requestCursor, page: next }];
}

function optionalPositiveInteger(value: unknown): number | null | undefined {
  if (typeof value !== "string") return null;
  const normalized = value.trim();
  if (normalized === "") return undefined;
  if (!/^[1-9][0-9]*$/.test(normalized)) return null;
  const parsed = Number(normalized);
  return Number.isSafeInteger(parsed) ? parsed : null;
}

function parsePageSize(value: unknown): number | undefined {
  const parsed = optionalPositiveInteger(value);
  if (parsed === undefined || parsed === null || parsed > maximumPageSize) {
    return undefined;
  }
  return parsed;
}

function optionalLocalDateTime(value: unknown): string | null | undefined {
  if (typeof value !== "string") return null;
  const normalized = value.trim();
  if (normalized === "") return undefined;
  const timestamp = Date.parse(normalized);
  if (Number.isNaN(timestamp)) return null;
  return new Date(timestamp).toISOString();
}

/**
 * Converts only form input into the frozen HTTP query contract. Cursor is
 * intentionally excluded: it can only come from a previous server response.
 */
export function parseCustomerListFilters(
  draft: CustomerListFilterDraft,
): CustomerFilterParseResult {
  const limit = parsePageSize(draft.limit);
  const ownerStaffID = optionalPositiveInteger(draft.ownerStaffID);
  const stageID = optionalPositiveInteger(draft.stageID);
  const channelID = optionalPositiveInteger(draft.channelID);
  const tagID = optionalPositiveInteger(draft.tagID);
  const addedAfter = optionalLocalDateTime(draft.addedAfter);
  const addedBefore = optionalLocalDateTime(draft.addedBefore);
  const lastInteractAfter = optionalLocalDateTime(draft.lastInteractAfter);
  const lastInteractBefore = optionalLocalDateTime(draft.lastInteractBefore);
  const keyword =
    typeof draft.keyword === "string" ? draft.keyword.trim() : undefined;

  if (
    !limit ||
    ownerStaffID === null ||
    stageID === null ||
    channelID === null ||
    tagID === null ||
    addedAfter === null ||
    addedBefore === null ||
    lastInteractAfter === null ||
    lastInteractBefore === null ||
    keyword === undefined ||
    typeof draft.isDeleted !== "boolean" ||
    Array.from(keyword).length > maximumKeywordLength ||
    (addedAfter && addedBefore && addedAfter > addedBefore) ||
    (lastInteractAfter &&
      lastInteractBefore &&
      lastInteractAfter > lastInteractBefore)
  ) {
    return { ok: false };
  }

  return {
    ok: true,
    filters: {
      limit,
      ...(keyword === "" ? {} : { keyword }),
      ...(ownerStaffID === undefined ? {} : { ownerStaffID }),
      ...(stageID === undefined ? {} : { stageID }),
      ...(channelID === undefined ? {} : { channelID }),
      ...(tagID === undefined ? {} : { tagID }),
      isDeleted: draft.isDeleted,
      ...(addedAfter === undefined ? {} : { addedAfter }),
      ...(addedBefore === undefined ? {} : { addedBefore }),
      ...(lastInteractAfter === undefined ? {} : { lastInteractAfter }),
      ...(lastInteractBefore === undefined ? {} : { lastInteractBefore }),
    },
  };
}

export function customerListParams(
  filters: CustomerListFilters,
  cursor?: string,
): ListCustomersParams {
  return {
    limit: filters.limit,
    is_deleted: filters.isDeleted,
    ...(filters.keyword === undefined ? {} : { keyword: filters.keyword }),
    ...(filters.ownerStaffID === undefined
      ? {}
      : { owner_staff_id: filters.ownerStaffID }),
    ...(filters.stageID === undefined ? {} : { stage_id: filters.stageID }),
    ...(filters.channelID === undefined
      ? {}
      : { channel_id: filters.channelID }),
    ...(filters.tagID === undefined ? {} : { tag_id: filters.tagID }),
    ...(filters.addedAfter === undefined
      ? {}
      : { added_after: filters.addedAfter }),
    ...(filters.addedBefore === undefined
      ? {}
      : { added_before: filters.addedBefore }),
    ...(filters.lastInteractAfter === undefined
      ? {}
      : { last_interact_after: filters.lastInteractAfter }),
    ...(filters.lastInteractBefore === undefined
      ? {}
      : { last_interact_before: filters.lastInteractBefore }),
    ...(cursor === undefined ? {} : { cursor }),
  };
}

export async function loadCustomers(
  transport: CustomerTransport,
  filters: CustomerListFilters,
  cursor?: string,
): Promise<CustomerLoadResult> {
  try {
    const response = await transport.list(customerListParams(filters, cursor), {
      credentials: "same-origin",
    });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 400) return { status: "invalid" };
    if (response.status !== 200) return { status: "unavailable" };

    const page = parseCustomerListPage(response.data);
    return page ? { status: "loaded", page } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
