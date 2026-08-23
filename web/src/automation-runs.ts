import {
  getLegacyInternalEvent,
  getLegacyInternalEventDiagnostics,
  listLegacyInternalEvents,
  listAutomationTriggerRuns,
  type GetLegacyInternalEventDiagnosticsParams,
  type ListLegacyInternalEventsParams,
  type ListAutomationTriggerRunsParams,
} from "./api/generated/health";

export type AutomationRunsRole = "admin" | "ops" | "sales";
export const AUTOMATION_RUNS_PAGE_SIZE = 50;

export interface AutomationRunRecord {
  readonly runID: string;
  readonly requestID: string;
  readonly agentCode: "tag-trigger-v1";
  readonly runStatus: "completed";
  readonly triggerSource: "customer.tag_applied";
  readonly customerID: number;
  readonly tagID: number;
  readonly sourceEventID: number;
  readonly triggeredEventID: number;
  readonly startedAt: string;
  readonly completedAt: string;
  readonly hasError: false;
}

export interface AutomationRunsPage {
  readonly items: readonly AutomationRunRecord[];
  readonly total: number;
  readonly page: number;
  readonly pageSize: number;
}

export type AutomationSourceEventConsumer =
  | "automation.tag-trigger.v1"
  | "stats.tag-applied.v1";
export type AutomationSourceEventDeliveryStatus =
  | "pending"
  | "processing"
  | "completed"
  | "final_failed"
  | "outcome_unknown";

export interface AutomationSourceEventDelivery {
  readonly consumer: AutomationSourceEventConsumer;
  readonly status: AutomationSourceEventDeliveryStatus;
  readonly attemptCount: number;
  readonly completedAt: string | null;
}

export interface AutomationSourceEvent {
  readonly eventID: number;
  readonly eventType: "customer.tag_applied";
  readonly occurredAt: string;
  readonly dispatched: boolean;
  readonly deliveries: readonly AutomationSourceEventDelivery[];
  readonly observedAt: string;
}

export type AutomationDiagnosticConsumer =
  | "automation.tag-trigger.v1"
  | "stats.tag-applied.v1"
  | "operation-cycle.fact.v1"
  | "cloud-campaign.fact.v1"
  | "outbound-campaign-handoff.fact.v1";
export type AutomationDiagnosticStatus =
  | "pending"
  | "processing"
  | "completed"
  | "final_failed"
  | "outcome_unknown";

export interface AutomationDiagnostics {
  readonly eventCount: number;
  readonly undispatchedEventCount: number;
  readonly deliveryCounts: Readonly<Record<AutomationDiagnosticStatus, number>>;
  readonly consumerRegistry: readonly Readonly<{
    readonly consumer: AutomationDiagnosticConsumer;
    readonly eventType: "customer.tag_applied" | "operation_cycle.fact_recorded" | "cloud_campaign.fact_recorded" | "outbound.campaign_handoff_fact_recorded";
  }>[];
  readonly observedAt: string;
  readonly observedDomains: readonly ["event_log", "event_deliveries"];
  readonly unobservedDomains: readonly ["river_queue", "outbound_provider", "external_delivery"];
}

type AutomationInternalEventTypeWithDeliveries =
  | "customer.tag_applied"
  | "operation_cycle.fact_recorded"
  | "cloud_campaign.fact_recorded"
  | "outbound.campaign_handoff_fact_recorded";

export interface AutomationInternalEventDelivery {
  readonly consumer: AutomationDiagnosticConsumer;
  readonly status: AutomationDiagnosticStatus;
  readonly attemptCount: number;
  readonly completedAt: string | null;
}

export interface AutomationInternalEvent {
  readonly eventID: number;
  readonly eventType: string;
  readonly occurredAt: string;
  readonly dispatched: boolean;
  readonly deliveries: readonly AutomationInternalEventDelivery[];
}

export interface AutomationInternalEventsPage {
  readonly items: readonly AutomationInternalEvent[];
  readonly total: number;
  readonly limit: number;
  readonly offset: number;
  readonly observedAt: string;
}

