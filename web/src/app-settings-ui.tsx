import React, { useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  appSettingsDraft,
  canManageAppSettings,
  generatedAppSettingsTransport,
  loadAppSettings,
  newAppSettingsRequestID,
  saveAppSettings,
  type AppSettingsEditableKey,
  type AppSettingsFailure,
  type AppSettingsRole,
  type AppSettingsSnapshot,
  type AppSettingsTransport,
} from "./app-settings";

const readMessages: Record<AppSettingsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有本地应用设置管理权限。",
  invalid: "本地应用设置响应不符合已冻结合同。",
  unavailable: "本地应用设置暂不可用，请稍后再查看。",
};
const saveMessages: Record<"unauthenticated" | "forbidden" | "invalid" | "conflict" | "unknown", string> = {
  unauthenticated: readMessages.unauthenticated,
  forbidden: readMessages.forbidden,
  invalid: "待保存的本地设置或服务器回执不符合已冻结合同。",
  conflict: "本地设置请求发生冲突；未自动重试，请刷新后核对本地设置。",
  unknown: "本地设置保存结果未知，已锁定本页再次保存；请刷新后核对本地设置。",
};
type ViewState =
  | { readonly kind: "loading"; readonly previous?: AppSettingsSnapshot }
  | { readonly kind: "ready"; readonly snapshot: AppSettingsSnapshot }
  | { readonly kind: "error"; readonly failure: AppSettingsFailure; readonly previous?: AppSettingsSnapshot };
type SaveState =
  | { readonly kind: "idle" }
  | { readonly kind: "saving" }
  | { readonly kind: "saved" }
  | { readonly kind: "error"; readonly message: string }
  | { readonly kind: "unknown"; readonly message: string };

function runtimeCookieHeader(): string { return typeof document === "undefined" ? "" : document.cookie; }
function displayDate(value: string | undefined): string { return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)) : "—"; }

export function AppSettingsView({ state, draft, confirmed, saveState, onDraftChange, onConfirmationChange, onSave }: {
  readonly state: ViewState;
  readonly draft: Record<AppSettingsEditableKey, string>;
  readonly confirmed: boolean;
  readonly saveState: SaveState;
  // eslint-disable-next-line no-unused-vars -- named tuple documents the controlled input callback.
  readonly onDraftChange: (...args: [AppSettingsEditableKey, string]) => void;
  // eslint-disable-next-line no-unused-vars -- named tuple documents the confirmation callback.
  readonly onConfirmationChange: (...args: [boolean]) => void;
  // eslint-disable-next-line no-unused-vars -- named tuple documents the form callback.
  readonly onSave: (...args: [React.FormEvent<HTMLFormElement>]) => void;
}): React.ReactElement {
  const snapshot = state.kind === "ready" ? state.snapshot : state.previous;
  const disabled = state.kind === "loading" || saveState.kind === "saving" || saveState.kind === "unknown" || !snapshot?.actionToken;
  return <section className="route-card" aria-labelledby="app-title"><p className="route-card__eyebrow">系统设置 · 本地配置</p><h1 id="app-title">本地应用设置</h1><p>只管理四项已持久化的非敏感本地设置。敏感信息只显示配置状态；保存不会启动 Worker、Outbound、Provider、发布或任何外部操作。</p>
    {snapshot ? <><section aria-label="本地设置摘要"><h2>配置摘要</h2><dl>{snapshot.summary.map((item) => <React.Fragment key={item.label}><dt>{item.label}</dt><dd>{item.value} · {item.description}</dd></React.Fragment>)}</dl></section>
      <section aria-label="敏感设置状态"><h2>敏感设置状态</h2><ul>{snapshot.masked.map((item) => <li key={item.key}><code>{item.key}</code>：{item.configured ? "已配置（已掩码）" : "未配置（已掩码）"}</li>)}</ul></section>
      <section aria-label="编辑本地设置"><h2>编辑本地设置</h2>{!snapshot.actionToken ? <p role="alert">当前会话没有本地设置保存令牌，未开放保存。</p> : null}<form onSubmit={onSave}><fieldset disabled={disabled}>{snapshot.editable.map((item) => <label key={item.key}>{item.label}<input aria-label={item.label} type={item.inputType} inputMode={item.inputType === "number" ? "numeric" : undefined} value={draft[item.key]} onChange={(event) => onDraftChange(item.key, event.currentTarget.value)} /></label>)}<label><input aria-label="确认保存本地设置" type="checkbox" checked={confirmed} onChange={(event) => onConfirmationChange(event.currentTarget.checked)} />我确认保存本次本地设置修改</label><button type="submit" disabled={!confirmed}>保存本地设置</button></fieldset></form>{saveState.kind === "saving" ? <p role="status">正在保存本地设置。</p> : null}{saveState.kind === "saved" ? <p role="status">本地设置已保存并通过回执校验。</p> : null}{saveState.kind === "error" || saveState.kind === "unknown" ? <p role="alert">{saveState.message}</p> : null}</section>
      <section aria-label="本地设置审计"><h2>最近本地审计</h2>{snapshot.audits.length ? <table><thead><tr><th>设置项</th><th>动作</th><th>操作人</th><th>时间</th></tr></thead><tbody>{snapshot.audits.map((entry) => <tr key={entry.id}><td><code>{entry.targetID}</code></td><td>{entry.actionType}</td><td>{entry.operator}</td><td>{displayDate(entry.createdAt)}</td></tr>)}</tbody></table> : <p>暂无本地设置审计记录。</p>}</section>
      <section aria-label="当前本地设置"><h2>当前设置</h2><table><thead><tr><th>设置项</th><th>当前值</th><th>来源</th><th>最近修改</th></tr></thead><tbody>{snapshot.editable.map((item) => <tr key={item.key}><td><code>{item.key}</code></td><td>{item.value || "—"}</td><td>{item.source}</td><td>{item.configured ? `${item.lastActionType} · ${item.lastModifiedBy ?? "—"} · ${displayDate(item.lastModifiedAt)}` : "—"}</td></tr>)}</tbody></table></section>
    </> : null}
    {state.kind === "loading" ? <p role="status">正在读取本地应用设置。</p> : null}{state.kind === "error" ? <p role="alert">{readMessages[state.failure]}</p> : null}
  </section>;
}

