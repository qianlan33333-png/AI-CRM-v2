/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import {
  loadCallbackAudit,
  loadCallbackAuditDetail,
  nextCallbackAuditOffset,
  parseCallbackAuditPage,
  previousCallbackAuditOffset,
  type CallbackInboxTransport,
} from "./wecom-callback-inbox";
import {
  invalidateCallbackAuditList,
  requestCallbackAudit,
  requestCallbackAuditDetail,
  type CallbackAuditDetailState,
  type CallbackAuditListFlight,
  type CallbackAuditPageState,
  WeComCallbackInboxPage,
} from "./wecom-callback-inbox-ui";

class TestNode {
  parentNode: TestNode | null = null;
  childNodes: TestNode[] = [];
  ownerDocument!: TestDocument;
  constructor(
    readonly nodeType: number,
    readonly nodeName: string,
  ) {}
  appendChild(node: TestNode): TestNode {
    node.parentNode = this;
    this.childNodes.push(node);
    return node;
  }
  insertBefore(node: TestNode, before: TestNode | null): TestNode {
    if (before === null) return this.appendChild(node);
    node.parentNode = this;
    this.childNodes.splice(this.childNodes.indexOf(before), 0, node);
    return node;
  }
  removeChild(node: TestNode): TestNode {
    this.childNodes.splice(this.childNodes.indexOf(node), 1);
    node.parentNode = null;
    return node;
  }
  get firstChild(): TestNode | null {
    return this.childNodes[0] ?? null;
  }
  get nextSibling(): TestNode | null {
    if (!this.parentNode) return null;
    return this.parentNode.childNodes[
      this.parentNode.childNodes.indexOf(this) + 1
    ] ?? null;
  }
  get textContent(): string {
    return this.childNodes.map((node) => node.textContent).join("");
  }
  set textContent(value: string) {
    this.childNodes =
      value === "" ? [] : [new TestText(value, this.ownerDocument)];
  }
  addEventListener(): void {}
  removeEventListener(): void {}
  contains(node: TestNode | null): boolean {
    return node === this || this.childNodes.some((child) => child.contains(node));
  }
}

class TestText extends TestNode {
  constructor(
    private data: string,
    ownerDocument: TestDocument,
  ) {
    super(3, "#text");
    this.ownerDocument = ownerDocument;
  }
  override get textContent(): string {
    return this.data;
  }
  override set textContent(value: string) {
    this.data = value;
  }
}

class TestElement extends TestNode {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style: Record<string, string> = {};
  private readonly attributes = new Map<string, string>();
  constructor(tagName: string, ownerDocument: TestDocument) {
    super(1, tagName.toUpperCase());
    this.tagName = tagName.toUpperCase();
    this.ownerDocument = ownerDocument;
  }
  get options(): TestElement[] {
    return this.childNodes.filter(
      (node): node is TestElement =>
        node instanceof TestElement && node.tagName === "OPTION",
    );
  }
  setAttribute(name: string, value: string): void {
    this.attributes.set(name, value);
  }
  removeAttribute(name: string): void {
    this.attributes.delete(name);
  }
  getAttribute(name: string): string | null {
    return this.attributes.get(name) ?? null;
  }
  hasAttribute(name: string): boolean {
    return this.attributes.has(name);
  }
}

class TestDocument extends TestNode {
  readonly nodeType = 9;
  readonly documentElement: TestElement;
  readonly body: TestElement;
  readonly defaultView: Record<string, unknown>;
  activeElement: TestElement | null;
  constructor() {
    super(9, "#document");
    this.ownerDocument = this;
    this.documentElement = this.createElement("html");
    this.body = this.createElement("body");
    this.documentElement.appendChild(this.body);
    this.appendChild(this.documentElement);
    this.activeElement = this.body;
    this.defaultView = { document: this, navigator: { userAgent: "node" } };
  }
  createElement(tagName: string): TestElement {
    return new TestElement(tagName, this);
  }
  createElementNS(_namespace: string, tagName: string): TestElement {
    return this.createElement(tagName);
  }
  createTextNode(value: string): TestText {
    return new TestText(value, this);
  }
  createComment(value: string): TestText {
    return new TestText(value, this);
  }
}

