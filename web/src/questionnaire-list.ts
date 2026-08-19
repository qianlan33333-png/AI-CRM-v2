import {
  deleteLegacyQuestionnaire,
  disableLegacyQuestionnaire,
  duplicateLegacyQuestionnaire,
  getLegacyQuestionnaire,
  getLegacyQuestionnairePreflight,
  getLegacyQuestionnaireResults,
  listLegacyQuestionnaires,
  type LegacyQuestionnaire,
} from "./api/generated/health";

export type QuestionnaireRole = "admin" | "ops" | "sales";
export const QUESTIONNAIRE_PAGE_SIZE = 50;

export interface QuestionnaireItem {
  readonly id: number;
  readonly name: string;
  readonly title: string;
  readonly publicPath: string;
  readonly isDisabled: boolean;
  readonly status: "active" | "disabled";
  readonly questionCount: number;
  readonly submissionCount: number;
  readonly updatedAt: string;
}

export interface QuestionnaireDefinitionOption {
  readonly text: string;
  readonly isOther: boolean;
  readonly otherPlaceholder: string;
  readonly sortOrder: number;
}

export interface QuestionnaireDefinitionQuestion {
  readonly type: "single_choice" | "multi_choice" | "textarea" | "mobile";
  readonly title: string;
  readonly required: boolean;
  readonly placeholderText: string;
  readonly sortOrder: number;
  readonly options: readonly QuestionnaireDefinitionOption[];
}

export interface QuestionnaireDefinition {
  readonly item: QuestionnaireItem;
  readonly description: string;
  readonly answerDisplayMode: "all_in_one" | "one_by_one";
  readonly questions: readonly QuestionnaireDefinitionQuestion[];
}

export interface QuestionnaireListTransport {
  readonly list: typeof listLegacyQuestionnaires;
  readonly definition: typeof getLegacyQuestionnaire;
  readonly disable: typeof disableLegacyQuestionnaire;
  readonly duplicate: typeof duplicateLegacyQuestionnaire;
  readonly remove: typeof deleteLegacyQuestionnaire;
  readonly preflight: typeof getLegacyQuestionnairePreflight;
  readonly results: typeof getLegacyQuestionnaireResults;
}

export const generatedQuestionnaireListTransport: QuestionnaireListTransport = {
  list: listLegacyQuestionnaires,
  definition: getLegacyQuestionnaire,
  disable: disableLegacyQuestionnaire,
  duplicate: duplicateLegacyQuestionnaire,
  remove: deleteLegacyQuestionnaire,
  preflight: getLegacyQuestionnairePreflight,
  results: getLegacyQuestionnaireResults,
};

export type QuestionnairePreflightStatus = "partial" | "ok";
export interface QuestionnairePreflightChecks {
  readonly wechatOAuthConfigured: false;
  readonly wecomContactConfigured: false;
  readonly debugSessionAPIEnabled: false;
  readonly wecomTagsAPIAvailable: false;
  readonly questionnaireAdminUIEnabled: true;
  readonly identityMapAvailable: false;
}
export interface QuestionnairePreflight {
  readonly checks: QuestionnairePreflightChecks;
  readonly status: QuestionnairePreflightStatus;
}

export type QuestionnaireFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";
export type QuestionnaireListResult =
  | {
      readonly status: "loaded";
      readonly items: readonly QuestionnaireItem[];
      readonly total: number;
      readonly limit: number;
      readonly offset: number;
    }
  | { readonly status: QuestionnaireFailure };
export type QuestionnaireMutationResult =
  { readonly status: "saved" } | { readonly status: QuestionnaireFailure };
export type QuestionnairePreflightResult =
  | { readonly status: "loaded"; readonly preflight: QuestionnairePreflight }
  | { readonly status: QuestionnaireFailure };
export interface QuestionnaireSubmissionAggregate {
  readonly submissionCount: number;
  readonly latestSubmittedAt: string | null;
  readonly averageScore: number;
}
export type QuestionnaireResultsResult =
  | {
      readonly status: "loaded";
      readonly aggregate: QuestionnaireSubmissionAggregate;
    }
  | { readonly status: QuestionnaireFailure };
