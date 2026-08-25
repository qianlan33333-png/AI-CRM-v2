/**
 * API 适配层 —— 预览与上线之间的唯一接缝。
 *
 *  - MockApi：仅供显式测试注入，数据落在 sessionStorage。
 *  - HttpApi：遗留页面 Adapter；新增接入必须改用 current Go OpenAPI generated operation。
 *
 * 页面模块只依赖 AdminApi 接口，不感知具体实现。
 */
import type {
  AdminDb,
  AiRecipient,
  AudienceGroup,
  AudiencePackage,
  AttachItem,
  ConfigCategory,
  Coupon,
  Customer,
  FunnelGridRow,
  FunnelView,
  ImageItem,
  MpItem,
  OwnerReassignmentPreview,
  Product,
  QuestionnaireOps,
  RadarEvent,
  RadarLink,
  RadarLinkInput,
  RadarMedia,
  SpProduct,
  TagGroup,
  WecomTag,
} from './types';
import { SEED_DB, deepCopy } from './mockData';
import { deleteProductDto } from '../../api/admin';
import { archiveCouponDto, copyCouponDto, deleteCouponDto, saveCouponDto, setCouponPublishedDto, type CouponWriteInput } from '../../api/admin';
import { archiveAudiencePackage, archiveServiceProductDto, archiveTagDto, copyAudiencePackageDto, copyProductDto, copyServiceProductDto, createOwnerReassignmentPreviewDto, deleteAttachmentItemDto, deleteAudienceGroup as deleteAudienceGroupDto, deleteImageItemDto, deleteMiniProgramItemDto, downloadAttachmentItemDto, downloadOwnerReassignmentReportDto, downloadOwnerReassignmentTemplateDto, executeOwnerReassignmentPreviewDto, getOwnerReassignmentPreviewDto, queueTagSyncDto, readAdminPage, readCouponSharePath, readRadarEvents, readRadarSharePath, saveAttachmentItemDto, saveAudienceGroup as saveAudienceGroupDto, saveImageItemDto, saveMiniProgramItemDto, saveProductDto, saveRadarLinkDto, saveServiceProductDto, saveTagDto, saveTagGroupDto, setAudiencePackageRunning, setCustomerTagDto, setProductEnabledDto, setRadarEnabled, setServiceProductEnabledDto, updateCustomerDto, uploadRadarImageDto, uploadRadarPdfDto, type AdminReadContext, type ProductWriteInput } from '../../api/admin';

/* ================= 接口定义 ================= */

export interface AdminApi {
  readonly mode: 'mock' | 'http';

  /** 聚合加载后台数据仓库 */
  loadDb(context?: AdminReadContext): Promise<AdminDb>;
  updateCustomer(id: number, input: { name?: string; stageId?: number | null }): Promise<Customer>;
  setCustomerTag(customerId: number, tagId: number, applied: boolean): Promise<void>;

  /* ---- 内容雷达 ---- */
  toggleRadarLink(id: number, enabled: boolean): Promise<void>;
  saveRadarLink(input: RadarLinkInput): Promise<RadarLink>;
  listRadarEvents(linkId: number): Promise<RadarEvent[]>;
  getRadarSharePath(linkId: number): Promise<string>;
  getCouponSharePath(couponId: number): Promise<string>;
  /** 上传雷达图片素材（multipart），返回可引用的素材描述 */
  uploadRadarImage(file: File): Promise<RadarMedia>;
  /** 上传雷达 PDF 素材 */
  uploadRadarPdf(file: File): Promise<RadarMedia>;

  /* ---- AI 助手 ---- */
  approveAiPlan(id: number): Promise<void>;
  rejectAiPlan(id: number): Promise<void>;
  listAiRecipients(planId: number): Promise<AiRecipient[]>;
  approveAiRecipient(planId: number, rcId: number): Promise<void>;
  rejectAiRecipient(planId: number, rcId: number): Promise<void>;
  /** 审阅备注实时写回（仅审阅可见） */
  updateRecipientNote(planId: number, rcId: number, taskIdx: number, note: string): Promise<void>;

  /* ---- 漏斗 / 多维数据看板 ---- */
  /** 全量行数据；筛选 / 分组 / 排序在前端视图层完成（与原型一致） */
  listFunnelRows(): Promise<FunnelGridRow[]>;
  listFunnelViews(): Promise<FunnelView[]>;
  saveFunnelViews(views: FunnelView[]): Promise<void>;

  /* ---- 自动化运营 · 人群包 ---- */
  saveAudienceGroup(input: { id?: number; name: string }): Promise<AudienceGroup>;
  deleteAudienceGroup(id: number): Promise<void>;
  saveAudiencePackage(input: Partial<AudiencePackage> & { id: number }): Promise<void>;
  toggleAudiencePackage(id: number, running: boolean): Promise<void>;
  copyAudiencePackage(id: number): Promise<AudiencePackage>;
  deleteAudiencePackage(id: number): Promise<void>;

