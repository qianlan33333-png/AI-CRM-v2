import {
  disableQuestionnairePublicDefinition as generatedDisableQuestionnairePublicDefinition,
  getQuestionnairePublicAnalytics,
  publishQuestionnairePublicDefinition as generatedPublishQuestionnairePublicDefinition,
} from "./api/generated/health";

export type QuestionnairePublicFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";

export interface PublicSurveyManagementReceipt {
  readonly questionnaireID: number;
  readonly slug: string;
  readonly definitionVersion: number;
  readonly state: "public" | "disabled";
}

export interface PublicSurveyAnalytics {
  readonly questionnaireID: number;
  readonly definitionVersion: number;
  readonly slug: string;
  readonly state: "public" | "disabled";
  readonly submissionCount: number;
  readonly questions: readonly {
    readonly questionID: number;
    readonly type: "single_choice" | "multi_choice";
    readonly sortOrder: number;
    readonly answeredCount: number;
    readonly options: readonly {
      readonly optionID: number;
      readonly sortOrder: number;
      readonly selectionCount: number;
    }[];
  }[];
}

export interface QuestionnairePublicAnalyticsTransport {
  readonly analytics: typeof getQuestionnairePublicAnalytics;
  readonly publish: typeof generatedPublishQuestionnairePublicDefinition;
  readonly disable: typeof generatedDisableQuestionnairePublicDefinition;
}

export const generatedQuestionnairePublicAnalyticsTransport: QuestionnairePublicAnalyticsTransport = {
  analytics: getQuestionnairePublicAnalytics,
  publish: generatedPublishQuestionnairePublicDefinition,
  disable: generatedDisableQuestionnairePublicDefinition,
};

export type PublicSurveyAnalyticsResult =
  | { readonly status: "loaded"; readonly analytics: PublicSurveyAnalytics }
  | { readonly status: QuestionnairePublicFailure };
export type PublicSurveyManagementResult =
  | { readonly status: "saved"; readonly receipt: PublicSurveyManagementReceipt }
  | { readonly status: QuestionnairePublicFailure };

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}
function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function slug(value: unknown): value is string {
  return typeof value === "string" && /^[a-z0-9][a-z0-9-]{0,119}$/.test(value);
}
function failure(status: number): QuestionnairePublicFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400) return "invalid";
  return "unavailable";
}
function validCSRF(value: string): boolean {
  return /^[A-Za-z0-9_-]{43}$/.test(value);
}
function validIdempotencyKey(value: string): boolean {
  return /^[A-Za-z0-9:_-]{16,128}$/.test(value) && value === value.trim();
}

export function newQuestionnairePublicIdempotencyKey(
  operation: "publish" | "disable",
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  try {
    const uuid = source?.randomUUID();
    return typeof uuid === "string" &&
      /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid)
      ? `questionnaire-public-${operation}:${uuid}`
      : undefined;
  } catch {
    return undefined;
  }
}

export function parsePublicSurveyManagementReceipt(
  value: unknown,
  questionnaireID: number,
  expectedSlug: string | undefined,
  expectedVersion: number | undefined,
  expectedState: "public" | "disabled",
): PublicSurveyManagementReceipt | undefined {
  if (
    !record(value) ||
    !exact(value, ["questionnaire_id", "slug", "definition_version", "state"]) ||
    !positive(value.questionnaire_id) ||
    value.questionnaire_id !== questionnaireID ||
    !slug(value.slug) ||
    (expectedSlug !== undefined && value.slug !== expectedSlug) ||
    !positive(value.definition_version) ||
    (expectedVersion !== undefined && value.definition_version !== expectedVersion) ||
    value.state !== expectedState
  ) return undefined;
  return {
    questionnaireID: value.questionnaire_id,
    slug: value.slug,
    definitionVersion: value.definition_version,
    state: value.state as "public" | "disabled",
  };
}

