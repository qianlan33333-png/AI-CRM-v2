import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  DeliveryLineagePage,
  DeliveryLineageView,
} from "./delivery-lineage-ui";
import type { DeliveryLineageTransport } from "./delivery-lineage";

function transport(): DeliveryLineageTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
  } as DeliveryLineageTransport;
}

describe("DeliveryLineagePage", () => {
  it("renders admin a local-internal read view", () => {
    const html = renderToStaticMarkup(
      <DeliveryLineagePage role="admin" transport={transport()} />,
    );
    expect(html).toContain('<h1 id="app-title">投递处理谱系</h1>');
    expect(html).toContain("仅展示本地内部处理状态");
    expect(html).toContain("正在读取投递处理谱系。");
    expect(html).not.toMatch(/href=|clipboard|provider/i);
  });

  it.each(["ops", "sales"] as const)("keeps %s fail-closed without a list request", (role) => {
    const client = transport();
    const html = renderToStaticMarkup(
      <DeliveryLineagePage role={role} transport={client} />,
    );
    expect(html).toContain("当前账号没有投递处理谱系访问权限。");
    expect(html).not.toContain("正在读取投递处理谱系。");
    expect(client.list).not.toHaveBeenCalled();
  });

  it("never turns the externally-unknown fields into a delivery assertion", () => {
    const html = renderToStaticMarkup(
      <DeliveryLineageView
        onLoad={vi.fn()}
        state={{
          kind: "ready",
          page: {
            items: [
              {
                lineageID: "outbound-task:42",
                recordKind: "outbound_task",
                internalState: "outcome_unknown",
                attemptCount: 2,
                updatedAt: "2026-08-19T08:00:00Z",
              },
            ],
            limit: 50,
            offset: 0,
            hasMore: false,
          },
        }}
      />,
    );
    expect(html).toContain("outbound-task:42");
    expect(html).toContain("outcome_unknown");
    expect(html).toContain("不表示任何企业微信、支付或其他外部投递已经执行、送达或收到回执");
    expect(html).not.toMatch(/已送达|已收到回执|href=|clipboard|provider/i);
  });
});