export type QuestionnaireDefinitionResult =
  | { readonly status: "loaded"; readonly definition: QuestionnaireDefinition }
  | { readonly status: QuestionnaireFailure };

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function exact(
  value: Record<string, unknown>,
  keys: readonly string[],
): boolean {
  const actual = Object.keys(value);
  return (
    actual.length === keys.length && actual.every((key) => keys.includes(key))
  );
}
function text(value: unknown, maximum: number, empty = false): value is string {
  return (
    typeof value === "string" &&
    (empty || value.length > 0) &&
    [...value].length <= maximum
  );
}
function questionnaireSlug(value: unknown): value is string {
  return (
    text(value, 200) &&
    value === value.trim() &&
    value !== "." &&
    value !== ".." &&
    !/[\\\\/?%#\u0000-\u001f\u007f]/.test(value)
  );
}
function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function timestamp(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
      value,
    ) &&
    Number.isFinite(Date.parse(value))
  );
}
function aggregateTimestamp(value: unknown): value is string {
  if (!timestamp(value)) return false;
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
  const calendar = new Date(0);
  calendar.setUTCFullYear(year, month - 1, day);
  calendar.setUTCHours(hour, minute, second, 0);
  return (
    calendar.getUTCFullYear() === year &&
    calendar.getUTCMonth() === month - 1 &&
    calendar.getUTCDate() === day &&
    calendar.getUTCHours() === hour &&
    calendar.getUTCMinutes() === minute &&
    calendar.getUTCSeconds() === second
  );
}
function allowed(
  value: Record<string, unknown>,
  required: readonly string[],
  optional: readonly string[] = [],
): boolean {
  return (
    required.every((key) => key in value) &&
    Object.keys(value).every(
      (key) => required.includes(key) || optional.includes(key),
    )
  );
}
function validation(value: unknown): boolean {
  if (value === undefined) return true;
  if (
    !record(value) ||
    !allowed(
      value,
      [],
      ["min_selections", "max_selections", "min_length", "max_length"],
    )
  )
    return false;
  return (
    (value.min_selections === undefined ||
      (nonnegative(value.min_selections) && value.min_selections <= 100)) &&
    (value.max_selections === undefined ||
      (positive(value.max_selections) && value.max_selections <= 100)) &&
    (value.min_length === undefined ||
      (nonnegative(value.min_length) && value.min_length <= 10000)) &&
    (value.max_length === undefined ||
      (positive(value.max_length) && value.max_length <= 10000))
  );
}
function strictlyOrdered(value: readonly unknown[]): boolean {
  let previous = -1;
  return value.every((entry) => {
    if (!record(entry) || !nonnegative(entry.sort_order)) return false;
    if (entry.sort_order <= previous) return false;
    previous = entry.sort_order;
    return true;
  });
}
function frozenQuestions(value: readonly unknown[]): boolean {
  return strictlyOrdered(value) && value.every((question) => {
    if (
      !record(question) ||
      !allowed(
        question,
        [
          "type",
          "title",
          "assessment_dimension_key",
          "sidebar_profile_field",
          "required",
          "sort_order",
          "placeholder_text",
          "options",
        ],
        ["id", "validation"],
      ) ||
      (question.type !== "single_choice" &&
        question.type !== "multi_choice" &&
        question.type !== "textarea" &&
        question.type !== "mobile") ||
      (question.id !== undefined && !positive(question.id)) ||
      !text(question.title, 500) ||
      !text(question.assessment_dimension_key, 200, true) ||
      !text(question.sidebar_profile_field, 200, true) ||
      typeof question.required !== "boolean" ||
      !nonnegative(question.sort_order) ||
      !text(question.placeholder_text, 500, true) ||
      !validation(question.validation) ||
      !Array.isArray(question.options) ||
      question.options.length > 100 ||
      !strictlyOrdered(question.options)
    )
      return false;
    return question.options.every(
      (option) =>
        record(option) &&
        allowed(
          option,
          [
            "option_text",
            "score",
            "assessment_type_key",
            "tag_codes",
            "is_other",
            "other_placeholder",
            "other_max_length",
            "sort_order",
          ],
          ["id"],
        ) &&
        (option.id === undefined || positive(option.id)) &&
        text(option.option_text, 500) &&
        typeof option.score === "number" &&
        Number.isFinite(option.score) &&
        text(option.assessment_type_key, 200, true) &&
        Array.isArray(option.tag_codes) &&
        option.tag_codes.length <= 100 &&
        option.tag_codes.every((tag) => text(tag, 200)) &&
        typeof option.is_other === "boolean" &&
        text(option.other_placeholder, 500, true) &&
        nonnegative(option.other_max_length) &&
        option.other_max_length <= 2000 &&
        nonnegative(option.sort_order),
    );
  });
}

