/* eslint-disable no-unused-vars -- callback signatures freeze the view contract. */
import React, { useCallback, useEffect, useRef, useState } from "react";
import { loadCustomerActivityAnalytics, type CustomerActivityAnalytics, type CustomerActivityAnalyticsLoadResult, type CustomerActivityAnalyticsTransport, type CustomerActivityWindowDays } from "./customer-activity-analytics";

type Failure = Exclude<CustomerActivityAnalyticsLoadResult, { readonly status: "loaded" }>["status"];
type State = { readonly kind: "loading"; readonly previous?: CustomerActivityAnalytics } | { readonly kind: "ready"; readonly analytics: CustomerActivityAnalytics } | { readonly kind: "error"; readonly failure: Failure; readonly previous?: CustomerActivityAnalytics };
interface Flight { readonly token: symbol; readonly days: CustomerActivityWindowDays }
export interface CustomerActivityAnalyticsPanelProps { readonly customerID: number; readonly transport?: CustomerActivityAnalyticsTransport; readonly onUnauthenticated?: () => void }

const messages: Record<Failure, string> = { invalid: "活动统计响应不符合安全合同。", unauthenticated: "登录状态已失效。", forbidden: "无权读取此客户活动统计。", not_found: "未找到此客户。", unavailable: "活动统计暂不可用，已保留上次结果。" };

export function CustomerActivityAnalyticsView({ state, selectedDays, onSelect }: { readonly state: State; readonly selectedDays: CustomerActivityWindowDays; readonly onSelect: (days: CustomerActivityWindowDays) => void }): React.ReactElement {
  const analytics = state.kind === "ready" ? state.analytics : state.previous;
  return <section className="route-card" aria-label="客户本地活动统计">
    <h2>客户本地活动统计</h2><p>仅汇总 CRM 本地事件类型与计数，不包含事件正文、操作人、身份值或外部调用状态。</p>
    <label>统计窗口<select aria-label="活动统计窗口" value={selectedDays} disabled={state.kind === "loading"} onChange={(event) => onSelect(Number(event.currentTarget.value) as CustomerActivityWindowDays)}><option value={7}>近 7 日</option><option value={30}>近 30 日</option><option value={90}>近 90 日</option></select></label>
    {state.kind === "loading" ? <p>正在读取本地统计…</p> : null}{state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
    {analytics ? <><dl><dt>事件数</dt><dd>{analytics.totalEvents}</dd><dt>活跃天数</dt><dd>{analytics.activeDays}</dd><dt>事件类型数</dt><dd>{analytics.uniqueEventTypes}</dd><dt>最后活动</dt><dd>{analytics.lastOccurredAt ?? "暂无"}</dd></dl>
      <h3>类型分布</h3>{analytics.typeFacets.length === 0 ? <p>窗口内暂无活动。</p> : <table><thead><tr><th>事件类型</th><th>数量</th><th>最近发生</th></tr></thead><tbody>{analytics.typeFacets.map((item) => <tr key={item.eventType}><td>{item.eventType}</td><td>{item.count}</td><td>{item.lastOccurredAt}</td></tr>)}</tbody></table>}{analytics.typeFacetsTruncated ? <p>仅显示数量最高的 50 种本地事件类型。</p> : null}
      <h3>按日计数</h3>{analytics.dailyCounts.length === 0 ? <p>窗口内无按日记录。</p> : <ul>{analytics.dailyCounts.map((item) => <li key={item.day}>{item.day}：{item.count}</li>)}</ul>}</> : null}
  </section>;
}

export function CustomerActivityAnalyticsPanel({ customerID, transport, onUnauthenticated }: CustomerActivityAnalyticsPanelProps): React.ReactElement | null {
  const [days, setDays] = useState<CustomerActivityWindowDays>(30); const [state, setState] = useState<State>({ kind: "loading" });
  const verified = useRef<CustomerActivityAnalytics>(); const flight = useRef<Flight>(); const generation = useRef(0); const lifetime = useRef(Symbol("customer-activity-analytics")); const unauthenticatedGeneration = useRef<number>();
  const load = useCallback(async (nextDays: CustomerActivityWindowDays) => {
    if (!transport || flight.current?.days === nextDays) return;
    const active: Flight = { token: Symbol("customer-activity-load"), days: nextDays }; flight.current = active; const current = ++generation.current; const activeLifetime = lifetime.current;
    setState({ kind: "loading", ...(verified.current ? { previous: verified.current } : {}) });
    try { const result = await loadCustomerActivityAnalytics(transport, customerID, nextDays); if (lifetime.current !== activeLifetime || generation.current !== current || flight.current?.token !== active.token) return;
      if (result.status === "loaded") { verified.current = result.analytics; setState({ kind: "ready", analytics: result.analytics }); return; }
      if (result.status === "unauthenticated" && unauthenticatedGeneration.current !== current) { unauthenticatedGeneration.current = current; onUnauthenticated?.(); }
      setState({ kind: "error", failure: result.status, ...(verified.current ? { previous: verified.current } : {}) });
    } finally { if (flight.current?.token === active.token) flight.current = undefined; }
  }, [customerID, onUnauthenticated, transport]);
  useEffect(() => { const activeLifetime = Symbol("customer-activity-analytics"); lifetime.current = activeLifetime; flight.current = undefined; verified.current = undefined; setDays(30); void load(30); return () => { if (lifetime.current === activeLifetime) { lifetime.current = Symbol("customer-activity-unmounted"); generation.current += 1; flight.current = undefined; } }; }, [customerID, load, transport]);
  if (!transport) return null;
  return <CustomerActivityAnalyticsView state={state} selectedDays={days} onSelect={(next) => { if (next === days || flight.current) return; setDays(next); void load(next); }} />;
}