  /* ---- 企微标签 ---- */
  saveTagGroup(input: { id?: number; name: string; firstTag?: string }): Promise<TagGroup>;
  saveTag(input: { id?: number; groupId: number; name: string }): Promise<WecomTag>;
  deleteTag(id: number): Promise<void>;
  syncWecomTags(): Promise<void>;

  /* ---- 问卷 · 运营配置 ---- */
  saveQuestionnaireOps(qid: number, ops: QuestionnaireOps): Promise<void>;

  /* ---- 素材库（按名称定位，null = 新建） ---- */
  saveImageItem(originalName: string | null, patch: Partial<ImageItem> & { name: string }): Promise<void>;
  deleteImageItem(item: ImageItem): Promise<void>;
  saveMpItem(originalName: string | null, patch: Partial<MpItem> & { name: string }): Promise<void>;
  deleteMpItem(item: MpItem): Promise<void>;
  saveAttachItem(originalName: string | null, patch: Partial<AttachItem> & { name: string }): Promise<void>;
  deleteAttachItem(item: AttachItem): Promise<void>;
  downloadAttachItem(item: AttachItem): Promise<Blob>;

  /* ---- 负责人迁移 · 本地安全事务 ---- */
  downloadOwnerReassignmentTemplate(): Promise<Blob>;
  createOwnerReassignmentPreview(csv: string): Promise<OwnerReassignmentPreview>;
  getOwnerReassignmentPreview(previewId: string): Promise<OwnerReassignmentPreview>;
  executeOwnerReassignmentPreview(preview: OwnerReassignmentPreview): Promise<OwnerReassignmentPreview>;
  downloadOwnerReassignmentReport(previewId: string, kind: 'errors' | 'results'): Promise<Blob>;

  /* ---- 普通商品 / 周期商品 ---- */
  saveProduct(input: ProductWriteInput): Promise<Product>;
  setProductEnabled(productId: number, enabled: boolean): Promise<Product>;
  copyProduct(productId: number): Promise<Product>;
  deleteProduct(productId: number): Promise<void>;
  saveServiceProduct(input: ProductWriteInput): Promise<SpProduct>;
  setServiceProductEnabled(productId: number, enabled: boolean): Promise<SpProduct>;
  copyServiceProduct(productId: number): Promise<SpProduct>;
  archiveServiceProduct(productId: number): Promise<void>;

  /* ---- 优惠券 ---- */
  saveCoupon(input: CouponWriteInput, publish: boolean): Promise<Coupon>;
  setCouponPublished(couponId: number, published: boolean): Promise<Coupon>;
  copyCoupon(couponId: number): Promise<Coupon>;
  archiveCoupon(couponId: number): Promise<void>;
  deleteCoupon(couponId: number): Promise<void>;

  /* ---- 配置中心 ---- */
  toggleConfigCategory(key: string, on: boolean): Promise<void>;
  saveConfigCategory(key: string, values: Record<string, string>, switches: Record<string, boolean>): Promise<void>;
  checkConfigCategory(key: string): Promise<string>;
}

/* ================= Mock 实现 ================= */

const SS_KEY = 'aicrm.mock.db.v4';
const MOCK_DELAY = 200;

function delay<T>(v: T, ms = MOCK_DELAY): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(v), ms));
}

export class MockApi implements AdminApi {
  readonly mode = 'mock' as const;
  private db: AdminDb;

  constructor() {
    this.db = this.restore();
  }

  private restore(): AdminDb {
    try {
      const raw = sessionStorage.getItem(SS_KEY);
      if (raw) return JSON.parse(raw) as AdminDb;
    } catch {
      /* 损坏则重建 */
    }
    const fresh = deepCopy(SEED_DB);
    sessionStorage.setItem(SS_KEY, JSON.stringify(fresh));
    return fresh;
  }

  private persist(): void {
    try {
      sessionStorage.setItem(SS_KEY, JSON.stringify(this.db));
    } catch {
      /* 存储满时静默降级为内存态 */
    }
  }

  loadDb(_context?: AdminReadContext): Promise<AdminDb> {
    this.db = this.restore();
    return delay(this.db, 120);
  }

  async updateCustomer(id: number, input: { name?: string; stageId?: number | null }): Promise<Customer> {
    const customer = this.db.rows.customers.find((item) => Number(item.id) === id);
    if (!customer) throw new Error('客户不存在');
    if (input.name != null) customer.name = input.name;
    if (input.stageId !== undefined) customer.stageId = input.stageId;
    this.persist();
    return delay(customer);
  }

  setCustomerTag(_customerId: number, _tagId: number, _applied: boolean): Promise<void> { return delay(undefined); }

  /* ---------- 内容雷达 ---------- */

  async toggleRadarLink(id: number, enabled: boolean): Promise<void> {
    const r = this.db.radarLinks.find((x) => x.id === id);
    if (r) {
      r.enabled = enabled;
      this.persist();
    }
    return delay(undefined);
  }

