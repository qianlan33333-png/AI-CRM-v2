/**
 * 端到端 DOM 渲染验证（jsdom）：
 * 加载 dist/ 生成页，执行真实 bundle，断言渲染结果与关键交互。
 */
import { JSDOM } from 'jsdom';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const DIST = path.join(ROOT, 'dist');

let pass = 0;
let fail = 0;
const ok = (name, cond) => {
  if (cond) {
    pass++;
    console.log('  ✓ ' + name);
  } else {
    fail++;
    console.log('  ✗ ' + name);
  }
};

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function loadPage(rel, { id, q } = {}) {
  const file = path.join(DIST, rel);
  let html = fs.readFileSync(file, 'utf8');
  // 用 jsdom 执行内联脚本：把 bundle 内联进去，避免资源加载配置
  html = html.replace(/<script src="[^"]*assets\/(admin|h5|sidebar)\.js"><\/script>/, (_m, name) => {
    const code = fs.readFileSync(path.join(DIST, 'assets', name + '.js'), 'utf8');
    return `<script>${code}</script>`;
  });
  const qs = q || (id != null ? 'id=' + id : '');
  const dom = new JSDOM(html, {
    url: 'http://localhost/' + rel + (qs ? '?' + qs : ''),
    runScripts: 'dangerously',
    pretendToBeVisual: true,
    beforeParse(window) {
      // Mock 仅由 DOM 回归测试显式注入；浏览器默认运行态不会走此路径。
      window.__AICRM_TEST_MOCK__ = true;
    },
  });
  // 等 loadDb（120ms）+ 二级加载（200ms）+ 余量
  await sleep(700);
  return dom;
}

const click = (dom, el) => el.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
const input = (dom, el, v) => {
  el.value = v;
  el.dispatchEvent(new dom.window.Event('input', { bubbles: true }));
};

/* ================= 后台 · 内容雷达 ================= */
console.log('admin/radar.html（列表）');
{
  const dom = await loadPage('admin/radar.html');
  const d = dom.window.document;
  ok('雷达列表渲染 5 行', d.querySelectorAll('#listRows tr').length === 5);
  ok('导航高亮落在内容雷达', !!d.querySelector('.nav-item.on[href="radar.html"]'));
  // 关键词筛选
  input(dom, d.querySelector('#fKeyword'), '沙龙');
  await sleep(30);
  ok('搜索「沙龙」后剩 1 行', d.querySelectorAll('#listRows tr').length === 1);
  input(dom, d.querySelector('#fKeyword'), '');
  await sleep(30);
  // 分享浮窗
  const shareBtn = d.querySelector('[data-share]');
  click(dom, shareBtn);
  await sleep(30);
  ok('分享浮窗打开且伪二维码渲染', d.querySelector('#shareMask').classList.contains('open') && !!d.querySelector('#shareQr svg'));
  click(dom, d.querySelector('#shareMask .modal-x'));
  await sleep(30);
  ok('关闭分享浮窗', !d.querySelector('#shareMask').classList.contains('open'));
  // 停用 → 写穿并 toast
  const toggleBtn = [...d.querySelectorAll('[data-toggle]')].find((b) => b.textContent.trim() === '停用');
  click(dom, toggleBtn);
  await sleep(500);
  ok('停用后 toast 反馈且按钮变「启用」', d.body.textContent.includes('已停用'));
  dom.window.close();
}

console.log('admin/radarDetail.html?id=2（详情）');
{
  const dom = await loadPage('admin/radarDetail.html', { id: 2 });
  const d = dom.window.document;
  ok('详情页读取 ?id=2 显示对应标题', d.body.textContent.includes('共学营开营通知'));
  ok('4 张统计卡（含授权转化率 79%）', d.querySelectorAll('.stat-row .stat').length === 4 && d.body.textContent.includes('79%'));
  ok('访问明细渲染 24 行', d.querySelectorAll('#dRows tr').length === 24);
  input(dom, d.querySelector('#dKeyword'), '2f9Qn');
  await sleep(30);
  ok('明细按外部联系人 ID 过滤', d.querySelectorAll('#dRows tr').length === 3);
  dom.window.close();
}

