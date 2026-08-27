import {
  getCouponDto,
  getServicePeriodMemberGridMetaDto,
  getServicePeriodMemberDto,
  listCouponClaimsDto,
  listCouponProductOptionsDto,
  queryServicePeriodMemberGridDto,
  readCouponSharePath,
  saveCouponDto,
  createServicePeriodMemberGridCollaboratorDto,
  deleteServicePeriodMemberGridCollaboratorDto,
  updateServicePeriodMemberFieldsDto,
  updateServicePeriodMemberGridCollaboratorDto,
  setMemberGridExternalShareDto,
  type CouponClaimPage,
  type CouponProductOptionPage,
  type CouponWriteInput,
  type MemberGridSourceFilter,
  type MemberGridGroupBy,
  type MemberGridSort,
  type MemberGridState,
  type MemberGridViewID,
  type ServicePeriodMemberDetail,
  type ServicePeriodMemberGridPage,
  type ServicePeriodMemberGridMeta,
} from '../../api/admin';
import type { Coupon } from '../../shared/api/types';
import { toast } from '../../shared/ui/feedback';
import { copyText, esc } from './util';

const control = 'height:34px;border:1px solid #DEE0E3;border-radius:6px;padding:0 10px;background:#fff;color:#344054;font-size:13px';
const button = 'height:32px;padding:0 12px;border:1px solid #D0D5DD;border-radius:6px;background:#fff;color:#344054;font-size:13px;cursor:pointer';
const primary = button + ';border-color:#3370ff;background:#3370ff;color:#fff';
type CouponDraft = {
  id?: number;
  name: string;
  discount: string;
  totalIssueLimit: string;
  perUserIssueLimit: string;
  claimStartsAt: string;
  claimEndsAt: string;
  validityMode: 'fixed_range' | 'relative_days';
  useStartsAt: string;
  useEndsAt: string;
  relativeValidityDays: string;
  instructions: string;
  targetRefs: string;
};

const queryID = (): number | undefined => {
  const raw = new URLSearchParams(location.search).get('id');
  if (raw == null) return undefined;
  const id = Number(raw);
  if (!Number.isSafeInteger(id) || id < 1) throw new Error('优惠券或周期商品 ID 无效');
  return id;
};
const dateInput = (value: string | null | undefined): string => value ? value.slice(0, 16) : '';
const money = (minor: number): string => `¥${(minor / 100).toFixed(2)}`;
const refs = (value: string): string[] => value.split(/[\s,，]+/).map((item) => item.trim()).filter(Boolean);
const valueOf = (stage: HTMLElement, id: string): string => (stage.querySelector<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(`#${id}`)?.value || '').trim();

function draftFrom(coupon?: Coupon): CouponDraft {
  return {
    id: coupon?.resourceId,
    name: coupon?.name || '',
    discount: coupon ? ((coupon.discountAmountTotal || 0) / 100).toFixed(2) : '',
    totalIssueLimit: coupon?.totalIssueLimit ? String(coupon.totalIssueLimit) : '',
    perUserIssueLimit: coupon?.perUserIssueLimit ? String(coupon.perUserIssueLimit) : '1',
    claimStartsAt: dateInput(coupon?.claimStartsAt),
    claimEndsAt: dateInput(coupon?.claimEndsAt),
    validityMode: coupon?.validityMode || 'relative_days',
    useStartsAt: dateInput(coupon?.useStartsAt),
    useEndsAt: dateInput(coupon?.useEndsAt),
    relativeValidityDays: coupon?.relativeValidityDays ? String(coupon.relativeValidityDays) : '',
    instructions: coupon?.instructions || '',
    targetRefs: (coupon?.targetRefs || []).join('\n'),
  };
}

function couponWrite(draft: CouponDraft): CouponWriteInput {
  const totalIssueLimit = Number(draft.totalIssueLimit);
  const perUserIssueLimit = Number(draft.perUserIssueLimit);
  const targetRefs = refs(draft.targetRefs);
  if (!draft.name || !/^\d+(\.\d{1,2})?$/.test(draft.discount)) throw new Error('请填写名称与最多两位小数的非负减免金额');
  if (!Number.isSafeInteger(totalIssueLimit) || totalIssueLimit < 1 || !Number.isSafeInteger(perUserIssueLimit) || perUserIssueLimit < 1) throw new Error('发行总量和单用户限领必须为正整数');
  if (!draft.claimStartsAt || !draft.claimEndsAt || new Date(draft.claimStartsAt) >= new Date(draft.claimEndsAt)) throw new Error('领取时间范围无效');
  if (!targetRefs.length) throw new Error('至少选择一个服务端商品引用');
  if (draft.validityMode === 'fixed_range' && (!draft.useStartsAt || !draft.useEndsAt || new Date(draft.useStartsAt) >= new Date(draft.useEndsAt))) throw new Error('固定使用时间范围无效');
  if (draft.validityMode === 'relative_days' && (!Number.isSafeInteger(Number(draft.relativeValidityDays)) || Number(draft.relativeValidityDays) < 1)) throw new Error('相对有效天数必须为正整数');
  return { id: draft.id, name: draft.name, discount: draft.discount, totalIssueLimit, perUserIssueLimit, claimStartsAt: draft.claimStartsAt, claimEndsAt: draft.claimEndsAt, validityMode: draft.validityMode, useStartsAt: draft.useStartsAt || undefined, useEndsAt: draft.useEndsAt || undefined, relativeValidityDays: draft.relativeValidityDays ? Number(draft.relativeValidityDays) : undefined, instructions: draft.instructions, targetRefs };
}

