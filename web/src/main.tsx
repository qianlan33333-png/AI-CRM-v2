/// <reference types="vite/client" />

import React, { useCallback, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  PermissionSessionCache,
  generatedAuthTransport,
  performLogout,
  permittedRoutePaths,
  type AuthPrincipal,
  type AuthTransport,
  type SessionResult,
} from "./auth";
import {
  AccountCard,
  LoginPage,
  PermissionNavigation,
  type AccountSummary,
  type LogoutState,
  type PermissionNavigationLink,
} from "./auth-ui";
import { CustomerListPage } from "./customers-ui";
import type { CustomerTransport } from "./customers";
import { CustomerDetailPage } from "./customer-detail-ui";
import type { CustomerDetailTransport } from "./customer-detail";
import { StagesPage } from "./stages-ui";
import type { StageTransport } from "./stages";
import { SegmentsPage } from "./segments-ui";
import type { SegmentTransport } from "./segments";
import { IdentityMergeReviewsPage } from "./identity-reviews-ui";
import type { IdentityReviewTransport } from "./identity-reviews";
import { IdentityConsolePage } from "./identity-console-ui";
import type { IdentityConsoleTransport } from "./identity-console";
import { MiniProgramLibraryPage } from "./miniprogram-library-ui";
import type { MiniProgramLibraryTransport } from "./miniprogram-library";
import { ImageLibraryPage } from "./image-library-ui";
import type { ImageLibraryTransport } from "./image-library";
import { HXCSenderPage } from "./hxc-sender-ui";
import type { HXCSenderTransport } from "./hxc-sender";
import { QuestionnaireListPage } from "./questionnaire-list-ui";
import type { QuestionnaireListTransport } from "./questionnaire-list";
import { PublicSurveyPage } from "./public-survey-ui";
import { publicSlug, type PublicSurveyTransport } from "./public-survey";
import { WecomTagsPage } from "./wecom-tags-ui";
import type { WecomTagsTransport } from "./wecom-tags";
import type { CallbackInboxTransport } from "./wecom-callback-inbox";
import { ChannelsPage } from "./channels-ui";
import type { ChannelsTransport } from "./channels";
import { CouponsPage } from "./coupons-ui";
import type { CouponsTransport } from "./coupons";
import { AutomationRunsPage } from "./automation-runs-ui";
import type { AutomationRunsTransport } from "./automation-runs";
import { AutomationAgentsPage } from "./automation-agents-ui";
import type { AutomationAgentsTransport } from "./automation-agents";
import { CloudOrchestratorWorkspace } from "./cloud-orchestrator-ui";
import {
  CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
  CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH,
  CLOUD_ORCHESTRATOR_PLANS_PATH,
  CLOUD_ORCHESTRATOR_ROOT_PATH,
  cloudOrchestratorCarrierRoute,
  cloudOrchestratorRoute,
  type CloudOrchestratorRoute,
} from "./cloud-orchestrator";
import { GroupOpsWorkspace } from "./group-ops-ui";
import {
  GROUP_OPS_PLANS_PATH,
  groupOpsCarrierRoute,
  groupOpsRoute,
  type GroupOpsRoute,
} from "./group-ops";
import { CommerceWorkspaces } from "./commerce-workspaces-ui";
import {
  ALIPAY_TRANSACTIONS_PATH,
  SERVICE_PRODUCT_NEW_PATH,
  SERVICE_PRODUCTS_PATH,
  WECHAT_PAY_PRODUCT_NEW_PATH,
  WECHAT_PAY_TRANSACTIONS_PATH,
  WECHAT_SHOP_TRANSACTIONS_PATH,
  commerceWorkspaceCarrierRoute,
  commerceWorkspaceRoute,
  type CommerceWorkspaceRoute,
} from "./commerce-workspaces";
import { GroupInviteLibraryPage } from "./group-invite-library-ui";
import type { GroupInviteLibraryTransport } from "./group-invite-library";
import { DeliveryLineagePage } from "./delivery-lineage-ui";
import type { DeliveryLineageTransport } from "./delivery-lineage";
import { DataHealthPage } from "./data-health-ui";
import type { DataHealthTransport } from "./data-health";
import { ExecutionRuntimePage } from "./execution-runtime-ui";
import type { ExecutionRuntimeTransport } from "./execution-runtime";
import { PushCenterPage } from "./push-center-ui";
import type { PushCenterTransport } from "./push-center";
import { OutboundOperationsPage } from "./outbound-operations-ui";
import type { OutboundOperationsTransport } from "./outbound-operations";
import { OrdersPage } from "./orders-ui";
import type { OrdersTransport } from "./orders";
import { ProductsPage } from "./products-ui";
import type { ProductsTransport } from "./products";
import { AppSettingsPage } from "./app-settings-ui";
import type { AppSettingsTransport } from "./app-settings";
import "./shell.css";

