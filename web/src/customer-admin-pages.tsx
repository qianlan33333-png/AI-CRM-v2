import React from "react";
import {
  CustomerDetailPage,
  type CustomerDetailPageProps,
} from "./customer-detail-ui";
import { CustomerContextPanel } from "./customer-context-ui";
import type { CustomerContextTransport } from "./customer-context";
import type { CustomerActivityAnalyticsTransport } from "./customer-activity-analytics";
import {
  ADMIN_CUSTOMER_LIST_PATH,
  adminCustomerContextPath,
  adminCustomerDetailPath,
} from "./customer-admin-routes";

export interface CustomerAliasNavigationProps {
  readonly customerID: number;
  readonly onNavigate?: React.MouseEventHandler<HTMLAnchorElement>;
}

export function CustomerAliasNavigation({
  customerID,
  onNavigate,
}: CustomerAliasNavigationProps): React.ReactElement {
  return (
    <nav aria-label="客户安全读取导航">
      <a href={ADMIN_CUSTOMER_LIST_PATH} onClick={onNavigate}>
        返回客户列表
      </a>
      <a href={adminCustomerDetailPath(customerID)} onClick={onNavigate}>
        查看客户详情
      </a>
      <a href={adminCustomerContextPath(customerID)} onClick={onNavigate}>
        查看 Customer 360 安全摘要
      </a>
    </nav>
  );
}

export type CustomerAdminDetailPageProps = Omit<
  CustomerDetailPageProps,
  "customerID"
> &
  CustomerAliasNavigationProps;

export function CustomerAdminDetailPage({
  customerID,
  onNavigate,
  ...detailProps
}: CustomerAdminDetailPageProps): React.ReactElement {
  return (
    <>
      <CustomerAliasNavigation
        customerID={customerID}
        onNavigate={onNavigate}
      />
      <CustomerDetailPage customerID={customerID} {...detailProps} />
    </>
  );
}

export interface CustomerAdminContextPageProps
  extends CustomerAliasNavigationProps {
  readonly transport?: CustomerContextTransport;
  readonly activityAnalyticsTransport?: CustomerActivityAnalyticsTransport;
  readonly onUnauthenticated?: () => void;
}

export function CustomerAdminContextPage({
  customerID,
  transport,
  activityAnalyticsTransport,
  onNavigate,
  onUnauthenticated,
}: CustomerAdminContextPageProps): React.ReactElement {
  return (
    <section className="customer-detail-page" aria-labelledby="app-title">
      <h1 id="app-title">Customer 360 安全摘要</h1>
      <CustomerAliasNavigation
        customerID={customerID}
        onNavigate={onNavigate}
      />
      <CustomerContextPanel
        customerID={customerID}
        transport={transport}
        activityAnalyticsTransport={activityAnalyticsTransport}
        onUnauthenticated={onUnauthenticated}
      />
    </section>
  );
}
