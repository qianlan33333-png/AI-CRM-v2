import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT,
  EXTERNAL_EFFECT_CLASSIFICATIONS,
  EXTERNAL_EFFECT_STATUSES,
  externalEffectsReadKey,
  loadExternalEffectsSnapshot,
  normalizeExternalEffectsFilters,
  type ExternalEffectClassification,
  type ExternalEffectsFailure,
  type ExternalEffectsFilterDraft,
  type ExternalEffectsFilters,
  type ExternalEffectsRole,
  type ExternalEffectsSnapshot,
  type ExternalEffectsTransport,
} from "./external-effects";
import "./external-effects.css";

const failureMessages: Readonly<Record<ExternalEffectsFailure, string>> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有 External Effects 本地诊断读取权限。",
  invalid: "返回值未通过完整 closed contract 校验，本次结果已整次拒绝。",
  unavailable: "External Effects 本地读取暂不可用，未拼接半成功数据。",
};

const statusLabels: Readonly<Record<string, string>> = {
  pending: "本地待处理",
  sending: "本地处理中（冻结）",
  sent: "本地 sent 事实（非送达证明）",
  retryable_failed: "本地可恢复失败",
  final_failed: "本地最终失败",
  outcome_unknown: "外部结果未知",
  cancelled: "本地已取消",
};

const classificationLabels: Readonly<Record<ExternalEffectClassification, string>> = {
  safe_local_handling: "可安全本地处理",
  frozen: "冻结",
  manual_review: "需要人工复核",
};

export type ExternalEffectsState =
  | {
    readonly kind: "loading";
    readonly filters: ExternalEffectsFilters;
    readonly cursor?: string;
    readonly previous?: ExternalEffectsSnapshot;
  }
  | { readonly kind: "ready"; readonly snapshot: ExternalEffectsSnapshot; }
  | {
    readonly kind: "error";
    readonly failure: ExternalEffectsFailure;
    readonly filters: ExternalEffectsFilters;
    readonly cursor?: string;
    readonly previous?: ExternalEffectsSnapshot;
  };

interface ActiveExternalEffectsRead {
  readonly key: string;
  readonly token: number;
  readonly abortController: AbortController;
  readonly promise: Promise<void>;
}

export interface ExternalEffectsReadController {
  readonly generation: { current: number; };
  readonly active: { current: ActiveExternalEffectsRead | undefined; };
  readonly verified: { current: ExternalEffectsSnapshot | undefined; };
  readonly mounted: { current: boolean; };
  readonly unauthenticatedNotified: { current: boolean; };
  readonly onState: (...[]: [ExternalEffectsState]) => void;
  readonly onUnauthenticated?: () => void;
}

export function invalidateExternalEffectsRead(
  controller: ExternalEffectsReadController,
): void {
  controller.mounted.current = false;
  controller.generation.current += 1;
  controller.active.current?.abortController.abort();
  controller.active.current = undefined;
}

export function startExternalEffectsRead(
  controller: ExternalEffectsReadController,
  transport: ExternalEffectsTransport,
  filters: ExternalEffectsFilters,
  cursor?: string,
): Promise<void> {
  if (!controller.mounted.current) return Promise.resolve();
  const normalized = normalizeExternalEffectsFilters(filters);
  if (normalized.status !== "valid") {
    controller.onState({
      kind: "error",
      failure: "invalid",
      filters: {},
      previous: controller.verified.current,
    });
    return Promise.resolve();
  }
  const key = externalEffectsReadKey(normalized.filters, cursor);
  if (key === "invalid") {
    controller.onState({
      kind: "error",
      failure: "invalid",
      filters: normalized.filters,
      previous: controller.verified.current,
    });
    return Promise.resolve();
  }
  const existing = controller.active.current;
  if (existing?.key === key) return existing.promise;

  existing?.abortController.abort();
  const abortController = new AbortController();
  const token = ++controller.generation.current;
  controller.onState({
    kind: "loading",
    filters: normalized.filters,
    cursor,
    previous: controller.verified.current,
  });

  const promise = (async () => {
    try {
      const result = await loadExternalEffectsSnapshot(
        transport,
        normalized.filters,
        cursor,
        abortController.signal,
      );
      if (
        !controller.mounted.current ||
        token !== controller.generation.current
      ) {
        return;
      }
      if (result.status === "loaded") {
        controller.verified.current = result.snapshot;
        controller.onState({ kind: "ready", snapshot: result.snapshot });
        return;
      }
      if (
        result.status === "unauthenticated" &&
        !controller.unauthenticatedNotified.current
      ) {
        controller.unauthenticatedNotified.current = true;
        controller.onUnauthenticated?.();
      }
      controller.onState({
        kind: "error",
        failure: result.status,
        filters: normalized.filters,
        cursor,
        previous: controller.verified.current,
      });
    } finally {
      if (
        token === controller.generation.current &&
        controller.active.current?.token === token
      ) {
        controller.active.current = undefined;
      }
    }
  })();

  controller.active.current = {
    key,
    token,
    abortController,
    promise,
  };
  return promise;
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
    timeZone: "Asia/Singapore",
  }).format(new Date(value));
}

