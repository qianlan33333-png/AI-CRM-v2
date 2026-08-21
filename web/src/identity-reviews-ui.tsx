import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  IdentityReviewListController,
  approveIdentityReview,
  generatedIdentityReviewTransport,
  identityMergeReviewStatuses,
  rejectIdentityReview,
  validIdentityMergeReviewStatus,
  type IdentityMergeReviewRecord,
  type IdentityMergeReviewStatus,
  type IdentityReviewFailure,
  type IdentityReviewListSnapshot,
  type IdentityReviewRole,
  type IdentityReviewTransport,
} from "./identity-reviews";
import "./identity-reviews.css";

export interface IdentityMergeReviewsPageProps {
  readonly role: IdentityReviewRole;
  readonly transport?: IdentityReviewTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
  readonly initialStatus?: IdentityMergeReviewStatus;
}

type CommandKeys = { readonly approve: string; readonly reject: string };
type MutationKind = "approve" | "reject";

const failureMessages: Record<IdentityReviewFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有人工审核的访问权限。",
  not_found: "这条审核记录已不存在，请刷新后重试。",
  conflict: "审核版本或客户归属已变化，本次操作未确认。请刷新后重新审阅。",
  invalid: "服务端事实或提交内容不符合封闭契约，请刷新后重试。",
  unavailable: "人工审核服务暂时不可用，已保留最近一次验证通过的数据。",
};

const statusLabels: Record<IdentityMergeReviewStatus, string> = {
  pending: "待审核",
  approved: "已批准",
  rejected: "已拒绝",
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
  return type === "phone" ? "已验证手机号冲突" : "跨平台标识冲突";
}

function displayDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function initialSnapshot(
  status: IdentityMergeReviewStatus,
  loading: boolean,
): IdentityReviewListSnapshot {
  return { activeStatus: status, loading, loadingMore: false };
}

