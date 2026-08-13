import { createServer } from "node:http";
import { readFile, stat, writeFile } from "node:fs/promises";
import { extname, resolve } from "node:path";
import { spawn } from "node:child_process";

const distDirectory = resolve("web/dist");
const chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const screenshotPath =
  process.env.AICRM_I8_SCREENSHOT ?? "/tmp/aicrm-i8-merge-reviews.png";
const csrf = "c".repeat(43);
const fingerprint = `hmac-sha256-v1:${"A".repeat(22)}`;
const requests = [];

function review(reviewID, customers) {
  return {
    review_id: reviewID,
    status: "pending",
    type: "phone",
    scope: "phone:e164",
    identity_fingerprint: fingerprint,
    customer_ids: customers,
    version: 1,
    created_at: "2026-08-13T08:00:00Z",
    resolved_at: null,
  };
}

const reviews = [review(17, [42, 84]), review(18, [126, 168])];

function sendJSON(response, status, body, headers = {}) {
  response.writeHead(status, {
    "content-type": "application/json",
    ...headers,
  });
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

async function requestBody(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

const server = createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", "http://127.0.0.1");
  if (url.pathname === "/api/v1/auth/session") {
    sendJSON(
      response,
      200,
      { admin_user_id: 1, role: "admin" },
      { "set-cookie": `aicrm_csrf=${csrf}; Path=/; SameSite=Lax` },
    );
    return;
  }
  if (
    url.pathname === "/api/v1/identity/merge-reviews" &&
    request.method === "GET"
  ) {
    sendJSON(response, 200, { items: reviews, next_cursor: null });
    return;
  }
  const action =
    /^\/api\/v1\/identity\/merge-reviews\/(17|18)\/(approve|reject)$/.exec(
      url.pathname,
    );
  if (action && request.method === "POST") {
    const body = await requestBody(request);
    const source = reviews.find(
      ({ review_id: reviewID }) => reviewID === Number(action[1]),
    );
    requests.push({ action: action[2], body, headers: request.headers });
    if (
      action[2] === "approve" &&
      requests.filter(({ action: recorded }) => recorded === "approve")
        .length === 1
    ) {
      sendJSON(response, 503, {
        code: "service_unavailable",
        message: "retry",
        request_id: "local-first-attempt",
      });
      return;
    }
    sendJSON(response, 200, {
      ...source,
      status: action[2] === "approve" ? "approved" : "rejected",
      version: 2,
      resolved_at: "2026-08-13T09:00:00Z",
    });
    return;
  }
  if (url.pathname.startsWith("/api/")) {
    sendJSON(response, 404, {
      code: "not_found",
      message: "not found",
      request_id: "local",
    });
    return;
  }

  const requestedPath = resolve(distDirectory, `.${url.pathname}`);
  const filePath = requestedPath.startsWith(`${distDirectory}/`)
    ? requestedPath
    : resolve(distDirectory, "index.html");
  try {
    if (!(await stat(filePath)).isFile()) throw new Error("not file");
    response.writeHead(200, { "content-type": contentType(filePath) });
    response.end(await readFile(filePath));
  } catch {
    response.writeHead(200, { "content-type": "text/html; charset=utf-8" });
    response.end(await readFile(resolve(distDirectory, "index.html")));
  }
});

function wait(milliseconds) {
  return new Promise((done) => setTimeout(done, milliseconds));
}

async function waitFor(condition, description) {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    if (await condition()) return;
    await wait(50);
  }
  throw new Error(`timed out waiting for ${description}`);
}

