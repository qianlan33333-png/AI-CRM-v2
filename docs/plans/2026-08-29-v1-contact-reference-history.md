# V1 客户绑定与员工目录最小正式落表

基线：已通过候选与 exact-main Full 的5f4b9f48。本包属于既定全量迁移，不扩大产品范围。源为已封存的 external_contact_bindings 1370条和 admin_wecom_directory_members 44条，不连接或修改V1。

## 选择与边界

采用 Contact-owned 不可变历史事实和真实列表/详情读取。直接导入 staff 会改变当前授权且丢失历史字段；仅保留归档不满足业务读取。两者均不采用。复用既有导入回执、对账及 Contact 历史页结构，不建新平台或调度。

- binding：完整来源摘要、原 person/owner/time 字段的历史表示；可选 exact scoped identity（当前1367条为 declared，不升级）。person 只经已验证 deferred-person 历史回执关联，不能把旧 person_id 当 Customer ID；无确定关联保留 NULL 及原因。
- directory：历史目录字段、原两个 corp 事实、时间与可选的已存在 staff 关联；不新增或更新 staff，不授予登录/角色，不修改 owner。企业归属不明确的1条保持未归属。
- 敏感标识按既有历史策略保持私有或摘要；API仅给人工核对所需事实，不公开原始 payload。保留归档，不丢信息。
- 不写当前 customers、identities、权限或发送任务，不产生 event、River 或 Provider 效果；不把历史落表当作当前 HXC 完成。

## 分工和验证

1. adapter leaf 仅负责独立来源转换和单测；主代理串行集成表结构、Port、API、生成客户端和中央文件。每条源核验序号、source/payload/field HMAC 及旧回执。
2. 在隔离 PG 验证选择1414条、导入、零新增重放、两次同摘要对账，以及旧回执、当前业务和队列不变；缺失关联显式保留，不猜测。
3. 一个集中 PR 完成领域/行为测试，最终精确 SHA 候选 Full 绿灯后合并；exact-main 绿灯后再正式备份、迁移、导入、双对账和独立验收部署。

范围不包含当前 staff 同步、授权传播、负责人转接、HXC 当前权益、真实群发或支付退款。继续遵守 V1 零修改、旧流量零切换。
