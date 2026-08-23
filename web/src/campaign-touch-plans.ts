/* eslint-disable no-unused-vars -- transport function types need parameter declarations. */
import {
  acceptOutboundCampaignHandoff,
  createCloudCampaignTouchPlan,
  getCloudCampaignTouchPlanReview,
  getOutboundCampaignHandoffSummary,
  listAIAudiencePackages,
  listCloudCampaignTouchPlanRecipients,
  listCloudCampaignTouchPlans,
  listCloudCampaigns,
  mutateCloudCampaignTouchPlanReview,
  reconcileOutboundCampaignHandoff,
} from "./api/generated/health";
import { readCSRFCookie } from "./auth";

export type CampaignTouchPlanResponse = {
  readonly status: number;
  readonly data: unknown;
};

export interface CampaignTouchPlansTransport {
  readonly listCampaigns: () => Promise<CampaignTouchPlanResponse>;
  readonly listAudiencePackages: () => Promise<CampaignTouchPlanResponse>;
  readonly listPlans: (
    ...args: [string, string?]
  ) => Promise<CampaignTouchPlanResponse>;
  readonly createPlan: (
    ...args: [
      string,
      {
        expected_campaign_version: number;
        source: {
          kind: "ai_audience_package_members";
          audience_package_id: number;
        };
      },
      RequestInit,
    ]
  ) => Promise<CampaignTouchPlanResponse>;
  readonly listRecipients: (
    ...args: [string, string, string?]
  ) => Promise<CampaignTouchPlanResponse>;
  readonly getReview: (
    ...args: [string, string]
  ) => Promise<CampaignTouchPlanResponse>;
  readonly mutateReview: (
    ...args: [
      string,
      string,
      "submit" | "approve" | "reject",
      { expected_version: number; confirmation?: string },
      RequestInit,
    ]
  ) => Promise<CampaignTouchPlanResponse>;
  readonly getHandoff: (
    ...args: [string, string]
  ) => Promise<CampaignTouchPlanResponse>;
  readonly acceptHandoff: (
    ...args: [string, string, { expected_review_version: number }, RequestInit]
  ) => Promise<CampaignTouchPlanResponse>;
  readonly reconcileHandoff: (
    ...args: [string, string]
  ) => Promise<CampaignTouchPlanResponse>;
}

const asResponse = async (
  response: Promise<{ status: number; data: unknown }>,
): Promise<CampaignTouchPlanResponse> => response;

export const generatedCampaignTouchPlansTransport: CampaignTouchPlansTransport =
  {
    listCampaigns: () =>
      asResponse(listCloudCampaigns(undefined, { credentials: "same-origin" })),
    listAudiencePackages: () =>
      asResponse(
        listAIAudiencePackages(
          { limit: 100, offset: 0 },
          { credentials: "same-origin" },
        ),
      ),
    listPlans: (campaignCode, cursor) =>
      asResponse(
        listCloudCampaignTouchPlans(
          campaignCode,
          { limit: 100, ...(cursor ? { cursor } : {}) },
          { credentials: "same-origin" },
        ),
      ),
    createPlan: (campaignCode, request, options) =>
      asResponse(createCloudCampaignTouchPlan(campaignCode, request, options)),
    listRecipients: (campaignCode, planID, cursor) =>
      asResponse(
        listCloudCampaignTouchPlanRecipients(
          campaignCode,
          planID,
          { limit: 100, ...(cursor ? { cursor } : {}) },
          { credentials: "same-origin" },
        ),
      ),
    getReview: (campaignCode, planID) =>
      asResponse(
        getCloudCampaignTouchPlanReview(campaignCode, planID, {
          credentials: "same-origin",
        }),
      ),
    mutateReview: (campaignCode, planID, operation, request, options) =>
      asResponse(
        mutateCloudCampaignTouchPlanReview(
          campaignCode,
          planID,
          operation,
          request,
          options,
        ),
      ),
    getHandoff: (campaignCode, planID) =>
      asResponse(
        getOutboundCampaignHandoffSummary(campaignCode, planID, {
          credentials: "same-origin",
        }),
      ),
    acceptHandoff: (campaignCode, planID, request, options) =>
      asResponse(
        acceptOutboundCampaignHandoff(campaignCode, planID, request, options),
      ),
    reconcileHandoff: (campaignCode, planID) =>
      asResponse(
        reconcileOutboundCampaignHandoff(campaignCode, planID, {
          credentials: "same-origin",
        }),
      ),
  };

