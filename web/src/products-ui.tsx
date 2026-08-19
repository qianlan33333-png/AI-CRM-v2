import React, { useEffect, useRef, useState } from "react";
import { canReadProducts, generatedProductsTransport, loadProducts, type ProductPage, type ProductsFailure, type ProductsRole, type ProductsTransport } from "./products";

const messages: Record<ProductsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。", forbidden: "当前账号没有产品目录访问权限。",
  invalid: "产品目录响应不符合已冻结的本地只读合同。", unavailable: "本地产品目录暂不可用，请稍后再查看。",
};
type ViewState = { readonly kind: "loading"; readonly previous?: ProductPage } | { readonly kind: "ready"; readonly page: ProductPage } | { readonly kind: "error"; readonly failure: ProductsFailure; readonly previous?: ProductPage };

function displayDate(value: string): string { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)); }

export function ProductsPage({ role, transport = generatedProductsTransport, onUnauthenticated }: { readonly role: ProductsRole; readonly transport?: ProductsTransport; readonly onUnauthenticated?: () => void }): React.ReactElement | null {
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const generation = useRef(0); const inFlight = useRef(false); const verified = useRef<ProductPage>(); const cursor = useRef<string>();
  const load = (next?: string) => {
    if (!canReadProducts(role) || inFlight.current) return;
    inFlight.current = true; const token = ++generation.current;
    setState({ kind: "loading", previous: verified.current });
    void loadProducts(transport, next).then((result) => {
      if (token !== generation.current) return;
      if (result.status === "loaded") { verified.current = result.page; cursor.current = next; setState({ kind: "ready", page: result.page }); return; }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setState({ kind: "error", failure: result.status, previous: verified.current });
    }).finally(() => { if (token === generation.current) inFlight.current = false; });
  };
  useEffect(() => { if (!canReadProducts(role)) return undefined; load(); return () => { generation.current++; inFlight.current = false; }; }, [role, transport]);
  if (!canReadProducts(role)) return null;
  const page = state.kind === "ready" ? state.page : state.previous;
  return <section aria-label="本地产品目录"><h1>产品目录</h1><p>仅显示已持久化的本地产品投影；不展示图片链接，也不执行支付或任何外部操作。</p>
    {page ? <table><thead><tr><th>ID</th><th>产品码</th><th>名称</th><th>描述</th><th>金额（最小货币单位）</th><th>库存</th><th>创建时间</th><th>更新时间</th></tr></thead><tbody>{page.items.map((item) => <tr key={item.id}><td>{item.id}</td><td>{item.productCode}</td><td>{item.name}</td><td>{item.description || "—"}</td><td>{item.priceMinor} {item.currency}</td><td>{item.stockQuantity}</td><td>{displayDate(item.createdAt)}</td><td>{displayDate(item.updatedAt)}</td></tr>)}</tbody></table> : null}
    {page?.items.length === 0 ? <p role="status">当前没有本地产品。</p> : null}
    {state.kind === "loading" ? <p role="status">正在读取本地产品目录。</p> : null}
    {state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
    <button type="button" disabled={state.kind === "loading" || page?.nextCursor === undefined} onClick={() => { if (page?.nextCursor) load(page.nextCursor); }}>下一页</button>
  </section>;
}
