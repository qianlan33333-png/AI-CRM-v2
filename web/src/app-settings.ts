import {
  getLegacyAppSettingsResource,
  saveLegacyAppSettingsResource,
  type GetLegacyAppSettingsResourceParams,
  type LegacyAppSettingsResourceSaveRequest,
} from "./api/generated/health";

export type AppSettingsRole = "admin" | "ops" | "sales";
export type AppSettingsEditableKey =
  | "wecom.corp_id"
  | "wecom.agent_id"
  | "outbound.rate_per_second"
  | "outbound.max_attempts";
export type AppSettingsMaskedKey =
  | "database.url"
  | "wecom.secret"
  | "wecom.callback_token"
  | "wecom.callback_aes_key"
  | "ai.api_key"
  | "auth.jwt_secret"
  | "extension.api_key_pepper"
  | "gateway.webhook_master_key";

const EDITABLE_KEYS = [
  "wecom.corp_id",
  "wecom.agent_id",
  "outbound.rate_per_second",
  "outbound.max_attempts",
] as const satisfies readonly AppSettingsEditableKey[];
const MASKED_KEYS = [
  "database.url",
  "wecom.secret",
  "wecom.callback_token",
  "wecom.callback_aes_key",
  "ai.api_key",
  "auth.jwt_secret",
  "extension.api_key_pepper",
  "gateway.webhook_master_key",
] as const satisfies readonly AppSettingsMaskedKey[];
const ALL_KEYS = [...EDITABLE_KEYS, ...MASKED_KEYS] as const;
const SUMMARY = [
  ["可直接编辑", "可以直接修改的设置项"],
  ["敏感信息", "只显示掩码的设置项"],
  ["已配置", "当前已经配置完成的设置项"],
] as const;

export interface AppSettingsEditableRow {
  readonly key: AppSettingsEditableKey;
  readonly label: AppSettingsEditableKey;
  readonly inputType: "text" | "number";
  readonly value: string;
  readonly configured: boolean;
  readonly source: "app_settings" | "config";
  readonly updatedAt?: string;
  readonly lastModifiedAt?: string;
  readonly lastModifiedBy?: string;
  readonly lastActionType: "create" | "update" | "empty";
}

export interface AppSettingsMaskedRow {
  readonly key: AppSettingsMaskedKey;
  readonly configured: boolean;
}

export interface AppSettingsAuditEntry {
  readonly id: number;
  readonly operator: string;
  readonly actionType: "create" | "update";
  readonly targetID: AppSettingsEditableKey;
  readonly createdAt: string;
}

export interface AppSettingsSnapshot {
  readonly editable: readonly AppSettingsEditableRow[];
  readonly masked: readonly AppSettingsMaskedRow[];
  readonly summary: readonly { readonly label: string; readonly value: number; readonly description: string }[];
  readonly audits: readonly AppSettingsAuditEntry[];
  readonly actionToken?: string;
}

export type AppSettingsFailure = "unauthenticated" | "forbidden" | "invalid" | "unavailable";
export type AppSettingsLoadResult =
  | { readonly status: "loaded"; readonly snapshot: AppSettingsSnapshot }
  | { readonly status: AppSettingsFailure };
export type AppSettingsSaveResult =
  | { readonly status: "saved"; readonly snapshot: AppSettingsSnapshot }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" | "conflict" | "unknown" };

export interface AppSettingsTransportResponse { readonly status: number; readonly data: unknown }
export interface AppSettingsTransport {
  // eslint-disable-next-line no-unused-vars -- generated read arguments document the exact resource contract.
  readonly read: (params: GetLegacyAppSettingsResourceParams | undefined, options: RequestInit) => Promise<AppSettingsTransportResponse>;
  // eslint-disable-next-line no-unused-vars -- generated write arguments document the exact resource contract.
  readonly save: (request: LegacyAppSettingsResourceSaveRequest, options: RequestInit) => Promise<AppSettingsTransportResponse>;
}

