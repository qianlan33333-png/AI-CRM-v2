/**
 * AI-CRM 领域模型类型定义
 * 覆盖后台三端（admin / sidebar / h5）共用的数据结构。
 * 字段命名与后端现有接口保持语义对应（external_userid / unionid 等）。
 */

/* ================= 内容雷达（字段与生产 API 对齐） ================= */

export type RadarType = 'link' | 'image' | 'pdf';
export type PdfStatus = 'pending' | 'processing' | 'ready' | 'failed';

export interface RadarLink {
  id: number;
  title: string;
  target_type: RadarType;
  /** 外部链接地址（target_type=link） */
  original_url: string;
  /** 素材文件名快照（image/pdf） */
  file_name_snapshot: string;
  /** 素材库条目 id（image/pdf） */
  media_item_id: string;
  enabled: boolean;
  auth_required: boolean;
  /** 创建人 userid */
  staff_id: string;
  /** 分享短码 */
  code: string;
  /** PV · 中转页到达 */
  total_landings: number;
  /** UV · 授权用户 */
  authorized_users: number;
  /** 查看次数 */
  view_count: number;
  last_viewed_at: string;
  pdf_processing_status?: PdfStatus;
  pdf_page_count?: number;
}

export interface RadarEvent {
  unionid_masked: string;
  external_userid: string;
  created_at: string;
}

export interface RadarMedia {
  id?: number;
  name: string;
  meta: string;
}

export interface RadarLinkInput {
  id?: number;
  title: string;
  target_type: RadarType;
  original_url: string;
  file_name_snapshot: string;
  media_item_id: string;
  enabled: boolean;
  auth_required: boolean;
}

/* ================= AI 助手（字段与生产对齐） ================= */

export type AiPlanStatus = 'pending_review' | 'approved' | 'rejected' | 'active';
export type AiRecipientStatus = 'pending' | 'approved' | 'rejected' | 'sent' | 'failed';
export type Tone = 'ok' | 'blue' | 'warn' | 'red' | 'gray' | 'purple';

export interface AiPlan {
  id: number;
  name: string;
  /** 计划编号 */
  code: string;
  /** 发送人 userid */
  owner: string;
  /** 创建人 userid */
  creator: string;
  updated: string;
  /** 目标人数 */
  target: number;
  status: AiPlanStatus;
}

export interface AiTask {
  no: number;
  kind: string;
  text: string;
  media: string[];
  /** 审阅备注（仅审阅可见） */
  note: string;
}

export interface AiRecipient {
  id: number;
  name: string;
  external_userid: string;
  owner: string;
  updated: string;
  taskCount: number;
  status: AiRecipientStatus;
  tasks: AiTask[];
}

/* ================= 漏斗 / 多维数据看板 ================= */

export type FunnelFieldType = 'text' | 'enum' | 'bool' | 'number' | 'date';

export interface FunnelField {
  key: string;
  title: string;
  type: FunnelFieldType;
  w: number;
  /** 冻结列序号（0/1/2） */
  frozen?: number;
}

/** 网格行：key → 值（bool 列用 '✓'/'✗' 存储，与生产 Tabulator 导出一致） */
export type FunnelGridRow = Record<string, string | number>;

export interface FunnelFilter {
  field: string;
  op: string;
  value: string;
}

export interface FunnelSort {
  field: string;
  dir: 'asc' | 'desc';
}

export interface FunnelView {
  name: string;
  filters: FunnelFilter[];
  group: string;
  sort: FunnelSort | null;
}

/* ================= 自动化运营 · 人群包（与生产 ai_audience_ops 对齐） ================= */

export interface AudienceGroup {
  id: number;
  name: string;
}

