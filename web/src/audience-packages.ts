export const AUDIENCE_PACKAGES_PATH = "/admin/automation-conversion";
export const AUDIENCE_PACKAGE_DETAIL_PREFIX =
  "/admin/automation-conversion/packages/";

export type AudiencePackageRole = "admin" | "ops" | "sales";

export type AudiencePackageRoute =
  | {
      readonly kind: "packages";
      readonly pathname: typeof AUDIENCE_PACKAGES_PATH;
    }
  | {
      readonly kind: "package_detail";
      readonly pathname: string;
      readonly packageID: string;
    };

function safePackageID(encoded: string): string | undefined {
  let decoded: string;
  try {
    decoded = decodeURIComponent(encoded);
  } catch {
    return undefined;
  }
  if (!/^[1-9]\d{0,18}$/u.test(decoded)) return undefined;
  return BigInt(decoded) <= 9_223_372_036_854_775_807n ? decoded : undefined;
}

export function audiencePackageRoute(
  pathname: string,
): AudiencePackageRoute | undefined {
  if (pathname === AUDIENCE_PACKAGES_PATH) {
    return { kind: "packages", pathname };
  }
  if (!pathname.startsWith(AUDIENCE_PACKAGE_DETAIL_PREFIX)) {
    return undefined;
  }
  const packageID = safePackageID(
    pathname.slice(AUDIENCE_PACKAGE_DETAIL_PREFIX.length),
  );
  return packageID
    ? { kind: "package_detail", pathname, packageID }
    : undefined;
}

export function audiencePackageCarrierRoute(
  search: string,
): AudiencePackageRoute | undefined {
  if (search === "") return undefined;
  let parameters: URLSearchParams;
  try {
    parameters = new URLSearchParams(search);
  } catch {
    return undefined;
  }
  const entries = [...parameters.entries()];
  if (entries.length !== 1 || entries[0][0] !== "legacy_admin_path") {
    return undefined;
  }
  return audiencePackageRoute(entries[0][1]);
}

export const audiencePackageWorkspaceLinks = [
  { href: AUDIENCE_PACKAGES_PATH, label: "人群包工作区" },
] as const;
