/* eslint-disable no-unused-vars -- callback tuple signatures document state-machine seams. */
import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  archiveAutomationAgent,
  copyAutomationAgent,
  createAutomationAgent,
  defaultAutomationAgentDraft,
  filterAutomationAgents,
  generatedAutomationAgentsTransport,
  loadAutomationAgentDetail,
  loadAutomationAgents,
  newAutomationAgentIdempotencyKey,
  publishAutomationAgent,
  saveAutomationAgentFixedContent,
  setAutomationAgentStatus,
  updateAutomationAgent,
  type AutomationAgentDetail,
  type AutomationAgentDraft,
  type AutomationAgentMutationResult,
  type AutomationAgentsFailure,
  type AutomationAgentsRole,
  type AutomationAgentsSnapshot,
  type AutomationAgentsTransport,
  type AutomationAgentStatus,
  type AutomationAgentType,
} from "./automation-agents";

const messages: Record<AutomationAgentsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有自动化话术目录访问权限。",
  invalid: "自动化话术目录响应不符合已冻结的本地合同。",
  unavailable: "自动化话术目录暂时不可用，请稍后手动刷新。",
};
const writeMessages: Record<Exclude<AutomationAgentMutationResult["status"], "succeeded" | "archived">, string> = {
  unauthenticated: messages.unauthenticated,
  forbidden: "当前账号没有本地自动化话术管理权限。",
  invalid: "本地配置命令或回执不符合已冻结合同。",
  not_found: "该本地自动化话术已不存在。",
  conflict: "本地自动化话术配置冲突，请刷新后核对。",
  unknown: "本地配置命令结果未知，已锁定本页后续写入；请刷新后核对本地目录。",
};
export type AutomationAgentsState =
  | { readonly kind: "loading"; readonly previous?: AutomationAgentsSnapshot }
  | { readonly kind: "ready"; readonly snapshot: AutomationAgentsSnapshot }
  | { readonly kind: "error"; readonly failure: AutomationAgentsFailure; readonly previous?: AutomationAgentsSnapshot };
export interface AutomationAgentsReadController {
  readonly generation: { current: number };
  readonly inFlight: { current: symbol | undefined };
  readonly verified: { current: AutomationAgentsSnapshot | undefined };
  readonly onState: (...args: [AutomationAgentsState]) => void;
  readonly onUnauthenticated?: () => void;
}
export function invalidateAutomationAgentsRead(controller: AutomationAgentsReadController): void {
  controller.generation.current += 1; controller.inFlight.current = undefined;
}
export async function startAutomationAgentsRead(controller: AutomationAgentsReadController, transport: AutomationAgentsTransport): Promise<void> {
  if (controller.inFlight.current) return;
  const token = Symbol("automation-agents-read"); controller.inFlight.current = token;
  const generation = ++controller.generation.current;
  controller.onState({ kind: "loading", previous: controller.verified.current });
  try {
    const result = await loadAutomationAgents(transport);
    if (generation !== controller.generation.current) return;
    if (result.status === "loaded") { controller.verified.current = result.snapshot; controller.onState({ kind: "ready", snapshot: result.snapshot }); return; }
    if (result.status === "unauthenticated") controller.onUnauthenticated?.();
    controller.onState({ kind: "error", failure: result.status, previous: controller.verified.current });
  } finally { if (controller.inFlight.current === token) controller.inFlight.current = undefined; }
}
function displayDate(value: string): string { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)); }
function statusLabel(status: AutomationAgentStatus): string { return status === "active" ? "启用" : "暂停"; }
function runtimeCookieHeader(): string { return typeof document === "undefined" ? "" : document.cookie; }

