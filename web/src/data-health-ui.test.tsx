import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import {
  DataHealthPage,
  DataHealthView,
  type DataHealthDetailState,
  type DataHealthOverviewState,
} from "./data-health-ui";
import {
  DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
  type DataHealthTransport,
} from "./data-health";

const observedAt = "2026-08-19T08:00:00Z";
const check = {
  checkID: "database_readiness" as const,
  title: "Database readiness",
  status: "ok" as const,
  severity: "green" as const,
  summary: "Local database is readable.",
  evidence: { database_readable: true },
  remediation: "No action required.",
  gateDecision: "pass" as const,
  reasonCode: "database_readable",
  owner: "platform_readiness" as const,
  candidateRelated: false as const,
  firstObservedAt: observedAt,
  lastObservedAt: observedAt,
  replayPolicy: "manual_after_remediation" as const,
};

const overviewState: DataHealthOverviewState = {
  kind: "ready",
  overview: {
    registry: {
      ok: true,
      checks: [
        check,
        {
          ...check,
          checkID: "migration_compatibility",
          title: "Migration compatibility",
        },
        {
          ...check,
          checkID: "outbound_outcome_unknown_backlog",
          title: "Outbound outcome-unknown backlog",
        },
        {
          ...check,
          checkID: "release_sha_complete",
          title: "Release SHA completeness",
        },
      ],
      registryID: "v2-core-readiness.v1",
      registrySHA256: "a".repeat(64),
      excludedLegacyCheckIDs: DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
      observedAt,
    },
    summary: {
      ok: true,
      checks: [
        check,
        {
          ...check,
          checkID: "migration_compatibility",
          title: "Migration compatibility",
        },
        {
          ...check,
          checkID: "outbound_outcome_unknown_backlog",
          title: "Outbound outcome-unknown backlog",
        },
        {
          ...check,
          checkID: "release_sha_complete",
          title: "Release SHA completeness",
        },
      ],
      registryID: "v2-core-readiness.v1",
      registrySHA256: "a".repeat(64),
      excludedLegacyCheckIDs: DATA_HEALTH_EXCLUDED_LEGACY_CHECK_IDS,
      observedAt,
      overallStatus: "ok",
      counts: { ok: 4, warn: 0, fail: 0, notApplicable: 0 },
      gateCounts: { pass: 4, warn: 0, block: 0 },
    },
  },
};

const idle: DataHealthDetailState = { kind: "idle" };

function transport(): DataHealthTransport {
  return {
    list: vi.fn(async () => ({ status: 200, data: {} })),
    summary: vi.fn(async () => ({ status: 200, data: {} })),
    detail: vi.fn(async () => ({ status: 200, data: {} })),
  } as DataHealthTransport;
}

describe("data-health UI boundary", () => {
  it("labels distinct observations and exactly nineteen exclusions without claiming an atomic snapshot", () => {
    const html = renderToStaticMarkup(
      <DataHealthView
        overviewState={overviewState}
        detailState={idle}
        onLoad={vi.fn()}
        onDetail={vi.fn()}
      />,
    );
    expect(html).toContain("汇总观测时间");
    expect(html).toContain("注册表观测时间");
    expect(html).toContain("已排除 legacy 检查 19 项");
    expect(html).toContain("不代表任一请求组成原子快照");
  });

  it.each(["ops", "sales"] as const)(
    "denies %s without any request",
    (role) => {
      const client = transport();
      const html = renderToStaticMarkup(
        <DataHealthPage role={role} transport={client} />,
      );
      expect(html).toContain("当前账号没有数据健康访问权限。");
      expect(client.list).not.toHaveBeenCalled();
      expect(client.summary).not.toHaveBeenCalled();
      expect(client.detail).not.toHaveBeenCalled();
    },
  );
});
