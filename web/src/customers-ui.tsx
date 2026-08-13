import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  appendCustomerListHistoryPage,
  defaultCustomerListFilterDraft,
  generatedCustomerTransport,
  loadCustomers,
  parseCustomerListFilters,
  type CustomerListFilterDraft,
  type CustomerListFilters,
  type CustomerListHistoryEntry,
  type CustomerListPage,
  type CustomerLoadFailure,
  type CustomerRecord,
  type CustomerRole,
  type CustomerTransport,
} from "./customers";
import "./customers-list.css";

export interface CustomerListPageProps {
  readonly role: CustomerRole;
  readonly transport?: CustomerTransport;
  readonly onUnauthenticated?: () => void;
  readonly onCustomerNavigate?: CustomerNavigateHandler;
}

export type CustomerNavigateHandler =
  React.MouseEventHandler<HTMLAnchorElement>;

export type CustomerListScreen =
  | { readonly kind: "loading" }
  | { readonly kind: "error"; readonly failure: CustomerLoadFailure }
  | {
      readonly kind: "ready";
      readonly page: CustomerListPage;
      readonly pageNumber: number;
      readonly hasPreviousPage: boolean;
      readonly loadingNextPage: boolean;
      readonly paginationFailure?: CustomerLoadFailure;
    };

interface CustomerListRequest {
  readonly filters: CustomerListFilters;
  readonly cursor?: string;
  readonly direction: "initial" | "next";
}

const roleLabels: Record<CustomerRole, string> = {
  admin: "管理员",
  ops: "运营",
  sales: "销售",
};

const failureMessages: Record<CustomerLoadFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有读取客户列表的权限。",
  invalid: "筛选条件未被服务端接受，请检查后重试。",
  unavailable: "客户列表暂时不可用，请稍后重试。",
};

function valueOrPlaceholder(value: number | null | undefined, empty: string) {
  return value === null || value === undefined ? empty : String(value);
}

function dateTimeCell(value: string | null | undefined) {
  if (!value) return "—";
  return <time dateTime={value}>{value}</time>;
}

function totalLabel(page: CustomerListPage): string {
  return page.totalIsEstimate
    ? `共 ${page.total.toLocaleString("en-US")}+ 名客户（估算）`
    : `共 ${page.total.toLocaleString("en-US")} 名客户`;
}

export function CustomerRows({
  items,
  onCustomerNavigate,
}: {
  readonly items: readonly CustomerRecord[];
  readonly onCustomerNavigate?: CustomerNavigateHandler;
}) {
  return (
    <tbody>
      {items.map((customer) => (
        <tr key={customer.id}>
          <th scope="row">
            <a
              className="customer-list__customer-name"
              href={`/customers/${customer.id}`}
              onClick={onCustomerNavigate}
            >
              {customer.name.trim() === "" ? "未命名客户" : customer.name}
            </a>
            <span className="customer-list__customer-id">
              OneID {customer.id}
            </span>
          </th>
          <td>{valueOrPlaceholder(customer.ownerStaffID, "未分配")}</td>
          <td>{valueOrPlaceholder(customer.stageID, "未设置")}</td>
          <td>{valueOrPlaceholder(customer.channelID, "未标记")}</td>
          <td>{dateTimeCell(customer.addedAt)}</td>
          <td>{dateTimeCell(customer.lastInteractAt)}</td>
          <td>{customer.isDeleted ? "已删除" : "正常"}</td>
        </tr>
      ))}
    </tbody>
  );
}

export interface CustomerListContentProps {
  readonly role: CustomerRole;
  readonly screen: CustomerListScreen;
  readonly onRetry: () => void;
  readonly onPreviousPage: () => void;
  readonly onNextPage: () => void;
  readonly onCustomerNavigate?: CustomerNavigateHandler;
}

/**
 * Pure result region for the parent-owned page shell. The role is display-only:
 * the server remains the sole source of authorization and data scope.
 */