function couponFormHtml(draft: CouponDraft, options: CouponProductOptionPage, optionQuery: string, optionType: 'all' | 'standard_product' | 'service_period'): string {
  const previous = options.offset > 0 ? `<button id="option-previous" style="${button}">上一页</button>` : '';
  const next = options.offset + options.items.length < options.total ? `<button id="option-next" style="${button}">下一页</button>` : '';
  const optionRows = options.items.map((item) => `<tr><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3"><strong>${esc(item.name)}</strong><div style="margin-top:3px;font:12px ui-monospace,Menlo,monospace;color:#667085">${esc(item.targetRef)}</div></td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${money(item.priceMinor)} ${esc(item.currency)}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3;text-align:right"><button data-target-ref="${esc(item.targetRef)}" style="${button}">加入适用范围</button></td></tr>`).join('') || '<tr><td colspan="3" style="padding:18px;text-align:center;color:#8F959E">当前筛选没有服务端返回的商品选项</td></tr>';
  const fixed = draft.validityMode === 'fixed_range';
  return `<div style="padding:20px;display:grid;gap:14px;align-content:start"><div style="display:flex;justify-content:space-between;gap:12px;align-items:center"><div><div style="font-size:12px;color:#8F959E">交易 / 优惠券</div><h1 style="margin:4px 0 0;font-size:20px">${draft.id ? '编辑优惠券' : '创建优惠券'}</h1></div><div style="display:flex;gap:8px"><button id="coupon-back" style="${button}">返回列表</button><button id="coupon-save-draft" style="${button}">保存草稿</button><button id="coupon-save-publish" style="${primary}">保存并发布</button></div></div><section style="max-width:980px;padding:16px;border:1px solid #DEE0E3;border-radius:8px;background:#fff"><h2 style="margin:0 0 14px;font-size:15px">OpenAPI 优惠券规则</h2><div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px"><label>名称<input id="coupon-name" value="${esc(draft.name)}" style="${control};width:100%;box-sizing:border-box"></label><label>减免金额（元）<input id="coupon-discount" value="${esc(draft.discount)}" inputmode="decimal" style="${control};width:100%;box-sizing:border-box"></label><label>发行总量<input id="coupon-total" value="${esc(draft.totalIssueLimit)}" type="number" min="1" style="${control};width:100%;box-sizing:border-box"></label><label>单用户限领<input id="coupon-per-user" value="${esc(draft.perUserIssueLimit)}" type="number" min="1" style="${control};width:100%;box-sizing:border-box"></label><label>领取开始<input id="coupon-claim-start" value="${esc(draft.claimStartsAt)}" type="datetime-local" style="${control};width:100%;box-sizing:border-box"></label><label>领取结束<input id="coupon-claim-end" value="${esc(draft.claimEndsAt)}" type="datetime-local" style="${control};width:100%;box-sizing:border-box"></label><label>有效期模式<select id="coupon-validity" style="${control};width:100%;box-sizing:border-box"><option value="relative_days"${fixed ? '' : ' selected'}>领取后相对天数</option><option value="fixed_range"${fixed ? ' selected' : ''}>固定时间范围</option></select></label><label>领取后有效天数<input id="coupon-relative-days" value="${esc(draft.relativeValidityDays)}" type="number" min="1" style="${control};width:100%;box-sizing:border-box"></label><label>固定使用开始<input id="coupon-use-start" value="${esc(draft.useStartsAt)}" type="datetime-local" style="${control};width:100%;box-sizing:border-box"></label><label>固定使用结束<input id="coupon-use-end" value="${esc(draft.useEndsAt)}" type="datetime-local" style="${control};width:100%;box-sizing:border-box"></label><label style="grid-column:1/-1">适用商品引用<textarea id="coupon-target-refs" rows="3" style="${control};height:auto;width:100%;box-sizing:border-box;padding:8px">${esc(draft.targetRefs)}</textarea></label><label style="grid-column:1/-1">使用说明<textarea id="coupon-instructions" rows="4" style="${control};height:auto;width:100%;box-sizing:border-box;padding:8px">${esc(draft.instructions)}</textarea></label></div></section><section style="max-width:980px;border:1px solid #DEE0E3;border-radius:8px;background:#fff;overflow:hidden"><div style="padding:14px 16px;border-bottom:1px solid #EEF0F3"><h2 style="margin:0 0 10px;font-size:15px">适用商品选择</h2><div style="display:flex;gap:8px;flex-wrap:wrap"><input id="option-query" value="${esc(optionQuery)}" placeholder="按商品名称搜索" style="${control};min-width:220px"><select id="option-type" style="${control}"><option value="all"${optionType === 'all' ? ' selected' : ''}>全部类型</option><option value="standard_product"${optionType === 'standard_product' ? ' selected' : ''}>普通商品</option><option value="service_period"${optionType === 'service_period' ? ' selected' : ''}>周期商品</option></select><button id="option-search" style="${button}">查询</button></div><p style="margin:10px 0 0;color:#8F5A16;font-size:12px">仅使用服务端 target_ref；周期商品筛选由当前 OpenAPI 返回空集，不在浏览器伪造可选项。</p></div><table style="width:100%;border-collapse:collapse"><thead><tr style="background:#FAFAFB;text-align:left;color:#667085"><th style="padding:9px 12px">商品</th><th style="padding:9px 12px">价格</th><th style="padding:9px 12px;text-align:right">操作</th></tr></thead><tbody>${optionRows}</tbody></table><div style="padding:12px 16px;display:flex;justify-content:space-between;align-items:center"><span style="font-size:12px;color:#667085">共 ${options.total} 项，当前 ${options.offset + (options.items.length ? 1 : 0)}–${options.offset + options.items.length}</span><div style="display:flex;gap:8px">${previous}${next}</div></div></section><div style="max-width:980px;padding:12px;border:1px solid #D6E4FF;border-radius:8px;background:#F5F8FF;color:#1849A9;font-size:13px;line-height:20px">保存仅调用当前 Coupon OpenAPI；不证明库存、领取、核销、支付或公开链接真实可用。</div></div>`;
}

