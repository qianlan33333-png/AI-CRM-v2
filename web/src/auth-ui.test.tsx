import { readFileSync } from "node:fs";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  AccountCard,
  LoginPage,
  PermissionNavigation,
  type LoginView,
  type PermissionNavigationLink,
} from "./auth-ui";

interface ElementMatch {
  readonly className?: string;
  readonly href?: string;
  readonly type: string;
}

function findElement(
  node: React.ReactNode,
  match: ElementMatch,
): React.ReactElement<Record<string, unknown>> | undefined {
  if (!React.isValidElement<Record<string, unknown>>(node)) return undefined;
  if (
    node.type === match.type &&
    (match.className === undefined ||
      node.props.className === match.className) &&
    (match.href === undefined || node.props.href === match.href)
  ) {
    return node;
  }

  for (const child of React.Children.toArray(
    node.props.children as React.ReactNode,
  )) {
    const found = findElement(child, match);
    if (found) return found;
  }

  return undefined;
}

function ParentShell({
  view,
  links,
}: {
  view: LoginView;
  links?: readonly PermissionNavigationLink[];
}) {
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳至主要内容
      </a>
      <header className="shell-header">AI-CRM</header>
      <div className="shell-frame">
        <nav className="shell-nav" aria-label="主导航">
          <PermissionNavigation activeHref="/customers" links={links} />
        </nav>
        <main id="main-content" className="shell-main" tabIndex={-1}>
          <LoginPage titleId="app-title" view={view} />
        </main>
      </div>
    </div>
  );
}

