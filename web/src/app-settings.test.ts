import { describe, expect, it, vi } from "vitest";
import {
  appSettingsChanged,
  appSettingsDraft,
  canManageAppSettings,
  loadAppSettings,
  newAppSettingsRequestID,
  parseAppSettingsSnapshot,
  saveAppSettings,
  type AppSettingsTransport,
} from "./app-settings";

const TOKEN = "a".repeat(43);
const editable = [
  { key: "wecom.corp_id", label: "wecom.corp_id", mode: "editable", input_type: "text", description: "", value: "corp-1", display_value: "corp-1", configured: true, source: "app_settings", version: "", updated_at: "2026-08-20T08:00:00Z", last_modified_at: "2026-08-20T08:00:00Z", last_modified_by: "admin:7", last_action_type: "create" },
  { key: "wecom.agent_id", label: "wecom.agent_id", mode: "editable", input_type: "number", description: "", value: "1", display_value: "1", configured: true, source: "app_settings", version: "", updated_at: "2026-08-20T08:00:00Z", last_modified_at: "2026-08-20T08:00:00Z", last_modified_by: "admin:7", last_action_type: "create" },
  { key: "outbound.rate_per_second", label: "outbound.rate_per_second", mode: "editable", input_type: "number", description: "", value: "5", display_value: "5", configured: true, source: "app_settings", version: "", updated_at: "2026-08-20T08:00:00Z", last_modified_at: "2026-08-20T08:00:00Z", last_modified_by: "admin:7", last_action_type: "create" },
  { key: "outbound.max_attempts", label: "outbound.max_attempts", mode: "editable", input_type: "number", description: "", value: "3", display_value: "3", configured: true, source: "app_settings", version: "", updated_at: "2026-08-20T08:00:00Z", last_modified_at: "2026-08-20T08:00:00Z", last_modified_by: "admin:7", last_action_type: "create" },
] as const;
const maskedKeys = ["database.url", "wecom.secret", "wecom.callback_token", "wecom.callback_aes_key", "ai.api_key", "auth.jwt_secret", "extension.api_key_pepper", "gateway.webhook_master_key"] as const;
const masked = maskedKeys.map((key, index) => ({ key, label: key, mode: "masked", input_type: "password", description: "", configured: index === 0, masked: true }));
const metadata = Object.fromEntries([...editable, ...masked].map((row) => [row.key, { key: row.key, label: row.label, mode: row.mode, input_type: row.input_type, description: "" }]));
const config = {
  rows: [...editable, ...masked], metadata_map: metadata,
  summary_cards: [
    { label: "可直接编辑", value: 4, description: "可以直接修改的设置项" },
    { label: "敏感信息", value: 8, description: "只显示掩码的设置项" },
    { label: "已配置", value: 5, description: "当前已经配置完成的设置项" },
  ],
  audit_entries: [
    { id: 2, operator: "admin:7", action_type: "update", target_id: "outbound.rate_per_second", created_at: "2026-08-20T08:01:00Z" },
    { id: 1, operator: "admin:7", action_type: "create", target_id: "wecom.corp_id", created_at: "2026-08-20T08:00:00Z" },
  ],
} as const;
const response = { ok: true, config, source_status: "next_read_model", fallback_used: false, admin_action_token: TOKEN } as const;

function transport(overrides: Partial<AppSettingsTransport> = {}): AppSettingsTransport {
  return {
    read: vi.fn(async () => ({ status: 503, data: {} })),
    save: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  };
}

describe("app-settings local read contract", () => {
  it("accepts only the complete fixed twelve-key local projection", () => {
    const snapshot = parseAppSettingsSnapshot(response);
    expect(snapshot?.actionToken).toBe(TOKEN);
    expect(snapshot?.editable[0]).toMatchObject({ key: "wecom.corp_id", value: "corp-1" });
    expect(snapshot?.masked[0]).toEqual({ key: "database.url", configured: true });
    expect(appSettingsDraft(snapshot!)).toEqual({ "wecom.corp_id": "corp-1", "wecom.agent_id": "1", "outbound.rate_per_second": "5", "outbound.max_attempts": "3" });
  });

  it("fails closed for omitted, reordered, contradictory, secret-leaking, or malformed projections", () => {
    const mutated = (changes: Record<string, unknown>) => ({ ...response, ...changes });
    const reorderedRows = [...config.rows]; [reorderedRows[0], reorderedRows[1]] = [reorderedRows[1]!, reorderedRows[0]!];
    for (const value of [
      mutated({ unknown: true }),
      { ...response, config: { ...config, rows: reorderedRows } },
      { ...response, config: { ...config, rows: [{ ...editable[0], input_type: "number" }, ...config.rows.slice(1)] } },
      { ...response, config: { ...config, rows: [{ ...editable[0], value: "", display_value: "" }, ...config.rows.slice(1)] } },
      { ...response, config: { ...config, rows: [...editable.slice(0, 2), { ...editable[2], value: "51", display_value: "51" }, editable[3], ...masked] } },
      { ...response, config: { ...config, rows: [...editable, { ...masked[0], value: "leak" }, ...masked.slice(1)] } },
      { ...response, config: { ...config, metadata_map: { ...metadata, "wecom.corp_id": { ...metadata["wecom.corp_id"], mode: "masked" } } } },
      { ...response, config: { ...config, summary_cards: [{ ...config.summary_cards[0], value: 3 }, ...config.summary_cards.slice(1)] } },
      { ...response, config: { ...config, audit_entries: [...config.audit_entries].reverse() } },
      { ...response, admin_action_token: "short" },
    ]) expect(parseAppSettingsSnapshot(value)).toBeUndefined();
  });

  it("uses one fixed same-origin GET without query or retry and keeps roles fail-closed", async () => {
    const client = transport({ read: vi.fn(async () => ({ status: 200, data: response })) });
    await expect(loadAppSettings(client)).resolves.toMatchObject({ status: "loaded" });
    expect(client.read).toHaveBeenCalledWith(undefined, { credentials: "same-origin" });
    expect(canManageAppSettings("admin")).toBe(true);
    expect(canManageAppSettings("ops")).toBe(false);
    expect(canManageAppSettings("sales")).toBe(false);
  });
});

