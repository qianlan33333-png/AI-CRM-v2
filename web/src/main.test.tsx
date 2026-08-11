import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getHealthz } from "./api/generated/health";
import {
  App,
  LOGIN_PATH,
  ROUTE_CHANGE_EVENT,
  handleNavigationClick,
  navigateTo,
  navigationLinks,
  routeForPathname,
  routeForURL,
  routes,
  shouldInterceptNavigationClick,
  subscribeToRouteChanges,
  type BrowserNavigator,
  type NavigationClick,
} from "./main";

const adminSession = {
  status: "authenticated",
  principal: { adminUserID: 7, role: "admin" },
} as const;

afterEach(() => vi.unstubAllGlobals());

function navigationEvent(
  overrides: Partial<NavigationClick> = {},
): NavigationClick {
  return {
    altKey: false,
    button: 0,
    ctrlKey: false,
    currentTarget: {
      hasAttribute: () => false,
      target: "",
    },
    defaultPrevented: false,
    metaKey: false,
    preventDefault: vi.fn(),
    shiftKey: false,
    ...overrides,
  };
}

function browser(): {
  browser: BrowserNavigator;
  events: EventTarget;
  pushState: ReturnType<typeof vi.fn>;
} {
  const events = new EventTarget();
  const pushState = vi.fn();

  return {
    browser: {
      events,
      history: { pushState },
      location: {
        href: "https://crm.example/customers?tab=active#timeline",
        origin: "https://crm.example",
      },
    },
    events,
    pushState,
  };
}

describe("Web shell routes", () => {
  it("matches only the six frozen pathname routes and renders a 404 for all others", () => {
    expect(routes).toHaveLength(6);

    for (const route of routes) {
      expect(routeForPathname(route.path)).toEqual(route);
    }

    expect(routeForPathname("/customers/")).toBeUndefined();
    expect(routeForPathname("/customer")).toBeUndefined();
    expect(routeForPathname("/settings/security")).toBeUndefined();

    const home = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(home).toContain("AI-CRM 运营指挥台");

    vi.stubGlobal("window", { location: { pathname: "/not-a-route" } });
    const missing = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(missing).toContain("404");
    expect(missing).toContain("未找到页面");
  });

  it("uses pathname only, ignoring query strings and hashes", () => {
    expect(routeForURL("/customers?stage=lead#timeline")).toEqual(
      routeForPathname("/customers"),
    );
    expect(routeForURL("/stages#draft")).toEqual(routeForPathname("/stages"));
    expect(routeForURL("/unknown?from=nav#top")).toBeUndefined();
  });

  it("renders skip navigation, landmarks, one h1, injected navigation, and environment status", () => {
    const html = renderToStaticMarkup(
      <App
        initialSession={adminSession}
        navigation={<a href="/custom">自定义导航</a>}
      />,
    );

    expect(html).toContain('href="#main-content"');
    expect(html).toContain("跳至主要内容");
    expect(html).toContain("<header");
    expect(html).toContain('aria-label="AI-CRM"');
    expect(html).toContain('aria-label="主导航"');
    expect(html).toContain('id="main-content"');
    expect(html).toContain("自定义导航");
    expect(html).toContain('aria-label="环境状态"');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
  });

  it("renders fail-closed login states without a fake provider request", () => {
    const anonymous = renderToStaticMarkup(
      <App initialSession={{ status: "unauthenticated" }} />,
    );
    expect(anonymous).toContain("登录运营工作台");
    expect(anonymous).toContain("企业微信登录（待接入）");
    expect(anonymous).toContain("disabled");
    expect(anonymous).not.toContain("qr");
    expect(anonymous.match(/<h1\b/g)).toHaveLength(1);

    const unavailable = renderToStaticMarkup(
      <App initialSession={{ status: "unavailable" }} />,
    );
    expect(unavailable).toContain("暂时无法确认登录状态");
    expect(unavailable).toContain("重试");
  });

  it("shows only routes allowed by the frozen role table", () => {
    expect(
      navigationLinks(adminSession.principal).map((link) => link.href),
    ).toEqual(["/", "/customers", "/stages", "/settings"]);
    expect(
      navigationLinks({ adminUserID: 8, role: "ops" }).map((link) => link.href),
    ).toEqual(["/", "/customers", "/stages"]);
    expect(
      navigationLinks({ adminUserID: 9, role: "sales", staffID: 11 }).map(
        (link) => link.href,
      ),
    ).toEqual(["/", "/customers", "/stages"]);

    const html = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(html).toContain('href="/settings"');
    expect(html).toContain('href="/stages"');
    expect(html).not.toContain('href="/segments"');
    expect(html).not.toContain('href="/outbound"');
  });

  it("renders an authenticated account view only for the exact login route", () => {
    vi.stubGlobal("window", { location: { pathname: LOGIN_PATH } });
    const login = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(login).toContain("已登录");
    expect(login).toContain("后台账号 #7");
    expect(login).toContain("返回运营工作台");

    vi.stubGlobal("window", { location: { pathname: `${LOGIN_PATH}/` } });
    const nearMiss = renderToStaticMarkup(
      <App initialSession={adminSession} />,
    );
    expect(nearMiss).toContain("404");
  });
});

