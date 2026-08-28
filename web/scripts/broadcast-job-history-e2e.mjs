import { build } from 'esbuild';
import { JSDOM } from 'jsdom';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const out = fs.mkdtempSync(path.join(os.tmpdir(), 'aicrm-broadcast-job-history-'));
const output = path.join(out, 'broadcastJobHistory.js');
const saved = { fetch: globalThis.fetch, document: globalThis.document, window: globalThis.window, location: globalThis.location };
const sleep = () => new Promise((resolve) => setTimeout(resolve, 0));

try {
  await build({ entryPoints: [path.join(root, 'src/admin/sections/broadcastJobHistory.ts')], bundle: true, platform: 'node', format: 'esm', outfile: output, logLevel: 'warning' });
  const { mountBroadcastJobHistory } = await import(pathToFileURL(output).href);
  const dom = new JSDOM('<main id="stage"></main>', { url: 'https://test.invalid/admin/broadcast-jobs.html?broadcast_job_history=1', pretendToBeVisual: true });
  Object.assign(globalThis, { document: dom.window.document, window: dom.window, location: dom.window.location });
  const stage = document.querySelector('#stage');
  const stamp = '2026-08-28T01:02:03.123456Z';
  const calls = []; let fail = false; let privateField = false; let empty = false;
  const job = (id) => ({ id, source_id: id + 100, original_source_type: '', source_table: 'legacy_broadcast_jobs', scheduled_for: stamp, priority: -2, original_status: '', requires_approval: false, approved_at: null, cancelled_at: null, target_count: 0, content_type: '', attempt_count: -1, sent_count: 0, failed_count: 0, created_at: stamp, updated_at: stamp, claimed_at: null, sent_at: null, lease_expires_at: null, business_domain: null, channel: null, target_kind: null, failure_type: null, max_attempts: -1, next_retry_at: null, dispatch_started_at: null, original_original_side_effect_executed: false, original_original_provider_result_received: false, original_original_reconciliation_required: false, completed_at: null, hold_at: null });
  globalThis.fetch = async (input, init = {}) => {
    const url = new URL(String(input), 'https://test.invalid'); calls.push({ url, init });
    if (fail) return new Response(JSON.stringify({ code: 'unavailable' }), { status: 503 });
    const detail = /^\/api\/admin\/broadcast-job-history\/[1-9]\d*$/.test(url.pathname);
    const id = Number(url.pathname.split('/').at(-1)); const limit = Number(url.searchParams.get('limit')); const offset = Number(url.searchParams.get('offset'));
    const item = privateField ? { ...job(detail ? id : 1), provider_token: 'private-should-never-render' } : job(detail ? id : 1);
    const body = { source: 'v1_history', read_only: true, real_external_call_executed: false, ...(detail ? { item } : { items: empty ? [] : Array.from({ length: Math.min(limit, Math.max(0, 51 - offset)) }, (_, index) => job(offset + index + 1)), total: empty ? 0 : 51, limit, offset }) };
    return new Response(JSON.stringify(body), { status: 200 });
  };

  await mountBroadcastJobHistory(stage);
  if (stage.querySelectorAll('tbody tr').length !== 50 || !stage.textContent.includes('原始任务观察') || [...stage.querySelectorAll('button')].some((button) => /发送|重试/.test(button.textContent))) throw new Error('list is not read-only or does not use the default page size');
  stage.querySelector('[data-broadcast-job-history-next]').click(); await sleep();
  if (calls.at(-1).url.pathname !== '/api/admin/broadcast-job-history' || calls.at(-1).url.search !== '?limit=50&offset=50' || !stage.textContent.includes('历史 #51')) throw new Error('pagination did not request the generated list endpoint');
  await mountBroadcastJobHistory(stage, { historyID: '7' });
  if (!stage.querySelector('[data-broadcast-job-history-id="7"]') || !stage.textContent.includes('源任务 #107') || calls.at(-1).url.pathname !== '/api/admin/broadcast-job-history/7') throw new Error('detail did not use exact history ID');
  privateField = true; await mountBroadcastJobHistory(stage, { historyID: '7' });
  if (!stage.querySelector('[role="alert"]') || stage.textContent.includes('private-should-never-render')) throw new Error('private response field was rendered');
  privateField = false; empty = true; await mountBroadcastJobHistory(stage);
  if (!stage.textContent.includes('暂无历史记录')) throw new Error('empty result was not rendered');
  empty = false; fail = true; await mountBroadcastJobHistory(stage);
  if (!stage.querySelector('[role="alert"]') || stage.textContent.includes('unavailable')) throw new Error('failed result retained raw or fallback data');
  if (!calls.every(({ init }) => init.method === 'GET' && init.credentials === 'include' && init.body === undefined)) throw new Error('history UI made a non-GET request');
  console.log('broadcast-job-history-e2e: PASS');
} finally {
  globalThis.fetch = saved.fetch;
  Object.assign(globalThis, { document: saved.document, window: saved.window, location: saved.location });
  fs.rmSync(out, { recursive: true, force: true });
}
