# C1 W3/W4 strict closeout evidence

Scope: the 11 strict pending rows from W3 and W4. This supplement improves
repeatable local behavior but does not promote any row to `IMPLEMENTED`.

## Added behavior

- WeCom acquisition-link update uses the official full-replacement request and
  performs a provider readback before returning the updated link. Delete returns
  only provider acceptance. Both writes preserve `outcome_unknown` and never
  retry after the provider boundary.
- WeCom tag catalog, acquisition asset/staff reads, and callback verification
  now have explicit malformed/readback failure tests. They never fall back to
  local or fixture data after a provider/protocol failure.
- AI Audience exposes one save -> configuration snapshot -> preview action.
  Materialization requires a current preview; an empty preview requires an
  explicit confirmation and then the existing separate local-materialization
  confirmation. None of these actions creates an outbound or Provider effect.

## Strict blockers kept open

- `LEGACY-S05-012`, `S06-004`, `S06-008`, and `S06-043` still require controlled
  WeCom/Staging evidence.
- `LEGACY-S06-012` still has no safe public HTTP write contract or durable
  idempotency receipt; WeCom does not provide the legacy enable/disable actions.
- `LEGACY-S06-022` still lacks the legacy template catalogue/parameter and
  historical filter semantics.
- `LEGACY-S06-044`, `S07-085`, and `S07-090` still require Provider execution,
  receipt, or reconciliation evidence; accepted/queued/local review is not send
  success.
- `LEGACY-S07-082` has no approved rule for selecting or aggregating multiple
  touch plans into one campaign member/status list.
- `LEGACY-S07-092` has no Campaign audit source carrying `trace_id` and a safe
  session audit reference. Existing Push Center trace filtering is not a
  substitute for that audit contract.

## Repeatable checks

- `go test ./internal/wecom/client ./internal/wecom/tag ./internal/wecom/callback ./internal/contact/app ./internal/contact/http ./internal/automation/http ./internal/campaign/app ./internal/outbound/app ./internal/externaleffects/app ./cmd/aicrm`
- `npm run typecheck`
- `npm run admin:adapter:contract`
- `npm run transport:contract`
- `npm run e2e` (`168` passed, `0` failed)
- `git diff --check`

The frontend checks ran on the leaf worktree containing the exact frontend
commit because the concentrated integration worktree had no installed Node
dependencies. Concentrated PR CI and candidate exact-SHA Full Nightly remain
mandatory before merge; exact-main Full Nightly remains mandatory after merge.
No Staging, production, or real Provider effect was executed.
