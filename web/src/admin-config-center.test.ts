import { describe, expect, it, vi } from "vitest";
import {
  ADMIN_CONFIG_ACTIONS,
  DIRECT_KEY_CONFIRMATION_PHRASES,
  AdminConfigResponseEpoch,
  activateAPIClient,
  canManageAdminConfig,
  checkCategoryLocally,
  createAPIClient,
  createReleaseDraft,
  disableAPIClient,
  disableDirectAPIKey,
  generateDirectAPIKey,
  hasCompleteAdminConfigActionTokens,
  loadAPIClients,
  loadAdminConfigOverview,
  loadCategory,
  loadDirectAPIKey,
  loadRelease,
  loadReleases,
  loadShadowComparison,
  newAdminConfigRequestID,
  normalizeSafeJSONObject,
  publishReleaseLocally,
  redactAdminConfigError,
  rollbackReleaseLocally,
  rotateAPIClientSecret,
  rotateDirectAPIKey,
  saveCategorySettings,
  setCategoryEnabled,
  updateAPIClient,
  validateReleaseLocally,
  type AdminConfigSecurity,
  type AdminConfigTransport,
  type AdminConfigTransportResponse,
  type AdminCredentialState,
  type AdminReleaseState,
  type SafeJSONObject,
} from "./admin-config-center";

const CSRF_TOKEN = "c".repeat(43);
const ACTION_TOKEN = "a".repeat(43);
const REQUEST_ID = "admin-config:123e4567-e89b-12d3-a456-426614174000";
const CLIENT_REF_V1 = "secret://adminops/api_client/partner.crm/abcdef1234567890";
const CLIENT_REF_V2 = "secret://adminops/api_client/partner.crm/1234567890abcdef";
const DIRECT_REF_V1 = "secret://adminops/direct_api_key/direct-default/abcdef1234567890";
const DIRECT_REF_V2 = "secret://adminops/direct_api_key/direct-default/1234567890abcdef";
const CREATED_AT = "2026-08-21T08:00:00Z";
const UPDATED_AT = "2026-08-21T08:01:00Z";

function security(overrides: Partial<AdminConfigSecurity> = {}): AdminConfigSecurity {
  return {
    csrfToken: () => CSRF_TOKEN,
    actionTokenFor: () => ACTION_TOKEN,
    requestID: () => REQUEST_ID,
    ...overrides,
  };
}

function clientRecord(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 7,
    kind: "api_client",
    client_id: "partner.crm",
    display_name: "Partner CRM",
    state: "pending_activation",
    secret_ref: CLIENT_REF_V1,
    secret_mask: "masked:…567890",
    version: 1,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
    ...overrides,
  };
}

function directRecord(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 8,
    kind: "direct_api_key",
    client_id: "direct-default",
    display_name: "Legacy direct API key",
    state: "active",
    secret_ref: DIRECT_REF_V1,
    secret_mask: "masked:…567890",
    version: 1,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
    ...overrides,
  };
}

function encodeJSON(value: unknown): string {
  const text = JSON.stringify(value);
  const bytes = new TextEncoder().encode(text);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return globalThis.btoa(binary);
}

function categoryRecord(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    Key: "ai_runtime",
    Enabled: true,
    Settings: encodeJSON({ model: "local", api_secret_ref: "secret://vault/ai/runtime" }),
    Version: 2,
    UpdatedBy: "admin:7",
    UpdatedAt: UPDATED_AT,
    ...overrides,
  };
}

function releaseRecord(state: AdminReleaseState, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const validated = state === "validated" || state === "published" || state === "rolled_back";
  const published = state === "published" || state === "rolled_back";
  return {
    ID: state === "rolled_back" ? 2 : 1,
    State: state,
    Changes: encodeJSON({ "feature.example": "enabled" }),
    Checksum: "b".repeat(64),
    BasedOnReleaseID: state === "rolled_back" ? 1 : null,
    RollbackOfReleaseID: state === "rolled_back" ? 1 : null,
    CreatedBy: "admin:7",
    PublishedBy: published ? "admin:7" : "",
    CreatedAt: state === "rolled_back" ? "2026-08-21T09:00:00Z" : CREATED_AT,
    ValidatedAt: validated ? (state === "rolled_back" ? "2026-08-21T09:00:00Z" : "2026-08-21T08:05:00Z") : null,
    PublishedAt: published ? (state === "rolled_back" ? "2026-08-21T09:00:00Z" : "2026-08-21T08:10:00Z") : null,
    ...overrides,
  };
}

