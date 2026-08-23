import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import type { SessionStorageLike } from "./campaign-touch-plan-core";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";
import type { TouchPlanReviewSnapshot } from "./campaign-touch-plan-review";
import type { CloudOrchestratorRole } from "./cloud-orchestrator";
import {
  OutboundCampaignHandoffMachine,
  generatedOutboundCampaignHandoffTransport,
  loadOutboundCampaignHandoffReconciliation,
  loadOutboundCampaignHandoffSummary,
  type HandoffMutationResult,
  type OutboundCampaignHandoffReconciliation,
  type OutboundCampaignHandoffSummary,
  type OutboundCampaignHandoffTransport,
} from "./outbound-campaign-handoff";

const unavailableStorage: SessionStorageLike = { getItem: () => { throw new Error("storage unavailable"); }, setItem: () => { throw new Error("storage unavailable"); }, removeItem: () => {} };
const unavailableKeySource = { randomUUID: () => { throw new Error("secure random unavailable"); } };
function runtimeStorage(): SessionStorageLike { try { return typeof window === "undefined" ? unavailableStorage : window.sessionStorage ?? unavailableStorage; } catch { return unavailableStorage; } }

export interface OutboundCampaignHandoffPanelProps {
  readonly role: CloudOrchestratorRole;
  readonly actorID: number;
  readonly plan: TouchPlanSummary;
  readonly approved: TouchPlanReviewSnapshot;
  readonly transport?: OutboundCampaignHandoffTransport;
  readonly sessionStorage?: SessionStorageLike;
  readonly readCookie?: () => string;
  readonly keySource?: { readonly randomUUID: () => string };
  readonly onUnauthenticated?: () => void;
}

function notice(result: HandoffMutationResult | undefined): string | undefined {
  if (!result) return undefined;
  switch (result.status) {
    case "completed": return "已接受为本地 held 事实；未创建 Outbound 发送任务，未调用 Provider，未证明发送或送达。";
    case "conflict": return "接受版本已冲突并完成本地回读；确认词已清空，请核对后创建新意图。";
    case "confirmation_required": return "必须输入与当前计划完全一致的 ACCEPT 确认词。";
    case "outcome_unknown": return "接受结果未知；原意图已保留，只允许精确重放。";
    case "replay_required": return "存在结果未知的在途意图，只允许精确重放。";
    case "replay_mismatch": return "当前 actor、计划或审核交接与在途意图不一致，禁止重放。";
    case "unauthenticated": return "登录已失效。";
    case "forbidden": return "当前账号无权接受本地交接。";
    case "not_found": return "已批准的本地交接事实不存在。";
    case "storage_unavailable": return "无法安全持久化接受意图，未发出请求。";
    case "inflight": return undefined;
    default: return "输入或服务响应不符合本地 held 安全合同。";
  }
}

function Counts({ value }: { readonly value: OutboundCampaignHandoffReconciliation }): React.ReactElement {
  return <dl aria-label="本地 held 对账">
    <dt>本地 held</dt><dd>{value.heldCount}</dd><dt>blocked</dt><dd>{value.blockedCount}</dd><dt>pending</dt><dd>{value.pendingCount}</dd>
    <dt>资格未评估</dt><dd>{value.notEvaluatedCount}</dd><dt>可进入后续处理</dt><dd>{value.eligibleCount}</dd><dt>inactive</dt><dd>{value.inactiveCount}</dd><dt>contact_policy</dt><dd>{value.contactPolicyCount}</dd>
  </dl>;
}

