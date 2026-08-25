import { apiRequestOptions, ApiError, request, unwrapGenerated } from './transport';
import { sidebarApi } from './sidebar';

function assert(ok: unknown, message: string): asserts ok {
  if (!ok) throw new Error(message);
}

export async function runTransportContractTests(): Promise<void> {
  const options = apiRequestOptions({ method: 'POST', headers: { 'X-Request-ID': 'test' } }, 'csrf_token=token%201; session=x');
  const headers = new Headers(options.headers);
  assert(options.credentials === 'include', 'same-origin cookie credentials must be included');
  assert(!(options.headers instanceof Headers), 'generated client options must use enumerable header records');
  assert(headers.get('X-CSRF-Token') === 'token 1', 'CSRF cookie must become X-CSRF-Token');
  assert(headers.get('X-Request-ID') === 'test', 'caller headers must survive transport');
  assert(unwrapGenerated({ status: 200, data: { cursor: 'opaque' } }).cursor === 'opaque', '2xx generated response must unwrap');
  try {
    unwrapGenerated({ status: 403, data: { code: 'forbidden' } });
    throw new Error('403 was accepted');
  } catch (error) {
    assert(error instanceof ApiError && error.kind === 'forbidden', '403 must be a structured forbidden error');
  }

  const originalFetch = globalThis.fetch;
  const sidebarRequests: Array<{ input: string; init?: RequestInit }> = [];
  globalThis.fetch = async (input, init) => {
    sidebarRequests.push({ input: String(input), init });
    const data = String(input).includes('chat-activity')
      ? { items: [{ chat_type: 'private', message_type: 'text', sent_at: '2026-08-26T01:00:00Z' }], safety: { local_only: true, provider_execution_eligible: false, real_external_call_executed: false } }
      : { items: [{ id: 7, event_type: 'survey_submitted', occurred_at: '2026-08-26T00:00:00Z' }], next_cursor: 'next-opaque', safety: { local_only: true, provider_execution_eligible: false, real_external_call_executed: false } };
    return new Response(JSON.stringify(data), { status: 200, headers: { 'Content-Type': 'application/json' } });
  };
  try {
    const timeline = await sidebarApi.timeline('sidebar-context', { limit: 20 });
    const chat = await sidebarApi.chatActivity('sidebar-context', { chat_type: 'private', limit: 10 });
    assert(timeline.items[0]?.event_type === 'survey_submitted' && timeline.next_cursor === 'next-opaque', 'Sidebar timeline response must retain safe DTO and cursor');
    assert(chat.items[0]?.chat_type === 'private', 'Sidebar chat activity response must retain safe metadata DTO');
    assert(sidebarRequests[0]?.input === '/api/sidebar/v2/timeline?limit=20', 'Sidebar timeline must use generated GET URL');
    assert(sidebarRequests[1]?.input === '/api/sidebar/v2/chat-activity?chat_type=private&limit=10', 'Sidebar chat activity must use generated GET URL');
    for (const call of sidebarRequests) {
      assert(call.init?.method === 'GET', 'Sidebar activity reads must use GET');
      assert(new Headers(call.init?.headers).get('X-Sidebar-Context-Token') === 'sidebar-context', 'Sidebar activity reads must carry scoped context token');
      assert(call.init?.credentials === 'include', 'Sidebar activity reads must include same-origin credentials');
    }
  } finally {
    globalThis.fetch = originalFetch;
  }

  let seen: RequestInit | undefined;
  globalThis.fetch = async (_input, init) => {
    seen = init;
    return new Response(JSON.stringify({ code: 'csrf_invalid' }), { status: 401, headers: { 'Content-Type': 'application/json' } });
  };
  try {
    await request('/api/v1/example', { method: 'PUT', headers: { 'X-CSRF-Token': 'explicit' } });
    throw new Error('401 was accepted');
  } catch (error) {
    assert(error instanceof ApiError && error.kind === 'unauthenticated', '401 must be a structured unauthenticated error');
    assert(seen?.credentials === 'include', 'direct fetch must include credentials');
    assert(new Headers(seen?.headers).get('X-CSRF-Token') === 'explicit', 'explicit CSRF header must not be overwritten');
  } finally {
    globalThis.fetch = originalFetch;
  }
}
