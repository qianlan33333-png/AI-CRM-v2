import React, { useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  canReadProducts,
  createLocalProduct,
  defaultProductDraft,
  generatedProductsTransport,
  loadProductDetail,
  loadProducts,
  newProductIdempotencyKey,
  productDraftProblem,
  type ProductCreateResult,
  type ProductDraft,
  type ProductListItem,
  type ProductPage,
  type ProductsFailure,
  type ProductsRole,
  type ProductsTransport,
} from "./products";

const messages: Record<ProductsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有产品目录访问权限。",
  invalid: "产品目录响应不符合已冻结的本地合同。",
  unavailable: "本地产品目录暂不可用，请稍后再查看。",
};
const createMessages: Record<Exclude<ProductCreateResult["status"], "created">, string> = {
  unauthenticated: messages.unauthenticated,
  forbidden: messages.forbidden,
  conflict: "本地产品创建请求已冲突，请检查产品码后重新提交。",
  invalid: "本地产品创建数据或回执不符合已冻结合同。",
  unknown: "本地产品创建结果未知，已锁定本页再次创建；请刷新后核对本地目录。",
};
type ViewState = { readonly kind: "loading"; readonly previous?: ProductPage } | { readonly kind: "ready"; readonly page: ProductPage } | { readonly kind: "error"; readonly failure: ProductsFailure; readonly previous?: ProductPage };
type DetailState = { readonly kind: "loading"; readonly id: number; readonly previous?: ProductListItem } | { readonly kind: "ready"; readonly product: ProductListItem } | { readonly kind: "error"; readonly id: number; readonly failure: ProductsFailure; readonly previous?: ProductListItem };
type CreateState = { readonly kind: "idle" } | { readonly kind: "saving" } | { readonly kind: "error"; readonly message: string } | { readonly kind: "unknown"; readonly message: string } | { readonly kind: "created"; readonly product: ProductListItem };

function displayDate(value: string): string { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)); }
function runtimeCookieHeader(): string { return typeof document === "undefined" ? "" : document.cookie; }

