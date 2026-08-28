import {
  getWeComContactHistoryEvent,
  getWeComContactHistoryRelation,
  listWeComContactHistoryEvents,
  listWeComContactHistoryRelations,
  type WeComContactHistoryEvent,
  type WeComContactHistoryEventPage,
  type WeComContactHistoryRelation,
  type WeComContactHistoryRelationPage,
} from './generated/health';
import { apiRequestOptions, unwrapGenerated } from './transport';

export type { WeComContactHistoryEvent, WeComContactHistoryRelation };
export type WeComContactHistoryPage<T> = { items: T[]; total: number; limit: number; offset: number };
type Row = Record<string, unknown>;
const invalid = (): never => { throw new Error('企微联系人历史响应不符合只读契约'); };
const integer = (value: unknown, minimum?: number): value is number => typeof value === 'number' && Number.isSafeInteger(value) && (minimum === undefined || value >= minimum);
const instant = (value: unknown): value is string => typeof value === 'string' && Number.isFinite(Date.parse(value));
function object(value: unknown, keys: string[]): Row {
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).length !== keys.length || Object.keys(value).some((key) => !keys.includes(key))) invalid();
  return value as Row;
}
function event(value: unknown): WeComContactHistoryEvent {
  const row = object(value, ['id', 'source_id', 'event_type', 'change_type', 'event_time', 'process_status', 'retry_count', 'created_at', 'updated_at', 'identity_sync_status']);
  if (!integer(row.id, 1) || !integer(row.source_id) || !['event_type', 'change_type', 'process_status', 'identity_sync_status'].every((key) => typeof row[key] === 'string') ||
    (row.event_time !== null && !integer(row.event_time)) || !integer(row.retry_count) || !instant(row.created_at) || !instant(row.updated_at)) invalid();
  return row as unknown as WeComContactHistoryEvent;
}
function relation(value: unknown): WeComContactHistoryRelation {
  const row = object(value, ['id', 'source_id', 'relation_status', 'is_primary', 'add_way', 'create_time', 'first_seen_at', 'last_seen_at', 'created_at', 'updated_at']);
  if (!integer(row.id, 1) || !integer(row.source_id) || typeof row.relation_status !== 'string' || typeof row.is_primary !== 'boolean' ||
    (row.add_way !== null && !integer(row.add_way)) || (row.create_time !== null && !integer(row.create_time)) ||
    !instant(row.first_seen_at) || !instant(row.last_seen_at) || !instant(row.created_at) || !instant(row.updated_at)) invalid();
  return row as unknown as WeComContactHistoryRelation;
}
function page<T>(value: unknown, offset: number, limit: number, convert: (item: unknown) => T): WeComContactHistoryPage<T> {
  const row = object(value, ['source', 'read_only', 'real_external_call_executed', 'items', 'total', 'limit', 'offset']);
  if (row.source !== 'v1_history' || row.read_only !== true || row.real_external_call_executed !== false || !Array.isArray(row.items) || !integer(row.total, 0) || row.limit !== limit || row.offset !== offset || row.items.length > limit) invalid();
  const items = row.items as unknown[];
  if (items.length !== Math.min(limit, Math.max(0, (row.total as number) - offset))) invalid();
  return { items: items.map(convert), total: row.total as number, limit, offset };
}
function detail<T extends { id: number }>(value: unknown, expectedID: number, convert: (item: unknown) => T): T {
  const row = object(value, ['source', 'read_only', 'real_external_call_executed', 'item']);
  if (row.source !== 'v1_history' || row.read_only !== true || row.real_external_call_executed !== false) invalid();
  const result = convert(row.item);
  if (result.id !== expectedID) invalid();
  return result;
}
function pagination(offset: number, limit: number): void {
  if (!integer(offset, 0) || offset > 2147483647 || !integer(limit, 1) || limit > 100) throw new Error('企微联系人历史分页参数无效');
}

export async function readWeComContactHistoryEvents(offset = 0, limit = 20): Promise<WeComContactHistoryPage<WeComContactHistoryEvent>> {
  pagination(offset, limit);
  return page(unwrapGenerated(await listWeComContactHistoryEvents({ offset, limit }, apiRequestOptions())), offset, limit, event);
}
export async function readWeComContactHistoryEvent(historyID: number): Promise<WeComContactHistoryEvent> {
  if (!integer(historyID, 1)) throw new Error('企微联系人事件历史 ID 无效');
  return detail(unwrapGenerated(await getWeComContactHistoryEvent(historyID, apiRequestOptions())), historyID, event);
}
export async function readWeComContactHistoryRelations(offset = 0, limit = 20): Promise<WeComContactHistoryPage<WeComContactHistoryRelation>> {
  pagination(offset, limit);
  return page(unwrapGenerated(await listWeComContactHistoryRelations({ offset, limit }, apiRequestOptions())), offset, limit, relation);
}
export async function readWeComContactHistoryRelation(historyID: number): Promise<WeComContactHistoryRelation> {
  if (!integer(historyID, 1)) throw new Error('企微联系人关系历史 ID 无效');
  return detail(unwrapGenerated(await getWeComContactHistoryRelation(historyID, apiRequestOptions())), historyID, relation);
}
