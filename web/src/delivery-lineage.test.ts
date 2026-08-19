import { describe, expect, it, vi } from "vitest";
import {
  DELIVERY_LINEAGE_PAGE_SIZE,
  loadDeliveryLineage,
  nextDeliveryLineagePage,
  parseDeliveryLineagePage,
  parseDeliveryLineageRecord,
  previousDeliveryLineagePage,
  type DeliveryLineageTransport,
} from "./delivery-lineage";

const eventRecord = {
  lineage_id: `event-delivery:v1:${"a".repeat(64)}`,
  record_kind: "event_delivery",
  internal_state: "completed",
  attempt_count: 1,
  updated_at: "2026-08-19T08:00:00Z",
  external_delivery: "unknown",
  external_receipt: "unknown",
};

const outboundRecord = {
  lineage_id: "outbound-task:42",
  record_kind: "outbound_task",
  internal_state: "outcome_unknown",
  attempt_count: 2,
  updated_at: "2026-08-19T08:00:01Z",
  external_delivery: "unknown",
  external_receipt: "unknown",
};

function envelope(
  items: readonly Record<string, unknown>[] = [eventRecord],
  offset = 0,
  hasMore = false,
) {
  return {
    ok: true,
    items,
    limit: DELIVERY_LINEAGE_PAGE_SIZE,
    offset,
    has_more: hasMore,
    interpretation: {
      kind: "internal_processing_only",
      external_delivery: "unknown",
      external_receipt: "unknown",
    },
  };
}

function transport(
  overrides: Partial<DeliveryLineageTransport> = {},
): DeliveryLineageTransport {
  return {
    list: vi.fn(async () => ({ status: 503, data: {} })),
    ...overrides,
  } as DeliveryLineageTransport;
}

describe("delivery-lineage local read contract", () => {
  it("accepts only the complete local-internal, external-unknown DTO", () => {
    expect(parseDeliveryLineageRecord(eventRecord)).toMatchObject({
      lineageID: eventRecord.lineage_id,
      internalState: "completed",
    });
    expect(
      parseDeliveryLineageRecord({
        ...outboundRecord,
        lineage_id: "outbound-task:9223372036854775807",
      }),
    ).toMatchObject({ lineageID: "outbound-task:9223372036854775807" });
    expect(parseDeliveryLineagePage(envelope([eventRecord, outboundRecord]), 0)).toMatchObject({
      offset: 0,
      hasMore: false,
    });
  });

  it.each([
    { ...eventRecord, provider_result: "sent" },
    { ...eventRecord, external_delivery: "sent" },
    { ...eventRecord, external_receipt: "received" },
    { ...eventRecord, lineage_id: "event-delivery:v1:short" },
    { ...eventRecord, attempt_count: -1 },
    { ...eventRecord, updated_at: "not-a-time" },
    { ...eventRecord, lineage_id: "outbound-task:42" },
    { ...eventRecord, record_kind: "outbound_task" },
    { ...eventRecord, internal_state: "sending" },
    { ...outboundRecord, lineage_id: `event-delivery:v1:${"b".repeat(64)}` },
    { ...outboundRecord, internal_state: "processing" },
    { ...outboundRecord, lineage_id: "outbound-task:9223372036854775808" },
  ])("rejects expanded or unsafe record %#", (value) => {
    expect(parseDeliveryLineageRecord(value)).toBeUndefined();
  });

  it("rejects non-internal interpretation, duplicate records, and false paging claims", () => {
    const valid = envelope();
    for (const value of [
      {
        ...valid,
        interpretation: { ...valid.interpretation, kind: "external_delivery" },
      },
      { ...valid, unexpected: true },
      envelope([eventRecord, eventRecord]),
      envelope([eventRecord], 0, true),
    ]) {
      expect(parseDeliveryLineagePage(value, 0)).toBeUndefined();
    }
  });
});

describe("delivery-lineage local transport", () => {
  it("uses one fixed same-origin GET without a write or retry", async () => {
    const client = transport({
      list: vi.fn(async () => ({ status: 200, data: envelope() })),
    });
    await expect(loadDeliveryLineage(client)).resolves.toMatchObject({ status: "loaded" });
    expect(client.list).toHaveBeenCalledOnce();
    expect(client.list).toHaveBeenCalledWith(
      { limit: 50, offset: 0 },
      { credentials: "same-origin" },
    );
  });

  it("fails closed for invalid offsets, rejected responses, and a network fault", async () => {
    const client = transport({
      list: vi
        .fn()
        .mockResolvedValueOnce({ status: 200, data: { ...envelope(), has_more: "yes" } })
        .mockResolvedValueOnce({ status: 401, data: {} })
        .mockRejectedValueOnce(new Error("offline")),
    });
    await expect(loadDeliveryLineage(client, 1)).resolves.toEqual({ status: "invalid" });
    expect(client.list).not.toHaveBeenCalled();
    await expect(loadDeliveryLineage(client)).resolves.toEqual({ status: "invalid" });
    await expect(loadDeliveryLineage(client)).resolves.toEqual({ status: "unauthenticated" });
    await expect(loadDeliveryLineage(client)).resolves.toEqual({ status: "unavailable" });
  });

  it("calculates only bounded page transitions", () => {
    const page = parseDeliveryLineagePage(envelope([eventRecord], 50), 50);
    if (!page) throw new Error("expected page");
    expect(previousDeliveryLineagePage(page)).toBe(0);
    expect(nextDeliveryLineagePage(page)).toBeUndefined();
  });
});
