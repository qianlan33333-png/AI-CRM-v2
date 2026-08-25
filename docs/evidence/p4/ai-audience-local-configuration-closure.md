# P4 AI Audience Local Configuration Closure (00084)

`P4-AI-AUDIENCE-LOCAL-CONFIGURATION-00084-2026-08-25` closes a V2-native
local configuration capability. It is deliberately narrower than the legacy
AI Audience page flows:

| V2 backend capability | Contract | Explicit boundary |
| --- | --- | --- |
| immutable typed Segment snapshots | `GET`/`PUT /api/admin/ai-audience/packages/{package_id}/configuration` | fixed schema version, canonical definition/package version/digest; no legacy template catalogue |
| deterministic local evaluation | `GET .../configuration-preview`; `POST .../configuration-materialize` | count/digest lineage only; no contact IDs, provider, enqueue, send, acceptance, or delivery claim |
| Contact-owned sender validation | existing `GET`/`PUT .../senders`; write uses `EligibleStaffReferenceReader` in the caller UoW | no copied staff SQL, directory sync, credential, provider authority, or send |
| versioned local automation binding | existing binding API with migration-00084 CAS field | no agent start or runtime invocation |

The similarly named legacy routes remain classified as `V2_NEW_SEMANTICS` in
the route ledger. In particular, legacy Matrix rows `LEGACY-S06-022`,
`LEGACY-S06-024`, and `LEGACY-S06-026` remain `NOT_STARTED/NOT_RUN`; their
complete old flows include the legacy template catalogue/SQL semantics,
external-directory synchronization, or send-record detail that this local
package does not implement. The removed send-record projection had no trusted
writer or EER lineage and therefore supplies no V2 capability claim.

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
database and runs the real HTTP → handler → service → caller UoW →
Contact/Automation ports → Segment evaluation → mutation/event/completed
receipt path. It proves exact replay, changed-payload conflict, no duplicate
events, failed-write rollback, deterministic preview/materialize lineage,
empty `84 → 83 → 84`, and populated-fact rollback guards. No placeholder
migrations or external calls are introduced by this package.
