import React, { useEffect, useRef, useState } from "react";
import { readCSRFCookie } from "./auth";
import {
  bindConsoleIdentity,
  generatedIdentityConsoleTransport,
  resolveConsoleIdentity,
  type IdentityBindResult,
  type IdentityConsoleFailure,
  type IdentityConsoleRef,
  type IdentityConsoleRole,
  type IdentityConsoleTransport,
  type IdentityResolveResult,
} from "./identity-console";

export interface IdentityConsolePageProps {
  readonly role: IdentityConsoleRole;
  readonly transport?: IdentityConsoleTransport;
  readonly readCookie?: () => string;
  readonly onUnauthenticated?: () => void;
}

const initialRef: IdentityConsoleRef = {
  type: "phone",
  scope: "phone:e164",
  value: "",
};

function browserCookie(): string {
  return typeof document === "undefined" ? "" : document.cookie;
}

function commandKey(): string {
  const entropy = crypto.getRandomValues(new Uint8Array(18));
  return `identity-bind-${btoa(String.fromCharCode(...entropy))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "")}`;
}

const messages: Record<IdentityConsoleFailure, string> = {
  unauthenticated: "登录状态已失效，请重新登录。",
  forbidden: "当前账号没有本地身份管理权限。",
  invalid: "身份字段不符合已冻结的本地合同，未发送或未接受本次操作。",
  conflict: "身份归属已变化，未重复发送；请重新查询后核对。",
  unavailable: "本地身份服务暂不可用；绑定结果待确认，页面已锁定以避免重复写入。",
};

function resultText(result: IdentityResolveResult): string {
  if (result.status === "found") return `已定位到本地客户 OneID：${result.customerID}`;
  return result.status === "conflict" ? "该身份存在本地归属冲突，请转人工待合并核查。" : "本地客户库中未找到该身份。";
}

function bindText(result: IdentityBindResult): string {
  switch (result.status) {
    case "bound": return `已绑定到本地客户 OneID：${result.customerID}`;
    case "already_bound": return `该身份已绑定到本地客户 OneID：${result.customerID}`;
    case "rejected": return "本地规则拒绝该绑定，未写入身份归属。";
  }
}

