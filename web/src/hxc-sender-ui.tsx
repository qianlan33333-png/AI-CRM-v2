import React, { useEffect, useState } from "react";
import { HXCSenderManager, type HXCSenderManagerTransport } from "./hxc-sender-manager";
import {
  generatedHXCSenderTransport,
  loadHXCSenders,
  type HXCSenderResult,
  type HXCSenderTransport,
} from "./hxc-sender";

export function HXCSenderPage({
  role,
  transport = generatedHXCSenderTransport,
  onUnauthenticated,
  managerTransport,
}: {
  readonly role: "admin" | "ops" | "sales";
  readonly transport?: HXCSenderTransport;
  readonly onUnauthenticated?: () => void;
  readonly managerTransport?: HXCSenderManagerTransport;
}): React.ReactElement {
  const [state, setState] = useState<HXCSenderResult>({
    status: "unavailable",
  });
  useEffect(() => {
    let active = true;
    void loadHXCSenders(transport).then((result) => {
      if (active) {
        if (result.status === "unauthenticated") onUnauthenticated?.();
        setState(result);
      }
    });
    return () => {
      active = false;
    };
  }, [transport, onUnauthenticated]);

  return <HXCSenderView role={role} state={state} managerTransport={managerTransport} onUnauthenticated={onUnauthenticated} />;
}

export function HXCSenderView({
  role,
  state,
  managerTransport,
  onUnauthenticated,
}: {
  readonly role: "admin" | "ops" | "sales";
  readonly state: HXCSenderResult;
  readonly managerTransport?: HXCSenderManagerTransport;
  readonly onUnauthenticated?: () => void;
}): React.ReactElement {
  if (role !== "admin")
    return (
      <section className="route-card">
        <h1 id="app-title">HXC 发件人配置</h1>
        <p>当前账号没有访问权限。</p>
      </section>
    );
  if (state.status !== "loaded")
    return (
      <section className="route-card">
        <h1 id="app-title">HXC 发件人配置</h1>
        <p>
          {state.status === "unavailable"
            ? "本地发件人投影暂不可用。"
            : state.status === "unauthenticated"
              ? "登录状态已失效，请重新登录。"
              : state.status === "forbidden"
                ? "当前账号没有访问权限。"
                : "本地发件人投影响应无效。"}
        </p>
      </section>
    );
  return (
    <section className="route-card" aria-labelledby="app-title">
      <p className="route-card__eyebrow">只读本地投影</p>
      <h1 id="app-title">HXC 发件人配置</h1>
      <p>{state.model.warning}</p>
      <dl>
        <dt>可用目录</dt>
        <dd>{state.model.directoryCount}</dd>
        <dt>已配置发件人</dt>
        <dd>{state.model.senderCount}</dd>
        <dt>活跃发件人</dt>
        <dd>{state.model.activeSenderCount}</dd>
        <dt>目录更新时间</dt>
        <dd>{state.model.lastSyncedAt || "无可用成员"}</dd>
      </dl>
      {state.model.emptyState ? (
        <p>当前没有可用本地成员。</p>
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
            {state.model.members.map((member) => (
              <tr key={member.wecomUserID}>
                <td>{member.wecomUserID}</td>
                <td>{member.displayName}</td>
                <td>{member.priority}</td>
                <td>
                  {member.isSender
                    ? member.isActive
                      ? "已启用"
                      : "待清理"
                    : "目录成员"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <HXCSenderManager role={role} items={state.model.sendConfigs} transport={managerTransport} onUnauthenticated={onUnauthenticated} />
    </section>
  );
}
