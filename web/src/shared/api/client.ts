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
  FunnelGridRow,
  FunnelView,
  ImageItem,
  MpItem,
  QuestionnaireOps,
  RadarEvent,
  RadarLink,
  RadarLinkInput,
  RadarMedia,
  TagGroup,
  WecomTag,
} from './types';
import { SEED_DB, deepCopy } from './mockData';
import { archiveAudiencePackage, archiveTagDto, copyAudiencePackageDto, deleteAudienceGroup as deleteAudienceGroupDto, queueTagSyncDto, readAdminPage, readRadarEvents, saveAudienceGroup as saveAudienceGroupDto, saveTagDto, saveTagGroupDto, setAudiencePackageRunning, setRadarEnabled, type AdminReadContext } from '../../api/admin';

/* ================= 接口定义 ================= */

export interface AdminApi {
  readonly mode: 'mock' | 'http';

  /** 聚合加载后台数据仓库 */
  loadDb(context?: AdminReadContext): Promise<AdminDb>;

  /* ---- 内容雷达 ---- */
  toggleRadarLink(id: number, enabled: boolean): Promise<void>;
  saveRadarLink(input: RadarLinkInput): Promise<RadarLink>;
  listRadarEvents(linkId: number): Promise<RadarEvent[]>;
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
  deleteImageItem(name: string): Promise<void>;
  saveMpItem(originalName: string | null, patch: Partial<MpItem> & { name: string }): Promise<void>;
  deleteMpItem(name: string): Promise<void>;
  saveAttachItem(originalName: string | null, patch: Partial<AttachItem> & { name: string }): Promise<void>;
  deleteAttachItem(name: string): Promise<void>;

  /* ---- 配置中心 ---- */
  toggleConfigCategory(key: string, on: boolean): Promise<void>;
  saveConfigCategory(key: string, values: Record<string, string>, switches: Record<string, boolean>): Promise<void>;
  checkConfigCategory(key: string): Promise<string>;
}

/* ================= 遗留壳 Adapter（待逐屏用 generated operation 替换） ================= */

