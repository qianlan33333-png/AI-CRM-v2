/* eslint-disable no-unused-vars -- compact DOM shim exposes React event props. */
import React, { act } from "react";
import { flushSync } from "react-dom";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CampaignTouchPlanReadTransport } from "./campaign-touch-plan-read";
import { CampaignTouchPlanReadPanel } from "./campaign-touch-plan-read-ui";

class Node {
  parentNode: Node | null = null;
  childNodes: Node[] = [];
  ownerDocument!: Document;
  constructor(
    readonly nodeType: number,
    readonly nodeName: string,
  ) {}
  appendChild(node: Node): Node {
    node.parentNode = this;
    this.childNodes.push(node);
    return node;
  }
  insertBefore(node: Node, before: Node | null): Node {
    if (!before) return this.appendChild(node);
    node.parentNode = this;
    this.childNodes.splice(this.childNodes.indexOf(before), 0, node);
    return node;
  }
  removeChild(node: Node): Node {
    this.childNodes.splice(this.childNodes.indexOf(node), 1);
    node.parentNode = null;
    return node;
  }
  get firstChild(): Node | null {
    return this.childNodes[0] ?? null;
  }
  get nextSibling(): Node | null {
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
    this.childNodes = value ? [new Text(value, this.ownerDocument)] : [];
  }
  addEventListener(): void {}
  removeEventListener(): void {}
  contains(node: Node | null): boolean {
    return (
      node === this || this.childNodes.some((child) => child.contains(node))
    );
  }
}
class Text extends Node {
  constructor(
    private value: string,
    owner: Document,
  ) {
    super(3, "#text");
    this.ownerDocument = owner;
  }
  override get textContent(): string {
    return this.value;
  }
  override set textContent(value: string) {
    this.value = value;
  }
}
class Element extends Node {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style = {};
  private attributes = new Map<string, string>();
  constructor(tag: string, owner: Document) {
    super(1, tag.toUpperCase());
    this.tagName = tag.toUpperCase();
    this.ownerDocument = owner;
  }
  get options(): Element[] {
    return this.childNodes.filter(
      (node): node is Element =>
        node instanceof Element && node.tagName === "OPTION",
    );
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
  getAttributeNames(): string[] {
    return [...this.attributes.keys()];
  }
  hasAttribute(name: string): boolean {
    return this.attributes.has(name);
  }
}
class Document extends Node {
  readonly documentElement: Element;
  readonly body: Element;
  readonly defaultView: Record<string, unknown>;
  activeElement: Element | null;
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
  createElement(tag: string): Element {
    return new Element(tag, this);
  }
  createElementNS(_namespace: string, tag: string): Element {
    return this.createElement(tag);
  }
  createTextNode(value: string): Text {
    return new Text(value, this);
  }
  createComment(value: string): Text {
    return new Text(value, this);
  }
}
function mount(): { root: Root; container: Element } {
  const document = new Document();
  const window = document.defaultView;
  Object.assign(window, {
    Node,
    Element,
    HTMLElement: Element,
    HTMLIFrameElement: Element,
    getSelection: () => null,
  });
  for (const [key, value] of Object.entries({
    document,
    window,
    Node,
    Element,
    HTMLElement: Element,
    HTMLIFrameElement: Element,
    IS_REACT_ACT_ENVIRONMENT: true,
  }))
    vi.stubGlobal(key, value);
  const container = document.createElement("div");
  document.body.appendChild(container);
  return {
    root: createRoot(container as unknown as globalThis.Element),
    container,
  };
}
function elements(root: Node, tag: string): Element[] {
  return [
    root,
    ...root.childNodes.flatMap((node) => elements(node, tag)),
  ].filter(
    (node): node is Element =>
      node instanceof Element && node.tagName === tag.toUpperCase(),
  );
}
function props<T extends Record<string, unknown>>(element: Element): T {
  const key = Object.keys(element).find((key) =>
    key.startsWith("__reactProps"),
  );
  if (!key) throw new Error("missing React props");
  return (element as unknown as Record<string, T>)[key];
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

const now = "2026-08-24T01:02:03.123456Z";
const digest = "a".repeat(64);
const safety = {
  local_only: true,
  provider_execution_eligible: false,
  runtime_executed: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const campaigns = {
  items: ["a", "b"].map((code) => ({
    campaign_code: code,
    name: code,
    approval_status: "draft",
    runtime_status: "idle",
    version: 1,
    created_by: 1,
    updated_by: 1,
    created_at: now,
    updated_at: now,
  })),
  local_projection: true,
  real_external_call_executed: false,
  real_send: false,
  runtime_executed: false,
};
const plan = (code: string) => ({
  id: `ctp_${code.repeat(64)}`,
  campaign_code: code,
  campaign_version: 1,
  source: {
    kind: "customer_selection",
    customer_selection: { id: "local_selection", version: "v1", digest },
  },
  target_count: 1,
  target_digest: digest,
  content_step_count: 1,
  content_digest: digest,
  owner_actor_id: 1,
  preview_exclusion_summary: {
    candidate_count: 1,
    active_customer_count: 1,
    inactive_excluded_count: 0,
    policy_excluded_count: 0,
  },
  created_at: now,
  ...safety,
});

afterEach(() => vi.unstubAllGlobals());
describe("CampaignTouchPlanReadPanel", () => {
  it("keeps a newer campaign selection when transport ignores the aborted older request", async () => {
    const a = deferred<{ status: number; data: unknown }>();
    const b = deferred<{ status: number; data: unknown }>();
    const transport: CampaignTouchPlanReadTransport = {
      listCampaigns: async () => ({ status: 200, data: campaigns }),
      getCampaign: async () => ({ status: 500, data: {} }),
      createPlan: async () => ({ status: 500, data: {} }),
      listPlans: (code) => (code === "a" ? a.promise : b.promise),
      getPlan: async () => ({ status: 500, data: {} }),
      listRecipients: async () => ({ status: 500, data: {} }),
    };
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <CampaignTouchPlanReadPanel actorID={1} transport={transport} />,
      );
    });
    const selector = elements(container, "select")[0];
    await act(async () => {
      props<{
        onChange: (event: { currentTarget: { value: string } }) => void;
      }>(selector).onChange({ currentTarget: { value: "a" } });
    });
    await act(async () => {
      props<{
        onChange: (event: { currentTarget: { value: string } }) => void;
      }>(selector).onChange({ currentTarget: { value: "b" } });
    });
    await act(async () => {
      b.resolve({
        status: 200,
        data: { items: [plan("b")], next_cursor: null, ...safety },
      });
    });
    await act(async () => {
      a.resolve({ status: 401, data: {} });
    });
    expect(container.textContent).toContain(plan("b").id);
    expect(container.textContent).not.toContain(plan("a").id);
    await act(async () => root.unmount());
  });

  it("ignores a stale actor 401 but reports the committed actor's 401", async () => {
    const first = deferred<{ status: number; data: unknown }>();
    const second = deferred<{ status: number; data: unknown }>();
    let calls = 0;
    const unauthenticated = vi.fn();
    const transport: CampaignTouchPlanReadTransport = {
      listCampaigns: () => (++calls === 1 ? first.promise : second.promise),
      getCampaign: async () => ({ status: 500, data: {} }),
      createPlan: async () => ({ status: 500, data: {} }),
      listPlans: async () => ({ status: 500, data: {} }),
      getPlan: async () => ({ status: 500, data: {} }),
      listRecipients: async () => ({ status: 500, data: {} }),
    };
    const { root } = mount();
    await act(async () => {
      root.render(
        <CampaignTouchPlanReadPanel
          actorID={1}
          transport={transport}
          onUnauthenticated={unauthenticated}
        />,
      );
    });
    await act(async () => {
      root.render(
        <CampaignTouchPlanReadPanel
          actorID={2}
          transport={transport}
          onUnauthenticated={unauthenticated}
        />,
      );
    });
    await act(async () => {
      first.resolve({ status: 401, data: {} });
    });
    expect(unauthenticated).not.toHaveBeenCalled();
    await act(async () => {
      second.resolve({ status: 401, data: {} });
    });
    expect(unauthenticated).toHaveBeenCalledTimes(1);
    await act(async () => root.unmount());
  });

  it("invalidates the old actor during a committed switch before passive cleanup", async () => {
    const first = deferred<{ status: number; data: unknown }>();
    const unauthenticated = vi.fn();
    let calls = 0;
    const transport: CampaignTouchPlanReadTransport = {
      listCampaigns: () =>
        ++calls === 1
          ? first.promise
          : Promise.resolve({ status: 200, data: campaigns }),
      getCampaign: async () => ({ status: 500, data: {} }),
      createPlan: async () => ({ status: 500, data: {} }),
      listPlans: async () => ({ status: 500, data: {} }),
      getPlan: async () => ({ status: 500, data: {} }),
      listRecipients: async () => ({ status: 500, data: {} }),
    };
    const { root } = mount();
    await act(async () => {
      root.render(
        <CampaignTouchPlanReadPanel
          actorID={1}
          transport={transport}
          onUnauthenticated={unauthenticated}
        />,
      );
    });
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", false);
    flushSync(() => {
      root.render(
        <CampaignTouchPlanReadPanel
          actorID={2}
          transport={transport}
          onUnauthenticated={unauthenticated}
        />,
      );
    });
    first.resolve({ status: 401, data: {} });
    await Promise.resolve();
    expect(unauthenticated).not.toHaveBeenCalled();
    await act(async () => root.unmount());
  });
});