function questionnaireDefinition(
  value: unknown,
): QuestionnaireDefinition | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "name",
      "title",
      "description",
      "answer_display_mode",
      "assessment_enabled",
      "assessment_config",
      "slug",
      "is_disabled",
      "questions",
      "score_rules",
      "id",
      "enabled",
      "status",
      "version",
      "question_count",
      "submission_count",
      "created_at",
      "updated_at",
      "public_path",
      "submitted_path",
    ])
  )
    return undefined;
  if (
    !positive(value.id) ||
    !text(value.name, 120) ||
    !text(value.title, 300) ||
    !text(value.description, 10000, true) ||
    !questionnaireSlug(value.slug) ||
    typeof value.is_disabled !== "boolean" ||
    typeof value.enabled !== "boolean" ||
    value.enabled === value.is_disabled ||
    (value.status !== "active" && value.status !== "disabled") ||
    (value.status === "active") === value.is_disabled ||
    !positive(value.version) ||
    !positive(value.question_count) ||
    !nonnegative(value.submission_count) ||
    !timestamp(value.updated_at) ||
    !timestamp(value.created_at) ||
    !Array.isArray(value.questions) ||
    value.questions.length < 1 ||
    value.questions.length > 100 ||
    value.question_count !== value.questions.length ||
    !frozenQuestions(value.questions) ||
    !Array.isArray(value.score_rules) ||
    value.score_rules.length !== 0 ||
    !record(value.assessment_config) ||
    Object.keys(value.assessment_config).length !== 0 ||
    (value.answer_display_mode !== "all_in_one" &&
      value.answer_display_mode !== "one_by_one") ||
    value.assessment_enabled !== false ||
    !text(value.public_path, Number.MAX_SAFE_INTEGER, true) ||
    value.public_path !== `/q/${value.slug}` ||
    !text(value.submitted_path, Number.MAX_SAFE_INTEGER, true)
  )
    return undefined;
  const questions = value.questions as readonly Record<string, unknown>[];
  return {
    item: {
      id: value.id,
      name: value.name,
      title: value.title,
      publicPath: value.public_path,
      isDisabled: value.is_disabled,
      status: value.status,
      questionCount: value.question_count,
      submissionCount: value.submission_count,
      updatedAt: value.updated_at,
    },
    description: value.description,
    answerDisplayMode: value.answer_display_mode,
    questions: questions.map((question) => {
      const options = question.options as readonly Record<string, unknown>[];
      return {
        type: question.type as QuestionnaireDefinitionQuestion["type"],
        title: question.title as string,
        required: question.required as boolean,
        placeholderText: question.placeholder_text as string,
        sortOrder: question.sort_order as number,
        options: options.map((option) => ({
          text: option.option_text as string,
          isOther: option.is_other as boolean,
          otherPlaceholder: option.other_placeholder as string,
          sortOrder: option.sort_order as number,
        })),
      };
    }),
  };
}

function questionnaire(value: unknown): QuestionnaireItem | undefined {
  return questionnaireDefinition(value)?.item;
}

function failure(status: number): QuestionnaireFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400) return "invalid";
  return "unavailable";
}

