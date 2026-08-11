import React from "react";

export interface PermissionNavigationLink {
  readonly href: string;
  readonly label: string;
}

export type PermissionNavigationCallback =
  React.MouseEventHandler<HTMLAnchorElement>;

export interface PermissionNavigationProps {
  readonly activeHref?: string;
  readonly links?: readonly PermissionNavigationLink[];
  readonly onNavigate?: PermissionNavigationCallback;
}

/**
 * Renders only links already permitted by the parent. The parent owns the
 * surrounding navigation landmark and the browser-history behavior.
 */
export function PermissionNavigation({
  activeHref,
  links,
  onNavigate,
}: PermissionNavigationProps) {
  if (!links?.length) return null;

  return (
    <ul className="permission-nav">
      {links.map((link) => (
        <li key={`${link.href}-${link.label}`}>
          <a
            aria-current={link.href === activeHref ? "page" : undefined}
            href={link.href}
            onClick={onNavigate}
          >
            {link.label}
          </a>
        </li>
      ))}
    </ul>
  );
}

export interface AccountSummary {
  readonly displayName: string;
  /** A label already derived by the parent; this component does not infer it. */
  readonly roleLabel: string;
}

export type LogoutState = "ready" | "pending" | "error";

export interface LogoutControl {
  readonly onRequest?: () => void;
  readonly state: LogoutState;
}

export interface AccountCardProps {
  readonly account: AccountSummary;
  readonly logout: LogoutControl;
}

/**
 * Displays an already-validated account. Logout is only a parent callback;
 * this component does not inspect browser state or make a request.
 */
export function AccountCard({ account, logout }: AccountCardProps) {
  const isPending = logout.state === "pending";
  const isUnavailable = !logout.onRequest || isPending;

  return (
    <section className="auth-account-card" aria-label="当前已登录账号">
      <p className="auth-account-card__eyebrow">已登录</p>
      <h2 className="auth-account-card__name">{account.displayName}</h2>
      <p className="auth-account-card__role">{account.roleLabel}</p>
      <div className="auth-account-card__actions">
        <button
          className="auth-logout"
          disabled={isUnavailable}
          onClick={isUnavailable ? undefined : logout.onRequest}
          type="button"
        >
          {isPending ? "正在退出…" : "退出登录"}
        </button>
        {logout.state === "error" ? (
          <p className="auth-logout__error" role="alert">
            退出未完成，当前账号仍保持登录状态。请重试。
          </p>
        ) : null}
      </div>
    </section>
  );
}

export interface AnonymousLoginView {
  readonly kind: "anonymous";
}

export interface CheckingLoginView {
  readonly kind: "checking";
}

export interface ServiceErrorLoginView {
  readonly kind: "service-error";
  readonly onRetry?: () => void;
}

export interface AuthenticatedLoginView {
  readonly account: AccountSummary;
  readonly kind: "authenticated";
  readonly logout: LogoutControl;
  readonly onReturnToWorkbench?: PermissionNavigationCallback;
  readonly workbenchLink: PermissionNavigationLink;
}

export type LoginView =
  | AnonymousLoginView
  | CheckingLoginView
  | ServiceErrorLoginView
  | AuthenticatedLoginView;

export interface LoginPageProps {
  /** The parent passes a single already-resolved display state. */
  readonly view: LoginView;
  /** Lets the parent keep its one page-heading relationship stable. */
  readonly titleId?: string;
}

function LoginHeading({
  id,
  children,
}: {
  id: string;
  children: React.ReactNode;
}) {
  return <h1 id={id}>{children}</h1>;
}

/**
 * A route body for a parent-owned main landmark. It deliberately has no
 * provider URL, network access, browser storage, or session logic.
 */
export function LoginPage({
  view,
  titleId = "auth-page-title",
}: LoginPageProps) {
  if (view.kind === "authenticated") {
    return (
      <section className="auth-page" aria-labelledby={titleId}>
        <p className="auth-page__eyebrow">账号状态</p>
        <LoginHeading id={titleId}>已登录</LoginHeading>
        <p className="auth-page__message">
          你可以返回运营工作台，或在此安全退出当前账号。
        </p>
        <div className="auth-page__body">
          <AccountCard account={view.account} logout={view.logout} />
          <a
            className="auth-return"
            href={view.workbenchLink.href}
            onClick={view.onReturnToWorkbench}
          >
            返回运营工作台
          </a>
        </div>
      </section>
    );
  }

  if (view.kind === "checking") {
    return (
      <section
        className="auth-page auth-page--checking"
        aria-labelledby={titleId}
      >
        <p className="auth-page__eyebrow">登录状态</p>
        <LoginHeading id={titleId}>正在确认登录状态</LoginHeading>
        <p className="auth-page__message" role="status">
          请稍候，页面会在状态确认后更新。
        </p>
      </section>
    );
  }

  if (view.kind === "service-error") {
    return (
      <section className="auth-page" aria-labelledby={titleId}>
        <p className="auth-page__eyebrow">登录状态</p>
        <LoginHeading id={titleId}>暂时无法确认登录状态</LoginHeading>
        <p className="auth-page__message" role="alert">
          请检查网络或稍后重试。当前页面不会将你视为已登录。
        </p>
        <button
          className="auth-retry"
          disabled={!view.onRetry}
          onClick={view.onRetry}
          type="button"
        >
          重试
        </button>
      </section>
    );
  }

  return (
    <section className="auth-page" aria-labelledby={titleId}>
      <p className="auth-page__eyebrow">AI-CRM</p>
      <LoginHeading id={titleId}>登录运营工作台</LoginHeading>
      <p className="auth-page__message">
        企业微信登录入口正在等待外部接入；当前无法从此页面继续登录。
      </p>
      <button className="auth-login-entry" disabled type="button">
        企业微信登录（待接入）
      </button>
    </section>
  );
}
