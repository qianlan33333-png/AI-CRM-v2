import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import { WeComCallbackInboxPage } from "./wecom-callback-inbox-ui";
import type { CallbackInboxTransport } from "./wecom-callback-inbox";
import {
  archiveWecomTag,
  archiveWecomTagGroup,
  confirmsArchivedWecomTag,
  confirmsArchivedWecomTagGroup,
  confirmsCreatedWecomTag,
  confirmsCreatedWecomTagGroup,
  createWecomTag,
  confirmsRenamedWecomTag,
  confirmsRenamedWecomTagGroup,
  createWecomTagGroup,
  filterWecomTagGroups,
  generatedWecomTagsTransport,
  loadWecomTagCatalog,
  loadWecomTagExecutionGate,
  newWecomTagIdempotencyKey,
  nextWecomTagPage,
  previousWecomTagPage,
  renameWecomTag,
  renameWecomTagGroup,
  wecomTagPage,
  wecomTagPageCount,
  wecomTagSearchState,
  type WecomTagCatalog,
  type WecomTagExecutionGate,
  type WecomTagExecutionGateResult,
  type WecomTagCreateResult,
  type WecomTagGroupCreateResult,
  type WecomTagGroupArchiveResult,
  type WecomTagGroupRenameResult,
  type WecomTagArchiveResult,
  type WecomTagRenameResult,
  type WecomTagsRole,
  type WecomTagsTransport,
  type WecomTag,
  type WecomTagGroup,
} from "./wecom-tags";

export interface WecomTagsPageProps {
  readonly role: WecomTagsRole;
  readonly transport?: WecomTagsTransport;
  readonly callbackTransport?: CallbackInboxTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}

export type WecomTagsViewState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly catalog: WecomTagCatalog }
  | { readonly kind: "error" };

type WecomTagExecutionGateViewState =
  | { readonly kind: "loading"; readonly previous?: WecomTagExecutionGate }
  | { readonly kind: "ready"; readonly gate: WecomTagExecutionGate }
  | {
      readonly kind: "error";
      readonly failure: Exclude<WecomTagExecutionGateResult["status"], "loaded">;
      readonly previous?: WecomTagExecutionGate;
    };

export type WecomTagCopyStatus = "idle" | "copied" | "unavailable" | "failed";

type ClipboardWriter = Pick<Clipboard, "writeText">;

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

const createMessages: Record<
  Exclude<WecomTagGroupCreateResult["status"], "created"> | "csrf_missing",
  string
> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有本地标签目录创建权限。",
  invalid: "标签组和首个标签均须为 1–200 个字符。",
  unknown: "创建结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
  csrf_missing: "安全令牌缺失，未发送创建请求。",
};

