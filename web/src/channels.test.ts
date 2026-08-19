import { afterEach, describe, expect, it, vi } from "vitest";
import {
  filterChannels,
  loadChannels,
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
  } as unknown as ChannelsTransport;
}

afterEach(() => vi.unstubAllGlobals());

describe("local channel list read boundary", () => {
  it("uses the existing same-origin Orval GET with the frozen complete local list parameters", async () => {
    const fetch = vi.fn(async () =>
      new Response(JSON.stringify(response), { status: 200 }),
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
      { ...response, channels: [{ ...response.channels[0], assignee_count: 1 }] },
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
        channels: [{ ...response.channels[0], created_at: "2026-02-31T00:00:00Z" }],
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
      loadChannels(transport(503, { ok: false, detail: "channel unavailable" })),
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
    expect(filterChannels(result.items, "公开", "all").map((item) => item.id)).toEqual([
      1,
    ]);
    expect(filterChannels(result.items, "ARCHIVE", "all").map((item) => item.id)).toEqual([
      2,
    ]);
    expect(filterChannels(result.items, "", "active").map((item) => item.id)).toEqual([
      1,
    ]);
    expect(filterChannels(result.items, "", "inactive")).toEqual([]);
    expect(filterChannels(result.items, "", "archived").map((item) => item.id)).toEqual([
      2,
    ]);
  });
});