console.log('admin/radarForm.html（新建校验）');
{
  const dom = await loadPage('admin/radarForm.html');
  const d = dom.window.document;
  const save = [...d.querySelectorAll('button')].find((b) => b.textContent.includes('保存内容雷达'));
  click(dom, save);
  await sleep(30);
  ok('名称为空时阻止保存并提示', d.querySelector('#fb-toast').textContent === '请输入内容名称');
  // 切换到图片类型 → 素材区出现
  click(dom, d.querySelector('.type-card[data-t="image"]'));
  await sleep(30);
  ok('切换图片类型后素材与独立目标地址同时显示', !d.querySelector('#cfgMedia').hidden && !d.querySelector('#cfgUrl').hidden);
  // 选素材（通用选择器）→ 填名称 → 保存成功跳列表前 toast
  click(dom, d.querySelector('#btnPick'));
  await sleep(450);
  ok('图片素材选择器打开', !!d.querySelector('.pk-mask'));
  click(dom, d.querySelector('.pk-mask [data-pk-id]'));
  await sleep(30);
  click(dom, d.querySelector('.pk-mask [data-pk="ok"]'));
  await sleep(30);
  ok('选择素材后写回表单', d.body.textContent.includes('来自素材库'));
  input(dom, d.querySelector('#fName'), 'e2e 测试图片雷达');
  input(dom, d.querySelector('#fUrl'), 'https://example.test/poster');
  click(dom, save);
  await sleep(600);
  ok('保存成功 toast', d.querySelector('#fb-toast').textContent === '已保存内容雷达');
  dom.window.close();
}

/* ================= 后台 · AI 助手 ================= */
console.log('admin/ai.html（计划列表）');
{
  const dom = await loadPage('admin/ai.html');
  const d = dom.window.document;
  ok('计划列表渲染 8 行', d.querySelectorAll('#planList .plan-row').length === 8);
  ok('统计卡：待审批 2 / 执行中 1', d.querySelector('#stPending').textContent === '2' && d.querySelector('#stActive').textContent === '1');
  input(dom, d.querySelector('#fKeyword'), '共学营');
  await sleep(30);
  ok('搜索「共学营」后剩 1 条', d.querySelectorAll('#planList .plan-row').length === 1);
  dom.window.close();
}

console.log('admin/aiDetail.html?id=7（详情 + 抽屉）');
{
  const dom = await loadPage('admin/aiDetail.html', { id: 7 });
  const d = dom.window.document;
  ok('计划信息含目标人数 1,632', d.body.textContent.includes('1,632'));
  ok('人员分页加载（50 / 180）', d.querySelector('#rcLoaded').textContent.includes('50 / 180'));
  ok('人员表渲染 50 行', d.querySelectorAll('#rcRows tr').length === 50);
  // 继续加载
  click(dom, d.querySelector('#rcMore'));
  await sleep(30);
  ok('继续加载后为 100 行', d.querySelectorAll('#rcRows tr').length === 100);
  // 打开人员抽屉
  const openBtn = [...d.querySelectorAll('#rcRows [data-rc]')].find((b) => b.tagName === 'BUTTON');
  click(dom, openBtn);
  await sleep(30);
  ok('人员抽屉打开（话术任务可见）', d.querySelector('#drawer').classList.contains('open') && d.body.textContent.includes('话术任务 1'));
  // 批准这个人
  click(dom, d.querySelector('#dwApprove'));
  await sleep(400);
  ok('批准人员后 toast + 状态级联', d.querySelector('#fb-toast').textContent === '已批准这个人发送');
  click(dom, d.querySelector('#dwClose'));
  await sleep(30);
  // 整单批准（确认浮窗 → 级联）
  click(dom, d.querySelector('#dApprove'));
  await sleep(30);
  ok('整单批准弹出确认浮窗', d.querySelector('#fb-mask').style.display === 'flex');
  click(dom, d.querySelector('#fb-ok'));
  await sleep(400);
  ok('批准后状态变「已批准」且按钮锁定', d.querySelector('#dStatus').textContent === '已批准' && d.querySelector('#dApprove').disabled);
  dom.window.close();
}

