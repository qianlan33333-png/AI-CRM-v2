import React, { useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  ADMIN_CONFIG_LOCAL_RELEASE_NOTICE,
  ADMIN_CONFIG_UNKNOWN_NOTICE,
  AdminConfigResponseEpoch,
  DIRECT_KEY_CONFIRMATION_PHRASES,
  activateAPIClient,
  canManageAdminConfig,
  checkCategoryLocally,
  createAPIClient,
  createReleaseDraft,
  disableAPIClient,
  disableDirectAPIKey,
  filterAPIClients,
  generateDirectAPIKey,
  generatedAdminConfigTransport,
  hasCompleteAdminConfigActionTokens,
  loadAdminConfigOverview,
  loadAPIClient,
  loadCategory,
  loadRelease,
  loadShadowComparison,
  newAdminConfigRequestID,
  parseSafeJSONObjectText,
  publishReleaseLocally,
  redactAdminConfigError,
  rollbackReleaseLocally,
  rotateAPIClientSecret,
  rotateDirectAPIKey,
  saveCategorySettings,
  setCategoryEnabled,
  updateAPIClient,
  validateReleaseLocally,
  type AdminCategory,
  type AdminConfigReadFailure,
  type AdminConfigReadResult,
  type AdminConfigRole,
  type AdminConfigSecurity,
  type AdminConfigTransport,
  type AdminConfigWriteFailure,
  type AdminCredential,
  type AdminCredentialState,
  type AdminRelease,
  type CategoryCheckResult,
  type DirectAPIKeySnapshot,
  type DirectKeyConfirmation,
  type SafeJSONObject,
  type ShadowComparison,
} from "./admin-config-center";

export type AdminConfigSection = "overview" | "clients" | "direct-key" | "categories" | "releases";

export type AdminConfigSectionState<T> =
  | { readonly kind: "loading"; readonly previous?: T }
  | { readonly kind: "ready"; readonly value: T }
  | { readonly kind: "error"; readonly failure: AdminConfigReadFailure; readonly previous?: T };

export type AdminConfigWriteViewState =
  | { readonly kind: "idle" }
  | { readonly kind: "saving"; readonly label: string }
  | { readonly kind: "saved"; readonly label: string }
  | { readonly kind: "error"; readonly failure: Exclude<AdminConfigWriteFailure, "conflict" | "unknown"> }
  | { readonly kind: "conflict" }
  | { readonly kind: "unknown" };

/* eslint-disable no-unused-vars -- named parameters document the public view callback contract. */
export interface AdminConfigCenterViewProps {
  readonly activeSection: AdminConfigSection;
  readonly clients: AdminConfigSectionState<readonly AdminCredential[]>;
  readonly directKey: AdminConfigSectionState<DirectAPIKeySnapshot>;
  readonly categories: AdminConfigSectionState<readonly AdminCategory[]>;
  readonly releases: AdminConfigSectionState<readonly AdminRelease[]>;
  readonly selectedClient?: AdminConfigSectionState<AdminCredential>;
  readonly selectedCategory?: AdminConfigSectionState<AdminCategory>;
  readonly selectedRelease?: AdminConfigSectionState<AdminRelease>;
  readonly shadowComparison?: AdminConfigSectionState<ShadowComparison>;
  readonly categoryCheck?: AdminConfigSectionState<CategoryCheckResult>;
  readonly writeState: AdminConfigWriteViewState;
  readonly writesAvailable: boolean;
  readonly onSectionChange: (section: AdminConfigSection) => void;
  readonly onRefresh: () => void;
  readonly onSelectClient: (clientID: string) => void;
  readonly onCreateClient: (input: { readonly clientID: string; readonly displayName: string; readonly metadata: SafeJSONObject }) => void;
  readonly onUpdateClient: (input: { readonly clientID: string; readonly expectedVersion: number; readonly displayName: string; readonly metadata: SafeJSONObject }) => void;
  readonly onRotateClient: (client: AdminCredential) => void;
  readonly onActivateClient: (client: AdminCredential, secretRef: string, copiedConfirmed: boolean) => void;
  readonly onDisableClient: (client: AdminCredential) => void;
  readonly onDirectKeyAction: (action: "generate" | "rotate" | "disable", confirmation: DirectKeyConfirmation) => void;
  readonly onSelectCategory: (key: string) => void;
  readonly onSetCategoryEnabled: (category: AdminCategory, enabled: boolean) => void;
  readonly onSaveCategorySettings: (category: AdminCategory, settings: SafeJSONObject) => void;
  readonly onCheckCategory: (category: AdminCategory) => void;
  readonly onCreateRelease: (changes: SafeJSONObject) => void;
  readonly onSelectRelease: (releaseID: number) => void;
  readonly onValidateRelease: (release: AdminRelease) => void;
  readonly onLoadShadowComparison: (release: AdminRelease) => void;
  readonly onPublishRelease: (release: AdminRelease) => void;
  readonly onRollbackRelease: (release: AdminRelease) => void;
}
/* eslint-enable no-unused-vars */

const sectionLabels: Readonly<Record<AdminConfigSection, string>> = {
  overview: "概览",
  clients: "API Clients",
  "direct-key": "Direct API Key",
  categories: "配置分类",
  releases: "Release 历史 / 详情",
};

const stateLabels: Readonly<Record<AdminCredentialState, string>> = {
  active: "已激活",
  disabled: "已停用",
  pending_activation: "待激活",
};

const releaseStateLabels: Readonly<Record<AdminRelease["state"], string>> = {
  draft: "草稿",
  validated: "已校验",
  published: "本地已发布",
  rolled_back: "本地回滚记录",
};

function sectionValue<T>(state: AdminConfigSectionState<T>): T | undefined {
  return state.kind === "ready" ? state.value : state.previous;
}

function displayDate(value: string | undefined): string {
  if (!value) return "—";
  try {
    return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value));
  } catch {
    return "—";
  }
}

function safeJSONText(value: SafeJSONObject): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return "{}";
  }
}

function SectionStatus<T>({ state, label }: { readonly state: AdminConfigSectionState<T>; readonly label: string }): React.ReactElement | null {
  if (state.kind === "loading") return <p className="admin-config-status" role="status">正在读取{label}。</p>;
  if (state.kind === "error") return <p className="admin-config-alert" role="alert">{label}：{redactAdminConfigError(state.failure)}</p>;
  return null;
}

