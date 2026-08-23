/* eslint-disable no-unused-vars -- transport parameters define the UI integration seam. */
import {
  createCloudCampaignTouchPlan,
  getCloudCampaign,
  getCloudCampaignTouchPlan,
  listCloudCampaigns,
  type CloudCampaignTouchPlanCreateRequest,
  type ListCloudCampaignsParams,
} from "./api/generated/health";

export type TransportResponse = {
  readonly status: number;
  readonly data: unknown;
};
export interface CampaignTouchPlanTransport {
  readonly listCampaigns: (
    params: ListCloudCampaignsParams,
    options: RequestInit,
  ) => Promise<TransportResponse>;
  readonly getCampaign: (
    campaignCode: string,
    options: RequestInit,
  ) => Promise<TransportResponse>;
  readonly createPlan: (
    campaignCode: string,
    request: CloudCampaignTouchPlanCreateRequest,
    options: RequestInit,
  ) => Promise<TransportResponse>;
  readonly getPlan: (
    campaignCode: string,
    planID: string,
    options: RequestInit,
  ) => Promise<TransportResponse>;
}

const response = async (
  value: Promise<{ readonly status: number; readonly data: unknown }>,
): Promise<TransportResponse> => value;
export const generatedCampaignTouchPlanTransport: CampaignTouchPlanTransport = {
  listCampaigns: (params, options) =>
    response(listCloudCampaigns(params, options)),
  getCampaign: (code, options) => response(getCloudCampaign(code, options)),
  createPlan: (code, request, options) =>
    response(createCloudCampaignTouchPlan(code, request, options)),
  getPlan: (code, id, options) =>
    response(getCloudCampaignTouchPlan(code, id, options)),
};

export interface CampaignDraftSummary {
  readonly code: string;
  readonly name: string;
  readonly version: number;
  readonly createdBy: number;
  readonly updatedBy: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}
export interface CampaignDraft extends CampaignDraftSummary {
  readonly steps: readonly CampaignDraftStep[];
}
export interface CampaignDraftStep {
  readonly index: number;
  readonly delayMinutes: number;
  readonly content: string;
}
export type CampaignDraftListResult =
  | {
      readonly status: "loaded";
      readonly campaigns: readonly CampaignDraftSummary[];
    }
  | { readonly status: "unauthenticated" | "forbidden" | "unavailable" };
export type CampaignDraftResult =
  | { readonly status: "loaded"; readonly campaign: CampaignDraft }
  | { readonly status: "unauthenticated" | "forbidden" | "unavailable" };

type PlanSource =
  | {
      readonly kind: "customer_selection";
      readonly id: "local_selection";
      readonly version: "v1";
      readonly digest: string;
    }
  | {
      readonly kind: "segment_members";
      readonly segmentID: number;
      readonly watermark: string;
      readonly digest: string;
    }
  | {
      readonly kind: "ai_audience_package_members";
      readonly packageID: number;
      readonly packageVersion: number;
      readonly watermark: string;
      readonly digest: string;
    };
export interface TouchPlanProjection {
  readonly id: string;
  readonly campaignCode: string;
  readonly campaignVersion: number;
  readonly source: PlanSource;
  readonly targetCount: number;
  readonly targetDigest: string;
  readonly steps: readonly CampaignDraftStep[];
  readonly contentDigest: string;
  readonly ownerActorID: number;
  readonly candidateCount: number;
  readonly activeCustomerCount: number;
  readonly inactiveExcludedCount: number;
  readonly policyExcludedCount: number;
  readonly createdAt: string;
  readonly localOnly: true;
  readonly providerExecutionEligible: false;
  readonly runtimeExecuted: false;
  readonly realExternalCallExecuted: false;
  readonly deliveryProven: false;
}

