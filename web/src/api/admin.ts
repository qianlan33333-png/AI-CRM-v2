/** Current Go OpenAPI -> Kimi Admin DTO boundary. No page imports generated data. */
import {
  addCustomerTag, getCustomer, getCustomerContext, listCustomerSurveyAnswers, listStages, removeCustomerTag, setCustomerStage, updateCustomer,
  getLegacyAttachment, getLegacyChannel, getLegacyCoupon, getLegacyCouponShare, getLegacyImage, getLegacyImageFacets,
  getLegacyMiniProgram, getLegacyOrder, getLegacyOrderItems, getLegacyQuestionnaire, getLegacyQuestionnaireResults,
  getAdminOpsCategory, getContactOwnerReassignmentPreview, getLegacyWecomTag, getLegacyWecomTagExecutionGate, getLegacyWecomTagGroup, getProduct,
  getServicePeriodMember, getServicePeriodMemberGridAccess, getServicePeriodMemberGridSchema, getServicePeriodMemberGridShareSettings,
  getServicePeriodProduct, getSurveyOperations, getSurveyOperationsPageData, getSurveySafeSubmissionAnalysis,
  listAdminOpsCategories, listCustomers, listLegacyAttachments, listLegacyChannelEntrants, listLegacyChannels, listLegacyCouponClaims,
  listLegacyCouponProductOptions, listLegacyCoupons, getLegacyImageList, listLegacyMiniPrograms, listLegacyOrders, listLegacyRefunds, listLegacyWechatOrderExternalEffects,
  listLegacyQuestionnaireSubmissions, listLegacyQuestionnaires, listLegacyWecomTagGroups, listLegacyWecomTags,
  listProductLocalEntitlements, listProducts, listServicePeriodMemberViews, listServicePeriodMembers, queryServicePeriodMemberGrid,
  listServicePeriodProducts, listSurveyQuestionnaireExternalPushLogs,
  activateAIAudiencePackage, archiveLegacyWecomTag, archiveLegacyWecomTagGroup, archiveAIAudiencePackage, archiveServicePeriodProduct, copyAIAudiencePackage, copyLegacyWechatPayProduct, copyServicePeriodProduct, createAIAudiencePackageGroup, createLegacyMiniProgram, createLegacyWecomTag, createLegacyWecomTagGroup, createProduct, createRadarLink, createServicePeriodProduct, createServicePeriodMemberGridCollaborator, deleteAIAudiencePackageGroup, deleteLegacyAttachment, deleteLegacyImage, deleteLegacyMiniProgram, deleteServicePeriodMemberGridCollaborator, disableLegacyWechatPayProduct, disableRadarLink, disableServicePeriodProduct, enableLegacyWechatPayProduct, enableRadarLink, enableServicePeriodProduct, executeContactOwnerReassignmentPreview, getAIAudiencePackage, getCreateContactOwnerReassignmentPreviewUrl, getDownloadContactOwnerReassignmentErrorsUrl, getDownloadContactOwnerReassignmentResultsUrl, getDownloadContactOwnerReassignmentTemplateUrl, getDownloadLegacyAttachmentUrl, getGetLegacyImageVariantUrl, getRadarLink, getRadarLinkShareProjection, listAIAudiencePackageGroups, listAIAudiencePackages, listRadarLinkEvents, listRadarLinks, pauseAIAudiencePackage, queueLegacyWecomTagSync, updateAIAudiencePackageGroup, updateLegacyAttachment, updateLegacyImage, updateLegacyMiniProgram, updateLegacyWecomTagGroupPatch, updateLegacyWecomTagPatch, updateProduct, updateRadarLink, updateServicePeriodMemberFields, updateServicePeriodMemberGridCollaborator, updateServicePeriodProduct, uploadLegacyAttachment, uploadLegacyImage, type ContactOwnerReassignmentPreview as ApiOwnerReassignmentPreview, type Customer as ApiCustomer, type LegacyChannel, type LegacyChannelListItem, type LegacyQuestionnaire, type RadarLink as ApiRadarLink,
} from './generated/health';
import { archiveLegacyHXCSendConfig, getLegacyHXCSendConfig, reorderLegacyHXCSendConfigs, upsertLegacyHXCSendConfig, type LegacyHXCSenderConfig } from './generated/health';
import { getLegacyAppSettingsResource, saveLegacyAppSettingsResource } from './generated/health';
import { getAdminOpsPushCapabilities, listAdminOpsReleases } from './generated/health';
import { archiveLegacyCoupon, copyLegacyCoupon, createLegacyCoupon, deleteLegacyCoupon, deleteLegacyWechatPayProduct, publishLegacyCoupon, stopLegacyCoupon, updateLegacyCoupon, type CouponUpsertRequest } from './generated/health';
import { createLegacyQuestionnaire, deleteLegacyQuestionnaire, disableLegacyQuestionnaire, duplicateLegacyQuestionnaire, enableLegacyQuestionnaire, publishQuestionnairePublicDefinition, updateLegacyQuestionnaire, type LegacyQuestionnaireCreateRequest } from './generated/health';
import { createLegacyChannel, updateLegacyChannel, type LegacyChannelWriteRequest } from './generated/health';
import { deleteAIAudienceAutomationBinding, getAIAudienceAutomationBinding, getAIAudienceConfigurationVersion, getAIAudiencePackageSenders, listAIAudiencePackageMembers, materializeAIAudienceConfiguration, previewAIAudienceConfiguration, putAIAudienceAutomationBinding, putAIAudienceConfigurationVersion, replaceAIAudiencePackageSenders, updateAIAudiencePackage, type AIAudiencePackageSender, type SegmentDefinition } from './generated/health';
import { activateGroupOpsPlan, addGroupOpsPlanGroupAsset, addGroupOpsPlanMember, addGroupOpsPlanNode, archiveGroupOpsPlan, createGroupOpsPlan, deleteGroupOpsPlan, getGroupOpsPlan, getGroupOpsWebhookDescriptor, listGroupOpsExecutions, listGroupOpsPlans, pauseGroupOpsPlan, previewGroupOpsPlanContent, previewGroupOpsRunDue, putGroupOpsWebhookDescriptor, removeGroupOpsPlanGroupAsset, removeGroupOpsPlanMember, removeGroupOpsPlanNode, updateGroupOpsPlan, updateGroupOpsPlanNode, type GroupOpsNodeRequest } from './generated/health';
import { deleteCloudCampaign, getCloudCampaign, getCloudCampaignTouchPlan, getCloudCampaignTouchPlanRecipient, getCloudCampaignTouchPlanRecipientReview, getCloudCampaignTouchPlanReview, listCloudCampaignPlans, listCloudCampaigns, listCloudCampaignTouchPlanRecipients, listCloudCampaignTouchPlans, mutateCloudCampaignTouchPlanRecipientReview, mutateCloudCampaignTouchPlanReview } from './generated/health';
import { acceptOutboundCampaignHandoff, dispatchOutboundCampaignHandoff, getOutboundCampaignDispatchReconciliation, getOutboundCampaignHandoffSummary, reconcileOutboundCampaignHandoff } from './generated/health';
import { createLegacyRefundIntent, createLegacyWechatRefundIntent, queueSurveyExternalPushTest, saveSurveyCompletionOperations, saveSurveyExternalPushOperations, type WechatShopRefundRequest } from './generated/health';
import { getChannelAcquisitionAsset, getChannelAcquisitionPreview, listChannelAcquisitionAssets, listChannelAcquisitionStaff, publishChannelAcquisitionAsset, updateChannelAcquisitionAssignees, type ChannelAcquisitionAssignmentRequest, type ChannelAcquisitionAssetPublishRequest } from './generated/health';
import type { AdminDb, AttachItem, Channel, ChannelAcquisitionAsset, ChannelAcquisitionAssetKind, ChannelAcquisitionAssignmentInput, ChannelAcquisitionAssignee, ChannelAcquisitionPreview, ChannelAcquisitionStaff, ChannelEntrant, ConfigCategory, Coupon, Customer, Customer360Context, Customer360ChatEntry, Customer360SurveyProjection, Customer360TimelineEntry, GroupOpsMaterialKind, GroupOpsMaterialPlan, ImageItem, MpItem, Order, OwnerReassignmentPreview, Product, Questionnaire, QuestionnaireOps, RadarLinkInput, RadarMedia, SpProduct, TagGroup, Tone, WecomTag } from '../shared/api/types';
import { ApiError, apiRequestOptions, request, unwrapGenerated } from './transport';

type Obj = Record<string, unknown>;
const obj = (value: unknown): Obj => value && typeof value === 'object' ? value as Obj : {};
const text = (value: unknown, fallback = '—'): string => value == null || value === '' ? fallback : String(value);
const list = (value: unknown, ...keys: string[]): unknown[] => { const source = obj(value); for (const key of keys) if (Array.isArray(source[key])) return source[key] as unknown[]; return []; };
const toneFor = (status: unknown): Tone => { const value = text(status, '').toLowerCase(); if (/active|enabled|paid|success|completed|published/.test(value)) return 'ok'; if (/pending|draft|processing/.test(value)) return 'warn'; if (/disabled|archived|failed|cancel|closed/.test(value)) return 'gray'; return 'blue'; };
const call = async <T>(request: Promise<T>): Promise<unknown> => unwrapGenerated(await request as { status: number; data: unknown }) as unknown;

export type CampaignFilter = {
  approvalStatus?: 'draft' | 'approved' | 'rejected';
  runtimeStatus?: 'idle' | 'planned' | 'paused';
};
export type CampaignListItem = { code: string; name: string; approvalStatus: string; runtimeStatus: string; version: number; updatedAt: string };
export type CampaignDetail = CampaignListItem & { steps: Array<{ index: number; delayMinutes: number; content: string }> };
export type CampaignTouchPlan = { id: string; campaignCode: string; campaignVersion: number; sourceKind: string; targetCount: number; contentStepCount: number; createdAt: string };
export type CampaignTouchPlanIndexItem = CampaignTouchPlan & { reviewStatus: 'draft' | 'pending_review' | 'approved' | 'rejected'; reviewVersion: number };
export type CampaignTouchPlanIndexPage = { items: CampaignTouchPlanIndexItem[]; nextCursor: string | null };
export type CampaignTouchPlanDetail = CampaignTouchPlan & { steps: Array<{ index: number; delayMinutes: number; content: string }> };
export type CampaignTouchPlanReview = { status: string; version: number; handoffStatus: string | null };
export type CampaignTouchPlanRecipient = { customerID: number };
export type CampaignTouchPlanRecipientPage = { items: CampaignTouchPlanRecipient[]; nextCursor: string | null };
export type CampaignTouchPlanRecipientReview = { customerID: number; messageOverride: string; status: 'pending_review' | 'approved' | 'rejected'; version: number; updatedAt: string };
export type CampaignOutboundHandoff = { id: number; campaignCode: string; planID: string; reviewVersion: number; status: 'held'; targetCount: number; stepCount: number; acceptedAt: string; providerExecutionEligible: boolean };
export type CampaignOutboundHandoffReconciliation = CampaignOutboundHandoff & { heldCount: number; blockedCount: number; pendingCount: number; notEvaluatedCount: number; eligibleCount: number; inactiveCount: number; contactPolicyCount: number };
export type CampaignOutboundDispatchReconciliation = { handoffID: number; blocked: number; accepted: number; queued: number; attempted: number; executed: number; outcomeUnknown: number; reconciled: number; retryableFailed: number; finalFailed: number; providerExecutionEligible: boolean };

const requiredText = (source: Obj, field: string): string => {
  const value = source[field];
  if (typeof value !== 'string' || !value) throw new Error(`Campaign 响应缺少 ${field}`);
  return value;
};
const requiredPositive = (source: Obj, field: string): number => {
  const value = Number(source[field]);
  if (!Number.isSafeInteger(value) || value < 1) throw new Error(`Campaign 响应缺少有效 ${field}`);
  return value;
};
const requiredCount = (source: Obj, field: string): number => {
  const value = Number(source[field]);
  if (!Number.isSafeInteger(value) || value < 0) throw new Error(`Campaign 响应缺少有效 ${field}`);
  return value;
};
const requireCampaignLocal = (source: Obj): void => {
  if (source.local_projection !== true || source.real_external_call_executed !== false || source.real_send !== false || source.runtime_executed !== false) throw new Error('Campaign 响应越过本地执行边界');
};
const requireTouchPlanLocal = (source: Obj): void => {
  if (source.local_only !== true || source.provider_execution_eligible !== false || source.runtime_executed === true || source.real_external_call_executed !== false || source.delivery_proven !== false) throw new Error('Touch plan 响应越过本地执行边界');
};
const requireOutboundHandoffSafety = (source: Obj): boolean => {
  const safety = obj(source.safety);
  if (safety.local_only !== true || typeof safety.provider_execution_eligible !== 'boolean' || safety.real_external_call_executed !== false || safety.delivery_proven !== false) throw new Error('Campaign handoff 响应越过本地执行边界');
  return safety.provider_execution_eligible;
};
const campaignHandoffDto = (value: unknown): CampaignOutboundHandoff => {
  const source = obj(value);
  const status = requiredText(source, 'status');
  if (status !== 'held') throw new Error('Campaign handoff 状态不受支持');
  return { id: requiredPositive(source, 'id'), campaignCode: requiredText(source, 'campaign_code'), planID: requiredText(source, 'plan_id'), reviewVersion: requiredPositive(source, 'review_version'), status, targetCount: requiredPositive(source, 'target_count'), stepCount: requiredPositive(source, 'step_count'), acceptedAt: requiredText(source, 'accepted_at'), providerExecutionEligible: requireOutboundHandoffSafety(source) };
};
const campaignHandoffReconciliationDto = (value: unknown): CampaignOutboundHandoffReconciliation => {
  const source = obj(value);
  return { ...campaignHandoffDto(source), heldCount: requiredCount(source, 'held_count'), blockedCount: requiredCount(source, 'blocked_count'), pendingCount: requiredCount(source, 'pending_count'), notEvaluatedCount: requiredCount(source, 'not_evaluated_count'), eligibleCount: requiredCount(source, 'eligible_count'), inactiveCount: requiredCount(source, 'inactive_count'), contactPolicyCount: requiredCount(source, 'contact_policy_count') };
};
const requireHandoffScope = (handoff: CampaignOutboundHandoff, campaignCode: string, planID: string): void => {
  if (handoff.campaignCode !== campaignCode || handoff.planID !== planID) throw new Error('Campaign handoff 返回范围不匹配');
};
const campaignDispatchReconciliationDto = (value: unknown): CampaignOutboundDispatchReconciliation => {
  const source = obj(value);
  if (typeof source.provider_execution_eligible !== 'boolean' || source.business_call_dispatched !== false || source.real_external_call_executed !== false || source.delivery_proven !== false) throw new Error('Campaign dispatch 响应越过本地执行边界');
  return { handoffID: requiredPositive(source, 'handoff_id'), blocked: requiredCount(source, 'blocked'), accepted: requiredCount(source, 'accepted'), queued: requiredCount(source, 'queued'), attempted: requiredCount(source, 'attempted'), executed: requiredCount(source, 'executed'), outcomeUnknown: requiredCount(source, 'outcome_unknown'), reconciled: requiredCount(source, 'reconciled'), retryableFailed: requiredCount(source, 'retryable_failed'), finalFailed: requiredCount(source, 'final_failed'), providerExecutionEligible: source.provider_execution_eligible };
};
const campaignItemDto = (value: unknown): CampaignListItem => {
  const source = obj(value);
  return { code: requiredText(source, 'campaign_code'), name: requiredText(source, 'name'), approvalStatus: requiredText(source, 'approval_status'), runtimeStatus: requiredText(source, 'runtime_status'), version: requiredPositive(source, 'version'), updatedAt: requiredText(source, 'updated_at') };
};
const touchPlanDto = (value: unknown): CampaignTouchPlan => {
  const source = obj(value);
  return { id: requiredText(source, 'id'), campaignCode: requiredText(source, 'campaign_code'), campaignVersion: requiredPositive(source, 'campaign_version'), sourceKind: requiredText(obj(source.source), 'kind'), targetCount: requiredPositive(source, 'target_count'), contentStepCount: requiredPositive(source, 'content_step_count'), createdAt: requiredText(source, 'created_at') };
};
const touchPlanStepsDto = (value: unknown): Array<{ index: number; delayMinutes: number; content: string }> => list(value, 'steps').map((item) => {
  const source = obj(item);
  return { index: requiredPositive(source, 'step_index'), delayMinutes: Number(source.delay_minutes), content: requiredText(source, 'content') };
});
const mutationOptions = (): RequestInit => {
  if (typeof globalThis.crypto?.randomUUID !== 'function') throw new Error('浏览器不支持安全幂等键，已拒绝提交 Campaign 本地审核');
  return apiRequestOptions({ headers: { 'Idempotency-Key': `campaign-review-${globalThis.crypto.randomUUID()}` } });
};
const handoffMutationOptions = (operation: 'accept' | 'dispatch'): RequestInit => {
  if (typeof globalThis.crypto?.randomUUID !== 'function') throw new Error('浏览器不支持安全幂等键，已拒绝提交 Campaign handoff 操作');
  return apiRequestOptions({ headers: { 'Idempotency-Key': `campaign-handoff-${operation}-${globalThis.crypto.randomUUID()}` } });
};