function WriteStatus({ state }: { readonly state: AdminConfigWriteViewState }): React.ReactElement | null {
  switch (state.kind) {
    case "idle": return null;
    case "saving": return <p className="admin-config-status" role="status">正在执行：{state.label}。请求只提交一次。</p>;
    case "saved": return <p className="admin-config-success" role="status">{state.label}已完成，并通过本地回读核验。</p>;
    case "error": return <p className="admin-config-alert" role="alert">{redactAdminConfigError(state.failure)}</p>;
    case "conflict": return <p className="admin-config-alert" role="alert">{redactAdminConfigError("conflict")}</p>;
    case "unknown": return <p className="admin-config-alert admin-config-alert--critical" role="alert">{ADMIN_CONFIG_UNKNOWN_NOTICE}</p>;
  }
}

function OverviewPanel({
  clients,
  directKey,
  categories,
  releases,
}: Pick<AdminConfigCenterViewProps, "clients" | "directKey" | "categories" | "releases">): React.ReactElement {
  const clientValue = sectionValue(clients);
  const keyValue = sectionValue(directKey);
  const categoryValue = sectionValue(categories);
  const releaseValue = sectionValue(releases);
  const activeClients = clientValue?.filter((item) => item.state === "active").length ?? 0;
  const enabledCategories = categoryValue?.filter((item) => item.enabled).length ?? 0;
  const latestRelease = releaseValue?.[0];
  return <section className="admin-config-panel" aria-labelledby="admin-config-overview-title">
    <div className="admin-config-panel__heading"><div><p className="admin-config-eyebrow">Admin · Local control plane</p><h2 id="admin-config-overview-title">配置控制中心概览</h2></div></div>
    <div className="admin-config-metrics" aria-label="配置摘要">
      <article><strong>{clientValue?.length ?? "—"}</strong><span>API Clients</span><small>{activeClients} 个已激活</small></article>
      <article><strong>{keyValue?.configured ? "已配置" : keyValue ? "未配置" : "—"}</strong><span>Direct API Key</span><small>{keyValue?.configured ? `${stateLabels[keyValue.state ?? "disabled"]} · v${keyValue.version ?? "—"}` : "仅显示掩码与状态"}</small></article>
      <article><strong>{categoryValue?.length ?? "—"}</strong><span>配置分类</span><small>{enabledCategories} 个已启用</small></article>
      <article><strong>{releaseValue?.length ?? "—"}</strong><span>Release 记录</span><small>{latestRelease ? `最近：${releaseStateLabels[latestRelease.state]}` : "无本地记录"}</small></article>
    </div>
    <div className="admin-config-grid admin-config-grid--two">
      <article className="admin-config-card"><h3>凭据安全边界</h3><p>只读取和提交 <code>secret_ref</code> 与 <code>secret_mask</code>。页面不接收、不显示、不记录真实 Secret。</p><SectionStatus state={clients} label="API Clients" /><SectionStatus state={directKey} label="Direct API Key" /></article>
      <article className="admin-config-card admin-config-card--gold"><h3>{ADMIN_CONFIG_LOCAL_RELEASE_NOTICE}</h3><p>发布与回滚只改变本地配置 Release 状态，不执行部署、Provider 调用或任何外部效果。</p><SectionStatus state={categories} label="配置分类" /><SectionStatus state={releases} label="Release" /></article>
    </div>
  </section>;
}

