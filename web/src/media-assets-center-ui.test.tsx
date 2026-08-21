import React from "react";
import { readFileSync } from "node:fs";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { ImageLibraryTransport } from "./image-library";
import type { MiniProgramLibraryTransport } from "./miniprogram-library";
import type { GroupInviteLibraryTransport } from "./group-invite-library";
import type {
  MediaAssetsCenterSnapshot,
  MediaAssetsCenterTransports,
} from "./media-assets-center";
import {
  ImageReferenceBlockerTable,
  MediaAssetsCenterPage,
  MediaAssetsOverview,
  StructuredMutationBlocker,
} from "./media-assets-center-ui";

const snapshot: MediaAssetsCenterSnapshot = {
  query: {
    filters: {
      search: "",
      status: "all",
      imageCategory: "",
      imageTags: "",
      imageOnlyUnlabeled: false,
    },
    offsets: { images: 0, miniprograms: 0, groupInvites: 0 },
  },
  images: {
    status: "loaded",
    items: [],
    total: 11,
    offset: 0,
    limit: 24,
    sourceCount: 0,
    visibleCount: 0,
    hasPrevious: false,
    hasNext: false,
    filterScope: "server",
  },
  miniprograms: { status: "error", failure: "unavailable" },
  groupInvites: {
    status: "loaded",
    items: [],
    total: 4,
    offset: 0,
    limit: 100,
    sourceCount: 0,
    visibleCount: 0,
    hasPrevious: false,
    hasNext: false,
    filterScope: "server_and_current_page",
  },
  verifiedAt: 1,
};

function transports(): MediaAssetsCenterTransports {
  return {
    images: {
      list: vi.fn(async () => ({ status: 503, data: {} })),
    } as unknown as ImageLibraryTransport,
    miniprograms: {
      list: vi.fn(async () => ({ status: 503, data: {} })),
    } as unknown as MiniProgramLibraryTransport,
    groupInvites: {
      list: vi.fn(async () => ({ status: 503, data: {} })),
    } as unknown as GroupInviteLibraryTransport,
  };
}

describe("MediaAssetsOverview", () => {
  it("renders three resources with independent counts and a partial failure", () => {
    const html = renderToStaticMarkup(
      <MediaAssetsOverview
        activeTab="images"
        snapshot={snapshot}
        onSelect={vi.fn()}
      />,
    );

    expect(html).toContain("图片素材");
    expect(html).toContain(">11<");
    expect(html).toContain("小程序卡片");
    expect(html).toContain("读取失败");
    expect(html).toContain("群邀请素材");
    expect(html).toContain(">4<");
    expect(html).toContain('aria-selected="true"');
  });

  it("renders a consistent loading navigation before effects run", () => {
    const html = renderToStaticMarkup(
      <MediaAssetsOverview activeTab="groupInvites" onSelect={vi.fn()} />,
    );
    expect((html.match(/读取中/g) ?? []).length).toBe(3);
    expect(html).toContain('role="tablist"');
  });
});

describe("structured blocking states", () => {
  it("shows each nonzero image reference count in a table", () => {
    const html = renderToStaticMarkup(
      <ImageReferenceBlockerTable
        imageID={9}
        references={{
          miniprograms: 2,
          campaignSteps: 1,
          groupInvites: 0,
          automationAgents: 0,
          channels: 3,
          importPreflights: 0,
        }}
      />,
    );
    expect(html).toContain("图片 #9 未删除");
    expect(html).toContain("小程序卡片");
    expect(html).toContain("活动步骤");
    expect(html).toContain("渠道配置");
    expect(html).toContain("共发现 6");
    expect(html).not.toContain("群邀请素材</td><td>0");
  });

  it("shows conflict type and explicitly records that no retry ran", () => {
    const html = renderToStaticMarkup(
      <StructuredMutationBlocker
        resource="小程序卡片"
        operation="删除"
        detail="本地引用检查返回 conflict"
      />,
    );
    expect(html).toContain("本地引用或版本冲突");
    expect(html).toContain("自动重试");
    expect(html).toContain("未执行");
  });
});

describe("MediaAssetsCenterPage safety copy and permission boundary", () => {
  it("keeps sales fail-closed and does not render operational controls", () => {
    const client = transports();
    const html = renderToStaticMarkup(
      <MediaAssetsCenterPage role="sales" transports={client} />,
    );
    expect(html).toContain("统一媒体资产运营中心");
    expect(html).toContain("当前账号没有媒体资产运营中心访问权限");
    expect(html).not.toContain("应用筛选");
    expect(client.images.list).not.toHaveBeenCalled();
    expect(client.miniprograms.list).not.toHaveBeenCalled();
    expect(client.groupInvites.list).not.toHaveBeenCalled();
  });

  it("labels all behavior as local and never claims external availability", () => {
    const html = renderToStaticMarkup(
      <MediaAssetsCenterPage role="ops" transports={transports()} />,
    );
    expect(html).toContain("本地事实");
    expect(html).toContain("不会上传到外部平台");
    expect(html).toContain("不会");
    expect(html).not.toMatch(/已在企微可用|Provider 已验证|群已可用|外发成功/);
  });

  it("contains no prohibited network protocol copies or thumbnail-cache operations", () => {
    const source = readFileSync(
      new URL("./media-assets-center-ui.tsx", import.meta.url),
      "utf8",
    );
    expect(source).not.toMatch(/resolveMiniProgramThumbnail/);
    expect(source).not.toMatch(/uploadLibraryImage/);
    expect(source).not.toMatch(/ImageLibraryPage/);
    expect(source).not.toMatch(/imageDataURL|importImageDataURL/);
    expect(source).not.toMatch(/\bfetch\s*\(/);
    expect(source).not.toMatch(/XMLHttpRequest|window\.open|navigator\.clipboard/);
    expect(source).not.toMatch(/api\/generated/);
  });
});
