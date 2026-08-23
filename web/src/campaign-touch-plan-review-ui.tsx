import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import type { SessionStorageLike } from "./campaign-touch-plan-core";
import type { TouchPlanSummary } from "./campaign-touch-plan-read";
import {
  CampaignTouchPlanReviewMachine,
  generatedCampaignTouchPlanReviewTransport,
  loadTouchPlanReview,
  type CampaignTouchPlanReviewTransport,
  type ReviewMutationResult,
  type ReviewOperation,
  type TouchPlanReview,
  type TouchPlanReviewHandoff,
  type TouchPlanReviewSnapshot,
} from "./campaign-touch-plan-review";
import type { CloudOrchestratorRole } from "./cloud-orchestrator";

const unavailableStorage: SessionStorageLike = {
  getItem: () => {
    throw new Error("session storage unavailable");
  },
  setItem: () => {
    throw new Error("session storage unavailable");
  },
  removeItem: () => {},
};
const unavailableKeySource = {
  randomUUID: () => {
    throw new Error("secure random source unavailable");
  },
};
function runtimeStorage(): SessionStorageLike {
  try {
    return typeof window === "undefined" ? unavailableStorage : (window.sessionStorage ?? unavailableStorage);
  } catch {
    return unavailableStorage;
  }
}

export interface CampaignTouchPlanReviewPanelProps {
  readonly role: CloudOrchestratorRole;
  readonly actorID: number;
  readonly plan: TouchPlanSummary;
  readonly transport?: CampaignTouchPlanReviewTransport;
  readonly sessionStorage?: SessionStorageLike;
  readonly readCookie?: () => string;
  readonly keySource?: { readonly randomUUID: () => string };
  readonly onUnauthenticated?: () => void;
  // eslint-disable-next-line no-unused-vars -- callback parameter documents the approved C2 seam.
  readonly onReviewSelected?: (review: TouchPlanReviewSnapshot | undefined) => void;
}

function message(result: ReviewMutationResult | undefined): string | undefined {
  if (!result) return undefined;
  switch (result.status) {
    case "completed":
      return "本地人工审核事实已更新；未执行 Outbound 接受、Provider 调用、发送或送达。";
    case "conflict":
      return "审核版本已冲突并完成本地回读；确认词已清空，请核对后创建新意图。";
    case "blocked":
      return "BLOCKED_REDLINE：本次人工审核已被安全红线阻断。";
    case "confirmation_required":
      return "必须输入与当前计划完全一致的确认词。";
    case "outcome_unknown":
      return "结果未知；原意图已保留，只允许精确重放。";
    case "replay_required":
      return "存在结果未知的在途意图，只允许精确重放。";
    case "replay_mismatch":
      return "当前计划、版本、操作或确认词与在途意图不一致，禁止重放。";
    case "unauthenticated":
      return "登录已失效。";
    case "forbidden":
      return "当前账号无权执行人工审核。";
    case "not_found":
      return "当前本地触达计划或审核事实不存在。";
    case "storage_unavailable":
      return "无法安全持久化审核意图，未发出请求。";
    case "inflight":
      return undefined;
    default:
      return "输入或服务响应不符合人工审核安全合同。";
  }
}
function statusLabel(review: TouchPlanReview): string {
  return {
    draft: "草稿",
    pending_review: "待人工审核",
    approved: "已批准本地交接",
    rejected: "已拒绝",
  }[review.status];
}

