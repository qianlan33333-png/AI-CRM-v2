/**
 * 漏斗 / 数据看板 —— 多维表格模块（方向稿 funnel-grid.html 的 TypeScript 移植）
 * 能力：视图系统（草稿 / 未保存提醒 / 保存 / 另存 / 重命名 / 副本 / 删除）、
 *       筛选条件构建器（文本/枚举/布尔/数字/日期五种）、分组（可折叠）、
 *       排序（表头点击切换）、全局搜索、行勾选、分享浮窗、群发浮窗、CSV 导出。
 * 行数据经 AdminApi 全量加载，筛选/分组/排序在前端视图层完成（与原型一致）。
 */
import type { AdminApi } from '../../shared/api/client';
import type { FunnelField, FunnelFieldType, FunnelFilter, FunnelGridRow, FunnelView, Tone } from '../../shared/api/types';
import { toast } from '../../shared/ui/feedback';
import { openPicker, type PickerItem, type PickerKind } from '../../shared/ui/picker';
import { downloadCsv } from '../../shared/ui/download';
import { esc, copyText } from './util';

/* ================= 字段定义（与生产 Tabulator 列对齐，取代表性子集） ================= */
const FIELDS: FunnelField[] = [
  { key: 'mobile_masked', title: '脱敏手机号', type: 'text', w: 120, frozen: 0 },
  { key: 'customer_name', title: '客户名', type: 'text', w: 128, frozen: 1 },
  { key: 'funnel_label', title: '漏斗状态', type: 'enum', w: 130, frozen: 2 },
  { key: 'external_userid', title: '企微外部联系人ID', type: 'text', w: 170 },
  { key: 'owner_userid', title: '企微跟进人', type: 'enum', w: 110 },
  { key: 'in_lead_pool', title: '在销售线索池', type: 'bool', w: 100 },
  { key: 'in_questionnaire', title: '填过激活问卷', type: 'bool', w: 110 },
  { key: 'questionnaire_count', title: '问卷提交次数', type: 'number', w: 110 },
  { key: 'last_questionnaire_at', title: '最近一次提交问卷', type: 'date', w: 150 },
  { key: 'is_wecom_added', title: '是否已加企微', type: 'bool', w: 100 },
  { key: 'class_term_label', title: '班期标签', type: 'enum', w: 120 },
  { key: 'first_entry_source', title: '首次入口来源', type: 'enum', w: 130 },
  { key: 'crm_hxc_state', title: 'CRM 标注激活态', type: 'enum', w: 130 },
  { key: 'hxc_member_status', title: '用户会员状态', type: 'enum', w: 110 },
  { key: 'membership_type', title: '会员类型', type: 'enum', w: 100 },
  { key: 'membership_days_left', title: '会员剩余天数', type: 'number', w: 110 },
  { key: 'hxc_silent_days', title: '沉默天数', type: 'number', w: 90 },
  { key: 'msg_user', title: '用户消息数', type: 'number', w: 100 },
  { key: 'msg_ai', title: 'AI 回复数', type: 'number', w: 100 },
  { key: 'last_msg_at', title: '最后活跃时间', type: 'date', w: 140 },
];

const ENUMS: Record<string, string[]> = {
  funnel_label: ['已激活并打开', '仅激活未打开', '注册但无会员', '未激活'],
  owner_userid: ['LinKaiYan', 'ZhangMin', 'LiYou', 'HuangYouCan', '—'],
  class_term_label: ['8.6 期', '8.13 期', '8.20 期', '体验营', ''],
  first_entry_source: ['视频号', '公众号', '雷达链接', '好友推荐', '直播', '线下'],
  crm_hxc_state: ['已激活', '未激活', '疑似激活'],
  hxc_member_status: ['正常', '过期', '未开通', ''],
  membership_type: ['年卡', '季卡', '月卡', '体验卡', ''],
};

const FUNNEL_TONE: Record<string, Tone> = { 已激活并打开: 'ok', 仅激活未打开: 'warn', 注册但无会员: 'red', 未激活: 'gray' };

const OPS: Record<FunnelFieldType, string[]> = {
  text: ['包含', '等于', '不等于'],
  enum: ['等于', '不等于'],
  bool: ['等于'],
  number: ['≥', '≤', '等于'],
  date: ['晚于', '早于'],
};

const fieldOf = (key: string): FunnelField => FIELDS.find((f) => f.key === key) as FunnelField;

