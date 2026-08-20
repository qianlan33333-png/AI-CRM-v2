import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import { CRMTagCatalogPage } from "./crm-tags-ui";
import type { CRMTagTransport } from "./crm-tags";
import {
  generatedStageTransport,
  loadStages,
  newStageIdempotencyKey,
  submitStageArchive,
  submitStageCreate,
  submitStageReorder,
  submitStageRename,
  type StageMutationFailure,
  type StageRecord,
  type StageRole,
  type StageTransport,
} from "./stages";

export interface StagesPageProps {
  readonly role: StageRole;
  readonly transport?: StageTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
  readonly crmTagTransport?: CRMTagTransport;
}

type PageState =
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly items: readonly StageRecord[] }
  | { readonly kind: "unavailable" };

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

const mutationMessages: Record<StageMutationFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "权限已变化，当前操作未执行。",
  not_found: "该阶段已不存在，请刷新列表后重试。",
  conflict: "阶段名称或排序与现有数据冲突，请修改后重试。",
  invalid: "提交内容不符合要求，请检查后重试。",
  unavailable: "结果尚未确认；为避免重复写入，已锁定阶段编辑。请刷新列表后核对。",
};

function sameStage(left: StageRecord, right: StageRecord): boolean {
  return (
    left.id === right.id &&
    left.name === right.name &&
    left.sortOrder === right.sortOrder &&
    JSON.stringify(left.config) === JSON.stringify(right.config)
  );
}

function parseSortOrder(value: string): number | undefined {
  if (value === "") return undefined;
  if (!/^-?\d+$/.test(value)) return Number.NaN;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : Number.NaN;
}