export interface AudiencePackage {
  id: number;
  name: string;
  /** 所属分组（0 = 未分组） */
  groupId: number;
  count: number;
  lastRefresh: string;
  /** 刷新方式文案：每日 2:00 / 每 3 分钟 / 手动 */
  refreshMode: string;
  running: boolean;
  /** 版本标记：历史配置 / v3 等 */
  version: string;
  /** 筛选逻辑简述 */
  definition: string;
  /** 增量刷新 select 值 */
  incremental: string;
  /** 每日快照 select 值 */
  daily: string;
  /** 绑定的自动化话术名称（'' = 未绑定） */
  boundAutomation: string;
}

export interface AudienceMember {
  name: string;
  external_userid: string;
  joinedAt: string;
}

export interface AudienceSender {
  priority: number;
  userid: string;
  rule: string;
  status: string;
}

export interface AudienceSendRecord {
  name: string;
  external_userid: string;
  source: string;
  status: string;
  tone: Tone;
  sentAt: string;
  failReason: string;
}

/* ================= 运营闭环 · 单次运行档案（与生产 operation_cycles_run 对齐） ================= */

export interface CycleAttemptStage {
  label: string;
  status: string;
}

export interface CycleAttempt {
  label: string;
  statusLabel: string;
  tone: Tone;
  summary: string;
  startedAt: string;
  finishedAt: string;
  stages: CycleAttemptStage[];
}

export interface CycleWindowMetric {
  label: string;
  value: string;
  desc: string;
}

export interface CycleWindow {
  label: string;
  statusLabel: string;
  tone: Tone;
  metrics: CycleWindowMetric[];
  start: string;
  end: string;
  quality: string;
  limitation: string;
}

/** 运行档案（8 个章节 + 证据索引） */
export interface CycleRun {
  id: number;
  label: string;
  objective: string;
  strategy: string;
  runKey: string;
  snapshotRev: string;
  audience: string;
  intendedSendAt: string;
  planScheduledFor: string;
  firstSentAt: string;
  lastSentAt: string;
  attempts: CycleAttempt[];
  /** 人群分层漏斗 */
  funnel: { label: string; value: string }[];
  audienceNote: string;
  reviewStatus: string;
  reviewTone: Tone;
  planVersion: string;
  planStatus: string;
  planSource: string;
  targetCount: string;
  delivery: {
    sent: string;
    failed: string;
    retryable: string;
    rate: string;
    statusLabel: string;
    source: string;
    failureSummary: string;
  };
  windows: CycleWindow[];
  retro: { summary: string; detail: string; findings: string[]; limitations: string[] };
  next: {
    statusLabel: string;
    tone: Tone;
    summary: string;
    rationale: string;
    confirmedAt: string;
    appliedVersion: string;
    note: string;
    changes: string[];
  };
  references: { label: string; desc: string }[];
}

/** 运营闭环任务列表行 */
export interface CycleTask {
  id: number;
  name: string;
  cron: string;
  dot: string;
  steps: { label: string; color: string; dim: boolean }[];
  action: string;
  runId: number;
}

/* ================= 问卷 · 运营配置（/admin/questionnaires/{id}/operations） ================= */

export interface QuestionnaireOps {
  postEnabled: boolean;
  /** channel_qr 展示渠道二维码 / redirect 直接跳转 */
  postType: 'channel_qr' | 'redirect';
  channelId: string;
  qrTitle: string;
  qrSubtitle: string;
  /** h5 H5 跳转地址 / urllink 动态 URL Link 接口 */
  redirectType: 'h5' | 'urllink';
  redirectUrl: string;
  pushEnabled: boolean;
  webhookUrl: string;
  subscribeType: string;
  expiresAt: string;
  serviceCycle: string;
  frequency: string;
  remark: string;
  customParams: { key: string; value: string }[];
}

/* ================= 企微标签管理 ================= */

export interface TagGroup {
  id: number;
  name: string;
}

export interface WecomTag {
  id: number;
  groupId: number;
  name: string;
  users: number;
  syncedAt: string;
}

/* ================= 列表页数据 ================= */

export interface Customer {
  name: string;
  id: string;
  owner: string;
  mobile: string;
}

export interface Tag {
  name: string;
}

export interface QaPair {
  q: string;
  a: string;
}

