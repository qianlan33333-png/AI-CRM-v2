/* eslint-disable no-unused-vars -- generic mutation callback names document local transport ownership. */
import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import { crmTagFailure, crmTagHeaders, crmTagIdempotencyKey, loadCRMTagCatalog, type CRMTagCatalog, type CRMTagFailure, type CRMTagRole, type CRMTagTransport } from "./crm-tags";

export interface CRMTagCatalogPageProps { readonly role: CRMTagRole; readonly transport?: CRMTagTransport; readonly readCookie?: () => string; readonly onUnauthenticated?: () => void; }
const message: Record<CRMTagFailure, string> = { unauthenticated: "登录状态已失效，请重新登录。", forbidden: "当前账号没有本地标签目录操作权限。", not_found: "本地标签目录项已不存在。", conflict: "标签目录已变化或仍被客户引用。", invalid: "名称或排序请求不符合本地目录合同。", unknown: "结果尚未确认；为避免重复写入，目录已锁定，请刷新后人工核对。" };
function cookie(): string { return typeof document === "undefined" ? "" : document.cookie; }

export function CRMTagCatalogPage({ role, transport, readCookie = cookie, onUnauthenticated }: CRMTagCatalogPageProps): React.ReactElement {
  const canWrite = role === "admin" || role === "ops";
  const [catalog, setCatalog] = useState<CRMTagCatalog>();
  const [notice, setNotice] = useState<string>();
  const [groupName, setGroupName] = useState("");
  const [tagNames, setTagNames] = useState<Record<number, string>>({});
  const busy = useRef(false); const locked = useRef(false);
  const read = useCallback(async (): Promise<CRMTagCatalog | undefined> => {
    const result = await loadCRMTagCatalog(transport ?? {});
    if (result.status === "loaded") { setCatalog(result.catalog); return result.catalog; }
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setNotice(message[result.status]); return undefined;
  }, [onUnauthenticated, transport]);
  useEffect(() => { if (canWrite) void read(); }, [canWrite, read]);
  const mutate = async (call: (options: RequestInit) => Promise<{ readonly status: number }>, verify: (next: CRMTagCatalog) => boolean) => {
    if (!canWrite || busy.current || locked.current) return;
    let csrf: string | undefined; try { csrf = readCSRFCookie(readCookie()); } catch { csrf = undefined; }
    const key = crmTagIdempotencyKey();
    if (!csrf || !key) { setNotice(!csrf ? "安全令牌缺失，未发送目录写请求。" : "安全随机源不可用，未发送目录写请求。"); return; }
    busy.current = true; setNotice(undefined);
    try {
      const result = await call(crmTagHeaders(csrf, key));
      const failure = crmTagFailure(result.status);
      if (result.status < 200 || result.status >= 300) { if (failure === "unknown") locked.current = true; setNotice(message[failure]); if (failure === "unauthenticated") onUnauthenticated?.(); return; }
      const reread = await read();
      if (!reread || !verify(reread)) { locked.current = true; setNotice(message.unknown); return; }
      setNotice("本地目录操作已由列表回读确认。");
    } catch { locked.current = true; setNotice(message.unknown); } finally { busy.current = false; }
  };
  if (!canWrite) return <section className="crm-tags-page"><h2>CRM 本地标签目录</h2><p role="alert">当前账号没有本地标签目录访问权限。</p></section>;
  if (!transport?.list) return <section className="crm-tags-page"><h2>CRM 本地标签目录</h2><p role="status">本地目录客户端尚未接入；未发送任何请求。</p></section>;
  return <section className="crm-tags-page" aria-labelledby="crm-tags-title"><h2 id="crm-tags-title">CRM 本地标签目录</h2><p>仅管理 CRM 本地分类；不触发企微同步、在线目录或 Provider 调用。</p>{notice && <p role="alert">{notice}</p>}<button type="button" onClick={() => void read()}>刷新目录</button>{transport.createGroup && <form onSubmit={(event) => { event.preventDefault(); const name = groupName.trim(); if (!name) { setNotice("标签组名称不能为空。"); return; } void mutate((options) => transport.createGroup!({ name }, options), (next) => next.groups.some((group) => group.name === name)).then(() => setGroupName("")); }}><label>新建标签组<input value={groupName} onChange={(event) => setGroupName(event.currentTarget.value)} /></label><button type="submit" disabled={locked.current}>新建</button></form>}{catalog && <ol>{catalog.groups.map((group, index) => <li key={group.id}><strong>{group.name}</strong>{transport.updateGroup && <button type="button" disabled={locked.current} onClick={() => { const name = window.prompt("本地标签组名称", group.name); if (name?.trim()) void mutate((options) => transport.updateGroup!(group.id, { name: name.trim() }, options), (next) => next.groups.some((item) => item.id === group.id && item.name === name.trim())); }}>改名</button>}{transport.reorderGroups && <><button type="button" disabled={locked.current || index === 0} onClick={() => { const ids = catalog.groups.map((item) => item.id); [ids[index - 1], ids[index]] = [ids[index], ids[index - 1]]; void mutate((options) => transport.reorderGroups!({ ids }, options), (next) => next.groups.map((item) => item.id).join(",") === ids.join(",")); }}>上移</button><button type="button" disabled={locked.current || index === catalog.groups.length - 1} onClick={() => { const ids = catalog.groups.map((item) => item.id); [ids[index + 1], ids[index]] = [ids[index], ids[index + 1]]; void mutate((options) => transport.reorderGroups!({ ids }, options), (next) => next.groups.map((item) => item.id).join(",") === ids.join(",")); }}>下移</button></>}{transport.archiveGroup && <button type="button" disabled={locked.current} onClick={() => { if (window.confirm(`确认归档本地标签组“${group.name}”？仍被客户引用时会被拒绝。`)) void mutate((options) => transport.archiveGroup!(group.id, options), (next) => !next.groups.some((item) => item.id === group.id)); }}>归档</button>}<ul>{catalog.tags.filter((tag) => tag.groupID === group.id).map((tag) => <li key={tag.id}>{tag.name}{transport.updateTag && <button type="button" onClick={() => { const name = window.prompt("本地标签名称", tag.name); if (name?.trim()) void mutate((options) => transport.updateTag!(tag.id, { name: name.trim() }, options), (next) => next.tags.some((item) => item.id === tag.id && item.name === name.trim())); }}>改名</button>}{transport.archiveTag && <button type="button" onClick={() => { if (window.confirm(`确认归档本地标签“${tag.name}”？仍被客户引用时会被拒绝。`)) void mutate((options) => transport.archiveTag!(tag.id, options), (next) => !next.tags.some((item) => item.id === tag.id)); }}>归档</button>}</li>)}</ul>{transport.createTag && <form onSubmit={(event) => { event.preventDefault(); const name = (tagNames[group.id] ?? "").trim(); if (name) void mutate((options) => transport.createTag!({ group_id: group.id, name }, options), (next) => next.tags.some((tag) => tag.groupID === group.id && tag.name === name)); }}><input aria-label={`${group.name} 新标签`} value={tagNames[group.id] ?? ""} onChange={(event) => setTagNames((current) => ({ ...current, [group.id]: event.currentTarget.value }))} /><button type="submit">添加标签</button></form>}</li>)}</ol>}</section>;
}
