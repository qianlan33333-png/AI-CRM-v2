import { describe, expect, it, vi } from "vitest";
import {
  loadExecutionRuntime,
  parseExecutionRuntime,
  type ExecutionRuntimeTransport,
} from "./execution-runtime";

const observedAt = "2026-08-19T08:00:00Z";
const runtime = {
  ok: true,
  control: {
    name: "local-control",
    state: "observed",
    details: { secret_ref: "[REDACTED]" },
    observed_at: observedAt,
  },
  observations: [
    {
      source: "channel_entry",
      queue: "channel",
      status: "observed",
      attempt: 2,
      status_url: "https://example.test/status",
      details: { external_userid: "[REDACTED]" },
      observed_at: observedAt,
    },
  ],
  truncated: false,
  observed_at: observedAt,
  observed_only: true,
  real_external_call_executed: false,
};

function transport(
  overrides: Partial<ExecutionRuntimeTransport> = {},
): ExecutionRuntimeTransport {
  return {
    read: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  } as ExecutionRuntimeTransport;
}

describe("execution runtime safe local-read contract", () => {
  it("projects only the frozen safe fields and discards diagnostics", () => {
    expect(parseExecutionRuntime(runtime)).toEqual({
      available: true,
      control: { name: "local-control", state: "observed", observedAt },
      observations: [
        {
          source: "channel_entry",
          queue: "channel",
          status: "observed",
          attempt: 2,
          observedAt,
        },
      ],
      truncated: false,
    });
    expect(JSON.stringify(parseExecutionRuntime(runtime))).not.toContain(
      "secret_ref",
    );
    expect(JSON.stringify(parseExecutionRuntime(runtime))).not.toContain(
      "status_url",
    );
    expect(JSON.stringify(parseExecutionRuntime(runtime))).not.toContain(
      "external_userid",
    );
  });

  it.each([
    { ...runtime, extra: true },
    { ...runtime, observed_only: false },
    { ...runtime, ok: false },
    { ...runtime, control: null },
    { ...runtime, real_external_call_executed: true },
    { ...runtime, control: { ...runtime.control, message: "no" } },
    { ...runtime, control: { ...runtime.control, details: null } },
    {
      ...runtime,
      observations: [
        { ...runtime.observations[0], details: { nested: { raw: "no" } } },
      ],
    },
    {
      ...runtime,
      observations: [{ ...runtime.observations[0], details: null }],
    },
    { ...runtime, observations: [{ ...runtime.observations[0], attempt: -1 }] },
  ])("fails closed for expanded or contradictory input %#", (value) => {
    expect(parseExecutionRuntime(value)).toBeUndefined();
  });

  it("uses one same-origin GET and maps status without retry", async () => {
    const client = transport({
      read: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: runtime })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadExecutionRuntime(client)).resolves.toMatchObject({
      status: "loaded",
    });
    await expect(loadExecutionRuntime(client)).resolves.toEqual({
      status: "unauthenticated",
    });
    await expect(loadExecutionRuntime(client)).resolves.toEqual({
      status: "unavailable",
    });
    expect(client.read).toHaveBeenCalledTimes(3);
    expect(client.read).toHaveBeenNthCalledWith(1, {
      credentials: "same-origin",
    });
  });
});
