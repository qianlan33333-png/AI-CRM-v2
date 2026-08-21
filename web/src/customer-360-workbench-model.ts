import type { AuthPrincipal } from "./auth";
import {
  isCustomerGender,
  isSafeAvatarURL,
  loadCustomerDetail,
  parseCustomerDetailResponse,
  type CustomerDetailLoadResult,
  type CustomerDetailSnapshot,
  type CustomerDetailTransport,
  type CustomerProfile,
  type CustomerProfileUpdate,
  type CustomerTag,
} from "./customer-detail";
import { isStrictRFC3339Timestamp } from "./customer-context";
import {
  loadStages,
  type StageLoadResult,
  type StageRecord,
  type StageTransport,
} from "./stages";

export type Customer360WorkbenchRole = "admin" | "ops";

export type Customer360AccessResult =
  | {
      readonly status: "allowed";
      readonly principal: AuthPrincipal & {
        readonly role: Customer360WorkbenchRole;
      };
    }
  | { readonly status: "unauthenticated" }
  | { readonly status: "forbidden" };

export type Customer360PanelFailure = Exclude<
  CustomerDetailLoadResult,
  { readonly status: "loaded" }
>["status"];

export type Customer360PanelState<T> =
  | { readonly kind: "loading"; readonly previous?: T }
  | { readonly kind: "ready"; readonly value: T }
  | {
      readonly kind: "error";
      readonly failure: Customer360PanelFailure | "invalid";
      readonly previous?: T;
    };

export interface Customer360CoreProjection {
  readonly customer: CustomerProfile;
  readonly tags: readonly CustomerTag[];
}

export type Customer360CoreReadResult =
  | { readonly status: "loaded"; readonly core: Customer360CoreProjection }
  | {
      readonly status:
        | "unauthenticated"
        | "forbidden"
        | "not_found"
        | "invalid"
        | "unavailable";
    };

export type Customer360MutationKind =
  | "profile"
  | "stage"
  | "tag-add"
  | "tag-remove";

export type Customer360MutationAction =
  | {
      readonly kind: "profile";
      readonly update: CustomerProfileUpdate;
    }
  | { readonly kind: "stage"; readonly stageID: number | null }
  | { readonly kind: "tag-add"; readonly tagID: number }
  | { readonly kind: "tag-remove"; readonly tagID: number };

export type Customer360KnownMutationFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "invalid"
  | "unavailable";

export type Customer360MutationResult =
  | {
      readonly status: "confirmed";
      readonly core: Customer360CoreProjection;
      readonly idempotencyKey: string;
    }
  | {
      readonly status: "rejected";
      readonly failure: Customer360KnownMutationFailure;
      readonly idempotencyKey: string;
    }
  | {
      readonly status: "conflict";
      readonly reason:
        | "expected_version_changed"
        | "server_conflict"
        | "readback_mismatch";
      readonly core?: Customer360CoreProjection;
      readonly idempotencyKey: string;
    }
  | {
      readonly status: "outcome_unknown";
      readonly reason:
        | "write_transport_failed"
        | "unexpected_write_status"
        | "readback_failed"
        | "readback_invalid"
        | "stale_readback";
      readonly idempotencyKey: string;
    };

export type Customer360MutationLockReason = "conflict" | "outcome_unknown";

export interface Customer360RequestToken {
  readonly customerID: number;
  readonly generation: number;
}

