# P3-C07C-R3A：Contact external-event append-only governance

## 输入与目标

- Base SHA：`d3af4cfd14bd002f409c3dbdff6265db4cd8a808`
- 依赖：该 SHA 的 application/go、repo-contract、secret-scan 都成功，且共享 PR 队列为空。
- 目标：只建立 C07C 的 append-only 治理边界，阻止对历史 slice ledger 条目的误命中修改。

## 允许路径

- `docs/execution/slices/P3-C07C-R3A.md`
- `docs/execution/slice-ledger.yml`
- `scripts/check_slice_ledger_history.rb`
- `scripts/check_repo_contract.sh`
- `scripts/test_repo_contract.sh`

## R3B / R3C 冻结边界

- R3B 仅在 R3A CLOSED 的新 exact main 上开始：`00011` registry DDL/down、Contact
  ownership/canonical、catalog/constraint/rollback PostgreSQL 验收与最小 migration
  hash/owner 门；不得修改 `AppendExternalEvent` runtime、port、store 或 sqlc。
- R3C 仅在 R3B CLOSED 的新 exact main 上开始：transaction-bound
  `AppendExternalEvent`、advisory lock、same key/fact/root replay、冲突、10-way
  concurrency、merge replay 与 rollback；不得修改 migration 或 ownership。
- 原 C07C、R4B replay 与 R1 均为 HARD STOP 只读证据，禁止 cherry-pick、继续、发布或
  改写其历史；历史修正计数不得清零。

## 历史条目零漂移收据

`scripts/check_slice_ledger_history.rb` 用结构化 YAML 解析，将 base 和 candidate
分别转为 `slice_id → canonical JSON` 映射。除 candidate 新增的 slice_id 外，每一条
历史条目必须完全相同；不使用行号、首次匹配或文本替换。R3A 另显式断言 `M0-7`、`P2-04`
与 `P3-S01` 不变：

```sh
ruby scripts/check_slice_ledger_history.rb \
  --base d3af4cfd14bd002f409c3dbdff6265db4cd8a808:docs/execution/slice-ledger.yml \
  --candidate :docs/execution/slice-ledger.yml \
  --assert-unchanged M0-7 \
  --assert-unchanged P2-04 \
  --assert-unchanged P3-S01
```

## 明确排除与回滚

- 不修改 migration、ownership、canonical、Contact runtime、ports、store、sqlc、API、UI、
  OpenAPI、Identity、Segment、event_log 或外部效果。
- 不执行生产数据库、live migration、真实企微或外发；回滚仅为 revert 本 PR。
