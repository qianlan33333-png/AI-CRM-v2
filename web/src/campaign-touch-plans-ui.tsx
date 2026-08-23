/* eslint-disable no-unused-vars -- component callback types need parameter declarations. */
import React, { useEffect, useMemo, useState } from "react";
import {
  generatedCampaignTouchPlansTransport,
  mutationOptions,
  mutationResult,
  parseAudiencePackages,
  parseCampaigns,
  parseHandoff,
  parsePlans,
  parseRecipients,
  parseReconciliation,
  parseReview,
  reviewRequest,
  type AudiencePackage,
  type Campaign,
  type CampaignTouchPlansTransport,
  type Handoff,
  type RecipientPage,
  type Reconciliation,
  type Review,
  type TouchPlan,
} from "./campaign-touch-plans";

type Role = "admin" | "ops" | "sales";
type Command =
  | {
      readonly kind: "create";
      readonly key: string;
      readonly campaign: Campaign;
      readonly audience: AudiencePackage;
    }
  | {
      readonly kind: "review";
      readonly key: string;
      readonly operation: "submit" | "approve" | "reject";
      readonly campaignCode: string;
      readonly planID: string;
      readonly expectedVersion: number;
      readonly confirmation?: string;
    }
  | {
      readonly kind: "accept";
      readonly key: string;
      readonly campaignCode: string;
      readonly planID: string;
      readonly reviewVersion: number;
    };

