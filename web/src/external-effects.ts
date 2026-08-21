export const EXTERNAL_EFFECTS_PAGE_SIZE = 50;
export const EXTERNAL_EFFECTS_DELIVERY_SEMANTICS =
  "local_state_not_delivery_proof" as const;

export const EXTERNAL_EFFECT_STATUSES = [
  "pending",
  "sending",
  "sent",
  "retryable_failed",
  "final_failed",
  "outcome_unknown",
  "cancelled",
] as const;
export type ExternalEffectStatus = (typeof EXTERNAL_EFFECT_STATUSES)[number];

export const EXTERNAL_EFFECT_CLASSIFICATIONS = [
  "safe_local_handling",
  "frozen",
  "manual_review",
] as const;
export type ExternalEffectClassification =
  (typeof EXTERNAL_EFFECT_CLASSIFICATIONS)[number];

export type ExternalEffectsRole = "admin" | "ops" | "sales";

export interface ExternalEffectsFilters {
  readonly status?: ExternalEffectStatus;
  readonly classification?: ExternalEffectClassification;
}

export interface ExternalEffectsFilterDraft {
  readonly status: string;
  readonly classification: string;
}

export const EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT: ExternalEffectsFilterDraft = {
  status: "",
  classification: "",
};

export interface ExternalEffectJob {
  readonly id: string;
  readonly status: ExternalEffectStatus;
  readonly classification: ExternalEffectClassification;
  readonly attemptCount: number;
  readonly createdAt: string;
  readonly statusUpdatedAt: string;
}

export interface ExternalEffectJobPage {
  readonly items: readonly ExternalEffectJob[];
  readonly nextCursor?: string;
  readonly pageSize: number;
  readonly appliedFilters: ExternalEffectsFilters;
  readonly providerExecutionEligible: false;
  readonly realExternalCallExecuted: false;
  readonly deliveryProven: false;
  readonly localFactOnly: true;
  readonly deliverySemantics: typeof EXTERNAL_EFFECTS_DELIVERY_SEMANTICS;
}

export interface ExternalEffectStatusCounts {
  readonly pending: number;
  readonly sending: number;
  readonly sent: number;
  readonly retryableFailed: number;
  readonly finalFailed: number;
  readonly outcomeUnknown: number;
  readonly cancelled: number;
}

export interface ExternalEffectClassificationCounts {
  readonly safeLocalHandling: number;
  readonly frozen: number;
  readonly manualReview: number;
}

export type ExternalEffectRiskLevel =
  | "none"
  | "manual_review_required"
  | "outcome_unknown_present";

export interface ExternalEffectRiskSummary {
  readonly level: ExternalEffectRiskLevel;
  readonly outcomeUnknownCount: number;
  readonly manualReviewCount: number;
  readonly manualReviewRequired: boolean;
}

export interface ExternalEffectsDiagnostics {
  readonly total: number;
  readonly byStatus: ExternalEffectStatusCounts;
  readonly byClassification: ExternalEffectClassificationCounts;
  readonly risk: ExternalEffectRiskSummary;
  readonly generatedAt: string;
  readonly providerExecutionEligible: false;
  readonly realExternalCallExecuted: false;
  readonly deliveryProven: false;
  readonly localFactOnly: true;
  readonly deliverySemantics: typeof EXTERNAL_EFFECTS_DELIVERY_SEMANTICS;
}

export interface ExternalEffectsSnapshot {
  readonly page: ExternalEffectJobPage;
  readonly diagnostics: ExternalEffectsDiagnostics;
  readonly filters: ExternalEffectsFilters;
  readonly cursor?: string;
}

export interface ExternalEffectsTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

export interface ExternalEffectsJobsTransportParams {
  readonly status?: ExternalEffectStatus;
  readonly classification?: ExternalEffectClassification;
  readonly cursor?: string;
  readonly limit: typeof EXTERNAL_EFFECTS_PAGE_SIZE;
}

