# P4 Customer Profile Read V2

`P4-CUSTOMER-PROFILE-READ-V2-2026-08-26` restores the backend read capability
for the Customer Profile message section and adds a safe V2 Survey submission
projection. It contains no frontend work and performs no external call.

## Message records

`GET /api/admin/customers/profile/messages` resolves verified `unionid` and/or
corp-scoped `external_userid` hints to exactly one canonical customer. Missing,
ambiguous, or conflicting identity facts fail closed. The response contains
only `chat_type`, `msgtype`, and `send_time`; it excludes message body,
sender/receiver, external identity, Provider ID, attachment, and receipt.

The default bound is 30. `fetch_all=true` and the legacy UI value
`fetch_all=1` use the fixed bound 100; `false`, `0`, or omission keep 30. Other
values are rejected. This closes `LEGACY-S05-006` and `LEGACY-S05-007` at the
backend contract level.

## Survey submission projection

`GET /api/admin/customers/profile/questionnaire-answers` accepts verified
`unionid`, corp-scoped `external_userid`, or strict E.164 `mobile` hints. When
more than one hint is supplied, all must resolve to the same canonical
customer. The Survey-owned read model returns submission IDs, questionnaire
IDs, timestamps, score, and choice question/option IDs only. Mobile, free-text
answers, external identities, mutable labels, result tokens, and receipts are
excluded.

The response deliberately uses `submissions` and `submission_count`, with
`legacy_parity_status=v2_submission_projection_not_legacy_answers` and
`assessment_status=v2_assessment_unavailable`. Current questionnaire
definitions are mutable and cannot truthfully reconstruct historical
question/option text, while V2 has no immutable assessment-result snapshot or
assessment engine.

On 2026-08-26 the repository owner selected the minimal W1 A path: accept the
safe V2 customer read flow as the launch replacement, without adding sensitive
identity fields, an assessment model, or a separate risk-control subsystem.
Accordingly `LEGACY-S05-001/002/003/005/008/009` are closed as
`safe_v2_customer_read_replacement`, not as strict legacy 1:1 parity.

PR #544 connects the existing generated APIs to the Admin customer list and
Customer360 views. The list uses keyword, mobile, owner, tag, and opaque cursor
parameters; detail navigation uses numeric Customer OneID. The detail view
renders the safe context and survey projection, has explicit empty/truncated/
assessment-unavailable states, and maps a real 404 to a returnable missing
state. Other read failures remain errors and never fall back to Seed/Mock.

Focused and race tests cover identity mismatch, unsupported `user_id`, strict
mobile normalization, bounded archive reads, PII-closed JSON, deterministic
submission ordering, and unavailable dependencies. OpenAPI and Orval expose
the exact V2 distinction for the later concentrated frontend integration.

Local evidence for PR #544 is `npm run admin:adapter:contract`, `npm run
typecheck`, `npm run build --silent`, and `node web/scripts/e2e.mjs` (83/83).
PR CI, main merge, exact-main Nightly, deployment, and external effects remain
separate states until their own receipts exist.
