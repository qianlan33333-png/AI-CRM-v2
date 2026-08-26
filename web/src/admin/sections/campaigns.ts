import {
  decideCampaignTouchPlanReviewDto,
  decideCampaignTouchPlanRecipientReviewDto,
  deleteCampaignDto,
  getCampaignDto,
  getCampaignTouchPlanDto,
  getCampaignTouchPlanRecipientDto,
  getCampaignTouchPlanRecipientReviewDto,
  getCampaignTouchPlanReviewDto,
  listCampaignsDto,
  listCampaignTouchPlanRecipientsDto,
  listCampaignTouchPlansDto,
  saveCampaignTouchPlanRecipientMessageDto,
  type CampaignDetail,
  type CampaignFilter,
  type CampaignTouchPlan,
  type CampaignTouchPlanDetail,
  type CampaignTouchPlanRecipient,
  type CampaignTouchPlanRecipientPage,
  type CampaignTouchPlanRecipientReview,
  type CampaignTouchPlanReview,
} from '../../api/admin';
import { confirmBox, toast } from '../../shared/ui/feedback';
import { esc } from './util';

type CampaignPage = {
  campaign: CampaignDetail;
  plans: CampaignTouchPlan[];
  plan?: CampaignTouchPlanDetail;
  review?: CampaignTouchPlanReview;
  recipientPage?: CampaignTouchPlanRecipientPage;
  recipient?: CampaignTouchPlanRecipient;
  recipientReview?: CampaignTouchPlanRecipientReview | null;
};

const button = 'height:30px;padding:0 11px;border:1px solid #D0D5DD;border-radius:6px;background:#fff;color:#344054;cursor:pointer';
const primaryButton = button + ';border-color:#3370ff;background:#3370ff;color:#fff';
const blocked = '<div style="padding:12px;border:1px solid #F5D6A7;border-radius:8px;background:#FFF9F0;color:#8F5A16;font-size:13px;line-height:20px"><strong>backend_blocked</strong>：当前真实 OpenAPI 未提供该读取契约，页面不会使用 Mock、Seed 或旧页面结果补齐。</div>';

const query = (): URLSearchParams => new URLSearchParams(location.search);
const goto = (campaignCode?: string, planID?: string, customerID?: number): void => {
  const params = new URLSearchParams();
  if (campaignCode) params.set('campaign', campaignCode);
  if (planID) params.set('plan', planID);
  if (customerID) params.set('recipient', String(customerID));
  location.href = `campaigns.html${params.size ? `?${params.toString()}` : ''}`;
};
const status = (value: string): string => `<span style="display:inline-flex;padding:2px 8px;border-radius:999px;background:#F2F4F7;color:#475467;font-size:12px">${esc(value)}</span>`;
const safety = '<p style="margin:0;color:#8F5A16;font-size:12px">只读快照/本地审核不证明 Provider 调用、外部发送或送达。</p>';

function shell(title: string, body: string): string {
  return `<div style="padding:20px;display:grid;gap:16px;align-content:start"><div><div style="font-size:12px;color:#8F959E">运营 / Cloud Campaign</div><h1 style="margin:4px 0 0;font-size:20px">${title}</h1></div>${body}</div>`;
}

function listHtml(rows: CampaignDetail[], filter: CampaignFilter): string {
  const rowHtml = rows.map((item) => `<tr><td style="padding:10px 12px;border-bottom:1px solid #EEF0F3"><button data-campaign="${esc(item.code)}" style="border:0;background:transparent;color:#1849A9;cursor:pointer;font-weight:600">${esc(item.name)}</button><div style="margin-top:3px;color:#8F959E;font-family:ui-monospace,Menlo,monospace;font-size:12px">${esc(item.code)}</div></td><td style="padding:10px 12px;border-bottom:1px solid #EEF0F3">${status(item.approvalStatus)}</td><td style="padding:10px 12px;border-bottom:1px solid #EEF0F3">${status(item.runtimeStatus)}</td><td style="padding:10px 12px;border-bottom:1px solid #EEF0F3">v${item.version}</td><td style="padding:10px 12px;border-bottom:1px solid #EEF0F3">${esc(item.updatedAt)}</td></tr>`).join('') || '<tr><td colspan="5" style="padding:24px;text-align:center;color:#8F959E">当前筛选下没有 Campaign</td></tr>';
  return shell('Campaign 本地生命周期', `<div style="display:flex;gap:10px;flex-wrap:wrap"><label>审核状态 <select id="campaign-approval"><option value="">全部</option>${['draft', 'approved', 'rejected'].map((value) => `<option value="${value}"${filter.approvalStatus === value ? ' selected' : ''}>${value}</option>`).join('')}</select></label><label>运行状态 <select id="campaign-runtime"><option value="">全部</option>${['idle', 'planned', 'paused'].map((value) => `<option value="${value}"${filter.runtimeStatus === value ? ' selected' : ''}>${value}</option>`).join('')}</select></label><button id="campaign-refresh" style="${button}">刷新</button></div><div style="border:1px solid #DEE0E3;border-radius:8px;overflow:hidden"><table style="width:100%;border-collapse:collapse"><thead><tr style="background:#FAFAFB;color:#667085;text-align:left"><th style="padding:10px 12px">Campaign</th><th style="padding:10px 12px">审核</th><th style="padding:10px 12px">运行</th><th style="padding:10px 12px">版本</th><th style="padding:10px 12px">更新时间</th></tr></thead><tbody>${rowHtml}</tbody></table></div><div style="display:grid;gap:10px"><div><h2 style="margin:0 0 6px;font-size:15px">Campaign 命中成员</h2>${blocked.replace('该读取契约', '按成员状态读取 Campaign 成员与总数的')}</div><div><h2 style="margin:0 0 6px;font-size:15px">可观察性与审计筛选</h2>${blocked.replace('该读取契约', '按 trace_id/session_id 刷新 observability 与 audit 的 JSON')}</div></div>`);
}

