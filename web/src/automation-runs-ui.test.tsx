import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  AutomationRunsPage,
  AutomationDiagnosticsPanel,
  AutomationInternalEventsPanel,
  AutomationSourceEventPanel,
  loadAutomationDiagnosticsState,
  loadAutomationInternalEventsState,
  loadAutomationSourceEventState,
  RunRows,
  type AutomationSourceEventState,
} from "./automation-runs-ui";
import type {
  AutomationDiagnostics,
  AutomationRunsPage as AutomationRunsPageData,
  AutomationRunsTransport,
} from "./automation-runs";

const diagnostics = {
  ok: true,
  filters: { event_type: "", consumer: "", status: "" },
  event_count: 2,
  undispatched_event_count: 1,
  delivery_counts: { pending: 1, processing: 2, completed: 3, final_failed: 4, outcome_unknown: 5 },
  consumer_registry: [
    { consumer: "automation.tag-trigger.v1", event_types: ["customer.tag_applied"] },
    { consumer: "stats.tag-applied.v1", event_types: ["customer.tag_applied"] },
    { consumer: "operation-cycle.fact.v1", event_types: ["operation_cycle.fact_recorded"] },
    { consumer: "cloud-campaign.fact.v1", event_types: ["cloud_campaign.fact_recorded"] },
    { consumer: "outbound-campaign-handoff.fact.v1", event_types: ["outbound.campaign_handoff_fact_recorded"] },
  ],
  observed_at: "2026-08-19T08:00:02Z",
  registry_id: "v2-internal-events.v1",
  source_status: "local_read_model",
  observed_domains: ["event_log", "event_deliveries"],
  unobserved_domains: ["river_queue", "outbound_provider", "external_delivery"],
  external_delivery: "unknown",
  route_owner: "ai_crm_next",
  real_external_call_executed: false,
};

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

const internalEvents = {
  ok: true,
  items: [{
    event_id: 52,
    event_type: "customer.tag_applied",
    occurred_at: "2026-08-19T08:01:00Z",
    dispatched: true,
    deliveries: [{
      consumer: "automation.tag-trigger.v1",
      status: "completed",
      attempt_count: 1,
      completed_at: "2026-08-19T08:01:01Z",
    }],
  }],
  total: 1,
  limit: 50,
  offset: 0,
  observed_at: "2026-08-19T08:02:00Z",
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
    diagnostics: vi.fn(async () => ({ status: 503, data: {} })),
    internalEvents: vi.fn(async () => ({ status: 503, data: {} })),
  } as AutomationRunsTransport;
}

function diagnosticsReader(client: AutomationRunsTransport) {
  if (!client.diagnostics) throw new Error("diagnostics reader required");
  return vi.mocked(client.diagnostics);
}

function diagnosticsController(
  client: AutomationRunsTransport,
  onUnauthenticated?: () => void,
) {
  const state = { current: { kind: "loading" } as const } as { current: import("./automation-runs-ui").AutomationDiagnosticsState };
  const generation = { current: 0 };
  const inFlight = { current: undefined as symbol | undefined };
  const onState = vi.fn((next: import("./automation-runs-ui").AutomationDiagnosticsState) => {
    state.current = next;
  });
  return {
    generation,
    inFlight,
    load: () => loadAutomationDiagnosticsState({ generation, inFlight, onState, onUnauthenticated, state, transport: client }),
    onState,
    state,
  };
}

function internalEventsReader(client: AutomationRunsTransport) {
  if (!client.internalEvents) throw new Error("internal event reader required");
  return vi.mocked(client.internalEvents);
}

