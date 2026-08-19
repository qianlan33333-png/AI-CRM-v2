import {
  createLegacyQuestionnaire,
  deleteLegacyQuestionnaire,
  disableLegacyQuestionnaire,
  duplicateLegacyQuestionnaire,
  getLegacyQuestionnaire,
  getLegacyQuestionnairePreflight,
  getLegacyQuestionnaireResults,
  listLegacyQuestionnaires,
  replaceLegacyQuestionnaire,
  type LegacyQuestionnaire,
  type LegacyQuestionnaireCreateRequest,
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
  readonly score: number;
  readonly assessmentTypeKey: string;
  readonly tagCodes: readonly string[];
  readonly isOther: boolean;
  readonly otherPlaceholder: string;
  readonly otherMaxLength: number;
  readonly sortOrder: number;
}

export interface QuestionnaireDefinitionQuestion {
  readonly type: "single_choice" | "multi_choice" | "textarea" | "mobile";
  readonly title: string;
  readonly assessmentDimensionKey: string;
  readonly sidebarProfileField: string;
  readonly required: boolean;
  readonly placeholderText: string;
  readonly validation?: {
    readonly minSelections?: number;
    readonly maxSelections?: number;
    readonly minLength?: number;
    readonly maxLength?: number;
  };
  readonly sortOrder: number;
  readonly options: readonly QuestionnaireDefinitionOption[];
}

export interface QuestionnaireDefinition {
  readonly item: QuestionnaireItem;
  readonly description: string;
  readonly answerDisplayMode: "all_in_one" | "one_by_one";
  readonly questions: readonly QuestionnaireDefinitionQuestion[];
}

export type QuestionnaireEditorQuestionType = QuestionnaireDefinitionQuestion["type"];
export interface QuestionnaireEditorOption {
  readonly optionText: string;
  readonly score: number;
  readonly assessmentTypeKey: string;
  readonly tagCodes: readonly string[];
  readonly isOther: boolean;
  readonly otherPlaceholder: string;
  readonly otherMaxLength: number;
  readonly sortOrder: number;
}
export interface QuestionnaireEditorQuestion {
  readonly type: QuestionnaireEditorQuestionType;
  readonly title: string;
  readonly assessmentDimensionKey: string;
  readonly sidebarProfileField: string;
  readonly required: boolean;
  readonly placeholderText: string;
  readonly validation?: {
    readonly minSelections?: number;
    readonly maxSelections?: number;
    readonly minLength?: number;
    readonly maxLength?: number;
  };
  readonly sortOrder: number;
  readonly options: readonly QuestionnaireEditorOption[];
}
export interface QuestionnaireEditorDraft {
  readonly name: string;
  readonly title: string;
  readonly description: string;
  readonly answerDisplayMode: "all_in_one" | "one_by_one";
  readonly slug: string;
  readonly isDisabled: boolean;
  readonly questions: readonly QuestionnaireEditorQuestion[];
}

export interface QuestionnaireListTransport {
  readonly list: typeof listLegacyQuestionnaires;
  readonly create: typeof createLegacyQuestionnaire;
  readonly definition: typeof getLegacyQuestionnaire;
  readonly replace: typeof replaceLegacyQuestionnaire;
  readonly disable: typeof disableLegacyQuestionnaire;
  readonly duplicate: typeof duplicateLegacyQuestionnaire;
  readonly remove: typeof deleteLegacyQuestionnaire;
  readonly preflight: typeof getLegacyQuestionnairePreflight;
  readonly results: typeof getLegacyQuestionnaireResults;
}