// React does not commit disabled state between two synchronous submits. Keep
// the actual component's ref-backed single-flight rule testable.
export function startWecomTagGroupCreate(
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

export function startWecomTagMutation<T>(
  lock: { current: boolean },
  execute: () => Promise<T>,
): Promise<T> | undefined {
  if (lock.current) return undefined;
  lock.current = true;
  return (async () => {
    try {
      return await execute();
    } finally {
      lock.current = false;
    }
  })();
}

// Keep the Page's actual group-rename lifecycle outside the view markup so
// its ref-backed single-flight, receipt reload, and fail-closed lock can be
// exercised without a browser-only DOM harness.
export interface WecomTagGroupRenameController {
  readonly transport: WecomTagsTransport;
  readonly readCookie: () => string;
  readonly onUnauthenticated?: () => void;
  readonly mutationInFlight: { current: boolean };
  readonly mutationLocked: { current: boolean };
  readonly lockMutations: () => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setRenaming: (value: boolean) => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setCatalog: (catalog: WecomTagCatalog) => void;
}

export interface WecomTagCreateController {
  readonly transport: WecomTagsTransport;
  readonly readCookie: () => string;
  readonly onUnauthenticated?: () => void;
  readonly mutationInFlight: { current: boolean };
  readonly mutationLocked: { current: boolean };
  readonly lockMutations: () => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setCreatingTag: (value: boolean) => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setCatalog: (catalog: WecomTagCatalog) => void;
}

export function submitWecomTagCreate(
  controller: WecomTagCreateController,
  group: WecomTagGroup,
  tagName: string,
): Promise<WecomTagCreateResult | undefined> | undefined {
  if (controller.mutationLocked.current) return undefined;
  return startWecomTagMutation(controller.mutationInFlight, async () => {
    let csrfToken: string | undefined;
    try {
      csrfToken = readCSRFCookie(controller.readCookie());
    } catch {
      csrfToken = undefined;
    }
    const idempotencyKey = newWecomTagIdempotencyKey();
    if (!csrfToken || !idempotencyKey) return { status: "invalid" };
    controller.setCreatingTag(true);
    try {
      const result = await createWecomTag(
        controller.transport,
        group,
        tagName,
        csrfToken,
        idempotencyKey,
      );
      if (result.status !== "created") {
        if (result.status === "unauthenticated") controller.onUnauthenticated?.();
        if (result.status === "unknown") controller.lockMutations();
        return result;
      }
      const refreshed = await loadWecomTagCatalog(controller.transport);
      if (refreshed.status === "unauthenticated") controller.onUnauthenticated?.();
      if (
        refreshed.status === "loaded" &&
        confirmsCreatedWecomTag(refreshed.catalog, result.tag)
      ) {
        controller.setCatalog(refreshed.catalog);
        return result;
      }
      controller.lockMutations();
      return { status: "unknown" };
    } finally {
      controller.setCreatingTag(false);
    }
  });
}

export function submitWecomTagGroupRename(
  controller: WecomTagGroupRenameController,
  group: WecomTagGroup,
  groupName: string,
): Promise<WecomTagGroupRenameResult | undefined> | undefined {
  if (controller.mutationLocked.current) return undefined;
  return startWecomTagMutation(controller.mutationInFlight, async () => {
    let csrfToken: string | undefined;
    try {
      csrfToken = readCSRFCookie(controller.readCookie());
    } catch {
      csrfToken = undefined;
    }
    const idempotencyKey = newWecomTagIdempotencyKey();
    if (!csrfToken || !idempotencyKey) return { status: "invalid" };

    controller.setRenaming(true);
    try {
      const result = await renameWecomTagGroup(
        controller.transport,
        group,
        groupName,
        csrfToken,
        idempotencyKey,
      );
      if (result.status !== "confirmed") {
        if (result.status === "unauthenticated") {
          controller.onUnauthenticated?.();
        }
        if (result.status === "unknown") controller.lockMutations();
        return result;
      }
      const refreshed = await loadWecomTagCatalog(controller.transport);
      if (refreshed.status === "unauthenticated") {
        controller.onUnauthenticated?.();
      }
      if (
        refreshed.status === "loaded" &&
        confirmsRenamedWecomTagGroup(refreshed.catalog, result.group)
      ) {
        controller.setCatalog(refreshed.catalog);
        return result;
      }
      controller.lockMutations();
      return { status: "unknown" };
    } finally {
      controller.setRenaming(false);
    }
  });
}

export interface WecomTagGroupArchiveController {
  readonly transport: WecomTagsTransport;
  readonly readCookie: () => string;
  readonly onUnauthenticated?: () => void;
  readonly mutationInFlight: { current: boolean };
  readonly mutationLocked: { current: boolean };
  readonly lockMutations: () => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setArchiving: (value: boolean) => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setCatalog: (catalog: WecomTagCatalog) => void;
}

export function submitWecomTagGroupArchive(
  controller: WecomTagGroupArchiveController,
  group: WecomTagGroup,
): Promise<WecomTagGroupArchiveResult | undefined> | undefined {
  if (controller.mutationLocked.current) return undefined;
  return startWecomTagMutation(controller.mutationInFlight, async () => {
    let csrfToken: string | undefined;
    try {
      csrfToken = readCSRFCookie(controller.readCookie());
    } catch {
      csrfToken = undefined;
    }
    const idempotencyKey = newWecomTagIdempotencyKey();
    if (!csrfToken || !idempotencyKey) return { status: "invalid" };

    controller.setArchiving(true);
    try {
      const result = await archiveWecomTagGroup(
        controller.transport,
        group,
        csrfToken,
        idempotencyKey,
      );
      if (result.status !== "archived") {
        if (result.status === "unauthenticated") {
          controller.onUnauthenticated?.();
        }
        if (result.status === "unknown") controller.lockMutations();
        return result;
      }
      const refreshed = await loadWecomTagCatalog(controller.transport);
      if (refreshed.status === "unauthenticated") {
        controller.onUnauthenticated?.();
      }
      if (
        refreshed.status === "loaded" &&
        confirmsArchivedWecomTagGroup(refreshed.catalog, result, group)
      ) {
        controller.setCatalog(refreshed.catalog);
        return result;
      }
      controller.lockMutations();
      return { status: "unknown" };
    } finally {
      controller.setArchiving(false);
    }
  });
}

export interface WecomTagArchiveController {
  readonly transport: WecomTagsTransport;
  readonly readCookie: () => string;
  readonly onUnauthenticated?: () => void;
  readonly mutationInFlight: { current: boolean };
  readonly mutationLocked: { current: boolean };
  readonly lockMutations: () => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setArchiving: (value: boolean) => void;
  // eslint-disable-next-line no-unused-vars -- named setter parameter is required by TS function-type syntax.
  readonly setCatalog: (catalog: WecomTagCatalog) => void;
}

export function submitWecomTagArchive(
  controller: WecomTagArchiveController,
  tag: WecomTag,
): Promise<WecomTagArchiveResult | undefined> | undefined {
  if (controller.mutationLocked.current) return undefined;
  return startWecomTagMutation(controller.mutationInFlight, async () => {
    let csrfToken: string | undefined;
    try {
      csrfToken = readCSRFCookie(controller.readCookie());
    } catch {
      csrfToken = undefined;
    }
    const idempotencyKey = newWecomTagIdempotencyKey();
    if (!csrfToken || !idempotencyKey) return { status: "invalid" };
    controller.setArchiving(true);
    try {
      const result = await archiveWecomTag(
        controller.transport,
        tag,
        csrfToken,
        idempotencyKey,
      );
      if (result.status !== "archived") {
        if (result.status === "unauthenticated")
          controller.onUnauthenticated?.();
        if (result.status === "unknown") controller.lockMutations();
        return result;
      }
      const refreshed = await loadWecomTagCatalog(controller.transport);
      if (refreshed.status === "unauthenticated")
        controller.onUnauthenticated?.();
      if (
        refreshed.status === "loaded" &&
        confirmsArchivedWecomTag(refreshed.catalog, result, tag)
      ) {
        controller.setCatalog(refreshed.catalog);
        return result;
      }
      controller.lockMutations();
      return { status: "unknown" };
    } finally {
      controller.setArchiving(false);
    }
  });
}

export async function copyWecomTagID(
  tagID: number,
  clipboard: ClipboardWriter | undefined = typeof navigator === "undefined"
    ? undefined
    : navigator.clipboard,
): Promise<Exclude<WecomTagCopyStatus, "idle">> {
  if (!clipboard || typeof clipboard.writeText !== "function") {
    return "unavailable";
  }
  try {
    await clipboard.writeText(String(tagID));
    return "copied";
  } catch {
    return "failed";
  }
}

export function WecomTagsPage({
  role,
  transport = generatedWecomTagsTransport,
  callbackTransport,
  readCookie = browserCookie,
  onUnauthenticated,
}: WecomTagsPageProps): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [state, setState] = useState<WecomTagsViewState>({ kind: "loading" });
  const [executionGate, setExecutionGate] =
    useState<WecomTagExecutionGateViewState>({ kind: "loading" });
  const [groupName, setGroupName] = useState("");
  const [firstTagName, setFirstTagName] = useState("");
  const [creating, setCreating] = useState(false);
  const [creatingTag, setCreatingTag] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [mutationUncertain, setMutationUncertain] = useState(false);
  const [createNotice, setCreateNotice] = useState<string>();
  const mutationInFlight = useRef(false);
  const mutationLocked = useRef(false);
  const executionGateGeneration = useRef(0);
  const executionGateFlight = useRef<number>();
  const executionGateVerified = useRef<WecomTagExecutionGate>();
  const unauthenticatedNotified = useRef(false);
  const reportUnauthenticated = useCallback(() => {
    if (unauthenticatedNotified.current) return;
    unauthenticatedNotified.current = true;
    onUnauthenticated?.();
  }, [onUnauthenticated]);
  const lockMutations = () => {
    mutationLocked.current = true;
    setMutationUncertain(true);
  };

  useEffect(() => {
    if (!canAccess) return undefined;
    let active = true;
    void loadWecomTagCatalog(transport).then((result) => {
      if (!active) return;
      if (result.status === "unauthenticated") reportUnauthenticated();
      setState(
        result.status === "loaded"
          ? { kind: "ready", catalog: result.catalog }
          : { kind: "error" },
      );
    });
    return () => {
      active = false;
    };
  }, [canAccess, reportUnauthenticated, transport]);

  const refreshExecutionGate = useCallback(() => {
    if (!canAccess || executionGateFlight.current !== undefined) return;
    const token = ++executionGateGeneration.current;
    executionGateFlight.current = token;
    setExecutionGate({
      kind: "loading",
      previous: executionGateVerified.current,
    });
    void loadWecomTagExecutionGate(transport)
      .then((result) => {
        if (token !== executionGateGeneration.current) return;
        if (result.status === "loaded") {
          executionGateVerified.current = result.gate;
          setExecutionGate({ kind: "ready", gate: result.gate });
          return;
        }
        if (result.status === "unauthenticated") reportUnauthenticated();
        setExecutionGate({
          kind: "error",
          failure: result.status,
          previous: executionGateVerified.current,
        });
      })
      .finally(() => {
        if (executionGateFlight.current === token) {
          executionGateFlight.current = undefined;
        }
      });
  }, [canAccess, reportUnauthenticated, transport]);

  useEffect(() => {
    if (!canAccess) return undefined;
    refreshExecutionGate();
    return () => {
      executionGateGeneration.current += 1;
      executionGateFlight.current = undefined;
    };
  }, [canAccess, refreshExecutionGate]);

  const submitCreate = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canAccess || mutationLocked.current || mutationUncertain) return;
    void startWecomTagGroupCreate(mutationInFlight, async () => {
      let csrfToken: string | undefined;
      try {
        csrfToken = readCSRFCookie(readCookie());
      } catch {
        csrfToken = undefined;
      }
      const idempotencyKey = newWecomTagIdempotencyKey();
      if (!csrfToken || !idempotencyKey) {
        setCreateNotice(createMessages.csrf_missing);
        return;
      }
      setCreating(true);
      setCreateNotice(undefined);
      try {
        const result = await createWecomTagGroup(
          transport,
          groupName,
          firstTagName,
          csrfToken,
          idempotencyKey,
        );
        if (result.status === "created") {
          setGroupName("");
          setFirstTagName("");
          setCreateNotice("本地标签组和首个标签已创建。");
          const refreshed = await loadWecomTagCatalog(transport);
          if (refreshed.status === "unauthenticated") onUnauthenticated?.();
          if (
            refreshed.status === "loaded" &&
            confirmsCreatedWecomTagGroup(refreshed.catalog, result)
          ) {
            setState({ kind: "ready", catalog: refreshed.catalog });
          } else {
            lockMutations();
            setCreateNotice(
              "创建已确认，但目录刷新未确认；请人工刷新页面后核对目录。",
            );
          }
          return;
        }
        if (result.status === "unauthenticated") onUnauthenticated?.();
        if (result.status === "unknown") lockMutations();
        setCreateNotice(createMessages[result.status]);
      } finally {
        setCreating(false);
      }
    });
  };

  const createPanel = canAccess ? (
    <section aria-labelledby="wecom-tag-create-title">
      <h2 id="wecom-tag-create-title">创建本地标签组</h2>
      <p>仅创建本地标签目录记录，不会同步或操作企微联系人。</p>
      <form onSubmit={submitCreate}>
        <fieldset
          disabled={creating || renaming || archiving || mutationUncertain}
        >
          <label>
            标签组名称
            <input
              value={groupName}
              onChange={(event) => setGroupName(event.currentTarget.value)}
            />
          </label>
          <label>
            首个标签名称
            <input
              value={firstTagName}
              onChange={(event) => setFirstTagName(event.currentTarget.value)}
            />
          </label>
          <button type="submit">{creating ? "正在创建…" : "创建标签组"}</button>
        </fieldset>
      </form>
      {createNotice ? (
        <p aria-live="polite" role={mutationUncertain ? "alert" : "status"}>
          {createNotice}
        </p>
      ) : null}
    </section>
  ) : undefined;

  const onRenameTag = async (
    tag: WecomTag,
    tagName: string,
  ): Promise<WecomTagRenameResult | undefined> => {
    if (!canAccess || mutationLocked.current || mutationUncertain)
      return undefined;
    return startWecomTagMutation(mutationInFlight, async () => {
      let csrfToken: string | undefined;
      try {
        csrfToken = readCSRFCookie(readCookie());
      } catch {
        csrfToken = undefined;
      }
      const idempotencyKey = newWecomTagIdempotencyKey();
      if (!csrfToken || !idempotencyKey) return { status: "invalid" };

      setRenaming(true);
      try {
        const result = await renameWecomTag(
          transport,
          tag,
          tagName,
          csrfToken,
          idempotencyKey,
        );
        if (result.status !== "confirmed") {
          if (result.status === "unauthenticated") onUnauthenticated?.();
          if (result.status === "unknown") lockMutations();
          return result;
        }
        const refreshed = await loadWecomTagCatalog(transport);
        if (refreshed.status === "unauthenticated") onUnauthenticated?.();
        if (
          refreshed.status === "loaded" &&
          confirmsRenamedWecomTag(refreshed.catalog, result.tag)
        ) {
          setState({ kind: "ready", catalog: refreshed.catalog });
          return result;
        }
        lockMutations();
        return { status: "unknown" };
      } finally {
        setRenaming(false);
      }
    });
  };

  const onCreateTag = async (
    group: WecomTagGroup,
    tagName: string,
  ): Promise<WecomTagCreateResult | undefined> => {
    if (!canAccess) return undefined;
    return submitWecomTagCreate(
      {
        transport,
        readCookie,
        onUnauthenticated,
        mutationInFlight,
        mutationLocked,
        lockMutations,
        setCreatingTag,
        setCatalog: (catalog) => setState({ kind: "ready", catalog }),
      },
      group,
      tagName,
    );
  };

  const onRenameGroup = async (
    group: WecomTagGroup,
    groupName: string,
  ): Promise<WecomTagGroupRenameResult | undefined> => {
    if (!canAccess) return undefined;
    return submitWecomTagGroupRename(
      {
        transport,
        readCookie,
        onUnauthenticated,
        mutationInFlight,
        mutationLocked,
        lockMutations,
        setRenaming,
        setCatalog: (catalog) => setState({ kind: "ready", catalog }),
      },
      group,
      groupName,
    );
  };

  const onArchiveGroup = async (
    group: WecomTagGroup,
  ): Promise<WecomTagGroupArchiveResult | undefined> => {
    if (!canAccess) return undefined;
    return submitWecomTagGroupArchive(
      {
        transport,
        readCookie,
        onUnauthenticated,
        mutationInFlight,
        mutationLocked,
        lockMutations,
        setArchiving,
        setCatalog: (catalog) => setState({ kind: "ready", catalog }),
      },
      group,
    );
  };

  const onArchiveTag = async (
    tag: WecomTag,
  ): Promise<WecomTagArchiveResult | undefined> => {
    if (!canAccess) return undefined;
    return submitWecomTagArchive(
      {
        transport,
        readCookie,
        onUnauthenticated,
        mutationInFlight,
        mutationLocked,
        lockMutations,
        setArchiving,
        setCatalog: (catalog) => setState({ kind: "ready", catalog }),
      },
      tag,
    );
  };

  return (
    <>
      <WecomTagsView
        createPanel={createPanel}
        mutationBusy={creating || creatingTag || renaming || archiving}
        mutationLocked={mutationUncertain}
        onCreateTag={onCreateTag}
        onArchiveGroup={onArchiveGroup}
        onArchiveTag={onArchiveTag}
        onRenameGroup={onRenameGroup}
        onRenameTag={onRenameTag}
        renaming={renaming}
        archiving={archiving}
        role={role}
        state={state}
      />
      <WecomTagExecutionGatePanel
        role={role}
        state={executionGate}
        onRefresh={refreshExecutionGate}
      />
      {role === "admin" ? (
        <WeComCallbackInboxPage
          role="admin"
          transport={callbackTransport}
          onUnauthenticated={onUnauthenticated}
        />
      ) : null}
    </>
  );
}

