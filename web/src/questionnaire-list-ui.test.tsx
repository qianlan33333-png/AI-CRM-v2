/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  copyQuestionnairePublicLink,
  performQuestionnairePageMutation,
  QuestionnaireEditorPanel,
  QuestionnaireListContent,
  QuestionnaireListPage,
} from "./questionnaire-list-ui";
import type {
  QuestionnaireDefinition,
  QuestionnaireItem,
  QuestionnaireListTransport,
} from "./questionnaire-list";
import { newQuestionnaireEditorDraft } from "./questionnaire-list";

class TestNode {
  parentNode: TestNode | null = null;
  childNodes: TestNode[] = [];
  ownerDocument!: TestDocument;
  constructor(readonly nodeType: number, readonly nodeName: string) {}
  appendChild(node: TestNode): TestNode { node.parentNode = this; this.childNodes.push(node); return node; }
  insertBefore(node: TestNode, before: TestNode | null): TestNode { if (before === null) return this.appendChild(node); node.parentNode = this; this.childNodes.splice(this.childNodes.indexOf(before), 0, node); return node; }
  removeChild(node: TestNode): TestNode { this.childNodes.splice(this.childNodes.indexOf(node), 1); node.parentNode = null; return node; }
  get firstChild(): TestNode | null { return this.childNodes[0] ?? null; }
  get nextSibling(): TestNode | null { if (!this.parentNode) return null; return this.parentNode.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null; }
  get textContent(): string { return this.childNodes.map((node) => node.textContent).join(""); }
  set textContent(value: string) { this.childNodes = value === "" ? [] : [new TestText(value, this.ownerDocument)]; }
  addEventListener(): void {}
  removeEventListener(): void {}
  contains(node: TestNode | null): boolean { return node === this || this.childNodes.some((child) => child.contains(node)); }
}
class TestText extends TestNode {
  constructor(private data: string, ownerDocument: TestDocument) { super(3, "#text"); this.ownerDocument = ownerDocument; }
  override get textContent(): string { return this.data; }
  override set textContent(value: string) { this.data = value; }
}
class TestElement extends TestNode {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style: Record<string, string> = {};
  private readonly attributes = new Map<string, string>();
  constructor(tagName: string, ownerDocument: TestDocument) { super(1, tagName.toUpperCase()); this.tagName = tagName.toUpperCase(); this.ownerDocument = ownerDocument; }
  get options(): TestElement[] { return this.childNodes.filter((node): node is TestElement => node instanceof TestElement && node.tagName === "OPTION"); }
  setAttribute(name: string, value: string): void { this.attributes.set(name, value); }
  removeAttribute(name: string): void { this.attributes.delete(name); }
  getAttribute(name: string): string | null { return this.attributes.get(name) ?? null; }
  hasAttribute(name: string): boolean { return this.attributes.has(name); }
}
class TestDocument extends TestNode {
  readonly nodeType = 9;
  readonly documentElement: TestElement;
  readonly body: TestElement;
  readonly defaultView: Record<string, unknown>;
  activeElement: TestElement | null;
  constructor() {
    super(9, "#document"); this.ownerDocument = this;
    this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement);
    this.activeElement = this.body;
    this.defaultView = { document: this, navigator: { userAgent: "node" } };
  }
  createElement(tagName: string): TestElement { return new TestElement(tagName, this); }
  createElementNS(_namespace: string, tagName: string): TestElement { return this.createElement(tagName); }
  createTextNode(value: string): TestText { return new TestText(value, this); }
  createComment(value: string): TestText { return new TestText(value, this); }
}