/* ================= 后台 · 漏斗多维表格 ================= */
console.log('admin/funnel.html（多维表格）');
{
  const dom = await loadPage('admin/funnel.html');
  const d = dom.window.document;
  ok('默认视图渲染 120 行', d.querySelectorAll('#gridBody tr').length === 120);
  ok('3 个预置视图页签', d.querySelectorAll('#viewTabs .vtab').length === 3);
  // 切换「拉回重点」视图（筛选 + 分组）
  click(dom, d.querySelectorAll('#viewTabs .vtab')[1]);
  await sleep(30);
  ok('切换视图后出现分组头', d.querySelectorAll('#gridBody .ghead').length > 0);
  ok('结果摘要显示分组信息', d.querySelector('#resultSummary').textContent.includes('已按「企微跟进人」分组'));
  // 折叠第一组
  click(dom, d.querySelector('#gridBody .ghead'));
  await sleep(30);
  ok('组头可折叠', d.querySelector('#gridBody .ghead').classList.contains('collapsed'));
  // 勾选一行
  const ck = d.querySelector('[data-ck]');
  ck.checked = true;
  ck.dispatchEvent(new dom.window.Event('change', { bubbles: true }));
  await sleep(30);
  ok('勾选行后底部显示已选 1 行', d.querySelector('#selInfo').textContent.includes('已选 1 行'));
  // 表头排序切换
  const th = [...d.querySelectorAll('#gridHead th')].find((t) => t.textContent.includes('沉默天数'));
  click(dom, th);
  await sleep(30);
  ok('点表头出现排序标记（视图进入未保存态）', !!d.querySelector('#gridHead .sort-mark') && !d.querySelector('#btnSaveView').hidden);
  // 分享浮窗
  click(dom, d.querySelector('#btnShare'));
  await sleep(30);
  ok('分享浮窗打开', d.querySelector('#shareMask').classList.contains('open'));
  click(dom, d.querySelector('#swShare'));
  await sleep(30);
  ok('开启外部分享生成链接', (d.querySelector('#shareUrl').value || '').includes('/share/funnel/v_'));
  // 邀请协作者 → 选客服组件
  click(dom, d.querySelector('#btnInvite'));
  await sleep(450);
  ok('邀请协作者弹出选客服组件', !!d.querySelector('.pk-mask'));
  click(dom, d.querySelector('.pk-mask [data-pk-id]'));
  await sleep(30);
  click(dom, d.querySelector('.pk-mask [data-pk="ok"]'));
  await sleep(30);
  ok('协作者名单新增 1 人', d.querySelectorAll('#collabList .collab').length === 3);
  click(dom, d.querySelector('#shareMask .modal-x'));
  await sleep(30);
  // 群发浮窗 → 素材选择器
  click(dom, d.querySelector('#btnBroadcast'));
  await sleep(30);
  click(dom, d.querySelector('[data-mat="image"]'));
  await sleep(450);
  ok('群发素材选择器打开', !!d.querySelector('.pk-mask'));
  click(dom, d.querySelector('.pk-mask [data-pk-id]'));
  await sleep(30);
  click(dom, d.querySelector('.pk-mask [data-pk="ok"]'));
  await sleep(30);
  ok('群发内容出现素材 chip', d.querySelector('#bcMats').textContent.length > 0);
  dom.window.close();
}

/* ================= 后台 · 模板页回归 ================= */
console.log('admin/questionnaires.html');
{
  const dom = await loadPage('admin/questionnaires.html');
  const d = dom.window.document;
  ok('问卷列表 6 行', d.querySelectorAll('tbody tr').length === 6);
  dom.window.close();
}

