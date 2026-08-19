import { describe, expect, it, vi } from "vitest";
import {
  firstPageQuery,
  formatFileSize,
  imagePreviewURL,
  IMAGE_LIBRARY_PAGE_SIZE,
  imageMetadataRequest,
  loadImageDetail,
  loadFacets,
  loadImages,
  MAX_IMAGE_FILE_SIZE,
  nextPageOffset,
  normalizeTagsInput,
  parseImageDetail,
  parseImageItem,
  previousPageOffset,
  uploadIdempotencyKey,
  uploadImage,
  updateImageMetadata,
  type ImageLibraryTransport,
  type ImageListQuery,
} from "./image-library";

const CSRF_TOKEN = "a".repeat(43);
const CSRF_COOKIE = `aicrm_csrf=${CSRF_TOKEN}`;

const flags = {
  source_status: "next_media_library",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  storage_adapter_mode: "postgresql",
  adapter_mode: "postgresql",
};
const imageItem = {
  id: 11,
  name: "封面",
  file_name: "cover.png",
  mime_type: "image/png",
  file_size: 1024,
  enabled: true,
  description: "",
  tags: ["活动"],
  category: "banner",
  width: 400,
  height: 300,
  created_at: "2026-08-17T12:00:00Z",
  updated_at: "2026-08-18T12:00:00Z",
  thumb_160_url: "/api/admin/image-library/11/variants/thumb_160",
  thumb_320_url: "/api/admin/image-library/11/variants/thumb_320",
  thumb_url: "/api/admin/image-library/11/variants/thumb_320",
  preview_url: "/api/admin/image-library/11/variants/mobile_1080",
  mobile_1080_url: "/api/admin/image-library/11/variants/mobile_1080",
  large_1440_url: "/api/admin/image-library/11/variants/large_1440",
  original_url: "/api/admin/image-library/11/variants/original",
};
const listPage = {
  ok: true,
  items: [imageItem],
  total: 1,
  limit: IMAGE_LIBRARY_PAGE_SIZE,
  offset: 0,
  count: 1,
  has_more: false,
  next_offset: null,
  ...flags,
};
const facetsPage = {
  ok: true,
  categories: ["banner"],
  tags: ["活动"],
  ...flags,
};
const detailItem = {
  id: 11,
  name: "封面",
  file_name: "cover.png",
  mime_type: "image/png",
  file_size: 1024,
  description: "首页封面",
  category: "banner",
  width: 400,
  height: 300,
  created_at: "2026-08-17T12:00:00Z",
  updated_at: "2026-08-18T12:00:00Z",
  content_type: "image/png",
  tags: ["活动"],
  enabled: false,
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
};
const detailSuccess = { ok: true, item: detailItem, ...flags };
const metadataDraft = {
  name: " 新封面 ",
  description: " 首页主图 ",
  tags: " hero, 首页,hero, ",
  category: " cover ",
};
const updateSuccess = {
  ok: true,
  item: {
    ...detailItem,
    name: "新封面",
    description: "首页主图",
    tags: ["hero", "首页"],
    category: "cover",
    enabled: true,
    updated_at: "2026-08-19T12:00:00Z",
  },
  ...flags,
  source_status: "local_repository_write",
};
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

function itemWithId(id: number): Record<string, unknown> {
  return {
    ...imageItem,
    id,
    thumb_160_url: `/api/admin/image-library/${id}/variants/thumb_160`,
    thumb_320_url: `/api/admin/image-library/${id}/variants/thumb_320`,
    thumb_url: `/api/admin/image-library/${id}/variants/thumb_320`,
    preview_url: `/api/admin/image-library/${id}/variants/mobile_1080`,
    mobile_1080_url: `/api/admin/image-library/${id}/variants/mobile_1080`,
    large_1440_url: `/api/admin/image-library/${id}/variants/large_1440`,
    original_url: `/api/admin/image-library/${id}/variants/original`,
  };
}

function transport(
  response: { status: number; data: unknown } = { status: 200, data: listPage },
): ImageLibraryTransport {
  const reply = async () => response;
  return {
    list: vi.fn(reply),
    detail: vi.fn(reply),
    facets: vi.fn(reply),
    upload: vi.fn(reply),
    update: vi.fn(reply),
  } as unknown as ImageLibraryTransport;
}

function query(partial: Partial<ImageListQuery> = {}): ImageListQuery {
  return {
    search: "",
    category: "",
    tags: "",
    onlyUnlabeled: false,
    offset: 0,
    ...partial,
  };
}

const pngFile = { type: "image/png", size: 1024 } as Blob;
const metadata = { name: "封面", description: "", tags: "", category: "" };
const IDEMPOTENCY_KEY = "image-upload-test-000000000000";

