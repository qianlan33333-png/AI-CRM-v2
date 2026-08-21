import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { CustomerDetailSnapshot, CustomerDetailTransport } from "./customer-detail";
import type { CustomerContextTransport } from "./customer-context";
import type { CustomerMergeHistoryTransport } from "./customer-merge-history";
import type { CustomerChatActivityTransport } from "./customer-chat-activity";
import type { CustomerActivityAnalyticsTransport } from "./customer-activity-analytics";
import type { StageTransport } from "./stages";
import {
  Customer360AuxiliaryPanels,
  Customer360MutationLockBanner,
  Customer360Workbench,
} from "./customer-360-workbench";

const snapshot: CustomerDetailSnapshot = {
  customer: {
    id: 7,
    name: "林小姐",
    gender: 1,
    stageID: 3,
    ownerStaffID: 11,
    channelID: 5,
    addedAt: "2026-08-20T00:00:00Z",
    lastInteractAt: "2026-08-20T01:00:00Z",
    isDeleted: false,
    createdAt: "2026-08-19T00:00:00Z",
    updatedAt: "2026-08-21T01:00:00Z",
  },
  tags: [{ id: 9, groupName: "意向", name: "已报名", sortOrder: 10 }],
  tagCatalog: [
    { id: 9, groupName: "意向", name: "已报名", sortOrder: 10 },
    { id: 10, groupName: "意向", name: "待联系", sortOrder: 20 },
  ],
  events: [
    {
      id: 12,
      eventType: "stage_changed",
      actor: "后台账号 #1",
      occurredAt: "2026-08-20T02:00:00Z",
    },
  ],
  eventsHaveMore: true,
  eventsNextCursor: "next-page",
};

function customerTransport(): CustomerDetailTransport {
  return {
    get: vi.fn(),
    update: vi.fn(),
    setStage: vi.fn(),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    listEvents: vi.fn(),
    listTags: vi.fn(),
  } as unknown as CustomerDetailTransport;
}

function stageTransport(): StageTransport {
  return {
    list: vi.fn(),
    create: vi.fn(),
    rename: vi.fn(),
  } as unknown as StageTransport;
}

const contextTransport = { get: vi.fn() } as unknown as CustomerContextTransport;
const mergeTransport = { get: vi.fn() } as unknown as CustomerMergeHistoryTransport;
const chatTransport = { get: vi.fn() } as unknown as CustomerChatActivityTransport;
const analyticsTransport = { get: vi.fn() } as unknown as CustomerActivityAnalyticsTransport;

function workbench(principal: { adminUserID: number; role: "admin" | "ops" | "sales" }) {
  return (
    <Customer360Workbench
      customerID={7}
      principal={principal}
      customerTransport={customerTransport()}
      stageTransport={stageTransport()}
      contextTransport={contextTransport}
      mergeHistoryTransport={mergeTransport}
      chatActivityTransport={chatTransport}
      activityAnalyticsTransport={analyticsTransport}
      initialCoreSnapshot={snapshot}
      initialStages={[{ id: 3, name: "已报名", sortOrder: 10, config: {} }]}
    />
  );
}

