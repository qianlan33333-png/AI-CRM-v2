import {
  approveIdentityMergeReview,
  listIdentityMergeReviews,
  rejectIdentityMergeReview,
  type ApproveIdentityMergeReviewRequest,
  type ListIdentityMergeReviewsParams,
  type RejectIdentityMergeReviewRequest,
} from "./api/generated/health";

export type IdentityReviewRole = "admin" | "ops" | "sales";

export interface IdentityMergeReviewRecord {
  readonly reviewID: number;
  readonly status: "pending" | "approved" | "rejected";
  readonly type: "unionid" | "phone";
  readonly scope: string;
  readonly identityFingerprint: string;
  readonly customerIDs: readonly [number, number];
  readonly version: number;
  readonly createdAt: string;
  readonly resolvedAt?: string;
}

export interface IdentityMergeReviewPage {
  readonly items: readonly IdentityMergeReviewRecord[];
  /** Opaque server-issued keyset cursor. The UI never decodes or synthesizes it. */
  readonly nextCursor?: string;
}

export interface IdentityReviewTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  params: ListIdentityMergeReviewsParams,
  options: RequestInit,
): Promise<IdentityReviewTransportResponse> {
  return listIdentityMergeReviews(params, options);
}

async function generatedApprove(
  reviewID: number,
  request: ApproveIdentityMergeReviewRequest,
  options: RequestInit,
): Promise<IdentityReviewTransportResponse> {
  return approveIdentityMergeReview(reviewID, request, options);
}

async function generatedReject(
  reviewID: number,
  request: RejectIdentityMergeReviewRequest,
  options: RequestInit,
): Promise<IdentityReviewTransportResponse> {
  return rejectIdentityMergeReview(reviewID, request, options);
}

export interface IdentityReviewTransport {
  readonly list: typeof generatedList;
  readonly approve: typeof generatedApprove;
  readonly reject: typeof generatedReject;
}

export const generatedIdentityReviewTransport: IdentityReviewTransport = {
  list: generatedList,
  approve: generatedApprove,
  reject: generatedReject,
};

export type IdentityReviewFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";

export type IdentityReviewListResult =
  | { readonly status: "loaded"; readonly page: IdentityMergeReviewPage }
  | { readonly status: IdentityReviewFailure };

export type IdentityReviewMutationResult =
  | { readonly status: "completed"; readonly review: IdentityMergeReviewRecord }
  | { readonly status: IdentityReviewFailure };

function plainRecord(value: unknown): value is Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function exactKeys(
  value: Record<string, unknown>,
  required: readonly string[],
): boolean {
  const allowed = new Set(required);
  return (
    required.every((key) => Object.hasOwn(value, key)) &&
    Object.keys(value).every((key) => allowed.has(key))
  );
}

function positiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function validDateTime(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) &&
    !Number.isNaN(Date.parse(value))
  );
}

const fingerprintPattern = /^hmac-sha256-v[1-9][0-9]*:[A-Za-z0-9_-]{21}[AQgw]$/;

export function parseIdentityMergeReview(
  value: unknown,
): IdentityMergeReviewRecord | undefined {
  if (!plainRecord(value)) return undefined;
  if (
    !exactKeys(value, [
      "review_id",
      "status",
      "type",
      "scope",
      "identity_fingerprint",
      "customer_ids",
      "version",
      "created_at",
      "resolved_at",
    ])
  ) {
    return undefined;
  }
  if (
    !positiveSafeInteger(value.review_id) ||
    (value.status !== "pending" &&
      value.status !== "approved" &&
      value.status !== "rejected") ||
    (value.type !== "unionid" && value.type !== "phone") ||
    typeof value.scope !== "string" ||
    value.scope.length === 0 ||
    typeof value.identity_fingerprint !== "string" ||
    !fingerprintPattern.test(value.identity_fingerprint) ||
    !Array.isArray(value.customer_ids) ||
    value.customer_ids.length !== 2 ||
    !positiveSafeInteger(value.customer_ids[0]) ||
    !positiveSafeInteger(value.customer_ids[1]) ||
    value.customer_ids[0] >= value.customer_ids[1] ||
    !positiveSafeInteger(value.version) ||
    !validDateTime(value.created_at)
  ) {
    return undefined;
  }
  if (
    (value.status === "pending" && value.resolved_at !== null) ||
    (value.status !== "pending" && !validDateTime(value.resolved_at))
  ) {
    return undefined;
  }
  if (
    typeof value.resolved_at === "string" &&
    Date.parse(value.resolved_at) < Date.parse(value.created_at)
  ) {
    return undefined;
  }

  return {
    reviewID: value.review_id,
    status: value.status,
    type: value.type,
    scope: value.scope,
    identityFingerprint: value.identity_fingerprint,
    customerIDs: [value.customer_ids[0], value.customer_ids[1]],
    version: value.version,
    createdAt: value.created_at,
    ...(typeof value.resolved_at === "string"
      ? { resolvedAt: value.resolved_at }
      : {}),
  };
}

