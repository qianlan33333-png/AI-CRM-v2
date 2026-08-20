/* eslint-disable no-unused-vars -- named callback parameters document the injected transport boundary. */
import React, { useEffect, useRef, useState } from "react";
import {
  archiveLegacyHXCSendConfig,
  getLegacyHXCSendConfig,
  reorderLegacyHXCSendConfigs,
  upsertLegacyHXCSendConfig,
} from "./api/generated/health";
import { readCSRFCookie } from "./auth";
import {
  loadHXCSenders,
  type HXCSenderConfig,
  type HXCSenderReadModel,
  type HXCSenderTransport,
} from "./hxc-sender";

export type HXCSenderManagerResult =
  | { readonly status: 200; readonly data: unknown }
  | { readonly status: 400 | 401 | 403 | 409 | 503; readonly data: unknown };

export interface HXCSenderManagerTransport {
  readonly save: (
    body: {
      id: string;
      sender_userid: string;
      display_name: string;
      priority: number;
      is_active: boolean;
    },
    options: RequestInit,
  ) => Promise<HXCSenderManagerResult>;
  readonly reorder: (
    body: { ids: string[] },
    options: RequestInit,
  ) => Promise<HXCSenderManagerResult>;
  readonly archive: (
    senderUserID: string,
    options: RequestInit,
  ) => Promise<HXCSenderManagerResult>;
  readonly reread: HXCSenderTransport["read"];
}

export const generatedHXCSenderManagerTransport: HXCSenderManagerTransport = {
  save: upsertLegacyHXCSendConfig,
  reorder: reorderLegacyHXCSendConfigs,
  archive: (senderUserID, options) =>
    archiveLegacyHXCSendConfig(encodeURIComponent(senderUserID), options),
  reread: getLegacyHXCSendConfig,
};

type Draft = {
  id: string;
  sender: string;
  name: string;
  priority: string;
  active: boolean;
};
const blank = (): Draft => ({
  id: "",
  sender: "",
  name: "",
  priority: "0",
  active: true,
});

function valid(value: Draft): boolean {
  const priority = Number(value.priority);
  return (
    value.id.trim() === value.id &&
    value.id.length > 0 &&
    value.id.length <= 200 &&
    value.sender.trim() === value.sender &&
    value.sender.length > 0 &&
    value.sender.length <= 200 &&
    value.name.trim() === value.name &&
    value.name.length <= 200 &&
    Number.isSafeInteger(priority) &&
    priority >= 0 &&
    priority <= 100000
  );
}

function idempotencyKey(operation: string): string | undefined {
  try {
    return `hxc-sender-${operation}-${globalThis.crypto.randomUUID()}`.slice(
      0,
      128,
    );
  } catch {
    return undefined;
  }
}

function requestOptions(csrf: string, key: string): RequestInit {
  return {
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrf, "Idempotency-Key": key },
  };
}

