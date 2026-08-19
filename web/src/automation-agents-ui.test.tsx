import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import {
  AutomationAgentsPage,
  AutomationAgentsView,
  invalidateAutomationAgentsRead,
  startAutomationAgentsRead,
  type AutomationAgentsState,
} from "./automation-agents-ui";
import type { AutomationAgentsSnapshot, AutomationAgentsTransport } from "./automation-agents";

const snapshot: AutomationAgentsSnapshot = {
  total: 1,
  items: [{ id: 7, type: "agent", typeLabel: "Agent 机器人", code: "agent_7", name: "欢迎话术", status: "active", updatedAt: "2026-08-20T08:00:00Z", materialSummary: { imageCount: 1, miniprogramCount: 0, attachmentCount: 2, groupInviteCount: 0 } }],
};

const response = {
  ok: true,
  items: [{ id: 7, automation_type: "agent", agent_code: "agent_7", agent_name: "欢迎话术", bound_package_key: "", bound_package_id: null, bound_package_name: "", fixed_material_summary: { image_count: 1, miniprogram_count: 0, attachment_count: 2, group_invite_count: 0 }, status: "active", updated_at: "2026-08-20T08:00:00Z" }],
  total: 1,
};

function transport(): AutomationAgentsTransport {
  return { read: vi.fn(async () => ({ status: 200, data: response })) } as AutomationAgentsTransport;
}

describe("automation-agent browser UI", () => {
  it("renders only the frozen local summary fields", () => {
    const state: AutomationAgentsState = { kind: "ready", snapshot };
    const html = renderToStaticMarkup(<AutomationAgentsView state={state} keyword="" type="all" status="all" onKeywordChange={vi.fn()} onTypeChange={vi.fn()} onStatusChange={vi.fn()} onLoad={vi.fn()} />);
    expect(html).toContain("欢迎话术");
    expect(html).toContain("图片 1");
    for (const blocked of ["role_prompt_secret", "raw-content", "image_library_ids", "bound_package_name"]) expect(html).not.toContain(blocked);
  });

  it.each(["ops", "sales"] as const)("denies %s without a request", (role) => {
    const client = transport();
    const html = renderToStaticMarkup(<AutomationAgentsPage role={role} transport={client} />);
    expect(html).toContain("当前账号没有自动化话术目录访问权限。");
    expect(client.read).not.toHaveBeenCalled();
  });

  it("single-flights, drops stale reads, preserves verified data on failure, and calls 401 once", async () => {
    let resolveOld: (() => void) | undefined;
    const old = new Promise<{ status: number; data: unknown }>((resolve) => { resolveOld = () => resolve({ status: 401, data: {} }); });
    const read = vi.fn().mockReturnValueOnce(old).mockResolvedValueOnce({ status: 200, data: response }).mockResolvedValueOnce({ status: 503, data: {} });
    const states: AutomationAgentsState[] = [];
    const onUnauthenticated = vi.fn();
    const controller = { generation: { current: 0 }, inFlight: { current: undefined as symbol | undefined }, verified: { current: undefined as AutomationAgentsSnapshot | undefined }, onState: (state: AutomationAgentsState) => states.push(state), onUnauthenticated };
    const oldRead = startAutomationAgentsRead(controller, { read } as AutomationAgentsTransport);
    await startAutomationAgentsRead(controller, { read } as AutomationAgentsTransport);
    expect(read).toHaveBeenCalledTimes(1);
    invalidateAutomationAgentsRead(controller);
    await startAutomationAgentsRead(controller, { read } as AutomationAgentsTransport);
    expect(states.at(-1)).toEqual({ kind: "ready", snapshot });
    resolveOld?.();
    await oldRead;
    expect(onUnauthenticated).not.toHaveBeenCalled();
    await startAutomationAgentsRead(controller, { read } as AutomationAgentsTransport);
    expect(states.at(-1)).toEqual({ kind: "error", failure: "unavailable", previous: snapshot });
    expect(controller.inFlight.current).toBeUndefined();
  });

  it("maps one current 401 to the existing session callback", async () => {
    const onUnauthenticated = vi.fn();
    const controller = { generation: { current: 0 }, inFlight: { current: undefined as symbol | undefined }, verified: { current: undefined as AutomationAgentsSnapshot | undefined }, onState: vi.fn(), onUnauthenticated };
    await startAutomationAgentsRead(controller, { read: vi.fn(async () => ({ status: 401, data: {} })) } as AutomationAgentsTransport);
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
  });
});