  saveRadarLink(input: RadarLinkInput): Promise<RadarLink> {
    let rec: RadarLink | undefined;
    if (input.id !== undefined) {
      rec = this.db.radarLinks.find((x) => x.id === input.id);
    }
    if (rec) {
      Object.assign(rec, input);
    } else {
      const nextId = Math.max(0, ...this.db.radarLinks.map((x) => x.id)) + 1;
      rec = {
        id: nextId,
        title: input.title || '未命名雷达',
        target_type: input.target_type,
        original_url: input.original_url,
        file_name_snapshot: input.file_name_snapshot,
        media_item_id: input.media_item_id,
        enabled: input.enabled,
        auth_required: input.auth_required,
        staff_id: 'HuangYouCan',
        code: Math.random().toString(36).slice(2, 8),
        total_landings: 0,
        authorized_users: 0,
        view_count: 0,
        last_viewed_at: '-',
      };
      this.db.radarLinks.unshift(rec);
    }
    this.persist();
    return delay(rec, 400);
  }

  listRadarEvents(_linkId: number): Promise<RadarEvent[]> {
    return delay(this.db.radarEvents);
  }

  getRadarSharePath(linkId: number): Promise<string> {
    const link = this.db.radarLinks.find((item) => item.id === linkId);
    return delay(link ? `/r/${link.code}` : '');
  }

  getCouponSharePath(couponId: number): Promise<string> {
    return delay(`/c/c-${couponId}`);
  }

  uploadRadarImage(file: File): Promise<RadarMedia> {
    return delay(
      {
        name: file.name,
        meta: `${file.type || 'image/*'} · ${(file.size / 1048576).toFixed(1)} MB · 刚上传`,
      },
      900,
    );
  }

  uploadRadarPdf(file: File): Promise<RadarMedia> {
    return delay(
      {
        name: file.name,
        meta: `${file.type || 'application/pdf'} · ${(file.size / 1048576).toFixed(1)} MB · 处理中`,
      },
      1200,
    );
  }

  /* ---------- AI 助手 ---------- */

  async approveAiPlan(id: number): Promise<void> {
    const p = this.db.aiPlans.find((x) => x.id === id);
    if (p) {
      p.status = 'approved';
      // 级联：待审阅人员一并批准
      for (const r of this.db.aiRcs[id] || []) {
        if (r.status === 'pending') r.status = 'approved';
      }
      this.persist();
    }
    return delay(undefined);
  }

  async rejectAiPlan(id: number): Promise<void> {
    const p = this.db.aiPlans.find((x) => x.id === id);
    if (p) {
      p.status = 'rejected';
      for (const r of this.db.aiRcs[id] || []) {
        if (r.status === 'pending') r.status = 'rejected';
      }
      this.persist();
    }
    return delay(undefined);
  }

  listAiRecipients(planId: number): Promise<AiRecipient[]> {
    return delay(this.db.aiRcs[planId] || []);
  }

  private findRc(planId: number, rcId: number): AiRecipient | undefined {
    return (this.db.aiRcs[planId] || []).find((x) => x.id === rcId);
  }

  async approveAiRecipient(planId: number, rcId: number): Promise<void> {
    const r = this.findRc(planId, rcId);
    if (r && r.status === 'pending') {
      r.status = 'approved';
      this.persist();
    }
    return delay(undefined);
  }

  async rejectAiRecipient(planId: number, rcId: number): Promise<void> {
    const r = this.findRc(planId, rcId);
    if (r && r.status === 'pending') {
      r.status = 'rejected';
      this.persist();
    }
    return delay(undefined);
  }

  async updateRecipientNote(planId: number, rcId: number, taskIdx: number, note: string): Promise<void> {
    const r = this.findRc(planId, rcId);
    if (r && r.tasks[taskIdx]) {
      r.tasks[taskIdx].note = note;
      this.persist();
    }
    return delay(undefined, 0);
  }

  /* ---------- 漏斗 ---------- */

  listFunnelRows(): Promise<FunnelGridRow[]> {
    return delay(this.db.funnelRows);
  }

  listFunnelViews(): Promise<FunnelView[]> {
    return delay(this.db.funnelViews);
  }

  async saveFunnelViews(views: FunnelView[]): Promise<void> {
    this.db.funnelViews = deepCopy(views);
    this.persist();
    return delay(undefined);
  }

  /* ---------- 自动化运营 · 人群包 ---------- */

  saveAudienceGroup(input: { id?: number; name: string }): Promise<AudienceGroup> {
    let g: AudienceGroup | undefined;
    if (input.id !== undefined) g = this.db.audienceGroups.find((x) => x.id === input.id);
    if (g) {
      g.name = input.name;
    } else {
      g = { id: Math.max(0, ...this.db.audienceGroups.map((x) => x.id)) + 1, name: input.name };
      this.db.audienceGroups.push(g);
    }
    this.persist();
    return delay(g, 400);
  }