function routeTransport(routes: Readonly<Record<string, readonly AdminConfigTransportResponse[] | AdminConfigTransportResponse>>): AdminConfigTransport {
  const counters = new Map<string, number>();
  return {
    request: vi.fn(async (path: string, init: RequestInit) => {
      const key = `${init.method ?? "GET"} ${path}`;
      const configured = routes[key];
      if (!configured) return { status: 500, data: { ok: false } };
      const responses = Array.isArray(configured) ? configured : [configured];
      const index = counters.get(key) ?? 0;
      counters.set(key, index + 1);
      return responses[Math.min(index, responses.length - 1)] ?? { status: 500, data: { ok: false } };
    }),
  };
}

function sequenceTransport(...responses: readonly AdminConfigTransportResponse[]): AdminConfigTransport {
  let index = 0;
  return {
    request: vi.fn(async () => responses[index++] ?? { status: 500, data: { ok: false } }),
  };
}

function requestCalls(transport: AdminConfigTransport): readonly [string, RequestInit][] {
  return (transport.request as ReturnType<typeof vi.fn>).mock.calls as unknown as readonly [string, RequestInit][];
}

function bodyAt(transport: AdminConfigTransport, index: number): Record<string, unknown> {
  const body = requestCalls(transport)[index]?.[1].body;
  return typeof body === "string" ? JSON.parse(body) as Record<string, unknown> : {};
}

describe("admin config read projections", () => {
  it("keeps RBAC admin-only and creates bounded request IDs", () => {
    expect(canManageAdminConfig("admin")).toBe(true);
    expect(canManageAdminConfig("ops")).toBe(false);
    expect(canManageAdminConfig("sales")).toBe(false);
    expect(newAdminConfigRequestID({ randomUUID: () => "123e4567-e89b-12d3-a456-426614174000" })).toBe(REQUEST_ID);
    expect(newAdminConfigRequestID({ randomUUID: () => "not-a-uuid" })).toBeUndefined();
  });

  it("rejects expired read epochs after a newer request or explicit invalidation", () => {
    const gate = new AdminConfigResponseEpoch();
    const expired = gate.begin();
    const current = gate.begin();
    expect(gate.accepts(expired)).toBe(false);
    expect(gate.accepts(current)).toBe(true);
    gate.invalidate();
    expect(gate.accepts(current)).toBe(false);
  });

  it("keeps safe JSON equality independent of object key order and permits bounded decimals", async () => {
    const input = { zeta: 2.5, alpha: { second: true, first: "safe" } } as const;
    const normalized = normalizeSafeJSONObject(input);
    expect(normalized?.zeta).toBe(2.5);
    expect(normalized?.alpha).toMatchObject({ second: true, first: "safe" });
    const current = categoryRecord();
    const reordered = { alpha: { first: "safe", second: true }, zeta: 2.5 };
    const updated = categoryRecord({ Settings: encodeJSON(reordered), Version: 3, UpdatedAt: "2026-08-21T08:02:00Z" });
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, category: current } },
      { status: 200, data: { ok: true, changed: true, config: updated, real_external_call_executed: false } },
      { status: 200, data: { ok: true, category: updated } },
    );
    await expect(saveCategorySettings(transport, security(), { key: "ai_runtime", expectedVersion: 2, settings: input })).resolves.toMatchObject({ status: "applied", value: { version: 3 } });
  });

  it("accepts only the closed credential projection and fails closed on added secret-bearing fields", async () => {
    const valid = routeTransport({ "GET /api/admin/config/api-clients": { status: 200, data: { ok: true, clients: [clientRecord()] } } });
    await expect(loadAPIClients(valid)).resolves.toMatchObject({ status: "loaded", value: [{ clientID: "partner.crm", secretRef: CLIENT_REF_V1, secretMask: "masked:…567890" }] });

    for (const malformed of [
      { ...clientRecord(), client_secret: "[blocked-test-sentinel]" },
      { ...clientRecord(), secret_mask: CLIENT_REF_V1 },
      { ...clientRecord(), state: "enabled" },
      { ...clientRecord(), version: 0 },
      { ...clientRecord(), client_id: "../partner" },
    ]) {
      const transport = routeTransport({ "GET /api/admin/config/api-clients": { status: 200, data: { ok: true, clients: [malformed] } } });
      await expect(loadAPIClients(transport)).resolves.toEqual({ status: "invalid" });
    }
  });

  it("strips Direct API Key references from the public frontend model", async () => {
    const transport = routeTransport({ "GET /api/admin/config/api-key": { status: 200, data: { ok: true, configured: true, api_key: directRecord() } } });
    const result = await loadDirectAPIKey(transport);
    expect(result).toMatchObject({ status: "loaded", value: { configured: true, state: "active", secretMask: "masked:…567890", version: 1 } });
    expect(JSON.stringify(result)).not.toContain(DIRECT_REF_V1);
    expect(JSON.stringify(result)).not.toContain("secretRef");
  });

  it("parses category byte projections while rejecting raw secret settings and malformed fallback objects", async () => {
    const valid = routeTransport({ "GET /api/admin/config/categories/ai_runtime": { status: 200, data: { ok: true, category: categoryRecord() } } });
    await expect(loadCategory(valid, "ai_runtime")).resolves.toMatchObject({ status: "loaded", value: { key: "ai_runtime", version: 2, settings: { model: "local", api_secret_ref: "secret://vault/ai/runtime" } } });

    const leaking = routeTransport({ "GET /api/admin/config/categories/ai_runtime": { status: 200, data: { ok: true, category: categoryRecord({ Settings: encodeJSON({ api_secret: "[blocked-test-sentinel]" }) }) } } });
    await expect(loadCategory(leaking, "ai_runtime")).resolves.toEqual({ status: "invalid" });

    const fallback = routeTransport({ "GET /api/admin/config/categories/new_local": { status: 200, data: { ok: true, category: { key: "new_local", enabled: false, settings: {} } } } });
    await expect(loadCategory(fallback, "new_local")).resolves.toMatchObject({ status: "loaded", value: { key: "new_local", version: 0, persisted: false } });
  });

  it("keeps partial overview reads independent and never retries failed sections", async () => {
    const transport = routeTransport({
      "GET /api/admin/config/api-clients": { status: 200, data: { ok: true, clients: [clientRecord()] } },
      "GET /api/admin/config/api-key": { status: 503, data: { ok: false, error: "admin_ops_unavailable", unsafe_detail: "not surfaced" } },
      "GET /api/admin/config/categories": { status: 200, data: { ok: true, categories: [categoryRecord({ Settings: encodeJSON({ api_secret: "[blocked-test-sentinel]" }) })] } },
      "GET /api/admin/config/releases": { status: 200, data: { ok: true, releases: [releaseRecord("draft")] } },
    });
    const result = await loadAdminConfigOverview(transport);
    expect(result.clients.status).toBe("loaded");
    expect(result.directKey.status).toBe("unavailable");
    expect(result.categories.status).toBe("invalid");
    expect(result.releases.status).toBe("loaded");
    expect(transport.request).toHaveBeenCalledTimes(4);
    expect(redactAdminConfigError("unavailable")).not.toContain("unsafe_detail");
  });
});