/* ================= 数据管道 ================= */
function match(r: FunnelGridRow, f: FunnelFilter): boolean {
  const v = r[f.field];
  const fd = fieldOf(f.field);
  if (f.value === '' && fd.type !== 'enum') return true;
  switch (f.op) {
    case '包含':
      return String(v || '').includes(f.value);
    case '等于':
      return String(v ?? '') === String(f.value);
    case '不等于':
      return String(v ?? '') !== String(f.value);
    case '≥':
      return Number(v || 0) >= Number(f.value);
    case '≤':
      return Number(v || 0) <= Number(f.value);
    case '晚于':
      return String(v || '') >= f.value;
    case '早于':
      return String(v || '') <= f.value && v !== '';
  }
  return true;
}

/* ================= 入口 ================= */
export interface FunnelGridOpts {
  /** 周期商品「数据」二级页：传入商品后头部/统计卡切换为商品语境 */
  product?: { code: string; name: string; price: string; status: string };
}

export async function mountFunnelGrid(root: HTMLElement, api: AdminApi, opts?: FunnelGridOpts): Promise<void> {
  if (api.mode === 'http') {
    root.className = 'labs sec-funnel';
    root.innerHTML = '<div class="card" style="margin:24px;padding:32px;text-align:center;color:#8F959E"><strong style="display:block;color:#1F2329;margin-bottom:8px">后端能力未就绪</strong>当前漏斗行/视图 DTO 与 Member Grid schema/query/views 契约不等价，页面未发送漏斗请求。</div>';
    return;
  }
  const [rows, views] = await Promise.all([api.listFunnelRows(), api.listFunnelViews()]);
  root.className = 'labs sec-funnel';

  const product = opts?.product;
  const crumbHtml = product
    ? `客户管理后台 / 交易 / <a href="spProducts.html" style="color:inherit;text-decoration:none">周期商品管理</a> / <b>${esc(product.name)} · 数据</b>`
    : `客户管理后台 / 运营 / <b>漏斗 / 数据看板</b>`;
  const titleText = product ? `${esc(product.name)} · 会员数据` : '漏斗 / 数据看板';
  const descText = product
    ? '该周期商品购买会员的多维数据表：视图 / 筛选 / 分组 / 排序 / 分享 · 每行 = 1 个手机号'
    : '借鉴周期商品「会员数据表」的多维视图能力：视图 / 筛选 / 分组 / 排序 / 分享 · 每行 = 1 个手机号';
  const statsHtml = product
    ? `<div class="card stat"><div class="stat-l">商品编码</div><div class="stat-v" style="font-size:15px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace">${esc(product.code)}</div><div class="stat-s">周期商品</div></div>
      <div class="card stat"><div class="stat-l">价格</div><div class="stat-v">${esc(product.price)}</div><div class="stat-s">售卖价</div></div>
      <div class="card stat ${product.status === '已上架' ? 'ok' : 'gray'}"><div class="stat-l">状态</div><div class="stat-v" style="color:${product.status === '已上架' ? '#2EA121' : '#646A73'}">${esc(product.status)}</div><div class="stat-s">上下架状态</div></div>
      <div class="card stat blue"><div class="stat-l">会员总数</div><div class="stat-v" style="color:#245BDB">${rows.length.toLocaleString()}</div><div class="stat-s">本商品会员数据行</div></div>`
    : `<div class="card stat"><div class="stat-l">总数（基础人群）</div><div class="stat-v">18,204</div><div class="stat-s">CRM 三表手机号并集</div></div>
      <div class="card stat ok"><div class="stat-l">已激活并打开</div><div class="stat-v" style="color:#2EA121">6,412</div><div class="stat-s">会员开通 + 真打开过小程序</div></div>
      <div class="card stat warn"><div class="stat-l">仅激活未打开（拉回重点）</div><div class="stat-v" style="color:#D97917">1,286</div><div class="stat-s">有会员资格 / 从未登录</div></div>
      <div class="card stat blue"><div class="stat-l">注册但无会员</div><div class="stat-v" style="color:#D83931">143</div><div class="stat-s">异常：登录过但会员缺失</div></div>
      <div class="card stat gray"><div class="stat-l">未激活</div><div class="stat-v" style="color:#646A73">10,363</div><div class="stat-s">在 CRM 但未开通</div></div>`;

  /* ---------- 状态 ---------- */
  let cur = 0;
  let draft: FunnelView | null = null;
  let openPanel: '' | 'filter' | 'group' | 'sort' = '';
  const collapsed = new Set<string>();
  const selected = new Set<string>();
  let search = '';
  let nameCb: ((n: string) => void) | null = null;

  const cfg = (): FunnelView => {
    if (!draft) draft = JSON.parse(JSON.stringify(views[cur])) as FunnelView;
    return draft;
  };
  const dirty = (): boolean => JSON.stringify(draft) !== JSON.stringify(views[cur]);

  root.innerHTML = `
    <div class="crumb">${crumbHtml}</div>
    <div class="page-head">
      <div><div class="page-title">${titleText}</div><div class="page-desc">${descText}</div></div>
      <button class="btn primary" id="btnRefresh">立即刷新</button>
    </div>

    <div class="stats">
      ${statsHtml}
    </div>

    <div class="card" style="padding:0">
      <div class="viewbar">
        <div id="viewTabs" style="display:flex;align-items:center;gap:2px;overflow:auto"></div>
        <button class="vtab-add" id="btnAddView" title="新建视图">＋</button>
        <button class="icon-btn vtab-menu" id="btnViewMenu" title="当前视图菜单">···</button>
        <button class="btn sm" id="btnShare" style="margin-left:6px">🌐 分享</button>
        <div class="menu-pop" id="viewMenu">
          <div id="vmRename">重命名视图</div>
          <div id="vmDup">另存为副本</div>
          <div class="red" id="vmDel">删除视图</div>
        </div>
      </div>

      <div class="grid-toolbar">
        <button class="tool-btn" id="btnFilter">⏷ 筛选 <span class="tool-count" id="cFilter" hidden></span></button>
        <button class="tool-btn" id="btnGroup">▦ 分组 <span class="tool-count" id="cGroup" hidden>1</span></button>
        <button class="tool-btn" id="btnSort">↕ 排序 <span class="tool-count" id="cSort" hidden>1</span></button>
        <div class="draft">
          <span class="draft-text" id="draftText" hidden>有未保存的视图修改</span>
          <button class="btn sm" id="btnSaveAs" hidden>另存为</button>
          <button class="btn sm primary" id="btnSaveView" hidden>保存视图</button>
        </div>
      </div>

      <div class="config-panel" id="panelFilter">
        <div class="panel-title">满足以下所有条件的行才会显示：</div>
        <div id="condList"></div>
        <button class="cond-add" id="btnAddCond">＋ 添加条件</button>
      </div>
      <div class="config-panel" id="panelGroup">
        <div class="panel-title">按字段分组（组可折叠）：</div>
        <div class="cond-row">
          <select class="select" id="groupField"><option value="">不分组</option></select>
          <span class="muted" style="font-size:12px">分组后按组名排序，组头显示行数</span>
        </div>
      </div>
      <div class="config-panel" id="panelSort">
        <div class="panel-title">排序规则：</div>
        <div class="cond-row">
          <select class="select" id="sortField"></select>
          <select class="select" id="sortDir" style="min-width:90px"><option value="desc">从高到低</option><option value="asc">从低到高</option></select>
          <button class="cond-add" id="btnClearSort" style="color:#D83931">清除排序</button>
        </div>
      </div>

      <div class="grid-meta">
        <span id="resultSummary">—</span>
        <span>上次刷新: <b>2026-08-05 02:10</b> · 状态: <b style="color:#2EA121">success</b> · 列可横向滚动，前 3 列冻结</span>
      </div>
      <div class="grid-scroll" id="gridScroll">
        <table class="grid" id="grid">
          <thead id="gridHead"></thead>
          <tbody id="gridBody"></tbody>
        </table>
      </div>
    </div>

    <div class="card" style="margin-top:14px;padding:12px 16px;display:flex;align-items:center;gap:10px;flex-wrap:wrap">
      <input class="input" id="globalSearch" placeholder="全局搜索（姓名 / 昵称 / 手机尾号 / 企微 ID…）" style="width:280px">
      <span class="muted" style="font-size:12px" id="selInfo">未选中行</span>
      <div style="flex:1"></div>
      <button class="btn primary" id="btnBroadcast">群发选中用户</button>
      <button class="btn" id="btnSenders">发送人管理</button>
      <button class="btn" id="btnCsv">导出 CSV（筛选后）</button>
    </div>

    <div class="mask" id="nameMask">
      <div class="modal narrow">
        <div class="modal-head"><span id="nameTitle">新建视图</span><button class="modal-x" data-close>×</button></div>
        <div class="modal-body"><div style="display:grid;gap:6px"><label style="font-size:12px;color:#646A73">视图名称</label><input class="input" id="viewNameInput" maxlength="30" placeholder="例如：本周拉回名单"></div></div>
        <div class="modal-foot"><button class="btn" data-close>取消</button><button class="btn primary" id="nameOk">确认</button></div>
      </div>
    </div>

    <div class="mask" id="shareMask">
      <div class="modal">
        <div class="modal-head"><div><div class="share-eyebrow">漏斗数据工作区</div><span>分享数据</span></div><button class="modal-x" data-close>×</button></div>
        <div class="modal-body" style="padding-top:6px">
          <div class="share-sec">
            <h3>邀请协作者</h3><p>从已同步的企微员工目录邀请，可设置为可查看或可编辑。</p>
            <div style="display:flex;gap:8px;margin-top:10px"><div style="flex:1;display:flex;align-items:center;height:34px;padding:0 10px;border:1px solid #DEE0E3;border-radius:6px;font-size:13px;color:#8F959E">通过「选择客服」组件邀请</div><button class="btn" id="btnInvite">选择客服</button></div>
            <div id="collabList" style="margin-top:8px">
              <div class="collab"><div class="collab-av">黄</div><div style="flex:1"><b style="font-size:13px">HuangYouCan</b><div class="muted" style="font-size:12px">所有者</div></div><span class="chip gray">可编辑</span></div>
              <div class="collab"><div class="collab-av" style="background:linear-gradient(135deg,#34C19B,#00A870)">林</div><div style="flex:1"><b style="font-size:13px">林小楷 LinKaiYan</b><div class="muted" style="font-size:12px">增长顾问</div></div><span class="chip blue">可查看</span></div>
            </div>
          </div>
          <div class="share-sec">
            <div style="display:flex;align-items:center;justify-content:space-between;gap:10px">
              <div><h3>外部分享</h3><p>获得链接的人可免登录查看数据与视图，无法编辑或改筛选。</p></div>
              <div class="sw" id="swShare"></div>
            </div>
            <div id="shareLinkRow" hidden style="margin-top:12px;display:flex;gap:8px">
              <input class="input" id="shareUrl" readonly style="flex:1"><button class="btn" id="btnCopyShare">复制链接</button>
            </div>
            <p id="shareHint" style="margin-top:8px">外部分享未开启</p>
          </div>
        </div>
      </div>
    </div>

    <div class="mask" id="bcMask">
      <div class="modal">
        <div class="modal-head"><span>群发选中用户</span><button class="modal-x" data-close>×</button></div>
        <div class="modal-body" style="display:grid;gap:14px">
          <div class="muted" id="bcInfo" style="font-size:12px;line-height:1.7">—</div>
          <div style="display:grid;gap:6px"><label style="font-size:12px;color:#646A73">文本内容</label>
            <textarea class="input" id="bcText" rows="4" style="height:auto;padding:10px;resize:vertical" placeholder="支持 {{客户名}} 等变量插入"></textarea></div>
          <div style="display:flex;gap:8px;flex-wrap:wrap">
            <button class="btn sm" data-mat="image">＋ 图片</button>
            <button class="btn sm" data-mat="mp">＋ 小程序</button>
            <button class="btn sm" data-mat="attach">＋ 附件</button>
          </div>
          <div id="bcMats" style="display:flex;gap:6px;flex-wrap:wrap"></div>
          <div class="muted" style="font-size:12px">发送人：HuangYouCan（在「发送人管理」中可调整优先级）</div>
        </div>
        <div class="modal-foot"><button class="btn" data-close>取消</button><button class="btn primary" id="bcOk">创建群发任务</button></div>
      </div>
    </div>`;

  const $ = <T extends HTMLElement>(s: string): T => root.querySelector(s) as T;

  /* ---------- 视图栏 ---------- */
  function renderTabs(): void {
    $('#viewTabs').innerHTML = views
      .map((v, i) => `<div class="vtab ${i === cur ? 'on' : ''}" data-tab="${i}">${esc(v.name)}</div>`)
      .join('');
    const d = dirty();
    ($('#draftText') as HTMLElement).hidden = !d;
    ($('#btnSaveView') as HTMLElement).hidden = !d;
    ($('#btnSaveAs') as HTMLElement).hidden = !d;
    const c = cfg();
    ($('#cFilter') as HTMLElement).hidden = !c.filters.length;
    $('#cFilter').textContent = String(c.filters.length);
    ($('#cGroup') as HTMLElement).hidden = !c.group;
    ($('#cSort') as HTMLElement).hidden = !c.sort;
    $('#btnFilter').classList.toggle('on', !!c.filters.length);
    $('#btnGroup').classList.toggle('on', !!c.group);
    $('#btnSort').classList.toggle('on', !!c.sort);
  }

  function openNameMask(title: string, value: string, cb: (n: string) => void): void {
    $('#nameTitle').textContent = title;
    ($('#viewNameInput') as HTMLInputElement).value = value;
    nameCb = cb;
    $('#nameMask').classList.add('open');
    ($('#viewNameInput') as HTMLInputElement).focus();
  }

  function persistViews(): void {
    void api.saveFunnelViews(views);
  }

  $('#btnAddView').addEventListener('click', () =>
    openNameMask('新建视图', '', (n) => {
      views.push({ name: n, filters: [], group: '', sort: null });
      cur = views.length - 1;
      draft = null;
      collapsed.clear();
      persistViews();
      sync();
    }),
  );
  $('#nameOk').addEventListener('click', () => {
    const n = ($('#viewNameInput') as HTMLInputElement).value.trim();
    if (!n) return toast('请输入视图名称', true);
    $('#nameMask').classList.remove('open');
    if (nameCb) nameCb(n);
  });
  $('#btnViewMenu').addEventListener('click', (e) => {
    e.stopPropagation();
    $('#viewMenu').classList.toggle('open');
  });
  document.addEventListener('click', () => $('#viewMenu').classList.remove('open'));
  $('#vmRename').addEventListener('click', () =>
    openNameMask('重命名视图', views[cur].name, (n) => {
      views[cur].name = n;
      persistViews();
      sync();
    }),
  );
  $('#vmDup').addEventListener('click', () => {
    views.splice(cur + 1, 0, JSON.parse(JSON.stringify({ ...views[cur], name: views[cur].name + ' 副本' })) as FunnelView);
    cur++;
    draft = null;
    persistViews();
    sync();
    toast('已创建副本');
  });
  $('#vmDel').addEventListener('click', () => {
    if (views.length <= 1) return toast('至少保留一个视图', true);
    views.splice(cur, 1);
    cur = Math.max(0, cur - 1);
    draft = null;
    persistViews();
    sync();
  });
  $('#btnSaveView').addEventListener('click', () => {
    views[cur] = JSON.parse(JSON.stringify(draft)) as FunnelView;
    draft = null;
    persistViews();
    sync();
    toast('视图已保存');
  });
  $('#btnSaveAs').addEventListener('click', () =>
    openNameMask('另存为新视图', views[cur].name + ' 副本', (n) => {
      views.push(JSON.parse(JSON.stringify({ ...cfg(), name: n })) as FunnelView);
      cur = views.length - 1;
      draft = null;
      persistViews();
      sync();
    }),
  );

  /* ---------- 面板开关 ---------- */
  function togglePanel(which: '' | 'filter' | 'group' | 'sort'): void {
    openPanel = openPanel === which ? '' : which;
    $('#panelFilter').classList.toggle('open', openPanel === 'filter');
    $('#panelGroup').classList.toggle('open', openPanel === 'group');
    $('#panelSort').classList.toggle('open', openPanel === 'sort');
    if (openPanel === 'filter') renderConds();
    if (openPanel === 'group') renderGroupPanel();
    if (openPanel === 'sort') renderSortPanel();
  }
  $('#btnFilter').addEventListener('click', () => togglePanel('filter'));
  $('#btnGroup').addEventListener('click', () => togglePanel('group'));
  $('#btnSort').addEventListener('click', () => togglePanel('sort'));

  function fieldOptions(sel: string): string {
    return FIELDS.map((f) => `<option value="${f.key}" ${sel === f.key ? 'selected' : ''}>${f.title}</option>`).join('');
  }

  function renderConds(): void {
    const c = cfg();
    $('#condList').innerHTML =
      c.filters
        .map((f, i) => {
          const fd = fieldOf(f.field);
          const ops = OPS[fd.type] || OPS.text;
          const valueInput =
            fd.type === 'enum'
              ? `<select class="select" data-cv="${i}"><option value="">（空）</option>${ENUMS[f.field].map((v) => `<option ${v === f.value ? 'selected' : ''}>${v}</option>`).join('')}</select>`
              : fd.type === 'bool'
                ? `<select class="select" data-cv="${i}"><option ${f.value === '✓' ? 'selected' : ''}>✓</option><option ${f.value === '✗' ? 'selected' : ''}>✗</option></select>`
                : `<input class="input" data-cv="${i}" value="${esc(f.value)}" placeholder="${fd.type === 'date' ? 'YYYY-MM-DD' : '值'}">`;
          return `<div class="cond-row">
            <select class="select" data-cf="${i}">${fieldOptions(f.field)}</select>
            <select class="select" data-co="${i}" style="min-width:80px">${ops.map((o) => `<option ${o === f.op ? 'selected' : ''}>${o}</option>`).join('')}</select>
            ${valueInput}
            <button class="icon-btn" data-cd="${i}" title="删除">×</button>
          </div>`;
        })
        .join('') || '<div class="muted" style="font-size:12px;margin-bottom:8px">暂无条件</div>';
  }
  $('#btnAddCond').addEventListener('click', () => {
    cfg().filters.push({ field: 'funnel_label', op: '等于', value: '' });
    renderConds();
    applyDraft();
  });

  root.addEventListener('change', (e) => {
    const t = e.target as HTMLElement;
    const cf = t.closest('[data-cf]') as HTMLSelectElement | null;
    const co = t.closest('[data-co]') as HTMLSelectElement | null;
    const cv = t.closest('[data-cv]') as HTMLSelectElement | HTMLInputElement | null;
    const ck = t.closest('[data-ck]') as HTMLInputElement | null;
    if (cf) {
      const f = cfg().filters[Number(cf.dataset.cf)];
      f.field = cf.value;
      f.op = OPS[fieldOf(cf.value).type][0];
      f.value = '';
      renderConds();
      applyDraft();
    }
    if (co) {
      cfg().filters[Number(co.dataset.co)].op = co.value;
      applyDraft();
    }
    if (cv && cv.tagName === 'SELECT') {
      cfg().filters[Number(cv.dataset.cv)].value = cv.value;
      applyDraft();
    }
    if (ck) {
      if (ck.checked) selected.add(ck.dataset.ck!);
      else selected.delete(ck.dataset.ck!);
      renderGrid();
    }
  });
  root.addEventListener('input', (e) => {
    const cv = (e.target as HTMLElement).closest('[data-cv]') as HTMLInputElement | null;
    if (cv && cv.tagName === 'INPUT') {
      cfg().filters[Number(cv.dataset.cv)].value = cv.value;
      applyDraft();
    }
  });

  function renderGroupPanel(): void {
    $('#groupField').innerHTML =
      '<option value="">不分组</option>' +
      FIELDS.map((f) => `<option value="${f.key}" ${cfg().group === f.key ? 'selected' : ''}>${f.title}</option>`).join('');
  }
  $('#groupField').addEventListener('change', (e) => {
    cfg().group = (e.target as HTMLSelectElement).value;
    applyDraft();
  });
  function renderSortPanel(): void {
    $('#sortField').innerHTML = FIELDS.map(
      (f) => `<option value="${f.key}" ${cfg().sort?.field === f.key ? 'selected' : ''}>${f.title}</option>`,
    ).join('');
    ($('#sortDir') as HTMLSelectElement).value = cfg().sort?.dir || 'desc';
  }
  $('#sortField').addEventListener('change', (e) => {
    cfg().sort = { field: (e.target as HTMLSelectElement).value, dir: ($('#sortDir') as HTMLSelectElement).value as 'asc' | 'desc' };
    applyDraft();
  });
  $('#sortDir').addEventListener('change', (e) => {
    const s = cfg().sort;
    if (s) {
      s.dir = (e.target as HTMLSelectElement).value as 'asc' | 'desc';
      applyDraft();
    }
  });
  $('#btnClearSort').addEventListener('click', () => {
    cfg().sort = null;
    renderSortPanel();
    applyDraft();
  });

  /* ---------- 数据管道 ---------- */
  function pipeline(): FunnelGridRow[] {
    let out = rows.filter((r) => cfg().filters.every((f) => match(r, f)));
    if (search) out = out.filter((r) => Object.values(r).some((x) => String(x ?? '').toLowerCase().includes(search)));
    const s = cfg().sort;
    if (s) {
      const fd = fieldOf(s.field);
      out = [...out].sort((a, b) => {
        const va = a[s.field];
        const vb = b[s.field];
        const cmp =
          fd.type === 'number' ? Number(va || 0) - Number(vb || 0) : String(va ?? '').localeCompare(String(vb ?? ''), 'zh');
        return s.dir === 'asc' ? cmp : -cmp;
      });
    }
    return out;
  }

  /* ---------- 渲染网格 ---------- */
  function cellHtml(f: FunnelField, r: FunnelGridRow): string {
    const v = r[f.key];
    if (f.key === 'funnel_label') return `<span class="chip ${FUNNEL_TONE[String(v)] || 'gray'}">${esc(v)}</span>`;
    if (f.type === 'bool') return v === '✓' ? '<span class="tick-ok">✓</span>' : '<span class="tick-no">✗</span>';
    if (f.type === 'number') return `<span class="num">${Number(v || 0).toLocaleString()}</span>`;
    return esc(v) || '<span class="muted">—</span>';
  }

  function rowHtml(r: FunnelGridRow, i: number): string {
    const rid = String(r.mobile_masked);
    return `<tr class="${i % 2 ? 'zebra' : ''}" data-row="${esc(rid)}">
      ${FIELDS.map((f) => {
        const first =
          f.key === 'mobile_masked'
            ? `<input type="checkbox" class="row-check" data-ck="${esc(rid)}" ${selected.has(rid) ? 'checked' : ''} style="margin-right:8px;vertical-align:-2px">`
            : '';
        return `<td class="${f.frozen != null ? 'frozen f' + f.frozen : ''}">${first}${cellHtml(f, r)}</td>`;
      }).join('')}
    </tr>`;
  }

  function renderGrid(): void {
    const list = pipeline();
    const s = cfg().sort;
    $('#gridHead').innerHTML = `<tr>${FIELDS.map(
      (f) =>
        `<th class="${f.frozen != null ? 'frozen f' + f.frozen : ''}" style="min-width:${f.w}px" data-th="${f.key}">${f.title}${
          s && s.field === f.key ? `<span class="sort-mark">${s.dir === 'asc' ? '↑' : '↓'}</span>` : ''
        }</th>`,
    ).join('')}</tr>`;
    let html = '';
    if (cfg().group) {
      const gf = cfg().group;
      const gtitle = fieldOf(gf).title;
      const groups: Record<string, FunnelGridRow[]> = {};
      list.forEach((r) => {
        const k = String(r[gf] || '（空）');
        (groups[k] = groups[k] || []).push(r);
      });
      Object.keys(groups)
        .sort((a, b) => a.localeCompare(b, 'zh'))
        .forEach((k) => {
          const gid = gf + '::' + k;
          const isC = collapsed.has(gid);
          html += `<tr class="ghead ${isC ? 'collapsed' : ''}" data-g="${esc(gid)}"><td colspan="${FIELDS.length}"><span class="gtri">▾</span>${esc(gtitle)}：${esc(k)}<span class="muted" style="font-weight:400">（${groups[k].length} 行）</span></td></tr>`;
          if (!isC) groups[k].forEach((r, i) => (html += rowHtml(r, i)));
        });
    } else {
      html = list.map((r, i) => rowHtml(r, i)).join('');
    }
    $('#gridBody').innerHTML =
      html || `<tr><td colspan="${FIELDS.length}" style="text-align:center;padding:40px;color:#8F959E">没有符合当前视图条件的数据</td></tr>`;
    $('#resultSummary').textContent = `共 ${list.length} 行 · ${cfg().group ? '已按「' + fieldOf(cfg().group).title + '」分组' : '未分组'}`;
    $('#selInfo').textContent = selected.size ? `已选 ${selected.size} 行` : '点击行可选中（用于群发）';
  }

  /* ---------- 交互绑定 ---------- */
  root.addEventListener('click', (e) => {
    const t = e.target as HTMLElement;
    const cd = t.closest('[data-cd]') as HTMLElement | null;
    if (cd) {
      cfg().filters.splice(Number(cd.dataset.cd), 1);
      renderConds();
      applyDraft();
      return;
    }
    const tab = t.closest('[data-tab]') as HTMLElement | null;
    if (tab) {
      if (dirty() && !window.confirm('当前视图有未保存的修改，切换将放弃。继续？')) return;
      cur = Number(tab.dataset.tab);
      draft = null;
      collapsed.clear();
      openPanel = '';
      togglePanel('');
      sync();
      return;
    }
    const g = t.closest('[data-g]') as HTMLElement | null;
    if (g) {
      const k = g.dataset.g!;
      if (collapsed.has(k)) collapsed.delete(k);
      else collapsed.add(k);
      renderGrid();
      return;
    }
    const th = t.closest('[data-th]') as HTMLElement | null;
    if (th) {
      const f = th.dataset.th!;
      const c = cfg();
      if (c.sort && c.sort.field === f) c.sort.dir = c.sort.dir === 'desc' ? 'asc' : 'desc';
      else c.sort = { field: f, dir: 'desc' };
      applyDraft();
    }
  });

  function applyDraft(): void {
    renderTabs();
    renderGrid();
  }
  function sync(): void {
    renderTabs();
    renderGrid();
  }

  $('#globalSearch').addEventListener('input', (e) => {
    search = (e.target as HTMLInputElement).value.trim().toLowerCase();
    renderGrid();
  });
  $('#btnRefresh').addEventListener('click', () => {
    const b = $('#btnRefresh') as HTMLButtonElement;
    b.disabled = true;
    b.textContent = '刷新中…';
    setTimeout(() => {
      b.disabled = false;
      b.textContent = '立即刷新';
      toast('刷新成功');
    }, 700);
  });

  /* ---------- 导出 ---------- */
  $('#btnCsv').addEventListener('click', () => {
    const list = pipeline();
    downloadCsv(
      'hxc-dashboard-filtered.csv',
      FIELDS.map((f) => f.title),
      list.map((r) => FIELDS.map((f) => r[f.key] ?? '')),
    );
    toast(`已导出 ${list.length} 行`);
  });

  /* ---------- 分享 ---------- */
  $('#btnShare').addEventListener('click', () => $('#shareMask').classList.add('open'));
  $('#swShare').addEventListener('click', () => {
    const on = $('#swShare').classList.toggle('on');
    ($('#shareLinkRow') as HTMLElement).hidden = !on;
    if (on) ($('#shareUrl') as HTMLInputElement).value = 'https://crm.example.com/share/funnel/v_' + Math.random().toString(36).slice(2, 8);
    $('#shareHint').textContent = on ? '外部分享已开启 · 链接 7 天后自动失效' : '外部分享未开启';
  });
  $('#btnCopyShare').addEventListener('click', () => copyText(($('#shareUrl') as HTMLInputElement).value, toast));
  $('#btnInvite').addEventListener('click', () => {
    void openPicker(api, { kind: 'members', multi: false, max: 1, title: '邀请协作者' }).then((r) => {
      if (!r || !r.length) return;
      const m = r[0];
      $('#collabList').insertAdjacentHTML(
        'beforeend',
        `<div class="collab"><div class="collab-av" style="background:linear-gradient(135deg,#F7BA1E,#D97917)">${esc(m.name[0])}</div><div style="flex:1"><b style="font-size:13px">${esc(m.name)}</b><div class="muted" style="font-size:12px">${esc(m.dept || '企微员工')}</div></div><span class="chip blue">可查看</span></div>`,
      );
      toast('已邀请 ' + m.name);
    });
  });

  /* ---------- 群发 ---------- */
  const bcMats: PickerItem[] = [];
  const renderBcMats = (): void => {
    $('#bcMats').innerHTML = bcMats
      .map(
        (m, i) =>
          `<span style="display:inline-flex;align-items:center;gap:6px;height:24px;padding:0 8px;border-radius:4px;background:#EFF4FF;color:#245BDB;font-size:12px">${esc(m.name)}<span data-mat-rm="${i}" style="color:#A9BEF0;cursor:pointer">×</span></span>`,
      )
      .join('');
    root.querySelectorAll('[data-mat-rm]').forEach((el) =>
      el.addEventListener('click', () => {
        bcMats.splice(Number((el as HTMLElement).dataset.matRm), 1);
        renderBcMats();
      }),
    );
  };
  $('#btnSenders').addEventListener('click', () => toast('跳转到 /admin/hxc-send-config（演示）'));
  $('#btnBroadcast').addEventListener('click', () => {
    if (!selected.size) return toast('请先在表格中勾选要群发的行', true);
    const list = pipeline().filter((r) => selected.has(String(r.mobile_masked)));
    const usable = list.filter((r) => r.external_userid);
    $('#bcInfo').innerHTML = `选中 <b>${selected.size}</b> 行 · 有企微外部联系人 ID 可发送 <b style="color:#245BDB">${usable.length}</b> 人 · 跳过 ${selected.size - usable.length} 人（缺少 ID / 免打扰）`;
    $('#bcMask').classList.add('open');
  });
  root.querySelectorAll('[data-mat]').forEach((b) =>
    b.addEventListener('click', () => {
      const kind = (b as HTMLElement).dataset.mat as PickerKind;
      void openPicker(api, { kind, selected: bcMats.filter((m) => m.kind === kind).map((m) => m.id) }).then((r) => {
        if (!r) return;
        const kept = bcMats.filter((m) => m.kind !== kind);
        const next = [...kept, ...r.map((m) => ({ ...m, kind }))];
        if (next.length > 9) return toast('素材最多 9 个', true);
        bcMats.splice(0, bcMats.length, ...next);
        renderBcMats();
        if (r.length) toast('已添加 ' + r.length + ' 个素材到群发内容');
      });
    }),
  );
  $('#bcOk').addEventListener('click', () => {
    $('#bcMask').classList.remove('open');
    const usable = pipeline().filter((r) => selected.has(String(r.mobile_masked)) && r.external_userid);
    toast(`群发任务已创建 · 可发送 ${usable.length} 人 · 素材 ${bcMats.length} 个 · 派发状态: queued`);
  });

  /* ---------- 通用浮窗 ---------- */
  root.querySelectorAll('[data-close]').forEach((b) =>
    b.addEventListener('click', () => (b as HTMLElement).closest('.mask')!.classList.remove('open')),
  );
  root.querySelectorAll('.mask').forEach((m) =>
    m.addEventListener('click', (e) => {
      if (e.target === m) m.classList.remove('open');
    }),
  );
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') root.querySelectorAll('.mask.open').forEach((m) => m.classList.remove('open'));
  });

  sync();
}
