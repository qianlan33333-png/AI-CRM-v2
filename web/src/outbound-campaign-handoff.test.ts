/* eslint-disable no-unused-vars -- deferred callback types document their payload. */
import { describe, expect, it, vi } from "vitest";
import type { SessionStorageLike } from "./campaign-touch-plan-core";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";
import type { TouchPlanReviewSnapshot } from "./campaign-touch-plan-review";
import {
  OutboundCampaignHandoffMachine,
  loadOutboundCampaignHandoffSummary,
  outboundCampaignHandoffStorageKey,
  parseOutboundCampaignHandoffReconciliation,
  parseOutboundCampaignHandoffSummary,
  type OutboundCampaignHandoffTransport,
} from "./outbound-campaign-handoff";

const plan: TouchPlanSummary = {
  id: `ctp_${"a".repeat(64)}`,
  campaignCode: "campaign_a",
  campaignVersion: 4,
  source: { kind: "customer_selection", digest: "b".repeat(64) },
  targetCount: 2,
  targetDigest: "c".repeat(64),
  stepCount: 1,
  contentDigest: "d".repeat(64),
  immutable: "verified-c1",
};
const approved: TouchPlanReviewSnapshot = {
  review: {
    status: "approved",
    version: 3,
    submittedByActorID: 6,
    submittedAt: "2026-08-24T01:00:00Z",
    reviewedByActorID: 7,
    reviewedAt: "2026-08-24T02:00:00.12Z",
  },
  handoff: {
    status: "pending_outbound_acceptance",
    reviewVersion: 3,
    createdAt: "2026-08-24T02:00:00.12Z",
  },
};
const safety = {
  local_only: true,
  provider_execution_eligible: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const summary = {
  id: 9,
  campaign_code: plan.campaignCode,
  plan_id: plan.id,
  review_version: 3,
  status: "held",
  target_count: 2,
  step_count: 1,
  accepted_at: "2026-08-24T02:00:01.12345Z",
  safety,
};
const reconciliation = {
  ...summary,
  held_count: 2,
  blocked_count: 0,
  pending_count: 0,
  not_evaluated_count: 2,
  eligible_count: 0,
  inactive_count: 0,
  contact_policy_count: 0,
};
const csrf = "x".repeat(43);

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
  accept: OutboundCampaignHandoffTransport["accept"] = vi.fn(async () => ({ status: 200, data: reconciliation })),
  getSummary: OutboundCampaignHandoffTransport["getSummary"] = vi.fn(async () => ({ status: 404, data: {} })),
  reconcile: OutboundCampaignHandoffTransport["reconcile"] = vi.fn(async () => ({ status: 200, data: reconciliation })),
): OutboundCampaignHandoffTransport {
  return { accept, getSummary, reconcile };
}
function machine(value: OutboundCampaignHandoffTransport, sessionStorage = storage()) {
  return {
    sessionStorage,
    value: new OutboundCampaignHandoffMachine({
      transport: value,
      sessionStorage,
      actorID: 7,
      keySource: { randomUUID: () => "11111111-1111-4111-8111-111111111111" },
    }),
  };
}

describe("outbound Campaign held parser", () => {
  it("accepts only closed exact summary and valid UTC microsecond timestamps", () => {
    for (const accepted_at of ["2026-08-24T02:00:01Z", "2026-08-24T02:00:01.1Z", "2026-08-24T02:00:01.12Z", "2026-08-24T02:00:01.12345Z", "2026-08-24T02:00:01.123456Z"])
      expect(parseOutboundCampaignHandoffSummary({ ...summary, accepted_at }, plan, approved)).toBeDefined();
    for (const accepted_at of ["2026-08-24T02:00:01.1234567Z", "2026-08-24T02:00:01+00:00", "2026-02-29T02:00:01Z", "2026-08-24T01:59:59Z"])
      expect(parseOutboundCampaignHandoffSummary({ ...summary, accepted_at }, plan, approved)).toBeUndefined();
    expect(parseOutboundCampaignHandoffSummary({ ...summary, extra: true }, plan, approved)).toBeUndefined();
    expect(parseOutboundCampaignHandoffSummary({ ...summary, review_version: 4 }, plan, approved)).toBeUndefined();
    expect(parseOutboundCampaignHandoffSummary({ ...summary, safety: { ...safety, provider_execution_eligible: true } }, plan, approved)).toBeUndefined();
  });

  it("enforces both reconciliation count equations and exact accept receipt counts", () => {
    expect(parseOutboundCampaignHandoffReconciliation(reconciliation, plan, approved, false)).toBeDefined();
    expect(parseOutboundCampaignHandoffReconciliation({ ...reconciliation, held_count: 1, blocked_count: 1 }, plan, approved, false)).toBeDefined();
    expect(parseOutboundCampaignHandoffReconciliation({ ...reconciliation, held_count: 1 }, plan, approved, false)).toBeUndefined();
    expect(parseOutboundCampaignHandoffReconciliation({ ...reconciliation, eligible_count: 1 }, plan, approved, false)).toBeUndefined();
    expect(parseOutboundCampaignHandoffReconciliation({ ...reconciliation, held_count: 1, blocked_count: 1 }, plan, approved, true)).toBeUndefined();
  });
});