function externalEffectStatusCount(
  snapshot: ExternalEffectsSnapshot,
  status: (typeof EXTERNAL_EFFECT_STATUSES)[number],
): number {
  switch (status) {
    case "pending":
      return snapshot.diagnostics.byStatus.pending;
    case "sending":
      return snapshot.diagnostics.byStatus.sending;
    case "sent":
      return snapshot.diagnostics.byStatus.sent;
    case "retryable_failed":
      return snapshot.diagnostics.byStatus.retryableFailed;
    case "final_failed":
      return snapshot.diagnostics.byStatus.finalFailed;
    case "outcome_unknown":
      return snapshot.diagnostics.byStatus.outcomeUnknown;
    case "cancelled":
      return snapshot.diagnostics.byStatus.cancelled;
  }
}

function SafetyBanner(): React.ReactElement {
  return (
    <div className="external-effects__safety" role="note">
      <strong>仅展示本地诊断事实。</strong>
      <span>
        provider_execution_eligible=false；real_external_call_executed=false；所有数量、
        pending/sending/sent 等状态都不构成送达证明。
      </span>
    </div>
  );
}

function Diagnostics({
  snapshot,
}: {
  readonly snapshot: ExternalEffectsSnapshot;
}): React.ReactElement {
  const value = snapshot.diagnostics;
  return (
    <section aria-labelledby="external-effects-diagnostics-title">
      <div className="external-effects__heading">
        <div>
          <p className="external-effects__eyebrow">本地计数与风险摘要</p>
          <h2 id="external-effects-diagnostics-title">诊断概览</h2>
        </div>
        <time dateTime={value.generatedAt}>{formatTime(value.generatedAt)}</time>
      </div>
      <div className="external-effects__metrics">
        <article>
          <span>本地任务总数</span>
          <strong>{formatNumber(value.total)}</strong>
        </article>
        <article>
          <span>可安全本地处理</span>
          <strong>{formatNumber(value.byClassification.safeLocalHandling)}</strong>
        </article>
        <article>
          <span>冻结</span>
          <strong>{formatNumber(value.byClassification.frozen)}</strong>
        </article>
        <article>
          <span>人工复核</span>
          <strong>{formatNumber(value.byClassification.manualReview)}</strong>
        </article>
      </div>
      <div className="external-effects__risk" data-level={value.risk.level}>
        <strong>风险级别：{value.risk.level}</strong>
        <span>
          outcome_unknown {formatNumber(value.risk.outcomeUnknownCount)} 项；人工复核汇总 {" "}
          {formatNumber(value.risk.manualReviewCount)} 项。
        </span>
      </div>
      <dl className="external-effects__status-counts">
        {EXTERNAL_EFFECT_STATUSES.map((status) => {
          const count = externalEffectStatusCount(snapshot, status);
          return (
            <div key={status}>
              <dt>{statusLabels[status]}</dt>
              <dd>{formatNumber(count)}</dd>
            </div>
          );
        })}
      </dl>
    </section>
  );
}

