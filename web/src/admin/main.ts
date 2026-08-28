/**
 * 后台页面入口：按 <body data-page> 分发。
 *  - 富交互页（雷达 / AI 助手 / 漏斗）→ sections/* 独立模块（真实 DOM + AdminApi）
 *  - 其余 28 屏 → mini-runtime 模板 + AdminController
 * 每页 HTML 由 scripts/build.mjs 生成（静态导航 shell）。
 */
import { mount } from '../shared/ui/runtime';
import { initFeedback } from '../shared/ui/feedback';
import { api } from '../shared/api/client';
import { AdminController } from './controller';
import { mountRadar } from './sections/radar';
import { mountAiAssistant } from './sections/aiAssistant';
import { mountFunnelGrid } from './sections/funnelGrid';
import { mountHXCHistory } from './sections/hxcHistory';
import { mountCampaignWorkspace } from './sections/campaigns';
import { mountCampaignHistory } from './sections/campaignHistory';
import { mountAdminAccess } from './sections/adminAccess';
import { mountSetupWizard } from './sections/setupWizard';
import { mountGroupOpsHistory } from './sections/groupOpsHistory';
import { esc } from './sections/util';
import { mountCouponData, mountCouponForm, mountServicePeriodMemberGrid } from './sections/commerce';
import { mountServicePeriodHistory } from './sections/servicePeriodHistory';
import { mountCouponHistory } from './sections/couponHistory';
import { mountMessageHistory } from './sections/messageHistory';
import { mountAudienceHistory } from './sections/audienceHistory';
import { renderLegacyMarketingHistory } from './sections/legacyMarketingHistory';
import { mountProfileCatalogHistory } from './sections/profileCatalogHistory';
import { mountAutomationHistory } from './sections/automationHistory';
import { mountRadarMarketingHistory } from './sections/radarMarketingHistory';
import { mountMemberGridHistory } from './sections/memberGridHistory';
import { mountContactHistory } from './sections/contactHistory';
import { mountSurveyUnresolvedHistory } from './sections/surveyUnresolvedHistory';
import { surveyUnresolvedHistoryHttp } from '../api/surveyUnresolvedHistoryHttp';
import { mountStaticHistory } from './sections/staticHistory';
import { mountCustomerStateHistory } from './sections/customerStateHistory';
import { mountMarketingStateHistory } from './sections/marketingStateHistory';
import { mountBroadcastJobHistory } from './sections/broadcastJobHistory';
import { mountOutboundTaskHistory } from './sections/outboundTaskHistory';
import { mountWeComContactHistory } from './sections/wecomContactHistory';

function showLoadError(stage: HTMLElement, error: unknown): void {
  stage.innerHTML = `<div style="margin:32px;padding:24px;border:1px solid #F2B8B5;border-radius:8px;color:#D83931;background:#FFF1F0">${error instanceof Error ? error.message : '页面数据读取失败'}</div>`;
}