describe("API Client safe lifecycle", () => {
  it("creates a pending client with CSRF, route action token and request ID, then verifies readback", async () => {
    const created = clientRecord();
    const transport = sequenceTransport(
      { status: 201, data: { ok: true, client: created, real_external_call_executed: false } },
      { status: 200, data: { ok: true, client: created } },
    );
    const metadata = { client_type: "service", token_ttl_minutes: 30, allowed_cidrs: ["10.0.0.0/8"] } as const;
    await expect(createAPIClient(transport, security(), { clientID: "partner.crm", displayName: "Partner CRM", metadata })).resolves.toMatchObject({ status: "applied", value: { state: "pending_activation", version: 1 } });
    expect(transport.request).toHaveBeenCalledTimes(2);
    const [path, init] = requestCalls(transport)[0] ?? [];
    expect(path).toBe("/api/admin/config/api-clients");
    expect(init.method).toBe("POST");
    expect(init.headers).toEqual({ Accept: "application/json", "Content-Type": "application/json", "X-CSRF-Token": CSRF_TOKEN, "X-Admin-Action-Token": ACTION_TOKEN, "X-Request-ID": REQUEST_ID });
    expect(bodyAt(transport, 0)).toEqual({ client_id: "partner.crm", display_name: "Partner CRM", metadata, confirm: true });
  });

  it("performs exact secret_ref activation self-check before the single write", async () => {
    const current = clientRecord();
    const active = clientRecord({ state: "active", version: 2, updated_at: UPDATED_AT });
    const mismatch = sequenceTransport({ status: 200, data: { ok: true, client: current } });
    await expect(activateAPIClient(mismatch, security(), { clientID: "partner.crm", expectedVersion: 1, secretRef: CLIENT_REF_V2, copiedConfirmed: true })).resolves.toEqual({ status: "invalid" });
    expect(mismatch.request).toHaveBeenCalledTimes(1);
    expect(requestCalls(mismatch)[0]?.[1].method).toBe("GET");

    const transport = sequenceTransport(
      { status: 200, data: { ok: true, client: current } },
      { status: 200, data: { ok: true, client: active, real_external_call_executed: false } },
      { status: 200, data: { ok: true, client: active } },
    );
    await expect(activateAPIClient(transport, security(), { clientID: "partner.crm", expectedVersion: 1, secretRef: CLIENT_REF_V1, copiedConfirmed: true })).resolves.toMatchObject({ status: "applied", value: { state: "active", version: 2 } });
    expect(bodyAt(transport, 1)).toEqual({ confirm: true, copied_confirmed: true, secret_ref: CLIENT_REF_V1 });
    expect(requestCalls(transport)[1]?.[0]).toBe("/api/admin/config/api-clients/partner.crm/activate");
  });

  it("edits a non-active client and verifies immutable credential fields on readback", async () => {
    const updated = clientRecord({ display_name: "Partner CRM v2", version: 2, updated_at: UPDATED_AT });
    const metadata = { client_type: "service", token_ttl_minutes: 45 };
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, client: clientRecord() } },
      { status: 200, data: { ok: true, client: updated, real_external_call_executed: false } },
      { status: 200, data: { ok: true, client: updated } },
    );
    await expect(updateAPIClient(transport, security(), { clientID: "partner.crm", expectedVersion: 1, displayName: "Partner CRM v2", metadata })).resolves.toMatchObject({ status: "applied", value: { displayName: "Partner CRM v2", version: 2 } });
    expect(bodyAt(transport, 1)).toEqual({ display_name: "Partner CRM v2", metadata, confirm: true });
  });

  it("rotates into pending activation and disables an active client with readback verification", async () => {
    const rotated = clientRecord({ secret_ref: CLIENT_REF_V2, secret_mask: "masked:…abcdef", version: 2, updated_at: UPDATED_AT });
    const rotateTransport = sequenceTransport(
      { status: 200, data: { ok: true, client: clientRecord() } },
      { status: 200, data: { ok: true, client: rotated, real_external_call_executed: false } },
      { status: 200, data: { ok: true, client: rotated } },
    );
    await expect(rotateAPIClientSecret(rotateTransport, security(), { clientID: "partner.crm", expectedVersion: 1 })).resolves.toMatchObject({ status: "applied", value: { state: "pending_activation", secretRef: CLIENT_REF_V2, version: 2 } });

    const active = clientRecord({ state: "active", version: 2, updated_at: UPDATED_AT });
    const disabled = clientRecord({ state: "disabled", version: 3, updated_at: "2026-08-21T08:02:00Z" });
    const disableTransport = sequenceTransport(
      { status: 200, data: { ok: true, client: active } },
      { status: 200, data: { ok: true, client: disabled, real_external_call_executed: false } },
      { status: 200, data: { ok: true, client: disabled } },
    );
    await expect(disableAPIClient(disableTransport, security(), { clientID: "partner.crm", expectedVersion: 2 })).resolves.toMatchObject({ status: "applied", value: { state: "disabled", version: 3 } });
    expect(bodyAt(disableTransport, 1)).toEqual({ confirm: true, enabled: false });
  });

  it("treats a concurrently deleted client as a state conflict before mutation", async () => {
    const transport = sequenceTransport({ status: 404, data: { ok: false, error: "admin_ops_not_found" } });
    await expect(rotateAPIClientSecret(transport, security(), { clientID: "partner.crm", expectedVersion: 1 })).resolves.toEqual({ status: "conflict" });
    expect(transport.request).toHaveBeenCalledTimes(1);
  });

  it("rejects stale versions before update and never sends the write", async () => {
    const transport = sequenceTransport({ status: 200, data: { ok: true, client: clientRecord({ version: 2, updated_at: UPDATED_AT }) } });
    await expect(updateAPIClient(transport, security(), { clientID: "partner.crm", expectedVersion: 1, displayName: "Partner CRM 2", metadata: {} })).resolves.toEqual({ status: "conflict" });
    expect(transport.request).toHaveBeenCalledTimes(1);
  });

  it("maps 409 to conflict and does not retry", async () => {
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, client: clientRecord() } },
      { status: 409, data: { ok: false, error: "admin_ops_conflict" } },
    );
    await expect(rotateAPIClientSecret(transport, security(), { clientID: "partner.crm", expectedVersion: 1 })).resolves.toEqual({ status: "conflict" });
    expect(transport.request).toHaveBeenCalledTimes(2);
  });

  it("locks as unknown when any mandatory post-write readback fails, including session expiry", async () => {
    const rotated = clientRecord({ state: "pending_activation", secret_ref: CLIENT_REF_V2, secret_mask: "masked:…abcdef", version: 2, updated_at: UPDATED_AT });
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, client: clientRecord() } },
      { status: 200, data: { ok: true, client: rotated, real_external_call_executed: false } },
      { status: 401, data: { ok: false, error: "unauthorized" } },
    );
    await expect(rotateAPIClientSecret(transport, security(), { clientID: "partner.crm", expectedVersion: 1 })).resolves.toEqual({ status: "unknown" });
    expect(transport.request).toHaveBeenCalledTimes(3);
  });

  it("treats 503 or a malformed success receipt as unknown without automatic retry", async () => {
    for (const response of [
      { status: 503, data: { ok: false, error: "admin_ops_unavailable" } },
      { status: 200, data: { ok: true, client: { ...clientRecord({ secret_ref: CLIENT_REF_V2 }), raw_secret: "[blocked-test-sentinel]" }, real_external_call_executed: false } },
    ]) {
      const transport = sequenceTransport(
        { status: 200, data: { ok: true, client: clientRecord() } },
        response,
      );
      await expect(rotateAPIClientSecret(transport, security(), { clientID: "partner.crm", expectedVersion: 1 })).resolves.toEqual({ status: "unknown" });
      expect(transport.request).toHaveBeenCalledTimes(2);
    }
  });
});

