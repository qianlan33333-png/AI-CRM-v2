import {
  approveIdentityMergeReview,
  listIdentityMergeReviews,
  rejectIdentityMergeReview,
  type ApproveIdentityMergeReviewRequest,
  type ListIdentityMergeReviewsParams,
  type RejectIdentityMergeReviewRequest,
} from "./api/generated/health";

export type IdentityReviewRole = "admin" | "ops" | "sales";
export type IdentityMergeReviewStatus = "pending" | "approved" | "rejected";

export const identityMergeReviewStatuses = [
  "pending",
  "approved",
  "rejected",
] as const satisfies readonly IdentityMergeReviewStatus[];

export function validIdentityMergeReviewStatus(
  value: unknown,
): value is IdentityMergeReviewStatus {
  return value === "pending" || value === "approved" || value === "rejected";
}

export interface IdentityMergeReviewRecord {
  readonly reviewID: number;
  readonly status: IdentityMergeReviewStatus;
  readonly type: "unionid" | "phone";
  readonly scope: string;
  readonly customerIDs: readonly [number, number];
  readonly identityFingerprint: string;
  readonly version: number;
  readonly createdAt: string;
  readonly resolvedAt?: string;
}

export interface IdentityMergeReviewPage {
  readonly status: IdentityMergeReviewStatus;
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
      "customer_ids",
      "identity_fingerprint",
      "version",
      "created_at",
      "resolved_at",
    ])
  ) {
    return undefined;
  }
  if (
    !positiveSafeInteger(value.review_id) ||
    !validIdentityMergeReviewStatus(value.status) ||
    (value.type !== "unionid" && value.type !== "phone") ||
    typeof value.scope !== "string" ||
    value.scope.length === 0 ||
    !Array.isArray(value.customer_ids) ||
    value.customer_ids.length !== 2 ||
    !positiveSafeInteger(value.customer_ids[0]) ||
    !positiveSafeInteger(value.customer_ids[1]) ||
    value.customer_ids[0] >= value.customer_ids[1] ||
    typeof value.identity_fingerprint !== "string" ||
    !/^hmac-sha256-v[1-9][0-9]*:[A-Za-z0-9_-]{21}[AQgw]$/.test(
      value.identity_fingerprint,
    ) ||
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
    customerIDs: [value.customer_ids[0], value.customer_ids[1]],
    identityFingerprint: value.identity_fingerprint,
    version: value.version,
    createdAt: value.created_at,
    ...(typeof value.resolved_at === "string"
      ? { resolvedAt: value.resolved_at }
      : {}),
  };
}