describe("History API navigation", () => {
  it("intercepts an ordinary same-origin primary click, pushes history, and dispatches a route change", () => {
    const { browser: fakeBrowser, events, pushState } = browser();
    const onRouteChange = vi.fn();
    events.addEventListener(ROUTE_CHANGE_EVENT, onRouteChange);
    const event = navigationEvent();

    expect(
      handleNavigationClick(event, "/stages?view=board#active", fakeBrowser),
    ).toBe(true);
    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(pushState).toHaveBeenCalledWith({}, "", "/stages?view=board#active");
    expect(onRouteChange).toHaveBeenCalledOnce();
  });

  it("preserves modifier, non-primary, target, download, and external link browser behavior", () => {
    const { browser: fakeBrowser, pushState } = browser();
    const variants = [
      navigationEvent({ altKey: true }),
      navigationEvent({ ctrlKey: true }),
      navigationEvent({ metaKey: true }),
      navigationEvent({ shiftKey: true }),
      navigationEvent({ button: 1 }),
      navigationEvent({
        currentTarget: { hasAttribute: () => false, target: "_blank" },
      }),
      navigationEvent({
        currentTarget: {
          hasAttribute: (name) => name === "download",
          target: "",
        },
      }),
    ];

    for (const event of variants) {
      expect(handleNavigationClick(event, "/customers", fakeBrowser)).toBe(
        false,
      );
      expect(event.preventDefault).not.toHaveBeenCalled();
    }

    const external = navigationEvent();
    expect(
      shouldInterceptNavigationClick(
        external,
        "https://outside.example/customers",
        fakeBrowser.location.origin,
      ),
    ).toBe(false);
    expect(
      handleNavigationClick(
        external,
        "https://outside.example/customers",
        fakeBrowser,
      ),
    ).toBe(false);
    expect(external.preventDefault).not.toHaveBeenCalled();
    expect(pushState).not.toHaveBeenCalled();
  });

  it("subscribes to popstate and custom route changes, then cleans both listeners up", () => {
    const events = new EventTarget();
    const onChange = vi.fn();
    const cleanup = subscribeToRouteChanges(events, onChange);

    events.dispatchEvent(new Event("popstate"));
    events.dispatchEvent(new Event(ROUTE_CHANGE_EVENT));
    expect(onChange).toHaveBeenCalledTimes(2);

    cleanup();
    events.dispatchEvent(new Event("popstate"));
    events.dispatchEvent(new Event(ROUTE_CHANGE_EVENT));
    expect(onChange).toHaveBeenCalledTimes(2);
  });

  it("does not dispatch a route change for an invalid or external history destination", () => {
    const { browser: fakeBrowser, events, pushState } = browser();
    const onRouteChange = vi.fn();
    events.addEventListener(ROUTE_CHANGE_EVENT, onRouteChange);

    expect(navigateTo("https://outside.example", fakeBrowser)).toBe(false);
    expect(navigateTo("http://[", fakeBrowser)).toBe(false);
    expect(pushState).not.toHaveBeenCalled();
    expect(onRouteChange).not.toHaveBeenCalled();
  });
});

describe("generated health contract", () => {
  it("uses the generated health client contract", async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        expect(input).toBe("/healthz");
        expect(init).toEqual({ method: "GET" });
        return new Response('{"status":"ok"}', { status: 200 });
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await getHealthz();

    expect(response.status).toBe(200);
    expect(response.data).toEqual({ status: "ok" });
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