export function IdentityConsolePage({
  role,
  transport = generatedIdentityConsoleTransport,
  readCookie = browserCookie,
  onUnauthenticated,
}: IdentityConsolePageProps): React.ReactElement {
  const permitted = role === "admin" || role === "ops";
  const [ref, setRef] = useState<IdentityConsoleRef>(initialRef);
  const [customerIDText, setCustomerIDText] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [readResult, setReadResult] = useState<IdentityResolveResult>();
  const [notice, setNotice] = useState<string>();
  const [busy, setBusy] = useState<"resolve" | "bind">();
  const [outcomeUnknown, setOutcomeUnknown] = useState(false);
  const generation = useRef(0);
  const active = useRef<{ readonly token: symbol; readonly action: "resolve" | "bind" }>();
  const invalidatedBind = useRef(false);
  const key = useRef(commandKey());
  const unauthenticatedNotified = useRef(false);

  useEffect(() => {
    const owner = ++generation.current;
    active.current = undefined;
    unauthenticatedNotified.current = false;
    if (invalidatedBind.current) {
      invalidatedBind.current = false;
      setOutcomeUnknown(true);
    }
    setBusy(undefined);
    return () => {
      if (generation.current !== owner) return;
      generation.current += 1;
      if (active.current?.action === "bind") invalidatedBind.current = true;
      active.current = undefined;
    };
  }, [transport, role, onUnauthenticated]);

  const editRef = (change: Partial<IdentityConsoleRef>) => {
    if (busy || outcomeUnknown) return;
    setRef((current) => ({ ...current, ...change }));
    setReadResult(undefined);
    setNotice(undefined);
    key.current = commandKey();
  };
  const notify = (failure: IdentityConsoleFailure, write: boolean) => {
    if (failure === "unauthenticated" && !unauthenticatedNotified.current) {
      unauthenticatedNotified.current = true;
      onUnauthenticated?.();
    }
    if (write && failure === "unavailable") setOutcomeUnknown(true);
    setNotice(messages[failure]);
  };
  const resolve = () => {
    if (!permitted || busy || outcomeUnknown || active.current !== undefined) return;
    const token = Symbol("identity-resolve");
    const owner = generation.current;
    active.current = { token, action: "resolve" };
    setBusy("resolve");
    setNotice(undefined);
    void resolveConsoleIdentity(transport, ref).then((next) => {
      if (generation.current !== owner || active.current?.token !== token) return;
      if (next.status === "resolved") {
        setReadResult(next.result);
        setNotice(resultText(next.result));
      } else notify(next.status, false);
    }).finally(() => {
      if (generation.current === owner && active.current?.token === token) {
        active.current = undefined;
        setBusy(undefined);
      }
    });
  };
  const bind = () => {
    if (!permitted || busy || outcomeUnknown || !confirmed || active.current !== undefined) return;
    const token = Symbol("identity-bind");
    const owner = generation.current;
    const customerID = Number(customerIDText);
    const csrf = readCSRFCookie(readCookie()) ?? "";
    active.current = { token, action: "bind" };
    setBusy("bind");
    setNotice(undefined);
    void bindConsoleIdentity(transport, customerID, ref, csrf, key.current).then((next) => {
      if (generation.current !== owner || active.current?.token !== token) return;
      if (next.status === "bound") {
        setNotice(bindText(next.result));
        setRef(initialRef);
        setCustomerIDText("");
        setConfirmed(false);
        setReadResult(undefined);
        key.current = commandKey();
      } else notify(next.status, true);
    }).finally(() => {
      if (generation.current === owner && active.current?.token === token) {
        active.current = undefined;
        setBusy(undefined);
      }
    });
  };

  if (!permitted) return <main className="route-card"><h1 id="app-title">本地身份控制台</h1><p>当前账号没有本地身份管理权限。</p></main>;
  const locked = busy !== undefined || outcomeUnknown;
  return <main className="route-card">
    <h1 id="app-title">本地身份控制台</h1>
    <p>仅查询或绑定本地客户 OneID；不触发 Provider、外发或自动合并。</p>
    {notice && <p role="status">{notice}</p>}
    {outcomeUnknown ? <p role="alert">绑定结果待确认。为避免重复写入，本页面已锁定，请由运营人员核查本地审计记录。</p> : <form onSubmit={(event) => { event.preventDefault(); bind(); }}>
      <fieldset disabled={locked}>
        <legend>身份引用</legend>
        <label>类型<select value={ref.type} onChange={(event) => editRef({ type: event.currentTarget.value as IdentityConsoleRef["type"] })}>
          <option value="phone">手机号</option><option value="unionid">UnionID</option><option value="wecom_external_userid">企微外部联系人 ID</option><option value="mp_openid">小程序 OpenID</option><option value="oa_openid">公众号 OpenID</option><option value="alipay_user_id">支付宝用户 ID</option><option value="ext">扩展身份</option>
        </select></label>
        <label>作用域<input value={ref.scope} onChange={(event) => editRef({ scope: event.currentTarget.value })} autoComplete="off" /></label>
        <label>身份值<input value={ref.value} onChange={(event) => editRef({ value: event.currentTarget.value })} autoComplete="off" /></label>
        <button type="button" onClick={resolve} disabled={locked}>查询本地归属</button>
      </fieldset>
      <fieldset disabled={locked}>
        <legend>显式本地绑定</legend>
        <label>客户 OneID<input inputMode="numeric" value={customerIDText} onChange={(event) => { setCustomerIDText(event.currentTarget.value); setNotice(undefined); key.current = commandKey(); }} /></label>
        <label><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.currentTarget.checked)} />我确认仅执行本地身份绑定。</label>
        <button type="submit" disabled={locked || !confirmed}>确认绑定</button>
      </fieldset>
    </form>}
    {readResult?.status === "conflict" && <p><a href="/identity/merge-reviews">查看本地人工待合并</a></p>}
  </main>;
}