export interface ExternalEffectsTransport {
  readonly readJobs: (
    ...[]: [ExternalEffectsJobsTransportParams, RequestInit]
  ) => Promise<ExternalEffectsTransportResponse>;
  readonly readDiagnostics: (
    ...[]: [RequestInit]
  ) => Promise<ExternalEffectsTransportResponse>;
}

export const generatedExternalEffectsTransport: ExternalEffectsTransport = {
  readJobs: (params, options) => listExternalEffectJobs(params, options),
  readDiagnostics: (options) => getExternalEffectsDiagnostics(options),
};

export type ExternalEffectsFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type ExternalEffectsLoadResult =
  | { readonly status: "loaded"; readonly snapshot: ExternalEffectsSnapshot; }
  | { readonly status: ExternalEffectsFailure; };

const statusSet = new Set<string>(EXTERNAL_EFFECT_STATUSES);
const classificationSet = new Set<string>(EXTERNAL_EFFECT_CLASSIFICATIONS);
const riskSet = new Set<string>([
  "none",
  "manual_review_required",
  "outcome_unknown_present",
]);
const jobIDPattern = /^eej_v1_[A-Za-z0-9_-]{22}$/u;
const cursorPattern = /^eec_v1_[A-Za-z0-9_-]{24,1000}$/u;
const timePattern =
  /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/u;

function plain(value: unknown): value is Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
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

function safeCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function validTime(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !timePattern.test(value) ||
    !Number.isFinite(Date.parse(value))
  ) {
    return false;
  }
  const match = value.match(
    /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)/u,
  );
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match
    .slice(1)
    .map(Number);
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, 0);
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day &&
    date.getUTCHours() === hour &&
    date.getUTCMinutes() === minute &&
    date.getUTCSeconds() === second
  );
}

function status(value: unknown): value is ExternalEffectStatus {
  return typeof value === "string" && statusSet.has(value);
}

function classification(
  value: unknown,
): value is ExternalEffectClassification {
  return typeof value === "string" && classificationSet.has(value);
}

export function externalEffectClassificationForStatus(
  value: ExternalEffectStatus,
): ExternalEffectClassification {
  switch (value) {
    case "pending":
    case "retryable_failed":
      return "safe_local_handling";
    case "sending":
    case "sent":
    case "cancelled":
      return "frozen";
    case "final_failed":
    case "outcome_unknown":
      return "manual_review";
  }
}

function validAttemptCount(
  value: number,
  taskStatus: ExternalEffectStatus,
): boolean {
  return taskStatus === "pending" || taskStatus === "cancelled"
    ? value === 0
    : value > 0;
}

function safety(value: Record<string, unknown>): boolean {
  return (
    value.provider_execution_eligible === false &&
    value.real_external_call_executed === false &&
    value.delivery_proven === false &&
    value.local_fact_only === true &&
    value.delivery_semantics === EXTERNAL_EFFECTS_DELIVERY_SEMANTICS
  );
}

function parseAppliedFilters(
  value: unknown,
): ExternalEffectsFilters | undefined {
  if (
    !plain(value) ||
    !exact(value, ["status", "classification"]) ||
    (value.status !== null && !status(value.status)) ||
    (value.classification !== null && !classification(value.classification))
  ) {
    return undefined;
  }
  const filters: {
    status?: ExternalEffectStatus;
    classification?: ExternalEffectClassification;
  } = {};
  if (value.status !== null) filters.status = value.status;
  if (value.classification !== null) {
    filters.classification = value.classification;
  }
  if (
    filters.status !== undefined &&
    filters.classification !== undefined &&
    externalEffectClassificationForStatus(filters.status) !==
    filters.classification
  ) {
    return undefined;
  }
  return filters;
}

function sameFilters(
  left: ExternalEffectsFilters,
  right: ExternalEffectsFilters,
): boolean {
  return (
    left.status === right.status && left.classification === right.classification
  );
}

