import { describe, expect, it, vi } from "vitest";
import {
  IdentityReviewListController,
  appendIdentityMergeReviewPage,
  approveIdentityReview,
  loadIdentityMergeReviews,
  parseIdentityMergeReview,
  parseIdentityMergeReviewPage,
  rejectIdentityReview,
  type IdentityReviewTransport,
  type IdentityReviewTransportResponse,
} from "./identity-reviews";

const csrf = "c".repeat(43);

const pending = {
  review_id: 17,
  status: "pending",
  type: "phone",
  scope: "phone:e164",
  customer_ids: [42, 84],
  version: 1,
  created_at: "2026-08-13T08:00:00Z",
  resolved_at: null,
} as const;

const approved = {
  ...pending,
  status: "approved",
  version: 2,
  resolved_at: "2026-08-13T09:00:00Z",
} as const;

const rejected = {
  ...pending,
  status: "rejected",
  version: 2,
  resolved_at: "2026-08-13T09:00:00Z",
} as const;

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

function deferred<T>() {
  let resolve!: (...[]: [T]) => void;
  const promise = new Promise<T>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

describe("identity merge-review closed contract parsing", () => {
  it("accepts exact facts for all three statuses", () => {
    expect(parseIdentityMergeReview(pending)).toEqual({
      reviewID: 17,
      status: "pending",
      type: "phone",
      scope: "phone:e164",
      customerIDs: [42, 84],
      version: 1,
      createdAt: "2026-08-13T08:00:00Z",
    });
    expect(parseIdentityMergeReview(approved)).toMatchObject({
      status: "approved",
      resolvedAt: "2026-08-13T09:00:00Z",
    });
    expect(parseIdentityMergeReview(rejected)).toMatchObject({
      status: "rejected",
      resolvedAt: "2026-08-13T09:00:00Z",
    });
  });

  it.each([
    { ...pending, identity_fingerprint: "hmac-sha256-v1:secret" },
    { ...pending, normalized_value: "+8613800138000" },
    { ...pending, unionid: "raw-unionid" },
    { ...pending, external_userid: "raw-external-userid" },
    { ...pending, payload: { raw: true } },
    { ...pending, review_id: 0 },
    { ...pending, status: "unknown" },
    { ...pending, type: "openid" },
    { ...pending, scope: "" },
    { ...pending, customer_ids: [84, 42] },
    { ...pending, customer_ids: [42, 42] },
    { ...pending, customer_ids: [42, 84, 126] },
    { ...pending, version: 1.5 },
    { ...pending, created_at: "not-a-date" },
    { ...pending, resolved_at: "2026-08-13T09:00:00Z" },
    { ...approved, resolved_at: null },
    { ...approved, resolved_at: "2026-08-13T07:00:00Z" },
  ])("rejects expanded, malformed, or contradictory review %#", (value) => {
    expect(parseIdentityMergeReview(value)).toBeUndefined();
  });

  it("binds each decoded page to the requested status", () => {
    expect(
      parseIdentityMergeReviewPage(
        { items: [pending], next_cursor: "opaque=server&cursor" },
        "pending",
      ),
    ).toMatchObject({ status: "pending", nextCursor: "opaque=server&cursor" });
    expect(
      parseIdentityMergeReviewPage(
        { items: [approved], next_cursor: null },
        "pending",
      ),
    ).toBeUndefined();
    expect(
      parseIdentityMergeReviewPage(
        { items: [approved], next_cursor: null },
        "approved",
      ),
    ).toMatchObject({ status: "approved" });
    expect(
      parseIdentityMergeReviewPage(
        { items: [rejected], next_cursor: null },
        "rejected",
      ),
    ).toMatchObject({ status: "rejected" });
  });

  it("rejects duplicate ids, expanded pages, impossible cursors, and cross-status append", () => {
    expect(
      parseIdentityMergeReviewPage(
        { items: [pending, pending], next_cursor: null },
        "pending",
      ),
    ).toBeUndefined();
    expect(
      parseIdentityMergeReviewPage(
        { items: [], next_cursor: "x".repeat(513) },
        "pending",
      ),
    ).toBeUndefined();
    expect(
      parseIdentityMergeReviewPage(
        { items: [], next_cursor: null, raw: true },
        "pending",
      ),
    ).toBeUndefined();

    const first = parseIdentityMergeReviewPage(
      { items: [pending], next_cursor: "server-only" },
      "pending",
    );
    const second = parseIdentityMergeReviewPage(
      { items: [{ ...pending, review_id: 18 }], next_cursor: null },
      "pending",
    );
    const history = parseIdentityMergeReviewPage(
      { items: [approved], next_cursor: null },
      "approved",
    );
    if (!first || !second || !history) throw new Error("valid fixture drift");
    expect(
      appendIdentityMergeReviewPage(first, second)?.items.map(
        ({ reviewID }) => reviewID,
      ),
    ).toEqual([17, 18]);
    expect(appendIdentityMergeReviewPage(first, first)).toBeUndefined();
    expect(appendIdentityMergeReviewPage(first, history)).toBeUndefined();
  });
});

describe("identity merge-review typed transport", () => {
  it("sends status and opaque cursor with same-origin credentials", async () => {
    const client = transport({
      list: vi.fn(async () => ({
        status: 200,
        data: { items: [approved], next_cursor: null },
      })),
    });
    await expect(
      loadIdentityMergeReviews(client, {
        status: "approved",
        cursor: "opaque+cursor",
      }),
    ).resolves.toMatchObject({ status: "loaded" });
    expect(client.list).toHaveBeenCalledWith(
      { status: "approved", cursor: "opaque+cursor", limit: 50 },
      { credentials: "same-origin", signal: undefined },
    );
  });

  it("fails closed for invalid input, bodies, statuses, and network errors", async () => {
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
    await expect(
      loadIdentityMergeReviews(client, {
        status: "pending",
        cursor: "",
      }),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.list).not.toHaveBeenCalled();
    await expect(
      loadIdentityMergeReviews(client, { status: "pending" }),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      loadIdentityMergeReviews(client, { status: "pending" }),
    ).resolves.toEqual({ status: "unauthenticated" });
    await expect(
      loadIdentityMergeReviews(client, { status: "pending" }),
    ).resolves.toEqual({ status: "unavailable" });
  });

  it("keeps strict approve/reject confirmation without a fingerprint field", async () => {
    const client = transport({
      approve: vi.fn(async () => ({ status: 200, data: approved })),
      reject: vi.fn(async () => ({ status: 200, data: rejected })),
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

describe("IdentityReviewListController ownership", () => {
  it("singleflights the exact same request", async () => {
    const request = deferred<IdentityReviewTransportResponse>();
    const client = transport({ list: vi.fn(() => request.promise) });
    const controller = new IdentityReviewListController(client);
    const first = controller.activate("pending");
    const second = controller.activate("pending");
    expect(first).toBe(second);
    expect(client.list).toHaveBeenCalledTimes(1);
    request.resolve({
      status: 200,
      data: { items: [pending], next_cursor: null },
    });
    await first;
    expect(controller.snapshot().page?.items).toHaveLength(1);
  });

  it("lets only the latest status generation publish a late response", async () => {
    const pendingRequest = deferred<IdentityReviewTransportResponse>();
    const approvedRequest = deferred<IdentityReviewTransportResponse>();
    const client = transport({
      list: vi.fn((params) =>
        params.status === "pending"
          ? pendingRequest.promise
          : approvedRequest.promise,
      ),
    });
    const controller = new IdentityReviewListController(client);
    const old = controller.activate("pending");
    const latest = controller.activate("approved");

    pendingRequest.resolve({
      status: 200,
      data: { items: [pending], next_cursor: null },
    });
    await old;
    expect(controller.snapshot()).toMatchObject({
      activeStatus: "approved",
      loading: true,
    });
    expect(controller.snapshot().page).toBeUndefined();

    approvedRequest.resolve({
      status: 200,
      data: { items: [approved], next_cursor: null },
    });
    await latest;
    expect(controller.snapshot()).toMatchObject({
      activeStatus: "approved",
      loading: false,
      page: { status: "approved" },
    });
  });

  it("replaces pagination, ignores its late response, and retains verified data on failure", async () => {
    const more = deferred<IdentityReviewTransportResponse>();
    const refresh = deferred<IdentityReviewTransportResponse>();
    const client = transport({
      list: vi
        .fn()
        .mockResolvedValueOnce({
          status: 200,
          data: { items: [pending], next_cursor: "next" },
        })
        .mockImplementationOnce(() => more.promise)
        .mockImplementationOnce(() => refresh.promise)
        .mockResolvedValueOnce({ status: 503, data: {} }),
    });
    const controller = new IdentityReviewListController(client);
    await controller.activate("pending");
    const old = controller.loadMore();
    const latest = controller.refresh();
    more.resolve({
      status: 200,
      data: { items: [{ ...pending, review_id: 18 }], next_cursor: null },
    });
    await old;
    expect(
      controller.snapshot().page?.items.map(({ reviewID }) => reviewID),
    ).toEqual([17]);
    refresh.resolve({
      status: 200,
      data: { items: [{ ...pending, review_id: 19 }], next_cursor: null },
    });
    await latest;
    expect(
      controller.snapshot().page?.items.map(({ reviewID }) => reviewID),
    ).toEqual([19]);

    await controller.refresh();
    expect(controller.snapshot()).toMatchObject({
      failure: "unavailable",
      page: { items: [{ reviewID: 19 }] },
    });
  });

  it("notifies 401 once, invalidates history after resolution, and ignores unmount completions", async () => {
    const notify = vi.fn();
    const client = transport({
      list: vi.fn(async () => ({ status: 401, data: {} })),
    });
    const controller = new IdentityReviewListController(
      client,
      "pending",
      notify,
    );
    await controller.activate("pending");
    await controller.refresh();
    expect(notify).toHaveBeenCalledTimes(1);

    const successClient = transport({
      list: vi.fn(async (params) => ({
        status: 200,
        data: {
          items: [params.status === "pending" ? pending : approved],
          next_cursor: null,
        },
      })),
    });
    const success = new IdentityReviewListController(successClient);
    await success.activate("pending");
    await success.activate("approved");
    success.acceptResolution(parseIdentityMergeReview(approved)!);
    await success.activate("pending");
    expect(success.snapshot().page?.items).toHaveLength(1);
    success.acceptResolution(parseIdentityMergeReview(approved)!);
    expect(success.snapshot().page?.items).toHaveLength(0);
    await success.activate("approved");
    expect(successClient.list).toHaveBeenCalledTimes(4);

    const late = deferred<IdentityReviewTransportResponse>();
    const disposed = new IdentityReviewListController(
      transport({ list: vi.fn(() => late.promise) }),
    );
    const snapshots = vi.fn();
    disposed.subscribe(snapshots);
    const inFlight = disposed.activate("pending");
    disposed.dispose();
    const callsAtDispose = snapshots.mock.calls.length;
    late.resolve({
      status: 200,
      data: { items: [pending], next_cursor: null },
    });
    await inFlight;
    expect(snapshots).toHaveBeenCalledTimes(callsAtDispose);
  });
});
