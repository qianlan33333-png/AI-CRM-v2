import {
  getSidebarMaterialThumbnailStatus,
  getSidebarAgentConfig,
  getStartSidebarOAuthUrl,
  getSidebarWorkbench,
  startSidebarOAuth,
  completeSidebarOAuth,
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

export type SidebarOAuthStartResponse = Awaited<
  ReturnType<typeof startSidebarOAuth>
>;
export type SidebarOAuthCallbackResponse = Awaited<
  ReturnType<typeof completeSidebarOAuth>
>;

export const sidebarApi = {
  mintContext: async (body: Parameters<typeof mintSidebarContext>[0]) =>
    unwrapGenerated(
      await mintSidebarContext(body, apiRequestOptions()),
    ) as SidebarContextResponse,
  agentConfig: async (url: string) =>
    unwrapGenerated(
      await getSidebarAgentConfig({ url }, apiRequestOptions()),
    ) as SidebarAgentConfigSignature,
  // OAuth endpoints intentionally return redirects. Keep their generated response
  // metadata so the browser can navigate to the provider without treating a 302 as
  // a successful local login.
  oauthStart: async (
    params: Parameters<typeof startSidebarOAuth>[0],
    init: RequestInit = {},
  ) =>
    startSidebarOAuth(
      params,
      apiRequestOptions({ ...init, redirect: "manual" }),
    ),
  oauthStartUrl: (params: Parameters<typeof startSidebarOAuth>[0]) =>
    getStartSidebarOAuthUrl(params),
  oauthCallback: async (
    params: Parameters<typeof completeSidebarOAuth>[0],
    init: RequestInit = {},
  ) =>
    completeSidebarOAuth(
      params,
      apiRequestOptions({ ...init, redirect: "manual" }),
    ),
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
    ),
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