export function normalizeExternalEffectsFilters(
  input: ExternalEffectsFilters | ExternalEffectsFilterDraft,
):
  | {
    readonly status: "valid";
    readonly filters: ExternalEffectsFilters;
    readonly key: string;
  }
  | { readonly status: "invalid"; readonly message: string; } {
  const rawStatus = input.status ?? "";
  const rawClassification = input.classification ?? "";
  if (
    typeof rawStatus !== "string" ||
    typeof rawClassification !== "string" ||
    rawStatus.trim() !== rawStatus ||
    rawClassification.trim() !== rawClassification ||
    (rawStatus !== "" && !statusSet.has(rawStatus)) ||
    (rawClassification !== "" && !classificationSet.has(rawClassification))
  ) {
    return { status: "invalid", message: "筛选值不在批准枚举内。" };
  }
  const filters: {
    status?: ExternalEffectStatus;
    classification?: ExternalEffectClassification;
  } = {};
  if (rawStatus !== "") filters.status = rawStatus as ExternalEffectStatus;
  if (rawClassification !== "") {
    filters.classification = rawClassification as ExternalEffectClassification;
  }
  if (
    filters.status !== undefined &&
    filters.classification !== undefined &&
    externalEffectClassificationForStatus(filters.status) !==
    filters.classification
  ) {
    return {
      status: "invalid",
      message: "状态与本地安全分类不一致。",
    };
  }
  return {
    status: "valid",
    filters,
    key: `${filters.status ?? ""}\u0000${filters.classification ?? ""}`,
  };
}

export function externalEffectsReadKey(
  filters: ExternalEffectsFilters,
  cursor?: string,
): string {
  const normalized = normalizeExternalEffectsFilters(filters);
  if (normalized.status !== "valid") return "invalid";
  return `${normalized.key}\u0000${cursor ?? ""}`;
}

function parseJob(value: unknown): ExternalEffectJob | undefined {
  if (
    !plain(value) ||
    !exact(value, [
      "id",
      "status",
      "classification",
      "attempt_count",
      "created_at",
      "status_updated_at",
    ]) ||
    typeof value.id !== "string" ||
    !jobIDPattern.test(value.id) ||
    !status(value.status) ||
    !classification(value.classification) ||
    value.classification !==
    externalEffectClassificationForStatus(value.status) ||
    !safeCount(value.attempt_count) ||
    !validAttemptCount(value.attempt_count, value.status) ||
    !validTime(value.created_at) ||
    !validTime(value.status_updated_at) ||
    Date.parse(value.status_updated_at) < Date.parse(value.created_at)
  ) {
    return undefined;
  }
  return {
    id: value.id,
    status: value.status,
    classification: value.classification,
    attemptCount: value.attempt_count,
    createdAt: value.created_at,
    statusUpdatedAt: value.status_updated_at,
  };
}