export async function listCampaignsDto(filter: CampaignFilter = {}): Promise<CampaignListItem[]> {
  const source = obj(await call(listCloudCampaigns({ approval_status: filter.approvalStatus, runtime_status: filter.runtimeStatus }, apiRequestOptions())));
  requireCampaignLocal(source);
  return list(source, 'items').map(campaignItemDto);
}
export async function listCampaignPlanIndexDto(reviewStatus?: CampaignTouchPlanIndexItem['reviewStatus'], cursor?: string): Promise<CampaignTouchPlanIndexPage> {
  const source = obj(await call(listCloudCampaignPlans({ review_status: reviewStatus, cursor, limit: 100 }, apiRequestOptions())));
  requireTouchPlanLocal(source);
  return {
    items: list(source, 'items').map((item) => {
      const row = obj(item);
      const plan = obj(row.plan);
      const status = row.review_status;
      requireTouchPlanLocal(plan);
      if (status !== 'draft' && status !== 'pending_review' && status !== 'approved' && status !== 'rejected') throw new Error('Campaign 计划审核状态无效');
      return { ...touchPlanDto(plan), reviewStatus: status, reviewVersion: requiredPositive(row, 'review_version') };
    }),
    nextCursor: typeof source.next_cursor === 'string' ? source.next_cursor : null,
  };
}
export async function getCampaignDto(campaignCode: string): Promise<CampaignDetail> {
  const source = obj(await call(getCloudCampaign(campaignCode, apiRequestOptions())));
  requireCampaignLocal(source);
  const campaign = campaignItemDto(source.campaign);
  return { ...campaign, steps: touchPlanStepsDto(source) };
}
export async function deleteCampaignDto(campaignCode: string): Promise<void> {
  const campaign = await getCampaignDto(campaignCode);
  const source = obj(await call(deleteCloudCampaign(campaignCode, { expected_version: campaign.version }, mutationOptions())));
  requireCampaignLocal(source);
  if (source.deleted !== true || requiredText(source, 'campaign_code') !== campaignCode) throw new Error('Campaign 删除响应不完整');
}
export async function listCampaignTouchPlansDto(campaignCode: string): Promise<CampaignTouchPlan[]> {
  const source = obj(await call(listCloudCampaignTouchPlans(campaignCode, { limit: 100 }, apiRequestOptions())));
  requireTouchPlanLocal(source);
  return list(source, 'items').map(touchPlanDto);
}
export async function getCampaignTouchPlanDto(campaignCode: string, planID: string): Promise<CampaignTouchPlanDetail> {
  const source = obj(await call(getCloudCampaignTouchPlan(campaignCode, planID, apiRequestOptions())));
  requireTouchPlanLocal(source);
  return { ...touchPlanDto(source), steps: touchPlanStepsDto(obj(source.content)) };
}
export async function getCampaignTouchPlanReviewDto(campaignCode: string, planID: string): Promise<CampaignTouchPlanReview> {
  const source = obj(await call(getCloudCampaignTouchPlanReview(campaignCode, planID, apiRequestOptions())));
  requireTouchPlanLocal(source);
  const review = obj(source.review);
  const handoff = obj(source.handoff);
  return { status: requiredText(review, 'status'), version: requiredPositive(review, 'version'), handoffStatus: typeof handoff.status === 'string' ? handoff.status : null };
}
export async function decideCampaignTouchPlanReviewDto(campaignCode: string, planID: string, operation: 'approve' | 'reject'): Promise<CampaignTouchPlanReview> {
  const current = await getCampaignTouchPlanReviewDto(campaignCode, planID);
  if (current.status !== 'pending_review') throw new Error('当前计划不在待审核状态，已拒绝提交');
  const source = obj(await call(mutateCloudCampaignTouchPlanReview(campaignCode, planID, operation, { expected_version: current.version, confirmation: `${operation.toUpperCase()} ${planID}` }, mutationOptions())));
  requireTouchPlanLocal(source);
  const review = obj(source.review);
  return { status: requiredText(review, 'status'), version: requiredPositive(review, 'version'), handoffStatus: typeof obj(source.handoff).status === 'string' ? String(obj(source.handoff).status) : null };
}
export async function listCampaignTouchPlanRecipientsDto(campaignCode: string, planID: string, cursor?: string): Promise<CampaignTouchPlanRecipientPage> {
  const source = obj(await call(listCloudCampaignTouchPlanRecipients(campaignCode, planID, { limit: 50, cursor }, apiRequestOptions())));
  requireTouchPlanLocal(source);
  return { items: list(source, 'items').map((item) => ({ customerID: requiredPositive(obj(item), 'canonical_customer_id') })), nextCursor: typeof source.next_cursor === 'string' ? source.next_cursor : null };
}
export async function getCampaignTouchPlanRecipientDto(campaignCode: string, planID: string, customerID: number): Promise<CampaignTouchPlanRecipient> {
  const source = obj(await call(getCloudCampaignTouchPlanRecipient(campaignCode, planID, customerID, apiRequestOptions())));
  requireTouchPlanLocal(source);
  const returnedID = requiredPositive(source, 'canonical_customer_id');
  if (returnedID !== customerID) throw new Error('Campaign 收件人范围不匹配');
  return { customerID: returnedID };
}
const recipientReviewDto = (value: unknown, customerID: number): CampaignTouchPlanRecipientReview => {
  const source = obj(value);
  const returnedID = requiredPositive(source, 'canonical_customer_id');
  const status = source.status;
  if (returnedID !== customerID || (status !== 'pending_review' && status !== 'approved' && status !== 'rejected')) throw new Error('Campaign 单客户审核范围不匹配');
  return { customerID: returnedID, messageOverride: typeof source.message_override === 'string' ? source.message_override : '', status, version: requiredPositive(source, 'version'), updatedAt: requiredText(source, 'updated_at') };
};
export async function getCampaignTouchPlanRecipientReviewDto(campaignCode: string, planID: string, customerID: number): Promise<CampaignTouchPlanRecipientReview | null> {
  try {
    const source = obj(await call(getCloudCampaignTouchPlanRecipientReview(campaignCode, planID, customerID, apiRequestOptions())));
    requireTouchPlanLocal(source);
    return recipientReviewDto(source.review, customerID);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null;
    throw error;
  }
}
export async function saveCampaignTouchPlanRecipientMessageDto(campaignCode: string, planID: string, customerID: number, messageOverride: string): Promise<CampaignTouchPlanRecipientReview> {
  const [planReview, current] = await Promise.all([getCampaignTouchPlanReviewDto(campaignCode, planID), getCampaignTouchPlanRecipientReviewDto(campaignCode, planID, customerID)]);
  if (!messageOverride.trim()) throw new Error('单客户消息不能为空');
  const source = obj(await call(mutateCloudCampaignTouchPlanRecipientReview(campaignCode, planID, customerID, 'message', { expected_plan_version: planReview.version, expected_recipient_version: current?.version || 0, message_override: messageOverride }, mutationOptions())));
  requireTouchPlanLocal(source);
  return recipientReviewDto(source.review, customerID);
}
export async function decideCampaignTouchPlanRecipientReviewDto(campaignCode: string, planID: string, customerID: number, operation: 'approve' | 'reject'): Promise<CampaignTouchPlanRecipientReview> {
  const [planReview, current] = await Promise.all([getCampaignTouchPlanReviewDto(campaignCode, planID), getCampaignTouchPlanRecipientReviewDto(campaignCode, planID, customerID)]);
  if (planReview.status !== 'pending_review') throw new Error('当前计划不在待审核状态，已拒绝单客户审核');
  if (current && current.status !== 'pending_review') throw new Error('当前单客户审核已终态，已拒绝重复操作');
  const source = obj(await call(mutateCloudCampaignTouchPlanRecipientReview(campaignCode, planID, customerID, operation, { expected_plan_version: planReview.version, expected_recipient_version: current?.version || 0 }, mutationOptions())));
  requireTouchPlanLocal(source);
  return recipientReviewDto(source.review, customerID);
}
export async function getCampaignOutboundHandoffDto(campaignCode: string, planID: string): Promise<CampaignOutboundHandoff> {
  const handoff = campaignHandoffDto(await call(getOutboundCampaignHandoffSummary(campaignCode, planID, apiRequestOptions())));
  requireHandoffScope(handoff, campaignCode, planID);
  return handoff;
}
export async function getCampaignOutboundHandoffReconciliationDto(campaignCode: string, planID: string): Promise<CampaignOutboundHandoffReconciliation> {
  const handoff = campaignHandoffReconciliationDto(await call(reconcileOutboundCampaignHandoff(campaignCode, planID, apiRequestOptions())));
  requireHandoffScope(handoff, campaignCode, planID);
  return handoff;
}
export async function getCampaignOutboundDispatchReconciliationDto(campaignCode: string, planID: string): Promise<CampaignOutboundDispatchReconciliation> {
  const handoff = await getCampaignOutboundHandoffDto(campaignCode, planID);
  const reconciliation = campaignDispatchReconciliationDto(await call(getOutboundCampaignDispatchReconciliation(campaignCode, planID, apiRequestOptions())));
  if (reconciliation.handoffID !== handoff.id) throw new Error('Campaign dispatch 返回 handoff 范围不匹配');
  return reconciliation;
}
export async function tryGetCampaignOutboundHandoffDto(campaignCode: string, planID: string): Promise<CampaignOutboundHandoff | null> {
  try {
    return await getCampaignOutboundHandoffDto(campaignCode, planID);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null;
    throw error;
  }
}
export async function tryGetCampaignOutboundDispatchReconciliationDto(campaignCode: string, planID: string): Promise<CampaignOutboundDispatchReconciliation | null> {
  try {
    return await getCampaignOutboundDispatchReconciliationDto(campaignCode, planID);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null;
    throw error;
  }
}
export async function acceptCampaignOutboundHandoffDto(campaignCode: string, planID: string): Promise<CampaignOutboundHandoffReconciliation> {
  const review = await getCampaignTouchPlanReviewDto(campaignCode, planID);
  if (review.status !== 'approved' || !review.handoffStatus) throw new Error('计划尚未完成本地审核，已拒绝受理 handoff');
  const handoff = campaignHandoffReconciliationDto(await call(acceptOutboundCampaignHandoff(campaignCode, planID, { expected_review_version: review.version }, handoffMutationOptions('accept'))));
  requireHandoffScope(handoff, campaignCode, planID);
  return handoff;
}
export async function dispatchCampaignOutboundHandoffDto(campaignCode: string, planID: string): Promise<CampaignOutboundDispatchReconciliation> {
  const handoff = await getCampaignOutboundHandoffDto(campaignCode, planID);
  if (handoff.status !== 'held') throw new Error('Campaign handoff 尚未处于受理状态，已拒绝排入本地 EER');
  const reconciliation = campaignDispatchReconciliationDto(await call(dispatchOutboundCampaignHandoff(campaignCode, planID, { external_gate: true }, handoffMutationOptions('dispatch'))));
  if (reconciliation.handoffID !== handoff.id) throw new Error('Campaign dispatch 返回 handoff 范围不匹配');
  return reconciliation;
}

export function customerPageDto(customer: ApiCustomer): Customer { return { id: String(customer.id), name: customer.name, owner: customer.owner_staff_id == null ? '未分配' : String(customer.owner_staff_id), stageId: customer.stage_id }; }
const requiredContextNumber = (value: unknown, field: string): number => { const number = Number(value); if (!Number.isSafeInteger(number) || number < 1) throw new Error(`客户安全上下文缺少有效 ${field}`); return number; };
const requiredContextText = (value: unknown, field: string): string => { if (typeof value !== 'string' || !value.trim()) throw new Error(`客户安全上下文缺少 ${field}`); return value; };
const optionalContextNumber = (value: unknown): number | null => { if (value == null) return null; const number = Number(value); return Number.isSafeInteger(number) && number >= 1 ? number : null; };
const optionalContextText = (value: unknown): string | null => typeof value === 'string' ? value : null;
export function customerContextPageDto(value: unknown): Customer360Context {
  const source = obj(value);
  const customer = obj(source.customer);
  const profile = {
    id: String(requiredContextNumber(customer.id, 'OneID')),
    name: requiredContextText(customer.name, '客户姓名'),
    owner: customer.owner_staff_id == null ? '未分配' : String(requiredContextNumber(customer.owner_staff_id, '负责人 staff_id')),
    stageId: optionalContextNumber(customer.stage_id),
    channelId: optionalContextNumber(customer.channel_id),
    addedAt: optionalContextText(customer.added_at),
    lastInteractAt: optionalContextText(customer.last_interact_at),
  };
  const tags = list(source, 'tags').map((item) => ({ name: requiredContextText(obj(item).name, '标签名称') }));
  const timeline: Customer360TimelineEntry[] = list(source, 'timeline').map((item) => {
    const entry = obj(item);
    return { id: requiredContextNumber(entry.id, '时间线事件 ID'), eventType: requiredContextText(entry.event_type, '时间线事件类型'), occurredAt: requiredContextText(entry.occurred_at, '时间线事件时间') };
  });
  const chatSource = obj(source.chat);
  const chatItems: Customer360ChatEntry[] = list(chatSource, 'items').map((item) => {
    const entry = obj(item);
    const chatType = entry.chat_type === 'private' || entry.chat_type === 'group' ? entry.chat_type : null;
    if (!chatType) throw new Error('客户安全上下文包含未知聊天类型');
    return { chatType, messageType: requiredContextText(entry.message_type, '聊天消息类型'), sentAt: requiredContextText(entry.sent_at, '聊天时间') };
  });
  const total = Number(chatSource.total);
  if (!Number.isSafeInteger(total) || total < 0) throw new Error('客户安全上下文缺少有效聊天总数');
  return {
    profile,
    tags,
    timeline,
    timelineNextCursor: typeof source.timeline_next_cursor === 'string' ? source.timeline_next_cursor : null,
    chat: { localArchiveAvailable: chatSource.local_archive_available === true, items: chatItems, total },
    nonAtomicSnapshot: source.non_atomic_snapshot === true,
    realExternalCallExecuted: source.real_external_call_executed === true,
  };
}
export function customerSurveyPageDto(value: unknown, expectedCustomerId: number): Customer360SurveyProjection {
  const source = obj(value);
  if (requiredContextNumber(source.customer_id, '问卷 OneID') !== expectedCustomerId) throw new Error('客户安全问卷投影 OneID 不匹配');
  if (source.identity_values_included !== false || source.free_text_included !== false || source.real_external_call_executed !== false) throw new Error('客户安全问卷投影违反禁止字段或外部调用约束');
  if (source.non_atomic_snapshot !== true || typeof source.scan_truncated !== 'boolean' || typeof source.result_truncated !== 'boolean') throw new Error('客户安全问卷投影缺少完整性边界');
  const items = list(source, 'items').map((item) => {
    const submission = obj(item);
    const score = Number(submission.score);
    if (!Number.isFinite(score)) throw new Error('客户安全问卷投影缺少有效分数');
    const choices = list(submission, 'choice_answers').map((choice) => {
      const answer = obj(choice);
      const questionType: 'single_choice' | 'multi_choice' | null = answer.question_type === 'single_choice' || answer.question_type === 'multi_choice' ? answer.question_type : null;
      if (!questionType) throw new Error('客户安全问卷投影包含未知题型');
      const optionIds = list(answer, 'option_ids').map((id) => requiredContextNumber(id, '选项 ID'));
      const sortOrder = Number(answer.sort_order);
      if (!Number.isSafeInteger(sortOrder) || sortOrder < 0) throw new Error('客户安全问卷投影缺少有效题目顺序');
      return { questionId: requiredContextNumber(answer.question_id, '题目 ID'), questionType, sortOrder, optionIds };
    });
    return { submissionId: requiredContextNumber(submission.submission_id, '提交 ID'), questionnaireId: requiredContextNumber(submission.questionnaire_id, '问卷 ID'), submittedAt: requiredContextText(submission.submitted_at, '提交时间'), score, choices };
  });
  return { items, scanTruncated: source.scan_truncated === true, resultTruncated: source.result_truncated === true, nonAtomicSnapshot: source.non_atomic_snapshot === true };
}
export function questionnairePageDto(questionnaire: LegacyQuestionnaire): Questionnaire { return { resourceId: questionnaire.id, publicPath: questionnaire.public_path, name: questionnaire.title, assess: questionnaire.assessment_enabled, off: questionnaire.is_disabled, action: questionnaire.status, created: questionnaire.created_at, count: String(questionnaire.submission_count), internalName: questionnaire.name, title: questionnaire.title, description: questionnaire.description, answerDisplayMode: questionnaire.answer_display_mode, assessmentEnabled: questionnaire.assessment_enabled, assessmentConfig: questionnaire.assessment_config, slug: questionnaire.slug, questions: questionnaire.questions, scoreRules: questionnaire.score_rules, version: questionnaire.version }; }
export function channelPageDto(channel: LegacyChannelListItem | LegacyChannel): Channel {
  const x = obj(channel);
  return {
    resourceId: Number(x.id), name: text(x.channel_name), code: text(x.channel_code), type: text(x.channel_type, 'qrcode'), status: text(x.status), tone: toneFor(x.status),
    mat: list(x, 'welcome_image_library_ids', 'welcome_attachment_library_ids').join('、') || '—', tag: text(x.entry_tag_name, '—'), tagTone: 'gray', users: text(x.channel_contact_count, '0'), qr: text(x.qr_download_url, '后端未返回二维码地址'),
    channelType: x.channel_type === 'wecom_customer_acquisition' ? 'wecom_customer_acquisition' : 'qrcode', carrierType: x.carrier_type === 'link' ? 'link' : 'qrcode', sceneValue: text(x.scene_value, ''), qrUrl: text(x.qr_url, ''), ownerStaffId: text(x.owner_staff_id, ''), customerChannel: text(x.customer_channel, ''), linkUrl: text(x.link_url, ''), finalUrl: text(x.final_url, ''), shareUrl: text(x.share_url, ''), copyText: text(x.copy_text, ''),
    welcomeMessage: text(x.welcome_message, ''), welcomeImageLibraryIds: list(x, 'welcome_image_library_ids').map(Number), welcomeMiniprogramLibraryIds: list(x, 'welcome_miniprogram_library_ids').map(Number), welcomeAttachmentLibraryIds: list(x, 'welcome_attachment_library_ids').map(Number), welcomeGroupInviteLibraryIds: list(x, 'welcome_group_invite_library_ids').map(Number),
    autoAcceptFriend: x.auto_accept_friend === true, entryTagId: text(x.entry_tag_id, ''), entryTagName: text(x.entry_tag_name, ''), entryTagGroupName: text(x.entry_tag_group_name, ''), assignmentMode: x.assignment_mode === 'multi_staff' ? 'multi_staff' : 'single_owner', assignmentStrategy: x.assignment_strategy === 'cap_switch' ? 'cap_switch' : 'ratio', overflowPolicy: text(x.overflow_policy, ''), assignmentConfig: obj(x.assignment_config_json),
  };
}