  async deleteAudienceGroup(id: number): Promise<void> {
    this.db.audienceGroups = this.db.audienceGroups.filter((x) => x.id !== id);
    for (const p of this.db.audiencePackages) if (p.groupId === id) p.groupId = 0;
    this.persist();
    return delay(undefined);
  }

  async saveAudiencePackage(input: Partial<AudiencePackage> & { id: number }): Promise<void> {
    const p = this.db.audiencePackages.find((x) => x.id === input.id);
    if (p) {
      Object.assign(p, input);
      this.persist();
    }
    return delay(undefined, 400);
  }

  async toggleAudiencePackage(id: number, running: boolean): Promise<void> {
    const p = this.db.audiencePackages.find((x) => x.id === id);
    if (p) {
      p.running = running;
      this.persist();
    }
    return delay(undefined);
  }

  copyAudiencePackage(id: number): Promise<AudiencePackage> {
    const src = this.db.audiencePackages.find((x) => x.id === id);
    if (!src) return delay(undefined as unknown as AudiencePackage);
    const copy: AudiencePackage = {
      ...deepCopy(src),
      id: Math.max(0, ...this.db.audiencePackages.map((x) => x.id)) + 1,
      name: src.name + '（副本）',
      count: 0,
      lastRefresh: '-',
      running: false,
    };
    this.db.audiencePackages.push(copy);
    this.persist();
    return delay(copy, 400);
  }

  async deleteAudiencePackage(id: number): Promise<void> {
    this.db.audiencePackages = this.db.audiencePackages.filter((x) => x.id !== id);
    this.persist();
    return delay(undefined);
  }

  /* ---------- 企微标签 ---------- */

  saveTagGroup(input: { id?: number; name: string; firstTag?: string }): Promise<TagGroup> {
    let g: TagGroup | undefined;
    if (input.id !== undefined) g = this.db.tagGroups.find((x) => x.id === input.id);
    if (g) {
      g.name = input.name;
    } else {
      g = { id: Math.max(0, ...this.db.tagGroups.map((x) => x.id)) + 1, name: input.name };
      this.db.tagGroups.push(g);
      if (input.firstTag) {
        this.db.wecomTags.push({
          id: Math.max(0, ...this.db.wecomTags.map((x) => x.id)) + 1,
          groupId: g.id,
          name: input.firstTag,
          users: 0,
          syncedAt: '刚刚',
        });
      }
    }
    this.persist();
    return delay(g, 400);
  }

  saveTag(input: { id?: number; groupId: number; name: string }): Promise<WecomTag> {
    let tg: WecomTag | undefined;
    if (input.id !== undefined) tg = this.db.wecomTags.find((x) => x.id === input.id);
    if (tg) {
      tg.name = input.name;
      tg.groupId = input.groupId;
    } else {
      tg = {
        id: Math.max(0, ...this.db.wecomTags.map((x) => x.id)) + 1,
        groupId: input.groupId,
        name: input.name,
        users: 0,
        syncedAt: '刚刚',
      };
      this.db.wecomTags.push(tg);
    }
    this.persist();
    return delay(tg, 400);
  }

  async deleteTag(id: number): Promise<void> {
    this.db.wecomTags = this.db.wecomTags.filter((x) => x.id !== id);
    this.persist();
    return delay(undefined);
  }

  async syncWecomTags(): Promise<void> {
    const now = '刚刚';
    for (const tg of this.db.wecomTags) tg.syncedAt = now;
    this.persist();
    return delay(undefined, 800);
  }

  /* ---------- 问卷 · 运营配置 ---------- */

  async saveQuestionnaireOps(qid: number, ops: QuestionnaireOps): Promise<void> {
    this.db.qOps[qid] = deepCopy(ops);
    this.persist();
    return delay(undefined, 500);
  }

  /* ---------- 素材库 ---------- */

  private upsertByName<T extends { name: string }>(list: T[], originalName: string | null, patch: Partial<T> & { name: string }): void {
    const it = originalName ? list.find((x) => x.name === originalName) : undefined;
    if (it) Object.assign(it, patch);
    else list.unshift(patch as T);
  }

  async saveImageItem(originalName: string | null, patch: Partial<ImageItem> & { name: string }): Promise<void> {
    this.upsertByName(this.db.rows.images, originalName, patch);
    this.persist();
    return delay(undefined, 500);
  }

  async deleteImageItem(item: ImageItem): Promise<void> {
    this.db.rows.images = this.db.rows.images.filter((x) => x.name !== item.name);
    this.persist();
    return delay(undefined);
  }

  async saveMpItem(originalName: string | null, patch: Partial<MpItem> & { name: string }): Promise<void> {
    this.upsertByName(this.db.rows.mpItems, originalName, patch);
    this.persist();
    return delay(undefined, 500);
  }

