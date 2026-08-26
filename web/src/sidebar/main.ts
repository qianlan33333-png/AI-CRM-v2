/**
 * 企微侧边栏入口。
 *
 * 这里仅消费当前 Go OpenAPI 的 sidebar V2 契约。没有上下文或真实读取失败时，
 * 页面保持失败/待授权状态，不回退到示例数据或静态成功文案。
 */
import { sidebarApi } from "../api/sidebar";
import type {
  SidebarAgentConfigSignature,
  SidebarChatActivityResponse,
  SidebarMaterialResponse,
  SidebarOrderResponse,
  SidebarPeriodicOrderResponse,
  SidebarPeriodicRemarkResponse,
  SidebarPhoneBindingResponse,
  SidebarProfileUpdateResponse,
  SidebarProfileUpdateSafety,
  SidebarQuestionnaireResponse,
  SidebarServicePeriodMember,
  SidebarSafety,
  SidebarTimelineResponse,
  SidebarWorkbenchResponse,
  UpdateSidebarProfileBodyPatch,
} from "../api/generated/health";
import { initFeedback } from "../shared/ui/feedback";

const SDK_TIMEOUT_MS = 5000;
const PROFILE_SAVE_DEBOUNCE_MS = 520;

export const PROFILE_FIELDS = [
  "source",
  "industry",
  "description",
  "needs",
  "pain_points",
] as const;
export type ProfileField = (typeof PROFILE_FIELDS)[number];

const PROFILE_LABELS: Record<ProfileField, string> = {
  source: "用户来源",
  industry: "行业信息",
  description: "行业具体描述",
  needs: "需求",
  pain_points: "卡点与跟进状态",
};

type BoundSidebarApi = Pick<
  typeof sidebarApi,
  | "mintContext"
  | "agentConfig"
  | "oauthStartUrl"
  | "oauthCallbackUrl"
  | "workbench"
  | "timeline"
  | "chatActivity"
  | "profile"
  | "bindPhone"
  | "questionnaires"
  | "orders"
  | "periodicOrders"
  | "updateRemark"
  | "materials"
  | "thumbnailPreview"
>;

interface SidebarWx {
  agentConfig(options: {
    corpid: string;
    agentid: string;
    timestamp: number;
    nonceStr: string;
    signature: string;
    jsApiList: string[];
    success?: (result?: Record<string, unknown>) => void;
    fail?: (result?: Record<string, unknown>) => void;
  }): void;
  invoke(
    method: string,
    payload: Record<string, unknown>,
    callback: (result?: Record<string, unknown>) => void,
  ): void;
}

declare global {
  interface Window {
    wx?: SidebarWx;
  }
}

type ReceiptStep = {
  key: "accepted" | "queued" | "outcome_unknown";
  label: string;
};

type SidebarTab =
  | "profile"
  | "questionnaires"
  | "timeline"
  | "chat_activity"
  | "orders"
  | "periodic_orders"
  | "materials";

function isSidebarTab(value: string | undefined): value is SidebarTab {
  return (
    value === "profile" ||
    value === "questionnaires" ||
    value === "timeline" ||
    value === "chat_activity" ||
    value === "orders" ||
    value === "periodic_orders" ||
    value === "materials"
  );
}

/**
 * Convert the profile write safety flags into a truthful local receipt sequence.
 * The API never claims that a WeCom effect has completed; an external effect with
 * no provider receipt remains outcome_unknown.
 */
export function profileReceiptSteps(
  safety: Pick<
    SidebarProfileUpdateSafety,
    "effect_queued" | "provider_execution_eligible"
  >,
): ReceiptStep[] {
  const steps: ReceiptStep[] = [
    { key: "accepted", label: "accepted · 本地已受理" },
  ];
  if (safety.effect_queued) {
    steps.push({ key: "queued", label: "queued · 异步效果已登记" });
    steps.push({
      key: "outcome_unknown",
      label: "outcome_unknown · 尚未收到企微回执",
    });
  }
  return steps;
}

function createElement<K extends keyof HTMLElementTagNameMap>(
  doc: Document,
  tag: K,
  className?: string,
  text?: string,
): HTMLElementTagNameMap[K] {
  const element = doc.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

function markBound(element: HTMLElement): void {
  (element as HTMLElement & { __dcBound?: boolean }).__dcBound = true;
}

function errorStatus(error: unknown): number | undefined {
  const status = (error as { status?: unknown } | null)?.status;
  return typeof status === "number" ? status : undefined;
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

function isProfileField(value: string | undefined): value is ProfileField {
  return (
    value !== undefined && (PROFILE_FIELDS as readonly string[]).includes(value)
  );
}

function firstString(
  value: Record<string, unknown> | undefined,
  keys: string[],
): string {
  if (!value) return "";
  for (const key of keys) {
    const candidate = value[key];
    if (typeof candidate === "string" && candidate.trim())
      return candidate.trim();
  }
  return "";
}

type ChatType = "all" | "private" | "group";
type MaterialFilter = "q" | "category" | "tags";
type ThumbnailStatus = "pending" | "ready" | "not_found" | "error";

function validateSidebarSafety(safety: SidebarSafety, label: string): void {
  if (
    !safety ||
    safety.local_only !== true ||
    safety.provider_execution_eligible !== false ||
    safety.real_external_call_executed !== false
  ) {
    throw new Error(`${label}安全声明不完整，已停止渲染。`);
  }
}

function withTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  message: string,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(message)), timeoutMs);
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve(value);
      },
      (error: unknown) => {
        clearTimeout(timer);
        reject(error);
      },
    );
  });
}

export class SidebarController {
  private readonly content: HTMLElement;
  private readonly tabs: HTMLElement;
  private readonly contextStatus: HTMLElement | null;
  private readonly sdkStatus: HTMLElement | null;
  private readonly customerName: HTMLElement | null;
  private readonly customerMeta: HTMLElement | null;
  private readonly externalUserid: HTMLElement | null;
  private readonly workflowTitle: HTMLElement | null;
  private readonly bindingState: HTMLElement | null;
  private eventsBound = false;
  private profileSaveTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly pendingProfileFields = new Set<ProfileField>();
  private savingProfile = false;
  private saveAgain = false;
  private externalUserId = "";
  private contextToken = "";
  private workbench: SidebarWorkbenchResponse | null = null;
  private activeTab: SidebarTab = "profile";
  private questionnaires: SidebarQuestionnaireResponse | null = null;
  private questionnaireLoading = false;
  private questionnaireError: unknown = null;
  private questionnaireRequestVersion = 0;
  private timeline: SidebarTimelineResponse | null = null;
  private timelineLoading = false;
  private timelineError: unknown = null;
  private timelineRequestVersion = 0;
  private chatActivity: SidebarChatActivityResponse | null = null;
  private chatActivityType: ChatType = "all";
  private chatActivityLoading = false;
  private chatActivityError: unknown = null;
  private chatActivityRequestVersion = 0;
  private orders: SidebarOrderResponse | null = null;
  private ordersLoading = false;
  private ordersError: unknown = null;
  private ordersRequestVersion = 0;
  private periodicOrders: SidebarPeriodicOrderResponse | null = null;
  private periodicOrdersLoading = false;
  private periodicOrdersError: unknown = null;
  private periodicOrdersRequestVersion = 0;
  private readonly periodicRemarkDrafts = new Map<string, string>();
  private readonly periodicRemarkStatuses = new Map<
    string,
    { message: string; failed: boolean }
  >();
  private readonly periodicRemarkSaving = new Set<string>();
  private materials: SidebarMaterialResponse | null = null;
  private materialFilters: Record<MaterialFilter, string> = {
    q: "",
    category: "",
    tags: "",
  };
  private materialsLoading = false;
  private materialsError: unknown = null;
  private materialsRequestVersion = 0;
  private readonly thumbnailStatuses = new Map<number, ThumbnailStatus>();
  private readonly thumbnailURLs = new Map<number, string>();
  private phoneBindingLoading = false;
  private phoneBindingMessage = "";

  constructor(
    private readonly api: BoundSidebarApi = sidebarApi,
    private readonly doc: Document = document,
  ) {
    const content = doc.getElementById("content");
    const tabs = doc.getElementById("tabs");
    if (!content || !tabs)
      throw new Error("Sidebar 页面缺少 content 或 tabs 容器");
    this.content = content;
    this.tabs = tabs;
    this.contextStatus = doc.getElementById("sidebar-context-status");
    this.sdkStatus = doc.getElementById("sidebar-jssdk-status");
    this.customerName = doc.getElementById("customer-name");
    this.customerMeta = doc.getElementById("customer-mobile");
    this.externalUserid = doc.getElementById("customer-external-userid");
    this.workflowTitle = doc.getElementById("workflow-title");
    this.bindingState = doc.getElementById("binding-state");
  }

  async boot(): Promise<void> {
    this.bindEvents();
    this.renderTabs(false);
    const callbackHandled = await this.handleOAuthCallback();
    if (callbackHandled) return;
    await this.initialize();
  }