export const ROUTES = {
  radarLinks: '/api/admin/radar-links',
  radarLink: (id: number | string) => `/api/admin/radar-links/${id}`,
  radarLinkEnable: (id: number | string) => `/api/admin/radar-links/${id}/enable`,
  radarLinkDisable: (id: number | string) => `/api/admin/radar-links/${id}/disable`,
  radarUploadImage: '/api/admin/radar-links/upload-image',
  radarUploadPdf: '/api/admin/radar-links/upload-pdf',
  aiReviewPlans: '/api/admin/ai-assist/review-plans',
  aiRecipients: (planId: number | string) => `/api/admin/ai-assist/review-plans/${planId}/recipients`,
  customers: '/api/admin/customers/',
  imageUpload: '/api/admin/image-library/upload',
  questionnaires: '/api/admin/questionnaires',
  funnelRows: '/api/admin/funnel/rows',
  funnelViews: '/api/admin/funnel/views',
  audiencePackages: '/api/admin/ai-audience/packages',
  audiencePackage: (id: number | string) => `/api/admin/ai-audience/packages/${id}`,
  audiencePackageGroups: '/api/admin/ai-audience/package-groups',
  audiencePackageGroup: (id: number | string) => `/api/admin/ai-audience/package-groups/${id}`,
  wecomTags: '/api/admin/wecom/tags',
  wecomTag: (id: number | string) => `/api/admin/wecom/tags/${id}`,
  wecomTagGroups: '/api/admin/wecom/tag-groups',
  wecomTagGroup: (id: number | string) => `/api/admin/wecom/tag-groups/${id}`,
  wecomTagsSync: '/api/admin/wecom/tags/sync',
  questionnaireOps: (id: number | string) => `/api/admin/questionnaires/${id}/operations`,
  imageLibrary: '/api/admin/image-library/items',
  mpLibrary: '/api/admin/miniprogram-library/items',
  attachLibrary: '/api/admin/attachment-library/items',
  configCategory: (key: string) => `/api/admin/config/categories/${key}`,
} as const;

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

  async deleteImageItem(name: string): Promise<void> {
    this.db.rows.images = this.db.rows.images.filter((x) => x.name !== name);
    this.persist();
    return delay(undefined);
  }

  async saveMpItem(originalName: string | null, patch: Partial<MpItem> & { name: string }): Promise<void> {
    this.upsertByName(this.db.rows.mpItems, originalName, patch);
    this.persist();
    return delay(undefined, 500);
  }

  async deleteMpItem(name: string): Promise<void> {
    this.db.rows.mpItems = this.db.rows.mpItems.filter((x) => x.name !== name);
    this.persist();
    return delay(undefined);
  }

  async saveAttachItem(originalName: string | null, patch: Partial<AttachItem> & { name: string }): Promise<void> {
    this.upsertByName(this.db.rows.attachItems, originalName, patch);
    this.persist();
    return delay(undefined, 500);
  }

  async deleteAttachItem(name: string): Promise<void> {
    this.db.rows.attachItems = this.db.rows.attachItems.filter((x) => x.name !== name);
    this.persist();
    return delay(undefined);
  }

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

  constructor(private opts: HttpApiOptions) {}

  private async req<T>(path: string, init?: RequestInit): Promise<T> {
    const headers: Record<string, string> = {
      ...(init?.headers as Record<string, string> | undefined),
    };
    if (this.opts.token) headers['Authorization'] = `Bearer ${this.opts.token}`;
    if (init?.body && !(init.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }
    const resp = await fetch(this.opts.baseUrl + path, { ...init, headers, credentials: 'include' });
    if (!resp.ok) throw new Error(`HTTP ${resp.status} ${resp.statusText} @ ${path}`);
    return (await resp.json()) as T;
  }

  async loadDb(context?: AdminReadContext): Promise<AdminDb> {
    // OpenAPI failure reaches the view's loading/error state; production never merges SEED_DB.
    return readAdminPage(context);
  }

  /* ---------- 内容雷达 ---------- */

  toggleRadarLink(id: number, enabled: boolean): Promise<void> {
    return setRadarEnabled(id, enabled);
  }

  async saveRadarLink(input: RadarLinkInput): Promise<RadarLink> {
    if (input.id !== undefined) {
      return this.req<RadarLink>(ROUTES.radarLink(input.id), {
        method: 'PUT',
        body: JSON.stringify(input),
      });
    }
    return this.req<RadarLink>(ROUTES.radarLinks, { method: 'POST', body: JSON.stringify(input) });
  }

  listRadarEvents(linkId: number): Promise<RadarEvent[]> {
    return readRadarEvents(linkId);
  }

  private async upload(path: string, file: File): Promise<RadarMedia> {
    const fd = new FormData();
    fd.append('file', file);
    await this.req<unknown>(path, { method: 'POST', body: fd });
    return { name: file.name, meta: `${file.type} · ${(file.size / 1048576).toFixed(1)} MB` };
  }

  uploadRadarImage(file: File): Promise<RadarMedia> {
    return this.upload(ROUTES.radarUploadImage, file);
  }

  uploadRadarPdf(file: File): Promise<RadarMedia> {
    return this.upload(ROUTES.radarUploadPdf, file);
  }

  /* ---------- AI 助手 ---------- */

  approveAiPlan(id: number): Promise<void> {
    return this.req<void>(`${ROUTES.aiReviewPlans}/${id}/approve`, { method: 'POST' });
  }

  rejectAiPlan(id: number): Promise<void> {
    return this.req<void>(`${ROUTES.aiReviewPlans}/${id}/reject`, { method: 'POST' });
  }

  listAiRecipients(planId: number): Promise<AiRecipient[]> {
    return this.req<AiRecipient[]>(ROUTES.aiRecipients(planId));
  }

  approveAiRecipient(planId: number, rcId: number): Promise<void> {
    return this.req<void>(`${ROUTES.aiRecipients(planId)}/${rcId}/approve`, { method: 'POST' });
  }

  rejectAiRecipient(planId: number, rcId: number): Promise<void> {
    return this.req<void>(`${ROUTES.aiRecipients(planId)}/${rcId}/reject`, { method: 'POST' });
  }

  updateRecipientNote(planId: number, rcId: number, taskIdx: number, note: string): Promise<void> {
    return this.req<void>(`${ROUTES.aiRecipients(planId)}/${rcId}/tasks/${taskIdx}/note`, {
      method: 'PUT',
      body: JSON.stringify({ note }),
    });
  }

  /* ---------- 漏斗 ---------- */

  listFunnelRows(): Promise<FunnelGridRow[]> {
    return this.req<FunnelGridRow[]>(ROUTES.funnelRows);
  }

  listFunnelViews(): Promise<FunnelView[]> {
    return this.req<FunnelView[]>(ROUTES.funnelViews);
  }

  saveFunnelViews(views: FunnelView[]): Promise<void> {
    return this.req<void>(ROUTES.funnelViews, { method: 'PUT', body: JSON.stringify(views) });
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
    return this.req<void>(ROUTES.questionnaireOps(qid), { method: 'PUT', body: JSON.stringify(ops) });
  }

  /* ---------- 素材库 ---------- */

  private saveLibraryItem<T extends { name: string }>(base: string, originalName: string | null, patch: Partial<T> & { name: string }): Promise<void> {
    if (originalName) {
      return this.req<void>(`${base}/${encodeURIComponent(originalName)}`, { method: 'PATCH', body: JSON.stringify(patch) });
    }
    return this.req<void>(base, { method: 'POST', body: JSON.stringify(patch) });
  }

  saveImageItem(originalName: string | null, patch: Partial<ImageItem> & { name: string }): Promise<void> {
    return this.saveLibraryItem(ROUTES.imageLibrary, originalName, patch);
  }

  deleteImageItem(name: string): Promise<void> {
    return this.req<void>(`${ROUTES.imageLibrary}/${encodeURIComponent(name)}`, { method: 'DELETE' });
  }

  saveMpItem(originalName: string | null, patch: Partial<MpItem> & { name: string }): Promise<void> {
    return this.saveLibraryItem(ROUTES.mpLibrary, originalName, patch);
  }

  deleteMpItem(name: string): Promise<void> {
    return this.req<void>(`${ROUTES.mpLibrary}/${encodeURIComponent(name)}`, { method: 'DELETE' });
  }

  saveAttachItem(originalName: string | null, patch: Partial<AttachItem> & { name: string }): Promise<void> {
    return this.saveLibraryItem(ROUTES.attachLibrary, originalName, patch);
  }

  deleteAttachItem(name: string): Promise<void> {
    return this.req<void>(`${ROUTES.attachLibrary}/${encodeURIComponent(name)}`, { method: 'DELETE' });
  }

  /* ---------- 配置中心 ---------- */

  toggleConfigCategory(key: string, on: boolean): Promise<void> {
    return this.req<void>(`${ROUTES.configCategory(key)}/${on ? 'enable' : 'disable'}`, { method: 'POST' });
  }

  saveConfigCategory(key: string, values: Record<string, string>, switches: Record<string, boolean>): Promise<void> {
    return this.req<void>(ROUTES.configCategory(key), {
      method: 'PUT',
      body: JSON.stringify({ values, switches }),
    });
  }

  async checkConfigCategory(key: string): Promise<string> {
    const r = await this.req<{ message?: string }>(`${ROUTES.configCategory(key)}/check`, { method: 'POST' });
    return r.message || '检查完成';
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
