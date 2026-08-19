import { describe, expect, it, vi } from "vitest";
import {
  PUSH_CENTER_SECTION_KEYS,
  loadPushCenterSections,
  parsePushCenterSections,
  type PushCenterTransport,
} from "./push-center";

const statusKeys = [
  "pending",
  "running",
  "succeeded",
  "sent",
  "simulated",
  "unknown_after_dispatch",
  "failed",
  "sent_with_shadow_warning",
  "shadow_failed_not_business_failed",
];
const response = {
  ok: true,
  sections: PUSH_CENTER_SECTION_KEYS.map((key, index) => ({
    key,
    label: `分区 ${index}`,
    effect_types: [],
    capability_key: "",
    count: index,
  })),
  status_definitions: statusKeys.map((key) => ({
    key,
    label: key,
    definition: "本地状态定义",
  })),
  filters: {},
  route_owner: "ai_crm_next",
};

const degradedResponse = {
  ok: true,
  degraded: true,
  error: "",
  error_code: "production_read_unavailable",
  source_status: "production_unavailable",
  read_model_status: "unavailable",
  capability_owner: "ai_crm_next/platform_foundation/push_center",
  page_error: "推送中心读模型暂不可用，请稍后重试。",
  diagnostics: {
    production_data_ready: false,
    fixture_mode: false,
    allow_fixture_repo_in_prod: false,
    error_class: "ReadModelUnavailableError",
  },
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  status_code: 200,
  items: [],
  total: 0,
  counts: {
    total: 0,
    by_effective_status: {},
    by_status: {},
    by_section: {},
    pending: 0,
    running: 0,
    sent: 0,
    failed: 0,
  },
  status_definitions: response.status_definitions,
  filters: {},
  limit: 50,
  offset: 0,
  sections: [],
};

describe("push center local section contract", () => {
  it("projects only section key, label, and count", () => {
    expect(parsePushCenterSections(response)).toEqual({
      sections: PUSH_CENTER_SECTION_KEYS.map((key, index) => ({
        key,
        label: `分区 ${index}`,
        count: index,
      })),
    });
    const serialized = JSON.stringify(parsePushCenterSections(response));
    for (const hidden of [
      "effect_types",
      "capability_key",
      "status_definitions",
      "filters",
    ])
      expect(serialized).not.toContain(hidden);
  });

  it.each([
    { ...response, extra: true },
    { ...response, filters: { external_userid: "wx-secret" } },
    { ...response, sections: response.sections.slice(1) },
    {
      ...response,
      sections: response.sections.map((item, index) =>
        index === 0 ? { ...item, key: "order" } : item,
      ),
    },
    { ...response, status_definitions: response.status_definitions.slice(1) },
    {
      ...response,
      status_definitions: response.status_definitions.map((item, index) =>
        index === 0 ? { ...item, key: "sent" } : item,
      ),
    },
    {
      ...response,
      sections: response.sections.map((item, index) =>
        index === 0 ? { ...item, count: -1 } : item,
      ),
    },
    { degraded: true, ...response },
  ])("fails closed for malformed or degraded data %#", (value) => {
    expect(parsePushCenterSections(value)).toBeUndefined();
  });

  it("uses one fixed no-query same-origin GET and never retries", async () => {
    const read = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: response })
      .mockResolvedValueOnce({ status: 401, data: {} })
      .mockResolvedValueOnce({ status: 200, data: degradedResponse });
    const transport = { read } as PushCenterTransport;
    await expect(loadPushCenterSections(transport)).resolves.toMatchObject({
      status: "loaded",
    });
    await expect(loadPushCenterSections(transport)).resolves.toEqual({
      status: "unauthenticated",
    });
    await expect(loadPushCenterSections(transport)).resolves.toEqual({
      status: "unavailable",
    });
    expect(read).toHaveBeenCalledTimes(3);
    expect(read).toHaveBeenNthCalledWith(1, { credentials: "same-origin" });
  });

  it.each([
    { degraded: true },
    { ...degradedResponse, filters: { external_userid: "wx-secret" } },
    {
      ...degradedResponse,
      status_definitions: [
        degradedResponse.status_definitions[0],
        degradedResponse.status_definitions[0],
        ...degradedResponse.status_definitions.slice(2),
      ],
    },
    { ...degradedResponse, extra: true },
  ])("rejects malformed degraded 200 data %#", async (data) => {
    await expect(
      loadPushCenterSections({
        read: vi.fn(async () => ({ status: 200, data })),
      } as PushCenterTransport),
    ).resolves.toEqual({ status: "invalid" });
  });
});