function Jobs({
  snapshot,
}: {
  readonly snapshot: ExternalEffectsSnapshot;
}): React.ReactElement {
  return (
    <section aria-labelledby="external-effects-jobs-title">
      <div className="external-effects__heading">
        <div>
          <p className="external-effects__eyebrow">脱敏 cursor 分页</p>
          <h2 id="external-effects-jobs-title">External Effects Jobs</h2>
        </div>
        <span>{snapshot.page.items.length} 条本地事实</span>
      </div>
      {snapshot.page.items.length === 0 ? (
        <p className="external-effects__empty">当前筛选没有本地任务事实。</p>
      ) : (
        <div className="external-effects__table-wrap">
          <table>
            <thead>
              <tr>
                <th>不可逆内部 ID</th>
                <th>本地状态</th>
                <th>安全分类</th>
                <th>尝试计数</th>
                <th>状态更新时间</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.page.items.map((item) => (
                <tr key={item.id}>
                  <td><code>{item.id}</code></td>
                  <td>{statusLabels[item.status]}</td>
                  <td>{classificationLabels[item.classification]}</td>
                  <td>{formatNumber(item.attemptCount)}</td>
                  <td><time dateTime={item.statusUpdatedAt}>{formatTime(item.statusUpdatedAt)}</time></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function FilterForm({
  draft,
  error,
  onChange,
  onApply,
  onReset,
}: {
  readonly draft: ExternalEffectsFilterDraft;
  readonly error?: string;
  readonly onChange: (
    ...[]: [keyof ExternalEffectsFilterDraft, string]
  ) => void;
  readonly onApply: () => void;
  readonly onReset: () => void;
}): React.ReactElement {
  return (
    <form
      className="external-effects__filters"
      onSubmit={(event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        onApply();
      }}
    >
      <label>
        本地状态
        <select
          value={draft.status}
          onChange={(event: React.ChangeEvent<HTMLSelectElement>) =>
            onChange("status", event.currentTarget.value)
          }
        >
          <option value="">全部批准状态</option>
          {EXTERNAL_EFFECT_STATUSES.map((value) => (
            <option key={value} value={value}>{statusLabels[value]}</option>
          ))}
        </select>
      </label>
      <label>
        本地安全分类
        <select
          value={draft.classification}
          onChange={(event: React.ChangeEvent<HTMLSelectElement>) =>
            onChange("classification", event.currentTarget.value)
          }
        >
          <option value="">全部分类</option>
          {EXTERNAL_EFFECT_CLASSIFICATIONS.map((value) => (
            <option key={value} value={value}>{classificationLabels[value]}</option>
          ))}
        </select>
      </label>
      <div className="external-effects__filter-actions">
        <button type="submit">应用本地筛选</button>
        <button type="button" onClick={onReset}>清空筛选</button>
      </div>
      {error ? <p role="alert">{error}</p> : null}
    </form>
  );
}

export function ExternalEffectsView({
  state,
  draft = EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT,
  filterError,
  canGoBack = false,
  onDraftChange = () => undefined,
  onApply = () => undefined,
  onReset = () => undefined,
  onRefresh,
  onNext = () => undefined,
  onPrevious = () => undefined,
}: {
  readonly state: ExternalEffectsState;
  readonly draft?: ExternalEffectsFilterDraft;
  readonly filterError?: string;
  readonly canGoBack?: boolean;
  readonly onDraftChange?: (
    ...[]: [keyof ExternalEffectsFilterDraft, string]
  ) => void;
  readonly onApply?: () => void;
  readonly onReset?: () => void;
  readonly onRefresh: () => void;
  readonly onNext?: () => void;
  readonly onPrevious?: () => void;
}): React.ReactElement {
  const snapshot =
    state.kind === "ready" ? state.snapshot : state.previous;
  return (
    <section className="external-effects route-card" aria-labelledby="external-effects-title">
      <p className="external-effects__eyebrow">Outbound · 只读安全诊断</p>
      <h1 id="external-effects-title">External Effects</h1>
      <p className="external-effects__intro">
        只读取 Outbound 本地状态、计数与时间；不展示接收者身份、消息正文、raw payload、
        Provider token 或 receipt 原文。
      </p>
      <SafetyBanner />
      <FilterForm
        draft={draft}
        error={filterError}
        onChange={onDraftChange}
        onApply={onApply}
        onReset={onReset}
      />
      {state.kind === "loading" ? <p role="status">正在读取 jobs 与 diagnostics。</p> : null}
      {state.kind === "error" ? (
        <p className="external-effects__error" role="alert">{failureMessages[state.failure]}</p>
      ) : null}
      {snapshot ? (
        <div className="external-effects__snapshot" data-stale={state.kind !== "ready" ? "true" : "false"}>
          {state.kind !== "ready" ? <p className="external-effects__stale">以下为最近一次已验证快照。</p> : null}
          <Diagnostics snapshot={snapshot} />
          <Jobs snapshot={snapshot} />
        </div>
      ) : null}
      <div className="external-effects__actions">
        <button type="button" disabled={state.kind === "loading"} onClick={onRefresh}>刷新本地诊断</button>
        <button type="button" disabled={!canGoBack || state.kind === "loading"} onClick={onPrevious}>上一页</button>
        <button
          type="button"
          disabled={!snapshot?.page.nextCursor || state.kind === "loading"}
          onClick={onNext}
        >下一页</button>
      </div>
    </section>
  );
}

export function ExternalEffectsPage({
  role,
  transport,
  onUnauthenticated,
}: {
  readonly role: ExternalEffectsRole;
  readonly transport: ExternalEffectsTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin" || role === "ops";
  const generation = useRef(0);
  const active = useRef<ActiveExternalEffectsRead>();
  const verified = useRef<ExternalEffectsSnapshot>();
  const mounted = useRef(true);
  const unauthenticatedNotified = useRef(false);
  const [draft, setDraft] = useState<ExternalEffectsFilterDraft>(
    EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT,
  );
  const [filters, setFilters] = useState<ExternalEffectsFilters>({});
  const [cursor, setCursor] = useState<string>();
  const [history, setHistory] = useState<readonly (string | undefined)[]>([]);
  const [filterError, setFilterError] = useState<string>();
  const [state, setState] = useState<ExternalEffectsState>({
    kind: "loading",
    filters: {},
  });
  const readKey = useMemo(
    () => externalEffectsReadKey(filters, cursor),
    [filters, cursor],
  );

  const controller = useCallback(
    (): ExternalEffectsReadController => ({
      generation,
      active,
      verified,
      mounted,
      unauthenticatedNotified,
      onState: setState,
      onUnauthenticated,
    }),
    [onUnauthenticated],
  );
  const load = useCallback(
    (nextFilters: ExternalEffectsFilters, nextCursor?: string) =>
      startExternalEffectsRead(
        controller(),
        transport,
        nextFilters,
        nextCursor,
      ),
    [controller, transport],
  );

  useEffect(() => {
    if (!canRead) return undefined;
    mounted.current = true;
    void load(filters, cursor);
    return () => invalidateExternalEffectsRead(controller());
  }, [canRead, controller, cursor, filters, load, readKey]);

  if (!canRead) {
    return (
      <section className="route-card" aria-labelledby="external-effects-title">
        <p className="external-effects__eyebrow">External Effects · 权限边界</p>
        <h1 id="external-effects-title">External Effects</h1>
        <p>当前账号没有 External Effects 本地诊断读取权限。</p>
      </section>
    );
  }

  const applyDraft = (): void => {
    const normalized = normalizeExternalEffectsFilters(draft);
    if (normalized.status !== "valid") {
      setFilterError(normalized.message);
      return;
    }
    setFilterError(undefined);
    setHistory([]);
    setCursor(undefined);
    if (externalEffectsReadKey(normalized.filters) === externalEffectsReadKey(filters)) {
      void load(normalized.filters);
    } else {
      setFilters(normalized.filters);
    }
  };
  const snapshot = state.kind === "ready" ? state.snapshot : state.previous;

  return (
    <ExternalEffectsView
      state={state}
      draft={draft}
      filterError={filterError}
      canGoBack={history.length > 0}
      onDraftChange={(key, value) => {
        setDraft((current) => ({ ...current, [key]: value }));
        setFilterError(undefined);
      }}
      onApply={applyDraft}
      onReset={() => {
        setDraft(EMPTY_EXTERNAL_EFFECTS_FILTER_DRAFT);
        setFilterError(undefined);
        setHistory([]);
        setCursor(undefined);
        if (externalEffectsReadKey(filters) === externalEffectsReadKey({})) void load({});
        else setFilters({});
      }}
      onRefresh={() => { void load(filters, cursor); }}
      onNext={() => {
        const next = snapshot?.page.nextCursor;
        if (!next) return;
        setHistory((current) => [...current, cursor]);
        setCursor(next);
      }}
      onPrevious={() => {
        setHistory((current) => {
          if (current.length === 0) return current;
          setCursor(current[current.length - 1]);
          return current.slice(0, -1);
        });
      }}
    />
  );
}
