/**
 * AI-CRM 前端构建脚本
 * 1. 屏幕模板 sc-for/sc-if → <template data-sc-*>（表安全的运行时指令）
 * 2. 后台：每屏生成独立 HTML（静态导航 shell + active 高亮 + 屏幕模板 + 全局浮层）
 * 3. 侧边栏 / H5：各自 shell
 * 4. esbuild 打包三端 TS 入口 → dist/assets/*.js
 * 5. 生成索引页（dist/index.html / admin / h5）
 */
import { build } from 'esbuild';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const SRC = path.join(ROOT, 'src');
const DIST = path.join(ROOT, 'dist');

const read = (p) => fs.readFileSync(p, 'utf8');
const write = (p, s) => {
  fs.mkdirSync(path.dirname(p), { recursive: true });
  fs.writeFileSync(p, s);
};

/* ---------- sc-* 模板转换（与原型 support.js 同规则） ---------- */
function transform(html) {
  let out = html
    .replace(
      /<sc-for\s+([^>]*?)list="([^"]*)"([^>]*?)as="([^"]*)"([^>]*)>/g,
      (_m, _a, list, _b, as) => `<template data-sc-for="${list}" data-as="${as}">`,
    )
    .replace(/<\/sc-for>/g, '</template>')
    .replace(/<sc-if\s+([^>]*?)value="([^"]*)"([^>]*)>/g, (_m, _a, val) => `<template data-sc-if="${val}">`)
    .replace(/<\/sc-if>/g, '</template>');
  if (out.includes('<sc-for') || out.includes('<sc-if')) {
    out = out
      .replace(/<sc-for([^>]*)>/g, (_m, attrs) => {
        const list = (attrs.match(/list="([^"]*)"/) || [])[1] || '';
        const as = (attrs.match(/as="([^"]*)"/) || [])[1] || 'item';
        return `<template data-sc-for="${list}" data-as="${as}">`;
      })
      .replace(/<sc-if([^>]*)>/g, (_m, attrs) => {
        const val = (attrs.match(/value="([^"]*)"/) || [])[1] || '';
        return `<template data-sc-if="${val}">`;
      });
  }
  return out;
}

/* ---------- 数据 ---------- */
const registry = JSON.parse(read(path.join(SRC, 'admin/registry.json')));
const navItems = JSON.parse(read(path.join(SRC, 'admin/nav.json')));
const h5Registry = JSON.parse(read(path.join(SRC, 'h5/registry.json')));

/** 富交互页（sections/* TS 模块渲染，不走模板） */
const RICH = new Set(['radar', 'radarDetail', 'radarForm', 'ai', 'aiDetail', 'funnel', 'spProductData', 'campaigns']);

/* ---------- 后台导航 ---------- */
function navHtml(activeNav) {
  let html = '';
  let lastGroup = null;
  for (const item of navItems) {
    if (item.group !== lastGroup) {
      html += `<div class="side-grp">${item.group}</div>\n`;
      lastGroup = item.group;
    }
    const on = item.key === activeNav ? ' on' : '';
    html += `<a class="nav-item${on}" href="${item.key}.html">${item.svg}<span>${item.label}</span></a>\n`;
  }
  return html;
}

function adminShell(screen, { rich }) {
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${screen.label} · AI-CRM 管理后台</title>
<link rel="stylesheet" href="../assets/tokens.css">
${rich ? '<link rel="stylesheet" href="../assets/labs.css">' : ''}
</head>
<body data-page="${screen.key}">
<div class="shell">
  <aside class="side">
    <div class="side-brand">
      <div class="mark">CRM</div>
      <div><div class="name">客户管理后台</div><div class="en">ADMIN CONSOLE</div></div>
    </div>
    <nav class="side-nav">
${navHtml(screen.nav)}    </nav>
    <div class="side-user">
      <div class="avatar">运</div>
      <div><div class="n">运营管理员</div><div class="s">退出登录</div></div>
    </div>
  </aside>
  <main id="stage" class="stage${rich ? ' rich' : ''}"></main>
</div>
<script src="../assets/admin.js"></script>
</body>
</html>
`;
}

function adminPage(screen) {
  if (RICH.has(screen.key)) return adminShell(screen, { rich: true });
  const tpl = transform(read(path.join(SRC, 'admin/templates', screen.key + '.html')));
  const shell = adminShell(screen, { rich: false });
  return shell.replace(
    '<script src="../assets/admin.js"></script>',
    `<template id="tpl">\n${tpl}\n</template>\n<script src="../assets/admin.js"></script>`,
  );
}

/* ---------- H5 页面 ---------- */
function h5Page(screen) {
  const tpl = transform(read(path.join(SRC, 'h5/templates', screen.key + '.html')));
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${screen.title} · AI-CRM 用户端</title>
<link rel="stylesheet" href="../assets/tokens.css">
</head>
<body data-page="${screen.key}">
<div class="h5-backdrop">
  <div>
    <div class="phone"><div id="screen" class="phone-screen"></div></div>
    <div style="text-align:center;margin-top:14px;font-size:12px;color:#8F959E"><a href="index.html">← 全部屏幕</a></div>
  </div>
</div>
<template id="tpl">
${tpl}
</template>
<script src="../assets/h5.js"></script>
</body>
</html>
`;
}