export function ProductsPage({
  role,
  transport = generatedProductsTransport,
  readCookie = runtimeCookieHeader,
  onUnauthenticated,
}: {
  readonly role: ProductsRole;
  readonly transport?: ProductsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement | null {
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [detail, setDetail] = useState<DetailState>();
  const [draft, setDraft] = useState<ProductDraft>(defaultProductDraft);
  const [createState, setCreateState] = useState<CreateState>({ kind: "idle" });
  const generation = useRef(0); const inFlight = useRef(false); const verified = useRef<ProductPage>(); const cursor = useRef<string>();
  const detailGeneration = useRef(0); const detailInFlight = useRef(new Map<number, symbol>()); const verifiedDetail = useRef<ProductListItem>();
  const createGeneration = useRef(0); const createInFlight = useRef<symbol>(); const createOutcomeUnknown = useRef(false);
  const canAccess = canReadProducts(role);
  const invalidateDetail = (clearView: boolean) => { detailGeneration.current++; detailInFlight.current.clear(); verifiedDetail.current = undefined; if (clearView) setDetail(undefined); };
  const closeDetail = () => { invalidateDetail(true); };
  const load = (next?: string) => {
    if (!canAccess || inFlight.current) return;
    closeDetail();
    inFlight.current = true; const token = ++generation.current;
    setState({ kind: "loading", previous: verified.current });
    void loadProducts(transport, next).then((result) => {
      if (token !== generation.current) return;
      if (result.status === "loaded") { verified.current = result.page; cursor.current = next; setState({ kind: "ready", page: result.page }); return; }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setState({ kind: "error", failure: result.status, previous: verified.current });
    }).finally(() => { if (token === generation.current) inFlight.current = false; });
  };
  const loadDetail = (productID: number) => {
    if (!canAccess || createInFlight.current || detailInFlight.current.has(productID)) return;
    const token = Symbol(); const detailToken = ++detailGeneration.current;
    detailInFlight.current.clear();
    detailInFlight.current.set(productID, token);
    const previous = verifiedDetail.current?.id === productID ? verifiedDetail.current : undefined;
    setDetail({ kind: "loading", id: productID, previous });
    void loadProductDetail(transport, productID).then((result) => {
      if (detailToken !== detailGeneration.current || detailInFlight.current.get(productID) !== token) return;
      if (result.status === "loaded") { verifiedDetail.current = result.product; setDetail({ kind: "ready", product: result.product }); return; }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setDetail({ kind: "error", id: productID, failure: result.status, previous });
    }).finally(() => { if (detailInFlight.current.get(productID) === token) detailInFlight.current.delete(productID); });
  };
  const submitCreate = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canAccess || createInFlight.current || createOutcomeUnknown.current) return;
    const problem = productDraftProblem(draft);
    if (problem) { setCreateState({ kind: "error", message: problem }); return; }
    let csrfToken: string | undefined;
    try { csrfToken = readCSRFCookie(readCookie()); } catch { csrfToken = undefined; }
    const idempotencyKey = newProductIdempotencyKey();
    if (!csrfToken || !idempotencyKey) { setCreateState({ kind: "error", message: createMessages.forbidden }); return; }
    const token = Symbol(); const tokenGeneration = ++createGeneration.current;
    createInFlight.current = token;
    setCreateState({ kind: "saving" });
    void createLocalProduct(transport, draft, csrfToken, idempotencyKey).then((result) => {
      if (createGeneration.current !== tokenGeneration || createInFlight.current !== token) return;
      if (result.status === "created") {
        setDraft(defaultProductDraft); setCreateState({ kind: "created", product: result.product });
        cursor.current = undefined; load();
        return;
      }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "unknown") {
        createOutcomeUnknown.current = true;
        setCreateState({ kind: "unknown", message: createMessages.unknown });
        return;
      }
      setCreateState({ kind: "error", message: createMessages[result.status] });
    }).finally(() => { if (createInFlight.current === token) createInFlight.current = undefined; });
  };
  useEffect(() => {
    if (!canAccess) return undefined;
    load();
    return () => { generation.current++; inFlight.current = false; invalidateDetail(false); createGeneration.current++; createInFlight.current = undefined; createOutcomeUnknown.current = false; };
  }, [canAccess, transport]);
  if (!canAccess) return null;
  const page = state.kind === "ready" ? state.page : state.previous;
  const detailProduct = detail?.kind === "ready" ? detail.product : detail?.previous;
  const busy = state.kind === "loading" || detail?.kind === "loading" || createState.kind === "saving";
  return <section aria-label="本地产品目录"><h1>产品目录</h1><p>仅显示或创建已持久化的本地产品投影；不展示图片链接，也不执行支付、Provider 或任何外部操作。</p>
    <section aria-label="新增本地产品"><h2>新增本地产品</h2><p>仅创建本地产品目录记录；图片引用固定为空且不在此处理。</p><form onSubmit={submitCreate}><fieldset disabled={busy || createOutcomeUnknown.current}><label>产品码<input aria-label="产品码" value={draft.productCode} onChange={(event) => setDraft({ ...draft, productCode: event.currentTarget.value })} /></label><label>名称<input aria-label="名称" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })} /></label><label>描述<textarea aria-label="描述" value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.currentTarget.value })} /></label><label>金额（最小货币单位）<input aria-label="金额（最小货币单位）" inputMode="numeric" value={draft.priceMinor} onChange={(event) => setDraft({ ...draft, priceMinor: event.currentTarget.value })} /></label><label>货币<input aria-label="货币" value={draft.currency} onChange={(event) => setDraft({ ...draft, currency: event.currentTarget.value })} /></label><label>库存<input aria-label="库存" inputMode="numeric" value={draft.stockQuantity} onChange={(event) => setDraft({ ...draft, stockQuantity: event.currentTarget.value })} /></label><button type="submit">创建本地产品</button></fieldset></form>{createState.kind === "saving" ? <p role="status">正在创建本地产品。</p> : null}{createState.kind === "error" || createState.kind === "unknown" ? <p role="alert">{createState.message}</p> : null}{createState.kind === "created" ? <p role="status">已创建本地产品：{createState.product.productCode}。</p> : null}</section>
    {page ? <table><thead><tr><th>ID</th><th>产品码</th><th>名称</th><th>描述</th><th>金额（最小货币单位）</th><th>库存</th><th>创建时间</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{page.items.map((item) => <tr key={item.id}><td>{item.id}</td><td>{item.productCode}</td><td>{item.name}</td><td>{item.description || "—"}</td><td>{item.priceMinor} {item.currency}</td><td>{item.stockQuantity}</td><td>{displayDate(item.createdAt)}</td><td>{displayDate(item.updatedAt)}</td><td><button type="button" disabled={busy} onClick={() => loadDetail(item.id)}>查看详情</button></td></tr>)}</tbody></table> : null}
    {page?.items.length === 0 ? <p role="status">当前没有本地产品。</p> : null}
    {state.kind === "loading" ? <p role="status">正在读取本地产品目录。</p> : null}
    {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
    {detail ? <section aria-label="本地产品详情"><h2>产品详情</h2>{detail.kind === "loading" ? <p role="status">正在读取本地产品详情。</p> : null}{detail.kind === "error" ? <p role="alert">{messages[detail.failure]}</p> : null}{detailProduct ? <dl><dt>ID</dt><dd>{detailProduct.id}</dd><dt>产品码</dt><dd>{detailProduct.productCode}</dd><dt>名称</dt><dd>{detailProduct.name}</dd><dt>描述</dt><dd>{detailProduct.description || "—"}</dd><dt>金额（最小货币单位）</dt><dd>{detailProduct.priceMinor} {detailProduct.currency}</dd><dt>库存</dt><dd>{detailProduct.stockQuantity}</dd><dt>创建时间</dt><dd>{displayDate(detailProduct.createdAt)}</dd><dt>更新时间</dt><dd>{displayDate(detailProduct.updatedAt)}</dd></dl> : null}<button type="button" disabled={busy} onClick={closeDetail}>关闭详情</button></section> : null}
    <button type="button" disabled={busy || page?.nextCursor === undefined} onClick={() => { if (page?.nextCursor) load(page.nextCursor); }}>下一页</button>
  </section>;
}
