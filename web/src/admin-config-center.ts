export type AdminConfigRole = "admin" | "ops" | "sales";
export type AdminCredentialState = "active" | "disabled" | "pending_activation";
export type AdminReleaseState = "draft" | "validated" | "published" | "rolled_back";

export const ADMIN_CONFIG_LOCAL_RELEASE_NOTICE = "本地配置状态，不等于部署";
export const ADMIN_CONFIG_UNKNOWN_NOTICE = "写入结果未知，已锁定后续写操作；请先重新读取本地状态，且不要自动重试。";

const MAX_RESPONSE_BYTES = 256 * 1024;
const MAX_SAFE_JSON_BYTES = 64 * 1024;
const MAX_JSON_DEPTH = 8;
const MAX_JSON_COLLECTION = 100;
const MAX_JSON_STRING_BYTES = 4096;
const MAX_CLIENT_ID_BYTES = 120;
const MAX_DISPLAY_NAME_BYTES = 200;
const MAX_CATEGORY_KEY_BYTES = 80;
const MAX_FILTER_BYTES = 200;
const TOKEN_PATTERN = /^[A-Za-z0-9_-]{43}$/;
const REQUEST_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$/;
const CLIENT_ID_PATTERN = /^[A-Za-z0-9._-]+$/;
const CATEGORY_KEY_PATTERN = /^[a-z][A-Za-z0-9._-]*$/;
const CHECKSUM_PATTERN = /^[a-f0-9]{64}$/;
const SECRET_REFERENCE_PATTERN = /^(?:secret:\/\/|secretref:)[^?&#\s]{1,238}$/;
const SECRET_MASK_PATTERN = /^masked(?::…[A-Za-z0-9_-]{6})?$/u;
const RFC3339_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;

export const ADMIN_CONFIG_ACTIONS = {
  createClient: { method: "POST", pattern: "/api/admin/config/api-clients" },
  updateClient: { method: "PUT", pattern: "/api/admin/config/api-clients/{client_id}" },
  activateClient: { method: "POST", pattern: "/api/admin/config/api-clients/{client_id}/activate" },
  rotateClient: { method: "POST", pattern: "/api/admin/config/api-clients/{client_id}/rotate-secret" },
  disableClient: { method: "PUT", pattern: "/api/admin/config/api-clients/{client_id}/enabled" },
  generateDirectKey: { method: "POST", pattern: "/api/admin/config/api-key/generate" },
  rotateDirectKey: { method: "POST", pattern: "/api/admin/config/api-key/rotate" },
  disableDirectKey: { method: "PUT", pattern: "/api/admin/config/api-key/enabled" },
  setCategoryEnabled: { method: "PUT", pattern: "/api/admin/config/categories/{category_key}/enabled" },
  saveCategorySettings: { method: "PUT", pattern: "/api/admin/config/categories/{category_key}/settings" },
  checkCategory: { method: "POST", pattern: "/api/admin/config/categories/{category_key}/check" },
  createRelease: { method: "POST", pattern: "/api/admin/config/releases" },
  validateRelease: { method: "POST", pattern: "/api/admin/config/releases/{release_id}/validate" },
  publishRelease: { method: "POST", pattern: "/api/admin/config/releases/{release_id}/publish" },
  rollbackRelease: { method: "POST", pattern: "/api/admin/config/releases/{release_id}/rollback" },
} as const;

export type AdminConfigAction = (typeof ADMIN_CONFIG_ACTIONS)[keyof typeof ADMIN_CONFIG_ACTIONS];

export const DIRECT_KEY_CONFIRMATION_PHRASES = {
  generate: "确认生成本地 API Key",
  rotate: "确认轮换本地 API Key",
  disable: "确认停用本地 API Key",
} as const;

export interface AdminCredential {
  readonly id: number;
  readonly kind: "api_client";
  readonly clientID: string;
  readonly displayName: string;
  readonly state: AdminCredentialState;
  readonly secretRef: string;
  readonly secretMask: string;
  readonly version: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface DirectAPIKeySnapshot {
  readonly configured: boolean;
  readonly state?: AdminCredentialState;
  readonly secretMask?: string;
  readonly version?: number;
  readonly createdAt?: string;
  readonly updatedAt?: string;
}

export type SafeJSONPrimitive = string | number | boolean | null;
export type SafeJSONValue = SafeJSONPrimitive | readonly SafeJSONValue[] | { readonly [key: string]: SafeJSONValue };
export type SafeJSONObject = { readonly [key: string]: SafeJSONValue };

export interface AdminCategory {
  readonly key: string;
  readonly enabled: boolean;
  readonly settings: SafeJSONObject;
  readonly version: number;
  readonly updatedBy?: string;
  readonly updatedAt?: string;
  readonly persisted: boolean;
}

export interface AdminRelease {
  readonly id: number;
  readonly state: AdminReleaseState;
  readonly changes: SafeJSONObject;
  readonly checksum: string;
  readonly basedOnReleaseID?: number;
  readonly rollbackOfReleaseID?: number;
  readonly createdBy: string;
  readonly publishedBy?: string;
  readonly createdAt: string;
  readonly validatedAt?: string;
  readonly publishedAt?: string;
}

export interface ShadowComparison {
  readonly releaseID: number;
  readonly available: boolean;
  readonly externalCalls: false;
}

export interface CategoryCheckResult {
  readonly categoryKey: string;
  readonly failed: number;
  readonly externalCalls: false;
  readonly category: AdminCategory;
}

export type AdminConfigReadFailure = "unauthenticated" | "forbidden" | "not_found" | "invalid" | "unavailable";
export type AdminConfigReadResult<T> =
  | { readonly status: "loaded"; readonly value: T }
  | { readonly status: AdminConfigReadFailure };

export type AdminConfigWriteFailure = "unauthenticated" | "forbidden" | "invalid" | "conflict" | "unavailable" | "unknown";
export type AdminConfigWriteResult<T> =
  | { readonly status: "applied"; readonly value: T }
  | { readonly status: AdminConfigWriteFailure };

export interface AdminConfigOverview {
  readonly clients: AdminConfigReadResult<readonly AdminCredential[]>;
  readonly directKey: AdminConfigReadResult<DirectAPIKeySnapshot>;
  readonly categories: AdminConfigReadResult<readonly AdminCategory[]>;
  readonly releases: AdminConfigReadResult<readonly AdminRelease[]>;
}

export interface AdminConfigTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

export interface AdminConfigTransport {
  // eslint-disable-next-line no-unused-vars -- named parameters document the transport contract.
  readonly request: (path: string, init: RequestInit) => Promise<AdminConfigTransportResponse>;
}

export interface AdminConfigSecurity {
  readonly csrfToken: () => string | undefined;
  // eslint-disable-next-line no-unused-vars -- named parameters document the action-token resolver contract.
  readonly actionTokenFor: (method: string, pattern: string) => string | undefined;
  readonly requestID: () => string | undefined;
}

/**
 * Monotonic gate used by the UI to discard responses from an older read.
 * A newer begin() or an explicit invalidate() makes every prior epoch stale.
 */
export class AdminConfigResponseEpoch {
  private current = 0;

  begin(): number {
    this.current += 1;
    return this.current;
  }

  accepts(epoch: number): boolean {
    return Number.isSafeInteger(epoch) && epoch > 0 && epoch === this.current;
  }

  invalidate(): void {
    this.current += 1;
  }
}

interface PublicCredentialRecord {
  readonly id: number;
  readonly kind: "api_client" | "direct_api_key";
  readonly clientID: string;
  readonly displayName: string;
  readonly state: AdminCredentialState;
  readonly secretRef: string;
  readonly secretMask: string;
  readonly version: number;
  readonly createdAt: string;
  readonly updatedAt: string;
}

interface MutationRequest {
  readonly path: string;
  readonly action: AdminConfigAction;
  readonly body: Readonly<Record<string, unknown>>;
  readonly successStatuses: readonly number[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function hasExactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function isSafePositiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function isSafeNonnegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length;
}

function validTrimmedText(value: unknown, maximumBytes: number, nonempty = true): value is string {
  return typeof value === "string" && value === value.trim() && (!nonempty || value.length > 0) && utf8Length(value) <= maximumBytes && !value.includes("\u0000");
}

function validClientID(value: unknown): value is string {
  return validTrimmedText(value, MAX_CLIENT_ID_BYTES) && value !== "." && value !== ".." && CLIENT_ID_PATTERN.test(value);
}

function validCategoryKey(value: unknown): value is string {
  return validTrimmedText(value, MAX_CATEGORY_KEY_BYTES) && CATEGORY_KEY_PATTERN.test(value);
}

function validTimestamp(value: unknown): value is string {
  if (typeof value !== "string" || !RFC3339_PATTERN.test(value) || value.length > 40) return false;
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return false;
  const [datePart] = value.split("T");
  const [yearText, monthText, dayText] = datePart.split("-");
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  if (month < 1 || month > 12 || day < 1) return false;
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return day <= (days[month - 1] ?? 0);
}

function validSecretReference(value: unknown): value is string {
  return typeof value === "string" && value.length <= 250 && value === value.trim() && SECRET_REFERENCE_PATTERN.test(value);
}

function validSecretMask(value: unknown): value is string {
  return typeof value === "string" && value.length <= 32 && SECRET_MASK_PATTERN.test(value) && !value.includes("secret://") && !value.includes("secretref:");
}

function validChecksum(value: unknown): value is string {
  return typeof value === "string" && CHECKSUM_PATTERN.test(value);
}

function validRequestID(value: unknown): value is string {
  return typeof value === "string" && REQUEST_ID_PATTERN.test(value);
}

function validToken(value: unknown): value is string {
  return typeof value === "string" && TOKEN_PATTERN.test(value);
}

function validCredentialState(value: unknown): value is AdminCredentialState {
  return value === "active" || value === "disabled" || value === "pending_activation";
}

function validReleaseState(value: unknown): value is AdminReleaseState {
  return value === "draft" || value === "validated" || value === "published" || value === "rolled_back";
}

function sensitiveKeyKind(key: string): "reference" | "mask" | "forbidden" | "none" {
  const lower = key.toLowerCase();
  if (lower === "__proto__" || lower === "prototype" || lower === "constructor") return "forbidden";
  const sensitive = lower.includes("secret") || lower.includes("password") || lower.includes("webhook") || lower === "token" || lower.endsWith("_token");
  if (!sensitive) return "none";
  if (lower.endsWith("_ref") || lower.endsWith("ref")) return "reference";
  if (lower.endsWith("_mask") || lower.endsWith("mask")) return "mask";
  return "forbidden";
}

interface SafeJSONOptions {
  readonly allowMasks: boolean;
  readonly requireObject: boolean;
  readonly requireNonemptyObject: boolean;
}

function validateSafeJSONNode(value: unknown, depth: number, options: SafeJSONOptions, seen: Set<object>): SafeJSONValue | undefined {
  if (value === null) return null;
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return Number.isFinite(value) && Math.abs(value) <= Number.MAX_SAFE_INTEGER ? value : undefined;
  if (typeof value === "string") {
    return utf8Length(value) <= MAX_JSON_STRING_BYTES && !value.includes("\u0000") ? value : undefined;
  }
  if (typeof value !== "object" || value === null || depth >= MAX_JSON_DEPTH || seen.has(value)) return undefined;
  seen.add(value);
  try {
    if (Array.isArray(value)) {
      if (value.length > MAX_JSON_COLLECTION) return undefined;
      const output: SafeJSONValue[] = [];
      for (const item of value) {
        const parsed = validateSafeJSONNode(item, depth + 1, options, seen);
        if (parsed === undefined) return undefined;
        output.push(parsed);
      }
      return output;
    }
    if (!isRecord(value)) return undefined;
    const entries = Object.entries(value);
    if (entries.length > MAX_JSON_COLLECTION) return undefined;
    const output: Record<string, SafeJSONValue> = Object.create(null) as Record<string, SafeJSONValue>;
    for (const [key, item] of entries) {
      if (!validTrimmedText(key, 160) || /[\r\n]/.test(key)) return undefined;
      const kind = sensitiveKeyKind(key);
      if (kind === "forbidden") return undefined;
      if (kind === "reference") {
        if (!validSecretReference(item)) return undefined;
        output[key] = item;
        continue;
      }
      if (kind === "mask") {
        if (!options.allowMasks || !validSecretMask(item)) return undefined;
        output[key] = item;
        continue;
      }
      const parsed = validateSafeJSONNode(item, depth + 1, options, seen);
      if (parsed === undefined) return undefined;
      output[key] = parsed;
    }
    return output;
  } finally {
    seen.delete(value);
  }
}

export function normalizeSafeJSONObject(
  value: unknown,
  options: Partial<SafeJSONOptions> = {},
): SafeJSONObject | undefined {
  const resolved: SafeJSONOptions = {
    allowMasks: options.allowMasks ?? false,
    requireObject: true,
    requireNonemptyObject: options.requireNonemptyObject ?? false,
  };
  if (!isRecord(value)) return undefined;
  const parsed = validateSafeJSONNode(value, 0, resolved, new Set<object>());
  if (!isRecord(parsed)) return undefined;
  if (resolved.requireNonemptyObject && Object.keys(parsed).length === 0) return undefined;
  let encoded: string;
  try {
    encoded = JSON.stringify(parsed);
  } catch {
    return undefined;
  }
  if (utf8Length(encoded) > MAX_SAFE_JSON_BYTES) return undefined;
  return parsed as SafeJSONObject;
}

export function parseSafeJSONObjectText(text: string, requireNonempty = false): SafeJSONObject | undefined {
  if (typeof text !== "string" || utf8Length(text) > MAX_SAFE_JSON_BYTES) return undefined;
  try {
    return normalizeSafeJSONObject(JSON.parse(text) as unknown, { requireNonemptyObject: requireNonempty });
  } catch {
    return undefined;
  }
}

function decodeBase64JSON(value: unknown, allowMasks: boolean): SafeJSONObject | undefined {
  if (typeof value !== "string" || value.length > Math.ceil(MAX_SAFE_JSON_BYTES * 4 / 3) + 8 || value.length % 4 !== 0 || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) return undefined;
  let binary: string;
  try {
    binary = globalThis.atob(value);
  } catch {
    return undefined;
  }
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  let decoded: string;
  try {
    decoded = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return undefined;
  }
  if (utf8Length(decoded) > MAX_SAFE_JSON_BYTES) return undefined;
  try {
    return normalizeSafeJSONObject(JSON.parse(decoded) as unknown, { allowMasks });
  } catch {
    return undefined;
  }
}

function parsePublicCredential(value: unknown): PublicCredentialRecord | undefined {
  const keys = ["id", "kind", "client_id", "display_name", "state", "secret_ref", "secret_mask", "version", "created_at", "updated_at"] as const;
  if (!isRecord(value) || !hasExactKeys(value, keys) || !isSafePositiveInteger(value.id) || (value.kind !== "api_client" && value.kind !== "direct_api_key") || !validClientID(value.client_id) || !validTrimmedText(value.display_name, MAX_DISPLAY_NAME_BYTES) || !validCredentialState(value.state) || !validSecretReference(value.secret_ref) || !validSecretMask(value.secret_mask) || !isSafePositiveInteger(value.version) || !validTimestamp(value.created_at) || !validTimestamp(value.updated_at)) return undefined;
  if (Date.parse(value.updated_at) < Date.parse(value.created_at)) return undefined;
  if (value.kind === "direct_api_key" && value.client_id !== "direct-default") return undefined;
  return {
    id: value.id,
    kind: value.kind,
    clientID: value.client_id,
    displayName: value.display_name,
    state: value.state,
    secretRef: value.secret_ref,
    secretMask: value.secret_mask,
    version: value.version,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

function toAPIClient(value: PublicCredentialRecord): AdminCredential | undefined {
  if (value.kind !== "api_client") return undefined;
  return {
    id: value.id,
    kind: "api_client",
    clientID: value.clientID,
    displayName: value.displayName,
    state: value.state,
    secretRef: value.secretRef,
    secretMask: value.secretMask,
    version: value.version,
    createdAt: value.createdAt,
    updatedAt: value.updatedAt,
  };
}

function toDirectSnapshot(value: PublicCredentialRecord): DirectAPIKeySnapshot | undefined {
  if (value.kind !== "direct_api_key") return undefined;
  return {
    configured: true,
    state: value.state,
    secretMask: value.secretMask,
    version: value.version,
    createdAt: value.createdAt,
    updatedAt: value.updatedAt,
  };
}

function parseAPIClientList(value: unknown): readonly AdminCredential[] | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "clients"]) || value.ok !== true || !Array.isArray(value.clients) || value.clients.length > 500) return undefined;
  const clients: AdminCredential[] = [];
  const identifiers = new Set<string>();
  for (const raw of value.clients) {
    const parsed = parsePublicCredential(raw);
    const client = parsed ? toAPIClient(parsed) : undefined;
    if (!client || identifiers.has(client.clientID)) return undefined;
    identifiers.add(client.clientID);
    clients.push(client);
  }
  return clients;
}

function parseAPIClientDetail(value: unknown): AdminCredential | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "client"]) || value.ok !== true) return undefined;
  const parsed = parsePublicCredential(value.client);
  return parsed ? toAPIClient(parsed) : undefined;
}