export async function mountCouponForm(stage: HTMLElement): Promise<void> {
  const id = queryID();
  let draft = draftFrom(id ? await getCouponDto(id) : undefined);
  let optionQuery = '';
  let optionType: 'all' | 'standard_product' | 'service_period' = 'all';
  let options = await listCouponProductOptionsDto({ productType: optionType, limit: 20, offset: 0 });
  const snapshot = (): void => {
    draft = { ...draft, name: valueOf(stage, 'coupon-name'), discount: valueOf(stage, 'coupon-discount'), totalIssueLimit: valueOf(stage, 'coupon-total'), perUserIssueLimit: valueOf(stage, 'coupon-per-user'), claimStartsAt: valueOf(stage, 'coupon-claim-start'), claimEndsAt: valueOf(stage, 'coupon-claim-end'), validityMode: valueOf(stage, 'coupon-validity') === 'fixed_range' ? 'fixed_range' : 'relative_days', useStartsAt: valueOf(stage, 'coupon-use-start'), useEndsAt: valueOf(stage, 'coupon-use-end'), relativeValidityDays: valueOf(stage, 'coupon-relative-days'), targetRefs: valueOf(stage, 'coupon-target-refs'), instructions: valueOf(stage, 'coupon-instructions') };
  };
  const render = (): void => {
    stage.innerHTML = couponFormHtml(draft, options, optionQuery, optionType);
    stage.querySelector<HTMLButtonElement>('#coupon-back')?.addEventListener('click', () => { location.href = 'coupons.html'; });
    stage.querySelector<HTMLButtonElement>('#option-search')?.addEventListener('click', () => {
      snapshot(); optionQuery = valueOf(stage, 'option-query'); const type = valueOf(stage, 'option-type'); optionType = type === 'standard_product' || type === 'service_period' ? type : 'all';
      void listCouponProductOptionsDto({ q: optionQuery || undefined, productType: optionType, limit: 20, offset: 0 }).then((page) => { options = page; render(); }).catch((error) => toast(error instanceof Error ? error.message : '商品选项读取失败', true));
    });
    stage.querySelector<HTMLButtonElement>('#option-previous')?.addEventListener('click', () => { snapshot(); void listCouponProductOptionsDto({ q: optionQuery || undefined, productType: optionType, limit: 20, offset: Math.max(0, options.offset - options.limit) }).then((page) => { options = page; render(); }).catch((error) => toast(error instanceof Error ? error.message : '商品选项读取失败', true)); });
    stage.querySelector<HTMLButtonElement>('#option-next')?.addEventListener('click', () => { snapshot(); void listCouponProductOptionsDto({ q: optionQuery || undefined, productType: optionType, limit: 20, offset: options.offset + options.limit }).then((page) => { options = page; render(); }).catch((error) => toast(error instanceof Error ? error.message : '商品选项读取失败', true)); });
    stage.querySelectorAll<HTMLButtonElement>('[data-target-ref]').forEach((element) => element.addEventListener('click', () => { snapshot(); const target = element.dataset.targetRef || ''; if (target && !refs(draft.targetRefs).includes(target)) draft.targetRefs = [...refs(draft.targetRefs), target].join('\n'); render(); }));
    ([['#coupon-save-draft', false], ['#coupon-save-publish', true]] as const).forEach(([selector, publish]) => stage.querySelector<HTMLButtonElement>(selector)?.addEventListener('click', () => {
      snapshot();
      let input: CouponWriteInput;
      try { input = couponWrite(draft); } catch (error) { toast(error instanceof Error ? error.message : '优惠券表单无效', true); return; }
      void saveCouponDto(input, publish).then((saved) => { toast(publish ? '优惠券已保存并发布' : '优惠券草稿已保存'); location.href = `couponForm.html?id=${saved.resourceId}`; }).catch((error) => toast(error instanceof Error ? error.message : '优惠券保存失败', true));
    }));
  };
  render();
}

