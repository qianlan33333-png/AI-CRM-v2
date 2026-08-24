/* eslint-disable no-unused-vars -- the minimal DOM shim exposes React DOM structural fields. */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CampaignTouchPlanTransport } from "./campaign-touch-plan-core";
import type { CampaignTouchPlanReadTransport } from "./campaign-touch-plan-read";
import type { CampaignTouchPlanReviewTransport } from "./campaign-touch-plan-review";
import type { OutboundCampaignHandoffTransport } from "./outbound-campaign-handoff";
import {
  CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH,
  CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH,
  CLOUD_ORCHESTRATOR_PLANS_PATH,
  cloudOrchestratorRoute,
  type CloudOrchestratorRole,
} from "./cloud-orchestrator";
import { CloudOrchestratorWorkspace } from "./cloud-orchestrator-ui";

class TestNode { parentNode: TestNode | null = null; childNodes: TestNode[] = []; ownerDocument!: TestDocument; constructor(readonly nodeType: number, readonly nodeName: string) {} appendChild(node: TestNode): TestNode { node.parentNode = this; this.childNodes.push(node); return node; } insertBefore(node: TestNode, before: TestNode | null): TestNode { if (!before) return this.appendChild(node); node.parentNode = this; this.childNodes.splice(this.childNodes.indexOf(before), 0, node); return node; } removeChild(node: TestNode): TestNode { this.childNodes.splice(this.childNodes.indexOf(node), 1); node.parentNode = null; return node; } get firstChild(): TestNode | null { return this.childNodes[0] ?? null; } get nextSibling(): TestNode | null { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null; } get textContent(): string { return this.childNodes.map((node) => node.textContent).join(""); } set textContent(value: string) { this.childNodes = value ? [new TestText(value, this.ownerDocument)] : []; } addEventListener(): void {} removeEventListener(): void {} contains(node: TestNode | null): boolean { return node === this || this.childNodes.some((child) => child.contains(node)); } }
class TestText extends TestNode { constructor(private value: string, owner: TestDocument) { super(3, "#text"); this.ownerDocument = owner; } override get textContent(): string { return this.value; } override set textContent(value: string) { this.value = value; } }
class TestElement extends TestNode { readonly tagName: string; readonly namespaceURI = "http://www.w3.org/1999/xhtml"; readonly style = {}; private attributes = new Map<string, string>(); constructor(tag: string, owner: TestDocument) { super(1, tag.toUpperCase()); this.tagName = tag.toUpperCase(); this.ownerDocument = owner; } get options(): TestElement[] { return this.childNodes.filter((node): node is TestElement => node instanceof TestElement && node.tagName === "OPTION"); } setAttribute(name: string, value: string): void { this.attributes.set(name, value); } removeAttribute(name: string): void { this.attributes.delete(name); } getAttribute(name: string): string | null { return this.attributes.get(name) ?? null; } getAttributeNames(): string[] { return [...this.attributes.keys()]; } hasAttribute(name: string): boolean { return this.attributes.has(name); } }
class TestDocument extends TestNode { readonly documentElement: TestElement; readonly body: TestElement; readonly defaultView: Record<string, unknown>; activeElement: TestElement | null; constructor() { super(9, "#document"); this.ownerDocument = this; this.documentElement = this.createElement("html"); this.body = this.createElement("body"); this.documentElement.appendChild(this.body); this.appendChild(this.documentElement); this.activeElement = this.body; this.defaultView = { document: this, navigator: { userAgent: "node" } }; } createElement(tag: string): TestElement { return new TestElement(tag, this); } createElementNS(_namespace: string, tag: string): TestElement { return this.createElement(tag); } createTextNode(value: string): TestText { return new TestText(value, this); } createComment(value: string): TestText { return new TestText(value, this); } }
function mount(): { root: Root; container: TestElement } { const document = new TestDocument(); const window = document.defaultView; Object.assign(window, { Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, getSelection: () => null }); for (const [key, value] of Object.entries({ document, window, Node: TestNode, Element: TestElement, HTMLElement: TestElement, HTMLIFrameElement: TestElement, IS_REACT_ACT_ENVIRONMENT: true })) vi.stubGlobal(key, value); const container = document.createElement("div"); document.body.appendChild(container); return { root: createRoot(container as unknown as Element), container }; }
function elements(root: TestNode, tag: string): TestElement[] { return [root, ...root.childNodes.flatMap((node) => elements(node, tag))].filter((node): node is TestElement => node instanceof TestElement && node.tagName === tag.toUpperCase()); }
function props<T>(element: TestElement): T { const key = Object.keys(element).find((item) => item.startsWith("__reactProps")); if (!key) throw new Error("missing React props"); return (element as unknown as Record<string, T>)[key]; }

