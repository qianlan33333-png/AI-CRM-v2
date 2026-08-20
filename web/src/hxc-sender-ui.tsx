import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  HXCSenderManager,
  type HXCSenderManagerTransport,
} from "./hxc-sender-manager";
import {
  filterHXCSenders,
  generatedHXCSenderTransport,
  loadHXCSenders,
  type HXCSenderReadModel,
  type HXCSenderResult,
  type HXCSenderStatusFilter,
  type HXCSenderTransport,
} from "./hxc-sender";

export type HXCSenderState =
  | { readonly kind: "loading"; readonly previous?: HXCSenderReadModel }
  | { readonly kind: "ready"; readonly model: HXCSenderReadModel }
  | {
      readonly kind: "error";
      readonly failure: Exclude<
        HXCSenderResult,
        { readonly status: "loaded" }
      >["status"];
      readonly previous?: HXCSenderReadModel;
    };

export interface HXCSenderReadController {
  readonly generation: { current: number };
  readonly inFlight: { current: symbol | undefined };
  readonly verified: { current: HXCSenderReadModel | undefined };
  // eslint-disable-next-line no-unused-vars -- named callback parameter documents the state boundary.
  readonly onState: (state: HXCSenderState) => void;
  readonly onUnauthenticated?: () => void;
}

export function invalidateHXCSenderRead(
  controller: HXCSenderReadController,
): void {
  controller.generation.current += 1;
  controller.inFlight.current = undefined;
}

export async function startHXCSenderRead(
  controller: HXCSenderReadController,
  transport: HXCSenderTransport,
): Promise<void> {
  if (controller.inFlight.current !== undefined) return;
  const token = Symbol("hxc-sender-read");
  controller.inFlight.current = token;
  const generation = ++controller.generation.current;
  controller.onState({
    kind: "loading",
    previous: controller.verified.current,
  });
  try {
    const result = await loadHXCSenders(transport);
    if (
      generation !== controller.generation.current ||
      controller.inFlight.current !== token
    )
      return;
    if (result.status === "loaded") {
      controller.verified.current = result.model;
      controller.onState({ kind: "ready", model: result.model });
      return;
    }
    if (result.status === "unauthenticated") controller.onUnauthenticated?.();
    controller.onState({
      kind: "error",
      failure: result.status,
      previous: controller.verified.current,
    });
  } finally {
    if (controller.inFlight.current === token)
      controller.inFlight.current = undefined;
  }
}

const messages: Record<
  Exclude<HXCSenderResult, { readonly status: "loaded" }>["status"],
  string
> = {
  unavailable: "本地发件人投影暂不可用。",
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有访问权限。",
  invalid: "本地发件人投影响应无效。",
};

function memberStatus(member: {
  readonly isSender: boolean;
  readonly isActive: boolean;
}): string {
  if (!member.isSender) return "目录成员";
  return member.isActive ? "已启用" : "待清理";
}