function couponDataHtml(coupon: Coupon, claims: CouponClaimPage, sharePath: string): string {
  const rows = claims.items.map((claim) => `<tr><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3;font:12px ui-monospace,Menlo,monospace">${esc(claim.claimRef)}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(claim.status)}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(claim.claimedAt)}</td></tr>`).join('') || '<tr><td colspan="3" style="padding:20px;text-align:center;color:#8F959E">当前页没有领取记录</td></tr>';
  const previous = claims.offset > 0 ? `<button id="claim-previous" style="${button}">上一页</button>` : '';
  const next = claims.offset + claims.items.length < claims.total ? `<button id="claim-next" style="${button}">下一页</button>` : '';
  const share = sharePath ? `<div style="display:flex;gap:8px;align-items:center"><code style="font-size:12px;color:#344054">${esc(sharePath)}</code><button id="coupon-copy-share" style="${button}">复制领取链接</button></div>` : '<button id="coupon-load-share" style="' + button + '">读取领取链接</button>';
  return `<div style="padding:20px;display:grid;gap:14px;align-content:start"><div style="display:flex;justify-content:space-between;gap:12px;align-items:center"><div><div style="font-size:12px;color:#8F959E">交易 / 优惠券</div><h1 style="margin:4px 0 0;font-size:20px">${esc(coupon.name)} · 领取数据</h1></div><div style="display:flex;gap:8px"><button id="coupon-data-back" style="${button}">返回列表</button><button id="coupon-data-edit" style="${primary}">编辑配置</button></div></div><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px;background:#fff;display:flex;gap:28px;flex-wrap:wrap"><span>状态：${esc(coupon.status)}</span><span>面额：${esc(coupon.off)}</span><span>发行 / 已领取：${esc(coupon.issue)}</span><span>适用范围：${esc(coupon.scope)}</span></section><section style="border:1px solid #DEE0E3;border-radius:8px;background:#fff;overflow:hidden"><div style="padding:14px 16px;border-bottom:1px solid #EEF0F3;display:flex;justify-content:space-between;align-items:center"><h2 style="margin:0;font-size:15px">脱敏领取记录</h2><span style="font-size:12px;color:#667085">共 ${claims.total} 条</span></div><table style="width:100%;border-collapse:collapse"><thead><tr style="background:#FAFAFB;text-align:left;color:#667085"><th style="padding:9px 12px">领取引用</th><th style="padding:9px 12px">状态</th><th style="padding:9px 12px">领取时间</th></tr></thead><tbody>${rows}</tbody></table><div style="padding:12px 16px;display:flex;justify-content:flex-end;gap:8px">${previous}${next}</div></section><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px;background:#fff"><h2 style="margin:0 0 10px;font-size:15px">优惠券分享</h2>${share}<p style="margin:10px 0 0;font-size:12px;color:#8F5A16">返回 URL 仅可复制，不证明公开链接、二维码、领取或核销真实有效。</p></section></div>`;
}

export async function mountCouponData(stage: HTMLElement): Promise<void> {
  const id = queryID();
  if (!id) throw new Error('领取数据需要有效优惠券 ID');
  const coupon = await getCouponDto(id);
  let claims = await listCouponClaimsDto(id, { limit: 50, offset: 0 });
  let sharePath = '';
  const render = (): void => {
    stage.innerHTML = couponDataHtml(coupon, claims, sharePath);
    stage.querySelector<HTMLButtonElement>('#coupon-data-back')?.addEventListener('click', () => { location.href = 'coupons.html'; });
    stage.querySelector<HTMLButtonElement>('#coupon-data-edit')?.addEventListener('click', () => { location.href = `couponForm.html?id=${id}`; });
    stage.querySelector<HTMLButtonElement>('#claim-previous')?.addEventListener('click', () => { void listCouponClaimsDto(id, { limit: claims.limit, offset: Math.max(0, claims.offset - claims.limit) }).then((page) => { claims = page; render(); }).catch((error) => toast(error instanceof Error ? error.message : '领取记录读取失败', true)); });
    stage.querySelector<HTMLButtonElement>('#claim-next')?.addEventListener('click', () => { void listCouponClaimsDto(id, { limit: claims.limit, offset: claims.offset + claims.limit }).then((page) => { claims = page; render(); }).catch((error) => toast(error instanceof Error ? error.message : '领取记录读取失败', true)); });
    stage.querySelector<HTMLButtonElement>('#coupon-load-share')?.addEventListener('click', () => { void readCouponSharePath(id).then((path) => { sharePath = path; render(); }).catch((error) => toast(error instanceof Error ? error.message : '分享地址读取失败', true)); });
    stage.querySelector<HTMLButtonElement>('#coupon-copy-share')?.addEventListener('click', () => copyText(sharePath, toast));
  };
  render();
}

