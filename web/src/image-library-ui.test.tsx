/* eslint-disable no-unused-vars -- minimal DOM shim exposes React structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  handleSearchKeyDown,
  importDataURLThenReload,
  ImageLibraryPage,
  ImagePreviewPanel,
  deleteImageThenReload,
  saveImageEnabledThenReload,
  saveMetadataThenReload,
  startImageMetadataSave,
  uploadThenReload,
} from "./image-library-ui";
import type { ImageDetail, ImageLibraryTransport } from "./image-library";

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
function elements(root: TestNode, tagName: string): TestElement[] { return [root, ...root.childNodes.flatMap((node) => elements(node, tagName))].filter((node): node is TestElement => node instanceof TestElement && node.tagName === tagName); }
function formFields(root: TestNode): TestElement[] { return [root, ...root.childNodes.flatMap(formFields)].filter((node): node is TestElement => node instanceof TestElement && (node.tagName === "INPUT" || node.tagName === "TEXTAREA")); }
function reactProps<T extends Record<string, unknown>>(element: TestElement): T { const key = Object.keys(element).find((candidate) => candidate.startsWith("__reactProps")); if (key === undefined) throw new Error("mounted element is missing React props"); return (element as unknown as Record<string, T>)[key]; }
function deferred<T>(): { readonly promise: Promise<T>; resolve(value: T): void } { let resolve!: (value: T) => void; return { promise: new Promise<T>((done) => { resolve = done; }), resolve }; }

const CSRF_TOKEN = "b".repeat(43);

const uploadSuccess = {
  ok: true,
  item: {
    id: 12,
    name: "cover.png",
    file_name: "cover.png",
    file_size: 1024,
    mime_type: "image/png",
    width: 400,
    height: 300,
    description: "",
    tags: "",
    category: "",
    created_at: "2026-08-17T12:00:00Z",
    updated_at: "2026-08-17T12:00:00Z",
  },
  source_status: "local_upload",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  storage_adapter_mode: "postgresql",
  adapter_mode: "postgresql",
};
const dataURLCreateSuccess = {
  ok: true,
  item: {
    id: 13,
    name: "封面",
    file_name: "cover.png",
    file_size: 4,
    mime_type: "image/png",
    width: 1,
    height: 1,
    enabled: true,
    description: "",
    tags: [],
    category: "",
    created_at: "2026-08-20T12:00:00Z",
    updated_at: "2026-08-20T12:00:00Z",
  },
  source_status: "local_repository_write",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  storage_adapter_mode: "postgresql",
  adapter_mode: "postgresql",
};
const dataURLDraft = {
  dataURL: "data:image/png;base64,QUJDRA==",
  fileName: "cover.png",
  name: "封面",
  description: "",
  tags: "",
  category: "",
  enabled: true,
};
const metadataDraft = {
  name: "封面",
  description: "首页主图",
  tags: "活动",
  category: "banner",
};
const metadataSuccess = {
  ok: true,
  item: {
    id: 11,
    name: "封面",
    file_name: "cover.png",
    mime_type: "image/png",
    file_size: 1024,
    description: "首页主图",
    category: "banner",
    width: 400,
    height: 300,
    created_at: "2026-08-17T12:00:00Z",
    updated_at: "2026-08-19T12:00:00Z",
    content_type: "image/png",
    tags: ["活动"],
    enabled: true,
    source: "upload",
    source_url: "",
    thumb_media_id: "",
    thumb_media_id_expires_at: "",
    ai_metadata: {},
    thumb_160_url: "/api/admin/image-library/11/variants/thumb_160",
    thumb_320_url: "/api/admin/image-library/11/variants/thumb_320",
    thumb_url: "/api/admin/image-library/11/variants/thumb_320",
    preview_url: "/api/admin/image-library/11/variants/mobile_1080",
    mobile_1080_url: "/api/admin/image-library/11/variants/mobile_1080",
    large_1440_url: "/api/admin/image-library/11/variants/large_1440",
    original_url: "/api/admin/image-library/11/variants/original",
  },
  source_status: "local_repository_write",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  storage_adapter_mode: "postgresql",
  adapter_mode: "postgresql",
};
const deleteSuccess = {
  ok: true,
  deleted: true,
  hard_deleted: true,
  id: 11,
  references_cleared: { miniprograms_cleared: 0, campaign_steps_cleared: 0 },
  source_status: "local_delete",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  storage_adapter_mode: "postgresql",
  adapter_mode: "postgresql",
};

const detail: ImageDetail = {
  id: 11,
  name: "封面",
  fileName: "cover.png",
  mimeType: "image/png",
  fileSize: 1024,
  enabled: true,
  description: "首页主图",
  tags: ["活动"],
  category: "banner",
  width: 400,
  height: 300,
  createdAt: "2026-08-17T12:00:00Z",
  updatedAt: "2026-08-19T12:00:00Z",
  previewURL: "/api/admin/image-library/11/variants/mobile_1080",
  originalURL: "/api/admin/image-library/11/variants/original",
};

function transport(): ImageLibraryTransport {
  const unavailable = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(unavailable),
    detail: vi.fn(unavailable),
    facets: vi.fn(unavailable),
    upload: vi.fn(unavailable),
    create: vi.fn(unavailable),
    update: vi.fn(unavailable),
    remove: vi.fn(unavailable),
  } as unknown as ImageLibraryTransport;
}

const imageListItem = {
  id: 11, name: "封面", file_name: "cover.png", mime_type: "image/png", file_size: 1024,
  enabled: true, description: "", tags: [], category: "banner", width: 400, height: 300,
  created_at: "2026-08-17T12:00:00Z", updated_at: "2026-08-18T12:00:00Z",
  thumb_160_url: "/api/admin/image-library/11/variants/thumb_160",
  thumb_320_url: "/api/admin/image-library/11/variants/thumb_320",
  thumb_url: "/api/admin/image-library/11/variants/thumb_320",
  preview_url: "/api/admin/image-library/11/variants/mobile_1080",
  mobile_1080_url: "/api/admin/image-library/11/variants/mobile_1080",
  large_1440_url: "/api/admin/image-library/11/variants/large_1440",
  original_url: "/api/admin/image-library/11/variants/original",
};
const localFlags = {
  route_owner: "ai_crm_next", fallback_used: false, real_external_call_executed: false,
  storage_adapter_mode: "postgresql", adapter_mode: "postgresql",
};
const imageListPage = {
  ok: true, items: [imageListItem], total: 1, limit: 24, offset: 0, count: 1, has_more: false, next_offset: null,
  source_status: "next_media_library", ...localFlags,
};
const imageFacetPage = {
  ok: true, categories: ["banner"], tags: [], source_status: "next_media_library", ...localFlags,
};

describe("ImageLibraryPage shell", () => {
  it.each(["admin", "ops"] as const)(
    "renders the complete browse/filter/upload shell for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <ImageLibraryPage role={role} transport={transport()} />,
      );
      expect(html).toContain('<h1 id="app-title">图片素材库</h1>');
      expect(html).toContain(
        "本页仅证明本地素材元数据和上传结果，不证明",
      );
      expect(html).toContain("图片列表");
      expect(html).toContain("搜索图片");
      expect(html).toContain("标签筛选");
      expect(html).toContain("分类筛选");
      expect(html).toContain("仅看未标注");
      expect(html).toContain("包含已停用");
      expect(html).toContain("上传图片");
      expect(html).toContain("粘贴 data URL 本地导入");
      expect(html).toContain("图片文件");
      expect(html).toContain("名称（可选）");
      expect(html).toContain("描述（可选）");
      expect(html).toContain("accept=\"image/png,image/jpeg,image/gif\"");
      expect(html).toContain('role="status"');
      expect(html).toContain('type="submit"');
      expect(html).not.toMatch(/<button(?![^>]*type=)[^>]*>/);
      // While the list request is in flight every filter control that would
      // immediately issue a new request must be disabled (SSR renders the
      // initial loading state).
      expect(html).toContain("<select disabled=\"\">");
      expect(html).toMatch(/<input type="checkbox" disabled=""/);
      expect(html).toMatch(/<button type="submit" disabled="">搜索<\/button>/);
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html.match(/<form\b/g)).toHaveLength(3);
      expect(html).not.toContain("aicrm_csrf");
      expect(html).not.toContain("<img");
    },
  );

  it("renders with the real default transport as well", () => {
    const html = renderToStaticMarkup(<ImageLibraryPage role="admin" />);
    expect(html).toContain('<h1 id="app-title">图片素材库</h1>');
    expect(html).toContain("正在读取图片素材");
  });

  it("keeps sales fail-closed without data, upload controls, or requests", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <ImageLibraryPage role="sales" transport={client} />,
    );
    expect(html).toContain('<h1 id="app-title">图片素材库</h1>');
    expect(html).toContain("当前账号没有图片素材库访问权限。");
    expect(html).toContain('role="alert"');
    expect(html).not.toContain("搜索图片");
    expect(html).not.toContain("图片列表");
    expect(html).not.toContain("上传图片");
    expect(html).not.toContain("<form");
    expect(html).not.toContain("<input");
    expect(client.list).not.toHaveBeenCalled();
    expect(client.facets).not.toHaveBeenCalled();
    expect(client.upload).not.toHaveBeenCalled();
    expect(client.update).not.toHaveBeenCalled();
    expect(client.remove).not.toHaveBeenCalled();
  });

  it("keeps Enter in search isolated from the upload form", () => {
    const preventDefault = vi.fn();
    const stopPropagation = vi.fn();
    const search = vi.fn();

    handleSearchKeyDown(
      { key: "Enter", preventDefault, stopPropagation },
      search,
    );
    expect(preventDefault).toHaveBeenCalledOnce();
    expect(stopPropagation).toHaveBeenCalledOnce();
    expect(search).toHaveBeenCalledOnce();

    for (const key of ["a", "Escape", "Tab"]) {
      const other = { key, preventDefault: vi.fn(), stopPropagation: vi.fn() };
      handleSearchKeyDown(other, search);
      expect(other.preventDefault).not.toHaveBeenCalled();
      expect(other.stopPropagation).not.toHaveBeenCalled();
    }
    expect(search).toHaveBeenCalledTimes(1);
  });
});

describe("ImageLibraryPage data URL import state machine", () => {
  function importForm(root: TestElement): TestElement {
    const forms = elements(root, "FORM");
    const form = forms[2];
    if (!form) throw new Error("data URL import form is missing");
    return form;
  }
  async function fillImport(root: TestElement): Promise<void> {
    const values = [
      dataURLDraft.dataURL,
      dataURLDraft.fileName,
      dataURLDraft.name,
    ];
    for (const [index, value] of values.entries()) {
      await act(async () => {
        reactProps<{ onChange?: (event: { currentTarget: { value: string } }) => void }>(formFields(importForm(root))[index]).onChange?.({ currentTarget: { value } });
      });
    }
  }
  function submit(root: TestElement): void {
    reactProps<{ onSubmit?: (event: { preventDefault(): void }) => void }>(importForm(root)).onSubmit?.({ preventDefault() {} });
  }

  it("mounts one same-tick import, clears the data URL before reload, and lets sales issue no request", async () => {
    const initial = deferred<{ status: number; data: unknown }>();
    const importPending = deferred<{ status: number; data: unknown }>();
    const list = vi.fn(() => list.mock.calls.length === 1 ? initial.promise : Promise.resolve({ status: 200, data: imageListPage }));
    const facets = vi.fn(async () => ({ status: 200, data: imageFacetPage }));
    const create = vi.fn(() => importPending.promise);
    const client = { list, facets, detail: vi.fn(async () => ({ status: 503, data: {} })), upload: vi.fn(), create, update: vi.fn(), remove: vi.fn() } as unknown as ImageLibraryTransport;
    const mounted = mountedRoot();
    await act(async () => { mounted.root.render(<ImageLibraryPage role="admin" readCookie={() => `aicrm_csrf=${CSRF_TOKEN}`} transport={client} />); });
    await act(async () => { initial.resolve({ status: 200, data: imageListPage }); await Promise.resolve(); });
    await fillImport(mounted.container);
    await act(async () => { submit(mounted.container); submit(mounted.container); await Promise.resolve(); });
    expect(create).toHaveBeenCalledOnce();
    await act(async () => { importPending.resolve({ status: 200, data: dataURLCreateSuccess }); await Promise.resolve(); await Promise.resolve(); });
    expect(mounted.container.textContent).toContain("图片已导入为本地素材");
    expect(reactProps<{ value?: string }>(formFields(importForm(mounted.container))[0]).value).toBe("");
    expect(list).toHaveBeenCalledTimes(2);
    expect(facets).toHaveBeenCalledTimes(2);
    await act(async () => { mounted.root.unmount(); });

    const sales = mountedRoot();
    await act(async () => { sales.root.render(<ImageLibraryPage role="sales" transport={client} />); await Promise.resolve(); });
    expect(list).toHaveBeenCalledTimes(2);
    expect(facets).toHaveBeenCalledTimes(2);
    expect(create).toHaveBeenCalledOnce();
    await act(async () => { sales.root.unmount(); });
  });

  it("keeps the draft and locks on an unknown result, while an active 401 is reported once", async () => {
    for (const status of [503, 401]) {
      const create = vi.fn(async () => ({ status, data: {} }));
      const onUnauthenticated = vi.fn();
      const client = {
        list: vi.fn(async () => ({ status: 200, data: imageListPage })),
        facets: vi.fn(async () => ({ status: 200, data: imageFacetPage })),
        detail: vi.fn(async () => ({ status: 503, data: {} })), upload: vi.fn(), create, update: vi.fn(), remove: vi.fn(),
      } as unknown as ImageLibraryTransport;
      const mounted = mountedRoot();
      await act(async () => { mounted.root.render(<ImageLibraryPage role="ops" readCookie={() => `aicrm_csrf=${CSRF_TOKEN}`} onUnauthenticated={onUnauthenticated} transport={client} />); await Promise.resolve(); });
      await fillImport(mounted.container);
      await act(async () => { submit(mounted.container); await Promise.resolve(); });
      expect(create).toHaveBeenCalledOnce();
      expect(reactProps<{ value?: string }>(formFields(importForm(mounted.container))[0]).value).toBe(dataURLDraft.dataURL);
      if (status === 503) {
        expect(mounted.container.textContent).toContain("本地导入结果未知");
        await act(async () => { submit(mounted.container); await Promise.resolve(); });
        expect(create).toHaveBeenCalledOnce();
      } else {
        expect(onUnauthenticated).toHaveBeenCalledOnce();
      }
      await act(async () => { mounted.root.unmount(); });
    }
  });

  it("invalidates a pending import on transport replacement or unmount without replaying its effects", async () => {
    const pendingA = deferred<{ status: number; data: unknown }>();
    const clientA = {
      list: vi.fn(async () => ({ status: 200, data: imageListPage })),
      facets: vi.fn(async () => ({ status: 200, data: imageFacetPage })),
      detail: vi.fn(async () => ({ status: 503, data: {} })),
      upload: vi.fn(),
      create: vi.fn(() => pendingA.promise),
      update: vi.fn(),
      remove: vi.fn(),
    } as unknown as ImageLibraryTransport;
    const clientB = {
      list: vi.fn(async () => ({ status: 200, data: imageListPage })),
      facets: vi.fn(async () => ({ status: 200, data: imageFacetPage })),
      detail: vi.fn(async () => ({ status: 503, data: {} })),
      upload: vi.fn(),
      create: vi.fn(async () => ({ status: 200, data: dataURLCreateSuccess })),
      update: vi.fn(),
      remove: vi.fn(),
    } as unknown as ImageLibraryTransport;
    const mounted = mountedRoot();
    const staleUnauthenticated = vi.fn();
    await act(async () => {
      mounted.root.render(
        <ImageLibraryPage
          role="admin"
          readCookie={() => `aicrm_csrf=${CSRF_TOKEN}`}
          onUnauthenticated={staleUnauthenticated}
          transport={clientA}
        />,
      );
      await Promise.resolve();
    });
    await fillImport(mounted.container);
    await act(async () => {
      submit(mounted.container);
      await Promise.resolve();
    });
    expect(clientA.create).toHaveBeenCalledOnce();

    await act(async () => {
      mounted.root.render(
        <ImageLibraryPage
          role="admin"
          readCookie={() => `aicrm_csrf=${CSRF_TOKEN}`}
          transport={clientB}
        />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("本地导入结果未知");
    expect(reactProps<{ value?: string }>(formFields(importForm(mounted.container))[0]).value).toBe(dataURLDraft.dataURL);

    await act(async () => {
      pendingA.resolve({ status: 200, data: dataURLCreateSuccess });
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(reactProps<{ value?: string }>(formFields(importForm(mounted.container))[0]).value).toBe(dataURLDraft.dataURL);
    expect(clientB.list).toHaveBeenCalledOnce();
    expect(clientB.facets).toHaveBeenCalledOnce();
    await act(async () => {
      submit(mounted.container);
      await Promise.resolve();
    });
    expect(clientB.create).not.toHaveBeenCalled();
    expect(staleUnauthenticated).not.toHaveBeenCalled();
    await act(async () => {
      mounted.root.unmount();
    });

    const pendingUnmount = deferred<{ status: number; data: unknown }>();
    const unmountedClient = {
      list: vi.fn(async () => ({ status: 200, data: imageListPage })),
      facets: vi.fn(async () => ({ status: 200, data: imageFacetPage })),
      detail: vi.fn(async () => ({ status: 503, data: {} })),
      upload: vi.fn(),
      create: vi.fn(() => pendingUnmount.promise),
      update: vi.fn(),
      remove: vi.fn(),
    } as unknown as ImageLibraryTransport;
    const afterUnmountUnauthenticated = vi.fn();
    const unmounted = mountedRoot();
    await act(async () => {
      unmounted.root.render(
        <ImageLibraryPage
          role="ops"
          readCookie={() => `aicrm_csrf=${CSRF_TOKEN}`}
          onUnauthenticated={afterUnmountUnauthenticated}
          transport={unmountedClient}
        />,
      );
      await Promise.resolve();
    });
    await fillImport(unmounted.container);
    await act(async () => {
      submit(unmounted.container);
      await Promise.resolve();
    });
    expect(unmountedClient.create).toHaveBeenCalledOnce();
    await act(async () => {
      unmounted.root.unmount();
      pendingUnmount.resolve({ status: 401, data: {} });
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(afterUnmountUnauthenticated).not.toHaveBeenCalled();
    expect(unmountedClient.list).toHaveBeenCalledOnce();
    expect(unmountedClient.facets).toHaveBeenCalledOnce();
  });
});

describe("ImagePreviewPanel", () => {
  it("keeps the default preview local and switches only to the validated original URL", () => {
    const standard = renderToStaticMarkup(
      <ImagePreviewPanel
        image={detail}
        mode="standard"
        onSelectMode={vi.fn()}
        onPreviewError={vi.fn()}
      />,
    );
    expect(standard).toContain(
      'src="/api/admin/image-library/11/variants/mobile_1080"',
    );
    expect(standard).toContain("查看原图");
    expect(standard).not.toContain("<a");
    expect(standard).not.toContain("href=");

    const original = renderToStaticMarkup(
      <ImagePreviewPanel
        image={detail}
        mode="original"
        errorMode="original"
        onSelectMode={vi.fn()}
        onPreviewError={vi.fn()}
      />,
    );
    expect(original).toContain(
      'src="/api/admin/image-library/11/variants/original"',
    );
    expect(original).toContain("原图本地预览未能加载");
    expect(original).not.toContain("https://");
    expect(original).not.toContain("href=");
  });
});

describe("upload then reload flow", () => {
  const file = { type: "image/png", size: 1024 } as Blob;
  const metadata = { name: "", description: "", tags: "", category: "" };
  const key = "image-upload-flow-0000000000";

  it("reloads exactly once after a confirmed upload", async () => {
    const client = transport();
    vi.mocked(client.upload).mockResolvedValue({
      status: 200,
      data: uploadSuccess,
    } as Awaited<ReturnType<ImageLibraryTransport["upload"]>>);
    const reload = vi.fn();

    const result = await uploadThenReload({
      transport: client,
      cookie: `aicrm_csrf=${CSRF_TOKEN}`,
      file,
      metadata,
      idempotencyKey: key,
      reload,
    });

    expect(result.status).toBe("uploaded");
    expect(client.upload).toHaveBeenCalledTimes(1);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("never reloads and never retries after failure or unknown outcome", async () => {
    for (const status of [400, 401, 403, 409, 422, 500]) {
      const client = transport();
      vi.mocked(client.upload).mockResolvedValue({ status, data: {} } as Awaited<
        ReturnType<ImageLibraryTransport["upload"]>
      >);
      const reload = vi.fn();
      const result = await uploadThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        file,
        metadata,
        idempotencyKey: key,
        reload,
      });
      expect(result.status, String(status)).not.toBe("uploaded");
      expect(client.upload).toHaveBeenCalledTimes(1);
      expect(reload).not.toHaveBeenCalled();
    }

    const throwing = transport();
    vi.mocked(throwing.upload).mockRejectedValue(new Error("network down"));
    const reload = vi.fn();
    const result = await uploadThenReload({
      transport: throwing,
      cookie: `aicrm_csrf=${CSRF_TOKEN}`,
      file,
      metadata,
      idempotencyKey: key,
      reload,
    });
    expect(result).toEqual({ status: "unavailable" });
    expect(throwing.upload).toHaveBeenCalledTimes(1);
    expect(reload).not.toHaveBeenCalled();
  });

  it("sends nothing and skips reload when the CSRF cookie is missing", async () => {
    const client = transport();
    const reload = vi.fn();
    const result = await uploadThenReload({
      transport: client,
      cookie: "other=1",
      file,
      metadata,
      idempotencyKey: key,
      reload,
    });
    expect(result).toEqual({ status: "csrf_missing" });
    expect(client.upload).not.toHaveBeenCalled();
    expect(reload).not.toHaveBeenCalled();
  });

  it("sends nothing and skips reload when the file fails client checks", async () => {
    const client = transport();
    const reload = vi.fn();
    const result = await uploadThenReload({
      transport: client,
      cookie: `aicrm_csrf=${CSRF_TOKEN}`,
      file: { type: "image/webp", size: 10 } as Blob,
      metadata,
      idempotencyKey: key,
      reload,
    });
    expect(result).toEqual({ status: "invalid" });
    expect(client.upload).not.toHaveBeenCalled();
    expect(reload).not.toHaveBeenCalled();
  });
});

describe("data URL import then reload flow", () => {
  it("clears the sensitive input before the confirmed list and facet reload", async () => {
    const client = transport();
    vi.mocked(client.create).mockResolvedValue({
      status: 200,
      data: dataURLCreateSuccess,
    } as unknown as Awaited<ReturnType<ImageLibraryTransport["create"]>>);
    const calls: string[] = [];
    await expect(
      importDataURLThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        draft: dataURLDraft,
        idempotencyKey: "image-data-url-import-flow-000000",
        isCurrent: () => true,
        clearConfirmedDataURL: () => calls.push("clear"),
        reload: () => calls.push("reload"),
      }),
    ).resolves.toMatchObject({ status: "created", image: { id: 13 } });
    expect(calls).toEqual(["clear", "reload"]);
    expect(client.create).toHaveBeenCalledOnce();
  });

  it("keeps the sensitive draft and does not reload for a failed or malformed result", async () => {
    for (const result of [
      { status: 503, data: {} },
      { status: 200, data: { ...dataURLCreateSuccess, item: { ...dataURLCreateSuccess.item, extra: true } } },
    ]) {
      const client = transport();
      vi.mocked(client.create).mockResolvedValue(result as Awaited<
        ReturnType<ImageLibraryTransport["create"]>
      >);
      const clear = vi.fn();
      const reload = vi.fn();
      await expect(
        importDataURLThenReload({
          transport: client,
          cookie: `aicrm_csrf=${CSRF_TOKEN}`,
          draft: dataURLDraft,
          idempotencyKey: "image-data-url-import-flow-000000",
          isCurrent: () => true,
          clearConfirmedDataURL: clear,
          reload,
        }),
      ).resolves.toEqual({ status: "unavailable" });
      expect(clear).not.toHaveBeenCalled();
      expect(reload).not.toHaveBeenCalled();
      expect(client.create).toHaveBeenCalledOnce();
    }
  });

  it("does not clear or reload a confirmed response after its lifetime became stale", async () => {
    const client = transport();
    vi.mocked(client.create).mockResolvedValue({
      status: 200,
      data: dataURLCreateSuccess,
    } as unknown as Awaited<ReturnType<ImageLibraryTransport["create"]>>);
    const clear = vi.fn();
    const reload = vi.fn();
    await expect(
      importDataURLThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        draft: dataURLDraft,
        idempotencyKey: "image-data-url-import-flow-000000",
        isCurrent: () => false,
        clearConfirmedDataURL: clear,
        reload,
      }),
    ).resolves.toMatchObject({ status: "created", image: { id: 13 } });
    expect(clear).not.toHaveBeenCalled();
    expect(reload).not.toHaveBeenCalled();
  });
});

describe("metadata save then reload flow", () => {
  it("reloads once after the strict local metadata result", async () => {
    const client = transport();
    vi.mocked(client.update).mockResolvedValue({
      status: 200,
      data: metadataSuccess,
    } as Awaited<ReturnType<ImageLibraryTransport["update"]>>);
    const reload = vi.fn();

    await expect(
      saveMetadataThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        imageID: 11,
        draft: metadataDraft,
        reload,
      }),
    ).resolves.toMatchObject({ status: "saved", image: { id: 11 } });
    expect(client.update).toHaveBeenCalledOnce();
    expect(reload).toHaveBeenCalledOnce();
  });

  it("does not retry or reload after failure, and the same-tick lock permits one PUT", async () => {
    const client = transport();
    const reload = vi.fn();
    const execute = vi.fn(async () => {
      await saveMetadataThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        imageID: 11,
        draft: metadataDraft,
        reload,
      });
    });
    const lock = { current: false };
    const first = startImageMetadataSave(lock, execute);
    const second = startImageMetadataSave(lock, execute);
    expect(first).toBeInstanceOf(Promise);
    expect(second).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();
    expect(client.update).toHaveBeenCalledOnce();
    expect(reload).not.toHaveBeenCalled();
    await first;
    expect(lock.current).toBe(false);
  });

  it("releases the save lock if the flow throws", async () => {
    const lock = { current: false };
    await expect(
      startImageMetadataSave(lock, async () => {
        throw new Error("local test failure");
      }),
    ).rejects.toThrow("local test failure");
    expect(lock.current).toBe(false);
  });
});

describe("local state and delete then reload flows", () => {
  it("reloads once after a confirmed enabled-state update", async () => {
    const client = transport();
    vi.mocked(client.update).mockResolvedValue({
      status: 200,
      data: metadataSuccess,
    } as Awaited<ReturnType<ImageLibraryTransport["update"]>>);
    const reload = vi.fn();
    await expect(
      saveImageEnabledThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        imageID: 11,
        enabled: true,
        reload,
      }),
    ).resolves.toMatchObject({ status: "saved", image: { id: 11 } });
    expect(reload).toHaveBeenCalledOnce();
  });

  it("reloads only after the exact local delete receipt", async () => {
    const client = transport();
    vi.mocked(client.remove).mockResolvedValue({
      status: 200,
      data: deleteSuccess,
    } as Awaited<ReturnType<ImageLibraryTransport["remove"]>>);
    const reload = vi.fn();
    await expect(
      deleteImageThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        imageID: 11,
        idempotencyKey: "image-delete-flow-0000000000",
        reload,
      }),
    ).resolves.toEqual({ status: "deleted", id: 11 });
    expect(client.remove).toHaveBeenCalledOnce();
    expect(reload).toHaveBeenCalledOnce();
  });

  it("does not retry or reload after a delete result of unknown", async () => {
    const client = transport();
    vi.mocked(client.remove).mockRejectedValue(new Error("network down"));
    const reload = vi.fn();
    await expect(
      deleteImageThenReload({
        transport: client,
        cookie: `aicrm_csrf=${CSRF_TOKEN}`,
        imageID: 11,
        idempotencyKey: "image-delete-flow-0000000000",
        reload,
      }),
    ).resolves.toEqual({ status: "unavailable" });
    expect(client.remove).toHaveBeenCalledOnce();
    expect(reload).not.toHaveBeenCalled();
  });
});
