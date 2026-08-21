import { describe, expect, it, vi } from "vitest";
import {
  CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
  CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE,
  CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
  canLoadNextCustomerChatActivityPage,
  generatedCustomerChatActivityTransport,
  loadCustomerChatActivity,
  parseCustomerChatActivityPage,
  type CustomerChatActivityFilter,
} from "./customer-chat-activity";

function item(index = 0, chatType: "private" | "group" = "private") {
  return {
    chat_type: chatType,
    message_type: index % 2 === 0 ? "text" : "image",
    sent_at: new Date(
      Date.UTC(2026, 7, 20, 12, 59, 0) - index * 60_000,
    ).toISOString(),
  };
}

function page(
  filter: CustomerChatActivityFilter = "all",
  items: unknown[] = [item()],
) {
  return {
    customer_id: 41,
    chat_type: filter,
    items,
    total: items.length,
    next_cursor: null,
    previous_cursor: null,
    non_atomic_snapshot: true,
    message_content_included: false,
    identity_values_included: false,
    provider_receipts_included: false,
    real_external_call_executed: false,
  };
}

describe("parseCustomerChatActivityPage", () => {
  it("accepts a strict zero-body local summary and carries only the request-bound limit", () => {
    expect(parseCustomerChatActivityPage(page(), 41, "all")).toEqual({
      customerID: 41,
      chatType: "all",
      limit: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
      offset: 0,
      items: [
        {
          chatType: "private",
          messageType: "text",
          sentAt: "2026-08-20T12:59:00.000Z",
        },
      ],
      total: 1,
    });
  });

  it("accepts a valid RFC3339 offset without normalizing the source value", () => {
    const value = page("all", [
      { ...item(), sent_at: "2026-08-20T20:59:00+08:00" },
    ]);
    expect(
      parseCustomerChatActivityPage(value, 41, "all")?.items[0]?.sentAt,
    ).toBe("2026-08-20T20:59:00+08:00");
  });

  it.each([
    ["extra page key", { ...page(), content_masked: "secret" }],
    ["wrong customer", { ...page(), customer_id: 42 }],
    ["content flag", { ...page(), message_content_included: true }],
    ["identity flag", { ...page(), identity_values_included: true }],
    ["provider flag", { ...page(), provider_receipts_included: true }],
    ["external flag", { ...page(), real_external_call_executed: true }],
    ["atomic claim", { ...page(), non_atomic_snapshot: false }],
    [
      "invalid calendar",
      page("all", [{ ...item(), sent_at: "2026-02-30T12:00:00Z" }]),
    ],
    ["ascending page", page("all", [item(1), item(0)])],
    ["private filter drift", page("private", [item(0, "group")])],
    ["extra item key", page("all", [{ ...item(), sender: "hidden" }])],
    [
      "unsafe message type",
      page("all", [{ ...item(), message_type: " text " }]),
    ],
  ])("rejects %s", (_name, value) => {
    const filter = value.chat_type === "private" ? "private" : "all";
    expect(parseCustomerChatActivityPage(value, 41, filter)).toBeUndefined();
  });

  it("rejects a response filter that differs from the request", () => {
    expect(
      parseCustomerChatActivityPage(
        { ...page(), chat_type: "private" },
        41,
        "all",
      ),
    ).toBeUndefined();
  });

  it("binds next-cursor validation to the exact requested summary or expanded limit", () => {
    const short = { ...page(), next_cursor: "next" };
    expect(parseCustomerChatActivityPage(short, 41, "all")).toBeUndefined();

    const summaryItems = Array.from(
      { length: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT },
      (_, index) => item(index),
    );
    const summary = {
      ...page("all", summaryItems),
      total: 101,
      next_cursor: "summary-next",
    };
    expect(
      parseCustomerChatActivityPage(summary, 41, "all")?.nextCursor,
    ).toBe("summary-next");
    expect(
      parseCustomerChatActivityPage(
        summary,
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
      ),
    ).toBeUndefined();

    const expandedItems = Array.from(
      { length: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT },
      (_, index) => item(index),
    );
    const expanded = {
      ...page("all", expandedItems),
      total: 101,
      next_cursor: "ignored-after-cap",
    };
    expect(
      parseCustomerChatActivityPage(
        expanded,
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
      ),
    ).toMatchObject({
      limit: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
      nextCursor: "ignored-after-cap",
      items: { length: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT },
    });
  });

  it("binds cursor shape and totals to the caller-owned offset", () => {
    const items = Array.from(
      { length: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT },
      (_, index) => item(index),
    );
    const secondPage = {
      ...page("all", items),
      total: 91,
      next_cursor: "next-60",
      previous_cursor: "previous-0",
    };

    expect(
      parseCustomerChatActivityPage(
        secondPage,
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
        30,
      ),
    ).toMatchObject({
      limit: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
      offset: 30,
      nextCursor: "next-60",
      previousCursor: "previous-0",
    });
    expect(
      parseCustomerChatActivityPage(
        secondPage,
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
        0,
      ),
    ).toBeUndefined();
    expect(
      parseCustomerChatActivityPage(
        { ...secondPage, previous_cursor: null },
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
        30,
      ),
    ).toBeUndefined();
    expect(
      parseCustomerChatActivityPage(
        { ...secondPage, total: 59 },
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
        30,
      ),
    ).toBeUndefined();
  });

  it("rejects more than 100 rows and invalid request limits", () => {
    const oversized = Array.from(
      { length: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT + 1 },
      (_, index) => item(index),
    );
    expect(
      parseCustomerChatActivityPage(
        page("all", oversized),
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
      ),
    ).toBeUndefined();
    expect(parseCustomerChatActivityPage(page(), 41, "all", 0)).toBeUndefined();
    expect(
      parseCustomerChatActivityPage(
        page(),
        41,
        "all",
        CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT + 1,
      ),
    ).toBeUndefined();
  });
});

