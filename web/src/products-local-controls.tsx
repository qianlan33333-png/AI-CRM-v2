import React, { useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
	defaultProductUpdateDraft,
	grantProductLocalEntitlement,
	loadProductDetail,
	loadProductLocalEntitlement,
	loadProductLocalEntitlements,
	newLocalProductIdempotencyKey,
	productUpdateRequest,
	revokeProductLocalEntitlement,
	updateLocalProduct,
	type LocalEntitlement,
	type ProductListItem,
	type ProductUpdateDraft,
	type ProductsTransport,
} from "./products";

type MutationState = { readonly kind: "idle" } | { readonly kind: "saving" } | { readonly kind: "error"; readonly message: string } | { readonly kind: "unknown"; readonly message: string } | { readonly kind: "done"; readonly message: string };
type EntitlementState = { readonly kind: "loading"; readonly previous?: readonly LocalEntitlement[] } | { readonly kind: "ready"; readonly items: readonly LocalEntitlement[] } | { readonly kind: "error"; readonly message: string; readonly previous?: readonly LocalEntitlement[] };

function runtimeCookieHeader(): string { return typeof document === "undefined" ? "" : document.cookie; }
function mutationMessage(status: string): string {
	switch (status) {
		case "unauthenticated": return "登录状态已失效，请重新登录。";
		case "forbidden": return "当前账号没有本地权益操作权限。";
		case "conflict": return "本地状态已变化或订单已存在权益，请刷新后核对。";
		case "invalid": return "请求不符合已冻结的本地合同。";
		default: return "结果未知，已锁定该动作；请刷新后核对已持久化状态。";
	}
}
function positiveID(value: string): number | undefined { return /^(?:[1-9]\d*)$/.test(value) && Number.isSafeInteger(Number(value)) ? Number(value) : undefined; }
function sameProduct(left: ProductListItem, right: ProductListItem): boolean {
	return left.id === right.id && left.version === right.version && left.productCode === right.productCode && left.name === right.name && left.description === right.description && left.priceMinor === right.priceMinor && left.currency === right.currency && left.stockQuantity === right.stockQuantity && left.createdBy === right.createdBy && left.createdAt === right.createdAt && left.updatedAt === right.updatedAt && left.images.length === right.images.length && left.images.every((image, index) => image === right.images[index]);
}
function sameEntitlement(left: LocalEntitlement, right: LocalEntitlement): boolean {
	return left.id === right.id && left.productId === right.productId && left.orderId === right.orderId && left.customerId === right.customerId && left.state === right.state && left.version === right.version && left.grantedAt === right.grantedAt && left.revokedAt === right.revokedAt;
}
function mutationNotice(state: MutationState): React.ReactElement | null {
	if (state.kind === "idle") return null;
	if (state.kind === "saving") return <p role="status">正在提交本地操作。</p>;
	return <p role={state.kind === "error" || state.kind === "unknown" ? "alert" : "status"}>{state.message}</p>;
}

