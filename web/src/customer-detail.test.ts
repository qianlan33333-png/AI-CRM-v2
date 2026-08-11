import { describe, expect, it, vi } from "vitest";
import {
  loadCustomerDetail,
  parseCustomerDetailResponse,
  parseTagCatalog,
  submitCustomerProfileUpdate,
  submitCustomerStageChange,
  submitCustomerTagAdd,
  submitCustomerTagRemoval,
  type CustomerDetailTransport,
  type CustomerDetailTransportResponse,
} from "./customer-detail";

const rawCustomer = {
  id: 7,
  name: "林小姐",
  avatar_url: "https://assets.invalid/avatar.png",
  gender: 1,
  stage_id: 3,
  owner_staff_id: 11,
  channel_id: 5,
  added_at: "2026-08-12T00:00:00Z",
  last_interact_at: "2026-08-12T01:00:00Z",
  is_deleted: false,
  extra: { source: "campaign" },
  created_at: "2026-08-11T00:00:00Z",
  updated_at: "2026-08-12T02:00:00Z",
};

const rawTag = {
  id: 9,
  group_id: 2,
  group_name: "意向",
  name: "已报名",
  sort_order: 10,
};

const rawEvent = {
  id: 12,
  customer_id: 7,
  event_type: "stage_changed",
  payload: { from: null, to: 3 },
  actor: "后台账号 #1",
  occurred_at: "2026-08-12T03:00:00Z",
};

type Response = CustomerDetailTransportResponse;

const detailResponse: Response = {
  status: 200,
  data: { customer: rawCustomer, tags: [rawTag] },
};
const eventsResponse: Response = {
  status: 200,
  data: { items: [rawEvent], next_cursor: "next-page" },
};
const tagsResponse: Response = { status: 200, data: { items: [rawTag] } };

function client(
  responses: Partial<{
    get: Response;
    update: Response;
    setStage: Response;
    addTag: Response;
    removeTag: Response;
    events: Response;
    tags: Response;
  }> = {},
): CustomerDetailTransport {
  return {
    get: vi.fn(async () => responses.get ?? detailResponse),
    update: vi.fn(async () => responses.update ?? { status: 200, data: rawCustomer }),
    setStage: vi.fn(async () => responses.setStage ?? { status: 200, data: rawCustomer }),
    addTag: vi.fn(async () => responses.addTag ?? { status: 204, data: {} }),
    removeTag: vi.fn(async () => responses.removeTag ?? { status: 204, data: {} }),
    listEvents: vi.fn(async () => responses.events ?? eventsResponse),
    listTags: vi.fn(async () => responses.tags ?? tagsResponse),
  };
}

