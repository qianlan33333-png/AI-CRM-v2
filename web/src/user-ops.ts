export const USER_OPS_PATH = "/admin/user-ops";
export const USER_OPS_UI_PATH = "/admin/user-ops/ui";

export type UserOpsRole = "admin" | "ops" | "sales";

export interface UserOpsRoute {
  readonly kind: "review_workspace";
  readonly pathname: typeof USER_OPS_PATH | typeof USER_OPS_UI_PATH;
}

export function userOpsRoute(pathname: string): UserOpsRoute | undefined {
  if (pathname !== USER_OPS_PATH && pathname !== USER_OPS_UI_PATH) {
    return undefined;
  }
  return { kind: "review_workspace", pathname };
}

export function userOpsCarrierRoute(search: string): UserOpsRoute | undefined {
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
  return userOpsRoute(entries[0][1]);
}
