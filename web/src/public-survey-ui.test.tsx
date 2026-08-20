/* eslint-disable no-unused-vars -- local DOM shim exposes React host properties. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { PublicSurveyPage } from "./public-survey-ui";
import type { PublicDefinition, PublicSurveyTransport } from "./public-survey";

class NodeShim {
  parentNode: NodeShim | null = null; childNodes: NodeShim[] = []; ownerDocument!: DocumentShim;
  constructor(readonly nodeType: number, readonly nodeName: string) {}
  appendChild(node: NodeShim) { node.parentNode = this; this.childNodes.push(node); return node; }
  insertBefore(node: NodeShim, before: NodeShim | null) { if (!before) return this.appendChild(node); node.parentNode = this; this.childNodes.splice(this.childNodes.indexOf(before), 0, node); return node; }
  removeChild(node: NodeShim) { this.childNodes.splice(this.childNodes.indexOf(node), 1); node.parentNode = null; return node; }
  get firstChild() { return this.childNodes[0] ?? null; }
  get nextSibling(): NodeShim | null { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null; }
  get textContent() { return this.childNodes.map((node) => node.textContent).join(""); }
  set textContent(value: string) { this.childNodes = value === "" ? [] : [new TextShim(value, this.ownerDocument)]; }
  addEventListener() {} removeEventListener() {} contains(node: NodeShim | null): boolean { return node === this || this.childNodes.some((child) => child.contains(node)); }
}
class TextShim extends NodeShim { constructor(private data: string, owner: DocumentShim) { super(3, "#text"); this.ownerDocument = owner; } override get textContent() { return this.data; } override set textContent(value: string) { this.data = value; } }
class ElementShim extends NodeShim {
  readonly tagName: string; readonly namespaceURI = "http://www.w3.org/1999/xhtml"; readonly style: Record<string, string> = {}; private attrs = new Map<string, string>();
  constructor(tag: string, owner: DocumentShim) { super(1, tag.toUpperCase()); this.tagName = tag.toUpperCase(); this.ownerDocument = owner; }
  setAttribute(key: string, value: string) { this.attrs.set(key, value); } removeAttribute(key: string) { this.attrs.delete(key); } getAttribute(key: string) { return this.attrs.get(key) ?? null; } hasAttribute(key: string) { return this.attrs.has(key); }
}
class DocumentShim extends NodeShim {
  readonly documentElement: ElementShim; readonly body: ElementShim; readonly defaultView: Record<string, unknown>; activeElement: ElementShim | null;
  constructor() { super(9, "#document"); this.ownerDocument = this; this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement); this.activeElement = this.body; this.defaultView = { document: this, navigator: { userAgent: "node" } }; }
  createElement(tag: string) { return new ElementShim(tag, this); } createElementNS(_: string, tag: string) { return this.createElement(tag); } createTextNode(value: string) { return new TextShim(value, this); } createComment(value: string) { return new TextShim(value, this); }
}
function mount() { const document = new DocumentShim(); const window = document.defaultView as Record<string, unknown>; Object.assign(window, { Node: NodeShim, Element: ElementShim, HTMLElement: ElementShim, HTMLIFrameElement: ElementShim, getSelection: () => null }); Object.assign(globalThis, { document, window, Node: NodeShim, Element: ElementShim, HTMLElement: ElementShim, HTMLIFrameElement: ElementShim, IS_REACT_ACT_ENVIRONMENT: true }); const container = document.createElement("div"); document.body.appendChild(container); return { root: createRoot(container as unknown as Element), container }; }
function elements(root: NodeShim, tag: string): ElementShim[] { return [root, ...root.childNodes.flatMap((node) => elements(node, tag))].filter((node): node is ElementShim => node instanceof ElementShim && node.tagName === tag); }
function props<T extends Record<string, unknown>>(element: ElementShim): T { const key = Object.keys(element).find((candidate) => candidate.startsWith("__reactProps")); if (!key) throw new Error("missing props"); return (element as unknown as Record<string, T>)[key]; }
function deferred<T>() { let resolve!: (value: T) => void; return { promise: new Promise<T>((done) => { resolve = done; }), resolve }; }
const definition = (title: string): PublicDefinition => ({ id: 1, slug: "public-1", title, description: "", answer_display_mode: "all_in_one", version: 2, questions: [{ id: 11, type: "single_choice", title: "选择", required: true, sort_order: 0, minimum_selections: 1, maximum_selections: 1, options: [{ id: 21, option_text: "是", sort_order: 0 }] }] });

describe("PublicSurveyPage lifecycle", () => {
  it("drops old definition and pending submit responses after replacement or unmount", async () => {
    const loadA = deferred<PublicDefinition>(); const loadB = deferred<PublicDefinition>(); const submitA = deferred<{ receipt: { questionnaire_id: number; questionnaire_slug: string; definition_version: number; submission_id: number }; result_token: string }>();
    const transportA: PublicSurveyTransport = { definition: vi.fn(() => loadA.promise), submit: vi.fn(() => submitA.promise), result: vi.fn() };
    const transportB: PublicSurveyTransport = { definition: vi.fn(() => loadB.promise), submit: vi.fn(), result: vi.fn() };
    const mounted = mount();
    await act(async () => { mounted.root.render(<PublicSurveyPage slug="public-1" transport={transportA} />); await Promise.resolve(); });
    await act(async () => { mounted.root.render(<PublicSurveyPage slug="public-1" transport={transportB} />); loadB.resolve(definition("B")); await Promise.resolve(); await Promise.resolve(); });
    await act(async () => { loadA.resolve(definition("A")); await Promise.resolve(); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("B"); expect(mounted.container.textContent).not.toContain("A");
    await act(async () => mounted.root.unmount());

    const pending = mount(); const load = vi.fn().mockResolvedValue(definition("U")); const transport: PublicSurveyTransport = { definition: load, submit: vi.fn(() => submitA.promise), result: vi.fn() };
    await act(async () => { pending.root.render(<PublicSurveyPage slug="public-1" transport={transport} />); await Promise.resolve(); await Promise.resolve(); });
    const input = elements(pending.container, "INPUT")[0]; await act(async () => { props<{ onChange?: (event: { currentTarget: { checked: boolean } }) => void }>(input).onChange?.({ currentTarget: { checked: true } }); await Promise.resolve(); });
    const form = elements(pending.container, "FORM")[0]; await act(async () => { props<{ onSubmit?: (event: { preventDefault: () => void }) => void }>(form).onSubmit?.({ preventDefault: () => undefined }); await Promise.resolve(); pending.root.unmount(); submitA.resolve({ receipt: { questionnaire_id: 1, questionnaire_slug: "public-1", definition_version: 2, submission_id: 3 }, result_token: "a".repeat(43) }); await Promise.resolve(); await Promise.resolve(); });
    expect(transport.result).not.toHaveBeenCalled();
  });

  it("submits once per tick and keeps a confirmed 202 accepted when result lookup fails", async () => {
    const submit = vi.fn().mockResolvedValue({
      receipt: { questionnaire_id: 1, questionnaire_slug: "public-1", definition_version: 2, submission_id: 3 },
      result_token: "a".repeat(43),
    });
    const result = vi.fn().mockRejectedValue(new Error("result unavailable"));
    const transport: PublicSurveyTransport = {
      definition: vi.fn().mockResolvedValue(definition("Confirmed")),
      submit,
      result,
    };
    const mounted = mount();
    await act(async () => {
      mounted.root.render(<PublicSurveyPage slug="public-1" transport={transport} />);
      await Promise.resolve();
      await Promise.resolve();
    });
    const input = elements(mounted.container, "INPUT")[0];
    await act(async () => {
      props<{ onChange?: (event: { currentTarget: { checked: boolean } }) => void }>(input).onChange?.({ currentTarget: { checked: true } });
      await Promise.resolve();
    });
    const form = elements(mounted.container, "FORM")[0];
    const onSubmit = props<{ onSubmit?: (event: { preventDefault: () => void }) => void }>(form).onSubmit;
    await act(async () => {
      onSubmit?.({ preventDefault: () => undefined });
      onSubmit?.({ preventDefault: () => undefined });
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(submit).toHaveBeenCalledOnce();
    expect(result).toHaveBeenCalledOnce();
    expect(mounted.container.textContent).toContain("提交已受理");
    expect(mounted.container.textContent).toContain("请勿重复提交");
    expect(elements(mounted.container, "FORM")).toHaveLength(0);
    await act(async () => mounted.root.unmount());
  });
});
