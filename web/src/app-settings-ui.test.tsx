import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { AppSettingsPage, AppSettingsView } from "./app-settings-ui";
import type { AppSettingsTransport } from "./app-settings";

const snapshot = {
  actionToken: "a".repeat(43),
  editable: [
    { key: "wecom.corp_id", label: "wecom.corp_id", inputType: "text", value: "corp-1", configured: true, source: "app_settings", updatedAt: "2026-08-20T08:00:00Z", lastModifiedAt: "2026-08-20T08:00:00Z", lastModifiedBy: "admin:7", lastActionType: "create" },
    { key: "wecom.agent_id", label: "wecom.agent_id", inputType: "number", value: "1", configured: true, source: "app_settings", updatedAt: "2026-08-20T08:00:00Z", lastModifiedAt: "2026-08-20T08:00:00Z", lastModifiedBy: "admin:7", lastActionType: "create" },
    { key: "outbound.rate_per_second", label: "outbound.rate_per_second", inputType: "number", value: "5", configured: true, source: "app_settings", updatedAt: "2026-08-20T08:00:00Z", lastModifiedAt: "2026-08-20T08:00:00Z", lastModifiedBy: "admin:7", lastActionType: "create" },
    { key: "outbound.max_attempts", label: "outbound.max_attempts", inputType: "number", value: "3", configured: true, source: "app_settings", updatedAt: "2026-08-20T08:00:00Z", lastModifiedAt: "2026-08-20T08:00:00Z", lastModifiedBy: "admin:7", lastActionType: "create" },
  ],
  masked: [
    { key: "database.url", configured: true }, { key: "wecom.secret", configured: false }, { key: "wecom.callback_token", configured: false }, { key: "wecom.callback_aes_key", configured: false }, { key: "ai.api_key", configured: true }, { key: "auth.jwt_secret", configured: true }, { key: "extension.api_key_pepper", configured: false }, { key: "gateway.webhook_master_key", configured: false },
  ],
  summary: [
    { label: "可直接编辑", value: 4, description: "可以直接修改的设置项" }, { label: "敏感信息", value: 8, description: "只显示掩码的设置项" }, { label: "已配置", value: 7, description: "当前已经配置完成的设置项" },
  ],
  audits: [{ id: 1, operator: "admin:7", actionType: "create", targetID: "wecom.corp_id", createdAt: "2026-08-20T08:00:00Z" }],
} as const;

function transport(): AppSettingsTransport {
  return {
    read: vi.fn(async () => ({ status: 503, data: {} })),
    save: vi.fn(async () => ({ status: 503, data: {} })),
  };
}

describe("AppSettingsView", () => {
  it("renders the local-only edit workflow and only masked secret states", () => {
    const html = renderToStaticMarkup(<AppSettingsView state={{ kind: "ready", snapshot }} draft={{ "wecom.corp_id": "corp-1", "wecom.agent_id": "1", "outbound.rate_per_second": "5", "outbound.max_attempts": "3" }} confirmed={false} saveState={{ kind: "idle" }} onDraftChange={vi.fn()} onConfirmationChange={vi.fn()} onSave={vi.fn()} />);
    expect(html).toContain('<h1 id="app-title">本地应用设置</h1>');
    expect(html).toContain("只管理四项已持久化的非敏感本地设置");
    expect(html).toContain("wecom.corp_id");
    expect(html).toMatch(/<code>database\.url<\/code>：已配置（已掩码）/);
    expect(html).toMatch(/<code>ai\.api_key<\/code>：已配置（已掩码）/);
    expect(html).not.toContain("password");
    expect(html).not.toContain('type="password"');
  });

  it("escapes local setting and audit text without creating links or external controls", () => {
    const html = renderToStaticMarkup(<AppSettingsView state={{ kind: "ready", snapshot: { ...snapshot, editable: [{ ...snapshot.editable[0], value: '<img src=x onerror="bad">' }, ...snapshot.editable.slice(1)], audits: [{ ...snapshot.audits[0], operator: '<img src=x onerror="bad">' }] } }} draft={{ "wecom.corp_id": "corp-1", "wecom.agent_id": "1", "outbound.rate_per_second": "5", "outbound.max_attempts": "3" }} confirmed={false} saveState={{ kind: "idle" }} onDraftChange={vi.fn()} onConfirmationChange={vi.fn()} onSave={vi.fn()} />);
    expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("href=");
  });

  it("keeps ops and sales fail-closed without issuing read or write requests", () => {
    for (const role of ["ops", "sales"] as const) {
      const client = transport();
      const html = renderToStaticMarkup(<AppSettingsPage role={role} transport={client} />);
      expect(html).toContain("当前账号没有本地应用设置管理权限。");
      expect(client.read).not.toHaveBeenCalled();
      expect(client.save).not.toHaveBeenCalled();
    }
  });

  it("preserves an already verified snapshot for local read failure and disables unknown writes", () => {
    const error = renderToStaticMarkup(<AppSettingsView state={{ kind: "error", failure: "unavailable", previous: snapshot }} draft={{ "wecom.corp_id": "corp-1", "wecom.agent_id": "1", "outbound.rate_per_second": "5", "outbound.max_attempts": "3" }} confirmed={false} saveState={{ kind: "unknown", message: "未知" }} onDraftChange={vi.fn()} onConfirmationChange={vi.fn()} onSave={vi.fn()} />);
    expect(error).toContain("corp-1");
    expect(error).toContain("本地应用设置暂不可用");
    expect(error).toContain("未知");
    expect(error).toMatch(/<fieldset disabled="">/);
  });
});