  private bindEvents(): void {
    if (this.eventsBound) return;
    this.eventsBound = true;
    this.tabs.addEventListener("click", (event) => {
      const button = (event.target as HTMLElement).closest<HTMLButtonElement>(
        "[data-sidebar-tab]",
      );
      if (!button || button.disabled) return;
      const tab = button.dataset.sidebarTab;
      if (isSidebarTab(tab)) {
        this.activateTab(tab);
      } else {
        this.setContextStatus(
          "该板块尚未接入当前 OpenAPI，已安全关闭。",
          "warn",
        );
      }
    });
    this.content.addEventListener("input", (event) => {
      const target = event.target as HTMLInputElement | HTMLTextAreaElement;
      const field = target.dataset.profileField;
      if (isProfileField(field) && this.workbench) {
        this.workbench.profile[field] = target.value;
        this.pendingProfileFields.add(field);
        this.setProfileSaveStatus("待保存：停止编辑 520ms 后自动保存。");
        this.scheduleProfileSave();
        return;
      }
      const materialFilter = target.dataset.materialFilter as
        | MaterialFilter
        | undefined;
      if (materialFilter && materialFilter in this.materialFilters)
        this.materialFilters[materialFilter] = target.value;
      const memberRef = target.dataset.periodicRemark;
      if (memberRef) this.periodicRemarkDrafts.set(memberRef, target.value);
    });
    this.content.addEventListener("change", (event) => {
      const target = event.target as HTMLSelectElement;
      if (target.dataset.chatFilter !== "chat_type") return;
      const value = target.value;
      this.chatActivityType =
        value === "private" || value === "group" ? value : "all";
      this.chatActivity = null;
      this.chatActivityError = null;
      this.renderActiveContent();
      void this.loadChatActivity();
    });
    this.content.addEventListener("click", (event) => {
      const button = (event.target as HTMLElement).closest<HTMLButtonElement>(
        "[data-sidebar-action], [data-material-keyword]",
      );
      if (!button || button.disabled) return;
      const keyword = button.dataset.materialKeyword;
      if (keyword !== undefined) {
        this.materialFilters.q = keyword;
        this.renderActiveContent();
        void this.loadMaterials();
        return;
      }
      const action = button.dataset.sidebarAction;
      if (action === "retry-context") {
        button.disabled = true;
        void this.initialize();
      } else if (action === "oauth") {
        button.disabled = true;
        void this.startOAuth(button);
      } else if (action === "retry-questionnaires") {
        button.disabled = true;
        void this.loadQuestionnaires();
      } else if (action === "retry-timeline") {
        button.disabled = true;
        void this.loadTimeline();
      } else if (action === "timeline-more") {
        void this.loadTimeline(this.timeline?.next_cursor);
      } else if (action === "retry-chat-activity") {
        button.disabled = true;
        void this.loadChatActivity();
      } else if (action === "chat-activity-more") {
        void this.loadChatActivity(this.chatActivity?.next_cursor);
      } else if (action === "retry-orders") {
        button.disabled = true;
        void this.loadOrders(this.orders?.items.length || 0);
      } else if (action === "orders-more") {
        void this.loadOrders(this.orders?.items.length || 0);
      } else if (action === "retry-periodic-orders") {
        button.disabled = true;
        void this.loadPeriodicOrders(this.periodicOrders?.items.length || 0);
      } else if (action === "periodic-orders-more") {
        void this.loadPeriodicOrders(this.periodicOrders?.items.length || 0);
      } else if (action === "periodic-remark-save") {
        void this.savePeriodicRemark(button);
      } else if (action === "retry-materials") {
        button.disabled = true;
        void this.loadMaterials();
      } else if (action === "materials-search") {
        void this.loadMaterials();
      } else if (action === "materials-more") {
        const response = this.materials;
        if (response)
          void this.loadMaterials(response.offset + response.items.length);
      } else if (action === "bind-phone") {
        void this.bindPhone();
      } else if (action === "open-related-questionnaires") {
        this.activateTab("questionnaires");
      } else if (action === "open-related-orders") {
        this.activateTab("orders");
      }
    });
  }

  private async initialize(): Promise<void> {
    this.setContextStatus("正在识别当前客户并准备本地上下文…");
    this.renderTabs(false);
    const query = new URLSearchParams(
      this.doc.defaultView?.location.search || "",
    );
    let externalUserid = this.queryExternalUserid(query);

    if (externalUserid) {
      this.externalUserId = externalUserid;
      // A query-bound customer can still use the local context path when the
      // browser is outside WeCom; SDK errors remain visible instead of blocking
      // the real local API call.
      await this.prepareJssdk();
    } else {
      const sdkReady = await this.prepareJssdk();
      if (sdkReady) {
        try {
          externalUserid = await this.resolveExternalUseridFromWx();
        } catch (error) {
          this.renderContextError(
            "企微未返回当前客户 external_userid，请从企微客户侧边栏重新打开。",
            errorMessage(error, "企微上下文读取失败"),
          );
          return;
        }
      }
    }

    if (!externalUserid) {
      this.renderContextError(
        "缺少 external_userid，不能创建 Sidebar 上下文。请从企微侧边栏打开，或补充有效客户上下文。",
      );
      return;
    }

    this.externalUserId = externalUserid;
    this.setContextStatus("正在读取客户范围的本地工作台…");
    try {
      const context = await this.api.mintContext({
        external_userid: externalUserid,
      });
      if (context.state === "viewer_session_required") {
        this.renderViewerSessionRequired();
        return;
      }
      if (context.state === "customer_not_bound") {
        this.renderContextError("当前员工无权查看该客户，客户上下文未建立。");
        return;
      }
      if (context.state !== "ready" || !context.context_token) {
        this.renderContextError(
          `Sidebar 上下文不可用：${context.state || "unknown"}`,
        );
        return;
      }
      this.contextToken = context.context_token;
      const workbench = await this.api.workbench(this.contextToken);
      this.renderWorkbench(workbench);
    } catch (error) {
      const status = errorStatus(error);
      const message =
        status === 401
          ? "登录状态已失效，请重新打开 Sidebar 或完成 OAuth 授权。"
          : status === 403
            ? "当前账号无权查看该客户，Sidebar 已安全关闭。"
            : errorMessage(error, "Sidebar 工作台读取失败");
      this.renderContextError(message);
    }
  }

  private queryExternalUserid(query: URLSearchParams): string {
    for (const key of ["external_userid", "externalUserid", "externalUserId"]) {
      const value = query.get(key)?.trim();
      if (value) return value;
    }
    return "";
  }

  private currentPageUrl(): string {
    const view = this.doc.defaultView;
    if (!view) return "";
    return view.location.href.split("#", 1)[0];
  }

  private nextPath(): string {
    const view = this.doc.defaultView;
    if (!view) return "/sidebar/index.html";
    const url = new URL(view.location.href);
    url.searchParams.delete("code");
    url.searchParams.delete("state");
    const query = url.searchParams.toString();
    return url.pathname + (query ? `?${query}` : "");
  }

  private async prepareJssdk(): Promise<boolean> {
    const view = this.doc.defaultView;
    const wx = view?.wx;
    if (!wx) {
      this.setSdkStatus("unavailable", "企微 SDK 不可用");
      this.setContextStatus(
        "企微 SDK 不可用：请从企微侧边栏打开；已有 external_userid 时仍会尝试读取本地工作台。",
        "warn",
      );
      return false;
    }
    const url = this.currentPageUrl();
    if (!url || url.length > 4096) {
      this.setSdkStatus("error", "JSSDK URL 无效");
      this.setContextStatus("JSSDK 配置读取失败：当前页面 URL 无效。", "error");
      return false;
    }
    this.setSdkStatus("loading", "读取 JSSDK…");
    try {
      const config = await withTimeout(
        this.api.agentConfig(url),
        SDK_TIMEOUT_MS,
        "JSSDK 配置读取超时，请重试。",
      );
      this.validateAgentConfig(config);
      await withTimeout(
        this.configureAgent(wx, config),
        SDK_TIMEOUT_MS,
        "企微 JSSDK 初始化超时，请从企微侧边栏重试。",
      );
      this.setSdkStatus("ready", "JSSDK 就绪");
      return true;
    } catch (error) {
      this.setSdkStatus("error", "JSSDK 配置失败");
      this.setContextStatus(
        `JSSDK 配置读取失败：${errorMessage(error, "请确认企微配置后重试。")}`,
        "error",
      );
      return false;
    }
  }

  private validateAgentConfig(config: SidebarAgentConfigSignature): void {
    if (
      !config ||
      config.signature_type !== "agent_config" ||
      !config.corp_id ||
      !Number.isFinite(config.agent_id) ||
      !config.nonce ||
      !Number.isFinite(config.timestamp) ||
      !config.signature ||
      !config.url
    ) {
      throw new Error("JSSDK agent_config 签名不完整。");
    }
  }

