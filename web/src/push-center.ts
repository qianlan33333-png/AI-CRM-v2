import {
  getLegacyPushCenterSections,
  getLegacyPushCenterStats,
} from "./api/generated/health";

export type PushCenterRole = "admin" | "ops" | "sales";

export const PUSH_CENTER_SECTION_KEYS = [
  "questionnaire",
  "order",
  "ai_assist",
  "private_broadcast",
  "group_ops",
  "group_broadcast",
  "customer_webhook",
  "tags",
  "welcome",
  "payment",
  "integrations",
  "test_receiver",
  "other",
] as const;

export const PUSH_CENTER_STATUS_KEYS = [
  "pending",
  "running",
  "succeeded",
  "sent",
  "simulated",
  "unknown_after_dispatch",
  "failed",
  "sent_with_shadow_warning",
  "shadow_failed_not_business_failed",
] as const;

export const PUSH_CENTER_LANE_KEYS = [
  "ai_generation",
  "outbound_webhook",
  "wecom_ai_assistant_bulk",
  "wecom_bulk",
  "wecom_interactive",
  "wecom_media",
] as const;

export const PUSH_CENTER_FILTER_KEYS = [
  "section",
  "effect_type",
  "status",
  "business_type",
  "business_id",
  "source_module",
  "created_from",
  "created_to",
] as const;

export type PushCenterSectionKey = (typeof PUSH_CENTER_SECTION_KEYS)[number];
export type PushCenterStatusKey = (typeof PUSH_CENTER_STATUS_KEYS)[number];
export type PushCenterLaneKey = (typeof PUSH_CENTER_LANE_KEYS)[number];
export type PushCenterFilterKey = (typeof PUSH_CENTER_FILTER_KEYS)[number];

export type PushCenterFilters = Readonly<
  Partial<Record<PushCenterFilterKey, string>>
>;

export type PushCenterFilterDraft = Readonly<Record<PushCenterFilterKey, string>>;

export const EMPTY_PUSH_CENTER_FILTER_DRAFT: PushCenterFilterDraft = {
  section: "",
  effect_type: "",
  status: "",
  business_type: "",
  business_id: "",
  source_module: "",
  created_from: "",
  created_to: "",
};

export type PushCenterFilterResult =
  | {
      readonly status: "valid";
      readonly filters: PushCenterFilters;
      readonly key: string;
    }
  | { readonly status: "invalid"; readonly message: string };

export interface PushCenterSection {
  readonly key: PushCenterSectionKey;
  readonly label: string;
  readonly effectTypes: readonly string[];
  readonly capabilityKey: string;
  readonly count: number;
}

export interface PushCenterStatusDefinition {
  readonly key: PushCenterStatusKey;
  readonly label: string;
  readonly definition: string;
}

export interface PushCenterInternalEventSummary {
  readonly rawOpen: number;
  readonly rawDue: number;
  readonly eligible: number;
  readonly failedRetryable: number;
  readonly failedTerminal: number;
  readonly blocked: number;
}

export interface PushCenterLaneSummary {
  readonly lane: PushCenterLaneKey;
  readonly maxInFlight: number;
  readonly enabled: boolean;
  readonly rolloutMode: string;
  readonly blockedUntil: string | null;
  readonly policyVersion: string;
  readonly rawOpen: number;
  readonly held: number;
  readonly eligible: number;
  readonly policyGated: number;
  readonly scheduled: number;
  readonly retryWait: number;
  readonly rateLimited: number;
  readonly inFlight: number;
  readonly unknown: number;
  readonly dlq: number;
  readonly oldestEligibleAgeSeconds: number;
  readonly internalEvent: PushCenterInternalEventSummary;
  readonly throughputLastMinute: number;
  readonly acceptedLastMinute: number;
  readonly p95QueueWaitMs: number;
  readonly p95ProviderCallMs: number;
  readonly estimatedDrainSeconds: number;
  readonly rateLimitCountLastHour: number;
  readonly taskAcceptanceRate1m: number;
}

