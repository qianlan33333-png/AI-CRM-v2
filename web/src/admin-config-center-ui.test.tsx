import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  AdminConfigCenterPage,
  AdminConfigCenterView,
  DirectAPIKeyPanel,
  ReleasePanel,
  type AdminConfigCenterViewProps,
} from "./admin-config-center-ui";
import type { AdminCategory, AdminConfigTransport, AdminCredential, AdminRelease, DirectAPIKeySnapshot } from "./admin-config-center";

const client: AdminCredential = {
  id: 7,
  kind: "api_client",
  clientID: "partner.crm",
  displayName: "Partner CRM",
  state: "pending_activation",
  secretRef: "secret://adminops/api_client/partner.crm/abcdef1234567890",
  secretMask: "masked:…567890",
  version: 1,
  createdAt: "2026-08-21T08:00:00Z",
  updatedAt: "2026-08-21T08:00:00Z",
};

const directKey: DirectAPIKeySnapshot = {
  configured: true,
  state: "active",
  secretMask: "masked:…567890",
  version: 3,
  createdAt: "2026-08-21T08:00:00Z",
  updatedAt: "2026-08-21T08:10:00Z",
};

const category: AdminCategory = {
  key: "ai_runtime",
  enabled: true,
  settings: { model: "local", api_secret_ref: "secret://vault/ai/runtime" },
  version: 2,
  updatedBy: "admin:7",
  updatedAt: "2026-08-21T08:01:00Z",
  persisted: true,
};

const release: AdminRelease = {
  id: 1,
  state: "validated",
  changes: { "feature.example": "enabled" },
  checksum: "b".repeat(64),
  createdBy: "admin:7",
  createdAt: "2026-08-21T08:00:00Z",
  validatedAt: "2026-08-21T08:05:00Z",
};

function callbacks(): Pick<AdminConfigCenterViewProps,
  "onSectionChange" | "onRefresh" | "onSelectClient" | "onCreateClient" | "onUpdateClient" | "onRotateClient" | "onActivateClient" | "onDisableClient" | "onDirectKeyAction" | "onSelectCategory" | "onSetCategoryEnabled" | "onSaveCategorySettings" | "onCheckCategory" | "onCreateRelease" | "onSelectRelease" | "onValidateRelease" | "onLoadShadowComparison" | "onPublishRelease" | "onRollbackRelease"> {
  return {
    onSectionChange: vi.fn(),
    onRefresh: vi.fn(),
    onSelectClient: vi.fn(),
    onCreateClient: vi.fn(),
    onUpdateClient: vi.fn(),
    onRotateClient: vi.fn(),
    onActivateClient: vi.fn(),
    onDisableClient: vi.fn(),
    onDirectKeyAction: vi.fn(),
    onSelectCategory: vi.fn(),
    onSetCategoryEnabled: vi.fn(),
    onSaveCategorySettings: vi.fn(),
    onCheckCategory: vi.fn(),
    onCreateRelease: vi.fn(),
    onSelectRelease: vi.fn(),
    onValidateRelease: vi.fn(),
    onLoadShadowComparison: vi.fn(),
    onPublishRelease: vi.fn(),
    onRollbackRelease: vi.fn(),
  };
}

function props(overrides: Partial<AdminConfigCenterViewProps> = {}): AdminConfigCenterViewProps {
  return {
    activeSection: "overview",
    clients: { kind: "ready", value: [client] },
    directKey: { kind: "ready", value: directKey },
    categories: { kind: "ready", value: [category] },
    releases: { kind: "ready", value: [release] },
    writeState: { kind: "idle" },
    writesAvailable: true,
    ...callbacks(),
    ...overrides,
  };
}

function transport(): AdminConfigTransport {
  return { request: vi.fn(async () => ({ status: 503, data: {} })) };
}

