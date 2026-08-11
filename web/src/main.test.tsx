import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getHealthz } from "./api/generated/health";
import {
  App,
  ROUTE_CHANGE_EVENT,
  handleNavigationClick,
  navigateTo,
  routeForPathname,
  routeForURL,
  routes,
  shouldInterceptNavigationClick,
  subscribeToRouteChanges,
  type BrowserNavigator,
  type NavigationClick,
} from "./main";

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

    const home = renderToStaticMarkup(<App />);
    expect(home).toContain("AI-CRM 运营指挥台");

    vi.stubGlobal("window", { location: { pathname: "/not-a-route" } });
    const missing = renderToStaticMarkup(<App />);
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
      <App navigation={<a href="/custom">自定义导航</a>} />,
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
