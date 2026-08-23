/* eslint-disable no-unused-vars -- transport inputs are the generated API seam. */
import {
  getCloudCampaignTouchPlan,
  listCloudCampaignTouchPlanRecipients,
  listCloudCampaignTouchPlans,
} from "./api/generated/health";
import {
  generatedCampaignTouchPlanTransport,
  type CampaignTouchPlanTransport,
} from "./campaign-touch-plan-core";

export interface CampaignTouchPlanReadTransport extends CampaignTouchPlanTransport {
  readonly listPlans: (
    ...args: Parameters<typeof listCloudCampaignTouchPlans>
  ) => Promise<{ readonly status: number; readonly data: unknown }>;
  readonly getPlan: (
    ...args: Parameters<typeof getCloudCampaignTouchPlan>
  ) => Promise<{ readonly status: number; readonly data: unknown }>;
  readonly listRecipients: (
    ...args: Parameters<typeof listCloudCampaignTouchPlanRecipients>
  ) => Promise<{ readonly status: number; readonly data: unknown }>;
}
export const generatedCampaignTouchPlanReadTransport: CampaignTouchPlanReadTransport =
  {
    ...generatedCampaignTouchPlanTransport,
    listPlans: (code, params, options) =>
      listCloudCampaignTouchPlans(code, params, options),
    getPlan: (code, id, options) =>
      getCloudCampaignTouchPlan(code, id, options),
    listRecipients: (code, id, params, options) =>
      listCloudCampaignTouchPlanRecipients(code, id, params, options),
  };
export type ReadFailure =
  "unauthenticated" | "forbidden" | "not_found" | "unavailable";
export interface TouchPlanSource {
  readonly kind:
    "customer_selection" | "segment_members" | "ai_audience_package_members";
  readonly digest: string;
  readonly id?: number;
  readonly watermark?: string;
  readonly version?: number;
}
export interface TouchPlanSummary {
  readonly id: string;
  readonly campaignCode: string;
  readonly campaignVersion: number;
  readonly source: TouchPlanSource;
  readonly targetCount: number;
  readonly targetDigest: string;
  readonly stepCount: number;
  readonly contentDigest: string;
  readonly immutable: string;
}
export type PlansResult =
  | { readonly status: "loaded"; readonly plans: readonly TouchPlanSummary[] }
  | { readonly status: ReadFailure };
export type DetailResult =
  | { readonly status: "loaded"; readonly plan: TouchPlanSummary }
  | { readonly status: ReadFailure };
export type RecipientsResult =
  | {
      readonly status: "loaded";
      readonly recipients: readonly number[];
      readonly nextCursor?: string;
    }
  | { readonly status: ReadFailure };

function exact(
  value: unknown,
  keys: readonly string[],
): Record<string, unknown> | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return undefined;
  const item = value as Record<string, unknown>;
  const actual = Object.keys(item);
  return actual.every((key) => keys.includes(key)) &&
    keys.every((key) => key === "next_cursor" || actual.includes(key))
    ? item
    : undefined;
}
const positive = (
  value: unknown,
  maximum = Number.MAX_SAFE_INTEGER,
): value is number =>
  typeof value === "number" &&
  Number.isSafeInteger(value) &&
  value >= 1 &&
  value <= maximum;
const nonnegative = (
  value: unknown,
  maximum = Number.MAX_SAFE_INTEGER,
): value is number =>
  typeof value === "number" &&
  Number.isSafeInteger(value) &&
  value >= 0 &&
  value <= maximum;
const digest = (value: unknown): value is string =>
  typeof value === "string" && /^[0-9a-f]{64}$/u.test(value);
const campaignCode = (value: unknown): value is string =>
  typeof value === "string" && /^[A-Za-z0-9._-]{1,96}$/u.test(value);
const planID = (value: unknown): value is string =>
  typeof value === "string" && /^ctp_[0-9a-f]{64}$/u.test(value);
const utcMicroseconds = (value: unknown): value is string => {
  if (typeof value !== "string") return false;
  const match =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})\.(\d{6})Z$/u.exec(value);
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
const failure = (status: number): ReadFailure =>
  status === 401
    ? "unauthenticated"
    : status === 403
      ? "forbidden"
      : status === 404
        ? "not_found"
        : "unavailable";
