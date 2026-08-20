/* eslint-disable no-unused-vars -- a small structural DOM shim exercises mounted lifecycle ownership. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { IdentityConsolePage } from "./identity-console-ui";
import type { IdentityConsoleTransport } from "./identity-console";

class NodeShim { parentNode: NodeShim | null = null; childNodes: NodeShim[] = []; ownerDocument!: DocumentShim; constructor(readonly nodeType: number, readonly nodeName: string) {} appendChild(node: NodeShim): NodeShim { node.parentNode = this; this.childNodes.push(node); return node; } insertBefore(node: NodeShim, before: NodeShim | null): NodeShim { if (!before) return this.appendChild(node); node.parentNode = this; this.childNodes.splice(this.childNodes.indexOf(before), 0, node); return node; } removeChild(node: NodeShim): NodeShim { this.childNodes.splice(this.childNodes.indexOf(node), 1); node.parentNode = null; return node; } get firstChild(): NodeShim | null { return this.childNodes[0] ?? null; } get nextSibling(): NodeShim | null { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null; } get textContent(): string { return this.childNodes.map((node) => node.textContent).join(""); } set textContent(value: string) { this.childNodes = value ? [new TextShim(value, this.ownerDocument)] : []; } addEventListener(): void {} removeEventListener(): void {} contains(node: NodeShim | null): boolean { return node === this || this.childNodes.some((child) => child.contains(node)); } }
class TextShim extends NodeShim { constructor(private value: string, document: DocumentShim) { super(3, "#text"); this.ownerDocument = document; } override get textContent(): string { return this.value; } override set textContent(value: string) { this.value = value; } }
class ElementShim extends NodeShim { readonly tagName: string; readonly namespaceURI = "http://www.w3.org/1999/xhtml"; readonly style: Record<string, string> = {}; private attrs = new Map<string, string>(); selected = false; constructor(name: string, document: DocumentShim) { super(1, name.toUpperCase()); this.tagName = name.toUpperCase(); this.ownerDocument = document; } get options(): ElementShim[] { return this.childNodes.filter((node): node is ElementShim => node instanceof ElementShim && node.tagName === "OPTION"); } setAttribute(name: string, value: string): void { this.attrs.set(name, value); } removeAttribute(name: string): void { this.attrs.delete(name); } getAttribute(name: string): string | null { return this.attrs.get(name) ?? null; } hasAttribute(name: string): boolean { return this.attrs.has(name); } }
class DocumentShim extends NodeShim { readonly documentElement: ElementShim; readonly body: ElementShim; readonly defaultView: Record<string, unknown>; activeElement: ElementShim | null; constructor() { super(9, "#document"); this.ownerDocument = this; this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement); this.activeElement = this.body; this.defaultView = { document: this, navigator: { userAgent: "node" } }; } createElement(name: string): ElementShim { return new ElementShim(name, this); } createElementNS(_space: string, name: string): ElementShim { return this.createElement(name); } createTextNode(value: string): TextShim { return new TextShim(value, this); } createComment(value: string): TextShim { return new TextShim(value, this); } }
function mount(): { root: Root; container: ElementShim } { const document = new DocumentShim(); const window = document.defaultView as Record<string, unknown>; Object.assign(window, { Node: NodeShim, Element: ElementShim, HTMLElement: ElementShim, HTMLIFrameElement: ElementShim, getSelection: () => null }); Object.assign(globalThis, { document, window, Node: NodeShim, Element: ElementShim, HTMLElement: ElementShim, HTMLIFrameElement: ElementShim, IS_REACT_ACT_ENVIRONMENT: true }); const container = document.createElement("div"); document.body.appendChild(container); return { root: createRoot(container as unknown as Element), container }; }
function all(node: NodeShim, name: string): ElementShim[] { return [node, ...node.childNodes.flatMap((child) => all(child, name))].filter((child): child is ElementShim => child instanceof ElementShim && child.tagName === name); }
function props<T extends Record<string, unknown>>(element: ElementShim): T { const key = Object.keys(element).find((name) => name.startsWith("__reactProps")); if (!key) throw new Error("missing react props"); return (element as unknown as Record<string, T>)[key]; }
function deferred<T>(): { promise: Promise<T>; resolve(value: T): void } { let resolve!: (value: T) => void; return { promise: new Promise<T>((done) => { resolve = done; }), resolve }; }

const csrf = "c".repeat(43);
const ref = { type: "phone" as const, scope: "phone:e164", value: "+8613800138000" };

describe("IdentityConsolePage", () => {
  it("keeps sales fail-closed and states the local-only boundary", () => {
    const client = { resolve: vi.fn(), bind: vi.fn() } as unknown as IdentityConsoleTransport;
    expect(renderToStaticMarkup(<IdentityConsolePage role="sales" transport={client} />)).toContain("没有本地身份管理权限");
    expect(client.resolve).not.toHaveBeenCalled(); expect(client.bind).not.toHaveBeenCalled();
    expect(renderToStaticMarkup(<IdentityConsolePage role="admin" transport={client} />)).toContain("不触发 Provider、外发或自动合并");
  });

  it("owns a same-tick local bind flight, clears raw draft only after a closed result, and sends no duplicate", async () => {
    const pending = deferred<{ status: number; data: unknown }>();
    const client = { resolve: vi.fn(async () => ({ status: 200, data: { status: "found", customer_id: 7 } })), bind: vi.fn(() => pending.promise) } as unknown as IdentityConsoleTransport;
    const mounted = mount();
    await act(async () => { mounted.root.render(<IdentityConsolePage role="admin" transport={client} readCookie={() => `aicrm_csrf=${csrf}`} />); });
    const fields = all(mounted.container, "INPUT");
    await act(async () => { props<{ onChange(event: { currentTarget: { value: string } }): void }>(fields[0]).onChange({ currentTarget: { value: ref.scope } }); props<{ onChange(event: { currentTarget: { value: string } }): void }>(fields[1]).onChange({ currentTarget: { value: ref.value } }); props<{ onChange(event: { currentTarget: { value: string } }): void }>(fields[2]).onChange({ currentTarget: { value: "7" } }); props<{ onChange(event: { currentTarget: { checked: boolean } }): void }>(fields[3]).onChange({ currentTarget: { checked: true } }); });
    const form = all(mounted.container, "FORM")[0];
    await act(async () => { const submit = props<{ onSubmit(event: { preventDefault(): void }): void }>(form).onSubmit; submit({ preventDefault() {} }); submit({ preventDefault() {} }); });
    expect(client.bind).toHaveBeenCalledTimes(1);
    expect(client.bind).toHaveBeenCalledWith({ customer_id: 7, ref }, expect.objectContaining({ credentials: "same-origin", headers: expect.objectContaining({ "X-CSRF-Token": csrf, "Idempotency-Key": expect.stringMatching(/^identity-bind-/) }) }));
    await act(async () => { pending.resolve({ status: 200, data: { status: "bound", customer_id: 7 } }); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("已绑定到本地客户 OneID：7");
    expect(props<{ value?: string }>(all(mounted.container, "INPUT")[1]).value).toBe("");
    await act(async () => { mounted.root.unmount(); });
  });

  it("locks a local write whose outcome is unknown and does not let an old pending response unlock a replacement", async () => {
    const oldRequest = deferred<{ status: number; data: unknown }>();
    const bind = vi.fn(() => oldRequest.promise);
    const client = { resolve: vi.fn(async () => ({ status: 503, data: {} })), bind } as unknown as IdentityConsoleTransport;
    const replacement = { resolve: vi.fn(async () => ({ status: 200, data: { status: "not_found" } })), bind: vi.fn() } as unknown as IdentityConsoleTransport;
    const mounted = mount();
    await act(async () => { mounted.root.render(<IdentityConsolePage role="ops" transport={client} readCookie={() => `aicrm_csrf=${csrf}`} />); });
    const fillAndSubmit = async () => { const inputs = all(mounted.container, "INPUT"); await act(async () => { props<{ onChange(event: { currentTarget: { value: string } }): void }>(inputs[0]).onChange({ currentTarget: { value: ref.scope } }); props<{ onChange(event: { currentTarget: { value: string } }): void }>(inputs[1]).onChange({ currentTarget: { value: ref.value } }); props<{ onChange(event: { currentTarget: { value: string } }): void }>(inputs[2]).onChange({ currentTarget: { value: "7" } }); props<{ onChange(event: { currentTarget: { checked: boolean } }): void }>(inputs[3]).onChange({ currentTarget: { checked: true } }); }); await act(async () => { props<{ onSubmit(event: { preventDefault(): void }): void }>(all(mounted.container, "FORM")[0]).onSubmit({ preventDefault() {} }); }); };
    await fillAndSubmit(); expect(bind).toHaveBeenCalledTimes(1);
    await act(async () => { mounted.root.render(<IdentityConsolePage role="ops" transport={replacement} readCookie={() => `aicrm_csrf=${csrf}`} />); });
    expect(mounted.container.textContent).toContain("绑定结果待确认");
    expect(all(mounted.container, "FORM")).toHaveLength(0);
    expect(replacement.bind).not.toHaveBeenCalled();
    await act(async () => { oldRequest.resolve({ status: 200, data: { status: "bound", customer_id: 7 } }); await Promise.resolve(); });
    expect(mounted.container.textContent).not.toContain("已绑定到本地客户 OneID：7");
    expect(replacement.bind).not.toHaveBeenCalled();
    await act(async () => { mounted.root.unmount(); });
  });

  it("releases an interrupted read flight so a replacement transport can resolve", async () => {
    const oldRequest = deferred<{ status: number; data: unknown }>();
    const first = { resolve: vi.fn(() => oldRequest.promise), bind: vi.fn() } as unknown as IdentityConsoleTransport;
    const replacement = { resolve: vi.fn(async () => ({ status: 200, data: { status: "found", customer_id: 8 } })), bind: vi.fn() } as unknown as IdentityConsoleTransport;
    const mounted = mount();
    await act(async () => { mounted.root.render(<IdentityConsolePage role="admin" transport={first} />); });
    const inputs = all(mounted.container, "INPUT");
    await act(async () => { props<{ onChange(event: { currentTarget: { value: string } }): void }>(inputs[0]).onChange({ currentTarget: { value: ref.scope } }); props<{ onChange(event: { currentTarget: { value: string } }): void }>(inputs[1]).onChange({ currentTarget: { value: ref.value } }); });
    await act(async () => { props<{ onClick(): void }>(all(mounted.container, "BUTTON")[0]).onClick(); });
    expect(first.resolve).toHaveBeenCalledTimes(1);
    await act(async () => { mounted.root.render(<IdentityConsolePage role="admin" transport={replacement} />); });
    await act(async () => { props<{ onClick(): void }>(all(mounted.container, "BUTTON")[0]).onClick(); await Promise.resolve(); });
    expect(replacement.resolve).toHaveBeenCalledTimes(1);
    expect(mounted.container.textContent).toContain("本地客户 OneID：8");
    await act(async () => { oldRequest.resolve({ status: 401, data: {} }); await Promise.resolve(); mounted.root.unmount(); });
  });
});
