import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { readCSRFCookie } from "./auth";
import {
  filterChannels,
  generatedChannelsTransport,
  paginateChannels,
  loadChannelDetail,
  loadChannels,
  newChannelConfigurationIdempotencyKey,
  newChannelStatusIdempotencyKey,
  saveChannelConfiguration,
  updateChannelStatus,
  type ChannelConfigurationInput,
  type ChannelConfigurationResult,
  type ChannelListItem,
  type ChannelDetail,
  type ChannelDetailResult,
  type ChannelListResult,
  type ChannelStatus,
  type ChannelStatusUpdateResult,
  type ChannelsFailure,
  type ChannelsRole,
  type ChannelsTransport,
  type ChannelStatusFilter,
} from "./channels";

const messages: Record<ChannelsFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有渠道列表访问权限。",
  unavailable: "本地渠道列表暂不可用。",
  invalid: "渠道列表响应不符合已冻结合同。",
};

export type ChannelsViewState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly items: readonly ChannelListItem[] }
  | { readonly kind: "error"; readonly failure: ChannelsFailure };
export type ChannelDetailState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly item: ChannelListItem; readonly previous?: ChannelDetail }
  | { readonly kind: "ready"; readonly item: ChannelListItem; readonly detail: ChannelDetail }
  | { readonly kind: "error"; readonly item: ChannelListItem; readonly failure: ChannelsFailure; readonly previous?: ChannelDetail };
type ChannelEditorState =
  | { readonly kind: "closed" }
  | { readonly kind: "create" }
  | { readonly kind: "edit"; readonly detail: ChannelDetail };

export interface ChannelStatusUpdateInput {
  readonly channelID: number;
  readonly status: ChannelStatus;
  readonly idempotencySource?: { readonly randomUUID: () => string };
  readonly readCookie: () => string;
  readonly transport: ChannelsTransport;
}

export interface ChannelConfigurationSaveInput {
  readonly operation: "create" | "update";
  readonly channelID?: number;
  readonly input: ChannelConfigurationInput;
  readonly idempotencySource?: { readonly randomUUID: () => string };
  readonly readCookie: () => string;
  readonly transport: ChannelsTransport;
}

export async function performChannelConfigurationSave({
  operation,
  channelID,
  input,
  idempotencySource,
  readCookie,
  transport,
}: ChannelConfigurationSaveInput): Promise<ChannelConfigurationResult> {
  let csrfToken: string | undefined;
  try {
    csrfToken = readCSRFCookie(readCookie());
  } catch {
    csrfToken = undefined;
  }
  if (!csrfToken) return { status: "forbidden" };
  const key = newChannelConfigurationIdempotencyKey(operation, idempotencySource);
  if (!key) return { status: "unknown" };
  return saveChannelConfiguration(transport, operation, input, channelID, csrfToken, key);
}

export async function performChannelStatusUpdate({
  channelID,
  status,
  idempotencySource,
  readCookie,
  transport,
}: ChannelStatusUpdateInput): Promise<ChannelStatusUpdateResult> {
  let csrfToken: string | undefined;
  try {
    csrfToken = readCSRFCookie(readCookie());
  } catch {
    csrfToken = undefined;
  }
  if (!csrfToken) return { status: "forbidden" };
  const idempotencyKey = newChannelStatusIdempotencyKey(idempotencySource);
  if (!idempotencyKey) return { status: "unknown" };
  return updateChannelStatus(
    transport,
    channelID,
    status,
    csrfToken,
    idempotencyKey,
  );
}

