/* eslint-disable no-unused-vars -- compact DOM shim and deferred signatures expose structural fields. */
import React, { act } from "react";
import { flushSync } from "react-dom";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  campaignTouchPlanPendingStorageKey,
  type CampaignTouchPlanTransport,
  type SessionStorageLike,
  type TransportResponse,
} from "./campaign-touch-plan-core";
import { CampaignTouchPlanPanel } from "./cloud-orchestrator-ui";

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
    if (!before) return this.appendChild(node);
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
    owner: TestDocument,
  ) {
    super(3, "#text");
    this.ownerDocument = owner;
  }
  override get textContent(): string {
    return this.data;
  }
  override set textContent(value: string) {
    this.data = value;
  }
  get nodeValue(): string {
    return this.data;
  }
  set nodeValue(value: string) {
    this.data = value;
  }
}
class TestElement extends TestNode {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style: Record<string, string> = {};
  private attributes = new Map<string, string>();
  constructor(tag: string, owner: TestDocument) {
    super(1, tag.toUpperCase());
    this.tagName = tag.toUpperCase();
    this.ownerDocument = owner;
  }
  get options(): TestElement[] {
    return this.childNodes.filter(
      (node): node is TestElement =>
        node instanceof TestElement && node.tagName === "OPTION",
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
  createElement(tag: string): TestElement {
    return new TestElement(tag, this);
  }
  createElementNS(_namespace: string, tag: string): TestElement {
    return this.createElement(tag);
  }
  createTextNode(value: string): TestText {
    return new TestText(value, this);
  }
  createComment(value: string): TestText {
    return new TestText(value, this);
  }
}
function mount(): { root: Root; container: TestElement } {
  const document = new TestDocument();
  const window = document.defaultView;
  Object.assign(window, {
    Node: TestNode,
    Element: TestElement,
    HTMLElement: TestElement,
    HTMLIFrameElement: TestElement,
    getSelection: () => null,
  });
  for (const [name, value] of Object.entries({
    document,
    window,
    Node: TestNode,
    Element: TestElement,
    HTMLElement: TestElement,
    HTMLIFrameElement: TestElement,
    IS_REACT_ACT_ENVIRONMENT: true,
  }))
    vi.stubGlobal(name, value);
  const container = document.createElement("div");
  document.body.appendChild(container);
  return { root: createRoot(container as unknown as Element), container };
}
function elements(root: TestNode, tag: string): TestElement[] {
  return [
    root,
    ...root.childNodes.flatMap((node) => elements(node, tag)),
  ].filter(
    (node): node is TestElement =>
      node instanceof TestElement && node.tagName === tag.toUpperCase(),
  );
}
function props<T extends Record<string, unknown>>(element: TestElement): T {
  const key = Object.keys(element).find((item) =>
    item.startsWith("__reactProps"),
  );
  if (!key) throw new Error("missing React props");
  return (element as unknown as Record<string, T>)[key];
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, reject, resolve };
}

const now = "2026-08-24T01:02:03.123456Z";
const csrf = "c".repeat(43);
const campaignSafety = {
  local_projection: true,
  real_external_call_executed: false,
  real_send: false,
  runtime_executed: false,
};
const campaign = (code = "spring", version = 4) => ({
  campaign_code: code,
  name: `${code} Campaign`,
  approval_status: "draft",
  runtime_status: "idle",
  version,
  created_by: 7,
  updated_by: 7,
  created_at: now,
  updated_at: now,
});
const campaignDetail = (code = "spring", version = 4) => ({
  campaign: campaign(code, version),
  steps: [{ step_index: 1, delay_minutes: 0, content: "本地内容" }],
  ...campaignSafety,
});
const plan = (code = "spring", version = 4, actor = 7) => ({
  id: `ctp_${"a".repeat(64)}`,
  campaign_code: code,
  campaign_version: version,
  source: {
    kind: "customer_selection",
    customer_selection: {
      id: "local_selection",
      version: "v1",
      digest: "a".repeat(64),
    },
  },
  target_count: 1,
  target_digest: "a".repeat(64),
  content: {
    steps: [{ step_index: 1, delay_minutes: 0, content: "本地内容" }],
    content_digest: "b".repeat(64),
  },
  owner_actor_id: actor,
  preview_exclusion_summary: {
    candidate_count: 1,
    active_customer_count: 1,
    inactive_excluded_count: 0,
    policy_excluded_count: 0,
  },
  created_at: now,
  local_only: true,
  provider_execution_eligible: false,
  runtime_executed: false,
  real_external_call_executed: false,
  delivery_proven: false,
});
class MemoryStorage implements SessionStorageLike {
  readonly values = new Map<string, string>();
  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }
  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
  removeItem(key: string): void {
    this.values.delete(key);
  }
}
function client(
  overrides: Partial<CampaignTouchPlanTransport> = {},
): CampaignTouchPlanTransport {
  return {
    listCampaigns: vi.fn(async () => ({
      status: 200,
      data: { items: [campaign()], ...campaignSafety },
    })),
    getCampaign: vi.fn(async (code) => ({
      status: 200,
      data: campaignDetail(code),
    })),
    createPlan: vi.fn(async (code) => ({ status: 201, data: plan(code) })),
    getPlan: vi.fn(async (code) => ({ status: 200, data: plan(code) })),
    ...overrides,
  };
}
const keySource = (value = "123e4567-e89b-42d3-a456-426614174000") => ({
  randomUUID: () => value,
});
async function renderPanel(
  root: Root,
  transport: CampaignTouchPlanTransport,
  storage: SessionStorageLike,
  actorID = 7,
  keys = keySource(
    actorID === 7 ? undefined : "223e4567-e89b-42d3-a456-426614174000",
  ),
  sourceKind: "customer_selection" | "segment_members" = "customer_selection",
  sourceID = "7",
) {
  await act(async () => {
    root.render(
      <CampaignTouchPlanPanel
        sourceKind={sourceKind}
        sourceID={sourceID}
        actorID={actorID}
        transport={transport}
        sessionStorage={storage}
        readCookie={() => `aicrm_csrf=${csrf}`}
        keySource={keys}
      />,
    );
  });
}
async function selectCampaign(container: TestElement, value = "spring") {
  const select = elements(container, "select")[0];
  await act(async () => {
    props<{ onChange(event: { currentTarget: { value: string } }): void }>(
      select,
    ).onChange({ currentTarget: { value } });
  });
}
async function confirm(container: TestElement) {
  const checkbox = elements(container, "input")[0];
  await act(async () => {
    props<{ onChange(event: { currentTarget: { checked: boolean } }): void }>(
      checkbox,
    ).onChange({ currentTarget: { checked: true } });
  });
}
function button(container: TestElement, label: string): TestElement {
  const found = elements(container, "button").find((item) =>
    item.textContent.includes(label),
  );
  if (!found) throw new Error(`missing button ${label}`);
  return found;
}
function SuspendAfterPanel({ onRender }: { readonly onRender: () => void }): never {
  onRender();
  throw new Promise<never>(() => {});
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("CampaignTouchPlanPanel", () => {
  it("blocks an over-safe source before any request", async () => {
    const transport = client();
    const storage = new MemoryStorage();
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <CampaignTouchPlanPanel
          sourceKind="customer_selection"
          sourceID="9223372036854775807"
          actorID={7}
          transport={transport}
          sessionStorage={storage}
          readCookie={() => `aicrm_csrf=${csrf}`}
          keySource={keySource()}
        />,
      );
    });
    expect(container.textContent).toContain("BLOCKED_REDLINE");
    expect(transport.listCampaigns).not.toHaveBeenCalled();
    await act(async () => root.unmount());
  });

  it("guards a deferred double click and reports only a verified local draft", async () => {
    const created = deferred<TransportResponse>();
    const transport = client({ createPlan: vi.fn(() => created.promise) });
    const { root, container } = mount();
    await renderPanel(root, transport, new MemoryStorage());
    await selectCampaign(container);
    await confirm(container);
    const create = props<{ onClick(): void }>(
      button(container, "创建本地草稿"),
    );
    await act(async () => {
      create.onClick();
      create.onClick();
    });
    expect(transport.createPlan).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("正在创建");
    await act(async () => {
      created.resolve({ status: 201, data: plan() });
      await created.promise;
    });
    expect(container.textContent).toContain("本地触达草稿已创建");
    expect(container.textContent).toContain(
      "不表示 Outbound、Provider 调用、发送或送达",
    );
    await act(async () => root.unmount());
  });

  it("ignores a create response that arrives after the actor changes", async () => {
    const created = deferred<TransportResponse>();
    const transport = client({ createPlan: vi.fn(() => created.promise) });
    const storage = new MemoryStorage();
    const { root, container } = mount();
    await renderPanel(root, transport, storage);
    await selectCampaign(container);
    await confirm(container);
    await act(async () => {
      props<{ onClick(): void }>(button(container, "创建本地草稿")).onClick();
    });
    await renderPanel(root, transport, storage, 8);
    await act(async () => {
      created.resolve({ status: 201, data: plan() });
      await created.promise;
    });
    expect(container.textContent).not.toContain("本地触达草稿已创建");
    expect(transport.createPlan).toHaveBeenCalledTimes(1);
    await act(async () => root.unmount());
  });

  it("keeps the committed actor current when a replacement render is abandoned", async () => {
    const created = deferred<TransportResponse>();
    const transport = client({ createPlan: vi.fn(() => created.promise) });
    const storage = new MemoryStorage();
    const replacementRendered = vi.fn();
    const { root, container } = mount();
    const initial = (
      <React.Suspense fallback={null}>
        <CampaignTouchPlanPanel
          sourceKind="customer_selection"
          sourceID="7"
          actorID={7}
          transport={transport}
          sessionStorage={storage}
          readCookie={() => `aicrm_csrf=${csrf}`}
          keySource={keySource()}
        />
      </React.Suspense>
    );
    await act(async () => root.render(initial));
    await selectCampaign(container);
    await confirm(container);
    await act(async () => {
      props<{ onClick(): void }>(elements(container, "button")[0]).onClick();
    });
    expect(
      props<{ disabled: boolean }>(elements(container, "button")[0]).disabled,
    ).toBe(true);

    await act(async () => {
      React.startTransition(() => {
        root.render(
          <React.Suspense fallback={null}>
            <CampaignTouchPlanPanel
              sourceKind="segment_members"
              sourceID="8"
              actorID={8}
              transport={transport}
              sessionStorage={storage}
              readCookie={() => `aicrm_csrf=${csrf}`}
              keySource={keySource("223e4567-e89b-42d3-a456-426614174000")}
            />
            <SuspendAfterPanel onRender={replacementRendered} />
          </React.Suspense>,
        );
      });
      await Promise.resolve();
    });
    expect(replacementRendered).toHaveBeenCalledTimes(1);

    await act(async () => {
      created.resolve({ status: 201, data: plan() });
      await created.promise;
    });
    expect(
      props<{ disabled: boolean }>(elements(container, "button")[0]).disabled,
    ).toBe(false);
    await act(async () => root.unmount());
  });

  it("does not let an old identity list 401 call authentication or populate its replacement", async () => {
    const oldList = deferred<TransportResponse>();
    const replacementList = deferred<TransportResponse>();
    const onUnauthenticated = vi.fn();
    const listCampaigns = vi
      .fn()
      .mockImplementationOnce(() => oldList.promise)
      .mockImplementationOnce(() => replacementList.promise);
    const transport = client({ listCampaigns });
    const storage = new MemoryStorage();
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <CampaignTouchPlanPanel
          sourceKind="customer_selection"
          sourceID="7"
          actorID={7}
          transport={transport}
          sessionStorage={storage}
          readCookie={() => `aicrm_csrf=${csrf}`}
          keySource={keySource()}
          onUnauthenticated={onUnauthenticated}
        />,
      );
    });

    flushSync(() => {
      root.render(
        <CampaignTouchPlanPanel
          sourceKind="segment_members"
          sourceID="8"
          actorID={8}
          transport={transport}
          sessionStorage={storage}
          readCookie={() => `aicrm_csrf=${csrf}`}
          keySource={keySource("223e4567-e89b-42d3-a456-426614174000")}
          onUnauthenticated={onUnauthenticated}
        />,
      );
    });
    oldList.resolve({ status: 401, data: {} });
    await act(async () => {
      await oldList.promise;
      await Promise.resolve();
    });

    expect(onUnauthenticated).not.toHaveBeenCalled();
    expect(elements(container, "option")).toHaveLength(1);
    await act(async () => root.unmount());
  });

  it.each([
    { label: "401", stale: { status: 401, data: {} } },
    {
      label: "different 200",
      stale: {
        status: 200,
        data: { items: [campaign("stale", 9)], ...campaignSafety },
      },
    },
  ] as const)(
    "keeps the latest StrictMode list after a stale $label",
    async ({ stale }) => {
      const first = deferred<TransportResponse>();
      const second = deferred<TransportResponse>();
      const onUnauthenticated = vi.fn();
      const transport = client({
        listCampaigns: vi
          .fn()
          .mockImplementationOnce(() => first.promise)
          .mockImplementationOnce(() => second.promise),
      });
      const { root, container } = mount();
      await act(async () => {
        root.render(
          <React.StrictMode>
            <CampaignTouchPlanPanel
              sourceKind="customer_selection"
              sourceID="7"
              actorID={7}
              transport={transport}
              sessionStorage={new MemoryStorage()}
              readCookie={() => `aicrm_csrf=${csrf}`}
              keySource={keySource()}
              onUnauthenticated={onUnauthenticated}
            />
          </React.StrictMode>,
        );
      });
      second.resolve({
        status: 200,
        data: { items: [campaign("latest", 8)], ...campaignSafety },
      });
      await act(async () => {
        await second.promise;
        await Promise.resolve();
      });
      expect(
        props<{ value: string }>(elements(container, "option")[1]).value,
      ).toBe("latest");

      first.resolve(stale);
      await act(async () => {
        await first.promise;
        await Promise.resolve();
      });
      expect(onUnauthenticated).not.toHaveBeenCalled();
      expect(
        props<{ value: string }>(elements(container, "option")[1]).value,
      ).toBe("latest");
      await act(async () => root.unmount());
    },
  );

  it("does not let an old identity detail populate the committed replacement", async () => {
    const oldDetail = deferred<TransportResponse>();
    const transport = client({ getCampaign: vi.fn(() => oldDetail.promise) });
    const storage = new MemoryStorage();
    const { root, container } = mount();
    await renderPanel(root, transport, storage);
    await selectCampaign(container);

    flushSync(() => {
      root.render(
        <CampaignTouchPlanPanel
          sourceKind="segment_members"
          sourceID="8"
          actorID={8}
          transport={transport}
          sessionStorage={storage}
          readCookie={() => `aicrm_csrf=${csrf}`}
          keySource={keySource("223e4567-e89b-42d3-a456-426614174000")}
        />,
      );
    });
    oldDetail.resolve({ status: 200, data: campaignDetail() });
    await act(async () => {
      await oldDetail.promise;
      await Promise.resolve();
    });

    expect(elements(container, "dl")).toHaveLength(0);
    await act(async () => root.unmount());
  });

  it("clears an old ready selection before a new identity can create", async () => {
    const transport = client();
    const storage = new MemoryStorage();
    const { root, container } = mount();
    await renderPanel(root, transport, storage);
    await selectCampaign(container);
    await confirm(container);

    flushSync(() => {
      root.render(
        <CampaignTouchPlanPanel
          sourceKind="segment_members"
          sourceID="8"
          actorID={8}
          transport={transport}
          sessionStorage={storage}
          readCookie={() => `aicrm_csrf=${csrf}`}
          keySource={keySource("223e4567-e89b-42d3-a456-426614174000")}
        />,
      );
    });
    props<{ onClick(): void }>(elements(container, "button")[0]).onClick();

    expect(transport.createPlan).not.toHaveBeenCalled();
    await act(async () => root.unmount());
  });

  it.each(["final", "conflict"] as const)(
    "ignores a stale machine-only %s result",
    async (kind) => {
      const created = deferred<TransportResponse>();
      const readback = deferred<TransportResponse>();
      const firstTransport = client({
        createPlan: vi.fn(() => created.promise),
        getCampaign: vi.fn(() => readback.promise),
        getPlan: vi.fn(() => readback.promise),
      });
      const replacementTransport = client();
      const storage = new MemoryStorage();
      const { root, container } = mount();
      await renderPanel(root, firstTransport, storage);
      await selectCampaign(container);
      await confirm(container);
      await act(async () => {
        props<{ onClick(): void }>(elements(container, "button")[0]).onClick();
      });

      flushSync(() => {
        root.render(
          <CampaignTouchPlanPanel
            sourceKind="customer_selection"
            sourceID="7"
            actorID={7}
            transport={replacementTransport}
            sessionStorage={storage}
            readCookie={() => `aicrm_csrf=${csrf}`}
            keySource={keySource("223e4567-e89b-42d3-a456-426614174000")}
          />,
        );
      });
      created.resolve(
        kind === "final"
          ? { status: 201, data: plan() }
          : { status: 409, data: { code: "CONFLICT" } },
      );
      await act(async () => {
        await created.promise;
        await Promise.resolve();
      });
      readback.resolve(
        kind === "final"
          ? { status: 200, data: plan() }
          : { status: 200, data: campaignDetail("spring", 9) },
      );
      await act(async () => {
        await readback.promise;
        await Promise.resolve();
      });

      expect(
        props<{ checked: boolean }>(elements(container, "input")[0]).checked,
      ).toBe(false);
      expect(
        elements(container, "p").filter(
          (item) => item.getAttribute("role") === "alert",
        ),
      ).toHaveLength(0);
      await act(async () => root.unmount());
    },
  );

  it("does not mistake its committed identity during StrictMode setup", async () => {
    const created = deferred<TransportResponse>();
    const transport = client({ createPlan: vi.fn(() => created.promise) });
    const storage = new MemoryStorage();
    const { root, container } = mount();
    await act(async () => {
      root.render(
        <React.StrictMode>
          <CampaignTouchPlanPanel
            sourceKind="customer_selection"
            sourceID="7"
            actorID={7}
            transport={transport}
            sessionStorage={storage}
            readCookie={() => `aicrm_csrf=${csrf}`}
            keySource={keySource()}
          />
        </React.StrictMode>,
      );
    });
    await selectCampaign(container);
    await confirm(container);
    await act(async () => {
      props<{ onClick(): void }>(elements(container, "button")[0]).onClick();
      created.resolve({ status: 201, data: plan() });
      await created.promise;
    });
    expect(
      props<{ disabled: boolean }>(elements(container, "button")[0]).disabled,
    ).toBe(false);
    await act(async () => root.unmount());
  });

  it("treats a malformed 201 as unknown and never renders success", async () => {
    const transport = client({
      createPlan: vi.fn(async () => ({ status: 201, data: { ok: true } })),
    });
    const { root, container } = mount();
    await renderPanel(root, transport, new MemoryStorage());
    await selectCampaign(container);
    await confirm(container);
    await act(async () => {
      props<{ onClick(): void }>(button(container, "创建本地草稿")).onClick();
    });
    expect(container.textContent).toContain("结果未知");
    expect(container.textContent).not.toContain("本地触达草稿已创建");
    expect(transport.getPlan).not.toHaveBeenCalled();
    await act(async () => root.unmount());
  });

  it("ignores a late campaign detail and fails closed on malformed detail", async () => {
    const first = deferred<TransportResponse>();
    const second = deferred<TransportResponse>();
    const transport = client({
      listCampaigns: vi.fn(async () => ({
        status: 200,
        data: {
          items: [campaign("first"), campaign("second", 8)],
          ...campaignSafety,
        },
      })),
      getCampaign: vi.fn((code) =>
        code === "first" ? first.promise : second.promise,
      ),
    });
    const { root, container } = mount();
    await renderPanel(root, transport, new MemoryStorage());
    await selectCampaign(container, "first");
    await selectCampaign(container, "second");
    await act(async () => {
      second.resolve({ status: 200, data: campaignDetail("second", 8) });
      await second.promise;
    });
    expect(container.textContent).toContain("版本 8");
    await act(async () => {
      first.resolve({
        status: 200,
        data: { ...campaignDetail("first"), provider_result: "forbidden" },
      });
      await first.promise;
    });
    expect(container.textContent).toContain("版本 8");
    expect(container.textContent).toContain("代码second版本版本 8");
    expect(container.textContent).not.toContain("代码first版本版本 4");
    await selectCampaign(container, "first");
    expect(container.textContent).toContain(
      "Campaign 明细响应不符合安全合同，禁止创建",
    );
    expect(
      props<{ disabled: boolean }>(button(container, "创建本地草稿")).disabled,
    ).toBe(true);
    await act(async () => root.unmount());
  });

  it("requires a new confirmed intent after 409 refresh and exposes a redline block without replay", async () => {
    const conflict = deferred<TransportResponse>();
    const createPlan = vi
      .fn()
      .mockImplementationOnce(() => conflict.promise)
      .mockResolvedValueOnce({
        status: 409,
        data: { code: "BLOCKED_REDLINE" },
      });
    const getCampaign = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: campaignDetail() })
      .mockResolvedValue({
        status: 200,
        data: campaignDetail("spring", 9),
      });
    const transport = client({ createPlan, getCampaign });
    const uuids = [
      "123e4567-e89b-42d3-a456-426614174000",
      "223e4567-e89b-42d3-a456-426614174000",
    ];
    const { root, container } = mount();
    await renderPanel(root, transport, new MemoryStorage(), 7, {
      randomUUID: () => uuids.shift() ?? "",
    });
    await selectCampaign(container);
    await confirm(container);
    await act(async () => {
      props<{ onClick(): void }>(button(container, "创建本地草稿")).onClick();
    });
    await act(async () => {
      conflict.resolve({ status: 409, data: { code: "CONFLICT" } });
      await conflict.promise;
    });
    expect(container.textContent).toContain("Campaign 事实已刷新");
    expect(getCampaign).toHaveBeenCalledTimes(2);
    expect(
      props<{ checked: boolean }>(elements(container, "input")[0]).checked,
    ).toBe(false);
    expect(
      elements(container, "button").some((item) =>
        item.textContent.includes("精确重放"),
      ),
    ).toBe(false);

    await confirm(container);
    await act(async () => {
      props<{ onClick(): void }>(button(container, "创建本地草稿")).onClick();
    });
    expect(createPlan).toHaveBeenCalledTimes(2);
    expect(createPlan.mock.calls[1][1].expected_campaign_version).toBe(9);
    expect(createPlan.mock.calls[0][2].headers["Idempotency-Key"]).not.toBe(
      createPlan.mock.calls[1][2].headers["Idempotency-Key"],
    );
    expect(container.textContent).toContain("BLOCKED_REDLINE");
    expect(
      elements(container, "button").some((item) =>
        item.textContent.includes("精确重放"),
      ),
    ).toBe(false);
    expect(container.textContent).not.toContain("本地触达草稿已创建");
    await act(async () => root.unmount());
  });

  it("keeps an unknown outcome for exact replay and isolates it by actor", async () => {
    const storage = new MemoryStorage();
    const transport = client({
      createPlan: vi
        .fn()
        .mockRejectedValueOnce(new Error("network"))
        .mockResolvedValueOnce({ status: 201, data: plan() }),
    });
    let mounted = mount();
    await renderPanel(mounted.root, transport, storage);
    await selectCampaign(mounted.container);
    await confirm(mounted.container);
    await act(async () => {
      props<{ onClick(): void }>(
        button(mounted.container, "创建本地草稿"),
      ).onClick();
    });
    expect(mounted.container.textContent).toContain("结果未知");
    const firstPending = storage.getItem(campaignTouchPlanPendingStorageKey(7));
    expect(firstPending).not.toBeNull();
    await act(async () => mounted.root.unmount());

    mounted = mount();
    await renderPanel(mounted.root, transport, storage);
    await selectCampaign(mounted.container);
    await confirm(mounted.container);
    await act(async () => {
      props<{ onClick(): void }>(
        button(mounted.container, "创建本地草稿"),
      ).onClick();
    });
    expect(mounted.container.textContent).toContain("只允许精确重放");
    expect(transport.createPlan).toHaveBeenCalledTimes(1);
    await act(async () => {
      props<{ onClick(): void }>(
        button(mounted.container, "精确重放"),
      ).onClick();
    });
    expect(transport.createPlan).toHaveBeenCalledTimes(2);
    expect(
      (transport.createPlan as ReturnType<typeof vi.fn>).mock.calls[0][2],
    ).toEqual(
      (transport.createPlan as ReturnType<typeof vi.fn>).mock.calls[1][2],
    );
    expect(storage.getItem(campaignTouchPlanPendingStorageKey(7))).toBeNull();
    await act(async () => mounted.root.unmount());

    storage.setItem(campaignTouchPlanPendingStorageKey(7), firstPending!);
    const actorEight = client({
      createPlan: vi.fn(async (code) => ({
        status: 201,
        data: plan(code, 4, 8),
      })),
      getPlan: vi.fn(async (code) => ({ status: 200, data: plan(code, 4, 8) })),
    });
    mounted = mount();
    await renderPanel(mounted.root, actorEight, storage, 8);
    await selectCampaign(mounted.container);
    await confirm(mounted.container);
    await act(async () => {
      props<{ onClick(): void }>(
        button(mounted.container, "创建本地草稿"),
      ).onClick();
    });
    expect(mounted.container.textContent).toContain("本地触达草稿已创建");
    expect(storage.getItem(campaignTouchPlanPendingStorageKey(7))).toBe(
      firstPending,
    );
    await act(async () => mounted.root.unmount());
  });
});
