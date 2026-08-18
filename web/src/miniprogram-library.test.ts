import { describe, expect, it, vi } from "vitest";
import {
  deleteMiniProgram,
  draftProblem,
  editorDraft,
  imagePickerPreviousOffset,
  loadLibraryImages,
  loadMiniPrograms,
  parseLibraryImage,
  parseMiniProgram,
  parseThumbnailResolution,
  resolveMiniProgramThumbnail,
  saveMiniProgram,
  setMiniProgramEnabled,
  uploadLibraryImage,
  MINIPROGRAM_PAGE_SIZE,
  IMAGE_PICKER_PAGE_SIZE,
  type MiniProgramLibraryTransport,
} from "./miniprogram-library";

const flags = {
  local_only: true,
  provider_call_executed: false,
  real_external_call_executed: false,
};
const item = {
  id: 7,
  name: "卡片",
  appid: "wx-demo",
  pagepath: "pages/home",
  page_path: "pages/home",
  title: "首页",
  thumb_image_url: "",
  thumb_image_base64: "",
  thumb_media_id: "",
  thumb_image_id: 11,
  enabled: true,
  created_at: "2026-08-17T12:00:00Z",
  updated_at: "2026-08-17T12:00:00Z",
  created_by: 1,
  updated_by: 1,
  version: 1,
};
const page = {
  ok: true,
  items: [item],
  miniprograms: [item],
  total: 1,
  limit: MINIPROGRAM_PAGE_SIZE,
  offset: 0,
  ...flags,
};
const mutation = {
  ok: true,
  item,
  miniprogram: item,
  item_id: 7,
  changed: true,
  thumb_resolve: null,
  ...flags,
};
const deleted = { ok: true, id: 7, item_id: 7, deleted: true, ...flags };
const resolution = {
  status: "resolved",
  cache_owner: "media.thumbnail_cache",
  cache_receipt: "local-cache:hit",
  thumb_media_id: "media-1",
  side_effect_executed: false,
  real_external_call_executed: false,
};
const resolveResponse = {
  ok: true,
  item,
  miniprogram: item,
  resolution,
  changed: false,
  thumb_media_id: "media-1",
  ...flags,
};
const imageItem = {
  id: 11,
  name: "封面",
  file_name: "cover.png",
  mime_type: "image/png",
  file_size: 1024,
  enabled: true,
  description: "",
  tags: [],
  category: "",
  width: 400,
  height: 300,
  created_at: "2026-08-17T12:00:00Z",
  updated_at: "2026-08-17T12:00:00Z",
  thumb_160_url: "/api/admin/image-library/11/variants/thumb_160",
  thumb_320_url: "/api/admin/image-library/11/variants/thumb_320",
  thumb_url: "/api/admin/image-library/11/variants/thumb_320",
  preview_url: "/api/admin/image-library/11/variants/mobile_1080",
  mobile_1080_url: "/api/admin/image-library/11/variants/mobile_1080",
  large_1440_url: "/api/admin/image-library/11/variants/large_1440",
  original_url: "/api/admin/image-library/11/variants/original",
};
const imagePage = {
  ok: true,
  items: [imageItem],
  total: 1,
  limit: IMAGE_PICKER_PAGE_SIZE,
  offset: 0,
  count: 1,
  has_more: false,
  next_offset: null,
  source_status: "next_media_library",
  route_owner: "ai_crm_next",
  fallback_used: false,
  real_external_call_executed: false,
  storage_adapter_mode: "postgresql",
  adapter_mode: "postgresql",
};
const uploadResponse = {
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

function transport(
  response: { status: number; data: unknown } = { status: 200, data: page },
): MiniProgramLibraryTransport {
  const result = async () => response;
  return {
    list: vi.fn(result),
    create: vi.fn(result),
    update: vi.fn(result),
    remove: vi.fn(result),
    resolve: vi.fn(result),
    listImages: vi.fn(result),
    uploadImage: vi.fn(result),
  } as unknown as MiniProgramLibraryTransport;
}

describe("miniprogram response boundary", () => {
  it("parses only an exact frozen MiniProgram shape", () => {
    expect(parseMiniProgram(item)).toMatchObject({
      id: 7,
      appID: "wx-demo",
      pagePath: "pages/home",
      thumbImageID: 11,
    });
    expect(parseMiniProgram({ ...item, extra: true })).toBeUndefined();
    expect(
      parseMiniProgram({ ...item, page_path: "pages/other" }),
    ).toBeUndefined();
    expect(
      parseMiniProgram({ ...item, thumb_image_base64: "aGk=" }),
    ).toBeUndefined();
    expect(parseMiniProgram({ ...item, id: 0 })).toBeUndefined();
    expect(parseMiniProgram({ ...item, thumb_image_id: 0 })).toBeUndefined();
    expect(parseMiniProgram({ ...item, thumb_image_id: null })).toBeUndefined();
    const missing: Record<string, unknown> = { ...item };
    delete missing.version;
    expect(parseMiniProgram(missing)).toBeUndefined();
  });

  it("parses only the frozen thumbnail resolution contract", () => {
    expect(parseThumbnailResolution(resolution)).toEqual({
      status: "resolved",
      cacheOwner: "media.thumbnail_cache",
      cacheReceipt: "local-cache:hit",
      thumbMediaID: "media-1",
    });
    expect(
      parseThumbnailResolution({
        status: "outcome_unknown",
        cache_owner: "media.thumbnail_cache",
        cache_receipt: "local-cache:unknown",
        side_effect_executed: false,
        real_external_call_executed: false,
      }),
    ).toMatchObject({ status: "outcome_unknown" });
    expect(
      parseThumbnailResolution({ ...resolution, side_effect_executed: true }),
    ).toBeUndefined();
    expect(
      parseThumbnailResolution({
        ...resolution,
        real_external_call_executed: true,
      }),
    ).toBeUndefined();
    expect(
      parseThumbnailResolution({ ...resolution, cache_owner: "provider" }),
    ).toBeUndefined();
    expect(
      parseThumbnailResolution({ ...resolution, status: "uploaded" }),
    ).toBeUndefined();
  });

  it("parses only an exact frozen Image Library item", () => {
    expect(parseLibraryImage(imageItem)).toMatchObject({
      id: 11,
      thumb160URL: "/api/admin/image-library/11/variants/thumb_160",
    });
    expect(parseLibraryImage({ ...imageItem, extra: 1 })).toBeUndefined();
    expect(
      parseLibraryImage({
        ...imageItem,
        thumb_160_url: "https://evil.example/x.png",
      }),
    ).toBeUndefined();
    expect(
      parseLibraryImage({ ...imageItem, file_size: 10485761 }),
    ).toBeUndefined();
  });
});

describe("miniprogram list", () => {
  it("sends the exact frozen query DTO with same-origin credentials", async () => {
    const client = transport();
    await expect(
      loadMiniPrograms(client, { search: "", enabledOnly: false, offset: 0 }),
    ).resolves.toMatchObject({ status: "loaded", total: 1 });
    expect(client.list).toHaveBeenCalledWith(
      { limit: MINIPROGRAM_PAGE_SIZE, offset: 0, enabled_only: false },
      { credentials: "same-origin" },
    );

    const searched = transport();
    await loadMiniPrograms(searched, {
      search: "  卡片 ",
      enabledOnly: true,
      offset: 40,
    });
    expect(searched.list).toHaveBeenCalledWith(
      {
        limit: MINIPROGRAM_PAGE_SIZE,
        offset: 40,
        enabled_only: true,
        q: "卡片",
      },
      { credentials: "same-origin" },
    );
  });

  it("fails closed on malformed, mirrored-mismatched, or externally-flagged pages", async () => {
    await expect(
      loadMiniPrograms(
        transport({
          status: 200,
          data: { ...page, miniprograms: [{ ...item, id: 8 }] },
        }),
        { search: "", enabledOnly: false, offset: 0 },
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      loadMiniPrograms(
        transport({
          status: 200,
          data: { ...page, provider_call_executed: true },
        }),
        { search: "", enabledOnly: false, offset: 0 },
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      loadMiniPrograms(
        transport({
          status: 200,
          data: { ...page, items: [{ ...item, extra: 1 }] },
        }),
        { search: "", enabledOnly: false, offset: 0 },
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      loadMiniPrograms(transport({ status: 401, data: {} }), {
        search: "",
        enabledOnly: false,
        offset: 0,
      }),
    ).resolves.toEqual({ status: "unauthenticated" });
    await expect(
      loadMiniPrograms(transport({ status: 503, data: {} }), {
        search: "",
        enabledOnly: false,
        offset: 0,
      }),
    ).resolves.toEqual({ status: "unavailable" });
  });
});

describe("miniprogram mutations", () => {
  const draft = {
    ...editorDraft(),
    name: "卡片",
    appID: "wx-demo",
    pagePath: "pages/home",
    title: "首页",
  };

  it("creates with the exact frozen DTO, CSRF, idempotency key, and same-origin", async () => {
    const client = transport({ status: 200, data: mutation });
    await expect(
      saveMiniProgram(client, undefined, draft, "csrf-token", "create-key"),
    ).resolves.toMatchObject({ status: "saved", item: { id: 7 } });
    expect(client.create).toHaveBeenCalledWith(
      {
        name: "卡片",
        appid: "wx-demo",
        pagepath: "pages/home",
        title: "首页",
      },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "csrf-token",
          "Idempotency-Key": "create-key",
        },
      },
    );
  });

  it("writes only server-issued positive thumb IDs and clears explicitly on update", async () => {
    const existing = parseMiniProgram(item)!;
    const client = transport({ status: 200, data: mutation });
    const withThumb = { ...editorDraft(existing), thumbImageID: 12 };
    await saveMiniProgram(
      client,
      existing,
      withThumb,
      "csrf-token",
      "update-key",
    );
    expect(client.update).toHaveBeenCalledWith(
      7,
      {
        name: "卡片",
        appid: "wx-demo",
        pagepath: "pages/home",
        title: "首页",
        thumb_image_id: 12,
      },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "csrf-token",
          "Idempotency-Key": "update-key",
        },
      },
    );

    const cleared = transport({ status: 200, data: mutation });
    const withoutThumb = {
      name: existing.name,
      appID: existing.appID,
      pagePath: existing.pagePath,
      title: existing.title,
    };
    await saveMiniProgram(
      cleared,
      existing,
      withoutThumb,
      "csrf-token",
      "update-key",
    );
    expect(cleared.update).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ thumb_image_id: null }),
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it("rejects invalid drafts without sending any request", async () => {
    const client = transport({ status: 200, data: mutation });
    await expect(
      saveMiniProgram(
        client,
        undefined,
        { ...draft, appID: "" },
        "csrf-token",
        "create-key",
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.create).not.toHaveBeenCalled();
    expect(draftProblem({ ...draft, name: "  " })).toBeDefined();
    expect(draftProblem({ ...draft, thumbImageID: 0 })).toBeDefined();
    expect(draftProblem(draft)).toBeUndefined();
  });

  it("treats mismatched or non-frozen success responses as unavailable", async () => {
    const existing = parseMiniProgram(item)!;
    await expect(
      saveMiniProgram(
        transport({
          status: 200,
          data: { ...mutation, item: { ...item, id: 8 }, item_id: 8 },
        }),
        existing,
        editorDraft(existing),
        "csrf-token",
        "update-key",
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      saveMiniProgram(
        transport({ status: 201, data: mutation }),
        undefined,
        draft,
        "csrf-token",
        "create-key",
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      saveMiniProgram(
        transport({ status: 409, data: {} }),
        undefined,
        draft,
        "csrf-token",
        "create-key",
      ),
    ).resolves.toEqual({ status: "conflict" });
    await expect(
      saveMiniProgram(
        transport({ status: 400, data: {} }),
        undefined,
        draft,
        "csrf-token",
        "create-key",
      ),
    ).resolves.toEqual({ status: "invalid" });
  });

  it("applies enable/disable only from the server response for the selected item", async () => {
    const off = { ...item, enabled: false };
    const client = transport({
      status: 200,
      data: { ...mutation, item: off, miniprogram: off },
    });
    await expect(
      setMiniProgramEnabled(client, 7, false, "csrf-token", "toggle-key"),
    ).resolves.toMatchObject({
      status: "saved",
      item: { id: 7, enabled: false },
    });
    expect(client.update).toHaveBeenCalledWith(
      7,
      { enabled: false },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "csrf-token",
          "Idempotency-Key": "toggle-key",
        },
      },
    );
    await expect(
      setMiniProgramEnabled(
        transport({ status: 200, data: mutation }),
        7,
        false,
        "csrf-token",
        "toggle-key",
      ),
    ).resolves.toEqual({ status: "unavailable" });
  });

  it("deletes only after an exact deleted receipt for the selected item", async () => {
    const client = transport({ status: 200, data: deleted });
    await expect(
      deleteMiniProgram(client, 7, "csrf-token", "delete-key"),
    ).resolves.toEqual({ status: "deleted" });
    expect(client.remove).toHaveBeenCalledWith(7, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": "csrf-token",
        "Idempotency-Key": "delete-key",
      },
    });
    await expect(
      deleteMiniProgram(
        transport({ status: 200, data: { ...deleted, id: 8, item_id: 8 } }),
        7,
        "csrf-token",
        "delete-key",
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      deleteMiniProgram(
        transport({ status: 200, data: { ...deleted, deleted: false } }),
        7,
        "csrf-token",
        "delete-key",
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      deleteMiniProgram(
        transport({ status: 404, data: {} }),
        7,
        "csrf-token",
        "delete-key",
      ),
    ).resolves.toEqual({ status: "not_found" });
  });

  it("resolves thumbnails only as a local-cache read with frozen flags", async () => {
    const client = transport({ status: 200, data: resolveResponse });
    await expect(
      resolveMiniProgramThumbnail(client, 7, "csrf-token", "resolve-key"),
    ).resolves.toMatchObject({
      status: "ok",
      resolution: { status: "resolved", thumbMediaID: "media-1" },
    });
    expect(client.resolve).toHaveBeenCalledWith(7, {
      credentials: "same-origin",
      headers: {
        "X-CSRF-Token": "csrf-token",
        "Idempotency-Key": "resolve-key",
      },
    });
    await expect(
      resolveMiniProgramThumbnail(
        transport({
          status: 200,
          data: {
            ...resolveResponse,
            resolution: { ...resolution, status: "outcome_unknown" },
          },
        }),
        7,
        "csrf-token",
        "resolve-key",
      ),
    ).resolves.toMatchObject({
      status: "ok",
      resolution: { status: "outcome_unknown" },
    });
    await expect(
      resolveMiniProgramThumbnail(
        transport({
          status: 200,
          data: { ...resolveResponse, real_external_call_executed: true },
        }),
        7,
        "csrf-token",
        "resolve-key",
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      resolveMiniProgramThumbnail(
        transport({ status: 404, data: {} }),
        7,
        "csrf-token",
        "resolve-key",
      ),
    ).resolves.toEqual({ status: "not_found" });
  });
});

describe("image library picker transport", () => {
  it("steps back by the fixed picker page size even on a short final page", () => {
    expect(imagePickerPreviousOffset(24)).toBe(0);
    expect(imagePickerPreviousOffset(48)).toBe(24);
    expect(imagePickerPreviousOffset(3)).toBe(0);
  });

  it("lists images with the frozen string scalar params and same-origin", async () => {
    const client = transport({ status: 200, data: imagePage });
    await expect(
      loadLibraryImages(client, { search: "封面", offset: 0 }),
    ).resolves.toMatchObject({ status: "loaded", total: 1, hasMore: false });
    expect(client.listImages).toHaveBeenCalledWith(
      {
        limit: String(IMAGE_PICKER_PAGE_SIZE),
        offset: "0",
        enabled_only: "true",
        q: "封面",
      },
      { credentials: "same-origin" },
    );
    await expect(
      loadLibraryImages(
        transport({
          status: 200,
          data: { ...imagePage, real_external_call_executed: true },
        }),
        { search: "", offset: 0 },
      ),
    ).resolves.toEqual({ status: "unavailable" });
    await expect(
      loadLibraryImages(transport({ status: 422, data: {} }), {
        search: "",
        offset: 0,
      }),
    ).resolves.toEqual({ status: "invalid" });
  });

  it("uploads only guarded local files and returns the server image ID", async () => {
    const file = new Blob(["png-bytes"], { type: "image/png" });
    const client = transport({ status: 200, data: uploadResponse });
    await expect(
      uploadLibraryImage(client, file, "cover.png", "csrf-token", "upload-key"),
    ).resolves.toEqual({
      status: "uploaded",
      image: { id: 12, name: "cover.png" },
    });
    expect(client.uploadImage).toHaveBeenCalledWith(
      { image: file, name: "cover.png" },
      {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "csrf-token",
          "Idempotency-Key": "upload-key",
        },
      },
    );
  });

  it("rejects non-image or oversized files without sending any request", async () => {
    const client = transport({ status: 200, data: uploadResponse });
    await expect(
      uploadLibraryImage(
        client,
        new Blob(["x"], { type: "text/plain" }),
        "a.txt",
        "csrf-token",
        "upload-key",
      ),
    ).resolves.toEqual({ status: "invalid" });
    const oversized = new Blob([new Uint8Array(10_485_761)], {
      type: "image/png",
    });
    await expect(
      uploadLibraryImage(
        client,
        oversized,
        "big.png",
        "csrf-token",
        "upload-key",
      ),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.uploadImage).not.toHaveBeenCalled();
    await expect(
      uploadLibraryImage(
        transport({ status: 400, data: {} }),
        new Blob(["x"], { type: "image/png" }),
        "x.png",
        "csrf-token",
        "upload-key",
      ),
    ).resolves.toEqual({ status: "invalid" });
  });
});