function mountedRoot(): { readonly root: Root; readonly container: TestElement } {
  const document = new TestDocument();
  const window = document.defaultView as Record<string, unknown>;
  Object.assign(window, {
    Node: TestNode,
    Element: TestElement,
    HTMLElement: TestElement,
    HTMLIFrameElement: TestElement,
    getSelection: () => null,
  });
  Object.assign(globalThis, {
    document,
    window,
    Node: TestNode,
    Element: TestElement,
    HTMLElement: TestElement,
    HTMLIFrameElement: TestElement,
    IS_REACT_ACT_ENVIRONMENT: true,
  });
  const container = document.createElement("div");
  document.body.appendChild(container);
  return { root: createRoot(container as unknown as Element), container };
}

function findButton(root: TestNode, text: string): () => void {
  const candidate = [root, ...root.childNodes.flatMap((child) => [child])]
    .flatMap(function visit(node): TestNode[] {
      return [node, ...node.childNodes.flatMap(visit)];
    })
    .find(
      (node): node is TestElement =>
        node instanceof TestElement &&
        node.tagName === "BUTTON" &&
        node.textContent === text,
    );
  if (!candidate) throw new Error(`missing ${text} button`);
  const key = Object.keys(candidate).find((value) =>
    value.startsWith("__reactProps"),
  );
  if (!key) throw new Error("mounted button is missing React props");
  const props = (candidate as unknown as Record<string, { onClick?: () => void }>)[
    key
  ];
  if (!props?.onClick) throw new Error("mounted button is missing click handler");
  return props.onClick;
}

const acceptedItem = {
  event_id: 7,
  event_type: "wecom.callback.accepted",
  occurred_at: "2026-08-20T08:00:00Z",
  dispatched: false,
  deliveries: [
    {
      consumer: "automation.tag-trigger.v1",
      status: "completed",
      attempt_count: 1,
      completed_at: "2026-08-20T08:00:01Z",
    },
  ],
};

function page(
  items: readonly unknown[] = [acceptedItem],
  offset = 0,
  total = items.length,
) {
  return {
    ok: true,
    items,
    total,
    limit: 50,
    offset,
    observed_at: "2026-08-20T08:00:02Z",
    registry_id: "v2-internal-events.v1",
    source_status: "local_read_model",
    delivery_observation_available: true,
    external_delivery: "unknown",
    route_owner: "ai_crm_next",
    real_external_call_executed: false,
  };
}

function detail(item: unknown = acceptedItem) {
  return {
    ok: true,
    item,
    observed_at: "2026-08-20T08:00:02Z",
    registry_id: "v2-internal-events.v1",
    source_status: "local_read_model",
    delivery_observation_available: true,
    external_delivery: "unknown",
    route_owner: "ai_crm_next",
    real_external_call_executed: false,
  };
}

function transport(
  overrides: Partial<CallbackInboxTransport> = {},
): CallbackInboxTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    detail: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  } as CallbackInboxTransport;
}

