import {
  createSegment,
  listSegmentMembers,
  listSegments,
  requestSegmentRefresh,
  updateSegment,
  type CreateSegmentRequest,
  type ListSegmentMembersParams,
  type ListSegmentsParams,
  type SegmentDefinition,
  type UpdateSegmentRequest,
} from "./api/generated/health";
import { parseCustomer, type CustomerRecord } from "./customers";

export type SegmentRole = "admin" | "ops" | "sales";
export type SegmentField =
  | "stage_id"
  | "owner_staff_id"
  | "channel_id"
  | "tag_id"
  | "added_at"
  | "last_interact_at"
  | "is_deleted";
export type SegmentOperator = "eq" | "in" | "has_any" | "before" | "after";

export interface SegmentPredicateDraft {
  readonly kind: "predicate";
  readonly field: SegmentField;
  readonly operator: SegmentOperator;
  readonly value: string;
}
export interface SegmentGroupDraft {
  readonly kind: "group";
  readonly combinator: "and" | "or";
  readonly children: readonly SegmentConditionDraft[];
}
export type SegmentConditionDraft = SegmentPredicateDraft | SegmentGroupDraft;

export interface SegmentEditorDraft {
  readonly name: string;
  readonly refreshMode: "manual" | "scheduled";
  readonly refreshCron: string;
  readonly condition: SegmentConditionDraft;
}

export interface SegmentRecord {
  readonly id: number;
  readonly name: string;
  readonly definition: SegmentDefinition;
  readonly refreshMode: "manual" | "scheduled";
  readonly refreshCron?: string;
  readonly memberCount: number;
  readonly refreshedAt?: string;
  readonly refreshStatus: "idle" | "running" | "failed";
  readonly lifecycleStatus: "active" | "archived";
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface SegmentTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedList(params: ListSegmentsParams, options: RequestInit) {
  return listSegments(params, options);
}
async function generatedCreate(request: CreateSegmentRequest, options: RequestInit) {
  return createSegment(request, options);
}
async function generatedUpdate(id: number, request: UpdateSegmentRequest, options: RequestInit) {
  return updateSegment(id, request, options);
}
async function generatedMembers(id: number, params: ListSegmentMembersParams, options: RequestInit) {
  return listSegmentMembers(id, params, options);
}
async function generatedRefresh(id: number, options: RequestInit) {
  return requestSegmentRefresh(id, options);
}

export interface SegmentTransport {
  readonly list: typeof generatedList;
  readonly create: typeof generatedCreate;
  readonly update: typeof generatedUpdate;
  readonly members: typeof generatedMembers;
  readonly refresh: typeof generatedRefresh;
  readonly archive?: (
    // eslint-disable-next-line no-unused-vars -- named parameter documents the generated transport contract.
    id: number,
    // eslint-disable-next-line no-unused-vars -- named parameter documents the generated transport contract.
    options: RequestInit,
  ) => Promise<SegmentTransportResponse>;
}

export const generatedSegmentTransport: SegmentTransport = {
  list: generatedList,
  create: generatedCreate,
  update: generatedUpdate,
  members: generatedMembers,
  refresh: generatedRefresh,
};

export type SegmentFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";
export type SegmentListResult =
  | { readonly status: "loaded"; readonly items: readonly SegmentRecord[]; readonly nextCursor?: string }
  | { readonly status: SegmentFailure };
export type SegmentMembersResult =
  | { readonly status: "loaded"; readonly items: readonly CustomerRecord[]; readonly nextCursor?: string }
  | { readonly status: SegmentFailure };
export type SegmentMutationResult =
  | { readonly status: "saved"; readonly segment: SegmentRecord }
  | { readonly status: SegmentFailure };
export type SegmentArchiveResult =
  | { readonly status: "archived"; readonly segment: SegmentRecord }
  | { readonly status: SegmentFailure };
export type BuildDefinitionResult =
  | { readonly ok: true; readonly definition: SegmentDefinition }
  | { readonly ok: false; readonly message: string };

const fields = new Set<SegmentField>([
  "stage_id", "owner_staff_id", "channel_id", "tag_id", "added_at", "last_interact_at", "is_deleted",
]);
const MAX_DEPTH = 8;
const MAX_NODES = 128;

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function timestamp(value: unknown): value is string {
  return typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value));
}
function cursor(value: unknown): value is string | null {
  return value === null || (typeof value === "string" && value.length > 0 && value.length <= 512);
}

