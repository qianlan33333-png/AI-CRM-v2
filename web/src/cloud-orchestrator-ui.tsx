import React, { useMemo, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  CampaignTouchPlanMachine,
  generatedCampaignTouchPlanTransport,
  loadDraftCampaign,
  loadDraftCampaigns,
  type CampaignDraft,
  type CampaignDraftSummary,
  type CampaignTouchPlanResult,
  type CampaignTouchPlanTransport,
  type SessionStorageLike,
} from "./campaign-touch-plan-core";
import {
  CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
  CLOUD_ORCHESTRATOR_PLANS_PATH,
  cloudOrchestratorWorkspaceLinks,
  type CloudOrchestratorRole,
  type CloudOrchestratorRoute,
  type CloudCampaignSourceKind,
} from "./cloud-orchestrator";

export function CloudOrchestratorBoundary(): React.ReactElement {
  return (
    <p role="note">
      此处仅承载本地审阅与导航。草稿、待审阅、页面可见或本地统计均不表示
      Provider 已调用、外部发送已执行或消息已送达。
    </p>
  );
}

export function CloudOrchestratorNavigation({
  role = "admin",
}: {
  readonly role?: CloudOrchestratorRole;
}): React.ReactElement {
  return (
    <nav aria-label="AI 助手工作区">
      {cloudOrchestratorWorkspaceLinks
        .filter(
          (link) =>
            role === "admin" || link.href === CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
        )
        .map((link) => (
          <a key={link.href} href={link.href}>
            {link.label}
          </a>
        ))}
    </nav>
  );
}

function PlansWorkspace(): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">运营计划审阅</h1>
      <CloudOrchestratorBoundary />
      <p role="status">
        页面载体已就绪；计划列表必须由后续冻结的本地只读合同提供，当前不猜测计划、受众或审批字段。
      </p>
    </section>
  );
}

function PlanDetailWorkspace({
  planID,
}: {
  readonly planID: string;
}): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">运营计划明细</h1>
      <CloudOrchestratorBoundary />
      <dl>
        <dt>计划标识</dt>
        <dd>{planID}</dd>
      </dl>
      <section aria-labelledby="cloud-plan-recipients">
        <h2 id="cloud-plan-recipients">目标人员审阅</h2>
        <p role="status">尚无已冻结的目标人员读模型，不展示或推断人员数据。</p>
      </section>
      <section aria-labelledby="cloud-plan-single-review">
        <h2 id="cloud-plan-single-review">单人审批</h2>
        <p role="status">尚无已冻结的单人审批合同，不提供可变更状态的操作。</p>
      </section>
      <a href={CLOUD_ORCHESTRATOR_PLANS_PATH}>返回运营计划</a>
    </section>
  );
}

type PanelAction =
  | { readonly status: "ready" | "pending" }
  | {
      readonly status: CampaignTouchPlanResult["status"];
      readonly result: CampaignTouchPlanResult;
    };

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

function runtimeSessionStorage(): SessionStorageLike {
  try {
    return typeof window === "undefined"
      ? unavailableStorage
      : (window.sessionStorage ?? unavailableStorage);
  } catch {
    return unavailableStorage;
  }
}

function sourceSummary(kind: CloudCampaignSourceKind, id: string): string {
  const label = {
    customer_selection: "单个 Customer OneID",
    segment_members: "Segment",
    ai_audience_package_members: "AI Audience package",
  }[kind];
  return `${label} #${id}`;
}

function safeSourceID(value: string): boolean {
  const number = Number(value);
  return Number.isSafeInteger(number) && number > 0 && String(number) === value;
}

