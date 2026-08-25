import { attachmentPageDto, audiencePackagePageDto, channelPageDto, configCategoryPageDto, couponPageDto, createOwnerReassignmentPreviewDto, customerPageDto, executeOwnerReassignmentPreviewDto, imagePageDto, miniProgramPageDto, orderPageDto, ownerReassignmentPreviewDto, productPageDto, questionnairePageDto, radarPageDto, readAdminRows, saveCouponDto, saveImageItemDto, saveProductDto, saveRadarLinkDto, saveServiceProductDto, serviceProductPageDto, setCustomerTagDto, tagPageDto, updateCustomerDto, uploadRadarPdfDto } from './admin';
import type { LegacyQuestionnaire } from './generated/health';
import { getAddCustomerTagUrl, getCreateContactOwnerReassignmentPreviewUrl, getCreateLegacyWecomTagUrl, getCreateRadarLinkUrl, getDownloadContactOwnerReassignmentResultsUrl, getDownloadContactOwnerReassignmentTemplateUrl, getExecuteContactOwnerReassignmentPreviewUrl, getGetAdminOpsCategoryUrl, getGetContactOwnerReassignmentPreviewUrl, getGetLegacyAttachmentUrl, getGetLegacyCouponUrl, getGetLegacyImageUrl, getGetLegacyOrderUrl, getGetLegacyQuestionnaireUrl, getGetLegacyWecomTagUrl, getGetProductUrl, getGetRadarLinkShareProjectionUrl, getGetServicePeriodProductUrl, getListAdminOpsCategoriesUrl, getListAIAudiencePackagesUrl, getListCustomersUrl, getListLegacyChannelsUrl, getListLegacyCouponsUrl, getListLegacyQuestionnairesUrl, getListProductsUrl, getListRadarLinksUrl, getListServicePeriodProductsUrl, getQueueLegacyWecomTagSyncUrl, getSetCustomerStageUrl, getUpdateCustomerUrl, getUpdateLegacyImageUrl, getUploadLegacyAttachmentUrl } from './generated/health';
import { ApiError } from './transport';
import { HttpApi } from '../shared/api/client';
import { getCreateProductUrl, getCreateServicePeriodProductUrl, getUpdateServicePeriodProductUrl } from './generated/health';
import { getArchiveLegacyCouponUrl, getCopyLegacyCouponUrl, getCreateLegacyCouponUrl, getDeleteLegacyCouponUrl, getPublishLegacyCouponUrl, getStopLegacyCouponUrl, getUpdateLegacyCouponUrl } from './generated/health';

function assert(ok: unknown, message: string): asserts ok { if (!ok) throw new Error(message); }
const response = (data: unknown, status = 200) => ({ status, data, headers: new Headers() });

