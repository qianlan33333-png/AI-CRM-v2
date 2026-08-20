export const ALIPAY_TRANSACTIONS_PATH = "/admin/alipay/transactions";
export const SERVICE_PRODUCTS_PATH = "/admin/service-period-products";
export const SERVICE_PRODUCT_NEW_PATH = `${SERVICE_PRODUCTS_PATH}/new`;
export const WECHAT_PAY_PRODUCTS_PATH = "/admin/wechat-pay/products";
export const WECHAT_PAY_PRODUCT_NEW_PATH = `${WECHAT_PAY_PRODUCTS_PATH}/new`;
export const WECHAT_PAY_TRANSACTIONS_PATH = "/admin/wechat-pay/transactions";
export const WECHAT_SHOP_TRANSACTIONS_PATH = "/admin/wechat-shop/transactions";

export type CommerceWorkspaceRole = "admin" | "ops" | "sales";
export type CommerceWorkspaceRoute =
  | {
      readonly kind: "alipay_transactions";
      readonly pathname: typeof ALIPAY_TRANSACTIONS_PATH;
    }
  | {
      readonly kind: "service_products";
      readonly pathname: typeof SERVICE_PRODUCTS_PATH;
    }
  | {
      readonly kind: "service_product_new";
      readonly pathname: typeof SERVICE_PRODUCT_NEW_PATH;
    }
  | {
      readonly kind: "service_product_edit";
      readonly pathname: string;
      readonly resourceID: string;
    }
  | {
      readonly kind: "service_product_data";
      readonly pathname: string;
      readonly resourceID: string;
    }
  | {
      readonly kind: "wechat_pay_product_new";
      readonly pathname: typeof WECHAT_PAY_PRODUCT_NEW_PATH;
    }
  | {
      readonly kind: "wechat_pay_product_edit";
      readonly pathname: string;
      readonly resourceID: string;
    }
  | {
      readonly kind: "wechat_pay_transactions";
      readonly pathname: typeof WECHAT_PAY_TRANSACTIONS_PATH;
    }
  | {
      readonly kind: "wechat_pay_transaction";
      readonly pathname: string;
      readonly resourceID: string;
    }
  | {
      readonly kind: "wechat_shop_transactions";
      readonly pathname: typeof WECHAT_SHOP_TRANSACTIONS_PATH;
    }
  | {
      readonly kind: "wechat_shop_transaction";
      readonly pathname: string;
      readonly resourceID: string;
    };

function safeIdentifier(value: string): string | undefined {
  let decoded: string;
  try {
    decoded = decodeURIComponent(value);
  } catch {
    return undefined;
  }
  return decoded.length > 0 &&
    [...decoded].length <= 200 &&
    decoded !== "." &&
    decoded !== ".." &&
    !/[\/%\\\u0000-\u001f\u007f]/u.test(decoded)
    ? decoded
    : undefined;
}

function identifiedRoute(
  pathname: string,
  prefix: string,
  suffix: string,
  kind:
    | "service_product_edit"
    | "service_product_data"
    | "wechat_pay_product_edit"
    | "wechat_pay_transaction"
    | "wechat_shop_transaction",
): CommerceWorkspaceRoute | undefined {
  if (!pathname.startsWith(prefix) || !pathname.endsWith(suffix))
    return undefined;
  const encoded = pathname.slice(
    prefix.length,
    suffix.length === 0 ? undefined : -suffix.length,
  );
  const resourceID = safeIdentifier(encoded);
  return resourceID ? { kind, pathname, resourceID } : undefined;
}

// Only the eleven not-yet-owned frozen admin page routes are recognized. No query, product,
// member, payment, refund, or provider state is accepted from the browser.
export function commerceWorkspaceRoute(
  pathname: string,
): CommerceWorkspaceRoute | undefined {
  switch (pathname) {
    case ALIPAY_TRANSACTIONS_PATH:
      return { kind: "alipay_transactions", pathname };
    case SERVICE_PRODUCTS_PATH:
      return { kind: "service_products", pathname };
    case SERVICE_PRODUCT_NEW_PATH:
      return { kind: "service_product_new", pathname };
    case WECHAT_PAY_PRODUCT_NEW_PATH:
      return { kind: "wechat_pay_product_new", pathname };
    case WECHAT_PAY_TRANSACTIONS_PATH:
      return { kind: "wechat_pay_transactions", pathname };
    case WECHAT_SHOP_TRANSACTIONS_PATH:
      return { kind: "wechat_shop_transactions", pathname };
  }
  return (
    identifiedRoute(
      pathname,
      `${SERVICE_PRODUCTS_PATH}/`,
      "/edit",
      "service_product_edit",
    ) ??
    identifiedRoute(
      pathname,
      `${SERVICE_PRODUCTS_PATH}/`,
      "/data",
      "service_product_data",
    ) ??
    identifiedRoute(
      pathname,
      `${WECHAT_PAY_PRODUCTS_PATH}/`,
      "/edit",
      "wechat_pay_product_edit",
    ) ??
    identifiedRoute(
      pathname,
      `${WECHAT_PAY_TRANSACTIONS_PATH}/`,
      "",
      "wechat_pay_transaction",
    ) ??
    identifiedRoute(
      pathname,
      `${WECHAT_SHOP_TRANSACTIONS_PATH}/`,
      "",
      "wechat_shop_transaction",
    )
  );
}

export function commerceWorkspaceCarrierRoute(
  search: string,
): CommerceWorkspaceRoute | undefined {
  if (search === "") return undefined;
  let parameters: URLSearchParams;
  try {
    parameters = new URLSearchParams(search);
  } catch {
    return undefined;
  }
  const entries = [...parameters.entries()];
  return entries.length === 1 && entries[0][0] === "legacy_admin_path"
    ? commerceWorkspaceRoute(entries[0][1])
    : undefined;
}

export const commerceWorkspaceLinks = [
  { href: SERVICE_PRODUCTS_PATH, label: "周期商品" },
  { href: WECHAT_PAY_PRODUCTS_PATH, label: "微信支付商品" },
  { href: WECHAT_PAY_TRANSACTIONS_PATH, label: "微信支付交易" },
  { href: WECHAT_SHOP_TRANSACTIONS_PATH, label: "微信小店交易" },
  { href: ALIPAY_TRANSACTIONS_PATH, label: "支付宝交易" },
] as const;
