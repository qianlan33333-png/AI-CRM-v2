import React, { useCallback, useEffect, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  deleteQuestionnaire,
  duplicateQuestionnaire,
  generatedQuestionnaireListTransport,
  loadQuestionnaires,
  loadQuestionnairePreflight,
  nextQuestionnaireOffset,
  previousQuestionnaireOffset,
  questionnaireMutationReloadOffset,
  setQuestionnaireEnabled,
  type QuestionnaireFailure,
  type QuestionnaireItem,
  type QuestionnaireListResult,
  type QuestionnaireListTransport,
  type QuestionnaireMutationResult,
  type QuestionnairePreflight,
  type QuestionnairePreflightResult,
} from "./questionnaire-list";

const messages: Record<QuestionnaireFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有问卷管理权限。",
  not_found: "问卷已不存在，请刷新后重试。",
  conflict: "请求与已有操作冲突，请刷新后重试。",
  invalid: "问卷响应或请求不符合已冻结合同。",
  unavailable: "问卷服务暂时不可用，请稍后重试。",
};

export type QuestionnaireListState =
  | { readonly kind: "loading" }
  | {
      readonly kind: "ready";
      readonly items: readonly QuestionnaireItem[];
      readonly total: number;
      readonly offset: number;
    }
  | { readonly kind: "error"; readonly failure: QuestionnaireFailure };

export type QuestionnairePreflightState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly preflight: QuestionnairePreflight }
  | { readonly kind: "error"; readonly failure: QuestionnaireFailure };

export type QuestionnaireMutationAction = "toggle" | "delete" | "duplicate";
export type QuestionnairePageMutationResult =
  QuestionnaireMutationResult | { readonly status: "cancelled" };

export interface QuestionnairePageMutationInput {
  readonly action: QuestionnaireMutationAction;
  readonly confirmDelete: Window["confirm"];
  readonly item: QuestionnaireItem;
  readonly onBusy: React.Dispatch<number | undefined>;
  readonly onUnauthenticated?: VoidFunction;
  readonly readCookie: () => string;
  readonly transport: QuestionnaireListTransport;
}

export async function performQuestionnairePageMutation({
  action,
  confirmDelete,
  item,
  onBusy,
  onUnauthenticated,
  readCookie,
  transport,
}: QuestionnairePageMutationInput): Promise<QuestionnairePageMutationResult> {
  let csrf: string | undefined;
  try {
    csrf = readCSRFCookie(readCookie());
  } catch {
    csrf = undefined;
  }
  if (!csrf) return { status: "forbidden" };
  if (
    action === "delete" &&
    !confirmDelete(`确认删除已停用问卷“${item.title}”？`)
  )
    return { status: "cancelled" };
  onBusy(item.id);
  try {
    const result =
      action === "delete"
        ? await deleteQuestionnaire(transport, item, csrf)
        : action === "duplicate"
          ? await duplicateQuestionnaire(transport, item, csrf)
          : await setQuestionnaireEnabled(transport, item, csrf);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    return result;
  } finally {
    onBusy(undefined);
  }
}

export interface QuestionnaireMutationRequest {
  readonly item: QuestionnaireItem;
  readonly action: QuestionnaireMutationAction;
}

export type QuestionnairePublicLinkCopyResult = "copied" | "manual" | "missing";

export interface QuestionnairePublicLinkBrowser {
  readonly location: Pick<Location, "origin">;
  readonly navigator: {
    readonly clipboard?: Pick<Clipboard, "writeText">;
  };
  readonly prompt: Window["prompt"];
}

export async function copyQuestionnairePublicLink(
  item: QuestionnaireItem,
  browser: QuestionnairePublicLinkBrowser = window,
): Promise<QuestionnairePublicLinkCopyResult> {
  if (!item.publicPath) return "missing";
  const publicURL = new URL(
    item.publicPath,
    browser.location.origin,
  ).toString();
  try {
    if (!browser.navigator.clipboard) throw new Error("clipboard unavailable");
    await browser.navigator.clipboard.writeText(publicURL);
    return "copied";
  } catch {
    browser.prompt("请手动复制公开链接：", publicURL);
    return "manual";
  }
}