  async deleteMpItem(item: MpItem): Promise<void> {
    this.db.rows.mpItems = this.db.rows.mpItems.filter((x) => x.name !== item.name);
    this.persist();
    return delay(undefined);
  }

  async saveAttachItem(originalName: string | null, patch: Partial<AttachItem> & { name: string }): Promise<void> {
    this.upsertByName(this.db.rows.attachItems, originalName, patch);
    this.persist();
    return delay(undefined, 500);
  }

  async deleteAttachItem(item: AttachItem): Promise<void> {
    this.db.rows.attachItems = this.db.rows.attachItems.filter((x) => x.name !== item.name);
    this.persist();
    return delay(undefined);
  }

  downloadAttachItem(_item: AttachItem): Promise<Blob> {
    return delay(new Blob(['mock pdf'], { type: 'application/pdf' }));
  }

  downloadOwnerReassignmentTemplate(): Promise<Blob> {
    return delay(new Blob(['customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n'], { type: 'text/csv' }));
  }

  createOwnerReassignmentPreview(_csv: string): Promise<OwnerReassignmentPreview> {
    return delay({ id: 'cor_0123456789012345678901', hash: 'a'.repeat(64), rows: [], issues: [], expiresAt: new Date(Date.now() + 3600000).toISOString(), executed: false, result: [] });
  }

  getOwnerReassignmentPreview(_previewId: string): Promise<OwnerReassignmentPreview> {
    return this.createOwnerReassignmentPreview('mock');
  }

  executeOwnerReassignmentPreview(preview: OwnerReassignmentPreview): Promise<OwnerReassignmentPreview> {
    return delay({ ...preview, executed: true, result: preview.rows.map((row) => ({ customerId: row.customerId, previousOwnerStaffId: row.expectedOwnerStaffId, targetOwnerStaffId: row.targetOwnerStaffId, updatedAt: new Date().toISOString() })) });
  }

  downloadOwnerReassignmentReport(_previewId: string, _kind: 'errors' | 'results'): Promise<Blob> {
    return delay(new Blob(['mock'], { type: 'text/csv' }));
  }

  saveProduct(input: ProductWriteInput): Promise<Product> {
    const item = input.id == null ? { resourceId: Date.now(), code: input.code, name: input.name, price: input.price, description: input.description, currency: input.currency, stockQuantity: input.stockQuantity, version: 1, lifecycle: 'draft', status: 'draft', tone: 'warn' as const, sold: '0', updated: '' } : this.db.rows.products.find((row) => row.resourceId === input.id)!;
    Object.assign(item, input, { resourceId: item.resourceId, version: (item.version || 0) + 1 });
    if (input.id == null) this.db.rows.products.push(item);
    this.persist(); return delay(item);
  }
  setProductEnabled(productId: number, enabled: boolean): Promise<Product> { const item = this.db.rows.products.find((row) => row.resourceId === productId)!; item.lifecycle = item.status = enabled ? 'enabled' : 'disabled'; this.persist(); return delay(item); }
  copyProduct(productId: number): Promise<Product> { const source = this.db.rows.products.find((row) => row.resourceId === productId)!; return this.saveProduct({ id: undefined, code: source.code + '-COPY', name: source.name + '（副本）', description: source.description || '', price: source.price, currency: source.currency || 'CNY', stockQuantity: source.stockQuantity || 0 }); }
  deleteProduct(productId: number): Promise<void> { this.db.rows.products = this.db.rows.products.filter((row) => row.resourceId !== productId); this.persist(); return delay(undefined); }
  saveServiceProduct(input: ProductWriteInput): Promise<SpProduct> { const item = input.id == null ? { resourceId: Date.now(), code: input.code, name: input.name, price: input.price, description: input.description, currency: input.currency, stockQuantity: input.stockQuantity, version: 1, lifecycle: 'draft', status: 'draft', tone: 'warn' as const, sold: '0', updated: '' } : this.db.rows.spProducts.find((row) => row.resourceId === input.id)!; Object.assign(item, input, { resourceId: item.resourceId, version: (item.version || 0) + 1 }); if (input.id == null) this.db.rows.spProducts.push(item); this.persist(); return delay(item); }
  setServiceProductEnabled(productId: number, enabled: boolean): Promise<SpProduct> { const item = this.db.rows.spProducts.find((row) => row.resourceId === productId)!; item.lifecycle = item.status = enabled ? 'enabled' : 'disabled'; this.persist(); return delay(item); }
  copyServiceProduct(productId: number): Promise<SpProduct> { const source = this.db.rows.spProducts.find((row) => row.resourceId === productId)!; return this.saveServiceProduct({ id: undefined, code: source.code + '-COPY', name: source.name + '（副本）', description: source.description || '', price: source.price, currency: source.currency || 'CNY', stockQuantity: source.stockQuantity || 0 }); }
  archiveServiceProduct(productId: number): Promise<void> { this.db.rows.spProducts = this.db.rows.spProducts.filter((row) => row.resourceId !== productId); this.persist(); return delay(undefined); }

