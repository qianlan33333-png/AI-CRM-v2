/* eslint-disable no-unused-vars -- transport arguments are the generated API seam. */
import {
  acceptOutboundCampaignHandoff,
  getOutboundCampaignHandoffSummary,
  reconcileOutboundCampaignHandoff,
  type OutboundCampaignHandoffAcceptRequest,
} from "./api/generated/health";
import type { SessionStorageLike } from "./campaign-touch-plan-core";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";
import type { TouchPlanReviewSnapshot } from "./campaign-touch-plan-review";

type TransportResponse = { readonly status: number; readonly data: unknown };
export interface OutboundCampaignHandoffTransport {
  readonly getSummary: (campaignCode: string, planID: string, options: RequestInit) => Promise<TransportResponse>;
  readonly accept: (campaignCode: string, planID: string, request: OutboundCampaignHandoffAcceptRequest, options: RequestInit) => Promise<TransportResponse>;
  readonly reconcile: (campaignCode: string, planID: string, options: RequestInit) => Promise<TransportResponse>;
}
export const generatedOutboundCampaignHandoffTransport: OutboundCampaignHandoffTransport = {
  getSummary: (code, id, options) => getOutboundCampaignHandoffSummary(code, id, options),
  accept: (code, id, request, options) => acceptOutboundCampaignHandoff(code, id, request, options),
  reconcile: (code, id, options) => reconcileOutboundCampaignHandoff(code, id, options),
};

export interface OutboundCampaignHandoffSummary {
  readonly id: number;
  readonly campaignCode: string;
  readonly planID: string;
  readonly reviewVersion: 3;
  readonly status: "held";
  readonly targetCount: number;
  readonly stepCount: number;
  readonly acceptedAt: string;
}
export interface OutboundCampaignHandoffReconciliation extends OutboundCampaignHandoffSummary {
  readonly heldCount: number;
  readonly blockedCount: number;
  readonly pendingCount: number;
  readonly notEvaluatedCount: number;
  readonly eligibleCount: number;
  readonly inactiveCount: number;
  readonly contactPolicyCount: number;
}

function exact(value: unknown, keys: readonly string[]): Record<string, unknown> | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return undefined;
  const item = value as Record<string, unknown>;
  const actual = Object.keys(item);
  return actual.length === keys.length && actual.every((key) => keys.includes(key)) ? item : undefined;
}
const integer = (value: unknown, min: number, max: number): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= min && value <= max;
const utcMicroseconds = (value: unknown): value is string => {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,6})?Z$/u.exec(value);
  if (!match) return false;
  const parsed = new Date(value);
  return Number.isFinite(parsed.getTime()) && parsed.getUTCFullYear() === Number(match[1]) && parsed.getUTCMonth() + 1 === Number(match[2]) &&
    parsed.getUTCDate() === Number(match[3]) && parsed.getUTCHours() === Number(match[4]) && parsed.getUTCMinutes() === Number(match[5]) && parsed.getUTCSeconds() === Number(match[6]);
};
const utcOrder = (value: string): string => value.replace(/(?:\.(\d{1,6}))?Z$/u, (_match, fraction = "") => `.${fraction.padEnd(6, "0")}Z`);
const summaryKeys = ["id", "campaign_code", "plan_id", "review_version", "status", "target_count", "step_count", "accepted_at", "safety"] as const;
const reconciliationKeys = [...summaryKeys.slice(0, 7), "held_count", "blocked_count", "pending_count", "not_evaluated_count", "eligible_count", "inactive_count", "contact_policy_count", "accepted_at", "safety"] as const;
const validClosure = (plan: TouchPlanSummary, approved: TouchPlanReviewSnapshot): boolean =>
  /^[A-Za-z0-9._-]{1,96}$/u.test(plan.campaignCode) && /^ctp_[0-9a-f]{64}$/u.test(plan.id) &&
  typeof plan.immutable === "string" && plan.immutable.length > 0 && integer(plan.targetCount, 1, 1000) && integer(plan.stepCount, 1, 100) &&
  approved.review.status === "approved" && approved.review.version === 3 && approved.handoff?.status === "pending_outbound_acceptance" &&
  approved.handoff.reviewVersion === 3 && utcMicroseconds(approved.review.reviewedAt) && utcMicroseconds(approved.handoff.createdAt) &&
  approved.review.reviewedAt === approved.handoff.createdAt;

