import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  handleImageSearchKeyDown,
  MiniProgramLibraryPage,
} from "./miniprogram-library-ui";
import { editorDraft } from "./miniprogram-library";
import type { MiniProgramLibraryTransport } from "./miniprogram-library";

function transport(): MiniProgramLibraryTransport {
  const unavailable = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(unavailable),
    create: vi.fn(unavailable),
    update: vi.fn(unavailable),
    remove: vi.fn(unavailable),
    resolve: vi.fn(unavailable),
    listImages: vi.fn(unavailable),
    uploadImage: vi.fn(unavailable),
  } as unknown as MiniProgramLibraryTransport;
}

describe("MiniProgramLibraryPage shell", () => {
  it.each(["admin", "ops"] as const)(
    "renders the complete UI flow shell for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <MiniProgramLibraryPage role={role} transport={transport()} />,
      );
      expect(html).toContain('<h1 id="app-title">小程序素材库</h1>');
      expect(html).toContain("素材列表");
      expect(html).toContain("搜索素材");
      expect(html).toContain("全部素材");
      expect(html).toContain("仅启用");
      expect(html).toContain("新建素材");
      expect(html).toContain("创建素材");
      expect(html).toContain("选择现有图片");
      expect(html).toContain("上传新图片");
      expect(html).toContain("不代表真实企业微信可用、已上传或已发送");
      expect(html).toContain('role="status"');
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toContain("aicrm_csrf");
    },
  );

  it("keeps sales fail-closed without rendering data controls", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <MiniProgramLibraryPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有小程序素材访问权限。");
    expect(html).not.toContain("新建素材");
    expect(html).not.toContain("素材列表");
    expect(client.list).not.toHaveBeenCalled();
    expect(client.listImages).not.toHaveBeenCalled();
  });

  it("starts an empty draft without a thumbnail and never infers IDs", () => {
    const draft = editorDraft();
    expect(draft).toEqual({ name: "", appID: "", pagePath: "", title: "" });
    expect("thumbImageID" in draft).toBe(false);
  });

  it("isolates Enter in image search from the outer save form", () => {
    const preventDefault = vi.fn();
    const stopPropagation = vi.fn();
    const search = vi.fn();

    handleImageSearchKeyDown(
      { key: "Enter", preventDefault, stopPropagation },
      search,
    );

    expect(preventDefault).toHaveBeenCalledOnce();
    expect(stopPropagation).toHaveBeenCalledOnce();
    expect(search).toHaveBeenCalledOnce();
  });
});
