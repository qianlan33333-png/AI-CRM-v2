export const CLOUD_ORCHESTRATOR_ROOT_PATH = "/admin/cloud-orchestrator";
export const CLOUD_ORCHESTRATOR_PLANS_PATH = "/admin/cloud-orchestrator/plans";
export const CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH =
  "/admin/cloud-orchestrator/campaigns";
export const CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH =
  "/admin/cloud-orchestrator/observability";

export type CloudOrchestratorRole = "admin" | "ops" | "sales";

export type CloudCampaignSourceKind =
  "customer_selection" | "segment_members" | "ai_audience_package_members";

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
      readonly source_kind?: undefined;
      readonly source_id?: undefined;
    }
  | {
      readonly kind: "campaigns";
      readonly pathname: typeof CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH;
      readonly source_kind: CloudCampaignSourceKind;
      readonly source_id: string;
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

function sourceKind(value: string): CloudCampaignSourceKind | undefined {
  return value === "customer_selection" ||
    value === "segment_members" ||
    value === "ai_audience_package_members"
    ? value
    : undefined;
}

function sourceID(value: string): string | undefined {
  if (!/^[1-9][0-9]{0,18}$/u.test(value)) return undefined;
  const parsed = BigInt(value);
  if (parsed > 9_223_372_036_854_775_807n) return undefined;
  return value;
}

function campaignRoute(
  search: string,
  requireCanonicalOrder: boolean,
): Extract<CloudOrchestratorRoute, { readonly kind: "campaigns" }> | undefined {
  if (search === "") {
    return { kind: "campaigns", pathname: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH };
  }
  let query: URLSearchParams;
  try {
    query = new URLSearchParams(search);
  } catch {
    return undefined;
  }
  const entries = [...query.entries()];
  if (entries.length !== 2) return undefined;
  const kind = sourceKind(query.get("source_kind") ?? "");
  const id = sourceID(query.get("source_id") ?? "");
  if (!kind || !id || query.getAll("source_kind").length !== 1 || query.getAll("source_id").length !== 1) {
    return undefined;
  }
  const canonical = `source_kind=${kind}&source_id=${id}`;
  const reverse = `source_id=${id}&source_kind=${kind}`;
  const raw = search.startsWith("?") ? search.slice(1) : search;
  if (raw !== canonical && (requireCanonicalOrder || raw !== reverse)) return undefined;
  return {
    kind: "campaigns",
    pathname: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
    source_kind: kind,
    source_id: id,
  };
}

export function campaignSourceHref(
  kind: CloudCampaignSourceKind,
  id: string | number,
): string | undefined {
  const canonicalKind = sourceKind(kind);
  const canonicalID =
    typeof id === "number" && (!Number.isSafeInteger(id) || id < 1)
      ? undefined
      : sourceID(String(id));
  return canonicalKind && canonicalID
    ? `${CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH}?source_kind=${canonicalKind}&source_id=${canonicalID}`
    : undefined;
}

// This parser recognizes only the five approved page capabilities. It does
// not infer a plan state, audience, approval payload, quality metric, or
// executable workflow from the URL.
export function cloudOrchestratorRoute(
  pathname: string,
  search = "",
): CloudOrchestratorRoute | undefined {
  if (pathname === CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH) {
    return campaignRoute(search, true);
  }
  if (search !== "") return undefined;
  switch (pathname) {
    case CLOUD_ORCHESTRATOR_ROOT_PATH:
      return { kind: "root", pathname };
    case CLOUD_ORCHESTRATOR_PLANS_PATH:
      return { kind: "plans", pathname };
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
  const inner = entries[0][1];
  if (search !== `?legacy_admin_path=${encodeURIComponent(inner)}`) {
    return undefined;
  }

  const separator = inner.indexOf("?");
  if (separator < 0) return cloudOrchestratorRoute(inner);
  if (inner.slice(0, separator) !== CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH) {
    return undefined;
  }

  return campaignRoute(inner.slice(separator + 1), false);
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
