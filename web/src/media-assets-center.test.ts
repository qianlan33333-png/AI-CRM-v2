import { describe, expect, it, vi } from "vitest";
import type {
  ImageDetail,
  ImageItem,
  ImageLibraryTransport,
  ImageListResult,
} from "./image-library";
import type {
  MiniProgramLibraryTransport,
  MiniProgramListResult,
  MiniProgramRecord,
} from "./miniprogram-library";
import type {
  GroupInviteLibraryItem,
  GroupInviteLibraryResult,
  GroupInviteLibraryTransport,
} from "./group-invite-library";
import {
  INITIAL_MEDIA_ASSETS_CENTER_QUERY,
  MEDIA_ASSET_NAVIGATION,
  MediaAssetsCenterReadController,
  MediaAssetsWriteLock,
  createMediaAssetsCenterLoaders,
  filterGroupInviteCurrentPage,
  imageReferenceBlockerRows,
  imageReferenceBlockerTotal,
  mediaMutationIdempotencyKey,
  sameGroupInviteRecord,
  sameImageDetail,
  sameMiniProgramRecord,
  verifyImageDeleted,
  verifyImageReadback,
  verifyImageUploadReadback,
  type MediaAssetsCenterLoaders,
  type MediaAssetsCenterTransports,
} from "./media-assets-center";

const image: ImageItem = {
  id: 1,
  name: "欢迎图",
  fileName: "welcome.png",
  mimeType: "image/png",
  fileSize: 100,
  enabled: true,
  description: "",
  tags: ["welcome"],
  category: "onboarding",
  width: 100,
  height: 100,
  createdAt: "2026-08-21T08:00:00Z",
  updatedAt: "2026-08-21T08:00:00Z",
};

const imageDetail: ImageDetail = {
  ...image,
  previewURL: "/api/admin/image-library/1/variants/mobile_1080",
  originalURL: "/api/admin/image-library/1/variants/original",
};

function imageDetailResponse(extra: Record<string, unknown> = {}) {
  return {
    status: 200,
    data: {
      ok: true,
      item: {
        id: 1,
        name: "欢迎图",
        file_name: "welcome.png",
        mime_type: "image/png",
        file_size: 100,
        description: "",
        category: "onboarding",
        width: 100,
        height: 100,
        created_at: "2026-08-21T08:00:00Z",
        updated_at: "2026-08-21T08:00:00Z",
        content_type: "image/png",
        tags: ["welcome"],
        enabled: true,
        source: "upload",
        source_url: "",
        thumb_media_id: "",
        thumb_media_id_expires_at: "",
        ai_metadata: {},
        thumb_160_url: "/api/admin/image-library/1/variants/thumb_160",
        thumb_320_url: "/api/admin/image-library/1/variants/thumb_320",
        thumb_url: "/api/admin/image-library/1/variants/thumb_320",
        preview_url: "/api/admin/image-library/1/variants/mobile_1080",
        mobile_1080_url: "/api/admin/image-library/1/variants/mobile_1080",
        large_1440_url: "/api/admin/image-library/1/variants/large_1440",
        original_url: "/api/admin/image-library/1/variants/original",
      },
      source_status: "next_media_library",
      route_owner: "ai_crm_next",
      fallback_used: false,
      real_external_call_executed: false,
      storage_adapter_mode: "postgresql",
      adapter_mode: "postgresql",
      ...extra,
    },
  };
}

const miniProgram: MiniProgramRecord = {
  id: 2,
  name: "预约卡",
  appID: "wx-local",
  pagePath: "pages/book/index",
  title: "立即预约",
  thumbImageID: 1,
  thumbMediaID: "",
  enabled: true,
  version: 1,
  createdAt: "2026-08-21T08:00:00Z",
  updatedAt: "2026-08-21T08:00:00Z",
};

const groupInvite: GroupInviteLibraryItem = {
  id: 3,
  name: "体验群",
  title: "加入体验群",
  description: "本地说明",
  joinURL: "https://work.weixin.qq.com/gm/local-token",
  coverImageID: 1,
  enabled: true,
  createdAt: "2026-08-21T08:00:00Z",
  updatedAt: "2026-08-21T08:00:00Z",
  version: 1,
};

