/* eslint-disable no-unused-vars -- transport signatures deliberately name their contract arguments. */
import {
  cancelLegacyOutboundJob,
  getLegacyOutboundJobReconciliation,
  listLegacyOutboundJobs,
  type ListLegacyOutboundJobsStatus,
} from "./api/generated/health";
export type OutboundOperationsRole = "admin" | "ops" | "sales";
export const OUTBOUND_OPERATIONS_PAGE_SIZE = 50;
const MAX_OFFSET = 1_000_000;
const INT64_MAX = "9223372036854775807";
const TASK_STATES = new Set(["pending", "sending", "sent", "retryable_failed", "final_failed", "outcome_unknown", "cancelled"]);
const ATTEMPT_STATES = new Set(["reserved", "dispatching", "succeeded", "retryable_failed", "final_failed", "outcome_unknown"]);
const JOB_KINDS = new Set(["outbound_enqueue_one", "outbound_enqueue_batch_task"]);

export interface OutboundTask {
  readonly taskID: string;
  readonly businessID?: string;
  readonly status: string;
  readonly attemptCount: number;
  readonly generation: number;
  readonly queueKind: string;
  readonly createdAt: string;
  readonly updatedAt: string;
}
export interface OutboundAttempt {
  readonly attemptID: string;
  readonly generation: number;
  readonly attempt: number;
  readonly maxAttempts: number;
  readonly state: string;
  readonly dispatchStartedAt?: string;
  readonly completedAt?: string;
}
export interface OutboundControlReceipt {
  readonly receiptID: string;
  readonly taskID: string;
  readonly operation: "cancel" | "manual_retry";
  readonly state: "completed";
  readonly generation: number;
  readonly queueKind: string;
  readonly taskStatus: string;
  readonly completedAt: string;
}
export interface OutboundTaskPage { readonly items: readonly OutboundTask[]; readonly offset: number; readonly hasMore: boolean; }
export interface OutboundReconciliation { readonly task: OutboundTask; readonly attempts: readonly OutboundAttempt[]; readonly receipts: readonly OutboundControlReceipt[]; }
export interface OutboundOperationsResponse { readonly status: number; readonly data: unknown; }
export interface OutboundOperationsTransport {
  readonly list: (params: { readonly status?: string; readonly businessID?: string; readonly offset: number }, options: RequestInit) => Promise<OutboundOperationsResponse>;
  readonly reconciliation: (taskID: string, options: RequestInit) => Promise<OutboundOperationsResponse>;
  /** Optional only for isolated read-only test fixtures; the default transport is generated and always supplies it. */
  readonly cancel?: (taskID: string, options: RequestInit) => Promise<OutboundOperationsResponse>;
}

function generatedTaskID(taskID: string): number | undefined {
  const parsed = Number(taskID);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}

/** Generated OpenAPI clients remain the only browser transport.  IDs beyond JavaScript's
 * safe-integer range fail closed rather than being rounded into another local task. */
export const generatedOutboundOperationsTransport: OutboundOperationsTransport = {
  list: async (params, options) => listLegacyOutboundJobs({
    status: params.status as ListLegacyOutboundJobsStatus | undefined,
    business_id: params.businessID === undefined ? undefined : generatedTaskID(params.businessID),
    limit: OUTBOUND_OPERATIONS_PAGE_SIZE,
    offset: params.offset,
  }, options),
  reconciliation: async (taskID, options) => {
    const id = generatedTaskID(taskID);
    return id === undefined ? { status: 400, data: {} } : getLegacyOutboundJobReconciliation(id, options);
  },
  cancel: async (taskID, options) => {
    const id = generatedTaskID(taskID);
    return id === undefined ? { status: 400, data: {} } : cancelLegacyOutboundJob(id, options);
  },
};
export type OutboundOperationsFailure = "unauthenticated" | "forbidden" | "invalid" | "unavailable";
export type OutboundOperationsResult<T> = { readonly status: "loaded"; readonly value: T } | { readonly status: OutboundOperationsFailure };
export type OutboundCancelResult =
  | { readonly status: "cancelled"; readonly receipt: OutboundControlReceipt }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" | "conflict" | "unknown" | "unavailable" };