const CSRF_TOKEN_PATTERN = /^[A-Za-z0-9_-]{43}$/;
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const IDEMPOTENCY_KEY_PATTERN =
  /^customer-360:(?:profile|stage|tag-add|tag-remove):[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function plainRecord(value: unknown): value is Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function exactKeys(
  value: Record<string, unknown>,
  allowed: readonly string[],
): boolean {
  return Object.keys(value).every((key) => allowed.includes(key));
}

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function optionalPositiveInteger(
  value: unknown,
): value is number | null | undefined {
  return value === undefined || value === null || positiveInteger(value);
}

export function resolveCustomer360Access(
  principal: unknown,
): Customer360AccessResult {
  if (principal === undefined || principal === null) {
    return { status: "unauthenticated" };
  }
  if (
    !plainRecord(principal) ||
    !exactKeys(principal, ["adminUserID", "role", "staffID"]) ||
    !positiveInteger(principal.adminUserID) ||
    !optionalPositiveInteger(principal.staffID)
  ) {
    return { status: "forbidden" };
  }
  if (principal.role !== "admin" && principal.role !== "ops") {
    return { status: "forbidden" };
  }
  return {
    status: "allowed",
    principal: {
      adminUserID: principal.adminUserID,
      role: principal.role,
      ...(typeof principal.staffID === "number"
        ? { staffID: principal.staffID }
        : {}),
    },
  };
}

export function validCustomer360CustomerID(value: unknown): value is number {
  return positiveInteger(value);
}

export class Customer360LatestRequestGate {
  private generation = 0;
  private customerID?: number;

  begin(customerID: number): Customer360RequestToken | undefined {
    if (!validCustomer360CustomerID(customerID)) return undefined;
    this.generation += 1;
    this.customerID = customerID;
    return { customerID, generation: this.generation };
  }

  invalidate(): void {
    this.generation += 1;
    this.customerID = undefined;
  }

  isCurrent(token: Customer360RequestToken): boolean {
    return (
      token.customerID === this.customerID &&
      token.generation === this.generation
    );
  }
}

export class Customer360MutationGuard {
  private inFlight = false;
  private locked?: Customer360MutationLockReason;

  begin(): "started" | "busy" | "locked" {
    if (this.locked) return "locked";
    if (this.inFlight) return "busy";
    this.inFlight = true;
    return "started";
  }

  finishKnown(): void {
    this.inFlight = false;
  }

  lock(reason: Customer360MutationLockReason): void {
    this.inFlight = false;
    this.locked = reason;
  }

  reset(): void {
    this.inFlight = false;
    this.locked = undefined;
  }

  state(): {
    readonly inFlight: boolean;
    readonly locked?: Customer360MutationLockReason;
  } {
    return {
      inFlight: this.inFlight,
      ...(this.locked ? { locked: this.locked } : {}),
    };
  }
}

export async function loadCustomer360Core(
  transport: CustomerDetailTransport,
  customerID: number,
): Promise<CustomerDetailLoadResult> {
  if (!validCustomer360CustomerID(customerID)) {
    return { status: "unavailable" };
  }
  return loadCustomerDetail(transport, customerID);
}

export async function loadCustomer360StageCatalog(
  transport: StageTransport,
): Promise<StageLoadResult> {
  return loadStages(transport);
}

function mapCoreReadFailure(status: number): Customer360CoreReadResult {
  if (status === 401) return { status: "unauthenticated" };
  if (status === 403) return { status: "forbidden" };
  if (status === 404) return { status: "not_found" };
  return { status: "unavailable" };
}

export async function readCustomer360CoreProjection(
  transport: CustomerDetailTransport,
  customerID: number,
): Promise<Customer360CoreReadResult> {
  if (!validCustomer360CustomerID(customerID)) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.get(customerID, {
      credentials: "same-origin",
    });
    if (response.status !== 200) return mapCoreReadFailure(response.status);
    const parsed = parseCustomerDetailResponse(response.data, customerID);
    return parsed
      ? { status: "loaded", core: parsed }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export function newCustomer360IdempotencyKey(
  kind: Customer360MutationKind,
  source: { readonly randomUUID: () => string } | undefined =
    globalThis.crypto,
): string | undefined {
  try {
    const uuid = source?.randomUUID();
    if (typeof uuid !== "string" || !UUID_PATTERN.test(uuid)) {
      return undefined;
    }
    return `customer-360:${kind}:${uuid}`;
  } catch {
    return undefined;
  }
}

function validProfileUpdate(value: unknown): value is CustomerProfileUpdate {
  if (
    !plainRecord(value) ||
    !exactKeys(value, [
      "name",
      "avatarURL",
      "gender",
      "ownerStaffID",
      "channelID",
    ]) ||
    typeof value.name !== "string" ||
    value.name.trim() === "" ||
    !(value.avatarURL === null || isSafeAvatarURL(value.avatarURL)) ||
    !(value.gender === null || isCustomerGender(value.gender)) ||
    !(value.ownerStaffID === null || positiveInteger(value.ownerStaffID)) ||
    !(value.channelID === null || positiveInteger(value.channelID))
  ) {
    return false;
  }
  return true;
}

function validMutationAction(value: unknown): value is Customer360MutationAction {
  if (!plainRecord(value) || typeof value.kind !== "string") return false;
  switch (value.kind) {
    case "profile":
      return (
        exactKeys(value, ["kind", "update"]) && validProfileUpdate(value.update)
      );
    case "stage":
      return (
        exactKeys(value, ["kind", "stageID"]) &&
        (value.stageID === null || positiveInteger(value.stageID))
      );
    case "tag-add":
    case "tag-remove":
      return exactKeys(value, ["kind", "tagID"]) && positiveInteger(value.tagID);
    default:
      return false;
  }
}

function expectedWriteStatus(action: Customer360MutationAction): number {
  return action.kind === "tag-add" || action.kind === "tag-remove" ? 204 : 200;
}

function mutationOptions(
  csrfToken: string,
  idempotencyKey: string,
  expectedVersion: string,
): RequestInit {
  return {
    credentials: "same-origin",
    headers: {
      "X-CSRF-Token": csrfToken,
      "Idempotency-Key": idempotencyKey,
      "X-Expected-Version": expectedVersion,
    },
  };
}

async function performMutationRequest(
  transport: CustomerDetailTransport,
  customerID: number,
  action: Customer360MutationAction,
  options: RequestInit,
): Promise<{ readonly status: number }> {
  switch (action.kind) {
    case "profile":
      return transport.update(
        customerID,
        {
          name: action.update.name,
          avatar_url: action.update.avatarURL,
          gender: action.update.gender,
          owner_staff_id: action.update.ownerStaffID,
          channel_id: action.update.channelID,
        },
        options,
      );
    case "stage":
      return transport.setStage(
        customerID,
        { stage_id: action.stageID },
        options,
      );
    case "tag-add":
      return transport.addTag(customerID, action.tagID, options);
    case "tag-remove":
      return transport.removeTag(customerID, action.tagID, options);
  }
}

function knownWriteFailure(
  status: number,
): Customer360KnownMutationFailure | "conflict" | undefined {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400 || status === 422) return "invalid";
  return undefined;
}

function optionalValueEquals<T>(
  current: T | undefined,
  expected: T | null,
): boolean {
  return expected === null ? current === undefined : current === expected;
}

function actionMatchesCore(
  action: Customer360MutationAction,
  core: Customer360CoreProjection,
): boolean {
  switch (action.kind) {
    case "profile":
      return (
        core.customer.name === action.update.name &&
        optionalValueEquals(core.customer.avatarURL, action.update.avatarURL) &&
        optionalValueEquals(core.customer.gender, action.update.gender) &&
        optionalValueEquals(
          core.customer.ownerStaffID,
          action.update.ownerStaffID,
        ) &&
        optionalValueEquals(core.customer.channelID, action.update.channelID)
      );
    case "stage":
      return optionalValueEquals(core.customer.stageID, action.stageID);
    case "tag-add":
      return core.tags.some((tag) => tag.id === action.tagID);
    case "tag-remove":
      return core.tags.every((tag) => tag.id !== action.tagID);
  }
}

function rejected(
  failure: Customer360KnownMutationFailure,
  idempotencyKey: string,
): Customer360MutationResult {
  return { status: "rejected", failure, idempotencyKey };
}

export async function executeCustomer360Mutation(
  transport: CustomerDetailTransport,
  customerID: number,
  expectedVersion: string,
  action: Customer360MutationAction,
  csrfToken: string,
  idempotencyKey: string,
): Promise<Customer360MutationResult> {
  if (
    !validCustomer360CustomerID(customerID) ||
    !isStrictRFC3339Timestamp(expectedVersion) ||
    !validMutationAction(action) ||
    !CSRF_TOKEN_PATTERN.test(csrfToken) ||
    !IDEMPOTENCY_KEY_PATTERN.test(idempotencyKey) ||
    !idempotencyKey.startsWith(`customer-360:${action.kind}:`)
  ) {
    return rejected("invalid", idempotencyKey);
  }

  const preflight = await readCustomer360CoreProjection(transport, customerID);
  if (preflight.status !== "loaded") {
    return rejected(preflight.status, idempotencyKey);
  }
  if (preflight.core.customer.updatedAt !== expectedVersion) {
    return {
      status: "conflict",
      reason: "expected_version_changed",
      core: preflight.core,
      idempotencyKey,
    };
  }

  let response: { readonly status: number };
  try {
    response = await performMutationRequest(
      transport,
      customerID,
      action,
      mutationOptions(csrfToken, idempotencyKey, expectedVersion),
    );
  } catch {
    return {
      status: "outcome_unknown",
      reason: "write_transport_failed",
      idempotencyKey,
    };
  }

  if (response.status !== expectedWriteStatus(action)) {
    const failure = knownWriteFailure(response.status);
    if (failure === "conflict") {
      return {
        status: "conflict",
        reason: "server_conflict",
        idempotencyKey,
      };
    }
    if (failure) return rejected(failure, idempotencyKey);
    return {
      status: "outcome_unknown",
      reason: "unexpected_write_status",
      idempotencyKey,
    };
  }

  let readback: Customer360CoreReadResult;
  try {
    readback = await readCustomer360CoreProjection(transport, customerID);
  } catch {
    return {
      status: "outcome_unknown",
      reason: "readback_failed",
      idempotencyKey,
    };
  }
  if (readback.status !== "loaded") {
    return {
      status: "outcome_unknown",
      reason:
        readback.status === "invalid" ? "readback_invalid" : "readback_failed",
      idempotencyKey,
    };
  }
  if (
    Date.parse(readback.core.customer.updatedAt) < Date.parse(expectedVersion)
  ) {
    return {
      status: "outcome_unknown",
      reason: "stale_readback",
      idempotencyKey,
    };
  }
  if (!actionMatchesCore(action, readback.core)) {
    return {
      status: "conflict",
      reason: "readback_mismatch",
      core: readback.core,
      idempotencyKey,
    };
  }
  return {
    status: "confirmed",
    core: readback.core,
    idempotencyKey,
  };
}

export function mergeConfirmedCustomer360Core(
  snapshot: CustomerDetailSnapshot,
  core: Customer360CoreProjection,
): CustomerDetailSnapshot | undefined {
  if (snapshot.customer.id !== core.customer.id) return undefined;
  return {
    ...snapshot,
    customer: core.customer,
    tags: core.tags,
  };
}

export function stageName(
  stages: readonly StageRecord[],
  stageID: number | undefined,
): string {
  if (stageID === undefined) return "未设置";
  const stage = stages.find((item) => item.id === stageID);
  return stage ? stage.name : `已归档或不可见阶段 #${stageID}`;
}