export interface AutomationRunsTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(
  params: ListAutomationTriggerRunsParams,
  options: RequestInit,
): Promise<AutomationRunsTransportResponse> {
  return listAutomationTriggerRuns(params, options);
}

async function generatedSourceEvent(
  eventID: number,
  options: RequestInit,
): Promise<AutomationRunsTransportResponse> {
  return getLegacyInternalEvent(String(eventID), options);
}

async function generatedDiagnostics(
  params: GetLegacyInternalEventDiagnosticsParams,
  options: RequestInit,
): Promise<AutomationRunsTransportResponse> {
  return getLegacyInternalEventDiagnostics(params, options);
}

async function generatedInternalEvents(
  params: ListLegacyInternalEventsParams,
  options: RequestInit,
): Promise<AutomationRunsTransportResponse> {
  return listLegacyInternalEvents(params, options);
}

export interface AutomationRunsTransport {
  readonly list: typeof generatedList;
  // Optional only to preserve existing injected list-only test transports;
  // production always supplies the generated same-origin reader below.
  readonly sourceEvent?: typeof generatedSourceEvent;
  readonly diagnostics?: typeof generatedDiagnostics;
  readonly internalEvents?: typeof generatedInternalEvents;
}

export const generatedAutomationRunsTransport: AutomationRunsTransport = {
  list: generatedList,
  sourceEvent: generatedSourceEvent,
  diagnostics: generatedDiagnostics,
  internalEvents: generatedInternalEvents,
};

export type AutomationRunsFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type AutomationRunsResult =
  | { readonly status: "loaded"; readonly page: AutomationRunsPage }
  | { readonly status: AutomationRunsFailure };

export type AutomationSourceEventFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "invalid"
  | "unavailable";

export type AutomationSourceEventResult =
  | { readonly status: "loaded"; readonly sourceEvent: AutomationSourceEvent }
  | { readonly status: AutomationSourceEventFailure };
export type AutomationDiagnosticsResult =
  | { readonly status: "loaded"; readonly diagnostics: AutomationDiagnostics }
  | { readonly status: AutomationRunsFailure };
export type AutomationInternalEventsResult =
  | { readonly status: "loaded"; readonly page: AutomationInternalEventsPage }
  | { readonly status: AutomationRunsFailure };

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

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function timestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) ||
    !Number.isFinite(Date.parse(value))
  ) {
    return false;
  }
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
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

const SOURCE_EVENT_CONSUMERS: ReadonlySet<AutomationSourceEventConsumer> =
  new Set(["automation.tag-trigger.v1", "stats.tag-applied.v1"]);
const SOURCE_EVENT_DELIVERY_STATUSES: ReadonlySet<AutomationSourceEventDeliveryStatus> =
  new Set([
    "pending",
    "processing",
    "completed",
    "final_failed",
    "outcome_unknown",
  ]);
const MAX_INT32 = 2_147_483_647;
const DIAGNOSTIC_STATUSES: readonly AutomationDiagnosticStatus[] = [
  "pending", "processing", "completed", "final_failed", "outcome_unknown",
];
const DIAGNOSTIC_REGISTRY: readonly Readonly<{
  readonly consumer: AutomationDiagnosticConsumer;
  readonly eventType: AutomationInternalEventTypeWithDeliveries;
}>[] = [
  { consumer: "automation.tag-trigger.v1", eventType: "customer.tag_applied" },
  { consumer: "stats.tag-applied.v1", eventType: "customer.tag_applied" },
  { consumer: "operation-cycle.fact.v1", eventType: "operation_cycle.fact_recorded" },
  { consumer: "cloud-campaign.fact.v1", eventType: "cloud_campaign.fact_recorded" },
  { consumer: "outbound-campaign-handoff.fact.v1", eventType: "outbound.campaign_handoff_fact_recorded" },
];
const DIAGNOSTIC_OBSERVED_DOMAINS = ["event_log", "event_deliveries"] as const;
const DIAGNOSTIC_UNOBSERVED_DOMAINS = [
  "river_queue", "outbound_provider", "external_delivery",
] as const;
const INTERNAL_EVENT_TYPES_WITH_DELIVERIES: ReadonlySet<AutomationInternalEventTypeWithDeliveries> = new Set([
  "customer.tag_applied",
  "operation_cycle.fact_recorded",
  "cloud_campaign.fact_recorded",
  "outbound.campaign_handoff_fact_recorded",
]);
const INTERNAL_EVENT_CONSUMERS_BY_TYPE: Readonly<
  Record<AutomationInternalEventTypeWithDeliveries, readonly AutomationDiagnosticConsumer[]>
