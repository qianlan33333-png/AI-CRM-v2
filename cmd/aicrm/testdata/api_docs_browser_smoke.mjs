// Headless browser smoke for the frozen legacy API documentation page
// (LEGACY-API-0003). The fixture HTML is rendered by the Go test helper
// TestLegacyAPIDocsRenderSmokeFixture so the browser always exercises the
// real embedded contract projection. No npm dependencies are required.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));
const chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const fixture = `/tmp/aicrm-api-docs-smoke-${process.pid}.html`;

const render = spawnSync(
  "go",
  ["test", "-count=1", "-run", "^TestLegacyAPIDocsRenderSmokeFixture$", "./cmd/aicrm"],
  { cwd: repoRoot, env: { ...process.env, AICRM_API_DOCS_SMOKE_OUT: fixture }, encoding: "utf8" },
);
if (render.status !== 0) {
  console.error(render.stdout);
  console.error(render.stderr);
  throw new Error("failed to render the API docs fixture via go test");
}
const html = await readFile(fixture);

const requests = [];
const server = createServer((request, response) => {
  const url = new URL(request.url ?? "/", "http://127.0.0.1");
  requests.push(url.pathname);
  if (url.pathname === "/admin/api-docs") {
    response.writeHead(200, {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
      "x-content-type-options": "nosniff",
      "referrer-policy": "no-referrer",
      "content-security-policy":
        "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; " +
        "img-src 'none'; connect-src 'none'; font-src 'none'; object-src 'none'; media-src 'none'; " +
        "frame-src 'none'; worker-src 'none'; child-src 'none'; form-action 'none'; base-uri 'none'",
    });
    response.end(html);
    return;
  }
  response.writeHead(404);
  response.end();
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
      target = pages.find((page) => page.type === "page" && page.url.includes("/admin/api-docs"));
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
  const devToolsPort = 44_000 + Math.floor(Math.random() * 1_000);
  const browser = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", `--remote-debugging-port=${devToolsPort}`, `--user-data-dir=/tmp/aicrm-api-docs-smoke-${process.pid}`, `http://127.0.0.1:${port}/admin/api-docs`]);
  let devTools;
  try {
    devTools = await connect(devToolsPort);
    const evaluate = devTools.evaluate.bind(devTools);

    // Page shell renders under the frozen CSP.
    await waitFor(async () => (await evaluate("document.title")) === "API 文档", "page title");
    const cardCount = await evaluate("document.querySelectorAll('.card').length");
    const statText = await evaluate("document.querySelector('.top-actions .stat')?.textContent ?? ''");
    if (!statText.includes(`共 ${cardCount} 个接口`) || cardCount < 40) {
      throw new Error(`endpoint count mismatch: cards=${cardCount} stat=${statText}`);
    }

    // The embedded Markdown payload parses and covers every copy target.
    const payload = await evaluate("JSON.parse(document.getElementById('apidoc-md-data').textContent)");
    if (!payload.full.startsWith("# AI-CRM API 文档")) throw new Error("full markdown payload broken");
    const copyKeys = await evaluate("[...document.querySelectorAll('[data-copy-md]')].map((button) => button.getAttribute('data-copy-md'))");
    for (const key of copyKeys) {
      const text = key === "full" ? payload.full : key.startsWith("group:") ? payload.groups[key.slice(6)] : payload.endpoints[key.slice(3)];
      if (!text) throw new Error(`copy target ${key} has no markdown payload`);
    }

    // Clipboard copy wiring works for endpoint, group and full scopes.
    await evaluate("Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: (text) => { window.__copied = text; return Promise.resolve(); } } })");
    await evaluate("document.querySelector('[data-copy-md=\"ep:get-api-customers\"]').click()");
    await waitFor(async () => Boolean(await evaluate("window.__copied")), "endpoint copy");
    if (!(await evaluate("window.__copied")).startsWith("## GET /api/customers")) throw new Error("endpoint markdown copy broken");
    await evaluate("window.__copied = null; document.querySelector('[data-copy-md=\"full\"]').click()");
    await waitFor(async () => Boolean(await evaluate("window.__copied")), "full copy");
    if (!(await evaluate("window.__copied")).startsWith("# AI-CRM API 文档")) throw new Error("full markdown copy broken");
    await waitFor(async () => (await evaluate("document.querySelector('[data-copy-md=\"full\"]').textContent")) === "已复制", "copied feedback");

    // Local substring search filters cards, quick index and the empty hint.
    await evaluate("(() => { const input = document.getElementById('api-docs-search'); input.value = 'image-library'; input.dispatchEvent(new Event('input', { bubbles: true })); })()");
    const filtered = await evaluate("[...document.querySelectorAll('.card')].filter((card) => card.style.display !== 'none').map((card) => card.getAttribute('data-search-text'))");
    if (filtered.length === 0 || filtered.some((text) => !text.toLowerCase().includes("image-library"))) {
      throw new Error(`search filter broken: ${JSON.stringify(filtered)}`);
    }
    const visibleQuickRows = await evaluate("[...document.querySelectorAll('[data-search-row]')].filter((row) => row.style.display !== 'none').length");
    if (visibleQuickRows !== filtered.length) throw new Error("quick index and cards disagree after search");
    await evaluate("(() => { const input = document.getElementById('api-docs-search'); input.value = 'zzz-no-such-endpoint'; input.dispatchEvent(new Event('input', { bubbles: true })); })()");
    const visibleAfterMiss = await evaluate("[...document.querySelectorAll('.card')].filter((card) => card.style.display !== 'none').length");
    const emptyVisible = await evaluate("document.getElementById('api-docs-empty').style.display === 'block'");
    if (visibleAfterMiss !== 0 || !emptyVisible) throw new Error("empty search state broken");

    // Hash navigation expands the target card and its group.
    await evaluate("(() => { const input = document.getElementById('api-docs-search'); input.value = ''; input.dispatchEvent(new Event('input', { bubbles: true })); })()");
    const cardInitiallyClosed = await evaluate("document.getElementById('get-api-customers').open === false");
    if (!cardInitiallyClosed) throw new Error("endpoint card should start collapsed");
    await evaluate("location.hash = '#get-api-customers'");
    await waitFor(async () => Boolean(await evaluate("document.getElementById('get-api-customers').open === true")), "hash expansion");

    // The page performs no secondary network fetches.
    const unexpected = requests.filter((path) => path !== "/admin/api-docs" && path !== "/favicon.ico");
    if (unexpected.length > 0) throw new Error(`unexpected network requests: ${unexpected.join(",")}`);

    console.log(`api docs browser smoke: PASS (${cardCount} endpoints)`);
  } finally {
    devTools?.close(); browser.kill("SIGTERM"); await new Promise((done) => server.close(done));
  }
}
await main();
