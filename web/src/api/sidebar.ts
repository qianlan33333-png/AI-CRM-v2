import {
  getSidebarMaterialThumbnailStatus,
  getSidebarAgentConfig,
  getCompleteSidebarOAuthUrl,
  getStartSidebarOAuthUrl,
  getSidebarWorkbench,
  listSidebarChatActivity,
  listSidebarMaterials,
  listSidebarOrders,
  listSidebarPeriodicOrders,
  listSidebarQuestionnaires,
  listSidebarTimeline,
  mintSidebarContext,
  updateSidebarPeriodicRemark,
  updateSidebarProfile,
  type SidebarChatActivityResponse,
  type SidebarAgentConfigSignature,
  type SidebarContextResponse,
  type SidebarQuestionnaireResponse,
  type SidebarProfileUpdateResponse,
  type SidebarTimelineResponse,
  type SidebarWorkbenchResponse,
} from "./generated/health";
import { apiRequestOptions, unwrapGenerated } from "./transport";

function scopedOptions(
  contextToken: string,
  init: RequestInit = {},
): RequestInit {
  return apiRequestOptions({
    ...init,
    headers: { ...init.headers, "X-Sidebar-Context-Token": contextToken },
  });
}

function newIdempotencyKey(): string {
  const randomUUID = globalThis.crypto?.randomUUID?.();
  return randomUUID
    ? `sidebar-profile-${randomUUID}`
    : `sidebar-profile-${Date.now()}-${Math.random().toString(36).slice(2)}`;
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
  oauthCallbackUrl: (params: Parameters<typeof getCompleteSidebarOAuthUrl>[0]) =>
    getCompleteSidebarOAuthUrl(params),
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
    idempotencyKey = newIdempotencyKey(),
  ) =>
    unwrapGenerated(
      await updateSidebarProfile(
        body,
        scopedOptions(contextToken, {
          headers: { "Idempotency-Key": idempotencyKey },
        }),
      ),
    ) as SidebarProfileUpdateResponse,
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
    ),
  periodicOrders: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarPeriodicOrders>[0],
  ) =>
    unwrapGenerated(
      await listSidebarPeriodicOrders(params, scopedOptions(contextToken)),
    ),
  updateRemark: async (
    contextToken: string,
    ...args: Parameters<typeof updateSidebarPeriodicRemark>
  ) =>
    unwrapGenerated(
      await updateSidebarPeriodicRemark(
        args[0],
        args[1],
        args[2],
        scopedOptions(contextToken),
      ),
    ),
  materials: async (
    contextToken: string,
    params?: Parameters<typeof listSidebarMaterials>[0],
  ) =>
    unwrapGenerated(
      await listSidebarMaterials(params, scopedOptions(contextToken)),
    ),
  thumbnail: async (contextToken: string, imageId: number) =>
    unwrapGenerated(
      await getSidebarMaterialThumbnailStatus(
        imageId,
        scopedOptions(contextToken),
      ),
    ),
};
