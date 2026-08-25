/**
 * 企微侧边栏入口：模板为纯静态（无绑定），只需安装全局反馈层。
 */
import { initFeedback } from '../shared/ui/feedback';
import { sidebarApi } from '../api/sidebar';
import type { SidebarChatActivityResponse, SidebarContextResponse, SidebarMaterialResponse, SidebarOrderResponse, SidebarPeriodicOrderResponse, SidebarQuestionnaireResponse, SidebarTimelineResponse, SidebarWorkbenchResponse } from '../api/generated/health';

function status(message: string, failed = false): void {
  const el = document.createElement('div');
  el.textContent = message;
  el.style.cssText = `position:sticky;top:44px;z-index:11;padding:8px 16px;font-size:12px;background:${failed ? '#FFF1F0' : '#EFF8FF'};color:${failed ? '#D83931' : '#245BDB'};border-bottom:1px solid ${failed ? '#F2B8B5' : '#D3E1FF'};`;
  document.body.prepend(el);
}

async function loadWorkbench(): Promise<void> {
  const query = new URLSearchParams(location.search);
  const externalUserid = query.get('external_userid');
  if (!externalUserid) {
    status('后端能力未就绪：缺少 external_userid，不能创建 Sidebar 上下文', true);
    return;
  }
  try {
    const context = await sidebarApi.mintContext({ external_userid: externalUserid }) as SidebarContextResponse;
    if (context.state !== 'ready' || !context.context_token) {
      status(`Sidebar 上下文不可用：${context.state}`, true);
      return;
    }
    const [workbench, questionnaires, orders, periodicOrders, materials, timeline, chatActivity] = await Promise.all([
      sidebarApi.workbench(context.context_token),
      sidebarApi.questionnaires(context.context_token),
      sidebarApi.orders(context.context_token),
      sidebarApi.periodicOrders(context.context_token),
      sidebarApi.materials(context.context_token),
      sidebarApi.timeline(context.context_token),
      sidebarApi.chatActivity(context.context_token),
    ]) as [SidebarWorkbenchResponse, SidebarQuestionnaireResponse, SidebarOrderResponse, SidebarPeriodicOrderResponse, SidebarMaterialResponse, SidebarTimelineResponse, SidebarChatActivityResponse];
    status(`已读取本地工作台：问卷 ${questionnaires.items.length}、订单 ${orders.total}、周期订单 ${periodicOrders.items.length}、素材 ${materials.total}、时间线 ${timeline.items.length}、聊天活动 ${chatActivity.items.length}；不含任何外部发送。`);
    document.body.dataset.sidebarCustomerId = String(workbench.profile.customer_id);
  } catch (error) {
    status(error instanceof Error ? error.message : 'Sidebar 工作台读取失败', true);
  }
}

function boot(): void {
  initFeedback();
  void loadWorkbench();
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', boot);
} else {
  boot();
}
