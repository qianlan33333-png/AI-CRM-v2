import {
  readWeComContactHistoryEvent,
  readWeComContactHistoryEvents,
  readWeComContactHistoryRelation,
  readWeComContactHistoryRelations,
  type WeComContactHistoryEvent,
  type WeComContactHistoryPage,
  type WeComContactHistoryRelation,
} from '../../api/wecomContactHistory';
import { esc } from './util';

type Kind = 'event' | 'relation';
type Options = { kind?: string; historyID?: string };
const button = 'padding:7px 12px;border:1px solid #D0D5DD;border-radius:6px;background:#fff;color:#344054;font-size:13px;cursor:pointer';
const cell = 'padding:10px 12px;border-bottom:1px solid #EEF0F3;text-align:left;vertical-align:top;white-space:pre-wrap';
const text = (value: string | number | null): string => value === null ? 'NULL' : value === '' ? '（空字符串）' : esc(String(value));
const rawInteger = (value: number | null): string => value === null ? 'NULL' : `原值 ${value}`;
function historyID(value: string | undefined): number | undefined {
  if (value === undefined) return undefined;
  const parsed = Number(value);
  if (!/^[1-9]\d*$/.test(value) || !Number.isSafeInteger(parsed)) throw new Error('企微联系人历史 ID 无效');
  return parsed;
}
function url(kind: Kind, id?: number): string {
  const query = new URLSearchParams({ wecom_contact_history: '1', history_kind: kind });
  if (id !== undefined) query.set('history_id', String(id));
  return `config.html?${query.toString()}`;
}
function nav(kind: Kind): string {
  return `<div style="display:flex;gap:8px;flex-wrap:wrap"><a style="${button};text-decoration:none;${kind === 'event' ? 'background:#EEF4FF;border-color:#84ADFF' : ''}" href="${url('event')}">V1 联系人事件</a><a style="${button};text-decoration:none;${kind === 'relation' ? 'background:#EEF4FF;border-color:#84ADFF' : ''}" href="${url('relation')}">V1 客户关系</a></div>`;
}
function eventCells(item: WeComContactHistoryEvent): string[] {
  return [
    `历史事件 #${item.id}<br>源 ID：${item.source_id}`,
    `事件：${text(item.event_type)}<br>变更：${text(item.change_type)}<br>处理：${text(item.process_status)}<br>同步：${text(item.identity_sync_status)}<br>重试原值：${item.retry_count}`,
    `EventTime：${rawInteger(item.event_time)}<br>创建：${text(item.created_at)}<br>更新：${text(item.updated_at)}`,
    `<a data-wecom-contact-history-id="${item.id}" href="${url('event', item.id)}">查看历史详情</a>`,
  ];
}
function relationCells(item: WeComContactHistoryRelation): string[] {
  return [
    `历史关系 #${item.id}<br>源 ID：${item.source_id}`,
    `状态：${text(item.relation_status)}<br>${item.is_primary ? 'V1主跟进标记（非当前负责人）' : '非 V1 主跟进标记'}<br>AddWay：${rawInteger(item.add_way)}`,
    `CreateTime：${rawInteger(item.create_time)}<br>首次观察：${text(item.first_seen_at)}<br>最后观察：${text(item.last_seen_at)}<br>创建：${text(item.created_at)}<br>更新：${text(item.updated_at)}`,
    `<a data-wecom-contact-history-id="${item.id}" href="${url('relation', item.id)}">查看历史详情</a>`,
  ];
}
function rows(cells: string[]): string { return `<tr>${cells.map((value) => `<td style="${cell}">${value}</td>`).join('')}</tr>`; }
function detailRows(kind: Kind, item: WeComContactHistoryEvent | WeComContactHistoryRelation): string {
  if (kind === 'event') {
    const value = item as WeComContactHistoryEvent;
    return rows(['历史 ID / 源 ID', `${value.id} / ${value.source_id}`, '事件 / 变更', `${text(value.event_type)} / ${text(value.change_type)}`]) +
      rows(['处理 / 同步', `${text(value.process_status)} / ${text(value.identity_sync_status)}`, '重试原值', String(value.retry_count)]) +
      rows(['EventTime（原整数，单位未知）', rawInteger(value.event_time), '创建 / 更新', `${text(value.created_at)} / ${text(value.updated_at)}`]);
  }
  const value = item as WeComContactHistoryRelation;
  return rows(['历史 ID / 源 ID', `${value.id} / ${value.source_id}`, '关系状态', text(value.relation_status)]) +
    rows(['主跟进', value.is_primary ? 'V1主跟进标记（非当前负责人）' : '非 V1 主跟进标记', 'AddWay 原整数', rawInteger(value.add_way)]) +
    rows(['CreateTime（原整数，单位未知）', rawInteger(value.create_time), '首次 / 最后观察', `${text(value.first_seen_at)} / ${text(value.last_seen_at)}`]) +
    rows(['创建 / 更新', `${text(value.created_at)} / ${text(value.updated_at)}`, '', '']);
}