function parseDirectAPIKey(value: unknown): DirectAPIKeySnapshot | undefined {
  if (!isRecord(value) || value.ok !== true || typeof value.configured !== "boolean") return undefined;
  if (value.configured === false) {
    if (!hasExactKeys(value, ["ok", "configured", "secret_values_exposed"]) || value.secret_values_exposed !== false) return undefined;
    return { configured: false };
  }
  if (!hasExactKeys(value, ["ok", "configured", "api_key"])) return undefined;
  const parsed = parsePublicCredential(value.api_key);
  return parsed ? toDirectSnapshot(parsed) : undefined;
}

function parseCategoryStruct(value: unknown, allowFallback: boolean): AdminCategory | undefined {
  if (!isRecord(value)) return undefined;
  if (allowFallback && hasExactKeys(value, ["key", "enabled", "settings"])) {
    if (!validCategoryKey(value.key) || typeof value.enabled !== "boolean") return undefined;
    const settings = normalizeSafeJSONObject(value.settings, { allowMasks: true });
    if (!settings) return undefined;
    return { key: value.key, enabled: value.enabled, settings, version: 0, persisted: false };
  }
  const keys = ["Key", "Enabled", "Settings", "Version", "UpdatedBy", "UpdatedAt"] as const;
  if (!hasExactKeys(value, keys) || !validCategoryKey(value.Key) || typeof value.Enabled !== "boolean" || !isSafePositiveInteger(value.Version) || !validTrimmedText(value.UpdatedBy, MAX_DISPLAY_NAME_BYTES) || !validTimestamp(value.UpdatedAt)) return undefined;
  const settings = decodeBase64JSON(value.Settings, true);
  if (!settings) return undefined;
  return { key: value.Key, enabled: value.Enabled, settings, version: value.Version, updatedBy: value.UpdatedBy, updatedAt: value.UpdatedAt, persisted: true };
}