function actionMessage(action: PanelAction): string | undefined {
  if (action.status === "ready" || action.status === "pending")
    return undefined;
  switch (action.status) {
    case "created":
      return `本地触达草稿已创建（${action.result.status === "created" ? action.result.plan.id : ""}）；不表示 Outbound、Provider 调用、发送或送达。`;
    case "outcome_unknown":
      return "结果未知；已保留原意图，只允许精确重放。";
    case "replay_required":
      return "检测到结果未知的在途意图，只允许精确重放。";
    case "conflict":
      return "Campaign 事实已刷新；必须重新确认并发起新意图。";
    case "blocked_redline":
      return "BLOCKED_REDLINE：触达计划已被安全红线阻断。";
    case "confirmation_required":
      return "请先确认仅创建本地草稿。";
    case "inflight":
      return "同一创建请求正在处理。";
    case "unauthenticated":
      return "登录已失效。";
    case "forbidden":
      return "当前账号无权创建本地触达草稿。";
    case "not_found":
      return "Campaign 已不存在，请刷新后重试。";
    case "replay_mismatch":
      return "当前选择与在途意图不一致，禁止重放。";
    case "no_pending":
      return "没有可重放的在途意图。";
    case "storage_unavailable":
      return "无法安全持久化创建意图，已停止请求。";
    default:
      return "输入或服务响应不符合安全合同，未确认创建成功。";
  }
}

interface CampaignTouchPlanPanelProps {
  readonly sourceKind: CloudCampaignSourceKind;
  readonly sourceID: string;
  readonly actorID: number;
  readonly transport?: CampaignTouchPlanTransport;
  readonly sessionStorage?: SessionStorageLike;
  readonly readCookie?: () => string;
  readonly keySource?: { readonly randomUUID: () => string };
  readonly onUnauthenticated?: () => void;
}

export function CampaignTouchPlanPanel(
  props: CampaignTouchPlanPanelProps,
): React.ReactElement {
  return (
    <CampaignTouchPlanPanelInner
      key={`${props.actorID}:${props.sourceKind}:${props.sourceID}`}
      {...props}
    />
  );
}

