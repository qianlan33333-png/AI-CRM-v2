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
import type { AdminDb, AudienceSender, QuestionnaireOps, Tone } from '../shared/api/types';
import { deepCopy } from '../shared/api/mockData';
import { emptyAdminDb } from '../api/admin';
import { toast, confirmBox, busy, simulateUpload } from '../shared/ui/feedback';
import { openPicker, type PickerItem, type PickerOpts } from '../shared/ui/picker';
import { copyText, renderFakeQr } from './sections/util';

const ACCENT = '#3370ff';

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
  /* ---- 分享组件 ---- */
  shareKind: string;
  shareTitle: string;
  shareUrl: string;
  shareCode: string;
  /* ---- 素材库 ---- */
  editingName: string;
  /* ---- 通用选择器草稿 ---- */
  /** 渠道表单 · 客服分配（null = 沿用种子） */
  cfStaff: PickerItem[] | null;
  /** 渠道表单 · 欢迎语素材已选 */
  cfMats: PickerItem[];
  /** 渠道表单 · 入渠标签（null = 默认 沙龙邀约/共学营） */
  cfTags: PickerItem[] | null;
  /** Agent 编辑 · 固定素材（null = 默认示例两条） */
  agMats: PickerItem[] | null;
  /** 负责人迁移 · 原/目标负责人 uid */
  migFromUid: string;
  migToUid: string;
  /** 问卷运营配置 · 绑定渠道码 code（'' = 未改） */
  opsChannelId: string;
  /** 商品/周期商品表单 · 引流渠道码 code */
  pfChannelId: string;
  spfChannelId: string;
};

/** 全部屏幕键（go 跳转表） */
const SCREENS = [
  'customers', 'customerDetail', 'questionnaires', 'questionnaireDetail', 'channels', 'channelForm',
  'orders', 'orderDetail', 'spProducts', 'coupons', 'couponForm', 'images', 'agents', 'agentEdit',
  'config', 'configDetail', 'automation', 'cycles', 'groupops', 'ai', 'funnel', 'radar', 'tags',
  'products', 'mpLib', 'attach', 'ownerMig', 'apidocs', 'productForm', 'spProductForm',
  'groupopsDetail', 'radarDetail', 'radarForm', 'aiDetail',
  'audienceEdit', 'cyclesDetail', 'questionnaireOps', 'spProductData', 'couponData',
] as const;

