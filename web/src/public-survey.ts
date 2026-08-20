/* eslint-disable no-unused-vars -- named transport and callback parameters document the strict public wire contract. */
import {
  getPublicSurveyDefinition,
  queryPublicSurveySubmissionResult,
  submitPublicSurvey,
} from "./api/generated/health";

export type PublicQuestionType = "single_choice" | "multi_choice";
export type PublicDefinition = { readonly id: number; readonly slug: string; readonly title: string; readonly description: string; readonly answer_display_mode: "all_in_one" | "one_by_one"; readonly version: number; readonly questions: readonly PublicQuestion[] };
export type PublicQuestion = { readonly id: number; readonly type: PublicQuestionType; readonly title: string; readonly required: boolean; readonly sort_order: number; readonly minimum_selections: number; readonly maximum_selections: number; readonly options: readonly PublicOption[] };
export type PublicOption = { readonly id: number; readonly option_text: string; readonly sort_order: number };
export type PublicAnswer = { readonly question_id: number; readonly option_ids: readonly number[] };
export type PublicSubmissionReceipt = { readonly questionnaire_id: number; readonly questionnaire_slug: string; readonly definition_version: number; readonly submission_id: number };
export type SubmissionResult = { readonly submission_id: number; readonly definition_version: number; readonly submitted_at: string; readonly local_only: true; readonly external_executed: false };
export interface PublicSurveyTransport {
  definition(slug: string): Promise<PublicDefinition>;
  submit(slug: string, input: { readonly version: number; readonly submission_key: string; readonly answers: readonly PublicAnswer[] }): Promise<{ readonly receipt: PublicSubmissionReceipt; readonly result_token: string }>;
  result(resultToken: string): Promise<SubmissionResult>;
}

type UnknownRecord = Record<string, unknown>;
const SLUG = /^[a-z0-9][a-z0-9-]{0,119}$/;
const TOKEN = /^[A-Za-z0-9_-]{43}$/;
function record(value: unknown): value is UnknownRecord { return typeof value === "object" && value !== null && !Array.isArray(value); }
function exact(value: UnknownRecord, keys: readonly string[]): boolean { const actual = Object.keys(value); return actual.length === keys.length && actual.every((key) => keys.includes(key)); }
function positive(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value > 0; }
function order(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 && value <= 99; }
function text(value: unknown, minimum: number, maximum: number): value is string { return typeof value === "string" && value.length >= minimum && value.length <= maximum; }
function rfc3339(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.exec(value);
  if (!match) return false;
  const [year, month, day, hour, minute, second] = match.slice(1, 7).map(Number);
  if (hour > 23 || minute > 59 || second > 59) return false;
  const calendar = new Date(0); calendar.setUTCFullYear(year, month - 1, day); calendar.setUTCHours(hour, minute, second, 0);
  return calendar.getUTCFullYear() === year && calendar.getUTCMonth() === month - 1 && calendar.getUTCDate() === day && calendar.getUTCHours() === hour && calendar.getUTCMinutes() === minute && calendar.getUTCSeconds() === second && Number.isFinite(Date.parse(value));
}

export function publicSlug(pathname: string): string | null { const match = /^\/q\/([a-z0-9][a-z0-9-]{0,119})$/.exec(pathname); return match ? match[1] : null; }