export type PushCenterRuntimeQueue =
  | { readonly status: "not_reported" }
  | {
      readonly status: "reported";
      readonly policyVersion: string;
      readonly activeGeneration: number;
      readonly claimEnabled: boolean;
      readonly rolloutMode: string;
      readonly lanes: readonly PushCenterLaneSummary[];
      readonly rawOpen: number;
      readonly held: number;
      readonly eligible: number;
      readonly policyGated: number;
      readonly scheduled: number;
      readonly retryWait: number;
      readonly rateLimited: number;
      readonly inFlight: number;
      readonly unknown: number;
      readonly dlq: number;
      readonly internalEvent: PushCenterInternalEventSummary;
    };

export interface PushCenterCounts {
  readonly total: number;
  readonly byEffectiveStatus: Readonly<Record<string, number>>;
  readonly byStatus: Readonly<Partial<Record<PushCenterStatusKey, number>>>;
  readonly bySection: Readonly<Partial<Record<PushCenterSectionKey, number>>>;
  readonly pending: number;
  readonly running: number;
  readonly succeeded: number;
  readonly sent: number;
  readonly failed: number;
  readonly shadowWarning: number;
}

export interface PushCenterDegradedBoundary {
  readonly sourceStatus: "production_unavailable";
  readonly readModelStatus: "unavailable";
  readonly pageError: string;
  readonly diagnostics: {
    readonly productionDataReady: false;
    readonly fixtureMode: false;
    readonly allowFixtureRepoInProd: false;
    readonly errorClass: "ReadModelUnavailableError";
  };
}

interface PushCenterSnapshotBase {
  readonly filters: PushCenterFilters;
  readonly statusDefinitions: readonly PushCenterStatusDefinition[];
  readonly runtimeQueue: PushCenterRuntimeQueue;
  readonly realExternalCallExecuted: false;
}

export type PushCenterSnapshot =
  | (PushCenterSnapshotBase & {
      readonly boundary: "normal";
      readonly sections: readonly PushCenterSection[];
      readonly counts: PushCenterCounts;
    })
  | (PushCenterSnapshotBase & {
      readonly boundary: "degraded";
      readonly degraded: PushCenterDegradedBoundary;
    });

export interface PushCenterTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

export interface PushCenterTransport {
  readonly readSections: (
    ...[]: [PushCenterFilters, RequestInit]
  ) => Promise<PushCenterTransportResponse>;
  readonly readStats: (
    ...[]: [PushCenterFilters, RequestInit]
  ) => Promise<PushCenterTransportResponse>;
}

export const generatedPushCenterTransport: PushCenterTransport = {
  readSections: (filters, options) =>
    getLegacyPushCenterSections(filters, options),
  readStats: (filters, options) => getLegacyPushCenterStats(filters, options),
};

export type PushCenterFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type PushCenterResult =
  | { readonly status: "loaded"; readonly snapshot: PushCenterSnapshot }
  | { readonly status: PushCenterFailure };

const STATUS_SET = new Set<string>(PUSH_CENTER_STATUS_KEYS);
const SECTION_SET = new Set<string>(PUSH_CENTER_SECTION_KEYS);
const LANE_SET = new Set<string>(PUSH_CENTER_LANE_KEYS);
const FILTER_SET = new Set<string>(PUSH_CENTER_FILTER_KEYS);
const EFFECTIVE_STATUS_SET = new Set<string>([
  ...PUSH_CENTER_STATUS_KEYS,
  "reconciled",
]);

