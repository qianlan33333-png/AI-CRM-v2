import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CustomerActivityAnalyticsPanel, CustomerActivityAnalyticsView } from "./customer-activity-analytics-ui";

const value = { customerID: 41, windowDays: 30 as const, from: "2026-07-21T10:00:00Z", through: "2026-08-20T10:00:00Z", totalEvents: 3, activeDays: 2, uniqueEventTypes: 1, lastOccurredAt: "2026-08-20T09:00:00Z", typeFacets: [{ eventType: "customer.updated", count: 3, lastOccurredAt: "2026-08-20T09:00:00Z" }], typeFacetsTruncated: false, dailyCounts: [{ day: "2026-08-19", count: 1 }, { day: "2026-08-20", count: 2 }] };

describe("CustomerActivityAnalyticsView", () => {
  it("renders only local safe aggregates", () => { const html = renderToStaticMarkup(<CustomerActivityAnalyticsView state={{ kind: "ready", analytics: value }} selectedDays={30} onSelect={vi.fn()} />); expect(html).toContain("客户本地活动统计"); expect(html).toContain("customer.updated"); expect(html).toContain("2026-08-20：2"); expect(html).not.toMatch(/payload|actor|identity|provider|外部调用状态：/i); });
  it("retains a verified result during a local read failure", () => { const html = renderToStaticMarkup(<CustomerActivityAnalyticsView state={{ kind: "error", failure: "unavailable", previous: value }} selectedDays={90} onSelect={vi.fn()} />); expect(html).toContain("已保留上次结果"); expect(html).toContain("customer.updated"); });
  it("hides safely until Lane E injects a real generated transport", () => expect(renderToStaticMarkup(<CustomerActivityAnalyticsPanel customerID={41} />)).toBe(""));
});