export function CustomerListContent({
  role,
  screen,
  onRetry,
  onPreviousPage,
  onNextPage,
  onCustomerNavigate,
}: CustomerListContentProps): React.ReactElement {
  if (screen.kind === "loading") {
    return (
      <p className="customer-list__state" role="status">
        正在读取客户列表…
      </p>
    );
  }

  if (screen.kind === "error") {
    return (
      <div className="customer-list__state" role="alert">
        <p>{failureMessages[screen.failure]}</p>
        <button type="button" onClick={onRetry}>
          重试
        </button>
      </div>
    );
  }

  const { page } = screen;
  if (page.items.length === 0) {
    return (
      <div className="customer-list__state" role="status">
        <p>没有符合当前筛选条件的客户。</p>
        <p className="customer-list__watermark">
          数据水位：<time dateTime={page.watermark}>{page.watermark}</time>
        </p>
      </div>
    );
  }

  return (
    <div className="customer-list__results">
      <div className="customer-list__summary" aria-live="polite">
        <p>{totalLabel(page)}</p>
        <p className="customer-list__watermark">
          数据水位：<time dateTime={page.watermark}>{page.watermark}</time>
        </p>
        <p className="customer-list__scope">
          当前为{roleLabels[role]}视图，数据范围由服务端权限规则决定。
        </p>
      </div>
      <div className="customer-list__table-wrap">
        <table>
          <caption>客户列表（排序与数据范围以服务端结果为准）</caption>
          <thead>
            <tr>
              <th scope="col">客户</th>
              <th scope="col">负责人</th>
              <th scope="col">阶段</th>
              <th scope="col">渠道</th>
              <th scope="col">加入时间</th>
              <th scope="col">最近互动</th>
              <th scope="col">状态</th>
            </tr>
          </thead>
          <CustomerRows
            items={page.items}
            onCustomerNavigate={onCustomerNavigate}
          />
        </table>
      </div>
      {screen.paginationFailure && (
        <div className="customer-list__pagination-error" role="alert">
          <p>翻到下一页失败：{failureMessages[screen.paginationFailure]}</p>
          <button type="button" onClick={onRetry}>
            重试下一页
          </button>
        </div>
      )}
      {screen.loadingNextPage && (
        <p className="customer-list__more-state" role="status">
          正在读取下一页客户…
        </p>
      )}
      <nav className="customer-list__pagination" aria-label="客户列表分页">
        <button
          disabled={!screen.hasPreviousPage || screen.loadingNextPage}
          type="button"
          onClick={onPreviousPage}
        >
          上一页
        </button>
        <p aria-live="polite">第 {screen.pageNumber} 页</p>
        <button
          disabled={!page.nextCursor || screen.loadingNextPage}
          type="button"
          onClick={onNextPage}
        >
          下一页
        </button>
      </nav>
    </div>
  );
}

function NumericFilter({
  id,
  label,
  value,
  onChange,
}: {
  readonly id: string;
  readonly label: string;
  readonly value: string;
  readonly onChange: React.ChangeEventHandler<HTMLInputElement>;
}) {
  return (
    <label htmlFor={id}>
      {label}
      <input
        id={id}
        inputMode="numeric"
        min="1"
        name={id}
        step="1"
        type="number"
        value={value}
        onChange={onChange}
      />
    </label>
  );
}

