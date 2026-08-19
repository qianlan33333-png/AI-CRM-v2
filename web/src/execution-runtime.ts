import { getLegacyExecutionRuntime } from "./api/generated/health";

export type ExecutionRuntimeRole = "admin" | "ops" | "sales";

export interface ExecutionRuntimeControl {
  readonly name: string;
  readonly state: string;
  readonly observedAt: string;
}

export interface ExecutionRuntimeObservation {
  readonly source: string;
  readonly queue: string;
  readonly status: string;
  readonly attempt: number;
  readonly observedAt: string;
}

export interface ExecutionRuntimeSnapshot {
  readonly available: boolean;
  readonly control: ExecutionRuntimeControl | null;
  readonly observations: readonly ExecutionRuntimeObservation[];
  readonly truncated: boolean;
}

export interface ExecutionRuntimeTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedRead(
  options: RequestInit,
): Promise<ExecutionRuntimeTransportResponse> {
  return getLegacyExecutionRuntime(options);
}

export interface ExecutionRuntimeTransport {
  readonly read: typeof generatedRead;
}

export const generatedExecutionRuntimeTransport: ExecutionRuntimeTransport = {
  read: generatedRead,
};

export type ExecutionRuntimeFailure =
  "unauthenticated" | "forbidden" | "invalid" | "unavailable";

export type ExecutionRuntimeResult =
  | { readonly status: "loaded"; readonly snapshot: ExecutionRuntimeSnapshot }
  | { readonly status: ExecutionRuntimeFailure };

function record(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype ||
      Object.getPrototypeOf(value) === null)
  );
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

function boundedText(value: unknown): value is string {
  return typeof value === "string" && Array.from(value).length <= 1024;
}

function timestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) ||
    !Number.isFinite(Date.parse(value))
  ) {
    return false;
  }
  const parts = value.match(
    /^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})/,
  );
  if (!parts) return false;
  const [year, month, day, hour, minute, second] = parts.slice(1).map(Number);
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, 0);
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day &&
    date.getUTCHours() === hour &&
    date.getUTCMinutes() === minute &&
    date.getUTCSeconds() === second
  );
}

// Validate diagnostic maps without returning them. The safe UI must never
// retain details, secret references, URLs, messages, or external identities.
function discardedTextMap(value: unknown): boolean {
  return (
    record(value) &&
    Object.entries(value).every(
      ([key, item]) => boundedText(key) && boundedText(item),
    )
  );
}

function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function parseControl(value: unknown): ExecutionRuntimeControl | undefined {
  if (
    !record(value) ||
    !exact(value, ["name", "state", "details", "observed_at"]) ||
    !boundedText(value.name) ||
    !boundedText(value.state) ||
    !discardedTextMap(value.details) ||
    !timestamp(value.observed_at)
  ) {
    return undefined;
  }
  return {
    name: value.name,
    state: value.state,
    observedAt: value.observed_at,
  };
}

function parseObservation(
  value: unknown,
): ExecutionRuntimeObservation | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "source",
      "queue",
      "status",
      "attempt",
      "status_url",
      "details",
      "observed_at",
    ]) ||
    !boundedText(value.source) ||
    !boundedText(value.queue) ||
    !boundedText(value.status) ||
    !nonnegative(value.attempt) ||
    !boundedText(value.status_url) ||
    !discardedTextMap(value.details) ||
    !timestamp(value.observed_at)
  ) {
    return undefined;
  }
  return {
    source: value.source,
    queue: value.queue,
    status: value.status,
    attempt: value.attempt,
    observedAt: value.observed_at,
  };
}

export function parseExecutionRuntime(
  value: unknown,
): ExecutionRuntimeSnapshot | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "ok",
      "control",
      "observations",
      "truncated",
      "observed_at",
      "observed_only",
      "real_external_call_executed",
    ]) ||
    typeof value.ok !== "boolean" ||
    !Array.isArray(value.observations) ||
    value.observations.length > 1024 ||
    typeof value.truncated !== "boolean" ||
    !timestamp(value.observed_at) ||
    value.observed_only !== true ||
    value.real_external_call_executed !== false
  ) {
    return undefined;
  }
  const control = value.control === null ? null : parseControl(value.control);
  if (control === undefined || value.ok !== (control !== null))
    return undefined;
  const observations = value.observations.map(parseObservation);
  if (observations.some((observation) => observation === undefined))
    return undefined;
  return {
    available: value.ok,
    control,
    observations: observations as ExecutionRuntimeObservation[],
    truncated: value.truncated,
  };
}

export async function loadExecutionRuntime(
  transport: ExecutionRuntimeTransport = generatedExecutionRuntimeTransport,
): Promise<ExecutionRuntimeResult> {
  try {
    const response = await transport.read({ credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status !== 200) return { status: "unavailable" };
    const snapshot = parseExecutionRuntime(response.data);
    return snapshot ? { status: "loaded", snapshot } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}
