import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  deleteQuestionnaire,
  duplicateQuestionnaire,
  addQuestionnaireEditorOption,
  addQuestionnaireEditorQuestion,
  generatedQuestionnaireListTransport,
  loadQuestionnaireEditor,
  loadQuestionnaireDefinition,
  loadQuestionnaires,
  loadQuestionnairePreflight,
  loadQuestionnaireResults,
  nextQuestionnaireOffset,
  previousQuestionnaireOffset,
  questionnaireMutationReloadOffset,
  moveQuestionnaireEditorOption,
  moveQuestionnaireEditorQuestion,
  newQuestionnaireEditorDraft,
  newQuestionnaireEditorIdempotencyKey,
  removeQuestionnaireEditorOption,
  removeQuestionnaireEditorQuestion,
  saveQuestionnaireEditor,
  setQuestionnaireEditorQuestionRequired,
  setQuestionnaireEditorQuestionType,
  setQuestionnaireEnabled,
  updateQuestionnaireEditorOption,
  updateQuestionnaireEditorQuestion,
  type QuestionnaireEditorDraft,
  type QuestionnaireEditorLoadResult,
  type QuestionnaireEditorQuestionType,
  type QuestionnaireEditorSaveResult,
  type QuestionnaireDefinition,
  type QuestionnaireDefinitionResult,
  type QuestionnaireFailure,
  type QuestionnaireItem,
  type QuestionnaireListResult,
  type QuestionnaireListTransport,
  type QuestionnaireMutationResult,
  type QuestionnairePreflight,
  type QuestionnairePreflightResult,
  type QuestionnaireSubmissionAggregate,
} from "./questionnaire-list";
import {
  disableQuestionnairePublicDefinition,
  generatedQuestionnairePublicAnalyticsTransport,
  loadQuestionnairePublicAnalytics,
  newQuestionnairePublicIdempotencyKey,
  publishQuestionnairePublicDefinition,
  type PublicSurveyAnalytics,
  type PublicSurveyManagementReceipt,
  type QuestionnairePublicAnalyticsTransport,
} from "./questionnaire-public-analytics";

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

export type QuestionnaireResultsState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly item: QuestionnaireItem;
      readonly previous?: QuestionnaireSubmissionAggregate;
    }
  | {
      readonly kind: "ready";
      readonly item: QuestionnaireItem;
      readonly aggregate: QuestionnaireSubmissionAggregate;
    }
  | {
      readonly kind: "error";
      readonly item: QuestionnaireItem;
      readonly failure: QuestionnaireFailure;
      readonly previous?: QuestionnaireSubmissionAggregate;
    };

export type QuestionnaireDefinitionState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly item: QuestionnaireItem;
      readonly previous?: QuestionnaireDefinition;
    }
  | {
      readonly kind: "ready";
      readonly item: QuestionnaireItem;
      readonly definition: QuestionnaireDefinition;
    }
  | {
      readonly kind: "error";
      readonly item: QuestionnaireItem;
      readonly failure: QuestionnaireFailure;
    readonly previous?: QuestionnaireDefinition;
  };

export type QuestionnaireEditorState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly item: QuestionnaireItem }
  | { readonly kind: "ready"; readonly item?: QuestionnaireItem; readonly draft: QuestionnaireEditorDraft }
  | { readonly kind: "saving"; readonly item?: QuestionnaireItem; readonly draft: QuestionnaireEditorDraft }
  | { readonly kind: "error"; readonly failure: QuestionnaireFailure; readonly item?: QuestionnaireItem; readonly draft?: QuestionnaireEditorDraft };

export type QuestionnairePublicAnalyticsState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly item: QuestionnaireItem; readonly previous?: PublicSurveyAnalytics }
  | { readonly kind: "ready"; readonly item: QuestionnaireItem; readonly analytics: PublicSurveyAnalytics; readonly receipt?: PublicSurveyManagementReceipt }
  | { readonly kind: "error"; readonly item: QuestionnaireItem; readonly failure: QuestionnaireFailure; readonly previous?: PublicSurveyAnalytics };

export type QuestionnairePublicMutationAction = "publish" | "disable";

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
  readonly onLoadResults?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    item: QuestionnaireItem,
  ) => void;
  readonly onLoadDefinition?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    item: QuestionnaireItem,
  ) => void;
  readonly onLoadPublicAnalytics?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    item: QuestionnaireItem,
  ) => void;
  readonly onMutatePublic?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    item: QuestionnaireItem,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    action: QuestionnairePublicMutationAction,
  ) => void;
  readonly onCreate?: VoidFunction;
  readonly onEdit?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    item: QuestionnaireItem,
  ) => void;
  readonly definition?: QuestionnaireDefinitionState;
  readonly preflight?: QuestionnairePreflightState;
  readonly results?: QuestionnaireResultsState;
  readonly publicAnalytics?: QuestionnairePublicAnalyticsState;
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

