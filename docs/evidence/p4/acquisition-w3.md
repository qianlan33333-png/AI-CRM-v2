# W3 Acquisition compatibility evidence

Scope: `LEGACY-S05-012`, `LEGACY-S06-002`, `LEGACY-S06-004`–`010`, `LEGACY-S06-012`, and `LEGACY-S06-043`.

## Closed behavior

- WeCom tag catalog: credentialed Provider read/sync leaf with explicit failure and receipt semantics; production Provider execution remains `NOT_EXECUTED`.
- Channel drawer and form: real local entrant reads, create/edit/not-found behavior, carrier switching, saved-link copy/share, welcome-media references, and tag selection.
- Channel assets: versioned accept/queue/execute/read/reconcile state; only `executed` plus a returned `asset_url` enables open/download/copy. `accepted` or `queued` is never presented as Provider success.
- Channel staff: `GET /api/admin/channels/{channel_id}/acquisition-staff` reads WeCom `get_follow_user_list` and returns only its intersection with active local staff. It never creates staff. The existing idempotent Channel UoW persists ratio or 24-hour cap assignment.
- Callback and OAuth: verified callback ACK and OAuth state/session flows fail closed; HEAD/OPTIONS compatibility never fabricates a callback result.

## Repeatable checks

- `go test ./internal/contact/app ./internal/contact/http ./internal/wecom/client ./cmd/aicrm`
- `make generate-check openapi-p1-contract ownership-lint ownership-lint-test`
- `npm test`
- `npm run typecheck`
- `npm run admin:adapter:contract`
- `npm run transport:contract`
- `npm run capabilities:contract`
- `npm run orval:check`

All checks above pass on the W3 candidate worktree. The concentrated PR CI and candidate Full Nightly are still required before merge; exact-main Full Nightly is still required after merge. No production Provider effect is claimed by this document.