function ReviewPanelInner({
  actorID,
  plan,
  transport = generatedCampaignTouchPlanReviewTransport,
  sessionStorage = runtimeStorage(),
  readCookie = () => (typeof document === "undefined" ? "" : document.cookie),
  keySource = typeof crypto === "undefined" ? unavailableKeySource : crypto,
  onUnauthenticated,
  onReviewSelected,
}: Omit<CampaignTouchPlanReviewPanelProps, "role">): React.ReactElement {
  const [review, setReview] = useState<TouchPlanReview>();
  const [handoff, setHandoff] = useState<TouchPlanReviewHandoff>();
  const [loadState, setLoadState] = useState<"loading" | "loaded" | "unavailable">("loading");
  const [confirmation, setConfirmation] = useState("");
  const [operation, setOperation] = useState<ReviewOperation>();
  const [action, setAction] = useState<ReviewMutationResult>();
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);
  const actionController = useRef<AbortController>();
  const mounted = useRef(false);
  const machine = useMemo(
    () =>
      new CampaignTouchPlanReviewMachine({
        transport,
        sessionStorage,
        actorID,
        keySource,
      }),
    [actorID, keySource, sessionStorage, transport],
  );
  const currentMachine = useRef(machine);
  useLayoutEffect(() => {
    mounted.current = true;
    currentMachine.current = machine;
    onReviewSelected?.(undefined);
    return () => {
      actionController.current?.abort();
      mounted.current = false;
      ++generation.current;
      onReviewSelected?.(undefined);
    };
  }, [machine, onReviewSelected]);
  const current = (value: number): boolean => mounted.current && currentMachine.current === machine && generation.current === value;

  useEffect(() => {
    const request = ++generation.current;
    const controller = new AbortController();
    setReview(undefined);
    setHandoff(undefined);
    setConfirmation("");
    setOperation(undefined);
    setAction(undefined);
    setBusy(false);
    setLoadState("loading");
    onReviewSelected?.(undefined);
    void loadTouchPlanReview(transport, plan, controller.signal).then((result) => {
      if (!current(request)) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "loaded") {
        setReview(result.review);
        setHandoff(result.handoff);
        setLoadState("loaded");
        onReviewSelected?.(result.review.status === "approved" && result.handoff ? { review: result.review, handoff: result.handoff } : undefined);
      } else {
        onReviewSelected?.(undefined);
        setLoadState("unavailable");
      }
    });
    return () => controller.abort();
  }, [machine, onReviewSelected, onUnauthenticated, plan, transport]);

  const run = async (nextOperation: ReviewOperation, replay: boolean): Promise<void> => {
    if (!review || !mounted.current || currentMachine.current !== machine || actionController.current) return;
    const controller = new AbortController();
    actionController.current = controller;
    const request = generation.current;
    setBusy(true);
    setOperation(nextOperation);
    let csrf = "";
    try {
      csrf = readCSRFCookie(readCookie()) ?? "";
    } catch {
      /* core fails closed */
    }
    const result = replay
      ? await machine.replay({
          plan,
          review,
          operation: nextOperation,
          confirmation,
          csrf,
          signal: controller.signal,
        })
      : await machine.start({
          plan,
          review,
          operation: nextOperation,
          confirmation,
          csrf,
          signal: controller.signal,
        });
    if (actionController.current === controller) actionController.current = undefined;
    if (!current(request) || result.status === "inflight") return;
    setBusy(false);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    if (result.status === "completed") {
      setReview(result.review);
      setHandoff(result.handoff);
      setConfirmation("");
      onReviewSelected?.(result.review.status === "approved" && result.handoff ? { review: result.review, handoff: result.handoff } : undefined);
    } else if (result.status === "conflict") {
      if (result.review) setReview(result.review);
      setHandoff(result.handoff);
      setConfirmation("");
      setOperation(undefined);
      onReviewSelected?.(result.review?.status === "approved" && result.handoff ? { review: result.review, handoff: result.handoff } : undefined);
    }
    setAction(result);
  };
  const replayAllowed = action?.status === "outcome_unknown" || action?.status === "replay_required";
  const notice = message(action);

  return (
    <section aria-labelledby="campaign-touch-plan-review-title">
      <h3 id="campaign-touch-plan-review-title">触达计划人工审核</h3>
      <p>计划：{plan.id}</p>
      <p role="note">只更新本地审核与交接事实，不执行 Outbound 接受、Provider 调用、发送或送达。</p>
      {loadState === "loading" ? <p role="status">正在读取本地审核事实…</p> : null}
      {loadState === "unavailable" ? <p role="alert">审核响应不符合安全合同，禁止操作。</p> : null}
      {review ? (
        <dl aria-label="人工审核事实">
          <dt>状态</dt>
          <dd>{statusLabel(review)}</dd>
          <dt>版本</dt>
          <dd>{review.version}</dd>
        </dl>
      ) : null}
      {review?.status === "draft" ? (
        <button type="button" disabled={busy || replayAllowed} onClick={() => void run("submit", false)}>
          提交人工审核
        </button>
      ) : null}
      {review?.status === "pending_review" ? (
        <>
          <label>
            审核确认词
            <input
              aria-label="审核确认词"
              value={confirmation}
              disabled={busy || replayAllowed}
              onChange={(event) => setConfirmation(event.currentTarget.value)}
            />
          </label>
          <button type="button" disabled={busy || replayAllowed || confirmation !== `APPROVE ${plan.id}`} onClick={() => void run("approve", false)}>
            批准本地交接
          </button>
          <button type="button" disabled={busy || replayAllowed || confirmation !== `REJECT ${plan.id}`} onClick={() => void run("reject", false)}>
            拒绝本地交接
          </button>
        </>
      ) : null}
      {replayAllowed && operation ? (
        <button type="button" disabled={busy} onClick={() => void run(operation, true)}>
          精确重放原审核意图
        </button>
      ) : null}
      {handoff ? <p role="status">本地交接状态：待 Outbound 接受（审核版本 {handoff.reviewVersion}）。</p> : null}
      {notice ? <p role={action?.status === "completed" ? "status" : "alert"}>{notice}</p> : null}
    </section>
  );
}

export function CampaignTouchPlanReviewPanel(props: CampaignTouchPlanReviewPanelProps): React.ReactElement {
  if (props.role === "sales")
    return (
      <section aria-label="触达计划人工审核">
        <p role="alert">当前账号没有人工审核权限。</p>
      </section>
    );
  return <ReviewPanelInner key={`${props.actorID}:${props.plan.campaignCode}:${props.plan.id}:${props.plan.immutable}`} {...props} />;
}