const NORMAL_SECTION_KEYS = [
  "ok",
  "sections",
  "status_definitions",
  "filters",
  "route_owner",
] as const;
const NORMAL_STATS_KEYS = [
  "ok",
  "counts",
  "sections",
  "status_definitions",
  "filters",
  "route_owner",
  "real_external_call_executed",
  "runtime_queue",
  "capability_owner",
] as const;
const DEGRADED_SECTION_KEYS = [
  "ok",
  "degraded",
  "error",
  "error_code",
  "source_status",
  "read_model_status",
  "capability_owner",
  "page_error",
  "diagnostics",
  "route_owner",
  "fallback_used",
  "real_external_call_executed",
  "status_code",
  "items",
  "total",
  "counts",
  "status_definitions",
  "filters",
  "limit",
  "offset",
  "sections",
] as const;
const DEGRADED_STATS_KEYS = [
  ...DEGRADED_SECTION_KEYS,
  "runtime_queue",
] as const;
const SECTION_KEYS = [
  "key",
  "label",
  "effect_types",
  "capability_key",
  "count",
] as const;
const STATUS_DEFINITION_KEYS = ["key", "label", "definition"] as const;
const COUNTS_KEYS = [
  "total",
  "by_effective_status",
  "by_status",
  "by_section",
  "pending",
  "running",
  "succeeded",
  "sent",
  "failed",
  "shadow_warning",
] as const;
const DEGRADED_COUNTS_KEYS = [
  "total",
  "by_effective_status",
  "by_status",
  "by_section",
  "pending",
  "running",
  "sent",
  "failed",
] as const;
const DIAGNOSTIC_KEYS = [
  "production_data_ready",
  "fixture_mode",
  "allow_fixture_repo_in_prod",
  "error_class",
] as const;
const INTERNAL_EVENT_KEYS = [
  "raw_open",
  "raw_due",
  "eligible",
  "failed_retryable",
  "failed_terminal",
  "blocked",
] as const;
const LANE_KEYS = [
  "lane",
  "max_in_flight",
  "enabled",
  "rollout_mode",
  "blocked_until",
  "policy_version",
  "raw_open",
  "held",
  "eligible",
  "policy_gated",
  "scheduled",
  "retry_wait",
  "rate_limited",
  "in_flight",
  "unknown",
  "dlq",
  "oldest_eligible_age_seconds",
  "internal_event",
  "throughput_last_minute",
  "accepted_last_minute",
  "p95_queue_wait_ms",
  "p95_provider_call_ms",
  "estimated_drain_seconds",
  "rate_limit_count_last_hour",
  "task_acceptance_rate_1m",
] as const;
const RUNTIME_QUEUE_KEYS = [
  "policy_version",
  "active_generation",
  "claim_enabled",
  "rollout_mode",
  "lanes",
  "raw_open",
  "held",
  "eligible",
  "policy_gated",
  "scheduled",
  "retry_wait",
  "rate_limited",
  "in_flight",
  "unknown",
  "dlq",
  "internal_event",
] as const;

function record(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null)
  );
}

function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function boundedText(
  value: unknown,
  maximum: number,
  allowEmpty = false,
): value is string {
  if (typeof value !== "string" || Array.from(value).length > maximum) return false;
  if (!allowEmpty && value.length === 0) return false;
  return value.trim() === value && !/[\u0000-\u001f\u007f]/u.test(value);
}

function nonnegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function nonnegativeNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function rfc3339(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2}(?:\.\d{1,9})?)?(?:Z|[+-]\d{2}:\d{2})$/u.test(
      value,
    ) &&
    Number.isFinite(Date.parse(value))
  );
}

function canonicalTimestamp(value: string): string | undefined {
  if (!rfc3339(value)) return undefined;
  return new Date(value).toISOString();
}

function filterMaximum(key: PushCenterFilterKey): number {
  return key === "created_from" || key === "created_to" ? 64 : 256;
}

export function pushCenterFilterKey(filters: PushCenterFilters): string {
  return JSON.stringify(
    PUSH_CENTER_FILTER_KEYS.map((key) => [key, filters[key] ?? ""]),
  );
}

export function normalizePushCenterFilters(value: unknown): PushCenterFilterResult {
  if (!record(value) || Object.keys(value).some((key) => !FILTER_SET.has(key))) {
    return { status: "invalid", message: "筛选条件包含未允许字段。" };
  }
  const filters: Partial<Record<PushCenterFilterKey, string>> = {};
  for (const key of PUSH_CENTER_FILTER_KEYS) {
    const raw = value[key];
    if (raw === undefined) continue;
    if (typeof raw !== "string") {
      return { status: "invalid", message: `${key} 必须是文本。` };
    }
    const normalized = raw.trim();
    if (normalized === "") continue;
    if (!boundedText(normalized, filterMaximum(key))) {
      return { status: "invalid", message: `${key} 含非法字符或长度超限。` };
    }
    if (key === "section" && !SECTION_SET.has(normalized)) {
      return { status: "invalid", message: "section 不是已知本地分区。" };
    }
    if (key === "status" && !STATUS_SET.has(normalized)) {
      return { status: "invalid", message: "status 不是已知内部状态。" };
    }
    if (key === "created_from" || key === "created_to") {
      const timestamp = canonicalTimestamp(normalized);
      if (timestamp === undefined) {
        return {
          status: "invalid",
          message: `${key} 必须是带时区的 RFC3339 时间。`,
        };
      }
      filters[key] = timestamp;
      continue;
    }
    filters[key] = normalized;
  }
  if (
    filters.created_from !== undefined &&
    filters.created_to !== undefined &&
    Date.parse(filters.created_from) > Date.parse(filters.created_to)
  ) {
    return {
      status: "invalid",
      message: "created_from 不能晚于 created_to。",
    };
  }
  const result = filters as PushCenterFilters;
  return { status: "valid", filters: result, key: pushCenterFilterKey(result) };
}

