import { describe, expect, it, vi } from "vitest";
import { cancelPendingOutboundTask, confirmsCancelledOutboundTask, generatedOutboundOperationsTransport, loadOutboundReconciliation, loadOutboundTasks, newOutboundCancelIdempotencyKey, parseOutboundTask, type OutboundOperationsTransport } from "./outbound-operations";
const job = { job_id: 42, task_id: 42, customer_id: 71, status: "outcome_unknown", attempt_count: 2, delivery_proven: false, queue_job: { river_job_id: 90, generation: 2, kind: "outbound_enqueue_one" }, created_at: "2026-08-20T08:00:00Z", status_updated_at: "2026-08-20T08:00:01Z" };
function client(): OutboundOperationsTransport {
  return {
    list: vi.fn(async () => ({
      status: 200,
      data: {
        ok: true,
        jobs: [job],
        items: [job],
        count: 1,
        has_more: false,
        limit: 50,
        offset: 0,
        source_status: "v2_outbound_service",
        fallback_used: false,
      },
    })),
    reconciliation: vi.fn(async () => ({
      status: 200,
      data: {
        ok: true,
        job,
        attempts: [],
        control_receipts: [],
        source_status: "v2_outbound_service",
        fallback_used: false,
      },
    })),
  };
}
describe("outbound observation boundary", () => { it("projects safe task", () => expect(parseOutboundTask(job)?.taskID).toBe("42")); it("uses local reads", async () => { const t = client(); await loadOutboundTasks(t); await loadOutboundReconciliation(t, "42"); expect(t.list).toHaveBeenCalledOnce(); expect(t.reconciliation).toHaveBeenCalledOnce(); }); });

describe("pre-provider outbound cancellation", () => {
  const pending = { ...job, status: "pending", attempt_count: 0 };
  const receipt = { receipt_id: 91, task_id: 42, operation: "cancel", state: "completed", generation: 2, river_job_id: 90, job_kind: "outbound_enqueue_one", event_id: 92, task_status: "cancelled", completed_at: "2026-08-20T08:00:02Z" };
  it("requires strict local 202 receipt and same-task reconciliation", async () => {
    const t: OutboundOperationsTransport = { ...client(), cancel: vi.fn(async () => ({ status: 202, data: { ok: true, control_receipt: receipt, source_status: "v2_outbound_cancel_service", fallback_used: false } })) };
    const result = await cancelPendingOutboundTask(t, parseOutboundTask(pending)!, "x".repeat(43), "outbound-cancel:123e4567-e89b-42d3-a456-426614174000");
    expect(result).toMatchObject({ status: "cancelled", receipt: { receiptID: "91", taskID: "42" } });
    expect(t.cancel).toHaveBeenCalledWith("42", { credentials: "same-origin", headers: { "X-CSRF-Token": "x".repeat(43), "Idempotency-Key": "outbound-cancel:123e4567-e89b-42d3-a456-426614174000" } });
    const cancelled = parseOutboundTask({ ...pending, status: "cancelled", status_updated_at: "2026-08-20T08:00:02Z" })!;
    const reconciliation = { task: cancelled, attempts: [], receipts: [{ receiptID: "91", taskID: "42", operation: "cancel" as const, state: "completed" as const, generation: 2, queueKind: "outbound_enqueue_one", taskStatus: "cancelled", completedAt: "2026-08-20T08:00:02Z" }] };
    expect(confirmsCancelledOutboundTask(reconciliation, (result as Extract<typeof result, { status: "cancelled" }>).receipt)).toBe(true);
    expect(confirmsCancelledOutboundTask({ ...reconciliation, receipts: [] }, (result as Extract<typeof result, { status: "cancelled" }>).receipt)).toBe(false);
  });
  it("enables the generated default transport instead of leaving a manifest-only control", () => {
    expect(generatedOutboundOperationsTransport.cancel).toBeTypeOf("function");
  });
  it("rejects non-pending commands, malformed receipts, and unsafe keys before any POST", async () => {
    const cancel = vi.fn(async () => ({ status: 202, data: { ok: true, control_receipt: { ...receipt, ignored: true }, source_status: "v2_outbound_cancel_service", fallback_used: false } }));
    const t: OutboundOperationsTransport = { ...client(), cancel };
    await expect(cancelPendingOutboundTask(t, parseOutboundTask(job)!, "x".repeat(43), "outbound-cancel:123e4567-e89b-42d3-a456-426614174000")).resolves.toEqual({ status: "invalid" });
    expect(cancel).not.toHaveBeenCalled();
    await expect(cancelPendingOutboundTask(t, parseOutboundTask(pending)!, "short", "short")).resolves.toEqual({ status: "invalid" });
    expect(cancel).not.toHaveBeenCalled();
    await expect(cancelPendingOutboundTask(t, parseOutboundTask(pending)!, "x".repeat(43), "outbound-cancel:123e4567-e89b-42d3-a456-426614174000")).resolves.toEqual({ status: "unknown" });
    expect(cancel).toHaveBeenCalledOnce();
    expect(newOutboundCancelIdempotencyKey({ randomUUID: () => "123e4567-e89b-42d3-a456-426614174000" })).toBe("outbound-cancel:123e4567-e89b-42d3-a456-426614174000");
    expect(newOutboundCancelIdempotencyKey({ randomUUID: () => "not-a-uuid" })).toBeUndefined();
  });
});
