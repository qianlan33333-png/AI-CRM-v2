import { afterEach, describe, expect, it, vi } from "vitest";
import {
  copyCoupon,
  filterCoupons,
  loadCoupons,
  newCouponCopyIdempotencyKey,
  type CouponsTransport,
} from "./coupons";
import { copyLegacyCoupon } from "./api/generated/health";

const sourceCoupon = {
  id: 7,
  name: "满减券",
  discount_amount_total: 100,
  currency: "CNY",
  status: "published",
  availability_status: "active",
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
  created_at: "2026-08-19T00:00:00Z",
  updated_at: "2026-08-19T01:02:03Z",
} as const;

function envelope(
  coupons: readonly Record<string, unknown>[] = [sourceCoupon],
  total = coupons.length,
  offset = 0,
) {
  return {
    ok: true,
    coupons,
    items: coupons,
    total,
    limit: 200,
    offset,
  };
}

function copiedEnvelope(extra: Record<string, unknown> = {}) {
  return {
    ok: true,
    coupon: {
      ...sourceCoupon,
      id: 8,
      name: "满减券 副本",
      status: "draft",
      availability_status: "draft",
      created_at: "2026-08-19T01:02:04Z",
      updated_at: "2026-08-19T01:02:04Z",
      ...extra,
    },
  };
}

function transport(
  listData: unknown = envelope(),
  copyData: unknown = copiedEnvelope(),
): CouponsTransport {
  return {
    list: vi.fn(async () => ({ status: 200, data: listData })),
    copy: vi.fn(async () => ({ status: 200, data: copyData })),
  } as unknown as CouponsTransport;
}

afterEach(() => vi.unstubAllGlobals());

describe("coupon list and copy local boundary", () => {
  it("uses the existing same-origin Orval APIs and an idempotency key only for the copy", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(envelope()), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(copiedEnvelope()), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);

    await expect(loadCoupons()).resolves.toMatchObject({ status: "loaded" });
    await expect(
      copyCoupon(
        {
          list: vi.fn(),
          copy: copyLegacyCoupon,
        } as unknown as CouponsTransport,
        7,
        "x".repeat(43),
        "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toMatchObject({ status: "copied", item: { id: 8 } });

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/admin/coupons?limit=200&offset=0",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/admin/coupons/7/copy",
      expect.objectContaining({
        credentials: "same-origin",
        method: "POST",
        headers: {
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key": "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
        },
      }),
    );
  });

  it("strictly decodes the complete local list before applying name and all seven availability filters", async () => {
    const statuses = [
      ["draft", "draft"],
      ["scheduled", "published"],
      ["active", "published"],
      ["sold_out", "published"],
      ["ended", "published"],
      ["stopped", "stopped"],
      ["archived", "archived"],
    ] as const;
    const rows = statuses.map(([availability_status, status], index) => ({
      ...sourceCoupon,
      id: index + 1,
      name: `${availability_status} coupon`,
      availability_status,
      status,
    }));
    const result = await loadCoupons(transport(envelope(rows)));
    if (result.status !== "loaded") throw new Error("fixture must load");
    expect(
      filterCoupons(result.items, "ACTIVE", "all").map((item) => item.id),
    ).toEqual([3]);
    for (const [availability] of statuses) {
      expect(
        filterCoupons(result.items, "", availability).map(
          (item) => item.availability,
        ),
      ).toEqual([availability]);
    }
  });

  it("fails closed for unknown response keys, non-seven-state availability, and incomplete pages", async () => {
    for (const data of [
      { ...envelope(), unexpected: true },
      envelope([{ ...sourceCoupon, availability_status: "deleted" }]),
      envelope([
        {
          ...sourceCoupon,
          target_refs: ["standard_product:7", "standard_product:7"],
        },
      ]),
      envelope([sourceCoupon], 2),
    ]) {
      await expect(loadCoupons(transport(data))).resolves.toEqual({
        status: "invalid",
      });
    }
  });

  it("requires a fresh valid key and strict fresh draft response without retries", async () => {
    const client = transport();
    await expect(
      copyCoupon(
        client,
        7,
        "not-a-43-character-base64url-csrf-token",
        "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.copy).not.toHaveBeenCalled();
    await expect(
      copyCoupon(client, 7, "x".repeat(43), "too-short"),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.copy).not.toHaveBeenCalled();

    await expect(
      copyCoupon(
        client,
        7,
        "x".repeat(43),
        "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({
      status: "copied",
      item: {
        id: 8,
        name: "满减券 副本",
        availability: "draft",
        createdAt: "2026-08-19T01:02:04Z",
        updatedAt: "2026-08-19T01:02:04Z",
      },
    });

    for (const body of [
      { ...copiedEnvelope(), unknown: true },
      copiedEnvelope({ id: 7 }),
      copiedEnvelope({ status: "published" }),
      copiedEnvelope({ issued_count: 1 }),
    ]) {
      await expect(
        copyCoupon(
          transport(envelope(), body),
          7,
          "x".repeat(43),
          "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
        ),
      ).resolves.toEqual({ status: "invalid" });
    }

    const unavailable = transport();
    vi.mocked(unavailable.copy).mockRejectedValue(new Error("network"));
    await expect(
      copyCoupon(
        unavailable,
        7,
        "x".repeat(43),
        "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "unavailable" });
    expect(unavailable.copy).toHaveBeenCalledTimes(1);
  });

  it("creates only a standards-valid 16-to-128 character browser key", () => {
    expect(
      newCouponCopyIdempotencyKey({
        randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
      }),
    ).toBe("coupon-copy:123e4567-e89b-42d3-a456-426614174000");
    expect(
      newCouponCopyIdempotencyKey({ randomUUID: () => "bad" }),
    ).toBeUndefined();
  });
});
