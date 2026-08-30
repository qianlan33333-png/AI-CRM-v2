/**
 * 后台控制器（TypeScript 版）—— 模板页（mini-runtime）专用。
 * 雷达 / AI 助手 / 漏斗 / 周期商品数据页四个富交互页不走这里，见 sections/* 模块。
 * 逻辑 1:1 移植自设计原型 Component 类，差异仅在：
 *  - 多页架构：go.xxx 为真实页面跳转（x.html）
 *  - 数据经 AdminApi 加载（mock=sessionStorage，上线切 HttpApi）
 *
 * 本控制器同时承载各模板页的弹窗 / Tab / 分组状态与真实写穿动作：
 * 自动化运营分组弹窗、人群包编辑器面板、问卷运营配置、企微标签编辑组件、
 * 商品/周期商品/优惠券分享组件、三素材库弹窗、配置中心类目详情。
 */
import { PageBase, type StyleObj, type Vals } from '../shared/ui/runtime';
import type { AdminApi } from '../shared/api/client';
import type { AdminDb, AudienceSender, Channel, ChannelAcquisitionAsset, ChannelAcquisitionAssignmentInput, ChannelAcquisitionAssignee, ChannelAcquisitionPreview, ChannelEntrant, ChannelHistoryPage, OwnerReassignmentPreview, QuestionnaireOps, Tone } from '../shared/api/types';
import { deepCopy } from '../shared/api/mockData';
import { emptyAdminDb } from '../api/admin';
import { buildChannelFinalUrl, channelAcquisitionAssetReady, listAudienceTemplatesDto, previewAudienceTemplateDto, saveAudienceTemplateConfigurationDto, type AudienceTemplate, type AudienceTemplateEvaluation, type AudienceTemplateKey, type AudienceTemplateParameters } from '../api/admin';
import type { AdminDbWithMiniProgramList, AdminReadContext, ChannelWriteInput, CouponWriteInput, CustomerListQuery, GroupOpsWriteInput, MiniProgramListPage, QuestionnaireWriteInput } from '../api/admin';
import { toast, confirmBox, busy } from '../shared/ui/feedback';
import { openPicker, type PickerItem, type PickerOpts } from '../shared/ui/picker';
import { copyText } from './sections/util';
import { downloadQr, renderQr } from './sections/qr';
import { ownerReassignmentCsvFromFile } from './ownerReassignmentFile';
import { openGroupOpsDirectory } from './sections/groupOpsDirectory';
import { createWeComAcquisitionLink, deleteWeComAcquisitionLink, getWeComAcquisitionLink, listWeComAcquisitionLinks, reconcileWeComAcquisitionLink, updateWeComAcquisitionLink, type CustomerAcquisitionLink, type CustomerAcquisitionLinkInput, type WeComAcquisitionLinkWriteResult } from '../api/wecomAcquisitionLinks';

const ACCENT = '#3370ff';
const CHANNEL_HISTORY_PAGE_SIZE = 50;
const MINI_PROGRAM_PAGE_SIZE = 50;

type AdminState = {
  cstep: number;
  astep: number;
  saving: boolean;
  /** 当前打开的弹窗（'' = 无）：group / share / imgUpload / imgEdit / mpCreate / mpEdit / attUpload / attEdit / tag / record */
  modal: string;
  /* ---- 自动化运营 ---- */
  groupId: number;
  groupMode: '' | 'create' | 'edit';
  editingGroupId: number;
  /* ---- 人群包编辑器 ---- */
  apanel: number;
  audiencePreview: { configurationVersion: number; memberCount: number; emptyConfirmed: boolean } | null;
  audienceTemplates: AudienceTemplate[];
  audienceTemplateError: string;
  audienceTemplateKey: AudienceTemplateKey;
  audienceTemplateParametersText: string;
  audienceTemplatePreview: AudienceTemplateEvaluation | null;
  /* ---- 企微标签 ---- */
  tagGroupId: number;
  tagMode: '' | 'create-group' | 'create-tag' | 'edit-group' | 'edit-tag';
  editingTagId: number;
  tagQ: string;
  /* ---- 问卷运营配置 ---- */
  opsTab: number;
  postEnabled: boolean;
  postType: 'channel_qr' | 'redirect';
  pushEnabled: boolean;
  opsLogKeyword: string;
  opsLogStatus: string;
  opsLogScope: 'questionnaire' | 'global';
  /* ---- 分享组件 ---- */
  shareKind: string;
  shareTitle: string;
  shareUrl: string;
  shareCode: string;
  /* ---- 素材库 ---- */
  editingName: string;
  miniProgramQuery: string;
  miniProgramOffset: number;
  miniProgramLoading: boolean;
  miniProgramError: string;
  /* ---- 通用选择器草稿 ---- */
  /** 渠道表单 · 客服分配（null = 沿用种子） */
  cfStaff: PickerItem[] | null;
  /** 渠道表单 · 欢迎语素材已选 */
  cfMats: PickerItem[];
  /** 渠道表单 · 入渠标签（null = 默认 沙龙邀约/共学营） */
  cfTags: PickerItem[] | null;
  /** Agent 编辑 · 固定素材（null = 默认示例两条） */
  agMats: PickerItem[] | null;
  /** 负责人迁移 · 当前持久预览 */
  migFileName: string;
  migPreview: OwnerReassignmentPreview | null;
  migConfirmed: boolean;
  /** 问卷运营配置 · 绑定渠道码 code（'' = 未改） */
  opsChannelId: string;
  /** 商品/周期商品表单 · 引流渠道码 code */
  pfChannelId: string;
  spfChannelId: string;
  /** 客户列表筛选与 opaque cursor 导航 */
  customerFilters: CustomerListFilters;
  customerCursors: string[];
  customerPage: number;
  customerLoading: boolean;
  customerError: string;
  channelFormNotFound: boolean;
  channelDrawerOpen: boolean;
  channelDrawerLoading: boolean;
  channelDrawerError: string;
  channelDrawerChannel: Channel | null;
  channelDrawerEntrants: ChannelEntrant[];
  channelDrawerPreview: ChannelAcquisitionPreview | null;
  channelDrawerAssets: ChannelAcquisitionAsset[];
  channelDrawerPreviewError: string;
  channelDrawerAssetError: string;
  channelDrawerAssetBusy: boolean;
  channelFormPreview: ChannelAcquisitionPreview | null;
  channelFormAssets: ChannelAcquisitionAsset[];
  channelFormPreviewError: string;
  channelFormAssetError: string;
  channelFormAssetBusy: boolean;
  channelProviderLinks: string[];
  channelProviderLink: CustomerAcquisitionLink | null;
  channelProviderReceipt: WeComAcquisitionLinkWriteResult | null;
  channelProviderBusy: boolean;
  channelProviderError: string;
  channelHistory: ChannelHistoryPage | null;
  channelHistoryLoading: boolean;
  channelHistoryError: string;
  /** 优惠券列表仅对已加载行做本地筛选，不产生新的服务端请求。 */
  couponQuery: string;
  couponStatus: string;
};

/** 全部屏幕键（go 跳转表） */
const SCREENS = [
  'customers', 'customerDetail', 'questionnaires', 'questionnaireDetail', 'channels', 'channelForm',
  'orders', 'orderDetail', 'spProducts', 'coupons', 'couponForm', 'images', 'agents', 'agentEdit',
  'config', 'configDetail', 'automation', 'cycles', 'groupops', 'ai', 'radar', 'tags',
  'products', 'mpLib', 'attach', 'ownerMig', 'apidocs', 'productForm', 'spProductForm',
  'groupopsDetail', 'radarDetail', 'radarForm', 'aiDetail',
  'audienceEdit', 'cyclesDetail', 'questionnaireOps', 'spProductData', 'couponData',
] as const;

interface FbEl extends HTMLElement {
  __fbBusy?: boolean;
}

type CustomerListFilters = {
  keyword: string;
  owner: string;
  mobile: string;
  tag: string;
};

export class AdminController extends PageBase {
  override state: AdminState = {
    cstep: 1,
    astep: 1,
    saving: false,
    modal: '',
    groupId: 0,
    groupMode: '',
    editingGroupId: 0,
    apanel: 1,
    audiencePreview: null,
    audienceTemplates: [],
    audienceTemplateError: '',
    audienceTemplateKey: 'active_contacts',
    audienceTemplateParametersText: '{}',
    audienceTemplatePreview: null,
    tagGroupId: 1,
    tagMode: '',
    editingTagId: 0,
    tagQ: '',
    opsTab: 1,
    postEnabled: true,
    postType: 'channel_qr',
    pushEnabled: true,
    opsLogKeyword: '',
    opsLogStatus: '',
    opsLogScope: 'questionnaire',
    shareKind: '',
    shareTitle: '',
    shareUrl: '',
    shareCode: '',
    editingName: '',
    miniProgramQuery: '',
    miniProgramOffset: 0,
    miniProgramLoading: false,
    miniProgramError: '',
    cfStaff: null,
    cfMats: [],
    cfTags: null,
    agMats: null,
    migFileName: '',
    migPreview: null,
    migConfirmed: false,
    opsChannelId: '',
    pfChannelId: 'shalongyaoyue',
    spfChannelId: 'shalongyaoyue',
    customerFilters: { keyword: '', owner: '', mobile: '', tag: '' },
    customerCursors: [],
    customerPage: 0,
    customerLoading: false,
    customerError: '',
    channelFormNotFound: false,
    channelDrawerOpen: false,
    channelDrawerLoading: false,
    channelDrawerError: '',
    channelDrawerChannel: null,
    channelDrawerEntrants: [],
    channelDrawerPreview: null,
    channelDrawerAssets: [],
    channelDrawerPreviewError: '',
    channelDrawerAssetError: '',
    channelDrawerAssetBusy: false,
    channelFormPreview: null,
    channelFormAssets: [],
    channelFormPreviewError: '',
    channelFormAssetError: '',
    channelFormAssetBusy: false,
    channelProviderLinks: [],
    channelProviderLink: null,
    channelProviderReceipt: null,
    channelProviderBusy: false,
    channelProviderError: '',
    channelHistory: null,
    channelHistoryLoading: false,
    channelHistoryError: '',
    couponQuery: '',
    couponStatus: '',
  };

  db: AdminDb = emptyAdminDb();

  /** 发送人白名单草稿（添加发送人未保存前的本地行） */
  private sendersDraft: AudienceSender[] | null = null;
  /** 问卷运营配置 · 自定义参数草稿 */
  private paramsDraft: { key: string; value: string }[] | null = null;
  /** 全局日志仅在 HttpApi 真实读取成功后缓存；不以 Mock 代替。 */
  private globalQuestionnairePushLogs: AdminDb['rows']['qApply'] | null = null;
  private imageObjectUrls: string[] = [];

  constructor(
    private api: AdminApi,
    readonly page: string,
  ) {
    super();
  }

  /** 页面入口调用：加载数据仓库 → 重渲染 */
  async init(): Promise<void> {
    const resourceId = this.qs().get(this.page === 'configDetail' ? 'cat' : 'id') || undefined;
    const context: AdminReadContext = { page: this.page, id: resourceId };
    if (this.page === 'customers') {
      const parsed = this.parseCustomerListQuery(this.state.customerFilters);
      if ('error' in parsed) throw new Error(parsed.error);
      context.customerList = parsed.query;
    }
    if (this.page === 'mpLib') context.miniProgramList = this.miniProgramListQuery(this.state.miniProgramOffset, this.state.miniProgramQuery);
    try {
      this.db = await this.api.loadDb(context);
      if (this.page === 'mpLib') {
        if (this.api.mode === 'http') this.validateMiniProgramPage(this.db, context.miniProgramList!);
        this.state.miniProgramLoading = false;
        this.state.miniProgramError = '';
      }
    } catch (error) {
      if (this.page !== 'mpLib') throw error;
      this.db = emptyAdminDb();
      this.state.miniProgramError = error instanceof Error ? error.message : '小程序素材读取失败';
    }
    if (this.page === 'audienceEdit') {
      this.state.audienceTemplateError = '';
      this.state.audienceTemplatePreview = null;
      this.state.audienceTemplates = [];
      if (this.api.mode === 'http') {
        try {
          this.state.audienceTemplates = await listAudienceTemplatesDto();
          if (!this.state.audienceTemplates.some((template) => template.key === this.state.audienceTemplateKey)) this.state.audienceTemplateKey = this.state.audienceTemplates[0]?.key || 'active_contacts';
        } catch (error) {
          this.state.audienceTemplateError = error instanceof Error ? error.message : 'Audience 模板目录读取失败';
        }
      }
    }
    if (this.page === 'channelForm') {
      this.state.channelFormNotFound = Boolean(resourceId && this.db.rows.channels.length === 0);
      this.state.channelHistory = null;
      this.state.channelHistoryLoading = false;
      this.state.channelHistoryError = '';
      this.state.channelProviderLinks = [];
      this.state.channelProviderLink = null;
      this.state.channelProviderReceipt = null;
      this.state.channelProviderBusy = false;
      this.state.channelProviderError = '';
    }
    if (this.page === 'channelForm' && resourceId && !this.state.channelFormNotFound) {
      const channelId = Number(resourceId);
      if (Number.isSafeInteger(channelId) && channelId > 0) await this.loadChannelFormAcquisitionData(channelId);
    } else if (this.page === 'channelForm') {
      this.state.channelFormPreview = null;
      this.state.channelFormAssets = [];
      this.state.channelFormPreviewError = '';
      this.state.channelFormAssetError = '';
    }
    if (this.page === 'customers') {
      this.state.customerPage = 0;
      this.state.customerCursors = [];
      this.state.customerLoading = false;
      this.state.customerError = '';
    }
    if (this.page === 'images' && this.api.mode === 'http') {
      for (const url of this.imageObjectUrls) URL.revokeObjectURL(url);
      this.imageObjectUrls = [];
      await Promise.all(this.db.rows.images.map(async (item) => {
        try {
          const url = URL.createObjectURL(await this.api.getImageThumbnail(item));
          this.imageObjectUrls.push(url);
          item.thumbnailUrl = url;
        } catch (error) {
          item.thumbnailError = error instanceof Error ? error.message : '缩略图读取失败';
        }
      }));
    }
    const previewId = this.page === 'ownerMig' ? this.qs().get('id') : null;
    if (previewId) this.state.migPreview = await this.api.getOwnerReassignmentPreview(previewId);
    // 问卷运营配置：首次进入把本地开关态同步为已保存值
    const ops = this.currentOps();
    if (ops && this.page === 'questionnaireOps') {
      this.state.postEnabled = ops.postEnabled;
      this.state.postType = ops.postType;
      this.state.pushEnabled = ops.pushEnabled;
      this.state.opsLogScope = 'questionnaire';
      this.globalQuestionnairePushLogs = null;
    }
    if (this.__render) this.__render();
  }

  /* ================= 导航 ================= */

  private goto(page: string, query = ''): void {
    location.href = page + '.html' + query;
  }

  private qs(): URLSearchParams {
    return new URLSearchParams(location.search);
  }

