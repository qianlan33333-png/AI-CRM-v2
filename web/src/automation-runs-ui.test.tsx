import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  AutomationRunsPage,
  AutomationSourceEventPanel,
  loadAutomationSourceEventState,
  RunRows,
  type AutomationSourceEventState,
} from "./automation-runs-ui";
import type {
  AutomationRunsPage as AutomationRunsPageData,
  AutomationRunsTransport,
} from "./automation-runs";

const sourceEvent = {
  ok: true,
  item: {
    event_id: 51,
    event_type: "customer.tag_applied",
    occurred_at: "2026-08-19T08:00:00Z",
    dispatched: true,
    deliveries: [
      {
        consumer: "automation.tag-trigger.v1",
        status: "completed",
        attempt_count: 1,
        completed_at: "2026-08-19T08:00:01Z",
      },
    ],
  },
  observed_at: "2026-08-19T08:00:02Z",
  registry_id: "v2-internal-events.v1",
  source_status: "local_read_model",
  delivery_observation_available: true,
  external_delivery: "unknown",
  route_owner: "ai_crm_next",
  real_external_call_executed: false,
};

function transport(): AutomationRunsTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    sourceEvent: vi.fn(async () => ({ status: 503, data: {} })),
  } as AutomationRunsTransport;
}

function sourceEventReader(client: AutomationRunsTransport) {
  if (!client.sourceEvent) throw new Error("source event reader required");
  return vi.mocked(client.sourceEvent);
}

function sourceEventController(
  client: AutomationRunsTransport,
  onUnauthenticated?: () => void,
) {
  const state = { current: { kind: "idle" } as AutomationSourceEventState };
  const generation = { current: 0 };
  const inFlight = { current: false };
  const onState = vi.fn((next: AutomationSourceEventState) => {
    state.current = next;
  });
  return {
    generation,
    inFlight,
    load: (eventID: number) =>
      loadAutomationSourceEventState(
        {
          generation,
          inFlight,
          onState,
          onUnauthenticated,
          state,
          transport: client,
        },
        eventID,
      ),
    onState,
    state,
  };
}

function elementChild(
  element: React.ReactElement<{ children?: React.ReactNode }>,
  index: number,
): React.ReactElement<{ children?: React.ReactNode }> {
  const child = React.Children.toArray(element.props.children)[index];
  if (!React.isValidElement(child)) throw new Error("expected element child");
  return child as React.ReactElement<{ children?: React.ReactNode }>;
}

function sourceEventButtons(
  page: AutomationRunsPageData,
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  onLoadSourceEvent: (eventID: number) => void,
): Array<React.ReactElement<{ onClick: () => void }>> {
  const root = RunRows({
    page,
    sourceEventBusy: false,
    onLoadSourceEvent,
  }) as React.ReactElement<{ children?: React.ReactNode }>;
  const table = elementChild(root, 1);
  const body = elementChild(table, 1);
  return React.Children.toArray(body.props.children).map((row) => {
    if (!React.isValidElement(row)) throw new Error("expected row");
    return elementChild(
      elementChild(
        row as React.ReactElement<{ children?: React.ReactNode }>,
        6,
      ),
      0,
    ) as React.ReactElement<{ onClick: () => void }>;
  });
}

