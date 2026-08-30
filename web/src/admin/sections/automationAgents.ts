import {
  archiveLegacyAutomationAgent,
  copyLegacyAutomationAgent,
  createLegacyAutomationAgent,
  getLegacyAutomationAgent,
  listLegacyAutomationAgents,
  precheckLegacyAutomationAgent,
  publishLegacyAutomationAgent,
  updateLegacyAutomationAgent,
} from '../../api/generated/p4-automation-agents/p4-automation-agents';
import {
  type LegacyAutomationAgentDetail,
  type LegacyAutomationAgentListResponse,
} from '../../api/generated/health.schemas';
import { apiRequestOptions, unwrapGenerated } from '../../api/transport';
import { esc } from './util';

function key(): string {
  return globalThis.crypto?.randomUUID?.() || `automation-agent-${Date.now()}-${Math.random()}`;
}

function writeOptions(): RequestInit {
  return apiRequestOptions({ headers: { 'Idempotency-Key': key() } });
}

function showResult(root: HTMLElement, message: string, failed = false): void {
  const result = root.querySelector<HTMLElement>('[data-agent-result]');
  if (!result) return;
  result.textContent = message;
  result.style.color = failed ? '#D83931' : '#2E7D32';
}

async function loadList(root: HTMLElement): Promise<void> {
  const data = unwrapGenerated(await listLegacyAutomationAgents(apiRequestOptions())) as LegacyAutomationAgentListResponse;
  if (!data.ok) throw new Error('自动化配置读取失败');
  const rows = data.items.map((item) => `<tr>
    <td>${esc(item.agent_name)}</td><td><code>${esc(item.agent_code)}</code></td><td>${esc(item.automation_type)}</td>
    <td>已暂停（执行关闭）</td><td>${esc(item.updated_at)}</td>
    <td style="display:flex;gap:8px;flex-wrap:wrap">
      <a href="agentEdit.html?id=${item.id}">编辑</a>
      <button type="button" data-agent-action="precheck" data-agent-id="${item.id}">启用前检查</button>
      <button type="button" data-agent-action="copy" data-agent-id="${item.id}">复制</button>
      <button type="button" data-agent-action="archive" data-agent-id="${item.id}">归档</button>
    </td>
  </tr>`).join('') || '<tr><td colspan="6">暂无当前自动化配置</td></tr>';
  root.innerHTML = `<section style="padding:24px">
    <div style="display:flex;justify-content:space-between;align-items:center"><div><h1>自动化话术 / Agent</h1><p>当前配置可真实编辑；迁移项默认暂停。此页面不运行 Agent、不发送消息，也不调用 Provider。</p></div><a href="agentEdit.html">新增</a></div>
    <div data-agent-result role="status" style="min-height:24px"></div>
    <table style="width:100%;background:#fff;border-collapse:collapse"><thead><tr><th>名称</th><th>编码</th><th>类型</th><th>状态</th><th>更新时间</th><th>操作</th></tr></thead><tbody>${rows}</tbody></table>
  </section>`;
  root.querySelectorAll<HTMLButtonElement>('[data-agent-action]').forEach((button) => button.addEventListener('click', () => {
    const agentID = Number(button.dataset.agentId);
    const action = button.dataset.agentAction;
    if (!Number.isSafeInteger(agentID) || agentID < 1) return;
    button.disabled = true;
    void (async () => {
      if (action === 'precheck') {
        const check = unwrapGenerated(await precheckLegacyAutomationAgent(agentID, apiRequestOptions())) as { configuration_ready: boolean; reasons: string[] };
        showResult(root, `当前不可启用：${check.reasons.join('、')}`, true);
        button.disabled = false;
        return;
      } else if (action === 'copy') unwrapGenerated(await copyLegacyAutomationAgent(agentID, writeOptions()));
      else if (action === 'archive') unwrapGenerated(await archiveLegacyAutomationAgent(agentID, writeOptions()));
      else throw new Error('未知操作');
      await loadList(root);
      showResult(root, '保存成功；未触发任何外部效果');
    })().catch((error) => {
      button.disabled = false;
      showResult(root, error instanceof Error ? error.message : '操作失败', true);
    });
  }));
}

function fixedContent(form: FormData) {
  return {
    content_text: String(form.get('content_text') || ''),
    image_library_ids: [], miniprogram_library_ids: [], attachment_library_ids: [], group_invite_library_ids: [],
  };
}

