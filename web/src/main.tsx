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
import { MiniProgramLibraryPage } from "./miniprogram-library-ui";
import type { MiniProgramLibraryTransport } from "./miniprogram-library";
import { ImageLibraryPage } from "./image-library-ui";
import type { ImageLibraryTransport } from "./image-library";
import { HXCSenderPage } from "./hxc-sender-ui";
import type { HXCSenderTransport } from "./hxc-sender";
import { QuestionnaireListPage } from "./questionnaire-list-ui";
import type { QuestionnaireListTransport } from "./questionnaire-list";
import { WecomTagsPage } from "./wecom-tags-ui";
import type { WecomTagsTransport } from "./wecom-tags";
import { ChannelsPage } from "./channels-ui";
import type { ChannelsTransport } from "./channels";
import { CouponsPage } from "./coupons-ui";
import type { CouponsTransport } from "./coupons";
import { AutomationRunsPage } from "./automation-runs-ui";
import type { AutomationRunsTransport } from "./automation-runs";
import { GroupInviteLibraryPage } from "./group-invite-library-ui";
import type { GroupInviteLibraryTransport } from "./group-invite-library";
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
export const GROUP_INVITE_LIBRARY_PATH = "/admin/group-invite-library";
export const LEGACY_ADMIN_PATH_PARAM = "legacy_admin_path";

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
    path: "/admin/group-invite-library",
    navigationLabel: "群邀请素材",
    title: "群邀请素材库",
    description: "本地群邀请卡元数据的只读浏览。",
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
    description: "设置模块边界已预留，尚未提供凭据、权限或配置编辑能力。",
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
  miniProgramTransport?: MiniProgramLibraryTransport;
  imageLibraryTransport?: ImageLibraryTransport;
  hxcSenderTransport?: HXCSenderTransport;
  questionnaireTransport?: QuestionnaireListTransport;
  wecomTagsTransport?: WecomTagsTransport;
  channelsTransport?: ChannelsTransport;
  couponsTransport?: CouponsTransport;
  automationRunsTransport?: AutomationRunsTransport;
  groupInviteLibraryTransport?: GroupInviteLibraryTransport;
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

// The legacy admin page carrier (`GET /admin/miniprogram-library` →
// `302 /?legacy_admin_path=%2Fadmin%2Fminiprogram-library`) is the only
// query-carried route source. Only the exact frozen whitelist value maps to a
// shell route; any other value is ignored and never navigated to.
export function carrierPathname(pathname: string, search: string): string {
  if (pathname !== "/" || search === "") return pathname;
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
    values[0] === COUPONS_PATH
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
  principal,
  customerTransport,
  customerDetailTransport,
  customerID,
  stageTransport,
  segmentTransport,
  identityReviewTransport,
  miniProgramTransport,
  imageLibraryTransport,
  hxcSenderTransport,
  questionnaireTransport,
  wecomTagsTransport,
  channelsTransport,
  couponsTransport,
  automationRunsTransport,
  groupInviteLibraryTransport,
  cookieHeader,
  onUnauthenticated,
}: {
  route: AppRoute | undefined;
  principal: AuthPrincipal;
  customerTransport?: CustomerTransport;
  customerDetailTransport?: CustomerDetailTransport;
  customerID?: number;
  stageTransport?: StageTransport;
  segmentTransport?: SegmentTransport;
  identityReviewTransport?: IdentityReviewTransport;
  miniProgramTransport?: MiniProgramLibraryTransport;
  imageLibraryTransport?: ImageLibraryTransport;
  hxcSenderTransport?: HXCSenderTransport;
  questionnaireTransport?: QuestionnaireListTransport;
  wecomTagsTransport?: WecomTagsTransport;
  channelsTransport?: ChannelsTransport;
  couponsTransport?: CouponsTransport;
  automationRunsTransport?: AutomationRunsTransport;
  groupInviteLibraryTransport?: GroupInviteLibraryTransport;
  cookieHeader: () => string;
  onUnauthenticated: () => void;
}) {
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

  if (route.path === GROUP_INVITE_LIBRARY_PATH) {
    return (
      <GroupInviteLibraryPage
        role={principal.role}
        transport={groupInviteLibraryTransport}
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
  if (base.length > 0 && principal.role === "admin") {
    permitted.add(QUESTIONNAIRE_LIST_PATH);
    permitted.add(AUTOMATION_RUNS_PATH);
  }
  if (
    base.length > 0 &&
    (principal.role === "admin" || principal.role === "ops")
  ) {
    permitted.add(WECOM_TAGS_PATH);
    permitted.add(CHANNELS_PATH);
    permitted.add(COUPONS_PATH);
    permitted.add(GROUP_INVITE_LIBRARY_PATH);
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
  miniProgramTransport,
  imageLibraryTransport,
  hxcSenderTransport,
  questionnaireTransport,
  wecomTagsTransport,
  channelsTransport,
  couponsTransport,
  automationRunsTransport,
  groupInviteLibraryTransport,
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
  const customerID = customerIDForPathname(effectivePathname);

  useEffect(() => {
    if (typeof window === "undefined") return undefined;
    return subscribeToRouteChanges(window, () => {
      setPathname(readPathname());
      setSearch(readSearch());
    });
  }, []);

  useEffect(() => {
    if (initialSession) return undefined;
    let active = true;
    void cache.load().then((result) => {
      if (active) setSession(result);
    });
    return () => {
      active = false;
    };
  }, [cache, initialSession]);

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
            principal={session.principal}
            customerTransport={customerTransport}
            customerDetailTransport={customerDetailTransport}
            customerID={customerID}
            stageTransport={stageTransport}
            segmentTransport={segmentTransport}
            identityReviewTransport={identityReviewTransport}
            miniProgramTransport={miniProgramTransport}
            imageLibraryTransport={imageLibraryTransport}
            hxcSenderTransport={hxcSenderTransport}
            questionnaireTransport={questionnaireTransport}
            wecomTagsTransport={wecomTagsTransport}
            channelsTransport={channelsTransport}
            couponsTransport={couponsTransport}
            automationRunsTransport={automationRunsTransport}
            groupInviteLibraryTransport={groupInviteLibraryTransport}
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
