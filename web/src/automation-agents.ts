/* eslint-disable no-unused-vars -- transport tuple signatures document generated integration seams. */
import {
  activateLegacyAutomationAgent,
  archiveLegacyAutomationAgent,
  copyLegacyAutomationAgent,
  createLegacyAutomationAgent,
  getLegacyAutomationAgent,
  listLegacyAutomationAgents,
  pauseLegacyAutomationAgent,
  publishLegacyAutomationAgent,
  saveLegacyAutomationAgentFixedContent,
  updateLegacyAutomationAgent,
} from "./api/generated/health";

export type AutomationAgentsRole = "admin" | "ops" | "sales";
export type AutomationAgentType = "agent" | "fixed_script";
export type AutomationAgentStatus = "active" | "paused";

export interface AutomationAgentMaterialSummary {
  readonly imageCount: number;
  readonly miniprogramCount: number;
  readonly attachmentCount: number;
  readonly groupInviteCount: number;
}

export interface AutomationAgentSummary {
  readonly id: number;
  readonly type: AutomationAgentType;
  readonly typeLabel: "Agent 机器人" | "固定话术";
  readonly code: string;
  readonly name: string;
  readonly status: AutomationAgentStatus;
  readonly updatedAt: string;
  readonly materialSummary: AutomationAgentMaterialSummary;
}

export interface AutomationAgentsSnapshot {
  readonly items: readonly AutomationAgentSummary[];
  readonly total: number;
}

export interface AutomationAgentsTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function generatedRead(options: RequestInit): Promise<AutomationAgentsTransportResponse> {
  return listLegacyAutomationAgents(options);
}

function generatedFixedContent(
  content: AutomationAgentFixedContentRequest["content_package"],
) {
  return {
    content_text: content.content_text,
    image_library_ids: [...content.image_library_ids],
    miniprogram_library_ids: [] as number[],
    attachment_library_ids: [] as number[],
    group_invite_library_ids: [] as number[],
  };
}

