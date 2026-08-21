import {
  createLegacyChannel,
  getLegacyChannel,
  listLegacyChannels,
  updateLegacyChannel,
} from "./api/generated/health";

export type ChannelsRole = "admin" | "ops" | "sales";
export type ChannelStatus = "active" | "inactive" | "archived";
export type ChannelStatusFilter = "all" | ChannelStatus;

export interface ChannelListItem {
  readonly id: number;
  readonly name: string;
  readonly code: string;
  readonly status: ChannelStatus;
  readonly assigneeCount: 0;
  readonly contactCount: 0;
  readonly createdAt: string;
  readonly updatedAt: string;
}
export interface ChannelDetail {
  readonly item: ChannelListItem;
  readonly channelType: "qrcode" | "wecom_customer_acquisition";
  readonly carrierType: "qrcode" | "link";
  readonly sceneValue: string;
  readonly qrURL: string;
  readonly ownerStaffID: string;
  readonly customerChannel: string;
  readonly linkURL: string;
  readonly finalURL: string;
  readonly welcomeMessage: string;
  readonly imageMaterialIDs: readonly number[];
  readonly miniProgramMaterialIDs: readonly number[];
  readonly attachmentMaterialIDs: readonly number[];
  readonly groupInviteMaterialIDs: readonly number[];
  readonly autoAcceptFriend: boolean;
  readonly entryTagID: string;
  readonly entryTagName: string;
  readonly entryTagGroupName: string;
  readonly assignmentMode: "single_owner" | "multi_staff";
  readonly assignmentStrategy: "ratio" | "cap_switch";
  readonly overflowPolicy: string;
  readonly hasAssignmentConfig: boolean;
  readonly imageMaterialCount: number;
  readonly miniProgramMaterialCount: number;
  readonly attachmentMaterialCount: number;
  readonly groupInviteMaterialCount: number;
}

// ChannelConfigurationInput is deliberately limited to the persisted Contact
// projection. It describes a local draft only; it neither creates a WeCom QR
// code nor publishes or opens a customer-acquisition link.
export interface ChannelConfigurationInput {
  readonly channelType: "qrcode" | "wecom_customer_acquisition";
  readonly carrierType: "qrcode" | "link";
  readonly channelName: string;
  readonly channelCode: string;
  readonly status: ChannelStatus;
  readonly sceneValue: string;
  readonly qrURL: string;
  readonly ownerStaffID: string;
  readonly customerChannel: string;
  readonly linkURL: string;
  readonly finalURL: string;
  readonly welcomeMessage: string;
  readonly imageMaterialIDs: readonly number[];
  readonly miniProgramMaterialIDs: readonly number[];
  readonly attachmentMaterialIDs: readonly number[];
  readonly groupInviteMaterialIDs: readonly number[];
  readonly autoAcceptFriend: boolean;
  readonly entryTagID: string;
  readonly entryTagName: string;
  readonly entryTagGroupName: string;
  readonly assignmentMode: "single_owner" | "multi_staff";
  readonly assignmentStrategy: "ratio" | "cap_switch";
  readonly overflowPolicy: string;
}

export type ChannelsFailure =
  "unauthenticated" | "forbidden" | "unavailable" | "invalid";

export type ChannelListResult =
  | { readonly status: "loaded"; readonly items: readonly ChannelListItem[] }
  | { readonly status: ChannelsFailure };

export type ChannelStatusUpdateResult =
  | {
      readonly status: "confirmed";
      readonly items: readonly ChannelListItem[];
    }
  | { readonly status: "unknown" }
  | { readonly status: ChannelsFailure };
export type ChannelDetailResult =
  | { readonly status: "loaded"; readonly detail: ChannelDetail }
  | { readonly status: ChannelsFailure };
export type ChannelConfigurationResult =
  | {
      readonly status: "confirmed";
      readonly items: readonly ChannelListItem[];
      readonly detail: ChannelDetail;
    }
  | { readonly status: "unknown" }
  | { readonly status: ChannelsFailure };

const channelListParams = { limit: 300, include_archived: true } as const;