describe("app-settings local save contract", () => {
  it("validates source-aligned editable input and returns only changes", () => {
    const snapshot = parseAppSettingsSnapshot(response)!;
    expect(appSettingsChanged(snapshot, { ...appSettingsDraft(snapshot), "outbound.rate_per_second": "6" })).toEqual({ "outbound.rate_per_second": "6" });
    for (const draft of [
      { ...appSettingsDraft(snapshot), "wecom.corp_id": " " },
      { ...appSettingsDraft(snapshot), "wecom.agent_id": "0" },
      { ...appSettingsDraft(snapshot), "outbound.rate_per_second": "51" },
      { ...appSettingsDraft(snapshot), "outbound.max_attempts": "1.0" },
    ]) expect(appSettingsChanged(snapshot, draft)).toBeUndefined();
  });

  it("allows untouched empty defaults while changing only one newly configured local setting", () => {
    const blankRows = [
      { ...editable[0], value: "", display_value: "", configured: false, source: "config", updated_at: "", last_modified_at: "", last_modified_by: "", last_action_type: "empty" },
      ...editable.slice(1),
      ...masked,
    ];
    const blank = parseAppSettingsSnapshot({ ...response, config: { ...config, rows: blankRows, summary_cards: [{ ...config.summary_cards[0] }, { ...config.summary_cards[1] }, { ...config.summary_cards[2], value: 4 }] } });
    expect(blank).toBeDefined();
    expect(appSettingsChanged(blank!, { ...appSettingsDraft(blank!), "wecom.corp_id": "corp-2" })).toEqual({ "wecom.corp_id": "corp-2" });
  });

  it("sends CSRF, route action token and one valid request identifier, then accepts only a closed confirmed receipt", async () => {
    const snapshot = parseAppSettingsSnapshot(response)!;
    const updatedRow = { ...editable[2], value: "6", display_value: "6", updated_at: "2026-08-20T08:02:00Z", last_modified_at: "2026-08-20T08:02:00Z", last_action_type: "update" };
    const receipt = { ok: true, changed: [{ key: "outbound.rate_per_second", value: "6" }], changed_count: 1, config: { ...config, rows: [editable[0], editable[1], updatedRow, editable[3], ...masked], summary_cards: config.summary_cards.map((item) => item.label === "已配置" ? { ...item, value: 5 } : item) }, source_status: "next_command", fallback_used: false, real_external_call_executed: false };
    const client = transport({ save: vi.fn(async () => ({ status: 200, data: receipt })) });
    await expect(saveAppSettings(client, snapshot, { ...appSettingsDraft(snapshot), "outbound.rate_per_second": "6" }, TOKEN, "app-settings:123e4567-e89b-12d3-a456-426614174000")).resolves.toMatchObject({ status: "saved" });
    expect(client.save).toHaveBeenCalledWith({ settings: { "outbound.rate_per_second": "6" }, confirm: true }, { credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": TOKEN, "X-Admin-Action-Token": TOKEN, "X-Request-ID": "app-settings:123e4567-e89b-12d3-a456-426614174000" } });
  });

  it("does not retry determinate failures and locks only unknown write outcomes", async () => {
    const snapshot = parseAppSettingsSnapshot(response)!;
    const draft = { ...appSettingsDraft(snapshot), "outbound.rate_per_second": "6" };
    for (const [status, outcome] of [[400, "invalid"], [401, "unauthenticated"], [403, "forbidden"], [409, "conflict"], [503, "unknown"]] as const) {
      const client = transport({ save: vi.fn(async () => ({ status, data: {} })) });
      await expect(saveAppSettings(client, snapshot, draft, TOKEN, "app-settings:123e4567-e89b-12d3-a456-426614174000")).resolves.toEqual({ status: outcome });
      expect(client.save).toHaveBeenCalledTimes(1);
    }
  });

  it("creates only bounded gateway request identifiers", () => {
    expect(newAppSettingsRequestID({ randomUUID: () => "123e4567-e89b-12d3-a456-426614174000" })).toBe("app-settings:123e4567-e89b-12d3-a456-426614174000");
    expect(newAppSettingsRequestID({ randomUUID: () => "not-a-uuid" })).toBeUndefined();
  });
});
