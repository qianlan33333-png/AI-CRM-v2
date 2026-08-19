import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  copyQuestionnairePublicLink,
  performQuestionnairePageMutation,
  QuestionnaireListContent,
  QuestionnaireListPage,
} from "./questionnaire-list-ui";
import type {
  QuestionnaireItem,
  QuestionnaireListTransport,
} from "./questionnaire-list";

const csrf = "x".repeat(43);
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
const active: QuestionnaireItem = {
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
const disabled: QuestionnaireItem = {
  ...active,
  isDisabled: true,
  status: "disabled",
};

function response(
  disabledValue: boolean,
  status: "enabled" | "disabled" | "deleted",
  extra: Record<string, unknown> = {},
) {
  const questionnaire = {
    ...item,
    is_disabled: disabledValue,
    enabled: !disabledValue,
    status: disabledValue ? "disabled" : "active",
  };
  return {
    ok: true,
    questionnaire_id: item.id,
    questionnaire,
    questions: questionnaire.questions,
    data: { questionnaire },
    write_model_status: status,
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
      data: response(
        body.is_disabled,
        body.is_disabled ? "disabled" : "enabled",
      ),
    })),
    remove: vi.fn(async () => ({
      status: 200,
      data: response(true, "deleted", {
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
    ...overrides,
  } as unknown as QuestionnaireListTransport;
}

function mutationArgs(
  override: Partial<
    Parameters<typeof performQuestionnairePageMutation>[0]
  > = {},
) {
  return {
    action: "toggle" as const,
    confirmDelete: vi.fn(() => true),
    item: active,
    onBusy: vi.fn(),
    readCookie: () => `aicrm_csrf=${csrf}`,
    transport: transport(),
    ...override,
  };
}

describe("QuestionnaireListPage UI", () => {
  it("rejects direct ops and sales access without rendering management controls", () => {
    for (const role of ["ops", "sales"] as const) {
      const client = transport();
      const html = renderToStaticMarkup(
        <QuestionnaireListPage role={role} transport={client} />,
      );
      expect(html).toContain("当前账号没有问卷管理权限。");
      expect(html).not.toContain("正在读取问卷列表");
      expect(html).not.toContain("删除");
      expect(client.list).not.toHaveBeenCalled();
    }
  });

  it("renders empty and paginated states, hides active delete, and disables all actions while busy", () => {
    const empty = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={undefined}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        state={{ kind: "ready", items: [], total: 0, offset: 0 }}
      />,
    );
    expect(empty).toContain("当前没有问卷。");
    const page = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={undefined}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        state={{ kind: "ready", items: [active], total: 51, offset: 0 }}
      />,
    );
    expect(page).toContain("请先停用后删除");
    expect(page).not.toContain(">删除<");
    expect(page).toContain(">下一页<");
    expect(page).not.toContain('disabled="">下一页');
    const busy = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={active.id}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        state={{
          kind: "ready",
          items: [active, disabled],
          total: 52,
          offset: 0,
        }}
      />,
    );
    expect(busy.match(/disabled=""/g)).toHaveLength(7);
  });

  it("copies the parsed same-origin public link without using transport", async () => {
    const writeText = vi.fn(async () => undefined);
    const prompt = vi.fn();
    await expect(
      copyQuestionnairePublicLink(active, {
        location: { origin: "https://crm.example.test" },
        navigator: { clipboard: { writeText } },
        prompt,
      }),
    ).resolves.toBe("copied");
    expect(writeText).toHaveBeenCalledWith(
      "https://crm.example.test/q/welcome",
    );
    expect(prompt).not.toHaveBeenCalled();
  });

  it("uses the manual-copy prompt when clipboard is missing or rejects", async () => {
    for (const clipboard of [
      undefined,
      {
        writeText: vi.fn(async () => {
          throw new Error("denied");
        }),
      },
    ]) {
      const prompt = vi.fn();
      await expect(
        copyQuestionnairePublicLink(active, {
          location: { origin: "https://crm.example.test" },
          navigator: { clipboard },
          prompt,
        }),
      ).resolves.toBe("manual");
      expect(prompt).toHaveBeenCalledWith(
        "请手动复制公开链接：",
        "https://crm.example.test/q/welcome",
      );
    }
  });

  it("reports a questionnaire without a public link without opening a prompt", async () => {
    const prompt = vi.fn();
    await expect(
      copyQuestionnairePublicLink(
        { ...active, publicPath: "" },
        {
          location: { origin: "https://crm.example.test" },
          navigator: {},
          prompt,
        },
      ),
    ).resolves.toBe("missing");
    expect(prompt).not.toHaveBeenCalled();
  });

  it("renders the six fail-closed preflight checks without changing operations", () => {
    const html = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={undefined}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        preflight={{
          kind: "ready",
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
        }}
        state={{ kind: "ready", items: [active], total: 1, offset: 0 }}
      />,
    );
    expect(html).toContain('data-testid="questionnaire-preflight"');
    expect(html).toContain("状态：partial");
    expect(html).toContain("wechat_oauth_configured");
    expect(html).toContain("questionnaire_admin_ui_enabled");
    expect(html).toContain("identity_map_available");
  });

  it("fails closed on a missing CSRF cookie before any mutation", async () => {
    const client = transport();
    await expect(
      performQuestionnairePageMutation(
        mutationArgs({ readCookie: () => "other=1", transport: client }),
      ),
    ).resolves.toEqual({ status: "forbidden" });
    expect(client.disable).not.toHaveBeenCalled();
    expect(client.remove).not.toHaveBeenCalled();
  });

  it("uses disable with the requested true or false state and balances global busy", async () => {
    const client = transport();
    const busy = vi.fn();
    await expect(
      performQuestionnairePageMutation(
        mutationArgs({ onBusy: busy, transport: client }),
      ),
    ).resolves.toEqual({ status: "saved" });
    await expect(
      performQuestionnairePageMutation(
        mutationArgs({ item: disabled, onBusy: busy, transport: client }),
      ),
    ).resolves.toEqual({ status: "saved" });
    expect(client.disable).toHaveBeenNthCalledWith(
      1,
      41,
      { is_disabled: true },
      expect.any(Object),
    );
    expect(client.disable).toHaveBeenNthCalledWith(
      2,
      41,
      { is_disabled: false },
      expect.any(Object),
    );
    expect(busy.mock.calls).toEqual([[41], [undefined], [41], [undefined]]);
  });

  it("requires delete confirmation and performs one confirmed delete", async () => {
    const client = transport();
    const cancel = vi.fn(() => false);
    await expect(
      performQuestionnairePageMutation(
        mutationArgs({
          action: "delete",
          confirmDelete: cancel,
          item: disabled,
          transport: client,
        }),
      ),
    ).resolves.toEqual({ status: "cancelled" });
    expect(cancel).toHaveBeenCalledWith("确认删除已停用问卷“欢迎问卷”？");
    expect(client.remove).not.toHaveBeenCalled();
    await expect(
      performQuestionnairePageMutation(
        mutationArgs({
          action: "delete",
          confirmDelete: vi.fn(() => true),
          item: disabled,
          transport: client,
        }),
      ),
    ).resolves.toEqual({ status: "saved" });
    expect(client.remove).toHaveBeenCalledTimes(1);
  });

  it("calls the unauthenticated callback once and never retries conflict or unavailable mutations", async () => {
    const callback = vi.fn();
    const unauthenticated = transport({
      disable: vi.fn(async () => ({ status: 401, data: {} })) as never,
    });
    await expect(
      performQuestionnairePageMutation(
        mutationArgs({
          onUnauthenticated: callback,
          transport: unauthenticated,
        }),
      ),
    ).resolves.toEqual({ status: "unauthenticated" });
    expect(callback).toHaveBeenCalledOnce();
    for (const status of [409, 503]) {
      const client = transport({
        disable: vi.fn(async () => ({ status, data: {} })) as never,
      });
      await expect(
        performQuestionnairePageMutation(mutationArgs({ transport: client })),
      ).resolves.toEqual({
        status: status === 409 ? "conflict" : "unavailable",
      });
      expect(client.disable).toHaveBeenCalledTimes(1);
    }
  });
});
