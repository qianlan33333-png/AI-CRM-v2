# P4 B1-WC02 企微联系人资料写回效果闭环

- 精确基线：`origin/main@1d9526f53da227a11267b70c091d1288c8861422`。
- 行为边界：单一 V2 后端行为：remark/profile 写回接受、EER queue/attempt、持久 receipt、`outcome_unknown` 人工对账。没有前端、目录同步、标签扩展或真实 Provider 调用。
- 规模：7 个手写运行时/存储/Provider/migration 文件，共 1018 行；比 P4 默认 1000 行上限多 18 行。不能拆分：EER binding migration、状态机、仅 outbound 写入适配器与持久 receipt 是同一可观察的 Provider 效果闭环。
- 验证：focused race test、vet、`generate-check`、`migration-validate`、`arch-import-lint` 均在本地通过；迁移在本机 PostgreSQL 16.13 全量 up/down 通过。专用 PG16.14 脚本为 `acceptance/wecom/b1_wc02_profile_effect_pg16.sh`，本机未伪称其通过。
- 外部效果：`NOT_EXECUTED`；HTTP 测试仅使用 `httptest`，无真实企微写操作。
