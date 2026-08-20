import { describe, expect, it } from "vitest";
import {
  USER_OPS_PATH,
  USER_OPS_UI_PATH,
  userOpsCarrierRoute,
  userOpsRoute,
} from "./user-ops";

describe("user operations closed review route", () => {
  it.each([USER_OPS_PATH, USER_OPS_UI_PATH])(
    "accepts the approved page %s",
    (pathname) => {
      expect(userOpsRoute(pathname)).toEqual({
        kind: "review_workspace",
        pathname,
      });
    },
  );

  it.each([
    "/admin/user-ops/",
    "/admin/user-ops/unknown",
    "/admin/user-ops/batch-send/preview",
    "/admin/user-ops/batch-send/execute",
    "/admin/user-ops/send-records",
    "/api/admin/user-ops",
  ])("rejects an API or action-shaped route %s", (pathname) => {
    expect(userOpsRoute(pathname)).toBeUndefined();
  });

  it("accepts one exact carrier parameter and rejects parameter drift", () => {
    expect(
      userOpsCarrierRoute(
        "?legacy_admin_path=%2Fadmin%2Fuser-ops%2Fui",
      ),
    ).toEqual({ kind: "review_workspace", pathname: USER_OPS_UI_PATH });
    expect(
      userOpsCarrierRoute(
        "?legacy_admin_path=%2Fadmin%2Fuser-ops&confirm=true",
      ),
    ).toBeUndefined();
    expect(
      userOpsCarrierRoute(
        "?legacy_admin_path=%2Fadmin%2Fuser-ops&selected=customer-1",
      ),
    ).toBeUndefined();
    expect(userOpsCarrierRoute("?other=value")).toBeUndefined();
  });
});
