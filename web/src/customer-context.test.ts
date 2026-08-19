import { describe, expect, it, vi } from "vitest";
import {
  CUSTOMER_CONTEXT_TIMELINE_PAGE_SIZE,
  isStrictRFC3339Timestamp,
  loadCustomerContext,
  loadCustomerContextTimelinePage,
  parseCustomerContext,
  type CustomerContextTransport,
  type CustomerContextTransportResponse,
} from "./customer-context";

const contextBody = {
  customer: {
    id: 7,
    name: "林小姐",
    stage_id: 3,
    owner_staff_id: 11,
    channel_id: 5,
    added_at: "2026-08-12T00:00:00Z",
    last_interact_at: "2026-08-12T01:00:00Z",
  },
  tags: [
    {
      id: 9,
      group_id: 2,
      group_name: "意向",
      group_sort_order: 1,
      name: "已报名",
      sort_order: 10,
    },
  ],
  timeline: [
    {
      id: 12,
      event_type: "stage_changed",
      occurred_at: "2026-08-12T03:00:00Z",
    },
  ],
  timeline_next_cursor: "opaque-next-page",
  chat: {
    local_archive_available: true,
    items: [
      {
        chat_type: "private",
        message_type: "text",
        sent_at: "2026-08-12T04:00:00Z",
      },
    ],
    total: 1,
  },
  non_atomic_snapshot: true,
  real_external_call_executed: false,
};

type Response = CustomerContextTransportResponse;

function transport(
  response: Response = { status: 200, data: contextBody },
): CustomerContextTransport {
  return {
    get: vi.fn(async () => response),
  } as unknown as CustomerContextTransport;
}

describe("Customer 360 local context parser", () => {
  it("maps only the closed local-safe projection", () => {
    expect(parseCustomerContext(contextBody, 7)).toEqual({
      customer: {
        id: 7,
        name: "林小姐",
        stageID: 3,
        ownerStaffID: 11,
        channelID: 5,
        addedAt: "2026-08-12T00:00:00Z",
        lastInteractAt: "2026-08-12T01:00:00Z",
      },
      tags: [
        {
          id: 9,
          groupName: "意向",
          groupSortOrder: 1,
          name: "已报名",
          sortOrder: 10,
        },
      ],
      timeline: [
        {
          id: 12,
          eventType: "stage_changed",
          occurredAt: "2026-08-12T03:00:00Z",
        },
      ],
      timelineNextCursor: "opaque-next-page",
      chat: {
        localArchiveAvailable: true,
        items: [
          {
            chatType: "private",
            messageType: "text",
            sentAt: "2026-08-12T04:00:00Z",
          },
        ],
        total: 1,
      },
    });
  });

  it.each([
    {
      ...contextBody,
      customer: {
        ...contextBody.customer,
        avatar_url: "https://forbidden.invalid",
      },
    },
    {
      ...contextBody,
      timeline: [{ ...contextBody.timeline[0], actor: "forbidden" }],
    },
    {
      ...contextBody,
      timeline: [{ ...contextBody.timeline[0], payload: { forbidden: true } }],
    },
    {
      ...contextBody,
      chat: {
        ...contextBody.chat,
        items: [{ ...contextBody.chat.items[0], content: "forbidden" }],
      },
    },
    { ...contextBody, provider_receipt: "forbidden" },
    { ...contextBody, non_atomic_snapshot: false },
    { ...contextBody, real_external_call_executed: true },
    {
      ...contextBody,
      chat: {
        local_archive_available: false,
        items: contextBody.chat.items,
        total: 1,
      },
    },
    { ...contextBody, timeline_next_cursor: "", timeline: [] },
    {
      ...contextBody,
      timeline: [
        { ...contextBody.timeline[0], occurred_at: "2026-02-30T03:00:00Z" },
      ],
    },
  ])("fails closed for expanded, unsafe, or invalid data %#", (value) => {
    expect(parseCustomerContext(value, 7)).toBeUndefined();
  });

  it.each([
    "2026-02-30T00:00:00Z",
    "2026-01-01T24:00:00Z",
    "2026-01-01T00:60:00Z",
    "2026-01-01",
    "2026-01-01T00:00:00+99:99",
  ])("rejects a non-RFC3339 calendar timestamp %s", (value) => {
    expect(isStrictRFC3339Timestamp(value)).toBe(false);
  });

  it("accepts a 200-rune supplementary-plane local label", () => {
    const messageType = "😀".repeat(200);
    expect(
      parseCustomerContext(
        {
          ...contextBody,
          chat: {
            ...contextBody.chat,
            items: [
              {
                ...contextBody.chat.items[0],
                message_type: messageType,
              },
            ],
          },
        },
        7,
      ),
    ).toMatchObject({ chat: { items: [{ messageType }] } });
  });
});

describe("Customer 360 local context transport", () => {
  it("uses same-origin GET, a fixed initial page size, and only server cursors", async () => {
    const client = transport();
    await expect(loadCustomerContext(client, 7)).resolves.toMatchObject({
      status: "loaded",
    });
    expect(client.get).toHaveBeenCalledWith(
      7,
      { limit: CUSTOMER_CONTEXT_TIMELINE_PAGE_SIZE },
      { credentials: "same-origin" },
    );
    await expect(
      loadCustomerContextTimelinePage(
        client,
        7,
        "opaque-next-page",
        new Set([12]),
      ),
    ).resolves.toEqual({ status: "unavailable" });

    const next = {
      ...contextBody,
      timeline: [
        {
          id: 11,
          event_type: "tag_added",
          occurred_at: "2026-08-12T02:00:00Z",
        },
      ],
      timeline_next_cursor: null,
    };
    const nextClient = transport({ status: 200, data: next });
    await expect(
      loadCustomerContextTimelinePage(
        nextClient,
        7,
        "opaque-next-page",
        new Set([12]),
      ),
    ).resolves.toEqual({
      status: "loaded",
      timeline: [
        { id: 11, eventType: "tag_added", occurredAt: "2026-08-12T02:00:00Z" },
      ],
    });
    expect(nextClient.get).toHaveBeenCalledWith(
      7,
      {
        cursor: "opaque-next-page",
        limit: CUSTOMER_CONTEXT_TIMELINE_PAGE_SIZE,
      },
      { credentials: "same-origin" },
    );
  });

  it.each([
    [401, "unauthenticated"],
    [403, "forbidden"],
    [404, "not_found"],
    [503, "unavailable"],
  ] as const)("maps %i without retry to %s", async (status, want) => {
    await expect(
      loadCustomerContext(transport({ status, data: {} }), 7),
    ).resolves.toEqual({ status: want });
  });
});