function CustomerListFiltersForm({
  draft,
  loading,
  error,
  onInputChange,
  onClear,
  onSubmit,
}: {
  readonly draft: CustomerListFilterDraft;
  readonly loading: boolean;
  readonly error?: string;
  readonly onInputChange: React.ChangeEventHandler<HTMLInputElement>;
  readonly onClear: () => void;
  readonly onSubmit: React.FormEventHandler<HTMLFormElement>;
}) {
  return (
    <form className="customer-list__filters" onSubmit={onSubmit}>
      <fieldset>
        <legend>筛选条件</legend>
        <label htmlFor="customer-keyword">
          关键词
          <input
            id="customer-keyword"
            maxLength={200}
            name="keyword"
            type="search"
            value={draft.keyword}
            onChange={onInputChange}
          />
        </label>
        <NumericFilter
          id="customer-owner-staff-id"
          label="负责人 ID"
          value={draft.ownerStaffID}
          onChange={onInputChange}
        />
        <NumericFilter
          id="customer-stage-id"
          label="阶段 ID"
          value={draft.stageID}
          onChange={onInputChange}
        />
        <NumericFilter
          id="customer-channel-id"
          label="渠道 ID"
          value={draft.channelID}
          onChange={onInputChange}
        />
        <NumericFilter
          id="customer-tag-id"
          label="标签 ID"
          value={draft.tagID}
          onChange={onInputChange}
        />
        <label htmlFor="customer-added-after">
          加入时间开始
          <input
            id="customer-added-after"
            name="addedAfter"
            type="datetime-local"
            value={draft.addedAfter}
            onChange={onInputChange}
          />
        </label>
        <label htmlFor="customer-added-before">
          加入时间结束
          <input
            id="customer-added-before"
            name="addedBefore"
            type="datetime-local"
            value={draft.addedBefore}
            onChange={onInputChange}
          />
        </label>
        <label htmlFor="customer-last-interact-after">
          最近互动开始
          <input
            id="customer-last-interact-after"
            name="lastInteractAfter"
            type="datetime-local"
            value={draft.lastInteractAfter}
            onChange={onInputChange}
          />
        </label>
        <label htmlFor="customer-last-interact-before">
          最近互动结束
          <input
            id="customer-last-interact-before"
            name="lastInteractBefore"
            type="datetime-local"
            value={draft.lastInteractBefore}
            onChange={onInputChange}
          />
        </label>
        <label htmlFor="customer-limit">
          每页数量
          <input
            id="customer-limit"
            inputMode="numeric"
            max="200"
            min="1"
            name="limit"
            step="1"
            type="number"
            value={draft.limit}
            onChange={onInputChange}
          />
        </label>
        <label
          className="customer-list__checkbox"
          htmlFor="customer-is-deleted"
        >
          <input
            checked={draft.isDeleted}
            id="customer-is-deleted"
            name="isDeleted"
            type="checkbox"
            onChange={onInputChange}
          />
          仅显示已删除客户
        </label>
        <button disabled={loading} type="submit">
          {loading ? "正在读取…" : "应用筛选"}
        </button>
        <button disabled={loading} type="button" onClick={onClear}>
          清空筛选
        </button>
      </fieldset>
      {error && (
        <p className="customer-list__filter-error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}

export function CustomerListPage({
  role,
  transport = generatedCustomerTransport,
  onUnauthenticated,
  onCustomerNavigate,
}: CustomerListPageProps): React.ReactElement {
  const [draft, setDraft] = useState<CustomerListFilterDraft>({
    ...defaultCustomerListFilterDraft,
  });
  const [screen, setScreen] = useState<CustomerListScreen>({ kind: "loading" });
  const [filterError, setFilterError] = useState<string>();
  const requestVersion = useRef(0);
  const appliedFilters = useRef<CustomerListFilters>({
    limit: 50,
    isDeleted: false,
  });
  const lastRequest = useRef<CustomerListRequest>({
    direction: "initial",
    filters: appliedFilters.current,
  });
  const pageHistory = useRef<readonly CustomerListHistoryEntry[]>([]);
  const pageHistoryIndex = useRef(0);

  const requestPage = useCallback(
    async (request: CustomerListRequest) => {
      const version = requestVersion.current + 1;
      requestVersion.current = version;
      lastRequest.current = request;
      if (request.direction === "next") {
        setScreen((current) =>
          current.kind === "ready"
            ? {
                ...current,
                loadingNextPage: true,
                paginationFailure: undefined,
              }
            : current,
        );
      } else {
        setScreen({ kind: "loading" });
      }

      const result = await loadCustomers(
        transport,
        request.filters,
        request.cursor,
      );
      if (version !== requestVersion.current) return;

      if (result.status !== "loaded") {
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setScreen((current) =>
          request.direction === "next" && current.kind === "ready"
            ? {
                ...current,
                loadingNextPage: false,
                paginationFailure: result.status,
              }
            : { kind: "error", failure: result.status },
        );
        return;
      }

      setScreen((current) => {
        if (request.direction === "next" && current.kind === "ready") {
          const history = appendCustomerListHistoryPage(
            pageHistory.current,
            request.cursor ?? "",
            result.page,
          );
          if (!history) {
            return {
              ...current,
              loadingNextPage: false,
              paginationFailure: "unavailable",
            };
          }
          pageHistory.current = history;
          pageHistoryIndex.current = history.length - 1;
          return {
            kind: "ready",
            loadingNextPage: false,
            hasPreviousPage: true,
            page: result.page,
            pageNumber: history.length,
          };
        }
        pageHistory.current = [{ page: result.page }];
        pageHistoryIndex.current = 0;
        return {
          kind: "ready",
          hasPreviousPage: false,
          loadingNextPage: false,
          page: result.page,
          pageNumber: 1,
        };
      });
    },
    [onUnauthenticated, transport],
  );

  useEffect(() => {
    void requestPage({ direction: "initial", filters: appliedFilters.current });
    return () => {
      requestVersion.current += 1;
    };
  }, [requestPage]);

  const updateDraft: React.ChangeEventHandler<HTMLInputElement> = (event) => {
    const { checked, name, type, value } = event.currentTarget;
    if (
      name !== "keyword" &&
      name !== "customer-owner-staff-id" &&
      name !== "customer-stage-id" &&
      name !== "customer-channel-id" &&
      name !== "customer-tag-id" &&
      name !== "addedAfter" &&
      name !== "addedBefore" &&
      name !== "lastInteractAfter" &&
      name !== "lastInteractBefore" &&
      name !== "limit" &&
      name !== "isDeleted"
    ) {
      return;
    }
    const key =
      name === "customer-owner-staff-id"
        ? "ownerStaffID"
        : name === "customer-stage-id"
          ? "stageID"
          : name === "customer-channel-id"
            ? "channelID"
            : name === "customer-tag-id"
              ? "tagID"
              : name;
    setDraft((current) =>
      key === "isDeleted"
        ? { ...current, isDeleted: type === "checkbox" && checked }
        : { ...current, [key]: value },
    );
  };

  const submitFilters: React.FormEventHandler<HTMLFormElement> = (event) => {
    event.preventDefault();
    const parsed = parseCustomerListFilters(draft);
    if (!parsed.ok) {
      setFilterError("筛选条件格式无效，未发送请求。");
      return;
    }
    setFilterError(undefined);
    appliedFilters.current = parsed.filters;
    void requestPage({ direction: "initial", filters: parsed.filters });
  };

  const clearFilters = () => {
    const clearedDraft = { ...defaultCustomerListFilterDraft };
    const parsed = parseCustomerListFilters(clearedDraft);
    if (!parsed.ok) return;
    setDraft(clearedDraft);
    setFilterError(undefined);
    appliedFilters.current = parsed.filters;
    void requestPage({ direction: "initial", filters: parsed.filters });
  };

  const retry = () => {
    void requestPage(lastRequest.current);
  };

  const loadNextPage = () => {
    if (
      screen.kind !== "ready" ||
      !screen.page.nextCursor ||
      screen.loadingNextPage
    ) {
      return;
    }
    const nextHistoryEntry =
      pageHistory.current[pageHistoryIndex.current + 1];
    if (nextHistoryEntry) {
      requestVersion.current += 1;
      pageHistoryIndex.current += 1;
      setScreen({
        kind: "ready",
        hasPreviousPage: true,
        loadingNextPage: false,
        page: nextHistoryEntry.page,
        pageNumber: pageHistoryIndex.current + 1,
      });
      return;
    }
    void requestPage({
      direction: "next",
      cursor: screen.page.nextCursor,
      filters: appliedFilters.current,
    });
  };

  const loadPreviousPage = () => {
    if (
      screen.kind !== "ready" ||
      screen.loadingNextPage ||
      pageHistoryIndex.current === 0
    ) {
      return;
    }
    requestVersion.current += 1;
    pageHistoryIndex.current -= 1;
    const page = pageHistory.current[pageHistoryIndex.current]?.page;
    if (!page) return;
    setScreen({
      kind: "ready",
      hasPreviousPage: pageHistoryIndex.current > 0,
      loadingNextPage: false,
      page,
      pageNumber: pageHistoryIndex.current + 1,
    });
  };

  return (
    <section className="customer-list" aria-labelledby="app-title">
      <div className="customer-list__heading">
        <div>
          <p className="route-card__eyebrow">客户</p>
          <h1 id="app-title">客户列表</h1>
          <p>使用服务端 keyset 游标分页；筛选和可见范围均由服务端执行。</p>
        </div>
        <p className="customer-list__role">当前角色：{roleLabels[role]}</p>
      </div>
      <CustomerListFiltersForm
        draft={draft}
        error={filterError}
        loading={screen.kind === "loading"}
        onClear={clearFilters}
        onInputChange={updateDraft}
        onSubmit={submitFilters}
      />
      <CustomerListContent
        role={role}
        screen={screen}
        onNextPage={loadNextPage}
        onPreviousPage={loadPreviousPage}
        onRetry={retry}
        onCustomerNavigate={onCustomerNavigate}
      />
    </section>
  );
}
