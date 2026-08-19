import { afterEach, describe, expect, it, vi } from "vitest";
import {
  archiveCoupon,
  canDeleteCoupon,
  canPublishCoupon,
  canStopCoupon,
  couponUpsertRequest,
  copyCoupon,
  createCouponDraft,
  deleteCoupon,
  filterCoupons,
  loadCouponClaims,
  loadCouponDetail,
  loadCouponProductOptions,
  loadCouponShare,
  loadCoupons,
  newCouponArchiveIdempotencyKey,
  newCouponCopyIdempotencyKey,
  newCouponDeleteIdempotencyKey,
  publishCoupon,
  stopCoupon,
  updateCouponDraft,
  type CouponsTransport,
} from "./coupons";
import {
  archiveLegacyCoupon,
  copyLegacyCoupon,
  createLegacyCoupon,
  deleteLegacyCoupon,
  getLegacyCoupon,
  getLegacyCouponShare,
  listLegacyCouponClaims,
  listLegacyCouponProductOptions,
  publishLegacyCoupon,
  stopLegacyCoupon,
  updateLegacyCoupon,
} from "./api/generated/health";

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

function archivedEnvelope(extra: Record<string, unknown> = {}) {
  return {
    ok: true,
    coupon: {
      ...sourceCoupon,
      status: "archived",
      availability_status: "archived",
      ...extra,
    },
  };
}

function transitionedEnvelope(
  status: "published" | "stopped",
  availability: "active" | "stopped",
  extra: Record<string, unknown> = {},
) {
  return {
    ok: true,
    coupon: {
      ...sourceCoupon,
      status,
      availability_status: availability,
      ...extra,
    },
    status,
    idempotent_same_state: true,
    fallback_used: false,
    real_external_call_executed: false,
  };
}

function deletedEnvelope(extra: Record<string, unknown> = {}) {
  return {
    ok: true,
    coupon: {
      ...sourceCoupon,
      status: "deleted",
      availability_status: "deleted",
      ...extra,
    },
  };
}

const sourceClaim = {
  id: 9,
  claim_ref: "cp_1234567890abcdef",
  status: "claimed",
  claimed_at: "2026-08-19T02:03:04Z",
} as const;

function claimsEnvelope(
  items: readonly Record<string, unknown>[] = [sourceClaim],
  total = items.length,
  offset = 0,
) {
  return { ok: true, items, total, limit: 50, offset };
}

const sourceProductOption = {
  id: 7,
  target_ref: "standard_product:7",
  name: "本地普通商品",
  price_minor: 9900,
  currency: "CNY",
} as const;

function productOptionsEnvelope(
  items: readonly Record<string, unknown>[] = [sourceProductOption],
  total = items.length,
  offset = 0,
) {
  return { ok: true, items, total, limit: 20, offset };
}

function shareEnvelope(extra: Record<string, unknown> = {}) {
  return { ok: true, public_slug: "c-7", url: "/c/c-7", ...extra };
}

function detailEnvelope(
  coupon: Record<string, unknown> = sourceCoupon,
  mirror: Record<string, unknown> = coupon,
) {
  return { ok: true, coupon, data: { coupon: mirror } };
}

function createdEnvelope(extra: Record<string, unknown> = {}) {
  const coupon = {
    ...sourceCoupon,
    id: 8,
    name: "新建本地草稿",
    instructions: "仅本地规则",
    target_refs: ["standard_product:7", "standard_product:8"],
    status: "draft",
    availability_status: "draft",
    issued_count: 0,
    created_at: "2026-08-19T01:02:04Z",
    updated_at: "2026-08-19T01:02:04Z",
    ...extra,
  };
  return {
    ok: true,
    coupon,
    coupon_id: coupon.id,
    fallback_used: false,
    create_replay_safe: false,
    real_external_call_executed: false,
  };
}

function updatedEnvelope(extra: Record<string, unknown> = {}) {
  return {
    ok: true,
    coupon: {
      ...sourceCoupon,
      name: "新建本地草稿",
      instructions: "仅本地规则",
      target_refs: ["standard_product:7", "standard_product:8"],
      status: "draft",
      availability_status: "draft",
      issued_count: 0,
      ...extra,
    },
    fallback_used: false,
    real_external_call_executed: false,
  };
}