describe("image item strict parser", () => {
  it("parses only the exact frozen twenty-field item", () => {
    expect(parseImageItem(imageItem)).toEqual({
      id: 11,
      name: "封面",
      fileName: "cover.png",
      mimeType: "image/png",
      fileSize: 1024,
      enabled: true,
      description: "",
      tags: ["活动"],
      category: "banner",
      width: 400,
      height: 300,
      createdAt: "2026-08-17T12:00:00Z",
      updatedAt: "2026-08-18T12:00:00Z",
    });
    expect(parseImageItem({ ...imageItem, extra: 1 })).toBeUndefined();
    const missing: Record<string, unknown> = { ...imageItem };
    delete missing.file_size;
    expect(parseImageItem(missing)).toBeUndefined();
    expect(parseImageItem(null)).toBeUndefined();
    expect(parseImageItem([imageItem])).toBeUndefined();
  });

  it("rejects scalar violations", () => {
    expect(parseImageItem({ ...imageItem, id: 0 })).toBeUndefined();
    expect(parseImageItem({ ...imageItem, id: 1.5 })).toBeUndefined();
    expect(
      parseImageItem({ ...imageItem, name: "x".repeat(201) }),
    ).toBeUndefined();
    expect(parseImageItem({ ...imageItem, file_name: "" })).toBeUndefined();
    expect(
      parseImageItem({ ...imageItem, file_name: " cover.png" }),
    ).toBeUndefined();
    expect(
      parseImageItem({ ...imageItem, mime_type: "image/webp" }),
    ).toBeUndefined();
    expect(parseImageItem({ ...imageItem, file_size: 0 })).toBeUndefined();
    expect(
      parseImageItem({ ...imageItem, file_size: MAX_IMAGE_FILE_SIZE + 1 }),
    ).toBeUndefined();
    expect(parseImageItem({ ...imageItem, enabled: false })).toBeUndefined();
    expect(parseImageItem({ ...imageItem, width: 0 })).toBeUndefined();
    expect(parseImageItem({ ...imageItem, height: 10001 })).toBeUndefined();
    expect(
      parseImageItem({ ...imageItem, category: "x".repeat(201) }),
    ).toBeUndefined();
    expect(
      parseImageItem({ ...imageItem, description: 1 }),
    ).toBeUndefined();
    expect(parseImageItem({ ...imageItem, created_at: "" })).toBeUndefined();
  });

  it("rejects tags array violations", () => {
    expect(
      parseImageItem({ ...imageItem, tags: Array(51).fill("t") }),
    ).toBeUndefined();
    expect(
      parseImageItem({ ...imageItem, tags: ["x".repeat(65)] }),
    ).toBeUndefined();
    expect(parseImageItem({ ...imageItem, tags: [1] })).toBeUndefined();
    expect(parseImageItem({ ...imageItem, tags: "活动" })).toBeUndefined();
  });

  it("rejects created_at/updated_at values that are not strict RFC3339 date-time", () => {
    const malformed = [
      "2026-13-01T12:00:00Z",
      "2026-02-30T12:00:00Z",
      "2025-02-29T12:00:00Z",
      "2026-04-31T12:00:00Z",
      "2026-08-17 12:00:00Z",
      "2026-08-17T12:00:00",
      "2026-08-17T25:00:00Z",
      "2026-08-17T12:60:00Z",
      "2026-08-17T12:00:60Z",
      "2026-08-17T12:00:00+25:00",
      "2026-08-17T12:00:00+08:60",
      "2026-08-17",
      "2026-08-17T12:00Z",
      "not-a-date",
      "",
    ];
    for (const bad of malformed) {
      expect(
        parseImageItem({ ...imageItem, created_at: bad }),
        bad,
      ).toBeUndefined();
      expect(
        parseImageItem({ ...imageItem, updated_at: bad }),
        bad,
      ).toBeUndefined();
    }
  });

  it("accepts well-formed RFC3339 timestamps with offsets and fractions", () => {
    const wellFormed = [
      "2026-08-17T12:00:00Z",
      "2026-08-17T12:00:00.123456789Z",
      "2026-08-17T12:00:00+08:00",
      "2026-08-17T12:00:00-05:30",
      "2024-02-29T23:59:59Z",
    ];
    for (const good of wellFormed) {
      expect(
        parseImageItem({ ...imageItem, created_at: good }),
        good,
      ).toBeDefined();
      expect(
        parseImageItem({ ...imageItem, updated_at: good }),
        good,
      ).toBeDefined();
    }
  });

  it("rejects variant URLs that are unsafe or inconsistent with the item id", () => {
    expect(
      parseImageItem({
        ...imageItem,
        thumb_160_url: "/api/admin/image-library/12/variants/thumb_160",
      }),
    ).toBeUndefined();
    expect(
      parseImageItem({
        ...imageItem,
        thumb_url: "/api/admin/image-library/11/variants/thumb_160",
      }),
    ).toBeUndefined();
    expect(
      parseImageItem({
        ...imageItem,
        preview_url: "/api/admin/image-library/11/variants/original",
      }),
    ).toBeUndefined();
    expect(
      parseImageItem({
        ...imageItem,
        original_url: "https://evil.example/11/original",
      }),
    ).toBeUndefined();
    expect(
      parseImageItem({
        ...imageItem,
        mobile_1080_url: "//evil.example/variants/mobile_1080",
      }),
    ).toBeUndefined();
    expect(
      parseImageItem({
        ...imageItem,
        large_1440_url: "/api/admin/image-library/11/variants/large_1440/extra",
      }),
    ).toBeUndefined();
  });
});

