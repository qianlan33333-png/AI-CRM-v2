/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  copyWecomTagID,
  startWecomTagGroupCreate,
  startWecomTagMutation,
  submitWecomTagCreate,
  submitWecomTagArchive,
  submitWecomTagGroupArchive,
  submitWecomTagGroupRename,
  WecomTagDetails,
  WecomTagGroupDetails,
  WecomTagExecutionGatePanel,
  WecomTagsPage,
  WecomTagsView,
} from "./wecom-tags-ui";
import type { WecomTagsTransport } from "./wecom-tags";

const catalog = {
  totalTags: 2,
  tagLimit: 1000,
  snapshotAt: "2026-08-19T00:00:00Z",
  groups: [
    {
      id: 1,
      name: "意向",
      sortOrder: 0,
      tags: [
        { id: 10, groupID: 1, groupName: "意向", name: "高意向", sortOrder: 0 },
        { id: 11, groupID: 1, groupName: "意向", name: "低意向", sortOrder: 1 },
      ],
    },
  ],
  tags: [
    { id: 10, groupID: 1, groupName: "意向", name: "高意向", sortOrder: 0 },
    { id: 11, groupID: 1, groupName: "意向", name: "低意向", sortOrder: 1 },
  ],
} as const;

function transport(): WecomTagsTransport {
  return {
    read: vi.fn(async () => ({ status: 503, data: {} })),
    executionGate: vi.fn(async () => ({ status: 503, data: {} })),
  } as unknown as WecomTagsTransport;
}

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
class TestText extends TestNode { constructor(private data: string, owner: TestDocument) { super(3, "#text"); this.ownerDocument = owner; } override get textContent(): string { return this.data; } override set textContent(value: string) { this.data = value; } }
class TestElement extends TestNode {
  readonly tagName: string; readonly namespaceURI = "http://www.w3.org/1999/xhtml"; readonly style: Record<string, string> = {}; private readonly attributes = new Map<string, string>();
  constructor(tag: string, owner: TestDocument) { super(1, tag.toUpperCase()); this.tagName = tag.toUpperCase(); this.ownerDocument = owner; }
  setAttribute(name: string, value: string): void { this.attributes.set(name, value); } removeAttribute(name: string): void { this.attributes.delete(name); } getAttribute(name: string): string | null { return this.attributes.get(name) ?? null; } hasAttribute(name: string): boolean { return this.attributes.has(name); }
}
class TestDocument extends TestNode {
  readonly nodeType = 9; readonly documentElement: TestElement; readonly body: TestElement; readonly defaultView: Record<string, unknown>; activeElement: TestElement | null;
  constructor() { super(9, "#document"); this.ownerDocument = this; this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement); this.activeElement = this.body; this.defaultView = { document: this, navigator: { userAgent: "node" } }; }
  createElement(tag: string): TestElement { return new TestElement(tag, this); } createElementNS(_namespace: string, tag: string): TestElement { return this.createElement(tag); } createTextNode(value: string): TestText { return new TestText(value, this); } createComment(value: string): TestText { return new TestText(value, this); }
}
function mountedRoot(): { readonly root: Root; readonly container: TestElement } {
  const document = new TestDocument(); const window = document.defaultView as Record<string, unknown>;
  Object.assign(window, { Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, getSelection: () => null });
  Object.assign(globalThis, { document, window, Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, IS_REACT_ACT_ENVIRONMENT: true });
  const container = document.createElement("div"); document.body.appendChild(container); return { root: createRoot(container as unknown as Element), container };
}
function deferred<T>(): { readonly promise: Promise<T>; resolve(value: T): void } { let resolve!: (value: T) => void; return { promise: new Promise<T>((done) => { resolve = done; }), resolve }; }

function rawTag(
  id: number,
  name: string,
  sortOrder: number,
  groupName: string,
) {
  return {
    tag_id: id,
    id,
    group_id: 1,
    group_name: groupName,
    tag_name: name,
    name,
    sort_order: sortOrder,
  };
}

function rawCatalog(groupName: string, childGroupName = groupName) {
  const tags = [
    rawTag(10, "高意向", 0, childGroupName),
    rawTag(11, "低意向", 1, groupName),
  ];
  return {
    ok: true,
    items: tags,
    tags,
    groups: [
      {
        group_id: 1,
        group_name: groupName,
        name: groupName,
        sort_order: 0,
        tags,
      },
    ],
    count: 2,
    total_tags: 2,
    tag_limit: 1000,
    synced_at: "2026-08-19T00:00:00Z",
    source_status: "local_catalog",
    read_model_status: "ready",
    route_owner: "ai_crm_next",
    fallback_used: false,
    real_external_call_executed: false,
    sync_executed: false,
    fixture_used: false,
  };
}