/* ================= 本轮新增：二级页 + 通用选择器 ================= */
console.log('admin/automation.html（新增分组弹窗 → 测试 Mock 创建）');
{
  const dom = await loadPage('admin/automation.html');
  const d = dom.window.document;
  const addBtn = [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '新增');
  click(dom, addBtn);
  await sleep(30);
  ok('新增分组弹窗打开', !!d.querySelector('#fGroupName'));
  input(dom, d.querySelector('#fGroupName'), 'e2e 测试分组');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '确认'));
  await sleep(500);
  ok('创建后分组出现在分组列表', d.body.textContent.includes('e2e 测试分组'));
  dom.window.close();
}

console.log('admin/audienceEdit.html?id=1（真实配置与发送人 DTO）');
{
  const dom = await loadPage('admin/audienceEdit.html', { id: 1 });
  const d = dom.window.document;
  const nav4 = [...d.querySelectorAll('button')].find((b) => b.textContent.includes('成员列表'));
  click(dom, nav4);
  await sleep(30);
  const h3 = [...d.querySelectorAll('h3')].find((h) => h.textContent === '成员列表');
  let panel = h3 && h3.parentElement;
  while (panel && panel.style.display !== 'block' && panel.style.display !== 'none') panel = panel.parentElement;
  ok('切到成员列表面板', !!panel && panel.style.display === 'block');
  const nav3 = [...d.querySelectorAll('button')].find((b) => b.textContent.includes('发送人白名单'));
  click(dom, nav3);
  await sleep(30);
  ok('发送人使用 sender_userid 明文 DTO', !!d.querySelector('#aeSenders') && d.body.textContent.includes('最多 5 位'));
  d.querySelector('#aeSenders').value = 'a\nb\nc\nd\ne\nf';
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '保存发送人白名单'));
  await sleep(30);
  ok('超过 5 位在发请求前被阻止', d.body.textContent.includes('发送人最多 5 位且不能重复'));
  dom.window.close();
}

console.log('admin/cyclesDetail.html?id=1（8 章运行档案）');
{
  const dom = await loadPage('admin/cyclesDetail.html', { id: 1 });
  const d = dom.window.document;
  ok(
    '八章档案渲染（含分窗口结果 / 结果复盘 / 证据索引）',
    d.body.textContent.includes('分窗口结果') && d.body.textContent.includes('结果复盘与限制') && d.body.textContent.includes('证据索引'),
  );
  dom.window.close();
}

console.log('admin/tags.html（新建标签测试 Mock 建行）');
{
  const dom = await loadPage('admin/tags.html');
  const d = dom.window.document;
  const before = d.querySelectorAll('tbody tr').length;
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '新建标签'));
  await sleep(30);
  ok('新建标签编辑组件打开', !!d.querySelector('#fTagName'));
  input(dom, d.querySelector('#fTagName'), 'e2e标签X');
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '创建'));
  await sleep(500);
  ok('标签 Mock 创建（行数 +1）', d.querySelectorAll('tbody tr').length === before + 1);
  dom.window.close();
}

console.log('admin/questionnaireOps.html?id=1（opaque 本地运营配置）');
{
  const dom = await loadPage('admin/questionnaireOps.html', { id: 1 });
  const d = dom.window.document;
  ok('只展示 opaque navigation target 与正整数渠道 ID', !!d.querySelector('#opsNavigationTarget') && !!d.querySelector('#opsChannelResourceId') && !d.querySelector('#opsRedirectUrl'));
  ok('外部推送只接受 configuration reference', !!d.querySelector('#opsConfigurationReference') && !d.querySelector('#opsWebhook'));
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.includes('测试推送')));
  await sleep(30);
  ok('测试外推明确阻断且未伪造成功', d.querySelector('#fb-toast').textContent.includes('未发送请求'));
  dom.window.close();
}

console.log('admin/couponData.html?id=0（5 统计卡 + 8 明细行 + 分享组件）');
{
  const dom = await loadPage('admin/couponData.html', { id: 0 });
  const d = dom.window.document;
  ok(
    '5 张统计卡文案齐全',
    ['累计领取', '当前可用', '支付预占', '已使用', '已过期'].every((t) => d.body.textContent.includes(t)),
  );
  ok('领取明细 8 行', d.querySelectorAll('tbody tr').length === 8);
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '分享'));
  await sleep(30);
  ok('分享弹窗打开且伪二维码渲染', !!d.querySelector('#shareQrBox svg') && d.body.textContent.includes('分享优惠券'));
  dom.window.close();
}

