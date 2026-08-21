import { describe, expect, it } from "vitest";
import {
  ADMIN_CUSTOMER_INVALID_PATH,
  ADMIN_CUSTOMER_LIST_PATH,
  CUSTOMER_LIST_PATH,
  adminCustomerContextPath,
  adminCustomerDetailPath,
  customerAdminCarrierPathname,
  customerPageRoute,
  isAdminCustomerNamespace,
  parseCanonicalCustomerID,
} from "./customer-admin-routes";

describe("customer admin route aliases", () => {
  it("keeps the canonical list and closes both admin aliases over one safe id", () => {
    expect(customerPageRoute(CUSTOMER_LIST_PATH)).toMatchObject({
      kind: "list",
      adminAlias: false,
    });
    expect(customerPageRoute(ADMIN_CUSTOMER_LIST_PATH)).toMatchObject({
      kind: "list",
      adminAlias: true,
    });
    expect(customerPageRoute("/admin/customers/42")).toEqual({
      kind: "detail",
      pathname: "/admin/customers/42",
      customerID: 42,
      adminAlias: true,
      listPath: ADMIN_CUSTOMER_LIST_PATH,
      contextPath: "/admin/customer-360/42",
    });
    expect(customerPageRoute("/admin/customer-360/42")).toEqual({
      kind: "context",
      pathname: "/admin/customer-360/42",
      customerID: 42,
      adminAlias: true,
      listPath: ADMIN_CUSTOMER_LIST_PATH,
      detailPath: "/admin/customers/42",
    });
  });

  it.each([
    "",
    "0",
    "-1",
    "+1",
    "01",
    "1.0",
    " 1",
    "1 ",
    "9007199254740992",
    "18446744073709551615",
    "not-a-number",
  ])("rejects non-canonical id %s", (value) => {
    expect(parseCanonicalCustomerID(value)).toBeUndefined();
  });

  it.each([
    "/admin/customers/",
    "/admin/customers/0",
    "/admin/customers/-1",
    "/admin/customers/01",
    "/admin/customers/9007199254740992",
    "/admin/customers/not-a-number",
    "/admin/customers/1/extra",
    "/admin/customers/1\\extra",
    "/admin\\customers\\1",
    "/admin/customers/1%2Fextra",
    "/admin%2Fcustomers%2F1",
    "/admin/customers/1%5Cextra",
    "/admin/customer-360",
    "/admin/customer-360/",
    "/admin/customer-360/0",
    "/admin/customer-360/01",
    "/admin/customer-360/1/extra",
    "/admin/customer-360/1%2Fextra",
    "/admin%2Fcustomer-360%2F1",
  ])("fails closed for malformed pathname %s", (pathname) => {
    expect(customerPageRoute(pathname)).toBeUndefined();
  });

  it("accepts exactly one closed carrier value", () => {
    expect(
      customerAdminCarrierPathname(
        "?legacy_admin_path=%2Fadmin%2Fcustomers",
      ),
    ).toBe(ADMIN_CUSTOMER_LIST_PATH);
    expect(
      customerAdminCarrierPathname(
        "?legacy_admin_path=%2Fadmin%2Fcustomers%2F42",
      ),
    ).toBe("/admin/customers/42");
    expect(
      customerAdminCarrierPathname(
        "?legacy_admin_path=%2Fadmin%2Fcustomer-360%2F42",
      ),
    ).toBe("/admin/customer-360/42");
  });

  it.each([
    "?legacy_admin_path=%2Fadmin%2Fcustomers%2F0",
    "?legacy_admin_path=%2Fadmin%2Fcustomers%2F01",
    "?legacy_admin_path=%2Fadmin%2Fcustomers%2Flegacy-text-key",
    "?legacy_admin_path=%2Fadmin%2Fcustomers%2F1%2Fextra",
    "?legacy_admin_path=%2Fadmin%2Fcustomers%2F1%5Cextra",
    "?legacy_admin_path=%2Fadmin%252Fcustomers%252F1",
    "?legacy_admin_path=%2Fadmin%2Fcustomers%2F1&extra=1",
    "?legacy_admin_path=%2Fadmin%2Fcustomers%2F1&legacy_admin_path=%2Fadmin%2Fcustomers%2F2",
  ])("maps malformed customer carrier to a fixed missing route: %s", (search) => {
    expect(customerAdminCarrierPathname(search)).toBe(
      ADMIN_CUSTOMER_INVALID_PATH,
    );
  });

  it("ignores unrelated carriers and never broadens canonical routes", () => {
    expect(
      customerAdminCarrierPathname("?legacy_admin_path=%2Fcustomers%2F42"),
    ).toBeUndefined();
    expect(
      customerAdminCarrierPathname(
        "?legacy_admin_path=%2Fadmin%2Fquestionnaires",
      ),
    ).toBeUndefined();
  });

  it("builds paths only from safe integers", () => {
    expect(adminCustomerDetailPath(42)).toBe("/admin/customers/42");
    expect(adminCustomerContextPath(42)).toBe("/admin/customer-360/42");
    for (const value of [0, -1, Number.MAX_SAFE_INTEGER + 1, Number.NaN]) {
      expect(adminCustomerDetailPath(value)).toBe(ADMIN_CUSTOMER_INVALID_PATH);
      expect(adminCustomerContextPath(value)).toBe(ADMIN_CUSTOMER_INVALID_PATH);
    }
  });

  it("recognizes malformed admin namespaces without reflecting their values", () => {
    expect(isAdminCustomerNamespace("/admin/customers/1/extra")).toBe(true);
    expect(isAdminCustomerNamespace("/admin\\customers\\1")).toBe(true);
    expect(isAdminCustomerNamespace("/admin%2Fcustomers%2F1")).toBe(true);
    expect(isAdminCustomerNamespace("/admin%252Fcustomers%252F1")).toBe(true);
    expect(isAdminCustomerNamespace("/admin/customer-360/1%2Fextra")).toBe(
      true,
    );
    expect(isAdminCustomerNamespace("/admin/customers-other/1")).toBe(false);
  });
});
