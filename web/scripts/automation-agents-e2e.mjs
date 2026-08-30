import { JSDOM } from 'jsdom';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const calls = [];
const agent = (patch = {}) => ({
  id: 7, automation_type: 'agent', agent_code: 'welcome_agent', agent_name: '欢迎 Agent',
  bound_package_key: '', bound_package_id: null, bound_package_name: '', fixed_material_summary: { image_count: 0, miniprogram_count: 0, attachment_count: 0, group_invite_count: 0 },
  status: 'paused', execution_enabled: false, materials_configured: false, updated_at: '2026-08-30T00:00:00Z', automation_type_label: 'Agent 机器人',
  draft_role_prompt: '旧角色', draft_task_prompt: '旧任务', published_role_prompt: '旧角色', published_task_prompt: '旧任务',
  draft_version: 1, published_version: 1, has_unpublished_changes: false,
  fixed_content_package: { content_text: '', image_library_ids: [], miniprogram_library_ids: [], attachment_library_ids: [], group_invite_library_ids: [] },
  fixed_content_package_preview: { content_text: '', material_summary: { image_count: 0, miniprogram_count: 0, attachment_count: 0, group_invite_count: 0 }, materials: [] },
  legacy_configuration: { scenario_code: 'welcome' }, ...patch,
});
const json = (body, status = 200) => ({ status, headers: new Headers({ 'Content-Type': 'application/json' }), text: async () => JSON.stringify(body) });

function page(name, query = '') {
  let html = fs.readFileSync(path.join(root, `dist/admin/${name}.html`), 'utf8');
  html = html.replace(/<script src="[^"]*assets\/admin\.js"><\/script>/, () => `<script>${fs.readFileSync(path.join(root, 'dist/assets/admin.js'), 'utf8')}</script>`);
  return new JSDOM(html, {
    url: `http://localhost/admin/${name}.html${query}`, runScripts: 'dangerously', pretendToBeVisual: true,
    beforeParse(window) {
      window.__AICRM_TEST_MOCK__ = false;
      window.document.cookie = `aicrm_csrf=${'c'.repeat(43)}`;
      window.fetch = async (input, init = {}) => {
        const url = new URL(String(input), window.location.origin); const method = init.method || 'GET';
        const call = { path: url.pathname, method, credentials: init.credentials, headers: new Headers(init.headers), body: init.body ? JSON.parse(String(init.body)) : undefined };
        calls.push(call);
        if (url.pathname === '/api/admin/automation-agents' && method === 'GET') return json({ ok: true, items: [agent()], total: 1 });
        if (url.pathname === '/api/admin/automation-agents/7' && method === 'GET') return json({ ok: true, agent: agent() });
        if (url.pathname === '/api/admin/automation-agents/7/precheck' && method === 'GET') return json({ ok: true, agent_id: 7, configuration_ready: true, materials_configured: false, execution_enabled: false, can_activate: false, reasons: ['material_unconfigured', 'execution_disabled'], real_external_call_executed: false });
        if (url.pathname === '/api/admin/automation-agents/7' && method === 'PATCH') return json({ ok: true, agent: agent({ agent_name: call.body.agent_name, draft_role_prompt: call.body.role_prompt, draft_task_prompt: call.body.task_prompt, legacy_configuration: call.body.legacy_configuration }) });
        return json({ code: 'unexpected_request', path: url.pathname }, 500);
      };
    },
  });
}

const list = page('agents');
await new Promise((resolve) => setTimeout(resolve, 50));
if (!list.window.document.body.textContent.includes('欢迎 Agent') || !list.window.document.body.textContent.includes('执行关闭') || list.window.document.body.textContent.includes('后端能力未就绪') || list.window.document.body.textContent.includes('启用</button>')) throw new Error('真实 Agent 列表未按关闭状态渲染');
list.window.document.querySelector('[data-agent-action="precheck"]').click();
await new Promise((resolve) => setTimeout(resolve, 50));
if (!calls.some((call) => call.path === '/api/admin/automation-agents/7/precheck' && call.method === 'GET') || !list.window.document.body.textContent.includes('execution_disabled')) throw new Error('启用前检查未读取真实关闭结果');
list.window.close();

const edit = page('agentEdit', '?id=7');
await new Promise((resolve) => setTimeout(resolve, 50));
const form = edit.window.document.querySelector('[data-agent-form]');
if (!form || !edit.window.document.body.textContent.includes('V1 必要业务配置 JSON') || !edit.window.document.body.textContent.includes('素材未配置') || form.querySelector('[name=image_library_ids]')) throw new Error('真实 Agent 编辑页未按无素材状态渲染');
form.querySelector('[name=agent_name]').value = '欢迎 Agent 新版';
form.querySelector('[name=role_prompt]').value = '新角色';
form.querySelector('[name=task_prompt]').value = '新任务';
form.querySelector('[name=legacy_configuration]').value = '{"scenario_code":"welcome-v2"}';
form.dispatchEvent(new edit.window.Event('submit', { bubbles: true, cancelable: true }));
await new Promise((resolve) => setTimeout(resolve, 50));
const update = calls.find((call) => call.path === '/api/admin/automation-agents/7' && call.method === 'PATCH');
if (!update || update.credentials !== 'include' || !update.headers.get('Idempotency-Key') || update.body.status !== 'paused' || update.body.agent_name !== '欢迎 Agent 新版' || update.body.role_prompt !== '新角色' || update.body.task_prompt !== '新任务' || update.body.legacy_configuration.scenario_code !== 'welcome-v2' || Object.values(update.body.fixed_content_package).some((value) => Array.isArray(value) && value.length > 0)) throw new Error('Agent 编辑未发出真实关闭写入请求');
if (calls.some((call) => /activate/i.test(call.path))) throw new Error('Agent 页面意外请求启用');
if (calls.some((call) => /send|dispatch|execute|provider|payment|refund/i.test(call.path))) throw new Error('Agent 编辑意外触发外部效果');
edit.window.close();
console.log('automation-agents-e2e: PASS');