type MemberGridFilters = { state: MemberGridState; source: MemberGridSourceFilter; sort: MemberGridSort; groupBy: MemberGridGroupBy; viewId: MemberGridViewID };

const memberStateLabel = (state: string): string => ({ active: '有效', expired: '已过期', removed: '已移除', all: '全部' }[state] || state);
const memberSourceLabel = (source: string): string => ({ manual: '手动', paid_order: '已支付订单' }[source] || source);
const memberTime = (value: string | null): string => value ? value.replace('T', ' ').replace('Z', '') : '—';

function memberGridHtml(grid: ServicePeriodMemberGridMeta, page: ServicePeriodMemberGridPage, filters: MemberGridFilters, cursorDepth: number, detail?: ServicePeriodMemberDetail, sharePath = ''): string {
  const columns = grid.columns.map((column) => `<tr><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3;font-family:ui-monospace,Menlo,monospace">${esc(column.key)}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(column.label)}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(column.type)}${column.nullable ? ' · nullable' : ''}</td></tr>`).join('');
  const views = grid.views.map((view) => `<li>${esc(view.name)} <span style="color:#8F959E">（内置只读）</span></li>`).join('');
  let previousState = '';
  const rows = page.rows.map((row) => {
    const group = filters.groupBy === 'state' && row.state !== previousState ? `<tr data-member-group="${esc(row.state)}"><td colspan="5" style="padding:7px 12px;background:#F5F7FA;color:#667085;font-size:12px;font-weight:600">${esc(memberStateLabel(row.state))}</td></tr>` : '';
    previousState = row.state;
    return group + `<tr><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3"><strong>${esc(row.displayName)}</strong><div style="margin-top:3px;font:12px ui-monospace,Menlo,monospace;color:#667085">${esc(row.memberRef)}</div></td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(memberStateLabel(row.state))}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(memberSourceLabel(row.source))}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">${esc(memberTime(row.startsAt))}<br>${esc(memberTime(row.expiresAt))}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3;text-align:right;font-variant-numeric:tabular-nums">v${row.version}<br><button data-member-edit="${esc(row.memberRef)}" style="${button};margin-top:5px">编辑备注/联盟</button></td></tr>`;
  }).join('') || '<tr><td colspan="5" style="padding:22px;text-align:center;color:#8F959E">当前筛选没有服务端返回的成员</td></tr>';
  const collaborators = grid.collaboratorRows.map((collaborator) => `<tr><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3;font-family:ui-monospace,Menlo,monospace">staff ${collaborator.staffId}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3"><select data-collab-permission="${collaborator.collaboratorId}" style="${control}"><option value="view"${collaborator.permission === 'view' ? ' selected' : ''}>查看</option><option value="edit"${collaborator.permission === 'edit' ? ' selected' : ''}>编辑（本地元数据）</option></select></td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3">v${collaborator.version}</td><td style="padding:9px 12px;border-bottom:1px solid #EEF0F3;text-align:right"><button data-collab-update="${collaborator.collaboratorId}" style="${button}">保存权限</button><button data-collab-remove="${collaborator.collaboratorId}" style="${button};margin-left:6px;color:#D83931">移除</button></td></tr>`).join('') || '<tr><td colspan="4" style="padding:18px;text-align:center;color:#8F959E">暂无本地协作者</td></tr>';
  const editor = detail ? `<section style="padding:14px;border:1px solid #C5D6FF;border-radius:8px;background:#F5F9FF"><div style="display:flex;justify-content:space-between;align-items:center;gap:10px"><h2 style="margin:0;font-size:15px">编辑成员本地字段</h2><button id="member-edit-cancel" style="${button}">取消</button></div><div style="margin-top:12px;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px"><label style="display:grid;gap:6px;font-size:12px;color:#646A73">备注<textarea id="member-remark" maxlength="500" rows="3" style="${control};height:auto;padding:8px 10px">${esc(detail.remark || '')}</textarea></label><label style="display:grid;gap:6px;font-size:12px;color:#646A73">联盟<input id="member-alliance" maxlength="120" value="${esc(detail.alliance || '')}" style="${control}"></label></div><div style="margin-top:10px;font-size:12px;color:#667085">${esc(detail.memberRef)} · 当前版本 v${detail.version} · 仅写入本地成员字段</div><button id="member-edit-save" style="${primary};margin-top:12px">保存本地字段</button></section>` : '';
  const shareStatus = grid.externalShareEnabled ? '已开启' : '已关闭';
  const shareLink = sharePath ? `<div style="margin-top:10px;display:flex;gap:8px;align-items:center;flex-wrap:wrap"><code style="font-size:12px;color:#344054">${esc(sharePath)}</code><button id="member-grid-share-copy" style="${button}">复制新链接</button></div>` : grid.externalShareEnabled ? '<p style="margin:10px 0 0;color:#8F5A16;font-size:12px">链接只在开启或同一幂等重试时返回；如已遗失，请先关闭再重新开启。</p>' : '';
  const share = `<div style="padding:12px;border:1px solid #C5D6FF;border-radius:8px;background:#F5F9FF;color:#1849A9;font-size:13px;line-height:20px"><strong>公开只读会员网格 ${shareStatus}</strong>：公开页只显示成员显示名、本地状态、来源和服务时间，不展示手机号、客户编号或外部身份。<div style="margin-top:10px"><button id="member-grid-share-toggle" style="${grid.externalShareEnabled ? button : primary}">${grid.externalShareEnabled ? '关闭公开网格' : '开启并生成链接'}</button></div>${shareLink}</div>`;
  const previous = cursorDepth > 0 ? `<button id="member-grid-previous" style="${button}">上一页</button>` : '';
  const next = page.hasMore ? `<button id="member-grid-next" style="${button}">下一页</button>` : '';
  return `<div style="padding:20px;display:grid;gap:14px;align-content:start"><div style="display:flex;justify-content:space-between;gap:12px;align-items:center"><div><div style="font-size:12px;color:#8F959E">交易 / 周期商品</div><h1 style="margin:4px 0 0;font-size:20px">${esc(grid.product.name)} · Member Grid</h1></div><button id="member-grid-back" style="${button}">返回周期商品</button></div>${editor}<section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px;background:#fff"><h2 style="margin:0 0 10px;font-size:15px">当前读取边界</h2><div style="display:flex;gap:18px;flex-wrap:wrap"><span>访问：可查看 / 可查询</span><span>当前页：${page.rows.length} 条</span><span>共享视图：${grid.views.length} 个内置只读视图</span><span>外部分享：backend_blocked</span></div></section><section style="border:1px solid #DEE0E3;border-radius:8px;background:#fff;overflow:hidden"><div style="padding:14px 16px;border-bottom:1px solid #EEF0F3;display:flex;justify-content:space-between;gap:12px;align-items:center;flex-wrap:wrap"><h2 style="margin:0;font-size:15px">会员数据（服务端 cursor）</h2><div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap"><select id="member-grid-view" style="${control}"><option value="default"${filters.viewId === 'default' ? ' selected' : ''}>默认视图</option><option value=""${filters.viewId === '' ? ' selected' : ''}>自定义查询</option></select><select id="member-grid-state" style="${control}"><option value="all"${filters.state === 'all' ? ' selected' : ''}>全部状态</option><option value="active"${filters.state === 'active' ? ' selected' : ''}>有效</option><option value="expired"${filters.state === 'expired' ? ' selected' : ''}>已过期</option><option value="removed"${filters.state === 'removed' ? ' selected' : ''}>已移除</option></select><select id="member-grid-source" style="${control}"><option value=""${filters.source === '' ? ' selected' : ''}>全部来源</option><option value="manual"${filters.source === 'manual' ? ' selected' : ''}>手动</option><option value="paid_order"${filters.source === 'paid_order' ? ' selected' : ''}>已支付订单</option></select><select id="member-grid-sort" style="${control}"><option value="updated_at_desc"${filters.sort === 'updated_at_desc' ? ' selected' : ''}>按更新时间</option><option value="starts_at_desc"${filters.sort === 'starts_at_desc' ? ' selected' : ''}>按开始时间</option></select><select id="member-grid-group" style="${control}"><option value=""${filters.groupBy === '' ? ' selected' : ''}>不分组</option><option value="state"${filters.groupBy === 'state' ? ' selected' : ''}>按状态分组</option></select><button id="member-grid-apply" style="${button}">查询</button><span style="font-size:12px;color:#667085">第 ${cursorDepth + 1} 页</span>${previous}${next}</div></div><div style="overflow-x:auto"><table style="width:100%;min-width:860px;border-collapse:collapse"><thead><tr style="background:#FAFAFB;text-align:left;color:#667085"><th style="padding:9px 12px">客户</th><th style="padding:9px 12px">状态</th><th style="padding:9px 12px">来源</th><th style="padding:9px 12px">起止时间</th><th style="padding:9px 12px;text-align:right">版本 / 操作</th></tr></thead><tbody>${rows}</tbody></table></div></section><section style="border:1px solid #DEE0E3;border-radius:8px;background:#fff;overflow:hidden"><div style="padding:14px 16px;border-bottom:1px solid #EEF0F3"><h2 style="margin:0;font-size:15px">服务端 schema（${grid.columns.length} 列）</h2></div><table style="width:100%;border-collapse:collapse"><thead><tr style="background:#FAFAFB;text-align:left;color:#667085"><th style="padding:9px 12px">字段</th><th style="padding:9px 12px">标签</th><th style="padding:9px 12px">类型</th></tr></thead><tbody>${columns}</tbody></table></section><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px;background:#fff"><h2 style="margin:0 0 8px;font-size:15px">只读视图</h2><ul style="margin:0;padding-left:20px">${views}</ul><p style="margin:10px 0 0;color:#8F5A16;font-size:12px">当前仅接入默认视图、更新/开始时间倒序和状态分组；遗留任意视图、排序和分组保持 backend_blocked。</p></section><section style="border:1px solid #DEE0E3;border-radius:8px;background:#fff;overflow:hidden"><div style="padding:14px 16px;border-bottom:1px solid #EEF0F3;display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap"><div><h2 style="margin:0;font-size:15px">本地协作者</h2><p style="margin:5px 0 0;font-size:12px;color:#8F5A16">写入的是本地 staff 元数据，不发送企微邀请、不证明 Provider 成功。</p></div><div style="display:flex;gap:8px;align-items:center"><input id="member-grid-staff" inputmode="numeric" placeholder="staff_id" style="${control};width:110px"><select id="member-grid-permission" style="${control}"><option value="view">查看</option><option value="edit">编辑（本地元数据）</option></select><button id="member-grid-add" style="${primary}">添加本地协作者</button></div></div><table style="width:100%;border-collapse:collapse"><thead><tr style="background:#FAFAFB;text-align:left;color:#667085"><th style="padding:9px 12px">成员</th><th style="padding:9px 12px">权限</th><th style="padding:9px 12px">版本</th><th style="padding:9px 12px;text-align:right">操作</th></tr></thead><tbody>${collaborators}</tbody></table></section><section style="padding:14px;border:1px solid #DEE0E3;border-radius:8px;background:#fff"><h2 style="margin:0 0 10px;font-size:15px">公开只读分享</h2>${share}<p style="margin:10px 0 0;color:#667085;font-size:12px">不拼接、不复制、不打开未经服务端公开契约返回的链接。</p></section><p style="margin:0;color:#8F5A16;font-size:12px">当前页面复用 Member Grid 的真实本地读取、成员字段 CAS 和协作者本地元数据操作；不把旧页面行为、Mock 或 Seed 当作上线能力。</p></div>`;
}

