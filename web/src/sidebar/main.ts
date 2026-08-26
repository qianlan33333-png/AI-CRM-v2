/**
 * 企微侧边栏入口。
 *
 * 这里仅消费当前 Go OpenAPI 的 sidebar V2 契约。没有上下文或真实读取失败时，
 * 页面保持失败/待授权状态，不回退到示例数据或静态成功文案。
 */
import { sidebarApi } from "../api/sidebar";
import type {
  SidebarAgentConfigSignature,
  SidebarProfileUpdateResponse,
  SidebarProfileUpdateSafety,
  SidebarQuestionnaireResponse,
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
  | "oauthStart"
  | "oauthStartUrl"
  | "oauthCallback"
  | "workbench"
  | "profile"
  | "questionnaires"
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

type SidebarTab = "profile" | "questionnaires";

function isSidebarTab(value: string | undefined): value is SidebarTab {
  return value === "profile" || value === "questionnaires";
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

function responseLocation(response: { headers?: Headers }): string {
  return (
    response.headers?.get("location") || response.headers?.get("Location") || ""
  );
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
  private questionnaireRequestVersion = 0;

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
      const target = event.target as HTMLTextAreaElement;
      const field = target.dataset.profileField;
      if (!isProfileField(field) || !this.workbench) return;
      this.workbench.profile[field] = target.value;
      this.pendingProfileFields.add(field);
      this.setProfileSaveStatus("待保存：停止编辑 520ms 后自动保存。");
      this.scheduleProfileSave();
    });
    this.content.addEventListener("click", (event) => {
      const button = (event.target as HTMLElement).closest<HTMLButtonElement>(
        "[data-sidebar-action]",
      );
      if (!button || button.disabled) return;
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
      const response = await this.api.oauthCallback({ code, state });
      if (response.status >= 300 && response.status < 400) {
        const location = responseLocation(response);
        if (location) {
          this.setContextStatus(
            "OAuth 回调已受理，正在重新读取 Sidebar 上下文…",
          );
          this.navigate(location);
          return true;
        }
        throw new Error("OAuth 回调缺少重定向地址。");
      }
      if (response.status < 200 || response.status >= 300) {
        throw new Error(`OAuth 回调失败（HTTP ${response.status}）。`);
      }
      return false;
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
      const response = await this.api.oauthStart({
        external_userid: this.externalUserId,
        next: this.nextPath(),
      });
      const responseStatus = response.status as number;
      if (responseStatus === 0) {
        // Cross-origin provider redirects are opaque to fetch in browsers. In
        // that case navigate to the exact generated route so the server can
        // return the provider redirect without exposing it to JavaScript.
        const route = this.api.oauthStartUrl({
          external_userid: this.externalUserId,
          next: this.nextPath(),
        });
        this.setContextStatus(
          "OAuth 已发起，等待企微回调；未将受理状态视为授权成功。",
        );
        this.navigate(route);
        return;
      }
      if (responseStatus < 300 || responseStatus >= 400) {
        throw new Error(`OAuth 发起失败（HTTP ${responseStatus}）。`);
      }
      const location = responseLocation(response);
      if (!location) throw new Error("OAuth 发起响应缺少重定向地址。");
      this.setContextStatus(
        "OAuth 已发起，等待企微回调；未将受理状态视为授权成功。",
      );
      this.navigate(location);
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
    this.questionnaireRequestVersion += 1;
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
      ["products", "商品"],
      ["orders", `订单 ${wb?.order_count ?? ""}`],
      ["coupons", "优惠券"],
      ["materials", `素材 ${wb?.material_count ?? ""}`],
      ["other_staff_messages", "其他客服聊天"],
    ] as const;
    this.tabs.replaceChildren(
      ...definitions.map(([key, label]) => {
        const button = createElement(this.doc, "button", "tab", label);
        button.type = "button";
        button.dataset.sidebarTab = key;
        const supported = key === "profile" || key === "questionnaires";
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
    if (tab === "profile") {
      this.renderActiveContent();
      return;
    }
    if (this.questionnaires) {
      this.renderActiveContent();
      return;
    }
    this.renderQuestionnaireLoading();
    void this.loadQuestionnaires();
  }

  private renderActiveContent(): void {
    const workbench = this.workbench;
    if (!workbench) return;
    if (this.activeTab === "questionnaires") {
      if (this.questionnaires) {
        this.renderQuestionnaires(this.questionnaires);
      } else {
        this.renderQuestionnaireLoading();
      }
      return;
    }
    this.content.replaceChildren(
      this.renderOverview(workbench),
      this.renderProfile(workbench),
    );
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
    return panel;
  }

  private renderQuestionnaireLoading(): void {
    const workbench = this.workbench;
    if (!workbench || this.activeTab !== "questionnaires") return;
    const panel = createElement(this.doc, "section", "sidebar-panel");
    panel.dataset.sidebarSection = "questionnaires";
    const head = createElement(this.doc, "div", "panel-head");
    head.append(createElement(this.doc, "h2", undefined, "问卷"));
    head.append(
      createElement(this.doc, "span", "panel-meta", "本地安全答案投影"),
    );
    panel.append(head);
    const status = createElement(
      this.doc,
      "div",
      "loading",
      "正在读取问卷答案…",
    );
    status.setAttribute("aria-busy", "true");
    panel.append(status);
    this.content.replaceChildren(this.renderOverview(workbench), panel);
  }

  private async loadQuestionnaires(): Promise<void> {
    if (!this.contextToken || !this.workbench) return;
    const requestVersion = ++this.questionnaireRequestVersion;
    try {
      const response = await this.api.questionnaires(this.contextToken, {
        limit: 100,
      });
      if (requestVersion !== this.questionnaireRequestVersion) return;
      this.validateQuestionnaires(response);
      this.questionnaires = response;
      if (this.activeTab === "questionnaires")
        this.renderQuestionnaires(response);
    } catch (error) {
      if (
        requestVersion !== this.questionnaireRequestVersion ||
        this.activeTab !== "questionnaires"
      )
        return;
      this.renderQuestionnaireError(error);
    }
  }

  private validateQuestionnaires(response: SidebarQuestionnaireResponse): void {
    if (
      !response ||
      !Array.isArray(response.items) ||
      typeof response.scan_truncated !== "boolean" ||
      typeof response.result_truncated !== "boolean" ||
      !response.safety ||
      typeof response.safety.local_only !== "boolean" ||
      typeof response.safety.provider_execution_eligible !== "boolean" ||
      typeof response.safety.real_external_call_executed !== "boolean"
    ) {
      throw new Error("问卷响应不完整，已停止渲染。");
    }
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

  private renderQuestionnaires(response: SidebarQuestionnaireResponse): void {
    const workbench = this.workbench;
    if (!workbench || this.activeTab !== "questionnaires") return;
    const panel = createElement(this.doc, "section", "sidebar-panel");
    panel.dataset.sidebarSection = "questionnaires";
    const head = createElement(this.doc, "div", "panel-head");
    head.append(createElement(this.doc, "h2", undefined, "问卷"));
    head.append(
      createElement(
        this.doc,
        "span",
        "panel-meta",
        `${response.items.length} 条 · 本地安全答案投影`,
      ),
    );
    panel.append(head);
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
    if (!response.items.length) {
      panel.append(createElement(this.doc, "div", "empty", "暂无问卷回答记录"));
      this.content.replaceChildren(this.renderOverview(workbench), panel);
      return;
    }
    const list = createElement(this.doc, "div", "list");
    for (const item of response.items)
      list.append(this.renderQuestionnaireItem(item));
    panel.append(list);
    const safety = createElement(
      this.doc,
      "div",
      "panel-meta",
      response.safety.local_only
        ? "数据来源：本地 CRM · 未执行企微外部调用"
        : "数据来源：受控本地投影 · 外部效果未验证",
    );
    safety.dataset.sidebarSafety = "local";
    panel.append(safety);
    this.content.replaceChildren(this.renderOverview(workbench), panel);
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

  private renderQuestionnaireError(error: unknown): void {
    const workbench = this.workbench;
    if (!workbench || this.activeTab !== "questionnaires") return;
    const panel = createElement(this.doc, "section", "sidebar-panel");
    panel.dataset.sidebarSection = "questionnaires";
    const head = createElement(this.doc, "div", "panel-head");
    head.append(createElement(this.doc, "h2", undefined, "问卷"));
    head.append(createElement(this.doc, "span", "panel-meta", "读取失败"));
    panel.append(head);
    const status = errorStatus(error);
    const message =
      status === 401
        ? "登录状态已失效，请重新打开 Sidebar 后重试。"
        : status === 403
          ? "当前账号无权查看该客户，问卷读取已安全关闭。"
          : `问卷读取失败：${errorMessage(error, "请稍后重试。")}`;
    panel.append(
      createElement(this.doc, "div", "sidebar-status error", message),
    );
    const controls = createElement(this.doc, "div", "context-actions");
    const retry = createElement(
      this.doc,
      "button",
      "btn primary",
      "重试读取问卷",
    );
    retry.type = "button";
    retry.dataset.sidebarAction = "retry-questionnaires";
    markBound(retry);
    controls.append(retry);
    panel.append(controls);
    this.content.replaceChildren(this.renderOverview(workbench), panel);
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
