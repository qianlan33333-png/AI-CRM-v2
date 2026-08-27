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
  assert(memberGridPublicShare.length === 0, 'Member Grid page must not retain stale backend_blocked rows after real share integration');
  assert(CAPABILITIES.some((cap) => cap.screen === 'spProductData' && cap.action === '成员网格公开只读分享、撤销与一次性链接' && cap.state === 'real'), 'Member Grid public read-only share must be classified as real');
  assert(CAPABILITIES.some((cap) => cap.screen.split('/').includes('spProductData') && cap.action === '周期商品分享读取、二维码与链接预览' && cap.state === 'real'), 'service product share must remain classified as real');
  assert(ADMIN_SCREENS.length === 40, 'Admin screen denominator changed without capability review');
  for (const screen of ADMIN_SCREENS) assert(CAPABILITIES.some((cap) => cap.surface === 'admin' && cap.screen.split('/').includes(screen)), `Admin screen has no capability classification: ${screen}`);
}
