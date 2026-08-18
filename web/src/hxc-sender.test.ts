import { describe, expect, it, vi } from "vitest";
import { loadHXCSenders, type HXCSenderTransport } from "./hxc-sender";

const body = {
  ok: true,
  source_status: "v2_local_staff",
  route_owner: "aicrm_v2",
  fallback_used: false,
  real_external_call_executed: false,
  send_configs: [
    {
      id: "cfg-1",
      sender_userid: "alice",
      display_name: "Alice",
      priority: 2,
      is_active: true,
      created_at: "2026-08-19T00:00:00Z",
      updated_at: "2026-08-19T00:00:00Z",
    },
  ],
  directory_candidates: [
    {
      wecom_userid: "alice",
      display_name: "Alice",
      position: "",
      wecom_status: 0,
      is_sender: true,
      priority: 2,
      is_active: true,
    },
  ],
  members: [
    {
      wecom_userid: "alice",
      display_name: "Alice",
      position: "",
      wecom_status: 0,
      is_sender: true,
      priority: 2,
      is_active: true,
    },
  ],
  directory_count: 1,
  sender_count: 1,
  active_sender_count: 1,
  last_synced_at: "2026-08-19T00:00:00Z",
  warnings: [
    "HXC senders use the local staff projection; no WeCom directory call was executed.",
  ],
  degraded: false,
  empty_state: false,
} as const;

function transport(status: number, data: unknown): HXCSenderTransport {
  return {
    read: vi.fn(async () => ({ status, data, headers: new Headers() })),
  } as unknown as HXCSenderTransport;
}

describe("HXC sender read boundary", () => {
  it("uses the generated operation result and accepts only the frozen projection", async () => {
    await expect(loadHXCSenders(transport(200, body))).resolves.toMatchObject({
      status: "loaded",
      model: {
        activeSenderCount: 1,
        members: [{ wecomUserID: "alice", isSender: true }],
      },
    });
  });

  it("rejects sensitive or non-frozen fields without rendering them", async () => {
    const sensitive = {
      ...body,
      members: [{ ...body.members[0], mobile: "should-never-render" }],
    };
    await expect(loadHXCSenders(transport(200, sensitive))).resolves.toEqual({
      status: "invalid",
    });
  });

  it("preserves fail-closed HTTP outcomes", async () => {
    await expect(loadHXCSenders(transport(401, {}))).resolves.toEqual({
      status: "unauthenticated",
    });
    await expect(loadHXCSenders(transport(403, {}))).resolves.toEqual({
      status: "forbidden",
    });
    await expect(loadHXCSenders(transport(503, {}))).resolves.toEqual({
      status: "unavailable",
    });
  });
});
