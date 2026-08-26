/**
 * 端到端 DOM 渲染验证（jsdom）：
 * 加载 dist/ 生成页，执行真实 bundle，断言渲染结果与关键交互。
 */
import { JSDOM } from 'jsdom';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const DIST = path.join(ROOT, 'dist');

let pass = 0;
let fail = 0;
const ok = (name, cond) => {
  if (cond) {
    pass++;
    console.log('  ✓ ' + name);
  } else {
    fail++;
    console.log('  ✗ ' + name);
  }
};

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function loadPage(rel, { id, q } = {}) {
  const file = path.join(DIST, rel);
  let html = fs.readFileSync(file, 'utf8');
  // 用 jsdom 执行内联脚本：把 bundle 内联进去，避免资源加载配置
  html = html.replace(/<script src="[^"]*assets\/(admin|h5|sidebar)\.js"><\/script>/, (_m, name) => {
    const code = fs.readFileSync(path.join(DIST, 'assets', name + '.js'), 'utf8');
    return `<script>${code}</script>`;
  });
  const qs = q || (id != null ? 'id=' + id : '');
  const dom = new JSDOM(html, {
    url: 'http://localhost/' + rel + (qs ? '?' + qs : ''),
    runScripts: 'dangerously',
    pretendToBeVisual: true,
    beforeParse(window) {
      // Mock 仅由 DOM 回归测试显式注入；浏览器默认运行态不会走此路径。
      window.__AICRM_TEST_MOCK__ = true;
      if (rel === 'admin/campaigns.html' && new URL(window.location.href).searchParams.get('legacy_admin_path') === '/admin/cloud-orchestrator/plans') {
        const json = (data, status = 200) => ({ ok: status >= 200 && status < 300, status, headers: new Headers({ 'Content-Type': 'application/json' }), text: async () => JSON.stringify(data) });
        const local = { local_only: true, provider_execution_eligible: false, runtime_executed: false, real_external_call_executed: false, delivery_proven: false };
        window.__planIndexCalls = [];
        window.fetch = async (input) => {
          const url = String(input);
          window.__planIndexCalls.push(url);
          return json({ items: [{ plan: { id: 'ctp_' + 'a'.repeat(64), campaign_code: 'spring-campaign', campaign_version: 3, source: { kind: 'customer_selection' }, target_count: 2, content_step_count: 1, created_at: '2026-08-27T00:00:00Z', ...local }, review_status: 'pending_review', review_version: 2 }], ...local });
        };
        return;
      }
      if (rel === 'admin/campaigns.html' && new URL(window.location.href).searchParams.get('recipient') === '7') {
        const json = (data, status = 200) => ({ ok: status >= 200 && status < 300, status, headers: new Headers({ 'Content-Type': 'application/json' }), text: async () => JSON.stringify(data) });
        const planID = 'ctp_' + 'a'.repeat(64);
        const local = { local_only: true, provider_execution_eligible: false, runtime_executed: false, real_external_call_executed: false, delivery_proven: false };
        const plan = { id: planID, campaign_code: 'spring-campaign', campaign_version: 3, source: { kind: 'customer_selection' }, target_count: 1, content_step_count: 1, created_at: '2026-08-27T00:00:00Z' };
        window.__recipientCalls = [];
        window.fetch = async (input) => {
          const url = String(input);
          window.__recipientCalls.push(url);
          if (url.endsWith('/recipients/7/review')) return json({ review: { canonical_customer_id: 7, status: 'pending_review', version: 1, updated_by_actor_id: 1, updated_at: '2026-08-27T00:00:00Z' }, ...local });
          if (url.endsWith(`/touch-plans/${planID}/review`)) return json({ review: { status: 'pending_review', version: 2 }, handoff: null, ...local });
          if (url.endsWith('/recipients/7')) return json({ canonical_customer_id: 7, ...local });
          if (url.endsWith('/recipients?limit=50')) return json({ items: [{ canonical_customer_id: 7 }], next_cursor: null, ...local });
          if (url.endsWith(`/touch-plans/${planID}`)) return json({ ...plan, content: { steps: [{ step_index: 1, delay_minutes: 0, content: '本地审核内容' }] }, ...local });
          if (url.includes('/touch-plans')) return json({ items: [plan], ...local });
          return json({ campaign: { campaign_code: 'spring-campaign', name: '春季激活', approval_status: 'draft', runtime_status: 'idle', version: 3, updated_at: '2026-08-27T00:00:00Z' }, steps: [], local_projection: true, real_external_call_executed: false, real_send: false, runtime_executed: false });
        };
        return;
      }
      if (rel === 'admin/campaigns.html' && new URL(window.location.href).searchParams.get('view') === 'observability') {
        const json = (data, status = 200) => ({ ok: status >= 200 && status < 300, status, headers: new Headers({ 'Content-Type': 'application/json' }), text: async () => JSON.stringify(data) });
        window.__observabilityCalls = [];
        window.fetch = async (input, init = {}) => {
          const url = String(input);
          window.__observabilityCalls.push({ url, init });
          const traceID = new URL('http://localhost' + url).searchParams.get('trace_id') || undefined;
          if (url.includes('/push-center/stats')) return json({ ok: true, counts: { total: 2, pending: 1, running: 0, succeeded: 0, sent: 1, failed: 0, shadow_warning: 0, by_effective_status: {}, by_status: {}, by_section: {} }, sections: [], status_definitions: [], filters: traceID ? { trace_id: traceID } : {}, route_owner: 'ai_crm_next', real_external_call_executed: false, runtime_queue: {}, capability_owner: 'ai_crm_next/platform_foundation/push_center' });
          return json({ ok: true, sections: [{ key: 'order', label: '订单', count: 2 }], status_definitions: [], filters: traceID ? { trace_id: traceID } : {}, route_owner: 'ai_crm_next' });
        };
        return;
      }
      if (rel === 'admin/campaigns.html' && new URL(window.location.href).searchParams.get('view') === 'external-effects') {
        const json = (data, status = 200) => ({ ok: status >= 200 && status < 300, status, headers: new Headers({ 'Content-Type': 'application/json' }), text: async () => JSON.stringify(data) });
        const local = { local_fact_only: true, real_external_call_executed: false, delivery_proven: false, delivery_semantics: 'local_state_not_delivery_proof' };
        const pushJob = { job_id: 18, task_id: 5, customer_id: 7, status: 'outcome_unknown', attempt_count: 1, failure_present: true, failure_class: 'outcome_unknown', provider_receipt_present: false, queue_job: { river_job_id: 9, generation: 1, kind: 'outbound_enqueue_one' }, created_at: '2026-08-27T00:00:00Z', status_updated_at: '2026-08-27T00:01:00Z', ...local };
        const pushEnvelope = { ok: true, fallback_used: false, source_status: 'v2_outbound_service', ...local };
        window.fetch = async (input) => {
          const url = String(input);
          if (url.includes('/external-effects/diagnostics')) return json({ accepted: 1, queued: 2, attempted: 1, outcome_unknown: 1, retryable_failed: 0 });
          if (url.includes('/external-effects/jobs')) return json({ ok: true, items: [{ id: 'eej_v1_abcdefghijklmnopqrstuv', status: 'outcome_unknown', classification: 'manual_review', attempt_count: 1, created_at: '2026-08-27T00:00:00Z', status_updated_at: '2026-08-27T00:01:00Z' }], next_cursor: null, page_size: 50, applied_filters: { status: null, classification: null }, provider_execution_eligible: false, ...local });
          if (url.includes('/external-effects')) return json({ items: [{ id: '18', owner: 'campaign', kind: 'campaign_dispatch', state: 'accepted', attempt_count: 0, generation: 1, updated_at: '2026-08-27T00:00:00Z' }] });
          if (url.includes('/push-center/sections')) return json({ ok: true, sections: [{ key: 'order', label: '订单', count: 2 }], status_definitions: [], filters: {}, route_owner: 'ai_crm_next' });
          if (url.includes('/push-center/stats')) return json({ ok: true, counts: { total: 2, pending: 1, running: 0, succeeded: 0, sent: 1, failed: 0, shadow_warning: 0, by_effective_status: {}, by_status: {}, by_section: {} }, sections: [], status_definitions: [], filters: {}, route_owner: 'ai_crm_next', real_external_call_executed: false, runtime_queue: {}, capability_owner: 'ai_crm_next/platform_foundation/push_center' });
          if (url.endsWith('/reconciliation')) return json({ ...pushEnvelope, job: pushJob, attempts: [{ attempt_id: 1, history_id: 2, generation: 1, river_job_id: 9, attempt: 1, max_attempts: 3, state: 'outcome_unknown', failure_present: true, failure_class: 'outcome_unknown', provider_receipt_present: false, dispatch_started_at: '2026-08-27T00:00:00Z', ...local }], control_receipts: [] });
          if (url.endsWith('/18')) return json({ ...pushEnvelope, job: pushJob });
          return json({ ...pushEnvelope, jobs: [pushJob], items: [pushJob], count: 1, has_more: false, limit: 50, offset: 0 });
        };
        return;
      }
      if (rel !== 'sidebar/index.html') return;
      window.URL.createObjectURL = () => 'blob:sidebar-thumbnail';
      window.URL.revokeObjectURL = () => {};
      window.wx = {
        agentConfig(options) { options.success?.({ err_msg: 'agentConfig:ok' }); },
        invoke(method, payload, callback) {
          window.__sidebarTest.wxMessages.push({ method, payload });
          callback({ err_msg: method + ':ok' });
        },
      };
      const scenario = new URL(window.location.href).searchParams.get('sidebar_case') || 'success';
      const safety = { local_only: true, provider_execution_eligible: false, real_external_call_executed: false };
      const memberRef = 'spm_' + 'A'.repeat(22);
      const profile = {
        customer_id: 7,
        name: '侧边栏测试客户',
        owner_staff_id: 9,
        source: '企微',
        industry: '教育',
        description: '测试画像',
        needs: '测试需求',
        pain_points: '测试卡点',
        updated_at: '2026-08-26T01:00:00Z',
      };
      const json = (data, status = 200) => ({
        ok: status >= 200 && status < 300,
        status,
        headers: new Headers({ 'Content-Type': 'application/json' }),
        text: async () => JSON.stringify(data),
        json: async () => data,
        blob: async () => new window.Blob([JSON.stringify(data)], { type: 'application/json' }),
        clone() { return this; },
      });
      window.__sidebarTest = { remarkBody: null, idempotencyKey: null, phoneBody: null, phoneKey: null, materialQueries: [], temporaryKeys: [], wxMessages: [] };
      window.fetch = async (input, init = {}) => {
        const url = String(input);
        if (url.includes('/jssdk/agent-config')) {
          return json({ signature_type: 'agent_config', corp_id: 'ww-test', agent_id: 1, nonce: 'nonce', timestamp: 1, signature: 'signature', url: 'http://localhost/sidebar/index.html' });
        }
        if (url.includes('/context-token')) {
          return json({ state: 'ready', context_token: 'sidebar-context-token-' + 'x'.repeat(52), customer_id: 7, owner_staff_id: 9, safety });
        }
        if (url.includes('/workbench')) {
          return json({ profile, questionnaire_count: scenario === 'empty' ? 0 : 1, order_count: scenario === 'success' ? 1 : 0, periodic_order_count: scenario === 'success' ? 1 : 0, material_count: scenario === 'success' ? 2 : 0, safety });
        }
        if (url.includes('/phone-binding')) {
          window.__sidebarTest.phoneBody = JSON.parse(init.body || '{}');
          window.__sidebarTest.phoneKey = new Headers(init.headers).get('Idempotency-Key');
          return json({ status: 'bound', safety });
        }
        if (url.includes('/questionnaires')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          return json({
            items: scenario === 'empty' ? [] : [{ submission_id: 11, questionnaire_id: 3, submitted_at: '2026-08-26T01:00:00Z', score: 8.5, choice_answers: [{ question_id: 2, question_type: 'single_choice', sort_order: 0, option_ids: [9] }] }],
            scan_truncated: false,
            result_truncated: false,
            safety,
          });
        }
        if (url.includes('/timeline')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          return json({
            items: scenario === 'empty' ? [] : [{ id: 7, event_type: 'survey_submitted', occurred_at: '2026-08-26T00:00:00Z' }],
            next_cursor: scenario === 'success' ? 'timeline-next' : undefined,
            safety,
          });
        }
        if (url.includes('/chat-activity')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          const chatType = url.includes('chat_type=group') ? 'group' : 'private';
          return json({
            items: scenario === 'empty' ? [] : [{ chat_type: chatType, message_type: 'text', sent_at: '2026-08-26T00:30:00Z' }],
            next_cursor: undefined,
            previous_cursor: undefined,
            safety,
          });
        }
        if (url.includes('/periodic-orders/') && url.includes('/remark')) {
          if (scenario === 'error') return json({ code: 'conflict' }, 409);
          window.__sidebarTest.remarkBody = JSON.parse(init.body || '{}');
          window.__sidebarTest.idempotencyKey = new Headers(init.headers).get('Idempotency-Key');
          return json({
            member: { member_ref: memberRef, service_product_id: 3, customer_id: 7, state: 'active', source: 'paid_order', starts_at: '2026-08-01T00:00:00Z', expires_at: '2026-09-01T00:00:00Z', remark: window.__sidebarTest.remarkBody.remark, alliance: '测试联盟', version: 2, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-26T02:00:00Z' },
            safety,
          });
        }
        if (url.includes('/periodic-orders')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          return json({
            items: scenario === 'empty' ? [] : [{ member_ref: memberRef, service_product_id: 3, customer_id: 7, state: 'active', source: 'paid_order', starts_at: '2026-08-01T00:00:00Z', expires_at: '2026-09-01T00:00:00Z', remark: '首期备注', alliance: '测试联盟', version: 1, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z' }],
            limit: 20,
            offset: 0,
            has_more: false,
            safety,
          });
        }
        if (url.includes('/orders')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          return json({
            items: scenario === 'empty' ? [] : [{ created_at: '2026-08-26T00:40:00Z', merchant_order_no: 'M20260826001', product_code: 'course-1', product_name: '测试课程', amount_yuan: '99.00', currency: 'CNY', status: 'paid', status_label: '已支付', provider: 'wechat_pay', provider_label: '微信支付' }],
            total: scenario === 'empty' ? 0 : 1,
            limit: 20,
            has_more: false,
            safety,
          });
        }
        if (url.includes('/shareable-products')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          return json({
            items: scenario === 'empty' ? [] : [
              { kind: 'ordinary', product_id: 41, product_code: 'course-ordinary', name: '普通课程', description: '真实普通商品字段', price_minor: 9900, currency: 'CNY', stock_quantity: 10, public_path: '/p/ordinary/41' },
              { kind: 'service_period', product_id: 42, product_code: 'course-period', name: '周期课程', description: '真实周期商品字段', price_minor: 19900, currency: 'CNY', stock_quantity: 8, public_path: '/p/service_period/42' },
            ],
            safety,
          });
        }
        if (url.includes('/temporary-media')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          window.__sidebarTest.temporaryKeys.push(new Headers(init.headers).get('Idempotency-Key'));
          return json({ image_id: 31, media_id: 'media-real-31', media_expires_at: '2026-08-28T00:00:00Z', upload_state: 'ready', provider_call_dispatched: true, real_external_call_executed: true, client_callback: 'not_called', delivery_state: 'not_sent_yet' });
        }
        if (url.includes('/materials/image/')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          if (url.includes('/image/32/')) return json({ code: 'not_found' }, 404);
          return {
            ok: true,
            status: 200,
            headers: new Headers({ 'Content-Type': 'image/png', ETag: '"thumb"' }),
            text: async () => '',
            blob: async () => new window.Blob(['image-bytes'], { type: 'image/png' }),
          };
        }
        if (url.includes('/materials')) {
          if (scenario === 'error') return json({ code: 'unavailable' }, 503);
          window.__sidebarTest.materialQueries.push(url);
          return json({
            items: scenario === 'empty' ? [] : [
              { id: 31, name: '欢迎海报', file_name: 'welcome.png', mime_type: 'image/png', file_size: 1024, description: '测试素材', tags: ['欢迎语'], category: '海报', width: 800, height: 600, updated_at: '2026-08-26T00:50:00Z', thumbnail_status: 'pending' },
              { id: 32, name: '课程卡片', file_name: 'course.png', mime_type: 'image/png', file_size: 2048, description: '', tags: ['课程'], category: '课程', width: 600, height: 400, updated_at: '2026-08-26T00:51:00Z', thumbnail_status: 'pending' },
            ],
            total: scenario === 'empty' ? 0 : 2,
            limit: 20,
            offset: 0,
            quick_keywords: ['欢迎语', '课程卡片'],
            safety,
          });
        }
        return json({ code: 'unexpected_sidebar_request' }, 500);
      };
    },
  });
  // 等 loadDb（120ms）+ 二级加载（200ms）+ 余量
  await sleep(700);
  return dom;
}

const click = (dom, el) => el.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
const input = (dom, el, v) => {
  el.value = v;
  el.dispatchEvent(new dom.window.Event('input', { bubbles: true }));
};

/* ================= 后台 · 内容雷达 ================= */
console.log('admin/radar.html（列表）');
{
  const dom = await loadPage('admin/radar.html');
  const d = dom.window.document;
  ok('雷达列表渲染 5 行', d.querySelectorAll('#listRows tr').length === 5);
  ok('导航高亮落在内容雷达', !!d.querySelector('.nav-item.on[href="radar.html"]'));
  // 关键词筛选
  input(dom, d.querySelector('#fKeyword'), '沙龙');
  await sleep(30);
  ok('搜索「沙龙」后剩 1 行', d.querySelectorAll('#listRows tr').length === 1);
  input(dom, d.querySelector('#fKeyword'), '');
  await sleep(30);
  // 分享浮窗
  const shareBtn = d.querySelector('[data-share]');
  click(dom, shareBtn);
  await sleep(30);
  ok('分享浮窗打开且伪二维码渲染', d.querySelector('#shareMask').classList.contains('open') && !!d.querySelector('#shareQr svg'));
  click(dom, d.querySelector('#shareMask .modal-x'));
  await sleep(30);
  ok('关闭分享浮窗', !d.querySelector('#shareMask').classList.contains('open'));
  // 停用 → 写穿并 toast
  const toggleBtn = [...d.querySelectorAll('[data-toggle]')].find((b) => b.textContent.trim() === '停用');
  click(dom, toggleBtn);
  await sleep(500);
  ok('停用后 toast 反馈且按钮变「启用」', d.body.textContent.includes('已停用'));
  dom.window.close();
}

console.log('admin/radarDetail.html?id=2（详情）');
{
  const dom = await loadPage('admin/radarDetail.html', { id: 2 });
  const d = dom.window.document;
  ok('详情页读取 ?id=2 显示对应标题', d.body.textContent.includes('共学营开营通知'));
  ok('4 张统计卡（含授权转化率 79%）', d.querySelectorAll('.stat-row .stat').length === 4 && d.body.textContent.includes('79%'));
  ok('访问明细渲染 24 行', d.querySelectorAll('#dRows tr').length === 24);
  input(dom, d.querySelector('#dKeyword'), '2f9Qn');
  await sleep(30);
  ok('明细按外部联系人 ID 过滤', d.querySelectorAll('#dRows tr').length === 3);
  dom.window.close();
}

console.log('admin/radarForm.html（新建校验）');
{
  const dom = await loadPage('admin/radarForm.html');
  const d = dom.window.document;
  const save = [...d.querySelectorAll('button')].find((b) => b.textContent.includes('保存内容雷达'));
  click(dom, save);
  await sleep(30);
  ok('名称为空时阻止保存并提示', d.querySelector('#fb-toast').textContent === '请输入内容名称');
  // 切换到图片类型 → 素材区出现
  click(dom, d.querySelector('.type-card[data-t="image"]'));
  await sleep(30);
  ok('切换图片类型后素材与独立目标地址同时显示', !d.querySelector('#cfgMedia').hidden && !d.querySelector('#cfgUrl').hidden);
  // 选素材（通用选择器）→ 填名称 → 保存成功跳列表前 toast
  click(dom, d.querySelector('#btnPick'));
  await sleep(450);
  ok('图片素材选择器打开', !!d.querySelector('.pk-mask'));
  click(dom, d.querySelector('.pk-mask [data-pk-id]'));
  await sleep(30);
  click(dom, d.querySelector('.pk-mask [data-pk="ok"]'));
  await sleep(30);
  ok('选择素材后写回表单', d.body.textContent.includes('来自素材库'));
  input(dom, d.querySelector('#fName'), 'e2e 测试图片雷达');
  input(dom, d.querySelector('#fUrl'), 'https://example.test/poster');
  click(dom, save);
  await sleep(600);
  ok('保存成功 toast', d.querySelector('#fb-toast').textContent === '已保存内容雷达');
  dom.window.close();
}

/* ================= 后台 · AI 助手 ================= */
console.log('admin/ai.html（计划列表）');
{
  const dom = await loadPage('admin/ai.html');
  const d = dom.window.document;
  ok('计划列表渲染 8 行', d.querySelectorAll('#planList .plan-row').length === 8);
  ok('统计卡：待审批 2 / 执行中 1', d.querySelector('#stPending').textContent === '2' && d.querySelector('#stActive').textContent === '1');
  input(dom, d.querySelector('#fKeyword'), '共学营');
  await sleep(30);
  ok('搜索「共学营」后剩 1 条', d.querySelectorAll('#planList .plan-row').length === 1);
  dom.window.close();
}

console.log('admin/aiDetail.html?id=7（详情 + 抽屉）');
{
  const dom = await loadPage('admin/aiDetail.html', { id: 7 });
  const d = dom.window.document;
  ok('计划信息含目标人数 1,632', d.body.textContent.includes('1,632'));
  ok('人员分页加载（50 / 180）', d.querySelector('#rcLoaded').textContent.includes('50 / 180'));
  ok('人员表渲染 50 行', d.querySelectorAll('#rcRows tr').length === 50);
  // 继续加载
  click(dom, d.querySelector('#rcMore'));
  await sleep(30);
  ok('继续加载后为 100 行', d.querySelectorAll('#rcRows tr').length === 100);
  // 打开人员抽屉
  const openBtn = [...d.querySelectorAll('#rcRows [data-rc]')].find((b) => b.tagName === 'BUTTON');
  click(dom, openBtn);
  await sleep(30);
  ok('人员抽屉打开（话术任务可见）', d.querySelector('#drawer').classList.contains('open') && d.body.textContent.includes('话术任务 1'));
  // 批准这个人
  click(dom, d.querySelector('#dwApprove'));
  await sleep(400);
  ok('批准人员后 toast + 状态级联', d.querySelector('#fb-toast').textContent === '已批准这个人发送');
  click(dom, d.querySelector('#dwClose'));
  await sleep(30);
  // 整单批准（确认浮窗 → 级联）
  click(dom, d.querySelector('#dApprove'));
  await sleep(30);
  ok('整单批准弹出确认浮窗', d.querySelector('#fb-mask').style.display === 'flex');
  click(dom, d.querySelector('#fb-ok'));
  await sleep(400);
  ok('批准后状态变「已批准」且按钮锁定', d.querySelector('#dStatus').textContent === '已批准' && d.querySelector('#dApprove').disabled);
  dom.window.close();
}

/* ================= 后台 · 漏斗多维表格 ================= */
console.log('admin/funnel.html（多维表格）');
{
  const dom = await loadPage('admin/funnel.html');
  const d = dom.window.document;
  ok('默认视图渲染 120 行', d.querySelectorAll('#gridBody tr').length === 120);
  ok('3 个预置视图页签', d.querySelectorAll('#viewTabs .vtab').length === 3);
  // 切换「拉回重点」视图（筛选 + 分组）
  click(dom, d.querySelectorAll('#viewTabs .vtab')[1]);
  await sleep(30);
  ok('切换视图后出现分组头', d.querySelectorAll('#gridBody .ghead').length > 0);
  ok('结果摘要显示分组信息', d.querySelector('#resultSummary').textContent.includes('已按「企微跟进人」分组'));
  // 折叠第一组
  click(dom, d.querySelector('#gridBody .ghead'));
  await sleep(30);
  ok('组头可折叠', d.querySelector('#gridBody .ghead').classList.contains('collapsed'));
  // 勾选一行
  const ck = d.querySelector('[data-ck]');
  ck.checked = true;
  ck.dispatchEvent(new dom.window.Event('change', { bubbles: true }));
  await sleep(30);
  ok('勾选行后底部显示已选 1 行', d.querySelector('#selInfo').textContent.includes('已选 1 行'));
  // 表头排序切换
  const th = [...d.querySelectorAll('#gridHead th')].find((t) => t.textContent.includes('沉默天数'));
  click(dom, th);
  await sleep(30);
  ok('点表头出现排序标记（视图进入未保存态）', !!d.querySelector('#gridHead .sort-mark') && !d.querySelector('#btnSaveView').hidden);
  // 分享浮窗
  click(dom, d.querySelector('#btnShare'));
  await sleep(30);
  ok('分享浮窗打开', d.querySelector('#shareMask').classList.contains('open'));
  click(dom, d.querySelector('#swShare'));
  await sleep(30);
  ok('开启外部分享生成链接', (d.querySelector('#shareUrl').value || '').includes('/share/funnel/v_'));
  // 邀请协作者 → 选客服组件
  click(dom, d.querySelector('#btnInvite'));
  await sleep(450);
  ok('邀请协作者弹出选客服组件', !!d.querySelector('.pk-mask'));
  click(dom, d.querySelector('.pk-mask [data-pk-id]'));
  await sleep(30);
  click(dom, d.querySelector('.pk-mask [data-pk="ok"]'));
  await sleep(30);
  ok('协作者名单新增 1 人', d.querySelectorAll('#collabList .collab').length === 3);
  click(dom, d.querySelector('#shareMask .modal-x'));
  await sleep(30);
  // 群发浮窗 → 素材选择器
  click(dom, d.querySelector('#btnBroadcast'));
  await sleep(30);
  click(dom, d.querySelector('[data-mat="image"]'));
  await sleep(450);
  ok('群发素材选择器打开', !!d.querySelector('.pk-mask'));
  click(dom, d.querySelector('.pk-mask [data-pk-id]'));
  await sleep(30);
  click(dom, d.querySelector('.pk-mask [data-pk="ok"]'));
  await sleep(30);
  ok('群发内容出现素材 chip', d.querySelector('#bcMats').textContent.length > 0);
  dom.window.close();
}

/* ================= 后台 · 模板页回归 ================= */
console.log('admin/questionnaires.html');
{
  const dom = await loadPage('admin/questionnaires.html');
  const d = dom.window.document;
  ok('问卷列表 6 行', d.querySelectorAll('tbody tr').length === 6);
  dom.window.close();
}

console.log('admin/customers.html（筛选、opaque cursor 翻页与详情导航）');
{
  const dom = await loadPage('admin/customers.html');
  const d = dom.window.document;
  ok('客户首屏按服务端页大小渲染 50 行', d.querySelectorAll('tbody tr').length === 50);
  ok('客户列表使用真实总数 55', d.body.textContent.includes('共 55 位客户') && d.body.textContent.includes('第 1 – 50 条，共 55 条'));
  ok('客户行详情链接使用 canonical numeric OneID', d.querySelector('a[href="customerDetail.html?id=1"]')?.__dcBound === true);

  input(dom, d.querySelector('#fCustomerKeyword'), '李思远');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '查询'));
  await sleep(300);
  ok('关键词查询只保留匹配客户并重置到第 1 页', d.querySelectorAll('tbody tr').length === 10 && d.body.textContent.includes('共 10 位客户') && d.body.textContent.includes('第 1 页'));

  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '清空'));
  await sleep(300);
  input(dom, d.querySelector('#fCustomerOwner'), '101');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '查询'));
  await sleep(300);
  ok('负责人 staff_id 查询按 canonical 参数过滤', d.querySelectorAll('tbody tr').length === 19 && d.body.textContent.includes('共 19 位客户'));

  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '清空'));
  await sleep(300);
  input(dom, d.querySelector('#fCustomerTag'), '2');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '查询'));
  await sleep(300);
  ok('标签 tag_id 查询按 canonical 参数过滤', d.querySelectorAll('tbody tr').length === 18 && d.body.textContent.includes('共 18 位客户'));

  input(dom, d.querySelector('#fCustomerMobile'), '138000000000');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '查询'));
  await sleep(30);
  ok('非法手机号在请求前显示 E.164 错误', d.querySelector('[data-customer-error]')?.textContent.includes('E.164'));

  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '清空'));
  await sleep(300);
  const next = [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '下一页');
  click(dom, next);
  await sleep(300);
  ok('下一页使用服务端 opaque cursor 并显示第 2 页 5 行', d.querySelectorAll('tbody tr').length === 5 && d.body.textContent.includes('第 2 页') && d.querySelector('a[href="customerDetail.html?id=51"]')?.__dcBound === true);
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '上一页'));
  await sleep(300);
  ok('上一页沿 cursor 栈返回第 1 页', d.querySelectorAll('tbody tr').length === 50 && d.body.textContent.includes('第 1 页'));
  dom.window.close();
}