function parseCategoryList(value: unknown): readonly AdminCategory[] | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "categories"]) || value.ok !== true || !Array.isArray(value.categories) || value.categories.length > 500) return undefined;
  const categories: AdminCategory[] = [];
  const keys = new Set<string>();
  for (const raw of value.categories) {
    const category = parseCategoryStruct(raw, false);
    if (!category || keys.has(category.key)) return undefined;
    keys.add(category.key);
    categories.push(category);
  }
  return categories.sort((left, right) => left.key < right.key ? -1 : left.key > right.key ? 1 : 0);
}

function parseCategoryDetail(value: unknown): AdminCategory | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "category"]) || value.ok !== true) return undefined;
  return parseCategoryStruct(value.category, true);
}

function nullablePositive(value: unknown): number | undefined | null {
  if (value === null) return undefined;
  return isSafePositiveInteger(value) ? value : null;
}

function nullableTimestamp(value: unknown): string | undefined | null {
  if (value === null) return undefined;
  return validTimestamp(value) ? value : null;
}

function parseReleaseStruct(value: unknown): AdminRelease | undefined {
  const keys = ["ID", "State", "Changes", "Checksum", "BasedOnReleaseID", "RollbackOfReleaseID", "CreatedBy", "PublishedBy", "CreatedAt", "ValidatedAt", "PublishedAt"] as const;
  if (!isRecord(value) || !hasExactKeys(value, keys) || !isSafePositiveInteger(value.ID) || !validReleaseState(value.State) || !validChecksum(value.Checksum) || !validTrimmedText(value.CreatedBy, MAX_DISPLAY_NAME_BYTES) || typeof value.PublishedBy !== "string" || value.PublishedBy !== value.PublishedBy.trim() || utf8Length(value.PublishedBy) > MAX_DISPLAY_NAME_BYTES || !validTimestamp(value.CreatedAt)) return undefined;
  const changes = decodeBase64JSON(value.Changes, false);
  if (!changes || Object.keys(changes).length === 0) return undefined;
  const basedOn = nullablePositive(value.BasedOnReleaseID);
  const rollbackOf = nullablePositive(value.RollbackOfReleaseID);
  const validatedAt = nullableTimestamp(value.ValidatedAt);
  const publishedAt = nullableTimestamp(value.PublishedAt);
  if (basedOn === null || rollbackOf === null || validatedAt === null || publishedAt === null) return undefined;
  if (validatedAt && Date.parse(validatedAt) < Date.parse(value.CreatedAt)) return undefined;
  if (publishedAt && Date.parse(publishedAt) < Date.parse(value.CreatedAt)) return undefined;
  if (validatedAt && publishedAt && Date.parse(publishedAt) < Date.parse(validatedAt)) return undefined;
  switch (value.State) {
    case "draft":
      if (validatedAt || publishedAt || value.PublishedBy !== "" || rollbackOf !== undefined) return undefined;
      break;
    case "validated":
      if (!validatedAt || publishedAt || value.PublishedBy !== "" || rollbackOf !== undefined) return undefined;
      break;
    case "published":
      if (!validatedAt || !publishedAt || !validTrimmedText(value.PublishedBy, MAX_DISPLAY_NAME_BYTES) || rollbackOf !== undefined) return undefined;
      break;
    case "rolled_back":
      if (!validatedAt || !publishedAt || !validTrimmedText(value.PublishedBy, MAX_DISPLAY_NAME_BYTES) || basedOn === undefined || rollbackOf === undefined || basedOn !== rollbackOf) return undefined;
      break;
  }
  return {
    id: value.ID,
    state: value.State,
    changes,
    checksum: value.Checksum,
    ...(basedOn === undefined ? {} : { basedOnReleaseID: basedOn }),
    ...(rollbackOf === undefined ? {} : { rollbackOfReleaseID: rollbackOf }),
    createdBy: value.CreatedBy,
    ...(value.PublishedBy === "" ? {} : { publishedBy: value.PublishedBy }),
    createdAt: value.CreatedAt,
    ...(validatedAt === undefined ? {} : { validatedAt }),
    ...(publishedAt === undefined ? {} : { publishedAt }),
  };
}