async function loadEditor(root: HTMLElement, agentID?: number): Promise<void> {
  const current = agentID ? (unwrapGenerated(await getLegacyAutomationAgent(agentID, apiRequestOptions())) as { ok: boolean; agent: LegacyAutomationAgentDetail }).agent : undefined;
  const content = current?.fixed_content_package;
  root.innerHTML = `<section style="padding:24px;max-width:1000px"><p><a href="agents.html">← 返回列表</a></p><h1>${current ? '编辑' : '新增'}自动化配置</h1>
    <p>保存只更新 V2 当前配置；不会运行 Agent、发送消息或调用 Provider。</p>
    <form data-agent-form style="display:grid;gap:14px;background:#fff;padding:20px">
      <label>名称<input name="agent_name" required maxlength="120" value="${esc(current?.agent_name || '')}"></label>
      <label>编码<input name="agent_code" required maxlength="120" pattern="[a-z0-9_-]+" ${current ? 'readonly' : ''} value="${esc(current?.agent_code || '')}"></label>
      <label>类型<select name="automation_type"><option value="agent" ${current?.automation_type !== 'fixed_script' ? 'selected' : ''}>Agent</option><option value="fixed_script" ${current?.automation_type === 'fixed_script' ? 'selected' : ''}>固定话术</option></select></label>
      <label>状态<input name="status" readonly value="paused（执行关闭）"></label>
      <label>角色话术<textarea name="role_prompt" maxlength="20000" rows="8">${esc(current?.draft_role_prompt || '')}</textarea></label>
      <label>任务话术<textarea name="task_prompt" maxlength="20000" rows="10">${esc(current?.draft_task_prompt || '')}</textarea></label>
      <label>固定文本<textarea name="content_text" maxlength="4000" rows="5">${esc(content?.content_text || '')}</textarea></label>
      <p role="status">素材未配置。旧素材引用已清空，当前页面不会请求已删除的素材。</p>
      <label>V1 必要业务配置 JSON<textarea name="legacy_configuration" rows="14">${esc(JSON.stringify(current?.legacy_configuration || {}, null, 2))}</textarea></label>
      <div data-agent-result role="status" style="min-height:24px"></div>
      <div style="display:flex;gap:10px"><button type="submit">保存</button>${current ? '<button type="button" data-agent-publish>发布当前草稿到本地版本</button>' : ''}</div>
    </form></section>`;
  const form = root.querySelector<HTMLFormElement>('[data-agent-form]');
  form?.addEventListener('submit', (event) => {
    event.preventDefault();
    const values = new FormData(form);
    void (async () => {
      const legacy = JSON.parse(String(values.get('legacy_configuration') || '{}')) as Record<string, unknown>;
      if (legacy === null || Array.isArray(legacy) || typeof legacy !== 'object') throw new Error('V1 必要业务配置必须是 JSON 对象');
      const payload = {
        agent_name: String(values.get('agent_name') || '').trim(),
        automation_type: String(values.get('automation_type')) as 'agent' | 'fixed_script',
        status: 'paused' as const,
        role_prompt: String(values.get('role_prompt') || ''),
        task_prompt: String(values.get('task_prompt') || ''),
        fixed_content_package: fixedContent(values),
        legacy_configuration: legacy,
      };
      const saved = current
        ? unwrapGenerated(await updateLegacyAutomationAgent(current.id, payload, writeOptions()))
        : unwrapGenerated(await createLegacyAutomationAgent({ ...payload, agent_code: String(values.get('agent_code') || '').trim() }, writeOptions()));
      const nextID = (saved as { agent: LegacyAutomationAgentDetail }).agent.id;
      location.href = `agentEdit.html?id=${nextID}&saved=1`;
    })().catch((error) => showResult(root, error instanceof Error ? error.message : '保存失败', true));
  });
  root.querySelector<HTMLButtonElement>('[data-agent-publish]')?.addEventListener('click', (event) => {
    const button = event.currentTarget as HTMLButtonElement;
    if (!current) return;
    button.disabled = true;
    void publishLegacyAutomationAgent(current.id, writeOptions()).then(unwrapGenerated).then(() => loadEditor(root, current.id)).catch((error) => {
      button.disabled = false;
      showResult(root, error instanceof Error ? error.message : '发布失败', true);
    });
  });
  if (new URLSearchParams(location.search).get('saved') === '1') showResult(root, '保存成功；已从服务端重新读取');
}

export async function mountAutomationAgents(root: HTMLElement, page: 'list' | 'edit'): Promise<void> {
  if (page === 'list') return loadList(root);
  const rawID = new URLSearchParams(location.search).get('id');
  const agentID = rawID ? Number(rawID) : undefined;
  if (rawID && (!Number.isSafeInteger(agentID) || (agentID || 0) < 1)) throw new Error('Agent ID 无效');
  return loadEditor(root, agentID);
}
