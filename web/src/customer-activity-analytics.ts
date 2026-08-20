/* eslint-disable no-unused-vars -- injected transport signatures freeze the browser contract. */
export type CustomerActivityWindowDays = 7 | 30 | 90;

export interface CustomerActivityTypeFacet { readonly eventType: string; readonly count: number; readonly lastOccurredAt: string }
export interface CustomerActivityDayCount { readonly day: string; readonly count: number }
export interface CustomerActivityAnalytics {
  readonly customerID: number; readonly windowDays: CustomerActivityWindowDays;
  readonly from: string; readonly through: string; readonly totalEvents: number;
  readonly activeDays: number; readonly uniqueEventTypes: number;
  readonly lastOccurredAt?: string; readonly typeFacets: readonly CustomerActivityTypeFacet[];
  readonly typeFacetsTruncated: boolean; readonly dailyCounts: readonly CustomerActivityDayCount[];
}

export interface CustomerActivityAnalyticsResponse { readonly status: number; readonly data: unknown }
export interface CustomerActivityAnalyticsTransport {
  readonly get: (customerID: number, params: { readonly window_days: CustomerActivityWindowDays }, options: RequestInit) => Promise<CustomerActivityAnalyticsResponse>;
}
export type CustomerActivityAnalyticsLoadResult =
  | { readonly status: "loaded"; readonly analytics: CustomerActivityAnalytics }
  | { readonly status: "invalid" | "unauthenticated" | "forbidden" | "not_found" | "unavailable" };

const KEYS = ["customer_id", "window_days", "from", "through", "total_events", "active_days", "unique_event_types", "last_occurred_at", "type_facets", "type_facets_truncated", "daily_counts", "payload_included", "actor_included", "identity_included", "real_external_call_executed"] as const;
const TYPE_KEYS = ["event_type", "count", "last_occurred_at"] as const;
const DAY_KEYS = ["day", "count"] as const;

function record(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value); }
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean { const actual = Object.keys(value); return actual.length === keys.length && keys.every((key) => Object.hasOwn(value, key)); }
function safePositiveID(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function nonnegative(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 0; }
function windowDays(value: unknown): value is CustomerActivityWindowDays { return value === 7 || value === 30 || value === 90; }
function safeText(value: unknown): value is string { return typeof value === "string" && value.trim() === value && value.length > 0 && new TextEncoder().encode(value).length <= 800 && !/[\u0000-\u001f\u007f]/u.test(value); }
function leap(year: number): boolean { return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0); }
function timestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(value);
  if (!match) return false;
  const year = Number(match[1]), month = Number(match[2]), day = Number(match[3]), hour = Number(match[4]), minute = Number(match[5]), second = Number(match[6]);
  const days = [31, leap(year) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days[month - 1] || hour > 23 || minute > 59 || second > 59) return false;
  if (match[8] !== "Z") { const offsetHour = Number(match[8].slice(1, 3)), offsetMinute = Number(match[8].slice(4, 6)); if (offsetHour > 23 || offsetMinute > 59) return false; }
  return Number.isFinite(Date.parse(value));
}
function day(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value); if (!match) return false;
  const year = Number(match[1]), month = Number(match[2]), date = Number(match[3]);
  const days = [31, leap(year) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]; return year >= 1 && month >= 1 && month <= 12 && date >= 1 && date <= days[month - 1];
}

export function parseCustomerActivityAnalytics(value: unknown, expectedCustomerID: number, expectedWindow: CustomerActivityWindowDays): CustomerActivityAnalytics | undefined {
  if (!record(value) || !exact(value, KEYS) || value.customer_id !== expectedCustomerID || value.window_days !== expectedWindow || !timestamp(value.from) || !timestamp(value.through) || Date.parse(value.from) >= Date.parse(value.through) || !nonnegative(value.total_events) || !nonnegative(value.active_days) || !nonnegative(value.unique_event_types) || (value.last_occurred_at !== null && !timestamp(value.last_occurred_at)) || !Array.isArray(value.type_facets) || !Array.isArray(value.daily_counts) || typeof value.type_facets_truncated !== "boolean" || value.payload_included !== false || value.actor_included !== false || value.identity_included !== false || value.real_external_call_executed !== false) return undefined;
  if ((value.total_events === 0) !== (value.last_occurred_at === null) || value.active_days > expectedWindow + 1 || value.unique_event_types > value.total_events) return undefined;
  const types: CustomerActivityTypeFacet[] = []; let typeCount = 0;
  for (const item of value.type_facets) {
    if (!record(item) || !exact(item, TYPE_KEYS) || !safeText(item.event_type) || !nonnegative(item.count) || item.count === 0 || !timestamp(item.last_occurred_at) || Date.parse(item.last_occurred_at) < Date.parse(value.from) || Date.parse(item.last_occurred_at) > Date.parse(value.through)) return undefined;
    const previous = types.at(-1); if (previous && (previous.count < item.count || (previous.count === item.count && previous.eventType >= item.event_type))) return undefined;
    types.push({ eventType: item.event_type, count: item.count, lastOccurredAt: item.last_occurred_at }); typeCount += item.count;
  }
  if (types.length > 50 || types.length > value.unique_event_types || (!value.type_facets_truncated && (types.length !== value.unique_event_types || typeCount !== value.total_events)) || (value.type_facets_truncated && types.length !== 50)) return undefined;
  const daily: CustomerActivityDayCount[] = []; let dailyCount = 0;
  for (const item of value.daily_counts) {
    if (!record(item) || !exact(item, DAY_KEYS) || !day(item.day) || !nonnegative(item.count) || item.count === 0 || (daily.at(-1)?.day ?? "") >= item.day) return undefined;
    daily.push({ day: item.day, count: item.count }); dailyCount += item.count;
  }
  if (daily.length !== value.active_days || dailyCount !== value.total_events) return undefined;
  return { customerID: expectedCustomerID, windowDays: expectedWindow, from: value.from, through: value.through, totalEvents: value.total_events, activeDays: value.active_days, uniqueEventTypes: value.unique_event_types, ...(typeof value.last_occurred_at === "string" ? { lastOccurredAt: value.last_occurred_at } : {}), typeFacets: types, typeFacetsTruncated: value.type_facets_truncated, dailyCounts: daily };
}

function failure(status: number): Exclude<CustomerActivityAnalyticsLoadResult, { readonly status: "loaded" }> { if (status === 401) return { status: "unauthenticated" }; if (status === 403) return { status: "forbidden" }; if (status === 404) return { status: "not_found" }; return { status: "unavailable" }; }
export async function loadCustomerActivityAnalytics(transport: CustomerActivityAnalyticsTransport, customerID: number, days: CustomerActivityWindowDays): Promise<CustomerActivityAnalyticsLoadResult> {
  if (!safePositiveID(customerID) || !windowDays(days)) return { status: "invalid" };
  try { const response = await transport.get(customerID, { window_days: days }, { credentials: "same-origin" }); if (response.status !== 200) return failure(response.status); const analytics = parseCustomerActivityAnalytics(response.data, customerID, days); return analytics ? { status: "loaded", analytics } : { status: "invalid" }; } catch { return { status: "unavailable" }; }
}
