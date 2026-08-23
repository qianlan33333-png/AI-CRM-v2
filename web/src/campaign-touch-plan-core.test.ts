import { describe, expect, it, vi } from "vitest";
import {
  CampaignTouchPlanInflightGuard,
  CampaignTouchPlanMachine,
  campaignTouchPlanPendingStorageKey,
  loadDraftCampaign,
  loadDraftCampaigns,
  type CampaignDraft,
  type CampaignTouchPlanTransport,
  type SessionStorageLike,
} from "./campaign-touch-plan-core";

const csrf = "c".repeat(43);
const uuidA = "123e4567-e89b-42d3-a456-426614174000";
const uuidB = "223e4567-e89b-42d3-a456-426614174000";
const digestA = "a".repeat(64);
const digestB = "b".repeat(64);
const planID = `ctp_${digestA}`;
const now = "2026-08-24T01:02:03.123456Z";
const safety = {
  local_only: true,
  provider_execution_eligible: false,
  runtime_executed: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const campaignSafety = {
  local_projection: true,
  real_external_call_executed: false,
  real_send: false,
  runtime_executed: false,
};

const rawCampaign = (version = 4) => ({
  campaign_code: "spring-campaign",
  name: "春季 Campaign",
  approval_status: "draft",
  runtime_status: "idle",
  version,
  created_by: 7,
  updated_by: 7,
  created_at: now,
  updated_at: now,
});
const rawCampaignDetail = (version = 4) => ({
  campaign: rawCampaign(version),
  steps: [{ step_index: 1, delay_minutes: 0, content: "本地内容" }],
  ...campaignSafety,
});
const sourceFact = (
  kind:
    "customer_selection" | "segment_members" | "ai_audience_package_members",
) => {
  if (kind === "customer_selection")
    return {
      kind,
      customer_selection: {
        id: "local_selection",
        version: "v1",
        digest: digestA,
      },
    };
  if (kind === "segment_members")
    return {
      kind,
      segment: {
        segment_id: 7,
        member_snapshot_watermark: now,
        digest: digestA,
      },
    };
  return {
    kind,
    audience_package: {
      package_id: 7,
      package_version: 2,
      member_snapshot_watermark: now,
      digest: digestA,
    },
  };
};
const rawPlan = (
  kind:
    | "customer_selection"
    | "segment_members"
    | "ai_audience_package_members" = "customer_selection",
  version = 4,
) => ({
  id: planID,
  campaign_code: "spring-campaign",
  campaign_version: version,
  source: sourceFact(kind),
  target_count: 1,
  target_digest: digestA,
  content: {
    steps: [{ step_index: 1, delay_minutes: 0, content: "本地内容" }],
    content_digest: digestB,
  },
  owner_actor_id: 7,
  preview_exclusion_summary: {
    candidate_count: 1,
    active_customer_count: 1,
    inactive_excluded_count: 0,
    policy_excluded_count: 0,
  },
  created_at: now,
  ...safety,
});

class MemorySessionStorage implements SessionStorageLike {
  readonly values = new Map<string, string>();
  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }
  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
  removeItem(key: string): void {
    this.values.delete(key);
  }
}

function response(status: number, data: unknown) {
  return { status, data };
}
function transport(
  overrides: Partial<CampaignTouchPlanTransport> = {},
): CampaignTouchPlanTransport {
  return {
    listCampaigns: vi.fn(async () =>
      response(200, { items: [rawCampaign()], ...campaignSafety }),
    ),
    getCampaign: vi.fn(async () => response(200, rawCampaignDetail())),
    createPlan: vi.fn(async () => response(201, rawPlan())),
    getPlan: vi.fn(async () => response(200, rawPlan())),
    ...overrides,
  };
}
async function draft(
  client: CampaignTouchPlanTransport = transport(),
): Promise<CampaignDraft> {
  const result = await loadDraftCampaign(client, "spring-campaign");
  if (result.status !== "loaded") throw new Error("draft fixture unavailable");
  return result.campaign;
}
function machine(
  client: CampaignTouchPlanTransport,
  storage = new MemorySessionStorage(),
  actorID = 7,
  uuids: string[] = [uuidA],
) {
  let index = 0;
  return new CampaignTouchPlanMachine({
    transport: client,
    sessionStorage: storage,
    actorID,
    keySource: { randomUUID: () => uuids[index++] ?? uuids.at(-1)! },
  });
}
function input(
  campaign: CampaignDraft,
  sourceKind = "customer_selection",
  sourceID = "7",
) {
  return {
    campaign,
    source_kind: sourceKind,
    source_id: sourceID,
    csrf,
    confirmed: true,
  };
}