function parseResponseFilters(
  value: unknown,
  expected: PushCenterFilters,
): PushCenterFilters | undefined {
  const parsed = normalizePushCenterFilters(value);
  if (parsed.status !== "valid" || parsed.key !== pushCenterFilterKey(expected)) {
    return undefined;
  }
  if (!record(value)) return undefined;
  for (const [key, raw] of Object.entries(value)) {
    if (!FILTER_SET.has(key) || typeof raw !== "string" || raw.trim() !== raw || raw === "") {
      return undefined;
    }
  }
  return parsed.filters;
}

function parseSection(
  value: unknown,
  expectedKey: PushCenterSectionKey,
): PushCenterSection | undefined {
  if (
    !record(value) ||
    !exact(value, SECTION_KEYS) ||
    value.key !== expectedKey ||
    !boundedText(value.label, 128) ||
    !Array.isArray(value.effect_types) ||
    value.effect_types.length > 32 ||
    !value.effect_types.every((item) => boundedText(item, 128)) ||
    new Set(value.effect_types).size !== value.effect_types.length ||
    !boundedText(value.capability_key, 128, true) ||
    !nonnegativeInteger(value.count)
  ) {
    return undefined;
  }
  return {
    key: expectedKey,
    label: value.label,
    effectTypes: [...value.effect_types] as string[],
    capabilityKey: value.capability_key,
    count: value.count,
  };
}

function parseSections(value: unknown): readonly PushCenterSection[] | undefined {
  if (!Array.isArray(value) || value.length !== PUSH_CENTER_SECTION_KEYS.length) {
    return undefined;
  }
  const sections = value.map((item, index) =>
    parseSection(item, PUSH_CENTER_SECTION_KEYS[index]),
  );
  if (sections.some((item) => item === undefined)) return undefined;
  return sections as PushCenterSection[];
}

function parseStatusDefinitions(
  value: unknown,
): readonly PushCenterStatusDefinition[] | undefined {
  if (!Array.isArray(value) || value.length !== PUSH_CENTER_STATUS_KEYS.length) {
    return undefined;
  }
  const definitions: PushCenterStatusDefinition[] = [];
  for (let index = 0; index < value.length; index += 1) {
    const item = value[index];
    const expectedKey = PUSH_CENTER_STATUS_KEYS[index];
    if (
      !record(item) ||
      !exact(item, STATUS_DEFINITION_KEYS) ||
      item.key !== expectedKey ||
      !boundedText(item.label, 128) ||
      !boundedText(item.definition, 2048)
    ) {
      return undefined;
    }
    definitions.push({
      key: expectedKey,
      label: item.label,
      definition: item.definition,
    });
  }
  return definitions;
}

function parseCountMap<K extends string>(
  value: unknown,
  allowed: ReadonlySet<string>,
): Readonly<Partial<Record<K, number>>> | undefined {
  if (!record(value)) return undefined;
  const result: Partial<Record<K, number>> = {};
  for (const [key, count] of Object.entries(value)) {
    if (!allowed.has(key) || !nonnegativeInteger(count)) return undefined;
    result[key as K] = count;
  }
  return result;
}

function sumCounts(value: Readonly<Record<string, number>>): number {
  return Object.values(value).reduce((sum, count) => sum + count, 0);
}