describe("image detail strict parser and local read", () => {
  it("uses only the validated detail URLs for explicit local preview modes", () => {
    const image = parseImageDetail(detailItem);
    expect(image).toBeDefined();
    expect(imagePreviewURL(image!, "standard")).toBe(
      "/api/admin/image-library/11/variants/mobile_1080",
    );
    expect(imagePreviewURL(image!, "original")).toBe(
      "/api/admin/image-library/11/variants/original",
    );
  });

  it("accepts only the exact no-query detail projection", () => {
    expect(parseImageDetail(detailItem)).toEqual({
      id: 11,
      name: "封面",
      fileName: "cover.png",
      mimeType: "image/png",
      fileSize: 1024,
      enabled: false,
      description: "首页封面",
      tags: ["活动"],
      category: "banner",
      width: 400,
      height: 300,
      createdAt: "2026-08-17T12:00:00Z",
      updatedAt: "2026-08-18T12:00:00Z",
      previewURL: "/api/admin/image-library/11/variants/mobile_1080",
      originalURL: "/api/admin/image-library/11/variants/original",
    });
  });

  it("rejects data, external URLs, mismatched IDs, and malformed detail fields", () => {
    const cases = [
      { ...detailItem, data_url: "data:image/png;base64,AA==" },
      { ...detailItem, data_base64: "AA==" },
      { ...detailItem, variant_url: detailItem.preview_url },
      {
        ...detailItem,
        preview_url: "https://evil.example/image.png",
      },
      {
        ...detailItem,
        original_url: "/api/admin/image-library/12/variants/original",
      },
      { ...detailItem, content_type: "image/jpeg" },
      { ...detailItem, source_url: "/unexpected" },
      { ...detailItem, ai_metadata: { provider: "unexpected" } },
      { ...detailItem, enabled: "true" },
      { ...detailItem, name: "cover\x00.png" },
      { ...detailItem, updated_at: "not-a-date" },
      { ...detailItem, extra: true },
    ];
    for (const value of cases) {
      expect(parseImageDetail(value)).toBeUndefined();
    }
  });

  it("performs exactly one parameter-free detail read and validates the full envelope", async () => {
    const client = transport({ status: 200, data: detailSuccess });
    await expect(loadImageDetail(client, 11)).resolves.toEqual({
      status: "loaded",
      image: expect.objectContaining({ id: 11 }),
    });
    expect(client.detail).toHaveBeenCalledOnce();
    expect(client.detail).toHaveBeenCalledWith("11");
    expect(client.list).not.toHaveBeenCalled();
    expect(client.facets).not.toHaveBeenCalled();
    expect(client.upload).not.toHaveBeenCalled();
  });

  it("fails closed for envelope, status, invalid IDs, and network errors without retrying", async () => {
    const malformed = transport({
      status: 200,
      data: { ...detailSuccess, fallback_used: true },
    });
    await expect(loadImageDetail(malformed, 11)).resolves.toEqual({
      status: "unavailable",
    });
    expect(malformed.detail).toHaveBeenCalledOnce();

    for (const [status, expected] of [
      [401, "unauthenticated"],
      [403, "forbidden"],
      [404, "unavailable"],
      [422, "invalid"],
      [503, "unavailable"],
    ] as const) {
      const client = transport({ status, data: {} });
      await expect(loadImageDetail(client, 11)).resolves.toEqual({
        status: expected,
      });
      expect(client.detail).toHaveBeenCalledOnce();
    }

    const throwing: ImageLibraryTransport = {
      list: vi.fn(),
      detail: vi.fn(async () => {
        throw new Error("network down");
      }),
      facets: vi.fn(),
      upload: vi.fn(),
    } as unknown as ImageLibraryTransport;
    await expect(loadImageDetail(throwing, 11)).resolves.toEqual({
      status: "unavailable",
    });
    expect(throwing.detail).toHaveBeenCalledOnce();
    await expect(loadImageDetail(throwing, 0)).resolves.toEqual({
      status: "invalid",
    });
    expect(throwing.detail).toHaveBeenCalledOnce();
  });
});