async function mountList<T>(section: HTMLElement, title: string, columns: string[], read: (offset: number) => Promise<WeComContactHistoryPage<T>>, render: (item: T) => string[]): Promise<void> {
  const load = async (offset: number): Promise<void> => {
    section.innerHTML = `<h2 style="font-size:16px">${title}</h2><p role="status">正在读取 V1 历史…</p>`;
    try {
      const page = await read(offset);
      const range = page.items.length ? `${page.offset + 1}–${page.offset + page.items.length}` : '0';
      section.innerHTML = `<h2 style="font-size:16px">${title}</h2><div style="overflow:auto"><table style="width:100%;min-width:900px;border-collapse:collapse;font-size:13px"><thead><tr style="background:#FAFAFB;color:#667085">${columns.map((value) => `<th style="${cell}">${value}</th>`).join('')}</tr></thead><tbody>${page.items.map((item) => rows(render(item))).join('') || `<tr><td colspan="${columns.length}" style="${cell}">暂无 V1 历史记录</td></tr>`}</tbody></table></div><div style="margin-top:12px;display:flex;justify-content:space-between;gap:12px"><span>共 ${page.total} 条 · 当前 ${range}</span><div><button data-wecom-history-prev style="${button}" ${page.offset === 0 ? 'disabled' : ''}>上一页</button> <button data-wecom-history-next style="${button}" ${page.offset + page.items.length >= page.total ? 'disabled' : ''}>下一页</button></div></div>`;
      section.querySelector('[data-wecom-history-prev]')?.addEventListener('click', () => { void load(Math.max(0, page.offset - page.limit)); });
      section.querySelector('[data-wecom-history-next]')?.addEventListener('click', () => { void load(page.offset + page.limit); });
    } catch (error) {
      section.innerHTML = `<h2 style="font-size:16px">${title}</h2><p role="alert" style="color:#D83931">${esc(error instanceof Error ? error.message : '历史读取失败')}；未显示旧数据，也未回退 Mock。</p><button data-wecom-history-retry style="${button}">重新读取</button>`;
      section.querySelector('[data-wecom-history-retry]')?.addEventListener('click', () => { void load(offset); });
    }
  };
  await load(0);
}

export async function mountWeComContactHistory(stage: HTMLElement, options: Options = {}): Promise<void> {
  let kind: Kind, id: number | undefined;
  try {
    kind = options.kind === undefined || options.kind === 'event' ? 'event' : options.kind === 'relation' ? 'relation' : (() => { throw new Error('企微联系人历史类型无效'); })();
    id = historyID(options.historyID);
  } catch (error) {
    stage.innerHTML = `<p role="alert">${esc(error instanceof Error ? error.message : '历史入口无效')}；未读取数据。</p>`;
    return;
  }
  stage.innerHTML = `<div data-wecom-contact-history style="flex:1;min-height:0;overflow:auto;padding:20px;display:grid;gap:16px;align-content:start"><header><a href="config.html">返回配置中心</a><h1 style="font-size:20px;margin:12px 0 8px">V1 企微联系人历史（只读）</h1><p style="margin:0;color:#8F5A16;line-height:1.7">仅展示已封存的 V1 历史观察；不会同步、重试、发送、执行负责人迁移或触发企微 Provider。历史 source ID 不是当前 V2 客户、负责人或企微能力证明。EventTime/CreateTime 保留原整数，单位未知，不转换为日期。</p><div style="margin-top:12px">${nav(kind)}</div></header><section data-wecom-contact-history-content></section></div>`;
  const section = stage.querySelector<HTMLElement>('[data-wecom-contact-history-content]')!;
  if (id !== undefined) {
    const load = async (): Promise<void> => {
      section.innerHTML = '<p role="status">正在读取 V1 历史…</p>';
      try {
        const item = kind === 'event' ? await readWeComContactHistoryEvent(id!) : await readWeComContactHistoryRelation(id!);
        section.innerHTML = `<a href="${url(kind)}">返回历史列表</a><h2 style="font-size:16px">${kind === 'event' ? 'V1 联系人事件详情' : 'V1 客户关系详情'}</h2><div style="overflow:auto"><table style="width:100%;max-width:900px;border-collapse:collapse;font-size:13px"><tbody>${detailRows(kind, item)}</tbody></table></div>`;
      } catch (error) {
        section.innerHTML = `<a href="${url(kind)}">返回历史列表</a><p role="alert" style="color:#D83931">${esc(error instanceof Error ? error.message : '历史读取失败')}；未显示旧数据，也未回退 Mock。</p><button data-wecom-history-retry style="${button}">重新读取</button>`;
        section.querySelector('[data-wecom-history-retry]')?.addEventListener('click', () => { void load(); });
      }
    };
    await load();
    return;
  }
  if (kind === 'event') await mountList(section, 'V1 联系人事件', ['历史引用', '原事件字段', '原时间', '只读入口'], readWeComContactHistoryEvents, eventCells);
  else await mountList(section, 'V1 客户关系', ['历史引用', '原关系字段', '原时间', '只读入口'], readWeComContactHistoryRelations, relationCells);
}
