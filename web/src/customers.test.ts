import { describe, expect, it, vi } from "vitest";
import {
  appendCustomerListHistoryPage,
  appendCustomerListPage,
  customerListParams,
  loadCustomers,
  parseCustomer,
  parseCustomerListFilters,
  parseCustomerListPage,
  type CustomerListFilterDraft,
  type CustomerListFilters,
  type CustomerTransport,
} from "./customers";

const rawCustomer = {
  id: 7,
  name: "陈晨",
  avatar_url: null,
  gender: 1,
  stage_id: 3,
  owner_staff_id: 8,
  channel_id: 5,
  added_at: "2026-08-12T08:00:00Z",
  last_interact_at: "2026-08-12T09:00:00Z",
  is_deleted: false,
  extra: { city: "上海" },
  created_at: "2026-08-12T07:00:00Z",
  updated_at: "2026-08-12T10:00:00Z",
};

const rawPage = {
  items: [rawCustomer],
  next_cursor: "opaque-server-cursor",
  total: 10_000,
  total_is_estimate: true,
  watermark: "2026-08-12T10:00:00Z",
};

const filters: CustomerListFilters = {
  addedAfter: "2026-08-11T00:00:00.000Z",
  addedBefore: "2026-08-12T00:00:00.000Z",
  channelID: 5,
  isDeleted: false,
  keyword: "陈晨",
  lastInteractAfter: "2026-08-11T01:00:00.000Z",
  lastInteractBefore: "2026-08-12T01:00:00.000Z",
  limit: 75,
  ownerStaffID: 8,
  stageID: 3,
  tagID: 9,
};

function transport(
  response: { readonly status: number; readonly data: unknown } = {
    status: 200,
    data: rawPage,
  },
): CustomerTransport {
  return { list: vi.fn(async () => response) };
}

describe("customer response parsing", () => {
  it("parses an exact, channel-neutral customer and frozen list envelope", () => {
    expect(
      parseCustomer({
        ...rawCustomer,
        avatar_url: "https://cdn.example.test/avatar.png",
      }),
    ).toBeDefined();
    expect(parseCustomer(rawCustomer)).toMatchObject({
      id: 7,
      ownerStaffID: 8,
      stageID: 3,
      addedAt: rawCustomer.added_at,
    });
    expect(parseCustomerListPage(rawPage)).toEqual({
      items: [
        {
          id: 7,
          name: "陈晨",
          stageID: 3,
          ownerStaffID: 8,
          channelID: 5,
          addedAt: rawCustomer.added_at,
          lastInteractAt: rawCustomer.last_interact_at,
          isDeleted: false,
        },
      ],
      nextCursor: "opaque-server-cursor",
      total: 10_000,
      totalIsEstimate: true,
      watermark: rawPage.watermark,
    });
  });

  it.each([
    { ...rawCustomer, external_userid: "forbidden" },
    { ...rawCustomer, id: 0 },
    { ...rawCustomer, avatar_url: "/relative-avatar.png" },
    { ...rawCustomer, avatar_url: 7 },
    { ...rawCustomer, avatar_url: " https://cdn.example.test/avatar.png" },
    { ...rawCustomer, avatar_url: "javascript:alert(1)" },
    { ...rawCustomer, avatar_url: "data:image/svg+xml,<svg/>" },
    { ...rawCustomer, avatar_url: "ftp://cdn.example.test/avatar.png" },
    {
      ...rawCustomer,
      avatar_url: "https://user:pass@cdn.example.test/avatar.png",
    },
    { ...rawCustomer, gender: 1.5 },
    { ...rawCustomer, gender: 2_147_483_648 },
    { ...rawCustomer, extra: [] },
    { ...rawCustomer, extra: new Date() },
    { ...rawCustomer, created_at: "not-a-date" },
    { ...rawCustomer, created_at: "2026-08-12T07:00:00" },
    { ...rawCustomer, updated_at: "2026-08-12T06:00:00Z" },
    { ...rawCustomer, added_at: "not-a-date" },
    { ...rawCustomer, owner_staff_id: 1.5 },
  ])("rejects malformed or expanded customer %#", (value) => {
    expect(parseCustomer(value)).toBeUndefined();
  });

  it.each([
    { ...rawPage, extra: true },
    { ...rawPage, next_cursor: "" },
    { ...rawPage, total: -1 },
    { ...rawPage, total: 0 },
    { ...rawPage, total: 10_001 },
    { ...rawPage, total: 9_999, total_is_estimate: true },
    { ...rawPage, items: [], next_cursor: "opaque-server-cursor" },
    { ...rawPage, items: [rawCustomer, rawCustomer] },
    { ...rawPage, items: [{ ...rawCustomer, id: 0 }] },
  ])("rejects malformed or impossible page %#", (value) => {
    expect(parseCustomerListPage(value)).toBeUndefined();
  });

  it("accepts only append pages with the same watermark and distinct customer ids", () => {
    const current = parseCustomerListPage(rawPage);
    const next = parseCustomerListPage({
      ...rawPage,
      items: [{ ...rawCustomer, id: 8 }],
      next_cursor: null,
    });
    if (!current || !next) throw new Error("expected valid pages");

    expect(
      appendCustomerListPage(current, next)?.items.map(({ id }) => id),
    ).toEqual([7, 8]);
    expect(
      appendCustomerListPage(current, {
        ...next,
        watermark: "2026-08-12T10:00:01Z",
      }),
    ).toBeUndefined();
    expect(
      appendCustomerListPage(current, {
        ...next,
        items: [{ ...next.items[0], id: 7 }],
      }),
    ).toBeUndefined();
  });

  it("keeps previous-page navigation local while accepting only its exact opaque cursor", () => {
    const first = parseCustomerListPage(rawPage);
    const second = parseCustomerListPage({
      ...rawPage,
      items: [{ ...rawCustomer, id: 8, name: "林小姐" }],
      next_cursor: null,
    });
    if (!first || !second) throw new Error("expected valid pages");

    const history = appendCustomerListHistoryPage(
      [{ page: first }],
      "opaque-server-cursor",
      second,
    );
    expect(history?.map((entry) => entry.page.items[0]?.id)).toEqual([7, 8]);
    expect(history?.[1]?.requestCursor).toBe("opaque-server-cursor");
    expect(
      appendCustomerListHistoryPage(
        [{ page: first }],
        "client-made-cursor",
        second,
      ),
    ).toBeUndefined();
  });
});

