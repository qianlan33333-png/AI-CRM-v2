import { describe, expect, it, vi } from "vitest";
import {
  GROUP_INVITE_PAGE_SIZE,
  loadGroupInviteLibrary,
  nextGroupInvitePage,
  parseGroupInviteLibraryItem,
  parseGroupInviteLibraryPage,
  previousGroupInvitePage,
  type GroupInviteLibraryTransport,
} from "./group-invite-library";

const invite = {
  id: 7,
  name: "体验群",
  title: "加入体验群",
  description: "点击卡片入群",
  join_url: "https://work.weixin.qq.com/gm/safe-token",
  cover_image_id: 19,
  enabled: true,
  created_by: 1,
  updated_by: 1,
  version: 1,
  created_at: "2026-08-19T08:00:00Z",
  updated_at: "2026-08-19T08:00:01Z",
};

function envelope(
  items: readonly Record<string, unknown>[] = [invite],
  total = items.length,
  offset = 0,
) {
  return {
    ok: true,
    items,
    group_invites: items,
    total,
    limit: GROUP_INVITE_PAGE_SIZE,
    offset,
    provider_call_executed: false,
  };
}

function transport(
  overrides: Partial<GroupInviteLibraryTransport> = {},
): GroupInviteLibraryTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  } as GroupInviteLibraryTransport;
}

describe("group-invite library local read contract", () => {
  it("accepts only the frozen local metadata and exact mirrored envelope", () => {
    expect(parseGroupInviteLibraryItem(invite)).toMatchObject({
      id: 7,
      title: "加入体验群",
      coverImageID: 19,
    });
    const zeroCover = parseGroupInviteLibraryItem({ ...invite, cover_image_id: 0 });
    expect(zeroCover).toMatchObject({ id: 7 });
    expect(zeroCover).not.toHaveProperty("coverImageID");
    expect(parseGroupInviteLibraryPage(envelope(), 0)).toMatchObject({
      total: 1,
      offset: 0,
    });
  });

  it.each([
    { ...invite, provider_state: "ready" },
    { ...invite, join_url: "https://outside.example/gm/safe" },
    { ...invite, join_url: "https://work.weixin.qq.com/gm/safe?target=outside" },
    { ...invite, join_url: "https://WORK.WEIXIN.QQ.COM/gm/safe" },
    { ...invite, archived_at: "2026-08-19T09:00:00Z" },
    { ...invite, cover_image_id: -1 },
    { ...invite, updated_at: "2026-08-19T07:59:59Z" },
  ])("rejects expanded, archived, or malformed local item %#", (value) => {
    expect(parseGroupInviteLibraryItem(value)).toBeUndefined();
  });

  it("rejects a non-mirrored, provider-marked, incomplete, or duplicate page", () => {
    const valid = envelope();
    for (const value of [
      { ...valid, provider_call_executed: true },
      { ...valid, group_invites: [{ ...invite, id: 8 }] },
      { ...valid, unexpected: true },
      envelope([invite], 2),
      envelope([invite, invite], 2),
    ]) {
      expect(parseGroupInviteLibraryPage(value, 0)).toBeUndefined();
    }
  });
});

describe("group-invite library local transport", () => {
  it("uses one fixed same-origin list GET without a detail or write", async () => {
    const client = transport({
      list: vi.fn(async () => ({ status: 200, data: envelope() })),
    });
    await expect(loadGroupInviteLibrary(client)).resolves.toMatchObject({
      status: "loaded",
    });
    expect(client.list).toHaveBeenCalledOnce();
    expect(client.list).toHaveBeenCalledWith(
      { limit: 100, offset: 0, enabled_only: false },
      { credentials: "same-origin" },
    );
  });

  it("fails closed for invalid offsets, response statuses, and network failure without retry", async () => {
    const client = transport({
      list: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: { ...envelope(), provider_call_executed: true } })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadGroupInviteLibrary(client, 1)).resolves.toEqual({ status: "invalid" });
    expect(client.list).not.toHaveBeenCalled();
    await expect(loadGroupInviteLibrary(client)).resolves.toEqual({ status: "invalid" });
    await expect(loadGroupInviteLibrary(client)).resolves.toEqual({ status: "unauthenticated" });
    await expect(loadGroupInviteLibrary(client)).resolves.toEqual({ status: "unavailable" });
  });

  it("calculates only bounded page transitions", () => {
    const page = parseGroupInviteLibraryPage(envelope([invite], 101, 100), 100);
    if (!page) throw new Error("expected page");
    expect(previousGroupInvitePage(page)).toBe(0);
    expect(nextGroupInvitePage(page)).toBeUndefined();
  });
});
