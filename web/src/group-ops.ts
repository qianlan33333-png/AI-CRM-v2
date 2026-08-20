export const GROUP_OPS_PLANS_PATH =
  "/admin/automation-conversion/group-ops/ui";
export const GROUP_OPS_PLAN_DETAIL_PREFIX =
  "/admin/automation-conversion/group-ops/plans/";

export type GroupOpsRole = "admin" | "ops" | "sales";

export type GroupOpsRoute =
  | {
      readonly kind: "plans";
      readonly pathname: typeof GROUP_OPS_PLANS_PATH;
    }
  | {
      readonly kind: "plan_detail";
      readonly pathname: string;
      readonly planID: string;
    };

function safePlanID(encoded: string): string | undefined {
  let decoded: string;
  try {
    decoded = decodeURIComponent(encoded);
  } catch {
    return undefined;
  }
  // The frozen legacy page contract uses a positive integer identifier. Keep
  // it as text after a lossless bigint range check so 64-bit IDs are never
  // rounded by JavaScript.
  if (!/^[1-9]\d{0,18}$/u.test(decoded)) return undefined;
  return BigInt(decoded) <= 9_223_372_036_854_775_807n ? decoded : undefined;
}

export function groupOpsRoute(pathname: string): GroupOpsRoute | undefined {
  if (pathname === GROUP_OPS_PLANS_PATH) {
    return { kind: "plans", pathname };
  }
  if (!pathname.startsWith(GROUP_OPS_PLAN_DETAIL_PREFIX)) {
    return undefined;
  }
  const planID = safePlanID(pathname.slice(GROUP_OPS_PLAN_DETAIL_PREFIX.length));
  return planID ? { kind: "plan_detail", pathname, planID } : undefined;
}

export function groupOpsCarrierRoute(search: string): GroupOpsRoute | undefined {
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
  return groupOpsRoute(entries[0][1]);
}

export const groupOpsWorkspaceLinks = [
  { href: GROUP_OPS_PLANS_PATH, label: "计划工作区" },
] as const;