console.log('admin/customerDetail.html（安全 Customer360）');
{
  const dom = await loadPage('admin/customerDetail.html', { id: 1 });
  const d = dom.window.document;
  ok('Customer360 渲染安全档案', d.body.textContent.includes('李思远') && d.body.textContent.includes('安全 Customer360') && d.body.textContent.includes('渠道 ID'));
  ok('Customer360 渲染标签与时间线摘要', d.querySelectorAll('[data-customer-not-found]').length === 0 && d.querySelectorAll('tbody tr').length === 2 && d.body.textContent.includes('owner.assigned'));
  ok('Customer360 聊天只展示零正文摘要', d.querySelectorAll('[data-customer-not-found]').length === 0 && d.body.textContent.includes('消息类型：text') && d.body.textContent.includes('消息类型：image') && d.body.textContent.includes('仅展示类型和时间，不展示正文'));
  const rendered = d.querySelector('#stage')?.textContent || '';
  ok('Customer360 不展示手机号与外部身份', !rendered.includes('手机号') && !rendered.includes('external_userid') && !rendered.includes('unionid') && !rendered.includes('这个周期服务能开发票吗？'));
  ok('Customer360 渲染安全问卷 ID 投影', d.querySelector('[data-customer-survey]')?.textContent.includes('提交 7001') && d.body.textContent.includes('题目 5') && d.body.textContent.includes('选项 12'));
  ok('Customer360 明确隐藏自由文本与评测', d.querySelector('[data-customer-answer-policy]')?.textContent.includes('不展示自由文本') && d.body.textContent.includes('当前 V2 契约不可用'));
  dom.window.close();
}

