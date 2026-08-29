# Actual sealed member-usage source preflight

V2-only read, no V1 connection or target write. All810554 rows passed source/payload/field HMAC, strict20-field decoding and consecutive ordinals. Runtime111.26s, GOMEMLIMIT128MiB; streaming without a whole-table slice/map.

Two initial formats correctly failed closed at row1. Actual sealed v1_archive_tables.pk_columns is `[generation,unionid,owner_userid]`; the local discovery manifest's reordered PK was not authoritative. SourceKeyHMAC uses exact PostgreSQL `jsonb_build_array(...)::text` bytes, including comma spaces, not compact Go JSON. Both incorrect order and compact JSON have negative unit tests; HMAC checks were never relaxed.

- Adapter source fix89ceeeec; integration814586cd.
- Actual test binary SHA256 `9a0ee986d4714b496be256bfa7761467afc3390e23e3926e65422a895f6485d4`.
- Remote log `/home/ubuntu/p4-hxc-member-usage-history/pg-key-89ceeee/preflight.log` SHA256 `b3a2ab936ecec05f97d153116b7e430c98b6abea8a4e4b29da301b678234f6e0`.
- Ordered ordinal/key/payload/field SHA256 `532eb50fc3ba4a0b6297cbcd71de66aa77c3f82e420c9e18e10d35b2e61b448f`.

This proves source adaptation only. Generic archive-terminal matching, formal typed store/import/replay/reconciliation and user-readable deployment are not yet accepted. No current member rights, owner, registration or Provider outcome is inferred from historical generations.
# Fresh-target statistics during rehearsal

At23750 imported rows, the fresh receipt table had no ANALYZE statistics. EXPLAIN selected the run-scope index and then filtered the source digest; index statistics showed1128125000 heap fetches. Root ran ANALYZE only on the isolated receipt and HXC target tables, with10s lock/30s statement timeouts. The complete primary-key lookup then replaced the repeated run scan, and the same binary continued without changing any business row, schema, or validation.

The runner now analyzes those two tables once after its first bounded batch, within that transaction. The current rehearsal remains binary f0dc348f plus the recorded manual statistics maintenance. Final updated-binary replay and reconciliation are still required before candidate closure; this note does not claim completed810554 import or formal deployment.
