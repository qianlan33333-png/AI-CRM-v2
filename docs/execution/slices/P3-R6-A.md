# P3-R6-A：Receipt safety gate governance only

## 输入与目标

- Base SHA：`f8bb43739f31b22a08fa8a533c67852de20dad21`。
- 前置：P3-R5-C1 已 CLOSED；优先的 Contact R3B #160 已合并，且该 exact main 的
  application/go、repo-contract、secret-scan 三门 required CI 全绿，共享 PR 队列为空。
- 目标：只冻结 R6-A/B/C/D 的边界、顺序与旧 R5-C2 的 superseded 映射，并以前向
  ledger 新条目证明全部历史条目零漂移。

## 允许路径

- `docs/execution/slices/P3-R6-A.md`
- `docs/execution/slice-ledger.yml`
- `scripts/check_repo_contract.sh`（只维护 ledger 的既有 SHA 锚点）

## 旧 R5-C2 superseded 映射

- `/private/tmp/aicrm-v2-r5-c2-receipt-pending-boundary` 及其中 WIP 是
  `HARD STOP / READ ONLY` 历史证据；不得继续、修复、清理、整片 cherry-pick、提交、
  push、PR、发布或清零其修正历史。
- 旧 R5-C2 的目标由 `P3-R6-A → P3-R6-B → P3-R6-C → P3-R6-D` 取代；这是
  rescope/superseded 映射，不表示旧候选已验收或可复用。
- 旧候选的三项 slice-induced 缺陷保持原样：多行约束续行误分类、初始 card 漏既有
  ledger SHA 维护、contains 锚点无法识别后续 `DROP CONSTRAINT` 削弱最终闭集。

## R6 冻结边界与严格顺序

- R6-B parser-foundation：只实现可移植的 SQL 顶层 DDL 与最终有效表约束 canonical
  模型及单元负例；必须处理多行约束和后续 `DROP CONSTRAINT`。不得编码
  receipt/pending 政策，不改 DDL、API 或 runtime。
- R6-C policy-gates：只基于已 CLOSED 的 R6-B 实现 receipt/pending 永久政策负例；
  不扩展 parser，不改 DDL、API 或 runtime。
- R6-D wiring-only：只把已 CLOSED 的 checker/tests 接入 repo-contract/CI，并维护精确
  ledger/hash；不得新增 parser 或政策能力。
- 必须串行 `A → B → C → D`，每片独立 PR、match-head squash、父/树证明与 exact-main
  三门 CI；任一片 `slice_induced >= 3` 立即 HARD STOP 并报告，不在原片续修。
- Contact R3B 占队列期间本片只形成了 clean local candidate；#160 CLOSED 后才重放。

## 历史 ledger 零漂移证明

使用既有结构化 YAML checker 将 base/candidate 映射成 `slice_id → canonical JSON`；
candidate 只可新增 `P3-R6-A`，全部 base 条目必须逐项完全相同：

```sh
ruby scripts/check_slice_ledger_history.rb \
  --base f8bb43739f31b22a08fa8a533c67852de20dad21:docs/execution/slice-ledger.yml \
  --candidate :docs/execution/slice-ledger.yml \
  --assert-unchanged P3-R5-C1
```

## 明确排除与回滚

- 不写或修改 parser、checker 逻辑、测试、业务代码、DDL、ADR、OpenAPI、ports、API、
  runtime、Make、依赖、CI workflow；不接线任何 focused checker。
- 不连接生产数据库，不执行 live migration、真实企微或外发。
- 回滚仅为 revert 本 PR；R6-B 必须等待本片 CLOSED 与 exact-main 三门 CI 全绿。