describe("Direct API Key double confirmation", () => {
  it("does not issue any request until both confirmation controls match", async () => {
    const transport = sequenceTransport();
    await expect(generateDirectAPIKey(transport, security(), { confirmed: true, confirmationText: "incorrect" })).resolves.toEqual({ status: "invalid" });
    await expect(generateDirectAPIKey(transport, security(), { confirmed: false, confirmationText: DIRECT_KEY_CONFIRMATION_PHRASES.generate })).resolves.toEqual({ status: "invalid" });
    expect(transport.request).not.toHaveBeenCalled();
  });

  it("generates with one write and returns a reference-free snapshot after readback", async () => {
    const created = directRecord();
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, configured: false, secret_values_exposed: false } },
      { status: 201, data: { ok: true, api_key: created, real_external_call_executed: false } },
      { status: 200, data: { ok: true, configured: true, api_key: created } },
    );
    const result = await generateDirectAPIKey(transport, security(), { confirmed: true, confirmationText: DIRECT_KEY_CONFIRMATION_PHRASES.generate });
    expect(result).toMatchObject({ status: "applied", value: { configured: true, secretMask: "masked:…567890", version: 1 } });
    expect(JSON.stringify(result)).not.toContain(DIRECT_REF_V1);
    expect(bodyAt(transport, 1)).toEqual({ confirm: true });
    expect(transport.request).toHaveBeenCalledTimes(3);
  });

  it("rotates and disables Direct API Key without exposing its reference", async () => {
    const expected = { configured: true, state: "active" as AdminCredentialState, secretMask: "masked:…567890", version: 1, createdAt: CREATED_AT, updatedAt: CREATED_AT };
    const rotatedRecord = directRecord({ state: "pending_activation", secret_ref: DIRECT_REF_V2, secret_mask: "masked:…abcdef", version: 2, updated_at: UPDATED_AT });
    const rotateTransport = sequenceTransport(
      { status: 200, data: { ok: true, configured: true, api_key: directRecord() } },
      { status: 200, data: { ok: true, api_key: rotatedRecord, real_external_call_executed: false } },
      { status: 200, data: { ok: true, configured: true, api_key: rotatedRecord } },
    );
    const rotated = await rotateDirectAPIKey(rotateTransport, security(), expected, { confirmed: true, confirmationText: DIRECT_KEY_CONFIRMATION_PHRASES.rotate });
    expect(rotated).toMatchObject({ status: "applied", value: { state: "pending_activation", secretMask: "masked:…abcdef", version: 2 } });
    expect(JSON.stringify(rotated)).not.toContain(DIRECT_REF_V2);

    const rotatedSnapshot = { configured: true, state: "pending_activation" as AdminCredentialState, secretMask: "masked:…abcdef", version: 2, createdAt: CREATED_AT, updatedAt: UPDATED_AT };
    const disabledRecord = directRecord({ state: "disabled", secret_ref: DIRECT_REF_V2, secret_mask: "masked:…abcdef", version: 3, updated_at: "2026-08-21T08:02:00Z" });
    const disableTransport = sequenceTransport(
      { status: 200, data: { ok: true, configured: true, api_key: rotatedRecord } },
      { status: 200, data: { ok: true, api_key: disabledRecord, real_external_call_executed: false } },
      { status: 200, data: { ok: true, configured: true, api_key: disabledRecord } },
    );
    const disabled = await disableDirectAPIKey(disableTransport, security(), rotatedSnapshot, { confirmed: true, confirmationText: DIRECT_KEY_CONFIRMATION_PHRASES.disable });
    expect(disabled).toMatchObject({ status: "applied", value: { state: "disabled", secretMask: "masked:…abcdef", version: 3 } });
    expect(JSON.stringify(disabled)).not.toContain(DIRECT_REF_V2);
  });

  it("requires a current snapshot before disable and fails closed on concurrent version change", async () => {
    const expected = { configured: true, state: "active" as AdminCredentialState, secretMask: "masked:…567890", version: 1, createdAt: CREATED_AT, updatedAt: CREATED_AT };
    const transport = sequenceTransport({ status: 200, data: { ok: true, configured: true, api_key: directRecord({ version: 2, updated_at: UPDATED_AT }) } });
    await expect(disableDirectAPIKey(transport, security(), expected, { confirmed: true, confirmationText: DIRECT_KEY_CONFIRMATION_PHRASES.disable })).resolves.toEqual({ status: "conflict" });
    expect(transport.request).toHaveBeenCalledTimes(1);
  });
});

