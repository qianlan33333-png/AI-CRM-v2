/* eslint-disable no-unused-vars -- transport arguments are the generated API seam. */
import { getCloudCampaignTouchPlanReview, mutateCloudCampaignTouchPlanReview, type CloudCampaignTouchPlanReviewMutationRequest } from "./api/generated/health";
import type { SessionStorageLike } from "./campaign-touch-plan-core";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";

type TransportResponse = { readonly status: number; readonly data: unknown };
export type ReviewOperation = "submit" | "approve" | "reject";
export interface CampaignTouchPlanReviewTransport {
  readonly getReview: (campaignCode: string, planID: string, options: RequestInit) => Promise<TransportResponse>;
  readonly mutateReview: (
    campaignCode: string,
    planID: string,
    operation: ReviewOperation,
    request: CloudCampaignTouchPlanReviewMutationRequest,
    options: RequestInit,
  ) => Promise<TransportResponse>;
}
export const generatedCampaignTouchPlanReviewTransport: CampaignTouchPlanReviewTransport = {
  getReview: (code, id, options) => getCloudCampaignTouchPlanReview(code, id, options),
  mutateReview: (code, id, operation, request, options) => mutateCloudCampaignTouchPlanReview(code, id, operation, request, options),
};

export type TouchPlanReview =
  | { readonly status: "draft"; readonly version: 1 }
  | {
      readonly status: "pending_review";
      readonly version: 2;
      readonly submittedByActorID: number;
      readonly submittedAt: string;
    }
  | {
      readonly status: "approved" | "rejected";
      readonly version: 3;
      readonly submittedByActorID: number;
      readonly submittedAt: string;
      readonly reviewedByActorID: number;
      readonly reviewedAt: string;
    };
export interface TouchPlanReviewHandoff {
  readonly status: "pending_outbound_acceptance";
  readonly reviewVersion: 3;
  readonly createdAt: string;
}
export interface TouchPlanReviewSnapshot {
  readonly review: TouchPlanReview;
  readonly handoff?: TouchPlanReviewHandoff;
}

function exact(value: unknown, keys: readonly string[]): Record<string, unknown> | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return undefined;
  const item = value as Record<string, unknown>;
  const actual = Object.keys(item);
  return actual.length === keys.length && actual.every((key) => keys.includes(key)) ? item : undefined;
}
const positive = (value: unknown): value is number => typeof value === "number" && Number.isSafeInteger(value) && value > 0;
const utcMicroseconds = (value: unknown): value is string => {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,6})?Z$/u.exec(value);
  if (!match) return false;
  const parsed = new Date(value);
  return (
    Number.isFinite(parsed.getTime()) &&
    parsed.getUTCFullYear() === Number(match[1]) &&
    parsed.getUTCMonth() + 1 === Number(match[2]) &&
    parsed.getUTCDate() === Number(match[3]) &&
    parsed.getUTCHours() === Number(match[4]) &&
    parsed.getUTCMinutes() === Number(match[5]) &&
    parsed.getUTCSeconds() === Number(match[6])
  );
};
const utcOrder = (value: string): string => value.replace(/(?:\.(\d{1,6}))?Z$/u, (_match, fraction = "") => `.${fraction.padEnd(6, "0")}Z`);

function parseReview(value: unknown): TouchPlanReview | undefined {
  const base = exact(value, ["status", "version"]);
  if (base?.status === "draft" && base.version === 1) return { status: "draft", version: 1 };
  const submitted = exact(value, ["status", "version", "submitted_by_actor_id", "submitted_at"]);
  if (
    submitted?.status === "pending_review" &&
    submitted.version === 2 &&
    positive(submitted.submitted_by_actor_id) &&
    utcMicroseconds(submitted.submitted_at)
  ) {
    return {
      status: "pending_review",
      version: 2,
      submittedByActorID: submitted.submitted_by_actor_id,
      submittedAt: submitted.submitted_at,
    };
  }
  const terminal = exact(value, ["status", "version", "submitted_by_actor_id", "submitted_at", "reviewed_by_actor_id", "reviewed_at"]);
  if (
    (terminal?.status === "approved" || terminal?.status === "rejected") &&
    terminal.version === 3 &&
    positive(terminal.submitted_by_actor_id) &&
    utcMicroseconds(terminal.submitted_at) &&
    positive(terminal.reviewed_by_actor_id) &&
    utcMicroseconds(terminal.reviewed_at) &&
    utcOrder(terminal.reviewed_at) >= utcOrder(terminal.submitted_at)
  ) {
    return {
      status: terminal.status,
      version: 3,
      submittedByActorID: terminal.submitted_by_actor_id,
      submittedAt: terminal.submitted_at,
      reviewedByActorID: terminal.reviewed_by_actor_id,
      reviewedAt: terminal.reviewed_at,
    };
  }
  return undefined;
}