function parseDefinition(value: unknown, depth = 1, count = { value: 0 }): SegmentDefinition | undefined {
  if (!record(value) || depth > MAX_DEPTH || ++count.value > MAX_NODES) return undefined;
  const keys = Object.keys(value);
  if (keys.length === 1 && (keys[0] === "and" || keys[0] === "or")) {
    const join = keys[0] as "and" | "or";
    const children = value[join];
    if (!Array.isArray(children) || children.length < 1 || children.length > 64) return undefined;
    const parsed = children.map((child) => parseDefinition(child, depth + 1, count));
    if (parsed.some((child) => child === undefined)) return undefined;
    return join === "and" ? { and: parsed as SegmentDefinition[] } : { or: parsed as SegmentDefinition[] };
  }
  if (keys.length !== 3 || !keys.includes("field") || !keys.includes("op") || !keys.includes("value")) return undefined;
  if (typeof value.field !== "string" || !fields.has(value.field as SegmentField) || typeof value.op !== "string") return undefined;
  const field = value.field as SegmentField;
  const op = value.op as SegmentOperator;
  const operand = value.value;
  if (field === "is_deleted") return op === "eq" && typeof operand === "boolean" ? { field, op, value: operand } : undefined;
  if (field === "added_at" || field === "last_interact_at") return (op === "before" || op === "after") && (timestamp(operand) || (typeof operand === "string" && /^-[1-9]\d{0,3}d$/.test(operand))) ? { field, op, value: operand } : undefined;
  if (field === "tag_id") return op === "has_any" && Array.isArray(operand) && operand.length > 0 && operand.length <= 1000 && operand.every(positive) ? { field, op, value: operand } : undefined;
  if (op === "eq" && positive(operand)) return { field, op, value: operand };
  return op === "in" && Array.isArray(operand) && operand.length > 0 && operand.length <= 1000 && operand.every(positive) ? { field, op, value: operand } : undefined;
}

export function parseSegment(value: unknown): SegmentRecord | undefined {
  if (!record(value)) return undefined;
  const allowed = new Set(["id", "name", "definition", "refresh_mode", "refresh_cron", "member_count", "refreshed_at", "refresh_status", "lifecycle_status", "created_at", "updated_at"]);
  if (Object.keys(value).length !== allowed.size || Object.keys(value).some((key) => !allowed.has(key))) return undefined;
  const definition = parseDefinition(value.definition);
  if (!positive(value.id) || typeof value.name !== "string" || value.name.length < 1 || value.name.length > 200 || !definition || (value.refresh_mode !== "manual" && value.refresh_mode !== "scheduled") || (value.refresh_cron !== null && (typeof value.refresh_cron !== "string" || value.refresh_cron.length < 1 || value.refresh_cron.length > 200)) || !nonnegative(value.member_count) || (value.refreshed_at !== null && !timestamp(value.refreshed_at)) || (value.refresh_status !== "idle" && value.refresh_status !== "running" && value.refresh_status !== "failed") || (value.lifecycle_status !== "active" && value.lifecycle_status !== "archived") || !timestamp(value.created_at) || !timestamp(value.updated_at)) return undefined;
  return { id: value.id, name: value.name, definition, refreshMode: value.refresh_mode, ...(value.refresh_cron === null ? {} : { refreshCron: value.refresh_cron }), memberCount: value.member_count, ...(value.refreshed_at === null ? {} : { refreshedAt: value.refreshed_at }), refreshStatus: value.refresh_status, lifecycleStatus: value.lifecycle_status, createdAt: value.created_at, updatedAt: value.updated_at };
}