describe("image metadata update transport", () => {
  it("normalizes and sends exactly the four frozen local metadata keys", async () => {
    const client = transport({ status: 200, data: updateSuccess });
    await expect(
      updateImageMetadata(client, CSRF_COOKIE, 11, metadataDraft),
    ).resolves.toMatchObject({ status: "saved", image: { id: 11 } });
    expect(client.update).toHaveBeenCalledTimes(1);
    expect(client.update).toHaveBeenCalledWith(
      "11",
      {
        name: "新封面",
        description: "首页主图",
        tags: ["hero", "首页"],
        category: "cover",
      },
      {
        credentials: "same-origin",
        headers: { "X-CSRF-Token": CSRF_TOKEN },
      },
    );
    expect(client.list).not.toHaveBeenCalled();
    expect(client.detail).not.toHaveBeenCalled();
    expect(client.facets).not.toHaveBeenCalled();
    expect(client.upload).not.toHaveBeenCalled();
  });

  it("rejects invalid metadata or CSRF before a write", async () => {
    const client = transport({ status: 200, data: updateSuccess });
    const fifty = Array.from({ length: 50 }, (_, index) => `tag-${index}`);
    expect(
      imageMetadataRequest({ ...metadataDraft, tags: fifty.join(",") }),
    ).toMatchObject({ tags: fifty });
    for (const draft of [
      { ...metadataDraft, name: "x".repeat(201) },
      { ...metadataDraft, description: "x".repeat(10001) },
      { ...metadataDraft, category: "x".repeat(201) },
      { ...metadataDraft, tags: Array.from({ length: 51 }, (_, i) => `t${i}`).join(",") },
      { ...metadataDraft, tags: "x".repeat(65) },
      { ...metadataDraft, tags: "safe,\x00bad" },
    ]) {
      await expect(
        updateImageMetadata(client, CSRF_COOKIE, 11, draft),
      ).resolves.toEqual({ status: "invalid" });
    }
    await expect(
      updateImageMetadata(client, "other=1", 11, metadataDraft),
    ).resolves.toEqual({ status: "csrf_missing" });
    expect(client.update).not.toHaveBeenCalled();
  });

  it("fails closed for a malformed, mismatched, disabled, failed, or unknown result without retry", async () => {
    for (const data of [
      { ...updateSuccess, extra: true },
      { ...updateSuccess, item: { ...updateSuccess.item, enabled: false } },
      { ...updateSuccess, item: { ...updateSuccess.item, name: "另一张图" } },
      { ...updateSuccess, source_status: "next_media_library" },
      { ...updateSuccess, item: { ...updateSuccess.item, data_url: "data:image/png;base64,AA==" } },
    ]) {
      const client = transport({ status: 200, data });
      await expect(
        updateImageMetadata(client, CSRF_COOKIE, 11, metadataDraft),
      ).resolves.toEqual({ status: "unavailable" });
      expect(client.update).toHaveBeenCalledTimes(1);
    }
    for (const [status, expected] of [
      [400, "invalid"],
      [401, "unauthenticated"],
      [403, "forbidden"],
      [404, "unavailable"],
      [503, "unavailable"],
    ] as const) {
      const client = transport({ status, data: {} });
      await expect(
        updateImageMetadata(client, CSRF_COOKIE, 11, metadataDraft),
      ).resolves.toEqual({ status: expected });
      expect(client.update).toHaveBeenCalledTimes(1);
    }
    const throwing: ImageLibraryTransport = {
      list: vi.fn(),
      detail: vi.fn(),
      facets: vi.fn(),
      upload: vi.fn(),
      update: vi.fn(async () => {
        throw new Error("connection reset");
      }),
    } as unknown as ImageLibraryTransport;
    await expect(
      updateImageMetadata(throwing, CSRF_COOKIE, 11, metadataDraft),
    ).resolves.toEqual({ status: "unavailable" });
    expect(throwing.update).toHaveBeenCalledTimes(1);
  });
});