describe("customer detail response parsing", () => {
  it("maps only the safe, channel-neutral detail projection", () => {
    expect(parseCustomerDetailResponse(detailResponse.data, 7)).toEqual({
      customer: {
        id: 7,
        name: "林小姐",
        avatarURL: "https://assets.invalid/avatar.png",
        gender: 1,
        stageID: 3,
        ownerStaffID: 11,
        channelID: 5,
        addedAt: "2026-08-12T00:00:00Z",
        lastInteractAt: "2026-08-12T01:00:00Z",
        isDeleted: false,
        createdAt: "2026-08-11T00:00:00Z",
        updatedAt: "2026-08-12T02:00:00Z",
      },
      tags: [
        { id: 9, groupName: "意向", name: "已报名", sortOrder: 10 },
      ],
    });
  });

  it.each([
    null,
    {},
    { customer: rawCustomer, tags: [], extra: true },
    { customer: { ...rawCustomer, id: 8 }, tags: [] },
    { customer: { ...rawCustomer, updated_at: "not-a-time" }, tags: [] },
    { customer: { ...rawCustomer, created_at: "2026-08-11" }, tags: [] },
    { customer: { ...rawCustomer, avatar_url: "javascript:alert(1)" }, tags: [] },
    { customer: { ...rawCustomer, avatar_url: "data:text/plain,unsafe" }, tags: [] },
    { customer: { ...rawCustomer, avatar_url: "ftp://assets.invalid/a" }, tags: [] },
    { customer: { ...rawCustomer, avatar_url: "https:assets.invalid/a" }, tags: [] },
    { customer: { ...rawCustomer, avatar_url: "https://name:secret@assets.invalid/a" }, tags: [] },
    { customer: { ...rawCustomer, gender: 32_768 }, tags: [] },
    { customer: { ...rawCustomer, gender: -32_769 }, tags: [] },
    {
      customer: {
        ...rawCustomer,
        created_at: "2026-08-13T00:00:00Z",
        updated_at: "2026-08-12T00:00:00Z",
      },
      tags: [],
    },
    { customer: rawCustomer, tags: [{ ...rawTag, id: 0 }] },
    { customer: rawCustomer, tags: [rawTag, rawTag] },
  ])("rejects malformed or mismatched detail %#", (value) => {
    expect(parseCustomerDetailResponse(value, 7)).toBeUndefined();
  });

  it("accepts null or omitted optional avatar data", () => {
    expect(
      parseCustomerDetailResponse(
        { customer: { ...rawCustomer, avatar_url: null }, tags: [] },
        7,
      ),
    ).toMatchObject({ customer: { id: 7 } });
    const { avatar_url: ignored, ...withoutAvatar } = rawCustomer;
    expect(
      parseCustomerDetailResponse({ customer: withoutAvatar, tags: [] }, 7),
    ).toMatchObject({ customer: { id: 7 } });
    expect(ignored).toBeTypeOf("string");
  });

  it.each([
    null,
    {},
    { items: "nope" },
    { items: [rawTag], extra: true },
    { items: [{ ...rawTag, group_name: 7 }] },
  ])("rejects an expanded or malformed tag catalog %#", (value) => {
    expect(parseTagCatalog(value)).toBeUndefined();
  });
});

describe("customer detail loading", () => {
  it("loads the detail, first timeline page and catalog with same-origin credentials", async () => {
    const transport = client();
    await expect(loadCustomerDetail(transport, 7)).resolves.toMatchObject({
      status: "loaded",
      snapshot: {
        customer: { id: 7, name: "林小姐" },
        events: [{ id: 12, actor: "后台账号 #1" }],
        eventsHaveMore: true,
      },
    });
    const result = await loadCustomerDetail(transport, 7);
    expect(result).toMatchObject({ status: "loaded" });
    if (result.status === "loaded") {
      expect(result.snapshot).not.toHaveProperty("nextEventCursor");
    }
    expect(transport.get).toHaveBeenCalledWith(7, {
      credentials: "same-origin",
    });
    expect(transport.listEvents).toHaveBeenCalledWith(
      7,
      { limit: 50 },
      { credentials: "same-origin" },
    );
    expect(transport.listTags).toHaveBeenCalledWith({
      credentials: "same-origin",
    });
    await expect(
      loadCustomerDetail(
        client({
          events: { status: 200, data: { items: [rawEvent], next_cursor: null } },
        }),
        7,
      ),
    ).resolves.toMatchObject({
      status: "loaded",
      snapshot: { eventsHaveMore: false },
    });
  });

  it.each([
    ["get", 401, "unauthenticated"],
    ["events", 404, "not_found"],
    ["tags", 403, "forbidden"],
    ["get", 503, "unavailable"],
  ] as const)("classifies %s status %i as %s", async (target, status, want) => {
    const transport = client({ [target]: { status, data: {} } });
    await expect(loadCustomerDetail(transport, 7)).resolves.toEqual({ status: want });
  });

  it("fails closed for a rejected request, mismatched event, or invalid id", async () => {
    const rejected = client();
    vi.mocked(rejected.get).mockRejectedValue(new Error("network details"));
    await expect(loadCustomerDetail(rejected, 7)).resolves.toEqual({
      status: "unavailable",
    });

    const mismatched = client({
      events: {
        status: 200,
        data: { items: [{ ...rawEvent, customer_id: 8 }], next_cursor: null },
      },
    });
    await expect(loadCustomerDetail(mismatched, 7)).resolves.toEqual({
      status: "unavailable",
    });
    await expect(loadCustomerDetail(client(), 0)).resolves.toEqual({
      status: "unavailable",
    });
  });

});

