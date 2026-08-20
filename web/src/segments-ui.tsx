import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  archiveSegment,
  buildDefinition,
  editorDraft,
  generatedSegmentTransport,
  loadSegmentMembers,
  loadSegments,
  refreshSegment,
  saveSegment,
  type SegmentConditionDraft,
  type SegmentEditorDraft,
  type SegmentFailure,
  type SegmentRecord,
  type SegmentRole,
  type SegmentTransport,
} from "./segments";
import "./segments.css";

export interface SegmentsPageProps {
  readonly role: SegmentRole;
  readonly transport?: SegmentTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}

type ListState = { readonly kind: "loading" } | { readonly kind: "ready"; readonly items: readonly SegmentRecord[]; readonly nextCursor?: string } | { readonly kind: "error"; readonly failure: SegmentFailure };
type MembersState = { readonly kind: "idle" } | { readonly kind: "loading" } | { readonly kind: "ready"; readonly items: readonly { id: number; name: string }[]; readonly nextCursor?: string } | { readonly kind: "error"; readonly failure: SegmentFailure };

const messages: Record<SegmentFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有此人群包操作权限。",
  not_found: "该人群包已不存在，请刷新列表后重试。",
  conflict: "重复提交与已有操作冲突，请刷新后重试。",
  invalid: "提交内容不符合已冻结的人群包条件规则。",
  unavailable: "人群包服务暂时不可用，请稍后重试。",
};
const fields = [
  ["stage_id", "阶段 ID"], ["owner_staff_id", "负责人 ID"], ["channel_id", "渠道 ID"], ["tag_id", "标签 ID"], ["added_at", "加入时间"], ["last_interact_at", "最近互动"], ["is_deleted", "删除状态"],
] as const;

function browserCookie(): string { return typeof document === "undefined" ? "" : document.cookie; }
function sameSegment(left: SegmentRecord, right: SegmentRecord): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}
function mutationKey(): string | undefined {
  try {
    const uuid = globalThis.crypto?.randomUUID();
    return typeof uuid === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(uuid)
      ? `segment:${uuid}`
      : undefined;
  } catch {
    return undefined;
  }
}
function operators(field: SegmentConditionDraft extends infer T ? T extends { readonly field: infer F } ? F : never : never): readonly [string, string][] {
  if (field === "is_deleted") return [["eq", "等于"]];
  if (field === "added_at" || field === "last_interact_at") return [["before", "早于"], ["after", "晚于"]];
  if (field === "tag_id") return [["has_any", "包含任一"]];
  return [["eq", "等于"], ["in", "属于任一"]];
}
function normalizedCondition(condition: SegmentConditionDraft): SegmentConditionDraft {
  if (condition.kind === "group") return condition;
  const allowed = operators(condition.field).map(([value]) => value);
  return allowed.includes(condition.operator) ? condition : { ...condition, operator: allowed[0] as SegmentConditionDraft extends infer T ? T extends { readonly operator: infer O } ? O : never : never };
}
function ConditionEditor({ value, onChange }: { readonly value: SegmentConditionDraft; readonly onChange: React.Dispatch<SegmentConditionDraft> }) {
  if (value.kind === "group") return <fieldset className="segment-condition segment-condition--group"><legend>组合条件</legend><label>组合方式<select value={value.combinator} onChange={(event) => onChange({ ...value, combinator: event.currentTarget.value as typeof value.combinator })}><option value="and">同时满足（AND）</option><option value="or">满足任一（OR）</option></select></label>{value.children.map((child, index) => <ConditionEditor key={index} value={child} onChange={(next) => onChange({ ...value, children: value.children.map((current, currentIndex) => currentIndex === index ? next : current) })} />)}<div className="segment-condition__actions"><button type="button" onClick={() => onChange({ ...value, children: [...value.children, { kind: "predicate", field: "stage_id", operator: "eq", value: "" }] })}>添加条件</button><button type="button" onClick={() => onChange({ ...value, children: [...value.children, { kind: "group", combinator: "and", children: [{ kind: "predicate", field: "stage_id", operator: "eq", value: "" }] }] })}>添加条件组</button></div></fieldset>;
  const allowed = operators(value.field);
  const inputHint = value.field === "is_deleted" ? "选择状态" : value.field === "added_at" || value.field === "last_interact_at" ? "例如 -30d 或 2026-08-13T00:00:00Z" : value.operator === "eq" ? "正整数 ID" : "多个正整数 ID，以英文逗号分隔";
  return <div className="segment-condition"><label>字段<select value={value.field} onChange={(event) => onChange(normalizedCondition({ ...value, field: event.currentTarget.value as typeof value.field }))}>{fields.map(([field, label]) => <option key={field} value={field}>{label}</option>)}</select></label><label>操作符<select value={value.operator} onChange={(event) => onChange({ ...value, operator: event.currentTarget.value as typeof value.operator })}>{allowed.map(([operator, label]) => <option key={operator} value={operator}>{label}</option>)}</select></label><label>条件值{value.field === "is_deleted" ? <select value={value.value} onChange={(event) => onChange({ ...value, value: event.currentTarget.value })}><option value="false">否（正常）</option><option value="true">是（已删除）</option></select> : <input aria-label="条件值" value={value.value} placeholder={inputHint} onChange={(event) => onChange({ ...value, value: event.currentTarget.value })} />}</label></div>;
}

