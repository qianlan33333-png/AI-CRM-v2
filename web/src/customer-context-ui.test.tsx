/* eslint-disable no-unused-vars -- minimal structural DOM shim for React state-machine tests. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CustomerContextPanel } from "./customer-context-ui";
import type {
  CustomerContextSnapshot,
  CustomerContextTransport,
} from "./customer-context";

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
    return (
      this.parentNode?.childNodes[
        this.parentNode.childNodes.indexOf(this) + 1
      ] ?? null
    );
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
    return (
      node === this || this.childNodes.some((child) => child.contains(node))
    );
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
function mountedRoot(): { root: Root; container: TestElement } {
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
function buttons(root: TestNode): TestElement[] {
  return [root, ...root.childNodes.flatMap(buttons)].filter(
    (node): node is TestElement =>
      node instanceof TestElement && node.tagName === "BUTTON",
  );
}
function reactProps<T extends Record<string, unknown>>(
  element: TestElement,
): T {
  const key = Object.keys(element).find((candidate) =>
    candidate.startsWith("__reactProps"),
  );
  if (!key) throw new Error("missing React props");
  return (element as unknown as Record<string, T>)[key];
}
function click(button: TestElement): void {
  reactProps<{ onClick?: () => void }>(button).onClick?.();
}
function deferred<T>(): { promise: Promise<T>; resolve(value: T): void } {
  let resolve!: (value: T) => void;
  return {
    promise: new Promise<T>((done) => {
      resolve = done;
    }),
    resolve,
  };
}

const snapshot: CustomerContextSnapshot = {
  customer: {
    id: 7,
    name: "林小姐",
    stageID: 3,
    addedAt: "2026-08-12T00:00:00Z",
  },
  tags: [
    {
      id: 9,
      groupName: "意向",
      groupSortOrder: 1,
      name: "已报名",
      sortOrder: 10,
    },
  ],
  timeline: [
    { id: 12, eventType: "stage_changed", occurredAt: "2026-08-12T03:00:00Z" },
  ],
  timelineNextCursor: "opaque-next",
  chat: {
    localArchiveAvailable: true,
    items: [
      {
        chatType: "private",
        messageType: "text",
        sentAt: "2026-08-12T04:00:00Z",
      },
    ],
    total: 1,
  },
};
function response(
  data: unknown,
  status = 200,
): { status: number; data: unknown } {
  return { status, data };
}
function body(value: CustomerContextSnapshot): unknown {
  return {
    customer: {
      id: value.customer.id,
      name: value.customer.name,
      stage_id: value.customer.stageID,
      added_at: value.customer.addedAt,
    },
    tags: value.tags.map((tag) => ({
      id: tag.id,
      group_id: 2,
      group_name: tag.groupName,
      group_sort_order: tag.groupSortOrder,
      name: tag.name,
      sort_order: tag.sortOrder,
    })),
    timeline: value.timeline.map((entry) => ({
      id: entry.id,
      event_type: entry.eventType,
      occurred_at: entry.occurredAt,
    })),
    timeline_next_cursor: value.timelineNextCursor ?? null,
    chat: {
      local_archive_available: value.chat.localArchiveAvailable,
      items: value.chat.items.map((entry) => ({
        chat_type: entry.chatType,
        message_type: entry.messageType,
        sent_at: entry.sentAt,
      })),
      total: value.chat.total,
    },
    non_atomic_snapshot: true,
    real_external_call_executed: false,
  };
}

describe("CustomerContextPanel", () => {
  it("renders only the safe context fields", () => {
    const html = renderToStaticMarkup(
      <CustomerContextPanel
        customerID={7}
        initialSnapshot={snapshot}
        transport={{ get: vi.fn() } as unknown as CustomerContextTransport}
      />,
    );
    expect(html).toContain("Customer 360 本地读取");
    expect(html).toContain("CRM 标签");
    expect(html).toContain("本地聊天摘要");
    expect(html).toContain("stage_changed");
    expect(html).toContain("private / text");
    for (const forbidden of [
      "avatar",
      "gender",
      "actor",
      "payload",
      "provider",
      "message body",
      "问卷",
    ])
      expect(html.toLowerCase()).not.toContain(forbidden);
  });

  it("mounts timeline pagination with same-tick singleflight, failure retention, and unmount stale-drop", async () => {
    const first = deferred<{ status: number; data: unknown }>();
    const unavailable = deferred<{ status: number; data: unknown }>();
    const stale = deferred<{ status: number; data: unknown }>();
    const get = vi.fn(
      () => [first, unavailable, stale][get.mock.calls.length - 1].promise,
    );
    const mounted = mountedRoot();
    const onUnauthenticated = vi.fn();
    await act(async () => {
      mounted.root.render(
        <CustomerContextPanel
          customerID={7}
          transport={{ get: async () => get() }}
          onUnauthenticated={onUnauthenticated}
        />,
      );
    });
    await act(async () => {
      first.resolve(response(body(snapshot)));
      await Promise.resolve();
    });
    const more = buttons(mounted.container).find(
      (button) => button.textContent === "加载更多时间线",
    );
    if (!more) throw new Error("missing more button");
    await act(async () => {
      click(more);
      click(more);
    });
    expect(get).toHaveBeenCalledTimes(2);
    await act(async () => {
      unavailable.resolve(response({}, 503));
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("stage_changed");
    expect(mounted.container.textContent).toContain("已显示的内容保持不变");
    const retry = buttons(mounted.container).find(
      (button) => button.textContent === "加载更多时间线",
    );
    if (!retry) throw new Error("missing retry button");
    await act(async () => {
      click(retry);
      mounted.root.unmount();
      stale.resolve(response({}, 401));
      await Promise.resolve();
    });
    expect(onUnauthenticated).not.toHaveBeenCalled();
  });

  it("calls the active initial 401 callback once", async () => {
    const onUnauthenticated = vi.fn();
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <CustomerContextPanel
          customerID={7}
          transport={{ get: async () => response({}, 401) }}
          onUnauthenticated={onUnauthenticated}
        />,
      );
      await Promise.resolve();
    });
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    await act(async () => {
      mounted.root.unmount();
    });
  });

  it("drops an old customer response after a customerID switch", async () => {
    const oldCustomer = deferred<{ status: number; data: unknown }>();
    const newCustomer = deferred<{ status: number; data: unknown }>();
    const onUnauthenticated = vi.fn();
    const get = vi.fn((customerID: number) =>
      customerID === 7 ? oldCustomer.promise : newCustomer.promise,
    );
    const mounted = mountedRoot();
    const secondSnapshot: CustomerContextSnapshot = {
      ...snapshot,
      customer: { ...snapshot.customer, id: 8, name: "新客户" },
      timeline: [
        {
          id: 13,
          eventType: "customer.created",
          occurredAt: "2026-08-12T05:00:00Z",
        },
      ],
      timelineNextCursor: undefined,
    };
    await act(async () => {
      mounted.root.render(
        <CustomerContextPanel
          customerID={7}
          transport={{ get: async (id) => get(id) }}
          onUnauthenticated={onUnauthenticated}
        />,
      );
    });
    await act(async () => {
      mounted.root.render(
        <CustomerContextPanel
          customerID={8}
          transport={{ get: async (id) => get(id) }}
          onUnauthenticated={onUnauthenticated}
        />,
      );
      newCustomer.resolve(response(body(secondSnapshot)));
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("新客户");
    await act(async () => {
      oldCustomer.resolve(response({}, 401));
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("新客户");
    expect(onUnauthenticated).not.toHaveBeenCalled();
    await act(async () => {
      mounted.root.unmount();
    });
  });
});
