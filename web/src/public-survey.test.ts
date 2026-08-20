import { describe, expect, it, vi } from "vitest";
import {
  createPublicSubmissionFlight,
  createPublicSurveyController,
  normalizePublicAnswers,
  parsePublicDefinition,
  parsePublicResult,
  parsePublicSubmissionResponse,
  PublicSurveyFailure,
  publicSlug,
  type PublicDefinition,
} from "./public-survey";

const definition = (): PublicDefinition => ({
  id: 1, slug: "public-1", title: "匿名问卷", description: "", answer_display_mode: "all_in_one", version: 2,
  questions: [{ id: 11, type: "single_choice", title: "选择", required: true, sort_order: 0, minimum_selections: 1, maximum_selections: 1, options: [{ id: 21, option_text: "是", sort_order: 0 }, { id: 22, option_text: "否", sort_order: 1 }] }],
});
const rawDefinition = () => ({ id: 1, slug: "public-1", title: "匿名问卷", description: "", answer_display_mode: "all_in_one", version: 2, questions: [{ id: 11, type: "single_choice", title: "选择", required: true, sort_order: 0, minimum_selections: 1, maximum_selections: 1, options: [{ id: 21, option_text: "是", sort_order: 0 }, { id: 22, option_text: "否", sort_order: 1 }] }] });
const receipt = { questionnaire_id: 1, questionnaire_slug: "public-1", definition_version: 2, submission_id: 3 };

describe("public survey consumer", () => {
  it("fails closed for extra fields, unordered definition, invalid selection bounds, and malformed result mirrors", () => {
    expect(parsePublicDefinition(rawDefinition())).toEqual(definition());
    expect(parsePublicDefinition({ ...rawDefinition(), extra: true })).toBeUndefined();
    expect(parsePublicDefinition({ ...rawDefinition(), questions: [{ ...rawDefinition().questions[0], sort_order: 1 }] })).toBeUndefined();
    expect(parsePublicDefinition({ ...rawDefinition(), questions: [{ ...rawDefinition().questions[0], maximum_selections: 3 }] })).toBeUndefined();
    expect(parsePublicSubmissionResponse({ receipt, result_token: "a".repeat(43) }, definition())).toEqual({ receipt, result_token: "a".repeat(43) });
    expect(parsePublicSubmissionResponse({ receipt: { ...receipt, questionnaire_slug: "other" }, result_token: "a".repeat(43) }, definition())).toBeUndefined();
    expect(parsePublicResult({ submission_id: 3, definition_version: 2, submitted_at: "2026-02-30T01:00:00Z", local_only: true, external_executed: false }, receipt)).toBeUndefined();
    expect(parsePublicResult({ submission_id: 3, definition_version: 2, submitted_at: "2026-02-28T01:00:00+08:00", local_only: true, external_executed: false }, receipt)).toBeDefined();
  });

  it("keeps the result token in controller closure and sends normalized, local-only requests", async () => {
    const transport = {
      definition: vi.fn().mockResolvedValue(definition()),
      submit: vi.fn().mockResolvedValue({ receipt, result_token: "a".repeat(43) }),
      result: vi.fn().mockResolvedValue({ submission_id: 3, definition_version: 2, submitted_at: "2026-02-28T01:00:00Z", local_only: true, external_executed: false }),
    };
    const controller = createPublicSurveyController(transport, "/q/public-1");
    await controller.load();
    await controller.submit({ version: 2, submission_key: "b".repeat(43), answers: [{ question_id: 11, option_ids: [21] }] });
    await controller.result();
    expect(transport.submit).toHaveBeenCalledWith("public-1", { version: 2, submission_key: "b".repeat(43), answers: [{ question_id: 11, option_ids: [21] }] });
    expect(transport.result).toHaveBeenCalledWith("a".repeat(43));
    expect(controller).not.toHaveProperty("result_token");
    expect(() => normalizePublicAnswers(definition(), [])).toThrow("missing required answer");
  });

  it("shares one same-tick request, keeps deterministic keys, and locks only uncertain outcomes", async () => {
    let resolve!: () => void;
    const pending = new Promise<void>((done) => { resolve = done; });
    const flight = createPublicSubmissionFlight(() => "k".repeat(43));
    const send = vi.fn(() => pending);
    const first = flight.submit(send); const second = flight.submit(send);
    expect(first).toBe(second); expect(send).toHaveBeenCalledOnce(); resolve(); await first;
    expect(flight.submissionKey).toBe("k".repeat(43));
    const unknown = createPublicSubmissionFlight(() => "x".repeat(43));
    await expect(unknown.submit(() => Promise.reject(new Error("network")))).rejects.toThrow("network");
    await expect(unknown.submit(() => Promise.resolve())).rejects.toThrow("unknown");
    const deterministic = createPublicSubmissionFlight(() => "z".repeat(43));
    await expect(deterministic.submit(() => Promise.reject(new PublicSurveyFailure("invalid")))).rejects.toThrow("invalid");
    await expect(deterministic.submit(() => Promise.resolve("later"))).resolves.toBe("later");
    expect(publicSlug("/q/public-1")).toBe("public-1");
    expect(publicSlug("/q/public-1?result_token=x")).toBeNull();
  });
});