export async function runAdminAdapterTests(): Promise<void> {
  // URL factories are generated from api/openapi.yaml; generated callers use GET for every read below.
  assert(getListCustomersUrl({ limit: 50 }) === '/api/v1/customers?limit=50', 'customer list URL/method');
  assert(getGetLegacyQuestionnaireUrl(4) === '/api/admin/questionnaires/4', 'questionnaire detail URL/method');
  assert(getListLegacyChannelsUrl({ limit: 50, include_archived: true }) === '/api/admin/channels?limit=50&include_archived=true', 'channel list URL/method');
  assert(getGetLegacyOrderUrl('WX-9') === '/api/admin/orders/WX-9', 'order detail URL/method');
  assert(getListProductsUrl() === '/api/v1/products', 'product list URL/method');
  assert(getGetProductUrl(7) === '/api/v1/products/7', 'product detail URL/method');
  assert(getCreateProductUrl() === '/api/v1/products', 'product create URL');
  assert(getListServicePeriodProductsUrl() === '/api/admin/service-period-products', 'service product list URL/method');
  assert(getGetServicePeriodProductUrl(8) === '/api/admin/service-period-products/8', 'service product detail URL/method');
  assert(getCreateServicePeriodProductUrl() === '/api/admin/service-period-products', 'service product create URL');
  assert(getUpdateServicePeriodProductUrl(8) === '/api/admin/service-period-products/8', 'service product update URL');
  assert(getListLegacyCouponsUrl() === '/api/admin/coupons', 'coupon list URL/method');
  assert(getGetLegacyCouponUrl(3) === '/api/admin/coupons/3', 'coupon detail URL/method');
  assert(getCreateLegacyCouponUrl() === '/api/admin/coupons', 'coupon create URL');
  assert(getUpdateLegacyCouponUrl(3) === '/api/admin/coupons/3', 'coupon update URL');
  assert(getPublishLegacyCouponUrl(3).endsWith('/3/publish') && getStopLegacyCouponUrl(3).endsWith('/3/stop'), 'coupon lifecycle URLs');
  assert(getCopyLegacyCouponUrl(3).endsWith('/3/copy') && getArchiveLegacyCouponUrl(3).endsWith('/3/archive') && getDeleteLegacyCouponUrl(3) === '/api/admin/coupons/3', 'coupon copy/archive/delete URLs');
  assert(getGetLegacyImageUrl('img-1') === '/api/admin/image-library/img-1', 'image detail URL/method');
  assert(getGetLegacyAttachmentUrl('att-1') === '/api/admin/attachment-library/att-1', 'attachment detail URL/method');
  assert(getGetLegacyWecomTagUrl(5) === '/api/admin/wecom/tags/5', 'tag detail URL/method');
  assert(getListRadarLinksUrl() === '/api/admin/radar-links', 'radar list URL/method');
  assert(getListAIAudiencePackagesUrl() === '/api/admin/ai-audience/packages', 'audience list URL/method');
  assert(getCreateLegacyWecomTagUrl() === '/api/admin/wecom/tags', 'tag create URL/method');
  assert(getQueueLegacyWecomTagSyncUrl() === '/api/admin/wecom/tags/sync', 'tag sync URL/method');
  assert(getCreateRadarLinkUrl() === '/api/admin/radar-links', 'radar create URL');
  assert(getGetRadarLinkShareProjectionUrl(5) === '/api/admin/radar-links/5/share', 'radar share URL');
  assert(getUpdateLegacyImageUrl('15') === '/api/admin/image-library/15', 'image update URL');
  assert(getUploadLegacyAttachmentUrl() === '/api/admin/attachment-library/upload', 'attachment upload URL');
  assert(getUpdateCustomerUrl(7) === '/api/v1/customers/7', 'customer update URL');
  assert(getSetCustomerStageUrl(7) === '/api/v1/customers/7/stage', 'customer stage URL');
  assert(getAddCustomerTagUrl(7, 9) === '/api/v1/customers/7/tags/9', 'customer tag URL');
  assert(getListAdminOpsCategoriesUrl() === '/api/admin/config/categories', 'config categories URL');
  assert(getGetAdminOpsCategoryUrl('wechat_pay') === '/api/admin/config/categories/wechat_pay', 'config category detail URL');
  assert(getDownloadContactOwnerReassignmentTemplateUrl() === '/api/v1/contact-owner-reassignments/template', 'owner reassignment template URL');
  assert(getCreateContactOwnerReassignmentPreviewUrl() === '/api/v1/contact-owner-reassignments/previews', 'owner reassignment preview URL');
  assert(getGetContactOwnerReassignmentPreviewUrl('cor_0123456789012345678901') === '/api/v1/contact-owner-reassignments/previews/cor_0123456789012345678901', 'owner reassignment read URL');
  assert(getExecuteContactOwnerReassignmentPreviewUrl('cor_0123456789012345678901').endsWith('/execute'), 'owner reassignment execute URL');
  assert(getDownloadContactOwnerReassignmentResultsUrl('cor_0123456789012345678901').endsWith('/results.csv'), 'owner reassignment result URL');

  const customer = customerPageDto({ id: 7, name: '陈晨', is_deleted: false, extra: {}, created_at: '2026-08-25T00:00:00Z', updated_at: '2026-08-25T00:00:00Z', owner_staff_id: 3 });
  assert(customer.id === '7' && customer.owner === '3' && customer.mobile === '—', 'customer response mapping');
  const questionnaire = questionnairePageDto({ id: 4, name: '诊断', title: '增长诊断', description: '', answer_display_mode: 'all_in_one', slug: 'growth', assessment_enabled: true, is_disabled: false, status: 'active', version: 2, question_count: 1, submission_count: 9, created_at: '2026-08-25T00:00:00Z', updated_at: '2026-08-25T00:00:00Z', public_path: '/q/growth', submitted_path: '/q/growth/submitted', questions: [] } as unknown as LegacyQuestionnaire);
  assert(questionnaire.name === '增长诊断' && questionnaire.count === '9', 'questionnaire response mapping');
  assert(channelPageDto({ id: 1, channel_name: '夏令营', channel_code: 'summer', status: 'active', assignee_count: 0, channel_contact_count: 0, created_at: '', updated_at: '' }).tone === 'ok', 'channel response mapping');
  assert(orderPageDto({ merchant_order_no: 'WX-9', provider: 'wechat_pay', product_name: '营', status: 'paid' }).plat === 'wechat_pay', 'order provider response mapping');
  assert(productPageDto({ id: 1, name: '商品', status: 'active' }).tone === 'ok', 'product response mapping');
  assert(serviceProductPageDto({ id: 2, name: '周期', status: 'disabled' }).tone === 'gray', 'service product response mapping');
  assert(couponPageDto({ name: '券', code: 'C', status: 'draft' }).tone === 'warn', 'coupon response mapping');
  assert(imagePageDto({ id: 11, file_name: 'a.png', enabled: false }).resourceId === '11', 'image response mapping keeps resource id');
  assert(attachmentPageDto({ id: 12, file_name: 'a.pdf', mime_type: 'application/pdf' }).resourceId === '12', 'attachment response mapping keeps resource id');
  assert(miniProgramPageDto({ id: 13, name: '小程序', thumbnail_status: 'ready' }).resourceId === 13, 'mini-program response mapping keeps resource id');
  assert(tagPageDto({ id: 1, group_id: 2, name: '新客', user_count: 6 }).users === 6, 'tag response mapping');
  assert(radarPageDto({ link_id: 5, public_code: 'rd_1234567890123456789012', name: '雷达', title: '内容', destination_url: 'https://example.test/r', cover_image_id: null, attachment_id: null, status: 'enabled', version: 2, created_by: 9, updated_by: 9, created_at: '', updated_at: '' }).enabled, 'radar response mapping');
  assert(audiencePackagePageDto({ package_id: 3, name: '沉默用户', group_id: null, lifecycle: 'active', version: 4, refresh_mode: 'manual', member_count: 12, refreshed_at: null }).count === 12, 'audience response mapping');
  assert(configCategoryPageDto({ key: 'wechat_pay', enabled: true }).on, 'config category safe response mapping');
  const ownerPreviewApi = { id: 'cor_0123456789012345678901', hash: 'a'.repeat(64), rows: [{ customer_id: 7, expected_owner_staff_id: 3, expected_updated_at: '2026-08-25T00:00:00Z', target_owner_staff_id: 9 }], issues: [{ line: 3, code: 'invalid_row' as const }], expires_at: '2026-08-25T01:00:00Z', executed: false, result: [] };
  const ownerPreview = ownerReassignmentPreviewDto(ownerPreviewApi);
  assert(ownerPreview.rows[0].customerId === 7 && ownerPreview.rows[0].targetOwnerStaffId === 9 && ownerPreview.issues[0].line === 3, 'owner reassignment response mapping');

  const savedFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response(JSON.stringify({ code: 'bad' }), { status: 503 });
  try { await readAdminRows(); assert(false, 'failed production read must not return seed'); } catch { /* expected: no SEED_DB fallback */ }
  finally { globalThis.fetch = savedFetch; }

  let productWrite: RequestInit | undefined;
  globalThis.fetch = async (_input, init) => { productWrite = init; return new Response(JSON.stringify({ id: 21, product_code: 'P-21', name: '课程', description: '说明', price_minor: 19900, currency: 'CNY', stock_quantity: 9, images: [], created_by: 1, created_at: '', updated_at: '', version: 1 }), { status: 201 }); };
  try {
    const productSaved = await saveProductDto({ code: 'P-21', name: '课程', description: '说明', price: '199.00', currency: 'CNY', stockQuantity: 9 });
    assert(productSaved.resourceId === 21 && productSaved.price === '199.00' && productWrite?.method === 'POST', 'product create method/response mapping');
    assert(JSON.parse(String(productWrite.body)).price_minor === 19900, 'product price request mapping');
  } finally { globalThis.fetch = savedFetch; }

  const couponCalls: Array<{ input: string; init?: RequestInit }> = [];
  globalThis.fetch = async (input, init) => {
    couponCalls.push({ input: String(input), init });
    const coupon = { id: 31, name: '新客券', discount_amount_total: 10000, total_issue_limit: 1200, issued_count: 0, per_user_issue_limit: 1, claim_starts_at: '2026-08-01T00:00:00Z', claim_ends_at: '2026-08-31T00:00:00Z', validity_mode: 'relative_days', relative_validity_days: 7, instructions: '说明', target_refs: ['SP-GROW-90'], status: init?.method === 'POST' && String(input).endsWith('/publish') ? 'published' : 'draft', version: 1 };
    return new Response(JSON.stringify({ coupon }), { status: 201 });
  };
  try {
    const coupon = await saveCouponDto({ name: '新客券', discount: '100.00', totalIssueLimit: 1200, perUserIssueLimit: 1, claimStartsAt: '2026-08-01T08:00', claimEndsAt: '2026-08-31T08:00', validityMode: 'relative_days', relativeValidityDays: 7, instructions: '说明', targetRefs: ['SP-GROW-90'] }, true);
    assert(coupon.resourceId === 31 && coupon.status === 'published' && coupon.scope === 'SP-GROW-90', 'coupon response mapping');
    assert(couponCalls[0].input === '/api/admin/coupons' && couponCalls[0].init?.method === 'POST', 'coupon create URL/method');
    const couponBody = JSON.parse(String(couponCalls[0].init?.body));
    assert(couponBody.discount_amount_total === 10000 && couponBody.target_refs[0] === 'SP-GROW-90' && couponBody.relative_validity_days === 7, 'coupon request DTO mapping');
    assert(couponCalls[1].input.endsWith('/31/publish') && couponCalls[1].init?.method === 'POST', 'coupon publish URL/method');
  } finally { globalThis.fetch = savedFetch; }

  const serviceCalls: Array<{ input: string; init?: RequestInit }> = [];
  globalThis.fetch = async (input, init) => {
    serviceCalls.push({ input: String(input), init });
    const product = { service_product_id: 8, product_code: 'SP-8', name: '季度', description: '说明', price_minor: 398000, currency: 'CNY', stock_quantity: 5, lifecycle: 'draft', enabled: false, archived: false, version: 3, created_at: '', updated_at: '' };
    return new Response(JSON.stringify(init?.method === 'GET' ? { ok: true, product } : { ok: true, product: { ...product, name: '季度新版', version: 4 } }), { status: 200 });
  };
  try {
    const serviceSaved = await saveServiceProductDto({ id: 8, code: 'SP-8', name: '季度新版', description: '说明', price: '3980.00', currency: 'CNY', stockQuantity: 5 });
    assert(serviceSaved.name === '季度新版' && serviceSaved.version === 4, 'service product update response mapping');
    assert(serviceCalls[0].init?.method === 'GET' && serviceCalls[1].init?.method === 'PUT', 'service product CAS read/update methods');
    assert(JSON.parse(String(serviceCalls[1].init?.body)).expected_version === 3, 'service product CAS version mapping');
  } finally { globalThis.fetch = savedFetch; }

  let ownerCreate: { input: string; init?: RequestInit } | undefined;
  globalThis.fetch = async (input, init) => { ownerCreate = { input: String(input), init }; return new Response(JSON.stringify(ownerPreviewApi), { status: 201 }); };
  try {
    const preview = await createOwnerReassignmentPreviewDto('customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n7,3,2026-08-25T00:00:00Z,9\n');
    assert(preview.id === ownerPreviewApi.id && ownerCreate?.input === '/api/v1/contact-owner-reassignments/previews', 'owner reassignment preview mapping/URL');
    assert(ownerCreate.init?.method === 'POST' && new Headers(ownerCreate.init.headers).get('Content-Type') === 'text/csv', 'owner reassignment preview method/content type');
    assert(String(ownerCreate.init?.body).startsWith('customer_id,'), 'owner reassignment CSV body must not be JSON quoted');
  } finally { globalThis.fetch = savedFetch; }

  let ownerExecute: RequestInit | undefined;
  globalThis.fetch = async (_input, init) => { ownerExecute = init; return new Response(JSON.stringify({ ...ownerPreviewApi, executed: true, result: [{ customer_id: 7, previous_owner_staff_id: 3, target_owner_staff_id: 9, updated_at: '2026-08-25T00:01:00Z' }] }), { status: 200 }); };
  try {
    const executed = await executeOwnerReassignmentPreviewDto(ownerPreview);
    const body = JSON.parse(String(ownerExecute?.body));
    assert(executed.executed && executed.result.length === 1 && ownerExecute?.method === 'POST', 'owner reassignment execute method/mapping');
    assert(body.preview_hash === 'a'.repeat(64) && body.confirmation === 'CONFIRM OWNER REASSIGNMENT', 'owner reassignment confirmation DTO');
    assert(Boolean(new Headers(ownerExecute?.headers).get('Idempotency-Key')), 'owner reassignment idempotency header');
  } finally { globalThis.fetch = savedFetch; }

  const calls: Array<{ input: string; init?: RequestInit }> = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(JSON.stringify({ link: { link_id: 5, public_code: 'rd_1234567890123456789012', name: '官网', title: '官网', destination_url: 'https://example.test', cover_image_id: null, attachment_id: null, status: 'disabled', version: 1, created_by: 9, updated_by: 9, created_at: '', updated_at: '' }, local_projection: true, real_external_call_executed: false }), { status: 201 });
  };
  try {
    const radar = await saveRadarLinkDto({ title: '官网', target_type: 'link', original_url: 'https://example.test', file_name_snapshot: '', media_item_id: '', enabled: false, auth_required: true });
    assert(radar.id === 5 && calls[0].input === '/api/admin/radar-links' && calls[0].init?.method === 'POST', 'radar create generated URL/method/mapping');
    assert(JSON.parse(String(calls[0].init?.body)).destination_url === 'https://example.test', 'radar create request mapping');
  } finally { globalThis.fetch = savedFetch; }

  let customerInit: RequestInit | undefined;
  globalThis.fetch = async (_input, init) => { customerInit = init; return new Response(JSON.stringify({ id: 7, name: '新姓名', owner_staff_id: 3, stage_id: 2, is_deleted: false, extra: {}, created_at: '', updated_at: '' }), { status: 200 }); };
  try {
    const customerResult = await updateCustomerDto(7, { name: '新姓名' });
    assert(customerResult.name === '新姓名' && customerInit?.method === 'PATCH', 'customer update method/mapping');
    assert(JSON.parse(String(customerInit.body)).name === '新姓名', 'customer update request DTO');
  } finally { globalThis.fetch = savedFetch; }

  let tagMethod = '';
  globalThis.fetch = async (_input, init) => { tagMethod = init?.method || ''; return new Response(null, { status: 204 }); };
  try { await setCustomerTagDto(7, 9, true); assert(tagMethod === 'PUT', 'customer tag add method'); }
  finally { globalThis.fetch = savedFetch; }

  globalThis.fetch = async () => new Response(JSON.stringify({ code: 'validation', message: 'bad', request_id: 'r', details: [] }), { status: 422 });
  try { await updateCustomerDto(7, { name: '' }); assert(false, 'customer 422 was accepted'); }
  catch (error) { assert(error instanceof ApiError && error.status === 422, 'customer 422 must stay structured'); }
  finally { globalThis.fetch = savedFetch; }

  let called = false;
  globalThis.fetch = async () => { called = true; return new Response('{}', { status: 200 }); };
  try {
    await new HttpApi({ baseUrl: '' }).approveAiPlan(1);
    assert(false, 'non-equivalent AI DTO was accepted');
  } catch (error) {
    assert(error instanceof Error && error.message.includes('后端能力未就绪') && !called, 'blocked AI action must not send request');
  } finally { globalThis.fetch = savedFetch; }

  globalThis.fetch = async () => new Response(JSON.stringify({ code: 'conflict' }), { status: 409 });
  try { await saveRadarLinkDto({ title: '冲突', target_type: 'link', original_url: 'https://example.test', file_name_snapshot: '', media_item_id: '', enabled: false, auth_required: true }); assert(false, '409 was accepted'); }
  catch (error) { assert(error instanceof ApiError && error.status === 409, 'radar 409 must stay structured'); }
  finally { globalThis.fetch = savedFetch; }

  let imageRequest: { input: string; init?: RequestInit } | undefined;
  globalThis.fetch = async (input, init) => { imageRequest = { input: String(input), init }; return new Response(JSON.stringify({ item: { id: 15 } }), { status: 200 }); };
  try {
    await saveImageItemDto('旧名', { resourceId: '15', name: '新名', desc: '说明', tags: '一, 二', enabled: true });
    assert(imageRequest?.input === '/api/admin/image-library/15' && imageRequest.init?.method === 'PUT', 'image update generated URL/method');
    assert(JSON.parse(String(imageRequest.init?.body)).tags.length === 2, 'image metadata mapping');
  } finally { globalThis.fetch = savedFetch; }

  let uploadInit: RequestInit | undefined;
  globalThis.fetch = async (_input, init) => { uploadInit = init; return new Response(JSON.stringify({ id: 16, name: '资料', file_name: '资料.pdf', file_size: 3, mime_type: 'application/pdf' }), { status: 200 }); };
  try {
    const media = await uploadRadarPdfDto(new File(['pdf'], '资料.pdf', { type: 'application/pdf' }));
    assert(media.id === 16 && uploadInit?.method === 'POST' && uploadInit.body instanceof FormData, 'attachment upload multipart mapping');
    assert((uploadInit.body as FormData).get('attachment') instanceof Blob, 'attachment multipart field');
  } finally { globalThis.fetch = savedFetch; }
  void response;
}