const draftInput = {
  name: "新建本地草稿",
  discountAmountTotal: "100",
  totalIssueLimit: "20",
  perUserIssueLimit: "1",
  claimStartsAt: "2026-08-19T00:00:00Z",
  claimEndsAt: "2026-08-20T00:00:00Z",
  validityMode: "relative_days" as const,
  useStartsAt: "",
  useEndsAt: "",
  relativeValidityDays: "30",
  instructions: "仅本地规则",
  targetRefs: "standard_product:7\nstandard_product:8",
};

const draftDetail = {
  id: 7,
  name: "满减券",
  status: "draft" as const,
  availability: "draft" as const,
  issuedCount: 0,
  createdAt: "2026-08-19T00:00:00Z",
  updatedAt: "2026-08-19T01:02:03Z",
  discountAmountTotal: 100,
  currency: "CNY" as const,
  totalIssueLimit: 20,
  perUserIssueLimit: 1,
  claimStartsAt: "2026-08-19T00:00:00Z",
  claimEndsAt: "2026-08-20T00:00:00Z",
  validityMode: "relative_days" as const,
  relativeValidityDays: 30,
  instructions: "",
  targetRefs: ["standard_product:7"],
  createdBy: 1,
  updatedBy: 1,
  version: 1,
};

function transport(
  listData: unknown = envelope(),
  copyData: unknown = copiedEnvelope(),
  claimsData: unknown = claimsEnvelope(),
  shareData: unknown = shareEnvelope(),
  archiveData: unknown = archivedEnvelope(),
  detailData: unknown = detailEnvelope(),
  createData: unknown = createdEnvelope(),
  updateData: unknown = updatedEnvelope(),
  productOptionsData: unknown = productOptionsEnvelope(),
  publishData: unknown = transitionedEnvelope("published", "active"),
  stopData: unknown = transitionedEnvelope("stopped", "stopped"),
  deleteData: unknown = deletedEnvelope(),
): CouponsTransport {
  return {
    list: vi.fn(async () => ({ status: 200, data: listData })),
    copy: vi.fn(async () => ({ status: 200, data: copyData })),
    claims: vi.fn(async () => ({ status: 200, data: claimsData })),
    productOptions: vi.fn(async () => ({
      status: 200,
      data: productOptionsData,
    })),
    detail: vi.fn(async () => ({ status: 200, data: detailData })),
    create: vi.fn(async () => ({ status: 200, data: createData })),
    update: vi.fn(async () => ({ status: 200, data: updateData })),
    share: vi.fn(async () => ({ status: 200, data: shareData })),
    archive: vi.fn(async () => ({ status: 200, data: archiveData })),
    publish: vi.fn(async () => ({ status: 200, data: publishData })),
    stop: vi.fn(async () => ({ status: 200, data: stopData })),
    delete: vi.fn(async () => ({ status: 200, data: deleteData })),
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
        status: "draft",
        availability: "draft",
        issuedCount: 0,
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

  it("constructs only the frozen local draft DTO for both validity modes", () => {
    expect(couponUpsertRequest(draftInput)).toEqual({
      name: "新建本地草稿",
      discount_amount_total: 100,
      total_issue_limit: 20,
      per_user_issue_limit: 1,
      claim_starts_at: "2026-08-19T00:00:00Z",
      claim_ends_at: "2026-08-20T00:00:00Z",
      validity_mode: "relative_days",
      use_starts_at: null,
      use_ends_at: null,
      relative_validity_days: 30,
      instructions: "仅本地规则",
      target_refs: ["standard_product:7", "standard_product:8"],
    });
    expect(
      couponUpsertRequest({
        ...draftInput,
        validityMode: "fixed_range",
        useStartsAt: "2026-08-20T00:00:00Z",
        useEndsAt: "2026-08-21T00:00:00Z",
      }),
    ).toMatchObject({
      validity_mode: "fixed_range",
      use_starts_at: "2026-08-20T00:00:00Z",
      use_ends_at: "2026-08-21T00:00:00Z",
      relative_validity_days: null,
    });
    for (const input of [
      { ...draftInput, perUserIssueLimit: "21" },
      { ...draftInput, claimEndsAt: draftInput.claimStartsAt },
      { ...draftInput, relativeValidityDays: "36501" },
      { ...draftInput, targetRefs: "standard_product:7\nstandard_product:7" },
      { ...draftInput, targetRefs: "standard_product:07" },
      { ...draftInput, discountAmountTotal: "9007199254740992" },
      { ...draftInput, instructions: "x".repeat(201) },
    ]) {
      expect(couponUpsertRequest(input)).toBeUndefined();
    }
  });

  it("creates and updates one local draft through the generated same-origin APIs with CSRF only", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(createdEnvelope()), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(updatedEnvelope({ name: "已更新草稿" })), {
          status: 200,
        }),
      );
    vi.stubGlobal("fetch", fetch);
    const csrf = "x".repeat(43);

    await expect(
      createCouponDraft(
        { create: createLegacyCoupon } as unknown as CouponsTransport,
        draftInput,
        csrf,
      ),
    ).resolves.toMatchObject({
      status: "created",
      item: { id: 8, issuedCount: 0 },
    });
    await expect(
      updateCouponDraft(
        { update: updateLegacyCoupon } as unknown as CouponsTransport,
        draftDetail,
        { ...draftInput, name: "已更新草稿" },
        csrf,
      ),
    ).resolves.toMatchObject({
      status: "updated",
      item: { id: 7, name: "已更新草稿" },
    });

    const expected = {
      name: "新建本地草稿",
      discount_amount_total: 100,
      total_issue_limit: 20,
      per_user_issue_limit: 1,
      claim_starts_at: "2026-08-19T00:00:00Z",
      claim_ends_at: "2026-08-20T00:00:00Z",
      validity_mode: "relative_days",
      use_starts_at: null,
      use_ends_at: null,
      relative_validity_days: 30,
      instructions: "仅本地规则",
      target_refs: ["standard_product:7", "standard_product:8"],
    };
    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/admin/coupons",
      expect.objectContaining({
        credentials: "same-origin",
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": csrf,
        },
        body: JSON.stringify(expected),
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/admin/coupons/7",
      expect.objectContaining({
        credentials: "same-origin",
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": csrf,
        },
      }),
    );
    expect(fetch.mock.calls[0]?.[1]).not.toHaveProperty(
      "headers.Idempotency-Key",
    );
  });

  it("fails closed when a create or update receipt drifts from the submitted local rule", async () => {
    for (const response of [
      createdEnvelope({ discount_amount_total: 101 }),
      createdEnvelope({
        target_refs: ["standard_product:8", "standard_product:7"],
      }),
      createdEnvelope({ claim_ends_at: "2026-08-21T00:00:00Z" }),
    ]) {
      await expect(
        createCouponDraft(
          transport(
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            response,
          ),
          draftInput,
          "x".repeat(43),
        ),
      ).resolves.toEqual({ status: "invalid", outcomeUncertain: true });
    }

    for (const response of [
      updatedEnvelope({ per_user_issue_limit: 2 }),
      updatedEnvelope({ instructions: "服务端漂移" }),
      updatedEnvelope({ use_starts_at: "2026-08-20T00:00:00Z" }),
    ]) {
      await expect(
        updateCouponDraft(
          transport(
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            response,
          ),
          draftDetail,
          draftInput,
          "x".repeat(43),
        ),
      ).resolves.toEqual({ status: "invalid", outcomeUncertain: true });
    }
  });

  it("does not send invalid writes and freezes uncertain create or update outcomes without retry", async () => {
    const client = transport();
    await expect(createCouponDraft(client, draftInput, "bad")).resolves.toEqual(
      { status: "invalid", outcomeUncertain: false },
    );
    expect(client.create).not.toHaveBeenCalled();
    await expect(
      updateCouponDraft(
        client,
        { ...draftDetail, status: "published", availability: "active" },
        draftInput,
        "x".repeat(43),
      ),
    ).resolves.toEqual({ status: "invalid", outcomeUncertain: false });
    expect(client.update).not.toHaveBeenCalled();

    const malformed = transport(
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      { ok: true },
    );
    await expect(
      createCouponDraft(malformed, draftInput, "x".repeat(43)),
    ).resolves.toEqual({ status: "invalid", outcomeUncertain: true });
    expect(malformed.create).toHaveBeenCalledOnce();

    const unavailable = transport();
    vi.mocked(unavailable.update).mockRejectedValue(new Error("network"));
    await expect(
      updateCouponDraft(unavailable, draftDetail, draftInput, "x".repeat(43)),
    ).resolves.toEqual({ status: "unavailable", outcomeUncertain: true });
    expect(unavailable.update).toHaveBeenCalledOnce();
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
    expect(
      newCouponArchiveIdempotencyKey({
        randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
      }),
    ).toBe("coupon-archive:123e4567-e89b-42d3-a456-426614174000");
    expect(
      newCouponDeleteIdempotencyKey({
        randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
      }),
    ).toBe("coupon-delete:123e4567-e89b-42d3-a456-426614174000");
  });

  it("archives one eligible local coupon through the existing same-origin Orval endpoint", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify(archivedEnvelope()), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);
    const item = {
      id: 7,
      name: "满减券",
      status: "published" as const,
      availability: "active" as const,
      issuedCount: 0,
      createdAt: "2026-08-19T00:00:00Z",
      updatedAt: "2026-08-19T01:02:03Z",
    };

    await expect(
      archiveCoupon(
        {
          list: vi.fn(),
          copy: vi.fn(),
          archive: archiveLegacyCoupon,
        } as unknown as CouponsTransport,
        item,
        "x".repeat(43),
        "coupon-archive:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toMatchObject({ status: "archived", item: { id: 7 } });
    expect(fetch).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledWith(
      "/api/admin/coupons/7/archive",
      expect.objectContaining({
        credentials: "same-origin",
        method: "POST",
        headers: {
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key":
            "coupon-archive:123e4567-e89b-42d3-a456-426614174000",
        },
      }),
    );
  });

  it("fails closed for ineligible source, bad CSRF or key, and any non-identical archive response", async () => {
    const item = {
      id: 7,
      name: "满减券",
      status: "published" as const,
      availability: "active" as const,
      issuedCount: 0,
      createdAt: "2026-08-19T00:00:00Z",
      updatedAt: "2026-08-19T01:02:03Z",
    };
    const client = transport();
    await expect(
      archiveCoupon(
        client,
        { ...item, status: "archived" as const },
        "x".repeat(43),
        "coupon-archive:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      archiveCoupon(
        client,
        item,
        "invalid",
        "coupon-archive:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      archiveCoupon(client, item, "x".repeat(43), "too-short"),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.archive).not.toHaveBeenCalled();

    for (const body of [
      { ...archivedEnvelope(), unknown: true },
      archivedEnvelope({ id: 8 }),
      archivedEnvelope({ status: "published", availability_status: "active" }),
      archivedEnvelope({ issued_count: 1 }),
    ]) {
      await expect(
        archiveCoupon(
          transport(
            envelope(),
            copiedEnvelope(),
            claimsEnvelope(),
            shareEnvelope(),
            body,
          ),
          item,
          "x".repeat(43),
          "coupon-archive:123e4567-e89b-42d3-a456-426614174000",
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
    const unavailable = transport();
    vi.mocked(unavailable.archive).mockRejectedValue(new Error("network"));
    await expect(
      archiveCoupon(
        unavailable,
        item,
        "x".repeat(43),
        "coupon-archive:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "unavailable" });
    expect(unavailable.archive).toHaveBeenCalledOnce();
  });

  it("uses the closed local-only publish, stop, and delete endpoints with their actual CSRF and key contracts", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify(transitionedEnvelope("published", "active")),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify(transitionedEnvelope("stopped", "stopped")),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(deletedEnvelope()), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);
    const draft = { ...draftDetail };
    const published = {
      ...draft,
      status: "published" as const,
      availability: "active" as const,
    };

    await expect(
      publishCoupon(
        { publish: publishLegacyCoupon } as CouponsTransport,
        draft,
        "x".repeat(43),
      ),
    ).resolves.toMatchObject({ status: "published", item: { id: 7 } });
    await expect(
      stopCoupon(
        { stop: stopLegacyCoupon } as CouponsTransport,
        published,
        "x".repeat(43),
      ),
    ).resolves.toMatchObject({ status: "stopped", item: { id: 7 } });
    await expect(
      deleteCoupon(
        { delete: deleteLegacyCoupon } as CouponsTransport,
        draft,
        "x".repeat(43),
        "coupon-delete:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toMatchObject({ status: "deleted", item: { id: 7 } });

    expect(fetch.mock.calls).toEqual([
      [
        "/api/admin/coupons/7/publish",
        expect.objectContaining({
          credentials: "same-origin",
          method: "POST",
          headers: { "X-CSRF-Token": "x".repeat(43) },
        }),
      ],
      [
        "/api/admin/coupons/7/stop",
        expect.objectContaining({
          credentials: "same-origin",
          method: "POST",
          headers: { "X-CSRF-Token": "x".repeat(43) },
        }),
      ],
      [
        "/api/admin/coupons/7",
        expect.objectContaining({
          credentials: "same-origin",
          method: "DELETE",
          headers: {
            "X-CSRF-Token": "x".repeat(43),
            "Idempotency-Key":
              "coupon-delete:123e4567-e89b-42d3-a456-426614174000",
          },
        }),
      ],
    ]);
  });

  it("allows only eligible local states and fails closed for receipt drift or unknown outcomes", async () => {
    const draft = { ...draftDetail };
    const published = {
      ...draft,
      status: "published" as const,
      availability: "active" as const,
    };
    expect(canPublishCoupon(draft)).toBe(true);
    expect(canStopCoupon(published)).toBe(true);
    expect(canDeleteCoupon(draft)).toBe(true);
    expect(canDeleteCoupon({ ...draft, issuedCount: 1 })).toBe(false);

    const client = transport(
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      { ...transitionedEnvelope("published", "active"), status: "stopped" },
      transitionedEnvelope("stopped", "stopped", { issued_count: 1 }),
      { ...deletedEnvelope(), coupon: { ...deletedEnvelope().coupon, id: 8 } },
    );
    await expect(publishCoupon(client, draft, "x".repeat(43))).resolves.toEqual(
      {
        status: "invalid",
        outcomeUncertain: true,
      },
    );
    await expect(
      stopCoupon(client, published, "x".repeat(43)),
    ).resolves.toEqual({
      status: "invalid",
      outcomeUncertain: true,
    });
    await expect(
      deleteCoupon(
        client,
        draft,
        "x".repeat(43),
        "coupon-delete:123e4567-e89b-42d3-a456-426614174000",
      ),
    ).resolves.toEqual({ status: "invalid", outcomeUncertain: true });

    const invalid = transport();
    await expect(
      publishCoupon(invalid, published, "x".repeat(43)),
    ).resolves.toEqual({
      status: "invalid",
      outcomeUncertain: false,
    });
    await expect(stopCoupon(invalid, draft, "bad")).resolves.toEqual({
      status: "invalid",
      outcomeUncertain: false,
    });
    await expect(
      deleteCoupon(
        invalid,
        { ...draft, issuedCount: 1 },
        "x".repeat(43),
        "bad",
      ),
    ).resolves.toEqual({
      status: "invalid",
      outcomeUncertain: false,
    });
    expect(invalid.publish).not.toHaveBeenCalled();
    expect(invalid.stop).not.toHaveBeenCalled();
    expect(invalid.delete).not.toHaveBeenCalled();

    const unavailable = transport();
    vi.mocked(unavailable.publish).mockRejectedValue(new Error("network"));
    await expect(
      publishCoupon(unavailable, draft, "x".repeat(43)),
    ).resolves.toEqual({ status: "unavailable", outcomeUncertain: true });
    expect(unavailable.publish).toHaveBeenCalledOnce();
  });

  it("reads one same-origin, fixed-size opaque claim page without a write", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify(claimsEnvelope([], 0)), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);

    await expect(
      loadCouponClaims(
        {
          list: vi.fn(),
          copy: vi.fn(),
          claims: listLegacyCouponClaims,
        } as unknown as CouponsTransport,
        7,
        0,
      ),
    ).resolves.toEqual({ status: "loaded", items: [], total: 0, offset: 0 });
    expect(fetch).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledWith(
      "/api/admin/coupons/7/claims?limit=50&offset=0",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
  });

  it("fails closed for a non-opaque claim DTO, a bad page, and a transport failure without retry", async () => {
    for (const data of [
      claimsEnvelope([{ ...sourceClaim, customer_id: 7 }]),
      claimsEnvelope([{ ...sourceClaim, claim_ref: "cp_short" }]),
      claimsEnvelope([{ ...sourceClaim, status: "redeemed" }]),
      claimsEnvelope([{ ...sourceClaim, claimed_at: "not-a-time" }]),
      claimsEnvelope([sourceClaim, sourceClaim]),
      claimsEnvelope([sourceClaim], 0),
      { ...claimsEnvelope(), unexpected: true },
    ]) {
      await expect(
        loadCouponClaims(transport(envelope(), copiedEnvelope(), data), 7, 0),
      ).resolves.toEqual({
        status: "invalid",
      });
    }
    const unavailable = transport();
    vi.mocked(unavailable.claims).mockRejectedValue(new Error("network"));
    await expect(loadCouponClaims(unavailable, 7, 0)).resolves.toEqual({
      status: "unavailable",
    });
    expect(unavailable.claims).toHaveBeenCalledOnce();

    const invalidOffset = transport();
    await expect(loadCouponClaims(invalidOffset, 7, 25)).resolves.toEqual({
      status: "invalid",
    });
    expect(invalidOffset.claims).not.toHaveBeenCalled();
  });

  it("reads exactly one same-origin standard-product page without a query or a write", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(productOptionsEnvelope([], 0)), {
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetch);

    await expect(
      loadCouponProductOptions(
        { productOptions: listLegacyCouponProductOptions } as CouponsTransport,
        0,
      ),
    ).resolves.toEqual({ status: "loaded", items: [], total: 0, offset: 0 });
    expect(fetch).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledWith(
      "/api/admin/coupons/product-options?product_type=standard_product&limit=20&offset=0",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
  });

  it("fails closed for product-option envelope, item, duplicate, and page drift without retry", async () => {
    for (const data of [
      { ...productOptionsEnvelope(), unexpected: true },
      productOptionsEnvelope([{ ...sourceProductOption, id: 0 }]),
      productOptionsEnvelope([
        { ...sourceProductOption, target_ref: "standard_product:8" },
      ]),
      productOptionsEnvelope([{ ...sourceProductOption, currency: "USD" }]),
      productOptionsEnvelope([{ ...sourceProductOption, price_minor: -1 }]),
      productOptionsEnvelope([
        { ...sourceProductOption, price_minor: 9007199254740992 },
      ]),
      productOptionsEnvelope(
        [sourceProductOption, { ...sourceProductOption, name: "重复商品" }],
        2,
      ),
      productOptionsEnvelope([sourceProductOption], 2),
      productOptionsEnvelope([], 1),
    ]) {
      await expect(
        loadCouponProductOptions(
          transport(
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            data,
          ),
          0,
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
    const unavailable = transport();
    vi.mocked(unavailable.productOptions).mockRejectedValue(
      new Error("network"),
    );
    await expect(loadCouponProductOptions(unavailable, 0)).resolves.toEqual({
      status: "unavailable",
    });
    expect(unavailable.productOptions).toHaveBeenCalledOnce();

    const invalidOffset = transport();
    await expect(loadCouponProductOptions(invalidOffset, 1)).resolves.toEqual({
      status: "invalid",
    });
    expect(invalidOffset.productOptions).not.toHaveBeenCalled();
  });

  it("reads one same-origin coupon rule detail through the generated GET only", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify(detailEnvelope()), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);

    await expect(
      loadCouponDetail(
        {
          list: vi.fn(),
          copy: vi.fn(),
          detail: getLegacyCoupon,
        } as unknown as CouponsTransport,
        7,
      ),
    ).resolves.toMatchObject({
      status: "loaded",
      detail: {
        id: 7,
        discountAmountTotal: 100,
        currency: "CNY",
        targetRefs: ["standard_product:7"],
      },
    });
    expect(fetch).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledWith(
      "/api/admin/coupons/7",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
  });

  it("fails closed when either detail mirror, its requested ID, or its DTO differs", async () => {
    const client = transport();
    await expect(loadCouponDetail(client, 0)).resolves.toEqual({
      status: "invalid",
    });
    expect(client.detail).not.toHaveBeenCalled();

    for (const body of [
      { ok: true, coupon: sourceCoupon },
      { ok: true, coupon: sourceCoupon, data: {} },
      { ...detailEnvelope(), unexpected: true },
      {
        ok: true,
        coupon: sourceCoupon,
        data: { coupon: sourceCoupon, extra: true },
      },
      detailEnvelope({ ...sourceCoupon, id: 8 }),
      detailEnvelope(sourceCoupon, { ...sourceCoupon, issued_count: 1 }),
      detailEnvelope(sourceCoupon, {
        ...sourceCoupon,
        target_refs: ["standard_product:8"],
      }),
      detailEnvelope({ ...sourceCoupon, instructions: "bad\x00value" }),
    ]) {
      await expect(
        loadCouponDetail(
          transport(
            undefined,
            undefined,
            undefined,
            undefined,
            undefined,
            body,
          ),
          7,
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
    const unavailable = transport();
    vi.mocked(unavailable.detail).mockRejectedValue(new Error("network"));
    await expect(loadCouponDetail(unavailable, 7)).resolves.toEqual({
      status: "unavailable",
    });
    expect(unavailable.detail).toHaveBeenCalledOnce();
  });

  it("reads exactly one same-origin local share URL for a published coupon", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify(shareEnvelope()), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);
    const item = {
      id: 7,
      name: "满减券",
      status: "published" as const,
      availability: "active" as const,
      issuedCount: 0,
      createdAt: "2026-08-19T00:00:00Z",
      updatedAt: "2026-08-19T01:02:03Z",
    };
    await expect(
      loadCouponShare(
        {
          list: vi.fn(),
          copy: vi.fn(),
          claims: vi.fn(),
          share: getLegacyCouponShare,
        } as unknown as CouponsTransport,
        item,
      ),
    ).resolves.toEqual({
      status: "loaded",
      share: { publicSlug: "c-7", url: "/c/c-7" },
    });
    expect(fetch).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledWith(
      "/api/admin/coupons/7/share",
      expect.objectContaining({ credentials: "same-origin", method: "GET" }),
    );
  });

  it("rejects non-published coupons and every non-local share response without retry", async () => {
    const published = {
      id: 7,
      name: "满减券",
      status: "published" as const,
      availability: "active" as const,
      issuedCount: 0,
      createdAt: "2026-08-19T00:00:00Z",
      updatedAt: "2026-08-19T01:02:03Z",
    };
    const draft = { ...published, status: "draft" as const };
    const client = transport();
    await expect(loadCouponShare(client, draft)).resolves.toEqual({
      status: "invalid",
    });
    expect(client.share).not.toHaveBeenCalled();

    for (const response of [
      shareEnvelope({ url: "https://outside.example/c/c-7" }),
      shareEnvelope({ public_slug: "c-8" }),
      shareEnvelope({ unexpected: true }),
    ]) {
      await expect(
        loadCouponShare(
          transport(envelope(), copiedEnvelope(), claimsEnvelope(), response),
          published,
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
    const unavailable = transport();
    vi.mocked(unavailable.share).mockRejectedValue(new Error("network"));
    await expect(loadCouponShare(unavailable, published)).resolves.toEqual({
      status: "unavailable",
    });
    expect(unavailable.share).toHaveBeenCalledOnce();
  });
});