function parseSummaryRecord(item: Record<string, unknown>, plan: TouchPlanSummary, approved: TouchPlanReviewSnapshot): OutboundCampaignHandoffSummary | undefined {
  const safety = exact(item.safety, ["local_only", "provider_execution_eligible", "real_external_call_executed", "delivery_proven"]);
  if (!validClosure(plan, approved) || !integer(item.id, 1, Number.MAX_SAFE_INTEGER) || item.campaign_code !== plan.campaignCode || item.plan_id !== plan.id ||
    item.review_version !== 3 || item.status !== "held" || item.target_count !== plan.targetCount || item.step_count !== plan.stepCount ||
    !utcMicroseconds(item.accepted_at) || utcOrder(item.accepted_at) < utcOrder(approved.handoff!.createdAt) || !safety || safety.local_only !== true ||
    safety.provider_execution_eligible !== false || safety.real_external_call_executed !== false || safety.delivery_proven !== false) return undefined;
  return { id: item.id, campaignCode: plan.campaignCode, planID: plan.id, reviewVersion: 3, status: "held", targetCount: plan.targetCount, stepCount: plan.stepCount, acceptedAt: item.accepted_at };
}
export function parseOutboundCampaignHandoffSummary(value: unknown, plan: TouchPlanSummary, approved: TouchPlanReviewSnapshot): OutboundCampaignHandoffSummary | undefined {
  const item = exact(value, summaryKeys);
  return item ? parseSummaryRecord(item, plan, approved) : undefined;
}
export function parseOutboundCampaignHandoffReconciliation(value: unknown, plan: TouchPlanSummary, approved: TouchPlanReviewSnapshot, exactAccept: boolean): OutboundCampaignHandoffReconciliation | undefined {
  const item = exact(value, reconciliationKeys);
  if (!item) return undefined;
  const header = parseSummaryRecord(item, plan, approved);
  const counts = [item.held_count, item.blocked_count, item.pending_count, item.not_evaluated_count, item.eligible_count, item.inactive_count, item.contact_policy_count];
  if (!header || !counts.every((count) => integer(count, 0, 1000))) return undefined;
  const [held, blocked, pending, notEvaluated, eligible, inactive, contactPolicy] = counts as number[];
  if (held + blocked + pending !== header.targetCount || notEvaluated + eligible + inactive + contactPolicy !== header.targetCount ||
    exactAccept && (held !== header.targetCount || blocked !== 0 || pending !== 0 || notEvaluated !== header.targetCount || eligible !== 0 || inactive !== 0 || contactPolicy !== 0)) return undefined;
  return { ...header, heldCount: held, blockedCount: blocked, pendingCount: pending, notEvaluatedCount: notEvaluated, eligibleCount: eligible, inactiveCount: inactive, contactPolicyCount: contactPolicy };
}

export type HandoffLoadResult =
  | { readonly status: "loaded"; readonly summary: OutboundCampaignHandoffSummary }
  | { readonly status: "not_accepted" | "unauthenticated" | "forbidden" | "not_found" | "unavailable" };
const failed = (status: number): Exclude<HandoffLoadResult["status"], "loaded"> =>
  status === 401 ? "unauthenticated" : status === 403 ? "forbidden" : status === 404 ? "not_accepted" : "unavailable";