export const generatedQuestionnaireListTransport: QuestionnaireListTransport = {
  list: listLegacyQuestionnaires,
  create: createLegacyQuestionnaire,
  definition: getLegacyQuestionnaire,
  replace: replaceLegacyQuestionnaire,
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
        assessmentDimensionKey: question.assessment_dimension_key as string,
        sidebarProfileField: question.sidebar_profile_field as string,
        required: question.required as boolean,
        placeholderText: question.placeholder_text as string,
        validation: question.validation === undefined ? undefined : {
          ...(record(question.validation) && question.validation.min_selections !== undefined ? { minSelections: question.validation.min_selections as number } : {}),
          ...(record(question.validation) && question.validation.max_selections !== undefined ? { maxSelections: question.validation.max_selections as number } : {}),
          ...(record(question.validation) && question.validation.min_length !== undefined ? { minLength: question.validation.min_length as number } : {}),
          ...(record(question.validation) && question.validation.max_length !== undefined ? { maxLength: question.validation.max_length as number } : {}),
        },
        sortOrder: question.sort_order as number,
        options: options.map((option) => ({
          text: option.option_text as string,
          score: option.score as number,
          assessmentTypeKey: option.assessment_type_key as string,
          tagCodes: option.tag_codes as readonly string[],
          isOther: option.is_other as boolean,
          otherPlaceholder: option.other_placeholder as string,
          otherMaxLength: option.other_max_length as number,
          sortOrder: option.sort_order as number,
        })),
      };
    }),
  };
}

function questionnaire(value: unknown): QuestionnaireItem | undefined {
  return questionnaireDefinition(value)?.item;
}

const QUESTION_TYPES: ReadonlySet<QuestionnaireEditorQuestionType> = new Set([
  "single_choice", "multi_choice", "textarea", "mobile",
]);

function editorOption(sortOrder: number): QuestionnaireEditorOption {
  return {
    optionText: "",
    score: 0,
    assessmentTypeKey: "",
    tagCodes: [],
    isOther: false,
    otherPlaceholder: "",
    otherMaxLength: 0,
    sortOrder,
  };
}

function editorQuestion(
  sortOrder: number,
  type: QuestionnaireEditorQuestionType = "textarea",
): QuestionnaireEditorQuestion {
  return {
    type,
    title: "",
    assessmentDimensionKey: "",
    sidebarProfileField: "",
    required: false,
    placeholderText: "",
    validation:
      type === "textarea"
        ? { minLength: 0, maxLength: 2000 }
        : type === "mobile"
          ? { minLength: 0, maxLength: 32 }
          : { minSelections: 0, maxSelections: type === "single_choice" ? 1 : 1 },
    sortOrder,
    options: type === "single_choice" || type === "multi_choice" ? [editorOption(0)] : [],
  };
}

export function newQuestionnaireEditorDraft(): QuestionnaireEditorDraft {
  return {
    name: "",
    title: "",
    description: "",
    answerDisplayMode: "all_in_one",
    slug: "",
    // A new definition stays local-disabled until the administrator explicitly enables it.
    isDisabled: true,
    questions: [editorQuestion(0)],
  };
}

export function questionnaireEditorDraft(
  definition: QuestionnaireDefinition,
): QuestionnaireEditorDraft {
  return {
    name: definition.item.name,
    title: definition.item.title,
    description: definition.description,
    answerDisplayMode: definition.answerDisplayMode,
    slug: definition.item.publicPath.slice(3),
    isDisabled: definition.item.isDisabled,
    questions: definition.questions.map((question) => ({
      type: question.type,
      title: question.title,
      assessmentDimensionKey: question.assessmentDimensionKey,
      sidebarProfileField: question.sidebarProfileField,
      required: question.required,
      placeholderText: question.placeholderText,
      validation: question.validation,
      sortOrder: question.sortOrder,
      options: question.options.map((option) => ({
        optionText: option.text,
        score: option.score,
        assessmentTypeKey: option.assessmentTypeKey,
        tagCodes: option.tagCodes,
        isOther: option.isOther,
        otherPlaceholder: option.otherPlaceholder,
        otherMaxLength: option.otherMaxLength,
        sortOrder: option.sortOrder,
      })),
    })),
  };
}

function reorderQuestions(
  questions: readonly QuestionnaireEditorQuestion[],
): readonly QuestionnaireEditorQuestion[] {
  return questions.map((question, index) => ({ ...question, sortOrder: index }));
}
function reorderOptions(
  options: readonly QuestionnaireEditorOption[],
): readonly QuestionnaireEditorOption[] {
  return options.map((option, index) => ({ ...option, sortOrder: index }));
}

