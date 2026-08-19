import { describe, expect, it, vi } from "vitest";
import {
  confirmsCreatedWecomTagGroup,
  createWecomTagGroup,
  filterWecomTagGroups,
  loadWecomTagCatalog,
  nextWecomTagPage,
  previousWecomTagPage,
  wecomTagPage,
  wecomTagPageCount,
  wecomTagSearchState,
  type WecomTagsTransport,
} from "./wecom-tags";

const CSRF_TOKEN = "c".repeat(43);
const IDEMPOTENCY_KEY = "k".repeat(43);

const tags = [
  {
    tag_id: 10,
    id: 10,
    group_id: 1,
    group_name: "意向",
    tag_name: "高意向",
    name: "高意向",
    sort_order: 0,
  },
  {
    tag_id: 11,
    id: 11,
    group_id: 1,
    group_name: "意向",
    tag_name: "低意向",
    name: "低意向",
    sort_order: 1,
  },
  {
    tag_id: 21,
    id: 21,
    group_id: 2,
    group_name: "来源",
    tag_name: "社群",
    name: "社群",
    sort_order: 0,
  },
] as const;

const catalog = {
  ok: true,
  items: tags,
  tags,
  groups: [
    {
      group_id: 1,
      group_name: "意向",
      name: "意向",
      sort_order: 0,
      tags: tags.slice(0, 2),
    },
    {
      group_id: 2,
      group_name: "来源",
      name: "来源",
      sort_order: 1,
      tags: tags.slice(2),
    },
  ],
  count: 3,
  total_tags: 3,
  tag_limit: 1000,
  synced_at: "2026-08-19T00:00:00Z",
  source_status: "local_catalog",
  read_model_status: "ready",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
} as const;

function transport(status: number, data: unknown): WecomTagsTransport {
  return {
    read: vi.fn(async () => ({ status, data, headers: new Headers() })),
  } as unknown as WecomTagsTransport;
}

const created = {
  ok: true,
  reason: "group_created",
  source_status: "local_catalog",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
  dry_run: false,
  group: { group_id: 31, group_name: "客户阶段", sort_order: 0 },
  tag: {
    tag_id: 41,
    group_id: 31,
    group_name: "客户阶段",
    tag_name: "新客",
    sort_order: 0,
  },
} as const;

