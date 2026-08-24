import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import { CustomerContextPanel } from "./customer-context-ui";
import type { CustomerContextTransport } from "./customer-context";
import { CustomerMergeHistoryPanel } from "./customer-merge-history-ui";
import type {
  CustomerMergeHistoryRole,
  CustomerMergeHistoryTransport,
} from "./customer-merge-history";
import { CustomerChatActivityPanel } from "./customer-chat-activity-ui";
import type {
  CustomerChatActivityRole,
  CustomerChatActivityTransport,
} from "./customer-chat-activity";
import type { CustomerActivityAnalyticsTransport } from "./customer-activity-analytics";
import { campaignSourceHref } from "./cloud-orchestrator";
import type { CustomerRole } from "./customers";
import {
  generatedCustomerDetailTransport,
  loadCustomerContactPolicy,
  isCustomerGender,
  isSafeAvatarURL,
  loadCustomerDetail,
  loadCustomerTimelinePage,
  newCustomerContactPolicyIdempotencyKey,
  submitCustomerContactPolicyClear,
  submitCustomerContactPolicySet,
  submitCustomerProfileUpdate,
  submitCustomerStageChange,
  submitCustomerTagAdd,
  submitCustomerTagRemoval,
  type CustomerDetailLoadResult,
  type CustomerDetailSnapshot,
  type CustomerDetailTransport,
  type CustomerContactPolicy,
  type CustomerContactPolicyMutationResult,
  type CustomerContactPolicyReason,
  type CustomerMutationFailure,
  type CustomerMutationResult,
  type CustomerProfile,
  type CustomerProfileUpdate,
  type CustomerTag,
} from "./customer-detail";
import "./customer-detail.css";

export interface CustomerDetailPageProps {
  readonly customerID: number;
  readonly actorID?: number;
  readonly role?: CustomerRole;
  readonly transport?: CustomerDetailTransport;
  readonly contextTransport?: CustomerContextTransport;
  readonly mergeHistoryRole?: CustomerMergeHistoryRole;
  readonly mergeHistoryTransport?: CustomerMergeHistoryTransport;
  readonly chatActivityRole?: CustomerChatActivityRole;
  readonly chatActivityTransport?: CustomerChatActivityTransport;
  readonly activityAnalyticsTransport?: CustomerActivityAnalyticsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
  readonly initialSnapshot?: CustomerDetailSnapshot;
  readonly contactPolicyKeySource?: { readonly randomUUID: () => string };
}

type PageState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly snapshot: CustomerDetailSnapshot }
  | { readonly kind: "unauthenticated" }
  | { readonly kind: "forbidden" }
  | { readonly kind: "not_found" }
  | { readonly kind: "unavailable" };

interface ProfileDraft {
  readonly name: string;
  readonly avatarURL: string;
  readonly gender: string;
  readonly ownerStaffID: string;
  readonly channelID: string;
}

const mutationMessages: Record<CustomerMutationFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有执行该操作的权限。",
  not_found: "客户或标签已不存在，请刷新后重试。",
  conflict: "服务端状态已变化，请刷新后确认再提交。",
  invalid: "提交内容不符合要求，请检查后重试。",
  unavailable: "客户服务暂时不可用，操作未确认，请稍后重试。",
};

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

function draftFromCustomer(customer: CustomerProfile): ProfileDraft {
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

function parseNullableInteger(value: string): number | null | undefined {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  if (!/^-?\d+$/.test(trimmed)) return undefined;
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) ? parsed : undefined;
}

function parseNullablePositiveInteger(
  value: string,
): number | null | undefined {
  const parsed = parseNullableInteger(value);
  if (parsed === null) return null;
  return parsed !== undefined && parsed > 0 ? parsed : undefined;
}

