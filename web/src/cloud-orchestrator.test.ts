import { describe, expect, it } from "vitest";
import {
  CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
  CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH,
  CLOUD_ORCHESTRATOR_PLANS_PATH,
  CLOUD_ORCHESTRATOR_ROOT_PATH,
  campaignSourceHref,
  cloudOrchestratorCarrierRoute,
  cloudOrchestratorRoute,
  cloudOrchestratorWorkspaceLinks,
  type CloudCampaignSourceKind,
} from "./cloud-orchestrator";

describe("cloud orchestrator approved page routes", () => {
  it.each([
    [CLOUD_ORCHESTRATOR_ROOT_PATH, "root"],
    [CLOUD_ORCHESTRATOR_PLANS_PATH, "plans"],
    [`${CLOUD_ORCHESTRATOR_PLANS_PATH}/plan_A-42`, "plan_detail"],
    [CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, "campaigns"],
    [CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH, "observability"],
  ])("accepts %s as %s", (pathname, kind) => {
    expect(cloudOrchestratorRoute(pathname)).toMatchObject({ kind, pathname });
  });

  it("decodes a plan identifier only for display and keeps it route-bound", () => {
    expect(
      cloudOrchestratorRoute(`${CLOUD_ORCHESTRATOR_PLANS_PATH}/plan%20one`),
    ).toMatchObject({
      kind: "plan_detail",
      planID: "plan one",
    });
  });

  it("builds and parses only the closed direct Campaign source URL", () => {
    expect(campaignSourceHref("customer_selection", 7)).toBe(
      `${CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH}?source_kind=customer_selection&source_id=7`,
    );
    expect(
      cloudOrchestratorRoute(
        CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
        "?source_kind=customer_selection&source_id=7",
      ),
    ).toEqual({
      kind: "campaigns",
      pathname: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
      source_kind: "customer_selection",
      source_id: "7",
    });
  });

  it("preserves a canonical int64 string in the direct Campaign URL", () => {
    const sourceID = "9223372036854775807";
    expect(campaignSourceHref("ai_audience_package_members", sourceID)).toBe(
      `${CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH}?source_kind=ai_audience_package_members&source_id=${sourceID}`,
    );
    expect(
      cloudOrchestratorRoute(
        CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
        `?source_kind=ai_audience_package_members&source_id=${sourceID}`,
      ),
    ).toEqual({
      kind: "campaigns",
      pathname: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
      source_kind: "ai_audience_package_members",
      source_id: sourceID,
    });
  });

  it.each([
    ["customer_selection", 0],
    ["customer_selection", Number.MAX_SAFE_INTEGER + 1],
    ["customer_selection", "07"],
    ["customer_selection", "7&filter=private"],
    ["untrusted" as CloudCampaignSourceKind, 7],
  ] as const)("rejects a noncanonical source URL input %s/%s", (kind, id) => {
    expect(campaignSourceHref(kind, id)).toBeUndefined();
  });

  it.each([
    "",
    "/admin/cloud-orchestrator/unknown",
    `${CLOUD_ORCHESTRATOR_PLANS_PATH}/`,
    `${CLOUD_ORCHESTRATOR_PLANS_PATH}/plan/nested`,
    `${CLOUD_ORCHESTRATOR_PLANS_PATH}/%2Fescaped`,
    `${CLOUD_ORCHESTRATOR_PLANS_PATH}/%0Aheader`,
    `${CLOUD_ORCHESTRATOR_PLANS_PATH}/..`,
  ])("rejects unapproved or unsafe path %s", (pathname) => {
    expect(cloudOrchestratorRoute(pathname)).toBeUndefined();
  });

  it.each([
    "?source_kind=customer_selection",
    "?source_kind=customer_selection&source_id=7&filter=private",
    "?source_id=7&source_kind=customer_selection",
    "?source_kind=customer_selection&source_id=07",
  ])("rejects a noncanonical direct Campaign query %s", (search) => {
    expect(
      cloudOrchestratorRoute(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, search),
    ).toBeUndefined();
  });

  it("accepts only an exact single carrier parameter", () => {
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH)}`,
      ),
    ).toMatchObject({ kind: "campaigns" });
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(CLOUD_ORCHESTRATOR_PLANS_PATH)}&legacy_admin_path=${encodeURIComponent(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH)}`,
      ),
    ).toBeUndefined();
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(CLOUD_ORCHESTRATOR_PLANS_PATH)}&result_token=secret`,
      ),
    ).toBeUndefined();
  });

  it.each([
    ["customer_selection", "1"],
    ["segment_members", "42"],
    ["ai_audience_package_members", "9223372036854775807"],
  ])("restores the exact %s source pair losslessly", (sourceKind, sourceID) => {
    const inner = `${CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH}?source_kind=${sourceKind}&source_id=${sourceID}`;
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(inner)}`,
      ),
    ).toEqual({
      kind: "campaigns",
      pathname: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
      source_kind: sourceKind,
      source_id: sourceID,
    });
  });

  it("accepts the closed pair in either order and returns a canonical route", () => {
    const inner = `${CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH}?source_id=7&source_kind=segment_members`;
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(inner)}`,
      ),
    ).toEqual({
      kind: "campaigns",
      pathname: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
      source_kind: "segment_members",
      source_id: "7",
    });
  });

  it.each([
    "source_kind=segment_members",
    "source_id=7",
    "source_kind=segment_members&source_id=7&return_to=customers",
    "source_kind=segment_members&source_kind=customer_selection&source_id=7",
    "source_kind=segment_members&source_id=7&source_id=8",
    "source_kind=unknown&source_id=7",
    "source_kind=segment_members&source_id=",
    "source_kind=segment_members&source_id=0",
    "source_kind=segment_members&source_id=-1",
    "source_kind=segment_members&source_id=01",
    "source_kind=segment_members&source_id=+1",
    "source_kind=segment_members&source_id=%2B1",
    "source_kind=segment_members&source_id=%201",
    "source_kind=segment_members&source_id=1%20",
    "source_kind=segment_members&source_id=9223372036854775808",
    "source_kind=segment_members&source_id=10000000000000000000",
    "source_kind=segment_members&source_id=%37",
    "source%5Fkind=segment_members&source_id=7",
    "source_kind=segment%5Fmembers&source_id=7",
    "source_kind=segment_members&source_id=%zz",
  ])("rejects malformed or noncanonical inner query %s", (rawQuery) => {
    const inner = `${CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH}?${rawQuery}`;
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(inner)}`,
      ),
    ).toBeUndefined();
  });

  it("rejects query context on every other Cloud route and noncanonical outer encoding", () => {
    const plans = `${CLOUD_ORCHESTRATOR_PLANS_PATH}?source_kind=segment_members&source_id=7`;
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(plans)}`,
      ),
    ).toBeUndefined();

    const campaign = `${CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH}?source_kind=segment_members&source_id=7`;
    expect(
      cloudOrchestratorCarrierRoute(`?legacy_admin_path=${campaign}`),
    ).toBeUndefined();
    expect(
      cloudOrchestratorCarrierRoute(
        `?legacy%5Fadmin_path=${encodeURIComponent(campaign)}`,
      ),
    ).toBeUndefined();
  });

  it("exposes only the three frozen workspace links", () => {
    expect(cloudOrchestratorWorkspaceLinks).toEqual([
      { href: CLOUD_ORCHESTRATOR_PLANS_PATH, label: "运营计划" },
      { href: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, label: "Campaign 审阅" },
      { href: CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH, label: "可观察性" },
    ]);
  });
});