function APIClientsPanel({
  state,
  selected,
  writesDisabled,
  onSelect,
  onCreate,
  onUpdate,
  onRotate,
  onActivate,
  onDisable,
}: {
  readonly state: AdminConfigSectionState<readonly AdminCredential[]>;
  readonly selected?: AdminConfigSectionState<AdminCredential>;
  readonly writesDisabled: boolean;
  readonly onSelect: AdminConfigCenterViewProps["onSelectClient"];
  readonly onCreate: AdminConfigCenterViewProps["onCreateClient"];
  readonly onUpdate: AdminConfigCenterViewProps["onUpdateClient"];
  readonly onRotate: AdminConfigCenterViewProps["onRotateClient"];
  readonly onActivate: AdminConfigCenterViewProps["onActivateClient"];
  readonly onDisable: AdminConfigCenterViewProps["onDisableClient"];
}): React.ReactElement {
  const clients = sectionValue(state) ?? [];
  const selectedClient = selected ? sectionValue(selected) : undefined;
  const [query, setQuery] = useState("");
  const [stateFilter, setStateFilter] = useState<"all" | AdminCredentialState>("all");
  const [createID, setCreateID] = useState("");
  const [createName, setCreateName] = useState("");
  const [createMetadata, setCreateMetadata] = useState("{}");
  const [editName, setEditName] = useState("");
  const [editMetadata, setEditMetadata] = useState("{}");
  const [activationRef, setActivationRef] = useState("");
  const [activationConfirmed, setActivationConfirmed] = useState(false);
  const [rotateConfirmed, setRotateConfirmed] = useState(false);
  const [disableConfirmed, setDisableConfirmed] = useState(false);
  const filtered = filterAPIClients(clients, query, stateFilter) ?? [];
  useEffect(() => {
    if (!selectedClient) return;
    setEditName(selectedClient.displayName);
    setEditMetadata("{}");
    setActivationRef("");
    setActivationConfirmed(false);
    setRotateConfirmed(false);
    setDisableConfirmed(false);
  }, [selectedClient?.clientID, selectedClient?.version]);
  const createSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const metadata = parseSafeJSONObjectText(createMetadata);
    if (metadata) onCreate({ clientID: createID.trim(), displayName: createName.trim(), metadata });
  };
  const updateSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedClient) return;
    const metadata = parseSafeJSONObjectText(editMetadata);
    if (metadata) onUpdate({ clientID: selectedClient.clientID, expectedVersion: selectedClient.version, displayName: editName.trim(), metadata });
  };
  return <section className="admin-config-panel" aria-labelledby="admin-config-clients-title">
    <div className="admin-config-panel__heading"><div><p className="admin-config-eyebrow">Credential references only</p><h2 id="admin-config-clients-title">API Clients</h2><p>列表筛选、创建、查看、非 active 编辑、轮换、激活与停用。激活必须手动回填当前 <code>secret_ref</code> 完成自检。</p></div></div>
    <SectionStatus state={state} label="API Clients" />
    <div className="admin-config-toolbar"><label>搜索<input value={query} maxLength={200} onChange={(event) => setQuery(event.currentTarget.value)} /></label><label>状态<select value={stateFilter} onChange={(event) => setStateFilter(event.currentTarget.value as "all" | AdminCredentialState)}><option value="all">全部</option><option value="active">已激活</option><option value="pending_activation">待激活</option><option value="disabled">已停用</option></select></label></div>
    <div className="admin-config-split">
      <div className="admin-config-list" aria-label="API Client 列表">{filtered.length ? filtered.map((client) => <button className={`admin-config-list__item${selectedClient?.clientID === client.clientID ? " is-selected" : ""}`} type="button" key={client.clientID} onClick={() => onSelect(client.clientID)}><span><strong>{client.displayName}</strong><code>{client.clientID}</code></span><span><em>{stateLabels[client.state]}</em><small>v{client.version}</small></span></button>) : <p className="admin-config-empty">没有符合条件的 API Client。</p>}</div>
      <div className="admin-config-detail">
        {selected ? <SectionStatus state={selected} label="API Client 详情" /> : null}
        {selectedClient ? <>
          <div className="admin-config-card"><div className="admin-config-card__title"><h3>{selectedClient.displayName}</h3><span className={`admin-config-badge admin-config-badge--${selectedClient.state}`}>{stateLabels[selectedClient.state]}</span></div><dl className="admin-config-definition"><dt>Client ID</dt><dd><code>{selectedClient.clientID}</code></dd><dt>Secret mask</dt><dd><code>{selectedClient.secretMask}</code></dd><dt>Secret reference</dt><dd><code className="admin-config-code-wrap">{selectedClient.secretRef}</code></dd><dt>版本</dt><dd>v{selectedClient.version}</dd><dt>最近更新</dt><dd>{displayDate(selectedClient.updatedAt)}</dd></dl></div>
          <form className="admin-config-card" onSubmit={updateSubmit}><h3>编辑非 active 配置</h3><p>读取合同不返回 metadata；此处保存的是明确输入的完整 metadata 对象，不猜测或合并未知字段。</p><fieldset disabled={writesDisabled || selectedClient.state === "active"}><label>显示名称<input value={editName} maxLength={200} onChange={(event) => setEditName(event.currentTarget.value)} /></label><label>Metadata JSON<textarea value={editMetadata} rows={7} spellCheck={false} onChange={(event) => setEditMetadata(event.currentTarget.value)} /></label><button type="submit">保存并回读核验</button></fieldset>{selectedClient.state === "active" ? <p className="admin-config-hint">active 客户端必须先停用，才能编辑。</p> : null}</form>
          <div className="admin-config-card"><h3>Secret 轮换</h3><label className="admin-config-check"><input type="checkbox" checked={rotateConfirmed} onChange={(event) => setRotateConfirmed(event.currentTarget.checked)} />我确认轮换只生成新的本地 Secret 引用，并进入待激活状态。</label><button type="button" disabled={writesDisabled || !rotateConfirmed} onClick={() => onRotate(selectedClient)}>轮换 Secret 引用</button></div>
          <div className="admin-config-card admin-config-card--gold"><h3>激活自检</h3><p>复制上方当前 <code>secret_ref</code>，原样回填。前端与服务端都会比对引用；不接受真实 Secret。</p><label>回填 secret_ref<input value={activationRef} autoComplete="off" onChange={(event) => setActivationRef(event.currentTarget.value)} /></label><label className="admin-config-check"><input type="checkbox" checked={activationConfirmed} onChange={(event) => setActivationConfirmed(event.currentTarget.checked)} />我确认已保存引用，并要求执行引用自检。</label><button type="button" disabled={writesDisabled || selectedClient.state === "active" || !activationConfirmed || activationRef !== selectedClient.secretRef} onClick={() => onActivate(selectedClient, activationRef, activationConfirmed)}>自检并激活</button></div>
          <div className="admin-config-card admin-config-card--danger"><h3>停用 Client</h3><label className="admin-config-check"><input type="checkbox" checked={disableConfirmed} onChange={(event) => setDisableConfirmed(event.currentTarget.checked)} />我确认停用该本地 API Client。</label><button type="button" disabled={writesDisabled || selectedClient.state === "disabled" || !disableConfirmed} onClick={() => onDisable(selectedClient)}>停用</button></div>
        </> : <p className="admin-config-empty">从左侧选择一个 Client 查看详情。</p>}
      </div>
    </div>
    <form className="admin-config-card admin-config-create" onSubmit={createSubmit}><h3>新建 API Client</h3><fieldset disabled={writesDisabled}><div className="admin-config-form-grid"><label>Client ID<input value={createID} maxLength={120} placeholder="partner.crm" onChange={(event) => setCreateID(event.currentTarget.value)} /></label><label>显示名称<input value={createName} maxLength={200} onChange={(event) => setCreateName(event.currentTarget.value)} /></label></div><label>Metadata JSON<textarea value={createMetadata} rows={7} spellCheck={false} onChange={(event) => setCreateMetadata(event.currentTarget.value)} /></label><button type="submit">创建待激活 Client</button></fieldset></form>
  </section>;
}

export function DirectAPIKeyPanel({
  state,
  writesDisabled,
  onAction,
}: {
  readonly state: AdminConfigSectionState<DirectAPIKeySnapshot>;
  readonly writesDisabled: boolean;
  readonly onAction: AdminConfigCenterViewProps["onDirectKeyAction"];
}): React.ReactElement {
  const snapshot = sectionValue(state);
  const [action, setAction] = useState<"generate" | "rotate" | "disable">(snapshot?.configured ? "rotate" : "generate");
  const [confirmed, setConfirmed] = useState(false);
  const [confirmationText, setConfirmationText] = useState("");
  useEffect(() => {
    if (snapshot?.configured && action === "generate") setAction("rotate");
    if (!snapshot?.configured && action !== "generate") setAction("generate");
    setConfirmed(false);
    setConfirmationText("");
  }, [snapshot?.configured, snapshot?.version]);
  const phrase = DIRECT_KEY_CONFIRMATION_PHRASES[action];
  const validConfirmation = confirmed && confirmationText === phrase;
  return <section className="admin-config-panel" aria-labelledby="admin-config-direct-title">
    <div className="admin-config-panel__heading"><div><p className="admin-config-eyebrow">Masked projection only</p><h2 id="admin-config-direct-title">Direct API Key</h2><p>只显示掩码、版本与本地状态。生成、轮换、停用均要求复选确认 + 精确确认短语。</p></div></div>
    <SectionStatus state={state} label="Direct API Key" />
    {snapshot ? <div className="admin-config-grid admin-config-grid--two">
      <article className="admin-config-card admin-config-card--key"><h3>当前状态</h3>{snapshot.configured ? <dl className="admin-config-definition"><dt>状态</dt><dd>{stateLabels[snapshot.state ?? "disabled"]}</dd><dt>掩码</dt><dd><code>{snapshot.secretMask}</code></dd><dt>版本</dt><dd>v{snapshot.version}</dd><dt>最近更新</dt><dd>{displayDate(snapshot.updatedAt)}</dd></dl> : <p className="admin-config-empty">尚未生成 Direct API Key。</p>}<p className="admin-config-hint">此投影不包含 Secret 引用或 Secret 值。</p></article>
      <form className="admin-config-card" onSubmit={(event) => { event.preventDefault(); onAction(action, { confirmed, confirmationText }); }}><h3>二次确认操作</h3><fieldset disabled={writesDisabled}><label>操作<select value={action} onChange={(event) => { setAction(event.currentTarget.value as "generate" | "rotate" | "disable"); setConfirmed(false); setConfirmationText(""); }}><option value="generate" disabled={snapshot.configured}>生成</option><option value="rotate" disabled={!snapshot.configured}>轮换</option><option value="disable" disabled={!snapshot.configured || snapshot.state === "disabled"}>停用</option></select></label><label className="admin-config-check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.currentTarget.checked)} />我理解该操作会改变本地凭据状态。</label><label>输入确认短语 <code>{phrase}</code><input value={confirmationText} autoComplete="off" onChange={(event) => setConfirmationText(event.currentTarget.value)} /></label><button type="submit" disabled={!validConfirmation}>提交一次并回读核验</button></fieldset></form>
    </div> : null}
  </section>;
}

