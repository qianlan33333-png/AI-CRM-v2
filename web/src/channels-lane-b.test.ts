import { describe, expect, it } from "vitest";
import {
  paginateChannels,
  type ChannelListItem,
} from "./channels";

const createdAt = "2026-08-19T00:00:00Z";
const updatedAt = "2026-08-19T01:02:03Z";

function item(id: number): ChannelListItem {
  return {
    id,
    name: `channel-name-${String(id).padStart(3, "0")}`,
    code: `channel-code-${String(id).padStart(3, "0")}`,
    status: "active",
    assigneeCount: 0,
    contactCount: 0,
    createdAt,
    updatedAt,
  };
}

describe("Lane B local channel pagination", () => {
  it("paginates the already-loaded closed local list without issuing another read", () => {
    const items = Array.from({ length: 45 }, (_, index) => item(index + 1));

    expect(paginateChannels(items, 1)).toMatchObject({
      page: 1,
      pageSize: 20,
      pageCount: 3,
      total: 45,
    });
    expect(paginateChannels(items, 1).items.map((value) => value.id)).toEqual(
      Array.from({ length: 20 }, (_, index) => index + 1),
    );
    expect(paginateChannels(items, 3).items.map((value) => value.id)).toEqual(
      [41, 42, 43, 44, 45],
    );
    expect(paginateChannels(items, 99)).toMatchObject({ page: 3 });
    expect(paginateChannels(items, Number.NaN, 0)).toMatchObject({
      page: 1,
      pageSize: 20,
    });
  });
});