console.log('admin/customerDetail.html?id=999（404 占位态）');
{
  const dom = await loadPage('admin/customerDetail.html', { id: 999 });
  const d = dom.window.document;
  const back = d.querySelector('[data-customer-not-found] button');
  ok('客户不存在显示明确占位', d.querySelector('[data-customer-not-found]')?.textContent.includes('客户档案不存在'));
  ok('客户不存在提供返回客户列表', back?.textContent.trim() === '返回客户列表' && back?.__dcBound === true && !d.querySelector('#fCustomerName'));
  dom.window.close();
}

/* ================= 本轮新增：二级页 + 通用选择器 ================= */
let audienceDetailPackageId = 1;
console.log('admin/automation.html（新增分组弹窗 → 测试 Mock 创建）');
{
  const dom = await loadPage('admin/automation.html');
  const d = dom.window.document;
  const audienceDetail = d.querySelector('a[href^="audienceEdit.html?id="]');
  const audienceDetailId = Number(audienceDetail && new URL(audienceDetail.href).searchParams.get('id'));
  if (Number.isSafeInteger(audienceDetailId) && audienceDetailId > 0) audienceDetailPackageId = audienceDetailId;
  ok('人群包详情链接保留实际 package_id', audienceDetail?.__dcBound === true && Number.isSafeInteger(audienceDetailId) && audienceDetailId > 0);
  const addBtn = [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '新增');
  click(dom, addBtn);
  await sleep(30);
  ok('新增分组弹窗打开', !!d.querySelector('#fGroupName'));
  input(dom, d.querySelector('#fGroupName'), 'e2e 测试分组');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '确认'));
  await sleep(500);
  ok('创建后分组出现在分组列表', d.body.textContent.includes('e2e 测试分组'));
  dom.window.close();
}