function CategoriesPanel({
  state,
  selected,
  check,
  writesDisabled,
  onSelect,
  onSetEnabled,
  onSaveSettings,
  onCheck,
}: {
  readonly state: AdminConfigSectionState<readonly AdminCategory[]>;
  readonly selected?: AdminConfigSectionState<AdminCategory>;
  readonly check?: AdminConfigSectionState<CategoryCheckResult>;
  readonly writesDisabled: boolean;
  readonly onSelect: AdminConfigCenterViewProps["onSelectCategory"];
  readonly onSetEnabled: AdminConfigCenterViewProps["onSetCategoryEnabled"];
  readonly onSaveSettings: AdminConfigCenterViewProps["onSaveCategorySettings"];
  readonly onCheck: AdminConfigCenterViewProps["onCheckCategory"];
}): React.ReactElement {
  const categories = sectionValue(state) ?? [];
  const category = selected ? sectionValue(selected) : undefined;
  const checkValue = check ? sectionValue(check) : undefined;
  const [settingsText, setSettingsText] = useState("{}");
  const [enabledConfirmed, setEnabledConfirmed] = useState(false);
  const [saveConfirmed, setSaveConfirmed] = useState(false);
  useEffect(() => {
    if (!category) return;
    setSettingsText(safeJSONText(category.settings));
    setEnabledConfirmed(false);
    setSaveConfirmed(false);
  }, [category?.key, category?.version]);
  const submitSettings = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!category || !saveConfirmed) return;
    const settings = parseSafeJSONObjectText(settingsText);
    if (settings) onSaveSettings(category, settings);
  };
  return <section className="admin-config-panel" aria-labelledby="admin-config-category-title">
    <div className="admin-config-panel__heading"><div><p className="admin-config-eyebrow">Existing category semantics only</p><h2 id="admin-config-category-title">配置分类</h2><p>读取既有分类，启停、保存已有 settings 对象并执行无外部调用的 local check；不创建新设置语义。</p></div></div>
    <SectionStatus state={state} label="配置分类" />
    <div className="admin-config-split">
      <div className="admin-config-list" aria-label="配置分类列表">{categories.length ? categories.map((item) => <button type="button" className={`admin-config-list__item${category?.key === item.key ? " is-selected" : ""}`} key={item.key} onClick={() => onSelect(item.key)}><span><strong>{item.key}</strong><small>v{item.version}</small></span><em>{item.enabled ? "已启用" : "已停用"}</em></button>) : <p className="admin-config-empty">暂无本地分类记录。</p>}</div>
      <div className="admin-config-detail">{selected ? <SectionStatus state={selected} label="分类详情" /> : null}{category ? <>
        <article className="admin-config-card"><div className="admin-config-card__title"><h3><code>{category.key}</code></h3><span className={`admin-config-badge ${category.enabled ? "admin-config-badge--active" : "admin-config-badge--disabled"}`}>{category.enabled ? "已启用" : "已停用"}</span></div><dl className="admin-config-definition"><dt>版本</dt><dd>v{category.version}</dd><dt>持久化</dt><dd>{category.persisted ? "是" : "尚无本地记录"}</dd><dt>更新人</dt><dd>{category.updatedBy ?? "—"}</dd><dt>更新时间</dt><dd>{displayDate(category.updatedAt)}</dd></dl></article>
        <article className="admin-config-card"><h3>启停</h3><label className="admin-config-check"><input type="checkbox" checked={enabledConfirmed} onChange={(event) => setEnabledConfirmed(event.currentTarget.checked)} />我确认只改变该分类的本地 enabled 状态。</label><button type="button" disabled={writesDisabled || !enabledConfirmed} onClick={() => onSetEnabled(category, !category.enabled)}>{category.enabled ? "停用分类" : "启用分类"}</button></article>
        <form className="admin-config-card" onSubmit={submitSettings}><h3>Settings JSON</h3><p>仅保存当前服务已接受的对象；敏感字段只能是安全引用，不接受真实 Secret。</p><fieldset disabled={writesDisabled}><label>配置对象<textarea value={settingsText} rows={12} spellCheck={false} onChange={(event) => setSettingsText(event.currentTarget.value)} /></label><label className="admin-config-check"><input type="checkbox" checked={saveConfirmed} onChange={(event) => setSaveConfirmed(event.currentTarget.checked)} />我确认完整替换当前 settings 对象。</label><button type="submit" disabled={!saveConfirmed}>保存并回读核验</button></fieldset></form>
        <article className="admin-config-card admin-config-card--gold"><h3>Local check</h3><p>只读取本地分类并返回本地摘要；合同明确 <code>external_calls=false</code>。</p><button type="button" disabled={writesDisabled} onClick={() => onCheck(category)}>执行一次 local check</button>{check ? <SectionStatus state={check} label="Local check" /> : null}{checkValue && checkValue.categoryKey === category.key ? <p className="admin-config-success">检查完成：failed={checkValue.failed}，external_calls=false。</p> : null}</article>
      </> : <p className="admin-config-empty">从左侧选择一个配置分类。</p>}</div>
    </div>
  </section>;
}