async function generatedRead(options?: RequestInit) {
  return listLegacyChannels(channelListParams, {
    credentials: "same-origin",
    ...options,
  });
}

async function generatedWrite(
  channelID: number,
  status: ChannelStatus,
  options?: RequestInit,
) {
  return updateLegacyChannel(
    channelID,
    { status },
    { ...options, credentials: "same-origin" },
  );
}
async function generatedCreate(
  input: ChannelConfigurationInput,
  options?: RequestInit,
) {
  return createLegacyChannel(channelConfigurationWire(input), {
    ...options,
    credentials: "same-origin",
  });
}
async function generatedConfigurationWrite(
  channelID: number,
  input: ChannelConfigurationInput,
  options?: RequestInit,
) {
  return updateLegacyChannel(channelID, channelConfigurationWire(input), {
    ...options,
    credentials: "same-origin",
  });
}
async function generatedDetail(channelID: number, options?: RequestInit) {
  return getLegacyChannel(channelID, { credentials: "same-origin", ...options });
}

export interface ChannelsTransport {
  readonly read: typeof generatedRead;
  readonly detail: typeof generatedDetail;
  readonly write: typeof generatedWrite;
  readonly create: typeof generatedCreate;
  readonly configure: typeof generatedConfigurationWrite;
}

export const generatedChannelsTransport: ChannelsTransport = {
  read: generatedRead,
  detail: generatedDetail,
  write: generatedWrite,
  create: generatedCreate,
  configure: generatedConfigurationWrite,
};

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
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

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function frozenText(value: unknown, allowEmpty = false): value is string {
  return (
    typeof value === "string" &&
    (allowEmpty || value.length > 0) &&
    [...value].length <= 200 &&
    value.trim() === value &&
    !value.includes("\x00")
  );
}
function longText(value: unknown): value is string {
  return typeof value === "string" && [...value].length <= 10_000 && !value.includes("\x00");
}
function optionalEnum<T extends string>(value: unknown, values: readonly T[]): value is T | undefined {
  return value === undefined || (typeof value === "string" && values.includes(value as T));
}
function localMaterialIDs(value: unknown): value is readonly number[] {
  return Array.isArray(value) && value.length <= 12 && value.every(positiveInteger) && new Set(value).size === value.length;
}

function frozenTimestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const parts = value.match(
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$/,
  );
  if (parts === null) return false;
  const [year, month, day, hour, minute, second] = parts
    .slice(1, 7)
    .map(Number);
  const offset = parts[7];
  if (
    hour > 23 ||
    minute > 59 ||
    second > 59 ||
    (offset !== "Z" &&
      (Number(offset.slice(1, 3)) > 23 || Number(offset.slice(4, 6)) > 59))
  ) {
    return false;
  }
  const calendar = new Date(0);
  calendar.setUTCFullYear(year, month - 1, day);
  calendar.setUTCHours(hour, minute, second, 0);
  return (
    calendar.getUTCFullYear() === year &&
    calendar.getUTCMonth() === month - 1 &&
    calendar.getUTCDate() === day &&
    calendar.getUTCHours() === hour &&
    calendar.getUTCMinutes() === minute &&
    calendar.getUTCSeconds() === second &&
    Number.isFinite(Date.parse(value))
  );
}

