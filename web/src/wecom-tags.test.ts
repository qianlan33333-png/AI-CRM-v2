import { describe, expect, it, vi } from "vitest";
import {
  archiveWecomTag,
  archiveWecomTagGroup,
  confirmsArchivedWecomTag,
  confirmsArchivedWecomTagGroup,
  confirmsCreatedWecomTag,
  confirmsCreatedWecomTagGroup,
  createWecomTag,
  confirmsRenamedWecomTag,
  confirmsRenamedWecomTagGroup,
  createWecomTagGroup,
  filterWecomTagGroups,
  loadWecomTagCatalog,
  loadWecomTagExecutionGate,
  nextWecomTagPage,
  previousWecomTagPage,
  renameWecomTag,
  renameWecomTagGroup,
  wecomTagPage,
  wecomTagPageCount,
  wecomTagSearchState,
  parseWecomTagGroupArchiveSuccess,
  parseWecomTagArchiveSuccess,
  parseWecomTagExecutionGate,
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

const executionGate = {
  provider_execution_eligible: false,
  local_command_acceptance_available: true,
  local_queue_available: true,
  sync_executed: false,
  observed_at: "2026-08-20T09:00:00Z",
  real_external_call_executed: false,
} as const;

function executionGateTransport(status: number, data: unknown): WecomTagsTransport {
  return {
    executionGate: vi.fn(async () => ({ status, data, headers: new Headers() })),
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

const renamed = {
  ok: true,
  reason: "tag_updated",
  source_status: "local_catalog",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
  dry_run: false,
  tag: {
    tag_id: 10,
    group_id: 1,
    group_name: "意向",
    tag_name: "重点跟进",
    sort_order: 0,
  },
} as const;

const tagCreated = {
  ok: true,
  reason: "tag_created",
  source_status: "local_catalog",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
  dry_run: false,
  tag: {
    tag_id: 12,
    group_id: 1,
    group_name: "意向",
    tag_name: "待跟进",
    sort_order: 2,
  },
} as const;

const renamedGroup = {
  ok: true,
  reason: "group_updated",
  source_status: "local_catalog",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
  dry_run: false,
  group: { group_id: 1, group_name: "意向阶段", sort_order: 0 },
} as const;

const archivedGroup = {
  ok: true,
  reason: "group_archived",
  source_status: "local_catalog",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
  dry_run: false,
  group: { group_id: 1, group_name: "archived:1", sort_order: 0 },
} as const;

const archiveValidated = {
  ok: true,
  reason: "group_archive_validated",
  source_status: "local_catalog",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
  dry_run: true,
} as const;

const archivedTag = {
  ok: true,
  reason: "tag_archived",
  source_status: "local_catalog",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  sync_executed: false,
  fixture_used: false,
  dry_run: false,
  tag: {
    tag_id: 10,
    group_id: 1,
    group_name: "意向",
    tag_name: "高意向",
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

describe("WeCom tag execution gate read boundary", () => {
  it("accepts only the closed safe local projection", async () => {
    const client = executionGateTransport(200, executionGate);
    await expect(loadWecomTagExecutionGate(client)).resolves.toEqual({
      status: "loaded",
      gate: {
        providerExecutionEligible: false,
        localCommandAcceptanceAvailable: true,
        localQueueAvailable: true,
        syncExecuted: false,
        observedAt: "2026-08-20T09:00:00Z",
        realExternalCallExecuted: false,
      },
    });
    expect(client.executionGate).toHaveBeenCalledOnce();
  });

  it("rejects payload, provider success claims, and malformed local observations", async () => {
    for (const value of [
      { ...executionGate, mode: "provider_execution_unavailable" },
      { ...executionGate, provider_execution_eligible: true },
      { ...executionGate, local_queue_available: false },
      { ...executionGate, observed_at: "2026-02-30T09:00:00Z" },
    ]) {
      await expect(
        loadWecomTagExecutionGate(executionGateTransport(200, value)),
      ).resolves.toEqual({ status: "invalid" });
    }
    expect(parseWecomTagExecutionGate({ ...executionGate, payload: {} })).toBeUndefined();
    await expect(
      loadWecomTagExecutionGate(executionGateTransport(401, {})),
    ).resolves.toEqual({ status: "unauthenticated" });
    await expect(
      loadWecomTagExecutionGate(executionGateTransport(403, {})),
    ).resolves.toEqual({ status: "forbidden" });
    await expect(
      loadWecomTagExecutionGate(executionGateTransport(503, {})),
    ).resolves.toEqual({ status: "unavailable" });
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
      confirmsCreatedWecomTagGroup({ ...confirmed, groups: [] }, result),
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

describe("WeCom local additional-tag creation boundary", () => {
  const group = { id: 1, name: "意向", sortOrder: 0 } as const;

  it("sends only the selected local group and normalized tag name", async () => {
    const client = {
      ...transport(200, catalog),
      createTag: vi.fn(async () => ({
        status: 200,
        data: tagCreated,
        headers: new Headers(),
      })),
    } as unknown as WecomTagsTransport;

    await expect(
      createWecomTag(client, group, " 待跟进 ", CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({
      status: "created",
      tag: {
        id: 12,
        groupID: 1,
        groupName: "意向",
        name: "待跟进",
        sortOrder: 2,
      },
    });
    expect(client.createTag).toHaveBeenCalledWith(
      { group_id: 1, group_name: "意向", tag_name: "待跟进" },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": CSRF_TOKEN,
          "Idempotency-Key": IDEMPOTENCY_KEY,
        },
      },
    );
  });

  it("fails closed without transport for malformed local input", async () => {
    const client = {
      ...transport(200, catalog),
      createTag: vi.fn(),
    } as unknown as WecomTagsTransport;
    await expect(
      createWecomTag(client, group, " ", CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      createWecomTag(client, { ...group, id: 0 }, "待跟进", CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.createTag).not.toHaveBeenCalled();
  });

  it("rejects receipt drift, dry-run, and unexpected keys as unknown", async () => {
    for (const data of [
      { ...tagCreated, tag: { ...tagCreated.tag, group_id: 2 } },
      { ...tagCreated, tag: { ...tagCreated.tag, tag_name: "已漂移" } },
      { ...tagCreated, dry_run: true },
      { ...tagCreated, unexpected: true },
    ]) {
      const client = {
        ...transport(200, catalog),
        createTag: vi.fn(async () => ({ status: 200, data, headers: new Headers() })),
      } as unknown as WecomTagsTransport;
      await expect(
        createWecomTag(client, group, "待跟进", CSRF_TOKEN, IDEMPOTENCY_KEY),
      ).resolves.toEqual({ status: "unknown" });
    }
  });

  it("requires the refreshed catalog to mirror the created tag", () => {
    const createdTag = {
      id: 12,
      groupID: 1,
      groupName: "意向",
      name: "待跟进",
      sortOrder: 2,
    } as const;
    const confirmed = {
      totalTags: 1,
      tagLimit: 1000,
      snapshotAt: "2026-08-19T00:00:00Z",
      groups: [{ ...group, tags: [createdTag] }],
      tags: [createdTag],
    };
    expect(confirmsCreatedWecomTag(confirmed, createdTag)).toBe(true);
    expect(confirmsCreatedWecomTag({ ...confirmed, groups: [] }, createdTag)).toBe(false);
    expect(
      confirmsCreatedWecomTag(
        { ...confirmed, tags: [{ ...createdTag, name: "已漂移" }] },
        createdTag,
      ),
    ).toBe(false);
  });
});

describe("WeCom local tag rename boundary", () => {
  it("sends only a normalized tag name with same-origin CSRF and a unique key", async () => {
    const client = {
      ...transport(200, catalog),
      rename: vi.fn(async () => ({
        status: 200,
        data: renamed,
        headers: new Headers(),
      })),
    } as unknown as WecomTagsTransport;
    const target = {
      id: 10,
      groupID: 1,
      groupName: "意向",
      name: "高意向",
      sortOrder: 0,
    } as const;

    await expect(
      renameWecomTag(client, target, " 重点跟进 ", CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({
      status: "confirmed",
      tag: {
        id: 10,
        groupID: 1,
        groupName: "意向",
        name: "重点跟进",
        sortOrder: 0,
      },
    });
    expect(client.rename).toHaveBeenCalledOnce();
    expect(client.rename).toHaveBeenCalledWith(
      10,
      { tag_name: "重点跟进" },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": CSRF_TOKEN,
          "Idempotency-Key": IDEMPOTENCY_KEY,
        },
      },
    );
  });

  it("fails closed on malformed, drifting, or wrong-target rename results", async () => {
    const target = {
      id: 10,
      groupID: 1,
      groupName: "意向",
      name: "高意向",
      sortOrder: 0,
    } as const;
    for (const data of [
      { ...renamed, unknown: true },
      { ...renamed, tag: { ...renamed.tag, tag_id: 11 } },
      { ...renamed, tag: { ...renamed.tag, group_id: 2 } },
      { ...renamed, tag: { ...renamed.tag, tag_name: "已漂移" } },
    ]) {
      const client = {
        ...transport(200, catalog),
        rename: vi.fn(async () => ({
          status: 200,
          data,
          headers: new Headers(),
        })),
      } as unknown as WecomTagsTransport;
      await expect(
        renameWecomTag(client, target, "重点跟进", CSRF_TOKEN, IDEMPOTENCY_KEY),
      ).resolves.toEqual({ status: "unknown" });
    }
  });

  it("does not send invalid local input or security metadata", async () => {
    const client = {
      ...transport(200, catalog),
      rename: vi.fn(),
    } as unknown as WecomTagsTransport;
    const target = {
      id: 10,
      groupID: 1,
      groupName: "意向",
      name: "高意向",
      sortOrder: 0,
    } as const;
    await expect(
      renameWecomTag(client, target, " ", CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      renameWecomTag(client, target, "重点跟进", "bad", IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.rename).not.toHaveBeenCalled();
  });

  it("requires the refreshed catalog to mirror every renamed tag field", async () => {
    const loaded = await loadWecomTagCatalog(transport(200, catalog));
    if (loaded.status !== "loaded") throw new Error("fixture must load");
    const result = {
      id: 10,
      groupID: 1,
      groupName: "意向",
      name: "高意向",
      sortOrder: 0,
    };
    expect(confirmsRenamedWecomTag(loaded.catalog, result)).toBe(true);
    expect(
      confirmsRenamedWecomTag(loaded.catalog, { ...result, name: "已漂移" }),
    ).toBe(false);
    expect(
      confirmsRenamedWecomTag(loaded.catalog, { ...result, groupID: 2 }),
    ).toBe(false);
  });
});

describe("WeCom local tag-group rename boundary", () => {
  it("sends only a normalized group name with same-origin CSRF and a unique key", async () => {
    const client = {
      ...transport(200, catalog),
      renameGroup: vi.fn(async () => ({
        status: 200,
        data: renamedGroup,
        headers: new Headers(),
      })),
    } as unknown as WecomTagsTransport;
    const target = { id: 1, name: "意向", sortOrder: 0 } as const;

    await expect(
      renameWecomTagGroup(
        client,
        target,
        " 意向阶段 ",
        CSRF_TOKEN,
        IDEMPOTENCY_KEY,
      ),
    ).resolves.toEqual({
      status: "confirmed",
      group: { id: 1, name: "意向阶段", sortOrder: 0 },
    });
    expect(client.renameGroup).toHaveBeenCalledOnce();
    expect(client.renameGroup).toHaveBeenCalledWith(
      1,
      { group_name: "意向阶段" },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": CSRF_TOKEN,
          "Idempotency-Key": IDEMPOTENCY_KEY,
        },
      },
    );
  });

  it("fails closed on malformed, dry-run, or drifting group results", async () => {
    const target = { id: 1, name: "意向", sortOrder: 0 } as const;
    for (const data of [
      { ...renamedGroup, unknown: true },
      { ...renamedGroup, group: { ...renamedGroup.group, group_id: 2 } },
      { ...renamedGroup, group: { ...renamedGroup.group, sort_order: 1 } },
      {
        ok: true,
        reason: "group_update_validated",
        source_status: "local_catalog",
        route_owner: "ai_crm_next",
        fallback_used: false,
        real_external_call_executed: false,
        sync_executed: false,
        fixture_used: false,
        dry_run: true,
      },
    ]) {
      const client = {
        ...transport(200, catalog),
        renameGroup: vi.fn(async () => ({
          status: 200,
          data,
          headers: new Headers(),
        })),
      } as unknown as WecomTagsTransport;
      await expect(
        renameWecomTagGroup(
          client,
          target,
          "意向阶段",
          CSRF_TOKEN,
          IDEMPOTENCY_KEY,
        ),
      ).resolves.toEqual({ status: "unknown" });
    }
  });

  it("does not send invalid local input or security metadata", async () => {
    const client = {
      ...transport(200, catalog),
      renameGroup: vi.fn(),
    } as unknown as WecomTagsTransport;
    const target = { id: 1, name: "意向", sortOrder: 0 } as const;
    await expect(
      renameWecomTagGroup(client, target, " ", CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      renameWecomTagGroup(client, target, "意向阶段", "bad", IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.renameGroup).not.toHaveBeenCalled();
  });

  it("requires the refreshed catalog to mirror the group and every child projection", async () => {
    const loaded = await loadWecomTagCatalog(transport(200, catalog));
    if (loaded.status !== "loaded") throw new Error("fixture must load");
    const result = { id: 1, name: "意向", sortOrder: 0 };
    expect(confirmsRenamedWecomTagGroup(loaded.catalog, result)).toBe(true);
    expect(
      confirmsRenamedWecomTagGroup(loaded.catalog, {
        ...result,
        name: "意向阶段",
      }),
    ).toBe(false);
    expect(
      confirmsRenamedWecomTagGroup(
        {
          ...loaded.catalog,
          tags: [
            { ...loaded.catalog.tags[0]!, groupName: "已漂移" },
            ...loaded.catalog.tags.slice(1),
          ],
        },
        result,
      ),
    ).toBe(false);
    expect(
      confirmsRenamedWecomTagGroup(
        {
          ...loaded.catalog,
          groups: [
            {
              ...loaded.catalog.groups[0]!,
              tags: [
                {
                  ...loaded.catalog.groups[0]!.tags[0]!,
                  groupName: "已漂移",
                },
                ...loaded.catalog.groups[0]!.tags.slice(1),
              ],
            },
            ...loaded.catalog.groups.slice(1),
          ],
        },
        result,
      ),
    ).toBe(false);
  });
});

describe("WeCom local tag-group archive boundary", () => {
  it("sends the required empty JSON body with same-origin security metadata", async () => {
    const client = {
      ...transport(200, catalog),
      archiveGroup: vi.fn(async () => ({
        status: 200,
        data: archivedGroup,
        headers: new Headers(),
      })),
    } as unknown as WecomTagsTransport;

    await expect(
      archiveWecomTagGroup(
        client,
        {
          id: 1,
          name: "意向",
          sortOrder: 0,
          tags: [
            {
              id: 10,
              groupID: 1,
              groupName: "意向",
              name: "高意向",
              sortOrder: 0,
            },
            {
              id: 11,
              groupID: 1,
              groupName: "意向",
              name: "低意向",
              sortOrder: 1,
            },
          ],
        },
        CSRF_TOKEN,
        IDEMPOTENCY_KEY,
      ),
    ).resolves.toEqual({
      status: "archived",
      group: { id: 1, name: "archived:1", sortOrder: 0 },
    });
    expect(client.archiveGroup).toHaveBeenCalledWith(
      1,
      {},
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": CSRF_TOKEN,
          "Idempotency-Key": IDEMPOTENCY_KEY,
        },
      },
    );
  });

  it("accepts only the two closed archive envelopes and never treats dry-run as completion", async () => {
    expect(parseWecomTagGroupArchiveSuccess(archivedGroup)).toEqual({
      status: "archived",
      group: { id: 1, name: "archived:1", sortOrder: 0 },
    });
    expect(parseWecomTagGroupArchiveSuccess(archiveValidated)).toEqual({
      status: "validated",
    });
    for (const data of [
      { ...archivedGroup, unexpected: true },
      { ...archivedGroup, group: { ...archivedGroup.group, unexpected: true } },
      { ...archiveValidated, dry_run: false },
    ]) {
      expect(parseWecomTagGroupArchiveSuccess(data)).toBeUndefined();
    }
    const client = {
      ...transport(200, catalog),
      archiveGroup: vi.fn(async () => ({
        status: 200,
        data: archiveValidated,
      })),
    } as unknown as WecomTagsTransport;
    await expect(
      archiveWecomTagGroup(
        client,
        { ...catalog.groups[0], id: 1, name: "意向", sortOrder: 0, tags: [] },
        CSRF_TOKEN,
        IDEMPOTENCY_KEY,
      ),
    ).resolves.toEqual({ status: "unknown" });
  });

  it("fails closed for response drift and requires the original group and all children to vanish after reread", async () => {
    const original = {
      id: 1,
      name: "意向",
      sortOrder: 0,
      tags: [
        { id: 10, groupID: 1, groupName: "意向", name: "高意向", sortOrder: 0 },
        { id: 11, groupID: 1, groupName: "意向", name: "低意向", sortOrder: 1 },
      ],
    } as const;
    const result = {
      status: "archived" as const,
      group: { id: 1, name: "archived:1", sortOrder: 0 },
    };
    const remaining = {
      totalTags: 1,
      tagLimit: 1000,
      snapshotAt: "2026-08-19T00:00:00Z",
      groups: [
        {
          id: 2,
          name: "来源",
          sortOrder: 1,
          tags: [
            {
              id: 21,
              groupID: 2,
              groupName: "来源",
              name: "社群",
              sortOrder: 0,
            },
          ],
        },
      ],
      tags: [
        { id: 21, groupID: 2, groupName: "来源", name: "社群", sortOrder: 0 },
      ],
    };
    expect(confirmsArchivedWecomTagGroup(remaining, result, original)).toBe(
      true,
    );
    expect(
      confirmsArchivedWecomTagGroup(
        { ...remaining, tags: [...remaining.tags, original.tags[0]] },
        result,
        original,
      ),
    ).toBe(false);
    const client = {
      ...transport(200, catalog),
      archiveGroup: vi.fn(async () => ({
        status: 200,
        data: {
          ...archivedGroup,
          group: { ...archivedGroup.group, group_name: "archived:2" },
        },
      })),
    } as unknown as WecomTagsTransport;
    await expect(
      archiveWecomTagGroup(client, original, CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "unknown" });
  });

  it("does not issue an archive for a malformed original group projection", async () => {
    const client = {
      ...transport(200, catalog),
      archiveGroup: vi.fn(),
    } as unknown as WecomTagsTransport;
    await expect(
      archiveWecomTagGroup(
        client,
        {
          id: 1,
          name: "意向",
          sortOrder: 0,
          tags: [
            {
              id: 10,
              groupID: 2,
              groupName: "意向",
              name: "高意向",
              sortOrder: 0,
            },
          ],
        },
        CSRF_TOKEN,
        IDEMPOTENCY_KEY,
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.archiveGroup).not.toHaveBeenCalled();
  });
});

describe("WeCom local tag archive boundary", () => {
  const original = {
    id: 10,
    groupID: 1,
    groupName: "意向",
    name: "高意向",
    sortOrder: 0,
  } as const;

  it("uses the generated DELETE transport with a required empty body and accepts only its closed normal receipt", async () => {
    const client = {
      ...transport(200, catalog),
      archiveTag: vi.fn(async () => ({ status: 200, data: archivedTag })),
    } as unknown as WecomTagsTransport;
    await expect(
      archiveWecomTag(client, original, CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({
      status: "archived",
      tag: original,
    });
    expect(client.archiveTag).toHaveBeenCalledWith(
      10,
      {},
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": CSRF_TOKEN,
          "Idempotency-Key": IDEMPOTENCY_KEY,
        },
      },
    );
  });

  it("fails closed for dry-run, receipt drift, extra fields, and an inconsistent reread", async () => {
    expect(parseWecomTagArchiveSuccess(archivedTag)).toEqual({
      status: "archived",
      tag: original,
    });
    const client = {
      ...transport(200, catalog),
      archiveTag: vi.fn(async () => ({
        status: 200,
        data: { ...archivedTag, unexpected: true },
      })),
    } as unknown as WecomTagsTransport;
    await expect(
      archiveWecomTag(client, original, CSRF_TOKEN, IDEMPOTENCY_KEY),
    ).resolves.toEqual({ status: "unknown" });
    expect(
      confirmsArchivedWecomTag(
        {
          totalTags: 2,
          tagLimit: 1000,
          snapshotAt: "2026-08-20T00:00:00Z",
          groups: [{ id: 1, name: "意向", sortOrder: 0, tags: [original] }],
          tags: [original],
        },
        { status: "archived", tag: original },
        original,
      ),
    ).toBe(false);
  });
});
