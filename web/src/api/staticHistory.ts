import { listStaticHistoryGroupInvite, getStaticHistoryGroupInvite, type StaticHistoryGroupInvite, listStaticHistoryProductPageSlice, getStaticHistoryProductPageSlice, type StaticHistoryProductPageSlice, listStaticHistoryCycleStrategy, getStaticHistoryCycleStrategy, type StaticHistoryCycleStrategy, listStaticHistoryCycleVersion, getStaticHistoryCycleVersion, type StaticHistoryCycleVersion, listStaticHistoryCycleDocument, getStaticHistoryCycleDocument, type StaticHistoryCycleDocument } from './generated/health';
import { apiRequestOptions, unwrapGenerated } from './transport';

export type StaticHistoryKind = 'GroupInvite' | 'ProductPageSlice' | 'CycleStrategy' | 'CycleVersion' | 'CycleDocument';
export type StaticHistoryItem = StaticHistoryGroupInvite | StaticHistoryProductPageSlice | StaticHistoryCycleStrategy | StaticHistoryCycleVersion | StaticHistoryCycleDocument;
export type StaticHistoryPage = { items: StaticHistoryItem[]; total: number; limit: number; offset: number };
type Row = Record<string, unknown>;
const invalid = (): never => { throw new Error('静态历史响应无效，未显示旧数据'); };
const integer = (x: unknown, min?: number): x is number => typeof x === 'number' && Number.isSafeInteger(x) && (min === undefined || x >= min);
const text = (x: unknown): x is string => typeof x === 'string';
const instant = (x: unknown): boolean => text(x) && /(?:Z|[+-]\d{2}:\d{2})$/.test(x) && Number.isFinite(Date.parse(x));
const digest = (x: unknown): boolean => Array.isArray(x) && x.length === 32 && x.every((v) => integer(v, 0) && v <= 255) && x.some((v) => v !== 0);
const nullable = (x: unknown, check: (v: unknown) => boolean): boolean => x === null || check(x);
const fields: Record<StaticHistoryKind, Record<string, (x: unknown) => boolean>> = {
 GroupInvite: {
  name: text,
  title: text,
  description: text,
  original_state: text,
  original_auto_create: (x: unknown) => typeof x === 'boolean',
  room_base_name: text,
  room_base_source_id: (x: unknown) => nullable(x, integer),
  original_enabled: (x: unknown) => typeof x === 'boolean',
  original_binding_state: text,
  created_at: instant,
  updated_at: instant,
 },
 ProductPageSlice: {
  product_source_id: integer,
  image_source_id: integer,
  sort_order: integer,
  original_enabled: (x: unknown) => typeof x === 'boolean',
  created_at: instant,
  updated_at: instant,
 },
 CycleStrategy: {
  strategy_key: text,
  title: text,
  description: text,
  cadence: text,
  timezone: text,
  original_status: text,
  current_version: integer,
  created_at: instant,
  updated_at: instant,
 },
 CycleVersion: {
  strategy_source_id: integer,
  strategy_history_id: (x: unknown) => integer(x, 1),
  version: integer,
  label: text,
  objective: text,
  version_hash: text,
  effective_from: (x: unknown) => nullable(x, instant),
  original_governance: text,
  confirmed_at: (x: unknown) => nullable(x, instant),
  operation_skill_hash: text,
  created_at: instant,
 },
 CycleDocument: {
  strategy_version_source_id: integer,
  version_history_id: (x: unknown) => integer(x, 1),
  schema_version: text,
  execution_guide_sha256: text,
  execution_guide_generated_at: (x: unknown) => nullable(x, instant),
  copy_guide_sha256: text,
  copy_guide_generated_at: (x: unknown) => nullable(x, instant),
  measurement_guide_sha256: text,
  measurement_guide_generated_at: (x: unknown) => nullable(x, instant),
  document_pack_hash: text,
  created_at: instant,
 },
};
function object(x: unknown, keys: string[]): Row {
 if (!x || typeof x !== 'object' || Array.isArray(x) || Object.keys(x).length !== keys.length || Object.keys(x).some((k) => !keys.includes(k))) invalid();
 return x as Row;
}
function envelope(x: unknown, keys: string[]): Row {
 const row = object(x, ['source', 'read_only', 'real_external_call_executed', ...keys]);
 if (row.source !== 'v1_history' || row.read_only !== true || row.real_external_call_executed !== false) invalid();
 return row;
}
function item(kind: StaticHistoryKind, x: unknown): StaticHistoryItem {
 const row = object(x, ['id', 'source_id', 'source_key_digest', 'source_payload_digest', ...Object.keys(fields[kind])]);
 if (!integer(row.id, 1) || !integer(row.source_id) || !digest(row.source_key_digest) || !digest(row.source_payload_digest) || Object.entries(fields[kind]).some(([key, check]) => !check(row[key]))) invalid();
 return row as unknown as StaticHistoryItem;
}
function page(kind: StaticHistoryKind, x: unknown, limit: number, offset: number, parent?: number): StaticHistoryPage {
 const row = envelope(x, ['items', 'total', 'limit', 'offset']);
 if (!Array.isArray(row.items) || !integer(row.total, 0) || row.limit !== limit || row.offset !== offset) invalid();
 const total = row.total as number, items = (row.items as unknown[]).map((x) => item(kind, x));
 if (items.length !== Math.min(limit, Math.max(0, total - offset)) || new Set(items.map((x) => x.id)).size !== items.length) invalid();
 if (parent !== undefined && items.some((x) => (x as unknown as Row)[kind === 'CycleVersion' ? 'strategy_history_id' : 'version_history_id'] !== parent)) invalid();
 return { items, total, limit, offset };
}
export async function readStaticHistory(kind: StaticHistoryKind, offset = 0, limit = 20, parent?: number): Promise<StaticHistoryPage> {
 if (!integer(limit, 1) || limit > 100 || !integer(offset, 0) || parent !== undefined && (!integer(parent, 1) || !['CycleVersion', 'CycleDocument'].includes(kind))) throw new Error('静态历史分页或父ID无效');
 switch (kind) {
 case 'GroupInvite': return page(kind, unwrapGenerated(await listStaticHistoryGroupInvite({limit,offset},apiRequestOptions())),limit,offset,parent);
 case 'ProductPageSlice': return page(kind, unwrapGenerated(await listStaticHistoryProductPageSlice({limit,offset},apiRequestOptions())),limit,offset,parent);
 case 'CycleStrategy': return page(kind, unwrapGenerated(await listStaticHistoryCycleStrategy({limit,offset},apiRequestOptions())),limit,offset,parent);
 case 'CycleVersion': return page(kind, unwrapGenerated(await listStaticHistoryCycleVersion({limit,offset,strategy_history_id:parent},apiRequestOptions())),limit,offset,parent);
 case 'CycleDocument': return page(kind, unwrapGenerated(await listStaticHistoryCycleDocument({limit,offset,version_history_id:parent},apiRequestOptions())),limit,offset,parent);
 }
}
export async function getStaticHistory(kind: StaticHistoryKind, id: number): Promise<StaticHistoryItem> {
 if (!integer(id, 1)) throw new Error('静态历史ID无效');
 let response: unknown;
 switch (kind) {
 case 'GroupInvite': response=unwrapGenerated(await getStaticHistoryGroupInvite(id,apiRequestOptions())); break;
 case 'ProductPageSlice': response=unwrapGenerated(await getStaticHistoryProductPageSlice(id,apiRequestOptions())); break;
 case 'CycleStrategy': response=unwrapGenerated(await getStaticHistoryCycleStrategy(id,apiRequestOptions())); break;
 case 'CycleVersion': response=unwrapGenerated(await getStaticHistoryCycleVersion(id,apiRequestOptions())); break;
 case 'CycleDocument': response=unwrapGenerated(await getStaticHistoryCycleDocument(id,apiRequestOptions())); break;
 }
 const result=item(kind,envelope(response,['item']).item);
 if(result.id!==id) invalid();
 return result;
}
