import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { readCSRFCookie } from "./auth";
import {
  archiveCoupon,
  canArchiveCoupon,
  createCouponDraft,
  copyCoupon,
  couponClaimsPageSize,
  filterCoupons,
  generatedCouponsTransport,
  loadCouponClaims,
  loadCouponDetail,
  loadCouponShare,
  loadCoupons,
  newCouponArchiveIdempotencyKey,
  newCouponCopyIdempotencyKey,
  type CouponAvailabilityFilter,
  type CouponArchiveResult,
  type CouponClaimItem,
  type CouponClaimsResult,
  type CouponCopyResult,
  type CouponDraftInput,
  type CouponDraftMutationResult,
  type CouponDetailResult,
  type CouponListItem,
  type CouponRuleDetail,
  type CouponShareResult,
  type CouponListResult,
  type CouponsFailure,
  type CouponsRole,
  type CouponsTransport,
  updateCouponDraft,
} from "./coupons";

const messages: Record<CouponsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有优惠券管理权限。",
  not_found: "要复制的优惠券已不存在，请刷新后重试。",
  conflict: "复制请求与已有操作冲突，请刷新后重试。",
  invalid: "优惠券响应或请求不符合已冻结合同。",
  unavailable: "本地优惠券服务暂不可用，请稍后重试。",
};

const shareMessages: Record<CouponsFailure, string> = {
  ...messages,
  not_found: "要生成链接的优惠券已不存在，请刷新后重试。",
  conflict: "只有已发布的本地优惠券可以生成分享链接。",
};

const archiveMessages: Record<CouponsFailure, string> = {
  ...messages,
  not_found: "要归档的优惠券已不存在，请刷新后重试。",
  conflict: "该优惠券当前不能归档，请刷新后重试。",
};

const detailMessages: Record<CouponsFailure, string> = {
  ...messages,
  not_found: "该优惠券规则已不存在，请刷新列表后重试。",
};

export type CouponsViewState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly items: readonly CouponListItem[] }
  | { readonly kind: "error"; readonly failure: CouponsFailure };

type CouponClaimsPage = {
  readonly items: readonly CouponClaimItem[];
  readonly total: number;
  readonly offset: number;
};

export type CouponClaimsViewState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly coupon: CouponListItem;
      readonly offset: number;
      readonly previous?: CouponClaimsPage;
    }
  | {
      readonly kind: "ready";
      readonly coupon: CouponListItem;
      readonly page: CouponClaimsPage;
    }
  | {
      readonly kind: "error";
      readonly coupon: CouponListItem;
      readonly failure: CouponsFailure;
      readonly previous?: CouponClaimsPage;
    };

export type CouponShareViewState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly coupon: CouponListItem;
      readonly previous?: string;
    }
  | {
      readonly kind: "ready";
      readonly coupon: CouponListItem;
      readonly url: string;
      readonly copyStatus?: "copied" | "manual";
    }
  | {
      readonly kind: "error";
      readonly coupon: CouponListItem;
      readonly failure: CouponsFailure;
      readonly previous?: string;
    };

export type CouponDetailViewState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "loading";
      readonly coupon: CouponListItem;
      readonly previous?: CouponRuleDetail;
    }
  | {
      readonly kind: "ready";
      readonly coupon: CouponListItem;
      readonly detail: CouponRuleDetail;
    }
  | {
      readonly kind: "error";
      readonly coupon: CouponListItem;
      readonly failure: CouponsFailure;
      readonly previous?: CouponRuleDetail;
    };

export type CouponEditorState =
  | { readonly kind: "idle" }
  | { readonly kind: "new" }
  | { readonly kind: "edit"; readonly detail: CouponRuleDetail };

export interface CouponCopyInput {
  readonly couponID: number;
  readonly idempotencySource?: { readonly randomUUID: () => string };
  readonly readCookie: () => string;
  readonly transport: CouponsTransport;
}

export interface CouponArchiveInput {
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly confirm: (message: string) => boolean;
  readonly idempotencySource?: { readonly randomUUID: () => string };
  readonly item: CouponListItem;
  readonly readCookie: () => string;
  readonly transport: CouponsTransport;
}

export async function performCouponCopy({
  couponID,
  idempotencySource,
  readCookie,
  transport,
}: CouponCopyInput): Promise<CouponCopyResult> {
  let csrf: string | undefined;
  try {
    csrf = readCSRFCookie(readCookie());
  } catch {
    csrf = undefined;
  }
  const idempotencyKey = newCouponCopyIdempotencyKey(idempotencySource);
  if (!csrf) return { status: "forbidden" };
  if (!idempotencyKey) return { status: "unavailable" };
  return copyCoupon(transport, couponID, csrf, idempotencyKey);
}

