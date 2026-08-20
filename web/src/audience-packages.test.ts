import { describe, expect, it } from "vitest";
import {
  AUDIENCE_PACKAGES_PATH,
  audiencePackageCarrierRoute,
  audiencePackageRoute,
} from "./audience-packages";

describe("audience package closed route parser", () => {
  it("recognizes only the list and detail workspace shapes", () => {
    expect(audiencePackageRoute(AUDIENCE_PACKAGES_PATH)).toEqual({
      kind: "packages",
      pathname: AUDIENCE_PACKAGES_PATH,
    });
    expect(
      audiencePackageRoute("/admin/automation-conversion/packages/42"),
    ).toEqual({
      kind: "package_detail",
      pathname: "/admin/automation-conversion/packages/42",
      packageID: "42",
    });
    expect(
      audiencePackageRoute(
        "/admin/automation-conversion/packages/9223372036854775807",
      ),
    ).toMatchObject({
      kind: "package_detail",
      packageID: "9223372036854775807",
    });
  });

  it.each([
    "/admin/automation-conversion/",
    "/admin/automation-conversion/programs/retired",
    "/admin/automation-conversion/packages/",
    "/admin/automation-conversion/packages/0",
    "/admin/automation-conversion/packages/-1",
    "/admin/automation-conversion/packages/01",
    "/admin/automation-conversion/packages/42/members",
    "/admin/automation-conversion/packages/42%2Fmembers",
    "/admin/automation-conversion/packages/9223372036854775808",
    "/api/admin/ai-audience/packages",
  ])("rejects an unfrozen or unsafe route %s", (pathname) => {
    expect(audiencePackageRoute(pathname)).toBeUndefined();
  });

  it("accepts one exact carrier parameter and rejects parameter drift", () => {
    expect(
      audiencePackageCarrierRoute(
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fpackages%2F9007199254740993",
      ),
    ).toMatchObject({
      kind: "package_detail",
      packageID: "9007199254740993",
    });
    expect(
      audiencePackageCarrierRoute(
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion&execute=true",
      ),
    ).toBeUndefined();
    expect(audiencePackageCarrierRoute("?other=value")).toBeUndefined();
  });
});