console.log('admin/audienceEdit.html?id={package_id}（真实配置与发送人 DTO）');
{
  const dom = await loadPage('admin/audienceEdit.html', { id: audienceDetailPackageId });
  const d = dom.window.document;
  const nav4 = [...d.querySelectorAll('button')].find((b) => b.textContent.includes('成员列表'));
  click(dom, nav4);
  await sleep(30);
  const h3 = [...d.querySelectorAll('h3')].find((h) => h.textContent === '成员列表');
  let panel = h3 && h3.parentElement;
  while (panel && panel.style.display !== 'block' && panel.style.display !== 'none') panel = panel.parentElement;
  ok('切到成员列表面板', !!panel && panel.style.display === 'block');
  const nav3 = [...d.querySelectorAll('button')].find((b) => b.textContent.includes('发送人白名单'));
  click(dom, nav3);
  await sleep(30);
  ok('发送人使用 sender_userid 明文 DTO', !!d.querySelector('#aeSenders') && d.body.textContent.includes('最多 5 位'));
  ok('模板目录缺契约明确标为 backend_blocked', d.querySelector('[data-template-contract="backend_blocked"]')?.textContent.includes('不会把 SegmentDefinition 伪装成可选模板'));
  d.querySelector('#aeSenders').value = 'a\nb\nc\nd\ne\nf';
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '保存发送人白名单'));
  await sleep(30);
  ok('超过 5 位在发请求前被阻止', d.body.textContent.includes('发送人最多 5 位且不能重复'));
  dom.window.close();
}