function AggregateValues({
  aggregate,
}: {
  readonly aggregate: QuestionnaireSubmissionAggregate;
}): React.ReactElement {
  return (
    <dl>
      <dt>提交数</dt>
      <dd>{aggregate.submissionCount}</dd>
      <dt>最近提交时间</dt>
      <dd>{aggregate.latestSubmittedAt ?? "暂无"}</dd>
      <dt>平均分</dt>
      <dd>{aggregate.averageScore}</dd>
    </dl>
  );
}

function QuestionnaireResultsPanel({
  state,
}: {
  readonly state: QuestionnaireResultsState;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const title = `提交汇总：${state.item.title}`;
  if (state.kind === "loading")
    return (
      <section data-testid="questionnaire-results">
        <h2>{title}</h2>
        {state.previous ? <AggregateValues aggregate={state.previous} /> : null}
        <p>
          {state.previous ? "正在刷新本地提交汇总。" : "正在读取本地提交汇总。"}
        </p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section data-testid="questionnaire-results">
        <h2>{title}</h2>
        {state.previous ? <AggregateValues aggregate={state.previous} /> : null}
        <p role="alert">{messages[state.failure]}</p>
      </section>
    );
  return (
    <section data-testid="questionnaire-results">
      <h2>{title}</h2>
      <AggregateValues aggregate={state.aggregate} />
    </section>
  );
}

function PublicAnalyticsValues({
  analytics,
}: {
  readonly analytics: PublicSurveyAnalytics;
}): React.ReactElement {
  return (
    <>
      <dl>
        <dt>本地提交计数</dt>
        <dd>{analytics.submissionCount}</dd>
        <dt>公开定义版本</dt>
        <dd>{analytics.definitionVersion}</dd>
      </dl>
      <table>
        <thead>
          <tr>
            <th>题目序号</th>
            <th>题型</th>
            <th>已答计数</th>
            <th>选项计数</th>
          </tr>
        </thead>
        <tbody>
          {analytics.questions.map((question) => (
            <tr key={question.questionID}>
              <td>{question.sortOrder + 1}</td>
              <td>{question.type}</td>
              <td>{question.answeredCount}</td>
              <td>{question.options.map((option) => `#${option.sortOrder + 1}: ${option.selectionCount}`).join("，")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

function QuestionnairePublicAnalyticsPanel({
  state,
}: {
  readonly state: QuestionnairePublicAnalyticsState;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const title = `匿名公开问卷本地聚合：${state.item.title}`;
  if (state.kind === "loading") return (
    <section data-testid="questionnaire-public-analytics">
      <h2>{title}</h2>
      {state.previous ? <PublicAnalyticsValues analytics={state.previous} /> : null}
      <p>{state.previous ? "正在刷新本地匿名聚合。" : "正在读取本地匿名聚合。"}</p>
    </section>
  );
  if (state.kind === "error") return (
    <section data-testid="questionnaire-public-analytics">
      <h2>{title}</h2>
      {state.previous ? <PublicAnalyticsValues analytics={state.previous} /> : null}
      <p role="alert">{messages[state.failure]}</p>
    </section>
  );
  return (
    <section data-testid="questionnaire-public-analytics">
      <h2>{title}</h2>
      {state.receipt ? <p role="status">本地公开快照状态已确认：{state.receipt.state}。</p> : null}
      <PublicAnalyticsValues analytics={state.analytics} />
      <p>仅展示本地计数；不显示身份、答案、令牌或 Provider 结果。</p>
    </section>
  );
}

function DefinitionValues({
  definition,
}: {
  readonly definition: QuestionnaireDefinition;
}): React.ReactElement {
  return (
    <>
      <dl>
        <dt>说明</dt>
        <dd>{definition.description || "暂无"}</dd>
        <dt>答题方式</dt>
        <dd>{definition.answerDisplayMode}</dd>
        <dt>状态</dt>
        <dd>{definition.item.isDisabled ? "已停用" : "已启用"}</dd>
      </dl>
      <ol>
        {definition.questions.map((question, index) => (
          <li key={`${question.sortOrder}-${question.title}`}>
            <p>
              第 {index + 1} 题：{question.title}（{question.type}，
              {question.required ? "必答" : "选答"}）
            </p>
            {question.placeholderText ? <p>{question.placeholderText}</p> : null}
            {question.options.length > 0 ? (
              <ol>
                {question.options.map((option, optionIndex) => (
                  <li key={`${option.sortOrder}-${option.text}`}>
                    {optionIndex + 1}. {option.text}
                    {option.isOther && option.otherPlaceholder
                      ? `（其他：${option.otherPlaceholder}）`
                      : ""}
                  </li>
                ))}
              </ol>
            ) : null}
          </li>
        ))}
      </ol>
    </>
  );
}

function QuestionnaireDefinitionPanel({
  state,
}: {
  readonly state: QuestionnaireDefinitionState;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const title = `问卷定义：${state.item.title}`;
  if (state.kind === "loading")
    return (
      <section data-testid="questionnaire-definition">
        <h2>{title}</h2>
        {state.previous ? <DefinitionValues definition={state.previous} /> : null}
        <p>{state.previous ? "正在刷新问卷定义。" : "正在读取问卷定义。"}</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section data-testid="questionnaire-definition">
        <h2>{title}</h2>
        {state.previous ? <DefinitionValues definition={state.previous} /> : null}
        <p role="alert">{messages[state.failure]}</p>
      </section>
    );
  return (
    <section data-testid="questionnaire-definition">
      <h2>{title}</h2>
      <DefinitionValues definition={state.definition} />
    </section>
  );
}

export function QuestionnaireEditorPanel({
  state,
  onCancel,
  onDraft,
  onSave,
  outcomeUnknown = false,
  externalWriteLocked = false,
}: {
  readonly state: QuestionnaireEditorState;
  readonly onCancel: VoidFunction;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onDraft: (value: QuestionnaireEditorDraft) => void;
  readonly onSave: VoidFunction;
  readonly outcomeUnknown?: boolean;
  readonly externalWriteLocked?: boolean;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  if (state.kind === "loading")
    return <section className="route-card"><h2>编辑问卷</h2><p>正在读取完整问卷定义。</p></section>;
  if (state.kind === "error" && !state.draft)
    return <section className="route-card"><h2>编辑问卷</h2><p role="alert">{messages[state.failure]}</p><button type="button" onClick={onCancel}>关闭</button></section>;
  const draft = state.draft;
  if (!draft) return null;
  const saving = state.kind === "saving" || outcomeUnknown || externalWriteLocked;
  const change = (value: Partial<QuestionnaireEditorDraft>) => onDraft({ ...draft, ...value });
  return (
    <section className="route-card" data-testid="questionnaire-editor">
      <h2>{state.item ? `编辑问卷：${state.item.title}` : "新建问卷草稿"}</h2>
      <p>仅保存本地问卷定义；不会读取提交答案、打开公开链接或触发任何 Provider。</p>
      {outcomeUnknown ? <p role="alert">保存结果未知。为避免重复写入，本页已锁定写操作；请刷新后核对本地列表。</p> : externalWriteLocked ? <p role="status">正在确认另一项本地问卷写入；编辑已暂时锁定。</p> : state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
      <fieldset disabled={saving}>
        <p><label>名称 <input value={draft.name} onChange={(event) => change({ name: event.currentTarget.value })} /></label></p>
        <p><label>标题 <input value={draft.title} onChange={(event) => change({ title: event.currentTarget.value })} /></label></p>
        <p><label>Slug <input value={draft.slug} onChange={(event) => change({ slug: event.currentTarget.value })} /></label></p>
        <p><label>说明 <textarea value={draft.description} onChange={(event) => change({ description: event.currentTarget.value })} /></label></p>
        <p><label>答题方式 <select value={draft.answerDisplayMode} onChange={(event) => change({ answerDisplayMode: event.currentTarget.value as QuestionnaireEditorDraft["answerDisplayMode"] })}><option value="all_in_one">all_in_one</option><option value="one_by_one">one_by_one</option></select></label></p>
        <p><label><input type="checkbox" checked={draft.isDisabled} onChange={(event) => change({ isDisabled: event.currentTarget.checked })} /> 保持停用</label></p>
      </fieldset>
      <h3>题目</h3>
      <p>
        <button type="button" disabled={saving || draft.questions.length >= 100} onClick={() => onDraft(addQuestionnaireEditorQuestion(draft, "textarea"))}>添加文本题</button>{" "}
        <button type="button" disabled={saving || draft.questions.length >= 100} onClick={() => onDraft(addQuestionnaireEditorQuestion(draft, "single_choice"))}>添加单选题</button>{" "}
        <button type="button" disabled={saving || draft.questions.length >= 100} onClick={() => onDraft(addQuestionnaireEditorQuestion(draft, "multi_choice"))}>添加多选题</button>{" "}
        <button type="button" disabled={saving || draft.questions.length >= 100} onClick={() => onDraft(addQuestionnaireEditorQuestion(draft, "mobile"))}>添加手机号题</button>
      </p>
      <ol>
        {draft.questions.map((question, questionIndex) => {
          const choice = question.type === "single_choice" || question.type === "multi_choice";
          return (
            <li key={`question-${question.sortOrder}`}>
              <fieldset disabled={saving}>
                <p><label>类型 <select value={question.type} onChange={(event) => onDraft(setQuestionnaireEditorQuestionType(draft, questionIndex, event.currentTarget.value as QuestionnaireEditorQuestionType))}><option value="textarea">textarea</option><option value="single_choice">single_choice</option><option value="multi_choice">multi_choice</option><option value="mobile">mobile</option></select></label></p>
                <p><label>题目 <input value={question.title} onChange={(event) => onDraft(updateQuestionnaireEditorQuestion(draft, questionIndex, { title: event.currentTarget.value }))} /></label></p>
                {!choice ? <p><label>提示 <input value={question.placeholderText} onChange={(event) => onDraft(updateQuestionnaireEditorQuestion(draft, questionIndex, { placeholderText: event.currentTarget.value }))} /></label></p> : null}
                <p><label><input type="checkbox" checked={question.required} onChange={(event) => onDraft(setQuestionnaireEditorQuestionRequired(draft, questionIndex, event.currentTarget.checked))} /> 必答</label></p>
                <p>
                  <button type="button" disabled={questionIndex === 0} onClick={() => onDraft(moveQuestionnaireEditorQuestion(draft, questionIndex, -1))}>上移题目</button>{" "}
                  <button type="button" disabled={questionIndex + 1 === draft.questions.length} onClick={() => onDraft(moveQuestionnaireEditorQuestion(draft, questionIndex, 1))}>下移题目</button>{" "}
                  <button type="button" disabled={draft.questions.length <= 1} onClick={() => onDraft(removeQuestionnaireEditorQuestion(draft, questionIndex))}>删除题目</button>
                </p>
                {choice ? (
                  <>
                    <h4>选项</h4>
                    <ol>
                      {question.options.map((option, optionIndex) => (
                        <li key={`option-${option.sortOrder}`}>
                          <label>文本 <input value={option.optionText} onChange={(event) => onDraft(updateQuestionnaireEditorOption(draft, questionIndex, optionIndex, { optionText: event.currentTarget.value }))} /></label>{" "}
                          <label><input type="checkbox" checked={option.isOther} onChange={(event) => onDraft(updateQuestionnaireEditorOption(draft, questionIndex, optionIndex, { isOther: event.currentTarget.checked, otherPlaceholder: event.currentTarget.checked ? option.otherPlaceholder : "", otherMaxLength: event.currentTarget.checked ? Math.max(1, option.otherMaxLength || 200) : 0 }))} /> 其他</label>{" "}
                          <button type="button" disabled={optionIndex === 0} onClick={() => onDraft(moveQuestionnaireEditorOption(draft, questionIndex, optionIndex, -1))}>上移</button>{" "}
                          <button type="button" disabled={optionIndex + 1 === question.options.length} onClick={() => onDraft(moveQuestionnaireEditorOption(draft, questionIndex, optionIndex, 1))}>下移</button>{" "}
                          <button type="button" disabled={question.options.length <= 1} onClick={() => onDraft(removeQuestionnaireEditorOption(draft, questionIndex, optionIndex))}>删除</button>
                        </li>
                      ))}
                    </ol>
                    <button type="button" disabled={question.options.length >= 100} onClick={() => onDraft(addQuestionnaireEditorOption(draft, questionIndex))}>添加选项</button>
                  </>
                ) : null}
              </fieldset>
            </li>
          );
        })}
      </ol>
      <p><button type="button" disabled={saving} onClick={onSave}>{saving ? "正在保存" : "保存完整定义"}</button>{" "}<button type="button" disabled={saving} onClick={onCancel}>取消</button></p>
    </section>
  );
}

export function QuestionnaireListContent({
  busy,
  notice,
  onLoad,
  onLoadDefinition = noopLoadDefinition,
  onLoadPublicAnalytics = noopLoadPublicAnalytics,
  onMutatePublic = noopMutatePublic,
  onCreate = noopCreate,
  onEdit = noopEdit,
  onMutate,
  onLoadResults = noopLoadResults,
  definition = { kind: "idle" },
  preflight,
  publicAnalytics = { kind: "idle" },
  results = { kind: "idle" },
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
      <p>提交汇总只读取本地 Survey 聚合，不包含提交者身份或答案。</p>
      <p><button type="button" disabled={busy !== undefined} onClick={onCreate}>新建问卷</button></p>
      {(notice ?? copyNotice) ? (
        <p role="status">{notice ?? copyNotice}</p>
      ) : null}
      <QuestionnaireResultsPanel state={results} />
      <QuestionnaireDefinitionPanel state={definition} />
      <QuestionnairePublicAnalyticsPanel state={publicAnalytics} />
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
                    onClick={() => onEdit(item)}
                  >
                    编辑问卷
                  </button>
                  <button
                    type="button"
                    disabled={busy !== undefined || activePublicPath(publicAnalytics, item.id) === undefined}
                    onClick={() => void copyPublicLink({ ...item, publicPath: activePublicPath(publicAnalytics, item.id) ?? "" })}
                  >
                    复制公开链接
                  </button>
                  <button
                    type="button"
                    disabled={
                      busy !== undefined ||
                      (definition.kind === "loading" &&
                        definition.item.id === item.id)
                    }
                    onClick={() => onLoadDefinition(item)}
                  >
                    查看问卷定义
                  </button>
                  <button
                    type="button"
                    disabled={busy !== undefined}
                    onClick={() => onLoadResults(item)}
                  >
                    查看提交汇总
                  </button>
                  <button
                    type="button"
                    disabled={busy !== undefined}
                    onClick={() => onLoadPublicAnalytics(item)}
                  >
                    查看匿名公开聚合
                  </button>
                  <button
                    type="button"
                    disabled={busy !== undefined || item.isDisabled || activePublicDefinitionVersion(publicAnalytics, item.id) !== undefined}
                    onClick={() => onMutatePublic(item, "publish")}
                  >
                    发布匿名公开快照
                  </button>
                  <button
                    type="button"
                    disabled={busy !== undefined || activePublicDefinitionVersion(publicAnalytics, item.id) === undefined}
                    onClick={() => onMutatePublic(item, "disable")}
                  >
                    停用匿名公开快照
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

function noopLoadResults(): void {}
function noopLoadDefinition(): void {}
function noopLoadPublicAnalytics(): void {}
function noopMutatePublic(): void {}
function noopCreate(): void {}
function noopEdit(): void {}

export function QuestionnaireListPage({
  role,
  transport = generatedQuestionnaireListTransport,
  publicAnalyticsTransport = generatedQuestionnairePublicAnalyticsTransport,
  readCookie = () => (typeof document === "undefined" ? "" : document.cookie),
  onUnauthenticated,
}: {
  readonly role: "admin" | "ops" | "sales";
  readonly transport?: QuestionnaireListTransport;
  readonly publicAnalyticsTransport?: QuestionnairePublicAnalyticsTransport;
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
  const [results, setResults] = useState<QuestionnaireResultsState>({
    kind: "idle",
  });
  const [definition, setDefinition] = useState<QuestionnaireDefinitionState>({
    kind: "idle",
  });
  const [editor, setEditor] = useState<QuestionnaireEditorState>({
    kind: "idle",
  });
  const [editorOutcomeUnknown, setEditorOutcomeUnknown] = useState(false);
  const [editorWriteLocked, setEditorWriteLocked] = useState(false);
  const [publicAnalytics, setPublicAnalytics] = useState<QuestionnairePublicAnalyticsState>({ kind: "idle" });
  const [publicOutcomeUnknown, setPublicOutcomeUnknown] = useState(false);
  const [publicWriteLocked, setPublicWriteLocked] = useState(false);
  const resultsGeneration = useRef(0);
  const definitionGeneration = useRef(0);
  const editorGeneration = useRef(0);
  const editorSaveToken = useRef<symbol>();
  const editorLifetime = useRef<symbol>();
  const publicGeneration = useRef(0);
  const publicMutationToken = useRef<symbol>();
  const publicLifetime = useRef<symbol>();
  const definitionInflight = useRef(new Set<number>());
  useEffect(() => {
    const lifetime = Symbol("questionnaire-editor-lifetime");
    const replacing = editorLifetime.current !== undefined;
    editorLifetime.current = lifetime;
    if (replacing) {
      // This is a mounted replacement lifetime (role/transport changed), not
      // the cleanup of the old instance.
      setEditor({ kind: "idle" });
      setEditorWriteLocked(false);
    }
    return () => {
      if (editorLifetime.current !== lifetime) return;
      // A route/transport lifetime boundary must not let an old save or its
      // confirmation read mutate a replacement page instance. Cleanup never
      // sets React state: it also runs for a real unmount.
      ++editorGeneration.current;
      editorSaveToken.current = undefined;
    };
  }, [role, transport]);
  useEffect(() => {
    const lifetime = Symbol("questionnaire-public-lifetime");
    const replacing = publicLifetime.current !== undefined;
    publicLifetime.current = lifetime;
    if (replacing) {
      setPublicAnalytics({ kind: "idle" });
      setPublicWriteLocked(false);
    }
    return () => {
      if (publicLifetime.current !== lifetime) return;
      ++publicGeneration.current;
      publicMutationToken.current = undefined;
    };
  }, [publicAnalyticsTransport, role]);
  const load = useCallback(
    (offset: number) => {
      ++resultsGeneration.current;
      ++definitionGeneration.current;
      ++publicGeneration.current;
      setResults({ kind: "idle" });
      setDefinition({ kind: "idle" });
      setPublicAnalytics({ kind: "idle" });
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
  const loadResults = useCallback(
    (item: QuestionnaireItem) => {
      const generation = ++resultsGeneration.current;
      const previous = retainedAggregate(results, item.id);
      setResults({ kind: "loading", item, previous });
      void loadQuestionnaireResults(transport, item).then((result) => {
        if (generation !== resultsGeneration.current) return;
        if (result.status === "loaded") {
          setResults({ kind: "ready", item, aggregate: result.aggregate });
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setResults({ kind: "error", item, failure: result.status, previous });
      });
    },
    [onUnauthenticated, results, transport],
  );
  const loadDefinition = useCallback(
    (item: QuestionnaireItem) => {
      if (definitionInflight.current.has(item.id)) return;
      definitionInflight.current.add(item.id);
      const generation = ++definitionGeneration.current;
      const previous = retainedDefinition(definition, item.id);
      setDefinition({ kind: "loading", item, previous });
      void loadQuestionnaireDefinition(transport, item)
        .then((result: QuestionnaireDefinitionResult) => {
          if (generation !== definitionGeneration.current) return;
          if (result.status === "loaded") {
            setDefinition({ kind: "ready", item, definition: result.definition });
            return;
          }
          if (result.status === "unauthenticated") onUnauthenticated?.();
          setDefinition({ kind: "error", item, failure: result.status, previous });
        })
        .finally(() => definitionInflight.current.delete(item.id));
    },
    [definition, onUnauthenticated, transport],
  );
  const openEditor = useCallback(
    (item: QuestionnaireItem) => {
      if (editorOutcomeUnknown || publicOutcomeUnknown || editorSaveToken.current !== undefined || publicMutationToken.current !== undefined) return;
      const generation = ++editorGeneration.current;
      setEditor({ kind: "loading", item });
      void loadQuestionnaireEditor(transport, item.id).then(
        (result: QuestionnaireEditorLoadResult) => {
          if (generation !== editorGeneration.current) return;
          if (result.status === "loaded") {
            setEditor({ kind: "ready", item: result.item, draft: result.draft });
            return;
          }
          if (result.status === "unauthenticated") onUnauthenticated?.();
          setEditor({ kind: "error", item, failure: result.status });
        },
      );
    },
    [editorOutcomeUnknown, onUnauthenticated, publicOutcomeUnknown, transport],
  );
  const beginCreate = useCallback(() => {
    if (editorOutcomeUnknown || publicOutcomeUnknown || editorSaveToken.current !== undefined || publicMutationToken.current !== undefined) return;
    ++editorGeneration.current;
    setEditor({ kind: "ready", draft: newQuestionnaireEditorDraft() });
  }, [editorOutcomeUnknown, publicOutcomeUnknown]);
  const closeEditor = useCallback(() => {
    ++editorGeneration.current;
    setEditor({ kind: "idle" });
  }, []);
  const saveEditor = useCallback(() => {
    if (editor.kind !== "ready" || editorSaveToken.current !== undefined || publicMutationToken.current !== undefined || editorOutcomeUnknown || publicOutcomeUnknown) return;
    let csrf: string | undefined;
    try {
      csrf = readCSRFCookie(readCookie());
    } catch {
      csrf = undefined;
    }
    const key = newQuestionnaireEditorIdempotencyKey(
      editor.item ? "replace" : "create",
    );
    if (!csrf || !key) {
      setEditor({ ...editor, kind: "error", failure: "forbidden" });
      return;
    }
    const token = Symbol("questionnaire-editor-save");
    editorSaveToken.current = token;
    setEditorWriteLocked(true);
    const generation = ++editorGeneration.current;
    setEditor({ ...editor, kind: "saving" });
    void (async () => {
      try {
        const result: QuestionnaireEditorSaveResult = await saveQuestionnaireEditor(
          transport,
          editor.item,
          editor.draft,
          csrf,
          key,
        );
        if (generation !== editorGeneration.current) return;
        if (result.status !== "saved") {
          if (result.status === "unauthenticated") onUnauthenticated?.();
          if (result.status === "unavailable" || result.status === "invalid") {
            setEditorOutcomeUnknown(true);
            setMutationNotice("问卷写入结果未知。为避免重复写入，请刷新后核对本地列表。");
          }
          setEditor({ kind: "error", item: editor.item, draft: editor.draft, failure: result.status });
          return;
        }
        const rereadGeneration = ++editorGeneration.current;
        setEditor({ kind: "loading", item: result.item });
        const reread = await loadQuestionnaireEditor(
          transport,
          result.item.id,
          result.request,
        );
        if (rereadGeneration !== editorGeneration.current) return;
        if (reread.status === "loaded") {
          setEditor({ kind: "ready", item: reread.item, draft: reread.draft });
          setMutationNotice(editor.item ? "问卷定义已保存，已重新读取确认。" : "问卷草稿已创建，已重新读取确认。");
          load(0);
          return;
        }
        if (reread.status === "unauthenticated") onUnauthenticated?.();
        setEditorOutcomeUnknown(true);
        setMutationNotice("问卷写入结果未知。为避免重复写入，请刷新后核对本地列表。");
        setEditor({ kind: "error", item: result.item, failure: reread.status });
      } finally {
        if (editorSaveToken.current === token) {
          editorSaveToken.current = undefined;
          setEditorWriteLocked(false);
        }
      }
    })();
  }, [editor, editorOutcomeUnknown, load, onUnauthenticated, publicOutcomeUnknown, readCookie, transport]);
  useEffect(() => {
    if (role === "admin" || role === "ops") {
      load(0);
      loadPreflight();
    }
  }, [load, loadPreflight, role]);
  const mutate = async (
    item: QuestionnaireItem,
    action: QuestionnaireMutationAction,
  ) => {
    if (busy !== undefined || editorOutcomeUnknown || publicOutcomeUnknown || editorSaveToken.current !== undefined || publicMutationToken.current !== undefined) return;
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
  const loadPublicAnalytics = useCallback(
    (item: QuestionnaireItem, definitionVersion?: number) => {
      if (editorSaveToken.current !== undefined || publicMutationToken.current !== undefined) return;
      const generation = ++publicGeneration.current;
      const previous = retainedPublicAnalytics(publicAnalytics, item.id, definitionVersion);
      setPublicAnalytics({ kind: "loading", item, previous });
      void loadQuestionnairePublicAnalytics(
        publicAnalyticsTransport,
        item.id,
        definitionVersion,
      ).then((result) => {
        if (generation !== publicGeneration.current) return;
        if (result.status === "loaded") {
          setPublicAnalytics({ kind: "ready", item, analytics: result.analytics });
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setPublicAnalytics({ kind: "error", item, failure: result.status, previous });
      });
    },
    [onUnauthenticated, publicAnalytics, publicAnalyticsTransport],
  );
  const mutatePublic = useCallback(
    (item: QuestionnaireItem, action: QuestionnairePublicMutationAction) => {
      if (
        busy !== undefined || editorOutcomeUnknown || publicOutcomeUnknown ||
        editorSaveToken.current !== undefined || publicMutationToken.current !== undefined ||
        (action === "publish" && item.isDisabled)
      ) return;
      const confirm = typeof window !== "undefined" && typeof window.confirm === "function"
        ? window.confirm
        : undefined;
      const message = action === "publish"
        ? `确认发布问卷“${item.title}”的本地匿名公开快照？`
        : `确认停用问卷“${item.title}”的本地匿名公开快照？`;
      if (!confirm || !confirm(message)) return;
      const publicDefinitionVersion = activePublicDefinitionVersion(publicAnalytics, item.id);
      if (action === "disable" && publicDefinitionVersion === undefined) {
        setMutationNotice("请先读取该问卷的匿名公开聚合，确认当前公开快照版本后再停用。");
        return;
      }
      let csrf: string | undefined;
      try {
        csrf = readCSRFCookie(readCookie());
      } catch {
        csrf = undefined;
      }
      const key = newQuestionnairePublicIdempotencyKey(action);
      if (!csrf || !key) {
        setPublicAnalytics({ kind: "error", item, failure: "forbidden", previous: retainedPublicAnalytics(publicAnalytics, item.id) });
        return;
      }
      const token = Symbol("questionnaire-public-mutation");
      publicMutationToken.current = token;
      setPublicWriteLocked(true);
      const generation = ++publicGeneration.current;
      setPublicAnalytics({ kind: "loading", item, previous: retainedPublicAnalytics(publicAnalytics, item.id) });
      void (async () => {
        try {
          const result = action === "publish"
            ? await publishQuestionnairePublicDefinition(publicAnalyticsTransport, item.id, item.slug, item.version, csrf, key)
            : await disableQuestionnairePublicDefinition(publicAnalyticsTransport, item.id, item.slug, publicDefinitionVersion!, csrf, key);
          if (generation !== publicGeneration.current) return;
          if (result.status !== "saved") {
            if (result.status === "unauthenticated") onUnauthenticated?.();
            if (result.status === "unavailable") {
              setPublicOutcomeUnknown(true);
              setMutationNotice("匿名公开快照写入结果未知。为避免重复写入，请刷新后核对本地状态。");
            }
            setPublicAnalytics({ kind: "error", item, failure: result.status, previous: retainedPublicAnalytics(publicAnalytics, item.id) });
            return;
          }
          const confirmationGeneration = ++publicGeneration.current;
          const analytics = await loadQuestionnairePublicAnalytics(
            publicAnalyticsTransport,
            item.id,
            result.receipt.definitionVersion,
          );
          if (confirmationGeneration !== publicGeneration.current) return;
          if (analytics.status === "loaded") {
            setPublicAnalytics({ kind: "ready", item, receipt: result.receipt, analytics: analytics.analytics });
            setMutationNotice(action === "publish" ? "匿名公开快照已确认，并已读取本地聚合。" : "匿名公开快照已停用，并已读取本地聚合。" );
            return;
          }
          if (analytics.status === "unauthenticated") onUnauthenticated?.();
          setPublicOutcomeUnknown(true);
          setMutationNotice("匿名公开快照回读未确认。为避免重复写入，请刷新后核对本地状态。");
          setPublicAnalytics({ kind: "error", item, failure: analytics.status, previous: retainedPublicAnalytics(publicAnalytics, item.id, result.receipt.definitionVersion) });
        } finally {
          if (publicMutationToken.current === token) {
            publicMutationToken.current = undefined;
            setPublicWriteLocked(false);
          }
        }
      })();
    },
    [busy, editorOutcomeUnknown, onUnauthenticated, publicAnalytics, publicAnalyticsTransport, publicOutcomeUnknown, readCookie],
  );
  if (role !== "admin" && role !== "ops")
    return (
      <section className="route-card">
        <h1 id="app-title">问卷列表</h1>
        <p>当前账号没有问卷管理权限。</p>
      </section>
    );
  return (
    <>
      <QuestionnaireEditorPanel
        state={editor}
        outcomeUnknown={editorOutcomeUnknown || publicOutcomeUnknown}
        externalWriteLocked={publicWriteLocked}
        onCancel={closeEditor}
        onDraft={(draft) => setEditor((current) => current.kind === "ready" || current.kind === "error" ? { ...current, kind: "ready", draft } : current)}
        onSave={saveEditor}
      />
      <QuestionnaireListContent
        busy={busy ?? (editorWriteLocked || editorOutcomeUnknown || publicWriteLocked || publicOutcomeUnknown ? -1 : undefined)}
        notice={mutationNotice}
        onCreate={beginCreate}
        onEdit={openEditor}
        onLoad={load}
        onLoadDefinition={loadDefinition}
        onLoadPublicAnalytics={loadPublicAnalytics}
        onMutatePublic={mutatePublic}
        onLoadResults={loadResults}
        onMutate={({ item, action }) => void mutate(item, action)}
        preflight={preflight}
        definition={definition}
        results={results}
        publicAnalytics={publicAnalytics}
        state={state}
      />
    </>
  );
}

function retainedAggregate(
  state: QuestionnaireResultsState,
  questionnaireID: number,
): QuestionnaireSubmissionAggregate | undefined {
  if (state.kind === "idle" || state.item.id !== questionnaireID)
    return undefined;
  return state.kind === "ready"
    ? state.aggregate
    : state.kind === "loading" || state.kind === "error"
      ? state.previous
      : undefined;
}

function retainedDefinition(
  state: QuestionnaireDefinitionState,
  questionnaireID: number,
): QuestionnaireDefinition | undefined {
  if (state.kind === "idle" || state.item.id !== questionnaireID)
    return undefined;
  return state.kind === "ready"
    ? state.definition
    : state.kind === "loading" || state.kind === "error"
      ? state.previous
      : undefined;
}

function retainedPublicAnalytics(
  state: QuestionnairePublicAnalyticsState,
  questionnaireID: number,
  definitionVersion?: number,
): PublicSurveyAnalytics | undefined {
  if (
    state.kind === "idle" || state.item.id !== questionnaireID ||
    (definitionVersion !== undefined && state.kind === "ready" && state.analytics.definitionVersion !== definitionVersion)
  ) return undefined;
  if (state.kind === "ready") return state.analytics;
  return definitionVersion === undefined || state.previous?.definitionVersion === definitionVersion ? state.previous : undefined;
}

function activePublicDefinitionVersion(
  state: QuestionnairePublicAnalyticsState,
  questionnaireID: number,
): number | undefined {
  if (state.kind !== "ready" || state.item.id !== questionnaireID || state.analytics.state !== "public") return undefined;
  return state.analytics.definitionVersion;
}

function activePublicPath(
  state: QuestionnairePublicAnalyticsState,
  questionnaireID: number,
): string | undefined {
  if (state.kind !== "ready" || state.item.id !== questionnaireID || state.analytics.state !== "public") return undefined;
  return `/q/${state.analytics.slug}`;
}