function parseChannel(value: unknown): ChannelListItem | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "id",
      "channel_name",
      "channel_code",
      "status",
      "assignee_count",
      "channel_contact_count",
      "created_at",
      "updated_at",
    ]) ||
    !positiveInteger(value.id) ||
    !frozenText(value.channel_name, true) ||
    !frozenText(value.channel_code, true) ||
    (value.status !== "active" &&
      value.status !== "inactive" &&
      value.status !== "archived") ||
    value.assignee_count !== 0 ||
    value.channel_contact_count !== 0 ||
    !frozenTimestamp(value.created_at) ||
    !frozenTimestamp(value.updated_at)
  ) {
    return undefined;
  }
  return {
    id: value.id,
    name: value.channel_name,
    code: value.channel_code,
    status: value.status,
    assigneeCount: 0,
    contactCount: 0,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

export function parseChannelList(
  value: unknown,
): readonly ChannelListItem[] | undefined {
  if (
    !record(value) ||
    !exact(value, ["ok", "channels", "reason", "source"]) ||
    value.ok !== true ||
    !Array.isArray(value.channels) ||
    value.reason !== "channels_listed" ||
    value.source !== "ai_crm_next"
  ) {
    return undefined;
  }
  const items = value.channels.map(parseChannel);
  if (
    items.includes(undefined) ||
    new Set((items as readonly ChannelListItem[]).map((item) => item.id))
      .size !== items.length
  ) {
    return undefined;
  }
  return items as readonly ChannelListItem[];
}

function platformError(value: unknown, code: string): boolean {
  if (!record(value)) return false;
  const expected = ["code", "message", "request_id"];
  const withDetails = [...expected, "details"];
  return (
    (exact(value, expected) || exact(value, withDetails)) &&
    value.code === code &&
    frozenText(value.message) &&
    frozenText(value.request_id) &&
    (value.details === undefined || Array.isArray(value.details))
  );
}

function compatibilityError(value: unknown): boolean {
  return (
    record(value) &&
    exact(value, ["ok", "detail"]) &&
    value.ok === false &&
    frozenText(value.detail)
  );
}

function failure(status: number, body: unknown): ChannelsFailure {
  if (status === 401 && platformError(body, "UNAUTHENTICATED")) {
    return "unauthenticated";
  }
  if (status === 403 && platformError(body, "UNAUTHORIZED")) {
    return "forbidden";
  }
  if (status === 400 && compatibilityError(body)) return "invalid";
  if (
    status === 503 &&
    (compatibilityError(body) || platformError(body, "DEPENDENCY_UNAVAILABLE"))
  ) {
    return "unavailable";
  }
  return "invalid";
}

export async function loadChannels(
  transport: ChannelsTransport = generatedChannelsTransport,
): Promise<ChannelListResult> {
  let response: Awaited<ReturnType<ChannelsTransport["read"]>>;
  try {
    response = await transport.read();
  } catch {
    return { status: "unavailable" };
  }
  if (response.status !== 200) {
    return { status: failure(response.status, response.data) };
  }
  const items = parseChannelList(response.data);
  return items ? { status: "loaded", items } : { status: "invalid" };
}

const DETAIL_REQUIRED = [
  "schema_version", "id", "channel_name", "channel_code", "status", "created_at", "updated_at",
  "assignees", "assignment_stats_24h", "assignee_count", "channel_contact_count",
  "latest_channel_entered_at", "qrcode_asset_id", "qrcode_status", "qr_download_url",
  "share_url", "copy_text",
] as const;
const DETAIL_OPTIONAL = [
  "channel_type", "carrier_type", "scene_value", "qr_url", "owner_staff_id",
  "customer_channel", "link_url", "final_url", "welcome_message",
  "welcome_image_library_ids", "welcome_miniprogram_library_ids",
  "welcome_attachment_library_ids", "welcome_group_invite_library_ids",
  "auto_accept_friend", "entry_tag_id", "entry_tag_name", "entry_tag_group_name",
  "assignment_mode", "assignment_strategy", "overflow_policy", "assignment_config_json",
] as const;
const DETAIL_KEYS: readonly string[] = [...DETAIL_REQUIRED, ...DETAIL_OPTIONAL];

function parseChannelDetail(value: unknown, item: ChannelListItem): ChannelDetail | undefined {
  if (!record(value)) return undefined;
  if (!exact(value, DETAIL_KEYS)) return undefined;
  if (
    value.schema_version !== 1 || !positiveInteger(value.id) || value.id !== item.id ||
    !frozenText(value.channel_name, true) || value.channel_name !== item.name ||
    !frozenText(value.channel_code, true) || value.channel_code !== item.code ||
    value.status !== item.status || !frozenTimestamp(value.created_at) || value.created_at !== item.createdAt ||
    !frozenTimestamp(value.updated_at) || value.updated_at !== item.updatedAt ||
    !Array.isArray(value.assignees) || value.assignees.length !== 0 ||
    !Array.isArray(value.assignment_stats_24h) || value.assignment_stats_24h.length !== 0 ||
    value.assignee_count !== 0 || value.channel_contact_count !== 0 ||
    value.latest_channel_entered_at !== "" || value.qrcode_asset_id !== 0 ||
    value.qrcode_status !== "not_generated" || value.qr_download_url !== "" ||
    value.share_url !== "" || value.copy_text !== "" ||
    !optionalEnum(value.channel_type, ["qrcode", "wecom_customer_acquisition"]) || value.channel_type === undefined ||
    !optionalEnum(value.carrier_type, ["qrcode", "link"]) || value.carrier_type === undefined ||
    !longText(value.scene_value) || !longText(value.qr_url) ||
    !longText(value.owner_staff_id) || !longText(value.customer_channel) ||
    !longText(value.link_url) || !longText(value.final_url) ||
    !longText(value.welcome_message) || !longText(value.entry_tag_id) ||
    !longText(value.entry_tag_name) || !longText(value.entry_tag_group_name) ||
    !longText(value.overflow_policy) ||
    typeof value.auto_accept_friend !== "boolean" ||
    !optionalEnum(value.assignment_mode, ["single_owner", "multi_staff"]) || value.assignment_mode === undefined ||
    !optionalEnum(value.assignment_strategy, ["ratio", "cap_switch"]) || value.assignment_strategy === undefined ||
    !localMaterialIDs(value.welcome_image_library_ids) || !localMaterialIDs(value.welcome_miniprogram_library_ids) ||
    !localMaterialIDs(value.welcome_attachment_library_ids) || !localMaterialIDs(value.welcome_group_invite_library_ids) ||
    !record(value.assignment_config_json)
  ) return undefined;
  return {
    item,
    channelType: value.channel_type,
    carrierType: value.carrier_type,
    sceneValue: value.scene_value,
    qrURL: value.qr_url,
    ownerStaffID: value.owner_staff_id,
    customerChannel: value.customer_channel,
    linkURL: value.link_url,
    finalURL: value.final_url,
    welcomeMessage: value.welcome_message,
    imageMaterialIDs: value.welcome_image_library_ids,
    miniProgramMaterialIDs: value.welcome_miniprogram_library_ids,
    attachmentMaterialIDs: value.welcome_attachment_library_ids,
    groupInviteMaterialIDs: value.welcome_group_invite_library_ids,
    autoAcceptFriend: value.auto_accept_friend,
    entryTagID: value.entry_tag_id,
    entryTagName: value.entry_tag_name,
    entryTagGroupName: value.entry_tag_group_name,
    assignmentMode: value.assignment_mode,
    assignmentStrategy: value.assignment_strategy,
    overflowPolicy: value.overflow_policy,
    hasAssignmentConfig: true,
    imageMaterialCount: value.welcome_image_library_ids.length,
    miniProgramMaterialCount: value.welcome_miniprogram_library_ids.length,
    attachmentMaterialCount: value.welcome_attachment_library_ids.length,
    groupInviteMaterialCount: value.welcome_group_invite_library_ids.length,
  };
}

function listItemFromDetail(value: Record<string, unknown>): ChannelListItem | undefined {
  return parseChannel({
    id: value.id,
    channel_name: value.channel_name,
    channel_code: value.channel_code,
    status: value.status,
    assignee_count: value.assignee_count,
    channel_contact_count: value.channel_contact_count,
    created_at: value.created_at,
    updated_at: value.updated_at,
  });
}

export async function loadChannelDetail(transport: ChannelsTransport, item: ChannelListItem): Promise<ChannelDetailResult> {
  try {
    const response = await transport.detail(item.id);
    if (response.status !== 200) return { status: failure(response.status, response.data) };
    const body: unknown = response.data;
    if (!record(body) || !exact(body, ["ok", "channel", "reason", "source"]) || body.ok !== true || body.reason !== "channel_loaded" || body.source !== "ai_crm_next") return { status: "invalid" };
    const detail = parseChannelDetail(body.channel, item);
    return detail ? { status: "loaded", detail } : { status: "invalid" };
  } catch { return { status: "unavailable" }; }
}

function validConfigurationInput(input: ChannelConfigurationInput): boolean {
  return (
    (input.channelType === "qrcode" || input.channelType === "wecom_customer_acquisition") &&
    (input.carrierType === "qrcode" || input.carrierType === "link") &&
    frozenText(input.channelName) && frozenText(input.channelCode) &&
    (input.status === "active" || input.status === "inactive" || input.status === "archived") &&
    [input.sceneValue, input.qrURL, input.ownerStaffID, input.customerChannel,
      input.linkURL, input.finalURL, input.welcomeMessage, input.entryTagID,
      input.entryTagName, input.entryTagGroupName, input.overflowPolicy].every(longText) &&
    localMaterialIDs(input.imageMaterialIDs) &&
    localMaterialIDs(input.miniProgramMaterialIDs) &&
    localMaterialIDs(input.attachmentMaterialIDs) &&
    localMaterialIDs(input.groupInviteMaterialIDs) &&
    typeof input.autoAcceptFriend === "boolean" &&
    (input.assignmentMode === "single_owner" || input.assignmentMode === "multi_staff") &&
    (input.assignmentStrategy === "ratio" || input.assignmentStrategy === "cap_switch")
  );
}

function channelConfigurationWire(input: ChannelConfigurationInput) {
  return {
    channel_type: input.channelType,
    carrier_type: input.carrierType,
    channel_name: input.channelName,
    channel_code: input.channelCode,
    status: input.status,
    scene_value: input.sceneValue,
    qr_url: input.qrURL,
    owner_staff_id: input.ownerStaffID,
    customer_channel: input.customerChannel,
    link_url: input.linkURL,
    final_url: input.finalURL,
    welcome_message: input.welcomeMessage,
    welcome_image_library_ids: [...input.imageMaterialIDs],
    welcome_miniprogram_library_ids: [...input.miniProgramMaterialIDs],
    welcome_attachment_library_ids: [...input.attachmentMaterialIDs],
    welcome_group_invite_library_ids: [...input.groupInviteMaterialIDs],
    auto_accept_friend: input.autoAcceptFriend,
    entry_tag_id: input.entryTagID,
    entry_tag_name: input.entryTagName,
    entry_tag_group_name: input.entryTagGroupName,
    assignment_mode: input.assignmentMode,
    assignment_strategy: input.assignmentStrategy,
    overflow_policy: input.overflowPolicy,
  };
}

function sameConfiguration(detail: ChannelDetail, input: ChannelConfigurationInput): boolean {
  return (
    detail.item.name === input.channelName && detail.item.code === input.channelCode &&
    detail.item.status === input.status && detail.channelType === input.channelType &&
    detail.carrierType === input.carrierType && detail.sceneValue === input.sceneValue &&
    detail.qrURL === input.qrURL && detail.ownerStaffID === input.ownerStaffID &&
    detail.customerChannel === input.customerChannel && detail.linkURL === input.linkURL &&
    detail.finalURL === input.finalURL && detail.welcomeMessage === input.welcomeMessage &&
    detail.autoAcceptFriend === input.autoAcceptFriend && detail.entryTagID === input.entryTagID &&
    detail.entryTagName === input.entryTagName && detail.entryTagGroupName === input.entryTagGroupName &&
    detail.assignmentMode === input.assignmentMode && detail.assignmentStrategy === input.assignmentStrategy &&
    detail.overflowPolicy === input.overflowPolicy &&
    sameIDs(detail.imageMaterialIDs, input.imageMaterialIDs) &&
    sameIDs(detail.miniProgramMaterialIDs, input.miniProgramMaterialIDs) &&
    sameIDs(detail.attachmentMaterialIDs, input.attachmentMaterialIDs) &&
    sameIDs(detail.groupInviteMaterialIDs, input.groupInviteMaterialIDs)
  );
}

function sameIDs(left: readonly number[], right: readonly number[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function sameChannelDetail(left: ChannelDetail, right: ChannelDetail): boolean {
  return left.item.id === right.item.id && left.item.name === right.item.name &&
    left.item.code === right.item.code && left.item.status === right.item.status &&
    left.channelType === right.channelType && left.carrierType === right.carrierType &&
    left.sceneValue === right.sceneValue && left.qrURL === right.qrURL &&
    left.ownerStaffID === right.ownerStaffID && left.customerChannel === right.customerChannel &&
    left.linkURL === right.linkURL && left.finalURL === right.finalURL &&
    left.welcomeMessage === right.welcomeMessage && left.autoAcceptFriend === right.autoAcceptFriend &&
    left.entryTagID === right.entryTagID && left.entryTagName === right.entryTagName &&
    left.entryTagGroupName === right.entryTagGroupName && left.assignmentMode === right.assignmentMode &&
    left.assignmentStrategy === right.assignmentStrategy && left.overflowPolicy === right.overflowPolicy &&
    sameIDs(left.imageMaterialIDs, right.imageMaterialIDs) &&
    sameIDs(left.miniProgramMaterialIDs, right.miniProgramMaterialIDs) &&
    sameIDs(left.attachmentMaterialIDs, right.attachmentMaterialIDs) &&
    sameIDs(left.groupInviteMaterialIDs, right.groupInviteMaterialIDs);
}

function parseChannelMutation(
  value: unknown,
  reason: "channel_created" | "channel_updated",
): ChannelDetail | undefined {
  if (
    !record(value) ||
    !exact(value, ["ok", "channel", "reason", "source", "fallback_used", "real_external_call_executed"]) ||
    value.ok !== true || value.reason !== reason || value.source !== "ai_crm_next" ||
    value.fallback_used !== false || value.real_external_call_executed !== false || !record(value.channel)
  ) return undefined;
  const item = listItemFromDetail(value.channel);
  return item ? parseChannelDetail(value.channel, item) : undefined;
}

function validMutationBoundary(csrfToken: string, idempotencyKey: string): boolean {
  return /^[A-Za-z0-9_-]{43}$/.test(csrfToken) &&
    idempotencyKey.length >= 16 && idempotencyKey.length <= 128 && idempotencyKey.trim() === idempotencyKey;
}

export async function saveChannelConfiguration(
  transport: ChannelsTransport,
  operation: "create" | "update",
  input: ChannelConfigurationInput,
  channelID: number | undefined,
  csrfToken: string,
  idempotencyKey: string,
): Promise<ChannelConfigurationResult> {
  if (!validConfigurationInput(input) || !validMutationBoundary(csrfToken, idempotencyKey) ||
    (operation === "update" && !positiveInteger(channelID))) return { status: "invalid" };

  let response: Awaited<ReturnType<ChannelsTransport["create"]>> | Awaited<ReturnType<ChannelsTransport["configure"]>>;
  try {
    const options = {
      credentials: "same-origin" as const,
      headers: { "X-CSRF-Token": csrfToken, "Idempotency-Key": idempotencyKey },
    };
    response = operation === "create"
      ? await transport.create(input, options)
      : await transport.configure(channelID as number, input, options);
  } catch {
    return { status: "unknown" };
  }
  const expectedStatus = operation === "create" ? 201 : 200;
  if (response.status !== expectedStatus) {
    const result = failure(response.status, response.data);
    return result === "unavailable" ? { status: "unknown" } : { status: result };
  }
  const mutation = parseChannelMutation(response.data, operation === "create" ? "channel_created" : "channel_updated");
  if (!mutation || !sameConfiguration(mutation, input) ||
    (operation === "update" && mutation.item.id !== channelID)) return { status: "unknown" };

  const reloaded = await loadChannels(transport);
  if (reloaded.status !== "loaded") return reloaded.status === "unauthenticated" ? reloaded : { status: "unknown" };
  const item = reloaded.items.find((candidate) => candidate.id === mutation.item.id);
  if (!item || item.name !== input.channelName || item.code !== input.channelCode || item.status !== input.status) return { status: "unknown" };
  const reread = await loadChannelDetail(transport, item);
  if (reread.status !== "loaded") return reread.status === "unauthenticated" ? reread : { status: "unknown" };
  return sameConfiguration(reread.detail, input)
    ? { status: "confirmed", items: reloaded.items, detail: reread.detail }
    : { status: "unknown" };
}

export function newChannelStatusIdempotencyKey(
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  try {
    const uuid = source?.randomUUID();
    if (
      typeof uuid !== "string" ||
      !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
        uuid,
      )
    ) {
      return undefined;
    }
    return `channel-status:${uuid}`;
  } catch {
    return undefined;
  }
}

export function newChannelConfigurationIdempotencyKey(
  operation: "create" | "update",
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  const uuid = newChannelStatusIdempotencyKey(source)?.replace("channel-status:", "");
  return uuid ? `channel-${operation}:${uuid}` : undefined;
}

export async function updateChannelStatus(
  transport: ChannelsTransport,
  channelID: number,
  status: ChannelStatus,
  csrfToken: string,
  idempotencyKey: string,
): Promise<ChannelStatusUpdateResult> {
  if (
    !positiveInteger(channelID) ||
    !/^[A-Za-z0-9_-]{43}$/.test(csrfToken) ||
    (status !== "active" && status !== "inactive" && status !== "archived") ||
    idempotencyKey.length < 16 ||
    idempotencyKey.length > 128 ||
    idempotencyKey.trim() !== idempotencyKey
  ) {
    return { status: "invalid" };
  }

  let response: Awaited<ReturnType<ChannelsTransport["write"]>>;
  try {
    response = await transport.write(channelID, status, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": csrfToken,
        "Idempotency-Key": idempotencyKey,
      },
    });
  } catch {
    return { status: "unknown" };
  }

  if (response.status !== 200) {
    const result = failure(response.status, response.data);
    return result === "unavailable" ? { status: "unknown" } : { status: result };
  }
  const mutation = parseChannelMutation(response.data, "channel_updated");
  if (!mutation || mutation.item.id !== channelID || mutation.item.status !== status) return { status: "unknown" };
  const reloaded = await loadChannels(transport);
  if (reloaded.status !== "loaded") {
    return reloaded.status === "unauthenticated"
      ? reloaded
      : { status: "unknown" };
  }
  const item = reloaded.items.find((candidate) => candidate.id === channelID && candidate.status === status);
  if (!item) return { status: "unknown" };
  const reread = await loadChannelDetail(transport, item);
  return reread.status === "loaded" && sameChannelDetail(mutation, reread.detail)
    ? { status: "confirmed", items: reloaded.items }
    : reread.status === "unauthenticated" ? reread : { status: "unknown" };
}