console.log('admin/configDetail.html?cat=wechat_pay（类目配置点渲染）');
{
  const dom = await loadPage('admin/configDetail.html', { q: 'cat=wechat_pay' });
  const d = dom.window.document;
  ok('微信支付类目字段渲染（含 WECHAT_PAY_MCH_ID）', d.body.textContent.includes('WECHAT_PAY_MCH_ID'));
  ok('支持检查的类目显示「检查」按钮', [...d.querySelectorAll('button')].some((b) => b.textContent.trim() === '检查'));
  dom.window.close();
}

console.log('admin/channelForm.html（完整 OpenAPI 渠道 DTO）');
{
  const dom = await loadPage('admin/channelForm.html');
  const d = dom.window.document;
  ok('载体、客服、素材与标签字段齐全', !!d.querySelector('#channelType') && !!d.querySelector('#channelOwner') && !!d.querySelector('#channelImageIds') && !!d.querySelector('#channelTagId'));
  ok('分配策略使用服务端 JSON DTO', !!d.querySelector('#channelAssignmentMode') && !!d.querySelector('#channelAssignmentStrategy') && !!d.querySelector('#channelAssignmentConfig'));
  d.querySelector('#channelName').value = '新客渠道';
  d.querySelector('#channelCode').value = 'new-customer';
  d.querySelector('#channelImageIds').value = 'not-an-id';
  click(dom, [...d.querySelectorAll('button')].find((b) => b.textContent.trim() === '保存渠道'));
  await sleep(30);
  ok('无效素材引用在发请求前被阻止', d.body.textContent.includes('素材引用必须是正整数 ID'));
  dom.window.close();
}

console.log('admin/ownerMig.html（本地安全 CSV 迁移边界）');
{
  const dom = await loadPage('admin/ownerMig.html');
  const d = dom.window.document;
  const csv = d.querySelector('#ownerMigCsv');
  ok('仅接受 CSV 且不再显示企微转接/欢迎语控件', csv?.getAttribute('accept')?.includes('.csv') && !d.body.textContent.includes('同时发起企微转接') && !d.body.textContent.includes('转接欢迎语'));
  ok('初始明确为空且真实动作均已绑定', d.body.textContent.includes('尚未生成迁移预览，不会发送执行请求') && [...d.querySelectorAll('button')].filter((b) => b.__dcBound).length >= 2);
  dom.window.close();
}

/* ================= H5 ================= */
console.log('h5/all.html');
{
  const dom = await loadPage('h5/all.html');
  const d = dom.window.document;
  const opts = [...d.querySelectorAll('label')].filter((l) => l.__dcBound);
  ok('单选+多选选项均已绑定点击', opts.length >= 9);
  const before = d.body.innerHTML;
  opts[1].dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
  await sleep(30);
  ok('点击单选切换选中态（重渲染）', d.body.innerHTML !== before);
  dom.window.close();
}

console.log('h5/one.html');
{
  const dom = await loadPage('h5/one.html');
  const d = dom.window.document;
  ok('显示第 3 / 12 题', d.body.textContent.includes('第 3 / 12 题'));
  const next = [...d.querySelectorAll('button')].find((b) => b.textContent === '下一题');
  next.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
  await sleep(30);
  ok('下一题 → 第 4 题', d.body.textContent.includes('第 4 / 12 题'));
  dom.window.close();
}

/* ================= 侧边栏 ================= */
console.log('sidebar/index.html');
{
  const dom = await loadPage('sidebar/index.html');
  const d = dom.window.document;
  ok('侧边栏渲染（含 WX 顶栏）', d.body.textContent.includes('WX'));
  dom.window.close();
}

console.log(`\n${pass} 通过 / ${fail} 失败`);
process.exit(fail ? 1 : 0);