export function ReleasePanel({
  state,
  selected,
  comparison,
  writesDisabled,
  onCreate,
  onSelect,
  onValidate,
  onCompare,
  onPublish,
  onRollback,
}: {
  readonly state: AdminConfigSectionState<readonly AdminRelease[]>;
  readonly selected?: AdminConfigSectionState<AdminRelease>;
  readonly comparison?: AdminConfigSectionState<ShadowComparison>;
  readonly writesDisabled: boolean;
  readonly onCreate: AdminConfigCenterViewProps["onCreateRelease"];
  readonly onSelect: AdminConfigCenterViewProps["onSelectRelease"];
  readonly onValidate: AdminConfigCenterViewProps["onValidateRelease"];
  readonly onCompare: AdminConfigCenterViewProps["onLoadShadowComparison"];
  readonly onPublish: AdminConfigCenterViewProps["onPublishRelease"];
  readonly onRollback: AdminConfigCenterViewProps["onRollbackRelease"];
}): React.ReactElement {
  const releases = sectionValue(state) ?? [];
  const release = selected ? sectionValue(selected) : undefined;
  const comparisonValue = comparison ? sectionValue(comparison) : undefined;
  const [changesText, setChangesText] = useState("{\n  \"feature.example\": \"enabled\"\n}");
  const [createConfirmed, setCreateConfirmed] = useState(false);
  const [publishConfirmed, setPublishConfirmed] = useState(false);
  const [rollbackConfirmed, setRollbackConfirmed] = useState(false);
  useEffect(() => {
    setPublishConfirmed(false);
    setRollbackConfirmed(false);
  }, [release?.id, release?.state]);
  const createSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!createConfirmed) return;
    const changes = parseSafeJSONObjectText(changesText, true);
    if (changes) onCreate(changes);
  };
  return <section className="admin-config-panel" aria-labelledby="admin-config-release-title">
    <div className="admin-config-release-boundary" role="note"><strong>{ADMIN_CONFIG_LOCAL_RELEASE_NOTICE}</strong><span>validate / publish / rollback 只改变本地配置 Release 记录，不执行生产部署、Provider 或外部调用。</span></div>
    <div className="admin-config-panel__heading"><div><p className="admin-config-eyebrow">Local release lifecycle</p><h2 id="admin-config-release-title">Release 历史 / 详情</h2></div></div>
    <SectionStatus state={state} label="Release 历史" />
    <div className="admin-config-split">
      <div className="admin-config-list" aria-label="Release 历史">{releases.length ? releases.map((item) => <button type="button" className={`admin-config-list__item${release?.id === item.id ? " is-selected" : ""}`} key={item.id} onClick={() => onSelect(item.id)}><span><strong>Release #{item.id}</strong><small>{displayDate(item.createdAt)}</small></span><em>{releaseStateLabels[item.state]}</em></button>) : <p className="admin-config-empty">暂无本地 Release。</p>}</div>
      <div className="admin-config-detail">{selected ? <SectionStatus state={selected} label="Release 详情" /> : null}{release ? <>
        <article className="admin-config-card admin-config-card--gold"><div className="admin-config-card__title"><h3>Release #{release.id}</h3><span className={`admin-config-badge admin-config-badge--release-${release.state}`}>{releaseStateLabels[release.state]}</span></div><dl className="admin-config-definition"><dt>Checksum</dt><dd><code className="admin-config-code-wrap">{release.checksum}</code></dd><dt>创建人</dt><dd>{release.createdBy}</dd><dt>创建时间</dt><dd>{displayDate(release.createdAt)}</dd><dt>校验时间</dt><dd>{displayDate(release.validatedAt)}</dd><dt>本地发布时间</dt><dd>{displayDate(release.publishedAt)}</dd><dt>回滚来源</dt><dd>{release.rollbackOfReleaseID ? `#${release.rollbackOfReleaseID}` : "—"}</dd></dl><h4>Changes</h4><pre>{safeJSONText(release.changes)}</pre></article>
        <article className="admin-config-card"><h3>校验与 Shadow compare</h3><button type="button" disabled={writesDisabled || release.state !== "draft"} onClick={() => onValidate(release)}>Validate 本地草稿</button><button type="button" className="admin-config-button--secondary" onClick={() => onCompare(release)}>读取 Shadow compare</button>{comparison ? <SectionStatus state={comparison} label="Shadow compare" /> : null}{comparisonValue?.releaseID === release.id ? <p className={comparisonValue.available ? "admin-config-success" : "admin-config-hint"}>comparison.available={String(comparisonValue.available)}；external_calls=false。</p> : null}</article>
        <article className="admin-config-card"><h3>Local publish</h3><label className="admin-config-check"><input type="checkbox" checked={publishConfirmed} onChange={(event) => setPublishConfirmed(event.currentTarget.checked)} />我确认这只是本地配置状态，不等于部署。</label><button type="button" disabled={writesDisabled || release.state !== "validated" || !publishConfirmed} onClick={() => onPublish(release)}>标记为本地已发布</button></article>
        <article className="admin-config-card admin-config-card--danger"><h3>Local rollback</h3><p>回滚会创建新的 <code>rolled_back</code> 本地记录；不会回退部署或调用外部系统。</p><label className="admin-config-check"><input type="checkbox" checked={rollbackConfirmed} onChange={(event) => setRollbackConfirmed(event.currentTarget.checked)} />我确认创建本地回滚记录。</label><button type="button" disabled={writesDisabled || release.state !== "published" || !rollbackConfirmed} onClick={() => onRollback(release)}>创建本地回滚记录</button></article>
      </> : <p className="admin-config-empty">从左侧选择一条 Release。</p>}</div>
    </div>
    <form className="admin-config-card admin-config-create" onSubmit={createSubmit}><h3>创建 Draft</h3><p>Changes 必须是非空安全 JSON 对象。含 secret/password/webhook/token 的字段只能保存有效引用。</p><fieldset disabled={writesDisabled}><label>Changes JSON<textarea value={changesText} rows={10} spellCheck={false} onChange={(event) => setChangesText(event.currentTarget.value)} /></label><label className="admin-config-check"><input type="checkbox" checked={createConfirmed} onChange={(event) => setCreateConfirmed(event.currentTarget.checked)} />我确认只创建本地 Draft。</label><button type="submit" disabled={!createConfirmed}>创建本地 Draft</button></fieldset></form>
  </section>;
}

export function AdminConfigCenterView(props: AdminConfigCenterViewProps): React.ReactElement {
  const writesLocked = props.writeState.kind === "saving" || props.writeState.kind === "unknown" || props.writeState.kind === "conflict";
  const writesDisabled = !props.writesAvailable || writesLocked;
  return <main className="admin-config-center" aria-labelledby="admin-config-title">
    <header className="admin-config-header"><div><p className="admin-config-eyebrow">AI-CRM-v2 · AdminOps</p><h1 id="admin-config-title">Admin 配置控制中心</h1><p>凭据、配置分类与本地 Release 生命周期的一体化工作台。所有数据均为本地配置事实。</p></div><button type="button" className="admin-config-button--secondary" onClick={props.onRefresh}>重新读取本地状态</button></header>
    <nav className="admin-config-tabs" aria-label="配置控制中心子区">{(Object.keys(sectionLabels) as AdminConfigSection[]).map((section) => <button type="button" key={section} aria-current={props.activeSection === section ? "page" : undefined} className={props.activeSection === section ? "is-active" : ""} onClick={() => props.onSectionChange(section)}>{sectionLabels[section]}</button>)}</nav>
    {!props.writesAvailable ? <p className="admin-config-alert" role="alert">中央壳层尚未提供当前会话的路由绑定 Action Token；所有写操作保持关闭。</p> : null}
    <WriteStatus state={props.writeState} />
    {props.activeSection === "overview" ? <OverviewPanel clients={props.clients} directKey={props.directKey} categories={props.categories} releases={props.releases} /> : null}
    {props.activeSection === "clients" ? <APIClientsPanel state={props.clients} selected={props.selectedClient} writesDisabled={writesDisabled} onSelect={props.onSelectClient} onCreate={props.onCreateClient} onUpdate={props.onUpdateClient} onRotate={props.onRotateClient} onActivate={props.onActivateClient} onDisable={props.onDisableClient} /> : null}
    {props.activeSection === "direct-key" ? <DirectAPIKeyPanel state={props.directKey} writesDisabled={writesDisabled} onAction={props.onDirectKeyAction} /> : null}
    {props.activeSection === "categories" ? <CategoriesPanel state={props.categories} selected={props.selectedCategory} check={props.categoryCheck} writesDisabled={writesDisabled} onSelect={props.onSelectCategory} onSetEnabled={props.onSetCategoryEnabled} onSaveSettings={props.onSaveCategorySettings} onCheck={props.onCheckCategory} /> : null}
    {props.activeSection === "releases" ? <ReleasePanel state={props.releases} selected={props.selectedRelease} comparison={props.shadowComparison} writesDisabled={writesDisabled} onCreate={props.onCreateRelease} onSelect={props.onSelectRelease} onValidate={props.onValidateRelease} onCompare={props.onLoadShadowComparison} onPublish={props.onPublishRelease} onRollback={props.onRollbackRelease} /> : null}
  </main>;
}

function fromReadResult<T>(result: AdminConfigReadResult<T>, previous?: T): AdminConfigSectionState<T> {
  return result.status === "loaded" ? { kind: "ready", value: result.value } : { kind: "error", failure: result.status, ...(previous === undefined ? {} : { previous }) };
}

function runtimeCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

// eslint-disable-next-line no-unused-vars -- named parameter documents the key selector contract.
function replaceByKey<T>(items: readonly T[], item: T, key: (value: T) => string | number): readonly T[] {
  const wanted = key(item);
  const index = items.findIndex((candidate) => key(candidate) === wanted);
  if (index < 0) return [item, ...items];
  return items.map((candidate, candidateIndex) => candidateIndex === index ? item : candidate);
}

interface AuthorizedAdminConfigCenterPageProps {
  readonly transport: AdminConfigTransport;
  // eslint-disable-next-line no-unused-vars -- named parameters document the action-token resolver contract.
  readonly actionTokenFor?: (method: string, pattern: string) => string | undefined;
  readonly readCookie: () => string;
  readonly requestIDFactory: () => string | undefined;
  readonly onUnauthenticated?: () => void;
}

function AuthorizedAdminConfigCenterPage({
  transport,
  actionTokenFor,
  readCookie,
  requestIDFactory,
  onUnauthenticated,
}: AuthorizedAdminConfigCenterPageProps): React.ReactElement {
  const [activeSection, setActiveSection] = useState<AdminConfigSection>("overview");
  const [clients, setClients] = useState<AdminConfigSectionState<readonly AdminCredential[]>>({ kind: "loading" });
  const [directKey, setDirectKey] = useState<AdminConfigSectionState<DirectAPIKeySnapshot>>({ kind: "loading" });
  const [categories, setCategories] = useState<AdminConfigSectionState<readonly AdminCategory[]>>({ kind: "loading" });
  const [releases, setReleases] = useState<AdminConfigSectionState<readonly AdminRelease[]>>({ kind: "loading" });
  const [selectedClient, setSelectedClient] = useState<AdminConfigSectionState<AdminCredential>>();
  const [selectedCategory, setSelectedCategory] = useState<AdminConfigSectionState<AdminCategory>>();
  const [selectedRelease, setSelectedRelease] = useState<AdminConfigSectionState<AdminRelease>>();
  const [shadowComparison, setShadowComparison] = useState<AdminConfigSectionState<ShadowComparison>>();
  const [categoryCheck, setCategoryCheck] = useState<AdminConfigSectionState<CategoryCheckResult>>();
  const [writeState, setWriteState] = useState<AdminConfigWriteViewState>({ kind: "idle" });
  const overviewGeneration = useRef(new AdminConfigResponseEpoch());
  const clientGeneration = useRef(new AdminConfigResponseEpoch());
  const categoryGeneration = useRef(new AdminConfigResponseEpoch());
  const releaseGeneration = useRef(new AdminConfigResponseEpoch());
  const comparisonGeneration = useRef(new AdminConfigResponseEpoch());
  const categoryCheckGeneration = useRef(new AdminConfigResponseEpoch());
  const selectedClientID = useRef<string>();
  const selectedCategoryKey = useRef<string>();
  const selectedReleaseID = useRef<number>();
  const writeInFlight = useRef<symbol>();

  const security = (): AdminConfigSecurity => ({
    csrfToken: () => {
      try {
        return readCSRFCookie(readCookie());
      } catch {
        return undefined;
      }
    },
    actionTokenFor: (method, pattern) => actionTokenFor?.(method, pattern),
    requestID: requestIDFactory,
  });

  const notifyAuth = (failure: AdminConfigReadFailure | AdminConfigWriteFailure) => {
    if (failure === "unauthenticated") onUnauthenticated?.();
  };

  const refreshAll = (manual: boolean) => {
    const generation = overviewGeneration.current.begin();
    clientGeneration.current.invalidate();
    categoryGeneration.current.invalidate();
    releaseGeneration.current.invalidate();
    comparisonGeneration.current.invalidate();
    categoryCheckGeneration.current.invalidate();
    setShadowComparison(undefined);
    setCategoryCheck(undefined);
    const previousClients = sectionValue(clients);
    const previousKey = sectionValue(directKey);
    const previousCategories = sectionValue(categories);
    const previousReleases = sectionValue(releases);
    setClients({ kind: "loading", ...(previousClients === undefined ? {} : { previous: previousClients }) });
    setDirectKey({ kind: "loading", ...(previousKey === undefined ? {} : { previous: previousKey }) });
    setCategories({ kind: "loading", ...(previousCategories === undefined ? {} : { previous: previousCategories }) });
    setReleases({ kind: "loading", ...(previousReleases === undefined ? {} : { previous: previousReleases }) });
    void loadAdminConfigOverview(transport).then((overview) => {
      if (!overviewGeneration.current.accepts(generation)) return;
      setClients(fromReadResult(overview.clients, previousClients));
      setDirectKey(fromReadResult(overview.directKey, previousKey));
      setCategories(fromReadResult(overview.categories, previousCategories));
      setReleases(fromReadResult(overview.releases, previousReleases));
      if (overview.clients.status === "loaded" && selectedClientID.current !== undefined) {
        const selected = overview.clients.value.find((item) => item.clientID === selectedClientID.current);
        setSelectedClient(selected ? { kind: "ready", value: selected } : { kind: "error", failure: "not_found" });
      }
      if (overview.categories.status === "loaded" && selectedCategoryKey.current !== undefined) {
        const selected = overview.categories.value.find((item) => item.key === selectedCategoryKey.current);
        setSelectedCategory(selected ? { kind: "ready", value: selected } : { kind: "error", failure: "not_found" });
      }
      if (overview.releases.status === "loaded" && selectedReleaseID.current !== undefined) {
        const selected = overview.releases.value.find((item) => item.id === selectedReleaseID.current);
        setSelectedRelease(selected ? { kind: "ready", value: selected } : { kind: "error", failure: "not_found" });
      }
      for (const result of [overview.clients, overview.directKey, overview.categories, overview.releases] as const) {
        if (result.status !== "loaded") notifyAuth(result.status);
      }
      if (manual && overview.clients.status === "loaded" && overview.directKey.status === "loaded" && overview.categories.status === "loaded" && overview.releases.status === "loaded") setWriteState({ kind: "idle" });
    });
  };

  useEffect(() => {
    refreshAll(false);
    return () => {
      overviewGeneration.current.invalidate();
      clientGeneration.current.invalidate();
      categoryGeneration.current.invalidate();
      releaseGeneration.current.invalidate();
      comparisonGeneration.current.invalidate();
      categoryCheckGeneration.current.invalidate();
      writeInFlight.current = undefined;
    };
  }, [transport]);

  const updateClientCollections = (client: AdminCredential) => {
    clientGeneration.current.invalidate();
    selectedClientID.current = client.clientID;
    setClients((current) => {
      const values = sectionValue(current) ?? [];
      return { kind: "ready", value: replaceByKey(values, client, (item) => item.clientID) };
    });
    setSelectedClient({ kind: "ready", value: client });
  };

  const updateCategoryCollections = (category: AdminCategory) => {
    categoryGeneration.current.invalidate();
    selectedCategoryKey.current = category.key;
    categoryCheckGeneration.current.invalidate();
    setCategories((current) => {
      const values = sectionValue(current) ?? [];
      const next = replaceByKey(values, category, (item) => item.key).slice().sort((left, right) => left.key < right.key ? -1 : left.key > right.key ? 1 : 0);
      return { kind: "ready", value: next };
    });
    setSelectedCategory({ kind: "ready", value: category });
  };

  const updateReleaseCollections = (release: AdminRelease) => {
    releaseGeneration.current.invalidate();
    selectedReleaseID.current = release.id;
    comparisonGeneration.current.invalidate();
    setReleases((current) => {
      const values = sectionValue(current) ?? [];
      const next = replaceByKey(values, release, (item) => item.id).slice().sort((left, right) => Date.parse(right.createdAt) - Date.parse(left.createdAt) || right.id - left.id);
      return { kind: "ready", value: next };
    });
    setSelectedRelease({ kind: "ready", value: release });
    setShadowComparison(undefined);
  };

  // eslint-disable-next-line no-unused-vars -- named parameter documents the successful write callback contract.
  const runWrite = <T,>(label: string, operation: () => Promise<{ readonly status: "applied"; readonly value: T } | { readonly status: AdminConfigWriteFailure }>, apply: (value: T) => void) => {
    if (writeInFlight.current || writeState.kind === "unknown" || writeState.kind === "conflict") return;
    const token = Symbol(label);
    writeInFlight.current = token;
    setWriteState({ kind: "saving", label });
    void Promise.resolve().then(operation).then((result) => {
      if (writeInFlight.current !== token) return;
      if (result.status === "applied") {
        overviewGeneration.current.invalidate();
        apply(result.value);
        setWriteState({ kind: "saved", label });
        return;
      }
      notifyAuth(result.status);
      if (result.status === "unknown") setWriteState({ kind: "unknown" });
      else if (result.status === "conflict") setWriteState({ kind: "conflict" });
      else setWriteState({ kind: "error", failure: result.status });
    }).catch(() => {
      if (writeInFlight.current === token) setWriteState({ kind: "unknown" });
    }).finally(() => {
      if (writeInFlight.current === token) writeInFlight.current = undefined;
    });
  };

  const selectClient = (clientID: string) => {
    selectedClientID.current = clientID;
    const generation = clientGeneration.current.begin();
    const previous = selectedClient && sectionValue(selectedClient)?.clientID === clientID ? sectionValue(selectedClient) : undefined;
    setSelectedClient({ kind: "loading", ...(previous === undefined ? {} : { previous }) });
    void loadAPIClient(transport, clientID).then((result) => {
      if (!clientGeneration.current.accepts(generation)) return;
      setSelectedClient(fromReadResult(result, previous));
      if (result.status !== "loaded") notifyAuth(result.status);
    });
  };

  const selectCategory = (key: string) => {
    selectedCategoryKey.current = key;
    const generation = categoryGeneration.current.begin();
    const previous = selectedCategory && sectionValue(selectedCategory)?.key === key ? sectionValue(selectedCategory) : undefined;
    setSelectedCategory({ kind: "loading", ...(previous === undefined ? {} : { previous }) });
    setCategoryCheck(undefined);
    void loadCategory(transport, key).then((result) => {
      if (!categoryGeneration.current.accepts(generation)) return;
      setSelectedCategory(fromReadResult(result, previous));
      if (result.status !== "loaded") notifyAuth(result.status);
    });
  };

  const selectRelease = (releaseID: number) => {
    selectedReleaseID.current = releaseID;
    const generation = releaseGeneration.current.begin();
    const previous = selectedRelease && sectionValue(selectedRelease)?.id === releaseID ? sectionValue(selectedRelease) : undefined;
    setSelectedRelease({ kind: "loading", ...(previous === undefined ? {} : { previous }) });
    setShadowComparison(undefined);
    void loadRelease(transport, releaseID).then((result) => {
      if (!releaseGeneration.current.accepts(generation)) return;
      setSelectedRelease(fromReadResult(result, previous));
      if (result.status !== "loaded") notifyAuth(result.status);
    });
  };

  const loadComparison = (release: AdminRelease) => {
    const generation = comparisonGeneration.current.begin();
    const previous = shadowComparison && sectionValue(shadowComparison)?.releaseID === release.id ? sectionValue(shadowComparison) : undefined;
    setShadowComparison({ kind: "loading", ...(previous === undefined ? {} : { previous }) });
    void loadShadowComparison(transport, release.id).then((result) => {
      if (!comparisonGeneration.current.accepts(generation)) return;
      setShadowComparison(fromReadResult(result, previous));
      if (result.status !== "loaded") notifyAuth(result.status);
    });
  };

  return <AdminConfigCenterView
    activeSection={activeSection}
    clients={clients}
    directKey={directKey}
    categories={categories}
    releases={releases}
    selectedClient={selectedClient}
    selectedCategory={selectedCategory}
    selectedRelease={selectedRelease}
    shadowComparison={shadowComparison}
    categoryCheck={categoryCheck}
    writeState={writeState}
    writesAvailable={hasCompleteAdminConfigActionTokens(actionTokenFor)}
    onSectionChange={setActiveSection}
    onRefresh={() => refreshAll(true)}
    onSelectClient={selectClient}
    onCreateClient={(input) => runWrite("创建 API Client", () => createAPIClient(transport, security(), input), updateClientCollections)}
    onUpdateClient={(input) => runWrite("保存 API Client", () => updateAPIClient(transport, security(), input), updateClientCollections)}
    onRotateClient={(client) => runWrite("轮换 API Client Secret 引用", () => rotateAPIClientSecret(transport, security(), { clientID: client.clientID, expectedVersion: client.version }), updateClientCollections)}
    onActivateClient={(client, secretRef, copiedConfirmed) => runWrite("激活 API Client", () => activateAPIClient(transport, security(), { clientID: client.clientID, expectedVersion: client.version, secretRef, copiedConfirmed }), updateClientCollections)}
    onDisableClient={(client) => runWrite("停用 API Client", () => disableAPIClient(transport, security(), { clientID: client.clientID, expectedVersion: client.version }), updateClientCollections)}
    onDirectKeyAction={(action, confirmation) => {
      const current = sectionValue(directKey);
      if (action === "generate") runWrite("生成 Direct API Key", () => generateDirectAPIKey(transport, security(), confirmation), (value) => setDirectKey({ kind: "ready", value }));
      else if (current) runWrite(action === "rotate" ? "轮换 Direct API Key" : "停用 Direct API Key", () => action === "rotate" ? rotateDirectAPIKey(transport, security(), current, confirmation) : disableDirectAPIKey(transport, security(), current, confirmation), (value) => setDirectKey({ kind: "ready", value }));
    }}
    onSelectCategory={selectCategory}
    onSetCategoryEnabled={(category, enabled) => runWrite(enabled ? "启用配置分类" : "停用配置分类", () => setCategoryEnabled(transport, security(), { key: category.key, expectedVersion: category.version, enabled }), updateCategoryCollections)}
    onSaveCategorySettings={(category, settings) => runWrite("保存配置分类 settings", () => saveCategorySettings(transport, security(), { key: category.key, expectedVersion: category.version, settings }), updateCategoryCollections)}
    onCheckCategory={(category) => {
      if (writeInFlight.current || writeState.kind === "unknown" || writeState.kind === "conflict") return;
      const previous = categoryCheck && sectionValue(categoryCheck)?.categoryKey === category.key ? sectionValue(categoryCheck) : undefined;
      const generation = categoryCheckGeneration.current.begin();
      setCategoryCheck({ kind: "loading", ...(previous === undefined ? {} : { previous }) });
      void checkCategoryLocally(transport, security(), category.key).then((result) => {
        if (!categoryCheckGeneration.current.accepts(generation)) return;
        if (result.status === "applied") setCategoryCheck({ kind: "ready", value: result.value });
        else {
          notifyAuth(result.status);
          setCategoryCheck({ kind: "error", failure: result.status === "unknown" || result.status === "conflict" ? "unavailable" : result.status, ...(previous === undefined ? {} : { previous }) });
        }
      }).catch(() => {
        if (categoryCheckGeneration.current.accepts(generation)) setCategoryCheck({ kind: "error", failure: "unavailable", ...(previous === undefined ? {} : { previous }) });
      });
    }}
    onCreateRelease={(changes) => runWrite("创建本地 Release Draft", () => createReleaseDraft(transport, security(), changes), updateReleaseCollections)}
    onSelectRelease={selectRelease}
    onValidateRelease={(release) => runWrite("Validate 本地 Release", () => validateReleaseLocally(transport, security(), { releaseID: release.id, expectedChecksum: release.checksum }), updateReleaseCollections)}
    onLoadShadowComparison={loadComparison}
    onPublishRelease={(release) => runWrite("标记 Release 为本地已发布", () => publishReleaseLocally(transport, security(), { releaseID: release.id, expectedChecksum: release.checksum }), updateReleaseCollections)}
    onRollbackRelease={(release) => runWrite("创建本地回滚记录", () => rollbackReleaseLocally(transport, security(), { releaseID: release.id, expectedChecksum: release.checksum }), updateReleaseCollections)}
  />;
}

export function AdminConfigCenterPage({
  role,
  transport = generatedAdminConfigTransport,
  actionTokenFor,
  readCookie = runtimeCookie,
  requestIDFactory = newAdminConfigRequestID,
  onUnauthenticated,
}: {
  readonly role: AdminConfigRole;
  readonly transport?: AdminConfigTransport;
  // eslint-disable-next-line no-unused-vars -- named parameters document the action-token resolver contract.
  readonly actionTokenFor?: (method: string, pattern: string) => string | undefined;
  readonly readCookie?: () => string;
  readonly requestIDFactory?: () => string | undefined;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  if (!canManageAdminConfig(role)) return <main className="admin-config-center" aria-labelledby="admin-config-title"><h1 id="admin-config-title">Admin 配置控制中心</h1><p className="admin-config-alert" role="alert">当前账号没有 Admin 配置管理权限。</p></main>;
  return <AuthorizedAdminConfigCenterPage transport={transport} actionTokenFor={actionTokenFor} readCookie={readCookie} requestIDFactory={requestIDFactory} onUnauthenticated={onUnauthenticated} />;
}