> = {
  "customer.tag_applied": [
    "automation.tag-trigger.v1",
    "stats.tag-applied.v1",
  ],
  "operation_cycle.fact_recorded": ["operation-cycle.fact.v1"],
  "cloud_campaign.fact_recorded": ["cloud-campaign.fact.v1"],
  "outbound.campaign_handoff_fact_recorded": ["outbound-campaign-handoff.fact.v1"],
};

function exactStrings(value: unknown, expected: readonly string[]): boolean {
  return (
    Array.isArray(value) &&
    value.length === expected.length &&
    value.every((item, index) => item === expected[index])
  );
}

export function parseAutomationDiagnostics(value: unknown): AutomationDiagnostics | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok", "filters", "event_count", "undispatched_event_count", "delivery_counts",
      "consumer_registry", "observed_at", "registry_id", "source_status",
      "observed_domains", "unobserved_domains", "external_delivery", "route_owner",
      "real_external_call_executed",
    ]) ||
    value.ok !== true ||
    !record(value.filters) ||
    !exact(value.filters, ["event_type", "consumer", "status"]) ||
    value.filters.event_type !== "" || value.filters.consumer !== "" || value.filters.status !== "" ||
    !nonnegative(value.event_count) || !nonnegative(value.undispatched_event_count) ||
    value.undispatched_event_count > value.event_count ||
    !record(value.delivery_counts) ||
    !Array.isArray(value.consumer_registry) ||
    value.consumer_registry.length !== DIAGNOSTIC_REGISTRY.length ||
    !timestamp(value.observed_at) ||
    value.registry_id !== "v2-internal-events.v1" ||
    value.source_status !== "local_read_model" ||
    !exactStrings(value.observed_domains, DIAGNOSTIC_OBSERVED_DOMAINS) ||
    !exactStrings(value.unobserved_domains, DIAGNOSTIC_UNOBSERVED_DOMAINS) ||
    value.external_delivery !== "unknown" || value.route_owner !== "ai_crm_next" ||
    value.real_external_call_executed !== false
  ) return undefined;
  const deliveryCounts = value.delivery_counts;
  const pending = deliveryCounts.pending;
  const processing = deliveryCounts.processing;
  const completed = deliveryCounts.completed;
  const finalFailed = deliveryCounts.final_failed;
  const outcomeUnknown = deliveryCounts.outcome_unknown;
  if (
    !exact(deliveryCounts, DIAGNOSTIC_STATUSES) ||
    !nonnegative(pending) || !nonnegative(processing) || !nonnegative(completed) ||
    !nonnegative(finalFailed) || !nonnegative(outcomeUnknown)
  ) return undefined;
  for (const [index, expected] of DIAGNOSTIC_REGISTRY.entries()) {
    const binding = value.consumer_registry[index];
    if (
      !record(binding) || !exact(binding, ["consumer", "event_types"]) ||
      binding.consumer !== expected.consumer || !exactStrings(binding.event_types, [expected.eventType])
    ) return undefined;
  }
  return {
    eventCount: value.event_count,
    undispatchedEventCount: value.undispatched_event_count,
    deliveryCounts: {
      pending,
      processing,
      completed,
      final_failed: finalFailed,
      outcome_unknown: outcomeUnknown,
    },
    consumerRegistry: DIAGNOSTIC_REGISTRY,
    observedAt: value.observed_at,
    observedDomains: [...DIAGNOSTIC_OBSERVED_DOMAINS],
    unobservedDomains: [...DIAGNOSTIC_UNOBSERVED_DOMAINS],
  };
}

