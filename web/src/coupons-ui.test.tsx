/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  CouponsPage,
  CouponsView,
  canStartCouponWrite,
  couponProductOptionsLoadingState,
  couponProductOptionsResultState,
  copyCouponShareURL,
  isCurrentCouponProductOptionsGeneration,
  lifecycleReloadMatches,
  nextCouponProductOptionsOffset,
  performCouponArchive,
  performCouponCopy,
  performCouponDelete,
  performCouponPublish,
  performCouponProductOptionsLoad,
  performCouponStop,
  previousCouponProductOptionsOffset,
  startCouponArchive,
  startCouponCopy,
  startCouponDetail,
  startCouponLifecycle,
  startCouponProductOptions,
  submitCouponDraftForm,
} from "./coupons-ui";
import type { CouponsTransport } from "./coupons";

const item = {
  id: 7,
  name: '<img src=x onerror="bad">',
  status: "published" as const,
  availability: "active" as const,
  issuedCount: 0,
  createdAt: "2026-08-19T00:00:00Z",
  updatedAt: "2026-08-19T01:02:03Z",
};

function transport(): CouponsTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    copy: vi.fn(async () => ({ status: 503, data: {} })),
    claims: vi.fn(async () => ({ status: 503, data: {} })),
    productOptions: vi.fn(async () => ({ status: 503, data: {} })),
    detail: vi.fn(async () => ({ status: 503, data: {} })),
    create: vi.fn(async () => ({ status: 503, data: {} })),
    update: vi.fn(async () => ({ status: 503, data: {} })),
    share: vi.fn(async () => ({ status: 503, data: {} })),
    archive: vi.fn(async () => ({ status: 503, data: {} })),
    publish: vi.fn(async () => ({ status: 503, data: {} })),
    stop: vi.fn(async () => ({ status: 503, data: {} })),
    delete: vi.fn(async () => ({ status: 503, data: {} })),
  } as unknown as CouponsTransport;
}

function archivedCoupon() {
  return {
    id: 7,
    name: item.name,
    discount_amount_total: 100,
    currency: "CNY",
    status: "archived",
    availability_status: "archived",
    total_issue_limit: 20,
    per_user_issue_limit: 1,
    issued_count: 0,
    claim_starts_at: "2026-08-19T00:00:00Z",
    claim_ends_at: "2026-08-20T00:00:00Z",
    validity_mode: "relative_days",
    use_starts_at: null,
    use_ends_at: null,
    relative_validity_days: 30,
    instructions: "",
    target_refs: ["standard_product:7"],
    created_by: 1,
    updated_by: 1,
    version: 1,
    created_at: "2026-08-19T00:00:00Z",
    updated_at: "2026-08-19T01:02:03Z",
  };
}

function lifecycleCoupon(
  status: "draft" | "published" | "stopped" | "deleted",
  availability: "draft" | "active" | "stopped" | "deleted",
  id = 7,
) {
  return {
    ...archivedCoupon(),
    id,
    status,
    availability_status: availability,
  };
}

function transitionReceipt(status: "published" | "stopped", id = 7) {
  return {
    ok: true,
    coupon: lifecycleCoupon(
      status,
      status === "published" ? "active" : "stopped",
      id,
    ),
    status,
    idempotent_same_state: true,
    fallback_used: false,
    real_external_call_executed: false,
  };
}

