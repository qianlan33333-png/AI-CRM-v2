import { describe, expect, it, vi } from "vitest";
import {
  PERMISSION_CACHE_TTL_MS,
  PermissionSessionCache,
  parsePrincipal,
  performLogout,
  permittedRoutePaths,
  readCSRFCookie,
  sessionResult,
  type AuthTransport,
} from "./auth";

const csrf = "a".repeat(43);

function session(role: "admin" | "ops" | "sales" = "admin") {
  return {
    status: 200,
    data: {
      admin_user_id: 7,
      role,
      ...(role === "sales" ? { staff_id: 9 } : {}),
    },
  };
}

describe("auth principal validation", () => {
  it.each([
    ["admin", { admin_user_id: 1, role: "admin" }],
    ["ops", { admin_user_id: 2, role: "ops", staff_id: 3 }],
    ["sales", { admin_user_id: 4, role: "sales", staff_id: 5 }],
  ])("accepts a valid %s principal", (_name, value) => {
    expect(parsePrincipal(value)).toBeDefined();
  });

  it.each([
    null,
    [],
    {},
    { admin_user_id: 0, role: "admin" },
    { admin_user_id: 1.5, role: "admin" },
    { admin_user_id: 1, role: "owner" },
    { admin_user_id: 1, role: "sales" },
    { admin_user_id: 1, role: "sales", staff_id: 0 },
    { admin_user_id: 1, role: "admin", extra: true },
  ])("rejects an unsafe principal %#", (value) => {
    expect(parsePrincipal(value)).toBeUndefined();
  });

  it("distinguishes 401, unavailable responses, and validated sessions", () => {
    expect(sessionResult({ status: 401, data: {} })).toEqual({
      status: "unauthenticated",
    });
    expect(sessionResult({ status: 500, data: session().data })).toEqual({
      status: "unavailable",
    });
    expect(sessionResult({ status: 200, data: { role: "admin" } })).toEqual({
      status: "unavailable",
    });
    expect(sessionResult(session())).toMatchObject({ status: "authenticated" });
  });
});

describe("PermissionSessionCache", () => {
  it("reuses a valid principal before 60 seconds and reloads exactly at expiry", async () => {
    let now = 0;
    const loader = vi.fn(async () => session());
    const cache = new PermissionSessionCache(loader, () => now);

    await cache.load();
    now = PERMISSION_CACHE_TTL_MS - 1;
    await cache.load();
    expect(loader).toHaveBeenCalledOnce();

    now = PERMISSION_CACHE_TTL_MS;
    await cache.load();
    expect(loader).toHaveBeenCalledTimes(2);
  });

  it("coalesces concurrent requests and supports forced refresh and invalidation", async () => {
    let resolve = (value: ReturnType<typeof session>) => {
      void value;
    };
    let loadCount = 0;
    const loader = vi.fn(() => {
      loadCount += 1;
      if (loadCount > 1) return Promise.resolve(session());
      return new Promise<ReturnType<typeof session>>(
        (done) => (resolve = done),
      );
    });
    const cache = new PermissionSessionCache(loader, () => 10);
    const first = cache.load();
    const second = cache.load(true);
    expect(loader).toHaveBeenCalledOnce();
    resolve(session());
    await expect(Promise.all([first, second])).resolves.toHaveLength(2);

    await cache.load(true);
    expect(loader).toHaveBeenCalledTimes(2);
    cache.invalidate();
    await cache.load();
    expect(loader).toHaveBeenCalledTimes(3);
  });

  it.each([
    async () => ({ status: 401, data: {} }),
    async () => ({ status: 500, data: {} }),
    async () => ({ status: 200, data: { admin_user_id: 1, role: "unknown" } }),
    async () => {
      throw new Error("offline");
    },
  ])("does not cache unsuccessful or malformed loads", async (loader) => {
    const tracked = vi.fn(loader);
    const cache = new PermissionSessionCache(tracked, () => 0);
    await cache.load();
    await cache.load();
    expect(tracked).toHaveBeenCalledTimes(2);
  });

  it("does not repopulate or return a principal when invalidated during an in-flight load", async () => {
    let resolve = (value: ReturnType<typeof session>) => {
      void value;
    };
    const loader = vi.fn(
      () => new Promise<ReturnType<typeof session>>((done) => (resolve = done)),
    );
    const cache = new PermissionSessionCache(loader, () => 0);

    const pending = cache.load();
    cache.invalidate();
    resolve(session());
    await expect(pending).resolves.toEqual({ status: "unauthenticated" });

    const afterInvalidation = cache.load();
    expect(loader).toHaveBeenCalledTimes(2);
    resolve(session());
    await expect(afterInvalidation).resolves.toMatchObject({
      status: "authenticated",
    });
  });
});

describe("permission navigation", () => {
  it("derives only currently frozen routes for each role", () => {
    expect(permittedRoutePaths({ adminUserID: 1, role: "admin" })).toEqual([
      "/",
      "/customers",
      "/stages",
      "/settings",
    ]);
    expect(permittedRoutePaths({ adminUserID: 1, role: "ops" })).toEqual([
      "/",
      "/customers",
      "/stages",
    ]);
    expect(
      permittedRoutePaths({ adminUserID: 1, role: "sales", staffID: 2 }),
    ).toEqual(["/", "/customers", "/stages"]);
    expect(permittedRoutePaths({ adminUserID: 1, role: "sales" })).toEqual([]);
  });
});

describe("logout boundary", () => {
  it("accepts exactly one well-shaped CSRF cookie", () => {
    expect(readCSRFCookie(`other=x; aicrm_csrf=${csrf}`)).toBe(csrf);
    expect(readCSRFCookie("other=x")).toBeUndefined();
    expect(readCSRFCookie("aicrm_csrf=short")).toBeUndefined();
    expect(
      readCSRFCookie(`aicrm_csrf=${csrf}; aicrm_csrf=${csrf}`),
    ).toBeUndefined();
  });

  it.each([
    [204, "logged_out", true],
    [401, "unauthenticated", true],
    [403, "forbidden", false],
    [500, "unavailable", false],
  ] as const)(
    "classifies logout %i without exposing the token",
    async (status, want, cleared) => {
      const transport: AuthTransport = {
        getSession: vi.fn(),
        logout: vi.fn(async (token) => {
          expect(token).toBe(csrf);
          return { status };
        }),
      };
      const cache = new PermissionSessionCache(
        async () => session(),
        () => 0,
      );
      await cache.load();
      const invalidate = vi.spyOn(cache, "invalidate");

      await expect(
        performLogout(transport, cache, `aicrm_csrf=${csrf}`),
      ).resolves.toBe(want);
      expect(invalidate).toHaveBeenCalledTimes(cleared ? 1 : 0);
      expect(want).not.toContain(csrf);
    },
  );

  it("does not call logout without a usable CSRF cookie or after a network error", async () => {
    const transport: AuthTransport = {
      getSession: vi.fn(),
      logout: vi.fn(async () => {
        throw new Error("offline");
      }),
    };
    const cache = new PermissionSessionCache(async () => session());

    await expect(performLogout(transport, cache, "")).resolves.toBe(
      "csrf_missing",
    );
    expect(transport.logout).not.toHaveBeenCalled();
    await expect(
      performLogout(transport, cache, `aicrm_csrf=${csrf}`),
    ).resolves.toBe("unavailable");
  });
});