describe("WeCom callback local-audit boundary", () => {
  it("projects only de-identified callback facts while validating the complete local delivery DTO", () => {
    expect(parseCallbackAuditPage(page(), "accepted", 0)).toEqual({
      items: [
        {
          eventID: 7,
          disposition: "accepted",
          occurredAt: "2026-08-20T08:00:00Z",
          dispatched: false,
        },
      ],
      total: 1,
      offset: 0,
      observedAt: "2026-08-20T08:00:02Z",
    });
  });

  it.each([
    { ...acceptedItem, event_type: "wecom.callback.rejected" },
    {
      ...acceptedItem,
      deliveries: [
        { ...acceptedItem.deliveries[0], consumer: "provider.receipt" },
      ],
    },
    {
      ...acceptedItem,
      deliveries: [{ ...acceptedItem.deliveries[0], payload: "secret" }],
    },
    { ...acceptedItem, event_id: 0 },
    { ...page(), external_delivery: "received" },
    { ...page(), unexpected: true },
  ])(
    "fails closed for an expanded, cross-kind, or non-local fact %#",
    (candidate) => {
      const value = Object.hasOwn(candidate, "ok")
        ? candidate
        : page([candidate]);
      expect(parseCallbackAuditPage(value, "accepted", 0)).toBeUndefined();
    },
  );

  it("uses one fixed same-origin GET for each read and never sends callback data", async () => {
    const client = transport({
      list: vi.fn(async () => ({ status: 200, data: page() })),
      detail: vi.fn(async () => ({ status: 200, data: detail() })),
    });
    await expect(loadCallbackAudit(client, "accepted")).resolves.toMatchObject({
      status: "loaded",
    });
    await expect(
      loadCallbackAuditDetail(client, "accepted", 7),
    ).resolves.toMatchObject({
      status: "loaded",
      item: { eventID: 7 },
    });
    expect(client.list).toHaveBeenCalledWith(
      { event_type: "wecom.callback.accepted", limit: "50", offset: "0" },
      { credentials: "same-origin" },
    );
    expect(client.detail).toHaveBeenCalledWith("7", {
      credentials: "same-origin",
    });
  });

  it("keeps the strict page boundary and calculates local paging only from verified totals", () => {
    const first = parseCallbackAuditPage(
      page(
        Array.from({ length: 50 }, (_, index) => ({
          ...acceptedItem,
          event_id: index + 1,
        })),
        0,
        51,
      ),
      "accepted",
      0,
    );
    if (!first) throw new Error("fixture must parse");
    expect(nextCallbackAuditOffset(first)).toBe(50);
    expect(previousCallbackAuditOffset(first)).toBeUndefined();
    expect(
      parseCallbackAuditPage(page([], 0, 1), "accepted", 0),
    ).toBeUndefined();
  });
});

