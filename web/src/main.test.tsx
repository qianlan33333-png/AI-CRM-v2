import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getHealthz } from "./api/generated/health";
import { App } from "./main";

afterEach(() => vi.unstubAllGlobals());

describe("App", () => {
  it("renders the accessible AI-CRM shell", () => {
    const html = renderToStaticMarkup(<App />);

    expect(html).toContain('id="app-title"');
    expect(html).toContain("AI-CRM");
    expect(html).toContain("Web 应用骨架已就绪");
  });

  it("uses the generated health client contract", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(input).toBe("/healthz");
      expect(init).toEqual({ method: "GET" });
      return new Response('{"status":"ok"}', { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const response = await getHealthz();

    expect(response.status).toBe(200);
    expect(response.data).toEqual({ status: "ok" });
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
