# HXC member-usage generation history

Scope: preserve all810554 sealed observations in public/ai_audience_hxc_member_usage_projection, not a current entitlement, owner or refresh implementation. Minimal A; no Provider calls or V1 mutation. Reuse HXC history UI and the existing archive reader, writer receipt and reconciliation patterns.

The actual V2 sealed v1_archive_tables.pk_columns uses PK order generation,unionid,owner_userid. The local aicrm-v1-full-manifest-20260827.json instead lists generation,owner_userid,unionid and is not authoritative for this sealed run. The first actual preflight rejected the first row; the adapter now follows the live sealed metadata and has an old-order rejection test. Preserve all20 fields, signed generation, nullable source times and JSON literalnull. Source unionid/owner/mobile hash/payload are private. Do not guess Customer/Staff links.

Execution: bounded streaming read-only full preflight; private typed Port/store/writer; fixed-size batch imports using existing same-transaction receipts; replay and streaming full-field reconciliation; read-only API and existing history-page kind. No new checkpoint framework. Review actual target/receipt/index disk needs before isolated rehearsal; no large anonymous volume or root-disk dump. Final migration number is assigned serially after prior verified baseline.

Gate: package/local checks -> concentrated PR CI -> exact final candidate Full green -> merge -> exact-main Full green -> fresh V2 backup/import/replay/double reconciliation/manual-test deployment. V1 and old traffic untouched; all real business effects disabled. These historical rows alone do not close current HXC funnel Excel capabilities.
