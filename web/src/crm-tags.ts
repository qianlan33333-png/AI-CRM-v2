/* eslint-disable no-unused-vars -- transport signatures name the generated contract parameters. */
export type CRMTagRole = "admin" | "ops" | "sales";

export interface CRMTagGroup { readonly id: number; readonly name: string; readonly sortOrder: number; }
export interface CRMTag { readonly id: number; readonly groupID: number; readonly groupName: string; readonly name: string; readonly sortOrder: number; }
export interface CRMTagCatalog { readonly groups: readonly CRMTagGroup[]; readonly tags: readonly CRMTag[]; }
export interface CRMTagResponse { readonly status: number; readonly data: unknown; }
export type CRMTagFailure = "unauthenticated" | "forbidden" | "not_found" | "conflict" | "invalid" | "unknown";
export type CRMTagTransport = {
  readonly list?: (options: RequestInit) => Promise<CRMTagResponse>;
  readonly createGroup?: (request: { readonly name: string }, options: RequestInit) => Promise<CRMTagResponse>;
  readonly updateGroup?: (id: number, request: { readonly name: string }, options: RequestInit) => Promise<CRMTagResponse>;
  readonly archiveGroup?: (id: number, options: RequestInit) => Promise<CRMTagResponse>;
  readonly reorderGroups?: (request: { readonly ids: readonly number[] }, options: RequestInit) => Promise<CRMTagResponse>;
  readonly createTag?: (request: { readonly group_id: number; readonly name: string }, options: RequestInit) => Promise<CRMTagResponse>;
  readonly updateTag?: (id: number, request: { readonly name: string }, options: RequestInit) => Promise<CRMTagResponse>;
  readonly archiveTag?: (id: number, options: RequestInit) => Promise<CRMTagResponse>;
  readonly reorderTags?: (request: { readonly ids: readonly number[] }, options: RequestInit) => Promise<CRMTagResponse>;
};

function record(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value); }
function positive(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function text(value: unknown): value is string { return typeof value === "string" && value.length > 0 && value.length <= 200 && value.trim() === value && !value.includes("\0"); }
function order(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 && value <= 2_147_483_647; }
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean { const actual = Object.keys(value); return actual.length === keys.length && actual.every((key) => keys.includes(key)); }

export function parseCRMTagCatalog(value: unknown): CRMTagCatalog | undefined {
  if (!record(value) || !exact(value, ["groups", "tags"]) || !Array.isArray(value.groups) || !Array.isArray(value.tags)) return undefined;
  const groups: CRMTagGroup[] = [];
  for (const item of value.groups) {
    if (!record(item) || !exact(item, ["id", "name", "sort_order"]) || !positive(item.id) || !text(item.name) || !order(item.sort_order)) return undefined;
    groups.push({ id: item.id, name: item.name, sortOrder: item.sort_order });
  }
  if (new Set(groups.map((group) => group.id)).size !== groups.length || groups.some((group, index) => index > 0 && groups[index - 1].sortOrder >= group.sortOrder)) return undefined;
  const groupNames = new Map(groups.map((group) => [group.id, group.name]));
  const tags: CRMTag[] = [];
  for (const item of value.tags) {
    if (!record(item) || !exact(item, ["id", "group_id", "group_name", "name", "sort_order"]) || !positive(item.id) || !positive(item.group_id) || !text(item.group_name) || !text(item.name) || !order(item.sort_order) || groupNames.get(item.group_id) !== item.group_name) return undefined;
    tags.push({ id: item.id, groupID: item.group_id, groupName: item.group_name, name: item.name, sortOrder: item.sort_order });
  }
  if (new Set(tags.map((tag) => tag.id)).size !== tags.length) return undefined;
  return { groups, tags };
}

export async function loadCRMTagCatalog(transport: CRMTagTransport): Promise<{ readonly status: "loaded"; readonly catalog: CRMTagCatalog } | { readonly status: CRMTagFailure }> {
  if (!transport.list) return { status: "unknown" };
  try {
    const response = await transport.list({ credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    const catalog = response.status === 200 ? parseCRMTagCatalog(response.data) : undefined;
    return catalog ? { status: "loaded", catalog } : { status: "unknown" };
  } catch { return { status: "unknown" }; }
}

export function crmTagIdempotencyKey(source: { readonly randomUUID: () => string } | undefined = globalThis.crypto): string | undefined {
  try { const value = source?.randomUUID(); return typeof value === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value) ? `crm-tag:${value}` : undefined; } catch { return undefined; }
}

export function crmTagHeaders(csrf: string, key: string): RequestInit { return { credentials: "same-origin", headers: { "X-CSRF-Token": csrf, "Idempotency-Key": key } }; }
export function crmTagFailure(status: number): CRMTagFailure { if (status === 401) return "unauthenticated"; if (status === 403) return "forbidden"; if (status === 404) return "not_found"; if (status === 409) return "conflict"; if (status === 400 || status === 422) return "invalid"; return "unknown"; }