export function parsePublicSurveyAnalytics(
  value: unknown,
  questionnaireID: number,
  definitionVersion: number | undefined,
): PublicSurveyAnalytics | undefined {
  if (
    !record(value) ||
    !exact(value, ["questionnaire_id", "definition_version", "slug", "state", "submission_count", "questions"]) ||
    value.questionnaire_id !== questionnaireID ||
    (definitionVersion !== undefined && value.definition_version !== definitionVersion) ||
    !positive(value.questionnaire_id) ||
    !positive(value.definition_version) ||
    !slug(value.slug) ||
    (value.state !== "public" && value.state !== "disabled") ||
    !nonnegative(value.submission_count) ||
    !Array.isArray(value.questions) || value.questions.length < 1 || value.questions.length > 100
  ) return undefined;
  const questionIDs = new Set<number>();
  const questions: Array<PublicSurveyAnalytics["questions"][number]> = [];
  for (let index = 0; index < value.questions.length; index++) {
    const question = value.questions[index];
    if (
      !record(question) ||
      !exact(question, ["question_id", "type", "sort_order", "answered_count", "options"]) ||
      !positive(question.question_id) || questionIDs.has(question.question_id) ||
      (question.type !== "single_choice" && question.type !== "multi_choice") ||
      question.sort_order !== index ||
      !nonnegative(question.answered_count) || question.answered_count > value.submission_count ||
      !Array.isArray(question.options) || question.options.length < 1 || question.options.length > 100
    ) return undefined;
    questionIDs.add(question.question_id);
    const optionIDs = new Set<number>();
    const options: Array<PublicSurveyAnalytics["questions"][number]["options"][number]> = [];
    for (let optionIndex = 0; optionIndex < question.options.length; optionIndex++) {
      const option = question.options[optionIndex];
      if (
        !record(option) ||
        !exact(option, ["option_id", "sort_order", "selection_count"]) ||
        !positive(option.option_id) || optionIDs.has(option.option_id) ||
        option.sort_order !== optionIndex ||
        !nonnegative(option.selection_count) || option.selection_count > question.answered_count
      ) return undefined;
      optionIDs.add(option.option_id);
      options.push({ optionID: option.option_id, sortOrder: option.sort_order, selectionCount: option.selection_count });
    }
    questions.push({
      questionID: question.question_id,
      type: question.type,
      sortOrder: question.sort_order,
      answeredCount: question.answered_count,
      options,
    });
  }
  return { questionnaireID, definitionVersion: value.definition_version, slug: value.slug, state: value.state, submissionCount: value.submission_count, questions };
}

export async function loadQuestionnairePublicAnalytics(
  transport: QuestionnairePublicAnalyticsTransport,
  questionnaireID: number,
  definitionVersion?: number,
): Promise<PublicSurveyAnalyticsResult> {
  if (!positive(questionnaireID) || (definitionVersion !== undefined && !positive(definitionVersion))) return { status: "invalid" };
  try {
    const response = await transport.analytics(questionnaireID, { definition_version: definitionVersion }, { credentials: "same-origin" });
    if (response.status !== 200) return { status: failure(response.status) };
    const analytics = parsePublicSurveyAnalytics(response.data, questionnaireID, definitionVersion);
    return analytics ? { status: "loaded", analytics } : { status: "invalid" };
  } catch {
    return { status: "unavailable" };
  }
}

async function mutateQuestionnairePublicDefinition(
  operation: "publish" | "disable",
  transport: QuestionnairePublicAnalyticsTransport,
  questionnaireID: number,
  slugValue: string,
  expectedVersion: number,
  csrf: string,
  idempotencyKey: string,
): Promise<PublicSurveyManagementResult> {
  if (!positive(questionnaireID) || !slug(slugValue) || !positive(expectedVersion) || !validCSRF(csrf) || !validIdempotencyKey(idempotencyKey)) return { status: "invalid" };
  const options: RequestInit = {
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrf, "Idempotency-Key": idempotencyKey },
  };
  try {
    const response = operation === "publish"
      ? await transport.publish(questionnaireID, { expected_questionnaire_version: expectedVersion }, options)
      : await transport.disable(questionnaireID, { expected_definition_version: expectedVersion }, options);
    if (response.status !== 200) return { status: failure(response.status) };
    const receipt = parsePublicSurveyManagementReceipt(
      response.data,
      questionnaireID,
      operation === "publish" ? slugValue : undefined,
      operation === "publish" ? undefined : expectedVersion,
      operation === "publish" ? "public" : "disabled",
    );
    // A malformed 200 can follow a committed write, so it is outcome-unknown.
    return receipt ? { status: "saved", receipt } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

export function publishQuestionnairePublicDefinition(
  transport: QuestionnairePublicAnalyticsTransport,
  questionnaireID: number,
  slugValue: string,
  expectedVersion: number,
  csrf: string,
  idempotencyKey: string,
): Promise<PublicSurveyManagementResult> {
  return mutateQuestionnairePublicDefinition("publish", transport, questionnaireID, slugValue, expectedVersion, csrf, idempotencyKey);
}

export function disableQuestionnairePublicDefinition(
  transport: QuestionnairePublicAnalyticsTransport,
  questionnaireID: number,
  slugValue: string,
  expectedVersion: number,
  csrf: string,
  idempotencyKey: string,
): Promise<PublicSurveyManagementResult> {
  return mutateQuestionnairePublicDefinition("disable", transport, questionnaireID, slugValue, expectedVersion, csrf, idempotencyKey);
}