export async function performCouponArchive({
  confirm,
  idempotencySource,
  item,
  readCookie,
  transport,
}: CouponArchiveInput): Promise<CouponArchiveResult> {
  if (!canArchiveCoupon(item)) return { status: "invalid" };
  try {
    if (!confirm(`确认归档本地优惠券“${item.name}”吗？`))
      return { status: "canceled" };
  } catch {
    return { status: "canceled" };
  }
  let csrf: string | undefined;
  try {
    csrf = readCSRFCookie(readCookie());
  } catch {
    csrf = undefined;
  }
  const idempotencyKey = newCouponArchiveIdempotencyKey(idempotencySource);
  if (!csrf) return { status: "forbidden" };
  if (!idempotencyKey) return { status: "unavailable" };
  return archiveCoupon(transport, item, csrf, idempotencyKey);
}

// This lock is deliberately independent of React state: two synchronous click
// handlers can run before React commits a disabled button state.
export function startCouponCopy(
  lock: { current: boolean },
  execute: () => Promise<void>,
): Promise<void> | undefined {
  if (lock.current) return undefined;
  lock.current = true;
  return (async () => {
    try {
      await execute();
    } finally {
      lock.current = false;
    }
  })();
}

export function startCouponArchive(
  lock: { current: boolean },
  execute: () => Promise<void>,
): Promise<void> | undefined {
  return startCouponCopy(lock, execute);
}

export function canStartCouponWrite(mutationUncertain: boolean): boolean {
  return !mutationUncertain;
}

export function startCouponDetail(
  inFlight: Set<number>,
  couponID: number,
  execute: () => Promise<void>,
): Promise<void> | undefined {
  if (!Number.isSafeInteger(couponID) || couponID < 1 || inFlight.has(couponID))
    return undefined;
  inFlight.add(couponID);
  return (async () => {
    try {
      await execute();
    } finally {
      inFlight.delete(couponID);
    }
  })();
}