export function parsePublicDefinition(value: unknown): PublicDefinition | undefined {
  if (!record(value) || !exact(value, ["id", "slug", "title", "description", "answer_display_mode", "version", "questions"]) || !positive(value.id) || !text(value.slug, 1, 120) || !SLUG.test(value.slug) || !text(value.title, 1, 300) || !text(value.description, 0, 10_000) || (value.answer_display_mode !== "all_in_one" && value.answer_display_mode !== "one_by_one") || !positive(value.version) || !Array.isArray(value.questions) || value.questions.length < 1 || value.questions.length > 100) return undefined;
  const ids = new Set<number>(); const optionIDs = new Set<number>(); const questions: PublicQuestion[] = [];
  for (const [index, item] of value.questions.entries()) {
    if (!record(item) || !exact(item, ["id", "type", "title", "required", "sort_order", "minimum_selections", "maximum_selections", "options"]) || !positive(item.id) || ids.has(item.id) || (item.type !== "single_choice" && item.type !== "multi_choice") || !text(item.title, 1, 500) || typeof item.required !== "boolean" || !order(item.sort_order) || item.sort_order !== index || !order(item.minimum_selections) || !positive(item.maximum_selections) || item.maximum_selections > 100 || item.minimum_selections > item.maximum_selections || (item.type === "single_choice" && item.maximum_selections !== 1) || (item.required && item.minimum_selections === 0) || !Array.isArray(item.options) || item.options.length < 1 || item.options.length > 100 || item.maximum_selections > item.options.length) return undefined;
    ids.add(item.id); const options: PublicOption[] = [];
    for (const [optionIndex, option] of item.options.entries()) {
      if (!record(option) || !exact(option, ["id", "option_text", "sort_order"]) || !positive(option.id) || optionIDs.has(option.id) || !text(option.option_text, 1, 500) || !order(option.sort_order) || option.sort_order !== optionIndex) return undefined;
      optionIDs.add(option.id); options.push({ id: option.id, option_text: option.option_text, sort_order: option.sort_order });
    }
    questions.push({ id: item.id, type: item.type, title: item.title, required: item.required, sort_order: item.sort_order, minimum_selections: item.minimum_selections, maximum_selections: item.maximum_selections, options });
  }
  return { id: value.id, slug: value.slug, title: value.title, description: value.description, answer_display_mode: value.answer_display_mode, version: value.version, questions };
}

function parseReceipt(value: unknown, definition: PublicDefinition): PublicSubmissionReceipt | undefined {
  if (!record(value) || !exact(value, ["questionnaire_id", "questionnaire_slug", "definition_version", "submission_id"]) || !positive(value.questionnaire_id) || !text(value.questionnaire_slug, 1, 120) || !SLUG.test(value.questionnaire_slug) || !positive(value.definition_version) || !positive(value.submission_id) || value.questionnaire_id !== definition.id || value.questionnaire_slug !== definition.slug || value.definition_version !== definition.version) return undefined;
  return { questionnaire_id: value.questionnaire_id, questionnaire_slug: value.questionnaire_slug, definition_version: value.definition_version, submission_id: value.submission_id };
}
export function parsePublicSubmissionResponse(value: unknown, definition: PublicDefinition): { readonly receipt: PublicSubmissionReceipt; readonly result_token: string } | undefined {
  if (!record(value) || !exact(value, ["receipt", "result_token"]) || !text(value.result_token, 43, 43) || !TOKEN.test(value.result_token)) return undefined;
  const receipt = parseReceipt(value.receipt, definition); return receipt ? { receipt, result_token: value.result_token } : undefined;
}
export function parsePublicResult(value: unknown, receipt: PublicSubmissionReceipt): SubmissionResult | undefined {
  if (!record(value) || !exact(value, ["submission_id", "definition_version", "submitted_at", "local_only", "external_executed"]) || !positive(value.submission_id) || !positive(value.definition_version) || !rfc3339(value.submitted_at) || value.local_only !== true || value.external_executed !== false || value.submission_id !== receipt.submission_id || value.definition_version !== receipt.definition_version) return undefined;
  return { submission_id: value.submission_id, definition_version: value.definition_version, submitted_at: value.submitted_at, local_only: true, external_executed: false };
}

export class PublicSurveyInputError extends Error {}
export class PublicSurveyFailure extends Error {
  constructor(
    readonly kind: "invalid" | "not_found" | "conflict" | "rate_limited" | "unknown",
  ) {
    super(kind);
  }
}
export function normalizePublicAnswers(definition: PublicDefinition, answers: readonly PublicAnswer[]): readonly PublicAnswer[] {
  if (answers.length > definition.questions.length) throw new PublicSurveyInputError("invalid answers");
  const questions = new Map(definition.questions.map((question) => [question.id, question])); const seen = new Set<number>(); const normalized: PublicAnswer[] = [];
  for (const answer of answers) {
    const question = questions.get(answer.question_id);
    if (!question || !positive(answer.question_id) || seen.has(answer.question_id) || !Array.isArray(answer.option_ids) || answer.option_ids.length < question.minimum_selections || answer.option_ids.length > question.maximum_selections) throw new PublicSurveyInputError("invalid answers");
    const options = new Set(question.options.map((option) => option.id)); const selected = new Set<number>();
    for (const id of answer.option_ids) { if (!positive(id) || !options.has(id) || selected.has(id)) throw new PublicSurveyInputError("invalid answers"); selected.add(id); }
    seen.add(answer.question_id); normalized.push({ question_id: answer.question_id, option_ids: [...selected].sort((a, b) => a - b) });
  }
  if (definition.questions.some((question) => question.required && !seen.has(question.id))) throw new PublicSurveyInputError("missing required answer");
  return normalized.sort((a, b) => a.question_id - b.question_id);
}

