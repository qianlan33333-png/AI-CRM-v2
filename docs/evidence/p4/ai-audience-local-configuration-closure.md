# P4 AI Audience Local Configuration Closure (00084)

`P4-AI-AUDIENCE-LOCAL-CONFIGURATION-00084-2026-08-25` closes a V2-native
local configuration capability. It is deliberately narrower than the legacy
AI Audience page flows:

| V2 backend capability | Contract | Explicit boundary |
| --- | --- | --- |
| immutable template/filter snapshots | `GET`/`PUT /api/admin/ai-audience/packages/{package_id}/template-config` | no legacy template catalogue, preview, activation, or execution |
| hardened local sender references | existing `GET`/`PUT .../senders` with migration-00084 storage | no directory sync, credential, provider authority, or send |
| PII-minimal send-record projection | `GET /api/admin/ai-audience/packages/{package_id}/send-records` | no content, recipient/sender ID, Provider result, send, acceptance, or delivery claim |
| versioned local automation binding | existing binding API with migration-00084 CAS field | no agent start or runtime invocation |

The similarly named legacy routes remain classified as `V2_NEW_SEMANTICS` in
the route ledger. In particular, legacy Matrix rows `LEGACY-S06-022`,
`LEGACY-S06-024`, and `LEGACY-S06-026` remain `NOT_STARTED/NOT_RUN`; their
complete old flows include template preview/catalogue, external-directory
synchronization, or record detail that this local package does not implement.

`LEGACY-T14-017` and `LEGACY-T14-018` remain pending migration decisions.
Migration 00084 creates only new V2 local storage and performs no legacy-data
import, inference, provider call, or external effect.

Verification is local only:

```text
make generate-openapi generate-sqlc generate-orval
make generate-check openapi-p1-contract feature-matrix-contract migration-validate replacement-baseline-contract legacy-route-export-test
go test -race -count=1 ./internal/segment/legacyaudience/... ./cmd/aicrm
P4AIAUDIENCE_TEST_DATABASE_URL=... make p4-ai-audience-local-configuration-acceptance
```

The acceptance creates only the dedicated `aicrm_test_ai_audience_00084`
database, proves empty `84 → 83 → 84`, runs the PG16 repository concurrency
test, and verifies that a populated configuration receipt prevents rollback.
No placeholder migrations are introduced by this package.