/* ---------- 侧边栏 ---------- */
function sidebarPage() {
  const tpl = read(path.join(SRC, 'sidebar/templates/index.html'));
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>企微侧边栏 · AI-CRM</title>
<link rel="stylesheet" href="../assets/tokens.css">
</head>
<body>
${tpl}
<script src="../assets/sidebar.js"></script>
</body>
</html>
`;
}

/* ---------- 索引页 ---------- */
function topIndex() {
  const adminLinks = registry.screens
    .map((s) => `<a href="admin/${s.key}.html">${s.label}${s.level === '二级' ? ' · 二级' : ''}</a>`)
    .join('\n');
  const h5Links = h5Registry.map((s) => `<a href="h5/${s.key}.html">${s.title}</a>`).join('\n');
  const navCount = registry.screens.filter((s) => s.isNav).length;
  const subCount = registry.screens.length - navCount;
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AI-CRM 前端 · TypeScript 版</title>
<link rel="stylesheet" href="assets/tokens.css">
</head>
<body>
<div class="ix-wrap">
  <h1 class="ix-title">AI-CRM 全新前端 · TypeScript 实现</h1>
  <p class="ix-sub">管理后台 ${registry.screens.length} 屏 / 企微侧边栏 / 用户端 H5 ${h5Registry.length} 屏 · mock 数据会话级写穿 · 接 API 即可上线</p>
  <div class="ix-grid">
    <a class="ix-card" href="admin/customers.html"><h2>管理后台 →</h2><p>${navCount} 个一级页 + ${subCount} 个二级页 · 雷达 / AI 助手 / 漏斗全真交互</p></a>
    <a class="ix-card" href="sidebar/index.html"><h2>企微侧边栏 →</h2><p>销售工作台 · 客户画像 / 快捷话术 / 跟进记录</p></a>
    <a class="ix-card" href="h5/index.html"><h2>用户端 H5 →</h2><p>问卷作答 · 测评报告 · 报名支付落地 12 屏</p></a>
  </div>
  <div class="ix-sec">管理后台 · 全部页面</div>
  <div class="ix-list">
${adminLinks}
  </div>
  <div class="ix-sec">用户端 H5 · 全部屏幕</div>
  <div class="ix-list">
${h5Links}
  </div>
</div>
</body>
</html>
`;
}

function h5Index() {
  const groups = { Q: '问卷作答流程', S: '报名 / 续费落地' };
  let html = '';
  for (const [g, label] of Object.entries(groups)) {
    html += `<div class="ix-sec">${label}</div><div class="ix-list">\n`;
    html += h5Registry
      .filter((s) => s.group === g)
      .map((s) => `<a href="${s.key}.html">${s.title}</a>`)
      .join('\n');
    html += '\n</div>\n';
  }
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>用户端 H5 · AI-CRM</title>
<link rel="stylesheet" href="../assets/tokens.css">
</head>
<body>
<div class="ix-wrap">
  <h1 class="ix-title">用户端 H5</h1>
  <p class="ix-sub">12 屏 · <a href="../index.html">← 返回总索引</a></p>
  ${html}
</div>
</body>
</html>
`;
}

const adminIndex = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta http-equiv="refresh" content="0; url=customers.html">
<title>AI-CRM 管理后台</title></head>
<body style="font-family:sans-serif;padding:40px">正在进入管理后台… <a href="customers.html">手动进入</a></body></html>
`;

/* ---------- 执行 ---------- */
async function main() {
  fs.rmSync(DIST, { recursive: true, force: true });

  // 后台 34 屏
  for (const s of registry.screens) {
    write(path.join(DIST, 'admin', s.key + '.html'), adminPage(s));
  }
  write(path.join(DIST, 'admin/index.html'), adminIndex);

  // H5 12 屏 + 索引
  for (const s of h5Registry) {
    write(path.join(DIST, 'h5', s.key + '.html'), h5Page(s));
  }
  write(path.join(DIST, 'h5/index.html'), h5Index());

  // 侧边栏
  write(path.join(DIST, 'sidebar/index.html'), sidebarPage());

  // 总索引
  write(path.join(DIST, 'index.html'), topIndex());

  // 样式
  write(path.join(DIST, 'assets/tokens.css'), read(path.join(SRC, 'shared/ui/tokens.css')));
  write(path.join(DIST, 'assets/labs.css'), read(path.join(SRC, 'admin/sections/labs.css')));

  // TS 打包
  await build({
    entryPoints: {
      admin: path.join(SRC, 'admin/main.ts'),
      h5: path.join(SRC, 'h5/main.ts'),
      sidebar: path.join(SRC, 'sidebar/main.ts'),
    },
    bundle: true,
    format: 'iife',
    target: 'es2020',
    outdir: path.join(DIST, 'assets'),
    minify: false,
    logLevel: 'warning',
  });

  const count = registry.screens.length + h5Registry.length + 3;
  console.log(`✓ build done: ${count} pages + 3 bundles → dist/`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
