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
import { mountCampaignWorkspace } from './sections/campaigns';
import { esc } from './sections/util';
import { mountCouponData, mountCouponForm, mountServicePeriodMemberGrid } from './sections/commerce';

function showLoadError(stage: HTMLElement, error: unknown): void {
  stage.innerHTML = `<div style="margin:32px;padding:24px;border:1px solid #F2B8B5;border-radius:8px;color:#D83931;background:#FFF1F0">${error instanceof Error ? error.message : '页面数据读取失败'}</div>`;
}

function boot(): void {
  const page = document.body.getAttribute('data-page') || 'customers';
  const stage = document.getElementById('stage');
  if (!stage) return;

  const rawId = Number(new URLSearchParams(location.search).get('id') || '');
  const id = rawId || undefined;

  /* ---- 富交互页：模块自管理反馈（toast/confirmBox 均引自 feedback.ts） ---- */
  switch (page) {
    case 'radar':
      void mountRadar(stage, api, { view: 'list' }).catch((error) => showLoadError(stage, error));
      return;
    case 'radarDetail':
      void mountRadar(stage, api, { view: 'detail', id }).catch((error) => showLoadError(stage, error));
      return;
    case 'radarForm':
      void mountRadar(stage, api, { view: 'form', id }).catch((error) => showLoadError(stage, error));
      return;
    case 'ai':
      void mountAiAssistant(stage, api, { view: 'list' }).catch((error) => showLoadError(stage, error));
      return;
    case 'aiDetail':
      void mountAiAssistant(stage, api, { view: 'detail', id }).catch((error) => showLoadError(stage, error));
      return;
    case 'funnel':
      void mountFunnelGrid(stage, api).catch((error) => showLoadError(stage, error));
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
    .then(() => mount(stage, tpl.innerHTML, controller))
    .catch((error) => showLoadError(stage, error));
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', boot);
} else {
  boot();
}
