import { renderToStaticMarkup } from "react-dom/server";
import React from "react";
import { describe, expect, it } from "vitest";
import { App } from "./main";

describe("App", () => {
  it("renders the accessible AI-CRM shell", () => {
    const html = renderToStaticMarkup(<App />);

    expect(html).toContain('id="app-title"');
    expect(html).toContain("AI-CRM");
    expect(html).toContain("Web 应用骨架已就绪");
  });
});
