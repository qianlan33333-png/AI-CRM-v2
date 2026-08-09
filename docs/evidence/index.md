# Evidence index

证据分层保存，不把 ChatGPT 对话或临时目录作为唯一事实源。

- `prototypes/`：被拒绝原型的脱敏结论、哈希和来源链接；不含源码包。
- `governance/`：仓库可见性、平台治理能力与 GitHub 操作证据；不改写历史证据。
- `phases/`：阶段累计 main SHA、最终 CI 与未验证外部门的权威 closeout。
- `slices/<slice-id>.md`：后续每片的输入/输出哈希、命令、退出码、PR 和
  外部门状态。
- `docs/execution/slice-ledger.yml`：机器可读的全局索引。

证据状态必须使用 `LOCAL`、`SYNTHETIC`、`STAGING`、`PRODUCTION`、
`NOT_EXECUTED` 或 `PENDING_EXTERNAL_GATE`，不得互相替代。
