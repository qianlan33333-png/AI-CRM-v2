import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  EMPTY_PUSH_CENTER_FILTER_DRAFT,
  PUSH_CENTER_FILTER_KEYS,
  PUSH_CENTER_SECTION_KEYS,
  PUSH_CENTER_STATUS_KEYS,
  generatedPushCenterTransport,
  loadPushCenterSnapshot,
  normalizePushCenterFilters,
  pushCenterFilterKey,
  type PushCenterFailure,
  type PushCenterFilterDraft,
  type PushCenterFilterKey,
  type PushCenterFilters,
  type PushCenterInternalEventSummary,
  type PushCenterRole,
  type PushCenterRuntimeQueue,
  type PushCenterSnapshot,
  type PushCenterTransport,
} from "./push-center";
import "./push-center.css";

const messages: Record<PushCenterFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有推送中心本地运营观测访问权限。",
  invalid: "两路推送中心响应未通过完整 closed contract 校验，已整次拒绝。",
  unavailable: "推送中心本地观测暂不可用；当前未拼接任何半成功数据。",
};

const sectionLabels: Readonly<Record<string, string>> = {
  questionnaire: "问卷外推",
  order: "订单外推",
  ai_assist: "AI 助手",
  private_broadcast: "私信群发",
  group_ops: "群自动化运营",
  group_broadcast: "群群发",
  customer_webhook: "客户自动化 Webhook",
  tags: "企微标签",
  welcome: "欢迎语",
  payment: "支付查询",
  integrations: "集成推送",
  test_receiver: "测试接收端",
  other: "其他",
};

export type PushCenterState =
  | {
      readonly kind: "loading";
      readonly filters: PushCenterFilters;
      readonly previous?: PushCenterSnapshot;
    }
  | { readonly kind: "ready"; readonly snapshot: PushCenterSnapshot }
  | {
      readonly kind: "error";
      readonly failure: PushCenterFailure;
      readonly filters: PushCenterFilters;
      readonly previous?: PushCenterSnapshot;
    };

interface ActivePushCenterRead {
  readonly key: string;
  readonly token: number;
  readonly abortController: AbortController;
  readonly promise: Promise<void>;
}

export interface PushCenterReadController {
  readonly generation: { current: number };
  readonly active: { current: ActivePushCenterRead | undefined };
  readonly verified: { current: PushCenterSnapshot | undefined };
  readonly mounted: { current: boolean };
  readonly unauthenticatedNotified: { current: boolean };
  // eslint-disable-next-line no-unused-vars -- named callback parameter documents the state sink.
  readonly onState: (state: PushCenterState) => void;
  readonly onUnauthenticated?: () => void;
}

export function invalidatePushCenterRead(
  controller: PushCenterReadController,
): void {
  controller.mounted.current = false;
  controller.generation.current += 1;
  controller.active.current?.abortController.abort();
  controller.active.current = undefined;
}

