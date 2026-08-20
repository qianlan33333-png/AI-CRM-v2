import { describe, expect, it, vi } from "vitest";
import { createPublicSubmissionFlight, createPublicSurveyController, publicSlug } from "./public-survey";

describe("public survey controller", () => {
  it("keeps the result token in closure and queries by POST transport only", async () => {
    const transport = { definition: vi.fn(), submit: vi.fn().mockResolvedValue({ receipt: { submission_id: 3 }, result_token: "a".repeat(43) }), result: vi.fn().mockResolvedValue({ submission_id: 3, definition_version: 1, submitted_at: "2026-01-01T00:00:00Z", local_only: true, external_executed: false }) };
    const controller = createPublicSurveyController(transport, "/q/public-1");
    await controller.submit({ version: 1, submission_key: "b".repeat(43), answers: [] });
    await controller.result();
    expect(transport.result).toHaveBeenCalledWith("a".repeat(43));
    expect(controller).not.toHaveProperty("result_token");
  });
  it("accepts only the frozen carrier pathname", () => {
    expect(publicSlug("/q/public-1")).toBe("public-1");
    expect(publicSlug("/q/public-1?result_token=x")).toBeNull();
  });
  it("shares one key and one deferred request, then locks unknown outcomes", async () => {
    let resolve!:()=>void; const deferred=new Promise<void>((done)=>{resolve=done}); const flight=createPublicSubmissionFlight(()=>"k".repeat(43)); const send=vi.fn(()=>deferred); const first=flight.submit(send); const second=flight.submit(send); expect(send).toHaveBeenCalledTimes(1); expect(first).toBe(second); resolve(); await first; expect(flight.submissionKey).toBe("k".repeat(43)); const failing=createPublicSubmissionFlight(()=>"x".repeat(43)); await expect(failing.submit(()=>Promise.reject(new Error("network")))).rejects.toThrow("network"); await expect(failing.submit(()=>Promise.resolve())).rejects.toThrow("unknown");
  });
});
