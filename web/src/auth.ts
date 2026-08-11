import { getAuthSession, logoutAdmin } from "./api/generated/health";

export const PERMISSION_CACHE_TTL_MS = 60_000;
export const CSRF_COOKIE_NAME = "aicrm_csrf";

export type AuthRole = "admin" | "ops" | "sales";

export interface AuthPrincipal {
  adminUserID: number;
  role: AuthRole;
  staffID?: number;
}

export type SessionResult =
  | { status: "authenticated"; principal: AuthPrincipal }
  | { status: "unauthenticated" }
  | { status: "unavailable" };

export type LogoutResult =
  | "logged_out"
  | "unauthenticated"
  | "csrf_missing"
  | "forbidden"
  | "unavailable";

async function loadGeneratedSession(): Promise<{
  status: number;
  data: unknown;
}> {
  const response = await getAuthSession({ credentials: "same-origin" });
  return { status: response.status, data: response.data };
}

async function logoutGeneratedSession(
  csrfToken: string,
): Promise<{ status: number }> {
  const response = await logoutAdmin({
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrfToken },
  });
  return { status: response.status };
}

export type AuthTransport = {
  getSession: typeof loadGeneratedSession;
  logout: typeof logoutGeneratedSession;
};

export const generatedAuthTransport: AuthTransport = {
  getSession: loadGeneratedSession,
  logout: logoutGeneratedSession,
};

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

export function parsePrincipal(value: unknown): AuthPrincipal | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return undefined;
  }

  const candidate = value as Record<string, unknown>;
  const allowedKeys = new Set(["admin_user_id", "role", "staff_id"]);
  if (Object.keys(candidate).some((key) => !allowedKeys.has(key))) {
    return undefined;
  }
  if (!positiveInteger(candidate.admin_user_id)) return undefined;
  if (
    candidate.role !== "admin" &&
    candidate.role !== "ops" &&
    candidate.role !== "sales"
  ) {
    return undefined;
  }

  const staff = candidate.staff_id;
  if (staff !== undefined && staff !== null && !positiveInteger(staff)) {
    return undefined;
  }
  if (candidate.role === "sales" && !positiveInteger(staff)) {
    return undefined;
  }

  return {
    adminUserID: candidate.admin_user_id,
    role: candidate.role,
    ...(positiveInteger(staff) ? { staffID: staff } : {}),
  };
}

export function sessionResult(response: {
  status: number;
  data: unknown;
}): SessionResult {
  if (response.status === 401) return { status: "unauthenticated" };
  if (response.status !== 200) return { status: "unavailable" };

  const principal = parsePrincipal(response.data);
  return principal
    ? { status: "authenticated", principal }
    : { status: "unavailable" };
}

export class PermissionSessionCache {
  private cached?: { principal: AuthPrincipal; expiresAt: number };
  private inFlight?: Promise<SessionResult>;
  private generation = 0;
  private readonly loadSession: () => Promise<{
    status: number;
    data: unknown;
  }>;
  private readonly now: () => number;

  constructor(
    loadSession: () => Promise<{
      status: number;
      data: unknown;
    }>,
    now: () => number = Date.now,
  ) {
    this.loadSession = loadSession;
    this.now = now;
  }

  load(force = false): Promise<SessionResult> {
    const current = this.now();
    if (!force && this.cached && current < this.cached.expiresAt) {
      return Promise.resolve({
        status: "authenticated",
        principal: this.cached.principal,
      });
    }
    if (this.inFlight) return this.inFlight;

    const requestGeneration = this.generation;
    const request = this.loadSession()
      .then(sessionResult)
      .catch((): SessionResult => ({ status: "unavailable" }))
      .then((result) => {
        if (requestGeneration !== this.generation) {
          return { status: "unauthenticated" } as SessionResult;
        }
        if (result.status === "authenticated") {
          this.cached = {
            principal: result.principal,
            expiresAt: this.now() + PERMISSION_CACHE_TTL_MS,
          };
        } else {
          this.cached = undefined;
        }
        return result;
      })
      .finally(() => {
        if (this.inFlight === request) this.inFlight = undefined;
      });
    this.inFlight = request;
    return request;
  }

  invalidate(): void {
    this.generation += 1;
    this.cached = undefined;
  }
}

export function permittedRoutePaths(
  principal: AuthPrincipal,
): readonly string[] {
  const validated = parsePrincipal({
    admin_user_id: principal.adminUserID,
    role: principal.role,
    ...(principal.staffID === undefined ? {} : { staff_id: principal.staffID }),
  });
  if (!validated) return [];

  const paths = ["/", "/customers", "/stages"];
  if (validated.role === "admin") paths.push("/settings");
  return paths;
}

export function readCSRFCookie(cookieHeader: string): string | undefined {
  const values = cookieHeader
    .split(";")
    .map((part) => part.trim())
    .filter((part) => part.startsWith(`${CSRF_COOKIE_NAME}=`))
    .map((part) => part.slice(CSRF_COOKIE_NAME.length + 1));

  if (values.length !== 1 || !/^[A-Za-z0-9_-]{43}$/.test(values[0] ?? "")) {
    return undefined;
  }
  return values[0];
}

export async function performLogout(
  transport: AuthTransport,
  cache: PermissionSessionCache,
  cookieHeader: string,
): Promise<LogoutResult> {
  const csrfToken = readCSRFCookie(cookieHeader);
  if (!csrfToken) return "csrf_missing";

  try {
    const response = await transport.logout(csrfToken);
    if (response.status === 204) {
      cache.invalidate();
      return "logged_out";
    }
    if (response.status === 401) {
      cache.invalidate();
      return "unauthenticated";
    }
    if (response.status === 403) return "forbidden";
    return "unavailable";
  } catch {
    return "unavailable";
  }
}
