import {
  getSidebarMaterialThumbnailStatus,
  getSidebarWorkbench,
  listSidebarMaterials,
  listSidebarOrders,
  listSidebarPeriodicOrders,
  listSidebarQuestionnaires,
  mintSidebarContext,
  updateSidebarPeriodicRemark,
  updateSidebarProfile,
} from './generated/health';
import { apiRequestOptions, unwrapGenerated } from './transport';

function scopedOptions(contextToken: string, init: RequestInit = {}): RequestInit {
  return apiRequestOptions({ ...init, headers: { ...init.headers, 'X-Sidebar-Context-Token': contextToken } });
}

export const sidebarApi = {
  mintContext: async (body: Parameters<typeof mintSidebarContext>[0]) => unwrapGenerated(await mintSidebarContext(body, apiRequestOptions())),
  workbench: async (contextToken: string) => unwrapGenerated(await getSidebarWorkbench(scopedOptions(contextToken))),
  profile: async (contextToken: string, body: Parameters<typeof updateSidebarProfile>[0]) => unwrapGenerated(await updateSidebarProfile(body, scopedOptions(contextToken))),
  questionnaires: async (contextToken: string, params?: Parameters<typeof listSidebarQuestionnaires>[0]) => unwrapGenerated(await listSidebarQuestionnaires(params, scopedOptions(contextToken))),
  orders: async (contextToken: string, params?: Parameters<typeof listSidebarOrders>[0]) => unwrapGenerated(await listSidebarOrders(params, scopedOptions(contextToken))),
  periodicOrders: async (contextToken: string, params?: Parameters<typeof listSidebarPeriodicOrders>[0]) => unwrapGenerated(await listSidebarPeriodicOrders(params, scopedOptions(contextToken))),
  updateRemark: async (contextToken: string, ...args: Parameters<typeof updateSidebarPeriodicRemark>) => unwrapGenerated(await updateSidebarPeriodicRemark(args[0], args[1], args[2], scopedOptions(contextToken))),
  materials: async (contextToken: string, params?: Parameters<typeof listSidebarMaterials>[0]) => unwrapGenerated(await listSidebarMaterials(params, scopedOptions(contextToken))),
  thumbnail: async (contextToken: string, imageId: number) => unwrapGenerated(await getSidebarMaterialThumbnailStatus(imageId, scopedOptions(contextToken))),
};
