import React, { useCallback, useEffect, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  appendIdentityMergeReviewPage,
  approveIdentityReview,
  generatedIdentityReviewTransport,
  loadIdentityMergeReviews,
  rejectIdentityReview,
  type IdentityMergeReviewPage,
  type IdentityMergeReviewRecord,
  type IdentityReviewFailure,
  type IdentityReviewRole,
  type IdentityReviewTransport,
} from "./identity-reviews";
import "./identity-reviews.css";

export interface IdentityMergeReviewsPageProps {
  readonly role: IdentityReviewRole;
  readonly transport?: IdentityReviewTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}

type ListState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly page: IdentityMergeReviewPage }
  | { readonly kind: "error"; readonly failure: IdentityReviewFailure };

type CommandKeys = { readonly approve: string; readonly reject: string };

const failureMessages: Record<IdentityReviewFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有人工待合并的操作权限。",
  not_found: "这条待办已不存在，请刷新列表后重试。",
  conflict: "待办版本或客户归属已变化，本次操作未执行。请刷新后重新审阅。",
  invalid: "服务端事实或提交内容不符合冻结契约，请刷新后重试。",
  unavailable: "人工待合并服务暂时不可用，请稍后重试。",
};

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

export function newIdentityReviewCommandKeys(): CommandKeys {
  const entropy = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 14)}`;
  return {
    approve: `identity-review-approve-${entropy}`,
    reject: `identity-review-reject-${entropy}`,
  };
}

function reviewTypeLabel(type: IdentityMergeReviewRecord["type"]): string {
  return type === "phone" ? "已验证手机号" : "UnionID";
}

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function IdentityMergeReviewsPage({
  role,
  transport = generatedIdentityReviewTransport,
  readCookie = browserCookie,
  onUnauthenticated,
}: IdentityMergeReviewsPageProps): React.ReactElement {
  const canReview = role === "admin" || role === "ops";
  const [list, setList] = useState<ListState>({ kind: "loading" });
  const [selected, setSelected] = useState<IdentityMergeReviewRecord>();
  const [primaryCustomerID, setPrimaryCustomerID] = useState<number>();
  const [reason, setReason] = useState("");
  const [keys, setKeys] = useState(newIdentityReviewCommandKeys);
  const [busy, setBusy] = useState<"approve" | "reject">();
  const [loadingMore, setLoadingMore] = useState(false);
  const [notice, setNotice] = useState<string>();

  const notifyFailure = useCallback(
    (failure: IdentityReviewFailure) => {
      setNotice(failureMessages[failure]);
      if (failure === "unauthenticated") onUnauthenticated?.();
    },
    [onUnauthenticated],
  );

  const loadFirst = useCallback(async () => {
    setList({ kind: "loading" });
    setSelected(undefined);
    setPrimaryCustomerID(undefined);
    setReason("");
    setKeys(newIdentityReviewCommandKeys());
    setNotice(undefined);
    const result = await loadIdentityMergeReviews(transport);
    if (result.status === "loaded") {
      setList({ kind: "ready", page: result.page });
      return;
    }
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setList({ kind: "error", failure: result.status });
  }, [onUnauthenticated, transport]);

  useEffect(() => {
    if (canReview) void loadFirst();
  }, [canReview, loadFirst]);

  const loadMore = async () => {
    if (list.kind !== "ready" || !list.page.nextCursor || loadingMore) return;
    setLoadingMore(true);
    setNotice(undefined);
    const result = await loadIdentityMergeReviews(
      transport,
      list.page.nextCursor,
    );
    setLoadingMore(false);
    if (result.status !== "loaded") {
      notifyFailure(result.status);
      return;
    }
    const page = appendIdentityMergeReviewPage(list.page, result.page);
    if (!page) {
      notifyFailure("invalid");
      return;
    }
    setList({ kind: "ready", page });
  };

  const choose = (review: IdentityMergeReviewRecord) => {
    setSelected(review);
    setPrimaryCustomerID(undefined);
    setReason("");
    setKeys(newIdentityReviewCommandKeys());
    setNotice(undefined);
  };

  const updateReason = (value: string) => {
    setReason(value);
    setKeys(newIdentityReviewCommandKeys());
    setNotice(undefined);
  };

  const updatePrimary = (customerID: number) => {
    setPrimaryCustomerID(customerID);
    setKeys(newIdentityReviewCommandKeys());
    setNotice(undefined);
  };

  const csrfToken = (): string | undefined => {
    try {
      return readCSRFCookie(readCookie());
    } catch {
      return undefined;
    }
  };

  const completeLocally = (
    resolved: IdentityMergeReviewRecord,
    message: string,
  ) => {
    setList((current) =>
      current.kind === "ready"
        ? {
            kind: "ready",
            page: {
              ...current.page,
              items: current.page.items.filter(
                ({ reviewID }) => reviewID !== resolved.reviewID,
              ),
            },
          }
        : current,
    );
    setSelected(undefined);
    setPrimaryCustomerID(undefined);
    setReason("");
    setNotice(message);
  };

  const approve = async () => {
    if (!selected || primaryCustomerID === undefined || busy) return;
    const token = csrfToken();
    if (!token) {
      setNotice("安全令牌缺失，未发送批准请求。");
      return;
    }
    const normalizedReason = reason.trim();
    setBusy("approve");
    setNotice(undefined);
    const result = await approveIdentityReview(
      transport,
      selected,
      primaryCustomerID,
      normalizedReason,
      token,
      keys.approve,
    );
    setBusy(undefined);
    if (result.status !== "completed") {
      notifyFailure(result.status);
      return;
    }
    completeLocally(
      result.review,
      `待办 #${result.review.reviewID} 已批准；主客户为 OneID ${primaryCustomerID}。`,
    );
  };

  const reject = async () => {
    if (!selected || busy) return;
    const token = csrfToken();
    if (!token) {
      setNotice("安全令牌缺失，未发送拒绝请求。");
      return;
    }
    const normalizedReason = reason.trim();
    setBusy("reject");
    setNotice(undefined);
    const result = await rejectIdentityReview(
      transport,
      selected,
      normalizedReason,
      token,
      keys.reject,
    );
    setBusy(undefined);
    if (result.status !== "completed") {
      notifyFailure(result.status);
      return;
    }
    completeLocally(
      result.review,
      `待办 #${result.review.reviewID} 已拒绝；客户绑定保持不变。`,
    );
  };

  if (!canReview) {
    return (
      <section className="identity-reviews" aria-labelledby="app-title">
        <p className="route-card__eyebrow">Identity</p>
        <h1 id="app-title">人工待合并</h1>
        <p className="identity-reviews__state" role="alert">
          当前账号没有人工待合并的访问权限。
        </p>
      </section>
    );
  }

  const reasonIsValid =
    reason.trim().length >= 1 && reason.trim().length <= 500;

  return (
    <section className="identity-reviews" aria-labelledby="app-title">
      <header className="identity-reviews__heading">
        <div>
          <p className="route-card__eyebrow">Identity · Manual Review</p>
          <h1 id="app-title">人工待合并</h1>
          <p>
            审阅已验证手机号等跨客户冲突。页面只展示去标识指纹与候选
            OneID，不展示原始标识值。
          </p>
        </div>
        <button
          type="button"
          disabled={busy !== undefined}
          onClick={() => void loadFirst()}
        >
          刷新待办
        </button>
      </header>

      {notice && (
        <p className="identity-reviews__notice" role="alert">
          {notice}
        </p>
      )}

      <div className="identity-reviews__grid">
        <section
          className="identity-reviews__panel"
          aria-labelledby="review-list-title"
        >
          <div className="identity-reviews__panel-heading">
            <h2 id="review-list-title">待合并列表</h2>
            {list.kind === "ready" && (
              <span>{list.page.items.length} 条待审</span>
            )}
          </div>
          {list.kind === "loading" && <p role="status">正在读取待合并事项…</p>}
          {list.kind === "error" && (
            <div role="alert">
              <p>{failureMessages[list.failure]}</p>
              <button type="button" onClick={() => void loadFirst()}>
                重试
              </button>
            </div>
          )}
          {list.kind === "ready" && (
            <>
              <ol className="identity-review-list">
                {list.page.items.map((review) => (
                  <li key={review.reviewID}>
                    <button
                      aria-pressed={selected?.reviewID === review.reviewID}
                      type="button"
                      onClick={() => choose(review)}
                    >
                      <span className="identity-review-list__type">
                        {reviewTypeLabel(review.type)}
                      </span>
                      <strong>待办 #{review.reviewID}</strong>
                      <span>
                        OneID {review.customerIDs[0]} ↔ {review.customerIDs[1]}
                      </span>
                      <time dateTime={review.createdAt}>
                        {displayDate(review.createdAt)}
                      </time>
                    </button>
                  </li>
                ))}
              </ol>
              {list.page.items.length === 0 && (
                <p className="identity-reviews__empty" role="status">
                  当前没有待审的合并事项。
                </p>
              )}
              {list.page.nextCursor && (
                <button
                  type="button"
                  disabled={loadingMore}
                  onClick={() => void loadMore()}
                >
                  {loadingMore ? "正在读取…" : "读取更多待办"}
                </button>
              )}
            </>
          )}
        </section>

        <section
          className="identity-reviews__panel identity-review-detail"
          aria-labelledby="review-detail-title"
        >
          <h2 id="review-detail-title">审阅与决策</h2>
          {!selected && (
            <p>请从左侧选择一条待办，核对两个候选客户后作出决定。</p>
          )}
          <fieldset disabled={!selected || busy !== undefined}>
            <legend className="sr-only">人工待合并决策</legend>
            {selected && (
              <>
                <dl className="identity-review-facts">
                  <div>
                    <dt>待办</dt>
                    <dd>
                      #{selected.reviewID} · 版本 {selected.version}
                    </dd>
                  </div>
                  <div>
                    <dt>标识类型</dt>
                    <dd>{reviewTypeLabel(selected.type)}</dd>
                  </div>
                  <div>
                    <dt>作用域</dt>
                    <dd>{selected.scope}</dd>
                  </div>
                  <div>
                    <dt>去标识指纹</dt>
                    <dd>
                      <code>{selected.identityFingerprint}</code>
                    </dd>
                  </div>
                </dl>
                <fieldset className="identity-review-candidates">
                  <legend>批准时选择主客户</legend>
                  <p>
                    必须主动选择一个主客户；另一个客户将按服务端冻结事务规则归并。
                  </p>
                  {selected.customerIDs.map((customerID) => (
                    <label key={customerID}>
                      <input
                        type="radio"
                        name="primary-customer"
                        checked={primaryCustomerID === customerID}
                        onChange={() => updatePrimary(customerID)}
                      />
                      <span>OneID {customerID}</span>
                      <a href={`/customers/${customerID}`}>查看客户</a>
                    </label>
                  ))}
                </fieldset>
              </>
            )}
            <label className="identity-review-reason">
              决策理由
              <textarea
                maxLength={500}
                rows={5}
                value={reason}
                placeholder="填写人工核验依据（1–500 字）"
                onChange={(event) => updateReason(event.currentTarget.value)}
              />
              <span>{reason.length}/500</span>
            </label>
            <div className="identity-review-actions">
              <button
                className="identity-review-actions__approve"
                type="button"
                disabled={
                  !reasonIsValid ||
                  primaryCustomerID === undefined ||
                  busy !== undefined
                }
                onClick={() => void approve()}
              >
                {busy === "approve" ? "正在批准…" : "批准并合并"}
              </button>
              <button
                className="identity-review-actions__reject"
                type="button"
                disabled={!reasonIsValid || busy !== undefined}
                onClick={() => void reject()}
              >
                {busy === "reject" ? "正在拒绝…" : "拒绝合并"}
              </button>
            </div>
          </fieldset>
        </section>
      </div>
    </section>
  );
}