describe("category local writes and checks", () => {
  it("rejects raw sensitive settings before any transport call", async () => {
    const transport = sequenceTransport();
    const unsafe = { api_secret: "[blocked-test-sentinel]" } as unknown as SafeJSONObject;
    expect(normalizeSafeJSONObject(unsafe)).toBeUndefined();
    await expect(saveCategorySettings(transport, security(), { key: "ai_runtime", expectedVersion: 2, settings: unsafe })).resolves.toEqual({ status: "invalid" });
    expect(transport.request).not.toHaveBeenCalled();
  });

  it("changes only the local category enabled flag and verifies version increment", async () => {
    const current = categoryRecord();
    const disabled = categoryRecord({ Enabled: false, Version: 3, UpdatedAt: "2026-08-21T08:02:00Z" });
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, category: current } },
      { status: 200, data: { ok: true, changed: true, config: disabled, real_external_call_executed: false } },
      { status: 200, data: { ok: true, category: disabled } },
    );
    await expect(setCategoryEnabled(transport, security(), { key: "ai_runtime", expectedVersion: 2, enabled: false })).resolves.toMatchObject({ status: "applied", value: { enabled: false, version: 3 } });
    expect(bodyAt(transport, 1)).toEqual({ enabled: false });
  });

  it("saves safe references and verifies the incremented category version", async () => {
    const current = categoryRecord();
    const changedSettings = { model: "local-v2", api_secret_ref: "secret://vault/ai/runtime-v2" };
    const updated = categoryRecord({ Settings: encodeJSON(changedSettings), Version: 3, UpdatedAt: "2026-08-21T08:02:00Z" });
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, category: current } },
      { status: 200, data: { ok: true, changed: true, config: updated, real_external_call_executed: false } },
      { status: 200, data: { ok: true, category: updated } },
    );
    await expect(saveCategorySettings(transport, security(), { key: "ai_runtime", expectedVersion: 2, settings: changedSettings })).resolves.toMatchObject({ status: "applied", value: { version: 3, settings: changedSettings } });
    expect(bodyAt(transport, 1)).toEqual({ settings: changedSettings });
    expect(requestCalls(transport)[1]?.[0]).toBe("/api/admin/config/categories/ai_runtime/settings");
  });

  it("preserves the existing first-write category default without inventing a new setting semantic", async () => {
    const fallback = { key: "new_local", enabled: false, settings: {} };
    const settings = { mode: "local" };
    const created = categoryRecord({ Key: "new_local", Enabled: true, Settings: encodeJSON(settings), Version: 1, UpdatedAt: "2026-08-21T08:02:00Z" });
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, category: fallback } },
      { status: 200, data: { ok: true, changed: true, config: created, real_external_call_executed: false } },
      { status: 200, data: { ok: true, category: created } },
    );
    await expect(saveCategorySettings(transport, security(), { key: "new_local", expectedVersion: 0, settings })).resolves.toMatchObject({ status: "applied", value: { enabled: true, persisted: true, version: 1 } });
  });

  it("accepts only a no-external-call local check receipt", async () => {
    const transport = sequenceTransport({ status: 200, data: { ok: true, summary: { category: "ai_runtime", failed: 0, external_calls: false }, config: categoryRecord(), real_external_call_executed: false } });
    await expect(checkCategoryLocally(transport, security(), "ai_runtime")).resolves.toMatchObject({ status: "applied", value: { failed: 0, externalCalls: false } });
    expect(bodyAt(transport, 0)).toEqual({});

    const malformed = sequenceTransport({ status: 200, data: { ok: true, summary: { category: "ai_runtime", failed: 0, external_calls: true }, config: categoryRecord(), real_external_call_executed: false } });
    await expect(checkCategoryLocally(malformed, security(), "ai_runtime")).resolves.toEqual({ status: "unavailable" });
  });
});