export function HXCSenderManager({
  role,
  items,
  transport = generatedHXCSenderManagerTransport,
  readCookie = () => (typeof document === "undefined" ? "" : document.cookie),
  onUnauthenticated,
  onConfirmed,
}: {
  readonly role: "admin" | "ops" | "sales";
  readonly items: readonly HXCSenderConfig[];
  readonly transport?: HXCSenderManagerTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
  readonly onConfirmed?: (model: HXCSenderReadModel) => void;
}): React.ReactElement | null {
  const [draft, setDraft] = useState(blank);
  const [busy, setBusy] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [notice, setNotice] = useState<string>();
  const active = useRef<symbol>();

  useEffect(
    () => () => {
      active.current = undefined;
    },
    [],
  );
  if (role !== "admin") return null;

  const write = async (
    operation: string,
    run: (options: RequestInit) => Promise<HXCSenderManagerResult>,
  ) => {
    if (busy || unknown || active.current) return;
    const csrf = readCSRFCookie(readCookie());
    const key = idempotencyKey(operation);
    if (!csrf || !key) {
      setNotice("安全令牌或幂等键缺失，未发送本地配置请求。");
      return;
    }
    const token = Symbol("hxc-write");
    active.current = token;
    setBusy(true);
    setNotice(undefined);
    try {
      const response = await run(requestOptions(csrf, key));
      if (active.current !== token) return;
      if (response.status === 401) {
        onUnauthenticated?.();
        setNotice("登录状态已失效。");
        return;
      }
      if (
        response.status === 400 ||
        response.status === 403 ||
        response.status === 409
      ) {
        setNotice("本地配置未确认，请刷新投影后修正。");
        return;
      }
      if (response.status !== 200) {
        setUnknown(true);
        setNotice("本地配置结果未知，已锁定以避免重复写入。");
        return;
      }
      const fresh = await loadHXCSenders({ read: transport.reread });
      if (active.current !== token) return;
      if (fresh.status !== "loaded") {
        if (fresh.status === "unauthenticated") onUnauthenticated?.();
        setUnknown(true);
        setNotice("本地配置回读未确认，已锁定以避免重复写入。");
        return;
      }
      onConfirmed?.(fresh.model);
      setDraft(blank());
      setNotice("本地发件人配置已回读确认。");
    } catch {
      if (active.current === token) {
        setUnknown(true);
        setNotice("本地配置结果未知，已锁定以避免重复写入。");
      }
    } finally {
      if (active.current === token) {
        active.current = undefined;
        setBusy(false);
      }
    }
  };

  const saveDraft = () => {
    if (!valid(draft)) {
      setNotice("请填写有效的本地配置字段。");
      return;
    }
    void write("save", (options) =>
      transport.save(
        {
          id: draft.id,
          sender_userid: draft.sender,
          display_name: draft.name,
          priority: Number(draft.priority),
          is_active: draft.active,
        },
        options,
      ),
    );
  };

  const saveItem = (item: HXCSenderConfig, activeValue: boolean) =>
    void write("active", (options) =>
      transport.save(
        {
          id: item.id,
          sender_userid: item.senderUserID,
          display_name: item.displayName,
          priority: item.priority,
          is_active: activeValue,
        },
        options,
      ),
    );

  const reorder = (from: number, to: number) => {
    if (to < 0 || to >= items.length) return;
    const ids = items.map((item) => item.id);
    const [moved] = ids.splice(from, 1);
    if (!moved) return;
    ids.splice(to, 0, moved);
    void write("reorder", (options) => transport.reorder({ ids }, options));
  };

  const archive = (senderUserID: string) => {
    if (!globalThis.confirm(`确认归档本地发件人配置 ${senderUserID}？`)) return;
    void write("archive", (options) =>
      transport.archive(senderUserID, options),
    );
  };

  return (
    <section aria-label="本地发件人管理">
      <h2>本地发件人管理</h2>
      <p>仅修改本地配置；不会同步企微目录或触发外发。</p>
      {notice ? <p role="status">{notice}</p> : null}
      {unknown ? (
        <p role="alert">写入状态待人工确认，请重新载入页面后再操作。</p>
      ) : (
        <>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              saveDraft();
            }}
          >
            <label>
              配置 ID
              <input
                value={draft.id}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    id: event.currentTarget.value,
                  }))
                }
                disabled={busy}
              />
            </label>
            <label>
              本地员工 ID
              <input
                value={draft.sender}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    sender: event.currentTarget.value,
                  }))
                }
                disabled={busy}
              />
            </label>
            <label>
              显示名
              <input
                value={draft.name}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    name: event.currentTarget.value,
                  }))
                }
                disabled={busy}
              />
            </label>
            <label>
              优先级
              <input
                inputMode="numeric"
                value={draft.priority}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    priority: event.currentTarget.value,
                  }))
                }
                disabled={busy}
              />
            </label>
            <label>
              <input
                type="checkbox"
                checked={draft.active}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    active: event.currentTarget.checked,
                  }))
                }
                disabled={busy}
              />
              启用
            </label>
            <button type="submit" disabled={busy}>
              保存本地配置
            </button>
          </form>
          <ul>
            {items.map((item, index) => (
              <li key={item.id}>
                {item.displayName || item.senderUserID}（{item.senderUserID}，
                {item.isActive ? "启用" : "停用"}）
                <button
                  type="button"
                  disabled={busy}
                  onClick={() =>
                    setDraft({
                      id: item.id,
                      sender: item.senderUserID,
                      name: item.displayName,
                      priority: String(item.priority),
                      active: item.isActive,
                    })
                  }
                >
                  编辑
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => saveItem(item, !item.isActive)}
                >
                  {item.isActive ? "停用" : "启用"}
                </button>
                <button
                  type="button"
                  disabled={busy || index === 0}
                  onClick={() => reorder(index, index - 1)}
                >
                  上移
                </button>
                <button
                  type="button"
                  disabled={busy || index === items.length - 1}
                  onClick={() => reorder(index, index + 1)}
                >
                  下移
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => archive(item.senderUserID)}
                >
                  归档
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}
