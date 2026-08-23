/* eslint-disable no-unused-vars -- minimal DOM shim fields are consumed by React DOM. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SessionStorageLike } from "./campaign-touch-plan-core";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";
import type { TouchPlanReviewSnapshot } from "./campaign-touch-plan-review";
import type { OutboundCampaignHandoffTransport } from "./outbound-campaign-handoff";
import { OutboundCampaignHandoffPanel } from "./outbound-campaign-handoff-ui";

class Node {
  parentNode: Node | null = null; childNodes: Node[] = []; ownerDocument!: Document;
  constructor(readonly nodeType: number, readonly nodeName: string) {}
  appendChild(node: Node): Node { node.parentNode = this; this.childNodes.push(node); return node; }
  insertBefore(node: Node, before: Node | null): Node { if (!before) return this.appendChild(node); node.parentNode = this; this.childNodes.splice(this.childNodes.indexOf(before), 0, node); return node; }
  removeChild(node: Node): Node { this.childNodes.splice(this.childNodes.indexOf(node), 1); node.parentNode = null; return node; }
  get firstChild(): Node | null { return this.childNodes[0] ?? null; }
  get nextSibling(): Node | null { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null; }
  get textContent(): string { return this.childNodes.map((node) => node.textContent).join(""); }
  set textContent(value: string) { this.childNodes = value ? [new Text(value, this.ownerDocument)] : []; }
  addEventListener(): void {} removeEventListener(): void {}
  contains(node: Node | null): boolean { return node === this || this.childNodes.some((child) => child.contains(node)); }
}
class Text extends Node {
  constructor(private value: string, owner: Document) { super(3, "#text"); this.ownerDocument = owner; }
  override get textContent(): string { return this.value; }
  override set textContent(value: string) { this.value = value; }
}
class Element extends Node {
  readonly tagName: string; readonly namespaceURI = "http://www.w3.org/1999/xhtml"; readonly style = {}; private attributes = new Map<string, string>();
  constructor(tag: string, owner: Document) { super(1, tag.toUpperCase()); this.tagName = tag.toUpperCase(); this.ownerDocument = owner; }
  setAttribute(name: string, value: string): void { this.attributes.set(name, value); }
  removeAttribute(name: string): void { this.attributes.delete(name); }
  getAttribute(name: string): string | null { return this.attributes.get(name) ?? null; }
  getAttributeNames(): string[] { return [...this.attributes.keys()]; }
  hasAttribute(name: string): boolean { return this.attributes.has(name); }
}
class Document extends Node {
  readonly documentElement: Element; readonly body: Element; readonly defaultView: Record<string, unknown>; activeElement: Element | null;
  constructor() { super(9, "#document"); this.ownerDocument = this; this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement); this.activeElement = this.body; this.defaultView = { document: this, navigator: { userAgent: "node" } }; }
  createElement(tag: string): Element { return new Element(tag, this); }
  createElementNS(_namespace: string, tag: string): Element { return this.createElement(tag); }
  createTextNode(value: string): Text { return new Text(value, this); }
  createComment(value: string): Text { return new Text(value, this); }
}
function mount(): { root: Root; container: Element } {
  const document = new Document(); const window = document.defaultView;
  Object.assign(window, { Node, Element, HTMLElement: Element, HTMLIFrameElement: Element, getSelection: () => null });
  for (const [key, value] of Object.entries({ document, window, Node, Element, HTMLElement: Element, HTMLIFrameElement: Element, IS_REACT_ACT_ENVIRONMENT: true })) vi.stubGlobal(key, value);
  const container = document.createElement("div"); document.body.appendChild(container);
  return { root: createRoot(container as unknown as globalThis.Element), container };
}
function elements(root: Node, tag: string): Element[] { return [root, ...root.childNodes.flatMap((node) => elements(node, tag))].filter((node): node is Element => node instanceof Element && node.tagName === tag.toUpperCase()); }
function props<T>(element: Element): T { const key = Object.keys(element).find((item) => item.startsWith("__reactProps")); if (!key) throw new Error("missing React props"); return (element as unknown as Record<string, T>)[key]; }
function deferred<T>() { let resolve!: (value: T) => void; return { promise: new Promise<T>((done) => { resolve = done; }), resolve }; }

const plan: TouchPlanSummary = { id: `ctp_${"a".repeat(64)}`, campaignCode: "campaign_a", campaignVersion: 4, source: { kind: "customer_selection", digest: "b".repeat(64) }, targetCount: 2, targetDigest: "c".repeat(64), stepCount: 1, contentDigest: "d".repeat(64), immutable: "verified-c1" };
const approved: TouchPlanReviewSnapshot = { review: { status: "approved", version: 3, submittedByActorID: 6, submittedAt: "2026-08-24T01:00:00Z", reviewedByActorID: 7, reviewedAt: "2026-08-24T02:00:00Z" }, handoff: { status: "pending_outbound_acceptance", reviewVersion: 3, createdAt: "2026-08-24T02:00:00Z" } };
const safety = { local_only: true, provider_execution_eligible: false, real_external_call_executed: false, delivery_proven: false };
const summary = { id: 9, campaign_code: plan.campaignCode, plan_id: plan.id, review_version: 3, status: "held", target_count: 2, step_count: 1, accepted_at: "2026-08-24T02:00:01Z", safety };
const reconciliation = { ...summary, held_count: 2, blocked_count: 0, pending_count: 0, not_evaluated_count: 2, eligible_count: 0, inactive_count: 0, contact_policy_count: 0 };
const storage = (): SessionStorageLike => { const values = new Map<string, string>(); return { getItem: (key) => values.get(key) ?? null, setItem: (key, value) => void values.set(key, value), removeItem: (key) => void values.delete(key) }; };
const csrf = `aicrm_csrf=${"x".repeat(43)}`;
afterEach(() => vi.unstubAllGlobals());

describe("OutboundCampaignHandoffPanel", () => {
  it("allows admin and ops but denies sales without reading", async () => {
    const denied: OutboundCampaignHandoffTransport = { getSummary: vi.fn(), accept: vi.fn(), reconcile: vi.fn() };
    expect(renderToStaticMarkup(<OutboundCampaignHandoffPanel role="sales" actorID={7} plan={plan} approved={approved} transport={denied} />)).toContain("没有本地交接接受权限");
    expect(denied.getSummary).not.toHaveBeenCalled();
    for (const role of ["admin", "ops"] as const) {
      const getSummary = vi.fn(async () => ({ status: 404, data: {} })); const mounted = mount();
      await act(async () => mounted.root.render(<OutboundCampaignHandoffPanel role={role} actorID={7} plan={plan} approved={approved} transport={{ getSummary, accept: vi.fn(), reconcile: vi.fn() }} />));
      expect(getSummary).toHaveBeenCalledOnce(); await act(async () => mounted.root.unmount());
    }
  });

  it("requires exact ACCEPT confirmation and renders only held local counts after acceptance", async () => {
    const accept = vi.fn(async () => ({ status: 200, data: reconciliation }));
    const transport: OutboundCampaignHandoffTransport = { getSummary: vi.fn(async () => ({ status: 404, data: {} })), accept, reconcile: vi.fn() };
    const mounted = mount();
    await act(async () => mounted.root.render(<OutboundCampaignHandoffPanel role="ops" actorID={7} plan={plan} approved={approved} transport={transport} sessionStorage={storage()} readCookie={() => csrf} keySource={{ randomUUID: () => "11111111-1111-4111-8111-111111111111" }} />));
    const button = elements(mounted.container, "button").find((item) => item.textContent === "接受为本地 held 事实")!;
    await act(async () => props<{ onClick(): void }>(button).onClick()); expect(accept).not.toHaveBeenCalled();
    const input = elements(mounted.container, "input")[0];
    await act(async () => props<{ onChange(event: { currentTarget: { value: string } }): void }>(input).onChange({ currentTarget: { value: `ACCEPT ${plan.id}` } }));
    await act(async () => props<{ onClick(): void }>(button).onClick());
    expect(accept).toHaveBeenCalledTimes(1); expect(mounted.container.textContent).toContain("本地 held2"); expect(mounted.container.textContent).toContain("资格未评估2");
    expect(mounted.container.textContent).toContain("不创建 Outbound 发送任务"); expect(mounted.container.textContent).not.toMatch(/已发送|已送达|Provider 已接受/);
    await act(async () => mounted.root.unmount());
  });

  it("aborts and ignores stale actor callbacks while reporting only the active 401", async () => {
    const first = deferred<{ status: number; data: unknown }>(); const second = deferred<{ status: number; data: unknown }>(); const signals: AbortSignal[] = []; let calls = 0;
    const transport: OutboundCampaignHandoffTransport = { getSummary: (_code, _id, options) => { signals.push(options.signal as AbortSignal); return ++calls === 1 ? first.promise : second.promise; }, accept: vi.fn(), reconcile: vi.fn() };
    const old401 = vi.fn(); const active401 = vi.fn(); const mounted = mount();
    await act(async () => mounted.root.render(<OutboundCampaignHandoffPanel role="admin" actorID={7} plan={plan} approved={approved} transport={transport} onUnauthenticated={old401} />));
    await act(async () => mounted.root.render(<OutboundCampaignHandoffPanel role="admin" actorID={8} plan={plan} approved={approved} transport={transport} onUnauthenticated={active401} />));
    expect(signals[0].aborted).toBe(true); await act(async () => first.resolve({ status: 401, data: {} })); expect(old401).not.toHaveBeenCalled();
    await act(async () => second.resolve({ status: 401, data: {} })); expect(active401).toHaveBeenCalledOnce(); await act(async () => mounted.root.unmount());
  });
});
