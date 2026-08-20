/* eslint-disable no-unused-vars -- minimal DOM shim exposes React structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  HXCSenderPage,
  HXCSenderView,
  startHXCSenderRead,
} from "./hxc-sender-ui";
import type { HXCSenderTransport } from "./hxc-sender";

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
  get options(): TestElement[] {
    return this.childNodes.filter(
      (node): node is TestElement =>
        node instanceof TestElement && node.tagName === "OPTION",
    );
  }
}
class TestDocument extends TestNode {
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
function mountedRoot(): {
  readonly root: Root;
  readonly container: TestElement;
} {
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
function deferred<T>(): {
  readonly promise: Promise<T>;
  resolve(value: T): void;
} {
  let resolve!: (value: T) => void;
  return {
    promise: new Promise<T>((done) => {
      resolve = done;
    }),
    resolve,
  };
}

const body = (userID = "alice", displayName = "Alice") => ({
  ok: true,
  source_status: "v2_local_staff",
  route_owner: "aicrm_v2",
  fallback_used: false,
  real_external_call_executed: false,
  send_configs: [
    {
      id: "cfg-1",
      sender_userid: userID,
      display_name: displayName,
      priority: 2,
      is_active: true,
      created_at: "2026-08-19T00:00:00Z",
      updated_at: "2026-08-19T00:00:00Z",
    },
  ],
  directory_candidates: [
    {
      wecom_userid: userID,
      display_name: displayName,
      position: "",
      wecom_status: 0,
      is_sender: true,
      priority: 2,
      is_active: true,
    },
  ],
  members: [
    {
      wecom_userid: userID,
      display_name: displayName,
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
});
function response(data: unknown, status = 200) {
  return { status, data, headers: new Headers() };
}

describe("HXCSenderPage", () => {
  it("renders only the frozen local projection and local-only filters", () => {
    const html = renderToStaticMarkup(
      <HXCSenderView
        state={{
          kind: "ready",
          model: {
            sendConfigs: [],
            members: [
              {
                wecomUserID: "alice",
                displayName: "Alice",
                position: "",
                wecomStatus: 0,
                isSender: true,
                priority: 2,
                isActive: true,
              },
            ],
            directoryCount: 1,
            senderCount: 1,
            activeSenderCount: 1,
            lastSyncedAt: "2026-08-19T00:00:00Z",
            warning:
              "HXC senders use the local staff projection; no WeCom directory call was executed.",
            emptyState: false,
          },
        }}
        keyword=""
        status="all"
        onKeyword={() => undefined}
        onStatus={() => undefined}
        onRefresh={() => undefined}
      />,
    );
    expect(html).toContain("只读本地投影");
    expect(html).toContain("本地搜索");
    expect(html).toContain("手动刷新本地投影");
    expect(html).toContain("Alice");
    expect(html).not.toMatch(/mobile|avatar|department|provider|token|secret/i);
  });

  it("makes zero requests for ops and sales", async () => {
    for (const role of ["ops", "sales"] as const) {
      const transport = {
        read: vi.fn(async () => response(body())),
      } as unknown as HXCSenderTransport;
      const { root } = mountedRoot();
      await act(async () => {
        root.render(<HXCSenderPage role={role} transport={transport} />);
      });
      expect(transport.read).not.toHaveBeenCalled();
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("gates same-tick refresh and drops stale transport and unmount results", async () => {
    const pendingA = deferred<ReturnType<typeof response>>();
    const pendingB = deferred<ReturnType<typeof response>>();
    const unauthenticated = vi.fn();
    const transportA = {
      read: vi.fn(() => pendingA.promise),
    } as unknown as HXCSenderTransport;
    const transportB = {
      read: vi.fn(() => pendingB.promise),
    } as unknown as HXCSenderTransport;
    const { root, container } = mountedRoot();
    await act(async () => {
      root.render(
        <HXCSenderPage
          role="admin"
          transport={transportA}
          onUnauthenticated={unauthenticated}
        />,
      );
    });
    expect(transportA.read).toHaveBeenCalledTimes(1);
    await act(async () => {
      root.render(
        <HXCSenderPage
          role="admin"
          transport={transportB}
          onUnauthenticated={unauthenticated}
        />,
      );
    });
    expect(transportB.read).toHaveBeenCalledTimes(1);
    await act(async () => {
      pendingA.resolve(response({}, 401));
      await pendingA.promise;
    });
    expect(unauthenticated).not.toHaveBeenCalled();
    await act(async () => {
      pendingB.resolve(response(body("bruno", "Bruno")));
      await pendingB.promise;
    });
    expect(container.textContent).toContain("Bruno");
    const controllerRead = deferred<ReturnType<typeof response>>();
    const controllerTransport = {
      read: vi.fn(() => controllerRead.promise),
    } as unknown as HXCSenderTransport;
    const controller = {
      generation: { current: 0 },
      inFlight: { current: undefined as symbol | undefined },
      verified: { current: undefined },
      onState: vi.fn(),
    };
    const first = startHXCSenderRead(controller, controllerTransport);
    const second = startHXCSenderRead(controller, controllerTransport);
    expect(controllerTransport.read).toHaveBeenCalledTimes(1);
    await act(async () => {
      controllerRead.resolve(response(body("carol", "Carol")));
      await Promise.all([first, second]);
    });
    expect(controller.inFlight.current).toBeUndefined();
    const pendingUnmount = deferred<ReturnType<typeof response>>();
    const transportUnmount = {
      read: vi.fn(() => pendingUnmount.promise),
    } as unknown as HXCSenderTransport;
    await act(async () => {
      root.render(
        <HXCSenderPage
          role="admin"
          transport={transportUnmount}
          onUnauthenticated={unauthenticated}
        />,
      );
    });
    await act(async () => {
      root.unmount();
      pendingUnmount.resolve(response({}, 401));
      await pendingUnmount.promise;
    });
    expect(unauthenticated).not.toHaveBeenCalled();
  });
});
