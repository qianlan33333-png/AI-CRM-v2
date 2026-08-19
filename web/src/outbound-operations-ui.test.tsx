/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { OutboundOperationsPage, OutboundOperationsView } from "./outbound-operations-ui";
import type { OutboundOperationsTransport } from "./outbound-operations";

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
  get options(): TestElement[] { return this.childNodes.filter((node): node is TestElement => node instanceof TestElement && node.tagName === "OPTION"); }
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
  constructor() { super(9, "#document"); this.ownerDocument = this; this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement); this.activeElement = this.body; this.defaultView = { document: this, navigator: { userAgent: "node" } }; }
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
function elements(root: TestNode, tagName: string): TestElement[] { return [root, ...root.childNodes.flatMap((node) => elements(node, tagName))].filter((node): node is TestElement => node instanceof TestElement && node.tagName === tagName); }
function buttons(root: TestNode): TestElement[] { return elements(root, "BUTTON"); }
function click(button: TestElement): void { const key = Object.keys(button).find((candidate) => candidate.startsWith("__reactProps")); const props = key === undefined ? undefined : (button as unknown as Record<string, { onClick?: () => void }>)[key]; if (!props?.onClick) throw new Error("mounted button is missing React click handler"); props.onClick(); }
function deferred<T>(): { readonly promise: Promise<T>; resolve(value: T): void } { let resolve!: (value: T) => void; return { promise: new Promise<T>((done) => { resolve = done; }), resolve }; }

const task = (taskID: number, updatedAt = "2026-08-20T08:00:01Z") => ({ job_id: taskID, task_id: taskID, customer_id: taskID + 100, status: "outcome_unknown", attempt_count: 2, delivery_proven: false, queue_job: { river_job_id: taskID + 200, generation: 2, kind: "outbound_enqueue_one" }, created_at: "2026-08-20T08:00:00Z", status_updated_at: updatedAt });
const page = (job: ReturnType<typeof task>) => ({ status: 200, data: { ok: true, jobs: [job], items: [job], count: 1, has_more: false, limit: 50, offset: 0, source_status: "v2_outbound_service", fallback_used: false } });
const reconciliation = (job: ReturnType<typeof task>) => ({ status: 200, data: { ok: true, job, attempts: [{ attempt_id: 501, history_id: 601, generation: 2, river_job_id: 701, attempt: 1, max_attempts: 2, state: "reserved" }], control_receipts: [], source_status: "v2_outbound_service", fallback_used: false } });

describe("outbound observation UI", () => {
  it("keeps the admin-only outbound carrier closed to ops and sales", () => { for (const role of ["ops", "sales"] as const) { const t: OutboundOperationsTransport = { list: vi.fn(), reconciliation: vi.fn() }; expect(renderToStaticMarkup(<OutboundOperationsPage role={role} transport={t} />)).toContain("没有本地投递运营查看权限"); expect(t.list).not.toHaveBeenCalled(); } });
  it("has no controls", () => { const value = { taskID: "42", status: "pending", attemptCount: 0, generation: 1, queueKind: "outbound_enqueue_one", createdAt: "2026-08-20T08:00:00Z", updatedAt: "2026-08-20T08:00:01Z" }; const html = renderToStaticMarkup(<OutboundOperationsView status="" businessID="" onStatus={vi.fn()} onBusinessID={vi.fn()} onLoad={vi.fn()} onSelect={vi.fn()} state={{ kind: "ready", page: { items: [value], offset: 0, hasMore: false } }} detail={{ kind: "ready", value: { task: value, attempts: [], receipts: [] } }} />); expect(html).not.toMatch(/取消|重排/); });
  it("releases token-owned page and reconciliation flights across a transport replacement", async () => {
    const aInitialPage = deferred<{ status: number; data: unknown }>(); const aPendingPage = deferred<{ status: number; data: unknown }>(); const aInitialDetail = deferred<{ status: number; data: unknown }>(); const aPendingDetail = deferred<{ status: number; data: unknown }>(); const bPage = deferred<{ status: number; data: unknown }>(); const bDetail = deferred<{ status: number; data: unknown }>();
    let aPageCalls = 0; let aDetailCalls = 0;
    const clientA: OutboundOperationsTransport = { list: vi.fn(() => (++aPageCalls === 1 ? aInitialPage.promise : aPendingPage.promise)), reconciliation: vi.fn(() => (++aDetailCalls === 1 ? aInitialDetail.promise : aPendingDetail.promise)) };
    const clientB: OutboundOperationsTransport = { list: vi.fn(() => bPage.promise), reconciliation: vi.fn(() => bDetail.promise) };
    const old401 = vi.fn(); const active401 = vi.fn(); const mounted = mountedRoot();
    const button = (text: string) => { const result = buttons(mounted.container).find((candidate) => candidate.textContent === text); if (!result) throw new Error(`missing ${text}`); return result; };
    await act(async () => { mounted.root.render(<OutboundOperationsPage role="admin" transport={clientA} onUnauthenticated={old401} />); await Promise.resolve(); });
    await act(async () => { aInitialPage.resolve(page(task(42))); await Promise.resolve(); });
    await act(async () => { click(button("读取本地对账")); await Promise.resolve(); });
    await act(async () => { aInitialDetail.resolve(reconciliation(task(42))); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("501");
    await act(async () => { click(button("筛选本地任务")); click(button("读取本地对账")); await Promise.resolve(); });
    expect(clientA.list).toHaveBeenCalledTimes(2); expect(clientA.reconciliation).toHaveBeenCalledTimes(2);
    await act(async () => { mounted.root.render(<OutboundOperationsPage role="admin" transport={clientB} onUnauthenticated={active401} />); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("42"); expect(clientB.list).toHaveBeenCalledOnce();
    await act(async () => { aPendingPage.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    await act(async () => { click(button("筛选本地任务")); await Promise.resolve(); });
    expect(clientB.list).toHaveBeenCalledOnce();
    await act(async () => { bPage.resolve(page(task(42, "2026-08-20T09:00:01Z"))); await Promise.resolve(); });
    await act(async () => { click(button("读取本地对账")); await Promise.resolve(); });
    expect(clientB.reconciliation).toHaveBeenCalledOnce();
    await act(async () => { aPendingDetail.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    await act(async () => { click(button("读取本地对账")); await Promise.resolve(); });
    expect(clientB.reconciliation).toHaveBeenCalledOnce();
    expect(mounted.container.textContent).toContain("2026-08-20T09:00:01Z"); expect(mounted.container.textContent).toContain("501"); expect(old401).not.toHaveBeenCalled(); expect(active401).not.toHaveBeenCalled();
    await act(async () => { bDetail.resolve({ status: 401, data: {} }); await Promise.resolve(); });
    expect(active401).toHaveBeenCalledOnce(); expect(mounted.container.textContent).toContain("501");
    await act(async () => { mounted.root.unmount(); });
  });
});