export function parseProfileDraft(
  draft: ProfileDraft,
): CustomerProfileUpdate | undefined {
  const gender = parseNullableInteger(draft.gender);
  const ownerStaffID = parseNullablePositiveInteger(draft.ownerStaffID);
  const channelID = parseNullablePositiveInteger(draft.channelID);
  if (
    draft.name.trim() === "" ||
    gender === undefined ||
    ownerStaffID === undefined ||
    channelID === undefined ||
    (gender !== null && !isCustomerGender(gender))
  ) {
    return undefined;
  }
  const avatarURL = draft.avatarURL.trim();
  if (avatarURL !== "" && !isSafeAvatarURL(avatarURL)) return undefined;
  return {
    name: draft.name,
    avatarURL: avatarURL === "" ? null : avatarURL,
    gender,
    ownerStaffID,
    channelID,
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

export function CustomerDetailReadNavigation({
  includeCustomer360 = false,
}: {
  readonly includeCustomer360?: boolean;
}): React.ReactElement {
  return (
    <nav aria-label="客户读取导航">
      <a href="/customers">返回客户列表</a>
      {includeCustomer360 && (
        <a href="#customer-360-summary">查看 Customer 360 摘要</a>
      )}
    </nav>
  );
}

export function CustomerDetailMissingState(): React.ReactElement {
  return (
    <section className="customer-detail-page" aria-labelledby="app-title">
      <h1 id="app-title">客户详情</h1>
      <div className="customer-detail-page__state" role="alert">
        <p>客户不存在，可能已被删除或当前账号没有可见范围。</p>
        <CustomerDetailReadNavigation />
      </div>
    </section>
  );
}

function applyLoadResult(
  result: CustomerDetailLoadResult,
  setPage: React.Dispatch<React.SetStateAction<PageState>>,
  onUnauthenticated?: () => void,
): void {
  if (result.status === "loaded") {
    setPage({ kind: "ready", snapshot: result.snapshot });
    return;
  }
  if (result.status === "unauthenticated") onUnauthenticated?.();
  setPage({ kind: result.status });
}

export type CustomerMutationFlowResult =
  | { readonly status: "confirmed"; readonly snapshot: CustomerDetailSnapshot }
  | {
      readonly status: "mutation_failed";
      readonly failure: CustomerMutationFailure;
    }
  | {
      readonly status: "unconfirmed";
      readonly failure: Exclude<CustomerDetailLoadResult["status"], "loaded">;
    };

export function startCustomerMutation(
  lock: { current: boolean },
  execute: () => Promise<CustomerMutationResult>,
  refetch: () => Promise<CustomerDetailLoadResult>,
): Promise<CustomerMutationFlowResult> | undefined {
  if (lock.current) return undefined;
  lock.current = true;
  return (async () => {
    try {
      const result = await execute();
      if (result.status !== "succeeded") {
        return { status: "mutation_failed", failure: result.status };
      }
      const refreshed = await refetch();
      return refreshed.status === "loaded"
        ? { status: "confirmed", snapshot: refreshed.snapshot }
        : { status: "unconfirmed", failure: refreshed.status };
    } catch {
      return { status: "mutation_failed", failure: "unavailable" };
    } finally {
      lock.current = false;
    }
  })();
}

type ContactPolicyState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly policy: CustomerContactPolicy }
  | { readonly kind: "unauthenticated" }
  | { readonly kind: "forbidden" }
  | { readonly kind: "not_found" }
  | { readonly kind: "unavailable" };

type ContactPolicyIntent =
  | {
      readonly kind: "set";
      readonly key: string;
      readonly expectedVersion: number;
      readonly reasonCode: CustomerContactPolicyReason;
      readonly suppressedUntil: string | null;
    }
  | {
      readonly kind: "clear";
      readonly key: string;
      readonly expectedVersion: number;
    };

const contactPolicyReasonLabels: Record<CustomerContactPolicyReason, string> = {
  manual_opt_out: "客户主动拒绝",
  compliance_hold: "合规保留",
  operator_hold: "运营暂缓",
};

function contactPolicyMessage(
  result: CustomerContactPolicyMutationResult | undefined,
): string | undefined {
  if (!result) return undefined;
  switch (result.status) {
    case "succeeded":
      return "本地触达策略已保存，并未验证或调用 Provider，也不证明发送或送达。";
    case "unauthenticated":
      return "登录状态已失效，未确认本地策略写入。";
    case "forbidden":
      return "当前账号没有修改本地触达策略的权限。";
    case "not_found":
      return "无法确认本地策略事实，未继续操作。";
    case "conflict":
      return "策略版本已变化，已重新读取本地事实；请重新确认。";
    case "invalid":
      return "策略输入或服务响应不符合安全合同，未继续操作。";
    case "unavailable":
      return "本地策略服务暂不可用，未确认写入。";
    case "outcome_unknown":
      return "写入结果未知；只允许精确重放原本地策略意图。";
  }
}

function contactPolicyReadMessage(
  state: ContactPolicyState,
): string | undefined {
  switch (state.kind) {
    case "unauthenticated":
      return "登录状态已失效，无法读取本地策略。";
    case "forbidden":
      return "当前账号无权读取本地策略。";
    case "not_found":
      return "无法读取本地策略事实。";
    case "unavailable":
      return "本地策略响应不可用或不符合安全合同。";
    default:
      return undefined;
  }
}

function policySuppressedUntil(value: string): string | null | undefined {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const milliseconds = Date.parse(trimmed);
  return Number.isNaN(milliseconds)
    ? undefined
    : new Date(milliseconds).toISOString();
}

function ContactPolicyPanel({
  customerID,
  transport,
  readCookie,
  keySource,
  onUnauthenticated,
}: {
  readonly customerID: number;
  readonly transport: CustomerDetailTransport;
  readonly readCookie: () => string;
  readonly keySource?: { readonly randomUUID: () => string };
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const [state, setState] = useState<ContactPolicyState>({ kind: "loading" });
  const [reasonCode, setReasonCode] =
    useState<CustomerContactPolicyReason>("manual_opt_out");
  const [suppressedUntil, setSuppressedUntil] = useState("");
  const [clearConfirmed, setClearConfirmed] = useState(false);
  const [mutation, setMutation] =
    useState<CustomerContactPolicyMutationResult>();
  const [busy, setBusy] = useState(false);
  const requestGeneration = useRef(0);
  const mutationLock = useRef(false);
  const replay = useRef<ContactPolicyIntent>();

  const reload = useCallback(async () => {
    const generation = requestGeneration.current + 1;
    requestGeneration.current = generation;
    setState({ kind: "loading" });
    const result = await loadCustomerContactPolicy(transport, customerID);
    if (requestGeneration.current !== generation) return undefined;
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setState(
      result.status === "loaded"
        ? { kind: "ready", policy: result.policy }
        : { kind: result.status },
    );
    return result;
  }, [customerID, onUnauthenticated, transport]);

  useEffect(() => {
    void reload();
    return () => {
      requestGeneration.current += 1;
    };
  }, [reload]);

  const csrf = (): string | undefined => {
    try {
      return readCSRFCookie(readCookie());
    } catch {
      return undefined;
    }
  };

  const execute = async (intent: ContactPolicyIntent): Promise<void> => {
    if (mutationLock.current) return;
    const token = csrf();
    if (!token) {
      setMutation({ status: "invalid" });
      return;
    }
    const generation = requestGeneration.current;
    mutationLock.current = true;
    setBusy(true);
    setMutation(undefined);
    const result =
      intent.kind === "set"
        ? await submitCustomerContactPolicySet(
            transport,
            customerID,
            intent,
            token,
            intent.key,
          )
        : await submitCustomerContactPolicyClear(
            transport,
            customerID,
            intent.expectedVersion,
            token,
            intent.key,
          );
    if (requestGeneration.current !== generation) return;
    mutationLock.current = false;
    setBusy(false);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    if (result.status === "succeeded") {
      replay.current = undefined;
      setClearConfirmed(false);
      setState({ kind: "ready", policy: result.policy });
    } else if (result.status === "conflict") {
      replay.current = undefined;
      setClearConfirmed(false);
      setMutation(result);
      await reload();
      return;
    } else if (result.status === "outcome_unknown") {
      replay.current = intent;
    } else {
      replay.current = undefined;
    }
    setMutation(result);
  };

  const startSet = (): void => {
    if (state.kind !== "ready" || busy) return;
    const until = policySuppressedUntil(suppressedUntil);
    const key = newCustomerContactPolicyIdempotencyKey("set", keySource);
    if (until === undefined || !key) {
      setMutation({ status: "invalid" });
      return;
    }
    void execute({
      kind: "set",
      key,
      expectedVersion: state.policy.policyPresent ? state.policy.version : 0,
      reasonCode,
      suppressedUntil: until,
    });
  };

  const startClear = (): void => {
    if (
      state.kind !== "ready" ||
      busy ||
      !clearConfirmed ||
      !state.policy.policyPresent ||
      state.policy.version < 1
    ) {
      return;
    }
    const key = newCustomerContactPolicyIdempotencyKey("clear", keySource);
    if (!key) {
      setMutation({ status: "invalid" });
      return;
    }
    void execute({
      kind: "clear",
      key,
      expectedVersion: state.policy.version,
    });
  };

  const readMessage = contactPolicyReadMessage(state);
  const mutationMessage = contactPolicyMessage(mutation);
  return (
    <section
      className="customer-detail-page__card"
      aria-labelledby="customer-contact-policy-title"
    >
      <h2 id="customer-contact-policy-title">本地客户触达策略</h2>
      <p className="customer-detail-page__meta" role="note">
        仅管理本地触达策略；不会验证或调用 Provider，也不证明发送或送达。
      </p>
      {state.kind === "loading" && <p role="status">正在读取本地策略事实…</p>}
      {readMessage && <p role="alert">{readMessage}</p>}
      {state.kind === "ready" && (
        <>
          <dl
            className="customer-detail-page__meta"
            aria-label="本地触达策略事实"
          >
            <dt>策略记录</dt>
            <dd>{state.policy.policyPresent ? "已设置" : "未设置"}</dd>
            <dt>当前可触达</dt>
            <dd>{state.policy.eligible ? "是" : "否"}</dd>
            <dt>当前抑制</dt>
            <dd>{state.policy.suppressionActive ? "是" : "否"}</dd>
            <dt>原因</dt>
            <dd>
              {state.policy.reasonCode
                ? contactPolicyReasonLabels[state.policy.reasonCode]
                : "未设置"}
            </dd>
            <dt>截止时间</dt>
            <dd>{formatDateTime(state.policy.suppressedUntil ?? undefined)}</dd>
            <dt>版本</dt>
            <dd>{state.policy.version}</dd>
            <dt>本地限定</dt>
            <dd>{state.policy.localOnly ? "是" : "否"}</dd>
          </dl>
          <fieldset disabled={busy || replay.current !== undefined}>
            <legend>设置或替换本地策略</legend>
            <label htmlFor="customer-contact-policy-reason">策略原因</label>
            <select
              id="customer-contact-policy-reason"
              value={reasonCode}
              onChange={(event) =>
                setReasonCode(
                  event.currentTarget.value as CustomerContactPolicyReason,
                )
              }
            >
              {Object.entries(contactPolicyReasonLabels).map(
                ([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ),
              )}
            </select>
            <label htmlFor="customer-contact-policy-until">
              抑制截止时间（可选）
            </label>
            <input
              id="customer-contact-policy-until"
              type="datetime-local"
              value={suppressedUntil}
              onChange={(event) =>
                setSuppressedUntil(event.currentTarget.value)
              }
            />
            <button type="button" onClick={startSet}>
              {busy ? "正在保存…" : "保存本地策略"}
            </button>
          </fieldset>
          {state.policy.policyPresent && state.policy.version >= 1 && (
            <fieldset disabled={busy || replay.current !== undefined}>
              <legend>清除本地策略</legend>
              <label>
                <input
                  type="checkbox"
                  checked={clearConfirmed}
                  onChange={(event) =>
                    setClearConfirmed(event.currentTarget.checked)
                  }
                />
                我已确认清除当前本地策略
              </label>
              <button
                type="button"
                disabled={!clearConfirmed}
                onClick={startClear}
              >
                清除本地策略
              </button>
            </fieldset>
          )}
          <button type="button" disabled={busy} onClick={() => void reload()}>
            刷新本地策略
          </button>
        </>
      )}
      {replay.current && (
        <button
          type="button"
          disabled={busy}
          onClick={() => void execute(replay.current!)}
        >
          精确重放原策略意图
        </button>
      )}
      {mutationMessage && (
        <p role={mutation?.status === "succeeded" ? "status" : "alert"}>
          {mutationMessage}
        </p>
      )}
    </section>
  );
}

export function CustomerDetailPage({
  customerID,
  actorID,
  role,
  transport = generatedCustomerDetailTransport,
  contextTransport,
  mergeHistoryRole,
  mergeHistoryTransport,
  chatActivityRole,
  chatActivityTransport,
  activityAnalyticsTransport,
  readCookie = browserCookie,
  onUnauthenticated,
  initialSnapshot,
  contactPolicyKeySource,
}: CustomerDetailPageProps): React.ReactElement {
  const [page, setPage] = useState<PageState>(() =>
    initialSnapshot
      ? { kind: "ready", snapshot: initialSnapshot }
      : { kind: "loading" },
  );
  const [notice, setNotice] = useState<string>();
  const [profile, setProfile] = useState<ProfileDraft>(() =>
    initialSnapshot
      ? draftFromCustomer(initialSnapshot.customer)
      : {
          name: "",
          avatarURL: "",
          gender: "",
          ownerStaffID: "",
          channelID: "",
        },
  );
  const [stageValue, setStageValue] = useState("");
  const [selectedTagID, setSelectedTagID] = useState("");
  const [mutationPending, setMutationPending] = useState(false);
  const [timelinePending, setTimelinePending] = useState(false);
  const mutationInFlight = useRef(false);
  const loadSequence = useRef(0);
  const timelineLoadSequence = useRef(0);
  const latestCustomerID = useRef(customerID);
  latestCustomerID.current = customerID;

  const refresh = useCallback(async () => {
    timelineLoadSequence.current += 1;
    setTimelinePending(false);
    const sequence = loadSequence.current + 1;
    loadSequence.current = sequence;
    setPage({ kind: "loading" });
    setNotice(undefined);
    const result = await loadCustomerDetail(transport, customerID);
    if (sequence !== loadSequence.current) return;
    applyLoadResult(result, setPage, onUnauthenticated);
  }, [customerID, onUnauthenticated, transport]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const loadedCustomer =
    page.kind === "ready" ? page.snapshot.customer : undefined;
  useEffect(() => {
    if (!loadedCustomer) return;
    setProfile(draftFromCustomer(loadedCustomer));
    setStageValue(
      loadedCustomer.stageID === undefined
        ? ""
        : String(loadedCustomer.stageID),
    );
  }, [loadedCustomer]);

  const csrfToken = (): string | undefined => {
    try {
      return readCSRFCookie(readCookie());
    } catch {
      return undefined;
    }
  };

  const handleMutationFailure = (failure: CustomerMutationFailure) => {
    setNotice(mutationMessages[failure]);
    if (failure === "unauthenticated") onUnauthenticated?.();
  };

  const completeMutation = async (
    execute: () => Promise<CustomerMutationResult>,
    successMessage: string,
  ) => {
    const operation = startCustomerMutation(mutationInFlight, execute, () =>
      loadCustomerDetail(transport, customerID),
    );
    if (!operation) return;
    timelineLoadSequence.current += 1;
    setTimelinePending(false);
    loadSequence.current += 1;
    setMutationPending(true);
    setNotice(undefined);
    try {
      const result = await operation;
      if (result.status === "mutation_failed") {
        handleMutationFailure(result.failure);
        return;
      }
      if (result.status === "unconfirmed") {
        if (result.failure === "unauthenticated") onUnauthenticated?.();
        setNotice("操作已提交，但未能重新读取服务端事实。请稍后刷新确认。");
        return;
      }
      setPage({ kind: "ready", snapshot: result.snapshot });
      setNotice(successMessage);
    } finally {
      setMutationPending(false);
    }
  };

  const loadMoreTimeline = async () => {
    if (page.kind !== "ready" || timelinePending || mutationPending) return;
    const cursor = page.snapshot.eventsNextCursor;
    if (!cursor) return;
    const sequence = timelineLoadSequence.current + 1;
    timelineLoadSequence.current = sequence;
    setTimelinePending(true);
    setNotice(undefined);
    try {
      const result = await loadCustomerTimelinePage(
        transport,
        customerID,
        cursor,
        new Set(page.snapshot.events.map((event) => event.id)),
      );
      if (
        sequence !== timelineLoadSequence.current ||
        latestCustomerID.current !== customerID
      ) {
        return;
      }
      if (result.status !== "loaded") {
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setNotice(
          result.status === "unauthenticated"
            ? "登录状态已失效，请重新登录。"
            : result.status === "forbidden"
              ? "当前账号没有查看更多时间线记录的权限。"
              : result.status === "not_found"
                ? "客户已不可见，请刷新资料确认。"
                : "时间线暂时无法继续加载，已显示的记录保持不变。",
        );
        return;
      }
      setPage((current) => {
        if (
          current.kind !== "ready" ||
          current.snapshot.customer.id !== customerID ||
          current.snapshot.eventsNextCursor !== cursor
        ) {
          return current;
        }
        return {
          kind: "ready",
          snapshot: {
            ...current.snapshot,
            events: [...current.snapshot.events, ...result.events],
            eventsHaveMore: result.nextCursor !== undefined,
            eventsNextCursor: result.nextCursor,
          },
        };
      });
    } finally {
      if (sequence === timelineLoadSequence.current) setTimelinePending(false);
    }
  };

  const requestProfileSave = async (
    event: React.FormEvent<HTMLFormElement>,
  ) => {
    event.preventDefault();
    const update = parseProfileDraft(profile);
    if (!update) {
      setNotice("资料格式不正确，未发送保存请求。");
      return;
    }
    const token = csrfToken();
    if (!token) {
      setNotice("安全令牌缺失，未发送保存请求。");
      return;
    }
    await completeMutation(
      () => submitCustomerProfileUpdate(transport, customerID, update, token),
      "资料已保存，并已重新读取服务端事实。",
    );
  };

  const requestStageSave = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const stageID = parseNullablePositiveInteger(stageValue);
    if (stageID === undefined) {
      setNotice("阶段编号必须为正整数，或留空以清除阶段。");
      return;
    }
    const token = csrfToken();
    if (!token) {
      setNotice("安全令牌缺失，未发送阶段请求。");
      return;
    }
    await completeMutation(
      () => submitCustomerStageChange(transport, customerID, stageID, token),
      "阶段已保存，并已重新读取服务端事实。",
    );
  };

  const requestTagAdd = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const tagID = parseNullablePositiveInteger(selectedTagID);
    if (tagID === undefined || tagID === null) {
      setNotice("请选择要添加的标签。");
      return;
    }
    const token = csrfToken();
    if (!token) {
      setNotice("安全令牌缺失，未发送标签请求。");
      return;
    }
    await completeMutation(
      () => submitCustomerTagAdd(transport, customerID, tagID, token),
      "标签已添加，并已重新读取服务端事实。",
    );
  };

  const requestTagRemoval = async (tagID: number) => {
    const token = csrfToken();
    if (!token) {
      setNotice("安全令牌缺失，未发送标签请求。");
      return;
    }
    await completeMutation(
      () => submitCustomerTagRemoval(transport, customerID, tagID, token),
      "标签已移除，并已重新读取服务端事实。",
    );
  };

  if (page.kind === "loading") {
    return (
      <section className="customer-detail-page" aria-labelledby="app-title">
        <h1 id="app-title">客户详情</h1>
        <p className="customer-detail-page__state" role="status">
          正在读取客户资料、标签和时间线…
        </p>
      </section>
    );
  }

  if (page.kind === "not_found") {
    return <CustomerDetailMissingState />;
  }

  if (page.kind === "unauthenticated") {
    return (
      <section className="customer-detail-page" aria-labelledby="app-title">
        <h1 id="app-title">客户详情</h1>
        <div className="customer-detail-page__state" role="alert">
          登录状态已失效，正在返回登录页。
        </div>
      </section>
    );
  }

  if (page.kind === "forbidden") {
    return (
      <section className="customer-detail-page" aria-labelledby="app-title">
        <h1 id="app-title">客户详情</h1>
        <div className="customer-detail-page__state" role="alert">
          当前账号没有查看该客户的权限。
        </div>
      </section>
    );
  }

  if (page.kind === "unavailable") {
    return (
      <section className="customer-detail-page" aria-labelledby="app-title">
        <h1 id="app-title">客户详情</h1>
        <div className="customer-detail-page__state" role="alert">
          <p>客户服务暂时不可用。</p>
          <button type="button" onClick={() => void refresh()}>
            重试
          </button>
        </div>
      </section>
    );
  }

  const {
    customer,
    tags,
    tagCatalog,
    events,
    eventsHaveMore,
    eventsNextCursor,
  } = page.snapshot;
  const attachedTagIDs = new Set(tags.map((tag) => tag.id));
  const availableTags = tagCatalog.filter((tag) => !attachedTagIDs.has(tag.id));
  const campaignHref =
    role === "admin" || role === "ops"
      ? campaignSourceHref("customer_selection", customer.id)
      : undefined;

  return (
    <section className="customer-detail-page" aria-labelledby="app-title">
      <header className="customer-detail-page__heading">
        <div>
          <p className="route-card__eyebrow">客户档案</p>
          <h1 id="app-title">客户详情</h1>
          <p>
            {customer.name} · OneID #{customer.id}
          </p>
        </div>
        <div>
          <CustomerDetailReadNavigation includeCustomer360 />
          {campaignHref && (
            <a href={campaignHref}>以 OneID {customer.id} 发起 Campaign</a>
          )}
          <button
            className="button-secondary"
            disabled={mutationPending}
            type="button"
            onClick={() => void refresh()}
          >
            刷新资料
          </button>
        </div>
      </header>

      {notice && (
        <p className="customer-detail-page__notice" role="alert">
          {notice}
        </p>
      )}

      <div id="customer-360-summary">
        <CustomerContextPanel
          customerID={customerID}
          transport={contextTransport}
          onUnauthenticated={onUnauthenticated}
          showChatSummary={!(chatActivityRole && chatActivityTransport)}
          activityAnalyticsTransport={activityAnalyticsTransport}
        />
      </div>

      {mergeHistoryTransport && mergeHistoryRole && (
        <CustomerMergeHistoryPanel
          customerID={customerID}
          role={mergeHistoryRole}
          transport={mergeHistoryTransport}
          onUnauthenticated={onUnauthenticated}
        />
      )}

      {chatActivityRole && chatActivityTransport && (
        <CustomerChatActivityPanel
          customerID={customerID}
          role={chatActivityRole}
          transport={chatActivityTransport}
          onUnauthenticated={onUnauthenticated}
        />
      )}

      <div className="customer-detail-page__grid">
        <form
          className="customer-detail-page__card"
          onSubmit={requestProfileSave}
        >
          <fieldset disabled={mutationPending}>
            <legend>资料操作</legend>
            <label htmlFor="customer-name">名称</label>
            <input
              id="customer-name"
              name="customer-name"
              value={profile.name}
              onChange={(event) =>
                setProfile((current) => ({
                  ...current,
                  name: event.currentTarget.value,
                }))
              }
            />
            <label htmlFor="customer-avatar-url">头像地址</label>
            <input
              id="customer-avatar-url"
              name="customer-avatar-url"
              value={profile.avatarURL}
              onChange={(event) =>
                setProfile((current) => ({
                  ...current,
                  avatarURL: event.currentTarget.value,
                }))
              }
            />
            <label htmlFor="customer-gender">性别编号</label>
            <input
              id="customer-gender"
              inputMode="numeric"
              name="customer-gender"
              value={profile.gender}
              onChange={(event) =>
                setProfile((current) => ({
                  ...current,
                  gender: event.currentTarget.value,
                }))
              }
            />
            <label htmlFor="customer-owner">负责人编号</label>
            <input
              id="customer-owner"
              inputMode="numeric"
              name="customer-owner"
              value={profile.ownerStaffID}
              onChange={(event) =>
                setProfile((current) => ({
                  ...current,
                  ownerStaffID: event.currentTarget.value,
                }))
              }
            />
            <label htmlFor="customer-channel">渠道编号</label>
            <input
              id="customer-channel"
              inputMode="numeric"
              name="customer-channel"
              value={profile.channelID}
              onChange={(event) =>
                setProfile((current) => ({
                  ...current,
                  channelID: event.currentTarget.value,
                }))
              }
            />
            <button type="submit">
              {mutationPending ? "正在保存…" : "保存资料"}
            </button>
          </fieldset>
        </form>

        <form
          className="customer-detail-page__card"
          onSubmit={requestStageSave}
        >
          <fieldset disabled={mutationPending}>
            <legend>阶段</legend>
            <p className="customer-detail-page__meta">
              当前阶段：
              {customer.stageID === undefined
                ? "未设置"
                : `#${customer.stageID}`}
            </p>
            <label htmlFor="customer-stage-id">阶段编号</label>
            <input
              id="customer-stage-id"
              inputMode="numeric"
              name="customer-stage-id"
              placeholder="留空以清除"
              value={stageValue}
              onChange={(event) => setStageValue(event.currentTarget.value)}
            />
            <button type="submit">
              {mutationPending ? "正在保存…" : "保存阶段"}
            </button>
          </fieldset>
        </form>

        <section
          className="customer-detail-page__card"
          aria-labelledby="customer-tags-title"
        >
          <h2 id="customer-tags-title">标签</h2>
          {tags.length === 0 ? (
            <p className="customer-detail-page__meta" role="status">
              暂无标签。
            </p>
          ) : (
            <ul className="customer-detail-page__tag-list">
              {tags.map((tag) => (
                <li key={tag.id}>
                  <span>{tagLabel(tag)}</span>
                  <button
                    disabled={mutationPending}
                    type="button"
                    onClick={() => void requestTagRemoval(tag.id)}
                  >
                    移除
                  </button>
                </li>
              ))}
            </ul>
          )}
          <form onSubmit={requestTagAdd}>
            <fieldset disabled={mutationPending || availableTags.length === 0}>
              <legend>添加标签</legend>
              <label htmlFor="customer-tag-id">可添加标签</label>
              <select
                id="customer-tag-id"
                name="customer-tag-id"
                value={selectedTagID}
                onChange={(event) =>
                  setSelectedTagID(event.currentTarget.value)
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
                {mutationPending ? "正在添加…" : "添加标签"}
              </button>
            </fieldset>
          </form>
        </section>

        {(role === "admin" || role === "ops") && (
          <ContactPolicyPanel
            key={`${customerID}:${actorID ?? 0}:${role}`}
            customerID={customerID}
            transport={transport}
            readCookie={readCookie}
            keySource={contactPolicyKeySource}
            onUnauthenticated={onUnauthenticated}
          />
        )}
      </div>

      <section
        className="customer-detail-page__card"
        aria-labelledby="customer-timeline-title"
      >
        <h2 id="customer-timeline-title">时间线</h2>
        {events.length === 0 ? (
          <p className="customer-detail-page__meta" role="status">
            暂无时间线记录。
          </p>
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
        {eventsHaveMore && eventsNextCursor && (
          <p className="customer-detail-page__meta" role="status">
            <button
              type="button"
              disabled={timelinePending || mutationPending}
              onClick={() => void loadMoreTimeline()}
            >
              {timelinePending ? "正在加载…" : "加载更多时间线"}
            </button>
          </p>
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
          <dt>更新时间</dt>
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