describe("customer filter parsing", () => {
  const validDraft: CustomerListFilterDraft = {
    addedAfter: "2026-08-11T08:00",
    addedBefore: "2026-08-12T08:00",
    channelID: "5",
    isDeleted: false,
    keyword: "陈晨",
    lastInteractAfter: "2026-08-11T09:00",
    lastInteractBefore: "2026-08-12T09:00",
    limit: "75",
    ownerStaffID: "8",
    stageID: "3",
    tagID: "9",
  };

  it("maps every frozen filter to the generated request shape", () => {
    const draft: CustomerListFilterDraft = {
      addedAfter: "2026-08-11T08:00",
      addedBefore: "2026-08-12T08:00",
      channelID: "5",
      isDeleted: true,
      keyword: "  陈晨  ",
      lastInteractAfter: "2026-08-11T09:00",
      lastInteractBefore: "2026-08-12T09:00",
      limit: "75",
      ownerStaffID: "8",
      stageID: "3",
      tagID: "9",
    };
    const parsed = parseCustomerListFilters(draft);

    expect(parsed).toMatchObject({
      ok: true,
      filters: {
        channelID: 5,
        isDeleted: true,
        keyword: "陈晨",
        limit: 75,
        ownerStaffID: 8,
        stageID: 3,
        tagID: 9,
      },
    });
    if (!parsed.ok) throw new Error("expected a valid filter draft");

    expect(
      customerListParams(parsed.filters, "opaque=cursor&only-server"),
    ).toEqual({
      added_after: new Date(draft.addedAfter).toISOString(),
      added_before: new Date(draft.addedBefore).toISOString(),
      channel_id: 5,
      cursor: "opaque=cursor&only-server",
      is_deleted: true,
      keyword: "陈晨",
      last_interact_after: new Date(draft.lastInteractAfter).toISOString(),
      last_interact_before: new Date(draft.lastInteractBefore).toISOString(),
      limit: 75,
      owner_staff_id: 8,
      stage_id: 3,
      tag_id: 9,
    });
  });

  it.each([
    { ...validDraft, limit: "0" },
    { ...validDraft, ownerStaffID: "0" },
    { ...validDraft, stageID: "1.5" },
    {
      ...validDraft,
      addedAfter: "2026-08-13T08:00",
      addedBefore: "2026-08-12T08:00",
    },
    {
      ...validDraft,
      lastInteractAfter: "2026-08-13T08:00",
      lastInteractBefore: "2026-08-12T08:00",
    },
    { ...validDraft, keyword: "x".repeat(201) },
  ])("rejects invalid client-side filter input %#", (invalid) => {
    expect(parseCustomerListFilters(invalid)).toEqual({ ok: false });
  });
});

describe("customer list transport", () => {
  it("uses only the generated list client shape, same-origin credentials, and a server cursor", async () => {
    const client = transport();

    await expect(
      loadCustomers(client, filters, "opaque-server-cursor"),
    ).resolves.toMatchObject({
      status: "loaded",
      page: { nextCursor: "opaque-server-cursor", totalIsEstimate: true },
    });
    expect(client.list).toHaveBeenCalledWith(
      {
        added_after: filters.addedAfter,
        added_before: filters.addedBefore,
        channel_id: 5,
        cursor: "opaque-server-cursor",
        is_deleted: false,
        keyword: "陈晨",
        last_interact_after: filters.lastInteractAfter,
        last_interact_before: filters.lastInteractBefore,
        limit: 75,
        owner_staff_id: 8,
        stage_id: 3,
        tag_id: 9,
      },
      { credentials: "same-origin" },
    );
  });

  it.each([
    [400, "invalid", {}],
    [401, "unauthenticated", {}],
    [403, "forbidden", {}],
    [500, "unavailable", {}],
    [200, "unavailable", { ...rawPage, total: 0 }],
  ] as const)(
    "classifies status %i as %s without exposing response details",
    async (status, want, data) => {
      await expect(
        loadCustomers(transport({ status, data }), filters),
      ).resolves.toEqual({ status: want });
    },
  );

  it("fails closed when the generated client rejects", async () => {
    const client = transport();
    vi.mocked(client.list).mockRejectedValue(new Error("sensitive endpoint"));

    await expect(loadCustomers(client, filters)).resolves.toEqual({
      status: "unavailable",
    });
  });
});
