# 营销状态历史最小落表（124）

V2封存归档的marketing current/history 77/224、value segment current/history342/344，共987条预检候选、0隔离。归现有Segment owner四张不可执行typed历史表；不生成当前CustomerID、member、score、trigger、事件或队列。

SourceID按源BIGINT signed保真，不加>0限制；nullable person/batch/submission仅私有源引用，不猜FK。源last_*等TEXT原样，不猜日期；rank/score按源INTEGER int32边界。三个归档HMAC、外部标识摘要及源JSON摘要均private；target digest必须明确覆盖所有private字段，不可直接marshal带json:"-"的Port。

ValueSegment候选的SourcePayloadDigest表示源JSON摘要，映射正式Port的StatePayloadDigest，不能与归档envelope的SourcePayloadDigest混淆。source key/payload/field必须绑定同一归档记录；无parent过滤；最小8个鉴权GET和只读UI，失败不Mock。主代理串行Port/DDL124/SQLC/OpenAPI/路由/生成/CLI；叶代理限定路径。

987真实首次、重放及双对账必须通过，source/target字段保真且事件/外部效果不增后，仍需集中PR候选Full、合并后exact-main Full才可正式V2部署。当前仅开发；历史落表不替代Excel当前能力或人工验收；V1零写，零切流。

2026-08-28隔离V2实际结果：network=none演练容器08:52:22Z升级124；首次PG测试发现直接Store夹具EnteredAt未转UTC微秒，最小修复仅规范化夹具，Writer归一化测试及断言不变。固定二进制重跑PG四类事实创建/读取/裸Tx/回滚通过。

77/224/342/344共987首次导入，0隔离；全量重放987；双次selected/receipt/imported/verified987，archive/quarantine0，seal `12778a359cbff0dfa9be7c205d5a3adc3005843e3f8a9659603bb037c5e7bb42`相同。旧Customer-state280重对账seal仍`99c4abbc969c11e6290050108f06e3a3b89ed47cd4cba840fb1ca196a1e59599`。effects/event_log/river_job前后后0|0|0；无V1连接、无current写入。

DDL124 SHA256 `90c686ec62d5a2d887b9a3bf65d79a22209a96df406844fde58289be5746fc7e`；PG测试二进制`33307718a94b7811cc93ef71d2e49d622b588c72ef906f486e4dbcc97f331f52`；Linux importer `68c9d5c7d6923b4814113d9d2e89baf8889e151e5fd0e3163a8a200e35bec989`。Go全仓、相关race/vet、OpenAPI/SQLC/Orval/ownership/Matrix通过，source-policy保持38；Web383/0加专项、adapter/transport通过。正式124和集中PR双Nightly尚未执行。
