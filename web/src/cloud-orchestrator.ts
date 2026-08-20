export const CLOUD_ORCHESTRATOR_ROOT_PATH = "/admin/cloud-orchestrator";
export const CLOUD_ORCHESTRATOR_PLANS_PATH = "/admin/cloud-orchestrator/plans";
export const CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH =
  "/admin/cloud-orchestrator/campaigns";
export const CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH =
  "/admin/cloud-orchestrator/observability";

export type CloudOrchestratorRole = "admin" | "ops" | "sales";

export type CloudOrchestratorRoute =
  | {
      readonly kind: "root";
      readonly pathname: typeof CLOUD_ORCHESTRATOR_ROOT_PATH;
    }
  | {
      readonly kind: "plans";
      readonly pathname: typeof CLOUD_ORCHESTRATOR_PLANS_PATH;
    }
  | {
      readonly kind: "plan_detail";
      readonly pathname: string;
      readonly planID: string;
    }
  | {
      readonly kind: "campaigns";
      readonly pathname: typeof CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH;
    }
  | {
      readonly kind: "observability";
      readonly pathname: typeof CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH;
    };

function safePlanID(value: string): string | undefined {
  let decoded: string;
  try {
    decoded = decodeURIComponent(value);
  } catch {
    return undefined;
  }
  if (
    decoded.length === 0 ||
    decoded === "." ||
    decoded === ".." ||
    /[\/\\\u0000-\u001f\u007f]/u.test(decoded)
  ) {
    return undefined;
  }
  return decoded;
}

// This parser recognizes only the five approved page capabilities. It does
// not infer a plan state, audience, approval payload, quality metric, or
// executable workflow from the URL.
export function cloudOrchestratorRoute(
  pathname: string,
): CloudOrchestratorRoute | undefined {
  switch (pathname) {
    case CLOUD_ORCHESTRATOR_ROOT_PATH:
      return { kind: "root", pathname };
    case CLOUD_ORCHESTRATOR_PLANS_PATH:
      return { kind: "plans", pathname };
    case CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH:
      return { kind: "campaigns", pathname };
    case CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH:
      return { kind: "observability", pathname };
  }

  const prefix = `${CLOUD_ORCHESTRATOR_PLANS_PATH}/`;
  if (!pathname.startsWith(prefix)) return undefined;
  const encodedPlanID = pathname.slice(prefix.length);
  const planID = safePlanID(encodedPlanID);
  return planID ? { kind: "plan_detail", pathname, planID } : undefined;
}

export function cloudOrchestratorCarrierRoute(
  search: string,
): CloudOrchestratorRoute | undefined {
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
  return cloudOrchestratorRoute(entries[0][1]);
}

export interface CloudOrchestratorWorkspaceLink {
  readonly href: string;
  readonly label: string;
}

export const cloudOrchestratorWorkspaceLinks: readonly CloudOrchestratorWorkspaceLink[] =
  [
    { href: CLOUD_ORCHESTRATOR_PLANS_PATH, label: "运营计划" },
    { href: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, label: "Campaign 审阅" },
    { href: CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH, label: "可观察性" },
  ];
