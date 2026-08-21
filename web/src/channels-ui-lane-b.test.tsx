import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ChannelsView } from "./channels-ui";
import type { ChannelDetail, ChannelListItem } from "./channels";

const createdAt = "2026-08-19T00:00:00Z";
const updatedAt = "2026-08-19T01:02:03Z";

function item(id: number): ChannelListItem {
  return {
    id,
    name: `channel-name-${String(id).padStart(3, "0")}`,
    code: `channel-code-${String(id).padStart(3, "0")}`,
    status: "active",
    assigneeCount: 0,
    contactCount: 0,
    createdAt,
    updatedAt,
  };
}

function detail(value: ChannelListItem): ChannelDetail {
  return {
    item: value,
    channelType: "qrcode",
    carrierType: "link",
    sceneValue: "local-scene",
    qrURL: "https://local.invalid/qr",
    ownerStaffID: "raw-owner-identity-secret",
    customerChannel: "opaque-customer-channel-secret",
    linkURL: "https://local.invalid/acquisition",
    finalURL: "/local/final",
    welcomeMessage: "raw-message-content-secret",
    imageMaterialIDs: [987_654],
    miniProgramMaterialIDs: [987_655],
    attachmentMaterialIDs: [987_656],
    groupInviteMaterialIDs: [987_657],
    autoAcceptFriend: false,
    entryTagID: "raw-entry-tag-id-secret",
    entryTagName: "raw-entry-tag-name-secret",
    entryTagGroupName: "raw-entry-tag-group-secret",
    assignmentMode: "single_owner",
    assignmentStrategy: "ratio",
    overflowPolicy: "opaque-overflow-policy-secret",
    hasAssignmentConfig: true,
    imageMaterialCount: 1,
    miniProgramMaterialCount: 1,
    attachmentMaterialCount: 1,
    groupInviteMaterialCount: 1,
  };
}

describe("Lane B safe local channel page", () => {
  it("shows a bounded first page and explicitly declares provider execution disabled", () => {
    const items = Array.from({ length: 25 }, (_, index) => item(index + 1));
    const html = renderToStaticMarkup(
      <ChannelsView role="admin" state={{ kind: "ready", items }} />,
    );

    expect(html).toContain('data-provider-execution-eligible="false"');
    expect(html).toContain("provider_execution_eligible=false");
    expect(html).toContain("channel-name-020");
    expect(html).not.toContain("channel-name-021");
    expect(html).toContain('aria-label="渠道列表分页"');
    expect(html).toContain("第 1 / 2 页，共 25 条");
  });

  it("never renders raw identities, welcome content, opaque references, or material IDs", () => {
    const value = item(1);
    const configuration = detail(value);
    const html = renderToStaticMarkup(
      <ChannelsView
        role="admin"
        state={{ kind: "ready", items: [value] }}
        detail={{ kind: "ready", item: value, detail: configuration }}
        editor={{ kind: "edit", detail: configuration }}
      />,
    );

    for (const secret of [
      configuration.ownerStaffID,
      configuration.customerChannel,
      configuration.welcomeMessage,
      String(configuration.imageMaterialIDs[0]),
      String(configuration.miniProgramMaterialIDs[0]),
      String(configuration.attachmentMaterialIDs[0]),
      String(configuration.groupInviteMaterialIDs[0]),
      configuration.entryTagID,
      configuration.entryTagName,
      configuration.entryTagGroupName,
      configuration.overflowPolicy,
    ]) {
      expect(html).not.toContain(secret);
    }
    expect(html).toContain("受保护本地引用（仅显示配置状态）");
    expect(html).toContain("图片 1，小程序 1，附件 1，群邀请 1");
    expect(html).toContain("provider_execution_eligible=false");
    expect(html).not.toMatch(/<a\b|href=|window\.open|navigator\.clipboard/i);
  });
});
