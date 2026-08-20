import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getHealthz } from "./api/generated/health";
import {
  App,
  IDENTITY_CONSOLE_PATH,
  IMAGE_LIBRARY_PATH,
  HXC_SENDER_PATH,
  QUESTIONNAIRE_LIST_PATH,
  WECOM_TAGS_PATH,
  CHANNELS_PATH,
  COUPONS_PATH,
  AUTOMATION_RUNS_PATH,
  AUTOMATION_AGENTS_PATH,
  GROUP_INVITE_LIBRARY_PATH,
  DELIVERY_LINEAGE_PATH,
  DATA_HEALTH_PATH,
  EXECUTION_RUNTIME_PATH,
  ORDERS_PATH,
  OUTBOUND_PATH,
  PRODUCTS_PATH,
  LOGIN_PATH,
  MINIPROGRAM_LIBRARY_PATH,
  ROUTE_CHANGE_EVENT,
  carrierPathname,
  customerIDForPathname,
  handleNavigationClick,
  navigateTo,
  navigationLinks,
  publicSurveySlug,
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
import type { AutomationAgentsTransport } from "./automation-agents";
import type { GroupInviteLibraryTransport } from "./group-invite-library";
import type { DeliveryLineageTransport } from "./delivery-lineage";
import type { DataHealthTransport } from "./data-health";
import type { OrdersTransport } from "./orders";
import type { AppSettingsTransport } from "./app-settings";
import type { PublicSurveyTransport } from "./public-survey";
import {
  ALIPAY_TRANSACTIONS_PATH,
  SERVICE_PRODUCT_NEW_PATH,
  SERVICE_PRODUCTS_PATH,
  WECHAT_PAY_TRANSACTIONS_PATH,
  WECHAT_SHOP_TRANSACTIONS_PATH,
} from "./commerce-workspaces";
import { GROUP_OPS_PLANS_PATH } from "./group-ops";
import { AUDIENCE_PACKAGES_PATH } from "./audience-packages";

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

function automationAgentsTransport(): AutomationAgentsTransport {
  const response = async () => ({ status: 503, data: {} });
  return { read: vi.fn(response) } as AutomationAgentsTransport;
}

function groupInviteLibraryTransport(): GroupInviteLibraryTransport {
  const response = async () => ({ status: 503, data: {} });
  return { list: vi.fn(response) } as GroupInviteLibraryTransport;
}

function deliveryLineageTransport(): DeliveryLineageTransport {
  const response = async () => ({ status: 503, data: {} });
  return { list: vi.fn(response) } as DeliveryLineageTransport;
}

function ordersTransport(): OrdersTransport {
  const response = async () => ({ status: 503, data: {} });
  return { list: vi.fn(response) } as unknown as OrdersTransport;
}

function appSettingsTransport(): AppSettingsTransport {
  const response = async () => ({ status: 503, data: {} });
  return {
    read: vi.fn(response),
    save: vi.fn(response),
  } as unknown as AppSettingsTransport;
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

  it("renders only the strict public-survey root carrier before authentication", () => {
    expect(publicSurveySlug("?public_survey_slug=summer-2026")).toBe(
      "summer-2026",
    );
    for (const search of [
      "",
      "?public_survey_slug=",
      "?public_survey_slug=Summer",
      "?public_survey_slug=survey%2Fother",
      "?public_survey_slug=one&public_survey_slug=two",
      "?public_survey_slug=one&result_token=secret",
      "?result_token=secret",
      "?legacy_admin_path=%2Fadmin%2Fquestionnaires",
    ]) {
      expect(publicSurveySlug(search), search).toBeUndefined();
    }

    vi.stubGlobal("window", {
      location: { pathname: "/", search: "?public_survey_slug=summer-2026" },
    });
    const client: PublicSurveyTransport = {
      definition: vi.fn(),
      submit: vi.fn(),
      result: vi.fn(),
    };
    const html = renderToStaticMarkup(
      <App
        initialSession={{ status: "unauthenticated" }}
        publicSurveyTransport={client}
      />,
    );
    expect(html).toContain("正在加载问卷");
    expect(html).not.toContain("登录运营工作台");
    expect(html).not.toContain("result_token");
    expect(client.definition).not.toHaveBeenCalled();
    expect(client.submit).not.toHaveBeenCalled();
    expect(client.result).not.toHaveBeenCalled();

    vi.stubGlobal("window", {
      location: {
        pathname: "/",
        search: "?public_survey_slug=summer&result_token=x",
      },
    });
    expect(
      renderToStaticMarkup(
        <App initialSession={{ status: "unauthenticated" }} />,
      ),
    ).toContain("登录运营工作台");
    vi.stubGlobal("window", {
      location: { pathname: "/q/summer-2026", search: "" },
    });
    expect(
      renderToStaticMarkup(
        <App initialSession={{ status: "unauthenticated" }} />,
      ),
    ).toContain("登录运营工作台");
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

  it("renders the group-invite library for admin and ops while sales remains inert", () => {
    vi.stubGlobal("window", {
      location: { pathname: GROUP_INVITE_LIBRARY_PATH },
    });
    const client = groupInviteLibraryTransport();
    const admin = renderToStaticMarkup(
      <App
        groupInviteLibraryTransport={client}
        initialSession={adminSession}
      />,
    );
    expect(admin).toContain('<h1 id="app-title">群邀请素材库</h1>');
    expect(admin).toContain("正在读取本地群邀请素材。");
    expect(admin).toContain(`href="${GROUP_INVITE_LIBRARY_PATH}"`);
    const ops = renderToStaticMarkup(
      <App
        groupInviteLibraryTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 8, role: "ops" },
        }}
      />,
    );
    expect(ops).toContain("正在读取本地群邀请素材。");
    const sales = renderToStaticMarkup(
      <App
        groupInviteLibraryTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 9, role: "sales", staffID: 11 },
        }}
      />,
    );
    expect(sales).toContain("当前账号没有群邀请素材库访问权限。");
    expect(sales).not.toContain(`href="${GROUP_INVITE_LIBRARY_PATH}"`);
    expect(client.list).not.toHaveBeenCalled();
  });

  it("renders delivery lineage only for admin while ops and sales remain inert", () => {
    vi.stubGlobal("window", { location: { pathname: DELIVERY_LINEAGE_PATH } });
    const client = deliveryLineageTransport();
    const admin = renderToStaticMarkup(
      <App deliveryLineageTransport={client} initialSession={adminSession} />,
    );
    expect(admin).toContain('<h1 id="app-title">投递处理谱系</h1>');
    expect(admin).toContain("正在读取投递处理谱系。");
    expect(admin).toContain(`href="${DELIVERY_LINEAGE_PATH}"`);
    for (const role of ["ops", "sales"] as const) {
      const html = renderToStaticMarkup(
        <App
          deliveryLineageTransport={client}
          initialSession={{
            status: "authenticated",
            principal: { adminUserID: 8, role },
          }}
        />,
      );
      expect(html).toContain("当前账号没有投递处理谱系访问权限。");
      expect(html).not.toContain(`href="${DELIVERY_LINEAGE_PATH}"`);
    }
    expect(client.list).not.toHaveBeenCalled();
  });

  it("renders the WeCom tags route for global admin and ops only", () => {
    vi.stubGlobal("window", { location: { pathname: WECOM_TAGS_PATH } });
    const client = wecomTagsTransport();
    const admin = renderToStaticMarkup(
      <App wecomTagsTransport={client} initialSession={adminSession} />,
    );
    expect(admin).toContain('<h1 id="app-title">企微标签目录</h1>');
    expect(admin).toContain("正在读取企微标签目录。");
    expect(admin).toContain(
      '<h2 id="wecom-callback-audit-title">企微回调本地审计</h2>',
    );
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
    expect(ops).not.toContain("企微回调本地审计");
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

  it("renders the questionnaire carrier route for admin and ops while sales remains fail-closed", () => {
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
    ).toContain("正在读取问卷列表");
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

  it("renders the automation-agent carrier route only for admin", () => {
    vi.stubGlobal("window", { location: { pathname: AUTOMATION_AGENTS_PATH } });
    const client = automationAgentsTransport();
    expect(
      renderToStaticMarkup(
        <App
          automationAgentsTransport={client}
          initialSession={adminSession}
        />,
      ),
    ).toContain("正在读取本地自动化话术摘要。");
    for (const role of ["ops", "sales"] as const) {
      const html = renderToStaticMarkup(
        <App
          automationAgentsTransport={client}
          initialSession={{
            status: "authenticated",
            principal: { adminUserID: 8, role },
          }}
        />,
      );
      expect(html).toContain("当前账号没有自动化话术目录访问权限。");
      expect(html).not.toContain(`href="${AUTOMATION_AGENTS_PATH}"`);
    }
    expect(client.read).not.toHaveBeenCalled();
  });

  it("renders the data-health route for admin and keeps non-admins fail-closed", () => {
    vi.stubGlobal("window", { location: { pathname: DATA_HEALTH_PATH } });
    const client = {
      list: vi.fn(),
      summary: vi.fn(),
      detail: vi.fn(),
    } as unknown as DataHealthTransport;
    expect(
      renderToStaticMarkup(
        <App dataHealthTransport={client} initialSession={adminSession} />,
      ),
    ).toContain("正在读取本地数据健康观测。");
    for (const role of ["ops", "sales"] as const) {
      expect(
        renderToStaticMarkup(
          <App
            dataHealthTransport={client}
            initialSession={{
              status: "authenticated",
              principal: { adminUserID: 8, role },
            }}
          />,
        ),
      ).toContain("当前账号没有数据健康访问权限。");
    }
    expect(client.list).not.toHaveBeenCalled();
    expect(client.summary).not.toHaveBeenCalled();
    expect(client.detail).not.toHaveBeenCalled();
  });

  it("renders the local order overview for admin and ops while sales remains inert", () => {
    vi.stubGlobal("window", { location: { pathname: ORDERS_PATH } });
    const client = ordersTransport();
    for (const role of ["admin", "ops"] as const) {
      expect(
        renderToStaticMarkup(
          <App
            ordersTransport={client}
            initialSession={{
              status: "authenticated",
              principal: { adminUserID: 8, role },
            }}
          />,
        ),
      ).toContain("正在读取本地订单总览。");
    }
    const sales = renderToStaticMarkup(
      <App
        ordersTransport={client}
        initialSession={{
          status: "authenticated",
          principal: { adminUserID: 9, role: "sales", staffID: 11 },
        }}
      />,
    );
    expect(sales).toContain("当前账号没有订单总览访问权限。");
    expect(sales).not.toContain(`href="${ORDERS_PATH}"`);
    expect(client.list).not.toHaveBeenCalled();
  });

  it("connects the admin-only settings route to the local settings page", () => {
    vi.stubGlobal("window", { location: { pathname: "/settings" } });
    const client = appSettingsTransport();
    const admin = renderToStaticMarkup(
      <App appSettingsTransport={client} initialSession={adminSession} />,
    );
    expect(admin).toContain("本地应用设置");
    expect(admin).toContain("正在读取本地应用设置");

    for (const role of ["ops", "sales"] as const) {
      const html = renderToStaticMarkup(
        <App
          appSettingsTransport={client}
          initialSession={{
            status: "authenticated",
            principal:
              role === "sales"
                ? { adminUserID: 9, role, staffID: 11 }
                : { adminUserID: 8, role },
          }}
        />,
      );
      expect(html).toContain("当前账号没有本地应用设置管理权限");
      expect(html).not.toContain('href="/settings"');
    }
    expect(client.read).not.toHaveBeenCalled();
    expect(client.save).not.toHaveBeenCalled();
  });

  it("connects the admin-only AI assistant carrier to the local read-only workspace", () => {
    vi.stubGlobal("window", {
      location: { pathname: "/admin/cloud-orchestrator/plans" },
    });
    const admin = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(admin).toContain("运营计划审阅");
    expect(admin).toContain("不表示 Provider 已调用");
    expect(admin).toContain('href="/admin/cloud-orchestrator/plans"');

    for (const role of ["ops", "sales"] as const) {
      const html = renderToStaticMarkup(
        <App
          initialSession={{
            status: "authenticated",
            principal:
              role === "sales"
                ? { adminUserID: 9, role, staffID: 11 }
                : { adminUserID: 8, role },
          }}
        />,
      );
      expect(html).toContain("当前账号没有 AI 助手本地审阅权限");
      expect(html).not.toContain('href="/admin/cloud-orchestrator/plans"');
    }
  });

  it("connects the admin-only group-operations carriers to the safe local workspace", () => {
    vi.stubGlobal("window", {
      location: {
        pathname: "/admin/automation-conversion/group-ops/plans/42",
        search: "",
      },
    });
    const admin = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(admin).toContain("群运营计划明细");
    expect(admin).toContain("<dd>42</dd>");
    expect(admin).toContain("不表示 Provider 已调用");
    expect(admin).not.toContain("创建群运营计划");
    expect(admin).not.toContain("同步运营人员");
    expect(admin).not.toContain("启动到期执行");

    for (const role of ["ops", "sales"] as const) {
      const html = renderToStaticMarkup(
        <App
          initialSession={{
            status: "authenticated",
            principal:
              role === "sales"
                ? { adminUserID: 9, role, staffID: 11 }
                : { adminUserID: 8, role },
          }}
        />,
      );
      expect(html).toContain("当前账号没有群运营本地工作区权限");
      expect(html).not.toContain("<dd>42</dd>");
    }
  });

  it("connects the admin-only audience-package carriers to the safe local workspace", () => {
    vi.stubGlobal("window", {
      location: {
        pathname: "/admin/automation-conversion/packages/9007199254740993",
        search: "",
      },
    });
    const admin = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(admin).toContain("AI Audience 人群包明细");
    expect(admin).toContain("9007199254740993");
    expect(admin).toContain("不证明发送权限");
    expect(admin).not.toContain("群发入口");

    for (const role of ["ops", "sales"] as const) {
      const html = renderToStaticMarkup(
        <App
          initialSession={{
            status: "authenticated",
            principal:
              role === "sales"
                ? { adminUserID: 9, role, staffID: 11 }
                : { adminUserID: 8, role },
          }}
        />,
      );
      expect(html).toContain("当前角色无权访问此工作区");
      expect(html).not.toContain("9007199254740993");
    }
  });

  it("mounts commerce workspaces and keeps payment and refund effects closed", () => {
    vi.stubGlobal("window", {
      location: {
        pathname: `${WECHAT_PAY_TRANSACTIONS_PATH}/order%20one`,
        search: "",
      },
    });
    const admin = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(admin).toContain("微信支付交易详情");
    expect(admin).toContain("order one");
    expect(admin).toContain("不提供退款、重试或 Provider 操作");
    expect(admin).not.toContain("提交退款");

    for (const role of ["ops", "sales"] as const) {
      const html = renderToStaticMarkup(
        <App
          initialSession={{
            status: "authenticated",
            principal:
              role === "sales"
                ? { adminUserID: 9, role, staffID: 11 }
                : { adminUserID: 8, role },
          }}
        />,
      );
      expect(html).toContain("没有交易与周期商品工作区权限");
      expect(html).not.toContain("order one");
    }
  });

  it("matches only the frozen pathname routes and renders a 404 for all others", () => {
    expect(routes).toHaveLength(35);

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

    vi.stubGlobal("window", { location: { pathname: IDENTITY_CONSOLE_PATH } });
    const identityConsole = renderToStaticMarkup(
      <App initialSession={adminSession} />,
    );
    expect(identityConsole).toContain("本地身份控制台");
    expect(identityConsole).toContain("不触发 Provider、外发或自动合并");

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
      IDENTITY_CONSOLE_PATH,
      "/identity/merge-reviews",
      "/admin/miniprogram-library",
      "/admin/image-library",
      "/admin/hxc-send-config",
      "/admin/questionnaires",
      "/admin/wecom-tags",
      "/admin/channels",
      "/admin/coupons",
      "/admin/automation-runs",
      AUTOMATION_AGENTS_PATH,
      "/admin/cloud-orchestrator/plans",
      GROUP_OPS_PLANS_PATH,
      AUDIENCE_PACKAGES_PATH,
      SERVICE_PRODUCTS_PATH,
      WECHAT_PAY_TRANSACTIONS_PATH,
      WECHAT_SHOP_TRANSACTIONS_PATH,
      ALIPAY_TRANSACTIONS_PATH,
      "/admin/group-invite-library",
      "/admin/delivery-lineage",
      "/admin/data-health",
      "/admin/execution-runtime",
      "/admin/orders",
      PRODUCTS_PATH,
      "/outbound",
      "/settings",
    ]);
    expect(
      navigationLinks({ adminUserID: 8, role: "ops" }).map((link) => link.href),
    ).toEqual([
      "/",
      "/customers",
      "/stages",
      "/segments",
      IDENTITY_CONSOLE_PATH,
      "/identity/merge-reviews",
      "/admin/miniprogram-library",
      "/admin/image-library",
      QUESTIONNAIRE_LIST_PATH,
      "/admin/wecom-tags",
      "/admin/channels",
      "/admin/coupons",
      "/admin/group-invite-library",
      "/admin/orders",
      PRODUCTS_PATH,
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
    expect(html).toContain(`href="${IDENTITY_CONSOLE_PATH}"`);
    expect(html).toContain('href="/identity/merge-reviews"');
    expect(html).toContain('href="/admin/miniprogram-library"');
    expect(html).toContain('href="/admin/image-library"');
    expect(html).toContain('href="/admin/questionnaires"');
    expect(html).toContain('href="/admin/wecom-tags"');
    expect(html).toContain('href="/admin/channels"');
    expect(html).toContain('href="/admin/coupons"');
    expect(html).toContain('href="/admin/automation-runs"');
    expect(html).toContain(`href="${AUTOMATION_AGENTS_PATH}"`);
    expect(html).toContain('href="/admin/cloud-orchestrator/plans"');
    expect(html).toContain(`href="${GROUP_OPS_PLANS_PATH}"`);
    expect(html).toContain(`href="${AUDIENCE_PACKAGES_PATH}"`);
    expect(html).toContain(`href="${SERVICE_PRODUCTS_PATH}"`);
    expect(html).toContain(`href="${WECHAT_PAY_TRANSACTIONS_PATH}"`);
    expect(html).toContain(`href="${WECHAT_SHOP_TRANSACTIONS_PATH}"`);
    expect(html).toContain(`href="${ALIPAY_TRANSACTIONS_PATH}"`);
    expect(html).toContain('href="/admin/group-invite-library"');
    expect(html).toContain('href="/admin/delivery-lineage"');
    expect(html).toContain('href="/admin/data-health"');
    expect(html).toContain(`href="${EXECUTION_RUNTIME_PATH}"`);
    expect(html).toContain(`href="${ORDERS_PATH}"`);
    expect(html).toContain(`href="${OUTBOUND_PATH}"`);
    expect(html).toContain(`href="${PRODUCTS_PATH}"`);
  });

  it("composes the Push Center sections with local task observation at outbound", () => {
    vi.stubGlobal("window", {
      location: { pathname: OUTBOUND_PATH, search: "" },
    });

    const html = renderToStaticMarkup(<App initialSession={adminSession} />);

    expect(html).toContain("推送中心");
    expect(html).toContain("仅展示本地分区聚合计数");
    expect(html).toContain("投递任务观察与本地对账");
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
    expect(
      carrierPathname("/", `?legacy_admin_path=${AUTOMATION_AGENTS_PATH}`),
    ).toBe(AUTOMATION_AGENTS_PATH);
    expect(
      carrierPathname(
        "/",
        "?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans",
      ),
    ).toBe("/admin/cloud-orchestrator/plans");
    expect(
      carrierPathname(
        "/",
        "?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans%2Fplan_A-42",
      ),
    ).toBe("/admin/cloud-orchestrator/plans/plan_A-42");
    expect(
      carrierPathname(
        "/",
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fui",
      ),
    ).toBe(GROUP_OPS_PLANS_PATH);
    expect(
      carrierPathname(
        "/",
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fplans%2F42",
      ),
    ).toBe("/admin/automation-conversion/group-ops/plans/42");
    expect(
      carrierPathname(
        "/",
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion",
      ),
    ).toBe(AUDIENCE_PACKAGES_PATH);
    expect(
      carrierPathname(
        "/",
        "?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fpackages%2F9007199254740993",
      ),
    ).toBe("/admin/automation-conversion/packages/9007199254740993");
    expect(
      carrierPathname("/", `?legacy_admin_path=${EXECUTION_RUNTIME_PATH}`),
    ).toBe(EXECUTION_RUNTIME_PATH);
    expect(carrierPathname("/", `?legacy_admin_path=${ORDERS_PATH}`)).toBe(
      ORDERS_PATH,
    );
    expect(carrierPathname("/", `?legacy_admin_path=${PRODUCTS_PATH}`)).toBe(
      PRODUCTS_PATH,
    );
    expect(
      carrierPathname("/", `?legacy_admin_path=${SERVICE_PRODUCT_NEW_PATH}`),
    ).toBe(SERVICE_PRODUCT_NEW_PATH);
    expect(
      carrierPathname(
        "/",
        `?legacy_admin_path=${encodeURIComponent(`${WECHAT_PAY_TRANSACTIONS_PATH}/order_A-42`)}`,
      ),
    ).toBe(`${WECHAT_PAY_TRANSACTIONS_PATH}/order_A-42`);

    for (const search of [
      "?legacy_admin_path=/admin/image-library",
      "?legacy_admin_path=/admin/wecom-tags/extra",
      "?legacy_admin_path=/admin/channels/extra",
      "?legacy_admin_path=/admin/coupons/extra",
      "?legacy_admin_path=/admin/automation-agents/extra",
      "?legacy_admin_path=/admin/automation-conversion/group-ops/groups/ui",
      "?legacy_admin_path=/admin/automation-conversion/group-ops/plans/01",
      "?legacy_admin_path=/admin/automation-conversion/group-ops/plans/42/nodes",
      "?legacy_admin_path=/admin/orders/extra",
      "?legacy_admin_path=/admin/wechat-pay/products/extra",
      "?legacy_admin_path=/admin/service-period-products/service/nested/edit",
      "?legacy_admin_path=/admin/wechat-pay/transactions/order%0Aheader",
      "?legacy_admin_path=/admin/wecom-tags&legacy_admin_path=/admin/wecom-tags",
      `?legacy_admin_path=${IMAGE_LIBRARY_PATH}`,
      `?legacy_admin_path=${HXC_SENDER_PATH}&legacy_admin_path=${HXC_SENDER_PATH}`,
      `?legacy_admin_path=${ORDERS_PATH}&legacy_admin_path=${ORDERS_PATH}`,
      `?legacy_admin_path=${PRODUCTS_PATH}&legacy_admin_path=${PRODUCTS_PATH}`,
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

  it("lands on the group-operations plan workspace after a carrier refresh", () => {
    vi.stubGlobal("window", {
      location: {
        pathname: "/",
        search:
          "?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fui",
      },
    });
    const html = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(html).toContain('<h1 id="group-ops-title">群运营计划</h1>');
    expect(html).toContain("计划列表与筛选");
    expect(html).not.toContain("AI-CRM 运营指挥台");
  });

  it("lands on the audience-package workspace after a carrier refresh", () => {
    vi.stubGlobal("window", {
      location: {
        pathname: "/",
        search: "?legacy_admin_path=%2Fadmin%2Fautomation-conversion",
      },
    });
    const html = renderToStaticMarkup(<App initialSession={adminSession} />);
    expect(html).toContain('<h1 id="audience-packages-title">AI Audience 人群包</h1>');
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
