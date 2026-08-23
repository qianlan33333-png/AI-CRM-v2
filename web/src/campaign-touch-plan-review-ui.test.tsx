/* eslint-disable no-unused-vars -- compact DOM shim exposes React event props. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";
import type { CampaignTouchPlanReviewTransport } from "./campaign-touch-plan-review";
import { CampaignTouchPlanReviewPanel } from "./campaign-touch-plan-review-ui";

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
  const key = Object.keys(element).find((item) =>
    item.startsWith("__reactProps"),
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

const at = "2026-08-24T01:02:03.123456Z";
const plan: TouchPlanSummary = {
  id: `ctp_${"a".repeat(64)}`,
  campaignCode: "campaign_a",
  campaignVersion: 1,
  source: { kind: "customer_selection", digest: "b".repeat(64) },
  targetCount: 1,
  targetDigest: "c".repeat(64),
  stepCount: 1,
  contentDigest: "d".repeat(64),
  immutable: "verified-c1",
};
const safety = {
  local_only: true,
  provider_execution_eligible: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const pending = {
  review: {
    status: "pending_review",
    version: 2,
    submitted_by_actor_id: 7,
    submitted_at: at,
  },
  ...safety,
};
const approved = {
  review: {
    ...pending.review,
    status: "approved",
    version: 3,
    reviewed_by_actor_id: 7,
    reviewed_at: at,
  },
  handoff: {
    status: "pending_outbound_acceptance",
    review_version: 3,
    created_at: at,
  },
  ...safety,
};
const sessionStorage = () => {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => void values.set(key, value),
    removeItem: (key: string) => void values.delete(key),
  };
};
const csrf = `aicrm_csrf=${"x".repeat(43)}`;

afterEach(() => vi.unstubAllGlobals());
describe("CampaignTouchPlanReviewPanel", () => {
  it("requires the exact decision phrase before POST and renders only the local handoff fact", async () => {
    const mutateReview = vi.fn(async () => ({ status: 200, data: approved }));
    const transport: CampaignTouchPlanReviewTransport = {
      getReview: vi.fn(async () => ({ status: 200, data: pending })),
      mutateReview,
    };
    const { root, container } = mount();
    await act(async () =>
      root.render(
        <CampaignTouchPlanReviewPanel
          role="ops"
          actorID={7}
          plan={plan}
          transport={transport}
          sessionStorage={sessionStorage()}
          readCookie={() => csrf}
          keySource={{
            randomUUID: () => "11111111-1111-4111-8111-111111111111",
          }}
        />,
      ),
    );
    const input = elements(container, "input")[0];
    const approve = elements(container, "button").find(
      (button) => button.textContent === "批准本地交接",
    )!;
    await act(async () => props<{ onClick(): void }>(approve).onClick());
    expect(mutateReview).not.toHaveBeenCalled();
    await act(async () =>
      props<{ onChange(event: { currentTarget: { value: string } }): void }>(
        input,
      ).onChange({ currentTarget: { value: `APPROVE ${plan.id}` } }),
    );
    await act(async () => props<{ onClick(): void }>(approve).onClick());
    expect(mutateReview).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("待 Outbound 接受");
    expect(container.textContent).not.toContain("已发送");
    await act(async () => root.unmount());
  });

  it("ignores aborted/stale actor callbacks and reports only the active 401", async () => {
    const first = deferred<{ status: number; data: unknown }>();
    const second = deferred<{ status: number; data: unknown }>();
    const signals: AbortSignal[] = [];
    let calls = 0;
    const transport: CampaignTouchPlanReviewTransport = {
      getReview: (_code, _id, options) => {
        signals.push(options.signal as AbortSignal);
        return ++calls === 1 ? first.promise : second.promise;
      },
      mutateReview: vi.fn(async () => ({ status: 401, data: {} })),
    };
    const unauthenticated = vi.fn();
    const { root, container } = mount();
    await act(async () =>
      root.render(
        <CampaignTouchPlanReviewPanel
          role="admin"
          actorID={7}
          plan={plan}
          transport={transport}
          onUnauthenticated={unauthenticated}
        />,
      ),
    );
    await act(async () =>
      root.render(
        <CampaignTouchPlanReviewPanel
          role="admin"
          actorID={8}
          plan={plan}
          transport={transport}
          onUnauthenticated={unauthenticated}
        />,
      ),
    );
    expect(signals[0].aborted).toBe(true);
    await act(async () => first.resolve({ status: 401, data: {} }));
    expect(unauthenticated).not.toHaveBeenCalled();
    await act(async () => second.resolve({ status: 200, data: pending }));
    expect(container.textContent).toContain("待人工审核");
    await act(async () => root.unmount());
  });

  it("invalidates stale plan mutation callbacks and reports an active mutation 401 only", async () => {
    const firstMutation = deferred<{ status: number; data: unknown }>();
    let mutationCalls = 0;
    const transport: CampaignTouchPlanReviewTransport = {
      getReview: vi.fn(async () => ({ status: 200, data: pending })),
      mutateReview: vi.fn(() =>
        ++mutationCalls === 1
          ? firstMutation.promise
          : Promise.resolve({ status: 401, data: {} }),
      ),
    };
    const unauthenticated = vi.fn();
    const common = {
      role: "ops" as const,
      actorID: 7,
      transport,
      sessionStorage: sessionStorage(),
      readCookie: () => csrf,
      keySource: {
        randomUUID: () =>
          mutationCalls === 0
            ? "11111111-1111-4111-8111-111111111111"
            : "22222222-2222-4222-8222-222222222222",
      },
      onUnauthenticated: unauthenticated,
    };
    const { root, container } = mount();
    await act(async () =>
      root.render(<CampaignTouchPlanReviewPanel {...common} plan={plan} />),
    );
    const confirm = async (selectedPlan: TouchPlanSummary) => {
      const input = elements(container, "input")[0];
      await act(async () =>
        props<{ onChange(event: { currentTarget: { value: string } }): void }>(
          input,
        ).onChange({ currentTarget: { value: `APPROVE ${selectedPlan.id}` } }),
      );
      const button = elements(container, "button").find(
        (item) => item.textContent === "批准本地交接",
      )!;
      await act(async () => props<{ onClick(): void }>(button).onClick());
    };
    await confirm(plan);
    const nextPlan = {
      ...plan,
      id: `ctp_${"e".repeat(64)}`,
      immutable: "next-c1",
    };
    await act(async () =>
      root.render(<CampaignTouchPlanReviewPanel {...common} plan={nextPlan} />),
    );
    await act(async () => firstMutation.resolve({ status: 401, data: {} }));
    expect(unauthenticated).not.toHaveBeenCalled();
    await confirm(nextPlan);
    expect(unauthenticated).toHaveBeenCalledTimes(1);
    await act(async () => root.unmount());
  });

  it("clears confirmation after a 409 GET refresh instead of swapping version or key", async () => {
    const mutateReview = vi.fn(async () => ({
      status: 409,
      data: { code: "CONFLICT" },
    }));
    const transport: CampaignTouchPlanReviewTransport = {
      getReview: vi.fn(async () => ({ status: 200, data: pending })),
      mutateReview,
    };
    const { root, container } = mount();
    await act(async () =>
      root.render(
        <CampaignTouchPlanReviewPanel
          role="admin"
          actorID={7}
          plan={plan}
          transport={transport}
          sessionStorage={sessionStorage()}
          readCookie={() => csrf}
          keySource={{
            randomUUID: () => "11111111-1111-4111-8111-111111111111",
          }}
        />,
      ),
    );
    const input = elements(container, "input")[0];
    await act(async () =>
      props<{ onChange(event: { currentTarget: { value: string } }): void }>(
        input,
      ).onChange({ currentTarget: { value: `APPROVE ${plan.id}` } }),
    );
    const button = elements(container, "button").find(
      (item) => item.textContent === "批准本地交接",
    )!;
    await act(async () => props<{ onClick(): void }>(button).onClick());
    expect(
      props<{ value: string }>(elements(container, "input")[0]).value,
    ).toBe("");
    expect(container.textContent).toContain("确认词已清空");
    expect(mutateReview).toHaveBeenCalledTimes(1);
    await act(async () => root.unmount());
  });

  it("allows admin and ops but denies sales without reading", async () => {
    for (const role of ["admin", "ops"] as const) {
      const getReview = vi.fn(async () => ({ status: 200, data: pending }));
      const { root, container } = mount();
      await act(async () =>
        root.render(
          <CampaignTouchPlanReviewPanel
            role={role}
            actorID={7}
            plan={plan}
            transport={{ getReview, mutateReview: vi.fn() }}
          />,
        ),
      );
      expect(container.textContent).toContain("触达计划人工审核");
      expect(getReview).toHaveBeenCalledTimes(1);
      await act(async () => root.unmount());
    }
    const getReview = vi.fn();
    const { root, container } = mount();
    await act(async () =>
      root.render(
        <CampaignTouchPlanReviewPanel
          role="sales"
          actorID={7}
          plan={plan}
          transport={{ getReview, mutateReview: vi.fn() }}
        />,
      ),
    );
    expect(container.textContent).toContain("没有人工审核权限");
    expect(getReview).not.toHaveBeenCalled();
    await act(async () => root.unmount());
  });
});