export interface QuestionnaireListContentProps {
  readonly busy: number | undefined;
  readonly notice?: string;
  readonly onLoad: React.Dispatch<number>;
  readonly onMutate: React.Dispatch<QuestionnaireMutationRequest>;
  readonly preflight?: QuestionnairePreflightState;
  readonly state: QuestionnaireListState;
}

function QuestionnairePreflightPanel({
  state,
}: {
  readonly state: QuestionnairePreflightState;
}): React.ReactElement {
  if (state.kind === "loading")
    return (
      <section data-testid="questionnaire-preflight">
        <h2>问卷预检</h2>
        <p>正在读取问卷预检。</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section data-testid="questionnaire-preflight">
        <h2>问卷预检</h2>
        <p>{messages[state.failure]}</p>
      </section>
    );
  const { checks } = state.preflight;
  return (
    <section data-testid="questionnaire-preflight">
      <h2>问卷预检</h2>
      <p>状态：{state.preflight.status}</p>
      <dl>
        <dt>wechat_oauth_configured</dt>
        <dd>{String(checks.wechatOAuthConfigured)}</dd>
        <dt>wecom_contact_configured</dt>
        <dd>{String(checks.wecomContactConfigured)}</dd>
        <dt>debug_session_api_enabled</dt>
        <dd>{String(checks.debugSessionAPIEnabled)}</dd>
        <dt>wecom_tags_api_available</dt>
        <dd>{String(checks.wecomTagsAPIAvailable)}</dd>
        <dt>questionnaire_admin_ui_enabled</dt>
        <dd>{String(checks.questionnaireAdminUIEnabled)}</dd>
        <dt>identity_map_available</dt>
        <dd>{String(checks.identityMapAvailable)}</dd>
      </dl>
    </section>
  );
}

