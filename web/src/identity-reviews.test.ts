import { describe, expect, it, vi } from "vitest";
import {
  appendIdentityMergeReviewPage,
  approveIdentityReview,
  loadIdentityMergeReviews,
  parseIdentityMergeReview,
  parseIdentityMergeReviewPage,
  rejectIdentityReview,
  type IdentityReviewTransport,
} from "./identity-reviews";

const fingerprint = `hmac-sha256-v1:${"A".repeat(22)}`;
const csrf = "c".repeat(43);

const pending = {
  review_id: 17,
  status: "pending",
  type: "phone",
  scope: "phone:e164",
  identity_fingerprint: fingerprint,
  customer_ids: [42, 84],
  version: 1,
  created_at: "2026-08-13T08:00:00Z",
  resolved_at: null,
};

const approved = {
  ...pending,
  status: "approved",
  version: 2,
  resolved_at: "2026-08-13T09:00:00Z",
};

const rejected = {
  ...pending,
  status: "rejected",
  version: 2,
  resolved_at: "2026-08-13T09:00:00Z",
};

function transport(
  overrides: Partial<IdentityReviewTransport> = {},
): IdentityReviewTransport {
  const unavailable = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(unavailable),
    approve: vi.fn(unavailable),
    reject: vi.fn(unavailable),
    ...overrides,
  } as IdentityReviewTransport;
}

describe("identity merge-review contract parsing", () => {
  it("accepts only the exact pending list facts and keeps the cursor opaque", () => {
    expect(parseIdentityMergeReview(pending)).toEqual({
      reviewID: 17,
      status: "pending",
      type: "phone",
      scope: "phone:e164",
      identityFingerprint: fingerprint,
      customerIDs: [42, 84],
      version: 1,
      createdAt: "2026-08-13T08:00:00Z",
    });
    expect(
      parseIdentityMergeReviewPage({
        items: [pending],
        next_cursor: "opaque=server&cursor",
      }),
    ).toMatchObject({ nextCursor: "opaque=server&cursor" });
  });

  it.each([
    { ...pending, raw_identity: "+8613800138000" },
    { ...pending, review_id: 0 },
    { ...pending, status: "unknown" },
    { ...pending, type: "openid" },
    { ...pending, scope: "" },
    { ...pending, identity_fingerprint: "sha256:raw" },
    { ...pending, customer_ids: [84, 42] },
    { ...pending, customer_ids: [42, 42] },
    { ...pending, customer_ids: [42, 84, 126] },
    { ...pending, version: 1.5 },
    { ...pending, created_at: "not-a-date" },
    { ...pending, resolved_at: "2026-08-13T09:00:00Z" },
    { ...approved, resolved_at: null },
    { ...approved, resolved_at: "2026-08-13T07:00:00Z" },
  ])("rejects malformed, expanded, or contradictory review %#", (value) => {
    expect(parseIdentityMergeReview(value)).toBeUndefined();
  });

  it("rejects non-pending list rows, duplicate ids, and impossible cursors", () => {
    expect(
      parseIdentityMergeReviewPage({ items: [approved], next_cursor: null }),
    ).toBeUndefined();
    expect(
      parseIdentityMergeReviewPage({
        items: [pending, pending],
        next_cursor: null,
      }),
    ).toBeUndefined();
    expect(
      parseIdentityMergeReviewPage({ items: [], next_cursor: "x".repeat(513) }),
    ).toBeUndefined();
    expect(
      parseIdentityMergeReviewPage({ items: [], next_cursor: null, raw: true }),
    ).toBeUndefined();
  });

  it("appends distinct server pages without interpreting cursors", () => {
    const first = parseIdentityMergeReviewPage({
      items: [pending],
      next_cursor: "server-only",
    });
    const second = parseIdentityMergeReviewPage({
      items: [{ ...pending, review_id: 18 }],
      next_cursor: null,
    });
    if (!first || !second) throw new Error("expected valid fixtures");
    expect(
      appendIdentityMergeReviewPage(first, second)?.items.map(
        ({ reviewID }) => reviewID,
      ),
    ).toEqual([17, 18]);
    expect(appendIdentityMergeReviewPage(first, first)).toBeUndefined();
  });
});