export function HXCSenderView({
  state,
  keyword,
  status,
  onKeyword,
  onStatus,
  onRefresh,
}: {
  readonly state: HXCSenderState;
  readonly keyword: string;
  readonly status: HXCSenderStatusFilter;
  // eslint-disable-next-line no-unused-vars -- named callback parameter documents the filter boundary.
  readonly onKeyword: (value: string) => void;
  // eslint-disable-next-line no-unused-vars -- named callback parameter documents the filter boundary.
  readonly onStatus: (value: HXCSenderStatusFilter) => void;
  readonly onRefresh: () => void;
}): React.ReactElement {
  const model = state.kind === "ready" ? state.model : state.previous;
  const members = model ? filterHXCSenders(model, keyword, status) : [];
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">只读本地投影</p>
      <h1 id="app-title">HXC 发件人配置</h1>
      <p>
        仅展示本地员工投影；不会读取企业微信目录、发送消息或执行其他外部调用。
      </p>
      {model ? (
        <>
          <p>{model.warning}</p>
          <dl>
            <dt>可用目录</dt>
            <dd>{model.directoryCount}</dd>
            <dt>已配置发件人</dt>
            <dd>{model.senderCount}</dd>
            <dt>活跃发件人</dt>
            <dd>{model.activeSenderCount}</dd>
            <dt>目录更新时间</dt>
            <dd>{model.lastSyncedAt || "无可用成员"}</dd>
          </dl>
          {model.emptyState ? (
            <p role="status">当前没有可用本地成员。</p>
          ) : (
            <>
              <p>
                <label>
                  本地搜索（企业微信 ID 或显示名）{" "}
                  <input
                    value={keyword}
                    onChange={(event) => onKeyword(event.currentTarget.value)}
                  />
                </label>{" "}
                <label>
                  状态{" "}
                  <select
                    value={status}
                    onChange={(event) =>
                      onStatus(
                        event.currentTarget.value as HXCSenderStatusFilter,
                      )
                    }
                  >
                    <option value="all">全部</option>
                    <option value="active">已启用</option>
                    <option value="inactive">待清理</option>
                    <option value="directory">仅目录成员</option>
                  </select>
                </label>
              </p>
              <p>
                当前本地筛选 {members.length} / {model.members.length} 名成员。
              </p>
              {members.length === 0 ? (
                <p role="status">没有符合当前本地筛选条件的成员。</p>
              ) : (
                <table>
                  <thead>
                    <tr>
                      <th>企业微信 ID</th>
                      <th>显示名</th>
                      <th>优先级</th>
                      <th>状态</th>
                    </tr>
                  </thead>
                  <tbody>
                    {members.map((member) => (
                      <tr key={member.wecomUserID}>
                        <td>{member.wecomUserID}</td>
                        <td>{member.displayName || "—"}</td>
                        <td>{member.priority}</td>
                        <td>{memberStatus(member)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </>
          )}
        </>
      ) : null}
      {state.kind === "loading" ? (
        <p role="status">正在读取本地发件人投影。</p>
      ) : null}
      {state.kind === "error" ? (
        <p role="alert">{messages[state.failure]}</p>
      ) : null}
      <p>
        <button
          type="button"
          disabled={state.kind === "loading"}
          onClick={onRefresh}
        >
          手动刷新本地投影
        </button>
      </p>
    </section>
  );
}

export function HXCSenderPage({
  role,
  transport = generatedHXCSenderTransport,
  onUnauthenticated,
  managerTransport,
  readCookie,
}: {
  readonly role: "admin" | "ops" | "sales";
  readonly transport?: HXCSenderTransport;
  readonly onUnauthenticated?: () => void;
  readonly managerTransport?: HXCSenderManagerTransport;
  readonly readCookie?: () => string;
}): React.ReactElement {
  const canRead = role === "admin";
  const generation = useRef(0);
  const inFlight = useRef<symbol>();
  const verified = useRef<HXCSenderReadModel>();
  const [state, setState] = useState<HXCSenderState>({ kind: "loading" });
  const [keyword, setKeyword] = useState("");
  const [status, setStatus] = useState<HXCSenderStatusFilter>("all");
  const load = useCallback(
    () =>
      startHXCSenderRead(
        {
          generation,
          inFlight,
          verified,
          onState: setState,
          onUnauthenticated,
        },
        transport,
      ),
    [onUnauthenticated, transport],
  );

  useEffect(() => {
    if (canRead) void load();
    return () => {
      invalidateHXCSenderRead({
        generation,
        inFlight,
        verified,
        onState: setState,
        onUnauthenticated,
      });
    };
  }, [canRead, load, onUnauthenticated]);

  if (!canRead)
    return (
      <section className="route-card" aria-labelledby="app-title">
        <h1 id="app-title">HXC 发件人配置</h1>
        <p>当前账号没有访问权限。</p>
      </section>
    );
  return (
    <>
      <HXCSenderView
        state={state}
        keyword={keyword}
        status={status}
        onKeyword={setKeyword}
        onStatus={setStatus}
        onRefresh={() => {
          void load();
        }}
      />
      {state.kind === "ready" ? (
        <HXCSenderManager
          role={role}
          items={state.model.sendConfigs}
          transport={managerTransport}
          readCookie={readCookie}
          onUnauthenticated={onUnauthenticated}
          onConfirmed={(model) => {
            verified.current = model;
            setState({ kind: "ready", model });
          }}
        />
      ) : null}
    </>
  );
}
