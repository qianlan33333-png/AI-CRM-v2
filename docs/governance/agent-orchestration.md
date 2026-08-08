# Codex 与内部 Terra 执行编排

状态：`ACTIVE`

本规则取代未来网页 ChatGPT Pro 单片交付，只改变执行治理。P0-S01 的 URL 与 `pro_url`
仅为历史 provenance；旧 handoff protocol 不可用于新任务。

相关约束见 [AGENTS.md](../../AGENTS.md)、[CONTRIBUTING.md](../../CONTRIBUTING.md)、
[Slice 模板](../execution/slice-card-template.md) 和 [执行台账](../execution/slice-ledger.yml)。

## 职责

| 工作 | Codex root | Terra |
|---|---|---|
| 架构、中央契约、任务卡、拆片与 DAG | 独占冻结/批准 | 不得扩约 |
| 本地实现与测试 | 分派、复验 | 仅任务白名单/worktree |
| Git/GitHub、验收、rebase、PR、merge、main CI | 独占且串行 | 不得操作 |
| 部署、真实迁移或外部调用 | 仅按既有授权 | 不得执行 |

root 非业务逻辑直接改动最多 20 行。中央合同 Terra 任务只可机械实现 root 冻结的
逐文件合同；业务 Slice 不得改中央契约。

## 派发与执行

任务卡必须 self-contained，记录 40 位 base SHA、稳定 task id、单一可观察目标、依赖
已满足状态、逐项允许/禁止路径、绝对 worktree、验收/外部门，以及
`executor_model=gpt-5.6-terra`、`reasoning_effort=ultra`。

最多 3 个 Terra 任务 active；仅依赖满足且白名单不重叠时按 DAG 并行。Terra 只在
分配 worktree 修改/测试，不得 stage、commit、push、PR、rebase、merge、部署、真实
迁移/外部调用、读取凭据或制作外部上传件。

失败先由同一 task follow-up 修正；连续两次同根因失败、越界或扩约即拒收重拆。不得
新建、上传或续接网页 ChatGPT Pro 对话；历史 URL 不构成未来指令。

## 回执与台账

Terra 对全部 `payload_paths`（含 untracked）按相对 PATH 的 `LC_ALL=C` 升序交回：

```text
MODE BYTES SHA256 PATH
```

`docs/execution/slice-ledger.yml` 与 `docs/evidence/slices/<ID>.md` 是不参加 payload
hash 的 receipt carriers。root stage 后仅对 payload 运行
`git diff --cached --binary <base_sha> -- <payload_paths...>`；其原始输出的 SHA-256
是 canonical `diff_sha256`，PR head/merge SHA 覆盖 carriers 完整性。

```yaml
execution_mode: internal_terra
executor_task_id: /root/example_task
executor_model: gpt-5.6-terra
reasoning_effort: ultra
worktree: /absolute/path/to/worktree
payload_paths: [path/to/implementation]
receipt_carrier_paths: [docs/execution/slice-ledger.yml, docs/evidence/slices/<ID>.md]
file_manifest_sha256: null
diff_sha256: null
```

未执行写 `NOT EXECUTED`，未授权外部门写 `PENDING_EXTERNAL_GATE`；local/mock/synthetic
不得称生产验证。本规则不授予部署、真实迁移、服务器、凭据、企微或真实数据权限。