describe("Campaign touch-plan frontend core", () => {
  it("strictly reads only closed draft+idle campaigns with valid local steps", async () => {
    const client = transport();
    await expect(loadDraftCampaigns(client)).resolves.toMatchObject({
      status: "loaded",
      campaigns: [{ code: "spring-campaign", version: 4 }],
    });
    expect(client.listCampaigns).toHaveBeenCalledWith(
      { approval_status: "draft", runtime_status: "idle" },
      { credentials: "same-origin" },
    );
    await expect(
      loadDraftCampaign(client, "spring-campaign"),
    ).resolves.toMatchObject({
      status: "loaded",
      campaign: { steps: [{ index: 1 }] },
    });

    for (const data of [
      {
        items: [{ ...rawCampaign(), approval_status: "approved" }],
        ...campaignSafety,
      },
      { items: [rawCampaign()], ...campaignSafety, real_send: true },
      {
        items: [{ ...rawCampaign(), created_at: "garbage" }],
        ...campaignSafety,
      },
      {
        items: [{ ...rawCampaign(), provider_result: "forbidden" }],
        ...campaignSafety,
      },
    ]) {
      await expect(
        loadDraftCampaigns(
          transport({ listCampaigns: vi.fn(async () => response(200, data)) }),
        ),
      ).resolves.toEqual({ status: "unavailable" });
    }
    for (const data of [
      { ...rawCampaignDetail(), steps: [] },
      {
        ...rawCampaignDetail(),
        steps: [{ step_index: 2, delay_minutes: 0, content: "x" }],
      },
      { ...rawCampaignDetail(), provider_result: "forbidden" },
    ]) {
      await expect(
        loadDraftCampaign(
          transport({ getCampaign: vi.fn(async () => response(200, data)) }),
          "spring-campaign",
        ),
      ).resolves.toEqual({ status: "unavailable" });
    }
  });

  it("maps the three launch sources only after the safe-integer confirmation gate", async () => {
    for (const [kind, expected] of [
      ["customer_selection", { kind: "customer_selection", customer_ids: [7] }],
      ["segment_members", { kind: "segment_members", segment_id: 7 }],
      [
        "ai_audience_package_members",
        { kind: "ai_audience_package_members", audience_package_id: 7 },
      ],
    ] as const) {
      const client = transport({
        createPlan: vi.fn(async () => response(201, rawPlan(kind))),
        getPlan: vi.fn(async () => response(200, rawPlan(kind))),
      });
      await expect(
        machine(client).start(input(await draft(), kind)),
      ).resolves.toMatchObject({ status: "created" });
      expect(client.createPlan).toHaveBeenCalledWith(
        "spring-campaign",
        { expected_campaign_version: 4, source: expected },
        expect.objectContaining({
          credentials: "same-origin",
          headers: expect.objectContaining({
            "X-CSRF-Token": csrf,
            "Idempotency-Key": `campaign-touch-plan:${uuidA}`,
          }),
        }),
      );
    }

    const client = transport();
    const creator = machine(client);
    const campaign = await draft();
    await expect(
      creator.start({ ...input(campaign), confirmed: false }),
    ).resolves.toEqual({ status: "confirmation_required" });
    await expect(
      creator.start({ ...input(campaign), csrf: "bad" }),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      creator.start(input(campaign, "segment_members", "9007199254740992")),
    ).resolves.toMatchObject({ status: "blocked_redline" });
    await expect(
      creator.start(input(campaign, "unknown", "7")),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.createPlan).not.toHaveBeenCalled();
  });

  it("blocks a duplicate synchronously while the first request is inflight", async () => {
    // eslint-disable-next-line no-unused-vars -- resolver parameter is supplied by Promise resolution.
    let release!: (value: ReturnType<typeof response>) => void;
    const pending = new Promise<ReturnType<typeof response>>((resolve) => {
      release = resolve;
    });
    const client = transport({ createPlan: vi.fn(() => pending) });
    const guard = new CampaignTouchPlanInflightGuard();
    const creator = new CampaignTouchPlanMachine({
      transport: client,
      sessionStorage: new MemorySessionStorage(),
      actorID: 7,
      keySource: { randomUUID: () => uuidA },
      inflightGuard: guard,
    });
    const request = input(await draft());
    const first = creator.start(request);
    expect(guard.isActive()).toBe(true);
    await expect(creator.start(request)).resolves.toEqual({
      status: "inflight",
    });
    expect(client.createPlan).toHaveBeenCalledTimes(1);
    release(response(201, rawPlan()));
    await expect(first).resolves.toMatchObject({ status: "created" });
    expect(guard.isActive()).toBe(false);
  });

  it("keeps one canonical pending intent and permits only exact replay", async () => {
    const storage = new MemorySessionStorage();
    const createPlan = vi
      .fn()
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce(response(201, rawPlan("segment_members")));
    const client = transport({
      createPlan,
      getPlan: vi.fn(async () => response(200, rawPlan("segment_members"))),
    });
    const creator = machine(client, storage);
    const request = input(await draft(), "segment_members");
    await expect(creator.start(request)).resolves.toEqual({
      status: "outcome_unknown",
    });
    const raw = storage.getItem(campaignTouchPlanPendingStorageKey(7));
    expect(raw).toContain(`"idempotency_key":"campaign-touch-plan:${uuidA}"`);
    expect(raw).toContain(`"source":{"kind":"segment_members","segment_id":7}`);
    await expect(creator.start(request)).resolves.toEqual({
      status: "replay_required",
    });
    await expect(
      creator.replay({ ...request, source_id: "8" }),
    ).resolves.toEqual({ status: "replay_mismatch" });
    expect(createPlan).toHaveBeenCalledTimes(1);
    await expect(creator.replay(request)).resolves.toMatchObject({
      status: "created",
    });
    expect(createPlan).toHaveBeenCalledTimes(2);
    const keys = createPlan.mock.calls.map(
      (call) => (call[2].headers as Record<string, string>)["Idempotency-Key"],
    );
    expect(keys).toEqual([
      `campaign-touch-plan:${uuidA}`,
      `campaign-touch-plan:${uuidA}`,
    ]);
    expect(storage.getItem(campaignTouchPlanPendingStorageKey(7))).toBeNull();
  });

  it("rejects a reordered non-canonical pending payload without a request", async () => {
    const storage = new MemorySessionStorage();
    storage.setItem(
      campaignTouchPlanPendingStorageKey(7),
      JSON.stringify({
        version: 1,
        idempotency_key: `campaign-touch-plan:${uuidA}`,
        payload: {
          source: { segment_id: 7, kind: "segment_members" },
          expected_campaign_version: 4,
          campaign_code: "spring-campaign",
        },
      }),
    );
    const client = transport();
    await expect(
      machine(client, storage).replay(input(await draft(), "segment_members")),
    ).resolves.toEqual({ status: "storage_unavailable" });
    expect(client.createPlan).not.toHaveBeenCalled();
  });

  it.each([
    ["arbitrary minimum-length text", "x".repeat(16)],
    ["arbitrary maximum-length text", "x".repeat(128)],
    ["wrong prefix case", `Campaign-touch-plan:${uuidA}`],
    ["extra suffix", `campaign-touch-plan:${uuidA}x`],
    [
      "unsupported UUID version",
      "campaign-touch-plan:123e4567-e89b-62d3-a456-426614174000",
    ],
  ])("rejects a tampered pending key (%s) without a request", async (_name, key) => {
    const storage = new MemorySessionStorage();
    storage.setItem(
      campaignTouchPlanPendingStorageKey(7),
      JSON.stringify({
        version: 1,
        idempotency_key: key,
        payload: {
          campaign_code: "spring-campaign",
          expected_campaign_version: 4,
          source: { kind: "segment_members", segment_id: 7 },
        },
      }),
    );
    const client = transport();
    await expect(
      machine(client, storage).replay(input(await draft(), "segment_members")),
    ).resolves.toEqual({ status: "storage_unavailable" });
    expect(client.createPlan).not.toHaveBeenCalled();
  });

  it("restores a pending key emitted from an uppercase canonical UUID", async () => {
    const upperUUID = uuidA.toUpperCase();
    const storage = new MemorySessionStorage();
    storage.setItem(
      campaignTouchPlanPendingStorageKey(7),
      JSON.stringify({
        version: 1,
        idempotency_key: `campaign-touch-plan:${upperUUID}`,
        payload: {
          campaign_code: "spring-campaign",
          expected_campaign_version: 4,
          source: { kind: "segment_members", segment_id: 7 },
        },
      }),
    );
    const createPlan = vi.fn(async () =>
      response(201, rawPlan("segment_members")),
    );
    const client = transport({
      createPlan,
      getPlan: vi.fn(async () => response(200, rawPlan("segment_members"))),
    });
    await expect(
      machine(client, storage).replay(input(await draft(), "segment_members")),
    ).resolves.toMatchObject({ status: "created" });
    expect(createPlan).toHaveBeenCalledWith(
      "spring-campaign",
      expect.any(Object),
      expect.objectContaining({
        headers: expect.objectContaining({
          "Idempotency-Key": `campaign-touch-plan:${upperUUID}`,
        }),
      }),
    );
  });

  it.each([
    [
      "network",
      () => Promise.reject(new Error("network")),
      async () => response(200, rawPlan()),
    ],
    [
      "malformed 201",
      async () => response(201, { ...rawPlan(), provider_result: "forbidden" }),
      async () => response(200, rawPlan()),
    ],
    [
      "malformed readback",
      async () => response(201, rawPlan()),
      async () => response(200, { ...rawPlan(), local_only: false }),
    ],
    [
      "mismatched readback",
      async () => response(201, rawPlan()),
      async () => response(200, { ...rawPlan(), target_digest: digestB }),
    ],
  ])(
    "keeps pending state when %s makes the outcome unknown",
    async (_name, create, read) => {
      const storage = new MemorySessionStorage();
      const creator = machine(
        transport({ createPlan: vi.fn(create), getPlan: vi.fn(read) }),
        storage,
      );
      const request = input(await draft());
      await expect(creator.start(request)).resolves.toEqual({
        status: "outcome_unknown",
      });
      expect(
        storage.getItem(campaignTouchPlanPendingStorageKey(7)),
      ).not.toBeNull();
      await expect(creator.start(request)).resolves.toEqual({
        status: "replay_required",
      });
    },
  );

  it("refreshes a 409 conflict but requires an explicit new intent and new key", async () => {
    const storage = new MemorySessionStorage();
    const createPlan = vi
      .fn()
      .mockResolvedValueOnce(response(409, { code: "CONFLICT" }))
      .mockResolvedValueOnce(response(201, rawPlan("customer_selection", 5)));
    const client = transport({
      createPlan,
      getCampaign: vi.fn(async () => response(200, rawCampaignDetail(5))),
      getPlan: vi.fn(async () =>
        response(200, rawPlan("customer_selection", 5)),
      ),
    });
    const creator = machine(client, storage, 7, [uuidA, uuidB]);
    await expect(creator.start(input(await draft()))).resolves.toMatchObject({
      status: "conflict",
      campaign: { version: 5 },
    });
    expect(createPlan).toHaveBeenCalledTimes(1);
    expect(storage.getItem(campaignTouchPlanPendingStorageKey(7))).toBeNull();
    await expect(creator.replay(input(await draft()))).resolves.toEqual({
      status: "no_pending",
    });
    await expect(
      creator.start(input(await draft(client))),
    ).resolves.toMatchObject({ status: "created" });
    const keys = createPlan.mock.calls.map(
      (call) => (call[2].headers as Record<string, string>)["Idempotency-Key"],
    );
    expect(keys).toEqual([
      `campaign-touch-plan:${uuidA}`,
      `campaign-touch-plan:${uuidB}`,
    ]);
  });

  it("never replays BLOCKED_REDLINE and isolates pending intent by actor", async () => {
    const blockedStorage = new MemorySessionStorage();
    const blockedClient = transport({
      createPlan: vi.fn(async () => response(409, { code: "BLOCKED_REDLINE" })),
    });
    const blocked = machine(blockedClient, blockedStorage);
    const request = input(await draft());
    await expect(blocked.start(request)).resolves.toMatchObject({
      status: "blocked_redline",
    });
    await expect(blocked.replay(request)).resolves.toEqual({
      status: "no_pending",
    });
    expect(blockedClient.createPlan).toHaveBeenCalledTimes(1);

    const shared = new MemorySessionStorage();
    const actor7 = machine(
      transport({
        createPlan: vi.fn(async () => {
          throw new Error("network");
        }),
      }),
      shared,
      7,
      [uuidA],
    );
    const actor8Client = transport({
      createPlan: vi.fn(async () =>
        response(201, { ...rawPlan(), owner_actor_id: 8 }),
      ),
      getPlan: vi.fn(async () =>
        response(200, { ...rawPlan(), owner_actor_id: 8 }),
      ),
    });
    const actor8 = machine(actor8Client, shared, 8, [uuidB]);
    await expect(actor7.start(request)).resolves.toEqual({
      status: "outcome_unknown",
    });
    await expect(actor8.replay(request)).resolves.toEqual({
      status: "no_pending",
    });
    await expect(actor8.start(request)).resolves.toMatchObject({
      status: "created",
    });
    expect(
      shared.getItem(campaignTouchPlanPendingStorageKey(7)),
    ).not.toBeNull();
    expect(shared.getItem(campaignTouchPlanPendingStorageKey(8))).toBeNull();
  });
});