function planRows(plans: CampaignTouchPlan[]): string {
  return plans.map((plan) => `<tr><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3"><button data-plan="${esc(plan.id)}" style="border:0;background:transparent;color:#1849A9;font-family:ui-monospace,Menlo,monospace;cursor:pointer">${esc(plan.id)}</button></td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(plan.sourceKind)}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${plan.targetCount}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${plan.contentStepCount}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(plan.createdAt)}</td></tr>`).join('') || '<tr><td colspan="5" style="padding:20px;text-align:center;color:#8F959E">暂无已冻结的本地 touch plan</td></tr>';
}

function campaignHtml(page: CampaignPage): string {
  const campaign = page.campaign;
  const steps = campaign.steps.map((step) => `<li style="margin:5px 0"><strong>第 ${step.index} 步 · 延迟 ${step.delayMinutes} 分钟</strong><div style="margin-top:3px;white-space:pre-wrap">${esc(step.content)}</div></li>`).join('') || '<li>无步骤</li>';
  return shell(esc(campaign.name), `<div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap"><button id="campaign-back" style="${button}">返回列表</button><span style="font-family:ui-monospace,Menlo,monospace;color:#667085">${esc(campaign.code)}</span>${status(campaign.approvalStatus)}${status(campaign.runtimeStatus)}<span>v${campaign.version}</span><button id="campaign-delete" style="${button};border-color:#F5B7B1;color:#B42318">删除本地 Campaign</button></div><div style="display:grid;grid-template-columns:minmax(280px,1fr) minmax(360px,1fr);gap:14px"><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px"><h2 style="margin:0 0 8px;font-size:15px">Campaign 详情</h2><ol style="margin:0;padding-left:20px">${steps}</ol>${safety}</section><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px"><h2 style="margin:0 0 8px;font-size:15px">运营计划（已冻结 touch plan）</h2><table style="width:100%;border-collapse:collapse"><thead><tr style="color:#667085;text-align:left"><th>计划</th><th>来源</th><th>目标</th><th>步骤</th><th>创建时间</th></tr></thead><tbody>${planRows(page.plans)}</tbody></table>${safety}</section></div>`);
}

