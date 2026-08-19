import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  CouponsPage,
  CouponsView,
  performCouponCopy,
  startCouponCopy,
} from "./coupons-ui";
import type { CouponsTransport } from "./coupons";

const item = {
  id: 7,
  name: '<img src=x onerror="bad">',
  availability: "active" as const,
  createdAt: "2026-08-19T00:00:00Z",
  updatedAt: "2026-08-19T01:02:03Z",
};

function transport(): CouponsTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    copy: vi.fn(async () => ({ status: 503, data: {} })),
  } as unknown as CouponsTransport;
}

describe("CouponsView", () => {
  it.each(["admin", "ops"] as const)(
    "renders the local list and copy action for %s only",
    (role) => {
      const html = renderToStaticMarkup(
        <CouponsView
          onCopy={vi.fn()}
          role={role}
          state={{ kind: "ready", items: [item] }}
        />,
      );
      expect(html).toContain('<h1 id="app-title">优惠券列表</h1>');
      expect(html).toContain("搜索优惠券名称");
      expect(html).toContain("可用状态");
      for (const status of [
        "draft",
        "scheduled",
        "active",
        "sold_out",
        "ended",
        "stopped",
        "archived",
      ]) {
        expect(html).toContain(`value="${status}"`);
      }
      expect(html).toContain("复制只会创建新的本地草稿");
      expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
      expect(html).not.toContain("<img");
      expect(html).not.toMatch(/payment|provider|claim|redeem|share/i);
    },
  );

  it("fails closed for sales without issuing either read or write", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <CouponsPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有优惠券管理权限。");
    expect(html).not.toContain("正在读取本地优惠券列表。");
    expect(html).not.toContain(">复制<");
    expect(client.list).not.toHaveBeenCalled();
    expect(client.copy).not.toHaveBeenCalled();
  });

  it("makes one same-click copy request with the CSRF cookie and no redirect", async () => {
    const client = transport();
    vi.mocked(client.copy).mockResolvedValue({
      status: 200,
      data: {
        ok: true,
        coupon: {
          id: 8,
          name: "副本",
          discount_amount_total: 100,
          currency: "CNY",
          status: "draft",
          availability_status: "draft",
          total_issue_limit: 20,
          per_user_issue_limit: 1,
          issued_count: 0,
          claim_starts_at: "2026-08-19T00:00:00Z",
          claim_ends_at: "2026-08-20T00:00:00Z",
          validity_mode: "relative_days",
          use_starts_at: null,
          use_ends_at: null,
          relative_validity_days: 30,
          instructions: "",
          target_refs: ["standard_product:7"],
          created_by: 1,
          updated_by: 1,
          version: 1,
          created_at: "2026-08-19T01:02:04Z",
          updated_at: "2026-08-19T01:02:04Z",
        },
      },
    } as never);

    await expect(
      performCouponCopy({
        couponID: item.id,
        idempotencySource: {
          randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
        },
        readCookie: () => `aicrm_csrf=${"x".repeat(43)}`,
        transport: client,
      }),
    ).resolves.toMatchObject({ status: "copied", item: { id: 8 } });
    expect(client.copy).toHaveBeenCalledTimes(1);
    expect(client.copy).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key": "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
        },
      }),
    );
  });

  it("allows only one copy when two clicks arrive in the same render tick", async () => {
    const lock = { current: false };
    const client = transport();
    const execute = vi.fn(async () => {
      await performCouponCopy({
        couponID: item.id,
        idempotencySource: {
          randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
        },
        readCookie: () => `aicrm_csrf=${"x".repeat(43)}`,
        transport: client,
      });
    });

    const first = startCouponCopy(lock, execute);
    const second = startCouponCopy(lock, execute);
    expect(first).toBeInstanceOf(Promise);
    expect(second).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();
    expect(client.copy).toHaveBeenCalledOnce();
    expect(lock.current).toBe(true);

    await first;
    expect(lock.current).toBe(false);
  });

  it("releases the single-flight lock when the copy flow throws", async () => {
    const lock = { current: false };
    await expect(
      startCouponCopy(lock, async () => {
        throw new Error("local test failure");
      }),
    ).rejects.toThrow("local test failure");
    expect(lock.current).toBe(false);
  });
});