export function addQuestionnaireEditorQuestion(
  draft: QuestionnaireEditorDraft,
  type: QuestionnaireEditorQuestionType = "textarea",
): QuestionnaireEditorDraft {
  if (!QUESTION_TYPES.has(type) || draft.questions.length >= 100) return draft;
  return {
    ...draft,
    questions: [...draft.questions, editorQuestion(draft.questions.length, type)],
  };
}
export function removeQuestionnaireEditorQuestion(
  draft: QuestionnaireEditorDraft,
  index: number,
): QuestionnaireEditorDraft {
  if (!Number.isSafeInteger(index) || index < 0 || index >= draft.questions.length || draft.questions.length <= 1)
    return draft;
  return { ...draft, questions: reorderQuestions(draft.questions.filter((_, position) => position !== index)) };
}
export function moveQuestionnaireEditorQuestion(
  draft: QuestionnaireEditorDraft,
  index: number,
  direction: -1 | 1,
): QuestionnaireEditorDraft {
  const target = index + direction;
  if (!Number.isSafeInteger(index) || target < 0 || target >= draft.questions.length) return draft;
  const questions = [...draft.questions];
  [questions[index], questions[target]] = [questions[target], questions[index]];
  return { ...draft, questions: reorderQuestions(questions) };
}
export function updateQuestionnaireEditorQuestion(
  draft: QuestionnaireEditorDraft,
  index: number,
  update: Partial<Omit<QuestionnaireEditorQuestion, "sortOrder" | "options">> & {
    readonly options?: readonly QuestionnaireEditorOption[];
  },
): QuestionnaireEditorDraft {
  if (!Number.isSafeInteger(index) || index < 0 || index >= draft.questions.length) return draft;
  const questions = draft.questions.map((question, position) =>
    position === index ? { ...question, ...update, sortOrder: question.sortOrder, options: update.options ?? question.options } : question,
  );
  return { ...draft, questions };
}
export function setQuestionnaireEditorQuestionType(
  draft: QuestionnaireEditorDraft,
  index: number,
  type: QuestionnaireEditorQuestionType,
): QuestionnaireEditorDraft {
  if (!QUESTION_TYPES.has(type)) return draft;
  const current = draft.questions[index];
  if (!current) return draft;
  const choice = type === "single_choice" || type === "multi_choice";
  const validation = choice
    ? { minSelections: current.required ? 1 : 0, maxSelections: type === "single_choice" ? 1 : Math.max(1, current.options.length) }
    : { minLength: current.required ? 1 : 0, maxLength: type === "mobile" ? 32 : 2000 };
  return updateQuestionnaireEditorQuestion(draft, index, {
    type,
    placeholderText: choice ? "" : current.placeholderText,
    validation,
    options: choice ? (current.options.length ? reorderOptions(current.options) : [editorOption(0)]) : [],
  });
}
export function setQuestionnaireEditorQuestionRequired(
  draft: QuestionnaireEditorDraft,
  index: number,
  required: boolean,
): QuestionnaireEditorDraft {
  const current = draft.questions[index];
  if (!current || typeof required !== "boolean") return draft;
  const choice = current.type === "single_choice" || current.type === "multi_choice";
  const validation = choice
    ? {
        minSelections: required ? 1 : 0,
        maxSelections: current.type === "single_choice" ? 1 : Math.max(1, current.options.length),
      }
    : {
        minLength: required ? 1 : 0,
        maxLength: current.type === "mobile" ? 32 : 2000,
      };
  return updateQuestionnaireEditorQuestion(draft, index, { required, validation });
}
export function addQuestionnaireEditorOption(
  draft: QuestionnaireEditorDraft,
  questionIndex: number,
): QuestionnaireEditorDraft {
  const question = draft.questions[questionIndex];
  if (!question || (question.type !== "single_choice" && question.type !== "multi_choice") || question.options.length >= 100)
    return draft;
  return updateQuestionnaireEditorQuestion(draft, questionIndex, {
    options: [...question.options, editorOption(question.options.length)],
  });
}
export function removeQuestionnaireEditorOption(
  draft: QuestionnaireEditorDraft,
  questionIndex: number,
  optionIndex: number,
): QuestionnaireEditorDraft {
  const question = draft.questions[questionIndex];
  if (!question || !Number.isSafeInteger(optionIndex) || optionIndex < 0 || optionIndex >= question.options.length || question.options.length <= 1)
    return draft;
  const options = reorderOptions(question.options.filter((_, index) => index !== optionIndex));
  return updateQuestionnaireEditorQuestion(draft, questionIndex, {
    options,
    validation: {
      minSelections: question.required ? 1 : 0,
      maxSelections: question.type === "single_choice" ? 1 : Math.min(options.length, question.validation?.maxSelections ?? options.length),
    },
  });
}
export function moveQuestionnaireEditorOption(
  draft: QuestionnaireEditorDraft,
  questionIndex: number,
  optionIndex: number,
  direction: -1 | 1,
): QuestionnaireEditorDraft {
  const question = draft.questions[questionIndex];
  const target = optionIndex + direction;
  if (!question || !Number.isSafeInteger(optionIndex) || target < 0 || target >= question.options.length) return draft;
  const options = [...question.options];
  [options[optionIndex], options[target]] = [options[target], options[optionIndex]];
  return updateQuestionnaireEditorQuestion(draft, questionIndex, { options: reorderOptions(options) });
}
export function updateQuestionnaireEditorOption(
  draft: QuestionnaireEditorDraft,
  questionIndex: number,
  optionIndex: number,
  update: Partial<Omit<QuestionnaireEditorOption, "sortOrder">>,
): QuestionnaireEditorDraft {
  const question = draft.questions[questionIndex];
  if (!question || !Number.isSafeInteger(optionIndex) || optionIndex < 0 || optionIndex >= question.options.length)
    return draft;
  return updateQuestionnaireEditorQuestion(draft, questionIndex, {
    options: question.options.map((option, index) => index === optionIndex ? { ...option, ...update, sortOrder: option.sortOrder } : option),
  });
}