export function AutomationAgentsView({ state, keyword, type, status, onKeywordChange, onTypeChange, onStatusChange, onLoad, onDetail }: {
  readonly state: AutomationAgentsState; readonly keyword: string; readonly type: AutomationAgentType | "all"; readonly status: AutomationAgentStatus | "all";
  readonly onKeywordChange: (...args: [string]) => void; readonly onTypeChange: (...args: [AutomationAgentType | "all"]) => void; readonly onStatusChange: (...args: [AutomationAgentStatus | "all"]) => void;
  readonly onLoad: () => void; readonly onDetail?: (...args: [number]) => void;
}): React.ReactElement {
  const snapshot = state.kind === "ready" ? state.snapshot : state.previous;
  const items = snapshot ? filterAutomationAgents(snapshot, keyword, type, status) : [];
  return <section className="route-card" aria-labelledby="app-title"><p className="route-card__eyebrow">自动化话术 · 本地管理</p><h1 id="app-title">自动化话术</h1><p>仅管理已持久化的本地配置；发布仅同步本地草稿版本，不执行运行、群发、Outbound 或 Provider 调用。</p>
    <p><label>本地搜索（名称或编码）<input value={keyword} onChange={(event) => onKeywordChange(event.currentTarget.value)} /></label>{" "}<label>类型<select value={type} onChange={(event) => onTypeChange(event.currentTarget.value as AutomationAgentType | "all")}><option value="all">全部</option><option value="agent">Agent 机器人</option><option value="fixed_script">固定话术</option></select></label>{" "}<label>状态<select value={status} onChange={(event) => onStatusChange(event.currentTarget.value as AutomationAgentStatus | "all")}><option value="all">全部</option><option value="active">启用</option><option value="paused">暂停</option></select></label></p>
    {snapshot ? <p>已验证本地摘要共 {snapshot.total} 条，当前筛选 {items.length} 条。</p> : null}
    {items.length ? <table><thead><tr><th>名称</th><th>编码</th><th>类型</th><th>状态</th><th>固定素材计数</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{items.map((item) => <tr key={item.id}><td>{item.name}</td><td>{item.code}</td><td>{item.typeLabel}</td><td>{statusLabel(item.status)}</td><td>{`图片 ${item.materialSummary.imageCount}；小程序 ${item.materialSummary.miniprogramCount}；附件 ${item.materialSummary.attachmentCount}；群邀请 ${item.materialSummary.groupInviteCount}`}</td><td>{displayDate(item.updatedAt)}</td><td>{onDetail ? <button type="button" disabled={state.kind === "loading"} onClick={() => onDetail(item.id)}>查看本地详情</button> : "—"}</td></tr>)}</tbody></table> : snapshot ? <p role="status">没有符合当前本地筛选条件的话术。</p> : null}
    {state.kind === "loading" ? <p role="status">正在读取本地自动化话术摘要。</p> : null}{state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}<button type="button" disabled={state.kind === "loading"} onClick={onLoad}>手动刷新本地摘要</button></section>;
}