function rawCatalogWithCreatedTag() {
  const tags = [
    rawTag(10, "高意向", 0, "意向"),
    rawTag(11, "低意向", 1, "意向"),
    rawTag(12, "待跟进", 2, "意向"),
  ];
  return {
    ok: true,
    items: tags,
    tags,
    groups: [{ group_id: 1, group_name: "意向", name: "意向", sort_order: 0, tags }],
    count: 3,
    total_tags: 3,
    tag_limit: 1000,
    synced_at: "2026-08-19T00:00:00Z",
    source_status: "local_catalog",
    read_model_status: "ready",
    route_owner: "ai_crm_next",
    fallback_used: false,
    real_external_call_executed: false,
    sync_executed: false,
    fixture_used: false,
  };
}

function tagCreated() {
  return {
    ok: true,
    reason: "tag_created",
    source_status: "local_catalog",
    route_owner: "ai_crm_next",
    fallback_used: false,
    real_external_call_executed: false,
    sync_executed: false,
    fixture_used: false,
    dry_run: false,
    tag: { tag_id: 12, group_id: 1, group_name: "意向", tag_name: "待跟进", sort_order: 2 },
  };
}

function groupUpdated(groupName: string) {
  return {
    ok: true,
    reason: "group_updated",
    source_status: "local_catalog",
    route_owner: "ai_crm_next",
    fallback_used: false,
    real_external_call_executed: false,
    sync_executed: false,
    fixture_used: false,
    dry_run: false,
    group: { group_id: 1, group_name: groupName, sort_order: 0 },
  };
}

function groupArchived() {
  return {
    ok: true,
    reason: "group_archived",
    source_status: "local_catalog",
    route_owner: "ai_crm_next",
    fallback_used: false,
    real_external_call_executed: false,
    sync_executed: false,
    fixture_used: false,
    dry_run: false,
    group: { group_id: 1, group_name: "archived:1", sort_order: 0 },
  };
}

function rawArchivedCatalog() {
  return {
    ok: true,
    items: [],
    tags: [],
    groups: [],
    count: 0,
    total_tags: 0,
    tag_limit: 1000,
    synced_at: "2026-08-19T00:00:00Z",
    source_status: "local_catalog",
    read_model_status: "ready",
    route_owner: "ai_crm_next",
    fallback_used: false,
    real_external_call_executed: false,
    sync_executed: false,
    fixture_used: false,
  };
}

function groupRenameController(
  client: WecomTagsTransport,
  options: {
    readonly onUnauthenticated?: () => void;
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    readonly setCatalog?: (value: unknown) => void;
  } = {},
) {
  const mutationInFlight = { current: false };
  const mutationLocked = { current: false };
  const setRenaming = vi.fn();
  const setCatalog = vi.fn(options.setCatalog);
  const lockMutations = vi.fn(() => {
    mutationLocked.current = true;
  });
  return {
    controller: {
      transport: client,
      readCookie: () => `aicrm_csrf=${"c".repeat(43)}`,
      onUnauthenticated: options.onUnauthenticated,
      mutationInFlight,
      mutationLocked,
      lockMutations,
      setRenaming,
      setCatalog,
    },
    mutationInFlight,
    mutationLocked,
    lockMutations,
    setRenaming,
    setCatalog,
  };
}

function tagCreateController(
  client: WecomTagsTransport,
  onUnauthenticated?: () => void,
) {
  const mutationInFlight = { current: false };
  const mutationLocked = { current: false };
  const lockMutations = vi.fn(() => {
    mutationLocked.current = true;
  });
  return {
    controller: {
      transport: client,
      readCookie: () => `aicrm_csrf=${"c".repeat(43)}`,
      onUnauthenticated,
      mutationInFlight,
      mutationLocked,
      lockMutations,
      setCreatingTag: vi.fn(),
      setCatalog: vi.fn(),
    },
    mutationInFlight,
    mutationLocked,
    lockMutations,
  };
}

function groupArchiveController(
  client: WecomTagsTransport,
  onUnauthenticated?: () => void,
) {
  const mutationInFlight = { current: false };
  const mutationLocked = { current: false };
  const setArchiving = vi.fn();
  const setCatalog = vi.fn();
  const lockMutations = vi.fn(() => {
    mutationLocked.current = true;
  });
  return {
    controller: {
      transport: client,
      readCookie: () => `aicrm_csrf=${"c".repeat(43)}`,
      onUnauthenticated,
      mutationInFlight,
      mutationLocked,
      lockMutations,
      setArchiving,
      setCatalog,
    },
    mutationLocked,
    lockMutations,
    setArchiving,
    setCatalog,
  };
}

