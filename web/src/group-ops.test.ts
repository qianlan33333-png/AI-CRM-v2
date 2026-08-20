import { describe, expect, it } from "vitest";
import {
  GROUP_OPS_PLANS_PATH,
  groupOpsCarrierRoute,
  groupOpsRoute,
} from "./group-ops";
import { getGetGroupOpsPlanDetailWorkspaceUrl } from "./api/generated/health";

describe("group operations closed route parser", () => {
  it("recognizes only the two frozen page shapes", () => {
    expect(groupOpsRoute(GROUP_OPS_PLANS_PATH)).toEqual({
      kind: "plans",
      pathname: GROUP_OPS_PLANS_PATH,
    });
    expect(
      groupOpsRoute("/admin/automation-conversion/group-ops/plans/42"),
    ).toEqual({
      kind: "plan_detail",
      pathname: "/admin/automation-conversion/group-ops/plans/42",
      planID: "42",
    });
    expect(
      groupOpsRoute(
        "/admin/automation-conversion/group-ops/plans/9223372036854775807",
      ),
    ).toMatchObject({ kind: "plan_detail", planID: "9223372036854775807" });
  });

  it.each([
    "/admin/automation-conversion/group-ops",
    "/admin/automation-conversion/group-ops/groups/ui",
    "/admin/automation-conversion/group-ops/plans/",
    "/admin/automation-conversion/group-ops/plans/0",
    "/admin/automation-conversion/group-ops/plans/-1",
    "/admin/automation-conversion/group-ops/plans/01",
    "/admin/automation-conversion/group-ops/plans/42/nodes",
    "/admin/automation-conversion/group-ops/plans/42%2Fnodes",
    "/admin/automation-conversion/group-ops/plans/9223372036854775808",
  ])("rejects an unfrozen or unsafe route %s", (pathname) => {
    expect(groupOpsRoute(pathname)).toBeUndefined();
  });

  it("accepts one exact carrier parameter and rejects parameter drift", () => {
    expect(
      groupOpsCarrierRoute(
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fplans%2F42",
      ),
    ).toMatchObject({ kind: "plan_detail", planID: "42" });
    expect(
      groupOpsCarrierRoute(
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fui&execute=true",
      ),
    ).toBeUndefined();
    expect(groupOpsCarrierRoute("?other=value")).toBeUndefined();
  });

  it("keeps an int64 identifier above JavaScript's safe integer limit intact", () => {
    expect(getGetGroupOpsPlanDetailWorkspaceUrl("9007199254740993")).toBe(
      "/admin/automation-conversion/group-ops/plans/9007199254740993",
    );
  });
});