export const generatedPublicSurveyTransport: PublicSurveyTransport = {
  async definition(slug) { const response = await getPublicSurveyDefinition(slug, { credentials: "same-origin" }); const definition = response.status === 200 ? parsePublicDefinition(response.data) : undefined; if (!definition || definition.slug !== slug) throw new Error("public definition unavailable"); return definition; },
  async submit(slug, input) {
    const response = await submitPublicSurvey(slug, { ...input, answers: input.answers.map((answer) => ({ question_id: answer.question_id, option_ids: [...answer.option_ids] })) }, { credentials: "same-origin" });
    if (response.status === 400) throw new PublicSurveyFailure("invalid");
    if (response.status === 404) throw new PublicSurveyFailure("not_found");
    if (response.status === 409) throw new PublicSurveyFailure("conflict");
    if (response.status === 429) throw new PublicSurveyFailure("rate_limited");
    if (response.status !== 202) throw new PublicSurveyFailure("unknown");
    return response.data as { readonly receipt: PublicSubmissionReceipt; readonly result_token: string };
  },
  async result(resultToken) { const response = await queryPublicSurveySubmissionResult({ result_token: resultToken }, { credentials: "same-origin" }); if (response.status !== 200) throw new Error("public result unavailable"); return response.data as SubmissionResult; },
};

export function createPublicSurveyController(transport: PublicSurveyTransport, pathname: string) {
  const slug = publicSlug(pathname); let definition: PublicDefinition | undefined; let receipt: PublicSubmissionReceipt | undefined; let token: string | undefined;
  return {
    async load() { if (!slug) throw new PublicSurveyInputError("invalid public survey path"); const next = await transport.definition(slug); if (!next || next.slug !== slug) throw new Error("public definition unavailable"); definition = next; return next; },
    async submit(input: { readonly version: number; readonly submission_key: string; readonly answers: readonly PublicAnswer[] }) { if (!slug || !definition || input.version !== definition.version || !TOKEN.test(input.submission_key)) throw new PublicSurveyInputError("invalid public submission"); const answers = normalizePublicAnswers(definition, input.answers); const response = await transport.submit(slug, { ...input, answers }); const parsed = parsePublicSubmissionResponse(response, definition); if (!parsed) throw new Error("public receipt unavailable"); receipt = parsed.receipt; token = parsed.result_token; return receipt; },
    async result() { if (!token || !receipt) throw new PublicSurveyInputError("missing public result token"); const parsed = parsePublicResult(await transport.result(token), receipt); if (!parsed) throw new Error("public result unavailable"); token = undefined; return parsed; },
  };
}

export function createPublicSubmissionFlight(makeKey: () => string) {
  let key: string | undefined; let active: Promise<unknown> | undefined; let unknown = false;
  return {
    submit<T>(run: (submissionKey: string) => Promise<T>): Promise<T> {
      if (unknown) return Promise.reject(new Error("submission status unknown"));
      if (active) return active as Promise<T>;
      key ??= makeKey(); active = run(key).catch((error: unknown) => { if (!(error instanceof PublicSurveyInputError) && !(error instanceof PublicSurveyFailure && error.kind !== "unknown")) unknown = true; throw error; }).finally(() => { active = undefined; });
      return active as Promise<T>;
    },
    invalidate() { unknown = true; active = undefined; },
    get submissionKey() { return key; },
    get unknown() { return unknown; },
  };
}
