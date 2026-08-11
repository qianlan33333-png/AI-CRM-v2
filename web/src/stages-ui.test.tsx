import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { StagesPage } from "./stages-ui";
import type { StageTransport } from "./stages";

function transport(): StageTransport {
  return {
    list: vi.fn(async () => ({ status: 200, data: { items: [] } })),
    create: vi.fn(async () => ({ status: 503, data: {} })),
    rename: vi.fn(async () => ({ status: 503, data: {} })),
  };
}

describe("StagesPage shell", () => {
  it.each(["admin", "ops"] as const)(
    "renders an accessible loading view and write controls for %s",
    (role) => {
      const html = renderToStaticMarkup(
        <StagesPage role={role} transport={transport()} />,
      );
      expect(html).toContain('<h1 id="app-title">阶段管理</h1>');
      expect(html).toContain('role="status"');
      expect(html).toContain("正在读取阶段");
      expect(html).toContain("新增阶段");
      expect(html).toContain("阶段名称");
      expect(html.match(/<h1\b/g)).toHaveLength(1);
      expect(html).not.toContain("aicrm_csrf");
      expect(html).not.toContain("X-CSRF-Token");
    },
  );

  it("keeps sales read-only", () => {
    const html = renderToStaticMarkup(
      <StagesPage role="sales" transport={transport()} />,
    );
    expect(html).toContain("正在读取阶段");
    expect(html).not.toContain("新增阶段");
    expect(html).not.toContain("保存改名");
    expect(html).not.toContain("<form");
  });
});