export function startChannelStatusUpdate(
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

export function ChannelsPage({
  role,
  transport = generatedChannelsTransport,
  readCookie = runtimeCookieHeader,
  onUnauthenticated,
}: {
  readonly role: ChannelsRole;
  readonly transport?: ChannelsTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<ChannelsViewState>({ kind: "loading" });
  const [busy, setBusy] = useState(false);
  const [writeLocked, setWriteLocked] = useState(false);
  const [notice, setNotice] = useState<string>();
  const [detail, setDetail] = useState<ChannelDetailState>({ kind: "idle" });
  const [editor, setEditor] = useState<ChannelEditorState>({ kind: "closed" });
  const loadGeneration = useRef(0);
  const updateInFlight = useRef(false);
  const detailGeneration = useRef(0);
  const detailInflight = useRef(new Set<number>());

  useEffect(() => {
    if (!canAccess) return undefined;
    let active = true;
    const generation = ++loadGeneration.current;
    void loadChannels(transport).then((result: ChannelListResult) => {
      if (!active || generation !== loadGeneration.current) return;
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

  const onStatusChange = useCallback(
    async (item: ChannelListItem, status: ChannelStatus) => {
      if (writeLocked) return;
      const operation = startChannelStatusUpdate(updateInFlight, async () => {
        ++loadGeneration.current;
        setBusy(true);
        try {
          const result = await performChannelStatusUpdate({
            channelID: item.id,
            status,
            readCookie,
            transport,
          });
          if (result.status === "confirmed") {
            setState({ kind: "ready", items: result.items });
            setNotice("本地渠道状态已更新。");
          } else {
            if (result.status === "unauthenticated") onUnauthenticated?.();
            if (result.status === "unknown") setWriteLocked(true);
            setNotice(
              result.status === "unknown"
                ? "更新结果未知，请刷新确认。"
                : messages[result.status],
            );
          }
        } finally {
          setBusy(false);
        }
      });
      if (operation) await operation;
    },
    [onUnauthenticated, readCookie, transport, writeLocked],
  );
  const onLoadDetail = useCallback((item: ChannelListItem) => {
    if (detailInflight.current.has(item.id)) return;
    detailInflight.current.add(item.id);
    const generation = ++detailGeneration.current;
    const previous = detail.kind !== "idle" && detail.item.id === item.id
      ? detail.kind === "ready" ? detail.detail : detail.previous
      : undefined;
    setDetail({ kind: "loading", item, previous });
    void loadChannelDetail(transport, item).then((result: ChannelDetailResult) => {
      if (generation !== detailGeneration.current) return;
      if (result.status === "loaded") { setDetail({ kind: "ready", item, detail: result.detail }); return; }
      if (result.status === "unauthenticated") onUnauthenticated?.();
      setDetail({ kind: "error", item, failure: result.status, previous });
    }).finally(() => detailInflight.current.delete(item.id));
  }, [detail, onUnauthenticated, transport]);

  const onSaveConfiguration = useCallback(async (input: ChannelConfigurationInput) => {
    if (writeLocked) return;
    const operation = startChannelStatusUpdate(updateInFlight, async () => {
      setBusy(true);
      try {
        const result = await performChannelConfigurationSave({
          operation: editor.kind === "create" ? "create" : "update",
          ...(editor.kind === "edit" ? { channelID: editor.detail.item.id } : {}),
          input,
          readCookie,
          transport,
        });
        if (result.status === "confirmed") {
          setState({ kind: "ready", items: result.items });
          setDetail({ kind: "ready", item: result.detail.item, detail: result.detail });
          setEditor({ kind: "closed" });
          setNotice(editor.kind === "create" ? "本地渠道已创建，仍未发布到企微。" : "本地渠道配置已更新，仍未发布到企微。");
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        if (result.status === "unknown") setWriteLocked(true);
        setNotice(result.status === "unknown" ? "结果未知，请刷新确认；本页写操作已锁定。" : messages[result.status]);
      } finally {
        setBusy(false);
      }
    });
    if (operation) await operation;
  }, [editor, onUnauthenticated, readCookie, transport, writeLocked]);

  const onStartCreate = useCallback(() => {
    if (!busy && !writeLocked) setEditor({ kind: "create" });
  }, [busy, writeLocked]);
  const onStartEdit = useCallback(() => {
    if (!busy && !writeLocked && detail.kind === "ready") setEditor({ kind: "edit", detail: detail.detail });
  }, [busy, detail, writeLocked]);

  return (
    <ChannelsView
      busy={busy}
      editor={editor}
      notice={notice}
      detail={detail}
      onCancelEdit={() => setEditor({ kind: "closed" })}
      onCreate={onStartCreate}
      onEdit={onStartEdit}
      onLoadDetail={onLoadDetail}
      onSaveConfiguration={onSaveConfiguration}
      onStatusChange={onStatusChange}
      role={role}
      state={state}
      writeLocked={writeLocked}
    />
  );
}

export function ChannelsView({
  busy = false,
  detail = { kind: "idle" },
  editor = { kind: "closed" },
  notice,
  onCancelEdit = noopEdit,
  onCreate = noopCreate,
  onEdit = noopEdit,
  onLoadDetail = noopDetail,
  onSaveConfiguration = noopSaveConfiguration,
  onStatusChange = noopStatusChange,
  role,
  state,
  writeLocked = false,
}: {
  readonly busy?: boolean;
  readonly detail?: ChannelDetailState;
  readonly editor?: ChannelEditorState;
  readonly notice?: string;
  readonly onCancelEdit?: () => void;
  readonly onCreate?: () => void;
  readonly onEdit?: () => void;
  readonly onLoadDetail?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    item: ChannelListItem,
  ) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onSaveConfiguration?: (input: ChannelConfigurationInput) => void;
  readonly onStatusChange?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
    item: ChannelListItem,
    // eslint-disable-next-line no-unused-vars -- named callback parameters are required by TS function-type syntax.
    status: ChannelStatus,
  ) => void;
  readonly role: ChannelsRole;
  readonly state: ChannelsViewState;
  readonly writeLocked?: boolean;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState<ChannelStatusFilter>("all");
  const [page, setPage] = useState(1);
  const filteredItems = useMemo(
    () =>
      state.kind === "ready"
        ? filterChannels(state.items, keyword, status)
        : [],
    [keyword, state, status],
  );
  const pagination = useMemo(
    () => paginateChannels(filteredItems, page),
    [filteredItems, page],
  );
  const items = pagination.items;

  if (!canAccess)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">渠道列表</h1>
        <p role="alert">当前账号没有渠道列表访问权限。</p>
      </section>
    );
  if (state.kind === "loading")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">渠道列表</h1>
        <p>正在读取本地渠道列表。</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">渠道列表</h1>
        <p role="alert">{messages[state.failure]}</p>
      </section>
    );

  return (
    <section className="route-card" aria-labelledby="app-title" data-provider-execution-eligible="false">
      <p className="route-card__eyebrow">本地渠道工作流</p>
      <h1 id="app-title">渠道列表</h1>
      <p>渠道配置仅保存在本地：企微二维码、获客链接、回调、发布和任何外部调用均未执行（provider=false）。provider_execution_eligible=false。</p>
      {notice ? <p role="status">{notice}</p> : null}
      {writeLocked ? <p role="alert">写入结果未知，请刷新确认；本页写操作已锁定。</p> : null}
      <ChannelDetailPanel state={detail} onEdit={onEdit} disabled={busy || writeLocked} />
      {editor.kind !== "closed" ? <ChannelConfigurationForm
        key={editor.kind === "edit" ? `edit:${editor.detail.item.id}` : "create"}
        busy={busy || writeLocked}
        existing={editor.kind === "edit" ? editor.detail : undefined}
        onCancel={onCancelEdit}
        onSave={onSaveConfiguration}
      /> : null}
      <p><button type="button" disabled={busy || writeLocked} onClick={onCreate}>新建本地渠道</button></p>
      <p>
        <label>
          搜索渠道名称或编码
          <input
            disabled={busy || writeLocked}
            type="search"
            value={keyword}
            onChange={(event) => { setKeyword(event.currentTarget.value); setPage(1); }}
          />
        </label>
      </p>
      <p>
        <label>
          渠道状态
          <select
            disabled={busy || writeLocked}
            value={status}
            onChange={(event) => {
              setStatus(event.currentTarget.value as ChannelStatusFilter);
              setPage(1);
            }}
          >
            <option value="all">全部</option>
            <option value="active">active</option>
            <option value="inactive">inactive</option>
            <option value="archived">archived</option>
          </select>
        </label>
      </p>
      {items.length === 0 ? (
        <p>
          {state.items.length === 0 ? "当前没有本地渠道。" : "没有匹配的渠道。"}
        </p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>渠道 ID</th>
              <th>名称</th>
              <th>编码</th>
              <th>状态</th>
              <th>本地分配人数</th>
              <th>本地进入人数</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>本地配置</th>
              <th>更新本地状态</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>{item.id}</td>
                <td>{item.name}</td>
                <td>{item.code}</td>
                <td>{item.status}</td>
                <td>{item.assigneeCount}</td>
                <td>{item.contactCount}</td>
                <td>{item.createdAt}</td>
                <td>{item.updatedAt}</td>
                <td><button type="button" disabled={busy || writeLocked || (detail.kind === "loading" && detail.item.id === item.id)} onClick={() => onLoadDetail(item)}>查看本地配置</button></td>
                <td>
                  <select
                    aria-label={`更新${item.name}的本地状态`}
                    disabled={busy || writeLocked}
                    onChange={(event) =>
                      onStatusChange(
                        item,
                        event.currentTarget.value as ChannelStatus,
                      )
                    }
                    value={item.status}
                  >
                    <option value="active">active</option>
                    <option value="inactive">inactive</option>
                    <option value="archived">archived</option>
                  </select>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {items.length > 0 ? (
        <nav aria-label="渠道列表分页">
          <button
            type="button"
            disabled={busy || writeLocked || pagination.page <= 1}
            onClick={() => setPage(pagination.page - 1)}
          >上一页</button>
          <span>第 {pagination.page} / {pagination.pageCount} 页，共 {pagination.total} 条</span>
          <button
            type="button"
            disabled={busy || writeLocked || pagination.page >= pagination.pageCount}
            onClick={() => setPage(pagination.page + 1)}
          >下一页</button>
        </nav>
      ) : null}
    </section>
  );
}

function noopStatusChange(): void {}
function noopDetail(): void {}
function noopEdit(): void {}
function noopCreate(): void {}
function noopSaveConfiguration(input: ChannelConfigurationInput): void { void input; }

function configured(value: string): "已配置" | "未配置" {
  return value === "" ? "未配置" : "已配置";
}

function ChannelDetailPanel({
  state,
  onEdit,
  disabled,
}: {
  readonly state: ChannelDetailState;
  readonly onEdit: () => void;
  readonly disabled: boolean;
}): React.ReactElement | null {
  if (state.kind === "idle") return null;
  const value = state.kind === "ready" ? state.detail : state.previous;
  return <section data-testid="channel-detail">
    <h2>本地配置：{state.item.name}</h2>
    {value ? <dl>
      <dt>渠道类型</dt><dd>{value.channelType ?? "未配置"}</dd>
      <dt>载体类型</dt><dd>{value.carrierType ?? "未配置"}</dd>
      <dt>场景值</dt><dd>{configured(value.sceneValue)}</dd>
      <dt>本地二维码配置</dt><dd>{configured(value.qrURL)}</dd>
      <dt>本地获客链接配置</dt><dd>{configured(value.linkURL)}</dd>
      <dt>本地最终链接配置</dt><dd>{configured(value.finalURL)}</dd>
      <dt>配置发布状态</dt><dd>未发布；未生成二维码；provider=false；provider_execution_eligible=false</dd>
      <dt>本地负责人</dt><dd>{configured(value.ownerStaffID)}</dd>
      <dt>本地客户渠道</dt><dd>{configured(value.customerChannel)}</dd>
      <dt>欢迎语</dt><dd>{configured(value.welcomeMessage)}</dd>
      <dt>本地素材引用</dt><dd>图片 {value.imageMaterialCount}，小程序 {value.miniProgramMaterialCount}，附件 {value.attachmentMaterialCount}，群邀请 {value.groupInviteMaterialCount}</dd>
      <dt>入渠本地标签引用</dt><dd>{configured(value.entryTagID)}</dd>
      <dt>人员分配</dt><dd>{value.hasAssignmentConfig ? "已配置" : "未配置"}</dd>
    </dl> : null}
    {state.kind === "ready" ? <p><button type="button" disabled={disabled} onClick={onEdit}>编辑本地配置</button></p> : null}
    {state.kind === "loading" ? <p>正在读取本地配置。</p> : state.kind === "error" ? <p role="alert">{messages[state.failure]}</p> : null}
  </section>;
}

export interface ChannelFormState {
  readonly channelType: "qrcode" | "wecom_customer_acquisition";
  readonly carrierType: "qrcode" | "link";
  readonly channelName: string;
  readonly channelCode: string;
  readonly status: ChannelStatus;
  readonly sceneValue: string;
  readonly qrURL: string;
  readonly ownerStaffID: string;
  readonly customerChannel: string;
  readonly linkURL: string;
  readonly finalURL: string;
  readonly welcomeMessage: string;
  readonly imageMaterialIDs: string;
  readonly miniProgramMaterialIDs: string;
  readonly attachmentMaterialIDs: string;
  readonly groupInviteMaterialIDs: string;
  readonly autoAcceptFriend: boolean;
  readonly entryTagID: string;
  readonly entryTagName: string;
  readonly entryTagGroupName: string;
  readonly assignmentMode: "single_owner" | "multi_staff";
  readonly assignmentStrategy: "ratio" | "cap_switch";
  readonly overflowPolicy: string;
}

function initialChannelForm(existing?: ChannelDetail): ChannelFormState {
  return existing ? {
    channelType: existing.channelType, carrierType: existing.carrierType,
    channelName: existing.item.name, channelCode: existing.item.code, status: existing.item.status,
    sceneValue: existing.sceneValue, qrURL: existing.qrURL, ownerStaffID: existing.ownerStaffID,
    customerChannel: existing.customerChannel, linkURL: existing.linkURL, finalURL: existing.finalURL,
    welcomeMessage: existing.welcomeMessage, imageMaterialIDs: existing.imageMaterialIDs.join(","),
    miniProgramMaterialIDs: existing.miniProgramMaterialIDs.join(","), attachmentMaterialIDs: existing.attachmentMaterialIDs.join(","),
    groupInviteMaterialIDs: existing.groupInviteMaterialIDs.join(","), autoAcceptFriend: existing.autoAcceptFriend,
    entryTagID: existing.entryTagID, entryTagName: existing.entryTagName, entryTagGroupName: existing.entryTagGroupName,
    assignmentMode: existing.assignmentMode, assignmentStrategy: existing.assignmentStrategy, overflowPolicy: existing.overflowPolicy,
  } : {
    channelType: "qrcode", carrierType: "qrcode", channelName: "", channelCode: "", status: "active",
    sceneValue: "", qrURL: "", ownerStaffID: "", customerChannel: "", linkURL: "", finalURL: "", welcomeMessage: "",
    imageMaterialIDs: "", miniProgramMaterialIDs: "", attachmentMaterialIDs: "", groupInviteMaterialIDs: "",
    autoAcceptFriend: false, entryTagID: "", entryTagName: "", entryTagGroupName: "",
    assignmentMode: "single_owner", assignmentStrategy: "ratio", overflowPolicy: "least_loaded",
  };
}

function parseLocalIDs(value: string): readonly number[] | undefined {
  const trimmed = value.trim();
  if (trimmed === "") return [];
  const ids = trimmed.split(",");
  if (ids.length > 12 || ids.some((id) => !/^[1-9][0-9]*$/.test(id))) return undefined;
  const parsed = ids.map(Number);
  return parsed.every(Number.isSafeInteger) && new Set(parsed).size === parsed.length ? parsed : undefined;
}

export function channelConfigurationFromForm(value: ChannelFormState): ChannelConfigurationInput | undefined {
  const imageMaterialIDs = parseLocalIDs(value.imageMaterialIDs);
  const miniProgramMaterialIDs = parseLocalIDs(value.miniProgramMaterialIDs);
  const attachmentMaterialIDs = parseLocalIDs(value.attachmentMaterialIDs);
  const groupInviteMaterialIDs = parseLocalIDs(value.groupInviteMaterialIDs);
  if (!imageMaterialIDs || !miniProgramMaterialIDs || !attachmentMaterialIDs || !groupInviteMaterialIDs) return undefined;
  return {
    channelType: value.channelType, carrierType: value.carrierType,
    channelName: value.channelName.trim(), channelCode: value.channelCode.trim(), status: value.status,
    sceneValue: value.sceneValue.trim(), qrURL: value.qrURL.trim(), ownerStaffID: value.ownerStaffID.trim(),
    customerChannel: value.customerChannel.trim(), linkURL: value.linkURL.trim(), finalURL: value.finalURL.trim(),
    welcomeMessage: value.welcomeMessage.trim(), imageMaterialIDs, miniProgramMaterialIDs, attachmentMaterialIDs,
    groupInviteMaterialIDs, autoAcceptFriend: value.autoAcceptFriend, entryTagID: value.entryTagID.trim(),
    entryTagName: value.entryTagName.trim(), entryTagGroupName: value.entryTagGroupName.trim(),
    assignmentMode: value.assignmentMode, assignmentStrategy: value.assignmentStrategy, overflowPolicy: value.overflowPolicy.trim(),
  };
}

function ChannelConfigurationForm({
  busy,
  existing,
  onCancel,
  onSave,
}: {
  readonly busy: boolean;
  readonly existing?: ChannelDetail;
  readonly onCancel: () => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
  readonly onSave: (input: ChannelConfigurationInput) => void;
}): React.ReactElement {
  const [form, setForm] = useState(() => initialChannelForm(existing));
  const [error, setError] = useState<string>();
  const set = <K extends keyof ChannelFormState>(key: K, value: ChannelFormState[K]) => setForm((current) => ({ ...current, [key]: value }));
  const save = () => {
    const input = channelConfigurationFromForm(form);
    if (!input) { setError("素材引用必须是最多12个、以英文逗号分隔且不重复的正整数 ID。"); return; }
    setError(undefined);
    onSave(input);
  };
  return <section data-testid="channel-configuration-form" aria-label={existing ? "编辑本地渠道配置" : "新建本地渠道"}>
    <h2>{existing ? "编辑本地渠道配置" : "新建本地渠道"}</h2>
    <p>仅保存本地配置。二维码/获客链接未发布、未生成，provider=false。provider_execution_eligible=false。</p>
    {error ? <p role="alert">{error}</p> : null}
    <p><label>渠道名称<input disabled={busy} value={form.channelName} onChange={(event) => set("channelName", event.currentTarget.value)} /></label></p>
    <p><label>渠道编码<input disabled={busy || existing !== undefined} value={form.channelCode} onChange={(event) => set("channelCode", event.currentTarget.value)} /></label></p>
    <p><label>渠道类型<select disabled={busy} value={form.channelType} onChange={(event) => set("channelType", event.currentTarget.value as ChannelFormState["channelType"])}><option value="qrcode">qrcode</option><option value="wecom_customer_acquisition">wecom_customer_acquisition</option></select></label></p>
    <p><label>载体类型<select disabled={busy} value={form.carrierType} onChange={(event) => set("carrierType", event.currentTarget.value as ChannelFormState["carrierType"])}><option value="qrcode">qrcode</option><option value="link">link</option></select></label></p>
    <p><label>本地状态<select disabled={busy} value={form.status} onChange={(event) => set("status", event.currentTarget.value as ChannelStatus)}><option value="active">active</option><option value="inactive">inactive</option><option value="archived">archived</option></select></label></p>
    <p><label>场景值<input disabled={busy} value={form.sceneValue} onChange={(event) => set("sceneValue", event.currentTarget.value)} /></label></p>
    <p><label>本地二维码配置（纯文本，未发布）<input disabled={busy} value={form.qrURL} onChange={(event) => set("qrURL", event.currentTarget.value)} /></label></p>
    <p><label>本地获客链接配置（纯文本，未发布）<input disabled={busy} value={form.linkURL} onChange={(event) => set("linkURL", event.currentTarget.value)} /></label></p>
    <p><label>本地最终链接配置（纯文本，不打开）<input disabled={busy} value={form.finalURL} onChange={(event) => set("finalURL", event.currentTarget.value)} /></label></p>
    <fieldset aria-label="受保护本地引用">
      <legend>受保护本地引用（仅显示配置状态）</legend>
      <p>本地负责人：{configured(form.ownerStaffID)}</p>
      <p>本地客户渠道：{configured(form.customerChannel)}</p>
      <p>欢迎语正文：{configured(form.welcomeMessage)}</p>
      <p>图片素材 ID：{configured(form.imageMaterialIDs)}</p>
      <p>小程序素材 ID：{configured(form.miniProgramMaterialIDs)}</p>
      <p>附件素材 ID：{configured(form.attachmentMaterialIDs)}</p>
      <p>群邀请素材 ID：{configured(form.groupInviteMaterialIDs)}</p>
      <p>入渠标签 ID：{configured(form.entryTagID)}</p>
      <p>当前主线没有可在本 Lane 使用的 closed 负责人、欢迎语、素材与标签选项目录；因此页面不展示或手工接收原始身份、消息正文或引用 ID，编辑时保持已有本地配置。</p>
    </fieldset>
    <p><label>自动通过好友<input disabled={busy} checked={form.autoAcceptFriend} type="checkbox" onChange={(event) => set("autoAcceptFriend", event.currentTarget.checked)} /></label></p>
    <p><label>分配模式<select disabled={busy} value={form.assignmentMode} onChange={(event) => set("assignmentMode", event.currentTarget.value as ChannelFormState["assignmentMode"])}><option value="single_owner">single_owner</option><option value="multi_staff">multi_staff</option></select></label></p>
    <p><label>分配策略<select disabled={busy} value={form.assignmentStrategy} onChange={(event) => set("assignmentStrategy", event.currentTarget.value as ChannelFormState["assignmentStrategy"])}><option value="ratio">ratio</option><option value="cap_switch">cap_switch</option></select></label></p>
    <p>溢出策略：{configured(form.overflowPolicy)}</p>
    <p><button type="button" disabled={busy} onClick={save}>{existing ? "保存本地配置" : "创建本地渠道"}</button><button type="button" disabled={busy} onClick={onCancel}>取消</button></p>
  </section>;
}

function runtimeCookieHeader(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}