function imageLoaded(items: readonly ImageItem[] = [image]): ImageListResult {
  return {
    status: "loaded",
    items,
    total: items.length,
    limit: 24,
    offset: 0,
    count: items.length,
    hasMore: false,
  };
}

function miniLoaded(
  items: readonly MiniProgramRecord[] = [miniProgram],
): MiniProgramListResult {
  return {
    status: "loaded",
    items,
    total: items.length,
    limit: 20,
    offset: 0,
  };
}

function groupLoaded(
  items: readonly GroupInviteLibraryItem[] = [groupInvite],
): GroupInviteLibraryResult {
  return {
    status: "loaded",
    page: {
      items,
      total: items.length,
      limit: 100,
      offset: 0,
    },
  };
}

function loaders(overrides: Partial<MediaAssetsCenterLoaders> = {}): MediaAssetsCenterLoaders {
  return {
    images: vi.fn(async () => imageLoaded()),
    miniprograms: vi.fn(async () => miniLoaded()),
    groupInvites: vi.fn(async () => groupLoaded()),
    ...overrides,
  };
}

function deferred<T>() {
  // eslint-disable-next-line no-unused-vars -- named parameter documents the deferred resolver contract.
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("unified media assets read orchestration", () => {
  it("defines exactly the three closed navigation resources", () => {
    expect(MEDIA_ASSET_NAVIGATION.map((item) => item.kind)).toEqual([
      "images",
      "miniprograms",
      "groupInvites",
    ]);
    expect(MEDIA_ASSET_NAVIGATION.every((item) => item.localFactDescription.length > 0)).toBe(true);
  });

  it("keeps verified sections independent when one parallel read fails", async () => {
    const controller = new MediaAssetsCenterReadController(() => 123);
    const result = await controller.load(
      "ops",
      INITIAL_MEDIA_ASSETS_CENTER_QUERY,
      loaders({ miniprograms: vi.fn(async () => ({ status: "unavailable" as const })) }),
    );

    expect(result.status).toBe("current");
    if (result.status !== "current") return;
    expect(result.snapshot.images.status).toBe("loaded");
    expect(result.snapshot.miniprograms).toEqual({
      status: "error",
      failure: "unavailable",
    });
    expect(result.snapshot.groupInvites.status).toBe("loaded");
    expect(result.snapshot.verifiedAt).toBe(123);
  });

  it("invalidates an older response when a tab or filter initiates a newer load", async () => {
    const firstImages = deferred<ImageListResult>();
    const images = vi
      .fn<MediaAssetsCenterLoaders["images"]>()
      .mockImplementationOnce(() => firstImages.promise)
      .mockResolvedValueOnce(imageLoaded([]));
    const client = loaders({ images });
    const controller = new MediaAssetsCenterReadController();

    const first = controller.load("admin", INITIAL_MEDIA_ASSETS_CENTER_QUERY, client);
    const second = controller.load(
      "admin",
      {
        ...INITIAL_MEDIA_ASSETS_CENTER_QUERY,
        filters: { ...INITIAL_MEDIA_ASSETS_CENTER_QUERY.filters, search: "new" },
      },
      client,
    );
    const secondResult = await second;
    firstImages.resolve(imageLoaded());
    const firstResult = await first;

    expect(secondResult.status).toBe("current");
    expect(firstResult).toEqual({ status: "stale" });
    expect(images).toHaveBeenCalledTimes(2);
  });

  it("fails sales closed without starting any resource request", async () => {
    const client = loaders();
    const result = await new MediaAssetsCenterReadController().load(
      "sales",
      INITIAL_MEDIA_ASSETS_CENTER_QUERY,
      client,
    );

    expect(result).toEqual({ status: "forbidden" });
    expect(client.images).not.toHaveBeenCalled();
    expect(client.miniprograms).not.toHaveBeenCalled();
    expect(client.groupInvites).not.toHaveBeenCalled();
  });

  it("passes the unified search and status through each frozen leaf query", async () => {
    const client = loaders();
    await new MediaAssetsCenterReadController().load(
      "admin",
      {
        filters: {
          search: "  预约  ",
          status: "disabled",
          imageCategory: "  海报  ",
          imageTags: "a,b",
          imageOnlyUnlabeled: true,
        },
        offsets: { images: 24, miniprograms: 20, groupInvites: 100 },
      },
      client,
    );

    expect(client.images).toHaveBeenCalledWith({
      search: "预约",
      category: "海报",
      tags: "a,b",
      onlyUnlabeled: true,
      includeDisabled: true,
      offset: 24,
    });
    expect(client.miniprograms).toHaveBeenCalledWith({
      search: "预约",
      enabledOnly: false,
      offset: 20,
    });
    expect(client.groupInvites).toHaveBeenCalledWith(100);
  });

  it("does not search group-invite join URLs or treat them as verified QR content", () => {
    const bySecretURL = filterGroupInviteCurrentPage([groupInvite], {
      ...INITIAL_MEDIA_ASSETS_CENTER_QUERY.filters,
      search: "local-token",
    });
    const byTitle = filterGroupInviteCurrentPage([groupInvite], {
      ...INITIAL_MEDIA_ASSETS_CENTER_QUERY.filters,
      search: "体验群",
    });
    expect(bySecretURL).toEqual([]);
    expect(byTitle).toEqual([groupInvite]);
  });
});

describe("closed parser and no-external-effect boundaries", () => {
  it("rejects a malicious mini-program response that claims an external call", async () => {
    const transports = {
      images: {
        list: vi.fn(async () => ({ status: 503, data: {} })),
      } as unknown as ImageLibraryTransport,
      miniprograms: {
        list: vi.fn(async () => ({
          status: 200,
          data: {
            ok: true,
            items: [],
            miniprograms: [],
            total: 0,
            limit: 20,
            offset: 0,
            local_only: true,
            provider_call_executed: true,
            real_external_call_executed: false,
          },
        })),
      } as unknown as MiniProgramLibraryTransport,
      groupInvites: {
        list: vi.fn(async () => ({
          status: 200,
          data: {
            ok: true,
            items: [],
            group_invites: [],
            total: 0,
            limit: 100,
            offset: 0,
            provider_call_executed: false,
          },
        })),
      } as unknown as GroupInviteLibraryTransport,
    } satisfies MediaAssetsCenterTransports;

    const result = await new MediaAssetsCenterReadController().load(
      "admin",
      INITIAL_MEDIA_ASSETS_CENTER_QUERY,
      createMediaAssetsCenterLoaders(transports),
    );

    expect(result.status).toBe("current");
    if (result.status !== "current") return;
    expect(result.snapshot.miniprograms).toEqual({
      status: "error",
      failure: "unavailable",
    });
  });

  it("overview reads never call mutation, thumbnail resolution, upload, or archive methods", async () => {
    const resolve = vi.fn();
    const uploadImage = vi.fn();
    const createMini = vi.fn();
    const updateMini = vi.fn();
    const removeMini = vi.fn();
    const createGroup = vi.fn();
    const updateGroup = vi.fn();
    const archiveGroup = vi.fn();
    const transports = {
      images: {
        list: vi.fn(async () => ({ status: 503, data: {} })),
      } as unknown as ImageLibraryTransport,
      miniprograms: {
        list: vi.fn(async () => ({ status: 503, data: {} })),
        resolve,
        uploadImage,
        create: createMini,
        update: updateMini,
        remove: removeMini,
      } as unknown as MiniProgramLibraryTransport,
      groupInvites: {
        list: vi.fn(async () => ({ status: 503, data: {} })),
        create: createGroup,
        update: updateGroup,
        archive: archiveGroup,
      } as unknown as GroupInviteLibraryTransport,
    } satisfies MediaAssetsCenterTransports;

    await new MediaAssetsCenterReadController().load(
      "admin",
      INITIAL_MEDIA_ASSETS_CENTER_QUERY,
      createMediaAssetsCenterLoaders(transports),
    );

    expect(resolve).not.toHaveBeenCalled();
    expect(uploadImage).not.toHaveBeenCalled();
    expect(createMini).not.toHaveBeenCalled();
    expect(updateMini).not.toHaveBeenCalled();
    expect(removeMini).not.toHaveBeenCalled();
    expect(createGroup).not.toHaveBeenCalled();
    expect(updateGroup).not.toHaveBeenCalled();
    expect(archiveGroup).not.toHaveBeenCalled();
  });
});

describe("write safety helpers", () => {
  it("renders image reference counts as structured blockers", () => {
    const references = {
      miniprograms: 2,
      campaignSteps: 0,
      groupInvites: 1,
      automationAgents: 0,
      channels: 3,
      importPreflights: 0,
    };
    expect(imageReferenceBlockerRows(references)).toEqual([
      { key: "miniprograms", label: "小程序卡片", count: 2 },
      { key: "groupInvites", label: "群邀请素材", count: 1 },
      { key: "channels", label: "渠道配置", count: 3 },
    ]);
    expect(imageReferenceBlockerTotal(references)).toBe(6);
  });

  it("locks unknown write outcomes without an automatic unlock", () => {
    const lock = new MediaAssetsWriteLock();
    lock.lock("miniprograms");
    expect(lock.isLocked("miniprograms")).toBe(true);
    expect(lock.isLocked("images")).toBe(false);
    expect(lock.snapshot()).toEqual({
      images: false,
      miniprograms: true,
      groupInvites: false,
    });
  });

  it("creates one valid printable idempotency key only from a UUID", () => {
    const key = mediaMutationIdempotencyKey(
      "groupInvites",
      "archive",
      () => "123e4567-e89b-12d3-a456-426614174000",
    );
    expect(key).toBe(
      "media-center:groupInvites:archive:123e4567-e89b-12d3-a456-426614174000",
    );
    expect(mediaMutationIdempotencyKey("miniprograms", "update", () => "bad")).toBeUndefined();
    expect(
      mediaMutationIdempotencyKey("miniprograms", "update", () => {
        throw new Error("uuid unavailable");
      }),
    ).toBeUndefined();
  });

  it("verifies image write-readback and deletion without retrying", async () => {
    const detail = vi.fn(async () => imageDetailResponse());
    const transport = { detail } as unknown as ImageLibraryTransport;

    expect(await verifyImageReadback(transport, imageDetail)).toEqual({
      status: "verified",
    });
    expect(
      await verifyImageUploadReadback(
        transport,
        {
          id: 1,
          name: "欢迎图",
          fileName: "welcome.png",
          fileSize: 100,
          width: 100,
          height: 100,
        },
        {
          name: " 欢迎图 ",
          description: "",
          tags: "welcome",
          category: " onboarding ",
        },
      ),
    ).toEqual({ status: "verified", image: imageDetail });

    const removedDetail = vi.fn(async () => ({ status: 404, data: {} }));
    expect(
      await verifyImageDeleted(
        { detail: removedDetail } as unknown as ImageLibraryTransport,
        1,
      ),
    ).toEqual({ status: "verified" });
    expect(removedDetail).toHaveBeenCalledTimes(1);
  });

  it("locks image readback fail closed when the response adds an unapproved field", async () => {
    const transport = {
      detail: vi.fn(async () => imageDetailResponse({ provider_verified: true })),
    } as unknown as ImageLibraryTransport;
    expect(await verifyImageReadback(transport, imageDetail)).toEqual({
      status: "unavailable",
    });
  });

  it("compares all closed write-readback fields", () => {
    expect(sameImageDetail(imageDetail, { ...imageDetail })).toBe(true);
    expect(
      sameImageDetail(imageDetail, { ...imageDetail, originalURL: "/other" }),
    ).toBe(false);
    expect(sameMiniProgramRecord(miniProgram, { ...miniProgram })).toBe(true);
    expect(
      sameMiniProgramRecord(miniProgram, { ...miniProgram, version: 2 }),
    ).toBe(false);
    expect(sameGroupInviteRecord(groupInvite, { ...groupInvite })).toBe(true);
    expect(
      sameGroupInviteRecord(groupInvite, { ...groupInvite, enabled: false }),
    ).toBe(false);
    expect(
      sameGroupInviteRecord(groupInvite, { ...groupInvite, updatedBy: 99 }),
    ).toBe(false);
  });
});