  saveCoupon(input: CouponWriteInput, publish: boolean): Promise<Coupon> {
    const item = input.id == null
      ? { resourceId: Date.now(), code: `C-${Date.now()}`, name: input.name, off: `¥${input.discount}`, scope: input.targetRefs.join('、'), window: `${input.claimStartsAt} – ${input.claimEndsAt}`, issue: `0 / ${input.totalIssueLimit}`, status: 'draft', tone: 'warn' as const }
      : this.db.rows.coupons.find((row) => row.resourceId === input.id)!;
    Object.assign(item, input, { status: publish ? 'published' : item.status, tone: publish ? 'ok' : item.tone, version: (item.version || 0) + 1 });
    if (input.id == null) this.db.rows.coupons.push(item);
    this.persist();
    return delay(item);
  }
  setCouponPublished(couponId: number, published: boolean): Promise<Coupon> { const item = this.db.rows.coupons.find((row) => row.resourceId === couponId)!; item.status = published ? 'published' : 'stopped'; item.tone = published ? 'ok' : 'gray'; this.persist(); return delay(item); }
  copyCoupon(couponId: number): Promise<Coupon> { const source = this.db.rows.coupons.find((row) => row.resourceId === couponId)!; return this.saveCoupon({ id: undefined, name: source.name + '（副本）', discount: String((source.discountAmountTotal || 0) / 100), totalIssueLimit: source.totalIssueLimit || 1, perUserIssueLimit: source.perUserIssueLimit || 1, claimStartsAt: source.claimStartsAt || new Date().toISOString(), claimEndsAt: source.claimEndsAt || new Date().toISOString(), validityMode: source.validityMode || 'relative_days', useStartsAt: source.useStartsAt || undefined, useEndsAt: source.useEndsAt || undefined, relativeValidityDays: source.relativeValidityDays || undefined, instructions: source.instructions || '', targetRefs: source.targetRefs || [] }, false); }
  archiveCoupon(couponId: number): Promise<void> { const item = this.db.rows.coupons.find((row) => row.resourceId === couponId); if (item) { item.status = 'archived'; item.tone = 'gray'; this.persist(); } return delay(undefined); }
  deleteCoupon(couponId: number): Promise<void> { this.db.rows.coupons = this.db.rows.coupons.filter((row) => row.resourceId !== couponId); this.persist(); return delay(undefined); }

  /* ---------- 配置中心 ---------- */

  private findConfigCat(key: string): ConfigCategory | undefined {
    return this.db.configCategories.find((x) => x.key === key);
  }

  async toggleConfigCategory(key: string, on: boolean): Promise<void> {
    const c = this.findConfigCat(key);
    if (c) {
      c.on = on;
      this.persist();
    }
    return delay(undefined, 300);
  }

  async saveConfigCategory(key: string, values: Record<string, string>, switches: Record<string, boolean>): Promise<void> {
    const c = this.findConfigCat(key);
    if (c) {
      for (const b of c.blocks) {
        for (const f of b.fields) {
          if (f.kind === 'switch') {
            if (switches[f.key] !== undefined) f.on = switches[f.key];
          } else if (f.kind === 'secret') {
            // 密钥：仅在填写了新值时更新，空值保持原状
            if (values[f.key]) {
              f.configured = true;
              f.value = values[f.key];
            }
          } else if (values[f.key] !== undefined) {
            f.value = values[f.key];
          }
        }
      }
      this.persist();
    }
    return delay(undefined, 600);
  }

  checkConfigCategory(key: string): Promise<string> {
    const c = this.findConfigCat(key);
    if (!c) return delay('类目不存在', 600);
    const missing: string[] = [];
    for (const b of c.blocks) {
      for (const f of b.fields) {
        if (f.kind === 'secret' && !f.configured && !f.value) missing.push(f.key);
      }
    }
    return delay(missing.length ? '检查发现 ' + missing.length + ' 项未设置：' + missing.slice(0, 3).join('、') + (missing.length > 3 ? ' 等' : '') : '检查通过，关键配置均已设置', 800);
  }
}

/* ================= 遗留 HTTP Adapter ================= */

export interface HttpApiOptions {
  /** 例：https://www.youcangogogo.com */
  baseUrl: string;
  /** 登录态凭证（cookie 同源时留空即可） */
  token?: string;
}

export class HttpApi implements AdminApi {
  readonly mode = 'http' as const;

  constructor(_opts: HttpApiOptions) {}

  async loadDb(context?: AdminReadContext): Promise<AdminDb> {
    // OpenAPI failure reaches the view's loading/error state; production never merges SEED_DB.
    return readAdminPage(context);
  }

  updateCustomer(id: number, input: { name?: string; stageId?: number | null }): Promise<Customer> { return updateCustomerDto(id, input); }