describe("presentational login page", () => {
  it("renders an honest unavailable WeCom entry without a provider link or QR code", () => {
    const html = renderToStaticMarkup(
      <LoginPage view={{ kind: "anonymous" }} />,
    );

    expect(html).toContain("登录运营工作台");
    expect(html).toContain("等待外部接入");
    expect(html).toContain("企业微信登录（待接入）");
    expect(html).toMatch(
      /<button[^>]*disabled=""[^>]*>企业微信登录（待接入）<\/button>/,
    );
    expect(html).toMatch(/<button[^>]*type="button"/);
    expect(html).not.toContain("二维码");
    expect(html).not.toMatch(/href="https?:\/\//);
    expect(html.match(/<h1\b/g)).toHaveLength(1);
  });

  it("shows a retryable service error without treating the visitor as authenticated", () => {
    const onRetry = vi.fn();
    const element = LoginPage({
      view: { kind: "service-error", onRetry },
    });
    const retry = findElement(element, {
      className: "auth-retry",
      type: "button",
    });
    const handler = retry?.props.onClick as
      React.MouseEventHandler<HTMLButtonElement> | undefined;
    const html = renderToStaticMarkup(element);

    expect(html).toContain("暂时无法确认登录状态");
    expect(html).toContain("当前页面不会将你视为已登录");
    expect(handler).toBeTypeOf("function");
    handler?.({} as React.MouseEvent<HTMLButtonElement>);
    expect(onRetry).toHaveBeenCalledOnce();

    const unavailableRetry = renderToStaticMarkup(
      <LoginPage view={{ kind: "service-error" }} />,
    );
    expect(unavailableRetry).toMatch(
      /<button[^>]*disabled=""[^>]*>重试<\/button>/,
    );
  });

  it("renders the checked account, role label, workbench return, and logout states from props", () => {
    const onReturnToWorkbench = vi.fn();
    const onLogout = vi.fn();
    const view: LoginView = {
      account: { displayName: "林青", roleLabel: "运营角色" },
      kind: "authenticated",
      logout: { onRequest: onLogout, state: "error" },
      onReturnToWorkbench,
      workbenchLink: { href: "/", label: "概览" },
    };
    const page = LoginPage({ view });
    const returnLink = findElement(page, {
      className: "auth-return",
      type: "a",
    });
    const returnHandler = returnLink?.props.onClick as
      React.MouseEventHandler<HTMLAnchorElement> | undefined;
    const logout = AccountCard({ account: view.account, logout: view.logout });
    const logoutButton = findElement(logout, {
      className: "auth-logout",
      type: "button",
    });
    const logoutHandler = logoutButton?.props.onClick as
      React.MouseEventHandler<HTMLButtonElement> | undefined;
    const html = renderToStaticMarkup(page);

    expect(html).toContain("已登录");
    expect(html).toContain("林青");
    expect(html).toContain("运营角色");
    expect(html).toContain('href="/"');
    expect(html).toContain("退出未完成，当前账号仍保持登录状态");
    expect(returnHandler).toBeTypeOf("function");
    returnHandler?.({} as React.MouseEvent<HTMLAnchorElement>);
    expect(onReturnToWorkbench).toHaveBeenCalledWith(expect.anything());
    expect(logoutHandler).toBeTypeOf("function");
    logoutHandler?.({} as React.MouseEvent<HTMLButtonElement>);
    expect(onLogout).toHaveBeenCalledOnce();

    const pending = renderToStaticMarkup(
      <AccountCard
        account={{ displayName: "林青", roleLabel: "运营角色" }}
        logout={{ onRequest: onLogout, state: "pending" }}
      />,
    );
    expect(pending).toContain("正在退出…");
    expect(pending).toMatch(/<button[^>]*disabled=""[^>]*>正在退出…<\/button>/);
  });
});

describe("permission navigation", () => {
  it("fails closed when no permitted links are supplied", () => {
    expect(PermissionNavigation({})).toBeNull();
    expect(PermissionNavigation({ links: [] })).toBeNull();
  });

  it("renders only parent-supplied links and forwards navigation without owning it", () => {
    const onNavigate = vi.fn();
    const links = [
      { href: "/", label: "概览" },
      { href: "/customers", label: "客户" },
    ] as const;
    const navigation = PermissionNavigation({
      activeHref: "/customers",
      links,
      onNavigate,
    });
    const customer = findElement(navigation, { href: "/customers", type: "a" });
    const handler = customer?.props.onClick as
      React.MouseEventHandler<HTMLAnchorElement> | undefined;
    const html = renderToStaticMarkup(navigation);

    expect(html).toContain("概览");
    expect(html).toContain("客户");
    expect(html).not.toContain("设置");
    expect(html).toContain('aria-current="page"');
    expect(handler).toBeTypeOf("function");
    handler?.({} as React.MouseEvent<HTMLAnchorElement>);
    expect(onNavigate).toHaveBeenCalledWith(expect.anything());
  });
});

describe("parent shell semantics and client boundary", () => {
  it("fits a parent-owned skip link, landmarks, and one page heading", () => {
    const html = renderToStaticMarkup(
      <ParentShell
        links={[{ href: "/customers", label: "客户" }]}
        view={{ kind: "anonymous" }}
      />,
    );

    expect(html).toContain('href="#main-content"');
    expect(html).toContain("跳至主要内容");
    expect(html).toContain("<header");
    expect(html).toContain('aria-label="主导航"');
    expect(html).toContain('<main id="main-content"');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
  });

  it("keeps sensitive transport and browser-storage values out of the rendered UI and component source", () => {
    const html = renderToStaticMarkup(
      <ParentShell
        links={[{ href: "/customers", label: "客户" }]}
        view={{ kind: "service-error", onRetry: vi.fn() }}
      />,
    );
    const source = readFileSync(
      new URL("./auth-ui.tsx", import.meta.url),
      "utf8",
    );
    const css = readFileSync(new URL("./shell.css", import.meta.url), "utf8");

    expect(html).not.toMatch(/csrf|bearer|token|cookie|aicrm_session/i);
    expect(source).not.toMatch(
      /\bfetch\b|document\.cookie|localStorage|sessionStorage|indexedDB|\.\/api\/generated/,
    );
    expect(source).not.toContain("/api/");
    expect(css).toContain("button:focus-visible");
    expect(css).toContain("@media (max-width: 720px)");
    expect(css).toContain(".permission-nav");
    expect(css).toContain("@media (prefers-reduced-motion: reduce)");
  });
});
