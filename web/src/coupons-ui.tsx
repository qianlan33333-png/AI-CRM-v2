import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { readCSRFCookie } from "./auth";
import {
  copyCoupon,
  couponClaimsPageSize,
  filterCoupons,
  generatedCouponsTransport,
  loadCouponClaims,
  loadCoupons,
  newCouponCopyIdempotencyKey,
  type CouponAvailabilityFilter,
  type CouponClaimItem,
  type CouponClaimsResult,
  type CouponCopyResult,
  type CouponListItem,
  type CouponListResult,
  type CouponsFailure,
  type CouponsRole,
  type CouponsTransport,
} from "./coupons";

const messages: Record<CouponsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有优惠券管理权限。",
  not_found: "要复制的优惠券已不存在，请刷新后重试。",
  conflict: "复制请求与已有操作冲突，请刷新后重试。",
  invalid: "优惠券响应或请求不符合已冻结合同。",
  unavailable: "本地优惠券服务暂不可用，请稍后重试。",
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

export interface CouponCopyInput {
  readonly couponID: number;
  readonly idempotencySource?: { readonly randomUUID: () => string };
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

export function CouponsPage({
  role,
  transport = generatedCouponsTransport,
  readCookie = runtimeCookieHeader,
  onUnauthenticated,
}: {
  readonly role: CouponsRole;
  readonly transport?: CouponsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<CouponsViewState>({ kind: "loading" });
  const [busyID, setBusyID] = useState<number>();
  const [notice, setNotice] = useState<string>();
  const [claimsState, setClaimsState] = useState<CouponClaimsViewState>({
    kind: "idle",
  });
  const copyInFlight = useRef(false);
  const claimsRequest = useRef(0);
  const claimsInFlight = useRef<string>();

  const reload = useCallback(async (): Promise<CouponListResult> => {
    const result = await loadCoupons(transport);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setState(
      result.status === "loaded"
        ? { kind: "ready", items: result.items }
        : { kind: "error", failure: result.status },
    );
    return result;
  }, [onUnauthenticated, transport]);

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
      const operation = startCouponCopy(copyInFlight, async () => {
        setBusyID(item.id);
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
          setBusyID(undefined);
        }
      });
      if (operation) await operation;
    },
    [onUnauthenticated, readCookie, reload, transport],
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

  return (
    <CouponsView
      busyID={busyID}
      claimsState={claimsState}
      notice={notice}
      onCopy={onCopy}
      onClaims={onClaims}
      role={role}
      state={state}
    />
  );
}

export function CouponsView({
  busyID,
  claimsState,
  notice,
  onCopy,
  onClaims,
  role,
  state,
}: {
  readonly busyID?: number;
  readonly claimsState?: CouponClaimsViewState;
  readonly notice?: string;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onCopy: (item: CouponListItem) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
  readonly onClaims?: (item: CouponListItem, offset?: number) => void;
  readonly role: CouponsRole;
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
                    disabled={busyID !== undefined}
                    onClick={() => onCopy(item)}
                  >
                    {busyID === item.id ? "正在复制…" : "复制"}
                  </button>
                  <button
                    type="button"
                    disabled={claimsState?.kind === "loading"}
                    onClick={() => onClaims?.(item)}
                  >
                    查看领取数据
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {claimsState && claimsState.kind !== "idle" ? (
        <CouponClaimsPanel claimsState={claimsState} onClaims={onClaims} />
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