describe("customer detail mutations", () => {
  const update = {
    name: "新名称",
    avatarURL: null,
    gender: null,
    ownerStaffID: 11,
    channelID: null,
  };
  const csrf = "A".repeat(43);

  it("uses exact CSRF and same-origin request options for every write", async () => {
    const transport = client();
    await expect(
      submitCustomerProfileUpdate(transport, 7, update, csrf),
    ).resolves.toEqual({ status: "succeeded" });
    expect(transport.update).toHaveBeenCalledWith(
      7,
      {
        name: "新名称",
        avatar_url: null,
        gender: null,
        owner_staff_id: 11,
        channel_id: null,
      },
      {
        credentials: "same-origin",
        headers: { "X-CSRF-Token": csrf },
      },
    );

    await expect(
      submitCustomerStageChange(transport, 7, 3, csrf),
    ).resolves.toEqual({ status: "succeeded" });
    await expect(submitCustomerTagAdd(transport, 7, 9, csrf)).resolves.toEqual({
      status: "succeeded",
    });
    await expect(
      submitCustomerTagRemoval(transport, 7, 9, csrf),
    ).resolves.toEqual({ status: "succeeded" });
    expect(transport.setStage).toHaveBeenCalledWith(
      7,
      { stage_id: 3 },
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(transport.addTag).toHaveBeenCalledWith(
      7,
      9,
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(transport.removeTag).toHaveBeenCalledWith(
      7,
      9,
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it.each([
    [401, "unauthenticated"],
    [403, "forbidden"],
    [404, "not_found"],
    [409, "conflict"],
    [422, "invalid"],
    [503, "unavailable"],
  ] as const)("classifies write status %i as %s", async (status, want) => {
    await expect(
      submitCustomerProfileUpdate(client({ update: { status, data: {} } }), 7, update, csrf),
    ).resolves.toEqual({ status: want });
  });

  it("never sends a malformed token, id, or unsafe profile", async () => {
    const transport = client();
    await expect(
      submitCustomerProfileUpdate(transport, 0, update, csrf),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      submitCustomerProfileUpdate(transport, 7, { ...update, name: "  " }, csrf),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      submitCustomerStageChange(transport, 7, 3, "short"),
    ).resolves.toEqual({ status: "invalid" });
    await expect(submitCustomerTagAdd(transport, 7, 0, csrf)).resolves.toEqual({
      status: "invalid",
    });
    expect(transport.update).not.toHaveBeenCalled();
    expect(transport.setStage).not.toHaveBeenCalled();
    expect(transport.addTag).not.toHaveBeenCalled();

    for (const invalidUpdate of [
      { ...update, avatarURL: "javascript:alert(1)" },
      { ...update, avatarURL: "data:text/plain,unsafe" },
      { ...update, avatarURL: "ftp://assets.invalid/a" },
      { ...update, avatarURL: "https:assets.invalid/a" },
      { ...update, avatarURL: "https://name:secret@assets.invalid/a" },
      { ...update, gender: 32_768 },
      { ...update, gender: -32_769 },
      { ...update, ownerStaffID: 0 },
      { ...update, channelID: Number.POSITIVE_INFINITY },
    ]) {
      const candidate = client();
      await expect(
        submitCustomerProfileUpdate(
          candidate,
          7,
          invalidUpdate,
          csrf,
        ),
      ).resolves.toEqual({ status: "invalid" });
      expect(candidate.update).not.toHaveBeenCalled();
    }
  });
});
