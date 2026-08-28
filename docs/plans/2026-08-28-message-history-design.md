# V1 聊天历史最小正式落表

- 来源仅为 V2 已封存的 `public/archived_messages` 53,882 行；不连接或改动 V1。
- 迁入 WeCom-owned `wecom_v1_message_history`，不强塞要求 customer/sent_at 非空的当前消息表，不触同步、事件、队列或 Provider。
- 源主键保留为 `source_id`；nullable seq/content 保持 NULL 区别。原正文、发送人/接收人/外部身份、raw_payload 均由既有加密归档及原 payload digest 完整绑定。正式读取只给脱敏正文，不把整个正文替换成占位文本。
- 现有 53,882 行的发送时间都是 V1 SDK 未记录时区的 civil clock；保留原字符串和 `civil_unzoned`，`sent_at=NULL`，禁止猜成 UTC。确有显式偏移的未来输入才保留真实 instant。
- Customer 只能由经验证的 DM01 lineage/receipt/actual-row digest 解析；不能凭 sender/receiver/房间号猜测。已核对 V1 archive SDK：unionid 来源于外部联系人的身份解析，群聊中仅表示历史关联而非群成员或整条群聊的所有者。复用严格的 unionid DM01 resolver，无可靠映射则 NULL。
- Writer、正式表与导入 receipt 同一事务；重放取真实 target 全字段 digest 比较。对账同时验证源 archive receipt、正式行和 journal，不能用导入计数代替。
- 只读管理入口与当前聊天分离：分页 GET、真实空态/错误态、历史映射与无时区标签；不开放发送/同步/重试。HTML 200 不算浏览器验收。
- Schema 115 仅预留于隔离 worktree；必须等 113/114 集成、候选 Nightly 和 exact-main Nightly 门禁完成后才能正式迁移。

## V2 隔离演练

- network=none PostgreSQL 16.14 已完成53,882条导入、0隔离、53,882条全量重放；两轮逐行对账均53,882/53,882，seal `0fcda5e339fe13dc7305f97e5222636595b7747511d6a0dc3cbe8169a7f6972c`，第二轮replayed=true。
- 53,663条经DM01映射，219条customer_id保持NULL；全部53,882条civil_unzoned、sent_at=NULL。external_effects/event_log/river_job均0未变。
- Owner SQLc真实PG roundtrip与强制rollback测试通过。尚未正式迁移、部署或完成正常浏览器登录验收。
