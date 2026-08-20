import { describe, expect, it, vi } from "vitest";
import {
  bindConsoleIdentity,
  canonicalIdentityConsoleRef,
  parseIdentityBindResult,
  parseIdentityResolveResult,
  resolveConsoleIdentity,
  type IdentityConsoleTransport,
} from "./identity-console";

const phone = { type: "phone" as const, scope: " phone:e164 ", value: " +86 138-0013-8000 " };

function transport(resolve: { status: number; data: unknown }, bind = resolve): IdentityConsoleTransport {
  return { resolve: vi.fn(async () => resolve), bind: vi.fn(async () => bind) } as unknown as IdentityConsoleTransport;
}

describe("identity local console contract", () => {
  it("canonicalizes only normalizer-compatible browser input", () => {
    expect(canonicalIdentityConsoleRef(phone)).toEqual({ type: "phone", scope: "phone:e164", value: "+8613800138000" });
    expect(canonicalIdentityConsoleRef({ ...phone, scope: "phone:e164 invalid" })).toBeUndefined();
    expect(canonicalIdentityConsoleRef({ ...phone, value: "+86\u0000138" })).toBeUndefined();
    expect(canonicalIdentityConsoleRef({ type: "unionid", scope: "wechat-open-platform:app", value: "🙂" })).toEqual({ type: "unionid", scope: "wechat-open-platform:app", value: "🙂" });
  });

  it("decodes exact result branches without returning identity values", () => {
    expect(parseIdentityResolveResult({ status: "found", customer_id: 7 })).toEqual({ status: "found", customerID: 7 });
    expect(parseIdentityResolveResult({ status: "found", customer_id: 7, value: "+86138" })).toBeUndefined();
    expect(parseIdentityResolveResult({ status: "not_found" })).toEqual({ status: "not_found" });
    expect(parseIdentityBindResult({ status: "merged", customer_id: 7, primary_customer_id: 3, merge_audit_id: 2 })).toEqual({ status: "merged", customerID: 7, primaryCustomerID: 3, mergeAuditID: 2 });
    expect(parseIdentityBindResult({ status: "manual_review", review_id: 9, identity_value: "raw" })).toBeUndefined();
  });

  it("uses same-origin reads and a CSRF/idempotent same-origin local bind", async () => {
    const client = transport({ status: 200, data: { status: "found", customer_id: 7 } }, { status: 200, data: { status: "bound", customer_id: 7 } });
    await expect(resolveConsoleIdentity(client, phone)).resolves.toEqual({ status: "resolved", result: { status: "found", customerID: 7 } });
    await expect(bindConsoleIdentity(client, 7, phone, "c".repeat(43), "identity-bind-key-0001")).resolves.toEqual({ status: "bound", result: { status: "bound", customerID: 7 } });
    expect(client.resolve).toHaveBeenCalledWith({ ref: { type: "phone", scope: "phone:e164", value: "+8613800138000" } }, { credentials: "same-origin" });
    expect(client.bind).toHaveBeenCalledWith({ customer_id: 7, ref: { type: "phone", scope: "phone:e164", value: "+8613800138000" } }, { credentials: "same-origin", headers: { "X-CSRF-Token": "c".repeat(43), "Idempotency-Key": "identity-bind-key-0001" } });
  });

  it("fails closed on malformed successful responses and classifies deterministic failures", async () => {
    await expect(resolveConsoleIdentity(transport({ status: 200, data: { status: "found", customer_id: 1, extra: true } }), phone)).resolves.toEqual({ status: "unavailable" });
    await expect(bindConsoleIdentity(transport({ status: 503, data: {} }, { status: 409, data: {} }), 1, phone, "c".repeat(43), "identity-bind-key-0002")).resolves.toEqual({ status: "conflict" });
    await expect(bindConsoleIdentity(transport({ status: 503, data: {} }), 1, phone, "bad", "identity-bind-key-0002")).resolves.toEqual({ status: "invalid" });
  });
});
