/**
 * 全局交互反馈层（TypeScript 版）
 * 合并两处来源：
 *  - 线上 admin_feedback.js：toast / 确认浮窗 / 提交 busy / 文件选择与上传提示的视觉
 *  - 未被控制器接管的业务按钮不会补模拟成功；它们必须由对应 Adapter 绑定。
 *
 * 原则：只补反馈，不改变既有业务流程；__dcBound（运行时已绑定）按钮自动跳过。
 */

const BOUND_MARK = '__dcBound';

interface FbElement extends HTMLElement {
  __dcBound?: boolean;
  __fbBusy?: boolean;
}

let installed = false;
let fbTimer: ReturnType<typeof setTimeout> | null = null;

/* ---------- 样式与 DOM ---------- */
function ensureUI(): void {
  if (document.getElementById('fb-toast')) return;
  const css = document.createElement('style');
  css.textContent = `
    #fb-toast{position:fixed;right:22px;bottom:22px;z-index:9999;background:#1F2329;color:#fff;padding:10px 16px;border-radius:8px;font-size:13px;box-shadow:0 12px 28px rgba(15,23,42,.25);display:none;font-family:inherit;animation:fb-in .18s ease-out;max-width:min(420px,80vw);line-height:1.5}
    #fb-toast.err{background:#D83931}
    @keyframes fb-in{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
    #fb-mask{position:fixed;inset:0;background:rgba(15,23,42,.34);z-index:9990;display:none;align-items:center;justify-content:center;padding:24px}
    #fb-panel{width:min(420px,100%);background:#fff;border-radius:12px;box-shadow:0 24px 64px rgba(15,23,42,.22);overflow:hidden;font-family:inherit}
    #fb-head{padding:18px 18px 0;font-size:15px;font-weight:600;color:#1F2329}
    #fb-body{padding:10px 18px 18px;font-size:13px;color:#646A73;line-height:1.7;white-space:pre-wrap}
    #fb-foot{display:flex;justify-content:flex-end;gap:10px;padding:0 18px 18px}
    .fb-btn{height:32px;padding:0 14px;border-radius:6px;border:1px solid #DEE0E3;background:#fff;color:#1F2329;font-size:13px;cursor:pointer}
    .fb-btn.primary{background:#3370ff;border-color:#3370ff;color:#fff}
    .fb-btn.danger{border-color:#F2B8B5;color:#D83931}
    #fb-prog-mask{position:fixed;inset:0;background:rgba(15,23,42,.34);z-index:9995;display:none;align-items:center;justify-content:center}
    #fb-prog{width:min(360px,90%);background:#fff;border-radius:12px;padding:22px;font-family:inherit}
    #fb-prog-track{height:6px;border-radius:99px;background:#EFF0F1;overflow:hidden;margin-top:14px}
    #fb-prog-bar{height:100%;width:0;background:#3370ff;border-radius:99px;transition:width .18s linear}
    .fb-busy{opacity:.65;pointer-events:none}
  `;
  document.head.appendChild(css);
  const mk = (html: string): Element => {
    const d = document.createElement('div');
    d.innerHTML = html;
    return d.firstElementChild as Element;
  };
  document.body.appendChild(mk('<div id="fb-toast"></div>'));
  document.body.appendChild(
    mk(`<div id="fb-mask"><div id="fb-panel">
      <div id="fb-head"></div><div id="fb-body"></div>
      <div id="fb-foot"><button class="fb-btn" id="fb-cancel">取消</button><button class="fb-btn primary" id="fb-ok">确认</button></div>
    </div></div>`),
  );
  document.body.appendChild(
    mk(`<div id="fb-prog-mask"><div id="fb-prog">
      <div style="font-size:14px;font-weight:600;color:#1F2329" id="fb-prog-title">正在上传</div>
      <div id="fb-prog-track"><div id="fb-prog-bar"></div></div>
      <div style="font-size:12px;color:#8F959E;margin-top:8px" id="fb-prog-pct">0%</div>
    </div></div>`),
  );
  document.getElementById('fb-mask')!.addEventListener('click', (e) => {
    if ((e.target as HTMLElement).id === 'fb-mask') hideConfirm();
  });
  document.getElementById('fb-cancel')!.addEventListener('click', () => hideConfirm());
}