describe("AdminConfigCenterView", () => {
  it("renders five clear subareas and the local Release boundary", () => {
    const html = renderToStaticMarkup(<AdminConfigCenterView {...props()} />);
    expect(html).toContain("Admin 配置控制中心");
    for (const label of ["概览", "API Clients", "Direct API Key", "配置分类", "Release 历史 / 详情"]) expect(html).toContain(label);
    expect(html).toContain("本地配置状态，不等于部署");
    expect(html).toContain("不执行部署、Provider 调用或任何外部效果");
  });

  it("keeps successful overview cards visible when other reads fail", () => {
    const activeClient = { ...client, state: "active" as const };
    const html = renderToStaticMarkup(<AdminConfigCenterView {...props({
      clients: { kind: "ready", value: [activeClient] },
      directKey: { kind: "error", failure: "unavailable" },
      categories: { kind: "error", failure: "invalid", previous: [category] },
    })} />);
    expect(html).toContain("1 个已激活");
    expect(html).toContain("Direct API Key：本地配置读取暂不可用");
    expect(html).toContain("配置分类：本地配置请求或响应不符合安全合同");
    expect(html).toContain("1 个已启用");
  });

  it("locks every write control after an unknown outcome until manual readback", () => {
    const html = renderToStaticMarkup(<AdminConfigCenterView {...props({ activeSection: "clients", selectedClient: { kind: "ready", value: client }, writeState: { kind: "unknown" } })} />);
    expect(html).toContain("写入结果未知，已锁定后续写操作");
    expect(html).toMatch(/<fieldset disabled="">/);
    expect(html).toContain("重新读取本地状态");
    expect(html).not.toContain("自动重试中");
  });

  it("escapes malicious display fields and never creates executable markup", () => {
    const malicious = { ...client, displayName: '<img src=x onerror="bad">' };
    const html = renderToStaticMarkup(<AdminConfigCenterView {...props({ activeSection: "clients", clients: { kind: "ready", value: [malicious] }, selectedClient: { kind: "ready", value: malicious } })} />);
    expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("onerror=\"bad\"");
  });

  it("shows the activation reference self-check and disables editing for active clients", () => {
    const active = { ...client, state: "active" as const, version: 2 };
    const html = renderToStaticMarkup(<AdminConfigCenterView {...props({ activeSection: "clients", clients: { kind: "ready", value: [active] }, selectedClient: { kind: "ready", value: active } })} />);
    expect(html).toContain("激活自检");
    expect(html).toContain("回填 secret_ref");
    expect(html).toContain("active 客户端必须先停用");
    expect(html).toMatch(/<fieldset disabled="">/);
  });
});

describe("Direct API Key safe UI", () => {
  it("renders only mask, version and state without any reference field", () => {
    const html = renderToStaticMarkup(<DirectAPIKeyPanel state={{ kind: "ready", value: directKey }} writesDisabled={false} onAction={vi.fn()} />);
    expect(html).toContain("masked:…567890");
    expect(html).toContain("v3");
    expect(html).toContain("已激活");
    expect(html).not.toContain("secret://");
    expect(html).not.toContain("secretRef");
    expect(html).not.toContain("secret_ref");
  });

  it("requires both a checkbox and exact phrase in the rendered workflow", () => {
    const html = renderToStaticMarkup(<DirectAPIKeyPanel state={{ kind: "ready", value: directKey }} writesDisabled={false} onAction={vi.fn()} />);
    expect(html).toContain("我理解该操作会改变本地凭据状态");
    expect(html).toContain("确认轮换本地 API Key");
    expect(html).toMatch(/<button type="submit" disabled="">提交一次并回读核验<\/button>/);
  });
});

describe("ReleasePanel local-only presentation", () => {
  it("labels publish and rollback as local state transitions, not deployment", () => {
    const html = renderToStaticMarkup(<ReleasePanel
      state={{ kind: "ready", value: [release] }}
      selected={{ kind: "ready", value: release }}
      comparison={{ kind: "ready", value: { releaseID: 1, available: true, externalCalls: false } }}
      writesDisabled={false}
      onCreate={vi.fn()}
      onSelect={vi.fn()}
      onValidate={vi.fn()}
      onCompare={vi.fn()}
      onPublish={vi.fn()}
      onRollback={vi.fn()}
    />);
    expect(html).toContain("本地配置状态，不等于部署");
    expect(html).toContain("标记为本地已发布");
    expect(html).toContain("创建本地回滚记录");
    expect(html).toContain("external_calls=false");
    expect(html).not.toContain("部署成功");
    expect(html).not.toContain("Provider 已执行");
  });
});

describe("AdminConfigCenterPage RBAC", () => {
  it("keeps ops and sales fail-closed without issuing reads or writes", () => {
    for (const role of ["ops", "sales"] as const) {
      const clientTransport = transport();
      const html = renderToStaticMarkup(<AdminConfigCenterPage role={role} transport={clientTransport} actionTokenFor={() => "a".repeat(43)} />);
      expect(html).toContain("当前账号没有 Admin 配置管理权限");
      expect(clientTransport.request).not.toHaveBeenCalled();
    }
  });

  it("keeps write controls closed when the central shell has not injected route tokens", () => {
    const html = renderToStaticMarkup(<AdminConfigCenterView {...props({ writesAvailable: false, activeSection: "direct-key" })} />);
    expect(html).toContain("中央壳层尚未提供当前会话的路由绑定 Action Token");
    expect(html).toMatch(/<fieldset disabled="">/);
  });

  it("keeps writes closed when a resolver exists but its token set is incomplete", () => {
    const html = renderToStaticMarkup(<AdminConfigCenterPage role="admin" transport={transport()} actionTokenFor={() => undefined} />);
    expect(html).toContain("中央壳层尚未提供当前会话的路由绑定 Action Token");
    expect(html).toMatch(/<fieldset disabled="">|配置控制中心概览/);
  });
});
