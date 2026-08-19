import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import {
  ExecutionRuntimePage,
  ExecutionRuntimeView,
  invalidateExecutionRuntimeRead,
  startExecutionRuntimeRead,
  type ExecutionRuntimeState,
} from "./execution-runtime-ui";
import type {
  ExecutionRuntimeSnapshot,
  ExecutionRuntimeTransport,
} from "./execution-runtime";

const state: ExecutionRuntimeState = {
  kind: "ready",
  snapshot: {
    available: true,
    control: {
      name: "local-control",
      state: "observed",
      observedAt: "2026-08-19T08:00:00Z",
    },
    observations: [
      {
        source: "channel_entry",
        queue: "channel",
        status: "observed",
        attempt: 2,
        observedAt: "2026-08-19T08:00:00Z",
      },
    ],
    truncated: false,
  },
};

function transport(): ExecutionRuntimeTransport {
  return {
    read: vi.fn(async () => ({ status: 200, data: {} })),
  } as ExecutionRuntimeTransport;
}

describe("execution runtime UI boundary", () => {
  it("renders only the approved local observation fields", () => {
    const html = renderToStaticMarkup(
      <ExecutionRuntimeView state={state} onLoad={vi.fn()} />,
    );
    expect(html).toContain("local-control");
    expect(html).toContain("channel_entry");
    expect(html).toContain("不触发 worker、Provider 或外部调用");
    for (const sensitive of [
      "details",
      "status_url",
      "message",
      "secret_ref",
      "external_userid",
    ])
      expect(html).not.toContain(sensitive);
  });

  it("keeps valid observations visible when the control is absent and marks truncation", () => {
    const html = renderToStaticMarkup(
      <ExecutionRuntimeView
        state={{
          kind: "ready",
          snapshot: {
            ...state.snapshot,
            available: false,
            control: null,
            truncated: true,
          },
        }}
        onLoad={vi.fn()}
      />,
    );
    expect(html).toContain("本地控制面当前未配置。");
    expect(html).toContain("channel_entry");
    expect(html).toContain("本地观测已截断。");
  });

  it.each(["ops", "sales"] as const)(
    "denies %s without any request",
    (role) => {
      const client = transport();
      const html = renderToStaticMarkup(
        <ExecutionRuntimePage role={role} transport={client} />,
      );
      expect(html).toContain("当前账号没有执行运行时访问权限。");
      expect(client.read).not.toHaveBeenCalled();
    },
  );

  it("single-flights same-tick reads and drops an unmounted old response", async () => {
    let resolveFirst: (() => void) | undefined;
    const first = new Promise<{ status: number; data: unknown }>((resolve) => {
      resolveFirst = () => resolve({ status: 503, data: {} });
    });
    const read = vi
      .fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({
        status: 200,
        data: {
          ok: true,
          control: {
            name: "current",
            state: "observed",
            details: {},
            observed_at: "2026-08-19T08:00:00Z",
          },
          observations: [],
          truncated: false,
          observed_at: "2026-08-19T08:00:00Z",
          observed_only: true,
          real_external_call_executed: false,
        },
      });
    const states: ExecutionRuntimeState[] = [];
    const controller = {
      generation: { current: 0 },
      inFlight: { current: false },
      verified: { current: undefined as ExecutionRuntimeSnapshot | undefined },
      onState: (next: ExecutionRuntimeState) => states.push(next),
    };
    const oldRead = startExecutionRuntimeRead(controller, {
      read,
    } as ExecutionRuntimeTransport);
    await startExecutionRuntimeRead(controller, {
      read,
    } as ExecutionRuntimeTransport);
    expect(read).toHaveBeenCalledTimes(1);
    invalidateExecutionRuntimeRead(controller);
    await startExecutionRuntimeRead(controller, {
      read,
    } as ExecutionRuntimeTransport);
    expect(states.at(-1)).toMatchObject({
      kind: "ready",
      snapshot: { control: { name: "current" } },
    });
    resolveFirst?.();
    await oldRead;
    expect(states.at(-1)).toMatchObject({
      kind: "ready",
      snapshot: { control: { name: "current" } },
    });
    expect(controller.inFlight.current).toBe(false);
  });

  it("calls the unauthenticated callback once and releases the read lock", async () => {
    const onUnauthenticated = vi.fn();
    const controller = {
      generation: { current: 0 },
      inFlight: { current: false },
      verified: { current: undefined as ExecutionRuntimeSnapshot | undefined },
      onState: vi.fn(),
      onUnauthenticated,
    };
    await startExecutionRuntimeRead(controller, {
      read: vi.fn(async () => ({ status: 401, data: {} })),
    } as ExecutionRuntimeTransport);
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    expect(controller.inFlight.current).toBe(false);
  });
});