function CampaignTouchPlanPanelInner({
  sourceKind,
  sourceID,
  actorID,
  transport = generatedCampaignTouchPlanTransport,
  sessionStorage = runtimeSessionStorage(),
  readCookie = () => (typeof document === "undefined" ? "" : document.cookie),
  keySource = typeof crypto !== "undefined" ? crypto : unavailableKeySource,
  onUnauthenticated,
}: CampaignTouchPlanPanelProps): React.ReactElement {
  const safeSource = safeSourceID(sourceID);
  const [campaigns, setCampaigns] = useState<readonly CampaignDraftSummary[]>(
    [],
  );
  const [listState, setListState] = useState<
    "loading" | "loaded" | "unavailable"
  >("loading");
  const [selectedCode, setSelectedCode] = useState("");
  const [selected, setSelected] = useState<CampaignDraft>();
  const [detailState, setDetailState] = useState<
    "idle" | "loading" | "loaded" | "unavailable"
  >("idle");
  const [confirmed, setConfirmed] = useState(false);
  const [action, setAction] = useState<PanelAction>({ status: "ready" });
  const listSequence = useRef(0);
  const detailSequence = useRef(0);
  const machine = useMemo(
    () =>
      new CampaignTouchPlanMachine({
        transport,
        sessionStorage,
        actorID,
        keySource,
      }),
    [actorID, keySource, sessionStorage, transport],
  );
  const mounted = useRef(false);
  const currentMachine = useRef(machine);
  React.useLayoutEffect(() => {
    mounted.current = true;
    currentMachine.current = machine;
    setAction({ status: "ready" });
    setConfirmed(false);
    return () => {
      mounted.current = false;
    };
  }, [machine]);
  const isCurrent = () => mounted.current && currentMachine.current === machine;

  React.useEffect(() => {
    if (!safeSource) return;
    const sequence = ++listSequence.current;
    setListState("loading");
    void loadDraftCampaigns(transport).then((result) => {
      if (!isCurrent() || sequence !== listSequence.current) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "loaded") {
        setCampaigns(result.campaigns);
        setListState("loaded");
      } else {
        setCampaigns([]);
        setListState("unavailable");
      }
    });
  }, [machine, onUnauthenticated, safeSource, transport]);

  React.useEffect(() => {
    const sequence = ++detailSequence.current;
    setSelected(undefined);
    setConfirmed(false);
    setAction({ status: "ready" });
    if (!selectedCode) {
      setDetailState("idle");
      return;
    }
    setDetailState("loading");
    void loadDraftCampaign(transport, selectedCode).then((result) => {
      if (!isCurrent() || sequence !== detailSequence.current) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "loaded") {
        setSelected(result.campaign);
        setDetailState("loaded");
      } else {
        setDetailState("unavailable");
      }
    });
  }, [machine, onUnauthenticated, selectedCode, transport]);

  const run = async (replay: boolean): Promise<void> => {
    if (!selected || !isCurrent()) return;
    setAction({ status: "pending" });
    let csrf = "";
    try {
      csrf = readCSRFCookie(readCookie()) ?? "";
    } catch {
      // The core will fail closed without a valid CSRF token.
    }
    const input = {
      campaign: selected,
      source_kind: sourceKind,
      source_id: sourceID,
      csrf,
      confirmed,
    };
    const result = replay
      ? await machine.replay(input)
      : await machine.start(input);
    if (!isCurrent() || result.status === "inflight") return;
    if (result.status === "unauthenticated") {
      if (!isCurrent()) return;
      onUnauthenticated?.();
    }
    if (result.status === "conflict" && result.campaign) {
      if (!isCurrent()) return;
      setSelected(result.campaign);
    }
    if (result.status === "conflict") {
      if (!isCurrent()) return;
      setConfirmed(false);
    }
    if (!isCurrent()) return;
    setAction({ status: result.status, result });
  };

  if (!safeSource) {
    return (
      <section aria-labelledby="campaign-touch-plan-title">
        <h2 id="campaign-touch-plan-title">创建触达计划</h2>
        <p>来源：{sourceSummary(sourceKind, sourceID)}</p>
        <p role="alert">
          BLOCKED_REDLINE：来源标识超出浏览器安全整数范围，未发出请求。
        </p>
      </section>
    );
  }

  const message = actionMessage(action);
  const replayAllowed =
    action.status === "outcome_unknown" || action.status === "replay_required";
  return (
    <section aria-labelledby="campaign-touch-plan-title">
      <h2 id="campaign-touch-plan-title">创建触达计划</h2>
      <p>来源：{sourceSummary(sourceKind, sourceID)}</p>
      <label>
        Campaign
        <select
          value={selectedCode}
          disabled={action.status === "pending" || listState !== "loaded"}
          onChange={(event) => setSelectedCode(event.currentTarget.value)}
        >
          <option value="">请选择本地草稿</option>
          {campaigns.map((campaign) => (
            <option key={campaign.code} value={campaign.code}>
              {campaign.name}
            </option>
          ))}
        </select>
      </label>
      {listState === "loading" ? (
        <p role="status">正在读取 Campaign 草稿…</p>
      ) : null}
      {listState === "unavailable" ? (
        <p role="alert">Campaign 列表响应不符合安全合同。</p>
      ) : null}
      {detailState === "loading" ? (
        <p role="status">正在校验 Campaign 明细…</p>
      ) : null}
      {detailState === "unavailable" ? (
        <p role="alert">Campaign 明细响应不符合安全合同，禁止创建。</p>
      ) : null}
      {selected ? (
        <dl aria-label="Campaign 明细摘要">
          <dt>名称</dt>
          <dd>{selected.name}</dd>
          <dt>代码</dt>
          <dd>{selected.code}</dd>
          <dt>版本</dt>
          <dd>版本 {selected.version}</dd>
          <dt>步骤</dt>
          <dd>{selected.steps.length} 个本地步骤</dd>
        </dl>
      ) : null}
      <label>
        <input
          type="checkbox"
          checked={confirmed}
          disabled={!selected || action.status === "pending"}
          onChange={(event) => setConfirmed(event.currentTarget.checked)}
        />
        仅创建本地草稿，不发送
      </label>
      <button
        type="button"
        disabled={
          !selected ||
          !confirmed ||
          action.status === "pending" ||
          replayAllowed
        }
        onClick={() => void run(false)}
      >
        {action.status === "pending" ? "正在创建…" : "创建本地草稿"}
      </button>
      {replayAllowed ? (
        <button type="button" onClick={() => void run(true)}>
          精确重放
        </button>
      ) : null}
      {message ? (
        <p role={action.status === "created" ? "status" : "alert"}>{message}</p>
      ) : null}
    </section>
  );
}

