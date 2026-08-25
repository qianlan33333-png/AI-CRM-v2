import { apiRequestOptions, ApiError, request, unwrapGenerated } from './transport';

function assert(ok: unknown, message: string): asserts ok {
  if (!ok) throw new Error(message);
}

export async function runTransportContractTests(): Promise<void> {
  const options = apiRequestOptions({ method: 'POST', headers: { 'X-Request-ID': 'test' } }, 'csrf_token=token%201; session=x');
  const headers = new Headers(options.headers);
  assert(options.credentials === 'include', 'same-origin cookie credentials must be included');
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