class TestNode {
  parentNode: TestNode | null = null;
  childNodes: TestNode[] = [];
  ownerDocument!: TestDocument;
  constructor(
    readonly nodeType: number,
    readonly nodeName: string,
  ) {}
  appendChild(node: TestNode): TestNode {
    node.parentNode = this;
    this.childNodes.push(node);
    return node;
  }
  insertBefore(node: TestNode, before: TestNode | null): TestNode {
    if (before === null) return this.appendChild(node);
    node.parentNode = this;
    this.childNodes.splice(this.childNodes.indexOf(before), 0, node);
    return node;
  }
  removeChild(node: TestNode): TestNode {
    this.childNodes.splice(this.childNodes.indexOf(node), 1);
    node.parentNode = null;
    return node;
  }
  get firstChild(): TestNode | null {
    return this.childNodes[0] ?? null;
  }
  get nextSibling(): TestNode | null {
    if (!this.parentNode) return null;
    return (
      this.parentNode.childNodes[
        this.parentNode.childNodes.indexOf(this) + 1
      ] ?? null
    );
  }
  get textContent(): string {
    return this.childNodes.map((node) => node.textContent).join("");
  }
  set textContent(value: string) {
    this.childNodes =
      value === "" ? [] : [new TestText(value, this.ownerDocument)];
  }
  addEventListener(): void {}
  removeEventListener(): void {}
  contains(node: TestNode | null): boolean {
    return (
      node === this || this.childNodes.some((child) => child.contains(node))
    );
  }
}
class TestText extends TestNode {
  constructor(
    private data: string,
    ownerDocument: TestDocument,
  ) {
    super(3, "#text");
    this.ownerDocument = ownerDocument;
  }
  override get textContent(): string {
    return this.data;
  }
  override set textContent(value: string) {
    this.data = value;
  }
}
class TestElement extends TestNode {
  readonly tagName: string;
  readonly namespaceURI = "http://www.w3.org/1999/xhtml";
  readonly style: Record<string, string> = {};
  private readonly attributes = new Map<string, string>();
  constructor(tagName: string, ownerDocument: TestDocument) {
    super(1, tagName.toUpperCase());
    this.tagName = tagName.toUpperCase();
    this.ownerDocument = ownerDocument;
  }
  get options(): TestElement[] {
    return this.childNodes.filter(
      (node): node is TestElement =>
        node instanceof TestElement && node.tagName === "OPTION",
    );
  }
  setAttribute(name: string, value: string): void {
    this.attributes.set(name, value);
  }
  removeAttribute(name: string): void {
    this.attributes.delete(name);
  }
  getAttribute(name: string): string | null {
    return this.attributes.get(name) ?? null;
  }
  hasAttribute(name: string): boolean {
    return this.attributes.has(name);
  }
}
class TestDocument extends TestNode {
  readonly nodeType = 9;
  readonly documentElement: TestElement;
  readonly body: TestElement;
  readonly defaultView: Record<string, unknown>;
  activeElement: TestElement | null;
  constructor() {
    super(9, "#document");
    this.ownerDocument = this;
    this.documentElement = this.createElement("html");
    this.body = this.createElement("body");
    this.documentElement.appendChild(this.body);
    this.appendChild(this.documentElement);
    this.activeElement = this.body;
    this.defaultView = { document: this, navigator: { userAgent: "node" } };
  }
  createElement(tagName: string): TestElement {
    return new TestElement(tagName, this);
  }
  createElementNS(_namespace: string, tagName: string): TestElement {
    return this.createElement(tagName);
  }
  createTextNode(value: string): TestText {
    return new TestText(value, this);
  }
  createComment(value: string): TestText {
    return new TestText(value, this);
  }
}
function mountedRoot(): {
  readonly root: Root;
  readonly container: TestElement;
} {
  const document = new TestDocument();
  const window = document.defaultView as Record<string, unknown>;
  Object.assign(window, {
    Node: TestNode,
    Element: TestElement,
    HTMLElement: TestElement,
    HTMLIFrameElement: TestElement,
    getSelection: () => null,
  });
  Object.assign(globalThis, {
    document,
    window,
    Node: TestNode,
    Element: TestElement,
    HTMLElement: TestElement,
    HTMLIFrameElement: TestElement,
    IS_REACT_ACT_ENVIRONMENT: true,
  });
  const container = document.createElement("div");
  document.body.appendChild(container);
  return { root: createRoot(container as unknown as Element), container };
}
function buttons(root: TestNode): TestElement[] {
  return [root, ...root.childNodes.flatMap(buttons)].filter(
    (node): node is TestElement =>
      node instanceof TestElement && node.tagName === "BUTTON",
  );
}
function reactProps<T extends Record<string, unknown>>(
  element: TestElement,
): T {
  const key = Object.keys(element).find((candidate) =>
    candidate.startsWith("__reactProps"),
  );
  if (key === undefined)
    throw new Error("mounted element is missing React props");
  return (element as unknown as Record<string, T>)[key];
}
function click(button: TestElement): void {
  const props = reactProps<{ onClick?: () => void }>(button);
  if (!props?.onClick)
    throw new Error("mounted button is missing React click handler");
  props.onClick();
}
function deferred<T>(): {
  readonly promise: Promise<T>;
  resolve(value: T): void;
} {
  let resolve!: (value: T) => void;
  return {
    promise: new Promise<T>((done) => {
      resolve = done;
    }),
    resolve,
  };
}

