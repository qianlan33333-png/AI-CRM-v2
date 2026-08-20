import { describe, expect, it, vi } from "vitest";
import {
  CUSTOMER_MERGE_HISTORY_PAGE_SIZE,
  generatedCustomerMergeHistoryTransport,
  loadCustomerMergeHistory,
  parseCustomerMergeHistoryPage,
  type CustomerMergeHistoryTransport,
} from "./customer-merge-history";

function item(id: number) {
  return {
    merge_audit_id: id,
    primary_customer_id: 41,
    merged_customer_id: 100 + id,
    mode: "auto",
    policy_version: "verified_unionid_unique_wecom_v1",
    merged_at: `2026-08-20T12:${String(id % 60).padStart(2, "0")}:00Z`,
  };
}

function page(items: unknown[] = [item(9)], nextCursor: string | null = null) {
  return {
    customer_id: 41,
    scope: "connected_component",
    items,
    next_cursor: nextCursor,
    identity_values_included: false,
    operator_identifiers_included: false,
    chat_content_included: false,
    real_external_call_executed: false,
  };
}

describe("customer merge history decoder", () => {
  it("uses the generated same-origin customer merge-history endpoint", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify(page()), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    try {
      await generatedCustomerMergeHistoryTransport.get(
        41,
        { cursor: "opaque", limit: 50 },
        { credentials: "same-origin" },
      );
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/customers/41/merge-history?cursor=opaque&limit=50",
        { credentials: "same-origin", method: "GET" },
      );
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it("accepts the closed redacted page and rejects unsafe drift", () => {
    expect(parseCustomerMergeHistoryPage(page(), 41)?.items[0]).toEqual({
      mergeAuditID: 9,
      primaryCustomerID: 41,
      mergedCustomerID: 109,
      mode: "auto",
      policyVersion: "verified_unionid_unique_wecom_v1",
      mergedAt: "2026-08-20T12:09:00Z",
    });
    const invalid = [
      { ...page(), identity_values_included: true },
      { ...page(), operator_identifiers_included: true },
      { ...page(), chat_content_included: true },
      { ...page(), real_external_call_executed: true },
      { ...page(), customer_id: 42 },
      { ...page(), extra: "identity" },
      page([{ ...item(9), operated_by: "admin:7" }]),
      page([{ ...item(9), primary_customer_id: 109 }]),
      page([item(8), item(9)]),
      page([item(9), item(9)]),
      page([{ ...item(9), mode: "provider" }]),
      page([{ ...item(9), policy_version: " raw " }]),
      page([{ ...item(9), merged_at: "2026-02-30T12:00:00Z" }]),
      page([item(9)], "cursor-with-short-page"),
    ];
    for (const value of invalid)
      expect(parseCustomerMergeHistoryPage(value, 41)).toBeUndefined();
  });

  it("accepts only a full page when a next cursor is present", () => {
    const items = Array.from(
      { length: CUSTOMER_MERGE_HISTORY_PAGE_SIZE },
      (_, index) => item(100 - index),
    );
    expect(
      parseCustomerMergeHistoryPage(page(items, "next"), 41)?.nextCursor,
    ).toBe("next");
  });

  it("uses one same-origin GET and maps failures without retries", async () => {
    const get = vi.fn<CustomerMergeHistoryTransport["get"]>(async () => ({
      status: 200,
      data: page(),
    }));
    const transport: CustomerMergeHistoryTransport = { get };
    await expect(
      loadCustomerMergeHistory(transport, 41),
    ).resolves.toMatchObject({ status: "loaded" });
    expect(get).toHaveBeenCalledTimes(1);
    expect(get).toHaveBeenCalledWith(
      41,
      { limit: CUSTOMER_MERGE_HISTORY_PAGE_SIZE },
      { credentials: "same-origin" },
    );

    for (const [status, expected] of [
      [401, "unauthenticated"],
      [403, "forbidden"],
      [400, "invalid"],
      [503, "unavailable"],
    ] as const) {
      get.mockResolvedValueOnce({ status, data: {} });
      await expect(
        loadCustomerMergeHistory(transport, 41, "cursor"),
      ).resolves.toEqual({ status: expected });
    }
    expect(get).toHaveBeenCalledTimes(5);
  });
});
