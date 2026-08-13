import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  CustomerListContent,
  CustomerListPage,
  CustomerRows,
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
  return {
    list: vi.fn(async () => ({
      status: 200,
      data: { ...page, next_cursor: null },
    })),
  };
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

function findAnchor(
  node: React.ReactNode,
  href: string,
): React.ReactElement | undefined {
  if (!React.isValidElement(node)) return undefined;
  if (node.type === "a" && node.props.href === href) return node;
  const children = React.Children.toArray(node.props.children);
  for (const child of children) {
    const found = findAnchor(child, href);
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
      expect(html).toContain("清空筛选");
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toContain("external_userid");
      expect(html).not.toContain("unionid");
    },
  );
});

describe("CustomerListContent states", () => {
  it("links every OneID to its exact detail route with one accessible click callback", () => {
    const onCustomerNavigate: React.MouseEventHandler<HTMLAnchorElement> =
      vi.fn();
    const content = CustomerRows({
      items: page.items,
      onCustomerNavigate,
    });
    const html = renderToStaticMarkup(content);
    const link = findAnchor(content, "/customers/7");

    expect(html).toContain('href="/customers/7"');
    expect(link?.props.children).toContain("\u9648\u6668");
    expect(link?.props.onClick).toBe(onCustomerNavigate);

    const click = {
      currentTarget: { href: "https://crm.example/customers/7" },
    };
    (link?.props.onClick as React.MouseEventHandler<HTMLAnchorElement>)(
      click as React.MouseEvent<HTMLAnchorElement>,
    );
    expect(onCustomerNavigate).toHaveBeenCalledOnce();
    expect(onCustomerNavigate).toHaveBeenCalledWith(click);
  });

  it("renders a semantic result table with watermark, 10k+ marker, and opaque page controls", () => {
    const screen: CustomerListScreen = {
      kind: "ready",
      hasPreviousPage: false,
      loadingNextPage: false,
      page,
      pageNumber: 1,
    };
    const html = renderToStaticMarkup(
      <CustomerListContent
        role="sales"
        screen={screen}
        onNextPage={vi.fn()}
        onPreviousPage={vi.fn()}
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
    expect(html).toContain("客户列表分页");
    expect(html).toContain("上一页");
    expect(html).toContain("第 1 页");
    expect(html).toContain("下一页");
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>上一页<\/button>/);
    expect(html).not.toContain(page.nextCursor);
  });

  it("renders loading, empty, full error, and retryable pagination error states accessibly", () => {
    const retry = vi.fn();
    const onNextPage = vi.fn();
    const onPreviousPage = vi.fn();
    const loading = renderToStaticMarkup(
      <CustomerListContent
        role="admin"
        screen={{ kind: "loading" }}
        onNextPage={onNextPage}
        onPreviousPage={onPreviousPage}
        onRetry={retry}
      />,
    );
    const empty = renderToStaticMarkup(
      <CustomerListContent
        role="admin"
        screen={{
          kind: "ready",
          hasPreviousPage: false,
          loadingNextPage: false,
          pageNumber: 1,
          page: {
            ...page,
            items: [],
            nextCursor: undefined,
            total: 0,
            totalIsEstimate: false,
          },
        }}
        onNextPage={onNextPage}
        onPreviousPage={onPreviousPage}
        onRetry={retry}
      />,
    );
    const failed = CustomerListContent({
      role: "admin",
      screen: { kind: "error", failure: "forbidden" },
      onNextPage,
      onPreviousPage,
      onRetry: retry,
    });
    const paginationFailed = renderToStaticMarkup(
      <CustomerListContent
        role="admin"
        screen={{
          kind: "ready",
          hasPreviousPage: true,
          loadingNextPage: false,
          pageNumber: 2,
          paginationFailure: "unavailable",
          page,
        }}
        onNextPage={onNextPage}
        onPreviousPage={onPreviousPage}
        onRetry={retry}
      />,
    );

    expect(loading).toContain('role="status"');
    expect(empty).toContain("没有符合当前筛选条件的客户。");
    expect(renderToStaticMarkup(failed)).toContain('role="alert"');
    expect(renderToStaticMarkup(failed)).toContain(
      "当前账号没有读取客户列表的权限。",
    );
    const retryButton = findButton(failed, "重试");
    expect(retryButton?.props.onClick).toBeTypeOf("function");
    (retryButton?.props.onClick as (() => void) | undefined)?.();
    expect(retry).toHaveBeenCalledOnce();
    expect(paginationFailed).toContain(
      "翻到下一页失败：客户列表暂时不可用，请稍后重试。",
    );
    expect(paginationFailed).toContain("重试下一页");
  });

  it("announces a pending next page without inventing rows or enabling duplicate navigation", () => {
    const html = renderToStaticMarkup(
      <CustomerListContent
        role="ops"
        screen={{
          kind: "ready",
          hasPreviousPage: true,
          loadingNextPage: true,
          page,
          pageNumber: 2,
        }}
        onNextPage={vi.fn()}
        onPreviousPage={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(html).toContain("正在读取下一页客户…");
    expect(html).toContain("陈晨");
    expect(html).toMatch(
      /<button[^>]*disabled=""[^>]*>下一页<\/button>/,
    );
  });
});
