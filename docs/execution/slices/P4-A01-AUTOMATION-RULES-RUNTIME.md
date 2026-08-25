# P4 A01 Automation Rules Runtime

This backend-only package restores the smallest complete Automation closure:

- administrators create, update, pause, activate, or archive closed
  `customer.tag_applied` rules with idempotency receipts;
- each mutation publishes an immutable rule-version snapshot;
- the existing `automation.tag-trigger.v1` delivery consumes the source event
  and, in the same transaction, creates a unique enrollment plus an immutable
  action snapshot;
- `record` completes a local receipt; `outbound_message` remains queued for
  Outbound plus External Effects Runtime and has no provider implementation,
  call, retry, or success claim in Automation;
- read endpoints expose only rule configuration and redacted execution state.

The package excludes a rule editor UI, arbitrary DSL, prompt/provider work,
historical migration, and any real external dispatch.  Provider result,
retry, `outcome_unknown`, and reconciliation stay owned by EER after an
explicit Outbound handoff.

Acceptance: `make p4-automation-rules-runtime-acceptance` with
`P4AUTOMATIONRULES_TEST_DATABASE_URL` set to the approved dedicated PG16 DB.