function tagArchiveController(
  client: WecomTagsTransport,
  onUnauthenticated?: () => void,
) {
  const mutationInFlight = { current: false };
  const mutationLocked = { current: false };
  const lockMutations = vi.fn(() => {
    mutationLocked.current = true;
  });
  return {
    controller: {
      transport: client,
      readCookie: () => `aicrm_csrf=${"c".repeat(43)}`,
      onUnauthenticated,
      mutationInFlight,
      mutationLocked,
      lockMutations,
      setArchiving: vi.fn(),
      setCatalog: vi.fn(),
    },
    mutationLocked,
    lockMutations,
  };
}

describe("WecomTagsView", () => {
  it.each(["admin", "ops"] as const)(
    "renders the read-only catalog for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <WecomTagsView role={role} state={{ kind: "ready", catalog }} />,
      );
      expect(html).toContain('<h1 id="app-title">企微标签目录</h1>');
      expect(html).toContain("标签总数");
      expect(html).toContain("标签上限");
      expect(html).toContain("本地目录快照时间（非企微同步）");
      expect(html).toContain("2026-08-19T00:00:00Z");
      expect(html).toContain("搜索标签组、标签名称或标签 ID");
      expect(html).toContain("高意向");
      expect(html).toContain("标签 ID");
      expect(html).toContain("上一页");
      expect(html).toContain("下一页");
      expect(html).toMatch(/<button[^>]*disabled=""[^>]*>上一页<\/button>/);
      expect(html).toMatch(/<button[^>]*disabled=""[^>]*>下一页<\/button>/);
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toMatch(/usage|sync|live/i);
      expect(html).not.toMatch(/csrf|provider|token|secret/i);
    },
  );

  it("clears the catalog for an unavailable result", () => {
    const html = renderToStaticMarkup(
      <WecomTagsView role="admin" state={{ kind: "error" }} />,
    );
    expect(html).toContain("企微标签目录暂不可用。");
    expect(html).not.toContain("高意向");
    expect(html).not.toContain("搜索标签组");
  });

  it("escapes tag text from the frozen catalog", () => {
    const html = renderToStaticMarkup(
      <WecomTagsView
        role="admin"
        state={{
          kind: "ready",
          catalog: {
            ...catalog,
            groups: [
              {
                ...catalog.groups[0],
                tags: [
                  {
                    ...catalog.groups[0].tags[0],
                    name: '<img src=x onerror="bad">',
                  },
                ],
              },
            ],
            tags: [
              {
                ...catalog.tags[0],
                name: '<img src=x onerror="bad">',
              },
            ],
          },
        }}
      />,
    );
    expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).not.toContain("<img");
  });

  it("keeps sales fail-closed without issuing a request", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <WecomTagsPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有企微标签目录访问权限。");
    expect(html).not.toContain("搜索标签组");
    expect(html).not.toContain("标签总数");
    expect(client.read).not.toHaveBeenCalled();
    expect(client.executionGate).not.toHaveBeenCalled();
  });

  it("renders the execution gate as a local observation without raw payload or provider success", () => {
    const html = renderToStaticMarkup(
      <WecomTagExecutionGatePanel
        role="admin"
        state={{
          kind: "ready",
          gate: {
            providerExecutionEligible: false,
            localCommandAcceptanceAvailable: true,
            localQueueAvailable: true,
            syncExecuted: false,
            observedAt: "2026-08-20T09:00:00Z",
            realExternalCallExecuted: false,
          },
        }}
        onRefresh={vi.fn()}
      />,
    );
    expect(html).toContain("本地标签执行前置状态");
    expect(html).toContain("不代表企微执行、送达或成功");
    expect(html).toContain("2026-08-20T09:00:00Z");
    expect(html).not.toMatch(/payload|mode|external_userid|unionid/i);
  });

  it("uses the mounted page's gate single-flight and drops an unmounted stale response", async () => {
    const pending = deferred<{ status: number; data: unknown }>();
    const executionGate = vi.fn(() => pending.promise);
    const client = {
      read: vi.fn(async () => ({ status: 503, data: {} })),
      executionGate,
    } as unknown as WecomTagsTransport;
    const onUnauthenticated = vi.fn();
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <WecomTagsPage role="ops" transport={client} onUnauthenticated={onUnauthenticated} />,
      );
      await Promise.resolve();
    });
    expect(executionGate).toHaveBeenCalledOnce();
    await act(async () => {
      mounted.root.render(
        <WecomTagsPage role="ops" transport={client} onUnauthenticated={onUnauthenticated} />,
      );
      await Promise.resolve();
    });
    expect(executionGate).toHaveBeenCalledOnce();
    await act(async () => {
      mounted.root.unmount();
      pending.resolve({ status: 401, data: {} });
      await Promise.resolve();
    });
    expect(onUnauthenticated).not.toHaveBeenCalled();
  });

  it("reports active catalog and gate authentication expiry only once", async () => {
    const client = {
      read: vi.fn(async () => ({ status: 401, data: {} })),
      executionGate: vi.fn(async () => ({ status: 401, data: {} })),
    } as unknown as WecomTagsTransport;
    const onUnauthenticated = vi.fn();
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <WecomTagsPage role="ops" transport={client} onUnauthenticated={onUnauthenticated} />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(client.read).toHaveBeenCalledOnce();
    expect(client.executionGate).toHaveBeenCalledOnce();
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    await act(async () => { mounted.root.unmount(); });
  });

  it("renders only the frozen tag detail fields and keeps text escaped", () => {
    const html = renderToStaticMarkup(
      <WecomTagDetails
        copyStatus="idle"
        onCopy={vi.fn()}
        tag={{
          id: 10,
          groupID: 1,
          name: '<img src=x onerror="bad">',
          groupName: "意向",
          sortOrder: 0,
        }}
      />,
    );
    expect(html).toContain("标签详情");
    expect(html).toContain("标签名称");
    expect(html).toContain("标签 ID");
    expect(html).toContain("标签组名称");
    expect(html).toContain("复制标签 ID");
    expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).not.toContain("<img");
    expect(html).not.toMatch(/usage_count|使用次数/i);
  });

  it("renders the local rename form only when the permitted catalog supplies a tag", () => {
    const rename = vi.fn(async () => ({
      status: "confirmed" as const,
      tag: catalog.tags[0],
    }));
    const admin = renderToStaticMarkup(
      <WecomTagDetails
        copyStatus="idle"
        onCopy={vi.fn()}
        onRename={rename}
        tag={{
          id: 10,
          groupID: 1,
          groupName: "意向",
          name: "高意向",
          sortOrder: 0,
        }}
      />,
    );
    expect(admin).toContain("本地标签名称");
    expect(admin).toContain("保存本地名称");
    expect(admin).not.toMatch(/sync|live|provider/i);

    const sales = renderToStaticMarkup(
      <WecomTagsView
        onRenameTag={rename}
        role="sales"
        state={{ kind: "ready", catalog }}
      />,
    );
    expect(sales).toContain("当前账号没有企微标签目录访问权限。");
    expect(sales).not.toContain("保存本地名称");
  });

  it("renders the local group rename form only inside the permitted catalog", () => {
    const rename = vi.fn(async () => ({
      status: "confirmed" as const,
      group: { id: 1, name: "意向阶段", sortOrder: 0 },
    }));
    const admin = renderToStaticMarkup(
      <WecomTagGroupDetails group={catalog.groups[0]} onRename={rename} />,
    );
    expect(admin).toContain("标签组详情");
    expect(admin).toContain("本地标签组名称");
    expect(admin).toContain("保存本地标签组名称");
    expect(admin).not.toMatch(/sync|live|provider/i);

    const sales = renderToStaticMarkup(
      <WecomTagsView
        onRenameGroup={rename}
        role="sales"
        state={{ kind: "ready", catalog }}
      />,
    );
    expect(sales).toContain("当前账号没有企微标签目录访问权限。");
    expect(sales).not.toContain("保存本地标签组名称");
  });

  it("renders only an explicit local archive confirmation control for permitted groups", () => {
    const archive = vi.fn(async () => ({
      status: "archived" as const,
      group: { id: 1, name: "archived:1", sortOrder: 0 },
    }));
    const html = renderToStaticMarkup(
      <WecomTagGroupDetails group={catalog.groups[0]} onArchive={archive} />,
    );
    expect(html).toContain("归档本地标签组");
    expect(html).not.toContain("确认归档本地标签组");
    expect(html).not.toMatch(/企微删除|sync|live|provider/i);
  });

  it("renders a separate explicit single-tag archive confirmation without external wording", () => {
    const archive = vi.fn(async () => ({
      status: "archived" as const,
      tag: catalog.tags[0],
    }));
    const html = renderToStaticMarkup(
      <WecomTagDetails
        copyStatus="idle"
        onArchive={archive}
        onCopy={vi.fn()}
        tag={catalog.tags[0]}
      />,
    );
    expect(html).toContain("归档本地标签");
    expect(html).not.toContain("确认归档本地标签");
    expect(html).not.toMatch(/sync|live|provider|企微删除/i);
  });

  it("renders an uncertain group rename as a read-only local form", () => {
    const html = renderToStaticMarkup(
      <WecomTagGroupDetails
        group={catalog.groups[0]}
        mutationLocked
        onRename={vi.fn(async () => undefined)}
      />,
    );
    expect(html).toMatch(/<fieldset disabled=""/);
    expect(html).toContain("保存本地标签组名称");
  });

  it("renders an uncertain rename as a read-only local form", () => {
    const html = renderToStaticMarkup(
      <WecomTagDetails
        copyStatus="idle"
        mutationLocked
        onCopy={vi.fn()}
        onRename={vi.fn(async () => undefined)}
        tag={{
          id: 10,
          groupID: 1,
          groupName: "意向",
          name: "高意向",
          sortOrder: 0,
        }}
      />,
    );
    expect(html).toMatch(/<fieldset disabled=""/);
    expect(html).toContain("保存本地名称");
  });

  it("reports copy success or failure once and leaves the displayed ID available for manual copy", async () => {
    const writeText = vi.fn(async () => undefined);
    await expect(copyWecomTagID(10, { writeText })).resolves.toBe("copied");
    expect(writeText).toHaveBeenCalledOnce();
    expect(writeText).toHaveBeenCalledWith("10");

    const failedWrite = vi.fn(async () => {
      throw new Error("denied");
    });
    await expect(copyWecomTagID(10, { writeText: failedWrite })).resolves.toBe(
      "failed",
    );
    expect(failedWrite).toHaveBeenCalledOnce();
    await expect(copyWecomTagID(10, undefined)).resolves.toBe("unavailable");

    const failed = renderToStaticMarkup(
      <WecomTagDetails
        copyStatus="failed"
        onCopy={vi.fn()}
        tag={{
          id: 10,
          groupID: 1,
          name: "高意向",
          groupName: "意向",
          sortOrder: 0,
        }}
      />,
    );
    expect(failed).toContain("<dd>10</dd>");
    expect(failed).toContain("复制失败，请手工复制上方标签 ID。");
  });
});