function parseReleaseList(value: unknown): readonly AdminRelease[] | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "releases"]) || value.ok !== true || !Array.isArray(value.releases) || value.releases.length > 100) return undefined;
  const releases: AdminRelease[] = [];
  const identifiers = new Set<number>();
  for (const raw of value.releases) {
    const release = parseReleaseStruct(raw);
    if (!release || identifiers.has(release.id)) return undefined;
    identifiers.add(release.id);
    releases.push(release);
  }
  for (let index = 1; index < releases.length; index += 1) {
    const prior = releases[index - 1];
    const current = releases[index];
    const priorTime = Date.parse(prior.createdAt);
    const currentTime = Date.parse(current.createdAt);
    if (priorTime < currentTime || (priorTime === currentTime && prior.id <= current.id)) return undefined;
  }
  return releases;
}

function parseReleaseDetail(value: unknown): AdminRelease | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "release"]) || value.ok !== true) return undefined;
  return parseReleaseStruct(value.release);
}

function parseShadowComparison(value: unknown, expectedID: number): ShadowComparison | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "comparison"]) || typeof value.ok !== "boolean" || !isRecord(value.comparison) || !hasExactKeys(value.comparison, ["release_id", "external_calls"]) || value.comparison.release_id !== expectedID || value.comparison.external_calls !== false) return undefined;
  return { releaseID: expectedID, available: value.ok, externalCalls: false };
}

function parseClientMutationReceipt(value: unknown): PublicCredentialRecord | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "client", "real_external_call_executed"]) || value.ok !== true || value.real_external_call_executed !== false) return undefined;
  return parsePublicCredential(value.client);
}

function parseDirectMutationReceipt(value: unknown): PublicCredentialRecord | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "api_key", "real_external_call_executed"]) || value.ok !== true || value.real_external_call_executed !== false) return undefined;
  return parsePublicCredential(value.api_key);
}

function parseCategoryMutationReceipt(value: unknown): AdminCategory | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "changed", "config", "real_external_call_executed"]) || value.ok !== true || value.changed !== true || value.real_external_call_executed !== false) return undefined;
  return parseCategoryStruct(value.config, false);
}

function parseCategoryCheckReceipt(value: unknown, expectedKey: string): CategoryCheckResult | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "summary", "config", "real_external_call_executed"]) || value.ok !== true || value.real_external_call_executed !== false || !isRecord(value.summary) || !hasExactKeys(value.summary, ["category", "failed", "external_calls"]) || value.summary.category !== expectedKey || !isSafeNonnegativeInteger(value.summary.failed) || value.summary.external_calls !== false) return undefined;
  const category = parseCategoryStruct(value.config, false);
  return category && category.key === expectedKey ? { categoryKey: expectedKey, failed: value.summary.failed, externalCalls: false, category } : undefined;
}

function parseReleaseMutationReceipt(value: unknown): AdminRelease | undefined {
  if (!isRecord(value) || !hasExactKeys(value, ["ok", "release", "real_external_call_executed"]) || value.ok !== true || value.real_external_call_executed !== false) return undefined;
  return parseReleaseStruct(value.release);
}

function sameClient(left: AdminCredential, right: AdminCredential): boolean {
  return left.id === right.id && left.clientID === right.clientID && left.displayName === right.displayName && left.state === right.state && left.secretRef === right.secretRef && left.secretMask === right.secretMask && left.version === right.version && left.createdAt === right.createdAt && left.updatedAt === right.updatedAt;
}

function sameDirect(left: DirectAPIKeySnapshot, right: DirectAPIKeySnapshot): boolean {
  return left.configured === right.configured && left.state === right.state && left.secretMask === right.secretMask && left.version === right.version && left.createdAt === right.createdAt && left.updatedAt === right.updatedAt;
}

function sameSafeJSONValue(left: SafeJSONValue, right: SafeJSONValue): boolean {
  if (left === right) return true;
  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return false;
    return left.every((item, index) => sameSafeJSONValue(item, right[index] as SafeJSONValue));
  }
  if (!isRecord(left) || !isRecord(right)) return false;
  const leftKeys = Object.keys(left).sort();
  const rightKeys = Object.keys(right).sort();
  if (leftKeys.length !== rightKeys.length || leftKeys.some((key, index) => key !== rightKeys[index])) return false;
  return leftKeys.every((key) => sameSafeJSONValue(left[key] as SafeJSONValue, right[key] as SafeJSONValue));
}

function sameJSON(left: SafeJSONObject, right: SafeJSONObject): boolean {
  return sameSafeJSONValue(left, right);
}

function sameCategory(left: AdminCategory, right: AdminCategory): boolean {
  return left.key === right.key && left.enabled === right.enabled && left.version === right.version && left.updatedBy === right.updatedBy && left.updatedAt === right.updatedAt && left.persisted === right.persisted && sameJSON(left.settings, right.settings);
}

function sameRelease(left: AdminRelease, right: AdminRelease): boolean {
  return left.id === right.id && left.state === right.state && left.checksum === right.checksum && left.basedOnReleaseID === right.basedOnReleaseID && left.rollbackOfReleaseID === right.rollbackOfReleaseID && left.createdBy === right.createdBy && left.publishedBy === right.publishedBy && left.createdAt === right.createdAt && left.validatedAt === right.validatedAt && left.publishedAt === right.publishedAt && sameJSON(left.changes, right.changes);
}

