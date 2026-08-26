# P4 AI Audience outbound receipts

`P4-AI-AUDIENCE-OUTBOUND-RECEIPTS-2026-08-26` closes the backend read path for
`LEGACY-S06-026`. It is built on the existing Campaign handoff and EER dispatch
facts rather than recreating the removed legacy send-record projection.

## Capability boundary

- `GET /api/admin/ai-audience/packages/{package_id}/send-records` returns a
  bounded, newest-first page of records belonging to that exact package.
- `GET /api/admin/ai-audience/packages/{package_id}/send-records/{record_id}`
  returns one record only when it belongs to the selected package.
- The response exposes local record ID, state, technical attempt count, safe
  failure class, receipt/result/delivery booleans, and timestamps. It never
  exposes customer OneID, sender userid, external userid, Provider msgid,
  payload, raw response, or receipt digest.
- `provider_result_received`, `business_call_dispatched`,
  `real_external_call_executed`, and `delivery_proven` are aggregated only from
  persisted Provider-attempt receipts. A queued/accepted record is never
  presented as a successful send.

Migration `00099_audience_outbound_receipts.sql` adds Audience relationship
owner snapshots, sender and target exclusion reasons, and immutable Provider
attempt evidence. Crash recovery reads the exact persisted attempt receipt:
an existing receipt completes the recorded attempt, while a missing receipt
becomes `outcome_unknown`; neither path calls Provider a second time.

## Verification state

PostgreSQL 16.14 acceptance covers migration `78/92/94/99`, an actual Audience
package snapshot, Campaign handoff, Contact-policy block, qualified dispatch,
EER queue, controlled fake Provider execution, immutable receipt, and the safe
list/detail readback for blocked, queued, and executed records. Focused Go,
race, source-policy, OpenAPI/Orval, SQLC, and generated-source checks are part
of the same package gate.

This document records branch-local capability evidence only. PR required CI,
main merge, exact-main Nightly, deployment, real WeCom execution, Provider
acceptance, and real delivery/reconciliation are separate and not claimed.
