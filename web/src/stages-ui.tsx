import React, { useCallback, useEffect, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  generatedStageTransport,
  loadStages,
  submitStageCreate,
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
  unavailable: "阶段服务暂时不可用，请稍后重试。",
};

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
}: StagesPageProps): React.ReactElement {
  const [page, setPage] = useState<PageState>({ kind: "loading" });
  const [notice, setNotice] = useState<string>();
  const [createName, setCreateName] = useState("");
  const [createSortOrder, setCreateSortOrder] = useState("");
  const [creating, setCreating] = useState(false);
  const [renamingID, setRenamingID] = useState<number>();
  const [renameNames, setRenameNames] = useState<Record<number, string>>({});
  const canWrite = role === "admin" || role === "ops";

  const load = useCallback(async () => {
    setPage({ kind: "loading" });
    setNotice(undefined);
    const result = await loadStages(transport);
    if (result.status === "unauthenticated") {
      onUnauthenticated?.();
      return;
    }
    setPage(
      result.status === "loaded"
        ? { kind: "ready", items: result.items }
        : { kind: "unavailable" },
    );
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
    setNotice(mutationMessages[failure]);
    if (failure === "unauthenticated") onUnauthenticated?.();
  };

  const requestCreate = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (creating || !canWrite) return;
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

    setCreating(true);
    setNotice(undefined);
    const result = await submitStageCreate(
      transport,
      {
        name: createName,
        ...(sortOrder === undefined ? {} : { sort_order: sortOrder }),
      },
      token,
    );
    setCreating(false);
    if (result.status !== "created") {
      handleFailure(result.status);
      return;
    }

    setCreateName("");
    setCreateSortOrder("");
    setNotice("阶段已新增。");
    const refreshed = await loadStages(transport);
    if (refreshed.status === "unauthenticated") {
      onUnauthenticated?.();
    } else if (refreshed.status === "loaded") {
      setPage({ kind: "ready", items: refreshed.items });
      setNotice("阶段已新增。");
    } else {
      setPage({ kind: "unavailable" });
    }
  };

  const requestRename = async (
    event: React.FormEvent<HTMLFormElement>,
    stage: StageRecord,
  ) => {
    event.preventDefault();
    if (renamingID !== undefined || !canWrite) return;
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

    setRenamingID(stage.id);
    setNotice(undefined);
    const result = await submitStageRename(
      transport,
      stage.id,
      { name },
      token,
    );
    setRenamingID(undefined);
    if (result.status !== "renamed") {
      handleFailure(result.status);
      return;
    }
    setPage((current) =>
      current.kind === "ready"
        ? {
            kind: "ready",
            items: current.items.map((item) =>
              item.id === result.stage.id ? result.stage : item,
            ),
          }
        : current,
    );
    setRenameNames((current) => ({
      ...current,
      [result.stage.id]: result.stage.name,
    }));
    setNotice("阶段已改名。");
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
          <fieldset disabled={creating}>
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
          {page.items.map((stage) => (
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
                  <button type="submit" disabled={renamingID !== undefined}>
                    {renamingID === stage.id ? "正在保存…" : "保存改名"}
                  </button>
                </form>
              )}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
