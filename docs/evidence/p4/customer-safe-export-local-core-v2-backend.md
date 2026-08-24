# Customer Safe Export Local Core：V2 后端能力账本

本包新增三个 native Contact operation：`createCustomerSafeExport`、
`getCustomerSafeExport` 与 `downloadCustomerSafeExport`。它们冻结当前 Customer
筛选结果到 Contact-owned 本地 snapshot；不恢复 `/admin/user-ops`，不改
`LEGACY-S06-039` 的 `DEPRECATED` 状态，也不把该 legacy 行计为完成。

导出限制为 10,000 行。持久行只含 customer id、display name、owner/stage/channel
id 和本地时间；没有 mobile、external_userid、unionid、avatar、extra、Provider 或
外部调用。CSV 在下载前做公式前缀保护。admin/ops 是 global；sales 被绑定到本人
staff scope，并在下载前复核当前 customer owner，漂移时 409 且不输出部分 CSV。

`POST` 需要 session CSRF 与 Idempotency-Key；actor/key/payload 的 receipt、snapshot
写入和 event log 在同一 UoW。相同 key/payload replay 同一结果，payload 不同则 409。
GET metadata/download 均 actor-bound、no-store。本地 event 仅为事实记录，未注册
provider/delivery binding；`real_external_call_executed=false` 绝不表示外部导出或发送。

验证包含 app/HTTP 的 RBAC、owner scope、receipt replay/conflict、closed CSV 和 owner
drift，以及 PG16.14 migration fresh up/down/up 与 nonempty-down guard。通过仅证明本地
能力和数据库事实，不证明 Matrix、main/Nightly、部署或外部效果。