function internalEventsController(
  client: AutomationRunsTransport,
  onUnauthenticated?: () => void,
) {
  const state = { current: { kind: "loading" } as const } as { current: import("./automation-runs-ui").AutomationInternalEventsState };
  const generation = { current: 0 };
  const inFlight = { current: undefined as symbol | undefined };
  const onState = vi.fn((next: import("./automation-runs-ui").AutomationInternalEventsState) => {
    state.current = next;
  });
  return {
    generation,
    inFlight,
    load: (offset: number) => loadAutomationInternalEventsState({ generation, inFlight, onState, onUnauthenticated, state, transport: client }, offset),
    onState,
    state,
  };
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
      expect(client.diagnostics).not.toHaveBeenCalled();
      expect(client.internalEvents).not.toHaveBeenCalled();
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

  it("renders only local diagnostic counts and never claims external delivery", () => {
    const parsed: AutomationDiagnostics = {
      eventCount: 2, undispatchedEventCount: 1,
      deliveryCounts: { pending: 1, processing: 2, completed: 3, final_failed: 4, outcome_unknown: 5 },
      consumerRegistry: [
        { consumer: "automation.tag-trigger.v1", eventType: "customer.tag_applied" },
        { consumer: "stats.tag-applied.v1", eventType: "customer.tag_applied" },
        { consumer: "operation-cycle.fact.v1", eventType: "operation_cycle.fact_recorded" },
        { consumer: "cloud-campaign.fact.v1", eventType: "cloud_campaign.fact_recorded" },
        { consumer: "outbound-campaign-handoff.fact.v1", eventType: "outbound.campaign_handoff_fact_recorded" },
      ],
      observedAt: "2026-08-19T08:00:02Z",
      observedDomains: ["event_log", "event_deliveries"],
      unobservedDomains: ["river_queue", "outbound_provider", "external_delivery"],
    };
    const html = renderToStaticMarkup(<AutomationDiagnosticsPanel state={{ kind: "ready", diagnostics: parsed }} />);
    expect(html).toContain("内部事件诊断摘要");
    expect(html).toContain("内部处理完成");
    expect(html).toContain("外部投递状态为 unknown");
    expect(html).toContain("未执行真实外部调用");
    expect(html).not.toMatch(/provider 已执行|外部投递成功|已送达/i);
  });

  it("renders local internal-event observations without external delivery claims", () => {
    const html = renderToStaticMarkup(
      <AutomationInternalEventsPanel
        state={{
          kind: "ready",
          page: {
            items: [{
              eventID: 52,
              eventType: "customer.tag_applied",
              occurredAt: "2026-08-19T08:01:00Z",
              dispatched: true,
              deliveries: [{
                consumer: "automation.tag-trigger.v1",
                status: "completed",
                attemptCount: 1,
                completedAt: "2026-08-19T08:01:01Z",
              }],
            }],
            total: 1,
            limit: 50,
            offset: 0,
            observedAt: "2026-08-19T08:02:00Z",
          },
        }}
        onLoad={vi.fn()}
      />,
    );
    expect(html).toContain("内部事件列表");
    expect(html).toContain("内部派发标记");
    expect(html).toContain("本地处理");
    expect(html).toContain("外部投递状态为 unknown");
    expect(html).not.toMatch(/provider 已执行|外部投递成功|已送达/i);
  });

  it("single-flights internal-event reads, preserves a verified page, and drops stale results", async () => {
    let release: (() => void) | undefined;
    const client = transport();
    internalEventsReader(client).mockImplementationOnce(
      () => new Promise((resolve) => { release = () => resolve({ status: 200, data: internalEvents }); }),
    );
    const controller = internalEventsController(client);
    const first = controller.load(0);
    expect(controller.load(50)).toBeUndefined();
    expect(internalEventsReader(client)).toHaveBeenCalledWith(
      { limit: "50", offset: "0" },
      { credentials: "same-origin" },
    );
    release?.();
    await first;
    expect(controller.state.current).toMatchObject({ kind: "ready", page: { total: 1 } });

    internalEventsReader(client).mockResolvedValueOnce({ status: 503, data: {} });
    await controller.load(0);
    expect(controller.state.current).toMatchObject({
      kind: "error", failure: "unavailable", previous: { total: 1 },
    });

    internalEventsReader(client).mockImplementationOnce(
      () => new Promise((resolve) => { release = () => resolve({ status: 200, data: internalEvents }); }),
    );
    const stale = controller.load(0);
    const statesBeforeUnmount = controller.onState.mock.calls.length;
    controller.generation.current += 1;
    release?.();
    await stale;
    expect(controller.onState).toHaveBeenCalledTimes(statesBeforeUnmount);
  });

  it("notifies an expired session after the active internal-event read", async () => {
    const client = transport();
    internalEventsReader(client).mockResolvedValue({ status: 401, data: {} });
    const onUnauthenticated = vi.fn();
    const controller = internalEventsController(client, onUnauthenticated);
    await controller.load(0);
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(controller.state.current).toMatchObject({ kind: "error", failure: "unauthenticated" });
  });

  it("allows a replacement internal-event effect without an old request unlocking it", async () => {
    // eslint-disable-next-line no-unused-vars -- the Promise resolver receives the synthetic transport response.
    const resolvers: ((value: { status: number; data: unknown }) => void)[] = [];
    const client = transport();
    internalEventsReader(client).mockImplementation(
      () => new Promise((resolve) => { resolvers.push(resolve); }),
    );
    const controller = internalEventsController(client);
    const oldRequest = controller.load(0);
    controller.generation.current += 1;
    controller.inFlight.current = undefined;
    const replacement = controller.load(0);
    expect(internalEventsReader(client)).toHaveBeenCalledTimes(2);

    resolvers[0]?.({ status: 200, data: internalEvents });
    await oldRequest;
    expect(controller.inFlight.current).toBeDefined();

    resolvers[1]?.({ status: 200, data: internalEvents });
    await replacement;
    expect(controller.state.current).toMatchObject({ kind: "ready", page: { total: 1 } });
    expect(controller.inFlight.current).toBeUndefined();
  });

  it("single-flights diagnostics, retains a verified summary, and invalidates unmounted results", async () => {
    let release: (() => void) | undefined;
    const client = transport();
    diagnosticsReader(client).mockImplementationOnce(
      () => new Promise((resolve) => { release = () => resolve({ status: 200, data: diagnostics }); }),
    );
    const controller = diagnosticsController(client);
    const first = controller.load();
    expect(controller.load()).toBeUndefined();
    expect(diagnosticsReader(client)).toHaveBeenCalledWith({}, { credentials: "same-origin" });
    release?.();
    await first;
    expect(controller.state.current).toMatchObject({ kind: "ready", diagnostics: { eventCount: 2 } });

    diagnosticsReader(client).mockResolvedValueOnce({ status: 503, data: {} });
    await controller.load();
    expect(controller.state.current).toMatchObject({ kind: "error", failure: "unavailable", previous: { eventCount: 2 } });

    diagnosticsReader(client).mockImplementationOnce(
      () => new Promise((resolve) => { release = () => resolve({ status: 200, data: diagnostics }); }),
    );
    const stale = controller.load();
    const statesBeforeUnmount = controller.onState.mock.calls.length;
    controller.generation.current += 1;
    release?.();
    await stale;
    expect(controller.onState).toHaveBeenCalledTimes(statesBeforeUnmount);
  });

  it("notifies an expired session after the active diagnostics read", async () => {
    const client = transport();
    diagnosticsReader(client).mockResolvedValue({ status: 401, data: {} });
    const onUnauthenticated = vi.fn();
    const controller = diagnosticsController(client, onUnauthenticated);
    await controller.load();
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(controller.state.current).toMatchObject({ kind: "error", failure: "unauthenticated" });
  });

  it("lets a replacement effect read while an old diagnostic request is stale without unlocking it", async () => {
    // eslint-disable-next-line no-unused-vars -- the Promise resolver receives the synthetic transport response.
    const resolvers: ((value: { status: number; data: unknown }) => void)[] = [];
    const client = transport();
    diagnosticsReader(client).mockImplementation(
      () => new Promise((resolve) => { resolvers.push(resolve); }),
    );
    const controller = diagnosticsController(client);
    const oldRequest = controller.load();
    controller.generation.current += 1;
    controller.inFlight.current = undefined;
    const replacement = controller.load();
    expect(diagnosticsReader(client)).toHaveBeenCalledTimes(2);

    resolvers[0]?.({ status: 200, data: diagnostics });
    await oldRequest;
    expect(controller.inFlight.current).toBeDefined();

    resolvers[1]?.({ status: 200, data: diagnostics });
    await replacement;
    expect(controller.state.current).toMatchObject({ kind: "ready", diagnostics: { eventCount: 2 } });
    expect(controller.inFlight.current).toBeUndefined();
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