describe("WeCom tag catalog read boundary", () => {
  it("accepts only the frozen ready catalog", async () => {
    const client = transport(200, catalog);
    const result = await loadWecomTagCatalog(client);
    expect(result.status).toBe("loaded");
    if (result.status !== "loaded") throw new Error("fixture must load");
    expect(result.catalog.totalTags).toBe(3);
    expect(result.catalog.tagLimit).toBe(1000);
    expect(result.catalog.snapshotAt).toBe("2026-08-19T00:00:00Z");
    expect(result.catalog.groups[0]?.tags[0]).toMatchObject({
      id: 10,
      name: "高意向",
    });
    expect(result.catalog.tags[2]).toMatchObject({
      id: 21,
      groupID: 2,
      groupName: "来源",
      name: "社群",
    });
    expect(client.read).toHaveBeenCalledWith();
  });

  it("fails closed for non-ready, malformed, authentication, and transport outcomes", async () => {
    const negativeSortTags = [
      { ...catalog.tags[0], sort_order: -1 },
      ...catalog.tags.slice(1),
    ];
    const negativeSortCatalog = {
      ...catalog,
      items: negativeSortTags,
      tags: negativeSortTags,
      groups: [
        { ...catalog.groups[0], tags: negativeSortTags.slice(0, 2) },
        { ...catalog.groups[1], tags: negativeSortTags.slice(2) },
      ],
    };
    for (const data of [
      { ...catalog, read_model_status: "unavailable" },
      { ...catalog, total_tags: 4 },
      { ...catalog, tag_limit: 999 },
      { ...catalog, synced_at: "2026-02-31T00:00:00Z" },
      negativeSortCatalog,
      {
        ...catalog,
        groups: [
          { ...catalog.groups[0], sort_order: 2_147_483_648 },
          catalog.groups[1],
        ],
      },
      {
        ...catalog,
        groups: [{ ...catalog.groups[0], tags: [] }, catalog.groups[1]],
      },
      { ...catalog, unknown: true },
    ]) {
      await expect(loadWecomTagCatalog(transport(200, data))).resolves.toEqual({
        status: "invalid",
      });
    }
    await expect(loadWecomTagCatalog(transport(401, {}))).resolves.toEqual({
      status: "unauthenticated",
    });
    await expect(loadWecomTagCatalog(transport(403, {}))).resolves.toEqual({
      status: "forbidden",
    });
    await expect(loadWecomTagCatalog(transport(503, {}))).resolves.toEqual({
      status: "unavailable",
    });
    const throwing = transport(200, catalog);
    vi.mocked(throwing.read).mockRejectedValue(new Error("network"));
    await expect(loadWecomTagCatalog(throwing)).resolves.toEqual({
      status: "unavailable",
    });
  });

  it("accepts the fixed sort-order boundaries", async () => {
    const boundedTags = [
      { ...catalog.tags[0], sort_order: 0 },
      { ...catalog.tags[1], sort_order: 2_147_483_647 },
      catalog.tags[2],
    ];
    const bounded = {
      ...catalog,
      items: boundedTags,
      tags: boundedTags,
      groups: [
        {
          ...catalog.groups[0],
          sort_order: 0,
          tags: boundedTags.slice(0, 2),
        },
        {
          ...catalog.groups[1],
          sort_order: 2_147_483_647,
          tags: boundedTags.slice(2),
        },
      ],
    };
    await expect(
      loadWecomTagCatalog(transport(200, bounded)),
    ).resolves.toMatchObject({
      status: "loaded",
    });
  });

  it("accepts a leap-day snapshot only when it is a real calendar date", async () => {
    await expect(
      loadWecomTagCatalog(
        transport(200, { ...catalog, synced_at: "2024-02-29T23:59:59+08:00" }),
      ),
    ).resolves.toMatchObject({ status: "loaded" });
  });

  it("filters group name, tag name, and tag ID locally and pages at twenty", async () => {
    const loaded = await loadWecomTagCatalog(transport(200, catalog));
    if (loaded.status !== "loaded") throw new Error("fixture must load");
    expect(filterWecomTagGroups(loaded.catalog, "来源")).toHaveLength(1);
    expect(
      filterWecomTagGroups(loaded.catalog, "高意向")[0]?.tags,
    ).toHaveLength(1);
    expect(filterWecomTagGroups(loaded.catalog, "21")[0]?.id).toBe(2);
    expect(filterWecomTagGroups(loaded.catalog, "不存在")).toEqual([]);
    expect(wecomTagSearchState(loaded.catalog, "社群")).toMatchObject({
      selectedGroupID: 2,
      page: 0,
    });

    for (const [total, pages] of [
      [0, 1],
      [1, 1],
      [19, 1],
      [20, 1],
      [21, 2],
      [40, 2],
      [41, 3],
    ] as const) {
      const paged = Array.from({ length: total }, (_, index) => ({
        id: index + 1,
        groupID: 1,
        groupName: "意向",
        name: `标签 ${index + 1}`,
        sortOrder: index,
      }));
      expect(wecomTagPageCount(paged), String(total)).toBe(pages);
      expect(nextWecomTagPage(pages - 1, paged), String(total)).toBeUndefined();
    }

    const many = Array.from({ length: 41 }, (_, index) => ({
      id: index + 1,
      groupID: 1,
      groupName: "意向",
      name: `标签 ${index + 1}`,
      sortOrder: index,
    }));
    expect(wecomTagPageCount(many)).toBe(3);
    expect(wecomTagPage(many, 0)).toHaveLength(20);
    expect(wecomTagPage(many, 1)[0]?.id).toBe(21);
    expect(wecomTagPage(many, 99)).toHaveLength(1);
    expect(previousWecomTagPage(0)).toBe(0);
    expect(previousWecomTagPage(2)).toBe(1);
    expect(nextWecomTagPage(1, many)).toBe(2);
    expect(nextWecomTagPage(2, many)).toBeUndefined();
  });
});

