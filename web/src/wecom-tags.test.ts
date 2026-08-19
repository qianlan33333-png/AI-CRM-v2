import { describe, expect, it, vi } from "vitest";
import {
  filterWecomTagGroups,
  loadWecomTagCatalog,
  nextWecomTagPage,
  previousWecomTagPage,
  wecomTagPage,
  wecomTagPageCount,
  wecomTagSearchState,
  type WecomTagsTransport,
} from "./wecom-tags";

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
