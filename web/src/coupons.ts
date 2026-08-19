import {
  archiveLegacyCoupon,
  copyLegacyCoupon,
  getLegacyCouponShare,
  listLegacyCouponClaims,
  listLegacyCoupons,
} from "./api/generated/health";

export type CouponsRole = "admin" | "ops" | "sales";
export type CouponAvailability =
  | "draft"
  | "scheduled"
  | "active"
  | "sold_out"
  | "ended"
  | "stopped"
  | "archived";
export type CouponAvailabilityFilter = "all" | CouponAvailability;
export type CouponStatus = "draft" | "published" | "stopped" | "archived";

export interface CouponListItem {
  readonly id: number;
  readonly name: string;
  readonly status: CouponStatus;
  readonly availability: CouponAvailability;
  readonly issuedCount: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface CouponClaimItem {
  readonly id: number;
  readonly claimRef: string;
  readonly claimedAt: string;
}

export interface CouponShare {
  readonly publicSlug: string;
  readonly url: string;
}

export function canArchiveCoupon(item: CouponListItem): boolean {
  return (
    item.status === "draft" ||
    item.status === "published" ||
    item.status === "stopped"
  );
}

export type CouponsFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";

export type CouponListResult =
  | { readonly status: "loaded"; readonly items: readonly CouponListItem[] }
  | { readonly status: CouponsFailure };
export type CouponCopyResult =
  | { readonly status: "copied"; readonly item: CouponListItem }
  | { readonly status: CouponsFailure };
export type CouponClaimsResult =
  | {
      readonly status: "loaded";
      readonly items: readonly CouponClaimItem[];
      readonly total: number;
      readonly offset: number;
    }
  | { readonly status: CouponsFailure };
export type CouponShareResult =
  | { readonly status: "loaded"; readonly share: CouponShare }
  | { readonly status: CouponsFailure };
export type CouponArchiveResult =
  | { readonly status: "archived"; readonly item: CouponListItem }
  | { readonly status: "canceled" }
  | { readonly status: CouponsFailure };

const couponPageSize = 200;
export const couponClaimsPageSize = 50;
const maximumCouponOffset = 1_000_000;

async function generatedList(
  params: Parameters<typeof listLegacyCoupons>[0],
  options?: RequestInit,
) {
  return listLegacyCoupons(params, { credentials: "same-origin", ...options });
}

async function generatedCopy(couponID: number, options?: RequestInit) {
  return copyLegacyCoupon(couponID, { credentials: "same-origin", ...options });
}

async function generatedClaims(
  couponID: number,
  params: Parameters<typeof listLegacyCouponClaims>[1],
  options?: RequestInit,
) {
  return listLegacyCouponClaims(couponID, params, {
    credentials: "same-origin",
    ...options,
  });
}

async function generatedShare(couponID: number, options?: RequestInit) {
  return getLegacyCouponShare(couponID, {
    credentials: "same-origin",
    ...options,
  });
}

async function generatedArchive(couponID: number, options?: RequestInit) {
  return archiveLegacyCoupon(couponID, {
    credentials: "same-origin",
    ...options,
  });
}

export interface CouponsTransport {
  readonly list: typeof generatedList;
  readonly copy: typeof generatedCopy;
  readonly claims: typeof generatedClaims;
  readonly share: typeof generatedShare;
  readonly archive: typeof generatedArchive;
}

export const generatedCouponsTransport: CouponsTransport = {
  list: generatedList,
  copy: generatedCopy,
  claims: generatedClaims,
  share: generatedShare,
  archive: generatedArchive,
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

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function text(value: unknown, maximum: number, empty = false): value is string {
  return (
    typeof value === "string" &&
    (empty || value.length > 0) &&
    [...value].length <= maximum &&
    !value.includes("\x00")
  );
}

function timestamp(value: unknown): value is string {
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

function availability(value: unknown): CouponAvailability | undefined {
  return value === "draft" ||
    value === "scheduled" ||
    value === "active" ||
    value === "sold_out" ||
    value === "ended" ||
    value === "stopped" ||
    value === "archived"
    ? value
    : undefined;
}

function targetRefs(value: unknown): value is readonly string[] {
  return (
    Array.isArray(value) &&
    value.length >= 1 &&
    value.length <= 100 &&
    value.every(
      (target) =>
        typeof target === "string" &&
        /^standard_product:[1-9][0-9]*$/.test(target),
    ) &&
    new Set(value).size === value.length
  );
}

function coupon(value: unknown): CouponListItem | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "id",
      "name",
      "discount_amount_total",
      "currency",
      "status",
      "availability_status",
      "total_issue_limit",
      "per_user_issue_limit",
      "issued_count",
      "claim_starts_at",
      "claim_ends_at",
      "validity_mode",
      "use_starts_at",
      "use_ends_at",
      "relative_validity_days",
      "instructions",
      "target_refs",
      "created_by",
      "updated_by",
      "version",
      "created_at",
      "updated_at",
    ])
  ) {
    return undefined;
  }
  const availabilityStatus = availability(value.availability_status);
  if (
    !positive(value.id) ||
    !text(value.name, 45) ||
    !positive(value.discount_amount_total) ||
    value.currency !== "CNY" ||
    (value.status !== "draft" &&
      value.status !== "published" &&
      value.status !== "stopped" &&
      value.status !== "archived") ||
    !availabilityStatus ||
    !positive(value.total_issue_limit) ||
    !positive(value.per_user_issue_limit) ||
    !nonnegative(value.issued_count) ||
    value.issued_count > value.total_issue_limit ||
    !timestamp(value.claim_starts_at) ||
    !timestamp(value.claim_ends_at) ||
    Date.parse(value.claim_starts_at) >= Date.parse(value.claim_ends_at) ||
    (value.validity_mode !== "fixed_range" &&
      value.validity_mode !== "relative_days") ||
    !text(value.instructions, 200, true) ||
    !targetRefs(value.target_refs) ||
    !positive(value.created_by) ||
    !positive(value.updated_by) ||
    !positive(value.version) ||
    !timestamp(value.created_at) ||
    !timestamp(value.updated_at)
  ) {
    return undefined;
  }
  if (value.validity_mode === "fixed_range") {
    if (
      !timestamp(value.use_starts_at) ||
      !timestamp(value.use_ends_at) ||
      Date.parse(value.use_starts_at) >= Date.parse(value.use_ends_at) ||
      value.relative_validity_days !== null
    ) {
      return undefined;
    }
  } else if (
    value.use_starts_at !== null ||
    value.use_ends_at !== null ||
    !positive(value.relative_validity_days) ||
    value.relative_validity_days > 36500
  ) {
    return undefined;
  }
  return {
    id: value.id,
    name: value.name,
    status: value.status,
    availability: availabilityStatus,
    issuedCount: value.issued_count,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

function claim(value: unknown): CouponClaimItem | undefined {
  if (
    !record(value) ||
    !exact(value, ["id", "claim_ref", "status", "claimed_at"]) ||
    !positive(value.id) ||
    !text(value.claim_ref, 67) ||
    !/^cp_[A-Za-z0-9_-]{16,64}$/.test(value.claim_ref) ||
    value.status !== "claimed" ||
    !timestamp(value.claimed_at)
  ) {
    return undefined;
  }
  return {
    id: value.id,
    claimRef: value.claim_ref,
    claimedAt: value.claimed_at,
  };
}

function sameItems(
  left: readonly CouponListItem[],
  right: readonly CouponListItem[],
): boolean {
  return (
    left.length === right.length &&
    left.every(
      (item, index) =>
        item.id === right[index]?.id &&
        item.name === right[index]?.name &&
        item.status === right[index]?.status &&
        item.availability === right[index]?.availability &&
        item.issuedCount === right[index]?.issuedCount &&
        item.createdAt === right[index]?.createdAt &&
        item.updatedAt === right[index]?.updatedAt,
    )
  );
}

function couponPage(
  value: unknown,
  offset: number,
):
  | { readonly items: readonly CouponListItem[]; readonly total: number }
  | undefined {
  if (
    !record(value) ||
    !exact(value, ["ok", "coupons", "items", "total", "limit", "offset"]) ||
    value.ok !== true ||
    !Array.isArray(value.coupons) ||
    !Array.isArray(value.items) ||
    !nonnegative(value.total) ||
    value.limit !== couponPageSize ||
    value.offset !== offset
  ) {
    return undefined;
  }
  const coupons = value.coupons.map(coupon);
  const items = value.items.map(coupon);
  if (
    coupons.includes(undefined) ||
    items.includes(undefined) ||
    !sameItems(
      coupons as readonly CouponListItem[],
      items as readonly CouponListItem[],
    ) ||
    value.total < items.length
  ) {
    return undefined;
  }
  return { items: items as readonly CouponListItem[], total: value.total };
}

function couponClaimsPage(
  value: unknown,
  offset: number,
):
  | { readonly items: readonly CouponClaimItem[]; readonly total: number }
  | undefined {
  if (
    !record(value) ||
    !exact(value, ["ok", "items", "total", "limit", "offset"]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    !nonnegative(value.total) ||
    value.limit !== couponClaimsPageSize ||
    value.offset !== offset ||
    offset % couponClaimsPageSize !== 0 ||
    value.items.length > couponClaimsPageSize ||
    value.total < offset + value.items.length ||
    (value.items.length < couponClaimsPageSize &&
      offset + value.items.length < value.total)
  ) {
    return undefined;
  }
  const items = value.items.map(claim);
  if (
    items.includes(undefined) ||
    new Set((items as readonly CouponClaimItem[]).map((item) => item.id))
      .size !== items.length ||
    new Set((items as readonly CouponClaimItem[]).map((item) => item.claimRef))
      .size !== items.length
  ) {
    return undefined;
  }
  return { items: items as readonly CouponClaimItem[], total: value.total };
}

function platformError(value: unknown, code: string): boolean {
  return (
    record(value) &&
    (exact(value, ["code", "message", "request_id"]) ||
      exact(value, ["code", "message", "request_id", "details"])) &&
    value.code === code &&
    text(value.message, 1000) &&
    text(value.request_id, 200) &&
    (value.details === undefined || Array.isArray(value.details))
  );
}

function compatibilityError(value: unknown): boolean {
  return (
    record(value) &&
    exact(value, ["ok", "detail", "message"]) &&
    value.ok === false &&
    text(value.detail, 1000) &&
    text(value.message, 1000)
  );
}

function failure(status: number, body: unknown): CouponsFailure {
  if (status === 401 && platformError(body, "UNAUTHENTICATED"))
    return "unauthenticated";
  if (status === 403 && platformError(body, "UNAUTHORIZED")) return "forbidden";
  if (
    status === 404 &&
    (compatibilityError(body) || platformError(body, "NOT_FOUND"))
  )
    return "not_found";
  if (
    status === 409 &&
    (compatibilityError(body) || platformError(body, "CONFLICT"))
  )
    return "conflict";
  if (
    status === 400 &&
    (compatibilityError(body) || platformError(body, "MALFORMED_REQUEST"))
  )
    return "invalid";
  if (
    status === 503 &&
    (compatibilityError(body) || platformError(body, "DEPENDENCY_UNAVAILABLE"))
  ) {
    return "unavailable";
  }
  return "invalid";
}

export async function loadCoupons(
  transport: CouponsTransport = generatedCouponsTransport,
): Promise<CouponListResult> {
  const items: CouponListItem[] = [];
  let offset = 0;
  let total: number | undefined;
  try {
    for (;;) {
      const response = await transport.list(
        { limit: couponPageSize, offset },
        { credentials: "same-origin" },
      );
      if (response.status !== 200)
        return { status: failure(response.status, response.data) };
      const page = couponPage(response.data, offset);
      if (!page || (total !== undefined && total !== page.total))
        return { status: "invalid" };
      total = page.total;
      items.push(...page.items);
      if (items.length === total) {
        return new Set(items.map((item) => item.id)).size === items.length
          ? { status: "loaded", items }
          : { status: "invalid" };
      }
      if (
        page.items.length === 0 ||
        items.length > total ||
        offset + page.items.length > maximumCouponOffset
      ) {
        return { status: "invalid" };
      }
      offset += page.items.length;
    }
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadCouponClaims(
  transport: CouponsTransport,
  couponID: number,
  offset: number,
): Promise<CouponClaimsResult> {
  if (
    !positive(couponID) ||
    !nonnegative(offset) ||
    offset % couponClaimsPageSize !== 0 ||
    offset > maximumCouponOffset
  ) {
    return { status: "invalid" };
  }
  let response: Awaited<ReturnType<CouponsTransport["claims"]>>;
  try {
    response = await transport.claims(
      couponID,
      { limit: couponClaimsPageSize, offset },
      { credentials: "same-origin" },
    );
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200)
    return { status: failure(response.status, response.data) };
  const page = couponClaimsPage(response.data, offset);
  return page ? { status: "loaded", ...page, offset } : { status: "invalid" };
}

export async function loadCouponShare(
  transport: CouponsTransport,
  item: CouponListItem,
): Promise<CouponShareResult> {
  if (!positive(item.id) || item.status !== "published")
    return { status: "invalid" };
  let response: Awaited<ReturnType<CouponsTransport["share"]>>;
  try {
    response = await transport.share(item.id, { credentials: "same-origin" });
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200)
    return { status: failure(response.status, response.data) };
  const data = response.data;
  const expectedSlug = `c-${item.id}`;
  const expectedURL = `/c/${expectedSlug}`;
  return record(data) &&
    exact(data, ["ok", "public_slug", "url"]) &&
    data.ok === true &&
    data.public_slug === expectedSlug &&
    data.url === expectedURL
    ? {
        status: "loaded",
        share: { publicSlug: expectedSlug, url: expectedURL },
      }
    : { status: "invalid" };
}

export function newCouponCopyIdempotencyKey(
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  return newCouponIdempotencyKey("coupon-copy", source);
}

export function newCouponArchiveIdempotencyKey(
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  return newCouponIdempotencyKey("coupon-archive", source);
}

function newCouponIdempotencyKey(
  prefix: string,
  source: { readonly randomUUID: () => string } | undefined,
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
    return `${prefix}:${uuid}`;
  } catch {
    return undefined;
  }
}

export async function copyCoupon(
  transport: CouponsTransport,
  couponID: number,
  csrf: string,
  idempotencyKey: string,
): Promise<CouponCopyResult> {
  if (
    !positive(couponID) ||
    !/^[A-Za-z0-9_-]{43}$/.test(csrf) ||
    idempotencyKey.length < 16 ||
    idempotencyKey.length > 128 ||
    idempotencyKey.trim() !== idempotencyKey
  ) {
    return { status: "invalid" };
  }
  let response: Awaited<ReturnType<CouponsTransport["copy"]>>;
  try {
    response = await transport.copy(couponID, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": csrf,
        "Idempotency-Key": idempotencyKey,
      },
    });
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200)
    return { status: failure(response.status, response.data) };
  if (
    !record(response.data) ||
    !exact(response.data, ["ok", "coupon"]) ||
    response.data.ok !== true
  ) {
    return { status: "invalid" };
  }
  const rawCoupon: unknown = response.data.coupon;
  const item = coupon(rawCoupon);
  const raw = record(rawCoupon) ? rawCoupon : undefined;
  return item &&
    raw &&
    item.id !== couponID &&
    item.availability === "draft" &&
    raw.status === "draft" &&
    raw.issued_count === 0
    ? { status: "copied", item }
    : { status: "invalid" };
}

export async function archiveCoupon(
  transport: CouponsTransport,
  item: CouponListItem,
  csrf: string,
  idempotencyKey: string,
): Promise<CouponArchiveResult> {
  if (
    !positive(item.id) ||
    !canArchiveCoupon(item) ||
    !/^[A-Za-z0-9_-]{43}$/.test(csrf) ||
    idempotencyKey.length < 16 ||
    idempotencyKey.length > 128 ||
    idempotencyKey.trim() !== idempotencyKey
  ) {
    return { status: "invalid" };
  }
  let response: Awaited<ReturnType<CouponsTransport["archive"]>>;
  try {
    response = await transport.archive(item.id, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": csrf,
        "Idempotency-Key": idempotencyKey,
      },
    });
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200)
    return { status: failure(response.status, response.data) };
  if (
    !record(response.data) ||
    !exact(response.data, ["ok", "coupon"]) ||
    response.data.ok !== true
  ) {
    return { status: "invalid" };
  }
  const rawCoupon: unknown = response.data.coupon;
  const archived = coupon(rawCoupon);
  return archived &&
    archived.id === item.id &&
    archived.status === "archived" &&
    archived.issuedCount === item.issuedCount
    ? { status: "archived", item: archived }
    : { status: "invalid" };
}

export function filterCoupons(
  items: readonly CouponListItem[],
  keyword: string,
  status: CouponAvailabilityFilter,
): readonly CouponListItem[] {
  const query = keyword.trim().toLocaleLowerCase();
  return items.filter(
    (item) =>
      (status === "all" || item.availability === status) &&
      (query === "" || item.name.toLocaleLowerCase().includes(query)),
  );
}