export function channelEntrantDto(value: unknown): ChannelEntrant {
  const x = obj(value);
  return { customerId: Number(x.customer_id), displayName: text(x.display_name), addedAt: text(x.added_at, ''), lastInteractAt: x.last_interact_at == null ? null : String(x.last_interact_at) };
}

export async function getChannelDto(channelId: number): Promise<Channel> {
  const result = obj(await call(getLegacyChannel(channelId, apiRequestOptions())));
  return channelPageDto((result.channel || result) as LegacyChannel);
}

export async function listChannelEntrantsDto(channelId: number): Promise<ChannelEntrant[]> {
  const result = await call(listLegacyChannelEntrants(channelId, { limit: 20 }, apiRequestOptions()));
  return list(result, 'items', 'entrants').map(channelEntrantDto);
}

const channelAssetKinds: ChannelAcquisitionAssetKind[] = ['contact_way_qrcode', 'customer_acquisition_link'];
const channelAssetStates = ['accepted', 'queued', 'attempted', 'executed', 'final_failed', 'outcome_unknown', 'reconciled'] as const;

function channelAcquisitionAssigneeDto(value: unknown): ChannelAcquisitionAssignee {
  const x = obj(value);
  const staffId = text(x.wecom_userid, '');
  const name = text(x.display_name, '');
  const priority = Number(x.priority);
  if (!staffId || !name || !Number.isSafeInteger(priority) || priority < 1) throw new Error('获客渠道客服分配响应不完整');
  return {
    staffId,
    name,
    status: text(x.status, 'active'),
    priority,
    ...(x.ratio_percent == null ? {} : { ratioPercent: Number(x.ratio_percent) }),
    ...(x.max_scans_24h == null ? {} : { maxScans24h: Number(x.max_scans_24h) }),
  };
}

export function channelAcquisitionAssetDto(value: unknown): ChannelAcquisitionAsset {
  const x = obj(value);
  const effectId = text(x.effect_id, '');
  const channelId = Number(x.channel_id);
  const kind = x.kind;
  const state = x.state;
  const assetVersion = Number(x.asset_version);
  if (!effectId || !Number.isSafeInteger(channelId) || channelId < 1 || !channelAssetKinds.includes(kind as ChannelAcquisitionAssetKind) || !channelAssetStates.includes(state as typeof channelAssetStates[number]) || !Number.isSafeInteger(assetVersion) || assetVersion < 1) throw new Error('获客渠道资产回执不完整');
  const assetUrl = typeof x.asset_url === 'string' && x.asset_url.trim() ? x.asset_url.trim() : typeof x.assetUrl === 'string' && x.assetUrl.trim() ? x.assetUrl.trim() : undefined;
  return {
    effectId,
    channelId,
    kind: kind as ChannelAcquisitionAssetKind,
    assetVersion,
    state: state as ChannelAcquisitionAsset['state'],
    updatedAt: text(x.updated_at, ''),
    createdAt: text(x.created_at, ''),
    ...(assetUrl ? { assetUrl } : {}),
    ...(x.queue_receipt_id ? { receiptId: String(x.queue_receipt_id) } : x.accept_receipt_id ? { receiptId: String(x.accept_receipt_id) } : {}),
    ...(typeof x.entrant_ready === 'boolean' ? { entrantReady: x.entrant_ready } : {}),
  };
}

export function channelAcquisitionPreviewDto(value: unknown): ChannelAcquisitionPreview {
  const x = obj(value);
  if (x.local_only !== true || x.provider_execution_eligible !== false || x.real_external_call_executed !== false) throw new Error('获客渠道预览违反本地-only执行边界');
  const channelId = Number(x.channel_id);
  const channelCode = text(x.channel_code, '');
  const channelName = text(x.channel_name, '');
  const lifecycle = obj(x.lifecycle);
  if (!Number.isSafeInteger(channelId) || channelId < 1 || !channelCode || !channelName) throw new Error('获客渠道预览缺少渠道标识');
  return {
    channelId,
    channelCode,
    channelName,
    assignees: list(x, 'assignees').map(channelAcquisitionAssigneeDto),
    lifecycleState: text(lifecycle.state, ''),
    blockers: list(lifecycle, 'readiness_blockers').map(String),
    localOnly: true,
    providerExecutionEligible: false,
    realExternalCallExecuted: false,
  };
}

export async function getChannelAcquisitionPreviewDto(channelId: number): Promise<ChannelAcquisitionPreview> {
  return channelAcquisitionPreviewDto(await call(getChannelAcquisitionPreview(channelId, apiRequestOptions())));
}

export async function listChannelAcquisitionStaffDto(channelId: number): Promise<ChannelAcquisitionStaff[]> {
  const result = obj(await call(listChannelAcquisitionStaff(channelId, apiRequestOptions())));
  if (Number(result.channel_id) !== channelId || result.provider_source !== 'wecom_follow_user_list' || result.provider_read_succeeded !== true || result.real_external_call_executed !== false) throw new Error('企微客服同步响应不完整');
  return list(result, 'items').map((value) => {
    const item = obj(value);
    const staffId = text(item.wecom_userid, '');
    const name = text(item.display_name, '');
    const priority = item.priority == null ? undefined : Number(item.priority);
    const ratioPercent = item.ratio_percent == null ? undefined : Number(item.ratio_percent);
    const maxScans24h = item.max_scans_24h == null ? undefined : Number(item.max_scans_24h);
    if (!staffId || !name || item.assigned !== true && item.assigned !== false || priority != null && (!Number.isSafeInteger(priority) || priority < 1) || ratioPercent != null && (!Number.isSafeInteger(ratioPercent) || ratioPercent < 1 || ratioPercent > 100) || maxScans24h != null && (!Number.isSafeInteger(maxScans24h) || maxScans24h < 1)) throw new Error('企微客服条目不完整');
    return { staffId, name, assigned: item.assigned, ...(priority == null ? {} : { priority }), ...(ratioPercent == null ? {} : { ratioPercent }), ...(maxScans24h == null ? {} : { maxScans24h }) };
  });
}

export async function updateChannelAcquisitionAssigneesDto(channelId: number, input: ChannelAcquisitionAssignmentInput): Promise<ChannelAcquisitionAssignee[]> {
  const payload: ChannelAcquisitionAssignmentRequest = {
    assignment_mode: input.assignmentMode,
    assignment_strategy: input.assignmentStrategy,
    overflow_policy: input.overflowPolicy,
    assignees: input.assignees.map((assignee) => ({
      staff_id: assignee.staffId,
      status: assignee.status,
      priority: assignee.priority,
      ratio_percent: assignee.ratioPercent,
      max_scans_24h: assignee.maxScans24h,
    })),
  };
  if (!payload.assignees.length) throw new Error('至少配置 1 位客服');
  const result = obj(await call(updateChannelAcquisitionAssignees(channelId, payload, apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-channel-assignees-${Date.now()}` } }))));
  if (result.local_only !== true || result.provider_execution_eligible !== false || result.real_external_call_executed !== false) throw new Error('客服分配响应违反本地-only执行边界');
  return list(result, 'assignees').map(channelAcquisitionAssigneeDto);
}

export async function listChannelAcquisitionAssetsDto(channelId: number): Promise<ChannelAcquisitionAsset[]> {
  const result = await call(listChannelAcquisitionAssets(channelId, { limit: 50 }, apiRequestOptions()));
  return list(result, 'items').map(channelAcquisitionAssetDto);
}

export async function publishChannelAcquisitionAssetDto(channelId: number, kind: ChannelAcquisitionAssetKind): Promise<ChannelAcquisitionAsset> {
  const payload: ChannelAcquisitionAssetPublishRequest = { kind };
  // The generated operation is intentionally 202-only: acceptance is queued, never execution.
  return channelAcquisitionAssetDto(await call(publishChannelAcquisitionAsset(channelId, payload, apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-channel-asset-${Date.now()}` } }))));
}

export async function getChannelAcquisitionAssetDto(channelId: number, effectId: string): Promise<ChannelAcquisitionAsset> {
  const result = await call(getChannelAcquisitionAsset(channelId, effectId, apiRequestOptions()));
  const source = obj(result);
  return channelAcquisitionAssetDto(source.asset || source);
}

export const channelAcquisitionAssetReady = (asset: ChannelAcquisitionAsset | null | undefined): boolean => asset?.state === 'executed' && Boolean(asset.assetUrl?.trim());

