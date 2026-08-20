import { describe, expect, it, vi } from "vitest";
import {
  disableQuestionnairePublicDefinition,
  loadQuestionnairePublicAnalytics,
  parsePublicSurveyAnalytics,
  publishQuestionnairePublicDefinition,
} from "./questionnaire-public-analytics";

const csrf = "x".repeat(43);
const key = "questionnaire-public-publish:12345678-1234-4123-8123-123456789abc";
const analytics = {
  questionnaire_id: 41,
  definition_version: 2,
  slug: "welcome",
  state: "public",
  submission_count: 3,
  questions: [{
    question_id: 11,
    type: "single_choice",
    sort_order: 0,
    answered_count: 3,
    options: [
      { option_id: 101, sort_order: 0, selection_count: 2 },
      { option_id: 102, sort_order: 1, selection_count: 1 },
    ],
  }],
};

function transport(overrides: Record<string, unknown> = {}) {
  return {
    analytics: vi.fn(async () => ({ status: 200, data: analytics })),
    publish: vi.fn(async () => ({ status: 200, data: { questionnaire_id: 41, slug: "welcome", definition_version: 2, state: "public" } })),
    disable: vi.fn(async () => ({ status: 200, data: { questionnaire_id: 41, slug: "welcome", definition_version: 2, state: "disabled" } })),
    ...overrides,
  };
}

describe("questionnaire public analytics transport", () => {
  it("reads only the ordered local count projection over same-origin transport", async () => {
    const client = transport();
    await expect(loadQuestionnairePublicAnalytics(client as never, 41, 2)).resolves.toEqual({
      status: "loaded",
      analytics: {
        questionnaireID: 41,
        definitionVersion: 2,
        slug: "welcome",
        state: "public",
        submissionCount: 3,
        questions: [{ questionID: 11, type: "single_choice", sortOrder: 0, answeredCount: 3, options: [{ optionID: 101, sortOrder: 0, selectionCount: 2 }, { optionID: 102, sortOrder: 1, selectionCount: 1 }] }],
      },
    });
    expect(client.analytics).toHaveBeenCalledWith(41, { definition_version: 2 }, { credentials: "same-origin" });
    await expect(loadQuestionnairePublicAnalytics(client as never, 41)).resolves.toMatchObject({
      status: "loaded",
      analytics: { definitionVersion: 2 },
    });
    expect(client.analytics).toHaveBeenLastCalledWith(41, { definition_version: undefined }, { credentials: "same-origin" });
  });

  it("fails closed for extra fields, duplicate ids, or unordered local counts", () => {
    for (const value of [
      { ...analytics, identity: "forbidden" },
      { ...analytics, questions: [{ ...analytics.questions[0], question_id: 11 }, { ...analytics.questions[0], sort_order: 1 }] },
      { ...analytics, questions: [{ ...analytics.questions[0], options: [{ ...analytics.questions[0].options[0], sort_order: 1 }] }] },
    ]) expect(parsePublicSurveyAnalytics(value, 41, 2)).toBeUndefined();
  });

  it("requires exact lifecycle receipts and sends CSRF plus one idempotency key", async () => {
    const client = transport({ publish: vi.fn(async () => ({ status: 200, data: { questionnaire_id: 41, slug: "welcome", definition_version: 7, state: "public" } })) });
    await expect(publishQuestionnairePublicDefinition(client as never, 41, "welcome", 2, csrf, key)).resolves.toEqual({
      status: "saved",
      receipt: { questionnaireID: 41, slug: "welcome", definitionVersion: 7, state: "public" },
    });
    expect(client.publish).toHaveBeenCalledWith(41, { expected_questionnaire_version: 2 }, {
      credentials: "same-origin",
      headers: { "X-CSRF-Token": csrf, "Idempotency-Key": key },
    });
    await expect(disableQuestionnairePublicDefinition(transport({ disable: vi.fn(async () => ({ status: 200, data: { questionnaire_id: 41, slug: "original-slug", definition_version: 2, state: "disabled" } })) }) as never, 41, "edited-slug", 2, csrf, key.replace("publish", "disable"))).resolves.toEqual({ status: "saved", receipt: { questionnaireID: 41, slug: "original-slug", definitionVersion: 2, state: "disabled" } });
    await expect(disableQuestionnairePublicDefinition(transport({ disable: vi.fn(async () => ({ status: 200, data: { questionnaire_id: 41, slug: "Unsafe Slug", definition_version: 2, state: "disabled" } })) }) as never, 41, "edited-slug", 2, csrf, key.replace("publish", "disable"))).resolves.toEqual({ status: "unavailable" });
  });
});
