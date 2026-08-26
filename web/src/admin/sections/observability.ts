import { readPushTraceObservabilityDto, type PushTraceObservability } from '../../api/push_observability';
import { esc } from './util';

const button = 'height:30px;padding:0 11px;border:1px solid #D0D5DD;border-radius:6px;background:#fff;color:#344054;cursor:pointer';

function shell(body: string): string {
  return `<div style="padding:20px;display:grid;gap:16px;align-content:start"><div><div style="font-size:12px;color:#8F959E">运营 / 可观察性</div><h1 style="margin:4px 0 0;font-size:20px">Push Center trace_id 可观察性</h1></div>${body}</div>`;
}

function card(label: string, value: number): string {
  return `<div style="padding:10px;border:1px solid #DEE0E3;border-radius:8px;background:#fff"><div style="font-size:12px;color:#667085">${esc(label)}</div><strong style="font-size:19px">${value}</strong></div>`;
}

function observabilityHtml(page: PushTraceObservability): string {
  const sections = page.sections.map((section) => `<tr><td style="padding:8px;border-bottom:1px solid #EEF0F3">${esc(section.label)}</td><td style="padding:8px;border-bottom:1px solid #EEF0F3;font-family:ui-monospace,Menlo,monospace">${esc(section.key)}</td><td style="padding:8px;border-bottom:1px solid #EEF0F3">${section.count}</td></tr>`).join('') || '<tr><td colspan="3" style="padding:16px;color:#8F959E">没有可展示的 section 聚合</td></tr>';
  const summary = page.degraded ? `<div style="padding:10px;border:1px solid #F5D6A7;border-radius:8px;background:#FFF9F0;color:#8F5A16;font-size:13px"><strong>Push Center degraded：</strong>${esc(page.message || '读模型暂不可用')}；不会把空聚合解释为零审计或成功。</div>` : `<div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(110px,1fr));gap:8px">${[card('总数（本地）', page.counts!.total), card('pending', page.counts!.pending), card('running', page.counts!.running), card('sent（本地状态）', page.counts!.sent), card('failed', page.counts!.failed)].join('')}</div>`;
  const scope = page.traceID ? `<div style="padding:10px;border:1px solid #D6E4FF;border-radius:8px;background:#F5F8FF;color:#1849A9;font-size:13px">已以 <code>${esc(page.traceID)}</code> 调用真实 Push Center sections/stats 聚合。它仅证明本地聚合的 trace_id 范围，不证明外部调用、完整 trace 链路或送达。</div>` : '<div style="padding:10px;border:1px solid #F5D6A7;border-radius:8px;background:#FFF9F0;color:#8F5A16;font-size:13px">未输入 trace_id：当前只刷新全局本地聚合。输入 trace_id 后才会请求相应的受限聚合；当前没有可渲染的 audit JSON。</div>';
  return shell(`<div style="display:flex;gap:8px;flex-wrap:wrap"><button id="observability-back" style="${button}">返回 Campaign</button><button id="observability-refresh" style="${button}">刷新可观察性</button></div><section style="display:grid;gap:10px;padding:14px;border:1px solid #DEE0E3;border-radius:8px"><div style="display:grid;grid-template-columns:minmax(240px,1fr) minmax(240px,1fr);gap:10px"><label style="display:grid;gap:5px;font-size:13px">trace_id（真实过滤）<input id="observability-trace" maxlength="200" value="${esc(page.traceID || '')}" placeholder="输入 trace_id 后筛选本地聚合" style="padding:8px;border:1px solid #D0D5DD;border-radius:6px"></label><label style="display:grid;gap:5px;font-size:13px;color:#8F5A16">session_id（backend_blocked）<input disabled value="当前 OpenAPI 无 session_id/audit JSON 契约" style="padding:8px;border:1px solid #F5D6A7;border-radius:6px;background:#FFF9F0;color:#8F5A16"></label></div><div style="display:flex;gap:8px"><button id="observability-filter" style="${button}">按 trace_id 刷新</button></div>${scope}<div style="padding:10px;border:1px solid #F5D6A7;border-radius:8px;background:#FFF9F0;color:#8F5A16;font-size:13px"><strong>backend_blocked：</strong>Excel 所列 <code>/api/admin/cloud-orchestrator/audit?trace_id&amp;session_id</code> 不在当前 OpenAPI；Internal Events 也不接受 trace_id/session_id。因此不会冒充审计列表或把 session_id 传给其他接口。</div>${summary}<table style="width:100%;border-collapse:collapse"><thead><tr style="text-align:left;color:#667085"><th style="padding:8px">section</th><th style="padding:8px">key</th><th style="padding:8px">count</th></tr></thead><tbody>${sections}</tbody></table></section>`);
}

function sessionBlockedHtml(traceID?: string): string {
  return shell(`<div style="padding:12px;border:1px solid #F5D6A7;border-radius:8px;background:#FFF9F0;color:#8F5A16;font-size:13px"><strong>backend_blocked：</strong>当前 OpenAPI 没有 session_id 可筛选的 observability/audit JSON；已拒绝把它降级为全局查询或改写为 trace_id。</div><label style="display:grid;gap:5px;font-size:13px">改用真实 trace_id 过滤（可选）<input id="observability-trace" maxlength="200" value="${esc(traceID || '')}" placeholder="输入 trace_id" style="padding:8px;border:1px solid #D0D5DD;border-radius:6px"></label><div style="display:flex;gap:8px"><button id="observability-filter" style="${button}">按 trace_id 刷新</button><button id="observability-back" style="${button}">返回 Campaign</button></div>`);
}

function route(traceID?: string): void {
  const params = new URLSearchParams({ view: 'observability' });
  if (traceID) params.set('trace_id', traceID);
  history.replaceState(null, '', `campaigns.html?${params.toString()}`);
}

function bindTraceInput(stage: HTMLElement): void {
  stage.querySelector<HTMLButtonElement>('#observability-filter')?.addEventListener('click', () => {
    const traceID = stage.querySelector<HTMLInputElement>('#observability-trace')?.value || '';
    void loadPushObservability(stage, traceID).catch((error) => renderError(stage, error));
  });
  stage.querySelector<HTMLButtonElement>('#observability-back')?.addEventListener('click', () => { location.href = 'campaigns.html'; });
}

function renderError(stage: HTMLElement, error: unknown): void {
  stage.innerHTML = shell(`<div style="padding:12px;border:1px solid #F2B8B5;border-radius:8px;background:#FFF1F0;color:#B42318">${esc(error instanceof Error ? error.message : '可观察性读取失败')}</div><button id="observability-back" style="${button}">返回 Campaign</button>`);
  stage.querySelector<HTMLButtonElement>('#observability-back')?.addEventListener('click', () => { location.href = 'campaigns.html'; });
}

async function loadPushObservability(stage: HTMLElement, inputTraceID?: string): Promise<void> {
  const page = await readPushTraceObservabilityDto(inputTraceID);
  route(page.traceID);
  stage.innerHTML = observabilityHtml(page);
  bindTraceInput(stage);
}

export async function mountPushObservability(stage: HTMLElement): Promise<void> {
  const params = new URLSearchParams(location.search);
  const sessionID = params.get('session_id');
  const traceID = params.get('trace_id') || undefined;
  if (sessionID) {
    stage.innerHTML = sessionBlockedHtml(traceID);
    bindTraceInput(stage);
    return;
  }
  await loadPushObservability(stage, traceID);
}
