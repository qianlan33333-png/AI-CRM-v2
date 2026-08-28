# HXC 历史观察正式落表（121，候选开发）

从已验证4904隔离工作树承接115–120本地验证组合；前序正式门禁全部通过后才提交集中PR并落正式库。

冻结归档8表2476行：6业务源2473（meta816、snapshot1326、activation149+142、lead13、batch27）；send_records_next2、hxc_send_config1继续仅归档。仅V2读取冻结归档，绝不连接或写V1。

采用HXC-owned五张typed历史表。两个activation源按source_table保留命名空间；snapshot只有unionid可以经已验证DM01解析nullable逻辑customer_id，不猜手机/其它身份关系，不建跨域FK。3个nullable DATE用YYYY-MM-DD字符串与SQL DATE保留日历原义。3类batch ref保留原source语义，不建可执行批次FK。源ID和计数有符号原值保留，不推测正数限制。

主代理串行Port、121、ownership、API/OpenAPI、生成与CLI集成；叶代理分工owner app/store与私有import/journal/reconcile。所有来源HMAC/ordinal/redaction校验，caller transaction写事实及journal，重放读实际目标并校验完整typed digest。

验证：包测试及PG事务回滚→2476首次/重放/双对账且current/event/job/effect不增→集中PR CI→最终精确SHA候选Full Nightly→绿灯合并→exact-main Full Nightly→仅V2部署。历史看板不等于当前HXC刷新/任务/群发/导出能力已销项。

2026-08-28 本地收单：10个AdminRead GET、生成客户端及funnel独立只读入口已接入；包级/race/vet、OpenAPI/ownership/source-policy基线38、生成一致性与Web383/0及专项通过。全仓首次遇到既有API启动时限失败，原日志保留；该测试连续3次及全仓复跑通过，未放宽时限。

V2网络隔离演练schema121：实际PG回环/整体回滚通过；首次导入2473+归档3、0隔离，完整重放2476；双对账2476/2476，seal `774edc6e46ca0f6d02ff24319348b0187b9c978c68535baae7d70aa5f8e05f7e`，effects/event_log/river_job前后0。修复了固定observed_snapshot标记的领域校验和fixture，不改变其它历史空文本。尚未提交本包PR、正式121或人工登录验收。
