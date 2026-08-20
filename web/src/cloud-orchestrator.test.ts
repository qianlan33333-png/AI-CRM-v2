import { describe, expect, it } from "vitest";
import {
  CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
  CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH,
  CLOUD_ORCHESTRATOR_PLANS_PATH,
  CLOUD_ORCHESTRATOR_ROOT_PATH,
  cloudOrchestratorCarrierRoute,
  cloudOrchestratorRoute,
  cloudOrchestratorWorkspaceLinks,
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

  it("exposes only the three frozen workspace links", () => {
    expect(cloudOrchestratorWorkspaceLinks).toEqual([
      { href: CLOUD_ORCHESTRATOR_PLANS_PATH, label: "运营计划" },
      { href: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, label: "Campaign 审阅" },
      { href: CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH, label: "可观察性" },
    ]);
  });
});