function parseCounts(
  value: unknown,
  sections: readonly PushCenterSection[],
): PushCenterCounts | undefined {
  if (!record(value) || !exact(value, COUNTS_KEYS)) return undefined;
  const byStatus = parseCountMap<PushCenterStatusKey>(value.by_status, STATUS_SET);
  const byEffectiveStatus = parseCountMap<string>(
    value.by_effective_status,
    EFFECTIVE_STATUS_SET,
  );
  const bySection = parseCountMap<PushCenterSectionKey>(
    value.by_section,
    SECTION_SET,
  );
  if (
    byStatus === undefined ||
    byEffectiveStatus === undefined ||
    bySection === undefined ||
    !nonnegativeInteger(value.total) ||
    !nonnegativeInteger(value.pending) ||
    !nonnegativeInteger(value.running) ||
    !nonnegativeInteger(value.succeeded) ||
    !nonnegativeInteger(value.sent) ||
    !nonnegativeInteger(value.failed) ||
    !nonnegativeInteger(value.shadow_warning)
  ) {
    return undefined;
  }
  const statusMap = byStatus as Readonly<Record<string, number>>;
  const effectiveMap = byEffectiveStatus as Readonly<Record<string, number>>;
  const sectionMap = bySection as Readonly<Record<string, number>>;
  if (
    sumCounts(statusMap) !== value.total ||
    sumCounts(effectiveMap) !== value.total ||
    sumCounts(sectionMap) !== value.total ||
    value.pending !== (byStatus.pending ?? 0) ||
    value.running !== (byStatus.running ?? 0) ||
    value.succeeded !== (byStatus.succeeded ?? 0) ||
    value.sent !==
      (byStatus.sent ?? 0) + (byStatus.sent_with_shadow_warning ?? 0) ||
    value.failed !== (byStatus.failed ?? 0) ||
    value.shadow_warning !==
      (byStatus.sent_with_shadow_warning ?? 0) +
        (byStatus.shadow_failed_not_business_failed ?? 0) ||
    sections.some(
      (section) => section.count !== (bySection[section.key] ?? 0),
    )
  ) {
    return undefined;
  }
  return {
    total: value.total,
    byEffectiveStatus: effectiveMap,
    byStatus,
    bySection,
    pending: value.pending,
    running: value.running,
    succeeded: value.succeeded,
    sent: value.sent,
    failed: value.failed,
    shadowWarning: value.shadow_warning,
  };
}

function parseInternalEvent(
  value: unknown,
): PushCenterInternalEventSummary | undefined {
  if (
    !record(value) ||
    !exact(value, INTERNAL_EVENT_KEYS) ||
    !nonnegativeInteger(value.raw_open) ||
    !nonnegativeInteger(value.raw_due) ||
    !nonnegativeInteger(value.eligible) ||
    !nonnegativeInteger(value.failed_retryable) ||
    !nonnegativeInteger(value.failed_terminal) ||
    !nonnegativeInteger(value.blocked)
  ) {
    return undefined;
  }
  return {
    rawOpen: value.raw_open,
    rawDue: value.raw_due,
    eligible: value.eligible,
    failedRetryable: value.failed_retryable,
    failedTerminal: value.failed_terminal,
    blocked: value.blocked,
  };
}