function CampaignsWorkspace({
  route,
  actorID,
  transport,
  sessionStorage,
  readCookie,
  keySource,
  onUnauthenticated,
}: {
  readonly route: Extract<
    CloudOrchestratorRoute,
    { readonly kind: "campaigns" }
  >;
  readonly actorID?: number;
  readonly transport?: CampaignTouchPlanTransport;
  readonly sessionStorage?: SessionStorageLike;
  readonly readCookie?: () => string;
  readonly keySource?: { readonly randomUUID: () => string };
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">Campaign 审阅工作区</h1>
      <CloudOrchestratorBoundary />
      {route.source_kind && route.source_id && actorID ? (
        <CampaignTouchPlanPanel
          sourceKind={route.source_kind}
          sourceID={route.source_id}
          actorID={actorID}
          transport={transport}
          sessionStorage={sessionStorage}
          readCookie={readCookie}
          keySource={keySource}
          onUnauthenticated={onUnauthenticated}
        />
      ) : (
        <p role="status">
          请从 Customer、Segment 或 AI Audience
          发起触达；此处不提供通用来源输入。
        </p>
      )}
    </section>
  );
}

function ObservabilityWorkspace(): React.ReactElement {
  return (
    <section aria-labelledby="cloud-orchestrator-title">
      <h1 id="cloud-orchestrator-title">AI 助手可观察性</h1>
      <CloudOrchestratorBoundary />
      <div aria-label="可观察性入口">
        <section>
          <h2>工单</h2>
          <p>仅接入已冻结的本地工单读模型。</p>
        </section>
        <section>
          <h2>审计</h2>
          <p>仅接入已冻结的本地审计读模型。</p>
        </section>
        <section>
          <h2>漏斗</h2>
          <p>仅接入已冻结的本地漏斗读模型。</p>
        </section>
        <section>
          <h2>Tool 调用统计</h2>
          <p>仅接入已冻结的本地调用观测。</p>
        </section>
      </div>
    </section>
  );
}

export function CloudOrchestratorWorkspace({
  role,
  route,
  actorID,
  campaignTransport,
  sessionStorage,
  readCookie,
  keySource,
  onUnauthenticated,
}: {
  readonly role: CloudOrchestratorRole;
  readonly route: CloudOrchestratorRoute;
  readonly actorID?: number;
  readonly campaignTransport?: CampaignTouchPlanTransport;
  readonly sessionStorage?: SessionStorageLike;
  readonly readCookie?: () => string;
  readonly keySource?: { readonly randomUUID: () => string };
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  if (role === "sales" || (role === "ops" && route.kind !== "campaigns")) {
    return (
      <section aria-labelledby="cloud-orchestrator-title">
        <h1 id="cloud-orchestrator-title">AI 助手</h1>
        <p role="alert">当前账号没有 AI 助手本地审阅权限。</p>
      </section>
    );
  }

  return (
    <main>
      <CloudOrchestratorNavigation role={role} />
      {route.kind === "root" || route.kind === "plans" ? (
        <PlansWorkspace />
      ) : null}
      {route.kind === "plan_detail" ? (
        <PlanDetailWorkspace planID={route.planID} />
      ) : null}
      {route.kind === "campaigns" ? (
        <CampaignsWorkspace
          route={route}
          actorID={actorID}
          transport={campaignTransport}
          sessionStorage={sessionStorage}
          readCookie={readCookie}
          keySource={keySource}
          onUnauthenticated={onUnauthenticated}
        />
      ) : null}
      {route.kind === "observability" ? <ObservabilityWorkspace /> : null}
    </main>
  );
}
