import { attachmentPageDto, audiencePackagePageDto, channelPageDto, couponPageDto, customerPageDto, imagePageDto, miniProgramPageDto, orderPageDto, productPageDto, questionnairePageDto, radarPageDto, readAdminRows, serviceProductPageDto, tagPageDto } from './admin';
import type { LegacyQuestionnaire } from './generated/health';
import { getGetLegacyAttachmentUrl, getGetLegacyCouponUrl, getGetLegacyImageUrl, getGetLegacyOrderUrl, getGetLegacyQuestionnaireUrl, getGetLegacyWecomTagUrl, getGetProductUrl, getGetServicePeriodProductUrl, getListAIAudiencePackagesUrl, getListCustomersUrl, getListLegacyChannelsUrl, getListLegacyCouponsUrl, getListLegacyQuestionnairesUrl, getListProductsUrl, getListRadarLinksUrl, getListServicePeriodProductsUrl } from './generated/health';

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

  const customer = customerPageDto({ id: 7, name: '陈晨', is_deleted: false, extra: {}, created_at: '2026-08-25T00:00:00Z', updated_at: '2026-08-25T00:00:00Z', owner_staff_id: 3 });
  assert(customer.id === '7' && customer.owner === '3' && customer.mobile === '—', 'customer response mapping');
  const questionnaire = questionnairePageDto({ id: 4, name: '诊断', title: '增长诊断', description: '', answer_display_mode: 'all_in_one', slug: 'growth', assessment_enabled: true, is_disabled: false, status: 'active', version: 2, question_count: 1, submission_count: 9, created_at: '2026-08-25T00:00:00Z', updated_at: '2026-08-25T00:00:00Z', public_path: '/q/growth', submitted_path: '/q/growth/submitted', questions: [] } as unknown as LegacyQuestionnaire);
  assert(questionnaire.name === '增长诊断' && questionnaire.count === '9', 'questionnaire response mapping');
  assert(channelPageDto({ id: 1, channel_name: '夏令营', channel_code: 'summer', status: 'active', assignee_count: 0, channel_contact_count: 0, created_at: '', updated_at: '' }).tone === 'ok', 'channel response mapping');
  assert(orderPageDto({ merchant_order_no: 'WX-9', provider: 'wechat_pay', product_name: '营', status: 'paid' }).plat === 'wechat_pay', 'order provider response mapping');
  assert(productPageDto({ id: 1, name: '商品', status: 'active' }).tone === 'ok', 'product response mapping');
  assert(serviceProductPageDto({ id: 2, name: '周期', status: 'disabled' }).tone === 'gray', 'service product response mapping');
  assert(couponPageDto({ name: '券', code: 'C', status: 'draft' }).tone === 'warn', 'coupon response mapping');
  assert(imagePageDto({ filename: 'a.png', enabled: false }).enabled === false, 'image response mapping');
  assert(attachmentPageDto({ filename: 'a.pdf', content_type: 'application/pdf' }).type === 'application/pdf', 'attachment response mapping');
  assert(miniProgramPageDto({ name: '小程序', thumbnail_status: 'ready' }).thumbOk, 'mini-program response mapping');
  assert(tagPageDto({ id: 1, group_id: 2, name: '新客', user_count: 6 }).users === 6, 'tag response mapping');
  assert(radarPageDto({ link_id: 5, public_code: 'rd_1234567890123456789012', name: '雷达', title: '内容', destination_url: 'https://example.test/r', cover_image_id: null, attachment_id: null, status: 'enabled', version: 2, created_by: 9, updated_by: 9, created_at: '', updated_at: '' }).enabled, 'radar response mapping');
  assert(audiencePackagePageDto({ package_id: 3, name: '沉默用户', group_id: null, lifecycle: 'active', version: 4, refresh_mode: 'manual', member_count: 12, refreshed_at: null }).count === 12, 'audience response mapping');

  const savedFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response(JSON.stringify({ code: 'bad' }), { status: 503 });
  try { await readAdminRows(); assert(false, 'failed production read must not return seed'); } catch { /* expected: no SEED_DB fallback */ }
  finally { globalThis.fetch = savedFetch; }
  void response;
}
