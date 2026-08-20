/* eslint-disable no-unused-vars -- callback type declarations intentionally retain named arguments. */
import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  cancelPendingOutboundTask,
  confirmsCancelledOutboundTask,
  generatedOutboundOperationsTransport,
  loadOutboundReconciliation,
  loadOutboundTasks,
  newOutboundCancelIdempotencyKey,
  OUTBOUND_OPERATIONS_PAGE_SIZE,
  type OutboundCancelResult,
  type OutboundOperationsFailure,
  type OutboundOperationsRole,
  type OutboundOperationsTransport,
  type OutboundReconciliation,
  type OutboundTask,
  type OutboundTaskPage,
} from "./outbound-operations";

const statusValues = ["", "pending", "sending", "sent", "retryable_failed", "final_failed", "outcome_unknown", "cancelled"] as const;
const messages: Record<OutboundOperationsFailure, string> = { unauthenticated: "登录状态已失效，请重新登录。", forbidden: "当前账号没有本地投递运营查看权限。", invalid: "本地投递运营响应不符合已冻结的只读合同。", unavailable: "本地投递运营暂时不可用，请稍后刷新确认。" };
type PageState = { readonly kind: "loading"; readonly previous?: OutboundTaskPage } | { readonly kind: "ready"; readonly page: OutboundTaskPage } | { readonly kind: "error"; readonly failure: OutboundOperationsFailure; readonly previous?: OutboundTaskPage };
type DetailState = { readonly kind: "idle" } | { readonly kind: "loading"; readonly previous?: OutboundReconciliation } | { readonly kind: "ready"; readonly value: OutboundReconciliation } | { readonly kind: "error"; readonly failure: OutboundOperationsFailure; readonly previous?: OutboundReconciliation };
type Flight = { readonly key: string; readonly token: symbol };
type CancelNotice = Exclude<OutboundCancelResult["status"], "cancelled"> | "csrf_missing" | "confirmed";

const cancelMessages: Record<CancelNotice, string> = {
  unauthenticated: "登录状态已失效，请重新登录。取消结果尚未确认，系统不会自动重试。",
  forbidden: "当前账号没有取消本地投递任务的权限。",
  invalid: "取消请求或本地收据不符合已冻结合同，未再次提交。",
  conflict: "任务已被本地 worker 认领或状态已变化，未触发外部投递控制。请刷新确认。",
  unknown: "取消结果未完成本地对账确认，系统不会自动重试；请人工刷新确认。",
  unavailable: "取消结果暂不可确认，系统不会自动重试；请人工刷新确认。",
  csrf_missing: "安全令牌缺失，未发送取消请求。",
  confirmed: "已确认取消尚未运行的本地任务；此结果不代表外部投递、送达或外部回执。",
};

function browserCookie(): string { return typeof document === "undefined" ? "" : document.cookie; }