export function AutomationAgentsPage({ role, transport = generatedAutomationAgentsTransport, readCookie = runtimeCookieHeader, onUnauthenticated }: {
  readonly role: AutomationAgentsRole; readonly transport?: AutomationAgentsTransport; readonly readCookie?: () => string; readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0); const inFlight = useRef<symbol>(); const verified = useRef<AutomationAgentsSnapshot>();
  const [state, setState] = useState<AutomationAgentsState>({ kind: "loading" }); const [keyword, setKeyword] = useState(""); const [type, setType] = useState<AutomationAgentType | "all">("all"); const [status, setStatus] = useState<AutomationAgentStatus | "all">("all");
  const [detail, setDetail] = useState<AutomationAgentDetail>(); const [detailFailure, setDetailFailure] = useState<AutomationAgentsFailure>();
  const [draft, setDraft] = useState<AutomationAgentDraft>(defaultAutomationAgentDraft); const [createMode, setCreateMode] = useState(false); const [writeMessage, setWriteMessage] = useState<string>();
  const [contentText, setContentText] = useState(""); const [imageIDs, setImageIDs] = useState(""); const [archiveConfirm, setArchiveConfirm] = useState(false);
  const detailGeneration = useRef(0); const detailInFlight = useRef<symbol>(); const writeInFlight = useRef<symbol>(); const writeUnknown = useRef(false);
  const load = useCallback(() => startAutomationAgentsRead({ generation, inFlight, verified, onState: setState, onUnauthenticated }, transport), [onUnauthenticated, transport]);
  const managementAvailable = Boolean(transport.get);
  const loadDetail = (id: number) => {
    if (!managementAvailable || detailInFlight.current || writeInFlight.current) return;
    const token = Symbol("automation-agent-detail"); detailInFlight.current = token; const current = ++detailGeneration.current; setDetailFailure(undefined);
    void loadAutomationAgentDetail(transport, id).then((result) => { if (current !== detailGeneration.current || detailInFlight.current !== token) return; if (result.status === "loaded") { setDetail(result.agent); setDraft({ name: result.agent.name, code: result.agent.code, type: result.agent.type, rolePrompt: result.agent.draftRolePrompt, taskPrompt: result.agent.draftTaskPrompt }); setContentText(result.agent.fixedContent.contentText); setImageIDs(result.agent.fixedContent.imageLibraryIDs.join(",")); return; } if (result.status === "unauthenticated") onUnauthenticated?.(); setDetailFailure(result.status); }).finally(() => { if (detailInFlight.current === token) detailInFlight.current = undefined; });
  };
  const command = (work: (...args: [string, string]) => Promise<AutomationAgentMutationResult>) => {
    if (writeInFlight.current || writeUnknown.current) return; let csrf: string | undefined;
    try { csrf = readCSRFCookie(readCookie()); } catch { csrf = undefined; } const key = newAutomationAgentIdempotencyKey();
    if (!csrf || !key) { setWriteMessage(writeMessages.forbidden); return; }
    const token = Symbol("automation-agent-write"); writeInFlight.current = token; setWriteMessage(undefined);
    void work(csrf, key).then((result) => { if (writeInFlight.current !== token) return; if (result.status === "succeeded") { setDetail(result.agent); setDraft({ name: result.agent.name, code: result.agent.code, type: result.agent.type, rolePrompt: result.agent.draftRolePrompt, taskPrompt: result.agent.draftTaskPrompt }); setContentText(result.agent.fixedContent.contentText); setImageIDs(result.agent.fixedContent.imageLibraryIDs.join(",")); void load(); return; } if (result.status === "archived") { setDetail(undefined); setArchiveConfirm(false); void load(); return; } if (result.status === "unauthenticated") onUnauthenticated?.(); if (result.status === "unknown") writeUnknown.current = true; setWriteMessage(writeMessages[result.status]); }).finally(() => { if (writeInFlight.current === token) writeInFlight.current = undefined; });
  };
  const parseIDs = (): readonly number[] | undefined => {
    const parts = imageIDs.trim() === "" ? [] : imageIDs.split(",").map((value) => value.trim());
    if (parts.some((value) => !/^[1-9]\d*$/.test(value))) return undefined; const ids = parts.map(Number);
    return ids.every((id) => Number.isSafeInteger(id)) ? ids : undefined;
  };
  useEffect(() => { if (canRead) void load(); return () => { invalidateAutomationAgentsRead({ generation, inFlight, verified, onState: setState }); detailGeneration.current += 1; detailInFlight.current = undefined; writeInFlight.current = undefined; writeUnknown.current = false; }; }, [canRead, load]);
  if (!canRead) return <section className="route-card" aria-labelledby="app-title"><h1 id="app-title">自动化话术</h1><p>当前账号没有自动化话术目录访问权限。</p></section>;
  const busy = Boolean(inFlight.current || detailInFlight.current || writeInFlight.current);
  return <><AutomationAgentsView state={state} keyword={keyword} type={type} status={status} onKeywordChange={setKeyword} onTypeChange={setType} onStatusChange={setStatus} onLoad={() => { void load(); }} onDetail={managementAvailable ? loadDetail : undefined} />
    {transport.create ? <section className="route-card" aria-label="新增本地自动化话术"><button type="button" disabled={busy || writeUnknown.current} onClick={() => setCreateMode(!createMode)}>{createMode ? "取消新增" : "新增本地自动化话术"}</button>{createMode ? <form onSubmit={(event) => { event.preventDefault(); command((csrf, key) => createAutomationAgent(transport, draft, csrf, key)); }}><fieldset disabled={busy || writeUnknown.current}><label>名称<input aria-label="名称" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })} /></label><label>编码<input aria-label="编码" value={draft.code} onChange={(event) => setDraft({ ...draft, code: event.currentTarget.value })} /></label><label>类型<select aria-label="编辑类型" value={draft.type} onChange={(event) => setDraft({ ...draft, type: event.currentTarget.value as AutomationAgentType })}><option value="agent">Agent 机器人</option><option value="fixed_script">固定话术</option></select></label><label>角色提示<textarea aria-label="角色提示" value={draft.rolePrompt} onChange={(event) => setDraft({ ...draft, rolePrompt: event.currentTarget.value })} /></label><label>任务提示<textarea aria-label="任务提示" value={draft.taskPrompt} onChange={(event) => setDraft({ ...draft, taskPrompt: event.currentTarget.value })} /></label><button type="submit">创建本地配置</button></fieldset></form> : null}</section> : null}
    {detail ? <section className="route-card" aria-label="本地自动化话术详情"><h2>本地自动化话术详情</h2>{detailFailure ? <p role="alert">{messages[detailFailure]}</p> : null}<p>编码：{detail.code}；草稿版本：{detail.draftVersion}；已发布版本：{detail.publishedVersion}。发布仅更新本地已发布版本。</p><form onSubmit={(event) => { event.preventDefault(); command((csrf, key) => updateAutomationAgent(transport, detail.id, draft, csrf, key)); }}><fieldset disabled={busy || writeUnknown.current || !transport.update}><label>名称<input aria-label="编辑名称" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })} /></label><label>类型<select aria-label="详情类型" value={draft.type} onChange={(event) => setDraft({ ...draft, type: event.currentTarget.value as AutomationAgentType })}><option value="agent">Agent 机器人</option><option value="fixed_script">固定话术</option></select></label><label>角色提示<textarea aria-label="编辑角色提示" value={draft.rolePrompt} onChange={(event) => setDraft({ ...draft, rolePrompt: event.currentTarget.value })} /></label><label>任务提示<textarea aria-label="编辑任务提示" value={draft.taskPrompt} onChange={(event) => setDraft({ ...draft, taskPrompt: event.currentTarget.value })} /></label><button type="submit">保存本地草稿</button></fieldset></form>
      {!detail.fixedContent.hasUnsupportedBindings && transport.saveFixedContent ? <form onSubmit={(event) => { event.preventDefault(); const ids = parseIDs(); if (!ids) { setWriteMessage(writeMessages.invalid); return; } command((csrf, key) => saveAutomationAgentFixedContent(transport, detail.id, draft.type, contentText, ids, csrf, key)); }}><fieldset disabled={busy || writeUnknown.current}><label>固定内容<textarea aria-label="固定内容" value={contentText} onChange={(event) => setContentText(event.currentTarget.value)} /></label><label>本地图片 ID（逗号分隔，最多 3 个）<input aria-label="本地图片 ID" value={imageIDs} onChange={(event) => setImageIDs(event.currentTarget.value)} /></label><button type="submit">保存本地固定内容</button></fieldset></form> : <p>该配置含未纳入本包 owner 合同的本地绑定，固定内容保持只读。</p>}
      <p>{transport.copy ? <button type="button" disabled={busy || writeUnknown.current} onClick={() => command((csrf, key) => copyAutomationAgent(transport, detail.id, csrf, key))}>复制本地配置</button> : null}{" "}{transport.publish ? <button type="button" disabled={busy || writeUnknown.current || !detail.hasUnpublishedChanges} onClick={() => command((csrf, key) => publishAutomationAgent(transport, detail.id, csrf, key))}>发布本地草稿</button> : null}{" "}{detail.status === "active" && transport.pause ? <button type="button" disabled={busy || writeUnknown.current} onClick={() => command((csrf, key) => setAutomationAgentStatus(transport, detail.id, "paused", csrf, key))}>暂停本地配置</button> : null}{" "}{detail.status === "paused" && transport.activate ? <button type="button" disabled={busy || writeUnknown.current} onClick={() => command((csrf, key) => setAutomationAgentStatus(transport, detail.id, "active", csrf, key))}>启用本地配置</button> : null}</p>
      {transport.archive ? <p><label><input type="checkbox" checked={archiveConfirm} disabled={busy || writeUnknown.current} onChange={(event) => setArchiveConfirm(event.currentTarget.checked)} />我确认归档这条本地配置</label><button type="button" disabled={busy || writeUnknown.current || !archiveConfirm} onClick={() => command((csrf, key) => archiveAutomationAgent(transport, detail.id, csrf, key))}>归档本地配置</button></p> : null}</section> : null}
    {writeMessage ? <p className="route-card" role="alert">{writeMessage}</p> : null}</>;
}
