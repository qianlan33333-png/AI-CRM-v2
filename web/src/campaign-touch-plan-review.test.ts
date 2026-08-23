import { describe, expect, it, vi } from "vitest";
import type { SessionStorageLike } from "./campaign-touch-plan-core";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";
import {
  CampaignTouchPlanReviewMachine,
  campaignTouchPlanReviewStorageKey,
  loadTouchPlanReview,
  parseTouchPlanReviewResponse,
  type CampaignTouchPlanReviewTransport,
} from "./campaign-touch-plan-review";

const at = "2026-08-24T01:02:03.123456Z";
const reviewedAt = "2026-08-24T02:03:04.654321Z";
const plan: TouchPlanSummary = {
  id: `ctp_${"a".repeat(64)}`,
  campaignCode: "campaign_a",
  campaignVersion: 4,
  source: { kind: "customer_selection", digest: "b".repeat(64) },
  targetCount: 1,
  targetDigest: "c".repeat(64),
  stepCount: 1,
  contentDigest: "d".repeat(64),
  immutable: "verified-c1",
};
const safety = {
  local_only: true,
  provider_execution_eligible: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const draft = { review: { status: "draft", version: 1 }, ...safety };
const pending = {
  review: {
    status: "pending_review",
    version: 2,
    submitted_by_actor_id: 7,
    submitted_at: at,
  },
  ...safety,
};
const approved = {
  review: {
    ...pending.review,
    status: "approved",
    version: 3,
    reviewed_by_actor_id: 8,
    reviewed_at: reviewedAt,
  },
  handoff: {
    status: "pending_outbound_acceptance",
    review_version: 3,
    created_at: reviewedAt,
  },
  ...safety,
};
const rejected = {
  review: { ...approved.review, status: "rejected" },
  ...safety,
};

function storage(): SessionStorageLike & { values: Map<string, string> } {
  const values = new Map<string, string>();
  return {
    values,
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => void values.set(key, value),
    removeItem: (key) => void values.delete(key),
  };
}
function transport(
  mutate: CampaignTouchPlanReviewTransport["mutateReview"] = vi.fn(
    async () => ({ status: 200, data: pending }),
  ),
  get: CampaignTouchPlanReviewTransport["getReview"] = vi.fn(async () => ({
    status: 200,
    data: draft,
  })),
): CampaignTouchPlanReviewTransport {
  return { mutateReview: mutate, getReview: get };
}
function machine(
  reviewTransport: CampaignTouchPlanReviewTransport,
  sessionStorage = storage(),
  uuids = ["11111111-1111-4111-8111-111111111111"],
) {
  let index = 0;
  return {
    sessionStorage,
    value: new CampaignTouchPlanReviewMachine({
      transport: reviewTransport,
      sessionStorage,
      actorID: 7,
      keySource: { randomUUID: () => uuids[index++] ?? uuids.at(-1)! },
    }),
  };
}
const csrf = "x".repeat(43);

describe("campaign touch-plan review parser", () => {
  it("accepts UTC RFC3339Nano fractions from zero through six digits only", () => {
    for (const timestamp of [
      "2026-08-24T01:02:03Z",
      "2026-08-24T01:02:03.1Z",
      "2026-08-24T01:02:03.12Z",
      "2026-08-24T01:02:03.12345Z",
      "2026-08-24T01:02:03.123456Z",
    ]) {
      expect(
        parseTouchPlanReviewResponse({
          ...pending,
          review: { ...pending.review, submitted_at: timestamp },
        }),
      ).toBeDefined();
    }
    for (const timestamp of [
      "2026-08-24T01:02:03.1234567Z",
      "2026-08-24T01:02:03.12+00:00",
      "2026-02-29T01:02:03.12Z",
    ]) {
      expect(
        parseTouchPlanReviewResponse({
          ...pending,
          review: { ...pending.review, submitted_at: timestamp },
        }),
      ).toBeUndefined();
    }
  });

  it("accepts only the closed v1/v2/v3 state machine and approved handoff", () => {
    expect(parseTouchPlanReviewResponse(draft)?.review.status).toBe("draft");
    expect(parseTouchPlanReviewResponse(pending)?.review.status).toBe(
      "pending_review",
    );
    expect(parseTouchPlanReviewResponse(approved)?.handoff?.reviewVersion).toBe(
      3,
    );
    expect(parseTouchPlanReviewResponse(rejected)?.review.status).toBe(
      "rejected",
    );

    for (const malformed of [
      { ...draft, review: { ...draft.review, submitted_at: at } },
      { ...pending, review: { ...pending.review, version: 3 } },
      {
        ...pending,
        review: { ...pending.review, submitted_at: at.slice(0, -1) },
      },
      { ...approved, handoff: undefined },
      { ...rejected, handoff: approved.handoff },
      { ...approved, handoff: { ...approved.handoff, review_version: 4 } },
      { ...approved, provider_execution_eligible: true },
      { ...approved, extra: false },
    ])
      expect(parseTouchPlanReviewResponse(malformed)).toBeUndefined();
  });

  it("fails a malformed 200 closed", async () => {
    await expect(
      loadTouchPlanReview(
        transport(
          undefined,
          vi.fn(async () => ({
            status: 200,
            data: { ...pending, extra: true },
          })),
        ),
        plan,
      ),
    ).resolves.toEqual({ status: "unavailable" });
  });

  it("loads a pending GET whose legal microseconds omit trailing zeroes", async () => {
    const shortened = {
      ...pending,
      review: { ...pending.review, submitted_at: "2026-08-24T01:02:03.12345Z" },
    };
    await expect(
      loadTouchPlanReview(
        transport(
          undefined,
          vi.fn(async () => ({ status: 200, data: shortened })),
        ),
        plan,
      ),
    ).resolves.toMatchObject({
      status: "loaded",
      review: {
        status: "pending_review",
        submittedAt: shortened.review.submitted_at,
      },
    });
  });
});

describe("CampaignTouchPlanReviewMachine", () => {
  it("completes submit and approve with legal shortened RFC3339Nano timestamps", async () => {
    const shortenedPending = {
      ...pending,
      review: { ...pending.review, submitted_at: "2026-08-24T01:02:03.12Z" },
    };
    const shortenedApproved = {
      ...approved,
      review: {
        ...approved.review,
        submitted_at: shortenedPending.review.submitted_at,
        reviewed_by_actor_id: 7,
        reviewed_at: "2026-08-24T02:03:04Z",
      },
      handoff: { ...approved.handoff, created_at: "2026-08-24T02:03:04Z" },
    };
    const submitted = machine(
      transport(vi.fn(async () => ({ status: 200, data: shortenedPending }))),
    ).value;
    await expect(
      submitted.start({
        plan,
        review: parseTouchPlanReviewResponse(draft)!.review,
        operation: "submit",
        confirmation: "",
        csrf,
      }),
    ).resolves.toMatchObject({
      status: "completed",
      review: { submittedAt: shortenedPending.review.submitted_at },
    });

    const decided = machine(
      transport(vi.fn(async () => ({ status: 200, data: shortenedApproved }))),
    ).value;
    await expect(
      decided.start({
        plan,
        review: {
          status: "pending_review",
          version: 2,
          submittedByActorID: 6,
          submittedAt: shortenedPending.review.submitted_at,
        },
        operation: "approve",
        confirmation: `APPROVE ${plan.id}`,
        csrf,
      }),
    ).resolves.toMatchObject({
      status: "completed",
      review: { reviewedAt: "2026-08-24T02:03:04Z" },
    });
  });

  it("completes an exact replay whose legal timestamp omits trailing zeroes", async () => {
    const shortened = {
      ...pending,
      review: { ...pending.review, submitted_at: "2026-08-24T01:02:03.12345Z" },
    };
    const mutate = vi
      .fn()
      .mockRejectedValueOnce(new Error("reset"))
      .mockResolvedValueOnce({ status: 200, data: shortened });
    const subject = machine(transport(mutate)).value;
    const input = {
      plan,
      review: parseTouchPlanReviewResponse(draft)!.review,
      operation: "submit" as const,
      confirmation: "",
      csrf,
    };
    await expect(subject.start(input)).resolves.toEqual({
      status: "outcome_unknown",
    });
    await expect(subject.replay(input)).resolves.toMatchObject({
      status: "completed",
      review: { submittedAt: shortened.review.submitted_at },
    });
    expect(mutate.mock.calls[0][4].headers["Idempotency-Key"]).toBe(
      mutate.mock.calls[1][4].headers["Idempotency-Key"],
    );
  });

  it("does not POST a decision without its exact confirmation and submits with empty confirmation", async () => {
    const reviewTransport = transport();
    const subject = machine(reviewTransport).value;
    await expect(
      subject.start({
        plan,
        review: parseTouchPlanReviewResponse(pending)!.review,
        operation: "approve",
        confirmation: "APPROVE",
        csrf,
      }),
    ).resolves.toEqual({ status: "confirmation_required" });
    expect(reviewTransport.mutateReview).not.toHaveBeenCalled();
    await subject.start({
      plan,
      review: parseTouchPlanReviewResponse(draft)!.review,
      operation: "submit",
      confirmation: "",
      csrf,
    });
    expect(reviewTransport.mutateReview).toHaveBeenCalledWith(
      plan.campaignCode,
      plan.id,
      "submit",
      { expected_version: 1 },
      expect.objectContaining({
        headers: expect.objectContaining({ "X-CSRF-Token": csrf }),
      }),
    );
  });

  it("uses CAS and a stable actor-scoped idempotency intent for exact replay only", async () => {
    const mutate = vi
      .fn()
      .mockRejectedValueOnce(new Error("connection reset"))
      .mockResolvedValueOnce({ status: 200, data: pending });
    const reviewTransport = transport(mutate);
    const fixture = machine(reviewTransport);
    const input = {
      plan,
      review: parseTouchPlanReviewResponse(draft)!.review,
      operation: "submit" as const,
      confirmation: "",
      csrf,
    };
    await expect(fixture.value.start(input)).resolves.toEqual({
      status: "outcome_unknown",
    });
    await expect(fixture.value.start(input)).resolves.toEqual({
      status: "replay_required",
    });
    await expect(
      fixture.value.replay({
        ...input,
        plan: { ...plan, immutable: "other-c1" },
      }),
    ).resolves.toEqual({ status: "replay_mismatch" });
    await expect(fixture.value.replay(input)).resolves.toMatchObject({
      status: "completed",
      review: { status: "pending_review", version: 2 },
    });
    expect(mutate).toHaveBeenCalledTimes(2);
    expect(mutate.mock.calls[0][3]).toEqual({ expected_version: 1 });
    expect(mutate.mock.calls[0][4].headers["Idempotency-Key"]).toBe(
      mutate.mock.calls[1][4].headers["Idempotency-Key"],
    );
    expect(
      fixture.sessionStorage.getItem(campaignTouchPlanReviewStorageKey(7)),
    ).toBeNull();
  });

  it("rejects non-canonical actor storage before issuing a POST", async () => {
    const reviewTransport = transport();
    const session = storage();
    session.setItem(
      campaignTouchPlanReviewStorageKey(7),
      JSON.stringify({
        version: 1,
        idempotency_key:
          "campaign-touch-plan-review:11111111-1111-4111-8111-111111111111",
        payload: {
          campaign_code: 7,
          plan_id: plan.id,
          plan_immutable: plan.immutable,
          operation: "submit",
          expected_version: 1,
          confirmation: "",
        },
      }),
    );
    const subject = machine(reviewTransport, session).value;
    await expect(
      subject.start({
        plan,
        review: parseTouchPlanReviewResponse(draft)!.review,
        operation: "submit",
        confirmation: "",
        csrf,
      }),
    ).resolves.toEqual({ status: "storage_unavailable" });
    expect(reviewTransport.mutateReview).not.toHaveBeenCalled();
  });

  it("refreshes GET on 409, then a reconfirmed action creates a new intent and key", async () => {
    const mutate = vi
      .fn()
      .mockResolvedValueOnce({ status: 409, data: { code: "CONFLICT" } })
      .mockResolvedValueOnce({ status: 200, data: pending });
    const get = vi.fn(async () => ({ status: 200, data: draft }));
    const subject = machine(transport(mutate, get), storage(), [
      "11111111-1111-4111-8111-111111111111",
      "22222222-2222-4222-8222-222222222222",
    ]).value;
    const input = {
      plan,
      review: parseTouchPlanReviewResponse(draft)!.review,
      operation: "submit" as const,
      confirmation: "",
      csrf,
    };
    await expect(subject.start(input)).resolves.toMatchObject({
      status: "conflict",
      review: { version: 1 },
    });
    await expect(subject.start(input)).resolves.toMatchObject({
      status: "completed",
    });
    expect(get).toHaveBeenCalledTimes(1);
    expect(mutate.mock.calls[0][4].headers["Idempotency-Key"]).not.toBe(
      mutate.mock.calls[1][4].headers["Idempotency-Key"],
    );
  });

  it.each([
    [409, { code: "BLOCKED_REDLINE" }, "blocked"],
    [400, { code: "MALFORMED_REQUEST" }, "invalid"],
    [401, { code: "UNAUTHENTICATED" }, "unauthenticated"],
    [403, { code: "FORBIDDEN" }, "forbidden"],
    [404, { code: "NOT_FOUND" }, "not_found"],
  ] as const)(
    "makes deterministic %s %s non-replayable",
    async (status, data, expected) => {
      const fixture = machine(transport(vi.fn(async () => ({ status, data }))));
      const input = {
        plan,
        review: parseTouchPlanReviewResponse(draft)!.review,
        operation: "submit" as const,
        confirmation: "",
        csrf,
      };
      await expect(fixture.value.start(input)).resolves.toMatchObject({
        status: expected,
      });
      await expect(fixture.value.replay(input)).resolves.toEqual({
        status: "no_pending",
      });
    },
  );

  it("guards synchronous double-clicks before React can commit disabled state", async () => {
    // eslint-disable-next-line no-unused-vars -- deferred resolver input defines the promise type.
    let resolve!: (value: { status: number; data: unknown }) => void;
    const mutate = vi.fn(
      () =>
        new Promise<{ status: number; data: unknown }>((done) => {
          resolve = done;
        }),
    );
    const subject = machine(transport(mutate)).value;
    const input = {
      plan,
      review: parseTouchPlanReviewResponse(draft)!.review,
      operation: "submit" as const,
      confirmation: "",
      csrf,
    };
    const first = subject.start(input);
    await expect(subject.start(input)).resolves.toEqual({ status: "inflight" });
    expect(mutate).toHaveBeenCalledTimes(1);
    resolve({ status: 200, data: pending });
    await expect(first).resolves.toMatchObject({ status: "completed" });
  });
});
