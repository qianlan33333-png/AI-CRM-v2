/**
 * 前端构建输入：每个屏幕的业务能力只可标为 real、backend_blocked 或 presentation_only。
 * backend_blocked 表示当前 OpenAPI 没有可安全调用的契约，页面不得伪造成功。
 */
export type CapabilityState = 'real' | 'backend_blocked' | 'presentation_only' | 'excluded_duplicate_page';

export type Capability = Readonly<{
  surface: 'admin' | 'h5' | 'sidebar';
  screen: string;
  action: string;
  state: CapabilityState;
  operation?: string;
  reason?: string;
}>;

export const CAPABILITIES: readonly Capability[] = [
  { surface: 'admin', screen: 'customers/customerDetail', action: '客户与关联读取', state: 'real', operation: 'listCustomers/getCustomer/getCustomerContext/listCustomerChatActivity/listCustomerSurveyAnswers/getCustomerActivityAnalytics' },
  { surface: 'admin', screen: 'customers/customerDetail', action: '客户编辑、阶段、标签', state: 'backend_blocked', operation: 'updateCustomer/setCustomerStage', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'questionnaires/questionnaireDetail/questionnaireOps', action: '问卷、结果、分析、运营与外推日志读取', state: 'real', operation: 'listLegacyQuestionnaires/getLegacyQuestionnaire/getLegacyQuestionnaireResults/listLegacyQuestionnaireSubmissions/getSurveySafeSubmissionAnalysis/getSurveyOperations/getSurveyOperationsPageData/listSurveyQuestionnaireExternalPushLogs' },
  { surface: 'admin', screen: 'questionnaires/questionnaireDetail/questionnaireOps', action: '问卷 CRUD、发布与运营写入', state: 'backend_blocked', operation: 'createLegacyQuestionnaire/updateLegacyQuestionnaire', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'channels/channelForm', action: '渠道与联系人读取', state: 'real', operation: 'listLegacyChannels/getLegacyChannel/listLegacyChannelEntrants' },
  { surface: 'admin', screen: 'channels/channelForm', action: '渠道保存', state: 'backend_blocked', operation: 'createLegacyChannel/updateLegacyChannel', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '订单、items 与支付来源读取', state: 'real', operation: 'listLegacyOrders/getLegacyOrder/getLegacyOrderItems' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '退款 intent 与外推写入', state: 'backend_blocked', operation: 'createLegacyRefundIntent', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品与权益读取', state: 'real', operation: 'listProducts/getProduct/listProductLocalEntitlements' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品写入', state: 'backend_blocked', operation: 'createProduct/updateProduct', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'spProducts/spProductForm/spProductData', action: '周期商品、成员和成员网格读取', state: 'real', operation: 'listServicePeriodProducts/getServicePeriodProduct/listServicePeriodMembers/getServicePeriodMemberGridAccess/getServicePeriodMemberGridSchema/listServicePeriodMemberViews/getServicePeriodMemberGridShareSettings' },
  { surface: 'admin', screen: 'spProducts/spProductForm/spProductData', action: '周期商品写入与启停', state: 'backend_blocked', operation: 'createServicePeriodProduct/updateServicePeriodProduct', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'coupons/couponForm/couponData', action: '优惠券、分享、领取和商品选项读取', state: 'real', operation: 'listLegacyCoupons/getLegacyCoupon/getLegacyCouponShare/listLegacyCouponClaims/listLegacyCouponProductOptions' },
  { surface: 'admin', screen: 'coupons/couponForm/couponData', action: '优惠券写入、发布与停用', state: 'backend_blocked', operation: 'createLegacyCoupon/publishLegacyCoupon', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '三素材库、facets 与详情读取', state: 'real', operation: 'getLegacyImageList/getLegacyImageFacets/getLegacyImage/listLegacyAttachments/getLegacyAttachment/listLegacyMiniPrograms/getLegacyMiniProgram' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '素材上传、编辑、删除、下载', state: 'backend_blocked', operation: 'uploadLegacyImage/updateLegacyImage/deleteLegacyImage', reason: '读取已接入；写入待批次2 DTO Adapter' },
  { surface: 'admin', screen: 'tags', action: '企微标签组、标签和 live gate 读取', state: 'real', operation: 'listLegacyWecomTags/listLegacyWecomTagGroups/getLegacyWecomTag/getLegacyWecomTagGroup/getLegacyWecomTagExecutionGate' },
  { surface: 'admin', screen: 'tags', action: '企微标签组/标签写入与同步队列', state: 'real', operation: 'createLegacyWecomTagGroup/updateLegacyWecomTagGroupPatch/createLegacyWecomTag/updateLegacyWecomTagPatch/archiveLegacyWecomTag/queueLegacyWecomTagSync' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '人群包与分组读取、分组增改删、启停、复制、归档', state: 'real', operation: 'listAIAudiencePackageGroups/listAIAudiencePackages/createAIAudiencePackageGroup/updateAIAudiencePackageGroup/deleteAIAudiencePackageGroup/activateAIAudiencePackage/pauseAIAudiencePackage/copyAIAudiencePackage/archiveAIAudiencePackage' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '人群条件、发送人、配置物化与群发确认', state: 'backend_blocked', operation: 'updateAIAudiencePackage/replaceAIAudiencePackageSenders/putAIAudienceConfigurationVersion/materializeAIAudienceConfiguration', reason: '当前 Kimi 表单未持有 SegmentDefinition、sender/version 与物化回执 DTO，不能安全构造请求' },
  { surface: 'admin', screen: 'groupops/groupopsDetail', action: '群运营计划及预览', state: 'backend_blocked', operation: 'listGroupOpsPlans/previewGroupOpsPlanContent', reason: '当前 Kimi Group Ops 壳未持有节点、群组、成员与 CAS version DTO' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '内容雷达列表、事件与启停', state: 'real', operation: 'listRadarLinks/getRadarLink/listRadarLinkEvents/enableRadarLink/disableRadarLink' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '雷达新建、编辑与素材绑定', state: 'backend_blocked', operation: 'createRadarLink/updateRadarLink', reason: 'Kimi 图片/PDF 表单只保留媒体展示名，未持有 cover_image_id 或 attachment_id 的版本化 DTO' },
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
