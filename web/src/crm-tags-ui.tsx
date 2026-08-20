/* eslint-disable no-unused-vars -- generic mutation callback names document local transport ownership. */
import React, { useCallback, useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  crmTagFailure,
  crmTagHeaders,
  crmTagIdempotencyKey,
  generatedCRMTagTransport,
  loadCRMTagCatalog,
  type CRMTagCatalog,
  type CRMTagFailure,
  type CRMTagRole,
  type CRMTagTransport,
} from "./crm-tags";

export interface CRMTagCatalogPageProps {
  readonly role: CRMTagRole;
  readonly transport?: CRMTagTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}
const message: Record<CRMTagFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有本地标签目录操作权限。",
  not_found: "本地标签目录项已不存在。",
  conflict: "标签目录已变化或仍被客户引用。",
  invalid: "名称或排序请求不符合本地目录合同。",
  unknown: "结果尚未确认；为避免重复写入，目录已锁定，请刷新后人工核对。",
};
function cookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

export function CRMTagCatalogPage({
  role,
  transport = generatedCRMTagTransport,
  readCookie = cookie,
  onUnauthenticated,
}: CRMTagCatalogPageProps): React.ReactElement {
  const canWrite = role === "admin" || role === "ops";
  const [catalog, setCatalog] = useState<CRMTagCatalog>();
  const [notice, setNotice] = useState<string>();
  const [groupName, setGroupName] = useState("");
  const [firstTagName, setFirstTagName] = useState("");
  const [tagNames, setTagNames] = useState<Record<number, string>>({});
  const [mutating, setMutating] = useState(false);
  const [locked, setLocked] = useState(false);
  const lockedRef = useRef(false);
  const lifetime = useRef(0);
  const readSerial = useRef(0);
  const readFlight = useRef<{
    readonly lifetime: number;
    readonly serial: number;
    readonly promise: Promise<CRMTagCatalog | undefined>;
  }>();
  const mutationToken = useRef<symbol>();
  const read = useCallback(
    (fresh = false): Promise<CRMTagCatalog | undefined> => {
      const activeLifetime = lifetime.current;
      const current = readFlight.current;
      if (!fresh && current?.lifetime === activeLifetime) {
        return current.promise;
      }
      const serial = ++readSerial.current;
      const promise = loadCRMTagCatalog(transport ?? {})
        .then((result) => {
          if (
            lifetime.current !== activeLifetime ||
            readFlight.current?.serial !== serial
          ) {
            return undefined;
          }
          if (result.status === "loaded") {
            setCatalog(result.catalog);
            return result.catalog;
          }
          if (result.status === "unauthenticated") onUnauthenticated?.();
          setNotice(message[result.status]);
          return undefined;
        })
        .finally(() => {
          if (
            readFlight.current?.lifetime === activeLifetime &&
            readFlight.current.serial === serial
          ) {
            readFlight.current = undefined;
          }
        });
      readFlight.current = { lifetime: activeLifetime, serial, promise };
      return promise;
    },
    [onUnauthenticated, transport],
  );
  useEffect(() => {
    const activeLifetime = ++lifetime.current;
    readSerial.current += 1;
    readFlight.current = undefined;
    if (lockedRef.current) {
      setLocked(true);
      setMutating(false);
      setNotice(message.unknown);
    }
    if (canWrite) void read(true);
    return () => {
      if (lifetime.current === activeLifetime) {
        lifetime.current += 1;
        readSerial.current += 1;
        readFlight.current = undefined;
        if (mutationToken.current !== undefined) {
          mutationToken.current = undefined;
          lockedRef.current = true;
        }
      }
    };
  }, [canWrite, read]);
  const mutate = async (
    call: (options: RequestInit) => Promise<{ readonly status: number }>,
    verify: (next: CRMTagCatalog) => boolean,
  ): Promise<boolean> => {
    if (!canWrite || mutationToken.current !== undefined || lockedRef.current)
      return false;
    let csrf: string | undefined;
    try {
      csrf = readCSRFCookie(readCookie());
    } catch {
      csrf = undefined;
    }
    const key = crmTagIdempotencyKey();
    if (!csrf || !key) {
      setNotice(
        !csrf
          ? "安全令牌缺失，未发送目录写请求。"
          : "安全随机源不可用，未发送目录写请求。",
      );
      return false;
    }
    const token = Symbol("crm-tag-mutation");
    const activeLifetime = lifetime.current;
    mutationToken.current = token;
    setMutating(true);
    setNotice(undefined);
    try {
      const result = await call(crmTagHeaders(csrf, key));
      if (
        mutationToken.current !== token ||
        lifetime.current !== activeLifetime
      ) {
        return false;
      }
      const failure = crmTagFailure(result.status);
      if (result.status < 200 || result.status >= 300) {
        if (failure === "unknown") {
          lockedRef.current = true;
          setLocked(true);
        }
        setNotice(message[failure]);
        if (failure === "unauthenticated") onUnauthenticated?.();
        return false;
      }
      const reread = await read(true);
      if (
        mutationToken.current !== token ||
        lifetime.current !== activeLifetime
      ) {
        return false;
      }
      if (!reread || !verify(reread)) {
        lockedRef.current = true;
        setLocked(true);
        setNotice(message.unknown);
        return false;
      }
      setNotice("本地目录操作已由列表回读确认。");
      return true;
    } catch {
      if (
        mutationToken.current === token &&
        lifetime.current === activeLifetime
      ) {
        lockedRef.current = true;
        setLocked(true);
        setNotice(message.unknown);
      }
      return false;
    } finally {
      if (mutationToken.current === token) {
        mutationToken.current = undefined;
        if (lifetime.current === activeLifetime) setMutating(false);
      }
    }
  };
  if (!canWrite)
    return (
      <section className="crm-tags-page">
        <h2>CRM 本地标签目录</h2>
        <p role="alert">当前账号没有本地标签目录访问权限。</p>
      </section>
    );
  if (!transport?.list)
    return (
      <section className="crm-tags-page">
        <h2>CRM 本地标签目录</h2>
        <p role="status">本地目录客户端尚未接入；未发送任何请求。</p>
      </section>
    );
  return (
    <section className="crm-tags-page" aria-labelledby="crm-tags-title">
      <h2 id="crm-tags-title">CRM 本地标签目录</h2>
      <p>仅管理 CRM 本地分类；不触发企微同步、在线目录或 Provider 调用。</p>
      {notice && <p role="alert">{notice}</p>}
      <button type="button" disabled={mutating} onClick={() => void read()}>
        刷新目录
      </button>
      {transport.createGroup && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            const name = groupName.trim();
            const first = firstTagName.trim();
            if (!name || !first) {
              setNotice("标签组和首个标签名称都不能为空。");
              return;
            }
            void mutate(
              (options) =>
                transport.createGroup!(
                  { name, first_tag_name: first },
                  options,
                ),
              (next) =>
                next.groups.some((group) => group.name === name) &&
                next.tags.some(
                  (tag) => tag.groupName === name && tag.name === first,
                ),
            ).then((confirmed) => {
              if (confirmed) {
                setGroupName("");
                setFirstTagName("");
              }
            });
          }}
        >
          <label>
            新建标签组
            <input
              value={groupName}
              onChange={(event) => setGroupName(event.currentTarget.value)}
            />
          </label>
          <label>
            首个标签
            <input
              value={firstTagName}
              onChange={(event) => setFirstTagName(event.currentTarget.value)}
            />
          </label>
          <button type="submit" disabled={locked || mutating}>
            新建
          </button>
        </form>
      )}
      {catalog && (
        <ol>
          {catalog.groups.map((group, index) => (
            <li key={group.id}>
              <strong>{group.name}</strong>
              {transport.updateGroup && (
                <button
                  type="button"
                  disabled={locked || mutating}
                  onClick={() => {
                    const name = window.prompt("本地标签组名称", group.name);
                    if (name?.trim())
                      void mutate(
                        (options) =>
                          transport.updateGroup!(
                            group.id,
                            { name: name.trim() },
                            options,
                          ),
                        (next) =>
                          next.groups.some(
                            (item) =>
                              item.id === group.id && item.name === name.trim(),
                          ),
                      );
                  }}
                >
                  改名
                </button>
              )}
              {transport.reorderGroups && (
                <>
                  <button
                    type="button"
                    disabled={locked || mutating || index === 0}
                    onClick={() => {
                      const ids = catalog.groups.map((item) => item.id);
                      [ids[index - 1], ids[index]] = [
                        ids[index],
                        ids[index - 1],
                      ];
                      void mutate(
                        (options) => transport.reorderGroups!({ ids }, options),
                        (next) =>
                          next.groups.map((item) => item.id).join(",") ===
                          ids.join(","),
                      );
                    }}
                  >
                    上移
                  </button>
                  <button
                    type="button"
                    disabled={
                      locked || mutating || index === catalog.groups.length - 1
                    }
                    onClick={() => {
                      const ids = catalog.groups.map((item) => item.id);
                      [ids[index + 1], ids[index]] = [
                        ids[index],
                        ids[index + 1],
                      ];
                      void mutate(
                        (options) => transport.reorderGroups!({ ids }, options),
                        (next) =>
                          next.groups.map((item) => item.id).join(",") ===
                          ids.join(","),
                      );
                    }}
                  >
                    下移
                  </button>
                </>
              )}
              {transport.archiveGroup && (
                <button
                  type="button"
                  disabled={locked || mutating}
                  onClick={() => {
                    if (
                      window.confirm(
                        `确认归档本地标签组“${group.name}”？仍被客户引用时会被拒绝。`,
                      )
                    )
                      void mutate(
                        (options) => transport.archiveGroup!(group.id, options),
                        (next) =>
                          !next.groups.some((item) => item.id === group.id),
                      );
                  }}
                >
                  归档
                </button>
              )}
              <ul>
                {catalog.tags
                  .filter((tag) => tag.groupID === group.id)
                  .map((tag, tagIndex, groupTags) => (
                    <li key={tag.id}>
                      {tag.name}
                      {transport.updateTag && (
                        <button
                          type="button"
                          disabled={locked || mutating}
                          onClick={() => {
                            const name = window.prompt(
                              "本地标签名称",
                              tag.name,
                            );
                            if (name?.trim())
                              void mutate(
                                (options) =>
                                  transport.updateTag!(
                                    tag.id,
                                    { name: name.trim() },
                                    options,
                                  ),
                                (next) =>
                                  next.tags.some(
                                    (item) =>
                                      item.id === tag.id &&
                                      item.name === name.trim(),
                                  ),
                              );
                          }}
                        >
                          改名
                        </button>
                      )}
                      {transport.reorderTags && (
                        <>
                          <button
                            type="button"
                            disabled={locked || mutating || tagIndex === 0}
                            onClick={() => {
                              const ids = catalog.tags.map((item) => item.id);
                              const previousID = groupTags[tagIndex - 1]?.id;
                              if (previousID === undefined) return;
                              const currentIndex = ids.indexOf(tag.id);
                              const previousIndex = ids.indexOf(previousID);
                              if (currentIndex < 0 || previousIndex < 0) return;
                              [ids[previousIndex], ids[currentIndex]] = [
                                ids[currentIndex],
                                ids[previousIndex],
                              ];
                              void mutate(
                                (options) =>
                                  transport.reorderTags!({ ids }, options),
                                (next) =>
                                  next.tags.map((item) => item.id).join(",") ===
                                  ids.join(","),
                              );
                            }}
                          >
                            标签上移
                          </button>
                          <button
                            type="button"
                            disabled={
                              locked ||
                              mutating ||
                              tagIndex === groupTags.length - 1
                            }
                            onClick={() => {
                              const ids = catalog.tags.map((item) => item.id);
                              const nextID = groupTags[tagIndex + 1]?.id;
                              if (nextID === undefined) return;
                              const currentIndex = ids.indexOf(tag.id);
                              const nextIndex = ids.indexOf(nextID);
                              if (currentIndex < 0 || nextIndex < 0) return;
                              [ids[nextIndex], ids[currentIndex]] = [
                                ids[currentIndex],
                                ids[nextIndex],
                              ];
                              void mutate(
                                (options) =>
                                  transport.reorderTags!({ ids }, options),
                                (next) =>
                                  next.tags.map((item) => item.id).join(",") ===
                                  ids.join(","),
                              );
                            }}
                          >
                            标签下移
                          </button>
                        </>
                      )}
                      {transport.archiveTag && (
                        <button
                          type="button"
                          disabled={locked || mutating}
                          onClick={() => {
                            if (
                              window.confirm(
                                `确认归档本地标签“${tag.name}”？仍被客户引用时会被拒绝。`,
                              )
                            )
                              void mutate(
                                (options) =>
                                  transport.archiveTag!(tag.id, options),
                                (next) =>
                                  !next.tags.some((item) => item.id === tag.id),
                              );
                          }}
                        >
                          归档
                        </button>
                      )}
                    </li>
                  ))}
              </ul>
              {transport.createTag && (
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    const name = (tagNames[group.id] ?? "").trim();
                    if (name)
                      void mutate(
                        (options) =>
                          transport.createTag!(
                            { group_id: group.id, name },
                            options,
                          ),
                        (next) =>
                          next.tags.some(
                            (tag) =>
                              tag.groupID === group.id && tag.name === name,
                          ),
                      );
                  }}
                >
                  <input
                    aria-label={`${group.name} 新标签`}
                    value={tagNames[group.id] ?? ""}
                    onChange={(event) =>
                      setTagNames((current) => ({
                        ...current,
                        [group.id]: event.currentTarget.value,
                      }))
                    }
                  />
                  <button type="submit" disabled={locked || mutating}>
                    添加标签
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