export const ROUTE_CHANGE_EVENT = "aicrm:route-change";
export const LOGIN_PATH = "/login";
export const MINIPROGRAM_LIBRARY_PATH = "/admin/miniprogram-library";
export const IMAGE_LIBRARY_PATH = "/admin/image-library";
export const HXC_SENDER_PATH = "/admin/hxc-send-config";
export const QUESTIONNAIRE_LIST_PATH = "/admin/questionnaires";
export const WECOM_TAGS_PATH = "/admin/wecom-tags";
export const CHANNELS_PATH = "/admin/channels";
export const COUPONS_PATH = "/admin/coupons";
export const AUTOMATION_RUNS_PATH = "/admin/automation-runs";
export const AUTOMATION_AGENTS_PATH = "/admin/automation-agents";
export const GROUP_INVITE_LIBRARY_PATH = "/admin/group-invite-library";
export const DELIVERY_LINEAGE_PATH = "/admin/delivery-lineage";
export const DATA_HEALTH_PATH = "/admin/data-health";
export const EXECUTION_RUNTIME_PATH = "/admin/execution-runtime";
export const ORDERS_PATH = "/admin/orders";
export const OUTBOUND_PATH = "/outbound";
export const PRODUCTS_PATH = "/admin/wechat-pay/products";
export const LEGACY_ADMIN_PATH_PARAM = "legacy_admin_path";
export const IDENTITY_CONSOLE_PATH = "/identity/console";

