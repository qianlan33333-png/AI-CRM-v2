import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  DATA_HEALTH_CHECK_IDS,
  generatedDataHealthTransport,
  loadDataHealthDetail,
  loadDataHealthOverview,
  type DataHealthCheck,
  type DataHealthCheckID,
  type DataHealthDetail,
  type DataHealthFailure,
  type DataHealthRegistry,
  type DataHealthRole,
  type DataHealthSummary,
  type DataHealthTransport,
} from "./data-health";

const messages: Record<DataHealthFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有数据健康访问权限。",
  not_found: "所选数据健康检查不存在。",
  invalid: "数据健康响应不符合已冻结的本地只读合同。",
  unavailable: "数据健康本地观测暂时不可用，请稍后手动刷新。",
};

export type DataHealthOverviewState =
  | { readonly kind: "loading"; readonly previous?: DataHealthOverview }
  | { readonly kind: "ready"; readonly overview: DataHealthOverview }
  | {
      readonly kind: "error";
      readonly failure: DataHealthFailure;
      readonly previous?: DataHealthOverview;
    };

export interface DataHealthOverview {
  readonly registry: DataHealthRegistry;
  readonly summary: DataHealthSummary;
}

export type DataHealthDetailState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly checkID: DataHealthCheckID;
      readonly previous?: DataHealthDetail;
    }
  | { readonly kind: "ready"; readonly detail: DataHealthDetail }
  | {
      readonly kind: "error";
      readonly checkID: DataHealthCheckID;
      readonly failure: DataHealthFailure;
      readonly previous?: DataHealthDetail;
    };

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

function Evidence({
  evidence,
}: {
  readonly evidence: DataHealthCheck["evidence"];
}): React.ReactElement {
  return (
    <ul>
      {Object.entries(evidence).map(([key, value]) => (
        <li key={key}>
          {key}: {String(value)}
        </li>
      ))}
    </ul>
  );
}

function CheckDetail({
  detail,
}: {
  readonly detail: DataHealthDetail;
}): React.ReactElement {
  const { check } = detail;
  return (
    <section aria-labelledby="data-health-detail-title">
      <h2 id="data-health-detail-title">检查详情：{check.title}</h2>
      <p>
        状态：{check.status}；门禁：{check.gateDecision}；原因：
        {check.reasonCode}
      </p>
      <p>{check.summary}</p>
      <p>处置：{check.remediation}</p>
      <p>证据：</p>
      <Evidence evidence={check.evidence} />
      <p>本次详情观测时间：{displayDate(detail.observedAt)}</p>
    </section>
  );
}

