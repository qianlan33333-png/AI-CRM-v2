/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { ProductLocalControls } from "./products-local-controls";
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
function elements(root: TestNode, tagName: string): TestElement[] {
  return [root, ...root.childNodes.flatMap((node) => elements(node, tagName))].filter((node): node is TestElement => node instanceof TestElement && node.tagName === tagName);
}
function formFields(root: TestNode): TestElement[] {
  return [root, ...root.childNodes.flatMap(formFields)].filter((node): node is TestElement => node instanceof TestElement && (node.tagName === "INPUT" || node.tagName === "TEXTAREA"));
}
function reactProps<T extends Record<string, unknown>>(element: TestElement): T {
  const key = Object.keys(element).find((candidate) => candidate.startsWith("__reactProps"));
  if (key === undefined) throw new Error("mounted element is missing React props");
  return (element as unknown as Record<string, T>)[key];
}
function click(button: TestElement): void {
  const props = reactProps<{ onClick?: () => void }>(button);
  if (!props?.onClick) throw new Error("mounted button is missing its React click handler");
  props.onClick();
}
function deferred<T>(): { readonly promise: Promise<T>; resolve(value: T): void } {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>((done) => { resolve = done; }), resolve };
}
const product = { id: 1, product_code: "SKU-1", name: "商品", description: "本地描述", price_minor: 1990, currency: "CNY", stock_quantity: 3, images: ["opaque-image-value"], created_by: 7, created_at: "2026-08-19T00:00:00Z", updated_at: "2026-08-19T00:00:00Z", version: 1 };