function parseLane(
  value: unknown,
  expectedLane: PushCenterLaneKey,
): PushCenterLaneSummary | undefined {
  if (
    !record(value) ||
    !exact(value, LANE_KEYS) ||
    value.lane !== expectedLane ||
    !LANE_SET.has(expectedLane) ||
    !nonnegativeInteger(value.max_in_flight) ||
    typeof value.enabled !== "boolean" ||
    !boundedText(value.rollout_mode, 128) ||
    !(
      value.blocked_until === null ||
      (rfc3339(value.blocked_until) && boundedText(value.blocked_until, 64))
    ) ||
    !boundedText(value.policy_version, 128) ||
    !nonnegativeInteger(value.raw_open) ||
    !nonnegativeInteger(value.held) ||
    !nonnegativeInteger(value.eligible) ||
    !nonnegativeInteger(value.policy_gated) ||
    !nonnegativeInteger(value.scheduled) ||
    !nonnegativeInteger(value.retry_wait) ||
    !nonnegativeInteger(value.rate_limited) ||
    !nonnegativeInteger(value.in_flight) ||
    !nonnegativeInteger(value.unknown) ||
    !nonnegativeInteger(value.dlq) ||
    !nonnegativeNumber(value.oldest_eligible_age_seconds) ||
    !nonnegativeNumber(value.throughput_last_minute) ||
    !nonnegativeNumber(value.accepted_last_minute) ||
    !nonnegativeNumber(value.p95_queue_wait_ms) ||
    !nonnegativeNumber(value.p95_provider_call_ms) ||
    !nonnegativeNumber(value.estimated_drain_seconds) ||
    !nonnegativeNumber(value.rate_limit_count_last_hour) ||
    !nonnegativeNumber(value.task_acceptance_rate_1m)
  ) {
    return undefined;
  }
  const internalEvent = parseInternalEvent(value.internal_event);
  if (internalEvent === undefined) return undefined;
  return {
    lane: expectedLane,
    maxInFlight: value.max_in_flight,
    enabled: value.enabled,
    rolloutMode: value.rollout_mode,
    blockedUntil: value.blocked_until,
    policyVersion: value.policy_version,
    rawOpen: value.raw_open,
    held: value.held,
    eligible: value.eligible,
    policyGated: value.policy_gated,
    scheduled: value.scheduled,
    retryWait: value.retry_wait,
    rateLimited: value.rate_limited,
    inFlight: value.in_flight,
    unknown: value.unknown,
    dlq: value.dlq,
    oldestEligibleAgeSeconds: value.oldest_eligible_age_seconds,
    internalEvent,
    throughputLastMinute: value.throughput_last_minute,
    acceptedLastMinute: value.accepted_last_minute,
    p95QueueWaitMs: value.p95_queue_wait_ms,
    p95ProviderCallMs: value.p95_provider_call_ms,
    estimatedDrainSeconds: value.estimated_drain_seconds,
    rateLimitCountLastHour: value.rate_limit_count_last_hour,
    taskAcceptanceRate1m: value.task_acceptance_rate_1m,
  };
}

export function parsePushCenterRuntimeQueue(
  value: unknown,
): PushCenterRuntimeQueue | undefined {
  if (record(value) && Object.keys(value).length === 0) {
    return { status: "not_reported" };
  }
  if (
    !record(value) ||
    !exact(value, RUNTIME_QUEUE_KEYS) ||
    !boundedText(value.policy_version, 128) ||
    !nonnegativeInteger(value.active_generation) ||
    typeof value.claim_enabled !== "boolean" ||
    !boundedText(value.rollout_mode, 128) ||
    !Array.isArray(value.lanes) ||
    value.lanes.length !== PUSH_CENTER_LANE_KEYS.length ||
    !nonnegativeInteger(value.raw_open) ||
    !nonnegativeInteger(value.held) ||
    !nonnegativeInteger(value.eligible) ||
    !nonnegativeInteger(value.policy_gated) ||
    !nonnegativeInteger(value.scheduled) ||
    !nonnegativeInteger(value.retry_wait) ||
    !nonnegativeInteger(value.rate_limited) ||
    !nonnegativeInteger(value.in_flight) ||
    !nonnegativeInteger(value.unknown) ||
    !nonnegativeInteger(value.dlq)
  ) {
    return undefined;
  }
  const lanes = value.lanes.map((lane, index) =>
    parseLane(lane, PUSH_CENTER_LANE_KEYS[index]),
  );
  const internalEvent = parseInternalEvent(value.internal_event);
  if (lanes.some((lane) => lane === undefined) || internalEvent === undefined) {
    return undefined;
  }
  return {
    status: "reported",
    policyVersion: value.policy_version,
    activeGeneration: value.active_generation,
    claimEnabled: value.claim_enabled,
    rolloutMode: value.rollout_mode,
    lanes: lanes as PushCenterLaneSummary[],
    rawOpen: value.raw_open,
    held: value.held,
    eligible: value.eligible,
    policyGated: value.policy_gated,
    scheduled: value.scheduled,
    retryWait: value.retry_wait,
    rateLimited: value.rate_limited,
    inFlight: value.in_flight,
    unknown: value.unknown,
    dlq: value.dlq,
    internalEvent,
  };
}

