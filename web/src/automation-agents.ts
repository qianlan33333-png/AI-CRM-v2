import { listLegacyAutomationAgents } from "./api/generated/health";

export type AutomationAgentsRole = "admin" | "ops" | "sales";
export type AutomationAgentType = "agent" | "fixed_script";
export type AutomationAgentStatus = "active" | "paused";

export interface AutomationAgentMaterialSummary {
  readonly imageCount: number;
  readonly miniprogramCount: number;
  readonly attachmentCount: number;
  readonly groupInviteCount: number;
}

export interface AutomationAgentSummary {
  readonly id: number;
  readonly type: AutomationAgentType;
  readonly typeLabel: "Agent 机器人" | "固定话术";
  readonly code: string;
  readonly name: string;
  readonly status: AutomationAgentStatus;
  readonly updatedAt: string;
  readonly materialSummary: AutomationAgentMaterialSummary;
}

export interface AutomationAgentsSnapshot {
  readonly items: readonly AutomationAgentSummary[];
  readonly total: number;
}

export interface AutomationAgentsTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedRead(
  options: RequestInit,
): Promise<AutomationAgentsTransportResponse> {
  return listLegacyAutomationAgents(options);
}

export interface AutomationAgentsTransport {
  readonly read: typeof generatedRead;
}

export const generatedAutomationAgentsTransport: AutomationAgentsTransport = {
  read: generatedRead,
};

export type AutomationAgentsFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "unavailable";

export type AutomationAgentsResult =
  | { readonly status: "loaded"; readonly snapshot: AutomationAgentsSnapshot }
  | { readonly status: AutomationAgentsFailure };

function record(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null)
  );
}

function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function text(value: unknown, minimum = 0, maximum = 120): value is string {
  return (
    typeof value === "string" &&
    Array.from(value).length >= minimum &&
    Array.from(value).length <= maximum
  );
}

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function timestamp(value: unknown): value is string {
  if (
    !text(value, 20, 64) ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) ||
    !Number.isFinite(Date.parse(value))
  ) {
    return false;
  }
  const match = value.match(
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/,
  );
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
  const calendar = new Date(0);
  calendar.setUTCFullYear(year, month - 1, day);
  calendar.setUTCHours(hour, minute, second, 0);
  return (
    calendar.getUTCFullYear() === year &&
    calendar.getUTCMonth() === month - 1 &&
    calendar.getUTCDate() === day &&
    calendar.getUTCHours() === hour &&
    calendar.getUTCMinutes() === minute &&
    calendar.getUTCSeconds() === second
  );
}

function materialSummary(value: unknown): AutomationAgentMaterialSummary | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "image_count",
      "miniprogram_count",
      "attachment_count",
      "group_invite_count",
    ]) ||
    !nonnegative(value.image_count) ||
    !nonnegative(value.miniprogram_count) ||
    !nonnegative(value.attachment_count) ||
    !nonnegative(value.group_invite_count)
  ) {
    return undefined;
  }
  return {
    imageCount: value.image_count,
    miniprogramCount: value.miniprogram_count,
    attachmentCount: value.attachment_count,
    groupInviteCount: value.group_invite_count,
  };
}

function parseItem(value: unknown): AutomationAgentSummary | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "id",
      "automation_type",
      "agent_code",
      "agent_name",
      "bound_package_key",
      "bound_package_id",
      "bound_package_name",
      "fixed_material_summary",
      "status",
      "updated_at",
    ]) ||
    !positive(value.id) ||
    (value.automation_type !== "agent" && value.automation_type !== "fixed_script") ||
    !text(value.agent_code, 1, 120) ||
    !/^[a-z0-9_-]+$/.test(value.agent_code) ||
    !text(value.agent_name, 1, 120) ||
    value.bound_package_key !== "" ||
    value.bound_package_id !== null ||
    value.bound_package_name !== "" ||
    (value.status !== "active" && value.status !== "paused") ||
    !timestamp(value.updated_at)
  ) {
    return undefined;
  }
  const summary = materialSummary(value.fixed_material_summary);
  if (!summary) return undefined;
  return {
    id: value.id,
    type: value.automation_type,
    typeLabel: value.automation_type === "agent" ? "Agent 机器人" : "固定话术",
    code: value.agent_code,
    name: value.agent_name,
    status: value.status,
    updatedAt: value.updated_at,
    materialSummary: summary,
  };
}

export function parseAutomationAgents(
  value: unknown,
): AutomationAgentsSnapshot | undefined {
  if (
    !record(value) ||
    !exact(value, ["ok", "items", "total"]) ||
    value.ok !== true ||
    !Array.isArray(value.items) ||
    value.items.length > 200 ||
    !nonnegative(value.total) ||
    value.total !== value.items.length
  ) {
    return undefined;
  }
  const items = value.items.map(parseItem);
  if (items.some((item) => item === undefined)) return undefined;
  const parsed = items as AutomationAgentSummary[];
  const ids = new Set(parsed.map((item) => item.id));
  if (ids.size !== parsed.length) return undefined;
  for (let index = 1; index < parsed.length; index += 1) {
    const previous = parsed[index - 1];
    const current = parsed[index];
    if (
      previous.updatedAt < current.updatedAt ||
      (previous.updatedAt === current.updatedAt && previous.id < current.id)
    ) {
      return undefined;
    }
  }
  return { items: parsed, total: value.total };
}

export function filterAutomationAgents(
  snapshot: AutomationAgentsSnapshot,
  keyword: string,
  type: AutomationAgentType | "all",
  status: AutomationAgentStatus | "all",
): readonly AutomationAgentSummary[] {
  const query = keyword.trim().toLocaleLowerCase();
  return snapshot.items.filter((item) =>
    (type === "all" || item.type === type) &&
    (status === "all" || item.status === status) &&
    (query === "" ||
      item.name.toLocaleLowerCase().includes(query) ||
      item.code.toLocaleLowerCase().includes(query)),
  );
}

export async function loadAutomationAgents(
  transport: AutomationAgentsTransport = generatedAutomationAgentsTransport,
): Promise<AutomationAgentsResult> {
  try {
    const response = await transport.read({ credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status !== 200) return { status: "unavailable" };
    const snapshot = parseAutomationAgents(response.data);
    return snapshot ? { status: "loaded", snapshot } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}