describe("ProductsPage", () => {
  it("does not render a products reader for sales", () => {
    expect(renderToStaticMarkup(<ProductsPage role="sales" />)).toBe("");
  });
  it("states its local-only and non-payment boundary", () => {
    const html = renderToStaticMarkup(<ProductsPage role="admin" transport={{ list: async () => ({ status: 503, data: {} }), get: async () => ({ status: 503, data: {} }) }} />);
    expect(html).toContain("不展示图片链接");
    expect(html).toContain("不执行支付");
  });
  it("mounts product creation with one POST, the closed request, root refresh, failure retention, and one 401 callback", async () => {
    const initial = deferred<{ status: number; data: unknown }>();
    const createPending = deferred<{ status: number; data: unknown }>();
    const refresh = deferred<{ status: number; data: unknown }>();
    const create = vi.fn(() => createPending.promise);
    const list = vi.fn(() => list.mock.calls.length === 1 ? initial.promise : refresh.promise);
    const onUnauthenticated = vi.fn(); const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ProductsPage role="admin" readCookie={() => `aicrm_csrf=${"c".repeat(43)}`} onUnauthenticated={onUnauthenticated} transport={{ list: async () => list(), get: async () => ({ status: 200, data: product }), create }} />); });
    await act(async () => { initial.resolve({ status: 200, data: { items: [product] } }); await Promise.resolve(); });
    const values = ["SKU-2", "本地商品", "本地描述", "1990", "cny", "3"];
    expect(formFields(mounted.container)).toHaveLength(6);
    for (const [index, value] of values.entries()) {
      await act(async () => { reactProps<{ onChange?: (event: { currentTarget: { value: string } }) => void }>(formFields(mounted.container)[index]).onChange?.({ currentTarget: { value } }); });
    }
    const form = elements(mounted.container, "FORM")[0];
    await act(async () => { reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(form).onSubmit?.({ preventDefault() {} }); reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(form).onSubmit?.({ preventDefault() {} }); });
    expect(create).toHaveBeenCalledTimes(1);
    expect((create.mock.calls as unknown[][])[0]?.[0]).toEqual({ product_code: "SKU-2", name: "本地商品", description: "本地描述", price_minor: 1990, currency: "CNY", stock_quantity: 3, images: [] });
    expect((create.mock.calls as unknown[][])[0]?.[1]).toMatchObject({ credentials: "same-origin", headers: { "X-CSRF-Token": "c".repeat(43) } });
    await act(async () => { createPending.resolve({ status: 201, data: { ...product, id: 2, product_code: "SKU-2", name: "本地商品", description: "本地描述", images: [] } }); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("已创建本地产品：SKU-2");
    expect(list).toHaveBeenCalledTimes(2);
    expect(onUnauthenticated).not.toHaveBeenCalled();
    expect(create).toHaveBeenCalledTimes(1);
    await act(async () => { mounted.root.unmount(); refresh.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    expect(onUnauthenticated).not.toHaveBeenCalled();
  });
  it("keeps the draft and locks a local create after an outcome-unknown response", async () => {
    const list = vi.fn(async () => ({ status: 200, data: { items: [product] } }));
    const create = vi.fn(async () => ({ status: 201, data: { ...product, id: 3, product_code: "SKU-3", name: "本地商品", description: "保留的草稿", images: [], created_at: "2026-02-30T00:00:00Z" } }));
    const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ProductsPage role="ops" readCookie={() => `aicrm_csrf=${"c".repeat(43)}`} transport={{ list: async () => list(), get: async () => ({ status: 200, data: product }), create }} />); await Promise.resolve(); });
    const values = ["SKU-3", "本地商品", "保留的草稿", "1990", "CNY", "3"];
    for (const [index, value] of values.entries()) {
      await act(async () => { reactProps<{ onChange?: (event: { currentTarget: { value: string } }) => void }>(formFields(mounted.container)[index]).onChange?.({ currentTarget: { value } }); });
    }
    const submit = () => reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(elements(mounted.container, "FORM")[0]).onSubmit?.({ preventDefault() {} });
    await act(async () => { submit(); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("创建结果未知");
    expect(reactProps<{ value?: string }>(formFields(mounted.container)[2]).value).toBe("保留的草稿");
    expect(list).toHaveBeenCalledTimes(1);
    await act(async () => { submit(); await Promise.resolve(); });
    expect(create).toHaveBeenCalledTimes(1);
    await act(async () => { mounted.root.unmount(); });
  });
  it("calls the unauthenticated callback once for an active product create", async () => {
    const onUnauthenticated = vi.fn(); const create = vi.fn(async () => ({ status: 401, data: {} })); const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ProductsPage role="admin" readCookie={() => `aicrm_csrf=${"c".repeat(43)}`} onUnauthenticated={onUnauthenticated} transport={{ list: async () => ({ status: 200, data: { items: [] } }), get: async () => ({ status: 200, data: product }), create }} />); await Promise.resolve(); });
    const values = ["SKU-4", "本地商品", "", "1", "CNY", "0"];
    for (const [index, value] of values.entries()) {
      await act(async () => { reactProps<{ onChange?: (event: { currentTarget: { value: string } }) => void }>(formFields(mounted.container)[index]).onChange?.({ currentTarget: { value } }); });
    }
    await act(async () => { reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(elements(mounted.container, "FORM")[0]).onSubmit?.({ preventDefault() {} }); await Promise.resolve(); });
    expect(create).toHaveBeenCalledTimes(1);
    expect(onUnauthenticated).toHaveBeenCalledTimes(1);
    await act(async () => { mounted.root.unmount(); });
  });
  it("mounts the injected local entitlement controls without rendering hidden customer facts", async () => {
    const versioned = { ...product, version: 1 };
    const entitlement = { id: 19, product_id: 1, order_id: 44, state: "active", version: 1, granted_at: "2026-08-20T09:00:00Z", revoked_at: null };
    const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ProductsPage role="admin" readCookie={() => `aicrm_csrf=${"c".repeat(43)}`} transport={{ list: async () => ({ status: 200, data: { items: [versioned] } }), get: async () => ({ status: 200, data: versioned }), update: async () => ({ status: 200, data: { ...versioned, version: 2 } }), listEntitlements: async () => ({ status: 200, data: { items: [entitlement] } }), getEntitlement: async () => ({ status: 200, data: entitlement }), grantEntitlement: async () => ({ status: 201, data: entitlement }), revokeEntitlement: async () => ({ status: 200, data: { ...entitlement, state: "revoked", version: 2, revoked_at: "2026-08-20T10:00:00Z" } }) }} />); await Promise.resolve(); });
    await act(async () => { click(buttons(mounted.container).find((button) => button.textContent === "查看详情")!); await Promise.resolve(); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("按版本更新");
    expect(mounted.container.textContent).toContain("授予本地权益");
		expect(mounted.container.textContent).not.toContain("customer_id");
    await act(async () => { mounted.root.unmount(); });
  });
  it("uses the mounted product controls' synchronous writer locks and turns a drifted readback into outcome-unknown", async () => {
    const updatePending = deferred<{ status: number; data: unknown }>();
    const versioned = { ...product, version: 1 };
    const update = vi.fn(() => updatePending.promise);
    const get = vi.fn(() => get.mock.calls.length === 1
      ? Promise.resolve({ status: 200, data: versioned })
      : Promise.resolve({ status: 200, data: { ...versioned, name: "已更新", version: 2, created_by: 99, updated_at: "2026-08-20T11:00:00Z" } }));
    const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ProductsPage role="admin" readCookie={() => `aicrm_csrf=${"c".repeat(43)}`} transport={{ list: async () => ({ status: 200, data: { items: [versioned] } }), get, update }} />); await Promise.resolve(); });
    await act(async () => { click(buttons(mounted.container).find((button) => button.textContent === "查看详情")!); await Promise.resolve(); });
    await act(async () => { reactProps<{ onChange?: (event: { currentTarget: { value: string } }) => void }>(formFields(mounted.container)[6]).onChange?.({ currentTarget: { value: "已更新" } }); });
    const updateForm = elements(mounted.container, "FORM")[1];
    await act(async () => { reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(updateForm).onSubmit?.({ preventDefault() {} }); reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(updateForm).onSubmit?.({ preventDefault() {} }); });
    expect(update).toHaveBeenCalledTimes(1);
    await act(async () => { updatePending.resolve({ status: 200, data: { ...versioned, name: "已更新", version: 2, updated_at: "2026-08-20T11:00:00Z" } }); await Promise.resolve(); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("结果未知");
    await act(async () => { reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(updateForm).onSubmit?.({ preventDefault() {} }); });
    expect(update).toHaveBeenCalledTimes(1);
    await act(async () => { mounted.root.unmount(); });
  });
  it("does not let an unmounted local update publish a stale 401", async () => {
    const response = deferred<{ status: number; data: unknown }>();
    const onUnauthenticated = vi.fn(); const versioned = { ...product, version: 1 };
    const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ProductsPage role="admin" readCookie={() => `aicrm_csrf=${"c".repeat(43)}`} onUnauthenticated={onUnauthenticated} transport={{ list: async () => ({ status: 200, data: { items: [versioned] } }), get: async () => ({ status: 200, data: versioned }), update: async () => response.promise }} />); await Promise.resolve(); });
    await act(async () => { click(buttons(mounted.container).find((button) => button.textContent === "查看详情")!); await Promise.resolve(); await Promise.resolve(); });
    await act(async () => { reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(elements(mounted.container, "FORM")[1]).onSubmit?.({ preventDefault() {} }); });
    await act(async () => { mounted.root.unmount(); response.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    expect(onUnauthenticated).not.toHaveBeenCalled();
  });
	it("locks an in-flight local update as unknown across a transport lifetime replacement", async () => {
		const pending = deferred<{ status: number; data: unknown }>();
		const updateA = vi.fn(() => pending.promise); const updateB = vi.fn(async () => ({ status: 200, data: { ...product, version: 2 } })); const onUnauthenticated = vi.fn();
		const mounted = mountedRoot(); const common = { product: { id: 1, productCode: "SKU-1", name: "商品", description: "本地描述", priceMinor: 1990, currency: "CNY", stockQuantity: 3, images: ["opaque-image-value"], createdBy: 7, createdAt: "2026-08-19T00:00:00Z", updatedAt: "2026-08-19T00:00:00Z", version: 1 }, readCookie: () => `aicrm_csrf=${"c".repeat(43)}`, onUnauthenticated, onProductUpdated: vi.fn() };
		const transportA = { list: async () => ({ status: 200, data: { items: [product] } }), get: async () => ({ status: 200, data: product }), update: updateA, listEntitlements: async () => ({ status: 200, data: { items: [] } }) };
		const transportB = { ...transportA, update: updateB };
		await act(async () => { mounted.root.render(<ProductLocalControls {...common} transport={transportA} />); await Promise.resolve(); });
		const updateForm = elements(mounted.container, "FORM")[0];
		await act(async () => { reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(updateForm).onSubmit?.({ preventDefault() {} }); });
		expect(updateA).toHaveBeenCalledTimes(1);
		await act(async () => { mounted.root.render(<ProductLocalControls {...common} transport={transportB} />); await Promise.resolve(); });
		expect(mounted.container.textContent).toContain("结果未知");
		await act(async () => { reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(elements(mounted.container, "FORM")[0]).onSubmit?.({ preventDefault() {} }); pending.resolve({ status: 401, data: {} }); await Promise.resolve(); });
		expect(updateB).not.toHaveBeenCalled();
		expect(onUnauthenticated).not.toHaveBeenCalled();
		await act(async () => { mounted.root.unmount(); });
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