describe("Customer360Workbench SSR", () => {
  it("renders one integrated admin/ops customer operation flow", () => {
    const html = renderToStaticMarkup(workbench({ adminUserID: 1, role: "ops" }));

    expect(html).toContain("客户 360 一体化运营工作台");
    expect(html.match(/<h1\b/g)).toHaveLength(1);
    expect(html).toContain("林小姐 · OneID #7");
    expect(html).toContain('href="#customer-360-core-panel"');
    expect(html).toContain('href="#customer-360-context-panel"');
    expect(html).toContain('href="#customer-360-chat-panel"');
    expect(html).toContain('href="#customer-360-analytics-panel"');
    expect(html).toContain('href="#customer-360-merge-panel"');

    expect(html).toContain("基本资料");
    expect(html).toContain("客户阶段");
    expect(html).toContain("客户标签");
    expect(html).toContain("客户时间线");
    expect(html).toContain("Customer 360 本地读取");
    expect(html).toContain("本地聊天活动");
    expect(html).toContain("客户本地活动统计");
    expect(html).toContain("OneID 合并历史");

    expect(html).toContain("本地保存成功不等于 Provider 同步、发送或处理成功");
    expect(html).toContain("expected_version");
    expect(html).toContain("加载更多时间线");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("unionid");
    expect(html).not.toContain("13800138000");
    expect(html).not.toContain("X-CSRF-Token");
  });

  it("fails closed for sales before mounting any customer panel", () => {
    const html = renderToStaticMarkup(
      workbench({ adminUserID: 3, role: "sales" }),
    );
    expect(html).toContain("不是 admin/ops");
    expect(html).toContain("未发送任何客户请求");
    expect(html).not.toContain("客户时间线");
    expect(html).not.toContain("本地聊天活动");
    expect(html).not.toContain("保存资料");
  });



  it("does not mount request-capable panels for an unauthorized role", () => {
    const customerGet = vi.fn();
    const contextGet = vi.fn();
    const mergeGet = vi.fn();
    const chatGet = vi.fn();
    const analyticsGet = vi.fn();
    const stageList = vi.fn();

    const html = renderToStaticMarkup(
      <Customer360Workbench
        customerID={7}
        principal={{ adminUserID: 3, role: "sales", staffID: 9 }}
        customerTransport={
          {
            ...customerTransport(),
            get: customerGet,
          } as unknown as CustomerDetailTransport
        }
        stageTransport={
          {
            ...stageTransport(),
            list: stageList,
          } as unknown as StageTransport
        }
        contextTransport={{ get: contextGet } as unknown as CustomerContextTransport}
        mergeHistoryTransport={{ get: mergeGet } as unknown as CustomerMergeHistoryTransport}
        chatActivityTransport={{ get: chatGet } as unknown as CustomerChatActivityTransport}
        activityAnalyticsTransport={{ get: analyticsGet } as unknown as CustomerActivityAnalyticsTransport}
      />,
    );

    expect(html).toContain("未发送任何客户请求");
    expect(customerGet).not.toHaveBeenCalled();
    expect(stageList).not.toHaveBeenCalled();
    expect(contextGet).not.toHaveBeenCalled();
    expect(mergeGet).not.toHaveBeenCalled();
    expect(chatGet).not.toHaveBeenCalled();
    expect(analyticsGet).not.toHaveBeenCalled();
  });

  it("does not render a snapshot that belongs to another customer", () => {
    const html = renderToStaticMarkup(
      <Customer360Workbench
        customerID={8}
        principal={{ adminUserID: 1, role: "admin" }}
        customerTransport={customerTransport()}
        stageTransport={stageTransport()}
        contextTransport={contextTransport}
        mergeHistoryTransport={mergeTransport}
        chatActivityTransport={chatTransport}
        activityAnalyticsTransport={analyticsTransport}
        initialCoreSnapshot={snapshot}
        initialStages={[]}
      />,
    );

    expect(html).toContain("OneID #8");
    expect(html).toContain("正在读取本地客户事实");
    expect(html).not.toContain("林小姐 · OneID #7");
    expect(html).not.toContain("后台账号 #1");
  });

  it("fails closed for malformed principal and invalid customer ID", () => {
    const malformed = renderToStaticMarkup(
      <Customer360Workbench
        customerID={7}
        principal={
          {
            adminUserID: 1,
            role: "admin",
            external_userid: "forbidden",
          } as never
        }
        customerTransport={customerTransport()}
      />,
    );
    expect(malformed).toContain("无权限访问");
    expect(malformed).not.toContain("forbidden");

    const invalidID = renderToStaticMarkup(
      <Customer360Workbench
        customerID={0}
        principal={{ adminUserID: 1, role: "admin" }}
        customerTransport={customerTransport()}
      />,
    );
    expect(invalidID).toContain("客户编号无效");
    expect(invalidID).not.toContain("保存资料");
  });

  it("renders explicit empty states without inventing customer activity", () => {
    const html = renderToStaticMarkup(
      <Customer360Workbench
        customerID={7}
        principal={{ adminUserID: 1, role: "admin" }}
        customerTransport={customerTransport()}
        stageTransport={stageTransport()}
        contextTransport={contextTransport}
        mergeHistoryTransport={mergeTransport}
        chatActivityTransport={chatTransport}
        activityAnalyticsTransport={analyticsTransport}
        initialCoreSnapshot={{
          ...snapshot,
          tags: [],
          events: [],
          eventsHaveMore: false,
          eventsNextCursor: undefined,
        }}
        initialStages={[]}
      />,
    );
    expect(html).toContain("暂无本地标签");
    expect(html).toContain("暂无本地时间线记录");
    expect(html).not.toContain("加载更多时间线");
  });
});

describe("Customer360Workbench partial and locked states", () => {
  it("keeps successful panels visible when another panel fails", () => {
    const html = renderToStaticMarkup(
      <Customer360AuxiliaryPanels
        contextPanel={<section role="alert">客户上下文暂不可用</section>}
        chatPanel={<section>聊天摘要已加载</section>}
        analyticsPanel={<section>活动分析已加载</section>}
        mergeHistoryPanel={<section>合并历史已加载</section>}
      />,
    );
    expect(html).toContain("客户上下文暂不可用");
    expect(html).toContain("聊天摘要已加载");
    expect(html).toContain("活动分析已加载");
    expect(html).toContain("合并历史已加载");
  });

  it.each([
    ["conflict", "并发冲突"],
    ["outcome_unknown", "写入结果未知"],
  ] as const)("renders %s as a fail-closed action lock", (reason, title) => {
    const html = renderToStaticMarkup(
      <Customer360MutationLockBanner
        state={{
          kind: "locked",
          reason,
          idempotencyKey:
            "customer-360:profile:123e4567-e89b-42d3-a456-426614174000",
        }}
        onRefresh={() => undefined}
      />,
    );
    expect(html).toContain(title);
    expect(html).toContain("不会自动重试");
    expect(html).toContain("重新读取本地事实并解锁");
  });
});