function terminalInternalEventDelivery(status: AutomationDiagnosticStatus): boolean {
  return (
    status === "completed" ||
    status === "final_failed" ||
    status === "outcome_unknown"
  );
}

function validInternalEventText(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    value.length === 0 ||
    value !== value.replace(/^[ \t\r\n\v\f]+|[ \t\r\n\v\f]+$/g, "") ||
    /[\u0000-\u001F\u007F]/.test(value) ||
    new TextEncoder().encode(value).length > 200
  ) return false;
  for (const character of value) {
    const codePoint = character.codePointAt(0);
    if (codePoint !== undefined && codePoint >= 0xD800 && codePoint <= 0xDFFF) {
      return false;
    }
  }
  return true;
}

function parseAutomationInternalEventDelivery(
  value: unknown,
  eventType: AutomationInternalEventTypeWithDeliveries,
): AutomationInternalEventDelivery | undefined {
  if (
    !record(value) ||
    !exact(value, ["consumer", "status", "attempt_count", "completed_at"]) ||
    typeof value.consumer !== "string" ||
    !INTERNAL_EVENT_CONSUMERS_BY_TYPE[eventType].includes(
      value.consumer as AutomationDiagnosticConsumer,
    ) ||
    typeof value.status !== "string" ||
    !DIAGNOSTIC_STATUSES.includes(value.status as AutomationDiagnosticStatus) ||
    !nonnegative(value.attempt_count) ||
    value.attempt_count > MAX_INT32 ||
    (terminalInternalEventDelivery(value.status as AutomationDiagnosticStatus)
      ? !timestamp(value.completed_at)
      : value.completed_at !== null)
  ) return undefined;
  return {
    consumer: value.consumer as AutomationDiagnosticConsumer,
    status: value.status as AutomationDiagnosticStatus,
    attemptCount: value.attempt_count,
    completedAt: terminalInternalEventDelivery(
      value.status as AutomationDiagnosticStatus,
    )
      ? (value.completed_at as string)
      : null,
  };
}

function parseAutomationInternalEvent(value: unknown): AutomationInternalEvent | undefined {
  if (
    !record(value) ||
    !exact(value, ["event_id", "event_type", "occurred_at", "dispatched", "deliveries"]) ||
    !positive(value.event_id) ||
    !validInternalEventText(value.event_type) ||
    !timestamp(value.occurred_at) ||
    typeof value.dispatched !== "boolean" ||
    !Array.isArray(value.deliveries) ||
    (INTERNAL_EVENT_TYPES_WITH_DELIVERIES.has(
      value.event_type as AutomationInternalEventTypeWithDeliveries,
    )
      ? value.deliveries.length > INTERNAL_EVENT_CONSUMERS_BY_TYPE[
          value.event_type as AutomationInternalEventTypeWithDeliveries
        ].length
      : value.deliveries.length !== 0)
  ) return undefined;
  const eventType = value.event_type as AutomationInternalEventTypeWithDeliveries;
  if (!INTERNAL_EVENT_TYPES_WITH_DELIVERIES.has(eventType)) {
    return {
      eventID: value.event_id,
      eventType: value.event_type,
      occurredAt: value.occurred_at,
      dispatched: value.dispatched,
      deliveries: [],
    };
  }
  const deliveries = value.deliveries.map((delivery) =>
    parseAutomationInternalEventDelivery(delivery, eventType),
  );
  if (
    deliveries.includes(undefined) ||
    new Set(
      (deliveries as readonly AutomationInternalEventDelivery[]).map(
        (delivery) => delivery.consumer,
      ),
    ).size !== deliveries.length ||
    (deliveries as readonly AutomationInternalEventDelivery[]).some(
      (delivery, index, all) =>
        index > 0 &&
        INTERNAL_EVENT_CONSUMERS_BY_TYPE[eventType].indexOf(
          all[index - 1]!.consumer,
        ) >= INTERNAL_EVENT_CONSUMERS_BY_TYPE[eventType].indexOf(delivery.consumer),
    )
  ) return undefined;
  return {
    eventID: value.event_id,
    eventType: value.event_type,
    occurredAt: value.occurred_at,
    dispatched: value.dispatched,
    deliveries: deliveries as readonly AutomationInternalEventDelivery[],
  };
}

