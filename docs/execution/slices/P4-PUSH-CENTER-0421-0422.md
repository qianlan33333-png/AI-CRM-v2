# P4 Push Center 0421/0422

## Scope

This integration completes only the two remaining read routes in the seven-route Push Center board:

| Legacy route | Canonical operation | Ownership |
| --- | --- | --- |
| LEGACY-API-0421 `GET /api/admin/push-center/sections` | `getLegacyPushCenterSections` | global `operations.read`, session, no CSRF |
| LEGACY-API-0422 `GET /api/admin/push-center/stats` | `getLegacyPushCenterStats` | global `operations.read`, session, no CSRF |

PR #200 already supplied 0416–0420 (jobs list/detail, cancel, reconciliation, retry). This slice adds exactly two routes; it does not reimplement or recount that merged five-route subset.

## Read model and safety boundary

Migration `00044_push_center_read_model.sql` owns a detached Push Center read projection and a readiness row. The initial readiness value is false, so missing production projection data returns the frozen HTTP 200 degraded payload rather than a fake zero statistic. The repository reads only this projection inside the platform UoW; it does not read another domain's owner tables.

The routes expose the frozen 15 trimmed filters, 13 ordered section definitions, nine ordered status definitions, effective-status aggregation, and the read-only runtime summary seam. `runtime_queue={}` is the safe result when no lane-summary adapter is available. The slice creates no queue, worker, Event, provider call, retry, receipt, or send operation. `queued`, `accepted`, `provider_accepted`, and `unknown_after_dispatch` are not converted into execution or delivery claims.

## Verification receipt

The required isolated PostgreSQL 16.14 proof is `43→44→43→44`. It verifies the no-tenant/no-foreign-key projection schema, default fail-closed readiness, all filter classes, `sent` shadow aggregation, the legal `reconciled` effective-status bucket, invalid-time degradation, concurrent UoW reads, and a 200,000-row trigram query plan with no Seq Scan.

This card records an integration candidate only. External-effects routes remain outside this two-route addition and no real external effect has been demonstrated.