export interface Campaign {
  readonly code: string;
  readonly name: string;
  readonly version: number;
  readonly approval: "draft" | "approved" | "rejected";
  readonly runtime: "idle" | "planned" | "paused";
}
export interface AudiencePackage {
  readonly id: number;
  readonly name: string;
  readonly version: number;
  readonly members: number;
}
export interface TouchPlan {
  readonly id: string;
  readonly campaignCode: string;
  readonly campaignVersion: number;
  readonly targetCount: number;
  readonly stepCount: number;
  readonly packageID?: number;
}
export interface Review {
  readonly status: "draft" | "pending_review" | "approved" | "rejected";
  readonly version: number;
  readonly handoff?: {
    readonly reviewVersion: number;
    readonly createdAt: string;
  };
}
export interface RecipientPage {
  readonly ids: readonly number[];
  readonly nextCursor?: string;
}
export interface Handoff {
  readonly id: number;
  readonly campaignCode: string;
  readonly planID: string;
  readonly reviewVersion: number;
  readonly targetCount: number;
  readonly stepCount: number;
  readonly acceptedAt: string;
}
export interface Reconciliation extends Handoff {
  readonly heldCount: number;
  readonly blockedCount: number;
  readonly pendingCount: number;
  readonly notEvaluatedCount: number;
  readonly eligibleCount: number;
  readonly inactiveCount: number;
  readonly contactPolicyCount: number;
}

