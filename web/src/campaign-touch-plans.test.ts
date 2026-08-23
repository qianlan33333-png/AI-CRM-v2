import { describe, expect, it } from "vitest";
import {
  mutationOptions,
  mutationResult,
  parseAudiencePackages,
  parseCampaigns,
  parseHandoff,
  parsePlans,
  parseRecipients,
  parseReview,
  reviewRequest,
} from "./campaign-touch-plans";

const safety = {
  local_only: true,
  provider_execution_eligible: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const planID = `ctp_${"a".repeat(64)}`;

describe("Campaign touch plan protocol", () => {
  it("accepts only the safe campaign and audience package projections", () => {
    expect(
      parseCampaigns({
        items: [
          {
            campaign_code: "c1",
            name: "Campaign",
            approval_status: "draft",
            runtime_status: "idle",
            version: 1,
            created_by: 1,
            updated_by: 1,
            created_at: "2026-08-24T00:00:00Z",
            updated_at: "2026-08-24T00:00:00Z",
          },
        ],
        local_projection: true,
        real_external_call_executed: false,
        real_send: false,
        runtime_executed: false,
      }),
    ).toEqual([
      {
        code: "c1",
        name: "Campaign",
        approval: "draft",
        runtime: "idle",
        version: 1,
      },
    ]);
    expect(
      parseAudiencePackages({
        items: [
          {
            package_id: 1,
            name: "Audience",
            group_id: null,
            lifecycle: "active",
            version: 2,
            refresh_mode: "manual",
            refresh_cron: null,
            member_count: 4,
            refreshed_at: null,
            refresh_status: "idle",
            created_at: "2026-08-24T00:00:00Z",
            updated_at: "2026-08-24T00:00:00Z",
          },
        ],
        limit: 100,
        offset: 0,
        total: 1,
        local_projection: true,
        real_external_call_executed: false,
      }),
    ).toEqual([{ id: 1, name: "Audience", version: 2, members: 4 }]);
    expect(
      parseAudiencePackages({
        items: [],
        limit: 100,
        offset: 0,
        total: 0,
        local_projection: true,
        real_external_call_executed: false,
        provider_id: "forbidden",
      }),
    ).toBeUndefined();
  });

  it("rejects an unsafe touch-plan projection and only returns canonical recipient IDs", () => {
    const item = {
      id: planID,
      campaign_code: "c1",
      campaign_version: 1,
      source: {
        kind: "ai_audience_package_members",
        audience_package: {
          package_id: 1,
          package_version: 1,
          member_snapshot_watermark: "w",
          digest: "a".repeat(64),
        },
      },
      target_count: 1,
      target_digest: "a".repeat(64),
      content_step_count: 1,
      content_digest: "a".repeat(64),
      owner_actor_id: 1,
      preview_exclusion_summary: {},
      created_at: "2026-08-24T00:00:00Z",
      ...safety,
      runtime_executed: false,
    };
    expect(
      parsePlans({
        items: [item],
        next_cursor: null,
        ...safety,
        runtime_executed: false,
      }),
    ).toMatchObject({ items: [{ id: planID, packageID: 1 }] });
    expect(
      parsePlans({
        items: [
          {
            ...item,
            source: {
              kind: "customer_selection",
              customer_selection: { id: "local_selection" },
            },
          },
        ],
        next_cursor: null,
        ...safety,
        runtime_executed: false,
      }),
    ).toBeUndefined();
    expect(
      parsePlans({
        items: [item],
        next_cursor: null,
        ...safety,
        runtime_executed: true,
      }),
    ).toBeUndefined();
    expect(
      parseRecipients({
        items: [{ canonical_customer_id: 42 }],
        next_cursor: null,
        ...safety,
      }),
    ).toEqual({ ids: [42] });
    expect(
      parseRecipients({
        items: [{ canonical_customer_id: 42, phone: "forbidden" }],
        next_cursor: null,
        ...safety,
      }),
    ).toBeUndefined();
  });

  it("keeps review and held handoff facts closed and local", () => {
    expect(
      parseReview({
        review: {
          status: "approved",
          version: 3,
          submitted_by_actor_id: 1,
          submitted_at: "t",
          reviewed_by_actor_id: 2,
          reviewed_at: "t",
        },
        handoff: {
          status: "pending_outbound_acceptance",
          review_version: 3,
          created_at: "t",
        },
        ...safety,
      }),
    ).toEqual({
      status: "approved",
      version: 3,
      handoff: { reviewVersion: 3, createdAt: "t" },
    });
    expect(
      parseHandoff({
        id: 7,
        campaign_code: "c1",
        plan_id: planID,
        review_version: 3,
        status: "held",
        target_count: 1,
        step_count: 1,
        accepted_at: "t",
        safety,
      }),
    ).toMatchObject({ id: 7, campaignCode: "c1", planID });
    expect(
      parseHandoff({
        id: 7,
        campaign_code: "c1",
        plan_id: planID,
        review_version: 3,
        status: "held",
        target_count: 1,
        step_count: 1,
        accepted_at: "t",
        safety: { ...safety, delivery_proven: true },
      }),
    ).toBeUndefined();
  });

  it("uses the existing same-origin CSRF token and retains exact CAS bodies", () => {
    const key = "touch-plan-review-0123456789";
    expect(mutationOptions(`aicrm_csrf=${"A".repeat(43)}`, key)).toMatchObject({
      credentials: "same-origin",
      headers: { "X-CSRF-Token": "A".repeat(43), "Idempotency-Key": key },
    });
    expect(mutationOptions("aicrm_csrf=short", key)).toBeUndefined();
    expect(reviewRequest("submit", planID, 1)).toEqual({ expected_version: 1 });
    expect(reviewRequest("approve", planID, 2, `APPROVE ${planID}`)).toEqual({
      expected_version: 2,
      confirmation: `APPROVE ${planID}`,
    });
    expect(reviewRequest("reject", planID, 2, "REJECT other")).toBeUndefined();
    expect(mutationResult(409)).toBe("conflict");
  });
});