export function WecomTagExecutionGatePanel({
  role,
  state,
  onRefresh,
}: {
  readonly role: WecomTagsRole;
  readonly state: WecomTagExecutionGateViewState;
  readonly onRefresh: () => void;
}): React.ReactElement | null {
  if (role !== "admin" && role !== "ops") return null;
  const gate = state.kind === "ready" ? state.gate : state.previous;
  return (
    <section aria-labelledby="wecom-tag-execution-gate-title">
      <h2 id="wecom-tag-execution-gate-title">本地标签执行前置状态</h2>
      <p>
        仅显示本地门禁与队列观测；不会发起同步、标记或取消标记，也不代表企微执行、送达或成功。
      </p>
      {gate ? (
        <dl>
          <dt>提供方执行资格</dt>
          <dd>不可用</dd>
          <dt>本地命令受理</dt>
          <dd>可用</dd>
          <dt>本地队列</dt>
          <dd>可用</dd>
          <dt>同步已执行</dt>
          <dd>否</dd>
          <dt>观测时间</dt>
          <dd>{gate.observedAt}</dd>
        </dl>
      ) : null}
      {state.kind === "loading" ? <p>正在读取本地门禁观测…</p> : null}
      {state.kind === "error" ? (
        <p role="alert">
          本地门禁观测不可用；不会据此推断企微执行或成功。
        </p>
      ) : null}
      <button type="button" onClick={onRefresh} disabled={state.kind === "loading"}>
        刷新本地观测
      </button>
    </section>
  );
}

