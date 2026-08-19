import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getHealthz } from "./api/generated/health";
import {
  App,
  IMAGE_LIBRARY_PATH,
  HXC_SENDER_PATH,
  QUESTIONNAIRE_LIST_PATH,
  WECOM_TAGS_PATH,
  CHANNELS_PATH,
  COUPONS_PATH,
  AUTOMATION_RUNS_PATH,
  LOGIN_PATH,
  MINIPROGRAM_LIBRARY_PATH,
  ROUTE_CHANGE_EVENT,
  carrierPathname,
  customerIDForPathname,
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
import type { CustomerDetailTransport } from "./customer-detail";
import type { ImageLibraryTransport } from "./image-library";
import type { WecomTagsTransport } from "./wecom-tags";
import type { ChannelsTransport } from "./channels";
import type { CouponsTransport } from "./coupons";
import type { AutomationRunsTransport } from "./automation-runs";

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

function customerDetailTransport(): CustomerDetailTransport {
  const response = async () => ({ status: 503, data: undefined });
  return {
    get: vi.fn(response),
    update: vi.fn(response),
    setStage: vi.fn(response),
    addTag: vi.fn(response),
    removeTag: vi.fn(response),
    listEvents: vi.fn(response),
    listTags: vi.fn(response),
  };
}

function imageLibraryTransport(): ImageLibraryTransport {
  const response = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(response),
    facets: vi.fn(response),
    upload: vi.fn(response),
  } as unknown as ImageLibraryTransport;
}

function wecomTagsTransport(): WecomTagsTransport {
  const response = async () => ({ status: 503, data: {} });
  return { read: vi.fn(response) } as unknown as WecomTagsTransport;
}

function channelsTransport(): ChannelsTransport {
  const response = async () => ({ status: 503, data: {} });
  return { read: vi.fn(response) } as unknown as ChannelsTransport;
}

function couponsTransport(): CouponsTransport {
  const response = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(response),
    copy: vi.fn(response),
  } as unknown as CouponsTransport;
}

function automationRunsTransport(): AutomationRunsTransport {
  const response = async () => ({ status: 503, data: {} });
  return { list: vi.fn(response) } as AutomationRunsTransport;
}