export function startPushCenterRead(
  controller: PushCenterReadController,
  transport: PushCenterTransport,
  filters: PushCenterFilters,
): Promise<void> {
  if (!controller.mounted.current) return Promise.resolve();
  const normalized = normalizePushCenterFilters(filters);
  if (normalized.status !== "valid") {
    controller.onState({
      kind: "error",
      failure: "invalid",
      filters: {},
      previous: controller.verified.current,
    });
    return Promise.resolve();
  }
  const existing = controller.active.current;
  if (existing?.key === normalized.key) return existing.promise;

  existing?.abortController.abort();
  const abortController = new AbortController();
  const token = ++controller.generation.current;
  controller.onState({
    kind: "loading",
    filters: normalized.filters,
    previous: controller.verified.current,
  });

  const promise = (async () => {
    try {
      const result = await loadPushCenterSnapshot(
        normalized.filters,
        transport,
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
    key: normalized.key,
    token,
    abortController,
    promise,
  };
  return promise;
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 }).format(
    value,
  );
}

function InternalEventSummary({
  value,
}: {
  readonly value: PushCenterInternalEventSummary;
}): React.ReactElement {
  const items = [
    ["raw_open", value.rawOpen],
    ["raw_due", value.rawDue],
    ["eligible", value.eligible],
    ["failed_retryable", value.failedRetryable],
    ["failed_terminal", value.failedTerminal],
    ["blocked", value.blocked],
  ] as const;
  return (
    <dl className="push-center__compact-metrics">
      {items.map(([label, count]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{formatNumber(count)}</dd>
        </div>
      ))}
    </dl>
  );
}

function RuntimeQueue({
  value,
}: {
  readonly value: PushCenterRuntimeQueue;
}): React.ReactElement {
  if (value.status === "not_reported") {
    return (
      <p className="push-center__boundary" role="status">
        runtime_queue 当前为 closed empty object：仅表示该只读接口未报告队列摘要，不代表队列为空、任务完成或 Provider 成功。
      </p>
    );
  }
  return (
    <div className="push-center__stack">
      <dl className="push-center__compact-metrics">
        <div>
          <dt>policy_version</dt>
          <dd>{value.policyVersion}</dd>
        </div>
        <div>
          <dt>active_generation</dt>
          <dd>{value.activeGeneration}</dd>
        </div>
        <div>
          <dt>claim_enabled</dt>
          <dd>{String(value.claimEnabled)}</dd>
        </div>
        <div>
          <dt>rollout_mode</dt>
          <dd>{value.rolloutMode}</dd>
        </div>
        <div>
          <dt>raw_open</dt>
          <dd>{formatNumber(value.rawOpen)}</dd>
        </div>
        <div>
          <dt>eligible</dt>
          <dd>{formatNumber(value.eligible)}</dd>
        </div>
        <div>
          <dt>in_flight</dt>
          <dd>{formatNumber(value.inFlight)}</dd>
        </div>
        <div>
          <dt>dlq</dt>
          <dd>{formatNumber(value.dlq)}</dd>
        </div>
      </dl>
      <section aria-labelledby="push-center-internal-events-title">
        <h3 id="push-center-internal-events-title">本地 internal event summary</h3>
        <InternalEventSummary value={value.internalEvent} />
      </section>
      <div className="push-center__table-wrap">
        <table>
          <caption>本地 runtime lane summary</caption>
          <thead>
            <tr>
              <th>lane</th>
              <th>启用</th>
              <th>模式</th>
              <th>open / eligible / in-flight</th>
              <th>等待与异常</th>
              <th>内部事件</th>
              <th>本地性能观测</th>
            </tr>
          </thead>
          <tbody>
            {value.lanes.map((lane) => (
              <tr key={lane.lane}>
                <th scope="row">{lane.lane}</th>
                <td>
                  {String(lane.enabled)} · max {lane.maxInFlight}
                </td>
                <td>
                  {lane.rolloutMode}
                  <br />
                  {lane.blockedUntil ?? "未阻塞"}
                </td>
                <td>
                  {lane.rawOpen} / {lane.eligible} / {lane.inFlight}
                </td>
                <td>
                  held {lane.held} · gated {lane.policyGated} · retry {lane.retryWait}
                  <br />
                  rate limited {lane.rateLimited} · unknown {lane.unknown} · dlq {lane.dlq}
                </td>
                <td>
                  due {lane.internalEvent.rawDue} · eligible {lane.internalEvent.eligible}
                  <br />
                  retryable {lane.internalEvent.failedRetryable} · terminal {lane.internalEvent.failedTerminal}
                </td>
                <td>
                  throughput {formatNumber(lane.throughputLastMinute)} / min
                  <br />
                  p95 queue {formatNumber(lane.p95QueueWaitMs)} ms · provider-call observation {formatNumber(lane.p95ProviderCallMs)} ms
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function AppliedFilters({
  filters,
}: {
  readonly filters: PushCenterFilters;
}): React.ReactElement {
  const entries = PUSH_CENTER_FILTER_KEYS.flatMap((key) =>
    filters[key] === undefined ? [] : ([[key, filters[key]]] as const),
  );
  return (
    <section aria-labelledby="push-center-applied-filters-title">
      <h2 id="push-center-applied-filters-title">已验证筛选</h2>
      {entries.length === 0 ? (
        <p>未应用本地筛选。</p>
      ) : (
        <dl className="push-center__compact-metrics">
          {entries.map(([key, value]) => (
            <div key={key}>
              <dt>{key}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}

function Snapshot({
  snapshot,
  stale,
}: {
  readonly snapshot: PushCenterSnapshot;
  readonly stale: boolean;
}): React.ReactElement {
  return (
    <div className="push-center__stack" data-stale={stale ? "true" : "false"}>
      {stale ? (
        <p className="push-center__boundary" role="status">
          以下为最近一次已完整验证的快照；当前刷新失败或新筛选尚未完成，筛选范围可能不同。
        </p>
      ) : null}
      <AppliedFilters filters={snapshot.filters} />
      {snapshot.boundary === "degraded" ? (
        <section
          className="push-center__degraded"
          aria-labelledby="push-center-degraded-title"
        >
          <h2 id="push-center-degraded-title">本地读模型 degraded boundary</h2>
          <p>{snapshot.degraded.pageError}</p>
          <dl className="push-center__compact-metrics">
            <div>
              <dt>source_status</dt>
              <dd>{snapshot.degraded.sourceStatus}</dd>
            </div>
            <div>
              <dt>read_model_status</dt>
              <dd>{snapshot.degraded.readModelStatus}</dd>
            </div>
            <div>
              <dt>error_class</dt>
              <dd>{snapshot.degraded.diagnostics.errorClass}</dd>
            </div>
            <div>
              <dt>production_data_ready</dt>
              <dd>{String(snapshot.degraded.diagnostics.productionDataReady)}</dd>
            </div>
          </dl>
          <p>
            degraded 响应没有 13 个分区计数，页面不会补零或拼接另一条响应的数据。
          </p>
        </section>
      ) : (
        <>
          <section aria-labelledby="push-center-counts-title">
            <h2 id="push-center-counts-title">本地状态计数</h2>
            <p className="push-center__normal-boundary">
              正常响应未返回 source_status；这里只确认 sections 与 stats 的本地投影合同已交叉验证。
            </p>
            <dl className="push-center__metrics">
              {[
                ["总数", snapshot.counts.total],
                ["pending", snapshot.counts.pending],
                ["running", snapshot.counts.running],
                ["succeeded", snapshot.counts.succeeded],
                ["sent", snapshot.counts.sent],
                ["failed", snapshot.counts.failed],
                ["shadow_warning", snapshot.counts.shadowWarning],
              ].map(([label, count]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{formatNumber(count as number)}</dd>
                </div>
              ))}
            </dl>
          </section>
          <section aria-labelledby="push-center-sections-title">
            <h2 id="push-center-sections-title">13 个本地 sections</h2>
            <div className="push-center__table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>section</th>
                    <th>标签</th>
                    <th>本地计数</th>
                    <th>capability</th>
                    <th>effect types</th>
                  </tr>
                </thead>
                <tbody>
                  {snapshot.sections.map((section) => (
                    <tr key={section.key}>
                      <th scope="row">{section.key}</th>
                      <td>{section.label}</td>
                      <td>{formatNumber(section.count)}</td>
                      <td>{section.capabilityKey || "—"}</td>
                      <td>{section.effectTypes.join("、") || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
      <section aria-labelledby="push-center-status-definitions-title">
        <h2 id="push-center-status-definitions-title">9 个内部 status definitions</h2>
        <div className="push-center__table-wrap">
          <table>
            <thead>
              <tr>
                <th>status</th>
                <th>本地名称</th>
                <th>定义</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.statusDefinitions.map((definition) => (
                <tr key={definition.key}>
                  <th scope="row">{definition.key}</th>
                  <td>{definition.label}</td>
                  <td>{definition.definition}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      <section aria-labelledby="push-center-runtime-title">
        <h2 id="push-center-runtime-title">runtime queue · 本地只读</h2>
        <RuntimeQueue value={snapshot.runtimeQueue} />
      </section>
      <p className="push-center__boundary">
        real_external_call_executed = {String(snapshot.realExternalCallExecuted)}；该值只描述本次读取接口，不证明历史任务没有外部效果。
      </p>
    </div>
  );
}

function FilterForm({
  draft,
  error,
  onChange,
  onApply,
  onReset,
}: {
  readonly draft: PushCenterFilterDraft;
  readonly error?: string;
  // eslint-disable-next-line no-unused-vars -- callback names document the UI contract.
  readonly onChange: (key: PushCenterFilterKey, value: string) => void;
  readonly onApply: () => void;
  readonly onReset: () => void;
}): React.ReactElement {
  return (
    <form
      className="push-center__filters"
      onSubmit={(event) => {
        event.preventDefault();
        onApply();
      }}
    >
      <div className="push-center__filter-grid">
        <label>
          section
          <select
            value={draft.section}
            onChange={(event) => onChange("section", event.currentTarget.value)}
          >
            <option value="">全部</option>
            {PUSH_CENTER_SECTION_KEYS.map((key) => (
              <option key={key} value={key}>
                {sectionLabels[key]} · {key}
              </option>
            ))}
          </select>
        </label>
        <label>
          effect_type
          <input
            value={draft.effect_type}
            maxLength={256}
            placeholder="例如 wecom.message.private.send"
            onChange={(event) =>
              onChange("effect_type", event.currentTarget.value)
            }
          />
        </label>
        <label>
          status
          <select
            value={draft.status}
            onChange={(event) => onChange("status", event.currentTarget.value)}
          >
            <option value="">全部</option>
            {PUSH_CENTER_STATUS_KEYS.map((key) => (
              <option key={key} value={key}>
                {key}
              </option>
            ))}
          </select>
        </label>
        <label>
          business_type
          <input
            value={draft.business_type}
            maxLength={256}
            onChange={(event) =>
              onChange("business_type", event.currentTarget.value)
            }
          />
        </label>
        <label>
          business_id
          <input
            value={draft.business_id}
            maxLength={256}
            onChange={(event) =>
              onChange("business_id", event.currentTarget.value)
            }
          />
        </label>
        <label>
          source_module
          <input
            value={draft.source_module}
            maxLength={256}
            onChange={(event) =>
              onChange("source_module", event.currentTarget.value)
            }
          />
        </label>
        <label>
          created_from
          <input
            value={draft.created_from}
            maxLength={64}
            placeholder="2026-08-20T00:00:00Z"
            onChange={(event) =>
              onChange("created_from", event.currentTarget.value)
            }
          />
        </label>
        <label>
          created_to
          <input
            value={draft.created_to}
            maxLength={64}
            placeholder="2026-08-20T23:59:59Z"
            onChange={(event) =>
              onChange("created_to", event.currentTarget.value)
            }
          />
        </label>
      </div>
      {error ? <p role="alert">{error}</p> : null}
      <div className="push-center__actions">
        <button type="submit">应用本地筛选</button>
        <button type="button" onClick={onReset}>
          清空筛选
        </button>
      </div>
    </form>
  );
}

export function PushCenterView({
  state,
  draft = EMPTY_PUSH_CENTER_FILTER_DRAFT,
  filterError,
  onDraftChange = () => undefined,
  onApply = () => undefined,
  onReset = () => undefined,
  onRefresh,
}: {
  readonly state: PushCenterState;
  readonly draft?: PushCenterFilterDraft;
  readonly filterError?: string;
  // eslint-disable-next-line no-unused-vars -- callback names document the view contract.
  readonly onDraftChange?: (key: PushCenterFilterKey, value: string) => void;
  readonly onApply?: () => void;
  readonly onReset?: () => void;
  readonly onRefresh: () => void;
}): React.ReactElement {
  const snapshot = state.kind === "ready" ? state.snapshot : state.previous;
  return (
    <section className="route-card push-center" aria-labelledby="app-title">
      <p className="route-card__eyebrow">推送中心 · 本地运营观测 · 只读</p>
      <h1 id="app-title">推送中心</h1>
      <p className="push-center__warning" role="note">
        本页的 completed、本地计数、队列摘要或 receipt 只代表本地投影/本地处理事实，不代表 Provider 已执行、已送达或业务成功。
      </p>
      <p>
        仅展示本地分区聚合计数；页面只并行读取既有 sections 与 stats 客户端；不提供取消、重新执行、回放或发送控制。
      </p>
      <FilterForm
        draft={draft}
        error={filterError}
        onChange={onDraftChange}
        onApply={onApply}
        onReset={onReset}
      />
      {state.kind === "loading" ? (
        <p role="status">正在用同一组筛选交叉读取 sections 与 stats。</p>
      ) : null}
      {state.kind === "error" ? (
        <p className="push-center__error" role="alert">
          {messages[state.failure]}
        </p>
      ) : null}
      {snapshot ? (
        <Snapshot snapshot={snapshot} stale={state.kind !== "ready"} />
      ) : null}
      <div className="push-center__actions">
        <button
          type="button"
          disabled={state.kind === "loading"}
          onClick={onRefresh}
        >
          刷新本地观测
        </button>
      </div>
    </section>
  );
}

export function PushCenterPage({
  role,
  transport = generatedPushCenterTransport,
  onUnauthenticated,
}: {
  readonly role: PushCenterRole;
  readonly transport?: PushCenterTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0);
  const active = useRef<ActivePushCenterRead>();
  const verified = useRef<PushCenterSnapshot>();
  const mounted = useRef(true);
  const unauthenticatedNotified = useRef(false);
  const [draft, setDraft] = useState<PushCenterFilterDraft>(
    EMPTY_PUSH_CENTER_FILTER_DRAFT,
  );
  const [appliedFilters, setAppliedFilters] = useState<PushCenterFilters>({});
  const [filterError, setFilterError] = useState<string>();
  const [state, setState] = useState<PushCenterState>({
    kind: "loading",
    filters: {},
  });
  const appliedKey = useMemo(
    () => pushCenterFilterKey(appliedFilters),
    [appliedFilters],
  );

  const controller = useCallback(
    (): PushCenterReadController => ({
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
    (filters: PushCenterFilters) =>
      startPushCenterRead(controller(), transport, filters),
    [controller, transport],
  );

  useEffect(() => {
    if (!canRead) return undefined;
    mounted.current = true;
    void load(appliedFilters);
    return () => invalidatePushCenterRead(controller());
  }, [appliedKey, canRead, controller, load, appliedFilters]);

  if (!canRead) {
    return (
      <section className="route-card" aria-labelledby="app-title">
        <p className="route-card__eyebrow">推送中心 · 权限边界</p>
        <h1 id="app-title">推送中心</h1>
        <p>当前账号没有推送中心本地运营观测访问权限。</p>
      </section>
    );
  }

  const applyDraft = (): void => {
    const normalized = normalizePushCenterFilters(draft);
    if (normalized.status !== "valid") {
      setFilterError(normalized.message);
      return;
    }
    setFilterError(undefined);
    if (normalized.key === appliedKey) {
      void load(normalized.filters);
      return;
    }
    setAppliedFilters(normalized.filters);
  };

  return (
    <PushCenterView
      state={state}
      draft={draft}
      filterError={filterError}
      onDraftChange={(key, value) => {
        setDraft((current) => ({ ...current, [key]: value }));
        setFilterError(undefined);
      }}
      onApply={applyDraft}
      onReset={() => {
        setDraft(EMPTY_PUSH_CENTER_FILTER_DRAFT);
        setFilterError(undefined);
        if (appliedKey === pushCenterFilterKey({})) void load({});
        else setAppliedFilters({});
      }}
      onRefresh={() => {
        void load(appliedFilters);
      }}
    />
  );
}