export function AppSettingsPage({
  role,
  transport = generatedAppSettingsTransport,
  readCookie = runtimeCookieHeader,
  onUnauthenticated,
}: {
  readonly role: AppSettingsRole;
  readonly transport?: AppSettingsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canAccess = canManageAppSettings(role);
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [draft, setDraft] = useState<Record<AppSettingsEditableKey, string>>({ "wecom.corp_id": "", "wecom.agent_id": "", "outbound.rate_per_second": "", "outbound.max_attempts": "" });
  const [confirmed, setConfirmed] = useState(false);
  const [saveState, setSaveState] = useState<SaveState>({ kind: "idle" });
  const generation = useRef(0); const loadInFlight = useRef<symbol>(); const saveInFlight = useRef<symbol>(); const verified = useRef<AppSettingsSnapshot>(); const unknownWrite = useRef(false);
  const load = () => {
    if (!canAccess || loadInFlight.current || saveInFlight.current) return;
    const token = Symbol("app-settings-read"); loadInFlight.current = token; const current = ++generation.current;
    setState({ kind: "loading", previous: verified.current });
    void loadAppSettings(transport).then((result) => {
      if (generation.current !== current || loadInFlight.current !== token) return;
      if (result.status === "loaded") { verified.current = result.snapshot; setDraft(appSettingsDraft(result.snapshot)); setState({ kind: "ready", snapshot: result.snapshot }); return; }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setState({ kind: "error", failure: result.status, previous: verified.current });
    }).finally(() => { if (loadInFlight.current === token) loadInFlight.current = undefined; });
  };
  useEffect(() => {
    if (canAccess) load();
    return () => { generation.current += 1; loadInFlight.current = undefined; saveInFlight.current = undefined; unknownWrite.current = false; };
  }, [canAccess, transport]);
  if (!canAccess) return <section className="route-card" aria-labelledby="app-title"><h1 id="app-title">本地应用设置</h1><p>当前账号没有本地应用设置管理权限。</p></section>;
  const submit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const snapshot = verified.current;
    if (!snapshot || !confirmed || saveInFlight.current || unknownWrite.current) return;
    let csrf: string | undefined;
    try { csrf = readCSRFCookie(readCookie()); } catch { csrf = undefined; }
    const requestID = newAppSettingsRequestID();
    if (!csrf || !requestID) { setSaveState({ kind: "error", message: saveMessages.forbidden }); return; }
    const token = Symbol("app-settings-save"); saveInFlight.current = token; const current = ++generation.current; setSaveState({ kind: "saving" });
    void saveAppSettings(transport, snapshot, draft, csrf, requestID).then((result) => {
      if (generation.current !== current || saveInFlight.current !== token) return;
      if (result.status === "saved") { verified.current = result.snapshot; setDraft(appSettingsDraft(result.snapshot)); setConfirmed(false); setState({ kind: "ready", snapshot: result.snapshot }); setSaveState({ kind: "saved" }); return; }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      if (result.status === "unknown") { unknownWrite.current = true; setSaveState({ kind: "unknown", message: saveMessages.unknown }); return; }
      setSaveState({ kind: "error", message: saveMessages[result.status] });
    }).finally(() => { if (saveInFlight.current === token) saveInFlight.current = undefined; });
  };
  return <AppSettingsView state={state} draft={draft} confirmed={confirmed} saveState={saveState} onDraftChange={(key, value) => { setDraft((current) => ({ ...current, [key]: value })); setSaveState({ kind: "idle" }); }} onConfirmationChange={setConfirmed} onSave={submit} />;
}
