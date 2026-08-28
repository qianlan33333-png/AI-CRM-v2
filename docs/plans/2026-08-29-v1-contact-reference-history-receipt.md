# 联系人引用历史隔离演练收单

本记录只证明 schema135 的历史事实迁移与只读行为，不代表当前目录、权限或负责人投影已重建，也不代表正式部署完成。

| 冻结来源 | 行数/字段数 | V2 历史目标 |
|---|---|---|
| external_contact_bindings | 1370 / 7 | contact_v1_external_binding_history |
| admin_wecom_directory_members | 44 / 19 | contact_v1_directory_member_history |

旧 migration mapping 的 5/17 字段清单早于本次冻结来源；保留其 REBUILD 决策，不把当前 staff 授权、identity assurance 或 owner 迁移误销项。以上历史目标独立承接完整冻结字段（私密字段保留 HMAC，原加密归档保留）。现有配置历史页增加 binding/directory 类型，四个 admin-read GET 提供列表和详情，失败不回退 Mock。

## 已执行的隔离验证

- 源代码 0d33eafc71f8cfd48ab6a881e94393f07528feb4；后续 ad18fca3 仅同步真实生成物哈希清单。
- 在 V2 network=none 演练容器的独立 aicrm_contact135 数据库，从 schema134 执行135。其他三个恢复演练数据库未改。
- Contact store 的真实 PostgreSQL caller-transaction/read/rollback 测试通过；首次 selected/imported=1414，重放 imported=0/replayed=1414。
- 两次 reconcile 均 selected/receipt/imported/verified=1414、archive/quarantine=0，第二次 replayed=true；摘要 `190a28f3e0031e3d9c571fa462516540b79c705ba0fe3f486cc00f978d8d8a0d`。
- 所有复制参考表的 binary COPY 数量/哈希前后一致，包括旧 archive/DM01/deferred-person 回执；customers/identities/staff 与任务、效果、事件和队列未改。正式库 schema134/启动时间未变。演练容器已停止、network=none。
- 本次仅从 V2 冻结数据读取，没有连接或修改 V1，没有切换旧流量，没有真实 Provider 效果。

远端运行记录：`/home/ubuntu/p4-contact-reference-history/rehearsal.log`，SHA256 `b6c1e970ebd585fea7a9f954c7c23556df125ebd453c087b51f6eee6abc454f5`。独立结果位于 `/data/aicrm/contact135-rehearsal/evidence-0d33eafc71f8/`；reconcile-replay.json SHA256 `e67a18cf72d8b48ef57c5804b50e3c6cba70b215880ae47622c8747fd9d7b392`。

本地验证：CLI/Contact/Identity/import-core/selector 测试与 race、OpenAPI 契约、两遍生成一致性、ownership/source-policy/architecture、迁移检查、typecheck、npm test（主 E2E 390/0）和 Web build 均通过。

## 尚未执行

本包候选 Full、合并后 exact-main Full、正式135迁移与独立入口部署尚待门禁后执行。严格 Excel 仍为256/286，本历史包不销项当前目录同步、权限传播、HXC 当前漏斗或群发。Sidebar 的平台入口、独立回调、config/agentConfig 与企微内打开仍未验收。
