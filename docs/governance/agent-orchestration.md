# Codex Sol 与按需 Terra 执行编排

状态：`ACTIVE`

本规则取代“P0 默认 Terra 实现”和未来网页 ChatGPT Pro 交付。P0-S01 的 URL、
`pro_url` 及已接受 Terra 回执仅为历史 provenance；不可用于新 P0 任务。

相关约束见 [AGENTS.md](../../AGENTS.md)、[CONTRIBUTING.md](../../CONTRIBUTING.md)、
[Slice 模板](../execution/slice-card-template.md) 和 [执行台账](../execution/slice-ledger.yml)。

## 职责

| 工作 | Codex Sol | 按需 Terra/独立 Agent |
|---|---|---|
| 架构、中央契约、任务卡、拆片与 DAG | 独占裁决 | 不得扩约 |
| P0 实现与测试 | 单 worktree 端到端完成 | 不启用 |
| 后续独立实现/调查/红队 | 冻结契约、分派、复验 | 仅白名单/worktree |
| Git/GitHub、验收、rebase、PR、merge、main CI | 非共享业务 PR 可并行验收；中央契约与 merge/main CI 串行 | 默认不操作 |
| 部署、真实迁移或外部调用 | 仅按既有授权 | 不得执行 |

## 垂直 Slice

每个 Slice 记录 40 位 base SHA、单一可观察目标、路径边界、契约、适用门禁与外部门。
P0 中 Sol 直接在同一 Slice 内完成必要中央契约、实现、作者测试、黑盒验收、
CI 接线、修正和 GitHub 全流程；不另立小型中间合同 PR、Terra 回执或上传包。

一个完整可观察行为尽量一个 PR。若超过 8 个手写文件、400 行手写 diff 或需要两个
独立公开行为，按可单独验收的行为拆分，不按“契约/实现/回执”层次拆分。

互不依赖、路径不重叠且不改共享契约的业务 PR 可并行运行门禁；Sol 必须按累计 main
串行执行中央契约裁决、最终 rebase、squash merge 和精确 main SHA CI。当前仓库虽已
公开，但尚未配置 GitHub Ruleset 或 branch protection，不得把流程纪律写成“已保护
分支”。

## 分阶段协作

- P0：单 Sol 端到端完成骨架。
- P1：可将互不依赖的事实盘点交 Terra 分组并行，Sol 汇总与裁决。
- P2：Sol 主做共享平台核心，孤立组件按需委派。
- P3/P4：在 API/公共契约冻结后，恢复 Sol 指挥与 Terra 并行；每个 PR 必须关闭
  ledger 官方业务 Slice，或关闭经用户/权威计划批准且能在 feature matrix 定位的
  完整业务 flow。禁止 parser/checker/governance-only PR；本次业务交付优先策略迁移
  是唯一例外，合并后例外关闭。
- 迁移与对账：必须由与实现者独立的 Agent 执行复核，不允许实现者自证。

并行实现须至少有两个互不依赖、路径不重叠、单任务足以覆盖交接成本的任务，且
公共契约已冻结。最多 3 个 Terra task。独立红队复核可作为单独委派，不参与主实现。

## 修正与交付归因

- 只有 `slice_induced` 参与降档与硬停：达到 2 时冻结范围并降档，允许当前片在不扩
  scope 下完成既定闭环；达到 3 时立即停报并重切更小业务片。
- `infra_induced` 与 `verification_induced` 精确记录但不降档、不硬停。机械环境、
  命令和测试夹具时序在原任务内修复；只有涉及共享基础设施或业务范围才另片。
- 预期生成物及既有 hash、manifest、ledger receipt 正常同步属于 Definition of Done；
  首次遗漏被门发现才记一次 `verification_induced`，并在原任务补齐。
- 独立安全片只限不可逆数据污染、鉴权、迁移或真实外发的明确风险；其余安全工作
  优先随业务垂直片完成。

## 委派边界

委派任务必须 self-contained，记录 task id、model/reasoning、绝对 worktree、精确 base SHA、
依赖和逐文件白名单。除非任务卡明确授权，代理不得 stage、commit、push、PR、rebase、
merge、部署、真实迁移/外部调用、读取凭据或制作外部上传件。

委派执行者不得改变 Sol 冻结的契约。失败时在同一任务返工；连续两次同根因失败、越界
或需要改公共契约时，由 Sol 拒收并重拆。不得新建、上传或续接网页 ChatGPT Pro 对话。

## 证据与台账

Sol 垂直 Slice 以 base SHA、PR head、squash merge SHA、精确 main SHA 的三条 CI、测试日志和
外部门状态作为主证据。委派任务仍对全部 `payload_paths`（含 untracked）按相对 PATH 的
`LC_ALL=C` 升序交回：

```text
MODE BYTES SHA256 PATH
```

`docs/execution/slice-ledger.yml` 与 `docs/evidence/slices/<ID>.md` 可作委派任务的 receipt
carriers；Sol 端到端 Slice 不要求中间 receipt。未执行写 `NOT EXECUTED`，未授权外部门写
`PENDING_EXTERNAL_GATE`；local/mock/synthetic 不得称生产验证。

本规则不授予部署、真实迁移、服务器、凭据、企微或真实数据权限。