export function QuestionnaireListContent({
  busy,
  notice,
  onLoad,
  onMutate,
  preflight,
  state,
}: QuestionnaireListContentProps): React.ReactElement {
  const [copyNotice, setCopyNotice] = useState<string>();
  const copyPublicLink = async (item: QuestionnaireItem) => {
    const result = await copyQuestionnairePublicLink(item);
    setCopyNotice(
      result === "copied"
        ? "公开链接已复制。"
        : result === "manual"
          ? "请在弹窗中手动复制公开链接。"
          : "该问卷没有公开链接。",
    );
  };
  const preflightPanel = preflight ? (
    <QuestionnairePreflightPanel state={preflight} />
  ) : null;
  if (state.kind === "loading")
    return (
      <section className="route-card">
        <h1 id="app-title">问卷列表</h1>
        {preflightPanel}
        <p>正在读取问卷列表。</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section className="route-card">
        <h1 id="app-title">问卷列表</h1>
        {preflightPanel}
        <p>{messages[state.failure]}</p>
        <button type="button" onClick={() => onLoad(0)}>
          重新加载
        </button>
      </section>
    );
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">Survey 本地管理</p>
      <h1 id="app-title">问卷列表</h1>
      {preflightPanel}
      <p>共 {state.total} 条问卷。写入成功后会重新加载列表。</p>
      {(notice ?? copyNotice) ? (
        <p role="status">{notice ?? copyNotice}</p>
      ) : null}
      {state.items.length === 0 ? (
        <p>当前没有问卷。</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>状态</th>
              <th>题目</th>
              <th>提交</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {state.items.map((item) => (
              <tr key={item.id}>
                <td>
                  {item.title}
                  <br />
                  <small>{item.name}</small>
                </td>
                <td>{item.isDisabled ? "已停用" : "已启用"}</td>
                <td>{item.questionCount}</td>
                <td>{item.submissionCount}</td>
                <td>
                  <button
                    type="button"
                    disabled={busy !== undefined}
                    onClick={() => void copyPublicLink(item)}
                  >
                    复制公开链接
                  </button>
                  <button
                    type="button"
                    disabled={busy !== undefined}
                    onClick={() => onMutate({ item, action: "duplicate" })}
                  >
                    复制问卷
                  </button>
                  <button
                    type="button"
                    disabled={busy !== undefined}
                    onClick={() => onMutate({ item, action: "toggle" })}
                  >
                    {item.isDisabled ? "启用" : "停用"}
                  </button>
                  {item.isDisabled ? (
                    <button
                      type="button"
                      disabled={busy !== undefined}
                      onClick={() => onMutate({ item, action: "delete" })}
                    >
                      删除
                    </button>
                  ) : (
                    <span> 请先停用后删除</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p>
        <button
          type="button"
          disabled={state.offset === 0 || busy !== undefined}
          onClick={() => onLoad(previousQuestionnaireOffset(state.offset))}
        >
          上一页
        </button>
        <button
          type="button"
          disabled={
            nextQuestionnaireOffset(
              state.offset,
              state.items.length,
              state.total,
            ) === undefined || busy !== undefined
          }
          onClick={() => {
            const offset = nextQuestionnaireOffset(
              state.offset,
              state.items.length,
              state.total,
            );
            if (offset !== undefined) onLoad(offset);
          }}
        >
          下一页
        </button>
      </p>
    </section>
  );
}

export function QuestionnaireListPage({
  role,
  transport = generatedQuestionnaireListTransport,
  readCookie = () => (typeof document === "undefined" ? "" : document.cookie),
  onUnauthenticated,
}: {
  readonly role: "admin" | "ops" | "sales";
  readonly transport?: QuestionnaireListTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const [state, setState] = useState<QuestionnaireListState>({
    kind: "loading",
  });
  const [preflight, setPreflight] = useState<QuestionnairePreflightState>({
    kind: "loading",
  });
  const [busy, setBusy] = useState<number>();
  const [mutationNotice, setMutationNotice] = useState<string>();
  const load = useCallback(
    (offset: number) => {
      setState({ kind: "loading" });
      void loadQuestionnaires(transport, offset).then(
        (result: QuestionnaireListResult) => {
          if (result.status === "loaded") {
            setState({
              kind: "ready",
              items: result.items,
              total: result.total,
              offset: result.offset,
            });
            return;
          }
          if (result.status === "unauthenticated") onUnauthenticated?.();
          setState({ kind: "error", failure: result.status });
        },
      );
    },
    [onUnauthenticated, transport],
  );
  const loadPreflight = useCallback(() => {
    setPreflight({ kind: "loading" });
    void loadQuestionnairePreflight(transport).then(
      (result: QuestionnairePreflightResult) => {
        if (result.status === "loaded") {
          setPreflight({ kind: "ready", preflight: result.preflight });
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setPreflight({ kind: "error", failure: result.status });
      },
    );
  }, [onUnauthenticated, transport]);
  useEffect(() => {
    if (role === "admin") {
      load(0);
      loadPreflight();
    }
  }, [load, loadPreflight, role]);
  const mutate = async (
    item: QuestionnaireItem,
    action: QuestionnaireMutationAction,
  ) => {
    if (busy !== undefined) return;
    setMutationNotice(undefined);
    const result = await performQuestionnairePageMutation({
      action,
      confirmDelete: (message) =>
        typeof window !== "undefined" && window.confirm(message),
      item,
      onBusy: setBusy,
      onUnauthenticated,
      readCookie,
      transport,
    });
    if (result.status === "cancelled") return;
    if (result.status === "saved") {
      if (action === "duplicate")
        setMutationNotice("问卷副本已创建，列表已刷新。");
      load(questionnaireMutationReloadOffset(result) ?? 0);
      return;
    }
    setState({ kind: "error", failure: result.status });
  };
  if (role !== "admin")
    return (
      <section className="route-card">
        <h1 id="app-title">问卷列表</h1>
        <p>当前账号没有问卷管理权限。</p>
      </section>
    );
  return (
    <QuestionnaireListContent
      busy={busy}
      notice={mutationNotice}
      onLoad={load}
      onMutate={({ item, action }) => void mutate(item, action)}
      preflight={preflight}
      state={state}
    />
  );
}
