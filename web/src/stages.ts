import {
  createStage,
  listStages,
  renameStage,
  type CreateStageRequest,
  type RenameStageRequest,
} from "./api/generated/health";

export type StageRole = "admin" | "ops" | "sales";

export interface StageRecord {
  readonly id: number;
  readonly name: string;
  readonly sortOrder: number;
  readonly config: unknown;
}

export interface StageTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function loadGeneratedStages(
  options: RequestInit,
): Promise<StageTransportResponse> {
  return listStages(options);
}

async function createGeneratedStage(
  request: CreateStageRequest,
  options: RequestInit,
): Promise<StageTransportResponse> {
  return createStage(request, options);
}

async function renameGeneratedStage(
  stageID: number,
  request: RenameStageRequest,
  options: RequestInit,
): Promise<StageTransportResponse> {
  return renameStage(stageID, request, options);
}

export type StageTransport = {
  list: typeof loadGeneratedStages;
  create: typeof createGeneratedStage;
  rename: typeof renameGeneratedStage;
};

export const generatedStageTransport: StageTransport = {
  list: loadGeneratedStages,
  create: createGeneratedStage,
  rename: renameGeneratedStage,
};

export type StageLoadResult =
  | { readonly status: "loaded"; readonly items: readonly StageRecord[] }
  | { readonly status: "unauthenticated" }
  | { readonly status: "unavailable" };

export type StageMutationFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";

type FailedStageMutation = { readonly status: StageMutationFailure };

export type StageCreateResult =
  | { readonly status: "created"; readonly stage: StageRecord }
  | FailedStageMutation;

export type StageRenameResult =
  | { readonly status: "renamed"; readonly stage: StageRecord }
  | FailedStageMutation;

function plainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(
  value: Record<string, unknown>,
  allowed: readonly string[],
): boolean {
  const keys = Object.keys(value);
  return (
    keys.length === allowed.length && keys.every((key) => allowed.includes(key))
  );
}

export function parseStage(value: unknown): StageRecord | undefined {
  if (!plainRecord(value)) return undefined;
  if (!hasOnlyKeys(value, ["id", "name", "sort_order", "config"])) {
    return undefined;
  }
  if (
    typeof value.id !== "number" ||
    !Number.isSafeInteger(value.id) ||
    value.id < 1 ||
    typeof value.name !== "string" ||
    value.name.length === 0 ||
    typeof value.sort_order !== "number" ||
    !Number.isSafeInteger(value.sort_order)
  ) {
    return undefined;
  }

  return {
    id: value.id,
    name: value.name,
    sortOrder: value.sort_order,
    config: value.config,
  };
}

export function parseStageList(
  value: unknown,
): readonly StageRecord[] | undefined {
  if (!plainRecord(value) || !hasOnlyKeys(value, ["items"])) return undefined;
  if (!Array.isArray(value.items)) return undefined;

  const items: StageRecord[] = [];
  for (const item of value.items) {
    const parsed = parseStage(item);
    if (!parsed) return undefined;
    items.push(parsed);
  }
  return items;
}

export async function loadStages(
  transport: StageTransport,
): Promise<StageLoadResult> {
  try {
    const response = await transport.list({ credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status !== 200) return { status: "unavailable" };

    const items = parseStageList(response.data);
    return items ? { status: "loaded", items } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

function mutationFailure(status: number): StageMutationFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400 || status === 422) return "invalid";
  return "unavailable";
}

export async function submitStageCreate(
  transport: StageTransport,
  request: CreateStageRequest,
  csrfToken: string,
): Promise<StageCreateResult> {
  try {
    const response = await transport.create(request, {
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrfToken },
    });
    if (response.status !== 201) {
      return { status: mutationFailure(response.status) };
    }
    const stage = parseStage(response.data);
    return stage ? { status: "created", stage } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function submitStageRename(
  transport: StageTransport,
  stageID: number,
  request: RenameStageRequest,
  csrfToken: string,
): Promise<StageRenameResult> {
  try {
    const response = await transport.rename(stageID, request, {
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrfToken },
    });
    if (response.status !== 200) {
      return { status: mutationFailure(response.status) };
    }
    const stage = parseStage(response.data);
    if (!stage || stage.id !== stageID) return { status: "unavailable" };
    return { status: "renamed", stage };
  } catch {
    return { status: "unavailable" };
  }
}
