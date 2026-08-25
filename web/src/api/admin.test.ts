import { attachmentPageDto, audiencePackagePageDto, channelPageDto, couponPageDto, customerPageDto, imagePageDto, miniProgramPageDto, orderPageDto, productPageDto, questionnairePageDto, radarPageDto, readAdminRows, saveImageItemDto, saveRadarLinkDto, serviceProductPageDto, tagPageDto, uploadRadarPdfDto } from './admin';
import type { LegacyQuestionnaire } from './generated/health';
import { getCreateLegacyWecomTagUrl, getCreateRadarLinkUrl, getGetLegacyAttachmentUrl, getGetLegacyCouponUrl, getGetLegacyImageUrl, getGetLegacyOrderUrl, getGetLegacyQuestionnaireUrl, getGetLegacyWecomTagUrl, getGetProductUrl, getGetRadarLinkShareProjectionUrl, getGetServicePeriodProductUrl, getListAIAudiencePackagesUrl, getListCustomersUrl, getListLegacyChannelsUrl, getListLegacyCouponsUrl, getListLegacyQuestionnairesUrl, getListProductsUrl, getListRadarLinksUrl, getListServicePeriodProductsUrl, getQueueLegacyWecomTagSyncUrl, getUpdateLegacyImageUrl, getUploadLegacyAttachmentUrl } from './generated/health';
import { ApiError } from './transport';
import { HttpApi } from '../shared/api/client';

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
  assert(getListServicePeriodProductsUrl() === '/api/admin/service-period-products', 'service product list URL/method');
  assert(getGetServicePeriodProductUrl(8) === '/api/admin/service-period-products/8', 'service product detail URL/method');
  assert(getListLegacyCouponsUrl() === '/api/admin/coupons', 'coupon list URL/method');
  assert(getGetLegacyCouponUrl(3) === '/api/admin/coupons/3', 'coupon detail URL/method');
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

  const savedFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response(JSON.stringify({ code: 'bad' }), { status: 503 });
  try { await readAdminRows(); assert(false, 'failed production read must not return seed'); } catch { /* expected: no SEED_DB fallback */ }
  finally { globalThis.fetch = savedFetch; }

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
