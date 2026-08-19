import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  handleSearchKeyDown,
  ImageLibraryPage,
  saveMetadataThenReload,
  startImageMetadataSave,
  uploadThenReload,
} from "./image-library-ui";
import type { ImageLibraryTransport } from "./image-library";

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

function transport(): ImageLibraryTransport {
  const unavailable = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(unavailable),
    detail: vi.fn(unavailable),
    facets: vi.fn(unavailable),
    upload: vi.fn(unavailable),
    update: vi.fn(unavailable),
  } as unknown as ImageLibraryTransport;
}

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
      expect(html).toContain("上传图片");
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
      expect(html.match(/<form\b/g)).toHaveLength(2);
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