function mountedRoot(): { readonly root: Root; readonly container: TestElement } {
  const document = new TestDocument();
  const window = document.defaultView as Record<string, unknown>;
  Object.assign(window, { Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, getSelection: () => null });
  Object.assign(globalThis, { document, window, Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, IS_REACT_ACT_ENVIRONMENT: true });
  const container = document.createElement("div"); document.body.appendChild(container);
  return { root: createRoot(container as unknown as Element), container };
}
function elements(root: TestNode, tagName: string): TestElement[] {
  return [root, ...root.childNodes.flatMap((node) => elements(node, tagName))].filter((node): node is TestElement => node instanceof TestElement && node.tagName === tagName);
}
function buttons(root: TestNode): TestElement[] { return elements(root, "BUTTON"); }
function reactProps<T extends Record<string, unknown>>(element: TestElement): T {
  const key = Object.keys(element).find((candidate) => candidate.startsWith("__reactProps"));
  if (key === undefined) throw new Error("mounted element is missing React props");
  return (element as unknown as Record<string, T>)[key];
}
function click(button: TestElement): void {
  const props = reactProps<{ onClick?: () => void }>(button);
  if (!props.onClick) throw new Error("mounted button is missing React click handler");
  props.onClick();
}
function deferred<T>(): { readonly promise: Promise<T>; resolve(value: T): void } {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>((done) => { resolve = done; }), resolve };
}

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
const definition: QuestionnaireDefinition = {
  item: active,
  description: "问卷说明",
  answerDisplayMode: "all_in_one",
  questions: [
    {
      type: "single_choice",
      title: "目标",
      assessmentDimensionKey: "",
      sidebarProfileField: "",
      required: true,
      placeholderText: "请选择",
      validation: { minSelections: 1, maxSelections: 1 },
      sortOrder: 0,
      options: [
        {
          text: "增长",
          score: 0,
          assessmentTypeKey: "",
          tagCodes: [],
          isOther: false,
          otherPlaceholder: "",
          otherMaxLength: 0,
          sortOrder: 0,
        },
      ],
    },
  ],
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

function duplicateResponse() {
  const questionnaire = {
    ...item,
    id: 42,
    slug: "welcome-copy",
    public_path: "/q/welcome-copy",
    is_disabled: true,
    enabled: false,
    status: "disabled",
  };
  return {
    ok: true,
    questionnaire_id: 42,
    source_questionnaire_id: item.id,
    questionnaire,
    questions: questionnaire.questions,
    data: { questionnaire },
    write_model_status: "duplicated",
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
    definition: vi.fn(async () => ({
      status: 200,
      data: {
        ok: true,
        questionnaire: item,
        questions: item.questions,
        data: { questionnaire: item },
      },
    })),
    disable: vi.fn(async (_id, body) => ({
      status: 200,
      data: response(
        body.is_disabled,
        body.is_disabled ? "disabled" : "enabled",
      ),
    })),
    duplicate: vi.fn(async () => ({
      status: 200,
      data: duplicateResponse(),
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
  it("allows admin and ops locally while sales remains inert", () => {
    for (const role of ["admin", "ops"] as const) {
      const client = transport();
      const html = renderToStaticMarkup(
        <QuestionnaireListPage role={role} transport={client} />,
      );
      expect(html).toContain("正在读取问卷列表");
      expect(html).not.toContain("当前账号没有问卷管理权限。");
      expect(html).not.toContain("删除");
      expect(client.list).not.toHaveBeenCalled();
    }
    const client = transport();
    const html = renderToStaticMarkup(<QuestionnaireListPage role="sales" transport={client} />);
    expect(html).toContain("当前账号没有问卷管理权限。");
    expect(client.list).not.toHaveBeenCalled();
  });

  it("keeps every list command locked until a successful editor save is semantically reread", async () => {
    const initial = deferred<{ status: number; data: unknown }>();
    const firstRead = deferred<{ status: number; data: unknown }>();
    const confirmationRead = deferred<{ status: number; data: unknown }>();
    let savedQuestionnaire: Record<string, unknown> = item;
    const envelope = (questionnaire: Record<string, unknown>) => ({
      ok: true,
      questionnaire,
      questions: questionnaire.questions,
      data: { questionnaire },
    });
    const list = vi.fn(() =>
      list.mock.calls.length === 1
        ? initial.promise
        : Promise.resolve({
            status: 200,
            data: {
              ok: true,
              questionnaires: [item, { ...item, id: 42, name: "disabled", title: "已停用", slug: "disabled", public_path: "/q/disabled", is_disabled: true, enabled: false, status: "disabled" }],
              items: [item, { ...item, id: 42, name: "disabled", title: "已停用", slug: "disabled", public_path: "/q/disabled", is_disabled: true, enabled: false, status: "disabled" }],
              data: { questionnaires: [item, { ...item, id: 42, name: "disabled", title: "已停用", slug: "disabled", public_path: "/q/disabled", is_disabled: true, enabled: false, status: "disabled" }] },
              total: 2,
              limit: 50,
              offset: 0,
            },
          }),
    );
    const definition = vi.fn(() =>
      definition.mock.calls.length === 1
        ? firstRead.promise
        : confirmationRead.promise,
    );
    const replace = vi.fn(async (_id: number, request: Record<string, unknown>) => {
      savedQuestionnaire = {
        ...item,
        ...request,
        id: item.id,
        enabled: !(request.is_disabled as boolean),
        status: request.is_disabled ? "disabled" : "active",
        question_count: (request.questions as readonly unknown[]).length,
        questions: request.questions,
      };
      return {
        status: 200,
        data: {
          ...envelope(savedQuestionnaire),
          questionnaire_id: item.id,
          write_model_status: "updated",
        },
      };
    });
    const client = transport({
      list: list as never,
      definition: definition as never,
      replace: replace as never,
    });
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <QuestionnaireListPage
          role="admin"
          readCookie={() => `aicrm_csrf=${csrf}`}
          transport={client}
        />,
      );
    });
    await act(async () => {
      initial.resolve({
        status: 200,
        data: {
          ok: true,
          questionnaires: [item, { ...item, id: 42, name: "disabled", title: "已停用", slug: "disabled", public_path: "/q/disabled", is_disabled: true, enabled: false, status: "disabled" }],
          items: [item, { ...item, id: 42, name: "disabled", title: "已停用", slug: "disabled", public_path: "/q/disabled", is_disabled: true, enabled: false, status: "disabled" }],
          data: { questionnaires: [item, { ...item, id: 42, name: "disabled", title: "已停用", slug: "disabled", public_path: "/q/disabled", is_disabled: true, enabled: false, status: "disabled" }] },
          total: 2,
          limit: 50,
          offset: 0,
        },
      });
      await Promise.resolve();
    });
    const byText = (text: string) => {
      const button = buttons(mounted.container).find((candidate) => candidate.textContent === text);
      if (!button) throw new Error(`missing ${text}`);
      return button;
    };
    await act(async () => { click(byText("编辑问卷")); });
    await act(async () => { firstRead.resolve({ status: 200, data: envelope(item) }); await Promise.resolve(); });
    await act(async () => { click(byText("保存完整定义")); await Promise.resolve(); });
    expect(replace).toHaveBeenCalledOnce();
    expect(definition).toHaveBeenCalledTimes(2);
    for (const text of ["新建问卷", "编辑问卷", "复制问卷", "停用", "删除"]) {
      expect(byText(text).hasAttribute("disabled")).toBe(true);
      await act(async () => { click(byText(text)); });
    }
    expect(replace).toHaveBeenCalledOnce();
    expect(definition).toHaveBeenCalledTimes(2);
    expect(client.disable).not.toHaveBeenCalled();
    expect(client.duplicate).not.toHaveBeenCalled();
    expect(client.remove).not.toHaveBeenCalled();
    await act(async () => {
      confirmationRead.resolve({ status: 200, data: envelope(savedQuestionnaire) });
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("问卷定义已保存，已重新读取确认。");
    expect(byText("停用").hasAttribute("disabled")).toBe(false);
    await act(async () => { mounted.root.unmount(); });
  });

  it("keeps every local write locked after an invalid or unavailable editor receipt", async () => {
    for (const response of [
      { status: 503, data: {} },
      { status: 200, data: { ok: true } },
    ]) {
      const replace = vi.fn(async () => response);
      const client = transport({ replace: replace as never });
      const mounted = mountedRoot();
      await act(async () => {
        mounted.root.render(
          <QuestionnaireListPage
            role="ops"
            readCookie={() => `aicrm_csrf=${csrf}`}
            transport={client}
          />,
        );
        await Promise.resolve();
      });
      const byText = (text: string) => {
        const button = buttons(mounted.container).find((candidate) => candidate.textContent === text);
        if (!button) throw new Error(`missing ${text}`);
        return button;
      };
      await act(async () => { click(byText("编辑问卷")); await Promise.resolve(); });
      await act(async () => { click(byText("保存完整定义")); await Promise.resolve(); });
      expect(replace).toHaveBeenCalledOnce();
      expect(mounted.container.textContent).toContain("问卷写入结果未知");
      for (const text of ["新建问卷", "编辑问卷", "复制问卷", "停用"]) {
        expect(byText(text).hasAttribute("disabled")).toBe(true);
        await act(async () => { click(byText(text)); });
      }
      expect(replace).toHaveBeenCalledOnce();
      expect(client.disable).not.toHaveBeenCalled();
      expect(client.duplicate).not.toHaveBeenCalled();
      await act(async () => { mounted.root.unmount(); });
    }
  });

  it("renders the local full-definition editor without public, submission, or provider controls", () => {
    const html = renderToStaticMarkup(
      <QuestionnaireEditorPanel
        state={{ kind: "ready", draft: newQuestionnaireEditorDraft() }}
        onCancel={() => undefined}
        onDraft={() => undefined}
        onSave={() => undefined}
      />,
    );
    expect(html).toContain("新建问卷草稿");
    expect(html).toContain("添加文本题");
    expect(html).toContain("添加单选题");
    expect(html).toContain("保存完整定义");
    expect(html).not.toContain("href=");
    expect(html).not.toContain("/q/");
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
    expect(page).toContain(">复制问卷<");
    expect(busy.match(/disabled=""/g)).toHaveLength(16);
  });

  it("renders only the local submission aggregate and retains it on a local result read failure", () => {
    const ready = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={undefined}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        results={{
          kind: "ready",
          item: active,
          aggregate: {
            submissionCount: 2,
            latestSubmittedAt: "2026-08-19T01:02:03Z",
            averageScore: 1.5,
          },
        }}
        state={{ kind: "ready", items: [active], total: 1, offset: 0 }}
      />,
    );
    expect(ready).toContain('data-testid="questionnaire-results"');
    expect(ready).toContain("提交汇总：欢迎问卷");
    expect(ready).toContain("提交者身份或答案");
    expect(ready).toContain("1.5");
    expect(ready).not.toMatch(
      /openid|unionid|external_userid|mobile|respondent|redirect|answer/i,
    );

    const failed = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={undefined}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        results={{
          kind: "error",
          item: active,
          failure: "unavailable",
          previous: {
            submissionCount: 2,
            latestSubmittedAt: "2026-08-19T01:02:03Z",
            averageScore: 1.5,
          },
        }}
        state={{ kind: "ready", items: [active], total: 1, offset: 0 }}
      />,
    );
    expect(failed).toContain("1.5");
    expect(failed).toContain("问卷服务暂时不可用，请稍后重试。");
  });

  it("renders a local questionnaire definition and never turns its public path into a link", () => {
    const ready = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={undefined}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        onLoadDefinition={vi.fn()}
        definition={{ kind: "ready", item: active, definition }}
        state={{ kind: "ready", items: [active], total: 1, offset: 0 }}
      />,
    );
    expect(ready).toContain('data-testid="questionnaire-definition"');
    expect(ready).toContain("问卷定义：欢迎问卷");
    expect(ready).toContain("问卷说明");
    expect(ready).toContain("第 1 题：目标");
    expect(ready).toContain("1. 增长");
    expect(ready).toContain(">查看问卷定义<");
    expect(ready).not.toContain('href="/q/welcome"');
    expect(ready).not.toContain("/q/welcome");

    const failed = renderToStaticMarkup(
      <QuestionnaireListContent
        busy={undefined}
        onLoad={vi.fn()}
        onMutate={vi.fn()}
        definition={{
          kind: "error",
          item: active,
          failure: "unavailable",
          previous: definition,
        }}
        state={{ kind: "ready", items: [active], total: 1, offset: 0 }}
      />,
    );
    expect(failed).toContain("问卷说明");
    expect(failed).toContain("问卷服务暂时不可用，请稍后重试。");
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
    expect(client.duplicate).not.toHaveBeenCalled();
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

  it("duplicates without confirmation, balances busy, and does not open an editor", async () => {
    const client = transport();
    const busy = vi.fn();
    const confirmDelete = vi.fn(() => true);
    await expect(
      performQuestionnairePageMutation(
        mutationArgs({
          action: "duplicate",
          confirmDelete,
          onBusy: busy,
          transport: client,
        }),
      ),
    ).resolves.toEqual({ status: "saved" });
    expect(confirmDelete).not.toHaveBeenCalled();
    expect(client.duplicate).toHaveBeenCalledTimes(1);
    expect(busy.mock.calls).toEqual([[41], [undefined]]);
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