export function parseAutomationInternalEventsPage(
  value: unknown,
  expectedOffset: number,
): AutomationInternalEventsPage | undefined {
  if (
    !nonnegative(expectedOffset) ||
    expectedOffset > 100_000 ||
    !record(value) ||
    !exact(value, [
      "ok", "items", "total", "limit", "offset", "observed_at", "registry_id",
      "source_status", "delivery_observation_available", "external_delivery", "route_owner",
      "real_external_call_executed",
    ]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    !nonnegative(value.total) ||
    value.limit !== AUTOMATION_RUNS_PAGE_SIZE ||
    value.offset !== expectedOffset ||
    !timestamp(value.observed_at) ||
    value.registry_id !== "v2-internal-events.v1" ||
    value.source_status !== "local_read_model" ||
    value.delivery_observation_available !== true ||
    value.external_delivery !== "unknown" ||
    value.route_owner !== "ai_crm_next" ||
    value.real_external_call_executed !== false ||
    value.items.length > value.limit
  ) return undefined;
  const items = value.items.map(parseAutomationInternalEvent);
  if (
    items.includes(undefined) ||
    new Set(items.map((item) => item?.eventID)).size !== items.length ||
    (items.length === 0
      ? value.offset < value.total
      : value.total < value.offset + items.length ||
        (value.offset + items.length < value.total && items.length !== value.limit)) ||
    (items as readonly AutomationInternalEvent[]).some((item, index, all) => {
      if (index === 0) return false;
      const previous = all[index - 1];
      if (!previous) return true;
      const previousTime = Date.parse(previous.occurredAt);
      const currentTime = Date.parse(item.occurredAt);
      return previousTime < currentTime ||
        (previousTime === currentTime && previous.eventID <= item.eventID);
    })
  ) return undefined;
  return {
    items: items as readonly AutomationInternalEvent[],
    total: value.total,
    limit: value.limit,
    offset: value.offset,
    observedAt: value.observed_at,
  };
}

function terminalSourceEventDelivery(
  status: AutomationSourceEventDeliveryStatus,
): boolean {
  return (
    status === "completed" ||
    status === "final_failed" ||
    status === "outcome_unknown"
  );
}

function parseAutomationSourceEventDelivery(
  value: unknown,
): AutomationSourceEventDelivery | undefined {
  if (
    !record(value) ||
    !exact(value, ["consumer", "status", "attempt_count", "completed_at"]) ||
    typeof value.consumer !== "string" ||
    !SOURCE_EVENT_CONSUMERS.has(value.consumer as AutomationSourceEventConsumer) ||
    typeof value.status !== "string" ||
    !SOURCE_EVENT_DELIVERY_STATUSES.has(
      value.status as AutomationSourceEventDeliveryStatus,
    ) ||
    !nonnegative(value.attempt_count) ||
    value.attempt_count > MAX_INT32 ||
    (terminalSourceEventDelivery(
      value.status as AutomationSourceEventDeliveryStatus,
    )
      ? !timestamp(value.completed_at)
      : value.completed_at !== null)
  ) {
    return undefined;
  }
  return {
    consumer: value.consumer as AutomationSourceEventConsumer,
    status: value.status as AutomationSourceEventDeliveryStatus,
    attemptCount: value.attempt_count,
    completedAt: terminalSourceEventDelivery(
      value.status as AutomationSourceEventDeliveryStatus,
    )
      ? (value.completed_at as string)
      : null,
  };
}

export function parseAutomationSourceEvent(
  value: unknown,
  expectedEventID: number,
): AutomationSourceEvent | undefined {
  if (
    !positive(expectedEventID) ||
    !record(value) ||
    !exact(value, [
      "ok",
      "item",
      "observed_at",
      "registry_id",
      "source_status",
      "delivery_observation_available",
      "external_delivery",
      "route_owner",
      "real_external_call_executed",
    ]) ||
    value.ok !== true ||
    !record(value.item) ||
    !exact(value.item, [
      "event_id",
      "event_type",
      "occurred_at",
      "dispatched",
      "deliveries",
    ]) ||
    value.item.event_id !== expectedEventID ||
    value.item.event_type !== "customer.tag_applied" ||
    !timestamp(value.item.occurred_at) ||
    typeof value.item.dispatched !== "boolean" ||
    !Array.isArray(value.item.deliveries) ||
    value.item.deliveries.length > SOURCE_EVENT_CONSUMERS.size ||
    !timestamp(value.observed_at) ||
    value.registry_id !== "v2-internal-events.v1" ||
    value.source_status !== "local_read_model" ||
    value.delivery_observation_available !== true ||
    value.external_delivery !== "unknown" ||
    value.route_owner !== "ai_crm_next" ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const deliveries = value.item.deliveries.map(parseAutomationSourceEventDelivery);
  if (
    deliveries.includes(undefined) ||
    new Set(
      (deliveries as readonly AutomationSourceEventDelivery[]).map(
        (delivery) => delivery.consumer,
      ),
    ).size !== deliveries.length
  ) {
    return undefined;
  }
  return {
    eventID: expectedEventID,
    eventType: "customer.tag_applied",
    occurredAt: value.item.occurred_at,
    dispatched: value.item.dispatched,
    deliveries: deliveries as readonly AutomationSourceEventDelivery[],
    observedAt: value.observed_at,
  };
}

export function parseAutomationRun(value: unknown): AutomationRunRecord | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "run_id",
      "request_id",
      "agent_code",
      "run_status",
      "trigger_source",
      "customer_id",
      "tag_id",
      "source_event_id",
      "triggered_event_id",
      "started_at",
      "completed_at",
      "has_error",
    ]) ||
    typeof value.run_id !== "string" ||
    !/^automation-trigger:[1-9]\d*$/.test(value.run_id) ||
    typeof value.request_id !== "string" ||
    !/^event:[1-9]\d*$/.test(value.request_id) ||
    value.agent_code !== "tag-trigger-v1" ||
    value.run_status !== "completed" ||
    value.trigger_source !== "customer.tag_applied" ||
    !positive(value.customer_id) ||
    !positive(value.tag_id) ||
    !positive(value.source_event_id) ||
    !positive(value.triggered_event_id) ||
    !timestamp(value.started_at) ||
    !timestamp(value.completed_at) ||
    Date.parse(value.completed_at) < Date.parse(value.started_at) ||
    value.has_error !== false
  ) {
    return undefined;
  }

  return {
    runID: value.run_id,
    requestID: value.request_id,
    agentCode: value.agent_code,
    runStatus: value.run_status,
    triggerSource: value.trigger_source,
    customerID: value.customer_id,
    tagID: value.tag_id,
    sourceEventID: value.source_event_id,
    triggeredEventID: value.triggered_event_id,
    startedAt: value.started_at,
    completedAt: value.completed_at,
    hasError: false,
  };
}