/* ---------- Toast ---------- */
export function toast(msg: string, err = false): void {
  ensureUI();
  const t = document.getElementById('fb-toast')!;
  t.textContent = msg;
  t.className = err ? 'err' : '';
  t.style.display = 'block';
  if (fbTimer) clearTimeout(fbTimer);
  fbTimer = setTimeout(() => {
    t.style.display = 'none';
  }, 2400);
}

/* ---------- 确认浮窗（Promise 版） ---------- */
let onOkCb: (() => void) | null = null;

function hideConfirm(): void {
  document.getElementById('fb-mask')!.style.display = 'none';
  onOkCb = null;
}

export function confirmBox(
  title: string,
  body: string,
  okLabel = '确认',
  danger = false,
  onOk?: () => void,
): void {
  ensureUI();
  document.getElementById('fb-head')!.textContent = title;
  document.getElementById('fb-body')!.textContent = body;
  const ok = document.getElementById('fb-ok') as HTMLButtonElement;
  ok.textContent = okLabel;
  ok.className = 'fb-btn ' + (danger ? 'danger' : 'primary');
  onOkCb = onOk || null;
  ok.onclick = () => {
    const cb = onOkCb;
    hideConfirm();
    if (cb) cb();
  };
  document.getElementById('fb-mask')!.style.display = 'flex';
}

/* ---------- 按钮 busy ---------- */
export function busy(btn: FbElement, ms: number, done?: () => void): void {
  if (btn.__fbBusy) return;
  btn.__fbBusy = true;
  const old = btn.textContent || '';
  btn.classList.add('fb-busy');
  btn.textContent = '⏳ ' + old;
  setTimeout(() => {
    btn.classList.remove('fb-busy');
    btn.textContent = old;
    btn.__fbBusy = false;
    if (done) done();
  }, ms);
}

/* ---------- 模拟上传进度浮窗（预览环境无真实后端） ---------- */
export function simulateUpload(label?: string, onDone?: () => void): void {
  ensureUI();
  const mask = document.getElementById('fb-prog-mask')!;
  const bar = document.getElementById('fb-prog-bar')!;
  const pct = document.getElementById('fb-prog-pct')!;
  document.getElementById('fb-prog-title')!.textContent = '正在上传 · ' + (label || '文件');
  mask.style.display = 'flex';
  let p = 0;
  const tick = setInterval(() => {
    p = Math.min(100, p + 9 + Math.random() * 12);
    bar.style.width = p + '%';
    pct.textContent = Math.floor(p) + '%';
    if (p >= 100) {
      clearInterval(tick);
      setTimeout(() => {
        mask.style.display = 'none';
        bar.style.width = '0';
        toast('上传完成');
        if (onDone) onDone();
      }, 320);
    }
  }, 150);
}

/* ---------- 委托：未接管按钮的文案模式匹配 ---------- */
function delegate(e: Event): void {
  const target = e.target as HTMLElement;
  if (target.closest('#fb-mask') || target.closest('#fb-prog-mask')) return;
  const btn = target.closest('button') as FbElement | null;
  if (!btn || btn[BOUND_MARK] || (btn as HTMLButtonElement).disabled) return;
  const t = (btn.textContent || '').trim();
  if (!t || t.length > 14) return;
  if (/删除|下架|停用|禁用|拒绝|驳回|上传|选择文件|更换图片|更换文件|导出|下载|保存|提交|发布|上线|发送|群发|推送|创建|新建|刷新|重试|重新加载|启用|生成/.test(t)) {
    toast('后端能力未就绪：该操作不可执行', true);
  }
}

/* ---------- 文件选择提示 ---------- */
function onFileChange(e: Event): void {
  const input = e.target as HTMLInputElement;
  if (!(input instanceof HTMLInputElement) || input.type !== 'file') return;
  if (!input.files || !input.files.length) return;
  let total = 0;
  for (let i = 0; i < input.files.length; i++) total += input.files[i].size || 0;
  const mb = total / 1048576;
  const sizeText = mb >= 1 ? mb.toFixed(1) + ' MB' : Math.max(1, Math.round(total / 1024)) + ' KB';
  const name = input.files.length === 1 ? input.files[0].name : input.files.length + ' 个文件';
  toast('已选择：' + name + '（' + sizeText + '）');
}

/** 安装全局反馈层（每个页面入口调用一次） */
export function initFeedback(): void {
  if (installed) return;
  installed = true;
  ensureUI();
  document.addEventListener('click', delegate, true);
  document.addEventListener('change', onFileChange, true);
}
