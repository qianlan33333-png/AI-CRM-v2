# P4-IMAGE-FACETS-0358 evidence

## Candidate receipt

- exact-main base: `90177c280021e7d0b6ea033e7e3203fa5a48620c`
- base tree: `bd08025eeab183cf1d2e7aee9a8c8ae663955ec8`
- Pro conversation: `https://chatgpt.com/c/6a82e9f1-d954-83ea-a7e4-12be96aaebb1`
- input ZIP: `93563` bytes, SHA-256 `24adda764d503085b055304441b6dd26ae66f8ecfafa61b15dbc7d7ceeee08bb`, gitleaks PASS
- candidate patch: `27945` bytes, SHA-256 `7b7ac7016fe99c5984e3d446c60be56d2f5bfaa2005414ece35bd06871dbea57`
- candidate handoff: `14164` bytes, SHA-256 `2b0f30e5a5b8f135c50d3b63931502971d2c98d1a4359c2f2991d21bba3ee0cd`
- candidate status: untrusted input until clean replay and local/remote gates complete.

## Primary Codex review

- patch applied cleanly only in a detached clean worktree at the exact base; dirty historical root was not used.
- all 9 candidate paths were reviewed; no migration, provider, worker, dependency, tenant, OneID or external-effect path was present.
- canonical OpenAPI, SQLc and Orval generation was executed; generated output was not hand-edited.
- primary Codex added a separate PostgreSQL 16.14 acceptance test and declarative acceptance entry to prove the actual Media repository read path and unchanged durable fact counts.
- `LEGACY-S07-049` remains open because its list API and UI flow are outside this API-only slice.
- first full repo-contract run rejected the Pro candidate test because its redundant forbidden-key list named runtime tenant identifiers. Primary Codex removed only those two redundant literals; the strict nine-key equality assertion still proves that no extra response field is possible. This is recorded as one slice-induced correction.
- PR #242 Candidate Merge Guard correctly rejected the first head because natural-language zero-migration evidence did not include the required formal `no_schema_or_external_effect` declaration in both the added Mapping and Slice card. Repair adds only those two declarations and freezes the slice at two slice-induced corrections; no runtime or product contract changes.

## Local verification

- focused application test: PASS
- focused handler/router test: PASS
- focused application/store/handler race: PASS
- PostgreSQL 16.14 acceptance on isolated `55431/aicrm_test`: PASS (normal and race)
- `go test ./...`: PASS
- canonical generation and repeated SQLc/Orval generation: PASS with no worktree/index drift
- acceptance manifest validation: PASS, 46 entries
- repo-contract: PASS after the single recorded candidate-test correction
- Web CI: PASS, 13 test files / 226 tests, production build and high-severity audit with zero findings
- pre-commit `make ci-go`: all preceding generated/full-test/vulnerability/P0/P2/P3 gates passed, then stopped at the intentionally clean-index-only release-binary gate; the complete command is rerun from the committed clean index before Push
- changed-file secret scan and committed-diff scan: pending before the local checkpoint commit

## External boundary

- historical/production database: NOT_EXECUTED
- staging/production deployment: NOT_EXECUTED
- provider, URL fetch, blob read, real WeCom/payment/outbound: NOT_EXECUTED
- status: INTEGRATING, not MERGED or DEPLOYED
