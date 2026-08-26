# AI Audience inbound webhook (LEGACY-S06-027)

Migration `00101_ai_audience_inbound_webhook_receipts.sql` introduces the
Audience-owned, local-only receipt and client transport-replay facts. Both
digest columns are exactly 32 bytes; business-event replay is unique per
package and transport replay is unique per client. The down migration stops
with SQLSTATE `55000` whenever either table contains a fact.

`POST /api/ai/audience/packages/{package_id}/webhook` is public only in the
transport sense: it requires the four `X-AICRM-*` HMAC headers, validates the
exact raw body, and fails closed with `503` when the optional configured
webhook secret is absent. The accepted `200` is record-only. It creates no
automation send plan, external-effect job, Provider request, or delivery
claim. The retired outbound-subscription paths remain human-session protected
and return `410`; they do not reintroduce subscription CRUD.

The PG16 selected-database acceptance verifies a valid record, exact replay,
business payload conflict, transport replay conflict, unknown package,
same-unit-of-work event rollback, and the populated down guard. Run it with:

```sh
P4AIAUDIENCEINBOUNDWEBHOOK_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:55432/aicrm_test?sslmode=disable \
  make p4-ai-audience-inbound-webhook-acceptance
```