export function StagesPage({
  role,
  transport = generatedStageTransport,
  readCookie = browserCookie,
  onUnauthenticated,
  crmTagTransport,
}: StagesPageProps): React.ReactElement {
  const [page, setPage] = useState<PageState>({ kind: "loading" });
  const [notice, setNotice] = useState<string>();
  const [createName, setCreateName] = useState("");
  const [createSortOrder, setCreateSortOrder] = useState("");
  const [creating, setCreating] = useState(false);
  const [renamingID, setRenamingID] = useState<number>();
  const [archivingID, setArchivingID] = useState<number>();
  const [renameNames, setRenameNames] = useState<Record<number, string>>({});
  const writeInFlight = useRef(false);
  const outcomeUnknown = useRef(false);
  const canWrite = role === "admin" || role === "ops";

  const load = useCallback(async () => {
    setPage({ kind: "loading" });
    setNotice(undefined);
    const result = await loadStages(transport);
    if (result.status === "unauthenticated") {
      onUnauthenticated?.();
      return;
    }
    if (result.status === "loaded") {
      outcomeUnknown.current = false;
      setPage({ kind: "ready", items: result.items });
      return;
    }
    setPage({ kind: "unavailable" });
  }, [onUnauthenticated, transport]);

  useEffect(() => {
    void load();
  }, [load]);

  const csrfToken = (): string | undefined => {
    try {
      return readCSRFCookie(readCookie());
    } catch {
      return undefined;
    }
  };

  const handleFailure = (failure: StageMutationFailure) => {
    if (failure === "unavailable") outcomeUnknown.current = true;
    setNotice(mutationMessages[failure]);
    if (failure === "unauthenticated") onUnauthenticated?.();
  };

  const requestCreate = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (creating || writeInFlight.current || outcomeUnknown.current || !canWrite) return;
    if (createName.trim() === "") {
      setNotice("阶段名称不能为空。");
      return;
    }
    const sortOrder = parseSortOrder(createSortOrder);
    if (Number.isNaN(sortOrder)) {
      setNotice("排序值必须是安全整数。");
      return;
    }
	const token = csrfToken();
	if (!token) {
      setNotice("安全令牌缺失，未发送新增请求。");
		return;
	}
	const idempotencyKey = newStageIdempotencyKey("create");
	if (!idempotencyKey) {
		setNotice("安全随机源不可用，未发送新增请求。");
		return;
	}

    writeInFlight.current = true;
    setCreating(true);
    setNotice(undefined);
    try {
      const result = await submitStageCreate(
        transport,
        {
          name: createName,
          ...(sortOrder === undefined ? {} : { sort_order: sortOrder }),
        },
        token,
        idempotencyKey,
      );
      if (result.status !== "created") {
        handleFailure(result.status);
        return;
      }

      const refreshed = await loadStages(transport);
      if (refreshed.status === "unauthenticated") {
        onUnauthenticated?.();
        return;
      }
      if (
        refreshed.status !== "loaded" ||
        !refreshed.items.some((item) => sameStage(item, result.stage))
      ) {
        outcomeUnknown.current = true;
        setNotice(mutationMessages.unavailable);
        return;
      }
      setPage({ kind: "ready", items: refreshed.items });
      setCreateName("");
      setCreateSortOrder("");
      setNotice("阶段已新增并已由列表回读确认。");
    } finally {
      writeInFlight.current = false;
      setCreating(false);
    }
  };

  const requestRename = async (
    event: React.FormEvent<HTMLFormElement>,
    stage: StageRecord,
  ) => {
    event.preventDefault();
    if (renamingID !== undefined || writeInFlight.current || outcomeUnknown.current || !canWrite) return;
    const name = renameNames[stage.id] ?? stage.name;
    if (name.trim() === "") {
      setNotice("阶段名称不能为空。");
      return;
    }
	const token = csrfToken();
    if (!token) {
      setNotice("安全令牌缺失，未发送改名请求。");
		return;
	}
	const idempotencyKey = newStageIdempotencyKey("rename");
	if (!idempotencyKey) {
		setNotice("安全随机源不可用，未发送改名请求。");
		return;
	}

    writeInFlight.current = true;
    setRenamingID(stage.id);
    setNotice(undefined);
    try {
      const result = await submitStageRename(
        transport,
        stage.id,
        { name },
        token,
        idempotencyKey,
      );
      if (result.status !== "renamed") {
        handleFailure(result.status);
        return;
      }
      const refreshed = await loadStages(transport);
      if (refreshed.status === "unauthenticated") {
        onUnauthenticated?.();
        return;
      }
      if (
        refreshed.status !== "loaded" ||
        !refreshed.items.some((item) => sameStage(item, result.stage))
      ) {
        outcomeUnknown.current = true;
        setNotice(mutationMessages.unavailable);
        return;
      }
      setPage({ kind: "ready", items: refreshed.items });
      setRenameNames((current) => ({
        ...current,
        [result.stage.id]: result.stage.name,
      }));
      setNotice("阶段已改名并已由列表回读确认。");
    } finally {
      writeInFlight.current = false;
      setRenamingID(undefined);
    }
  };

  const requestReorder = async (items: readonly StageRecord[], from: number, to: number) => {
    if (writeInFlight.current || outcomeUnknown.current || !canWrite || !transport.reorder) return;
    const token = csrfToken();
    const idempotencyKey = newStageIdempotencyKey("reorder");
    if (!token || !idempotencyKey) {
      setNotice(!token ? "安全令牌缺失，未发送排序请求。" : "安全随机源不可用，未发送排序请求。");
      return;
    }
    const reordered = [...items];
    const [moved] = reordered.splice(from, 1);
    reordered.splice(to, 0, moved);
    const ids = reordered.map((item) => item.id);
    writeInFlight.current = true;
    setNotice(undefined);
    try {
      const result = await submitStageReorder(transport, ids, token, idempotencyKey);
      if (result.status !== "reordered") {
        handleFailure(result.status);
        return;
      }
      const refreshed = await loadStages(transport);
      if (refreshed.status === "unauthenticated") {
        onUnauthenticated?.();
        return;
      }
      if (
        refreshed.status !== "loaded" ||
        refreshed.items.length !== result.items.length ||
        refreshed.items.some((item, index) => !sameStage(item, result.items[index]))
      ) {
        outcomeUnknown.current = true;
        setNotice(mutationMessages.unavailable);
        return;
      }
      setPage({ kind: "ready", items: refreshed.items });
      setNotice("阶段排序已由列表回读确认。");
    } finally {
      writeInFlight.current = false;
    }
  };

  const requestArchive = async (stage: StageRecord) => {
    if (writeInFlight.current || outcomeUnknown.current || !canWrite || !transport.archive) return;
    if (typeof window === "undefined" || !window.confirm(`确认归档本地阶段“${stage.name}”？仍被客户引用时服务端会拒绝。`)) return;
    const token = csrfToken();
    const idempotencyKey = newStageIdempotencyKey("archive");
    if (!token || !idempotencyKey) {
      setNotice(!token ? "安全令牌缺失，未发送归档请求。" : "安全随机源不可用，未发送归档请求。");
      return;
    }
    writeInFlight.current = true;
    setArchivingID(stage.id);
    setNotice(undefined);
    try {
      const result = await submitStageArchive(transport, stage.id, token, idempotencyKey);
      if (result.status !== "archived") {
        handleFailure(result.status);
        return;
      }
      const refreshed = await loadStages(transport);
      if (refreshed.status === "unauthenticated") {
        onUnauthenticated?.();
        return;
      }
      if (refreshed.status !== "loaded" || refreshed.items.some((item) => item.id === stage.id)) {
        outcomeUnknown.current = true;
        setNotice(mutationMessages.unavailable);
        return;
      }
      setPage({ kind: "ready", items: refreshed.items });
      setNotice("阶段已归档并已由列表回读确认。");
    } finally {
      writeInFlight.current = false;
      setArchivingID(undefined);
    }
  };

  return (
    <section className="stages-page" aria-labelledby="app-title">
      <div className="stages-page__heading">
        <div>
          <p className="route-card__eyebrow">客户配置</p>
          <h1 id="app-title">阶段管理</h1>
          <p>查看全局客户阶段；列表顺序由服务端统一维护。</p>
        </div>
        {page.kind !== "loading" && (
          <button className="button-secondary" type="button" onClick={load}>
            刷新列表
          </button>
        )}
      </div>

      {notice && (
        <p className="stages-page__notice" role="alert">
          {notice}
        </p>
      )}

      {canWrite && (
        <form className="stage-create" onSubmit={requestCreate}>
          <fieldset disabled={creating || outcomeUnknown.current || renamingID !== undefined || archivingID !== undefined}>
            <legend>新增阶段</legend>
            <label>
              阶段名称
              <input
                name="stage-name"
                value={createName}
                onChange={(event) => setCreateName(event.currentTarget.value)}
              />
            </label>
            <label>
              排序值（可选）
              <input
                inputMode="numeric"
                name="stage-sort-order"
                value={createSortOrder}
                onChange={(event) =>
                  setCreateSortOrder(event.currentTarget.value)
                }
              />
            </label>
            <button type="submit">{creating ? "正在新增…" : "新增阶段"}</button>
          </fieldset>
        </form>
      )}

      {page.kind === "loading" && (
        <p className="stages-page__state" role="status">
          正在读取阶段…
        </p>
      )}
      {page.kind === "unavailable" && (
        <div className="stages-page__state" role="alert">
          <p>阶段服务暂时不可用。</p>
          <button type="button" onClick={load}>
            重试
          </button>
        </div>
      )}
      {page.kind === "ready" && page.items.length === 0 && (
        <p className="stages-page__state" role="status">
          暂无阶段。
        </p>
      )}
      {page.kind === "ready" && page.items.length > 0 && (
        <ol className="stage-list">
          {page.items.map((stage, index) => (
            <li className="stage-list__item" key={stage.id}>
              <div>
                <strong>{stage.name}</strong>
                <span>排序 {stage.sortOrder}</span>
              </div>
              {canWrite && (
                <form onSubmit={(event) => requestRename(event, stage)}>
                  <label htmlFor={`stage-name-${stage.id}`}>改名</label>
                  <input
                    id={`stage-name-${stage.id}`}
                    value={renameNames[stage.id] ?? stage.name}
                    onChange={(event) =>
                      setRenameNames((current) => ({
                        ...current,
                        [stage.id]: event.currentTarget.value,
                      }))
                    }
                  />
                  <button type="submit" disabled={renamingID !== undefined || creating || archivingID !== undefined || outcomeUnknown.current}>
                    {renamingID === stage.id ? "正在保存…" : "保存改名"}
                  </button>
                  {transport.reorder && (
                    <>
                      <button type="button" disabled={index === 0 || archivingID !== undefined || outcomeUnknown.current} onClick={() => void requestReorder(page.items, index, index - 1)}>上移</button>
                      <button type="button" disabled={index === page.items.length - 1 || archivingID !== undefined || outcomeUnknown.current} onClick={() => void requestReorder(page.items, index, index + 1)}>下移</button>
                    </>
                  )}
                  {transport.archive && (
                    <button type="button" disabled={archivingID !== undefined || outcomeUnknown.current} onClick={() => void requestArchive(stage)}>
                      {archivingID === stage.id ? "正在归档…" : "归档阶段"}
                    </button>
                  )}
                </form>
              )}
            </li>
          ))}
        </ol>
      )}
      <CRMTagCatalogPage role={role} transport={crmTagTransport} readCookie={readCookie} onUnauthenticated={onUnauthenticated} />
    </section>
  );
}