export interface AutomationAgentsTransport {
  readonly read: typeof generatedRead;
  // Lane E owns registration of these existing server handlers in the root
  // OpenAPI/Orval contract. Optional members keep this branch from inventing
  // paths or bypassing generated transport before that integration lands.
  readonly get?: (...args: [number, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly create?: (...args: [AutomationAgentCreateRequest, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly update?: (...args: [number, AutomationAgentUpdateRequest, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly copy?: (...args: [number, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly publish?: (...args: [number, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly activate?: (...args: [number, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly pause?: (...args: [number, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly archive?: (...args: [number, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
  readonly saveFixedContent?: (...args: [number, AutomationAgentFixedContentRequest, RequestInit]) => Promise<AutomationAgentsTransportResponse>;
}

export const generatedAutomationAgentsTransport: AutomationAgentsTransport = {
  read: generatedRead,
  get: async (id, options) => getLegacyAutomationAgent(id, options),
  create: async (request, options) =>
    createLegacyAutomationAgent(
      {
        ...request,
        fixed_content_package: generatedFixedContent(
          request.fixed_content_package,
        ),
      },
      options,
    ),
  update: async (id, request, options) =>
    updateLegacyAutomationAgent(id, request, options),
  copy: async (id, options) => copyLegacyAutomationAgent(id, options),
  publish: async (id, options) => publishLegacyAutomationAgent(id, options),
  activate: async (id, options) => activateLegacyAutomationAgent(id, options),
  pause: async (id, options) => pauseLegacyAutomationAgent(id, options),
  archive: async (id, options) => archiveLegacyAutomationAgent(id, options),
  saveFixedContent: async (id, request, options) =>
    saveLegacyAutomationAgentFixedContent(
      id,
      { content_package: generatedFixedContent(request.content_package) },
      options,
    ),
};

export type AutomationAgentsFailure = "unauthenticated" | "forbidden" | "invalid" | "unavailable";
export type AutomationAgentsResult = { readonly status: "loaded"; readonly snapshot: AutomationAgentsSnapshot } | { readonly status: AutomationAgentsFailure };

export interface AutomationAgentFixedContent {
  readonly contentText: string;
  readonly imageLibraryIDs: readonly number[];
  readonly hasUnsupportedBindings: boolean;
}
export interface AutomationAgentDetail extends AutomationAgentSummary {
  readonly draftRolePrompt: string;
  readonly draftTaskPrompt: string;
  readonly publishedRolePrompt: string;
  readonly publishedTaskPrompt: string;
  readonly draftVersion: number;
  readonly publishedVersion: number;
  readonly hasUnpublishedChanges: boolean;
  readonly fixedContent: AutomationAgentFixedContent;
}
export interface AutomationAgentDraft {
  readonly name: string;
  readonly code: string;
  readonly type: AutomationAgentType;
  readonly rolePrompt: string;
  readonly taskPrompt: string;
}
export interface AutomationAgentFixedContentRequest {
  readonly content_package: {
    readonly content_text: string;
    readonly image_library_ids: readonly number[];
    readonly miniprogram_library_ids: readonly [];
    readonly attachment_library_ids: readonly [];
    readonly group_invite_library_ids: readonly [];
  };
}
export interface AutomationAgentCreateRequest {
  readonly agent_name: string;
  readonly agent_code: string;
  readonly automation_type: AutomationAgentType;
  readonly status: "active";
  readonly role_prompt: string;
  readonly task_prompt: string;
  readonly fixed_content_package: AutomationAgentFixedContentRequest["content_package"];
}
export interface AutomationAgentUpdateRequest {
  readonly agent_name: string;
  readonly automation_type: AutomationAgentType;
  readonly role_prompt: string;
  readonly task_prompt: string;
}
export type AutomationAgentDetailResult = { readonly status: "loaded"; readonly agent: AutomationAgentDetail } | { readonly status: AutomationAgentsFailure };
export type AutomationAgentMutationResult =
  | { readonly status: "succeeded"; readonly agent: AutomationAgentDetail }
  | { readonly status: "archived"; readonly id: number }
  | { readonly status: "unauthenticated" | "forbidden" | "invalid" | "not_found" | "conflict" | "unknown" };
export const defaultAutomationAgentDraft: AutomationAgentDraft = { name: "", code: "", type: "agent", rolePrompt: "", taskPrompt: "" };

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);
}
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}
function text(value: unknown, minimum = 0, maximum = 120): value is string {
  return typeof value === "string" && Array.from(value).length >= minimum && Array.from(value).length <= maximum;
}
function positive(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function nonnegative(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 0; }
function timestamp(value: unknown): value is string {
  if (!text(value, 20, 64) || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) || !Number.isFinite(Date.parse(value))) return false;
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
  const calendar = new Date(0); calendar.setUTCFullYear(year, month - 1, day); calendar.setUTCHours(hour, minute, second, 0);
  return calendar.getUTCFullYear() === year && calendar.getUTCMonth() === month - 1 && calendar.getUTCDate() === day && calendar.getUTCHours() === hour && calendar.getUTCMinutes() === minute && calendar.getUTCSeconds() === second;
}
function materialSummary(value: unknown): AutomationAgentMaterialSummary | undefined {
  if (!record(value) || !exact(value, ["image_count", "miniprogram_count", "attachment_count", "group_invite_count"]) || !nonnegative(value.image_count) || !nonnegative(value.miniprogram_count) || !nonnegative(value.attachment_count) || !nonnegative(value.group_invite_count)) return undefined;
  return { imageCount: value.image_count, miniprogramCount: value.miniprogram_count, attachmentCount: value.attachment_count, groupInviteCount: value.group_invite_count };
}
function parseItem(value: unknown): AutomationAgentSummary | undefined {
  if (!record(value) || !exact(value, ["id", "automation_type", "agent_code", "agent_name", "bound_package_key", "bound_package_id", "bound_package_name", "fixed_material_summary", "status", "updated_at"]) || !positive(value.id) || (value.automation_type !== "agent" && value.automation_type !== "fixed_script") || !text(value.agent_code, 1, 120) || !/^[a-z0-9_-]+$/.test(value.agent_code) || !text(value.agent_name, 1, 120) || value.bound_package_key !== "" || value.bound_package_id !== null || value.bound_package_name !== "" || (value.status !== "active" && value.status !== "paused") || !timestamp(value.updated_at)) return undefined;
  const summary = materialSummary(value.fixed_material_summary);
  return summary ? { id: value.id, type: value.automation_type, typeLabel: value.automation_type === "agent" ? "Agent 机器人" : "固定话术", code: value.agent_code, name: value.agent_name, status: value.status, updatedAt: value.updated_at, materialSummary: summary } : undefined;
}
export function parseAutomationAgents(value: unknown): AutomationAgentsSnapshot | undefined {
  if (!record(value) || !exact(value, ["ok", "items", "total"]) || value.ok !== true || !Array.isArray(value.items) || value.items.length > 200 || !nonnegative(value.total) || value.total !== value.items.length) return undefined;
  const items = value.items.map(parseItem);
  if (items.some((item) => item === undefined)) return undefined;
  const parsed = items as AutomationAgentSummary[];
  if (new Set(parsed.map((item) => item.id)).size !== parsed.length) return undefined;
  for (let index = 1; index < parsed.length; index += 1) {
    const previous = parsed[index - 1]; const current = parsed[index];
    if (previous.updatedAt < current.updatedAt || (previous.updatedAt === current.updatedAt && previous.id < current.id)) return undefined;
  }
  return { items: parsed, total: value.total };
}
export function filterAutomationAgents(snapshot: AutomationAgentsSnapshot, keyword: string, type: AutomationAgentType | "all", status: AutomationAgentStatus | "all"): readonly AutomationAgentSummary[] {
  const query = keyword.trim().toLocaleLowerCase();
  return snapshot.items.filter((item) => (type === "all" || item.type === type) && (status === "all" || item.status === status) && (query === "" || item.name.toLocaleLowerCase().includes(query) || item.code.toLocaleLowerCase().includes(query)));
}
export async function loadAutomationAgents(transport: AutomationAgentsTransport = generatedAutomationAgentsTransport): Promise<AutomationAgentsResult> {
  try {
    const response = await transport.read({ credentials: "same-origin" });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status !== 200) return { status: "unavailable" };
    const snapshot = parseAutomationAgents(response.data);
    return snapshot ? { status: "loaded", snapshot } : { status: "invalid" };
  } catch { return { status: "unavailable" }; }
}

const DETAIL_KEYS = [
  "id", "automation_type", "agent_code", "agent_name", "bound_package_key", "bound_package_id", "bound_package_name", "fixed_material_summary", "status", "updated_at",
  "automation_type_label", "draft_role_prompt", "draft_task_prompt", "published_role_prompt", "published_task_prompt", "draft_version", "published_version", "has_unpublished_changes", "fixed_content_package", "fixed_content_package_preview",
] as const;
const CONTENT_KEYS = ["content_text", "image_library_ids", "miniprogram_library_ids", "attachment_library_ids", "group_invite_library_ids"] as const;

function positiveIDs(value: unknown, limit: number): readonly number[] | undefined {
  if (!Array.isArray(value) || value.length > limit || !value.every(positive)) return undefined;
  return new Set(value).size === value.length ? value : undefined;
}
function dynamicCard(value: unknown): boolean {
  return record(value) && Object.keys(value).every((key) => ["schema_version", "appid", "title", "pagepath", "card_id", "cid", "cover_image_id"].includes(key));
}
function parseFixedContent(value: unknown): AutomationAgentFixedContent | undefined {
  if (!record(value)) return undefined;
  const hasDynamic = Object.hasOwn(value, "dynamic_miniprogram_card");
  if (!exact(value, hasDynamic ? [...CONTENT_KEYS, "dynamic_miniprogram_card"] : CONTENT_KEYS) || !text(value.content_text, 0, 4_000)) return undefined;
  const images = positiveIDs(value.image_library_ids, 3); const minis = positiveIDs(value.miniprogram_library_ids, 1);
  const attachments = positiveIDs(value.attachment_library_ids, 9); const invites = positiveIDs(value.group_invite_library_ids, 1);
  if (!images || !minis || !attachments || !invites || images.length + minis.length + attachments.length + invites.length > 9 || (hasDynamic && !dynamicCard(value.dynamic_miniprogram_card))) return undefined;
  return { contentText: value.content_text, imageLibraryIDs: images, hasUnsupportedBindings: minis.length > 0 || attachments.length > 0 || invites.length > 0 || hasDynamic };
}
function sameSummary(left: AutomationAgentMaterialSummary, right: AutomationAgentMaterialSummary): boolean {
  return left.imageCount === right.imageCount && left.miniprogramCount === right.miniprogramCount && left.attachmentCount === right.attachmentCount && left.groupInviteCount === right.groupInviteCount;
}
export function parseAutomationAgentDetail(value: unknown, requestedID: number): AutomationAgentDetail | undefined {
  if (!positive(requestedID) || !record(value) || !exact(value, ["ok", "agent"]) || value.ok !== true || !record(value.agent)) return undefined;
  const agent = value.agent;
  if (!exact(agent, DETAIL_KEYS)) return undefined;
  const summary = parseItem(Object.fromEntries(["id", "automation_type", "agent_code", "agent_name", "bound_package_key", "bound_package_id", "bound_package_name", "fixed_material_summary", "status", "updated_at"].map((key) => [key, agent[key]])));
  if (!summary || summary.id !== requestedID || agent.automation_type_label !== summary.typeLabel || !text(agent.draft_role_prompt, 0, 20_000) || !text(agent.draft_task_prompt, 0, 20_000) || !text(agent.published_role_prompt, 0, 20_000) || !text(agent.published_task_prompt, 0, 20_000) || !positive(agent.draft_version) || !positive(agent.published_version) || agent.published_version > agent.draft_version || typeof agent.has_unpublished_changes !== "boolean") return undefined;
  const fixedContent = parseFixedContent(agent.fixed_content_package);
  if (!fixedContent || !record(agent.fixed_content_package_preview) || !exact(agent.fixed_content_package_preview, ["content_text", "material_summary", "materials"]) || agent.fixed_content_package_preview.content_text !== fixedContent.contentText || !Array.isArray(agent.fixed_content_package_preview.materials) || agent.fixed_content_package_preview.materials.length !== 0) return undefined;
  const preview = materialSummary(agent.fixed_content_package_preview.material_summary);
  const rawContent = agent.fixed_content_package;
  const miniprograms = record(rawContent) ? positiveIDs(rawContent.miniprogram_library_ids, 1) : undefined;
  const attachments = record(rawContent) ? positiveIDs(rawContent.attachment_library_ids, 9) : undefined;
  const invites = record(rawContent) ? positiveIDs(rawContent.group_invite_library_ids, 1) : undefined;
  if (!preview || !miniprograms || !attachments || !invites || !sameSummary(preview, summary.materialSummary) || preview.imageCount !== fixedContent.imageLibraryIDs.length || preview.miniprogramCount !== miniprograms.length || preview.attachmentCount !== attachments.length || preview.groupInviteCount !== invites.length || agent.has_unpublished_changes !== (agent.draft_version !== agent.published_version || agent.draft_role_prompt !== agent.published_role_prompt || agent.draft_task_prompt !== agent.published_task_prompt)) return undefined;
  return { ...summary, draftRolePrompt: agent.draft_role_prompt, draftTaskPrompt: agent.draft_task_prompt, publishedRolePrompt: agent.published_role_prompt, publishedTaskPrompt: agent.published_task_prompt, draftVersion: agent.draft_version, publishedVersion: agent.published_version, hasUnpublishedChanges: agent.has_unpublished_changes, fixedContent };
}

export function normalizeAutomationAgentDraft(value: AutomationAgentDraft): AutomationAgentDraft | undefined {
  const draft = { name: value.name.trim(), code: value.code.trim(), type: value.type, rolePrompt: value.rolePrompt.trim(), taskPrompt: value.taskPrompt.trim() };
  return text(draft.name, 1, 120) && text(draft.code, 1, 120) && /^[a-z0-9_-]+$/.test(draft.code) && (draft.type === "agent" || draft.type === "fixed_script") && text(draft.rolePrompt, 0, 20_000) && text(draft.taskPrompt, 0, 20_000) ? draft : undefined;
}
export function automationAgentCreateRequest(draft: AutomationAgentDraft): AutomationAgentCreateRequest | undefined {
  const value = normalizeAutomationAgentDraft(draft); if (!value) return undefined;
  return { agent_name: value.name, agent_code: value.code, automation_type: value.type, status: "active", role_prompt: value.rolePrompt, task_prompt: value.taskPrompt, fixed_content_package: { content_text: "", image_library_ids: [], miniprogram_library_ids: [], attachment_library_ids: [], group_invite_library_ids: [] } };
}
export function automationAgentUpdateRequest(draft: AutomationAgentDraft): AutomationAgentUpdateRequest | undefined {
  const value = normalizeAutomationAgentDraft(draft); return value ? { agent_name: value.name, automation_type: value.type, role_prompt: value.rolePrompt, task_prompt: value.taskPrompt } : undefined;
}
export function automationAgentFixedContentRequest(type: AutomationAgentType, contentText: string, imageIDs: readonly number[]): AutomationAgentFixedContentRequest | undefined {
  const content = contentText.trim();
  if (!text(content, 0, 4_000) || (type !== "fixed_script" && content !== "") || imageIDs.length > 3 || !imageIDs.every(positive) || new Set(imageIDs).size !== imageIDs.length) return undefined;
  return { content_package: { content_text: content, image_library_ids: [...imageIDs], miniprogram_library_ids: [], attachment_library_ids: [], group_invite_library_ids: [] } };
}
export function newAutomationAgentIdempotencyKey(source: { readonly randomUUID: () => string } | undefined = globalThis.crypto): string | undefined {
  try { const uuid = source?.randomUUID(); return typeof uuid === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid) ? `automation-agent:${uuid}` : undefined; } catch { return undefined; }
}
function writeOptions(csrf: string, key: string, body = false): RequestInit | undefined {
  if (!/^[A-Za-z0-9_-]{43}$/.test(csrf) || !/^[A-Za-z0-9:_-]{16,128}$/.test(key)) return undefined;
  return { credentials: "same-origin", headers: { ...(body ? { "Content-Type": "application/json" } : {}), "X-CSRF-Token": csrf, "Idempotency-Key": key } };
}
function mutationFailure(status: number): Exclude<AutomationAgentMutationResult, { readonly status: "succeeded" } | { readonly status: "archived" }> {
  if (status === 401) return { status: "unauthenticated" }; if (status === 403) return { status: "forbidden" }; if (status === 400) return { status: "invalid" }; if (status === 404) return { status: "not_found" }; if (status === 409) return { status: "conflict" }; return { status: "unknown" };
}
export async function loadAutomationAgentDetail(transport: AutomationAgentsTransport, id: number): Promise<AutomationAgentDetailResult> {
  if (!positive(id) || !transport.get) return { status: "invalid" };
  try { const response = await transport.get(id, { credentials: "same-origin" }); if (response.status === 401) return { status: "unauthenticated" }; if (response.status === 403) return { status: "forbidden" }; if (response.status !== 200) return { status: "unavailable" }; const agent = parseAutomationAgentDetail(response.data, id); return agent ? { status: "loaded", agent } : { status: "invalid" }; } catch { return { status: "unavailable" }; }
}
async function commandDetail(command: ((...args: [RequestInit]) => Promise<AutomationAgentsTransportResponse>) | undefined, expectedID: number | undefined, csrf: string, key: string): Promise<AutomationAgentMutationResult> {
  const options = writeOptions(csrf, key); if (!command || !options) return { status: "invalid" };
  try { const response = await command(options); if (response.status !== 200) return mutationFailure(response.status); if (!record(response.data) || !record(response.data.agent) || !positive(response.data.agent.id)) return { status: "unknown" }; const agent = parseAutomationAgentDetail(response.data, response.data.agent.id); return agent && (expectedID === undefined || agent.id === expectedID) ? { status: "succeeded", agent } : { status: "unknown" }; } catch { return { status: "unknown" }; }
}
export async function createAutomationAgent(transport: AutomationAgentsTransport, draft: AutomationAgentDraft, csrf: string, key: string): Promise<AutomationAgentMutationResult> {
  const request = automationAgentCreateRequest(draft); const options = writeOptions(csrf, key, true); if (!request || !options || !transport.create) return { status: "invalid" };
  try { const response = await transport.create(request, options); if (response.status !== 200) return mutationFailure(response.status); if (!record(response.data) || !record(response.data.agent) || !positive(response.data.agent.id)) return { status: "unknown" }; const agent = parseAutomationAgentDetail(response.data, response.data.agent.id); return agent && agent.code === request.agent_code && agent.name === request.agent_name && agent.type === request.automation_type && agent.status === "active" && agent.draftRolePrompt === request.role_prompt && agent.draftTaskPrompt === request.task_prompt ? { status: "succeeded", agent } : { status: "unknown" }; } catch { return { status: "unknown" }; }
}
export async function updateAutomationAgent(transport: AutomationAgentsTransport, id: number, draft: AutomationAgentDraft, csrf: string, key: string): Promise<AutomationAgentMutationResult> {
  const request = automationAgentUpdateRequest(draft); const options = writeOptions(csrf, key, true); if (!request || !options || !transport.update) return { status: "invalid" };
  try { const response = await transport.update(id, request, options); if (response.status !== 200) return mutationFailure(response.status); const agent = parseAutomationAgentDetail(response.data, id); return agent && agent.name === request.agent_name && agent.type === request.automation_type && agent.draftRolePrompt === request.role_prompt && agent.draftTaskPrompt === request.task_prompt ? { status: "succeeded", agent } : { status: "unknown" }; } catch { return { status: "unknown" }; }
}
export async function saveAutomationAgentFixedContent(transport: AutomationAgentsTransport, id: number, type: AutomationAgentType, content: string, images: readonly number[], csrf: string, key: string): Promise<AutomationAgentMutationResult> {
  const request = automationAgentFixedContentRequest(type, content, images); const options = writeOptions(csrf, key, true); if (!request || !options || !transport.saveFixedContent) return { status: "invalid" };
  try { const response = await transport.saveFixedContent(id, request, options); if (response.status !== 200) return mutationFailure(response.status); const agent = parseAutomationAgentDetail(response.data, id); return agent && agent.fixedContent.contentText === request.content_package.content_text && agent.fixedContent.imageLibraryIDs.join(",") === request.content_package.image_library_ids.join(",") ? { status: "succeeded", agent } : { status: "unknown" }; } catch { return { status: "unknown" }; }
}
export function copyAutomationAgent(transport: AutomationAgentsTransport, id: number, csrf: string, key: string): Promise<AutomationAgentMutationResult> { return commandDetail(transport.copy ? (options) => transport.copy!(id, options) : undefined, undefined, csrf, key); }
export function publishAutomationAgent(transport: AutomationAgentsTransport, id: number, csrf: string, key: string): Promise<AutomationAgentMutationResult> { return commandDetail(transport.publish ? (options) => transport.publish!(id, options) : undefined, id, csrf, key); }
export function setAutomationAgentStatus(transport: AutomationAgentsTransport, id: number, status: AutomationAgentStatus, csrf: string, key: string): Promise<AutomationAgentMutationResult> { const operation = status === "active" ? transport.activate : transport.pause; return commandDetail(operation ? (options) => operation(id, options) : undefined, id, csrf, key); }
export async function archiveAutomationAgent(transport: AutomationAgentsTransport, id: number, csrf: string, key: string): Promise<AutomationAgentMutationResult> {
  const options = writeOptions(csrf, key); if (!positive(id) || !transport.archive || !options) return { status: "invalid" };
  try { const response = await transport.archive(id, options); if (response.status !== 200) return mutationFailure(response.status); return record(response.data) && exact(response.data, ["ok", "agent"]) && response.data.ok === true && record(response.data.agent) && exact(response.data.agent, ["id", "status"]) && response.data.agent.id === id && response.data.agent.status === "archived" ? { status: "archived", id } : { status: "unknown" }; } catch { return { status: "unknown" }; }
}