export function parseTouchPlanReviewResponse(value: unknown): TouchPlanReviewSnapshot | undefined {
  const withHandoff = exact(value, ["review", "handoff", "local_only", "provider_execution_eligible", "real_external_call_executed", "delivery_proven"]);
  const body = withHandoff ?? exact(value, ["review", "local_only", "provider_execution_eligible", "real_external_call_executed", "delivery_proven"]);
  if (
    !body ||
    body.local_only !== true ||
    body.provider_execution_eligible !== false ||
    body.real_external_call_executed !== false ||
    body.delivery_proven !== false
  )
    return undefined;
  const review = parseReview(body.review);
  if (!review) return undefined;
  if (!withHandoff) return review.status === "approved" ? undefined : { review };
  const handoff = exact(withHandoff.handoff, ["status", "review_version", "created_at"]);
  return review.status === "approved" &&
    handoff?.status === "pending_outbound_acceptance" &&
    handoff.review_version === 3 &&
    utcMicroseconds(handoff.created_at) &&
    handoff.created_at === review.reviewedAt
    ? {
        review,
        handoff: {
          status: "pending_outbound_acceptance",
          reviewVersion: 3,
          createdAt: handoff.created_at,
        },
      }
    : undefined;
}

export type ReviewLoadResult =
  | ({ readonly status: "loaded" } & TouchPlanReviewSnapshot)
  | {
      readonly status: "unauthenticated" | "forbidden" | "not_found" | "unavailable";
    };
const failed = (status: number): Exclude<ReviewLoadResult["status"], "loaded"> =>
  status === 401 ? "unauthenticated" : status === 403 ? "forbidden" : status === 404 ? "not_found" : "unavailable";
const validPlan = (plan: TouchPlanSummary): boolean =>
  /^[A-Za-z0-9._-]{1,96}$/u.test(plan.campaignCode) &&
  /^ctp_[0-9a-f]{64}$/u.test(plan.id) &&
  positive(plan.campaignVersion) &&
  typeof plan.immutable === "string" &&
  plan.immutable.length > 0;

