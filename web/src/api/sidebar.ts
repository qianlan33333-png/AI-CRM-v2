import {
  bindSidebarPhone,
  getSidebarMaterialThumbnailStatus,
  getGetSidebarMaterialThumbnailPreviewUrl,
  getSidebarAgentConfig,
  getCompleteSidebarOAuthUrl,
  getStartSidebarOAuthUrl,
  getSidebarWorkbench,
  listSidebarChatActivity,
  listSidebarMaterials,
  listSidebarOrders,
  listSidebarPeriodicOrders,
  listSidebarQuestionnaires,
  listSidebarShareableProducts,
  listSidebarTimeline,
  mintSidebarContext,
  prepareSidebarImageTemporaryMedia,
  updateSidebarPeriodicRemark,
  updateSidebarProfile,
  type SidebarChatActivityResponse,
  type SidebarAgentConfigSignature,
  type SidebarContextResponse,
  type SidebarMaterialResponse,
  type SidebarOrderResponse,
  type SidebarPeriodicOrderResponse,
  type SidebarPeriodicRemarkResponse,
  type SidebarPhoneBindingResponse,
  type SidebarQuestionnaireResponse,
  type SidebarShareableProductResponse,
  type SidebarProfileUpdateResponse,
  type SidebarTemporaryMediaResponse,
  type SidebarThumbnailPendingResponse,
  type SidebarTimelineResponse,
  type SidebarWorkbenchResponse,
} from "./generated/health";
import { apiRequestOptions, request, unwrapGenerated } from "./transport";

function scopedOptions(
  contextToken: string,
  init: RequestInit = {},
): RequestInit {
  return apiRequestOptions({
    ...init,
    headers: { ...init.headers, "X-Sidebar-Context-Token": contextToken },
  });
}

export function newSidebarIdempotencyKey(scope: string): string {
  const randomUUID = globalThis.crypto?.randomUUID?.();
  return randomUUID
    ? `${scope}-${randomUUID}`
    : `${scope}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export const sidebarApi = {
  mintContext: async (body: Parameters<typeof mintSidebarContext>[0]) =>
    unwrapGenerated(
      await mintSidebarContext(body, apiRequestOptions()),
    ) as SidebarContextResponse,
  agentConfig: async (url: string) =>
    unwrapGenerated(
      await getSidebarAgentConfig({ url }, apiRequestOptions()),
    ) as SidebarAgentConfigSignature,
  // OAuth is a browser navigation protocol. Building the generated URL without
  // prefetching avoids creating duplicate state/binding records before redirect.
  oauthStartUrl: (params: Parameters<typeof getStartSidebarOAuthUrl>[0]) =>
    getStartSidebarOAuthUrl(params),
  oauthCallbackUrl: (
    params: Parameters<typeof getCompleteSidebarOAuthUrl>[0],
  ) => getCompleteSidebarOAuthUrl(params),
  workbench: async (contextToken: string) =>
    unwrapGenerated(
      await getSidebarWorkbench(scopedOptions(contextToken)),
    ) as SidebarWorkbenchResponse,
  timeline: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarTimeline>[0],
  ) =>
    unwrapGenerated(
      await listSidebarTimeline(params, scopedOptions(contextToken)),
    ) as SidebarTimelineResponse,
  chatActivity: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarChatActivity>[0],
  ) =>
    unwrapGenerated(
      await listSidebarChatActivity(params, scopedOptions(contextToken)),
    ) as SidebarChatActivityResponse,
  profile: async (
    contextToken: string,
    body: Parameters<typeof updateSidebarProfile>[0],
    idempotencyKey = newSidebarIdempotencyKey("sidebar-profile"),
  ) =>
    unwrapGenerated(
      await updateSidebarProfile(
        body,
        scopedOptions(contextToken, {
          headers: { "Idempotency-Key": idempotencyKey },
        }),
      ),
    ) as SidebarProfileUpdateResponse,
  bindPhone: async (
    contextToken: string,
    body: Parameters<typeof bindSidebarPhone>[0],
    idempotencyKey = newSidebarIdempotencyKey("sidebar-phone"),
  ) =>
    unwrapGenerated(
      await bindSidebarPhone(
        body,
        scopedOptions(contextToken, {
          headers: { "Idempotency-Key": idempotencyKey },
        }),
      ),
    ) as SidebarPhoneBindingResponse,
  questionnaires: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarQuestionnaires>[0],
  ) =>
    unwrapGenerated(
      await listSidebarQuestionnaires(params, scopedOptions(contextToken)),
    ) as SidebarQuestionnaireResponse,
  orders: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarOrders>[0],
  ) =>
    unwrapGenerated(
      await listSidebarOrders(params, scopedOptions(contextToken)),
    ) as SidebarOrderResponse,
  periodicOrders: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarPeriodicOrders>[0],
  ) =>
    unwrapGenerated(
      await listSidebarPeriodicOrders(params, scopedOptions(contextToken)),
    ) as SidebarPeriodicOrderResponse,
  updateRemark: async (
    contextToken: string,
    serviceProductId: number,
    memberRef: string,
    body: Parameters<typeof updateSidebarPeriodicRemark>[2],
    idempotencyKey = newSidebarIdempotencyKey("sidebar-periodic-remark"),
  ) =>
    unwrapGenerated(
      await updateSidebarPeriodicRemark(
        serviceProductId,
        memberRef,
        body,
        scopedOptions(contextToken, {
          headers: { "Idempotency-Key": idempotencyKey },
        }),
      ),
    ) as SidebarPeriodicRemarkResponse,
  materials: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarMaterials>[0],
  ) =>
    unwrapGenerated(
      await listSidebarMaterials(params, scopedOptions(contextToken)),
    ) as SidebarMaterialResponse,
  shareableProducts: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarShareableProducts>[0],
  ) =>
    unwrapGenerated(
      await listSidebarShareableProducts(params, scopedOptions(contextToken)),
    ) as SidebarShareableProductResponse,
  prepareTemporaryImage: async (
    contextToken: string,
    imageId: number,
    idempotencyKey: string,
  ) =>
    unwrapGenerated(
      await prepareSidebarImageTemporaryMedia(
        imageId,
        scopedOptions(contextToken, {
          headers: { "Idempotency-Key": idempotencyKey },
        }),
      ),
    ) as SidebarTemporaryMediaResponse,
  thumbnail: async (contextToken: string, imageId: number) =>
    unwrapGenerated(
      await getSidebarMaterialThumbnailStatus(
        imageId,
        scopedOptions(contextToken),
      ),
    ) as SidebarThumbnailPendingResponse,
  thumbnailPreview: async (contextToken: string, imageId: number) => {
    // Orval 7.21 currently parses multi-content binary responses as JSON even
    // though it emits Blob types. Keep the generated URL and shared transport,
    // then read the successful response as bytes.
    const response = await request(
      getGetSidebarMaterialThumbnailPreviewUrl(imageId),
      scopedOptions(contextToken),
    );
    return response.blob();
  },
};