describe("image list transport", () => {
  it("loads a strict frozen page and exposes server pagination facts", async () => {
    const client = transport();
    const result = await loadImages(client, query());
    expect(result).toEqual({
      status: "loaded",
      items: [parseImageItem(imageItem)],
      total: 1,
      limit: IMAGE_LIBRARY_PAGE_SIZE,
      offset: 0,
      count: 1,
      hasMore: false,
    });
    expect(client.list).toHaveBeenCalledWith(
      {
        limit: String(IMAGE_LIBRARY_PAGE_SIZE),
        offset: "0",
        enabled_only: "true",
      },
      { credentials: "same-origin" },
    );
  });

  it("encodes every legal filter exactly once", async () => {
    const client = transport();
    await loadImages(
      client,
      query({
        search: " 封面 ",
        category: " banner ",
        tags: " 活动, 主推 ,活动,,",
        onlyUnlabeled: true,
        offset: 48,
      }),
    );
    expect(client.list).toHaveBeenCalledWith(
      {
        limit: String(IMAGE_LIBRARY_PAGE_SIZE),
        offset: "48",
        enabled_only: "true",
        q: "封面",
        category: "banner",
        tags: "活动,主推",
        only_unlabeled: "true",
      },
      { credentials: "same-origin" },
    );
    expect(vi.mocked(client.list).mock.calls[0][0]).not.toHaveProperty(
      "tag_group",
    );
  });

  it("omits empty filters and clamps invalid offsets", async () => {
    const client = transport();
    await loadImages(client, query({ search: "   ", tags: ", ," }));
    expect(client.list).toHaveBeenCalledWith(
      {
        limit: String(IMAGE_LIBRARY_PAGE_SIZE),
        offset: "0",
        enabled_only: "true",
      },
      { credentials: "same-origin" },
    );
    await loadImages(client, query({ offset: -12 }));
    expect(vi.mocked(client.list).mock.calls[1][0]).toMatchObject({
      offset: "0",
    });
    await loadImages(client, query({ offset: Number.NaN }));
    expect(vi.mocked(client.list).mock.calls[2][0]).toMatchObject({
      offset: "0",
    });
  });

  it("rejects malformed success envelopes instead of guessing", async () => {
    const malformed: Record<string, unknown>[] = [
      { ...listPage, extra: 1 },
      { ...listPage, ok: false },
      { ...listPage, count: 2 },
      { ...listPage, next_offset: -1 },
      { ...listPage, fallback_used: true },
      { ...listPage, source_status: "legacy_proxy" },
      { ...listPage, route_owner: "legacy" },
      { ...listPage, storage_adapter_mode: "memory" },
      { ...listPage, adapter_mode: "s3" },
      { ...listPage, real_external_call_executed: true },
      { ...listPage, limit: 501 },
      { ...listPage, items: [{ ...imageItem, extra: 1 }] },
      { ...listPage, items: "nope" },
    ];
    const withoutKey: Record<string, unknown> = { ...listPage };
    delete withoutKey.has_more;
    malformed.push(withoutKey);
    for (const data of malformed) {
      const result = await loadImages(transport({ status: 200, data }), query());
      expect(result, JSON.stringify(data)).toEqual({ status: "unavailable" });
    }
  });

  it("rejects cross-field contradictions in the page envelope", async () => {
    const contradictory: Record<string, unknown>[] = [
      // count exceeds the echoed limit
      {
        ...listPage,
        items: [itemWithId(11), itemWithId(12)],
        total: 5,
        limit: 1,
        count: 2,
        has_more: true,
        next_offset: 2,
      },
      // count exceeds the total
      { ...listPage, total: 0 },
      // empty page before the end of the result window
      { ...listPage, items: [], total: 5, count: 0, has_more: true, next_offset: 0 },
      // nonempty page that does not fit inside the total window
      { ...listPage, total: 1, offset: 1, has_more: false, next_offset: null },
      // has_more=true with a wrong next_offset
      { ...listPage, total: 5, has_more: true, next_offset: 99 },
      // has_more=true with next_offset null
      { ...listPage, total: 5, has_more: true, next_offset: null },
      // has_more=true on the exact last page
      { ...listPage, has_more: true, next_offset: 1 },
      // has_more=false while more items remain
      { ...listPage, total: 5, has_more: false, next_offset: null },
      // has_more=false with a non-null next_offset
      { ...listPage, total: 1, has_more: false, next_offset: 1 },
    ];
    for (const data of contradictory) {
      const result = await loadImages(
        transport({ status: 200, data }),
        query(),
      );
      expect(result, JSON.stringify(data)).toEqual({ status: "unavailable" });
    }
  });

  it("accepts legal first, middle, last, and past-the-end pages", async () => {
    const middle = await loadImages(
      transport({
        status: 200,
        data: {
          ...listPage,
          total: 25,
          has_more: true,
          next_offset: 1,
        },
      }),
      query(),
    );
    expect(middle).toMatchObject({
      status: "loaded",
      total: 25,
      count: 1,
      hasMore: true,
      nextOffset: 1,
    });

    const last = await loadImages(
      transport({
        status: 200,
        data: {
          ...listPage,
          total: 25,
          offset: 24,
          has_more: false,
          next_offset: null,
        },
      }),
      query({ offset: 24 }),
    );
    expect(last).toMatchObject({
      status: "loaded",
      offset: 24,
      count: 1,
      hasMore: false,
    });

    const emptyPastEnd = await loadImages(
      transport({
        status: 200,
        data: {
          ...listPage,
          items: [],
          total: 20,
          offset: 48,
          count: 0,
          has_more: false,
          next_offset: null,
        },
      }),
      query({ offset: 48 }),
    );
    expect(emptyPastEnd).toMatchObject({
      status: "loaded",
      items: [],
      offset: 48,
      count: 0,
      hasMore: false,
    });

    const emptyLibrary = await loadImages(
      transport({
        status: 200,
        data: {
          ...listPage,
          items: [],
          total: 0,
          offset: 0,
          count: 0,
          has_more: false,
          next_offset: null,
        },
      }),
      query(),
    );
    expect(emptyLibrary).toMatchObject({ status: "loaded", total: 0 });
  });

  it("maps HTTP and network failures to the frozen status set", async () => {
    const expectations: [number, string][] = [
      [401, "unauthenticated"],
      [403, "forbidden"],
      [400, "invalid"],
      [422, "invalid"],
      [409, "conflict"],
      [500, "unavailable"],
      [503, "unavailable"],
    ];
    for (const [status, expected] of expectations) {
      const result = await loadImages(
        transport({ status, data: {} }),
        query(),
      );
      expect(result, String(status)).toEqual({ status: expected });
    }
    const throwing: ImageLibraryTransport = {
      list: vi.fn(async () => {
        throw new Error("network down");
      }),
      facets: vi.fn(),
      upload: vi.fn(),
    } as unknown as ImageLibraryTransport;
    expect(await loadImages(throwing, query())).toEqual({
      status: "unavailable",
    });
    expect(throwing.list).toHaveBeenCalledTimes(1);
  });
});