function failure(status: number): SegmentFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400 || status === 422) return "invalid";
  return "unavailable";
}

export async function loadSegments(transport: SegmentTransport, cursorValue?: string): Promise<SegmentListResult> {
  try {
    const response = await transport.list({ limit: 50, ...(cursorValue ? { cursor: cursorValue } : {}) }, { credentials: "same-origin" });
    if (response.status !== 200 || !record(response.data) || !Array.isArray(response.data.items) || !cursor(response.data.next_cursor)) return response.status === 200 ? { status: "unavailable" } : { status: failure(response.status) };
    const items = response.data.items.map(parseSegment);
    return items.some((item) => !item || item.lifecycleStatus !== "active") ? { status: "unavailable" } : { status: "loaded", items: items as SegmentRecord[], ...(response.data.next_cursor ? { nextCursor: response.data.next_cursor } : {}) };
  } catch { return { status: "unavailable" }; }
}

export async function loadSegmentMembers(transport: SegmentTransport, id: number, cursorValue?: string): Promise<SegmentMembersResult> {
  try {
    const response = await transport.members(id, { limit: 50, ...(cursorValue ? { cursor: cursorValue } : {}) }, { credentials: "same-origin" });
    if (response.status !== 200 || !record(response.data) || !Array.isArray(response.data.items) || !cursor(response.data.next_cursor)) return response.status === 200 ? { status: "unavailable" } : { status: failure(response.status) };
    const items = response.data.items.map(parseCustomer);
    return items.some((item) => !item) ? { status: "unavailable" } : { status: "loaded", items: items as CustomerRecord[], ...(response.data.next_cursor ? { nextCursor: response.data.next_cursor } : {}) };
  } catch { return { status: "unavailable" }; }
}

export function editorDraft(segment?: SegmentRecord): SegmentEditorDraft {
  return segment ? { name: segment.name, refreshMode: segment.refreshMode, refreshCron: segment.refreshCron ?? "", condition: draftFromDefinition(segment.definition) } : { name: "", refreshMode: "manual", refreshCron: "", condition: { kind: "predicate", field: "stage_id", operator: "eq", value: "" } };
}

function draftFromDefinition(definition: SegmentDefinition): SegmentConditionDraft {
  if ("and" in definition) return { kind: "group", combinator: "and", children: definition.and.map(draftFromDefinition) };
  if ("or" in definition) return { kind: "group", combinator: "or", children: definition.or.map(draftFromDefinition) };
  return { kind: "predicate", field: definition.field, operator: definition.op, value: Array.isArray(definition.value) ? definition.value.join(",") : String(definition.value) };
}

