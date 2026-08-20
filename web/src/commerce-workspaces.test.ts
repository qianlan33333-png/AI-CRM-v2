import { describe, expect, it } from "vitest";
import {
  ALIPAY_TRANSACTIONS_PATH,
  SERVICE_PRODUCT_NEW_PATH,
  SERVICE_PRODUCTS_PATH,
  WECHAT_PAY_PRODUCT_NEW_PATH,
  WECHAT_PAY_PRODUCTS_PATH,
  WECHAT_PAY_TRANSACTIONS_PATH,
  WECHAT_SHOP_TRANSACTIONS_PATH,
  commerceWorkspaceCarrierRoute,
  commerceWorkspaceLinks,
  commerceWorkspaceRoute,
} from "./commerce-workspaces";

describe("commerce workspace routes", () => {
  it.each([
    [ALIPAY_TRANSACTIONS_PATH, "alipay_transactions"],
    [SERVICE_PRODUCTS_PATH, "service_products"],
    [SERVICE_PRODUCT_NEW_PATH, "service_product_new"],
    [`${SERVICE_PRODUCTS_PATH}/service_A-42/edit`, "service_product_edit"],
    [`${SERVICE_PRODUCTS_PATH}/service_A-42/data`, "service_product_data"],
    [WECHAT_PAY_PRODUCT_NEW_PATH, "wechat_pay_product_new"],
    [
      `${WECHAT_PAY_PRODUCTS_PATH}/product_A-42/edit`,
      "wechat_pay_product_edit",
    ],
    [WECHAT_PAY_TRANSACTIONS_PATH, "wechat_pay_transactions"],
    [`${WECHAT_PAY_TRANSACTIONS_PATH}/order_A-42`, "wechat_pay_transaction"],
    [WECHAT_SHOP_TRANSACTIONS_PATH, "wechat_shop_transactions"],
    [`${WECHAT_SHOP_TRANSACTIONS_PATH}/order_A-42`, "wechat_shop_transaction"],
  ])("accepts %s as %s", (pathname, kind) => {
    expect(commerceWorkspaceRoute(pathname)).toMatchObject({ pathname, kind });
  });

  it("decodes only a display identifier", () => {
    expect(
      commerceWorkspaceRoute(`${WECHAT_PAY_TRANSACTIONS_PATH}/order%20one`),
    ).toMatchObject({ resourceID: "order one" });
  });

  it("uses the frozen 200-code-point identifier limit", () => {
    expect(
      commerceWorkspaceRoute(
        `${WECHAT_PAY_TRANSACTIONS_PATH}/${"交".repeat(200)}`,
      ),
    ).toBeDefined();
    expect(
      commerceWorkspaceRoute(
        `${WECHAT_PAY_TRANSACTIONS_PATH}/${"交".repeat(201)}`,
      ),
    ).toBeUndefined();
  });

  it.each([
    "",
    "/admin/service-period-products/unknown",
    `${SERVICE_PRODUCTS_PATH}/`,
    `${SERVICE_PRODUCTS_PATH}/service/nested/edit`,
    `${SERVICE_PRODUCTS_PATH}/%2Fescaped/edit`,
    `${WECHAT_PAY_TRANSACTIONS_PATH}/order/nested`,
    `${WECHAT_PAY_TRANSACTIONS_PATH}/order%252Fescape`,
    `${WECHAT_PAY_TRANSACTIONS_PATH}/order%09tab`,
    `${WECHAT_SHOP_TRANSACTIONS_PATH}/%0Aheader`,
  ])("rejects an unapproved or unsafe path %s", (pathname) => {
    expect(commerceWorkspaceRoute(pathname)).toBeUndefined();
  });

  it("accepts only one exact carrier parameter", () => {
    expect(
      commerceWorkspaceCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(WECHAT_PAY_TRANSACTIONS_PATH)}`,
      ),
    ).toMatchObject({ kind: "wechat_pay_transactions" });
    expect(
      commerceWorkspaceCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(WECHAT_PAY_TRANSACTIONS_PATH)}&result_token=secret`,
      ),
    ).toBeUndefined();
    expect(
      commerceWorkspaceCarrierRoute(
        `?legacy_admin_path=${encodeURIComponent(WECHAT_PAY_TRANSACTIONS_PATH)}&legacy_admin_path=${encodeURIComponent(ALIPAY_TRANSACTIONS_PATH)}`,
      ),
    ).toBeUndefined();
  });

  it("exposes only the five frozen top-level workspaces", () => {
    expect(commerceWorkspaceLinks).toHaveLength(5);
    expect(commerceWorkspaceLinks.map((link) => link.href)).toEqual([
      SERVICE_PRODUCTS_PATH,
      WECHAT_PAY_PRODUCTS_PATH,
      WECHAT_PAY_TRANSACTIONS_PATH,
      WECHAT_SHOP_TRANSACTIONS_PATH,
      ALIPAY_TRANSACTIONS_PATH,
    ]);
  });
});
