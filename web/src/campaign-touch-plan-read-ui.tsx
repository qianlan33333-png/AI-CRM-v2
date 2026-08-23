import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  generatedCampaignTouchPlanReadTransport,
  loadTouchPlanDetail,
  loadTouchPlanRecipients,
  loadTouchPlans,
  type CampaignTouchPlanReadTransport,
  type TouchPlanSummary,
} from "./campaign-touch-plan-read";
import {
  loadDraftCampaigns,
  type CampaignDraftSummary,
} from "./campaign-touch-plan-core";

type State = "idle" | "loading" | "loaded" | "unavailable";
type Props = {
  readonly actorID?: number;
  readonly transport?: CampaignTouchPlanReadTransport;
  readonly onUnauthenticated?: () => void;
};

function sourceLabel(plan: TouchPlanSummary): string {
  switch (plan.source.kind) {
    case "customer_selection":
      return "单个 Customer OneID 本地快照";
    case "segment_members":
      return `Segment #${plan.source.id} 本地快照`;
    case "ai_audience_package_members":
      return `AI Audience package #${plan.source.id} 本地快照`;
  }
}
function ReadPanelInner({
  transport = generatedCampaignTouchPlanReadTransport,
  onUnauthenticated,
}: Props): React.ReactElement {
  const [campaigns, setCampaigns] = useState<readonly CampaignDraftSummary[]>(
    [],
  );
  const [campaignCode, setCampaignCode] = useState("");
  const [plans, setPlans] = useState<readonly TouchPlanSummary[]>([]);
  const [selected, setSelected] = useState<TouchPlanSummary>();
  const [detail, setDetail] = useState<TouchPlanSummary>();
  const [recipients, setRecipients] = useState<readonly number[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [campaignState, setCampaignState] = useState<State>("loading");
  const [planState, setPlanState] = useState<State>("idle");
  const [detailState, setDetailState] = useState<State>("idle");
  const [recipientState, setRecipientState] = useState<State>("idle");
  const campaignGeneration = useRef(0);
  const planGeneration = useRef(0);
  const detailGeneration = useRef(0);
  const recipientGeneration = useRef(0);
  const recipientController = useRef<AbortController>();
  const mounted = useRef(false);
  useLayoutEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const active = (
    generation: number,
    current: React.MutableRefObject<number>,
  ): boolean => mounted.current && generation === current.current;
  const reject = (
    status: string,
    generation: number,
    current: React.MutableRefObject<number>,
    setState: React.Dispatch<React.SetStateAction<State>>,
  ): boolean => {
    if (!active(generation, current)) return true;
    if (status === "unauthenticated") onUnauthenticated?.();
    setState("unavailable");
    return true;
  };

  useEffect(() => {
    const generation = ++campaignGeneration.current;
    const controller = new AbortController();
    setCampaigns([]);
    setCampaignCode("");
    setPlans([]);
    setSelected(undefined);
    setDetail(undefined);
    setRecipients([]);
    setNextCursor(undefined);
    setCampaignState("loading");
    setPlanState("idle");
    setDetailState("idle");
    setRecipientState("idle");
    const signaledTransport = {
      ...transport,
      listCampaigns: (
        params: Parameters<typeof transport.listCampaigns>[0],
        options: Parameters<typeof transport.listCampaigns>[1],
      ) =>
        transport.listCampaigns(params, {
          ...options,
          signal: controller.signal,
        }),
    };
    void loadDraftCampaigns(signaledTransport).then((result) => {
      if (!active(generation, campaignGeneration)) return;
      if (result.status !== "loaded") {
        reject(result.status, generation, campaignGeneration, setCampaignState);
        return;
      }
      setCampaigns(result.campaigns);
      setCampaignState("loaded");
    });
    return () => controller.abort();
  }, [transport]);

  useEffect(() => {
    const generation = ++planGeneration.current;
    const controller = new AbortController();
    setPlans([]);
    setSelected(undefined);
    setDetail(undefined);
    setRecipients([]);
    setNextCursor(undefined);
    setDetailState("idle");
    setRecipientState("idle");
    if (!campaignCode) {
      setPlanState("idle");
      return () => controller.abort();
    }
    setPlanState("loading");
    void loadTouchPlans(transport, campaignCode, controller.signal).then(
      (result) => {
        if (!active(generation, planGeneration) || campaignCode === "") return;
        if (result.status !== "loaded") {
          reject(result.status, generation, planGeneration, setPlanState);
          return;
        }
        if (result.plans.some((plan) => plan.campaignCode !== campaignCode)) {
          setPlanState("unavailable");
          return;
        }
        setPlans(result.plans);
        setPlanState("loaded");
      },
    );
    return () => controller.abort();
  }, [campaignCode, transport]);

  useEffect(() => {
    const generation = ++detailGeneration.current;
    const controller = new AbortController();
    ++recipientGeneration.current;
    recipientController.current?.abort();
    setDetail(undefined);
    setRecipients([]);
    setNextCursor(undefined);
    setRecipientState("idle");
    if (!selected) {
      setDetailState("idle");
      return () => controller.abort();
    }
    const selectedID = selected.id;
    setDetailState("loading");
    void loadTouchPlanDetail(
      transport,
      campaignCode,
      selected,
      controller.signal,
    ).then((result) => {
      if (
        !active(generation, detailGeneration) ||
        selectedID !== selected.id ||
        selected.campaignCode !== campaignCode
      )
        return;
      if (result.status !== "loaded") {
        reject(result.status, generation, detailGeneration, setDetailState);
        return;
      }
      setDetail(result.plan);
      setDetailState("loaded");
    });
    return () => controller.abort();
  }, [campaignCode, selected, transport]);

  const loadRecipients = (cursor?: string): void => {
    if (!detail) return;
    const generation = ++recipientGeneration.current;
    const controller = new AbortController();
    recipientController.current?.abort();
    recipientController.current = controller;
    const previous = cursor ? recipients : [];
    const selectedID = detail.id;
    setRecipientState("loading");
    void loadTouchPlanRecipients(
      transport,
      campaignCode,
      selectedID,
      detail.targetCount,
      cursor,
      previous,
      controller.signal,
    ).then((result) => {
      if (
        !active(generation, recipientGeneration) ||
        selectedID !== detail.id ||
        detail.campaignCode !== campaignCode
      )
        return;
      if (result.status !== "loaded") {
        reject(
          result.status,
          generation,
          recipientGeneration,
          setRecipientState,
        );
        return;
      }
      setRecipients([...previous, ...result.recipients]);
      setNextCursor(result.nextCursor);
      setRecipientState("loaded");
    });
  };
  useEffect(() => {
    if (detail) loadRecipients();
    // Selection identity is intentionally the only trigger; a cursor click is explicit.
  }, [detail]);
  useEffect(() => () => recipientController.current?.abort(), []);

  return (
    <section aria-labelledby="campaign-touch-plan-read-title">
      <h2 id="campaign-touch-plan-read-title">本地触达计划审阅</h2>
      <label>
        Campaign
        <select
          aria-label="只读 Campaign"
          value={campaignCode}
          disabled={campaignState !== "loaded"}
          onChange={(event) => setCampaignCode(event.currentTarget.value)}
        >
          <option value="">请选择 Campaign</option>
          {campaigns.map((campaign) => (
            <option key={campaign.code} value={campaign.code}>
              {campaign.name}
            </option>
          ))}
        </select>
      </label>
      {campaignState === "unavailable" ? (
        <p role="alert">Campaign 列表响应不符合安全合同。</p>
      ) : null}
      {campaignCode ? (
        <label>
          本地触达计划
          <select
            aria-label="只读触达计划"
            value={selected?.id ?? ""}
            disabled={planState !== "loaded"}
            onChange={(event) =>
              setSelected(
                plans.find((plan) => plan.id === event.currentTarget.value),
              )
            }
          >
            <option value="">请选择本地计划</option>
            {plans.map((plan) => (
              <option key={plan.id} value={plan.id}>
                {plan.id}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      {planState === "unavailable" ||
      detailState === "unavailable" ||
      recipientState === "unavailable" ? (
        <p role="alert">计划或目标人员响应不符合安全合同。</p>
      ) : null}
      {detail ? (
        <>
          <dl aria-label="本地触达计划快照">
            <dt>来源</dt>
            <dd>{sourceLabel(detail)}</dd>
            <dt>目标数</dt>
            <dd>{detail.targetCount}</dd>
            <dt>步骤</dt>
            <dd>{detail.stepCount} 个本地步骤</dd>
            <dt>目标摘要</dt>
            <dd>{detail.targetDigest}</dd>
            <dt>内容摘要</dt>
            <dd>{detail.contentDigest}</dd>
          </dl>
          <section aria-labelledby="touch-plan-recipients">
            <h3 id="touch-plan-recipients">目标人员（canonical OneID）</h3>
            <ul>
              {recipients.map((id) => (
                <li key={id}>{id}</li>
              ))}
            </ul>
            {nextCursor ? (
              <>
                <p role="status">仅显示已读取的目标人员页。</p>
                <button
                  type="button"
                  disabled={recipientState === "loading"}
                  onClick={() => loadRecipients(nextCursor)}
                >
                  继续读取目标人员
                </button>
              </>
            ) : recipientState === "loaded" ? (
              <p role="status">目标人员本地快照已读取完成。</p>
            ) : null}
          </section>
        </>
      ) : null}
    </section>
  );
}
export function CampaignTouchPlanReadPanel(props: Props): React.ReactElement {
  return (
    <ReadPanelInner
      key={`${props.actorID ?? 0}:${props.transport === undefined ? "generated" : "injected"}`}
      {...props}
    />
  );
}