export function parseExternalEffectJobPage(
  value: unknown,
  expectedFilters: ExternalEffectsFilters,
): ExternalEffectJobPage | undefined {
  if (
    !plain(value) ||
    !exact(value, [
      "ok",
      "items",
      "next_cursor",
      "page_size",
      "applied_filters",
      "provider_execution_eligible",
      "real_external_call_executed",
      "delivery_proven",
      "local_fact_only",
      "delivery_semantics",
    ]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    value.items.length > EXTERNAL_EFFECTS_PAGE_SIZE ||
    value.page_size !== EXTERNAL_EFFECTS_PAGE_SIZE ||
    (value.next_cursor !== null &&
      (typeof value.next_cursor !== "string" ||
        !cursorPattern.test(value.next_cursor) ||
        value.items.length === 0)) ||
    !safety(value)
  ) {
    return undefined;
  }
  const appliedFilters = parseAppliedFilters(value.applied_filters);
  if (!appliedFilters || !sameFilters(appliedFilters, expectedFilters)) {
    return undefined;
  }
  const items = value.items.map(parseJob);
  if (
    items.includes(undefined) ||
    new Set(
      (items as readonly ExternalEffectJob[]).map((item) => item.id),
    ).size !== items.length ||
    (items as readonly ExternalEffectJob[]).some(
      (item) =>
        (expectedFilters.status !== undefined &&
          item.status !== expectedFilters.status) ||
        (expectedFilters.classification !== undefined &&
          item.classification !== expectedFilters.classification),
    )
  ) {
    return undefined;
  }
  return {
    items: items as readonly ExternalEffectJob[],
    nextCursor:
      value.next_cursor === null ? undefined : value.next_cursor,
    pageSize: EXTERNAL_EFFECTS_PAGE_SIZE,
    appliedFilters,
    providerExecutionEligible: false,
    realExternalCallExecuted: false,
    deliveryProven: false,
    localFactOnly: true,
    deliverySemantics: EXTERNAL_EFFECTS_DELIVERY_SEMANTICS,
  };
}

function parseStatusCounts(
  value: unknown,
): ExternalEffectStatusCounts | undefined {
  if (
    !plain(value) ||
    !exact(value, [
      "pending",
      "sending",
      "sent",
      "retryable_failed",
      "final_failed",
      "outcome_unknown",
      "cancelled",
    ]) ||
    !safeCount(value.pending) ||
    !safeCount(value.sending) ||
    !safeCount(value.sent) ||
    !safeCount(value.retryable_failed) ||
    !safeCount(value.final_failed) ||
    !safeCount(value.outcome_unknown) ||
    !safeCount(value.cancelled)
  ) {
    return undefined;
  }
  return {
    pending: value.pending,
    sending: value.sending,
    sent: value.sent,
    retryableFailed: value.retryable_failed,
    finalFailed: value.final_failed,
    outcomeUnknown: value.outcome_unknown,
    cancelled: value.cancelled,
  };
}

function parseClassificationCounts(
  value: unknown,
): ExternalEffectClassificationCounts | undefined {
  if (
    !plain(value) ||
    !exact(value, ["safe_local_handling", "frozen", "manual_review"]) ||
    !safeCount(value.safe_local_handling) ||
    !safeCount(value.frozen) ||
    !safeCount(value.manual_review)
  ) {
    return undefined;
  }
  return {
    safeLocalHandling: value.safe_local_handling,
    frozen: value.frozen,
    manualReview: value.manual_review,
  };
}

function checkedSum(values: readonly number[]): number | undefined {
  let result = 0;
  for (const value of values) {
    result += value;
    if (!Number.isSafeInteger(result)) return undefined;
  }
  return result;
}

function parseRisk(
  value: unknown,
  statusCounts: ExternalEffectStatusCounts,
  classificationCounts: ExternalEffectClassificationCounts,
): ExternalEffectRiskSummary | undefined {
  if (
    !plain(value) ||
    !exact(value, [
      "level",
      "outcome_unknown_count",
      "manual_review_count",
      "manual_review_required",
    ]) ||
    typeof value.level !== "string" ||
    !riskSet.has(value.level) ||
    !safeCount(value.outcome_unknown_count) ||
    !safeCount(value.manual_review_count) ||
    typeof value.manual_review_required !== "boolean" ||
    value.outcome_unknown_count !== statusCounts.outcomeUnknown ||
    value.manual_review_count !== classificationCounts.manualReview ||
    value.manual_review_required !== (value.manual_review_count > 0)
  ) {
    return undefined;
  }
  const expectedLevel: ExternalEffectRiskLevel =
    value.outcome_unknown_count > 0
      ? "outcome_unknown_present"
      : value.manual_review_count > 0
        ? "manual_review_required"
        : "none";
  if (value.level !== expectedLevel) return undefined;
  return {
    level: value.level as ExternalEffectRiskLevel,
    outcomeUnknownCount: value.outcome_unknown_count,
    manualReviewCount: value.manual_review_count,
    manualReviewRequired: value.manual_review_required,
  };
}

export function parseExternalEffectsDiagnostics(
  value: unknown,
): ExternalEffectsDiagnostics | undefined {
  if (
    !plain(value) ||
    !exact(value, [
      "ok",
      "counts",
      "risk_summary",
      "generated_at",
      "provider_execution_eligible",
      "real_external_call_executed",
      "delivery_proven",
      "local_fact_only",
      "delivery_semantics",
    ]) ||
    value.ok !== true ||
    !plain(value.counts) ||
    !exact(value.counts, ["total", "by_status", "by_classification"]) ||
    !safeCount(value.counts.total) ||
    !validTime(value.generated_at) ||
    !safety(value)
  ) {
    return undefined;
  }
  const byStatus = parseStatusCounts(value.counts.by_status);
  const byClassification = parseClassificationCounts(
    value.counts.by_classification,
  );
  if (!byStatus || !byClassification) return undefined;
  const statusTotal = checkedSum([
    byStatus.pending,
    byStatus.sending,
    byStatus.sent,
    byStatus.retryableFailed,
    byStatus.finalFailed,
    byStatus.outcomeUnknown,
    byStatus.cancelled,
  ]);
  const classificationTotal = checkedSum([
    byClassification.safeLocalHandling,
    byClassification.frozen,
    byClassification.manualReview,
  ]);
  const expectedSafeLocalHandling = checkedSum([
    byStatus.pending,
    byStatus.retryableFailed,
  ]);
  const expectedFrozen = checkedSum([
    byStatus.sending,
    byStatus.sent,
    byStatus.cancelled,
  ]);
  const expectedManualReview = checkedSum([
    byStatus.finalFailed,
    byStatus.outcomeUnknown,
  ]);
  if (
    statusTotal === undefined ||
    classificationTotal === undefined ||
    expectedSafeLocalHandling === undefined ||
    expectedFrozen === undefined ||
    expectedManualReview === undefined ||
    statusTotal !== value.counts.total ||
    classificationTotal !== value.counts.total ||
    byClassification.safeLocalHandling !== expectedSafeLocalHandling ||
    byClassification.frozen !== expectedFrozen ||
    byClassification.manualReview !== expectedManualReview
  ) {
    return undefined;
  }
  const risk = parseRisk(value.risk_summary, byStatus, byClassification);
  if (!risk) return undefined;
  return {
    total: value.counts.total,
    byStatus,
    byClassification,
    risk,
    generatedAt: value.generated_at,
    providerExecutionEligible: false,
    realExternalCallExecuted: false,
    deliveryProven: false,
    localFactOnly: true,
    deliverySemantics: EXTERNAL_EFFECTS_DELIVERY_SEMANTICS,
  };
}

function failure(statuses: readonly number[]): ExternalEffectsFailure {
  if (statuses.includes(401)) return "unauthenticated";
  if (statuses.includes(403)) return "forbidden";
  if (statuses.some((value) => value === 400 || value === 422)) {
    return "invalid";
  }
  return "unavailable";
}

export async function loadExternalEffectsSnapshot(
  transport: ExternalEffectsTransport,
  filters: ExternalEffectsFilters,
  cursor?: string,
  signal?: AbortSignal,
): Promise<ExternalEffectsLoadResult> {
  const normalized = normalizeExternalEffectsFilters(filters);
  if (
    normalized.status !== "valid" ||
    (cursor !== undefined && !cursorPattern.test(cursor))
  ) {
    return { status: "invalid" };
  }
  const options: RequestInit = { credentials: "same-origin", signal };
  try {
    const [jobsResponse, diagnosticsResponse] = await Promise.all([
      transport.readJobs(
        {
          ...normalized.filters,
          cursor,
          limit: EXTERNAL_EFFECTS_PAGE_SIZE,
        },
        options,
      ),
      transport.readDiagnostics(options),
    ]);
    if (jobsResponse.status !== 200 || diagnosticsResponse.status !== 200) {
      return {
        status: failure([jobsResponse.status, diagnosticsResponse.status]),
      };
    }
    const page = parseExternalEffectJobPage(
      jobsResponse.data,
      normalized.filters,
    );
    const diagnostics = parseExternalEffectsDiagnostics(
      diagnosticsResponse.data,
    );
    if (!page || !diagnostics) return { status: "invalid" };
    return {
      status: "loaded",
      snapshot: {
        page,
        diagnostics,
        filters: normalized.filters,
        cursor,
      },
    };
  } catch {
    return { status: "unavailable" };
  }
}
import {
  getExternalEffectsDiagnostics,
  listExternalEffectJobs,
} from "./api/generated/health";