function textExact(value: unknown, maximum: number, empty = false): value is string {
  return text(value, maximum, empty) && value === value.trim();
}
function editorValidation(value: QuestionnaireEditorQuestion["validation"]): boolean {
  if (value === undefined) return true;
  return validation({
    ...(value.minSelections === undefined ? {} : { min_selections: value.minSelections }),
    ...(value.maxSelections === undefined ? {} : { max_selections: value.maxSelections }),
    ...(value.minLength === undefined ? {} : { min_length: value.minLength }),
    ...(value.maxLength === undefined ? {} : { max_length: value.maxLength }),
  });
}
export function questionnaireEditorRequest(
  draft: QuestionnaireEditorDraft,
): LegacyQuestionnaireCreateRequest | undefined {
  if (
    !textExact(draft.name, 120) ||
    !textExact(draft.title, 300) ||
    !textExact(draft.description, 10000, true) ||
    !questionnaireSlug(draft.slug) ||
    (draft.answerDisplayMode !== "all_in_one" && draft.answerDisplayMode !== "one_by_one") ||
    typeof draft.isDisabled !== "boolean" ||
    draft.questions.length < 1 || draft.questions.length > 100
  ) return undefined;
  const questions = draft.questions.map((question, index) => ({
    type: question.type,
    title: question.title,
    assessment_dimension_key: question.assessmentDimensionKey,
    sidebar_profile_field: question.sidebarProfileField,
    required: question.required,
    sort_order: index,
    placeholder_text: question.placeholderText,
    ...(question.validation === undefined ? {} : {
      validation: {
        ...(question.validation.minSelections === undefined ? {} : { min_selections: question.validation.minSelections }),
        ...(question.validation.maxSelections === undefined ? {} : { max_selections: question.validation.maxSelections }),
        ...(question.validation.minLength === undefined ? {} : { min_length: question.validation.minLength }),
        ...(question.validation.maxLength === undefined ? {} : { max_length: question.validation.maxLength }),
      },
    }),
    options: question.options.map((option, optionIndex) => ({
      option_text: option.optionText,
      score: option.score,
      assessment_type_key: option.assessmentTypeKey,
      tag_codes: [...option.tagCodes],
      is_other: option.isOther,
      other_placeholder: option.otherPlaceholder,
      other_max_length: option.otherMaxLength,
      sort_order: optionIndex,
    })),
  }));
  if (!frozenQuestions(questions) || !questions.every((question) => {
    const source = draft.questions[question.sort_order];
    return source !== undefined && editorValidation(source.validation);
  })) return undefined;
  return {
    name: draft.name,
    title: draft.title,
    description: draft.description,
    answer_display_mode: draft.answerDisplayMode,
    assessment_enabled: false,
    assessment_config: {},
    slug: draft.slug,
    is_disabled: draft.isDisabled,
    questions,
    score_rules: [],
  };
}

