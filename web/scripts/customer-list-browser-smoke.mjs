import { createServer } from "node:http";
import { readFile, stat } from "node:fs/promises";
import { resolve, extname } from "node:path";
import { spawn } from "node:child_process";

const distDirectory = resolve("web/dist");
const chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const customerRequests = [];

function customer(id, name) {
  return {
    id,
    name,
    avatar_url: null,
    gender: null,
    stage_id: 3,
    owner_staff_id: 8,
    channel_id: 5,
    added_at: "2026-08-12T08:00:00Z",
    last_interact_at: "2026-08-12T09:00:00Z",
    is_deleted: false,
    extra: {},
    created_at: "2026-08-12T07:00:00Z",
    updated_at: "2026-08-12T10:00:00Z",
  };
}

function page(items, nextCursor) {
  return {
    items,
    next_cursor: nextCursor,
    total: 2,
    total_is_estimate: false,
    watermark: "2026-08-12T10:00:00Z",
  };
}

function sendJSON(response, status, body) {
  response.writeHead(status, { "content-type": "application/json" });
  response.end(JSON.stringify(body));
}

function contentType(filePath) {
  return (
    {
      ".css": "text/css; charset=utf-8",
      ".html": "text/html; charset=utf-8",
      ".js": "text/javascript; charset=utf-8",
    }[extname(filePath)] ?? "application/octet-stream"
  );
}

const server = createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", "http://127.0.0.1");
  if (url.pathname === "/api/v1/auth/session") {
    sendJSON(response, 200, { admin_user_id: 1, role: "admin" });
    return;
  }
  if (url.pathname === "/api/v1/customers") {
    customerRequests.push(url);
    const cursor = url.searchParams.get("cursor");
    if (cursor === null) {
      sendJSON(response, 200, page([customer(7, "陈晨")], "opaque-next-page"));
      return;
    }
    if (cursor === "opaque-next-page") {
      sendJSON(response, 200, page([customer(8, "林小姐")], null));
      return;
    }
    sendJSON(response, 400, {});
    return;
  }
  if (url.pathname.startsWith("/api/")) {
    sendJSON(response, 404, {});
    return;
  }

  const requestedPath = resolve(distDirectory, `.${url.pathname}`);
  const filePath = requestedPath.startsWith(`${distDirectory}/`)
    ? requestedPath
    : resolve(distDirectory, "index.html");
  try {
    const metadata = await stat(filePath);
    if (!metadata.isFile()) throw new Error("not a file");
    response.writeHead(200, { "content-type": contentType(filePath) });
    response.end(await readFile(filePath));
  } catch {
    response.writeHead(200, { "content-type": "text/html; charset=utf-8" });
    response.end(await readFile(resolve(distDirectory, "index.html")));
  }
});

function wait(milliseconds) {
  return new Promise((resolveWait) => setTimeout(resolveWait, milliseconds));
}

async function waitFor(condition, description) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if (await condition()) return;
    await wait(50);
  }
  throw new Error(`timed out waiting for ${description}`);
}

async function connectDevTools(port) {
  let target;
  await waitFor(async () => {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/list`);
      const pages = await response.json();
      target = pages.find(
        (item) => item.type === "page" && item.url.includes("/customers"),
      );
      return Boolean(target?.webSocketDebuggerUrl);
    } catch {
      return false;
    }
  }, "Chrome DevTools");

  const socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolveOpen, rejectOpen) => {
    socket.addEventListener("open", resolveOpen, { once: true });
    socket.addEventListener("error", rejectOpen, { once: true });
  });
  const pending = new Map();
  let sequence = 0;
  socket.addEventListener("message", ({ data }) => {
    const message = JSON.parse(data);
    const resolver = pending.get(message.id);
    if (resolver) {
      pending.delete(message.id);
      resolver(message);
    }
  });
  return {
    evaluate(expression) {
      const id = (sequence += 1);
      socket.send(
        JSON.stringify({
          id,
          method: "Runtime.evaluate",
          params: { expression, returnByValue: true },
        }),
      );
      return new Promise((resolveMessage, rejectMessage) => {
        pending.set(id, (message) => {
          if (message.error || message.result?.exceptionDetails) {
            rejectMessage(new Error(JSON.stringify(message)));
            return;
          }
          resolveMessage(message.result.result.value);
        });
      });
    },
    close() {
      socket.close();
    },
  };
}

async function main() {
  await new Promise((resolveListen) => server.listen(0, "127.0.0.1", resolveListen));
  const port = server.address().port;
  const devToolsPort = 42_000 + Math.floor(Math.random() * 1_000);
  const browser = spawn(chrome, [
    "--headless=new",
    "--disable-gpu",
    "--no-first-run",
    `--remote-debugging-port=${devToolsPort}`,
    `--user-data-dir=/tmp/aicrm-customer-list-smoke-${process.pid}`,
    `http://127.0.0.1:${port}/customers`,
  ]);
  let devTools;
  try {
    devTools = await connectDevTools(devToolsPort);
    const evaluate = devTools.evaluate.bind(devTools);
    await waitFor(
      async () => Boolean(await evaluate("document.body?.innerText.includes('陈晨')")),
      "first customer page",
    );
    await evaluate(
      "[...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '下一页').click()",
    );
    await waitFor(
      async () => Boolean(await evaluate("document.body?.innerText.includes('林小姐')")),
      "second customer page",
    );
    if (await evaluate("document.body?.innerText.includes('陈晨')")) {
      throw new Error("next page appended the previous page's row");
    }
    await evaluate(
      "[...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '上一页').click()",
    );
    await waitFor(
      async () => Boolean(await evaluate("document.body?.innerText.includes('陈晨')")),
      "previous customer page",
    );
    const beforeFilter = customerRequests.length;
    await evaluate(`(() => {
      const input = document.querySelector('#customer-keyword');
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setter.call(input, '陈晨');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.closest('form').requestSubmit();
    })()`);
    await waitFor(
      async () => customerRequests.slice(beforeFilter).some((url) => url.searchParams.get("keyword") === "陈晨"),
      "filtered customer request",
    );
    const beforeClear = customerRequests.length;
    await evaluate(
      "[...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '清空筛选').click()",
    );
    await waitFor(
      async () => customerRequests.slice(beforeClear).some((url) => !url.searchParams.has("keyword")),
      "cleared customer request",
    );
    await waitFor(
      async () => Boolean(await evaluate("document.body?.innerText.includes('陈晨')")),
      "cleared customer result page",
    );
    await evaluate("document.querySelector('a[href=\"/customers/7\"]').click()");
    await waitFor(
      async () => (await evaluate("location.pathname")) === "/customers/7",
      "detail route navigation",
    );
    console.log("customer-list browser smoke: PASS");
  } finally {
    devTools?.close();
    browser.kill("SIGTERM");
    await new Promise((resolveClose) => server.close(resolveClose));
  }
}

await main();