console.log('admin/cyclesDetail.html?id=1（8 章运行档案）');
{
  const dom = await loadPage('admin/cyclesDetail.html', { id: 1 });
  const d = dom.window.document;
  ok(
    '八章档案渲染（含分窗口结果 / 结果复盘 / 证据索引）',
    d.body.textContent.includes('分窗口结果') && d.body.textContent.includes('结果复盘与限制') && d.body.textContent.includes('证据索引'),
  );
  dom.window.close();
}

console.log('admin/tags.html（新建标签测试 Mock 建行）');
{
  const dom = await loadPage('admin/tags.html');
  const d = dom.window.document;
  const before = d.querySelectorAll('tbody tr').length;
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '新建标签'));
  await sleep(30);
  ok('新建标签编辑组件打开', !!d.querySelector('#fTagName'));
  input(dom, d.querySelector('#fTagName'), 'e2e标签X');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '创建'));
  await sleep(500);
  ok('标签 Mock 创建（行数 +1）', d.querySelectorAll('tbody tr').length === before + 1);
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '同步企微标签'));
  await sleep(850);
  ok('标签同步只确认受理，未宣称 Provider 成功', d.querySelector('#fb-toast')?.textContent.includes('已受理') && d.querySelector('#fb-toast')?.textContent.includes('尚未收到 Provider 同步结果'));
  dom.window.close();
}

