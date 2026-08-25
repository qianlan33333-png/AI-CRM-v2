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

function boot(): void {
  const page = document.body.getAttribute('data-page') || 'customers';
  const stage = document.getElementById('stage');
  if (!stage) return;

  const rawId = Number(new URLSearchParams(location.search).get('id') || '');
  const id = rawId || undefined;

  /* ---- 富交互页：模块自管理反馈（toast/confirmBox 均引自 feedback.ts） ---- */
  switch (page) {
    case 'radar':
      void mountRadar(stage, api, { view: 'list' });
      return;
    case 'radarDetail':
      void mountRadar(stage, api, { view: 'detail', id });
      return;
    case 'radarForm':
      void mountRadar(stage, api, { view: 'form', id });
      return;
    case 'ai':
      void mountAiAssistant(stage, api, { view: 'list' });
      return;
    case 'aiDetail':
      void mountAiAssistant(stage, api, { view: 'detail', id });
      return;
    case 'funnel':
      void mountFunnelGrid(stage, api);
      return;
    case 'spProductData': {
      void (async () => {
        const db = await api.loadDb();
        const list = db.rows.spProducts;
        const p = list[(id ?? 0) % Math.max(list.length, 1)] || list[0];
        await mountFunnelGrid(stage, api, {
          product: p ? { code: p.code, name: p.name, price: p.price, status: p.status } : undefined,
        });
      })();
      return;
    }
  }

  /* ---- 模板页：mini-runtime + 全局反馈委托 ---- */
  const tpl = document.getElementById('tpl') as HTMLTemplateElement | null;
  if (!tpl) return;
  const controller = new AdminController(api, page);
  mount(stage, tpl.innerHTML, controller);
  initFeedback();
  void controller.init();
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', boot);
} else {
  boot();
}
