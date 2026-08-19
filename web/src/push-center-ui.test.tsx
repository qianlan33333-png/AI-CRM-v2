import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import {
  PushCenterPage,
  PushCenterView,
  invalidatePushCenterRead,
  startPushCenterRead,
  type PushCenterState,
} from "./push-center-ui";
import {
  PUSH_CENTER_SECTION_KEYS,
  type PushCenterSnapshot,
  type PushCenterTransport,
} from "./push-center";

const payload = {
  ok: true,
  sections: PUSH_CENTER_SECTION_KEYS.map((key, index) => ({
    key,
    label: `分区 ${index}`,
    effect_types: ["hidden.effect"],
    capability_key: "hidden_capability",
    count: index,
  })),
  status_definitions: [
    "pending",
    "running",
    "succeeded",
    "sent",
    "simulated",
    "unknown_after_dispatch",
    "failed",
    "sent_with_shadow_warning",
    "shadow_failed_not_business_failed",
  ].map((key) => ({ key, label: key, definition: "hidden definition" })),
  filters: {},
  route_owner: "ai_crm_next",
};
const snapshot: PushCenterSnapshot = {
  sections: PUSH_CENTER_SECTION_KEYS.map((key, index) => ({
    key,
    label: `分区 ${index}`,
    count: index,
  })),
};
const ready: PushCenterState = { kind: "ready", snapshot };

describe("push center UI boundary", () => {
  it("renders safe section counts only", () => {
    const html = renderToStaticMarkup(
      <PushCenterView state={ready} onLoad={vi.fn()} />,
    );
    expect(html).toContain("分区 0");
    expect(html).toContain("不触发 worker、Provider、外部发送或重试");
    for (const hidden of [
      "effect_types",
      "capability_key",
      "status_definitions",
      "filters",
      "hidden.effect",
      "hidden definition",
    ])
      expect(html).not.toContain(hidden);
  });

  it.each(["ops", "sales"] as const)("denies %s without a request", (role) => {
    const transport = { read: vi.fn() } as PushCenterTransport;
    const html = renderToStaticMarkup(
      <PushCenterPage role={role} transport={transport} />,
    );
    expect(html).toContain("没有推送中心本地总览访问权限");
    expect(transport.read).not.toHaveBeenCalled();
  });

  it("single-flights, preserves verified data on failure, and drops an old finally", async () => {
    let resolveOld: (() => void) | undefined;
    const old = new Promise<{ status: number; data: unknown }>((resolve) => {
      resolveOld = () => resolve({ status: 503, data: {} });
    });
    let resolveCurrent: (() => void) | undefined;
    const current = new Promise<{ status: number; data: unknown }>(
      (resolve) => {
        resolveCurrent = () => resolve({ status: 200, data: payload });
      },
    );
    const read = vi
      .fn()
      .mockReturnValueOnce(old)
      .mockReturnValueOnce(current)
      .mockResolvedValueOnce({ status: 503, data: {} });
    const states: PushCenterState[] = [];
    const controller = {
      generation: { current: 0 },
      inFlight: { current: false },
      verified: { current: undefined as PushCenterSnapshot | undefined },
      onState: (next: PushCenterState) => states.push(next),
    };
    const first = startPushCenterRead(controller, {
      read,
    } as PushCenterTransport);
    await startPushCenterRead(controller, { read } as PushCenterTransport);
    expect(read).toHaveBeenCalledTimes(1);
    invalidatePushCenterRead(controller);
    const second = startPushCenterRead(controller, {
      read,
    } as PushCenterTransport);
    expect(read).toHaveBeenCalledTimes(2);
    resolveOld?.();
    await first;
    expect(controller.inFlight.current).toBe(true);
    expect(states.at(-1)).toMatchObject({ kind: "loading" });
    resolveCurrent?.();
    await second;
    expect(states.at(-1)).toEqual({ kind: "ready", snapshot });
    await startPushCenterRead(controller, { read } as PushCenterTransport);
    expect(states.at(-1)).toMatchObject({
      kind: "error",
      previous: snapshot,
    });
  });

  it("calls the 401 callback once", async () => {
    const onUnauthenticated = vi.fn();
    const controller = {
      generation: { current: 0 },
      inFlight: { current: false },
      verified: { current: undefined as PushCenterSnapshot | undefined },
      onState: vi.fn(),
      onUnauthenticated,
    };
    await startPushCenterRead(controller, {
      read: vi.fn(async () => ({ status: 401, data: {} })),
    } as PushCenterTransport);
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    expect(controller.inFlight.current).toBe(false);
  });
});