describe("Web shell routes", () => {
  it("accepts only an exact positive safe OneID customer-detail pathname", () => {
    expect(customerIDForPathname("/customers/1")).toBe(1);
    expect(customerIDForPathname("/customers/42")).toBe(42);
    expect(customerIDForPathname("/customers/9007199254740991")).toBe(
      Number.MAX_SAFE_INTEGER,
    );

    for (const pathname of [
      "/customers/0",
      "/customers/-1",
      "/customers/01",
      "/customers/1.0",
      "/customers/9007199254740992",
      "/customers/1/",
      "/customers/1/timeline",
      "/customers/1?tab=timeline",
      "/customer/1",
    ]) {
      expect(customerIDForPathname(pathname), pathname).toBeUndefined();
    }
  });

  it("renders the injected customer-detail route and keeps Customers navigation active", () => {
    vi.stubGlobal("window", { location: { pathname: "/customers/42" } });
    const detailTransport = customerDetailTransport();
    const readCookie = vi.fn(() => "aicrm_csrf=test-token");

    const html = renderToStaticMarkup(
      <App
        customerDetailTransport={detailTransport}
        cookieHeader={readCookie}
        initialSession={adminSession}
      />,
    );

    expect(html).toContain('<h1 id="app-title">\u5ba2\u6237\u8be6\u60c5</h1>');
    expect(html).toContain(
      "\u6b63\u5728\u8bfb\u53d6\u5ba2\u6237\u8d44\u6599\u3001\u6807\u7b7e\u548c\u65f6\u95f4\u7ebf",
    );
    expect(html).toMatch(
      /<a aria-current="page" href="\/customers">\u5ba2\u6237<\/a>/,
    );
    expect(html).not.toContain("\u672a\u627e\u5230\u9875\u9762");
  });

  it("renders the injected image-library route and keeps sales fail-closed", () => {
    vi.stubGlobal("window", { location: { pathname: "/admin/image-library" } });
    const client = imageLibraryTransport();
    const html = renderToStaticMarkup(
      <App imageLibraryTransport={client} initialSession={adminSession} />,
    );
    expect(html).toContain('<h1 id="app-title">图片素材库</h1>');
    expect(html).toContain("搜索图片");
    expect(html).toMatch(
      /<a aria-current="page" href="\/admin\/image-library">图片素材<\/a>/,
    );

    const sales = renderToStaticMarkup(
      <App
        imageLibraryTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 9, role: "sales", staffID: 11 },
        }}
      />,
    );
    expect(sales).toContain("当前账号没有图片素材库访问权限。");
    expect(sales).not.toContain("搜索图片");
    expect(sales).not.toContain('href="/admin/image-library"');
    expect(client.list).not.toHaveBeenCalled();
    expect(client.facets).not.toHaveBeenCalled();
  });

  it("renders the WeCom tags route for global admin and ops only", () => {
    vi.stubGlobal("window", { location: { pathname: WECOM_TAGS_PATH } });
    const client = wecomTagsTransport();
    const admin = renderToStaticMarkup(
      <App wecomTagsTransport={client} initialSession={adminSession} />,
    );
    expect(admin).toContain('<h1 id="app-title">企微标签目录</h1>');
    expect(admin).toContain("正在读取企微标签目录。");
    const ops = renderToStaticMarkup(
      <App
        wecomTagsTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 8, role: "ops" },
        }}
      />,
    );
    expect(ops).toContain("正在读取企微标签目录。");
    const sales = renderToStaticMarkup(
      <App
        wecomTagsTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 9, role: "sales", staffID: 11 },
        }}
      />,
    );
    expect(sales).toContain("当前账号没有企微标签目录访问权限。");
    expect(client.read).not.toHaveBeenCalled();
  });

  it("renders the channel route for global admin and ops only", () => {
    vi.stubGlobal("window", { location: { pathname: CHANNELS_PATH } });
    const client = channelsTransport();
    const admin = renderToStaticMarkup(
      <App channelsTransport={client} initialSession={adminSession} />,
    );
    expect(admin).toContain('<h1 id="app-title">渠道列表</h1>');
    expect(admin).toContain("正在读取本地渠道列表。");
    const ops = renderToStaticMarkup(
      <App
        channelsTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 8, role: "ops" },
        }}
      />,
    );
    expect(ops).toContain("正在读取本地渠道列表。");
    const sales = renderToStaticMarkup(
      <App
        channelsTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 9, role: "sales", staffID: 11 },
        }}
      />,
    );
    expect(sales).toContain("当前账号没有渠道列表访问权限。");
    expect(client.read).not.toHaveBeenCalled();
  });

  it("renders the coupon carrier route for admin and ops only", () => {
    vi.stubGlobal("window", { location: { pathname: COUPONS_PATH } });
    const client = couponsTransport();
    expect(
      renderToStaticMarkup(
        <App couponsTransport={client} initialSession={adminSession} />,
      ),
    ).toContain("正在读取本地优惠券列表。");
    expect(
      renderToStaticMarkup(
        <App
          couponsTransport={client}
          initialSession={{
            status: "authenticated",
            principal: { adminUserID: 8, role: "ops" },
          }}
        />,
      ),
    ).toContain("正在读取本地优惠券列表。");
    expect(
      renderToStaticMarkup(
        <App
          couponsTransport={client}
          initialSession={{
            status: "authenticated",
            principal: { adminUserID: 9, role: "sales", staffID: 11 },
          }}
        />,
      ),
    ).toContain("当前账号没有优惠券管理权限。");
    expect(client.list).not.toHaveBeenCalled();
  });

  it("renders the questionnaire carrier route for admin and keeps non-admins fail-closed", () => {
    vi.stubGlobal("window", {
      location: { pathname: QUESTIONNAIRE_LIST_PATH },
    });
    expect(
      renderToStaticMarkup(<App initialSession={adminSession} />),
    ).toContain("正在读取问卷列表");
    expect(
      renderToStaticMarkup(
        <App
          initialSession={{
            status: "authenticated",
            principal: { adminUserID: 8, role: "ops" },
          }}
        />,
      ),
    ).toContain("当前账号没有问卷管理权限。");
    expect(
      renderToStaticMarkup(
        <App
          initialSession={{
            status: "authenticated",
            principal: { adminUserID: 9, role: "sales", staffID: 11 },
          }}
        />,
      ),
    ).toContain("当前账号没有问卷管理权限。");
  });

  it("renders the automation receipts route for admin and keeps non-admins fail-closed", () => {
    vi.stubGlobal("window", { location: { pathname: AUTOMATION_RUNS_PATH } });
    const client = automationRunsTransport();
    expect(
      renderToStaticMarkup(
        <App automationRunsTransport={client} initialSession={adminSession} />,
      ),
    ).toContain("正在读取自动化运行记录。");
    for (const role of ["ops", "sales"] as const) {
      expect(
        renderToStaticMarkup(
          <App
            automationRunsTransport={client}
            initialSession={{
              status: "authenticated",
              principal: { adminUserID: 8, role },
            }}
          />,
        ),
      ).toContain("当前账号没有自动化运行记录访问权限。");
    }
    expect(client.list).not.toHaveBeenCalled();
  });

  it("matches only the frozen pathname routes and renders a 404 for all others", () => {
    expect(routes).toHaveLength(15);

    for (const route of routes) {
      expect(routeForPathname(route.path)).toEqual(route);
    }

    expect(routeForPathname("/customers/")).toBeUndefined();
    expect(routeForPathname("/customer")).toBeUndefined();
    expect(routeForPathname("/settings/security")).toBeUndefined();

    const home = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(home).toContain("AI-CRM 运营指挥台");

    vi.stubGlobal("window", { location: { pathname: "/stages" } });
    const stages = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(stages).toContain("阶段管理");
    expect(stages).not.toContain("P2-17 将在此接入真实能力");

    vi.stubGlobal("window", { location: { pathname: "/customers" } });
    const customers = renderToStaticMarkup(
      <App initialSession={adminSession} />,
    );
    expect(customers).toContain("客户列表");
    expect(customers).toContain("筛选条件");
    expect(customers).not.toContain("客户模块边界已预留");

    vi.stubGlobal("window", { location: { pathname: "/segments" } });
    const segments = renderToStaticMarkup(
      <App initialSession={adminSession} />,
    );
    expect(segments).toContain("人群包列表");
    expect(segments).toContain("条件编辑器");
    expect(segments).not.toContain("人群包模块边界已预留");

    vi.stubGlobal("window", {
      location: { pathname: "/identity/merge-reviews" },
    });
    const identityReviews = renderToStaticMarkup(
      <App initialSession={adminSession} />,
    );
    expect(identityReviews).toContain("待合并列表");
    expect(identityReviews).toContain("审阅与决策");
    expect(identityReviews).not.toContain("模块边界");

    vi.stubGlobal("window", {
      location: { pathname: "/admin/miniprogram-library" },
    });
    const miniProgramLibrary = renderToStaticMarkup(
      <App initialSession={adminSession} />,
    );
    expect(miniProgramLibrary).toContain("小程序素材库");
    expect(miniProgramLibrary).toContain("素材列表");
    expect(miniProgramLibrary).not.toContain("模块边界");

    vi.stubGlobal("window", { location: { pathname: "/admin/image-library" } });
    const imageLibrary = renderToStaticMarkup(
      <App initialSession={adminSession} />,
    );
    expect(imageLibrary).toContain('<h1 id="app-title">图片素材库</h1>');
    expect(imageLibrary).toContain("图片列表");
    expect(imageLibrary).toContain("上传图片");
    expect(imageLibrary).not.toContain("模块边界");

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
    ).toEqual([
      "/",
      "/customers",
      "/stages",
      "/segments",
      "/identity/merge-reviews",
      "/admin/miniprogram-library",
      "/admin/image-library",
      "/admin/hxc-send-config",
      "/admin/questionnaires",
      "/admin/wecom-tags",
      "/admin/channels",
      "/admin/coupons",
      "/admin/automation-runs",
      "/settings",
    ]);
    expect(
      navigationLinks({ adminUserID: 8, role: "ops" }).map((link) => link.href),
    ).toEqual([
      "/",
      "/customers",
      "/stages",
      "/segments",
      "/identity/merge-reviews",
      "/admin/miniprogram-library",
      "/admin/image-library",
      "/admin/wecom-tags",
      "/admin/channels",
      "/admin/coupons",
    ]);
    expect(
      navigationLinks({ adminUserID: 9, role: "sales", staffID: 11 }).map(
        (link) => link.href,
      ),
    ).toEqual(["/", "/customers", "/stages"]);

    const html = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(html).toContain('href="/settings"');
    expect(html).toContain('href="/stages"');
    expect(html).toContain('href="/segments"');
    expect(html).toContain('href="/identity/merge-reviews"');
    expect(html).toContain('href="/admin/miniprogram-library"');
    expect(html).toContain('href="/admin/image-library"');
    expect(html).toContain('href="/admin/questionnaires"');
    expect(html).toContain('href="/admin/wecom-tags"');
    expect(html).toContain('href="/admin/channels"');
    expect(html).toContain('href="/admin/coupons"');
    expect(html).toContain('href="/admin/automation-runs"');
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

describe("legacy admin path carrier", () => {
  it("maps only the exact frozen carrier value to the MiniProgram route", () => {
    expect(
      carrierPathname("/", "?legacy_admin_path=%2Fadmin%2Fminiprogram-library"),
    ).toBe(MINIPROGRAM_LIBRARY_PATH);
    expect(carrierPathname("/", `?legacy_admin_path=${HXC_SENDER_PATH}`)).toBe(
      HXC_SENDER_PATH,
    );
    expect(
      carrierPathname("/", `?legacy_admin_path=${MINIPROGRAM_LIBRARY_PATH}`),
    ).toBe(MINIPROGRAM_LIBRARY_PATH);
    expect(
      carrierPathname("/", `?legacy_admin_path=${QUESTIONNAIRE_LIST_PATH}`),
    ).toBe(QUESTIONNAIRE_LIST_PATH);
    expect(carrierPathname("/", `?legacy_admin_path=${WECOM_TAGS_PATH}`)).toBe(
      WECOM_TAGS_PATH,
    );
    expect(carrierPathname("/", `?legacy_admin_path=${CHANNELS_PATH}`)).toBe(
      CHANNELS_PATH,
    );
    expect(carrierPathname("/", `?legacy_admin_path=${COUPONS_PATH}`)).toBe(
      COUPONS_PATH,
    );

    for (const search of [
      "?legacy_admin_path=/admin/image-library",
      "?legacy_admin_path=/admin/wecom-tags/extra",
      "?legacy_admin_path=/admin/channels/extra",
      "?legacy_admin_path=/admin/coupons/extra",
      "?legacy_admin_path=/admin/wecom-tags&legacy_admin_path=/admin/wecom-tags",
      `?legacy_admin_path=${IMAGE_LIBRARY_PATH}`,
      `?legacy_admin_path=${HXC_SENDER_PATH}&legacy_admin_path=${HXC_SENDER_PATH}`,
      "?legacy_admin_path=https://evil.example",
      "?legacy_admin_path=//evil.example",
      "?legacy_admin_path=/admin/miniprogram-library/extra",
      "?legacy_admin_path=/admin/miniprogram-library&legacy_admin_path=/admin/miniprogram-library",
      "?other=/admin/miniprogram-library",
      "",
    ]) {
      expect(carrierPathname("/", search), search).toBe("/");
    }
    expect(
      carrierPathname(
        "/customers",
        "?legacy_admin_path=%2Fadmin%2Fminiprogram-library",
      ),
    ).toBe("/customers");
  });

  it("lands on the MiniProgram page after a carrier refresh without falling back to the overview", () => {
    vi.stubGlobal("window", {
      location: {
        pathname: "/",
        search: "?legacy_admin_path=%2Fadmin%2Fminiprogram-library",
      },
    });
    const html = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(html).toContain('<h1 id="app-title">小程序素材库</h1>');
    expect(html).toContain("素材列表");
    expect(html).not.toContain("运营工作台骨架已就绪");
    expect(html).toMatch(
      /<a aria-current="page" href="\/admin\/miniprogram-library">小程序素材<\/a>/,
    );
  });

  it("lands on the WeCom tags page after a carrier refresh", () => {
    vi.stubGlobal("window", {
      location: {
        pathname: "/",
        search: "?legacy_admin_path=%2Fadmin%2Fwecom-tags",
      },
    });
    const html = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(html).toContain('<h1 id="app-title">企微标签目录</h1>');
    expect(html).not.toContain("AI-CRM 运营指挥台");
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