export async function mountServicePeriodMemberGrid(stage: HTMLElement): Promise<void> {
  const id = queryID();
  if (!id) throw new Error('Member Grid 需要有效周期商品 ID');
  let grid = await getServicePeriodMemberGridMetaDto(id);
  let filters: MemberGridFilters = { state: 'all', source: '', sort: 'updated_at_desc', groupBy: '', viewId: 'default' };
  let cursors = [''];
  let page = await queryServicePeriodMemberGridDto(id, { state: filters.state, source: filters.source, sort: filters.sort, groupBy: filters.groupBy, viewId: filters.viewId, limit: 50 });
  let detail: ServicePeriodMemberDetail | undefined;
  let sharePath = '';

  const showError = (error: unknown, fallback: string): void => toast(error instanceof Error ? error.message : fallback, true);
  const currentCursor = (): string => cursors[cursors.length - 1] || '';
  const reloadPage = async (): Promise<void> => { page = await queryServicePeriodMemberGridDto(id, { ...filters, limit: 50, cursor: currentCursor() || undefined }); render(); };
  const refreshGrid = async (): Promise<void> => { grid = await getServicePeriodMemberGridMetaDto(id); await reloadPage(); };
  const render = (): void => {
    stage.innerHTML = memberGridHtml(grid, page, filters, cursors.length - 1, detail, sharePath).replace('外部分享：backend_blocked', `公开只读会员网格：${grid.externalShareEnabled ? '已开启' : '已关闭'}`);
    stage.querySelector<HTMLButtonElement>('#member-grid-back')?.addEventListener('click', () => { location.href = 'spProducts.html'; });
    stage.querySelector<HTMLButtonElement>('#member-grid-apply')?.addEventListener('click', () => {
      const state = valueOf(stage, 'member-grid-state') as MemberGridState;
      const source = valueOf(stage, 'member-grid-source') as MemberGridSourceFilter;
      const viewId = valueOf(stage, 'member-grid-view') === 'default' ? 'default' : '';
      const sort = valueOf(stage, 'member-grid-sort') === 'starts_at_desc' ? 'starts_at_desc' : 'updated_at_desc';
      const groupBy = valueOf(stage, 'member-grid-group') === 'state' ? 'state' : '';
      filters = viewId === 'default' ? { state: 'all', source: '', sort: 'updated_at_desc', groupBy: '', viewId } : { state: ['active', 'expired', 'removed', 'all'].includes(state) ? state : 'all', source: ['', 'manual', 'paid_order'].includes(source) ? source : '', sort, groupBy, viewId };
      cursors = ['']; detail = undefined; void reloadPage().catch((error) => showError(error, 'Member Grid 查询失败'));
    });
    stage.querySelector<HTMLButtonElement>('#member-grid-previous')?.addEventListener('click', () => { if (cursors.length < 2) return; cursors.pop(); void reloadPage().catch((error) => showError(error, 'Member Grid 上一页读取失败')); });
    stage.querySelector<HTMLButtonElement>('#member-grid-next')?.addEventListener('click', () => { if (!page.hasMore || !page.nextCursor) return; cursors.push(page.nextCursor); void reloadPage().catch((error) => showError(error, 'Member Grid 下一页读取失败')); });
    stage.querySelectorAll<HTMLButtonElement>('[data-member-edit]').forEach((buttonElement) => buttonElement.addEventListener('click', () => {
      const ref = buttonElement.dataset.memberEdit || '';
      void getServicePeriodMemberDto(id, ref).then((member) => { detail = member; render(); }).catch((error) => showError(error, '成员详情读取失败'));
    }));
    stage.querySelector<HTMLButtonElement>('#member-edit-cancel')?.addEventListener('click', () => { detail = undefined; render(); });
    stage.querySelector<HTMLButtonElement>('#member-edit-save')?.addEventListener('click', () => {
      if (!detail) return;
      const member = detail;
      void updateServicePeriodMemberFieldsDto(id, member.memberRef, { expectedVersion: member.version, remark: valueOf(stage, 'member-remark'), alliance: valueOf(stage, 'member-alliance') }).then((saved) => { detail = saved; toast('成员备注/联盟已保存（本地）'); return reloadPage(); }).catch((error) => showError(error, '成员字段保存失败'));
    });
    stage.querySelector<HTMLButtonElement>('#member-grid-add')?.addEventListener('click', () => {
      const staffId = Number(valueOf(stage, 'member-grid-staff')); const permission = valueOf(stage, 'member-grid-permission') === 'edit' ? 'edit' : 'view';
      void createServicePeriodMemberGridCollaboratorDto(id, { staffId, permission }).then(() => refreshGrid()).then(() => toast('本地协作者配置已保存；未发送企微邀请/Provider')).catch((error) => showError(error, '本地协作者添加失败'));
    });
    stage.querySelectorAll<HTMLButtonElement>('[data-collab-update]').forEach((buttonElement) => buttonElement.addEventListener('click', () => {
      const collaboratorId = Number(buttonElement.dataset.collabUpdate); const collaborator = grid.collaboratorRows.find((item) => item.collaboratorId === collaboratorId);
      if (!collaborator) return; const select = stage.querySelector<HTMLSelectElement>(`[data-collab-permission="${collaboratorId}"]`); const selected = select?.value === 'edit' ? 'edit' : 'view';
      void updateServicePeriodMemberGridCollaboratorDto(id, collaboratorId, { expectedVersion: collaborator.version, permission: selected }).then(() => refreshGrid()).then(() => toast('本地协作者权限已保存；未改变企微/Provider 权限')).catch((error) => showError(error, '本地协作者权限保存失败'));
    }));
    stage.querySelectorAll<HTMLButtonElement>('[data-collab-remove]').forEach((buttonElement) => buttonElement.addEventListener('click', () => {
      const collaboratorId = Number(buttonElement.dataset.collabRemove); const collaborator = grid.collaboratorRows.find((item) => item.collaboratorId === collaboratorId);
      if (!collaborator || !window.confirm(`确认移除本地协作者 staff ${collaborator.staffId}？`)) return;
      void deleteServicePeriodMemberGridCollaboratorDto(id, collaboratorId, collaborator.version).then(() => refreshGrid()).then(() => toast('本地协作者已移除；未调用企微/Provider')).catch((error) => showError(error, '本地协作者移除失败'));
    }));
    stage.querySelector<HTMLButtonElement>('#member-grid-share-toggle')?.addEventListener('click', () => {
      const enable = !grid.externalShareEnabled;
      void setMemberGridExternalShareDto(id, enable, grid.externalShareVersion).then((result) => {
        grid = { ...grid, externalShareEnabled: result.enabled, externalShareVersion: result.version };
        sharePath = result.publicPath;
        render();
        toast(enable ? '公开只读会员网格已开启；请立即保存本次返回的新链接' : '公开只读会员网格已关闭；旧链接已失效');
      }).catch((error) => showError(error, enable ? '公开分享开启失败' : '公开分享关闭失败'));
    });
    stage.querySelector<HTMLButtonElement>('#member-grid-share-copy')?.addEventListener('click', () => copyText(new URL(sharePath, location.origin).toString(), toast));
  };
  render();
}