export const generatedAppSettingsTransport: AppSettingsTransport = {
  read: getLegacyAppSettingsResource,
  save: saveLegacyAppSettingsResource,
};

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);
}
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}
function positive(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function nonnegative(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 0; }
function inSet<T extends string>(value: unknown, values: readonly T[]): value is T { return typeof value === "string" && values.includes(value as T); }
function text(value: unknown, maximum: number, nonempty = false): value is string {
  return typeof value === "string" && value === value.trim() && (!nonempty || value.length > 0) && new TextEncoder().encode(value).length <= maximum;
}
function timestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const matched = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-](\d{2}):(\d{2}))$/.exec(value);
  if (!matched) return false;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, , offsetHourText, offsetMinuteText] = matched;
  const year = Number(yearText); const month = Number(monthText); const day = Number(dayText); const hour = Number(hourText); const minute = Number(minuteText); const second = Number(secondText);
  const offsetHour = offsetHourText === undefined ? 0 : Number(offsetHourText); const offsetMinute = offsetMinuteText === undefined ? 0 : Number(offsetMinuteText);
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return month >= 1 && month <= 12 && day >= 1 && day <= days[month - 1] && hour <= 23 && minute <= 59 && second <= 59 && offsetHour <= 23 && offsetMinute <= 59;
}
function token(value: unknown): value is string { return typeof value === "string" && /^[A-Za-z0-9_-]{43}$/.test(value); }
function integerText(value: string, minimum: bigint, maximum: bigint): boolean {
  if (!/^(?:0|[1-9]\d*)$/.test(value)) return false;
  try { const parsed = BigInt(value); return parsed >= minimum && parsed <= maximum; } catch { return false; }
}
function validEditableValue(key: AppSettingsEditableKey, value: string): boolean {
  switch (key) {
    case "wecom.corp_id": return text(value, 256, true);
    case "wecom.agent_id": return integerText(value, 1n, 9_223_372_036_854_775_807n);
    case "outbound.rate_per_second": return integerText(value, 1n, 50n);
    case "outbound.max_attempts": return integerText(value, 1n, 10n);
  }
}

function parseEditable(value: unknown): AppSettingsEditableRow | undefined {
  const keys = ["key", "label", "mode", "input_type", "description", "value", "display_value", "configured", "source", "version", "updated_at", "last_modified_at", "last_modified_by", "last_action_type"];
  if (!record(value) || !exact(value, keys) || !inSet(value.key, EDITABLE_KEYS) || value.label !== value.key || value.mode !== "editable" ||
    (value.input_type !== "text" && value.input_type !== "number") || value.description !== "" || !text(value.value, 256) || value.display_value !== value.value ||
    typeof value.configured !== "boolean" || (value.source !== "app_settings" && value.source !== "config") || value.version !== "" ||
    !inSet(value.last_action_type, ["create", "update", "empty"] as const)) return undefined;
  if (value.key === "wecom.corp_id" ? value.input_type !== "text" : value.input_type !== "number") return undefined;
  if (value.configured !== (value.source === "app_settings")) return undefined;
  if (!value.configured) {
    if (value.value !== "" || value.updated_at !== "" || value.last_modified_at !== "" || value.last_modified_by !== "" || value.last_action_type !== "empty") return undefined;
    return { key: value.key, label: value.key, inputType: value.input_type, value: "", configured: false, source: "config", lastActionType: "empty" };
  }
  if (!validEditableValue(value.key, value.value) || !timestamp(value.updated_at) || !timestamp(value.last_modified_at) || !text(value.last_modified_by, 200, true) || (value.last_action_type !== "create" && value.last_action_type !== "update")) return undefined;
  return { key: value.key, label: value.key, inputType: value.input_type, value: value.value, configured: true, source: "app_settings", updatedAt: value.updated_at, lastModifiedAt: value.last_modified_at, lastModifiedBy: value.last_modified_by, lastActionType: value.last_action_type };
}

function parseMasked(value: unknown): AppSettingsMaskedRow | undefined {
  const keys = ["key", "label", "mode", "input_type", "description", "configured", "masked"];
  if (!record(value) || !exact(value, keys) || !inSet(value.key, MASKED_KEYS) || value.label !== value.key || value.mode !== "masked" || value.input_type !== "password" || value.description !== "" || typeof value.configured !== "boolean" || value.masked !== true) return undefined;
  return { key: value.key, configured: value.configured };
}

function parseAudit(value: unknown): AppSettingsAuditEntry | undefined {
  if (!record(value) || !exact(value, ["id", "operator", "action_type", "target_id", "created_at"]) || !positive(value.id) || !text(value.operator, 200, true) ||
    (value.action_type !== "create" && value.action_type !== "update") || !inSet(value.target_id, EDITABLE_KEYS) || !timestamp(value.created_at)) return undefined;
  return { id: value.id, operator: value.operator, actionType: value.action_type, targetID: value.target_id, createdAt: value.created_at };
}