function planHtml(page: CampaignPage): string {
  const plan = page.plan!;
  const review = page.review!;
  const recipients = page.recipientPage?.items || [];
  const steps = plan.steps.map((step) => `<li><strong>第 ${step.index} 步 · 延迟 ${step.delayMinutes} 分钟</strong><div style="white-space:pre-wrap">${esc(step.content)}</div></li>`).join('') || '<li>无步骤</li>';
  const recipientRows = recipients.map((recipient) => `<tr><td style="padding:8px;border-bottom:1px solid #EEF0F3;font-family:ui-monospace,Menlo,monospace">${recipient.customerID}</td><td style="padding:8px;border-bottom:1px solid #EEF0F3"><button data-recipient="${recipient.customerID}" style="${button}">读取范围详情</button></td></tr>`).join('') || '<tr><td colspan="2" style="padding:16px;color:#8F959E">没有可展示的收件人</td></tr>';
  const recipientDetail = page.recipient ? `<div style="display:grid;gap:10px;padding:10px;border:1px solid #D6E4FF;border-radius:6px;background:#F5F8FF"><div>已验证当前 plan 范围内 canonical_customer_id：<strong>${page.recipient.customerID}</strong>。当前契约不含昵称、成员状态或消息任务。</div><label style="display:grid;gap:5px">单客户消息覆盖<textarea id="recipient-message" maxlength="4000" rows="4" style="padding:8px;border:1px solid #D0D5DD;border-radius:6px">${esc(page.recipientReview?.messageOverride || '')}</textarea></label><div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap"><button id="recipient-message-save" style="${button}">保存本地消息</button>${page.review?.status === 'pending_review' && (!page.recipientReview || page.recipientReview.status === 'pending_review') ? `<button id="recipient-review-approve" style="${primaryButton}">批准该客户</button><button id="recipient-review-reject" style="${button};border-color:#F5B7B1;color:#B42318">拒绝该客户</button>` : ''}${page.recipientReview ? status(page.recipientReview.status) + `<span style="font-size:12px;color:#667085">v${page.recipientReview.version}</span>` : '<span style="font-size:12px;color:#667085">尚无本地单客户审核记录</span>'}</div><p style="margin:0;color:#8F5A16;font-size:12px">保存、批准、拒绝均只写本地 review，不会创建发送任务或调用 Provider。</p></div>` : '';
  const actions = review.status === 'pending_review' ? `<button id="plan-approve" style="${primaryButton}">批准本地审核</button><button id="plan-reject" style="${button};border-color:#F5B7B1;color:#B42318">拒绝本地审核</button>` : `<span style="color:#8F959E;font-size:13px">当前状态不是 pending_review，不能提交批准/拒绝。</span>`;
  const more = page.recipientPage?.nextCursor ? `<button id="recipient-more" style="${button};margin-top:10px">加载更多目标人员</button>` : '';
  return shell('Touch plan 本地审核', `<div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap"><button id="plan-back" style="${button}">返回 Campaign</button><span style="font-family:ui-monospace,Menlo,monospace;color:#667085">${esc(plan.id)}</span>${status(review.status)}<span>审核版本 v${review.version}</span>${review.handoffStatus ? status(review.handoffStatus) : ''}</div><div style="padding:12px;border:1px solid #D6E4FF;border-radius:8px;background:#F5F8FF;color:#1849A9;font-size:13px">批准只写入本地 touch-plan review；即使生成 handoff 也仍是 pending outbound acceptance，不等于发送或送达。</div><div style="display:grid;grid-template-columns:minmax(280px,1fr) minmax(360px,1fr);gap:14px"><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px"><h2 style="margin:0 0 8px;font-size:15px">计划详情</h2><p>来源：${esc(plan.sourceKind)} · 目标：${plan.targetCount} · Campaign 版本：v${plan.campaignVersion}</p><ol style="padding-left:20px">${steps}</ol><div style="display:flex;gap:8px;flex-wrap:wrap">${actions}</div></section><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px"><h2 style="margin:0 0 8px;font-size:15px">目标人员（canonical OneID）</h2><table style="width:100%;border-collapse:collapse"><thead><tr style="text-align:left;color:#667085"><th style="padding:8px">OneID</th><th style="padding:8px">范围详情</th></tr></thead><tbody>${recipientRows}</tbody></table>${more}${recipientDetail}</section></div>`);
}

async function loadList(stage: HTMLElement, filter: CampaignFilter = {}): Promise<void> {
  const rows = await listCampaignsDto(filter);
  stage.innerHTML = listHtml(rows.map((item) => ({ ...item, steps: [] })), filter);
  stage.querySelectorAll<HTMLButtonElement>('[data-campaign]').forEach((element) => element.addEventListener('click', () => goto(element.dataset.campaign)));
  stage.querySelector<HTMLButtonElement>('#campaign-refresh')?.addEventListener('click', () => {
    const approval = (stage.querySelector<HTMLSelectElement>('#campaign-approval')?.value || '') as CampaignFilter['approvalStatus'] | '';
    const runtime = (stage.querySelector<HTMLSelectElement>('#campaign-runtime')?.value || '') as CampaignFilter['runtimeStatus'] | '';
    void loadList(stage, { approvalStatus: approval || undefined, runtimeStatus: runtime || undefined }).catch((error) => showError(stage, error));
  });
}

function showError(stage: HTMLElement, error: unknown): void {
  stage.innerHTML = shell('Campaign 本地工作区', `<div style="padding:14px;border:1px solid #F2B8B5;border-radius:8px;background:#FFF1F0;color:#B42318">${esc(error instanceof Error ? error.message : '读取失败')}</div><button id="campaign-retry" style="${button}">返回并重试</button>`);
  stage.querySelector<HTMLButtonElement>('#campaign-retry')?.addEventListener('click', () => goto());
}