function exact(
  value: unknown,
  keys: readonly string[],
): Record<string, unknown> | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return undefined;
  const candidate = value as Record<string, unknown>;
  const actual = Object.keys(candidate);
  return actual.length === keys.length &&
    actual.every((key) => keys.includes(key))
    ? candidate
    : undefined;
}
const positive = (
  value: unknown,
  maximum = Number.MAX_SAFE_INTEGER,
): value is number =>
  typeof value === "number" &&
  Number.isSafeInteger(value) &&
  value > 0 &&
  value <= maximum;
const nonnegative = (
  value: unknown,
  maximum = Number.MAX_SAFE_INTEGER,
): value is number =>
  typeof value === "number" &&
  Number.isSafeInteger(value) &&
  value >= 0 &&
  value <= maximum;
const safeText = (value: unknown, maximum: number): value is string =>
  typeof value === "string" && value.length > 0 && [...value].length <= maximum;
const digest = (value: unknown): value is string =>
  typeof value === "string" && /^[0-9a-f]{64}$/u.test(value);
const utcTimestamp = (value: unknown): value is string => {
  if (typeof value !== "string") return false;
  const match =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?Z$/u.exec(value);
  if (!match) return false;
  const parsed = new Date(value);
  const [, year, month, day, hour, minute, second] = match.map(Number);
  return (
    Number.isFinite(parsed.getTime()) &&
    parsed.getUTCFullYear() === year &&
    parsed.getUTCMonth() === month - 1 &&
    parsed.getUTCDate() === day &&
    parsed.getUTCHours() === hour &&
    parsed.getUTCMinutes() === minute &&
    parsed.getUTCSeconds() === second
  );
};

