# AI-CRM 前端（TypeScript 版）

整套新前端能力的 TypeScript 重写：**管理后台 34 屏 + 企微侧边栏 + 用户端 H5 12 屏**。
视觉与交互 1:1 源自设计原型，数据层走当前 Go OpenAPI 生成客户端与统一同源 transport。
Mock 仅用于显式注入的 DOM 测试，浏览器运行态不会回退到 Mock。

## 快速开始

```bash
npm install
npm run dev        # 构建 + 启动预览（默认 http://localhost:7100，支持 -- --port 8080 --watch）
```

其他命令：

```bash
npm run build      # 仅构建到 dist/
npm run typecheck  # TypeScript strict 检查
npm run smoke      # 绑定冒烟（40 屏模板 ↔ 控制器 vals 全量校验）
npm run e2e        # jsdom 端到端渲染/交互断言（39 项）
npm run verify     # 以上全部
```

## 目录结构

```
src/
  shared/
    ui/runtime.ts    迷你模板运行时（{{ }} 插值 / sc-for / sc-if / setState 重渲染）
    ui/feedback.ts   全局反馈层（toast / 确认浮窗 / busy / 上传进度 / 按钮文案委托）
    ui/download.ts   CSV 导出（BOM 前缀，Excel 中文不乱码）
    ui/tokens.css    基础样式 + 三端 shell
    api/types.ts     领域模型类型（雷达 / AI 计划 / 漏斗 / 列表数据 / H5，字段对齐生产 API）
    api/mockData.ts  mock 种子数据（含确定性造数器，与原型内嵌数据一致）
    api/transport.ts 同源 cookie、CSRF、401/403、结构化错误统一 transport
    api/generated/   由当前 Go OpenAPI 生成，禁止手改
  admin/
    controller.ts    后台控制器（模板页业务逻辑与绑定值）
    main.ts          页面入口（按 body[data-page] 分发：富交互页 → sections，其余 → 模板）
    sections/        富交互模块：radar.ts（列表/详情/表单）、aiAssistant.ts（列表/详情/人员抽屉）、
                     funnelGrid.ts（多维表格：视图/筛选/分组/排序/分享/群发）+ labs.css
    templates/       28 屏模板（雷达 / AI / 漏斗 6 屏已由 sections 接管）
    registry.json    屏幕注册表（导航归属/级别）
    nav.json         侧边导航（分组/图标/文案）
  sidebar/           企微侧边栏（纯静态 + 反馈层）
  h5/
    controller.ts    H5 控制器（选项点选 / 逐题作答 / 授权报名支付流程）
    templates/       12 屏模板
scripts/
  build.mjs          构建：模板转换 + 每屏 HTML 生成 + esbuild 打包
  dev.mjs            预览服务器（--port --host --watch）
  smoke.mjs / e2e.mjs
dist/                构建产物（直接可静态托管）
```

## 后端接入约束

- URL、方法和 DTO 只可来自 `src/api/generated/health.ts`；不要在页面中拼接 API 路径。
- 所有 generated response 都要经 `unwrapGenerated` 解包，401/403 与结构化失败不能渲染为成功。
- 每屏行为的 `real`、`backend_blocked`、`presentation_only` 状态见 `src/api/capabilities.ts`；
  `backend_blocked` 必须显示不可执行，不能 toast 成功或定时跳转。

## 页面与路由约定

- 多页架构：每屏一个 HTML，导航为真实跳转；
- 二级详情经 query 传参：`radarDetail.html?id=2`、`radarForm.html?id=2`（编辑）、
  `radarForm.html`（新建）、`aiDetail.html?id=7`；
- 写操作（启停 / 审批 / 保存）经 API 层写穿，mock 下刷新页面状态保留（sessionStorage）。