export function CouponsPage({
  confirm = runtimeConfirm,
  role,
  transport = generatedCouponsTransport,
  readCookie = runtimeCookieHeader,
  onUnauthenticated,
}: {
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly confirm?: (message: string) => boolean;
  readonly role: CouponsRole;
  readonly transport?: CouponsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<CouponsViewState>({ kind: "loading" });
  const [busyID, setBusyID] = useState<number>();
  const [busyAction, setBusyAction] = useState<
    "copy" | "archive" | "create" | "update"
  >();
  const [notice, setNotice] = useState<string>();
  const [claimsState, setClaimsState] = useState<CouponClaimsViewState>({
    kind: "idle",
  });
  const [shareState, setShareState] = useState<CouponShareViewState>({
    kind: "idle",
  });
  const [detailState, setDetailState] = useState<CouponDetailViewState>({
    kind: "idle",
  });
  const [editor, setEditor] = useState<CouponEditorState>({ kind: "idle" });
  const [mutationUncertain, setMutationUncertain] = useState(false);
  const couponMutationInFlight = useRef(false);
  const claimsRequest = useRef(0);
  const claimsInFlight = useRef<string>();
  const shareRequest = useRef(0);
  const shareInFlight = useRef<number>();
  const detailRequest = useRef(0);
  const detailInFlight = useRef(new Set<number>());
  const editRequest = useRef(0);

  const reload = useCallback(
    async (preserveReady = false): Promise<CouponListResult> => {
      const result = await loadCoupons(transport);
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "loaded") {
        setState({ kind: "ready", items: result.items });
      } else if (!preserveReady) {
        setState({ kind: "error", failure: result.status });
      }
      return result;
    },
    [onUnauthenticated, transport],
  );

  useEffect(() => {
    if (!canAccess) return undefined;
    let active = true;
    void loadCoupons(transport).then((result) => {
      if (!active) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setState(
        result.status === "loaded"
          ? { kind: "ready", items: result.items }
          : { kind: "error", failure: result.status },
      );
    });
    return () => {
      active = false;
    };
  }, [canAccess, onUnauthenticated, transport]);

  const onCopy = useCallback(
    async (item: CouponListItem) => {
      if (!canStartCouponWrite(mutationUncertain)) {
        setNotice("上一笔本地草稿请求结果不确定，请先刷新列表后再继续。");
        return;
      }
      const operation = startCouponCopy(couponMutationInFlight, async () => {
        setBusyID(item.id);
        setBusyAction("copy");
        try {
          const result = await performCouponCopy({
            couponID: item.id,
            readCookie,
            transport,
          });
          if (result.status === "copied") {
            setNotice(`已复制为本地草稿“${result.item.name}”，正在刷新列表。`);
            await reload();
          } else {
            if (result.status === "unauthenticated") onUnauthenticated?.();
            setNotice(messages[result.status]);
          }
        } finally {
          setBusyAction(undefined);
          setBusyID(undefined);
        }
      });
      if (operation) await operation;
    },
    [mutationUncertain, onUnauthenticated, readCookie, reload, transport],
  );

  const onArchive = useCallback(
    async (item: CouponListItem) => {
      if (!canStartCouponWrite(mutationUncertain)) {
        setNotice("上一笔本地草稿请求结果不确定，请先刷新列表后再继续。");
        return;
      }
      const operation = startCouponArchive(couponMutationInFlight, async () => {
        setBusyID(item.id);
        setBusyAction("archive");
        try {
          const result = await performCouponArchive({
            confirm,
            item,
            readCookie,
            transport,
          });
          if (result.status === "archived") {
            setNotice(`已归档本地优惠券“${result.item.name}”，正在刷新列表。`);
            const refreshed = await reload(true);
            if (refreshed.status !== "loaded") {
              setNotice(
                `已归档本地优惠券“${result.item.name}”，${messages[refreshed.status]}列表保留原数据。`,
              );
            }
          } else if (result.status !== "canceled") {
            if (result.status === "unauthenticated") onUnauthenticated?.();
            setNotice(archiveMessages[result.status]);
          }
        } finally {
          setBusyAction(undefined);
          setBusyID(undefined);
        }
      });
      if (operation) await operation;
    },
    [
      confirm,
      mutationUncertain,
      onUnauthenticated,
      readCookie,
      reload,
      transport,
    ],
  );

  const onClaims = useCallback(
    async (item: CouponListItem, offset = 0) => {
      const key = `${item.id}:${offset}`;
      if (claimsInFlight.current === key) return;
      claimsInFlight.current = key;
      const request = ++claimsRequest.current;
      const previous =
        claimsState.kind === "ready" && claimsState.coupon.id === item.id
          ? claimsState.page
          : claimsState.kind === "error" && claimsState.coupon.id === item.id
            ? claimsState.previous
            : undefined;
      setClaimsState({ kind: "loading", coupon: item, offset, previous });
      let result: CouponClaimsResult;
      try {
        result = await loadCouponClaims(transport, item.id, offset);
      } finally {
        if (claimsInFlight.current === key) claimsInFlight.current = undefined;
      }
      if (request !== claimsRequest.current) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setClaimsState(
        result.status === "loaded"
          ? {
              kind: "ready",
              coupon: item,
              page: {
                items: result.items,
                total: result.total,
                offset: result.offset,
              },
            }
          : { kind: "error", coupon: item, failure: result.status, previous },
      );
    },
    [claimsState, onUnauthenticated, transport],
  );

  const onShare = useCallback(
    async (item: CouponListItem) => {
      if (item.status !== "published" || shareInFlight.current === item.id)
        return;
      shareInFlight.current = item.id;
      const request = ++shareRequest.current;
      const previous =
        shareState.kind === "ready" && shareState.coupon.id === item.id
          ? shareState.url
          : shareState.kind === "error" && shareState.coupon.id === item.id
            ? shareState.previous
            : undefined;
      setShareState({ kind: "loading", coupon: item, previous });
      let result: CouponShareResult;
      try {
        result = await loadCouponShare(transport, item);
      } finally {
        if (shareInFlight.current === item.id)
          shareInFlight.current = undefined;
      }
      if (request !== shareRequest.current) return;
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setShareState(
        result.status === "loaded"
          ? { kind: "ready", coupon: item, url: result.share.url }
          : { kind: "error", coupon: item, failure: result.status, previous },
      );
    },
    [onUnauthenticated, shareState, transport],
  );

  const onDetail = useCallback(
    async (item: CouponListItem) => {
      const operation = startCouponDetail(
        detailInFlight.current,
        item.id,
        async () => {
          const request = ++detailRequest.current;
          const previous =
            detailState.kind === "ready" && detailState.coupon.id === item.id
              ? detailState.detail
              : detailState.kind === "error" &&
                  detailState.coupon.id === item.id
                ? detailState.previous
                : undefined;
          setDetailState({ kind: "loading", coupon: item, previous });
          const result: CouponDetailResult = await loadCouponDetail(
            transport,
            item.id,
          );
          if (request !== detailRequest.current) return;
          if (result.status === "unauthenticated") onUnauthenticated?.();
          setDetailState(
            result.status === "loaded"
              ? { kind: "ready", coupon: item, detail: result.detail }
              : {
                  kind: "error",
                  coupon: item,
                  failure: result.status,
                  previous,
                },
          );
        },
      );
      if (operation) await operation;
    },
    [detailState, onUnauthenticated, transport],
  );

  const onCreate = useCallback(() => {
    if (!canStartCouponWrite(mutationUncertain)) {
      setNotice("上一笔本地草稿请求结果不确定，请先刷新列表后再继续。");
      return;
    }
    setEditor({ kind: "new" });
  }, [mutationUncertain]);

  const onCancelEditor = useCallback(() => setEditor({ kind: "idle" }), []);

  const onEdit = useCallback(
    async (item: CouponListItem) => {
      if (mutationUncertain || item.status !== "draft") return;
      const operation = startCouponDetail(
        detailInFlight.current,
        item.id,
        async () => {
          const request = ++editRequest.current;
          const result = await loadCouponDetail(transport, item.id);
          if (request !== editRequest.current) return;
          if (result.status === "unauthenticated") onUnauthenticated?.();
          if (result.status === "loaded" && result.detail.status === "draft") {
            setEditor({ kind: "edit", detail: result.detail });
            return;
          }
          setNotice(
            result.status === "loaded"
              ? "该规则已不再是本地草稿，请刷新列表后重试。"
              : detailMessages[result.status],
          );
        },
      );
      if (operation) await operation;
    },
    [mutationUncertain, onUnauthenticated, transport],
  );

  const onSubmitDraft = useCallback(
    async (input: CouponDraftInput) => {
      if (editor.kind === "idle") return;
      if (!canStartCouponWrite(mutationUncertain)) {
        setNotice("上一笔本地草稿请求结果不确定，请先刷新列表后再继续。");
        return;
      }
      const target = editor;
      const operation = startCouponCopy(couponMutationInFlight, async () => {
        setBusyAction(target.kind === "new" ? "create" : "update");
        setBusyID(target.kind === "edit" ? target.detail.id : 0);
        let csrf: string | undefined;
        try {
          csrf = readCSRFCookie(readCookie());
        } catch {
          csrf = undefined;
        }
        let result: CouponDraftMutationResult;
        if (!csrf) {
          result = { status: "forbidden", outcomeUncertain: false };
        } else if (target.kind === "new") {
          result = await createCouponDraft(transport, input, csrf);
        } else {
          result = await updateCouponDraft(
            transport,
            target.detail,
            input,
            csrf,
          );
        }
        try {
          if (result.status === "created" || result.status === "updated") {
            const verb = result.status === "created" ? "创建" : "更新";
            setEditor({ kind: "idle" });
            setNotice(`本地草稿“${result.item.name}”已${verb}，正在刷新列表。`);
            const refreshed = await reload(true);
            if (refreshed.status !== "loaded") {
              setNotice(
                `本地草稿“${result.item.name}”已${verb}，${messages[refreshed.status]}列表保留原数据。`,
              );
            }
          } else {
            if (result.status === "unauthenticated") onUnauthenticated?.();
            if (result.outcomeUncertain) {
              setMutationUncertain(true);
              setNotice(
                "本地草稿请求结果不确定，未自动重试。请刷新列表后人工确认是否已创建或更新。",
              );
            } else {
              setNotice(messages[result.status]);
            }
          }
        } finally {
          setBusyAction(undefined);
          setBusyID(undefined);
        }
      });
      if (operation) await operation;
    },
    [
      editor,
      mutationUncertain,
      onUnauthenticated,
      readCookie,
      reload,
      transport,
    ],
  );

  const onRefreshAfterUncertain = useCallback(async () => {
    if (!mutationUncertain) return;
    const refreshed = await reload(true);
    if (refreshed.status === "loaded") {
      setMutationUncertain(false);
      setEditor({ kind: "idle" });
      setNotice("本地列表已刷新，请人工确认后再继续操作。");
    }
  }, [mutationUncertain, reload]);

  const onCopyShare = useCallback(async () => {
    if (shareState.kind !== "ready") return;
    const copyStatus = await copyCouponShareURL(shareState.url);
    setShareState({ ...shareState, copyStatus });
  }, [shareState]);

  return (
    <CouponsView
      busyAction={busyAction}
      busyID={busyID}
      claimsState={claimsState}
      detailState={detailState}
      editor={editor}
      mutationUncertain={mutationUncertain}
      notice={notice}
      onCopy={onCopy}
      onArchive={onArchive}
      onClaims={onClaims}
      onDetail={onDetail}
      onCreate={onCreate}
      onCancelEditor={onCancelEditor}
      onEdit={onEdit}
      onSubmitDraft={onSubmitDraft}
      onRefreshAfterUncertain={onRefreshAfterUncertain}
      role={role}
      shareState={shareState}
      onShare={onShare}
      onCopyShare={onCopyShare}
      state={state}
    />
  );
}

