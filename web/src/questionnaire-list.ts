import {
  deleteLegacyQuestionnaire,
  disableLegacyQuestionnaire,
  listLegacyQuestionnaires,
  type LegacyQuestionnaire,
} from "./api/generated/health";

export type QuestionnaireRole = "admin" | "ops" | "sales";
export const QUESTIONNAIRE_PAGE_SIZE = 50;

export interface QuestionnaireItem {
  readonly id: number;
  readonly name: string;
  readonly title: string;
  readonly isDisabled: boolean;
  readonly status: "active" | "disabled";
  readonly questionCount: number;
  readonly submissionCount: number;
  readonly updatedAt: string;
}

export interface QuestionnaireListTransport {
  readonly list: typeof listLegacyQuestionnaires;
  readonly disable: typeof disableLegacyQuestionnaire;
  readonly remove: typeof deleteLegacyQuestionnaire;
}

export const generatedQuestionnaireListTransport: QuestionnaireListTransport = {
  list: listLegacyQuestionnaires,
  disable: disableLegacyQuestionnaire,
  remove: deleteLegacyQuestionnaire,
};

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
function frozenQuestions(value: readonly unknown[]): boolean {
  return value.every((question) => {
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
      question.options.length > 100
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

function questionnaire(value: unknown): QuestionnaireItem | undefined {
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
    !text(value.slug, 200) ||
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
    !text(value.submitted_path, Number.MAX_SAFE_INTEGER, true)
  )
    return undefined;
  return {
    id: value.id,
    name: value.name,
    title: value.title,
    isDisabled: value.is_disabled,
    status: value.status,
    questionCount: value.question_count,
    submissionCount: value.submission_count,
    updatedAt: value.updated_at,
  };
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