describe("local Release lifecycle boundary", () => {
  it("creates only a local draft and rejects deployment-bearing or malformed receipts", async () => {
    const draft = releaseRecord("draft");
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, release: draft, real_external_call_executed: false } },
      { status: 200, data: { ok: true, release: draft } },
    );
    await expect(createReleaseDraft(transport, security(), { "feature.example": "enabled" })).resolves.toMatchObject({ status: "applied", value: { state: "draft" } });
    expect(bodyAt(transport, 0)).toEqual({ changes: { "feature.example": "enabled" }, confirm: true });
    expect(JSON.stringify(bodyAt(transport, 0))).not.toContain("deploy");

    const malformed = sequenceTransport({ status: 200, data: { ok: true, release: { ...draft, DeploymentID: "outside-contract" }, real_external_call_executed: false } });
    await expect(createReleaseDraft(malformed, security(), { "feature.example": "enabled" })).resolves.toEqual({ status: "unknown" });
  });

  it("validates a draft with one state transition and readback", async () => {
    const draft = releaseRecord("draft");
    const validated = releaseRecord("validated");
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, release: draft } },
      { status: 200, data: { ok: true, release: validated, real_external_call_executed: false } },
      { status: 200, data: { ok: true, release: validated } },
    );
    await expect(validateReleaseLocally(transport, security(), { releaseID: 1, expectedChecksum: "b".repeat(64) })).resolves.toMatchObject({ status: "applied", value: { state: "validated" } });
    expect(requestCalls(transport)[1]?.[0]).toBe("/api/admin/config/releases/1/validate");
    expect(bodyAt(transport, 1)).toEqual({});
  });

  it("reads shadow compare as local-only and fails closed when external_calls is not false", async () => {
    const transport = routeTransport({ "GET /api/admin/config/releases/1/shadow-compare": { status: 200, data: { ok: true, comparison: { release_id: 1, external_calls: false } } } });
    await expect(loadShadowComparison(transport, 1)).resolves.toEqual({ status: "loaded", value: { releaseID: 1, available: true, externalCalls: false } });
    const unsafe = routeTransport({ "GET /api/admin/config/releases/1/shadow-compare": { status: 200, data: { ok: true, comparison: { release_id: 1, external_calls: true } } } });
    await expect(loadShadowComparison(unsafe, 1)).resolves.toEqual({ status: "invalid" });
  });

  it("publishes only a validated local release with checksum binding", async () => {
    const validated = releaseRecord("validated");
    const published = releaseRecord("published");
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, release: validated } },
      { status: 200, data: { ok: true, release: published, real_external_call_executed: false } },
      { status: 200, data: { ok: true, release: published } },
    );
    await expect(publishReleaseLocally(transport, security(), { releaseID: 1, expectedChecksum: "b".repeat(64) })).resolves.toMatchObject({ status: "applied", value: { state: "published" } });
    expect(bodyAt(transport, 1)).toEqual({ confirm: true, checksum: "b".repeat(64) });
    expect(requestCalls(transport)[1]?.[0]).toBe("/api/admin/config/releases/1/publish");
  });

  it("creates a new rolled_back local record while verifying the original remains published", async () => {
    const published = releaseRecord("published");
    const rolledBack = releaseRecord("rolled_back");
    const transport = sequenceTransport(
      { status: 200, data: { ok: true, release: published } },
      { status: 200, data: { ok: true, release: rolledBack, real_external_call_executed: false } },
      { status: 200, data: { ok: true, release: rolledBack } },
      { status: 200, data: { ok: true, release: published } },
    );
    await expect(rollbackReleaseLocally(transport, security(), { releaseID: 1, expectedChecksum: "b".repeat(64) })).resolves.toMatchObject({ status: "applied", value: { id: 2, state: "rolled_back", rollbackOfReleaseID: 1 } });
    expect(requestCalls(transport)[1]?.[0]).toBe("/api/admin/config/releases/1/rollback");
    expect(bodyAt(transport, 1)).toEqual({ confirm: true });
    expect(transport.request).toHaveBeenCalledTimes(4);
  });

  it("rejects stale state before local publish without issuing a write", async () => {
    const transport = sequenceTransport({ status: 200, data: { ok: true, release: releaseRecord("draft") } });
    await expect(publishReleaseLocally(transport, security(), { releaseID: 1, expectedChecksum: "b".repeat(64) })).resolves.toEqual({ status: "conflict" });
    expect(transport.request).toHaveBeenCalledTimes(1);
  });

  it("orders release history by parsed time rather than RFC3339 string shape", async () => {
    const later = releaseRecord("draft", { ID: 2, CreatedAt: "2026-08-21T08:00:00.5Z" });
    const earlier = releaseRecord("draft", { ID: 1, CreatedAt: "2026-08-21T08:00:00Z" });
    const transport = routeTransport({ "GET /api/admin/config/releases": { status: 200, data: { ok: true, releases: [later, earlier] } } });
    await expect(loadReleases(transport)).resolves.toMatchObject({ status: "loaded", value: [{ id: 2 }, { id: 1 }] });
  });

  it("parses a standalone release detail without any deployment proof", async () => {
    const transport = routeTransport({ "GET /api/admin/config/releases/1": { status: 200, data: { ok: true, release: releaseRecord("published") } } });
    const result = await loadRelease(transport, 1);
    expect(result).toMatchObject({ status: "loaded", value: { state: "published" } });
    expect(JSON.stringify(result)).not.toContain("deployment");
    expect(JSON.stringify(result)).not.toContain("provider");
  });
});

