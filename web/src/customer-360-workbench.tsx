import React, {
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { readCSRFCookie, type AuthPrincipal } from "./auth";
import {
  generatedCustomerDetailTransport,
  loadCustomerTimelinePage,
  type CustomerDetailSnapshot,
  type CustomerDetailTransport,
  type CustomerProfile,
  type CustomerTag,
} from "./customer-detail";
import { parseProfileDraft } from "./customer-detail-ui";
import {
  generatedCustomerContextTransport,
  type CustomerContextTransport,
} from "./customer-context";
import { CustomerContextPanel } from "./customer-context-ui";
import {
  generatedCustomerMergeHistoryTransport,
  type CustomerMergeHistoryTransport,
} from "./customer-merge-history";
import { CustomerMergeHistoryPanel } from "./customer-merge-history-ui";
import {
  generatedCustomerChatActivityTransport,
  type CustomerChatActivityTransport,
} from "./customer-chat-activity";
import { CustomerChatActivityPanel } from "./customer-chat-activity-ui";
import {
  generatedCustomerActivityAnalyticsTransport,
  type CustomerActivityAnalyticsTransport,
} from "./customer-activity-analytics";
import { CustomerActivityAnalyticsPanel } from "./customer-activity-analytics-ui";
import {
  generatedStageTransport,
  type StageRecord,
  type StageTransport,
} from "./stages";
import {
  Customer360LatestRequestGate,
  Customer360MutationGuard,
  executeCustomer360Mutation,
  loadCustomer360Core,
  loadCustomer360StageCatalog,
  mergeConfirmedCustomer360Core,
  newCustomer360IdempotencyKey,
  resolveCustomer360Access,
  stageName,
  validCustomer360CustomerID,
  type Customer360MutationAction,
  type Customer360MutationLockReason,
  type Customer360MutationResult,
  type Customer360PanelState,
  type Customer360WorkbenchRole,
} from "./customer-360-workbench-model";
import "./customer-detail.css";
import "./customer-360-workbench.css";

interface ProfileDraft {
  readonly name: string;
  readonly avatarURL: string;
  readonly gender: string;
  readonly ownerStaffID: string;
  readonly channelID: string;
}

type CoreState = Customer360PanelState<CustomerDetailSnapshot>;
type StageState = Customer360PanelState<readonly StageRecord[]>;
type MutationUIState =
  | { readonly kind: "idle" }
  | { readonly kind: "pending" }
  | {
      readonly kind: "locked";
      readonly reason: Customer360MutationLockReason;
      readonly idempotencyKey: string;
    };

export interface Customer360WorkbenchProps {
  readonly customerID: number;
  readonly principal?: AuthPrincipal;
  readonly customerTransport?: CustomerDetailTransport;
  readonly stageTransport?: StageTransport;
  readonly contextTransport?: CustomerContextTransport;
  readonly mergeHistoryTransport?: CustomerMergeHistoryTransport;
  readonly chatActivityTransport?: CustomerChatActivityTransport;
  readonly activityAnalyticsTransport?: CustomerActivityAnalyticsTransport;
  readonly readCookie?: () => string;
  readonly randomUUIDSource?: { readonly randomUUID: () => string };
  readonly onUnauthenticated?: () => void;
  readonly initialCoreSnapshot?: CustomerDetailSnapshot;
  readonly initialStages?: readonly StageRecord[];
}

export interface Customer360AuxiliaryPanelsProps {
  readonly contextPanel: React.ReactNode;
  readonly chatPanel: React.ReactNode;
  readonly analyticsPanel: React.ReactNode;
  readonly mergeHistoryPanel: React.ReactNode;
}

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

function profileDraft(customer: CustomerProfile): ProfileDraft {
  return {
    name: customer.name,
    avatarURL: customer.avatarURL ?? "",
    gender: customer.gender === undefined ? "" : String(customer.gender),
    ownerStaffID:
      customer.ownerStaffID === undefined ? "" : String(customer.ownerStaffID),
    channelID:
      customer.channelID === undefined ? "" : String(customer.channelID),
  };
}

function emptyProfileDraft(): ProfileDraft {
  return {
    name: "",
    avatarURL: "",
    gender: "",
    ownerStaffID: "",
    channelID: "",
  };
}

function formatDateTime(value: string | undefined): string {
  if (!value) return "未记录";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? "未记录"
    : parsed.toLocaleString("zh-CN", { hour12: false });
}

function tagLabel(tag: CustomerTag): string {
  return tag.groupName ? `${tag.groupName} / ${tag.name}` : tag.name;
}

function currentValue<T>(state: Customer360PanelState<T>): T | undefined {
  if (state.kind === "ready") return state.value;
  return state.previous;
}

function coreFailureMessage(state: Extract<CoreState, { kind: "error" }>): string {
  switch (state.failure) {
    case "unauthenticated":
      return "登录状态已失效，客户事实读取已停止。";
    case "forbidden":
      return "当前账号无权读取该客户。";
    case "not_found":
      return "客户不存在，可能已删除或已超出当前可见范围。";
    case "invalid":
      return "客户响应不符合安全合同，已拒绝展示。";
    default:
      return state.previous
        ? "最新客户事实读取失败，继续显示上次已验证结果。"
        : "客户本地事实暂不可用。";
  }
}

function stageFailureMessage(state: Extract<StageState, { kind: "error" }>): string {
  return state.previous
    ? "阶段目录刷新失败，继续显示上次已验证目录。"
    : state.failure === "unauthenticated"
      ? "登录状态已失效，阶段目录读取已停止。"
      : "阶段目录暂不可用；为安全起见已禁用阶段修改。";
}

function mutationFailureMessage(result: Customer360MutationResult): string {
  if (result.status === "rejected") {
    switch (result.failure) {
      case "unauthenticated":
        return "登录状态已失效，未继续客户写入。";
      case "forbidden":
        return "当前账号无权修改该客户，本次请求已被拒绝。";
      case "not_found":
        return "客户或标签已不存在，请重新读取后确认。";
      case "invalid":
        return "提交内容或安全参数无效，未完成客户写入。";
      default:
        return "写前本地事实核验失败，未发送修改请求。";
    }
  }
  if (result.status === "conflict") {
    return result.reason === "expected_version_changed"
      ? "客户版本已变化，本次写入未发送。动作已锁定，请重新读取后再决定。"
      : "客户并发状态发生冲突。动作已锁定，绝不自动重试。";
  }
  if (result.status === "outcome_unknown") {
    return "请求结果未知，无法确认本地事实是否已改变。动作已锁定，绝不自动重试；请先重新读取。";
  }
  return "客户本地事实已写入，并完成严格回读核验。";
}

function mutationSuccessMessage(action: Customer360MutationAction): string {
  switch (action.kind) {
    case "profile":
      return "客户资料已写入本地 CRM，并完成严格回读核验。";
    case "stage":
      return "客户阶段已写入本地 CRM，并完成严格回读核验。";
    case "tag-add":
      return "客户标签已添加到本地 CRM，并完成严格回读核验。";
    case "tag-remove":
      return "客户标签已从本地 CRM 移除，并完成严格回读核验。";
  }
}

export function Customer360AuxiliaryPanels({
  contextPanel,
  chatPanel,
  analyticsPanel,
  mergeHistoryPanel,
}: Customer360AuxiliaryPanelsProps): React.ReactElement {
  return (
    <div className="customer-360-workbench__auxiliary-grid">
      <div id="customer-360-context-panel">{contextPanel}</div>
      <div id="customer-360-chat-panel">{chatPanel}</div>
      <div id="customer-360-analytics-panel">{analyticsPanel}</div>
      <div id="customer-360-merge-panel">{mergeHistoryPanel}</div>
    </div>
  );
}

export function Customer360MutationLockBanner({
  state,
  onRefresh,
}: {
  readonly state: Extract<MutationUIState, { kind: "locked" }>;
  readonly onRefresh: () => void;
}): React.ReactElement {
  return (
    <section
      className="customer-360-workbench__lock"
      role="alert"
      aria-labelledby="customer-360-lock-title"
    >
      <h2 id="customer-360-lock-title">
        {state.reason === "outcome_unknown" ? "写入结果未知" : "并发冲突"}
      </h2>
      <p>
        当前客户的写操作已锁定。系统不会自动重试，也不会以界面乐观状态代替本地事实。
      </p>
      <p className="customer-360-workbench__technical-fact">
        本次幂等键：<code>{state.idempotencyKey}</code>
      </p>
      <button type="button" onClick={onRefresh}>
        重新读取本地事实并解锁
      </button>
    </section>
  );
}

function Customer360AccessState({
  kind,
}: {
  readonly kind: "unauthenticated" | "forbidden" | "invalid_customer";
}): React.ReactElement {
  const message =
    kind === "unauthenticated"
      ? "登录状态不可用，未发送任何客户请求。"
      : kind === "invalid_customer"
        ? "客户编号无效，未发送任何客户请求。"
        : "当前账号不是 admin/ops，客户 360 工作台已在请求前关闭，未发送任何客户请求。";
  return (
    <main className="customer-360-workbench" aria-labelledby="app-title">
      <h1 id="app-title">客户 360 一体化运营工作台</h1>
      <section className="customer-360-workbench__access-state" role="alert">
        <h2>无权限访问</h2>
        <p>{message}</p>
        <a href="/admin/customers">返回客户列表</a>
      </section>
    </main>
  );
}

function CoreWorkbenchPanel({
  customerID,
  state,
  stagesState,
  profile,
  stageValue,
  selectedTagID,
  mutationState,
  timelinePending,
  onProfileChange,
  onStageChange,
  onSelectedTagChange,
  onProfileSubmit,
  onStageSubmit,
  onTagAdd,
  onTagRemove,
  onTimelineMore,
  onRefresh,
}: {
  readonly customerID: number;
  readonly state: CoreState;
  readonly stagesState: StageState;
  readonly profile: ProfileDraft;
  readonly stageValue: string;
  readonly selectedTagID: string;
  readonly mutationState: MutationUIState;
  readonly timelinePending: boolean;
  // eslint-disable-next-line no-unused-vars -- named parameter documents the component callback contract.
  readonly onProfileChange: (next: ProfileDraft) => void;
  // eslint-disable-next-line no-unused-vars -- named parameter documents the component callback contract.
  readonly onStageChange: (next: string) => void;
  // eslint-disable-next-line no-unused-vars -- named parameter documents the component callback contract.
  readonly onSelectedTagChange: (next: string) => void;
  readonly onProfileSubmit: React.FormEventHandler<HTMLFormElement>;
  readonly onStageSubmit: React.FormEventHandler<HTMLFormElement>;
  readonly onTagAdd: React.FormEventHandler<HTMLFormElement>;
  // eslint-disable-next-line no-unused-vars -- named parameter documents the component callback contract.
  readonly onTagRemove: (tagID: number) => void;
  readonly onTimelineMore: () => void;
  readonly onRefresh: () => void;
}): React.ReactElement {
  const candidateSnapshot = currentValue(state);
  const snapshot =
    candidateSnapshot?.customer.id === customerID
      ? candidateSnapshot
      : undefined;
  const stages = currentValue(stagesState) ?? [];
  const mutationDisabled = mutationState.kind !== "idle";

  if (!snapshot) {
    if (state.kind !== "error") {
      return (
        <section className="customer-360-workbench__panel" role="status">
          <h2>客户基本资料、阶段、标签与时间线</h2>
          <p>正在读取本地客户事实…</p>
        </section>
      );
    }
    return (
      <section className="customer-360-workbench__panel" role="alert">
        <h2>客户基本资料、阶段、标签与时间线</h2>
        <p>{coreFailureMessage(state)}</p>
        <button type="button" onClick={onRefresh}>
          重试本地读取
        </button>
      </section>
    );
  }

  const { customer, tags, tagCatalog, events } = snapshot;
  const attachedTagIDs = new Set(tags.map((tag) => tag.id));
  const availableTags = tagCatalog.filter((tag) => !attachedTagIDs.has(tag.id));
  const currentStageMissing =
    customer.stageID !== undefined &&
    !stages.some((stage) => stage.id === customer.stageID);
  const canChangeStage =
    !mutationDisabled &&
    (stagesState.kind === "ready" || stagesState.previous !== undefined);

  return (
    <section
      id="customer-360-core-panel"
      className="customer-360-workbench__panel"
      aria-labelledby="customer-360-core-title"
    >
      <div className="customer-360-workbench__panel-heading">
        <div>
          <h2 id="customer-360-core-title">
            客户基本资料、阶段、标签与时间线
          </h2>
          <p>
            {customer.name} · OneID #{customer.id} · 版本 {customer.updatedAt}
          </p>
        </div>
        <button
          className="button-secondary"
          disabled={mutationState.kind === "pending"}
          type="button"
          onClick={onRefresh}
        >
          刷新本地事实
        </button>
      </div>

      {state.kind === "loading" && (
        <p className="customer-360-workbench__stale" role="status">
          正在刷新；当前展示的是上次已验证本地事实。
        </p>
      )}
      {state.kind === "error" && (
        <p className="customer-360-workbench__stale" role="alert">
          {coreFailureMessage(state)}
        </p>
      )}

      <div className="customer-360-workbench__core-grid">
        <form
          className="customer-360-workbench__card"
          onSubmit={onProfileSubmit}
        >
          <fieldset disabled={mutationDisabled || state.kind !== "ready"}>
            <legend>基本资料</legend>
            <label htmlFor="customer-360-name">名称</label>
            <input
              id="customer-360-name"
              name="customer-360-name"
              value={profile.name}
              onChange={(event) =>
                onProfileChange({
                  ...profile,
                  name: event.currentTarget.value,
                })
              }
            />
            <label htmlFor="customer-360-avatar">头像地址</label>
            <input
              id="customer-360-avatar"
              name="customer-360-avatar"
              value={profile.avatarURL}
              onChange={(event) =>
                onProfileChange({
                  ...profile,
                  avatarURL: event.currentTarget.value,
                })
              }
            />
            <label htmlFor="customer-360-gender">性别编号</label>
            <input
              id="customer-360-gender"
              inputMode="numeric"
              name="customer-360-gender"
              value={profile.gender}
              onChange={(event) =>
                onProfileChange({
                  ...profile,
                  gender: event.currentTarget.value,
                })
              }
            />
            <label htmlFor="customer-360-owner">负责人编号</label>
            <input
              id="customer-360-owner"
              inputMode="numeric"
              name="customer-360-owner"
              value={profile.ownerStaffID}
              onChange={(event) =>
                onProfileChange({
                  ...profile,
                  ownerStaffID: event.currentTarget.value,
                })
              }
            />
            <label htmlFor="customer-360-channel">渠道编号</label>
            <input
              id="customer-360-channel"
              inputMode="numeric"
              name="customer-360-channel"
              value={profile.channelID}
              onChange={(event) =>
                onProfileChange({
                  ...profile,
                  channelID: event.currentTarget.value,
                })
              }
            />
            <button type="submit">
              {mutationState.kind === "pending" ? "正在核验…" : "保存资料"}
            </button>
          </fieldset>
        </form>

        <form
          className="customer-360-workbench__card"
          onSubmit={onStageSubmit}
        >
          <fieldset disabled={!canChangeStage || state.kind !== "ready"}>
            <legend>客户阶段</legend>
            <p className="customer-detail-page__meta">
              当前阶段：{stageName(stages, customer.stageID)}
            </p>
            {stagesState.kind === "loading" && (
              <p role="status">正在刷新阶段目录…</p>
            )}
            {stagesState.kind === "error" && (
              <p role="alert">{stageFailureMessage(stagesState)}</p>
            )}
            <label htmlFor="customer-360-stage">选择阶段</label>
            <select
              id="customer-360-stage"
              name="customer-360-stage"
              value={stageValue}
              onChange={(event) => onStageChange(event.currentTarget.value)}
            >
              <option value="">清除阶段</option>
              {currentStageMissing && customer.stageID !== undefined && (
                <option value={customer.stageID}>
                  已归档或不可见阶段 #{customer.stageID}
                </option>
              )}
              {stages.map((stage) => (
                <option key={stage.id} value={stage.id}>
                  {stage.name}
                </option>
              ))}
            </select>
            <button type="submit">
              {mutationState.kind === "pending" ? "正在核验…" : "保存阶段"}
            </button>
          </fieldset>
        </form>

        <section
          className="customer-360-workbench__card"
          aria-labelledby="customer-360-tags-title"
        >
          <h3 id="customer-360-tags-title">客户标签</h3>
          {tags.length === 0 ? (
            <p role="status">暂无本地标签。</p>
          ) : (
            <ul className="customer-detail-page__tag-list">
              {tags.map((tag) => (
                <li key={tag.id}>
                  <span>{tagLabel(tag)}</span>
                  <button
                    disabled={mutationDisabled || state.kind !== "ready"}
                    type="button"
                    onClick={() => onTagRemove(tag.id)}
                  >
                    移除
                  </button>
                </li>
              ))}
            </ul>
          )}
          <form onSubmit={onTagAdd}>
            <fieldset
              disabled={
                mutationDisabled ||
                state.kind !== "ready" ||
                availableTags.length === 0
              }
            >
              <legend>添加标签</legend>
              <label htmlFor="customer-360-tag">可添加标签</label>
              <select
                id="customer-360-tag"
                name="customer-360-tag"
                value={selectedTagID}
                onChange={(event) =>
                  onSelectedTagChange(event.currentTarget.value)
                }
              >
                <option value="">请选择</option>
                {availableTags.map((tag) => (
                  <option key={tag.id} value={tag.id}>
                    {tagLabel(tag)}
                  </option>
                ))}
              </select>
              <button type="submit">
                {mutationState.kind === "pending" ? "正在核验…" : "添加标签"}
              </button>
            </fieldset>
          </form>
        </section>
      </div>

      <section
        className="customer-360-workbench__card customer-360-workbench__timeline"
        aria-labelledby="customer-360-timeline-title"
      >
        <h3 id="customer-360-timeline-title">客户时间线</h3>
        {events.length === 0 ? (
          <p role="status">暂无本地时间线记录。</p>
        ) : (
          <ol className="customer-detail-page__timeline">
            {events.map((event) => (
              <li key={event.id}>
                <strong>{event.eventType}</strong>
                <span>执行者：{event.actor}</span>
                <time dateTime={event.occurredAt}>
                  {formatDateTime(event.occurredAt)}
                </time>
              </li>
            ))}
          </ol>
        )}
        {snapshot.eventsHaveMore && snapshot.eventsNextCursor && (
          <button
            type="button"
            disabled={
              mutationDisabled || state.kind !== "ready" || timelinePending
            }
            onClick={onTimelineMore}
          >
            {timelinePending ? "正在加载…" : "加载更多时间线"}
          </button>
        )}
      </section>

      <dl className="customer-detail-page__facts">
        <div>
          <dt>加入时间</dt>
          <dd>{formatDateTime(customer.addedAt)}</dd>
        </div>
        <div>
          <dt>最近互动</dt>
          <dd>{formatDateTime(customer.lastInteractAt)}</dd>
        </div>
        <div>
          <dt>创建时间</dt>
          <dd>{formatDateTime(customer.createdAt)}</dd>
        </div>
        <div>
          <dt>更新时间 / expected_version</dt>
          <dd>{formatDateTime(customer.updatedAt)}</dd>
        </div>
        <div>
          <dt>记录状态</dt>
          <dd>{customer.isDeleted ? "已删除" : "有效"}</dd>
        </div>
      </dl>
    </section>
  );
}

function AuthorizedCustomer360Workbench({
  customerID,
  role,
  customerTransport,
  stageTransport,
  contextTransport,
  mergeHistoryTransport,
  chatActivityTransport,
  activityAnalyticsTransport,
  readCookie,
  randomUUIDSource,
  onUnauthenticated,
  initialCoreSnapshot,
  initialStages,
}: Omit<Customer360WorkbenchProps, "principal"> & {
  readonly role: Customer360WorkbenchRole;
  readonly customerTransport: CustomerDetailTransport;
  readonly stageTransport: StageTransport;
  readonly contextTransport: CustomerContextTransport;
  readonly mergeHistoryTransport: CustomerMergeHistoryTransport;
  readonly chatActivityTransport: CustomerChatActivityTransport;
  readonly activityAnalyticsTransport: CustomerActivityAnalyticsTransport;
  readonly readCookie: () => string;
}): React.ReactElement {
  const validInitialCore =
    initialCoreSnapshot?.customer.id === customerID
      ? initialCoreSnapshot
      : undefined;
  const [coreState, setCoreState] = useState<CoreState>(() =>
    validInitialCore
      ? { kind: "ready", value: validInitialCore }
      : { kind: "loading" },
  );
  const [stagesState, setStagesState] = useState<StageState>(() =>
    initialStages
      ? { kind: "ready", value: initialStages }
      : { kind: "loading" },
  );
  const [profile, setProfile] = useState<ProfileDraft>(() =>
    validInitialCore
      ? profileDraft(validInitialCore.customer)
      : emptyProfileDraft(),
  );
  const [stageValue, setStageValue] = useState(() =>
    validInitialCore?.customer.stageID === undefined
      ? ""
      : String(validInitialCore.customer.stageID),
  );
  const [selectedTagID, setSelectedTagID] = useState("");
  const [notice, setNotice] = useState<string>();
  const [timelinePending, setTimelinePending] = useState(false);
  const [mutationState, setMutationState] = useState<MutationUIState>({
    kind: "idle",
  });
  const coreVerified = useRef<CustomerDetailSnapshot | undefined>(
    validInitialCore,
  );
  const stagesVerified = useRef<readonly StageRecord[] | undefined>(
    initialStages,
  );
  const coreGate = useRef(new Customer360LatestRequestGate());
  const stagesGate = useRef(new Customer360LatestRequestGate());
  const timelineGate = useRef(new Customer360LatestRequestGate());
  const mutationGuard = useRef(new Customer360MutationGuard());
  const mutationGeneration = useRef(0);

  const applyCoreSnapshot = useCallback((snapshot: CustomerDetailSnapshot) => {
    coreVerified.current = snapshot;
    setCoreState({ kind: "ready", value: snapshot });
    setProfile(profileDraft(snapshot.customer));
    setStageValue(
      snapshot.customer.stageID === undefined
        ? ""
        : String(snapshot.customer.stageID),
    );
    setSelectedTagID("");
  }, []);

  const refreshCore = useCallback(
    async (preservePrevious = true) => {
      timelineGate.current.invalidate();
      setTimelinePending(false);
      const token = coreGate.current.begin(customerID);
      if (!token) return undefined;
      const previous =
        preservePrevious && coreVerified.current?.customer.id === customerID
          ? coreVerified.current
          : undefined;
      setCoreState({
        kind: "loading",
        ...(previous ? { previous } : {}),
      });
      const result = await loadCustomer360Core(customerTransport, customerID);
      if (!coreGate.current.isCurrent(token)) return undefined;
      if (result.status === "loaded") {
        applyCoreSnapshot(result.snapshot);
        return result;
      }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setCoreState({
        kind: "error",
        failure: result.status,
        ...(previous ? { previous } : {}),
      });
      return result;
    },
    [
      applyCoreSnapshot,
      customerID,
      customerTransport,
      onUnauthenticated,
    ],
  );

  const refreshStages = useCallback(
    async (preservePrevious = true) => {
      const token = stagesGate.current.begin(customerID);
      if (!token) return undefined;
      const previous = preservePrevious ? stagesVerified.current : undefined;
      setStagesState({
        kind: "loading",
        ...(previous ? { previous } : {}),
      });
      const result = await loadCustomer360StageCatalog(stageTransport);
      if (!stagesGate.current.isCurrent(token)) return undefined;
      if (result.status === "loaded") {
        stagesVerified.current = result.items;
        setStagesState({ kind: "ready", value: result.items });
        return result;
      }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setStagesState({
        kind: "error",
        failure: result.status,
        ...(previous ? { previous } : {}),
      });
      return result;
    },
    [customerID, onUnauthenticated, stageTransport],
  );

  useEffect(() => {
    mutationGeneration.current += 1;
    timelineGate.current.invalidate();
    setTimelinePending(false);
    mutationGuard.current.reset();
    setMutationState({ kind: "idle" });
    setNotice(undefined);

    const nextInitial =
      initialCoreSnapshot?.customer.id === customerID
        ? initialCoreSnapshot
        : undefined;
    coreVerified.current = nextInitial;
    stagesVerified.current = initialStages;
    if (nextInitial) {
      applyCoreSnapshot(nextInitial);
    } else {
      setCoreState({ kind: "loading" });
      setProfile(emptyProfileDraft());
      setStageValue("");
      setSelectedTagID("");
    }
    setStagesState(
      initialStages
        ? { kind: "ready", value: initialStages }
        : { kind: "loading" },
    );

    void refreshCore(Boolean(nextInitial));
    void refreshStages(Boolean(initialStages));
    return () => {
      mutationGeneration.current += 1;
      coreGate.current.invalidate();
      stagesGate.current.invalidate();
      timelineGate.current.invalidate();
    };
  }, [
    applyCoreSnapshot,
    customerID,
    initialCoreSnapshot,
    initialStages,
    refreshCore,
    refreshStages,
  ]);

  const unlockAfterRefresh = useCallback(async () => {
    setNotice("正在重新读取本地事实；锁定期间不会发送写请求。");
    const result = await refreshCore(true);
    if (result?.status !== "loaded") {
      setNotice("本地事实仍未确认，写操作继续锁定。");
      return;
    }
    mutationGuard.current.reset();
    setMutationState({ kind: "idle" });
    setNotice("已重新读取本地事实，写操作已解锁。请重新确认后提交。");
  }, [refreshCore]);

  const runMutation = useCallback(
    async (action: Customer360MutationAction) => {
      const start = mutationGuard.current.begin();
      if (start === "busy") return;
      if (start === "locked") {
        setNotice("当前动作已锁定，请先重新读取本地事实。");
        return;
      }
      if (
        coreState.kind !== "ready" ||
        coreState.value.customer.id !== customerID
      ) {
        mutationGuard.current.finishKnown();
        setNotice("当前不是最新已验证客户事实，未发送写请求。");
        return;
      }

      let csrfToken: string | undefined;
      try {
        csrfToken = readCSRFCookie(readCookie());
      } catch {
        csrfToken = undefined;
      }
      const idempotencyKey = newCustomer360IdempotencyKey(
        action.kind,
        randomUUIDSource,
      );
      if (!csrfToken || !idempotencyKey) {
        mutationGuard.current.finishKnown();
        setNotice("安全令牌或幂等键不可用，未发送写请求。");
        return;
      }

      timelineGate.current.invalidate();
      setTimelinePending(false);
      const operationGeneration = mutationGeneration.current;
      const operationCustomerID = customerID;
      const previousSnapshot = coreState.value;
      setMutationState({ kind: "pending" });
      setNotice("正在核验 expected_version 并提交一次本地写入…");

      const result = await executeCustomer360Mutation(
        customerTransport,
        operationCustomerID,
        previousSnapshot.customer.updatedAt,
        action,
        csrfToken,
        idempotencyKey,
      );
      if (
        mutationGeneration.current !== operationGeneration ||
        operationCustomerID !== customerID
      ) {
        return;
      }

      if (result.status === "confirmed") {
        mutationGuard.current.finishKnown();
        setMutationState({ kind: "idle" });
        const merged = mergeConfirmedCustomer360Core(
          previousSnapshot,
          result.core,
        );
        if (!merged) {
          mutationGuard.current.lock("outcome_unknown");
          setMutationState({
            kind: "locked",
            reason: "outcome_unknown",
            idempotencyKey: result.idempotencyKey,
          });
          setNotice("写后回读客户编号不一致，结果未知，动作已锁定。");
          return;
        }
        applyCoreSnapshot(merged);
        setNotice(mutationSuccessMessage(action));
        void refreshCore(true);
        return;
      }

      setNotice(mutationFailureMessage(result));
      if (
        result.status === "rejected" &&
        result.failure === "unauthenticated"
      ) {
        onUnauthenticated?.();
      }
      if (result.status === "conflict") {
        mutationGuard.current.lock("conflict");
        if (result.core) {
          const merged = mergeConfirmedCustomer360Core(
            previousSnapshot,
            result.core,
          );
          if (merged) applyCoreSnapshot(merged);
        }
        setMutationState({
          kind: "locked",
          reason: "conflict",
          idempotencyKey: result.idempotencyKey,
        });
        return;
      }
      if (result.status === "outcome_unknown") {
        mutationGuard.current.lock("outcome_unknown");
        setMutationState({
          kind: "locked",
          reason: "outcome_unknown",
          idempotencyKey: result.idempotencyKey,
        });
        return;
      }
      mutationGuard.current.finishKnown();
      setMutationState({ kind: "idle" });
    },
    [
      applyCoreSnapshot,
      coreState,
      customerID,
      customerTransport,
      onUnauthenticated,
      randomUUIDSource,
      readCookie,
      refreshCore,
    ],
  );

  const submitProfile: React.FormEventHandler<HTMLFormElement> = (event) => {
    event.preventDefault();
    const update = parseProfileDraft(profile);
    if (!update) {
      setNotice("资料格式不正确，未发送写请求。");
      return;
    }
    void runMutation({ kind: "profile", update });
  };

  const submitStage: React.FormEventHandler<HTMLFormElement> = (event) => {
    event.preventDefault();
    const stageID = stageValue === "" ? null : Number(stageValue);
    if (
      stageID !== null &&
      (!Number.isSafeInteger(stageID) || stageID <= 0)
    ) {
      setNotice("阶段必须来自已验证目录，未发送写请求。");
      return;
    }
    const stages = currentValue(stagesState) ?? [];
    const currentID =
      coreState.kind === "ready" ? coreState.value.customer.stageID : undefined;
    if (
      stageID !== null &&
      stageID !== currentID &&
      !stages.some((stage) => stage.id === stageID)
    ) {
      setNotice("阶段不在已验证目录中，未发送写请求。");
      return;
    }
    void runMutation({ kind: "stage", stageID });
  };

  const submitTagAdd: React.FormEventHandler<HTMLFormElement> = (event) => {
    event.preventDefault();
    const tagID = Number(selectedTagID);
    const snapshot =
      coreState.kind === "ready" &&
      coreState.value.customer.id === customerID
        ? coreState.value
        : undefined;
    if (
      !snapshot ||
      !Number.isSafeInteger(tagID) ||
      tagID <= 0 ||
      !snapshot.tagCatalog.some((tag) => tag.id === tagID) ||
      snapshot.tags.some((tag) => tag.id === tagID)
    ) {
      setNotice("标签不在可添加目录中，未发送写请求。");
      return;
    }
    void runMutation({ kind: "tag-add", tagID });
  };

  const loadMoreTimeline = useCallback(async () => {
    if (
      coreState.kind !== "ready" ||
      coreState.value.customer.id !== customerID ||
      mutationState.kind !== "idle" ||
      timelinePending
    ) {
      return;
    }
    const cursor = coreState.value.eventsNextCursor;
    if (!cursor) return;
    const token = timelineGate.current.begin(customerID);
    if (!token) return;
    setTimelinePending(true);
    setNotice(undefined);
    try {
      const result = await loadCustomerTimelinePage(
        customerTransport,
        customerID,
        cursor,
        new Set(coreState.value.events.map((event) => event.id)),
      );
      if (!timelineGate.current.isCurrent(token)) return;
      if (result.status !== "loaded") {
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setNotice(
          result.status === "unauthenticated"
            ? "登录状态已失效，请重新登录。"
            : result.status === "forbidden"
              ? "当前账号无权继续读取客户时间线。"
              : result.status === "not_found"
                ? "客户已不可见，已显示记录保持不变。"
                : "时间线分页暂不可用，已显示记录保持不变。",
        );
        return;
      }
      setCoreState((current) => {
        if (
          current.kind !== "ready" ||
          current.value.customer.id !== customerID ||
          current.value.eventsNextCursor !== cursor
        ) {
          return current;
        }
        const next: CustomerDetailSnapshot = {
          ...current.value,
          events: [...current.value.events, ...result.events],
          eventsHaveMore: result.nextCursor !== undefined,
          eventsNextCursor: result.nextCursor,
        };
        coreVerified.current = next;
        return { kind: "ready", value: next };
      });
    } finally {
      if (timelineGate.current.isCurrent(token)) setTimelinePending(false);
    }
  }, [
    coreState,
    customerID,
    customerTransport,
    mutationState.kind,
    onUnauthenticated,
    timelinePending,
  ]);

  const coreSnapshotCandidate = currentValue(coreState);
  const coreSnapshot =
    coreSnapshotCandidate?.customer.id === customerID
      ? coreSnapshotCandidate
      : undefined;

  return (
    <main className="customer-360-workbench" aria-labelledby="app-title">
      <header className="customer-360-workbench__header">
        <div>
          <p className="route-card__eyebrow">Lane A · 本地客户运营</p>
          <h1 id="app-title">客户 360 一体化运营工作台</h1>
          <p>
            {coreSnapshot
              ? `${coreSnapshot.customer.name} · OneID #${coreSnapshot.customer.id}`
              : `OneID #${customerID}`}
          </p>
        </div>
        <nav aria-label="客户 360 工作台导航">
          <a href="/admin/customers">返回客户列表</a>
          <a href="#customer-360-core-panel">资料与时间线</a>
          <a href="#customer-360-context-panel">客户上下文</a>
          <a href="#customer-360-chat-panel">聊天摘要</a>
          <a href="#customer-360-analytics-panel">活动分析</a>
          <a href="#customer-360-merge-panel">OneID 合并历史</a>
        </nav>
      </header>

      <section className="customer-360-workbench__boundary" role="note">
        <strong>安全边界：</strong>
        这里只读取或修改本地 CRM 客户事实；不会调用企微 Provider、群发、支付或退款。
        本地保存成功不等于 Provider 同步、发送或处理成功。
      </section>

      {notice && (
        <p className="customer-360-workbench__notice" role="alert">
          {notice}
        </p>
      )}

      {mutationState.kind === "locked" && (
        <Customer360MutationLockBanner
          state={mutationState}
          onRefresh={() => void unlockAfterRefresh()}
        />
      )}

      <CoreWorkbenchPanel
        customerID={customerID}
        state={coreState}
        stagesState={stagesState}
        profile={profile}
        stageValue={stageValue}
        selectedTagID={selectedTagID}
        mutationState={mutationState}
        timelinePending={timelinePending}
        onProfileChange={setProfile}
        onStageChange={setStageValue}
        onSelectedTagChange={setSelectedTagID}
        onProfileSubmit={submitProfile}
        onStageSubmit={submitStage}
        onTagAdd={submitTagAdd}
        onTagRemove={(tagID) => void runMutation({ kind: "tag-remove", tagID })}
        onTimelineMore={() => void loadMoreTimeline()}
        onRefresh={() => void refreshCore(true)}
      />

      <Customer360AuxiliaryPanels
        contextPanel={
          <CustomerContextPanel
            key={`context-${customerID}`}
            customerID={customerID}
            transport={contextTransport}
            onUnauthenticated={onUnauthenticated}
            showChatSummary={false}
          />
        }
        chatPanel={
          <CustomerChatActivityPanel
            key={`chat-${customerID}`}
            customerID={customerID}
            role={role}
            transport={chatActivityTransport}
            onUnauthenticated={onUnauthenticated}
          />
        }
        analyticsPanel={
          <CustomerActivityAnalyticsPanel
            key={`analytics-${customerID}`}
            customerID={customerID}
            transport={activityAnalyticsTransport}
            onUnauthenticated={onUnauthenticated}
          />
        }
        mergeHistoryPanel={
          <CustomerMergeHistoryPanel
            key={`merge-${customerID}`}
            customerID={customerID}
            role={role}
            transport={mergeHistoryTransport}
            onUnauthenticated={onUnauthenticated}
          />
        }
      />
    </main>
  );
}

export function Customer360Workbench({
  customerID,
  principal,
  customerTransport = generatedCustomerDetailTransport,
  stageTransport = generatedStageTransport,
  contextTransport = generatedCustomerContextTransport,
  mergeHistoryTransport = generatedCustomerMergeHistoryTransport,
  chatActivityTransport = generatedCustomerChatActivityTransport,
  activityAnalyticsTransport = generatedCustomerActivityAnalyticsTransport,
  readCookie = browserCookie,
  randomUUIDSource,
  onUnauthenticated,
  initialCoreSnapshot,
  initialStages,
}: Customer360WorkbenchProps): React.ReactElement {
  if (!validCustomer360CustomerID(customerID)) {
    return <Customer360AccessState kind="invalid_customer" />;
  }
  const access = resolveCustomer360Access(principal);
  if (access.status !== "allowed") {
    return <Customer360AccessState kind={access.status} />;
  }
  return (
    <AuthorizedCustomer360Workbench
      key={customerID}
      customerID={customerID}
      role={access.principal.role}
      customerTransport={customerTransport}
      stageTransport={stageTransport}
      contextTransport={contextTransport}
      mergeHistoryTransport={mergeHistoryTransport}
      chatActivityTransport={chatActivityTransport}
      activityAnalyticsTransport={activityAnalyticsTransport}
      readCookie={readCookie}
      randomUUIDSource={randomUUIDSource}
      onUnauthenticated={onUnauthenticated}
      initialCoreSnapshot={initialCoreSnapshot}
      initialStages={initialStages}
    />
  );
}