function Reconciliation({ value }: { readonly value: OutboundReconciliation }): React.ReactElement {
  return <aside aria-label="本地尝试与控制收据"><h2>本地尝试与控制收据</h2><p>仅为内部处理和本地收据对账；不代表外部投递、送达或外部回执。</p><h3>内部尝试</h3>{value.attempts.length === 0 ? <p>暂无本地尝试记录。</p> : <table><thead><tr><th>尝试 ID</th><th>代际</th><th>状态</th><th>次数</th><th>开始</th><th>完成</th></tr></thead><tbody>{value.attempts.map((item) => <tr key={item.attemptID}><td>{item.attemptID}</td><td>{item.generation}</td><td>{item.state}</td><td>{item.attempt}/{item.maxAttempts}</td><td>{item.dispatchStartedAt ?? "—"}</td><td>{item.completedAt ?? "—"}</td></tr>)}</tbody></table>}<h3>本地控制收据</h3>{value.receipts.length === 0 ? <p>暂无本地控制收据。</p> : <table><thead><tr><th>收据 ID</th><th>操作</th><th>内部状态</th><th>代际</th><th>完成时间</th></tr></thead><tbody>{value.receipts.map((item) => <tr key={item.receiptID}><td>{item.receiptID}</td><td>{item.operation}</td><td>{item.taskStatus}</td><td>{item.generation}</td><td>{item.completedAt}</td></tr>)}</tbody></table>}</aside>;
}
export function OutboundOperationsView({ state, detail, onLoad, onSelect, onCancel, canCancel, cancelLocked, confirmingTaskID, onConfirmTask, cancelNotice, status, businessID, onStatus, onBusinessID }: { readonly state: PageState; readonly detail: DetailState; readonly onLoad: (offset: number) => void; readonly onSelect: (taskID: string) => void; readonly onCancel?: (task: OutboundTask) => void; readonly canCancel: boolean; readonly cancelLocked: boolean; readonly confirmingTaskID?: string; readonly onConfirmTask: (taskID: string | undefined) => void; readonly cancelNotice?: CancelNotice; readonly status: string; readonly businessID: string; readonly onStatus: (value: string) => void; readonly onBusinessID: (value: string) => void; }): React.ReactElement {
  const page = state.kind === "ready" ? state.page : state.previous;
  const reconciliation = detail.kind === "ready" ? detail.value : detail.kind === "loading" || detail.kind === "error" ? detail.previous : undefined;
  return <section className="route-card" aria-labelledby="app-title"><p className="route-card__eyebrow">自动化运营 · 本地任务</p><h1 id="app-title">投递任务观察与本地对账</h1><p>本页只读取本地任务、内部尝试和本地收据。任何状态均不表示企业微信或其他外部系统已经执行、送达或确认。</p><p><label>内部状态 <select value={status} onChange={(event) => onStatus(event.target.value)} disabled={state.kind === "loading"}><option value="">全部</option>{statusValues.slice(1).map((item) => <option key={item} value={item}>{item}</option>)}</select></label>{" "}<label>批次 ID <input value={businessID} inputMode="numeric" onChange={(event) => onBusinessID(event.target.value)} disabled={state.kind === "loading"} /></label>{" "}<button type="button" onClick={() => onLoad(0)} disabled={state.kind === "loading"}>筛选本地任务</button></p>{page ? <table><thead><tr><th>任务 ID</th><th>批次 ID</th><th>内部状态</th><th>尝试次数</th><th>代际</th><th>队列种类</th><th>更新时间</th><th>本地对账</th><th>本地取消</th></tr></thead><tbody>{page.items.length === 0 ? <tr><td colSpan={9}>当前筛选没有本地任务。</td></tr> : page.items.map((item) => <tr key={item.taskID}><td>{item.taskID}</td><td>{item.businessID ?? "—"}</td><td>{item.status}</td><td>{item.attemptCount}</td><td>{item.generation}</td><td>{item.queueKind}</td><td>{item.updatedAt}</td><td><button type="button" onClick={() => onSelect(item.taskID)} disabled={detail.kind === "loading"}>读取本地对账</button></td><td>{canCancel && item.status === "pending" ? <><label><input aria-label={`确认取消本地任务 ${item.taskID}`} type="checkbox" checked={confirmingTaskID === item.taskID} disabled={cancelLocked} onChange={(event) => onConfirmTask(event.currentTarget.checked ? item.taskID : undefined)} />我确认仅取消尚未运行的本地任务</label><button type="button" onClick={() => onCancel?.(item)} disabled={cancelLocked || confirmingTaskID !== item.taskID}>取消本地任务</button></> : "—"}</td></tr>)}</tbody></table> : null}{state.kind === "loading" ? <p role="status">正在读取本地投递任务。</p> : null}{state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}{detail.kind === "loading" ? <p role="status">正在读取本地尝试与收据。</p> : null}{detail.kind === "error" ? <p role="alert">{messages[detail.failure]}</p> : null}{cancelNotice ? <p role="alert">{cancelMessages[cancelNotice]}</p> : null}{reconciliation ? <Reconciliation value={reconciliation} /> : null}{page ? <p><button type="button" disabled={state.kind === "loading" || page.offset === 0} onClick={() => onLoad(Math.max(0, page.offset - OUTBOUND_OPERATIONS_PAGE_SIZE))}>上一页</button>{" "}<button type="button" disabled={state.kind === "loading" || !page.hasMore} onClick={() => onLoad(page.offset + OUTBOUND_OPERATIONS_PAGE_SIZE)}>下一页</button></p> : null}</section>;
}
export function OutboundOperationsPage({ role, transport = generatedOutboundOperationsTransport, readCookie = browserCookie, onUnauthenticated }: { readonly role: OutboundOperationsRole; readonly transport?: OutboundOperationsTransport; readonly readCookie?: () => string; readonly onUnauthenticated?: () => void; }): React.ReactElement {
  const canRead = role === "admin"; const pageGeneration = useRef(0); const detailGeneration = useRef(0); const mutationGeneration = useRef(0); const lifetime = useRef(0); const pageFlight = useRef<Flight>(); const detailFlight = useRef<Flight>(); const mutationFlight = useRef<Flight>(); const mutationUnknown = useRef(false); const verifiedPage = useRef<OutboundTaskPage>(); const verifiedDetail = useRef<OutboundReconciliation>(); const unauthenticated = useRef(false); const [status, setStatus] = useState(""); const [businessID, setBusinessID] = useState(""); const [state, setState] = useState<PageState>({ kind: "loading" }); const [detail, setDetail] = useState<DetailState>({ kind: "idle" }); const [confirmingTaskID, setConfirmingTaskID] = useState<string>(); const [cancelNotice, setCancelNotice] = useState<CancelNotice>();
  const report401 = useCallback(() => { if (!unauthenticated.current) { unauthenticated.current = true; onUnauthenticated?.(); } }, [onUnauthenticated]);
  const load = useCallback(async (offset: number) => { const params = { status: status || undefined, businessID: businessID || undefined, offset }; const key = JSON.stringify(params); if (pageFlight.current?.key === key) return; const flight: Flight = { key, token: Symbol("outbound-page") }; pageFlight.current = flight; const generation = ++pageGeneration.current; setState({ kind: "loading", previous: verifiedPage.current }); try { const result = await loadOutboundTasks(transport, params); if (generation !== pageGeneration.current) return; if (result.status === "loaded") { verifiedPage.current = result.value; setState({ kind: "ready", page: result.value }); return; } if (result.status === "unauthenticated") report401(); setState({ kind: "error", failure: result.status, previous: verifiedPage.current }); } finally { if (pageFlight.current?.token === flight.token) pageFlight.current = undefined; } }, [businessID, report401, status, transport]);
  const select = useCallback(async (taskID: string) => { if (detailFlight.current?.key === taskID) return; const flight: Flight = { key: taskID, token: Symbol("outbound-reconciliation") }; detailFlight.current = flight; const generation = ++detailGeneration.current; setDetail({ kind: "loading", previous: verifiedDetail.current }); try { const result = await loadOutboundReconciliation(transport, taskID); if (generation !== detailGeneration.current) return; if (result.status === "loaded") { verifiedDetail.current = result.value; setDetail({ kind: "ready", value: result.value }); return; } if (result.status === "unauthenticated") report401(); setDetail({ kind: "error", failure: result.status, previous: verifiedDetail.current }); } finally { if (detailFlight.current?.token === flight.token) detailFlight.current = undefined; } }, [report401, transport]);
  const cancel = useCallback(async (task: OutboundTask) => {
    if (mutationUnknown.current || mutationFlight.current || task.status !== "pending" || confirmingTaskID !== task.taskID || !transport.cancel) return;
    let csrf: string | undefined; try { csrf = readCSRFCookie(readCookie()); } catch { csrf = undefined; }
    const key = newOutboundCancelIdempotencyKey(); if (!csrf || !key) { setCancelNotice("csrf_missing"); return; }
    const flight: Flight = { key: task.taskID, token: Symbol("outbound-cancel") }; mutationFlight.current = flight; const generation = ++mutationGeneration.current; const activeLifetime = lifetime.current; setCancelNotice(undefined);
    try {
      const result = await cancelPendingOutboundTask(transport, task, csrf, key);
      if (generation !== mutationGeneration.current || lifetime.current !== activeLifetime || mutationFlight.current?.token !== flight.token) return;
      if (result.status !== "cancelled") { if (result.status === "unauthenticated") report401(); if (result.status === "unknown" || result.status === "unavailable" || result.status === "unauthenticated") mutationUnknown.current = true; setCancelNotice(result.status); return; }
      const reread = await loadOutboundReconciliation(transport, task.taskID);
      if (generation !== mutationGeneration.current || lifetime.current !== activeLifetime || mutationFlight.current?.token !== flight.token) return;
      if (reread.status === "loaded" && confirmsCancelledOutboundTask(reread.value, result.receipt)) { verifiedDetail.current = reread.value; setDetail({ kind: "ready", value: reread.value }); setConfirmingTaskID(undefined); setCancelNotice("confirmed"); void load(0); return; }
      if (reread.status === "unauthenticated") report401(); mutationUnknown.current = true; setCancelNotice(reread.status === "loaded" ? "unknown" : reread.status === "unavailable" ? "unknown" : reread.status);
    } finally { if (mutationFlight.current?.token === flight.token) mutationFlight.current = undefined; }
  }, [confirmingTaskID, load, readCookie, report401, transport]);
  useEffect(() => { const currentLifetime = ++lifetime.current; if (canRead) { setDetail(verifiedDetail.current ? { kind: "ready", value: verifiedDetail.current } : { kind: "idle" }); void load(0); } return () => { pageGeneration.current += 1; detailGeneration.current += 1; mutationGeneration.current += 1; if (mutationFlight.current) mutationUnknown.current = true; pageFlight.current = undefined; detailFlight.current = undefined; mutationFlight.current = undefined; if (lifetime.current === currentLifetime) lifetime.current += 1; }; }, [canRead, load]);
  if (!canRead) return <section className="route-card" aria-labelledby="app-title"><h1 id="app-title">投递任务运营与本地对账</h1><p>当前账号没有本地投递运营查看权限。</p></section>;
  return <OutboundOperationsView state={state} detail={detail} onLoad={(offset) => void load(offset)} onSelect={(taskID) => void select(taskID)} onCancel={(task) => void cancel(task)} canCancel={transport.cancel !== undefined} cancelLocked={mutationUnknown.current || mutationFlight.current !== undefined} confirmingTaskID={confirmingTaskID} onConfirmTask={setConfirmingTaskID} cancelNotice={cancelNotice} status={status} businessID={businessID} onStatus={setStatus} onBusinessID={setBusinessID} />;
}