function encodePathSegment(value: string): string | undefined {
  if (!validClientID(value) && !validCategoryKey(value)) return undefined;
  const encoded = encodeURIComponent(value);
  return encoded.includes("%2F") || encoded.includes("%5C") ? undefined : encoded;
}

function releasePath(id: number): string | undefined {
  return isSafePositiveInteger(id) ? `/api/admin/config/releases/${String(id)}` : undefined;
}

function readFailure(status: number): AdminConfigReadFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status >= 500) return "unavailable";
  return "invalid";
}

async function readResource<T>(
  transport: AdminConfigTransport,
  path: string,
  // eslint-disable-next-line no-unused-vars -- named parameter documents the parser contract.
  parser: (value: unknown) => T | undefined,
): Promise<AdminConfigReadResult<T>> {
  try {
    const response = await transport.request(path, { method: "GET", credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status !== 200) return { status: readFailure(response.status) };
    const value = parser(response.data);
    return value === undefined ? { status: "invalid" } : { status: "loaded", value };
  } catch {
    return { status: "unavailable" };
  }
}

function writeFailure(status: number): AdminConfigWriteFailure {
  if (status === 400 || status === 404 || status === 405 || status === 422) return "invalid";
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 409) return "conflict";
  return "unknown";
}

function secureHeaders(security: AdminConfigSecurity, action: AdminConfigAction): HeadersInit | undefined {
  let csrfToken: string | undefined;
  let actionToken: string | undefined;
  let requestID: string | undefined;
  try {
    csrfToken = security.csrfToken();
    actionToken = security.actionTokenFor(action.method, action.pattern);
    requestID = security.requestID();
  } catch {
    return undefined;
  }
  if (!validToken(csrfToken) || !validToken(actionToken) || !validRequestID(requestID)) return undefined;
  return {
    Accept: "application/json",
    "Content-Type": "application/json",
    "X-CSRF-Token": csrfToken,
    "X-Admin-Action-Token": actionToken,
    "X-Request-ID": requestID,
  };
}

async function performMutation(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  request: MutationRequest,
): Promise<AdminConfigTransportResponse | AdminConfigWriteFailure> {
  const headers = secureHeaders(security, request.action);
  if (!headers) return "forbidden";
  let body: string;
  try {
    body = JSON.stringify(request.body);
  } catch {
    return "invalid";
  }
  if (utf8Length(body) > MAX_SAFE_JSON_BYTES) return "invalid";
  try {
    const response = await transport.request(request.path, {
      method: request.action.method,
      credentials: "same-origin",
      headers,
      body,
    });
    if (!request.successStatuses.includes(response.status)) return writeFailure(response.status);
    return response;
  } catch {
    return "unknown";
  }
}

function preflightFailure<T>(result: AdminConfigReadResult<T>): AdminConfigWriteFailure | undefined {
  if (result.status === "loaded") return undefined;
  if (result.status === "unauthenticated" || result.status === "forbidden" || result.status === "invalid") return result.status;
  if (result.status === "not_found") return "conflict";
  return "unavailable";
}

export const generatedAdminConfigTransport: AdminConfigTransport = {
  async request(path, init): Promise<AdminConfigTransportResponse> {
    const response = await fetch(path, { ...init, credentials: "same-origin", redirect: "error", cache: "no-store" });
    const contentLength = response.headers.get("content-length");
    if (contentLength !== null && Number(contentLength) > MAX_RESPONSE_BYTES) return { status: response.status, data: undefined };
    const text = await response.text();
    if (utf8Length(text) > MAX_RESPONSE_BYTES) return { status: response.status, data: undefined };
    if (text === "") return { status: response.status, data: undefined };
    try {
      return { status: response.status, data: JSON.parse(text) as unknown };
    } catch {
      return { status: response.status, data: undefined };
    }
  },
};

export function canManageAdminConfig(role: AdminConfigRole): boolean {
  return role === "admin";
}

export function hasCompleteAdminConfigActionTokens(
  // eslint-disable-next-line no-unused-vars -- named parameters document the action-token resolver contract.
  resolver: ((method: string, pattern: string) => string | undefined) | undefined,
): boolean {
  if (!resolver) return false;
  try {
    return Object.values(ADMIN_CONFIG_ACTIONS).every((action) => validToken(resolver(action.method, action.pattern)));
  } catch {
    return false;
  }
}

export function newAdminConfigRequestID(source: { readonly randomUUID: () => string } | undefined = globalThis.crypto): string | undefined {
  try {
    const uuid = source?.randomUUID();
    if (typeof uuid !== "string" || !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid)) return undefined;
    const value = `admin-config:${uuid}`;
    return validRequestID(value) ? value : undefined;
  } catch {
    return undefined;
  }
}

export function filterAPIClients(
  clients: readonly AdminCredential[],
  query: string,
  state: "all" | AdminCredentialState,
): readonly AdminCredential[] | undefined {
  const normalized = query.trim().toLowerCase();
  if (utf8Length(normalized) > MAX_FILTER_BYTES) return undefined;
  return clients.filter((client) => {
    if (state !== "all" && client.state !== state) return false;
    return normalized === "" || `${client.clientID} ${client.displayName}`.toLowerCase().includes(normalized);
  });
}

export async function loadAPIClients(transport: AdminConfigTransport): Promise<AdminConfigReadResult<readonly AdminCredential[]>> {
  return readResource(transport, "/api/admin/config/api-clients", parseAPIClientList);
}

export async function loadAPIClient(transport: AdminConfigTransport, clientID: string): Promise<AdminConfigReadResult<AdminCredential>> {
  const encoded = validClientID(clientID) ? encodePathSegment(clientID) : undefined;
  if (!encoded) return { status: "invalid" };
  return readResource(transport, `/api/admin/config/api-clients/${encoded}`, parseAPIClientDetail);
}

export async function loadDirectAPIKey(transport: AdminConfigTransport): Promise<AdminConfigReadResult<DirectAPIKeySnapshot>> {
  return readResource(transport, "/api/admin/config/api-key", parseDirectAPIKey);
}

export async function loadCategories(transport: AdminConfigTransport): Promise<AdminConfigReadResult<readonly AdminCategory[]>> {
  return readResource(transport, "/api/admin/config/categories", parseCategoryList);
}

export async function loadCategory(transport: AdminConfigTransport, key: string): Promise<AdminConfigReadResult<AdminCategory>> {
  const encoded = validCategoryKey(key) ? encodePathSegment(key) : undefined;
  if (!encoded) return { status: "invalid" };
  return readResource(transport, `/api/admin/config/categories/${encoded}`, parseCategoryDetail);
}

export async function loadReleases(transport: AdminConfigTransport): Promise<AdminConfigReadResult<readonly AdminRelease[]>> {
  return readResource(transport, "/api/admin/config/releases", parseReleaseList);
}

export async function loadRelease(transport: AdminConfigTransport, id: number): Promise<AdminConfigReadResult<AdminRelease>> {
  const path = releasePath(id);
  if (!path) return { status: "invalid" };
  return readResource(transport, path, parseReleaseDetail);
}

export async function loadShadowComparison(transport: AdminConfigTransport, id: number): Promise<AdminConfigReadResult<ShadowComparison>> {
  const path = releasePath(id);
  if (!path) return { status: "invalid" };
  return readResource(transport, `${path}/shadow-compare`, (value) => parseShadowComparison(value, id));
}