export function filterChannels(
  items: readonly ChannelListItem[],
  keyword: string,
  status: ChannelStatusFilter,
): readonly ChannelListItem[] {
  const query = keyword.trim().toLocaleLowerCase();
  return items.filter(
    (item) =>
      (status === "all" || item.status === status) &&
      (query === "" ||
        item.name.toLocaleLowerCase().includes(query) ||
        item.code.toLocaleLowerCase().includes(query)),
  );
}

export const CHANNEL_PAGE_SIZE = 20;

export interface ChannelPage {
  readonly items: readonly ChannelListItem[];
  readonly page: number;
  readonly pageSize: number;
  readonly pageCount: number;
  readonly total: number;
}

export function paginateChannels(
  items: readonly ChannelListItem[],
  requestedPage: number,
  requestedPageSize = CHANNEL_PAGE_SIZE,
): ChannelPage {
  const pageSize = Number.isSafeInteger(requestedPageSize) &&
    requestedPageSize >= 1 && requestedPageSize <= 100
    ? requestedPageSize
    : CHANNEL_PAGE_SIZE;
  const pageCount = Math.max(1, Math.ceil(items.length / pageSize));
  const page = Number.isSafeInteger(requestedPage)
    ? Math.min(Math.max(requestedPage, 1), pageCount)
    : 1;
  const offset = (page - 1) * pageSize;
  return {
    items: items.slice(offset, offset + pageSize),
    page,
    pageSize,
    pageCount,
    total: items.length,
  };
}