export async function loadOutboundCampaignHandoffSummary(transport: OutboundCampaignHandoffTransport, plan: TouchPlanSummary, approved: TouchPlanReviewSnapshot, signal?: AbortSignal): Promise<HandoffLoadResult> {
  if (!validClosure(plan, approved)) return { status: "unavailable" };
  try {
    const response = await transport.getSummary(plan.campaignCode, plan.id, { credentials: "same-origin", signal });
    if (response.status !== 200) return { status: failed(response.status) };
    const summary = parseOutboundCampaignHandoffSummary(response.data, plan, approved);
    return summary ? { status: "loaded", summary } : { status: "unavailable" };
  } catch { return { status: "unavailable" }; }
}
export async function loadOutboundCampaignHandoffReconciliation(transport: OutboundCampaignHandoffTransport, plan: TouchPlanSummary, approved: TouchPlanReviewSnapshot, signal?: AbortSignal): Promise<{ readonly status: "loaded"; readonly reconciliation: OutboundCampaignHandoffReconciliation } | { readonly status: "unauthenticated" | "forbidden" | "not_found" | "unavailable" }> {
  if (!validClosure(plan, approved)) return { status: "unavailable" };
  try {
    const response = await transport.reconcile(plan.campaignCode, plan.id, { credentials: "same-origin", signal });
    if (response.status !== 200) return { status: response.status === 401 ? "unauthenticated" : response.status === 403 ? "forbidden" : response.status === 404 ? "not_found" : "unavailable" };
    const reconciliation = parseOutboundCampaignHandoffReconciliation(response.data, plan, approved, false);
    return reconciliation ? { status: "loaded", reconciliation } : { status: "unavailable" };
  } catch { return { status: "unavailable" }; }
}