async function loadCampaign(stage: HTMLElement, campaignCode: string, planID?: string, customerID?: number): Promise<void> {
  const [campaign, plans] = await Promise.all([getCampaignDto(campaignCode), listCampaignTouchPlansDto(campaignCode)]);
  if (!planID) {
    stage.innerHTML = campaignHtml({ campaign, plans });
    stage.querySelector<HTMLButtonElement>('#campaign-back')?.addEventListener('click', () => goto());
    stage.querySelectorAll<HTMLButtonElement>('[data-plan]').forEach((element) => element.addEventListener('click', () => goto(campaignCode, element.dataset.plan)));
    stage.querySelector<HTMLButtonElement>('#campaign-delete')?.addEventListener('click', () => confirmBox('删除本地 Campaign', `确认删除「${campaign.name}」？仅当服务端允许草稿且空闲时才会删除；不会清除任何外部历史任务。`, '确认删除', true, () => {
      void deleteCampaignDto(campaignCode).then(() => { toast('本地 Campaign 已删除'); goto(); }).catch((error) => toast(error instanceof Error ? error.message : 'Campaign 删除失败', true));
    }));
    return;
  }
  const [plan, review, recipientPage] = await Promise.all([getCampaignTouchPlanDto(campaignCode, planID), getCampaignTouchPlanReviewDto(campaignCode, planID), listCampaignTouchPlanRecipientsDto(campaignCode, planID)]);
  const [recipient, recipientReview] = customerID == null ? [undefined, undefined] as const : await Promise.all([getCampaignTouchPlanRecipientDto(campaignCode, planID, customerID), getCampaignTouchPlanRecipientReviewDto(campaignCode, planID, customerID)] as const);
  const renderPlan = (nextPage: CampaignTouchPlanRecipientPage): void => {
    stage.innerHTML = planHtml({ campaign, plans, plan, review, recipientPage: nextPage, recipient, recipientReview });
    stage.querySelector<HTMLButtonElement>('#plan-back')?.addEventListener('click', () => goto(campaignCode));
    stage.querySelectorAll<HTMLButtonElement>('[data-recipient]').forEach((element) => element.addEventListener('click', () => goto(campaignCode, planID, Number(element.dataset.recipient))));
    stage.querySelector<HTMLButtonElement>('#recipient-more')?.addEventListener('click', () => {
      void listCampaignTouchPlanRecipientsDto(campaignCode, planID, nextPage.nextCursor || undefined).then((more) => renderPlan({ items: [...nextPage.items, ...more.items], nextCursor: more.nextCursor })).catch((error) => toast(error instanceof Error ? error.message : '目标人员读取失败', true));
    });
    (['approve', 'reject'] as const).forEach((operation) => stage.querySelector<HTMLButtonElement>(`#plan-${operation}`)?.addEventListener('click', () => confirmBox(`${operation === 'approve' ? '批准' : '拒绝'}本地审核`, '该操作仅写入本地 review，不会调用 Provider 或发送消息。', `确认${operation === 'approve' ? '批准' : '拒绝'}`, operation === 'reject', () => {
      void decideCampaignTouchPlanReviewDto(campaignCode, planID, operation).then(() => { toast('本地审核状态已更新'); goto(campaignCode, planID); }).catch((error) => toast(error instanceof Error ? error.message : '本地审核失败', true));
    })));
    stage.querySelector<HTMLButtonElement>('#recipient-message-save')?.addEventListener('click', () => {
      const message = stage.querySelector<HTMLTextAreaElement>('#recipient-message')?.value || '';
      void saveCampaignTouchPlanRecipientMessageDto(campaignCode, planID, customerID!, message).then(() => { toast('单客户本地消息已保存'); goto(campaignCode, planID, customerID); }).catch((error) => toast(error instanceof Error ? error.message : '单客户消息保存失败', true));
    });
    (['approve', 'reject'] as const).forEach((operation) => stage.querySelector<HTMLButtonElement>(`#recipient-review-${operation}`)?.addEventListener('click', () => confirmBox(`${operation === 'approve' ? '批准' : '拒绝'}该客户`, '该操作仅写入当前 touch plan 的本地单客户 review，不会发送消息。', `确认${operation === 'approve' ? '批准' : '拒绝'}`, operation === 'reject', () => {
      void decideCampaignTouchPlanRecipientReviewDto(campaignCode, planID, customerID!, operation).then(() => { toast('单客户本地审核已更新'); goto(campaignCode, planID, customerID); }).catch((error) => toast(error instanceof Error ? error.message : '单客户审核失败', true));
    })));
  };
  renderPlan(recipientPage);
}

export async function mountCampaignWorkspace(stage: HTMLElement): Promise<void> {
  const params = query();
  const campaignCode = params.get('campaign') || undefined;
  const planID = params.get('plan') || undefined;
  const rawCustomerID = Number(params.get('recipient') || '');
  const customerID = Number.isSafeInteger(rawCustomerID) && rawCustomerID > 0 ? rawCustomerID : undefined;
  if (planID && !campaignCode) throw new Error('缺少 Campaign code，拒绝读取未限定范围的 touch plan');
  if (campaignCode) return loadCampaign(stage, campaignCode, planID, customerID);
  return loadList(stage);
}