export function IdentityMergeReviewsPage({
  role,
  transport = generatedIdentityReviewTransport,
  readCookie = browserCookie,
  onUnauthenticated,
  initialStatus = "pending",
}: IdentityMergeReviewsPageProps): React.ReactElement {
  const canReview = role === "admin" || role === "ops";
  const safeInitialStatus = validIdentityMergeReviewStatus(initialStatus)
    ? initialStatus
    : "pending";
  const controllerRef = useRef<IdentityReviewListController>();
  const mutationGenerationRef = useRef(0);
  const mutationLockRef = useRef(false);
  const unauthenticatedNotifiedRef = useRef(false);
  const mountedRef = useRef(false);

  const [snapshot, setSnapshot] = useState<IdentityReviewListSnapshot>(() =>
    initialSnapshot(safeInitialStatus, canReview),
  );
  const [selected, setSelected] = useState<IdentityMergeReviewRecord>();
  const [primaryCustomerID, setPrimaryCustomerID] = useState<number>();
  const [reason, setReason] = useState("");
  const [keys, setKeys] = useState(newIdentityReviewCommandKeys);
  const [busy, setBusy] = useState<MutationKind>();
  const [notice, setNotice] = useState<string>();

  const notifyUnauthenticatedOnce = useCallback(() => {
    if (unauthenticatedNotifiedRef.current) return;
    unauthenticatedNotifiedRef.current = true;
    onUnauthenticated?.();
  }, [onUnauthenticated]);

  const invalidateMutationOwner = useCallback(() => {
    mutationGenerationRef.current += 1;
    mutationLockRef.current = false;
    setBusy(undefined);
  }, []);

  const resetSelection = useCallback(() => {
    setSelected(undefined);
    setPrimaryCustomerID(undefined);
    setReason("");
    setKeys(newIdentityReviewCommandKeys());
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    if (!canReview) {
      setSnapshot(initialSnapshot(safeInitialStatus, false));
      return () => {
        mountedRef.current = false;
        invalidateMutationOwner();
      };
    }

    const controller = new IdentityReviewListController(
      transport,
      safeInitialStatus,
      notifyUnauthenticatedOnce,
    );
    controllerRef.current = controller;
    const unsubscribe = controller.subscribe((next) => {
      if (controllerRef.current === controller) setSnapshot(next);
    });
    void controller.activate(safeInitialStatus);

    return () => {
      mountedRef.current = false;
      invalidateMutationOwner();
      unsubscribe();
      controller.dispose();
      if (controllerRef.current === controller) controllerRef.current = undefined;
    };
  }, [
    canReview,
    invalidateMutationOwner,
    notifyUnauthenticatedOnce,
    safeInitialStatus,
    transport,
  ]);

  useEffect(() => {
    if (!selected) return;
    const remainsVisible =
      selected.status === snapshot.activeStatus &&
      snapshot.page?.items.some(
        ({ reviewID, version }) =>
          reviewID === selected.reviewID && version === selected.version,
      );
    if (!remainsVisible) resetSelection();
  }, [resetSelection, selected, snapshot.activeStatus, snapshot.page]);

  const choose = (review: IdentityMergeReviewRecord) => {
    invalidateMutationOwner();
    setSelected(review);
    setPrimaryCustomerID(undefined);
    setReason("");
    setKeys(newIdentityReviewCommandKeys());
    setNotice(undefined);
  };

  const switchStatus = (status: IdentityMergeReviewStatus) => {
    if (status === snapshot.activeStatus) return;
    invalidateMutationOwner();
    resetSelection();
    setNotice(undefined);
    void controllerRef.current?.activate(status);
  };

  const refresh = () => {
    invalidateMutationOwner();
    resetSelection();
    setNotice(undefined);
    void controllerRef.current?.refresh();
  };

  const loadMore = () => {
    setNotice(undefined);
    void controllerRef.current?.loadMore();
  };

  const csrfToken = (): string | undefined => {
    try {
      return readCSRFCookie(readCookie());
    } catch {
      return undefined;
    }
  };

  const notifyFailure = (failure: IdentityReviewFailure) => {
    setNotice(failureMessages[failure]);
    if (failure === "unauthenticated") notifyUnauthenticatedOnce();
  };

  const runMutation = async (kind: MutationKind) => {
    if (
      mutationLockRef.current ||
      !selected ||
      selected.status !== "pending" ||
      snapshot.activeStatus !== "pending"
    ) {
      return;
    }
    if (kind === "approve" && primaryCustomerID === undefined) return;
    const token = csrfToken();
    if (!token) {
      notifyFailure("invalid");
      return;
    }

    mutationLockRef.current = true;
    const owner = ++mutationGenerationRef.current;
    setBusy(kind);
    setNotice(undefined);
    const result =
      kind === "approve"
        ? await approveIdentityReview(
            transport,
            selected,
            primaryCustomerID as number,
            reason,
            token,
            keys.approve,
          )
        : await rejectIdentityReview(
            transport,
            selected,
            reason,
            token,
            keys.reject,
          );

    if (
      !mountedRef.current ||
      owner !== mutationGenerationRef.current ||
      controllerRef.current === undefined
    ) {
      return;
    }
    mutationLockRef.current = false;
    setBusy(undefined);
    if (result.status !== "completed") {
      notifyFailure(result.status);
      return;
    }

    controllerRef.current.acceptResolution(result.review);
    resetSelection();
    setNotice(
      `服务端已确认审核 #${result.review.reviewID} 为${statusLabels[result.review.status]}；已从待审核列表移除。`,
    );
  };

  if (!canReview) {
    return (
      <section className="identity-reviews" aria-labelledby="app-title">
        <p className="route-card__eyebrow">Identity</p>
        <h1 id="app-title">OneID 人工审核</h1>
        <p className="identity-reviews__state" role="alert">
          当前账号没有人工审核的访问权限。
        </p>
      </section>
    );
  }

  const page = snapshot.page;
  const reasonIsValid =
    reason.trim() === reason && reason.length >= 1 && reason.length <= 500;
  const isPendingSelection =
    snapshot.activeStatus === "pending" && selected?.status === "pending";

  return (
    <section className="identity-reviews" aria-labelledby="app-title">
      <header className="identity-reviews__heading">
        <div>
          <p className="route-card__eyebrow">Identity · Manual Review</p>
          <h1 id="app-title">OneID 人工审核</h1>
          <p>
            仅展示封闭审核事实与候选 OneID；不展示任何原始标识值、手机号值、
            跨平台标识值、审核指纹或原始 Provider 载荷。
          </p>
        </div>
        <button
          type="button"
          disabled={snapshot.loading || snapshot.loadingMore || busy !== undefined}
          onClick={refresh}
        >
          刷新当前状态
        </button>
      </header>

      <nav className="identity-reviews__tabs" aria-label="审核状态">
        {identityMergeReviewStatuses.map((status) => (
          <button
            key={status}
            type="button"
            aria-current={snapshot.activeStatus === status ? "page" : undefined}
            disabled={busy !== undefined}
            onClick={() => switchStatus(status)}
          >
            {statusLabels[status]}
          </button>
        ))}
      </nav>

      {(notice || snapshot.failure) && (
        <p className="identity-reviews__notice" role="alert">
          {notice ?? failureMessages[snapshot.failure as IdentityReviewFailure]}
        </p>
      )}

      <div className="identity-reviews__grid">
        <section
          className="identity-reviews__panel"
          aria-labelledby="review-list-title"
        >
          <div className="identity-reviews__panel-heading">
            <h2 id="review-list-title">{statusLabels[snapshot.activeStatus]}列表</h2>
            {page && <span>{page.items.length} 条已验证记录</span>}
          </div>

          {snapshot.loading && !page && <p role="status">正在读取审核记录…</p>}
          {!snapshot.loading && !page && snapshot.failure && (
            <div role="alert">
              <p>{failureMessages[snapshot.failure]}</p>
              <button type="button" onClick={refresh}>
                重试
              </button>
            </div>
          )}
          {page && (
            <>
              {page.items.length === 0 ? (
                <p className="identity-reviews__empty">
                  当前没有{statusLabels[snapshot.activeStatus]}记录。
                </p>
              ) : (
                <ol className="identity-review-list">
                  {page.items.map((review) => (
                    <li key={review.reviewID}>
                      <button
                        aria-pressed={selected?.reviewID === review.reviewID}
                        type="button"
                        onClick={() => choose(review)}
                      >
                        <span className="identity-review-list__type">
                          {reviewTypeLabel(review.type)}
                        </span>
                        <strong>审核 #{review.reviewID}</strong>
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
              )}
              {page.nextCursor && (
                <button
                  type="button"
                  disabled={snapshot.loadingMore || snapshot.loading || busy !== undefined}
                  onClick={loadMore}
                >
                  {snapshot.loadingMore ? "正在加载…" : "加载下一页"}
                </button>
              )}
            </>
          )}
        </section>

        <section
          className="identity-reviews__panel identity-review-detail"
          aria-labelledby="review-detail-title"
        >
          <h2 id="review-detail-title">
            {snapshot.activeStatus === "pending" ? "审阅与决策" : "只读审核历史"}
          </h2>
          {!selected ? (
            <p className="identity-reviews__empty">
              选择一条{statusLabels[snapshot.activeStatus]}记录查看封闭事实。
            </p>
          ) : (
            <>
              <dl className="identity-review-facts">
                <div>
                  <dt>审核编号</dt>
                  <dd>#{selected.reviewID}</dd>
                </div>
                <div>
                  <dt>状态</dt>
                  <dd>{statusLabels[selected.status]}</dd>
                </div>
                <div>
                  <dt>冲突类型</dt>
                  <dd>{reviewTypeLabel(selected.type)}</dd>
                </div>
                <div>
                  <dt>Scope</dt>
                  <dd>{selected.scope}</dd>
                </div>
                <div>
                  <dt>候选 OneID</dt>
                  <dd>
                    {selected.customerIDs[0]}、{selected.customerIDs[1]}
                  </dd>
                </div>
                <div>
                  <dt>版本</dt>
                  <dd>{selected.version}</dd>
                </div>
                <div>
                  <dt>创建时间</dt>
                  <dd>{displayDate(selected.createdAt)}</dd>
                </div>
                {selected.resolvedAt && (
                  <div>
                    <dt>决议时间</dt>
                    <dd>{displayDate(selected.resolvedAt)}</dd>
                  </div>
                )}
              </dl>

              {isPendingSelection && (
                <fieldset disabled={busy !== undefined}>
                  <fieldset className="identity-review-candidates">
                    <legend>批准时选择主 OneID</legend>
                    <p>仅批准操作需要选择；拒绝不会提交主 OneID。</p>
                    {selected.customerIDs.map((customerID) => (
                      <label key={customerID}>
                        <input
                          type="radio"
                          name="primary-customer"
                          checked={primaryCustomerID === customerID}
                          onChange={() => {
                            setPrimaryCustomerID(customerID);
                            setKeys(newIdentityReviewCommandKeys());
                            setNotice(undefined);
                          }}
                        />
                        <span>OneID {customerID}</span>
                        <a href={`/customers/${customerID}`}>查看客户事实</a>
                      </label>
                    ))}
                  </fieldset>

                  <label className="identity-review-reason">
                    <span>审核理由（1–500 字）</span>
                    <textarea
                      rows={5}
                      maxLength={500}
                      value={reason}
                      onChange={(event) => {
                        setReason(event.target.value);
                        setKeys(newIdentityReviewCommandKeys());
                        setNotice(undefined);
                      }}
                    />
                    <span>{reason.length}/500</span>
                  </label>

                  <div className="identity-review-actions">
                    <button
                      type="button"
                      disabled={
                        !reasonIsValid || primaryCustomerID === undefined || busy !== undefined
                      }
                      onClick={() => void runMutation("approve")}
                    >
                      {busy === "approve" ? "正在确认…" : "批准并合并"}
                    </button>
                    <button
                      className="identity-review-actions__reject"
                      type="button"
                      disabled={!reasonIsValid || busy !== undefined}
                      onClick={() => void runMutation("reject")}
                    >
                      {busy === "reject" ? "正在确认…" : "拒绝合并"}
                    </button>
                  </div>
                </fieldset>
              )}
            </>
          )}
        </section>
      </div>
    </section>
  );
}