export async function loadQuestionnaires(
  transport: QuestionnaireListTransport = generatedQuestionnaireListTransport,
  offset = 0,
): Promise<QuestionnaireListResult> {
  try {
    const response = await transport.list(
      { limit: QUESTIONNAIRE_PAGE_SIZE, offset },
      { credentials: "same-origin" },
    );
    if (response.status !== 200) return { status: failure(response.status) };
    const body: unknown = response.data;
    if (
      !record(body) ||
      !exact(body, [
        "ok",
        "questionnaires",
        "items",
        "data",
        "total",
        "limit",
        "offset",
      ]) ||
      body.ok !== true ||
      !Array.isArray(body.questionnaires) ||
      !Array.isArray(body.items) ||
      !record(body.data) ||
      !exact(body.data, ["questionnaires"]) ||
      !Array.isArray(body.data.questionnaires) ||
      !nonnegative(body.total) ||
      body.limit !== QUESTIONNAIRE_PAGE_SIZE ||
      body.offset !== offset
    )
      return { status: "invalid" };
    if (
      body.items.length > body.limit ||
      body.offset + body.items.length > body.total ||
      JSON.stringify(body.items) !== JSON.stringify(body.questionnaires) ||
      JSON.stringify(body.items) !== JSON.stringify(body.data.questionnaires)
    )
      return { status: "invalid" };
    const items = body.items.map(questionnaire);
    const listed = body.questionnaires.map(questionnaire);
    const nested = body.data.questionnaires.map(questionnaire);
    if (
      items.some((item) => !item) ||
      listed.some((item) => !item) ||
      nested.some((item) => !item) ||
      JSON.stringify(items) !== JSON.stringify(listed) ||
      JSON.stringify(items) !== JSON.stringify(nested) ||
      new Set(items.map((item) => item?.id)).size !== items.length
    )
      return { status: "invalid" };
    if (items.length === 0 && body.offset < body.total)
      return { status: "invalid" };
    return {
      status: "loaded",
      items: items as QuestionnaireItem[],
      total: body.total,
      limit: body.limit,
      offset: body.offset,
    };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadQuestionnairePreflight(
  transport: QuestionnaireListTransport = generatedQuestionnaireListTransport,
): Promise<QuestionnairePreflightResult> {
  try {
    const response = await transport.preflight({ credentials: "same-origin" });
    if (response.status !== 200) return { status: failure(response.status) };
    const body: unknown = response.data;
    if (
      !record(body) ||
      !exact(body, ["ok", "checks", "status"]) ||
      body.ok !== true ||
      (body.status !== "partial" && body.status !== "ok") ||
      !record(body.checks) ||
      !exact(body.checks, [
        "wechat_oauth_configured",
        "wecom_contact_configured",
        "debug_session_api_enabled",
        "wecom_tags_api_available",
        "questionnaire_admin_ui_enabled",
        "identity_map_available",
      ]) ||
      body.checks.wechat_oauth_configured !== false ||
      body.checks.wecom_contact_configured !== false ||
      body.checks.debug_session_api_enabled !== false ||
      body.checks.wecom_tags_api_available !== false ||
      body.checks.questionnaire_admin_ui_enabled !== true ||
      body.checks.identity_map_available !== false
    )
      return { status: "invalid" };
    const status: QuestionnairePreflightStatus =
      body.checks.wechat_oauth_configured &&
      body.checks.wecom_contact_configured &&
      body.checks.wecom_tags_api_available
        ? "ok"
        : "partial";
    if (body.status !== status) return { status: "invalid" };
    return {
      status: "loaded",
      preflight: {
        status,
        checks: {
          wechatOAuthConfigured: body.checks.wechat_oauth_configured,
          wecomContactConfigured: body.checks.wecom_contact_configured,
          debugSessionAPIEnabled: body.checks.debug_session_api_enabled,
          wecomTagsAPIAvailable: body.checks.wecom_tags_api_available,
          questionnaireAdminUIEnabled:
            body.checks.questionnaire_admin_ui_enabled,
          identityMapAvailable: body.checks.identity_map_available,
        },
      },
    };
  } catch {
    return { status: "unavailable" };
  }
}

function aggregate(
  value: unknown,
): QuestionnaireSubmissionAggregate | undefined {
  if (
    !record(value) ||
    !exact(value, [
      "submission_count",
      "latest_submitted_at",
      "average_score",
      "rules",
    ]) ||
    !nonnegative(value.submission_count) ||
    typeof value.average_score !== "number" ||
    !Number.isFinite(value.average_score) ||
    value.average_score < 0 ||
    !Array.isArray(value.rules) ||
    value.rules.length !== 0 ||
    (value.latest_submitted_at !== null &&
      !aggregateTimestamp(value.latest_submitted_at)) ||
    (value.submission_count === 0 &&
      (value.latest_submitted_at !== null || value.average_score !== 0)) ||
    (value.submission_count > 0 && value.latest_submitted_at === null)
  ) {
    return undefined;
  }
  return {
    submissionCount: value.submission_count,
    latestSubmittedAt: value.latest_submitted_at,
    averageScore: value.average_score,
  };
}

function equalAggregate(
  left: QuestionnaireSubmissionAggregate,
  right: QuestionnaireSubmissionAggregate,
): boolean {
  return (
    left.submissionCount === right.submissionCount &&
    left.latestSubmittedAt === right.latestSubmittedAt &&
    Object.is(left.averageScore, right.averageScore)
  );
}

export async function loadQuestionnaireResults(
  transport: QuestionnaireListTransport,
  item: QuestionnaireItem,
): Promise<QuestionnaireResultsResult> {
  if (!positive(item.id)) return { status: "invalid" };
  try {
    const response = await transport.results(item.id, {
      credentials: "same-origin",
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const body: unknown = response.data;
    if (
      !record(body) ||
      !exact(body, [
        "ok",
        "questionnaire_id",
        "results",
        "data",
        "side_effect_executed",
      ]) ||
      body.ok !== true ||
      body.questionnaire_id !== item.id ||
      body.side_effect_executed !== false ||
      !record(body.data) ||
      !exact(body.data, ["results"])
    ) {
      return { status: "invalid" };
    }
    const parsed = aggregate(body.results);
    const mirrored = aggregate(body.data.results);
    return parsed && mirrored && equalAggregate(parsed, mirrored)
      ? { status: "loaded", aggregate: parsed }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export async function loadQuestionnaireDefinition(
  transport: QuestionnaireListTransport,
  item: QuestionnaireItem,
): Promise<QuestionnaireDefinitionResult> {
  if (!positive(item.id)) return { status: "invalid" };
  try {
    const response = await transport.definition(item.id, {
      credentials: "same-origin",
    });
    if (response.status !== 200) return { status: failure(response.status) };
    const body: unknown = response.data;
    if (
      !record(body) ||
      !exact(body, ["ok", "questionnaire", "questions", "data"]) ||
      body.ok !== true ||
      !record(body.questionnaire) ||
      !Array.isArray(body.questions) ||
      !record(body.data) ||
      !exact(body.data, ["questionnaire"]) ||
      JSON.stringify(body.questions) !==
        JSON.stringify(body.questionnaire.questions) ||
      JSON.stringify(body.data.questionnaire) !==
        JSON.stringify(body.questionnaire)
    )
      return { status: "invalid" };
    const definition = questionnaireDefinition(body.questionnaire);
    return definition && definition.item.id === item.id
      ? { status: "loaded", definition }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

function writeOptions(csrfToken: string, operation: string): RequestInit {
  return {
    credentials: "same-origin",
    headers: {
      "X-CSRF-Token": csrfToken,
      "Idempotency-Key": `questionnaire-${operation}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`,
    },
  };
}
function mutation(
  body: unknown,
  id: number,
  status: "enabled" | "disabled" | "deleted",
): boolean {
  const keys =
    status === "deleted"
      ? [
          "ok",
          "questionnaire",
          "questions",
          "data",
          "write_model_status",
          "questionnaire_id",
          "deleted",
          "delete_mode",
        ]
      : [
          "ok",
          "questionnaire",
          "questions",
          "data",
          "write_model_status",
          "questionnaire_id",
        ];
  if (
    !record(body) ||
    !exact(body, keys) ||
    body.ok !== true ||
    body.questionnaire_id !== id ||
    body.write_model_status !== status ||
    !record(body.questionnaire) ||
    !questionnaire(body.questionnaire) ||
    !Array.isArray(body.questions) ||
    JSON.stringify(body.questions) !==
      JSON.stringify(body.questionnaire.questions) ||
    !record(body.data) ||
    !exact(body.data, ["questionnaire"]) ||
    JSON.stringify(body.data.questionnaire) !==
      JSON.stringify(body.questionnaire)
  )
    return false;
  const item = questionnaire(body.questionnaire);
  if (
    !item ||
    (status === "enabled" && item.isDisabled) ||
    ((status === "disabled" || status === "deleted") && !item.isDisabled)
  )
    return false;
  return (
    status !== "deleted" ||
    (body.deleted === true && body.delete_mode === "hard_delete")
  );
}
function duplicateMutation(body: unknown, sourceID: number): boolean {
  if (
    !record(body) ||
    !exact(body, [
      "ok",
      "questionnaire",
      "questions",
      "data",
      "write_model_status",
      "questionnaire_id",
      "source_questionnaire_id",
    ]) ||
    body.ok !== true ||
    !positive(body.questionnaire_id) ||
    body.questionnaire_id === sourceID ||
    body.source_questionnaire_id !== sourceID ||
    body.write_model_status !== "duplicated" ||
    !record(body.questionnaire) ||
    !questionnaire(body.questionnaire) ||
    !Array.isArray(body.questions) ||
    JSON.stringify(body.questions) !==
      JSON.stringify(body.questionnaire.questions) ||
    !record(body.data) ||
    !exact(body.data, ["questionnaire"]) ||
    JSON.stringify(body.data.questionnaire) !==
      JSON.stringify(body.questionnaire)
  )
    return false;
  const copy = questionnaire(body.questionnaire);
  return (
    copy !== undefined &&
    copy.id === body.questionnaire_id &&
    copy.isDisabled &&
    copy.status === "disabled"
  );
}
export async function duplicateQuestionnaire(
  transport: QuestionnaireListTransport,
  item: QuestionnaireItem,
  csrfToken: string,
): Promise<QuestionnaireMutationResult> {
  try {
    const response = await transport.duplicate(
      item.id,
      undefined,
      writeOptions(csrfToken, "duplicate"),
    );
    return response.status === 200 && duplicateMutation(response.data, item.id)
      ? { status: "saved" }
      : {
          status:
            response.status === 200 ? "invalid" : failure(response.status),
        };
  } catch {
    return { status: "unavailable" };
  }
}
export async function setQuestionnaireEnabled(
  transport: QuestionnaireListTransport,
  item: QuestionnaireItem,
  csrfToken: string,
): Promise<QuestionnaireMutationResult> {
  try {
    const disabled = !item.isDisabled;
    const response = await transport.disable(
      item.id,
      { is_disabled: disabled },
      writeOptions(csrfToken, disabled ? "disable" : "enable"),
    );
    return response.status === 200 &&
      mutation(response.data, item.id, disabled ? "disabled" : "enabled")
      ? { status: "saved" }
      : {
          status:
            response.status === 200 ? "invalid" : failure(response.status),
        };
  } catch {
    return { status: "unavailable" };
  }
}
export async function deleteQuestionnaire(
  transport: QuestionnaireListTransport,
  item: QuestionnaireItem,
  csrfToken: string,
): Promise<QuestionnaireMutationResult> {
  if (!item.isDisabled) return { status: "invalid" };
  try {
    const response = await transport.remove(
      item.id,
      writeOptions(csrfToken, "delete"),
    );
    return response.status === 200 &&
      mutation(response.data, item.id, "deleted")
      ? { status: "saved" }
      : {
          status:
            response.status === 200 ? "invalid" : failure(response.status),
        };
  } catch {
    return { status: "unavailable" };
  }
}

export function previousQuestionnaireOffset(offset: number): number {
  return Math.max(0, offset - QUESTIONNAIRE_PAGE_SIZE);
}
export function nextQuestionnaireOffset(
  offset: number,
  itemCount: number,
  total: number,
): number | undefined {
  if (itemCount < 1 || offset + itemCount >= total) return undefined;
  return offset + itemCount;
}
export function questionnaireMutationReloadOffset(
  result: QuestionnaireMutationResult,
): number | undefined {
  return result.status === "saved" ? 0 : undefined;
}
export function isQuestionnaire(value: unknown): value is LegacyQuestionnaire {
  return questionnaire(value) !== undefined;
}