afterEach(() => vi.unstubAllGlobals());

function render(
  pathname: string,
  role: CloudOrchestratorRole = "admin",
): string {
  const route = cloudOrchestratorRoute(pathname);
  if (!route) throw new Error("test route must be valid");
  return renderToStaticMarkup(
    <CloudOrchestratorWorkspace role={role} route={route} />,
  );
}

describe("CloudOrchestratorWorkspace", () => {
  it("renders a plan-list review carrier without inventing an approval action", () => {
    const html = render(CLOUD_ORCHESTRATOR_PLANS_PATH);
    expect(html).toContain("运营计划审阅");
    expect(html).toContain("当前不猜测计划、受众或审批字段");
    expect(html).not.toContain("<button");
    expect(html).not.toContain("<form");
    expect(html).not.toContain("action_token");
  });

  it("renders the exact plan identifier and closed target-person review sections", () => {
    const html = render(`${CLOUD_ORCHESTRATOR_PLANS_PATH}/plan%20one`);
    expect(html).toContain("plan one");
    expect(html).toContain("目标人员审阅");
    expect(html).toContain("单人审批");
    expect(html).toContain("不展示或推断人员数据");
    expect(html).not.toContain("<button");
  });

  it("renders the campaign workspace and observability navigation without execution claims", () => {
    const html = render(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH);
    expect(html).toContain("Campaign 审阅工作区");
    expect(html).toContain("本地触达计划审阅");
    expect(html).toContain('href="/admin/cloud-orchestrator/observability"');
    expect(html).toContain("Provider 已调用");
    expect(html).toContain("外部发送已执行");
  });

  it("renders only the four approved observability entry labels", () => {
    const html = render(CLOUD_ORCHESTRATOR_OBSERVABILITY_PATH);
    for (const label of ["工单", "审计", "漏斗", "Tool 调用统计"]) {
      expect(html).toContain(label);
    }
    expect(html).not.toContain("成功率");
    expect(html).not.toContain("质量分");
  });

  it.each(["ops", "sales"] as const)("fails closed for the %s role", (role) => {
    const html = render(CLOUD_ORCHESTRATOR_PLANS_PATH, role);
    expect(html).toContain("没有 AI 助手本地审阅权限");
    expect(html).not.toContain("运营计划审阅");
  });

  it("allows ops to open only the Campaign carrier and keeps sales closed", () => {
    const ops = render(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, "ops");
    expect(ops).toContain("Campaign 审阅工作区");
    expect(ops).toContain("Campaign 审阅");
    expect(ops).not.toContain("运营计划");
    expect(ops).not.toContain("可观察性");
    expect(ops).not.toContain("触达计划人工审核");

    const sales = render(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, "sales");
    expect(sales).toContain("没有 AI 助手本地审阅权限");
    expect(sales).not.toContain("Campaign 审阅工作区");
    expect(sales).not.toContain("触达计划人工审核");
  });

  it("lets ops complete the Campaign list and detail reads required before C1 creation", async () => {
    const timestamp = "2026-08-24T01:02:03Z";
    const summary = { campaign_code: "spring", name: "Spring", approval_status: "draft", runtime_status: "idle", version: 1, created_by: 7, updated_by: 7, created_at: timestamp, updated_at: timestamp };
    const safety = { local_projection: true, real_external_call_executed: false, real_send: false, runtime_executed: false };
    const transport: CampaignTouchPlanTransport = {
      listCampaigns: vi.fn(async () => ({ status: 200, data: { items: [summary], ...safety } })),
      getCampaign: vi.fn(async () => ({ status: 200, data: { campaign: summary, steps: [{ step_index: 1, delay_minutes: 0, content: "local" }], ...safety } })),
      createPlan: vi.fn(),
      getPlan: vi.fn(),
    };
    const route = cloudOrchestratorRoute(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH, "?source_kind=customer_selection&source_id=7")!;
    const mounted = mount();
    await act(async () => mounted.root.render(<CloudOrchestratorWorkspace role="ops" route={route} actorID={7} campaignTransport={transport} />));
    await act(async () => props<{ onChange(event: { currentTarget: { value: string } }): void }>(elements(mounted.container, "select")[0]).onChange({ currentTarget: { value: "spring" } }));
    expect(transport.listCampaigns).toHaveBeenCalledTimes(1);
    expect(transport.getCampaign).toHaveBeenCalledWith("spring", expect.objectContaining({ credentials: "same-origin" }));
    expect(mounted.container.textContent).toContain("1 个本地步骤");
    await act(async () => mounted.root.unmount());
  });

  it("mounts held acceptance only from the verified C1 plan and closed approved C2 seam", async () => {
    const now = "2026-08-24T01:02:03.123456Z"; const digest = "a".repeat(64); const planID = `ctp_${digest}`;
    const safe = { local_only: true, provider_execution_eligible: false, runtime_executed: false, real_external_call_executed: false, delivery_proven: false };
    const rawPlan = { id: planID, campaign_code: "a", campaign_version: 1, source: { kind: "customer_selection", customer_selection: { id: "local_selection", version: "v1", digest } }, target_count: 1, target_digest: digest, content_step_count: 1, content_digest: digest, owner_actor_id: 7, preview_exclusion_summary: { candidate_count: 1, active_customer_count: 1, inactive_excluded_count: 0, policy_excluded_count: 0 }, created_at: now, ...safe };
    const { content_step_count: _count, content_digest, ...rest } = rawPlan;
    const read: CampaignTouchPlanReadTransport = {
      listCampaigns: vi.fn(async () => ({ status: 200, data: { items: [{ campaign_code: "a", name: "a", approval_status: "draft", runtime_status: "idle", version: 1, created_by: 7, updated_by: 7, created_at: now, updated_at: now }], local_projection: true, real_external_call_executed: false, real_send: false, runtime_executed: false } })),
      getCampaign: vi.fn(async () => ({ status: 500, data: {} })), createPlan: vi.fn(async () => ({ status: 500, data: {} })),
      listPlans: vi.fn(async () => ({ status: 200, data: { items: [rawPlan], next_cursor: null, ...safe } })),
      getPlan: vi.fn(async () => ({ status: 200, data: { ...rest, content: { steps: [{ step_index: 1, delay_minutes: 0, content: "local" }], content_digest } } })),
      listRecipients: vi.fn(async () => ({ status: 200, data: { items: [{ canonical_customer_id: 1 }], next_cursor: null, local_only: true, provider_execution_eligible: false, real_external_call_executed: false, delivery_proven: false } })),
    };
    const review: CampaignTouchPlanReviewTransport = { getReview: vi.fn(async () => ({ status: 200, data: { review: { status: "approved", version: 3, submitted_by_actor_id: 7, submitted_at: now, reviewed_by_actor_id: 7, reviewed_at: now }, handoff: { status: "pending_outbound_acceptance", review_version: 3, created_at: now }, local_only: true, provider_execution_eligible: false, real_external_call_executed: false, delivery_proven: false } })), mutateReview: vi.fn() };
    const getSummary = vi.fn(async () => ({ status: 404, data: {} }));
    const handoff: OutboundCampaignHandoffTransport = { getSummary, accept: vi.fn(), reconcile: vi.fn() };
    const route = cloudOrchestratorRoute(CLOUD_ORCHESTRATOR_CAMPAIGNS_PATH)!; const mounted = mount();
    await act(async () => mounted.root.render(<CloudOrchestratorWorkspace role="ops" route={route} actorID={7} campaignReadTransport={read} campaignReviewTransport={review} campaignHandoffTransport={handoff} />));
    await act(async () => props<{ onChange(event: { currentTarget: { value: string } }): void }>(elements(mounted.container, "select")[0]).onChange({ currentTarget: { value: "a" } }));
    await act(async () => props<{ onChange(event: { currentTarget: { value: string } }): void }>(elements(mounted.container, "select")[1]).onChange({ currentTarget: { value: planID } }));
    expect(getSummary, mounted.container.textContent).toHaveBeenCalledWith("a", planID, expect.objectContaining({ credentials: "same-origin" }));
    expect(mounted.container.textContent).toContain("Outbound 本地 held 交接");
    await act(async () => mounted.root.unmount());
  });
});