export const routes = [
  {
    path: "/",
    navigationLabel: "概览",
    title: "AI-CRM 运营指挥台",
    description: "运营工作台骨架已就绪；模块能力将由后续切片逐步接入。",
  },
  {
    path: "/customers",
    navigationLabel: "客户",
    title: "客户",
    description: "客户模块边界已预留，尚未接入客户数据或操作。",
  },
  {
    path: "/stages",
    navigationLabel: "阶段",
    title: "阶段",
    description: "阶段管理的稳定路由已预留，P2-17 将在此接入真实能力。",
  },
  {
    path: "/segments",
    navigationLabel: "人群包",
    title: "人群包",
    description: "人群包模块边界已预留，尚未接入筛选或物化任务。",
  },
  {
    path: IDENTITY_CONSOLE_PATH,
    navigationLabel: "身份绑定",
    title: "本地身份控制台",
    description: "查询本地身份归属并执行显式本地绑定。",
  },
  {
    path: "/identity/merge-reviews",
    navigationLabel: "待合并",
    title: "人工待合并",
    description: "Identity 人工待合并审阅与决策。",
  },
  {
    path: "/admin/miniprogram-library",
    navigationLabel: "小程序素材",
    title: "小程序素材库",
    description: "小程序素材的本地媒体库工作区。",
  },
  {
    path: "/admin/image-library",
    navigationLabel: "图片素材",
    title: "图片素材库",
    description: "图片素材的本地媒体库工作区。",
  },
  {
    path: "/admin/hxc-send-config",
    navigationLabel: "HXC 发件人",
    title: "HXC 发件人配置",
    description: "HXC 本地发件人目录与配置的只读视图。",
  },
  {
    path: "/admin/questionnaires",
    navigationLabel: "问卷",
    title: "问卷列表",
    description: "问卷定义的本地管理列表。",
  },
  {
    path: "/admin/wecom-tags",
    navigationLabel: "企微标签",
    title: "企微标签目录",
    description: "企微标签的本地只读目录。",
  },
  {
    path: "/admin/channels",
    navigationLabel: "渠道",
    title: "渠道列表",
    description: "本地渠道资源的只读列表。",
  },
  {
    path: "/admin/coupons",
    navigationLabel: "优惠券",
    title: "优惠券列表",
    description: "本地优惠券规则的浏览和复制。",
  },
  {
    path: "/admin/automation-runs",
    navigationLabel: "自动化运行",
    title: "自动化运行记录",
    description: "自动化触发记录的脱敏只读视图。",
  },
  {
    path: "/admin/automation-agents",
    navigationLabel: "自动化话术",
    title: "自动化话术",
    description: "本地自动化话术配置摘要的只读浏览。",
  },
  {
    path: CLOUD_ORCHESTRATOR_ROOT_PATH,
    navigationLabel: "AI 助手",
    title: "AI 助手工作区",
    description: "AI 助手本地审阅工作区入口。",
  },
  {
    path: CLOUD_ORCHESTRATOR_PLANS_PATH,
    navigationLabel: "AI 助手",
    title: "运营计划审阅",
    description: "本地运营计划审阅载体。",
  },
  {
    path: CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
    navigationLabel: "Campaign 审阅",
    title: "Campaign 审阅",
    description: "本地 Campaign 审阅载体。",
  },
  {
    path: CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH,
    navigationLabel: "AI 可观察性",
    title: "AI 助手可观察性",
    description: "本地可观察性入口载体。",
  },
  {
    path: GROUP_OPS_PLANS_PATH,
    navigationLabel: "群运营计划",
    title: "群运营计划",
    description: "群运营计划的本地安全工作区载体。",
  },
  {
    path: SERVICE_PRODUCTS_PATH,
    navigationLabel: "周期商品",
    title: "周期商品",
    description: "周期商品的本地管理工作区载体。",
  },
  {
    path: SERVICE_PRODUCT_NEW_PATH,
    navigationLabel: "新建周期商品",
    title: "新建周期商品",
    description: "周期商品新建工作区载体。",
  },
  {
    path: WECHAT_PAY_PRODUCT_NEW_PATH,
    navigationLabel: "新建微信支付商品",
    title: "新建微信支付商品",
    description: "微信支付商品新建工作区载体。",
  },
  {
    path: WECHAT_PAY_TRANSACTIONS_PATH,
    navigationLabel: "微信支付交易",
    title: "微信支付交易",
    description: "微信支付交易的本地只读工作区载体。",
  },
  {
    path: WECHAT_SHOP_TRANSACTIONS_PATH,
    navigationLabel: "微信小店交易",
    title: "微信小店交易",
    description: "微信小店交易的本地只读工作区载体。",
  },
  {
    path: ALIPAY_TRANSACTIONS_PATH,
    navigationLabel: "支付宝交易",
    title: "支付宝交易",
    description: "支付宝交易的本地只读工作区载体。",
  },
  {
    path: "/admin/group-invite-library",
    navigationLabel: "群邀请素材",
    title: "群邀请素材库",
    description: "本地群邀请卡元数据的只读浏览。",
  },
  {
    path: "/admin/delivery-lineage",
    navigationLabel: "投递谱系",
    title: "投递处理谱系",
    description: "本地内部处理状态的只读浏览。",
  },
  {
    path: "/admin/data-health",
    navigationLabel: "数据健康",
    title: "数据健康",
    description: "四项本地平台就绪度检查的只读视图。",
  },
  {
    path: "/admin/execution-runtime",
    navigationLabel: "执行运行时",
    title: "执行运行时",
    description: "本地执行运行时观察的安全只读视图。",
  },
  {
    path: "/admin/orders",
    navigationLabel: "订单",
    title: "订单总览",
    description: "已持久化订单投影的本地只读浏览。",
  },
  {
    path: "/admin/wechat-pay/products",
    navigationLabel: "产品",
    title: "产品目录",
    description: "已持久化产品投影的本地只读浏览。",
  },
  {
    path: "/outbound",
    navigationLabel: "群发任务",
    title: "群发任务",
    description: "群发任务模块边界已预留，尚未接入发送、统计或外部渠道。",
  },
  {
    path: "/settings",
    navigationLabel: "设置",
    title: "系统设置",
    description: "管理已持久化的非敏感本地设置，敏感信息仅显示掩码状态。",
  },
] as const;

export type AppRoute = (typeof routes)[number];

export type RouteEventTarget = Pick<
  EventTarget,
  "addEventListener" | "removeEventListener"
>;

export interface BrowserNavigator {
  events: Pick<EventTarget, "dispatchEvent">;
  history: Pick<History, "pushState">;
  location: Pick<Location, "href" | "origin">;
}

export interface NavigationClick {
  altKey: boolean;
  button: number;
  ctrlKey: boolean;
  currentTarget: Pick<HTMLAnchorElement, "hasAttribute" | "target">;
  defaultPrevented: boolean;
  metaKey: boolean;
  preventDefault: () => void;
  shiftKey: boolean;
}