function plain(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value) && (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null); }
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean { const actual = Object.keys(value); return actual.length === keys.length && actual.every((key) => keys.includes(key)); }
function optionalExact(value: Record<string, unknown>, required: readonly string[], optional: readonly string[]): boolean { const actual = Object.keys(value); return actual.length >= required.length && actual.every((key) => required.includes(key) || optional.includes(key)) && required.every((key) => Object.hasOwn(value, key)); }
function positiveID(value: unknown): value is string { return typeof value === "number" ? Number.isSafeInteger(value) && value > 0 : typeof value === "string" && /^[1-9]\d*$/.test(value) && (value.length < INT64_MAX.length || value.length === INT64_MAX.length && value <= INT64_MAX); }
function positive(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function nonnegative(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 0; }
function time(value: unknown): value is string { if (typeof value !== "string" || !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/.test(value) || !Number.isFinite(Date.parse(value))) return false; const match = value.match(/^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)/); if (!match) return false; const [year, month, day, hour, minute, second] = match.slice(1).map(Number); const date = new Date(0); date.setUTCFullYear(year, month - 1, day); date.setUTCHours(hour, minute, second, 0); return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day && date.getUTCHours() === hour && date.getUTCMinutes() === minute && date.getUTCSeconds() === second; }
function id(value: string | number): string { return String(value); }
function nullableTime(value: unknown): string | undefined { return value === undefined ? undefined : time(value) ? value : undefined; }
function safeRawEqual(left: unknown, right: unknown): boolean { return JSON.stringify(left) === JSON.stringify(right); }

function parseFailure(value: unknown): boolean { return plain(value) && exact(value, ["kind", "code"]) && typeof value.kind === "string" && value.kind.length > 0 && value.kind.length <= 64 && typeof value.code === "string" && value.code.length <= 256; }
function parseProviderReceipt(value: unknown): boolean { return plain(value) && exact(value, ["message_id", "confirmed_at"]) && typeof value.message_id === "string" && value.message_id.length > 0 && value.message_id.length <= 512 && time(value.confirmed_at); }
function parseQueueJob(value: unknown): { generation: number; kind: string } | undefined { if (!plain(value) || !exact(value, ["river_job_id", "generation", "kind"]) || !positiveID(value.river_job_id) || !positive(value.generation) || typeof value.kind !== "string" || !JOB_KINDS.has(value.kind)) return undefined; return { generation: value.generation, kind: value.kind }; }

