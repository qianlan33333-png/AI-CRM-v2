/**
 * 前端构建输入：每个屏幕的业务能力只可标为 real、backend_blocked 或 presentation_only。
 * backend_blocked 表示当前 OpenAPI 没有可安全调用的契约，页面不得伪造成功。
 */
export type CapabilityState = 'real' | 'backend_blocked' | 'presentation_only';

export type Capability = Readonly<{
  surface: 'admin' | 'h5' | 'sidebar';
  screen: string;
  action: string;
  state: CapabilityState;
  operation?: string;
  reason?: string;
}>;

export const CAPABILITIES: readonly Capability[] = [
  { surface: 'admin', screen: 'customers/customerDetail', action: '客户读取、编辑、阶段、标签、轨迹', state: 'backend_blocked', operation: 'listCustomers/getCustomer/updateCustomer/setCustomerStage', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'questionnaires/questionnaireDetail/questionnaireOps', action: '问卷 CRUD、公开发布、结果、运营读取', state: 'backend_blocked', operation: 'listLegacyQuestionnaires/createLegacyQuestionnaire/updateLegacyQuestionnaire', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'channels/channelForm', action: '渠道读取与保存', state: 'backend_blocked', operation: 'listLegacyChannels/createLegacyChannel/updateLegacyChannel', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '订单、退款、导出', state: 'backend_blocked', operation: 'listLegacyOrders/getLegacyOrder/createLegacyRefundIntent', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品 CRUD、权益', state: 'backend_blocked', operation: 'listProducts/createProduct/updateProduct', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'spProducts/spProductForm/spProductData', action: '周期商品、会员网格', state: 'backend_blocked', operation: 'listServicePeriodProducts/queryServicePeriodMemberGrid', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'coupons/couponForm/couponData', action: '优惠券 CRUD、发布、停用、领取明细', state: 'backend_blocked', operation: 'listLegacyCoupons/createLegacyCoupon/publishLegacyCoupon', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '素材读取、上传、编辑、下载', state: 'backend_blocked', operation: 'listLegacyImages/listLegacyAttachments/listLegacyMiniPrograms', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'tags', action: '企微标签读取与管理', state: 'backend_blocked', operation: 'listLegacyWecomTags/createLegacyWecomTag', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '人群包、成员、发送人、配置', state: 'backend_blocked', operation: 'listAIAudiencePackages/getAIAudiencePackageSenders', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'groupops/groupopsDetail', action: '群运营计划及预览', state: 'backend_blocked', operation: 'listGroupOpsPlans/previewGroupOpsPlanContent', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '内容雷达读取、保存、启停、事件', state: 'backend_blocked', operation: 'listRadarLinks/createRadarLink/updateRadarLink', reason: 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter' },
  { surface: 'admin', screen: 'ai', action: 'AI 计划审批', state: 'backend_blocked', reason: '当前导入壳使用的 ai-assist/review-plans DTO 不在 OpenAPI 中' },
  { surface: 'admin', screen: 'funnel/cycles', action: '原型漏斗和复盘会话', state: 'backend_blocked', reason: '当前 DTO 与 OpenAPI 不匹配' },
  { surface: 'admin', screen: 'config/agents/ownerMig/apidocs', action: '页面壳与导航', state: 'presentation_only' },
  { surface: 'h5', screen: 'auth/all/one/loading/error/result/done', action: '公开问卷读取、OAuth、提交、结果查询', state: 'real', operation: 'getPublicSurveyDefinition/submitPublicSurvey/queryPublicSurveySubmissionResult' },
  { surface: 'h5', screen: 'signup/active/expired/pay/qr', action: '报名或支付跳转', state: 'backend_blocked', reason: '当前公开契约没有该 H5 支付或报名会话' },
  { surface: 'sidebar', screen: 'index', action: 'context-token、workbench、profile、订单、问卷、周期订单、素材、缩略图、备注', state: 'real', operation: 'mintSidebarContext/getSidebarWorkbench/updateSidebarProfile/listSidebarOrders' },
  { surface: 'sidebar', screen: 'index', action: '素材发送', state: 'backend_blocked', reason: 'OpenAPI 只有读取素材，没有发送契约' },
] as const;

export function capabilityFor(surface: Capability['surface'], screen: string): readonly Capability[] {
  return CAPABILITIES.filter((capability) => capability.surface === surface && capability.screen.split('/').includes(screen));
}