function parseProjection(value: unknown): Omit<AppSettingsSnapshot, "actionToken"> | undefined {
  if (!record(value) || !exact(value, ["rows", "metadata_map", "summary_cards", "audit_entries"]) || !Array.isArray(value.rows) || !record(value.metadata_map) || !Array.isArray(value.summary_cards) || !Array.isArray(value.audit_entries) || value.rows.length !== 12 || value.summary_cards.length !== 3 || value.audit_entries.length > 10) return undefined;
  const metadataKeys = Object.keys(value.metadata_map);
  if (metadataKeys.length !== ALL_KEYS.length || !ALL_KEYS.every((key) => metadataKeys.includes(key))) return undefined;
  const editable: AppSettingsEditableRow[] = []; const masked: AppSettingsMaskedRow[] = [];
  for (let index = 0; index < value.rows.length; index += 1) {
    const raw = value.rows[index]; const wanted = ALL_KEYS[index];
    const parsed = index < EDITABLE_KEYS.length ? parseEditable(raw) : parseMasked(raw);
    if (!parsed || parsed.key !== wanted) return undefined;
    const metadata = value.metadata_map[wanted];
    const metaKeys = ["key", "label", "mode", "input_type", "description"];
    if (!record(metadata) || !exact(metadata, metaKeys) || metadata.key !== wanted || metadata.label !== wanted || metadata.description !== "" ||
      metadata.mode !== (index < EDITABLE_KEYS.length ? "editable" : "masked") || metadata.input_type !== (index < EDITABLE_KEYS.length ? (wanted === "wecom.corp_id" ? "text" : "number") : "password")) return undefined;
    if (index < EDITABLE_KEYS.length) editable.push(parsed as AppSettingsEditableRow); else masked.push(parsed as AppSettingsMaskedRow);
  }
  const summary = value.summary_cards.map((card, index) => {
    if (!record(card) || !exact(card, ["label", "value", "description"]) || card.label !== SUMMARY[index]?.[0] || card.description !== SUMMARY[index]?.[1] || !nonnegative(card.value) || card.value > 12) return undefined;
    return { label: card.label, value: card.value, description: card.description };
  });
  if (summary.some((item) => item === undefined) || summary[0]?.value !== editable.length || summary[1]?.value !== masked.length || summary[2]?.value !== editable.filter((item) => item.configured).length + masked.filter((item) => item.configured).length) return undefined;
  const audits = value.audit_entries.map(parseAudit);
  if (audits.some((item) => item === undefined)) return undefined;
  const parsedAudits = audits as AppSettingsAuditEntry[];
  for (let index = 1; index < parsedAudits.length; index += 1) {
    const prior = parsedAudits[index - 1]; const current = parsedAudits[index];
    if (prior.createdAt < current.createdAt || (prior.createdAt === current.createdAt && prior.id <= current.id)) return undefined;
  }
  return { editable, masked, summary: summary as { label: string; value: number; description: string }[], audits: parsedAudits };
}

export function parseAppSettingsSnapshot(value: unknown): AppSettingsSnapshot | undefined {
  const keys = record(value) ? Object.keys(value) : [];
  if (!record(value) || !keys.every((key) => ["ok", "config", "source_status", "fallback_used", "admin_action_token"].includes(key)) || !keys.includes("ok") || !keys.includes("config") || !keys.includes("source_status") || !keys.includes("fallback_used") || value.ok !== true || value.source_status !== "next_read_model" || value.fallback_used !== false) return undefined;
  if ("admin_action_token" in value && !token(value.admin_action_token)) return undefined;
  const projection = parseProjection(value.config);
  return projection ? { ...projection, ...(typeof value.admin_action_token === "string" ? { actionToken: value.admin_action_token } : {}) } : undefined;
}

function sameEditable(left: AppSettingsEditableRow, right: AppSettingsEditableRow): boolean {
  return left.key === right.key && left.value === right.value && left.configured === right.configured && left.source === right.source && left.updatedAt === right.updatedAt && left.lastModifiedAt === right.lastModifiedAt && left.lastModifiedBy === right.lastModifiedBy && left.lastActionType === right.lastActionType;
}