export type QuestionnaireEditorLoadResult =
  | { readonly status: "loaded"; readonly item: QuestionnaireItem; readonly draft: QuestionnaireEditorDraft }
  | { readonly status: QuestionnaireFailure };
export type QuestionnaireEditorSaveResult =
  | { readonly status: "saved"; readonly item: QuestionnaireItem; readonly request: LegacyQuestionnaireCreateRequest }
  | { readonly status: QuestionnaireFailure };

function sameEditorValidation(
  left: LegacyQuestionnaireCreateRequest["questions"][number]["validation"] | undefined,
  right: LegacyQuestionnaireCreateRequest["questions"][number]["validation"] | undefined,
): boolean {
  const leftKeys = left === undefined ? [] : Object.keys(left).sort();
  const rightKeys = right === undefined ? [] : Object.keys(right).sort();
  return leftKeys.length === rightKeys.length && leftKeys.every((key, index) => key === rightKeys[index] && left?.[key as keyof typeof left] === right?.[key as keyof typeof right]);
}

function sameEditorRequest(
  left: LegacyQuestionnaireCreateRequest,
  right: LegacyQuestionnaireCreateRequest,
): boolean {
  if (
    left.name !== right.name || left.title !== right.title ||
    left.description !== right.description ||
    left.answer_display_mode !== right.answer_display_mode ||
    left.assessment_enabled !== right.assessment_enabled ||
    JSON.stringify(left.assessment_config) !== JSON.stringify(right.assessment_config) ||
    left.slug !== right.slug || left.is_disabled !== right.is_disabled ||
    JSON.stringify(left.score_rules) !== JSON.stringify(right.score_rules) ||
    left.questions.length !== right.questions.length
  ) return false;
  return left.questions.every((question, index) => {
    const candidate = right.questions[index];
    if (!candidate || question.type !== candidate.type || question.title !== candidate.title ||
      question.assessment_dimension_key !== candidate.assessment_dimension_key ||
      question.sidebar_profile_field !== candidate.sidebar_profile_field || question.required !== candidate.required ||
      question.sort_order !== candidate.sort_order || question.placeholder_text !== candidate.placeholder_text ||
      !sameEditorValidation(question.validation, candidate.validation) || question.options.length !== candidate.options.length) return false;
    return question.options.every((option, optionIndex) => {
      const other = candidate.options[optionIndex];
      return other !== undefined && option.option_text === other.option_text && option.score === other.score &&
        option.assessment_type_key === other.assessment_type_key && JSON.stringify(option.tag_codes) === JSON.stringify(other.tag_codes) &&
        option.is_other === other.is_other && option.other_placeholder === other.other_placeholder &&
        option.other_max_length === other.other_max_length && option.sort_order === other.sort_order;
    });
  });
}

function editorDefinitionRequest(
  definition: QuestionnaireDefinition,
): LegacyQuestionnaireCreateRequest | undefined {
  return questionnaireEditorRequest(questionnaireEditorDraft(definition));
}

function editorDefinitionResult(
  body: unknown,
  expectedID: number,
  expectedRequest?: LegacyQuestionnaireCreateRequest,
): QuestionnaireDefinition | undefined {
  if (
    !record(body) ||
    !exact(body, ["ok", "questionnaire", "questions", "data"]) ||
    body.ok !== true ||
    !record(body.questionnaire) ||
    !Array.isArray(body.questions) ||
    !record(body.data) ||
    !exact(body.data, ["questionnaire"]) ||
    JSON.stringify(body.questions) !== JSON.stringify(body.questionnaire.questions) ||
    JSON.stringify(body.data.questionnaire) !== JSON.stringify(body.questionnaire)
  ) return undefined;
  const definition = questionnaireDefinition(body.questionnaire);
  if (definition?.item.id !== expectedID) return undefined;
  const observed = editorDefinitionRequest(definition);
  return observed !== undefined && (expectedRequest === undefined || sameEditorRequest(observed, expectedRequest))
    ? definition
    : undefined;
}

