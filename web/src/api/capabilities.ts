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
  { surface: 'admin', screen: 'customerDetail', action: '客户姓名、阶段与标签写入', state: 'real', operation: 'updateCustomer/setCustomerStage/addCustomerTag/removeCustomerTag' },
  { surface: 'admin', screen: 'questionnaires/questionnaireDetail/questionnaireOps', action: '问卷、结果、分析、运营与外推日志读取', state: 'real', operation: 'listLegacyQuestionnaires/getLegacyQuestionnaire/getLegacyQuestionnaireResults/listLegacyQuestionnaireSubmissions/getSurveySafeSubmissionAnalysis/getSurveyOperations/getSurveyOperationsPageData/listSurveyQuestionnaireExternalPushLogs' },
  { surface: 'admin', screen: 'questionnaires/questionnaireDetail', action: '问卷 CRUD 与发布', state: 'backend_blocked', operation: 'createLegacyQuestionnaire/updateLegacyQuestionnaire', reason: '当前问卷编辑壳未绑定完整 questions/options/assessment DTO，不能提交静态演示字段' },
  { surface: 'admin', screen: 'questionnaireOps', action: '问卷运营写入', state: 'backend_blocked', operation: 'saveSurveyCompletionOperations/saveSurveyExternalPushOperations', reason: '当前运营表单使用 URL，OpenAPI 只接受 opaque navigation_target_id/configuration_reference，语义不等价' },
  { surface: 'admin', screen: 'channels/channelForm', action: '渠道与联系人读取', state: 'real', operation: 'listLegacyChannels/getLegacyChannel/listLegacyChannelEntrants' },
  { surface: 'admin', screen: 'channels/channelForm', action: '渠道保存', state: 'backend_blocked', operation: 'createLegacyChannel/updateLegacyChannel', reason: '当前渠道壳的载体、客服、素材与标签仍是静态展示，未形成 OpenAPI 要求的完整配置 DTO' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '订单、items 与支付来源读取', state: 'real', operation: 'listLegacyOrders/getLegacyOrder/getLegacyOrderItems' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '退款 intent 与外推写入', state: 'backend_blocked', operation: 'createLegacyRefundIntent', reason: '当前退款表单未区分 provider；OpenAPI 只接受 WeChat Shop canonical refund reservation，不能对其他支付来源误调用' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品与权益读取', state: 'real', operation: 'listProducts/getProduct/listProductLocalEntitlements' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品写入与生命周期', state: 'backend_blocked', operation: 'createProduct/updateProduct/enableLegacyWechatPayProduct/disableLegacyWechatPayProduct', reason: 'JSON 商品读取未返回 lifecycle，当前表单仍是静态值，无法安全决定 CAS 启停或提交真实字段' },
  { surface: 'admin', screen: 'spProducts/spProductForm/spProductData', action: '周期商品、成员和成员网格读取', state: 'real', operation: 'listServicePeriodProducts/getServicePeriodProduct/listServicePeriodMembers/getServicePeriodMemberGridAccess/getServicePeriodMemberGridSchema/listServicePeriodMemberViews/getServicePeriodMemberGridShareSettings' },
  { surface: 'admin', screen: 'spProducts/spProductForm/spProductData', action: '周期商品写入与启停', state: 'backend_blocked', operation: 'createServicePeriodProduct/updateServicePeriodProduct', reason: '当前周期商品表单仍是静态值，未绑定 product_code/price_minor/stock/version DTO' },
  { surface: 'admin', screen: 'coupons/couponForm/couponData', action: '优惠券、分享、领取和商品选项读取', state: 'real', operation: 'listLegacyCoupons/getLegacyCoupon/getLegacyCouponShare/listLegacyCouponClaims/listLegacyCouponProductOptions' },
  { surface: 'admin', screen: 'coupons/couponForm/couponData', action: '优惠券写入、发布与停用', state: 'backend_blocked', operation: 'createLegacyCoupon/publishLegacyCoupon', reason: '当前优惠券表单的商品引用与有效期控件仍为静态展示，未形成契约要求的完整 DTO' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '三素材库、facets 与详情读取', state: 'real', operation: 'getLegacyImageList/getLegacyImageFacets/getLegacyImage/listLegacyAttachments/getLegacyAttachment/listLegacyMiniPrograms/getLegacyMiniProgram' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '素材上传、编辑与删除', state: 'real', operation: 'uploadLegacyImage/updateLegacyImage/deleteLegacyImage/uploadLegacyAttachment/updateLegacyAttachment/deleteLegacyAttachment/createLegacyMiniProgram/updateLegacyMiniProgram/deleteLegacyMiniProgram' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '附件二进制下载', state: 'real', operation: 'downloadLegacyAttachment' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '图片缩略图读取', state: 'backend_blocked', operation: 'getLegacyImageVariant', reason: 'OpenAPI operation 已存在，但当前图片卡片尚未绑定 blob variant 生命周期' },
  { surface: 'admin', screen: 'tags', action: '企微标签组、标签和 live gate 读取', state: 'real', operation: 'listLegacyWecomTags/listLegacyWecomTagGroups/getLegacyWecomTag/getLegacyWecomTagGroup/getLegacyWecomTagExecutionGate' },
  { surface: 'admin', screen: 'tags', action: '企微标签组/标签写入与同步队列', state: 'real', operation: 'createLegacyWecomTagGroup/updateLegacyWecomTagGroupPatch/createLegacyWecomTag/updateLegacyWecomTagPatch/archiveLegacyWecomTag/queueLegacyWecomTagSync' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '人群包与分组读取、分组增改删、启停、复制、归档', state: 'real', operation: 'listAIAudiencePackageGroups/listAIAudiencePackages/createAIAudiencePackageGroup/updateAIAudiencePackageGroup/deleteAIAudiencePackageGroup/activateAIAudiencePackage/pauseAIAudiencePackage/copyAIAudiencePackage/archiveAIAudiencePackage' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '人群条件、发送人、配置物化与群发确认', state: 'backend_blocked', operation: 'updateAIAudiencePackage/replaceAIAudiencePackageSenders/putAIAudienceConfigurationVersion/materializeAIAudienceConfiguration', reason: '当前 Kimi 表单未持有 SegmentDefinition、sender/version 与物化回执 DTO，不能安全构造请求' },
  { surface: 'admin', screen: 'groupops/groupopsDetail', action: '群运营计划及预览', state: 'backend_blocked', operation: 'listGroupOpsPlans/previewGroupOpsPlanContent', reason: '当前 Kimi Group Ops 壳未持有节点、群组、成员与 CAS version DTO' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '内容雷达列表、事件、分享与启停', state: 'real', operation: 'listRadarLinks/getRadarLink/listRadarLinkEvents/getRadarLinkShareProjection/enableRadarLink/disableRadarLink' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '链接型雷达新建与编辑', state: 'real', operation: 'createRadarLink/updateRadarLink' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '图片/PDF 雷达素材上传', state: 'real', operation: 'uploadLegacyImage/uploadLegacyAttachment' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '图片/PDF 雷达保存', state: 'backend_blocked', operation: 'createRadarLink/updateRadarLink', reason: 'Radar 契约除素材 ID 外仍强制要求独立 HTTPS destination_url，当前图片/PDF 表单没有该等价字段' },
  { surface: 'admin', screen: 'ai', action: 'AI 计划审批', state: 'backend_blocked', reason: '当前导入壳使用的 ai-assist/review-plans DTO 不在 OpenAPI 中' },
  { surface: 'admin', screen: 'funnel/cycles', action: '原型漏斗和复盘会话', state: 'backend_blocked', reason: '当前 DTO 与 OpenAPI 不匹配' },
  { surface: 'admin', screen: 'config/configDetail', action: '配置类目安全投影读取', state: 'real', operation: 'listAdminOpsCategories/getAdminOpsCategory' },
  { surface: 'admin', screen: 'configDetail', action: '配置类目启停、保存与检查', state: 'backend_blocked', operation: 'setAdminOpsCategoryEnabled/setAdminOpsCategorySettings/checkAdminOpsCategory', reason: '这些写入要求 route-bound Admin Action Token，但当前 JSON category DTO 不返回 token' },
  { surface: 'admin', screen: 'ownerMig', action: '本地负责人迁移模板、持久预览、确认执行与脱敏报告', state: 'real', operation: 'downloadContactOwnerReassignmentTemplate/createContactOwnerReassignmentPreview/getContactOwnerReassignmentPreview/executeContactOwnerReassignmentPreview/downloadContactOwnerReassignmentErrors/downloadContactOwnerReassignmentResults' },
  { surface: 'admin', screen: 'agents/apidocs', action: '页面壳与导航', state: 'presentation_only' },
  { surface: 'h5', screen: 'auth/all/one/loading/error/result/done', action: '公开问卷读取、OAuth、提交、结果查询', state: 'real', operation: 'getPublicSurveyDefinition/submitPublicSurvey/queryPublicSurveySubmissionResult' },
  { surface: 'h5', screen: 'signup/active/expired/pay/qr', action: '报名或支付跳转', state: 'backend_blocked', reason: '当前公开契约没有该 H5 支付或报名会话' },
  { surface: 'sidebar', screen: 'index', action: 'context-token、workbench、profile、订单、问卷、周期订单、素材、缩略图、备注', state: 'real', operation: 'mintSidebarContext/getSidebarWorkbench/updateSidebarProfile/listSidebarOrders' },
  { surface: 'sidebar', screen: 'index', action: '素材发送', state: 'backend_blocked', reason: 'OpenAPI 只有读取素材，没有发送契约' },
] as const;

export function capabilityFor(surface: Capability['surface'], screen: string): readonly Capability[] {
  return CAPABILITIES.filter((capability) => capability.surface === surface && capability.screen.split('/').includes(screen));
}