export function SegmentsPage({ role, transport = generatedSegmentTransport, readCookie = browserCookie, onUnauthenticated }: SegmentsPageProps): React.ReactElement {
  const canWrite = role === "admin" || role === "ops";
  const [list, setList] = useState<ListState>({ kind: "loading" });
  const [members, setMembers] = useState<MembersState>({ kind: "idle" });
  const [selected, setSelected] = useState<SegmentRecord>();
  const [draft, setDraft] = useState<SegmentEditorDraft>(editorDraft());
  const [notice, setNotice] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const writeInFlight = useRef(false);
  const outcomeUnknown = useRef(false);

  const loadList = useCallback(async (cursor?: string) => {
    setList({ kind: "loading" });
    const result = await loadSegments(transport, cursor);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setList(result.status === "loaded" ? { kind: "ready", items: result.items, ...(result.nextCursor ? { nextCursor: result.nextCursor } : {}) } : { kind: "error", failure: result.status });
  }, [onUnauthenticated, transport]);
  const loadMembers = useCallback(async (segment: SegmentRecord, cursor?: string) => {
    setMembers({ kind: "loading" });
    const result = await loadSegmentMembers(transport, segment.id, cursor);
    if (result.status === "unauthenticated") onUnauthenticated?.();
    setMembers(result.status === "loaded" ? { kind: "ready", items: result.items.map(({ id, name }) => ({ id, name })), ...(result.nextCursor ? { nextCursor: result.nextCursor } : {}) } : { kind: "error", failure: result.status });
  }, [onUnauthenticated, transport]);
  useEffect(() => { if (canWrite) void loadList(); }, [canWrite, loadList]);

  const select = (segment: SegmentRecord) => { setSelected(segment); setDraft(editorDraft(segment)); setNotice(undefined); void loadMembers(segment); };
  const csrf = (): string | undefined => { try { return readCSRFCookie(readCookie()); } catch { return undefined; } };
  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canWrite || saving || writeInFlight.current || outcomeUnknown.current) return;
    const definition = buildDefinition(draft.condition);
    if (!definition.ok) { setNotice(definition.message); return; }
    const token = csrf();
    if (!token) { setNotice("安全令牌缺失，未发送保存请求。"); return; }
	const key = mutationKey();
	if (!key) { setNotice("安全随机源不可用，未发送保存请求。"); return; }
    writeInFlight.current = true;
    setSaving(true); setNotice(undefined);
    try {
	  const result = await saveSegment(transport, selected, draft, token, key);
      if (result.status !== "saved") {
        if (result.status === "unavailable") outcomeUnknown.current = true;
        setNotice(messages[result.status]);
        if (result.status === "unauthenticated") onUnauthenticated?.();
        return;
      }
      const reread = await loadSegments(transport);
      if (reread.status === "unauthenticated") {
        onUnauthenticated?.();
        return;
      }
      if (reread.status !== "loaded" || !reread.items.some((item) => sameSegment(item, result.segment))) {
        outcomeUnknown.current = true;
        setNotice(messages.unavailable);
        return;
      }
      setList({ kind: "ready", items: reread.items, ...(reread.nextCursor ? { nextCursor: reread.nextCursor } : {}) });
      setSelected(result.segment); setDraft(editorDraft(result.segment));
      const membersRead = await loadSegmentMembers(transport, result.segment.id);
      if (membersRead.status === "unauthenticated") {
        onUnauthenticated?.();
        return;
      }
      if (membersRead.status !== "loaded") {
        outcomeUnknown.current = true;
        setNotice(messages.unavailable);
        return;
      }
      setMembers({ kind: "ready", items: membersRead.items.map(({ id, name }) => ({ id, name })), ...(membersRead.nextCursor ? { nextCursor: membersRead.nextCursor } : {}) });
      setNotice(selected ? "人群包已保存并已回读确认。" : "人群包已创建并已回读确认。");
    } finally {
      writeInFlight.current = false;
      setSaving(false);
    }
  };
  const refresh = async () => {
    if (!selected || refreshing || writeInFlight.current || outcomeUnknown.current || !canWrite) return;
    const token = csrf();
    if (!token) { setNotice("安全令牌缺失，未发送手动刷新请求。"); return; }
	const key = mutationKey();
	if (!key) { setNotice("安全随机源不可用，未发送手动刷新请求。"); return; }
    writeInFlight.current = true;
    setRefreshing(true); setNotice(undefined);
    try {
	  const result = await refreshSegment(transport, selected.id, token, key);
      if (result !== "accepted") {
        if (result === "unavailable") outcomeUnknown.current = true;
        setNotice(messages[result]);
        if (result === "unauthenticated") onUnauthenticated?.();
        return;
      }
      const [reread, membersRead] = await Promise.all([
        loadSegments(transport),
        loadSegmentMembers(transport, selected.id),
      ]);
      if (reread.status === "unauthenticated" || membersRead.status === "unauthenticated") {
        onUnauthenticated?.();
        return;
      }
      const confirmed = reread.status === "loaded" &&
        reread.items.some((item) => item.id === selected.id) &&
        membersRead.status === "loaded";
      if (!confirmed) {
        outcomeUnknown.current = true;
        setNotice(messages.unavailable);
        return;
      }
      setList({ kind: "ready", items: reread.items, ...(reread.nextCursor ? { nextCursor: reread.nextCursor } : {}) });
      setMembers({ kind: "ready", items: membersRead.items.map(({ id, name }) => ({ id, name })), ...(membersRead.nextCursor ? { nextCursor: membersRead.nextCursor } : {}) });
      setNotice("刷新请求已接受并由列表与预览回读确认；这不表示成员已更新或产生任何外部效果。");
    } finally {
      writeInFlight.current = false;
      setRefreshing(false);
    }
  };
  const archive = async () => {
    if (!selected || !transport.archive || archiving || writeInFlight.current || outcomeUnknown.current || !canWrite) return;
    if (typeof window === "undefined" || !window.confirm(`确认归档本地人群包“${selected.name}”？归档会保留当前快照且停止后续刷新。`)) return;
    const token = csrf();
    const key = mutationKey();
    if (!token || !key) { setNotice(!token ? "安全令牌缺失，未发送归档请求。" : "安全随机源不可用，未发送归档请求。"); return; }
    writeInFlight.current = true;
    setArchiving(true); setNotice(undefined);
    try {
      const result = await archiveSegment(transport, selected.id, token, key);
      if (result.status !== "archived") {
        if (result.status === "unavailable") outcomeUnknown.current = true;
        setNotice(messages[result.status]);
        if (result.status === "unauthenticated") onUnauthenticated?.();
        return;
      }
      const reread = await loadSegments(transport);
      if (reread.status === "unauthenticated") { onUnauthenticated?.(); return; }
      if (reread.status !== "loaded" || reread.items.some((item) => item.id === selected.id)) {
        outcomeUnknown.current = true;
        setNotice(messages.unavailable);
        return;
      }
      setList({ kind: "ready", items: reread.items, ...(reread.nextCursor ? { nextCursor: reread.nextCursor } : {}) });
      setSelected(undefined); setDraft(editorDraft()); setMembers({ kind: "idle" });
      setNotice("人群包已归档并已由列表回读确认；保留的快照不会再刷新。");
    } finally {
      writeInFlight.current = false;
      setArchiving(false);
    }
  };

  if (!canWrite) return <section className="segments-page" aria-labelledby="app-title"><p className="route-card__eyebrow">人群包</p><h1 id="app-title">人群包</h1><p className="segments-page__state" role="alert">当前账号没有人群包访问权限。</p></section>;
  return <section className="segments-page" aria-labelledby="app-title"><div className="segments-page__heading"><div><p className="route-card__eyebrow">受众运营</p><h1 id="app-title">人群包</h1><p>条件、成员数和刷新状态均以服务端当前事实为准。</p></div><button type="button" disabled={outcomeUnknown.current || writeInFlight.current} onClick={() => { setSelected(undefined); setDraft(editorDraft()); setMembers({ kind: "idle" }); setNotice("已开始创建新的人群包。"); }}>新建人群包</button></div>{notice && <p className="segments-page__notice" role="alert">{notice}</p>}<div className="segments-page__grid"><section className="segments-page__panel" aria-labelledby="segment-list-title"><h2 id="segment-list-title">人群包列表</h2>{list.kind === "loading" && <p role="status">正在读取人群包…</p>}{list.kind === "error" && <div role="alert"><p>{messages[list.failure]}</p><button type="button" onClick={() => void loadList()}>重试</button></div>}{list.kind === "ready" && <><ul className="segment-list">{list.items.map((segment) => <li key={segment.id}><button aria-pressed={selected?.id === segment.id} disabled={writeInFlight.current || outcomeUnknown.current} type="button" onClick={() => select(segment)}><strong>{segment.name}</strong><span>{segment.memberCount} 名成员 · {segment.refreshStatus}</span></button></li>)}</ul>{list.items.length === 0 && <p role="status">暂无人群包。请创建第一个人群包。</p>}{list.nextCursor && <button type="button" onClick={() => void loadList(list.nextCursor)}>读取更多人群包</button>}</>}</section><form className="segments-page__panel segment-editor" onSubmit={save}><h2>{selected ? "编辑人群包" : "新建人群包"}</h2><fieldset disabled={saving || archiving || outcomeUnknown.current}><label>名称<input maxLength={200} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })} /></label><label>刷新方式<select value={draft.refreshMode} onChange={(event) => setDraft({ ...draft, refreshMode: event.currentTarget.value as SegmentEditorDraft["refreshMode"] })}><option value="manual">手动刷新</option><option value="scheduled">定时刷新</option></select></label>{draft.refreshMode === "scheduled" && <label>定时表达式<input maxLength={200} value={draft.refreshCron} onChange={(event) => setDraft({ ...draft, refreshCron: event.currentTarget.value })} /></label>}<fieldset className="segment-editor__conditions"><legend>条件编辑器</legend><p>只支持已冻结字段与操作符；不接受 JSON、标识符或 SQL 文本。</p><ConditionEditor value={draft.condition} onChange={(condition) => setDraft({ ...draft, condition })} />{draft.condition.kind === "predicate" && <button type="button" onClick={() => setDraft({ ...draft, condition: { kind: "group", combinator: "and", children: [draft.condition] } })}>组合 AND/OR 条件</button>}</fieldset><button type="submit">{saving ? "正在保存…" : selected ? "保存人群包" : "创建人群包"}</button></fieldset></form><section className="segments-page__panel" aria-labelledby="member-preview-title"><h2 id="member-preview-title">成员预览</h2>{!selected && <p>选择一个人群包后预览其已物化成员。</p>}{selected && <><p className="segment-preview__meta">当前人群包：{selected.name} · 服务端成员数：{selected.memberCount}</p><button type="button" disabled={refreshing || archiving || outcomeUnknown.current} onClick={() => void refresh()}>{refreshing ? "正在请求刷新…" : "手动刷新"}</button>{transport.archive && <button type="button" disabled={archiving || refreshing || outcomeUnknown.current} onClick={() => void archive()}>{archiving ? "正在归档…" : "归档人群包"}</button>}{members.kind === "loading" && <p role="status">正在读取已物化成员…</p>}{members.kind === "error" && <div role="alert"><p>{messages[members.failure]}</p><button type="button" onClick={() => void loadMembers(selected)}>重试预览</button></div>}{members.kind === "ready" && <><ol className="segment-preview__members">{members.items.map((member) => <li key={member.id}>{member.name.trim() || "未命名客户"} <span>OneID {member.id}</span></li>)}</ol>{members.items.length === 0 && <p role="status">该人群包当前没有已物化成员。</p>}{members.nextCursor && <button type="button" onClick={() => void loadMembers(selected, members.nextCursor)}>读取更多成员</button>}</>}</>}</section></div></section>;
}