export async function loadAdminConfigOverview(transport: AdminConfigTransport): Promise<AdminConfigOverview> {
  const [clients, directKey, categories, releases] = await Promise.all([
    loadAPIClients(transport),
    loadDirectAPIKey(transport),
    loadCategories(transport),
    loadReleases(transport),
  ]);
  return { clients, directKey, categories, releases };
}

export interface CreateAPIClientInput {
  readonly clientID: string;
  readonly displayName: string;
  readonly metadata: SafeJSONObject;
}

export async function createAPIClient(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: CreateAPIClientInput,
): Promise<AdminConfigWriteResult<AdminCredential>> {
  if (!validClientID(input.clientID) || !validTrimmedText(input.displayName, MAX_DISPLAY_NAME_BYTES)) return { status: "invalid" };
  const metadata = normalizeSafeJSONObject(input.metadata);
  if (!metadata) return { status: "invalid" };
  const response = await performMutation(transport, security, {
    path: "/api/admin/config/api-clients",
    action: ADMIN_CONFIG_ACTIONS.createClient,
    body: { client_id: input.clientID, display_name: input.displayName, metadata, confirm: true },
    successStatuses: [201],
  });
  if (typeof response === "string") return { status: response };
  const receipt = parseClientMutationReceipt(response.data);
  const client = receipt ? toAPIClient(receipt) : undefined;
  if (!client || client.clientID !== input.clientID || client.displayName !== input.displayName || client.state !== "pending_activation" || client.version !== 1) return { status: "unknown" };
  const readback = await loadAPIClient(transport, input.clientID);
  if (readback.status !== "loaded") return { status: "unknown" };
  return sameClient(readback.value, client) ? { status: "applied", value: readback.value } : { status: "conflict" };
}

export interface UpdateAPIClientInput {
  readonly clientID: string;
  readonly expectedVersion: number;
  readonly displayName: string;
  readonly metadata: SafeJSONObject;
}

export async function updateAPIClient(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: UpdateAPIClientInput,
): Promise<AdminConfigWriteResult<AdminCredential>> {
  if (!validClientID(input.clientID) || !isSafePositiveInteger(input.expectedVersion) || !validTrimmedText(input.displayName, MAX_DISPLAY_NAME_BYTES)) return { status: "invalid" };
  const metadata = normalizeSafeJSONObject(input.metadata);
  if (!metadata) return { status: "invalid" };
  const current = await loadAPIClient(transport, input.clientID);
  const failure = preflightFailure(current);
  if (failure) return { status: failure };
  if (current.status !== "loaded") return { status: "unavailable" };
  if (current.value.version !== input.expectedVersion || current.value.state === "active") return { status: "conflict" };
  const encoded = encodePathSegment(input.clientID);
  if (!encoded) return { status: "invalid" };
  const response = await performMutation(transport, security, {
    path: `/api/admin/config/api-clients/${encoded}`,
    action: ADMIN_CONFIG_ACTIONS.updateClient,
    body: { display_name: input.displayName, metadata, confirm: true },
    successStatuses: [200],
  });
  if (typeof response === "string") return { status: response };
  const receiptRecord = parseClientMutationReceipt(response.data);
  const receipt = receiptRecord ? toAPIClient(receiptRecord) : undefined;
  if (!receipt || receipt.id !== current.value.id || receipt.clientID !== input.clientID || receipt.version !== current.value.version + 1 || receipt.displayName !== input.displayName || receipt.state !== current.value.state || receipt.secretRef !== current.value.secretRef || receipt.secretMask !== current.value.secretMask || receipt.createdAt !== current.value.createdAt) return { status: "unknown" };
  const readback = await loadAPIClient(transport, input.clientID);
  if (readback.status !== "loaded") return { status: "unknown" };
  return sameClient(readback.value, receipt) ? { status: "applied", value: readback.value } : { status: "conflict" };
}

interface ExistingClientActionInput {
  readonly clientID: string;
  readonly expectedVersion: number;
}

async function clientStateMutation(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: ExistingClientActionInput,
  // eslint-disable-next-line no-unused-vars -- named parameters document the mutation factory contract.
  request: (client: AdminCredential, encoded: string) => MutationRequest | undefined,
  // eslint-disable-next-line no-unused-vars -- named parameters document the receipt verifier contract.
  verify: (before: AdminCredential, after: AdminCredential) => boolean,
): Promise<AdminConfigWriteResult<AdminCredential>> {
  if (!validClientID(input.clientID) || !isSafePositiveInteger(input.expectedVersion)) return { status: "invalid" };
  const current = await loadAPIClient(transport, input.clientID);
  const failure = preflightFailure(current);
  if (failure) return { status: failure };
  if (current.status !== "loaded") return { status: "unavailable" };
  if (current.value.version !== input.expectedVersion) return { status: "conflict" };
  const encoded = encodePathSegment(input.clientID);
  const mutation = encoded ? request(current.value, encoded) : undefined;
  if (!mutation) return { status: "invalid" };
  const response = await performMutation(transport, security, mutation);
  if (typeof response === "string") return { status: response };
  const receiptRecord = parseClientMutationReceipt(response.data);
  const receipt = receiptRecord ? toAPIClient(receiptRecord) : undefined;
  if (!receipt || receipt.id !== current.value.id || receipt.clientID !== input.clientID || receipt.version !== current.value.version + 1 || receipt.createdAt !== current.value.createdAt || !verify(current.value, receipt)) return { status: "unknown" };
  const readback = await loadAPIClient(transport, input.clientID);
  if (readback.status !== "loaded") return { status: "unknown" };
  return sameClient(readback.value, receipt) ? { status: "applied", value: readback.value } : { status: "conflict" };
}

export async function rotateAPIClientSecret(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: ExistingClientActionInput,
): Promise<AdminConfigWriteResult<AdminCredential>> {
  return clientStateMutation(
    transport,
    security,
    input,
    (_client, encoded) => ({ path: `/api/admin/config/api-clients/${encoded}/rotate-secret`, action: ADMIN_CONFIG_ACTIONS.rotateClient, body: { confirm: true }, successStatuses: [200] }),
    (before, after) => after.state === "pending_activation" && after.secretRef !== before.secretRef && after.secretMask !== before.secretMask && after.displayName === before.displayName,
  );
}

export interface ActivateAPIClientInput extends ExistingClientActionInput {
  readonly secretRef: string;
  readonly copiedConfirmed: boolean;
}

export async function activateAPIClient(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: ActivateAPIClientInput,
): Promise<AdminConfigWriteResult<AdminCredential>> {
  if (!validSecretReference(input.secretRef) || input.copiedConfirmed !== true) return { status: "invalid" };
  return clientStateMutation(
    transport,
    security,
    input,
    (client, encoded) => client.secretRef === input.secretRef && client.state !== "active"
      ? { path: `/api/admin/config/api-clients/${encoded}/activate`, action: ADMIN_CONFIG_ACTIONS.activateClient, body: { confirm: true, copied_confirmed: true, secret_ref: input.secretRef }, successStatuses: [200] }
      : undefined,
    (before, after) => after.state === "active" && after.secretRef === before.secretRef && after.secretMask === before.secretMask && after.displayName === before.displayName,
  );
}

