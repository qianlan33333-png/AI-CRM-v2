/* eslint-disable no-unused-vars -- compact structural DOM shim for React lifecycle tests. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CustomerMergeHistoryPanel } from "./customer-merge-history-ui";
import type { CustomerMergeHistoryTransportResponse } from "./customer-merge-history";

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

function page(id: number) {
  return {
    customer_id: 41,
    scope: "connected_component",
    items: [
      {
        merge_audit_id: id,
        primary_customer_id: 41,
        merged_customer_id: 100 + id,
        mode: "manual",
        policy_version: `policy-${id}`,
        merged_at: "2026-08-20T12:00:00Z",
      },
    ],
    next_cursor: null,
    identity_values_included: false,
    operator_identifiers_included: false,
    chat_content_included: false,
    real_external_call_executed: false,
  };
}

describe("CustomerMergeHistoryPanel", () => {
  it("renders no unsafe controls or transport for sales", () => {
    const get = vi.fn();
    const html = renderToStaticMarkup(
      <CustomerMergeHistoryPanel
        customerID={41}
        role="sales"
        transport={{ get }}
      />,
    );
    expect(html).toBe("");
    expect(get).not.toHaveBeenCalled();
  });

  it("releases old token on transport replacement and ignores stale completion", async () => {
    const first = deferred<CustomerMergeHistoryTransportResponse>();
    const second = deferred<CustomerMergeHistoryTransportResponse>();
    const getA = vi.fn(() => first.promise);
    const getB = vi.fn(() => second.promise);
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <CustomerMergeHistoryPanel
          customerID={41}
          role="admin"
          transport={{ get: getA }}
        />,
      );
    });
    expect(getA).toHaveBeenCalledTimes(1);
    await act(async () => {
      root.render(
        <CustomerMergeHistoryPanel
          customerID={41}
          role="admin"
          transport={{ get: getB }}
        />,
      );
    });
    expect(getB).toHaveBeenCalledTimes(1);
    await act(async () => {
      first.resolve({ status: 200, data: page(9) });
      await first.promise;
    });
    expect(container.textContent).not.toContain("policy-9");
    await act(async () => {
      second.resolve({ status: 200, data: page(8) });
      await second.promise;
    });
    expect(container.textContent).toContain("policy-8");
    expect(container.textContent).not.toContain("policy-9");
    await act(async () => {
      root.unmount();
    });
  });

  it("notifies active 401 exactly once", async () => {
    const onUnauthenticated = vi.fn();
    const get = vi.fn(async () => ({ status: 401, data: {} }));
    const { root } = mount();
    await act(async () => {
      root.render(
        <CustomerMergeHistoryPanel
          customerID={41}
          role="ops"
          transport={{ get }}
          onUnauthenticated={onUnauthenticated}
        />,
      );
    });
    expect(get).toHaveBeenCalledTimes(1);
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    await act(async () => {
      root.unmount();
    });
  });

  it("drops a pending response after unmount without an auth callback", async () => {
    const pending = deferred<CustomerMergeHistoryTransportResponse>();
    const onUnauthenticated = vi.fn();
    const get = vi.fn(() => pending.promise);
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <CustomerMergeHistoryPanel
          customerID={41}
          role="admin"
          transport={{ get }}
          onUnauthenticated={onUnauthenticated}
        />,
      );
    });
    expect(get).toHaveBeenCalledTimes(1);
    await act(async () => {
      root.unmount();
    });
    await act(async () => {
      pending.resolve({ status: 401, data: {} });
      await pending.promise;
    });
    expect(onUnauthenticated).not.toHaveBeenCalled();
    expect(container.textContent).toBe("");
  });
});
