/* eslint-disable no-unused-vars -- minimal DOM shim fields are consumed by React DOM. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  CustomerDetailMissingState,
  CustomerDetailPage,
  parseProfileDraft,
  startCustomerMutation,
} from "./customer-detail-ui";
import type {
  CustomerDetailSnapshot,
  CustomerDetailTransport,
} from "./customer-detail";

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
    this.childNodes = value ? [new TestText(value, this.ownerDocument)] : [];
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
    private value: string,
    owner: TestDocument,
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

class TestElement extends TestNode {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style = {};
  private attributes = new Map<string, string>();
  constructor(tag: string, owner: TestDocument) {
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
  get options(): TestElement[] {
    return this.childNodes.filter(
      (node): node is TestElement =>
        node instanceof TestElement && node.tagName === "OPTION",
    );
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
  for (const [key, value] of Object.entries({
    document,
    window,
    Node: TestNode,
    Element: TestElement,
    HTMLElement: TestElement,
    HTMLIFrameElement: TestElement,
    IS_REACT_ACT_ENVIRONMENT: true,
  })) {
    vi.stubGlobal(key, value);
  }
  const container = document.createElement("div");
  document.body.appendChild(container);
  return {
    root: createRoot(container as unknown as globalThis.Element),
    container,
  };
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

function reactProps<T>(element: TestElement): T {
  const key = Object.keys(element).find((item) =>
    item.startsWith("__reactProps"),
  );
  if (!key) throw new Error("missing React props");
  return (element as unknown as Record<string, T>)[key];
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  return {
    promise: new Promise<T>((done) => {
      resolve = done;
    }),
    resolve,
  };
}

afterEach(() => vi.unstubAllGlobals());

const snapshot: CustomerDetailSnapshot = {
  customer: {
    id: 7,
    name: "林小姐",
    gender: 1,
    stageID: 3,
    ownerStaffID: 11,
    channelID: 5,
    addedAt: "2026-08-12T00:00:00Z",
    lastInteractAt: "2026-08-12T01:00:00Z",
    isDeleted: false,
    createdAt: "2026-08-11T00:00:00Z",
    updatedAt: "2026-08-12T02:00:00Z",
  },
  tags: [{ id: 9, groupName: "意向", name: "已报名", sortOrder: 10 }],
  tagCatalog: [
    { id: 9, groupName: "意向", name: "已报名", sortOrder: 10 },
    { id: 10, groupName: "意向", name: "待联系", sortOrder: 20 },
  ],
  events: [
    {
      id: 12,
      eventType: "stage_changed",
      actor: "后台账号 #1",
      occurredAt: "2026-08-12T03:00:00Z",
    },
  ],
  eventsHaveMore: true,
  eventsNextCursor: "next-page",
};

function transport(): CustomerDetailTransport {
  return {
    get: vi.fn(),
    update: vi.fn(),
    setStage: vi.fn(),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    getContactPolicy: vi.fn(),
    setContactPolicy: vi.fn(),
    clearContactPolicy: vi.fn(),
    listEvents: vi.fn(),
    listTags: vi.fn(),
  } as unknown as CustomerDetailTransport;
}

describe("CustomerDetailPage", () => {
  it("renders accessible detail, profile, stage, tag, and timeline controls", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={snapshot}
        transport={transport()}
      />,
    );
    expect(html).toContain('<h1 id="app-title">客户详情</h1>');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
    expect(html).toContain("资料操作");
    expect(html).toContain("阶段编号");
    expect(html).toContain("添加标签");
    expect(html).toContain("时间线");
    expect(html).toContain("后台账号 #1");
    expect(html).toContain("加载更多时间线");
    expect(html).toContain("<fieldset");
    expect(html).toContain("<label");
    expect(html).toContain('href="/customers"');
    expect(html).toContain('href="#customer-360-summary"');
    expect(html).toContain('id="customer-360-summary"');
    expect(html).not.toContain("aicrm_csrf");
    expect(html).not.toContain("X-CSRF-Token");
  });

  it.each(["admin", "ops"] as const)(
    "offers the current canonical OneID Campaign entry to %s",
    (role) => {
      const html = renderToStaticMarkup(
        <CustomerDetailPage
          customerID={7}
          initialSnapshot={snapshot}
          role={role}
          transport={transport()}
        />,
      );
      expect(html).toContain(
        'href="/admin/cloud-orchestrator/campaigns?source_kind=customer_selection&amp;source_id=7"',
      );
      expect(html).toContain("以 OneID 7 发起 Campaign");
    },
  );

  it("keeps the detail Campaign entry hidden for sales", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={snapshot}
        role="sales"
        transport={transport()}
      />,
    );
    expect(html).not.toContain("发起 Campaign");
  });

  it("renders the 404-safe missing state with an explicit return to the list", () => {
    const html = renderToStaticMarkup(<CustomerDetailMissingState />);
    expect(html).toContain('role="alert"');
    expect(html).toContain("客户不存在");
    expect(html).toContain('aria-label="客户读取导航"');
    expect(html).toContain('href="/customers"');
    expect(html).toContain("返回客户列表");
    expect(html).not.toContain("external_userid");
    expect(html).not.toContain("unionid");
    expect(html).not.toContain("手机号");
  });

  it("mounts the injected zero-body chat activity panel without transport during SSR", () => {
    const get = vi.fn();
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={snapshot}
        transport={transport()}
        chatActivityRole="ops"
        chatActivityTransport={{ get }}
      />,
    );
    expect(html).toContain("本地聊天活动");
    expect(html).toContain("不读取正文、身份值、媒体");
    expect(get).not.toHaveBeenCalled();
  });

  it("starts with an accessible loading status without requiring browser globals", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage customerID={7} transport={transport()} />,
    );
    expect(html).toContain("正在读取客户资料、标签和时间线");
    expect(html).toContain('role="status"');
    expect(html.match(/<h1\b/g)).toHaveLength(1);
  });

  it.each([
    "javascript:alert(1)",
    "data:text/plain,unsafe",
    "ftp://assets.invalid/a",
    "https:assets.invalid/a",
    "https://name:secret@assets.invalid/a",
  ])("rejects an unsafe profile avatar URL %s", (avatarURL) => {
    expect(
      parseProfileDraft({
        name: "林小姐",
        avatarURL,
        gender: "1",
        ownerStaffID: "11",
        channelID: "5",
      }),
    ).toBeUndefined();
  });

  it("rejects an out-of-range profile gender before transport", () => {
    expect(
      parseProfileDraft({
        name: "林小姐",
        avatarURL: "https://assets.invalid/avatar.png",
        gender: "32768",
        ownerStaffID: "11",
        channelID: "5",
      }),
    ).toBeUndefined();
  });

  it("renders empty tags and timeline as explicit server-backed states", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={{
          ...snapshot,
          tags: [],
          events: [],
          eventsHaveMore: false,
          eventsNextCursor: undefined,
        }}
        transport={transport()}
      />,
    );
    expect(html).toContain("暂无标签。");
    expect(html).toContain("暂无时间线记录。");
    expect(html).not.toContain("加载更多时间线");
  });

  it("does not offer timeline pagination without a server cursor", () => {
    const html = renderToStaticMarkup(
      <CustomerDetailPage
        customerID={7}
        initialSnapshot={{
          ...snapshot,
          eventsHaveMore: true,
          eventsNextCursor: undefined,
        }}
        transport={transport()}
      />,
    );
    expect(html).not.toContain("加载更多时间线");
  });
});

describe("customer mutation orchestration", () => {
  it("locks synchronously, rejects a duplicate, then refetches after success", async () => {
    const lock = { current: false };
    let release: () => void = () => {};
    const execute = vi.fn(
      () =>
        new Promise<{ status: "succeeded" }>((resolve) => {
          release = () => resolve({ status: "succeeded" });
        }),
    );
    const refetch = vi.fn(
      async () => ({ status: "loaded", snapshot }) as const,
    );

    const first = startCustomerMutation(lock, execute, refetch);
    expect(first).toBeInstanceOf(Promise);
    expect(lock.current).toBe(true);
    expect(startCustomerMutation(lock, execute, refetch)).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();

    release();
    await expect(first).resolves.toEqual({ status: "confirmed", snapshot });
    expect(refetch).toHaveBeenCalledOnce();
    expect(lock.current).toBe(false);
  });

  it("does not refetch a rejected write and keeps failed refetch distinct", async () => {
    const failedRefetch = vi.fn(
      async () => ({ status: "unavailable" }) as const,
    );
    await expect(
      startCustomerMutation(
        { current: false },
        async () => ({ status: "forbidden" }),
        failedRefetch,
      ),
    ).resolves.toEqual({ status: "mutation_failed", failure: "forbidden" });
    expect(failedRefetch).not.toHaveBeenCalled();

    await expect(
      startCustomerMutation(
        { current: false },
        async () => ({ status: "succeeded" }),
        failedRefetch,
      ),
    ).resolves.toEqual({ status: "unconfirmed", failure: "unavailable" });
  });

  it("releases the lock and fails closed when an operation throws", async () => {
    const lock = { current: false };
    await expect(
      startCustomerMutation(
        lock,
        async () => {
          throw new Error("sensitive transport detail");
        },
        vi.fn(),
      ),
    ).resolves.toEqual({ status: "mutation_failed", failure: "unavailable" });
    expect(lock.current).toBe(false);
  });
});

const policyData = {
  customer_id: 7,
  version: 3,
  policy_present: true,
  eligible: false,
  suppression_active: true,
  reason_code: "manual_opt_out",
  suppressed_until: null,
  local_only: true,
  provider_execution_eligible: false,
  real_external_call_executed: false,
  delivery_proven: false,
};
const emptyPolicyData = {
  ...policyData,
  version: 0,
  policy_present: false,
  eligible: true,
  suppression_active: false,
  reason_code: null,
};
const csrf = `aicrm_csrf=${"x".repeat(43)}`;
const detailData = {
  customer: {
    id: 7,
    name: "林小姐",
    is_deleted: false,
    extra: {},
    created_at: "2026-08-11T00:00:00Z",
    updated_at: "2026-08-12T02:00:00Z",
  },
  tags: [],
};

function policyTransport(overrides: Partial<CustomerDetailTransport> = {}) {
  return {
    ...transport(),
    get: vi.fn(async () => ({ status: 200, data: detailData })),
    listEvents: vi.fn(async () => ({
      status: 200,
      data: { items: [], next_cursor: null },
    })),
    listTags: vi.fn(async () => ({ status: 200, data: { items: [] } })),
    getContactPolicy: vi.fn(async () => ({ status: 200, data: policyData })),
    setContactPolicy: vi.fn(async () => ({ status: 200, data: policyData })),
    clearContactPolicy: vi.fn(async () => ({
      status: 200,
      data: emptyPolicyData,
    })),
    ...overrides,
  } as CustomerDetailTransport;
}

async function settle(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("CustomerDetailPage contact policy integration", () => {
  it("keeps sales from rendering or requesting policy facts", async () => {
    const client = policyTransport();
    const mounted = mount();
    await act(async () =>
      mounted.root.render(
        <CustomerDetailPage
          customerID={7}
          actorID={11}
          initialSnapshot={snapshot}
          role="sales"
          transport={client}
        />,
      ),
    );
    await settle();
    expect(mounted.container.textContent).not.toContain("本地客户触达策略");
    expect(client.getContactPolicy).not.toHaveBeenCalled();
    await act(async () => mounted.root.unmount());
  });

  it("sends one exact versioned set request despite a double click", async () => {
    const write = deferred<{ status: number; data: unknown }>();
    const client = policyTransport({
      setContactPolicy: vi.fn(() => write.promise),
    });
    const mounted = mount();
    await act(async () =>
      mounted.root.render(
        <CustomerDetailPage
          customerID={7}
          actorID={11}
          initialSnapshot={snapshot}
          role="ops"
          transport={client}
          readCookie={() => csrf}
          contactPolicyKeySource={{
            randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
          }}
        />,
      ),
    );
    await settle();
    const save = elements(mounted.container, "button").find(
      (element) => element.textContent === "保存本地策略",
    )!;
    await act(async () => {
      reactProps<{ onClick(): void }>(save).onClick();
      reactProps<{ onClick(): void }>(save).onClick();
    });
    expect(client.setContactPolicy).toHaveBeenCalledOnce();
    expect(client.setContactPolicy).toHaveBeenCalledWith(
      7,
      {
        expected_version: 3,
        reason_code: "manual_opt_out",
        suppressed_until: null,
      },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key":
            "customer-contact-policy:set:123e4567-e89b-42d3-a456-426614174000",
        },
      },
    );
    await act(async () => write.resolve({ status: 200, data: policyData }));
    expect(mounted.container.textContent).toContain("本地触达策略已保存");
    expect(mounted.container.textContent).toContain("不证明发送或送达");
    await act(async () => mounted.root.unmount());
  });

  it("uses expected_version zero for a new policy and replays only an unknown outcome", async () => {
    const write = vi
      .fn()
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({ status: 200, data: policyData });
    const client = policyTransport({
      getContactPolicy: vi.fn(async () => ({
        status: 200,
        data: emptyPolicyData,
      })),
      setContactPolicy: write,
    });
    const mounted = mount();
    await act(async () =>
      mounted.root.render(
        <CustomerDetailPage
          customerID={7}
          actorID={11}
          initialSnapshot={snapshot}
          role="ops"
          transport={client}
          readCookie={() => csrf}
          contactPolicyKeySource={{
            randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
          }}
        />,
      ),
    );
    await settle();
    const save = elements(mounted.container, "button").find(
      (element) => element.textContent === "保存本地策略",
    )!;
    await act(async () => reactProps<{ onClick(): void }>(save).onClick());
    expect(write).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ expected_version: 0 }),
      expect.anything(),
    );
    const replay = elements(mounted.container, "button").find(
      (element) => element.textContent === "精确重放原策略意图",
    )!;
    await act(async () => reactProps<{ onClick(): void }>(replay).onClick());
    expect(write).toHaveBeenCalledTimes(2);
    expect(
      (write.mock.calls[0][2].headers as Record<string, string>)[
        "Idempotency-Key"
      ],
    ).toBe(
      (write.mock.calls[1][2].headers as Record<string, string>)[
        "Idempotency-Key"
      ],
    );
    await act(async () => mounted.root.unmount());
  });

  it("refreshes after a conflict and ignores stale actor reads", async () => {
    const first = deferred<{ status: number; data: unknown }>();
    const second = deferred<{ status: number; data: unknown }>();
    let reads = 0;
    const getContactPolicy = vi.fn(() =>
      ++reads === 1 ? first.promise : second.promise,
    );
    const oldUnauthenticated = vi.fn();
    const currentUnauthenticated = vi.fn();
    const client = policyTransport({ getContactPolicy });
    const mounted = mount();
    await act(async () =>
      mounted.root.render(
        <CustomerDetailPage
          customerID={7}
          actorID={11}
          initialSnapshot={snapshot}
          role="admin"
          transport={client}
          onUnauthenticated={oldUnauthenticated}
        />,
      ),
    );
    await settle();
    await act(async () =>
      mounted.root.render(
        <CustomerDetailPage
          customerID={7}
          actorID={12}
          initialSnapshot={snapshot}
          role="admin"
          transport={client}
          onUnauthenticated={currentUnauthenticated}
        />,
      ),
    );
    await settle();
    await act(async () => first.resolve({ status: 401, data: {} }));
    expect(oldUnauthenticated).not.toHaveBeenCalled();
    await act(async () =>
      second.resolve({ status: 200, data: emptyPolicyData }),
    );
    expect(currentUnauthenticated).not.toHaveBeenCalled();
    expect(mounted.container.textContent).toContain("策略记录未设置");
    await act(async () => mounted.root.unmount());
  });

  it("clears the confirmation after a 409 and re-reads the latest fact", async () => {
    const getContactPolicy = vi
      .fn()
      .mockResolvedValueOnce({ status: 200, data: policyData })
      .mockResolvedValue({ status: 200, data: emptyPolicyData });
    const client = policyTransport({
      getContactPolicy,
      setContactPolicy: vi.fn(async () => ({ status: 409, data: {} })),
    });
    const mounted = mount();
    await act(async () =>
      mounted.root.render(
        <CustomerDetailPage
          customerID={7}
          actorID={11}
          initialSnapshot={snapshot}
          role="ops"
          transport={client}
          readCookie={() => csrf}
          contactPolicyKeySource={{
            randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
          }}
        />,
      ),
    );
    await settle();
    const save = elements(mounted.container, "button").find(
      (element) => element.textContent === "保存本地策略",
    )!;
    const readsBeforeConflict = getContactPolicy.mock.calls.length;
    await act(async () => reactProps<{ onClick(): void }>(save).onClick());
    expect(getContactPolicy.mock.calls.length).toBeGreaterThan(
      readsBeforeConflict,
    );
    expect(mounted.container.textContent).toContain("策略记录未设置");
    expect(mounted.container.textContent).toContain("版本已变化");
    expect(mounted.container.textContent).not.toContain(
      "我已确认清除当前本地策略",
    );
    await act(async () => mounted.root.unmount());
  });
});
