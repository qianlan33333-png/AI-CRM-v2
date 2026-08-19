import { afterEach, describe, expect, it, vi } from "vitest";
import {
  filterChannels,
  loadChannelDetail,
  loadChannels,
  newChannelConfigurationIdempotencyKey,
  newChannelStatusIdempotencyKey,
  saveChannelConfiguration,
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
function detailChannel(extra: Record<string, unknown> = {}) {
  return {
    schema_version: 1, id: 1, channel_name: "公开课", channel_code: "course", status: "active",
    created_at: "2026-08-19T00:00:00Z", updated_at: "2026-08-19T01:02:03Z",
    assignees: [], assignment_stats_24h: [], assignee_count: 0, channel_contact_count: 0,
    latest_channel_entered_at: "", qrcode_asset_id: 0, qrcode_status: "not_generated",
    qr_download_url: "", share_url: "", copy_text: "",
    channel_type: "qrcode", carrier_type: "qrcode", scene_value: "", qr_url: "",
    owner_staff_id: "", customer_channel: "", link_url: "", final_url: "", welcome_message: "",
    welcome_image_library_ids: [], welcome_miniprogram_library_ids: [],
    welcome_attachment_library_ids: [], welcome_group_invite_library_ids: [],
    auto_accept_friend: false, entry_tag_id: "", entry_tag_name: "", entry_tag_group_name: "",
    assignment_mode: "single_owner", assignment_strategy: "ratio", overflow_policy: "least_loaded",
    assignment_config_json: {}, ...extra,
  };
}
function detailResponse(channel: unknown = detailChannel(), extra: Record<string, unknown> = {}) {
  return { ok: true, channel, reason: "channel_loaded", source: "ai_crm_next", ...extra };
}

afterEach(() => vi.unstubAllGlobals());

describe("local channel list read boundary", () => {
  it("reads one strict local detail through the same-origin generated GET only", async () => {
    const list = await loadChannels(transport(200, response));
    if (list.status !== "loaded") throw new Error("fixture must load");
    const client = {
      read: vi.fn(), write: vi.fn(),
      detail: vi.fn(async () => ({ status: 200, data: detailResponse(), headers: new Headers() })),
    } as unknown as ChannelsTransport;
    await expect(loadChannelDetail(client, list.items[0])).resolves.toMatchObject({
      status: "loaded", detail: { item: list.items[0], imageMaterialCount: 0, hasAssignmentConfig: true },
    });
    expect(client.detail).toHaveBeenCalledWith(1);
    expect(client.read).not.toHaveBeenCalled();
    expect(client.write).not.toHaveBeenCalled();
  });

  it("fails closed for extra, stale, unsafe fixed, and malformed local detail facts without retry", async () => {
    const list = await loadChannels(transport(200, response));
    if (list.status !== "loaded") throw new Error("fixture must load");
    const missingSchemaVersion = detailChannel();
    delete (missingSchemaVersion as Record<string, unknown>).schema_version;
    for (const data of [
      detailResponse(detailChannel({ unexpected: true })),
      detailResponse(missingSchemaVersion),
      detailResponse(detailChannel({ schema_version: 0 })),
      detailResponse(detailChannel({ schema_version: 2 })),
      detailResponse(detailChannel({ schema_version: "1" })),
      detailResponse(detailChannel({ channel_name: "不同渠道" })),
      detailResponse(detailChannel({ qrcode_status: "generated" })),
      detailResponse(detailChannel({ welcome_image_library_ids: [0] })),
      detailResponse(detailChannel({ assignment_config_json: [] })),
      detailResponse(detailChannel(), { source: "legacy" }),
    ]) {
      await expect(loadChannelDetail({
        read: vi.fn(), write: vi.fn(), detail: vi.fn(async () => ({ status: 200, data, headers: new Headers() })),
      } as unknown as ChannelsTransport, list.items[0])).resolves.toEqual({ status: "invalid" });
    }
    const unavailable = { read: vi.fn(), write: vi.fn(), detail: vi.fn(async () => { throw new Error("offline"); }) } as unknown as ChannelsTransport;
    await expect(loadChannelDetail(unavailable, list.items[0])).resolves.toEqual({ status: "unavailable" });
    expect(unavailable.detail).toHaveBeenCalledTimes(1);
  });
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
        data: { ok: true, channel: detailChannel({ status: "inactive" }), reason: "channel_updated", source: "ai_crm_next", fallback_used: false, real_external_call_executed: false },
        headers: new Headers(),
      })),
      detail: vi.fn(async () => ({ status: 200, data: detailResponse(detailChannel({ status: "inactive" })), headers: new Headers() })),
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
    expect(client.detail).toHaveBeenCalledWith(1);
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
            ok: true, channel: detailChannel({ status: "inactive" }), reason: "channel_updated",
            source: "ai_crm_next", fallback_used: false, real_external_call_executed: false,
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(refreshed), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(detailResponse(detailChannel({ status: "inactive" }))), { status: 200 }),
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
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      "/api/admin/channels/1",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
    expect(fetch).toHaveBeenCalledTimes(3);
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

  it("keeps definite status-write rejections distinct from an unknown outcome", async () => {
    const cases: readonly [number, unknown, "invalid" | "forbidden"][] = [
      [400, { ok: false, detail: "invalid channel status" }, "invalid"],
      [403, { code: "UNAUTHORIZED", message: "Permission is denied.", request_id: "request-403" }, "forbidden"],
      [404, { ok: false, detail: "channel not found" }, "invalid"],
      [409, { ok: false, detail: "channel status conflict" }, "invalid"],
    ];
    for (const [status, data, expected] of cases) {
      const client = {
        read: vi.fn(),
        detail: vi.fn(),
        create: vi.fn(),
        configure: vi.fn(),
        write: vi.fn(async () => ({ status, data, headers: new Headers() })),
      } as unknown as ChannelsTransport;
      await expect(
        updateChannelStatus(
          client,
          1,
          "inactive",
          "a".repeat(43),
          "channel-status:123e4567-e89b-42d3-a456-426614174000",
        ),
      ).resolves.toEqual({ status: expected });
      expect(client.write).toHaveBeenCalledOnce();
      expect(client.read).not.toHaveBeenCalled();
      expect(client.detail).not.toHaveBeenCalled();
    }
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

  it("creates one full local configuration through the generated POST and confirms its strict mutation DTO plus list/detail rereads", async () => {
    const input = {
      channelType: "qrcode" as const, carrierType: "qrcode" as const,
      channelName: "公开课", channelCode: "course", status: "active" as const,
      sceneValue: "", qrURL: "", ownerStaffID: "", customerChannel: "", linkURL: "", finalURL: "",
      welcomeMessage: "", imageMaterialIDs: [], miniProgramMaterialIDs: [], attachmentMaterialIDs: [], groupInviteMaterialIDs: [],
      autoAcceptFriend: false, entryTagID: "", entryTagName: "", entryTagGroupName: "",
      assignmentMode: "single_owner" as const, assignmentStrategy: "ratio" as const, overflowPolicy: "least_loaded",
    };
    const mutation = {
      ok: true, channel: detailChannel(), reason: "channel_created", source: "ai_crm_next",
      fallback_used: false, real_external_call_executed: false,
    } as const;
    const client = {
      create: vi.fn(async () => ({ status: 201, data: mutation, headers: new Headers() })),
      configure: vi.fn(),
      read: vi.fn(async () => ({ status: 200, data: response, headers: new Headers() })),
      detail: vi.fn(async () => ({ status: 200, data: detailResponse(), headers: new Headers() })),
      write: vi.fn(),
    } as unknown as ChannelsTransport;
    await expect(saveChannelConfiguration(client, "create", input, undefined, "a".repeat(43), "channel-create:123e4567-e89b-42d3-a456-426614174000"))
      .resolves.toMatchObject({ status: "confirmed", detail: { item: { id: 1, name: "公开课" } } });
    expect(client.create).toHaveBeenCalledWith(input, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": "a".repeat(43),
        "Idempotency-Key": "channel-create:123e4567-e89b-42d3-a456-426614174000",
      },
    });
    expect(client.read).toHaveBeenCalledTimes(1);
    expect(client.detail).toHaveBeenCalledWith(1);
    expect(client.configure).not.toHaveBeenCalled();
  });

  it("fails closed and marks the result unknown without reread for malformed or uncertain configuration mutations", async () => {
    const input = {
      channelType: "qrcode" as const, carrierType: "qrcode" as const,
      channelName: "公开课", channelCode: "course", status: "active" as const,
      sceneValue: "", qrURL: "", ownerStaffID: "", customerChannel: "", linkURL: "", finalURL: "",
      welcomeMessage: "", imageMaterialIDs: [], miniProgramMaterialIDs: [], attachmentMaterialIDs: [], groupInviteMaterialIDs: [],
      autoAcceptFriend: false, entryTagID: "", entryTagName: "", entryTagGroupName: "",
      assignmentMode: "single_owner" as const, assignmentStrategy: "ratio" as const, overflowPolicy: "least_loaded",
    };
    const client = {
      create: vi.fn(async () => ({ status: 201, data: { ok: true, channel: detailChannel(), reason: "channel_created", source: "ai_crm_next", fallback_used: false, real_external_call_executed: true }, headers: new Headers() })),
      configure: vi.fn(), read: vi.fn(), detail: vi.fn(), write: vi.fn(),
    } as unknown as ChannelsTransport;
    await expect(saveChannelConfiguration(client, "create", input, undefined, "a".repeat(43), "channel-create:123e4567-e89b-42d3-a456-426614174000"))
      .resolves.toEqual({ status: "unknown" });
    expect(client.read).not.toHaveBeenCalled();
    expect(client.detail).not.toHaveBeenCalled();

    expect(newChannelConfigurationIdempotencyKey("update", { randomUUID: () => "123e4567-e89b-42d3-a456-426614174000" }))
      .toBe("channel-update:123e4567-e89b-42d3-a456-426614174000");
  });

  it("uses the generated same-origin POST without replacing opaque assignment details", async () => {
    const input = {
      channelType: "qrcode" as const, carrierType: "qrcode" as const,
      channelName: "公开课", channelCode: "course", status: "active" as const,
      sceneValue: "", qrURL: "https://local.example/qr", ownerStaffID: "staff-1", customerChannel: "",
      linkURL: "https://local.example/link", finalURL: "", welcomeMessage: "欢迎",
      imageMaterialIDs: [7], miniProgramMaterialIDs: [], attachmentMaterialIDs: [], groupInviteMaterialIDs: [],
      autoAcceptFriend: false, entryTagID: "11", entryTagName: "新客", entryTagGroupName: "来源",
      assignmentMode: "single_owner" as const, assignmentStrategy: "ratio" as const, overflowPolicy: "least_loaded",
    };
    const channel = detailChannel({
      qr_url: input.qrURL, owner_staff_id: input.ownerStaffID, link_url: input.linkURL,
      welcome_message: input.welcomeMessage, welcome_image_library_ids: [7], entry_tag_id: "11",
      entry_tag_name: "新客", entry_tag_group_name: "来源", assignment_config_json: { retained: "server-owned" },
    });
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true, channel, reason: "channel_created", source: "ai_crm_next", fallback_used: false, real_external_call_executed: false }), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(response), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(detailResponse(channel)), { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    await expect(saveChannelConfiguration(
      (await import("./channels")).generatedChannelsTransport,
      "create", input, undefined, "a".repeat(43), "channel-create:123e4567-e89b-42d3-a456-426614174000",
    )).resolves.toMatchObject({ status: "confirmed" });
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/admin/channels", expect.objectContaining({
      credentials: "same-origin", method: "POST",
      headers: expect.objectContaining({ "X-CSRF-Token": "a".repeat(43) }),
      body: JSON.stringify({
        channel_type: "qrcode", carrier_type: "qrcode", channel_name: "公开课", channel_code: "course", status: "active",
        scene_value: "", qr_url: input.qrURL, owner_staff_id: "staff-1", customer_channel: "", link_url: input.linkURL,
        final_url: "", welcome_message: "欢迎", welcome_image_library_ids: [7], welcome_miniprogram_library_ids: [],
        welcome_attachment_library_ids: [], welcome_group_invite_library_ids: [], auto_accept_friend: false,
        entry_tag_id: "11", entry_tag_name: "新客", entry_tag_group_name: "来源", assignment_mode: "single_owner",
        assignment_strategy: "ratio", overflow_policy: "least_loaded",
      }),
    }));
  });
});
