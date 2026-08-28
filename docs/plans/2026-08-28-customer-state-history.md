# 客户状态历史最小落表（123）

按20260827实际manifest及V2封存归档预检，处理class_user_status_current72、class_user_status_history199、class_term_tag_mapping9，共280条。旧静态mapping不作为当前源schema依据。当前snapshot源无id/PK，不能补造SourceID；其余源signed id/class_term_no保持原值，时间按UTC微秒，空文本、false、updated早于created不猜改。

复用Contact owner的三张typed不可执行历史表，不新增领域。三份source key/payload/field HMAC保留。客户名/owner/unionid和strategy/group/tag源字符串只供私有迁移保真，API JSON排除；target digest仍须覆盖这些字段，不能直接json.Marshal带json:"-"的Port后漏校验。actor/error/flags保持候选中的摘要；JSON字面null的flags合法，不还原成活配置。

不创建current customer/status/tag，不调用Provider、不产生事件或周期任务。无可信跨表父关联，不建猜测FK，也不推测当前OneID。Writer/Journal使用同一callerTx，sourceHex绑定SourceKeyDigest；Reader支持pool及裸Tx，优先callerTx，12个SQLC查询；最小6个AdminRead/human-session GET及只读页面，失败不Mock。

主代理串行Port/123/SQLC/ownership/OpenAPI/路由与CLI，叶代理只按限定路径开发。280来源真实首次/重放/双对账以及隔离PG回滚后，才进入集中PR候选Full→合并→exact-main Full；此前正式库不升级。V1绝对只读，零切流，真实效果关闭；历史落表不等于当前业务能力销项。

2026-08-28 隔离演练结果：仅 V2 的 network=none 演练容器升级至123；Contact真实PG事务/裸Tx读取/回滚测试通过。72条快照、199条变化、9条标签映射首次导入280，完整重放280；双次对账 selected/receipt/imported/verified 均280、archive/quarantine均0，seal `99c4abbc969c11e6290050108f06e3a3b89ed47cd4cba840fb1ca196a1e59599` 相同。旧static-tail 54条重对账仍为seal `9d5d3e4d50899b7f23983371ab471c2126f632d3c2dea097fae8b8e226f1db44`，没有改变旧口径。

Linux importer SHA256 `999be8a6569eceba5ca6fdaea5ec9964e8720566662e4e0ff5f53e89bad2af5f`；DDL123 SHA256 `23c73e81c0da853ab426f966647c2aa4354fac8b1172e58b2459d784df9a2718`。隔离库external_effects/event_log/river_job全过程0/0/0，未连接V1。全Go首次并行运行仅p2s11 API进程健康启动超时；原测试单跑及全仓原样重跑均通过，未修改断言或超时。race/vet、生成/所有权/OpenAPI/Orval/Matrix检查通过；Web383通过/0失败及Customer-state专项通过。正式123、集中PR双Nightly、普通用户登录及用户人工测试仍待完成。