  setCustomerTag(customerId: number, tagId: number, applied: boolean): Promise<void> { return setCustomerTagDto(customerId, tagId, applied); }

  /* ---------- 内容雷达 ---------- */

  toggleRadarLink(id: number, enabled: boolean): Promise<void> {
    return setRadarEnabled(id, enabled);
  }

  async saveRadarLink(input: RadarLinkInput): Promise<RadarLink> {
    return saveRadarLinkDto(input);
  }

  listRadarEvents(linkId: number): Promise<RadarEvent[]> {
    return readRadarEvents(linkId);
  }

  getRadarSharePath(linkId: number): Promise<string> {
    return readRadarSharePath(linkId);
  }

  getCouponSharePath(couponId: number): Promise<string> {
    return readCouponSharePath(couponId);
  }

  uploadRadarImage(file: File): Promise<RadarMedia> {
    return uploadRadarImageDto(file);
  }

  uploadRadarPdf(file: File): Promise<RadarMedia> {
    return uploadRadarPdfDto(file);
  }

  /* ---------- AI 助手 ---------- */

  approveAiPlan(id: number): Promise<void> {
    void id;
    return Promise.reject(new Error('后端能力未就绪：当前 AI 审阅壳 DTO 与 Cloud Orchestrator 计划审批契约不等价'));
  }

  rejectAiPlan(id: number): Promise<void> {
    void id;
    return Promise.reject(new Error('后端能力未就绪：当前 AI 审阅壳 DTO 与 Cloud Orchestrator 计划审批契约不等价'));
  }

  listAiRecipients(planId: number): Promise<AiRecipient[]> {
    void planId;
    return Promise.reject(new Error('后端能力未就绪：当前 AI 收件人 DTO 与 Cloud Orchestrator recipient 契约不等价'));
  }

  approveAiRecipient(planId: number, rcId: number): Promise<void> {
    void planId; void rcId;
    return Promise.reject(new Error('后端能力未就绪：当前 AI 收件人审阅 DTO 与 Cloud Orchestrator review 契约不等价'));
  }

  rejectAiRecipient(planId: number, rcId: number): Promise<void> {
    void planId; void rcId;
    return Promise.reject(new Error('后端能力未就绪：当前 AI 收件人审阅 DTO 与 Cloud Orchestrator review 契约不等价'));
  }

  updateRecipientNote(planId: number, rcId: number, taskIdx: number, note: string): Promise<void> {
    void planId; void rcId; void taskIdx; void note;
    return Promise.reject(new Error('后端能力未就绪：当前 AI 任务备注没有等价 OpenAPI operation'));
  }

  /* ---------- 漏斗 ---------- */

  listFunnelRows(): Promise<FunnelGridRow[]> {
    return Promise.reject(new Error('后端能力未就绪：当前漏斗行 DTO 与 Member Grid 查询契约不等价'));
  }

  listFunnelViews(): Promise<FunnelView[]> {
    return Promise.reject(new Error('后端能力未就绪：当前漏斗视图 DTO 与 Member Grid views 契约不等价'));
  }

  saveFunnelViews(views: FunnelView[]): Promise<void> {
    void views;
    return Promise.reject(new Error('后端能力未就绪：当前漏斗视图 DTO 与 Member Grid views 契约不等价'));
  }

  /* ---------- 自动化运营 · 人群包 ---------- */

  async saveAudienceGroup(input: { id?: number; name: string }): Promise<AudienceGroup> {
    return saveAudienceGroupDto(input);
  }

  deleteAudienceGroup(id: number): Promise<void> {
    return deleteAudienceGroupDto(id);
  }

  saveAudiencePackage(input: Partial<AudiencePackage> & { id: number }): Promise<void> {
    void input;
    return Promise.reject(new Error('后端能力未就绪：当前人群包表单缺少 SegmentDefinition 与版本字段，不能安全调用更新契约'));
  }

  toggleAudiencePackage(id: number, running: boolean): Promise<void> {
    return setAudiencePackageRunning(id, running);
  }

  copyAudiencePackage(id: number): Promise<AudiencePackage> {
    return copyAudiencePackageDto(id);
  }

  deleteAudiencePackage(id: number): Promise<void> {
    return archiveAudiencePackage(id);
  }

  /* ---------- 企微标签 ---------- */

  async saveTagGroup(input: { id?: number; name: string; firstTag?: string }): Promise<TagGroup> {
    return saveTagGroupDto(input);
  }

  async saveTag(input: { id?: number; groupId: number; name: string }): Promise<WecomTag> {
    return saveTagDto(input);
  }

  deleteTag(id: number): Promise<void> {
    return archiveTagDto(id);
  }

  syncWecomTags(): Promise<void> {
    return queueTagSyncDto().then(() => undefined);
  }

  /* ---------- 问卷 · 运营配置 ---------- */

