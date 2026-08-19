import { describe, expect, it, vi } from "vitest";
import {
  archiveAutomationAgent,
  automationAgentFixedContentRequest,
  createAutomationAgent,
  filterAutomationAgents,
  loadAutomationAgents,
  parseAutomationAgentDetail,
  parseAutomationAgents,
  saveAutomationAgentFixedContent,
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

const detailAgent = {
  ...item,
  fixed_material_summary: { image_count: 1, miniprogram_count: 0, attachment_count: 0, group_invite_count: 0 },
  automation_type_label: "Agent 机器人",
  draft_role_prompt: "角色",
  draft_task_prompt: "任务",
  published_role_prompt: "角色",
  published_task_prompt: "任务",
  draft_version: 1,
  published_version: 1,
  has_unpublished_changes: false,
  fixed_content_package: {
    content_text: "",
    image_library_ids: [9],
    miniprogram_library_ids: [],
    attachment_library_ids: [],
    group_invite_library_ids: [],
  },
  fixed_content_package_preview: {
    content_text: "",
    material_summary: { image_count: 1, miniprogram_count: 0, attachment_count: 0, group_invite_count: 0 },
    materials: [],
  },
};

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

  it("strictly decodes the complete local detail while retaining only image bindings", () => {
    expect(parseAutomationAgentDetail({ ok: true, agent: detailAgent }, 7)).toMatchObject({
      id: 7,
      draftRolePrompt: "角色",
      fixedContent: { imageLibraryIDs: [9], hasUnsupportedBindings: false },
    });
    expect(parseAutomationAgentDetail({ ok: true, agent: { ...detailAgent, fixed_content_package_preview: { ...detailAgent.fixed_content_package_preview, materials: [{ url: "https://outside.invalid" }] } } }, 7)).toBeUndefined();
    expect(parseAutomationAgentDetail({ ok: true, agent: { ...detailAgent, bound_package_key: "retired" } }, 7)).toBeUndefined();
    const withUnsupportedBinding = {
      ...detailAgent,
      fixed_material_summary: { image_count: 1, miniprogram_count: 1, attachment_count: 0, group_invite_count: 0 },
      fixed_content_package: { ...detailAgent.fixed_content_package, miniprogram_library_ids: [1] },
      fixed_content_package_preview: { ...detailAgent.fixed_content_package_preview, material_summary: { image_count: 1, miniprogram_count: 1, attachment_count: 0, group_invite_count: 0 } },
    };
    expect(parseAutomationAgentDetail({ ok: true, agent: withUnsupportedBinding }, 7)?.fixedContent.hasUnsupportedBindings).toBe(true);
  });

  it("uses generated command seams with CSRF and unique idempotency, and treats malformed success as unknown", async () => {
    const create = vi.fn(async () => ({ status: 200, data: { ok: true, agent: { ...detailAgent, id: 8, agent_code: "agent_8", agent_name: "新话术", draft_role_prompt: "", draft_task_prompt: "", published_role_prompt: "", published_task_prompt: "", fixed_content_package: { ...detailAgent.fixed_content_package, image_library_ids: [] }, fixed_content_package_preview: { ...detailAgent.fixed_content_package_preview, material_summary: { image_count: 0, miniprogram_count: 0, attachment_count: 0, group_invite_count: 0 } }, fixed_material_summary: { image_count: 0, miniprogram_count: 0, attachment_count: 0, group_invite_count: 0 } } } }));
    const client = transport({ create });
    const csrf = "a".repeat(43); const key = "automation-agent:11111111-1111-4111-8111-111111111111";
    await expect(createAutomationAgent(client, { name: "新话术", code: "agent_8", type: "agent", rolePrompt: "", taskPrompt: "" }, csrf, key)).resolves.toMatchObject({ status: "succeeded", agent: { id: 8 } });
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ agent_code: "agent_8", fixed_content_package: expect.objectContaining({ image_library_ids: [] }) }), { credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf, "Idempotency-Key": key } });
    await expect(createAutomationAgent(client, { name: "新话术", code: "agent_8", type: "agent", rolePrompt: "", taskPrompt: "" }, csrf, "bad")).resolves.toEqual({ status: "invalid" });
  });

  it("keeps fixed bindings local and verifies archive's narrow closed receipt", async () => {
    expect(automationAgentFixedContentRequest("agent", "not allowed", [])).toBeUndefined();
    expect(automationAgentFixedContentRequest("fixed_script", "本地固定内容", [1, 1])).toBeUndefined();
    const csrf = "b".repeat(43); const key = "automation-agent:11111111-1111-4111-8111-111111111111";
    const client = transport({
      saveFixedContent: vi.fn(async () => ({ status: 503, data: {} })),
      archive: vi.fn(async () => ({ status: 200, data: { ok: true, agent: { id: 7, status: "archived" } } })),
    });
    await expect(saveAutomationAgentFixedContent(client, 7, "fixed_script", "本地固定内容", [9], csrf, key)).resolves.toEqual({ status: "unknown" });
    await expect(archiveAutomationAgent(client, 7, csrf, key)).resolves.toEqual({ status: "archived", id: 7 });
    await expect(archiveAutomationAgent(transport({ archive: vi.fn(async () => ({ status: 200, data: { ok: true, agent: { id: 7, status: "active" } } })) }), 7, csrf, key)).resolves.toEqual({ status: "unknown" });
  });
});
