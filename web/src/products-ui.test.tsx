/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { ProductsPage } from "./products-ui";

class TestNode {
  parentNode: TestNode | null = null;
  childNodes: TestNode[] = [];
  ownerDocument!: TestDocument;
  constructor(readonly nodeType: number, readonly nodeName: string) {}
  appendChild(node: TestNode): TestNode { node.parentNode = this; this.childNodes.push(node); return node; }
  insertBefore(node: TestNode, before: TestNode | null): TestNode { if (before === null) return this.appendChild(node); node.parentNode = this; this.childNodes.splice(this.childNodes.indexOf(before), 0, node); return node; }
  removeChild(node: TestNode): TestNode { this.childNodes.splice(this.childNodes.indexOf(node), 1); node.parentNode = null; return node; }
  get firstChild(): TestNode | null { return this.childNodes[0] ?? null; }
  get nextSibling(): TestNode | null { if (!this.parentNode) return null; return this.parentNode.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null; }
  get textContent(): string { return this.childNodes.map((node) => node.textContent).join(""); }
  set textContent(value: string) { this.childNodes = value === "" ? [] : [new TestText(value, this.ownerDocument)]; }
  addEventListener(): void {}
  removeEventListener(): void {}
  contains(node: TestNode | null): boolean { return node === this || this.childNodes.some((child) => child.contains(node)); }
}
class TestText extends TestNode {
  constructor(private data: string, ownerDocument: TestDocument) { super(3, "#text"); this.ownerDocument = ownerDocument; }
  override get textContent(): string { return this.data; }
  override set textContent(value: string) { this.data = value; }
}
class TestElement extends TestNode {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style: Record<string, string> = {};
  private readonly attributes = new Map<string, string>();
  constructor(tagName: string, ownerDocument: TestDocument) { super(1, tagName.toUpperCase()); this.tagName = tagName.toUpperCase(); this.ownerDocument = ownerDocument; }
  setAttribute(name: string, value: string): void { this.attributes.set(name, value); }
  removeAttribute(name: string): void { this.attributes.delete(name); }
  getAttribute(name: string): string | null { return this.attributes.get(name) ?? null; }
  hasAttribute(name: string): boolean { return this.attributes.has(name); }
}
class TestDocument extends TestNode {
  readonly nodeType = 9;
  readonly documentElement: TestElement;
  readonly body: TestElement;
  readonly defaultView: Record<string, unknown>;
  activeElement: TestElement | null;
  constructor() {
    super(9, "#document"); this.ownerDocument = this;
    this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement);
    this.activeElement = this.body;
    this.defaultView = { document: this, navigator: { userAgent: "node" } };
  }
  createElement(tagName: string): TestElement { return new TestElement(tagName, this); }
  createElementNS(_namespace: string, tagName: string): TestElement { return this.createElement(tagName); }
  createTextNode(value: string): TestText { return new TestText(value, this); }
  createComment(value: string): TestText { return new TestText(value, this); }
}

function mountedRoot(): { readonly root: Root; readonly container: TestElement } {
  const document = new TestDocument();
  const window = document.defaultView as Record<string, unknown>;
  Object.assign(window, { Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, getSelection: () => null });
  Object.assign(globalThis, { document, window, Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, IS_REACT_ACT_ENVIRONMENT: true });
  const container = document.createElement("div"); document.body.appendChild(container);
  return { root: createRoot(container as unknown as Element), container };
}

function buttons(root: TestNode): TestElement[] {
  return [root, ...root.childNodes.flatMap(buttons)].filter((node): node is TestElement => node instanceof TestElement && node.tagName === "BUTTON");
}
function click(button: TestElement): void {
  const key = Object.keys(button).find((candidate) => candidate.startsWith("__reactProps"));
  const props = key === undefined ? undefined : (button as unknown as Record<string, { onClick?: () => void }>)[key];
  if (!props?.onClick) throw new Error("mounted button is missing its React click handler");
  props.onClick();
}
function deferred<T>(): { readonly promise: Promise<T>; resolve(value: T): void } {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>((done) => { resolve = done; }), resolve };
}
const product = { id: 1, product_code: "SKU-1", name: "商品", description: "本地描述", price_minor: 1990, currency: "CNY", stock_quantity: 3, images: ["opaque-image-value"], created_by: 7, created_at: "2026-08-19T00:00:00Z", updated_at: "2026-08-19T00:00:00Z" };

describe("ProductsPage", () => {
  it("does not render a products reader for sales", () => {
    expect(renderToStaticMarkup(<ProductsPage role="sales" />)).toBe("");
  });
  it("states its local-only and non-payment boundary", () => {
    const html = renderToStaticMarkup(<ProductsPage role="admin" transport={{ list: async () => ({ status: 503, data: {} }), get: async () => ({ status: 503, data: {} }) }} />);
    expect(html).toContain("不展示图片链接");
    expect(html).toContain("不执行支付");
  });
  it("mounts the detail state machine with exact IDs, singleflight, stale-response, failure-retention, and one 401 callback", async () => {
    const initial = deferred<{ status: number; data: unknown }>();
    const first = deferred<{ status: number; data: unknown }>();
    const second = deferred<{ status: number; data: unknown }>();
    const unavailable = deferred<{ status: number; data: unknown }>();
    const invalid = deferred<{ status: number; data: unknown }>();
    const unauthenticated = deferred<{ status: number; data: unknown }>();
    const unmountRequest = deferred<{ status: number; data: unknown }>();
    const pending = [first, second, unavailable, invalid, unauthenticated, unmountRequest];
    const get = vi.fn((_id: number) => pending[get.mock.calls.length - 1].promise);
    const onUnauthenticated = vi.fn(); const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ProductsPage role="admin" onUnauthenticated={onUnauthenticated} transport={{ list: async () => initial.promise, get: async (id) => get(id) }} />); });
    await act(async () => { initial.resolve({ status: 200, data: { items: [product, { ...product, id: 2, product_code: "SKU-2" }] } }); await Promise.resolve(); });
    const rows = buttons(mounted.container).filter((button) => button.textContent === "查看详情");
    expect(rows).toHaveLength(2);
    await act(async () => { click(rows[0]); click(rows[0]); });
    expect(get.mock.calls).toEqual([[1]]);
    await act(async () => { click(rows[1]); first.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    expect(get.mock.calls).toEqual([[1], [2]]);
    expect(onUnauthenticated).not.toHaveBeenCalled();
    await act(async () => { second.resolve({ status: 200, data: { ...product, id: 2, product_code: "SKU-2" } }); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("SKU-2");
    await act(async () => { click(rows[1]); unavailable.resolve({ status: 503, data: {} }); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("SKU-2");
    await act(async () => { click(rows[1]); invalid.resolve({ status: 200, data: { ...product, id: 2, product_code: "SKU-2", image_url: "https://forbidden.example" } }); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("SKU-2");
    await act(async () => { click(rows[1]); unauthenticated.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    await act(async () => { click(rows[1]); });
    await act(async () => { mounted.root.unmount(); unmountRequest.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
  });
});
