# P4-MA-AB：消息归档完整 A+B 兼容板块

## 冻结输入

- fresh replay Base 为实时 exact-green `6f2ef91ad90c736b6dc3b685429ec40e5a7ae2e7`；独立 worktree 为
  `/Users/qianlan/Downloads/新CRM/worktrees/AI-CRM-v2-p4-message-archive-ab-replay-6f2ef91-20260815`。
  旧 Python/API/UI 仅从冻结 SHA `6cb989c071255437d75953dabb943318a74eb8f4` 读取路由、查询参数和响应键证据；前一轮
  本地提交 `187b8f7` 也仅作为冻结语义证据。两者均未 cherry-pick 或复制运行时。
- 一次完整闭合 8 条冻结 API：`LEGACY-API-0599/0600/0633/0682/0685/0687/0688/0689`。
  A 路由 `0633/0685` 和 B 路由 `0599/0600/0682/0687/0688/0689` 共同交付；P1
  `docs/api-mapping.jsonl` 的历史 MIGRATE/DEFERRED 决策保持字节不变，不伪造为 P1 已迁移。
- `LEGACY-T14-025 archive_sync_state`、`LEGACY-T14-026 archived_messages` 的 DROP 决策保持不变；不导入、复活或改写历史表。
  fresh replay 的 `00040_wecom_message_archive.sql` 只创建 WeCom-owned 本地投影和 accepted-only receipt；不改动已关闭订单的 `00039`。
- 完整 A+B 合同已明确授权且超出 P3/P4 常规 12 手写文件预算：本片 22 个手写/治理文件、无新产品能力，
  仅包含 8 路兼容 transport、既有 Identity/Contact/Events ports、SQLC、PG acceptance、权限与治理收据。

## 所有权、安全和外部边界

- `wecom_message_archive_records` 只保存脱敏消息文本（`content_masked`），不保存 UnionID、raw payload 或 tenant；
  `customer_id` 是经 OneID 和 Contact 归属检查的逻辑关联，不建跨域 FK。响应将 UnionID 清空、员工/会话标识掩码，
  并二次掩码手机号。
- 人类 session 延续 actor/capability：`message.archive.read`、`message.archive.execute`、
  `message.archive.external.read` 只允许 admin/ops global；搜索/历史使用既有 `customers.read` 和
  owner-scope Contact 校验。UnionID 查找跨所有合法 Identity binding，但双命中即 fail-closed，不猜 scope。
- `POST /api/archive/sync` 必须带 session-bound CSRF 和 `Idempotency-Key`。命令、receipt 和
  `wecom.message_archive_sync_accepted` event 同一 UoW；响应只表示 `accepted`，明确
  `side_effect_executed=false`、`real_external_call_executed=false`。没有 WeCom client、River worker、provider receipt、
  `queued` 执行宣称或 `outcome_unknown` 自动重试。
- 旧 `/api/messages/*/archive|history` 仅返回 410 兼容 envelope；没有新 UI、DTO 猜测、tenant 字段/索引/权限/测试、
  真实企微、生产写入、旧数据导入或部署。

## 本地验收与修正状态

- fresh replay 已两次重新生成 SQLC/shared，并通过 migration validate、focused normal/boundary/error/ownership/contract、
  session/RBAC/CSRF transport。重建后的 PG16.14 c07 `aicrm_test` 从 exact-main 的 39 真实执行
  `39→40→39→40`：records/receipts 存在性、无 tenant、无跨域 FK、有效约束/索引，且
  Event/Customer/Identity 历史指纹保持不变。
- `slice_induced=2`：首次 SQLC 聚合时间类型不能安全表达 nullable receipt 时间，改为显式最后 accepted 时间查询；
  第二次 focused test 发现 service 未复核 store 返回的 chat-type/keyword 约束，补为 fail-closed 校验。达到阈值后
  状态冻结为 **repair-only**，后续仅允许原 DoD、永久负例、生成物、ledger、集成、CI/PR/exact-main 修复。
- 上一轮的 `verification_induced=4` 仅作历史收据；本 fresh replay 的 verification 为：已关闭订单使 API/权限/迁移锚点变更，
  仅重放同一归档语义并将迁移安全顺延为 `00040`；fresh worktree 缺少锁定 Orval 后 bootstrap；旧 c07 的同号 `00038`
  历史与 fresh main 不兼容，故只重建本片指定的本地测试库；PR #224 首轮 CI 发现 7 个直接消费者
  （O6A/O6B1/O6B2、D01、L01、SI00B、Order A+B）仍将“当前全库水位”固定为 39，统一校正为 40；
  仅 Order A+B 增加最终 current 40 守卫，其既有 `38→39→38→39` 证明保持不变。未回滚或改写 `00039`。
- `PRODUCTION_DATABASE_NOT_EXECUTED`、`LIVE_MIGRATION_NOT_EXECUTED`、`REAL_WECOM_NOT_EXECUTED`、
  `REAL_EXTERNAL_EFFECT_NOT_EXECUTED`、`DEPLOYMENT_NOT_EXECUTED`。integration token 仅授权 GitHub 集成，不授权上述外部操作。
