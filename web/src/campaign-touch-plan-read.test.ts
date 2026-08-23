import { describe, expect, it } from "vitest";
import {
  loadTouchPlanDetail,
  loadTouchPlanRecipients,
  loadTouchPlans,
  type CampaignTouchPlanReadTransport,
} from "./campaign-touch-plan-read";

const digest = "a".repeat(64);
const planID = `ctp_${digest}`;
const timestamp = "2026-08-24T01:02:03.123456Z";
const safety = {
  local_only: true,
  provider_execution_eligible: false,
  runtime_executed: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const source = {
  kind: "segment_members",
  segment: {
    segment_id: 7,
    member_snapshot_watermark: timestamp,
    digest,
  },
};
const summary = {
  id: planID,
  campaign_code: "spring",
  campaign_version: 4,
  source,
  target_count: 2,
  target_digest: digest,
  content_step_count: 2,
  content_digest: digest,
  owner_actor_id: 9,
  preview_exclusion_summary: {
    candidate_count: 3,
    active_customer_count: 2,
    inactive_excluded_count: 1,
    policy_excluded_count: 0,
  },
  created_at: timestamp,
  ...safety,
};
const {
  content_step_count: ignoredStepCount,
  content_digest: ignoredContentDigest,
  ...detailBase
} = summary;
void ignoredStepCount;
void ignoredContentDigest;
const detail = {
  ...detailBase,
  content: {
    steps: [
      { step_index: 1, delay_minutes: 0, content: "local one" },
      { step_index: 2, delay_minutes: 3, content: "local two" },
    ],
    content_digest: digest,
  },
};
const recipientSafety = {
  local_only: true,
  provider_execution_eligible: false,
  real_external_call_executed: false,
  delivery_proven: false,
};

function transport(data: unknown): CampaignTouchPlanReadTransport {
  return {
    listCampaigns: async () => ({ status: 200, data: { items: [] } }),
    getCampaign: async () => ({ status: 500, data: {} }),
    createPlan: async () => ({ status: 500, data: {} }),
    listPlans: async () => ({ status: 200, data }),
    getPlan: async () => ({ status: 200, data }),
    listRecipients: async () => ({ status: 200, data }),
  };
}

describe("Campaign touch-plan read contract", () => {
  it("accepts only a closed plan page and detail which agree exactly", async () => {
    const page = await loadTouchPlans(
      transport({ items: [summary], ...safety }),
      "spring",
    );
    expect(page.status).toBe("loaded");
    if (page.status !== "loaded") return;
    const result = await loadTouchPlanDetail(
      transport(detail),
      "spring",
      page.plans[0],
    );
    expect(result.status).toBe("loaded");
  });

  it("fails closed for mismatched summary detail, gapped steps, or invalid exclusions", async () => {
    const page = await loadTouchPlans(
      transport({ items: [summary], next_cursor: null, ...safety }),
      "spring",
    );
    if (page.status !== "loaded") throw new Error("fixture failed");
    for (const invalid of [
      { ...detail, campaign_version: 5 },
      {
        ...detail,
        content: {
          ...detail.content,
          steps: [{ ...detail.content.steps[0], step_index: 2 }],
        },
      },
      {
        ...detail,
        preview_exclusion_summary: {
          ...summary.preview_exclusion_summary,
          candidate_count: 2,
        },
      },
      { ...detail, created_at: "2026-08-24T01:02:03+08:00" },
    ]) {
      await expect(
        loadTouchPlanDetail(transport(invalid), "spring", page.plans[0]),
      ).resolves.toEqual({ status: "unavailable" });
    }
  });

  it("fails closed when a detail replaces any immutable owner, creation time, or valid exclusion snapshot", async () => {
    const page = await loadTouchPlans(
      transport({ items: [summary], ...safety }),
      "spring",
    );
    if (page.status !== "loaded") throw new Error("fixture failed");
    for (const invalid of [
      { ...detail, owner_actor_id: 10 },
      { ...detail, created_at: "2026-08-24T01:02:03.123457Z" },
      {
        ...detail,
        preview_exclusion_summary: {
          candidate_count: 4,
          active_customer_count: 2,
          inactive_excluded_count: 2,
          policy_excluded_count: 0,
        },
      },
    ]) {
      await expect(
        loadTouchPlanDetail(transport(invalid), "spring", page.plans[0]),
      ).resolves.toEqual({ status: "unavailable" });
    }
  });

  it("accepts a recipient page only when it is canonical and closes the terminal target count", async () => {
    const page = await loadTouchPlanRecipients(
      transport({
        items: [{ canonical_customer_id: 1 }, { canonical_customer_id: 2 }],
        next_cursor: null,
        ...recipientSafety,
      }),
      "spring",
      planID,
      2,
    );
    expect(page).toEqual({
      status: "loaded",
      recipients: [1, 2],
      nextCursor: undefined,
    });
  });

  it("binds a nonterminal cursor to its page's final canonical OneID", async () => {
    const first = await loadTouchPlanRecipients(
      transport({
        items: [{ canonical_customer_id: 1 }],
        next_cursor: "1",
        ...recipientSafety,
      }),
      "spring",
      planID,
      2,
    );
    expect(first).toEqual({
      status: "loaded",
      recipients: [1],
      nextCursor: "1",
    });
    const final = await loadTouchPlanRecipients(
      transport({
        items: [{ canonical_customer_id: 2 }],
        next_cursor: null,
        ...recipientSafety,
      }),
      "spring",
      planID,
      2,
      "1",
      [1],
    );
    expect(final).toEqual({
      status: "loaded",
      recipients: [2],
      nextCursor: undefined,
    });
  });

  it("fails closed for recipient duplicates, a noncanonical cursor, or a nonterminal count mismatch", async () => {
    for (const data of [
      {
        items: [{ canonical_customer_id: 2 }, { canonical_customer_id: 2 }],
        next_cursor: null,
        ...recipientSafety,
      },
      {
        items: [{ canonical_customer_id: 1 }],
        next_cursor: "01",
        ...recipientSafety,
      },
      {
        items: [{ canonical_customer_id: 1 }],
        next_cursor: null,
        ...recipientSafety,
      },
    ]) {
      await expect(
        loadTouchPlanRecipients(transport(data), "spring", planID, 2),
      ).resolves.toEqual({ status: "unavailable" });
    }
  });

  it("binds a requested recipient cursor to the existing page's final OneID", async () => {
    await expect(
      loadTouchPlanRecipients(
        transport({
          items: [{ canonical_customer_id: 6 }],
          next_cursor: null,
          ...recipientSafety,
        }),
        "spring",
        planID,
        2,
        "5",
        [1],
      ),
    ).resolves.toEqual({ status: "unavailable" });
  });

  it("rejects a continuation prior when its cursor is omitted", async () => {
    await expect(
      loadTouchPlanRecipients(
        transport({
          items: [{ canonical_customer_id: 2 }],
          next_cursor: null,
          ...recipientSafety,
        }),
        "spring",
        planID,
        2,
        undefined,
        [1],
      ),
    ).resolves.toEqual({ status: "unavailable" });
  });
});