describe("WeCom local tag-group creation boundary", () => {
  it("sends only normalized local names with same-origin CSRF and a unique key", async () => {
    const client = {
      ...transport(200, catalog),
      create: vi.fn(async () => ({
        status: 200,
        data: created,
        headers: new Headers(),
      })),
    } as unknown as WecomTagsTransport;

    await expect(
      createWecomTagGroup(
        client,
        " 客户阶段 ",
        " 新客 ",
        CSRF_TOKEN,
        IDEMPOTENCY_KEY,
      ),
    ).resolves.toMatchObject({
      status: "created",
      group: { id: 31, name: "客户阶段" },
      tag: { id: 41, groupID: 31, name: "新客" },
    });
    expect(client.create).toHaveBeenCalledOnce();
    expect(client.create).toHaveBeenCalledWith(
      { group_name: "客户阶段", first_tag_name: "新客" },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": CSRF_TOKEN,
          "Idempotency-Key": IDEMPOTENCY_KEY,
        },
      },
    );
  });

  it("fails closed when either created name drifts from this request", async () => {
    for (const response of [
      {
        ...created,
        group: { ...created.group, group_name: "其他组" },
        tag: { ...created.tag, group_name: "其他组" },
      },
      { ...created, tag: { ...created.tag, tag_name: "其他标签" } },
    ]) {
      const client = {
        ...transport(200, catalog),
        create: vi.fn(async () => ({
          status: 200,
          data: response,
          headers: new Headers(),
        })),
      } as unknown as WecomTagsTransport;
      await expect(
        createWecomTagGroup(
          client,
          "客户阶段",
          "新客",
          CSRF_TOKEN,
          IDEMPOTENCY_KEY,
        ),
      ).resolves.toEqual({ status: "unknown" });
    }
  });

  it("does not send malformed local input or a bad security header", async () => {
    const client = {
      ...transport(200, catalog),
      create: vi.fn(),
    } as unknown as WecomTagsTransport;
    await expect(
      createWecomTagGroup(client, " ", "新客", CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      createWecomTagGroup(client, "组", "标签", "bad", IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.create).not.toHaveBeenCalled();
  });

  it("counts Unicode runes, not UTF-16 code units, at the 200-rune boundary", async () => {
    const supplementary = "\u{1F600}".repeat(200);
    const client = {
      ...transport(200, catalog),
      create: vi.fn(async () => ({
        status: 200,
        data: {
          ...created,
          group: { ...created.group, group_name: supplementary },
          tag: {
            ...created.tag,
            group_name: supplementary,
            tag_name: supplementary,
          },
        },
        headers: new Headers(),
      })),
    } as unknown as WecomTagsTransport;
    await expect(
      createWecomTagGroup(
        client,
        supplementary,
        supplementary,
        CSRF_TOKEN,
        IDEMPOTENCY_KEY,
      ),
    ).resolves.toMatchObject({ status: "created" });
    expect(client.create).toHaveBeenCalledOnce();

    const rejected = {
      ...transport(200, catalog),
      create: vi.fn(),
    } as unknown as WecomTagsTransport;
    await expect(
      createWecomTagGroup(
        rejected,
        "\u{1F600}".repeat(201),
        "新客",
        CSRF_TOKEN,
        IDEMPOTENCY_KEY,
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(rejected.create).not.toHaveBeenCalled();
  });

  it("requires the refreshed local catalog to mirror the confirmed group and tag", () => {
    const result = {
      status: "created" as const,
      group: { id: 31, name: "客户阶段", sortOrder: 0 },
      tag: {
        id: 41,
        groupID: 31,
        groupName: "客户阶段",
        name: "新客",
        sortOrder: 0,
      },
    };
    const confirmed = {
      totalTags: 1,
      tagLimit: 1000,
      snapshotAt: "2026-08-19T00:00:00Z",
      groups: [
        {
          id: 31,
          name: "客户阶段",
          sortOrder: 0,
          tags: [result.tag],
        },
      ],
      tags: [result.tag],
    };
    expect(confirmsCreatedWecomTagGroup(confirmed, result)).toBe(true);
    expect(
      confirmsCreatedWecomTagGroup(
        { ...confirmed, groups: [] },
        result,
      ),
    ).toBe(false);
    expect(
      confirmsCreatedWecomTagGroup(
        {
          ...confirmed,
          tags: [{ ...result.tag, name: "已漂移" }],
        },
        result,
      ),
    ).toBe(false);
  });
});