export async function disableAPIClient(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: ExistingClientActionInput,
): Promise<AdminConfigWriteResult<AdminCredential>> {
  return clientStateMutation(
    transport,
    security,
    input,
    (client, encoded) => client.state !== "disabled"
      ? { path: `/api/admin/config/api-clients/${encoded}/enabled`, action: ADMIN_CONFIG_ACTIONS.disableClient, body: { confirm: true, enabled: false }, successStatuses: [200] }
      : undefined,
    (before, after) => after.state === "disabled" && after.secretRef === before.secretRef && after.secretMask === before.secretMask && after.displayName === before.displayName,
  );
}

export interface DirectKeyConfirmation {
  readonly confirmed: boolean;
  readonly confirmationText: string;
}

function validDirectKeyConfirmation(action: keyof typeof DIRECT_KEY_CONFIRMATION_PHRASES, confirmation: DirectKeyConfirmation): boolean {
  return confirmation.confirmed === true && confirmation.confirmationText === DIRECT_KEY_CONFIRMATION_PHRASES[action];
}

async function directKeyMutation(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  action: keyof typeof DIRECT_KEY_CONFIRMATION_PHRASES,
  confirmation: DirectKeyConfirmation,
  expected?: DirectAPIKeySnapshot,
): Promise<AdminConfigWriteResult<DirectAPIKeySnapshot>> {
  if (!validDirectKeyConfirmation(action, confirmation)) return { status: "invalid" };
  const current = await loadDirectAPIKey(transport);
  const failure = preflightFailure(current);
  if (failure) return { status: failure };
  if (current.status !== "loaded") return { status: "unavailable" };
  if (expected && !sameDirect(current.value, expected)) return { status: "conflict" };
  if (action === "generate" && current.value.configured) return { status: "conflict" };
  if (action !== "generate" && !current.value.configured) return { status: "conflict" };
  if (action === "disable" && current.value.state === "disabled") return { status: "conflict" };
  const spec = action === "generate"
    ? { path: "/api/admin/config/api-key/generate", action: ADMIN_CONFIG_ACTIONS.generateDirectKey, body: { confirm: true }, successStatuses: [201] as const }
    : action === "rotate"
      ? { path: "/api/admin/config/api-key/rotate", action: ADMIN_CONFIG_ACTIONS.rotateDirectKey, body: { confirm: true }, successStatuses: [200] as const }
      : { path: "/api/admin/config/api-key/enabled", action: ADMIN_CONFIG_ACTIONS.disableDirectKey, body: { confirm: true, enabled: false }, successStatuses: [200] as const };
  const response = await performMutation(transport, security, spec);
  if (typeof response === "string") return { status: response };
  const receipt = parseDirectMutationReceipt(response.data);
  const snapshot = receipt ? toDirectSnapshot(receipt) : undefined;
  if (!receipt || !snapshot || receipt.kind !== "direct_api_key" || !snapshot.configured) return { status: "unknown" };
  if (current.value.configured && (snapshot.version !== (current.value.version ?? 0) + 1 || snapshot.createdAt !== current.value.createdAt)) return { status: "unknown" };
  if (action === "generate" && (snapshot.version !== 1 || snapshot.state !== "active")) return { status: "unknown" };
  if (action === "disable" && (snapshot.state !== "disabled" || snapshot.secretMask !== current.value.secretMask)) return { status: "unknown" };
  if (action === "rotate" && (snapshot.state !== "pending_activation" || current.value.secretMask === snapshot.secretMask)) return { status: "unknown" };
  const readback = await loadDirectAPIKey(transport);
  if (readback.status !== "loaded") return { status: "unknown" };
  return sameDirect(readback.value, snapshot) ? { status: "applied", value: readback.value } : { status: "conflict" };
}

export async function generateDirectAPIKey(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  confirmation: DirectKeyConfirmation,
): Promise<AdminConfigWriteResult<DirectAPIKeySnapshot>> {
  return directKeyMutation(transport, security, "generate", confirmation);
}

export async function rotateDirectAPIKey(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  expected: DirectAPIKeySnapshot,
  confirmation: DirectKeyConfirmation,
): Promise<AdminConfigWriteResult<DirectAPIKeySnapshot>> {
  return directKeyMutation(transport, security, "rotate", confirmation, expected);
}

export async function disableDirectAPIKey(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  expected: DirectAPIKeySnapshot,
  confirmation: DirectKeyConfirmation,
): Promise<AdminConfigWriteResult<DirectAPIKeySnapshot>> {
  return directKeyMutation(transport, security, "disable", confirmation, expected);
}

interface CategoryMutationInput {
  readonly key: string;
  readonly expectedVersion: number;
}

async function categoryMutation(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: CategoryMutationInput,
  // eslint-disable-next-line no-unused-vars -- named parameters document the mutation factory contract.
  request: (current: AdminCategory, encoded: string) => MutationRequest | undefined,
  // eslint-disable-next-line no-unused-vars -- named parameters document the receipt verifier contract.
  verify: (before: AdminCategory, after: AdminCategory) => boolean,
): Promise<AdminConfigWriteResult<AdminCategory>> {
  if (!validCategoryKey(input.key) || !isSafeNonnegativeInteger(input.expectedVersion)) return { status: "invalid" };
  const current = await loadCategory(transport, input.key);
  const failure = preflightFailure(current);
  if (failure) return { status: failure };
  if (current.status !== "loaded") return { status: "unavailable" };
  if (current.value.version !== input.expectedVersion) return { status: "conflict" };
  const encoded = encodePathSegment(input.key);
  const mutation = encoded ? request(current.value, encoded) : undefined;
  if (!mutation) return { status: "invalid" };
  const response = await performMutation(transport, security, mutation);
  if (typeof response === "string") return { status: response };
  const receipt = parseCategoryMutationReceipt(response.data);
  const expectedVersion = current.value.persisted ? current.value.version + 1 : 1;
  if (!receipt || receipt.key !== input.key || receipt.version !== expectedVersion || !verify(current.value, receipt)) return { status: "unknown" };
  const readback = await loadCategory(transport, input.key);
  if (readback.status !== "loaded") return { status: "unknown" };
  return sameCategory(readback.value, receipt) ? { status: "applied", value: readback.value } : { status: "conflict" };
}

export async function setCategoryEnabled(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: CategoryMutationInput & { readonly enabled: boolean },
): Promise<AdminConfigWriteResult<AdminCategory>> {
  if (typeof input.enabled !== "boolean") return { status: "invalid" };
  return categoryMutation(
    transport,
    security,
    input,
    (_current, encoded) => ({ path: `/api/admin/config/categories/${encoded}/enabled`, action: ADMIN_CONFIG_ACTIONS.setCategoryEnabled, body: { enabled: input.enabled }, successStatuses: [200] }),
    (before, after) => after.enabled === input.enabled && sameJSON(after.settings, before.settings),
  );
}

