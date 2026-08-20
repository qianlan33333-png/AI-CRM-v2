import { describe, expect, it, vi } from "vitest";
import {
  loadStages,
  parseStage,
  parseStageList,
  submitStageArchive,
  submitStageCreate,
  submitStageReorder,
  submitStageRename,
  type StageTransport,
} from "./stages";

const rawStage = {
  id: 7,
  name: "已联系",
  sort_order: 20,
  config: { color: "amber" },
};

function transport(
  response: { status: number; data: unknown } = {
    status: 200,
    data: { items: [rawStage] },
  },
): StageTransport {
  return {
    list: vi.fn(async () => response),
    create: vi.fn(async () => response),
    rename: vi.fn(async () => response),
  };
}

describe("stage response parsing", () => {
  it("parses an exact stage and preserves service order", () => {
    expect(parseStage(rawStage)).toEqual({
      id: 7,
      name: "已联系",
      sortOrder: 20,
      config: { color: "amber" },
    });
    expect(
      parseStageList({
        items: [
          { ...rawStage, id: 2, name: "第二", sort_order: 9 },
          { ...rawStage, id: 1, name: "第一", sort_order: 1 },
        ],
      })?.map((stage) => stage.id),
    ).toEqual([2, 1]);
  });

  it.each([
    null,
    [],
    {},
    { ...rawStage, extra: true },
    { id: 0, name: "阶段", sort_order: 1, config: {} },
    { id: 1.5, name: "阶段", sort_order: 1, config: {} },
    { id: 1, name: "", sort_order: 1, config: {} },
    { id: 1, name: "阶段", sort_order: 1.5, config: {} },
    { id: 1, name: "阶段", sort_order: 1 },
  ])("rejects an unsafe stage %#", (value) => {
    expect(parseStage(value)).toBeUndefined();
  });

  it.each([
    null,
    [],
    {},
    { items: "not-an-array" },
    { items: [rawStage], extra: true },
    { items: [rawStage, { ...rawStage, id: 0 }] },
  ])("rejects a partial or expanded list %#", (value) => {
    expect(parseStageList(value)).toBeUndefined();
  });
});

describe("stage transport classification", () => {
  it("loads only a 200 exact list with same-origin credentials", async () => {
    const client = transport();
    await expect(loadStages(client)).resolves.toMatchObject({
      status: "loaded",
      items: [{ id: 7, name: "已联系" }],
    });
    expect(client.list).toHaveBeenCalledWith({ credentials: "same-origin" });
  });

  it.each([
    [401, "unauthenticated", {}],
    [403, "unavailable", {}],
    [503, "unavailable", {}],
    [200, "unavailable", { items: [{ ...rawStage, id: 0 }] }],
  ] as const)("classifies list status %i as %s", async (status, want, data) => {
    await expect(loadStages(transport({ status, data }))).resolves.toEqual({
      status: want,
    });
  });

  it("fails closed when list transport rejects", async () => {
    const client = transport();
    vi.mocked(client.list).mockRejectedValue(new Error("offline with details"));
    await expect(loadStages(client)).resolves.toEqual({
      status: "unavailable",
    });
  });

	const key = "stage-create:123e4567-e89b-42d3-a456-426614174000";
  it("creates with exact input, same-origin credentials, CSRF, and idempotency", async () => {
    const client = transport({ status: 201, data: rawStage });
    await expect(
      submitStageCreate(
        client,
		{ name: "  原样名称  ", sort_order: 9 },
		"token",
		key,
      ),
    ).resolves.toMatchObject({ status: "created", stage: { id: 7 } });
    expect(client.create).toHaveBeenCalledWith(
      { name: "  原样名称  ", sort_order: 9 },
      {
        credentials: "same-origin",
		headers: { "X-CSRF-Token": "token", "Idempotency-Key": key },
      },
    );
  });

  it.each([
    [400, "invalid"],
    [401, "unauthenticated"],
    [403, "forbidden"],
    [404, "not_found"],
    [409, "conflict"],
    [422, "invalid"],
    [503, "unavailable"],
    [599, "unavailable"],
  ] as const)("classifies mutation status %i as %s", async (status, want) => {
    const client = transport({ status, data: {} });
    await expect(
		submitStageCreate(client, { name: "阶段" }, "token", key),
    ).resolves.toEqual({ status: want });
  });

  it("renames the requested id and rejects mismatched or malformed success bodies", async () => {
    const client = transport({ status: 200, data: rawStage });
    await expect(
		submitStageRename(client, 7, { name: "新名称" }, "token", key),
    ).resolves.toMatchObject({ status: "renamed", stage: { id: 7 } });
    expect(client.rename).toHaveBeenCalledWith(
      7,
      { name: "新名称" },
      {
        credentials: "same-origin",
		headers: { "X-CSRF-Token": "token", "Idempotency-Key": key },
      },
    );

    vi.mocked(client.rename).mockResolvedValue({
      status: 200,
      data: { ...rawStage, id: 8 },
    });
    await expect(
		submitStageRename(client, 7, { name: "新名称" }, "token", key),
    ).resolves.toEqual({ status: "unavailable" });
  });

  it("never exposes a rejected transport error", async () => {
    const client = transport();
    vi.mocked(client.create).mockRejectedValue(new Error("secret endpoint"));
    vi.mocked(client.rename).mockRejectedValue(new Error("secret endpoint"));
    await expect(
		submitStageCreate(client, { name: "阶段" }, "token", key),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
		submitStageRename(client, 7, { name: "阶段" }, "token", key),
    ).resolves.toEqual({ status: "unavailable" });
  });

  it("reorders only the complete exact server ordering and uses one protected request", async () => {
    const reorder = vi.fn(async () => ({
      status: 200,
      data: {
        items: [
          { ...rawStage, id: 2, name: "第二" },
          { ...rawStage, id: 1, name: "第一" },
        ],
      },
    }));
    const client: StageTransport = {
      ...transport(),
      reorder,
    };
    await expect(submitStageReorder(client, [2, 1], "token", key)).resolves.toMatchObject({
      status: "reordered",
      items: [{ id: 2 }, { id: 1 }],
    });
    expect(client.reorder).toHaveBeenCalledWith(
      { ids: [2, 1] },
      { credentials: "same-origin", headers: { "X-CSRF-Token": "token", "Idempotency-Key": key } },
    );
    reorder.mockResolvedValue({
      status: 200,
      data: { items: [{ ...rawStage, id: 1 }, { ...rawStage, id: 2 }] },
    });
    await expect(submitStageReorder(client, [2, 1], "token", key)).resolves.toEqual({ status: "unavailable" });
  });

  it("archives only the requested exact stage and never sends when the generated operation is absent", async () => {
    await expect(submitStageArchive(transport(), 7, "token", key)).resolves.toEqual({ status: "unavailable" });
    const client: StageTransport = {
      ...transport(),
      archive: vi.fn(async () => ({ status: 200, data: rawStage })),
    };
    await expect(submitStageArchive(client, 7, "token", key)).resolves.toMatchObject({ status: "archived", stage: { id: 7 } });
    expect(client.archive).toHaveBeenCalledWith(
      7,
      { credentials: "same-origin", headers: { "X-CSRF-Token": "token", "Idempotency-Key": key } },
    );
  });
});