describe("identity merge-review typed transport", () => {
  it("loads through the generated client shape with same-origin credentials", async () => {
    const client = transport({
      list: vi.fn(async () => ({
        status: 200,
        data: { items: [pending], next_cursor: null },
      })),
    });
    await expect(
      loadIdentityMergeReviews(client, "opaque+cursor"),
    ).resolves.toMatchObject({
      status: "loaded",
    });
    expect(client.list).toHaveBeenCalledWith(
      { cursor: "opaque+cursor", limit: 50 },
      { credentials: "same-origin" },
    );
  });

  it("fails closed for invalid input, response bodies, statuses, and network errors", async () => {
    const client = transport({
      list: vi
        .fn()
        .mockResolvedValueOnce({
          status: 200,
          data: { items: [approved], next_cursor: null },
        })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadIdentityMergeReviews(client, "")).resolves.toEqual({
      status: "invalid",
    });
    expect(client.list).not.toHaveBeenCalled();
    await expect(loadIdentityMergeReviews(client)).resolves.toEqual({
      status: "invalid",
    });
    await expect(loadIdentityMergeReviews(client)).resolves.toEqual({
      status: "unauthenticated",
    });
    await expect(loadIdentityMergeReviews(client)).resolves.toEqual({
      status: "unavailable",
    });
  });

  it("approves with explicit primary, version, reason, CSRF, and stable command key", async () => {
    const client = transport({
      approve: vi.fn(async () => ({ status: 200, data: approved })),
    });
    const review = parseIdentityMergeReview(pending);
    if (!review) throw new Error("expected valid pending review");

    await expect(
      approveIdentityReview(
        client,
        review,
        42,
        "运营确认属于同一客户",
        csrf,
        "identity-review-command-17",
      ),
    ).resolves.toMatchObject({
      status: "completed",
      review: { status: "approved" },
    });
    expect(client.approve).toHaveBeenCalledWith(
      17,
      {
        expected_version: 1,
        primary_customer_id: 42,
        reason: "运营确认属于同一客户",
      },
      {
        credentials: "same-origin",
        headers: {
          "Idempotency-Key": "identity-review-command-17",
          "X-CSRF-Token": csrf,
        },
      },
    );
  });

  it("rejects without a primary and preserves the same frozen command headers", async () => {
    const client = transport({
      reject: vi.fn(async () => ({ status: 200, data: rejected })),
    });
    const review = parseIdentityMergeReview(pending);
    if (!review) throw new Error("expected valid pending review");

    await expect(
      rejectIdentityReview(
        client,
        review,
        "手机号已换主",
        csrf,
        "identity-review-reject-17",
      ),
    ).resolves.toMatchObject({
      status: "completed",
      review: { status: "rejected" },
    });
    expect(client.reject).toHaveBeenCalledWith(
      17,
      { expected_version: 1, reason: "手机号已换主" },
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.objectContaining({
          "Idempotency-Key": "identity-review-reject-17",
          "X-CSRF-Token": csrf,
        }),
      }),
    );
  });

  it("does not send malformed commands and rejects drifted success facts", async () => {
    const client = transport({
      approve: vi.fn(async () => ({
        status: 200,
        data: { ...approved, customer_ids: [42, 126] },
      })),
      reject: vi.fn(async () => ({ status: 409, data: {} })),
    });
    const review = parseIdentityMergeReview(pending);
    if (!review) throw new Error("expected valid pending review");

    await expect(
      approveIdentityReview(
        client,
        review,
        126,
        "reason",
        csrf,
        "command-key-is-long-enough",
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.approve).not.toHaveBeenCalled();
    await expect(
      approveIdentityReview(
        client,
        review,
        42,
        " reason ",
        csrf,
        "command-key-is-long-enough",
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.approve).not.toHaveBeenCalled();
    await expect(
      approveIdentityReview(
        client,
        review,
        42,
        "reason",
        "short",
        "command-key-is-long-enough",
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.approve).not.toHaveBeenCalled();

    await expect(
      approveIdentityReview(
        client,
        review,
        42,
        "reason",
        csrf,
        "command-key-is-long-enough",
      ),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      rejectIdentityReview(
        client,
        review,
        "reason",
        csrf,
        "command-key-is-long-enough",
      ),
    ).resolves.toEqual({ status: "conflict" });
  });
});