export function WecomTagsView({
  role,
  state,
  createPanel,
  mutationBusy = false,
  mutationLocked = false,
  onArchiveGroup,
  onArchiveTag,
  onCreateTag,
  onRenameGroup,
  onRenameTag,
  renaming = false,
  archiving = false,
}: {
  readonly role: WecomTagsRole;
  readonly state: WecomTagsViewState;
  readonly createPanel?: React.ReactNode;
  readonly mutationBusy?: boolean;
  readonly mutationLocked?: boolean;
  readonly onArchiveGroup?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    group: WecomTagGroup,
  ) => Promise<WecomTagGroupArchiveResult | undefined>;
  readonly onArchiveTag?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tag: WecomTag,
  ) => Promise<WecomTagArchiveResult | undefined>;
  readonly onCreateTag?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    group: WecomTagGroup,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tagName: string,
  ) => Promise<WecomTagCreateResult | undefined>;
  readonly onRenameGroup?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    group: WecomTagGroup,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    groupName: string,
  ) => Promise<WecomTagGroupRenameResult | undefined>;
  readonly onRenameTag?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tag: WecomTag,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tagName: string,
  ) => Promise<WecomTagRenameResult | undefined>;
  readonly renaming?: boolean;
  readonly archiving?: boolean;
}): React.ReactElement {
  const canAccess = role === "admin" || role === "ops";
  const [query, setQuery] = useState("");
  const groups = useMemo(
    () =>
      state.kind === "ready" ? filterWecomTagGroups(state.catalog, query) : [],
    [query, state],
  );
  const [selectedGroupID, setSelectedGroupID] = useState<number>();
  const [selectedTagID, setSelectedTagID] = useState<number>();
  const [copyStatus, setCopyStatus] = useState<WecomTagCopyStatus>("idle");
  const [page, setPage] = useState(0);

  useEffect(() => {
    const next =
      state.kind === "ready"
        ? wecomTagSearchState(state.catalog, query)
        : { selectedGroupID: undefined, page: 0 as const };
    setSelectedGroupID(next.selectedGroupID);
    setSelectedTagID(undefined);
    setCopyStatus("idle");
    setPage(0);
  }, [state]);

  if (!canAccess)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">企微标签目录</h1>
        <p role="alert">当前账号没有企微标签目录访问权限。</p>
      </section>
    );
  if (state.kind === "loading")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">企微标签目录</h1>
        <p>正在读取企微标签目录。</p>
      </section>
    );
  if (state.kind === "error")
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">企微标签目录</h1>
        <p role="alert">企微标签目录暂不可用。</p>
      </section>
    );

  const selected =
    groups.find((group) => group.id === selectedGroupID) ?? groups[0];
  const tags = selected?.tags ?? [];
  const currentPage = Math.min(page, wecomTagPageCount(tags) - 1);
  const visibleTags = wecomTagPage(tags, currentPage);
  const nextPage = nextWecomTagPage(currentPage, tags);
  const selectedTag = tags.find((tag) => tag.id === selectedTagID);

  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">只读目录</p>
      <h1 id="app-title">企微标签目录</h1>
      <dl>
        <dt>标签总数</dt>
        <dd>{state.catalog.totalTags}</dd>
        <dt>标签上限</dt>
        <dd>{state.catalog.tagLimit}</dd>
        <dt>本地目录快照时间（非企微同步）</dt>
        <dd>{state.catalog.snapshotAt}</dd>
      </dl>
      {createPanel}
      <label>
        搜索标签组、标签名称或标签 ID
        <input
          type="search"
          value={query}
          onChange={(event) => {
            const nextQuery = event.currentTarget.value;
            const next = wecomTagSearchState(state.catalog, nextQuery);
            setQuery(nextQuery);
            setSelectedGroupID(next.selectedGroupID);
            setSelectedTagID(undefined);
            setCopyStatus("idle");
            setPage(next.page);
          }}
        />
      </label>
      <section aria-label="标签组">
        <h2>标签组</h2>
        {groups.length === 0 ? (
          <p>没有匹配的标签组。</p>
        ) : (
          <ul>
            {groups.map((group) => (
              <li key={group.id}>
                <button
                  type="button"
                  aria-pressed={selected?.id === group.id}
                  onClick={() => {
                    setSelectedGroupID(group.id);
                    setSelectedTagID(undefined);
                    setCopyStatus("idle");
                    setPage(0);
                  }}
                >
                  {group.name}（{group.tags.length}）
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      {selected ? (
        <section aria-labelledby="wecom-tag-list-title">
          <h2 id="wecom-tag-list-title">{selected.name}</h2>
          <WecomTagGroupDetails
            group={selected}
            mutationBusy={mutationBusy}
            mutationLocked={mutationLocked}
            onArchive={onArchiveGroup}
            onRename={onRenameGroup}
            renaming={renaming}
            archiving={archiving}
          />
          {onCreateTag ? (
            <WecomTagCreateForm
              busy={mutationBusy}
              group={selected}
              mutationLocked={mutationLocked}
              onCreate={onCreateTag}
            />
          ) : null}
          {visibleTags.length === 0 ? (
            <p>当前筛选下没有标签。</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>标签 ID</th>
                  <th>标签名称</th>
                </tr>
              </thead>
              <tbody>
                {visibleTags.map((tag) => (
                  <tr key={tag.id}>
                    <td>{tag.id}</td>
                    <td>
                      <button
                        type="button"
                        aria-pressed={selectedTag?.id === tag.id}
                        onClick={() => {
                          setSelectedTagID(tag.id);
                          setCopyStatus("idle");
                        }}
                      >
                        {tag.name}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p>
            第 {currentPage + 1} 页，共 {wecomTagPageCount(tags)} 页
          </p>
          <button
            type="button"
            disabled={currentPage === 0}
            onClick={() => setPage(previousWecomTagPage(currentPage))}
          >
            上一页
          </button>
          <button
            type="button"
            disabled={nextPage === undefined}
            onClick={() => setPage(nextPage ?? currentPage)}
          >
            下一页
          </button>
          {selectedTag ? (
            <WecomTagDetails
              copyStatus={copyStatus}
              mutationBusy={mutationBusy}
              mutationLocked={mutationLocked}
              onCopy={() => {
                void copyWecomTagID(selectedTag.id).then(setCopyStatus);
              }}
              onArchive={onArchiveTag}
              onRename={onRenameTag}
              renaming={renaming}
              tag={selectedTag}
            />
          ) : null}
        </section>
      ) : null}
    </section>
  );
}

export function WecomTagGroupDetails({
  group,
  mutationBusy = false,
  mutationLocked = false,
  onArchive,
  onRename,
  renaming = false,
  archiving = false,
}: {
  readonly group: WecomTagGroup;
  readonly mutationBusy?: boolean;
  readonly mutationLocked?: boolean;
  readonly onArchive?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    group: WecomTagGroup,
  ) => Promise<WecomTagGroupArchiveResult | undefined>;
  readonly onRename?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    group: WecomTagGroup,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    groupName: string,
  ) => Promise<WecomTagGroupRenameResult | undefined>;
  readonly renaming?: boolean;
  readonly archiving?: boolean;
}): React.ReactElement {
  const [groupName, setGroupName] = useState(group.name);
  const [renameNotice, setRenameNotice] = useState<string>();
  const [archiveConfirmation, setArchiveConfirmation] = useState(false);
  const [archiveNotice, setArchiveNotice] = useState<string>();

  useEffect(() => {
    setGroupName(group.name);
    setRenameNotice(undefined);
    setArchiveConfirmation(false);
    setArchiveNotice(undefined);
  }, [group.id, group.name]);

  return (
    <section aria-labelledby="wecom-tag-group-detail-title">
      <h3 id="wecom-tag-group-detail-title">标签组详情</h3>
      <dl>
        <dt>标签组名称</dt>
        <dd>{group.name}</dd>
        <dt>标签组 ID</dt>
        <dd>{group.id}</dd>
      </dl>
      {onRename ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (mutationLocked || mutationBusy) return;
            void onRename(group, groupName).then((result) => {
              if (!result) return;
              const notices: Record<
                WecomTagGroupRenameResult["status"],
                string
              > = {
                confirmed: "本地标签组名称已更新。",
                unauthenticated: "登录状态已失效，请重新登录。",
                forbidden: "当前账号没有本地标签组改名权限。",
                invalid: "标签组名称不符合已冻结的本地合同。",
                unknown:
                  "改名结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
              };
              setRenameNotice(notices[result.status]);
            });
          }}
        >
          <fieldset disabled={mutationLocked || mutationBusy}>
            <label>
              本地标签组名称
              <input
                value={groupName}
                onChange={(event) => setGroupName(event.currentTarget.value)}
              />
            </label>
            <button type="submit">
              {renaming ? "正在保存…" : "保存本地标签组名称"}
            </button>
          </fieldset>
        </form>
      ) : null}
      {renameNotice ? (
        <p aria-live="polite" role={mutationLocked ? "alert" : "status"}>
          {renameNotice}
        </p>
      ) : null}
      {onArchive ? (
        <section aria-labelledby="wecom-tag-group-archive-title">
          <h4 id="wecom-tag-group-archive-title">归档本地标签组</h4>
          {archiveConfirmation ? (
            <>
              <p>确认归档本地标签组“{group.name}”及其本地标签？</p>
              <button
                type="button"
                disabled={mutationLocked || mutationBusy}
                onClick={() => {
                  if (mutationLocked || mutationBusy) return;
                  void onArchive(group).then((result) => {
                    if (!result) return;
                    const notices: Record<
                      WecomTagGroupArchiveResult["status"],
                      string
                    > = {
                      archived: "本地标签组已归档。",
                      unauthenticated: "登录状态已失效，请重新登录。",
                      forbidden: "当前账号没有本地标签组归档权限。",
                      invalid: "本地标签组归档请求未通过校验。",
                      unknown:
                        "本地归档结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
                    };
                    if (result.status === "archived") {
                      setArchiveConfirmation(false);
                    }
                    setArchiveNotice(notices[result.status]);
                  });
                }}
              >
                {archiving ? "正在归档…" : "确认归档本地标签组"}
              </button>
              <button
                type="button"
                disabled={mutationLocked || mutationBusy}
                onClick={() => setArchiveConfirmation(false)}
              >
                取消
              </button>
            </>
          ) : (
            <button
              type="button"
              disabled={mutationLocked || mutationBusy}
              onClick={() => setArchiveConfirmation(true)}
            >
              归档本地标签组
            </button>
          )}
          {archiveNotice ? (
            <p aria-live="polite" role={mutationLocked ? "alert" : "status"}>
              {archiveNotice}
            </p>
          ) : null}
        </section>
      ) : null}
    </section>
  );
}

function WecomTagCreateForm({
  busy,
  group,
  mutationLocked,
  onCreate,
}: {
  readonly busy: boolean;
  readonly group: WecomTagGroup;
  readonly mutationLocked: boolean;
  readonly onCreate: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    group: WecomTagGroup,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tagName: string,
  ) => Promise<WecomTagCreateResult | undefined>;
}): React.ReactElement {
  const [tagName, setTagName] = useState("");
  const [notice, setNotice] = useState<string>();

  useEffect(() => {
    setTagName("");
    setNotice(undefined);
  }, [group.id, group.name]);

  return (
    <section aria-labelledby="wecom-tag-create-title">
      <h3 id="wecom-tag-create-title">新增本地标签</h3>
      <p>标签将仅写入已选本地标签组，不会同步或操作企微联系人。</p>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (busy || mutationLocked) return;
          void onCreate(group, tagName).then((result) => {
            if (!result) return;
            const messages: Record<WecomTagCreateResult["status"], string> = {
              created: "本地标签已创建。",
              unauthenticated: "登录状态已失效，请重新登录。",
              forbidden: "当前账号没有本地标签目录创建权限。",
              invalid: "标签名称不符合已冻结的本地合同。",
              unknown:
                "创建结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
            };
            if (result.status === "created") setTagName("");
            setNotice(messages[result.status]);
          });
        }}
      >
        <fieldset disabled={busy || mutationLocked}>
          <label>
            本地标签名称
            <input
              value={tagName}
              onChange={(event) => setTagName(event.currentTarget.value)}
            />
          </label>
          <button type="submit">新增本地标签</button>
        </fieldset>
      </form>
      {notice ? (
        <p aria-live="polite" role={mutationLocked ? "alert" : "status"}>
          {notice}
        </p>
      ) : null}
    </section>
  );
}