/** Parses the complete wire object, but deliberately projects no customer, owner, provider or failure data into the browser model. */
export function parseOutboundTask(value: unknown): OutboundTask | undefined {
  if (!plain(value) || !optionalExact(value, ["job_id", "task_id", "customer_id", "status", "attempt_count", "delivery_proven", "queue_job", "created_at", "status_updated_at"], ["owner_staff_id", "business_id", "batch_chunk_index", "failure", "provider_receipt"]) || !positiveID(value.job_id) || !positiveID(value.task_id) || id(value.job_id) !== id(value.task_id) || !positiveID(value.customer_id) || (value.owner_staff_id !== undefined && !positiveID(value.owner_staff_id)) || (value.business_id !== undefined && !positiveID(value.business_id)) || (value.batch_chunk_index !== undefined && !nonnegative(value.batch_chunk_index)) || typeof value.status !== "string" || !TASK_STATES.has(value.status) || !nonnegative(value.attempt_count) || typeof value.delivery_proven !== "boolean" || !time(value.created_at) || !time(value.status_updated_at)) return undefined;
  const queue = parseQueueJob(value.queue_job); if (!queue) return undefined;
  if ((value.failure !== undefined && !parseFailure(value.failure)) || (value.provider_receipt !== undefined && !parseProviderReceipt(value.provider_receipt))) return undefined;
  if (value.status === "sent" ? value.delivery_proven !== true || value.provider_receipt === undefined : value.delivery_proven !== false || value.provider_receipt !== undefined) return undefined;
  return { taskID: id(value.task_id), businessID: value.business_id === undefined ? undefined : id(value.business_id), status: value.status, attemptCount: value.attempt_count, generation: queue.generation, queueKind: queue.kind, createdAt: value.created_at, updatedAt: value.status_updated_at };
}
export function parseOutboundAttempt(value: unknown): OutboundAttempt | undefined {
  if (!plain(value) || !optionalExact(value, ["attempt_id", "history_id", "generation", "river_job_id", "attempt", "max_attempts", "state"], ["failure", "provider_receipt", "dispatch_started_at", "completed_at"]) || !positiveID(value.attempt_id) || !positiveID(value.history_id) || !positive(value.generation) || !positiveID(value.river_job_id) || !positive(value.attempt) || !positive(value.max_attempts) || value.attempt > value.max_attempts || typeof value.state !== "string" || !ATTEMPT_STATES.has(value.state) || (value.failure !== undefined && !parseFailure(value.failure)) || (value.provider_receipt !== undefined && !parseProviderReceipt(value.provider_receipt)) || (value.dispatch_started_at !== undefined && !time(value.dispatch_started_at)) || (value.completed_at !== undefined && !time(value.completed_at))) return undefined;
  if (value.state === "succeeded" ? value.provider_receipt === undefined || value.completed_at === undefined : value.provider_receipt !== undefined) return undefined;
  return { attemptID: id(value.attempt_id), generation: value.generation, attempt: value.attempt, maxAttempts: value.max_attempts, state: value.state, dispatchStartedAt: nullableTime(value.dispatch_started_at), completedAt: nullableTime(value.completed_at) };
}
export function parseOutboundControlReceipt(value: unknown, taskID: string): OutboundControlReceipt | undefined {
  if (!plain(value) || !exact(value, ["receipt_id", "task_id", "operation", "state", "generation", "river_job_id", "job_kind", "event_id", "task_status", "completed_at"]) || !positiveID(value.receipt_id) || !positiveID(value.task_id) || id(value.task_id) !== taskID || (value.operation !== "cancel" && value.operation !== "manual_retry") || value.state !== "completed" || !positive(value.generation) || !positiveID(value.river_job_id) || typeof value.job_kind !== "string" || !JOB_KINDS.has(value.job_kind) || !positiveID(value.event_id) || typeof value.task_status !== "string" || !TASK_STATES.has(value.task_status) || !time(value.completed_at)) return undefined;
  if ((value.operation === "cancel" && value.task_status !== "cancelled") || (value.operation === "manual_retry" && value.task_status !== "pending")) return undefined;
  return { receiptID: id(value.receipt_id), taskID, operation: value.operation, state: "completed", generation: value.generation, queueKind: value.job_kind, taskStatus: value.task_status, completedAt: value.completed_at };
}
function source(value: unknown): boolean { return value === "v2_outbound_service"; }
function listPage(value: unknown, offset: number): OutboundTaskPage | undefined {
  if (!plain(value) || !exact(value, ["ok", "jobs", "items", "count", "has_more", "limit", "offset", "source_status", "fallback_used"]) || value.ok !== true || !Array.isArray(value.jobs) || !Array.isArray(value.items) || value.count !== value.items.length || value.limit !== OUTBOUND_OPERATIONS_PAGE_SIZE || value.offset !== offset || typeof value.has_more !== "boolean" || value.items.length > OUTBOUND_OPERATIONS_PAGE_SIZE || (value.has_more && value.items.length !== OUTBOUND_OPERATIONS_PAGE_SIZE) || !source(value.source_status) || value.fallback_used !== false || !safeRawEqual(value.jobs, value.items)) return undefined;
  const items = value.items.map(parseOutboundTask); return items.includes(undefined) || new Set((items as readonly OutboundTask[]).map((item) => item.taskID)).size !== items.length ? undefined : { items: items as readonly OutboundTask[], offset, hasMore: value.has_more };
}
export function sameSafeTask(left: OutboundTask, right: OutboundTask): boolean { return left.taskID === right.taskID && left.businessID === right.businessID && left.status === right.status && left.attemptCount === right.attemptCount && left.generation === right.generation && left.queueKind === right.queueKind && left.createdAt === right.createdAt && left.updatedAt === right.updatedAt; }
function reconciliation(value: unknown, taskID: string): OutboundReconciliation | undefined { if (!plain(value) || !exact(value, ["ok", "job", "attempts", "control_receipts", "source_status", "fallback_used"]) || value.ok !== true || !Array.isArray(value.attempts) || !Array.isArray(value.control_receipts) || !source(value.source_status) || value.fallback_used !== false) return undefined; const task = parseOutboundTask(value.job); const attempts = value.attempts.map(parseOutboundAttempt); const receipts = value.control_receipts.map((item) => parseOutboundControlReceipt(item, taskID)); if (!task || task.taskID !== taskID || attempts.includes(undefined) || receipts.includes(undefined) || new Set((attempts as readonly OutboundAttempt[]).map((item) => item.attemptID)).size !== attempts.length || new Set((receipts as readonly OutboundControlReceipt[]).map((item) => item.receiptID)).size !== receipts.length) return undefined; return { task, attempts: attempts as readonly OutboundAttempt[], receipts: receipts as readonly OutboundControlReceipt[] }; }
function failure(status: number): OutboundOperationsFailure { if (status === 401) return "unauthenticated"; if (status === 403) return "forbidden"; if (status === 400 || status === 404 || status === 422) return "invalid"; return "unavailable"; }
export async function loadOutboundTasks(transport: OutboundOperationsTransport, params: { readonly status?: string; readonly businessID?: string; readonly offset?: number } = {}): Promise<OutboundOperationsResult<OutboundTaskPage>> { const offset = params.offset ?? 0; if (!nonnegative(offset) || offset > MAX_OFFSET || offset % OUTBOUND_OPERATIONS_PAGE_SIZE !== 0 || (params.status !== undefined && !TASK_STATES.has(params.status)) || (params.businessID !== undefined && !positiveID(params.businessID))) return { status: "invalid" }; try { const response = await transport.list({ status: params.status, businessID: params.businessID, offset }, { credentials: "same-origin" }); if (response.status !== 200) return { status: failure(response.status) }; const page = listPage(response.data, offset); return page ? { status: "loaded", value: page } : { status: "invalid" }; } catch { return { status: "unavailable" }; } }
export async function loadOutboundReconciliation(transport: OutboundOperationsTransport, taskID: string): Promise<OutboundOperationsResult<OutboundReconciliation>> { if (!positiveID(taskID)) return { status: "invalid" }; try { const response = await transport.reconciliation(taskID, { credentials: "same-origin" }); if (response.status !== 200) return { status: failure(response.status) }; const value = reconciliation(response.data, taskID); return value ? { status: "loaded", value } : { status: "invalid" }; } catch { return { status: "unavailable" }; } }