describe("image facets transport", () => {
  it("loads strict facets including legitimately empty arrays", async () => {
    const client = transport({ status: 200, data: facetsPage });
    expect(await loadFacets(client)).toEqual({
      status: "loaded",
      facets: { categories: ["banner"], tags: ["活动"] },
    });
    expect(client.facets).toHaveBeenCalledWith({
      credentials: "same-origin",
    });

    const empty = transport({
      status: 200,
      data: { ...facetsPage, categories: [], tags: [] },
    });
    expect(await loadFacets(empty)).toEqual({
      status: "loaded",
      facets: { categories: [], tags: [] },
    });
  });

  it("never fabricates empty facets on failure", async () => {
    const malformed: Record<string, unknown>[] = [
      { ...facetsPage, extra: 1 },
      { ...facetsPage, ok: false },
      { ...facetsPage, categories: ["banner", 1] },
      { ...facetsPage, categories: "banner" },
      { ...facetsPage, tags: ["x".repeat(65)] },
      { ...facetsPage, fallback_used: true },
      { ...facetsPage, source_status: "local_upload" },
    ];
    const missing: Record<string, unknown> = { ...facetsPage };
    delete missing.tags;
    malformed.push(missing);
    for (const data of malformed) {
      expect(
        await loadFacets(transport({ status: 200, data })),
        JSON.stringify(data),
      ).toEqual({ status: "unavailable" });
    }
    expect(await loadFacets(transport({ status: 401, data: {} }))).toEqual({
      status: "unauthenticated",
    });
    expect(await loadFacets(transport({ status: 403, data: {} }))).toEqual({
      status: "forbidden",
    });
    expect(await loadFacets(transport({ status: 500, data: {} }))).toEqual({
      status: "unavailable",
    });
    const throwing: ImageLibraryTransport = {
      list: vi.fn(),
      facets: vi.fn(async () => {
        throw new Error("offline");
      }),
      upload: vi.fn(),
    } as unknown as ImageLibraryTransport;
    expect(await loadFacets(throwing)).toEqual({ status: "unavailable" });
  });
});

describe("pagination helpers", () => {
  it("steps back by the fixed page size without going below zero", () => {
    expect(previousPageOffset(0)).toBe(0);
    expect(previousPageOffset(-24)).toBe(0);
    expect(previousPageOffset(IMAGE_LIBRARY_PAGE_SIZE)).toBe(0);
    expect(previousPageOffset(IMAGE_LIBRARY_PAGE_SIZE * 2)).toBe(
      IMAGE_LIBRARY_PAGE_SIZE,
    );
    expect(previousPageOffset(IMAGE_LIBRARY_PAGE_SIZE + 6)).toBe(6);
    expect(previousPageOffset(1.5)).toBe(0);
  });

  it("trusts only server pagination facts", () => {
    expect(
      nextPageOffset({ offset: 0, count: 24, hasMore: false }),
    ).toBeUndefined();
    expect(
      nextPageOffset({ offset: 0, count: 24, hasMore: true, nextOffset: 24 }),
    ).toBe(24);
    expect(nextPageOffset({ offset: 24, count: 24, hasMore: true })).toBe(48);
    expect(
      nextPageOffset({ offset: 0, count: 0, hasMore: true, nextOffset: 0 }),
    ).toBe(0);
  });

  it("resets only the offset when returning to the first page", () => {
    expect(
      firstPageQuery(
        query({
          search: "封面",
          category: "banner",
          tags: "活动",
          onlyUnlabeled: true,
          offset: 96,
        }),
      ),
    ).toEqual({
      search: "封面",
      category: "banner",
      tags: "活动",
      onlyUnlabeled: true,
      offset: 0,
    });
  });
});