function boot(): void {
  const page = document.body.getAttribute('data-page') || 'customers';
  const stage = document.getElementById('stage');
  if (!stage) return;

  const historyQuery = new URLSearchParams(location.search);
  if (page === 'automation' && historyQuery.get('outbound_task_history') === '1') {
    void mountOutboundTaskHistory(stage, { historyID: historyQuery.get('history_id') ?? undefined }).catch(() => { stage.innerHTML = '<p role="alert">外发任务历史读取失败；未创建或发送任务。</p>'; });
    return;
  }
  if (page === 'questionnaires' && historyQuery.get('unresolved_history') === '1') {
    void mountSurveyUnresolvedHistory(stage, surveyUnresolvedHistoryHttp, {
      historyID: historyQuery.get('history_id') ?? undefined,
    }).catch((error) => showLoadError(stage, error));
    return;
  }
  if (page === 'automation' && historyQuery.get('legacy_marketing_history') === '1') {
    void renderLegacyMarketingHistory(stage).catch(() => { stage.innerHTML = '<section data-legacy-marketing-history><h1>V1 旧版营销历史（只读）</h1><p role="alert">历史数据读取失败；未更改当前分层。</p></section>'; });
    return;
  }
  if (page === 'automation' && historyQuery.get('broadcast_job_history') === '1') {
    void mountBroadcastJobHistory(stage, { historyID: historyQuery.get('history_id') ?? undefined }).catch(() => { stage.innerHTML = '<p role="alert">群发任务历史读取失败；未创建或发送任务。</p>'; });
    return;
  }
  if (page === 'config' && historyQuery.get('marketing_state_history') === '1') {
    void mountMarketingStateHistory(stage).catch(() => { stage.innerHTML = '<p role="alert">营销状态历史读取失败；未进入当前配置。</p>'; });
    return;
  }
  if (page === 'config' && historyQuery.get('customer_state_history') === '1') {
    void mountCustomerStateHistory(stage).catch(() => { stage.innerHTML = '<p role="alert">客户状态历史读取失败；未进入当前配置。</p>'; });
    return;
  }
  if (page === 'config' && historyQuery.get('static_history') === '1') {
    void mountStaticHistory(stage).catch(() => { stage.innerHTML = '<p role="alert">静态历史读取失败；未进入当前配置。</p>'; });
    return;
  }
  if (page === 'funnel' && historyQuery.get('hxc_history') === '1') {
    void mountHXCHistory(stage, {
      kind: historyQuery.get('history_kind') ?? undefined,
      historyID: historyQuery.get('history_id') ?? undefined,
    }).catch((error) => showLoadError(stage, error));
    return;
  }
  if ((page === 'radar' && historyQuery.get('click_history') === '1') || (page === 'ai' && historyQuery.get('marketing_config_history') === '1')) {
    void mountRadarMarketingHistory(stage, {
      kind: page === 'radar' ? 'radar_click' : historyQuery.get('history_kind') ?? 'marketing_config',
      historyID: historyQuery.get('history_id') ?? undefined,
    }).catch(() => { stage.innerHTML = '<p role="alert">历史参数或读取失败；未进入当前业务。</p>'; });
    return;
  }
  if (page === 'config' && historyQuery.get('automation_history') === '1') {
    void mountAutomationHistory(stage, {
      kind: historyQuery.get('history_kind') ?? undefined,
      historyID: historyQuery.get('history_id') ?? undefined,
    }).catch(() => { stage.innerHTML = '<section data-automation-history><h1>V1 自动化历史（只读）</h1><p role="alert">历史参数或读取失败；未进入当前配置。</p></section>'; });
    return;
  }

  const rawId = Number(new URLSearchParams(location.search).get('id') || '');
  const id = rawId || undefined;

  const historyParams = new URLSearchParams(location.search);
  if (page === 'config' && historyParams.get('wecom_contact_history') === '1') {
    void mountWeComContactHistory(stage, {
      kind: historyParams.get('history_kind') ?? undefined,
      historyID: historyParams.get('history_id') ?? undefined,
    }).catch((error) => showLoadError(stage, error));
    return;
  }
  if (page === 'ownerMig' && historyParams.get('contact_history') === '1') {
    void mountContactHistory(stage, {
      kind: historyParams.get('history_kind') ?? undefined,
      historyID: historyParams.get('history_id') ?? undefined,
      customerID: historyParams.get('customer_id') ?? undefined,
    }).catch((error) => showLoadError(stage, error));
    return;
  }

  if (page === 'spProductData' && historyParams.get('member_grid_history') === '1') {
    void mountMemberGridHistory(stage, {
      kind: historyParams.get('history_kind') ?? undefined,
      historyID: historyParams.get('history_id') ?? undefined,
      productID: historyParams.get('product_id') ?? undefined,
      customerID: historyParams.get('customer_id') ?? undefined,
    }).catch((error) => showLoadError(stage, error));
    return;
  }

  const qs = new URLSearchParams(location.search);
  if (page === 'customers' && qs.get('message_history') === '1') {
    void mountMessageHistory(stage, {
      historyID: qs.get('history_message_id') ?? undefined,
      customerID: qs.get('customer_id') ?? undefined,
    }).catch((error) => showLoadError(stage, error));
    return;
  }

  if (page === 'config' && qs.get('profile_catalog_history') === '1') {
    void mountProfileCatalogHistory(stage, {
      templateID: qs.get('history_template_id') ?? undefined,
      categoryID: qs.get('history_category_id') ?? undefined,
      view: qs.get('history_view') ?? undefined,
    }).catch((error) => showLoadError(stage, error));
    return;
  }

  if (page === 'campaigns' && new URLSearchParams(location.search).get('history') === '1') {
    void mountCampaignHistory(stage).catch((error) => showLoadError(stage, error));
    return;
  }

  if (['coupons', 'couponData'].includes(page) && new URLSearchParams(location.search).get('history') === '1') {
    const historyID = page === 'couponData' ? new URLSearchParams(location.search).get('id') || '' : undefined;
    void mountCouponHistory(stage, historyID).catch((error) => showLoadError(stage, error));
    return;
  }
  if ((page === 'groupops' || page === 'groupopsDetail') && new URLSearchParams(location.search).get('history') === '1') {
    void mountGroupOpsHistory(stage, { view: page === 'groupops' ? 'list' : 'detail', planID: new URLSearchParams(location.search).get('id') || undefined })
      .catch((error) => showLoadError(stage, error));
    return;
  }

  /* ---- 富交互页：模块自管理反馈（toast/confirmBox 均引自 feedback.ts） ---- */
  switch (page) {
    case 'spProducts':
      if (new URLSearchParams(location.search).get('history') === '1') {
        void mountServicePeriodHistory(stage).catch((error) => showLoadError(stage, error));
        return;
      }
      break;
    case 'automation': {
      const query = new URLSearchParams(location.search);
      if (query.get('history') === '1') {
        void mountAudienceHistory(stage, {
          packageID: query.get('history_package_id') ?? undefined,
          definitionID: query.get('history_definition_id') ?? undefined,
          ruleID: query.get('history_rule_id') ?? undefined,
        }).catch(() => { stage.innerHTML = '<section data-audience-history><h1>V1 历史（只读）</h1><p role="alert">历史数据读取失败；未进入当前人群管理。</p></section>'; });
        return;
      }
      break;
    }
    case 'radar':
      void mountRadar(stage, api, { view: 'list' }).then(() => { stage.insertAdjacentHTML('afterbegin', '<p><a href="radar.html?click_history=1">V1 Radar 历史点击（只读）</a></p>'); }).catch((error) => showLoadError(stage, error));
      return;
    case 'radarDetail':
      void mountRadar(stage, api, { view: 'detail', id }).catch((error) => showLoadError(stage, error));
      return;
    case 'radarForm':
      void mountRadar(stage, api, { view: 'form', id }).catch((error) => showLoadError(stage, error));
      return;
    case 'ai':
      void mountAiAssistant(stage, api, { view: 'list' }).then(() => { stage.insertAdjacentHTML('afterbegin', '<p><a href="ai.html?marketing_config_history=1">V1 营销自动化历史（只读）</a></p>'); }).catch((error) => showLoadError(stage, error));
      return;
    case 'aiDetail':
      void mountAiAssistant(stage, api, { view: 'detail', id }).catch((error) => showLoadError(stage, error));
      return;
    case 'funnel':
      void mountFunnelGrid(stage, api).catch((error) => showLoadError(stage, error))
        .finally(() => { stage.insertAdjacentHTML('afterbegin', '<p><a href="funnel.html?hxc_history=1">V1 HXC历史观察（只读）</a></p>'); });
      return;
    case 'campaigns':
      void mountCampaignWorkspace(stage).catch((error) => showLoadError(stage, error));
      return;
    case 'couponForm':
      if (api.mode === 'http') {
        void mountCouponForm(stage).catch((error) => showLoadError(stage, error));
        return;
      }
      break;
    case 'couponData':
      if (api.mode === 'http') {
        void mountCouponData(stage).catch((error) => showLoadError(stage, error));
        return;
      }
      break;
    case 'spProductData': {
      const historyID = new URLSearchParams(location.search).get('history');
      if (historyID !== null) {
        void mountServicePeriodHistory(stage, historyID).catch((error) => showLoadError(stage, error));
        return;
      }
      if (api.mode === 'http') {
        void mountServicePeriodMemberGrid(stage).catch((error) => showLoadError(stage, error));
        return;
      }
      void (async () => {
        const db = await api.loadDb({ page: 'spProductData', id: id == null ? undefined : String(id) });
        const list = db.rows.spProducts;
        const p = list[(id ?? 0) % Math.max(list.length, 1)] || list[0];
        await mountFunnelGrid(stage, api, {
          product: p ? { code: p.code, name: p.name, price: p.price, status: p.status } : undefined,
        });
      })().catch((error) => showLoadError(stage, error));
      return;
    }
  }

  /* ---- 模板页：mini-runtime + 全局反馈委托 ---- */
  const tpl = document.getElementById('tpl') as HTMLTemplateElement | null;
  if (!tpl) return;
  const controller = new AdminController(api, page);
  initFeedback();
  stage.textContent = '正在读取页面数据…';
  void controller.init()
    .then(async () => {
      mount(stage, tpl.innerHTML, controller);
      if (page === 'automation') {
        stage.insertAdjacentHTML('afterbegin', '<p><a href="automation.html?outbound_task_history=1">V1 外发任务历史（只读）</a></p>');
        stage.insertAdjacentHTML('afterbegin', '<p><a href="automation.html?legacy_marketing_history=1">V1 旧版营销快照（只读）</a></p>');
        stage.insertAdjacentHTML('afterbegin', '<p><a href="automation.html?broadcast_job_history=1">V1 群发任务历史（只读）</a></p>');
      }
      if (page === 'groupops') {
        stage.insertAdjacentHTML('afterbegin', '<p><a href="groupops.html?history=1">V1 群运营历史（只读）</a></p>');
      }
      if (page === 'config') {
        stage.insertAdjacentHTML('afterbegin', '<p><a href="config.html?marketing_state_history=1">V1 营销状态历史（只读）</a></p>');
        stage.insertAdjacentHTML('afterbegin', '<p><a href="config.html?customer_state_history=1">V1 客户状态历史（只读）</a></p>');
        stage.insertAdjacentHTML('afterbegin', '<p><a href="config.html?static_history=1">V1 静态历史（只读）</a></p>');
        stage.insertAdjacentHTML('afterbegin', '<p><a href="config.html?automation_history=1">V1 自动化历史（只读）</a></p>');
        stage.insertAdjacentHTML('afterbegin', '<p><a href="config.html?wecom_contact_history=1">V1 企微联系人历史（只读）</a></p>');
        const setupWizard = stage.querySelector<HTMLElement>('#setup-wizard-card');
        if (setupWizard) await mountSetupWizard(setupWizard);
        const adminAccess = stage.querySelector<HTMLElement>('#admin-access-card');
        if (adminAccess) await mountAdminAccess(adminAccess);
      }
    })
    .catch((error) => showLoadError(stage, error));
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', boot);
} else {
  boot();
}
