import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  CustomerDetailPage,
  parseProfileDraft,
  startCustomerMutation,
} from "./customer-detail-ui";
import type {
  CustomerDetailSnapshot,
  CustomerDetailTransport,
} from "./customer-detail";

const snapshot: CustomerDetailSnapshot = {
  customer: {
    id: 7,
    name: "林小姐",
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
  tags: [{ id: 9, groupName: "意向", name: "已报名", sortOrder: 10 }],
  tagCatalog: [
    { id: 9, groupName: "意向", name: "已报名", sortOrder: 10 },
    { id: 10, groupName: "意向", name: "待联系", sortOrder: 20 },
  ],
  events: [
    {
      id: 12,
      eventType: "stage_changed",
      actor: "后台账号 #1",
      occurredAt: "2026-08-12T03:00:00Z",
    },
  ],
  eventsHaveMore: true,
};

function transport(): CustomerDetailTransport {
  return {
    get: vi.fn(),
    update: vi.fn(),
    setStage: vi.fn(),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    listEvents: vi.fn(),
    listTags: vi.fn(),
  } as unknown as CustomerDetailTransport;
}

describe("CustomerDetailPage", () => {
  it("renders accessible detail, profile, stage, tag, and timeline controls", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={snapshot}
        transport={transport()}
      />,
    );
    expect(html).toContain('<h1 id="app-title">客户详情</h1>');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
    expect(html).toContain("资料操作");
    expect(html).toContain("阶段编号");
    expect(html).toContain("添加标签");
    expect(html).toContain("时间线");
    expect(html).toContain("后台账号 #1");
    expect(html).toContain("仅展示最近 50 条，更多记录待后续加载。");
    expect(html).toContain("<fieldset");
    expect(html).toContain("<label");
    expect(html).not.toContain("aicrm_csrf");
    expect(html).not.toContain("X-CSRF-Token");
  });

  it("starts with an accessible loading status without requiring browser globals", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage customerID={7} transport={transport()} />,
    );
    expect(html).toContain("正在读取客户资料、标签和时间线");
    expect(html).toContain('role="status"');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
  });

  it.each([
    "javascript:alert(1)",
    "data:text/plain,unsafe",
    "ftp://assets.invalid/a",
    "https:assets.invalid/a",
    "https://name:secret@assets.invalid/a",
  ])("rejects an unsafe profile avatar URL %s", (avatarURL) => {
    expect(
      parseProfileDraft({
        name: "林小姐",
        avatarURL,
        gender: "1",
        ownerStaffID: "11",
        channelID: "5",
      }),
    ).toBeUndefined();
  });

  it("rejects an out-of-range profile gender before transport", () => {
    expect(
      parseProfileDraft({
        name: "林小姐",
        avatarURL: "https://assets.invalid/avatar.png",
        gender: "32768",
        ownerStaffID: "11",
        channelID: "5",
      }),
    ).toBeUndefined();
  });

  it("renders empty tags and timeline as explicit server-backed states", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={{
          ...snapshot,
          tags: [],
          events: [],
          eventsHaveMore: false,
        }}
        transport={transport()}
      />,
    );
    expect(html).toContain("暂无标签。");
    expect(html).toContain("暂无时间线记录。");
    expect(html).not.toContain("更多记录待后续加载");
  });
});

describe("customer mutation orchestration", () => {
  it("locks synchronously, rejects a duplicate, then refetches after success", async () => {
    const lock = { current: false };
    let release: () => void = () => {};
    const execute = vi.fn(
      () =>
        new Promise<{ status: "succeeded" }>((resolve) => {
          release = () => resolve({ status: "succeeded" });
        }),
    );
    const refetch = vi.fn(
      async () => ({ status: "loaded", snapshot }) as const,
    );

    const first = startCustomerMutation(lock, execute, refetch);
    expect(first).toBeInstanceOf(Promise);
    expect(lock.current).toBe(true);
    expect(startCustomerMutation(lock, execute, refetch)).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();

    release();
    await expect(first).resolves.toEqual({ status: "confirmed", snapshot });
    expect(refetch).toHaveBeenCalledOnce();
    expect(lock.current).toBe(false);
  });

  it("does not refetch a rejected write and keeps failed refetch distinct", async () => {
    const failedRefetch = vi.fn(
      async () => ({ status: "unavailable" }) as const,
    );
    await expect(
      startCustomerMutation(
        { current: false },
        async () => ({ status: "forbidden" }),
        failedRefetch,
      ),
    ).resolves.toEqual({ status: "mutation_failed", failure: "forbidden" });
    expect(failedRefetch).not.toHaveBeenCalled();

    await expect(
      startCustomerMutation(
        { current: false },
        async () => ({ status: "succeeded" }),
        failedRefetch,
      ),
    ).resolves.toEqual({ status: "unconfirmed", failure: "unavailable" });
  });

  it("releases the lock and fails closed when an operation throws", async () => {
    const lock = { current: false };
    await expect(
      startCustomerMutation(
        lock,
        async () => {
          throw new Error("sensitive transport detail");
        },
        vi.fn(),
      ),
    ).resolves.toEqual({ status: "mutation_failed", failure: "unavailable" });
    expect(lock.current).toBe(false);
  });
});