console.log('admin/questionnaireOps.html?id=1（opaque 本地运营配置）');
{
  const dom = await loadPage('admin/questionnaireOps.html', { id: 1 });
  const d = dom.window.document;
  ok('只展示 opaque navigation target 与正整数渠道 ID', !!d.querySelector('#opsNavigationTarget') && !!d.querySelector('#opsChannelResourceId') && !d.querySelector('#opsRedirectUrl'));
  ok('外部推送只接受 configuration reference', !!d.querySelector('#opsConfigurationReference') && !d.querySelector('#opsWebhook'));
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.includes('测试推送')));
  await sleep(30);
  ok('测试外推明确为本地 queued 记录且未宣称派发', d.querySelector('#fb-body').textContent.includes('不执行外部派发'));
  dom.window.close();
}

console.log('admin/couponData.html?id=0（5 统计卡 + 8 明细行 + 分享组件）');
{
  const dom = await loadPage('admin/couponData.html', { id: 0 });
  const d = dom.window.document;
  ok(
    '5 张统计卡文案齐全',
    ['累计领取', '当前可用', '支付预占', '已使用', '已过期'].every((t) => d.body.textContent.includes(t)),
  );
  ok('领取明细 8 行', d.querySelectorAll('tbody tr').length === 8);
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '分享'));
  await sleep(30);
  ok('分享弹窗打开且伪二维码渲染', !!d.querySelector('#shareQrBox svg') && d.body.textContent.includes('分享优惠券'));
  dom.window.close();
}

console.log('admin/configDetail.html?cat=wechat_pay（类目配置点渲染）');
{
  const dom = await loadPage('admin/configDetail.html', { q: 'cat=wechat_pay' });
  const d = dom.window.document;
  ok('微信支付类目字段渲染（含 WECHAT_PAY_MCH_ID）', d.body.textContent.includes('WECHAT_PAY_MCH_ID'));
  ok('支持检查的类目显示「检查」按钮', [...d.querySelectorAll('button')].some((b) => b.textContent.trim() === '检查'));
  dom.window.close();
}

console.log('admin/channelForm.html（完整 OpenAPI 渠道 DTO）');
{
  const dom = await loadPage('admin/channelForm.html');
  const d = dom.window.document;
  ok('载体、客服、素材与标签字段齐全', !!d.querySelector('#channelType') && !!d.querySelector('#channelOwner') && !!d.querySelector('#channelImageIds') && !!d.querySelector('#channelTagId'));
  ok('分配策略使用服务端 JSON DTO', !!d.querySelector('#channelAssignmentMode') && !!d.querySelector('#channelAssignmentStrategy') && !!d.querySelector('#channelAssignmentConfig'));
  d.querySelector('#channelName').value = '新客渠道';
  d.querySelector('#channelCode').value = 'new-customer';
  d.querySelector('#channelImageIds').value = 'not-an-id';
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '保存渠道'));
  await sleep(30);
  ok('无效素材引用在发请求前被阻止', d.body.textContent.includes('素材引用必须是正整数 ID'));
  dom.window.close();
}

console.log('admin/ownerMig.html（本地安全 CSV 迁移边界）');
{
  const dom = await loadPage('admin/ownerMig.html');
  const d = dom.window.document;
  const csv = d.querySelector('#ownerMigCsv');
  ok('仅接受 CSV 且不再显示企微转接/欢迎语控件', csv?.getAttribute('accept')?.includes('.csv') && !d.body.textContent.includes('同时发起企微转接') && !d.body.textContent.includes('转接欢迎语'));
  ok('初始明确为空且真实动作均已绑定', d.body.textContent.includes('尚未生成迁移预览，不会发送执行请求') && [...d.querySelectorAll('button')].filter((b) => b.__dcBound).length >= 2);
  dom.window.close();
}

/* ================= H5 ================= */
console.log('h5/all.html');
{
  const dom = await loadPage('h5/all.html');
  const d = dom.window.document;
  const opts = [...d.querySelectorAll('label')].filter((l) => l.__dcBound);
  ok('单选+多选选项均已绑定点击', opts.length >= 9);
  const before = d.body.innerHTML;
  opts[1].dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
  await sleep(30);
  ok('点击单选切换选中态（重渲染）', d.body.innerHTML !== before);
  dom.window.close();
}

console.log('h5/one.html');
{
  const dom = await loadPage('h5/one.html');
  const d = dom.window.document;
  ok('显示第 3 / 12 题', d.body.textContent.includes('第 3 / 12 题'));
  const next = [...d.querySelectorAll('button')].find((b) => b.textContent === '下一题');
  next.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
  await sleep(30);
  ok('下一题 → 第 4 题', d.body.textContent.includes('第 4 / 12 题'));
  dom.window.close();
}

/* ================= 侧边栏 ================= */
console.log('sidebar/index.html');
{
  const dom = await loadPage('sidebar/index.html');
  const d = dom.window.document;
  ok('侧边栏渲染（含 WX 顶栏）', d.body.textContent.includes('WX'));
  dom.window.close();
}

console.log('sidebar/index.html（问卷读取状态）');
for (const scenario of ['success', 'empty', 'error']) {
  const dom = await loadPage('sidebar/index.html', { q: 'external_userid=ext-7&sidebar_case=' + scenario });
  const d = dom.window.document;
  const questionnaireTab = d.querySelector('[data-sidebar-tab="questionnaires"]');
  ok('workbench ready 后问卷 tab 可用', questionnaireTab && !questionnaireTab.disabled);
  ok('真实 Sidebar tabs 可用、未接入能力仍关闭',
    !d.querySelector('[data-sidebar-tab="timeline"]').disabled &&
    !d.querySelector('[data-sidebar-tab="chat_activity"]').disabled &&
    !d.querySelector('[data-sidebar-tab="orders"]').disabled &&
    !d.querySelector('[data-sidebar-tab="periodic_orders"]').disabled &&
    !d.querySelector('[data-sidebar-tab="materials"]').disabled &&
    !d.querySelector('[data-sidebar-tab="products"]').disabled &&
    !d.querySelector('[data-sidebar-tab="other_staff_messages"]').disabled &&
    d.querySelector('[data-sidebar-tab="coupons"]').disabled);
  click(dom, questionnaireTab);
  ok('问卷切换先显示 loading', d.body.textContent.includes('正在读取问卷答案'));
  await sleep(30);
  if (scenario === 'success') {
    ok('问卷真实读取并可展开答案', d.body.textContent.includes('展开答案（1）') && !!d.querySelector('.questionnaire-answers'));
  } else if (scenario === 'empty') {
    ok('问卷空结果显示 empty', d.body.textContent.includes('暂无问卷回答记录'));
  } else {
    ok('问卷失败显示 error 与重试', d.body.textContent.includes('问卷读取失败') && d.body.textContent.includes('重试读取问卷'));
  }
  dom.window.close();
}