function parseSaveReceipt(value: unknown, expected: Record<AppSettingsEditableKey, string>, prior: AppSettingsSnapshot): AppSettingsSnapshot | undefined {
  if (!record(value) || !exact(value, ["ok", "changed", "changed_count", "config", "source_status", "fallback_used", "real_external_call_executed"]) || value.ok !== true || !Array.isArray(value.changed) || !nonnegative(value.changed_count) || value.changed_count !== value.changed.length || value.source_status !== "next_command" || value.fallback_used !== false || value.real_external_call_executed !== false) return undefined;
  const changedKeys = Object.keys(expected).sort();
  if (value.changed.length !== changedKeys.length) return undefined;
  for (let index = 0; index < value.changed.length; index += 1) {
    const changed = value.changed[index]; const key = changedKeys[index];
    if (!record(changed) || !exact(changed, ["key", "value"]) || changed.key !== key || changed.value !== expected[key as AppSettingsEditableKey]) return undefined;
  }
  const projection = parseProjection(value.config);
  if (!projection || projection.masked.some((row, index) => row.key !== prior.masked[index]?.key || row.configured !== prior.masked[index]?.configured)) return undefined;
  for (const item of projection.editable) {
    const old = prior.editable.find((candidate) => candidate.key === item.key);
    if (!old) return undefined;
    const wanted = expected[item.key];
    if (wanted === undefined ? !sameEditable(item, old) : item.value !== wanted || !item.configured) return undefined;
  }
  return { ...projection, actionToken: prior.actionToken };
}

export function canManageAppSettings(role: AppSettingsRole): boolean { return role === "admin"; }

export function appSettingsDraft(snapshot: AppSettingsSnapshot): Record<AppSettingsEditableKey, string> {
  return Object.fromEntries(snapshot.editable.map((row) => [row.key, row.value])) as Record<AppSettingsEditableKey, string>;
}

export function normalizeAppSettingsDraft(draft: Record<AppSettingsEditableKey, string>): Record<AppSettingsEditableKey, string> | undefined {
  const normalized = Object.fromEntries(EDITABLE_KEYS.map((key) => [key, draft[key]?.trim()])) as Record<AppSettingsEditableKey, string>;
  if (!EDITABLE_KEYS.every((key) => validEditableValue(key, normalized[key]))) return undefined;
  return normalized;
}

export function appSettingsChanged(snapshot: AppSettingsSnapshot, draft: Record<AppSettingsEditableKey, string>): Record<AppSettingsEditableKey, string> | undefined {
  const values = {} as Record<AppSettingsEditableKey, string>;
  for (const row of snapshot.editable) {
    const value = draft[row.key]?.trim();
    if (value === row.value) continue;
    if (!validEditableValue(row.key, value)) return undefined;
    values[row.key] = value;
  }
  return values;
}

export function newAppSettingsRequestID(source: { readonly randomUUID: () => string } | undefined = globalThis.crypto): string | undefined {
  try {
    const uuid = source?.randomUUID();
    return typeof uuid === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid) ? `app-settings:${uuid}` : undefined;
  } catch { return undefined; }
}

export async function loadAppSettings(transport: AppSettingsTransport): Promise<AppSettingsLoadResult> {
  try {
    const response = await transport.read(undefined, { credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status !== 200) return { status: "unavailable" };
    const snapshot = parseAppSettingsSnapshot(response.data);
    return snapshot ? { status: "loaded", snapshot } : { status: "invalid" };
  } catch { return { status: "unavailable" }; }
}

export async function saveAppSettings(
  transport: AppSettingsTransport,
  snapshot: AppSettingsSnapshot,
  draft: Record<AppSettingsEditableKey, string>,
  csrfToken: string,
  requestID: string,
): Promise<AppSettingsSaveResult> {
  const changed = appSettingsChanged(snapshot, draft);
  if (!changed || Object.keys(changed).length === 0 || !token(csrfToken) || !token(snapshot.actionToken) || !/^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$/.test(requestID)) return { status: "invalid" };
  try {
    const response = await transport.save({ settings: changed, confirm: true }, { credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken, "X-Admin-Action-Token": snapshot.actionToken, "X-Request-ID": requestID } });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 400) return { status: "invalid" };
    if (response.status === 409) return { status: "conflict" };
    if (response.status !== 200) return { status: "unknown" };
    const saved = parseSaveReceipt(response.data, changed, snapshot);
    return saved ? { status: "saved", snapshot: saved } : { status: "unknown" };
  } catch { return { status: "unknown" }; }
}
