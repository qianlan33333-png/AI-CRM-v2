import { JSDOM } from 'jsdom';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildTestBrowserBundle } from './test-browser-bundle.mjs';

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const html = fs.readFileSync(path.join(root, 'dist/admin/productForm.html'), 'utf8');
const bundle = await buildTestBrowserBundle(path.join(root, 'src/admin/main.ts'));
const calls = [];
const projection = {
  schema_version: 1, status: 'draft', enabled: false, buy_button_text: '购买课程', require_mobile: false,
  lead_program_id: null, lead_channel_id: null, lead_qr_title: '', lead_qr_subtitle: '', completion_redirect_enabled: false,
  completion_redirect_url: '', completion_target: null, wecom_tagging: {}, slices: [],
};
const product = (patch = {}) => ({
  id: 21, product_code: 'P-21', name: '课程', description: '说明', price_minor: 19900, currency: 'CNY', stock_quantity: 9,
  images: ['/api/admin/image-library/8/variants/original'], admin_projection: projection, created_by: 1,
  created_at: '2026-08-30T00:00:00Z', updated_at: '2026-08-30T00:00:00Z', version: 1, ...patch,
});
const json = (body, status = 200) => ({ ok: status >= 200 && status < 300, status, headers: new Headers({ 'Content-Type': 'application/json' }), text: async () => JSON.stringify(body) });
const dom = new JSDOM(html, {
  url: 'http://localhost/admin/productForm.html?id=21', runScripts: 'outside-only', pretendToBeVisual: true,
  beforeParse(window) {
    window.__AICRM_TEST_MOCK__ = false;
    window.document.cookie = `aicrm_csrf=${'c'.repeat(43)}`;
    window.fetch = async (input, init = {}) => {
      const url = new URL(String(input), window.location.origin); const method = init.method || 'GET';
      calls.push({ path: url.pathname, method, headers: new Headers(init.headers), body: init.body ? JSON.parse(String(init.body)) : undefined });
      if (url.pathname === '/api/admin/channels') return json({ channels: [] });
      if (url.pathname === '/api/v1/products' && method === 'GET') return json({ items: [product()] });
      if (url.pathname === '/api/v1/products/21/local-entitlements') return json({ items: [] });
      if (url.pathname === '/api/admin/wechat-pay/products/21/external-push') {
        if (method === 'PUT') return json({ product_id: 21, product_kind: 'wechat_pay', enabled: true, configuration_reference: 'paid-course-21', updated_at: '2026-08-30T00:01:00Z' });
        return json({ product_id: 21, product_kind: 'wechat_pay', enabled: false, updated_at: '2026-08-30T00:00:00Z' });
      }
      if (url.pathname === '/api/v1/products/21' && method === 'PUT') return json(product({ name: '课程新版', images: init.body ? JSON.parse(String(init.body)).images : [], admin_projection: init.body ? JSON.parse(String(init.body)).admin_projection : projection, version: 2 }));
      if (url.pathname === '/api/v1/products/21') return json(product());
      return json({ code: 'unexpected_request', path: url.pathname }, 500);
    };
  },
});
dom.window.eval(bundle);
await new Promise((resolve) => setTimeout(resolve, 60));
const document = dom.window.document;
if (!document.querySelector('#pfImages') || !document.querySelector('#pfWecomTagging') || !document.querySelector('#pfExternalPushReference') || document.body.textContent.includes('后端能力未就绪')) throw new Error('普通商品完整编辑表单未渲染');
document.querySelector('#pfName').value = '课程新版';
document.querySelector('#pfImages').value = 'https://cdn.example.test/course.png';
document.querySelector('#pfBuyButtonText').value = '立即购买';
document.querySelector('#pfRequireMobile').value = 'true';
document.querySelector('#pfCompletionRedirectEnabled').value = 'true';
document.querySelector('#pfCompletionRedirectUrl').value = 'https://example.test/complete';
document.querySelector('#pfWecomTagging').value = '{"tag_ids":["tag-1"]}';
document.querySelector('#pfExternalPushEnabled').value = 'true';
document.querySelector('#pfExternalPushReference').value = 'paid-course-21';
[...document.querySelectorAll('button')].find((button) => button.textContent.includes('保存商品'))?.click();
await new Promise((resolve) => setTimeout(resolve, 60));
const update = calls.find((call) => call.path === '/api/v1/products/21' && call.method === 'PUT');
const push = calls.find((call) => call.path.endsWith('/external-push') && call.method === 'PUT');
if (!update || update.body.images[0] !== 'https://cdn.example.test/course.png' || update.body.admin_projection.buy_button_text !== '立即购买' || update.body.admin_projection.wecom_tagging.tag_ids[0] !== 'tag-1' || !update.headers.get('Idempotency-Key') || !push || push.body.configuration_reference !== 'paid-course-21' || !push.headers.get('Idempotency-Key')) throw new Error('普通商品完整编辑未发出真实写入请求');
if (calls.some((call) => /\/test$|checkout|refund|dispatch|send/.test(call.path))) throw new Error('商品编辑意外触发外部效果');
dom.window.close();
console.log('product-edit-e2e: PASS');