describe("local tag-group creation lock", () => {
  it("permits one same-tick submission and always releases its ref lock", async () => {
    const lock = { current: false };
    const execute = vi.fn(async () => undefined);
    const first = startWecomTagGroupCreate(lock, execute);
    const second = startWecomTagGroupCreate(lock, execute);
    expect(first).toBeInstanceOf(Promise);
    expect(second).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();
    await first;
    expect(lock.current).toBe(false);
  });

  it("releases the ref lock when a local failure escapes", async () => {
    const lock = { current: false };
    await expect(
      startWecomTagGroupCreate(lock, async () => {
        throw new Error("local failure");
      }),
    ).rejects.toThrow("local failure");
    expect(lock.current).toBe(false);
  });

  it("shares one same-tick mutation lock across create and rename paths", async () => {
    const lock = { current: false };
    let release: (() => void) | undefined;
    const first = startWecomTagGroupCreate(
      lock,
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    const second = startWecomTagMutation(lock, async () => "second");
    expect(first).toBeInstanceOf(Promise);
    expect(second).toBeUndefined();
    release?.();
    await expect(first).resolves.toBeUndefined();
    expect(lock.current).toBe(false);
  });
});

describe("WecomTagsPage additional-tag controller", () => {
  it("creates exactly one selected-group tag, then publishes only its confirmed catalog reread", async () => {
    const client = {
      read: vi.fn(async () => ({ status: 200, data: rawCatalogWithCreatedTag() })),
      createTag: vi.fn(async () => ({ status: 200, data: tagCreated() })),
    } as unknown as WecomTagsTransport;
    const fixture = tagCreateController(client);
    await expect(
      submitWecomTagCreate(fixture.controller, catalog.groups[0], " 待跟进 "),
    ).resolves.toMatchObject({
      status: "created",
      tag: { id: 12, groupID: 1, groupName: "意向", name: "待跟进" },
    });
    expect(client.createTag).toHaveBeenCalledOnce();
    expect(client.createTag).toHaveBeenCalledWith(
      { group_id: 1, group_name: "意向", tag_name: "待跟进" },
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.objectContaining({
          "X-CSRF-Token": "c".repeat(43),
          "Idempotency-Key": expect.stringMatching(/^[A-Za-z0-9_-]{43}$/),
        }),
      }),
    );
    expect(fixture.controller.setCatalog).toHaveBeenCalledOnce();
    expect(fixture.lockMutations).not.toHaveBeenCalled();
  });

  it("shares the same-tick lock and locks without replacing catalog after unconfirmed creation", async () => {
    let release:
      // eslint-disable-next-line no-unused-vars -- deferred resolver accepts the transport response.
      ((value: { status: number; data: unknown }) => void) | undefined;
    const client = {
      read: vi.fn(),
      createTag: vi.fn(
        () => new Promise<{ status: number; data: unknown }>((resolve) => { release = resolve; }),
      ),
    } as unknown as WecomTagsTransport;
    const fixture = tagCreateController(client);
    const first = submitWecomTagCreate(fixture.controller, catalog.groups[0], "待跟进");
    const second = submitWecomTagCreate(fixture.controller, catalog.groups[0], "待跟进");
    expect(second).toBeUndefined();
    expect(client.createTag).toHaveBeenCalledOnce();
    release?.({ status: 503, data: {} });
    await expect(first).resolves.toEqual({ status: "unknown" });
    expect(fixture.lockMutations).toHaveBeenCalledOnce();
    expect(fixture.controller.setCatalog).not.toHaveBeenCalled();
  });

  it("reports a 401 once and preserves the old catalog", async () => {
    const onUnauthenticated = vi.fn();
    const client = {
      read: vi.fn(),
      createTag: vi.fn(async () => ({ status: 401, data: {} })),
    } as unknown as WecomTagsTransport;
    const fixture = tagCreateController(client, onUnauthenticated);
    await expect(
      submitWecomTagCreate(fixture.controller, catalog.groups[0], "待跟进"),
    ).resolves.toEqual({ status: "unauthenticated" });
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(fixture.controller.setCatalog).not.toHaveBeenCalled();
  });
});

