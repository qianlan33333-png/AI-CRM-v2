import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  handleImageSearchKeyDown,
  MiniProgramDetailPanel,
  MiniProgramLibraryPage,
} from "./miniprogram-library-ui";
import { editorDraft } from "./miniprogram-library";
import type { MiniProgramLibraryTransport } from "./miniprogram-library";

function transport(): MiniProgramLibraryTransport {
  const unavailable = async () => ({ status: 503, data: {} });
  return {
    list: vi.fn(unavailable),
    detail: vi.fn(unavailable),
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
    expect(client.detail).not.toHaveBeenCalled();
  });

  it("renders only frozen local detail fields and hides thumbnail cache values", () => {
    const html = renderToStaticMarkup(
      <MiniProgramDetailPanel
        state={{
          kind: "ready",
          item: {
            id: 7,
            name: "卡片",
            appID: "wx-demo",
            pagePath: "pages/home",
            title: "首页",
            thumbImageID: 11,
            thumbMediaID: "thumb-media-secret",
            enabled: true,
            version: 1,
            createdAt: "2026-08-19T08:00:00Z",
            updatedAt: "2026-08-19T08:00:01Z",
          },
        }}
      />,
    );
    expect(html).toContain("本地素材详情");
    expect(html).toContain("缩略图素材 ID");
    expect(html).toContain("wx-demo");
    expect(html).not.toContain("thumb-media-secret");
    expect(html).not.toMatch(/thumb_media_id|href=|clipboard|window\.open|<img/i);
  });

  it("keeps a verified detail visible beside a local detail failure", () => {
    const html = renderToStaticMarkup(
      <MiniProgramDetailPanel
        state={{
          kind: "error",
          itemID: 7,
          failure: "unavailable",
          previous: {
            id: 7,
            name: "卡片",
            appID: "wx-demo",
            pagePath: "pages/home",
            title: "首页",
            thumbMediaID: "thumb-media-secret",
            enabled: true,
            version: 1,
            createdAt: "2026-08-19T08:00:00Z",
            updatedAt: "2026-08-19T08:00:01Z",
          },
        }}
      />,
    );
    expect(html).toContain("卡片");
    expect(html).toContain("小程序素材服务暂时不可用");
    expect(html).not.toContain("thumb-media-secret");
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