export interface AppProps {
  navigation?: React.ReactNode;
  cache?: PermissionSessionCache;
  transport?: AuthTransport;
  customerTransport?: CustomerTransport;
  customerDetailTransport?: CustomerDetailTransport;
  stageTransport?: StageTransport;
  segmentTransport?: SegmentTransport;
  identityReviewTransport?: IdentityReviewTransport;
  identityConsoleTransport?: IdentityConsoleTransport;
  miniProgramTransport?: MiniProgramLibraryTransport;
  imageLibraryTransport?: ImageLibraryTransport;
  hxcSenderTransport?: HXCSenderTransport;
  questionnaireTransport?: QuestionnaireListTransport;
  publicSurveyTransport?: PublicSurveyTransport;
  wecomTagsTransport?: WecomTagsTransport;
  callbackInboxTransport?: CallbackInboxTransport;
  channelsTransport?: ChannelsTransport;
  couponsTransport?: CouponsTransport;
  automationRunsTransport?: AutomationRunsTransport;
  automationAgentsTransport?: AutomationAgentsTransport;
  groupInviteLibraryTransport?: GroupInviteLibraryTransport;
  deliveryLineageTransport?: DeliveryLineageTransport;
  dataHealthTransport?: DataHealthTransport;
  executionRuntimeTransport?: ExecutionRuntimeTransport;
  pushCenterTransport?: PushCenterTransport;
  outboundOperationsTransport?: OutboundOperationsTransport;
  ordersTransport?: OrdersTransport;
  productsTransport?: ProductsTransport;
  appSettingsTransport?: AppSettingsTransport;
  cookieHeader?: () => string;
  initialSession?: SessionResult;
}

export function routeForPathname(pathname: string): AppRoute | undefined {
  return routes.find((route) => route.path === pathname);
}

export function customerIDForPathname(pathname: string): number | undefined {
  const match = /^\/customers\/([1-9]\d*)$/.exec(pathname);
  if (!match) return undefined;
  const customerID = Number(match[1]);
  return Number.isSafeInteger(customerID) ? customerID : undefined;
}

export function routeForURL(
  href: string,
  base = "https://aicrm.invalid",
): AppRoute | undefined {
  return routeForPathname(new URL(href, base).pathname);
}

export function readPathname(): string {
  if (typeof window === "undefined") return "/";
  return window.location.pathname;
}

export function readSearch(): string {
  if (typeof window === "undefined") return "";
  return typeof window.location.search === "string"
    ? window.location.search
    : "";
}

// This public carrier is intentionally narrower than the legacy admin one:
// its root query has exactly one safe slug and cannot carry a result token or
// any other browser-owned state into the public screen.
export function publicSurveySlug(search: string): string | undefined {
  if (search === "") return undefined;
  let params: URLSearchParams;
  try {
    params = new URLSearchParams(search);
  } catch {
    return undefined;
  }
  const entries = [...params.entries()];
  if (entries.length !== 1 || entries[0][0] !== "public_survey_slug")
    return undefined;
  return publicSlug(`/q/${entries[0][1]}`) ?? undefined;
}

// The legacy admin page carrier (`GET /admin/miniprogram-library` →
// `302 /?legacy_admin_path=%2Fadmin%2Fminiprogram-library`) is the only
// query-carried route source. Only the exact frozen whitelist value maps to a
// shell route; any other value is ignored and never navigated to.
export function carrierPathname(pathname: string, search: string): string {
  if (pathname !== "/" || search === "") return pathname;
  const cloudOrchestratorCarrier = cloudOrchestratorCarrierRoute(search);
  if (cloudOrchestratorCarrier) return cloudOrchestratorCarrier.pathname;
  const groupOpsCarrier = groupOpsCarrierRoute(search);
  if (groupOpsCarrier) return groupOpsCarrier.pathname;
  const commerceCarrier = commerceWorkspaceCarrierRoute(search);
  if (commerceCarrier) return commerceCarrier.pathname;
  let params: URLSearchParams;
  try {
    params = new URLSearchParams(search);
  } catch {
    return pathname;
  }
  const values = params.getAll(LEGACY_ADMIN_PATH_PARAM);
  if (values.length !== 1) return pathname;
  return values[0] === MINIPROGRAM_LIBRARY_PATH ||
    values[0] === HXC_SENDER_PATH ||
    values[0] === QUESTIONNAIRE_LIST_PATH ||
    values[0] === WECOM_TAGS_PATH ||
    values[0] === CHANNELS_PATH ||
    values[0] === COUPONS_PATH ||
    values[0] === AUTOMATION_AGENTS_PATH ||
    values[0] === EXECUTION_RUNTIME_PATH ||
    values[0] === ORDERS_PATH ||
    values[0] === PRODUCTS_PATH
    ? values[0]
    : pathname;
}

export function subscribeToRouteChanges(
  target: RouteEventTarget,
  onChange: () => void,
): () => void {
  const listener: EventListener = () => onChange();
  target.addEventListener("popstate", listener);
  target.addEventListener(ROUTE_CHANGE_EVENT, listener);

  return () => {
    target.removeEventListener("popstate", listener);
    target.removeEventListener(ROUTE_CHANGE_EVENT, listener);
  };
}

export function shouldInterceptNavigationClick(
  event: NavigationClick,
  destination: string,
  origin: string,
): boolean {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.altKey ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey ||
    event.currentTarget.target !== "" ||
    event.currentTarget.hasAttribute("download")
  ) {
    return false;
  }

  try {
    return new URL(destination, origin).origin === origin;
  } catch {
    return false;
  }
}

