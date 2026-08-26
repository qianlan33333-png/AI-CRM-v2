import { ADMIN_SCREENS, CAPABILITIES } from './capabilities';

function assert(ok: unknown, message: string): asserts ok { if (!ok) throw new Error(message); }

export function runCapabilityTests(): void {
  const stale = CAPABILITIES.filter((cap) => cap.state === 'backend_blocked' && cap.reason === 'OpenAPI operation 已存在，Kimi 壳尚未完成 DTO Adapter');
  assert(stale.length === 0, 'existing OpenAPI operation must be real or have a precise semantic reason');
  const pending = CAPABILITIES.filter((cap) => cap.state === 'backend_blocked' && /待批次|adapter.pending|DTO Adapter/i.test(cap.reason || ''));
  assert(pending.length === 0, 'adapter-pending is not an allowed backend_blocked reason');
  assert(CAPABILITIES.every((cap) => cap.state !== 'excluded_duplicate_page'), 'excluded legacy rows belong only in docs/frontend-capability-scope.md');
  assert(CAPABILITIES.filter((cap) => cap.state === 'real').length > 0, 'real inventory must not be empty');
  const memberGridPublicShare = CAPABILITIES.filter((cap) => cap.surface === 'admin' && cap.screen === 'spProductData' && cap.state === 'backend_blocked');
  assert(memberGridPublicShare.length === 3 && memberGridPublicShare.some((cap) => cap.action === '周期商品公开 Member Grid 读取') && memberGridPublicShare.some((cap) => cap.action === '周期商品分享读取、二维码与链接预览') && memberGridPublicShare.some((cap) => cap.action === 'Member Grid 外部分享开关'), 'Member Grid public sharing gaps must stay explicitly backend_blocked');
  assert(ADMIN_SCREENS.length === 40, 'Admin screen denominator changed without capability review');
  for (const screen of ADMIN_SCREENS) assert(CAPABILITIES.some((cap) => cap.surface === 'admin' && cap.screen.split('/').includes(screen)), `Admin screen has no capability classification: ${screen}`);
}
