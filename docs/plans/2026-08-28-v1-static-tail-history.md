# V1 静态尾表最小历史落表（122，隔离演练完成）

仅处理冻结归档五表54行：Media群邀请4、Product商品图片切片46、OperationCycle策略1/版本2/文档1。不创建当前邀请链接、商品页面、周期策略或运行任务，原执行JSON/URL/文档正文保留封存归档。

采用三owner五张typed历史表，全部保留源signed ID、计数、原状态、空文本、nullable和UTC时间。周期版本/文档只建立同owner历史父ID；其余源引用不猜映射为当前V2外键。SourcePayloadDigest绑定原archive PayloadHMAC，不能用原JSON SHA替代；候选SealedSourceDigest必须等于该HMAC。

主线串行Port/DDL122/SQLC/ownership/API生成与CLI组合；叶线只写对应owner app/store及私有import/journal。沿既有UoW+journal、typed目标摘要/重放、失败关闭、分页GET，不新增通用平台或风控体系。

2026-08-28已用修正后的4b02e7bb候选在V2 schema121 network-none只读预检54候选/0隔离，effects/events/jobs0。新Port/DDL只是开发契约，不算正式落表、部署或Excel当前能力销项。后续须同Tx真实PG回环/回滚、54首次/重放/双对账、集中PR CI及候选Full、exact-main后才部署122。

2026-08-28 16:09后实际完成：schema122隔离PG三owner存取/回滚全部PASS；54首次与54全量重放、双次selected/receipt/imported/verified=54，archive/quarantine=0，seal `9d5d3e4d50899b7f23983371ab471c2126f632d3c2dea097fae8b8e226f1db44` 一致。importer SHA256 `15659367f7c81c8bb5d64eb09f6b63edafbb37d5f7d1e96d4db33cddedebacfc`，effects/events/jobs每次0|0|0。源码接通10个AdminRead/human-session只读GET及config.html?static_history=1；五类型列表/详情/父历史筛选，生产失败无Mock，全部安全DTO可查看。

相关Go/race/vet、OpenAPI/Orval、SQLC生成/ownership/source-policy baseline38、Matrix/replacement通过；Web383通过/0失败，静态历史专项通过。以上仅为本地与V2隔离演练证据，正式库仍117，122未合并部署。前序118–121及本包仍须各自最终候选Full绿灯后合并、exact-main Full绿灯后正式部署；不计当前能力销项，不触V1/切流。