describe("WeCom callback audit controllers", () => {
  it("single-flights same-tick reads, discards an old response, and preserves a verified page on failure", async () => {
    let resolveFirst:
      ((...args: [{ status: number; data: unknown }]) => void) | undefined;
    let resolveSecond:
      ((...args: [{ status: number; data: unknown }]) => void) | undefined;
    const list = vi.fn(
      () =>
        new Promise<{ status: number; data: unknown }>((resolve) => {
          if (!resolveFirst) resolveFirst = resolve;
          else resolveSecond = resolve;
        }),
    ) as CallbackInboxTransport["list"];
    const client = transport({
      list,
    });
    const token = { current: 0 };
    const flight = {
      current: undefined as CallbackAuditListFlight | undefined,
    };
    const verified = {
      current: undefined as ReturnType<typeof parseCallbackAuditPage>,
    };
    const states: CallbackAuditPageState[] = [];
    const controller = {
      transport: client,
      token,
      flight,
      verified,
      unauthenticatedNotified: { current: false },
      setState: (state: CallbackAuditPageState) => states.push(state),
    };
    const first = requestCallbackAudit(controller, "accepted", 0);
    expect(requestCallbackAudit(controller, "accepted", 0)).toBeUndefined();
    const second = requestCallbackAudit(controller, "rejected", 0);
    resolveSecond?.({
      status: 200,
      data: page([{ ...acceptedItem, event_type: "wecom.callback.rejected" }]),
    });
    await second;
    resolveFirst?.({ status: 503, data: {} });
    await first;
    expect(states.at(-1)).toMatchObject({
      kind: "ready",
      page: { items: [{ disposition: "rejected" }] },
    });
    expect(flight.current).toBeUndefined();

    const unavailable = transport({
      list: vi.fn(async () => ({ status: 503, data: {} })),
    });
    await requestCallbackAudit(
      { ...controller, transport: unavailable },
      "rejected",
      0,
    );
    expect(states.at(-1)).toMatchObject({
      kind: "error",
      previous: { items: [{ disposition: "rejected" }] },
    });
  });

  it("notifies 401 once and keeps an earlier safe detail after a same-kind failure", async () => {
    const notified = vi.fn();
    const token = { current: 0 };
    const states: CallbackAuditDetailState[] = [];
    const client = transport({
      detail: vi.fn(async () => ({ status: 401, data: {} })),
    });
    await requestCallbackAuditDetail(
      {
        transport: client,
        onUnauthenticated: notified,
        token,
        flight: { current: undefined },
        verified: { current: undefined },
        unauthenticatedNotified: { current: false },
        setState: (state) => states.push(state),
      },
      "accepted",
      7,
    );
    expect(notified).toHaveBeenCalledOnce();
    expect(states.at(-1)).toEqual({
      loading: false,
      failure: "unauthenticated",
    });
  });

  it("releases the effect-owned list request so a replacement transport can load before the old request settles", async () => {
    let resolveOld:
      | ((value: { status: number; data: unknown }) => void)
      | undefined;
    const oldTransport = transport({
      list: vi.fn(
        () =>
          new Promise<{ status: number; data: unknown }>((resolve) => {
            resolveOld = resolve;
          }),
      ),
    });
    const replacementTransport = transport({
      list: vi.fn(async () => ({ status: 200, data: page() })),
    });
    const token = { current: 0 };
    const flight = {
      current: undefined as CallbackAuditListFlight | undefined,
    };
    const verified = {
      current: undefined as ReturnType<typeof parseCallbackAuditPage>,
    };
    const states: CallbackAuditPageState[] = [];
    const first = requestCallbackAudit(
      {
        transport: oldTransport,
        token,
        flight,
        verified,
        unauthenticatedNotified: { current: false },
        setState: (state) => states.push(state),
      },
      "accepted",
      0,
    );
    invalidateCallbackAuditList(token, flight);
    const replacement = requestCallbackAudit(
      {
        transport: replacementTransport,
        token,
        flight,
        verified,
        unauthenticatedNotified: { current: false },
        setState: (state) => states.push(state),
      },
      "accepted",
      0,
    );
    expect(replacementTransport.list).toHaveBeenCalledOnce();
    await replacement;
    resolveOld?.({ status: 503, data: {} });
    await first;
    expect(states.at(-1)).toMatchObject({
      kind: "ready",
      page: { items: [{ eventID: 7 }] },
    });
    expect(flight.current).toBeUndefined();
  });

  it("mounts A to B list replacement, notifies active 401 once, and preserves the prior page", async () => {
    let resolveOld:
      | ((value: { status: number; data: unknown }) => void)
      | undefined;
    const oldTransport = transport({
      list: vi.fn(
        () =>
          new Promise<{ status: number; data: unknown }>((resolve) => {
            resolveOld = resolve;
          }),
      ),
    });
    const authenticated = vi.fn();
    const replacementTransport = transport({
      list: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: page() })
        .mockResolvedValue({ status: 401, data: {} }),
    });
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        React.createElement(WeComCallbackInboxPage, {
          role: "admin",
          transport: oldTransport,
          onUnauthenticated: authenticated,
        }),
      );
    });
    await act(async () => {
      mounted.root.render(
        React.createElement(WeComCallbackInboxPage, {
          role: "admin",
          transport: replacementTransport,
          onUnauthenticated: authenticated,
        }),
      );
      await Promise.resolve();
    });
    expect(replacementTransport.list).toHaveBeenCalledOnce();
    expect(mounted.container.textContent).toContain("本地事件 ID");
    resolveOld?.({ status: 503, data: {} });
    await act(async () => {
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("本地事件 ID");
    const refresh = findButton(mounted.container, "刷新");
    await act(async () => {
      refresh();
      await Promise.resolve();
    });
    expect(authenticated).toHaveBeenCalledOnce();
    expect(mounted.container.textContent).toContain("本地事件 ID");
    await act(async () => {
      refresh();
      await Promise.resolve();
      mounted.root.unmount();
    });
    expect(authenticated).toHaveBeenCalledOnce();
  });
});
