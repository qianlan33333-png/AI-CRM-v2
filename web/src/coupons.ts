import {
  archiveLegacyCoupon,
  copyLegacyCoupon,
  createLegacyCoupon,
  getLegacyCoupon,
  getLegacyCouponShare,
  listLegacyCouponClaims,
  listLegacyCouponProductOptions,
  listLegacyCoupons,
  updateLegacyCoupon,
  type CouponUpsertRequest,
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

export interface CouponRuleDetail extends CouponListItem {
  readonly discountAmountTotal: number;
  readonly currency: "CNY";
  readonly totalIssueLimit: number;
  readonly perUserIssueLimit: number;
  readonly claimStartsAt: string;
  readonly claimEndsAt: string;
  readonly validityMode: "fixed_range" | "relative_days";
  readonly useStartsAt?: string;
  readonly useEndsAt?: string;
  readonly relativeValidityDays?: number;
  readonly instructions: string;
  readonly targetRefs: readonly string[];
  readonly createdBy: number;
  readonly updatedBy: number;
  readonly version: number;
}

export interface CouponDraftInput {
  readonly name: string;
  readonly discountAmountTotal: string;
  readonly totalIssueLimit: string;
  readonly perUserIssueLimit: string;
  readonly claimStartsAt: string;
  readonly claimEndsAt: string;
  readonly validityMode: "fixed_range" | "relative_days";
  readonly useStartsAt: string;
  readonly useEndsAt: string;
  readonly relativeValidityDays: string;
  readonly instructions: string;
  readonly targetRefs: string;
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

export interface CouponProductOption {
  readonly id: number;
  readonly targetRef: string;
  readonly name: string;
  readonly priceMinor: number;
  readonly currency: "CNY";
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
export type CouponProductOptionsResult =
  | {
      readonly status: "loaded";
      readonly items: readonly CouponProductOption[];
      readonly total: number;
      readonly offset: number;
    }
  | { readonly status: CouponsFailure };
export type CouponShareResult =
  | { readonly status: "loaded"; readonly share: CouponShare }
  | { readonly status: CouponsFailure };
export type CouponDetailResult =
  | { readonly status: "loaded"; readonly detail: CouponRuleDetail }
  | { readonly status: CouponsFailure };
export type CouponDraftMutationResult =
  | { readonly status: "created"; readonly item: CouponRuleDetail }
  | { readonly status: "updated"; readonly item: CouponRuleDetail }
  | {
      readonly status: CouponsFailure;
      readonly outcomeUncertain: boolean;
    };
export type CouponArchiveResult =
  | { readonly status: "archived"; readonly item: CouponListItem }
  | { readonly status: "canceled" }
  | { readonly status: CouponsFailure };

const couponPageSize = 200;
export const couponClaimsPageSize = 50;
export const couponProductOptionsPageSize = 20;
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

async function generatedProductOptions(
  params: Parameters<typeof listLegacyCouponProductOptions>[0],
  options?: RequestInit,
) {
  return listLegacyCouponProductOptions(params, {
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

async function generatedDetail(couponID: number, options?: RequestInit) {
  return getLegacyCoupon(couponID, { credentials: "same-origin", ...options });
}

async function generatedCreate(
  input: CouponUpsertRequest,
  options?: RequestInit,
) {
  return createLegacyCoupon(input, { credentials: "same-origin", ...options });
}

async function generatedUpdate(
  couponID: number,
  input: CouponUpsertRequest,
  options?: RequestInit,
) {
  return updateLegacyCoupon(couponID, input, {
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
  readonly productOptions: typeof generatedProductOptions;
  readonly detail: typeof generatedDetail;
  readonly create: typeof generatedCreate;
  readonly update: typeof generatedUpdate;
  readonly share: typeof generatedShare;
  readonly archive: typeof generatedArchive;
}

export const generatedCouponsTransport: CouponsTransport = {
  list: generatedList,
  copy: generatedCopy,
  claims: generatedClaims,
  productOptions: generatedProductOptions,
  detail: generatedDetail,
  create: generatedCreate,
  update: generatedUpdate,
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

function couponDetail(value: unknown): CouponRuleDetail | undefined {
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
  const base: CouponListItem = {
    id: value.id,
    name: value.name,
    status: value.status as CouponStatus,
    availability: availabilityStatus,
    issuedCount: value.issued_count,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
  if (value.validity_mode === "fixed_range") {
    return {
      ...base,
      discountAmountTotal: value.discount_amount_total,
      currency: "CNY",
      totalIssueLimit: value.total_issue_limit,
      perUserIssueLimit: value.per_user_issue_limit,
      claimStartsAt: value.claim_starts_at,
      claimEndsAt: value.claim_ends_at,
      validityMode: "fixed_range",
      useStartsAt: value.use_starts_at as string,
      useEndsAt: value.use_ends_at as string,
      instructions: value.instructions,
      targetRefs: [...value.target_refs],
      createdBy: value.created_by,
      updatedBy: value.updated_by,
      version: value.version,
    };
  }
  return {
    ...base,
    discountAmountTotal: value.discount_amount_total,
    currency: "CNY",
    totalIssueLimit: value.total_issue_limit,
    perUserIssueLimit: value.per_user_issue_limit,
    claimStartsAt: value.claim_starts_at,
    claimEndsAt: value.claim_ends_at,
    validityMode: "relative_days",
    relativeValidityDays: value.relative_validity_days as number,
    instructions: value.instructions,
    targetRefs: [...value.target_refs],
    createdBy: value.created_by,
    updatedBy: value.updated_by,
    version: value.version,
  };
}

function coupon(value: unknown): CouponListItem | undefined {
  const detail = couponDetail(value);
  return detail
    ? {
        id: detail.id,
        name: detail.name,
        status: detail.status,
        availability: detail.availability,
        issuedCount: detail.issuedCount,
        createdAt: detail.createdAt,
        updatedAt: detail.updatedAt,
      }
    : undefined;
}

function safePositiveInteger(value: string): number | undefined {
  if (!/^[1-9][0-9]*$/.test(value)) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}

export function couponUpsertRequest(
  input: CouponDraftInput,
): CouponUpsertRequest | undefined {
  const name = input.name.trim();
  const instructions = input.instructions.trim();
  const discountAmountTotal = safePositiveInteger(input.discountAmountTotal);
  const totalIssueLimit = safePositiveInteger(input.totalIssueLimit);
  const perUserIssueLimit = safePositiveInteger(input.perUserIssueLimit);
  const targets = input.targetRefs.split("\n");
  if (
    !text(name, 45) ||
    !discountAmountTotal ||
    !totalIssueLimit ||
    !perUserIssueLimit ||
    perUserIssueLimit > totalIssueLimit ||
    !timestamp(input.claimStartsAt) ||
    !timestamp(input.claimEndsAt) ||
    Date.parse(input.claimStartsAt) >= Date.parse(input.claimEndsAt) ||
    !text(instructions, 200, true) ||
    !targetRefs(targets)
  ) {
    return undefined;
  }
  if (input.validityMode === "fixed_range") {
    if (
      !timestamp(input.useStartsAt) ||
      !timestamp(input.useEndsAt) ||
      Date.parse(input.useStartsAt) >= Date.parse(input.useEndsAt)
    ) {
      return undefined;
    }
    return {
      name,
      discount_amount_total: discountAmountTotal,
      total_issue_limit: totalIssueLimit,
      per_user_issue_limit: perUserIssueLimit,
      claim_starts_at: input.claimStartsAt,
      claim_ends_at: input.claimEndsAt,
      validity_mode: "fixed_range",
      use_starts_at: input.useStartsAt,
      use_ends_at: input.useEndsAt,
      relative_validity_days: null,
      instructions,
      target_refs: targets,
    };
  }
  const relativeValidityDays = safePositiveInteger(input.relativeValidityDays);
  if (
    input.validityMode !== "relative_days" ||
    !relativeValidityDays ||
    relativeValidityDays > 36500
  ) {
    return undefined;
  }
  return {
    name,
    discount_amount_total: discountAmountTotal,
    total_issue_limit: totalIssueLimit,
    per_user_issue_limit: perUserIssueLimit,
    claim_starts_at: input.claimStartsAt,
    claim_ends_at: input.claimEndsAt,
    validity_mode: "relative_days",
    use_starts_at: null,
    use_ends_at: null,
    relative_validity_days: relativeValidityDays,
    instructions,
    target_refs: targets,
  };
}

function sameCouponDetail(
  left: CouponRuleDetail,
  right: CouponRuleDetail,
): boolean {
  return (
    left.id === right.id &&
    left.name === right.name &&
    left.status === right.status &&
    left.availability === right.availability &&
    left.issuedCount === right.issuedCount &&
    left.createdAt === right.createdAt &&
    left.updatedAt === right.updatedAt &&
    left.discountAmountTotal === right.discountAmountTotal &&
    left.currency === right.currency &&
    left.totalIssueLimit === right.totalIssueLimit &&
    left.perUserIssueLimit === right.perUserIssueLimit &&
    left.claimStartsAt === right.claimStartsAt &&
    left.claimEndsAt === right.claimEndsAt &&
    left.validityMode === right.validityMode &&
    left.useStartsAt === right.useStartsAt &&
    left.useEndsAt === right.useEndsAt &&
    left.relativeValidityDays === right.relativeValidityDays &&
    left.instructions === right.instructions &&
    left.createdBy === right.createdBy &&
    left.updatedBy === right.updatedBy &&
    left.version === right.version &&
    left.targetRefs.length === right.targetRefs.length &&
    left.targetRefs.every((target, index) => target === right.targetRefs[index])
  );
}

function couponDetailResponse(
  value: unknown,
  couponID: number,
): CouponRuleDetail | undefined {
  if (
    !positive(couponID) ||
    !record(value) ||
    !exact(value, ["ok", "coupon", "data"]) ||
    value.ok !== true ||
    !record(value.data) ||
    !exact(value.data, ["coupon"])
  ) {
    return undefined;
  }
  const primary = couponDetail(value.coupon);
  const mirror = couponDetail(value.data.coupon);
  return primary &&
    mirror &&
    primary.id === couponID &&
    sameCouponDetail(primary, mirror)
    ? primary
    : undefined;
}

function sameTimestamp(
  value: string | undefined,
  expected: string | null | undefined,
): boolean {
  return (
    typeof value === "string" &&
    typeof expected === "string" &&
    Date.parse(value) === Date.parse(expected)
  );
}

function sameRuleResponse(
  item: CouponRuleDetail,
  request: CouponUpsertRequest,
): boolean {
  if (
    item.name !== request.name ||
    item.discountAmountTotal !== request.discount_amount_total ||
    item.totalIssueLimit !== request.total_issue_limit ||
    item.perUserIssueLimit !== request.per_user_issue_limit ||
    !sameTimestamp(item.claimStartsAt, request.claim_starts_at) ||
    !sameTimestamp(item.claimEndsAt, request.claim_ends_at) ||
    item.validityMode !== request.validity_mode ||
    item.instructions !== (request.instructions ?? "") ||
    item.targetRefs.length !== request.target_refs.length ||
    !item.targetRefs.every(
      (target, index) => target === request.target_refs[index],
    )
  ) {
    return false;
  }
  if (request.validity_mode === "fixed_range") {
    return (
      sameTimestamp(item.useStartsAt, request.use_starts_at) &&
      sameTimestamp(item.useEndsAt, request.use_ends_at) &&
      item.relativeValidityDays === undefined
    );
  }
  return (
    item.useStartsAt === undefined &&
    item.useEndsAt === undefined &&
    item.relativeValidityDays === request.relative_validity_days
  );
}

function createCouponResponse(
  value: unknown,
  request: CouponUpsertRequest,
): CouponRuleDetail | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "coupon",
      "coupon_id",
      "fallback_used",
      "create_replay_safe",
      "real_external_call_executed",
    ]) ||
    value.ok !== true ||
    value.fallback_used !== false ||
    value.create_replay_safe !== false ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const item = couponDetail(value.coupon);
  return item &&
    value.coupon_id === item.id &&
    item.status === "draft" &&
    item.availability === "draft" &&
    item.issuedCount === 0 &&
    sameRuleResponse(item, request)
    ? item
    : undefined;
}

function updateCouponResponse(
  value: unknown,
  current: CouponRuleDetail,
  request: CouponUpsertRequest,
): CouponRuleDetail | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "coupon",
      "fallback_used",
      "real_external_call_executed",
    ]) ||
    value.ok !== true ||
    value.fallback_used !== false ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const item = couponDetail(value.coupon);
  return item &&
    item.id === current.id &&
    item.status === "draft" &&
    item.availability === "draft" &&
    item.issuedCount === current.issuedCount &&
    sameRuleResponse(item, request)
    ? item
    : undefined;
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

function couponProductOption(value: unknown): CouponProductOption | undefined {
  if (
    !record(value) ||
    !exact(value, ["id", "target_ref", "name", "price_minor", "currency"]) ||
    !positive(value.id) ||
    value.target_ref !== `standard_product:${value.id}` ||
    !text(value.name, 200) ||
    !nonnegative(value.price_minor) ||
    value.currency !== "CNY"
  ) {
    return undefined;
  }
  return {
    id: value.id,
    targetRef: value.target_ref,
    name: value.name,
    priceMinor: value.price_minor,
    currency: "CNY",
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

function couponProductOptionsPage(
  value: unknown,
  offset: number,
):
  | { readonly items: readonly CouponProductOption[]; readonly total: number }
  | undefined {
  if (
    !record(value) ||
    !exact(value, ["ok", "items", "total", "limit", "offset"]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    !nonnegative(value.total) ||
    value.limit !== couponProductOptionsPageSize ||
    value.offset !== offset ||
    offset % couponProductOptionsPageSize !== 0 ||
    value.items.length > couponProductOptionsPageSize ||
    value.total < offset + value.items.length ||
    (value.items.length < couponProductOptionsPageSize &&
      offset + value.items.length < value.total)
  ) {
    return undefined;
  }
  const items = value.items.map(couponProductOption);
  if (
    items.includes(undefined) ||
    new Set((items as readonly CouponProductOption[]).map((item) => item.id))
      .size !== items.length ||
    new Set(
      (items as readonly CouponProductOption[]).map((item) => item.targetRef),
    ).size !== items.length
  ) {
    return undefined;
  }
  return { items: items as readonly CouponProductOption[], total: value.total };
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

export async function loadCouponProductOptions(
  transport: CouponsTransport,
  offset: number,
): Promise<CouponProductOptionsResult> {
  if (
    !nonnegative(offset) ||
    offset % couponProductOptionsPageSize !== 0 ||
    offset > maximumCouponOffset
  ) {
    return { status: "invalid" };
  }
  let response: Awaited<ReturnType<CouponsTransport["productOptions"]>>;
  try {
    response = await transport.productOptions(
      {
        product_type: "standard_product",
        limit: couponProductOptionsPageSize,
        offset,
      },
      { credentials: "same-origin" },
    );
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200)
    return { status: failure(response.status, response.data) };
  const page = couponProductOptionsPage(response.data, offset);
  return page ? { status: "loaded", ...page, offset } : { status: "invalid" };
}

export async function loadCouponDetail(
  transport: CouponsTransport,
  couponID: number,
): Promise<CouponDetailResult> {
  if (!positive(couponID)) return { status: "invalid" };
  let response: Awaited<ReturnType<CouponsTransport["detail"]>>;
  try {
    response = await transport.detail(couponID, { credentials: "same-origin" });
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200)
    return { status: failure(response.status, response.data) };
  const detail = couponDetailResponse(response.data, couponID);
  return detail ? { status: "loaded", detail } : { status: "invalid" };
}

function validCSRF(value: string): boolean {
  return /^[A-Za-z0-9_-]{43}$/.test(value);
}

function knownMutationFailure(
  status: number,
  body: unknown,
): Exclude<
  CouponDraftMutationResult,
  { readonly status: "created" | "updated" }
> {
  const result = failure(status, body);
  return { status: result, outcomeUncertain: result === "unavailable" };
}

export async function createCouponDraft(
  transport: CouponsTransport,
  input: CouponDraftInput,
  csrf: string,
): Promise<CouponDraftMutationResult> {
  const request = couponUpsertRequest(input);
  if (!request || !validCSRF(csrf))
    return { status: "invalid", outcomeUncertain: false };
  let response: Awaited<ReturnType<CouponsTransport["create"]>>;
  try {
    response = await transport.create(request, {
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrf },
    });
  } catch {
    return { status: "unavailable", outcomeUncertain: true };
  }
  if (response.status !== 200)
    return knownMutationFailure(response.status, response.data);
  const item = createCouponResponse(response.data, request);
  return item
    ? { status: "created", item }
    : { status: "invalid", outcomeUncertain: true };
}

export async function updateCouponDraft(
  transport: CouponsTransport,
  current: CouponRuleDetail,
  input: CouponDraftInput,
  csrf: string,
): Promise<CouponDraftMutationResult> {
  const request = couponUpsertRequest(input);
  if (
    !request ||
    !validCSRF(csrf) ||
    current.status !== "draft" ||
    current.availability !== "draft" ||
    !positive(current.id)
  ) {
    return { status: "invalid", outcomeUncertain: false };
  }
  let response: Awaited<ReturnType<CouponsTransport["update"]>>;
  try {
    response = await transport.update(current.id, request, {
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrf },
    });
  } catch {
    return { status: "unavailable", outcomeUncertain: true };
  }
  if (response.status !== 200)
    return knownMutationFailure(response.status, response.data);
  const item = updateCouponResponse(response.data, current, request);
  return item
    ? { status: "updated", item }
    : { status: "invalid", outcomeUncertain: true };
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
