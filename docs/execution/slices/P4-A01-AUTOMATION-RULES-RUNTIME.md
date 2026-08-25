# P4 A01 Automation Rules Runtime

This backend-only package restores the smallest complete Automation closure:

- administrators create, update, pause, activate, or archive closed
  `customer.tag_applied` rules with idempotency receipts;
- each mutation publishes an immutable rule-version snapshot;
- the existing `automation.tag-trigger.v1` delivery consumes the source event
  and, in the same transaction, creates a unique enrollment plus an immutable
  action snapshot;
- `record` completes a local receipt; `outbound_message` requires the existing
  V2 `text.notice.v1` template reference (no legacy message body was proven),
  then atomically creates an opaque `eer_N` binding and an `outbound` River
  job through EER. The persisted EER envelope contains only immutable digests;
  it is not a Provider call, provider receipt, or delivery proof;
- the default River adapter is explicitly disabled and writes only a local
  `final_failed` projection. A transport ambiguity projects
  `outcome_unknown`, returns success to River to prohibit automatic retry, and
  can advance only through the same EER lease/fence/evidence-digest manual
  reconciliation boundary;
- read endpoints expose only rule configuration and redacted execution state;
  the reconcile endpoint returns a local projection, never delivery proof.

The package excludes a rule editor UI, arbitrary DSL, prompt/provider work,
historical migration, and any real external dispatch. This PR claims EER and
River local acceptance only; it does not claim Provider execution, receipt,
or delivery.

Evidence classification:

- `docs/feature-matrix.csv`: 0 diff. The closed V2 rule/runtime has no proven
  legacy 1:1 row, so existing Automation Agents and Group Ops rows are not
  reused or marked complete.
- `docs/replacement/legacy-route-disposition-ledger.csv`: 0 diff. No legacy
  route is reclassified by inference; the package exposes native V2 APIs.
- `docs/replacement/data-migration-ledger.csv`: 0 diff. Historical automation
  candidates retain their existing evidence status; no source data migration
  has run.
- `docs/evidence/p4/backend-capability-ledger.md`: records A01 as a V2 backend
  capability outside the frozen legacy-local 10/73 denominator.

Acceptance: `make p4-automation-rules-runtime-acceptance` with
`P4AUTOMATIONRULES_TEST_DATABASE_URL` set to the approved dedicated PG16 DB.
