import { describe, expect, it, vi } from "vitest";
import {
  filterAutomationAgents,
  loadAutomationAgents,
  parseAutomationAgents,
  type AutomationAgentsTransport,
} from "./automation-agents";

const updatedAt = "2026-08-20T08:00:00Z";
const item = {
  id: 7,
  automation_type: "agent",
  agent_code: "agent_7",
  agent_name: "欢迎话术",
  bound_package_key: "",
  bound_package_id: null,
  bound_package_name: "",
  fixed_material_summary: {
    image_count: 1,
    miniprogram_count: 0,
    attachment_count: 2,
    group_invite_count: 0,
  },
  status: "active",
  updated_at: updatedAt,
};

function transport(
  overrides: Partial<AutomationAgentsTransport> = {},
): AutomationAgentsTransport {
  return {
    read: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  } as AutomationAgentsTransport;
}

describe("automation-agent local summary contract", () => {
  it("retains only the fixed local summary and validates discarded compatibility fields", () => {
    const parsed = parseAutomationAgents({ ok: true, items: [item], total: 1 });
    expect(parsed).toEqual({
      total: 1,
      items: [
        {
          id: 7,
          type: "agent",
          typeLabel: "Agent 机器人",
          code: "agent_7",
          name: "欢迎话术",
          status: "active",
          updatedAt,
          materialSummary: {
            imageCount: 1,
            miniprogramCount: 0,
            attachmentCount: 2,
            groupInviteCount: 0,
          },
        },
      ],
    });
    expect(JSON.stringify(parsed)).not.toContain("bound_package");
  });

  it.each([
    { ok: true, items: [{ ...item, draft_role_prompt: "secret" }], total: 1 },
    { ok: true, items: [{ ...item, bound_package_name: "binding" }], total: 1 },
    { ok: true, items: [{ ...item, automation_type_label: "固定话术" }], total: 1 },
    { ok: true, items: [{ ...item, bound_package_id: 1 }], total: 1 },
    { ok: true, items: [{ ...item, updated_at: "2026-02-30T08:00:00Z" }], total: 1 },
    { ok: true, items: [item, item], total: 2 },
    { ok: true, items: [item], total: 2 },
  ])("fails closed for expanded or contradictory summary %#", (value) => {
    expect(parseAutomationAgents(value)).toBeUndefined();
  });

  it("filters the verified snapshot locally by keyword, type, and status", () => {
    const snapshot = parseAutomationAgents({
      ok: true,
      items: [
        item,
        {
          ...item,
          id: 6,
          automation_type: "fixed_script",
          agent_code: "fixed_6",
          agent_name: "跟进脚本",
          status: "paused",
          updated_at: "2026-08-19T08:00:00Z",
        },
      ],
      total: 2,
    });
    expect(snapshot).toBeDefined();
    expect(filterAutomationAgents(snapshot!, "脚本", "fixed_script", "paused")).toHaveLength(1);
    expect(filterAutomationAgents(snapshot!, "agent_7", "all", "all")).toHaveLength(1);
  });

  it("uses a fixed same-origin GET without query or retry", async () => {
    const client = transport({
      read: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: { ok: true, items: [item], total: 1 } })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadAutomationAgents(client)).resolves.toMatchObject({ status: "loaded" });
    await expect(loadAutomationAgents(client)).resolves.toEqual({ status: "unauthenticated" });
    await expect(loadAutomationAgents(client)).resolves.toEqual({ status: "unavailable" });
    expect(client.read).toHaveBeenCalledTimes(3);
    expect(client.read).toHaveBeenNthCalledWith(1, { credentials: "same-origin" });
  });
});
