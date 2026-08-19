import { describe, expect, it, vi } from "vitest";
import { loadOutboundReconciliation, loadOutboundTasks, parseOutboundTask, type OutboundOperationsTransport } from "./outbound-operations";
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