function HandoffPanelInner({
  actorID, plan, approved, transport = generatedOutboundCampaignHandoffTransport, sessionStorage = runtimeStorage(),
  readCookie = () => typeof document === "undefined" ? "" : document.cookie,
  keySource = typeof crypto === "undefined" ? unavailableKeySource : crypto, onUnauthenticated,
}: Omit<OutboundCampaignHandoffPanelProps, "role">): React.ReactElement {
  const [loadState, setLoadState] = useState<"loading" | "not_accepted" | "loaded" | "unavailable">("loading");
  const [summary, setSummary] = useState<OutboundCampaignHandoffSummary>();
  const [reconciliation, setReconciliation] = useState<OutboundCampaignHandoffReconciliation>();
  const [confirmation, setConfirmation] = useState("");
  const [action, setAction] = useState<HandoffMutationResult>();
  const [busy, setBusy] = useState(false);
  const generation = useRef(0); const controller = useRef<AbortController>(); const mounted = useRef(false);
  const machine = useMemo(() => new OutboundCampaignHandoffMachine({ transport, sessionStorage, actorID, keySource }), [actorID, keySource, sessionStorage, transport]);
  const currentMachine = useRef(machine);
  useLayoutEffect(() => { mounted.current = true; currentMachine.current = machine; return () => { controller.current?.abort(); mounted.current = false; ++generation.current; }; }, [machine]);
  const current = (request: number): boolean => mounted.current && currentMachine.current === machine && generation.current === request;

  useEffect(() => {
    const request = ++generation.current; const abort = new AbortController(); controller.current?.abort(); controller.current = abort;
    setLoadState("loading"); setSummary(undefined); setReconciliation(undefined); setConfirmation(""); setAction(undefined); setBusy(false);
    void loadOutboundCampaignHandoffSummary(transport, plan, approved, abort.signal).then((result) => {
      if (controller.current === abort) controller.current = undefined; if (!current(request)) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "loaded") { setSummary(result.summary); setLoadState("loaded"); }
      else setLoadState(result.status === "not_accepted" ? "not_accepted" : "unavailable");
    });
    return () => abort.abort();
  }, [approved, machine, onUnauthenticated, plan, transport]);

  const csrf = (): string => { try { return readCSRFCookie(readCookie()) ?? ""; } catch { return ""; } };
  const accept = async (replay: boolean): Promise<void> => {
    if (!mounted.current || currentMachine.current !== machine || controller.current) return;
    const abort = new AbortController(); controller.current = abort; const request = generation.current; setBusy(true);
    const input = { plan, approved, confirmation, csrf: csrf(), signal: abort.signal };
    const result = replay ? await machine.replay(input) : await machine.start(input);
    if (controller.current === abort) controller.current = undefined; if (!current(request) || result.status === "inflight") return;
    setBusy(false); if (result.status === "unauthenticated") onUnauthenticated?.();
    if (result.status === "completed") { setSummary(result.reconciliation); setReconciliation(result.reconciliation); setLoadState("loaded"); setConfirmation(""); }
    else if (result.status === "conflict") { setSummary(result.summary); setReconciliation(undefined); setLoadState(result.summary ? "loaded" : "not_accepted"); setConfirmation(""); }
    setAction(result);
  };
  const reconcile = async (): Promise<void> => {
    if (!summary || controller.current) return; const abort = new AbortController(); controller.current = abort; const request = generation.current; setBusy(true);
    const result = await loadOutboundCampaignHandoffReconciliation(transport, plan, approved, abort.signal);
    if (controller.current === abort) controller.current = undefined; if (!current(request)) return; setBusy(false);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    if (result.status === "loaded") setReconciliation(result.reconciliation); else setAction({ status: result.status === "unavailable" ? "invalid" : result.status });
  };
  const replayAllowed = action?.status === "outcome_unknown" || action?.status === "replay_required";
  return <section aria-labelledby="outbound-campaign-handoff-title">
    <h3 id="outbound-campaign-handoff-title">Outbound 本地 held 交接</h3><p>计划：{plan.id}</p>
    <p role="note">接受仅创建本地 held 快照与内部事件投递；不创建 Outbound 发送任务，不调用 Provider，不证明发送或送达。</p>
    {loadState === "loading" ? <p role="status">正在读取本地交接事实…</p> : null}
    {loadState === "unavailable" ? <p role="alert">本地交接响应不符合安全合同，禁止操作。</p> : null}
    {loadState === "not_accepted" ? <><label>接受确认词<input aria-label="接受确认词" value={confirmation} disabled={busy || replayAllowed} onChange={(event) => setConfirmation(event.currentTarget.value)} /></label>
      <button type="button" disabled={busy || replayAllowed || confirmation !== `ACCEPT ${plan.id}`} onClick={() => void accept(false)}>接受为本地 held 事实</button></> : null}
    {summary ? <><p role="status">本地状态：held；审核版本 {summary.reviewVersion}；目标 {summary.targetCount}；步骤 {summary.stepCount}。</p>
      <button type="button" disabled={busy} onClick={() => void reconcile()}>刷新本地核对</button></> : null}
    {reconciliation ? <Counts value={reconciliation} /> : null}
    {replayAllowed ? <button type="button" disabled={busy} onClick={() => void accept(true)}>精确重放原接受意图</button> : null}
    {notice(action) ? <p role={action?.status === "completed" ? "status" : "alert"}>{notice(action)}</p> : null}
  </section>;
}

export function OutboundCampaignHandoffPanel(props: OutboundCampaignHandoffPanelProps): React.ReactElement {
  if (props.role === "sales") return <section aria-label="Outbound 本地 held 交接"><p role="alert">当前账号没有本地交接接受权限。</p></section>;
  const createdAt = props.approved.handoff?.createdAt ?? "invalid";
  return <HandoffPanelInner key={`${props.actorID}:${props.plan.campaignCode}:${props.plan.id}:${props.plan.immutable}:${createdAt}`} {...props} />;
}
