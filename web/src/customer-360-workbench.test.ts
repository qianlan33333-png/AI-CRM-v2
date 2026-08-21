import { describe, expect, it, vi } from "vitest";
import type { CustomerDetailTransport } from "./customer-detail";
import {
  Customer360LatestRequestGate,
  Customer360MutationGuard,
  executeCustomer360Mutation,
  mergeConfirmedCustomer360Core,
  newCustomer360IdempotencyKey,
  readCustomer360CoreProjection,
  resolveCustomer360Access,
  stageName,
  type Customer360MutationAction,
} from "./customer-360-workbench-model";

const CSRF = "A".repeat(43);
const UUID = "123e4567-e89b-42d3-a456-426614174000";
const IDEMPOTENCY = `customer-360:profile:${UUID}`;
const VERSION_1 = "2026-08-21T01:00:00Z";
const VERSION_2 = "2026-08-21T02:00:00Z";

function customer(
  id: number,
  updatedAt: string,
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    id,
    name: "林小姐",
    gender: 1,
    stage_id: 3,
    owner_staff_id: 11,
    channel_id: 5,
    added_at: "2026-08-20T00:00:00Z",
    last_interact_at: "2026-08-20T01:00:00Z",
    is_deleted: false,
    extra: {},
    created_at: "2026-08-19T00:00:00Z",
    updated_at: updatedAt,
    ...overrides,
  };
}

function tag(id: number): Record<string, unknown> {
  return {
    id,
    group_id: 2,
    group_name: "意向",
    name: id === 9 ? "已报名" : "待联系",
    sort_order: id * 10,
  };
}

function detail(
  id: number,
  updatedAt: string,
  overrides: Record<string, unknown> = {},
  tags: readonly Record<string, unknown>[] = [tag(9)],
): Record<string, unknown> {
  return {
    customer: customer(id, updatedAt, overrides),
    tags,
  };
}

function transport(overrides: Partial<CustomerDetailTransport> = {}) {
  return {
    get: vi.fn(),
    update: vi.fn(),
    setStage: vi.fn(),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    listEvents: vi.fn(),
    listTags: vi.fn(),
    ...overrides,
  } as unknown as CustomerDetailTransport;
}

const profileAction: Customer360MutationAction = {
  kind: "profile",
  update: {
    name: "林女士",
    avatarURL: null,
    gender: 1,
    ownerStaffID: 11,
    channelID: 5,
  },
};

describe("customer 360 RBAC and request ownership", () => {
  it("allows only exact admin/ops principals and fails closed for malformed input", () => {
    expect(
      resolveCustomer360Access({ adminUserID: 1, role: "admin" }),
    ).toMatchObject({ status: "allowed" });
    expect(
      resolveCustomer360Access({ adminUserID: 2, role: "ops", staffID: 8 }),
    ).toMatchObject({ status: "allowed" });
    expect(
      resolveCustomer360Access({ adminUserID: 3, role: "sales", staffID: 9 }),
    ).toEqual({ status: "forbidden" });
    expect(resolveCustomer360Access(undefined)).toEqual({
      status: "unauthenticated",
    });
    expect(
      resolveCustomer360Access({
        adminUserID: 1,
        role: "admin",
        external_userid: "leak",
      }),
    ).toEqual({ status: "forbidden" });
    expect(resolveCustomer360Access({ adminUserID: 0, role: "ops" })).toEqual({
      status: "forbidden",
    });
  });

  it("invalidates an old customer response after customer_id switches", async () => {
    const gate = new Customer360LatestRequestGate();
    const old = gate.begin(7);
    expect(old).toBeDefined();

    let release: () => void = () => {};
    const delayed = new Promise<void>((resolve) => {
      release = resolve;
    });
    let applied = false;
    const oldRequest = delayed.then(() => {
      if (old && gate.isCurrent(old)) applied = true;
    });

    const current = gate.begin(8);
    expect(current).toBeDefined();
    release();
    await oldRequest;

    expect(applied).toBe(false);
    expect(old && gate.isCurrent(old)).toBe(false);
    expect(current && gate.isCurrent(current)).toBe(true);
  });

  it("locks duplicate and unknown-outcome writes until an explicit reset", () => {
    const guard = new Customer360MutationGuard();
    expect(guard.begin()).toBe("started");
    expect(guard.begin()).toBe("busy");
    guard.lock("outcome_unknown");
    expect(guard.begin()).toBe("locked");
    expect(guard.state()).toEqual({
      inFlight: false,
      locked: "outcome_unknown",
    });
    guard.reset();
    expect(guard.begin()).toBe("started");
    guard.finishKnown();
    expect(guard.state()).toEqual({ inFlight: false });
  });
});