console.log('sidebar/index.html（V2 安全活动、订单、素材与周期备注）');
{
  const dom = await loadPage('sidebar/index.html', { q: 'external_userid=ext-7&sidebar_case=success' });
  const d = dom.window.document;

  input(dom, d.querySelector('#sidebar-phone-input'), '+8613800138000');
  click(dom, d.querySelector('[data-sidebar-action="bind-phone"]'));
  await sleep(30);
  ok('手机号只绑定当前 Sidebar 客户并携带幂等键',
    d.querySelector('#sidebar-phone-status')?.textContent.includes('本地事实') &&
    dom.window.__sidebarTest.phoneBody?.mobile === '+8613800138000' &&
    dom.window.__sidebarTest.phoneKey?.startsWith('sidebar-phone-'));

  click(dom, d.querySelector('[data-sidebar-tab="timeline"]'));
  await sleep(30);
  ok('时间线只展示安全事件元数据',
    d.querySelectorAll('[data-timeline-event-id]').length === 1 &&
    d.querySelector('[data-sidebar-section="timeline"]')?.textContent.includes('survey_submitted') &&
    !d.querySelector('[data-sidebar-section="timeline"]')?.textContent.includes('payload') &&
    !d.querySelector('[data-sidebar-section="timeline"]')?.textContent.includes('actor'));
  ok('问卷来源事件只导航到已加载问卷板块', !!d.querySelector('[data-sidebar-action="open-related-questionnaires"]'));
  click(dom, d.querySelector('[data-sidebar-action="timeline-more"]'));
  await sleep(30);
  ok('时间线使用 opaque cursor 加载更多', d.querySelectorAll('[data-timeline-event-id]').length === 2);

  click(dom, d.querySelector('[data-sidebar-tab="chat_activity"]'));
  await sleep(30);
  ok('聊天活动独立标注 V2 补充能力且不展示正文',
    d.querySelector('[data-sidebar-capability="v2-supplement"]')?.textContent.includes('不计 LEGACY-S05-028 销项') &&
    d.querySelectorAll('[data-chat-activity-at]').length === 1 &&
    !d.body.textContent.includes('消息正文'));
  const chatFilter = d.querySelector('[data-chat-filter="chat_type"]');
  chatFilter.value = 'group';
  chatFilter.dispatchEvent(new dom.window.Event('change', { bubbles: true }));
  await sleep(30);
  ok('聊天活动支持私聊/群聊筛选', d.body.textContent.includes('群聊 · text'));

  click(dom, d.querySelector('[data-sidebar-tab="orders"]'));
  await sleep(30);
  const orderCard = d.querySelector('[data-order-no="M20260826001"]');
  ok('普通订单渲染安全订单字段',
    orderCard?.textContent.includes('测试课程') &&
    orderCard?.textContent.includes('99.00 CNY') &&
    !orderCard?.textContent.includes('payer_name'));
  const orderDetail = d.querySelector('[data-order-detail="local"]');
  ok('普通订单详情在当前客户范围内本地展开',
    orderDetail?.tagName === 'DETAILS' &&
    orderDetail?.textContent.includes('订单号 M20260826001') &&
    orderDetail?.textContent.includes('商品编码 course-1') &&
    !orderDetail?.textContent.includes('/api/admin/orders/'));

  click(dom, d.querySelector('[data-sidebar-tab="periodic_orders"]'));
  await sleep(30);
  ok('周期订单渲染 canonical member 与版本',
    d.querySelectorAll('[data-periodic-member-ref]').length === 1 &&
    d.body.textContent.includes('member_ref spm_') &&
    d.body.textContent.includes('version 1'));
  input(dom, d.querySelector('[data-periodic-remark]'), '更新后的备注');
  click(dom, d.querySelector('[data-sidebar-action="periodic-remark-save"]'));
  await sleep(30);
  ok('周期备注写入回执含 accepted 与新 CAS 版本',
    d.querySelector('[data-periodic-remark-receipt="accepted"]')?.textContent.includes('version 2') &&
    dom.window.__sidebarTest.remarkBody?.expected_version === 1 &&
    dom.window.__sidebarTest.remarkBody?.remark === '更新后的备注' &&
    typeof dom.window.__sidebarTest.idempotencyKey === 'string' &&
    dom.window.__sidebarTest.idempotencyKey.startsWith('sidebar-periodic-remark-'));

  click(dom, d.querySelector('[data-sidebar-tab="products"]'));
  await sleep(30);
  ok('可分享商品同时渲染普通与周期商品，且只显示本地字段',
    d.querySelectorAll('article[data-product-kind="ordinary"]').length === 1 &&
    d.querySelectorAll('article[data-product-kind="service_period"]').length === 1 &&
    d.body.textContent.includes('普通课程') && d.body.textContent.includes('周期课程'));
  click(dom, d.querySelector('[data-sidebar-action="send-product"]'));
  await sleep(30);
  const productMessage = dom.window.__sidebarTest.wxMessages.find((entry) => entry.payload?.msgtype === 'news');
  ok('商品卡片仅以 JSSDK 回调为 receipt，明确 delivery_unknown',
    productMessage?.method === 'sendChatMessage' &&
    productMessage?.payload?.news?.link === 'http://localhost/p/ordinary/41' &&
    d.body.textContent.includes('client_callback · JSSDK 已回调') &&
    d.body.textContent.includes('delivery_unknown · 未取得企微外部送达回执'));

  click(dom, d.querySelector('[data-sidebar-tab="materials"]'));
  await sleep(30);
  ok('素材支持搜索/分类/标签筛选与元数据',
    !!d.querySelector('#material-q') && !!d.querySelector('#material-category') && !!d.querySelector('#material-tags') &&
    d.querySelectorAll('article[data-material-id]').length === 2 &&
    d.body.textContent.includes('welcome.png') && d.body.textContent.includes('800×600'));
  click(dom, d.querySelector('[data-sidebar-action="send-material-image"]'));
  await sleep(30);
  const imageMessage = dom.window.__sidebarTest.wxMessages.find((entry) => entry.payload?.msgtype === 'image');
  ok('图片先取得临时 media_id 再调用 JSSDK，receipt 不宣称外部送达',
    dom.window.__sidebarTest.temporaryKeys[0]?.startsWith('sidebar-image-temporary-media-') &&
    imageMessage?.method === 'sendChatMessage' &&
    imageMessage?.payload?.image?.mediaid === 'media-real-31' &&
    d.body.textContent.includes('delivery_unknown · 未取得企微外部送达回执'));
  input(dom, d.querySelector('#material-q'), '欢迎');
  input(dom, d.querySelector('#material-category'), '海报');
  input(dom, d.querySelector('#material-tags'), '欢迎语');
  click(dom, d.querySelector('[data-sidebar-action="materials-search"]'));
  await sleep(30);
  ok('素材筛选请求沿用真实 q/category/tags 参数',
    dom.window.__sidebarTest.materialQueries.some((url) => url.includes('q=%E6%AC%A2%E8%BF%8E') && url.includes('category=%E6%B5%B7%E6%8A%A5') && url.includes('tags=%E6%AC%A2%E8%BF%8E%E8%AF%AD')));
  ok('缩略图展示真实本地图片或明确 not_found',
    d.querySelector('[data-thumbnail-status="ready"]') &&
    d.querySelector('[data-thumbnail-status="not_found"]') &&
    d.querySelector('[data-material-preview="ready"]')?.getAttribute('src') === 'blob:sidebar-thumbnail');
  dom.window.close();
}

