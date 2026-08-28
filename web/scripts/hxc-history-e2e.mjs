import { build } from 'esbuild';
import { JSDOM } from 'jsdom';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const out = fs.mkdtempSync(path.join(os.tmpdir(), 'aicrm-hxc-history-'));
const output = path.join(out, 'hxcHistory.js');
const adapterOutput = path.join(out, 'hxcHistory.test.js');
await build({ entryPoints: [path.join(root, 'src/api/hxcHistory.test.ts')], bundle: true, platform: 'node', format: 'esm', outfile: adapterOutput, logLevel: 'warning' });
const { runHxcHistoryAdapterTests } = await import(pathToFileURL(adapterOutput).href);
await runHxcHistoryAdapterTests();
await build({ entryPoints: [path.join(root, 'src/admin/sections/hxcHistory.ts')], bundle: true, platform: 'node', format: 'esm', outfile: output, logLevel: 'warning' });
const { mountHXCHistory } = await import(pathToFileURL(output).href);
const dom = new JSDOM('<main id="stage"></main>', { url: 'https://test.invalid/funnel.html?hxc_history=1&history_kind=snapshot' });
const saved = { fetch: globalThis.fetch, document: globalThis.document, window: globalThis.window, location: globalThis.location };
Object.assign(globalThis, { document: dom.window.document, window: dom.window, location: dom.window.location });
const digest = Array(32).fill(2), stamp = '2026-08-28T01:02:03.123456Z';
const item = { id: 4, source_id: -7, source_key_digest: digest, source_payload_digest: digest, customer_id: null, observation: 'observed_snapshot', observed_at: stamp, in_lead_pool: false, in_people: false, in_questionnaire: false, class_term_no: null, class_term_label: '<tag>', crm_hxc_state: '', crm_created_at: '2024-02-29', last_questionnaire_at: null, hxc_member_hit: false, hxc_user_hit: false, funnel_state: '', hxc_member_status: '', hxc_registered_at: null, hxc_last_login_at: null, membership_type: '', membership_status: '', membership_end_at: null, membership_days_left: null, consultation_used: null, consultation_limit: null, conversation_chat: 0, conversation_consult: 0, conversation_lesson: 0, messages_user: 0, messages_ai: 0, consult_completed: 0, last_message_at: null, subscription_tier: '', subscription_expires: null, subscription_quota: null, subscription_used: null, subscription_period_start: null };
const calls = []; let fail = false;
globalThis.fetch = async (input, init = {}) => { const url = new URL(String(input), 'https://test.invalid'); const offset = Number(url.searchParams.get('offset') ?? '0'); calls.push({ url: String(input), init }); const items = offset === 0 ? Array.from({ length: 20 }, (_, index) => ({ ...item, id: index + 1 })) : [{ ...item, id: 21 }]; return new Response(fail ? '{"raw":"NO"}' : JSON.stringify({ source: 'v1_history', read_only: true, real_external_call_executed: false, items, total: 21, limit: 20, offset }), { status: fail ? 503 : 200 }); };
try {
  const stage = document.querySelector('#stage'); await mountHXCHistory(stage);
  if (!stage.textContent.includes('2024-02-29') || stage.innerHTML.includes('<tag>') || stage.querySelectorAll('[data-hxc-kind]').length !== 5 || !Array.from(stage.querySelectorAll('[data-hxc-kind]')).every((link) => link.getAttribute('href')?.startsWith('funnel.html?hxc_history=1&history_kind=')) || stage.querySelector('input,textarea') || Array.from(stage.querySelectorAll('button')).some((x) => /群发|刷新漏斗|权益/.test(x.textContent))) throw new Error('historical snapshot DOM boundary failed');
  fail = true; stage.querySelector('[data-hxc-next]').click(); await new Promise((resolve) => setTimeout(resolve, 0));
  if (!stage.querySelector('[role="alert"]') || stage.textContent.includes('NO')) throw new Error('failed page retained data or raw error');
  if (!calls.every((x) => x.init.method === 'GET' && x.init.credentials === 'include' && x.init.body === undefined)) throw new Error('non-GET history request');
  console.log('hxc-history-e2e: PASS');
} finally { globalThis.fetch = saved.fetch; Object.assign(globalThis, { document: saved.document, window: saved.window, location: saved.location }); dom.window.close(); fs.rmSync(out, { recursive: true, force: true }); }