function listEnvelope(items: readonly Record<string, unknown>[]) {
  return {
    ok: true,
    coupons: items,
    items,
    total: items.length,
    limit: 200,
    offset: 0,
  };
}

describe("CouponsView", () => {
  it.each(["admin", "ops"] as const)(
    "renders the local list and copy action for %s only",
    (role) => {
      const html = renderToStaticMarkup(
        <CouponsView
          onCopy={vi.fn()}
          role={role}
          state={{ kind: "ready", items: [item] }}
        />,
      );
      expect(html).toContain('<h1 id="app-title">优惠券列表</h1>');
      expect(html).toContain("搜索优惠券名称");
      expect(html).toContain("可用状态");
      for (const status of [
        "draft",
        "scheduled",
        "active",
        "sold_out",
        "ended",
        "stopped",
        "archived",
      ]) {
        expect(html).toContain(`value="${status}"`);
      }
      expect(html).toContain("复制只会创建新的本地草稿");
      expect(html).toContain("发布、停止和删除仅改变本地优惠券规则状态");
      expect(html).toContain("分享链接");
      expect(html).toContain(">归档<");
      expect(html).toContain("停止本地规则");
      expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
      expect(html).not.toContain("<img");
      expect(html).not.toMatch(/payment|provider|redeem|share/i);
    },
  );

  it("fails closed for sales without issuing either read or write", () => {
    const client = transport();
    const html = renderToStaticMarkup(
      <CouponsPage role="sales" transport={client} />,
    );
    expect(html).toContain("当前账号没有优惠券管理权限。");
    expect(html).not.toContain("正在读取本地优惠券列表。");
    expect(html).not.toContain(">复制<");
    expect(client.list).not.toHaveBeenCalled();
    expect(client.copy).not.toHaveBeenCalled();
    expect(client.archive).not.toHaveBeenCalled();
    expect(client.claims).not.toHaveBeenCalled();
    expect(client.productOptions).not.toHaveBeenCalled();
    expect(client.detail).not.toHaveBeenCalled();
    expect(client.create).not.toHaveBeenCalled();
    expect(client.update).not.toHaveBeenCalled();
    expect(client.share).not.toHaveBeenCalled();
    expect(client.publish).not.toHaveBeenCalled();
    expect(client.stop).not.toHaveBeenCalled();
    expect(client.delete).not.toHaveBeenCalled();
  });

  it("renders only the eligible local lifecycle controls without claiming payment or provider effects", () => {
    const draftHTML = renderToStaticMarkup(
      <CouponsView
        onCopy={vi.fn()}
        onDelete={vi.fn()}
        onPublish={vi.fn()}
        role="admin"
        state={{
          kind: "ready",
          items: [{ ...item, status: "draft", availability: "draft" }],
        }}
      />,
    );
    expect(draftHTML).toContain("发布本地规则");
    expect(draftHTML).toContain("删除未领取草稿");
    expect(draftHTML).not.toContain("停止本地规则");
    expect(draftHTML).not.toMatch(/payment|provider|redeem/i);

    const stoppedHTML = renderToStaticMarkup(
      <CouponsView
        onCopy={vi.fn()}
        onStop={vi.fn()}
        role="ops"
        state={{
          kind: "ready",
          items: [{ ...item, status: "stopped", availability: "stopped" }],
        }}
      />,
    );
    expect(stoppedHTML).not.toContain("发布本地规则");
    expect(stoppedHTML).not.toContain("停止本地规则");
    expect(stoppedHTML).not.toContain("删除未领取草稿");
  });

  it("mounts the row lifecycle state machine with the clicked ID, same-tick singleflight, and a local readback", async () => {
    const initial = deferred<{ status: number; data: unknown }>();
    const publish = deferred<{ status: number; data: unknown }>();
    const first = lifecycleCoupon("draft", "draft", 7);
    const second = lifecycleCoupon("draft", "draft", 8);
    const list = vi.fn(() =>
      list.mock.calls.length === 1
        ? initial.promise
        : Promise.resolve({
            status: 200,
            data: listEnvelope([
              first,
              lifecycleCoupon("published", "active", 8),
            ]),
          }),
    );
    const publishRule = vi.fn((_id: number) => publish.promise);
    const mounted = mountedRoot();
    await act(async () => {
      mounted.root.render(
        <CouponsPage
          readCookie={() => `aicrm_csrf=${"x".repeat(43)}`}
          role="admin"
          transport={{
            ...transport(),
            list: async () => list() as never,
            publish: async (id) => publishRule(id) as never,
          }}
        />,
      );
    });
    await act(async () => {
      initial.resolve({ status: 200, data: listEnvelope([first, second]) });
      await Promise.resolve();
    });
    const publishButtons = buttons(mounted.container).filter(
      (button) => button.textContent === "发布本地规则",
    );
    expect(publishButtons).toHaveLength(2);
    await act(async () => {
      click(publishButtons[1]);
      click(publishButtons[1]);
    });
    expect(publishRule).toHaveBeenCalledTimes(1);
    expect(publishRule).toHaveBeenCalledWith(8);
    await act(async () => {
      publish.resolve({ status: 200, data: transitionReceipt("published", 8) });
      await Promise.resolve();
    });
    expect(list).toHaveBeenCalledTimes(2);
    expect(mounted.container.textContent).toContain("已发布");
    await act(async () => {
      mounted.root.unmount();
    });
  });

  it("renders a separate local product projection without changing the coupon draft", () => {
    const html = renderToStaticMarkup(
      <CouponsView
        onCopy={vi.fn()}
        onProductOptions={vi.fn()}
        productOptionsState={{
          kind: "ready",
          page: {
            items: [
              {
                id: 7,
                targetRef: "standard_product:7",
                name: '<img src=x onerror="bad">',
                priceMinor: 9900,
                currency: "CNY",
              },
            ],
            total: 21,
            offset: 0,
          },
        }}
        role="ops"
        state={{ kind: "ready", items: [item] }}
      />,
    );
    expect(html).toContain("本地普通商品选项");
    expect(html).toContain("价格（分）不代表当前价格、库存、支付或权益可用。");
    expect(html).toContain("standard_product:7");
    expect(html).toContain("9900");
    expect(html).toContain("CNY");
    expect(html).toContain("&lt;img src=x onerror=&quot;bad&quot;&gt;");
    expect(html).not.toContain("<img");
    expect(html).not.toContain('name="targetRefs"');
    expect(html).toContain('<button type="button">下一页</button>');
  });

  it("single-flights product reads, discards stale generations, retains a verified page, and reports 401 once", async () => {
    const lock = { current: false };
    let release: (() => void) | undefined;
    const first = startCouponProductOptions(lock, async () => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
    });
    const duplicate = startCouponProductOptions(lock, async () => {
      throw new Error("must not execute");
    });
    expect(first).toBeInstanceOf(Promise);
    expect(duplicate).toBeUndefined();
    release?.();
    await first;
    expect(lock.current).toBe(false);

    const page = {
      items: [
        {
          id: 7,
          targetRef: "standard_product:7",
          name: "本地普通商品",
          priceMinor: 9900,
          currency: "CNY" as const,
        },
      ],
      total: 41,
      offset: 20,
    };
    expect(couponProductOptionsLoadingState(20, page)).toEqual({
      kind: "loading",
      offset: 20,
      previous: page,
    });
    expect(
      couponProductOptionsResultState({ status: "unavailable" }, 20, page),
    ).toEqual({
      kind: "error",
      offset: 20,
      failure: "unavailable",
      previous: page,
    });
    expect(isCurrentCouponProductOptionsGeneration(3, 3)).toBe(true);
    expect(isCurrentCouponProductOptionsGeneration(3, 4)).toBe(false);
    expect(previousCouponProductOptionsOffset(page)).toBe(0);
    expect(nextCouponProductOptionsOffset(page)).toBe(40);
    expect(
      previousCouponProductOptionsOffset({ ...page, offset: 0 }),
    ).toBeUndefined();
    expect(
      nextCouponProductOptionsOffset({ ...page, total: 21, offset: 20 }),
    ).toBeUndefined();

    const unauthenticated = transport();
    vi.mocked(unauthenticated.productOptions).mockResolvedValue({
      status: 401,
      data: {
        code: "UNAUTHENTICATED",
        message: "登录失效",
        request_id: "req-product-options-1",
      },
    } as never);
    const onUnauthenticated = vi.fn();
    await expect(
      performCouponProductOptionsLoad({
        offset: 0,
        onUnauthenticated,
        transport: unauthenticated,
      }),
    ).resolves.toEqual({ status: "unauthenticated" });
    expect(onUnauthenticated).toHaveBeenCalledOnce();
    expect(unauthenticated.productOptions).toHaveBeenCalledOnce();
  });

  it("locks every coupon write entry point while a draft outcome is uncertain", () => {
    const submit = vi.fn();
    const html = renderToStaticMarkup(
      <CouponsView
        editor={{ kind: "new" }}
        mutationUncertain
        onArchive={vi.fn()}
        onCopy={vi.fn()}
        onCreate={vi.fn()}
        onEdit={vi.fn()}
        onSubmitDraft={submit}
        role="admin"
        state={{
          kind: "ready",
          items: [{ ...item, status: "draft", availability: "draft" }],
        }}
      />,
    );
    expect(canStartCouponWrite(true)).toBe(false);
    expect(canStartCouponWrite(false)).toBe(true);
    expect(html).toContain("系统不会自动重试。请刷新列表后人工确认。");
    expect(html).toContain("草稿表单已只读；刷新本地列表成功前不能保存。");
    expect(html).toMatch(/<input[^>]*disabled=""/);
    expect(html).toMatch(/<select[^>]*disabled=""/);
    expect(html).toMatch(/<textarea[^>]*disabled=""/);
    expect(html).toContain('<button type="submit" disabled="">');
    expect(html.match(/disabled=""/g)?.length).toBeGreaterThanOrEqual(14);
    submitCouponDraftForm(
      true,
      {
        name: "本地草稿",
        discountAmountTotal: "100",
        totalIssueLimit: "20",
        perUserIssueLimit: "1",
        claimStartsAt: "2026-08-19T00:00:00Z",
        claimEndsAt: "2026-08-20T00:00:00Z",
        validityMode: "relative_days",
        useStartsAt: "",
        useEndsAt: "",
        relativeValidityDays: "30",
        instructions: "",
        targetRefs: "standard_product:7",
      },
      submit,
    );
    expect(submit).not.toHaveBeenCalled();
  });

  it("renders the complete local-draft form without product lookup, publish, claim, payment, or provider controls", () => {
    const html = renderToStaticMarkup(
      <CouponsView
        editor={{ kind: "new" }}
        onCopy={vi.fn()}
        onCreate={vi.fn()}
        onSubmitDraft={vi.fn()}
        role="ops"
        state={{
          kind: "ready",
          items: [{ ...item, status: "draft", availability: "draft" }],
        }}
      />,
    );
    expect(html).toContain('aria-label="新建本地优惠券草稿"');
    expect(html).toContain("优惠金额（分）");
    expect(html).toContain("领取开始时间（RFC3339）");
    expect(html).toContain("相对有效天数");
    expect(html).toContain("canonical standard_product:ID");
    expect(html).toContain(
      "只保存本地草稿；不会发布、停止、领取、支付或调用第三方服务。",
    );
    expect(html).toContain(">编辑本地草稿<");
    expect(html).not.toMatch(/product-options|Idempotency-Key|provider|href=/i);
  });

  it("renders only a validated local rule detail, preserves it while a same-card refresh is loading, and has no external action", () => {
    const detail = {
      ...item,
      discountAmountTotal: 100,
      currency: "CNY" as const,
      totalIssueLimit: 20,
      perUserIssueLimit: 1,
      claimStartsAt: "2026-08-19T00:00:00Z",
      claimEndsAt: "2026-08-20T00:00:00Z",
      validityMode: "relative_days" as const,
      relativeValidityDays: 30,
      instructions: "仅本地规则",
      targetRefs: ["standard_product:7"],
      createdBy: 1,
      updatedBy: 1,
      version: 1,
    };
    const html = renderToStaticMarkup(
      <CouponsView
        detailState={{ kind: "loading", coupon: item, previous: detail }}
        onCopy={vi.fn()}
        onDetail={vi.fn()}
        role="admin"
        state={{ kind: "ready", items: [item] }}
      />,
    );
    expect(html).toContain("正在读取规则…");
    expect(html).toContain('aria-label="优惠券本地规则详情"');
    expect(html).toContain("正在读取本地优惠券规则。");
    expect(html).toContain(
      "仅显示已保存的本地规则，不代表可领取、可用或已发生外部效果。",
    );
    expect(html).toContain("standard_product:7");
    expect(html).not.toMatch(/provider|payment|redeem|send|href=/i);
  });

  it("shows a local share URL only for a published coupon and retains it for manual copy", () => {
    const html = renderToStaticMarkup(
      <CouponsView
        onCopy={vi.fn()}
        onCopyShare={vi.fn()}
        onShare={vi.fn()}
        role="admin"
        shareState={{
          kind: "ready",
          coupon: item,
          url: "/c/c-7",
          copyStatus: "manual",
        }}
        state={{
          kind: "ready",
          items: [
            item,
            { ...item, id: 8, status: "draft" },
            { ...item, id: 9, status: "stopped" },
            { ...item, id: 10, status: "archived" },
          ],
        }}
      />,
    );
    expect(html).toContain('aria-label="优惠券本地分享链接"');
    expect(html).toContain("/c/c-7");
    expect(html).toContain("无法访问剪贴板，请手工复制上方链接。");
    expect(html.match(/>分享链接</g)).toHaveLength(1);
    expect(html.match(/>归档</g)).toHaveLength(3);
    expect(html).not.toMatch(/qrcode|payment|provider|redeem|send/i);
  });

  it("copies only a validated local URL and leaves it manually available when clipboard fails", async () => {
    const clipboard = { writeText: vi.fn(async () => undefined) };
    await expect(copyCouponShareURL("/c/c-7", clipboard)).resolves.toBe(
      "copied",
    );
    expect(clipboard.writeText).toHaveBeenCalledWith("/c/c-7");

    const rejected = {
      writeText: vi.fn(async () => Promise.reject(new Error("denied"))),
    };
    await expect(copyCouponShareURL("/c/c-7", rejected)).resolves.toBe(
      "manual",
    );
    await expect(
      copyCouponShareURL("https://outside.example/c/c-7", clipboard),
    ).resolves.toBe("manual");
    expect(clipboard.writeText).toHaveBeenCalledOnce();
  });

  it("renders only the frozen opaque claim projection and bounded pagination", () => {
    const html = renderToStaticMarkup(
      <CouponsView
        claimsState={{
          kind: "ready",
          coupon: item,
          page: {
            items: [
              {
                id: 9,
                claimRef: "cp_1234567890abcdef",
                claimedAt: "2026-08-19T02:03:04Z",
              },
            ],
            total: 51,
            offset: 0,
          },
        }}
        onClaims={vi.fn()}
        onCopy={vi.fn()}
        role="ops"
        state={{ kind: "ready", items: [item] }}
      />,
    );
    expect(html).toContain('aria-label="优惠券领取数据"');
    expect(html).toContain("领取记录 ID");
    expect(html).toContain("领取凭据");
    expect(html).toContain("claimed");
    expect(html).toContain("下一页");
    expect(html).not.toMatch(/customer|identity|mobile|payment|provider/i);
  });

  it("makes one same-click copy request with the CSRF cookie and no redirect", async () => {
    const client = transport();
    vi.mocked(client.copy).mockResolvedValue({
      status: 200,
      data: {
        ok: true,
        coupon: {
          id: 8,
          name: "副本",
          discount_amount_total: 100,
          currency: "CNY",
          status: "draft",
          availability_status: "draft",
          total_issue_limit: 20,
          per_user_issue_limit: 1,
          issued_count: 0,
          claim_starts_at: "2026-08-19T00:00:00Z",
          claim_ends_at: "2026-08-20T00:00:00Z",
          validity_mode: "relative_days",
          use_starts_at: null,
          use_ends_at: null,
          relative_validity_days: 30,
          instructions: "",
          target_refs: ["standard_product:7"],
          created_by: 1,
          updated_by: 1,
          version: 1,
          created_at: "2026-08-19T01:02:04Z",
          updated_at: "2026-08-19T01:02:04Z",
        },
      },
    } as never);

    await expect(
      performCouponCopy({
        couponID: item.id,
        idempotencySource: {
          randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
        },
        readCookie: () => `aicrm_csrf=${"x".repeat(43)}`,
        transport: client,
      }),
    ).resolves.toMatchObject({ status: "copied", item: { id: 8 } });
    expect(client.copy).toHaveBeenCalledTimes(1);
    expect(client.copy).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key": "coupon-copy:123e4567-e89b-42d3-a456-426614174000",
        },
      }),
    );
  });

  it("requires confirmation then archives once with the CSRF cookie and a unique key", async () => {
    const client = transport();
    vi.mocked(client.archive).mockResolvedValue({
      status: 200,
      data: { ok: true, coupon: archivedCoupon() },
    } as never);
    const confirm = vi.fn(() => true);

    await expect(
      performCouponArchive({
        confirm,
        idempotencySource: {
          randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
        },
        item,
        readCookie: () => `aicrm_csrf=${"x".repeat(43)}`,
        transport: client,
      }),
    ).resolves.toMatchObject({ status: "archived", item: { id: 7 } });
    expect(confirm).toHaveBeenCalledWith(
      `确认归档本地优惠券“${item.name}”吗？`,
    );
    expect(client.archive).toHaveBeenCalledOnce();
    expect(client.archive).toHaveBeenCalledWith(
      item.id,
      expect.objectContaining({
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key":
            "coupon-archive:123e4567-e89b-42d3-a456-426614174000",
        },
      }),
    );

    await expect(
      performCouponArchive({
        confirm: () => false,
        item,
        readCookie: () => `aicrm_csrf=${"x".repeat(43)}`,
        transport: client,
      }),
    ).resolves.toEqual({ status: "canceled" });
    expect(client.archive).toHaveBeenCalledOnce();
  });

  it("executes only the eligible local publish, stop, or confirmed draft delete with exact request guards", async () => {
    const client = transport();
    const draft = {
      ...item,
      status: "draft" as const,
      availability: "draft" as const,
    };
    vi.mocked(client.publish).mockResolvedValue({
      status: 200,
      data: transitionReceipt("published"),
    } as never);
    vi.mocked(client.stop).mockResolvedValue({
      status: 200,
      data: transitionReceipt("stopped"),
    } as never);
    vi.mocked(client.delete).mockResolvedValue({
      status: 200,
      data: {
        ok: true,
        coupon: lifecycleCoupon("deleted", "deleted"),
      },
    } as never);
    const readCookie = () => `aicrm_csrf=${"x".repeat(43)}`;

    await expect(
      performCouponPublish({ item: draft, readCookie, transport: client }),
    ).resolves.toMatchObject({ status: "published", item: { id: 7 } });
    await expect(
      performCouponStop({ item, readCookie, transport: client }),
    ).resolves.toMatchObject({ status: "stopped", item: { id: 7 } });
    await expect(
      performCouponDelete({
        confirm: () => true,
        idempotencySource: {
          randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
        },
        item: draft,
        readCookie,
        transport: client,
      }),
    ).resolves.toMatchObject({ status: "deleted", item: { id: 7 } });
    expect(client.publish).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        credentials: "same-origin",
        headers: { "X-CSRF-Token": "x".repeat(43) },
      }),
    );
    expect(client.stop).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        credentials: "same-origin",
        headers: { "X-CSRF-Token": "x".repeat(43) },
      }),
    );
    expect(client.delete).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        credentials: "same-origin",
        headers: {
          "X-CSRF-Token": "x".repeat(43),
          "Idempotency-Key":
            "coupon-delete:123e4567-e89b-42d3-a456-426614174000",
        },
      }),
    );
    await expect(
      performCouponDelete({
        confirm: () => false,
        item: draft,
        readCookie,
        transport: client,
      }),
    ).resolves.toEqual({ status: "canceled" });
    expect(client.delete).toHaveBeenCalledOnce();
  });

  it("allows only one copy when two clicks arrive in the same render tick", async () => {
    const lock = { current: false };
    const client = transport();
    const execute = vi.fn(async () => {
      await performCouponCopy({
        couponID: item.id,
        idempotencySource: {
          randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
        },
        readCookie: () => `aicrm_csrf=${"x".repeat(43)}`,
        transport: client,
      });
    });

    const first = startCouponCopy(lock, execute);
    const second = startCouponCopy(lock, execute);
    expect(first).toBeInstanceOf(Promise);
    expect(second).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();
    expect(client.copy).toHaveBeenCalledOnce();
    expect(lock.current).toBe(true);

    await first;
    expect(lock.current).toBe(false);
  });

  it("allows only one confirmed archive when two clicks arrive in the same render tick", async () => {
    const lock = { current: false };
    const client = transport();
    vi.mocked(client.archive).mockResolvedValue({
      status: 200,
      data: { ok: true, coupon: archivedCoupon() },
    } as never);
    const execute = vi.fn(async () => {
      await performCouponArchive({
        confirm: () => true,
        idempotencySource: {
          randomUUID: () => "123e4567-e89b-42d3-a456-426614174000",
        },
        item,
        readCookie: () => `aicrm_csrf=${"x".repeat(43)}`,
        transport: client,
      });
    });

    const first = startCouponArchive(lock, execute);
    const second = startCouponArchive(lock, execute);
    expect(first).toBeInstanceOf(Promise);
    expect(second).toBeUndefined();
    expect(execute).toHaveBeenCalledOnce();
    expect(client.archive).toHaveBeenCalledOnce();
    await first;
    expect(lock.current).toBe(false);
  });

  it("single-flights all local lifecycle writes with the same ref lock", async () => {
    const lock = { current: false };
    let release: (() => void) | undefined;
    const first = startCouponLifecycle(lock, async () => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
    });
    const duplicate = startCouponLifecycle(lock, async () => {
      throw new Error("must not execute");
    });
    expect(first).toBeInstanceOf(Promise);
    expect(duplicate).toBeUndefined();
    release?.();
    await first;
    expect(lock.current).toBe(false);
  });

  it("requires the post-mutation local list readback to bind the returned rule or confirm deletion", () => {
    const published = {
      status: "published" as const,
      item: {
        ...item,
        status: "published" as const,
        availability: "active" as const,
      },
    };
    const deleted = {
      status: "deleted" as const,
      item: {
        ...item,
        status: "draft" as const,
        availability: "draft" as const,
      },
    };
    expect(
      lifecycleReloadMatches(published, {
        status: "loaded",
        items: [published.item],
      }),
    ).toBe(true);
    expect(
      lifecycleReloadMatches(published, {
        status: "loaded",
        items: [{ ...published.item, issuedCount: 1 }],
      }),
    ).toBe(false);
    expect(
      lifecycleReloadMatches(deleted, { status: "loaded", items: [] }),
    ).toBe(true);
    expect(
      lifecycleReloadMatches(deleted, {
        status: "loaded",
        items: [deleted.item],
      }),
    ).toBe(false);
  });

  it("allows one in-flight detail request per coupon and releases each local lock", async () => {
    const inFlight = new Set<number>();
    let releaseFirst: (() => void) | undefined;
    const first = startCouponDetail(inFlight, 7, async () => {
      await new Promise<void>((resolve) => {
        releaseFirst = resolve;
      });
    });
    const duplicate = startCouponDetail(inFlight, 7, async () => undefined);
    const other = startCouponDetail(inFlight, 8, async () => undefined);
    expect(first).toBeInstanceOf(Promise);
    expect(duplicate).toBeUndefined();
    expect(other).toBeInstanceOf(Promise);
    expect(inFlight).toEqual(new Set([7, 8]));
    await other;
    expect(inFlight).toEqual(new Set([7]));
    releaseFirst?.();
    await first;
    expect(inFlight).toEqual(new Set());
    expect(
      startCouponDetail(inFlight, 0, async () => undefined),
    ).toBeUndefined();
  });

  it("releases the single-flight lock when the copy flow throws", async () => {
    const lock = { current: false };
    await expect(
      startCouponCopy(lock, async () => {
        throw new Error("local test failure");
      }),
    ).rejects.toThrow("local test failure");
    expect(lock.current).toBe(false);
  });
});
