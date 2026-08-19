import { afterEach, describe, expect, it, vi } from "vitest";
import {
  filterChannels,
  loadChannels,
  newChannelStatusIdempotencyKey,
  updateChannelStatus,
  type ChannelsTransport,
} from "./channels";

const response = {
  ok: true,
  channels: [
    {
      id: 1,
      channel_name: "公开课",
      channel_code: "course",
      status: "active",
      assignee_count: 0,
      channel_contact_count: 0,
      created_at: "2026-08-19T00:00:00Z",
      updated_at: "2026-08-19T01:02:03Z",
    },
    {
      id: 2,
      channel_name: "已归档",
      channel_code: "archive-1",
      status: "archived",
      assignee_count: 0,
      channel_contact_count: 0,
      created_at: "2026-08-18T00:00:00+08:00",
      updated_at: "2026-08-18T01:02:03+08:00",
    },
  ],
  reason: "channels_listed",
  source: "ai_crm_next",
} as const;

function transport(status: number, data: unknown): ChannelsTransport {
  return {
    read: vi.fn(async () => ({ status, data, headers: new Headers() })),
    write: vi.fn(),
  } as unknown as ChannelsTransport;
}

afterEach(() => vi.unstubAllGlobals());

describe("local channel list read boundary", () => {
  it("uses the existing same-origin Orval GET with the frozen complete local list parameters", async () => {
    const fetch = vi.fn(
      async () => new Response(JSON.stringify(response), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetch);

    await expect(loadChannels()).resolves.toMatchObject({ status: "loaded" });
    expect(fetch).toHaveBeenCalledWith(
      "/api/admin/channels?limit=300&include_archived=true",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
  });

  it("keeps only the frozen local fields and fixed zero-count statistics", async () => {
    const result = await loadChannels(transport(200, response));
    expect(result).toEqual({
      status: "loaded",
      items: [
        {
          id: 1,
          name: "公开课",
          code: "course",
          status: "active",
          assigneeCount: 0,
          contactCount: 0,
          createdAt: "2026-08-19T00:00:00Z",
          updatedAt: "2026-08-19T01:02:03Z",
        },
        {
          id: 2,
          name: "已归档",
          code: "archive-1",
          status: "archived",
          assigneeCount: 0,
          contactCount: 0,
          createdAt: "2026-08-18T00:00:00+08:00",
          updatedAt: "2026-08-18T01:02:03+08:00",
        },
      ],
    });
  });

  it("fails closed for malformed success and error envelopes", async () => {
    for (const data of [
      { ...response, unknown: true },
      { ...response, source: "legacy" },
      { ...response, channels: [{ ...response.channels[0], id: 0 }] },
      {
        ...response,
        channels: [{ ...response.channels[0], assignee_count: 1 }],
      },
      {
        ...response,
        channels: [
          {
            ...response.channels[0],
            welcome_message: "must never enter the view model",
          },
        ],
      },
      {
        ...response,
        channels: [
          { ...response.channels[0], created_at: "2026-02-31T00:00:00Z" },
        ],
      },
    ]) {
      await expect(loadChannels(transport(200, data))).resolves.toEqual({
        status: "invalid",
      });
    }

    await expect(
      loadChannels(
        transport(401, {
          code: "UNAUTHENTICATED",
          message: "Authentication is required.",
          request_id: "request-1",
        }),
      ),
    ).resolves.toEqual({ status: "unauthenticated" });
    await expect(
      loadChannels(
        transport(403, {
          code: "UNAUTHORIZED",
          message: "Permission is denied.",
          request_id: "request-2",
        }),
      ),
    ).resolves.toEqual({ status: "forbidden" });
    await expect(
      loadChannels(
        transport(503, { ok: false, detail: "channel unavailable" }),
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(loadChannels(transport(401, {}))).resolves.toEqual({
      status: "invalid",
    });

    const failing = transport(200, response);
    vi.mocked(failing.read).mockRejectedValue(new Error("network"));
    await expect(loadChannels(failing)).resolves.toEqual({
      status: "unavailable",
    });
  });

  it("filters only already loaded channel name or code and the exact frozen statuses", async () => {
    const result = await loadChannels(transport(200, response));
    if (result.status !== "loaded") throw new Error("fixture must load");
    expect(
      filterChannels(result.items, "公开", "all").map((item) => item.id),
    ).toEqual([1]);
    expect(
      filterChannels(result.items, "ARCHIVE", "all").map((item) => item.id),
    ).toEqual([2]);
    expect(
      filterChannels(result.items, "", "active").map((item) => item.id),
    ).toEqual([1]);
    expect(filterChannels(result.items, "", "inactive")).toEqual([]);
    expect(
      filterChannels(result.items, "", "archived").map((item) => item.id),
    ).toEqual([2]);
  });

  it("PATCHes only the requested status with same-origin CSRF and confirms only through the strict local list", async () => {
    const refreshed = {
      ...response,
      channels: [
        { ...response.channels[0], status: "inactive" },
        response.channels[1],
      ],
    };
    const client: ChannelsTransport = {
      read: vi.fn(async () => ({
        status: 200,
        data: refreshed,
        headers: new Headers(),
      })),
      write: vi.fn(async () => ({
        status: 200,
        // This must remain unread: success responses carry an unfrozen legacy projection.
        data: new Proxy(
          {},
          {
            get() {
              throw new Error("must not consume channel mutation projection");
            },
          },
        ),
        headers: new Headers(),
      })),
    } as unknown as ChannelsTransport;

    await expect(
      updateChannelStatus(
        client,
        1,
        "inactive",
        "a".repeat(43),
        "channel-status:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({
      status: "confirmed",
      items: [
        {
          id: 1,
          name: "公开课",
          code: "course",
          status: "inactive",
          assigneeCount: 0,
          contactCount: 0,
          createdAt: "2026-08-19T00:00:00Z",
          updatedAt: "2026-08-19T01:02:03Z",
        },
        {
          id: 2,
          name: "已归档",
          code: "archive-1",
          status: "archived",
          assigneeCount: 0,
          contactCount: 0,
          createdAt: "2026-08-18T00:00:00+08:00",
          updatedAt: "2026-08-18T01:02:03+08:00",
        },
      ],
    });
    expect(client.write).toHaveBeenCalledWith(1, "inactive", {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": "a".repeat(43),
        "Idempotency-Key":
          "channel-status:123e4567-e89b-42d3-a456-426614174000",
      },
    });
    expect(client.read).toHaveBeenCalledTimes(1);
  });

  it("uses only the generated PATCH and one existing safe-list GET", async () => {
    const refreshed = {
      ...response,
      channels: [
        { ...response.channels[0], status: "inactive" },
        response.channels[1],
      ],
    };
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            ok: true,
            channel: { qr_url: "https://must-not-be-used.example/qr" },
            reason: "channel_updated",
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(refreshed), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);

    await expect(
      updateChannelStatus(
        // Default transport is deliberately used here to pin the Orval request boundary.
        (await import("./channels")).generatedChannelsTransport,
        1,
        "inactive",
        "a".repeat(43),
        "channel-status:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toMatchObject({ status: "confirmed" });
    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/admin/channels/1",
      expect.objectContaining({
        credentials: "same-origin",
        method: "PATCH",
        body: JSON.stringify({ status: "inactive" }),
        headers: expect.objectContaining({
          "Content-Type": "application/json",
          "X-CSRF-Token": "a".repeat(43),
          "Idempotency-Key":
            "channel-status:123e4567-e89b-42d3-a456-426614174000",
        }),
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/admin/channels?limit=300&include_archived=true",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("does not claim an update when the safe list cannot confirm the exact target state", async () => {
    const client: ChannelsTransport = {
      read: vi.fn(async () => ({
        status: 200,
        data: response,
        headers: new Headers(),
      })),
      write: vi.fn(async () => ({
        status: 200,
        data: {},
        headers: new Headers(),
      })),
    } as unknown as ChannelsTransport;

    await expect(
      updateChannelStatus(
        client,
        1,
        "inactive",
        "a".repeat(43),
        "channel-status:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "unknown" });

    vi.mocked(client.read).mockRejectedValueOnce(new Error("list network"));
    await expect(
      updateChannelStatus(
        client,
        1,
        "inactive",
        "a".repeat(43),
        "channel-status:123e4567-e89b-42d3-a456-426614174001",
      ),
    ).resolves.toEqual({ status: "unknown" });
  });

  it("makes no retry after a transport failure and rejects malformed write inputs", async () => {
    const client: ChannelsTransport = {
      read: vi.fn(async () => ({
        status: 200,
        data: response,
        headers: new Headers(),
      })),
      write: vi.fn(async () => {
        throw new Error("mutation network");
      }),
    } as unknown as ChannelsTransport;

    await expect(
      updateChannelStatus(
        client,
        1,
        "inactive",
        "a".repeat(43),
        "channel-status:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "unknown" });
    expect(client.write).toHaveBeenCalledTimes(1);
    expect(client.read).not.toHaveBeenCalled();

    await expect(
      updateChannelStatus(client, 0, "inactive", "bad", "short"),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.write).toHaveBeenCalledTimes(1);
  });

  it("uses a unique valid channel status receipt key or stops before the request", () => {
    expect(
      newChannelStatusIdempotencyKey({
        randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
      }),
    ).toBe("channel-status:123e4567-e89b-42d3-a456-426614174000");
    expect(
      newChannelStatusIdempotencyKey({ randomUUID: () => "not-a-uuid" }),
    ).toBeUndefined();
  });
});