async function connect(port) {
  let target;
  await waitFor(async () => {
    try {
      const pages = await (
        await fetch(`http://127.0.0.1:${port}/json/list`)
      ).json();
      target = pages.find(
        (page) =>
          page.type === "page" && page.url.includes("/identity/merge-reviews"),
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
  const command = (method, params = {}) => {
    const id = ++sequence;
    socket.send(JSON.stringify({ id, method, params }));
    return new Promise((resolveMessage, rejectMessage) => {
      pending.set(id, (message) => {
        if (message.error || message.result?.exceptionDetails) {
          rejectMessage(new Error(JSON.stringify(message)));
          return;
        }
        resolveMessage(message.result);
      });
    });
  };
  return {
    evaluate: async (expression) =>
      (await command("Runtime.evaluate", { expression, returnByValue: true }))
        .result.value,
    screenshot: () =>
      command("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: true,
      }),
    close: () => socket.close(),
  };
}

async function main() {
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const port = server.address().port;
  const devToolsPort = 44_000 + Math.floor(Math.random() * 1_000);
  const browser = spawn(chrome, [
    "--headless=new",
    "--disable-gpu",
    "--no-first-run",
    "--window-size=1440,1000",
    `--remote-debugging-port=${devToolsPort}`,
    `--user-data-dir=/tmp/aicrm-i8-smoke-${process.pid}`,
    `http://127.0.0.1:${port}/identity/merge-reviews`,
  ]);
  let devTools;
  try {
    devTools = await connect(devToolsPort);
    const evaluate = devTools.evaluate.bind(devTools);
    await waitFor(
      async () =>
        Boolean(
          await evaluate("document.body?.innerText.includes('待办 #17')"),
        ),
      "merge-review list",
    );
    await evaluate(
      "[...document.querySelectorAll('.identity-review-list button')][0].click()",
    );
    await waitFor(
      async () =>
        Boolean(
          await evaluate("document.body?.innerText.includes('去标识指纹')"),
        ),
      "review facts",
    );
    await evaluate(`(() => {
      document.querySelector('input[name="primary-customer"]').click();
      const input = document.querySelector('.identity-review-reason textarea');
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set;
      setter.call(input, '运营已核验为同一客户'); input.dispatchEvent(new Event('input', { bubbles: true }));
      [...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '批准并合并').click();
    })()`);
    await waitFor(
      () => requests.some(({ action }) => action === "approve"),
      "first approve request",
    );
    await waitFor(
      async () =>
        Boolean(
          await evaluate(
            "document.body?.innerText.includes('人工待合并服务暂时不可用')",
          ),
        ),
      "retryable approve result",
    );
    await evaluate(
      "[...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '批准并合并').click()",
    );
    await waitFor(
      () => requests.filter(({ action }) => action === "approve").length === 2,
      "retried approve request",
    );
    await waitFor(
      async () =>
        Boolean(
          await evaluate(
            "document.body?.innerText.includes('主客户为 OneID 42')",
          ),
        ),
      "approve result",
    );

    await evaluate(
      "[...document.querySelectorAll('.identity-review-list button')][0].click()",
    );
    await evaluate(`(() => {
      const input = document.querySelector('.identity-review-reason textarea');
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set;
      setter.call(input, '手机号已确认换主'); input.dispatchEvent(new Event('input', { bubbles: true }));
      [...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === '拒绝合并').click();
    })()`);
    await waitFor(
      () => requests.some(({ action }) => action === "reject"),
      "reject request",
    );
    await waitFor(
      async () =>
        Boolean(
          await evaluate(
            "document.body?.innerText.includes('客户绑定保持不变')",
          ),
        ),
      "reject result",
    );

    for (const request of requests) {
      if (
        request.headers["x-csrf-token"] !== csrf ||
        !request.headers["idempotency-key"]
      ) {
        throw new Error("review request lost CSRF or idempotency boundary");
      }
    }
    const approveRequests = requests.filter(
      ({ action }) => action === "approve",
    );
    const approveRequest = approveRequests[1];
    const rejectRequest = requests.find(({ action }) => action === "reject");
    if (
      approveRequests[0].headers["idempotency-key"] !==
        approveRequests[1].headers["idempotency-key"] ||
      JSON.stringify(approveRequests[0].body) !==
        JSON.stringify(approveRequests[1].body) ||
      approveRequest.body.primary_customer_id !== 42 ||
      approveRequest.body.expected_version !== 1 ||
      rejectRequest.body.primary_customer_id !== undefined ||
      rejectRequest.body.expected_version !== 1
    ) {
      throw new Error(
        "review request expanded or lost the frozen command shape",
      );
    }
    const screenshot = await devTools.screenshot();
    await writeFile(screenshotPath, Buffer.from(screenshot.data, "base64"));
    console.log(
      `identity reviews browser smoke: PASS screenshot=${screenshotPath}`,
    );
  } finally {
    devTools?.close();
    browser.kill("SIGTERM");
    await new Promise((done) => server.close(done));
  }
}

await main();
