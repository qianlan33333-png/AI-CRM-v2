import { describe, expect, it, vi } from "vitest";
import {
  deleteQuestionnaire,
  loadQuestionnaires,
  nextQuestionnaireOffset,
  previousQuestionnaireOffset,
  questionnaireMutationReloadOffset,
  setQuestionnaireEnabled,
  type QuestionnaireItem,
  type QuestionnaireListTransport,
} from "./questionnaire-list";

const item = {
  id: 41,
  name: "welcome",
  title: "欢迎问卷",
  description: "",
  answer_display_mode: "all_in_one",
  assessment_enabled: false,
  assessment_config: {},
  slug: "welcome",
  is_disabled: false,
  questions: [
    {
      id: 1,
      type: "single_choice",
      title: "目标",
      assessment_dimension_key: "",
      sidebar_profile_field: "",
      required: true,
      sort_order: 0,
      placeholder_text: "",
      options: [
        {
          id: 1,
          option_text: "增长",
          score: 0,
          assessment_type_key: "",
          tag_codes: [],
          is_other: false,
          other_placeholder: "",
          other_max_length: 0,
          sort_order: 0,
        },
      ],
    },
  ],
  score_rules: [],
  enabled: true,
  status: "active",
  version: 1,
  question_count: 1,
  submission_count: 0,
  created_at: "2026-08-19T00:00:00Z",
  updated_at: "2026-08-19T00:00:00Z",
  public_path: "/q/welcome",
  submitted_path: "/admin/questionnaires/41/submissions",
};
function responseQuestionnaire(disabled: boolean) {
  return {
    ...item,
    is_disabled: disabled,
    enabled: !disabled,
    status: disabled ? "disabled" : "active",
  };
}
function mutationResponse(
  disabled: boolean,
  write_model_status: "enabled" | "disabled" | "deleted",
  extra: Record<string, unknown> = {},
) {
  const questionnaire = responseQuestionnaire(disabled);
  return {
    ok: true,
    questionnaire_id: item.id,
    questionnaire,
    questions: questionnaire.questions,
    data: { questionnaire },
    write_model_status,
    ...extra,
  };
}
function transport(
  overrides: Partial<QuestionnaireListTransport> = {},
): QuestionnaireListTransport {
  return {
    list: vi.fn(async () => ({
      status: 200,
      data: {
        ok: true,
        questionnaires: [item],
        items: [item],
        data: { questionnaires: [item] },
        total: 1,
        limit: 50,
        offset: 0,
      },
    })),
    disable: vi.fn(async (_id, body) => ({
      status: 200,
      data: mutationResponse(
        body.is_disabled,
        body.is_disabled ? "disabled" : "enabled",
      ),
    })),
    remove: vi.fn(async () => ({
      status: 200,
      data: mutationResponse(true, "deleted", {
        deleted: true,
        delete_mode: "hard_delete",
      }),
    })),
    ...overrides,
  } as unknown as QuestionnaireListTransport;
}
const parsed: QuestionnaireItem = {
  id: 41,
  name: "welcome",
  title: "欢迎问卷",
  isDisabled: false,
  status: "active",
  questionCount: 1,
  submissionCount: 0,
  updatedAt: "2026-08-19T00:00:00Z",
};