export function buildChannelFinalUrl(linkUrl: string, customerChannel: string): string {
  const link = linkUrl.trim();
  const channel = customerChannel.trim();
  if (!link || !channel) return link;
  try {
    const url = new URL(link, typeof window === 'undefined' ? 'http://localhost' : window.location.origin);
    url.searchParams.set('customer_channel', channel);
    return url.toString();
  } catch {
    return link + (link.includes('?') ? '&' : '?') + 'customer_channel=' + encodeURIComponent(channel);
  }
}
export const orderPageDto = (value: unknown): Order => { const x = obj(value); return { time: text(x.created_at), no: text(x.merchant_order_no, text(x.order_no)), plat: text(x.provider_label, text(x.provider)), payer: text(x.payer_name), uid: text(x.payer_id), product: text(x.product_name), amount: text(x.amount), status: text(x.status_label, text(x.status)), tone: toneFor(x.status), pay: text(x.currency) }; };
export const productPageDto = (value: unknown): Product => { const source = obj(value); const x = obj(source.product || value); const lifecycle = text(x.lifecycle, text(x.status, x.enabled === true ? 'enabled' : x.enabled === false ? 'disabled' : '未投影')); return { resourceId: Number(x.id), code: text(x.product_code), name: text(x.name), price: (Number(x.price_minor || 0) / 100).toFixed(2), description: text(x.description, ''), currency: text(x.currency, 'CNY'), stockQuantity: Number(x.stock_quantity || 0), version: Number(x.version || 0), lifecycle, status: lifecycle, tone: toneFor(lifecycle), sold: text(x.sold_count, '0'), updated: text(x.updated_at) }; };
export const serviceProductPageDto = (value: unknown): SpProduct => { const source = obj(value); const x = obj(source.product || value); const lifecycle = text(x.lifecycle, text(x.status, x.enabled === true ? 'enabled' : x.enabled === false ? 'disabled' : '未投影')); return { resourceId: Number(x.service_product_id || x.id), code: text(x.product_code), name: text(x.name), price: (Number(x.price_minor || 0) / 100).toFixed(2), description: text(x.description, ''), currency: text(x.currency, 'CNY'), stockQuantity: Number(x.stock_quantity || 0), version: Number(x.version || 0), lifecycle, status: lifecycle, tone: toneFor(lifecycle), sold: text(x.member_count, '0'), updated: text(x.updated_at) }; };
export const couponPageDto = (value: unknown): Coupon => { const source = obj(value); const x = obj(source.coupon || value); const availabilityStatus = text(x.availability_status, text(x.status)); return { resourceId: Number(x.id), name: text(x.name), code: 'c-' + text(x.id), discountAmountTotal: Number(x.discount_amount_total || 0), totalIssueLimit: Number(x.total_issue_limit || 0), perUserIssueLimit: Number(x.per_user_issue_limit || 1), claimStartsAt: text(x.claim_starts_at, ''), claimEndsAt: text(x.claim_ends_at, ''), validityMode: x.validity_mode === 'fixed_range' ? 'fixed_range' : 'relative_days', useStartsAt: typeof x.use_starts_at === 'string' ? x.use_starts_at : null, useEndsAt: typeof x.use_ends_at === 'string' ? x.use_ends_at : null, relativeValidityDays: x.relative_validity_days == null ? null : Number(x.relative_validity_days), instructions: text(x.instructions, ''), targetRefs: list(x, 'target_refs').map(String), version: Number(x.version || 0), off: `¥${(Number(x.discount_amount_total || 0) / 100).toFixed(2)}`, scope: list(x, 'target_refs').join('、') || '—', window: `${text(x.claim_starts_at)} 至 ${text(x.claim_ends_at)}`, issue: `${text(x.issued_count, '0')} / ${text(x.total_issue_limit, '0')}`, availabilityStatus, status: text(x.status), tone: toneFor(availabilityStatus) }; };
export const imagePageDto = (value: unknown): ImageItem => { const x = obj(value); return { resourceId: text(x.id, ''), name: text(x.name, text(x.file_name, text(x.filename))), size: text(x.file_size, text(x.size)), tag: text(x.category), tone: toneFor(x.status), bg: '#EFF4FF', desc: text(x.description, ''), tags: Array.isArray(x.tags) ? x.tags.map(String).join(', ') : text(x.tags, ''), enabled: x.enabled !== false, uploadedAt: text(x.created_at) }; };
export const miniProgramPageDto = (value: unknown): MpItem => { const x = obj(value); return { resourceId: Number(x.id), name: text(x.name), appid: text(x.appid, text(x.app_id)), pagepath: text(x.pagepath, text(x.page_path)), cardTitle: text(x.title), thumbStatus: text(x.thumbnail_status), thumbOk: x.thumbnail_status === 'ready', enabled: x.enabled !== false, bg: '#EFF4FF' }; };
export const attachmentPageDto = (value: unknown): AttachItem => { const x = obj(value); return { resourceId: text(x.id, ''), name: text(x.name, text(x.file_name, text(x.filename))), type: text(x.mime_type, text(x.content_type)), size: text(x.file_size, text(x.size)), tags: Array.isArray(x.tags) ? x.tags.map(String).join(', ') : text(x.tags, ''), uploadedAt: text(x.created_at), enabled: x.enabled !== false }; };
export const tagGroupPageDto = (value: unknown): TagGroup => { const x = obj(value); return { id: Number(x.id), name: text(x.name) }; };
export const tagPageDto = (value: unknown): WecomTag => { const x = obj(value); return { id: Number(x.id), groupId: Number(x.group_id), name: text(x.name), users: Number(x.user_count || 0), syncedAt: text(x.updated_at) }; };
export const radarPageDto = (link: ApiRadarLink): AdminDb['radarLinks'][number] => ({ id: link.link_id, title: link.title, target_type: link.attachment_id ? 'pdf' : link.cover_image_id ? 'image' : 'link', original_url: link.destination_url, file_name_snapshot: '', media_item_id: String(link.attachment_id || link.cover_image_id || ''), enabled: link.status === 'enabled', auth_required: true, staff_id: String(link.created_by), code: link.public_code, total_landings: 0, authorized_users: 0, view_count: 0, last_viewed_at: link.updated_at });
export const questionnaireOpsPageDto = (value: unknown): QuestionnaireOps => { const source = obj(value); const completion = obj(source.completion); const external = obj(source.external_push); const enabled = external.enabled === true; return { completionNavigationTargetId: text(completion.navigation_target_id, ''), completionChannelId: completion.channel_id == null ? '' : String(completion.channel_id), externalPushConfigurationReference: text(external.configuration_reference, ''), localOnly: source.local_only !== false, postEnabled: Boolean(completion.navigation_target_id || completion.channel_id), postType: completion.channel_id == null ? 'redirect' : 'channel_qr', channelId: completion.channel_id == null ? '' : String(completion.channel_id), qrTitle: '', qrSubtitle: '', redirectType: 'h5', redirectUrl: '', pushEnabled: enabled, webhookUrl: '', subscribeType: '', expiresAt: '', serviceCycle: '', frequency: '', remark: '', customParams: [] }; };
export const hxcSenderPageDto = (item: LegacyHXCSenderConfig): AdminDb['rows']['agents'][number] => ({ senderId: item.id, priority: item.priority, isActive: item.is_active, name: item.display_name || item.sender_userid, code: item.sender_userid, type: 'HXC 本地发送人', material: `优先级 ${item.priority}`, status: item.is_active ? '启用中' : '已停用', tone: item.is_active ? 'ok' : 'gray' });
export const audienceGroupPageDto = (value: unknown): AdminDb['audienceGroups'][number] => ({ id: Number(obj(value).group_id), name: text(obj(value).name) });
export const audiencePackagePageDto = (value: unknown): AdminDb['audiencePackages'][number] => { const x = obj(value); const packageVersion = Number(x.version || 0); return { id: Number(x.package_id), name: text(x.name), groupId: Number(x.group_id || 0), count: Number(x.member_count || 0), lastRefresh: text(x.refreshed_at), refreshMode: text(x.refresh_mode), running: x.lifecycle === 'active', version: 'v' + text(x.version), packageVersion, refreshCron: typeof x.refresh_cron === 'string' ? x.refresh_cron : null, definition: x.definition ? JSON.stringify(x.definition, null, 2) : '', incremental: x.refresh_mode === 'scheduled' ? 'scheduled' : 'manual', daily: text(x.refresh_cron, ''), boundAutomation: '' }; };
export const groupOpsPlanDto = (value: unknown): AdminDb['groupOpsPlans'][number] => { const x = obj(value); const queueCount = Number(x.queue_count || 0); if (!Number.isSafeInteger(queueCount) || queueCount < 0) throw new Error('Group Ops queue_count 无效'); return { id: text(x.plan_id), name: text(x.name), status: ['active', 'paused', 'archived'].includes(text(x.status)) ? text(x.status) as 'active' | 'paused' | 'archived' : 'draft', revision: Number(x.revision), queueCount, updatedAt: text(x.updated_at) }; };
const groupOpsMaterialPlanDto = (value: unknown): GroupOpsMaterialPlan => {
  const refs = obj(value).references;
  if (!Array.isArray(refs)) throw new Error('Group Ops material_plan 缺少 references');
  return { references: refs.map((item) => {
    const ref = obj(item); const id = Number(ref.id); const kind = text(ref.kind, '') as GroupOpsMaterialKind;
    if (!['image', 'miniprogram', 'attachment', 'group_invite'].includes(kind) || !Number.isSafeInteger(id) || id < 1) throw new Error('Group Ops material_plan 包含无效素材引用');
    return { kind, id };
  }) };
};
export const groupOpsDetailDto = (value: unknown, preview?: unknown): NonNullable<AdminDb['groupOpsDetail']> => { const x = obj(value); const validation = obj(preview); return { plan: groupOpsPlanDto(x.plan), staffIds: list(x, 'members').map((item) => Number(obj(item).staff_id)), assets: list(x, 'group_assets').map((item) => ({ id: text(obj(item).group_asset_id), reference: text(obj(item).asset_reference) })), nodes: list(x, 'nodes').map((item) => ({ id: text(obj(item).node_id), position: Number(obj(item).position), kind: obj(item).kind === 'delay' ? 'delay' : 'message', messageText: text(obj(item).message_text, ''), delayMinutes: obj(item).delay_minutes == null ? undefined : Number(obj(item).delay_minutes), materialReference: text(obj(item).material_reference, ''), materialPlan: groupOpsMaterialPlanDto(obj(item).material_plan) })), webhookReference: text(obj(x.webhook_descriptor).reference, ''), previewLines: list(validation, 'preview_lines').map(String), previewIssues: list(validation, 'issue_codes').map(String) }; };
const groupOpsPreviewDto = (planId: string, value: unknown): AdminDb['rows']['orderKv'] => {
  const source = obj(value);
  if (text(source.plan_id) !== planId || source.real_external_call_executed !== false || source.provider_accepted !== false || source.delivery_proven !== false) throw new Error('Group Ops run-due 预览越过本地读取边界');
  const due = Number(source.due_execution_count);
  const revision = Number(source.snapshot_revision);
  if (!Number.isSafeInteger(due) || due < 0 || !Number.isSafeInteger(revision) || revision < 1 || typeof source.provider_execution_eligible !== 'boolean') throw new Error('Group Ops run-due 预览响应不完整');
  return [
    { k: 'run-due 预览 · 快照 revision', v: String(revision), mono: false },
    { k: '到期执行候选', v: String(due), mono: false },
    { k: '下一次到期', v: text(source.next_due_at, '—'), mono: false },
    { k: '阻断原因', v: list(source, 'blockers').map(String).join('、') || '无', mono: false },
    { k: '本地执行资格', v: source.provider_execution_eligible ? 'eligible（仅预览，未调用 Provider）' : 'not eligible', mono: false },
  ];
};
const groupOpsWebhookDescriptorDto = (value: unknown): AdminDb['rows']['orderKv'] => {
  const source = obj(value);
  if (source.real_external_call_executed !== false || typeof source.provider_execution_eligible !== 'boolean') throw new Error('Group Ops webhook 描述符越过本地读取边界');
  return [
    { k: 'Webhook 描述符', v: source.configured === true ? text(source.description, 'local opaque reference only') : 'not configured', mono: false },
    { k: 'Webhook opaque reference', v: typeof source.reference === 'string' ? source.reference : '—', mono: true },
  ];
};
const groupOpsExecutionRows = (planId: string, value: unknown): AdminDb['rows']['orderEvents'] => {
  const source = obj(value);
  const stateHint: Record<string, string> = {
    accepted: '已接受内部执行；不等于 Provider 调用或送达',
    provider_accepted: 'Provider 已受理；仍不等于送达',
    delivery_proven: '已由已验证 Provider 回执证明送达',
    outcome_unknown: '结果未知，需人工对账；禁止自动重试',
    reconciled: '已基于证据完成本地对账',
    final_failed: '最终失败',
  };
  return list(source, 'items').map((item) => {
    const execution = obj(item);
    const state = text(execution.state);
    if (text(execution.plan_id) !== planId || !stateHint[state]) throw new Error('Group Ops execution 返回范围或状态不匹配');
    if (execution.delivery_proven === true && (state !== 'delivery_proven' || execution.provider_receipt_present !== true)) throw new Error('Group Ops execution 缺少可验证送达回执');
    return { time: text(execution.updated_at), ev: `execution ${text(execution.execution_id)} · attempts ${text(execution.attempt_count, '0')}`, st: stateHint[state], tone: toneFor(state) };
  });
};
export const configCategoryPageDto = (value: unknown): ConfigCategory => { const x = obj(value); return { key: text(x.key), label: text(x.key), group: '本地安全配置', on: x.enabled === true, toggleable: true, checkSupported: true, blocks: [] }; };
export const appSettingsPageDto = (value: unknown): ConfigCategory => { const source = obj(value); const config = obj(source.config); return { key: 'app-settings', label: '应用设置', group: '本地安全配置', on: true, toggleable: false, checkSupported: false, actionToken: text(source.admin_action_token, ''), blocks: [{ title: '非敏感应用设置', fields: list(config, 'rows').map((entry) => { const row = obj(entry); const masked = row.mode === 'masked'; return { key: text(row.key), label: text(row.label, text(row.key)), kind: masked ? 'secret' as const : row.input_type === 'number' ? 'number' as const : 'text' as const, value: masked ? '' : text(row.value, ''), configured: masked ? row.configured === true : undefined }; }) }] }; };
export const readOnlyConfigPageDto = (key: 'push-capabilities' | 'releases', value: unknown): ConfigCategory => { const source = obj(value); const rows = key === 'releases' ? list(source, 'releases').map((item) => { const release = obj(item); return { key: `release.${text(release.id)}`, label: `Release ${text(release.id)} · ${text(release.state)}`, value: text(release.checksum), kind: 'readonly' as const }; }) : Object.entries(obj(source.capabilities)).map(([name, setting]) => ({ key: name, label: name, value: typeof setting === 'object' ? JSON.stringify(setting) : text(setting), kind: 'readonly' as const })); return { key, label: key === 'releases' ? '配置发布记录' : 'Push 能力', group: '本地安全配置', on: true, toggleable: false, checkSupported: false, blocks: [{ title: key === 'releases' ? '本地发布记录' : '当前能力安全投影', fields: rows }] }; };
export const ownerReassignmentPreviewDto = (preview: ApiOwnerReassignmentPreview): OwnerReassignmentPreview => ({
  id: preview.id,
  hash: preview.hash,
  rows: preview.rows.map((row) => ({ customerId: row.customer_id, expectedOwnerStaffId: row.expected_owner_staff_id, expectedUpdatedAt: row.expected_updated_at, targetOwnerStaffId: row.target_owner_staff_id })),
  issues: preview.issues.map((issue) => ({ line: issue.line, code: issue.code })),
  expiresAt: preview.expires_at,
  executed: preview.executed,
  result: (preview.result || []).map((row) => ({ customerId: row.customer_id, previousOwnerStaffId: row.previous_owner_staff_id, targetOwnerStaffId: row.target_owner_staff_id, updatedAt: row.updated_at })),
});

export async function downloadOwnerReassignmentTemplateDto(): Promise<Blob> {
  return (await request(getDownloadContactOwnerReassignmentTemplateUrl())).blob();
}

export async function createOwnerReassignmentPreviewDto(csv: string): Promise<OwnerReassignmentPreview> {
  if (!csv.trim()) throw new Error('请选择非空 CSV 文件');
  if (new Blob([csv]).size > 1024 * 1024) throw new Error('CSV 文件不能超过 1 MiB');
  const response = await request(getCreateContactOwnerReassignmentPreviewUrl(), { method: 'POST', headers: { 'Content-Type': 'text/csv' }, body: csv });
  return ownerReassignmentPreviewDto(await response.json() as ApiOwnerReassignmentPreview);
}

export async function getOwnerReassignmentPreviewDto(previewId: string): Promise<OwnerReassignmentPreview> {
  return ownerReassignmentPreviewDto(await call(getContactOwnerReassignmentPreview(previewId, apiRequestOptions())) as ApiOwnerReassignmentPreview);
}