export function parseAutomationRunsPage(
  value: unknown,
  expectedPage: number,
): AutomationRunsPage | undefined {
  if (
    !positive(expectedPage) ||
    expectedPage > 10_000 ||
    !record(value) ||
    !exact(value, ["items", "total", "page", "page_size", "visibility"]) ||
    !Array.isArray(value.items) ||
    !nonnegative(value.total) ||
    value.page !== expectedPage ||
    !positive(value.page_size) ||
    value.page_size !== AUTOMATION_RUNS_PAGE_SIZE ||
    value.visibility !== "masked" ||
    value.items.length > value.page_size
  ) {
    return undefined;
  }
  const items = value.items.map(parseAutomationRun);
  if (
    items.some((item) => item === undefined) ||
    value.total < items.length ||
    new Set(items.map((item) => item?.runID)).size !== items.length
  ) {
    return undefined;
  }
  return {
    items: items as AutomationRunRecord[],
    total: value.total,
    page: value.page,
    pageSize: value.page_size,
  };
}

function failure(status: number): AutomationRunsFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 400) return "invalid";
  return "unavailable";
}

function sourceEventFailure(status: number): AutomationSourceEventFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 400) return "invalid";
  return "unavailable";
}

export async function loadAutomationRuns(
  transport: AutomationRunsTransport,
  page = 1,
): Promise<AutomationRunsResult> {
  if (!positive(page) || page > 10_000) return { status: "invalid" };
  try {
    const response = await transport.list(
      { page, page_size: AUTOMATION_RUNS_PAGE_SIZE, visibility: "masked" },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const parsed = parseAutomationRunsPage(response.data, page);
    return parsed ? { status: "loaded", page: parsed } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadAutomationSourceEvent(
  transport: AutomationRunsTransport,
  eventID: number,
): Promise<AutomationSourceEventResult> {
  if (!positive(eventID)) return { status: "invalid" };
  if (!transport.sourceEvent) return { status: "unavailable" };
  try {
    const response = await transport.sourceEvent(eventID, {
      credentials: "same-origin",
    });
    if (response.status !== 200) {
      return { status: sourceEventFailure(response.status) };
    }
    const sourceEvent = parseAutomationSourceEvent(response.data, eventID);
    return sourceEvent
      ? { status: "loaded", sourceEvent }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadAutomationDiagnostics(
  transport: AutomationRunsTransport,
): Promise<AutomationDiagnosticsResult> {
  if (!transport.diagnostics) return { status: "unavailable" };
  try {
    const response = await transport.diagnostics({}, { credentials: "same-origin" });
    if (response.status !== 200) return { status: failure(response.status) };
    const diagnostics = parseAutomationDiagnostics(response.data);
    return diagnostics ? { status: "loaded", diagnostics } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadAutomationInternalEvents(
  transport: AutomationRunsTransport,
  offset = 0,
): Promise<AutomationInternalEventsResult> {
  if (!nonnegative(offset) || offset > 100_000 || !transport.internalEvents) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.internalEvents(
      { limit: String(AUTOMATION_RUNS_PAGE_SIZE), offset: String(offset) },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const page = parseAutomationInternalEventsPage(response.data, offset);
    return page ? { status: "loaded", page } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

// React does not commit a disabled button between two synchronous clicks. The
// ref gate prevents a second local GET while the first source-event read is
// still pending; it never retries or mutates state on its own.
export function startAutomationSourceEventRead(
  lock: { current: boolean },
  execute: () => Promise<void>,
): Promise<void> | undefined {
  if (lock.current) return undefined;
  lock.current = true;
  return (async () => {
    try {
      await execute();
    } finally {
      lock.current = false;
    }
  })();
}

export function previousAutomationRunsPage(
  page: AutomationRunsPage,
): number | undefined {
  return page.page > 1 ? page.page - 1 : undefined;
}

export function nextAutomationRunsPage(
  page: AutomationRunsPage,
): number | undefined {
  return page.page < 10_000 && page.page * page.pageSize < page.total
    ? page.page + 1
    : undefined;
}

export function previousAutomationInternalEventsOffset(
  page: AutomationInternalEventsPage,
): number | undefined {
  return page.offset >= page.limit ? page.offset - page.limit : undefined;
}

export function nextAutomationInternalEventsOffset(
  page: AutomationInternalEventsPage,
): number | undefined {
  const nextOffset = page.offset + page.limit;
  return page.offset + page.items.length < page.total && nextOffset <= 100_000
    ? nextOffset
    : undefined;
}
