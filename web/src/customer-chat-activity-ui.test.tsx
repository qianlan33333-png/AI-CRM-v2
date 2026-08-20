/* eslint-disable no-unused-vars -- compact structural DOM shim for React lifecycle tests. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CustomerChatActivityPanel } from "./customer-chat-activity-ui";
import type { CustomerChatActivityTransportResponse } from "./customer-chat-activity";

class NodeStub {
  parentNode: NodeStub | null = null;
  childNodes: NodeStub[] = [];
  ownerDocument!: DocumentStub;
  constructor(
    readonly nodeType: number,
    readonly nodeName: string,
  ) {}
  appendChild(node: NodeStub): NodeStub {
    node.parentNode = this;
    this.childNodes.push(node);
    return node;
  }
  insertBefore(node: NodeStub, before: NodeStub | null): NodeStub {
    if (!before) return this.appendChild(node);
    node.parentNode = this;
    this.childNodes.splice(this.childNodes.indexOf(before), 0, node);
    return node;
  }
  removeChild(node: NodeStub): NodeStub {
    this.childNodes.splice(this.childNodes.indexOf(node), 1);
    node.parentNode = null;
    return node;
  }
  get firstChild(): NodeStub | null {
    return this.childNodes[0] ?? null;
  }
  get nextSibling(): NodeStub | null {
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
      value === "" ? [] : [new TextStub(value, this.ownerDocument)];
  }
  addEventListener(): void {}
  removeEventListener(): void {}
  contains(node: NodeStub | null): boolean {
    return (
      node === this || this.childNodes.some((child) => child.contains(node))
    );
  }
}

class TextStub extends NodeStub {
  constructor(
    private data: string,
    owner: DocumentStub,
  ) {
    super(3, "#text");
    this.ownerDocument = owner;
  }
  override get textContent(): string {
    return this.data;
  }
  override set textContent(value: string) {
    this.data = value;
  }
}

class ElementStub extends NodeStub {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style: Record<string, string> = {};
  private attributes = new Map<string, string>();
  constructor(tag: string, owner: DocumentStub) {
    super(1, tag.toUpperCase());
    this.tagName = tag.toUpperCase();
    this.ownerDocument = owner;
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
}

class DocumentStub extends NodeStub {
  readonly documentElement: ElementStub;
  readonly body: ElementStub;
  readonly defaultView: Record<string, unknown>;
  activeElement: ElementStub | null;
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
  createElement(tag: string): ElementStub {
    return new ElementStub(tag, this);
  }
  createElementNS(_namespace: string, tag: string): ElementStub {
    return this.createElement(tag);
  }
  createTextNode(value: string): TextStub {
    return new TextStub(value, this);
  }
  createComment(value: string): TextStub {
    return new TextStub(value, this);
  }
}

function mount(): { root: Root; container: ElementStub } {
  const document = new DocumentStub();
  const window = document.defaultView;
  Object.assign(window, {
    Node: NodeStub,
    Element: ElementStub,
    HTMLElement: ElementStub,
    HTMLIFrameElement: ElementStub,
    getSelection: () => null,
  });
  Object.assign(globalThis, {
    document,
    window,
    Node: NodeStub,
    Element: ElementStub,
    HTMLElement: ElementStub,
    IS_REACT_ACT_ENVIRONMENT: true,
  });
  const container = document.createElement("div");
  document.body.appendChild(container);
  return { root: createRoot(container as unknown as Element), container };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function response(
  messageType: string,
  nextCursor: string | null = null,
  count = 1,
) {
  return {
    customer_id: 41,
    chat_type: "all",
    items: Array.from({ length: count }, (_, index) => ({
      chat_type: index % 2 === 0 ? "private" : "group",
      message_type: messageType,
      sent_at: `2026-08-20T12:${String(59 - index).padStart(2, "0")}:00Z`,
    })),
    total: nextCursor ? count + 1 : count,
    next_cursor: nextCursor,
    previous_cursor: null,
    non_atomic_snapshot: true,
    message_content_included: false,
    identity_values_included: false,
    provider_receipts_included: false,
    real_external_call_executed: false,
  };
}

function elements(node: NodeStub, tag: string): ElementStub[] {
  const found: ElementStub[] = [];
  if (node instanceof ElementStub && node.tagName === tag.toUpperCase())
    found.push(node);
  for (const child of node.childNodes) found.push(...elements(child, tag));
  return found;
}

function reactProps<T>(node: ElementStub): T {
  const key = Object.keys(node).find((value) =>
    value.startsWith("__reactProps$"),
  );
  if (!key) throw new Error("react props not found");
  return (node as unknown as Record<string, unknown>)[key] as T;
}

describe("CustomerChatActivityPanel", () => {
  it("renders only safe zero-body labels in static markup", () => {
    const html = renderToStaticMarkup(
      <CustomerChatActivityPanel
        customerID={41}
        role="admin"
        transport={{ get: vi.fn() }}
      />,
    );
    expect(html).toContain("本地聊天活动");
    expect(html).toContain("不读取正文、身份值、媒体");
    expect(html).not.toContain("content_masked");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("href=");
  });

  it("releases pending A for replacement B and ignores stale completion", async () => {
    const first = deferred<CustomerChatActivityTransportResponse>();
    const second = deferred<CustomerChatActivityTransportResponse>();
    const getA = vi.fn(() => first.promise);
    const getB = vi.fn(() => second.promise);
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <CustomerChatActivityPanel
          customerID={41}
          role="admin"
          transport={{ get: getA }}
        />,
      );
    });
    expect(getA).toHaveBeenCalledTimes(1);
    await act(async () => {
      root.render(
        <CustomerChatActivityPanel
          customerID={41}
          role="admin"
          transport={{ get: getB }}
        />,
      );
    });
    expect(getB).toHaveBeenCalledTimes(1);
    await act(async () => {
      first.resolve({ status: 200, data: response("stale-type") });
      await first.promise;
    });
    expect(container.textContent).not.toContain("stale-type");
    await act(async () => {
      second.resolve({ status: 200, data: response("active-type") });
      await second.promise;
    });
    expect(container.textContent).toContain("active-type");
    await act(async () => root.unmount());
  });

  it("locks same-tick pagination and preserves the verified page on failure", async () => {
    const next = deferred<CustomerChatActivityTransportResponse>();
    const get = vi
      .fn()
      .mockResolvedValueOnce({
        status: 200,
        data: response("text", "next", 50),
      })
      .mockImplementationOnce(() => next.promise);
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <CustomerChatActivityPanel
          customerID={41}
          role="ops"
          transport={{ get }}
        />,
      );
    });
    const button = elements(container, "button").find(
      (value) => value.textContent === "下一页",
    );
    if (!button) throw new Error("next button missing");
    const props = reactProps<{ onClick?: () => void }>(button);
    await act(async () => {
      props.onClick?.();
      props.onClick?.();
    });
    expect(get).toHaveBeenCalledTimes(2);
    await act(async () => {
      next.resolve({ status: 503, data: {} });
      await next.promise;
    });
    expect(container.textContent).toContain("text");
    expect(container.textContent).toContain("已验证页面保持不变");
    await act(async () => root.unmount());
  });

  it("drops a pending unmounted 401 and calls active 401 once", async () => {
    const pending = deferred<CustomerChatActivityTransportResponse>();
    const staleAuth = vi.fn();
    const first = mount();
    await act(async () => {
      first.root.render(
        <CustomerChatActivityPanel
          customerID={41}
          role="sales"
          transport={{ get: vi.fn(() => pending.promise) }}
          onUnauthenticated={staleAuth}
        />,
      );
    });
    await act(async () => first.root.unmount());
    await act(async () => {
      pending.resolve({ status: 401, data: {} });
      await pending.promise;
    });
    expect(staleAuth).not.toHaveBeenCalled();

    const activeAuth = vi.fn();
    const active = mount();
    await act(async () => {
      active.root.render(
        <CustomerChatActivityPanel
          customerID={41}
          role="sales"
          transport={{ get: vi.fn(async () => ({ status: 401, data: {} })) }}
          onUnauthenticated={activeAuth}
        />,
      );
    });
    expect(activeAuth).toHaveBeenCalledTimes(1);
    await act(async () => active.root.unmount());
  });
});