interface FbEl extends HTMLElement {
  __fbBusy?: boolean;
}

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
    tagGroupId: 1,
    tagMode: '',
    editingTagId: 0,
    tagQ: '',
    opsTab: 1,
    postEnabled: true,
    postType: 'channel_qr',
    pushEnabled: true,
    shareKind: '',
    shareTitle: '',
    shareUrl: '',
    shareCode: '',
    editingName: '',
    cfStaff: null,
    cfMats: [],
    cfTags: null,
    agMats: null,
    migFromUid: 'LiYou',
    migToUid: 'LinKaiYan',
    opsChannelId: '',
    pfChannelId: 'shalongyaoyue',
    spfChannelId: 'shalongyaoyue',
  };

  db: AdminDb = emptyAdminDb();

  /** 发送人白名单草稿（添加发送人未保存前的本地行） */
  private sendersDraft: AudienceSender[] | null = null;
  /** 问卷运营配置 · 自定义参数草稿 */
  private paramsDraft: { key: string; value: string }[] | null = null;

  constructor(
    private api: AdminApi,
    readonly page: string,
  ) {
    super();
  }

  /** 页面入口调用：加载数据仓库 → 重渲染 */
  async init(): Promise<void> {
    this.db = await this.api.loadDb({ page: this.page, id: this.qs().get('id') || undefined });
    // 问卷运营配置：首次进入把本地开关态同步为已保存值
    const ops = this.currentOps();
    if (ops && this.page === 'questionnaireOps') {
      this.state.postEnabled = ops.postEnabled;
      this.state.postType = ops.postType;
      this.state.pushEnabled = ops.pushEnabled;
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
    if (el) renderFakeQr(el, code);
  }

  copyShareLink(): void {
    copyText(this.state.shareUrl, toast);
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

  private saveAudienceBasic(): void {
    const pkg = this.audiencePkg();
    if (!pkg) return;
    const val = (id: string): string => (document.getElementById(id) as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null)?.value || '';
    void this.api
      .saveAudiencePackage({
        id: pkg.id,
        name: val('aeName') || pkg.name,
        definition: val('aeDef'),
        groupId: Number(val('aeGroup') || String(pkg.groupId)),
        incremental: val('aeInc') || pkg.incremental,
        daily: val('aeDaily') || pkg.daily,
      })
      .then(() => {
        toast('基础配置已保存');
        void this.init();
      });
  }

  private bindAutomation(name: string): void {
    const pkg = this.audiencePkg();
    if (!pkg) return;
    void this.api.saveAudiencePackage({ id: pkg.id, boundAutomation: name }).then(() => {
      toast('已绑定自动化话术「' + name + '」');
      void this.init();
    });
  }

  private unbindAutomation(): void {
    const pkg = this.audiencePkg();
    if (!pkg || !pkg.boundAutomation) return;
    confirmBox('解除绑定', '解除后该人群包将停止自动化触达，确认继续？', '解除绑定', true, () => {
      void this.api.saveAudiencePackage({ id: pkg.id, boundAutomation: '' }).then(() => {
        toast('已解除绑定');
        void this.init();
      });
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
    this.sendersDraft = null;
    toast('发送人白名单已保存');
    this.setState({});
  }

  /* ================= 通用选择器接入 ================= */

  private pick(opts: PickerOpts): Promise<PickerItem[] | null> {
    return openPicker(this.api, opts);
  }

  private channelName(code: string): string {
    return this.db.rows.channels.find((c) => c.code === code)?.name || '不配置引流渠道码';
  }

  private staffName(uid: string): string {
    return this.db.staff.find((s) => s.uid === uid)?.name || uid;
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

  /* ---- 负责人迁移 ---- */
  private migPick(which: 'from' | 'to'): void {
    void this.pick({ kind: 'members', multi: false, max: 1, title: which === 'from' ? '选择原负责人' : '选择目标负责人' }).then((r) => {
      if (!r || !r.length) return;
      this.setState(which === 'from' ? { migFromUid: r[0].id } : { migToUid: r[0].id });
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
    next.postEnabled = this.state.postEnabled;
    next.postType = this.state.postType;
    next.pushEnabled = this.state.pushEnabled;
    if (this.state.opsTab === 1) {
      next.channelId = this.state.opsChannelId ? this.channelName(this.state.opsChannelId) : next.channelId;
      next.qrTitle = this.opsInputVal('opsQrTitle');
      next.qrSubtitle = this.opsInputVal('opsQrSub');
      next.redirectType = (this.opsInputVal('opsRedirectType') as 'h5' | 'urllink') || next.redirectType;
      next.redirectUrl = this.opsInputVal('opsRedirectUrl');
    } else {
      next.webhookUrl = this.opsInputVal('opsWebhook');
      next.subscribeType = this.opsInputVal('opsSubType') || next.subscribeType;
      next.expiresAt = this.opsInputVal('opsExpire');
      next.serviceCycle = this.opsInputVal('opsCycle');
      next.frequency = this.opsInputVal('opsFreq') || next.frequency;
      next.remark = this.opsInputVal('opsRemark');
      // 自定义参数：按 DOM 行收集
      const rows = Array.from(document.querySelectorAll('#opsParams .ops-param-row'));
      if (rows.length) {
        next.customParams = rows.map((r) => {
          const inputs = r.querySelectorAll('input');
          return { key: (inputs[0]?.value || '').trim(), value: (inputs[1]?.value || '').trim() };
        }).filter((p) => p.key);
      }
    }
    void this.api.saveQuestionnaireOps(this.currentQid(), next).then(() => {
      toast('运营配置已保存');
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
    });
  }

  private checkConfig(): void {
    const cat = this.currentConfigCat();
    if (!cat) return;
    void this.api.checkConfigCategory(cat.key).then((msg) => toast(msg, msg.startsWith('检查发现')));
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
      broadcast: (ev: Event) => {
        confirmBox('确认群发', '将向「' + p.name + '」内 ' + p.count.toLocaleString() + ' 人发送已绑定的话术，确认继续？', '确认群发', false, () => {
          busy(ev.currentTarget as FbEl, 700, () => toast('群发任务已创建'));
        });
      },
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
      act: (ev: Event) => busy(ev.currentTarget as FbEl, 600, () => toast('复盘会话已创建，请在复盘面板中填写结论')),
    }));
    const run = this.db.cycleRuns[this.pageId()] || this.db.cycleRuns[1];
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
    const qRow = rows.questionnaires[qid];
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
      saveQr: () => toast('二维码已保存到下载目录'),
      close: () => this.closeModal(),
    };

    /* ================= 素材库 ================= */
    const imageCards = rows.images.map((m) => ({
      ...m,
      cs: mk(m.tone),
      thumb: { height: '104px', background: m.bg, borderBottom: '1px solid #EFF0F1', cursor: 'pointer' } as StyleObj,
      off: m.enabled ? {} : { opacity: '0.55' } as StyleObj,
      open: () => this.setState({ modal: 'imgEdit', editingName: m.name }),
    }));
    const editingImg = imageCards.find((x) => x.name === s.editingName);
    const mpCards = rows.mpItems.map((m) => ({
      ...m,
      statusColor: m.thumbOk ? '#2EA121' : '#D97917',
      off: m.enabled ? {} : { opacity: '0.55' } as StyleObj,
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
      edit: () => this.goto('productForm'),
      shareIt: () => this.openShare('商品', p.name, p.code.toLowerCase()),
      toggle: () => {
        toast('后端能力未就绪：商品读取 DTO 未返回 lifecycle，无法安全决定启用或停用', true);
      },
      toggleText: p.status === '已上架' ? '下架' : '上架',
    }));
    const spRows = rows.spProducts.map((p, idx) => ({
      ...p,
      cs: mk(p.tone),
      edit: () => this.goto('spProductForm'),
      data: () => this.goto('spProductData', '?id=' + idx),
      shareIt: () => this.openShare('周期商品', p.name, p.code.toLowerCase()),
      toggle: () => {
        toast('后端能力未就绪：周期商品页面尚未绑定 lifecycle/version，未发送请求', true);
      },
      toggleText: p.status === '已上架' ? '下架' : '上架',
    }));
    const couponRows = rows.coupons.map((r, idx) => ({
      ...r,
      cs: mk(r.tone),
      data: () => this.goto('couponData', '?id=' + idx),
      shareIt: () => {
        if (this.api.mode === 'mock') return this.openShare('优惠券', r.name, r.code, `/c/c-${r.resourceId || idx + 1}`);
        if (!r.resourceId) return toast('优惠券缺少服务端资源 ID，无法读取分享地址', true);
        void this.api.getCouponSharePath(r.resourceId).then((path) => this.openShare('优惠券', r.name, r.code, path)).catch((error) => toast(error instanceof Error ? error.message : '分享地址读取失败', true));
      },
    }));
    const couponIdx = this.pageId();
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
        });
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
            });
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

    return {
      go,

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
      mig: {
        fromName: this.staffName(s.migFromUid),
        toName: this.staffName(s.migToUid),
        pickFrom: () => this.migPick('from'),
        pickTo: () => this.migPick('to'),
      },
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
        senders: aeSenders,
        members: aeMembers,
        memberTotal: aeMembers.length + ' 人（共 ' + (aePkg?.countText || '0') + ' 人，显示前 200）',
        records: aeRecordRows,
        recordTotal: aeRecords.length ? '共 ' + aeRecords.length + ' 条' : '暂无发送记录',
        agents: aeAgents,
        saveBasic: () => this.saveAudienceBasic(),
        unbind: () => this.unbindAutomation(),
        addSender: () => this.addSender(),
        saveSenders: () => this.saveSenders(),
        back: () => this.goto('automation'),
        refresh: (ev: Event) => busy(ev.currentTarget as FbEl, 600, () => toast('已刷新')),
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
        testPush: () => toast('后端能力未就绪：当前表单没有 OpenAPI 要求的 opaque configuration reference，未发送请求', true),
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
              toast('已与企微同步，共 ' + tagCapacity + ' 个标签');
              void this.init();
            }),
          );
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
        replaceImage: () => simulateUpload('替换图片'),
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
        resolve: (ev: Event) => busy(ev.currentTarget as FbEl, 800, () => toast('缩略图缓存已刷新')),
        pickThumb: () => simulateUpload('缩略图'),
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
        editConfig: () => this.goto('couponForm'),
        back: () => this.goto('coupons'),
        shareIt: () => couponRows[couponIdx]?.shareIt(),
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
          shareIt: () => this.openShare('问卷', r.name, 'q' + idx, r.publicPath),
        })),
        qSubs: rows.qSubs,
        qApply: rows.qApply.map((r) => ({ ...r, cs: mk(r.tone) })),
        edTools: rows.edTools,
        edQs: rows.edQs.map((q) => ({ ...q, isOpts: !q.input })),
        edAssignees: rows.edAssignees,
        chStats: rows.chStats,
        channels: rows.channels.map((r) => ({
          ...r,
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