export function ProductLocalControls({ product, transport, readCookie = runtimeCookieHeader, onUnauthenticated, onProductUpdated }: {
	readonly product: ProductListItem;
	readonly transport: ProductsTransport;
	readonly readCookie?: () => string;
	readonly onUnauthenticated?: () => void;
	// eslint-disable-next-line no-unused-vars -- callback parameter documents the updated local product.
	readonly onProductUpdated: (product: ProductListItem) => void;
}): React.ReactElement {
	const [updateDraft, setUpdateDraft] = useState<ProductUpdateDraft | undefined>(() => defaultProductUpdateDraft(product));
	const [updateState, setUpdateState] = useState<MutationState>({ kind: "idle" });
	const [grantOrderID, setGrantOrderID] = useState(""); const [grantState, setGrantState] = useState<MutationState>({ kind: "idle" });
	const [entitlements, setEntitlements] = useState<EntitlementState>({ kind: "loading" }); const [selected, setSelected] = useState<LocalEntitlement>(); const [revokeCandidate, setRevokeCandidate] = useState<LocalEntitlement>(); const [revokeState, setRevokeState] = useState<MutationState>({ kind: "idle" });
	const updateUnknown = useRef(false); const grantUnknown = useRef(false); const revokeUnknown = useRef(new Set<number>()); const generation = useRef(0); const detailGeneration = useRef(0);
	const updateInFlight = useRef<symbol>(); const grantInFlight = useRef<symbol>(); const revokeInFlight = useRef(new Map<number, symbol>());
	const csrf = (): string | undefined => { try { return readCSRFCookie(readCookie()); } catch { return undefined; } };
	const load = () => {
		if (transport.listEntitlements === undefined) { setEntitlements({ kind: "error", message: "本地权益传输尚未接入。" }); return; }
		const token = ++generation.current; const previous = entitlements.kind === "ready" ? entitlements.items : entitlements.kind === "error" ? entitlements.previous : undefined;
		setEntitlements({ kind: "loading", previous });
		void loadProductLocalEntitlements(transport, product.id).then((result) => {
			if (token !== generation.current) return;
			if (result.status === "loaded") { setEntitlements({ kind: "ready", items: result.items }); return; }
			if (result.status === "unauthenticated") onUnauthenticated?.();
			setEntitlements({ kind: "error", message: mutationMessage(result.status), previous });
		});
	};
	useEffect(() => { setUpdateDraft(defaultProductUpdateDraft(product)); updateUnknown.current = false; setUpdateState({ kind: "idle" }); load(); return () => { generation.current++; detailGeneration.current++; updateInFlight.current = undefined; grantInFlight.current = undefined; revokeInFlight.current.clear(); }; }, [product.id, product.version, transport]);
	const submitUpdate = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault(); if (updateDraft === undefined || updateInFlight.current !== undefined || updateUnknown.current) return;
		if (productUpdateRequest(updateDraft) === undefined) { setUpdateState({ kind: "error", message: "请填写合法的版本、名称、描述、金额、货币与库存。" }); return; }
		const token = csrf(); const key = newLocalProductIdempotencyKey("update"); if (!token || !key) { setUpdateState({ kind: "error", message: mutationMessage("forbidden") }); return; }
		const requestToken = Symbol(); updateInFlight.current = requestToken; setUpdateState({ kind: "saving" });
		void updateLocalProduct(transport, product, updateDraft, token, key).then(async (result) => {
			if (updateInFlight.current !== requestToken) return;
			if (result.status === "updated") { const readback = await loadProductDetail(transport, product.id); if (updateInFlight.current !== requestToken) return; if (readback.status === "loaded" && sameProduct(readback.product, result.product)) { onProductUpdated(readback.product); setUpdateState({ kind: "done", message: "本地产品已按版本条件更新。" }); return; } updateUnknown.current = true; setUpdateState({ kind: "unknown", message: mutationMessage("unknown") }); return; }
			if (result.status === "unauthenticated") onUnauthenticated?.();
			if (result.status === "unknown") { updateUnknown.current = true; setUpdateState({ kind: "unknown", message: mutationMessage(result.status) }); return; }
			setUpdateState({ kind: "error", message: mutationMessage(result.status) });
		}).finally(() => { if (updateInFlight.current === requestToken) updateInFlight.current = undefined; });
	};
	const submitGrant = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault(); if (grantInFlight.current !== undefined || grantUnknown.current) return;
		const orderID = positiveID(grantOrderID); const token = csrf(); const key = newLocalProductIdempotencyKey("grant"); if (!orderID || !token || !key) { setGrantState({ kind: "error", message: "请填写合法订单 ID 并保持登录状态。" }); return; }
		const requestToken = Symbol(); grantInFlight.current = requestToken; setGrantState({ kind: "saving" });
		void grantProductLocalEntitlement(transport, product.id, orderID, token, key).then(async (result) => {
			if (grantInFlight.current !== requestToken) return;
			if (result.status === "granted") { const readback = await loadProductLocalEntitlements(transport, product.id); if (grantInFlight.current !== requestToken) return; if (readback.status === "loaded" && readback.items.some((item) => sameEntitlement(item, result.entitlement))) { setGrantOrderID(""); setGrantState({ kind: "done", message: "本地权益已授予。" }); load(); return; } grantUnknown.current = true; setGrantState({ kind: "unknown", message: mutationMessage("unknown") }); return; }
			if (result.status === "unauthenticated") onUnauthenticated?.();
			if (result.status === "unknown") { grantUnknown.current = true; setGrantState({ kind: "unknown", message: mutationMessage(result.status) }); return; }
			setGrantState({ kind: "error", message: mutationMessage(result.status) });
		}).finally(() => { if (grantInFlight.current === requestToken) grantInFlight.current = undefined; });
	};
	const showDetail = (id: number) => { const token = ++detailGeneration.current; void loadProductLocalEntitlement(transport, id).then((result) => { if (token !== detailGeneration.current) return; if (result.status === "loaded") setSelected(result.entitlement); else { if (result.status === "unauthenticated") onUnauthenticated?.(); setSelected(undefined); } }); };
	const revoke = (entitlement: LocalEntitlement) => {
		if (revokeInFlight.current.has(entitlement.id) || revokeUnknown.current.has(entitlement.id)) return;
		const token = csrf(); const key = newLocalProductIdempotencyKey("revoke"); if (!token || !key) { setRevokeState({ kind: "error", message: mutationMessage("forbidden") }); return; }
		const requestToken = Symbol(); revokeInFlight.current.set(entitlement.id, requestToken); setRevokeState({ kind: "saving" });
		void revokeProductLocalEntitlement(transport, entitlement, token, key).then(async (result) => {
			if (revokeInFlight.current.get(entitlement.id) !== requestToken) return;
			if (result.status === "revoked") { const readback = await loadProductLocalEntitlement(transport, entitlement.id); if (revokeInFlight.current.get(entitlement.id) !== requestToken) return; if (readback.status === "loaded" && sameEntitlement(readback.entitlement, result.entitlement)) { setRevokeCandidate(undefined); setRevokeState({ kind: "done", message: "本地权益已撤销。" }); setSelected(readback.entitlement); load(); return; } revokeUnknown.current.add(entitlement.id); setRevokeState({ kind: "unknown", message: mutationMessage("unknown") }); return; }
			if (result.status === "unauthenticated") onUnauthenticated?.();
			if (result.status === "unknown") { revokeUnknown.current.add(entitlement.id); setRevokeState({ kind: "unknown", message: mutationMessage(result.status) }); return; }
			setRevokeState({ kind: "error", message: mutationMessage(result.status) });
		}).finally(() => { if (revokeInFlight.current.get(entitlement.id) === requestToken) revokeInFlight.current.delete(entitlement.id); });
	};
	const entitlementItems = entitlements.kind === "ready" ? entitlements.items : entitlements.kind === "loading" || entitlements.kind === "error" ? entitlements.previous : undefined;
	return <section aria-label="本地产品更新与权益"><h3>本地产品更新</h3>{updateDraft === undefined ? <p role="status">产品版本传输尚未接入，不能执行条件更新。</p> : <form onSubmit={submitUpdate}><fieldset disabled={updateState.kind === "saving" || updateUnknown.current}><label>名称<input aria-label="更新名称" value={updateDraft.name} onChange={(event) => setUpdateDraft({ ...updateDraft, name: event.currentTarget.value })} /></label><label>描述<textarea aria-label="更新描述" value={updateDraft.description} onChange={(event) => setUpdateDraft({ ...updateDraft, description: event.currentTarget.value })} /></label><label>金额<input aria-label="更新金额" value={updateDraft.priceMinor} onChange={(event) => setUpdateDraft({ ...updateDraft, priceMinor: event.currentTarget.value })} /></label><label>货币<input aria-label="更新货币" value={updateDraft.currency} onChange={(event) => setUpdateDraft({ ...updateDraft, currency: event.currentTarget.value })} /></label><label>库存<input aria-label="更新库存" value={updateDraft.stockQuantity} onChange={(event) => setUpdateDraft({ ...updateDraft, stockQuantity: event.currentTarget.value })} /></label><button type="submit">按版本更新</button></fieldset></form>}{mutationNotice(updateState)}
		<h3>本地权益</h3><p>仅根据已存在的本地 paid 订单投影授予，不会调用支付、退款、Provider 或外部系统。</p><form onSubmit={submitGrant}><fieldset disabled={grantState.kind === "saving" || grantUnknown.current}><label>已支付订单 ID<input aria-label="已支付订单 ID" value={grantOrderID} onChange={(event) => setGrantOrderID(event.currentTarget.value)} /></label><button type="submit">授予本地权益</button></fieldset></form>{mutationNotice(grantState)}
		{entitlements.kind === "loading" ? <p role="status">正在读取本地权益。</p> : null}{entitlements.kind === "error" ? <p role="alert">{entitlements.message}</p> : null}{entitlementItems?.map((item) => <article key={item.id}><p>权益 #{item.id}：{item.state}，订单 #{item.orderId}，版本 {item.version}</p><button type="button" onClick={() => showDetail(item.id)}>查看权益详情</button>{item.state === "active" ? <button type="button" disabled={revokeInFlight.current.has(item.id) || revokeUnknown.current.has(item.id)} onClick={() => setRevokeCandidate(item)}>撤销本地权益</button> : null}</article>)}
		{revokeCandidate ? <p role="alert">确认撤销权益 #{revokeCandidate.id}？此操作只改变本地权益状态。<button type="button" onClick={() => revoke(revokeCandidate)}>确认撤销</button><button type="button" onClick={() => setRevokeCandidate(undefined)}>取消</button></p> : null}
		{selected ? <dl aria-label="本地权益详情"><dt>ID</dt><dd>{selected.id}</dd><dt>状态</dt><dd>{selected.state}</dd><dt>订单 ID</dt><dd>{selected.orderId}</dd><dt>版本</dt><dd>{selected.version}</dd></dl> : null}{mutationNotice(revokeState)}</section>;
}