export function WecomTagDetails({
  copyStatus,
  mutationBusy = false,
  mutationLocked = false,
  onArchive,
  onCopy,
  onRename,
  renaming = false,
  tag,
}: {
  readonly copyStatus: WecomTagCopyStatus;
  readonly mutationBusy?: boolean;
  readonly mutationLocked?: boolean;
  readonly onArchive?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tag: WecomTag,
  ) => Promise<WecomTagArchiveResult | undefined>;
  readonly onCopy: () => void;
  readonly onRename?: (
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tag: WecomTag,
    // eslint-disable-next-line no-unused-vars -- named callback parameter is required by TS function-type syntax.
    tagName: string,
  ) => Promise<WecomTagRenameResult | undefined>;
  readonly renaming?: boolean;
  readonly tag: WecomTag;
}): React.ReactElement {
  const [tagName, setTagName] = useState(tag.name);
  const [renameNotice, setRenameNotice] = useState<string>();
  const [archiveConfirmation, setArchiveConfirmation] = useState(false);
  const [archiveNotice, setArchiveNotice] = useState<string>();

  useEffect(() => {
    setTagName(tag.name);
    setRenameNotice(undefined);
    setArchiveConfirmation(false);
    setArchiveNotice(undefined);
  }, [tag.id, tag.name]);

  return (
    <section aria-labelledby="wecom-tag-detail-title">
      <h2 id="wecom-tag-detail-title">标签详情</h2>
      <dl>
        <dt>标签名称</dt>
        <dd>{tag.name}</dd>
        <dt>标签 ID</dt>
        <dd>{tag.id}</dd>
        <dt>标签组名称</dt>
        <dd>{tag.groupName}</dd>
      </dl>
      <button type="button" onClick={onCopy}>
        复制标签 ID
      </button>
      {onRename ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (mutationLocked || mutationBusy) return;
            void onRename(tag, tagName).then((result) => {
              if (!result) return;
              const notices: Record<WecomTagRenameResult["status"], string> = {
                confirmed: "本地标签名称已更新。",
                unauthenticated: "登录状态已失效，请重新登录。",
                forbidden: "当前账号没有本地标签目录改名权限。",
                invalid: "标签名称不符合已冻结的本地合同。",
                unknown:
                  "改名结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
              };
              setRenameNotice(notices[result.status]);
            });
          }}
        >
          <fieldset disabled={mutationLocked || mutationBusy}>
            <label>
              本地标签名称
              <input
                value={tagName}
                onChange={(event) => setTagName(event.currentTarget.value)}
              />
            </label>
            <button type="submit">
              {renaming ? "正在保存…" : "保存本地名称"}
            </button>
          </fieldset>
        </form>
      ) : null}
      {renameNotice ? (
        <p aria-live="polite" role={mutationLocked ? "alert" : "status"}>
          {renameNotice}
        </p>
      ) : null}
      {onArchive ? (
        <section aria-labelledby="wecom-tag-archive-title">
          <h3 id="wecom-tag-archive-title">归档本地标签</h3>
          {archiveConfirmation ? (
            <>
              <p>确认归档本地标签“{tag.name}”？</p>
              <button
                type="button"
                disabled={mutationLocked || mutationBusy}
                onClick={() => {
                  if (mutationLocked || mutationBusy) return;
                  void onArchive(tag).then((result) => {
                    if (!result) return;
                    const notices: Record<
                      WecomTagArchiveResult["status"],
                      string
                    > = {
                      archived: "本地标签已归档。",
                      unauthenticated: "登录状态已失效，请重新登录。",
                      forbidden: "当前账号没有本地标签归档权限。",
                      invalid: "本地标签归档请求未通过校验。",
                      unknown:
                        "本地归档结果未确认，系统不会自动重试；请人工刷新页面后核对目录。",
                    };
                    if (result.status === "archived")
                      setArchiveConfirmation(false);
                    setArchiveNotice(notices[result.status]);
                  });
                }}
              >
                {mutationBusy ? "正在归档…" : "确认归档本地标签"}
              </button>
              <button
                type="button"
                disabled={mutationLocked || mutationBusy}
                onClick={() => setArchiveConfirmation(false)}
              >
                取消
              </button>
            </>
          ) : (
            <button
              type="button"
              disabled={mutationLocked || mutationBusy}
              onClick={() => setArchiveConfirmation(true)}
            >
              归档本地标签
            </button>
          )}
          {archiveNotice ? (
            <p aria-live="polite" role={mutationLocked ? "alert" : "status"}>
              {archiveNotice}
            </p>
          ) : null}
        </section>
      ) : null}
      {copyStatus === "copied" ? (
        <p aria-live="polite" role="status">
          标签 ID 已复制。
        </p>
      ) : null}
      {copyStatus === "unavailable" ? (
        <p aria-live="polite" role="status">
          当前浏览器不支持复制，请手工复制上方标签 ID。
        </p>
      ) : null}
      {copyStatus === "failed" ? (
        <p aria-live="polite" role="status">
          复制失败，请手工复制上方标签 ID。
        </p>
      ) : null}
    </section>
  );
}