describe("questionnaire list transport", () => {
  it("strictly accepts the frozen list envelope and paging", async () => {
    const client = transport();
    await expect(loadQuestionnaires(client)).resolves.toEqual({
      status: "loaded",
      items: [parsed],
      total: 1,
      limit: 50,
      offset: 0,
    });
    expect(client.list).toHaveBeenCalledWith(
      { limit: 50, offset: 0 },
      { credentials: "same-origin" },
    );
    expect(previousQuestionnaireOffset(0)).toBe(0);
    expect(previousQuestionnaireOffset(50)).toBe(0);
    expect(nextQuestionnaireOffset(0, 3, 10)).toBe(3);
    expect(nextQuestionnaireOffset(3, 7, 10)).toBeUndefined();
    expect(nextQuestionnaireOffset(0, 0, 1)).toBeUndefined();
    expect(questionnaireMutationReloadOffset({ status: "saved" })).toBe(0);
    expect(
      questionnaireMutationReloadOffset({ status: "conflict" }),
    ).toBeUndefined();
  });
  it("fails closed for malformed DTOs and auth errors", async () => {
    await expect(
      loadQuestionnaires(
        transport({
          list: vi.fn(async () => ({
            status: 200,
            data: {
              ok: true,
              questionnaires: [],
              items: [],
              data: {},
              total: 0,
              limit: 50,
              offset: 0,
            },
          })) as never,
        }),
      ),
    ).resolves.toEqual({ status: "invalid" });
    for (const malicious of [
      { ...item, assessment_config: { unexpected: true } },
      { ...item, assessment_enabled: true },
      { ...item, created_at: "2026-08-19" },
      {
        ...item,
        questions: [
          {
            ...item.questions[0],
            options: [
              { ...item.questions[0].options[0], other_max_length: 2001 },
            ],
          },
        ],
      },
    ]) {
      await expect(
        loadQuestionnaires(
          transport({
            list: vi.fn(async () => ({
              status: 200,
              data: {
                ok: true,
                questionnaires: [malicious],
                items: [malicious],
                data: { questionnaires: [malicious] },
                total: 1,
                limit: 50,
                offset: 0,
              },
            })) as never,
          }),
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
    const wrongCount = { ...item, question_count: 2 };
    await expect(
      loadQuestionnaires(
        transport({
          list: vi.fn(async () => ({
            status: 200,
            data: {
              ok: true,
              questionnaires: [wrongCount],
              items: [wrongCount],
              data: { questionnaires: [wrongCount] },
              total: 1,
              limit: 50,
              offset: 0,
            },
          })) as never,
        }),
      ),
    ).resolves.toEqual({ status: "invalid" });
    await expect(
      loadQuestionnaires(
        transport({
          list: vi.fn(async () => ({ status: 401, data: {} })) as never,
        }),
      ),
    ).resolves.toEqual({ status: "unauthenticated" });
  });
  it("uses only disable for both state transitions, validates success, and never deletes an active questionnaire", async () => {
    const client = transport();
    await expect(
      setQuestionnaireEnabled(client, parsed, "x".repeat(43)),
    ).resolves.toEqual({ status: "saved" });
    expect(client.disable).toHaveBeenCalledWith(
      41,
      { is_disabled: true },
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.objectContaining({
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key": expect.stringMatching(/^questionnaire-disable-/),
        }),
      }),
    );
    const disabled = {
      ...parsed,
      isDisabled: true,
      status: "disabled" as const,
    };
    await expect(
      setQuestionnaireEnabled(client, disabled, "y".repeat(43)),
    ).resolves.toEqual({ status: "saved" });
    expect(client.disable).toHaveBeenCalledWith(
      41,
      { is_disabled: false },
      expect.objectContaining({
        headers: expect.objectContaining({
          "Idempotency-Key": expect.stringMatching(/^questionnaire-enable-/),
        }),
      }),
    );
    await expect(
      deleteQuestionnaire(client, disabled, "x".repeat(43)),
    ).resolves.toEqual({ status: "saved" });
    await expect(
      deleteQuestionnaire(client, parsed, "x".repeat(43)),
    ).resolves.toEqual({ status: "invalid" });
    expect(client.remove).toHaveBeenCalledTimes(1);
  });
  it("fails closed when a 200 mutation response does not prove the requested change", async () => {
    const client = transport({
      disable: vi.fn(async () => ({ status: 200, data: {} })) as never,
    });
    await expect(
      setQuestionnaireEnabled(client, parsed, "x".repeat(43)),
    ).resolves.toEqual({ status: "invalid" });
    const disabled = {
      ...parsed,
      isDisabled: true,
      status: "disabled" as const,
    };
    const deleteClient = transport({
      remove: vi.fn(async () => ({
        status: 200,
        data: mutationResponse(true, "deleted", {
          deleted: false,
          delete_mode: "hard_delete",
        }),
      })) as never,
    });
    await expect(
      deleteQuestionnaire(deleteClient, disabled, "x".repeat(43)),
    ).resolves.toEqual({ status: "invalid" });
    const enabledDeleteClient = transport({
      remove: vi.fn(async () => ({
        status: 200,
        data: mutationResponse(false, "deleted", {
          deleted: true,
          delete_mode: "hard_delete",
        }),
      })) as never,
    });
    await expect(
      deleteQuestionnaire(enabledDeleteClient, disabled, "x".repeat(43)),
    ).resolves.toEqual({ status: "invalid" });
  });
});
