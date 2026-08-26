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

export const ADMIN_SCREENS = ['customers', 'customerDetail', 'questionnaires', 'questionnaireDetail', 'channels', 'channelForm', 'orders', 'orderDetail', 'spProducts', 'coupons', 'couponForm', 'images', 'agents', 'agentEdit', 'config', 'configDetail', 'automation', 'cycles', 'groupops', 'campaigns', 'ai', 'funnel', 'radar', 'tags', 'products', 'mpLib', 'attach', 'ownerMig', 'apidocs', 'productForm', 'spProductForm', 'groupopsDetail', 'radarDetail', 'radarForm', 'aiDetail', 'audienceEdit', 'cyclesDetail', 'questionnaireOps', 'spProductData', 'couponData'] as const;

export const CAPABILITIES: readonly Capability[] = [
  { surface: 'admin', screen: 'customers/customerDetail', action: '客户与关联读取', state: 'real', operation: 'listCustomers/getCustomer/getCustomerContext/listCustomerChatActivity/listCustomerSurveyAnswers/getCustomerActivityAnalytics' },
  { surface: 'admin', screen: 'customerDetail', action: '客户姓名、阶段与标签写入', state: 'real', operation: 'updateCustomer/setCustomerStage/addCustomerTag/removeCustomerTag' },
  { surface: 'admin', screen: 'questionnaires/questionnaireDetail/questionnaireOps', action: '问卷、结果、分析、运营与外推日志读取', state: 'real', operation: 'listLegacyQuestionnaires/getLegacyQuestionnaire/getLegacyQuestionnaireResults/listLegacyQuestionnaireSubmissions/getSurveySafeSubmissionAnalysis/getSurveyOperations/getSurveyOperationsPageData/listSurveyQuestionnaireExternalPushLogs' },
  { surface: 'admin', screen: 'questionnaires/questionnaireDetail', action: '问卷 CRUD、复制、启停与公开定义发布', state: 'real', operation: 'createLegacyQuestionnaire/updateLegacyQuestionnaire/deleteLegacyQuestionnaire/duplicateLegacyQuestionnaire/enableLegacyQuestionnaire/disableLegacyQuestionnaire/publishQuestionnairePublicDefinition' },
  { surface: 'admin', screen: 'questionnaireOps', action: '问卷本地 opaque 提交后动作与外部推送绑定保存', state: 'real', operation: 'saveSurveyCompletionOperations/saveSurveyExternalPushOperations' },
  { surface: 'admin', screen: 'questionnaireOps', action: '创建本地 queued 外推测试记录', state: 'real', operation: 'queueSurveyExternalPushTest' },
  { surface: 'admin', screen: 'questionnaireOps', action: '执行真实外部推送', state: 'backend_blocked', reason: '当前问卷运营 OpenAPI 不提供从页面直接执行 Provider 派发的 operation' },
  { surface: 'admin', screen: 'channels/channelForm', action: '渠道与联系人读取', state: 'real', operation: 'listLegacyChannels/getLegacyChannel/listLegacyChannelEntrants' },
  { surface: 'admin', screen: 'channels/channelForm', action: '渠道、载体、客服、素材、标签与分配策略保存', state: 'real', operation: 'createLegacyChannel/updateLegacyChannel' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '订单、items 与支付来源读取', state: 'real', operation: 'listLegacyOrders/getLegacyOrder/getLegacyOrderItems' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '微信支付与微信小店退款 intent/receipt', state: 'real', operation: 'createLegacyWechatRefundIntent/createLegacyRefundIntent' },
  { surface: 'admin', screen: 'orders/orderDetail', action: '支付宝退款 intent', state: 'backend_blocked', reason: '当前 OpenAPI 没有支付宝退款 intent operation，页面按订单 provider 阻止请求' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品与权益读取', state: 'real', operation: 'listProducts/getProduct/listProductLocalEntitlements' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品 CRUD 与本地生命周期', state: 'real', operation: 'createProduct/updateProduct/enableLegacyWechatPayProduct/disableLegacyWechatPayProduct/copyLegacyWechatPayProduct/deleteLegacyWechatPayProduct' },
  { surface: 'admin', screen: 'products/productForm', action: '普通商品公开分享与页面动作配置', state: 'backend_blocked', operation: 'getLegacyWechatPayProductShare', reason: '当前安全投影明确返回 no_authoritative_public_purchase_route，且核心商品 DTO 不包含页面动作配置' },
  { surface: 'admin', screen: 'spProducts/spProductForm/spProductData', action: '周期商品、成员和成员网格读取', state: 'real', operation: 'listServicePeriodProducts/getServicePeriodProduct/listServicePeriodMembers/getServicePeriodMemberGridAccess/getServicePeriodMemberGridSchema/listServicePeriodMemberViews/getServicePeriodMemberGridShareSettings' },
  { surface: 'admin', screen: 'spProducts/spProductForm/spProductData', action: '周期商品 CRUD、启停、复制与归档', state: 'real', operation: 'createServicePeriodProduct/updateServicePeriodProduct/enableServicePeriodProduct/disableServicePeriodProduct/copyServicePeriodProduct/archiveServicePeriodProduct' },
  { surface: 'admin', screen: 'spProducts/spProductForm', action: '周期商品公开购买与页面动作配置', state: 'backend_blocked', reason: '当前 OpenAPI 明确不提供周期商品公开购买能力，核心 DTO 也不包含页面素材和 URL 外推字段' },
  { surface: 'admin', screen: 'spProductData', action: '周期商品公开 Member Grid 读取', state: 'backend_blocked', reason: '当前 OpenAPI 没有 /shared/service-period-member-grid、公开 bootstrap 或公开 query 契约；内部 Member Grid 是 human_session/local_read_model，不能外推' },
  { surface: 'admin', screen: 'spProductData', action: '周期商品分享读取、二维码与链接预览', state: 'backend_blocked', reason: '当前 OpenAPI 没有 GET /api/admin/service-period-products/{service_product_id}/share；页面不生成二维码、链接或预览' },
  { surface: 'admin', screen: 'spProductData', action: 'Member Grid 外部分享开关', state: 'backend_blocked', operation: 'getServicePeriodMemberGridShareSettings', reason: '该读取契约明确固定 external_share_supported/external_share_enabled 为 false，且没有外部分享 PUT operation；本地协作者不等价外部分享' },
  { surface: 'admin', screen: 'coupons/couponForm/couponData', action: '优惠券、分享、领取和商品选项读取', state: 'real', operation: 'listLegacyCoupons/getLegacyCoupon/getLegacyCouponShare/listLegacyCouponClaims/listLegacyCouponProductOptions' },
  { surface: 'admin', screen: 'coupons/couponForm/couponData', action: '优惠券创建、更新、发布、停用、复制、归档与草稿删除', state: 'real', operation: 'createLegacyCoupon/updateLegacyCoupon/publishLegacyCoupon/stopLegacyCoupon/copyLegacyCoupon/archiveLegacyCoupon/deleteLegacyCoupon' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '三素材库、facets 与详情读取', state: 'real', operation: 'getLegacyImageList/getLegacyImageFacets/getLegacyImage/listLegacyAttachments/getLegacyAttachment/listLegacyMiniPrograms/getLegacyMiniProgram' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '素材上传、编辑与删除', state: 'real', operation: 'uploadLegacyImage/updateLegacyImage/deleteLegacyImage/uploadLegacyAttachment/updateLegacyAttachment/deleteLegacyAttachment/createLegacyMiniProgram/updateLegacyMiniProgram/deleteLegacyMiniProgram' },
  { surface: 'admin', screen: 'images/attach/mpLib', action: '附件二进制下载', state: 'real', operation: 'downloadLegacyAttachment' },
  { surface: 'admin', screen: 'images', action: '图片缩略图读取与 object URL 生命周期', state: 'real', operation: 'getLegacyImageVariant' },
  { surface: 'admin', screen: 'tags', action: '企微标签组、标签和 live gate 读取', state: 'real', operation: 'listLegacyWecomTags/listLegacyWecomTagGroups/getLegacyWecomTag/getLegacyWecomTagGroup/getLegacyWecomTagExecutionGate' },
  { surface: 'admin', screen: 'tags', action: '企微标签组/标签写入与同步队列', state: 'real', operation: 'createLegacyWecomTagGroup/updateLegacyWecomTagGroupPatch/createLegacyWecomTag/updateLegacyWecomTagPatch/archiveLegacyWecomTag/queueLegacyWecomTagSync' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '人群包与分组读取、分组增改删、启停、复制、归档', state: 'real', operation: 'listAIAudiencePackageGroups/listAIAudiencePackages/createAIAudiencePackageGroup/updateAIAudiencePackageGroup/deleteAIAudiencePackageGroup/activateAIAudiencePackage/pauseAIAudiencePackage/copyAIAudiencePackage/archiveAIAudiencePackage' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '人群条件、发送人、自动化绑定、配置版本、预览与成员物化', state: 'real', operation: 'getAIAudiencePackage/getAIAudienceAutomationBinding/getAIAudiencePackageSenders/getAIAudienceConfigurationVersion/listAIAudiencePackageMembers/updateAIAudiencePackage/replaceAIAudiencePackageSenders/putAIAudienceAutomationBinding/deleteAIAudienceAutomationBinding/putAIAudienceConfigurationVersion/previewAIAudienceConfiguration/materializeAIAudienceConfiguration' },
  { surface: 'admin', screen: 'campaigns', action: 'Campaign 列表状态筛选、详情与受服务端状态约束的删除', state: 'real', operation: 'listCloudCampaigns/getCloudCampaign/deleteCloudCampaign' },
  { surface: 'admin', screen: 'campaigns', action: 'Campaign scoped touch-plan、审核与 canonical OneID 收件人分页读取', state: 'real', operation: 'listCloudCampaignTouchPlans/getCloudCampaignTouchPlan/getCloudCampaignTouchPlanReview/mutateCloudCampaignTouchPlanReview/listCloudCampaignTouchPlanRecipients/getCloudCampaignTouchPlanRecipient' },
  { surface: 'admin', screen: 'campaigns', action: 'Touch-plan 单客户本地消息覆盖、批准与拒绝', state: 'real', operation: 'getCloudCampaignTouchPlanRecipientReview/mutateCloudCampaignTouchPlanRecipientReview' },
  { surface: 'admin', screen: 'campaigns', action: 'Campaign 成员状态、消息任务与 trace/session 审计筛选', state: 'backend_blocked', reason: '当前 OpenAPI 没有 Campaign 成员状态、收件人消息任务或可筛选 observability/audit JSON 契约；旧 observability 跳转不是可渲染证据' },
  { surface: 'admin', screen: 'automation/audienceEdit', action: '从人群包直接创建真实外部群发', state: 'backend_blocked', reason: '当前 AI Audience OpenAPI 不提供群发任务创建与 Provider receipt 契约' },
  { surface: 'admin', screen: 'groupops/groupopsDetail', action: '群运营计划、成员、群资产、节点、Webhook 描述符、内容预览与生命周期', state: 'real', operation: 'listGroupOpsPlans/getGroupOpsPlan/createGroupOpsPlan/updateGroupOpsPlan/deleteGroupOpsPlan/activateGroupOpsPlan/pauseGroupOpsPlan/archiveGroupOpsPlan/addGroupOpsPlanMember/removeGroupOpsPlanMember/addGroupOpsPlanGroupAsset/removeGroupOpsPlanGroupAsset/addGroupOpsPlanNode/updateGroupOpsPlanNode/removeGroupOpsPlanNode/putGroupOpsWebhookDescriptor/previewGroupOpsPlanContent/listGroupOpsExecutions' },
  { surface: 'admin', screen: 'groupops/groupopsDetail', action: '接受 run-due、broadcast、webhook 或真实外部群发', state: 'backend_blocked', reason: '这些 operation 会进入执行边界；本前端替换任务明确禁止执行真实群发，页面只维护本地计划定义和读取执行证据' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '内容雷达列表、事件、分享与启停', state: 'real', operation: 'listRadarLinks/getRadarLink/listRadarLinkEvents/getRadarLinkShareProjection/enableRadarLink/disableRadarLink' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '链接、图片与 PDF 雷达新建和编辑', state: 'real', operation: 'createRadarLink/updateRadarLink' },
  { surface: 'admin', screen: 'radar/radarDetail/radarForm', action: '图片/PDF 雷达素材上传', state: 'real', operation: 'uploadLegacyImage/uploadLegacyAttachment' },
  { surface: 'admin', screen: 'ai/aiDetail', action: 'AI 计划审批', state: 'backend_blocked', reason: '当前导入壳使用的 ai-assist/review-plans DTO 不在 OpenAPI 中' },
  { surface: 'admin', screen: 'funnel/cycles/cyclesDetail', action: '原型漏斗和复盘会话', state: 'backend_blocked', reason: '当前壳 DTO 与 Member Grid、execution-runtime 契约不等价' },
  { surface: 'admin', screen: 'config/configDetail', action: '配置类目安全投影读取', state: 'real', operation: 'listAdminOpsCategories/getAdminOpsCategory' },
  { surface: 'admin', screen: 'config/configDetail', action: '应用设置读取与非敏感字段保存', state: 'real', operation: 'getLegacyAppSettingsResource/saveLegacyAppSettingsResource' },
  { surface: 'admin', screen: 'config/configDetail', action: 'Push 能力与配置发布记录读取', state: 'real', operation: 'getAdminOpsPushCapabilities/listAdminOpsReleases' },
  { surface: 'admin', screen: 'configDetail', action: '配置类目启停、保存与检查', state: 'backend_blocked', operation: 'setAdminOpsCategoryEnabled/setAdminOpsCategorySettings/checkAdminOpsCategory', reason: '这些写入要求 route-bound Admin Action Token，但当前 JSON category DTO 不返回 token' },
  { surface: 'admin', screen: 'configDetail', action: 'Push 调度器与单项能力写入', state: 'backend_blocked', operation: 'setAdminOpsPushScheduler/setAdminOpsPushCapability', reason: '这些写入要求 route-bound Admin Action Token，但当前 Push 能力安全投影 DTO 不返回 token' },
  { surface: 'admin', screen: 'configDetail', action: '配置发布创建、校验、发布与回滚', state: 'backend_blocked', operation: 'createAdminOpsRelease/validateAdminOpsRelease/publishAdminOpsRelease/rollbackAdminOpsRelease', reason: '这些写入要求 route-bound Admin Action Token，当前 Kimi 配置页也没有可等价映射的 release changes 与 checksum 确认表单' },
  { surface: 'admin', screen: 'ownerMig', action: '本地负责人迁移模板、持久预览、确认执行与脱敏报告', state: 'real', operation: 'downloadContactOwnerReassignmentTemplate/createContactOwnerReassignmentPreview/getContactOwnerReassignmentPreview/executeContactOwnerReassignmentPreview/downloadContactOwnerReassignmentErrors/downloadContactOwnerReassignmentResults' },
  { surface: 'admin', screen: 'agents/agentEdit', action: 'HXC 本地发送人读取、保存、排序与归档', state: 'real', operation: 'getLegacyHXCSendConfig/upsertLegacyHXCSendConfig/reorderLegacyHXCSendConfigs/archiveLegacyHXCSendConfig' },
  { surface: 'admin', screen: 'agents/agentEdit', action: '通用 Agent Prompt、版本与固定素材写入', state: 'backend_blocked', reason: '通用 Agent DTO 与 Audience automation binding、Group Ops plan/node 契约不等价；发送人工作区不暴露这些字段' },
  { surface: 'admin', screen: 'apidocs', action: 'OpenAPI 文档浏览、搜索与复制', state: 'presentation_only' },
  { surface: 'h5', screen: 'auth/all/one/loading/error/result/done', action: '公开问卷读取、OAuth、提交、结果查询', state: 'real', operation: 'getPublicSurveyDefinition/submitPublicSurvey/queryPublicSurveySubmissionResult' },
  { surface: 'h5', screen: 'signup/active/expired/pay/qr', action: '报名或支付跳转', state: 'backend_blocked', reason: '当前公开契约没有该 H5 支付或报名会话' },
  { surface: 'sidebar', screen: 'index', action: 'context-token、workbench、安全时间线、聊天活动、profile、订单、问卷、周期订单、素材、缩略图、备注', state: 'real', operation: 'mintSidebarContext/getSidebarWorkbench/listSidebarTimeline/listSidebarChatActivity/updateSidebarProfile/listSidebarOrders' },
  { surface: 'sidebar', screen: 'index', action: '素材发送', state: 'backend_blocked', reason: 'OpenAPI 只有读取素材，没有发送契约' },
] as const;

export function capabilityFor(surface: Capability['surface'], screen: string): readonly Capability[] {
  return CAPABILITIES.filter((capability) => capability.surface === surface && capability.screen.split('/').includes(screen));
}