  saveQuestionnaireOps(qid: number, ops: QuestionnaireOps): Promise<void> {
    void qid; void ops;
    return Promise.reject(new Error('后端能力未就绪：当前运营表单使用 URL，OpenAPI 仅接受 opaque navigation/configuration reference，DTO 不等价'));
  }

  /* ---------- 素材库 ---------- */

  saveImageItem(originalName: string | null, patch: Partial<ImageItem> & { name: string }): Promise<void> {
    return saveImageItemDto(originalName, patch);
  }

  deleteImageItem(item: ImageItem): Promise<void> {
    return deleteImageItemDto(item);
  }

  saveMpItem(originalName: string | null, patch: Partial<MpItem> & { name: string }): Promise<void> {
    return saveMiniProgramItemDto(originalName, patch);
  }

  deleteMpItem(item: MpItem): Promise<void> {
    return deleteMiniProgramItemDto(item);
  }

  saveAttachItem(originalName: string | null, patch: Partial<AttachItem> & { name: string }): Promise<void> {
    return saveAttachmentItemDto(originalName, patch);
  }

  deleteAttachItem(item: AttachItem): Promise<void> {
    return deleteAttachmentItemDto(item);
  }

  downloadAttachItem(item: AttachItem): Promise<Blob> {
    return downloadAttachmentItemDto(item);
  }

  downloadOwnerReassignmentTemplate(): Promise<Blob> { return downloadOwnerReassignmentTemplateDto(); }
  createOwnerReassignmentPreview(csv: string): Promise<OwnerReassignmentPreview> { return createOwnerReassignmentPreviewDto(csv); }
  getOwnerReassignmentPreview(previewId: string): Promise<OwnerReassignmentPreview> { return getOwnerReassignmentPreviewDto(previewId); }
  executeOwnerReassignmentPreview(preview: OwnerReassignmentPreview): Promise<OwnerReassignmentPreview> { return executeOwnerReassignmentPreviewDto(preview); }
  downloadOwnerReassignmentReport(previewId: string, kind: 'errors' | 'results'): Promise<Blob> { return downloadOwnerReassignmentReportDto(previewId, kind); }

  saveProduct(input: ProductWriteInput): Promise<Product> { return saveProductDto(input); }
  setProductEnabled(productId: number, enabled: boolean): Promise<Product> { return setProductEnabledDto(productId, enabled); }
  copyProduct(productId: number): Promise<Product> { return copyProductDto(productId); }
  deleteProduct(productId: number): Promise<void> { return deleteProductDto(productId); }
  saveServiceProduct(input: ProductWriteInput): Promise<SpProduct> { return saveServiceProductDto(input); }
  setServiceProductEnabled(productId: number, enabled: boolean): Promise<SpProduct> { return setServiceProductEnabledDto(productId, enabled); }
  copyServiceProduct(productId: number): Promise<SpProduct> { return copyServiceProductDto(productId); }
  archiveServiceProduct(productId: number): Promise<void> { return archiveServiceProductDto(productId); }
  saveCoupon(input: CouponWriteInput, publish: boolean): Promise<Coupon> { return saveCouponDto(input, publish); }
  setCouponPublished(couponId: number, published: boolean): Promise<Coupon> { return setCouponPublishedDto(couponId, published); }
  copyCoupon(couponId: number): Promise<Coupon> { return copyCouponDto(couponId); }
  archiveCoupon(couponId: number): Promise<void> { return archiveCouponDto(couponId); }
  deleteCoupon(couponId: number): Promise<void> { return deleteCouponDto(couponId); }

  /* ---------- 配置中心 ---------- */

  toggleConfigCategory(key: string, on: boolean): Promise<void> {
    void key; void on;
    return Promise.reject(new Error('后端能力未就绪：配置写入要求 route-bound Admin Action Token，当前 JSON DTO 未提供'));
  }

  saveConfigCategory(key: string, values: Record<string, string>, switches: Record<string, boolean>): Promise<void> {
    void key; void values; void switches;
    return Promise.reject(new Error('后端能力未就绪：配置 settings 是 closed allowlist 且要求 Admin Action Token，当前表单 DTO 不等价'));
  }

  checkConfigCategory(key: string): Promise<string> {
    void key;
    return Promise.reject(new Error('后端能力未就绪：配置检查要求 route-bound Admin Action Token，当前 JSON DTO 未提供'));
  }
}

/**
 * 生产入口见文件末尾；Mock 只由测试运行时显式注入。
 */
/**
 * 生产入口绝不回退到 Mock。DOM E2E 在创建 JSDOM 时显式注入此测试标记，
 * 以便继续验证模板绑定；浏览器运行时一律使用同源 HTTP transport。
 */
const runtime = globalThis as typeof globalThis & { __AICRM_TEST_MOCK__?: boolean };
export const api: AdminApi = runtime.__AICRM_TEST_MOCK__
  ? new MockApi()
  : new HttpApi({ baseUrl: typeof location === 'undefined' ? '' : location.origin });