console.log('sidebar/index.html（新增能力空态与失败态）');
{
  const empty = await loadPage('sidebar/index.html', { q: 'external_userid=ext-7&sidebar_case=empty' });
  const emptyDoc = empty.window.document;
  for (const tab of ['timeline', 'chat_activity', 'orders', 'periodic_orders', 'products', 'materials']) {
    click(empty, emptyDoc.querySelector(`[data-sidebar-tab="${tab}"]`));
    await sleep(30);
    ok(`${tab} 空态清晰`, emptyDoc.body.textContent.includes(tab === 'timeline' ? '暂无时间线记录' : tab === 'chat_activity' ? '暂无聊天活动记录' : tab === 'orders' ? '暂无普通订单记录' : tab === 'periodic_orders' ? '暂无周期订单记录' : tab === 'products' ? '暂无可分享的已启用商品' : '暂无匹配素材'));
  }
  empty.window.close();

  const failed = await loadPage('sidebar/index.html', { q: 'external_userid=ext-7&sidebar_case=error' });
  const failedDoc = failed.window.document;
  click(failed, failedDoc.querySelector('[data-sidebar-tab="timeline"]'));
  await sleep(30);
  ok('时间线失败态提供重试', failedDoc.body.textContent.includes('时间线读取失败') && !!failedDoc.querySelector('[data-sidebar-action="retry-timeline"]'));
  click(failed, failedDoc.querySelector('[data-sidebar-tab="periodic_orders"]'));
  await sleep(30);
  ok('周期订单失败态提供重试', failedDoc.body.textContent.includes('周期订单读取失败') && !!failedDoc.querySelector('[data-sidebar-action="retry-periodic-orders"]'));
  failed.window.close();
}

console.log('admin/campaigns.html（External Effects / Push Center 本地边界）');
{
  const effects = await loadPage('admin/campaigns.html', { q: 'view=external-effects' });
  await sleep(40);
  const doc = effects.window.document;
  const effectsText = doc.querySelector('#stage')?.textContent || '';
  ok('External Effects 显示本地事实边界与 outcome_unknown 人工确认',
    effectsText.includes('不证明 Provider 调用、外部发送或送达') &&
    effectsText.includes('outcome_unknown 存在') &&
    effectsText.includes('不得自动重试'));
  ok('Push Center 把 sent 标为本地状态，且未知结果没有重试操作',
    effectsText.includes('sent（本地状态）') &&
    effectsText.includes('结果未知：人工确认，禁止重试') &&
    !doc.querySelector('[data-push-retry="18"]'));
  ok('缺失 run-due 契约明确 backend_blocked', effectsText.includes('backend_blocked') && effectsText.includes('run-due'));
  effects.window.close();

  const detail = await loadPage('admin/campaigns.html', { q: 'view=external-effects&job=18' });
  await sleep(40);
  const detailDoc = detail.window.document;
  const detailText = detailDoc.querySelector('#stage')?.textContent || '';
  ok('Push Center job 详情只呈现本地 attempt/控制回执，不泄露收件人字段',
    detailText.includes('Push Center job 本地对账') &&
    detailText.includes('结果未知：需人工确认，禁止重试') &&
    !detailText.includes('customer_id') &&
    !detailText.includes('owner_staff_id'));
  detail.window.close();
}

console.log('admin/campaigns.html（运营计划全局本地审核列表）');
{
  const plans = await loadPage('admin/campaigns.html', { q: 'legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans' });
  await sleep(40);
  const doc = plans.window.document;
  let text = doc.querySelector('#stage')?.textContent || '';
  ok('运营计划入口读取真实全局计划索引并保留本地边界',
    text.includes('运营计划本地审核') && text.includes('spring-campaign') && text.includes('pending_review') &&
    text.includes('不代表发送、Provider 执行或送达') && plans.window.__planIndexCalls.some((url) => url.includes('/api/admin/cloud-orchestrator/plans')));
  input(plans, doc.querySelector('#plan-index-status'), 'pending_review');
  click(plans, doc.querySelector('#plan-index-refresh'));
  await sleep(40);
  text = doc.querySelector('#stage')?.textContent || '';
  ok('审核状态筛选传给真实计划索引且页面不展示收件人或消息正文',
    plans.window.__planIndexCalls.some((url) => url.includes('review_status=pending_review')) &&
    !text.includes('customer_ids') && !text.includes('message_override'));
  plans.window.close();
}

console.log('admin/campaigns.html（目标人员 Customer360 链接）');
{
  const planID = 'ctp_' + 'a'.repeat(64);
  const recipient = await loadPage('admin/campaigns.html', { q: `campaign=spring-campaign&plan=${planID}&recipient=7` });
  await sleep(40);
  const doc = recipient.window.document;
  const text = doc.querySelector('#stage')?.textContent || '';
  const customer360 = doc.querySelector('a[href="customerDetail.html?id=7"]');
  ok('已验证的 plan 目标只用 canonical OneID 链接既有 Customer360 档案',
    customer360?.textContent === '在 Customer360 查看档案' &&
    recipient.window.__recipientCalls.some((url) => url.endsWith('/recipients/7')));
  ok('目标人员列表没有可信状态投影时保持无成员状态筛选',
    text.includes('当前契约不含昵称、成员状态或消息任务') &&
    !doc.querySelector('[data-recipient-status]') &&
    recipient.window.__recipientCalls.every((url) => !url.includes('status=')));
  recipient.window.close();
}

console.log('admin/campaigns.html（trace_id 可观察性边界）');
{
  const observability = await loadPage('admin/campaigns.html', { q: 'view=observability' });
  await sleep(40);
  const doc = observability.window.document;
  let text = doc.querySelector('#stage')?.textContent || '';
  ok('无 trace_id 时刷新本地聚合，并明确没有 audit JSON',
    text.includes('未输入 trace_id') && text.includes('当前没有可渲染的 audit JSON') &&
    observability.window.__observabilityCalls.length === 2 && observability.window.__observabilityCalls.every((call) => !call.url.includes('trace_id=')));
  input(observability, doc.querySelector('#observability-trace'), 'trace-audit-7');
  click(observability, doc.querySelector('#observability-filter'));
  await sleep(40);
  text = doc.querySelector('#stage')?.textContent || '';
  ok('trace_id 只筛选真实 Push Center sections/stats，不伪造 session/audit',
    text.includes('已以 trace-audit-7 调用真实 Push Center sections/stats 聚合') &&
    observability.window.__observabilityCalls.some((call) => call.url.includes('trace_id=trace-audit-7')) &&
    observability.window.__observabilityCalls.every((call) => !call.url.includes('session_id')));
  observability.window.close();

  const session = await loadPage('admin/campaigns.html', { q: 'view=observability&session_id=session-7' });
  await sleep(40);
  const sessionText = session.window.document.querySelector('#stage')?.textContent || '';
  ok('session_id 缺契约时 fail closed 且不降级为全局查询',
    sessionText.includes('backend_blocked') && sessionText.includes('session_id') && session.window.__observabilityCalls.length === 0);
  session.window.close();
}

console.log(`\n${pass} 通过 / ${fail} 失败`);
process.exit(fail ? 1 : 0);
