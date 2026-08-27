const summaryEndpoint = '/api/public/member-grid-shares/summary';
const tokenPattern = /^[A-Za-z0-9_-]{43}$/;
const states = ['active', 'expired', 'removed'] as const;

type MemberState = typeof states[number];
type Bucket = { state: MemberState; count: number };
type Summary = { buckets: Bucket[]; asOf: string };

const labels: Record<MemberState, string> = {
  active: '有效',
  expired: '已过期',
  removed: '已移除',
};

function readSummary(value: unknown): Summary | undefined {
  if (value == null || typeof value !== 'object') return undefined;
  const source = value as Record<string, unknown>;
  if (typeof source.as_of !== 'string' || source.as_of.trim() === '' || !Array.isArray(source.buckets)) return undefined;
  const byState = new Map<MemberState, number>();
  for (const item of source.buckets) {
    if (item == null || typeof item !== 'object') return undefined;
    const bucket = item as Record<string, unknown>;
    if (!states.includes(bucket.state as MemberState) || !Number.isSafeInteger(bucket.count) || (bucket.count as number) < 0 || byState.has(bucket.state as MemberState)) return undefined;
    byState.set(bucket.state as MemberState, bucket.count as number);
  }
  if (byState.size !== states.length) return undefined;
  return { asOf: source.as_of, buckets: states.map((state) => ({ state, count: byState.get(state) || 0 })) };
}

function renderFailure(stage: HTMLElement): void {
  const document = stage.ownerDocument;
  const panel = document.createElement('main');
  const title = document.createElement('h1');
  title.textContent = 'Member Grid 公开汇总';
  const message = document.createElement('p');
  message.textContent = '暂时无法读取分享汇总。';
  panel.append(title, message);
  stage.replaceChildren(panel);
}

function renderLoading(stage: HTMLElement): void {
  const document = stage.ownerDocument;
  const panel = document.createElement('main');
  const title = document.createElement('h1');
  title.textContent = 'Member Grid 公开汇总';
  const message = document.createElement('p');
  message.textContent = '正在读取汇总…';
  panel.append(title, message);
  stage.replaceChildren(panel);
}

function renderSummary(stage: HTMLElement, summary: Summary): void {
  const document = stage.ownerDocument;
  const panel = document.createElement('main');
  const title = document.createElement('h1');
  title.textContent = 'Member Grid 公开汇总';
  const list = document.createElement('dl');
  for (const bucket of summary.buckets) {
    const state = document.createElement('dt');
    state.textContent = labels[bucket.state];
    const count = document.createElement('dd');
    count.textContent = String(bucket.count);
    list.append(state, count);
  }
  const asOf = document.createElement('p');
  asOf.textContent = `汇总截至：${summary.asOf}`;
  panel.append(title, list, asOf);
  stage.replaceChildren(panel);
}

export async function mountMemberGridShare(stage: HTMLElement): Promise<void> {
  const token = window.location.hash.slice(1);
  window.history.replaceState(null, '', window.location.pathname + window.location.search);
  if (!tokenPattern.test(token)) {
    renderFailure(stage);
    return;
  }

  renderLoading(stage);
  try {
    const response = await fetch(summaryEndpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
      credentials: 'omit',
      cache: 'no-store',
    });
    const summary = response.ok ? readSummary(await response.json()) : undefined;
    if (!summary) throw new Error('summary unavailable');
    renderSummary(stage, summary);
  } catch {
    renderFailure(stage);
  }
}