describe("upload transport", () => {
  it("rejects unsupported files before any request", async () => {
    const client = transport({ status: 200, data: uploadSuccess });
    for (const file of [
      { type: "image/webp", size: 10 } as Blob,
      { type: "image/png", size: 0 } as Blob,
      { type: "image/png", size: MAX_IMAGE_FILE_SIZE + 1 } as Blob,
      { type: "", size: 10 } as Blob,
    ]) {
      expect(
        await uploadImage(
          client,
          CSRF_COOKIE,
          file,
          metadata,
          IDEMPOTENCY_KEY,
        ),
      ).toEqual({ status: "invalid" });
    }
    expect(client.upload).not.toHaveBeenCalled();
  });

  it("rejects out-of-contract metadata and idempotency keys before any request", async () => {
    const client = transport({ status: 200, data: uploadSuccess });
    expect(
      await uploadImage(client, CSRF_COOKIE, pngFile, {
        ...metadata,
        name: "x".repeat(201),
      }, IDEMPOTENCY_KEY),
    ).toEqual({ status: "invalid" });
    expect(
      await uploadImage(client, CSRF_COOKIE, pngFile, {
        ...metadata,
        category: "x".repeat(201),
      }, IDEMPOTENCY_KEY),
    ).toEqual({ status: "invalid" });
    expect(
      await uploadImage(client, CSRF_COOKIE, pngFile, {
        ...metadata,
        description: "x".repeat(10001),
      }, IDEMPOTENCY_KEY),
    ).toEqual({ status: "invalid" });
    expect(
      await uploadImage(client, CSRF_COOKIE, pngFile, metadata, "short"),
    ).toEqual({ status: "invalid" });
    expect(
      await uploadImage(
        client,
        CSRF_COOKIE,
        pngFile,
        metadata,
        "k".repeat(129),
      ),
    ).toEqual({ status: "invalid" });
    expect(client.upload).not.toHaveBeenCalled();
  });

  it("fails closed without a single valid CSRF cookie", async () => {
    const client = transport({ status: 200, data: uploadSuccess });
    for (const cookie of [
      "",
      "other=1",
      `aicrm_csrf=too-short`,
      `aicrm_csrf=${CSRF_TOKEN}; aicrm_csrf=${CSRF_TOKEN}`,
      `aicrm_csrf=${CSRF_TOKEN}!`,
    ]) {
      expect(
        await uploadImage(
          client,
          cookie,
          pngFile,
          metadata,
          IDEMPOTENCY_KEY,
        ),
        cookie,
      ).toEqual({ status: "csrf_missing" });
    }
    expect(client.upload).not.toHaveBeenCalled();
  });

  it("sends only the frozen multipart fields with CSRF and idempotency headers", async () => {
    const client = transport({ status: 200, data: uploadSuccess });
    const result = await uploadImage(
      client,
      `other=1; aicrm_csrf=${CSRF_TOKEN}`,
      pngFile,
      {
        name: " 封面 ",
        description: "首页主图",
        tags: " 活动, 主推 ,活动,",
        category: " banner ",
      },
      IDEMPOTENCY_KEY,
    );
    expect(result).toEqual({
      status: "uploaded",
      image: {
        id: 12,
        name: "cover.png",
        fileName: "cover.png",
        fileSize: 1024,
        width: 400,
        height: 300,
      },
    });
    expect(client.upload).toHaveBeenCalledTimes(1);
    const [body, options] = vi.mocked(client.upload).mock.calls[0];
    expect(body).toEqual({
      image: pngFile,
      name: "封面",
      description: "首页主图",
      tags: "活动,主推",
      category: "banner",
    });
    expect(options).toEqual({
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": CSRF_TOKEN,
        "Idempotency-Key": IDEMPOTENCY_KEY,
      },
    });
    expect(options?.headers).not.toHaveProperty("Content-Type");
  });

  it("omits empty metadata fields", async () => {
    const client = transport({ status: 200, data: uploadSuccess });
    await uploadImage(
      client,
      CSRF_COOKIE,
      pngFile,
      { name: "  ", description: "", tags: " ,", category: "" },
      IDEMPOTENCY_KEY,
    );
    expect(vi.mocked(client.upload).mock.calls[0][0]).toEqual({
      image: pngFile,
    });
  });

  it("maps upload failures to safe statuses without leaking backend text", async () => {
    const errorEnvelope = {
      ok: false,
      error: "pq: duplicate key value violates unique constraint",
      source_status: "next_media_library_error",
      route_owner: "ai_crm_next",
      fallback_used: false,
      real_external_call_executed: false,
    };
    const expectations: [number, unknown, string][] = [
      [400, errorEnvelope, "invalid"],
      [401, {}, "unauthenticated"],
      [403, {}, "forbidden"],
      [409, {}, "conflict"],
      [422, {}, "invalid"],
      [500, {}, "unavailable"],
    ];
    for (const [status, data, expected] of expectations) {
      const client = transport({ status: status as number, data });
      expect(
        await uploadImage(
          client,
          CSRF_COOKIE,
          pngFile,
          metadata,
          IDEMPOTENCY_KEY,
        ),
        String(status),
      ).toEqual({ status: expected });
      expect(client.upload).toHaveBeenCalledTimes(1);
    }
  });

  it("rejects upload success items with malformed timestamps", async () => {
    const malformed = [
      "2026-13-01T12:00:00Z",
      "2026-02-30T12:00:00Z",
      "2026-08-17 12:00:00Z",
      "2026-08-17T12:00:00",
      "2026-08-17T25:00:00Z",
      "not-a-date",
      "",
    ];
    for (const bad of malformed) {
      const client = transport({
        status: 200,
        data: {
          ...uploadSuccess,
          item: { ...uploadSuccess.item, created_at: bad },
        },
      });
      expect(
        await uploadImage(
          client,
          CSRF_COOKIE,
          pngFile,
          metadata,
          IDEMPOTENCY_KEY,
        ),
        bad,
      ).toEqual({ status: "unavailable" });
      const clientUpdated = transport({
        status: 200,
        data: {
          ...uploadSuccess,
          item: { ...uploadSuccess.item, updated_at: bad },
        },
      });
      expect(
        await uploadImage(
          clientUpdated,
          CSRF_COOKIE,
          pngFile,
          metadata,
          IDEMPOTENCY_KEY,
        ),
        bad,
      ).toEqual({ status: "unavailable" });
    }
  });

  it("treats malformed success payloads and network errors as unknown without retrying", async () => {
    const malformed = transport({
      status: 200,
      data: { ...uploadSuccess, source_status: "next_media_library" },
    });
    expect(
      await uploadImage(
        malformed,
        CSRF_COOKIE,
        pngFile,
        metadata,
        IDEMPOTENCY_KEY,
      ),
    ).toEqual({ status: "unavailable" });
    expect(malformed.upload).toHaveBeenCalledTimes(1);

    const badItem = transport({
      status: 200,
      data: { ...uploadSuccess, item: { ...uploadSuccess.item, extra: 1 } },
    });
    expect(
      await uploadImage(
        badItem,
        CSRF_COOKIE,
        pngFile,
        metadata,
        IDEMPOTENCY_KEY,
      ),
    ).toEqual({ status: "unavailable" });

    const throwing: ImageLibraryTransport = {
      list: vi.fn(),
      facets: vi.fn(),
      upload: vi.fn(async () => {
        throw new Error("connection reset");
      }),
    } as unknown as ImageLibraryTransport;
    expect(
      await uploadImage(
        throwing,
        CSRF_COOKIE,
        pngFile,
        metadata,
        IDEMPOTENCY_KEY,
      ),
    ).toEqual({ status: "unavailable" });
    expect(throwing.upload).toHaveBeenCalledTimes(1);
  });
});