function safety(value: Record<string, unknown>, runtime = true): boolean {
  return (
    value.local_only === true &&
    value.provider_execution_eligible === false &&
    value.real_external_call_executed === false &&
    value.delivery_proven === false &&
    (!runtime || value.runtime_executed === false)
  );
}
function source(value: unknown): TouchPlanSource | undefined {
  const customer = exact(value, ["kind", "customer_selection"]);
  if (customer?.kind === "customer_selection") {
    const fact = exact(customer.customer_selection, [
      "id",
      "version",
      "digest",
    ]);
    return fact?.id === "local_selection" &&
      fact.version === "v1" &&
      digest(fact.digest)
      ? { kind: "customer_selection", digest: fact.digest }
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
      utcMicroseconds(fact.member_snapshot_watermark) &&
      digest(fact.digest)
      ? {
          kind: "segment_members",
          id: fact.segment_id,
          watermark: fact.member_snapshot_watermark,
          digest: fact.digest,
        }
      : undefined;
  }
  const audience = exact(value, ["kind", "audience_package"]);
  const fact =
    audience?.kind === "ai_audience_package_members"
      ? exact(audience.audience_package, [
          "package_id",
          "package_version",
          "member_snapshot_watermark",
          "digest",
        ])
      : undefined;
  return fact &&
    positive(fact.package_id) &&
    positive(fact.package_version) &&
    utcMicroseconds(fact.member_snapshot_watermark) &&
    digest(fact.digest)
    ? {
        kind: "ai_audience_package_members",
        id: fact.package_id,
        version: fact.package_version,
        watermark: fact.member_snapshot_watermark,
        digest: fact.digest,
      }
    : undefined;
}
function summary(value: unknown): TouchPlanSummary | undefined {
  const item = exact(value, [
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
  if (
    !item ||
    !planID(item.id) ||
    !campaignCode(item.campaign_code) ||
    !positive(item.campaign_version) ||
    !positive(item.target_count, 1000) ||
    !digest(item.target_digest) ||
    !positive(item.content_step_count, 100) ||
    !digest(item.content_digest) ||
    !positive(item.owner_actor_id) ||
    !utcMicroseconds(item.created_at) ||
    !safety(item)
  )
    return undefined;
  const itemSource = source(item.source);
  const excluded = exact(item.preview_exclusion_summary, [
    "candidate_count",
    "active_customer_count",
    "inactive_excluded_count",
    "policy_excluded_count",
  ]);
  if (
    !itemSource ||
    !excluded ||
    !positive(excluded.candidate_count, 1000) ||
    !positive(excluded.active_customer_count, 1000) ||
    !nonnegative(excluded.inactive_excluded_count, 1000) ||
    !nonnegative(excluded.policy_excluded_count, 1000) ||
    excluded.candidate_count !==
      excluded.active_customer_count + excluded.inactive_excluded_count ||
    item.target_count + excluded.policy_excluded_count !==
      excluded.active_customer_count
  )
    return undefined;
  const immutable = `${item.owner_actor_id}\u0000${item.created_at}\u0000${excluded.candidate_count}\u0000${excluded.active_customer_count}\u0000${excluded.inactive_excluded_count}\u0000${excluded.policy_excluded_count}`;
  return {
    id: item.id,
    campaignCode: item.campaign_code,
    campaignVersion: item.campaign_version,
    source: itemSource,
    targetCount: item.target_count,
    targetDigest: item.target_digest,
    stepCount: item.content_step_count,
    contentDigest: item.content_digest,
    immutable,
  };
}
function decimalCursor(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^[1-9][0-9]{0,19}$/u.test(value) &&
    Number.isSafeInteger(Number(value))
  );
}

export async function loadTouchPlans(
  transport: CampaignTouchPlanReadTransport,
  code: string,
  signal?: AbortSignal,
): Promise<PlansResult> {
  if (!campaignCode(code)) return { status: "unavailable" };
  try {
    const result = await transport.listPlans(
      code,
      { limit: 100 },
      { credentials: "same-origin", signal },
    );
    if (result.status !== 200) return { status: failure(result.status) };
    const body = exact(result.data, [
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
      !safety(body) ||
      !Array.isArray(body.items) ||
      body.items.length > 100 ||
      !(body.next_cursor === null || body.next_cursor === undefined)
    )
      return { status: "unavailable" };
    const plans = body.items.map(summary);
    return plans.every(Boolean) &&
      new Set(plans.map((item) => item?.id)).size === plans.length &&
      plans.every((item) => item?.campaignCode === code)
      ? { status: "loaded", plans: plans as TouchPlanSummary[] }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
export async function loadTouchPlanDetail(
  transport: CampaignTouchPlanReadTransport,
  code: string,
  selected: TouchPlanSummary,
  signal?: AbortSignal,
): Promise<DetailResult> {
  if (
    !campaignCode(code) ||
    selected.campaignCode !== code ||
    !planID(selected.id)
  )
    return { status: "unavailable" };
  try {
    const result = await transport.getPlan(code, selected.id, {
      credentials: "same-origin",
      signal,
    });
    if (result.status !== 200) return { status: failure(result.status) };
    const item = exact(result.data, [
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
    if (!item || !safety(item)) return { status: "unavailable" };
    const content = exact(item.content, ["steps", "content_digest"]);
    if (
      !content ||
      !Array.isArray(content.steps) ||
      content.steps.length !== selected.stepCount ||
      !digest(content.content_digest)
    )
      return { status: "unavailable" };
    const reformed = {
      ...item,
      content_step_count: content.steps.length,
      content_digest: content.content_digest,
    };
    delete (reformed as Record<string, unknown>).content;
    const parsed = summary(reformed);
    const steps = content.steps.map((step, index) => {
      const candidate = exact(step, ["step_index", "delay_minutes", "content"]);
      return candidate &&
        candidate.step_index === index + 1 &&
        nonnegative(candidate.delay_minutes, 2_147_483_647) &&
        typeof candidate.content === "string" &&
        candidate.content.length >= 1 &&
        candidate.content.length <= 4000
        ? index + 1
        : undefined;
    });
    return parsed &&
      JSON.stringify(parsed) === JSON.stringify(selected) &&
      steps.every(Boolean)
      ? { status: "loaded", plan: parsed }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
export async function loadTouchPlanRecipients(
  transport: CampaignTouchPlanReadTransport,
  code: string,
  id: string,
  targetCount: number,
  cursor?: string,
  prior: readonly number[] = [],
  signal?: AbortSignal,
): Promise<RecipientsResult> {
  if (
    !campaignCode(code) ||
    !planID(id) ||
    !positive(targetCount, 1000) ||
    (cursor !== undefined && !decimalCursor(cursor)) ||
    (cursor === undefined) !== (prior.length === 0) ||
    (cursor !== undefined && cursor !== String(prior.at(-1))) ||
    prior.some(
      (item, index) =>
        !positive(item) || (index > 0 && prior[index - 1] >= item),
    )
  )
    return { status: "unavailable" };
  try {
    const result = await transport.listRecipients(
      code,
      id,
      { cursor, limit: 100 },
      { credentials: "same-origin", signal },
    );
    if (result.status !== 200) return { status: failure(result.status) };
    const body = exact(result.data, [
      "items",
      "next_cursor",
      "local_only",
      "provider_execution_eligible",
      "real_external_call_executed",
      "delivery_proven",
    ]);
    if (
      !body ||
      !safety(body, false) ||
      !Array.isArray(body.items) ||
      body.items.length > 100 ||
      !(
        body.next_cursor === null ||
        body.next_cursor === undefined ||
        decimalCursor(body.next_cursor)
      )
    )
      return { status: "unavailable" };
    const recipients = body.items
      .map(
        (item) => exact(item, ["canonical_customer_id"])?.canonical_customer_id,
      )
      .filter((item): item is number => positive(item));
    const lower = cursor === undefined ? (prior.at(-1) ?? 0) : Number(cursor);
    const terminal = body.next_cursor === null || body.next_cursor === undefined;
    return recipients.length === body.items.length &&
      recipients.every(
        (item, index) =>
          item > lower && (index === 0 || recipients[index - 1] < item),
      ) &&
      (body.next_cursor === undefined ||
        body.next_cursor === null ||
        body.next_cursor === String(recipients.at(-1))) &&
      (terminal
        ? prior.length + recipients.length === targetCount
        : prior.length + recipients.length < targetCount)
      ? {
          status: "loaded",
          recipients,
          nextCursor: body.next_cursor ?? undefined,
        }
      : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
