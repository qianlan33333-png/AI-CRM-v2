import { getLegacyHXCSendConfig } from "./api/generated/health";

export interface HXCSenderConfig {
  readonly id: string;
  readonly senderUserID: string;
  readonly displayName: string;
  readonly priority: number;
  readonly isActive: boolean;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface HXCDirectoryCandidate {
  readonly wecomUserID: string;
  readonly displayName: string;
  readonly position: "";
  readonly wecomStatus: 0;
  readonly isSender: boolean;
  readonly priority: number;
  readonly isActive: boolean;
}

export interface HXCSenderReadModel {
  readonly sendConfigs: readonly HXCSenderConfig[];
  readonly members: readonly HXCDirectoryCandidate[];
  readonly directoryCount: number;
  readonly senderCount: number;
  readonly activeSenderCount: number;
  readonly lastSyncedAt: string;
  readonly warning: string;
  readonly emptyState: boolean;
}

export type HXCSenderResult =
  | { readonly status: "loaded"; readonly model: HXCSenderReadModel }
  | {
      readonly status:
        "unauthenticated" | "forbidden" | "unavailable" | "invalid";
    };

async function generatedRead(options?: RequestInit) {
  return getLegacyHXCSendConfig({ credentials: "same-origin", ...options });
}

export interface HXCSenderTransport {
  readonly read: typeof generatedRead;
}

export const generatedHXCSenderTransport: HXCSenderTransport = {
  read: generatedRead,
};

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function text(value: unknown, empty = false): value is string {
  return (
    typeof value === "string" &&
    (empty || value.length > 0) &&
    value.length <= 200 &&
    value.trim() === value
  );
}

function integer(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function exact(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  const actual = Object.keys(value);
  return (
    actual.length === keys.length && actual.every((key) => keys.includes(key))
  );
}

function config(value: unknown): HXCSenderConfig | undefined {
  if (
    !object(value) ||
    !exact(value, [
      "id",
      "sender_userid",
      "display_name",
      "priority",
      "is_active",
      "created_at",
      "updated_at",
    ]) ||
    !text(value.id) ||
    !text(value.sender_userid) ||
    !text(value.display_name, true) ||
    !integer(value.priority) ||
    value.priority > 100000 ||
    typeof value.is_active !== "boolean" ||
    !text(value.created_at) ||
    !text(value.updated_at)
  ) {
    return undefined;
  }
  return {
    id: value.id,
    senderUserID: value.sender_userid,
    displayName: value.display_name,
    priority: value.priority,
    isActive: value.is_active,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

function candidate(value: unknown): HXCDirectoryCandidate | undefined {
  if (
    !object(value) ||
    !exact(value, [
      "wecom_userid",
      "display_name",
      "position",
      "wecom_status",
      "is_sender",
      "priority",
      "is_active",
    ]) ||
    !text(value.wecom_userid) ||
    !text(value.display_name) ||
    value.position !== "" ||
    value.wecom_status !== 0 ||
    typeof value.is_sender !== "boolean" ||
    !integer(value.priority) ||
    value.priority > 100000 ||
    typeof value.is_active !== "boolean"
  ) {
    return undefined;
  }
  return {
    wecomUserID: value.wecom_userid,
    displayName: value.display_name,
    position: "",
    wecomStatus: 0,
    isSender: value.is_sender,
    priority: value.priority,
    isActive: value.is_active,
  };
}

export async function loadHXCSenders(
  transport: HXCSenderTransport = generatedHXCSenderTransport,
): Promise<HXCSenderResult> {
  let response: Awaited<ReturnType<HXCSenderTransport["read"]>>;
  try {
    response = await transport.read();
  } catch {
    return { status: "unavailable" };
  }

  if (response.status === 401) return { status: "unauthenticated" };
  if (response.status === 403) return { status: "forbidden" };
  if (response.status !== 200) return { status: "unavailable" };

  const body: unknown = response.data;
  if (
    !object(body) ||
    !exact(body, [
      "ok",
      "source_status",
      "route_owner",
      "fallback_used",
      "real_external_call_executed",
      "send_configs",
      "directory_candidates",
      "members",
      "directory_count",
      "sender_count",
      "active_sender_count",
      "last_synced_at",
      "warnings",
      "degraded",
      "empty_state",
    ]) ||
    body.ok !== true ||
    body.source_status !== "v2_local_staff" ||
    body.route_owner !== "aicrm_v2" ||
    body.fallback_used !== false ||
    body.real_external_call_executed !== false ||
    body.degraded !== false ||
    !Array.isArray(body.send_configs) ||
    !Array.isArray(body.directory_candidates) ||
    !Array.isArray(body.members) ||
    !integer(body.directory_count) ||
    !integer(body.sender_count) ||
    !integer(body.active_sender_count) ||
    !text(body.last_synced_at, true) ||
    !Array.isArray(body.warnings) ||
    body.warnings.length !== 1 ||
    body.warnings[0] !==
      "HXC senders use the local staff projection; no WeCom directory call was executed." ||
    typeof body.empty_state !== "boolean"
  ) {
    return { status: "invalid" };
  }

  const configs = body.send_configs.map(config);
  const directory = body.directory_candidates.map(candidate);
  const members = body.members.map(candidate);
  if (
    configs.includes(undefined) ||
    directory.includes(undefined) ||
    members.includes(undefined) ||
    body.directory_count !== directory.length ||
    body.sender_count !== configs.length ||
    body.empty_state !== (members.length === 0) ||
    JSON.stringify(directory) !== JSON.stringify(members)
  ) {
    return { status: "invalid" };
  }

  return {
    status: "loaded",
    model: {
      sendConfigs: configs as HXCSenderConfig[],
      members: members as HXCDirectoryCandidate[],
      directoryCount: body.directory_count,
      senderCount: body.sender_count,
      activeSenderCount: body.active_sender_count,
      lastSyncedAt: body.last_synced_at,
      warning: body.warnings[0],
      emptyState: body.empty_state,
    },
  };
}