function operatorAllowed(field: SegmentField, operator: SegmentOperator): boolean {
  if (field === "is_deleted") return operator === "eq";
  if (field === "added_at" || field === "last_interact_at") return operator === "before" || operator === "after";
  if (field === "tag_id") return operator === "has_any";
  return operator === "eq" || operator === "in";
}
function numericValues(value: string): number[] | undefined {
  const values = value.split(",").map((item) => item.trim());
  if (values.length < 1 || values.length > 1000 || values.some((item) => !/^[1-9]\d*$/.test(item))) return undefined;
  const parsed = values.map(Number);
  return parsed.every(Number.isSafeInteger) ? parsed : undefined;
}
function buildCondition(condition: SegmentConditionDraft): BuildDefinitionResult {
  if (condition.kind === "group") {
    if (condition.children.length < 1 || condition.children.length > 64) return { ok: false, message: "每个条件组必须包含 1 至 64 条条件。" };
    const children = condition.children.map(buildCondition);
    const failed = children.find((child): child is { ok: false; message: string } => !child.ok);
    if (failed) return failed;
    const definitions = children.map((child) => (child as { ok: true; definition: SegmentDefinition }).definition);
    return { ok: true, definition: condition.combinator === "and" ? { and: definitions } : { or: definitions } };
  }
  if (!operatorAllowed(condition.field, condition.operator)) return { ok: false, message: "该字段不支持所选操作符。" };
  if (condition.field === "is_deleted") {
    if (condition.value !== "true" && condition.value !== "false") return { ok: false, message: "删除状态只能选择是或否。" };
    return { ok: true, definition: { field: condition.field, op: "eq", value: condition.value === "true" } };
  }
  if (condition.field === "added_at" || condition.field === "last_interact_at") {
    if (!/^-[1-9]\d{0,3}d$/.test(condition.value) && !timestamp(condition.value)) return { ok: false, message: "时间条件使用 UTC 时间或相对天数（如 -30d）。" };
    return { ok: true, definition: { field: condition.field, op: condition.operator, value: condition.value } };
  }
  const values = numericValues(condition.value);
  if (!values) return { ok: false, message: "ID 条件使用正整数；多个 ID 以英文逗号分隔。" };
  return { ok: true, definition: { field: condition.field, op: condition.operator, value: condition.operator === "eq" ? values[0] : values } };
}

export function buildDefinition(condition: SegmentConditionDraft): BuildDefinitionResult { return buildCondition(condition); }

function requestHeaders(csrfToken: string, idempotencyKey: string): RequestInit {
  return { credentials: "same-origin", headers: { "X-CSRF-Token": csrfToken, "Idempotency-Key": idempotencyKey } };
}
function requestFor(draft: SegmentEditorDraft, definition: SegmentDefinition): CreateSegmentRequest {
  return { name: draft.name.trim(), definition, refresh_mode: draft.refreshMode, ...(draft.refreshMode === "scheduled" ? { refresh_cron: draft.refreshCron.trim() } : { refresh_cron: null }) };
}
export async function saveSegment(transport: SegmentTransport, existing: SegmentRecord | undefined, draft: SegmentEditorDraft, csrfToken: string, idempotencyKey: string): Promise<SegmentMutationResult> {
  const built = buildDefinition(draft.condition);
  if (!built.ok || draft.name.trim().length < 1 || draft.name.trim().length > 200 || (draft.refreshMode === "scheduled" && draft.refreshCron.trim().length < 1)) return { status: "invalid" };
  try {
    const request = requestFor(draft, built.definition);
    const response = existing ? await transport.update(existing.id, request, requestHeaders(csrfToken, idempotencyKey)) : await transport.create(request, requestHeaders(csrfToken, idempotencyKey));
    if (response.status !== (existing ? 200 : 201)) return { status: failure(response.status) };
    const segment = parseSegment(response.data);
    return !segment || (existing && segment.id !== existing.id) ? { status: "unavailable" } : { status: "saved", segment };
  } catch { return { status: "unavailable" }; }
}
export async function refreshSegment(transport: SegmentTransport, id: number, csrfToken: string, idempotencyKey: string): Promise<"accepted" | SegmentFailure> {
  try {
    const response = await transport.refresh(id, requestHeaders(csrfToken, idempotencyKey));
    if (response.status !== 202 || !record(response.data) || response.data.status !== "accepted" || response.data.segment_id !== id) return response.status === 202 ? "unavailable" : failure(response.status);
    return "accepted";
  } catch { return "unavailable"; }
}

export async function archiveSegment(
  transport: SegmentTransport,
  id: number,
  csrfToken: string,
  idempotencyKey: string,
): Promise<SegmentArchiveResult> {
  if (!transport.archive) return { status: "unavailable" };
  try {
    const response = await transport.archive(id, requestHeaders(csrfToken, idempotencyKey));
    if (response.status !== 200) return { status: failure(response.status) };
    const segment = parseSegment(response.data);
    return segment && segment.id === id && segment.lifecycleStatus === "archived" ? { status: "archived", segment } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}