function RegistryRows({
  registry,
  detailState,
  onDetail,
}: {
  readonly registry: DataHealthRegistry;
  readonly detailState: DataHealthDetailState;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onDetail: (checkID: DataHealthCheckID) => void;
}): React.ReactElement {
  return (
    <section aria-labelledby="data-health-registry-title">
      <h2 id="data-health-registry-title">固定本地检查注册表</h2>
      <p>注册表观测时间：{displayDate(registry.observedAt)}</p>
      <p>
        注册表：{registry.registryID}；已排除 legacy 检查{" "}
        {registry.excludedLegacyCheckIDs.length} 项。
      </p>
      <table>
        <thead>
          <tr>
            <th>检查</th>
            <th>状态</th>
            <th>门禁</th>
            <th>摘要</th>
            <th>本地操作</th>
          </tr>
        </thead>
        <tbody>
          {registry.checks.map((check) => (
            <tr key={check.checkID}>
              <td>{check.title}</td>
              <td>{check.status}</td>
              <td>{check.gateDecision}</td>
              <td>{check.summary}</td>
              <td>
                <button
                  type="button"
                  disabled={
                    detailState.kind === "loading" &&
                    detailState.checkID === check.checkID
                  }
                  onClick={() => onDetail(check.checkID)}
                >
                  查看检查详情
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

export function DataHealthView({
  overviewState,
  detailState,
  onLoad,
  onDetail,
}: {
  readonly overviewState: DataHealthOverviewState;
  readonly detailState: DataHealthDetailState;
  readonly onLoad: () => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onDetail: (checkID: DataHealthCheckID) => void;
}): React.ReactElement {
  const overview =
    overviewState.kind === "ready"
      ? overviewState.overview
      : overviewState.previous;
  const detail =
    detailState.kind === "ready"
      ? detailState.detail
      : detailState.kind === "loading" || detailState.kind === "error"
        ? detailState.previous
        : undefined;
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">平台就绪度 · 本地只读</p>
      <h1 id="app-title">数据健康</h1>
      <p>
        只读取四项本地就绪度检查；不触发外部调用，也不代表任一请求组成原子快照。
      </p>
      {overview ? (
        <>
          <section aria-labelledby="data-health-summary-title">
            <h2 id="data-health-summary-title">汇总</h2>
            <p>汇总观测时间：{displayDate(overview.summary.observedAt)}</p>
            <p>
              总体状态：{overview.summary.overallStatus}；通过{" "}
              {overview.summary.gateCounts.pass}，警告{" "}
              {overview.summary.gateCounts.warn}，阻断{" "}
              {overview.summary.gateCounts.block}。
            </p>
            <p>
              检查计数：正常 {overview.summary.counts.ok}，警告{" "}
              {overview.summary.counts.warn}，失败{" "}
              {overview.summary.counts.fail}。
            </p>
          </section>
          <RegistryRows
            registry={overview.registry}
            detailState={detailState}
            onDetail={onDetail}
          />
        </>
      ) : null}
      {overviewState.kind === "loading" ? (
        <p role="status">正在读取本地数据健康观测。</p>
      ) : null}
      {overviewState.kind === "error" ? (
        <p role="alert">{messages[overviewState.failure]}</p>
      ) : null}
      <p>
        <button
          type="button"
          disabled={overviewState.kind === "loading"}
          onClick={onLoad}
        >
          手动刷新本地观测
        </button>
      </p>
      {detailState.kind === "loading" ? (
        <p role="status">正在读取检查详情。</p>
      ) : null}
      {detailState.kind === "error" ? (
        <p role="alert">{messages[detailState.failure]}</p>
      ) : null}
      {detail ? <CheckDetail detail={detail} /> : null}
    </section>
  );
}

export function DataHealthPage({
  role,
  transport = generatedDataHealthTransport,
  onUnauthenticated,
}: {
  readonly role: DataHealthRole;
  readonly transport?: DataHealthTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const overviewGeneration = useRef(0);
  const detailGeneration = useRef(0);
  const verifiedOverview = useRef<DataHealthOverview>();
  const verifiedDetail = useRef<DataHealthDetail>();
  const [overviewState, setOverviewState] = useState<DataHealthOverviewState>({
    kind: "loading",
  });
  const [detailState, setDetailState] = useState<DataHealthDetailState>({
    kind: "idle",
  });

  const loadOverview = useCallback(async () => {
    const currentGeneration = ++overviewGeneration.current;
    setOverviewState({ kind: "loading", previous: verifiedOverview.current });
    const result = await loadDataHealthOverview(transport);
    if (currentGeneration !== overviewGeneration.current) return;
    if (result.status === "loaded") {
      const overview = { registry: result.registry, summary: result.summary };
      verifiedOverview.current = overview;
      setOverviewState({ kind: "ready", overview });
      return;
    }
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setOverviewState({
      kind: "error",
      failure: result.status,
      previous: verifiedOverview.current,
    });
  }, [onUnauthenticated, transport]);

  const loadDetail = useCallback(
    async (checkID: DataHealthCheckID) => {
      if (!DATA_HEALTH_CHECK_IDS.includes(checkID)) return;
      const currentGeneration = ++detailGeneration.current;
      const previous =
        verifiedDetail.current?.check.checkID === checkID
          ? verifiedDetail.current
          : undefined;
      setDetailState({ kind: "loading", checkID, previous });
      const result = await loadDataHealthDetail(transport, checkID);
      if (currentGeneration !== detailGeneration.current) return;
      if (result.status === "loaded") {
        verifiedDetail.current = result.detail;
        setDetailState({ kind: "ready", detail: result.detail });
        return;
      }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setDetailState({
        kind: "error",
        checkID,
        failure: result.status,
        previous,
      });
    },
    [onUnauthenticated, transport],
  );

  useEffect(() => {
    if (canRead) void loadOverview();
    return () => {
      overviewGeneration.current += 1;
      detailGeneration.current += 1;
    };
  }, [canRead, loadOverview]);

  if (!canRead) {
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">数据健康</h1>
        <p>当前账号没有数据健康访问权限。</p>
      </section>
    );
  }

  return (
    <DataHealthView
      overviewState={overviewState}
      detailState={detailState}
      onLoad={() => void loadOverview()}
      onDetail={(checkID) => void loadDetail(checkID)}
    />
  );
}