export interface Msg {
  who: string;
  time: string;
  text: string;
  me: boolean;
}

export interface Stat {
  label: string;
  value: string;
  unit: string;
}

export interface Questionnaire {
  /** OpenAPI questionnaire id; only used for real detail navigation. */
  resourceId?: number;
  name: string;
  assess: boolean;
  off: boolean;
  action: string;
  created: string;
  count: string;
}

export interface QSub {
  time: string;
  uid: string;
  by: string;
  score: string;
  tags: string[];
}

export interface QApply {
  time: string;
  sid: string;
  uid: string;
  status: string;
  tone: Tone;
  err: string;
}

export interface EdTool {
  m: string;
  t: string;
  d: string;
}

export interface EdQuestion {
  tag: string;
  title: string;
  ph: string;
  input: boolean;
  opts: string[];
}

export interface EdAssignee {
  name: string;
  uid: string;
  ratio: string;
}

export interface Channel {
  /** OpenAPI channel id; only used for real detail navigation. */
  resourceId?: number;
  name: string;
  /** 渠道编码（选渠道码组件行内展示） */
  code: string;
  type: string;
  status: string;
  tone: Tone;
  mat: string;
  tag: string;
  tagTone: Tone;
  users: string;
  qr: string;
}

/** 企微员工目录条目（通用选择器 · 选客服人员） */
export interface StaffMember {
  name: string;
  uid: string;
  dept: string;
}

/** 客户群条目（通用选择器 · 选择群聊） */
export interface GroupChat {
  name: string;
  /** 剩余可进人数 */
  left: number;
  size: number;
}

export interface Order {
  time: string;
  no: string;
  plat: string;
  payer: string;
  uid: string;
  product: string;
  amount: string;
  status: string;
  tone: Tone;
  pay: string;
}

export interface Kv {
  k: string;
  v: string;
  mono: boolean;
}

export interface OrderEvent {
  time: string;
  ev: string;
  st: string;
  tone: Tone;
}

export interface Product {
  code: string;
  name: string;
  price: string;
  status: string;
  tone: Tone;
  sold: string;
  updated: string;
}

export interface SpProduct {
  code: string;
  name: string;
  price: string;
  status: string;
  tone: Tone;
  sold: string;
  updated: string;
}

export interface Coupon {
  /** OpenAPI coupon id; only used for real detail navigation. */
  resourceId?: number;
  name: string;
  /** 分享短码（生成分享链接 / 二维码） */
  code: string;
  off: string;
  scope: string;
  window: string;
  issue: string;
  status: string;
  tone: Tone;
}

/** 优惠券领取与使用明细（/admin/coupons/{id}/data） */
export interface CouponClaim {
  user: string;
  status: string;
  tone: Tone;
  claimedAt: string;
  validWindow: string;
  product: string;
  orderNo: string;
  usedAt: string;
}

export interface ImageItem {
  resourceId?: string;
  file?: File;
  name: string;
  size: string;
  tag: string;
  tone: Tone;
  bg: string;
  /** 描述（编辑弹窗） */
  desc: string;
  /** 标签组（逗号分隔展示） */
  tags: string;
  enabled: boolean;
  uploadedAt: string;
}

/** 小程序素材库条目 */
export interface MpItem {
  resourceId?: number;
  name: string;
  appid: string;
  pagepath: string;
  cardTitle: string;
  /** 企微缩略图缓存状态文案 */
  thumbStatus: string;
  thumbOk: boolean;
  enabled: boolean;
  bg: string;
}

/** 附件素材库条目 */
export interface AttachItem {
  resourceId?: string;
  file?: File;
  name: string;
  type: string;
  size: string;
  tags: string;
  uploadedAt: string;
  enabled: boolean;
}

export interface Agent {
  name: string;
  code: string;
  type: string;
  material: string;
  status: string;
  tone: Tone;
}

export interface Label {
  label: string;
}

export interface DepItem {
  t: string;
}

