import { readPushTraceObservabilityDto } from './push_observability';

function assert(ok: unknown, message: string): asserts ok { if (!ok) throw new Error(message); }

const sections = (traceID?: string) => ({ ok: true, sections: [{ key: 'order', label: '订单', count: 2 }], status_definitions: [], filters: traceID ? { trace_id: traceID } : {}, route_owner: 'ai_crm_next' });
const stats = (traceID?: string) => ({ ok: true, counts: { total: 2, pending: 1, running: 0, succeeded: 0, sent: 1, failed: 0, shadow_warning: 0, by_effective_status: {}, by_status: {}, by_section: {} }, sections: [], status_definitions: [], filters: traceID ? { trace_id: traceID } : {}, route_owner: 'ai_crm_next', real_external_call_executed: false, runtime_queue: {}, capability_owner: 'ai_crm_next/platform_foundation/push_center' });

export async function runPushObservabilityAdapterTests(): Promise<void> {
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  const savedFetch = globalThis.fetch;
  globalThis.fetch = async (input, init) => {
    const url = String(input);
    calls.push({ url, init });
    const traceID = new URL('http://localhost' + url).searchParams.get('trace_id') || undefined;
    return new Response(JSON.stringify(url.includes('/stats') ? stats(traceID) : sections(traceID)), { status: 200 });
  };
  try {
    const filtered = await readPushTraceObservabilityDto(' trace-audit-7 ');
    assert(filtered.traceID === 'trace-audit-7' && filtered.counts?.sent === 1 && filtered.sections[0].key === 'order', 'trace_id maps existing Push Center aggregation only');
    assert(calls.length === 2 && calls.every((call) => call.init?.method === 'GET' && call.url.includes('trace_id=trace-audit-7') && !call.url.includes('session_id')), 'trace_id is sent to generated sections/stats and session_id is never invented');
    calls.length = 0;
    await readPushTraceObservabilityDto();
    assert(calls.length === 2 && calls.every((call) => !call.url.includes('trace_id=')), 'unfiltered refresh remains a real global summary without a fake audit request');
  } finally { globalThis.fetch = savedFetch; }

  globalThis.fetch = async (input) => {
    const url = String(input);
    return new Response(JSON.stringify(url.includes('/stats') ? { ...stats('trace-audit-7'), real_external_call_executed: true } : sections('trace-audit-7')), { status: 200 });
  };
  try { await readPushTraceObservabilityDto('trace-audit-7'); assert(false, 'unsafe Push Center stats must fail closed'); }
  catch (error) { assert(error instanceof Error && error.message.includes('本地读模型边界'), 'unsafe Push Center stats fails closed'); }
  finally { globalThis.fetch = savedFetch; }

  globalThis.fetch = async (input) => {
    const url = String(input);
    const degraded = { ok: true, degraded: true, error: '', error_code: 'production_read_unavailable', source_status: 'production_unavailable', read_model_status: 'unavailable', capability_owner: 'ai_crm_next/platform_foundation/push_center', page_error: '推送中心读模型暂不可用，请稍后重试。', diagnostics: {}, route_owner: 'ai_crm_next', fallback_used: false, real_external_call_executed: false, status_code: 200, items: [], total: 0, counts: { total: 0, pending: 0, running: 0, sent: 0, failed: 0, by_effective_status: {}, by_status: {}, by_section: {} }, status_definitions: [], filters: {}, limit: 50, offset: 0, sections: [], runtime_queue: {} };
    return new Response(JSON.stringify(url.includes('/stats') ? degraded : degraded), { status: 200 });
  };
  try {
    const unavailable = await readPushTraceObservabilityDto();
    assert(unavailable.degraded && unavailable.counts === null && unavailable.sections.length === 0, 'degraded Push Center response never becomes zero audit data');
  } finally { globalThis.fetch = savedFetch; }
}