export async function loadQuestionnaireEditor(
  transport: QuestionnaireListTransport,
  questionnaireID: number,
  expectedRequest?: LegacyQuestionnaireCreateRequest,
): Promise<QuestionnaireEditorLoadResult> {
  if (!positive(questionnaireID)) return { status: "invalid" };
  try {
    const response = await transport.definition(questionnaireID, { credentials: "same-origin" });
    if (response.status !== 200) return { status: failure(response.status) };
    const definition = editorDefinitionResult(response.data, questionnaireID, expectedRequest);
    return definition
      ? { status: "loaded", item: definition.item, draft: questionnaireEditorDraft(definition) }
      : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

export function newQuestionnaireEditorIdempotencyKey(
  operation: "create" | "replace",
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  try {
    const uuid = source?.randomUUID();
    return typeof uuid === "string" &&
      /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid)
      ? `questionnaire-${operation}:${uuid}`
      : undefined;
  } catch {
    return undefined;
  }
}
function validEditorCSRF(value: string): boolean {
  return /^[A-Za-z0-9_-]{43}$/.test(value);
}
function validEditorIdempotencyKey(value: string): boolean {
  return /^[A-Za-z0-9:_-]{16,128}$/.test(value) && value === value.trim();
}
function editorWriteOptions(csrfToken: string, idempotencyKey: string): RequestInit | undefined {
  return validEditorCSRF(csrfToken) && validEditorIdempotencyKey(idempotencyKey)
    ? {
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": csrfToken,
          "Idempotency-Key": idempotencyKey,
        },
      }
    : undefined;
}
function editorMutationResult(
  body: unknown,
  expectedID: number | undefined,
  kind: "created" | "updated",
  expectedRequest: LegacyQuestionnaireCreateRequest,
): QuestionnaireDefinition | undefined {
  const keys = kind === "created"
    ? ["ok", "questionnaire", "questionnaire_id", "questions", "data"]
    : ["ok", "questionnaire", "questions", "data", "write_model_status", "questionnaire_id"];
  if (
    !record(body) ||
    !exact(body, keys) ||
    body.ok !== true ||
    !positive(body.questionnaire_id) ||
    (expectedID !== undefined && body.questionnaire_id !== expectedID) ||
    (kind === "updated" && body.write_model_status !== "updated") ||
    !record(body.questionnaire) ||
    !Array.isArray(body.questions) ||
    !record(body.data) ||
    !exact(body.data, ["questionnaire"]) ||
    JSON.stringify(body.questions) !== JSON.stringify(body.questionnaire.questions) ||
    JSON.stringify(body.data.questionnaire) !== JSON.stringify(body.questionnaire)
  ) return undefined;
  const definition = questionnaireDefinition(body.questionnaire);
  const observed = definition === undefined ? undefined : editorDefinitionRequest(definition);
  return definition?.item.id === body.questionnaire_id && observed !== undefined && sameEditorRequest(observed, expectedRequest)
    ? definition
    : undefined;
}
export async function saveQuestionnaireEditor(
  transport: QuestionnaireListTransport,
  existing: QuestionnaireItem | undefined,
  draft: QuestionnaireEditorDraft,
  csrfToken: string,
  idempotencyKey: string,
): Promise<QuestionnaireEditorSaveResult> {
  const request = questionnaireEditorRequest(draft);
  const options = editorWriteOptions(csrfToken, idempotencyKey);
  if (!request || !options || (existing !== undefined && !positive(existing.id))) return { status: "invalid" };
  try {
    const response = existing === undefined
      ? await transport.create(request, options)
      : await transport.replace(existing.id, request, options);
    if (response.status !== 200) return { status: failure(response.status) };
    const definition = editorMutationResult(
      response.data,
      existing?.id,
      existing === undefined ? "created" : "updated",
      request,
    );
    return definition ? { status: "saved", item: definition.item, request } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
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