describe("WecomTagsPage group rename controller", () => {
  it("forwards the selected group and normalized input, then publishes only the confirmed catalog", async () => {
    const client = {
      read: vi.fn(async () => ({ status: 200, data: rawCatalog("意向阶段") })),
      renameGroup: vi.fn(async () => ({
        status: 200,
        data: groupUpdated("意向阶段"),
      })),
    } as unknown as WecomTagsTransport;
    const fixture = groupRenameController(client);

    await expect(
      submitWecomTagGroupRename(
        fixture.controller,
        catalog.groups[0],
        " 意向阶段 ",
      ),
    ).resolves.toEqual({
      status: "confirmed",
      group: { id: 1, name: "意向阶段", sortOrder: 0 },
    });

    expect(client.renameGroup).toHaveBeenCalledOnce();
    expect(client.renameGroup).toHaveBeenCalledWith(
      1,
      { group_name: "意向阶段" },
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.objectContaining({
          "X-CSRF-Token": "c".repeat(43),
          "Idempotency-Key": expect.stringMatching(/^[A-Za-z0-9_-]{43}$/),
        }),
      }),
    );
    expect(client.read).toHaveBeenCalledOnce();
    expect(fixture.setCatalog).toHaveBeenCalledOnce();
    expect(fixture.lockMutations).not.toHaveBeenCalled();
    expect(fixture.setRenaming).toHaveBeenNthCalledWith(1, true);
    expect(fixture.setRenaming).toHaveBeenLastCalledWith(false);
  });

  it("allows one same-tick PATCH and locks after an unconfirmed transport result without replacing the old catalog", async () => {
    let release:
      ((...args: [{ status: number; data: unknown }]) => void) | undefined; // eslint-disable-line no-unused-vars -- deferred resolver accepts a response.
    const client = {
      read: vi.fn(),
      renameGroup: vi.fn(
        () =>
          new Promise<{ status: number; data: unknown }>((resolve) => {
            release = resolve;
          }),
      ),
    } as unknown as WecomTagsTransport;
    const fixture = groupRenameController(client);

    const first = submitWecomTagGroupRename(
      fixture.controller,
      catalog.groups[0],
      "意向阶段",
    );
    const second = submitWecomTagGroupRename(
      fixture.controller,
      catalog.groups[0],
      "意向阶段",
    );
    expect(second).toBeUndefined();
    expect(client.renameGroup).toHaveBeenCalledOnce();

    release?.({ status: 503, data: {} });
    await expect(first).resolves.toEqual({ status: "unknown" });
    expect(fixture.mutationLocked.current).toBe(true);
    expect(fixture.lockMutations).toHaveBeenCalledOnce();
    expect(fixture.setCatalog).not.toHaveBeenCalled();
    expect(client.read).not.toHaveBeenCalled();
    expect(
      submitWecomTagGroupRename(
        fixture.controller,
        catalog.groups[0],
        "意向阶段",
      ),
    ).toBeUndefined();
    expect(client.renameGroup).toHaveBeenCalledOnce();
  });

  it("calls the 401 callback once and preserves the old catalog", async () => {
    const onUnauthenticated = vi.fn();
    const client = {
      read: vi.fn(),
      renameGroup: vi.fn(async () => ({ status: 401, data: {} })),
    } as unknown as WecomTagsTransport;
    const fixture = groupRenameController(client, { onUnauthenticated });

    await expect(
      submitWecomTagGroupRename(
        fixture.controller,
        catalog.groups[0],
        "意向阶段",
      ),
    ).resolves.toEqual({ status: "unauthenticated" });
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(client.read).not.toHaveBeenCalled();
    expect(fixture.setCatalog).not.toHaveBeenCalled();
    expect(fixture.mutationLocked.current).toBe(false);
  });

  it("preserves the old catalog for an invalid result and locks if the refreshed child tag projection drifts", async () => {
    const invalidClient = {
      read: vi.fn(),
      renameGroup: vi.fn(async () => ({ status: 400, data: {} })),
    } as unknown as WecomTagsTransport;
    const invalid = groupRenameController(invalidClient);
    await expect(
      submitWecomTagGroupRename(
        invalid.controller,
        catalog.groups[0],
        "意向阶段",
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(invalid.setCatalog).not.toHaveBeenCalled();
    expect(invalid.mutationLocked.current).toBe(false);

    const driftClient = {
      read: vi.fn(async () => ({
        status: 200,
        data: rawCatalog("意向阶段", "意向"),
      })),
      renameGroup: vi.fn(async () => ({
        status: 200,
        data: groupUpdated("意向阶段"),
      })),
    } as unknown as WecomTagsTransport;
    const drift = groupRenameController(driftClient);
    await expect(
      submitWecomTagGroupRename(
        drift.controller,
        catalog.groups[0],
        "意向阶段",
      ),
    ).resolves.toEqual({ status: "unknown" });
    expect(drift.setCatalog).not.toHaveBeenCalled();
    expect(drift.mutationLocked.current).toBe(true);
    expect(drift.lockMutations).toHaveBeenCalledOnce();
    expect(
      submitWecomTagGroupRename(
        drift.controller,
        catalog.groups[0],
        "意向阶段",
      ),
    ).toBeUndefined();
    expect(driftClient.renameGroup).toHaveBeenCalledOnce();
  });
});