describe("input helpers", () => {
  it("normalizes tags input by trimming, dropping empties, and deduping", () => {
    expect(normalizeTagsInput(" a, b ,a,, c ,")).toBe("a,b,c");
    expect(normalizeTagsInput("")).toBe("");
    expect(normalizeTagsInput(" , ,")).toBe("");
    expect(normalizeTagsInput("活动，主推")).toBe("活动，主推");
  });

  it("mints unique idempotency keys within the frozen length window", () => {
    const first = uploadIdempotencyKey();
    const second = uploadIdempotencyKey();
    expect(first).not.toBe(second);
    for (const key of [first, second]) {
      expect(key.length).toBeGreaterThanOrEqual(16);
      expect(key.length).toBeLessThanOrEqual(128);
      expect(key).toMatch(/^image-upload-[a-z0-9]+-[a-z0-9]{12}$/);
    }
    expect(uploadIdempotencyKey(() => 0.123456789)).toMatch(
      /^image-upload-[a-z0-9]+-[a-z0-9]{12}$/,
    );
  });

  it("formats file sizes for display", () => {
    expect(formatFileSize(512)).toBe("512 B");
    expect(formatFileSize(2048)).toBe("2.0 KiB");
    expect(formatFileSize(5 * 1024 * 1024)).toBe("5.0 MiB");
    expect(formatFileSize(-1)).toBe("未知");
  });
});
