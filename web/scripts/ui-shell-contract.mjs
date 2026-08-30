import fs from 'node:fs';
import path from 'node:path';
import { JSDOM } from 'jsdom';

const root = path.resolve(import.meta.dirname, '..');
const read = (relative) => fs.readFileSync(path.join(root, relative), 'utf8');
const ok = (condition, message) => {
  if (!condition) throw new Error(message);
};

const nav = JSON.parse(read('src/admin/nav.json'));
ok(nav.some((item) => item.key === 'funnel' && item.label === '漏斗 / 数据看板'), '一级导航必须保留 Kimi 漏斗入口');
ok(!nav.some((item) => item.key === 'campaigns'), 'Cloud Campaign 不得占用 Kimi 一级导航');

const registry = JSON.parse(read('src/admin/registry.json'));
const campaign = registry.screens.find((item) => item.key === 'campaigns');
ok(campaign?.isNav === false, 'Cloud Campaign 只能作为隐藏路由保留');

const blockedPages = ['active', 'auth', 'done', 'error', 'expired', 'loading', 'pay', 'qr', 'signup'];
for (const page of blockedPages) {
  const dom = new JSDOM(`<body>${read(`src/h5/templates/${page}.html`)}</body>`);
  const banner = dom.window.document.querySelector('[data-h5-blocked]');
  ok(banner, `H5 ${page} 必须显示明确 blocked 状态`);
  ok(banner.parentElement === dom.window.document.body, `H5 ${page} blocked 提示不得塞入 44px 标题栏`);
  ok(dom.window.document.querySelector('button:not([disabled])') === null, `H5 ${page} 不得暴露可执行的伪业务按钮`);
}

const all = read('src/h5/templates/all.html');
const one = read('src/h5/templates/one.html');
ok(all.includes('list="{{ questions }}"') && all.includes('data-h5-receipt'), 'H5 整页问卷必须遍历真实题目并显示回执');
ok(one.includes('{{ qTitle }}') && one.includes('data-h5-progress') && one.includes('data-h5-receipt'), 'H5 逐题页必须消费真实题干、进度与回执');

const config = read('src/admin/templates/config.html');
ok(config.includes('open-setup-wizard') && config.includes('open-admin-access'), '接入和访问控制必须收进原配置类目表');
ok(!config.includes('setup-wizard-card') && !config.includes('admin-access-card'), '配置首屏不得再附加两张独立大卡');

const orders = read('src/admin/templates/orders.html');
ok(orders.includes('{{ orderPage.query }}') && orders.includes('{{ orderPage.clear }}'), '交易页必须保留 Kimi 查询/清空主流程');
ok(!orders.includes('共 486 条'), '交易页不得显示硬编码订单总数');

const automation = read('src/admin/sections/automationAgents.ts');
ok(automation.includes('客户管理后台 / 配置及后台') && automation.includes('grid-template-columns:226px'), '自动化 HTTP 运行态必须使用 Kimi 列表/编辑层级');

const commerce = read('src/admin/sections/commerce.ts');
ok((commerce.match(/alignKimiWorkspace\(stage\)/g) || []).length >= 3, '优惠券与 Member Grid HTTP 运行态必须套用 Kimi 工作区层级');

const groupOps = read('src/admin/templates/groupops.html');
const groupOpsDetail = read('src/admin/templates/groupopsDetail.html');
ok(groupOps.includes('计划列表') && groupOps.includes('运营成员选项'), '群运营列表必须保留 Kimi 统计卡和计划表层级');
ok(groupOpsDetail.includes('grid-template-columns:226px') && groupOpsDetail.includes('Webhook 与执行投影（高级只读）'), '群运营详情必须保留 Kimi 四步层级并下沉技术投影');

console.log('ui shell contract: ok');