describe("customer chat activity read boundary", () => {
  const boundedPage = (offset: number) => ({
    customerID: 41,
    chatType: "all" as const,
    limit: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
    offset,
    items: [],
    total: 130,
    nextCursor: `cursor-${offset + CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT}`,
  });

  it("allows only complete 30-row pages that remain inside the first 100 rows", () => {
    expect(canLoadNextCustomerChatActivityPage(boundedPage(0))).toBe(true);
    expect(canLoadNextCustomerChatActivityPage(boundedPage(30))).toBe(true);
    expect(canLoadNextCustomerChatActivityPage(boundedPage(60))).toBe(false);
    expect(
      canLoadNextCustomerChatActivityPage({
        ...boundedPage(0),
        limit: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
      }),
    ).toBe(false);
    expect(
      canLoadNextCustomerChatActivityPage({
        ...boundedPage(0),
        nextCursor: undefined,
      }),
    ).toBe(false);
  });
});

describe("loadCustomerChatActivity", () => {
  it("uses the generated same-origin customer chat-activity endpoint", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify(page("private")), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    try {
      await generatedCustomerChatActivityTransport.get(
        41,
        { chat_type: "private", cursor: "opaque", limit: 50 },
        { credentials: "same-origin" },
      );
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/customers/41/chat-activity?chat_type=private&cursor=opaque&limit=50",
        { credentials: "same-origin", method: "GET" },
      );
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("uses one same-origin GET with the 30-row safe summary by default", async () => {
    const get = vi.fn(async () => ({ status: 200, data: page("private") }));
    await expect(
      loadCustomerChatActivity({ get }, 41, "private", "cursor"),
    ).resolves.toMatchObject({
      status: "loaded",
      page: { limit: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT },
    });
    expect(get).toHaveBeenCalledTimes(1);
    expect(get).toHaveBeenCalledWith(
      41,
      {
        chat_type: "private",
        cursor: "cursor",
        limit: CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE,
      },
      { credentials: "same-origin" },
    );
  });

  it("allows one explicit 100-row local metadata expansion", async () => {
    const items = Array.from(
      { length: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT },
      (_, index) => item(index),
    );
    const get = vi.fn(async () => ({
      status: 200,
      data: page("all", items),
    }));
    await expect(
      loadCustomerChatActivity(
        { get },
        41,
        "all",
        undefined,
        CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
      ),
    ).resolves.toMatchObject({
      status: "loaded",
      page: {
        limit: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT,
        items: { length: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT },
      },
    });
    expect(get).toHaveBeenCalledWith(
      41,
      { limit: CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT },
      { credentials: "same-origin" },
    );
  });

  it("carries a validated offset alongside an opaque cursor without adding it to the wire query", async () => {
    const items = Array.from(
      { length: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT },
      (_, index) => item(index),
    );
    const get = vi.fn(async () => ({
      status: 200,
      data: {
        ...page("all", items),
        total: 91,
        next_cursor: "next-60",
        previous_cursor: "previous-0",
      },
    }));

    await expect(
      loadCustomerChatActivity(
        { get },
        41,
        "all",
        "cursor-30",
        CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
        30,
      ),
    ).resolves.toMatchObject({
      status: "loaded",
      page: { offset: 30, limit: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT },
    });
    expect(get).toHaveBeenCalledWith(
      41,
      { cursor: "cursor-30", limit: CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT },
      { credentials: "same-origin" },
    );
  });

  it("rejects a positive offset without its server-issued cursor", async () => {
    const get = vi.fn();
    await expect(
      loadCustomerChatActivity(
        { get },
        41,
        "all",
        undefined,
        CUSTOMER_CHAT_ACTIVITY_SUMMARY_LIMIT,
        30,
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(get).not.toHaveBeenCalled();
  });

  it("rejects an invalid limit before transport", async () => {
    const get = vi.fn();
    await expect(
      loadCustomerChatActivity(
        { get },
        41,
        "all",
        undefined,
        CUSTOMER_CHAT_ACTIVITY_MAXIMUM_LIMIT + 1,
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(get).not.toHaveBeenCalled();
  });

  it.each([
    [401, "unauthenticated"],
    [403, "forbidden"],
    [404, "not_found"],
    [400, "invalid"],
    [503, "unavailable"],
  ] as const)("maps %d without retry", async (status, expected) => {
    const get = vi.fn(async () => ({ status, data: {} }));
    await expect(loadCustomerChatActivity({ get }, 41, "all")).resolves.toEqual(
      { status: expected },
    );
    expect(get).toHaveBeenCalledTimes(1);
  });

  it("treats network and malformed 200 as unavailable without retry", async () => {
    const network = vi.fn(async () => {
      throw new Error("private transport detail");
    });
    await expect(
      loadCustomerChatActivity({ get: network }, 41, "all"),
    ).resolves.toEqual({ status: "unavailable" });
    expect(network).toHaveBeenCalledTimes(1);

    const malformed = vi.fn(async () => ({
      status: 200,
      data: { ...page(), content_masked: "must never be accepted" },
    }));
    await expect(
      loadCustomerChatActivity({ get: malformed }, 41, "all"),
    ).resolves.toEqual({ status: "unavailable" });
    expect(malformed).toHaveBeenCalledTimes(1);
  });
});