function sameValue(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

interface NormalSectionsEnvelope {
  readonly kind: "normal";
  readonly sections: readonly PushCenterSection[];
  readonly statusDefinitions: readonly PushCenterStatusDefinition[];
  readonly filters: PushCenterFilters;
}

interface NormalStatsEnvelope extends NormalSectionsEnvelope {
  readonly counts: PushCenterCounts;
  readonly runtimeQueue: PushCenterRuntimeQueue;
}

interface DegradedEnvelope {
  readonly kind: "degraded";
  readonly statusDefinitions: readonly PushCenterStatusDefinition[];
  readonly filters: PushCenterFilters;
  readonly runtimeQueue?: PushCenterRuntimeQueue;
  readonly degraded: PushCenterDegradedBoundary;
}

function parseNormalSections(
  value: unknown,
  expectedFilters: PushCenterFilters,
): NormalSectionsEnvelope | undefined {
  if (
    !record(value) ||
    !exact(value, NORMAL_SECTION_KEYS) ||
    value.ok !== true ||
    value.route_owner !== "ai_crm_next"
  ) {
    return undefined;
  }
  const sections = parseSections(value.sections);
  const statusDefinitions = parseStatusDefinitions(value.status_definitions);
  const filters = parseResponseFilters(value.filters, expectedFilters);
  if (sections === undefined || statusDefinitions === undefined || filters === undefined) {
    return undefined;
  }
  return { kind: "normal", sections, statusDefinitions, filters };
}

function parseNormalStats(
  value: unknown,
  expectedFilters: PushCenterFilters,
): NormalStatsEnvelope | undefined {
  if (
    !record(value) ||
    !exact(value, NORMAL_STATS_KEYS) ||
    value.ok !== true ||
    value.route_owner !== "ai_crm_next" ||
    value.real_external_call_executed !== false ||
    value.capability_owner !== "ai_crm_next/platform_foundation/push_center"
  ) {
    return undefined;
  }
  const sections = parseSections(value.sections);
  const statusDefinitions = parseStatusDefinitions(value.status_definitions);
  const filters = parseResponseFilters(value.filters, expectedFilters);
  const runtimeQueue = parsePushCenterRuntimeQueue(value.runtime_queue);
  if (
    sections === undefined ||
    statusDefinitions === undefined ||
    filters === undefined ||
    runtimeQueue === undefined
  ) {
    return undefined;
  }
  const counts = parseCounts(value.counts, sections);
  if (counts === undefined) return undefined;
  return {
    kind: "normal",
    sections,
    statusDefinitions,
    filters,
    counts,
    runtimeQueue,
  };
}

function parseDegradedCounts(value: unknown): boolean {
  return (
    record(value) &&
    exact(value, DEGRADED_COUNTS_KEYS) &&
    value.total === 0 &&
    value.pending === 0 &&
    value.running === 0 &&
    value.sent === 0 &&
    value.failed === 0 &&
    record(value.by_effective_status) &&
    Object.keys(value.by_effective_status).length === 0 &&
    record(value.by_status) &&
    Object.keys(value.by_status).length === 0 &&
    record(value.by_section) &&
    Object.keys(value.by_section).length === 0
  );
}

function parseDegradedDiagnostics(
  value: unknown,
): PushCenterDegradedBoundary["diagnostics"] | undefined {
  if (
    !record(value) ||
    !exact(value, DIAGNOSTIC_KEYS) ||
    value.production_data_ready !== false ||
    value.fixture_mode !== false ||
    value.allow_fixture_repo_in_prod !== false ||
    value.error_class !== "ReadModelUnavailableError"
  ) {
    return undefined;
  }
  return {
    productionDataReady: false,
    fixtureMode: false,
    allowFixtureRepoInProd: false,
    errorClass: "ReadModelUnavailableError",
  };
}

function parseDegradedEnvelope(
  value: unknown,
  expectedFilters: PushCenterFilters,
  includeRuntimeQueue: boolean,
): DegradedEnvelope | undefined {
  const keys = includeRuntimeQueue ? DEGRADED_STATS_KEYS : DEGRADED_SECTION_KEYS;
  if (
    !record(value) ||
    !exact(value, keys) ||
    value.ok !== true ||
    value.degraded !== true ||
    value.error !== "" ||
    value.error_code !== "production_read_unavailable" ||
    value.source_status !== "production_unavailable" ||
    value.read_model_status !== "unavailable" ||
    value.capability_owner !== "ai_crm_next/platform_foundation/push_center" ||
    value.page_error !== "推送中心读模型暂不可用，请稍后重试。" ||
    value.route_owner !== "ai_crm_next" ||
    value.fallback_used !== false ||
    value.real_external_call_executed !== false ||
    value.status_code !== 200 ||
    !Array.isArray(value.items) ||
    value.items.length !== 0 ||
    value.total !== 0 ||
    !parseDegradedCounts(value.counts) ||
    value.limit !== 50 ||
    value.offset !== 0 ||
    !Array.isArray(value.sections) ||
    value.sections.length !== 0
  ) {
    return undefined;
  }
  const diagnostics = parseDegradedDiagnostics(value.diagnostics);
  const statusDefinitions = parseStatusDefinitions(value.status_definitions);
  const filters = parseResponseFilters(value.filters, expectedFilters);
  if (diagnostics === undefined || statusDefinitions === undefined || filters === undefined) {
    return undefined;
  }
  const runtimeQueue = includeRuntimeQueue
    ? parsePushCenterRuntimeQueue(value.runtime_queue)
    : undefined;
  if (includeRuntimeQueue && runtimeQueue === undefined) return undefined;
  return {
    kind: "degraded",
    statusDefinitions,
    filters,
    runtimeQueue,
    degraded: {
      sourceStatus: "production_unavailable",
      readModelStatus: "unavailable",
      pageError: value.page_error,
      diagnostics,
    },
  };
}

export function parsePushCenterPair(
  sectionsValue: unknown,
  statsValue: unknown,
  expectedFilters: PushCenterFilters,
): PushCenterSnapshot | undefined {
  const sectionsNormal = parseNormalSections(sectionsValue, expectedFilters);
  const statsNormal = parseNormalStats(statsValue, expectedFilters);
  if (sectionsNormal !== undefined || statsNormal !== undefined) {
    if (
      sectionsNormal === undefined ||
      statsNormal === undefined ||
      !sameValue(sectionsNormal.filters, statsNormal.filters) ||
      !sameValue(sectionsNormal.sections, statsNormal.sections) ||
      !sameValue(
        sectionsNormal.statusDefinitions,
        statsNormal.statusDefinitions,
      )
    ) {
      return undefined;
    }
    return {
      boundary: "normal",
      filters: statsNormal.filters,
      sections: statsNormal.sections,
      statusDefinitions: statsNormal.statusDefinitions,
      counts: statsNormal.counts,
      runtimeQueue: statsNormal.runtimeQueue,
      realExternalCallExecuted: false,
    };
  }
  const sectionsDegraded = parseDegradedEnvelope(
    sectionsValue,
    expectedFilters,
    false,
  );
  const statsDegraded = parseDegradedEnvelope(statsValue, expectedFilters, true);
  if (
    sectionsDegraded === undefined ||
    statsDegraded === undefined ||
    !sameValue(sectionsDegraded.filters, statsDegraded.filters) ||
    !sameValue(
      sectionsDegraded.statusDefinitions,
      statsDegraded.statusDefinitions,
    ) ||
    !sameValue(sectionsDegraded.degraded, statsDegraded.degraded) ||
    statsDegraded.runtimeQueue === undefined
  ) {
    return undefined;
  }
  return {
    boundary: "degraded",
    filters: statsDegraded.filters,
    statusDefinitions: statsDegraded.statusDefinitions,
    runtimeQueue: statsDegraded.runtimeQueue,
    realExternalCallExecuted: false,
    degraded: statsDegraded.degraded,
  };
}

export async function loadPushCenterSnapshot(
  filters: PushCenterFilters = {},
  transport: PushCenterTransport = generatedPushCenterTransport,
  signal?: AbortSignal,
): Promise<PushCenterResult> {
  const normalized = normalizePushCenterFilters(filters);
  if (normalized.status !== "valid") return { status: "invalid" };
  const options: RequestInit = { credentials: "same-origin", signal };
  try {
    const [sectionsResponse, statsResponse] = await Promise.all([
      transport.readSections(normalized.filters, options),
      transport.readStats(normalized.filters, options),
    ]);
    const statuses = [sectionsResponse.status, statsResponse.status];
    if (statuses.includes(401)) return { status: "unauthenticated" };
    if (statuses.includes(403)) return { status: "forbidden" };
    if (statuses.some((status) => status !== 200)) {
      return { status: "unavailable" };
    }
    const snapshot = parsePushCenterPair(
      sectionsResponse.data,
      statsResponse.data,
      normalized.filters,
    );
    return snapshot === undefined
      ? { status: "invalid" }
      : { status: "loaded", snapshot };
  } catch {
    return { status: "unavailable" };
  }
}