function record(
  value: unknown,
  keys: readonly string[],
): Record<string, unknown> | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return undefined;
  const candidate = value as Record<string, unknown>;
  return Object.keys(candidate).every((key) => keys.includes(key))
    ? candidate
    : undefined;
}
function text(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}
function integer(value: unknown, minimum = 1): number | undefined {
  return typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value >= minimum
    ? value
    : undefined;
}
function exactSafety(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  return (
    keys.every(
      (key) =>
        value[key] === (key === "local_projection" || key === "local_only"),
    ) && value.provider_execution_eligible !== true
  );
}
function campaign(value: unknown): Campaign | undefined {
  const item = record(value, [
    "campaign_code",
    "name",
    "approval_status",
    "runtime_status",
    "version",
    "created_by",
    "updated_by",
    "created_at",
    "updated_at",
  ]);
  if (!item) return undefined;
  const code = text(item.campaign_code);
  const name = text(item.name);
  const version = integer(item.version);
  if (
    !code ||
    !name ||
    !version ||
    !integer(item.created_by) ||
    !integer(item.updated_by) ||
    !text(item.created_at) ||
    !text(item.updated_at)
  )
    return undefined;
  if (
    item.approval_status !== "draft" &&
    item.approval_status !== "approved" &&
    item.approval_status !== "rejected"
  )
    return undefined;
  if (
    item.runtime_status !== "idle" &&
    item.runtime_status !== "planned" &&
    item.runtime_status !== "paused"
  )
    return undefined;
  return {
    code,
    name,
    version,
    approval: item.approval_status,
    runtime: item.runtime_status,
  };
}
function audience(value: unknown): AudiencePackage | undefined {
  const item = record(value, [
    "package_id",
    "name",
    "group_id",
    "lifecycle",
    "version",
    "refresh_mode",
    "refresh_cron",
    "member_count",
    "refreshed_at",
    "refresh_status",
    "created_at",
    "updated_at",
  ]);
  if (!item) return undefined;
  const id = integer(item.package_id);
  const name = text(item.name);
  const version = integer(item.version);
  const members = integer(item.member_count, 0);
  return id && name && version && members !== undefined
    ? { id, name, version, members }
    : undefined;
}
function plan(value: unknown): TouchPlan | undefined {
  const item = record(value, [
    "id",
    "campaign_code",
    "campaign_version",
    "source",
    "target_count",
    "target_digest",
    "content_step_count",
    "content_digest",
    "owner_actor_id",
    "preview_exclusion_summary",
    "created_at",
    "local_only",
    "provider_execution_eligible",
    "runtime_executed",
    "real_external_call_executed",
    "delivery_proven",
  ]);
  if (!item) return undefined;
  const id = text(item.id);
  const campaignCode = text(item.campaign_code);
  const campaignVersion = integer(item.campaign_version);
  const targetCount = integer(item.target_count);
  const stepCount = integer(item.content_step_count);
  if (
    !id ||
    !campaignCode ||
    !campaignVersion ||
    !targetCount ||
    !stepCount ||
    !integer(item.owner_actor_id) ||
    !text(item.created_at) ||
    !exactSafety(item, [
      "local_only",
      "provider_execution_eligible",
      "runtime_executed",
      "real_external_call_executed",
      "delivery_proven",
    ])
  )
    return undefined;
  const source = record(item.source, ["kind", "audience_package"]);
  const fact = source
    ? record(source.audience_package, [
        "package_id",
        "package_version",
        "member_snapshot_watermark",
        "digest",
      ])
    : undefined;
  const packageID = fact ? integer(fact.package_id) : undefined;
  if (source?.kind !== "ai_audience_package_members" || !packageID)
    return undefined;
  return {
    id,
    campaignCode,
    campaignVersion,
    targetCount,
    stepCount,
    packageID,
  };
}
function review(value: unknown): Review | undefined {
  const item = record(value, [
    "review",
    "handoff",
    "local_only",
    "provider_execution_eligible",
    "real_external_call_executed",
    "delivery_proven",
  ]);
  if (
    !item ||
    !exactSafety(item, [
      "local_only",
      "provider_execution_eligible",
      "real_external_call_executed",
      "delivery_proven",
    ])
  )
    return undefined;
  const fact = record(item.review, [
    "status",
    "version",
    "submitted_by_actor_id",
    "submitted_at",
    "reviewed_by_actor_id",
    "reviewed_at",
  ]);
  if (!fact) return undefined;
  const version = integer(fact.version);
  if (
    !version ||
    (fact.status !== "draft" &&
      fact.status !== "pending_review" &&
      fact.status !== "approved" &&
      fact.status !== "rejected")
  )
    return undefined;
  if (item.handoff === undefined) return { status: fact.status, version };
  const handoff = record(item.handoff, [
    "status",
    "review_version",
    "created_at",
  ]);
  if (!handoff || handoff.status !== "pending_outbound_acceptance")
    return undefined;
  const reviewVersion = integer(handoff.review_version, 3);
  const createdAt = text(handoff.created_at);
  return reviewVersion && createdAt
    ? { status: fact.status, version, handoff: { reviewVersion, createdAt } }
    : undefined;
}
function handoff(
  value: unknown,
  reconciliation: boolean,
): Handoff | Reconciliation | undefined {
  const base = [
    "id",
    "campaign_code",
    "plan_id",
    "review_version",
    "status",
    "target_count",
    "step_count",
    "accepted_at",
    "safety",
  ];
  const extra = [
    "held_count",
    "blocked_count",
    "pending_count",
    "not_evaluated_count",
    "eligible_count",
    "inactive_count",
    "contact_policy_count",
  ];
  const item = record(value, reconciliation ? [...base, ...extra] : base);
  const safety = item
    ? record(item.safety, [
        "local_only",
        "provider_execution_eligible",
        "real_external_call_executed",
        "delivery_proven",
      ])
    : undefined;
  if (
    !item ||
    !safety ||
    !exactSafety(safety, [
      "local_only",
      "provider_execution_eligible",
      "real_external_call_executed",
      "delivery_proven",
    ]) ||
    item.status !== "held"
  )
    return undefined;
  const id = integer(item.id);
  const campaignCode = text(item.campaign_code);
  const planID = text(item.plan_id);
  const reviewVersion = integer(item.review_version, 3);
  const targetCount = integer(item.target_count);
  const stepCount = integer(item.step_count);
  const acceptedAt = text(item.accepted_at);
  if (
    !id ||
    !campaignCode ||
    !planID ||
    !reviewVersion ||
    !targetCount ||
    !stepCount ||
    !acceptedAt
  )
    return undefined;
  const result: Handoff = {
    id,
    campaignCode,
    planID,
    reviewVersion,
    targetCount,
    stepCount,
    acceptedAt,
  };
  if (!reconciliation) return result;
  if (extra.some((key) => integer(item[key], 0) === undefined))
    return undefined;
  return {
    ...result,
    heldCount: item.held_count as number,
    blockedCount: item.blocked_count as number,
    pendingCount: item.pending_count as number,
    notEvaluatedCount: item.not_evaluated_count as number,
    eligibleCount: item.eligible_count as number,
    inactiveCount: item.inactive_count as number,
    contactPolicyCount: item.contact_policy_count as number,
  };
}

