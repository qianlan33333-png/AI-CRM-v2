import React, { useEffect, useRef, useState } from "react";
import { canReadProducts, generatedProductsTransport, loadProductDetail, loadProducts, type ProductListItem, type ProductPage, type ProductsFailure, type ProductsRole, type ProductsTransport } from "./products";

const messages: Record<ProductsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。", forbidden: "当前账号没有产品目录访问权限。",
  invalid: "产品目录响应不符合已冻结的本地只读合同。", unavailable: "本地产品目录暂不可用，请稍后再查看。",
};
type ViewState = { readonly kind: "loading"; readonly previous?: ProductPage } | { readonly kind: "ready"; readonly page: ProductPage } | { readonly kind: "error"; readonly failure: ProductsFailure; readonly previous?: ProductPage };
type DetailState = { readonly kind: "loading"; readonly id: number; readonly previous?: ProductListItem } | { readonly kind: "ready"; readonly product: ProductListItem } | { readonly kind: "error"; readonly id: number; readonly failure: ProductsFailure; readonly previous?: ProductListItem };

function displayDate(value: string): string { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)); }

export function ProductsPage({ role, transport = generatedProductsTransport, onUnauthenticated }: { readonly role: ProductsRole; readonly transport?: ProductsTransport; readonly onUnauthenticated?: () => void }): React.ReactElement | null {
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [detail, setDetail] = useState<DetailState>();
  const generation = useRef(0); const inFlight = useRef(false); const verified = useRef<ProductPage>(); const cursor = useRef<string>();
  const detailGeneration = useRef(0); const detailInFlight = useRef(new Map<number, symbol>()); const verifiedDetail = useRef<ProductListItem>();
  const invalidateDetail = (clearView: boolean) => { detailGeneration.current++; detailInFlight.current.clear(); verifiedDetail.current = undefined; if (clearView) setDetail(undefined); };
  const closeDetail = () => { invalidateDetail(true); };
  const load = (next?: string) => {
    if (!canReadProducts(role) || inFlight.current) return;
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
    if (!canReadProducts(role) || detailInFlight.current.has(productID)) return;
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
  useEffect(() => { if (!canReadProducts(role)) return undefined; load(); return () => { generation.current++; inFlight.current = false; invalidateDetail(false); }; }, [role, transport]);
  if (!canReadProducts(role)) return null;
  const page = state.kind === "ready" ? state.page : state.previous;
  const detailProduct = detail?.kind === "ready" ? detail.product : detail?.previous;
  return <section aria-label="本地产品目录"><h1>产品目录</h1><p>仅显示已持久化的本地产品投影；不展示图片链接，也不执行支付或任何外部操作。</p>
    {page ? <table><thead><tr><th>ID</th><th>产品码</th><th>名称</th><th>描述</th><th>金额（最小货币单位）</th><th>库存</th><th>创建时间</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{page.items.map((item) => <tr key={item.id}><td>{item.id}</td><td>{item.productCode}</td><td>{item.name}</td><td>{item.description || "—"}</td><td>{item.priceMinor} {item.currency}</td><td>{item.stockQuantity}</td><td>{displayDate(item.createdAt)}</td><td>{displayDate(item.updatedAt)}</td><td><button type="button" disabled={state.kind === "loading" || (detail?.kind === "loading" && detail.id === item.id)} onClick={() => loadDetail(item.id)}>查看详情</button></td></tr>)}</tbody></table> : null}
    {page?.items.length === 0 ? <p role="status">当前没有本地产品。</p> : null}
    {state.kind === "loading" ? <p role="status">正在读取本地产品目录。</p> : null}
    {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
    {detail ? <section aria-label="本地产品详情"><h2>产品详情</h2>{detail.kind === "loading" ? <p role="status">正在读取本地产品详情。</p> : null}{detail.kind === "error" ? <p role="alert">{messages[detail.failure]}</p> : null}{detailProduct ? <dl><dt>ID</dt><dd>{detailProduct.id}</dd><dt>产品码</dt><dd>{detailProduct.productCode}</dd><dt>名称</dt><dd>{detailProduct.name}</dd><dt>描述</dt><dd>{detailProduct.description || "—"}</dd><dt>金额（最小货币单位）</dt><dd>{detailProduct.priceMinor} {detailProduct.currency}</dd><dt>库存</dt><dd>{detailProduct.stockQuantity}</dd><dt>创建时间</dt><dd>{displayDate(detailProduct.createdAt)}</dd><dt>更新时间</dt><dd>{displayDate(detailProduct.updatedAt)}</dd></dl> : null}<button type="button" onClick={closeDetail}>关闭详情</button></section> : null}
    <button type="button" disabled={state.kind === "loading" || page?.nextCursor === undefined} onClick={() => { if (page?.nextCursor) load(page.nextCursor); }}>下一页</button>
  </section>;
}