export async function loadTouchPlanReview(
  transport: CampaignTouchPlanReviewTransport,
  plan: TouchPlanSummary,
  signal?: AbortSignal,
): Promise<ReviewLoadResult> {
  if (!validPlan(plan)) return { status: "unavailable" };
  try {
    const response = await transport.getReview(plan.campaignCode, plan.id, {
      credentials: "same-origin",
      signal,
    });
    if (response.status !== 200) return { status: failed(response.status) };
    const snapshot = parseTouchPlanReviewResponse(response.data);
    return snapshot ? { status: "loaded", ...snapshot } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

interface IntentPayload {
  readonly campaign_code: string;
  readonly plan_id: string;
  readonly plan_immutable: string;
  readonly operation: ReviewOperation;
  readonly expected_version: number;
  readonly confirmation: string;
}
interface PendingIntent {
  readonly version: 1;
  readonly idempotency_key: string;
  readonly payload: IntentPayload;
}
export const campaignTouchPlanReviewStorageKey = (actorID: number): string => `aicrm:campaign-touch-plan-review:v1:actor:${actorID}`;
const keyPattern = /^campaign-touch-plan-review:[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu;
function readPending(storage: SessionStorageLike, actorID: number): PendingIntent | "none" | "invalid" {
  try {
    const raw = storage.getItem(campaignTouchPlanReviewStorageKey(actorID));
    if (raw === null) return "none";
    const value: unknown = JSON.parse(raw);
    const item = exact(value, ["version", "idempotency_key", "payload"]);
    const payload = exact(item?.payload, ["campaign_code", "plan_id", "plan_immutable", "operation", "expected_version", "confirmation"]);
    if (
      item?.version !== 1 ||
      typeof item.idempotency_key !== "string" ||
      !keyPattern.test(item.idempotency_key) ||
      !payload ||
      typeof payload.campaign_code !== "string" ||
      !/^[A-Za-z0-9._-]{1,96}$/u.test(payload.campaign_code) ||
      typeof payload.plan_id !== "string" ||
      !/^ctp_[0-9a-f]{64}$/u.test(payload.plan_id) ||
      typeof payload.plan_immutable !== "string" ||
      typeof payload.operation !== "string" ||
      !["submit", "approve", "reject"].includes(payload.operation) ||
      !positive(payload.expected_version) ||
      typeof payload.confirmation !== "string"
    )
      return "invalid";
    const normalized = {
      version: 1 as const,
      idempotency_key: item.idempotency_key,
      payload: payload as unknown as IntentPayload,
    };
    return JSON.stringify(normalized) === raw ? normalized : "invalid";
  } catch {
    return "invalid";
  }
}
function writePending(storage: SessionStorageLike, actorID: number, value: PendingIntent): boolean {
  try {
    const raw = JSON.stringify(value);
    storage.setItem(campaignTouchPlanReviewStorageKey(actorID), raw);
    return storage.getItem(campaignTouchPlanReviewStorageKey(actorID)) === raw;
  } catch {
    return false;
  }
}
function clearPending(storage: SessionStorageLike, actorID: number): void {
  try {
    storage.removeItem(campaignTouchPlanReviewStorageKey(actorID));
  } catch {
    /* deterministic server results stay deterministic */
  }
}

export interface ReviewMutationInput {
  readonly plan: TouchPlanSummary;
  readonly review: TouchPlanReview;
  readonly operation: ReviewOperation;
  readonly confirmation: string;
  readonly csrf: string;
  readonly signal?: AbortSignal;
}
export type ReviewMutationResult =
  | ({ readonly status: "completed" } & TouchPlanReviewSnapshot)
  | ({ readonly status: "conflict" } & Partial<TouchPlanReviewSnapshot>)
  | {
      readonly status:
        | "blocked"
        | "confirmation_required"
        | "invalid"
        | "inflight"
        | "replay_required"
        | "replay_mismatch"
        | "no_pending"
        | "storage_unavailable"
        | "unauthenticated"
        | "forbidden"
        | "not_found"
        | "outcome_unknown";
    };

export class CampaignTouchPlanReviewMachine {
  private active = false;
  constructor(
    private readonly options: {
      readonly transport: CampaignTouchPlanReviewTransport;
      readonly sessionStorage: SessionStorageLike;
      readonly actorID: number;
      readonly keySource: { readonly randomUUID: () => string };
    },
  ) {}
  start(input: ReviewMutationInput): Promise<ReviewMutationResult> {
    return this.run(input, false);
  }
  replay(input: ReviewMutationInput): Promise<ReviewMutationResult> {
    return this.run(input, true);
  }

  private intent(input: ReviewMutationInput): IntentPayload | "confirmation" | "invalid" {
    if (!validPlan(input.plan) || !positive(this.options.actorID) || !/^[A-Za-z0-9_-]{43}$/u.test(input.csrf)) return "invalid";
    const expected =
      input.review.status === "draft" && input.operation === "submit"
        ? ""
        : input.review.status === "pending_review" && input.operation === "approve"
          ? `APPROVE ${input.plan.id}`
          : input.review.status === "pending_review" && input.operation === "reject"
            ? `REJECT ${input.plan.id}`
            : undefined;
    if (expected === undefined) return "invalid";
    if (input.confirmation !== expected) return "confirmation";
    return {
      campaign_code: input.plan.campaignCode,
      plan_id: input.plan.id,
      plan_immutable: input.plan.immutable,
      operation: input.operation,
      expected_version: input.review.version,
      confirmation: input.confirmation,
    };
  }
  private async run(input: ReviewMutationInput, replay: boolean): Promise<ReviewMutationResult> {
    if (this.active) return { status: "inflight" };
    this.active = true;
    try {
      const payload = this.intent(input);
      if (payload === "confirmation") return { status: "confirmation_required" };
      if (payload === "invalid") return { status: "invalid" };
      const stored = readPending(this.options.sessionStorage, this.options.actorID);
      if (stored === "invalid") return { status: "storage_unavailable" };
      let pending: PendingIntent;
      if (replay) {
        if (stored === "none") return { status: "no_pending" };
        if (JSON.stringify(stored.payload) !== JSON.stringify(payload)) return { status: "replay_mismatch" };
        pending = stored;
      } else {
        if (stored !== "none") return { status: "replay_required" };
        let uuid = "";
        try {
          uuid = this.options.keySource.randomUUID();
        } catch {
          return { status: "invalid" };
        }
        const idempotencyKey = `campaign-touch-plan-review:${uuid}`;
        if (!keyPattern.test(idempotencyKey)) return { status: "invalid" };
        pending = { version: 1, idempotency_key: idempotencyKey, payload };
        if (!writePending(this.options.sessionStorage, this.options.actorID, pending)) return { status: "storage_unavailable" };
      }
      return await this.send(input, pending);
    } finally {
      this.active = false;
    }
  }
  private async send(input: ReviewMutationInput, pending: PendingIntent): Promise<ReviewMutationResult> {
    const request: CloudCampaignTouchPlanReviewMutationRequest =
      pending.payload.operation === "submit"
        ? { expected_version: pending.payload.expected_version }
        : {
            expected_version: pending.payload.expected_version,
            confirmation: pending.payload.confirmation,
          };
    let response: TransportResponse;
    try {
      response = await this.options.transport.mutateReview(pending.payload.campaign_code, pending.payload.plan_id, pending.payload.operation, request, {
        credentials: "same-origin",
        signal: input.signal,
        headers: {
          "X-CSRF-Token": input.csrf,
          "Idempotency-Key": pending.idempotency_key,
        },
      });
    } catch {
      return { status: "outcome_unknown" };
    }
    if (response.status === 200) {
      const snapshot = parseTouchPlanReviewResponse(response.data);
      const expected = pending.payload.operation === "submit" ? "pending_review" : pending.payload.operation === "approve" ? "approved" : "rejected";
      const actorMatches =
        snapshot?.review.status === "pending_review"
          ? snapshot.review.submittedByActorID === this.options.actorID
          : snapshot?.review.status === "approved" || snapshot?.review.status === "rejected"
            ? snapshot.review.reviewedByActorID === this.options.actorID
            : false;
      if (!snapshot || snapshot.review.status !== expected || !actorMatches) return { status: "outcome_unknown" };
      clearPending(this.options.sessionStorage, this.options.actorID);
      return { status: "completed", ...snapshot };
    }
    if (response.status === 409) {
      clearPending(this.options.sessionStorage, this.options.actorID);
      const error = exact(response.data, ["code"]);
      if (error?.code === "BLOCKED_REDLINE") return { status: "blocked" };
      if (error?.code !== "CONFLICT") return { status: "invalid" };
      const refreshed = await loadTouchPlanReview(this.options.transport, input.plan, input.signal);
      if (refreshed.status === "loaded")
        return {
          status: "conflict",
          review: refreshed.review,
          ...(refreshed.handoff ? { handoff: refreshed.handoff } : {}),
        };
      return refreshed.status === "unavailable" ? { status: "conflict" } : { status: refreshed.status };
    }
    if ([400, 401, 403, 404].includes(response.status)) {
      clearPending(this.options.sessionStorage, this.options.actorID);
      return {
        status: response.status === 401 ? "unauthenticated" : response.status === 403 ? "forbidden" : response.status === 404 ? "not_found" : "invalid",
      };
    }
    return { status: "outcome_unknown" };
  }
}