describe("WecomTagsPage group archive controller", () => {
  it("sends one same-tick local archive, rereads the catalog, and publishes only after group and tags disappear", async () => {
    const client = {
      read: vi.fn(async () => ({ status: 200, data: rawArchivedCatalog() })),
      archiveGroup: vi.fn(async () => ({ status: 200, data: groupArchived() })),
    } as unknown as WecomTagsTransport;
    const fixture = groupArchiveController(client);

    await expect(
      submitWecomTagGroupArchive(fixture.controller, catalog.groups[0]),
    ).resolves.toEqual({
      status: "archived",
      group: { id: 1, name: "archived:1", sortOrder: 0 },
    });
    expect(client.archiveGroup).toHaveBeenCalledWith(
      1,
      {},
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.objectContaining({
          "X-CSRF-Token": "c".repeat(43),
          "Idempotency-Key": expect.stringMatching(/^[A-Za-z0-9_-]{43}$/),
        }),
      }),
    );
    expect(client.read).toHaveBeenCalledOnce();
    expect(fixture.setCatalog).toHaveBeenCalledOnce();
    expect(fixture.lockMutations).not.toHaveBeenCalled();
    expect(fixture.setArchiving).toHaveBeenNthCalledWith(1, true);
    expect(fixture.setArchiving).toHaveBeenLastCalledWith(false);
  });

  it("locks every write after uncertain archive outcomes but preserves the current catalog for deterministic failures", async () => {
    let release:
      ((...args: [{ status: number; data: unknown }]) => void) | undefined; // eslint-disable-line no-unused-vars -- deferred resolver accepts a response.
    const unknownClient = {
      read: vi.fn(),
      archiveGroup: vi.fn(
        () =>
          new Promise<{ status: number; data: unknown }>((resolve) => {
            release = resolve;
          }),
      ),
    } as unknown as WecomTagsTransport;
    const unknown = groupArchiveController(unknownClient);
    const first = submitWecomTagGroupArchive(
      unknown.controller,
      catalog.groups[0],
    );
    const second = submitWecomTagGroupArchive(
      unknown.controller,
      catalog.groups[0],
    );
    expect(second).toBeUndefined();
    release?.({ status: 503, data: {} });
    await expect(first).resolves.toEqual({ status: "unknown" });
    expect(unknown.mutationLocked.current).toBe(true);
    expect(unknown.lockMutations).toHaveBeenCalledOnce();
    expect(unknown.setCatalog).not.toHaveBeenCalled();
    expect(
      submitWecomTagGroupArchive(unknown.controller, catalog.groups[0]),
    ).toBeUndefined();

    const invalidClient = {
      read: vi.fn(),
      archiveGroup: vi.fn(async () => ({ status: 404, data: {} })),
    } as unknown as WecomTagsTransport;
    const invalid = groupArchiveController(invalidClient);
    await expect(
      submitWecomTagGroupArchive(invalid.controller, catalog.groups[0]),
    ).resolves.toEqual({ status: "invalid" });
    expect(invalid.mutationLocked.current).toBe(false);
    expect(invalid.setCatalog).not.toHaveBeenCalled();
  });

  it("calls the 401 callback once and does not reread after authentication expiry", async () => {
    const onUnauthenticated = vi.fn();
    const client = {
      read: vi.fn(),
      archiveGroup: vi.fn(async () => ({ status: 401, data: {} })),
    } as unknown as WecomTagsTransport;
    const fixture = groupArchiveController(client, onUnauthenticated);
    await expect(
      submitWecomTagGroupArchive(fixture.controller, catalog.groups[0]),
    ).resolves.toEqual({ status: "unauthenticated" });
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(client.read).not.toHaveBeenCalled();
    expect(fixture.setCatalog).not.toHaveBeenCalled();
  });
});

