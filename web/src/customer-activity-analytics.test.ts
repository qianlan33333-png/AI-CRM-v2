import { describe, expect, it, vi } from "vitest";
import { loadCustomerActivityAnalytics, parseCustomerActivityAnalytics, type CustomerActivityAnalyticsTransport } from "./customer-activity-analytics";

const analytics = { customer_id: 41, window_days: 30, from: "2026-07-21T10:00:00Z", through: "2026-08-20T10:00:00Z", total_events: 3, active_days: 2, unique_event_types: 2, last_occurred_at: "2026-08-20T09:00:00Z", type_facets: [{ event_type: "customer.updated", count: 2, last_occurred_at: "2026-08-20T09:00:00Z" }, { event_type: "customer.created", count: 1, last_occurred_at: "2026-08-19T09:00:00Z" }], type_facets_truncated: false, daily_counts: [{ day: "2026-08-19", count: 1 }, { day: "2026-08-20", count: 2 }], payload_included: false, actor_included: false, identity_included: false, real_external_call_executed: false };

describe("customer activity analytics contract", () => {
  it("parses only the closed payload-free projection", () => expect(parseCustomerActivityAnalytics(analytics, 41, 30)).toEqual({ customerID: 41, windowDays: 30, from: analytics.from, through: analytics.through, totalEvents: 3, activeDays: 2, uniqueEventTypes: 2, lastOccurredAt: analytics.last_occurred_at, typeFacets: [{ eventType: "customer.updated", count: 2, lastOccurredAt: "2026-08-20T09:00:00Z" }, { eventType: "customer.created", count: 1, lastOccurredAt: "2026-08-19T09:00:00Z" }], typeFacetsTruncated: false, dailyCounts: [{ day: "2026-08-19", count: 1 }, { day: "2026-08-20", count: 2 }] }));
  it.each([
    { ...analytics, payload: { answer: "secret" } }, { ...analytics, payload_included: true }, { ...analytics, actor_included: true }, { ...analytics, identity_included: true }, { ...analytics, real_external_call_executed: true },
    { ...analytics, customer_id: 42 }, { ...analytics, window_days: 7 }, { ...analytics, from: "2026-02-30T10:00:00Z" }, { ...analytics, last_occurred_at: null },
    { ...analytics, type_facets: [analytics.type_facets[1], analytics.type_facets[0]] }, { ...analytics, daily_counts: [analytics.daily_counts[1], analytics.daily_counts[0]] },
    { ...analytics, total_events: 4 }, { ...analytics, active_days: 3 }, { ...analytics, unique_event_types: 3 },
  ])("rejects malformed or leaking payload %#", (value) => expect(parseCustomerActivityAnalytics(value, 41, 30)).toBeUndefined());
  it("accepts a genuine empty local window", () => expect(parseCustomerActivityAnalytics({ ...analytics, total_events: 0, active_days: 0, unique_event_types: 0, last_occurred_at: null, type_facets: [], daily_counts: [] }, 41, 30)).toMatchObject({ totalEvents: 0, typeFacets: [] }));
  it("uses one same-origin GET and classifies failures", async () => {
    const get = vi.fn(async () => ({ status: 200, data: analytics })); const transport: CustomerActivityAnalyticsTransport = { get };
    await expect(loadCustomerActivityAnalytics(transport, 41, 30)).resolves.toMatchObject({ status: "loaded" });
    expect(get).toHaveBeenCalledWith(41, { window_days: 30 }, { credentials: "same-origin" }); expect(get).toHaveBeenCalledTimes(1);
    for (const [status, expected] of [[401, "unauthenticated"], [403, "forbidden"], [404, "not_found"], [503, "unavailable"]] as const) { await expect(loadCustomerActivityAnalytics({ get: async () => ({ status, data: {} }) }, 41, 30)).resolves.toEqual({ status: expected }); }
    await expect(loadCustomerActivityAnalytics({ get: async () => { throw new Error("network"); } }, 41, 30)).resolves.toEqual({ status: "unavailable" });
  });
});
