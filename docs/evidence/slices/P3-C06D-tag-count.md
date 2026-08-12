# P3-C06D 标签有界总数证据

## 触发证据

- source main SHA：`5d5b2a4ef00f10f19b42b184d6cbb2a24c1825d2`
- application main CI：`https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31581541139`
- evidence class：`authorized_test_server_synthetic`
- runner exit：`1`
- offline verifier exit：`1`
- receipt：SHA-256 `0ecac38fb876f32caa0869cafe1048e40fe00673719f3b3d3dc73d61c189ddb6`，
  `67,346,022` bytes
- matrix：4,096 combinations / 81,920 measured samples
- result：`HARD_GATE_FAILED`
- global P50/P95/max：`16.846982 / 106.508798 / 563.411078 ms`
- per-case P95 `>=200ms`：78，selector masks 仅 `16(tag)` / `17(tag+keyword)`
- slowest：`selectors-17-deleted-false-added-before-interact-closed-page-next-limit-50`，
  P95 `478.294753ms`

全局 P95 不是通过条件；runner/verifier 的失败结果优先。完整 receipt 保留在授权
测试机私有证据目录，不纳入仓库，且不包含 DSN。

## 根因与候选计划

最慢场景的 raw EXPLAIN 摘要：

- `CountCustomerIDsBounded` execution `324.592ms`，shared hit `573,569`
- customers `idx_customers_deleted_keyset` 实际返回 4,227、filter 移除 185,773
- customer_tags `idx_customer_tags_tag` 返回 12,739
- `ListCustomers` execution `3.928ms`
- 两条计划均无目标 Seq Scan

相同授权测试数据上的只读候选 EXPLAIN，以标签索引驱动 lateral customers PK
查找，两次 execution 为 `47.571ms` 与 `50.346ms`，且无目标 Seq Scan。这是
优化方向证据，不冒充新的
4,096 场景最终通过结果；新 main 的完整整轮仍为 `NOT EXECUTED`。

## Terra 机械实现

- task：`p3_c06d_sqlc_optimization`
- frozen base：`5d5b2a4ef00f10f19b42b184d6cbb2a24c1825d2`
- delegated head：`1e74fd56d7ec612e6f371d818770941764248468`
- manifest SHA-256：`e6d519f1b7e0bb9aab0046c29048beda42b61eef4dea56a1671a8ab0d4e044e3`
- status：`SOL_REVIEW_INTEGRATED`

Terra 未 push/PR/merge；Sol 独立同步 acceptance 合同、生成物清单、全量门禁与
最终发布。