describe("outbound Campaign held state machine", () => {
  it("treats summary 404 as not accepted and rejects malformed 200", async () => {
    expect(await loadOutboundCampaignHandoffSummary(transport(), plan, approved)).toEqual({ status: "not_accepted" });
    const malformed = transport(vi.fn(), vi.fn(async () => ({ status: 200, data: { ...summary, status: "sent" } })));
    expect(await loadOutboundCampaignHandoffSummary(malformed, plan, approved)).toEqual({ status: "unavailable" });
  });

  it("requires exact confirmation, sends CSRF/CAS, and records a stable actor-scoped intent", async () => {
    const accept = vi.fn(async () => ({ status: 200, data: reconciliation }));
    const sessionStorage = storage();
    const subject = machine(transport(accept), sessionStorage).value;
    expect(await subject.start({ plan, approved, confirmation: "", csrf })).toEqual({ status: "confirmation_required" });
    expect(accept).not.toHaveBeenCalled();
    const result = await subject.start({ plan, approved, confirmation: `ACCEPT ${plan.id}`, csrf });
    expect(result.status).toBe("completed");
    expect(accept).toHaveBeenCalledWith(plan.campaignCode, plan.id, { expected_review_version: 3 }, {
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrf, "Idempotency-Key": "outbound-campaign-handoff:11111111-1111-4111-8111-111111111111" },
      signal: undefined,
    });
    expect(sessionStorage.values.has(outboundCampaignHandoffStorageKey(7))).toBe(false);
    expect(sessionStorage.values.has(outboundCampaignHandoffStorageKey(8))).toBe(false);
  });

  it("preserves only outcome-unknown exact replay and uses the same idempotency key", async () => {
    const accept = vi.fn()
      .mockResolvedValueOnce({ status: 503, data: {} })
      .mockResolvedValueOnce({ status: 200, data: reconciliation });
    const sessionStorage = storage();
    const subject = machine(transport(accept), sessionStorage).value;
    const input = { plan, approved, confirmation: `ACCEPT ${plan.id}`, csrf };
    expect(await subject.start(input)).toEqual({ status: "outcome_unknown" });
    expect(await subject.start(input)).toEqual({ status: "replay_required" });
    expect(await subject.replay({ ...input, confirmation: "ACCEPT wrong" })).toEqual({ status: "confirmation_required" });
    expect((await subject.replay(input)).status).toBe("completed");
    expect(accept.mock.calls[0][3].headers["Idempotency-Key"]).toBe(accept.mock.calls[1][3].headers["Idempotency-Key"]);
  });

  it("clears a 409 intent, refreshes summary without auto retry, and makes a new intent possible", async () => {
    const accept = vi.fn(async () => ({ status: 409, data: { code: "CONFLICT" } }));
    const getSummary = vi.fn(async () => ({ status: 200, data: summary }));
    const sessionStorage = storage();
    const subject = machine(transport(accept, getSummary), sessionStorage).value;
    const input = { plan, approved, confirmation: `ACCEPT ${plan.id}`, csrf };
    const result = await subject.start(input);
    expect(result.status).toBe("conflict");
    expect(getSummary).toHaveBeenCalledTimes(1);
    expect(accept).toHaveBeenCalledTimes(1);
    expect(sessionStorage.values.size).toBe(0);
  });

  it("has a synchronous double-click guard and does not retain deterministic 4xx", async () => {
    let resolve!: (value: { status: number; data: unknown }) => void;
    const accept = vi.fn(() => new Promise<{ status: number; data: unknown }>((done) => { resolve = done; }));
    const sessionStorage = storage();
    const subject = machine(transport(accept), sessionStorage).value;
    const input = { plan, approved, confirmation: `ACCEPT ${plan.id}`, csrf };
    const first = subject.start(input);
    expect(await subject.start(input)).toEqual({ status: "inflight" });
    resolve({ status: 400, data: {} });
    expect(await first).toEqual({ status: "invalid" });
    expect(sessionStorage.values.size).toBe(0);
  });
});
