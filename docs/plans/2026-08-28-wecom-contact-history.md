# 企微联系人历史最小落表

两表归 Contact：事件日志56880、跟进关系50872。已在V2 network=none演练库只读预检107752候选、0隔离；未写正式表。源码基于最近已验证dcf28fe，119–124由前序集中交付；125迁移号仅预留，不能跳过前序门禁或直接在正式库执行。

保留签名范围的原始ID、nullable整数时间和历史状态；不猜OneID，不折叠多跟进人为一个owner，不重放回调。源HMAC、身份/错误/原文摘要及Follow State均私有；原始载荷留在认证归档。领域Writer需同一事务写历史和回执，重放必须全字段摘要一致；Store仅通过SQLC、Reader失败关闭。

已新增四个AdminRead GET（事件/关系列表与详情）和一个只读页面，不提供执行、同步、分配或重试按钮。时间整数单位未知，界面不作日期转换。主代理串行Port、DDL、SQLC、OpenAPI、路由、生成及CLI；叶代理仅实现各自限定文件。

实际隔离125：107752首次导入与107752重放均零隔离。首次对账定位到归档SQL错误列名，326b9dc2仅修正为field_digest/source_key_digest/payload_digest，无目标或receipt修改；随后双对账source/receipt/imported/verified均107752，同一seal 5e01d4c0019dbce4369eb40e0890fa4b0780b5a62bbfd9ce296dcf7c633541e5，完整PG对账回归27.76秒通过。effects/events/jobs始终0/0/0；正式schema118未变。Source-policy保持38。尚未通过本包候选/exact-main或正式部署。

验收依次为本地测试/实际隔离PG首次重放双对账、集中PR CI及最终精确候选Full、exact-main Full，再经备份后正式部署。历史数据不等于当前企微业务能力销项。V1绝对只读、零切流、真实外部效果关闭。