/** 配置字段（与生产 config_category_detail 渲染对齐） */
export interface ConfigFieldDef {
  /** 环境变量 key，如 WECOM_CORP_ID */
  key: string;
  /** 展示名（默认即 key） */
  label: string;
  kind: 'text' | 'secret' | 'switch' | 'number' | 'textarea' | 'readonly';
  value: string;
  /** switch 态 */
  on?: boolean;
  /** secret 态：是否已配置（决定占位文案 已设置/未设置） */
  configured?: boolean;
}

export interface ConfigBlock {
  title: string;
  fields: ConfigFieldDef[];
}

/** 配置类目（生产 platform/admin_config/category_registry.py 的 10 个类目） */
export interface ConfigCategory {
  key: string;
  label: string;
  group: string;
  on: boolean;
  /** 是否允许前台切换生效开关 */
  toggleable: boolean;
  checkSupported: boolean;
  blocks: ConfigBlock[];
}

/** 各列表页的静态数据集（对应原型 rows 对象） */
export interface RowsData {
  customers: Customer[];
  tags: Tag[];
  qa: QaPair[];
  msgs: Msg[];
  qStats: Stat[];
  questionnaires: Questionnaire[];
  qSubs: QSub[];
  qApply: QApply[];
  edTools: EdTool[];
  edQs: EdQuestion[];
  edAssignees: EdAssignee[];
  chStats: Stat[];
  channels: Channel[];
  orders: Order[];
  orderKv: Kv[];
  orderEvents: OrderEvent[];
  spProducts: SpProduct[];
  products: Product[];
  coupons: Coupon[];
  images: ImageItem[];
  mpItems: MpItem[];
  attachItems: AttachItem[];
  agents: Agent[];
  agentSlots: Label[];
  agentDeps: DepItem[];
}

/** 后台聚合数据仓库（mock 持久化单元） */
export interface AdminDb {
  radarLinks: RadarLink[];
  radarEvents: RadarEvent[];
  aiPlans: AiPlan[];
  /** 计划 id → 该计划目标人员列表 */
  aiRcs: Record<number, AiRecipient[]>;
  funnelRows: FunnelGridRow[];
  funnelViews: FunnelView[];
  /* ---- 自动化运营 · 人群包 ---- */
  audienceGroups: AudienceGroup[];
  audiencePackages: AudiencePackage[];
  /** 人群包 id → 成员列表 */
  audienceMembers: Record<number, AudienceMember[]>;
  /** 人群包 id → 发送人白名单 */
  audienceSenders: Record<number, AudienceSender[]>;
  /** 人群包 id → 发送记录 */
  audienceRecords: Record<number, AudienceSendRecord[]>;
  /* ---- 运营闭环 ---- */
  cycleTasks: CycleTask[];
  /** 运行 id → 单次运行档案 */
  cycleRuns: Record<number, CycleRun>;
  /* ---- 问卷运营配置（问卷序号 → 配置） ---- */
  qOps: Record<number, QuestionnaireOps>;
  /* ---- 企微标签 ---- */
  tagGroups: TagGroup[];
  wecomTags: WecomTag[];
  /* ---- 优惠券数据页（优惠券序号 → 领取明细） ---- */
  couponClaims: Record<number, CouponClaim[]>;
  /* ---- 配置中心（10 类目全配置点） ---- */
  configCategories: ConfigCategory[];
  /* ---- 通用选择器数据源（企微员工目录 / 客户群） ---- */
  staff: StaffMember[];
  groupChats: GroupChat[];
  rows: RowsData;
}

/* ================= 用户端 H5 ================= */

export interface H5Option {
  text: string;
  on: boolean;
  kind?: 'box' | 'dot';
}

export interface H5Dim {
  name: string;
  score: number;
  max: number;
  desc: string;
  tone: 'ok' | 'warn' | 'accent';
}

export interface H5Data {
  single: H5Option[];
  multi: H5Option[];
  step: H5Option[];
  blank: H5Option[];
  dims: H5Dim[];
}
