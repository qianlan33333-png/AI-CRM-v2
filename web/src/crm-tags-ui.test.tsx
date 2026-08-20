/* eslint-disable no-unused-vars -- minimal DOM shim exposes React structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CRMTagCatalogPage } from "./crm-tags-ui";
import type { CRMTagTransport } from "./crm-tags";

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
    return (
      this.parentNode.childNodes[
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
function elements(root: TestNode, tagName: string): TestElement[] {
  return [
    root,
    ...root.childNodes.flatMap((node) => elements(node, tagName)),
  ].filter(
    (node): node is TestElement =>
      node instanceof TestElement && node.tagName === tagName,
  );
}
function reactProps<T extends Record<string, unknown>>(
  element: TestElement,
): T {
  const key = Object.keys(element).find((candidate) =>
    candidate.startsWith("__reactProps"),
  );
  if (key === undefined)
    throw new Error("mounted element is missing React props");
  return (element as unknown as Record<string, T>)[key];
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

const catalog = (groupName: string, tagIDs = [21, 22, 31]) => ({
  groups: [
    { id: 11, name: groupName, sort_order: 0 },
    { id: 12, name: "Fit", sort_order: 1 },
  ],
  tags: tagIDs.map((id, index) => ({
    id,
    group_id: id === 31 ? 12 : 11,
    group_name: id === 31 ? "Fit" : groupName,
    name: `Tag-${id}`,
    sort_order: index,
  })),
});
describe("CRMTagCatalogPage", () => {
  it("keeps sales fail-closed and does not expose mutations", () => {
    const html = renderToStaticMarkup(<CRMTagCatalogPage role="sales" />);
    expect(html).toContain("没有本地标签目录访问权限");
    expect(html).not.toContain("新建");
  });
  it("does not send when an explicitly empty local catalog transport is injected", () => {
    const html = renderToStaticMarkup(
      <CRMTagCatalogPage role="admin" transport={{}} />,
    );
    expect(html).toContain("尚未接入");
    expect(html).toContain("未发送任何请求");
  });
  it("mounts the generated local catalog workflow by default", () => {
    const html = renderToStaticMarkup(<CRMTagCatalogPage role="admin" />);
    expect(html).toContain("新建标签组");
    expect(html).toContain("首个标签");
  });

  it("drops a stale catalog read after transport replacement and unmount", async () => {
    const pendingA = deferred<{ status: number; data: unknown }>();
    const pendingB = deferred<{ status: number; data: unknown }>();
    const unauthenticated = vi.fn();
    const transportA: CRMTagTransport = {
      list: vi.fn(() => pendingA.promise),
    };
    const transportB: CRMTagTransport = {
      list: vi.fn(() => pendingB.promise),
    };
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <CRMTagCatalogPage
          role="admin"
          transport={transportA}
          onUnauthenticated={unauthenticated}
        />,
      );
      await Promise.resolve();
    });
    expect(transportA.list).toHaveBeenCalledOnce();
    await act(async () => {
      mounted.root.render(
        <CRMTagCatalogPage
          role="admin"
          transport={transportB}
          onUnauthenticated={unauthenticated}
        />,
      );
      await Promise.resolve();
    });
    expect(transportB.list).toHaveBeenCalledOnce();
    await act(async () => {
      pendingB.resolve({ status: 200, data: catalog("Current") });
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("Current");
    await act(async () => {
      pendingA.resolve({ status: 200, data: catalog("Stale") });
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("Current");
    expect(mounted.container.textContent).not.toContain("Stale");
    await act(async () => mounted.root.unmount());

    const pendingUnmount = deferred<{ status: number; data: unknown }>();
    const unmounted = mountedRoot();
    const unmountedTransport: CRMTagTransport = {
      list: vi.fn(() => pendingUnmount.promise),
    };
    await act(async () => {
      unmounted.root.render(
        <CRMTagCatalogPage
          role="ops"
          transport={unmountedTransport}
          onUnauthenticated={unauthenticated}
        />,
      );
      await Promise.resolve();
      unmounted.root.unmount();
      pendingUnmount.resolve({ status: 401, data: {} });
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(unauthenticated).not.toHaveBeenCalled();
  });

  it("sends one exact full tag reorder and accepts only the confirmed reread order", async () => {
    const list = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: catalog("Lifecycle") })
      .mockResolvedValueOnce({
        status: 200,
        data: catalog("Lifecycle", [22, 21, 31]),
      });
    const reorderTags = vi.fn(async () => ({
      status: 200,
      data: { items: [] },
    }));
    const transport: CRMTagTransport = { list, reorderTags };
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <CRMTagCatalogPage
          role="admin"
          transport={transport}
          readCookie={() => `aicrm_csrf=${"b".repeat(43)}`}
        />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    const down = elements(mounted.container, "BUTTON").find(
      (button) => button.textContent === "标签下移",
    );
    if (down === undefined) throw new Error("missing tag reorder button");
    const props = reactProps<{ onClick?: () => void }>(down);
    await act(async () => {
      props.onClick?.();
      props.onClick?.();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(reorderTags).toHaveBeenCalledOnce();
    const [request, options] = reorderTags.mock.calls[0] as unknown as [
      { ids: number[] },
      RequestInit,
    ];
    expect(request.ids).toEqual([22, 21, 31]);
    expect(options.credentials).toBe("same-origin");
    expect(options.headers).toMatchObject({
      "X-CSRF-Token": "b".repeat(43),
    });
    expect(
      String((options.headers as Record<string, string>)["Idempotency-Key"])
        .length,
    ).toBeGreaterThanOrEqual(16);
    expect(list).toHaveBeenCalledTimes(2);
    expect(mounted.container.textContent).toContain("列表回读确认");
    await act(async () => mounted.root.unmount());
  });

  it("locks outcome unknown when a pending tag write is replaced or unmounted without letting the old write reread", async () => {
    const pendingA = deferred<{ status: number; data: unknown }>();
    const pendingUnmount = deferred<{ status: number; data: unknown }>();
    const unauthenticated = vi.fn();
    const transportA: CRMTagTransport = {
      list: vi.fn().mockResolvedValue({ status: 200, data: catalog("A") }),
      reorderTags: vi.fn(() => pendingA.promise),
    };
    const transportB: CRMTagTransport = {
      list: vi.fn().mockResolvedValue({ status: 200, data: catalog("B") }),
    };
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <CRMTagCatalogPage
          role="admin"
          transport={transportA}
          readCookie={() => `aicrm_csrf=${"b".repeat(43)}`}
          onUnauthenticated={unauthenticated}
        />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    const down = elements(mounted.container, "BUTTON").find(
      (button) => button.textContent === "标签下移",
    );
    if (down === undefined) throw new Error("missing tag reorder button");
    await act(async () => {
      reactProps<{ onClick?: () => void }>(down).onClick?.();
      await Promise.resolve();
    });
    expect(transportA.reorderTags).toHaveBeenCalledOnce();
    await act(async () => {
      mounted.root.render(
        <CRMTagCatalogPage
          role="admin"
          transport={transportB}
          readCookie={() => `aicrm_csrf=${"b".repeat(43)}`}
          onUnauthenticated={unauthenticated}
        />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("结果尚未确认");
    expect(transportB.list).toHaveBeenCalledOnce();
    await act(async () => {
      pendingA.resolve({ status: 401, data: {} });
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(transportA.list).toHaveBeenCalledOnce();
    expect(unauthenticated).not.toHaveBeenCalled();
    await act(async () => mounted.root.unmount());

    const unmountedTransport: CRMTagTransport = {
      list: vi.fn().mockResolvedValue({ status: 200, data: catalog("U") }),
      reorderTags: vi.fn(() => pendingUnmount.promise),
    };
    const unmounted = mountedRoot();
    await act(async () => {
      unmounted.root.render(
        <CRMTagCatalogPage
          role="ops"
          transport={unmountedTransport}
          readCookie={() => `aicrm_csrf=${"b".repeat(43)}`}
          onUnauthenticated={unauthenticated}
        />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    const unmountDown = elements(unmounted.container, "BUTTON").find(
      (button) => button.textContent === "标签下移",
    );
    if (unmountDown === undefined) throw new Error("missing tag reorder button");
    await act(async () => {
      reactProps<{ onClick?: () => void }>(unmountDown).onClick?.();
      await Promise.resolve();
      unmounted.root.unmount();
      pendingUnmount.resolve({ status: 401, data: {} });
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(unmountedTransport.list).toHaveBeenCalledOnce();
    expect(unauthenticated).not.toHaveBeenCalled();
  });
});