export async function executeOwnerReassignmentPreviewDto(preview: OwnerReassignmentPreview): Promise<OwnerReassignmentPreview> {
  const options = apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-${Date.now()}` } });
  const result = await call(executeContactOwnerReassignmentPreview(preview.id, { preview_hash: preview.hash, confirmation: 'CONFIRM OWNER REASSIGNMENT' }, options));
  return ownerReassignmentPreviewDto(result as ApiOwnerReassignmentPreview);
}

export async function downloadOwnerReassignmentReportDto(previewId: string, kind: 'errors' | 'results'): Promise<Blob> {
  const url = kind === 'errors' ? getDownloadContactOwnerReassignmentErrorsUrl(previewId) : getDownloadContactOwnerReassignmentResultsUrl(previewId);
  return (await request(url)).blob();
}
export async function setRadarEnabled(linkId: number, enabled: boolean): Promise<void> { const current = obj(await call(getRadarLink(linkId, apiRequestOptions()))).link as ApiRadarLink; const request = { expected_version: current.version }; await call(enabled ? enableRadarLink(linkId, request, apiRequestOptions()) : disableRadarLink(linkId, request, apiRequestOptions())); }
export async function readRadarEvents(linkId: number): Promise<AdminDb['radarEvents']> { const page = await call(listRadarLinkEvents(linkId, undefined, apiRequestOptions())); return list(page, 'items').map((item) => ({ unionid_masked: text(obj(item).unionid_masked), external_userid: text(obj(item).external_userid), created_at: text(obj(item).occurred_at, text(obj(item).created_at)) })); }
export async function readRadarSharePath(linkId: number): Promise<string> { const projection = obj(await call(getRadarLinkShareProjection(linkId, apiRequestOptions()))); if (projection.available !== true || typeof projection.share_path !== 'string') throw new Error('后端尚未提供可用的 Radar 公开分享路径'); return projection.share_path; }
export async function readCouponSharePath(couponId: number): Promise<string> { const projection = obj(await call(getLegacyCouponShare(couponId, apiRequestOptions()))); if (typeof projection.url !== 'string') throw new Error('后端尚未提供可用的优惠券分享路径'); return projection.url; }
export async function updateCustomerDto(customerId: number, input: { name?: string; stageId?: number | null }): Promise<Customer> {
  const opt = apiRequestOptions(); let customer: ApiCustomer | undefined;
  if (input.name != null) customer = await call(updateCustomer(customerId, { name: input.name }, opt)) as ApiCustomer;
  if (input.stageId !== undefined) customer = await call(setCustomerStage(customerId, { stage_id: input.stageId }, opt)) as ApiCustomer;
  if (!customer) customer = await call(getCustomer(customerId, opt)) as ApiCustomer;
  return customerPageDto(customer);
}
export async function setCustomerTagDto(customerId: number, tagId: number, applied: boolean): Promise<void> { await call(applied ? addCustomerTag(customerId, tagId, apiRequestOptions()) : removeCustomerTag(customerId, tagId, apiRequestOptions())); }

export type ProductWriteInput = { id?: number; code: string; name: string; description: string; price: string; currency: string; stockQuantity: number };
const priceMinor = (value: string): number => { if (!/^\d+(\.\d{1,2})?$/.test(value.trim())) throw new Error('价格必须是最多两位小数的非负金额'); return Math.round(Number(value) * 100); };
export async function saveProductDto(input: ProductWriteInput): Promise<Product> {
  const opt = apiRequestOptions();
  if (input.id == null) return productPageDto(await call(createProduct({ product_code: input.code, name: input.name, description: input.description, price_minor: priceMinor(input.price), currency: input.currency, stock_quantity: input.stockQuantity, images: [] }, opt)));
  const current = obj(await call(getProduct(input.id, opt)));
  return productPageDto(await call(updateProduct(input.id, { expected_version: Number(current.version), name: input.name, description: input.description, price_minor: priceMinor(input.price), currency: input.currency, stock_quantity: input.stockQuantity }, opt)));
}
export async function setProductEnabledDto(productId: number, enabled: boolean): Promise<Product> { const current = obj(await call(getProduct(productId, apiRequestOptions()))); return productPageDto(await call(enabled ? enableLegacyWechatPayProduct(productId, { expected_version: Number(current.version) }, apiRequestOptions()) : disableLegacyWechatPayProduct(productId, { expected_version: Number(current.version) }, apiRequestOptions()))); }
export async function copyProductDto(productId: number): Promise<Product> { const current = obj(await call(getProduct(productId, apiRequestOptions()))); return productPageDto(await call(copyLegacyWechatPayProduct(productId, { expected_version: Number(current.version) }, apiRequestOptions()))); }
export async function deleteProductDto(productId: number): Promise<void> { const current = obj(await call(getProduct(productId, apiRequestOptions()))); await call(deleteLegacyWechatPayProduct(productId, { expected_version: Number(current.version) }, apiRequestOptions())); }

export async function saveServiceProductDto(input: ProductWriteInput): Promise<SpProduct> {
  const opt = apiRequestOptions();
  if (input.id == null) { const result = obj(await call(createServicePeriodProduct({ product_code: input.code, name: input.name, description: input.description, price_minor: priceMinor(input.price), currency: input.currency, stock_quantity: input.stockQuantity }, opt))); return serviceProductPageDto(result.product || result); }
  const currentResult = obj(await call(getServicePeriodProduct(input.id, opt))); const current = obj(currentResult.product || currentResult);
  const result = obj(await call(updateServicePeriodProduct(input.id, { expected_version: Number(current.version), name: input.name, description: input.description, price_minor: priceMinor(input.price), currency: input.currency, stock_quantity: input.stockQuantity }, opt))); return serviceProductPageDto(result.product || result);
}
async function serviceProductVersion(productId: number): Promise<number> { const result = obj(await call(getServicePeriodProduct(productId, apiRequestOptions()))); return Number(obj(result.product || result).version); }
export async function setServiceProductEnabledDto(productId: number, enabled: boolean): Promise<SpProduct> { const version = await serviceProductVersion(productId); const result = obj(await call(enabled ? enableServicePeriodProduct(productId, { expected_version: version }, apiRequestOptions()) : disableServicePeriodProduct(productId, { expected_version: version }, apiRequestOptions()))); return serviceProductPageDto(result.product || result); }
export async function copyServiceProductDto(productId: number): Promise<SpProduct> { const result = obj(await call(copyServicePeriodProduct(productId, { expected_version: await serviceProductVersion(productId) }, apiRequestOptions()))); return serviceProductPageDto(result.product || result); }
export async function archiveServiceProductDto(productId: number): Promise<void> { await call(archiveServicePeriodProduct(productId, { expected_version: await serviceProductVersion(productId) }, apiRequestOptions())); }

export type CouponWriteInput = { id?: number; name: string; discount: string; totalIssueLimit: number; perUserIssueLimit: number; claimStartsAt: string; claimEndsAt: string; validityMode: 'fixed_range' | 'relative_days'; useStartsAt?: string; useEndsAt?: string; relativeValidityDays?: number; instructions: string; targetRefs: string[] };
export type CouponProductOptionPage = { items: Array<{ targetRef: string; name: string; priceMinor: number; currency: string }>; total: number; limit: number; offset: number };
export type CouponClaimPage = { items: Array<{ claimRef: string; status: string; claimedAt: string }>; total: number; limit: number; offset: number };
export type MemberGridState = 'active' | 'expired' | 'removed' | 'all';
export type MemberGridSource = 'manual' | 'paid_order';
export type MemberGridSourceFilter = MemberGridSource | '';
export type ServicePeriodMemberGridRow = { memberRef: string; serviceProductId: number; customerId: number; state: MemberGridState; source: MemberGridSource; startsAt: string; expiresAt: string | null; expiredAt: string | null; removedAt: string | null; version: number; updatedAt: string; displayName: string };
export type ServicePeriodMemberGridPage = { rows: ServicePeriodMemberGridRow[]; limit: number; nextCursor: string; hasMore: boolean };
export type ServicePeriodMemberDetail = ServicePeriodMemberGridRow & { remark: string | null; alliance: string | null; createdAt: string };
export type ServicePeriodMemberGridCollaborator = { collaboratorId: number; serviceProductId: number; staffId: number; permission: 'view' | 'edit'; version: number; invitedBy: number; createdAt: string; updatedAt: string };
export type ServicePeriodMemberGridMeta = { product: SpProduct; columns: Array<{ key: string; label: string; type: string; nullable: boolean }>; views: Array<{ id: string; name: string; readOnly: boolean }>; collaborators: number; collaboratorRows: ServicePeriodMemberGridCollaborator[] };
const requiredPositive = (value: unknown, field: string): number => { const number = Number(value); if (!Number.isSafeInteger(number) || number < 1) throw new Error(`响应缺少有效 ${field}`); return number; };
const requiredNonNegative = (value: unknown, field: string): number => { const number = Number(value); if (!Number.isSafeInteger(number) || number < 0) throw new Error(`响应缺少有效 ${field}`); return number; };
const requiredString = (value: unknown, field: string): string => { if (typeof value !== 'string' || !value) throw new Error(`响应缺少 ${field}`); return value; };
const nullableString = (value: unknown, field: string): string | null => value == null ? null : requiredString(value, field);
const memberState = (value: unknown, field: string): MemberGridState => { if (value === 'active' || value === 'expired' || value === 'removed') return value; throw new Error(`响应包含未知 ${field}`); };
const memberSource = (value: unknown, field: string): MemberGridSource => { if (value === 'manual' || value === 'paid_order') return value; throw new Error(`响应包含未知 ${field}`); };
const memberRef = (value: unknown): string => { const ref = requiredString(value, 'member_ref'); if (!/^spm_[A-Za-z0-9_-]{22}$/.test(ref)) throw new Error('响应包含无效 member_ref'); return ref; };
const memberRowDto = (value: unknown, productId: number): ServicePeriodMemberGridRow => {
  const source = obj(value);
  if (requiredPositive(source.service_product_id, 'service_product_id') !== productId) throw new Error('Member Grid 行商品范围不匹配');
  return { memberRef: memberRef(source.member_ref), serviceProductId: productId, customerId: requiredPositive(source.customer_id, 'customer_id'), state: memberState(source.state, 'state'), source: memberSource(source.source, 'source'), startsAt: requiredString(source.starts_at, 'starts_at'), expiresAt: nullableString(source.expires_at, 'expires_at'), expiredAt: nullableString(source.expired_at, 'expired_at'), removedAt: nullableString(source.removed_at, 'removed_at'), version: requiredPositive(source.version, 'version'), updatedAt: requiredString(source.updated_at, 'updated_at'), displayName: requiredString(source.display_name, 'display_name') };
};
const collaboratorDto = (value: unknown, productId: number): ServicePeriodMemberGridCollaborator => {
  const source = obj(value);
  if (requiredPositive(source.service_product_id, 'collaborator.service_product_id') !== productId) throw new Error('Member Grid 协作者商品范围不匹配');
  if (source.permission !== 'view' && source.permission !== 'edit') throw new Error('Member Grid 协作者权限未知');
  return { collaboratorId: requiredPositive(source.collaborator_id, 'collaborator_id'), serviceProductId: productId, staffId: requiredPositive(source.staff_id, 'staff_id'), permission: source.permission, version: requiredPositive(source.version, 'collaborator.version'), invitedBy: requiredPositive(source.invited_by, 'invited_by'), createdAt: requiredString(source.created_at, 'collaborator.created_at'), updatedAt: requiredString(source.updated_at, 'collaborator.updated_at') };
};
const localCollaboratorResult = (value: unknown, productId: number): ServicePeriodMemberGridCollaborator => {
  const source = obj(value);
  if (source.edit_permission_is_local_metadata_only !== true || source.grants_central_products_permission !== false) throw new Error('Member Grid 协作者响应越过本地元数据边界');
  return collaboratorDto(source.collaborator || source, productId);
};
export async function getCouponDto(couponId: number): Promise<Coupon> { const result = obj(await call(getLegacyCoupon(couponId, apiRequestOptions()))); const coupon = couponPageDto(result.coupon || result); if (coupon.resourceId !== couponId) throw new Error('优惠券响应范围不匹配'); return coupon; }
export async function listCouponProductOptionsDto(input: { q?: string; productType?: 'all' | 'standard_product' | 'service_period'; limit?: number; offset?: number } = {}): Promise<CouponProductOptionPage> {
  const page = obj(await call(listLegacyCouponProductOptions({ q: input.q, product_type: input.productType, limit: input.limit, offset: input.offset }, apiRequestOptions())));
  if (page.ok !== true) throw new Error('优惠券商品选项响应不完整');
  return { items: list(page, 'items').map((item) => { const source = obj(item); return { targetRef: requiredString(source.target_ref, 'target_ref'), name: requiredString(source.name, 'name'), priceMinor: requiredNonNegative(source.price_minor, 'price_minor'), currency: requiredString(source.currency, 'currency') }; }), total: requiredNonNegative(page.total, 'total'), limit: requiredPositive(page.limit, 'limit'), offset: requiredNonNegative(page.offset, 'offset') };
}
export async function listCouponClaimsDto(couponId: number, input: { limit?: number; offset?: number } = {}): Promise<CouponClaimPage> {
  const page = obj(await call(listLegacyCouponClaims(couponId, { limit: input.limit, offset: input.offset }, apiRequestOptions())));
  if (page.ok !== true) throw new Error('优惠券领取记录响应不完整');
  return { items: list(page, 'items').map((item) => { const source = obj(item); return { claimRef: requiredString(source.claim_ref, 'claim_ref'), status: requiredString(source.status, 'status'), claimedAt: requiredString(source.claimed_at, 'claimed_at') }; }), total: requiredNonNegative(page.total, 'total'), limit: requiredPositive(page.limit, 'limit'), offset: requiredNonNegative(page.offset, 'offset') };
}
export async function getServicePeriodMemberGridMetaDto(productId: number): Promise<ServicePeriodMemberGridMeta> {
  const options = apiRequestOptions();
  const [productResult, accessResult, schemaResult, viewsResult, shareResult] = await Promise.all([
    call(getServicePeriodProduct(productId, options)),
    call(getServicePeriodMemberGridAccess(productId, options)),
    call(getServicePeriodMemberGridSchema(productId, options)),
    call(listServicePeriodMemberViews(productId, options)),
    call(getServicePeriodMemberGridShareSettings(productId, options)),
  ]);
  const productSource = obj(productResult); const access = obj(accessResult); const schema = obj(schemaResult); const views = obj(viewsResult); const share = obj(shareResult);
  if (requiredPositive(access.product_id, 'access.product_id') !== productId || requiredPositive(schema.service_product_id, 'schema.service_product_id') !== productId || requiredPositive(views.product_id, 'views.product_id') !== productId || requiredPositive(share.service_product_id, 'share.service_product_id') !== productId) throw new Error('Member Grid 响应范围不匹配');
  if (access.can_view !== true || access.can_query !== true) throw new Error('当前账号无 Member Grid 读取权限');
  if (access.can_manage_views !== false || access.can_share !== false || share.external_share_supported !== false || share.external_share_enabled !== false || share.real_external_call_executed !== false || share.collaborator_edit_is_local_metadata_only !== true || share.collaborator_edit_grants_central_permission !== false) throw new Error('Member Grid 响应越过本地只读边界');
  const columns = list(schema, 'columns').map((item) => { const column = obj(item); return { key: requiredString(column.key, 'column.key'), label: requiredString(column.label, 'column.label'), type: requiredString(column.type, 'column.type'), nullable: column.nullable === true }; });
  const builtInViews = list(views, 'views').map((item) => { const view = obj(item); if (view.source !== 'built_in' || view.read_only !== true) throw new Error('Member Grid 视图不是受限内置视图'); return { id: requiredString(view.id, 'view.id'), name: requiredString(view.name, 'view.name'), readOnly: true }; });
  if (columns.length !== 12 || builtInViews.length < 1) throw new Error('Member Grid 闭合 schema 或内置视图响应不完整');
  const collaboratorRows = list(share, 'collaborators').map((item) => collaboratorDto(item, productId));
  return { product: serviceProductPageDto(productSource.product || productSource), columns, views: builtInViews, collaborators: collaboratorRows.length, collaboratorRows };
}
export async function queryServicePeriodMemberGridDto(productId: number, input: { state?: MemberGridState; source?: MemberGridSourceFilter; limit?: number; cursor?: string } = {}): Promise<ServicePeriodMemberGridPage> {
  if (!Number.isSafeInteger(productId) || productId < 1) throw new Error('Member Grid 商品 ID 无效');
  const state = input.state || 'all'; const source = input.source || ''; const limit = input.limit ?? 50;
  if (!['active', 'expired', 'removed', 'all'].includes(state) || !['', 'manual', 'paid_order'].includes(source) || !Number.isSafeInteger(limit) || limit < 1 || limit > 50) throw new Error('Member Grid 查询条件无效');
  const page = obj(await call(queryServicePeriodMemberGrid(productId, { state, source: source || undefined, limit, cursor: input.cursor || '' }, apiRequestOptions())));
  if (!Array.isArray(page.rows) || typeof page.next_cursor !== 'string' || (page.has_more !== true && page.has_more !== false)) throw new Error('Member Grid 查询响应不完整');
  const rows = page.rows.map((item) => memberRowDto(item, productId));
  const pageLimit = requiredPositive(page.limit, 'limit'); if (pageLimit > 50 || pageLimit !== limit) throw new Error('Member Grid 查询页大小不匹配');
  const nextCursor = page.next_cursor; if (page.has_more && !nextCursor) throw new Error('Member Grid 下一页缺少 cursor');
  return { rows, limit: pageLimit, nextCursor, hasMore: page.has_more };
}
export async function getServicePeriodMemberDto(productId: number, ref: string): Promise<ServicePeriodMemberDetail> {
  const requestedRef = memberRef(ref);
  const result = obj(await call(getServicePeriodMember(productId, requestedRef, apiRequestOptions()))); const member = obj(result.member || result); const row = memberRowDto({ ...member, display_name: member.display_name || member.name || member.member_ref }, productId);
  if (row.memberRef !== requestedRef) throw new Error('Member Grid 成员响应范围不匹配');
  return { ...row, remark: nullableString(member.remark, 'remark'), alliance: nullableString(member.alliance, 'alliance'), createdAt: requiredString(member.created_at, 'created_at') };
}
export type ServicePeriodMemberFieldsInput = { expectedVersion: number; remark?: string | null; alliance?: string | null };
const localField = (value: string | null | undefined, field: string, max: number): string | null | undefined => { if (value === undefined) return undefined; if (value === null) return null; const clean = value.trim(); if (!clean) return null; if (clean.length > max) throw new Error(`${field} 不能超过 ${max} 个字符`); return clean; };
export async function updateServicePeriodMemberFieldsDto(productId: number, ref: string, input: ServicePeriodMemberFieldsInput): Promise<ServicePeriodMemberDetail> {
  const requestedRef = memberRef(ref);
  if (!Number.isSafeInteger(input.expectedVersion) || input.expectedVersion < 1) throw new Error('成员版本无效');
  const remark = localField(input.remark, '备注', 500); const alliance = localField(input.alliance, '联盟', 120); const body: { expected_version: number; remark?: string | null; alliance?: string | null } = { expected_version: input.expectedVersion };
  if (remark !== undefined) body.remark = remark; if (alliance !== undefined) body.alliance = alliance;
  const result = obj(await call(updateServicePeriodMemberFields(productId, requestedRef, body, apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-member-${Date.now()}` } })))); const member = obj(result.member || result); const row = memberRowDto({ ...member, display_name: member.display_name || member.name || member.member_ref }, productId);
  if (row.memberRef !== requestedRef) throw new Error('Member Grid 成员响应范围不匹配');
  return { ...row, remark: nullableString(member.remark, 'remark'), alliance: nullableString(member.alliance, 'alliance'), createdAt: requiredString(member.created_at, 'created_at') };
}
export async function createServicePeriodMemberGridCollaboratorDto(productId: number, input: { staffId: number; permission: 'view' | 'edit' }): Promise<ServicePeriodMemberGridCollaborator> {
  if (!Number.isSafeInteger(input.staffId) || input.staffId < 1) throw new Error('协作者 staff_id 必须为正整数');
  const result = await call(createServicePeriodMemberGridCollaborator(productId, { expected_version: 0, staff_id: input.staffId, permission: input.permission }, apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-member-collab-${Date.now()}` } })));
  return localCollaboratorResult(result, productId);
}
export async function updateServicePeriodMemberGridCollaboratorDto(productId: number, collaboratorId: number, input: { expectedVersion: number; permission: 'view' | 'edit' }): Promise<ServicePeriodMemberGridCollaborator> {
  if (!Number.isSafeInteger(collaboratorId) || collaboratorId < 1 || !Number.isSafeInteger(input.expectedVersion) || input.expectedVersion < 1) throw new Error('协作者版本或 ID 无效');
  const result = await call(updateServicePeriodMemberGridCollaborator(productId, collaboratorId, { expected_version: input.expectedVersion, permission: input.permission }, apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-member-collab-${Date.now()}` } })));
  return localCollaboratorResult(result, productId);
}
export async function deleteServicePeriodMemberGridCollaboratorDto(productId: number, collaboratorId: number, expectedVersion: number): Promise<ServicePeriodMemberGridCollaborator> {
  if (!Number.isSafeInteger(collaboratorId) || collaboratorId < 1 || !Number.isSafeInteger(expectedVersion) || expectedVersion < 1) throw new Error('协作者版本或 ID 无效');
  const result = obj(await call(deleteServicePeriodMemberGridCollaborator(productId, collaboratorId, { expected_version: expectedVersion }, apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-member-collab-${Date.now()}` } }))));
  if (result.deleted !== true) throw new Error('协作者删除响应不完整');
  return localCollaboratorResult(result, productId);
}
const couponRequest = (input: CouponWriteInput): CouponUpsertRequest => ({
  name: input.name,
  discount_amount_total: priceMinor(input.discount),
  total_issue_limit: input.totalIssueLimit,
  per_user_issue_limit: input.perUserIssueLimit,
  claim_starts_at: new Date(input.claimStartsAt).toISOString(),
  claim_ends_at: new Date(input.claimEndsAt).toISOString(),
  validity_mode: input.validityMode,
  use_starts_at: input.validityMode === 'fixed_range' && input.useStartsAt ? new Date(input.useStartsAt).toISOString() : null,
  use_ends_at: input.validityMode === 'fixed_range' && input.useEndsAt ? new Date(input.useEndsAt).toISOString() : null,
  relative_validity_days: input.validityMode === 'relative_days' ? input.relativeValidityDays || null : null,
  instructions: input.instructions,
  target_refs: input.targetRefs,
});
export async function saveCouponDto(input: CouponWriteInput, publish: boolean): Promise<Coupon> { const opt = apiRequestOptions(); const saved = obj(await call(input.id == null ? createLegacyCoupon(couponRequest(input), opt) : updateLegacyCoupon(input.id, couponRequest(input), opt))); const coupon = couponPageDto(saved.coupon || saved); if (!publish) return coupon; const id = coupon.resourceId; if (!id) throw new Error('后端未返回优惠券 ID，无法发布'); return couponPageDto(await call(publishLegacyCoupon(id, opt))); }
export async function setCouponPublishedDto(couponId: number, published: boolean): Promise<Coupon> { return couponPageDto(await call(published ? publishLegacyCoupon(couponId, apiRequestOptions()) : stopLegacyCoupon(couponId, apiRequestOptions()))); }
export async function copyCouponDto(couponId: number): Promise<Coupon> { return couponPageDto(await call(copyLegacyCoupon(couponId, apiRequestOptions()))); }
export async function archiveCouponDto(couponId: number): Promise<void> { await call(archiveLegacyCoupon(couponId, apiRequestOptions())); }
export async function deleteCouponDto(couponId: number): Promise<void> { await call(deleteLegacyCoupon(couponId, apiRequestOptions())); }
export type QuestionnaireWriteInput = LegacyQuestionnaireCreateRequest & { id?: number };
export async function saveQuestionnaireDto(input: QuestionnaireWriteInput, publish: boolean): Promise<Questionnaire> {
  const { id, ...payload } = input;
  const result = obj(await call(id == null ? createLegacyQuestionnaire(payload, apiRequestOptions()) : updateLegacyQuestionnaire(id, payload, apiRequestOptions()) as ReturnType<typeof createLegacyQuestionnaire>));
  const saved = (obj(result.data).questionnaire || result.questionnaire) as LegacyQuestionnaire | undefined;
  const questionnaireId = Number(saved?.id || result.questionnaire_id || id);
  if (!questionnaireId) throw new Error('后端未返回问卷 ID');
  if (publish) {
    const version = Number(saved?.version || obj(obj(await call(getLegacyQuestionnaire(questionnaireId, apiRequestOptions()))).questionnaire).version);
    await call(enableLegacyQuestionnaire(questionnaireId, apiRequestOptions()));
    await call(publishQuestionnairePublicDefinition(questionnaireId, { expected_questionnaire_version: version }, apiRequestOptions()));
  }
  const detail = obj(await call(getLegacyQuestionnaire(questionnaireId, apiRequestOptions())));
  return questionnairePageDto((detail.questionnaire || obj(detail.data).questionnaire) as LegacyQuestionnaire);
}
export async function setQuestionnaireEnabledDto(questionnaireId: number, enabled: boolean): Promise<void> { await call(enabled ? enableLegacyQuestionnaire(questionnaireId, apiRequestOptions()) : disableLegacyQuestionnaire(questionnaireId, { is_disabled: true }, apiRequestOptions())); }
export async function duplicateQuestionnaireDto(questionnaireId: number): Promise<Questionnaire> { const result = obj(await call(duplicateLegacyQuestionnaire(questionnaireId, undefined, apiRequestOptions()))); const id = Number(result.questionnaire_id || obj(result.questionnaire).id); if (!id) throw new Error('后端未返回复制后的问卷 ID'); const detail = obj(await call(getLegacyQuestionnaire(id, apiRequestOptions()))); return questionnairePageDto((detail.questionnaire || obj(detail.data).questionnaire) as LegacyQuestionnaire); }
export async function deleteQuestionnaireDto(questionnaireId: number): Promise<void> { await call(deleteLegacyQuestionnaire(questionnaireId, apiRequestOptions())); }
export type ChannelWriteInput = LegacyChannelWriteRequest & { id?: number };
export async function saveChannelDto(input: ChannelWriteInput): Promise<Channel> { const { id, ...payload } = input; const options = apiRequestOptions({ headers: { 'Idempotency-Key': globalThis.crypto?.randomUUID?.() || `web-channel-${Date.now()}` } }); const result = obj(await call(id == null ? createLegacyChannel(payload, options) : updateLegacyChannel(id, payload, options) as ReturnType<typeof createLegacyChannel>)); return channelPageDto(result.channel as LegacyChannel); }
export async function saveRadarLinkDto(input: RadarLinkInput): Promise<AdminDb['radarLinks'][number]> {
  if (!/^https:\/\//.test(input.original_url)) throw new Error('Radar 目标地址必须是 HTTPS');
  const mediaId = input.target_type === 'link' ? undefined : Number(input.media_item_id);
  if (input.target_type !== 'link' && (!Number.isInteger(mediaId) || Number(mediaId) < 1)) throw new Error('图片/PDF Radar 必须选择带服务端 ID 的素材');
  const refs = { cover_image_id: input.target_type === 'image' ? mediaId : null, attachment_id: input.target_type === 'pdf' ? mediaId : null };
  const opt = apiRequestOptions();
  if (input.id == null) {
    const created = obj(await call(createRadarLink({ expected_version: 0, name: input.title, title: input.title, destination_url: input.original_url, ...refs }, opt)));
    return radarPageDto(created.link as ApiRadarLink);
  }
  const current = obj(await call(getRadarLink(input.id, opt))).link as ApiRadarLink;
  const updated = obj(await call(updateRadarLink(input.id, { expected_version: current.version, name: input.title, title: input.title, destination_url: input.original_url, ...refs }, opt)));
  return radarPageDto(updated.link as ApiRadarLink);
}
export async function uploadRadarImageDto(file: File): Promise<RadarMedia> { const result = obj(await call(uploadLegacyImage({ image: file, name: file.name }, apiRequestOptions()))); const item = obj(result.item); return { id: Number(item.id), name: text(item.name, file.name), meta: `${text(item.mime_type, file.type)} · ${text(item.file_size, String(file.size))} bytes` }; }
export async function uploadRadarPdfDto(file: File): Promise<RadarMedia> { const item = obj(await call(uploadLegacyAttachment({ attachment: file, name: file.name }, apiRequestOptions()))); return { id: Number(item.id), name: text(item.name, file.name), meta: `${text(item.mime_type, file.type)} · ${text(item.file_size, String(file.size))} bytes` }; }
const splitTags = (value: string | undefined): string[] => (value || '').split(/[,，]/).map((tag) => tag.trim()).filter(Boolean);
async function uniqueMediaId(kind: 'image' | 'attachment' | 'mini', name: string): Promise<string | number> {
  const opt = apiRequestOptions();
  const response = kind === 'image' ? await call(getLegacyImageList(undefined, opt)) : kind === 'attachment' ? await call(listLegacyAttachments(undefined, opt)) : await call(listLegacyMiniPrograms(undefined, opt));
  const collection = kind === 'image' ? 'images' : kind === 'attachment' ? 'attachments' : 'mini_programs';
  const matches = list(response, 'items', collection).map(obj).filter((item) => text(item.name, text(item.file_name)) === name);
  if (matches.length !== 1) throw new Error(matches.length ? `存在多个同名素材「${name}」，请刷新后按资源 ID 操作` : `素材「${name}」不存在或已删除`);
  return kind === 'mini' ? Number(matches[0].id) : text(matches[0].id, '');
}
export async function saveImageItemDto(originalName: string | null, patch: Partial<ImageItem> & { name: string }): Promise<void> {
  if (!originalName) { if (!patch.file) throw new Error('请选择真实图片文件后再上传'); await call(uploadLegacyImage({ image: patch.file, name: patch.name, description: patch.desc, tags: patch.tags, category: patch.tag }, apiRequestOptions())); return; }
  const id = patch.resourceId || String(await uniqueMediaId('image', originalName));
  await call(updateLegacyImage(id, { name: patch.name, description: patch.desc, tags: patch.tags == null ? undefined : splitTags(patch.tags), category: patch.tag, enabled: patch.enabled }, apiRequestOptions()));
}
export async function deleteImageItemDto(item: ImageItem): Promise<void> { const id = item.resourceId || String(await uniqueMediaId('image', item.name)); await call(deleteLegacyImage(id, undefined, apiRequestOptions())); }
export async function getImageThumbnailDto(item: ImageItem): Promise<Blob> { const id = item.resourceId || String(await uniqueMediaId('image', item.name)); return (await request(getGetLegacyImageVariantUrl(id, 'thumb_320'))).blob(); }
export async function saveAttachmentItemDto(originalName: string | null, patch: Partial<AttachItem> & { name: string }): Promise<void> {
  if (!originalName) { if (!patch.file) throw new Error('请选择真实 PDF 文件后再上传'); await call(uploadLegacyAttachment({ attachment: patch.file, name: patch.name, tags: patch.tags }, apiRequestOptions())); return; }
  const id = patch.resourceId || String(await uniqueMediaId('attachment', originalName)); const current = obj(await call(getLegacyAttachment(id, apiRequestOptions())));
  await call(updateLegacyAttachment(id, { expected_version: Number(current.version), name: patch.name, description: text(current.description, ''), tags: patch.tags == null ? list(current, 'tags').map(String) : splitTags(patch.tags), enabled: patch.enabled ?? current.enabled !== false }, apiRequestOptions()));
}
export async function deleteAttachmentItemDto(item: AttachItem): Promise<void> { const id = item.resourceId || String(await uniqueMediaId('attachment', item.name)); await call(deleteLegacyAttachment(id, apiRequestOptions())); }
export async function downloadAttachmentItemDto(item: AttachItem): Promise<Blob> { const id = item.resourceId || String(await uniqueMediaId('attachment', item.name)); return (await request(getDownloadLegacyAttachmentUrl(id))).blob(); }
export async function saveMiniProgramItemDto(originalName: string | null, patch: Partial<MpItem> & { name: string }): Promise<void> {
  const payload = { name: patch.name, appid: patch.appid, pagepath: patch.pagepath, title: patch.cardTitle || patch.name, enabled: patch.enabled };
  if (!originalName) { if (!patch.appid || !patch.pagepath) throw new Error('请填写 AppID 与页面路径'); await call(createLegacyMiniProgram(payload, apiRequestOptions())); return; }
  const id = patch.resourceId || Number(await uniqueMediaId('mini', originalName)); await call(updateLegacyMiniProgram(id, payload, apiRequestOptions()));
}
export async function deleteMiniProgramItemDto(item: MpItem): Promise<void> { const id = item.resourceId || Number(await uniqueMediaId('mini', item.name)); await call(deleteLegacyMiniProgram(id, apiRequestOptions())); }
async function audiencePackageVersion(packageId: number): Promise<number> { return Number(obj(obj(await call(getAIAudiencePackage(packageId, apiRequestOptions()))).package).version); }
export async function saveAudienceGroup(input: { id?: number; name: string }): Promise<AdminDb['audienceGroups'][number]> {
  const opt = apiRequestOptions();
  if (input.id == null) {
    const result = obj(await call(createAIAudiencePackageGroup({ name: input.name, expected_version: 0 }, opt)));
    return audienceGroupPageDto(result.group);
  }
  const groups = await call(listAIAudiencePackageGroups(opt));
  const existing = list(groups, 'items').map(obj).find((item) => Number(item.group_id) === input.id);
  if (!existing) throw new Error('标签组不存在或已被删除');
  const result = obj(await call(updateAIAudiencePackageGroup(input.id, { name: input.name, expected_version: Number(existing.version) }, opt)));
  return audienceGroupPageDto(result.group);
}
export async function deleteAudienceGroup(groupId: number): Promise<void> { const groups = await call(listAIAudiencePackageGroups(apiRequestOptions())); const existing = list(groups, 'items').map(obj).find((item) => Number(item.group_id) === groupId); if (!existing) throw new Error('标签组不存在或已被删除'); await call(deleteAIAudiencePackageGroup(groupId, { expected_version: Number(existing.version) }, apiRequestOptions())); }
export async function setAudiencePackageRunning(packageId: number, running: boolean): Promise<void> { const expected_version = await audiencePackageVersion(packageId); await call(running ? activateAIAudiencePackage(packageId, { expected_version }, apiRequestOptions()) : pauseAIAudiencePackage(packageId, { expected_version }, apiRequestOptions())); }
export async function copyAudiencePackageDto(packageId: number): Promise<AdminDb['audiencePackages'][number]> { const expected_version = await audiencePackageVersion(packageId); const result = obj(await call(copyAIAudiencePackage(packageId, { expected_version }, apiRequestOptions()))); return audiencePackagePageDto(result.package); }
export async function archiveAudiencePackage(packageId: number): Promise<void> { const expected_version = await audiencePackageVersion(packageId); await call(archiveAIAudiencePackage(packageId, { expected_version }, apiRequestOptions())); }
export type AudiencePackageWriteInput = { id: number; name: string; groupId: number | null; definition: SegmentDefinition; refreshMode: 'manual' | 'scheduled'; refreshCron: string | null };
export type AudienceEvaluation = { configurationVersion: number; packageVersion: number; memberCount: number; memberDigest: string; evaluatedAt: string; materialized: boolean };
export async function saveAudiencePackageDto(input: AudiencePackageWriteInput): Promise<AdminDb['audiencePackages'][number]> { const current = obj(obj(await call(getAIAudiencePackage(input.id, apiRequestOptions()))).package); const result = obj(await call(updateAIAudiencePackage(input.id, { name: input.name, group_id: input.groupId, definition: input.definition, refresh_mode: input.refreshMode, refresh_cron: input.refreshCron, expected_version: Number(current.version) }, apiRequestOptions()))); return audiencePackagePageDto(result.package); }
export async function replaceAudienceSendersDto(packageId: number, senders: AIAudiencePackageSender[]): Promise<AdminDb['audienceSenders'][number]> { const result = obj(await call(replaceAIAudiencePackageSenders(packageId, { items: senders }, apiRequestOptions()))); return list(result, 'items').map((item) => ({ priority: Number(obj(item).sort_order), userid: text(obj(item).sender_userid), rule: '服务端顺序', status: obj(item).is_enabled === false ? '停用' : '启用' })); }
export async function setAudienceBindingDto(packageId: number, automationAgentId: number | null): Promise<void> { const current = obj(await call(getAIAudienceAutomationBinding(packageId, apiRequestOptions()))).binding; const expectedVersion = Number(obj(current).version || 0); if (automationAgentId == null) { if (current) await call(deleteAIAudienceAutomationBinding(packageId, { expected_version: expectedVersion }, apiRequestOptions())); return; } await call(putAIAudienceAutomationBinding(packageId, { automation_agent_id: automationAgentId, expected_version: expectedVersion }, apiRequestOptions())); }
export async function snapshotAudienceConfigurationDto(packageId: number): Promise<number> { const [pkgResult, cfgResult] = await Promise.all([call(getAIAudiencePackage(packageId, apiRequestOptions())), call(getAIAudienceConfigurationVersion(packageId, apiRequestOptions()))]); const pkg = obj(obj(pkgResult).package); const cfg = obj(obj(cfgResult).configuration); const result = obj(await call(putAIAudienceConfigurationVersion(packageId, { expected_version: Number(cfg.version || 0), expected_package_version: Number(pkg.version) }, apiRequestOptions()))); return Number(obj(result.configuration).version); }
const audienceEvaluationDto = (value: unknown): AudienceEvaluation => { const x = obj(value); return { configurationVersion: Number(x.configuration_version), packageVersion: Number(x.package_version), memberCount: Number(x.member_count), memberDigest: text(x.member_digest), evaluatedAt: text(x.evaluated_at), materialized: x.materialized === true }; };
export async function previewAudienceConfigurationDto(packageId: number): Promise<AudienceEvaluation> { const cfg = obj(obj(await call(getAIAudienceConfigurationVersion(packageId, apiRequestOptions()))).configuration); if (!cfg.version) throw new Error('请先保存配置版本'); return audienceEvaluationDto(await call(previewAIAudienceConfiguration(packageId, { configuration_version: Number(cfg.version) }, apiRequestOptions()))); }
export async function materializeAudienceConfigurationDto(packageId: number): Promise<AudienceEvaluation> { const [pkgResult, cfgResult] = await Promise.all([call(getAIAudiencePackage(packageId, apiRequestOptions())), call(getAIAudienceConfigurationVersion(packageId, apiRequestOptions()))]); const pkg = obj(obj(pkgResult).package); const cfg = obj(obj(cfgResult).configuration); if (!cfg.version) throw new Error('请先保存配置版本'); return audienceEvaluationDto(await call(materializeAIAudienceConfiguration(packageId, { configuration_version: Number(cfg.version), expected_package_version: Number(pkg.version) }, apiRequestOptions()))); }
export type GroupOpsWriteInput = { id?: string; name: string; staffIds: number[]; assetReferences: string[]; nodes: Array<{ id?: string; position: number; kind: 'message' | 'delay'; messageText?: string; delayMinutes?: number; materialReference?: string; materialPlan?: GroupOpsMaterialPlan }>; webhookReference?: string };
async function readGroupOpsDetail(planId: string): Promise<NonNullable<AdminDb['groupOpsDetail']>> { const detail = await call(getGroupOpsPlan(planId, apiRequestOptions())); return groupOpsDetailDto(detail); }
export async function saveGroupOpsPlanDto(input: GroupOpsWriteInput): Promise<NonNullable<AdminDb['groupOpsDetail']>> {
  const opt = apiRequestOptions();
  let detail: NonNullable<AdminDb['groupOpsDetail']>;
  if (!input.id) { detail = groupOpsDetailDto(await call(createGroupOpsPlan({ name: input.name }, opt))); }
  else { detail = await readGroupOpsDetail(input.id); if (detail.plan.name !== input.name) { await call(updateGroupOpsPlan(input.id, { expected_revision: detail.plan.revision, name: input.name }, opt)); detail = await readGroupOpsDetail(input.id); } }
  const planId = detail.plan.id;
  for (const staffId of detail.staffIds.filter((id) => !input.staffIds.includes(id))) { await call(removeGroupOpsPlanMember(planId, String(staffId), { expected_revision: detail.plan.revision }, opt)); detail = await readGroupOpsDetail(planId); }
  for (const staffId of input.staffIds.filter((id) => !detail.staffIds.includes(id))) { await call(addGroupOpsPlanMember(planId, { expected_revision: detail.plan.revision, staff_id: staffId }, opt)); detail = await readGroupOpsDetail(planId); }
  for (const asset of detail.assets.filter((item) => !input.assetReferences.includes(item.reference))) { await call(removeGroupOpsPlanGroupAsset(planId, asset.id, { expected_revision: detail.plan.revision }, opt)); detail = await readGroupOpsDetail(planId); }
  for (const reference of input.assetReferences.filter((value) => !detail.assets.some((item) => item.reference === value))) { await call(addGroupOpsPlanGroupAsset(planId, { expected_revision: detail.plan.revision, asset_reference: reference }, opt)); detail = await readGroupOpsDetail(planId); }
  for (const node of detail.nodes.filter((item) => item.id && !input.nodes.some((candidate) => candidate.id === item.id))) { await call(removeGroupOpsPlanNode(planId, node.id!, { expected_revision: detail.plan.revision }, opt)); detail = await readGroupOpsDetail(planId); }
  for (const node of input.nodes) { const payload: GroupOpsNodeRequest = { expected_revision: detail.plan.revision, position: node.position, kind: node.kind, message_text: node.kind === 'message' ? node.messageText : undefined, delay_minutes: node.kind === 'delay' ? node.delayMinutes : undefined, material_plan: node.materialPlan || { references: [] } }; if (node.id && detail.nodes.some((item) => item.id === node.id)) await call(updateGroupOpsPlanNode(planId, node.id, payload, opt)); else await call(addGroupOpsPlanNode(planId, payload, opt)); detail = await readGroupOpsDetail(planId); }
  if ((detail.webhookReference || '') !== (input.webhookReference || '')) { await call(putGroupOpsWebhookDescriptor(planId, { expected_revision: detail.plan.revision, reference: input.webhookReference || undefined }, opt)); detail = await readGroupOpsDetail(planId); }
  const preview = await call(previewGroupOpsPlanContent(planId, opt));
  return groupOpsDetailDto(await call(getGroupOpsPlan(planId, opt)), preview);
}
export async function transitionGroupOpsPlanDto(planId: string, action: 'activate' | 'pause' | 'archive'): Promise<void> { const revision = (await readGroupOpsDetail(planId)).plan.revision; await call(action === 'activate' ? activateGroupOpsPlan(planId, { expected_revision: revision }, apiRequestOptions()) : action === 'pause' ? pauseGroupOpsPlan(planId, { expected_revision: revision }, apiRequestOptions()) : archiveGroupOpsPlan(planId, { expected_revision: revision }, apiRequestOptions())); }
export async function deleteGroupOpsPlanDto(planId: string): Promise<void> { const revision = (await readGroupOpsDetail(planId)).plan.revision; await call(deleteGroupOpsPlan(planId, { expected_revision: revision }, apiRequestOptions())); }
export type RefundIntentInput = { provider: string; orderNo: string; amount: string; reason: string; transactionIdConfirmation: string; checked: boolean; productId?: string; skuId?: string; refundCount?: number; reasonCode?: WechatShopRefundRequest['reason_code'] };
export type RefundIntentResult = { id: string; state: string; provider: string; realExternalCallExecuted: boolean; deliveryProven: boolean };
export async function createRefundIntentDto(input: RefundIntentInput): Promise<RefundIntentResult> {
  if (!input.checked || input.transactionIdConfirmation !== input.orderNo) throw new Error('必须勾选确认并完整输入当前订单号');
  const refund_amount_total = priceMinor(input.amount);
  const payload = { refund_amount_total, reason: input.reason, transaction_id_confirmation: input.transactionIdConfirmation, checked: true };
  let result: unknown;
  if (input.provider === 'wechat_shop') {
    const productId = input.productId?.trim();
    const skuId = input.skuId?.trim();
    if (!productId || !skuId || !Number.isInteger(input.refundCount) || Number(input.refundCount) < 1 || Number(input.refundCount) > 1_000_000 || !input.reasonCode) throw new Error('微信小店退款需要商品、SKU、退款数量和官方售后原因码');
    const request: WechatShopRefundRequest = { provider: 'wechat_shop', order_no: input.orderNo, product_id: productId, sku_id: skuId, refund_count: Number(input.refundCount), reason_code: input.reasonCode, ...payload };
    result = await call(createLegacyRefundIntent(request, apiRequestOptions()));
  } else if (input.provider === 'wechat_pay' || input.provider === 'wechat') result = await call(createLegacyWechatRefundIntent(input.orderNo, { provider: 'wechat', order_no: input.orderNo, ...payload }, apiRequestOptions()));
  else throw new Error(`后端能力未就绪：${input.provider || '未知'} 支付来源没有等价退款 intent operation`);
  const x = obj(result);
  return { id: text(x.id, text(x.refund_id)), state: text(x.state, text(x.status)), provider: text(x.provider, input.provider), realExternalCallExecuted: x.real_external_call_executed === true, deliveryProven: x.delivery_proven === true };
}
export async function saveQuestionnaireOpsDto(questionnaireId: number, ops: QuestionnaireOps): Promise<void> { const opaque = /^[A-Za-z0-9._:-]{1,128}$/; const navigation = ops.completionNavigationTargetId.trim(); const reference = ops.externalPushConfigurationReference.trim(); const channel = ops.completionChannelId.trim(); if (navigation && !opaque.test(navigation)) throw new Error('提交后导航目标必须是 1-128 位 opaque reference，不能填写 URL'); if (reference && !opaque.test(reference)) throw new Error('外部推送配置必须是 1-128 位 opaque reference，不能填写 URL'); if (ops.pushEnabled && !reference) throw new Error('启用外部推送时必须提供 configuration reference'); const channel_id = channel ? Number(channel) : undefined; if (channel && (!Number.isInteger(channel_id) || Number(channel_id) < 1)) throw new Error('渠道资源 ID 必须是正整数'); const opt = apiRequestOptions(); await call(saveSurveyCompletionOperations(questionnaireId, navigation || channel_id ? { navigation_target_id: navigation || undefined, channel_id } : {}, opt)); await call(saveSurveyExternalPushOperations(questionnaireId, { enabled: ops.pushEnabled, configuration_reference: ops.pushEnabled ? reference : undefined }, opt)); }
export async function queueQuestionnairePushTestDto(questionnaireId: number): Promise<{ id: number; status: string; attemptCount: number }> { const result = obj(await call(queueSurveyExternalPushTest(questionnaireId, apiRequestOptions()))); return { id: Number(result.test_run_id), status: text(result.status), attemptCount: Number(result.attempt_count || 0) }; }
export type HxcSenderWriteInput = { id: string; senderUserid: string; displayName: string; priority: number; active: boolean };
export async function saveHxcSenderDto(input: HxcSenderWriteInput): Promise<AdminDb['rows']['agents'][number]> { if (!input.id || !input.senderUserid) throw new Error('配置 ID 和 sender_userid 不能为空'); if (!Number.isInteger(input.priority) || input.priority < 0 || input.priority > 100000) throw new Error('优先级必须是 0-100000 的整数'); const result = obj(await call(upsertLegacyHXCSendConfig({ id: input.id, sender_userid: input.senderUserid, display_name: input.displayName, priority: input.priority, is_active: input.active }, apiRequestOptions()))); return hxcSenderPageDto(result.item as LegacyHXCSenderConfig); }
export async function reorderHxcSendersDto(ids: string[]): Promise<void> { const clean = ids.map((id) => id.trim()).filter(Boolean); if (!clean.length || new Set(clean).size !== clean.length) throw new Error('排序列表不能为空且 ID 不能重复'); await call(reorderLegacyHXCSendConfigs({ ids: clean }, apiRequestOptions())); }
export async function archiveHxcSenderDto(senderUserid: string): Promise<void> { await call(archiveLegacyHXCSendConfig(senderUserid, apiRequestOptions())); }
export async function saveAppSettingsDto(category: ConfigCategory, values: Record<string, string>): Promise<void> { if (!category.actionToken) throw new Error('后端未返回 route-bound Admin Action Token，未发送请求'); const settings: Record<string, string | number> = {}; for (const field of category.blocks.flatMap((block) => block.fields)) { if (field.kind === 'secret') continue; const value = values[field.key]; if (value === undefined) continue; if (field.kind === 'number') { const number = Number(value); if (!Number.isFinite(number)) throw new Error(`${field.key} 必须是数字`); settings[field.key] = number; } else settings[field.key] = value; } await call(saveLegacyAppSettingsResource({ settings, confirm: true, admin_action_token: category.actionToken }, apiRequestOptions())); }
const writeMeta = () => ({ idempotency_key: globalThis.crypto?.randomUUID?.() || `web-${Date.now()}` });
export async function saveTagGroupDto(input: { id?: number; name: string; firstTag?: string }): Promise<TagGroup> { const opt = apiRequestOptions(); if (input.id == null) { if (!input.firstTag) throw new Error('新建标签组必须提供首个标签'); const result = obj(await call(createLegacyWecomTagGroup({ group_name: input.name, first_tag_name: input.firstTag, ...writeMeta() }, opt))); return tagGroupPageDto(result.group); } const result = obj(await call(updateLegacyWecomTagGroupPatch(input.id, { group_name: input.name, ...writeMeta() }, opt))); return tagGroupPageDto(result.group); }
export async function saveTagDto(input: { id?: number; groupId: number; name: string }): Promise<WecomTag> { const opt = apiRequestOptions(); if (input.id != null) { const result = obj(await call(updateLegacyWecomTagPatch(input.id, { tag_name: input.name, ...writeMeta() }, opt))); return tagPageDto(result.tag); } const group = obj(await call(getLegacyWecomTagGroup(input.groupId, opt))).group; if (!group) throw new Error('标签组不存在或已被删除'); const result = obj(await call(createLegacyWecomTag({ group_id: input.groupId, group_name: text(obj(group).name), tag_name: input.name, ...writeMeta() }, opt))); return tagPageDto(result.tag); }
export async function archiveTagDto(tagId: number): Promise<void> { await call(archiveLegacyWecomTag(tagId, writeMeta(), apiRequestOptions())); }
export async function queueTagSyncDto(): Promise<unknown> { return call(queueLegacyWecomTagSync(writeMeta(), apiRequestOptions())); }

export function emptyAdminDb(): AdminDb { return { radarLinks: [], radarEvents: [], aiPlans: [], aiRcs: {}, funnelRows: [], funnelViews: [], audienceGroups: [], audiencePackages: [], audienceMembers: {}, audienceSenders: {}, audienceRecords: {}, groupOpsPlans: [], groupOpsDetail: null, cycleTasks: [], cycleRuns: {}, qOps: {}, tagGroups: [], wecomTags: [], couponClaims: {}, configCategories: [], staff: [], groupChats: [], customerList: { total: 0, totalIsEstimate: false, nextCursor: null }, customerDetail: { status: 'not_found', context: null, survey: null, error: '' }, rows: { customers: [], tags: [], qa: [], msgs: [], qStats: [], questionnaires: [], qSubs: [], qApply: [], edTools: [], edQs: [], edAssignees: [], chStats: [], channels: [], orders: [], orderKv: [], orderEvents: [], spProducts: [], products: [], coupons: [], images: [], mpItems: [], attachItems: [], agents: [], agentSlots: [], agentDeps: [] } }; }
export interface CustomerListQuery {
  cursor?: string;
  keyword?: string;
  mobile?: string;
  ownerStaffId?: number;
  tagId?: number;
}
export interface AdminReadContext { page?: string; id?: string; customerList?: CustomerListQuery }

/** Shared lists are loaded from current operations only. A rejected request reaches the page error state. */
export async function readAdminRows(page?: string, customerList?: CustomerListQuery): Promise<AdminDb> {
  const opt = apiRequestOptions();
  const needs = (...screens: string[]) => !page || screens.includes(page);
  const skip = Promise.resolve({});
  const customerParams = {
    limit: 50,
    ...(customerList?.cursor ? { cursor: customerList.cursor } : {}),
    ...(customerList?.keyword ? { keyword: customerList.keyword } : {}),
    ...(customerList?.mobile ? { mobile: customerList.mobile } : {}),
    ...(customerList?.ownerStaffId == null ? {} : { owner_staff_id: customerList.ownerStaffId }),
    ...(customerList?.tagId == null ? {} : { tag_id: customerList.tagId }),
  };
  const responses = await Promise.all([
    needs('customers') ? call(listCustomers(customerParams, opt)) : skip,
    needs('questionnaires', 'questionnaireDetail', 'questionnaireOps') ? call(listLegacyQuestionnaires({ limit: 50, offset: 0 }, opt)) : skip,
    needs('channels', 'channelForm', 'questionnaireOps', 'productForm', 'spProductForm') ? call(listLegacyChannels({ limit: 50, include_archived: true }, opt)) : skip,
    needs('orders', 'orderDetail') ? call(listLegacyOrders(undefined, opt)) : skip,
    needs('products', 'productForm') ? call(listProducts(undefined, opt)) : skip,
    needs('spProducts', 'spProductForm', 'spProductData') ? call(listServicePeriodProducts(undefined, opt)) : skip,
    needs('coupons', 'couponForm', 'couponData') ? call(listLegacyCoupons(undefined, opt)) : skip,
    needs('images') ? call(getLegacyImageList(undefined, opt)) : skip,
    needs('attach') ? call(listLegacyAttachments(undefined, opt)) : skip,
    needs('mpLib') ? call(listLegacyMiniPrograms(undefined, opt)) : skip,
    needs('tags', 'channelForm') ? call(listLegacyWecomTagGroups(opt)) : skip,
    needs('tags', 'channelForm') ? call(listLegacyWecomTags(opt)) : skip,
    needs('radar', 'radarDetail', 'radarForm') ? call(listRadarLinks(undefined, opt)) : skip,
    needs('automation', 'audienceEdit') ? call(listAIAudiencePackageGroups(opt)) : skip,
    needs('automation', 'audienceEdit') ? call(listAIAudiencePackages(undefined, opt)) : skip,
    needs('groupops', 'groupopsDetail') ? call(listGroupOpsPlans({ limit: 100, offset: 0 }, opt)) : skip,
    needs('config', 'configDetail') ? call(listAdminOpsCategories(opt)) : skip,
    needs('agents', 'agentEdit') ? call(getLegacyHXCSendConfig(opt)) : skip,
    needs('config', 'configDetail') ? call(getLegacyAppSettingsResource(undefined, opt)) : skip,
    needs('config', 'configDetail') ? call(getAdminOpsPushCapabilities(opt)) : skip,
    needs('config', 'configDetail') ? call(listAdminOpsReleases(opt)) : skip,
  ]);
  const [customers, questionnaires, channels, orders, products, spProducts, coupons, images, attachments, minis, tagGroups, tags, radar, audienceGroups, audiencePackages, groupOps, config, hxc, appSettings, pushCapabilities, releases] = responses; const db = emptyAdminDb();
  db.rows.customers = list(customers, 'items').map((x) => customerPageDto(x as ApiCustomer)); const customerSource = obj(customers); db.customerList = { total: typeof customerSource.total === 'number' ? customerSource.total : db.rows.customers.length, totalIsEstimate: customerSource.total_is_estimate === true, nextCursor: typeof customerSource.next_cursor === 'string' ? customerSource.next_cursor : null }; db.rows.questionnaires = list(questionnaires, 'items', 'questionnaires').map((x) => questionnairePageDto(x as LegacyQuestionnaire)); db.rows.channels = list(channels, 'channels', 'items').map((x) => channelPageDto(x as LegacyChannelListItem)); db.rows.orders = list(orders, 'items', 'orders').map(orderPageDto); db.rows.products = list(products, 'items').map(productPageDto); db.rows.spProducts = list(spProducts, 'items').map(serviceProductPageDto); db.rows.coupons = list(coupons, 'items', 'coupons').map(couponPageDto); db.rows.images = list(images, 'items', 'images').map(imagePageDto); db.rows.attachItems = list(attachments, 'items', 'attachments').map(attachmentPageDto); db.rows.mpItems = list(minis, 'items', 'mini_programs').map(miniProgramPageDto); db.tagGroups = list(tagGroups, 'items', 'groups').map(tagGroupPageDto); db.wecomTags = list(tags, 'items', 'tags').map(tagPageDto); db.radarLinks = list(radar, 'items').map((x) => radarPageDto(x as ApiRadarLink)); db.audienceGroups = list(audienceGroups, 'items', 'groups').map(audienceGroupPageDto); db.audiencePackages = list(audiencePackages, 'items').map(audiencePackagePageDto); db.groupOpsPlans = list(groupOps, 'items').map(groupOpsPlanDto); db.configCategories = list(config, 'categories', 'items').map(configCategoryPageDto); if (obj(appSettings).config) db.configCategories.push(appSettingsPageDto(appSettings)); if (obj(pushCapabilities).capabilities) db.configCategories.push(readOnlyConfigPageDto('push-capabilities', pushCapabilities)); if (Array.isArray(obj(releases).releases)) db.configCategories.push(readOnlyConfigPageDto('releases', releases)); db.rows.agents = list(hxc, 'send_configs').map((x) => hxcSenderPageDto(x as LegacyHXCSenderConfig)); return db;
}

/** Detail page reads are deliberately page-scoped and never synthesize demo records. */
export async function readAdminPage(context: AdminReadContext = {}): Promise<AdminDb> {
  const db = await readAdminRows(context.page, context.customerList); const id = context.id || ''; const opt = apiRequestOptions(); const numeric = Number(id);
  if (context.page === 'customerDetail') {
    if (!id || !/^[1-9][0-9]*$/.test(id) || !Number.isSafeInteger(numeric)) {
      db.customerDetail = { status: 'not_found', context: null, survey: null, error: '客户档案不存在或 OneID 无效' };
      return db;
    }
    try {
      const [rawContext, rawSurvey, stages] = await Promise.all([call(getCustomerContext(numeric, { limit: 20 }, opt)), call(listCustomerSurveyAnswers(numeric, { limit: 30 }, opt)), call(listStages(opt))]);
      const customerContext = customerContextPageDto(rawContext);
      if (customerContext.profile.id !== String(numeric)) throw new Error('客户安全上下文 OneID 不匹配');
      db.customerDetail = { status: 'ready', context: customerContext, survey: customerSurveyPageDto(rawSurvey, numeric), error: '' };
      db.rows.tags = customerContext.tags;
      db.rows.orderKv = list(stages, 'items').map((x) => ({ k: text(obj(x).name), v: text(obj(x).id), mono: false }));
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        db.customerDetail = { status: 'not_found', context: null, survey: null, error: '客户档案不存在或当前账号不可见' };
        return db;
      }
      throw error;
    }
  }
  if (!id) return db;
  if (context.page === 'questionnaireDetail' || context.page === 'questionnaireOps') { const [detail, results, submissions, analysis, operations, pageData, logs] = await Promise.all([call(getLegacyQuestionnaire(numeric, opt)), call(getLegacyQuestionnaireResults(numeric, opt)), call(listLegacyQuestionnaireSubmissions(numeric, undefined, opt)), call(getSurveySafeSubmissionAnalysis(numeric, undefined, opt)), call(getSurveyOperations(numeric, opt)), call(getSurveyOperationsPageData(numeric, opt)), call(listSurveyQuestionnaireExternalPushLogs(numeric, undefined, opt))]); const q = obj(detail).questionnaire || detail; db.rows.questionnaires = [questionnairePageDto(q as LegacyQuestionnaire)]; db.qOps[numeric] = questionnaireOpsPageDto(operations); db.rows.qSubs = list(submissions, 'items', 'submissions').map((x) => ({ time: text(obj(x).submitted_at), uid: text(obj(x).customer_id), by: text(obj(x).customer_name), score: text(obj(x).score), tags: list(obj(x).tags).map(String) })); db.rows.qApply = list(logs, 'items', 'logs').map((x) => ({ time: text(obj(x).created_at), sid: text(obj(x).submission_id), uid: text(obj(x).external_userid), status: text(obj(x).status), tone: toneFor(obj(x).status), err: text(obj(x).error, '') })); void results; void analysis; void pageData; }
  if (context.page === 'channelForm') {
    try {
      const detail = await call(getLegacyChannel(numeric, opt));
      db.rows.channels = [channelPageDto((obj(detail).channel || detail) as LegacyChannel)];
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        db.rows.channels = [];
        return db;
      }
      throw error;
    }
  }
  if (context.page === 'orderDetail') { const [detail, items, refunds, effects] = await Promise.all([call(getLegacyOrder(id, undefined, opt)), call(getLegacyOrderItems(id, undefined, opt)), call(listLegacyRefunds(undefined, opt)), call(listLegacyWechatOrderExternalEffects(id, opt))]); db.rows.orders = [orderPageDto(obj(detail).order || detail)]; db.rows.orderKv = Object.entries(obj(detail)).map(([k, v]) => ({ k, v: text(v), mono: false })); db.rows.orderEvents = [...list(items, 'items').map((x) => ({ time: text(obj(x).created_at), ev: text(obj(x).name), st: text(obj(x).status), tone: toneFor(obj(x).status) })), ...list(refunds, 'items', 'refunds').map((x) => ({ time: text(obj(x).created_at), ev: '退款 ' + text(obj(x).refund_no), st: text(obj(x).status), tone: toneFor(obj(x).status) })), ...list(effects, 'items', 'effects').map((x) => ({ time: text(obj(x).created_at), ev: '外推回执', st: text(obj(x).status), tone: toneFor(obj(x).status) }))]; }
  if (context.page === 'productForm') { const [detail, entitlements] = await Promise.all([call(getProduct(numeric, opt)), call(listProductLocalEntitlements(numeric, undefined, opt))]); db.rows.products = [productPageDto(detail)]; db.rows.orderKv = list(entitlements, 'items').map((x) => ({ k: text(obj(x).name), v: text(obj(x).status), mono: false })); }
  if (context.page === 'spProductForm') { const [detail, members, access, schema, views, share] = await Promise.all([call(getServicePeriodProduct(numeric, opt)), call(listServicePeriodMembers(numeric, undefined, opt)), call(getServicePeriodMemberGridAccess(numeric, opt)), call(getServicePeriodMemberGridSchema(numeric, opt)), call(listServicePeriodMemberViews(numeric, opt)), call(getServicePeriodMemberGridShareSettings(numeric, opt))]); db.rows.spProducts = [serviceProductPageDto(detail)]; db.rows.orderKv = [...list(members, 'items').map((x) => ({ k: text(obj(x).name), v: text(obj(x).status), mono: false })), { k: 'member-grid', v: text(obj(schema).version), mono: false }, { k: 'views', v: String(list(views, 'items').length), mono: false }, { k: 'share', v: text(obj(share).enabled), mono: false }, { k: 'access', v: text(obj(access).role), mono: false }]; }
  if (context.page === 'spProductData') { const [detail, grid, access, schema, views, share] = await Promise.all([call(getServicePeriodProduct(numeric, opt)), call(queryServicePeriodMemberGrid(numeric, { state: 'all', limit: 50 }, opt)), call(getServicePeriodMemberGridAccess(numeric, opt)), call(getServicePeriodMemberGridSchema(numeric, opt)), call(listServicePeriodMemberViews(numeric, opt)), call(getServicePeriodMemberGridShareSettings(numeric, opt))]); db.rows.spProducts = [serviceProductPageDto(detail)]; db.rows.orderKv = [...list(grid, 'rows').map((x) => { const row = obj(x); return { k: `${text(row.display_name)} (${text(row.member_ref)})`, v: `${text(row.state)} · ${text(row.source)} · ${text(row.updated_at)}`, mono: false }; }), { k: 'member-grid-columns', v: String(list(obj(schema), 'columns').length), mono: false }, { k: 'views', v: String(list(views, 'views', 'items').length), mono: false }, { k: 'external-share-enabled', v: text(obj(share).external_share_enabled), mono: false }, { k: 'can-query', v: text(obj(access).can_query), mono: false }]; }
  if (context.page === 'couponForm' || context.page === 'couponData') { const [detail, share, claims, options] = await Promise.all([call(getLegacyCoupon(numeric, opt)), call(getLegacyCouponShare(numeric, opt)), call(listLegacyCouponClaims(numeric, undefined, opt)), call(listLegacyCouponProductOptions(undefined, opt))]); db.rows.coupons = [couponPageDto(obj(detail).coupon || detail)]; db.couponClaims[0] = list(claims, 'items', 'claims').map((x) => ({ user: text(obj(x).customer_name), status: text(obj(x).status), tone: toneFor(obj(x).status), claimedAt: text(obj(x).claimed_at), validWindow: text(obj(x).valid_window), product: text(obj(x).product_name), orderNo: text(obj(x).order_no), usedAt: text(obj(x).used_at) })); db.rows.orderKv = [...list(options, 'items').map((x) => ({ k: text(obj(x).label, text(obj(x).name)), v: text(obj(x).value, text(obj(x).id)), mono: false })), { k: 'share', v: text(obj(share).url), mono: true }]; }
  if (context.page === 'images') { const [detail, facets] = await Promise.all([call(getLegacyImage(id, undefined, opt)), call(getLegacyImageFacets(opt))]); db.rows.images = [imagePageDto(obj(detail).item || detail)]; db.rows.orderKv = [{ k: 'facets', v: String(list(facets, 'items', 'facets').length), mono: false }]; }
  if (context.page === 'attach') { const detail = await call(getLegacyAttachment(id, opt)); db.rows.attachItems = [attachmentPageDto(obj(detail).item || detail)]; }
  if (context.page === 'mpLib') { const detail = await call(getLegacyMiniProgram(numeric, opt)); db.rows.mpItems = [miniProgramPageDto(obj(detail).item || detail)]; }
  if (context.page === 'tags') { const [group, tag, gate] = await Promise.all([call(getLegacyWecomTagGroup(numeric, opt)), call(getLegacyWecomTag(numeric, opt)), call(getLegacyWecomTagExecutionGate(opt))]); db.tagGroups = [tagGroupPageDto(obj(group).group || group)]; db.wecomTags = [tagPageDto(obj(tag).tag || tag)]; db.rows.orderKv = [{ k: 'live-gate', v: text(obj(gate).status), mono: false }]; }
  if (context.page === 'audienceEdit') { const [detail, bindingResult, senderResult, configResult, memberResult] = await Promise.all([call(getAIAudiencePackage(numeric, opt)), call(getAIAudienceAutomationBinding(numeric, opt)), call(getAIAudiencePackageSenders(numeric, opt)), call(getAIAudienceConfigurationVersion(numeric, opt)), call(listAIAudiencePackageMembers(numeric, { limit: 200, offset: 0 }, opt))]); const pkg = audiencePackagePageDto(obj(detail).package); const binding = obj(obj(bindingResult).binding); const configVersion = obj(obj(configResult).configuration); pkg.bindingAgentId = Number(binding.automation_agent_id || 0) || undefined; pkg.bindingVersion = Number(binding.version || 0); pkg.boundAutomation = pkg.bindingAgentId ? String(pkg.bindingAgentId) : ''; pkg.configurationVersion = Number(configVersion.version || 0) || undefined; db.audiencePackages = [pkg]; db.audienceSenders[numeric] = list(senderResult, 'items').map((item) => ({ priority: Number(obj(item).sort_order), userid: text(obj(item).sender_userid), rule: '服务端顺序', status: obj(item).is_enabled === false ? '停用' : '启用' })); db.audienceMembers[numeric] = list(memberResult, 'items').map((item) => ({ name: text(obj(item).nickname), external_userid: text(obj(item).external_userid), joinedAt: text(obj(item).entered_at) })); }
  if (context.page === 'groupopsDetail') {
    const [detail, preview, runDue, executions, webhook] = await Promise.all([
      call(getGroupOpsPlan(id, opt)),
      call(previewGroupOpsPlanContent(id, opt)),
      call(previewGroupOpsRunDue(id, opt)),
      call(listGroupOpsExecutions(id, { limit: 100, offset: 0 }, opt)),
      call(getGroupOpsWebhookDescriptor(id, opt)),
    ]);
    db.groupOpsDetail = groupOpsDetailDto(detail, preview);
    db.groupOpsPlans = [db.groupOpsDetail.plan];
    db.rows.orderKv = [...groupOpsPreviewDto(id, runDue), ...groupOpsWebhookDescriptorDto(webhook)];
    db.rows.orderEvents = groupOpsExecutionRows(id, executions);
  }
  if (context.page === 'configDetail' && !['app-settings', 'push-capabilities', 'releases'].includes(id)) { const detail = obj(await call(getAdminOpsCategory(id, opt))); const category = detail.category; if (category) db.configCategories = [configCategoryPageDto(category)]; }
  return db;
}
