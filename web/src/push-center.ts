import { getLegacyPushCenterSections } from "./api/generated/health";

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

const PUSH_CENTER_STATUS_KEYS = [
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

const PUSH_CENTER_DEGRADED_KEYS = [
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

export interface PushCenterSection {
  readonly key: (typeof PUSH_CENTER_SECTION_KEYS)[number];
  readonly label: string;
  readonly count: number;
}

export interface PushCenterSnapshot {
  readonly sections: readonly PushCenterSection[];
}

export interface PushCenterTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedRead(
  options: RequestInit,
): Promise<PushCenterTransportResponse> {
  // Undefined is intentional: this overview never submits any PII-bearing
  // Push Center filter to the server.
  return getLegacyPushCenterSections(undefined, options);
}

export interface PushCenterTransport {
  readonly read: typeof generatedRead;
}

export const generatedPushCenterTransport: PushCenterTransport = {
  read: generatedRead,
};

export type PushCenterFailure =
  "unauthenticated" | "forbidden" | "invalid" | "unavailable";

export type PushCenterResult =
  | { readonly status: "loaded"; readonly snapshot: PushCenterSnapshot }
  | { readonly status: PushCenterFailure };

function record(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null)
  );
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

function boundedText(value: unknown, maximum = 1024): value is string {
  return typeof value === "string" && Array.from(value).length <= maximum;
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function emptyFilters(value: unknown): boolean {
  return record(value) && Object.keys(value).length === 0;
}

function validDegradedCounts(value: unknown): boolean {
  return (
    record(value) &&
    exact(value, [
      "total",
      "by_effective_status",
      "by_status",
      "by_section",
      "pending",
      "running",
      "sent",
      "failed",
    ]) &&
    value.total === 0 &&
    value.pending === 0 &&
    value.running === 0 &&
    value.sent === 0 &&
    value.failed === 0 &&
    emptyFilters(value.by_effective_status) &&
    emptyFilters(value.by_status) &&
    emptyFilters(value.by_section)
  );
}

function validDegradedDiagnostics(value: unknown): boolean {
  return (
    record(value) &&
    exact(value, [
      "production_data_ready",
      "fixture_mode",
      "allow_fixture_repo_in_prod",
      "error_class",
    ]) &&
    value.production_data_ready === false &&
    value.fixture_mode === false &&
    value.allow_fixture_repo_in_prod === false &&
    value.error_class === "ReadModelUnavailableError"
  );
}

function isPushCenterSectionsDegraded(value: unknown): boolean {
  return (
    record(value) &&
    exact(value, PUSH_CENTER_DEGRADED_KEYS) &&
    value.ok === true &&
    value.degraded === true &&
    value.error === "" &&
    value.error_code === "production_read_unavailable" &&
    value.source_status === "production_unavailable" &&
    value.read_model_status === "unavailable" &&
    value.capability_owner === "ai_crm_next/platform_foundation/push_center" &&
    value.page_error === "推送中心读模型暂不可用，请稍后重试。" &&
    validDegradedDiagnostics(value.diagnostics) &&
    value.route_owner === "ai_crm_next" &&
    value.fallback_used === false &&
    value.real_external_call_executed === false &&
    value.status_code === 200 &&
    Array.isArray(value.items) &&
    value.items.length === 0 &&
    value.total === 0 &&
    validDegradedCounts(value.counts) &&
    validStatusDefinitions(value.status_definitions) &&
    emptyFilters(value.filters) &&
    value.limit === 50 &&
    value.offset === 0 &&
    Array.isArray(value.sections) &&
    value.sections.length === 0
  );
}

function parseSection(
  value: unknown,
  expectedKey: (typeof PUSH_CENTER_SECTION_KEYS)[number],
): PushCenterSection | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "key",
      "label",
      "effect_types",
      "capability_key",
      "count",
    ]) ||
    value.key !== expectedKey ||
    !boundedText(value.label, 128) ||
    !Array.isArray(value.effect_types) ||
    value.effect_types.length > 16 ||
    !value.effect_types.every((item) => boundedText(item, 128)) ||
    !boundedText(value.capability_key, 128) ||
    !nonnegative(value.count)
  ) {
    return undefined;
  }
  // effect_types and capability_key are contract-validated but deliberately
  // discarded: they are not a provider-result view or a control surface.
  return { key: expectedKey, label: value.label, count: value.count };
}

function validStatusDefinitions(value: unknown): boolean {
  return (
    Array.isArray(value) &&
    value.length === PUSH_CENTER_STATUS_KEYS.length &&
    value.every((item, index) => {
      if (!record(item) || !exact(item, ["key", "label", "definition"]))
        return false;
      return (
        item.key === PUSH_CENTER_STATUS_KEYS[index] &&
        boundedText(item.label, 128) &&
        boundedText(item.definition)
      );
    })
  );
}

export function parsePushCenterSections(
  value: unknown,
): PushCenterSnapshot | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "sections",
      "status_definitions",
      "filters",
      "route_owner",
    ]) ||
    value.ok !== true ||
    value.route_owner !== "ai_crm_next" ||
    !emptyFilters(value.filters) ||
    !Array.isArray(value.sections) ||
    value.sections.length !== PUSH_CENTER_SECTION_KEYS.length ||
    !validStatusDefinitions(value.status_definitions)
  ) {
    return undefined;
  }
  const sections = value.sections.map((item, index) =>
    parseSection(item, PUSH_CENTER_SECTION_KEYS[index]),
  );
  if (sections.some((section) => section === undefined)) return undefined;
  return { sections: sections as PushCenterSection[] };
}

export async function loadPushCenterSections(
  transport: PushCenterTransport = generatedPushCenterTransport,
): Promise<PushCenterResult> {
  try {
    const response = await transport.read({ credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status !== 200) return { status: "unavailable" };
    if (isPushCenterSectionsDegraded(response.data)) {
      return { status: "unavailable" };
    }
    const snapshot = parsePushCenterSections(response.data);
    return snapshot ? { status: "loaded", snapshot } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}
