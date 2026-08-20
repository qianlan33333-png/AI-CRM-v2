import { describe, expect, it, vi } from "vitest";
import {
  CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE,
  loadCustomerChatActivity,
  parseCustomerChatActivityPage,
  type CustomerChatActivityFilter,
} from "./customer-chat-activity";

function item(index = 0, chatType: "private" | "group" = "private") {
  return {
    chat_type: chatType,
    message_type: index % 2 === 0 ? "text" : "image",
    sent_at: `2026-08-20T${String(12 - Math.floor(index / 60)).padStart(2, "0")}:${String(59 - (index % 60)).padStart(2, "0")}:00Z`,
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
  it("accepts a strict zero-body local page and drops no hidden fields", () => {
    expect(parseCustomerChatActivityPage(page(), 41, "all")).toEqual({
      customerID: 41,
      chatType: "all",
      items: [
        {
          chatType: "private",
          messageType: "text",
          sentAt: "2026-08-20T12:59:00Z",
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

  it("requires a full page before accepting a next cursor", () => {
    const short = { ...page(), next_cursor: "next" };
    expect(parseCustomerChatActivityPage(short, 41, "all")).toBeUndefined();
    const items = Array.from(
      { length: CUSTOMER_CHAT_ACTIVITY_PAGE_SIZE },
      (_, index) => item(index),
    );
    const full = { ...page("all", items), total: 51, next_cursor: "next" };
    expect(parseCustomerChatActivityPage(full, 41, "all")?.nextCursor).toBe(
      "next",
    );
  });
});

describe("loadCustomerChatActivity", () => {
  it("uses one same-origin GET with a fixed limit and explicit safe filter", async () => {
    const get = vi.fn(async () => ({ status: 200, data: page("private") }));
    await expect(
      loadCustomerChatActivity({ get }, 41, "private", "cursor"),
    ).resolves.toMatchObject({ status: "loaded" });
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
