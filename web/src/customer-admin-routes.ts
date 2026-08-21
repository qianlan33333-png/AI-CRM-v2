export const CUSTOMER_LIST_PATH = "/customers";
export const ADMIN_CUSTOMER_LIST_PATH = "/admin/customers";
export const ADMIN_CUSTOMER_360_ROOT_PATH = "/admin/customer-360";
export const ADMIN_CUSTOMER_INVALID_PATH = "/admin/customers/_invalid";
export const LEGACY_ADMIN_PATH_PARAM = "legacy_admin_path";

interface CustomerListRoute {
  readonly kind: "list";
  readonly pathname: typeof CUSTOMER_LIST_PATH | typeof ADMIN_CUSTOMER_LIST_PATH;
  readonly adminAlias: boolean;
}

interface CustomerDetailRoute {
  readonly kind: "detail";
  readonly pathname: string;
  readonly customerID: number;
  readonly adminAlias: boolean;
  readonly listPath: typeof CUSTOMER_LIST_PATH | typeof ADMIN_CUSTOMER_LIST_PATH;
  readonly contextPath: string;
}

interface CustomerContextRoute {
  readonly kind: "context";
  readonly pathname: string;
  readonly customerID: number;
  readonly adminAlias: true;
  readonly listPath: typeof ADMIN_CUSTOMER_LIST_PATH;
  readonly detailPath: string;
}

export type CustomerPageRoute =
  | CustomerListRoute
  | CustomerDetailRoute
  | CustomerContextRoute;

const canonicalCustomerID = /^[1-9][0-9]*$/;

export function parseCanonicalCustomerID(value: string): number | undefined {
  if (!canonicalCustomerID.test(value)) return undefined;
  const customerID = Number(value);
  return Number.isSafeInteger(customerID) && customerID > 0
    ? customerID
    : undefined;
}

function parseDetailPath(
  pathname: string,
  prefix: string,
): number | undefined {
  if (!pathname.startsWith(prefix)) return undefined;
  const value = pathname.slice(prefix.length);
  if (value === "" || value.includes("/")) return undefined;
  return parseCanonicalCustomerID(value);
}

export function adminCustomerDetailPath(customerID: number): string {
  if (!Number.isSafeInteger(customerID) || customerID < 1) {
    return ADMIN_CUSTOMER_INVALID_PATH;
  }
  return `${ADMIN_CUSTOMER_LIST_PATH}/${customerID}`;
}

export function adminCustomerContextPath(customerID: number): string {
  if (!Number.isSafeInteger(customerID) || customerID < 1) {
    return ADMIN_CUSTOMER_INVALID_PATH;
  }
  return `${ADMIN_CUSTOMER_360_ROOT_PATH}/${customerID}`;
}

export function customerPageRoute(
  pathname: string,
): CustomerPageRoute | undefined {
  if (pathname.includes("%") || pathname.includes("\\")) return undefined;

  if (pathname === CUSTOMER_LIST_PATH) {
    return { kind: "list", pathname: CUSTOMER_LIST_PATH, adminAlias: false };
  }
  if (pathname === ADMIN_CUSTOMER_LIST_PATH) {
    return {
      kind: "list",
      pathname: ADMIN_CUSTOMER_LIST_PATH,
      adminAlias: true,
    };
  }

  const canonicalID = parseDetailPath(pathname, `${CUSTOMER_LIST_PATH}/`);
  if (canonicalID !== undefined) {
    return {
      kind: "detail",
      pathname,
      customerID: canonicalID,
      adminAlias: false,
      listPath: CUSTOMER_LIST_PATH,
      contextPath: "#customer-360-summary",
    };
  }

  const adminID = parseDetailPath(
    pathname,
    `${ADMIN_CUSTOMER_LIST_PATH}/`,
  );
  if (adminID !== undefined) {
    return {
      kind: "detail",
      pathname,
      customerID: adminID,
      adminAlias: true,
      listPath: ADMIN_CUSTOMER_LIST_PATH,
      contextPath: adminCustomerContextPath(adminID),
    };
  }

  const contextID = parseDetailPath(
    pathname,
    `${ADMIN_CUSTOMER_360_ROOT_PATH}/`,
  );
  if (contextID !== undefined) {
    return {
      kind: "context",
      pathname,
      customerID: contextID,
      adminAlias: true,
      listPath: ADMIN_CUSTOMER_LIST_PATH,
      detailPath: adminCustomerDetailPath(contextID),
    };
  }

  return undefined;
}

function namespaceForms(root: string): readonly string[] {
  return [root, `/${root.slice(1).replaceAll("/", "\\")}`];
}

function rawCustomerNamespaceValue(value: string): boolean {
  return [ADMIN_CUSTOMER_LIST_PATH, ADMIN_CUSTOMER_360_ROOT_PATH].some(
    (root) =>
      namespaceForms(root).some(
        (form) =>
          value === form ||
          value.startsWith(`${form}/`) ||
          value.startsWith(`${form}\\`),
      ),
  );
}

function customerNamespaceValue(value: string): boolean {
  let candidate = value;
  for (let pass = 0; pass < 3; pass += 1) {
    if (rawCustomerNamespaceValue(candidate)) return true;
    try {
      const decoded = decodeURIComponent(candidate);
      if (decoded === candidate) return false;
      candidate = decoded;
    } catch {
      return false;
    }
  }
  return false;
}

export function isAdminCustomerNamespace(pathname: string): boolean {
  return customerNamespaceValue(pathname);
}

// The carrier is deliberately closed: one exact query entry, one recognized
// admin customer pathname, and no reflected invalid value. A customer-shaped
// malformed value maps to one fixed missing route so the page can offer the
// safe list return without retaining attacker-controlled text.
export function customerAdminCarrierPathname(
  search: string,
): string | undefined {
  if (search === "") return undefined;

  let params: URLSearchParams;
  try {
    params = new URLSearchParams(search);
  } catch {
    return undefined;
  }
  const entries = [...params.entries()];
  const customerValue = entries.find(
    ([key, value]) =>
      key === LEGACY_ADMIN_PATH_PARAM && customerNamespaceValue(value),
  );
  if (!customerValue) return undefined;
  if (
    entries.length !== 1 ||
    customerValue[0] !== LEGACY_ADMIN_PATH_PARAM
  ) {
    return ADMIN_CUSTOMER_INVALID_PATH;
  }

  const route = customerPageRoute(customerValue[1]);
  return route?.adminAlias ? route.pathname : ADMIN_CUSTOMER_INVALID_PATH;
}