describe("write security failures", () => {
  it("opens writes only when every exact route tuple has a valid token", () => {
    expect(hasCompleteAdminConfigActionTokens(() => ACTION_TOKEN)).toBe(true);
    expect(hasCompleteAdminConfigActionTokens(undefined)).toBe(false);
    expect(hasCompleteAdminConfigActionTokens((method, pattern) => method === "POST" && pattern.endsWith("/publish") ? undefined : ACTION_TOKEN)).toBe(false);
    expect(hasCompleteAdminConfigActionTokens(() => { throw new Error("expired token map"); })).toBe(false);
  });

  it("fails before mutation when CSRF, action token, or request ID is missing", async () => {
    for (const missing of [
      security({ csrfToken: () => undefined }),
      security({ actionTokenFor: () => undefined }),
      security({ requestID: () => undefined }),
      security({ csrfToken: () => { throw new Error("cookie unavailable"); } }),
      security({ actionTokenFor: () => { throw new Error("token map unavailable"); } }),
      security({ requestID: () => { throw new Error("random source unavailable"); } }),
    ]) {
      const transport = sequenceTransport(
        { status: 200, data: { ok: true, configured: false, secret_values_exposed: false } },
      );
      await expect(generateDirectAPIKey(transport, missing, { confirmed: true, confirmationText: DIRECT_KEY_CONFIRMATION_PHRASES.generate })).resolves.toEqual({ status: "forbidden" });
      expect(transport.request).toHaveBeenCalledTimes(1);
    }
  });

  it("uses the exact route-bound action tuple for client activation", () => {
    expect(ADMIN_CONFIG_ACTIONS.activateClient).toEqual({ method: "POST", pattern: "/api/admin/config/api-clients/{client_id}/activate" });
  });
});