export function parseIdentityMergeReviewPage(
  value: unknown,
): IdentityMergeReviewPage | undefined {
  if (!plainRecord(value) || !exactKeys(value, ["items", "next_cursor"])) {
    return undefined;
  }
  if (
    !Array.isArray(value.items) ||
    (value.next_cursor !== null &&
      (typeof value.next_cursor !== "string" ||
        value.next_cursor.length === 0 ||
        value.next_cursor.length > 512))
  ) {
    return undefined;
  }
  const items = value.items.map(parseIdentityMergeReview);
  if (
    items.some((item) => item === undefined || item.status !== "pending") ||
    new Set(items.map((item) => item?.reviewID)).size !== items.length
  ) {
    return undefined;
  }

  return {
    items: items as IdentityMergeReviewRecord[],
    ...(typeof value.next_cursor === "string"
      ? { nextCursor: value.next_cursor }
      : {}),
  };
}

export function appendIdentityMergeReviewPage(
  current: IdentityMergeReviewPage,
  next: IdentityMergeReviewPage,
): IdentityMergeReviewPage | undefined {
  const items = [...current.items, ...next.items];
  if (new Set(items.map(({ reviewID }) => reviewID)).size !== items.length) {
    return undefined;
  }
  return {
    items,
    ...(next.nextCursor ? { nextCursor: next.nextCursor } : {}),
  };
}

function failureForStatus(status: number): IdentityReviewFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400 || status === 422) return "invalid";
  return "unavailable";
}

export async function loadIdentityMergeReviews(
  transport: IdentityReviewTransport,
  cursor?: string,
): Promise<IdentityReviewListResult> {
  if (cursor !== undefined && (cursor.length === 0 || cursor.length > 512)) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.list(
      { limit: 50, ...(cursor === undefined ? {} : { cursor }) },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) {
      return { status: failureForStatus(response.status) };
    }
    const page = parseIdentityMergeReviewPage(response.data);
    return page ? { status: "loaded", page } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

function validReason(reason: string): boolean {
  return reason === reason.trim() && reason.length >= 1 && reason.length <= 500;
}

function validCommandKey(key: string): boolean {
  return key.length >= 16 && key.length <= 128;
}

function validCSRFToken(token: string): boolean {
  return /^[A-Za-z0-9_-]{43}$/.test(token);
}

function matchesResolvedReview(
  pending: IdentityMergeReviewRecord,
  resolved: IdentityMergeReviewRecord | undefined,
  status: "approved" | "rejected",
): resolved is IdentityMergeReviewRecord {
  return Boolean(
    resolved &&
    resolved.reviewID === pending.reviewID &&
    resolved.status === status &&
    resolved.type === pending.type &&
    resolved.scope === pending.scope &&
    resolved.identityFingerprint === pending.identityFingerprint &&
    resolved.customerIDs[0] === pending.customerIDs[0] &&
    resolved.customerIDs[1] === pending.customerIDs[1] &&
    resolved.version === pending.version + 1 &&
    resolved.createdAt === pending.createdAt &&
    resolved.resolvedAt,
  );
}

function mutationOptions(csrfToken: string, commandKey: string): RequestInit {
  return {
    credentials: "same-origin",
    headers: {
      "Idempotency-Key": commandKey,
      "X-CSRF-Token": csrfToken,
    },
  };
}

export async function approveIdentityReview(
  transport: IdentityReviewTransport,
  pending: IdentityMergeReviewRecord,
  primaryCustomerID: number,
  reason: string,
  csrfToken: string,
  commandKey: string,
): Promise<IdentityReviewMutationResult> {
  if (
    pending.status !== "pending" ||
    !pending.customerIDs.includes(primaryCustomerID) ||
    !validReason(reason) ||
    !validCSRFToken(csrfToken) ||
    !validCommandKey(commandKey)
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.approve(
      pending.reviewID,
      {
        expected_version: pending.version,
        primary_customer_id: primaryCustomerID,
        reason,
      },
      mutationOptions(csrfToken, commandKey),
    );
    if (response.status !== 200) {
      return { status: failureForStatus(response.status) };
    }
    const review = parseIdentityMergeReview(response.data);
    return matchesResolvedReview(pending, review, "approved")
      ? { status: "completed", review }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function rejectIdentityReview(
  transport: IdentityReviewTransport,
  pending: IdentityMergeReviewRecord,
  reason: string,
  csrfToken: string,
  commandKey: string,
): Promise<IdentityReviewMutationResult> {
  if (
    pending.status !== "pending" ||
    !validReason(reason) ||
    !validCSRFToken(csrfToken) ||
    !validCommandKey(commandKey)
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.reject(
      pending.reviewID,
      { expected_version: pending.version, reason },
      mutationOptions(csrfToken, commandKey),
    );
    if (response.status !== 200) {
      return { status: failureForStatus(response.status) };
    }
    const review = parseIdentityMergeReview(response.data);
    return matchesResolvedReview(pending, review, "rejected")
      ? { status: "completed", review }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}
