import { describe, expect, it, vi } from "vitest";
import {
  deleteQuestionnaire,
  duplicateQuestionnaire,
  loadQuestionnaires,
  loadQuestionnairePreflight,
  loadQuestionnaireResults,
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
function responseQuestionnaire(
  disabled: boolean,
  id = item.id,
  slug = item.slug,
) {
  return {
    ...item,
    id,
    slug,
    public_path: `/q/${slug}`,
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
function duplicateMutationResponse(
  extra: Record<string, unknown> = {},
  disabled = true,
) {
  const questionnaire = responseQuestionnaire(disabled, 42, "welcome-copy");
  return {
    ok: true,
    questionnaire_id: questionnaire.id,
    source_questionnaire_id: item.id,
    questionnaire,
    questions: questionnaire.questions,
    data: { questionnaire },
    write_model_status: "duplicated",
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
    duplicate: vi.fn(async () => ({
      status: 200,
      data: duplicateMutationResponse(),
    })),
    remove: vi.fn(async () => ({
      status: 200,
      data: mutationResponse(true, "deleted", {
        deleted: true,
        delete_mode: "hard_delete",
      }),
    })),
    preflight: vi.fn(async () => ({
      status: 200,
      data: {
        ok: true,
        checks: {
          wechat_oauth_configured: false,
          wecom_contact_configured: false,
          debug_session_api_enabled: false,
          wecom_tags_api_available: false,
          questionnaire_admin_ui_enabled: true,
          identity_map_available: false,
        },
        status: "partial",
      },
    })),
    results: vi.fn(async () => ({ status: 200, data: resultsResponse() })),
    ...overrides,
  } as unknown as QuestionnaireListTransport;
}
function resultsResponse(extra: Record<string, unknown> = {}) {
  const results = {
    submission_count: 0,
    latest_submitted_at: null,
    average_score: 0,
    rules: [],
  };
  return {
    ok: true,
    questionnaire_id: item.id,
    results,
    data: { results },
    side_effect_executed: false,
    ...extra,
  };
}
function listResponse(questionnaire: Record<string, unknown>) {
  return {
    status: 200,
    data: {
      ok: true,
      questionnaires: [questionnaire],
      items: [questionnaire],
      data: { questionnaires: [questionnaire] },
      total: 1,
      limit: 50,
      offset: 0,
    },
  };
}
const parsed: QuestionnaireItem = {
  id: 41,
  name: "welcome",
  title: "欢迎问卷",
  publicPath: "/q/welcome",
  isDisabled: false,
  status: "active",
  questionCount: 1,
  submissionCount: 0,
  updatedAt: "2026-08-19T00:00:00Z",
};

describe("questionnaire list transport", () => {
  it("reads only the strict local submission aggregate with no query or write options", async () => {
    const client = transport();
    await expect(loadQuestionnaireResults(client, parsed)).resolves.toEqual({
      status: "loaded",
      aggregate: {
        submissionCount: 0,
        latestSubmittedAt: null,
        averageScore: 0,
      },
    });
    expect(client.results).toHaveBeenCalledWith(41, {
      credentials: "same-origin",
    });
    expect(client.list).not.toHaveBeenCalled();
    expect(client.disable).not.toHaveBeenCalled();
    expect(client.duplicate).not.toHaveBeenCalled();
    expect(client.remove).not.toHaveBeenCalled();
  });

  it("fails closed for PII, unfrozen aggregate shapes, side effects, and invalid result invariants", async () => {
    const aggregate = {
      submission_count: 2,
      latest_submitted_at: "2026-08-19T01:02:03Z",
      average_score: 1.5,
      rules: [],
    };
    const accepted = resultsResponse({
      results: aggregate,
      data: { results: aggregate },
    });
    await expect(
      loadQuestionnaireResults(
        transport({
          results: vi.fn(async () => ({
            status: 200,
            data: accepted,
          })) as never,
        }),
        parsed,
      ),
    ).resolves.toEqual({
      status: "loaded",
      aggregate: {
        submissionCount: 2,
        latestSubmittedAt: "2026-08-19T01:02:03Z",
        averageScore: 1.5,
      },
    });

    for (const data of [
      resultsResponse({ side_effect_executed: true }),
      resultsResponse({ questionnaire_id: 42 }),
      resultsResponse({
        data: { results: { ...aggregate, average_score: 3 } },
      }),
      resultsResponse({
        results: { ...aggregate, openid: "must-not-enter-ui" },
      }),
      resultsResponse({ results: { ...aggregate, rules: [{}] } }),
      resultsResponse({ results: { ...aggregate, average_score: -1.5 } }),
      resultsResponse({ results: { ...aggregate, latest_submitted_at: null } }),
      resultsResponse({
        results: {
          submission_count: 0,
          latest_submitted_at: "2026-08-19T01:02:03Z",
          average_score: 0,
          rules: [],
        },
      }),
      resultsResponse({
        results: {
          submission_count: 0,
          latest_submitted_at: null,
          average_score: 1,
          rules: [],
        },
      }),
      resultsResponse({
        results: {
          ...aggregate,
          latest_submitted_at: "2026-02-31T01:02:03Z",
        },
      }),
    ]) {
      await expect(
        loadQuestionnaireResults(
          transport({
            results: vi.fn(async () => ({ status: 200, data })) as never,
          }),
          parsed,
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
  });

  it("maps aggregate read failures once without retry", async () => {
    for (const [status, expected] of [
      [400, "invalid"],
      [401, "unauthenticated"],
      [403, "forbidden"],
      [404, "not_found"],
      [503, "unavailable"],
    ] as const) {
      const client = transport({
        results: vi.fn(async () => ({ status, data: {} })) as never,
      });
      await expect(loadQuestionnaireResults(client, parsed)).resolves.toEqual({
        status: expected,
      });
      expect(client.results).toHaveBeenCalledTimes(1);
    }
    const offline = transport({
      results: vi.fn(async () => {
        throw new Error("offline");
      }) as never,
    });
    await expect(loadQuestionnaireResults(offline, parsed)).resolves.toEqual({
      status: "unavailable",
    });
    expect(offline.results).toHaveBeenCalledTimes(1);
  });

  it("strictly accepts the local declaration-only preflight snapshot", async () => {
    const client = transport();
    await expect(loadQuestionnairePreflight(client)).resolves.toEqual({
      status: "loaded",
      preflight: {
        status: "partial",
        checks: {
          wechatOAuthConfigured: false,
          wecomContactConfigured: false,
          debugSessionAPIEnabled: false,
          wecomTagsAPIAvailable: false,
          questionnaireAdminUIEnabled: true,
          identityMapAvailable: false,
        },
      },
    });
    expect(client.preflight).toHaveBeenCalledWith({
      credentials: "same-origin",
    });
    await expect(
      loadQuestionnairePreflight(
        transport({
          preflight: vi.fn(async () => ({
            status: 200,
            data: {
              ok: true,
              checks: {
                wechat_oauth_configured: false,
                wecom_contact_configured: false,
                debug_session_api_enabled: false,
                wecom_tags_api_available: false,
                questionnaire_admin_ui_enabled: true,
                identity_map_available: false,
                unexpected: false,
              },
              status: "partial",
            },
          })) as never,
        }),
      ),
    ).resolves.toEqual({ status: "invalid" });
  });

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
      { ...item, public_path: "https://untrusted.example/q/welcome" },
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
  it("accepts only safe single-segment slugs paired with their public path", async () => {
    for (const slug of [
      "../admin/users",
      "a/b",
      "a\\\\b",
      ".",
      "..",
      "question?next=/admin",
      "question#section",
      "%2fadmin",
    ]) {
      const malicious = { ...item, slug, public_path: `/q/${slug}` };
      await expect(
        loadQuestionnaires(
          transport({
            list: vi.fn(async () => listResponse(malicious)) as never,
          }),
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
    const boundary = {
      ...item,
      slug: "问卷.v2-2026_08",
      public_path: "/q/问卷.v2-2026_08",
    };
    await expect(
      loadQuestionnaires(
        transport({
          list: vi.fn(async () => listResponse(boundary)) as never,
        }),
      ),
    ).resolves.toMatchObject({
      status: "loaded",
      items: [expect.objectContaining({ publicPath: boundary.public_path })],
    });
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
  it("duplicates once with an empty request body and a bounded idempotency key", async () => {
    const client = transport();
    await expect(
      duplicateQuestionnaire(client, parsed, "x".repeat(43)),
    ).resolves.toEqual({ status: "saved" });
    expect(client.duplicate).toHaveBeenCalledTimes(1);
    expect(client.duplicate).toHaveBeenCalledWith(
      parsed.id,
      undefined,
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.objectContaining({
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key": expect.stringMatching(/^questionnaire-duplicate-/),
        }),
      }),
    );
    const options = vi.mocked(client.duplicate).mock.calls[0][2] as RequestInit;
    const key = (options.headers as Record<string, string>)["Idempotency-Key"];
    expect(key.length).toBeGreaterThanOrEqual(16);
    expect(key.length).toBeLessThanOrEqual(128);
    expect(questionnaireMutationReloadOffset({ status: "saved" })).toBe(0);
  });
  it("fails closed for malformed duplicate receipts and maps every failure without retry", async () => {
    for (const data of [
      duplicateMutationResponse({ questionnaire_id: item.id }),
      duplicateMutationResponse({ source_questionnaire_id: 42 }),
      duplicateMutationResponse({ write_model_status: "disabled" }),
      duplicateMutationResponse({}, false),
    ]) {
      await expect(
        duplicateQuestionnaire(
          transport({
            duplicate: vi.fn(async () => ({ status: 200, data })) as never,
          }),
          parsed,
          "x".repeat(43),
        ),
      ).resolves.toEqual({ status: "invalid" });
    }
    for (const [status, result] of [
      [400, "invalid"],
      [401, "unauthenticated"],
      [403, "forbidden"],
      [404, "not_found"],
      [409, "conflict"],
      [503, "unavailable"],
    ] as const) {
      const client = transport({
        duplicate: vi.fn(async () => ({ status, data: {} })) as never,
      });
      await expect(
        duplicateQuestionnaire(client, parsed, "x".repeat(43)),
      ).resolves.toEqual({ status: result });
      expect(client.duplicate).toHaveBeenCalledTimes(1);
    }
    await expect(
      duplicateQuestionnaire(
        transport({
          duplicate: vi.fn(async () => {
            throw new Error("offline");
          }) as never,
        }),
        parsed,
        "x".repeat(43),
      ),
    ).resolves.toEqual({ status: "unavailable" });
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