  private configureAgent(
    wx: SidebarWx,
    config: SidebarAgentConfigSignature,
  ): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      if (typeof wx.agentConfig !== "function") {
        reject(new Error("当前企微 SDK 不支持 agentConfig。"));
        return;
      }
      let settled = false;
      const finish = (error?: Error) => {
        if (settled) return;
        settled = true;
        if (error) reject(error);
        else resolve();
      };
      try {
        wx.agentConfig({
          corpid: config.corp_id,
          agentid: String(config.agent_id),
          timestamp: config.timestamp,
          nonceStr: config.nonce,
          signature: config.signature,
          jsApiList: ["getContext", "getCurExternalContact"],
          success: () => finish(),
          fail: (result) =>
            finish(
              new Error(
                `企微 agentConfig 失败：${firstString(result, ["errmsg", "err_msg", "message"]) || "未知错误"}`,
              ),
            ),
        });
      } catch (error) {
        finish(
          new Error(
            `企微 agentConfig 失败：${errorMessage(error, "未知错误")}`,
          ),
        );
      }
    });
  }

  private invokeWx(
    wx: SidebarWx,
    method: string,
    payload: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    return withTimeout(
      new Promise<Record<string, unknown>>((resolve, reject) => {
        try {
          wx.invoke(method, payload, (result) => {
            const response = result || {};
            const message = firstString(response, [
              "errmsg",
              "err_msg",
              "message",
            ]);
            if (
              message &&
              !/:ok$/i.test(message) &&
              /(fail|error)/i.test(message)
            ) {
              reject(new Error(`企微 ${method} 失败：${message}`));
              return;
            }
            resolve(response);
          });
        } catch (error) {
          reject(
            new Error(
              `企微 ${method} 失败：${errorMessage(error, "未知错误")}`,
            ),
          );
        }
      }),
      SDK_TIMEOUT_MS,
      `企微 ${method} 超时，请重试。`,
    );
  }

  private async resolveExternalUseridFromWx(): Promise<string> {
    const wx = this.doc.defaultView?.wx;
    if (!wx || typeof wx.invoke !== "function")
      throw new Error("企微 SDK 不支持上下文读取。");
    // getContext proves that the agent context was established. Its userId is
    // the employee identity and must not be mistaken for the external contact.
    await this.invokeWx(wx, "getContext", {});
    const contact = await this.invokeWx(wx, "getCurExternalContact", {});
    const externalUserid = firstString(contact, [
      "external_userid",
      "externalUserid",
      "userId",
      "user_id",
    ]);
    if (!externalUserid)
      throw new Error("企微未返回当前客户 external_userid。");
    return externalUserid;
  }

  private async handleOAuthCallback(): Promise<boolean> {
    const view = this.doc.defaultView;
    if (!view) return false;
    const query = new URLSearchParams(view.location.search);
    const code = query.get("code")?.trim() || "";
    const state = query.get("state")?.trim() || "";
    if (!code && !state) return false;
    if (!code || !state) {
      this.renderContextError("OAuth 回调参数不完整，未建立员工会话。");
      return true;
    }
    this.setContextStatus("正在接收 OAuth 回调并建立员工会话…");
    try {
      const route = this.api.oauthCallbackUrl({ code, state });
      this.setContextStatus(
        "OAuth 回调正在由服务端验证，尚未确认员工会话…",
      );
      this.navigate(route);
      return true;
    } catch (error) {
      this.renderContextError(
        `OAuth 回调失败：${errorMessage(error, "未建立员工会话。")}`,
      );
      return true;
    }
  }

  private async startOAuth(button: HTMLButtonElement): Promise<void> {
    if (!this.externalUserId) {
      this.renderContextError("缺少 external_userid，不能发起 OAuth。");
      return;
    }
    this.setContextStatus("正在发起 OAuth 回退；尚未确认员工会话…");
    try {
      const route = this.api.oauthStartUrl({
        external_userid: this.externalUserId,
        next: this.nextPath(),
      });
      this.setContextStatus(
        "OAuth 已发起，等待企微回调；未将受理状态视为授权成功。",
      );
      this.navigate(route);
    } catch (error) {
      button.disabled = false;
      this.renderContextError(
        `OAuth 发起失败：${errorMessage(error, "请稍后重试。")}`,
      );
    }
  }

  private navigate(location: string): void {
    const view = this.doc.defaultView;
    if (!view) return;
    try {
      view.location.assign(new URL(location, view.location.origin).toString());
    } catch (error) {
      this.setContextStatus(
        `OAuth 重定向失败：${errorMessage(error, "请手动重新打开 Sidebar。")}`,
        "error",
      );
    }
  }

  private renderWorkbench(workbench: SidebarWorkbenchResponse): void {
    this.validateWorkbench(workbench);
    this.workbench = workbench;
    this.activeTab = "profile";
    this.questionnaires = null;
    this.questionnaireLoading = false;
    this.questionnaireError = null;
    this.questionnaireRequestVersion += 1;
    this.timeline = null;
    this.timelineError = null;
    this.timelineLoading = false;
    this.timelineRequestVersion += 1;
    this.chatActivity = null;
    this.chatActivityType = "all";
    this.chatActivityError = null;
    this.chatActivityLoading = false;
    this.chatActivityRequestVersion += 1;
    this.orders = null;
    this.ordersError = null;
    this.ordersLoading = false;
    this.ordersRequestVersion += 1;
    this.periodicOrders = null;
    this.periodicOrdersError = null;
    this.periodicOrdersLoading = false;
    this.periodicOrdersRequestVersion += 1;
    this.periodicRemarkDrafts.clear();
    this.periodicRemarkStatuses.clear();
    this.periodicRemarkSaving.clear();
    this.materials = null;
    this.materialsError = null;
    this.materialsLoading = false;
    this.materialsRequestVersion += 1;
    this.thumbnailStatuses.clear();
    this.clearThumbnailURLs();
    this.phoneBindingLoading = false;
    this.phoneBindingMessage = "";
    this.renderTop();
    this.renderTabs(true);
    this.renderActiveContent();
    this.setContextStatus(
      "客户范围工作台已就绪：当前数据来自本地 CRM；真实企微外部效果仍需单独回执。",
    );
  }

  private validateWorkbench(workbench: SidebarWorkbenchResponse): void {
    const profile = workbench?.profile;
    if (!profile || !profile.updated_at || !workbench.safety)
      throw new Error("工作台响应不完整，已停止渲染。");
    validateSidebarSafety(workbench.safety, "工作台");
    if (
      !profile.name ||
      !Number.isInteger(profile.customer_id) ||
      !Number.isInteger(profile.owner_staff_id)
    )
      throw new Error("工作台客户档案响应不完整，已停止渲染。");
    for (const field of PROFILE_FIELDS) {
      if (typeof profile[field] !== "string")
        throw new Error("工作台画像字段响应不完整，已停止渲染。");
    }
    for (const count of [
      workbench.questionnaire_count,
      workbench.order_count,
      workbench.periodic_order_count,
      workbench.material_count,
    ]) {
      if (!Number.isInteger(count) || count < 0)
        throw new Error("工作台统计响应不完整，已停止渲染。");
    }
  }

  private renderTop(): void {
    const profile = this.workbench?.profile;
    if (!profile) return;
    if (this.customerName)
      this.customerName.textContent =
        profile.name || `客户 #${profile.customer_id}`;
    if (this.customerMeta)
      this.customerMeta.textContent = `客户 ID ${profile.customer_id} · 负责人 #${profile.owner_staff_id}`;
    if (this.externalUserid)
      this.externalUserid.textContent = this.externalUserId
        ? `外部联系人 ID ${this.externalUserId}`
        : "";
    if (this.workflowTitle)
      this.workflowTitle.textContent = "本地工作台 · 外部效果以回执为准";
    if (this.bindingState) {
      this.bindingState.textContent = "context ready";
      this.bindingState.className = "binding-state ready";
    }
    const root = this.doc.getElementById("sidebar-workbench-root");
    if (root) root.dataset.sidebarCustomerId = String(profile.customer_id);
  }

  private renderTabs(ready: boolean): void {
    const wb = this.workbench;
    const definitions = [
      ["profile", "核心画像"],
      ["questionnaires", `问卷 ${wb?.questionnaire_count ?? ""}`],
      ["timeline", "时间线"],
      ["chat_activity", "聊天活动 · V2 补充"],
      ["products", "商品"],
      ["orders", `订单 ${wb?.order_count ?? ""}`],
      ["periodic_orders", `周期订单 ${wb?.periodic_order_count ?? ""}`],
      ["coupons", "优惠券"],
      ["materials", `素材 ${wb?.material_count ?? ""}`],
      ["other_staff_messages", "其他客服聊天"],
    ] as const;
    this.tabs.replaceChildren(
      ...definitions.map(([key, label]) => {
        const button = createElement(this.doc, "button", "tab", label);
        button.type = "button";
        button.dataset.sidebarTab = key;
        const supported = isSidebarTab(key);
        button.disabled = !ready || !supported;
        if (key === this.activeTab) {
          button.classList.add("active");
        }
        if (supported && ready) {
          markBound(button);
        }
        return button;
      }),
    );
  }

  private activateTab(tab: SidebarTab): void {
    if (!this.workbench || !this.contextToken) return;
    this.activeTab = tab;
    this.renderTabs(true);
    this.renderActiveContent();
    if (tab === "questionnaires" && !this.questionnaires)
      void this.loadQuestionnaires();
    else if (tab === "timeline" && !this.timeline) void this.loadTimeline();
    else if (tab === "chat_activity" && !this.chatActivity)
      void this.loadChatActivity();
    else if (tab === "orders" && !this.orders) void this.loadOrders();
    else if (tab === "periodic_orders" && !this.periodicOrders)
      void this.loadPeriodicOrders();
    else if (tab === "materials" && !this.materials) void this.loadMaterials();
  }

  private renderActiveContent(): void {
    const workbench = this.workbench;
    if (!workbench) return;
    const panel =
      this.activeTab === "profile"
        ? this.renderProfile(workbench)
        : this.activeTab === "questionnaires"
          ? this.renderQuestionnairesPanel()
          : this.activeTab === "timeline"
            ? this.renderTimelinePanel()
            : this.activeTab === "chat_activity"
              ? this.renderChatActivityPanel()
              : this.activeTab === "orders"
                ? this.renderOrdersPanel()
                : this.activeTab === "periodic_orders"
                  ? this.renderPeriodicOrdersPanel()
                  : this.renderMaterialsPanel();
    this.content.replaceChildren(this.renderOverview(workbench), panel);
  }

  private renderOverview(workbench: SidebarWorkbenchResponse): HTMLElement {
    const panel = createElement(this.doc, "section", "sidebar-panel");
    panel.dataset.sidebarSection = "workbench-overview";
    const head = createElement(this.doc, "div", "panel-head");
    head.append(createElement(this.doc, "h2", undefined, "工作台概览"));
    head.append(createElement(this.doc, "span", "panel-meta", "V2 本地投影"));
    panel.append(head);
    const safety = createElement(
      this.doc,
      "div",
      "panel-meta",
      workbench.safety.local_only
        ? "数据来源：本地 CRM · 真实企微外呼：未执行"
        : "数据来源：受控本地投影 · 外部效果需单独核对",
    );
    safety.dataset.sidebarSafety = "local";
    panel.append(safety);
    const grid = createElement(this.doc, "div", "summary-grid");
    grid.append(
      this.summaryItem("问卷", workbench.questionnaire_count),
      this.summaryItem("订单", workbench.order_count),
      this.summaryItem("周期订单", workbench.periodic_order_count),
      this.summaryItem("素材", workbench.material_count),
    );
    panel.append(grid);
    return panel;
  }

  private summaryItem(label: string, value: number): HTMLElement {
    const item = createElement(this.doc, "div", "summary-item");
    item.dataset.summaryKey = label;
    item.append(createElement(this.doc, "div", "summary-label", label));
    item.append(createElement(this.doc, "div", "summary-value", String(value)));
    return item;
  }

  private renderProfile(workbench: SidebarWorkbenchResponse): HTMLElement {
    const panel = createElement(this.doc, "section", "sidebar-panel");
    panel.dataset.sidebarSection = "profile";
    const head = createElement(this.doc, "div", "panel-head");
    head.append(createElement(this.doc, "h2", undefined, "核心画像"));
    head.append(
      createElement(this.doc, "span", "panel-meta", "停留 520ms 自动保存"),
    );
    panel.append(head);
    const editor = createElement(this.doc, "div", "profile-editor");
    for (const field of PROFILE_FIELDS) {
      const label = createElement(this.doc, "label", "profile-field");
      label.append(
        createElement(this.doc, "span", undefined, PROFILE_LABELS[field]),
      );
      const input = createElement(this.doc, "textarea");
      input.dataset.profileField = field;
      input.name = field;
      input.rows =
        field === "description" || field === "needs" || field === "pain_points"
          ? 3
          : 2;
      input.maxLength = field === "source" || field === "industry" ? 200 : 2000;
      input.value = workbench.profile[field] || "";
      input.setAttribute("aria-label", PROFILE_LABELS[field]);
      label.append(input);
      editor.append(label);
    }
    panel.append(editor);
    const status = createElement(
      this.doc,
      "div",
      "profile-save-status",
      "修改后停留 520ms 自动保存；仅写入本地 CRM，不显示外部同步成功。",
    );
    status.id = "profile-save-status";
    status.dataset.receipt = "idle";
    panel.append(status);
    const updated = createElement(
      this.doc,
      "div",
      "panel-meta",
      `最后本地更新：${workbench.profile.updated_at}`,
    );
    updated.id = "profile-updated-at";
    panel.append(updated);
    const phone = createElement(this.doc, "div", "profile-field");
    phone.append(
      createElement(this.doc, "span", undefined, "手机号绑定（本地 Identity）"),
    );
    const phoneInput = createElement(this.doc, "input") as HTMLInputElement;
    phoneInput.id = "sidebar-phone-input";
    phoneInput.type = "tel";
    phoneInput.inputMode = "tel";
    phoneInput.placeholder = "+8613800138000";
    phoneInput.maxLength = 16;
    phoneInput.setAttribute("aria-label", "手机号（E.164）");
    phone.append(phoneInput);
    const phoneActions = createElement(this.doc, "div", "context-actions");
    const bindPhone = createElement(
      this.doc,
      "button",
      "btn primary",
      this.phoneBindingLoading ? "绑定中…" : "绑定手机号",
    ) as HTMLButtonElement;
    bindPhone.type = "button";
    bindPhone.disabled = this.phoneBindingLoading;
    bindPhone.dataset.sidebarAction = "bind-phone";
    markBound(bindPhone);
    phoneActions.append(bindPhone);
    phone.append(phoneActions);
    const phoneStatus = createElement(
      this.doc,
      "div",
      "panel-meta",
      this.phoneBindingMessage ||
        "仅绑定当前客户；不支持从其他客户强制抢占手机号。",
    );
    phoneStatus.id = "sidebar-phone-status";
    phone.append(phoneStatus);
    panel.append(phone);
    return panel;
  }

  private async bindPhone(): Promise<void> {
    if (!this.contextToken || this.phoneBindingLoading) return;
    const input = this.doc.getElementById(
      "sidebar-phone-input",
    ) as HTMLInputElement | null;
    const mobile = input?.value.trim() || "";
    if (!/^\+[1-9][0-9]{1,14}$/.test(mobile)) {
      this.phoneBindingMessage = "请输入 E.164 手机号，例如 +8613800138000。";
      this.renderActiveContent();
      return;
    }
    this.phoneBindingLoading = true;
    this.phoneBindingMessage = "正在写入本地 Identity…";
    this.renderActiveContent();
    const rerendered = this.doc.getElementById(
      "sidebar-phone-input",
    ) as HTMLInputElement | null;
    if (rerendered) rerendered.value = mobile;
    try {
      const response = await this.api.bindPhone(this.contextToken, { mobile });
      this.validatePhoneBinding(response);
      this.phoneBindingMessage =
        response.status === "bound"
          ? "手机号已绑定到当前客户（本地事实）。"
          : response.status === "already_bound"
            ? "该手机号已绑定到当前客户，无需重复操作。"
            : "该手机号已属于其他客户，本次未改动。";
    } catch (error) {
      this.phoneBindingMessage = `手机号绑定失败：${errorMessage(error, "请稍后重试。")}`;
    } finally {
      this.phoneBindingLoading = false;
      if (this.activeTab === "profile") {
        this.renderActiveContent();
        const current = this.doc.getElementById(
          "sidebar-phone-input",
        ) as HTMLInputElement | null;
        if (current) current.value = mobile;
      }
    }
  }

  private validatePhoneBinding(response: SidebarPhoneBindingResponse): void {
    if (
      !response ||
      !["bound", "already_bound", "rejected"].includes(response.status)
    )
      throw new Error("手机号绑定响应不完整，已停止更新状态。");
    validateSidebarSafety(response.safety, "手机号绑定");
  }

  private panelShell(
    section: string,
    title: string,
    meta: string,
  ): HTMLElement {
    const panel = createElement(this.doc, "section", "sidebar-panel");
    panel.dataset.sidebarSection = section;
    const head = createElement(this.doc, "div", "panel-head");
    head.append(createElement(this.doc, "h2", undefined, title));
    head.append(createElement(this.doc, "span", "panel-meta", meta));
    panel.append(head);
    return panel;
  }

  private appendSafety(panel: HTMLElement, safety: SidebarSafety): void {
    const note = createElement(
      this.doc,
      "div",
      "panel-meta",
      "数据来源：本地 CRM · 未执行企微外部调用",
    );
    note.dataset.sidebarSafety = "local";
    panel.append(note);
    if (safety.local_only !== true) note.textContent = "安全声明异常，未执行外部调用。";
  }

  private appendRetry(
    panel: HTMLElement,
    message: string,
    action: string,
    label: string,
  ): void {
    panel.append(createElement(this.doc, "div", "sidebar-status error", message));
    const controls = createElement(this.doc, "div", "context-actions");
    const retry = createElement(this.doc, "button", "btn primary", label);
    retry.type = "button";
    retry.dataset.sidebarAction = action;
    markBound(retry);
    controls.append(retry);
    panel.append(controls);
  }

  private appendLoading(panel: HTMLElement, message: string): void {
    const status = createElement(this.doc, "div", "loading", message);
    status.setAttribute("aria-busy", "true");
    panel.append(status);
  }

  private async loadQuestionnaires(): Promise<void> {
    if (!this.contextToken || !this.workbench) return;
    const requestVersion = ++this.questionnaireRequestVersion;
    this.questionnaireLoading = true;
    this.questionnaireError = null;
    this.renderActiveContent();
    try {
      const response = await this.api.questionnaires(this.contextToken, {
        limit: 100,
      });
      if (requestVersion !== this.questionnaireRequestVersion) return;
      this.validateQuestionnaires(response);
      this.questionnaires = response;
    } catch (error) {
      if (requestVersion !== this.questionnaireRequestVersion) return;
      this.questionnaireError = error;
    } finally {
      if (requestVersion === this.questionnaireRequestVersion) {
        this.questionnaireLoading = false;
        if (this.activeTab === "questionnaires") this.renderActiveContent();
      }
    }
  }

  private validateQuestionnaires(response: SidebarQuestionnaireResponse): void {
    if (
      !response ||
      !Array.isArray(response.items) ||
      typeof response.scan_truncated !== "boolean" ||
      typeof response.result_truncated !== "boolean"
    ) {
      throw new Error("问卷响应不完整，已停止渲染。");
    }
    validateSidebarSafety(response.safety, "问卷");
    for (const item of response.items) {
      if (
        !Number.isInteger(item.submission_id) ||
        item.submission_id < 1 ||
        !Number.isInteger(item.questionnaire_id) ||
        item.questionnaire_id < 1 ||
        typeof item.submitted_at !== "string" ||
        !Number.isFinite(item.score) ||
        !Array.isArray(item.choice_answers)
      ) {
        throw new Error("问卷答案响应不完整，已停止渲染。");
      }
      for (const answer of item.choice_answers) {
        if (
          !Number.isInteger(answer.question_id) ||
          answer.question_id < 1 ||
          (answer.question_type !== "single_choice" &&
            answer.question_type !== "multi_choice") ||
          !Number.isInteger(answer.sort_order) ||
          answer.sort_order < 0 ||
          !Array.isArray(answer.option_ids)
        ) {
          throw new Error("问卷选项答案响应不完整，已停止渲染。");
        }
      }
    }
  }

  private renderQuestionnairesPanel(): HTMLElement {
    const panel = this.panelShell(
      "questionnaires",
      "问卷",
      this.questionnaires
        ? `${this.questionnaires.items.length} 条 · 本地安全答案投影`
        : "本地安全答案投影",
    );
    if (!this.questionnaires) {
      if (this.questionnaireError) {
        const status = errorStatus(this.questionnaireError);
        const message =
          status === 401
            ? "登录状态已失效，请重新打开 Sidebar 后重试。"
            : status === 403
              ? "当前账号无权查看该客户，问卷读取已安全关闭。"
              : `问卷读取失败：${errorMessage(this.questionnaireError, "请稍后重试。")}`;
        this.appendRetry(panel, message, "retry-questionnaires", "重试读取问卷");
      } else {
        this.appendLoading(panel, "正在读取问卷答案…");
      }
      return panel;
    }
    const response = this.questionnaires;
    if (response.scan_truncated || response.result_truncated) {
      const warning = createElement(
        this.doc,
        "div",
        "sidebar-status warn",
        "问卷结果已按安全上限截断，页面仅展示当前返回的答案。",
      );
      warning.dataset.questionnaireTruncated = "true";
      panel.append(warning);
    }
    if (!response.items.length)
      panel.append(createElement(this.doc, "div", "empty", "暂无问卷回答记录"));
    else {
      const list = createElement(this.doc, "div", "list");
      for (const item of response.items)
        list.append(this.renderQuestionnaireItem(item));
      panel.append(list);
    }
    this.appendSafety(panel, response.safety);
    return panel;
  }

  private renderQuestionnaireItem(
    item: SidebarQuestionnaireResponse["items"][number],
  ): HTMLElement {
    const card = createElement(this.doc, "article", "list-item");
    card.dataset.questionnaireSubmissionId = String(item.submission_id);
    const main = createElement(this.doc, "div", "item-main");
    main.append(
      createElement(
        this.doc,
        "div",
        "item-title",
        `问卷 #${item.questionnaire_id}`,
      ),
      createElement(
        this.doc,
        "div",
        "item-meta",
        `提交时间 ${item.submitted_at} · 得分 ${item.score}`,
      ),
    );
    card.append(main);
    const details = createElement(this.doc, "details", "questionnaire-answers");
    const summary = createElement(
      this.doc,
      "summary",
      "link-button",
      `展开答案（${item.choice_answers.length}）`,
    );
    details.append(summary);
    if (!item.choice_answers.length) {
      details.append(createElement(this.doc, "div", "empty", "暂无选择题答案"));
    } else {
      const answers = createElement(this.doc, "div", "answer-list");
      for (const answer of item.choice_answers) {
        const type = answer.question_type === "multi_choice" ? "多选" : "单选";
        const optionIds = answer.option_ids.length
          ? answer.option_ids.map((optionId) => `选项 #${optionId}`).join("、")
          : "未选择选项";
        answers.append(
          createElement(
            this.doc,
            "div",
            "answer-item",
            `第 ${answer.sort_order + 1} 题 · 问题 #${answer.question_id} · ${type} · ${optionIds}`,
          ),
        );
      }
      details.append(answers);
    }
    card.append(details);
    return card;
  }

  private validateTimeline(response: SidebarTimelineResponse): void {
    if (!response || !Array.isArray(response.items))
      throw new Error("时间线响应不完整，已停止渲染。");
    validateSidebarSafety(response.safety, "时间线");
    if (response.next_cursor !== undefined && !response.next_cursor)
      throw new Error("时间线游标响应不完整，已停止渲染。");
    for (const item of response.items) {
      if (
        !Number.isInteger(item.id) ||
        item.id < 1 ||
        typeof item.event_type !== "string" ||
        !item.event_type ||
        typeof item.occurred_at !== "string" ||
        !item.occurred_at
      )
        throw new Error("时间线安全元数据响应不完整，已停止渲染。");
    }
  }

  private async loadTimeline(cursor?: string): Promise<void> {
    if (!this.contextToken || !this.workbench) return;
    const append = Boolean(cursor && this.timeline);
    const requestVersion = ++this.timelineRequestVersion;
    this.timelineLoading = true;
    this.timelineError = null;
    if (!append) this.timeline = null;
    if (this.activeTab === "timeline") this.renderActiveContent();
    try {
      const response = await this.api.timeline(this.contextToken, {
        cursor,
        limit: 20,
      });
      if (requestVersion !== this.timelineRequestVersion) return;
      this.validateTimeline(response);
      this.timeline = append && this.timeline
        ? { ...response, items: [...this.timeline.items, ...response.items] }
        : response;
    } catch (error) {
      if (requestVersion !== this.timelineRequestVersion) return;
      this.timelineError = error;
    } finally {
      if (requestVersion === this.timelineRequestVersion) {
        this.timelineLoading = false;
        if (this.activeTab === "timeline") this.renderActiveContent();
      }
    }
  }

  private renderTimelinePanel(): HTMLElement {
    const response = this.timeline;
    const panel = this.panelShell(
      "timeline",
      "时间线",
      response ? `${response.items.length} 条 · 安全元数据` : "安全元数据",
    );
    if (!response) {
      if (this.timelineError)
        this.appendRetry(
          panel,
          `时间线读取失败：${errorMessage(this.timelineError, "请稍后重试。")}`,
          "retry-timeline",
          "重试读取时间线",
        );
      else this.appendLoading(panel, "正在读取安全时间线…");
      return panel;
    }
    if (this.timelineError)
      panel.append(
        createElement(
          this.doc,
          "div",
          "sidebar-status error",
          `加载更多失败：${errorMessage(this.timelineError, "请稍后重试。")}`,
        ),
      );
    if (!response.items.length)
      panel.append(createElement(this.doc, "div", "empty", "暂无时间线记录"));
    else {
      const list = createElement(this.doc, "div", "list");
      for (const item of response.items) {
        const card = createElement(this.doc, "article", "list-item");
        card.dataset.timelineEventId = String(item.id);
        card.append(
          createElement(this.doc, "div", "item-title", item.event_type),
          createElement(this.doc, "div", "item-meta", `发生时间 ${item.occurred_at}`),
        );
        const relatedTab = ["survey.submitted", "survey_submitted"].includes(item.event_type)
          ? "questionnaires"
          : item.event_type.startsWith("order.")
            ? "orders"
            : "";
        if (relatedTab) {
          const related = createElement(
            this.doc,
            "button",
            "link-button",
            relatedTab === "questionnaires" ? "查看相关问卷" : "查看相关订单",
          ) as HTMLButtonElement;
          related.type = "button";
          related.dataset.sidebarAction = relatedTab === "questionnaires"
            ? "open-related-questionnaires"
            : "open-related-orders";
          markBound(related);
          card.append(related);
        }
        list.append(card);
      }
      panel.append(list);
    }
    if (response.next_cursor) {
      const controls = createElement(this.doc, "div", "context-actions");
      const more = createElement(
        this.doc,
        "button",
        "btn ghost",
        this.timelineLoading ? "正在加载…" : "加载更多时间线",
      );
      more.type = "button";
      more.disabled = this.timelineLoading;
      more.dataset.sidebarAction = "timeline-more";
      markBound(more);
      controls.append(more);
      panel.append(controls);
    }
    this.appendSafety(panel, response.safety);
    return panel;
  }

  private validateChatActivity(response: SidebarChatActivityResponse): void {
    if (!response || !Array.isArray(response.items))
      throw new Error("聊天活动响应不完整，已停止渲染。");
    validateSidebarSafety(response.safety, "聊天活动");
    for (const cursor of [response.next_cursor, response.previous_cursor]) {
      if (cursor !== undefined && !cursor)
        throw new Error("聊天活动游标响应不完整，已停止渲染。");
    }
    for (const item of response.items) {
      if (
        (item.chat_type !== "private" && item.chat_type !== "group") ||
        typeof item.message_type !== "string" ||
        !item.message_type ||
        typeof item.sent_at !== "string" ||
        !item.sent_at
      )
        throw new Error("聊天活动安全元数据响应不完整，已停止渲染。");
    }
  }

  private async loadChatActivity(cursor?: string): Promise<void> {
    if (!this.contextToken || !this.workbench) return;
    const append = Boolean(cursor && this.chatActivity);
    const requestVersion = ++this.chatActivityRequestVersion;
    this.chatActivityLoading = true;
    this.chatActivityError = null;
    if (!append) this.chatActivity = null;
    if (this.activeTab === "chat_activity") this.renderActiveContent();
    try {
      const response = await this.api.chatActivity(this.contextToken, {
        chat_type:
          this.chatActivityType === "all" ? undefined : this.chatActivityType,
        cursor,
        limit: 50,
      });
      if (requestVersion !== this.chatActivityRequestVersion) return;
      this.validateChatActivity(response);
      this.chatActivity = append && this.chatActivity
        ? { ...response, items: [...this.chatActivity.items, ...response.items] }
        : response;
    } catch (error) {
      if (requestVersion !== this.chatActivityRequestVersion) return;
      this.chatActivityError = error;
    } finally {
      if (requestVersion === this.chatActivityRequestVersion) {
        this.chatActivityLoading = false;
        if (this.activeTab === "chat_activity") this.renderActiveContent();
      }
    }
  }

  private renderChatActivityPanel(): HTMLElement {
    const response = this.chatActivity;
    const panel = this.panelShell(
      "chat-activity",
      "聊天活动",
      response ? `${response.items.length} 条 · V2 补充能力` : "V2 补充能力",
    );
    panel.dataset.sidebarCapability = "v2-supplement";
    panel.append(
      createElement(
        this.doc,
        "div",
        "sidebar-status warn",
        "V2 补充能力 · 不计 LEGACY-S05-028 销项；仅展示聊天类型和时间，不展示正文、参与者或外部回执。",
      ),
    );
    const controls = createElement(this.doc, "div", "filter-row");
    const label = createElement(this.doc, "label", "filter-control");
    label.append(createElement(this.doc, "span", undefined, "会话类型"));
    const select = createElement(this.doc, "select");
    select.dataset.chatFilter = "chat_type";
    select.setAttribute("aria-label", "聊天活动会话类型");
    for (const [value, text] of [
      ["all", "全部"],
      ["private", "私聊"],
      ["group", "群聊"],
    ] as const) {
      const option = createElement(this.doc, "option", undefined, text);
      option.value = value;
      option.selected = this.chatActivityType === value;
      select.append(option);
    }
    label.append(select);
    controls.append(label);
    panel.append(controls);
    if (!response) {
      if (this.chatActivityError)
        this.appendRetry(
          panel,
          `聊天活动读取失败：${errorMessage(this.chatActivityError, "请稍后重试。")}`,
          "retry-chat-activity",
          "重试读取聊天活动",
        );
      else this.appendLoading(panel, "正在读取聊天活动元数据…");
      return panel;
    }
    if (this.chatActivityError)
      panel.append(
        createElement(
          this.doc,
          "div",
          "sidebar-status error",
          `加载更多失败：${errorMessage(this.chatActivityError, "请稍后重试。")}`,
        ),
      );
    if (!response.items.length)
      panel.append(createElement(this.doc, "div", "empty", "暂无聊天活动记录"));
    else {
      const list = createElement(this.doc, "div", "list");
      for (const item of response.items) {
        const card = createElement(this.doc, "article", "list-item");
        card.dataset.chatActivityAt = item.sent_at;
        card.append(
          createElement(
            this.doc,
            "div",
            "item-title",
            `${item.chat_type === "private" ? "私聊" : "群聊"} · ${item.message_type}`,
          ),
          createElement(this.doc, "div", "item-meta", `发送时间 ${item.sent_at}`),
        );
        list.append(card);
      }
      panel.append(list);
    }
    if (response.next_cursor) {
      const controlsMore = createElement(this.doc, "div", "context-actions");
      const more = createElement(
        this.doc,
        "button",
        "btn ghost",
        this.chatActivityLoading ? "正在加载…" : "加载更多聊天活动",
      );
      more.type = "button";
      more.disabled = this.chatActivityLoading;
      more.dataset.sidebarAction = "chat-activity-more";
      markBound(more);
      controlsMore.append(more);
      panel.append(controlsMore);
    }
    this.appendSafety(panel, response.safety);
    return panel;
  }

  private validateOrderResponse(response: SidebarOrderResponse): void {
    if (
      !response ||
      !Array.isArray(response.items) ||
      !Number.isInteger(response.total) ||
      response.total < 0 ||
      !Number.isInteger(response.limit) ||
      response.limit < 1 ||
      typeof response.has_more !== "boolean"
    )
      throw new Error("订单响应不完整，已停止渲染。");
    validateSidebarSafety(response.safety, "订单");
    for (const item of response.items) {
      if (
        typeof item.created_at !== "string" ||
        typeof item.merchant_order_no !== "string" ||
        typeof item.product_code !== "string" ||
        typeof item.product_name !== "string" ||
        typeof item.amount_yuan !== "string" ||
        typeof item.currency !== "string" ||
        typeof item.status !== "string" ||
        typeof item.status_label !== "string" ||
        typeof item.provider !== "string" ||
        typeof item.provider_label !== "string"
      )
        throw new Error("订单安全投影响应不完整，已停止渲染。");
    }
  }

  private async loadOrders(offset = 0): Promise<void> {
    if (!this.contextToken || !this.workbench) return;
    const append = Boolean(offset && this.orders);
    const requestVersion = ++this.ordersRequestVersion;
    this.ordersLoading = true;
    this.ordersError = null;
    if (!append) this.orders = null;
    if (this.activeTab === "orders") this.renderActiveContent();
    try {
      const response = await this.api.orders(this.contextToken, {
        limit: 20,
        offset,
      });
      if (requestVersion !== this.ordersRequestVersion) return;
      this.validateOrderResponse(response);
      this.orders = append && this.orders
        ? { ...response, items: [...this.orders.items, ...response.items] }
        : response;
    } catch (error) {
      if (requestVersion !== this.ordersRequestVersion) return;
      this.ordersError = error;
    } finally {
      if (requestVersion === this.ordersRequestVersion) {
        this.ordersLoading = false;
        if (this.activeTab === "orders") this.renderActiveContent();
      }
    }
  }

  private renderOrdersPanel(): HTMLElement {
    const response = this.orders;
    const panel = this.panelShell(
      "orders",
      "订单",
      response ? `${response.total} 条 · 安全本地投影` : "安全本地投影",
    );
    if (!response) {
      if (this.ordersError)
        this.appendRetry(
          panel,
          `订单读取失败：${errorMessage(this.ordersError, "请稍后重试。")}`,
          "retry-orders",
          "重试读取订单",
        );
      else this.appendLoading(panel, "正在读取订单…");
      return panel;
    }
    if (this.ordersError)
      panel.append(
        createElement(
          this.doc,
          "div",
          "sidebar-status error",
          `加载更多失败：${errorMessage(this.ordersError, "请稍后重试。")}`,
        ),
      );
    if (!response.items.length)
      panel.append(createElement(this.doc, "div", "empty", "暂无普通订单记录"));
    else {
      const list = createElement(this.doc, "div", "list");
      for (const item of response.items) {
        const card = createElement(this.doc, "article", "list-item");
        card.dataset.orderNo = item.merchant_order_no;
        card.append(
          createElement(this.doc, "div", "item-title", item.product_name),
          createElement(
            this.doc,
            "div",
            "item-meta",
            `${item.amount_yuan} ${item.currency} · ${item.status_label || item.status} · ${item.provider_label || item.provider}`,
          ),
        );
        const detail = createElement(
          this.doc,
          "details",
          "order-detail",
        ) as HTMLDetailsElement;
        detail.dataset.orderDetail = "local";
        detail.append(
          createElement(this.doc, "summary", "link-button", "展开安全订单详情"),
          createElement(
            this.doc,
            "div",
            "item-meta",
            `订单号 ${item.merchant_order_no} · 商品编码 ${item.product_code}`,
          ),
          createElement(
            this.doc,
            "div",
            "item-meta",
            `渠道 ${item.provider_label || item.provider} · 创建 ${item.created_at}`,
          ),
        );
        card.append(detail);
        list.append(card);
      }
      panel.append(list);
    }
    if (response.has_more) {
      const controls = createElement(this.doc, "div", "context-actions");
      const more = createElement(
        this.doc,
        "button",
        "btn ghost",
        this.ordersLoading ? "正在加载…" : "加载更多订单",
      );
      more.type = "button";
      more.disabled = this.ordersLoading;
      more.dataset.sidebarAction = "orders-more";
      markBound(more);
      controls.append(more);
      panel.append(controls);
    }
    this.appendSafety(panel, response.safety);
    return panel;
  }

  private validatePeriodicMember(member: SidebarServicePeriodMember): void {
    if (
      !member ||
      !/^spm_[A-Za-z0-9_-]{22}$/.test(member.member_ref) ||
      !Number.isInteger(member.service_product_id) ||
      member.service_product_id < 1 ||
      !Number.isInteger(member.customer_id) ||
      member.customer_id < 1 ||
      !["active", "expired", "removed"].includes(member.state) ||
      !["manual", "paid_order"].includes(member.source) ||
      typeof member.starts_at !== "string" ||
      !Number.isInteger(member.version) ||
      member.version < 1 ||
      typeof member.created_at !== "string" ||
      typeof member.updated_at !== "string" ||
      (member.remark !== undefined && typeof member.remark !== "string") ||
      (member.alliance !== undefined && typeof member.alliance !== "string")
    )
      throw new Error("周期订单安全投影响应不完整，已停止渲染。");
  }

  private validatePeriodicOrders(response: SidebarPeriodicOrderResponse): void {
    if (
      !response ||
      !Array.isArray(response.items) ||
      !Number.isInteger(response.limit) ||
      response.limit < 1 ||
      !Number.isInteger(response.offset) ||
      response.offset < 0 ||
      typeof response.has_more !== "boolean"
    )
      throw new Error("周期订单响应不完整，已停止渲染。");
    validateSidebarSafety(response.safety, "周期订单");
    for (const member of response.items) this.validatePeriodicMember(member);
  }

  private async loadPeriodicOrders(offset = 0): Promise<void> {
    if (!this.contextToken || !this.workbench) return;
    const append = Boolean(offset && this.periodicOrders);
    const requestVersion = ++this.periodicOrdersRequestVersion;
    this.periodicOrdersLoading = true;
    this.periodicOrdersError = null;
    if (!append) this.periodicOrders = null;
    if (this.activeTab === "periodic_orders") this.renderActiveContent();
    try {
      const response = await this.api.periodicOrders(this.contextToken, {
        limit: 20,
        offset,
      });
      if (requestVersion !== this.periodicOrdersRequestVersion) return;
      this.validatePeriodicOrders(response);
      this.periodicOrders = append && this.periodicOrders
        ? { ...response, items: [...this.periodicOrders.items, ...response.items] }
        : response;
    } catch (error) {
      if (requestVersion !== this.periodicOrdersRequestVersion) return;
      this.periodicOrdersError = error;
    } finally {
      if (requestVersion === this.periodicOrdersRequestVersion) {
        this.periodicOrdersLoading = false;
        if (this.activeTab === "periodic_orders") this.renderActiveContent();
      }
    }
  }

  private renderPeriodicOrdersPanel(): HTMLElement {
    const response = this.periodicOrders;
    const panel = this.panelShell(
      "periodic-orders",
      "周期订单",
      response ? `${response.items.length} 条 · canonical member 投影` : "canonical member 投影",
    );
    if (!response) {
      if (this.periodicOrdersError)
        this.appendRetry(
          panel,
          `周期订单读取失败：${errorMessage(this.periodicOrdersError, "请稍后重试。")}`,
          "retry-periodic-orders",
          "重试读取周期订单",
        );
      else this.appendLoading(panel, "正在读取周期订单…");
      return panel;
    }
    if (this.periodicOrdersError)
      panel.append(
        createElement(
          this.doc,
          "div",
          "sidebar-status error",
          `加载更多失败：${errorMessage(this.periodicOrdersError, "请稍后重试。")}`,
        ),
      );
    if (!response.items.length)
      panel.append(createElement(this.doc, "div", "empty", "暂无周期订单记录"));
    else {
      const list = createElement(this.doc, "div", "list");
      for (const member of response.items) list.append(this.renderPeriodicMember(member));
      panel.append(list);
    }
    if (response.has_more) {
      const controls = createElement(this.doc, "div", "context-actions");
      const more = createElement(
        this.doc,
        "button",
        "btn ghost",
        this.periodicOrdersLoading ? "正在加载…" : "加载更多周期订单",
      );
      more.type = "button";
      more.disabled = this.periodicOrdersLoading;
      more.dataset.sidebarAction = "periodic-orders-more";
      markBound(more);
      controls.append(more);
      panel.append(controls);
    }
    this.appendSafety(panel, response.safety);
    return panel;
  }

  private renderPeriodicMember(member: SidebarServicePeriodMember): HTMLElement {
    const card = createElement(this.doc, "article", "list-item");
    card.dataset.periodicMemberRef = member.member_ref;
    card.append(
      createElement(
        this.doc,
        "div",
        "item-title",
        `${member.state} · 服务商品 #${member.service_product_id}`,
      ),
      createElement(
        this.doc,
        "div",
        "item-meta",
        `${member.source === "paid_order" ? "付费订单" : "人工登记"} · ${member.starts_at}${member.expires_at ? ` 至 ${member.expires_at}` : ""} · version ${member.version}`,
      ),
      createElement(
        this.doc,
        "div",
        "item-meta",
        `member_ref ${member.member_ref}${member.alliance ? ` · 联盟 ${member.alliance}` : ""}`,
      ),
    );
    const label = createElement(this.doc, "label", "remark-editor");
    label.append(createElement(this.doc, "span", undefined, "周期订单备注"));
    const textarea = createElement(this.doc, "textarea");
    textarea.dataset.periodicRemark = member.member_ref;
    textarea.rows = 2;
    textarea.maxLength = 500;
    textarea.value = this.periodicRemarkDrafts.get(member.member_ref) ?? member.remark ?? "";
    textarea.setAttribute("aria-label", `周期订单 ${member.member_ref} 备注`);
    label.append(textarea);
    card.append(label);
    const controls = createElement(this.doc, "div", "context-actions");
    const save = createElement(this.doc, "button", "btn primary", "保存备注");
    save.type = "button";
    save.dataset.sidebarAction = "periodic-remark-save";
    save.dataset.memberRef = member.member_ref;
    save.dataset.serviceProductId = String(member.service_product_id);
    save.disabled = this.periodicRemarkSaving.has(member.member_ref);
    markBound(save);
    controls.append(save);
    card.append(controls);
    const status = this.periodicRemarkStatuses.get(member.member_ref);
    if (status) {
      const receipt = createElement(
        this.doc,
        "div",
        `remark-status${status.failed ? " error" : ""}`,
        status.message,
      );
      receipt.dataset.periodicRemarkReceipt = status.failed ? "failed" : "accepted";
      card.append(receipt);
    }
    return card;
  }

  private validatePeriodicRemark(response: SidebarPeriodicRemarkResponse): void {
    if (!response?.member) throw new Error("备注保存响应不完整，未显示成功。");
    this.validatePeriodicMember(response.member);
    validateSidebarSafety(response.safety, "周期订单备注");
  }

  private async savePeriodicRemark(button: HTMLButtonElement): Promise<void> {
    const memberRef = button.dataset.memberRef || "";
    const serviceProductId = Number(button.dataset.serviceProductId);
    const current = this.periodicOrders?.items.find(
      (item) => item.member_ref === memberRef,
    );
    if (!current || !Number.isInteger(serviceProductId) || serviceProductId < 1)
      return;
    const remark = (this.periodicRemarkDrafts.get(memberRef) ?? current.remark ?? "").trim();
    if (!remark) {
      this.periodicRemarkStatuses.set(memberRef, {
        message: "备注不能为空，未发起写入。",
        failed: true,
      });
      this.renderActiveContent();
      return;
    }
    if (this.periodicRemarkSaving.has(memberRef)) return;
    this.periodicRemarkSaving.add(memberRef);
    this.periodicRemarkStatuses.delete(memberRef);
    this.renderActiveContent();
    try {
      const response = await this.api.updateRemark(
        this.contextToken,
        serviceProductId,
        memberRef,
        { expected_version: current.version, remark },
      );
      this.validatePeriodicRemark(response);
      const index = this.periodicOrders?.items.findIndex(
        (item) => item.member_ref === memberRef,
      );
      if (index !== undefined && index >= 0 && this.periodicOrders) {
        this.periodicOrders.items[index] = response.member;
        this.periodicRemarkDrafts.delete(memberRef);
      }
      this.periodicRemarkStatuses.set(memberRef, {
        message: `备注已保存：accepted · 本地提交成功（CAS version ${response.member.version}）。`,
        failed: false,
      });
    } catch (error) {
      this.periodicRemarkStatuses.set(memberRef, {
        message:
          errorStatus(error) === 409
            ? "备注保存冲突：版本已变化，请刷新周期订单后重试。"
            : `备注保存失败：${errorMessage(error, "请稍后重试。")}`,
        failed: true,
      });
    } finally {
      this.periodicRemarkSaving.delete(memberRef);
      if (this.activeTab === "periodic_orders") this.renderActiveContent();
    }
  }

  private validateMaterials(response: SidebarMaterialResponse): void {
    if (
      !response ||
      !Array.isArray(response.items) ||
      !Number.isInteger(response.total) ||
      response.total < 0 ||
      !Number.isInteger(response.limit) ||
      response.limit < 1 ||
      !Number.isInteger(response.offset) ||
      response.offset < 0 ||
      !Array.isArray(response.quick_keywords)
    )
      throw new Error("素材响应不完整，已停止渲染。");
    validateSidebarSafety(response.safety, "素材");
    for (const item of response.items) {
      if (
        !Number.isInteger(item.id) ||
        item.id < 1 ||
        typeof item.name !== "string" ||
        typeof item.file_name !== "string" ||
        !item.file_name ||
        typeof item.mime_type !== "string" ||
        !item.mime_type ||
        !Number.isInteger(item.file_size) ||
        item.file_size < 1 ||
        typeof item.description !== "string" ||
        !Array.isArray(item.tags) ||
        typeof item.category !== "string" ||
        !Number.isInteger(item.width) ||
        item.width < 1 ||
        !Number.isInteger(item.height) ||
        item.height < 1 ||
        typeof item.updated_at !== "string" ||
        item.thumbnail_status !== "pending"
      )
        throw new Error("素材元数据响应不完整，已停止渲染。");
    }
  }

  private async loadMaterials(offset = 0): Promise<void> {
    if (!this.contextToken || !this.workbench) return;
    const append = Boolean(offset && this.materials);
    const requestVersion = ++this.materialsRequestVersion;
    this.materialsLoading = true;
    this.materialsError = null;
    if (!append) {
      this.materials = null;
      this.thumbnailStatuses.clear();
      this.clearThumbnailURLs();
    }
    if (this.activeTab === "materials") this.renderActiveContent();
    try {
      const params = {
        q: this.materialFilters.q.trim() || undefined,
        category: this.materialFilters.category.trim() || undefined,
        tags: this.materialFilters.tags.trim() || undefined,
        limit: 20,
        offset,
      };
      const response = await this.api.materials(this.contextToken, params);
      if (requestVersion !== this.materialsRequestVersion) return;
      this.validateMaterials(response);
      this.materials = append && this.materials
        ? { ...response, items: [...this.materials.items, ...response.items] }
        : response;
    } catch (error) {
      if (requestVersion !== this.materialsRequestVersion) return;
      this.materialsError = error;
    } finally {
      if (requestVersion === this.materialsRequestVersion) {
        this.materialsLoading = false;
        if (this.activeTab === "materials") this.renderActiveContent();
      }
    }
  }

  private clearThumbnailURLs(): void {
    const revoke = this.doc.defaultView?.URL?.revokeObjectURL;
    if (typeof revoke === "function") {
      for (const url of this.thumbnailURLs.values()) revoke.call(this.doc.defaultView?.URL, url);
    }
    this.thumbnailURLs.clear();
  }

  private queueThumbnailStatus(imageId: number): void {
    if (this.thumbnailStatuses.has(imageId)) return;
    this.thumbnailStatuses.set(imageId, "pending");
    void this.loadThumbnailStatus(imageId);
  }

  private async loadThumbnailStatus(imageId: number): Promise<void> {
    if (!this.contextToken) return;
    try {
      const blob = await this.api.thumbnailPreview(this.contextToken, imageId);
      if (!blob || blob.size < 1 || !["image/png", "image/jpeg", "image/gif"].includes(blob.type))
        throw new Error("缩略图二进制响应不完整。");
      const create = this.doc.defaultView?.URL?.createObjectURL;
      if (typeof create === "function") {
        const previous = this.thumbnailURLs.get(imageId);
        if (previous) this.doc.defaultView?.URL?.revokeObjectURL(previous);
        this.thumbnailURLs.set(imageId, create.call(this.doc.defaultView?.URL, blob));
      }
      this.thumbnailStatuses.set(imageId, "ready");
    } catch (error) {
      this.thumbnailStatuses.set(
        imageId,
        errorStatus(error) === 404 ? "not_found" : "error",
      );
    }
    if (this.activeTab === "materials") this.renderActiveContent();
  }

  private renderMaterialsPanel(): HTMLElement {
    const response = this.materials;
    const panel = this.panelShell(
      "materials",
      "素材",
      response ? `${response.total} 条 · 本地图片元数据` : "本地图片元数据",
    );
    const filters = createElement(this.doc, "div", "material-filters");
    for (const [key, labelText, placeholder] of [
      ["q", "搜索", "名称、文件名或描述"],
      ["category", "分类", "分类"],
      ["tags", "标签", "逗号分隔标签"],
    ] as const) {
      const label = createElement(this.doc, "label", "filter-control");
      label.append(createElement(this.doc, "span", undefined, labelText));
      const input = createElement(this.doc, "input");
      input.id = `material-${key}`;
      input.dataset.materialFilter = key;
      input.value = this.materialFilters[key];
      input.placeholder = placeholder;
      input.maxLength = key === "tags" ? 500 : 200;
      label.append(input);
      filters.append(label);
    }
    const search = createElement(this.doc, "button", "btn primary", "搜索素材");
    search.type = "button";
    search.dataset.sidebarAction = "materials-search";
    markBound(search);
    filters.append(search);
    panel.append(filters);
    if (response?.quick_keywords?.length) {
      const quick = createElement(this.doc, "div", "quick-keywords");
      quick.append(createElement(this.doc, "span", "panel-meta", "快捷关键词"));
      for (const keyword of response.quick_keywords) {
        const button = createElement(this.doc, "button", "link-button", keyword);
        button.type = "button";
        button.dataset.materialKeyword = keyword;
        markBound(button);
        quick.append(button);
      }
      panel.append(quick);
    }
    if (!response) {
      if (this.materialsError)
        this.appendRetry(
          panel,
          `素材读取失败：${errorMessage(this.materialsError, "请稍后重试。")}`,
          "retry-materials",
          "重试读取素材",
        );
      else this.appendLoading(panel, "正在读取素材元数据…");
      return panel;
    }
    if (this.materialsError)
      panel.append(
        createElement(
          this.doc,
          "div",
          "sidebar-status error",
          `加载更多失败：${errorMessage(this.materialsError, "请稍后重试。")}`,
        ),
      );
    if (!response.items.length)
      panel.append(createElement(this.doc, "div", "empty", "暂无匹配素材"));
    else {
      const list = createElement(this.doc, "div", "list");
      for (const item of response.items) {
        this.queueThumbnailStatus(item.id);
        const card = createElement(this.doc, "article", "list-item");
        card.dataset.materialId = String(item.id);
        const status = this.thumbnailStatuses.get(item.id) || "pending";
        const statusLabel =
          status === "ready"
            ? "thumbnail: ready"
            : status === "not_found"
            ? "thumbnail: not_found"
            : status === "error"
              ? "thumbnail: 读取失败"
              : "thumbnail: pending";
        const badge = createElement(this.doc, "span", "thumbnail-status", statusLabel);
        badge.dataset.thumbnailStatus = status;
        const previewURL = this.thumbnailURLs.get(item.id);
        if (status === "ready" && previewURL) {
          const preview = createElement(this.doc, "img") as HTMLImageElement;
          preview.src = previewURL;
          preview.alt = item.name || item.file_name;
          preview.loading = "lazy";
          preview.dataset.materialPreview = "ready";
          card.append(preview);
        }
        card.append(
          createElement(this.doc, "div", "item-title", item.name || item.file_name),
          createElement(
            this.doc,
            "div",
            "item-meta",
            `${item.file_name} · ${item.mime_type} · ${item.file_size} bytes · ${item.width}×${item.height}`,
          ),
          createElement(
            this.doc,
            "div",
            "item-meta",
            `分类 ${item.category || "未分类"}${item.tags.length ? ` · 标签 ${item.tags.join("、")}` : ""} · 更新 ${item.updated_at}`,
          ),
          badge,
        );
        if (item.description)
          card.append(createElement(this.doc, "div", "item-meta", item.description));
        list.append(card);
      }
      panel.append(list);
    }
    if (response.offset + response.items.length < response.total) {
      const controls = createElement(this.doc, "div", "context-actions");
      const more = createElement(
        this.doc,
        "button",
        "btn ghost",
        this.materialsLoading ? "正在加载…" : "加载更多素材",
      );
      more.type = "button";
      more.disabled = this.materialsLoading;
      more.dataset.sidebarAction = "materials-more";
      markBound(more);
      controls.append(more);
      panel.append(controls);
    }
    this.appendSafety(panel, response.safety);
    return panel;
  }

  private scheduleProfileSave(): void {
    if (this.profileSaveTimer) clearTimeout(this.profileSaveTimer);
    this.profileSaveTimer = setTimeout(() => {
      this.profileSaveTimer = null;
      void this.flushProfileSave();
    }, PROFILE_SAVE_DEBOUNCE_MS);
  }

  private async flushProfileSave(): Promise<void> {
    if (this.savingProfile) {
      this.saveAgain = true;
      return;
    }
    if (
      !this.contextToken ||
      !this.workbench ||
      this.pendingProfileFields.size === 0
    )
      return;
    const fields = new Set(this.pendingProfileFields);
    this.pendingProfileFields.clear();
    const profile = this.workbench.profile;
    const expectedUpdatedAt = profile.updated_at;
    const snapshot: Partial<Record<ProfileField, string>> = {};
    const patch = {} as UpdateSidebarProfileBodyPatch;
    for (const field of fields) {
      snapshot[field] = profile[field];
      patch[field] = profile[field];
    }
    this.savingProfile = true;
    this.setProfileSaveStatus("正在保存本地画像…");
    try {
      const response = await this.api.profile(this.contextToken, {
        expected_updated_at: expectedUpdatedAt,
        patch,
      });
      this.validateProfileUpdate(response);
      for (const field of fields) {
        if (profile[field] === snapshot[field])
          profile[field] = response.profile[field];
      }
      profile.updated_at = response.profile.updated_at;
      const updated = this.doc.getElementById("profile-updated-at");
      if (updated) updated.textContent = `最后本地更新：${profile.updated_at}`;
      this.renderProfileReceipt(response);
    } catch (error) {
      for (const field of fields) {
        if (profile[field] === snapshot[field])
          this.pendingProfileFields.add(field);
      }
      const status = errorStatus(error);
      const message =
        status === 409
          ? "画像保存冲突：数据已被其他操作更新，请刷新工作台后再编辑。"
          : `画像保存失败：${errorMessage(error, "请稍后重试。")}`;
      this.setProfileSaveStatus(message, true);
    } finally {
      this.savingProfile = false;
      if (this.saveAgain) {
        this.saveAgain = false;
        this.scheduleProfileSave();
      }
    }
  }

  private validateProfileUpdate(response: SidebarProfileUpdateResponse): void {
    if (!response?.profile?.updated_at || !response.safety)
      throw new Error("画像保存响应不完整，未显示成功。");
    for (const field of PROFILE_FIELDS) {
      if (typeof response.profile[field] !== "string")
        throw new Error("画像保存响应不完整，未显示成功。");
    }
  }

  private renderProfileReceipt(response: SidebarProfileUpdateResponse): void {
    const steps = profileReceiptSteps(response.safety);
    const labels = steps.map((step) => step.label).join(" · ");
    const external = response.safety.real_external_call_executed
      ? "外部调用已执行，回执需另行核对。"
      : response.safety.effect_queued &&
          response.safety.provider_execution_eligible
        ? "真实企微外呼未执行；等待 Provider 回执。"
        : "真实企微外呼未执行。";
    this.setProfileSaveStatus(`画像保存：${labels}。${external}`);
    const status = this.doc.getElementById("profile-save-status");
    if (!status) return;
    status.dataset.receipt = steps.map((step) => step.key).join(",");
    const receipt = createElement(this.doc, "div", "receipt");
    for (const step of steps) {
      const item = createElement(
        this.doc,
        "span",
        step.key === "outcome_unknown" ? "unknown" : step.key,
        step.label,
      );
      item.dataset.receiptStep = step.key;
      receipt.append(item);
    }
    status.replaceChildren(
      createElement(
        this.doc,
        "span",
        undefined,
        `画像保存：${labels}。${external}`,
      ),
      receipt,
    );
  }

  private setProfileSaveStatus(message: string, failed = false): void {
    const status = this.doc.getElementById("profile-save-status");
    if (!status) return;
    status.className = `profile-save-status${failed ? " error" : ""}`;
    status.textContent = message;
  }

  private renderViewerSessionRequired(): void {
    this.renderTabs(false);
    this.renderContextError(
      "需要先确认当前员工身份，才能读取客户范围数据。OAuth 回退只建立员工会话，返回后仍需重新读取本地上下文。",
      undefined,
      "warn",
      [{ label: "通过企微 OAuth 授权", action: "oauth", primary: true }],
    );
  }

  private renderContextError(
    message: string,
    detail?: string,
    tone: "error" | "warn" = "error",
    actions: Array<{
      label: string;
      action: "retry-context" | "oauth";
      primary?: boolean;
    }> = [{ label: "重试读取", action: "retry-context" }],
  ): void {
    this.setContextStatus(detail ? `${message} ${detail}` : message, tone);
    this.tabs.replaceChildren();
    const panel = createElement(this.doc, "section", "sidebar-panel");
    panel.dataset.sidebarSection = "context-error";
    const status = createElement(
      this.doc,
      "div",
      `sidebar-status ${tone}`,
      message,
    );
    status.dataset.contextState = tone;
    panel.append(status);
    const controls = createElement(this.doc, "div", "context-actions");
    for (const item of actions) {
      const button = createElement(
        this.doc,
        "button",
        `btn${item.primary ? " primary" : " ghost"}`,
        item.label,
      );
      button.type = "button";
      button.dataset.sidebarAction = item.action;
      markBound(button);
      controls.append(button);
    }
    panel.append(controls);
    this.content.replaceChildren(panel);
  }

  private setContextStatus(
    message: string,
    tone: "error" | "warn" | "" = "",
  ): void {
    if (!this.contextStatus) return;
    this.contextStatus.className = `sidebar-status${tone ? ` ${tone}` : ""}`;
    this.contextStatus.textContent = message;
  }

  private setSdkStatus(
    state: "idle" | "loading" | "ready" | "unavailable" | "error",
    message: string,
  ): void {
    if (!this.sdkStatus) return;
    this.sdkStatus.dataset.state = state;
    this.sdkStatus.textContent = message;
  }
}

export function startSidebar(
  doc: Document = document,
  api: BoundSidebarApi = sidebarApi,
): SidebarController {
  initFeedback();
  const controller = new SidebarController(api, doc);
  void controller.boot();
  return controller;
}

if (typeof document !== "undefined") {
  if (document.readyState === "loading")
    document.addEventListener("DOMContentLoaded", () => startSidebar());
  else startSidebar();
}
