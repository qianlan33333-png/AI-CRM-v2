# V1 Contact 历史最小正式落表

- 只读V2封存来源：sidebar59、owner results34、sessions2、previews35；不连接V1。前两者迁入Contact-owned独立历史表，后两者只作结果关联上下文并保留归档终态，不恢复token、会话权限或执行。
- 冻结manifest中sidebar仅7列、无主键、没有created_at；不能照本地较新V1 DDL猜补字段或假设unionid唯一。以source_key_hmac作为来源唯一键，原payload digest绑定所有归档字段。
- Sidebar只保留原业务文本和updated_at。customer_id仅由严格DM01 unionid lineage/receipt/actual-root解析，缺映射NULL；updated_by不猜Staff，不覆盖当前Sidebar CAS/profile。
- Owner结果保留范围、原hash、计数、欢迎语、输入选项和原时间。V1 wecom_success等只是历史记录，不表示V2执行或Provider成功。原result/job/session/userid、展示名/operator、rows/stats JSON仍在加密archive，不能变成可执行参数。
- 空session是V1允许的值，session_relation和preview_relation保持unresolved；只有原session存在、preview.executed_result_id指向同结果且session一致时才能resolved。不得由空session反推关系。
- Schema116、Port、SQLc、OpenAPI、中央注册由主代理串行。Writer+receipt同caller事务，重放重新读取实际目标并比对全字段摘要。四张来源共130条必须逐行守恒（93业务候选+37归档上下文），不能用计数冒充对账。
- 历史GET/页面独立只读，分页、空态、失败关闭；无迁移执行/同步/发送入口。候选与exact-main Full Nightly通过后方能在V2正式迁移；目前只是开发准备。