describe("customer 360 strict local mutation protocol", () => {
  it("generates one operation-bound idempotency key", () => {
    expect(
      newCustomer360IdempotencyKey("profile", { randomUUID: () => UUID }),
    ).toBe(IDEMPOTENCY);
    expect(
      newCustomer360IdempotencyKey("stage", {
        randomUUID: () => "not-a-uuid",
      }),
    ).toBeUndefined();
  });

  it("preflights expected_version, writes once, and confirms by strict readback", async () => {
    const get = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_1) })
      .mockResolvedValueOnce({
        status: 200,
        data: detail(7, VERSION_2, { name: "林女士" }),
      });
    const update = vi.fn().mockResolvedValue({ status: 200, data: {} });
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toMatchObject({
      status: "confirmed",
      idempotencyKey: IDEMPOTENCY,
      core: { customer: { id: 7, name: "林女士", updatedAt: VERSION_2 } },
    });

    expect(get).toHaveBeenCalledTimes(2);
    expect(update).toHaveBeenCalledOnce();
    expect(update).toHaveBeenCalledWith(
      7,
      {
        name: "林女士",
        avatar_url: null,
        gender: 1,
        owner_staff_id: 11,
        channel_id: 5,
      },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": CSRF,
          "Idempotency-Key": IDEMPOTENCY,
          "X-Expected-Version": VERSION_1,
        },
      },
    );
  });

  it("detects a changed expected_version before write and does not send mutation", async () => {
    const get = vi.fn().mockResolvedValue({
      status: 200,
      data: detail(7, VERSION_2),
    });
    const update = vi.fn();
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toMatchObject({
      status: "conflict",
      reason: "expected_version_changed",
    });
    expect(update).not.toHaveBeenCalled();
    expect(get).toHaveBeenCalledOnce();
  });

  it("keeps an explicit 409 distinct and never retries", async () => {
    const get = vi.fn().mockResolvedValue({
      status: 200,
      data: detail(7, VERSION_1),
    });
    const update = vi.fn().mockResolvedValue({ status: 409, data: {} });
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toEqual({
      status: "conflict",
      reason: "server_conflict",
      idempotencyKey: IDEMPOTENCY,
    });
    expect(update).toHaveBeenCalledOnce();
    expect(get).toHaveBeenCalledOnce();
  });

  it("marks a transport interruption as unknown outcome and never retries", async () => {
    const get = vi.fn().mockResolvedValue({
      status: 200,
      data: detail(7, VERSION_1),
    });
    const update = vi.fn().mockRejectedValue(new Error("connection reset"));
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toEqual({
      status: "outcome_unknown",
      reason: "write_transport_failed",
      idempotencyKey: IDEMPOTENCY,
    });
    expect(update).toHaveBeenCalledOnce();
    expect(get).toHaveBeenCalledOnce();
  });

  it("treats an unexpected write status as unknown instead of retrying", async () => {
    const get = vi.fn().mockResolvedValue({
      status: 200,
      data: detail(7, VERSION_1),
    });
    const update = vi.fn().mockResolvedValue({ status: 503, data: {} });
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toMatchObject({
      status: "outcome_unknown",
      reason: "unexpected_write_status",
    });
    expect(update).toHaveBeenCalledOnce();
  });

  it("fails closed when readback contains an identity field", async () => {
    const get = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_1) })
      .mockResolvedValueOnce({
        status: 200,
        data: detail(7, VERSION_2, {
          name: "林女士",
          external_userid: "wm-forbidden",
        }),
      });
    const update = vi.fn().mockResolvedValue({ status: 200, data: {} });
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toEqual({
      status: "outcome_unknown",
      reason: "readback_invalid",
      idempotencyKey: IDEMPOTENCY,
    });
    expect(update).toHaveBeenCalledOnce();
  });

  it("confirms a tag add only when strict readback contains the tag", async () => {
    const tagKey = `customer-360:tag-add:${UUID}`;
    const get = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_1) })
      .mockResolvedValueOnce({
        status: 200,
        data: detail(7, VERSION_2, {}, [tag(9), tag(10)]),
      });
    const addTag = vi.fn().mockResolvedValue({ status: 204, data: undefined });
    const api = transport({ get, addTag });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        { kind: "tag-add", tagID: 10 },
        CSRF,
        tagKey,
      ),
    ).resolves.toMatchObject({ status: "confirmed" });
    expect(addTag).toHaveBeenCalledOnce();
  });



  it("writes a verified stage once and confirms the selected catalog ID", async () => {
    const stageKey = `customer-360:stage:${UUID}`;
    const get = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_1) })
      .mockResolvedValueOnce({
        status: 200,
        data: detail(7, VERSION_2, { stage_id: 5 }),
      });
    const setStage = vi.fn().mockResolvedValue({ status: 200, data: {} });
    const api = transport({ get, setStage });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        { kind: "stage", stageID: 5 },
        CSRF,
        stageKey,
      ),
    ).resolves.toMatchObject({ status: "confirmed" });
    expect(setStage).toHaveBeenCalledOnce();
    expect(setStage).toHaveBeenCalledWith(
      7,
      { stage_id: 5 },
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.objectContaining({
          "Idempotency-Key": stageKey,
          "X-Expected-Version": VERSION_1,
        }),
      }),
    );
  });

  it("removes a tag once and confirms absence by strict readback", async () => {
    const tagKey = `customer-360:tag-remove:${UUID}`;
    const get = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_1) })
      .mockResolvedValueOnce({
        status: 200,
        data: detail(7, VERSION_2, {}, []),
      });
    const removeTag = vi.fn().mockResolvedValue({ status: 204, data: undefined });
    const api = transport({ get, removeTag });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        { kind: "tag-remove", tagID: 9 },
        CSRF,
        tagKey,
      ),
    ).resolves.toMatchObject({ status: "confirmed" });
    expect(removeTag).toHaveBeenCalledOnce();
  });

  it("locks a successful-status write whose readback does not match", async () => {
    const get = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_1) })
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_2) });
    const update = vi.fn().mockResolvedValue({ status: 200, data: {} });
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toMatchObject({
      status: "conflict",
      reason: "readback_mismatch",
    });
    expect(update).toHaveBeenCalledOnce();
    expect(get).toHaveBeenCalledTimes(2);
  });

  it("treats an older post-write projection as an unknown stale readback", async () => {
    const staleVersion = "2026-08-21T00:30:00Z";
    const get = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: detail(7, VERSION_1) })
      .mockResolvedValueOnce({
        status: 200,
        data: detail(7, staleVersion, { name: "林女士" }),
      });
    const update = vi.fn().mockResolvedValue({ status: 200, data: {} });
    const api = transport({ get, update });

    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        CSRF,
        IDEMPOTENCY,
      ),
    ).resolves.toEqual({
      status: "outcome_unknown",
      reason: "stale_readback",
      idempotencyKey: IDEMPOTENCY,
    });
    expect(update).toHaveBeenCalledOnce();
  });

  it("rejects malformed security inputs before any transport call", async () => {
    const api = transport();
    await expect(
      executeCustomer360Mutation(
        api,
        7,
        VERSION_1,
        profileAction,
        "bad-token",
        IDEMPOTENCY,
      ),
    ).resolves.toEqual({
      status: "rejected",
      failure: "invalid",
      idempotencyKey: IDEMPOTENCY,
    });
    expect(api.get).not.toHaveBeenCalled();
    expect(api.update).not.toHaveBeenCalled();
  });
});

