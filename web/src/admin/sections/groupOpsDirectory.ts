import { readGroupOpsDirectory, readGroupOpsOwners, refreshGroupOpsDirectory } from '../../api/groupOpsDirectory';
import type { GroupOpsDirectoryPage } from '../../api/generated/health.schemas';
import { confirmBox } from '../../shared/ui/feedback';
import { esc } from './util';

/** Selection only edits the form; the existing Save path owns CAS binding/removal. */
export function openGroupOpsDirectory(options: { selected?: string[] } = {}): Promise<string[] | null> {
  return new Promise((resolve) => {
    const selected = new Set(options.selected || []);
    const labels = new Map<string, string>();
    const mask = document.createElement('div');
    mask.id = 'group-directory';
    mask.className = 'group-ops-picker__mask';
    const editable = options.selected !== undefined;
    let owners: Array<{ staffId: number; name: string }> = [];
    let owner = 0, offset = 0, generation = 0;
    let loading = true, closed = false, error = '', notice = '', refreshKey = '', query = '';
    let current: GroupOpsDirectoryPage | null = null;
    const finish = (value: string[] | null): void => { closed = true; generation++; mask.remove(); resolve(value); };
    const button = (action: string, label: string, disabled = false, primary = false): string => `<button class="${primary ? 'group-ops-picker__primary' : 'group-ops-picker__button'}" data-gd="${action}" ${disabled ? 'disabled' : ''}>${label}</button>`;
    function render(): void {
      if (closed) return;
      const visibleItems = (current?.items || []).filter((item) => !query || `${item.display_name} ${item.chat_reference}`.toLowerCase().includes(query.toLowerCase()));
      mask.innerHTML = `<section class="group-ops-picker" role="dialog" aria-modal="true" aria-label="可管理群目录">
        <header class="group-ops-picker__header"><div><strong>选择群聊</strong><p>从运营成员的真实群目录中选择</p></div>${button('close', '关闭')}</header>
        <p class="group-ops-picker__warning">当前列表是本地目录快照，不证明当前企微权限或群消息送达。刷新可能读取企微名下群并更新本地目录；不触发群发。</p>
        <div class="group-ops-picker__toolbar">
          <label>运营成员<select data-gd="owner" ${loading ? 'disabled' : ''}><option value="">请选择负责人</option>${owners.map((item) => `<option value="${item.staffId}" ${item.staffId === owner ? 'selected' : ''}>${esc(item.name)} · staff_id=${item.staffId}</option>`).join('')}</select></label>
          <label>搜索群聊<input data-gd-search type="search" value="${esc(query)}" placeholder="输入群名称或群聊 ID"></label>
          <div class="group-ops-picker__actions">${button('read', '重读本地目录', loading || !owner)} ${button('refresh', '刷新此成员名下群', loading || !owner)}</div>
        </div>
        <div class="group-ops-picker__status" role="status">${esc(loading ? '正在读取…' : error || notice || (!owners.length ? '暂无可信运营成员，无法读取或刷新目录' : !owner ? '请选择负责人后读取本地目录' : ''))}</div>
        <div class="group-ops-picker__body">
          <div class="group-ops-picker__results">
            ${current ? (visibleItems.length ? visibleItems.map((item) => `<label class="group-ops-picker__card"><span class="group-ops-picker__check">${editable ? `<input type="checkbox" aria-label="选择 ${esc(item.display_name)}" data-gd-ref="${esc(item.chat_reference)}" ${selected.has(item.chat_reference) ? 'checked' : ''}>` : '只读'}</span><span><strong>${esc(item.display_name)}</strong><small>${esc(item.chat_reference)}</small></span><span class="group-ops-picker__meta">${item.member_count} 人<br>${esc(item.refreshed_at)}</span></label>`).join('') : `<p class="group-ops-picker__empty">${query ? '没有匹配的群聊' : '该成员本地目录为空；不等于企微没有群'}</p>`) : '<p class="group-ops-picker__empty">选择运营成员后读取群目录</p>'}
            ${current ? `<footer class="group-ops-picker__pagination"><span>共 ${current.total} 条</span><div>${button('prev', '上一页', loading || offset === 0)} ${button('next', '下一页', loading || !current.has_more)}</div></footer>` : ''}
          </div>
          ${editable ? `<aside class="group-ops-picker__selected"><strong>待保存群选择（${selected.size}）</strong><p>切换负责人和翻页会保留选择。未确认的原引用仅在显式移除时取消绑定。</p><div>${[...selected].map((ref) => `<div class="group-ops-picker__chip"><span>${esc(labels.get(ref) || '未在已加载目录确认')}<small>${esc(ref)}</small></span><button data-gd-remove="${esc(ref)}">移除</button></div>`).join('') || '<p class="group-ops-picker__empty">尚未选择群聊</p>'}</div>${button('apply', '使用此选择（仍需保存计划）', false, true)}</aside>` : ''}
        </div>
      </section>`;
      mask.querySelectorAll<HTMLElement>('button,input,select').forEach((element) => { (element as HTMLElement & { __dcBound?: boolean }).__dcBound = true; });
      mask.querySelector<HTMLSelectElement>('[data-gd="owner"]')!.onchange = (event) => {
        owner = Number((event.target as HTMLSelectElement).value); offset = 0; refreshKey = ''; current = null; notice = ''; error = '';
        if (owner) void read(); else render();
      };
      const search = mask.querySelector<HTMLInputElement>('[data-gd-search]');
      if (search) search.oninput = () => { query = search.value; render(); mask.querySelector<HTMLInputElement>('[data-gd-search]')?.focus(); };
      mask.querySelectorAll<HTMLButtonElement>('[data-gd]').forEach((element) => {
        element.onclick = () => {
          switch (element.dataset.gd) {
            case 'close': finish(null); break;
            case 'apply': finish([...selected]); break;
            case 'read': void read(); break;
            case 'prev': offset -= 50; void read(); break;
            case 'next': offset += 50; void read(); break;
            case 'refresh': {
              const chosenOwner = owner;
              confirmBox('刷新企微群目录', '此操作可能实际读取所选成员名下的企微群并更新本地快照；不发送群消息。仅在已获本次目录读取授权时继续。当前响应不提供 Provider 读取回执。', '确认读取目录', false, () => { if (!closed && owner === chosenOwner && !loading) void read(true); });
              break;
            }
          }
        };
      });
      mask.querySelectorAll<HTMLInputElement>('[data-gd-ref]').forEach((element) => { element.onchange = () => { const ref = element.dataset.gdRef!; if (element.checked) selected.add(ref); else selected.delete(ref); render(); }; });
      mask.querySelectorAll<HTMLButtonElement>('[data-gd-remove]').forEach((element) => { element.onclick = () => { selected.delete(element.dataset.gdRemove!); render(); }; });
    }
    async function read(refresh = false): Promise<void> {
      const request = ++generation;
      loading = true; error = ''; notice = ''; current = null; render();
      try {
        if (refresh && !refreshKey) refreshKey = crypto.randomUUID();
        const result = refresh ? await refreshGroupOpsDirectory(owner, refreshKey) : await readGroupOpsDirectory(owner, 50, offset);
        if (closed || request !== generation) return;
        current = result; offset = result.offset;
        result.items.forEach((item) => labels.set(item.chat_reference, item.display_name));
        if (refresh) { refreshKey = ''; notice = '服务器返回目录快照；不代表 Provider 接受或消息送达，当前响应无 Provider 读取回执。'; }
      } catch { if (!closed && request === generation) error = '目录读取失败，未使用 Mock 或旧页数据。可重读本地目录；刷新重试须再次确认。'; }
      finally { if (!closed && request === generation) { loading = false; render(); } }
    }
    document.body.appendChild(mask); render();
    void readGroupOpsOwners().then((value) => { owners = value; }).catch(() => { error = '运营成员读取失败，未使用 Mock；请关闭后重试'; }).finally(() => { loading = false; render(); });
  });
}