export function parseCampaigns(
  value: unknown,
): readonly Campaign[] | undefined {
  const body = record(value, [
    "items",
    "local_projection",
    "real_external_call_executed",
    "real_send",
    "runtime_executed",
  ]);
  if (
    !body ||
    !Array.isArray(body.items) ||
    !exactSafety(body, [
      "local_projection",
      "real_external_call_executed",
      "real_send",
      "runtime_executed",
    ])
  )
    return undefined;
  const items = body.items.map(campaign);
  return items.every(Boolean) ? (items as Campaign[]) : undefined;
}
export function parseAudiencePackages(
  value: unknown,
): readonly AudiencePackage[] | undefined {
  const body = record(value, [
    "items",
    "limit",
    "offset",
    "total",
    "local_projection",
    "real_external_call_executed",
  ]);
  if (
    !body ||
    !Array.isArray(body.items) ||
    body.local_projection !== true ||
    body.real_external_call_executed !== false
  )
    return undefined;
  const items = body.items.map(audience);
  return items.every(Boolean) ? (items as AudiencePackage[]) : undefined;
}
export function parsePlans(
  value: unknown,
):
  | { readonly items: readonly TouchPlan[]; readonly nextCursor?: string }
  | undefined {
  const body = record(value, [
    "items",
    "next_cursor",
    "local_only",
    "provider_execution_eligible",
    "runtime_executed",
    "real_external_call_executed",
    "delivery_proven",
  ]);
  if (
    !body ||
    !Array.isArray(body.items) ||
    !exactSafety(body, [
      "local_only",
      "provider_execution_eligible",
      "runtime_executed",
      "real_external_call_executed",
      "delivery_proven",
    ]) ||
    (body.next_cursor !== undefined &&
      body.next_cursor !== null &&
      !text(body.next_cursor))
  )
    return undefined;
  const items = body.items.map(plan);
  return items.every(Boolean)
    ? {
        items: items as TouchPlan[],
        ...(typeof body.next_cursor === "string"
          ? { nextCursor: body.next_cursor }
          : {}),
      }
    : undefined;
}
export function parseReview(value: unknown): Review | undefined {
  return review(value);
}
export function parseRecipients(value: unknown): RecipientPage | undefined {
  const body = record(value, [
    "items",
    "next_cursor",
    "local_only",
    "provider_execution_eligible",
    "real_external_call_executed",
    "delivery_proven",
  ]);
  if (
    !body ||
    !Array.isArray(body.items) ||
    !exactSafety(body, [
      "local_only",
      "provider_execution_eligible",
      "real_external_call_executed",
      "delivery_proven",
    ]) ||
    (body.next_cursor !== undefined &&
      body.next_cursor !== null &&
      !text(body.next_cursor))
  )
    return undefined;
  const ids = body.items.map((item) => {
    const row = record(item, ["canonical_customer_id"]);
    return row ? integer(row.canonical_customer_id) : undefined;
  });
  return ids.every((id) => id !== undefined)
    ? {
        ids: ids as number[],
        ...(typeof body.next_cursor === "string"
          ? { nextCursor: body.next_cursor }
          : {}),
      }
    : undefined;
}
export function parseHandoff(value: unknown): Handoff | undefined {
  const result = handoff(value, false);
  return result && !("heldCount" in result) ? result : undefined;
}
export function parseReconciliation(
  value: unknown,
): Reconciliation | undefined {
  const result = handoff(value, true);
  return result && "heldCount" in result ? result : undefined;
}

export type MutationResult =
  "ok" | "conflict" | "unauthenticated" | "unavailable" | "csrf_missing";
export function mutationOptions(
  cookieHeader: string,
  idempotencyKey: string,
): RequestInit | undefined {
  const csrf = readCSRFCookie(cookieHeader);
  if (!csrf || !/^[A-Za-z0-9:_-]{16,128}$/u.test(idempotencyKey))
    return undefined;
  return {
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrf, "Idempotency-Key": idempotencyKey },
  };
}
export function mutationResult(status: number): MutationResult {
  return status === 200 || status === 201
    ? "ok"
    : status === 409
      ? "conflict"
      : status === 401
        ? "unauthenticated"
        : "unavailable";
}
export function reviewRequest(
  operation: "submit" | "approve" | "reject",
  planID: string,
  expectedVersion: number,
  confirmation = "",
): { expected_version: number; confirmation?: string } | undefined {
  if (!integer(expectedVersion)) return undefined;
  if (operation === "submit") return { expected_version: expectedVersion };
  return confirmation === `${operation.toUpperCase()} ${planID}`
    ? { expected_version: expectedVersion, confirmation }
    : undefined;
}