export function parseIdentityMergeReviewPage(
  value: unknown,
  expectedStatus: IdentityMergeReviewStatus,
): IdentityMergeReviewPage | undefined {
  if (
    !validIdentityMergeReviewStatus(expectedStatus) ||
    !plainRecord(value) ||
    !exactKeys(value, ["items", "next_cursor"])
  ) {
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
    items.some(
      (item) => item === undefined || item.status !== expectedStatus,
    ) ||
    new Set(items.map((item) => item?.reviewID)).size !== items.length
  ) {
    return undefined;
  }

  return {
    status: expectedStatus,
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
  if (current.status !== next.status) return undefined;
  const items = [...current.items, ...next.items];
  if (new Set(items.map(({ reviewID }) => reviewID)).size !== items.length) {
    return undefined;
  }
  return {
    status: current.status,
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

export interface LoadIdentityMergeReviewsOptions {
  readonly status: IdentityMergeReviewStatus;
  readonly cursor?: string;
  readonly signal?: AbortSignal;
}

export async function loadIdentityMergeReviews(
  transport: IdentityReviewTransport,
  options: LoadIdentityMergeReviewsOptions,
): Promise<IdentityReviewListResult> {
  const { status, cursor, signal } = options;
  if (
    !validIdentityMergeReviewStatus(status) ||
    (cursor !== undefined && (cursor.length === 0 || cursor.length > 512))
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.list(
      { status, limit: 50, ...(cursor === undefined ? {} : { cursor }) },
      { credentials: "same-origin", signal },
    );
    if (response.status !== 200) {
      return { status: failureForStatus(response.status) };
    }
    const page = parseIdentityMergeReviewPage(response.data, status);
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
    resolved.customerIDs[0] === pending.customerIDs[0] &&
    resolved.customerIDs[1] === pending.customerIDs[1] &&
    resolved.identityFingerprint === pending.identityFingerprint &&
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

export interface IdentityReviewListSnapshot {
  readonly activeStatus: IdentityMergeReviewStatus;
  readonly page?: IdentityMergeReviewPage;
  readonly loading: boolean;
  readonly loadingMore: boolean;
  readonly failure?: IdentityReviewFailure;
}

type IdentityReviewListListener = (...[]: [IdentityReviewListSnapshot]) => void;

type ListRequestMode = "replace" | "append";

type InFlightListRequest = {
  readonly key: string;
  readonly token: number;
  readonly controller: AbortController;
  readonly promise: Promise<IdentityReviewListResult>;
};

/**
 * Owns the review-list request lifecycle independently of React rendering.
 * Exactly one request generation may publish state. A different status,
 * cursor, refresh, or pagination request replaces the previous owner; an
 * identical request shares the same promise.
 */
export class IdentityReviewListController {
  private readonly pages = new Map<
    IdentityMergeReviewStatus,
    IdentityMergeReviewPage
  >();
  private readonly listeners = new Set<IdentityReviewListListener>();
  private activeStatus: IdentityMergeReviewStatus;
  private generation = 0;
  private current?: InFlightListRequest;
  private failure?: IdentityReviewFailure;
  private loading = false;
  private loadingMore = false;
  private disposed = false;
  private unauthenticatedNotified = false;
  private readonly transport: IdentityReviewTransport;
  private readonly onUnauthenticated?: () => void;

  public constructor(
    transport: IdentityReviewTransport,
    initialStatus: IdentityMergeReviewStatus = "pending",
    onUnauthenticated?: () => void,
  ) {
    this.transport = transport;
    this.onUnauthenticated = onUnauthenticated;
    this.activeStatus = validIdentityMergeReviewStatus(initialStatus)
      ? initialStatus
      : "pending";
  }

  public snapshot(): IdentityReviewListSnapshot {
    return {
      activeStatus: this.activeStatus,
      ...(this.pages.has(this.activeStatus)
        ? { page: this.pages.get(this.activeStatus) }
        : {}),
      loading: this.loading,
      loadingMore: this.loadingMore,
      ...(this.failure ? { failure: this.failure } : {}),
    };
  }

  public subscribe(listener: IdentityReviewListListener): () => void {
    if (this.disposed) return () => undefined;
    this.listeners.add(listener);
    listener(this.snapshot());
    return () => this.listeners.delete(listener);
  }

  public activate(
    status: IdentityMergeReviewStatus,
  ): Promise<IdentityReviewListResult> {
    if (!validIdentityMergeReviewStatus(status)) {
      return Promise.resolve({ status: "invalid" });
    }
    if (this.activeStatus !== status) {
      this.replaceOwner();
      this.activeStatus = status;
      this.failure = undefined;
      this.loading = false;
      this.loadingMore = false;
      this.publish();
    }
    return this.request(status, undefined, "replace");
  }

  public refresh(): Promise<IdentityReviewListResult> {
    return this.request(this.activeStatus, undefined, "replace");
  }

  public loadMore(): Promise<IdentityReviewListResult> {
    const page = this.pages.get(this.activeStatus);
    if (!page?.nextCursor) {
      return Promise.resolve({ status: "invalid" });
    }
    return this.request(this.activeStatus, page.nextCursor, "append");
  }

  public acceptResolution(review: IdentityMergeReviewRecord): void {
    if (
      this.disposed ||
      (review.status !== "approved" && review.status !== "rejected")
    ) {
      return;
    }
    const pending = this.pages.get("pending");
    if (pending) {
      this.pages.set("pending", {
        ...pending,
        items: pending.items.filter(
          ({ reviewID }) => reviewID !== review.reviewID,
        ),
      });
    }
    // A future visit must re-read the server-owned history partition.
    this.pages.delete(review.status);
    this.failure = undefined;
    this.publish();
  }

  public dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.replaceOwner();
    this.listeners.clear();
  }

  private request(
    status: IdentityMergeReviewStatus,
    cursor: string | undefined,
    mode: ListRequestMode,
  ): Promise<IdentityReviewListResult> {
    if (this.disposed) return Promise.resolve({ status: "unavailable" });
    const key = `${status}\u0000${mode}\u0000${cursor ?? ""}`;
    if (this.current?.key === key) return this.current.promise;

    this.replaceOwner();
    const token = ++this.generation;
    const controller = new AbortController();
    this.failure = undefined;
    this.loading = mode === "replace";
    this.loadingMore = mode === "append";
    this.publish();

    const promise = loadIdentityMergeReviews(this.transport, {
      status,
      ...(cursor === undefined ? {} : { cursor }),
      signal: controller.signal,
    }).then((result) => {
      if (!this.owns(token)) return result;
      this.loading = false;
      this.loadingMore = false;
      if (result.status === "loaded") {
        const nextPage =
          mode === "append"
            ? appendIdentityMergeReviewPage(
                this.pages.get(status) ?? {
                  status,
                  items: [],
                  ...(cursor ? { nextCursor: cursor } : {}),
                },
                result.page,
              )
            : result.page;
        if (!nextPage) {
          this.failure = "invalid";
        } else {
          this.pages.set(status, nextPage);
          this.failure = undefined;
        }
      } else {
        this.failure = result.status;
        if (
          result.status === "unauthenticated" &&
          !this.unauthenticatedNotified
        ) {
          this.unauthenticatedNotified = true;
          this.onUnauthenticated?.();
        }
      }
      if (this.current?.token === token) this.current = undefined;
      this.publish();
      return result;
    });

    this.current = { key, token, controller, promise };
    return promise;
  }

  private owns(token: number): boolean {
    return (
      !this.disposed &&
      this.current?.token === token &&
      this.generation === token
    );
  }

  private replaceOwner(): void {
    this.generation++;
    this.current?.controller.abort();
    this.current = undefined;
  }

  private publish(): void {
    if (this.disposed) return;
    const snapshot = this.snapshot();
    for (const listener of this.listeners) listener(snapshot);
  }
}
