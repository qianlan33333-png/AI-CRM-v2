import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  createPublicSubmissionFlight,
  createPublicSurveyController,
  generatedPublicSurveyTransport,
  PublicSurveyFailure,
  PublicSurveyInputError,
  type PublicAnswer,
  type PublicDefinition,
  type PublicSurveyTransport,
} from "./public-survey";

type State = "loading" | "ready" | "submitting" | "accepted" | "unknown" | "error";

function submissionKey(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
}

export function PublicSurveyPage({
  slug,
  transport = generatedPublicSurveyTransport,
}: {
  readonly slug: string;
  readonly transport?: PublicSurveyTransport;
}) {
  const controller = useMemo(
    () => createPublicSurveyController(transport, `/q/${slug}`),
    [slug, transport],
  );
  const [definition, setDefinition] = useState<PublicDefinition>();
  const [answers, setAnswers] = useState<readonly PublicAnswer[]>([]);
  const [state, setState] = useState<State>("loading");
  const [notice, setNotice] = useState<string>();
  const [retryBlocked, setRetryBlocked] = useState(false);
  const flight = useRef(createPublicSubmissionFlight(submissionKey));
  const generation = useRef(0);
  const submitToken = useRef<symbol>();
  const outcomeUnknown = useRef(false);

  useEffect(() => {
    const active = ++generation.current;
    setDefinition(undefined);
    setAnswers([]);
    setNotice(undefined);
    setRetryBlocked(false);
    if (outcomeUnknown.current) {
      setState("unknown");
      return undefined;
    }
    flight.current = createPublicSubmissionFlight(submissionKey);
    setState("loading");
    void controller.load().then(
      (next) => {
        if (generation.current !== active) return;
        setDefinition(next);
        setState("ready");
      },
      () => {
        if (generation.current === active) setState("error");
      },
    );
    return () => {
      if (generation.current !== active) return;
      generation.current += 1;
      if (submitToken.current !== undefined) {
        submitToken.current = undefined;
        outcomeUnknown.current = true;
        flight.current.invalidate();
      }
    };
  }, [controller]);

  const choose = (questionID: number, optionID: number, multiple: boolean, checked: boolean) => {
    setAnswers((current) => {
      const previous = current.find((answer) => answer.question_id === questionID)?.option_ids ?? [];
      const optionIDs = multiple
        ? checked
          ? [...previous, optionID]
          : previous.filter((id) => id !== optionID)
        : [optionID];
      return [...current.filter((answer) => answer.question_id !== questionID), { question_id: questionID, option_ids: optionIDs }];
    });
  };

  const submit = () => {
    if (!definition || state !== "ready" || submitToken.current !== undefined || outcomeUnknown.current) return;
    const token = Symbol("public-survey-submit");
    const active = generation.current;
    submitToken.current = token;
    setState("submitting");
    setNotice(undefined);
    void flight.current
      .submit((key) =>
        controller
          .submit({ version: definition.version, submission_key: key, answers })
          .then(async () => {
            if (generation.current !== active || submitToken.current !== token)
              throw new PublicSurveyInputError("stale public submission");
            try {
              return await controller.result();
            } catch {
              if (generation.current !== active || submitToken.current !== token)
                throw new PublicSurveyInputError("stale public submission");
              // A strict 202 receipt already confirms the local write. A later
              // result-read failure must never invite a duplicate submission.
              setNotice("提交已在本地受理，结果查询暂不可用。请勿重复提交。");
              return undefined;
            }
          }),
      )
      .then(
        () => {
          if (generation.current === active && submitToken.current === token) {
            setAnswers([]);
            setState("accepted");
          }
        },
        (error: unknown) => {
          if (generation.current !== active || submitToken.current !== token) return;
          if (error instanceof PublicSurveyInputError) {
            setState("ready");
            setNotice("请完成必填题，并确认每题的选择数量。");
            return;
          }
          if (error instanceof PublicSurveyFailure && error.kind !== "unknown") {
            setState("ready");
            if (error.kind === "conflict") {
              setRetryBlocked(true);
              setNotice("提交编号冲突，未重发。请刷新后核对提交结果，再决定下一步。");
            } else if (error.kind === "invalid") {
              setNotice("答案不符合问卷规则，请修正后重试。");
            } else if (error.kind === "not_found") {
              setNotice("问卷已不可用；草稿已保留。");
            } else {
              setNotice("提交频率受限；草稿已保留，请稍后重试。");
            }
            return;
          }
          outcomeUnknown.current = true;
          setState("unknown");
        },
      )
      .finally(() => {
        if (generation.current === active && submitToken.current === token)
          submitToken.current = undefined;
      });
  };

  if (state === "loading") return <main className="route-card"><p>正在加载问卷…</p></main>;
  if (state === "error" || !definition) return <main className="route-card"><h1>问卷暂不可用</h1><p>请稍后重试。</p></main>;
  if (state === "accepted") return <main className="route-card"><h1>提交已受理</h1><p>本次问卷仅在本地受理，不会触发外部动作。</p>{notice && <p role="alert">{notice}</p>}</main>;
  if (state === "unknown") return <main className="route-card"><h1>提交状态待确认</h1><p>为避免重复提交，本页面不会自动重试；请稍后刷新并联系运营人员核查。</p></main>;
  return (
    <main className="route-card">
      <h1>{definition.title}</h1>
      <p>{definition.description}</p>
      {notice && <p role="alert">{notice}</p>}
      <form onSubmit={(event) => { event.preventDefault(); submit(); }}>
        {definition.questions.map((question) => (
          <fieldset key={question.id} disabled={state === "submitting"}>
            <legend>{question.title}</legend>
            {question.options.map((option) => {
              const selected = answers.find((answer) => answer.question_id === question.id)?.option_ids.includes(option.id) ?? false;
              return <label key={option.id}><input type={question.type === "multi_choice" ? "checkbox" : "radio"} name={`q-${question.id}`} value={option.id} checked={selected} onChange={(event) => choose(question.id, option.id, question.type === "multi_choice", event.currentTarget.checked)} />{option.option_text}</label>;
            })}
          </fieldset>
        ))}
        <button disabled={state === "submitting" || retryBlocked} type="submit">提交</button>
      </form>
    </main>
  );
}