  private readCustomerFilters(): CustomerListFilters {
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | null)?.value.trim() || '';
    return { keyword: value('fCustomerKeyword'), owner: value('fCustomerOwner'), mobile: value('fCustomerMobile'), tag: value('fCustomerTag') };
  }

  private parseCustomerListQuery(filters: CustomerListFilters, cursor?: string): { query: CustomerListQuery } | { error: string } {
    const query: CustomerListQuery = {};
    if (filters.keyword) query.keyword = filters.keyword;
    if (filters.mobile) {
      if (!/^\+[1-9][0-9]{1,14}$/.test(filters.mobile)) return { error: '手机号必须是 E.164 格式，例如 +8613800000000' };
      query.mobile = filters.mobile;
    }
    if (filters.owner) {
      const ownerStaffId = Number(filters.owner);
      if (!Number.isSafeInteger(ownerStaffId) || ownerStaffId < 1) return { error: '负责人必须填写正整数 staff_id' };
      query.ownerStaffId = ownerStaffId;
    }
    if (filters.tag) {
      const tagId = Number(filters.tag);
      if (!Number.isSafeInteger(tagId) || tagId < 1) return { error: '标签必须填写正整数 tag_id' };
      query.tagId = tagId;
    }
    if (cursor) query.cursor = cursor;
    return { query };
  }

  private async loadCustomerPage(page: number, cursor: string | undefined, cursorStack: string[], filters = this.state.customerFilters): Promise<void> {
    if (this.state.customerLoading) return;
    const parsed = this.parseCustomerListQuery(filters, cursor);
    if ('error' in parsed) {
      this.setState({ customerError: parsed.error });
      return;
    }
    this.db = emptyAdminDb();
    this.setState({ customerLoading: true, customerError: '' });
    try {
      this.db = await this.api.loadDb({ page: 'customers', customerList: parsed.query });
      this.setState({ customerPage: page, customerCursors: cursorStack, customerLoading: false, customerError: '' });
    } catch (error) {
      this.db = emptyAdminDb();
      this.setState({ customerLoading: false, customerError: error instanceof Error ? error.message : '客户列表读取失败' });
    }
  }

  private queryCustomers(): void {
    if (this.state.customerLoading) return;
    const filters = this.readCustomerFilters();
    const parsed = this.parseCustomerListQuery(filters);
    if ('error' in parsed) {
      this.setState({ customerFilters: filters, customerError: parsed.error });
      return;
    }
    this.setState({ customerFilters: filters, customerCursors: [], customerPage: 0, customerError: '' });
    void this.loadCustomerPage(0, undefined, [], filters);
  }

  private clearCustomers(): void {
    if (this.state.customerLoading) return;
    const filters: CustomerListFilters = { keyword: '', owner: '', mobile: '', tag: '' };
    this.setState({ customerFilters: filters, customerCursors: [], customerPage: 0, customerError: '' });
    void this.loadCustomerPage(0, undefined, [], filters);
  }

  private nextCustomerPage(): void {
    if (this.state.customerLoading) return;
    const cursor = this.db.customerList.nextCursor;
    if (!cursor) return;
    const page = this.state.customerPage + 1;
    const cursorStack = this.state.customerCursors.slice();
    cursorStack[page - 1] = cursor;
    void this.loadCustomerPage(page, cursor, cursorStack);
  }

  private previousCustomerPage(): void {
    if (this.state.customerLoading || this.state.customerPage === 0) return;
    const page = this.state.customerPage - 1;
    const cursor = page === 0 ? undefined : this.state.customerCursors[page - 1];
    void this.loadCustomerPage(page, cursor, this.state.customerCursors.slice(0, page));
  }

  private miniProgramListQuery(offset: number, q: string): NonNullable<AdminReadContext['miniProgramList']> {
    return { limit: MINI_PROGRAM_PAGE_SIZE, offset, ...(q.trim() ? { q: q.trim() } : {}) };
  }

  private miniProgramPage(): MiniProgramListPage | undefined {
    return (this.db as AdminDbWithMiniProgramList).miniProgramList;
  }

  private validateMiniProgramPage(db: AdminDb, query: NonNullable<AdminReadContext['miniProgramList']>): MiniProgramListPage {
    const page = (db as AdminDbWithMiniProgramList).miniProgramList;
    if (!page || page.limit !== query.limit || page.offset !== query.offset || page.q !== (query.q || '') || !Number.isSafeInteger(page.total) || page.total < 0) throw new Error('小程序素材分页响应无效');
    return page;
  }

  private async loadMiniProgramPage(offset: number, q = this.state.miniProgramQuery): Promise<void> {
    if (this.state.miniProgramLoading) return;
    this.db = emptyAdminDb();
    this.setState({ miniProgramQuery: q.trim(), miniProgramOffset: offset, miniProgramLoading: true, miniProgramError: '' });
    try {
      const db = await this.api.loadDb({ page: 'mpLib', miniProgramList: this.miniProgramListQuery(offset, q) });
      const page = this.validateMiniProgramPage(db, this.miniProgramListQuery(offset, q));
      this.db = db;
      this.setState({ miniProgramQuery: q.trim(), miniProgramOffset: page.offset, miniProgramLoading: false, miniProgramError: '' });
    } catch (error) {
      this.db = emptyAdminDb();
      this.setState({ miniProgramLoading: false, miniProgramError: error instanceof Error ? error.message : '小程序素材读取失败' });
    }
  }

  private queryMiniPrograms(): void {
    const q = (document.getElementById('fMpQuery') as HTMLInputElement | null)?.value || '';
    void this.loadMiniProgramPage(0, q);
  }

  private clearMiniPrograms(): void {
    void this.loadMiniProgramPage(0, '');
  }

  private previousMiniProgramPage(): void {
    const page = this.miniProgramPage();
    if (!page || page.offset === 0) return;
    void this.loadMiniProgramPage(Math.max(0, page.offset - page.limit));
  }

  private nextMiniProgramPage(): void {
    const page = this.miniProgramPage();
    if (!page || page.offset + this.db.rows.mpItems.length >= page.total) return;
    void this.loadMiniProgramPage(page.offset + page.limit);
  }

  private pageId(): number {
    const raw = Number(this.qs().get('id') || '');
    return raw || 0;
  }

  /* ================= 样式助手 ================= */

  chip(tone: Tone): StyleObj {
    const m: Record<Tone, [string, string]> = {
      ok: ['#EBF9EC', '#2EA121'],
      blue: ['#EFF4FF', '#245BDB'],
      warn: ['#FFF7E8', '#D97917'],
      red: ['#FDECEE', '#D83931'],
      gray: ['#F2F3F5', '#646A73'],
      purple: ['#F4EDFF', '#7F3BF5'],
    };
    const c = m[tone] || m.gray;
    return {
      display: 'inline-flex', alignItems: 'center', height: '22px', padding: '0 8px',
      borderRadius: '4px', background: c[0], color: c[1], fontSize: '12px', whiteSpace: 'nowrap',
    };
  }

  sw(on: boolean, accent: string): { knob: StyleObj; track: StyleObj } {
    return {
      knob: {
        position: 'absolute', top: '2px', left: on ? '18px' : '2px', width: '14px', height: '14px',
        borderRadius: '50%', background: '#fff', transition: 'left .16s ease',
        boxShadow: '0 1px 2px rgba(0,0,0,.15)',
      },
      track: {
        position: 'relative', display: 'inline-block', width: '34px', height: '18px',
        borderRadius: '9px', background: on ? accent : '#DEE0E3', cursor: 'pointer', flex: 'none',
      },
    };
  }

  private inputStyle(w = '100%'): StyleObj {
    return {
      height: '32px', width: w, maxWidth: '100%', border: '1px solid #DEE0E3', borderRadius: '6px',
      padding: '0 10px', fontSize: '13px', background: '#fff', color: '#1F2329',
    };
  }

  /* ================= 通用弹窗 ================= */

  closeModal(): void {
    this.setState({ modal: '', editingName: '' });
    this.sendersDraft = null;
  }

  /* ================= 分享组件（商品 / 周期商品 / 优惠券 / 问卷共用） ================= */

  openShare(kind: string, title: string, code: string, path?: string): void {
    if (this.api.mode === 'http' && !path) {
      toast(`后端能力未就绪：${kind}暂无可用公开分享地址`, true);
      return;
    }
    this.setState({
      modal: 'share',
      shareKind: kind,
      shareTitle: title,
      shareCode: code,
      shareUrl: path ? new URL(path, location.origin).toString() : 'https://mock.invalid/s/' + code,
    });
    const el = document.getElementById('shareQrBox');
    if (el) renderQr(el, this.state.shareUrl, `${kind}分享`);
  }

  copyShareLink(): void {
    copyText(this.state.shareUrl, toast);
  }

  previewShareLink(): void {
    window.open(this.state.shareUrl, '_blank', 'noopener,noreferrer');
  }

  /* ================= 自动化运营 · 人群包 ================= */

  private audienceGroupsVals(accent: string): Record<string, unknown>[] {
    const all = [{ id: 0, name: '未分组' }, ...this.db.audienceGroups];
    return all.map((g) => {
      const on = this.state.groupId === g.id;
      return {
        ...g,
        count: this.db.audiencePackages.filter((p) => p.groupId === g.id).length,
        pick: () => this.setState({ groupId: g.id }),
        box: {
          display: 'grid', gridTemplateColumns: 'minmax(0,1fr) auto', gap: '10px', alignItems: 'center',
          minHeight: '42px', padding: '9px 12px', borderRadius: '8px', cursor: 'pointer',
          border: on ? '1px solid #528BFF' : '1px solid #DEE0E3',
          background: on ? '#EEF4FF' : '#fff',
          color: on ? '#1849A9' : '#344054',
        } as StyleObj,
        cnt: { fontSize: '12px', color: on ? '#1849A9' : '#98A2B3' } as StyleObj,
      };
    });
  }

  private openGroupModal(mode: 'create' | 'edit'): void {
    this.setState({ modal: 'group', groupMode: mode, editingGroupId: this.state.groupId });
    if (mode === 'edit') {
      const g = this.db.audienceGroups.find((x) => x.id === this.state.groupId);
      const input = document.getElementById('fGroupName') as HTMLInputElement | null;
      if (input && g) input.value = g.name;
    }
  }

  private saveGroup(): void {
    const input = document.getElementById('fGroupName') as HTMLInputElement | null;
    const name = (input?.value || '').trim();
    if (!name) {
      toast('请输入分组名称', true);
      return;
    }
    const mode = this.state.groupMode;
    const id = mode === 'edit' ? this.state.editingGroupId : undefined;
    void this.api.saveAudienceGroup({ id, name }).then(() => {
      toast(mode === 'edit' ? '分组已重命名' : '分组「' + name + '」已创建');
      this.setState({ modal: '' });
      void this.init();
    });
  }

  private deleteGroup(): void {
    const g = this.db.audienceGroups.find((x) => x.id === this.state.groupId);
    if (!g) return;
    const count = this.db.audiencePackages.filter((p) => p.groupId === g.id).length;
    if (count > 0) {
      confirmBox('无法删除', '分组「' + g.name + '」下还有 ' + count + ' 个人群包，请先移出或删除这些人群包。', '知道了');
      return;
    }
    confirmBox('删除分组', '确认删除分组「' + g.name + '」？该操作不可撤销。', '确认删除', true, () => {
      void this.api.deleteAudienceGroup(g.id).then(() => {
        toast('分组已删除');
        this.setState({ groupId: 0 });
        void this.init();
      });
    });
  }

  /* ================= 人群包编辑器（audienceEdit） ================= */

  private audiencePkg() {
    return this.db.audiencePackages.find((p) => p.id === this.pageId()) || this.db.audiencePackages[0];
  }

  private aeSelectOpts(cur: string, opts: [string, string][]): Record<string, unknown>[] {
    return opts.map(([v, t]) => ({ v, t, sel: v === cur, not: v !== cur }));
  }

  private audienceWriteInput(): import('../api/admin').AudiencePackageWriteInput | null {
    const pkg = this.audiencePkg();
    if (!pkg) return null;
    const val = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null)?.value || '';
    let definition: import('../api/admin').AudiencePackageWriteInput['definition'];
    try { definition = JSON.parse(val('aeDef')) as import('../api/admin').AudiencePackageWriteInput['definition']; }
    catch { toast('SegmentDefinition 不是有效 JSON', true); return null; }
    const refreshMode = val('aeRefreshMode') === 'scheduled' ? 'scheduled' : 'manual';
    const refreshCron = val('aeRefreshCron').trim() || null;
    if (refreshMode === 'scheduled' && !refreshCron) { toast('定时刷新必须填写 cron', true); return null; }
    return { id: pkg.id, name: val('aeName') || pkg.name, definition, groupId: Number(val('aeGroup')) || null, refreshMode, refreshCron };
  }

  private audienceConfigurationAllowed(): boolean {
    if (!this.audiencePkg()?.running) return true;
    toast('当前人群包为 active；请先停止后再保存或预览本地配置', true);
    return false;
  }

  private saveAudienceBasic(): void {
    const input = this.audienceWriteInput();
    const pkg = this.audiencePkg();
    if (!input || !pkg || !this.audienceConfigurationAllowed()) return;
    void this.api
      .saveAudiencePackage(input)
      .then((saved) => {
        Object.assign(pkg, saved);
        this.setState({ audiencePreview: null });
        toast('基础配置已保存');
      }).catch((error) => toast(error instanceof Error ? error.message : '人群包保存失败', true));
  }

  private bindAutomation(name: string): void {
    const pkg = this.audiencePkg();
    if (!pkg) return;
    const id = Number(name);
    if (!Number.isInteger(id) || id < 1) { toast('请输入有效 automation_agent_id', true); return; }
    void this.api.setAudienceBinding(pkg.id, id).then(() => {
      toast('已绑定自动化 Agent #' + id);
      void this.init();
    }).catch((error) => toast(error instanceof Error ? error.message : '自动化绑定失败', true));
  }

  private unbindAutomation(): void {
    const pkg = this.audiencePkg();
    if (!pkg || !pkg.boundAutomation) return;
    confirmBox('解除绑定', '解除后该人群包将停止自动化触达，确认继续？', '解除绑定', true, () => {
    void this.api.setAudienceBinding(pkg.id, null).then(() => {
      toast('已解除绑定');
      void this.init();
    }).catch((error) => toast(error instanceof Error ? error.message : '解除绑定失败', true));
    });
  }

  private addSender(): void {
    const pkg = this.audiencePkg();
    if (!pkg) return;
    const base = this.db.audienceSenders[pkg.id] || [];
    void this.pick({ kind: 'members', subtitle: '加入发送人白名单（保存后生效）' }).then((r) => {
      if (!r || !r.length) return;
      if (!this.sendersDraft) this.sendersDraft = [];
      r.forEach((m) => {
        this.sendersDraft!.push({
          priority: base.length + this.sendersDraft!.length + 1,
          userid: m.uid || m.id,
          rule: '默认',
          status: '待生效',
        });
      });
      this.setState({});
      toast('已添加 ' + r.length + ' 位发送人（保存后生效）');
    });
  }

  private saveSenders(): void {
    const pkg = this.audiencePkg();
    if (!pkg) return;
    const raw = (document.getElementById('aeSenders') as HTMLTextAreaElement | null)?.value || '';
    const ids = raw.split(/[\s,，]+/).map((value) => value.trim()).filter(Boolean);
    if (ids.length > 5 || new Set(ids).size !== ids.length) { toast('发送人最多 5 位且不能重复', true); return; }
    void this.api.replaceAudienceSenders(pkg.id, ids.map((sender_userid, index) => ({ sender_userid, sort_order: index + 1, is_enabled: true }))).then(() => { this.sendersDraft = null; toast('发送人白名单已保存'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '发送人保存失败', true));
  }

  private saveAudienceBinding(): void { this.bindAutomation((document.getElementById('aeAutomationId') as HTMLInputElement | null)?.value || ''); }
  private snapshotAudience(): void {
    const pkg = this.audiencePkg();
    if (!pkg || !this.audienceConfigurationAllowed()) return;
    void this.api.snapshotAudienceConfiguration(pkg.id).then((version) => {
      pkg.configurationVersion = version;
      this.setState({ audiencePreview: null });
      toast(`配置版本 v${version} 已保存`);
    }).catch((error) => toast(error instanceof Error ? error.message : '配置版本保存失败', true));
  }

  private saveAndPreviewAudience(): void {
    const input = this.audienceWriteInput();
    const pkg = this.audiencePkg();
    if (!input || !pkg || !this.audienceConfigurationAllowed()) return;
    void this.api.saveAudiencePackage(input)
      .then((saved) => {
        Object.assign(pkg, saved);
        return this.api.snapshotAudienceConfiguration(pkg.id);
      })
      .then(() => this.api.previewAudienceConfiguration(pkg.id))
      .then((result) => this.showAudiencePreview(pkg, result, '已保存新配置版本并预览'))
      .catch((error) => toast(error instanceof Error ? error.message : '保存并预览失败', true));
  }

  private audienceTemplateInput(): { templateKey: AudienceTemplateKey; parameters: AudienceTemplateParameters } | null {
    const key = (document.getElementById('aeTemplateKey') as HTMLSelectElement | null)?.value as AudienceTemplateKey | undefined;
    if (!key || !this.state.audienceTemplates.some((template) => template.key === key)) {
      toast('请选择当前 V2 模板目录中的模板', true);
      return null;
    }
    const raw = (document.getElementById('aeTemplateParams') as HTMLTextAreaElement | null)?.value || '{}';
    let parsed: unknown;
    try { parsed = JSON.parse(raw); } catch { toast('模板参数不是有效 JSON', true); return null; }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) { toast('模板参数必须是 JSON 对象', true); return null; }
    const parameters = parsed as AudienceTemplateParameters;
    const template = this.state.audienceTemplates.find((item) => item.key === key);
    if (template?.parameters.some((parameter) => parameter.required && !Array.isArray((parameters as Record<string, unknown>)[parameter.key]))) {
      toast('当前模板的必填参数必须填写正整数数组', true);
      return null;
    }
    return { templateKey: key, parameters };
  }

  private previewAudienceTemplate(): void {
    const pkg = this.audiencePkg();
    const input = this.audienceTemplateInput();
    if (!pkg || !input || this.api.mode !== 'http' || !this.audienceConfigurationAllowed()) return;
    void previewAudienceTemplateDto(pkg.id, input)
      .then((result) => {
        this.setState({ audienceTemplateKey: input.templateKey, audienceTemplateParametersText: JSON.stringify(input.parameters, null, 2), audienceTemplatePreview: result });
        toast(`模板预览完成：${result.memberCount} 人`);
      })
      .catch((error) => toast(error instanceof Error ? error.message : 'Audience 模板预览失败', true));
  }

  private saveAudienceTemplate(): void {
    const pkg = this.audiencePkg();
    const input = this.audienceTemplateInput();
    if (!pkg || !input || this.api.mode !== 'http' || !this.audienceConfigurationAllowed()) return;
    const expectedPackageVersion = pkg.packageVersion;
    if (typeof expectedPackageVersion !== 'number' || !Number.isSafeInteger(expectedPackageVersion) || expectedPackageVersion < 1) { toast('当前人群包缺少有效包版本，已拒绝保存模板', true); return; }
    void saveAudienceTemplateConfigurationDto(pkg.id, {
      ...input,
      expectedPackageVersion,
      expectedConfigurationVersion: pkg.configurationVersion || 0,
    }).then((result) => {
      pkg.packageVersion = result.packageVersion;
      pkg.version = `v${result.packageVersion}`;
      pkg.configurationVersion = result.configurationVersion;
      pkg.definition = JSON.stringify(result.definition, null, 2);
      this.setState({ audienceTemplateKey: input.templateKey, audienceTemplateParametersText: JSON.stringify(input.parameters, null, 2), audienceTemplatePreview: result });
      toast(`模板配置已保存：${result.memberCount} 人；仅本地配置`);
    }).catch((error) => toast(error instanceof Error ? error.message : 'Audience 模板保存失败', true));
  }

  private previewAudience(): void {
    const pkg = this.audiencePkg();
    if (!pkg || !this.audienceConfigurationAllowed()) return;
    void this.api.previewAudienceConfiguration(pkg.id)
      .then((result) => this.showAudiencePreview(pkg, result, '已预览当前已保存配置'))
      .catch((error) => toast(error instanceof Error ? error.message : '配置预览失败', true));
  }

  private showAudiencePreview(pkg: NonNullable<ReturnType<AdminController['audiencePkg']>>, result: import('../api/admin').AudienceEvaluation, success: string): void {
    pkg.configurationVersion = result.configurationVersion;
    const preview = { configurationVersion: result.configurationVersion, memberCount: result.memberCount, emptyConfirmed: result.memberCount !== 0 };
    this.setState({ audiencePreview: preview });
    if (result.memberCount !== 0) {
      toast(`${success}：${result.memberCount} 人 · 配置 v${result.configurationVersion}`);
      return;
    }
    confirmBox('确认空人群预览', `配置 v${result.configurationVersion} 的预览结果为 0 人。不会创建群发或调用 Provider；若要物化空人群，必须在此明确确认。`, '确认空人群', true, () => {
      this.setState({ audiencePreview: { ...preview, emptyConfirmed: true } });
      toast('已确认空人群；仍需单独确认物化本地成员事实');
    });
  }

  private materializeAudience(): void {
    const pkg = this.audiencePkg();
    const preview = this.state.audiencePreview;
    if (!pkg || !this.audienceConfigurationAllowed()) return;
    if (!preview) { toast('请先保存并预览当前配置', true); return; }
    if (preview.memberCount === 0 && !preview.emptyConfirmed) {
      toast('空人群需先明确确认，已拒绝物化', true);
      return;
    }
    const countText = preview.memberCount === 0 ? '当前已确认空人群（0 人）' : `当前预览 ${preview.memberCount} 人`;
    confirmBox('物化人群成员', `${countText} · 配置 v${preview.configurationVersion}。该操作只写入本地 Segment 成员事实，不会触发企微群发或调用 Provider。确认继续？`, '确认物化', false, () => {
      void this.api.materializeAudienceConfiguration(pkg.id).then((result) => {
        this.setState({ audiencePreview: { configurationVersion: result.configurationVersion, memberCount: result.memberCount, emptyConfirmed: result.memberCount !== 0 || preview.emptyConfirmed } });
        toast(`物化完成：${result.memberCount} 人 · 未触发外部群发`);
      }).catch((error) => toast(error instanceof Error ? error.message : '人群物化失败', true));
    });
  }

  /* ================= 通用选择器接入 ================= */

  private pick(opts: PickerOpts): Promise<PickerItem[] | null> {
    return openPicker(this.api, opts);
  }

  private channelCopyValue(channel: Channel | null | undefined): string {
    return [channel?.copyText, channel?.shareUrl, channel?.finalUrl, channel?.linkUrl, channel?.qrUrl].find((value) => Boolean(value?.trim()))?.trim() || '';
  }

  private channelShareValue(channel: Channel | null | undefined): string {
    return [channel?.shareUrl, channel?.finalUrl, channel?.linkUrl, channel?.copyText, channel?.qrUrl].find((value) => Boolean(value?.trim()))?.trim() || '';
  }

  private copyChannelLink(channel: Channel | null | undefined): void {
    const value = this.channelCopyValue(channel);
    if (!value) return toast('当前渠道没有可复制链接', true);
    copyText(value, (message, error) => toast(message, error));
  }

  private shareChannelLink(channel: Channel | null | undefined): void {
    const value = this.channelShareValue(channel);
    if (!value) return toast('当前渠道没有可分享链接', true);
    if (typeof navigator !== 'undefined' && typeof navigator.share === 'function') {
      void navigator.share({ title: channel?.name || '企微获客助手链接', url: value }).catch(() => this.copyChannelLink(channel));
      return;
    }
    this.copyChannelLink(channel);
  }

  private async loadChannelAcquisitionAssets(channelId: number): Promise<ChannelAcquisitionAsset[]> {
    const items = (await this.api.listChannelAcquisitionAssets(channelId)).slice().sort((a, b) => b.assetVersion - a.assetVersion || b.updatedAt.localeCompare(a.updatedAt));
    const latest = items[0];
    if (!latest?.effectId) return items;
    try {
      const current = await this.api.getChannelAcquisitionAsset(channelId, latest.effectId);
      return [current, ...items.filter((item) => item.effectId !== current.effectId)];
    } catch {
      return items;
    }
  }

  private channelAssetKindLabel(kind: ChannelAcquisitionAsset['kind']): string {
    return kind === 'contact_way_qrcode' ? '二维码' : '获客链接';
  }

  private channelAssetStatusLabel(asset: ChannelAcquisitionAsset | null | undefined): string {
    if (!asset) return '尚未申请';
    if (asset.state === 'final_failed') return '执行失败';
    if (asset.state === 'outcome_unknown') return '结果未知，需对账';
    if (asset.state === 'executed') return channelAcquisitionAssetReady(asset) ? '已执行' : '已执行但未返回受控资产地址';
    if (asset.state === 'reconciled') return channelAcquisitionAssetReady(asset) ? '已对账' : '已对账但未返回受控资产地址';
    return '已排队';
  }

  private channelAssetOpen(asset: ChannelAcquisitionAsset | null | undefined): void {
    if (!channelAcquisitionAssetReady(asset)) return toast('资产尚未执行完成或服务端未返回受控地址', true);
    window.open(asset!.kind === 'contact_way_qrcode' ? asset!.downloadUrl : asset!.assetUrl, '_blank', 'noopener');
  }

  private channelAssetDownload(asset: ChannelAcquisitionAsset | null | undefined): void {
    if (!channelAcquisitionAssetReady(asset)) return toast('资产尚未执行完成或服务端未返回受控地址', true);
    const anchor = document.createElement('a');
    anchor.href = asset!.kind === 'contact_way_qrcode' ? asset!.downloadUrl! : asset!.assetUrl!;
    anchor.download = `${asset!.kind}-${asset!.assetVersion}`;
    anchor.target = '_blank';
    anchor.rel = 'noopener';
    anchor.click();
  }

  private channelAssetCopy(asset: ChannelAcquisitionAsset | null | undefined): void {
    if (!channelAcquisitionAssetReady(asset)) return toast('资产尚未执行完成或服务端未返回受控地址', true);
    copyText(asset!.kind === 'contact_way_qrcode' ? asset!.downloadUrl! : asset!.assetUrl!, (message, error) => toast(message, error));
  }

  private requestChannelAsset(channelId: number | undefined, kind: ChannelAcquisitionAsset['kind'], target: 'drawer' | 'form'): void {
    if (!channelId || !Number.isSafeInteger(channelId) || channelId < 1) return toast('渠道缺少有效服务端 ID', true);
    const busyKey = target === 'drawer' ? 'channelDrawerAssetBusy' : 'channelFormAssetBusy';
    const errorKey = target === 'drawer' ? 'channelDrawerAssetError' : 'channelFormAssetError';
    const assetsKey = target === 'drawer' ? 'channelDrawerAssets' : 'channelFormAssets';
    this.setState({ [busyKey]: true, [errorKey]: '' });
    void this.api.publishChannelAcquisitionAsset(channelId, kind).then(async (queued) => {
      const currentAssets = target === 'drawer' ? this.state.channelDrawerAssets : this.state.channelFormAssets;
      this.setState({ [assetsKey]: [queued, ...currentAssets.filter((item) => item.effectId !== queued.effectId)], [busyKey]: false });
      toast(`${this.channelAssetKindLabel(kind)}已排队；未证明 Provider 执行`);
      try {
        const assets = await this.loadChannelAcquisitionAssets(channelId);
        this.setState({ [assetsKey]: assets.some((item) => item.effectId === queued.effectId) ? assets : [queued, ...assets] });
      } catch (error) {
        this.setState({ [errorKey]: error instanceof Error ? error.message : '资产状态读取失败' });
      }
    }).catch((error) => {
      this.setState({ [busyKey]: false, [errorKey]: error instanceof Error ? error.message : '资产申请失败' });
      toast(error instanceof Error ? error.message : '资产申请失败', true);
    });
  }

  private openChannelDrawer(channelId: number | undefined): void {
    if (!channelId || !Number.isSafeInteger(channelId) || channelId < 1) return toast('渠道缺少有效服务端 ID', true);
    this.setState({ channelDrawerOpen: true, channelDrawerLoading: true, channelDrawerError: '', channelDrawerChannel: null, channelDrawerEntrants: [], channelDrawerPreview: null, channelDrawerAssets: [], channelDrawerPreviewError: '', channelDrawerAssetError: '' });
    void (async () => {
      let channel: Channel;
      try {
        channel = await this.api.getChannel(channelId);
      } catch (error) {
        this.setState({ channelDrawerLoading: false, channelDrawerError: error instanceof Error ? error.message : '渠道详情读取失败' });
        return;
      }
      const [entrants, preview, assets] = await Promise.allSettled([
        this.api.listChannelEntrants(channelId),
        this.api.getChannelAcquisitionPreview(channelId),
        this.loadChannelAcquisitionAssets(channelId),
      ]);
      this.setState({
        channelDrawerLoading: false,
        channelDrawerChannel: channel,
        channelDrawerEntrants: entrants.status === 'fulfilled' ? entrants.value : [],
        channelDrawerError: entrants.status === 'rejected' ? '近期进入用户读取失败，当前显示空列表' : '',
        channelDrawerPreview: preview.status === 'fulfilled' ? preview.value : null,
        channelDrawerPreviewError: preview.status === 'rejected' ? (preview.reason instanceof Error ? preview.reason.message : '本地分配配置读取失败') : '',
        channelDrawerAssets: assets.status === 'fulfilled' ? assets.value : [],
        channelDrawerAssetError: assets.status === 'rejected' ? (assets.reason instanceof Error ? assets.reason.message : '资产状态读取失败') : '',
      });
    })();
  }

  private closeChannelDrawer(): void {
    this.setState({ channelDrawerOpen: false, channelDrawerLoading: false, channelDrawerError: '', channelDrawerChannel: null, channelDrawerEntrants: [], channelDrawerPreview: null, channelDrawerAssets: [], channelDrawerPreviewError: '', channelDrawerAssetError: '', channelDrawerAssetBusy: false });
  }

  private async loadChannelFormAcquisitionData(channelId: number): Promise<void> {
    this.state.channelFormPreview = null;
    this.state.channelFormAssets = [];
    this.state.channelFormPreviewError = '';
    this.state.channelFormAssetError = '';
    const [preview, assets, staff] = await Promise.allSettled([
      this.api.getChannelAcquisitionPreview(channelId),
      this.loadChannelAcquisitionAssets(channelId),
      this.api.listChannelAcquisitionStaff(channelId),
    ]);
    this.state.channelFormPreview = preview.status === 'fulfilled' ? preview.value : null;
    const previewError = preview.status === 'rejected' ? (preview.reason instanceof Error ? preview.reason.message : '本地分配配置读取失败') : '';
    const staffError = staff.status === 'rejected' ? (staff.reason instanceof Error ? staff.reason.message : '企微客服目录读取失败') : '';
    this.state.channelFormPreviewError = [previewError, staffError].filter(Boolean).join('；');
    this.state.channelFormAssets = assets.status === 'fulfilled' ? assets.value : [];
    this.state.channelFormAssetError = assets.status === 'rejected' ? (assets.reason instanceof Error ? assets.reason.message : '资产状态读取失败') : '';
    this.db.staff = staff.status === 'fulfilled' ? staff.value.map((item) => ({ name: item.name, uid: item.staffId, dept: '企微可用客服' })) : [];
    if (this.state.channelFormPreview?.assignees.length && !this.state.cfStaff) {
      this.state.cfStaff = this.state.channelFormPreview.assignees.map((assignee) => ({ id: assignee.staffId, uid: assignee.staffId, name: assignee.name }));
    }
  }

  private loadChannelHistory(offset: number): void {
    const channelId = this.db.rows.channels[0]?.resourceId;
    if (this.page !== 'channelForm' || this.api.mode !== 'http' || !Number.isSafeInteger(channelId) || !channelId || channelId < 1 || !Number.isSafeInteger(offset) || offset < 0) {
      this.setState({ channelHistory: null, channelHistoryLoading: false, channelHistoryError: 'V1 历史仅支持已保存渠道的 HTTP 只读读取' });
      return;
    }
    this.setState({ channelHistory: null, channelHistoryLoading: true, channelHistoryError: '' });
    void this.api.getChannelHistory(channelId, CHANNEL_HISTORY_PAGE_SIZE, offset).then((history) => {
      if (history.channelId !== channelId) throw new Error('V1 渠道历史返回了其他渠道');
      this.setState({ channelHistory: history, channelHistoryLoading: false, channelHistoryError: '' });
    }).catch((error) => {
      this.setState({ channelHistory: null, channelHistoryLoading: false, channelHistoryError: error instanceof Error ? error.message : 'V1 历史读取失败' });
    });
  }

  private channelFormValue(id: string): string {
    return (document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null)?.value.trim() || '';
  }

  private channelProviderKey(scope: string): string {
    if (typeof globalThis.crypto?.randomUUID !== 'function') throw new Error('浏览器不支持安全幂等键，已拒绝提交企微获客链接操作');
    return `wecom-link-${scope}-${globalThis.crypto.randomUUID()}`;
  }

  private channelProviderLinkID(): string {
    return this.channelFormValue('providerLinkId');
  }

  private channelProviderInput(): CustomerAcquisitionLinkInput {
    const userIds = this.channelFormValue('providerLinkUserIds').split(',').map((item) => item.trim()).filter(Boolean);
    const departmentText = this.channelFormValue('providerLinkDepartmentIds').split(',').map((item) => item.trim()).filter(Boolean);
    if (departmentText.some((item) => !/^[1-9][0-9]*$/.test(item))) throw new Error('部门 ID 必须是逗号分隔的正整数');
    return {
      link_name: this.channelFormValue('providerLinkName'),
      user_ids: userIds,
      department_ids: departmentText.map(Number),
      skip_verify: (document.getElementById('providerLinkSkipVerify') as HTMLInputElement | null)?.checked === true,
    };
  }

  private channelProviderResult(result: WeComAcquisitionLinkWriteResult, success: string): void {
    this.setState({
      channelProviderBusy: false,
      channelProviderError: result.outcome === 'unknown' ? 'Provider 结果未知；禁止重试写操作，请使用下方回执做显式对账。' : '',
      channelProviderReceipt: result,
      channelProviderLink: result.receipt.link || this.state.channelProviderLink,
    });
    toast(result.outcome === 'applied' ? success : result.outcome === 'unknown' ? '结果未知，等待人工对账' : result.outcome === 'pending' ? '操作已受理，尚未证明 Provider 执行' : result.outcome === 'not_applied' ? '对账确认 Provider 未落地' : 'Provider 操作最终失败', result.outcome === 'failed');
  }

  private loadChannelProviderLinks(): void {
    if (this.state.channelProviderBusy) return;
    this.setState({ channelProviderBusy: true, channelProviderError: '' });
    void listWeComAcquisitionLinks('', 100).then((page) => this.setState({ channelProviderBusy: false, channelProviderLinks: page.items.map((item) => item.link_id), channelProviderError: '' })).catch((error) => this.setState({ channelProviderBusy: false, channelProviderError: error instanceof Error ? error.message : '企微获客链接列表读取失败' }));
  }

  private loadChannelProviderLink(): void {
    if (this.state.channelProviderBusy) return;
    const linkId = this.channelProviderLinkID();
    this.setState({ channelProviderBusy: true, channelProviderError: '' });
    void getWeComAcquisitionLink(linkId).then((link) => this.setState({ channelProviderBusy: false, channelProviderLink: link, channelProviderReceipt: null, channelProviderError: '' })).catch((error) => this.setState({ channelProviderBusy: false, channelProviderError: error instanceof Error ? error.message : '企微获客链接读取失败' }));
  }

  private createChannelProviderLink(): void {
    if (this.state.channelProviderBusy) return;
    let input: CustomerAcquisitionLinkInput;
    let key: string;
    try { input = this.channelProviderInput(); key = this.channelProviderKey('create'); } catch (error) { return toast(error instanceof Error ? error.message : '企微获客链接输入无效', true); }
    this.setState({ channelProviderBusy: true, channelProviderError: '' });
    void createWeComAcquisitionLink(input, key).then((result) => this.channelProviderResult(result, 'Provider 已确认创建获客链接')).catch((error) => this.setState({ channelProviderBusy: false, channelProviderError: error instanceof Error ? error.message : '企微获客链接创建失败' }));
  }

  private updateChannelProviderLink(): void {
    if (this.state.channelProviderBusy) return;
    let input: CustomerAcquisitionLinkInput;
    let key: string;
    try { input = this.channelProviderInput(); key = this.channelProviderKey('update'); } catch (error) { return toast(error instanceof Error ? error.message : '企微获客链接输入无效', true); }
    const linkId = this.channelProviderLinkID();
    this.setState({ channelProviderBusy: true, channelProviderError: '' });
    void updateWeComAcquisitionLink(linkId, input, key).then((result) => this.channelProviderResult(result, 'Provider 已确认更新获客链接')).catch((error) => this.setState({ channelProviderBusy: false, channelProviderError: error instanceof Error ? error.message : '企微获客链接更新失败' }));
  }

  private deleteChannelProviderLink(): void {
    if (this.state.channelProviderBusy) return;
    const linkId = this.channelProviderLinkID();
    confirmBox('删除企微获客链接', `该操作会在开关允许时调用 Provider 删除 ${linkId}。确认继续？`, '确认删除', true, () => {
      let key: string;
      try { key = this.channelProviderKey('delete'); } catch (error) { return toast(error instanceof Error ? error.message : '无法生成安全幂等键', true); }
      this.setState({ channelProviderBusy: true, channelProviderError: '' });
      void deleteWeComAcquisitionLink(linkId, key).then((result) => this.channelProviderResult(result, 'Provider 已确认删除获客链接')).catch((error) => this.setState({ channelProviderBusy: false, channelProviderError: error instanceof Error ? error.message : '企微获客链接删除失败' }));
    });
  }

  private reconcileChannelProviderLink(): void {
    if (this.state.channelProviderBusy) return;
    const receiptId = Number(this.channelFormValue('providerLinkReceiptId'));
    const resolution = this.channelFormValue('providerLinkResolution') === 'provider_not_applied' ? 'provider_not_applied' : 'provider_applied';
    const digest = this.channelFormValue('providerLinkEvidenceDigest');
    let key: string;
    try { key = this.channelProviderKey('reconcile'); } catch (error) { return toast(error instanceof Error ? error.message : '无法生成安全幂等键', true); }
    this.setState({ channelProviderBusy: true, channelProviderError: '' });
    void reconcileWeComAcquisitionLink(this.channelProviderLinkID(), receiptId, resolution, digest, key).then((result) => this.channelProviderResult(result, '对账确认 Provider 已落地')).catch((error) => this.setState({ channelProviderBusy: false, channelProviderError: error instanceof Error ? error.message : '企微获客链接对账失败' }));
  }

  private currentChannelFinalUrl(): string {
    if (this.channelFormValue('channelCarrier') !== 'link') return '';
    return buildChannelFinalUrl(this.channelFormValue('channelLinkUrl'), this.channelFormValue('channelCustomerChannel'));
  }

  private refreshChannelFinalUrlPreview(): void {
    const carrierType = this.channelFormValue('channelCarrier');
    const node = document.getElementById('channelFinalUrlPreview');
    const finalUrl = this.currentChannelFinalUrl();
    if (node) node.textContent = finalUrl || (carrierType === 'link' ? '填写链接 URL 后生成本地预览' : '二维码载体不生成本地链接预览');
    const input = document.getElementById('channelFinalUrl') as HTMLInputElement | null;
    if (input) input.value = finalUrl;
  }

  private copyChannelFormLink(): void {
    const value = this.channelFormValue('channelFinalUrl') || this.currentChannelFinalUrl() || this.channelFormValue('channelLinkUrl') || this.channelFormValue('channelQrUrl');
    this.copyChannelLink(value ? { name: this.channelFormValue('channelName'), finalUrl: value, linkUrl: value, copyText: value } as Channel : null);
  }

  private shareChannelFormLink(): void {
    const value = this.channelFormValue('channelFinalUrl') || this.currentChannelFinalUrl() || this.channelFormValue('channelLinkUrl') || this.channelFormValue('channelQrUrl');
    this.shareChannelLink(value ? { name: this.channelFormValue('channelName'), finalUrl: value, linkUrl: value, copyText: value } as Channel : null);
  }

  private channelName(code: string): string {
    return this.db.rows.channels.find((c) => c.code === code)?.name || '不配置引流渠道码';
  }

  /** 合并素材已选：同类型整体替换，跨类型累加，上限 9 */
  private mergeMats(cur: PickerItem[], kind: string, picked: PickerItem[]): PickerItem[] | null {
    const kept = cur.filter((m) => m.kind !== kind);
    const next = [...kept, ...picked.map((m) => ({ ...m, kind }))];
    if (next.length > 9) {
      toast('素材最多 9 个，超出部分未添加', true);
      return null;
    }
    return next;
  }

  /** 已选素材行（渠道表单欢迎语 / Agent 固定素材共用渲染数据） */
  private matRow(m: PickerItem, onRemove: () => void) {
    const iconMap: Record<string, { bg: string; color: string; text: string }> = {
      image: { bg: m.bg || 'linear-gradient(135deg,#DCE7FF,#B9CDFF)', color: 'transparent', text: '' },
      mp: { bg: m.bg || 'linear-gradient(135deg,#D8F5DE,#AEE7BD)', color: 'transparent', text: '' },
      attach: { bg: '#FFF7ED', color: '#C2410C', text: (m.chip || 'FILE').slice(0, 4) },
      group: { bg: '#EBF9EC', color: '#2EA121', text: '群' },
    };
    const ic = iconMap[m.kind || 'image'] || iconMap.image;
    return {
      ...m,
      thumb: {
        width: '36px', height: '36px', borderRadius: '6px', background: ic.bg, flex: 'none',
        display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '12px', color: ic.color, fontWeight: 700,
      } as StyleObj,
      thumbText: ic.text,
      rm: onRemove,
    };
  }

  /* ---- 渠道表单 ---- */
  private cfAddStaff(): void {
    void this.pick({ kind: 'members', selected: (this.state.cfStaff || []).map((i) => i.id) }).then((r) => {
      if (!r) return;
      if (!r.length) {
        toast('至少保留 1 位客服', true);
        return;
      }
      this.setState({ cfStaff: r });
      toast('客服分配已更新（' + r.length + ' 人，按比例均分）');
    });
  }

  private cfAddMaterial(kind: 'image' | 'mp' | 'attach' | 'group'): void {
    const label = { image: '图片', mp: '小程序', attach: '附件', group: '客户群' }[kind];
    void this.pick({ kind, selected: this.state.cfMats.filter((m) => m.kind === kind).map((m) => m.id) }).then((r) => {
      if (!r) return;
      const next = this.mergeMats(this.state.cfMats, kind, r);
      if (!next) return;
      this.setState({ cfMats: next });
      if (r.length) toast('已添加 ' + r.length + ' 个' + label);
    });
  }

  private cfPickTags(): void {
    void this.pick({ kind: 'tags', selected: (this.state.cfTags || []).map((t) => t.id) }).then((r) => {
      if (!r) return;
      this.setState({ cfTags: r });
      toast(r.length ? '入渠标签已更新（' + r.length + ' 个）' : '已清空入渠标签');
    });
  }

  private selectChannelTag(): void {
    const id = this.channelFormValue('channelTagId');
    const tag = this.db.wecomTags.find((item) => String(item.id) === id);
    const group = tag ? this.db.tagGroups.find((item) => item.id === tag.groupId) : undefined;
    const name = document.getElementById('channelTagName') as HTMLInputElement | null;
    const groupName = document.getElementById('channelTagGroup') as HTMLInputElement | null;
    if (name) name.value = tag?.name || '';
    if (groupName) groupName.value = group?.name || '';
  }

  private clearChannelTag(): void {
    const select = document.getElementById('channelTagId') as HTMLSelectElement | null;
    if (select) select.value = '';
    const name = document.getElementById('channelTagName') as HTMLInputElement | null;
    const groupName = document.getElementById('channelTagGroup') as HTMLInputElement | null;
    if (name) name.value = '';
    if (groupName) groupName.value = '';
  }

  private channelAssignmentInput(value: (id: string) => string): { input?: ChannelAcquisitionAssignmentInput; error?: string } {
    const selected = (this.state.cfStaff || []).map((item) => (item.uid || item.id).trim()).filter(Boolean);
    const owner = value('channelOwner');
    const staffIds = selected.length ? selected : owner ? [owner] : [];
    if (!staffIds.length) return {};
    if (staffIds.length > 5 || new Set(staffIds).size !== staffIds.length) return { error: '客服最多 5 位且不能重复' };
    const mode = value('channelAssignmentMode') === 'multi_staff' ? 'multi_staff' : 'single_owner';
    const strategy = value('channelAssignmentStrategy') === 'cap_switch' ? 'cap_switch' : 'ratio';
    const parseNumbers = (id: string, label: string): { values?: number[]; error?: string } => {
      const raw = value(id);
      if (!raw) return {};
      const values = raw.split(/[\s,，]+/).filter(Boolean).map(Number);
      if (values.length !== staffIds.length || values.some((item) => !Number.isSafeInteger(item))) return { error: `${label}必须与客服数量一致且为整数` };
      return { values };
    };
    const ratios = parseNumbers('channelAssignmentRatios', '比例');
    if (ratios.error) return ratios;
    const caps = parseNumbers('channelAssignmentCaps', '24 小时上限');
    if (caps.error) return caps;
    if (ratios.values?.some((item) => item < 1 || item > 100) || (ratios.values && ratios.values.reduce((sum, item) => sum + item, 0) !== 100)) return { error: '比例必须在 1-100 之间且总和为 100' };
    if (caps.values?.some((item) => item < 1)) return { error: '24 小时上限必须是正整数' };
    const defaultRatios = ratios.values || staffIds.map((_item, index) => index === 0 ? 100 - Math.floor(100 / staffIds.length) * (staffIds.length - 1) : Math.floor(100 / staffIds.length));
    return {
      input: {
        assignmentMode: mode,
        assignmentStrategy: strategy,
        overflowPolicy: value('channelOverflow'),
        assignees: staffIds.map((staffId, index) => ({ staffId, status: 'active', priority: index + 1, ...(strategy === 'ratio' ? { ratioPercent: defaultRatios[index] } : {}), ...(caps.values ? { maxScans24h: caps.values[index] } : {}) })),
      },
    };
  }

  /* ---- Agent 固定素材 ---- */
  private agAddMaterial(kind: 'image' | 'mp' | 'attach' | 'group'): void {
    const cur = this.state.agMats || this.defaultAgMats();
    void this.pick({ kind, selected: cur.filter((m) => m.kind === kind).map((m) => m.id) }).then((r) => {
      if (!r) return;
      const next = this.mergeMats(cur, kind, r);
      if (next === null) return;
      this.setState({ agMats: next });
    });
  }

  private defaultAgMats(): PickerItem[] {
    return [
      { id: '共学营预告主视觉.png', name: '共学营预告主视觉.png', sub: '图片 · 1080×1920', kind: 'image', bg: 'linear-gradient(135deg,#DCE7FF,#B9CDFF)' },
      { id: '5 天共学营 · 3 群', name: '5 天共学营 · 3 群', sub: '客户群邀请 · 剩余 118 人', kind: 'group' },
    ];
  }

  /* ---- 负责人迁移：仅调用 OpenAPI 声明的本地安全 CSV 事务 ---- */
  private saveBlob(blob: Blob, filename: string): void {
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  }

  private exportWechatOrders(): void {
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLSelectElement | null)?.value.trim() || '';
    const from = value('orderCreatedFrom');
    const to = value('orderCreatedTo');
    if (from && to && from > to) { toast('开始日期不能晚于结束日期', true); return; }
    this.setState({ saving: true });
    void this.api.exportWechatOrders({
      transactionId: value('orderTransactionId'),
      mobile: value('orderMobile'),
      productCode: value('orderProductCode'),
      status: value('orderStatus'),
      createdFrom: from ? `${from}T00:00:00Z` : undefined,
      createdTo: to ? `${to}T23:59:59Z` : undefined,
    }).then((blob) => {
      this.setState({ saving: false });
      this.saveBlob(blob, 'wechat-pay-orders.csv');
      toast('已下载当前筛选条件的微信支付交易 CSV');
    }).catch((error) => { this.setState({ saving: false }); toast(error instanceof Error ? error.message : '微信支付交易导出失败', true); });
  }

  private migDownloadTemplate(): void {
    void this.api.downloadOwnerReassignmentTemplate()
      .then((blob) => this.saveBlob(blob, '负责人迁移模板.csv'))
      .catch((error) => toast(error instanceof Error ? error.message : '模板下载失败', true));
  }

  private migParseCsv(): void {
    const input = document.getElementById('ownerMigCsv') as HTMLInputElement | null;
    const file = input?.files?.[0];
    if (!file) { toast('请先选择 CSV 或 Excel 文件', true); return; }
    this.setState({ saving: true, migFileName: file.name });
    void ownerReassignmentCsvFromFile(file)
      .then((csv) => this.api.createOwnerReassignmentPreview(csv))
      .then((preview) => {
        this.setState({ saving: false, migPreview: preview, migConfirmed: false });
        toast(`预览已生成：${preview.rows.length} 条可执行，${preview.issues.length} 条问题`);
      })
      .catch((error) => {
        this.setState({ saving: false });
        toast(error instanceof Error ? error.message : 'CSV 预览失败', true);
      });
  }

  private migDownloadReport(kind: 'errors' | 'results'): void {
    const preview = this.state.migPreview;
    if (!preview) { toast('请先生成预览', true); return; }
    void this.api.downloadOwnerReassignmentReport(preview.id, kind)
      .then((blob) => this.saveBlob(blob, `负责人迁移-${kind === 'errors' ? '错误' : '结果'}-${preview.id}.csv`))
      .catch((error) => toast(error instanceof Error ? error.message : '报告下载失败', true));
  }

  private migSetConfirmed(event: Event): void {
    this.state.migConfirmed = (event.currentTarget as HTMLInputElement).checked;
  }

  private migExecute(): void {
    const preview = this.state.migPreview;
    if (!preview) { toast('请先生成预览', true); return; }
    if (preview.executed) { toast('该预览已经执行，不能重复提交', true); return; }
    if (!preview.rows.length) { toast('预览中没有可执行记录', true); return; }
    if (!this.state.migConfirmed) { toast('请先勾选二次确认', true); return; }
    confirmBox('执行负责人迁移', `将提交 ${preview.rows.length} 条本地负责人变更。此操作不调用企微 Provider，且不能自动回滚。`, '确认执行', true, () => {
      this.setState({ saving: true });
      void this.api.executeOwnerReassignmentPreview(preview)
        .then((result) => {
          this.setState({ saving: false, migPreview: result, migConfirmed: false });
          toast(`本地迁移已提交：后端确认 ${result.result.length} 条结果；未调用企微 Provider`);
        })
        .catch((error) => {
          this.setState({ saving: false });
          toast(error instanceof Error ? error.message : '负责人迁移提交失败', true);
        });
    });
  }

  /* ---- 渠道码选择（问卷运营配置 / 商品表单 / 周期商品表单） ---- */
  private pickChannelFor(target: 'ops' | 'pf' | 'spf'): void {
    const cur = target === 'ops' ? this.state.opsChannelId : target === 'pf' ? this.state.pfChannelId : this.state.spfChannelId;
    void this.pick({ kind: 'channel', noneOption: '不配置引流渠道码', selected: cur ? [cur] : [] }).then((r) => {
      if (r === null) return;
      const id = r[0]?.id || '';
      if (target === 'ops') this.setState({ opsChannelId: id });
      else if (target === 'pf') this.setState({ pfChannelId: id });
      else this.setState({ spfChannelId: id });
    });
  }

  /* ================= 问卷 · 运营配置 ================= */

  private currentQid(): number {
    return this.db.rows.questionnaires[0]?.resourceId ?? this.pageId();
  }

  private currentOps(): QuestionnaireOps | undefined {
    return this.db.qOps[this.currentQid()];
  }

  private opsInputVal(id: string): string {
    return (document.getElementById(id) as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null)?.value || '';
  }

  private saveOps(): void {
    const ops = this.currentOps();
    if (!ops) { toast('后端未返回可编辑的问卷运营配置 DTO，未发送请求', true); return; }
    const next: QuestionnaireOps = deepCopy(ops);
    next.completionNavigationTargetId = this.opsInputVal('opsNavigationTarget');
    next.completionChannelId = this.opsInputVal('opsChannelResourceId');
    next.pushEnabled = (document.getElementById('opsPushEnabled') as HTMLInputElement | null)?.checked === true;
    next.externalPushConfigurationReference = this.opsInputVal('opsConfigurationReference');
    void this.api.saveQuestionnaireOps(this.currentQid(), next).then(() => {
      toast('本地 opaque 运营配置已保存；未触发外部推送');
      this.paramsDraft = null;
      void this.init();
    }).catch((error) => toast(error instanceof Error ? error.message : '运营配置保存失败', true));
  }

  /* ================= 企微标签 ================= */

  private openTagModal(mode: AdminState['tagMode'], tagId = 0): void {
    this.setState({ modal: 'tag', tagMode: mode, editingTagId: tagId });
  }

  private saveTagModal(): void {
    const mode = this.state.tagMode;
    const val = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLSelectElement | null)?.value.trim() || '';
    if (mode === 'create-group' || mode === 'edit-group') {
      const name = val('fTagGroupName');
      if (!name) {
        toast('请输入标签组名称', true);
        return;
      }
      const firstTag = mode === 'create-group' ? val('fTagFirst') : '';
      if (mode === 'create-group' && !firstTag) {
        toast('请输入第一个标签名称', true);
        return;
      }
      void this.api.saveTagGroup({ id: mode === 'edit-group' ? this.state.tagGroupId : undefined, name, firstTag }).then((g) => {
        toast(mode === 'edit-group' ? '标签组已重命名' : '标签组「' + name + '」已创建');
        this.setState({ modal: '', tagGroupId: g.id });
        void this.init();
      });
      return;
    }
    const name = val('fTagName');
    if (!name) {
      toast('请输入标签名称', true);
      return;
    }
    const groupId = mode === 'edit-tag'
      ? (this.db.wecomTags.find((x) => x.id === this.state.editingTagId)?.groupId ?? this.state.tagGroupId)
      : Number(val('fTagGroup') || this.state.tagGroupId);
    void this.api.saveTag({ id: mode === 'edit-tag' ? this.state.editingTagId : undefined, groupId, name }).then(() => {
      toast(mode === 'edit-tag' ? '标签已更新' : '标签「' + name + '」已创建');
      this.setState({ modal: '' });
      void this.init();
    });
  }

  /* ================= 素材库 ================= */

  private readModalInputs(ids: string[]): Record<string, string> {
    const out: Record<string, string> = {};
    for (const id of ids) out[id] = (document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | null)?.value.trim() || '';
    return out;
  }

  private saveImage(): void {
    const v = this.readModalInputs(['fImgName', 'fImgDesc', 'fImgTags']);
    if (!v.fImgName) {
      toast('请输入素材名称', true);
      return;
    }
    void this.api.saveImageItem(this.state.editingName || null, {
      name: v.fImgName, desc: v.fImgDesc, tags: v.fImgTags,
    }).then(() => {
      toast('素材已保存');
      this.setState({ modal: '', editingName: '' });
      void this.init();
    });
  }

  private saveMp(): void {
    const v = this.readModalInputs(['fMpName', 'fMpAppid', 'fMpPath', 'fMpTitle']);
    if (!v.fMpName || !v.fMpAppid) {
      toast('请填写素材名称与 AppID', true);
      return;
    }
    void this.api.saveMpItem(this.state.editingName || null, {
      name: v.fMpName, appid: v.fMpAppid, pagepath: v.fMpPath, cardTitle: v.fMpTitle,
    }).then(() => {
      toast(this.state.editingName ? '小程序素材已保存' : '小程序卡片已创建');
      this.setState({ modal: '', editingName: '' });
      void this.init();
    });
  }

  private saveAttach(): void {
    const v = this.readModalInputs(['fAttName', 'fAttTags']);
    if (!v.fAttName) {
      toast('请输入附件名称', true);
      return;
    }
    void this.api.saveAttachItem(this.state.editingName || null, {
      name: v.fAttName, tags: v.fAttTags,
    }).then(() => {
      toast('附件已保存');
      this.setState({ modal: '', editingName: '' });
      void this.init();
    });
  }

  /* ================= 配置中心 ================= */

  private currentConfigCat() {
    const key = this.qs().get('cat') || 'wecom_base';
    return this.db.configCategories.find((c) => c.key === key) || this.db.configCategories[0];
  }

  private saveConfig(): void {
    const cat = this.currentConfigCat();
    if (!cat) return;
    const values: Record<string, string> = {};
    const switches: Record<string, boolean> = {};
    for (const el of Array.from(document.querySelectorAll('[data-cfg]'))) {
      const input = el as HTMLInputElement;
      values[input.getAttribute('data-cfg') || ''] = input.value;
    }
    for (const b of cat.blocks) {
      for (const f of b.fields) if (f.kind === 'switch') switches[f.key] = f.on === true;
    }
    void this.api.saveConfigCategory(cat.key, values, switches).then(() => {
      toast('「' + cat.label + '」配置已保存');
      void this.init();
    }).catch((error) => toast(error instanceof Error ? error.message : '配置保存失败', true));
  }

  private checkConfig(): void {
    const cat = this.currentConfigCat();
    if (!cat) return;
    void this.api.checkConfigCategory(cat.key).then((msg) => toast(msg, msg.startsWith('检查发现'))).catch((error) => toast(error instanceof Error ? error.message : '配置检查失败', true));
  }

  /* ================= 普通商品 / 周期商品 ================= */

  private saveCommerceProduct(kind: 'product' | 'service'): void {
    const prefix = kind === 'product' ? 'pf' : 'spf';
    const value = (name: string): string => (document.getElementById(prefix + name) as HTMLInputElement | HTMLTextAreaElement | null)?.value.trim() || '';
    const id = Number(this.qs().get('id') || '') || undefined;
    const input: import('../api/admin').ProductWriteInput = { id, code: value('Code'), name: value('Name'), description: value('Description'), price: value('Price'), currency: value('Currency') || 'CNY', stockQuantity: Number(value('Stock')) };
    if (!input.code || !input.name) { toast('商品编码和名称不能为空', true); return; }
    if (!Number.isInteger(input.stockQuantity) || input.stockQuantity < 0) { toast('库存必须是非负整数', true); return; }
    {
      const current = kind === 'product' ? this.db.rows.products.find((row) => row.resourceId === id) : this.db.rows.spProducts.find((row) => row.resourceId === id);
      const projection = current?.adminProjection || { schemaVersion: 1 as const, status: 'draft', enabled: false, buyButtonText: '', requireMobile: false, leadProgramId: null, leadChannelId: null, leadQrTitle: '', leadQrSubtitle: '', completionRedirectEnabled: false, completionRedirectUrl: '', completionTarget: null, wecomTagging: {}, slices: [] };
      const optionalID = (name: string): number | null => { const raw = value(name); if (!raw) return null; const parsed = Number(raw); if (!Number.isSafeInteger(parsed) || parsed < 1) throw new Error(`${name} 必须是正整数`); return parsed; };
      try {
        const images = value('Images').split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
        if (images.length > 20 || images.some((url) => url.length > 2048 || (!url.startsWith('/') && !/^https:\/\//.test(url)))) throw new Error('页面素材最多 20 条，且必须是同源路径或 HTTPS 地址');
        const completionTarget = value('CompletionTarget') ? JSON.parse(value('CompletionTarget')) as Record<string, unknown> : null;
        const wecomTagging = value('WecomTagging') ? JSON.parse(value('WecomTagging')) as Record<string, unknown> : {};
        if ((completionTarget !== null && (Array.isArray(completionTarget) || typeof completionTarget !== 'object')) || Array.isArray(wecomTagging) || typeof wecomTagging !== 'object') throw new Error('跳转和企微标签配置必须是 JSON 对象');
        input.images = images;
        input.adminProjection = { ...projection, buyButtonText: value('BuyButtonText'), requireMobile: value('RequireMobile') === 'true', leadProgramId: optionalID('LeadProgramId'), leadChannelId: optionalID('LeadChannelId'), leadQrTitle: value('LeadQrTitle'), leadQrSubtitle: value('LeadQrSubtitle'), completionRedirectEnabled: value('CompletionRedirectEnabled') === 'true', completionRedirectUrl: value('CompletionRedirectUrl'), completionTarget, wecomTagging };
        const pushEnabled = value('ExternalPushEnabled') === 'true';
        const configurationReference = value('ExternalPushReference');
        if (pushEnabled && !/^[A-Za-z0-9._:-]{1,128}$/.test(configurationReference)) throw new Error('启用外推时必须填写 1-128 位配置引用');
        input.externalPush = { enabled: pushEnabled, configurationReference };
      } catch (error) { toast(error instanceof Error ? error.message : '商品运营配置无效', true); return; }
    }
    const action = kind === 'product' ? this.api.saveProduct(input) : this.api.saveServiceProduct(input);
    void action.then((saved) => {
      toast(`${kind === 'product' ? '普通' : '周期'}商品已保存，服务端版本 ${saved.version || '—'}`);
      this.goto(kind === 'product' ? 'products' : 'spProducts');
    }).catch((error) => toast(error instanceof Error ? error.message : '商品保存失败', true));
  }

  private blocked(message: string): void {
    toast('后端能力未就绪：' + message + '，未发送请求', true);
  }

  private submitRefundIntent(): void {
    const order = this.db.rows.orders[0];
    if (!order) { toast('后端未返回订单详情，未发送请求', true); return; }
    if (order.recordOrigin === 'v1_history') { toast('V1历史只读，非V2支付/退款确认', true); return; }
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLSelectElement | null)?.value.trim() || '';
    const checked = (document.getElementById('refundChecked') as HTMLInputElement | null)?.checked === true;
    const input = { provider: order.plat, orderNo: order.no, amount: value('refundAmount'), reason: value('refundReason'), transactionIdConfirmation: value('refundOrderConfirmation'), checked };
    if (!checked || input.transactionIdConfirmation !== order.no) { toast('请勾选确认并完整输入当前订单号', true); return; }
    confirmBox('创建退款 intent', '仅提交后端退款意图并展示真实 receipt 状态；这不代表退款已成功或外部渠道已确认。', '确认创建 intent', true, () => {
      this.setState({ saving: true });
      void this.api.createRefundIntent(input).then((result) => {
        this.setState({ saving: false });
        toast(`退款 intent ${result.id || '—'} 已返回，状态 ${result.state || '—'}；外部调用 ${result.realExternalCallExecuted ? '已执行' : '未执行'}，交付证据 ${result.deliveryProven ? '已确认' : '未确认'}`);
        void this.init();
      }).catch((error) => { this.setState({ saving: false }); toast(error instanceof Error ? error.message : '退款 intent 创建失败', true); });
    });
  }

  private saveHxcSender(): void {
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | null)?.value.trim() || '';
    const input = { id: value('hxcId'), senderUserid: value('hxcUserid'), displayName: value('hxcName'), priority: Number(value('hxcPriority')), active: (document.getElementById('hxcActive') as HTMLInputElement | null)?.checked === true };
    void this.api.saveHxcSender(input).then((item) => { toast(`发送人 ${item.code} 已保存为本地配置；未调用企微 Provider`); this.goto('agents'); }).catch((error) => toast(error instanceof Error ? error.message : '发送人保存失败', true));
  }

  private reorderHxcSenders(): void {
    const ids = ((document.getElementById('hxcOrder') as HTMLTextAreaElement | null)?.value || '').split(/[,，\n]/).map((id) => id.trim()).filter(Boolean);
    void this.api.reorderHxcSenders(ids).then(() => { toast('本地发送人顺序已保存；未调用企微 Provider'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '发送人排序失败', true));
  }

  private archiveHxcSender(senderUserid: string): void {
    confirmBox('归档发送人配置', `仅归档 ${senderUserid} 的本地配置，不删除企微成员。确认继续？`, '确认归档', true, () => { void this.api.archiveHxcSender(senderUserid).then(() => { toast('本地发送人配置已归档；未调用企微 Provider'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '发送人归档失败', true)); });
  }

  private refreshHxcDirectory(): void {
    if (this.api.mode !== 'http') return this.blocked('发送资格校验只支持当前 HttpApi / OpenAPI；Mock 不提供伪成功');
    confirmBox('校验 HXC 发送资格', '仅读取企微成员资格并回读既有本地 staff 目录；不会创建、更新发送人，也不会发送消息。', '确认校验', true, () => {
      void this.api.refreshHxcDirectory().then((result) => { toast(`已校验 ${result.syncedCount} 位本地发送资格交集；未发送消息`); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : 'HXC 发送资格校验失败', true));
    });
  }

  private saveCouponForm(publish: boolean): void {
    if (this.api.mode !== 'http') return this.blocked('优惠券规则写入只能由当前 HttpApi / OpenAPI 执行；测试 Mock 不提供伪成功');
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null)?.value.trim() || '';
    const validityMode = value('couponValidityMode') === 'fixed_range' ? 'fixed_range' : 'relative_days';
    const input: CouponWriteInput = {
      id: Number(this.qs().get('id') || '') || undefined,
      name: value('couponName'),
      discount: value('couponDiscount'),
      totalIssueLimit: Number(value('couponTotal')),
      perUserIssueLimit: Number(value('couponPerUser')),
      claimStartsAt: value('couponClaimStart'),
      claimEndsAt: value('couponClaimEnd'),
      validityMode,
      useStartsAt: value('couponUseStart') || undefined,
      useEndsAt: value('couponUseEnd') || undefined,
      relativeValidityDays: Number(value('couponRelativeDays')) || undefined,
      instructions: value('couponInstructions'),
      targetRefs: value('couponTargetRefs').split(/[\s,，]+/).map((item) => item.trim()).filter(Boolean),
    };
    if (!input.name || !/^\d+(\.\d{1,2})?$/.test(input.discount)) return toast('请填写名称与最多两位小数的非负减免金额', true);
    if (!Number.isInteger(input.totalIssueLimit) || input.totalIssueLimit < 1 || !Number.isInteger(input.perUserIssueLimit) || input.perUserIssueLimit < 1) return toast('发行总量和单用户限领必须为正整数', true);
    if (!input.claimStartsAt || !input.claimEndsAt || new Date(input.claimStartsAt) >= new Date(input.claimEndsAt)) return toast('领取时间范围无效', true);
    if (!input.targetRefs.length) return toast('至少填写一个服务端商品引用', true);
    if (validityMode === 'fixed_range' && (!input.useStartsAt || !input.useEndsAt || new Date(input.useStartsAt) >= new Date(input.useEndsAt))) return toast('固定使用时间范围无效', true);
    if (validityMode === 'relative_days' && (!input.relativeValidityDays || input.relativeValidityDays < 1)) return toast('相对有效天数必须为正整数', true);
    void this.api.saveCoupon(input, publish).then((saved) => {
      toast(publish ? '优惠券已保存并发布' : `优惠券草稿已保存，服务端版本 ${saved.version || '—'}`);
      this.goto('coupons');
    }).catch((error) => toast(error instanceof Error ? error.message : '优惠券保存失败', true));
  }

  private saveQuestionnaireForm(publish: boolean): void {
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null)?.value.trim() || '';
    const checked = (id: string): boolean => (document.getElementById(id) as HTMLInputElement | null)?.checked === true;
    let questions: QuestionnaireWriteInput['questions'];
    let assessmentConfig: QuestionnaireWriteInput['assessment_config'];
    try {
      questions = JSON.parse(value('questionnaireQuestions')) as QuestionnaireWriteInput['questions'];
      assessmentConfig = JSON.parse(value('questionnaireAssessmentConfig') || '{}') as QuestionnaireWriteInput['assessment_config'];
    } catch { return toast('题目或测评配置不是有效 JSON', true); }
    if (!Array.isArray(questions) || questions.length < 1 || questions.some((q) => !q || typeof q.title !== 'string' || !q.title.trim() || !['single_choice', 'multi_choice', 'textarea', 'mobile'].includes(q.type) || !Array.isArray(q.options))) return toast('题目 JSON 不符合当前 OpenAPI：至少一题，且需包含合法 type/title/options', true);
    const input: QuestionnaireWriteInput = {
      id: Number(this.qs().get('id') || '') || undefined,
      name: value('questionnaireName'), title: value('questionnaireTitle'), description: value('questionnaireDescription'),
      answer_display_mode: value('questionnaireDisplay') === 'one_by_one' ? 'one_by_one' : 'all_in_one',
      assessment_enabled: checked('questionnaireAssessmentEnabled'), assessment_config: assessmentConfig,
      slug: value('questionnaireSlug'), is_disabled: checked('questionnaireDisabled'), questions, score_rules: [],
    };
    if (!input.name || !input.title || !/^[a-z0-9][a-z0-9-]{0,119}$/.test(input.slug)) return toast('请填写问卷名称、标题和合法 slug', true);
    void this.api.saveQuestionnaire(input, publish).then((saved) => {
      toast(publish ? `公开定义已发布（服务端版本 ${saved.version || '—'}）` : `问卷已保存（服务端版本 ${saved.version || '—'}）`);
      this.goto('questionnaires');
    }).catch((error) => toast(error instanceof Error ? error.message : '问卷保存失败', true));
  }

  private saveChannelForm(): void {
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null)?.value.trim() || '';
    const checked = (id: string): boolean => (document.getElementById(id) as HTMLInputElement | null)?.checked === true;
    const ids = (id: string): number[] => value(id).split(/[\s,，]+/).filter(Boolean).map(Number);
    let assignmentConfig: Record<string, unknown>;
    try { assignmentConfig = JSON.parse(value('channelAssignmentConfig') || '{}') as Record<string, unknown>; }
    catch { return toast('客服分配配置不是有效 JSON', true); }
    const carrierType = value('channelCarrier') === 'link' ? 'link' : 'qrcode';
    const linkUrl = value('channelLinkUrl');
    const customerChannel = value('channelCustomerChannel');
    if (carrierType === 'link' && !linkUrl) return toast('链接载体必须填写链接 URL', true);
    const input: ChannelWriteInput = {
      id: Number(this.qs().get('id') || '') || undefined,
      channel_type: value('channelType') === 'wecom_customer_acquisition' ? 'wecom_customer_acquisition' : 'qrcode',
      carrier_type: carrierType,
      channel_name: value('channelName'), channel_code: value('channelCode'), scene_value: value('channelScene'), qr_url: value('channelQrUrl'),
      status: ['active', 'archived'].includes(value('channelStatus')) ? value('channelStatus') as 'active' | 'archived' : 'inactive',
      owner_staff_id: value('channelOwner'), customer_channel: customerChannel, link_url: linkUrl, final_url: carrierType === 'link' ? buildChannelFinalUrl(linkUrl, customerChannel) : value('channelFinalUrl'),
      welcome_message: value('channelWelcome'), welcome_image_library_ids: ids('channelImageIds'), welcome_miniprogram_library_ids: ids('channelMiniIds'), welcome_attachment_library_ids: ids('channelAttachmentIds'), welcome_group_invite_library_ids: ids('channelGroupInviteIds'),
      auto_accept_friend: checked('channelAutoAccept'), entry_tag_id: value('channelTagId'), entry_tag_name: value('channelTagName'), entry_tag_group_name: value('channelTagGroup'),
      assignment_mode: value('channelAssignmentMode') === 'multi_staff' ? 'multi_staff' : 'single_owner', assignment_strategy: value('channelAssignmentStrategy') === 'cap_switch' ? 'cap_switch' : 'ratio', overflow_policy: value('channelOverflow'), assignment_config_json: assignmentConfig,
    };
    if (!input.channel_name || !input.channel_code) return toast('渠道名称和编码不能为空', true);
    if ([...(input.welcome_image_library_ids || []), ...(input.welcome_miniprogram_library_ids || []), ...(input.welcome_attachment_library_ids || []), ...(input.welcome_group_invite_library_ids || [])].some((id) => !Number.isInteger(id) || id < 1)) return toast('素材引用必须是正整数 ID', true);
    const assignment = input.channel_type === 'wecom_customer_acquisition' ? this.channelAssignmentInput(value) : {};
    if (assignment.error) return toast(assignment.error, true);
    void this.api.saveChannel(input).then(async (saved) => {
      if (assignment.input) {
        const channelId = saved.resourceId || input.id;
        if (!channelId) throw new Error('渠道保存未返回服务端 ID，无法保存本地客服分配');
        await this.api.updateChannelAcquisitionAssignees(channelId, assignment.input);
      }
      toast(assignment.input ? '渠道与本地分配配置已保存；未证明企微客服同步' : '渠道配置已保存（本地事实，不代表企微执行）');
      this.goto('channels');
    }).catch((error) => toast(error instanceof Error ? error.message : '渠道或本地分配配置保存失败', true));
  }

  private openGroupOpsDirectory(): void {
    if (this.api.mode !== 'http') return toast('群目录需要真实 HTTP 与可信 owner_staff_id；测试 Mock 不提供目录刷新', true);
    const textarea = document.getElementById('groupOpsAssets') as HTMLTextAreaElement | null;
    void openGroupOpsDirectory(textarea ? { selected: textarea.value.split(/[\s,，]+/).filter(Boolean) } : {}).then((selected) => {
      if (!textarea || selected === null) return;
      textarea.value = selected.join('\n');
      toast('已更新待保存群选择；点击保存完整计划后写入，未发送群消息');
    });
  }

  private saveGroupOpsForm(): void {
    const value = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | null)?.value.trim() || '';
    let nodes: GroupOpsWriteInput['nodes'];
    try { nodes = JSON.parse(value('groupOpsNodes') || '[]') as GroupOpsWriteInput['nodes']; }
    catch { return toast('节点配置不是有效 JSON', true); }
    const staffSelect = document.getElementById('groupOpsStaff') as HTMLSelectElement | null;
    const staffIds = Array.from(staffSelect?.selectedOptions || []).map((option) => Number(option.value));
    const assetReferences = value('groupOpsAssets').split(/[\s,，]+/).filter(Boolean);
    if (!value('groupOpsName')) return toast('计划名称不能为空', true);
    if (staffIds.length > 5 || staffIds.some((id) => !Number.isSafeInteger(id) || id < 1) || new Set(staffIds).size !== staffIds.length) return toast('运营成员必须来自可信目录，且最多选择 5 位', true);
    if (!Array.isArray(nodes) || nodes.some((node) => {
      const refs = node.materialPlan?.references || [];
      return !Number.isInteger(node.position) || node.position < 1 || !['message', 'delay'].includes(node.kind) || Boolean(node.materialReference) || !refs.every((ref) => ['image', 'miniprogram', 'attachment', 'group_invite'].includes(ref.kind) && Number.isSafeInteger(ref.id) && ref.id > 0) || (node.kind === 'message' && !node.messageText && !refs.length) || (node.kind === 'delay' && (!node.delayMinutes || node.delayMinutes < 1 || refs.length));
    })) return toast('节点 JSON 不符合 GroupOpsNodeRequest；旧 material_reference 不可执行，请使用 typed materialPlan', true);
    const input: GroupOpsWriteInput = { id: this.qs().get('id') || undefined, name: value('groupOpsName'), staffIds, assetReferences, nodes, webhookReference: value('groupOpsWebhook') || undefined };
    void this.api.saveGroupOpsPlan(input).then((detail) => { toast(`群运营计划已保存，revision ${detail.plan.revision}；未触发 Provider`); this.goto('groupopsDetail', '?id=' + detail.plan.id); }).catch((error) => toast(error instanceof Error ? error.message : '群运营计划保存失败', true));
  }

  private pickGroupOpsMaterial(kind: 'image' | 'miniprogram' | 'attachment'): void {
    const textarea = document.getElementById('groupOpsNodes') as HTMLTextAreaElement | null;
    const position = Number((document.getElementById('groupOpsMaterialNodePosition') as HTMLInputElement | null)?.value);
    let nodes: GroupOpsWriteInput['nodes'];
    try { nodes = JSON.parse(textarea?.value || '[]') as GroupOpsWriteInput['nodes']; }
    catch { return toast('节点配置不是有效 JSON', true); }
    const node = nodes.find((item) => item.position === position);
    if (!textarea || !node || node.kind !== 'message') return toast('请选择一个已有的消息节点 position', true);
    const pickerKind: 'image' | 'mp' | 'attach' = kind === 'miniprogram' ? 'mp' : kind === 'attachment' ? 'attach' : 'image';
    const max = kind === 'image' ? 3 : kind === 'miniprogram' ? 1 : 9;
    const prior = (node.materialPlan?.references || []).filter((ref) => ref.kind === kind);
    const page = kind === 'image' ? 'images' : kind === 'miniprogram' ? 'mpLib' : 'attach';
    void this.api.loadDb({ page }).then((db) => this.pick({ kind: pickerKind, selected: prior.map((ref) => String(ref.id)), max, db })).then((picked) => {
      if (picked === null) return;
      const selected = picked.map((item) => Number(item.id));
      if (selected.some((id) => !Number.isSafeInteger(id) || id < 1)) return toast('素材列表未返回稳定数值 ID，未写入节点', true);
      const references = [...(node.materialPlan?.references || []).filter((ref) => ref.kind !== kind), ...selected.map((id) => ({ kind, id }))];
      if (references.length > 9) return toast('单个消息节点最多选择 9 个素材', true);
      node.materialReference = undefined;
      node.materialPlan = { references };
      textarea.value = JSON.stringify(nodes, null, 2);
      toast(`已写入 ${selected.length} 个${kind === 'image' ? '图片' : kind === 'miniprogram' ? '小程序' : '附件'}素材；保存后以服务端重读为准`);
    }).catch((error) => toast(error instanceof Error ? error.message : '素材读取失败', true));
  }

  /* ================= 模板绑定值 ================= */

  renderVals(): Vals {
    const s = this.state;
    const accent = ACCENT;
    const mk = (tone: Tone): StyleObj => this.chip(tone);

    /* ---- 导航跳转 ---- */
    const go: Record<string, () => void> = {};
    SCREENS.forEach((k) => {
      go[k] = () => this.goto(k);
    });

    /* ---- 渠道表单五步 ---- */
    const cstep = s.cstep;
    const cgo: Record<string, () => void> = {};
    const cn: Record<string, StyleObj> = {};
    const cp: Record<string, StyleObj> = {};
    [1, 2, 3, 4, 5].forEach((i) => {
      const on = cstep === i;
      cgo[i] = () => this.setState({ cstep: i });
      cn[i] = {
        display: 'flex', alignItems: 'center', gap: '10px', height: '44px', padding: '0 12px',
        borderRadius: '8px', cursor: 'pointer', fontSize: '14px',
        background: on ? '#EFF4FF' : 'transparent',
        color: on ? accent : '#1F2329',
        fontWeight: on ? 600 : 400,
      };
      cn['dot' + i] = {
        width: '22px', height: '22px', borderRadius: '50%', flex: 'none',
        display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '12px',
        background: on ? accent : '#F2F3F5', color: on ? '#fff' : '#8F959E', fontWeight: 500,
      };
      cp[i] = { display: on ? 'block' : 'none' };
    });

    /* ---- Agent 编辑四步 ---- */
    const astep = s.astep || 1;
    const ago: Record<string, () => void> = {};
    const an: Record<string, StyleObj> = {};
    const ap: Record<string, StyleObj> = {};
    [1, 2, 3, 4].forEach((i) => {
      const on = astep === i;
      ago[i] = () => this.setState({ astep: i });
      an[i] = {
        display: 'flex', alignItems: 'center', gap: '10px', height: '44px', padding: '0 12px',
        borderRadius: '8px', cursor: 'pointer', fontSize: '14px',
        background: on ? '#EFF4FF' : 'transparent', color: on ? accent : '#1F2329',
        fontWeight: on ? 600 : 400,
      };
      an['dot' + i] = {
        width: '22px', height: '22px', borderRadius: '50%', flex: 'none', display: 'flex',
        alignItems: 'center', justifyContent: 'center', fontSize: '12px',
        background: on ? accent : '#F2F3F5', color: on ? '#fff' : '#8F959E', fontWeight: 500,
      };
      ap[i] = { display: on ? 'block' : 'none' };
    });

    const rows = this.db.rows;
    const customerContext = this.db.customerDetail.context;
    const customerSurvey = this.db.customerDetail.survey;
    const customerItem = customerContext?.profile || rows.customers[0] || { name: '', id: '', owner: '', stageId: null };
    const customerId = Number(customerItem?.id || 0);
    const customerAction = async (action: () => Promise<unknown>, success: string): Promise<void> => {
      try { await action(); toast(success); await this.init(); }
      catch (error) { toast(error instanceof Error ? error.message : '客户操作失败', true); }
    };

    /* ================= 自动化运营 · 人群包 ================= */
    const audGroups = this.audienceGroupsVals(accent);
    const curGroupName = s.groupId === 0 ? '未分组' : this.db.audienceGroups.find((g) => g.id === s.groupId)?.name || '未分组';
    const audPkgs = this.db.audiencePackages.filter((p) => p.groupId === s.groupId);
    const audienceRows = audPkgs.map((p) => ({
      ...p,
      countText: p.count.toLocaleString(),
      toggleText: p.running ? '停止' : '启用',
      edit: () => this.goto('audienceEdit', '?id=' + p.id),
      copyPkg: () => {
        void this.api.copyAudiencePackage(p.id).then(() => {
          toast('已复制人群包「' + p.name + '」');
          void this.init();
        });
      },
      toggle: () => {
        if (p.running) {
          confirmBox('停止人群包', '停止后「' + p.name + '」将不再自动刷新与触达，确认停止？', '确认停止', true, () => {
            void this.api.toggleAudiencePackage(p.id, false).then(() => {
              toast('已停止');
              void this.init();
            });
          });
        } else {
          void this.api.toggleAudiencePackage(p.id, true).then(() => {
            toast('已启用');
            void this.init();
          });
        }
      },
      del: () => {
        confirmBox('删除人群包', '确认删除「' + p.name + '」？删除后不可恢复。', '确认删除', true, () => {
          void this.api.deleteAudiencePackage(p.id).then(() => {
            toast('已删除');
            void this.init();
          });
        });
      },
      broadcast: () => this.blocked('AI 人群包 API 不等于群发任务创建契约'),
      verStyle: { display: 'inline-flex', alignItems: 'center', height: '20px', padding: '0 8px', border: '1px solid #DEE0E3', borderRadius: '999px', background: '#F8FAFC', color: '#667085', fontSize: '11px', whiteSpace: 'nowrap' } as StyleObj,
      refreshStyle: { display: 'inline-flex', alignItems: 'center', height: '22px', padding: '0 9px', border: '1px solid #DBEAFE', borderRadius: '999px', background: '#EFF6FF', color: '#1D4ED8', fontSize: '12px', whiteSpace: 'nowrap' } as StyleObj,
    }));

    /* ================= 人群包编辑器 ================= */
    const pkg = this.audiencePkg();
    const aePkg = pkg
      ? {
          ...pkg,
          countText: pkg.count.toLocaleString(),
          groupName: pkg.groupId === 0 ? '未分组' : this.db.audienceGroups.find((g) => g.id === pkg.groupId)?.name || '未分组',
          statusText: pkg.running ? '运行中' : '已停止',
          incrementalText: pkg.incremental === 'incremental_3m' ? '每 3 分钟' : '关闭',
          dailyText: pkg.daily === 'daily_0200' ? '每日 2:00' : '关闭',
          boundText: pkg.boundAutomation || '未绑定',
          definitionText: pkg.definition || '{}',
          refreshCronText: pkg.refreshCron || '',
          bindingAgentIdText: pkg.bindingAgentId ? String(pkg.bindingAgentId) : '',
          configurationText: pkg.configurationVersion ? `v${pkg.configurationVersion}（已加载）` : '尚未保存配置版本',
        }
      : null;
    const aeNav: Record<string, StyleObj> = {};
    const aeGo: Record<string, () => void> = {};
    const aePanel: Record<string, StyleObj> = {};
    ['基础配置', '自动化话术能力', '发送人白名单', '成员列表', '发送记录'].forEach((label, idx) => {
      const i = idx + 1;
      const on = s.apanel === i;
      aeGo[i] = () => this.setState({ apanel: i });
      aeNav[i] = {
        display: 'flex', alignItems: 'center', gap: '10px', height: '44px', padding: '0 12px',
        borderRadius: '8px', cursor: 'pointer', fontSize: '14px', border: '0', background: on ? '#EFF4FF' : 'transparent',
        color: on ? accent : '#1F2329', fontWeight: on ? 600 : 400, width: '100%', textAlign: 'left',
      };
      aeNav['dot' + i] = {
        width: '22px', height: '22px', borderRadius: '50%', flex: 'none', display: 'flex',
        alignItems: 'center', justifyContent: 'center', fontSize: '12px',
        background: on ? accent : '#F2F3F5', color: on ? '#fff' : '#8F959E', fontWeight: 500,
      };
      aePanel[i] = { display: on ? 'block' : 'none' };
      void label;
    });
    const aeGroupOpts = pkg
      ? [{ id: 0, name: '未分组' }, ...this.db.audienceGroups].map((g) => ({ v: String(g.id), t: g.name, sel: g.id === pkg.groupId, not: g.id !== pkg.groupId }))
      : [];
    const aeIncOpts = pkg ? this.aeSelectOpts(pkg.incremental, [['off', '关闭'], ['incremental_3m', '每 3 分钟']]) : [];
    const aeDailyOpts = pkg ? this.aeSelectOpts(pkg.daily, [['off', '关闭'], ['daily_0200', '每日 2:00']]) : [];
    const aeSenders = pkg ? [...(this.db.audienceSenders[pkg.id] || []), ...(this.sendersDraft || [])] : [];
    const aeMembers = pkg ? this.db.audienceMembers[pkg.id] || [] : [];
    const aeRecords = pkg ? this.db.audienceRecords[pkg.id] || [] : [];
    const aePreview = this.state.audiencePreview
      ? {
          ...this.state.audiencePreview,
          state: this.state.audiencePreview.memberCount === 0 ? (this.state.audiencePreview.emptyConfirmed ? 'empty_confirmed' : 'empty_pending') : 'ready',
          message: this.state.audiencePreview.memberCount === 0
            ? (this.state.audiencePreview.emptyConfirmed ? '空人群已明确确认；仍需单独确认物化。' : '空人群尚未确认，物化已拒绝。')
            : '预览已完成；物化仍只写本地成员事实。',
      }
      : null;
    const aeTemplateOpts = this.state.audienceTemplates.map((template) => ({
      v: template.key,
      t: `${template.key} · v${template.version}`,
      sel: template.key === this.state.audienceTemplateKey,
      not: template.key !== this.state.audienceTemplateKey,
    }));
    const aeTemplatePreview = this.state.audienceTemplatePreview
      ? { ...this.state.audienceTemplatePreview, savedText: this.state.audienceTemplatePreview.saved ? '已保存' : '仅预览' }
      : null;
    const aeRecordRows = aeRecords.map((r) => ({ ...r, cs: mk(r.tone) }));
    const aeAgents = rows.agents.map((a) => ({
      ...a,
      isBound: pkg ? pkg.boundAutomation === a.name : false,
      bind: () => this.bindAutomation(a.name),
      card: {
        display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px',
        padding: '12px 14px', borderRadius: '8px', marginBottom: '8px',
        border: pkg && pkg.boundAutomation === a.name ? '1px solid #528BFF' : '1px solid #DEE0E3',
        background: pkg && pkg.boundAutomation === a.name ? '#F5F8FF' : '#fff',
      } as StyleObj,
    }));

    /* ================= 运营闭环 ================= */
    const cycleRows = this.db.cycleTasks.map((t) => ({
      ...t,
      steps: t.steps.map((st) => ({ ...st, tc: st.dim ? '#A6AAB0' : '#1F2329' })),
      viewDetail: () => this.goto('cyclesDetail', '?id=' + t.runId),
      act: () => this.blocked('当前复盘会话壳与 execution-runtime DTO 不等价'),
    }));
    const run = this.db.cycleRuns[this.pageId()] || this.db.cycleRuns[1];
    const groupOpsRows = this.db.groupOpsPlans.map((plan) => ({
      ...plan, cs: mk(plan.status === 'active' ? 'ok' : plan.status === 'draft' ? 'warn' : 'gray'),
      edit: () => this.goto('groupopsDetail', '?id=' + plan.id),
      toggleText: plan.status === 'active' ? '暂停' : '启用',
      toggle: () => { const action = plan.status === 'active' ? 'pause' : 'activate'; const runAction = () => void this.api.transitionGroupOpsPlan(plan.id, action).then(() => { toast(action === 'activate' ? '计划已启用' : '计划已暂停'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '计划状态变更失败', true)); if (action === 'pause') confirmBox('暂停计划', '暂停后不再接受新的 run-due。确认暂停？', '确认暂停', true, runAction); else runAction(); },
      archive: () => confirmBox('归档计划', '归档保留执行证据且不可直接恢复。确认归档？', '确认归档', true, () => { void this.api.transitionGroupOpsPlan(plan.id, 'archive').then(() => { toast('计划已归档'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '计划归档失败', true)); }),
      del: () => { if (plan.status !== 'draft') return toast('仅草稿计划可删除；其他状态请归档', true); confirmBox('删除草稿', '确认删除该草稿计划？', '确认删除', true, () => { void this.api.deleteGroupOpsPlan(plan.id).then(() => { toast('草稿计划已删除'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '计划删除失败', true)); }); },
    }));
    const groupOpsDetail = this.db.groupOpsDetail;
    const selectedGroupOpsStaff = new Set(groupOpsDetail?.staffIds || []);
    const visibleGroupOpsStaff = new Set(this.db.staff.map((member) => Number(member.uid)));
    const groupOpsMemberOptions = [
      ...this.db.staff.map((member) => ({ ...member, selected: selectedGroupOpsStaff.has(Number(member.uid)), unselected: !selectedGroupOpsStaff.has(Number(member.uid)) })),
      ...[...selectedGroupOpsStaff].filter((staffID) => !visibleGroupOpsStaff.has(staffID)).map((staffID) => ({ uid: String(staffID), name: `已绑定 staff ${staffID}`, dept: '目录当前不可见', selected: true, unselected: false })),
    ];
    const hxcEditId = this.qs().get('id') || '';
    const hxcEdit = rows.agents.find((item) => item.code === hxcEditId || item.senderId === hxcEditId);
    const runVals = run
      ? {
          ...run,
          reviewCs: mk(run.reviewTone),
          next: { ...run.next, cs: mk(run.next.tone) },
          windows: run.windows.map((w) => ({ ...w, cs: mk(w.tone), hasMetrics: w.metrics.length > 0 })),
          attempts: run.attempts.map((a) => ({ ...a, cs: mk(a.tone), stages: a.stages.map((sg) => ({ ...sg, dot: sg.status === 'ok' ? '#2EA121' : '#D97917' })) })),
          funnel: run.funnel.map((f, i) => ({ ...f, w: Math.max(18, 100 - i * 18) + '%' })),
        }
      : null;

    /* ================= 问卷 · 运营配置 ================= */
    const qid = this.currentQid();
    const qRow = rows.questionnaires.find((item) => item.resourceId === qid);
    const ops = this.currentOps();
    const opsNav: Record<string, StyleObj> = {};
    const opsGo: Record<string, () => void> = {};
    const opsPanel: Record<string, StyleObj> = {};
    ['提交后动作', '外部推送'].forEach((label, idx) => {
      const i = idx + 1;
      const on = s.opsTab === i;
      opsGo[i] = () => this.setState({ opsTab: i });
      opsNav[i] = {
        display: 'flex', alignItems: 'center', gap: '10px', height: '44px', padding: '0 12px',
        borderRadius: '8px', cursor: 'pointer', fontSize: '14px', border: '0', background: on ? '#EFF4FF' : 'transparent',
        color: on ? accent : '#1F2329', fontWeight: on ? 600 : 400, width: '100%', textAlign: 'left',
      };
      opsNav['dot' + i] = {
        width: '22px', height: '22px', borderRadius: '50%', flex: 'none', display: 'flex',
        alignItems: 'center', justifyContent: 'center', fontSize: '12px',
        background: on ? accent : '#F2F3F5', color: on ? '#fff' : '#8F959E', fontWeight: 500,
      };
      opsPanel[i] = { display: on ? 'block' : 'none' };
      void label;
    });
    const opsParams = [...(ops?.customParams || []), ...(this.paramsDraft || [])];
    const opsCard = (on: boolean): StyleObj => ({
      border: on ? '1px solid #528BFF' : '1px solid #DEE0E3',
      background: on ? '#F5F8FF' : '#fff',
      borderRadius: '8px', padding: '14px', cursor: 'pointer',
    });
    const redirectTypeOpts = ops ? this.aeSelectOpts(ops.redirectType, [['h5', 'H5 跳转地址'], ['urllink', '动态 URL Link 接口']]) : [];
    const freqOpts = ops ? this.aeSelectOpts(ops.frequency, [['实时推送', '实时推送'], ['每 10 分钟汇总', '每 10 分钟汇总'], ['每小时汇总', '每小时汇总']]) : [];
    const channelOpts = rows.channels.map((c, i) => ({ v: c.name, t: c.name, sel: i === 0, not: i !== 0 }));
    const opsLogSource = s.opsLogScope === 'global' ? this.globalQuestionnairePushLogs || [] : rows.qApply;
    const opsLogRows = opsLogSource.filter((row) => { const keyword = s.opsLogKeyword.trim().toLowerCase(); return (!keyword || row.sid.toLowerCase().includes(keyword) || row.uid.toLowerCase().includes(keyword)) && (!s.opsLogStatus || row.status === s.opsLogStatus); });

    /* ================= 企微标签 ================= */
    const tagQ = s.tagQ.trim();
    const tagGroups = this.db.tagGroups.map((g) => {
      const on = s.tagGroupId === g.id;
      return {
        ...g,
        count: this.db.wecomTags.filter((x) => x.groupId === g.id).length,
        pick: () => this.setState({ tagGroupId: g.id }),
        row: {
          display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '8px', height: '34px',
          padding: '0 10px', borderRadius: '6px', cursor: 'pointer', fontSize: '13px',
          background: on ? '#EFF4FF' : 'transparent', color: on ? accent : '#1F2329', fontWeight: on ? 600 : 400,
        } as StyleObj,
        cnt: { fontSize: '12px', color: on ? accent : '#A6AAB0' } as StyleObj,
      };
    });
    const curTagGroup = this.db.tagGroups.find((g) => g.id === s.tagGroupId) || this.db.tagGroups[0];
    const tagRows = this.db.wecomTags
      .filter((x) => x.groupId === (curTagGroup?.id ?? 1))
      .filter((x) => !tagQ || x.name.includes(tagQ))
      .map((x) => ({
        ...x,
        edit: () => this.openTagModal('edit-tag', x.id),
        del: () =>
          confirmBox('删除标签', '确认删除标签「' + x.name + '」？已打标客户不受影响。', '确认删除', true, () => {
            void this.api.deleteTag(x.id).then(() => {
              toast('标签已删除');
              void this.init();
            });
          }),
      }));
    const tagCapacity = this.db.wecomTags.length;
    const tm = s.tagMode;
    const editingTag = this.db.wecomTags.find((x) => x.id === s.editingTagId);
    const tagModal = {
      title: tm === 'create-group' ? '新建标签组' : tm === 'create-tag' ? '新建标签' : tm === 'edit-group' ? '编辑组名' : '编辑标签',
      isCreateGroup: tm === 'create-group',
      isCreateTag: tm === 'create-tag',
      isEditGroup: tm === 'edit-group',
      isEditTag: tm === 'edit-tag',
      isGroupForm: tm === 'create-group' || tm === 'edit-group',
      isTagForm: tm === 'create-tag' || tm === 'edit-tag',
      groupName: tm === 'edit-group' ? curTagGroup?.name || '' : '',
      tagName: tm === 'edit-tag' ? editingTag?.name || '' : '',
      groupOpts: this.db.tagGroups.map((g) => ({ v: String(g.id), t: g.name, sel: g.id === s.tagGroupId, not: g.id !== s.tagGroupId })),
      okLabel: tm === 'create-group' || tm === 'create-tag' ? '创建' : '保存',
    };

    /* ================= 分享组件 ================= */
    const share = {
      open: s.modal === 'share',
      kind: s.shareKind,
      title: s.shareTitle,
      url: s.shareUrl,
      copyLink: () => this.copyShareLink(),
      preview: () => this.previewShareLink(),
      saveQr: () => downloadQr(s.shareUrl, `${s.shareCode || 'share'}-qr.svg`),
      close: () => this.closeModal(),
    };

    /* ================= 素材库 ================= */
    const imageCards = rows.images.map((m) => ({
      ...m,
      cs: mk(m.tone),
      thumb: { height: '104px', background: m.thumbnailUrl ? `url(${m.thumbnailUrl}) center / cover no-repeat` : m.bg, borderBottom: '1px solid #EFF0F1', cursor: 'pointer' } as StyleObj,
      off: m.enabled ? {} : { opacity: '0.55' } as StyleObj,
      open: () => this.setState({ modal: 'imgEdit', editingName: m.name }),
    }));
    const editingImg = imageCards.find((x) => x.name === s.editingName);
    const mpCards = rows.mpItems.map((m) => ({
      ...m,
      statusColor: m.thumbOk ? '#2EA121' : '#D97917',
      off: m.enabled ? {} : { opacity: '0.55' } as StyleObj,
      enabledLabel: m.enabled ? '已启用' : '已停用',
      edit: () => this.setState({ modal: 'mpEdit', editingName: m.name }),
      del: () =>
        confirmBox('删除小程序素材', '确认删除「' + m.name + '」？删除后不可恢复。', '确认删除', true, () => {
          void this.api.deleteMpItem(m).then(() => {
            toast('已删除');
            void this.init();
          });
        }),
    }));
    const editingMp = mpCards.find((x) => x.name === s.editingName);
    const miniProgramMeta = this.miniProgramPage();
    const miniProgramTotal = miniProgramMeta?.total ?? (this.api.mode === 'mock' ? rows.mpItems.length : 0);
    const miniProgramOffset = miniProgramMeta?.offset ?? s.miniProgramOffset;
    const miniProgramRangeStart = rows.mpItems.length === 0 ? 0 : miniProgramOffset + 1;
    const miniProgramRangeEnd = rows.mpItems.length === 0 ? 0 : miniProgramOffset + rows.mpItems.length;
    const attachRows = rows.attachItems.map((a) => ({
      ...a,
      badge: a.type === 'PDF' ? { background: '#FFF7ED', color: '#C2410C' } : a.type === 'XLSX' ? { background: '#ECFDF3', color: '#067647' } : { background: '#EFF4FF', color: '#245BDB' },
      rowStyle: a.enabled ? {} : { background: '#FAFAFB' },
      edit: () => this.setState({ modal: 'attEdit', editingName: a.name }),
      download: () => {
        void this.api.downloadAttachItem(a).then((blob) => {
          const url = URL.createObjectURL(blob);
          const anchor = document.createElement('a');
          anchor.href = url;
          anchor.download = a.name;
          anchor.click();
          URL.revokeObjectURL(url);
        }).catch((error) => toast(error instanceof Error ? error.message : '附件下载失败', true));
      },
      del: () =>
        confirmBox('删除附件', '确认删除「' + a.name + '」？删除后不可恢复。', '确认删除', true, () => {
          void this.api.deleteAttachItem(a).then(() => {
            toast('已删除');
            void this.init();
          });
        }),
    }));
    const editingAtt = attachRows.find((x) => x.name === s.editingName);

    /* ================= 商品 / 周期商品 / 优惠券 ================= */
    const productRows = rows.products.map((p) => ({
      ...p,
      cs: mk(p.tone),
      edit: () => p.resourceId ? this.goto('productForm', '?id=' + p.resourceId) : toast('商品缺少服务端 ID', true),
      shareIt: () => this.blocked('商品分享投影返回 no_authoritative_public_purchase_route'),
      copyIt: () => p.resourceId && void this.api.copyProduct(p.resourceId).then(() => { toast('商品副本已创建为草稿'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '商品复制失败', true)),
      del: () => p.resourceId && confirmBox('删除商品', '仅未被引用的本地草稿可删除。确认继续？', '确认删除', true, () => { void this.api.deleteProduct(p.resourceId!).then(() => { toast('商品已删除'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '商品删除失败', true)); }),
      toggle: () => {
        if (!p.resourceId) return toast('商品缺少服务端 ID', true);
        const enabled = p.lifecycle !== 'enabled';
        void this.api.setProductEnabled(p.resourceId, enabled).then(() => { toast(enabled ? '商品已启用' : '商品已停用'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '商品状态变更失败', true));
      },
      toggleText: p.lifecycle === 'enabled' ? '停用' : '启用',
    }));
    const spRows = rows.spProducts.map((p) => ({
      ...p,
      cs: mk(p.tone),
      edit: () => p.resourceId ? this.goto('spProductForm', '?id=' + p.resourceId) : toast('周期商品缺少服务端 ID', true),
      data: () => p.resourceId ? this.goto('spProductData', '?id=' + p.resourceId) : toast('周期商品缺少服务端 ID', true),
      shareIt: () => {
        if (!p.resourceId) return toast('周期商品缺少服务端 ID', true);
        void this.api.getServiceProductSharePath(p.resourceId).then((path) => this.openShare('周期商品', p.name, p.code, path)).catch((error) => toast(error instanceof Error ? error.message : '分享地址读取失败', true));
      },
      copyIt: () => p.resourceId && void this.api.copyServiceProduct(p.resourceId).then(() => { toast('周期商品副本已创建为草稿'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '周期商品复制失败', true)),
      archive: () => p.resourceId && confirmBox('归档周期商品', '归档会保留成员历史，确认继续？', '确认归档', true, () => { void this.api.archiveServiceProduct(p.resourceId!).then(() => { toast('周期商品已归档'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '周期商品归档失败', true)); }),
      toggle: () => {
        if (!p.resourceId) return toast('周期商品缺少服务端 ID', true);
        const enabled = p.lifecycle !== 'enabled';
        void this.api.setServiceProductEnabled(p.resourceId, enabled).then(() => { toast(enabled ? '周期商品已启用' : '周期商品已停用'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '周期商品状态变更失败', true));
      },
      toggleText: p.lifecycle === 'enabled' ? '停用' : '启用',
    }));
    const couponRows = rows.coupons.filter((r) => (!s.couponQuery || r.name.toLowerCase().includes(s.couponQuery.toLowerCase())) && (!s.couponStatus || (r.availabilityStatus || r.status) === s.couponStatus)).map((r, idx) => ({
      ...r,
      displayStatus: r.availabilityStatus || r.status,
      cs: mk(r.tone),
      edit: () => r.resourceId ? this.goto('couponForm', '?id=' + r.resourceId) : toast('优惠券缺少服务端资源 ID', true),
      data: () => r.resourceId ? this.goto('couponData', '?id=' + r.resourceId) : this.goto('couponData', '?id=' + idx),
      copyIt: () => {
        if (this.api.mode !== 'http') return this.blocked('复制优惠券只能由当前 HttpApi / OpenAPI 执行；测试 Mock 不提供伪成功');
        if (!r.resourceId) return toast('优惠券缺少服务端资源 ID', true);
        void this.api.copyCoupon(r.resourceId).then((copied) => copied.resourceId ? this.goto('couponForm', '?id=' + copied.resourceId) : toast('优惠券复制响应缺少资源 ID', true)).catch((error) => toast(error instanceof Error ? error.message : '优惠券复制失败', true));
      },
      toggle: () => {
        if (this.api.mode !== 'http') return this.blocked('优惠券生命周期只能由当前 HttpApi / OpenAPI 执行；测试 Mock 不提供伪成功');
        if (!r.resourceId) return toast('优惠券缺少服务端资源 ID', true);
        const published = r.status !== 'published' && r.status !== 'active';
        const run = () => void this.api.setCouponPublished(r.resourceId!, published).then(() => { toast(published ? '优惠券已发布' : '优惠券已停用'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '优惠券状态变更失败', true));
        if (published) confirmBox('发布优惠券', '发布后按后端规则开放领取。确认发布？', '确认发布', true, run); else confirmBox('停用优惠券', '停用后不能继续领取，已领取券仍按后端规则处理。确认停用？', '确认停用', true, run);
      },
      toggleText: r.status === 'published' || r.status === 'active' ? '停用' : '发布',
      archive: () => {
        if (this.api.mode !== 'http') return this.blocked('优惠券归档只能由当前 HttpApi / OpenAPI 执行；测试 Mock 不提供伪成功');
        if (r.resourceId) confirmBox('归档优惠券', '归档后保留领取和核销记录。确认归档？', '确认归档', true, () => { void this.api.archiveCoupon(r.resourceId!).then(() => { toast('优惠券已归档'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '优惠券归档失败', true)); });
      },
      del: () => {
        if (this.api.mode !== 'http') return this.blocked('优惠券删除只能由当前 HttpApi / OpenAPI 执行；测试 Mock 不提供伪成功');
        if (r.resourceId) confirmBox('删除优惠券草稿', '仅未发布且无领取记录的草稿可删除。确认删除？', '确认删除', true, () => { void this.api.deleteCoupon(r.resourceId!).then(() => { toast('优惠券草稿已删除'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '优惠券删除失败', true)); });
      },
      shareIt: () => {
        if (this.api.mode !== 'http') return this.blocked('优惠券分享地址只能由当前 HttpApi / OpenAPI 读取；测试 Mock 不提供伪成功');
        if (!r.resourceId) return toast('优惠券缺少服务端资源 ID，无法读取分享地址', true);
        void this.api.getCouponSharePath(r.resourceId).then((path) => this.openShare('优惠券', r.name, r.code, path)).catch((error) => toast(error instanceof Error ? error.message : '分享地址读取失败', true));
      },
    }));
    const couponIdx = this.api.mode === 'http' ? 0 : this.pageId();
    const coupon = rows.coupons[couponIdx] || rows.coupons[0];
    const claims = this.db.couponClaims[couponIdx] || [];
    const claimRows = claims.map((c) => ({ ...c, cs: mk(c.tone) }));
    const cntOf = (st: string): number => claims.filter((c) => c.status === st).length;
    const couponStats = [
      { label: '累计领取', value: String(claims.length), sub: '发行 ' + (coupon?.issue.split('/')[0].trim() || '-') },
      { label: '当前可用', value: String(cntOf('可用') || 0), sub: '未使用且在有效期内' },
      { label: '支付预占', value: String(cntOf('已预占') || 0), sub: '下单未支付锁定' },
      { label: '已使用', value: String(cntOf('已使用') || 0), sub: '已核销抵扣' },
      { label: '已过期', value: String(cntOf('已过期') || 0), sub: '超过有效期未使用' },
    ];

    const productFormValue = this.qs().get('id') ? rows.products[0] : undefined;
    const serviceFormValue = this.qs().get('id') ? rows.spProducts[0] : undefined;
    const couponFormValue = this.qs().get('id') ? rows.coupons[0] : undefined;
    const questionnaireFormValue = this.page === 'questionnaireDetail' && this.qs().get('id') ? rows.questionnaires[0] : undefined;
    const channelFormValue = this.page === 'channelForm' && Boolean(this.qs().get('id')) ? rows.channels[0] : undefined;
    const channelFinalUrlPreview = channelFormValue?.carrierType === 'link' ? buildChannelFinalUrl(channelFormValue.linkUrl || '', channelFormValue.customerChannel || '') : '';
    const dateInput = (value?: string | null): string => value ? value.slice(0, 16) : '';
    const channelAssetKind = (channel: Channel | null | undefined): ChannelAcquisitionAsset['kind'] => channel?.carrierType === 'link' ? 'customer_acquisition_link' : 'contact_way_qrcode';
    const channelAssetView = (asset: ChannelAcquisitionAsset | undefined) => {
      const ready = channelAcquisitionAssetReady(asset);
      return {
        has: Boolean(asset),
        noAsset: !asset,
        kind: asset ? this.channelAssetKindLabel(asset.kind) : '',
        status: this.channelAssetStatusLabel(asset),
        version: asset?.assetVersion || '—',
        effectId: asset?.effectId || '—',
        ready,
        url: ready ? (asset!.kind === 'contact_way_qrcode' ? asset!.downloadUrl : asset!.assetUrl) : '',
        open: () => this.channelAssetOpen(asset),
        download: () => this.channelAssetDownload(asset),
        copy: () => this.channelAssetCopy(asset),
      };
    };
    const latestDrawerAsset = channelAssetView(s.channelDrawerAssets[0]);
    const latestFormAsset = channelAssetView(s.channelFormAssets[0]);
    const channelProviderLink = s.channelProviderLink;
    const channelProviderReceipt = s.channelProviderReceipt;
    const channelHistory = s.channelHistory;
    const channelHistoryAvailable = this.api.mode === 'http' && Number.isSafeInteger(channelFormValue?.resourceId) && (channelFormValue?.resourceId || 0) > 0;
    const channelHistoryRange = !channelHistory || channelHistory.total === 0
      ? '暂无归档联系人'
      : `第 ${channelHistory.offset + 1} – ${channelHistory.offset + channelHistory.contacts.length} 条，共 ${channelHistory.total} 条`;
    const channelHistoryContacts = (channelHistory?.contacts || []).map((item) => ({
      sourceContactId: String(item.sourceContactId),
      customerId: item.customerId == null ? '未核验' : String(item.customerId),
      ownerReference: item.ownerReference || '—',
      firstEnteredAt: item.firstEnteredAt,
      lastEnteredAt: item.lastEnteredAt,
      enterCount: String(item.enterCount),
    }));
    const channelHistoryAssignees = (channelHistory?.assignees || []).map((item) => ({
      sourceAssigneeId: String(item.sourceAssigneeId),
      displayNameSnapshot: item.displayNameSnapshot || '—',
      staffReference: item.staffReference || '—',
      status: item.status || '—',
      priority: String(item.priority),
      ratio: item.ratioPercent == null ? '—' : `${item.ratioPercent}%`,
      maxScans24h: item.maxScans24h == null ? '—' : String(item.maxScans24h),
      sourceCreatedAt: item.sourceCreatedAt,
      sourceUpdatedAt: item.sourceUpdatedAt,
    }));
    const channelTagOptions = this.db.wecomTags.map((tag) => {
      const selected = String(tag.id) === (channelFormValue?.entryTagId || '');
      return { id: String(tag.id), name: tag.name, groupName: this.db.tagGroups.find((group) => group.id === tag.groupId)?.name || `标签组 ${tag.groupId}`, selected, notSelected: !selected };
    });
    const formStaffRows = s.cfStaff
      ? s.cfStaff.map((member) => ({ name: member.name, uid: member.uid || member.id }))
      : (s.channelFormPreview?.assignees || []).map((member) => ({ name: member.name, uid: member.staffId }));

    /* ================= 配置中心 ================= */
    const configRows = this.db.configCategories.map((c) => ({
      ...c,
      status: c.on ? '已生效' : '未生效',
      cs: mk(c.on ? 'ok' : 'gray'),
      sw: this.sw(c.on, accent),
      toggle: () => {
        void this.api.toggleConfigCategory(c.key, !c.on).then(() => {
          toast('「' + c.label + '」已' + (!c.on ? '启用' : '停用'));
          void this.init();
        }).catch((error) => toast(error instanceof Error ? error.message : '配置启停失败', true));
      },
      open: () => this.goto('configDetail', '?cat=' + c.key),
    }));
    const cfgCat = this.currentConfigCat();
    const cfgVals = cfgCat
      ? {
          ...cfgCat,
          status: cfgCat.on ? '已生效' : '未生效',
          cs: mk(cfgCat.on ? 'ok' : 'gray'),
          sw: this.sw(cfgCat.on, accent),
          toggle: () => {
            void this.api.toggleConfigCategory(cfgCat.key, !cfgCat.on).then(() => {
              toast('「' + cfgCat.label + '」已' + (!cfgCat.on ? '启用' : '停用'));
              void this.init();
            }).catch((error) => toast(error instanceof Error ? error.message : '配置启停失败', true));
          },
          blocks: cfgCat.blocks.map((b) => ({
            title: b.title,
            fields: b.fields.map((f) => ({
              ...f,
              isSwitch: f.kind === 'switch',
              isSecret: f.kind === 'secret',
              isText: f.kind === 'text',
              isTextarea: f.kind === 'textarea',
              isNumber: f.kind === 'number',
              isReadonly: f.kind === 'readonly',
              ph: f.kind === 'secret' ? (f.configured ? '已设置' : '未设置') : '',
              sw: this.sw(f.on === true, accent),
              flip: () => {
                f.on = !f.on;
                this.setState({});
              },
              inputStyle: {
                height: '32px', width: 'min(480px,100%)', border: '1px solid #DEE0E3', borderRadius: '6px',
                padding: '0 10px', fontSize: '13px', background: f.kind === 'readonly' ? '#F7F8FA' : '#fff',
                color: f.kind === 'readonly' ? '#646A73' : '#1F2329',
                fontFamily: 'ui-monospace,SFMono-Regular,Menlo,monospace',
              } as StyleObj,
              areaStyle: {
                width: 'min(480px,100%)', minHeight: '96px', border: '1px solid #DEE0E3', borderRadius: '6px',
                padding: '8px 10px', fontSize: '12px', fontFamily: 'ui-monospace,SFMono-Regular,Menlo,monospace',
              } as StyleObj,
            })),
          })),
        }
      : null;

    const customerMeta = this.db.customerList || { total: rows.customers.length, totalIsEstimate: false, nextCursor: null };
    const customerTimeline = customerContext?.timeline || [];
    const customerChat = customerContext?.chat.items || [];
    const customerTags = customerContext?.tags || [];
    const customerStart = rows.customers.length ? this.state.customerPage * 50 + 1 : 0;
    const customerEnd = rows.customers.length ? customerStart + rows.customers.length - 1 : 0;
    const customerEstimate = customerMeta.totalIsEstimate ? '（估算）' : '';
    const customerButtonStyle = (enabled: boolean): StyleObj => ({
      height: '28px', minWidth: '28px', padding: '0 8px', border: '1px solid #DEE0E3', borderRadius: '6px',
      background: '#fff', color: enabled ? '#1F2329' : '#BBBFC4', fontSize: '12px', cursor: enabled ? 'pointer' : 'not-allowed',
    });

    return {
      go,
      customersPage: {
        filters: this.state.customerFilters,
        totalLabel: `共 ${customerMeta.total.toLocaleString()} 位客户${customerEstimate}`,
        rangeLabel: this.state.customerLoading ? '正在读取客户列表…' : rows.customers.length ? `第 ${customerStart} – ${customerEnd} 条，共 ${customerMeta.total.toLocaleString()} 条${customerEstimate}` : `暂无客户，共 ${customerMeta.total.toLocaleString()} 条${customerEstimate}`,
        previous: () => this.previousCustomerPage(),
        next: () => this.nextCustomerPage(),
        query: () => this.queryCustomers(),
        clear: () => this.clearCustomers(),
        page: this.state.customerPage + 1,
        previousStyle: customerButtonStyle(this.state.customerPage > 0 && !this.state.customerLoading),
        nextStyle: customerButtonStyle(Boolean(customerMeta.nextCursor) && !this.state.customerLoading),
        loading: this.state.customerLoading,
        error: this.state.customerError,
        empty: rows.customers.length === 0 && !this.state.customerLoading && !this.state.customerError,
      },
      customerDetail: {
        ready: this.db.customerDetail.status === 'ready' && Boolean(customerContext),
        notFound: this.db.customerDetail.status === 'not_found',
        timeline: customerTimeline,
        timelineEmpty: customerTimeline.length === 0,
        tags: customerTags,
        chatItems: customerChat.map((item) => ({ channel: item.chatType === 'private' ? '私聊' : '群聊', type: item.messageType, time: item.sentAt })),
        chatEmpty: customerChat.length === 0,
        chatTotalLabel: `共 ${customerContext?.chat.total || 0} 条安全摘要`,
        archiveLabel: customerContext?.chat.localArchiveAvailable ? '本地摘要可用' : '本地摘要不可用',
        snapshotLabel: customerContext?.nonAtomicSnapshot ? '非原子本地快照' : '本地安全快照',
        providerLabel: customerContext?.realExternalCallExecuted ? '发生外部调用' : '未调用外部 Provider',
        addedAt: customerContext?.profile.addedAt || '—',
        lastInteractAt: customerContext?.profile.lastInteractAt || '—',
        channelId: customerContext?.profile.channelId == null ? '—' : String(customerContext.profile.channelId),
        hxcAvailable: customerContext?.hxc.available === true,
        hxcUnavailable: customerContext?.hxc.available !== true,
        hxcStatusAvailable: Boolean(customerContext?.hxc.status),
        hxcEmpty: customerContext?.hxc.available === true && !customerContext.hxc.status,
        hxcLastSyncedAt: customerContext?.hxc.lastSyncedAt || '暂无成功同步',
        hxcTier: customerContext?.hxc.status?.subscriptionTier || '—',
        hxcExpiresAt: customerContext?.hxc.status?.subscriptionExpiresAt || '—',
        hxcDaysRemaining: customerContext?.hxc.status ? String(customerContext.hxc.status.daysRemaining) : '—',
        hxcChatUsage: customerContext?.hxc.status ? `${customerContext.hxc.status.currentPeriodUsed} / ${customerContext.hxc.status.monthlyChatQuota}` : '—',
        hxcConsultationUsage: customerContext?.hxc.status ? `${customerContext.hxc.status.consultationUsed} / ${customerContext.hxc.status.consultationLimit}` : '—',
        hxcSessions: customerContext?.hxc.status ? `${customerContext.hxc.status.sessions7d} / ${customerContext.hxc.status.sessions30d} / ${customerContext.hxc.status.sessionsTotal}` : '—',
        hxcMessages: customerContext?.hxc.status ? `${customerContext.hxc.status.userMessages7d} / ${customerContext.hxc.status.userMessages30d} / ${customerContext.hxc.status.userMessagesTotal}` : '—',
        hxcLastUsedAt: customerContext?.hxc.status?.lastUsedAt || '—',
        hxcLastCapability: customerContext?.hxc.status?.lastCapability || '—',
        hxcProfile: customerContext?.hxc.status ? [customerContext.hxc.status.businessStage, customerContext.hxc.status.mainLineType, customerContext.hxc.status.userSegment, ...customerContext.hxc.status.focusTopics, customerContext.hxc.status.painTag].filter(Boolean).join(' · ') || '—' : '—',
        surveyItems: (customerSurvey?.items || []).map((item) => ({
          submissionId: String(item.submissionId),
          questionnaireId: String(item.questionnaireId),
          submittedAt: item.submittedAt,
          score: String(item.score),
          choices: item.choices.map((choice) => `题目 ${choice.questionId}（${choice.questionType}）→ 选项 ${choice.optionIds.join('、') || '无'}`).join('；') || '无选择题答案',
        })),
        surveyEmpty: (customerSurvey?.items.length || 0) === 0,
        surveyTruncated: Boolean(customerSurvey?.scanTruncated || customerSurvey?.resultTruncated),
        surveyCompletenessLabel: customerSurvey?.scanTruncated || customerSurvey?.resultTruncated ? '结果不完整' : '当前扫描范围内完整',
      },
      productFormPage: {
        title: productFormValue ? '编辑普通商品' : '创建普通商品',
        item: productFormValue || { code: '', name: '', price: '0.00', description: '', currency: 'CNY', stockQuantity: 0, images: [], adminProjection: { schemaVersion: 1, status: 'draft', enabled: false, buyButtonText: '', requireMobile: false, leadProgramId: null, leadChannelId: null, leadQrTitle: '', leadQrSubtitle: '', completionRedirectEnabled: false, completionRedirectUrl: '', completionTarget: null, wecomTagging: {}, slices: [] }, externalPush: { enabled: false, configurationReference: '', updatedAt: '' } },
        imagesText: (productFormValue?.images || []).join('\n'),
        completionTargetText: productFormValue?.adminProjection?.completionTarget ? JSON.stringify(productFormValue.adminProjection.completionTarget, null, 2) : '',
        wecomTaggingText: JSON.stringify(productFormValue?.adminProjection?.wecomTagging || {}, null, 2),
        requireMobileOff: productFormValue?.adminProjection?.requireMobile !== true,
        completionRedirectOff: productFormValue?.adminProjection?.completionRedirectEnabled !== true,
        externalPushOff: productFormValue?.externalPush?.enabled !== true,
        save: () => this.saveCommerceProduct('product'),
      },
      spProductFormPage: {
        title: serviceFormValue ? '编辑周期商品' : '创建周期商品',
        item: serviceFormValue || { code: '', name: '', price: '0.00', description: '', currency: 'CNY', stockQuantity: 0, images: [], adminProjection: { schemaVersion: 1, status: 'service_period_draft', enabled: false, buyButtonText: '', requireMobile: false, leadProgramId: null, leadChannelId: null, leadQrTitle: '', leadQrSubtitle: '', completionRedirectEnabled: false, completionRedirectUrl: '', completionTarget: null, wecomTagging: {}, slices: [] }, externalPush: { enabled: false, configurationReference: '', updatedAt: '' } },
        imagesText: (serviceFormValue?.images || []).join('\n'),
        completionTargetText: serviceFormValue?.adminProjection?.completionTarget ? JSON.stringify(serviceFormValue.adminProjection.completionTarget, null, 2) : '',
        wecomTaggingText: JSON.stringify(serviceFormValue?.adminProjection?.wecomTagging || {}, null, 2),
        requireMobileOff: serviceFormValue?.adminProjection?.requireMobile !== true,
        completionRedirectOff: serviceFormValue?.adminProjection?.completionRedirectEnabled !== true,
        externalPushOff: serviceFormValue?.externalPush?.enabled !== true,
        save: () => this.saveCommerceProduct('service'),
      },
      couponFormPage: {
        title: couponFormValue ? '编辑优惠券' : '创建优惠券',
        item: couponFormValue ? {
          ...couponFormValue,
          discount: ((couponFormValue.discountAmountTotal || 0) / 100).toFixed(2),
          claimStartsAtInput: dateInput(couponFormValue.claimStartsAt),
          claimEndsAtInput: dateInput(couponFormValue.claimEndsAt),
          useStartsAtInput: dateInput(couponFormValue.useStartsAt),
          useEndsAtInput: dateInput(couponFormValue.useEndsAt),
          targetRefsText: (couponFormValue.targetRefs || []).join(', '),
          fixedSelected: couponFormValue.validityMode === 'fixed_range',
          relativeSelected: couponFormValue.validityMode !== 'fixed_range',
        } : {
          name: '', discount: '0.00', totalIssueLimit: 1, perUserIssueLimit: 1,
          claimStartsAtInput: '', claimEndsAtInput: '', validityMode: 'relative_days',
          useStartsAtInput: '', useEndsAtInput: '', relativeValidityDays: 7,
          instructions: '', targetRefsText: '', fixedSelected: false, relativeSelected: true,
        },
        saveDraft: () => this.saveCouponForm(false),
        savePublish: () => this.saveCouponForm(true),
      },
      questionnaireFormPage: {
        title: questionnaireFormValue ? '编辑问卷' : '创建问卷',
        item: questionnaireFormValue ? {
          ...questionnaireFormValue,
          questionsJson: JSON.stringify(questionnaireFormValue.questions || [], null, 2),
          assessmentConfigJson: JSON.stringify(questionnaireFormValue.assessmentConfig || {}, null, 2),
          allSelected: questionnaireFormValue.answerDisplayMode !== 'one_by_one',
          oneSelected: questionnaireFormValue.answerDisplayMode === 'one_by_one',
          assessmentDisabled: !questionnaireFormValue.assessmentEnabled,
          enabledState: !questionnaireFormValue.off,
        } : {
          internalName: '', title: '', description: '', slug: '', assessmentEnabled: false, off: true,
          questionsJson: JSON.stringify([{ type: 'textarea', title: '请填写问题', assessment_dimension_key: '', sidebar_profile_field: '', required: true, sort_order: 0, placeholder_text: '', validation: {}, options: [] }], null, 2),
          assessmentConfigJson: '{}', allSelected: true, oneSelected: false, assessmentDisabled: true, enabledState: false,
        },
        save: () => this.saveQuestionnaireForm(false),
        publish: () => this.saveQuestionnaireForm(true),
      },
      questionnairePage: {
        create: () => this.goto('questionnaireDetail'),
        blockedTemplate: () => this.blocked('当前 OpenAPI 没有独立的测评问卷模板资源；可在问卷 JSON 中配置 assessment 字段'),
      },
      channelDrawer: {
        open: s.channelDrawerOpen,
        loading: s.channelDrawerLoading,
        error: s.channelDrawerError,
        previewError: s.channelDrawerPreviewError,
        assetError: s.channelDrawerAssetError,
        ready: Boolean(s.channelDrawerChannel),
        channel: s.channelDrawerChannel ? {
          ...s.channelDrawerChannel,
          link: this.channelShareValue(s.channelDrawerChannel),
          statusLabel: ({ active: '启用', inactive: '停用', archived: '归档' } as Record<string, string>)[s.channelDrawerChannel.status] || s.channelDrawerChannel.status,
          copy: () => this.copyChannelLink(s.channelDrawerChannel),
          share: () => this.shareChannelLink(s.channelDrawerChannel),
        } : { name: '', code: '', statusLabel: '', link: '', copy: () => undefined, share: () => undefined },
        preview: s.channelDrawerPreview ? {
          state: s.channelDrawerPreview.lifecycleState || '—',
          assignees: s.channelDrawerPreview.assignees.map((item) => ({ name: item.name, staffId: item.staffId, ratio: item.ratioPercent == null ? '—' : `${item.ratioPercent}%`, cap: item.maxScans24h == null ? '—' : String(item.maxScans24h) })),
          assigneesEmpty: s.channelDrawerPreview.assignees.length === 0,
          hasAssignees: s.channelDrawerPreview.assignees.length > 0,
          blockers: s.channelDrawerPreview.blockers.join('、') || '无',
          localOnly: s.channelDrawerPreview.localOnly,
        } : { state: '—', assignees: [], assigneesEmpty: true, hasAssignees: false, blockers: '—', localOnly: true },
        asset: latestDrawerAsset,
        assetRequest: () => this.requestChannelAsset(s.channelDrawerChannel?.resourceId, channelAssetKind(s.channelDrawerChannel), 'drawer'),
        assetBusy: s.channelDrawerAssetBusy,
        entrants: s.channelDrawerEntrants.map((item) => ({ ...item, lastInteract: item.lastInteractAt || '—' })),
        entrantsEmpty: s.channelDrawerEntrants.length === 0,
        close: () => this.closeChannelDrawer(),
        stop: (event: Event) => event.stopPropagation(),
      },
      channelFormPage: {
        title: s.channelFormNotFound ? '渠道不存在' : channelFormValue ? '编辑渠道' : '创建渠道',
        notFound: s.channelFormNotFound,
        exists: !s.channelFormNotFound,
        finalUrlPreview: channelFormValue?.carrierType === 'link' ? channelFinalUrlPreview || '填写链接 URL 后生成本地预览' : '二维码载体不生成本地链接预览',
        copyLink: () => this.copyChannelFormLink(),
        shareLink: () => this.shareChannelFormLink(),
        preview: () => this.refreshChannelFinalUrlPreview(),
        selectTag: () => this.selectChannelTag(),
        clearTag: () => this.clearChannelTag(),
        tags: channelTagOptions,
        staffRows: formStaffRows,
        staffCount: `${formStaffRows.length} / 5`,
        hasStaff: formStaffRows.length > 0,
        noStaff: formStaffRows.length === 0,
        pickStaff: () => this.cfAddStaff(),
        previewError: s.channelFormPreviewError,
        assignmentPreview: s.channelFormPreview ? {
          state: s.channelFormPreview.lifecycleState || '—',
          blockers: s.channelFormPreview.blockers.join('、') || '无',
          assignees: s.channelFormPreview.assignees.map((item) => ({ name: item.name, staffId: item.staffId, ratio: item.ratioPercent == null ? '—' : `${item.ratioPercent}%`, cap: item.maxScans24h == null ? '—' : String(item.maxScans24h) })),
          hasAssignees: s.channelFormPreview.assignees.length > 0,
          noAssignees: s.channelFormPreview.assignees.length === 0,
        } : { state: '—', blockers: '未读取', assignees: [], hasAssignees: false, noAssignees: true },
        assetError: s.channelFormAssetError,
        asset: latestFormAsset,
        assetBusy: s.channelFormAssetBusy,
        hasResource: Boolean(channelFormValue?.resourceId),
        noResource: !channelFormValue?.resourceId,
        assetKindLabel: this.channelAssetKindLabel(channelAssetKind(channelFormValue)),
        assetRequest: () => this.requestChannelAsset(channelFormValue?.resourceId, channelAssetKind(channelFormValue), 'form'),
        providerLinks: {
          busy: s.channelProviderBusy,
          error: s.channelProviderError,
          ids: s.channelProviderLinks,
          hasIds: s.channelProviderLinks.length > 0,
          noIds: s.channelProviderLinks.length === 0,
          loadList: () => this.loadChannelProviderLinks(),
          loadOne: () => this.loadChannelProviderLink(),
          create: () => this.createChannelProviderLink(),
          update: () => this.updateChannelProviderLink(),
          delete: () => this.deleteChannelProviderLink(),
          reconcile: () => this.reconcileChannelProviderLink(),
          item: channelProviderLink ? {
            id: channelProviderLink.link_id,
            name: channelProviderLink.link_name,
            url: channelProviderLink.url,
            userIds: channelProviderLink.user_ids.join(', '),
            departmentIds: channelProviderLink.department_ids.join(', '),
            skipVerify: channelProviderLink.skip_verify,
            noSkipVerify: !channelProviderLink.skip_verify,
          } : { id: '', name: '', url: '', userIds: '', departmentIds: '', skipVerify: false, noSkipVerify: true },
          receipt: channelProviderReceipt ? {
            has: true,
            id: String(channelProviderReceipt.receipt.receipt_id),
            state: channelProviderReceipt.receipt.state,
            outcome: channelProviderReceipt.outcome,
            dispatched: channelProviderReceipt.receipt.business_endpoint_dispatched ? '是' : '否',
            external: channelProviderReceipt.receipt.real_external_call_executed ? '是' : '否',
            digest: channelProviderReceipt.receipt.outcome_digest || '—',
            canReconcile: channelProviderReceipt.canReconcile,
          } : { has: false, id: '', state: '', outcome: '', dispatched: '否', external: '否', digest: '—', canReconcile: false },
        },
        history: {
          visible: channelHistoryAvailable && this.qs().get('history') === '1',
          available: channelHistoryAvailable,
          loading: s.channelHistoryLoading,
          error: s.channelHistoryError,
          loaded: channelHistory !== null,
          canLoad: channelHistoryAvailable && !s.channelHistoryLoading && channelHistory === null,
          canReload: channelHistoryAvailable && !s.channelHistoryLoading && channelHistory !== null,
          hasContacts: channelHistoryContacts.length > 0,
          noContacts: channelHistory !== null && channelHistoryContacts.length === 0,
          hasAssignees: channelHistoryAssignees.length > 0,
          noAssignees: channelHistory !== null && channelHistoryAssignees.length === 0,
          contacts: channelHistoryContacts,
          assignees: channelHistoryAssignees,
          range: channelHistoryRange,
          showPrevious: Boolean(channelHistory && channelHistory.offset > 0),
          showNext: Boolean(channelHistory && channelHistory.offset + channelHistory.contacts.length < channelHistory.total),
          load: () => this.loadChannelHistory(0),
          reload: () => this.loadChannelHistory(0),
          previous: () => this.loadChannelHistory(Math.max(0, (channelHistory?.offset || 0) - CHANNEL_HISTORY_PAGE_SIZE)),
          next: () => this.loadChannelHistory((channelHistory?.offset || 0) + CHANNEL_HISTORY_PAGE_SIZE),
        },
        item: channelFormValue ? {
          ...channelFormValue,
          finalUrl: channelFinalUrlPreview || channelFormValue.finalUrl || '',
          imageIds: (channelFormValue.welcomeImageLibraryIds || []).join(', '), miniIds: (channelFormValue.welcomeMiniprogramLibraryIds || []).join(', '), attachmentIds: (channelFormValue.welcomeAttachmentLibraryIds || []).join(', '), groupInviteIds: (channelFormValue.welcomeGroupInviteLibraryIds || []).join(', '), assignmentConfigJson: JSON.stringify(channelFormValue.assignmentConfig || {}, null, 2),
          noEntryTag: !channelFormValue.entryTagId, hasEntryTag: Boolean(channelFormValue.entryTagId),
          qrcodeType: channelFormValue.channelType !== 'wecom_customer_acquisition', acquisitionType: channelFormValue.channelType === 'wecom_customer_acquisition', qrcodeCarrier: channelFormValue.carrierType !== 'link', linkCarrier: channelFormValue.carrierType === 'link',
          statusActive: channelFormValue.status === 'active', statusInactive: channelFormValue.status === 'inactive', statusArchived: channelFormValue.status === 'archived', singleOwner: channelFormValue.assignmentMode !== 'multi_staff', multiStaff: channelFormValue.assignmentMode === 'multi_staff', ratio: channelFormValue.assignmentStrategy !== 'cap_switch', capSwitch: channelFormValue.assignmentStrategy === 'cap_switch', autoAcceptOff: !channelFormValue.autoAcceptFriend,
        } : { name: '', code: '', channelType: 'qrcode', carrierType: 'qrcode', status: 'inactive', sceneValue: '', qrUrl: '', ownerStaffId: '', customerChannel: '', linkUrl: '', finalUrl: '', welcomeMessage: '', imageIds: '', miniIds: '', attachmentIds: '', groupInviteIds: '', autoAcceptFriend: false, autoAcceptOff: true, entryTagId: '', entryTagName: '', entryTagGroupName: '', noEntryTag: true, hasEntryTag: false, assignmentMode: 'single_owner', assignmentStrategy: 'ratio', overflowPolicy: '', assignmentConfigJson: '{}', qrcodeType: true, acquisitionType: false, qrcodeCarrier: true, linkCarrier: false, statusActive: false, statusInactive: true, statusArchived: false, singleOwner: true, multiStaff: false, ratio: true, capSwitch: false },
        save: () => this.saveChannelForm(),
      },
      orderDetailPage: {
        item: rows.orders[0] || { no: '—', plat: '未知', status: '暂无订单', amount: '0.00', tone: 'gray', recordOrigin: 'native', historicalRefunds: [] },
        statusStyle: mk(rows.orders[0]?.tone || 'gray'),
        isHistorical: rows.orders[0]?.recordOrigin === 'v1_history',
        isNative: rows.orders[0]?.recordOrigin !== 'v1_history',
        hasHistoricalRefunds: (rows.orders[0]?.historicalRefunds?.length || 0) > 0,
        noHistoricalRefunds: (rows.orders[0]?.historicalRefunds?.length || 0) === 0,
        historicalRefunds: rows.orders[0]?.historicalRefunds || [],
        submitRefund: () => this.submitRefundIntent(),
      },
      groupOpsPage: { rows: groupOpsRows, total: groupOpsRows.length, members: this.db.staff, memberCount: this.db.staff.length, create: () => this.goto('groupopsDetail'), directory: () => this.openGroupOpsDirectory() },
      groupOpsDetailPage: {
        item: groupOpsDetail ? { ...groupOpsDetail, assetText: groupOpsDetail.assets.map((asset) => asset.reference).join('\n'), nodesJson: JSON.stringify(groupOpsDetail.nodes, null, 2), previewText: groupOpsDetail.previewLines.join('\n') || '暂无可预览内容', issuesText: groupOpsDetail.previewIssues.join('、') || '无' } : { plan: { name: '', revision: 0, status: 'draft', id: '' }, assetText: '', nodesJson: JSON.stringify([{ position: 1, kind: 'message', messageText: '请输入群消息', materialReference: '' }], null, 2), webhookReference: '', webhookUrl: '', previewText: '保存后由 previewGroupOpsPlanContent 返回', issuesText: '尚未校验' },
        members: groupOpsMemberOptions,
        directory: () => this.openGroupOpsDirectory(),
        save: () => this.saveGroupOpsForm(), back: () => this.goto('groupops'), pickImage: () => this.pickGroupOpsMaterial('image'), pickMiniProgram: () => this.pickGroupOpsMaterial('miniprogram'), pickAttachment: () => this.pickGroupOpsMaterial('attachment'), copyWebhookUrl: () => { const value = groupOpsDetail?.webhookUrl || ''; if (!value) return toast('尚未配置可复制的 Webhook URL', true); copyText(value, (message, error) => toast(message, error)); },
      },
      hxcPage: {
        rows: rows.agents.map((item) => ({ ...item, cs: mk(item.tone), edit: () => this.goto('agentEdit', '?id=' + encodeURIComponent(item.code)), archive: () => this.archiveHxcSender(item.code) })),
        orderText: rows.agents.map((item) => item.senderId || item.code).join('\n'),
        create: () => this.goto('agentEdit'),
        refresh: () => this.refreshHxcDirectory(),
        reorder: () => this.reorderHxcSenders(),
        item: hxcEdit ? { ...hxcEdit, activeOff: hxcEdit.isActive === false } : { senderId: '', code: '', name: '', priority: rows.agents.length, isActive: true, activeOff: false },
        save: () => this.saveHxcSender(),
        back: () => this.goto('agents'),
      },

      /* ---- 渠道表单 ---- */
      cgo, cn, cp,
      stepTitle: ['基础配置', '渠道载体', '客服分配', '欢迎语素材', '入渠标签'][cstep - 1],
      welcomePreview:
        '{{客户名}} 恭喜你报名成功#5天沙龙邀约破局共学营\n⭕辛苦填一下问卷，让我们更好了解你的需求，给你提供更好的服务：\nhttps://www.xinliushangye.com/s/salon-yixiang-gongxueying\n\n⏰开营时间：8月6日\n🎯进群时间：8月5日',
      welcomeText:
        '{{客户名}} 恭喜你报名成功#5天沙龙邀约破局共学营\n⭕辛苦填一下问卷，让我们更好了解你的需求，给你提供更好的服务：\nhttps://www.xinliushangye.com/s/salon-yixiang-gongxueying\n\n⏰开营时间：8月6日\n🎯进群时间：8月5日',

      /* ---- Agent 编辑 ---- */
      ago, an, ap,
      aTitle: ['基本信息', '当前绑定人群包', 'Prompt 配置', '固定素材'][astep - 1],
      promptTokens: ['插入 {{问卷信息}}', '插入 {{最近20条聊天信息}}', '插入 {{用户标签}}', '插入 {{激活信息}}'].map((t) => ({ t })),
      taskPrompt: '问卷信息：\n{{问卷信息}}\n\n从下面 12 门课程中推荐 2 门：',

      /* ---- 通用选择器写回（渠道表单 / Agent / 迁移 / 表单渠道码） ---- */
      cf: (() => {
        const staffSrc: { name: string; uid: string; ratio: string }[] = s.cfStaff
          ? s.cfStaff.map((m) => ({ name: m.name, uid: m.uid || m.id, ratio: Math.round(100 / Math.max(s.cfStaff!.length, 1)) + '%' }))
          : rows.edAssignees;
        const mats = s.cfMats.map((m) =>
          this.matRow(m, () => this.setState({ cfMats: s.cfMats.filter((x) => !(x.id === m.id && x.kind === m.kind)) })),
        );
        const cnt = (k: string): number => s.cfMats.filter((m) => m.kind === k).length;
        return {
          staffRows: staffSrc,
          staffCount: staffSrc.length + ' / 5',
          addStaff: () => this.cfAddStaff(),
          mats,
          hasMats: mats.length > 0,
          noMats: mats.length === 0,
          matCountText:
            '已选 ' + mats.length + ' / 9 个素材 · 图片 ' + cnt('image') + ' · 小程序 ' + cnt('mp') + ' · 附件 ' + cnt('attach') + ' · 客户群 ' + cnt('group'),
          addImage: () => this.cfAddMaterial('image'),
          addMp: () => this.cfAddMaterial('mp'),
          addAttach: () => this.cfAddMaterial('attach'),
          addGroup: () => this.cfAddMaterial('group'),
          tagsText: s.cfTags ? (s.cfTags.length ? s.cfTags.map((t) => t.name).join(' / ') : '未配置') : '沙龙邀约 / 共学营',
          pickTags: () => this.cfPickTags(),
        };
      })(),
      ag: (() => {
        const src = s.agMats || this.defaultAgMats();
        const mats = src.map((m) =>
          this.matRow(m, () => this.setState({ agMats: src.filter((x) => !(x.id === m.id && x.kind === m.kind)) })),
        );
        const cnt = (k: string): number => src.filter((m) => m.kind === k).length;
        return {
          mats,
          hasMats: mats.length > 0,
          noMats: mats.length === 0,
          matCountText: '已选 ' + cnt('image') + ' 图片 / ' + cnt('mp') + ' 小程序 / ' + cnt('attach') + ' PDF / ' + cnt('group') + ' 客户群',
          addImage: () => this.agAddMaterial('image'),
          addMp: () => this.agAddMaterial('mp'),
          addAttach: () => this.agAddMaterial('attach'),
          addGroup: () => this.agAddMaterial('group'),
        };
      })(),
      mig: (() => {
        const preview = s.migPreview;
        return {
          fromName: '由导入文件逐行指定',
          toName: '由导入文件逐行指定',
          pickFrom: () => toast('当前 OpenAPI 仅支持在 CSV / XLSX 中逐行指定原负责人；未发起请求', true),
          pickTo: () => toast('当前 OpenAPI 仅支持在 CSV / XLSX 中逐行指定目标负责人；未发起请求', true),
          fileName: s.migFileName || '尚未选择 CSV',
          downloadTemplate: () => this.migDownloadTemplate(),
          parseCsv: () => this.migParseCsv(),
          setConfirmed: (event: Event) => this.migSetConfirmed(event),
          execute: () => this.migExecute(),
          downloadErrors: () => this.migDownloadReport('errors'),
          downloadResults: () => this.migDownloadReport('results'),
          hasPreview: Boolean(preview),
          noPreview: !preview,
          id: preview?.id || '—',
          hash: preview?.hash || '—',
          expiresAt: preview?.expiresAt || '—',
          status: preview?.executed ? '已执行（本地事务）' : '待执行',
          rows: (preview?.rows || []).map((row) => ({ customerId: row.customerId, fromId: row.expectedOwnerStaffId, toId: row.targetOwnerStaffId, expectedUpdatedAt: row.expectedUpdatedAt })),
          issues: (preview?.issues || []).map((issue) => ({ line: issue.line, code: issue.code })),
          results: (preview?.result || []).map((row) => ({ customerId: row.customerId, fromId: row.previousOwnerStaffId, toId: row.targetOwnerStaffId, updatedAt: row.updatedAt })),
          validCount: preview?.rows.length || 0,
          issueCount: preview?.issues.length || 0,
          resultCount: preview?.result.length || 0,
          hasIssues: Boolean(preview?.issues.length),
          hasResults: Boolean(preview?.result.length),
          canExecute: Boolean(preview && !preview.executed && preview.rows.length && !s.saving),
          savingText: s.saving ? '处理中…' : '上传并生成预览',
        };
      })(),
      pf: { channelText: this.channelName(s.pfChannelId), pickChannel: () => this.pickChannelFor('pf') },
      spf: { channelText: this.channelName(s.spfChannelId), pickChannel: () => this.pickChannelFor('spf') },

      /* ================= 各页交互 ================= */

      /* 自动化运营 */
      aud: {
        groups: audGroups,
        customCount: this.db.audienceGroups.length,
        curGroupName,
        isDefaultGroup: s.groupId === 0,
        isCustomGroup: s.groupId !== 0,
        rows: audienceRows,
        total: audPkgs.length,
        openCreateGroup: () => this.openGroupModal('create'),
        openEditGroup: () => this.openGroupModal('edit'),
        deleteGroup: () => this.deleteGroup(),
        saveGroup: () => this.saveGroup(),
        closeModal: () => this.closeModal(),
        groupModalOpen: s.modal === 'group',
        groupModalTitle: s.groupMode === 'edit' ? '编辑分组' : '新增分组',
      },

      /* 人群包编辑器 */
      ae: {
        pkg: aePkg,
        nav: aeNav,
        goPanel: aeGo,
        panel: aePanel,
        groupOpts: aeGroupOpts,
        incOpts: aeIncOpts,
        dailyOpts: aeDailyOpts,
        refreshModeOpts: this.aeSelectOpts(pkg?.refreshMode || 'manual', [['manual', '手动'], ['scheduled', '定时']]),
        senders: aeSenders,
        sendersText: aeSenders.map((sender) => sender.userid).join('\n'),
        members: aeMembers,
        memberTotal: aeMembers.length + ' 人（共 ' + (aePkg?.countText || '0') + ' 人，显示前 200）',
        records: aeRecordRows,
        recordTotal: aeRecords.length ? '共 ' + aeRecords.length + ' 条' : '暂无发送记录',
        preview: aePreview,
        templates: this.state.audienceTemplates,
        templateOpts: aeTemplateOpts,
        templateReady: this.state.audienceTemplates.length > 0,
        templateError: this.state.audienceTemplateError,
        templateParamsText: this.state.audienceTemplateParametersText,
        templatePreview: aeTemplatePreview,
        agents: aeAgents,
        saveBasic: () => this.saveAudienceBasic(),
        saveBinding: () => this.saveAudienceBinding(),
        unbind: () => this.unbindAutomation(),
        addSender: () => this.addSender(),
        saveSenders: () => this.saveSenders(),
        refresh: () => this.previewAudience(),
        back: () => this.goto('automation'),
        snapshot: () => this.snapshotAudience(),
        previewConfiguration: () => this.previewAudience(),
        savePreview: () => this.saveAndPreviewAudience(),
        previewTemplate: () => this.previewAudienceTemplate(),
        saveTemplate: () => this.saveAudienceTemplate(),
        materialize: () => this.materializeAudience(),
      },

      /* 运营闭环 */
      cycles: { rows: cycleRows, total: this.db.cycleTasks.length },
      run: runVals,

      /* 问卷运营配置 */
      qops: {
        q: qRow ? { ...qRow, index: qid, status: qRow.off ? '已停用' : '启用中' } : null,
        ops,
        nav: opsNav,
        goTab: opsGo,
        panel: opsPanel,
        postOn: s.postEnabled,
        postSw: this.sw(s.postEnabled, accent),
        flipPost: () => this.setState({ postEnabled: !s.postEnabled }),
        isQr: s.postType === 'channel_qr',
        isRedirect: s.postType === 'redirect',
        cardQr: opsCard(s.postType === 'channel_qr'),
        cardRedirect: opsCard(s.postType === 'redirect'),
        pickQr: () => this.setState({ postType: 'channel_qr' }),
        pickRedirect: () => this.setState({ postType: 'redirect' }),
        pushOn: s.pushEnabled,
        pushOff: ops ? !ops.pushEnabled : false,
        pushSw: this.sw(s.pushEnabled, accent),
        flipPush: () => this.setState({ pushEnabled: !s.pushEnabled }),
        params: opsParams,
        addParam: () => {
          if (!this.paramsDraft) this.paramsDraft = [];
          this.paramsDraft.push({ key: '', value: '' });
          this.setState({});
        },
        channelOpts,
        channelText: s.opsChannelId ? this.channelName(s.opsChannelId) : ops?.channelId || '不配置引流渠道码',
        pickChannel: () => this.pickChannelFor('ops'),
        redirectTypeOpts,
        freqOpts,
        save: () => this.saveOps(),
        testPush: () => { const qid = this.currentQid(); confirmBox('创建本地推送测试记录', '该 operation 只创建 queued 本地测试记录，不执行外部派发。确认继续？', '确认创建', false, () => { void this.api.queueQuestionnairePushTest(qid).then((result) => toast(`本地测试记录 ${result.id} 已创建，状态 ${result.status}，attempt_count ${result.attemptCount}；未执行外部派发`)).catch((error) => toast(error instanceof Error ? error.message : '测试记录创建失败', true)); }); },
        showQuestionnaireLogs: () => this.setState({ opsLogScope: 'questionnaire', opsLogKeyword: '', opsLogStatus: '' }),
        showGlobalLogs: () => { if (this.api.mode !== 'http') { toast('backend_blocked：测试/本地模式不使用 Mock 全局外推日志', true); return; } void this.api.listGlobalQuestionnairePushLogs().then((logs) => { this.globalQuestionnairePushLogs = logs; this.setState({ opsLogScope: 'global', opsLogKeyword: '', opsLogStatus: '' }); }).catch((error) => toast(error instanceof Error ? error.message : '全局外推日志读取失败', true)); },
        filterLogs: () => this.setState({ opsLogKeyword: this.opsInputVal('opsLogKeyword').trim(), opsLogStatus: this.opsInputVal('opsLogStatus') }),
        resetLogs: () => this.setState({ opsLogKeyword: '', opsLogStatus: '' }),
        logScope: s.opsLogScope === 'global' ? '全部问卷本地外推测试记录' : '当前问卷本地外推测试记录',
        questionnaireLogScopeStyle: { height: '28px', padding: '0 10px', border: '1px solid #DEE0E3', borderRadius: '6px', background: s.opsLogScope === 'questionnaire' ? '#EFF4FF' : '#fff', color: s.opsLogScope === 'questionnaire' ? accent : '#646A73', fontSize: '12px', cursor: 'pointer' },
        globalLogScopeStyle: { height: '28px', padding: '0 10px', border: '1px solid #DEE0E3', borderRadius: '6px', background: s.opsLogScope === 'global' ? '#EFF4FF' : '#fff', color: s.opsLogScope === 'global' ? accent : '#646A73', fontSize: '12px', cursor: 'pointer' },
        logKeyword: s.opsLogKeyword,
        logStatusAll: !s.opsLogStatus,
        logStatusQueued: s.opsLogStatus === 'queued',
        logCount: opsLogRows.length,
        copyPublic: () => qRow?.publicPath ? copyText(new URL(qRow.publicPath, location.origin).toString(), toast) : toast('后端未返回问卷公开地址', true),
        openPublic: () => qRow?.publicPath ? window.open(new URL(qRow.publicPath, location.origin).toString(), '_blank', 'noopener') : toast('后端未返回问卷公开地址', true),
        viewLogs: () => this.setState({ opsTab: 2 }),
        back: () => this.goto('questionnaires'),
      },

      /* 企微标签 */
      tagsPage: {
        groups: tagGroups,
        cur: curTagGroup,
        rows: tagRows,
        capacity: tagCapacity,
        capacityPct: Math.min(100, Math.round((tagCapacity / 1000) * 100)) + '%',
        modal: tagModal,
        modalOpen: s.modal === 'tag',
        openCreateGroup: () => this.openTagModal('create-group'),
        openCreateTag: () => this.openTagModal('create-tag'),
        openEditGroup: () => this.openTagModal('edit-group'),
        closeModal: () => this.closeModal(),
        save: () => this.saveTagModal(),
        sync: (ev: Event) => {
          const btn = ev.currentTarget as FbEl;
          void this.api.syncWecomTags().then(() =>
            busy(btn, 0, () => {
              toast('标签同步已受理；尚未收到 Provider 同步结果');
              void this.init();
            }),
          ).catch((error) => toast(error instanceof Error ? error.message : '标签同步受理失败', true));
        },
        search: (ev: Event) => this.setState({ tagQ: (ev.target as HTMLInputElement).value }),
      },

      /* 分享组件 */
      share,

      /* 素材库 */
      imagesPage: {
        cards: imageCards,
        editing: editingImg || null,
        editOpen: s.modal === 'imgEdit' && !!editingImg,
        uploadOpen: s.modal === 'imgUpload',
        openUpload: () => this.setState({ modal: 'imgUpload', editingName: '' }),
        closeModal: () => this.closeModal(),
        save: () => this.saveImage(),
        toggleText: editingImg ? (editingImg.enabled ? '停用' : '启用') : '',
        toggle: () => {
          if (!editingImg) return;
          void this.api.saveImageItem(editingImg.name, { name: editingImg.name, enabled: !editingImg.enabled }).then(() => {
            toast(editingImg.enabled ? '已停用' : '已启用');
            this.setState({ modal: '' });
            void this.init();
          });
        },
        del: () => {
          if (!editingImg) return;
          confirmBox('删除素材', '确认删除「' + editingImg.name + '」？删除后不可恢复。', '确认删除', true, () => {
            void this.api.deleteImageItem(editingImg).then(() => {
              toast('已删除');
              this.setState({ modal: '' });
              void this.init();
            });
          });
        },
        replaceImage: () => this.blocked('请在上传表单选择真实文件后提交；不提供模拟替换'),
        submitUpload: () => {
          const v = this.readModalInputs(['fImgUpName', 'fImgUpTags']);
          const fileInput = document.getElementById('fImgUpFile') as HTMLInputElement | null;
          const file = fileInput?.files?.[0];
          const fname = file?.name;
          const name = v.fImgUpName || fname || '未命名图片';
          if (!file) return toast('请选择真实图片文件', true);
          void this.api
            .saveImageItem(null, {
              name, file, tags: v.fImgUpTags, desc: '', size: String(file.size), tag: v.fImgUpTags.split(/[,，]/)[0] || '未标记',
              tone: 'gray', bg: 'linear-gradient(135deg,#EFF4FF,#D6E4FF)', enabled: true, uploadedAt: '刚刚',
            })
            .then(() => { toast('图片已上传'); this.setState({ modal: '' }); void this.init(); })
            .catch((error) => toast(error instanceof Error ? error.message : '图片上传失败', true));
        },
      },
      mpPage: {
        cards: mpCards,
        query: s.miniProgramQuery,
        loading: s.miniProgramLoading,
        error: s.miniProgramError,
        empty: !s.miniProgramLoading && !s.miniProgramError && mpCards.length === 0,
        rangeLabel: `显示 ${miniProgramRangeStart}-${miniProgramRangeEnd} / ${miniProgramTotal}`,
        canPrevious: miniProgramOffset > 0,
        canNext: miniProgramOffset + rows.mpItems.length < miniProgramTotal,
        search: () => this.queryMiniPrograms(),
        clear: () => this.clearMiniPrograms(),
        retry: () => void this.loadMiniProgramPage(s.miniProgramOffset, s.miniProgramQuery),
        previous: () => this.previousMiniProgramPage(),
        next: () => this.nextMiniProgramPage(),
        editing: editingMp || null,
        editOpen: s.modal === 'mpEdit' && !!editingMp,
        createOpen: s.modal === 'mpCreate',
        openCreate: () => this.setState({ modal: 'mpCreate', editingName: '' }),
        closeModal: () => this.closeModal(),
        save: () => this.saveMp(),
        toggleText: editingMp ? (editingMp.enabled ? '停用' : '启用') : '',
        toggle: () => {
          if (!editingMp) return;
          void this.api.saveMpItem(editingMp.name, { name: editingMp.name, enabled: !editingMp.enabled }).then(() => {
            toast(editingMp.enabled ? '已停用' : '已启用');
            this.setState({ modal: '' });
            void this.init();
          });
        },
        del: () => {
          if (!editingMp) return;
          confirmBox('删除小程序素材', '确认删除「' + editingMp.name + '」？删除后不可恢复。', '确认删除', true, () => {
            void this.api.deleteMpItem(editingMp).then(() => {
              toast('已删除');
              this.setState({ modal: '' });
              void this.init();
            });
          });
        },
        resolve: () => this.blocked('当前小程序素材契约没有缩略图缓存刷新 operation'),
        pickThumb: () => this.blocked('当前小程序素材契约没有独立缩略图上传 operation'),
      },
      attachPage: {
        rows: attachRows,
        editing: editingAtt || null,
        editOpen: s.modal === 'attEdit' && !!editingAtt,
        uploadOpen: s.modal === 'attUpload',
        openUpload: () => this.setState({ modal: 'attUpload', editingName: '' }),
        closeModal: () => this.closeModal(),
        save: () => this.saveAttach(),
        toggleText: editingAtt ? (editingAtt.enabled ? '停用' : '启用') : '',
        toggle: () => {
          if (!editingAtt) return;
          void this.api.saveAttachItem(editingAtt.name, { name: editingAtt.name, enabled: !editingAtt.enabled }).then(() => {
            toast(editingAtt.enabled ? '已停用' : '已启用');
            this.setState({ modal: '' });
            void this.init();
          });
        },
        del: () => {
          if (!editingAtt) return;
          confirmBox('删除附件', '确认删除「' + editingAtt.name + '」？删除后不可恢复。', '确认删除', true, () => {
            void this.api.deleteAttachItem(editingAtt).then(() => {
              toast('已删除');
              this.setState({ modal: '' });
              void this.init();
            });
          });
        },
        submitUpload: () => {
          const v = this.readModalInputs(['fAttUpName', 'fAttUpTags']);
          const fileInput = document.getElementById('fAttUpFile') as HTMLInputElement | null;
          const file = fileInput?.files?.[0];
          const fname = file?.name;
          const name = v.fAttUpName || fname || '未命名附件';
          if (!file) return toast('请选择真实 PDF 文件', true);
          const ext = name.includes('.') ? name.split('.').pop()!.toUpperCase() : 'PDF';
          void this.api
            .saveAttachItem(null, { name, file, tags: v.fAttUpTags, type: ext, size: String(file.size), uploadedAt: '刚刚', enabled: true })
            .then(() => { toast('附件已上传'); this.setState({ modal: '' }); void this.init(); })
            .catch((error) => toast(error instanceof Error ? error.message : '附件上传失败', true));
        },
      },

      /* 商品 / 周期商品 / 优惠券 */
      productsPage: { rows: productRows, total: rows.products.length },
      spPage: { rows: spRows, total: rows.spProducts.length },
      couponDataPage: {
        coupon: coupon ? { ...coupon, cs: mk(coupon.tone), index: couponIdx } : null,
        stats: couponStats,
        claims: claimRows,
        hasClaims: claimRows.length > 0,
        noClaims: claimRows.length === 0,
        editConfig: () => coupon?.resourceId ? this.goto('couponForm', '?id=' + coupon.resourceId) : this.goto('couponForm'),
        back: () => this.goto('coupons'),
        shareIt: () => couponRows[couponIdx]?.shareIt(),
      },
      couponPage: {
        query: s.couponQuery,
        status: s.couponStatus,
        statusOptions: [
          { value: '', label: '全部状态', selected: !s.couponStatus, unselected: Boolean(s.couponStatus) },
          ...['draft', 'scheduled', 'active', 'sold_out', 'ended', 'stopped', 'archived'].map((value) => ({ value, label: value === 'draft' ? '草稿' : value === 'scheduled' ? '未开始' : value === 'active' ? '进行中' : value === 'sold_out' ? '已领完' : value === 'ended' ? '已结束' : value === 'stopped' ? '已停止' : '已归档', selected: s.couponStatus === value, unselected: s.couponStatus !== value })),
        ],
        setQuery: (event: Event) => this.setState({ couponQuery: (event.currentTarget as HTMLInputElement).value }),
        setStatus: (event: Event) => this.setState({ couponStatus: (event.currentTarget as HTMLSelectElement).value }),
      },

      customer: {
        item: customerItem,
        stages: rows.orderKv,
        save: () => {
          const name = (document.getElementById('fCustomerName') as HTMLInputElement | null)?.value.trim() || '';
          const stageRaw = (document.getElementById('fCustomerStage') as HTMLInputElement | null)?.value.trim() || '';
          if (!customerId || !name) return toast('客户 ID 或姓名为空，未发送请求', true);
          const stageId = stageRaw ? Number(stageRaw) : null;
          if (stageRaw && (!Number.isInteger(stageId) || Number(stageId) < 1)) return toast('阶段 ID 必须是正整数或留空', true);
          void customerAction(() => this.api.updateCustomer(customerId, { name, stageId }), '客户资料与阶段已保存');
        },
        addTag: () => {
          const tagId = Number((document.getElementById('fCustomerTag') as HTMLInputElement | null)?.value || 0);
          if (!customerId || !Number.isInteger(tagId) || tagId < 1) return toast('请输入有效标签 ID', true);
          void customerAction(() => this.api.setCustomerTag(customerId, tagId, true), '客户标签已添加');
        },
        removeTag: () => {
          const tagId = Number((document.getElementById('fCustomerTag') as HTMLInputElement | null)?.value || 0);
          if (!customerId || !Number.isInteger(tagId) || tagId < 1) return toast('请输入有效标签 ID', true);
          void customerAction(() => this.api.setCustomerTag(customerId, tagId, false), '客户标签已移除');
        },
        reload: () => void this.init(),
      },

      /* 配置中心 */
      configPage: { rows: configRows, total: this.db.configCategories.length },
      cfg: {
        cat: cfgVals,
        save: () => this.saveConfig(),
        check: () => this.checkConfig(),
        back: () => this.goto('config'),
      },

      /* ================= 列表页数据 ================= */
      orderPage: { exportWechat: () => this.exportWechatOrders(), saving: this.state.saving },
      rows: {
        customers: rows.customers.map((r) => ({ ...r, view: () => this.goto('customerDetail', '?id=' + encodeURIComponent(r.id)) })),
        tags: rows.tags,
        qa: rows.qa,
        msgs: rows.msgs.map((m) => ({
          ...m,
          wrap: { alignSelf: m.me ? 'flex-end' : 'flex-start', maxWidth: '86%', textAlign: m.me ? 'right' : 'left' },
          bubble: {
            display: 'inline-block', padding: '8px 12px',
            borderRadius: m.me ? '10px 10px 2px 10px' : '10px 10px 10px 2px',
            background: m.me ? accent : '#F2F3F5', color: m.me ? '#fff' : '#1F2329',
            fontSize: '13px', lineHeight: '20px', textAlign: 'left',
          },
        })),
        qStats: rows.qStats,
        questionnaires: rows.questionnaires.map((r, idx) => ({
          ...r,
          status: r.off ? '已停用' : '启用中',
          cs: mk(r.off ? 'red' : 'ok'),
          toggle: r.off ? '启用' : '停用',
          rowStyle: r.off ? { background: '#FAFAFB' } : {},
          nameStyle: { fontSize: '13px', fontWeight: 600, color: r.off ? '#A6AAB0' : '#1F2329' },
          delStyle: { fontSize: '13px', cursor: r.off ? 'pointer' : 'not-allowed', color: r.off ? '#D83931' : '#BBBFC4' },
          view: () => this.goto('questionnaireDetail', '?id=' + (r.resourceId ?? idx)),
          opsGo: () => this.goto('questionnaireOps', '?id=' + (r.resourceId ?? idx)),
          copyIt: () => r.resourceId && void this.api.duplicateQuestionnaire(r.resourceId).then(() => { toast('问卷副本已创建'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '问卷复制失败', true)),
          toggleIt: () => {
            if (!r.resourceId) return toast('问卷缺少服务端 ID', true);
            const enabled = r.off;
            const run = () => void this.api.setQuestionnaireEnabled(r.resourceId!, enabled).then(() => { toast(enabled ? '问卷已启用' : '问卷已停用'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '问卷状态变更失败', true));
            if (enabled) run(); else confirmBox('停用问卷', '停用后公开定义不能继续提交。确认停用？', '确认停用', true, run);
          },
          shareIt: () => r.publicPath ? this.openShare('问卷', r.name, 'q' + idx, r.publicPath) : this.blocked('问卷尚未发布公开定义，后端未返回 public_path'),
          download: () => this.blocked('当前 OpenAPI 提供提交列表读取，但没有问卷结果文件导出 operation'),
          del: () => {
            if (!r.resourceId || !r.off) return toast('仅已停用问卷可删除', true);
            confirmBox('删除问卷', '删除会保留后端按契约允许保留的历史数据。确认删除？', '确认删除', true, () => { void this.api.deleteQuestionnaire(r.resourceId!).then(() => { toast('问卷已删除'); void this.init(); }).catch((error) => toast(error instanceof Error ? error.message : '问卷删除失败', true)); });
          },
        })),
        qSubs: rows.qSubs,
        qApply: opsLogRows.map((r) => ({ ...r, cs: mk(r.tone) })),
        edTools: rows.edTools,
        edQs: rows.edQs.map((q) => ({ ...q, isOpts: !q.input })),
        edAssignees: rows.edAssignees,
        chStats: rows.chStats,
        channels: rows.channels.map((r) => ({
          ...r,
          view: () => this.openChannelDrawer(r.resourceId),
          edit: () => this.goto('channelForm', r.resourceId == null ? '' : '?id=' + r.resourceId),
          cs: mk(r.tone), tcs: mk(r.tagTone), typeCs: mk('blue'), matCs: mk('gray'), welCs: mk('ok'),
        })),
        orders: rows.orders.map((r) => ({ ...r, cs: mk(r.tone), view: () => this.goto('orderDetail', '?id=' + encodeURIComponent(r.no)) })),
        orderKv: rows.orderKv.map((r) => ({
          ...r,
          vs: {
            fontSize: '13px', color: '#1F2329',
            fontFamily: r.mono ? 'ui-monospace,SFMono-Regular,Menlo,monospace' : 'inherit',
          },
        })),
        orderEvents: rows.orderEvents.map((r) => ({ ...r, cs: mk(r.tone) })),
        spProducts: spRows,
        products: productRows,
        coupons: couponRows,
        images: imageCards,
        mpItems: mpCards,
        attachItems: attachRows,
        agents: rows.agents.map((r) => ({ ...r, cs: mk(r.tone), typeCs: mk('gray'), matCs: mk('gray') })),
        agentSlots: rows.agentSlots,
        agentDeps: rows.agentDeps,
      },
    };
  }
}