describe("AutomationRunsPage shell", () => {
  it("renders the local masked receipt shell for admin only", () => {
    const html = renderToStaticMarkup(
      <AutomationRunsPage role="admin" transport={transport()} />,
    );
    expect(html).toContain('<h1 id="app-title">自动化运行记录</h1>');
    expect(html).toContain("只读脱敏收据");
    expect(html).toContain("正在读取自动化运行记录。");
    expect(html).toContain(
      "不代表任何企业微信、支付或其他外部效果已经执行或成功",
    );
    expect(html).not.toContain("unionid");
    expect(html).not.toContain("userid");
  });

  it.each(["ops", "sales"] as const)(
    "keeps %s fail-closed without issuing a read",
    (role) => {
      const client = transport();
      const html = renderToStaticMarkup(
        <AutomationRunsPage role={role} transport={client} />,
      );
      expect(html).toContain("当前账号没有自动化运行记录访问权限。");
      expect(html).not.toContain("正在读取自动化运行记录。");
      expect(client.list).not.toHaveBeenCalled();
      expect(client.sourceEvent).not.toHaveBeenCalled();
    },
  );

  it("passes each row's source event ID, never its tag ID, to the actual button click", () => {
    const page: AutomationRunsPageData = {
      page: 1,
      pageSize: 50,
      total: 2,
      items: [
        {
          runID: "automation-trigger:11",
          requestID: "event:21",
          agentCode: "tag-trigger-v1",
          runStatus: "completed",
          triggerSource: "customer.tag_applied",
          customerID: 31,
          tagID: 41,
          sourceEventID: 51,
          triggeredEventID: 61,
          startedAt: "2026-08-19T08:00:00Z",
          completedAt: "2026-08-19T08:00:01Z",
          hasError: false,
        },
        {
          runID: "automation-trigger:12",
          requestID: "event:22",
          agentCode: "tag-trigger-v1",
          runStatus: "completed",
          triggerSource: "customer.tag_applied",
          customerID: 32,
          tagID: 42,
          sourceEventID: 52,
          triggeredEventID: 62,
          startedAt: "2026-08-19T08:01:00Z",
          completedAt: "2026-08-19T08:01:01Z",
          hasError: false,
        },
      ],
    };
    const onLoadSourceEvent = vi.fn();
    const buttons = sourceEventButtons(page, onLoadSourceEvent);

    buttons[0]?.props.onClick();
    buttons[1]?.props.onClick();
    expect(onLoadSourceEvent).toHaveBeenNthCalledWith(1, 51);
    expect(onLoadSourceEvent).toHaveBeenNthCalledWith(2, 52);
    expect(onLoadSourceEvent).not.toHaveBeenCalledWith(41);
    expect(onLoadSourceEvent).not.toHaveBeenCalledWith(42);
  });

  it("renders only local source-event facts and keeps external delivery unknown", () => {
    const html = renderToStaticMarkup(
      <AutomationSourceEventPanel
        state={{
          kind: "ready",
          sourceEvent: {
            eventID: 51,
            eventType: "customer.tag_applied",
            occurredAt: "2026-08-19T08:00:00Z",
            dispatched: true,
            observedAt: "2026-08-19T08:00:02Z",
            deliveries: [
              {
                consumer: "automation.tag-trigger.v1",
                status: "completed",
                attemptCount: 1,
                completedAt: "2026-08-19T08:00:01Z",
              },
            ],
          },
        }}
      />,
    );
    expect(html).toContain("源内部事件");
    expect(html).toContain("外部投递为 unknown");
    expect(html).toContain("automation.tag-trigger.v1");
    expect(html).not.toMatch(
      /external_delivery|real_external_call_executed|unionid|external_userid|mobile/i,
    );
  });

  it("runs the page source-event controller with the row event ID only once per tick", async () => {
    let release: (() => void) | undefined;
    const client = transport();
    sourceEventReader(client).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = () => resolve({ status: 200, data: sourceEvent });
        }),
    );
    const controller = sourceEventController(client);

    const first = controller.load(51);
    const duplicate = controller.load(61);
    expect(first).toBeDefined();
    expect(duplicate).toBeUndefined();
    expect(client.sourceEvent).toHaveBeenCalledOnce();
    expect(client.sourceEvent).toHaveBeenCalledWith(51, {
      credentials: "same-origin",
    });
    expect(controller.state.current).toMatchObject({
      kind: "loading",
      eventID: 51,
    });

    release?.();
    await first;
    expect(controller.state.current).toMatchObject({
      kind: "ready",
      sourceEvent: { eventID: 51 },
    });
    expect(controller.inFlight.current).toBe(false);
  });

  it("discards an unmounted generation and retains the verified same-ID detail on failure", async () => {
    const client = transport();
    sourceEventReader(client)
      .mockResolvedValueOnce({ status: 200, data: sourceEvent })
      .mockResolvedValueOnce({ status: 503, data: {} })
      .mockResolvedValueOnce({ status: 200, data: {} });
    const controller = sourceEventController(client);

    await controller.load(51);
    await controller.load(51);
    expect(controller.state.current).toMatchObject({
      kind: "error",
      failure: "unavailable",
      eventID: 51,
      previous: { eventID: 51 },
    });
    await controller.load(51);
    expect(controller.state.current).toMatchObject({
      kind: "error",
      failure: "invalid",
      eventID: 51,
      previous: { eventID: 51 },
    });

    let release: (() => void) | undefined;
    sourceEventReader(client).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = () => resolve({ status: 200, data: sourceEvent });
        }),
    );
    const pending = controller.load(51);
    const statesBeforeUnmount = controller.onState.mock.calls.length;
    controller.generation.current += 1;
    release?.();
    await pending;
    expect(controller.onState).toHaveBeenCalledTimes(statesBeforeUnmount);
  });

  it("notifies an expired session once after the active source-event request", async () => {
    const client = transport();
    sourceEventReader(client).mockResolvedValue({ status: 401, data: {} });
    const onUnauthenticated = vi.fn();
    const controller = sourceEventController(client, onUnauthenticated);

    await controller.load(51);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(controller.state.current).toMatchObject({
      kind: "error",
      failure: "unauthenticated",
      eventID: 51,
    });
  });
});
