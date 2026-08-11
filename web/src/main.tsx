/// <reference types="vite/client" />

import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import "./shell.css";

export const ROUTE_CHANGE_EVENT = "aicrm:route-change";

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
}

export function routeForPathname(pathname: string): AppRoute | undefined {
  return routes.find((route) => route.path === pathname);
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

function DefaultNavigation({ pathname }: { pathname: string }) {
  return (
    <ul className="shell-nav__list">
      {routes.map((route) => (
        <li key={route.path}>
          <a
            aria-current={route.path === pathname ? "page" : undefined}
            href={route.path}
            onClick={(event) => handleNavigationClick(event, route.path)}
          >
            {route.navigationLabel}
          </a>
        </li>
      ))}
    </ul>
  );
}

function PageContent({ route }: { route: AppRoute | undefined }) {
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

  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">模块边界</p>
      <h1 id="app-title">{route.title}</h1>
      <p>{route.description}</p>
    </section>
  );
}

export function App({ navigation }: AppProps) {
  const [pathname, setPathname] = useState(readPathname);
  const route = routeForPathname(pathname);

  useEffect(() => {
    if (typeof window === "undefined") return undefined;
    return subscribeToRouteChanges(window, () => setPathname(readPathname()));
  }, []);

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
      </header>
      <div className="shell-frame">
        <nav className="shell-nav" aria-label="主导航">
          {navigation ?? <DefaultNavigation pathname={pathname} />}
        </nav>
        <main id="main-content" className="shell-main" tabIndex={-1}>
          <PageContent route={route} />
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