interface IntentPayload { readonly campaign_code: string; readonly plan_id: string; readonly plan_immutable: string; readonly review_version: 3; readonly handoff_created_at: string; readonly target_count: number; readonly step_count: number; }
interface PendingIntent { readonly version: 1; readonly idempotency_key: string; readonly payload: IntentPayload; }
export const outboundCampaignHandoffStorageKey = (actorID: number): string => `aicrm:outbound-campaign-handoff:v1:actor:${actorID}`;
const keyPattern = /^outbound-campaign-handoff:[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu;
function readPending(storage: SessionStorageLike, actorID: number): PendingIntent | "none" | "invalid" {
  try {
    const raw = storage.getItem(outboundCampaignHandoffStorageKey(actorID));
    if (raw === null) return "none";
    const value: unknown = JSON.parse(raw); const item = exact(value, ["version", "idempotency_key", "payload"]);
    const payload = exact(item?.payload, ["campaign_code", "plan_id", "plan_immutable", "review_version", "handoff_created_at", "target_count", "step_count"]);
    if (item?.version !== 1 || typeof item.idempotency_key !== "string" || !keyPattern.test(item.idempotency_key) || !payload) return "invalid";
    const normalized = { version: 1 as const, idempotency_key: item.idempotency_key, payload: payload as unknown as IntentPayload };
    return JSON.stringify(normalized) === raw ? normalized : "invalid";
  } catch { return "invalid"; }
}
function writePending(storage: SessionStorageLike, actorID: number, value: PendingIntent): boolean {
  try { const raw = JSON.stringify(value); storage.setItem(outboundCampaignHandoffStorageKey(actorID), raw); return storage.getItem(outboundCampaignHandoffStorageKey(actorID)) === raw; } catch { return false; }
}
function clearPending(storage: SessionStorageLike, actorID: number): void { try { storage.removeItem(outboundCampaignHandoffStorageKey(actorID)); } catch { /* deterministic result */ } }

export interface HandoffMutationInput { readonly plan: TouchPlanSummary; readonly approved: TouchPlanReviewSnapshot; readonly confirmation: string; readonly csrf: string; readonly signal?: AbortSignal; }
export type HandoffMutationResult =
  | { readonly status: "completed"; readonly reconciliation: OutboundCampaignHandoffReconciliation }
  | { readonly status: "conflict"; readonly summary?: OutboundCampaignHandoffSummary }
  | { readonly status: "confirmation_required" | "invalid" | "inflight" | "replay_required" | "replay_mismatch" | "no_pending" | "storage_unavailable" | "unauthenticated" | "forbidden" | "not_found" | "outcome_unknown" };

export class OutboundCampaignHandoffMachine {
  private active = false;
  constructor(private readonly options: { readonly transport: OutboundCampaignHandoffTransport; readonly sessionStorage: SessionStorageLike; readonly actorID: number; readonly keySource: { readonly randomUUID: () => string } }) {}
  start(input: HandoffMutationInput): Promise<HandoffMutationResult> { return this.run(input, false); }
  replay(input: HandoffMutationInput): Promise<HandoffMutationResult> { return this.run(input, true); }
  private intent(input: HandoffMutationInput): IntentPayload | "confirmation" | "invalid" {
    if (!validClosure(input.plan, input.approved) || !integer(this.options.actorID, 1, Number.MAX_SAFE_INTEGER) || !/^[A-Za-z0-9_-]{43}$/u.test(input.csrf)) return "invalid";
    if (input.confirmation !== `ACCEPT ${input.plan.id}`) return "confirmation";
    return { campaign_code: input.plan.campaignCode, plan_id: input.plan.id, plan_immutable: input.plan.immutable, review_version: 3, handoff_created_at: input.approved.handoff!.createdAt, target_count: input.plan.targetCount, step_count: input.plan.stepCount };
  }
  private async run(input: HandoffMutationInput, replay: boolean): Promise<HandoffMutationResult> {
    if (this.active) return { status: "inflight" }; this.active = true;
    try {
      const payload = this.intent(input); if (payload === "confirmation") return { status: "confirmation_required" }; if (payload === "invalid") return { status: "invalid" };
      const stored = readPending(this.options.sessionStorage, this.options.actorID); if (stored === "invalid") return { status: "storage_unavailable" };
      let pending: PendingIntent;
      if (replay) { if (stored === "none") return { status: "no_pending" }; if (JSON.stringify(stored.payload) !== JSON.stringify(payload)) return { status: "replay_mismatch" }; pending = stored; }
      else {
        if (stored !== "none") return { status: "replay_required" };
        let uuid = ""; try { uuid = this.options.keySource.randomUUID(); } catch { return { status: "invalid" }; }
        const idempotencyKey = `outbound-campaign-handoff:${uuid}`; if (!keyPattern.test(idempotencyKey)) return { status: "invalid" };
        pending = { version: 1, idempotency_key: idempotencyKey, payload }; if (!writePending(this.options.sessionStorage, this.options.actorID, pending)) return { status: "storage_unavailable" };
      }
      return await this.send(input, pending);
    } finally { this.active = false; }
  }
  private async send(input: HandoffMutationInput, pending: PendingIntent): Promise<HandoffMutationResult> {
    let response: TransportResponse;
    try { response = await this.options.transport.accept(pending.payload.campaign_code, pending.payload.plan_id, { expected_review_version: pending.payload.review_version }, { credentials: "same-origin", signal: input.signal, headers: { "X-CSRF-Token": input.csrf, "Idempotency-Key": pending.idempotency_key } }); }
    catch { return { status: "outcome_unknown" }; }
    if (response.status === 200) {
      const reconciliation = parseOutboundCampaignHandoffReconciliation(response.data, input.plan, input.approved, true);
      if (!reconciliation) return { status: "outcome_unknown" };
      clearPending(this.options.sessionStorage, this.options.actorID); return { status: "completed", reconciliation };
    }
    if (response.status === 409) {
      clearPending(this.options.sessionStorage, this.options.actorID);
      const refreshed = await loadOutboundCampaignHandoffSummary(this.options.transport, input.plan, input.approved, input.signal);
      return refreshed.status === "loaded" ? { status: "conflict", summary: refreshed.summary } : refreshed.status === "not_accepted" || refreshed.status === "unavailable" ? { status: "conflict" } : { status: refreshed.status };
    }
    if ([400, 401, 403, 404].includes(response.status)) {
      clearPending(this.options.sessionStorage, this.options.actorID);
      return { status: response.status === 401 ? "unauthenticated" : response.status === 403 ? "forbidden" : response.status === 404 ? "not_found" : "invalid" };
    }
    return { status: "outcome_unknown" };
  }
}