describe("customer 360 read fail-closed helpers", () => {
  it("rejects a malicious detail DTO without returning the raw payload", async () => {
    const api = transport({
      get: vi.fn().mockResolvedValue({
        status: 200,
        data: detail(7, VERSION_1, { mobile: "13800138000" }),
      }),
    });
    await expect(readCustomer360CoreProjection(api, 7)).resolves.toEqual({
      status: "invalid",
    });
  });

  it("merges only a same-customer confirmed readback", () => {
    const snapshot = {
      customer: {
        id: 7,
        name: "林小姐",
        isDeleted: false,
        createdAt: "2026-08-19T00:00:00Z",
        updatedAt: VERSION_1,
      },
      tags: [],
      tagCatalog: [],
      events: [],
      eventsHaveMore: false,
    };
    const same = {
      customer: { ...snapshot.customer, name: "林女士", updatedAt: VERSION_2 },
      tags: [],
    };
    expect(mergeConfirmedCustomer360Core(snapshot, same)?.customer.name).toBe(
      "林女士",
    );
    expect(
      mergeConfirmedCustomer360Core(snapshot, {
        customer: { ...same.customer, id: 8 },
        tags: [],
      }),
    ).toBeUndefined();
  });

  it("uses the verified stage catalog and keeps unknown stage IDs explicit", () => {
    const stages = [{ id: 3, name: "已报名", sortOrder: 10, config: {} }];
    expect(stageName(stages, 3)).toBe("已报名");
    expect(stageName(stages, undefined)).toBe("未设置");
    expect(stageName(stages, 99)).toBe("已归档或不可见阶段 #99");
  });
});