export function CouponsView({
  busyAction,
  busyID,
  claimsState,
  detailState,
  editor,
  mutationUncertain,
  notice,
  onCopy,
  onArchive,
  onClaims,
  onCancelEditor,
  onCreate,
  onDetail,
  onEdit,
  onRefreshAfterUncertain,
  onCopyShare,
  onShare,
  onSubmitDraft,
  role,
  shareState,
  state,
}: {
  readonly busyAction?: "copy" | "archive" | "create" | "update";
  readonly busyID?: number;
  readonly claimsState?: CouponClaimsViewState;
  readonly detailState?: CouponDetailViewState;
  readonly editor?: CouponEditorState;
  readonly mutationUncertain?: boolean;
  readonly notice?: string;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onCopy: (item: CouponListItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onArchive?: (item: CouponListItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onClaims?: (item: CouponListItem, offset?: number) => void;
  readonly onCancelEditor?: () => void;
  readonly onCreate?: () => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onDetail?: (item: CouponListItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onEdit?: (item: CouponListItem) => void;
  readonly onRefreshAfterUncertain?: () => void;
  readonly onCopyShare?: () => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onShare?: (item: CouponListItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onSubmitDraft?: (input: CouponDraftInput) => void;
  readonly role: CouponsRole;
  readonly shareState?: CouponShareViewState;
  readonly state: CouponsViewState;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState<CouponAvailabilityFilter>("all");
  const items = useMemo(
    () =>
      state.kind === "ready" ? filterCoupons(state.items, keyword, status) : [],
    [keyword, state, status],
  );

  if (!canAccess)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">优惠券列表</h1>
        <p role="alert">当前账号没有优惠券管理权限。</p>
      </section>
    );
  if (state.kind === "loading")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">优惠券列表</h1>
        {notice ? <p role="status">{notice}</p> : null}
        <p>正在读取本地优惠券列表。</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">优惠券列表</h1>
        {notice ? <p role="status">{notice}</p> : null}
        <p role="alert">{messages[state.failure]}</p>
      </section>
    );

  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">本地优惠券规则</p>
      <h1 id="app-title">优惠券列表</h1>
      <p>复制只会创建新的本地草稿，不会领取、核销或调用支付及第三方服务。</p>
      {notice ? <p role="status">{notice}</p> : null}
      <p>
        <button
          type="button"
          disabled={busyAction !== undefined || mutationUncertain}
          onClick={onCreate}
        >
          新建本地草稿
        </button>
      </p>
      {mutationUncertain ? (
        <p role="alert">
          本地草稿请求结果不确定，系统不会自动重试。请刷新列表后人工确认。
          <button type="button" onClick={onRefreshAfterUncertain}>
            刷新本地列表
          </button>
        </p>
      ) : null}
      {editor && editor.kind !== "idle" ? (
        <CouponDraftForm
          key={
            editor.kind === "new"
              ? "new"
              : `edit-${editor.detail.id}-${editor.detail.version}`
          }
          busy={busyAction === "create" || busyAction === "update"}
          detail={editor.kind === "edit" ? editor.detail : undefined}
          locked={mutationUncertain === true}
          onCancel={onCancelEditor}
          onSubmit={onSubmitDraft}
        />
      ) : null}
      <p>
        <label>
          搜索优惠券名称
          <input
            type="search"
            value={keyword}
            onChange={(event) => setKeyword(event.currentTarget.value)}
          />
        </label>
      </p>
      <p>
        <label>
          可用状态
          <select
            value={status}
            onChange={(event) =>
              setStatus(event.currentTarget.value as CouponAvailabilityFilter)
            }
          >
            <option value="all">全部</option>
            <option value="draft">draft</option>
            <option value="scheduled">scheduled</option>
            <option value="active">active</option>
            <option value="sold_out">sold_out</option>
            <option value="ended">ended</option>
            <option value="stopped">stopped</option>
            <option value="archived">archived</option>
          </select>
        </label>
      </p>
      {items.length === 0 ? (
        <p>
          {state.items.length === 0
            ? "当前没有本地优惠券。"
            : "没有匹配的优惠券。"}
        </p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>优惠券 ID</th>
              <th>名称</th>
              <th>可用状态</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>{item.id}</td>
                <td>{item.name}</td>
                <td>{item.availability}</td>
                <td>{item.createdAt}</td>
                <td>{item.updatedAt}</td>
                <td>
                  <button
                    type="button"
                    disabled={busyID !== undefined || mutationUncertain}
                    onClick={() => onCopy(item)}
                  >
                    {busyID === item.id && busyAction === "copy"
                      ? "正在复制…"
                      : "复制"}
                  </button>
                  {canArchiveCoupon(item) ? (
                    <button
                      type="button"
                      disabled={busyID !== undefined || mutationUncertain}
                      onClick={() => onArchive?.(item)}
                    >
                      {busyID === item.id && busyAction === "archive"
                        ? "正在归档…"
                        : "归档"}
                    </button>
                  ) : null}
                  <button
                    type="button"
                    disabled={claimsState?.kind === "loading"}
                    onClick={() => onClaims?.(item)}
                  >
                    查看领取数据
                  </button>
                  <button
                    type="button"
                    disabled={
                      mutationUncertain ||
                      (detailState?.kind === "loading" &&
                        detailState.coupon.id === item.id)
                    }
                    onClick={() => onDetail?.(item)}
                  >
                    {detailState?.kind === "loading" &&
                    detailState.coupon.id === item.id
                      ? "正在读取规则…"
                      : "查看规则详情"}
                  </button>
                  {item.status === "draft" ? (
                    <button
                      type="button"
                      disabled={busyAction !== undefined || mutationUncertain}
                      onClick={() => onEdit?.(item)}
                    >
                      编辑本地草稿
                    </button>
                  ) : null}
                  {item.status === "published" ? (
                    <button
                      type="button"
                      disabled={shareState?.kind === "loading"}
                      onClick={() => onShare?.(item)}
                    >
                      分享链接
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {claimsState && claimsState.kind !== "idle" ? (
        <CouponClaimsPanel claimsState={claimsState} onClaims={onClaims} />
      ) : null}
      {detailState && detailState.kind !== "idle" ? (
        <CouponDetailPanel detailState={detailState} />
      ) : null}
      {shareState && shareState.kind !== "idle" ? (
        <CouponSharePanel onCopyShare={onCopyShare} shareState={shareState} />
      ) : null}
    </section>
  );
}

export async function copyCouponShareURL(
  url: string,
  clipboard: Pick<Clipboard, "writeText"> | undefined = runtimeClipboard(),
): Promise<"copied" | "manual"> {
  if (!/^\/c\/c-[1-9][0-9]*$/.test(url)) return "manual";
  try {
    await clipboard?.writeText(url);
    return clipboard ? "copied" : "manual";
  } catch {
    return "manual";
  }
}

function CouponSharePanel({
  onCopyShare,
  shareState,
}: {
  readonly onCopyShare?: () => void;
  readonly shareState: Exclude<CouponShareViewState, { readonly kind: "idle" }>;
}): React.ReactElement {
  const url =
    shareState.kind === "ready" ? shareState.url : shareState.previous;
  const loading = shareState.kind === "loading";
  const error = shareState.kind === "error" ? shareState.failure : undefined;
  const copyStatus =
    shareState.kind === "ready" ? shareState.copyStatus : undefined;
  return (
    <section aria-label="优惠券本地分享链接">
      <h2>本地分享链接：{shareState.coupon.name}</h2>
      <p>仅显示本地相对链接，不代表二维码、领取、核销或外部发送已发生。</p>
      {loading ? <p role="status">正在读取本地分享链接。</p> : null}
      {error ? <p role="alert">{shareMessages[error]}</p> : null}
      {url ? (
        <>
          <p>
            <code>{url}</code>
          </p>
          <button type="button" disabled={loading} onClick={onCopyShare}>
            复制链接
          </button>
          {copyStatus === "copied" ? <p role="status">链接已复制。</p> : null}
          {copyStatus === "manual" ? (
            <p role="status">无法访问剪贴板，请手工复制上方链接。</p>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function draftInput(detail?: CouponRuleDetail): CouponDraftInput {
  return {
    name: detail?.name ?? "",
    discountAmountTotal: detail ? String(detail.discountAmountTotal) : "",
    totalIssueLimit: detail ? String(detail.totalIssueLimit) : "",
    perUserIssueLimit: detail ? String(detail.perUserIssueLimit) : "1",
    claimStartsAt: detail?.claimStartsAt ?? "",
    claimEndsAt: detail?.claimEndsAt ?? "",
    validityMode: detail?.validityMode ?? "relative_days",
    useStartsAt: detail?.useStartsAt ?? "",
    useEndsAt: detail?.useEndsAt ?? "",
    relativeValidityDays: detail?.relativeValidityDays
      ? String(detail.relativeValidityDays)
      : "",
    instructions: detail?.instructions ?? "",
    targetRefs: detail?.targetRefs.join("\n") ?? "",
  };
}

function CouponDraftForm({
  busy,
  detail,
  locked,
  onCancel,
  onSubmit,
}: {
  readonly busy: boolean;
  readonly detail?: CouponRuleDetail;
  readonly locked: boolean;
  readonly onCancel?: () => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onSubmit?: (input: CouponDraftInput) => void;
}): React.ReactElement {
  const [input, setInput] = useState<CouponDraftInput>(() =>
    draftInput(detail),
  );
  const update = <K extends keyof CouponDraftInput>(
    key: K,
    value: CouponDraftInput[K],
  ) => {
    if (locked) return;
    setInput((current) => ({ ...current, [key]: value }));
  };
  return (
    <form
      aria-label={detail ? "编辑本地优惠券草稿" : "新建本地优惠券草稿"}
      onSubmit={(event) => {
        event.preventDefault();
        submitCouponDraftForm(locked, input, onSubmit);
      }}
    >
      <h2>{detail ? `编辑本地草稿：${detail.name}` : "新建本地草稿"}</h2>
      <p>只保存本地草稿；不会发布、停止、领取、支付或调用第三方服务。</p>
      {locked ? (
        <p role="status">
          请求结果不确定，草稿表单已只读；刷新本地列表成功前不能保存。
        </p>
      ) : null}
      <p>
        <label>
          名称
          <input
            required
            maxLength={45}
            disabled={busy || locked}
            value={input.name}
            onChange={(event) => update("name", event.currentTarget.value)}
          />
        </label>
      </p>
      <p>
        <label>
          优惠金额（分）
          <input
            required
            inputMode="numeric"
            disabled={busy || locked}
            value={input.discountAmountTotal}
            onChange={(event) =>
              update("discountAmountTotal", event.currentTarget.value)
            }
          />
        </label>
        <label>
          总发放上限
          <input
            required
            inputMode="numeric"
            disabled={busy || locked}
            value={input.totalIssueLimit}
            onChange={(event) =>
              update("totalIssueLimit", event.currentTarget.value)
            }
          />
        </label>
        <label>
          每人上限
          <input
            required
            inputMode="numeric"
            disabled={busy || locked}
            value={input.perUserIssueLimit}
            onChange={(event) =>
              update("perUserIssueLimit", event.currentTarget.value)
            }
          />
        </label>
      </p>
      <p>
        <label>
          领取开始时间（RFC3339）
          <input
            required
            disabled={busy || locked}
            value={input.claimStartsAt}
            onChange={(event) =>
              update("claimStartsAt", event.currentTarget.value)
            }
          />
        </label>
        <label>
          领取结束时间（RFC3339）
          <input
            required
            disabled={busy || locked}
            value={input.claimEndsAt}
            onChange={(event) =>
              update("claimEndsAt", event.currentTarget.value)
            }
          />
        </label>
      </p>
      <p>
        <label>
          有效期规则
          <select
            disabled={busy || locked}
            value={input.validityMode}
            onChange={(event) =>
              update(
                "validityMode",
                event.currentTarget.value as CouponDraftInput["validityMode"],
              )
            }
          >
            <option value="relative_days">relative_days</option>
            <option value="fixed_range">fixed_range</option>
          </select>
        </label>
      </p>
      {input.validityMode === "fixed_range" ? (
        <p>
          <label>
            使用开始时间（RFC3339）
            <input
              required
              disabled={busy || locked}
              value={input.useStartsAt}
              onChange={(event) =>
                update("useStartsAt", event.currentTarget.value)
              }
            />
          </label>
          <label>
            使用结束时间（RFC3339）
            <input
              required
              disabled={busy || locked}
              value={input.useEndsAt}
              onChange={(event) =>
                update("useEndsAt", event.currentTarget.value)
              }
            />
          </label>
        </p>
      ) : (
        <p>
          <label>
            相对有效天数
            <input
              required
              inputMode="numeric"
              disabled={busy || locked}
              value={input.relativeValidityDays}
              onChange={(event) =>
                update("relativeValidityDays", event.currentTarget.value)
              }
            />
          </label>
        </p>
      )}
      <p>
        <label>
          使用说明
          <textarea
            maxLength={200}
            disabled={busy || locked}
            value={input.instructions}
            onChange={(event) =>
              update("instructions", event.currentTarget.value)
            }
          />
        </label>
      </p>
      <p>
        <label>
          适用商品引用（每行一个 canonical standard_product:ID）
          <textarea
            required
            disabled={busy || locked}
            value={input.targetRefs}
            onChange={(event) =>
              update("targetRefs", event.currentTarget.value)
            }
          />
        </label>
      </p>
      <p>
        <button type="submit" disabled={busy || locked}>
          {busy ? "正在保存本地草稿…" : "保存本地草稿"}
        </button>
        <button type="button" disabled={busy} onClick={onCancel}>
          取消
        </button>
      </p>
    </form>
  );
}

export function submitCouponDraftForm(
  locked: boolean,
  input: CouponDraftInput,
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by the public helper type.
  onSubmit?: (input: CouponDraftInput) => void,
): void {
  if (!locked) onSubmit?.(input);
}

function CouponDetailPanel({
  detailState,
}: {
  readonly detailState: Exclude<
    CouponDetailViewState,
    { readonly kind: "idle" }
  >;
}): React.ReactElement {
  const detail =
    detailState.kind === "ready" ? detailState.detail : detailState.previous;
  const loading = detailState.kind === "loading";
  const error = detailState.kind === "error" ? detailState.failure : undefined;
  return (
    <section aria-label="优惠券本地规则详情">
      <h2>规则详情：{detailState.coupon.name}</h2>
      <p>仅显示已保存的本地规则，不代表可领取、可用或已发生外部效果。</p>
      {loading ? <p role="status">正在读取本地优惠券规则。</p> : null}
      {error ? <p role="alert">{detailMessages[error]}</p> : null}
      {detail ? (
        <dl>
          <dt>优惠金额（分）</dt>
          <dd>
            {detail.discountAmountTotal} {detail.currency}
          </dd>
          <dt>总发放上限</dt>
          <dd>{detail.totalIssueLimit}</dd>
          <dt>每人上限</dt>
          <dd>{detail.perUserIssueLimit}</dd>
          <dt>领取时间</dt>
          <dd>
            {detail.claimStartsAt} 至 {detail.claimEndsAt}
          </dd>
          <dt>有效期规则</dt>
          <dd>
            {detail.validityMode === "fixed_range"
              ? `${detail.useStartsAt} 至 ${detail.useEndsAt}`
              : `${detail.relativeValidityDays} 天`}
          </dd>
          <dt>适用商品引用</dt>
          <dd>{detail.targetRefs.join(", ")}</dd>
          <dt>使用说明</dt>
          <dd>{detail.instructions || "—"}</dd>
        </dl>
      ) : null}
    </section>
  );
}

function CouponClaimsPanel({
  claimsState,
  onClaims,
}: {
  readonly claimsState: Exclude<
    CouponClaimsViewState,
    { readonly kind: "idle" }
  >;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onClaims?: (item: CouponListItem, offset?: number) => void;
}): React.ReactElement {
  const page =
    claimsState.kind === "ready" ? claimsState.page : claimsState.previous;
  const loading = claimsState.kind === "loading";
  const error = claimsState.kind === "error" ? claimsState.failure : undefined;
  const canGoPrevious = Boolean(page && page.offset >= couponClaimsPageSize);
  const canGoNext = Boolean(
    page && page.offset + page.items.length < page.total,
  );
  return (
    <section aria-label="优惠券领取数据">
      <h2>领取数据：{claimsState.coupon.name}</h2>
      {loading ? <p role="status">正在读取本地领取记录。</p> : null}
      {error ? <p role="alert">{messages[error]}</p> : null}
      {page ? (
        <>
          <p>
            共 {page.total} 条，当前第 {page.offset + 1} 至{" "}
            {page.offset + page.items.length} 条。
          </p>
          {page.items.length === 0 ? (
            <p>当前没有本地领取记录。</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>领取记录 ID</th>
                  <th>领取凭据</th>
                  <th>状态</th>
                  <th>领取时间</th>
                </tr>
              </thead>
              <tbody>
                {page.items.map((claim) => (
                  <tr key={claim.id}>
                    <td>{claim.id}</td>
                    <td>{claim.claimRef}</td>
                    <td>claimed</td>
                    <td>{claim.claimedAt}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p>
            <button
              type="button"
              disabled={loading || !canGoPrevious}
              onClick={() =>
                onClaims?.(
                  claimsState.coupon,
                  page.offset - couponClaimsPageSize,
                )
              }
            >
              上一页
            </button>
            <button
              type="button"
              disabled={loading || !canGoNext}
              onClick={() =>
                onClaims?.(
                  claimsState.coupon,
                  page.offset + couponClaimsPageSize,
                )
              }
            >
              下一页
            </button>
          </p>
        </>
      ) : null}
    </section>
  );
}

function runtimeCookieHeader(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

function runtimeConfirm(message: string): boolean {
  try {
    return typeof window !== "undefined" && window.confirm(message);
  } catch {
    return false;
  }
}

function runtimeClipboard(): Pick<Clipboard, "writeText"> | undefined {
  try {
    return typeof navigator === "undefined" ? undefined : navigator.clipboard;
  } catch {
    return undefined;
  }
}