function validCSRF(value: string): boolean { return /^[A-Za-z0-9_-]{43}$/.test(value); }
function validIdempotencyKey(value: string): boolean { return /^[A-Za-z0-9:_-]{16,128}$/.test(value) && value === value.trim(); }
function cancelReceipt(value: unknown, taskID: string): OutboundControlReceipt | undefined {
  if (!plain(value) || !exact(value, ["ok", "control_receipt", "source_status", "fallback_used"]) || value.ok !== true || value.source_status !== "v2_outbound_cancel_service" || value.fallback_used !== false) return undefined;
  const receipt = parseOutboundControlReceipt(value.control_receipt, taskID);
  return receipt?.operation === "cancel" && receipt.taskStatus === "cancelled" ? receipt : undefined;
}

/** This is deliberately a pre-provider control: a valid 202 only proves the local
 * cancellation receipt. The caller must read the same task back before changing UI state. */
export async function cancelPendingOutboundTask(transport: OutboundOperationsTransport, task: OutboundTask, csrf: string, idempotencyKey: string): Promise<OutboundCancelResult> {
  if (task.status !== "pending" || !positiveID(task.taskID) || !validCSRF(csrf) || !validIdempotencyKey(idempotencyKey) || !transport.cancel) return { status: "invalid" };
  try {
    const response = await transport.cancel(task.taskID, { credentials: "same-origin", headers: { "X-CSRF-Token": csrf, "Idempotency-Key": idempotencyKey } });
    if (response.status === 202) { const receipt = cancelReceipt(response.data, task.taskID); return receipt ? { status: "cancelled", receipt } : { status: "unknown" }; }
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 400 || response.status === 404 || response.status === 422) return { status: "invalid" };
    if (response.status === 409) return { status: "conflict" };
    return { status: "unknown" };
  } catch { return { status: "unknown" }; }
}

export function newOutboundCancelIdempotencyKey(source: { readonly randomUUID: () => string } | undefined = globalThis.crypto): string | undefined {
  try { const uuid = source?.randomUUID(); return typeof uuid === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid) ? `outbound-cancel:${uuid}` : undefined; } catch { return undefined; }
}

export function confirmsCancelledOutboundTask(value: OutboundReconciliation, receipt: OutboundControlReceipt): boolean {
  return value.task.status === "cancelled" && value.task.taskID === receipt.taskID && value.task.generation === receipt.generation && value.task.queueKind === receipt.queueKind && value.receipts.some((item) => item.receiptID === receipt.receiptID && item.operation === "cancel" && item.state === "completed" && item.taskStatus === "cancelled" && item.generation === receipt.generation && item.queueKind === receipt.queueKind && item.completedAt === receipt.completedAt);
}