export async function saveCategorySettings(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: CategoryMutationInput & { readonly settings: SafeJSONObject },
): Promise<AdminConfigWriteResult<AdminCategory>> {
  const settings = normalizeSafeJSONObject(input.settings);
  if (!settings) return { status: "invalid" };
  return categoryMutation(
    transport,
    security,
    input,
    (_current, encoded) => ({ path: `/api/admin/config/categories/${encoded}/settings`, action: ADMIN_CONFIG_ACTIONS.saveCategorySettings, body: { settings }, successStatuses: [200] }),
    (before, after) => after.enabled === (before.persisted ? before.enabled : true) && sameJSON(after.settings, settings),
  );
}

export async function checkCategoryLocally(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  key: string,
): Promise<AdminConfigWriteResult<CategoryCheckResult>> {
  if (!validCategoryKey(key)) return { status: "invalid" };
  const encoded = encodePathSegment(key);
  if (!encoded) return { status: "invalid" };
  const response = await performMutation(transport, security, {
    path: `/api/admin/config/categories/${encoded}/check`,
    action: ADMIN_CONFIG_ACTIONS.checkCategory,
    body: {},
    successStatuses: [200],
  });
  if (typeof response === "string") return { status: response === "unknown" ? "unavailable" : response };
  const result = parseCategoryCheckReceipt(response.data, key);
  return result ? { status: "applied", value: result } : { status: "unavailable" };
}

export async function createReleaseDraft(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  changesInput: SafeJSONObject,
): Promise<AdminConfigWriteResult<AdminRelease>> {
  const changes = normalizeSafeJSONObject(changesInput, { requireNonemptyObject: true });
  if (!changes) return { status: "invalid" };
  const response = await performMutation(transport, security, {
    path: "/api/admin/config/releases",
    action: ADMIN_CONFIG_ACTIONS.createRelease,
    body: { changes, confirm: true },
    successStatuses: [200],
  });
  if (typeof response === "string") return { status: response };
  const receipt = parseReleaseMutationReceipt(response.data);
  if (!receipt || receipt.state !== "draft" || !sameJSON(receipt.changes, changes)) return { status: "unknown" };
  const readback = await loadRelease(transport, receipt.id);
  if (readback.status !== "loaded") return { status: "unknown" };
  return sameRelease(readback.value, receipt) ? { status: "applied", value: readback.value } : { status: "conflict" };
}

interface ReleaseActionInput {
  readonly releaseID: number;
  readonly expectedState: AdminReleaseState;
  readonly expectedChecksum: string;
}

async function releaseMutation(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: ReleaseActionInput,
  // eslint-disable-next-line no-unused-vars -- named parameters document the mutation factory contract.
  request: (current: AdminRelease, path: string) => MutationRequest | undefined,
  // eslint-disable-next-line no-unused-vars -- named parameters document the receipt verifier contract.
  verify: (before: AdminRelease, after: AdminRelease) => boolean,
): Promise<AdminConfigWriteResult<AdminRelease>> {
  if (!isSafePositiveInteger(input.releaseID) || !validReleaseState(input.expectedState) || !validChecksum(input.expectedChecksum)) return { status: "invalid" };
  const current = await loadRelease(transport, input.releaseID);
  const failure = preflightFailure(current);
  if (failure) return { status: failure };
  if (current.status !== "loaded") return { status: "unavailable" };
  if (current.value.state !== input.expectedState || current.value.checksum !== input.expectedChecksum) return { status: "conflict" };
  const path = releasePath(input.releaseID);
  const mutation = path ? request(current.value, path) : undefined;
  if (!mutation) return { status: "invalid" };
  const response = await performMutation(transport, security, mutation);
  if (typeof response === "string") return { status: response };
  const receipt = parseReleaseMutationReceipt(response.data);
  if (!receipt || !verify(current.value, receipt)) return { status: "unknown" };
  const readback = await loadRelease(transport, receipt.id);
  if (readback.status !== "loaded") return { status: "unknown" };
  if (!sameRelease(readback.value, receipt)) return { status: "conflict" };
  return { status: "applied", value: readback.value };
}

export async function validateReleaseLocally(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: Omit<ReleaseActionInput, "expectedState">,
): Promise<AdminConfigWriteResult<AdminRelease>> {
  return releaseMutation(
    transport,
    security,
    { ...input, expectedState: "draft" },
    (_current, path) => ({ path: `${path}/validate`, action: ADMIN_CONFIG_ACTIONS.validateRelease, body: {}, successStatuses: [200] }),
    (before, after) => after.id === before.id && after.state === "validated" && after.checksum === before.checksum && sameJSON(after.changes, before.changes) && after.basedOnReleaseID === before.basedOnReleaseID && after.rollbackOfReleaseID === before.rollbackOfReleaseID && after.createdBy === before.createdBy && after.createdAt === before.createdAt && after.publishedBy === undefined && after.validatedAt !== undefined && after.publishedAt === undefined,
  );
}

export async function publishReleaseLocally(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: Omit<ReleaseActionInput, "expectedState">,
): Promise<AdminConfigWriteResult<AdminRelease>> {
  return releaseMutation(
    transport,
    security,
    { ...input, expectedState: "validated" },
    (current, path) => ({ path: `${path}/publish`, action: ADMIN_CONFIG_ACTIONS.publishRelease, body: { confirm: true, checksum: current.checksum }, successStatuses: [200] }),
    (before, after) => after.id === before.id && after.state === "published" && after.checksum === before.checksum && sameJSON(after.changes, before.changes) && after.basedOnReleaseID === before.basedOnReleaseID && after.rollbackOfReleaseID === before.rollbackOfReleaseID && after.createdBy === before.createdBy && after.createdAt === before.createdAt && after.validatedAt === before.validatedAt && after.publishedAt !== undefined && after.publishedBy !== undefined,
  );
}

export async function rollbackReleaseLocally(
  transport: AdminConfigTransport,
  security: AdminConfigSecurity,
  input: Omit<ReleaseActionInput, "expectedState">,
): Promise<AdminConfigWriteResult<AdminRelease>> {
  const result = await releaseMutation(
    transport,
    security,
    { ...input, expectedState: "published" },
    (_current, path) => ({ path: `${path}/rollback`, action: ADMIN_CONFIG_ACTIONS.rollbackRelease, body: { confirm: true }, successStatuses: [200] }),
    (before, after) => after.id !== before.id && after.state === "rolled_back" && after.rollbackOfReleaseID === before.id && after.basedOnReleaseID === before.id && after.checksum === before.checksum && sameJSON(after.changes, before.changes),
  );
  if (result.status !== "applied") return result;
  const original = await loadRelease(transport, input.releaseID);
  if (original.status !== "loaded") return { status: "unknown" };
  if (original.value.state !== "published" || original.value.checksum !== input.expectedChecksum) return { status: "conflict" };
  return result;
}

export function redactAdminConfigError(status: AdminConfigReadFailure | AdminConfigWriteFailure): string {
  switch (status) {
    case "unauthenticated": return "登录状态已失效，请重新登录。";
    case "forbidden": return "当前会话没有 Admin 配置管理权限或缺少安全令牌。";
    case "not_found": return "本地配置记录不存在。";
    case "invalid": return "本地配置请求或响应不符合安全合同。";
    case "conflict": return "本地配置已发生版本或状态冲突；未自动重试，请刷新后核对。";
    case "unavailable": return "本地配置读取暂不可用；没有执行写操作。";
    case "unknown": return ADMIN_CONFIG_UNKNOWN_NOTICE;
  }
}
