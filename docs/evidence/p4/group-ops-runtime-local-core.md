# P4 Group Ops Runtime backend evidence

Baseline: `origin/main=4ad7edb126715401776a90502d289795c154c596`.
Local package commits: `6cdb8592` and `bdbd1e4a`. Migration: `00085`.
This evidence is local-branch evidence only; it is not a merge, deployment, Provider call, or delivery receipt.

## Capability boundary

- Existing `00063` plans, members, group assets, nodes, webhook descriptor, and content preview remain the local configuration source.
- `00085` adds immutable runs and executions, group-directory and refresh receipts, node material references, and an External Effect Registry owner/kind for `group_ops_broadcast`.
- Run-due, broadcast, and webhook acceptance atomically bind fixed content and material snapshots to EER. A `202` means EER acceptance only. It does not mean Provider acceptance or delivery.
- Execution projection keeps `provider_accepted` and `delivery_proven` independent. `outcome_unknown` has no automatic retry transition and can move only through the manual, lease-fenced reconcile command.
- Group and operation-member refresh use an injected read-only source. The production composition root injects no source, so refresh returns `provider_disabled` and performs no Provider call.
- Public broadcast and webhook protocols use an injected authenticator. The production composition root injects no authenticator, so both routes fail closed with `503`; this package does not invent the pending API-client JWT or webhook-HMAC credential policy.

## API and route evidence

The exact backend routes are registered and described in OpenAPI, including:

- plan list/create/get/PUT/delete/enable/disable, plan group bindings, standard nodes, webhook descriptor, run-due preview and acceptance;
- group directory and picker read/explicit refresh;
- `GET/POST /api/admin/common/operation-members` for `scope=group_ops`, with USER OPS responsibility folded into this package and no separate board;
- `POST /api/automation/group-ops/broadcast` and `POST /api/automation/group-ops/webhooks/{webhook_key}`;
- execution projection and manual reconcile as native runtime operations.

`docs/api-mapping.jsonl` and the deterministic route/protocol ledgers carry 25 exact Group Ops legacy mappings. `LEGACY-API-0165` remains `DEFERRED_POST_LAUNCH`; the native execution projection does not forge that frozen mapping. `LEGACY-S06-036` and the other cancelled USER OPS rows remain `DEPRECATED`.

## Migration evidence

- PG server: PostgreSQL `16.14` (`server_version_num=160014`).
- Fresh migration reaches waterline `85`.
- A populated Group Ops runtime table blocks down migration and leaves waterline `85`.
- Empty rollback and reapply complete `85 -> 78 -> 85`.
- Store integration verifies immutable content/material snapshots and EER binding in one transaction.
- No legacy production data import or backfill ran. The data-migration ledger is therefore unchanged.
- This isolated lane intentionally lacks reserved migrations `00079` through `00084`; `migration-mapping-contract` must be rerun only after root serial integration restores the contiguous migration set.

## Verification receipts

- `make p4-group-ops-runtime-acceptance` — PASS on PG16.14, including race tests, populated down guard, and empty down/up.
- `scripts/ci/run_selected_go.sh selected groupops,segment,externaleffects,composition false` — PASS.
- `scripts/ci/run_selected_database.sh selected groupops` — PASS, including `generated-check` and migration integration.
- `make openapi-p1-contract replacement-baseline-contract p1-reconciliation-contract feature-matrix-contract ownership-lint` — PASS for the gates available in this isolated lane.
- OpenAPI, sqlc, and Orval were generated; generated source hashes match.

## Matrix and external-effect statement

`LEGACY-S06-028` through `LEGACY-S06-035` are recorded as `IN_PROGRESS/NOT_RUN`: the backend package is locally verified but has no PR/merge receipt, so the Matrix deliberately does not claim `IMPLEMENTED` or synthetic/staging/production closure.

`PROVIDER_DISABLED`; `PROTOCOL_AUTH_INJECTION_PENDING`; `REAL_PROVIDER_NOT_EXECUTED`; `REAL_WEBHOOK_NOT_EXECUTED`; `DELIVERY_NOT_PROVEN`; `PRODUCTION_DATABASE_NOT_EXECUTED`; `DEPLOYMENT_NOT_EXECUTED`.
