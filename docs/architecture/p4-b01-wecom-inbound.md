# P4 B01 WeCom inbound ownership

## Boundary

`internal/wecom` owns the verified callback envelope, durable local inbox,
WeCom directory-page handoff, and critical River job payload. It may call only
the existing `internal/identity/port` contract for verified
`wecom_external_userid` facts. Identity owns normalization, attribution,
pending/conflict state, customer merge/timeline writes, and all related
receipts.

The callback transport remains the provider-facing cryptographic boundary at
`/api/wecom/events` and `/wecom/external-contact/callback`. It verifies and
decrypts before B01 inserts a local inbox fact. The HTTP request never performs
Identity processing or a Provider write.

## Runtime sequence

```text
WeCom callback
  -> verify/decrypt/envelope validation
  -> wecom_contact_inbox INSERT ON CONFLICT DO NOTHING
  -> critical River job INSERT in the same transaction
  -> encrypted success ACK
  -> critical worker claims inbox with lease/fence
  -> IdentityContactProcessor -> identity.Ingest port
  -> attributed | pending_identity | conflict local terminal fact
```

Directory sync follows the same local handoff contract. Each
`external_userid` is inserted and queued before the successor cursor is
advanced. A failed handoff therefore leaves the cursor unchanged and is
recoverable on restart.

## Non-effects

B01 does not call a WeCom write API, send messages, create welcome/tag effects,
or use `external_effects` as an inbound record. An encrypted ACK, inbox row,
River job, Identity receipt, and local terminal state are local facts only;
none is Provider delivery or external success evidence.