describe("WecomTagsPage tag archive controller", () => {
  it("single-flights archive, rereads before publishing, and locks an unconfirmed result", async () => {
    const receipt = {
      ok: true,
      reason: "tag_archived",
      source_status: "local_catalog",
      route_owner: "ai_crm_next",
      fallback_used: false,
      real_external_call_executed: false,
      sync_executed: false,
      fixture_used: false,
      dry_run: false,
      tag: {
        tag_id: 10,
        group_id: 1,
        group_name: "意向",
        tag_name: "高意向",
        sort_order: 0,
      },
    };
    const remaining = rawCatalog("意向");
    const nextTags = remaining.tags.filter((tag) => tag.tag_id !== 10);
    const client = {
      archiveTag: vi.fn(async () => ({ status: 200, data: receipt })),
      read: vi.fn(async () => ({
        status: 200,
        data: {
          ...remaining,
          items: nextTags,
          tags: nextTags,
          groups: [{ ...remaining.groups[0], tags: nextTags }],
          count: 1,
          total_tags: 1,
        },
      })),
    } as unknown as WecomTagsTransport;
    const fixture = tagArchiveController(client);
    const first = submitWecomTagArchive(fixture.controller, catalog.tags[0]);
    expect(
      submitWecomTagArchive(fixture.controller, catalog.tags[0]),
    ).toBeUndefined();
    await expect(first).resolves.toEqual({
      status: "archived",
      tag: catalog.tags[0],
    });
    expect(client.archiveTag).toHaveBeenCalledOnce();
    expect(client.read).toHaveBeenCalledOnce();

    const unknown = tagArchiveController({
      archiveTag: vi.fn(async () => ({ status: 503, data: {} })),
      read: vi.fn(),
    } as unknown as WecomTagsTransport);
    await expect(
      submitWecomTagArchive(unknown.controller, catalog.tags[0]),
    ).resolves.toEqual({ status: "unknown" });
    expect(unknown.mutationLocked.current).toBe(true);
    expect(unknown.lockMutations).toHaveBeenCalledOnce();
  });

  it("notifies one 401 without rereading or replacing the catalog", async () => {
    const onUnauthenticated = vi.fn();
    const fixture = tagArchiveController(
      {
        archiveTag: vi.fn(async () => ({ status: 401, data: {} })),
        read: vi.fn(),
      } as unknown as WecomTagsTransport,
      onUnauthenticated,
    );
    await expect(
      submitWecomTagArchive(fixture.controller, catalog.tags[0]),
    ).resolves.toEqual({ status: "unauthenticated" });
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(fixture.controller.transport.read).not.toHaveBeenCalled();
  });
});