export function navigateTo(
  destination: string,
  browser: BrowserNavigator,
): boolean {
  let url: URL;
  try {
    url = new URL(destination, browser.location.href);
  } catch {
    return false;
  }

  if (url.origin !== browser.location.origin) return false;

  browser.history.pushState({}, "", `${url.pathname}${url.search}${url.hash}`);
  browser.events.dispatchEvent(new Event(ROUTE_CHANGE_EVENT));
  return true;
}

function browserNavigator(): BrowserNavigator | undefined {
  if (typeof window === "undefined") return undefined;

  return {
    events: window,
    history: window.history,
    location: window.location,
  };
}

export function handleNavigationClick(
  event: NavigationClick,
  destination: string,
  browser = browserNavigator(),
): boolean {
  if (
    !browser ||
    !shouldInterceptNavigationClick(event, destination, browser.location.origin)
  ) {
    return false;
  }

  event.preventDefault();
  return navigateTo(destination, browser);
}

function PageContent({
  route,
  cloudOrchestrator,
  groupOps,
  commerceWorkspace,
  principal,
  customerTransport,
  customerDetailTransport,
  customerID,
  stageTransport,
  segmentTransport,
  identityReviewTransport,
  identityConsoleTransport,
  miniProgramTransport,
  imageLibraryTransport,
  hxcSenderTransport,
  questionnaireTransport,
  wecomTagsTransport,
  callbackInboxTransport,
  channelsTransport,
  couponsTransport,
  automationRunsTransport,
  automationAgentsTransport,
  groupInviteLibraryTransport,
  deliveryLineageTransport,
  dataHealthTransport,
  executionRuntimeTransport,
  pushCenterTransport,
  outboundOperationsTransport,
  ordersTransport,
  productsTransport,
  appSettingsTransport,
  cookieHeader,
  onUnauthenticated,
}: {
  route: AppRoute | undefined;
  cloudOrchestrator?: CloudOrchestratorRoute;
  groupOps?: GroupOpsRoute;
  commerceWorkspace?: CommerceWorkspaceRoute;
  principal: AuthPrincipal;
  customerTransport?: CustomerTransport;
  customerDetailTransport?: CustomerDetailTransport;
  customerID?: number;
  stageTransport?: StageTransport;
  segmentTransport?: SegmentTransport;
  identityReviewTransport?: IdentityReviewTransport;
  identityConsoleTransport?: IdentityConsoleTransport;
  miniProgramTransport?: MiniProgramLibraryTransport;
  imageLibraryTransport?: ImageLibraryTransport;
  hxcSenderTransport?: HXCSenderTransport;
  questionnaireTransport?: QuestionnaireListTransport;
  wecomTagsTransport?: WecomTagsTransport;
  callbackInboxTransport?: CallbackInboxTransport;
  channelsTransport?: ChannelsTransport;
  couponsTransport?: CouponsTransport;
  automationRunsTransport?: AutomationRunsTransport;
  automationAgentsTransport?: AutomationAgentsTransport;
  groupInviteLibraryTransport?: GroupInviteLibraryTransport;
  deliveryLineageTransport?: DeliveryLineageTransport;
  dataHealthTransport?: DataHealthTransport;
  executionRuntimeTransport?: ExecutionRuntimeTransport;
  pushCenterTransport?: PushCenterTransport;
  outboundOperationsTransport?: OutboundOperationsTransport;
  ordersTransport?: OrdersTransport;
  productsTransport?: ProductsTransport;
  appSettingsTransport?: AppSettingsTransport;
  cookieHeader: () => string;
  onUnauthenticated: () => void;
}) {
  if (groupOps) {
    return <GroupOpsWorkspace role={principal.role} route={groupOps} />;
  }
  if (commerceWorkspace) {
    return (
      <CommerceWorkspaces role={principal.role} route={commerceWorkspace} />
    );
  }
  if (cloudOrchestrator) {
    return (
      <CloudOrchestratorWorkspace
        role={principal.role}
        route={cloudOrchestrator}
      />
    );
  }

  if (customerID !== undefined) {
    return (
      <CustomerDetailPage
        customerID={customerID}
        transport={customerDetailTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (!route) {
    return (
      <section
        className="route-card route-card--missing"
        aria-labelledby="app-title"
      >
        <p className="route-card__eyebrow">404</p>
        <h1 id="app-title">未找到页面</h1>
        <p>此地址不属于当前 Web shell 的已冻结路由。</p>
        <a
          className="route-card__return"
          href="/"
          onClick={(event) => handleNavigationClick(event, "/")}
        >
          返回运营指挥台
        </a>
      </section>
    );
  }

  if (route.path === "/stages") {
    return (
      <StagesPage
        role={principal.role}
        transport={stageTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === "/customers") {
    return (
      <CustomerListPage
        role={principal.role}
        transport={customerTransport}
        onUnauthenticated={onUnauthenticated}
        onCustomerNavigate={(event) =>
          handleNavigationClick(event, event.currentTarget.href)
        }
      />
    );
  }

  if (route.path === "/segments") {
    return (
      <SegmentsPage
        role={principal.role}
        transport={segmentTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === "/identity/merge-reviews") {
    return (
      <IdentityMergeReviewsPage
        role={principal.role}
        transport={identityReviewTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === IDENTITY_CONSOLE_PATH) {
    return (
      <IdentityConsolePage
        role={principal.role}
        transport={identityConsoleTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === MINIPROGRAM_LIBRARY_PATH) {
    return (
      <MiniProgramLibraryPage
        role={principal.role}
        transport={miniProgramTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === IMAGE_LIBRARY_PATH) {
    return (
      <ImageLibraryPage
        role={principal.role}
        transport={imageLibraryTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === HXC_SENDER_PATH) {
    return (
      <HXCSenderPage
        role={principal.role}
        transport={hxcSenderTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === QUESTIONNAIRE_LIST_PATH) {
    return (
      <QuestionnaireListPage
        role={principal.role}
        transport={questionnaireTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === WECOM_TAGS_PATH) {
    return (
      <WecomTagsPage
        role={principal.role}
        transport={wecomTagsTransport}
        callbackTransport={callbackInboxTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === CHANNELS_PATH) {
    return (
      <ChannelsPage
        role={principal.role}
        transport={channelsTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === COUPONS_PATH) {
    return (
      <CouponsPage
        role={principal.role}
        transport={couponsTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === AUTOMATION_RUNS_PATH) {
    return (
      <AutomationRunsPage
        role={principal.role}
        transport={automationRunsTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === AUTOMATION_AGENTS_PATH) {
    return (
      <AutomationAgentsPage
        role={principal.role}
        transport={automationAgentsTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === GROUP_INVITE_LIBRARY_PATH) {
    return (
      <GroupInviteLibraryPage
        role={principal.role}
        transport={groupInviteLibraryTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === DELIVERY_LINEAGE_PATH) {
    return (
      <DeliveryLineagePage
        role={principal.role}
        transport={deliveryLineageTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === DATA_HEALTH_PATH) {
    return (
      <DataHealthPage
        role={principal.role}
        transport={dataHealthTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === EXECUTION_RUNTIME_PATH) {
    return (
      <ExecutionRuntimePage
        role={principal.role}
        transport={executionRuntimeTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  if (route.path === OUTBOUND_PATH) {
    return (
      <>
        <PushCenterPage
          role={principal.role}
          transport={pushCenterTransport}
          onUnauthenticated={onUnauthenticated}
        />
        <OutboundOperationsPage
          role={principal.role}
          transport={outboundOperationsTransport}
          onUnauthenticated={onUnauthenticated}
        />
      </>
    );
  }

  if (route.path === ORDERS_PATH) {
    return (
      <OrdersPage
        role={principal.role}
        transport={ordersTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }
  if (route.path === PRODUCTS_PATH) {
    return (
      <ProductsPage
        role={principal.role}
        transport={productsTransport}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }
  if (route.path === "/settings") {
    return (
      <AppSettingsPage
        role={principal.role}
        transport={appSettingsTransport}
        readCookie={cookieHeader}
        onUnauthenticated={onUnauthenticated}
      />
    );
  }

  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">模块边界</p>
      <h1 id="app-title">{route.title}</h1>
      <p>{route.description}</p>
    </section>
  );
}

const runtimeCache = new PermissionSessionCache(
  generatedAuthTransport.getSession,
);

function runtimeCookieHeader(): string {
  if (typeof document === "undefined") return "";
  return document.cookie;
}

function accountSummary(principal: AuthPrincipal): AccountSummary {
  const labels: Record<AuthPrincipal["role"], string> = {
    admin: "管理员",
    ops: "运营",
    sales: "销售",
  };
  return {
    displayName: `后台账号 #${principal.adminUserID}`,
    roleLabel: labels[principal.role],
  };
}

export function navigationLinks(
  principal: AuthPrincipal,
): readonly PermissionNavigationLink[] {
  const base = permittedRoutePaths(principal);
  const permitted = new Set(base);
  // Mirrors the frozen server capability map: media.library.read is held by
  // admin and ops only, so sales never sees the media library navigation
  // entries.
  if (
    base.length > 0 &&
    (principal.role === "admin" || principal.role === "ops")
  ) {
    permitted.add(MINIPROGRAM_LIBRARY_PATH);
    permitted.add(IMAGE_LIBRARY_PATH);
  }
  if (base.length > 0 && principal.role === "admin") {
    permitted.add(HXC_SENDER_PATH);
  }
  if (
    base.length > 0 &&
    (principal.role === "admin" || principal.role === "ops")
  ) {
    permitted.add(QUESTIONNAIRE_LIST_PATH);
  }
  if (base.length > 0 && principal.role === "admin") {
    permitted.add(AUTOMATION_RUNS_PATH);
    permitted.add(AUTOMATION_AGENTS_PATH);
    permitted.add(DELIVERY_LINEAGE_PATH);
    permitted.add(DATA_HEALTH_PATH);
    permitted.add(EXECUTION_RUNTIME_PATH);
    permitted.add(OUTBOUND_PATH);
    permitted.add(CLOUD_ORCHESTRATOR_PLANS_PATH);
    permitted.add(GROUP_OPS_PLANS_PATH);
    permitted.add(SERVICE_PRODUCTS_PATH);
    permitted.add(WECHAT_PAY_TRANSACTIONS_PATH);
    permitted.add(WECHAT_SHOP_TRANSACTIONS_PATH);
    permitted.add(ALIPAY_TRANSACTIONS_PATH);
  }
  if (
    base.length > 0 &&
    (principal.role === "admin" || principal.role === "ops")
  ) {
    permitted.add(WECOM_TAGS_PATH);
    permitted.add(CHANNELS_PATH);
    permitted.add(COUPONS_PATH);
    permitted.add(GROUP_INVITE_LIBRARY_PATH);
    permitted.add(ORDERS_PATH);
    permitted.add(PRODUCTS_PATH);
  }
  return routes
    .filter((route) => permitted.has(route.path))
    .map((route) => ({ href: route.path, label: route.navigationLabel }));
}

type AppSessionState = SessionResult | { status: "checking" };

export function App({
  navigation,
  cache = runtimeCache,
  transport = generatedAuthTransport,
  customerTransport,
  customerDetailTransport,
  stageTransport,
  segmentTransport,
  identityReviewTransport,
  identityConsoleTransport,
  miniProgramTransport,
  imageLibraryTransport,
  hxcSenderTransport,
  questionnaireTransport,
  publicSurveyTransport,
  wecomTagsTransport,
  callbackInboxTransport,
  channelsTransport,
  couponsTransport,
  automationRunsTransport,
  automationAgentsTransport,
  groupInviteLibraryTransport,
  deliveryLineageTransport,
  dataHealthTransport,
  executionRuntimeTransport,
  pushCenterTransport,
  outboundOperationsTransport,
  ordersTransport,
  productsTransport,
  appSettingsTransport,
  cookieHeader = runtimeCookieHeader,
  initialSession,
}: AppProps) {
  const [pathname, setPathname] = useState(readPathname);
  const [search, setSearch] = useState(readSearch);
  const [session, setSession] = useState<AppSessionState>(
    initialSession ?? { status: "checking" },
  );
  const [logoutState, setLogoutState] = useState<LogoutState>("ready");
  const effectivePathname = carrierPathname(pathname, search);
  const route = routeForPathname(effectivePathname);
  const cloudOrchestrator = cloudOrchestratorRoute(effectivePathname);
  const groupOps = groupOpsRoute(effectivePathname);
  const commerceWorkspace = commerceWorkspaceRoute(effectivePathname);
  const customerID = customerIDForPathname(effectivePathname);
  const publicSurvey = pathname === "/" ? publicSurveySlug(search) : undefined;

  useEffect(() => {
    if (typeof window === "undefined") return undefined;
    return subscribeToRouteChanges(window, () => {
      setPathname(readPathname());
      setSearch(readSearch());
    });
  }, []);

  useEffect(() => {
    if (initialSession || publicSurvey) return undefined;
    let active = true;
    void cache.load().then((result) => {
      if (active) setSession(result);
    });
    return () => {
      active = false;
    };
  }, [cache, initialSession, publicSurvey]);

  const retrySession = () => {
    setSession({ status: "checking" });
    void cache.load(true).then(setSession);
  };

  const requestLogout = () => {
    if (logoutState === "pending") return;
    let cookies: string;
    try {
      cookies = cookieHeader();
    } catch {
      setLogoutState("error");
      return;
    }
    setLogoutState("pending");
    void performLogout(transport, cache, cookies).then((result) => {
      if (result === "logged_out" || result === "unauthenticated") {
        setLogoutState("ready");
        setSession({ status: "unauthenticated" });
        return;
      }
      setLogoutState("error");
    });
  };

  const markSessionUnauthenticated = useCallback(() => {
    cache.invalidate();
    setSession({ status: "unauthenticated" });
  }, [cache]);

  const loginView = (() => {
    if (session.status === "checking") return { kind: "checking" } as const;
    if (session.status === "unavailable") {
      return { kind: "service-error", onRetry: retrySession } as const;
    }
    if (session.status === "unauthenticated") {
      return { kind: "anonymous" } as const;
    }
    return {
      kind: "authenticated",
      account: accountSummary(session.principal),
      logout: { state: logoutState, onRequest: requestLogout },
      workbenchLink: { href: "/", label: "返回运营指挥台" },
      onReturnToWorkbench: (event: React.MouseEvent<HTMLAnchorElement>) =>
        handleNavigationClick(event, event.currentTarget.href),
    } as const;
  })();

  if (publicSurvey)
    return (
      <PublicSurveyPage slug={publicSurvey} transport={publicSurveyTransport} />
    );

  if (session.status !== "authenticated" || pathname === LOGIN_PATH) {
    return (
      <div className="app-shell app-shell--login">
        <a className="skip-link" href="#main-content">
          跳至主要内容
        </a>
        <header className="shell-header">
          <a
            aria-label="AI-CRM"
            className="product-mark"
            href="/"
            onClick={(event) => handleNavigationClick(event, "/")}
          >
            <span aria-hidden="true">AI</span>
            <span>CRM</span>
          </a>
          <p className="product-context">运营指挥台</p>
        </header>
        <main id="main-content" className="login-main" tabIndex={-1}>
          <LoginPage view={loginView} titleId="app-title" />
        </main>
      </div>
    );
  }

  const links = navigationLinks(session.principal);
  const account = accountSummary(session.principal);

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳至主要内容
      </a>
      <header className="shell-header">
        <a
          aria-label="AI-CRM"
          className="product-mark"
          href="/"
          onClick={(event) => handleNavigationClick(event, "/")}
        >
          <span aria-hidden="true">AI</span>
          <span>CRM</span>
        </a>
        <p className="product-context">运营指挥台</p>
        <AccountCard
          account={account}
          logout={{ state: logoutState, onRequest: requestLogout }}
        />
      </header>
      <div className="shell-frame">
        <nav className="shell-nav" aria-label="主导航">
          {navigation ?? (
            <PermissionNavigation
              activeHref={
                customerID === undefined ? effectivePathname : "/customers"
              }
              links={links}
              onNavigate={(event) =>
                handleNavigationClick(event, event.currentTarget.href)
              }
            />
          )}
        </nav>
        <main id="main-content" className="shell-main" tabIndex={-1}>
          <PageContent
            route={route}
            cloudOrchestrator={cloudOrchestrator}
            groupOps={groupOps}
            commerceWorkspace={commerceWorkspace}
            principal={session.principal}
            customerTransport={customerTransport}
            customerDetailTransport={customerDetailTransport}
            customerID={customerID}
            stageTransport={stageTransport}
            segmentTransport={segmentTransport}
            identityReviewTransport={identityReviewTransport}
            identityConsoleTransport={identityConsoleTransport}
            miniProgramTransport={miniProgramTransport}
            imageLibraryTransport={imageLibraryTransport}
            hxcSenderTransport={hxcSenderTransport}
            questionnaireTransport={questionnaireTransport}
            wecomTagsTransport={wecomTagsTransport}
            callbackInboxTransport={callbackInboxTransport}
            channelsTransport={channelsTransport}
            couponsTransport={couponsTransport}
            automationRunsTransport={automationRunsTransport}
            automationAgentsTransport={automationAgentsTransport}
            groupInviteLibraryTransport={groupInviteLibraryTransport}
            deliveryLineageTransport={deliveryLineageTransport}
            dataHealthTransport={dataHealthTransport}
            executionRuntimeTransport={executionRuntimeTransport}
            pushCenterTransport={pushCenterTransport}
            outboundOperationsTransport={outboundOperationsTransport}
            ordersTransport={ordersTransport}
            productsTransport={productsTransport}
            appSettingsTransport={appSettingsTransport}
            cookieHeader={cookieHeader}
            onUnauthenticated={markSessionUnauthenticated}
          />
        </main>
      </div>
      <aside className="environment-status" aria-label="环境状态" role="status">
        <span aria-hidden="true" className="environment-status__dot" />
        <span>应用骨架已就绪</span>
        <span className="environment-status__detail">接口状态待接入</span>
      </aside>
    </div>
  );
}

export function mount(root: HTMLElement) {
  createRoot(root).render(<App />);
}

if (typeof document !== "undefined") {
  const root = document.getElementById("root");
  if (!root) throw new Error("AI-CRM web root is missing");
  mount(root);
}
