import { describe, expect, it, vi } from "vitest";
import {
  buildDefinition,
  editorDraft,
  loadSegmentMembers,
  loadSegments,
  parseSegment,
  refreshSegment,
  saveSegment,
  type SegmentTransport,
} from "./segments";

const segment = {
  id: 17,
  name: "近期活跃客户",
  definition: { and: [{ field: "stage_id", op: "eq", value: 3 }] },
  refresh_mode: "manual",
  refresh_cron: null,
  member_count: 2,
  refreshed_at: "2026-08-13T08:00:00Z",
  refresh_status: "idle",
  created_at: "2026-08-12T08:00:00Z",
  updated_at: "2026-08-13T08:00:00Z",
};
const customer = {
  id: 9,
  name: "陈晨",
  is_deleted: false,
  extra: {},
  created_at: "2026-08-12T08:00:00Z",
  updated_at: "2026-08-12T08:00:00Z",
};

function transport(response: { status: number; data: unknown } = { status: 200, data: { items: [segment], next_cursor: null } }): SegmentTransport {
  const result = async () => response;
  return { list: vi.fn(result), create: vi.fn(result), update: vi.fn(result), members: vi.fn(result), refresh: vi.fn(result) } as unknown as SegmentTransport;
}

describe("segment response boundary", () => {
  it("parses only an exact frozen Segment shape", () => {
    expect(parseSegment(segment)).toMatchObject({ id: 17, name: "近期活跃客户", memberCount: 2 });
    expect(parseSegment({ ...segment, extra: true })).toBeUndefined();
    expect(parseSegment({ ...segment, definition: { field: "raw_sql", op: "eq", value: 1 } })).toBeUndefined();
    expect(parseSegment({ ...segment, member_count: -1 })).toBeUndefined();
  });

  it("loads opaque cursors without parsing them and rejects malformed pages", async () => {
    const client = transport();
    await expect(loadSegments(client, "opaque-next-page")).resolves.toEqual({ status: "loaded", items: [parseSegment(segment)] });
    expect(client.list).toHaveBeenCalledWith({ limit: 50, cursor: "opaque-next-page" }, { credentials: "same-origin" });
    await expect(loadSegments(transport({ status: 200, data: { items: [segment], next_cursor: "" } }))).resolves.toEqual({ status: "unavailable" });
  });

  it("loads materialized members through the typed transport", async () => {
    const client = transport({ status: 200, data: { items: [customer], next_cursor: "opaque-members" } });
    await expect(loadSegmentMembers(client, 17)).resolves.toMatchObject({ status: "loaded", items: [{ id: 9 }], nextCursor: "opaque-members" });
    expect(client.members).toHaveBeenCalledWith(17, { limit: 50 }, { credentials: "same-origin" });
  });
});

describe("closed condition editor", () => {
  it("builds only frozen DSL predicates and groups", () => {
    expect(buildDefinition({ kind: "predicate", field: "tag_id", operator: "has_any", value: "2, 8" })).toEqual({ ok: true, definition: { field: "tag_id", op: "has_any", value: [2, 8] } });
    expect(buildDefinition({ kind: "group", combinator: "or", children: [{ kind: "predicate", field: "is_deleted", operator: "eq", value: "false" }, { kind: "predicate", field: "added_at", operator: "after", value: "-30d" }] })).toMatchObject({ ok: true, definition: { or: expect.any(Array) } });
  });

  it.each([
    { kind: "predicate", field: "stage_id", operator: "eq", value: "1; DROP TABLE segments" },
    { kind: "predicate", field: "tag_id", operator: "eq", value: "1" },
    { kind: "predicate", field: "is_deleted", operator: "eq", value: "raw_json" },
  ] as const)("rejects a non-frozen input %#", (condition) => {
    expect(buildDefinition(condition)).toEqual(expect.objectContaining({ ok: false }));
  });
});

describe("segment mutations", () => {
  it("uses CSRF, a stable idempotency key, and same-origin credentials for create", async () => {
    const client = transport({ status: 201, data: segment });
    await expect(saveSegment(client, undefined, editorDraft(), "csrf-token", "create-key")).resolves.toEqual({ status: "invalid" });
    const draft = { ...editorDraft(), name: "近期活跃客户", condition: { kind: "predicate" as const, field: "stage_id" as const, operator: "eq" as const, value: "3" } };
    await expect(saveSegment(client, undefined, draft, "csrf-token", "create-key")).resolves.toMatchObject({ status: "saved", segment: { id: 17 } });
    expect(client.create).toHaveBeenCalledWith({ name: "近期活跃客户", definition: { field: "stage_id", op: "eq", value: 3 }, refresh_mode: "manual", refresh_cron: null }, { credentials: "same-origin", headers: { "X-CSRF-Token": "csrf-token", "Idempotency-Key": "create-key" } });
  });

  it("updates only the selected Segment and treats a mismatched success response as unavailable", async () => {
    const existing = parseSegment(segment)!;
    const draft = { ...editorDraft(existing), name: "新的名称", condition: { kind: "predicate" as const, field: "stage_id" as const, operator: "eq" as const, value: "3" } };
    const client = transport({ status: 200, data: { ...segment, name: "新的名称" } });
    await expect(saveSegment(client, existing, draft, "csrf-token", "update-key")).resolves.toMatchObject({ status: "saved", segment: { name: "新的名称" } });
    expect(client.update).toHaveBeenCalledWith(17, expect.objectContaining({ name: "新的名称" }), expect.objectContaining({ credentials: "same-origin" }));
    await expect(saveSegment(transport({ status: 200, data: { ...segment, id: 18 } }), existing, draft, "csrf-token", "update-key")).resolves.toEqual({ status: "unavailable" });
  });

  it("accepts refresh only for the selected Segment and does not infer member completion", async () => {
    const client = transport({ status: 202, data: { status: "accepted", segment_id: 17 } });
    await expect(refreshSegment(client, 17, "csrf-token", "refresh-key")).resolves.toBe("accepted");
    expect(client.refresh).toHaveBeenCalledWith(17, { credentials: "same-origin", headers: { "X-CSRF-Token": "csrf-token", "Idempotency-Key": "refresh-key" } });
    await expect(refreshSegment(transport({ status: 202, data: { status: "accepted", segment_id: 18 } }), 17, "csrf-token", "refresh-key")).resolves.toBe("unavailable");
  });
});
