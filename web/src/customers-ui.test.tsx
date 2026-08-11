import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  CustomerListContent,
  CustomerListPage,
  type CustomerListScreen,
} from "./customers-ui";
import type { CustomerTransport } from "./customers";

const page = {
  items: [
    {
      id: 7,
      name: "陈晨",
      ownerStaffID: 8,
      stageID: 3,
      channelID: 5,
      addedAt: "2026-08-12T08:00:00Z",
      lastInteractAt: null,
      isDeleted: false,
      extra: {},
      createdAt: "2026-08-12T07:00:00Z",
      updatedAt: "2026-08-12T10:00:00Z",
    },
  ],
  nextCursor: "opaque-server-cursor",
  total: 10_000,
  totalIsEstimate: true,
  watermark: "2026-08-12T10:00:00Z",
} as const;

function transport(): CustomerTransport {
  return { list: vi.fn(async () => ({ status: 200, data: { ...page, next_cursor: null } })) };
}

function findButton(
  node: React.ReactNode,
  text: string,
): React.ReactElement | undefined {
  if (!React.isValidElement(node)) return undefined;
  if (node.type === "button" && node.props.children === text) return node;
  const children = React.Children.toArray(node.props.children);
  for (const child of children) {
    const found = findButton(child, text);
    if (found) return found;
  }
  return undefined;
}

describe("CustomerListPage shell", () => {
  it.each([
    ["admin", "管理员"],
    ["ops", "运营"],
    ["sales", "销售"],
  ] as const)(
    "renders all frozen list filters and the role label for %s without changing access behavior",
    (role, label) => {
      const html = renderToStaticMarkup(
        <CustomerListPage role={role} transport={transport()} />,
      );

      expect(html).toContain('<h1 id="app-title">客户列表</h1>');
      expect(html).toContain(`当前角色：${label}`);
      expect(html).toContain('role="status"');
      expect(html).toContain("正在读取客户列表");
      for (const field of [
        "关键词",
        "负责人 ID",
        "阶段 ID",
        "渠道 ID",
        "标签 ID",
        "加入时间开始",
        "加入时间结束",
        "最近互动开始",
        "最近互动结束",
        "每页数量",
        "仅显示已删除客户",
      ]) {
        expect(html).toContain(field);
      }
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toContain("external_userid");
      expect(html).not.toContain("unionid");
    },
  );
});

describe("CustomerListContent states", () => {
  it("renders a semantic result table with watermark, 10k+ marker, and opaque next page action", () => {
    const screen: CustomerListScreen = {
      kind: "ready",
      loadingMore: false,
      page,
    };
    const html = renderToStaticMarkup(
      <CustomerListContent
        role="sales"
        screen={screen}
        onLoadMore={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(html).toContain("共 10,000+ 名客户（估算）");
    expect(html).toContain("数据水位：");
    expect(html).toContain(page.watermark);
    expect(html).toContain("当前为销售视图，数据范围由服务端权限规则决定。");
    expect(html).toContain("<table>");
    expect(html).toContain("客户列表（排序与数据范围以服务端结果为准）");
    expect(html).toContain("OneID 7");
    expect(html).toContain("加载更多客户");
    expect(html).not.toContain(page.nextCursor);
  });

  it("renders loading, empty, full error, and retryable pagination error states accessibly", () => {
    const retry = vi.fn();
    const onLoadMore = vi.fn();
    const loading = renderToStaticMarkup(
      <CustomerListContent
        role="admin"
        screen={{ kind: "loading" }}
        onLoadMore={onLoadMore}
        onRetry={retry}
      />,
    );
    const empty = renderToStaticMarkup(
      <CustomerListContent
        role="admin"
        screen={{
          kind: "ready",
          loadingMore: false,
          page: { ...page, items: [], nextCursor: undefined, total: 0, totalIsEstimate: false },
        }}
        onLoadMore={onLoadMore}
        onRetry={retry}
      />,
    );
    const failed = CustomerListContent({
      role: "admin",
      screen: { kind: "error", failure: "forbidden" },
      onLoadMore,
      onRetry: retry,
    });
    const paginationFailed = renderToStaticMarkup(
      <CustomerListContent
        role="admin"
        screen={{
          kind: "ready",
          loadingMore: false,
          paginationFailure: "unavailable",
          page,
        }}
        onLoadMore={onLoadMore}
        onRetry={retry}
      />,
    );

    expect(loading).toContain('role="status"');
    expect(empty).toContain("没有符合当前筛选条件的客户。");
    expect(renderToStaticMarkup(failed)).toContain('role="alert"');
    expect(renderToStaticMarkup(failed)).toContain("当前账号没有读取客户列表的权限。");
    const retryButton = findButton(failed, "重试");
    expect(retryButton?.props.onClick).toBeTypeOf("function");
    (retryButton?.props.onClick as (() => void) | undefined)?.();
    expect(retry).toHaveBeenCalledOnce();
    expect(paginationFailed).toContain("继续加载失败：客户列表暂时不可用，请稍后重试。");
    expect(paginationFailed).toContain("重试加载更多");
  });

  it("announces a pending next page without inventing rows or hiding its retry action", () => {
    const html = renderToStaticMarkup(
      <CustomerListContent
        role="ops"
        screen={{ kind: "ready", loadingMore: true, page }}
        onLoadMore={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(html).toContain("正在加载更多客户…");
    expect(html).toContain("陈晨");
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>正在加载更多…<\/button>/);
  });
});
