import {
  bindIdentity,
  resolveIdentity,
  type BindIdentityRequest,
  type ResolveIdentityRequest,
} from "./api/generated/health";

export type IdentityConsoleRole = "admin" | "ops" | "sales";
export type IdentityKind = "wecom_external_userid" | "unionid" | "mp_openid" | "oa_openid" | "alipay_user_id" | "phone" | "ext";
export type IdentityConsoleRef = { readonly type: IdentityKind; readonly scope: string; readonly value: string };
export type IdentityResolveResult = { readonly status: "found"; readonly customerID: number } | { readonly status: "not_found" | "conflict" };
export type IdentityBindResult =
  | { readonly status: "bound" | "already_bound"; readonly customerID: number }
  | { readonly status: "merged"; readonly customerID: number; readonly primaryCustomerID: number; readonly mergeAuditID: number }
  | { readonly status: "manual_review"; readonly reviewID: number }
  | { readonly status: "rejected" };
export type IdentityConsoleFailure = "unauthenticated" | "forbidden" | "invalid" | "conflict" | "unavailable";
export type IdentityConsoleTransportResponse = { readonly status: number; readonly data: unknown };

async function generatedResolve(request: ResolveIdentityRequest, options: RequestInit): Promise<IdentityConsoleTransportResponse> {
  const response = await resolveIdentity(request, options);
  return { status: response.status, data: response.data };
}
async function generatedBind(request: BindIdentityRequest, options: RequestInit): Promise<IdentityConsoleTransportResponse> {
  const response = await bindIdentity(request, options);
  return { status: response.status, data: response.data };
}
export interface IdentityConsoleTransport { readonly resolve: typeof generatedResolve; readonly bind: typeof generatedBind; }
export const generatedIdentityConsoleTransport: IdentityConsoleTransport = { resolve: generatedResolve, bind: generatedBind };

const sameOrigin: RequestInit = { credentials: "same-origin" };
const keyPattern = /^[A-Za-z0-9_-]{16,128}$/;
const kinds = new Set<IdentityKind>(["wecom_external_userid", "unionid", "mp_openid", "oa_openid", "alipay_user_id", "phone", "ext"]);
function record(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value); }
function exact(value: unknown, keys: readonly string[]): value is Record<string, unknown> { return record(value) && Object.keys(value).length === keys.length && Object.keys(value).every((key) => keys.includes(key)); }
function positive(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function noControl(value: string): boolean { return !/[\u0000-\u001f\u007f]/.test(value); }
function byteLength(value: string): number { return new TextEncoder().encode(value).byteLength; }

export function canonicalIdentityConsoleRef(value: IdentityConsoleRef): IdentityConsoleRef | undefined {
  const scope = value.scope.trim();
  const rawValue = value.value.trim();
  if (!kinds.has(value.type) || byteLength(scope) > 256 || byteLength(rawValue) === 0 || byteLength(rawValue) > 1024 || /\s/.test(scope) || !noControl(scope) || !noControl(rawValue)) return undefined;
  if (value.type === "phone") {
    const compact = rawValue.replace(/[\s().-]/g, "");
    return scope === "phone:e164" && /^\+[1-9][0-9]{1,14}$/.test(compact) ? { ...value, scope, value: compact } : undefined;
  }
  const prefix = value.type === "wecom_external_userid" ? "wecom-corp:"
    : value.type === "unionid" ? "wechat-open-platform:"
      : value.type === "mp_openid" || value.type === "oa_openid" ? "wechat-app:"
        : value.type === "alipay_user_id" ? "alipay-app:" : "ext:";
  return scope.startsWith(prefix) && byteLength(scope) > byteLength(prefix) ? { ...value, scope, value: rawValue } : undefined;
}

export function validIdentityConsoleRef(value: IdentityConsoleRef): boolean {
  return canonicalIdentityConsoleRef(value) !== undefined;
}

export function parseIdentityResolveResult(value: unknown): IdentityResolveResult | undefined {
  if (exact(value, ["status", "customer_id"]) && value.status === "found" && positive(value.customer_id)) return { status: "found", customerID: value.customer_id };
  if (exact(value, ["status"]) && (value.status === "not_found" || value.status === "conflict")) return { status: value.status };
  return undefined;
}
export function parseIdentityBindResult(value: unknown): IdentityBindResult | undefined {
  if (exact(value, ["status", "customer_id"]) && (value.status === "bound" || value.status === "already_bound") && positive(value.customer_id)) return { status: value.status, customerID: value.customer_id };
  if (exact(value, ["status", "customer_id", "primary_customer_id", "merge_audit_id"]) && value.status === "merged" && positive(value.customer_id) && positive(value.primary_customer_id) && positive(value.merge_audit_id)) return { status: "merged", customerID: value.customer_id, primaryCustomerID: value.primary_customer_id, mergeAuditID: value.merge_audit_id };
  if (exact(value, ["status", "review_id"]) && value.status === "manual_review" && positive(value.review_id)) return { status: "manual_review", reviewID: value.review_id };
  if (exact(value, ["status"]) && value.status === "rejected") return { status: "rejected" };
  return undefined;
}
function failure(status: number): IdentityConsoleFailure { if (status === 401) return "unauthenticated"; if (status === 403) return "forbidden"; if (status === 400 || status === 422) return "invalid"; if (status === 409) return "conflict"; return "unavailable"; }

export async function resolveConsoleIdentity(transport: IdentityConsoleTransport, ref: IdentityConsoleRef): Promise<{ readonly status: "resolved"; readonly result: IdentityResolveResult } | { readonly status: IdentityConsoleFailure }> {
  const canonical = canonicalIdentityConsoleRef(ref);
  if (!canonical) return { status: "invalid" };
  try {
    const response = await transport.resolve({ ref: canonical }, sameOrigin);
    if (response.status !== 200) return { status: failure(response.status) };
    const result = parseIdentityResolveResult(response.data);
    return result ? { status: "resolved", result } : { status: "unavailable" };
  } catch { return { status: "unavailable" }; }
}
export async function bindConsoleIdentity(transport: IdentityConsoleTransport, customerID: number, ref: IdentityConsoleRef, csrf: string, idempotencyKey: string): Promise<{ readonly status: "bound"; readonly result: IdentityBindResult } | { readonly status: IdentityConsoleFailure }> {
  const canonical = canonicalIdentityConsoleRef(ref);
  if (!positive(customerID) || !canonical || !/^[A-Za-z0-9_-]{43}$/.test(csrf) || !keyPattern.test(idempotencyKey)) return { status: "invalid" };
  try {
    const response = await transport.bind({ customer_id: customerID, ref: canonical }, { credentials: "same-origin", headers: { "X-CSRF-Token": csrf, "Idempotency-Key": idempotencyKey } });
    if (response.status !== 200) return { status: failure(response.status) };
    const result = parseIdentityBindResult(response.data);
    return result ? { status: "bound", result } : { status: "unavailable" };
  } catch { return { status: "unavailable" }; }
}
