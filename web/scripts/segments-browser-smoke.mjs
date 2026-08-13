import { createServer } from "node:http";
import { readFile, stat } from "node:fs/promises";
import { resolve, extname } from "node:path";
import { spawn } from "node:child_process";

const distDirectory = resolve("web/dist");
const chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const csrf = "a".repeat(43);
const requests = [];
const segment = {
  id: 17,
  name: "近期活跃客户",
  definition: { and: [{ field: "stage_id", op: "eq", value: 3 }] },
  refresh_mode: "manual",
  refresh_cron: null,
  member_count: 1,
  refreshed_at: "2026-08-13T08:00:00Z",
  refresh_status: "idle",
  created_at: "2026-08-12T08:00:00Z",
  updated_at: "2026-08-13T08:00:00Z",
};
const customer = {
  id: 9, name: "陈晨", avatar_url: null, gender: null, stage_id: 3,
  owner_staff_id: 8, channel_id: 5, added_at: "2026-08-12T08:00:00Z",
  last_interact_at: "2026-08-12T09:00:00Z", is_deleted: false, extra: {},
  created_at: "2026-08-12T07:00:00Z", updated_at: "2026-08-12T10:00:00Z",
};

function sendJSON(response, status, body, headers = {}) {
  response.writeHead(status, { "content-type": "application/json", ...headers });
  response.end(JSON.stringify(body));
}
function contentType(filePath) {
  return ({ ".css": "text/css; charset=utf-8", ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8" }[extname(filePath)] ?? "application/octet-stream");
}
async function requestBody(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

const server = createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", "http://127.0.0.1");
  if (url.pathname === "/api/v1/auth/session") {
    sendJSON(response, 200, { admin_user_id: 1, role: "admin" }, { "set-cookie": `aicrm_csrf=${csrf}; Path=/; SameSite=Lax` });
    return;
  }
  if (url.pathname === "/api/v1/segments" && request.method === "GET") {
    sendJSON(response, 200, { items: [segment], next_cursor: null });
    return;
  }
  if (url.pathname === "/api/v1/segments" && request.method === "POST") {
    const body = await requestBody(request);
    requests.push({ kind: "create", body, headers: request.headers });
    sendJSON(response, 201, { ...segment, id: 18, name: body.name, definition: body.definition });
    return;
  }
  if (url.pathname === "/api/v1/segments/17" && request.method === "PATCH") {
    const body = await requestBody(request);
    requests.push({ kind: "update", body, headers: request.headers });
    sendJSON(response, 200, { ...segment, ...body, refresh_cron: body.refresh_cron ?? null });
    return;
  }
  if (/^\/api\/v1\/segments\/(17|18)\/members$/.test(url.pathname)) {
    sendJSON(response, 200, { items: [customer], next_cursor: null });
    return;
  }
  if (url.pathname === "/api/v1/segments/18/refresh" && request.method === "POST") {
    requests.push({ kind: "refresh", headers: request.headers });
    sendJSON(response, 202, { status: "accepted", segment_id: 18 });
    return;
  }
  if (url.pathname.startsWith("/api/")) {
    sendJSON(response, 404, { code: "not_found", message: "not found", request_id: "local" });
    return;
  }
  const requestedPath = resolve(distDirectory, `.${url.pathname}`);
  const filePath = requestedPath.startsWith(`${distDirectory}/`) ? requestedPath : resolve(distDirectory, "index.html");
  try {
    if (!(await stat(filePath)).isFile()) throw new Error("not file");
    response.writeHead(200, { "content-type": contentType(filePath) });
    response.end(await readFile(filePath));
  } catch {
    response.writeHead(200, { "content-type": "text/html; charset=utf-8" });
    response.end(await readFile(resolve(distDirectory, "index.html")));
  }
});
function wait(milliseconds) { return new Promise((done) => setTimeout(done, milliseconds)); }
async function waitFor(condition, description) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if (await condition()) return;
    await wait(50);
  }
  throw new Error(`timed out waiting for ${description}`);
}
async function connect(port) {
  let target;
  await waitFor(async () => {
    try {
      const pages = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      target = pages.find((page) => page.type === "page" && page.url.includes("/segments"));
      return Boolean(target?.webSocketDebuggerUrl);
    } catch { return false; }
  }, "Chrome DevTools");
  const socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolveOpen, rejectOpen) => { socket.addEventListener("open", resolveOpen, { once: true }); socket.addEventListener("error", rejectOpen, { once: true }); });
  const pending = new Map(); let sequence = 0;
  socket.addEventListener("message", ({ data }) => { const message = JSON.parse(data); const resolveMessage = pending.get(message.id); if (resolveMessage) { pending.delete(message.id); resolveMessage(message); } });
  return {
    evaluate(expression) {
      const id = ++sequence;
      socket.send(JSON.stringify({ id, method: "Runtime.evaluate", params: { expression, returnByValue: true } }));
      return new Promise((resolveMessage, rejectMessage) => pending.set(id, (message) => message.error || message.result?.exceptionDetails ? rejectMessage(new Error(JSON.stringify(message))) : resolveMessage(message.result.result.value)));
    },
    close() { socket.close(); },
  };
}

async function main() {
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const port = server.address().port;
  const devToolsPort = 43_000 + Math.floor(Math.random() * 1_000);
  const browser = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", `--remote-debugging-port=${devToolsPort}`, `--user-data-dir=/tmp/aicrm-segments-smoke-${process.pid}`, `http://127.0.0.1:${port}/segments`]);
  let devTools;
  try {
    devTools = await connect(devToolsPort);
    const evaluate = devTools.evaluate.bind(devTools);
    await waitFor(async () => Boolean(await evaluate("document.body?.innerText.includes('近期活跃客户')")), "Segment list");
    await evaluate("[...document.querySelectorAll('.segment-list button')][0].click()");
    await waitFor(async () => Boolean(await evaluate("document.body?.innerText.includes('陈晨')")), "materialized member preview");
    await evaluate("[...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '新建人群包').click()");
    await evaluate(`(() => {
      const inputs = [...document.querySelectorAll('input')];
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setter.call(inputs[0], '高意向客户'); inputs[0].dispatchEvent(new Event('input', { bubbles: true }));
      setter.call(inputs[1], '3'); inputs[1].dispatchEvent(new Event('input', { bubbles: true }));
      inputs[0].closest('form').requestSubmit();
    })()`);
    await waitFor(() => requests.some((request) => request.kind === "create"), "create request");
    const created = requests.find((request) => request.kind === "create");
    if (created.body.definition.field !== "stage_id" || created.body.definition.value !== 3 || created.headers["x-csrf-token"] !== csrf || !created.headers["idempotency-key"]) throw new Error("create request lost frozen DSL, CSRF, or idempotency boundary");
    await waitFor(async () => Boolean(await evaluate("document.body?.innerText.includes('当前人群包：高意向客户')")), "created Segment selection");
    await evaluate("[...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '手动刷新').click()");
    await waitFor(() => requests.some((request) => request.kind === "refresh"), "refresh request");
    const refreshed = requests.find((request) => request.kind === "refresh");
    if (refreshed.headers["x-csrf-token"] !== csrf || !refreshed.headers["idempotency-key"]) throw new Error("refresh request lost CSRF or idempotency boundary");
    await waitFor(async () => Boolean(await evaluate("document.body?.innerText.includes('这不表示成员已经更新')")), "accepted-not-complete notice");
    console.log("segments browser smoke: PASS");
  } finally {
    devTools?.close(); browser.kill("SIGTERM"); await new Promise((done) => server.close(done));
  }
}
await main();