function runtimeCookieHeader(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}
function idempotencyKey(kind: string): string {
  return `campaign-touch-plan-${kind}-${globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-local`}`;
}

function safeLoad<T>(
  response: { status: number; data: unknown },
  parser: (unknown: unknown) => T | undefined,
): T | undefined {
  return response.status === 200 ? parser(response.data) : undefined;
}

export function CampaignTouchPlansWorkspace({
  role,
  transport = generatedCampaignTouchPlansTransport,
  readCookie = runtimeCookieHeader,
  newKey = idempotencyKey,
  onUnauthenticated,
}: {
  readonly role: Role;
  readonly transport?: CampaignTouchPlansTransport;
  readonly readCookie?: () => string;
  readonly newKey?: (kind: string) => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const [campaigns, setCampaigns] = useState<readonly Campaign[]>([]);
  const [audiences, setAudiences] = useState<readonly AudiencePackage[]>([]);
  const [campaignCode, setCampaignCode] = useState("");
  const [audienceID, setAudienceID] = useState(0);
  const [plans, setPlans] = useState<readonly TouchPlan[]>([]);
  const [planID, setPlanID] = useState("");
  const [review, setReview] = useState<Review>();
  const [recipients, setRecipients] = useState<RecipientPage>();
  const [handoff, setHandoff] = useState<Handoff>();
  const [reconciliation, setReconciliation] = useState<Reconciliation>();
  const [confirmation, setConfirmation] = useState("");
  const [pending, setPending] = useState<Command>();
  const [notice, setNotice] = useState("");

  const campaign = useMemo(
    () => campaigns.find((item) => item.code === campaignCode),
    [campaignCode, campaigns],
  );
  const audience = useMemo(
    () => audiences.find((item) => item.id === audienceID),
    [audienceID, audiences],
  );

  useEffect(() => {
    if (role === "sales") return;
    let active = true;
    void Promise.all([
      transport.listCampaigns(),
      transport.listAudiencePackages(),
    ])
      .then(([campaignResponse, audienceResponse]) => {
        if (!active) return;
        if (
          campaignResponse.status === 401 ||
          audienceResponse.status === 401
        ) {
          onUnauthenticated?.();
          return;
        }
        const nextCampaigns = safeLoad(campaignResponse, parseCampaigns);
        const nextAudiences = safeLoad(audienceResponse, parseAudiencePackages);
        if (!nextCampaigns || !nextAudiences) {
          setNotice("无法读取本地 Campaign 或人群包投影。");
          return;
        }
        setCampaigns(nextCampaigns);
        setAudiences(nextAudiences);
        setCampaignCode((current) => current || nextCampaigns[0]?.code || "");
        setAudienceID((current) => current || nextAudiences[0]?.id || 0);
      })
      .catch(() => {
        if (active) setNotice("无法读取本地投影。");
      });
    return () => {
      active = false;
    };
  }, [role, transport, onUnauthenticated]);

  const refreshPlan = async (code: string, id: string): Promise<void> => {
    const [reviewResponse, recipientResponse, handoffResponse] =
      await Promise.all([
        transport.getReview(code, id),
        transport.listRecipients(code, id),
        transport.getHandoff(code, id),
      ]);
    if (
      [reviewResponse, recipientResponse, handoffResponse].some(
        (response) => response.status === 401,
      )
    ) {
      onUnauthenticated?.();
      return;
    }
    const nextReview = safeLoad(reviewResponse, parseReview);
    const nextRecipients = safeLoad(recipientResponse, parseRecipients);
    if (!nextReview || !nextRecipients) {
      setNotice("本地审核快照格式无效或暂不可用。");
      return;
    }
    setReview(nextReview);
    setRecipients(nextRecipients);
    const nextHandoff = safeLoad(handoffResponse, parseHandoff);
    setHandoff(nextHandoff);
    if (!nextHandoff) {
      setReconciliation(undefined);
      return;
    }
    const reconciliationResponse = await transport.reconcileHandoff(code, id);
    if (reconciliationResponse.status === 401) {
      onUnauthenticated?.();
      return;
    }
    setReconciliation(safeLoad(reconciliationResponse, parseReconciliation));
  };

  const refreshCampaign = async (code: string): Promise<void> => {
    const [campaignResponse, plansResponse] = await Promise.all([
      transport.listCampaigns(),
      transport.listPlans(code),
    ]);
    if (campaignResponse.status === 401 || plansResponse.status === 401) {
      onUnauthenticated?.();
      return;
    }
    const nextCampaigns = safeLoad(campaignResponse, parseCampaigns);
    const nextPlans = safeLoad(plansResponse, parsePlans);
    if (!nextCampaigns || !nextPlans) {
      setNotice("无法回读服务器当前版本。 ");
      return;
    }
    setCampaigns(nextCampaigns);
    setPlans(nextPlans.items);
  };

  const loadRecipients = async (cursor?: string): Promise<void> => {
    if (!campaignCode || !planID) return;
    const response = await transport.listRecipients(
      campaignCode,
      planID,
      cursor,
    );
    if (response.status === 401) {
      onUnauthenticated?.();
      return;
    }
    const next = safeLoad(response, parseRecipients);
    if (!next) {
      setNotice("无法读取目标快照。 ");
      return;
    }
    setRecipients(next);
  };

  useEffect(() => {
    if (!campaignCode) return;
    let active = true;
    void transport
      .listPlans(campaignCode)
      .then((response) => {
        if (!active) return;
        if (response.status === 401) {
          onUnauthenticated?.();
          return;
        }
        const next = safeLoad(response, parsePlans);
        if (!next) {
          setNotice("无法读取本地 touch plan。 ");
          return;
        }
        setPlans(next.items);
        setPlanID((current) =>
          current && next.items.some((item) => item.id === current)
            ? current
            : next.items[0]?.id || "",
        );
      })
      .catch(() => {
        if (active) setNotice("无法读取本地 touch plan。 ");
      });
    return () => {
      active = false;
    };
  }, [campaignCode, transport, onUnauthenticated]);

  useEffect(() => {
    if (!campaignCode || !planID) return;
    void refreshPlan(campaignCode, planID).catch(() =>
      setNotice("无法读取本地审核快照。"),
    );
  }, [campaignCode, planID, transport]);

  const execute = async (command: Command): Promise<void> => {
    const options = mutationOptions(readCookie(), command.key);
    if (!options) {
      setNotice("缺少同源 CSRF 或请求键无效，未发起变更。");
      return;
    }
    let response: { status: number; data: unknown };
    try {
      if (command.kind === "create") {
        response = await transport.createPlan(
          command.campaign.code,
          {
            expected_campaign_version: command.campaign.version,
            source: {
              kind: "ai_audience_package_members",
              audience_package_id: command.audience.id,
            },
          },
          options,
        );
      } else if (command.kind === "review") {
        const request = reviewRequest(
          command.operation,
          command.planID,
          command.expectedVersion,
          command.confirmation,
        );
        if (!request) {
          setNotice("确认短语必须精确匹配当前 touch plan，未发起变更。");
          return;
        }
        response = await transport.mutateReview(
          command.campaignCode,
          command.planID,
          command.operation,
          request,
          options,
        );
      } else {
        response = await transport.acceptHandoff(
          command.campaignCode,
          command.planID,
          { expected_review_version: command.reviewVersion },
          options,
        );
      }
    } catch {
      setPending(command);
      setNotice("请求结果未确认；可用相同请求键重放。 ");
      return;
    }
    if (response.status === 401) {
      onUnauthenticated?.();
      return;
    }
    const result = mutationResult(response.status);
    if (result === "conflict") {
      setPending(undefined);
      setNotice("服务器版本已变化，已读取当前本地快照。");
      if (command.kind === "create") {
        await refreshCampaign(command.campaign.code);
      } else await refreshPlan(command.campaignCode, command.planID);
      return;
    }
    if (result !== "ok") {
      setPending(command);
      setNotice("请求结果未确认；可用相同请求键重放。 ");
      return;
    }
    setPending(undefined);
    setConfirmation("");
    setNotice("本地事实已读取；不表示外部调用、发送或交付。");
    if (command.kind === "create") {
      setCampaignCode(command.campaign.code);
      const plansResponse = await transport.listPlans(command.campaign.code);
      const next = safeLoad(plansResponse, parsePlans);
      if (next) {
        setPlans(next.items);
        setPlanID(next.items[0]?.id || "");
      }
    } else await refreshPlan(command.campaignCode, command.planID);
  };

  if (role === "sales")
    return (
      <section aria-labelledby="campaign-touch-plans-title">
        <h1 id="campaign-touch-plans-title">Campaign 审阅</h1>
        <p role="alert">当前账号没有 Campaign 本地审阅权限。</p>
      </section>
    );

  return (
    <section aria-labelledby="campaign-touch-plans-title">
      <h1 id="campaign-touch-plans-title">Campaign 本地审核</h1>
      <p role="note">
        所有结果均为本地事实。目标仅展示 canonical customer ID；本地 held
        交接只记录 held 状态和内部 Events delivery，不表示外部调用、发送或交付。
      </p>
      {notice ? <p role="status">{notice}</p> : null}
      <label>
        Campaign
        <select
          aria-label="选择 Campaign"
          value={campaignCode}
          onChange={(event) => setCampaignCode(event.currentTarget.value)}
        >
          {campaigns.map((item) => (
            <option key={item.code} value={item.code}>
              {item.name}（v{item.version}）
            </option>
          ))}
        </select>
      </label>
      <label>
        来源人群包
        <select
          aria-label="选择来源人群包"
          value={audienceID || ""}
          onChange={(event) => setAudienceID(Number(event.currentTarget.value))}
        >
          {audiences.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}（{item.members} 位，本地快照）
            </option>
          ))}
        </select>
      </label>
      <button
        type="button"
        disabled={!campaign || !audience}
        onClick={() => {
          if (campaign && audience)
            void execute({
              kind: "create",
              key: newKey("create"),
              campaign,
              audience,
            });
        }}
      >
        从人群包冻结 touch plan
      </button>
      <label>
        Touch plan
        <select
          aria-label="选择 touch plan"
          value={planID}
          onChange={(event) => setPlanID(event.currentTarget.value)}
        >
          {plans.map((item) => (
            <option key={item.id} value={item.id}>
              {item.id}（{item.targetCount} 个目标）
            </option>
          ))}
        </select>
      </label>
      {planID ? (
        <section aria-labelledby="campaign-review-title">
          <h2 id="campaign-review-title">审核状态</h2>
          <p>
            {review ? `${review.status}，版本 ${review.version}` : "读取中"}
          </p>
          {review?.status === "draft" ? (
            <button
              type="button"
              onClick={() =>
                void execute({
                  kind: "review",
                  key: newKey("submit"),
                  operation: "submit",
                  campaignCode,
                  planID,
                  expectedVersion: review.version,
                })
              }
            >
              提交审核
            </button>
          ) : null}
          {review?.status === "pending_review" ? (
            <>
              <label>
                确认短语
                <input
                  aria-label="审核确认短语"
                  value={confirmation}
                  onChange={(event) =>
                    setConfirmation(event.currentTarget.value)
                  }
                />
              </label>
              <button
                type="button"
                onClick={() =>
                  void execute({
                    kind: "review",
                    key: newKey("approve"),
                    operation: "approve",
                    campaignCode,
                    planID,
                    expectedVersion: review.version,
                    confirmation,
                  })
                }
              >
                批准本地交接
              </button>
              <button
                type="button"
                onClick={() =>
                  void execute({
                    kind: "review",
                    key: newKey("reject"),
                    operation: "reject",
                    campaignCode,
                    planID,
                    expectedVersion: review.version,
                    confirmation,
                  })
                }
              >
                驳回 touch plan
              </button>
            </>
          ) : null}
          <h2>目标快照</h2>
          <p>仅 canonical customer ID，不做身份富化。</p>
          <ul>
            {recipients?.ids.map((id) => (
              <li key={id}>{id}</li>
            ))}
          </ul>
          {recipients?.nextCursor ? (
            <button
              type="button"
              onClick={() => void loadRecipients(recipients.nextCursor)}
            >
              读取下一页 canonical customer ID
            </button>
          ) : null}
          {review?.handoff ? (
            <section aria-labelledby="campaign-handoff-title">
              <h2 id="campaign-handoff-title">本地 held 交接</h2>
              <p>
                审核版本 {review.handoff.reviewVersion}；目标{" "}
                {handoff?.targetCount ?? "读取中"}；步骤{" "}
                {handoff?.stepCount ?? "读取中"}。
              </p>
              <button
                type="button"
                onClick={() =>
                  void execute({
                    kind: "accept",
                    key: newKey("accept"),
                    campaignCode,
                    planID,
                    reviewVersion: review.handoff!.reviewVersion,
                  })
                }
              >
                接纳本地交接
              </button>
              {reconciliation ? (
                <p>
                  held {reconciliation.heldCount}，blocked{" "}
                  {reconciliation.blockedCount}，pending{" "}
                  {reconciliation.pendingCount}。
                </p>
              ) : null}
            </section>
          ) : null}
        </section>
      ) : null}
      {pending ? (
        <button type="button" onClick={() => void execute(pending)}>
          以相同请求键重放未确认请求
        </button>
      ) : null}
    </section>
  );
}