function campaign(value: unknown): CampaignDraftSummary | undefined {
  const item = exact(value, [
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
  return item &&
    typeof item.campaign_code === "string" &&
    /^[A-Za-z0-9._-]{1,96}$/u.test(item.campaign_code) &&
    safeText(item.name, 160) &&
    item.approval_status === "draft" &&
    item.runtime_status === "idle" &&
    positive(item.version) &&
    positive(item.created_by) &&
    positive(item.updated_by) &&
    utcTimestamp(item.created_at) &&
    utcTimestamp(item.updated_at)
    ? {
        code: item.campaign_code,
        name: item.name,
        version: item.version,
        createdBy: item.created_by,
        updatedBy: item.updated_by,
        createdAt: item.created_at,
        updatedAt: item.updated_at,
      }
    : undefined;
}
function campaignStep(
  value: unknown,
  expectedIndex: number,
): CampaignDraftStep | undefined {
  const item = exact(value, ["step_index", "delay_minutes", "content"]);
  return item &&
    item.step_index === expectedIndex &&
    nonnegative(item.delay_minutes, 2_147_483_647) &&
    safeText(item.content, 4000)
    ? {
        index: expectedIndex,
        delayMinutes: item.delay_minutes,
        content: item.content,
      }
    : undefined;
}
function campaignDetail(
  value: unknown,
  expectedCode: string,
): CampaignDraft | undefined {
  const body = exact(value, [
    "campaign",
    "steps",
    "local_projection",
    "real_external_call_executed",
    "real_send",
    "runtime_executed",
  ]);
  if (
    !body ||
    body.local_projection !== true ||
    body.real_external_call_executed !== false ||
    body.real_send !== false ||
    body.runtime_executed !== false ||
    !Array.isArray(body.steps) ||
    body.steps.length < 1 ||
    body.steps.length > 100
  )
    return undefined;
  const summary = campaign(body.campaign);
  if (!summary || summary.code !== expectedCode) return undefined;
  const steps = body.steps.map((item, index) => campaignStep(item, index + 1));
  return steps.every(Boolean)
    ? { ...summary, steps: steps as CampaignDraftStep[] }
    : undefined;
}

function failure(
  status: number,
): "unauthenticated" | "forbidden" | "unavailable" {
  return status === 401
    ? "unauthenticated"
    : status === 403
      ? "forbidden"
      : "unavailable";
}
export async function loadDraftCampaigns(
  transport: CampaignTouchPlanTransport,
): Promise<CampaignDraftListResult> {
  try {
    const result = await transport.listCampaigns(
      { approval_status: "draft", runtime_status: "idle" },
      { credentials: "same-origin" },
    );
    if (result.status !== 200) return { status: failure(result.status) };
    const body = exact(result.data, [
      "items",
      "local_projection",
      "real_external_call_executed",
      "real_send",
      "runtime_executed",
    ]);
    if (
      !body ||
      body.local_projection !== true ||
      body.real_external_call_executed !== false ||
      body.real_send !== false ||
      body.runtime_executed !== false ||
      !Array.isArray(body.items) ||
      body.items.length > 100
    )
      return { status: "unavailable" };
    const campaigns = body.items.map(campaign);
    const codes = campaigns.map((item) => item?.code);
    return campaigns.every(Boolean) && new Set(codes).size === codes.length
      ? { status: "loaded", campaigns: campaigns as CampaignDraftSummary[] }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
export async function loadDraftCampaign(
  transport: CampaignTouchPlanTransport,
  campaignCode: string,
): Promise<CampaignDraftResult> {
  if (!/^[A-Za-z0-9._-]{1,96}$/u.test(campaignCode))
    return { status: "unavailable" };
  try {
    const result = await transport.getCampaign(campaignCode, {
      credentials: "same-origin",
    });
    if (result.status !== 200) return { status: failure(result.status) };
    const parsed = campaignDetail(result.data, campaignCode);
    return parsed
      ? { status: "loaded", campaign: parsed }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

function planSource(value: unknown): PlanSource | undefined {
  const base = exact(value, ["kind", "customer_selection"]);
  if (base?.kind === "customer_selection") {
    const fact = exact(base.customer_selection, ["id", "version", "digest"]);
    return fact?.id === "local_selection" &&
      fact.version === "v1" &&
      digest(fact.digest)
      ? {
          kind: "customer_selection",
          id: "local_selection",
          version: "v1",
          digest: fact.digest,
        }
      : undefined;
  }
  const segment = exact(value, ["kind", "segment"]);
  if (segment?.kind === "segment_members") {
    const fact = exact(segment.segment, [
      "segment_id",
      "member_snapshot_watermark",
      "digest",
    ]);
    return fact &&
      positive(fact.segment_id) &&
      utcTimestamp(fact.member_snapshot_watermark) &&
      digest(fact.digest)
      ? {
          kind: "segment_members",
          segmentID: fact.segment_id,
          watermark: fact.member_snapshot_watermark,
          digest: fact.digest,
        }
      : undefined;
  }
  const audience = exact(value, ["kind", "audience_package"]);
  if (audience?.kind !== "ai_audience_package_members") return undefined;
  const fact = exact(audience.audience_package, [
    "package_id",
    "package_version",
    "member_snapshot_watermark",
    "digest",
  ]);
  return fact &&
    positive(fact.package_id) &&
    positive(fact.package_version) &&
    utcTimestamp(fact.member_snapshot_watermark) &&
    digest(fact.digest)
    ? {
        kind: "ai_audience_package_members",
        packageID: fact.package_id,
        packageVersion: fact.package_version,
        watermark: fact.member_snapshot_watermark,
        digest: fact.digest,
      }
    : undefined;
}
function touchPlan(value: unknown): TouchPlanProjection | undefined {
  const body = exact(value, [
    "id",
    "campaign_code",
    "campaign_version",
    "source",
    "target_count",
    "target_digest",
    "content",
    "owner_actor_id",
    "preview_exclusion_summary",
    "created_at",
    "local_only",
    "provider_execution_eligible",
    "runtime_executed",
    "real_external_call_executed",
    "delivery_proven",
  ]);
  if (
    !body ||
    typeof body.id !== "string" ||
    !/^ctp_[0-9a-f]{64}$/u.test(body.id) ||
    typeof body.campaign_code !== "string" ||
    !/^[A-Za-z0-9._-]{1,96}$/u.test(body.campaign_code) ||
    !positive(body.campaign_version) ||
    !positive(body.target_count, 1000) ||
    !digest(body.target_digest) ||
    !positive(body.owner_actor_id) ||
    !utcTimestamp(body.created_at) ||
    body.local_only !== true ||
    body.provider_execution_eligible !== false ||
    body.runtime_executed !== false ||
    body.real_external_call_executed !== false ||
    body.delivery_proven !== false
  )
    return undefined;
  const source = planSource(body.source);
  const content = exact(body.content, ["steps", "content_digest"]);
  const preview = exact(body.preview_exclusion_summary, [
    "candidate_count",
    "active_customer_count",
    "inactive_excluded_count",
    "policy_excluded_count",
  ]);
  if (
    !source ||
    !content ||
    !Array.isArray(content.steps) ||
    content.steps.length < 1 ||
    content.steps.length > 100 ||
    !digest(content.content_digest) ||
    !preview ||
    !positive(preview.candidate_count, 1000) ||
    !positive(preview.active_customer_count, 1000) ||
    !nonnegative(preview.inactive_excluded_count, 1000) ||
    !nonnegative(preview.policy_excluded_count, 1000) ||
    preview.candidate_count !==
      preview.active_customer_count + preview.inactive_excluded_count ||
    body.target_count + preview.policy_excluded_count !==
      preview.active_customer_count
  )
    return undefined;
  const steps = content.steps.map((item, index) =>
    campaignStep(item, index + 1),
  );
  return steps.every(Boolean)
    ? {
        id: body.id,
        campaignCode: body.campaign_code,
        campaignVersion: body.campaign_version,
        source,
        targetCount: body.target_count,
        targetDigest: body.target_digest,
        steps: steps as CampaignDraftStep[],
        contentDigest: content.content_digest,
        ownerActorID: body.owner_actor_id,
        candidateCount: preview.candidate_count,
        activeCustomerCount: preview.active_customer_count,
        inactiveExcludedCount: preview.inactive_excluded_count,
        policyExcludedCount: preview.policy_excluded_count,
        createdAt: body.created_at,
        localOnly: true,
        providerExecutionEligible: false,
        runtimeExecuted: false,
        realExternalCallExecuted: false,
        deliveryProven: false,
      }
    : undefined;
}

export interface SessionStorageLike {
  readonly getItem: (key: string) => string | null;
  readonly setItem: (key: string, value: string) => void;
  readonly removeItem: (key: string) => void;
}
export const campaignTouchPlanPendingStorageKey = (actorID: number): string =>
  `aicrm:campaign-touch-plan:v1:actor:${actorID}`;

type IntentSource = CloudCampaignTouchPlanCreateRequest["source"];
interface IntentPayload {
  readonly campaign_code: string;
  readonly expected_campaign_version: number;
  readonly source: IntentSource;
}
interface PendingIntent {
  readonly version: 1;
  readonly idempotency_key: string;
  readonly payload: IntentPayload;
}
type PendingRead =
  | { readonly status: "none" }
  | { readonly status: "invalid" }
  | {
      readonly status: "loaded";
      readonly value: PendingIntent;
      readonly raw: string;
    };

function validSourceRequest(value: unknown): value is IntentSource {
  const customer = exact(value, ["kind", "customer_ids"]);
  if (customer?.kind === "customer_selection")
    return (
      Array.isArray(customer.customer_ids) &&
      customer.customer_ids.length === 1 &&
      positive(customer.customer_ids[0])
    );
  const segment = exact(value, ["kind", "segment_id"]);
  if (segment?.kind === "segment_members") return positive(segment.segment_id);
  const audience = exact(value, ["kind", "audience_package_id"]);
  return (
    audience?.kind === "ai_audience_package_members" &&
    positive(audience.audience_package_id)
  );
}
function validPayload(value: unknown): value is IntentPayload {
  const payload = exact(value, [
    "campaign_code",
    "expected_campaign_version",
    "source",
  ]);
  return Boolean(
    payload &&
    typeof payload.campaign_code === "string" &&
    /^[A-Za-z0-9._-]{1,96}$/u.test(payload.campaign_code) &&
    positive(payload.expected_campaign_version) &&
    validSourceRequest(payload.source),
  );
}
function canonicalPayload(value: IntentPayload): IntentPayload {
  let source: IntentSource;
  if (value.source.kind === "customer_selection")
    source = {
      kind: "customer_selection",
      customer_ids: [value.source.customer_ids[0]],
    };
  else if (value.source.kind === "segment_members")
    source = { kind: "segment_members", segment_id: value.source.segment_id };
  else
    source = {
      kind: "ai_audience_package_members",
      audience_package_id: value.source.audience_package_id,
    };
  return {
    campaign_code: value.campaign_code,
    expected_campaign_version: value.expected_campaign_version,
    source,
  };
}
const campaignTouchPlanKeyPrefix = "campaign-touch-plan:";
const campaignTouchPlanUUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu;
const validCampaignTouchPlanUUID = (value: unknown): value is string =>
  typeof value === "string" && campaignTouchPlanUUID.test(value);
const validKey = (value: unknown): value is string =>
  typeof value === "string" &&
  value.startsWith(campaignTouchPlanKeyPrefix) &&
  validCampaignTouchPlanUUID(value.slice(campaignTouchPlanKeyPrefix.length));
function readPending(
  storage: SessionStorageLike,
  actorID: number,
): PendingRead {
  try {
    const raw = storage.getItem(campaignTouchPlanPendingStorageKey(actorID));
    if (raw === null) return { status: "none" };
    const value: unknown = JSON.parse(raw);
    const item = exact(value, ["version", "idempotency_key", "payload"]);
    if (
      !item ||
      item.version !== 1 ||
      !validKey(item.idempotency_key) ||
      !validPayload(item.payload)
    )
      return { status: "invalid" };
    const normalized: PendingIntent = {
      version: 1,
      idempotency_key: item.idempotency_key,
      payload: canonicalPayload(item.payload),
    };
    return JSON.stringify(normalized) === raw
      ? { status: "loaded", value: normalized, raw }
      : { status: "invalid" };
  } catch {
    return { status: "invalid" };
  }
}
function writePending(
  storage: SessionStorageLike,
  actorID: number,
  value: PendingIntent,
): boolean {
  try {
    storage.setItem(
      campaignTouchPlanPendingStorageKey(actorID),
      JSON.stringify(value),
    );
    return (
      storage.getItem(campaignTouchPlanPendingStorageKey(actorID)) ===
      JSON.stringify(value)
    );
  } catch {
    return false;
  }
}
function clearPending(storage: SessionStorageLike, actorID: number): void {
  try {
    storage.removeItem(campaignTouchPlanPendingStorageKey(actorID));
  } catch {
    // A successful local fact remains successful even if browser storage cleanup fails.
  }
}

export class CampaignTouchPlanInflightGuard {
  private active = false;
  enter(): boolean {
    if (this.active) return false;
    this.active = true;
    return true;
  }
  leave(): void {
    this.active = false;
  }
  isActive(): boolean {
    return this.active;
  }
}

export interface CampaignTouchPlanInput {
  readonly campaign: CampaignDraft;
  readonly source_kind: string;
  readonly source_id: string;
  readonly csrf: string;
  readonly confirmed: boolean;
}
export type CampaignTouchPlanResult =
  | { readonly status: "created"; readonly plan: TouchPlanProjection }
  | { readonly status: "conflict"; readonly campaign?: CampaignDraft }
  | {
      readonly status: "blocked_redline";
      readonly reason: "unsafe_source_id" | "server";
    }
  | {
      readonly status:
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

type IntentResult =
  | { readonly status: "valid"; readonly payload: IntentPayload }
  | { readonly status: "invalid" }
  | { readonly status: "blocked_redline" };
function validDraft(value: CampaignDraft): boolean {
  return (
    /^[A-Za-z0-9._-]{1,96}$/u.test(value.code) &&
    safeText(value.name, 160) &&
    positive(value.version) &&
    positive(value.createdBy) &&
    positive(value.updatedBy) &&
    utcTimestamp(value.createdAt) &&
    utcTimestamp(value.updatedAt) &&
    Array.isArray(value.steps) &&
    value.steps.length >= 1 &&
    value.steps.length <= 100 &&
    value.steps.every(
      (step, index) =>
        step.index === index + 1 &&
        nonnegative(step.delayMinutes, 2_147_483_647) &&
        safeText(step.content, 4000),
    )
  );
}
function intent(input: CampaignTouchPlanInput): IntentResult {
  if (!validDraft(input.campaign) || typeof input.source_id !== "string")
    return { status: "invalid" };
  if (!/^[1-9][0-9]*$/u.test(input.source_id)) return { status: "invalid" };
  const sourceID = Number(input.source_id);
  if (!Number.isSafeInteger(sourceID) || sourceID < 1)
    return { status: "blocked_redline" };
  let source: IntentSource;
  if (input.source_kind === "customer_selection")
    source = { kind: "customer_selection", customer_ids: [sourceID] };
  else if (input.source_kind === "segment_members")
    source = { kind: "segment_members", segment_id: sourceID };
  else if (input.source_kind === "ai_audience_package_members")
    source = {
      kind: "ai_audience_package_members",
      audience_package_id: sourceID,
    };
  else return { status: "invalid" };
  return {
    status: "valid",
    payload: {
      campaign_code: input.campaign.code,
      expected_campaign_version: input.campaign.version,
      source,
    },
  };
}
function newKey(source: {
  readonly randomUUID: () => string;
}): string | undefined {
  try {
    const uuid = source.randomUUID();
    return validCampaignTouchPlanUUID(uuid)
      ? `${campaignTouchPlanKeyPrefix}${uuid}`
      : undefined;
  } catch {
    return undefined;
  }
}
function sameSteps(
  left: readonly CampaignDraftStep[],
  right: readonly CampaignDraftStep[],
): boolean {
  return (
    left.length === right.length &&
    left.every(
      (step, index) =>
        step.index === right[index]?.index &&
        step.delayMinutes === right[index]?.delayMinutes &&
        step.content === right[index]?.content,
    )
  );
}
function planMatchesIntent(
  plan: TouchPlanProjection,
  payload: IntentPayload,
  campaign: CampaignDraft,
  actorID: number,
): boolean {
  if (
    plan.campaignCode !== payload.campaign_code ||
    plan.campaignVersion !== payload.expected_campaign_version ||
    plan.ownerActorID !== actorID ||
    !sameSteps(plan.steps, campaign.steps) ||
    plan.source.kind !== payload.source.kind
  )
    return false;
  if (payload.source.kind === "customer_selection")
    return (
      plan.candidateCount === 1 && plan.source.kind === "customer_selection"
    );
  if (payload.source.kind === "segment_members")
    return (
      plan.source.kind === "segment_members" &&
      plan.source.segmentID === payload.source.segment_id
    );
  return (
    plan.source.kind === "ai_audience_package_members" &&
    plan.source.packageID === payload.source.audience_package_id
  );
}

export class CampaignTouchPlanMachine {
  private readonly transport: CampaignTouchPlanTransport;
  private readonly storage: SessionStorageLike;
  private readonly actorID: number;
  private readonly keySource: { readonly randomUUID: () => string };
  private readonly guard: CampaignTouchPlanInflightGuard;

  constructor(options: {
    readonly transport: CampaignTouchPlanTransport;
    readonly sessionStorage: SessionStorageLike;
    readonly actorID: number;
    readonly keySource: { readonly randomUUID: () => string };
    readonly inflightGuard?: CampaignTouchPlanInflightGuard;
  }) {
    this.transport = options.transport;
    this.storage = options.sessionStorage;
    this.actorID = options.actorID;
    this.keySource = options.keySource;
    this.guard = options.inflightGuard ?? new CampaignTouchPlanInflightGuard();
  }

  start(input: CampaignTouchPlanInput): Promise<CampaignTouchPlanResult> {
    return this.run(input, false);
  }
  replay(input: CampaignTouchPlanInput): Promise<CampaignTouchPlanResult> {
    return this.run(input, true);
  }

  private async run(
    input: CampaignTouchPlanInput,
    replay: boolean,
  ): Promise<CampaignTouchPlanResult> {
    if (!this.guard.enter()) return { status: "inflight" };
    try {
      if (!input.confirmed) return { status: "confirmation_required" };
      if (!/^[A-Za-z0-9_-]{43}$/u.test(input.csrf) || !positive(this.actorID))
        return { status: "invalid" };
      const next = intent(input);
      if (next.status === "invalid") return { status: "invalid" };
      if (next.status === "blocked_redline")
        return { status: "blocked_redline", reason: "unsafe_source_id" };
      const pending = readPending(this.storage, this.actorID);
      if (pending.status === "invalid")
        return { status: "storage_unavailable" };

      let active: PendingIntent;
      if (replay) {
        if (pending.status === "none") return { status: "no_pending" };
        if (
          JSON.stringify(pending.value.payload) !== JSON.stringify(next.payload)
        )
          return { status: "replay_mismatch" };
        active = pending.value;
      } else {
        if (pending.status === "loaded") return { status: "replay_required" };
        const idempotencyKey = newKey(this.keySource);
        if (!idempotencyKey) return { status: "invalid" };
        active = {
          version: 1,
          idempotency_key: idempotencyKey,
          payload: next.payload,
        };
        if (!writePending(this.storage, this.actorID, active))
          return { status: "storage_unavailable" };
      }
      return await this.send(input, active);
    } finally {
      this.guard.leave();
    }
  }

  private async send(
    input: CampaignTouchPlanInput,
    active: PendingIntent,
  ): Promise<CampaignTouchPlanResult> {
    let created: TransportResponse;
    try {
      created = await this.transport.createPlan(
        active.payload.campaign_code,
        {
          expected_campaign_version: active.payload.expected_campaign_version,
          source: active.payload.source,
        },
        {
          credentials: "same-origin",
          headers: {
            "X-CSRF-Token": input.csrf,
            "Idempotency-Key": active.idempotency_key,
          },
        },
      );
    } catch {
      return { status: "outcome_unknown" };
    }
    if (created.status === 409) {
      const error = exact(created.data, ["code"]);
      if (error?.code === "BLOCKED_REDLINE") {
        clearPending(this.storage, this.actorID);
        return { status: "blocked_redline", reason: "server" };
      }
      if (error?.code !== "CONFLICT") return { status: "outcome_unknown" };
      clearPending(this.storage, this.actorID);
      const refreshed = await loadDraftCampaign(
        this.transport,
        active.payload.campaign_code,
      );
      return refreshed.status === "loaded"
        ? { status: "conflict", campaign: refreshed.campaign }
        : { status: "conflict" };
    }
    if ([400, 401, 403, 404].includes(created.status)) {
      clearPending(this.storage, this.actorID);
      if (created.status === 401) return { status: "unauthenticated" };
      if (created.status === 403) return { status: "forbidden" };
      if (created.status === 404) return { status: "not_found" };
      return { status: "invalid" };
    }
    if (created.status !== 201) return { status: "outcome_unknown" };
    const first = touchPlan(created.data);
    if (
      !first ||
      !planMatchesIntent(first, active.payload, input.campaign, this.actorID)
    )
      return { status: "outcome_unknown" };
    let readback: TransportResponse;
    try {
      readback = await this.transport.getPlan(first.campaignCode, first.id, {
        credentials: "same-origin",
      });
    } catch {
      return { status: "outcome_unknown" };
    }
    if (readback.status !== 200) return { status: "outcome_unknown" };
    const second = touchPlan(readback.data);
    if (
      !second ||
      !planMatchesIntent(
        second,
        active.payload,
        input.campaign,
        this.actorID,
      ) ||
      JSON.stringify(first) !== JSON.stringify(second)
    )
      return { status: "outcome_unknown" };
    clearPending(this.storage, this.actorID);
    return { status: "created", plan: second };
  }
}
